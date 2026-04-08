# controller-runtime Pattern

**Category**: Platform Pattern  
**Applies To**: All Kubernetes operators  
**Last Updated**: 2026-04-08  

## Overview

OpenShift operators use controller-runtime for reconciliation loops. This provides watch/cache infrastructure, leader election, and reconciliation patterns.

## Basic Pattern

```go
import (
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch resource
    obj := &MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // 2. Reconcile to desired state
    if err := r.reconcile(ctx, obj); err != nil {
        return ctrl.Result{}, err
    }
    
    // 3. Update status
    if err := r.Status().Update(ctx, obj); err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```

## Key Concepts

- **Watches**: Monitor resources for changes (create/update/delete events)
- **Informers**: Cache resources locally (reduces API load, enables fast lookups)
- **Reconciliation**: Drive actual state → desired state idempotently
- **Requeue**: Retry on transient errors (exponential backoff automatically applied)
- **Client**: Unified interface for reading/writing Kubernetes resources

## Reconciliation Loop

```
Event → Queue → Reconcile() → Success/Error
  ↑                               ↓
  └───────── Requeue ←────────────┘
```

**Important**: Reconcile() should be idempotent - safe to call multiple times.

## Common Patterns

### Requeue with Delay
```go
// Requeue after 30 seconds (e.g., waiting for external dependency)
return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
```

### Error Handling
```go
// Return error - controller-runtime handles requeue with exponential backoff
if err := r.reconcile(ctx, obj); err != nil {
    return ctrl.Result{}, fmt.Errorf("reconcile failed: %w", err)
}
```

### Ignore Not Found
```go
// Resource deleted - don't requeue
if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
    return ctrl.Result{}, client.IgnoreNotFound(err)
}
```

### Watches and Owned Resources
```go
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&MyResource{}).                    // Primary resource
        Owns(&corev1.ConfigMap{}).             // Watch owned ConfigMaps
        Watches(&source.Kind{Type: &corev1.Secret{}}, 
                handler.EnqueueRequestsFromMapFunc(r.findObjectsForSecret)).
        Complete(r)
}
```

## Status Updates

**Critical**: Update status in separate call to avoid conflicts.

```go
// Update spec
if err := r.Update(ctx, obj); err != nil {
    return ctrl.Result{}, err
}

// Update status separately
if err := r.Status().Update(ctx, obj); err != nil {
    return ctrl.Result{}, err
}
```

## Finalizers Integration

```go
import "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Handle deletion
    if !obj.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(obj, myFinalizer) {
            if err := r.cleanup(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
            controllerutil.RemoveFinalizer(obj, myFinalizer)
            return ctrl.Result{}, r.Update(ctx, obj)
        }
        return ctrl.Result{}, nil
    }
    
    // Add finalizer
    if !controllerutil.ContainsFinalizer(obj, myFinalizer) {
        controllerutil.AddFinalizer(obj, myFinalizer)
        return ctrl.Result{}, r.Update(ctx, obj)
    }
    
    // Normal reconciliation
    return ctrl.Result{}, r.reconcile(ctx, obj)
}
```

## Client Usage

```go
// List resources
list := &MyResourceList{}
if err := r.List(ctx, list, client.InNamespace("default")); err != nil {
    return err
}

// Get by name
obj := &MyResource{}
if err := r.Get(ctx, types.NamespacedName{Name: "foo", Namespace: "default"}, obj); err != nil {
    return err
}

// Create
newObj := &MyResource{...}
if err := r.Create(ctx, newObj); err != nil {
    return err
}

// Update
obj.Spec.Field = "new-value"
if err := r.Update(ctx, obj); err != nil {
    return err
}

// Delete
if err := r.Delete(ctx, obj); err != nil {
    return err
}
```

## Performance Best Practices

1. **Use indexes**: For efficient lookups in List operations
2. **Minimize API calls**: Use cached client when possible
3. **Batch updates**: Group related changes together
4. **Watch selectively**: Only watch resources you need
5. **Use field selectors**: Reduce watch scope

## Example: Index Setup

```go
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
    // Create index for fast lookups
    if err := mgr.GetFieldIndexer().IndexField(
        context.Background(),
        &corev1.Pod{},
        "spec.nodeName",
        func(obj client.Object) []string {
            pod := obj.(*corev1.Pod)
            return []string{pod.Spec.NodeName}
        },
    ); err != nil {
        return err
    }
    
    return ctrl.NewControllerManagedBy(mgr).For(&MyResource{}).Complete(r)
}

// Use index in reconciliation
func (r *Reconciler) reconcile(ctx context.Context, obj *MyResource) error {
    pods := &corev1.PodList{}
    if err := r.List(ctx, pods, client.MatchingFields{"spec.nodeName": "node-1"}); err != nil {
        return err
    }
    // Process pods...
}
```

## Examples in Components

| Component | Controller | Notes |
|-----------|-----------|-------|
| machine-config-operator | MachineConfigController | Renders configs for node pools |
| cluster-network-operator | NetworkController | Manages CNI (SDN/OVN) |
| machine-api-operator | MachineController | Node lifecycle management |
| cluster-version-operator | ClusterVersionController | Platform upgrade orchestration |

## References

- **Upstream**: https://github.com/kubernetes-sigs/controller-runtime
- **Kubebuilder Book**: https://book.kubebuilder.io/
- **Leader Election**: [leader-election.md](./leader-election.md)
- **Finalizers**: [finalizers.md](./finalizers.md)
