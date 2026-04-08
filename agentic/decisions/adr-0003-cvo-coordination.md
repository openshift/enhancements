---
title: CVO Coordinates Operator Upgrades
status: Accepted
date: 2026-04-08
affected_components:
  - cluster-version-operator
  - All ClusterOperators
---

# ADR 0003: CVO Coordinates Operator Upgrades

## Status

**Accepted**

## Context

OpenShift clusters have 50+ operators that must be upgraded in a coordinated manner. Uncoordinated upgrades can cause downtime or data loss.

## Decision

Cluster Version Operator (CVO) coordinates all operator upgrades based on release payload and dependency graph.

## Rationale

- ✅ **Ordered upgrades**: Critical operators (API server) upgrade before dependents
- ✅ **Atomic rollback**: If upgrade fails, can rollback entire cluster
- ✅ **Version skew control**: Ensures compatible versions across operators
- ✅ **Single source of truth**: Release payload defines all operator versions
- ✅ **Observable progress**: ClusterVersion aggregates all operator status

## Alternatives Considered

### Independent Operator Upgrades

- **Pro**: Operators can upgrade independently, faster iteration
- **Con**: Version skew issues, coordination challenges
- **Con**: Risk of incompatible operator versions

### Manual Upgrade Ordering

- **Pro**: Full control over upgrade sequence
- **Con**: Error-prone, doesn't scale to 50+ operators
- **Con**: No enforcement of ordering

### Declarative Dependencies (DAG)

- **Pro**: Operators declare dependencies explicitly
- **Con**: Complex dependency resolution
- **Con**: Risk of circular dependencies

## Implementation

### Upgrade Ordering

CVO enforces upgrade order:

```
1. cluster-etcd-operator (foundation)
   ↓
2. kube-apiserver (API availability)
   ↓
3. kube-controller-manager
   ↓
4. kube-scheduler
   ↓
5. machine-config-operator (node updates)
   ↓
6. network-operator
   ↓
7. Other operators (parallel when safe)
```

### Release Payload

All operator versions in single payload:

```yaml
# release-4.16.0
apiVersion: image.openshift.io/v1
kind: ImageStream
spec:
  tags:
  - name: machine-config-operator
    from:
      kind: DockerImage
      name: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:abc
  - name: cluster-network-operator
    from:
      kind: DockerImage
      name: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:def
```

### Status Aggregation

CVO aggregates all ClusterOperator status:

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
status:
  conditions:
  - type: Available
    status: "True"  # All operators Available
  - type: Progressing
    status: "False"  # All operators done upgrading
  - type: Degraded
    status: "False"  # No operators Degraded
```

## Consequences

**Positive**:
- Predictable upgrades
- Reduced risk of version skew
- Single upgrade trigger for entire cluster
- Automatic rollback on failure

**Negative**:
- Operators cannot upgrade independently
- Slower upgrade cycle (coordinated releases)
- CVO is single point of failure for upgrades
- Must follow CVO ordering (can't skip)

## Operator Requirements

### Report Status

All operators must create ClusterOperator:

```go
co := &configv1.ClusterOperator{
    ObjectMeta: metav1.ObjectMeta{
        Name: "my-operator",
    },
    Status: configv1.ClusterOperatorStatus{
        Conditions: []configv1.ClusterOperatorStatusCondition{
            {
                Type:   configv1.OperatorAvailable,
                Status: configv1.ConditionTrue,
            },
        },
    },
}
```

### Respect Version

Operators must use version from release payload:

```go
// Read version from environment
version := os.Getenv("RELEASE_VERSION")

// Report in ClusterOperator
co.Status.Versions = []configv1.OperandVersion{
    {
        Name:    "operator",
        Version: version,
    },
}
```

### Block Unsafe Upgrades

Operators can block upgrades if unsafe:

```go
co.Status.Conditions = append(co.Status.Conditions,
    configv1.ClusterOperatorStatusCondition{
        Type:    configv1.OperatorUpgradeable,
        Status:  configv1.ConditionFalse,
        Reason:  "UnsafeConfiguration",
        Message: "Cannot upgrade with custom kernel parameters",
    },
)
```

## Affected Components

| Component | Role |
|-----------|------|
| **cluster-version-operator** | Coordinates upgrades |
| **All ClusterOperators** | Report status, follow CVO coordination |
| **Release engineering** | Builds coordinated release payload |
| **QE** | Tests complete upgrade path |

## Monitoring

```promql
# Track upgrade progress
cluster_version_operator_current_version_info

# Detect stuck upgrades
cluster_operator_conditions{type="Progressing", status="True"} > 1h

# Detect degraded operators
cluster_operator_conditions{type="Degraded", status="True"}
```

## References

- **CVO**: https://github.com/openshift/cluster-version-operator
- **Upgrade Process**: [../platform/operator-patterns/upgrade-strategies.md](../platform/operator-patterns/upgrade-strategies.md)
- **ClusterVersion**: [../domain/openshift/clusterversion.md](../domain/openshift/clusterversion.md)
