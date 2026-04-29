# Upgrade Strategies

**Category**: OpenShift-Specific Pattern  
**Applies To**: All ClusterOperators  
**Last Updated**: 2026-04-28  

## Overview

OpenShift upgrades are orchestrated by the Cluster Version Operator (CVO). Every operator must support N→N+1 version skew and coordinate with CVO for zero-downtime upgrades.

**Goal**: Upgrade 1000-node cluster with zero application downtime.

## Key Concepts

- **CVO Ordering**: etcd → kube-apiserver → kube-controller-manager → operators
- **Version Skew**: N→N+1 support (current and next minor version)
- **Progressing Condition**: Signals upgrade in progress
- **Upgradeable Condition**: Blocks upgrade if False
- **Rolling Updates**: Update nodes one at a time

## Upgrade Phases

| Phase | CVO Action | Operator Responsibility |
|-------|-----------|------------------------|
| **Pre-upgrade** | Check Upgradeable=True | Set Upgradeable=False if unsafe |
| **Upgrade** | Update operator image | Reconcile new version |
| **Rollout** | Wait for Progressing=False | Update workloads, report progress |
| **Post-upgrade** | Verify Available=True | Resume normal operation |

## CVO Orchestration

```
CVO upgrade order:
1. etcd operator
2. kube-apiserver operator
3. kube-controller-manager operator
4. kube-scheduler operator
5. All other operators (parallel)
```

**Why this order?**
- etcd must be healthy before API server updates
- API server must be upgraded before controllers (version skew)
- Operators depend on API server, so upgrade last

## Implementation

### Setting Upgradeable Condition

```go
func (r *Reconciler) checkUpgradeable(ctx context.Context) (bool, string, string) {
    // Check if upgrade would be unsafe
    
    // Example: Block if custom config is set
    if hasUnsupportedConfig() {
        return false, "UnsupportedConfig", 
            "Custom configuration detected, manual intervention required"
    }
    
    // Example: Block if degraded
    if isDegraded() {
        return false, "Degraded",
            "Component is degraded, fix issues before upgrading"
    }
    
    // Safe to upgrade
    return true, "AsExpected", "Ready for upgrade"
}

func (r *Reconciler) updateStatus(ctx context.Context, co *configv1.ClusterOperator) {
    upgradeable, reason, msg := r.checkUpgradeable(ctx)
    
    setCondition(&co.Status.Conditions,
        configv1.OperatorUpgradeable,
        boolToStatus(upgradeable),
        reason, msg)
}
```

### Handling Version Skew

```go
func (r *Reconciler) reconcile(ctx context.Context, obj *MyResource) error {
    // Get desired version from CVO
    desiredVersion := os.Getenv("RELEASE_VERSION")
    
    // Get current version from status
    currentVersion := obj.Status.Version
    
    if desiredVersion != currentVersion {
        // Upgrade in progress
        if err := r.upgradeWorkload(ctx, obj, desiredVersion); err != nil {
            return err
        }
        
        // Update status
        obj.Status.Version = desiredVersion
        setCondition(&obj.Status.Conditions,
            configv1.OperatorProgressing,
            configv1.ConditionTrue,
            "RollingOut",
            fmt.Sprintf("Upgrading to %s", desiredVersion))
    }
    
    return nil
}
```

### Rolling Update Pattern

```go
func (r *Reconciler) upgradeDeployment(ctx context.Context, desired *appsv1.Deployment) error {
    current := &appsv1.Deployment{}
    err := r.Get(ctx, client.ObjectKeyFromObject(desired), current)
    
    if err != nil {
        return err
    }
    
    // Update with RollingUpdate strategy
    desired.Spec.Strategy = appsv1.DeploymentStrategy{
        Type: appsv1.RollingUpdateDeploymentStrategyType,
        RollingUpdate: &appsv1.RollingUpdateDeployment{
            MaxUnavailable: &intstr.IntOrString{IntVal: 1},
            MaxSurge:       &intstr.IntOrString{IntVal: 1},
        },
    }
    
    // Set PodDisruptionBudget for HA components
    // (separate resource, not shown here)
    
    return r.Update(ctx, desired)
}
```

