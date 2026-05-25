# Governance

## Overview

{{PROJECT_NAME}} is an open source project governed by its maintainers and community contributors. This document describes how decisions are made and how to participate.

## Project Roles

### Contributor
Anyone who opens an issue, submits a PR, improves documentation, or participates in discussions. No formal process is required beyond following [CONTRIBUTING.md](CONTRIBUTING.md) and the [Code of Conduct](CODE_OF_CONDUCT.md).

### Reviewer
An experienced contributor trusted to review changes in a specific area. Reviewers can approve changes but should not merge without maintainer approval unless the repo explicitly grants that permission.

### Maintainer
Maintainers have merge rights and are responsible for the overall health of the repository. The current list of maintainers is in [MAINTAINERS.md](MAINTAINERS.md).

**To become a Maintainer:**
1. Have a track record of quality contributions over a sustained period
2. Be nominated by an existing maintainer
3. Receive approval from at least 2/3 of current maintainers using lazy consensus over 7 days
4. Be added to `MAINTAINERS.md` and `CODEOWNERS` via pull request

**Stepping down / Emeritus:**
Inactive maintainers should move to emeritus status through a pull request that updates [MAINTAINERS.md](MAINTAINERS.md).

## Decision Making

Default decision making uses **lazy consensus**: a proposed change is accepted unless a maintainer objects within 7 days.

For significant architectural, governance, or ecosystem decisions, open an issue or discussion first and document the outcome in the repository.

If consensus cannot be reached, active maintainers vote. Each maintainer has one vote and a simple majority decides.

## Scope By Repository Type

- **Type A / Core:** full governance applies, including architecture and release policy decisions
- **Type B / Plugin Collection:** governance applies to plugin quality gates, release expectations, and compatibility policy
- **Type C / Single Plugin:** maintainers may simplify process, but ownership and decision rules should remain documented
- **Type D / Documentation:** governance can stay lightweight, but maintainers, review expectations, and change process should still be explicit

## Changes to Governance

Changes to this document require maintainer approval and should not be merged unilaterally by the proposing author.
