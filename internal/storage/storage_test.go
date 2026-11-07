package storage

import (
	"gemini-switcher/internal/model"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestStorageSaveAndLoad(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gemini-storage-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	s, err := NewStorage(tmpDir)
	if err != nil {
		t.Fatalf("NewStorage failed: %v", err)
	}

	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}
	if !cfg.AutoSwitchEnabled || cfg.Threshold5H != 0.05 {
		t.Errorf("unexpected default config: %+v", cfg)
	}

	cfg.Threshold5H = 0.08
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig failed: %v", err)
	}

	cfg2, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("reload config failed: %v", err)
	}
	if cfg2.Threshold5H != 0.08 {
		t.Errorf("expected 0.08, got %f", cfg2.Threshold5H)
	}

	accs := []*model.AccountProfile{
		{
			Email:          "one@example.com",
			CredentialBlob: "blob1",
			LastSwitchedAt: time.Now().UTC().Truncate(time.Second),
		},
	}
	if err := s.SaveAccounts(accs); err != nil {
		t.Fatalf("SaveAccounts failed: %v", err)
	}

	loaded, err := s.LoadAccounts()
	if err != nil {
		t.Fatalf("LoadAccounts failed: %v", err)
	}
	if len(loaded) != 1 || loaded[0].Email != "one@example.com" {
		t.Fatalf("unexpected loaded accounts: %+v", loaded)
	}

	bakPath := filepath.Join(tmpDir, "accounts.json.bak")
	if _, err := os.Stat(bakPath); err != nil {
		t.Errorf("backup file was not created: %v", err)
	}
}
