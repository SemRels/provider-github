// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The provider-github Authors

package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	plugin "github.com/SemRels/provider-github/internal/plugin"
)

func TestRunSuccess(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	uploadCalled := false
	code := run(context.Background(), envMap(map[string]string{
		"GITHUB_TOKEN":      "token",
		"GITHUB_REPOSITORY": "owner/repo",
		"SEMREL_TAG_NAME":   "v1.2.3",
	}), &stdout, &stderr, func(_ context.Context, cfg plugin.Config) (*plugin.Release, error) {
		if cfg.Token != "token" || cfg.Owner != "owner" || cfg.Repo != "repo" || cfg.TagName != "v1.2.3" {
			t.Fatalf("unexpected config: %+v", cfg)
		}
		return &plugin.Release{ID: 42, URL: "https://github.com/owner/repo/releases/tag/v1.2.3"}, nil
	}, func(_ context.Context, _ plugin.Config, _ *plugin.Release, _ io.Writer) {
		uploadCalled = true
	})
	if code != 0 {
		t.Fatalf("run() code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if !uploadCalled {
		t.Fatal("expected uploadAssets to be called")
	}
	if stderr.String() != "plugin_schema_version=1\n" || !strings.Contains(stdout.String(), "created v1.2.3") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunDryRun(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	uploadCalled := false
	code := run(context.Background(), envMap(map[string]string{
		"GITHUB_REPOSITORY":    "owner/repo",
		"SEMREL_NEXT_VERSION":  "1.2.3",
		"SEMREL_DRY_RUN":       "true",
		"SEMREL_PLUGIN_ASSETS": "dist/*.tar.gz",
	}), &stdout, &stderr, func(_ context.Context, cfg plugin.Config) (*plugin.Release, error) {
		if !cfg.DryRun || cfg.TagName != "v1.2.3" {
			t.Fatalf("unexpected config: %+v", cfg)
		}
		return &plugin.Release{URL: "https://github.com/owner/repo/releases/tag/v1.2.3"}, nil
	}, func(_ context.Context, _ plugin.Config, _ *plugin.Release, _ io.Writer) {
		uploadCalled = true
	})
	if code != 0 {
		t.Fatalf("run() code = %d, want 0 (stderr=%q)", code, stderr.String())
	}
	if uploadCalled {
		t.Fatal("uploadAssets should not be called during dry-run")
	}
	if stderr.String() != "plugin_schema_version=1\n" || !strings.Contains(stdout.String(), "dry-run") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunConfigError(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(context.Background(), envMap(map[string]string{
		"SEMREL_TAG_NAME": "v1.2.3",
	}), &stdout, &stderr, func(context.Context, plugin.Config) (*plugin.Release, error) {
		t.Fatal("createRelease should not be called")
		return nil, nil
	}, func(context.Context, plugin.Config, *plugin.Release, io.Writer) {
		t.Fatal("uploadAssets should not be called")
	})
	if code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "GITHUB_REPOSITORY") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunCreateReleaseError(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	code := run(context.Background(), envMap(map[string]string{
		"GITHUB_TOKEN":      "token",
		"GITHUB_REPOSITORY": "owner/repo",
		"SEMREL_TAG_NAME":   "v1.2.3",
	}), &stdout, &stderr, func(context.Context, plugin.Config) (*plugin.Release, error) {
		return nil, errors.New("boom")
	}, func(context.Context, plugin.Config, *plugin.Release, io.Writer) {
		t.Fatal("uploadAssets should not be called")
	})
	if code != 1 {
		t.Fatalf("run() code = %d, want 1", code)
	}
	if stdout.Len() != 0 || !strings.Contains(stderr.String(), "boom") {
		t.Fatalf("stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func envMap(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}
