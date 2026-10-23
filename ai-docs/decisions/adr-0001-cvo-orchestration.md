# ADR-0001: CVO as Root Upgrade Orchestrator

**Status**: Implemented
**Date**: 2026-10-23

---

## Context

OpenShift is composed of ~100 components (operators and operands) that must be installed and upgraded in a coordinated manner. The platform needs a single entity that:
- Knows the desired state of all platform components
- Can orchestrate ordered rollouts during upgrades
- Reports overall cluster health and upgrade progress
- Prevents partial upgrades from leaving the cluster in an inconsistent state

## Decision

The **Cluster Version Operator (CVO)** is the root operator for all OpenShift core components. It manages ClusterOperators by applying their manifests from the release payload in a defined order.

### Key Design Choices

1. **Runlevel-based ordering**: The CVO sweeps manifests alphabetically by payload filename and waits for the current runlevel to complete before advancing to the next. Components within the same runlevel may proceed in parallel
2. **Release image as source of truth**: The CVO reads manifests from the release image, providing versioned desired state. A rollout can remain partial if it encounters failures
3. **ClusterOperator status contract**: Each operator reports `Available`, `Progressing`, and `Degraded` conditions. The CVO waits for operators to become `Available` at each runlevel before proceeding
4. **Rollback is limited**: An administrator can attempt rollback only to a previous version within the current minor version, and the attempt may fail and leave the cluster unrecoverable

### CVO Does NOT:
- Manage OLM-installed operators (those use OLM's upgrade lifecycle)
- Make decisions about WHEN to upgrade (that's the admin's choice)
- Automatically roll back (the admin must initiate)

## Consequences

- All CVO-managed operators must report status via ClusterOperator conditions
- Operators must handle N-1 → N version skew during rolling upgrades
- The CVO blocks on operators that report `Available=False` during upgrades
- Operators can signal `Upgradeable=False` to block minor upgrades when preconditions are unmet; patch upgrades may still proceed

## Rationale

The alternatives considered were:
- **Declarative GitOps**: Too complex for a self-hosted platform where the control plane itself is being upgraded
- **OLM for everything**: OLM's dependency resolution adds latency and complexity unsuitable for core platform components
- **Manual ordering**: Error-prone, doesn't scale to ~100 components

The CVO approach provides a simple, reliable orchestration model that works even when the API server itself is being upgraded.

## References

- [dev-guide/cluster-version-operator/](../../dev-guide/cluster-version-operator/) — CVO developer documentation
- [openshift/cluster-version-operator](https://github.com/openshift/cluster-version-operator) — CVO source code
- [domain/openshift/clusterversion.md](../domain/openshift/clusterversion.md) — ClusterVersion resource documentation
