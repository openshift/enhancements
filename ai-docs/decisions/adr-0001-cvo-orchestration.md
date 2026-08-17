# ADR-0001: CVO Orchestrates Cluster Upgrades

**Status**: Accepted  
**Date**: 2026-06-24  
**Scope**: Cross-repository  

## Context

OpenShift clusters contain dozens of operators and platform components that must be upgraded together when moving between versions. These components have ordering dependencies: for example, kube-apiserver must be updated before kube-controller-manager, and core API servers must be available before higher-level operators can reconcile.

The upgrade process must:
- Handle component interdependencies safely
- Complete control plane upgrades within a reasonable time period (30m–1h)
- Avoid disrupting running application workloads
- Be resilient to failures, including node reboots and pod evictions mid-upgrade
- Enforce N-1 minor version compatibility between components

## Decision

The Cluster Version Operator (CVO) is the single, centralized orchestrator for all cluster upgrades. It installs and updates all "second-level" operators by applying their manifests from the release image in a deterministic order.

### Ordering model

The CVO loads manifests from the `/release-manifests` directory within the release image. During upgrades, manifests are applied in lexicographic order using a runlevel convention:

```
0000_<runlevel>_<component-name>_<manifest-filename>
```

Assigned runlevels (from [CVO dev guide: operators.md](../../dev-guide/cluster-version-operator/dev/operators.md) and [upgrades.md](../../dev-guide/cluster-version-operator/dev/upgrades.md)):

| Runlevel | Components |
|----------|-----------|
| 00-04 | CVO itself |
| 10-29 | Kubernetes operators (cluster-config at 10; etcd, kube-apiserver, kube-controller-manager, kube-scheduler at 20+) |
| 30-39 | Machine API |
| 50 | Non-order-specific operators (default runlevel; includes OLM, monitoring, samples, service-ca, machine-approver) |
| 60-69 | OpenShift core operators (openshift-apiserver, etc.) |
| 70 | Disruptive node-level components (DNS, network/SDN, multus) |
| 80 | Machine operators |

**Note**: The [operators.md](../../dev-guide/cluster-version-operator/dev/operators.md) dev guide contains stale runlevel assignments for several operators. The table above reflects actual release image manifest prefixes, which differ from operators.md for Network/DNS (70 not 07/08), cluster-config (10 not 05), and service-ca/machine-approver (50 not 09).

Components sharing the same runlevel run in parallel (e.g., `0000_50_cluster-monitoring-operator_*` and `0000_50_cluster-samples-operator_*` execute concurrently). Within a component, manifests apply in lexicographic order.

Install flattens inter-runlevel barriers (components across different runlevels can start in parallel), but intra-component ordering is preserved (e.g., a CRD manifest is applied before the Deployment within the same component). Upgrades apply strict runlevel ordering — the next runlevel does not start until the previous one completes.

### Upgrade completion criteria

The CVO uses ClusterOperator status conditions to determine when each component has finished upgrading:

| Operation | Version | Available | Degraded | Progressing | Upgradeable |
|-----------|---------|-----------|----------|-------------|-------------|
| Install completion | any | true | any | any | any |
| Begin patch upgrade | any | any | any | any | any |
| Begin minor upgrade | any | any | any | any | not false |
| Begin upgrade (w/ force) | any | any | any | any | any |
| Upgrade completion | target version (declared in operator status) | true | false | any | any |

### Self-managing design

The CVO is itself a regular pod in the cluster. During upgrades, when the MCO updates the operating system or when the release image updates the CVO image, the CVO gets drained and restarted like any other pod. The new CVO pod observes current cluster state and resumes reconciliation — there is no special handoff mechanism.

This is intentional: by not special-casing its own upgrade, the CVO restart works the same way as it would after a kernel panic, hardware failure, or network partition. The "normal" code path and the "exceptional" path are identical, keeping the upgrade process robust and continuously tested.

## Rationale

- **Deterministic ordering** prevents subtle failures from components upgrading before their prerequisites are ready. Operators that lack sophistication about detecting their own prerequisites get safety from runlevel ordering.
- **Centralized enforcement** means upgrade safety invariants (version gating, Upgradeable condition checks, override blocking) are enforced in one place rather than distributed across every operator.
- **Self-managing resilience** ensures the upgrade process is robust against any disruption — the same reconciliation logic handles both normal restarts and failure recovery.
- **Parallel execution within runlevels** keeps upgrade times reasonable while preserving safety between runlevels.

## Consequences

### Positive

- All component upgrades follow a single, predictable ordering model
- Upgrade failures are observable through ClusterVersion and ClusterOperator conditions
- The system self-heals after any disruption during upgrade
- N-1 version compatibility is enforced across all components

### Negative

- Adding a new component to the payload requires choosing a runlevel and understanding the ordering implications
- The conservative ordering is sometimes stricter than necessary — most components tolerate version skew during upgrades, but ordering is kept conservative for edge cases
- Node upgrades currently proceed in the same sequence as control plane upgrades; future improvements may allow independent node upgrade scheduling

## Alternatives Considered

### Alternative 1: OLM-managed upgrades

**Description**: Use the Operator Lifecycle Manager to coordinate all operator upgrades through its dependency resolution system.

**Rejected because**: OLM is itself a second-level operator managed by CVO (runlevel 50–59). It manages optional/add-on operators, not the platform itself. Platform components need to be available before OLM can function, creating a bootstrap dependency that cannot be resolved from within OLM.

### Alternative 2: Distributed coordination

**Description**: Each operator independently detects when its prerequisites are ready and upgrades itself, using leader election and CRD-based signaling.

**Rejected because**: Distributing upgrade coordination across dozens of operators would require every operator to implement prerequisite detection correctly. The CVO's bias toward predictable ordering exists precisely because "operators that lack sophistication about detecting their prerequisites" benefit from centralized ordering. A distributed model trades simplicity for flexibility that is rarely needed.

### Alternative 3: Manual operator ordering

**Description**: Administrators manually trigger component upgrades in the correct order.

**Rejected because**: OpenShift 4's design goal is that clusters "self-manage" by default. Manual ordering is error-prone, scales poorly, and conflicts with the operator-driven automation model.

## References

- Dev guide: [Upgrades and order](../../dev-guide/cluster-version-operator/dev/upgrades.md)
- Dev guide: [Operator integration with CVO](../../dev-guide/cluster-version-operator/dev/operators.md)
- Dev guide: [ClusterOperator conditions](../../dev-guide/cluster-version-operator/dev/clusteroperator.md)
- AI docs: [ClusterVersion resource](../domain/openshift/clusterversion.md)
- AI docs: [Upgrade strategies](../platform/openshift-specifics/upgrade-strategies.md)
