# Implementing Features

**Purpose**: Guide developers from approved enhancement to merged code  
**Prerequisite**: Enhancement in `implementable` status  
**Last Updated**: 2026-10-23

## Overview

After an enhancement is approved, implementation follows a structured path through API changes, controller logic, testing, and documentation.

## Implementation Flow

```text
Enhancement Approved
    → API Changes (openshift/api)
    → Controller Implementation (component repo)
    → Tests (unit, integration, e2e)
    → Documentation
    → Release Notes
    → Enhancement Status Update
```

## Phase 1: API Changes

If the feature introduces or modifies APIs:

1. **Define types** in [openshift/api](https://github.com/openshift/api)
2. **Follow conventions**: See `./dev-guide/api-conventions.md`
3. **Generate code**: `make generate` after type changes
4. **Version appropriately**: alpha features use `v1alpha1`
5. **Update vendor**: Bump `openshift/api` dependency in component repo

```bash
# In openshift/api
# Edit types in the appropriate package
make generate
make verify
```

## Phase 2: Controller Logic

1. **Create or update controller** in the component repository
2. **Follow patterns**: Use [controller-runtime](../platform/operator-patterns/controller-runtime.md) or library-go as appropriate
3. **Add feature gate** if needed (alpha features gated by default)
4. **Report status**: Update [status conditions](../platform/operator-patterns/status-conditions.md)
5. **Handle cleanup**: Add [finalizers](../platform/operator-patterns/finalizers.md) if managing external resources

## Phase 3: Testing

Follow the [testing pyramid](../practices/testing/pyramid.md):

| Test Type | What to Test | When to Run |
|-----------|-------------|-------------|
| **Unit** | Business logic, edge cases | `make test-unit` |
| **Integration** | Controller reconciliation, API validation | `make test-integration` |
| **E2E** | Full feature workflow, upgrade paths | `make test-e2e` |

## Phase 4: Documentation & Release

1. **Update operator docs** in the component repository
2. **Add release notes** annotation to PRs
3. **Update enhancement status** to `implemented`
4. **Verify in CI**: Ensure all CI jobs pass

## Checklist

- [ ] API types defined and generated (if applicable)
- [ ] Controller logic implemented
- [ ] Feature gate added (if alpha)
- [ ] Unit tests cover core logic
- [ ] Integration tests cover reconciliation
- [ ] E2E tests cover user workflow
- [ ] Status conditions reported correctly
- [ ] RBAC updated for new resources
- [ ] must-gather updated to collect new resources
- [ ] Documentation updated
- [ ] Enhancement status updated to `implemented`

## Multi-Week Features

For features spanning multiple weeks, use [exec-plans](exec-plans/README.md) to track progress and coordinate across teams.

## References

- **Process**: [enhancement-process.md](enhancement-process.md) - How to write enhancements
- **API**: [api-evolution.md](../practices/development/api-evolution.md) - API versioning rules
- **Dev Guide**: `./dev-guide/` for development conventions
- **Component Docs**: Individual component repositories for implementation details
