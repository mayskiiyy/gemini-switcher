package model

import (
	"time"
)

const (
	StatusActive          = "ACTIVE"
	StatusReady           = "READY"
	StatusCooldown5H      = "COOLDOWN_5H"
	StatusExhaustedWeekly = "EXHAUSTED_WEEKLY"
)

type QuotaBucket struct {
	RemainingFraction float64   `json:"remaining_fraction"`
	ResetTime         time.Time `json:"reset_time"`
	Window            string    `json:"window"`
}

type AccountProfile struct {
	Email          string      `json:"email"`
	Alias          string      `json:"alias,omitempty"`
	CredentialBlob string      `json:"credential_blob"`
	LastSwitchedAt time.Time   `json:"last_switched_at"`
	Quota5H        QuotaBucket `json:"quota_5h"`
	QuotaWeekly    QuotaBucket `json:"quota_weekly"`
	Status         string      `json:"status"`
	Paused         bool        `json:"paused"`
	Priority       int         `json:"priority"`
}

type Config struct {
	Language             string  `json:"language"`
	AutoSwitchEnabled    bool    `json:"auto_switch_enabled"`
	CheckIntervalSeconds int     `json:"check_interval_seconds"`
	Threshold5H          float64 `json:"threshold_5h"`
	ThresholdWeekly      float64 `json:"threshold_weekly"`
	NotifyOnSwitch       bool    `json:"notify_on_switch"`
	NotifyMode           string  `json:"notify_mode"`
}

func DefaultConfig() *Config {
	return &Config{
		Language:             "auto",
		AutoSwitchEnabled:    true,
		CheckIntervalSeconds: 60,
		Threshold5H:          0.05,
		ThresholdWeekly:      0.05,
		NotifyOnSwitch:       true,
		NotifyMode:           "all",
	}
}

func (a *AccountProfile) IsEligible(now time.Time, thresh5h, threshWk float64) bool {
	if a.Paused {
		return false
	}
	if a.QuotaWeekly.RemainingFraction <= threshWk && !a.QuotaWeekly.ResetTime.IsZero() && now.Before(a.QuotaWeekly.ResetTime) {
		return false
	}
	if a.Quota5H.RemainingFraction <= thresh5h {
		if !a.Quota5H.ResetTime.IsZero() && now.Before(a.Quota5H.ResetTime) {
			return false
		}
	}
	return true
}
