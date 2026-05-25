// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The provider-github Authors

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	plugin "github.com/SemRels/provider-github/internal/plugin"
)

func run(ctx context.Context, getenv func(string) string, stdout, stderr io.Writer, createRelease func(context.Context, plugin.Config) (*plugin.Release, error)) int {
	cfg, err := plugin.ConfigFromEnv(getenv)
	if err != nil {
		fmt.Fprintln(stderr, "provider-github:", err)
		return 1
	}

	release, err := createRelease(ctx, cfg)
	if err != nil {
		fmt.Fprintln(stderr, "provider-github:", err)
		return 1
	}

	if cfg.DryRun {
		fmt.Fprintf(stdout, "provider-github: dry-run: would create %s for %s/%s at %s\n", cfg.TagName, cfg.Owner, cfg.Repo, release.URL)
		return 0
	}

	fmt.Fprintf(stdout, "provider-github: created %s for %s/%s (id=%d) %s\n", cfg.TagName, cfg.Owner, cfg.Repo, release.ID, release.URL)
	return 0
}

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	os.Exit(run(ctx, os.Getenv, os.Stdout, os.Stderr, plugin.CreateRelease))
}
