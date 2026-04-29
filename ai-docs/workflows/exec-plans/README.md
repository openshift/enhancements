# Exec-Plans: Feature Implementation Tracking

**Purpose**: Track multi-week feature implementation progress (Tier 1 guidance)  
**Last Updated**: 2026-04-29  

## Overview

Exec-plans are ephemeral documents that track feature implementation. They bridge the gap between enhancement (design) and implementation (code).

**Key Points**:
- Guidance lives here (Tier 1 platform docs)
- Actual exec-plans live in component repos (`agentic/exec-plans/active/`)
- Exec-plans are temporary → extract to permanent docs, then delete

## When to Use Exec-Plans

| Scenario | Use Exec-Plan? | Why |
|----------|---------------|-----|
| Multi-week feature (3+ PRs) | ✅ Yes | Track progress, coordinate PRs |
| Single PR feature | ❌ No | PR description sufficient |
| Bug fix | ❌ No | Issue tracker sufficient |
| Multi-component coordination | ✅ Yes | Track cross-repo dependencies |
| Experimental spike | ❌ No | Use spike branch, not exec-plan |

**Rule of thumb**: If you'll have 3+ PRs over 2+ weeks, use an exec-plan.

## Exec-Plan vs Enhancement

| Aspect | Enhancement | Exec-Plan |
|--------|-------------|-----------|
| **Purpose** | Design approval | Implementation tracking |
| **Location** | `enhancements/` (permanent) | `agentic/exec-plans/active/` (ephemeral) |
| **Scope** | What and why | How and when |
| **Lifetime** | Permanent (historical record) | Temporary (delete after completion) |
| **Audience** | Reviewers, future developers | Current implementation team |

**Relationship**: Enhancement = design spec, Exec-plan = implementation plan

## Structure

```
component-repo/
└── agentic/
    └── exec-plans/
        ├── active/
        │   ├── YYYY-MM-my-feature.md        # Active plans
        │   └── YYYY-MM-another-feature.md
        ├── archive/                          # Completed (or use git history)
        └── template.md                       # Copy this for new plans
```

## Template

See [template.md](template.md) for the full template.

**Core sections**:
1. **Overview**: What, why, timeline
2. **Implementation Tasks**: Checklist of PRs/work items
3. **Dependencies**: Blockers, cross-component coordination
4. **Progress Log**: Weekly updates
5. **Decisions**: Key technical decisions made during implementation
6. **Completion**: Outcome, follow-ups, knowledge extraction

## Workflow

### 1. Start: Create Exec-Plan

```bash
# Copy template
cp agentic/exec-plans/template.md agentic/exec-plans/active/2026-04-my-feature.md

# Fill in:
# - Overview (what, why, timeline)
# - Implementation tasks (checklist)
# - Dependencies
```

### 2. During: Update Progress

```bash
# Weekly updates:
# - Check off completed tasks
# - Add progress log entries
# - Document decisions
# - Update timeline if needed
```

### 3. Complete: Extract & Delete

```bash
# Extract knowledge:
# - Key decisions → ADR in decisions/
# - Architecture changes → Update architecture.md
# - Patterns → Update relevant pattern docs

# Then delete or archive exec-plan:
git rm agentic/exec-plans/active/2026-04-my-feature.md
# Or: mv active/2026-04-my-feature.md archive/
```

## Best Practices

1. **Keep Updated**: Update at least weekly (more often for active development)
   
2. **Check Off Tasks**: Visible progress motivates and informs stakeholders
   
3. **Document Decisions**: Capture "why" not just "what"
   - Why we chose approach X over Y
   - Why we changed direction mid-implementation
   
4. **Extract Knowledge**: Don't let decisions disappear
   - Significant decisions → ADR
   - Architecture changes → Permanent docs
   - Patterns → Platform or component guides
   
5. **Delete When Done**: Exec-plans are ephemeral
   - Extract value to permanent docs
   - Delete to avoid doc rot

## Example Timeline

```
Week 1:
  [x] Create exec-plan
  [x] PR #123: API changes
  [ ] PR #124: Controller logic

Week 2:
  [x] PR #124: Controller logic
  [ ] PR #125: E2E tests
  Decision: Changed approach from X to Y because...

Week 3:
  [x] PR #125: E2E tests
  [x] All PRs merged
  
Week 4:
  [x] Extract decisions to ADR
  [x] Update architecture docs
  [x] Delete exec-plan
```

## Integration with Other Docs

| Doc Type | Relationship | Example |
|----------|--------------|---------|
| **Enhancement** | Exec-plan implements enhancement | Enhancement: design, Exec-plan: tracking |
| **ADR** | Extract key decisions from exec-plan | "Why we chose etcd" → ADR |
| **Architecture** | Update with changes from exec-plan | New component → architecture.md |
| **Pattern Docs** | Extract patterns discovered | New pattern → pattern doc |

## Antipatterns

❌ **Permanent exec-plans**: Exec-plans should be deleted after completion  
❌ **No updates**: Exec-plan created but never updated (use PR description instead)  
❌ **Too detailed**: Line-by-line code plan (let PRs evolve)  
❌ **No extraction**: Delete without extracting decisions to permanent docs  
❌ **Single PR features**: Don't need exec-plan (PR description sufficient)

## Related

- **Template**: [template.md](template.md) - Copy this for new exec-plans
- **Enhancement Process**: [../enhancement-process.md](../enhancement-process.md)
- **Feature Implementation**: [../implementing-features.md](../implementing-features.md)
- **ADRs**: [../../decisions/](../../decisions/) - Where to extract key decisions
