# Finalizers Pattern

**Category**: Platform Pattern  
**Applies To**: Operators managing external resources  
**Last Updated**: 2026-04-29  

## Overview

Finalizers enable cleanup of external resources before a Kubernetes object is deleted. They block deletion until the finalizer is removed, allowing controllers to perform cleanup logic.

**Use Case**: Delete external resources (cloud VMs, load balancers) when Kubernetes object is deleted.

## Key Concepts

- **Finalizer**: String in `metadata.finalizers` that blocks deletion
- **DeletionTimestamp**: Set when object is marked for deletion
- **Cleanup Logic**: Controller removes external resources, then removes finalizer
- **Blocking Deletion**: Object stays in "deleting" state until finalizers removed

## Lifecycle

```
1. User creates object
   → Controller adds finalizer

2. User deletes object
   → DeletionTimestamp set (object marked for deletion)
   → Object still exists (blocked by finalizer)

3. Controller sees DeletionTimestamp
   → Cleanup external resources
   → Remove finalizer

4. No finalizers remain
   → Kubernetes deletes object from etcd
```

## Implementation

### Adding Finalizer

```go
import (
    "sigs.k8s.io/controller-runtime/pkg/controllerutil"
)

const finalizerName = "myoperator.example.com/finalizer"

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &v1.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // Add finalizer if not present
    if obj.DeletionTimestamp.IsZero() {
        if !controllerutil.ContainsFinalizer(obj, finalizerName) {
            controllerutil.AddFinalizer(obj, finalizerName)
            if err := r.Update(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
        }
    } else {
        // Handle deletion
        if controllerutil.ContainsFinalizer(obj, finalizerName) {
            if err := r.cleanup(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
            
            controllerutil.RemoveFinalizer(obj, finalizerName)
            if err := r.Update(ctx, obj); err != nil {
                return ctrl.Result{}, err
            }
        }
    }
    
    return ctrl.Result{}, nil
}
```

### Cleanup Logic

```go
func (r *Reconciler) cleanup(ctx context.Context, obj *v1.MyResource) error {
    // Delete external resources
    if err := r.deleteCloudInstance(ctx, obj); err != nil {
        return fmt.Errorf("failed to delete cloud instance: %w", err)
    }
    
    if err := r.deleteLoadBalancer(ctx, obj); err != nil {
        return fmt.Errorf("failed to delete load balancer: %w", err)
    }
    
    log.Info("cleaned up external resources", "name", obj.Name)
    return nil
}
```

## Best Practices

1. **Use Domain-Specific Finalizer Names**: Include domain to avoid conflicts
   ```go
   const finalizerName = "machine.openshift.io/finalizer"  // ✅ Good
   const finalizerName = "finalizer"                        // ❌ Bad (generic)
   ```

2. **Idempotent Cleanup**: Handle cleanup being called multiple times
   ```go
   func (r *Reconciler) cleanup(ctx context.Context, obj *v1.MyResource) error {
       // Check if resource exists before deleting
       instance, err := r.cloudProvider.GetInstance(obj.Spec.InstanceID)
       if err != nil {
           if isNotFoundError(err) {
               return nil  // Already deleted
           }
           return err
       }
       
       return r.cloudProvider.DeleteInstance(instance.ID)
   }
   ```

3. **Handle Errors Gracefully**: Don't remove finalizer if cleanup fails
   ```go
   if err := r.cleanup(ctx, obj); err != nil {
       // Log error, requeue for retry
       return ctrl.Result{}, err  // Don't remove finalizer
   }
   ```

4. **Set Status During Cleanup**: Update status to show cleanup in progress
   ```go
   if !obj.DeletionTimestamp.IsZero() {
       obj.Status.Phase = "Terminating"
       r.Status().Update(ctx, obj)
   }
   ```

