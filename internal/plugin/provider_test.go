// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The provider-github Authors

package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	semrelv1 "github.com/SemRels/provider-github/internal/gen/v1"
	github "github.com/google/go-github/v68/github"
	"github.com/stretchr/testify/require"
)

func newTestGitHubProvider(t *testing.T, handler http.Handler) (*GitHubProvider, *httptest.Server) {
	t.Helper()

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	provider, err := NewGitHubProvider("test-token")
	require.NoError(t, err)

	baseURL, err := url.Parse(server.URL + "/")
	require.NoError(t, err)

	provider.client.BaseURL = baseURL
	provider.client.UploadURL = baseURL
	return provider, server
}

func testReleaseContext() *semrelv1.ReleaseContext {
	return &semrelv1.ReleaseContext{
		RepoOwner: "owner",
		RepoName:  "repo",
		Branch:    "main",
		Config:    map[string]string{"owner": "owner", "repo": "repo"},
	}
}

func TestNewGitHubProvider_EmptyToken(t *testing.T) {
	t.Parallel()

	provider, err := NewGitHubProvider("")

	require.Nil(t, provider)
	require.Error(t, err)
}

func TestNewGitHubProvider_ValidToken(t *testing.T) {
	t.Parallel()

	provider, err := NewGitHubProvider("token")

	require.NoError(t, err)
	require.NotNil(t, provider)
	require.Equal(t, "provider-github", provider.Name())
}

func TestGetLastRelease_NoReleases(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.NoError(t, json.NewEncoder(w).Encode([]*github.RepositoryRelease{}))
	})
	mux.HandleFunc("/repos/owner/repo/tags", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodGet, r.Method)
		require.NoError(t, json.NewEncoder(w).Encode([]*github.RepositoryTag{}))
	})

	provider, _ := newTestGitHubProvider(t, mux)

	resp, err := provider.GetLastRelease(context.Background(), &semrelv1.GetLastReleaseRequest{Ctx: testReleaseContext()})

	require.NoError(t, err)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Version)
	require.Equal(t, uint32(0), resp.Version.GetMajor())
	require.Empty(t, resp.GetTagSha())
}

