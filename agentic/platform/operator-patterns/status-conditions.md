# Operator Status Conditions Pattern

**Category**: Platform Pattern  
**Applies To**: All ClusterOperators  
**Last Updated**: 2026-04-08  

## Overview

All ClusterOperators report status using Available/Progressing/Degraded conditions to communicate health to the Cluster Version Operator (CVO).

## Condition Types

| Condition | Meaning | When to Set |
|-----------|---------|-------------|
| Available | Operator functional | Reconciliation successful, all pods running |
| Progressing | Operator updating | During rollouts, config changes, version upgrades |
| Degraded | Operator failing | Errors, unable to reconcile, dependency failures |
| Upgradeable | Safe to upgrade | Prerequisites met, no blocking conditions |

## Implementation

```go
import "github.com/openshift/library-go/pkg/operator/v1helpers"

v1helpers.SetOperatorCondition(&operatorStatus.Conditions,
    operatorv1.OperatorCondition{
        Type:   operatorv1.OperatorStatusTypeAvailable,
        Status: operatorv1.ConditionTrue,
        Reason: "AsExpected",
        Message: "All components running",
    })
```

## Best Practices

1. **Set all conditions**: Always maintain Available, Progressing, Degraded
2. **Meaningful reasons**: Use descriptive reason strings (CamelCase)
3. **Actionable messages**: Help users understand what's wrong and how to fix it
4. **Transition accuracy**: Update status when state changes, not just periodically
5. **Avoid flip-flopping**: Don't toggle conditions rapidly (use grace periods)

## Common Condition Patterns

### Normal Operation
```yaml
conditions:
- type: Available
  status: "True"
  reason: AsExpected
- type: Progressing
  status: "False"
  reason: AsExpected
- type: Degraded
  status: "False"
  reason: AsExpected
```

### During Upgrade
```yaml
conditions:
- type: Available
  status: "True"
  reason: AsExpected
- type: Progressing
  status: "True"
  reason: RollingOut
  message: "Rolling out new version 4.15.0"
- type: Degraded
  status: "False"
  reason: AsExpected
```

### Degraded State
```yaml
conditions:
- type: Available
  status: "True"
  reason: Degraded
  message: "Partial functionality available"
- type: Progressing
  status: "False"
  reason: AsExpected
- type: Degraded
  status: "True"
  reason: NodeConfigFailed
  message: "3/10 nodes failed to apply configuration: permission denied on /etc/crio/crio.conf"
```

## Condition Lifecycle

```
Initial → Progressing (True) → Available (True), Progressing (False)
       ↓
       → Degraded (True) → Fix applied → Degraded (False), Available (True)
```

## Examples in Components

| Component | Implementation | Notes |
|-----------|---------------|-------|
| machine-config-operator | pkg/operator/status.go | Sets Degraded when nodes fail to apply configs |
| cluster-version-operator | pkg/cvo/status.go | Aggregates all operator statuses |
| cluster-network-operator | pkg/operator/status.go | Sets Progressing during SDN→OVN migration |
| machine-api-operator | pkg/operator/status.go | Reports machine provisioning failures |

## Debugging Status Conditions

```bash
# Check operator status
oc get clusteroperator <name> -o yaml

# Watch for condition changes
oc get clusteroperator <name> -w

# View condition history
oc describe clusteroperator <name>

# Check all operators
oc get clusteroperators
```

## Common Pitfalls

1. **Not setting Progressing=False**: Operators stuck in "Progressing" forever
2. **Generic messages**: "Error occurred" vs "Failed to apply MachineConfig: permission denied"
3. **Missing Upgradeable**: CVO can't determine if upgrade is safe
4. **Rapid toggles**: Conditions changing every few seconds (add hysteresis)

## References

- **Library-go**: https://github.com/openshift/library-go/pkg/operator
- **CVO Status Aggregation**: [cluster-operators.md](../openshift-specifics/cluster-operators.md)
- **ClusterOperator API**: [clusteroperator.md](../../domain/openshift/clusteroperator.md)
