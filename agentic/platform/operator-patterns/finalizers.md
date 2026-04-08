# Finalizers Pattern

**Category**: Platform Pattern  
**Applies To**: Operators managing external resources  
**Last Updated**: 2026-04-08  

## Overview

Finalizers allow operators to perform cleanup before Kubernetes deletes a resource. Critical for resources that manage external state (cloud resources, node configuration).

## Why Finalizers

**Problem**: User deletes CustomResource, Kubernetes immediately removes it, operator never runs cleanup.

**Solution**: Finalizer blocks deletion until operator completes cleanup and removes the finalizer.

## How It Works

```
User: kubectl delete myresource foo
  ↓
Kubernetes: Sets .metadata.deletionTimestamp (marks for deletion)
  ↓
Controller: Sees deletionTimestamp != nil
  ↓
Controller: Runs cleanup (delete cloud VM, release IPs, etc.)
  ↓
Controller: Removes finalizer from .metadata.finalizers[]
  ↓
Kubernetes: Finalizers empty → actually deletes resource
```

## Implementation

### Basic Pattern

```go
import "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

const myFinalizer = "myresource.myapi.openshift.io/finalizer"

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Resource being deleted?
    if !obj.DeletionTimestamp.IsZero() {
        if controllerutil.ContainsFinalizer(obj, myFinalizer) {
            // Run cleanup
            if err := r.cleanup(ctx, obj); err != nil {
                // Requeue on cleanup failure
                return ctrl.Result{}, err
            }
            
            // Remove finalizer
            controllerutil.RemoveFinalizer(obj, myFinalizer)
            if err := r.Update(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
        }
        // No finalizer or already removed - let Kubernetes delete
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
    return r.reconcile(ctx, obj)
}
```

### Cleanup Logic

```go
func (r *Reconciler) cleanup(ctx context.Context, obj *MyResource) error {
    log.Info("Cleaning up external resources", "name", obj.Name)
    
    // 1. Delete cloud resources
    if err := r.cloudClient.DeleteVM(obj.Status.VMInstanceID); err != nil {
        if !isNotFound(err) {
            return fmt.Errorf("failed to delete VM: %w", err)
        }
        // VM already deleted - continue
    }
    
    // 2. Release IPs
    if err := r.releaseIPAddress(obj.Status.IPAddress); err != nil {
        return err
    }
    
    // 3. Delete owned resources
    if err := r.deleteOwnedConfigMaps(ctx, obj); err != nil {
        return err
    }
    
    log.Info("Cleanup complete", "name", obj.Name)
    return nil
}
```

## Best Practices

1. **Domain-based naming**: `myresource.myapi.openshift.io/finalizer`
2. **Idempotent cleanup**: Safe to call multiple times
3. **Handle missing resources**: External resource already deleted is OK
4. **Timeout protection**: Set upper bound on cleanup duration
5. **Don't add finalizer retroactively**: Only add on creation or first reconciliation
6. **One finalizer per responsibility**: Separate finalizers for separate cleanup tasks

## Multiple Finalizers

```go
const (
    cloudResourceFinalizer = "cloud.myapi.openshift.io/finalizer"
    nodeConfigFinalizer    = "node.myapi.openshift.io/finalizer"
)

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    if !obj.DeletionTimestamp.IsZero() {
        // Clean up cloud resources
        if controllerutil.ContainsFinalizer(obj, cloudResourceFinalizer) {
            if err := r.cleanupCloudResources(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
            controllerutil.RemoveFinalizer(obj, cloudResourceFinalizer)
            if err := r.Update(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
        }
        
        // Clean up node configuration
        if controllerutil.ContainsFinalizer(obj, nodeConfigFinalizer) {
            if err := r.cleanupNodeConfig(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
            controllerutil.RemoveFinalizer(obj, nodeConfigFinalizer)
            if err := r.Update(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
        }
        
        return ctrl.Result{}, nil
    }
    
    // Add both finalizers
    if !controllerutil.ContainsFinalizer(obj, cloudResourceFinalizer) {
        controllerutil.AddFinalizer(obj, cloudResourceFinalizer)
        if err := r.Update(ctx, obj); err != nil {
            return ctrl.Result{}, err
        }
    }
    if !controllerutil.ContainsFinalizer(obj, nodeConfigFinalizer) {
        controllerutil.AddFinalizer(obj, nodeConfigFinalizer)
        if err := r.Update(ctx, obj); err != nil {
            return ctrl.Result{}, err
        }
    }
    
    return r.reconcile(ctx, obj)
}
```

## Timeout Protection

```go
func (r *Reconciler) cleanup(ctx context.Context, obj *MyResource) error {
    // Set timeout for cleanup
    ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
    defer cancel()
    
    if err := r.performCleanup(ctx, obj); err != nil {
        if ctx.Err() == context.DeadlineExceeded {
            log.Error(err, "Cleanup timed out", "name", obj.Name)
            // Option 1: Return error and retry
            return fmt.Errorf("cleanup timeout: %w", err)
            
            // Option 2: Force proceed (dangerous!)
            // log.Warn("Forcing deletion despite timeout")
            // return nil
        }
        return err
    }
    
    return nil
}
```

## Debugging Stuck Deletions

```bash
# Resource stuck in Terminating?
oc get myresource -o yaml

# Check finalizers
oc get myresource foo -o jsonpath='{.metadata.finalizers}'

# Check deletionTimestamp
oc get myresource foo -o jsonpath='{.metadata.deletionTimestamp}'

# Force delete (dangerous - skips cleanup!)
oc patch myresource foo -p '{"metadata":{"finalizers":null}}' --type=merge

# Better: Check operator logs for cleanup failures
oc logs -n openshift-my-operator deployment/my-operator | grep -i cleanup
```

## Common Pitfalls

1. **Infinite cleanup loop**: Cleanup always fails, finalizer never removed
2. **Deadlock**: Finalizer depends on resource that's also being deleted
3. **Forgot to remove finalizer**: Resource stuck in Terminating forever
4. **Not handling missing resources**: Cleanup fails if external resource already gone
5. **Adding finalizer on every reconciliation**: Creates unnecessary Update calls

## Examples in Components

| Component | Finalizer | Cleanup Task |
|-----------|-----------|-------------|
| machine-api-operator | machine.cluster.k8s.io | Delete cloud VMs, release IPs |
| machine-config-operator | machineconfiguration.openshift.io | Drain nodes, remove configurations |
| cluster-network-operator | network.operator.openshift.io | Clean up network interfaces |
| ingress-operator | ingress.openshift.io | Delete cloud load balancers |

## Finalizer vs OwnerReferences

| Use | Finalizer | OwnerReference |
|-----|-----------|----------------|
| External cleanup (cloud resources) | ✅ Yes | ❌ No |
| Owned Kubernetes resources | ❌ No (use OwnerReference) | ✅ Yes |
| Custom cleanup logic | ✅ Yes | ❌ No |
| Automatic garbage collection | ❌ No | ✅ Yes |

**Rule**: Use OwnerReferences for Kubernetes resources, Finalizers for external resources.

## References

- **K8s Finalizers**: https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/
- **controller-runtime**: https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/controller/controllerutil
- **Owner References**: [owner-references.md](./owner-references.md)