func TestGetLastRelease_WithRelease(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode([]*github.RepositoryRelease{
			{TagName: github.Ptr("v1.2.3"), TargetCommitish: github.Ptr("sha123")},
			{TagName: github.Ptr("v1.10.0"), TargetCommitish: github.Ptr("sha999")},
			{TagName: github.Ptr("not-semver"), TargetCommitish: github.Ptr("ignored")},
		}))
	})
	mux.HandleFunc("/repos/owner/repo/tags", func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("tags endpoint should not be called when releases contain semver tags")
	})
	mux.HandleFunc("/repos/owner/repo/git/ref/tags/v1.10.0", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"not found"}`))
	})

	provider, _ := newTestGitHubProvider(t, mux)

	resp, err := provider.GetLastRelease(context.Background(), &semrelv1.GetLastReleaseRequest{Ctx: testReleaseContext()})

	require.NoError(t, err)
	require.NotNil(t, resp.Version)
	require.Equal(t, uint32(1), resp.Version.GetMajor())
	require.Equal(t, uint32(10), resp.Version.GetMinor())
	require.Equal(t, uint32(0), resp.Version.GetPatch())
	require.Equal(t, "sha999", resp.GetTagSha())
}

func TestGetLastRelease_WithTagsOnly(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode([]*github.RepositoryRelease{}))
	})
	mux.HandleFunc("/repos/owner/repo/tags", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode([]*github.RepositoryTag{
			{Name: github.Ptr("v1.0.0"), Commit: &github.Commit{SHA: github.Ptr("sha100")}},
			{Name: github.Ptr("v2.1.0"), Commit: &github.Commit{SHA: github.Ptr("sha210")}},
		}))
	})

	provider, _ := newTestGitHubProvider(t, mux)

	resp, err := provider.GetLastRelease(context.Background(), &semrelv1.GetLastReleaseRequest{Ctx: testReleaseContext()})

	require.NoError(t, err)
	require.Equal(t, uint32(2), resp.Version.GetMajor())
	require.Equal(t, uint32(1), resp.Version.GetMinor())
	require.Equal(t, "sha210", resp.GetTagSha())
}

func TestGetLastRelease_APIError(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	})

	provider, _ := newTestGitHubProvider(t, mux)

	resp, err := provider.GetLastRelease(context.Background(), &semrelv1.GetLastReleaseRequest{Ctx: testReleaseContext()})

	require.Nil(t, resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "list releases")
}

func TestGetCommitsSince_WithSHA(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/compare/base123...main", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(&github.CommitsComparison{
			Commits: []*github.RepositoryCommit{{
				SHA: github.Ptr("commit123"),
				Commit: &github.Commit{
					Message: github.Ptr("feat: add thing"),
					Author: &github.CommitAuthor{
						Name:  github.Ptr("Ada"),
						Email: github.Ptr("ada@example.com"),
						Date:  &github.Timestamp{Time: time.Unix(1700000000, 0)},
					},
				},
			}},
		}))
	})
	mux.HandleFunc("/repos/owner/repo/commits/commit123", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(&github.RepositoryCommit{
			SHA: github.Ptr("commit123"),
			Commit: &github.Commit{
				Message: github.Ptr("feat: add thing"),
				Author: &github.CommitAuthor{
					Name:  github.Ptr("Ada"),
					Email: github.Ptr("ada@example.com"),
					Date:  &github.Timestamp{Time: time.Unix(1700000000, 0)},
				},
			},
			Files: []*github.CommitFile{{Filename: github.Ptr("main.go")}, {Filename: github.Ptr("go.mod")}},
		}))
	})

	provider, _ := newTestGitHubProvider(t, mux)

	resp, err := provider.GetCommitsSince(context.Background(), &semrelv1.GetCommitsSinceRequest{
		Ctx:      testReleaseContext(),
		SinceSha: "base123",
	})

	require.NoError(t, err)
	require.Len(t, resp.GetCommits(), 1)
	commit := resp.GetCommits()[0]
	require.Equal(t, "commit123", commit.GetSha())
	require.Equal(t, "feat: add thing", commit.GetRawMessage())
	require.Equal(t, []string{"main.go", "go.mod"}, commit.GetFilesChanged())
	require.Equal(t, "Ada", commit.GetAuthorName())
}

func TestGetCommitsSince_NoSHA(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/commits", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode([]*github.RepositoryCommit{{
			SHA:    github.Ptr("head123"),
			Commit: &github.Commit{Message: github.Ptr("fix: repair")},
		}}))
	})
	mux.HandleFunc("/repos/owner/repo/commits/head123", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(&github.RepositoryCommit{
			SHA: github.Ptr("head123"),
			Commit: &github.Commit{
				Message: github.Ptr("fix: repair"),
				Author: &github.CommitAuthor{
					Name:  github.Ptr("Bob"),
					Email: github.Ptr("bob@example.com"),
					Date:  &github.Timestamp{Time: time.Unix(1700001000, 0)},
				},
			},
			Files: []*github.CommitFile{{Filename: github.Ptr("internal/plugin/provider.go")}},
		}))
	})

	provider, _ := newTestGitHubProvider(t, mux)

	resp, err := provider.GetCommitsSince(context.Background(), &semrelv1.GetCommitsSinceRequest{Ctx: testReleaseContext()})

	require.NoError(t, err)
	require.Len(t, resp.GetCommits(), 1)
	require.Equal(t, "head123", resp.GetCommits()[0].GetSha())
}

func TestCreateRelease_DryRun(t *testing.T) {
	t.Parallel()

	called := false
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		called = true
		t.Fatalf("create release endpoint should not be called in dry-run mode")
	})

	provider, _ := newTestGitHubProvider(t, mux)
	ctx := testReleaseContext()
	ctx.DryRun = true
	ctx.NextVersion = &semrelv1.SemanticVersion{Major: 1, Minor: 2, Patch: 3}

	resp, err := provider.CreateRelease(context.Background(), &semrelv1.CreateReleaseRequest{Ctx: ctx, Changelog: "notes"})

	require.NoError(t, err)
	require.False(t, called)
	require.Contains(t, resp.GetReleaseUrl(), "dry-run")
	require.Equal(t, "dry-run", resp.GetReleaseId())
}

func TestCreateRelease_Success(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		var release github.RepositoryRelease
		require.NoError(t, json.NewDecoder(r.Body).Decode(&release))
		require.Equal(t, "v1.2.3", release.GetTagName())
		require.Equal(t, "v1.2.3", release.GetName())
		require.NoError(t, json.NewEncoder(w).Encode(&github.RepositoryRelease{
			ID:      github.Ptr(int64(42)),
			HTMLURL: github.Ptr("https://github.com/owner/repo/releases/tag/v1.2.3"),
		}))
	})

	provider, _ := newTestGitHubProvider(t, mux)
	ctx := testReleaseContext()
	ctx.NextVersion = &semrelv1.SemanticVersion{Major: 1, Minor: 2, Patch: 3}

	resp, err := provider.CreateRelease(context.Background(), &semrelv1.CreateReleaseRequest{Ctx: ctx, Changelog: "notes"})

	require.NoError(t, err)
	require.Equal(t, "42", resp.GetReleaseId())
	require.Equal(t, "https://github.com/owner/repo/releases/tag/v1.2.3", resp.GetReleaseUrl())
}

func TestCreateRelease_RateLimit(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-RateLimit-Remaining", "0")
		w.Header().Set("X-RateLimit-Reset", fmt.Sprintf("%d", time.Now().Add(time.Minute).Unix()))
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"API rate limit exceeded"}`))
	})

	provider, _ := newTestGitHubProvider(t, mux)
	ctx := testReleaseContext()
	ctx.NextVersion = &semrelv1.SemanticVersion{Major: 1, Minor: 0, Patch: 0}

	resp, err := provider.CreateRelease(context.Background(), &semrelv1.CreateReleaseRequest{Ctx: ctx, Changelog: "notes"})

	require.Nil(t, resp)
	require.Error(t, err)
	require.Contains(t, strings.ToLower(err.Error()), "rate limit")
}

