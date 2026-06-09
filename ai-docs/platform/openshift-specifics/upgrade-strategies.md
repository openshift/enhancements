# Upgrade Strategies

**Category**: OpenShift-Specific Pattern  
**Applies To**: All ClusterOperators  
**Last Updated**: 2026-05-26
**Scope**: Primarily standalone clusters; see [HCP Differences](#hypershift--hosted-control-planes-hcp) section

## Overview

OpenShift upgrades are orchestrated by the Cluster Version Operator (CVO). Every operator must support N→N+1 version skew and coordinate with CVO for zero-downtime upgrades.

**Goal**: Upgrade 1000-node cluster with zero application downtime.

**⚠️ Form Factor Note**: This document describes upgrade behavior in **standalone OpenShift clusters** (self-hosted control plane). For Hypershift/HCP deployments, control plane and data plane upgrade separately—see the HCP section below.

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

CVO applies manifests in lexicographic order by filename prefix during upgrades.

**Runlevel ordering** (from [CVO dev docs](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/operators.md)):
```
Runlevel 00-09: Core platform (CVO, network, DNS, certs)
Runlevel 10-29: Kubernetes operators (API server, controllers, scheduler)  
Runlevel 30-39: Machine API
Runlevel 50-59: Operator Lifecycle Manager
Runlevel 60-69: OpenShift core operators
```

**Key behaviors**:
- Components at same runlevel execute in **parallel**
- Runlevel ordering provides deterministic apply sequence
- Generally ensures core components are ready before dependent operators
- **In practice**: Strict ordering rarely critical; most components tolerate version skew during upgrades
- Ordering is conservative to handle rare edge cases, not strict runtime dependencies

## Implementation

### Setting Upgradeable Condition

```go
func (r *Reconciler) checkUpgradeable(ctx context.Context) (bool, string, string) {
    // Check if upgrade would be unsafe based on current cluster state
    
    // Example: Block if unsupported configuration requires manual intervention
    if hasUnsupportedConfig() {
        return false, "UnsupportedConfiguration", 
            "Unsupported configuration requires manual steps before upgrade. See documentation for migration path."
    }
    
    // Example: Block if upgrade would fail due to missing prerequisites
    if missingUpgradePrerequisites() {
        return false, "MissingPrerequisites",
            "Required migration steps not completed. Run 'oc adm upgrade --check' for details."
    }
    
    // Note: Degraded state alone should NOT set Upgradeable=False
    // Only set Upgradeable=False if upgrade would make things worse or fail
    
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

### Handling Version Skew (Level-Driven Reconciliation)

```go
// Level-driven: This reconcile function runs continuously (every N seconds or on watch events)
// It always converges toward the desired state, not reacting to state changes
func (r *Reconciler) reconcile(ctx context.Context, obj *MyResource) error {
    // Desired state (level): what should exist
    desiredVersion := os.Getenv("RELEASE_VERSION")
    
    // Current state (level): what actually exists
    currentVersion := obj.Status.Version
    
    // Continuous reconciliation: always ensure current matches desired
    // This check runs on every reconcile loop, not just when version changes
    if desiredVersion != currentVersion {
        // Converge toward desired state
        if err := r.ensureVersion(ctx, obj, desiredVersion); err != nil {
            // Requeue - will retry on next reconcile
            return err
        }
        
        // Update status to reflect progress
        obj.Status.Version = desiredVersion
        setCondition(&obj.Status.Conditions,
            configv1.OperatorProgressing,
            configv1.ConditionTrue,
            "RollingOut",
            fmt.Sprintf("Converging to %s", desiredVersion))
    }
    
    // Continue reconciling other aspects - level-driven means
    // we continuously ensure ALL desired state, not just version
    return r.ensureWorkloadState(ctx, obj)
}

// ❌ Anti-pattern: Edge-driven (don't do this)
// func (r *Reconciler) onVersionChange(old, new string) {
//     // Only runs when version changes - misses corrections needed
// }
```

**Level-driven reconciliation pattern**: Per [controller-runtime documentation](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/reconcile), "Reconciliation is level-based, meaning action isn't driven off changes in individual Events, but instead is driven by actual cluster state read from the apiserver or a local cache." The reconcile function observes current state and continuously converges toward desired state, rather than reacting to specific events. This ensures idempotency and resilience—missed events are automatically corrected on the next reconcile loop.

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

### API Version Transitions (OpenShift Pattern)

**OpenShift approach**: Transition API versions across OpenShift minor releases without serving multiple versions simultaneously (no conversion webhooks needed).

```yaml
# Release N: Serve v1alpha1 only
versions:
- name: v1alpha1
  served: true
  storage: true

# Release N+1: Introduce v1, serve both (read-only migration window)
versions:
- name: v1
  served: true
  storage: true  # New storage version
- name: v1alpha1
  served: true   # Still served for read
  storage: false
  deprecated: true
  
# Release N+2: Stop serving v1alpha1
versions:
- name: v1
  served: true
  storage: true
- name: v1alpha1
  served: false   # No longer served
  storage: false
```

**Migration steps**:
1. Release N+1: Change storage version, mark old version deprecated
2. During N+1: All writes go to new version, reads accepted from both
3. Existing objects migrated during upgrade (touched objects auto-convert)
4. Release N+2: Stop serving old version entirely

**OpenShift avoids**:
- ❌ Conversion webhooks (adds complexity, rarely needed)
- ❌ Serving multiple versions long-term (increases API surface)

See [API evolution](../../practices/development/api-evolution.md) for version transition patterns.

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

## Control Plane Upgrade Pattern

**Static Pod + PDB Guard Pattern**: All control plane components use this pattern for upgrades:

1. **Static pod** runs the actual component (etcd, kube-apiserver, kube-controller-manager, kube-scheduler)
2. **Guard pod** (Deployment) mirrors the static pod state
3. **PodDisruptionBudget** on guard pod prevents node drain if it would violate availability
4. Ensures at least N-1 replicas available during node upgrades

**Why this pattern?**
- Control plane runs as static pods (not managed by scheduler)
- PDB only works on pods managed by controllers (Deployment, StatefulSet)
- Guard pods enable PDB protection for static pods

## Examples in Components

| Component | Upgrade Strategy | Notes |
|-----------|------------------|-------|
| etcd | Static pod + PDB guard | Must maintain quorum (2/3 pods) |
| kube-apiserver | Static pod + PDB guard | PDB ensures 1+ available |
| kube-controller-manager | Static pod + PDB guard | Same pattern as above |
| kube-scheduler | Static pod + PDB guard | Same pattern as above |
| machine-config-operator | Node drain → reboot → uncordon | Can take 30+ minutes per node |
| cluster-network-operator | Update CNI plugins in rolling fashion | Brief network interruptions possible |

## Antipatterns

❌ **Breaking API changes**: Removing fields in minor version  
❌ **All replicas down**: No PodDisruptionBudget for HA components  
❌ **Long rollouts**: Not reporting Progressing condition  
❌ **Ignoring Upgradeable**: Allowing unsafe upgrades to proceed  
❌ **Blocking CVO**: Long-running reconciliation preventing other operators from upgrading

## Hypershift / Hosted Control Planes (HCP)

**⚠️ Critical Difference**: In HCP, the control plane and data plane upgrade **independently**.

### HCP Upgrade Model

```
Standalone:
  CVO upgrades control plane → operators → workloads (all in same cluster)

HCP:
  Management Cluster:
    HyperShift Operator upgrades → Hosted Control Plane components
  Guest Cluster:
    CVO in guest upgrades → operators in guest → workloads
```

### Key Differences

| Aspect | Standalone | HCP |
|--------|-----------|-----|
| **Control plane location** | Same cluster as workloads | Management cluster |
| **Upgrade orchestrator** | CVO in same cluster | HyperShift Operator (mgmt) + CVO (guest) |
| **Version skew** | N→N+1 within cluster | Control plane and data plane can differ |
| **Node upgrades** | MCO reboots nodes | Guest MCO manages guest nodes; mgmt cluster nodes managed separately |
| **Operator location** | Depends—some in mgmt, some in guest | Must explicitly consider |

**Note on node upgrades in HCP**: The control plane runs as **pods** in the management cluster (not on dedicated nodes). The management cluster has its own worker nodes where these control plane pods run. Those management cluster nodes are upgraded by the **management cluster's MCO**, independently from the guest cluster. The **guest cluster's MCO** only manages guest cluster worker nodes.

### HCP Considerations for Operators

**If your operator runs in the management cluster**:
- ✅ Upgraded by HyperShift Operator, not CVO
- ✅ Must tolerate guest cluster version skew (guest may be older)
- ✅ Network access to guest cluster required (via KAS)

**If your operator runs in the guest cluster**:
- ✅ Upgraded by CVO in guest (same as standalone)
- ✅ Must tolerate control plane version skew (control plane may be newer)
- ✅ API calls go through remote KAS (in management cluster)

**If your operator is split across both**:
- ✅ Coordination required—design for independent upgrade order
- ✅ Must tolerate partial upgrade state (mgmt upgraded, guest not yet)
- ✅ Document dependencies in enhancement proposal

### SNO (Single-Node OpenShift) Considerations

**Resource Constraints**:
- All overhead runs on ONE node (control plane + workloads)
- No HA—single point of failure during node reboot
- Node upgrade = full cluster downtime (1 node can't drain itself)

**Upgrade Impact**:
```yaml
# Standalone HA cluster
3 control plane nodes + N worker nodes
→ Rolling update, zero downtime

# SNO
1 node
→ Node reboot = cluster downtime (~5-10 minutes)
```

**Design Considerations**:
- **Platform operator downtime**: Acceptable during node reboot (unavoidable in SNO)
- **User-facing services** (ingress, router): Aim to minimize interruption
  - Use graceful shutdown patterns
  - Pre-pull images to reduce restart time
  - Minimize unavailability window
- Ensure rapid reconciliation after reboot
- Test with `replica: 1` configurations

### HA Cluster Upgrade Expectations

**For HA (3+ control plane node) clusters**:

- **Platform operators**: Brief downtime acceptable during rollout
  - Operators can restart/upgrade one at a time
  - Controller temporarily unavailable (reconciliation paused)
  
- **User-facing services** (ingress, router, load balancers): **Zero downtime expected**
  - Must use PodDisruptionBudgets
  - Rolling updates with N-1 available
  - Graceful connection draining
  
- **Application workloads**: Should not experience interruption
  - Proper PDB configuration required
  - Applications must handle pod churn

### MicroShift Considerations

**Different Upgrade Model**:
- No CVO—upgrades via RPM/greenboot
- No MCO—host OS managed separately
- Operators may not exist (smaller footprint)

**If your enhancement involves**:
- ✅ New APIs → May not be available in MicroShift
- ✅ Operators → May not run in MicroShift
- ✅ CVO coordination → Not applicable to MicroShift

See [topology-considerations-guide.md](../../workflows/topology-considerations-guide.md) for comprehensive form factor guidance.

## References

- **CVO**: [Cluster Version Operator](https://github.com/openshift/cluster-version-operator)
- **Enhancement**: [Upgrade Ordering](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/operators.md)
- **API**: [ClusterVersion](https://github.com/openshift/api/blob/master/config/v1/types_cluster_version.go)
- **Pattern**: Implements "Upgrade Safety" from [DESIGN_PHILOSOPHY.md](../../DESIGN_PHILOSOPHY.md)
- **Form Factors**: [topology-considerations-guide.md](../../workflows/topology-considerations-guide.md) - Comprehensive HCP/SNO/MicroShift guidance
- **HCP Enhancements** (authoritative sources for HCP upgrade behavior):
  - [hypershift-control-plane-version-status.md](../../../enhancements/hypershift/hypershift-control-plane-version-status.md) - Management vs guest upgrade orchestration
  - [monitoring.md](../../../enhancements/hypershift/monitoring.md) - Control plane/guest cluster separation
