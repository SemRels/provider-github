# SPDX-License-Identifier: Apache-2.0
# SPDX-FileCopyrightText: 2026 The plugin-template Authors

.PHONY: build test lint coverage release build-all-platforms clean

PLUGIN_NAME ?= plugin
DIST_DIR ?= dist

build:
	mkdir -p bin
	go build -o bin/$(PLUGIN_NAME) ./cmd/plugin

test:
	go test -v ./...

lint:
	golangci-lint run

coverage:
	go test -cover ./...

build-all-platforms:
	mkdir -p $(DIST_DIR)
	GOOS=linux GOARCH=amd64 go build -o $(DIST_DIR)/$(PLUGIN_NAME)-linux-amd64 ./cmd/plugin
	GOOS=linux GOARCH=arm64 go build -o $(DIST_DIR)/$(PLUGIN_NAME)-linux-arm64 ./cmd/plugin
	GOOS=darwin GOARCH=amd64 go build -o $(DIST_DIR)/$(PLUGIN_NAME)-darwin-amd64 ./cmd/plugin
	GOOS=darwin GOARCH=arm64 go build -o $(DIST_DIR)/$(PLUGIN_NAME)-darwin-arm64 ./cmd/plugin
	GOOS=windows GOARCH=amd64 go build -o $(DIST_DIR)/$(PLUGIN_NAME)-windows-amd64.exe ./cmd/plugin
	GOOS=windows GOARCH=arm64 go build -o $(DIST_DIR)/$(PLUGIN_NAME)-windows-arm64.exe ./cmd/plugin

release:
	@echo "Build all release artifacts locally with 'make build-all-platforms' and push a v*.*.* tag to trigger .github/workflows/release.yml"

clean:
	go clean
