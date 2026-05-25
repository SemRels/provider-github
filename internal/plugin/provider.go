// SPDX-License-Identifier: Apache-2.0
// SPDX-FileCopyrightText: 2026 The provider-github Authors

package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	semrelv1 "github.com/SemRels/provider-github/internal/gen/v1"
	github "github.com/google/go-github/v68/github"
	hclog "github.com/hashicorp/go-hclog"
	"golang.org/x/oauth2"
)

var semverTagPattern = regexp.MustCompile(`^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+([0-9A-Za-z.-]+))?$`)

// Provider defines the SemRel GitHub provider contract.
type Provider interface {
	Name() string
	HealthCheck(context.Context) error
	GetLastRelease(context.Context, *semrelv1.GetLastReleaseRequest) (*semrelv1.GetLastReleaseResponse, error)
	GetCommitsSince(context.Context, *semrelv1.GetCommitsSinceRequest) (*semrelv1.GetCommitsSinceResponse, error)
	CreateRelease(context.Context, *semrelv1.CreateReleaseRequest) (*semrelv1.CreateReleaseResponse, error)
	UploadAsset(context.Context, *semrelv1.UploadAssetRequest) (*semrelv1.UploadAssetResponse, error)
}

// GitHubProvider implements the SemRel provider contract against the GitHub API.
type GitHubProvider struct {
	client *github.Client
	logger hclog.Logger
	token  string
}

func newLogger() hclog.Logger {
	return hclog.New(&hclog.LoggerOptions{
		Name:   "provider-github",
		Output: os.Stderr,
		Level:  hclog.Debug,
	})
}

// NewGitHubProvider creates an authenticated GitHub provider.
func NewGitHubProvider(token string) (*GitHubProvider, error) {
	if strings.TrimSpace(token) == "" {
		return nil, fmt.Errorf("github token is required")
	}

	tokenSource := oauth2.StaticTokenSource(&oauth2.Token{AccessToken: token})
	httpClient := oauth2.NewClient(context.Background(), tokenSource)

	return &GitHubProvider{
		client: github.NewClient(httpClient),
		logger: newLogger(),
		token:  token,
	}, nil
}

// NewProvider preserves the legacy constructor signature.
func NewProvider(name string) *GitHubProvider {
	_ = name

	token := strings.TrimSpace(os.Getenv("GITHUB_TOKEN"))
	logger := newLogger()

	if token != "" {
		provider, err := NewGitHubProvider(token)
		if err == nil {
			return provider
		}
		logger.Warn("failed to create authenticated provider", "error", err)
	}

	return &GitHubProvider{
		client: github.NewClient(nil),
		logger: logger,
		token:  token,
	}
}

func (p *GitHubProvider) Name() string {
	return "provider-github"
}

func (p *GitHubProvider) HealthCheck(ctx context.Context) error {
	if strings.TrimSpace(p.token) == "" {
		p.logger.Warn("GITHUB_TOKEN not configured; skipping authenticated GitHub health check")
		return nil
	}

	client := p.clientForContext(nil)
	if _, _, err := client.RateLimit.Get(ctx); err != nil {
		if rateErr := parseRateLimit(err); rateErr != nil {
			return rateErr
		}
		return fmt.Errorf("github health check failed: %w", err)
	}

	return nil
}

func (p *GitHubProvider) GetLastRelease(ctx context.Context, req *semrelv1.GetLastReleaseRequest) (*semrelv1.GetLastReleaseResponse, error) {
	releaseCtx := req.GetCtx()
	owner, repo, err := repoDetails(releaseCtx)
	if err != nil {
		return nil, err
	}

	client := p.clientForContext(releaseCtx)

	var bestVersion *semrelv1.SemanticVersion
	var bestTag string
	var fallbackSHA string

	releases, err := p.listAllReleases(ctx, client, owner, repo)
	if err != nil {
		return nil, err
	}

	for _, release := range releases {
		version, parseErr := tagToVersion(release.GetTagName())
		if parseErr != nil {
			continue
		}
		if bestVersion == nil || compareVersions(version, bestVersion) > 0 {
			bestVersion = version
			bestTag = release.GetTagName()
			fallbackSHA = release.GetTargetCommitish()
		}
	}

	if bestVersion != nil {
		tagSHA, err := p.resolveTagSHA(ctx, client, owner, repo, bestTag, fallbackSHA)
		if err != nil {
			return nil, err
		}
		return &semrelv1.GetLastReleaseResponse{Version: bestVersion, TagSha: tagSHA}, nil
	}

	tags, err := p.listAllTags(ctx, client, owner, repo)
	if err != nil {
		return nil, err
	}

	for _, tag := range tags {
		version, parseErr := tagToVersion(tag.GetName())
		if parseErr != nil {
			continue
		}
		if bestVersion == nil || compareVersions(version, bestVersion) > 0 {
			bestVersion = version
			fallbackSHA = tag.GetCommit().GetSHA()
		}
	}

	if bestVersion == nil {
		return &semrelv1.GetLastReleaseResponse{
			Version: &semrelv1.SemanticVersion{},
			TagSha:  "",
		}, nil
	}

	return &semrelv1.GetLastReleaseResponse{Version: bestVersion, TagSha: fallbackSHA}, nil
}

