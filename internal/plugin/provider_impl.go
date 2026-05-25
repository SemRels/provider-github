// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The SemRels Authors

// Package plugin implements the ProviderPlugin for GitHub using the GitHub REST API.
package plugin

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-github/v69/github"
	semrelv1 "github.com/SemRels/semrel-api/api/gen/v1"
)

// Provider implements semrelv1.ProviderPluginServer backed by the GitHub REST API.
type Provider struct {
	semrelv1.UnimplementedProviderPluginServer

	// newClient creates a GitHub client; injectable for testing.
	newClient func(token string) *github.Client
}

// New returns a Provider that creates real GitHub API clients.
func New() *Provider {
	return &Provider{newClient: defaultClient}
}

// NewWithClient returns a Provider with an injected client factory (for tests).
func NewWithClient(factory func(string) *github.Client) *Provider {
	return &Provider{newClient: factory}
}

func defaultClient(token string) *github.Client {
	return github.NewClient(nil).WithAuthToken(token)
}

func tokenFromCtx(ctx *semrelv1.ReleaseContext) string {
	if ctx != nil {
		if t := ctx.GetConfig()["github_token"]; t != "" {
			return t
		}
	}
	if t := os.Getenv("GITHUB_TOKEN"); t != "" {
		return t
	}
	return os.Getenv("GH_TOKEN")
}

// GetLastRelease returns the latest published GitHub Release tag and its SHA.
func (p *Provider) GetLastRelease(ctx context.Context, req *semrelv1.GetLastReleaseRequest) (*semrelv1.GetLastReleaseResponse, error) {
	rctx := req.GetCtx()
	client := p.newClient(tokenFromCtx(rctx))

	release, resp, err := client.Repositories.GetLatestRelease(ctx, rctx.GetRepoOwner(), rctx.GetRepoName())
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			// No releases yet — return empty version
			return &semrelv1.GetLastReleaseResponse{}, nil
		}
		return nil, fmt.Errorf("GetLatestRelease: %w", err)
	}

	tagName := release.GetTagName()
	ver, err := parseVersion(tagName)
	if err != nil {
		return &semrelv1.GetLastReleaseResponse{}, nil
	}

	// Resolve the tag to a commit SHA
	ref, _, err := client.Git.GetRef(ctx, rctx.GetRepoOwner(), rctx.GetRepoName(), "tags/"+tagName)
	tagSHA := ""
	if err == nil {
		tagSHA = ref.GetObject().GetSHA()
	}

	return &semrelv1.GetLastReleaseResponse{
		Version: ver,
		TagSha:  tagSHA,
	}, nil
}

