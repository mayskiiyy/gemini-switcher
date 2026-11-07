package updater

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const (
	CurrentVersion = "v1.0.0"
	DefaultRepoURL = "https://github.com/mayskiiyy/gemini-switcher"
)

var LatestEndpoint = "https://api.github.com/repos/mayskiiyy/gemini-switcher/releases/latest"

type UpdateInfo struct {
	CurrentVersion string
	LatestVersion  string
	HasUpdate      bool
	ReleaseURL     string
	ReleaseNotes   string
	PublishedAt    time.Time
}

func CheckForUpdate(ctx context.Context) (*UpdateInfo, error) {
	return checkWithEndpoint(ctx, LatestEndpoint, CurrentVersion)
}

func checkWithEndpoint(ctx context.Context, endpoint, currentVer string) (*UpdateInfo, error) {
	reqCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Gemini-Switcher/1.0.0")
	req.Header.Set("Accept", "application/vnd.github.v3+json")

	client := &http.Client{
		Timeout: 6 * time.Second,
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return &UpdateInfo{
			CurrentVersion: currentVer,
			LatestVersion:  currentVer,
			HasUpdate:      false,
			ReleaseURL:     DefaultRepoURL,
		}, nil
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status code: %d", resp.StatusCode)
	}

	var rel struct {
		TagName     string    `json:"tag_name"`
		HTMLURL     string    `json:"html_url"`
		Body        string    `json:"body"`
		PublishedAt time.Time `json:"published_at"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release json: %w", err)
	}

	hasUpdate, err := CompareVersions(currentVer, rel.TagName)
	if err != nil {
		return nil, err
	}

	releaseURL := rel.HTMLURL
	if releaseURL == "" {
		releaseURL = DefaultRepoURL
	}

	return &UpdateInfo{
		CurrentVersion: currentVer,
		LatestVersion:  rel.TagName,
		HasUpdate:      hasUpdate,
		ReleaseURL:     releaseURL,
		ReleaseNotes:   rel.Body,
		PublishedAt:    rel.PublishedAt,
	}, nil
}

func CompareVersions(current, latest string) (bool, error) {
	curr, err := parseVersion(current)
	if err != nil {
		return false, err
	}
	lat, err := parseVersion(latest)
	if err != nil {
		return false, err
	}

	for i := 0; i < 3; i++ {
		if lat[i] > curr[i] {
			return true, nil
		}
		if lat[i] < curr[i] {
			return false, nil
		}
	}
	return false, nil
}

func parseVersion(v string) ([3]int, error) {
	v = strings.TrimSpace(v)
	v = strings.TrimPrefix(v, "v")
	v = strings.TrimPrefix(v, "V")
	if v == "" {
		return [3]int{}, fmt.Errorf("empty version string")
	}

	if idx := strings.IndexAny(v, "-+"); idx != -1 {
		v = v[:idx]
	}

	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return [3]int{}, fmt.Errorf("invalid semver format: %s", v)
	}

	var res [3]int
	for i := 0; i < 3; i++ {
		p := parts[i]
		if p == "" {
			return [3]int{}, fmt.Errorf("invalid empty semver part in: %s", v)
		}
		val, err := strconv.Atoi(p)
		if err != nil || val < 0 {
			return [3]int{}, fmt.Errorf("invalid semver part %q in: %s", p, v)
		}
		res[i] = val
	}
	return res, nil
}