func (p *GitHubProvider) GetCommitsSince(ctx context.Context, req *semrelv1.GetCommitsSinceRequest) (*semrelv1.GetCommitsSinceResponse, error) {
	releaseCtx := req.GetCtx()
	owner, repo, err := repoDetails(releaseCtx)
	if err != nil {
		return nil, err
	}

	client := p.clientForContext(releaseCtx)
	sinceSHA := strings.TrimSpace(req.GetSinceSha())
	if sinceSHA == "" && !isZeroVersion(releaseCtx.GetLastVersion()) {
		tag := versionToTag(releaseCtx.GetLastVersion())
		sinceSHA, err = p.resolveTagSHA(ctx, client, owner, repo, tag, "")
		if err != nil {
			return nil, fmt.Errorf("resolve last version tag %q: %w", tag, err)
		}
	}

	if sinceSHA == "" {
		commits, err := p.listRecentCommits(ctx, client, owner, repo)
		if err != nil {
			return nil, err
		}
		return &semrelv1.GetCommitsSinceResponse{Commits: commits}, nil
	}

	head := strings.TrimSpace(releaseCtx.GetBranch())
	if head == "" {
		head = "HEAD"
	}

	comparison, _, err := client.Repositories.CompareCommits(ctx, owner, repo, sinceSHA, head, &github.ListOptions{PerPage: 100})
	if err != nil {
		if rateErr := parseRateLimit(err); rateErr != nil {
			return nil, rateErr
		}
		return nil, fmt.Errorf("compare commits %s...%s: %w", sinceSHA, head, err)
	}

	commits := make([]*semrelv1.Commit, 0, len(comparison.Commits))
	for _, repositoryCommit := range comparison.Commits {
		commit, err := p.commitToProto(ctx, client, owner, repo, repositoryCommit)
		if err != nil {
			return nil, err
		}
		commits = append(commits, commit)
	}

	return &semrelv1.GetCommitsSinceResponse{Commits: commits}, nil
}

func (p *GitHubProvider) CreateRelease(ctx context.Context, req *semrelv1.CreateReleaseRequest) (*semrelv1.CreateReleaseResponse, error) {
	releaseCtx := req.GetCtx()
	owner, repo, err := repoDetails(releaseCtx)
	if err != nil {
		return nil, err
	}
	if releaseCtx.GetNextVersion() == nil {
		return nil, fmt.Errorf("next version is required")
	}

	tag := versionToTag(releaseCtx.GetNextVersion())
	releaseURL := fmt.Sprintf("https://github.com/%s/%s/releases/tag/%s", owner, repo, tag)
	if releaseCtx.GetDryRun() {
		return &semrelv1.CreateReleaseResponse{
			ReleaseUrl: releaseURL + " (dry-run)",
			ReleaseId:  "dry-run",
		}, nil
	}

	client := p.clientForContext(releaseCtx)
	title := tag
	draft := false
	prerelease := releaseCtx.GetNextVersion().GetPreRelease() != ""

	release := &github.RepositoryRelease{
		TagName:    github.Ptr(tag),
		Name:       github.Ptr(title),
		Body:       github.Ptr(req.GetChangelog()),
		Draft:      github.Ptr(draft),
		Prerelease: github.Ptr(prerelease),
	}
	if branch := strings.TrimSpace(releaseCtx.GetBranch()); branch != "" {
		release.TargetCommitish = github.Ptr(branch)
	}

	createdRelease, _, err := client.Repositories.CreateRelease(ctx, owner, repo, release)
	if err != nil {
		if rateErr := parseRateLimit(err); rateErr != nil {
			return nil, rateErr
		}
		return nil, fmt.Errorf("create release %q: %w", tag, err)
	}

	return &semrelv1.CreateReleaseResponse{
		ReleaseUrl: createdRelease.GetHTMLURL(),
		ReleaseId:  strconv.FormatInt(createdRelease.GetID(), 10),
	}, nil
}

