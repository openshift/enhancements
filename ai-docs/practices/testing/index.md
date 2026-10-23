# Testing Practices

Testing patterns and guidelines for OpenShift components.

## Documents

- [pyramid.md](pyramid.md) - Testing pyramid: 60% unit / 30% integration / 10% e2e

## Quick Reference

| Test Type | Command | Speed | CI Stage |
|-----------|---------|-------|----------|
| Unit | `make test-unit` | Fast | Pre-submit |
| Integration | `make test-integration` | Medium | Pre-submit |
| E2E | `make test-e2e` | Slow | Post-submit / periodic |

## Related

- [DESIGN_PHILOSOPHY.md](../../DESIGN_PHILOSOPHY.md) - Quality principles
- [controller-runtime.md](../../platform/operator-patterns/controller-runtime.md) - What to test in operators