func TestUploadAsset_Success(t *testing.T) {
	t.Parallel()

	tempFile, err := os.CreateTemp("", "provider-github-asset-*.txt")
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = os.Remove(tempFile.Name())
	})
	_, err = tempFile.WriteString("artifact")
	require.NoError(t, err)
	require.NoError(t, tempFile.Close())

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/releases/99/assets", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, http.MethodPost, r.Method)
		require.Equal(t, "binary.txt", r.URL.Query().Get("name"))
		require.NoError(t, json.NewEncoder(w).Encode(&github.ReleaseAsset{
			BrowserDownloadURL: github.Ptr("https://github.com/owner/repo/releases/download/v1.2.3/binary.txt"),
		}))
	})

	provider, _ := newTestGitHubProvider(t, mux)

	resp, err := provider.UploadAsset(context.Background(), &semrelv1.UploadAssetRequest{
		Ctx:         testReleaseContext(),
		ReleaseId:   "99",
		AssetPath:   tempFile.Name(),
		AssetName:   "binary.txt",
		ContentType: "text/plain",
	})

	require.NoError(t, err)
	require.Equal(t, "https://github.com/owner/repo/releases/download/v1.2.3/binary.txt", resp.GetAssetUrl())
}

func TestTagToVersion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		tag         string
		wantVersion *semrelv1.SemanticVersion
		wantErr     bool
	}{
		{
			name:        "prefixed",
			tag:         "v1.2.3",
			wantVersion: &semrelv1.SemanticVersion{Major: 1, Minor: 2, Patch: 3},
		},
		{
			name:        "unprefixed",
			tag:         "1.2.3",
			wantVersion: &semrelv1.SemanticVersion{Major: 1, Minor: 2, Patch: 3},
		},
		{
			name:        "prerelease",
			tag:         "v2.0.0-alpha.1",
			wantVersion: &semrelv1.SemanticVersion{Major: 2, Minor: 0, Patch: 0, PreRelease: "alpha.1"},
		},
		{
			name:    "invalid",
			tag:     "main",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			version, err := tagToVersion(tt.tag)

			if tt.wantErr {
				require.Error(t, err)
				return
			}

			require.NoError(t, err)
			require.Equal(t, tt.wantVersion, version)
		})
	}
}

