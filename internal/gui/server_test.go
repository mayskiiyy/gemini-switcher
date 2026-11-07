package gui

import (
	"bytes"
	"encoding/json"
	"gemini-switcher/internal/model"
	"gemini-switcher/internal/storage"
	"gemini-switcher/internal/switcher"
	"gemini-switcher/internal/updater"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func setupTestServer(t *testing.T) (*Server, *storage.Storage, *switcher.Switcher, func()) {
	tmpDir, err := os.MkdirTemp("", "gui-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	st, err := storage.NewStorage(tmpDir)
	if err != nil {
		os.RemoveAll(tmpDir)
		t.Fatalf("failed to create storage: %v", err)
	}

	sw := switcher.NewSwitcher(st)
	srv := NewServer(st, sw)

	cleanup := func() {
		_ = srv.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return srv, st, sw, cleanup
}

func TestDashboardHandler(t *testing.T) {
	srv, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	srv.handleDashboard(w, req)

	resp := w.Result()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Gemini Switcher") {
		t.Fatalf("expected body to contain 'Gemini Switcher', got: %s", body)
	}

	req404 := httptest.NewRequest(http.MethodGet, "/random-path", nil)
	w404 := httptest.NewRecorder()
	srv.handleDashboard(w404, req404)
	if w404.Result().StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown path, got %d", w404.Result().StatusCode)
	}
}

func TestStatusHandler(t *testing.T) {
	srv, st, _, cleanup := setupTestServer(t)
	defer cleanup()

	acc1 := &model.AccountProfile{
		Email: "acc1@example.com",
		Alias: "Primary",
		Quota5H: model.QuotaBucket{
			RemainingFraction: 0.85,
		},
		QuotaWeekly: model.QuotaBucket{
			RemainingFraction: 0.90,
		},
	}
	_ = st.SaveAccounts([]*model.AccountProfile{acc1})

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()
	srv.handleStatus(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Result().StatusCode)
	}

	var res statusResponse
	if err := json.NewDecoder(w.Body).Decode(&res); err != nil {
		t.Fatalf("failed to decode status response: %v", err)
	}

	if len(res.Accounts) != 1 || res.Accounts[0].Email != "acc1@example.com" {
		t.Fatalf("unexpected accounts in response: %+v", res.Accounts)
	}
	if res.Config == nil {
		t.Fatalf("expected non-nil config in response")
	}
}

func TestConfigHandler(t *testing.T) {
	srv, st, _, cleanup := setupTestServer(t)
	defer cleanup()

	newT5H := 0.10
	newTWk := 0.01
	newAuto := false
	newNotify := "switch_only"
	payload := map[string]interface{}{
		"auto_switch_enabled": newAuto,
		"threshold_5h":        newT5H,
		"threshold_weekly":    newTWk,
		"notify_mode":         newNotify,
	}
	bodyBytes, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/api/config", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	srv.handleConfig(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Result().StatusCode, w.Body.String())
	}

	cfg, err := st.LoadConfig()
	if err != nil {
		t.Fatalf("failed to load saved config: %v", err)
	}
	if cfg.AutoSwitchEnabled != false || cfg.Threshold5H != 0.10 || cfg.ThresholdWeekly != 0.01 || cfg.NotifyMode != "switch_only" {
		t.Fatalf("config not updated as expected: %+v", cfg)
	}
}

func TestAccountUpdateAndDelete(t *testing.T) {
	srv, st, _, cleanup := setupTestServer(t)
	defer cleanup()

	acc1 := &model.AccountProfile{
		Email:    "test@example.com",
		Alias:    "OldAlias",
		Paused:   false,
		Priority: 0,
	}
	_ = st.SaveAccounts([]*model.AccountProfile{acc1})

	updatePayload := map[string]interface{}{
		"email":    "test@example.com",
		"alias":    "NewAlias",
		"paused":   true,
		"priority": 2,
	}
	uBytes, _ := json.Marshal(updatePayload)
	req := httptest.NewRequest(http.MethodPost, "/api/account/update", bytes.NewReader(uBytes))
	w := httptest.NewRecorder()
	srv.handleAccountUpdate(w, req)

	if w.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Result().StatusCode)
	}

	accs, _ := st.LoadAccounts()
	if len(accs) != 1 || accs[0].Alias != "NewAlias" || !accs[0].Paused || accs[0].Priority != 2 {
		t.Fatalf("account update failed: %+v", accs[0])
	}

	delPayload := map[string]interface{}{
		"email": "test@example.com",
	}
	dBytes, _ := json.Marshal(delPayload)
	reqDel := httptest.NewRequest(http.MethodPost, "/api/account/delete", bytes.NewReader(dBytes))
	wDel := httptest.NewRecorder()
	srv.handleAccountDelete(wDel, reqDel)

	if wDel.Result().StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", wDel.Result().StatusCode)
	}

	accsAfter, _ := st.LoadAccounts()
	if len(accsAfter) != 0 {
		t.Fatalf("account delete failed, accounts remaining: %d", len(accsAfter))
	}
}

func TestServerStartAndLifecycle(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "gui-lifecycle-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	st, _ := storage.NewStorage(tmpDir)
	sw := switcher.NewSwitcher(st)

	srv, err := StartServer(st, sw)
	if err != nil {
		t.Fatalf("StartServer failed: %v", err)
	}
	defer srv.Close()

	if srv.Port() <= 0 {
		t.Fatalf("invalid server port: %d", srv.Port())
	}
	if !strings.HasPrefix(srv.URL(), "http://127.0.0.1:") {
		t.Fatalf("invalid server url: %s", srv.URL())
	}

	resp, err := http.Get(srv.URL() + "/api/status")
	if err != nil {
		t.Fatalf("failed to query started server: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 from live server, got %d", resp.StatusCode)
	}
}

func TestLanguageConfig(t *testing.T) {
	srv, st, _, cleanup := setupTestServer(t)
	defer cleanup()
	for _, lang := range []string{"en", "de", "pl", "uk", "ru", "auto"} {
		req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"language":"`+lang+`"}`))
		rec := httptest.NewRecorder()
		srv.handleConfig(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("language %s: status %d", lang, rec.Code)
		}
		cfg, err := st.LoadConfig()
		if err != nil || cfg.Language != lang {
			t.Fatalf("language %s: config=%+v err=%v", lang, cfg, err)
		}
	}
	req := httptest.NewRequest(http.MethodPost, "/api/config", strings.NewReader(`{"language":"invalid"}`))
	rec := httptest.NewRecorder()
	srv.handleConfig(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid language: status %d", rec.Code)
	}
}

func TestCheckUpdateHandler(t *testing.T) {
	srv, _, _, cleanup := setupTestServer(t)
	defer cleanup()

	mockRelease := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.1.0","html_url":"https://github.com/mayskiiyy/gemini-switcher/releases/tag/v1.1.0"}`))
	}))
	defer mockRelease.Close()

	orig := updater.LatestEndpoint
	updater.LatestEndpoint = mockRelease.URL
	defer func() { updater.LatestEndpoint = orig }()

	req := httptest.NewRequest(http.MethodGet, "/api/check-update", nil)
	rec := httptest.NewRecorder()
	srv.handleCheckUpdate(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}

	var info updater.UpdateInfo
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if !info.HasUpdate || info.LatestVersion != "v1.1.0" {
		t.Fatalf("unexpected info: %+v", info)
	}

	reqBad := httptest.NewRequest(http.MethodDelete, "/api/check-update", nil)
	recBad := httptest.NewRecorder()
	srv.handleCheckUpdate(recBad, reqBad)
	if recBad.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", recBad.Code)
	}
}
