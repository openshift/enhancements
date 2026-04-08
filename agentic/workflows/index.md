# Workflows Index

**Last Updated**: 2026-04-08  

## Overview

Common workflows for OpenShift development.

## Workflows

| Workflow | Purpose | File |
|----------|---------|------|
| Enhancement Process | Propose new features | [enhancement-process.md](./enhancement-process.md) |
| Implementing Features | Build approved enhancements | [implementing-features.md](./implementing-features.md) |

## Quick Reference

### Enhancement Process

1. Write proposal (enhancements/<area>/<feature>.md)
2. Submit PR for review
3. Address feedback
4. Get approval (area owner + API reviewer)
5. Implement

See [enhancement-process.md](./enhancement-process.md)

### Implementation

1. Break down work
2. API changes first (openshift/api)
3. Implement controller
4. Add tests (unit, integration, e2e)
5. Test upgrades
6. Submit PRs

See [implementing-features.md](./implementing-features.md)

## Timeline Expectations

| Activity | Duration |
|----------|----------|
| Enhancement review | 4-7 weeks |
| API implementation | 1-2 weeks |
| Controller implementation | 2-4 weeks |
| Testing | 2-3 weeks |
| Code review | 1-2 weeks |
| **Total** | **10-18 weeks** |

## See Also

- [Development Practices](../practices/development/) - Git workflow, code review
- [Testing Practices](../practices/testing/) - Test requirements
- [ADRs](../decisions/) - Architectural decisions
