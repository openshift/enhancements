# Architectural Decision Records (ADRs)

Cross-repository architectural decisions that shape OpenShift platform.

## Purpose

ADRs document significant decisions affecting multiple components. They explain:
- What decision was made
- Why it was made
- What alternatives were considered
- Consequences and trade-offs

## Template

Use [adr-template.md](adr-template.md) for new ADRs.

## Scope

**Include here** (cross-repo decisions):
- Platform-wide patterns (why etcd, why CVO orchestration)
- API design principles (why status conditions pattern)
- Architectural constraints (why immutable nodes)

**Exclude** (component-specific):
- Component implementation details (belongs in component repo)
- Technology choices for single component
- Temporary decisions

## ADR List

**Note**: This is a starter list. Create ADRs as needed for cross-repo decisions.

Proposed ADRs:
- `adr-0001-cvo-orchestration.md` - Why CVO orchestrates upgrades
- `adr-0002-immutable-nodes.md` - Why RHCOS uses immutable infrastructure
- `adr-0003-status-conditions-pattern.md` - Why Available/Progressing/Degraded

## Creating ADRs

```bash
# Copy template
cp ai-docs/decisions/adr-template.md ai-docs/decisions/adr-0001-my-decision.md

# Fill in:
# - Title, Date, Status
# - Context, Decision, Consequences
# - Alternatives considered

# Add to this index

# Create PR
```

## ADR Numbering

- Use 4-digit numbers with leading zeros: `adr-0001-`
- Sequential numbering (next available number)
- Include slug in filename: `adr-0001-topic-name.md`

## ADR Status

| Status | Meaning |
|--------|---------|
| **Proposed** | Under discussion |
| **Accepted** | Decision made, being implemented |
| **Deprecated** | Superseded by another ADR |
| **Superseded** | Replaced (link to new ADR) |

## Related

- **Pattern**: [Architectural Decision Records](https://adr.github.io/)
- **Enhancement Process**: [../workflows/enhancement-process.md](../workflows/enhancement-process.md)
