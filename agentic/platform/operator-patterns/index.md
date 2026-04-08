# Operator Patterns Index

**Last Updated**: 2026-04-08  

## Overview

Standard patterns used by all OpenShift operators. These patterns ensure consistency, reliability, and maintainability across the platform.

## Core Patterns

| Pattern | Purpose | File |
|---------|---------|------|
| **Status Conditions** | Report operator health to CVO | [status-conditions.md](./status-conditions.md) |
| **controller-runtime** | Reconciliation loop framework | [controller-runtime.md](./controller-runtime.md) |
| **Leader Election** | High availability for controllers | [leader-election.md](./leader-election.md) |
| **RBAC Patterns** | ServiceAccount and permissions | [rbac-patterns.md](./rbac-patterns.md) |
| **Finalizers** | Resource cleanup on deletion | [finalizers.md](./finalizers.md) |
| **Webhooks** | Admission control (validation/mutation) | [webhooks.md](./webhooks.md) |
| **Owner References** | Resource ownership and garbage collection | [owner-references.md](./owner-references.md) |
| **Upgrade Strategies** | Safe operator upgrades | [upgrade-strategies.md](./upgrade-strategies.md) |
| **must-gather** | Diagnostic data collection | [must-gather.md](./must-gather.md) |

## Pattern Relationships

```
controller-runtime
    ├── Uses: Leader Election (HA)
    ├── Uses: RBAC Patterns (permissions)
    ├── Integrates: Webhooks (admission control)
    └── Implements: Status Conditions (health reporting)

Finalizers + Owner References
    └── Complementary cleanup strategies
        ├── Finalizers: External resources
        └── Owner References: Kubernetes resources

Upgrade Strategies
    ├── Requires: Status Conditions (Upgradeable)
    └── Coordinates: CVO orchestration
```

## Usage Guidelines

**All ClusterOperators should**:
- ✅ Use controller-runtime for reconciliation
- ✅ Report status conditions (Available/Progressing/Degraded/Upgradeable)
- ✅ Implement must-gather for diagnostics
- ✅ Follow RBAC least privilege
- ✅ Use owner references for owned Kubernetes resources
- ✅ Use finalizers for external resource cleanup

**Optional patterns** (use when applicable):
- Leader election (multi-replica operators)
- Webhooks (validation/mutation requirements)
- Specific upgrade strategies (depends on workload type)

## Pattern Selection Guide

| Scenario | Recommended Patterns |
|----------|---------------------|
| **New operator from scratch** | controller-runtime + status-conditions + RBAC + must-gather |
| **Manages cloud resources** | + finalizers |
| **Creates Kubernetes resources** | + owner-references |
| **Multi-replica deployment** | + leader-election |
| **Custom validation needed** | + webhooks |
| **Critical infrastructure** | + upgrade-strategies (coordination) |

## Examples by Component

| Component | Patterns Used |
|-----------|--------------|
| **machine-config-operator** | All patterns (comprehensive operator) |
| **cluster-network-operator** | controller-runtime, status-conditions, finalizers, upgrade-strategies |
| **machine-api-operator** | controller-runtime, leader-election, finalizers, owner-references |
| **console-operator** | controller-runtime, status-conditions, webhooks, owner-references |

## Learning Path

**Beginner** (start here):
1. [controller-runtime.md](./controller-runtime.md) - Core reconciliation pattern
2. [status-conditions.md](./status-conditions.md) - Health reporting
3. [rbac-patterns.md](./rbac-patterns.md) - Permissions

**Intermediate**:
4. [owner-references.md](./owner-references.md) - Resource ownership
5. [webhooks.md](./webhooks.md) - Admission control
6. [leader-election.md](./leader-election.md) - High availability

**Advanced**:
7. [finalizers.md](./finalizers.md) - Cleanup logic
8. [upgrade-strategies.md](./upgrade-strategies.md) - Coordinated upgrades
9. [must-gather.md](./must-gather.md) - Diagnostics

## See Also

- **OpenShift Specifics**: [../openshift-specifics/](../openshift-specifics/)
- **Engineering Practices**: [../../practices/](../../practices/)
- **Domain Concepts**: [../../domain/](../../domain/)
