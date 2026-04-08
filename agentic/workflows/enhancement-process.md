# Enhancement Process

**Last Updated**: 2026-04-08  

## Overview

How to propose and implement enhancements in OpenShift.

## Process Flow

```
1. Proposal → 2. Review → 3. Approval → 4. Implementation → 5. Graduation
```

## 1. Write Enhancement Proposal

Create `enhancements/<area>/<feature-name>.md`:

```markdown
---
title: Feature Name
authors:
  - "@your-github"
reviewers:
  - "@reviewer1"
  - "@reviewer2"
approvers:
  - "@approver1"
creation-date: 2024-01-15
last-updated: 2024-01-15
status: provisional
---

# Feature Name

## Summary

One paragraph summary of the feature.

## Motivation

### Goals
- Goal 1
- Goal 2

### Non-Goals
- Non-goal 1

## Proposal

### User Stories

**As a** cluster admin  
**I want** to configure X  
**So that** I can achieve Y

### API Extensions

```yaml
apiVersion: myapi.openshift.io/v1
kind: MyResource
spec:
  newField: value
```

### Implementation Details

Technical approach, components affected, etc.

### Risks and Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|-----------|
| Risk 1 | High | Low | Mitigation strategy |

## Design Details

### Test Plan

- Unit tests: X
- Integration tests: Y
- E2E tests: Z

### Graduation Criteria

**Alpha**:
- Feature implemented
- Basic E2E tests

**Beta**:
- Passing upgrade tests
- No major bugs

**GA**:
- Documented
- Stable for 2 releases

### Upgrade / Downgrade Strategy

How does this affect upgrades?

### Version Skew Strategy

How does this handle version differences?

## Implementation History

- 2024-01-15: Initial proposal
- 2024-02-01: Approved
- 2024-03-15: Alpha implementation
```

## 2. Submit for Review

```bash
git checkout -b enhancements/my-feature
git add enhancements/my-area/my-feature.md
git commit -m "Enhancement: My Feature"
gh pr create --base master
```

## 3. Address Review Comments

Reviewers will check:
- **Technical feasibility**: Can this be implemented?
- **Alignment**: Does it fit OpenShift goals?
- **Upgrade impact**: Safe to upgrade?
- **API design**: Follows conventions?
- **User experience**: Easy to use?

## 4. Get Approval

Required approvals:
- ✅ Area owner (OWNERS file)
- ✅ API reviewers (if adding/changing APIs)
- ✅ Architecture review (for large changes)
- ✅ Release team (if affects release)

## 5. Implement

After approval:

```bash
# Reference enhancement in implementation PRs
git commit -m "Implement my-feature

Enhancement: https://github.com/openshift/enhancements/pull/123

This implements the controller for my-feature as described in
the enhancement proposal."
```

## 6. Graduate (if applicable)

For features with maturity levels:

| Level | Meaning | Requirements |
|-------|---------|--------------|
| **Alpha** | Tech preview, may change | Implementation + basic tests |
| **Beta** | Supported, API stable | Upgrade tests + docs |
| **GA** | Production-ready | Stable for 2+ releases |

## Enhancement Template

Location: `enhancements/TEMPLATE.md`

Required sections:
- Summary
- Motivation (Goals/Non-Goals)
- Proposal (User Stories, API, Implementation)
- Design Details (Test Plan, Graduation Criteria, Upgrade Strategy)

## Review Timeline

| Stage | Expected Time |
|-------|--------------|
| Initial review | 1-2 weeks |
| Iterations | 2-4 weeks |
| Approval | 1 week |
| **Total** | **4-7 weeks** |

## Common Mistakes

### ❌ Too vague

```markdown
## Proposal
Make X better.
```

### ✅ Specific

```markdown
## Proposal
Add field `maxRetries: int` to configure retry behavior.
Default: 3. Range: 0-10.
```

### ❌ No user story

```markdown
## Motivation
We need feature X.
```

### ✅ User-focused

```markdown
## Motivation
**As a** cluster admin  
**I want** to limit retry attempts  
**So that** I can prevent infinite retry loops
```

## Examples

| Enhancement | Complexity | Review Time |
|-------------|-----------|-------------|
| Add optional API field | Low | 1-2 weeks |
| New CRD + controller | Medium | 4-6 weeks |
| Multi-component feature | High | 8-12 weeks |

## References

- **Enhancement Repo**: https://github.com/openshift/enhancements
- **Template**: https://github.com/openshift/enhancements/blob/master/TEMPLATE.md
- **Implementing Features**: [implementing-features.md](./implementing-features.md)
- **API Evolution**: [../practices/development/api-evolution.md](../practices/development/api-evolution.md)
