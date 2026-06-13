# Operator Patterns

Standard patterns used across all OpenShift operators.

## Core Patterns

- [controller-runtime.md](controller-runtime.md) - Reconciliation loop pattern (watch → reconcile → update status)
- [status-conditions.md](status-conditions.md) - Available/Progressing/Degraded health reporting
- [webhooks.md](webhooks.md) - Validation, mutation, and conversion webhooks

## Resource Management

- [finalizers.md](finalizers.md) - Cleanup external resources before deletion
- [rbac.md](rbac.md) - Service account and RBAC permissions

## Operations

- [must-gather.md](must-gather.md) - Debugging and diagnostics collection

## Related

- [OpenShift-Specific Patterns](../openshift-specifics/) - Upgrade strategies, CVO coordination
