// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The provider-github Authors

package grpc

import (
	"context"
	"fmt"
	"net"
	"os"

	semrelv1 "github.com/SemRels/provider-github/internal/gen/v1"
	semrelplugin "github.com/SemRels/provider-github/internal/plugin"
	hclog "github.com/hashicorp/go-hclog"
	goplugin "github.com/hashicorp/go-plugin"
	"google.golang.org/grpc"
)

// HandshakeConfig is the go-plugin handshake. Host must match these values.
var HandshakeConfig = goplugin.HandshakeConfig{
	ProtocolVersion:  1,
	MagicCookieKey:   "SEMREL_PLUGIN",
	MagicCookieValue: "provider",
}

// ProviderGRPCPlugin implements goplugin.GRPCPlugin for the ProviderPlugin service.
type ProviderGRPCPlugin struct {
	goplugin.NetRPCUnsupportedPlugin
	Impl semrelplugin.Provider
}

func (p *ProviderGRPCPlugin) GRPCServer(broker *goplugin.GRPCBroker, s *grpc.Server) error {
	semrelv1.RegisterProviderPluginServer(s, &ProviderServer{provider: p.Impl})
	return nil
}

func (p *ProviderGRPCPlugin) GRPCClient(ctx context.Context, broker *goplugin.GRPCBroker, c *grpc.ClientConn) (interface{}, error) {
	return semrelv1.NewProviderPluginClient(c), nil
}

// ProviderServer adapts plugin.Provider to the gRPC ProviderPluginServer interface.
type ProviderServer struct {
	semrelv1.UnimplementedProviderPluginServer
	provider semrelplugin.Provider
}

func NewProviderServer(provider semrelplugin.Provider) *ProviderServer {
	return &ProviderServer{provider: provider}
}

// GetLastRelease delegates to the provider.
func (s *ProviderServer) GetLastRelease(ctx context.Context, req *semrelv1.GetLastReleaseRequest) (*semrelv1.GetLastReleaseResponse, error) {
	return s.provider.GetLastRelease(ctx, req)
}

// GetCommitsSince delegates to the provider.
func (s *ProviderServer) GetCommitsSince(ctx context.Context, req *semrelv1.GetCommitsSinceRequest) (*semrelv1.GetCommitsSinceResponse, error) {
	return s.provider.GetCommitsSince(ctx, req)
}

// CreateRelease delegates to the provider.
func (s *ProviderServer) CreateRelease(ctx context.Context, req *semrelv1.CreateReleaseRequest) (*semrelv1.CreateReleaseResponse, error) {
	return s.provider.CreateRelease(ctx, req)
}

// UploadAsset delegates to the provider.
func (s *ProviderServer) UploadAsset(ctx context.Context, req *semrelv1.UploadAssetRequest) (*semrelv1.UploadAssetResponse, error) {
	return s.provider.UploadAsset(ctx, req)
}

// Health is a convenience method for local testing (not an RPC).
func (s *ProviderServer) Health(ctx context.Context) (*HealthResponse, error) {
	if err := s.provider.HealthCheck(ctx); err != nil {
		return nil, err
	}
	return &HealthResponse{Name: s.provider.Name()}, nil
}

// HealthResponse for smoke testing.
type HealthResponse struct {
	Name string
}

// Serve starts the plugin server using go-plugin. This is the real gRPC server entry point.
func Serve(provider semrelplugin.Provider) {
	logger := hclog.New(&hclog.LoggerOptions{
		Name:   "provider-github",
		Output: os.Stderr,
		Level:  hclog.Debug,
	})

	goplugin.Serve(&goplugin.ServeConfig{
		HandshakeConfig: HandshakeConfig,
		Plugins: goplugin.PluginSet{
			"provider": &ProviderGRPCPlugin{Impl: provider},
		},
		GRPCServer: goplugin.DefaultGRPCServer,
		Logger:     logger,
	})
}

// StartTestServer starts a real gRPC server on a random port for integration testing.
// Returns the server, listener, and address. Caller is responsible for stopping.
func StartTestServer(provider semrelplugin.Provider) (*grpc.Server, net.Listener, string, error) {
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, nil, "", fmt.Errorf("listen: %w", err)
	}
	server := grpc.NewServer()
	semrelv1.RegisterProviderPluginServer(server, &ProviderServer{provider: provider})
	go func() {
		_ = server.Serve(lis)
	}()
	return server, lis, lis.Addr().String(), nil
}