func (p *GitHubProvider) UploadAsset(ctx context.Context, req *semrelv1.UploadAssetRequest) (*semrelv1.UploadAssetResponse, error) {
	releaseCtx := req.GetCtx()
	owner, repo, err := repoDetails(releaseCtx)
	if err != nil {
		return nil, err
	}

	releaseID, err := strconv.ParseInt(strings.TrimSpace(req.GetReleaseId()), 10, 64)
	if err != nil {
		return nil, fmt.Errorf("parse release id %q: %w", req.GetReleaseId(), err)
	}

	file, err := os.Open(req.GetAssetPath())
	if err != nil {
		return nil, fmt.Errorf("open asset %q: %w", req.GetAssetPath(), err)
	}
	defer func() { _ = file.Close() }()

	assetName := strings.TrimSpace(req.GetAssetName())
	if assetName == "" {
		assetName = filepath.Base(req.GetAssetPath())
	}

	client := p.clientForContext(releaseCtx)
	asset, _, err := client.Repositories.UploadReleaseAsset(ctx, owner, repo, releaseID, &github.UploadOptions{
		Name:      assetName,
		Label:     assetName,
		MediaType: req.GetContentType(),
	}, file)
	if err != nil {
		if rateErr := parseRateLimit(err); rateErr != nil {
			return nil, rateErr
		}
		return nil, fmt.Errorf("upload asset %q: %w", assetName, err)
	}

	return &semrelv1.UploadAssetResponse{AssetUrl: asset.GetBrowserDownloadURL()}, nil
}

func (p *GitHubProvider) clientForContext(releaseCtx *semrelv1.ReleaseContext) *github.Client {
	if releaseCtx == nil {
		return p.client
	}

	apiURL := strings.TrimSpace(releaseCtx.GetConfig()["api_url"])
	if apiURL == "" {
		return p.client
	}

	parsedURL, err := url.Parse(ensureTrailingSlash(apiURL))
	if err != nil {
		p.logger.Warn("invalid api_url; using default GitHub base URL", "api_url", apiURL, "error", err)
		return p.client
	}

	newClient := github.NewClient(p.client.Client())
	newClient.BaseURL = parsedURL
	newClient.UploadURL = parsedURL
	return newClient
}

