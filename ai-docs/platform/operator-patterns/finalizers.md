# Finalizers Pattern

**Category**: Platform Pattern  
**Applies To**: Operators managing external resources or requiring cleanup  
**Last Updated**: 2026-10-23  

## Overview

Finalizers prevent resource deletion until cleanup logic completes. They ensure external resources (cloud infrastructure, certificates, DNS records) are properly cleaned up when Kubernetes resources are deleted.

**Purpose**: Guarantee cleanup of external state when Kubernetes resources are deleted.

## Key Concepts

- **Finalizer**: A string in `metadata.finalizers` that blocks deletion
- **DeletionTimestamp**: Set when a delete request is received; non-nil means deletion is pending
- **Cleanup**: The operator's logic that runs before removing the finalizer
- **Foreground Deletion**: Kubernetes waits for finalizers before garbage collection

## Lifecycle

```text
1. CREATE: Add finalizer to resource
2. RECONCILE: Normal operation (finalizer present, no DeletionTimestamp)
3. DELETE: Kubernetes sets DeletionTimestamp, resource stays in API
4. CLEANUP: Operator detects DeletionTimestamp, runs cleanup logic
5. REMOVE: Operator removes finalizer → Kubernetes deletes the resource
```

## Implementation

```go
import (
    "context"
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const myFinalizer = "my.openshift.io/cleanup"

func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &myv1.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }

    // Check if being deleted
    if !obj.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(obj, myFinalizer) {
            // Run cleanup logic
            if err := r.cleanupExternalResources(ctx, obj); err != nil {
                return ctrl.Result{}, err // Requeue on failure
            }

            // Remove finalizer after cleanup succeeds
            controllerutil.RemoveFinalizer(obj, myFinalizer)
            if err := r.Update(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
        }
        return ctrl.Result{}, nil
    }

    // Add finalizer if not present
    if !controllerutil.ContainsFinalizer(obj, myFinalizer) {
        controllerutil.AddFinalizer(obj, myFinalizer)
        if err := r.Update(ctx, obj); err != nil {
            return ctrl.Result{}, err
        }
    }

    // Normal reconciliation
    return r.reconcileNormal(ctx, obj)
}

func (r *MyReconciler) cleanupExternalResources(ctx context.Context, obj *myv1.MyResource) error {
    // Delete cloud resources, DNS records, certificates, etc.
    // Must be idempotent - may be called multiple times on retry
    return nil
}
```

## Best Practices

1. **Add Finalizer on Create**: Ensure cleanup runs even if operator was down during delete
2. **Idempotent Cleanup**: Cleanup function may be called multiple times on retries
3. **Handle Missing Resources**: External resource may already be gone; don't error on not-found
4. **Timeout Consideration**: Cleanup should not block indefinitely; consider time limits
5. **Finalizer Naming**: Use domain-qualified names (e.g., `my.openshift.io/cleanup`)

## Common Patterns

### Cleanup with Timeout
```go
func (r *MyReconciler) cleanupExternalResources(ctx context.Context, obj *myv1.MyResource) error {
    ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
    defer cancel()

    if err := r.deleteCloudResource(ctx, obj.Status.ResourceID); err != nil {
        if isNotFound(err) {
            return nil // Already cleaned up
        }
        return err // Retry
    }
    return nil
}
```

### Multiple Finalizers
```go
// Different operators can add their own finalizers
const (
    networkFinalizer  = "network.openshift.io/cleanup"
    storageFinalizer  = "storage.openshift.io/cleanup"
)
```

## Antipatterns

❌ **Forgetting to add finalizer**: Resource deleted before cleanup runs  
❌ **Non-idempotent cleanup**: Fails on retry because resource already deleted  
❌ **Blocking forever**: Cleanup hangs indefinitely, resource stuck in Terminating  
❌ **Removing finalizer before cleanup**: External resources leak  
❌ **Finalizer on resources you don't own**: Only add finalizers to resources your operator manages

## References

- **Upstream**: [Kubernetes Finalizers](https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/)
- **API**: [ObjectMeta.Finalizers](https://pkg.go.dev/k8s.io/apimachinery/pkg/apis/meta/v1#ObjectMeta)
- **Helper**: [controllerutil.AddFinalizer](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/controller/controllerutil)
- **Pattern**: Complements [controller-runtime.md](./controller-runtime.md) reconcile loop
