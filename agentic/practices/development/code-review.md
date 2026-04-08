# Code Review

**Category**: Engineering Practice  
**Applies To**: All OpenShift repositories  
**Last Updated**: 2026-04-08  

## Overview

Code review process using LGTM (Looks Good To Me) and Approval workflow in OpenShift.

## Review Roles

| Role | Permission | Command | Meaning |
|------|-----------|---------|---------|
| **Reviewer** | OWNERS file "reviewers" | `/lgtm` | Code looks good technically |
| **Approver** | OWNERS file "approvers" | `/approve` | Approved for merge |
| **Anyone** | Public | Comment, suggest | Feedback |

## OWNERS File

```yaml
# openshift/machine-config-operator/OWNERS
reviewers:
  - user1
  - user2
  - user3
approvers:
  - maintainer1
  - maintainer2
emeritus_approvers:
  - former-maintainer
```

### Directory-Specific OWNERS

```
pkg/
├── controller/
│   └── OWNERS  # Specific owners for controller code
├── daemon/
│   └── OWNERS  # Different owners for daemon code
└── OWNERS      # Default for pkg/
```

## Review Process

### 1. Create Pull Request

```bash
gh pr create --title "Fix memory leak" --body "Fixes #123"
```

### 2. Automated Checks

- CI runs automatically (unit, verify, e2e)
- Prow assigns reviewers from OWNERS
- Prow comments with test status

### 3. Code Review

Reviewers check:
- Code correctness
- Test coverage
- Documentation
- API compatibility
- Performance impact

### 4. LGTM

```
/lgtm
```

**Effect**: Adds `lgtm` label

**Requirements**:
- Reviewer listed in OWNERS
- All conversations resolved
- CI passing

### 5. Approve

```
/approve
```

**Effect**: Adds `approved` label

**Requirements**:
- Approver listed in OWNERS
- LGTM received
- No hold labels

### 6. Merge

**Automatic** when:
- ✅ approved
- ✅ lgtm
- ✅ CI passing
- ❌ No hold

## Review Checklist

### Code Quality

- [ ] Code is readable and maintainable
- [ ] Functions are small and focused
- [ ] No obvious bugs or logic errors
- [ ] Error handling is appropriate
- [ ] No code duplication

### Testing

- [ ] Unit tests added for new code
- [ ] Integration tests for component interactions
- [ ] E2E tests for user-facing changes
- [ ] Test coverage adequate (>60%)
- [ ] Tests are not flaky

### API Changes

- [ ] API changes backward compatible
- [ ] New fields have `+optional` or defaults
- [ ] Validation added for new fields
- [ ] Conversion webhook if multi-version
- [ ] API documentation updated

### Security

- [ ] No secrets in logs
- [ ] RBAC follows least privilege
- [ ] Input validation present
- [ ] No SQL injection, XSS vulnerabilities
- [ ] Sensitive data encrypted

### Performance

- [ ] No obvious performance regressions
- [ ] Loops are efficient
- [ ] Database queries optimized
- [ ] Memory usage reasonable
- [ ] No blocking operations in hot paths

### Documentation

- [ ] README updated if needed
- [ ] API docs updated
- [ ] Comments explain "why" not "what"
- [ ] Complex logic has explanatory comments

## Review Comments

### Constructive Feedback

**Good**:
```
This loop might be slow with 1000+ items. Consider using a map for O(1) lookups:

```go
itemMap := make(map[string]Item)
for _, item := range items {
    itemMap[item.Name] = item
}
```
```

**Bad**:
```
This is slow. Fix it.
```

### Nit vs Blocking

**Nit** (non-blocking):
```
Nit: Variable name could be more descriptive
```

**Blocking** (must fix):
```
This will cause a memory leak. Must be fixed before merge.
```

### Asking Questions

```
Why is this timeout set to 30s? Is that based on testing?
```

## Responding to Reviews

### Addressing Comments

```
> Why is this timeout set to 30s?

Based on testing with 100 nodes, worst case was 25s. 
Added 20% buffer for safety.
```

### Pushing Changes

```bash
# Address feedback
git add file.go
git commit --amend
git push --force
```

### Marking Resolved

After fixing, comment:
```
Fixed in latest commit
```

## Prow Commands

| Command | Effect |
|---------|--------|
| `/lgtm` | Add LGTM label |
| `/lgtm cancel` | Remove LGTM |
| `/approve` | Add approved label |
| `/approve cancel` | Remove approved |
| `/hold` | Block merge |
| `/hold cancel` | Remove hold |
| `/retest` | Re-run failed tests |
| `/assign @user` | Assign reviewer |
| `/cc @user` | Request review |
| `/close` | Close PR |
| `/reopen` | Reopen PR |

## Self-Review

Before requesting review:

```bash
# Run locally
make verify
make test-unit
make test-integration

# Self-review diff
git diff master...HEAD

# Check PR size
git diff --stat master...HEAD
# Target: <400 lines
```

## Review Time Expectations

| PR Size | Expected Review Time |
|---------|---------------------|
| < 50 lines | Same day |
| 50-200 lines | 1-2 days |
| 200-500 lines | 2-3 days |
| 500+ lines | Consider splitting |

## Examples

### Small PR (Fast Review)

```
Files: 2
Lines: +15, -3
Tests: 1 unit test added
Review time: 1 hour
```

### Medium PR

```
Files: 8
Lines: +200, -50
Tests: Unit + integration
Review time: 1 day
```

### Large PR (Consider Splitting)

```
Files: 25
Lines: +1000, -300
Tests: Comprehensive
Review time: 3-5 days
```

## Common Issues

### Stale LGTM

If code changes after `/lgtm`, label is automatically removed. Need new `/lgtm`.

### Merge Conflicts

```bash
# Rebase on master
git fetch origin
git rebase origin/master

# Resolve conflicts
git add <resolved-files>
git rebase --continue

# Force push
git push --force
```

### Failed CI

Don't `/lgtm` until CI passes. Fix failures first.

## Best Practices

### For Authors

- Keep PRs small and focused
- Write clear PR description
- Add tests for all changes
- Self-review before requesting review
- Respond to comments promptly
- Mark conversations as resolved

### For Reviewers

- Review within 1-2 days
- Be constructive and specific
- Explain reasoning for suggestions
- Distinguish nit vs blocking
- Approve when ready, don't bikeshed

## References

- **OWNERS**: https://github.com/kubernetes/community/blob/master/contributors/guide/owners.md
- **Prow**: https://prow.ci.openshift.org/command-help
- **Git Workflow**: [git-workflow.md](./git-workflow.md)
- **API Evolution**: [api-evolution.md](./api-evolution.md)
