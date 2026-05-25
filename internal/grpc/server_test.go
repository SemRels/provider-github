// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The provider-github Authors

package grpc

import (
	"context"
	"fmt"
	"testing"

	semrelv1 "github.com/SemRels/provider-github/internal/gen/v1"
	"github.com/stretchr/testify/require"
	grpcclient "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type mockProvider struct {
	healthCalled bool
	lastRelease  *semrelv1.GetLastReleaseResponse
}

func (m *mockProvider) Name() string {
	return "provider-github"
}

func (m *mockProvider) HealthCheck(context.Context) error {
	m.healthCalled = true
	return nil
}

func (m *mockProvider) GetLastRelease(context.Context, *semrelv1.GetLastReleaseRequest) (*semrelv1.GetLastReleaseResponse, error) {
	if m.lastRelease != nil {
		return m.lastRelease, nil
	}
	return &semrelv1.GetLastReleaseResponse{Version: &semrelv1.SemanticVersion{}}, nil
}

func (m *mockProvider) GetCommitsSince(context.Context, *semrelv1.GetCommitsSinceRequest) (*semrelv1.GetCommitsSinceResponse, error) {
	return &semrelv1.GetCommitsSinceResponse{}, nil
}

func (m *mockProvider) CreateRelease(context.Context, *semrelv1.CreateReleaseRequest) (*semrelv1.CreateReleaseResponse, error) {
	return &semrelv1.CreateReleaseResponse{}, nil
}

func (m *mockProvider) UploadAsset(context.Context, *semrelv1.UploadAssetRequest) (*semrelv1.UploadAssetResponse, error) {
	return &semrelv1.UploadAssetResponse{}, nil
}

func TestNewProviderServer(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{}
	server := NewProviderServer(provider)

	require.NotNil(t, server)
	require.Same(t, provider, server.provider)
}

func TestProviderServer_Health(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{}
	server := NewProviderServer(provider)

	resp, err := server.Health(context.Background())

	require.NoError(t, err)
	require.True(t, provider.healthCalled)
	require.Equal(t, "provider-github", resp.Name)
}

func startTestServerForTest(t *testing.T, provider *mockProvider) semrelv1.ProviderPluginClient {
	t.Helper()
	server, listener, address, err := StartTestServer(provider)
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})
	conn, err := grpcclient.NewClient(address, grpcclient.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	return semrelv1.NewProviderPluginClient(conn)
}

func TestStartTestServer_Lifecycle(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{
		lastRelease: &semrelv1.GetLastReleaseResponse{
			Version: &semrelv1.SemanticVersion{Major: 1, Minor: 2, Patch: 3},
			TagSha:  "abc123",
		},
	}
	client := startTestServerForTest(t, provider)

	resp, err := client.GetLastRelease(context.Background(), &semrelv1.GetLastReleaseRequest{Ctx: &semrelv1.ReleaseContext{}})

	require.NoError(t, err)
	require.Equal(t, uint32(1), resp.GetVersion().GetMajor())
	require.Equal(t, uint32(2), resp.GetVersion().GetMinor())
	require.Equal(t, uint32(3), resp.GetVersion().GetPatch())
	require.Equal(t, "abc123", resp.GetTagSha())
}

func TestProviderServer_GetCommitsSince(t *testing.T) {
	t.Parallel()

	client := startTestServerForTest(t, &mockProvider{})
	resp, err := client.GetCommitsSince(context.Background(), &semrelv1.GetCommitsSinceRequest{
		Ctx:      &semrelv1.ReleaseContext{},
		SinceSha: "deadbeef",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestProviderServer_CreateRelease(t *testing.T) {
	t.Parallel()

	client := startTestServerForTest(t, &mockProvider{})
	resp, err := client.CreateRelease(context.Background(), &semrelv1.CreateReleaseRequest{
		Ctx:       &semrelv1.ReleaseContext{},
		Changelog: "## Changes",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestProviderServer_UploadAsset(t *testing.T) {
	t.Parallel()

	client := startTestServerForTest(t, &mockProvider{})
	resp, err := client.UploadAsset(context.Background(), &semrelv1.UploadAssetRequest{
		Ctx:       &semrelv1.ReleaseContext{},
		ReleaseId: "123",
	})

	require.NoError(t, err)
	require.NotNil(t, resp)
}

func TestProviderGRPCPlugin_GRPCServer(t *testing.T) {
	t.Parallel()

	provider := &mockProvider{}
	p := &ProviderGRPCPlugin{Impl: provider}
	s := grpcclient.NewServer()
	err := p.GRPCServer(nil, s)

	require.NoError(t, err)
}

func TestProviderGRPCPlugin_GRPCClient(t *testing.T) {
	t.Parallel()

	// Start a real test server so we have a valid ClientConn to use.
	server, listener, address, err := StartTestServer(&mockProvider{})
	require.NoError(t, err)
	t.Cleanup(func() {
		server.Stop()
		_ = listener.Close()
	})

	conn, err := grpcclient.NewClient(address, grpcclient.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	p := &ProviderGRPCPlugin{Impl: &mockProvider{}}
	iface, err := p.GRPCClient(context.Background(), nil, conn)

	require.NoError(t, err)
	require.NotNil(t, iface)
	_, ok := iface.(semrelv1.ProviderPluginClient)
	require.True(t, ok, "expected ProviderPluginClient")
}

func TestProviderServer_Health_Error(t *testing.T) {
	t.Parallel()

	srv := NewProviderServer(&errorProvider{})
	_, err := srv.Health(context.Background())
	require.Error(t, err)
	require.Contains(t, err.Error(), "health failed")
}

// errorProvider returns errors from HealthCheck.
type errorProvider struct{ mockProvider }

func (e *errorProvider) HealthCheck(context.Context) error {
	return fmt.Errorf("health failed")
}
