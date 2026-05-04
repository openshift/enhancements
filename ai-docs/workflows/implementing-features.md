# Implementing Features

**Purpose**: Structured workflow from enhancement to production  
**Last Updated**: 2026-04-29  

## Overview

Feature implementation follows: Spec → Plan → Build → Test → Review → Ship

## Workflow Stages

| Stage | Input | Output | Duration |
|-------|-------|--------|----------|
| **1. Spec** | Enhancement proposal | Approved enhancement | 1-2 weeks |
| **2. Plan** | Enhancement | Implementation plan | 1-3 days |
| **3. Build** | Plan | Code + tests | 1-8 weeks |
| **4. Test** | Code | Passing CI | Continuous |
| **5. Review** | PR | Approved PR | 1-3 days |
| **6. Ship** | Merged PR | Production release | 1-4 weeks |

## Stage 1: Spec (Enhancement)

**Goal**: Get approval for feature design

**Checklist**:
- [ ] Enhancement proposal written (see [enhancement-process.md](enhancement-process.md))
- [ ] API design reviewed
- [ ] Risks and mitigations identified
- [ ] Test plan defined
- [ ] Graduation criteria set (DevPreview → TechPreview → GA)
- [ ] Enhancement PR merged

**Artifacts**: Merged enhancement in `enhancements/`

## Stage 2: Plan (Implementation Plan)

**Goal**: Break enhancement into implementable tasks

**Checklist**:
- [ ] Identify affected components
- [ ] List required API changes
- [ ] Define PR sequence (API → implementation → tests)
- [ ] Estimate timeline
- [ ] Identify dependencies
- [ ] Create exec-plan if multi-week (see [exec-plans/](exec-plans/))

**Artifacts**: Implementation plan (PR description or exec-plan doc)

**Example Plan**:
```markdown
## Implementation Plan

### PR 1: API Changes
- Add new CRD field `spec.newFeature`
- Update validation webhook
- Files: api/, webhook/

### PR 2: Controller Logic
- Implement reconciliation for new field
- Add unit tests
- Files: controllers/

### PR 3: E2E Tests
- Add upgrade test
- Add feature test
- Files: test/e2e/

Timeline: 3 weeks
Dependencies: None
```

## Stage 3: Build (Code + Tests)

**Goal**: Implement feature with tests

**Checklist**:
- [ ] API changes (CRD, types, validation)
- [ ] Controller logic (reconciliation)
- [ ] Unit tests (>80% coverage)
- [ ] Integration tests (API contracts)
- [ ] E2E tests (critical paths)
- [ ] Documentation (godoc, user-facing)
- [ ] Feature gate if TechPreview

**Test Pyramid**:
- 60% unit tests (fast, isolated)
- 30% integration tests (envtest)
- 10% e2e tests (full cluster)

See [../practices/testing/pyramid.md](../practices/testing/pyramid.md)

## Stage 4: Test (CI Validation)

**Goal**: Ensure tests pass in CI

**Checklist**:
- [ ] Unit tests pass (`make test-unit`)
- [ ] Integration tests pass (`make test-integration`)
- [ ] E2E tests pass (CI job)
- [ ] Linting passes (`make verify`)
- [ ] No flaky tests
- [ ] Coverage meets target

**Common CI Jobs**:
- `pull-ci-<org>-<repo>-<branch>-unit`
- `pull-ci-<org>-<repo>-<branch>-e2e-aws`
- `pull-ci-<org>-<repo>-<branch>-verify`

## Stage 5: Review (PR Review)

**Goal**: Get code review and approval

**Checklist**:
- [ ] PR description explains changes
- [ ] Tests included
- [ ] Documentation updated
- [ ] CHANGELOG entry added (if applicable)
- [ ] API review approved (if API changes)
- [ ] Code review approved (/lgtm)
- [ ] Required approvers approved (/approve)

**Review Areas**:
- Code quality (readability, maintainability)
- Test coverage (edge cases, error paths)
- API design (backward compatibility)
- Performance (no obvious regressions)
- Security (no vulnerabilities)

## Stage 6: Ship (Production Release)

**Goal**: Feature available in release

**Checklist**:
- [ ] PR merged to main branch
- [ ] Feature included in next release image
- [ ] Release notes written
- [ ] Documentation published
- [ ] Metrics dashboard created (if applicable)
- [ ] Alerts configured (if applicable)

**Graduation Path**:
```
DevPreview (4.16) → TechPreview (4.17) → GA (4.18)
```

## Feature States

| State | Meaning | Support Level | API Stability |
|-------|---------|--------------|---------------|
| **DevPreview** | Experimental | Best-effort | May change |
| **TechPreview** | Feature-complete | Supported (no SLA) | Mostly stable |
| **GA** | Production-ready | Fully supported | Stable |

## Multi-PR Strategy

**Pattern**: API → Implementation → Tests

```
PR 1 (API):
  - Add CRD fields
  - Update validation
  - Merge: Week 1

PR 2 (Implementation):
  - Add controller logic
  - Add unit tests
  - Depends on: PR 1
  - Merge: Week 2

PR 3 (E2E):
  - Add e2e tests
  - Depends on: PR 2
  - Merge: Week 3
```

**Benefits**:
- Smaller, easier-to-review PRs
- Incremental progress
- Easier to revert if needed

## Rollout Strategy

| Strategy | Use Case | Example |
|----------|----------|---------|
| **Feature Gate** | TechPreview features | `FeatureGates: [MyFeature]` |
| **Operator Flag** | Opt-in features | `--enable-my-feature=true` |
| **Progressive Rollout** | Gradual activation | Enable 10% → 50% → 100% |
| **API Version** | Breaking changes | v1alpha1 → v1beta1 → v1 |

## Monitoring Post-Ship

**Checklist**:
- [ ] Monitor error rates (Prometheus)
- [ ] Check for support cases (Jira)
- [ ] Watch for regression bugs
- [ ] Verify upgrade scenarios work
- [ ] Gather user feedback

## Common Pitfalls

❌ **No test plan**: Feature works locally but breaks in CI  
❌ **Missing upgrade path**: Old clusters break on upgrade  
❌ **Insufficient testing**: Only happy path tested  
❌ **No feature gate**: Can't disable if bugs found  
❌ **Unclear PR sequence**: Dependencies not obvious

## Examples

| Feature | PRs | Timeline | Notes |
|---------|-----|----------|-------|
| Machine API | 5 PRs | 8 weeks | API, controller, provider, tests |
| ClusterVersion | 3 PRs | 4 weeks | API, CVO logic, e2e |
| Feature Gates | 2 PRs | 2 weeks | Implementation, docs |

## Related

- **Enhancement Process**: [enhancement-process.md](enhancement-process.md)
- **Exec Plans**: [exec-plans/](exec-plans/) - Track multi-week features
- **Testing**: [../practices/testing/pyramid.md](../practices/testing/pyramid.md)
- **API Evolution**: [../practices/development/api-evolution.md](../practices/development/api-evolution.md)
