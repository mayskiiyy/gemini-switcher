package credmgr

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestParseSessionEmail(t *testing.T) {
	sample := `{"token":{"access_token":"ya29.xyz"},"auth_method":"consumer","id_token_email":"vladlen@example.com"}`
	email, err := ParseSessionEmail(sample)
	if err != nil {
		t.Fatalf("ParseSessionEmail failed: %v", err)
	}
	if email != "vladlen@example.com" {
		t.Fatalf("expected vladlen@example.com, got %s", email)
	}
}

func TestLiveReadAntigravityCredential(t *testing.T) {
	cred, err := ReadAntigravityCredential()
	if err != nil {
		t.Logf("ReadAntigravityCredential error: %v", err)
		return
	}
	if len(cred) == 0 {
		t.Errorf("expected non-empty credential")
	}
	email, err := ParseSessionEmail(cred)
	if err != nil {
		t.Errorf("failed to parse email: %v", err)
	}
	if email == "" {
		t.Errorf("expected non-empty email")
	}
}

func TestUpdateAppStorageUsername(t *testing.T) {
	tmpDir := t.TempDir()
	t.Setenv("APPDATA", tmpDir)
	agDir := filepath.Join(tmpDir, "Antigravity")
	if err := os.MkdirAll(agDir, 0700); err != nil {
		t.Fatalf("failed to create dir: %v", err)
	}
	initial := `{"jetski.onboarding.lastLoginUsername": "old@example.com", "other": 123}`
	storagePath := filepath.Join(agDir, "app_storage.json")
	if err := os.WriteFile(storagePath, []byte(initial), 0600); err != nil {
		t.Fatalf("failed to write initial storage: %v", err)
	}

	if err := UpdateAppStorageUsername("new@example.com"); err != nil {
		t.Fatalf("UpdateAppStorageUsername failed: %v", err)
	}

	data, err := os.ReadFile(storagePath)
	if err != nil {
		t.Fatalf("failed to read storage: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("failed to unmarshal storage: %v", err)
	}
	if m["jetski.onboarding.lastLoginUsername"] != "new@example.com" {
		t.Errorf("expected new@example.com, got %v", m["jetski.onboarding.lastLoginUsername"])
	}
}
