package autostart

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

func TestAutostart(t *testing.T) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	var origVal string
	if err == nil {
		origVal, _, _ = k.GetStringValue(appName)
		k.Close()
	}
	defer func() {
		if origVal != "" {
			k2, _, e := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
			if e == nil {
				_ = k2.SetStringValue(appName, origVal)
				k2.Close()
			}
		} else {
			_ = SetEnabled(false)
		}
	}()

	if err := SetEnabled(true); err != nil {
		t.Fatalf("failed to enable autostart: %v", err)
	}

	enabled, err := IsEnabled()
	if err != nil {
		t.Fatalf("failed to check autostart: %v", err)
	}
	if !enabled {
		t.Fatalf("expected autostart to be enabled")
	}

	if err := SetEnabled(false); err != nil {
		t.Fatalf("failed to disable autostart: %v", err)
	}

	enabled, err = IsEnabled()
	if err != nil {
		t.Fatalf("failed to check autostart after disable: %v", err)
	}
	if enabled {
		t.Fatalf("expected autostart to be disabled")
	}

	if err := SetEnabled(false); err != nil {
		t.Fatalf("idempotent disable failed: %v", err)
	}

	if err := SetEnabled(true); err != nil {
		t.Fatalf("failed to re-enable: %v", err)
	}
	if err := EnsureFlag(); err != nil {
		t.Fatalf("EnsureFlag failed: %v", err)
	}
	enabled, err = IsEnabled()
	if err != nil || !enabled {
		t.Fatalf("expected enabled after EnsureFlag")
	}
}
