// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The provider-github Authors

package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

func TestConfigFromEnvUsesPluginOverrides(t *testing.T) {
	t.Parallel()

	cfg, err := ConfigFromEnv(envMap(map[string]string{
		"SEMREL_PLUGIN_TOKEN":      "plugin-token",
		"GITHUB_TOKEN":             "fallback-token",
		"SEMREL_PLUGIN_OWNER":      "plugin-owner",
		"SEMREL_PLUGIN_REPO":       "plugin-repo",
		"GITHUB_REPOSITORY":        "fallback-owner/fallback-repo",
		"SEMREL_TAG_NAME":          "v1.2.3",
		"SEMREL_PLUGIN_NAME":       "Release 1.2.3",
		"SEMREL_CHANGELOG":         "notes",
		"SEMREL_PLUGIN_DRAFT":      "true",
		"SEMREL_PLUGIN_PRERELEASE": "false",
		"SEMREL_PLUGIN_BASE_URL":   "https://ghe.example/api/v3",
		"SEMREL_PLUGIN_ASSETS":     "dist/*.tar.gz, dist/*.zip",
	}))
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	if cfg.Token != "plugin-token" || cfg.Owner != "plugin-owner" || cfg.Repo != "plugin-repo" {
		t.Fatalf("unexpected repo config: %+v", cfg)
	}
	if cfg.BaseURL != "https://ghe.example/api/v3" || cfg.TagName != "v1.2.3" || cfg.Name != "Release 1.2.3" {
		t.Fatalf("unexpected release config: %+v", cfg)
	}
	if cfg.Assets != "dist/*.tar.gz, dist/*.zip" {
		t.Fatalf("unexpected assets: %+v", cfg)
	}
	if !cfg.Draft || cfg.Prerelease || cfg.DryRun {
		t.Fatalf("unexpected flags: %+v", cfg)
	}
}

func TestConfigFromEnvParsesFallbacks(t *testing.T) {
	t.Parallel()

	cfg, err := ConfigFromEnv(envMap(map[string]string{
		"GITHUB_TOKEN":        "fallback-token",
		"GITHUB_REPOSITORY":   "SemRels/provider-github",
		"SEMREL_NEXT_VERSION": "1.2.3-alpha.1",
		"SEMREL_CHANGELOG":    "notes",
		"SEMREL_DRY_RUN":      "true",
	}))
	if err != nil {
		t.Fatalf("ConfigFromEnv() error = %v", err)
	}

	if cfg.Token != "fallback-token" || cfg.Owner != "SemRels" || cfg.Repo != "provider-github" {
		t.Fatalf("unexpected repo fallback: %+v", cfg)
	}
	if cfg.BaseURL != defaultBaseURL || cfg.TagName != "v1.2.3-alpha.1" || cfg.Name != "v1.2.3-alpha.1" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if !cfg.Prerelease || !cfg.DryRun {
		t.Fatalf("expected prerelease dry-run config, got %+v", cfg)
	}
}

func TestConfigFromEnvValidationErrors(t *testing.T) {
	t.Parallel()

	tests := map[string]map[string]string{
		"missing repository": {
			"GITHUB_TOKEN":    "token",
			"SEMREL_TAG_NAME": "v1.2.3",
		},
		"missing tag": {
			"GITHUB_TOKEN":      "token",
			"GITHUB_REPOSITORY": "owner/repo",
		},
		"missing token": {
			"GITHUB_REPOSITORY": "owner/repo",
			"SEMREL_TAG_NAME":   "v1.2.3",
		},
		"invalid repository": {
			"GITHUB_TOKEN":      "token",
			"GITHUB_REPOSITORY": "owner",
			"SEMREL_TAG_NAME":   "v1.2.3",
		},
	}

	for name, values := range tests {
		name, values := name, values
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if _, err := ConfigFromEnv(envMap(values)); err == nil {
				t.Fatalf("ConfigFromEnv() error = nil, want non-nil")
			}
		})
	}
}

