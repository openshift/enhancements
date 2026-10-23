# Finalizers

**Category**: Operator Pattern
**Last Updated**: 2026-10-23

---

## Overview

Finalizers prevent Kubernetes from deleting a resource until the controller has cleaned up external dependencies. They are essential for operators that manage resources outside the Kubernetes API (cloud resources, DNS records, certificates).

## How Finalizers Work

```
1. Controller adds finalizer to resource on create/update
2. User requests deletion → Kubernetes sets deletionTimestamp
3. Resource enters "terminating" state (still exists)
4. Controller detects deletionTimestamp, performs cleanup
5. Controller removes finalizer
6. Kubernetes garbage-collects the resource
```

## Key Conventions

1. **Always add finalizers in the reconcile loop**, not in a webhook — this ensures the finalizer is present even if the webhook was bypassed
2. **Remove the finalizer last** — after ALL external cleanup is confirmed complete
3. **Handle missing external resources gracefully** — if the external resource is already gone, remove the finalizer without error
4. **Use a controller-specific finalizer name** — convention: `<group>/<controller-name>` (e.g., `machine.openshift.io/machine`)
5. **Never block on finalizer removal** — if cleanup fails, requeue with backoff rather than blocking indefinitely

## Common Mistakes

- **Forgetting to handle the deletion path** — the controller must check `DeletionTimestamp` and handle cleanup
- **Leaving orphaned finalizers** — if the controller is removed, resources with its finalizer can never be deleted
- **Using finalizers for in-cluster resources** — prefer owner references for resources within the same cluster

## Owner References vs Finalizers

| Mechanism | Use When | Cleanup |
|-----------|----------|---------|
| **Owner references** | Child is in the same cluster/namespace | Automatic garbage collection |
| **Finalizers** | External resources, cross-namespace, cross-cluster | Manual cleanup by controller |

## Platform Documentation

For operator development conventions, see [dev-guide/operators.md](../../../dev-guide/operators.md).
