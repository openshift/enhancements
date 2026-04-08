# Upgrade Strategies Pattern

**Category**: Platform Pattern  
**Applies To**: All ClusterOperators  
**Last Updated**: 2026-04-08  

## Overview

Upgrade strategies ensure safe, coordinated updates across the OpenShift cluster. The Cluster Version Operator (CVO) orchestrates upgrades following specific patterns.

## OpenShift Upgrade Flow

```
CVO starts upgrade
    ↓
1. Update ClusterOperators one by one (ordered)
    ↓
2. Each operator:
   - Checks Upgradeable condition
   - Updates workload (Rolling/Recreate/OnDelete)
   - Reports Progressing, then Available
    ↓
3. CVO waits for Available before proceeding
    ↓
All operators updated → Upgrade complete
```

## Upgrade Ordering

CVO upgrades operators in this order:

1. **Core infrastructure**: CVO, etcd, kube-apiserver
2. **Control plane**: kube-controller-manager, kube-scheduler
3. **Platform services**: machine-config-operator, network
4. **Add-on operators**: monitoring, logging, console
5. **Worker nodes**: machine-config-operator applies node updates

## Upgradeable Condition

Operators must report if it's safe to upgrade:

```yaml
conditions:
- type: Upgradeable
  status: "True"
  reason: AsExpected
  message: "Safe to upgrade"
```

### Blocking Upgrades

```yaml
conditions:
- type: Upgradeable
  status: "False"
  reason: HighPodChurn
  message: "Cannot upgrade: 20% of nodes currently rebooting"
```

**When to block**:
- Active node reboots (MCO)
- Migration in progress (CNO during SDN→OVN)
- Cluster in degraded state
- Incompatible configuration

## Implementation

### Checking Upgradeable

```go
func (r *Reconciler) isUpgradeable(ctx context.Context) (bool, string, error) {
    // Check if nodes are rebooting
    rebootingNodes, err := r.countRebootingNodes(ctx)
    if err != nil {
        return false, "", err
    }
    
    if rebootingNodes > 0.2 * totalNodes {
        return false, "HighPodChurn", nil
    }
    
    // Check if migration in progress
    if r.isMigrationInProgress(ctx) {
        return false, "MigrationInProgress", nil
    }
    
    return true, "AsExpected", nil
}

func (r *Reconciler) updateUpgradeableCondition(ctx context.Context) error {
    upgradeable, reason, err := r.isUpgradeable(ctx)
    if err != nil {
        return err
    }
    
    status := operatorv1.ConditionTrue
    message := "Safe to upgrade"
    if !upgradeable {
        status = operatorv1.ConditionFalse
        message = fmt.Sprintf("Cannot upgrade: %s", reason)
    }
    
    v1helpers.SetOperatorCondition(&r.operatorStatus.Conditions,
        operatorv1.OperatorCondition{
            Type:    operatorv1.OperatorStatusTypeUpgradeable,
            Status:  status,
            Reason:  reason,
            Message: message,
        })
    
    return nil
}
```

## Workload Update Strategies

### RollingUpdate (Recommended)

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-operator
spec:
  replicas: 3
  strategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
      maxSurge: 1
  template:
    # ...
```

**Behavior**: Updates pods gradually (1-2 at a time).

**Pros**: Zero downtime, safe rollback.

**Cons**: Mixed versions temporarily running.

### Recreate

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-operator
spec:
  replicas: 1
  strategy:
    type: Recreate
  template:
    # ...
```

**Behavior**: Deletes all old pods, then creates new ones.

**Pros**: No mixed versions, simpler.

**Cons**: Downtime during update.

**Use case**: Single-replica operators, stateful workloads.

### DaemonSet OnDelete

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: node-agent
spec:
  updateStrategy:
    type: OnDelete
  template:
    # ...
```

**Behavior**: New version only deployed when pod is manually deleted.

**Pros**: Full control over update timing.

**Cons**: Manual intervention required.

**Use case**: Node-level agents requiring coordinated updates (MCO).

### DaemonSet RollingUpdate

```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: node-agent
spec:
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 10%
  template:
    # ...