// GetCommitsSince returns all commits between sinceSHA and HEAD on ctx.Branch.
func (p *Provider) GetCommitsSince(ctx context.Context, req *semrelv1.GetCommitsSinceRequest) (*semrelv1.GetCommitsSinceResponse, error) {
	rctx := req.GetCtx()
	client := p.newClient(tokenFromCtx(rctx))

	opts := &github.CommitsListOptions{
		SHA: rctx.GetBranch(),
		ListOptions: github.ListOptions{PerPage: 250},
	}
	// GitHub's ListCommits supports "since" as a timestamp only, not a SHA.
	// sinceSHA filtering is handled below: we stop when we encounter the known SHA.

	var allCommits []*semrelv1.Commit
	for {
		commits, resp, err := client.Repositories.ListCommits(ctx, rctx.GetRepoOwner(), rctx.GetRepoName(), opts)
		if err != nil {
			return nil, fmt.Errorf("ListCommits: %w", err)
		}

		for _, c := range commits {
			if c.GetSHA() == req.GetSinceSha() {
				goto done
			}
			allCommits = append(allCommits, ghCommitToProto(c))
		}

		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
done:

	return &semrelv1.GetCommitsSinceResponse{Commits: allCommits}, nil
}

// CreateRelease creates a GitHub Release with the provided changelog as body.
func (p *Provider) CreateRelease(ctx context.Context, req *semrelv1.CreateReleaseRequest) (*semrelv1.CreateReleaseResponse, error) {
	rctx := req.GetCtx()
	client := p.newClient(tokenFromCtx(rctx))

	ver := rctx.GetNextVersion()
	tagName := fmt.Sprintf("v%d.%d.%d", ver.GetMajor(), ver.GetMinor(), ver.GetPatch())
	if pre := ver.GetPreRelease(); pre != "" {
		tagName += "-" + pre
	}

	releaseReq := &github.RepositoryRelease{
		TagName:         github.Ptr(tagName),
		TargetCommitish: github.Ptr(rctx.GetBranch()),
		Name:            github.Ptr(tagName),
		Body:            github.Ptr(req.GetChangelog()),
		Draft:           github.Ptr(false),
		Prerelease:      github.Ptr(ver.GetPreRelease() != ""),
	}

	if rctx.GetDryRun() {
		return &semrelv1.CreateReleaseResponse{
			ReleaseUrl: fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s (dry-run)",
				rctx.GetRepoOwner(), rctx.GetRepoName(), tagName),
			ReleaseId: "dry-run",
		}, nil
	}

	release, _, err := client.Repositories.CreateRelease(ctx, rctx.GetRepoOwner(), rctx.GetRepoName(), releaseReq)
	if err != nil {
		return nil, fmt.Errorf("CreateRelease: %w", err)
	}

	return &semrelv1.CreateReleaseResponse{
		ReleaseUrl: release.GetHTMLURL(),
		ReleaseId:  fmt.Sprintf("%d", release.GetID()),
	}, nil
}

// UploadAsset uploads a file to an existing GitHub Release.
func (p *Provider) UploadAsset(ctx context.Context, req *semrelv1.UploadAssetRequest) (*semrelv1.UploadAssetResponse, error) {
	rctx := req.GetCtx()
	client := p.newClient(tokenFromCtx(rctx))

	releaseID := int64(0)
	if _, err := fmt.Sscanf(req.GetReleaseId(), "%d", &releaseID); err != nil {
		return nil, fmt.Errorf("invalid release_id %q: %w", req.GetReleaseId(), err)
	}

	f, err := os.Open(req.GetAssetPath())
	if err != nil {
		return nil, fmt.Errorf("open asset %q: %w", req.GetAssetPath(), err)
	}
	defer func() { _ = f.Close() }()

	contentType := req.GetContentType()
	if contentType == "" {
		contentType = mime.TypeByExtension(filepath.Ext(req.GetAssetName()))
	}
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	opts := &github.UploadOptions{Name: req.GetAssetName(), MediaType: contentType}
	asset, _, err := client.Repositories.UploadReleaseAsset(ctx, rctx.GetRepoOwner(), rctx.GetRepoName(), releaseID, opts, f)
	if err != nil {
		return nil, fmt.Errorf("UploadReleaseAsset: %w", err)
	}

	return &semrelv1.UploadAssetResponse{AssetUrl: asset.GetBrowserDownloadURL()}, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func parseVersion(tag string) (*semrelv1.SemanticVersion, error) {
	tag = strings.TrimPrefix(tag, "v")
	var major, minor, patch uint32
	if _, err := fmt.Sscanf(tag, "%d.%d.%d", &major, &minor, &patch); err != nil {
		return nil, fmt.Errorf("parse tag %q: %w", tag, err)
	}
	return &semrelv1.SemanticVersion{Major: major, Minor: minor, Patch: patch}, nil
}

func ghCommitToProto(c *github.RepositoryCommit) *semrelv1.Commit {
	proto := &semrelv1.Commit{
		Sha: c.GetSHA(),
	}
	if c.Commit != nil {
		proto.RawMessage = c.Commit.GetMessage()
		if c.Commit.Author != nil {
			proto.AuthorName = c.Commit.Author.GetName()
			proto.AuthorEmail = c.Commit.Author.GetEmail()
			if c.Commit.Author.Date != nil {
				proto.Timestamp = c.Commit.Author.Date.GetTime().Unix()
			}
		}
	}
	_ = time.Now // suppress unused import if needed
	return proto
}