func TestVersionToTag(t *testing.T) {
	t.Parallel()

	require.Equal(t, "v1.2.3", versionToTag(&semrelv1.SemanticVersion{Major: 1, Minor: 2, Patch: 3}))
	require.Equal(t, "v2.0.0-alpha.1", versionToTag(&semrelv1.SemanticVersion{Major: 2, Patch: 0, PreRelease: "alpha.1"}))
	require.Equal(t, "v3.1.4+build.5", versionToTag(&semrelv1.SemanticVersion{Major: 3, Minor: 1, Patch: 4, BuildMetadata: "build.5"}))
}

func TestNewProvider_UsesEnvToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "env-token")

	provider := NewProvider("")

	require.NotNil(t, provider)
	require.Equal(t, "provider-github", provider.Name())
	require.Equal(t, "env-token", provider.token)
}

func TestHealthCheck_NoToken(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "")

	provider := NewProvider("")
	provider.token = ""

	require.NoError(t, provider.HealthCheck(context.Background()))
}

func TestHealthCheck_Success(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/rate_limit", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(map[string]any{
			"resources": map[string]any{
				"core": map[string]any{
					"limit":     5000,
					"remaining": 4999,
					"reset":     time.Now().Add(time.Minute).Unix(),
				},
			},
			"rate": map[string]any{
				"limit":     5000,
				"remaining": 4999,
				"reset":     time.Now().Add(time.Minute).Unix(),
			},
		}))
	})

	provider, _ := newTestGitHubProvider(t, mux)

	require.NoError(t, provider.HealthCheck(context.Background()))
}

func TestGetCommitsSince_DerivesSHAFromLastVersion(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/git/ref/tags/v1.2.3", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(&github.Reference{
			Object: &github.GitObject{Type: github.Ptr("commit"), SHA: github.Ptr("base123")},
		}))
	})
	mux.HandleFunc("/repos/owner/repo/compare/base123...main", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(&github.CommitsComparison{
			Commits: []*github.RepositoryCommit{{
				SHA:    github.Ptr("derived123"),
				Commit: &github.Commit{Message: github.Ptr("feat: derived")},
			}},
		}))
	})
	mux.HandleFunc("/repos/owner/repo/commits/derived123", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(&github.RepositoryCommit{
			SHA: github.Ptr("derived123"),
			Commit: &github.Commit{
				Message: github.Ptr("feat: derived"),
				Author:  &github.CommitAuthor{Name: github.Ptr("Derive")},
			},
			Files: []*github.CommitFile{{Filename: github.Ptr("derived.go")}},
		}))
	})

	provider, _ := newTestGitHubProvider(t, mux)
	ctx := testReleaseContext()
	ctx.LastVersion = &semrelv1.SemanticVersion{Major: 1, Minor: 2, Patch: 3}

	resp, err := provider.GetCommitsSince(context.Background(), &semrelv1.GetCommitsSinceRequest{Ctx: ctx})

	require.NoError(t, err)
	require.Len(t, resp.GetCommits(), 1)
	require.Equal(t, "derived123", resp.GetCommits()[0].GetSha())
}

func TestUploadAsset_InvalidReleaseID(t *testing.T) {
	t.Parallel()

	provider := NewProvider("")

	resp, err := provider.UploadAsset(context.Background(), &semrelv1.UploadAssetRequest{
		Ctx:       testReleaseContext(),
		ReleaseId: "not-a-number",
	})

	require.Nil(t, resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "parse release id")
}

func TestResolveTagSHA_AnnotatedTag(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/repos/owner/repo/git/ref/tags/v1.2.3", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(&github.Reference{
			Object: &github.GitObject{Type: github.Ptr("tag"), SHA: github.Ptr("tagsha")},
		}))
	})
	mux.HandleFunc("/repos/owner/repo/git/tags/tagsha", func(w http.ResponseWriter, r *http.Request) {
		require.NoError(t, json.NewEncoder(w).Encode(&github.Tag{
			Object: &github.GitObject{Type: github.Ptr("commit"), SHA: github.Ptr("commitsha")},
		}))
	})

	provider, _ := newTestGitHubProvider(t, mux)

	sha, err := provider.resolveTagSHA(context.Background(), provider.client, "owner", "repo", "v1.2.3", "")

	require.NoError(t, err)
	require.Equal(t, "commitsha", sha)
}

