// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The provider-github Authors

package main

import (
	"log"
	"os"
	"strings"

	grpcserver "github.com/SemRels/provider-github/internal/grpc"
	semrelplugin "github.com/SemRels/provider-github/internal/plugin"
)

func main() {
	log.SetOutput(os.Stderr)
	log.SetFlags(log.LstdFlags | log.Lshortfile)

	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))

	var provider *semrelplugin.GitHubProvider
	var err error
	if token != "" {
		provider, err = semrelplugin.NewGitHubProvider(token)
		if err != nil {
			log.Printf("Warning: %v - falling back to unauthenticated GitHub client", err)
			provider = semrelplugin.NewProvider("")
		}
	} else {
		provider = semrelplugin.NewProvider("")
	}

	grpcserver.Serve(provider)
}
