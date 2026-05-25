// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The plugin-template Authors

package plugin

import "context"

// Provider defines the minimal contract a SemRel provider plugin should implement.
type Provider interface {
	Name() string
	HealthCheck(context.Context) error
}

// ProviderPlugin is a small default implementation that can be extended or replaced.
type ProviderPlugin struct {
	name string
}

func NewProvider(name string) *ProviderPlugin {
	if name == "" {
		name = "replace-me"
	}

	return &ProviderPlugin{name: name}
}

func (p *ProviderPlugin) Name() string {
	return p.name
}

func (p *ProviderPlugin) HealthCheck(context.Context) error {
	return nil
}
