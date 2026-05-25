# provider-github

`provider-github` is a SemRel subprocess plugin that creates GitHub releases with the REST API. It reads SemRel release context from environment variables, performs a single `POST /repos/{owner}/{repo}/releases`, and exits with a normal process status code.

## Features

- No gRPC transport
- No `hashicorp/go-plugin`
- Token lookup from `SEMREL_PLUGIN_TOKEN` with `GITHUB_TOKEN` fallback
- Repository lookup from `SEMREL_PLUGIN_OWNER` / `SEMREL_PLUGIN_REPO` with `GITHUB_REPOSITORY` fallback
- Dry-run support via `SEMREL_DRY_RUN`
- Optional GitHub Enterprise or test base URL override via `SEMREL_PLUGIN_BASE_URL`

## Environment variables

### Required release context

- `SEMREL_TAG_NAME`, `SEMREL_NEXT_VERSION`, or `SEMREL_VERSION`
- `SEMREL_PLUGIN_OWNER` and `SEMREL_PLUGIN_REPO`, or `GITHUB_REPOSITORY=owner/repo`
- `SEMREL_PLUGIN_TOKEN` or `GITHUB_TOKEN` unless `SEMREL_DRY_RUN=true`

### Optional plugin settings

- `SEMREL_CHANGELOG`: release notes body
- `SEMREL_PLUGIN_NAME`: release title (defaults to the tag)
- `SEMREL_PLUGIN_DRAFT`: `true` to create a draft release
- `SEMREL_PLUGIN_PRERELEASE`: `true` to force the prerelease flag
- `SEMREL_PLUGIN_BASE_URL`: defaults to `https://api.github.com`

## Example

```powershell
$env:SEMREL_PLUGIN_TOKEN = "ghp_xxx"
$env:GITHUB_REPOSITORY = "SemRels/semrel"
$env:SEMREL_TAG_NAME = "v1.2.3"
$env:SEMREL_CHANGELOG = "## What's Changed"
go run ./cmd/plugin
```

## Exit behavior

- Exit code `0`: release created successfully, or dry-run completed
- Exit code `1`: configuration or GitHub API failure

## Building

```powershell
go build ./...
make build
```

## Testing

```powershell
go test ./... -coverprofile=coverage.out
```

Tests use `httptest.NewServer` for the GitHub API and cover the subprocess entrypoint separately.

## Development notes

- Module path remains `github.com/SemRels/provider-github`
- Minimum Go version is `1.25`