func TestCreateReleaseSuccess(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/releases" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.Header.Get("X-GitHub-Api-Version"); got != "" {
			t.Fatalf("x-github-api-version = %q, want empty for custom base URL", got)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("json decode: %v", err)
		}
		if payload["tag_name"] != "v1.2.3" || payload["name"] != "v1.2.3" || payload["body"] != "notes" {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		if payload["draft"] != true || payload["prerelease"] != false {
			t.Fatalf("unexpected flags: %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":42,"html_url":"https://github.com/owner/repo/releases/tag/v1.2.3","upload_url":"https://uploads.example.test/repos/owner/repo/releases/42/assets{?name,label}"}`))
	}))
	defer server.Close()

	creator := New(server.Client())
	release, err := creator.CreateRelease(context.Background(), Config{
		Token:   "token",
		Owner:   "owner",
		Repo:    "repo",
		BaseURL: server.URL,
		TagName: "v1.2.3",
		Name:    "v1.2.3",
		Body:    "notes",
		Draft:   true,
	})
	if err != nil {
		t.Fatalf("CreateRelease() error = %v", err)
	}
	if release.ID != 42 || release.URL != "https://github.com/owner/repo/releases/tag/v1.2.3" || release.UploadURL == "" {
		t.Fatalf("unexpected release: %+v", release)
	}
}

func TestCreateReleaseDryRun(t *testing.T) {
	t.Parallel()

	release, err := CreateRelease(context.Background(), Config{
		Owner:   "owner",
		Repo:    "repo",
		TagName: "v1.2.3",
		Name:    "v1.2.3",
		DryRun:  true,
	})
	if err != nil {
		t.Fatalf("CreateRelease() error = %v", err)
	}
	if release.ID != 0 || release.URL != "https://github.com/owner/repo/releases/tag/v1.2.3" {
		t.Fatalf("unexpected dry-run release: %+v", release)
	}
}

func TestCreateReleaseErrors(t *testing.T) {
	t.Parallel()

	t.Run("http error message", func(t *testing.T) {
		t.Parallel()
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_, _ = w.Write([]byte(`{"message":"Validation Failed"}`))
		}))
		defer server.Close()

		_, err := CreateRelease(context.Background(), Config{
			Token:   "token",
			Owner:   "owner",
			Repo:    "repo",
			BaseURL: server.URL,
			TagName: "v1.2.3",
			Name:    "v1.2.3",
		})
		if err == nil || !strings.Contains(err.Error(), "Validation Failed") {
			t.Fatalf("CreateRelease() error = %v, want validation failure", err)
		}
	})

	t.Run("invalid base url", func(t *testing.T) {
		t.Parallel()
		_, err := CreateRelease(context.Background(), Config{
			Token:   "token",
			Owner:   "owner",
			Repo:    "repo",
			BaseURL: "://bad",
			TagName: "v1.2.3",
			Name:    "v1.2.3",
		})
		if err == nil || !strings.Contains(err.Error(), "parse github base url") {
			t.Fatalf("CreateRelease() error = %v, want parse error", err)
		}
	})

	t.Run("transport failure", func(t *testing.T) {
		creator := New(&http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, errors.New("boom")
		})})
		_, err := creator.CreateRelease(context.Background(), Config{
			Token:   "token",
			Owner:   "owner",
			Repo:    "repo",
			BaseURL: "https://api.github.com",
			TagName: "v1.2.3",
			Name:    "v1.2.3",
		})
		if err == nil || !strings.Contains(err.Error(), "post release") {
			t.Fatalf("CreateRelease() error = %v, want transport error", err)
		}
	})
}

func TestUploadReleaseAssetsNoAssetsConfigured(t *testing.T) {
	t.Parallel()

	var called atomic.Bool
	cli := &client{httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called.Store(true)
		return nil, errors.New("should not be called")
	})}}
	var stderr bytes.Buffer
	cli.UploadReleaseAssets(context.Background(), Config{}, &Release{ID: 42}, &stderr)
	if called.Load() {
		t.Fatal("expected no upload attempt")
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestUploadReleaseAssetsNoMatchesWarns(t *testing.T) {

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(cwd); chdirErr != nil {
			t.Fatalf("Chdir cleanup error = %v", chdirErr)
		}
	})
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	var called atomic.Bool
	cli := &client{httpClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		called.Store(true)
		return nil, errors.New("should not be called")
	})}}
	var stderr bytes.Buffer
	cli.UploadReleaseAssets(context.Background(), Config{Assets: "dist/*.zip"}, &Release{ID: 42}, &stderr)
	if called.Load() {
		t.Fatal("expected no upload attempt")
	}
	if got := stderr.String(); !strings.Contains(got, `asset pattern "dist/*.zip" matched no files`) {
		t.Fatalf("stderr = %q", got)
	}
}

func TestUploadReleaseAssetsExpandsGlobs(t *testing.T) {

	dir := t.TempDir()
	assetOne := filepath.Join(dir, "dist", "app_linux.tar.gz")
	assetTwo := filepath.Join(dir, "dist", "app_darwin.zip")
	if err := os.MkdirAll(filepath.Dir(assetOne), 0o755); err != nil {
		t.Fatalf("MkdirAll() error = %v", err)
	}
	if err := os.WriteFile(assetOne, []byte("linux"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	if err := os.WriteFile(assetTwo, []byte("darwin"), 0o644); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd() error = %v", err)
	}
	t.Cleanup(func() {
		if chdirErr := os.Chdir(cwd); chdirErr != nil {
			t.Fatalf("Chdir cleanup error = %v", chdirErr)
		}
	})
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("Chdir() error = %v", err)
	}

	uploaded := make(chan string, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/repos/owner/repo/releases/42/assets" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token" {
			t.Fatalf("authorization = %q", got)
		}
		if got := r.URL.Query().Get("name"); got == "" {
			t.Fatal("missing asset name query")
		}
		// GitHub's real asset-upload endpoint requires an explicit
		// Content-Length and rejects chunked transfer encoding, so guard
		// against regressing to an *os.File body without a set length.
		if r.ContentLength < 0 {
			t.Fatalf("ContentLength = %d, want a non-negative explicit length (chunked transfer encoding not allowed)", r.ContentLength)
		}
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll() error = %v", err)
		}
		if int64(len(body)) != r.ContentLength {
			t.Fatalf("body length = %d, Content-Length header = %d", len(body), r.ContentLength)
		}
		uploaded <- r.URL.Query().Get("name") + ":" + string(body)
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"state":"uploaded"}`))
	}))
	defer server.Close()

	var stderr bytes.Buffer
	cli := &client{httpClient: server.Client()}
	cli.UploadReleaseAssets(context.Background(), Config{
		Token:   "token",
		Owner:   "owner",
		Repo:    "repo",
		BaseURL: server.URL,
		Assets:  "dist/*.tar.gz, dist/*.zip",
	}, &Release{ID: 42}, &stderr)
	close(uploaded)

	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	got := make([]string, 0, 2)
	for item := range uploaded {
		got = append(got, item)
	}
	if len(got) != 2 {
		t.Fatalf("uploaded %d assets, want 2 (%v)", len(got), got)
	}
	joined := strings.Join(got, ",")
	if !strings.Contains(joined, "app_linux.tar.gz:linux") || !strings.Contains(joined, "app_darwin.zip:darwin") {
		t.Fatalf("unexpected uploads: %v", got)
	}
}

