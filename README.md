# provider-github

[![Latest Release](https://img.shields.io/github/v/release/SemRels/provider-github?label=version\&color=blue)](https://github.com/SemRels/provider-github/releases/latest)

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
  --certificate-identity-regexp 'https://github.com/SemRels/provider-github/.github/workflows/release.yml.*' \
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
```

## `SEMREL_PLUGIN_*` variables

| Name | Required | Description | Default |
| --- | --- | --- | --- |
| `SEMREL_PLUGIN_TOKEN` | Required | GitHub token with `repo` scope. | None |
| `SEMREL_PLUGIN_OWNER` | Optional | Repository owner. Defaults from the git remote when available. | Derived from git remote |
| `SEMREL_PLUGIN_REPO` | Optional | Repository name. Defaults from the git remote when available. | Derived from git remote |
| `SEMREL_PLUGIN_DRAFT` | Optional | Create the release as a draft. | false |
| `SEMREL_PLUGIN_PRERELEASE` | Optional | Mark the release as a prerelease. | false |
| `SEMREL_PLUGIN_ASSETS` | Optional | Comma-separated file paths or glob patterns to upload as GitHub Release assets. | None |

## `SEMREL_*` release context used

| Variable | Description |
| --- | --- |
| `SEMREL_VERSION` | Resolved release version for the current run. |
| `SEMREL_TAG_NAME` | Git tag name semrel will create or publish. |
| `SEMREL_NEXT_VERSION` | Next version computed by semrel for the release. |
| `SEMREL_CHANGELOG` | Generated changelog text for the release. |
| `SEMREL_DRY_RUN` | Whether semrel is running in dry-run mode. |

## Example behavior

The plugin creates a GitHub release for the current tag, publishes the changelog as release notes, and can upload matching assets listed in `SEMREL_PLUGIN_ASSETS`.

Examples:

```bash
SEMREL_PLUGIN_ASSETS=dist/*.tar.gz,dist/*.zip,build/myapp
```

## License

Apache-2.0
