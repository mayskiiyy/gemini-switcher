package gui

import (
	"encoding/json"
	"fmt"
	"gemini-switcher/internal/autostart"
	"gemini-switcher/internal/credmgr"
	"gemini-switcher/internal/icons"
	"gemini-switcher/internal/model"
	"gemini-switcher/internal/quota"
	"gemini-switcher/internal/storage"
	"gemini-switcher/internal/switcher"
	"gemini-switcher/internal/updater"
	"net"
	"net/http"
)

type Server struct {
	storage  *storage.Storage
	sw       *switcher.Switcher
	listener net.Listener
	server   *http.Server
	port     int
	url      string
}

type statusResponse struct {
	Accounts    []*model.AccountProfile `json:"accounts"`
	ActiveEmail string                  `json:"active_email"`
	Config      *model.Config           `json:"config"`
	Autostart   bool                    `json:"autostart"`
}

func NewServer(st *storage.Storage, sw *switcher.Switcher) *Server {
	return &Server{
		storage: st,
		sw:      sw,
	}
}

func StartServer(st *storage.Storage, sw *switcher.Switcher) (*Server, error) {
	s := NewServer(st, sw)
	if err := s.Start(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *Server) Start() error {
	ln, err := net.Listen("tcp", "127.0.0.1:28795")
	if err != nil {
		ln, err = net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			return err
		}
	}
	s.listener = ln
	s.port = ln.Addr().(*net.TCPAddr).Port
	s.url = fmt.Sprintf("http://127.0.0.1:%d", s.port)
	setGlobalServer(s.url, s.port)

	mux := http.NewServeMux()
	mux.HandleFunc("/favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/x-icon")
		_, _ = w.Write(icons.Green)
	})
	mux.HandleFunc("/", s.handleDashboard)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/switch", s.handleSwitch)
	mux.HandleFunc("/api/capture", s.handleCapture)
	mux.HandleFunc("/api/refresh", s.handleRefresh)
	mux.HandleFunc("/api/account/update", s.handleAccountUpdate)
	mux.HandleFunc("/api/account/delete", s.handleAccountDelete)
	mux.HandleFunc("/api/config", s.handleConfig)
	mux.HandleFunc("/api/check-update", s.handleCheckUpdate)

	s.server = &http.Server{Handler: mux}
	go func() {
		_ = s.server.Serve(ln)
	}()
	return nil
}

func (s *Server) URL() string {
	return s.url
}

func (s *Server) Port() int {
	return s.port
}

func (s *Server) Close() error {
	if s.server != nil {
		return s.server.Close()
	}
	if s.listener != nil {
		return s.listener.Close()
	}
	return nil
}

func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(dashboardHTML))
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	accs, err := s.storage.LoadAccounts()
	if err != nil {
		accs = []*model.AccountProfile{}
	}

	var activeEmail string
	blob, err := credmgr.ReadAntigravityCredential()
	if err == nil && blob != "" {
		activeEmail, _ = credmgr.ParseSessionEmail(blob)
	}

	cfg, err := s.storage.LoadConfig()
	if err != nil {
		cfg = model.DefaultConfig()
	}

	isAuto, _ := autostart.IsEnabled()

	resp := statusResponse{
		Accounts:    accs,
		ActiveEmail: activeEmail,
		Config:      cfg,
		Autostart:   isAuto,
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleSwitch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	accs, err := s.storage.LoadAccounts()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	var target *model.AccountProfile
	for _, a := range accs {
		if a.Email == req.Email {
			target = a
			break
		}
	}
	if target == nil {
		target = &model.AccountProfile{Email: req.Email}
	}

	if err := s.sw.SwitchTo(target); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (s *Server) handleCapture(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	acc, err := s.sw.CaptureCurrentAccount()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":      true,
		"account": acc,
	})
}

func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var activeEmail string
	blob, err := credmgr.ReadAntigravityCredential()
	if err == nil && blob != "" {
		activeEmail, _ = credmgr.ParseSessionEmail(blob)
	}

	qRes, qErr := quota.FetchActiveQuota()
	if qErr != nil {
		http.Error(w, qErr.Error(), http.StatusInternalServerError)
		return
	}

	swapped, rotErr := s.sw.RotateIfNeeded(activeEmail, qRes)
	resp := map[string]interface{}{
		"ok":      true,
		"swapped": swapped != nil,
		"quota":   qRes,
	}
	if rotErr != nil {
		resp["rotation_error"] = rotErr.Error()
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func (s *Server) handleAccountUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email    string `json:"email"`
		Alias    string `json:"alias"`
		Paused   bool   `json:"paused"`
		Priority int    `json:"priority"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.sw.UpdateAccount(req.Email, req.Alias, req.Paused, req.Priority); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (s *Server) handleAccountDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if err := s.sw.DeleteAccount(req.Email); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AutoSwitchEnabled    *bool    `json:"auto_switch_enabled"`
		CheckIntervalSeconds *int     `json:"check_interval_seconds"`
		Threshold5H          *float64 `json:"threshold_5h"`
		ThresholdWeekly      *float64 `json:"threshold_weekly"`
		NotifyOnSwitch       *bool    `json:"notify_on_switch"`
		NotifyMode           *string  `json:"notify_mode"`
		Autostart            *bool    `json:"autostart"`
		Language             *string  `json:"language"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.Autostart != nil {
		_ = autostart.SetEnabled(*req.Autostart)
	}

	cfg, err := s.storage.LoadConfig()
	if err != nil {
		cfg = model.DefaultConfig()
	}

	if req.Language != nil {
		switch *req.Language {
		case "auto", "ru", "en", "de", "pl", "uk":
			cfg.Language = *req.Language
		default:
			http.Error(w, "invalid language", http.StatusBadRequest)
			return
		}
	}
	if req.AutoSwitchEnabled != nil {
		cfg.AutoSwitchEnabled = *req.AutoSwitchEnabled
	}
	if req.CheckIntervalSeconds != nil {
		cfg.CheckIntervalSeconds = *req.CheckIntervalSeconds
	}
	if req.Threshold5H != nil {
		cfg.Threshold5H = *req.Threshold5H
	}
	if req.ThresholdWeekly != nil {
		cfg.ThresholdWeekly = *req.ThresholdWeekly
	}
	if req.NotifyOnSwitch != nil {
		cfg.NotifyOnSwitch = *req.NotifyOnSwitch
	}
	if req.NotifyMode != nil {
		cfg.NotifyMode = *req.NotifyMode
	}

	if err := s.storage.SaveConfig(cfg); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	isAuto, _ := autostart.IsEnabled()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":        true,
		"config":    cfg,
		"autostart": isAuto,
	})
}

func (s *Server) handleCheckUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	info, err := updater.CheckForUpdate(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(info)
}

