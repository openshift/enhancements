# Status Conditions Pattern

**Category**: Platform Pattern  
**Applies To**: All ClusterOperators, Recommended for all Operators  
**Last Updated**: 2026-04-28  

## Overview

Status conditions provide standardized health reporting for OpenShift components. Every ClusterOperator must report Available, Progressing, and Degraded conditions.

**Purpose**: Enable cluster-wide health monitoring and upgrade orchestration.

## Key Concepts

- **Condition**: Named state with Type, Status, Reason, Message
- **Available**: Component is functioning and serving requests
- **Progressing**: Component is reconciling changes (upgrade, config change)
- **Degraded**: Component is running but functionality is impaired
- **ObservedGeneration**: Detect spec changes requiring reconciliation

## Condition Types

### Required for ClusterOperators

| Type | Meaning | True When | False When |
|------|---------|-----------|------------|
| **Available** | Component is functional | API serving, pods ready | Pods not ready, API down |
| **Progressing** | Reconciliation in progress | Upgrade/rollout active | Desired state reached |
| **Degraded** | Impaired but functional | Partial failure, degraded mode | Fully healthy |

### Optional Conditions

| Type | Use Case |
|------|----------|
| **Upgradeable** | Blocks cluster upgrade if False |
| **Disabled** | Component intentionally disabled |

## Implementation

```go
import (
    configv1 "github.com/openshift/api/config/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Update condition helper
func setCondition(conditions *[]configv1.ClusterOperatorStatusCondition, 
                  condType configv1.ClusterStatusConditionType,
                  status configv1.ConditionStatus,
                  reason, message string) {
    now := metav1.Now()
    
    for i := range *conditions {
        if (*conditions)[i].Type == condType {
            // Update existing
            if (*conditions)[i].Status != status {
                (*conditions)[i].LastTransitionTime = now
            }
            (*conditions)[i].Status = status
            (*conditions)[i].Reason = reason
            (*conditions)[i].Message = message
            (*conditions)[i].LastHeartbeatTime = now
            return
        }
    }
    
    // Add new condition
    *conditions = append(*conditions, configv1.ClusterOperatorStatusCondition{
        Type:               condType,
        Status:             status,
        Reason:             reason,
        Message:            message,
        LastTransitionTime: now,
        LastHeartbeatTime:  now,
    })
}
```

## Best Practices

1. **Always Set All Three**: Available, Progressing, Degraded must always be set
   
2. **Heartbeat Updates**: Update LastHeartbeatTime even if condition unchanged
   - Shows controller is alive
   - Stale heartbeat triggers alerts
   
3. **Reason vs Message**:
   - **Reason**: Machine-readable CamelCase token (e.g., `PodsNotReady`)
   - **Message**: Human-readable sentence (e.g., "2 of 3 pods are not ready")
   
4. **ObservedGeneration**: Track spec changes
   ```go
   status.ObservedGeneration = obj.Generation
   ```

5. **Transition Timing**: LastTransitionTime changes only when Status changes

## Common Patterns

### Healthy State
```yaml
status:
  conditions:
  - type: Available
    status: "True"
    reason: AsExpected
    message: "All pods are ready"
  - type: Progressing
    status: "False"
    reason: AsExpected
    message: "Desired state reached"
  - type: Degraded
    status: "False"
    reason: AsExpected
    message: "Component is healthy"
```

### Upgrade in Progress
```yaml
status:
  conditions:
  - type: Available
    status: "True"
    reason: AsExpected
    message: "3 of 3 pods ready"
  - type: Progressing
    status: "True"
    reason: RollingOut
    message: "Rolling out new version 4.16.0"
  - type: Degraded
    status: "False"
    reason: AsExpected
    message: "Rollout is healthy"
```

### Degraded State
```yaml
status:
  conditions:
  - type: Available
    status: "True"
    reason: DegradedMode
    message: "Serving requests with reduced capacity"
  - type: Progressing
    status: "False"
    reason: AsExpected
    message: "No changes in progress"
  - type: Degraded
    status: "True"
    reason: PodCrashLooping
    message: "Pod api-server-xyz is crash looping, 1 of 3 replicas unavailable"
```

### Unavailable State
```yaml
status:
  conditions:
  - type: Available
    status: "False"
    reason: NoPodsReady
    message: "0 of 3 pods are ready"
  - type: Progressing
    status: "True"
    reason: Reconciling
    message: "Attempting to start pods"
  - type: Degraded
    status: "True"
    reason: AllPodsDown
    message: "Component is not serving requests"
```

## Decision Logic

```go
func computeConditions(deployment *appsv1.Deployment) []configv1.ClusterOperatorStatusCondition {
    available := false
    progressing := false
    degraded := false
    
    // Available: at least 1 pod ready
    if deployment.Status.AvailableReplicas > 0 {
        available = true
    }
    
    // Progressing: rollout in progress
    if deployment.Status.UpdatedReplicas < deployment.Status.Replicas {
        progressing = true
    }
    
    // Degraded: some pods not ready
    if deployment.Status.AvailableReplicas < deployment.Status.Replicas {
        degraded = true
    }
    
    return []configv1.ClusterOperatorStatusCondition{
        makeCondition(configv1.OperatorAvailable, available, ...),
        makeCondition(configv1.OperatorProgressing, progressing, ...),
        makeCondition(configv1.OperatorDegraded, degraded, ...),
    }
}
```

## Monitoring

```promql
# Alert on unavailable ClusterOperator
cluster_operator_conditions{type="Available", status="False"}

# Alert on prolonged Degraded
cluster_operator_conditions{type="Degraded", status="True"}

# Alert on stale heartbeat
time() - cluster_operator_conditions_last_heartbeat > 300
```

## Examples in Components

| Component | Typical States | Notes |
|-----------|---------------|-------|
| kube-apiserver | Available during rollout | Uses PodDisruptionBudget |
| machine-config-operator | Progressing during node updates | Can take 30+ minutes |
| cluster-network-operator | Degraded if SDN pods crash | Network still partially functional |

## Antipatterns

❌ **Skipping conditions**: All three (Available/Progressing/Degraded) must be set  
❌ **Stale status**: Not updating heartbeat regularly  
❌ **Unclear messages**: "Error occurred" (not useful) vs "Pod xyz crash looping: OOMKilled"  
❌ **Wrong semantics**: Available=False during normal upgrade (should be True)

## References

- **API**: [github.com/openshift/api/config/v1](https://github.com/openshift/api/blob/master/config/v1/types_cluster_operator.go)
- **Enhancement**: [cluster-operator-conditions](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/clusteroperator.md)
- **Command**: `oc get clusteroperators` to view all conditions
- **Pattern**: Implements "Observability by Default" from [DESIGN_PHILOSOPHY.md](../../DESIGN_PHILOSOPHY.md)
