# Git Workflow

**Category**: Engineering Practice  
**Applies To**: All OpenShift repositories  
**Last Updated**: 2026-04-08  

## Overview

Standard Git workflow for OpenShift development using feature branches, pull requests, and semantic commits.

## Branch Strategy

### Main Branches

```
master (or main)
  ↓
release-4.16
  ↓
release-4.15
  ↓
release-4.14
```

**Rules**:
- `master`: Active development, latest code
- `release-X.Y`: Maintained releases, backports only
- All changes go to `master` first, then cherry-pick to release branches

### Feature Branches

```
username/feature-description
  └─> PR to master
```

**Naming conventions**:
```
username/OCPBUGS-12345-fix-memory-leak
username/add-webhook-support
username/update-dependencies
```

## Commit Messages

### Format

```
<area>: <short summary (50 chars)>

<Detailed explanation (72 chars per line)>

Why this change is needed.
What it changes.
How it was tested.

Fixes: https://issues.redhat.com/browse/OCPBUGS-12345
```

### Examples

**Good**:
```
controller: Fix memory leak in reconciliation loop

The reconcile loop was not releasing watch handles, causing
memory to grow unbounded over time.

Added cleanup in finalizer to release watches when resources
are deleted.

Tested by running operator for 24h with 1000 resources.
Memory usage now stable at 100MB.

Fixes: https://issues.redhat.com/browse/OCPBUGS-12345
```

**Bad**:
```
fix bug
```

### Commit Message Rules

1. ✅ Start with lowercase area: `controller:`, `pkg/util:`, `docs:`
2. ✅ Imperative mood: "Fix bug" not "Fixed bug"
3. ✅ 50 char summary, 72 char body lines
4. ✅ Reference Jira/GitHub issues
5. ❌ Don't end summary with period

## Pull Request Workflow

### 1. Create Feature Branch

```bash
git checkout master
git pull origin master
git checkout -b username/OCPBUGS-12345-fix-leak
```

### 2. Make Changes

```bash
# Make changes
git add pkg/controller/reconcile.go
git commit -m "controller: Fix memory leak

..."

# Run tests locally
make verify
make test-unit
```

### 3. Push and Create PR

```bash
git push origin username/OCPBUGS-12345-fix-leak

# Create PR via GitHub CLI
gh pr create --base master --head username/OCPBUGS-12345-fix-leak \
  --title "Fix memory leak in reconciliation loop" \
  --body "Fixes https://issues.redhat.com/browse/OCPBUGS-12345"
```

### 4. Address Review Comments

```bash
# Make changes based on feedback
git add pkg/controller/reconcile.go
git commit --amend  # Amend last commit
git push --force    # Force push (safe on feature branch)
```

### 5. Merge

**Merge strategies**:
- **Squash merge**: Multiple commits → single commit (preferred for small PRs)
- **Merge commit**: Preserve commit history (for large features)
- **Rebase**: Avoid (breaks cherry-pick traceability)

## Cherry-Picking to Release Branches

### When to Cherry-Pick

- ✅ Bug fixes
- ✅ Security patches
- ✅ Documentation updates
- ❌ New features (except in rare cases)

### Process

```bash
# 1. Merge to master first
# PR #123 merged to master with commit abc123

# 2. Cherry-pick to release branch
git checkout release-4.16
git pull origin release-4.16
git cherry-pick abc123

# 3. If conflicts, resolve and continue
# Edit conflicting files
git add <files>
git cherry-pick --continue

# 4. Push and create PR
git push origin username/OCPBUGS-12345-fix-leak-4.16
gh pr create --base release-4.16 \
  --title "[release-4.16] Fix memory leak" \
  --body "Cherry-pick of #123"
```

### Cherry-Pick PR Format

```
[release-4.16] controller: Fix memory leak

Cherry-pick of #123
Original commit: abc123

/cherry-pick release-4.15
```

## Revert Process

```bash
# Create revert commit
git revert abc123

# Push and create PR
git push origin username/revert-memory-leak-fix
gh pr create --base master \
  --title "Revert: Fix memory leak" \
  --body "Reverting #123 due to regression"
```

**Revert commit message**:
```
Revert "controller: Fix memory leak"

This reverts commit abc123.

Reason: Introduced regression in reconciliation latency.
Breaking CI: https://prow.ci.openshift.org/...

Will resubmit with fix after investigation.
```

## Commit Best Practices

### Atomic Commits

Each commit should be self-contained:

```bash
# Good: Separate commits for different changes
git commit -m "api: Add new field to MachineConfig"
git commit -m "controller: Use new field in reconciliation"
git commit -m "tests: Add tests for new field"

# Bad: Mixing unrelated changes
git commit -m "Add field, fix bug, update deps"
```

### Commit Often

```bash
# Commit working changes frequently
git commit -m "WIP: Add validation logic"
git commit -m "WIP: Add tests"
git commit -m "Clean up validation code"

# Squash before creating PR
git rebase -i master
# Mark commits as "squash" except first
```

### Sign Commits

```bash
# Configure GPG signing
git config --global user.signingkey <key-id>
git config --global commit.gpgsign true

# Or sign individual commits
git commit -S -m "controller: Fix bug"
```

## PR Best Practices

### PR Size

**Target**: <400 lines changed

**Too large?** Split into multiple PRs:
```
PR #1: Add API changes
PR #2: Implement controller logic
PR #3: Add tests
PR #4: Update documentation
```

### PR Description

Include:
```markdown
## Summary
Brief description of changes

## Related Issues
Fixes: https://issues.redhat.com/browse/OCPBUGS-12345

## Testing
- [ ] Unit tests added
- [ ] Integration tests added
- [ ] Manual testing on cluster

## Upgrade Impact
None / Describe impact
```

### Draft PRs

Use draft PRs for work in progress:
```bash
gh pr create --draft
```

Convert to ready when done:
```
/ready
```

## Examples by Repository

| Repository | Branch Strategy | Merge Policy |
|------------|----------------|--------------|
| kubernetes/kubernetes | master + release branches | Squash for small, merge for large |
| openshift/machine-config-operator | master + release branches | Squash preferred |
| openshift/enhancements | master only | Squash always |

## Common Mistakes

### ❌ Committing to master directly

```bash
# Don't do this
git checkout master
git commit -m "fix"
git push origin master
```

### ❌ Large uncommitted changes

```bash
# Don't accumulate large uncommitted diffs
git status
# 50 files changed...
```

### ❌ Merge conflicts on master

```bash
# Don't merge master into feature branch
git merge master  # Bad

# Rebase instead
git rebase master  # Good
```

### ❌ Pushing broken code

```bash
# Always run tests before pushing
make verify test-unit
git push
```

## Useful Git Commands

```bash
# View commit history
git log --oneline --graph

# Amend last commit
git commit --amend

# Interactive rebase
git rebase -i HEAD~3

# Stash uncommitted changes
git stash
git stash pop

# View diff
git diff
git diff --staged

# Undo last commit (keep changes)
git reset --soft HEAD~1

# View file history
git log --follow path/to/file
```

## References

- **Conventional Commits**: https://www.conventionalcommits.org/
- **Git Best Practices**: https://git-scm.com/book/en/v2
- **OpenShift Workflow**: https://github.com/openshift/enhancements/tree/master/dev-guide
- **Code Review**: [code-review.md](./code-review.md)
