# Development Practices

Development conventions and API evolution guidelines for OpenShift components.

## Documents

- [api-evolution.md](api-evolution.md) - API versioning, deprecation, and backward compatibility

## Key Conventions

| Area | Convention |
|------|-----------|
| **API Versions** | alpha → beta → stable; follow Kubernetes API conventions |
| **Backward Compatibility** | N→N+1 version support required for all APIs |
| **Deprecation** | Minimum 2 release deprecation window before removal |
| **Generated Code** | Never hand-edit `zz_generated*` files; use `make generate` |
| **Code Style** | Follow `CONVENTIONS.md` in the enhancements repo |

## Related

- [upgrade-strategies.md](../../platform/openshift-specifics/upgrade-strategies.md) - Version skew during upgrades
- [controller-runtime.md](../../platform/operator-patterns/controller-runtime.md) - Operator development framework
- **Authoritative**: `./dev-guide/` for development conventions, `./guidelines/` for enhancement process
