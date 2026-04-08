# Owner References Pattern

**Category**: Platform Pattern  
**Applies To**: All operators creating dependent resources  
**Last Updated**: 2026-04-08  

## Overview

OwnerReferences establish parent-child relationships between Kubernetes resources. When the owner is deleted, Kubernetes automatically garbage collects owned resources.

## Why Owner References

**Problem**: Operator creates Deployment, ConfigMap, Service for a CustomResource. User deletes CustomResource - orphaned resources remain.

**Solution**: Set OwnerReference on created resources. Kubernetes deletes them automatically when owner is deleted.

## How It Works

```
User: kubectl delete myresource foo
  ↓
Kubernetes: Deletes MyResource/foo
  ↓
Garbage Collector: Finds all resources with ownerReference to MyResource/foo
  ↓
Garbage Collector: Deletes Deployment, ConfigMap, Service
```

## Implementation

### controller-runtime Pattern

```go
import (
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *Reconciler) reconcile(ctx context.Context, obj *MyResource) error {
    // Create Deployment owned by MyResource
    deployment := &appsv1.Deployment{
        ObjectMeta: metav1.ObjectMeta{
            Name:      obj.Name + "-deployment",
            Namespace: obj.Namespace,
        },
        Spec: appsv1.DeploymentSpec{
            // ... deployment spec
        },
    }
    
    // Set owner reference
    if err := controllerutil.SetControllerReference(obj, deployment, r.Scheme); err != nil {
        return err
    }
    
    if err := r.Create(ctx, deployment); err != nil {
        return err
    }
    
    return nil
}
```

### Manual Pattern

```go
import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

func (r *Reconciler) reconcile(ctx context.Context, obj *MyResource) error {
    configMap := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      obj.Name + "-config",
            Namespace: obj.Namespace,
            OwnerReferences: []metav1.OwnerReference{
                {
                    APIVersion:         obj.APIVersion,
                    Kind:               obj.Kind,
                    Name:               obj.Name,
                    UID:                obj.UID,
                    Controller:         ptr.To(true),
                    BlockOwnerDeletion: ptr.To(true),
                },
            },
        },
        Data: map[string]string{
            "config.yaml": "...",
        },
    }
    
    if err := r.Create(ctx, configMap); err != nil {
        return err
    }
    
    return nil
}
```

## Owner Reference Fields

| Field | Type | Purpose |
|-------|------|---------|
| APIVersion | string | Owner's API version (e.g., "myapi.openshift.io/v1") |
| Kind | string | Owner's Kind (e.g., "MyResource") |
| Name | string | Owner's name |
| UID | UID | Owner's unique ID (prevents matching deleted/recreated owner) |
| Controller | *bool | True if this is the controlling owner (only one allowed) |
| BlockOwnerDeletion | *bool | True to block owner deletion until this resource is deleted |

## Controller vs Non-Controller Owner

```go
// Controller owner (only one per resource)
Controller: ptr.To(true)
// Used for primary parent relationship
// Example: Deployment owns ReplicaSet

// Non-controller owner (can have multiple)
Controller: ptr.To(false)
// Used for tracking, not primary relationship
// Example: PVC references StorageClass
```

## Cross-Namespace Ownership

**Important**: OwnerReferences only work within the same namespace.

```go
// ❌ This won't work - cross-namespace
owner := &MyResource{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "foo",
        Namespace: "namespace-a",
    },
}

owned := &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "bar",
        Namespace: "namespace-b",  // Different namespace!
    },
}

controllerutil.SetControllerReference(owner, owned, scheme)
// Error: cross-namespace owner references are disallowed
```

**Solution**: Use labels for tracking instead.

```go
owned := &corev1.ConfigMap{
    ObjectMeta: metav1.ObjectMeta{
        Name:      "bar",
        Namespace: "namespace-b",
        Labels: map[string]string{
            "app.kubernetes.io/managed-by": "my-operator",
            "myresource.myapi.openshift.io/name": "foo",
        },
    },
}
```

## Cluster-Scoped Resources

Cluster-scoped resources can own namespace-scoped resources:

```yaml
# ClusterRole (cluster-scoped) can own RoleBinding (namespace-scoped)
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-binding
  namespace: default
  ownerReferences:
  - apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRole
    name: my-cluster-role
    uid: abc-123
```

But namespace-scoped resources **cannot** own cluster-scoped resources.

## BlockOwnerDeletion

```go
BlockOwnerDeletion: ptr.To(true)
```

**Effect**: Owner cannot be deleted until owned resource is deleted.

**Use case**: Prevent data loss (e.g., PVC blocking PV deletion).

**Caution**: Can cause deadlocks if two resources block each other.

## Garbage Collection Policies

| Policy | Behavior | Set By |
|--------|----------|--------|
| Foreground | Delete owned resources before owner | `propagationPolicy: Foreground` |
| Background | Delete owner immediately, clean up owned later (default) | `propagationPolicy: Background` |
| Orphan | Delete owner, leave owned resources | `propagationPolicy: Orphan` |

```bash
# Delete with orphan policy (leaves owned resources)
oc delete myresource foo --cascade=orphan
```

## Debugging

```bash
# Check owner references
oc get deployment my-deployment -o yaml | grep -A 10 ownerReferences

# Find resources owned by a resource
oc get all -l app.kubernetes.io/instance=my-resource

# Check garbage collector logs (API server)
oc logs -n kube-system kube-apiserver-... | grep -i garbage
```

## Best Practices

1. **Use SetControllerReference**: Handles all fields correctly
2. **Set on creation**: Add owner reference when creating resource
3. **Single controller**: Only one `Controller: true` per resource
4. **Same namespace**: Owner and owned must be in same namespace
5. **Use labels too**: For querying and cross-namespace tracking
6. **Avoid circular references**: Don't create ownership loops

## Common Patterns

### Deployment Owns ReplicaSet Owns Pod

```
Deployment (controller: true)
    ↓ owns
ReplicaSet (controller: true)
    ↓ owns
Pod
```

### CustomResource Owns Multiple Resources

```
MyResource
    ↓ owns
    ├── Deployment (controller: true)
    ├── Service (controller: true)
    ├── ConfigMap (controller: true)
    └── Secret (controller: true)
```

## Owner References vs Finalizers

| Use | OwnerReference | Finalizer |
|-----|----------------|-----------|
| Kubernetes resources | ✅ Yes | ❌ No |
| External resources (cloud VMs, etc.) | ❌ No | ✅ Yes |
| Automatic cleanup | ✅ Yes | ❌ No (manual) |
| Custom cleanup logic | ❌ No | ✅ Yes |

**Rule**: OwnerReferences for Kubernetes resources, Finalizers for external cleanup.

## Examples in Components

| Component | Owner | Owned |
|-----------|-------|-------|
| machine-config-operator | MachineConfigPool | MachineConfig |
| cluster-network-operator | Network | NetworkAttachmentDefinition |
| cluster-version-operator | ClusterVersion | ClusterOperator (no ownership, monitored) |
| ingress-operator | IngressController | Deployment, Service |

## Common Pitfalls

1. **Cross-namespace ownership**: Doesn't work, use labels instead
2. **Circular ownership**: A owns B, B owns A - deadlock
3. **Multiple controllers**: Two owners with `Controller: true`
4. **Missing UID**: OwnerReference without UID can match wrong resource
5. **Not handling conflicts**: Update conflicts when setting owner reference

## References

- **K8s Garbage Collection**: https://kubernetes.io/docs/concepts/architecture/garbage-collection/
- **OwnerReference API**: https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/owner-reference/
- **Finalizers**: [finalizers.md](./finalizers.md)