const dashboardHTML = `<!DOCTYPE html>
<html lang="ru">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Gemini Switcher</title>
<link rel="icon" href="/favicon.ico">
<style>
:root {
  --bg-canvas: #090a0f;
  --bg-surface: #11131a;
  --bg-surface-elevated: #161922;
  --bg-surface-inset: #0c0d12;
  --border-subtle: rgba(255, 255, 255, 0.08);
  --border-strong: rgba(255, 255, 255, 0.16);
  --border-active: rgba(16, 185, 129, 0.55);
  --text-primary: #f8fafc;
  --text-secondary: #94a3b8;
  --text-muted: #64748b;
  --accent-emerald: #10b981;
  --accent-amber: #f59e0b;
  --accent-rose: #ef4444;
  --accent-blue: #3b82f6;
  --font-sans: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, system-ui, sans-serif;
  --font-mono: "Cascadia Code", "SF Mono", "JetBrains Mono", Consolas, ui-monospace, monospace;
}
* { box-sizing: border-box; margin: 0; padding: 0; }
body {
  background: var(--bg-canvas);
  color: var(--text-primary);
  font-family: var(--font-sans);
  font-size: 13px;
  line-height: 1.45;
  user-select: none;
  overflow-x: hidden;
  -webkit-font-smoothing: antialiased;
}
::-webkit-scrollbar { width: 5px; }
::-webkit-scrollbar-track { background: var(--bg-canvas); }
::-webkit-scrollbar-thumb { background: rgba(255, 255, 255, 0.12); border-radius: 3px; }
::-webkit-scrollbar-thumb:hover { background: rgba(255, 255, 255, 0.22); }

.app {
  display: flex;
  flex-direction: column;
  height: 100vh;
  max-width: 520px;
  margin: 0 auto;
  background: var(--bg-canvas);
}

header {
  padding: 12px 16px;
  background: var(--bg-surface);
  border-bottom: 1px solid var(--border-subtle);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.05);
  flex-shrink: 0;
}
.header-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  margin-bottom: 10px;
}
.brand {
  display: flex;
  align-items: center;
  gap: 8px;
}
.brand-icon {
  width: 22px;
  height: 22px;
  border-radius: 5px;
  background: var(--bg-surface-elevated);
  border: 1px solid var(--border-strong);
  display: flex;
  align-items: center;
  justify-content: center;
  color: var(--text-primary);
}
.brand-name {
  font-size: 13px;
  font-weight: 700;
  letter-spacing: -0.2px;
  color: var(--text-primary);
}
.status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 3px 9px;
  border-radius: 9999px;
  background: var(--bg-surface-inset);
  border: 1px solid var(--border-subtle);
  font-size: 11px;
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
  color: var(--text-secondary);
  max-width: 230px;
}
.status-dot {
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--text-muted);
  flex-shrink: 0;
}
.status-dot.active {
  background: var(--accent-emerald);
}
.status-pill-text {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.action-bar {
  display: flex;
  gap: 6px;
}
.btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  padding: 5px 11px;
  border-radius: 5px;
  font-size: 12px;
  font-weight: 600;
  cursor: pointer;
  border: 1px solid var(--border-subtle);
  background: var(--bg-surface-elevated);
  color: var(--text-primary);
  transition: all 0.12s cubic-bezier(0.16, 1, 0.3, 1);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
}
.btn:focus-visible {
  outline: 2px solid var(--accent-blue);
  outline-offset: 2px;
}
.btn:hover:not(:disabled) {
  border-color: var(--border-strong);
  background: rgba(255, 255, 255, 0.08);
}
.btn:active:not(:disabled) {
  transform: translateY(1px);
}
.btn:disabled {
  opacity: 0.45;
  cursor: not-allowed;
}
.btn-primary {
  background: #2563eb;
  border-color: #1d4ed8;
  color: #ffffff;
}
.btn-primary:hover:not(:disabled) {
  background: #1d4ed8;
}
.btn-danger-outline {
  background: transparent;
  border-color: rgba(239, 68, 68, 0.25);
  color: #f87171;
}
.btn-danger-outline:hover:not(:disabled) {
  background: rgba(239, 68, 68, 0.12);
  border-color: rgba(239, 68, 68, 0.45);
}
.kbd-hint {
  font-family: var(--font-mono);
  font-size: 10px;
  opacity: 0.55;
  margin-left: 2px;
}

.content {
  flex: 1;
  overflow-y: auto;
  padding: 12px 16px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.account-card {
  background: var(--bg-surface);
  border: 1px solid var(--border-subtle);
  border-radius: 8px;
  padding: 12px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.03);
  transition: border-color 0.12s ease;
}
.account-card.active-card {
  border-color: var(--border-active);
  background: linear-gradient(180deg, rgba(16, 185, 129, 0.03) 0%, var(--bg-surface) 100%);
}
.account-card.paused-card {
  opacity: 0.6;
}

.card-top {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 8px;
}
.card-identity {
  display: flex;
  flex-direction: column;
  gap: 3px;
  min-width: 0;
  flex: 1;
}
.card-email-line {
  display: flex;
  align-items: center;
  gap: 6px;
}
.card-email {
  font-size: 13px;
  font-weight: 600;
  color: var(--text-primary);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  user-select: text;
}
.alias-chip {
  font-size: 11px;
  color: var(--text-secondary);
  background: var(--bg-surface-inset);
  padding: 1px 6px;
  border-radius: 4px;
  cursor: pointer;
  border: 1px solid var(--border-subtle);
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.alias-chip:hover {
  border-color: var(--border-strong);
  color: var(--text-primary);
}
.alias-input {
  font-size: 11px;
  font-family: var(--font-sans);
  color: var(--text-primary);
  background: var(--bg-surface-inset);
  border: 1px solid var(--accent-blue);
  border-radius: 4px;
  padding: 2px 6px;
  outline: none;
  width: 140px;
}

.card-chips {
  display: flex;
  align-items: center;
  gap: 5px;
  flex-shrink: 0;
}
.chip {
  font-size: 11px;
  font-weight: 600;
  padding: 2px 6px;
  border-radius: 4px;
  display: inline-flex;
  align-items: center;
  gap: 4px;
}
.chip-active {
  background: rgba(16, 185, 129, 0.15);
  color: #34d399;
  border: 1px solid rgba(16, 185, 129, 0.35);
}
.chip-paused {
  background: rgba(245, 158, 11, 0.15);
  color: #fbbf24;
  border: 1px solid rgba(245, 158, 11, 0.35);
}
.chip-priority {
  font-family: var(--font-mono);
  background: var(--bg-surface-inset);
  color: var(--text-muted);
  border: 1px solid var(--border-subtle);
  cursor: pointer;
  padding: 2px 5px;
}
.chip-priority:hover {
  border-color: var(--border-strong);
  color: var(--text-primary);
}

.telemetry-grid {
  display: flex;
  flex-direction: column;
  gap: 8px;
  background: var(--bg-surface-inset);
  border: 1px solid var(--border-subtle);
  border-radius: 6px;
  padding: 8px 10px;
}
.telemetry-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.telemetry-header {
  display: flex;
  justify-content: space-between;
  align-items: baseline;
  font-size: 11px;
}
.telemetry-label {
  color: var(--text-secondary);
  font-weight: 500;
}
.telemetry-values {
  display: flex;
  align-items: baseline;
  gap: 6px;
  font-family: var(--font-mono);
  font-variant-numeric: tabular-nums;
}
.telemetry-pct {
  font-weight: 700;
  font-size: 12px;
  color: var(--text-primary);
}
.telemetry-timer {
  color: var(--text-muted);
  font-size: 11px;
  display: flex;
  align-items: center;
  gap: 3px;
}

.meter-track {
  height: 5px;
  background: rgba(255, 255, 255, 0.07);
  border-radius: 2px;
  overflow: hidden;
  position: relative;
}
.meter-ticks {
  position: absolute;
  inset: 0;
  display: flex;
  justify-content: space-between;
  padding: 0 25%;
  pointer-events: none;
}
.meter-tick {
  width: 1px;
  height: 100%;
  background: rgba(0, 0, 0, 0.4);
}
.meter-fill {
  height: 100%;
  border-radius: 2px;
  transition: width 0.25s ease-out, background-color 0.25s ease-out;
}

.card-footer {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding-top: 2px;
}
.card-footer-left {
  display: flex;
  gap: 6px;
}

.modal-backdrop {
  position: fixed;
  inset: 0;
  background: rgba(0, 0, 0, 0.75);
  backdrop-filter: blur(4px);
  display: flex;
  align-items: center;
  justify-content: center;
  padding: 16px;
  z-index: 100;
}
.modal-backdrop.hidden {
  display: none;
}
.modal-dialog {
  background: var(--bg-surface);
  border: 1px solid var(--border-strong);
  border-radius: 10px;
  width: 100%;
  max-width: 440px;
  box-shadow: 0 20px 48px rgba(0, 0, 0, 0.6);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}
.modal-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  padding: 12px 16px;
  border-bottom: 1px solid var(--border-subtle);
  background: var(--bg-surface-elevated);
}
.modal-title {
  font-size: 13px;
  font-weight: 700;
  color: var(--text-primary);
}
.modal-close {
  background: none;
  border: none;
  color: var(--text-muted);
  cursor: pointer;
  padding: 4px;
  border-radius: 4px;
  display: flex;
  align-items: center;
}
.modal-close:hover {
  color: var(--text-primary);
}
.modal-close:focus-visible {
  outline: 2px solid var(--accent-blue);
}
.modal-content {
  padding: 14px 16px;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.config-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.config-label-wrap {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.config-title {
  font-size: 12px;
  font-weight: 600;
  color: var(--text-primary);
}
.config-subtitle {
  font-size: 11px;
  color: var(--text-secondary);
}

.toggle {
  position: relative;
  display: inline-block;
  width: 36px;
  height: 20px;
  flex-shrink: 0;
}
.toggle input {
  opacity: 0;
  width: 0;
  height: 0;
}
.toggle-slider {
  position: absolute;
  cursor: pointer;
  inset: 0;
  background-color: var(--bg-surface-inset);
  border: 1px solid var(--border-strong);
  transition: 0.15s ease;
  border-radius: 20px;
}
.toggle-slider:before {
  position: absolute;
  content: "";
  height: 14px;
  width: 14px;
  left: 2px;
  bottom: 2px;
  background-color: var(--text-muted);
  transition: 0.15s ease;
  border-radius: 50%;
}
.toggle input:checked + .toggle-slider {
  background-color: var(--accent-emerald);
  border-color: #059669;
}
.toggle input:checked + .toggle-slider:before {
  transform: translateX(16px);
  background-color: #ffffff;
}

.form-select {
  background: var(--bg-surface-inset);
  color: var(--text-primary);
  border: 1px solid var(--border-subtle);
  border-radius: 5px;
  padding: 4px 8px;
  font-size: 12px;
  font-family: var(--font-sans);
  outline: none;
  cursor: pointer;
}
.form-select:focus-visible {
  border-color: var(--accent-blue);
  outline: 2px solid var(--accent-blue);
}

.modal-footer {
  padding: 10px 16px;
  border-top: 1px solid var(--border-subtle);
  display: flex;
  justify-content: flex-end;
  gap: 6px;
  background: var(--bg-surface-elevated);
}

.toast {
  position: fixed;
  bottom: 14px;
  left: 50%;
  transform: translateX(-50%) translateY(70px);
  background: var(--bg-surface-elevated);
  color: var(--text-primary);
  border: 1px solid var(--border-strong);
  padding: 6px 14px;
  border-radius: 6px;
  font-size: 12px;
  font-weight: 500;
  box-shadow: 0 10px 28px rgba(0, 0, 0, 0.5);
  transition: transform 0.2s cubic-bezier(0.16, 1, 0.3, 1), opacity 0.2s ease;
  opacity: 0;
  pointer-events: none;
  z-index: 200;
}
.toast.active {
  transform: translateX(-50%) translateY(0);
  opacity: 1;
}
.toast.toast-error {
  border-color: rgba(239, 68, 68, 0.45);
  color: #f87171;
}
.toast.toast-success {
  border-color: rgba(16, 185, 129, 0.45);
  color: #34d399;
}

.empty-view {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  padding: 44px 20px;
  text-align: center;
  color: var(--text-muted);
  border: 1px dashed var(--border-subtle);
  border-radius: 8px;
  margin-top: 16px;
  gap: 6px;
}
.empty-view-text {
  font-size: 12px;
  color: var(--text-secondary);
}

/* Quiet control panel: one accent, flat account rows, and readable quota data. */
:root { --bg-canvas:#101714; --bg-surface:#18211d; --bg-surface-elevated:#25312b; --bg-surface-inset:#121a16; --border-subtle:#34443a; --border-strong:#53685a; --border-active:#75e0b4; --text-primary:#f2f1df; --text-secondary:#b7c4b6; --text-muted:#91a294; --accent-emerald:#75e0b4; --accent-blue:#75e0b4; }
html, body { overflow-x:clip; }
body { font-size:14px; user-select:text; }
.app { max-width:680px; min-height:100vh; height:auto; }
header { padding:28px 28px 22px; background:transparent; box-shadow:none; }
.header-row { margin-bottom:25px; gap:14px; align-items:center; }
.brand { gap:12px; min-width:0; }
.brand-icon { width:38px; height:38px; border:0; background:transparent; }
.brand-icon img { display:block; border-radius:9px; }
.brand-name { font-size:18px; letter-spacing:-.4px; }
.status-pill { border:0; background:transparent; font-family:var(--font-sans); font-size:12px; padding:0; max-width:45%; }
.status-dot { width:7px; height:7px; }
.action-bar { gap:8px; flex-wrap:wrap; }
.btn { min-height:36px; padding:8px 12px; border-radius:5px; font-size:12px; box-shadow:none; transition:background-color .15s, border-color .15s; }
.btn-primary { background:var(--accent-emerald); border-color:var(--accent-emerald); color:#102119; }
.btn-primary:hover:not(:disabled) { background:#a0edcb; border-color:#a0edcb; }
.btn:active:not(:disabled) { transform:none; }
.section-heading { display:flex; align-items:baseline; justify-content:space-between; gap:12px; padding:24px 28px 5px; }
.section-heading h1 { font-size:22px; font-weight:600; letter-spacing:-.4px; }
.section-heading span { color:var(--text-muted); font-size:12px; }
.content { overflow:visible; padding:8px 28px 28px; gap:0; }
.account-card { background:transparent; border:0; border-bottom:1px solid var(--border-subtle); border-radius:0; padding:20px 0; gap:15px; box-shadow:none; }
.account-card.active-card { background:transparent; border-color:var(--border-subtle); }
.account-card.paused-card { opacity:.7; }
.card-email { font-size:14px; }
.alias-chip { background:transparent; border:0; padding:3px 0; color:var(--text-muted); }
.chip { padding:4px 7px; border-radius:3px; }
.chip-active { background:transparent; color:var(--accent-emerald); border:0; }
.chip-priority { font-family:var(--font-sans); color:var(--text-secondary); }
.telemetry-grid { background:transparent; border:0; border-radius:0; padding:0; display:grid; grid-template-columns:repeat(2,minmax(0,1fr)); gap:18px; }
.telemetry-header { gap:8px; flex-wrap:wrap; font-size:12px; }
.telemetry-values { font-family:var(--font-sans); }
.telemetry-pct { font-size:14px; }
.telemetry-timer { font-size:11px; color:var(--text-muted); }
.meter-track { height:4px; background:var(--border-subtle); }
.meter-ticks { display:none; }
.meter-fill { transition:background-color .15s; }
.card-footer { padding-top:0; }
.card-footer .btn { min-height:32px; background:transparent; }
.card-footer .btn:hover:not(:disabled) { background:var(--bg-surface-elevated); }
.empty-view { border-radius:4px; }
button:focus-visible, input:focus-visible, select:focus-visible { outline:2px solid var(--accent-emerald); outline-offset:2px; }
.toggle input:focus-visible + .toggle-slider { outline:2px solid var(--accent-emerald); outline-offset:2px; }
@media (max-width:420px) { header { padding:20px 18px; } .section-heading { padding:20px 18px 5px; } .content { padding:8px 18px 24px; } .header-row { align-items:flex-start; } .status-pill { max-width:42%; } .telemetry-grid { grid-template-columns:1fr; gap:12px; } }
@media (prefers-reduced-motion:reduce) { *, *::before, *::after { transition-duration:0s !important; animation-duration:0s !important; } }

/* Compact desktop layout */
:root { --bg-canvas:#111416; --bg-surface:#1b2023; --bg-surface-elevated:#293135; --bg-surface-inset:#151a1d; --border-subtle:#353e41; --border-strong:#526065; --text-primary:#f1f3f0; --text-secondary:#c0cbca; --text-muted:#a1b0ad; --accent-emerald:#76d7b3; --accent-blue:#76d7b3; }
body { font-size:14px; }
.app { width:100%; max-width:820px; min-height:100vh; padding:0 32px; }
header { padding:24px 0 20px; }
.header-row { margin-bottom:20px; }
.brand-name { font-size:19px; }
.status-pill { max-width:48%; font-size:12px; background:var(--bg-surface); border:1px solid var(--border-subtle); padding:8px 12px; border-radius:6px; }
.action-bar { gap:8px; }
.btn { min-height:38px; font-size:13px; padding:8px 14px; }
.section-heading { padding:26px 0 16px; }
.section-heading h1 { font-size:21px; }
.section-heading span { font-size:13px; }
.content { padding:0 0 32px; gap:10px; }
.account-card { padding:18px 20px; gap:15px; border:1px solid var(--border-subtle); border-radius:8px; background:var(--bg-surface); }
.account-card.active-card { background:var(--bg-surface); border-color:#5d9e84; box-shadow:inset 3px 0 0 var(--accent-emerald); }
.card-email { font-size:15px; }
.card-identity { gap:4px; }
.alias-chip { font-size:12px; }
.telemetry-grid { gap:24px; }
.telemetry-item { gap:7px; }
.telemetry-header { font-size:12px; }
.telemetry-pct { font-size:15px; }
.telemetry-timer { font-size:12px; }
.meter-track { height:6px; border-radius:3px; }
.meter-fill { border-radius:3px; }
.card-footer { border-top:1px solid var(--border-subtle); padding-top:12px; }
.card-footer .btn { min-height:34px; }
.chip-active { color:var(--accent-emerald); background:#233c32; padding:5px 8px; }
.chip-priority { background:var(--bg-surface-inset); }
@media (max-width:560px) { .app { padding:0 18px; } header { padding:20px 0 16px; } .header-row { align-items:center; } .status-pill { max-width:44%; } .section-heading { padding:20px 0 14px; } .content { padding:0 0 24px; } .account-card { padding:16px; } .telemetry-grid { grid-template-columns:1fr; gap:13px; } }

/* Wide workspace */
:root { --bg-canvas:#101419; --bg-surface:#1b222a; --bg-surface-elevated:#2a3440; --bg-surface-inset:#141a20; --border-subtle:#34414a; --border-strong:#5c707c; --text-primary:#eef4f3; --text-secondary:#bac8cb; --text-muted:#9baeb3; --accent-emerald:#82d6bb; --accent-blue:#82d6bb; }
.app { max-width:1240px; width:100%; padding:0 36px; }
header { padding:24px 0 20px; }
.header-row { margin-bottom:20px; }
.header-tools { display:flex; gap:10px; align-items:center; min-width:0; }
.language-label { color:var(--text-secondary); font-size:12px; }
.language-select { background:var(--bg-surface); color:var(--text-primary); border:1px solid var(--border-subtle); border-radius:6px; padding:8px; font:inherit; }
.status-pill { max-width:260px; }
.section-heading { padding:25px 0 15px; }
.content { display:grid; grid-template-columns:repeat(3,minmax(0,1fr)); align-content:start; gap:14px; }
.account-card { min-width:0; padding:20px; gap:17px; border:1px solid var(--border-subtle); border-radius:10px; background:var(--bg-surface); transition:border-color .18s ease, background-color .18s ease, box-shadow .18s ease; }
.account-card:hover { border-color:var(--border-strong); background:var(--bg-surface-elevated); }
.account-card.active-card { border-color:#568b7d; background:var(--bg-surface); box-shadow:inset 3px 0 0 var(--accent-emerald); }
.account-card.active-card:hover { background:var(--bg-surface-elevated); }
.card-email { font-size:15px; }
.telemetry-grid { grid-template-columns:1fr; gap:12px; }
.telemetry-header { font-size:13px; }
.card-footer { margin-top:auto; gap:8px; }
.card-footer-left { min-width:0; flex-wrap:wrap; }
.card-footer > .btn-danger-outline { flex-shrink:0; }
.card-email { white-space:normal; overflow-wrap:anywhere; }
.btn { transition:background-color .16s ease,border-color .16s ease,color .16s ease; }
.modal-dialog { animation:modal-in .18s ease-out; }
@keyframes modal-in { from { opacity:0; transform:translateY(8px); } to { opacity:1; transform:translateY(0); } }
.empty-view { grid-column:1/-1; }
@media (max-width:850px) { .content { grid-template-columns:repeat(2,minmax(0,1fr)); } }
@media (max-width:700px) { .app { padding:0 20px; } .content { grid-template-columns:1fr; } .header-row { flex-wrap:wrap; } .header-tools { max-width:100%; } }
@media (max-width:430px) { .status-pill { max-width:160px; } .language-label { display:none; } }
@media (prefers-reduced-motion:reduce) { *, *::before, *::after { animation:none !important; transition:none !important; } }
</style>
</head>
<body>
<div class="app">
  <header>
    <div class="header-row">
      <div class="brand">
        <div class="brand-icon">
          <img src="/favicon.ico" alt="" width="36" height="36">
        </div>
        <div class="brand-name">Gemini Switcher</div>
      </div>
      <div class="header-tools"><label class="language-label" for="language-select" data-i18n="Язык">Язык</label><select class="language-select" id="language-select" aria-label="Language" onchange="setLanguage(this.value)"><option value="auto">System language</option><option value="ru">Русский</option><option value="en">English</option><option value="de">Deutsch</option><option value="pl">Polski</option><option value="uk">Українська</option></select><div class="status-pill" id="header-status-pill">
        <span class="status-pill-text" id="header-status-text">Загрузка...</span>
      </div></div>
    </div>
    <div class="action-bar">
      <button class="btn btn-primary" id="btn-capture" onclick="onCaptureClick()">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="12" y1="5" x2="12" y2="19"></line><line x1="5" y1="12" x2="19" y2="12"></line></svg>
        <span>Захватить сессию</span>
      </button>
      <button class="btn" id="btn-refresh" onclick="onRefreshClick()">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.2"><path d="M21.5 2v6h-6M2.5 22v-6h6M2 11.5a10 10 0 0 1 18.8-4.3M22 12.5a10 10 0 0 1-18.8 4.2"/></svg>
        <span>Обновить</span>
        <span class="kbd-hint">[R]</span>
      </button>
      <button class="btn" id="btn-settings" onclick="openSettingsModal()">
        <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="3"></circle><path d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 0 1 0 2.83 2 2 0 0 1-2.83 0l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-2 2 2 2 0 0 1-2-2v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 0 1-2.83 0 2 2 0 0 1 0-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1-2-2 2 2 0 0 1 2-2h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 2-2 2 2 0 0 1 2 2v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 0 1 2.83 0 2 2 0 0 1 0 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 2 2 2 2 0 0 1-2 2h-.09a1.65 1.65 0 0 0-1.51 1z"></path></svg>
        <span>Настройки</span>
      </button>
    </div>
  </header>

  <div class="section-heading"><h1>Аккаунты</h1><span>Лимиты и переключение</span></div>
  <div class="content" id="accounts-container">
    <div class="empty-view">
      <div class="empty-view-text">Загрузка аккаунтов...</div>
    </div>
  </div>
</div>

<div class="modal-backdrop hidden" id="settings-modal" onclick="onBackdropClick(event)">
  <div class="modal-dialog">
    <div class="modal-header">
      <div class="modal-title">Параметры ротации</div>
      <button class="modal-close" onclick="closeSettingsModal()">
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><line x1="18" y1="6" x2="6" y2="18"></line><line x1="6" y1="6" x2="18" y2="18"></line></svg>
      </button>
    </div>
    <div class="modal-content">
      <div class="config-row">
        <div class="config-label-wrap">
          <div class="config-title">Автозапуск с Windows</div>
          <div class="config-subtitle">Запуск в системном трее при загрузке системы</div>
        </div>
        <label class="toggle">
          <input type="checkbox" id="cfg-autostart">
          <span class="toggle-slider"></span>
        </label>
      </div>

      <div class="config-row">
        <div class="config-label-wrap">
          <div class="config-title">Автопереключение</div>
          <div class="config-subtitle">Ротация сессии при исчерпании квоты</div>
        </div>
        <label class="toggle">
          <input type="checkbox" id="cfg-autoswitch">
          <span class="toggle-slider"></span>
        </label>
      </div>

      <div class="config-row">
        <div class="config-label-wrap">
          <div class="config-title">Порог 5ч окна</div>
          <div class="config-subtitle">Ротация при остатке скользящего лимита</div>
        </div>
        <select class="form-select" id="cfg-thresh-5h">
          <option value="0.01">1% (до упора)</option>
          <option value="0.05">5% (рекомендуемый)</option>
          <option value="0.10">10% (с запасом)</option>
        </select>
      </div>

      <div class="config-row">
        <div class="config-label-wrap">
          <div class="config-title">Порог недельного окна</div>
          <div class="config-subtitle">Предохранитель от блокировки на неделю</div>
        </div>
        <select class="form-select" id="cfg-thresh-wk">
          <option value="0.01">1% (до упора)</option>
          <option value="0.05">5% (рекомендуемый)</option>
          <option value="0.10">10% (с запасом)</option>
        </select>
      </div>

      <div class="config-row">
        <div class="config-label-wrap">
          <div class="config-title">Уведомления Windows</div>
          <div class="config-subtitle">Toast-оповещения в центре уведомлений</div>
        </div>
        <select class="form-select" id="cfg-notify-mode">
          <option value="all">Все события</option>
          <option value="switch_only">Только смена аккаунта</option>
          <option value="none">Выключены</option>
        </select>
      </div>
    </div>
    <div class="modal-footer">
      <button class="btn" onclick="closeSettingsModal()">Отмена</button>
      <button class="btn btn-primary" onclick="saveSettings()">Применить</button>
    </div>
  </div>
</div>

<div class="toast" id="toast"></div>

<script>

var translations = {"Язык":{"en":"Language","de":"Sprache","pl":"Język","uk":"Мова"},"Захватить сессию":{"en":"Capture session","de":"Sitzung erfassen","pl":"Zapisz sesję","uk":"Зберегти сесію"},"Обновить":{"en":"Refresh","de":"Aktualisieren","pl":"Odśwież","uk":"Оновити"},"Настройки":{"en":"Settings","de":"Einstellungen","pl":"Ustawienia","uk":"Налаштування"},"Аккаунты":{"en":"Accounts","de":"Konten","pl":"Konta","uk":"Облікові записи"},"Лимиты и переключение":{"en":"Limits and switching","de":"Limits und Wechsel","pl":"Limity i przełączanie","uk":"Ліміти та перемикання"},"Загрузка аккаунтов...":{"en":"Loading accounts...","de":"Konten werden geladen...","pl":"Ładowanie kont...","uk":"Завантаження облікових записів..."},"Параметры ротации":{"en":"Rotation settings","de":"Rotationseinstellungen","pl":"Ustawienia rotacji","uk":"Налаштування ротації"},"Автозапуск с Windows":{"en":"Start with Windows","de":"Mit Windows starten","pl":"Uruchamiaj z Windows","uk":"Запускати з Windows"},"Запуск в системном трее при загрузке системы":{"en":"Start in the system tray when Windows starts","de":"Beim Windows-Start im Infobereich starten","pl":"Uruchamiaj w zasobniku po starcie systemu","uk":"Запускати в системному треї разом із Windows"},"Автопереключение":{"en":"Automatic switching","de":"Automatischer Wechsel","pl":"Automatyczne przełączanie","uk":"Автоматичне перемикання"},"Ротация сессии при исчерпании квоты":{"en":"Switch sessions when quota runs out","de":"Sitzung bei erschöpftem Kontingent wechseln","pl":"Przełączaj sesję po wyczerpaniu limitu","uk":"Перемикати сесію після вичерпання квоти"},"Порог 5ч окна":{"en":"5-hour threshold","de":"Schwelle für 5 Stunden","pl":"Próg 5 godzin","uk":"Поріг 5 годин"},"Ротация при остатке скользящего лимита":{"en":"Switch at remaining quota","de":"Wechsel beim verbleibenden Kontingent","pl":"Przełącz przy pozostałym limicie","uk":"Перемикати за залишком квоти"},"Порог недельного окна":{"en":"Weekly threshold","de":"Wöchentliche Schwelle","pl":"Próg tygodniowy","uk":"Тижневий поріг"},"Предохранитель от блокировки на неделю":{"en":"Protect against exhausting the weekly quota","de":"Schutz vor Erschöpfung des Wochenlimits","pl":"Ochrona przed wyczerpaniem limitu tygodniowego","uk":"Захист від вичерпання тижневого ліміту"},"1% (до упора)":{"en":"1% (minimum)","de":"1% (Minimum)","pl":"1% (minimum)","uk":"1% (мінімум)"},"5% (рекомендуемый)":{"en":"5% (recommended)","de":"5% (empfohlen)","pl":"5% (zalecane)","uk":"5% (рекомендовано)"},"10% (с запасом)":{"en":"10% (with buffer)","de":"10% (mit Reserve)","pl":"10% (z zapasem)","uk":"10% (із запасом)"},"Уведомления Windows":{"en":"Windows notifications","de":"Windows-Benachrichtigungen","pl":"Powiadomienia Windows","uk":"Сповіщення Windows"},"Toast-оповещения в центре уведомлений":{"en":"Notifications in the system tray","de":"Benachrichtigungen im Infobereich","pl":"Powiadomienia w zasobniku systemowym","uk":"Сповіщення в системному треї"},"Все события":{"en":"All events","de":"Alle Ereignisse","pl":"Wszystkie zdarzenia","uk":"Усі події"},"Только смена аккаунта":{"en":"Account switches only","de":"Nur Kontowechsel","pl":"Tylko zmiana konta","uk":"Лише зміна облікового запису"},"Выключены":{"en":"Off","de":"Aus","pl":"Wyłączone","uk":"Вимкнено"},"Отмена":{"en":"Cancel","de":"Abbrechen","pl":"Anuluj","uk":"Скасувати"},"Применить":{"en":"Apply","de":"Übernehmen","pl":"Zastosuj","uk":"Застосувати"},"Нет активной сессии":{"en":"No active session","de":"Keine aktive Sitzung","pl":"Brak aktywnej sesji","uk":"Немає активної сесії"},"Пул пуст. Нажмите «Захватить сессию», чтобы сохранить текущий аккаунт.":{"en":"No accounts yet. Capture the current session to add one.","de":"Noch keine Konten. Erfassen Sie die aktuelle Sitzung.","pl":"Brak kont. Zapisz bieżącą sesję, aby dodać konto.","uk":"Облікових записів ще немає. Збережіть поточну сесію."},"+ имя":{"en":"+ name","de":"+ Name","pl":"+ nazwa","uk":"+ ім’я"},"● активен":{"en":"● active","de":"● aktiv","pl":"● aktywne","uk":"● активний"},"пауза":{"en":"paused","de":"pausiert","pl":"wstrzymane","uk":"призупинено"},"5ч лимит":{"en":"5-hour limit","de":"5-Stunden-Limit","pl":"Limit 5 godzin","uk":"Ліміт 5 годин"},"Недельный лимит":{"en":"Weekly limit","de":"Wochenlimit","pl":"Limit tygodniowy","uk":"Тижневий ліміт"},"Переключить":{"en":"Switch","de":"Wechseln","pl":"Przełącz","uk":"Перемкнути"},"Включить":{"en":"Resume","de":"Fortsetzen","pl":"Wznów","uk":"Відновити"},"Пауза":{"en":"Pause","de":"Pausieren","pl":"Wstrzymaj","uk":"Пауза"},"Удалить из пула":{"en":"Remove account","de":"Konto entfernen","pl":"Usuń konto","uk":"Видалити запис"},"Удалить аккаунт из пула":{"en":"Remove account","de":"Konto entfernen","pl":"Usuń konto","uk":"Видалити обліковий запис"},"доступен":{"en":"available","de":"verfügbar","pl":"dostępny","uk":"доступний"},"Захват сессии из Antigravity...":{"en":"Capturing Antigravity session...","de":"Antigravity-Sitzung wird erfasst...","pl":"Zapisywanie sesji Antigravity...","uk":"Збереження сесії Antigravity..."},"Сессия сохранена в пул":{"en":"Session saved","de":"Sitzung gespeichert","pl":"Sesja zapisana","uk":"Сесію збережено"},"Ошибка захвата":{"en":"Capture failed","de":"Erfassung fehlgeschlagen","pl":"Nie udało się zapisać sesji","uk":"Не вдалося зберегти сесію"},"Запрос актуальных квот...":{"en":"Refreshing quotas...","de":"Kontingente werden aktualisiert...","pl":"Odświeżanie limitów...","uk":"Оновлення квот..."},"Лимит исчерпан: выполнен переход":{"en":"Limit reached: account switched","de":"Limit erreicht: Konto gewechselt","pl":"Limit wyczerpany: zmieniono konto","uk":"Ліміт вичерпано: обліковий запис змінено"},"Квоты обновлены":{"en":"Quotas updated","de":"Kontingente aktualisiert","pl":"Limity odświeżone","uk":"Квоти оновлено"},"Ошибка обновления":{"en":"Refresh failed","de":"Aktualisierung fehlgeschlagen","pl":"Nie udało się odświeżyć","uk":"Не вдалося оновити"},"Переключение сессии...":{"en":"Switching session...","de":"Sitzung wird gewechselt...","pl":"Przełączanie sesji...","uk":"Перемикання сесії..."},"Ошибка переключения":{"en":"Switch failed","de":"Wechsel fehlgeschlagen","pl":"Nie udało się przełączyć","uk":"Не вдалося перемкнути"},"Аккаунт удален":{"en":"Account removed","de":"Konto entfernt","pl":"Konto usunięte","uk":"Обліковий запис видалено"},"Настройки сохранены":{"en":"Settings saved","de":"Einstellungen gespeichert","pl":"Ustawienia zapisane","uk":"Налаштування збережено"},"Ошибка сохранения":{"en":"Save failed","de":"Speichern fehlgeschlagen","pl":"Nie udało się zapisać","uk":"Не вдалося зберегти"},"Удалить аккаунт ":{"en":"Remove account ","de":"Konto entfernen: ","pl":"Usunąć konto ","uk":"Видалити обліковий запис "}," из пула?":{"en":"?","de":"?","pl":"?","uk":"?"},"Активен: ":{"en":"Active: ","de":"Aktiv: ","pl":"Aktywne: ","uk":"Активний: "},"Ошибка сети: ":{"en":"Network error: ","de":"Netzwerkfehler: ","pl":"Błąd sieci: ","uk":"Помилка мережі: "},"Ошибка: ":{"en":"Error: ","de":"Fehler: ","pl":"Błąd: ","uk":"Помилка: "},"Имя...":{"en":"Name...","de":"Name...","pl":"Nazwa...","uk":"Ім’я..."}};
function resolveLanguage(value) {
  if (!value || value === 'auto') value = (navigator.language || 'en').toLowerCase().split(/[-_]/)[0];
  return ['ru', 'en', 'de', 'pl', 'uk'].indexOf(value) >= 0 ? value : 'en';
}
var language = resolveLanguage('auto');
function tr(value) { var row = translations[value]; return row && row[language] || value; }
function translate(root) {
  var walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  var node;
  while ((node = walker.nextNode())) {
    if (node.parentElement && /^(SCRIPT|STYLE)$/.test(node.parentElement.tagName)) continue;
    var source = node._sourceText || node.nodeValue;
    var trimmed = source.trim();
    if (translations[trimmed]) { node._sourceText = source; node.nodeValue = source.replace(trimmed, tr(trimmed)); }
  }
  root.querySelectorAll('[title], [aria-label], [placeholder]').forEach(function(el) {
    ['title','aria-label','placeholder'].forEach(function(attr) {
      var key = 'source' + attr.replace('-', '');
      var original = el.dataset[key] || el.getAttribute(attr);
      if (original && translations[original]) { el.dataset[key] = original; el.setAttribute(attr, tr(original)); }
    });
  });
}
function setLanguage(value) {
  fetch('/api/config', { method:'POST', headers:{'Content-Type':'application/json'}, body:JSON.stringify({language:value}) })
    .then(function(res) { if (!res.ok) throw new Error('save failed'); return res.json(); })
    .then(function(data) { language = resolveLanguage(data.config.language); document.documentElement.lang = language; translate(document.body); renderUI(); })
    .catch(function(err) { showToast('Ошибка сохранения: ' + err.message, true); });
}

var state = {
  accounts: [],
  active_email: '',
  config: null,
  autostart: false
};
var toastTimer = null;
var isEditingAlias = false;

function showToast(text, isError) {
  var t = document.getElementById('toast');
  t.textContent = tr(text);
  t.className = 'toast active ' + (isError ? 'toast-error' : 'toast-success');
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(function() {
    t.className = 'toast';
  }, 3000);
}

function formatCountdown(resetTimeStr) {
  if (!resetTimeStr) return '—';
  var target = new Date(resetTimeStr).getTime();
  if (isNaN(target) || target <= 0) return '—';
  var diff = target - Date.now();
  if (diff <= 0) return tr('доступен');
  var totalSec = Math.floor(diff / 1000);
  var hours = Math.floor(totalSec / 3600);
  var minutes = Math.floor((totalSec % 3600) / 60);
  if (hours > 24) {
    var days = Math.floor(hours / 24);
    return '~' + days + (language === 'ru' || language === 'uk' ? 'д' : 'd');
  }
  if (hours > 0) {
    return '~' + hours + (language === 'ru' ? 'ч ' : language === 'uk' ? 'г ' : 'h ') + minutes + (language === 'ru' || language === 'uk' ? 'м' : 'm');
  }
  return '~' + minutes + (language === 'ru' || language === 'uk' ? 'м' : 'm');
}

function getProgressColor(fraction) {
  if (fraction <= 0.05) return 'var(--accent-rose)';
  if (fraction <= 0.20) return 'var(--accent-amber)';
  return 'var(--accent-emerald)';
}

function escapeHtml(str) {
  if (!str) return '';
  return String(str)
    .replace(/&/g, '&amp;')
    .replace(/</g, '&lt;')
    .replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;');
}

function renderUI() {
  var pillText = document.getElementById('header-status-text');
  if (state.active_email) {
    var activeAcc = state.accounts.find(function(a) { return a.email === state.active_email; });
    var label = state.active_email;
    if (activeAcc && activeAcc.alias) {
      label = activeAcc.alias + ' (' + state.active_email + ')';
    }
    pillText.textContent = label;
  } else {
    pillText.textContent = tr('Нет активной сессии');
  }

  var container = document.getElementById('accounts-container');
  if (!state.accounts || state.accounts.length === 0) {
    container.innerHTML = '<div class="empty-view"><div class="empty-view-text">Пул пуст. Нажмите «Захватить сессию», чтобы сохранить текущий аккаунт.</div></div>';
    translate(container);
    return;
  }

  if (isEditingAlias) return;

  var html = '';
  state.accounts.forEach(function(acc) {
    var isActive = acc.email === state.active_email;
    var isPaused = !!acc.paused;
    var rem5h = (acc.quota_5h && typeof acc.quota_5h.remaining_fraction === 'number') ? acc.quota_5h.remaining_fraction : 1.0;
    var remWk = (acc.quota_weekly && typeof acc.quota_weekly.remaining_fraction === 'number') ? acc.quota_weekly.remaining_fraction : 1.0;
    var pct5h = Math.round(rem5h * 100);
    var pctWk = Math.round(remWk * 100);
    var color5h = getProgressColor(rem5h);
    var colorWk = getProgressColor(remWk);
    var cd5h = formatCountdown(acc.quota_5h ? acc.quota_5h.reset_time : null);
    var cdWk = formatCountdown(acc.quota_weekly ? acc.quota_weekly.reset_time : null);

    var cardClass = 'account-card';
    if (isActive) cardClass += ' active-card';
    if (isPaused) cardClass += ' paused-card';

    html += '<div class="' + cardClass + '">';
    html += '  <div class="card-top">';
    html += '    <div class="card-identity">';
    html += '      <div class="card-email-line">';
    html += '        <div class="card-email" title="' + escapeHtml(acc.email) + '">' + escapeHtml(acc.email) + '</div>';
    html += '      </div>';
    html += '      <div id="alias-wrap-' + escapeHtml(acc.email) + '">';
    if (acc.alias) {
      html += '        <span class="alias-chip" onclick="startEditAlias(\'' + escapeHtml(acc.email) + '\', \'' + escapeHtml(acc.alias) + '\')">' + escapeHtml(acc.alias) + '</span>';
    } else {
      html += '        <span class="alias-chip" onclick="startEditAlias(\'' + escapeHtml(acc.email) + '\', \'\')">+ имя</span>';
    }
    html += '      </div>';
    html += '    </div>';
    html += '    <div class="card-chips">';
    if (isActive) {
      html += '      <span class="chip chip-active">● активен</span>';
    }
    if (isPaused) {
      html += '      <span class="chip chip-paused">пауза</span>';
    }
    html += '    </div>';
    html += '  </div>';

    html += '  <div class="telemetry-grid">';
    html += '    <div class="telemetry-item">';
    html += '      <div class="telemetry-header">';
    html += '        <span class="telemetry-label">5ч лимит</span>';
    html += '        <div class="telemetry-values">';
    html += '          <span class="telemetry-pct" style="color:' + color5h + '">' + pct5h + '%</span>';
    html += '          <span class="telemetry-timer"><svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>' + cd5h + '</span>';
    html += '        </div>';
    html += '      </div>';
    html += '      <div class="meter-track">';
    html += '        <div class="meter-ticks"><div class="meter-tick"></div><div class="meter-tick"></div></div>';
    html += '        <div class="meter-fill" style="width:' + pct5h + '%; background-color:' + color5h + '"></div>';
    html += '      </div>';
    html += '    </div>';

    html += '    <div class="telemetry-item">';
    html += '      <div class="telemetry-header">';
    html += '        <span class="telemetry-label">Недельный лимит</span>';
    html += '        <div class="telemetry-values">';
    html += '          <span class="telemetry-pct" style="color:' + colorWk + '">' + pctWk + '%</span>';
    html += '          <span class="telemetry-timer"><svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"></circle><polyline points="12 6 12 12 16 14"></polyline></svg>' + cdWk + '</span>';
    html += '        </div>';
    html += '      </div>';
    html += '      <div class="meter-track">';
    html += '        <div class="meter-ticks"><div class="meter-tick"></div><div class="meter-tick"></div></div>';
    html += '        <div class="meter-fill" style="width:' + pctWk + '%; background-color:' + colorWk + '"></div>';
    html += '      </div>';
    html += '    </div>';
    html += '  </div>';

    html += '  <div class="card-footer">';
    html += '    <div class="card-footer-left">';
    if (!isActive) {
      html += '      <button class="btn" onclick="onSwitchClick(\'' + escapeHtml(acc.email) + '\')"><svg width="10" height="10" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polygon points="5 3 19 12 5 21 5 3"></polygon></svg><span>Переключить</span></button>';
    }
    if (isPaused) {
      html += '      <button class="btn" onclick="togglePause(\'' + escapeHtml(acc.email) + '\', false)"><span>Включить</span></button>';
    } else {
      html += '      <button class="btn" onclick="togglePause(\'' + escapeHtml(acc.email) + '\', true)"><span>Пауза</span></button>';
    }
    html += '    </div>';
    html += '    <button class="btn btn-danger-outline" title="Удалить из пула" aria-label="Удалить аккаунт из пула" onclick="onDeleteClick(\'' + escapeHtml(acc.email) + '\')"><svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="3 6 5 6 21 6"></polyline><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6m3 0V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"></path></svg></button>';
    html += '  </div>';
    html += '</div>';
  });
  container.innerHTML = html;
  translate(container);
}

function fetchStatus() {
  fetch('/api/status')
    .then(function(res) { return res.json(); })
    .then(function(data) {
      state = data;
      language = resolveLanguage(data.config && data.config.language);
      document.documentElement.lang = language;
      document.getElementById('language-select').value = data.config.language || 'auto';
      translate(document.body);
      renderUI();
    })
    .catch(function() {});
}

function onCaptureClick() {
  var btn = document.getElementById('btn-capture');
  btn.disabled = true;
  showToast('Захват сессии из Antigravity...', false);
  fetch('/api/capture', { method: 'POST' })
    .then(function(res) { return res.json(); })
    .then(function(data) {
      btn.disabled = false;
      if (data.ok) {
        showToast('Сессия сохранена в пул', false);
        fetchStatus();
      } else {
        showToast('Ошибка захвата', true);
      }
    })
    .catch(function(err) {
      btn.disabled = false;
      showToast(tr('Ошибка сети: ') + err.message, true);
    });
}

function onRefreshClick() {
  var btn = document.getElementById('btn-refresh');
  btn.disabled = true;
  showToast('Запрос актуальных квот...', false);
  fetch('/api/refresh', { method: 'POST' })
    .then(function(res) { return res.json(); })
    .then(function(data) {
      btn.disabled = false;
      if (data.ok) {
        showToast(data.swapped ? 'Лимит исчерпан: выполнен переход' : 'Квоты обновлены', false);
        fetchStatus();
      } else {
        showToast('Ошибка обновления', true);
      }
    })
    .catch(function(err) {
      btn.disabled = false;
      showToast(tr('Ошибка сети: ') + err.message, true);
    });
}

function onSwitchClick(email) {
  showToast('Переключение сессии...', false);
  fetch('/api/switch', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: email })
  })
    .then(function(res) { return res.json(); })
    .then(function(data) {
      if (data.ok) {
        showToast(tr('Активен: ') + email, false);
        fetchStatus();
      } else {
        showToast('Ошибка переключения', true);
      }
    })
    .catch(function(err) {
      showToast(tr('Ошибка: ') + err.message, true);
    });
}

function togglePause(email, paused) {
  var acc = state.accounts.find(function(a) { return a.email === email; });
  var alias = acc ? acc.alias : '';
  var priority = acc ? (acc.priority || 0) : 0;
  fetch('/api/account/update', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: email, alias: alias, paused: paused, priority: priority })
  })
    .then(function() { fetchStatus(); })
    .catch(function(err) { showToast(tr('Ошибка: ') + err.message, true); });
}

function onDeleteClick(email) {
  if (!confirm(tr('Удалить аккаунт ') + email + tr(' из пула?'))) return;
  fetch('/api/account/delete', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: email })
  })
    .then(function() {
      showToast('Аккаунт удален', false);
      fetchStatus();
    })
    .catch(function(err) { showToast(tr('Ошибка: ') + err.message, true); });
}

function startEditAlias(email, currentAlias) {
  isEditingAlias = true;
  var wrap = document.getElementById('alias-wrap-' + email);
  if (!wrap) return;
  wrap.innerHTML = '<input type="text" class="alias-input" id="alias-input-' + escapeHtml(email) + '" value="' + escapeHtml(currentAlias) + '" placeholder="Имя...">';
  translate(wrap);
  var inp = document.getElementById('alias-input-' + email);
  inp.focus();
  inp.select();

  function commit() {
    isEditingAlias = false;
    var newAlias = inp.value.trim();
    var acc = state.accounts.find(function(a) { return a.email === email; });
    var paused = acc ? !!acc.paused : false;
    var prio = acc ? (acc.priority || 0) : 0;
    fetch('/api/account/update', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ email: email, alias: newAlias, paused: paused, priority: prio })
    }).then(function() { fetchStatus(); });
  }

  inp.onkeydown = function(e) {
    if (e.key === 'Enter') {
      commit();
    } else if (e.key === 'Escape') {
      isEditingAlias = false;
      renderUI();
    }
  };
  inp.onblur = commit;
}

function openSettingsModal() {
  if (state.config) {
    document.getElementById('cfg-autoswitch').checked = !!state.config.auto_switch_enabled;
    document.getElementById('cfg-thresh-5h').value = String(state.config.threshold_5h || 0.05);
    document.getElementById('cfg-thresh-wk').value = String(state.config.threshold_weekly || 0.05);
    document.getElementById('cfg-notify-mode').value = state.config.notify_mode || 'all';
  }
  document.getElementById('cfg-autostart').checked = !!state.autostart;
  document.getElementById('settings-modal').classList.remove('hidden');
}

function closeSettingsModal() {
  document.getElementById('settings-modal').classList.add('hidden');
}

function onBackdropClick(e) {
  if (e.target.id === 'settings-modal') {
    closeSettingsModal();
  }
}

function saveSettings() {
  var autoSwitch = document.getElementById('cfg-autoswitch').checked;
  var autostartVal = document.getElementById('cfg-autostart').checked;
  var thresh5h = parseFloat(document.getElementById('cfg-thresh-5h').value);
  var threshWk = parseFloat(document.getElementById('cfg-thresh-wk').value);
  var notifyMode = document.getElementById('cfg-notify-mode').value;

  fetch('/api/config', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      auto_switch_enabled: autoSwitch,
      autostart: autostartVal,
      threshold_5h: thresh5h,
      threshold_weekly: threshWk,
      notify_mode: notifyMode,
      notify_on_switch: notifyMode !== 'none'
    })
  })
    .then(function(res) { return res.json(); })
    .then(function(data) {
      if (data.ok) {
        showToast('Настройки сохранены', false);
        closeSettingsModal();
        fetchStatus();
      } else {
        showToast('Ошибка сохранения', true);
      }
    })
    .catch(function(err) {
      showToast(tr('Ошибка: ') + err.message, true);
    });
}

document.addEventListener('keydown', function(e) {
  if (e.key === 'Escape') {
    closeSettingsModal();
  } else if ((e.key === 'r' || e.key === 'R') && !isEditingAlias && document.activeElement.tagName !== 'INPUT') {
    onRefreshClick();
  }
});

fetchStatus();
setInterval(fetchStatus, 2500);
</script>
</body>
</html>`
