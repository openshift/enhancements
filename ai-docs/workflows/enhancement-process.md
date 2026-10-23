# Enhancement Process

**Category**: Workflow
**Authoritative Source**: [guidelines/enhancement_template.md](../../guidelines/enhancement_template.md)
**Last Updated**: 2026-10-23

---

## Overview

OpenShift enhancements are design proposals that document significant changes to the platform. They serve as the record of conversations, decisions, and rationale behind designs. The authoritative template and instructions are in `guidelines/enhancement_template.md`.

## Process Summary

```
1. Pick a domain (networking, storage, auth, etc.)
2. Copy guidelines/enhancement_template.md → enhancements/<domain>/
3. Fill out metadata and overview sections
4. Create PR, assign domain experts as reviewers
5. Build consensus — drive the review process
6. Merge when design is complete enough for implementation
7. Update the EP as implementation details change
```

## Key Principles

- **Iterate**: Merge with enough detail for TechPreview, update before GA promotion
- **Build consensus**: The author drives the process — find stakeholders, address concerns, get approval
- **Document decisions**: EPs are the historical record; merge them even if implementation is deferred

## Required Sections

Every enhancement must include (enforced by linter CI):

| Section | Purpose |
|---------|---------|
| **Summary** | One-paragraph overview for release notes |
| **Motivation** | Why this change is needed |
| **Proposal** | Technical design |
| **API Extensions** | New/modified CRDs, webhooks, finalizers |
| **Topology Considerations** | Impact on SNO, HyperShift, MicroShift, OKE |
| **Implementation Details** | Phased rollout plan |
| **Drawbacks** | Reasons NOT to implement |
| **Alternatives** | Other approaches considered |
| **Test Plan** | How the feature will be tested |
| **Graduation Criteria** | TechPreview → GA promotion requirements |
| **Operational Aspects** | Monitoring, alerting, failure modes |

## Topology Considerations

Every enhancement must address deployment topology impact. See [topology-considerations-guide.md](topology-considerations-guide.md) for the detailed guide covering:
- **Standalone (HA)** — standard multi-node cluster
- **Single-node OpenShift (SNO)** — single control plane + worker node
- **HyperShift / Hosted Control Planes** — control plane separated from data plane
- **MicroShift** — minimal OpenShift for edge devices
- **OKE** — OpenShift Kubernetes Engine (subset of features)

## Roles

| Role | Responsibility |
|------|---------------|
| **Author** | Drives the process, responds to feedback, ensures EP is merged |
| **Reviewer** | Domain expert; reviews specific aspects noted in metadata |
| **Approver** | Single person; ensures EP gets needed reviews, determines consensus |
| **API Approver** | Required for API changes; assigned via #forum-api-review |

## Status Lifecycle

```
provisional → implementable → implemented
                            → deferred
                            → rejected
                            → withdrawn
                            → replaced
```

## Common Reasons for Enhancement Rejection

Based on historical team discussions:
- Insufficient topology considerations (particularly missing HyperShift or SNO analysis)
- Missing or vague graduation criteria
- API design that doesn't follow `dev-guide/api-conventions.md`
- No clear test plan or disruption analysis
- Scope creep without updated design rationale

## Platform Documentation

For the authoritative template, see [guidelines/enhancement_template.md](../../guidelines/enhancement_template.md). For topology guidance, see [topology-considerations-guide.md](topology-considerations-guide.md). For API conventions, see [dev-guide/api-conventions.md](../../dev-guide/api-conventions.md).
