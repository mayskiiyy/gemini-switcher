package updater

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name       string
		current    string
		latest     string
		wantUpdate bool
		wantErr    bool
	}{
		{
			name:       "patch bump with v prefix",
			current:    "v1.0.0",
			latest:     "v1.0.1",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "minor bump with v prefix",
			current:    "v1.0.0",
			latest:     "v1.1.0",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "major bump with v prefix",
			current:    "v1.0.0",
			latest:     "v2.0.0",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "identical versions with v prefix",
			current:    "v1.0.0",
			latest:     "v1.0.0",
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name:       "identical versions without v prefix",
			current:    "1.0.0",
			latest:     "1.0.0",
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name:       "patch bump without v prefix",
			current:    "1.0.0",
			latest:     "1.0.1",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "mixed v prefix current has v",
			current:    "v1.0.0",
			latest:     "1.0.1",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "mixed v prefix latest has v",
			current:    "1.0.0",
			latest:     "v1.0.1",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "older patch version",
			current:    "v1.0.1",
			latest:     "v1.0.0",
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name:       "older minor version",
			current:    "v1.2.0",
			latest:     "v1.1.9",
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name:       "older major version",
			current:    "v2.0.0",
			latest:     "v1.9.9",
			wantUpdate: false,
			wantErr:    false,
		},
		{
			name:       "prerelease on latest",
			current:    "v1.0.0",
			latest:     "v1.0.1-rc1",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "whitespace trimmed",
			current:    " v1.0.0 ",
			latest:     " v1.0.1 ",
			wantUpdate: true,
			wantErr:    false,
		},
		{
			name:       "invalid current version string",
			current:    "invalid",
			latest:     "v1.0.0",
			wantUpdate: false,
			wantErr:    true,
		},
		{
			name:       "invalid latest version string",
			current:    "v1.0.0",
			latest:     "invalid",
			wantUpdate: false,
			wantErr:    true,
		},
		{
			name:       "empty current version",
			current:    "",
			latest:     "v1.0.0",
			wantUpdate: false,
			wantErr:    true,
		},
		{
			name:       "empty latest version",
			current:    "v1.0.0",
			latest:     "",
			wantUpdate: false,
			wantErr:    true,
		},
		{
			name:       "two part version",
			current:    "1.0",
			latest:     "1.0.0",
			wantUpdate: false,
			wantErr:    true,
		},
		{
			name:       "four part version",
			current:    "1.0.0.0",
			latest:     "1.0.0",
			wantUpdate: false,
			wantErr:    true,
		},
		{
			name:       "non-numeric part",
			current:    "1.0.a",
			latest:     "1.0.0",
			wantUpdate: false,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CompareVersions(tt.current, tt.latest)
			if (err != nil) != tt.wantErr {
				t.Fatalf("CompareVersions(%q, %q) error = %v, wantErr %v", tt.current, tt.latest, err, tt.wantErr)
			}
			if got != tt.wantUpdate {
				t.Fatalf("CompareVersions(%q, %q) = %v, want %v", tt.current, tt.latest, got, tt.wantUpdate)
			}
		})
	}
}

func TestCheckWithEndpoint(t *testing.T) {
	t.Run("has newer update", func(t *testing.T) {
		var receivedUserAgent string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			receivedUserAgent = r.Header.Get("User-Agent")
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"tag_name": "v1.1.0",
				"html_url": "https://github.com/mayskiiyy/gemini-switcher/releases/tag/v1.1.0",
				"body": "Fix bug and add feature",
				"published_at": "2026-11-05T14:30:00Z"
			}`))
		}))
		defer srv.Close()

		info, err := checkWithEndpoint(context.Background(), srv.URL, "v1.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if receivedUserAgent != "Gemini-Switcher/1.0.0" {
			t.Errorf("User-Agent = %q, want %q", receivedUserAgent, "Gemini-Switcher/1.0.0")
		}
		if info.CurrentVersion != "v1.0.0" {
			t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, "v1.0.0")
		}
		if info.LatestVersion != "v1.1.0" {
			t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "v1.1.0")
		}
		if !info.HasUpdate {
			t.Errorf("HasUpdate = false, want true")
		}
		if info.ReleaseURL != "https://github.com/mayskiiyy/gemini-switcher/releases/tag/v1.1.0" {
			t.Errorf("ReleaseURL = %q, want %q", info.ReleaseURL, "https://github.com/mayskiiyy/gemini-switcher/releases/tag/v1.1.0")
		}
		if info.ReleaseNotes != "Fix bug and add feature" {
			t.Errorf("ReleaseNotes = %q, want %q", info.ReleaseNotes, "Fix bug and add feature")
		}
		expectedTime, _ := time.Parse(time.RFC3339, "2026-11-05T14:30:00Z")
		if !info.PublishedAt.Equal(expectedTime) {
			t.Errorf("PublishedAt = %v, want %v", info.PublishedAt, expectedTime)
		}
	})

	t.Run("already up to date", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"tag_name": "v1.0.0",
				"html_url": "https://github.com/mayskiiyy/gemini-switcher/releases/tag/v1.0.0",
				"body": "Initial release",
				"published_at": "2026-11-01T10:00:00Z"
			}`))
		}))
		defer srv.Close()

		info, err := checkWithEndpoint(context.Background(), srv.URL, "v1.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.HasUpdate {
			t.Errorf("HasUpdate = true, want false")
		}
		if info.LatestVersion != "v1.0.0" {
			t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "v1.0.0")
		}
	})

	t.Run("release endpoint returns 404", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer srv.Close()

		info, err := checkWithEndpoint(context.Background(), srv.URL, "v1.0.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if info.HasUpdate {
			t.Errorf("HasUpdate = true, want false")
		}
		if info.CurrentVersion != "v1.0.0" {
			t.Errorf("CurrentVersion = %q, want %q", info.CurrentVersion, "v1.0.0")
		}
		if info.LatestVersion != "v1.0.0" {
			t.Errorf("LatestVersion = %q, want %q", info.LatestVersion, "v1.0.0")
		}
		if info.ReleaseURL != DefaultRepoURL {
			t.Errorf("ReleaseURL = %q, want %q", info.ReleaseURL, DefaultRepoURL)
		}
	})

	t.Run("release endpoint returns 500 error", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		_, err := checkWithEndpoint(context.Background(), srv.URL, "v1.0.0")
		if err == nil {
			t.Fatalf("expected error for 500 status, got nil")
		}
	})

	t.Run("invalid json response", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{not-json`))
		}))
		defer srv.Close()

		_, err := checkWithEndpoint(context.Background(), srv.URL, "v1.0.0")
		if err == nil {
			t.Fatalf("expected error for malformed json, got nil")
		}
	})

	t.Run("invalid version in tag", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{
				"tag_name": "unknown-build",
				"html_url": "https://github.com/mayskiiyy/gemini-switcher/releases/tag/unknown",
				"body": "Test",
				"published_at": "2026-11-01T10:00:00Z"
			}`))
		}))
		defer srv.Close()

		_, err := checkWithEndpoint(context.Background(), srv.URL, "v1.0.0")
		if err == nil {
			t.Fatalf("expected error for invalid tag semver, got nil")
		}
	})
}
