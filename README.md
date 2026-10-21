# provider-github

[![Latest Release](https://img.shields.io/github/v/release/SemRels/provider-github?label=version&color=blue)](https://github.com/SemRels/provider-github/releases/latest)
[![CI](https://github.com/SemRels/provider-github/actions/workflows/ci.yml/badge.svg)](https://github.com/SemRels/provider-github/actions/workflows/ci.yml)
[![License](https://img.shields.io/github/license/SemRels/provider-github)](LICENSE)
[![OpenSSF Scorecard](https://api.scorecard.dev/projects/github.com/SemRels/provider-github/badge)](https://scorecard.dev/viewer/?uri=github.com/SemRels/provider-github)
[![REUSE status](https://api.reuse.software/badge/github.com/SemRels/provider-github)](https://api.reuse.software/info/github.com/SemRels/provider-github)

Publishes the semrel release to GitHub Releases.

This plugin is distributed as the standalone Go binary `semrel-plugin-provider-github`. Semrel executes the binary as a subprocess, provides plugin configuration through `SEMREL_PLUGIN_*` environment variables, provides release context through `SEMREL_*` environment variables, reads standard output, and treats exit code `0` as success and any non-zero exit code as failure. Install the binary in `~/.semrel/plugins/` or anywhere on your `$PATH`.

## Installation

### Binary

```bash
go install github.com/SemRels/provider-github/cmd/plugin@latest
```

### Docker

Pre-built, multi-platform images (linux/amd64, linux/arm64) are published to the GitHub Container Registry on every release:

```bash
docker pull ghcr.io/semrels/provider-github:latest
```

Images are signed with [cosign](https://github.com/sigstore/cosign) and include a full SBOM attestation. Verify the signature:

```bash
cosign verify ghcr.io/semrels/provider-github:latest \
  --certificate-identity-regexp 'https://github.com/SemRels/provider-github/.github/workflows/container.yml.*' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
```


## Configuration

```yaml
plugins:
  - name: provider-github
    path: ~/.semrel/plugins/semrel-plugin-provider-github
    env:
      SEMREL_PLUGIN_TOKEN: "${GITHUB_TOKEN}"
      SEMREL_PLUGIN_OWNER: "SemRels"
      SEMREL_PLUGIN_REPO: "provider-github"
      SEMREL_PLUGIN_DRAFT: "false"
      SEMREL_PLUGIN_PRERELEASE: "false"
      SEMREL_PLUGIN_ASSETS: "dist/*.tar.gz,dist/*.zip,build/myapp"
      SEMREL_PLUGIN_COMMENT_ON_PRS: "true"
```

## `SEMREL_PLUGIN_*` variables

| Name | Required | Description | Default |
| --- | --- | --- | --- |
| `SEMREL_PLUGIN_TOKEN` | Required | GitHub token with `repo` scope (falls back to `GITHUB_TOKEN`). | None |
| `SEMREL_PLUGIN_OWNER` | Optional | Repository owner. Defaults from `GITHUB_REPOSITORY` or the git remote when available. | Derived from git remote |
| `SEMREL_PLUGIN_REPO` | Optional | Repository name. Defaults from `GITHUB_REPOSITORY` or the git remote when available. | Derived from git remote |
| `SEMREL_PLUGIN_BASE_URL` | Optional | GitHub API base URL (highest priority override). | `https://api.github.com` |
| `SEMREL_PLUGIN_API_URL` | Optional | GitHub API base URL alias (used when `SEMREL_PLUGIN_BASE_URL` is not set). | `https://api.github.com` |
| `SEMREL_PLUGIN_CA_CERT` | Optional | Path to a PEM-encoded CA certificate bundle added to the HTTP client's trusted roots. | None |
| `SEMREL_PLUGIN_INSECURE_SKIP_VERIFY` | Optional | Skip TLS certificate verification (`true`/`1`). Use only as a last resort. | `false` |
| `SEMREL_PLUGIN_DRAFT` | Optional | Create the release as a draft. | false |
| `SEMREL_PLUGIN_PRERELEASE` | Optional | Mark the release as a prerelease. | false |
| `SEMREL_PLUGIN_ASSETS` | Optional | Comma-separated file paths or glob patterns to upload as GitHub Release assets. | None |
| `SEMREL_PLUGIN_COMMENT_ON_PRS` | Optional | Add an idempotent comment to pull requests included in the release. Defaults to true; set to `false` to disable. The token needs pull-request read and issue-comment write permissions. | true |

## `SEMREL_*` and GitHub environment context used

| Variable | Description |
| --- | --- |
| `SEMREL_VERSION` | Resolved release version for the current run. |
| `SEMREL_TAG_NAME` | Git tag name semrel will create or publish. |
| `SEMREL_NEXT_VERSION` | Next version computed by semrel for the release. |
| `SEMREL_CHANGELOG` | Generated changelog text for the release. |
| `SEMREL_DRY_RUN` | Whether semrel is running in dry-run mode. |
| `GITHUB_API_URL` | Automatically used as the API base URL in GitHub Actions (including GitHub Enterprise Server) when `SEMREL_PLUGIN_BASE_URL` and `SEMREL_PLUGIN_API_URL` are not set. |
| `GITHUB_REPOSITORY` | Fallback `owner/repo` when `SEMREL_PLUGIN_OWNER` / `SEMREL_PLUGIN_REPO` are not set. |
| `GITHUB_TOKEN` | Fallback token when `SEMREL_PLUGIN_TOKEN` is not set. |

## GitHub Enterprise

`provider-github` supports **GitHub Enterprise Server (GHES)** and **GitHub Enterprise Cloud with data residency (`*.ghe.com`)** out of the box:

- **API URL resolution order:** `SEMREL_PLUGIN_BASE_URL` → `SEMREL_PLUGIN_API_URL` → `GITHUB_API_URL` → `https://api.github.com`. Inside GitHub Actions on GHES, `GITHUB_API_URL` (for example `https://ghe.example.com/api/v3`) is picked up automatically.
- **Endpoint & upload routing:** `/api/v3` prefixes and trailing slashes are preserved when resolving endpoints, and release asset uploads use the `upload_url` returned by the release response (or `/api/uploads/` / `uploads.<subdomain>.ghe.com` fallback).
- **API version header:** `X-GitHub-Api-Version` is sent only to `api.github.com` and `*.ghe.com` hosts so older GHES versions that do not recognize the header are not rejected.
- **Custom CA / self-signed certificates:** Set `SEMREL_PLUGIN_CA_CERT=/path/to/ca-bundle.pem` to trust an internal or self-signed CA certificate bundle. As a last resort, `SEMREL_PLUGIN_INSECURE_SKIP_VERIFY=true` disables TLS verification.

```yaml
plugins:
  - name: provider-github
    path: ~/.semrel/plugins/semrel-plugin-provider-github
    env:
      SEMREL_PLUGIN_TOKEN: "${GITHUB_TOKEN}"
      SEMREL_PLUGIN_BASE_URL: "https://ghe.example.com/api/v3"
      SEMREL_PLUGIN_CA_CERT: "/etc/ssl/certs/internal-ca.pem"
```

## Example behavior

The plugin creates a GitHub release for the current tag, publishes the changelog as release notes, and can upload matching assets listed in `SEMREL_PLUGIN_ASSETS`. By default it finds pull requests associated with commits since the previous release and comments with a link to the new release. Set `SEMREL_PLUGIN_COMMENT_ON_PRS` to `false` to disable comments. Re-running the same release does not duplicate comments.

Examples:

```bash
SEMREL_PLUGIN_ASSETS=dist/*.tar.gz,dist/*.zip,build/myapp
```

## License

Apache-2.0
