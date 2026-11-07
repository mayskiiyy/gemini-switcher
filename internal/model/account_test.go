package model

import (
	"testing"
	"time"
)

func TestAccountProfileEligibility(t *testing.T) {
	now := time.Now()
	acc := &AccountProfile{
		Email: "test@example.com",
		Quota5H: QuotaBucket{
			RemainingFraction: 0.8,
			ResetTime:         now.Add(2 * time.Hour),
		},
		QuotaWeekly: QuotaBucket{
			RemainingFraction: 0.9,
			ResetTime:         now.Add(48 * time.Hour),
		},
	}
	if !acc.IsEligible(now, 0.05, 0.05) {
		t.Errorf("expected account to be eligible")
	}

	acc.QuotaWeekly.RemainingFraction = 0.02
	if acc.IsEligible(now, 0.05, 0.05) {
		t.Errorf("expected account with weekly <= 0.05 to be ineligible")
	}

	acc.QuotaWeekly.RemainingFraction = 0.9
	acc.Quota5H.RemainingFraction = 0.01
	if acc.IsEligible(now, 0.05, 0.05) {
		t.Errorf("expected account with 5h <= 0.05 before reset to be ineligible")
	}

	acc.Quota5H.ResetTime = now.Add(-1 * time.Minute)
	if !acc.IsEligible(now, 0.05, 0.05) {
		t.Errorf("expected account with expired 5h reset time to be eligible")
	}
}
