# Architectural Decision Records Index

**Last Updated**: 2026-04-08  

## Purpose

Cross-repo architectural decisions affecting multiple OpenShift components.

## Active ADRs

| ADR | Title | Status | Date | Affected Components |
|-----|-------|--------|------|---------------------|
| [adr-0001](./adr-0001-operator-sdk.md) | Use operator-sdk for New Operators | Accepted | 2026-04-08 | All new operators |
| [adr-0002](./adr-0002-etcd-backend.md) | Use etcd for Cluster State | Accepted | 2026-04-08 | All components |
| [adr-0003](./adr-0003-cvo-coordination.md) | CVO Coordinates Upgrades | Accepted | 2026-04-08 | All ClusterOperators |

## Template

See [adr-template.md](./adr-template.md) for creating new ADRs.

## When to Create ADR

Create ADR when:
- Decision affects >1 repository
- Significant architectural impact
- Alternatives were considered
- Rationale should be documented for future reference

**Don't create ADR for**:
- Component-specific decisions (use component's agentic/decisions/)
- Implementation details
- Temporary experiments
- Obvious technical choices

## ADR Lifecycle

```
Proposed → Review → Accepted → Implemented
                  ↓
                Rejected
```

**Superseded**: When a newer ADR replaces an old one

## Examples

### Platform-Level (Tier 1)

- Use etcd for state storage
- CVO coordinates upgrades
- operator-sdk for new operators

### Component-Level (Tier 2)

- MCO uses Ignition for configuration (in MCO repo)
- CNO chose OVN-Kubernetes over OpenShift SDN (in CNO repo)

## Creating New ADR

1. Copy the template file (see Template section above)
2. Number sequentially (next available number)
3. Fill in all sections
4. Create PR for review
5. After approval, merge and update this index

## See Also

- [Enhancement Process](../workflows/enhancement-process.md)
- [Platform Patterns](../platform/)
- [Repository Index](../references/repo-index.md)
