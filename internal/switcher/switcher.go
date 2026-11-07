package switcher

import (
	"fmt"
	"gemini-switcher/internal/credmgr"
	"gemini-switcher/internal/model"
	"gemini-switcher/internal/quota"
	"gemini-switcher/internal/storage"
	"sort"
	"sync"
	"time"
)

type Switcher struct {
	storage *storage.Storage
	mu      sync.Mutex
}

func NewSwitcher(s *storage.Storage) *Switcher {
	return &Switcher{storage: s}
}

func (sw *Switcher) CaptureCurrentAccount() (*model.AccountProfile, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	blob, err := credmgr.ReadAntigravityCredential()
	if err != nil {
		return nil, fmt.Errorf("read credential failed: %w", err)
	}

	email, err := credmgr.ParseSessionEmail(blob)
	if err != nil {
		return nil, fmt.Errorf("parse email failed: %w", err)
	}

	accs, err := sw.storage.LoadAccounts()
	if err != nil {
		return nil, err
	}

	var found *model.AccountProfile
	for _, a := range accs {
		if a.Email == email {
			found = a
			break
		}
	}

	now := time.Now().UTC()
	if found == nil {
		found = &model.AccountProfile{
			Email: email,
			Quota5H: model.QuotaBucket{
				RemainingFraction: 1.0,
				Window:            "5h",
			},
			QuotaWeekly: model.QuotaBucket{
				RemainingFraction: 1.0,
				Window:            "weekly",
			},
		}
		accs = append(accs, found)
	}

	found.CredentialBlob = blob
	found.LastSwitchedAt = now
	found.Status = model.StatusActive

	for _, a := range accs {
		if a.Email != email && a.Status == model.StatusActive {
			a.Status = model.StatusReady
		}
	}

	qRes, qErr := quota.FetchActiveQuota()
	if qErr == nil && qRes != nil {
		found.Quota5H = qRes.Quota5H
		found.QuotaWeekly = qRes.QuotaWeekly
	}

	if err := sw.storage.SaveAccounts(accs); err != nil {
		return nil, err
	}

	return found, nil
}

func (sw *Switcher) RotateIfNeeded(activeEmail string, qRes *quota.QuotaResult) (*model.AccountProfile, error) {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	cfg, err := sw.storage.LoadConfig()
	if err != nil || !cfg.AutoSwitchEnabled {
		return nil, nil
	}

	if activeEmail == "" || qRes == nil {
		return nil, nil
	}

	now := time.Now().UTC()
	accs, err := sw.storage.LoadAccounts()
	if err != nil {
		return nil, err
	}

	var current *model.AccountProfile
	for _, a := range accs {
		if a.Email == activeEmail {
			current = a
			break
		}
	}

	if current == nil {
		return nil, nil
	}

	current.Quota5H = qRes.Quota5H
	current.QuotaWeekly = qRes.QuotaWeekly
	if qRes.Quota5H.RemainingFraction <= cfg.Threshold5H {
		current.Status = model.StatusCooldown5H
	} else if qRes.QuotaWeekly.RemainingFraction <= cfg.ThresholdWeekly {
		current.Status = model.StatusExhaustedWeekly
	} else {
		current.Status = model.StatusActive
	}
	_ = sw.storage.SaveAccounts(accs)

	needsSwitch := (qRes.Quota5H.RemainingFraction <= cfg.Threshold5H) || (qRes.QuotaWeekly.RemainingFraction <= cfg.ThresholdWeekly)
	if !needsSwitch {
		return nil, nil
	}

	best := sw.selectBestCandidate(accs, activeEmail, now, cfg.Threshold5H, cfg.ThresholdWeekly)
	if best == nil {
		return nil, fmt.Errorf("all accounts are currently cooling down")
	}

	if err := sw.applySwitch(best, accs); err != nil {
		return nil, err
	}
	return best, nil
}

func (sw *Switcher) SwitchTo(target *model.AccountProfile) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	accs, err := sw.storage.LoadAccounts()
	if err != nil {
		return err
	}

	var match *model.AccountProfile
	for _, a := range accs {
		if a.Email == target.Email {
			match = a
			break
		}
	}
	if match == nil {
		match = target
		accs = append(accs, match)
	}

	return sw.applySwitch(match, accs)
}

func (sw *Switcher) DeleteAccount(email string) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	accs, err := sw.storage.LoadAccounts()
	if err != nil {
		return err
	}

	var updated []*model.AccountProfile
	for _, a := range accs {
		if a.Email != email {
			updated = append(updated, a)
		}
	}
	return sw.storage.SaveAccounts(updated)
}

func (sw *Switcher) UpdateAccount(email, alias string, paused bool, priority int) error {
	sw.mu.Lock()
	defer sw.mu.Unlock()

	accs, err := sw.storage.LoadAccounts()
	if err != nil {
		return err
	}

	for _, a := range accs {
		if a.Email == email {
			a.Alias = alias
			a.Paused = paused
			a.Priority = priority
			break
		}
	}
	return sw.storage.SaveAccounts(accs)
}

func (sw *Switcher) applySwitch(target *model.AccountProfile, accs []*model.AccountProfile) error {
	if err := credmgr.WriteAntigravityCredential(target.CredentialBlob); err != nil {
		return fmt.Errorf("write credential failed: %w", err)
	}

	_ = credmgr.UpdateAppStorageUsername(target.Email)

	now := time.Now().UTC()
	target.LastSwitchedAt = now
	target.Status = model.StatusActive

	for _, a := range accs {
		if a.Email != target.Email && a.Status == model.StatusActive {
			a.Status = model.StatusReady
		}
	}

	_ = sw.storage.SaveAccounts(accs)
	_ = RestartAntigravityLanguageServer()
	return nil
}

func (sw *Switcher) selectBestCandidate(accs []*model.AccountProfile, currentEmail string, now time.Time, thresh5h, threshWk float64) *model.AccountProfile {
	var eligible []*model.AccountProfile
	for _, a := range accs {
		if a.Email == currentEmail {
			continue
		}
		if a.IsEligible(now, thresh5h, threshWk) {
			eligible = append(eligible, a)
		}
	}

	if len(eligible) == 0 {
		return nil
	}

	sort.Slice(eligible, func(i, j int) bool {
		pI := eligible[i].Priority
		pJ := eligible[j].Priority
		if pI > 0 && pJ > 0 && pI != pJ {
			return pI < pJ
		}
		if pI > 0 && pJ == 0 {
			return true
		}
		if pI == 0 && pJ > 0 {
			return false
		}

		remI := eligible[i].Quota5H.RemainingFraction
		if !eligible[i].Quota5H.ResetTime.IsZero() && now.After(eligible[i].Quota5H.ResetTime) {
			remI = 1.0
		}
		remJ := eligible[j].Quota5H.RemainingFraction
		if !eligible[j].Quota5H.ResetTime.IsZero() && now.After(eligible[j].Quota5H.ResetTime) {
			remJ = 1.0
		}

		if remI != remJ {
			return remI > remJ
		}
		return eligible[i].LastSwitchedAt.Before(eligible[j].LastSwitchedAt)
	})

	return eligible[0]
}
