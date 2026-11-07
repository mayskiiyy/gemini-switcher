package switcher

import (
	"gemini-switcher/internal/model"
	"gemini-switcher/internal/quota"
	"gemini-switcher/internal/storage"
	"os"
	"testing"
	"time"
)

func TestSelectBestCandidate(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "switcher-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	sw := NewSwitcher(s)

	now := time.Now()
	acc1 := &model.AccountProfile{
		Email:          "acc1@example.com",
		Quota5H:        model.QuotaBucket{RemainingFraction: 0.02, ResetTime: now.Add(2 * time.Hour)},
		QuotaWeekly:    model.QuotaBucket{RemainingFraction: 0.8},
		LastSwitchedAt: now.Add(-3 * time.Hour),
	}
	acc2 := &model.AccountProfile{
		Email:          "acc2@example.com",
		Quota5H:        model.QuotaBucket{RemainingFraction: 0.70},
		QuotaWeekly:    model.QuotaBucket{RemainingFraction: 0.9},
		LastSwitchedAt: now.Add(-2 * time.Hour),
	}
	acc3 := &model.AccountProfile{
		Email:          "acc3@example.com",
		Quota5H:        model.QuotaBucket{RemainingFraction: 0.95},
		QuotaWeekly:    model.QuotaBucket{RemainingFraction: 0.9},
		LastSwitchedAt: now.Add(-1 * time.Hour),
	}
	accs := []*model.AccountProfile{acc1, acc2, acc3}

	best := sw.selectBestCandidate(accs, "acc1@example.com", now, 0.05, 0.05)
	if best == nil || best.Email != "acc3@example.com" {
		t.Fatalf("expected acc3 to be selected, got: %+v", best)
	}

	bestNoExclude := sw.selectBestCandidate(accs, "acc2@example.com", now, 0.05, 0.05)
	if bestNoExclude == nil || bestNoExclude.Email != "acc3@example.com" {
		t.Fatalf("expected acc3 to be selected, got: %+v", bestNoExclude)
	}

	accExhaustedWeekly := &model.AccountProfile{
		Email:       "exhausted@example.com",
		Quota5H:     model.QuotaBucket{RemainingFraction: 0.99},
		QuotaWeekly: model.QuotaBucket{RemainingFraction: 0.01, ResetTime: now.Add(24 * time.Hour)},
	}
	accExpiredCooldown := &model.AccountProfile{
		Email:       "expired@example.com",
		Quota5H:     model.QuotaBucket{RemainingFraction: 0.01, ResetTime: now.Add(-5 * time.Minute)},
		QuotaWeekly: model.QuotaBucket{RemainingFraction: 0.8},
	}
	accsWithCooldowns := []*model.AccountProfile{accExhaustedWeekly, accExpiredCooldown}

	bestCooldown := sw.selectBestCandidate(accsWithCooldowns, "none@example.com", now, 0.05, 0.05)
	if bestCooldown == nil || bestCooldown.Email != "expired@example.com" {
		t.Fatalf("expected expired to be selected as reset passed, got: %+v", bestCooldown)
	}

	allIneligible := []*model.AccountProfile{acc1, accExhaustedWeekly}
	if sw.selectBestCandidate(allIneligible, "acc1@example.com", now, 0.05, 0.05) != nil {
		t.Fatalf("expected nil when all accounts are ineligible")
	}

	accTie1 := &model.AccountProfile{
		Email:          "tie1@example.com",
		Quota5H:        model.QuotaBucket{RemainingFraction: 0.80},
		QuotaWeekly:    model.QuotaBucket{RemainingFraction: 0.80},
		LastSwitchedAt: now.Add(-4 * time.Hour),
	}
	accTie2 := &model.AccountProfile{
		Email:          "tie2@example.com",
		Quota5H:        model.QuotaBucket{RemainingFraction: 0.80},
		QuotaWeekly:    model.QuotaBucket{RemainingFraction: 0.80},
		LastSwitchedAt: now.Add(-1 * time.Hour),
	}
	bestTie := sw.selectBestCandidate([]*model.AccountProfile{accTie1, accTie2}, "", now, 0.05, 0.05)
	if bestTie == nil || bestTie.Email != "tie1@example.com" {
		t.Fatalf("expected least recently switched accTie1 to be selected, got: %+v", bestTie)
	}
}