func TestHelpers(t *testing.T) {
	t.Parallel()

	if got := coalesce("", "  ", "value", "later"); got != "value" {
		t.Fatalf("coalesce() = %q, want value", got)
	}
	if got := normalizeVersion("1.2.3"); got != "v1.2.3" {
		t.Fatalf("normalizeVersion() = %q", got)
	}
	if got := normalizeVersion("v1.2.3"); got != "v1.2.3" {
		t.Fatalf("normalizeVersion() = %q", got)
	}
	if value, ok := parseBool("yes"); !ok || !value {
		t.Fatalf("parseBool(yes) = %v, %v", value, ok)
	}
	if value, ok := parseBool("off"); !ok || value {
		t.Fatalf("parseBool(off) = %v, %v", value, ok)
	}
	if value, ok := parseBool("maybe"); ok || value {
		t.Fatalf("parseBool(maybe) = %v, %v", value, ok)
	}
	if message := readMessage(strings.NewReader(`{"message":"boom"}`)); message != "boom" {
		t.Fatalf("readMessage() = %q", message)
	}
	if message := readMessage(strings.NewReader("not-json")); message != "" {
		t.Fatalf("readMessage() = %q, want empty", message)
	}
	if got := splitAssets(" dist/*.tgz, ,dist/*.zip "); len(got) != 2 || got[0] != "dist/*.tgz" || got[1] != "dist/*.zip" {
		t.Fatalf("splitAssets() = %#v", got)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func envMap(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