func TestRepoDetails(t *testing.T) {
	t.Parallel()

	owner, repo, err := repoDetails(&semrelv1.ReleaseContext{Config: map[string]string{"owner": "cfg-owner", "repository": "cfg-repo"}})
	require.NoError(t, err)
	require.Equal(t, "cfg-owner", owner)
	require.Equal(t, "cfg-repo", repo)

	_, _, err = repoDetails(&semrelv1.ReleaseContext{})
	require.Error(t, err)
}

func TestClientForContext_APIURL(t *testing.T) {
	t.Parallel()

	provider := NewProvider("")
	client := provider.clientForContext(&semrelv1.ReleaseContext{Config: map[string]string{"api_url": "https://ghe.example/api/v3"}})

	require.Equal(t, "https://ghe.example/api/v3/", client.BaseURL.String())
	require.Equal(t, "https://ghe.example/api/v3/", client.UploadURL.String())
}

func TestParseRateLimit_Abuse(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	require.NoError(t, err)
	retryAfter := 30 * time.Second

	wrapped := parseRateLimit(&github.AbuseRateLimitError{
		Response:   &http.Response{StatusCode: http.StatusForbidden, Request: req},
		Message:    "slow down",
		RetryAfter: &retryAfter,
	})

	require.Error(t, wrapped)
	require.Contains(t, wrapped.Error(), "retry after")
}

func TestComparePreReleaseAndHelpers(t *testing.T) {
	t.Parallel()

	require.Equal(t, 1, comparePreRelease("", "alpha.1"))
	require.Equal(t, -1, comparePreRelease("alpha.1", "alpha.2"))
	require.Equal(t, 1, comparePreRelease("beta", "alpha"))
	require.Equal(t, -1, comparePreRelease("1", "alpha"))
	require.Equal(t, 0, compareVersions(&semrelv1.SemanticVersion{Major: 1, Minor: 0, Patch: 0}, &semrelv1.SemanticVersion{Major: 1, Minor: 0, Patch: 0}))
	require.Equal(t, -1, compareUint32(1, 2))
	require.Equal(t, 1, compareUint32(3, 2))
	require.Equal(t, "https://example.com/api/", ensureTrailingSlash("https://example.com/api"))
	require.Equal(t, "https://example.com/api/", ensureTrailingSlash("https://example.com/api/"))
}

func TestCreateRelease_RequiresNextVersion(t *testing.T) {
	t.Parallel()

	provider := NewProvider("")
	resp, err := provider.CreateRelease(context.Background(), &semrelv1.CreateReleaseRequest{Ctx: testReleaseContext()})

	require.Nil(t, resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "next version")
}

func TestUploadAsset_OpenError(t *testing.T) {
	t.Parallel()

	provider := NewProvider("")
	resp, err := provider.UploadAsset(context.Background(), &semrelv1.UploadAssetRequest{
		Ctx:       testReleaseContext(),
		ReleaseId: "99",
		AssetPath: "missing-file.txt",
	})

	require.Nil(t, resp)
	require.Error(t, err)
	require.Contains(t, err.Error(), "open asset")
}

func TestClientForContext_InvalidAPIURL(t *testing.T) {
	t.Parallel()

	provider := NewProvider("")
	client := provider.clientForContext(&semrelv1.ReleaseContext{Config: map[string]string{"api_url": "://bad"}})

	require.Same(t, provider.client, client)
}

func TestParseRateLimit_RateLimit(t *testing.T) {
	t.Parallel()

	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	require.NoError(t, err)

	wrapped := parseRateLimit(&github.RateLimitError{
		Response: &http.Response{StatusCode: http.StatusForbidden, Request: req},
		Message:  "rate limit exceeded",
	})

	require.Error(t, wrapped)
	require.Contains(t, strings.ToLower(wrapped.Error()), "rate limit")
}
