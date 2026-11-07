package quota

import (
	"context"
	"encoding/json"
	"fmt"
	"gemini-switcher/internal/model"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type QuotaResult struct {
	Quota5H     model.QuotaBucket
	QuotaWeekly model.QuotaBucket
}

type cliBucket struct {
	Id                string  `json:"id"`
	Window            string  `json:"window"`
	RemainingFraction float64 `json:"remaining_fraction"`
	ResetTime         string  `json:"reset_time"`
}

type cliGroup struct {
	Name    string      `json:"name"`
	Buckets []cliBucket `json:"buckets"`
}

type cliEnvelope struct {
	Command struct {
		Data struct {
			Groups []cliGroup `json:"groups"`
		} `json:"data"`
	} `json:"command"`
}

func ParseQuotaOutput(raw []byte) (*QuotaResult, error) {
	var env cliEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	res := &QuotaResult{}
	for _, grp := range env.Command.Data.Groups {
		if grp.Name == "Gemini Models" {
			for _, b := range grp.Buckets {
				rt, _ := time.Parse(time.RFC3339, b.ResetTime)
				bucket := model.QuotaBucket{
					RemainingFraction: b.RemainingFraction,
					ResetTime:         rt,
					Window:            b.Window,
				}
				if b.Id == "gemini-5h" || b.Window == "5h" {
					res.Quota5H = bucket
				} else if b.Id == "gemini-weekly" || b.Window == "weekly" {
					res.QuotaWeekly = bucket
				}
			}
		}
	}
	return res, nil
}

func findAgyPath() string {
	if localAppData := os.Getenv("LOCALAPPDATA"); localAppData != "" {
		p := filepath.Join(localAppData, "agy", "bin", "agy.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		p = filepath.Join(localAppData, "Programs", "Antigravity", "bin", "agy.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if userProfile := os.Getenv("USERPROFILE"); userProfile != "" {
		p := filepath.Join(userProfile, "AppData", "Local", "agy", "bin", "agy.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
		p = filepath.Join(userProfile, ".local", "bin", "agy.exe")
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	if p, err := exec.LookPath("agy.exe"); err == nil {
		return p
	}
	return "agy.exe"
}

func isolatedQuotaCommand(probeDir string) *exec.Cmd {
	return isolatedQuotaCommandContext(context.Background(), probeDir)
}

func isolatedQuotaCommandContext(ctx context.Context, probeDir string) *exec.Cmd {
	cfgDir := filepath.Join(probeDir, ".gemini", "config")
	_ = os.MkdirAll(cfgDir, 0700)
	_ = os.WriteFile(filepath.Join(cfgDir, "mcp_config.json"), []byte(`{"mcpServers":{}}`), 0600)
	_ = os.WriteFile(filepath.Join(cfgDir, "plugins.json"), []byte(`{}`), 0600)
	_ = os.WriteFile(filepath.Join(cfgDir, "settings.json"), []byte(`{"plugins":{},"mcpServers":{},"auto_update":false}`), 0600)

	updaterDir := filepath.Join(probeDir, ".gemini", "antigravity-cli", "updater")
	_ = os.MkdirAll(updaterDir, 0700)
	_ = os.WriteFile(filepath.Join(updaterDir, "update.lock"), []byte(""), 0600)
	_ = os.WriteFile(filepath.Join(updaterDir, "update_status.json"), []byte(`{"success":true,"message":"Already on latest version."}`), 0600)

	cmd := exec.CommandContext(ctx, findAgyPath(), "--output-format", "json", "-p", "/quota")
	cmd.Dir = probeDir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: 0x08000000,
	}

	var cleanPathParts []string
	for _, p := range filepath.SplitList(os.Getenv("PATH")) {
		lp := strings.ToLower(p)
		if strings.Contains(lp, "git") || strings.Contains(lp, "node") || strings.Contains(lp, "npm") || strings.Contains(lp, "pnpm") || strings.Contains(lp, "yarn") {
			continue
		}
		cleanPathParts = append(cleanPathParts, p)
	}
	cleanPath := strings.Join(cleanPathParts, string(filepath.ListSeparator))

	env := make([]string, 0, len(os.Environ())+10)
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if !strings.EqualFold(k, "USERPROFILE") && !strings.EqualFold(k, "HOME") &&
			!strings.EqualFold(k, "ANTIGRAVITY_CONFIG") && !strings.EqualFold(k, "GEMINI_DIR") &&
			!strings.EqualFold(k, "APPDATA") && !strings.EqualFold(k, "PATH") {
			env = append(env, e)
		}
	}
	cmd.Env = append(env,
		"USERPROFILE="+probeDir,
		"HOME="+probeDir,
		"ANTIGRAVITY_CONFIG="+probeDir,
		"GEMINI_DIR="+probeDir,
		"APPDATA="+probeDir,
		"PATH="+cleanPath,
		"AGY_CLI_DISABLE_AUTO_UPDATE=true",
		"AGY_CLI_NONINTERACTIVE_HEADLESS=true",
		"GIT_OPTIONAL_LOCKS=0",
		"GIT_TERMINAL_PROMPT=0",
	)
	return cmd
}

func FetchActiveQuota() (*QuotaResult, error) {
	probeDir := filepath.Join(os.TempDir(), fmt.Sprintf("gemini-switcher-probe-%d", os.Getpid()))
	_ = os.MkdirAll(probeDir, 0700)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	cmd := isolatedQuotaCommandContext(ctx, probeDir)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("agy execution failed: %w", err)
	}
	return ParseQuotaOutput(out)
}
