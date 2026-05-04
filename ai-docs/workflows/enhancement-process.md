# Enhancement Process

**Purpose**: AI-optimized guide for writing enhancement proposals  
**Authoritative Source**: [../guidelines/enhancement_template.md](../../guidelines/enhancement_template.md)  
**Last Updated**: 2026-04-29  

## Quick Start

| Step | Action | Output |
|------|--------|--------|
| 1 | Copy template | `cp guidelines/enhancement_template.md enhancements/my-feature.md` |
| 2 | Fill required sections | Summary, Motivation, Proposal |
| 3 | Create PR | Tag with `kind/enhancement` |
| 4 | Address review | Update based on feedback |
| 5 | Merge | Enhancement approved |

## Required Sections

| Section | Purpose | Length | Required |
|---------|---------|--------|----------|
| **Summary** | 1-sentence description | 1 sentence | ✅ |
| **Motivation** | Why this feature | 2-5 paragraphs | ✅ |
| **Goals** | What we will do | Bulleted list | ✅ |
| **Non-Goals** | What we won't do | Bulleted list | ✅ |
| **Proposal** | How it works | 1-3 pages | ✅ |
| **Risks and Mitigations** | What could go wrong | Table | ✅ |
| **Drawbacks** | Why not to do this | 1-2 paragraphs | Optional |
| **Alternatives** | Other approaches considered | List with rationale | Optional |
| **Implementation Details** | Technical design | API examples, diagrams | ✅ |
| **Test Plan** | How to verify | Test scenarios | ✅ |
| **Graduation Criteria** | When to promote | DevPreview → TechPreview → GA | ✅ |
| **Upgrade/Downgrade** | Migration strategy | Version compatibility | If applicable |

## Enhancement States

```
Idea → Proposal → Implementable → Implemented → Stable
  ↓        ↓            ↓              ↓          ↓
Draft    Review     Approved       Merged      GA
```

## Template Structure

```yaml
---
title: my-feature
authors:
  - "@myusername"
reviewers:
  - "@reviewer1"
approvers:
  - "@approver1"
creation-date: 2026-04-29
status: implementable
---

# My Feature

## Summary

One sentence describing the feature.

## Motivation

### Goals
- Goal 1
- Goal 2

### Non-Goals
- Non-goal 1

## Proposal

### User Stories
- As a cluster admin, I want...

### API Changes
```yaml
apiVersion: config.openshift.io/v1
kind: MyResource
spec:
  newField: value
```

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| Risk 1 | Mitigation 1 |

### Test Plan
- Unit tests for...
- E2E tests for...

### Graduation Criteria
- DevPreview: API defined, basic implementation
- TechPreview: Feature complete, tested
- GA: Production-ready, stable API

## Implementation History
- 2026-04-29: Initial proposal
```

## API Design Checklist

- [ ] API group and version chosen (config.openshift.io/v1, etc.)
- [ ] CRD schema defined with validation
- [ ] Status conditions follow Available/Progressing/Degraded pattern
- [ ] Backward compatibility considered
- [ ] Upgrade/downgrade path documented
- [ ] Default values specified

## Review Process

| Stage | Criteria | Reviewers |
|-------|----------|-----------|
| **Initial Review** | Complete template, clear motivation | Enhancement team |
| **API Review** | API follows conventions | API review team |
| **Implementation Review** | Technical feasibility | Component owners |
| **Approval** | All concerns addressed | Approvers |

## Common Mistakes

❌ **Missing motivation**: "We need feature X" (why?)  
❌ **Unclear proposal**: High-level description without implementation details  
❌ **No test plan**: How will we verify this works?  
❌ **Ignoring upgrades**: What happens to existing clusters?  
❌ **Breaking changes**: Not considering backward compatibility

## Examples

| Enhancement | Type | Notes |
|-------------|------|-------|
| [ClusterOperator Conditions](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/clusteroperator.md) | Pattern | Status reporting standard |
| [Machine API](https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/) | Feature | Node lifecycle management |
| [Feature Gates](https://github.com/openshift/enhancements/blob/master/dev-guide/featuresets.md) | Process | Graduation criteria |

## Workflow Commands

```bash
# Create enhancement
cp guidelines/enhancement_template.md enhancements/my-feature.md

# Validate format
make verify

# Create PR
gh pr create --title "Enhancement: My Feature" --body "..." --label kind/enhancement

# View enhancements
ls enhancements/
```

## Related

- **Authoritative Template**: [../guidelines/enhancement_template.md](../../guidelines/enhancement_template.md)
- **API Conventions**: [../practices/development/api-evolution.md](../practices/development/api-evolution.md)
- **Feature Development**: [implementing-features.md](implementing-features.md)
