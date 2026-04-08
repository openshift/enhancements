# ClusterOperator

**Type**: OpenShift Platform API  
**API Group**: `config.openshift.io/v1`  
**Last Updated**: 2026-04-08  

## Overview

ClusterOperator is how operators report status to CVO (Cluster Version Operator). Each operator creates one ClusterOperator resource.

## API Structure

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterOperator
metadata:
  name: machine-config
spec: {}  # Always empty
status:
  conditions:
  - type: Available
    status: "True"
    reason: "AsExpected"
    message: "All components running"
  - type: Progressing
    status: "False"
    reason: "AsExpected"
  - type: Degraded
    status: "False"
    reason: "AsExpected"
  versions:
  - name: operator
    version: 4.16.0
  relatedObjects:
  - group: machineconfiguration.openshift.io
    resource: machineconfigpools
    name: worker
```

## Status Conditions

| Condition | Meaning |
|-----------|---------|
| **Available** | Operator is functional |
| **Progressing** | Upgrade or rollout in progress |
| **Degraded** | Partial functionality |
| **Upgradeable** | Safe to upgrade (optional) |

See [../../platform/operator-patterns/status-conditions.md](../../platform/operator-patterns/status-conditions.md)

## Lifecycle

1. **Operator starts** → Creates ClusterOperator
2. **Initializing** → Available=False, Progressing=True
3. **Running** → Available=True, Progressing=False
4. **Upgrading** → Progressing=True
5. **Degraded** → Degraded=True (but may still be Available)

## Examples

| Component | ClusterOperator Name | Purpose |
|-----------|---------------------|---------|
| machine-config-operator | machine-config | Node configuration |
| cluster-network-operator | network | SDN/OVN networking |
| cluster-version-operator | version | Cluster version |

## References

- **Pattern**: [../../platform/operator-patterns/status-conditions.md](../../platform/operator-patterns/status-conditions.md)
- **Lifecycle**: [../../platform/openshift-specifics/operator-lifecycle.md](../../platform/openshift-specifics/operator-lifecycle.md)
- **API**: https://github.com/openshift/api/blob/master/config/v1/types_cluster_operator.go
