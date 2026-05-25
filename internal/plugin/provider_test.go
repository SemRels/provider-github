// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The plugin-template Authors

package plugin

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestNewProviderDefaultsName(t *testing.T) {
	t.Parallel()

	provider := NewProvider("")

	require.Equal(t, "replace-me", provider.Name())
	require.NoError(t, provider.HealthCheck(context.Background()))
}

func TestNewProviderUsesProvidedName(t *testing.T) {
	t.Parallel()

	provider := NewProvider("provider-example")

	require.Equal(t, "provider-example", provider.Name())
}
