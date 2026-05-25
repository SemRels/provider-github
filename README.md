# provider-github

GitHub release provider plugin for [SemRel](https://github.com/SemRels/semrel).

Creates GitHub Releases, fetches commit history, and uploads release assets
via the [GitHub REST API](https://docs.github.com/en/rest).

## What It Does

| RPC              | Description                                          |
|------------------|------------------------------------------------------|
| `GetLastRelease` | Fetches the latest published GitHub Release and tag SHA |
| `GetCommitsSince`| Returns commits between a SHA and HEAD on the release branch |
| `CreateRelease`  | Creates a new GitHub Release (supports dry-run)      |
| `UploadAsset`    | Uploads a release artifact to an existing release    |

## Configuration (`.semrel.yaml`)

```yaml
plugins:
  - name: provider-github
    type: provider
    config:
      github_token: ${GITHUB_TOKEN}  # optional; falls back to GITHUB_TOKEN/GH_TOKEN env vars
```

## Required Permissions

The `GITHUB_TOKEN` (or `GH_TOKEN`) must have:
- `contents: write` — to create tags and releases

## Development

```bash
go test ./...
go build ./cmd/plugin
```

## License

Apache-2.0 — see [LICENSE](LICENSE).

