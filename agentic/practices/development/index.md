# Development Practices Index

**Last Updated**: 2026-04-08  

## Overview

Development practices and workflows for OpenShift contributors.

## Core Practices

| Practice | Purpose | File |
|----------|---------|------|
| Git Workflow | Branching, commits, PRs | [git-workflow.md](./git-workflow.md) |
| API Evolution | Backward compatible API changes | [api-evolution.md](./api-evolution.md) |
| Code Review | LGTM/approval process | [code-review.md](./code-review.md) |

## Quick Reference

### Git Workflow

- Feature branches: `username/feature-description`
- Commits: `<area>: <summary>` (50 chars)
- Merge to `master` first, then cherry-pick to release branches

See [git-workflow.md](./git-workflow.md)

### API Changes

- Never break backward compatibility in stable APIs
- Add optional fields only
- Deprecate for 3+ releases before removing
- Provide conversion webhooks for multi-version

See [api-evolution.md](./api-evolution.md)

### Code Review

- `/lgtm` from reviewer (code quality check)
- `/approve` from approver (authorized for merge)
- Both required plus passing CI

See [code-review.md](./code-review.md)

## Common Patterns

### Feature Branch

```bash
git checkout -b username/OCPBUGS-123-fix-bug master
# Make changes
git commit -m "controller: Fix bug"
gh pr create
```

### Responding to Review

```bash
# Address feedback
git commit --amend
git push --force
```

### Cherry-Pick to Release

```bash
git cherry-pick <commit-sha>
git push origin username/fix-4.16
gh pr create --base release-4.16
```

## See Also

- [Testing Practices](../testing/) - Test requirements
- [Security Practices](../security/) - Security review
- [Workflows](../../workflows/) - Feature implementation workflow
