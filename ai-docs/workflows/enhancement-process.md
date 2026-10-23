# Enhancement Process

**Purpose**: Guide authors through writing and submitting OpenShift enhancement proposals  
**Authoritative Source**: `./guidelines/enhancement_template.md`  
**Last Updated**: 2026-10-23

## Overview

OpenShift enhancements are design documents that describe significant changes to the platform. They ensure cross-team visibility, architectural review, and documentation of design decisions.

**When required**: New features, significant behavioral changes, API additions, cross-component changes.

**Not required**: Bug fixes, minor refactors, documentation updates.

## Process Flow

```text
1. Idea → 2. Enhancement PR → 3. Review → 4. Approval → 5. Implementation → 6. Status Update
```

## Step-by-Step

### 1. Write the Enhancement

Start from the template:
```bash
cp guidelines/enhancement_template.md enhancements/your-area/your-feature.md
```

**Required sections** (from the template):
- Summary
- Motivation (goals, non-goals)
- Proposal (API changes, workflow, topology considerations)
- Design details (test plan, graduation criteria)
- Operational aspects (failure modes, support procedures)
- Alternatives

### 2. Submit PR

```bash
git checkout -b enhancement-your-feature
git add enhancements/your-area/your-feature.md
git commit -m "enhancements/your-area: your-feature proposal"
git push origin enhancement-your-feature
```

**PR requirements**:
- One enhancement per PR
- File in correct subdirectory under `enhancements/`
- Title matches the enhancement title
- Label with component area

### 3. Review Process

| Reviewer | Role |
|----------|------|
| **Component lead** | Technical accuracy |
| **Staff engineer** | Architectural alignment |
| **QE representative** | Testability |
| **Docs representative** | Documentation impact |

### 4. Enhancement States

| Status | Meaning |
|--------|---------|
| **provisional** | Under discussion, not yet approved |
| **implementable** | Approved for implementation |
| **implemented** | Code merged and released |
| **deferred** | Postponed to future release |
| **rejected** | Not accepted |
| **withdrawn** | Withdrawn by author |
| **replaced** | Superseded by another enhancement |

### 5. After Approval

- Update enhancement status to `implementable`
- Create Jira epic/stories for tracking
- Begin implementation following [implementing-features.md](implementing-features.md)
- Update enhancement status as work progresses

## Topology Considerations

Every enhancement must address deployment topologies. See [topology-considerations-guide.md](topology-considerations-guide.md) for:
- Standalone / HA clusters
- Single Node OpenShift (SNO)
- Hypershift / Hosted Control Planes
- MicroShift
- OpenShift Kubernetes Engine (OKE)

## Common Mistakes

❌ **Skipping topology section**: "Not applicable" without explanation is insufficient  
❌ **Missing test plan**: Every enhancement needs a concrete test strategy  
❌ **No graduation criteria**: Define alpha → beta → GA gates  
❌ **Vague alternatives**: "We considered other approaches" — be specific  
❌ **Implementation before approval**: Don't start coding before the enhancement is approved

## References

- **Template**: `./guidelines/enhancement_template.md` (authoritative)
- **Examples**: Browse `./enhancements/` for merged examples
- **Topology**: [topology-considerations-guide.md](topology-considerations-guide.md)
- **Implementation**: [implementing-features.md](implementing-features.md)
