# Controller Runtime Pattern

**Category**: Platform Pattern  
**Applies To**: All Kubernetes/OpenShift operators  
**Last Updated**: 2026-10-23  

## Overview

Controller-runtime is the standard Go framework for building Kubernetes operators. It provides the reconciliation loop, client, cache, and manager abstractions that OpenShift operators build upon.

**Purpose**: Standardize operator development with a common framework for watch, cache, and reconcile patterns.

## Key Concepts

- **Manager**: Coordinates controllers, caches, clients, and leader election
- **Controller**: Watches resources and triggers reconciliation
- **Reconciler**: Implements the reconcile loop (level-driven, not edge-driven)
- **Client**: Reads/writes Kubernetes resources (cached reads, direct writes)
- **Cache**: In-memory informer-backed store for fast reads
- **Predicate**: Filters watch events to reduce unnecessary reconciliations

## Reconciliation Model

```text
Watch Event → Predicate Filter → Work Queue → Reconcile(Request) → Requeue/Done
```

**Level-driven**: Reconcile observes current state and converges toward desired state. It does not react to specific events — missed events are corrected on the next loop iteration.

```go
import (
    "context"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/client"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // 1. Fetch the resource
    obj := &myv1.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // 2. Compare desired vs actual state
    // 3. Converge toward desired state
    // 4. Update status

    return ctrl.Result{}, nil
}
```

## Manager Setup

```go
func main() {
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        Scheme:                 scheme,
        MetricsBindAddress:     ":8080",
        HealthProbeBindAddress: ":8081",
        LeaderElection:         true,
        LeaderElectionID:       "my-operator-lock",
    })

    if err := (&MyReconciler{
        Client: mgr.GetClient(),
        Scheme: mgr.GetScheme(),
    }).SetupWithManager(mgr); err != nil {
        os.Exit(1)
    }

    mgr.Start(ctrl.SetupSignalHandler())
}
```

## Watch Patterns

### Primary Resource
```go
func (r *MyReconciler) SetupWithManager(mgr ctrl.Manager) error {
    return ctrl.NewControllerManagedBy(mgr).
        For(&myv1.MyResource{}).          // Primary watch
        Owns(&appsv1.Deployment{}).       // Watch owned Deployments
        Watches(&corev1.Secret{},         // Watch related Secrets
            handler.EnqueueRequestsFromMapFunc(r.findObjectsForSecret)).
        WithEventFilter(predicate.GenerationChangedPredicate{}).
        Complete(r)
}
```

### Owner References
```go
// Set owner reference so owned resources trigger reconciliation
ctrl.SetControllerReference(owner, owned, r.Scheme)
```

## Best Practices

1. **Idempotent Reconcile**: Every call must produce the same result for the same state
2. **Return Early on NotFound**: Use `client.IgnoreNotFound(err)` for deleted resources
3. **Use Status Subresource**: Update `.status` separately from `.spec`
4. **Requeue on Transient Errors**: Return `ctrl.Result{RequeueAfter: time.Minute}` for retry
5. **Predicate Filtering**: Use `GenerationChangedPredicate` to skip status-only updates

## Common Patterns

### Requeue Strategies

| Scenario | Return Value | Effect |
|----------|-------------|--------|
| Success, no requeue | `ctrl.Result{}, nil` | Wait for next watch event |
| Requeue immediately | `ctrl.Result{Requeue: true}, nil` | Re-enter queue |
| Requeue after delay | `ctrl.Result{RequeueAfter: 30*time.Second}, nil` | Delayed retry |
| Error, auto-requeue | `ctrl.Result{}, err` | Exponential backoff |

### Status Update Pattern
```go
// Always update status as last step
defer func() {
    if err := r.Status().Update(ctx, obj); err != nil {
        log.Error(err, "Failed to update status")
    }
}()
```

## OpenShift Considerations

- **library-go**: Some OpenShift operators use `library-go` controllers instead of controller-runtime. Check imports per controller — do not assume uniformity
- **Apply Methods**: Operators may use SSA (`client.Apply`), strategic merge patch, or `resourceapply` from library-go. Verify per-controller
- **Leader Election**: Required for HA deployments; uses Lease-based election

## Examples in Components

| Component | Framework | Notes |
|-----------|----------|-------|
| cluster-monitoring-operator | controller-runtime | Standard reconcile loop |
| machine-config-operator | library-go | Uses library-go controllers |
| cluster-network-operator | controller-runtime | Multiple controllers |
| cluster-storage-operator | library-go + controller-runtime | Mixed framework usage |

## Antipatterns

❌ **Edge-driven logic**: Reacting to specific events instead of current state  
❌ **Blocking reconcile**: Long-running operations in the reconcile loop  
❌ **Direct API reads**: Bypassing the cache for reads (use cache, write directly)  
❌ **Missing error handling**: Not returning errors for requeue  
❌ **Assuming uniform framework**: Different controllers may use different frameworks

## References

- **Upstream**: [controller-runtime](https://pkg.go.dev/sigs.k8s.io/controller-runtime)
- **Book**: [Kubebuilder Book](https://book.kubebuilder.io/)
- **API**: [reconcile package](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/reconcile)
- **Pattern**: Implements "Level-Driven Reconciliation" from [DESIGN_PHILOSOPHY.md](../../DESIGN_PHILOSOPHY.md)
