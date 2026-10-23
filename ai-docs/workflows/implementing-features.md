# Implementing Features

**Category**: Workflow
**Authoritative Source**: [dev-guide/](../../dev-guide/)
**Last Updated**: 2026-10-23

---

## Overview

This guide covers the workflow for implementing features in OpenShift after an enhancement proposal has been approved. The dev-guide contains authoritative implementation conventions.

## Implementation Workflow

```
1. Enhancement approved (status: implementable)
2. Create Jira Epic linked to enhancement tracking ticket
3. Define feature gate in openshift/api and vendor it into the component
4. Implement behind feature gate (TechPreviewNoUpgrade)
5. Write unit and integration tests
6. Submit PRs to component repo(s)
7. Add e2e tests to openshift/origin if needed
8. Update CI configuration in openshift/release
9. Graduate through feature gate stages
10. Update enhancement status to "implemented"
```

## Feature Gate Stages

| Stage | Audience | Stability | Upgrade Support |
|-------|----------|-----------|-----------------|
| **TechPreviewNoUpgrade** | Internal/early adopters | May change between releases | No upgrade path guaranteed |
| **Accessible-by-default** | All users (opt-in) | Stable API, feature accessible | Upgrade support required |
| **On-by-default** | All users (enabled) | Stable API, stable behavior | Full upgrade support required |

Feature gates are defined in [openshift/api](https://github.com/openshift/api) and must trace through: definition → runtime check → startup wiring. See [dev-guide/featuresets.md](../../dev-guide/featuresets.md).

## PR Conventions for OpenShift Repos

- PRs to OpenShift repos that use the Prow Jira plugin must reference a Jira bug
- PRs targeting release branches must include `[release-XX]` prefix in the title
- CI runs presubmit jobs defined in openshift/release; check results before requesting review

## New Component Checklist

For adding a new component to OpenShift, see [dev-guide/new-components.md](../../dev-guide/new-components.md). Key decisions:
- **Payload vs OLM**: Is this a core platform component (CVO-managed) or optional (OLM-managed)?
- **Release image**: Does this component need to be in the release payload?
- **CI configuration**: Add ci-operator config and jobs in openshift/release

## Development Phases

Feature implementation follows phased development as described in [dev-guide/development-phases.md](../../dev-guide/development-phases.md):
1. **Dev Preview** — feature available for evaluation
2. **Tech Preview** — feature available with limited support
3. **GA** — feature fully supported with upgrade guarantees

## Platform Documentation

For coding standards, see [CONVENTIONS.md](../../CONVENTIONS.md). For operator development, see [dev-guide/operators.md](../../dev-guide/operators.md). For API conventions, see [dev-guide/api-conventions.md](../../dev-guide/api-conventions.md).
