# {{PLUGIN_NAME}}

> Replace this description with what your SemRel plugin does.

This repository is based on the `SemRels/plugin-template` GitHub template and provides a clean starting point for provider, analyzer, generator, updater, or hook plugins.

## Repository Layout

```text
cmd/plugin/              Plugin entry point
internal/plugin/         Business logic scaffold
internal/grpc/           gRPC transport scaffold
proto/v1                 Symlink to the SemRel protobuf contract
.github/workflows/       CI, release, and security automation
```

## Installation

Published binaries are distributed through releases and synchronized to `registry.semrel.io`.

## Development

```bash
go build ./cmd/plugin
go test ./...
```

## Configuration

See the SemRel documentation for plugin configuration and runtime integration details:

- https://github.com/SemRels/semrel
- https://registry.semrel.io

## Next Steps

1. Replace all `{{...}}` placeholders.
2. Rename the module path in `go.mod`.
3. Implement your plugin logic in `internal/plugin/`.
4. Wire generated protobuf bindings into `internal/grpc/`.
5. Create your first tagged release with `v*.*.*`.
