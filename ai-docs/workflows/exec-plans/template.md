# Exec-Plan: [Feature Name]

**Status**: Active | Blocked | Completed  
**Start Date**: YYYY-MM-DD  
**Target Completion**: YYYY-MM-DD  
**Owner**: @username  
**Related Enhancement**: [Link to enhancement](../../enhancements/my-feature.md)  

---

## Overview

### What
Brief description of what this feature does (1-2 sentences).

### Why
Why we're building this (link to enhancement for details).

### Timeline
- **Start**: YYYY-MM-DD
- **Target**: YYYY-MM-DD
- **Status**: On track | At risk | Delayed

---

## Implementation Tasks

### Phase 1: API Changes
- [ ] Define CRD schema (PR #XXX)
- [ ] Add validation webhook (PR #XXX)
- [ ] Update API types (PR #XXX)
- [ ] API review approved

**Target**: Week 1

### Phase 2: Controller Implementation
- [ ] Implement reconciliation logic (PR #XXX)
- [ ] Add unit tests (>80% coverage)
- [ ] Add integration tests
- [ ] Handle error cases

**Target**: Week 2-3

### Phase 3: Testing & Documentation
- [ ] E2E tests for critical paths (PR #XXX)
- [ ] Upgrade/downgrade tests
- [ ] Documentation (user-facing)
- [ ] Metrics and alerts

**Target**: Week 4

---

## Dependencies

### Blockers
- [ ] Dependency 1: Description (blocked by #XXX)
- [ ] Dependency 2: Description (waiting on team Y)

### Cross-Component Coordination
- [ ] Component A: API contract defined
- [ ] Component B: Integration tested

---

## Progress Log

### YYYY-MM-DD (Week 4)
**Progress**:
- Completed PR #125 (E2E tests)
- All PRs merged

**Next**:
- Extract decisions to ADR
- Delete exec-plan

### YYYY-MM-DD (Week 3)
**Progress**:
- Completed PR #124 (controller logic)
- Started PR #125 (E2E tests)

**Blockers**: None

**Next**:
- Finish E2E tests
- Final review

### YYYY-MM-DD (Week 2)
**Progress**:
- PR #123 merged (API changes)
- Started controller implementation

**Decisions**:
- Changed from approach X to Y because... (extract to ADR)

**Next**:
- Complete controller logic
- Add integration tests

### YYYY-MM-DD (Week 1)
**Progress**:
- Created exec-plan
- Opened PR #123 (API changes)

**Next**:
- Complete API review
- Start controller implementation

---

## Decisions

### Decision 1: [Title]
**Date**: YYYY-MM-DD  
**Decision**: What we decided  
**Rationale**: Why we decided this  
**Alternatives**: What we considered  
**Status**: Extract to ADR: [ ] Yes [x] No  

### Decision 2: [Title]
**Date**: YYYY-MM-DD  
**Decision**: ...  
**Rationale**: ...  
**Status**: Extract to ADR: [x] Yes [ ] No  

---

## Completion Checklist

### Code
- [ ] All PRs merged
- [ ] Tests passing in CI
- [ ] Documentation updated
- [ ] Feature gate configured (if TechPreview)

### Knowledge Extraction
- [ ] Key decisions extracted to ADRs
- [ ] Architecture docs updated
- [ ] Patterns documented (if new patterns emerged)
- [ ] Lessons learned captured

### Cleanup
- [ ] Exec-plan archived or deleted
- [ ] Related issues closed
- [ ] Team notified of completion

---

## Outcome

**Status**: [Completed | Deferred | Cancelled]  
**Completion Date**: YYYY-MM-DD  

### What Shipped
- Feature X in version Y.Z
- PRs: #123, #124, #125

### Follow-Ups
- [ ] Follow-up 1: Description (issue #XXX)
- [ ] Follow-up 2: Description (issue #XXX)

### Knowledge Extracted
- ADR: [Link to decision doc]
- Architecture update: [Link]
- Pattern doc: [Link]

---

**Note**: This exec-plan is ephemeral. After completion, extract key decisions and delete this file.
