package autostart

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

const (
	runKeyPath = `Software\Microsoft\Windows\CurrentVersion\Run`
	appName    = "GeminiSwitcher"
)

func IsEnabled() (bool, error) {
	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.QUERY_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	defer k.Close()

	val, _, err := k.GetStringValue(appName)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	cleanedVal := strings.Trim(val, "\"")
	cleanedVal = strings.TrimSpace(strings.TrimSuffix(cleanedVal, "--autostart"))
	cleanedVal = strings.Trim(cleanedVal, "\"")
	return strings.EqualFold(filepath.Clean(cleanedVal), filepath.Clean(exe)), nil
}

func SetEnabled(enabled bool) error {
	if enabled {
		exe, err := os.Executable()
		if err != nil {
			return err
		}
		k, _, err := registry.CreateKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
		if err != nil {
			return err
		}
		defer k.Close()
		return k.SetStringValue(appName, `"`+exe+`" --autostart`)
	}

	k, err := registry.OpenKey(registry.CURRENT_USER, runKeyPath, registry.SET_VALUE)
	if err != nil {
		if errors.Is(err, registry.ErrNotExist) {
			return nil
		}
		return err
	}
	defer k.Close()

	err = k.DeleteValue(appName)
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}

func EnsureFlag() error {
	enabled, err := IsEnabled()
	if err != nil || !enabled {
		return err
	}
	return SetEnabled(true)
}
