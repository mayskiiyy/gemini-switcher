package quota

import (
	"testing"
)

func TestParseQuotaOutput(t *testing.T) {
	sample := `{
		"command": {
			"data": {
				"groups": [
					{
						"name": "Gemini Models",
						"buckets": [
							{
								"id": "gemini-weekly",
								"remaining_fraction": 0.83,
								"reset_time": "2026-10-31T09:29:13Z",
								"window": "weekly"
							},
							{
								"id": "gemini-5h",
								"remaining_fraction": 0.74,
								"reset_time": "2026-10-29T12:03:57Z",
								"window": "5h"
							}
						]
					}
				]
			}
		}
	}`

	res, err := ParseQuotaOutput([]byte(sample))
	if err != nil {
		t.Fatalf("ParseQuotaOutput failed: %v", err)
	}
	if res.Quota5H.RemainingFraction != 0.74 {
		t.Errorf("expected 0.74, got %f", res.Quota5H.RemainingFraction)
	}
	if res.QuotaWeekly.RemainingFraction != 0.83 {
		t.Errorf("expected 0.83, got %f", res.QuotaWeekly.RemainingFraction)
	}
}

func TestParseQuotaOutputInvalid(t *testing.T) {
	_, err := ParseQuotaOutput([]byte("invalid json"))
	if err == nil {
		t.Errorf("expected error for invalid json, got nil")
	}
}

func TestIsolatedQuotaCommand(t *testing.T) {
	tmpDir := t.TempDir()
	cmd := isolatedQuotaCommand(tmpDir)
	if cmd.Dir != tmpDir {
		t.Errorf("expected dir %s, got %s", tmpDir, cmd.Dir)
	}
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags != 0x08000000 {
		t.Errorf("expected HideWindow=true and CreationFlags=0x08000000")
	}
	foundUserprofile := false
	foundHome := false
	foundDisableUpdate := false
	for _, env := range cmd.Env {
		if env == "USERPROFILE="+tmpDir {
			foundUserprofile = true
		}
		if env == "HOME="+tmpDir {
			foundHome = true
		}
		if env == "AGY_CLI_DISABLE_AUTO_UPDATE=true" {
			foundDisableUpdate = true
		}
	}
	if !foundUserprofile || !foundHome || !foundDisableUpdate {
		t.Errorf("expected USERPROFILE, HOME, and AGY_CLI_DISABLE_AUTO_UPDATE in env")
	}
}

func TestLiveFetchActiveQuota(t *testing.T) {
	res, err := FetchActiveQuota()
	if err != nil {
		t.Fatalf("FetchActiveQuota failed: %v", err)
	}
	if res == nil {
		t.Fatalf("expected non-nil QuotaResult")
	}
	t.Logf("Live Quota 5H: %f, Weekly: %f", res.Quota5H.RemainingFraction, res.QuotaWeekly.RemainingFraction)
}
