# Controller Runtime Pattern

**Category**: Platform Pattern  
**Applies To**: All Operators  
**Last Updated**: 2026-05-26 
**Scope**: All form factors (see HCP note below)

## Overview

The controller-runtime pattern implements the Kubernetes reconciliation loop: continuously compare desired state (spec) with current state (status) and take actions to converge.

**Core Principle**: Watch → Reconcile → Update Status → Repeat

**⚠️ Form Factor Note**: In **Hypershift/HCP**, operators may run in:
- **Guest cluster**: Same as standalone (operator manages guest workloads)
- **Management cluster**: Operator manages hosted control plane components
- **Split across both**: Some controllers in management, some in guest (requires careful design)

See [Hypershift Operator Placement](#hypershift-operator-placement) section below for guidance.

## Key Concepts

- **Reconcile Loop**: Core function that runs when resources change
- **Watch**: Monitor specific resources for changes (create/update/delete events)
- **Requeue**: Return from reconcile with delay to retry later
- **Idempotence**: Reconcile must handle being called multiple times safely
- **Level-Triggered**: React to current state, not edge events

## Implementation

```go
import (
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch current resource
    obj := &MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // 2. Compare desired (obj.Spec) vs current state
    current := r.getCurrentState()
    
    // 3. Take action to converge
    if !stateMatches(obj.Spec, current) {
        if err := r.reconcileState(ctx, obj); err != nil {
            return ctrl.Result{}, err
        }
    }
    
    // 4. Update status
    obj.Status.ObservedGeneration = obj.Generation
    if err := r.Status().Update(ctx, obj); err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```

## Best Practices

1. **Idempotent Reconciliation**: Calling reconcile multiple times = same result
   - Check current state before creating resources
   - Use `CreateOrUpdate()` instead of `Create()`
   
2. **Status Updates Separate from Spec**: 
   - Use `Status().Update()` not `Update()`
   - ObservedGeneration pattern to detect spec changes
   
3. **Requeue Strategy**:
   - `Result{Requeue: true}` - retry immediately
   - `Result{RequeueAfter: 5*time.Minute}` - retry after delay
   - Return error - exponential backoff

4. **Garbage Collection**:
   - Use `OwnerReferences` for automatic cleanup
   - Set `controller: true` for primary owner

## Common Patterns

### Basic Reconcile Structure
```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Pattern: Fetch → Check → Act → Status → Requeue
    
    // 1. Fetch
    obj := &v1.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // 2. Check deletion
    if !obj.DeletionTimestamp.IsZero() {
        return r.handleDeletion(ctx, obj)
    }
    
    // 3. Reconcile owned resources
    if err := r.ensureDeployment(ctx, obj); err != nil {
        return ctrl.Result{}, err
    }
    
    if err := r.ensureService(ctx, obj); err != nil {
        return ctrl.Result{}, err
    }
    
    // 4. Update status
    return r.updateStatus(ctx, obj)
}
```

### Watch Setup
```go
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&v1.MyResource{}).               // Primary resource
        Owns(&appsv1.Deployment{}).          // Owned resources (auto-watch)
        Watches(
            &corev1.ConfigMap{},             // External resource
            handler.EnqueueRequestsFromMapFunc(r.mapConfigMapToOwner),
        ).
        Complete(r)
}
```

### Error Handling
```go
// Transient error - retry with backoff
if err := r.createPod(ctx, obj); err != nil {
    return ctrl.Result{}, fmt.Errorf("failed to create pod: %w", err)
}

// Expected condition - requeue later
if !r.isReady(obj) {
    return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
}

// Success - no requeue
return ctrl.Result{}, nil
```

## Watches and Events

| Watch Type | When to Use | Example |
|------------|-------------|---------|
| `For()` | Primary resource | `.For(&MyResource{})` |
| `Owns()` | Resources with OwnerReference | `.Owns(&Deployment{})` |
| `Watches()` | External resources | Cluster-scoped resources, ConfigMaps |

## Examples in Components

| Component | Pattern | Notes |
|-----------|---------|-------|
| cluster-version-operator | CVO reconciles ClusterVersion | Orchestrates operator upgrades |
| machine-api-operator | Machine controller | Creates cloud instances |
| cluster-network-operator | Network config reconciliation | Multi-resource coordination |

## Antipatterns

❌ **Imperative logic**: Storing state in controller (use cluster state as source of truth)  
❌ **Long-running operations**: Blocking reconcile loop (use requeue instead)  
❌ **Event-driven**: Relying on event order (level-triggered, not edge-triggered)  
❌ **Side effects without idempotency**: Creating resources without checking existence

## Hypershift Operator Placement

In **Hypershift/HCP**, operators run in different locations depending on what they manage.

### Decision Matrix

| Operator Manages | Runs In | Example | Why |
|------------------|---------|---------|-----|
| **Control plane components** | Management cluster | kube-apiserver-operator, etcd-operator | Control plane pods run in management cluster |
| **Guest workloads** | Guest cluster | openshift-monitoring (workload metrics) | Monitors workloads running in guest cluster |
| **Platform infrastructure** | Management cluster | HyperShift Operator | Orchestrates HostedCluster lifecycle |
| **Both control plane and guest** | Split deployment | cluster-network-operator | Control plane networking in mgmt, pod networking in guest |

### Design Considerations

1. **Where does the operator watch resources?**
   - Management cluster: `HostedCluster`, `HostedControlPlane`, control plane pods
   - Guest cluster: `ClusterOperator`, workload resources (Pods, Deployments)

2. **Version skew tolerance**
   - Management cluster operator may be newer than guest cluster
   - Must tolerate N→N+1 version differences

3. **RBAC and permissions**
   - Management cluster: Needs RBAC for `HostedCluster`, infrastructure resources
   - Guest cluster: Same RBAC as standalone operator

4. **Cross-cluster communication**
   - If operator in management needs to watch guest resources → requires KAS access
   - If operator in guest needs to watch management resources → rarely needed, avoid

### Testing HCP Operators

```bash
# Check where operator runs
oc --context=management get pods -A | grep my-operator
oc --context=guest get pods -A | grep my-operator

# Check what operator watches
oc --context=management logs -n my-namespace my-operator-pod | grep "Watching"

# Verify RBAC in correct cluster
oc --context=management get clusterrole my-operator-role
```

See [topology-considerations-guide.md](../../workflows/topology-considerations-guide.md) for comprehensive HCP guidance.

## References

- **Upstream**: [controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)
- **Book**: [Kubebuilder Book - Controllers](https://book.kubebuilder.io/cronjob-tutorial/controller-implementation.html)
- **OpenShift**: [status-conditions.md](./status-conditions.md)
- **Pattern**: Implements "Desired State" from [DESIGN_PHILOSOPHY.md](../../DESIGN_PHILOSOPHY.md)
- **HCP Enhancements** (authoritative sources for HCP operator placement):
  - [hypershift-control-plane-version-status.md](../../../enhancements/hypershift/hypershift-control-plane-version-status.md) - CPO in management, CVO in guest
  - [node-tuning.md](../../../enhancements/hypershift/node-tuning.md) - Example of split operator pattern
