# Reliability Practices

Reliability patterns and SLI/SLO/SLA definitions for OpenShift.

## Guidance

Reliability practices are primarily documented in:
- [../../dev-guide/](../../../dev-guide/) - Component readiness, release blockers
- [Status Conditions Pattern](../../platform/operator-patterns/status-conditions.md)

## SLI/SLO/SLA

| Term | Definition | Example |
|------|------------|---------|
| **SLI** | Service Level Indicator (metric) | API server 99th percentile latency |
| **SLO** | Service Level Objective (target) | p99 latency < 1s |
| **SLA** | Service Level Agreement (contract) | 99.95% uptime guarantee |

## Degraded Mode Patterns

| Pattern | Use Case | Example |
|---------|----------|---------|
| **Partial Availability** | Some replicas down | 2/3 pods ready → Available=True, Degraded=True |
| **Reduced Capacity** | Performance degraded | Serving requests but slower |
| **Read-Only Mode** | Write path broken | API reads work, writes fail |

## High Availability

| Pattern | When to Use | Reference |
|---------|-------------|-----------|
| **PodDisruptionBudget** | Prevent all replicas down | [PDB Docs](https://kubernetes.io/docs/tasks/run-application/configure-pdb/) |
| **Multiple Replicas** | Redundancy | 3+ replicas for HA |
| **Leader Election** | Single writer | [controller-runtime leader election](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/leaderelection) |

## Related

- **Dev Guide**: [../../dev-guide/component-readiness.md](../../../dev-guide/component-readiness.md)
- **Status Conditions**: [../../platform/operator-patterns/status-conditions.md](../../platform/operator-patterns/status-conditions.md)
- **Upgrade Strategies**: [../../platform/openshift-specifics/upgrade-strategies.md](../../platform/openshift-specifics/upgrade-strategies.md)