## Best Practices

1. **Always Support N→N+1**: Operator version X must work with X-1 and X+1
   - API compatibility (don't remove fields)
   - Schema changes must be backward compatible
   
2. **Use Progressing Condition**: Set to True during rollout
   ```yaml
   conditions:
   - type: Progressing
     status: "True"
     reason: RollingOut
     message: "Updating to version 4.16.0"
   ```

3. **Block Unsafe Upgrades**: Set Upgradeable=False if upgrade would break
   ```yaml
   conditions:
   - type: Upgradeable
     status: "False"
     reason: UnsupportedConfiguration
     message: "Remove custom config before upgrading"
   ```

4. **PodDisruptionBudgets**: Prevent all replicas going down
   ```yaml
   apiVersion: policy/v1
   kind: PodDisruptionBudget
   metadata:
     name: my-component
   spec:
     minAvailable: 1
     selector:
       matchLabels:
         app: my-component
   ```

5. **Graceful Shutdown**: Handle SIGTERM properly (30s default grace period)

## Version Skew Scenarios

### API Compatibility

```go
// ✅ Backward compatible (adding optional field)
type MyResourceSpec struct {
    Replicas int  `json:"replicas"`
    Image    string `json:"image"`
    NewField *string `json:"newField,omitempty"` // Optional, defaults to nil
}

// ❌ Not backward compatible (removing field)
type MyResourceSpec struct {
    Image string `json:"image"`
    // Replicas removed - breaks old clients!
}
```

### Storage Version Migration

```yaml
# Old version (v1alpha1)
apiVersion: example.com/v1alpha1
kind: MyResource
spec:
  field: value

# Hub version (v1)
apiVersion: example.com/v1
kind: MyResource
spec:
  newField: value

# Use conversion webhooks to handle both
```

## Node Upgrades

**Machine Config Operator (MCO) pattern**:

1. MCO creates new MachineConfig
2. Nodes reboot one at a time (drain → reboot → uncordon)
3. Max 1 node upgrading per pool at a time (configurable)

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfigPool
metadata:
  name: worker
spec:
  maxUnavailable: 1  # Only 1 node at a time
  paused: false
```

## Monitoring Upgrades

```bash
# Check upgrade progress
oc get clusterversion

# Watch operator status
oc get clusteroperators

# View progressing operators
oc get co -o json | jq '.items[] | select(.status.conditions[] | select(.type=="Progressing" and .status=="True")) | .metadata.name'
```

## Examples in Components

| Component | Upgrade Strategy | Notes |
|-----------|------------------|-------|
| etcd | One pod at a time, quorum maintained | Must maintain 2/3 quorum |
| kube-apiserver | Rolling update, PDB ensures 1+ available | Uses PodDisruptionBudget |
| machine-config-operator | Node drain → reboot → uncordon | Can take 30+ minutes per node |
| cluster-network-operator | Update CNI plugins in rolling fashion | Brief network interruptions possible |

## Antipatterns

❌ **Breaking API changes**: Removing fields in minor version  
❌ **All replicas down**: No PodDisruptionBudget for HA components  
❌ **Long rollouts**: Not reporting Progressing condition  
❌ **Ignoring Upgradeable**: Allowing unsafe upgrades to proceed  
❌ **Blocking CVO**: Long-running reconciliation preventing other operators from upgrading

## References

- **CVO**: [Cluster Version Operator](https://github.com/openshift/cluster-version-operator)
- **Enhancement**: [Upgrade Ordering](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/operators.md)
- **API**: [ClusterVersion](https://github.com/openshift/api/blob/master/config/v1/types_cluster_version.go)
- **Pattern**: Implements "Upgrade Safety" from [DESIGN_PHILOSOPHY.md](../../DESIGN_PHILOSOPHY.md)
