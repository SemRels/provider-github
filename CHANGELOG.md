## v0.5.3 (2026-08-17)

### Bug Fixes

* remove commit_changelog:false with no replacement plugin - CHANGELOG.md was never updated
* resolve OCI digests via jq instead of oras tag lookup (Docker rewrites bare tags to :latest)