```

**Behavior**: Updates node pods gradually.

**Pros**: Automatic, controlled rollout.

**Cons**: Less control than OnDelete.

## Version Skew Considerations

OpenShift supports **N-1 version skew** between control plane and nodes.

```
Control Plane: 4.15
Nodes: 4.14 or 4.15 (OK)
Nodes: 4.13 (NOT OK)
```

### Handling Version Skew

```go
func (r *Reconciler) reconcile(ctx context.Context, obj *MyResource) error {
    // Get cluster version
    cv := &configv1.ClusterVersion{}
    if err := r.Get(ctx, types.NamespacedName{Name: "version"}, cv); err != nil {
        return err
    }
    
    currentVersion := cv.Status.Desired.Version
    
    // Check if node is behind
    nodeVersion := obj.Status.CurrentVersion
    if isVersionBehind(nodeVersion, currentVersion) {
        // Apply version-specific logic
        return r.reconcileBackwardsCompatible(ctx, obj, nodeVersion)
    }
    
    return r.reconcileNormal(ctx, obj)
}
```

## Machine Config Operator Pattern

MCO has special upgrade requirements:

1. **Drain nodes**: Cordon and drain before rebooting
2. **Reboot coordination**: Limit concurrent reboots (maxUnavailable)
3. **Wait for ready**: Ensure node healthy before next
4. **Block cluster upgrade**: Set Upgradeable=False during mass reboots

```go
func (r *MCOReconciler) updateUpgradeableCondition(ctx context.Context) error {
    // Count nodes currently rebooting
    rebootingCount, totalCount := r.countRebootingNodes(ctx)
    
    // Block if >20% nodes rebooting
    if float64(rebootingCount)/float64(totalCount) > 0.2 {
        v1helpers.SetOperatorCondition(&r.operatorStatus.Conditions,
            operatorv1.OperatorCondition{
                Type:    operatorv1.OperatorStatusTypeUpgradeable,
                Status:  operatorv1.ConditionFalse,
                Reason:  "TooManyNodesRebooting",
                Message: fmt.Sprintf("%d/%d nodes rebooting", rebootingCount, totalCount),
            })
        return nil
    }
    
    // Safe to upgrade
    v1helpers.SetOperatorCondition(&r.operatorStatus.Conditions,
        operatorv1.OperatorCondition{
            Type:    operatorv1.OperatorStatusTypeUpgradeable,
            Status:  operatorv1.ConditionTrue,
            Reason:  "AsExpected",
            Message: "Safe to upgrade",
        })
    return nil
}
```

## Testing Upgrades

```bash
# Trigger upgrade
oc adm upgrade --to=4.15.0

# Monitor upgrade progress
oc adm upgrade

# Check operator status
oc get clusteroperators

# Watch specific operator
oc get clusteroperator machine-config -w

# Check upgrade history
oc get clusterversion -o yaml | grep -A 20 history
```

## Best Practices

1. **Report Upgradeable**: Always maintain Upgradeable condition
2. **Use RollingUpdate**: Prefer rolling updates for zero downtime
3. **Limit disruption**: Set appropriate maxUnavailable
4. **Test upgrade paths**: Validate N-1 version skew scenarios
5. **Monitor progress**: Set Progressing=True during updates
6. **Handle failures**: Requeue on transient errors, block on persistent issues
7. **Document blockers**: Clear messages why upgrade is blocked

## Common Upgrade Blockers

| Blocker | Operator | Reason |
|---------|----------|--------|
| Nodes rebooting | MCO | Prevent simultaneous control plane + node disruption |
| Network migration | CNO | SDN→OVN requires stable cluster |
| etcd defrag | etcd | High load during defragmentation |
| Storage migration | Storage | Volume migrations in progress |
| Config invalid | Any | User-provided config incompatible with new version |

## Debugging Stuck Upgrades

```bash
# Check which operator is blocking
oc get clusteroperators | grep -v "True.*False.*False"

# Check Upgradeable condition
oc get clusteroperator <name> -o jsonpath='{.status.conditions[?(@.type=="Upgradeable")]}'

# Check Progressing condition
oc get clusteroperator <name> -o jsonpath='{.status.conditions[?(@.type=="Progressing")]}'

# View operator logs
oc logs -n openshift-<name> deployment/<operator-name>
```

## Examples in Components

| Component | Update Strategy | Special Handling |
|-----------|----------------|------------------|
| machine-config-operator | OnDelete | Coordinates node reboots, blocks when >20% rebooting |
| cluster-network-operator | RollingUpdate | Blocks during SDN→OVN migration |
| kube-apiserver | RollingUpdate | Critical - updates first |
| console-operator | RollingUpdate | Non-critical - updates late |
| monitoring | RollingUpdate | Can tolerate brief downtime |

## References

- **CVO Design**: [cluster-version.md](../../domain/openshift/clusterversion.md)
- **Status Conditions**: [status-conditions.md](./status-conditions.md)
- **K8s Update Strategies**: https://kubernetes.io/docs/concepts/workloads/controllers/deployment/#strategy
