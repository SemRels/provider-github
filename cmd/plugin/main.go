// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The plugin-template Authors

package main

import (
	"context"
	"log"
	"os"

	grpcserver "github.com/SemRels/plugin-template/internal/grpc"
	semrelplugin "github.com/SemRels/plugin-template/internal/plugin"
)

func main() {
	provider := semrelplugin.NewProvider("replace-me")
	server := grpcserver.NewProviderServer(provider)

	if _, err := server.Health(context.Background()); err != nil {
		log.Printf("plugin health check failed: %v", err)
		os.Exit(1)
	}

	log.Printf("%s plugin template is ready", provider.Name())
}
