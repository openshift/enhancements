# Operator Patterns

Standard patterns for building OpenShift operators. These patterns apply across all component repositories.

## Patterns

- [controller-runtime.md](controller-runtime.md) - Reconciliation framework, manager setup, watch patterns
- [status-conditions.md](status-conditions.md) - Available/Progressing/Degraded health reporting
- [webhooks.md](webhooks.md) - Admission webhooks for validation and mutation
- [finalizers.md](finalizers.md) - Cleanup external resources on deletion
- [rbac.md](rbac.md) - Service account permissions and least privilege
- [must-gather.md](must-gather.md) - Diagnostic data collection

## Decision Guide

| Need | Pattern |
|------|---------|
| Build a new operator | Start with [controller-runtime.md](controller-runtime.md) |
| Report operator health | Follow [status-conditions.md](status-conditions.md) |
| Validate API inputs | Add [webhooks.md](webhooks.md) |
| Clean up cloud resources | Use [finalizers.md](finalizers.md) |
| Scope permissions | Apply [rbac.md](rbac.md) |
| Enable debugging | Implement [must-gather.md](must-gather.md) |

## Related

- [upgrade-strategies.md](../openshift-specifics/upgrade-strategies.md) - Upgrade coordination with CVO
- [DESIGN_PHILOSOPHY.md](../../DESIGN_PHILOSOPHY.md) - Core architectural principles
