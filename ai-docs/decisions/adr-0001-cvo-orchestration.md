---
title: CVO as Upgrade Orchestrator
status: Accepted
date: 2019-01-01
affected_components:
  - cluster-version-operator
  - all-clusteroperators
---

# ADR 0001: CVO as Upgrade Orchestrator

## Status

**Accepted**

## Context

OpenShift needs a mechanism to orchestrate upgrades across dozens of operators and platform components. The system must handle ordering dependencies, version skew, and rollback in a distributed system.

## Decision

The Cluster Version Operator (CVO) serves as the centralized upgrade orchestrator. CVO applies manifests in lexicographic order by filename prefix (runlevels), manages operator lifecycle, and monitors health through ClusterOperator status conditions.

## Rationale

- **Deterministic ordering**: Runlevel-based manifest ordering provides predictable upgrade sequence
- **Health monitoring**: ClusterOperator conditions (Available/Progressing/Degraded) give CVO visibility into component health
- **Declarative**: Release payloads contain all manifests; CVO applies the desired state
- **Upgrade safety**: Upgradeable condition allows operators to block unsafe upgrades

## Alternatives Considered

### Distributed Coordination (No Central Orchestrator)
- **Pro**: No single point of failure for orchestration
- **Con**: Difficult to ensure ordering, hard to debug upgrade failures
- **Why not chosen**: Complexity of distributed coordination outweighed resilience benefits

### OLM-Based Orchestration
- **Pro**: Reuse existing OLM dependency resolution
- **Con**: OLM designed for optional operators, not platform-critical components
- **Why not chosen**: Platform operators need tighter control than OLM provides

## Consequences

**Positive**:
- Single place to monitor upgrade progress (`oc get clusterversion`)
- Operators have clear contract for upgrade participation (conditions)
- Deterministic upgrade ordering across releases

**Negative**:
- CVO is a critical single point of failure during upgrades
- All operators must implement ClusterOperator status reporting
- Runlevel changes require careful coordination across teams

## Affected Components

- **cluster-version-operator**: Implements orchestration logic
- **All ClusterOperators**: Must report status conditions per the CVO contract

## References

- **Implementation**: [cluster-version-operator](https://github.com/openshift/cluster-version-operator)
- **Dev Guide**: [CVO Operators Guide](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/operators.md)
- **Pattern**: [upgrade-strategies.md](../platform/openshift-specifics/upgrade-strategies.md)
- **Pattern**: [status-conditions.md](../platform/operator-patterns/status-conditions.md)
