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

- [ADR-0001: CVO Orchestrates Cluster Upgrades](adr-0001-cvo-orchestration.md) — Why a single centralized operator orchestrates all upgrades via runlevel ordering
- [ADR-0002: Immutable Node Infrastructure](adr-0002-immutable-nodes.md) — Why RHCOS + rpm-ostree + Ignition + MachineConfig instead of traditional package management
- [ADR-0003: Standardized Status Conditions Pattern](adr-0003-status-conditions-pattern.md) — Why Available/Progressing/Degraded as the uniform operator health contract

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