func (p *GitHubProvider) listAllReleases(ctx context.Context, client *github.Client, owner, repo string) ([]*github.RepositoryRelease, error) {
	allReleases := make([]*github.RepositoryRelease, 0)
	opts := &github.ListOptions{PerPage: 100}

	for {
		releases, resp, err := client.Repositories.ListReleases(ctx, owner, repo, opts)
		if err != nil {
			if rateErr := parseRateLimit(err); rateErr != nil {
				return nil, rateErr
			}
			return nil, fmt.Errorf("list releases for %s/%s: %w", owner, repo, err)
		}
		allReleases = append(allReleases, releases...)
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allReleases, nil
}

func (p *GitHubProvider) listAllTags(ctx context.Context, client *github.Client, owner, repo string) ([]*github.RepositoryTag, error) {
	allTags := make([]*github.RepositoryTag, 0)
	opts := &github.ListOptions{PerPage: 100}

	for {
		tags, resp, err := client.Repositories.ListTags(ctx, owner, repo, opts)
		if err != nil {
			if rateErr := parseRateLimit(err); rateErr != nil {
				return nil, rateErr
			}
			return nil, fmt.Errorf("list tags for %s/%s: %w", owner, repo, err)
		}
		allTags = append(allTags, tags...)
		if resp == nil || resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return allTags, nil
}

func (p *GitHubProvider) listRecentCommits(ctx context.Context, client *github.Client, owner, repo string) ([]*semrelv1.Commit, error) {
	opts := &github.CommitsListOptions{ListOptions: github.ListOptions{PerPage: 100}}
	recentCommits := make([]*semrelv1.Commit, 0, 100)

	for len(recentCommits) < 100 {
		commits, resp, err := client.Repositories.ListCommits(ctx, owner, repo, opts)
		if err != nil {
			if rateErr := parseRateLimit(err); rateErr != nil {
				return nil, rateErr
			}
			return nil, fmt.Errorf("list commits for %s/%s: %w", owner, repo, err)
		}
		for _, repositoryCommit := range commits {
			commit, err := p.commitToProto(ctx, client, owner, repo, repositoryCommit)
			if err != nil {
				return nil, err
			}
			recentCommits = append(recentCommits, commit)
			if len(recentCommits) >= 100 {
				break
			}
		}
		if resp == nil || resp.NextPage == 0 || len(commits) == 0 {
			break
		}
		opts.Page = resp.NextPage
	}

	return recentCommits, nil
}

func (p *GitHubProvider) commitToProto(ctx context.Context, client *github.Client, owner, repo string, repositoryCommit *github.RepositoryCommit) (*semrelv1.Commit, error) {
	if repositoryCommit == nil {
		return &semrelv1.Commit{}, nil
	}

	sha := repositoryCommit.GetSHA()
	detailedCommit, _, err := client.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		if rateErr := parseRateLimit(err); rateErr != nil {
			return nil, rateErr
		}
		return nil, fmt.Errorf("get commit %s: %w", sha, err)
	}

	rawMessage := detailedCommit.GetCommit().GetMessage()
	if rawMessage == "" {
		rawMessage = repositoryCommit.GetCommit().GetMessage()
	}

	authorName := detailedCommit.GetCommit().GetAuthor().GetName()
	authorEmail := detailedCommit.GetCommit().GetAuthor().GetEmail()
	timestamp := int64(0)
	if authoredAt := detailedCommit.GetCommit().GetAuthor().GetDate(); !authoredAt.IsZero() {
		timestamp = authoredAt.Unix()
	}

	filesChanged := make([]string, 0, len(detailedCommit.Files))
	for _, file := range detailedCommit.Files {
		if file == nil {
			continue
		}
		if filename := file.GetFilename(); filename != "" {
			filesChanged = append(filesChanged, filename)
		}
	}

	return &semrelv1.Commit{
		Sha:          sha,
		RawMessage:   rawMessage,
		FilesChanged: filesChanged,
		AuthorName:   authorName,
		AuthorEmail:  authorEmail,
		Timestamp:    timestamp,
	}, nil
}

func (p *GitHubProvider) resolveTagSHA(ctx context.Context, client *github.Client, owner, repo, tag, fallback string) (string, error) {
	if strings.TrimSpace(tag) == "" {
		return strings.TrimSpace(fallback), nil
	}

	ref, _, err := client.Git.GetRef(ctx, owner, repo, "tags/"+tag)
	if err != nil {
		if fallback != "" {
			return strings.TrimSpace(fallback), nil
		}
		if rateErr := parseRateLimit(err); rateErr != nil {
			return "", rateErr
		}
		return "", fmt.Errorf("get ref for tag %q: %w", tag, err)
	}

	if ref.GetObject().GetType() == "tag" {
		annotatedTag, _, err := client.Git.GetTag(ctx, owner, repo, ref.GetObject().GetSHA())
		if err != nil {
			if rateErr := parseRateLimit(err); rateErr != nil {
				return "", rateErr
			}
			return "", fmt.Errorf("resolve annotated tag %q: %w", tag, err)
		}
		return annotatedTag.GetObject().GetSHA(), nil
	}

	return ref.GetObject().GetSHA(), nil
}

func repoDetails(releaseCtx *semrelv1.ReleaseContext) (string, string, error) {
	if releaseCtx == nil {
		return "", "", fmt.Errorf("release context is required")
	}

	owner := strings.TrimSpace(releaseCtx.GetRepoOwner())
	repo := strings.TrimSpace(releaseCtx.GetRepoName())
	if owner == "" {
		owner = strings.TrimSpace(releaseCtx.GetConfig()["owner"])
	}
	if repo == "" {
		repo = strings.TrimSpace(releaseCtx.GetConfig()["repo"])
	}
	if repo == "" {
		repo = strings.TrimSpace(releaseCtx.GetConfig()["repository"])
	}

	if owner == "" || repo == "" {
		return "", "", fmt.Errorf("repository owner and name are required")
	}

	return owner, repo, nil
}

func parseRateLimit(err error) error {
	if err == nil {
		return nil
	}

	var rateLimitErr *github.RateLimitError
	if errors.As(err, &rateLimitErr) {
		return fmt.Errorf("github rate limit exceeded: %w", err)
	}

	var abuseRateLimitErr *github.AbuseRateLimitError
	if errors.As(err, &abuseRateLimitErr) {
		retryAfter := abuseRateLimitErr.GetRetryAfter()
		if retryAfter > 0 {
			return fmt.Errorf("github abuse rate limit triggered; retry after %s: %w", retryAfter, err)
		}
		return fmt.Errorf("github abuse rate limit triggered: %w", err)
	}

	return nil
}

func versionToTag(version *semrelv1.SemanticVersion) string {
	if version == nil {
		return ""
	}

	tag := fmt.Sprintf("v%d.%d.%d", version.GetMajor(), version.GetMinor(), version.GetPatch())
	if version.GetPreRelease() != "" {
		tag += "-" + version.GetPreRelease()
	}
	if version.GetBuildMetadata() != "" {
		tag += "+" + version.GetBuildMetadata()
	}
	return tag
}

func tagToVersion(tag string) (*semrelv1.SemanticVersion, error) {
	matches := semverTagPattern.FindStringSubmatch(strings.TrimSpace(tag))
	if matches == nil {
		return nil, fmt.Errorf("invalid semantic version tag: %q", tag)
	}

	major, err := strconv.ParseUint(matches[1], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse major version: %w", err)
	}
	minor, err := strconv.ParseUint(matches[2], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse minor version: %w", err)
	}
	patch, err := strconv.ParseUint(matches[3], 10, 32)
	if err != nil {
		return nil, fmt.Errorf("parse patch version: %w", err)
	}

	return &semrelv1.SemanticVersion{
		Major:         uint32(major),
		Minor:         uint32(minor),
		Patch:         uint32(patch),
		PreRelease:    matches[4],
		BuildMetadata: matches[5],
	}, nil
}

func compareVersions(left, right *semrelv1.SemanticVersion) int {
	if diff := compareUint32(left.GetMajor(), right.GetMajor()); diff != 0 {
		return diff
	}
	if diff := compareUint32(left.GetMinor(), right.GetMinor()); diff != 0 {
		return diff
	}
	if diff := compareUint32(left.GetPatch(), right.GetPatch()); diff != 0 {
		return diff
	}
	return comparePreRelease(left.GetPreRelease(), right.GetPreRelease())
}

func compareUint32(left, right uint32) int {
	switch {
	case left < right:
		return -1
	case left > right:
		return 1
	default:
		return 0
	}
}

func comparePreRelease(left, right string) int {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	switch {
	case left == right:
		return 0
	case left == "":
		return 1
	case right == "":
		return -1
	}

	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	maxParts := len(leftParts)
	if len(rightParts) > maxParts {
		maxParts = len(rightParts)
	}

	for i := 0; i < maxParts; i++ {
		if i >= len(leftParts) {
			return -1
		}
		if i >= len(rightParts) {
			return 1
		}

		leftPart := leftParts[i]
		rightPart := rightParts[i]

		leftNumber, leftErr := strconv.Atoi(leftPart)
		rightNumber, rightErr := strconv.Atoi(rightPart)

		if leftErr == nil && rightErr == nil {
			switch {
			case leftNumber < rightNumber:
				return -1
			case leftNumber > rightNumber:
				return 1
			}
			continue
		}
		if leftErr == nil {
			return -1
		}
		if rightErr == nil {
			return 1
		}
		switch strings.Compare(leftPart, rightPart) {
		case -1:
			return -1
		case 1:
			return 1
		}
	}

	return 0
}

func isZeroVersion(version *semrelv1.SemanticVersion) bool {
	return version == nil || (version.GetMajor() == 0 && version.GetMinor() == 0 && version.GetPatch() == 0 && version.GetPreRelease() == "" && version.GetBuildMetadata() == "")
}

func ensureTrailingSlash(value string) string {
	if strings.HasSuffix(value, "/") {
		return value
	}
	return value + "/"
}

var _ Provider = (*GitHubProvider)(nil)
