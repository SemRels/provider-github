# Contributing to {{PROJECT_NAME}}

Thank you for your interest in contributing.

## Before You Start

- Read [CODE_OF_CONDUCT.md](CODE_OF_CONDUCT.md)
- Read [SECURITY.md](SECURITY.md) before reporting vulnerabilities
- Check the README for project-specific setup commands

## Workflow

1. Fork the repository and clone it locally
2. Create a topic branch from `{{DEFAULT_BRANCH}}`
3. Keep changes focused and easy to review
4. Run the relevant tests, lint checks, and build steps for the repo type
5. Update docs when behavior, APIs, or contributor workflow changes
6. Open a pull request against `{{DEFAULT_BRANCH}}`

## Commit Messages

SemRels repositories should prefer [Conventional Commits](https://www.conventionalcommits.org/).

Examples:

```text
feat(scope): add new capability
fix(scope): correct broken behavior
chore(ci): update workflow configuration
```

## DCO / Signed-off-by

Type A and Type B repositories should normally enable DCO. If `.github/workflows/dco.yml` is present, every commit must contain a valid `Signed-off-by` trailer.

```bash
git commit -s -m "feat: my change"
git config --global format.signoff true
```

## Pull Request Checklist

- [ ] Tests pass
- [ ] Linting passes
- [ ] Docs are updated if needed
- [ ] New files include SPDX headers where applicable
- [ ] Commits are signed off when DCO is enabled

## License Headers

New source files should include SPDX metadata matching the repo license policy.
