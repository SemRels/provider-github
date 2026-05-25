// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The SemRels Authors

package plugin

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/go-github/v69/github"
	semrelv1 "github.com/SemRels/semrel-api/api/gen/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"net/http/httptest"
	"encoding/json"
)

func newTestServer(t *testing.T, mux *http.ServeMux) (*httptest.Server, *github.Client) {
	t.Helper()
	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	client, err := github.NewClient(nil).WithEnterpriseURLs(ts.URL+"/", ts.URL+"/")
	require.NoError(t, err)
	return ts, client
}

func testProvider(client *github.Client) *Provider {
	return NewWithClient(func(_ string) *github.Client { return client })
}

func ctx() *semrelv1.ReleaseContext {
	return &semrelv1.ReleaseContext{
		RepoOwner: "SemRels",
		RepoName:  "myrepo",
		Branch:    "main",
	}
}

func TestGetLastRelease_NoRelease(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/SemRels/myrepo/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		fmt.Fprint(w, `{"message":"Not Found"}`) //nolint:errcheck
	})

	_, client := newTestServer(t, mux)
	p := testProvider(client)

	resp, err := p.GetLastRelease(t.Context(), &semrelv1.GetLastReleaseRequest{Ctx: ctx()})
	require.NoError(t, err)
	assert.Nil(t, resp.GetVersion())
}

func TestGetLastRelease_WithRelease(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/repos/SemRels/myrepo/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"tag_name": "v1.2.3",
			"html_url": "https://github.com/SemRels/myrepo/releases/tag/v1.2.3",
		})
	})
	mux.HandleFunc("/api/v3/repos/SemRels/myrepo/git/ref/tags/v1.2.3", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"ref": "refs/tags/v1.2.3",
			"object": map[string]string{"sha": "abc123", "type": "commit"},
		})
	})

	_, client := newTestServer(t, mux)
	p := testProvider(client)

	resp, err := p.GetLastRelease(t.Context(), &semrelv1.GetLastReleaseRequest{Ctx: ctx()})
	require.NoError(t, err)
	require.NotNil(t, resp.GetVersion())
	assert.Equal(t, uint32(1), resp.GetVersion().GetMajor())
	assert.Equal(t, uint32(2), resp.GetVersion().GetMinor())
	assert.Equal(t, uint32(3), resp.GetVersion().GetPatch())
	assert.Equal(t, "abc123", resp.GetTagSha())
}

func TestCreateRelease_DryRun(t *testing.T) {
	t.Parallel()

	p := NewWithClient(func(_ string) *github.Client { return github.NewClient(nil) })

	rctx := ctx()
	rctx.DryRun = true
	rctx.NextVersion = &semrelv1.SemanticVersion{Major: 2, Minor: 0, Patch: 0}

	resp, err := p.CreateRelease(t.Context(), &semrelv1.CreateReleaseRequest{
		Ctx:       rctx,
		Changelog: "## Changes\n- feat: something",
	})
	require.NoError(t, err)
	assert.Contains(t, resp.GetReleaseUrl(), "dry-run")
	assert.Equal(t, "dry-run", resp.GetReleaseId())
}

func TestParseVersion(t *testing.T) {
	t.Parallel()

	cases := []struct {
		tag   string
		major uint32
		minor uint32
		patch uint32
	}{
		{"v1.2.3", 1, 2, 3},
		{"1.0.0", 1, 0, 0},
		{"v0.10.99", 0, 10, 99},
	}

	for _, tc := range cases {
		t.Run(tc.tag, func(t *testing.T) {
			t.Parallel()
			ver, err := parseVersion(tc.tag)
			require.NoError(t, err)
			assert.Equal(t, tc.major, ver.GetMajor())
			assert.Equal(t, tc.minor, ver.GetMinor())
			assert.Equal(t, tc.patch, ver.GetPatch())
		})
	}
}
