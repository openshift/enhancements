# Controller Runtime Patterns

**Category**: Operator Pattern
**Last Updated**: 2026-10-23

---

## Overview

OpenShift operators use two primary controller frameworks. Understanding which framework a component uses is essential — they have different APIs, startup patterns, and resource management approaches.

## Framework Comparison

| Aspect | controller-runtime | library-go |
|--------|-------------------|------------|
| **Import** | `sigs.k8s.io/controller-runtime` | `github.com/openshift/library-go/pkg/operator` |
| **Reconcile signature** | `Reconcile(ctx, Request) (Result, error)` | Sync-based with `SyncContext` |
| **Resource apply** | `client.Patch` (supports multiple patch types; SSA requires explicit apply patch type with field-manager) or `client.Update` | `resourceapply` helpers |
| **Used by** | Newer operators, OLM-managed | Core platform operators |
| **Manager** | `ctrl.NewManager()` | Custom startup via `library-go/pkg/cmd` |

## Level-Driven Reconciliation

Both frameworks implement **level-driven** (not edge-driven) reconciliation:
- Controllers reconcile the full desired state on every trigger
- The reconcile loop is idempotent — running it twice produces the same result
- Watch events are triggers, not the source of truth; always re-read current state

## Startup Pattern

### controller-runtime
```
main() → manager.New() → controller.Setup() → manager.Start()
```
Controllers register via `SetupWithManager(mgr)`, declaring watches on owned/watched resources.

### library-go
```
main() → operator.New() → controller.New() → operator.Run()
```
Controllers register through library-go's operator framework, using `factory.Controller()` pattern with informer-based watches.

## Key Conventions

1. **Never mix frameworks** within a single controller — use the framework the repo has standardized on
2. **Always re-read state** at the start of reconciliation; never cache state between reconcile calls
3. **Use context cancellation** for graceful shutdown
4. **Log at appropriate levels** — use `log.Error` for errors; reserve `V(0)` for high-priority informational messages, `V(2)` for state changes, `V(4)` for detailed reconciliation

## Platform Documentation

For operator development conventions, see [dev-guide/operators.md](../../../dev-guide/operators.md). For status reporting patterns, see [status-conditions.md](status-conditions.md).
