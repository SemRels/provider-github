// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The provider-github Authors

package plugin

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.github.com"

// Config contains the release request derived from the SemRel environment.
type Config struct {
	Token      string
	Owner      string
	Repo       string
	BaseURL    string
	TagName    string
	Name       string
	Body       string
	Draft      bool
	Prerelease bool
	DryRun     bool
}

// Release is the minimal GitHub release response used by the subprocess entrypoint.
type Release struct {
	ID  int64
	URL string
}

// Creator creates GitHub releases.
type Creator interface {
	CreateRelease(context.Context, Config) (*Release, error)
}

type client struct {
	httpClient *http.Client
}

// New returns a Creator backed by the GitHub releases API.
func New(httpClient *http.Client) Creator {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 30 * time.Second}
	}
	return &client{httpClient: httpClient}
}

// ConfigFromEnv builds release configuration from SemRel and plugin environment variables.
func ConfigFromEnv(getenv func(string) string) (Config, error) {
	var repoOwner string
	var repoName string
	repository := strings.TrimSpace(getenv("GITHUB_REPOSITORY"))
	if repository != "" {
		parsedOwner, parsedRepo, err := parseRepository(repository)
		if err != nil {
			return Config{}, err
		}
		repoOwner = parsedOwner
		repoName = parsedRepo
	}

	tagName := strings.TrimSpace(coalesce(
		getenv("SEMREL_PLUGIN_TAG_NAME"),
		getenv("SEMREL_TAG_NAME"),
		normalizeVersion(getenv("SEMREL_NEXT_VERSION")),
		normalizeVersion(getenv("SEMREL_VERSION")),
	))
	prerelease, hasPrerelease := parseBool(getenv("SEMREL_PLUGIN_PRERELEASE"))
	if !hasPrerelease {
		prerelease = strings.Contains(tagName, "-")
	}

	cfg := Config{
		Token:      strings.TrimSpace(coalesce(getenv("SEMREL_PLUGIN_TOKEN"), getenv("GITHUB_TOKEN"))),
		Owner:      strings.TrimSpace(coalesce(getenv("SEMREL_PLUGIN_OWNER"), repoOwner)),
		Repo:       strings.TrimSpace(coalesce(getenv("SEMREL_PLUGIN_REPO"), repoName)),
		BaseURL:    strings.TrimSpace(coalesce(getenv("SEMREL_PLUGIN_BASE_URL"), getenv("SEMREL_PLUGIN_API_URL"), defaultBaseURL)),
		TagName:    tagName,
		Name:       strings.TrimSpace(coalesce(getenv("SEMREL_PLUGIN_NAME"), tagName)),
		Body:       getenv("SEMREL_CHANGELOG"),
		Draft:      parseBoolValue(getenv("SEMREL_PLUGIN_DRAFT")),
		Prerelease: prerelease,
		DryRun:     parseBoolValue(getenv("SEMREL_DRY_RUN")),
	}

	if err := validateConfig(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// CreateRelease uses the default HTTP client to create a GitHub release.
func CreateRelease(ctx context.Context, cfg Config) (*Release, error) {
	return New(nil).CreateRelease(ctx, cfg)
}

func (c *client) CreateRelease(ctx context.Context, cfg Config) (*Release, error) {
	if err := validateConfig(cfg); err != nil {
		return nil, err
	}

	releaseURL := fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", cfg.Owner, cfg.Repo, cfg.TagName)
	if cfg.DryRun {
		return &Release{ID: 0, URL: releaseURL}, nil
	}

	baseURL, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/") + "/")
	if err != nil {
		return nil, fmt.Errorf("parse github base url: %w", err)
	}

	payload := struct {
		TagName    string `json:"tag_name"`
		Name       string `json:"name"`
		Body       string `json:"body"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}{
		TagName:    cfg.TagName,
		Name:       cfg.Name,
		Body:       cfg.Body,
		Draft:      cfg.Draft,
		Prerelease: cfg.Prerelease,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal release payload: %w", err)
	}

	endpoint, err := baseURL.Parse(fmt.Sprintf("repos/%s/%s/releases", url.PathEscape(cfg.Owner), url.PathEscape(cfg.Repo)))
	if err != nil {
		return nil, fmt.Errorf("build releases endpoint: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint.String(), bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("build release request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "provider-github")
	if baseURL.Host == "api.github.com" {
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post release: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		message := readMessage(resp.Body)
		if message == "" {
			message = resp.Status
		}
		return nil, fmt.Errorf("create release failed: %s", message)
	}

	var parsed struct {
		ID      int64  `json:"id"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, fmt.Errorf("decode release response: %w", err)
	}

	return &Release{ID: parsed.ID, URL: parsed.HTMLURL}, nil
}

func validateConfig(cfg Config) error {
	if cfg.Owner == "" || cfg.Repo == "" {
		return fmt.Errorf("SEMREL_PLUGIN_OWNER/SEMREL_PLUGIN_REPO or GITHUB_REPOSITORY is required")
	}
	if cfg.TagName == "" {
		return fmt.Errorf("SEMREL_TAG_NAME, SEMREL_NEXT_VERSION, or SEMREL_VERSION is required")
	}
	if cfg.Name == "" {
		return fmt.Errorf("release name is required")
	}
	if !cfg.DryRun && cfg.Token == "" {
		return fmt.Errorf("SEMREL_PLUGIN_TOKEN or GITHUB_TOKEN is required")
	}
	return nil
}

func parseRepository(repository string) (string, string, error) {
	repository = strings.TrimSpace(repository)
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		return "", "", fmt.Errorf("GITHUB_REPOSITORY must be owner/repo")
	}
	return strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1]), nil
}

func normalizeVersion(version string) string {
	version = strings.TrimSpace(version)
	if version == "" {
		return ""
	}
	if strings.HasPrefix(version, "v") {
		return version
	}
	return "v" + version
}

func parseBoolValue(raw string) bool {
	value, _ := parseBool(raw)
	return value
}

func parseBool(raw string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "1", "true", "yes", "on":
		return true, true
	case "0", "false", "no", "off":
		return false, true
	default:
		return false, false
	}
}

func readMessage(body io.Reader) string {
	var payload struct {
		Message string `json:"message"`
	}
	if err := json.NewDecoder(body).Decode(&payload); err == nil && strings.TrimSpace(payload.Message) != "" {
		return payload.Message
	}
	return ""
}

func coalesce(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
