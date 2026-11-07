package credmgr

import (
	"encoding/json"
	"os"
	"path/filepath"
)

func UpdateAppStorageUsername(username string) error {
	appData := os.Getenv("APPDATA")
	if appData == "" {
		return nil
	}
	storagePath := filepath.Join(appData, "Antigravity", "app_storage.json")
	data, err := os.ReadFile(storagePath)
	if err != nil {
		return err
	}

	var m map[string]interface{}
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}

	m["jetski.onboarding.lastLoginUsername"] = username
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(storagePath, out, 0644)
}