func TestRotateIfNeeded_NoSwitchWhenLimitsSufficient(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "switcher-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	acc1 := &model.AccountProfile{
		Email:       "active@example.com",
		Quota5H:     model.QuotaBucket{RemainingFraction: 0.50},
		QuotaWeekly: model.QuotaBucket{RemainingFraction: 0.80},
		Status:      model.StatusActive,
	}
	acc2 := &model.AccountProfile{
		Email:       "backup@example.com",
		Quota5H:     model.QuotaBucket{RemainingFraction: 1.00},
		QuotaWeekly: model.QuotaBucket{RemainingFraction: 1.00},
		Status:      model.StatusReady,
	}
	_ = s.SaveAccounts([]*model.AccountProfile{acc1, acc2})

	sw := NewSwitcher(s)

	qRes := &quota.QuotaResult{
		Quota5H:     model.QuotaBucket{RemainingFraction: 0.45},
		QuotaWeekly: model.QuotaBucket{RemainingFraction: 0.75},
	}
	switched, err := sw.RotateIfNeeded("active@example.com", qRes)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if switched != nil {
		t.Fatalf("should not switch when active account has 45%% quota, got: %+v", switched)
	}

	switchedUnknown, err := sw.RotateIfNeeded("unknown@example.com", qRes)
	if err != nil || switchedUnknown != nil {
		t.Fatalf("should not switch when email is not in pool")
	}

	switchedNil, err := sw.RotateIfNeeded("active@example.com", nil)
	if err != nil || switchedNil != nil {
		t.Fatalf("should not switch when qRes is nil")
	}
}

func TestDeleteAndUpdateAccount(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "switcher-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}

	acc1 := &model.AccountProfile{Email: "user1@example.com"}
	acc2 := &model.AccountProfile{Email: "user2@example.com"}
	_ = s.SaveAccounts([]*model.AccountProfile{acc1, acc2})

	sw := NewSwitcher(s)

	err = sw.UpdateAccount("user1@example.com", "Личный", true, 1)
	if err != nil {
		t.Fatalf("failed to update account: %v", err)
	}

	accs, _ := s.LoadAccounts()
	if accs[0].Alias != "Личный" || !accs[0].Paused || accs[0].Priority != 1 {
		t.Fatalf("unexpected account state: %+v", accs[0])
	}

	err = sw.DeleteAccount("user1@example.com")
	if err != nil {
		t.Fatalf("failed to delete account: %v", err)
	}

	accs, _ = s.LoadAccounts()
	if len(accs) != 1 || accs[0].Email != "user2@example.com" {
		t.Fatalf("unexpected remaining accounts: %+v", accs)
	}
}

func TestPriorityCandidateSelection(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "switcher-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := storage.NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("failed to create storage: %v", err)
	}
	sw := NewSwitcher(s)

	now := time.Now()
	accLowPriorityHighQuota := &model.AccountProfile{
		Email:       "prio2@example.com",
		Quota5H:     model.QuotaBucket{RemainingFraction: 0.99},
		QuotaWeekly: model.QuotaBucket{RemainingFraction: 0.99},
		Priority:    2,
	}
	accHighPriorityLowerQuota := &model.AccountProfile{
		Email:       "prio1@example.com",
		Quota5H:     model.QuotaBucket{RemainingFraction: 0.50},
		QuotaWeekly: model.QuotaBucket{RemainingFraction: 0.99},
		Priority:    1,
	}
	accPaused := &model.AccountProfile{
		Email:       "paused@example.com",
		Quota5H:     model.QuotaBucket{RemainingFraction: 1.00},
		QuotaWeekly: model.QuotaBucket{RemainingFraction: 1.00},
		Priority:    1,
		Paused:      true,
	}

	best := sw.selectBestCandidate([]*model.AccountProfile{accLowPriorityHighQuota, accHighPriorityLowerQuota, accPaused}, "", now, 0.05, 0.05)
	if best == nil || best.Email != "prio1@example.com" {
		t.Fatalf("expected priority 1 account to be selected, got: %+v", best)
	}
}
