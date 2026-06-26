# ADR-0003: Standardized Status Conditions Pattern

**Status**: Accepted  
**Date**: 2026-06-24  
**Scope**: Cross-repository  

## Context

OpenShift platform operators report health and progress through ClusterOperator resources. The CVO reads these status reports to make upgrade decisions — determining when a component has finished upgrading, whether the cluster is healthy enough to proceed, and whether a minor upgrade should be allowed.

Without a standardized condition scheme:
- Each operator would report status differently, making cluster-wide health assessment impossible to automate
- The CVO would need per-operator logic to interpret health signals
- Administrators would have no consistent way to diagnose operator problems across the platform
- Monitoring and alerting could not use uniform queries across all operators

## Decision

All ClusterOperators must report three required status conditions: **Available**, **Progressing**, and **Degraded**. Two additional optional conditions are defined: **Upgradeable** and **EvaluationConditionsDetected**.

### Required conditions

**Available**
- `True`: The component (operator and all configured operands) is functional and available in the cluster
- `False`: At least part of the component is non-functional and requires immediate administrator intervention
- A component must **not** report `Available=False` during the course of a normal upgrade

**Progressing**
- `True`: The component is actively rolling out new code, propagating config changes, or moving from one steady state to another
- `False`: The component has reached its desired state; no changes are in progress
- Should not report `Progressing=True` for normal reconciliation without action, or for resource adjustments from node scaling
- A component in a cluster with fewer than 250 nodes must complete a version change within 20 minutes (90 minutes for MCO, which must restart control plane nodes)

**Degraded**
- `True`: The component does not match its desired state over a period of time, resulting in lower quality of service
- `False`: The component is fully healthy
- Represents a **persistent** observation — should not oscillate in and out of Degraded state
- A component must **not** report `Degraded=True` during the course of a normal upgrade
- A component may be both Available and Degraded (e.g., 3 replicas desired, 1 crash-looping — serving requests but impaired)

### Optional conditions

**Upgradeable**
- `False`: The cluster-version operator will prevent minor OpenShift updates (patch updates are not blocked)
- The message field should explain what the administrator must do to unblock the upgrade
- Missing, `True`, or `Unknown` all allow upgrades to proceed

**EvaluationConditionsDetected**
- Indicates the result of detection logic evaluating the introduction of an invasive change that could cause alerts, breakages, or upgrade failures

### CVO upgrade decision table

| Operation | Version | Available | Degraded | Progressing | Upgradeable |
|-----------|---------|-----------|----------|-------------|-------------|
| Install completion | any | true | any | any | any |
| Begin patch upgrade | any | any | any | any | any |
| Begin minor upgrade | any | any | any | any | not false |
| Upgrade completion | target version (declared in operator status) | true | false | any | any |

Install flattens inter-runlevel barriers (components across different runlevels can start in parallel, though intra-component ordering is preserved). Upgrade will not proceed to the next runlevel until the previous runlevel completes (Available=true AND Degraded=false AND target version declared in operator status).

### Message conventions

- **Reason**: Machine-readable CamelCase (e.g., `AsExpected`, `PodsNotReady`, `RollingOut`)
- **Available message**: Single sentence without punctuation describing what is available
- **Progressing message**: Terse, 5–10 words describing current state (shown as default column in CLI)
- **Degraded message**: Detailed (a few sentences at most) explanation sufficient for triage
- All messages start with a capital letter
- Operators should set reason and message for both happy and sad conditions (e.g., `AsExpected` / `All is well` for healthy state)

### Example

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterOperator
metadata:
  name: authentication
status:
  conditions:
  - type: Available
    status: "True"
    reason: AsExpected
    message: "Cluster has deployed 4.17.0"
  - type: Progressing
    status: "False"
    reason: AsExpected
    message: "Cluster version is 4.17.0"
  - type: Degraded
    status: "False"
    reason: AsExpected
```

## Rationale

- **Three orthogonal concerns**: Available answers "can you use it now?", Progressing answers "is something changing?", Degraded answers "is there a persistent problem?" This captures all meaningful operator states without an explosion of condition types.
- **CVO automation**: The CVO can make upgrade decisions using a simple, uniform table across all operators without per-operator interpretation logic.
- **Consistent observability**: Administrators and monitoring systems use the same condition types across all operators. Prometheus queries like `cluster_operator_conditions{condition="Degraded"}` work uniformly.
- **Explicit happy reasons** (`AsExpected`) enable aggregation queries that distinguish healthy from unhealthy without mixing in time-series values.

## Consequences

### Positive

- Cluster-wide health is assessable through a single `oc get clusteroperators` command
- Upgrade decisions are automated and consistent
- Monitoring and alerting use uniform queries across all platform operators
- New operators get a clear contract for status reporting

### Negative

- The three-condition model may not capture every nuance of an operator's state — operators sometimes add custom conditions for component-specific signals
- Time bounds on Progressing (20 minutes / 90 minutes) can trigger false alerts if an operator's reconciliation is legitimately slow
- The prohibition against reporting Degraded during normal upgrades requires operators to distinguish upgrade-related transient errors from genuine degradation

### Neutral

- The pattern is specific to ClusterOperator resources; individual operator CRDs may use different condition types for their own operands

## Alternatives Considered

### Alternative 1: Free-form status fields

**Description**: Each operator reports status using whatever fields and values make sense for its domain (e.g., `.status.phase`, `.status.health`, free-text `.status.message`).

**Pros**:
- Maximum flexibility for each operator
- No need to agree on shared semantics

**Cons**:
- CVO cannot automate upgrade decisions without per-operator parsing logic
- Administrators must learn each operator's status conventions
- Monitoring cannot use uniform queries

**Rejected because**: The value of standardized conditions grows with the number of operators. OpenShift's platform has dozens of ClusterOperators — free-form status would make automated upgrade orchestration and cluster-wide health assessment impractical.

### Alternative 2: Kubernetes-native conditions without standardization

**Description**: Use the Kubernetes `conditions` array but without requiring specific condition types. Each operator defines its own condition types.

**Pros**:
- Uses standard Kubernetes API conventions
- Operators can define domain-specific conditions

**Cons**:
- No guaranteed condition types means CVO cannot rely on any specific condition existing
- Cluster-wide queries require knowing each operator's condition vocabulary
- No uniform contract for upgrade readiness

**Rejected because**: The Kubernetes conditions API provides the mechanism but not the semantics. Standardizing on Available/Progressing/Degraded gives the CVO and administrators a guaranteed, uniform contract while still allowing operators to add custom conditions alongside the required three.

## References

- Dev guide: [ClusterOperator conditions](../../dev-guide/cluster-version-operator/dev/clusteroperator.md)
- AI docs: [Status conditions pattern](../platform/operator-patterns/status-conditions.md)
- AI docs: [ClusterOperator resource](../domain/openshift/clusteroperator.md)
- AI docs: [Design Philosophy — Observability by Default](../DESIGN_PHILOSOPHY.md)
- API: [ClusterStatusConditionType](https://pkg.go.dev/github.com/openshift/api/config/v1#ClusterStatusConditionType)