5. **Timeout Cleanup**: Don't block forever on stuck cleanup
   ```go
   cleanupCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
   defer cancel()
   
   if err := r.cleanup(cleanupCtx, obj); err != nil {
       return ctrl.Result{}, err
   }
   ```

## Common Patterns

### Machine Finalizer (Machine API)

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  name: my-machine
  finalizers:
  - machine.machine.openshift.io  # Blocks deletion until cloud VM deleted
spec:
  providerSpec:
    value:
      instanceType: m5.large
```

### Multiple Finalizers

```go
const (
    finalizerVM        = "myoperator.example.com/vm"
    finalizerStorage   = "myoperator.example.com/storage"
)

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    obj := &v1.MyResource{}
    r.Get(ctx, req.NamespacedName, obj)
    
    if obj.DeletionTimestamp.IsZero() {
        // Add both finalizers
        controllerutil.AddFinalizer(obj, finalizerVM)
        controllerutil.AddFinalizer(obj, finalizerStorage)
        r.Update(ctx, obj)
    } else {
        // Remove in order: storage first, then VM
        if controllerutil.ContainsFinalizer(obj, finalizerStorage) {
            r.cleanupStorage(ctx, obj)
            controllerutil.RemoveFinalizer(obj, finalizerStorage)
            r.Update(ctx, obj)
        }
        
        if controllerutil.ContainsFinalizer(obj, finalizerVM) {
            r.cleanupVM(ctx, obj)
            controllerutil.RemoveFinalizer(obj, finalizerVM)
            r.Update(ctx, obj)
        }
    }
    
    return ctrl.Result{}, nil
}
```

## Debugging

### View Finalizers

```bash
# Check finalizers on object
oc get machine my-machine -o jsonpath='{.metadata.finalizers}'

# View object stuck in deletion
oc get machines | grep Terminating
```

### Remove Stuck Finalizer (Emergency)

```bash
# Only use if cleanup is stuck and manual intervention needed
oc patch machine my-machine -p '{"metadata":{"finalizers":[]}}' --type=merge
```

**Warning**: This bypasses cleanup logic and may leak external resources.

## Examples in Components

| Component | Finalizer Use | Cleanup Action |
|-----------|--------------|----------------|
| machine-api-operator | `machine.machine.openshift.io` | Delete cloud VM |
| cluster-network-operator | `network.operator.openshift.io/finalizer` | Remove network config |
| cluster-autoscaler | `autoscaler.openshift.io/finalizer` | Delete autoscaler config |

## Troubleshooting

### Object Stuck in Terminating

**Symptoms**: `oc get` shows object in "Terminating" state for >5 minutes

**Causes**:
1. Cleanup logic failing (check operator logs)
2. External resource doesn't exist (idempotent cleanup not implemented)
3. Controller not running (no reconciliation)

**Debug**:
```bash
# Check controller logs
oc logs -n my-operator deployment/my-operator

# Check finalizers
oc get myresource my-obj -o yaml | grep -A5 finalizers

# View events
oc describe myresource my-obj
```

### Finalizer Not Added

**Symptoms**: Object deleted immediately without cleanup

**Causes**:
1. Finalizer not added during creation
2. Race condition (object deleted before finalizer added)

**Fix**: Add finalizer in admission webhook or immediately on creation

## Antipatterns

❌ **Removing finalizer before cleanup**: Leaks external resources  
❌ **Generic finalizer names**: Conflicts with other operators  
❌ **Not handling cleanup errors**: Object stuck forever  
❌ **Synchronous long-running cleanup**: Blocks reconcile loop  
❌ **No idempotent cleanup**: Fails if cleanup called twice

## References

- **Kubernetes**: [Finalizers](https://kubernetes.io/docs/concepts/overview/working-with-objects/finalizers/)
- **controller-runtime**: [controllerutil](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/controllerutil)
- **OpenShift**: Machine API uses finalizers extensively
- **Related**: [controller-runtime.md](./controller-runtime.md)
