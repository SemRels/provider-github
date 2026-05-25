# provider-github

`provider-github` is the GitHub provider MVP plugin for SemRel. It exposes the SemRel `ProviderPlugin` gRPC service through `hashicorp/go-plugin`, then translates SemRel release operations into GitHub API calls for releases, tags, commits, and release assets.

## Features

- Resolve the latest released semantic version from GitHub releases or tags
- Collect commits since the previous release (or recent commits when no release exists)
- Create GitHub releases for the calculated next version
- Upload release assets to an existing GitHub release
- Run over `go-plugin` with the SemRel handshake contract

## Configuration

### Environment variables

- `GITHUB_TOKEN`: Personal access token or GitHub App installation token used for authenticated API calls

If `GITHUB_TOKEN` is not set, the plugin still starts and serves the handshake/gRPC transport, but GitHub API calls may be limited or fail for private repositories.

### Plugin config keys

These keys are read from `ReleaseContext.config`:

- `owner`: fallback repository owner when `repo_owner` is not populated
- `repo`: fallback repository name when `repo_name` is not populated
- `repository`: accepted alias for `repo`
- `api_url`: optional GitHub API base URL override (for GitHub Enterprise Server or tests)

## Example `.semrel.yaml`

```yaml
plugins:
  - name: provider-github
    type: provider
    command: ./bin/plugin
    config:
      owner: SemRels
      repo: semrel
      api_url: https://api.github.com
```

Set `GITHUB_TOKEN` in the environment before SemRel launches the plugin.

## How it is launched

SemRel starts the compiled plugin binary as a child process. The binary calls `plugin.Serve()` and uses the `go-plugin` gRPC transport.

Important transport behavior:

- stdout is reserved for the `go-plugin` handshake
- application logging must go to stderr
- the plugin blocks in `Serve()` until the host disconnects

## Handshake contract

Host and plugin must agree on these constants:

```go
var HandshakeConfig = plugin.HandshakeConfig{
    ProtocolVersion:  1,
    MagicCookieKey:   "SEMREL_PLUGIN",
    MagicCookieValue: "provider",
}
```

## Error handling

Common runtime scenarios:

- **Missing token**: the binary still starts; `HealthCheck` warns and skips the authenticated rate-limit probe
- **Rate limits**: GitHub rate-limit and abuse-limit responses are detected and returned as wrapped errors
- **Repository not found / auth denied**: GitHub HTTP errors are returned with context from the failing operation
- **No releases or tags**: `GetLastRelease` returns an empty semantic version and empty tag SHA
- **Dry run**: `CreateRelease` returns a synthetic URL and does not call GitHub

## Building

```powershell
go build ./...
make build
```

The built plugin binary is produced from `cmd/plugin`.

## Testing

```powershell
go test -v -cover ./...
make test
```

Provider tests use `net/http/httptest` to mock the GitHub API. gRPC transport tests start a real in-process gRPC server.

## Development notes

- Module path: `github.com/SemRels/provider-github`
- Generated protobuf bindings live in `internal/gen/v1`
- Do not log application messages to stdout; only `go-plugin` should write there during handshake
