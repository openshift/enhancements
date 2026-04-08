---
title: Use operator-sdk for New Operators
status: Accepted
date: 2026-04-08
affected_components:
  - All new operators
---

# ADR 0001: Use operator-sdk for New Operators

## Status

**Accepted**

## Context

OpenShift needs a standardized approach for building Kubernetes operators. Multiple frameworks exist (operator-sdk, kubebuilder, custom controllers).

## Decision

Use operator-sdk (based on kubebuilder) for all new OpenShift operators.

## Rationale

- ✅ **Industry standard**: Based on kubebuilder, widely adopted
- ✅ **Code generation**: Automatic RBAC, CRD, webhook scaffolding
- ✅ **Best practices**: Enforces controller-runtime patterns
- ✅ **OLM integration**: Built-in Operator Lifecycle Manager support
- ✅ **Testing tools**: Integration and E2E test helpers

## Alternatives Considered

### Custom controller with client-go

- **Pro**: Full control, minimal dependencies
- **Con**: Reinvent the wheel, inconsistent patterns across operators

### Kubebuilder directly

- **Pro**: Upstream project, well documented
- **Con**: operator-sdk adds OpenShift-specific features (OLM, bundle creation)

### Metacontroller

- **Pro**: Declarative, no code needed
- **Con**: Limited flexibility, not suitable for complex operators

## Consequences

**Positive**:
- Consistent operator structure across OpenShift
- Faster development with scaffolding
- Automatic updates to best practices via operator-sdk upgrades

**Negative**:
- Learning curve for operator-sdk
- Dependency on external tool
- Must stay current with operator-sdk versions

## Implementation

### New Operator Scaffolding

```bash
operator-sdk init --domain=openshift.io --repo=github.com/openshift/my-operator
operator-sdk create api --group=myapi --version=v1 --kind=MyResource
```

### Migration from Custom Controllers

Existing custom controllers can continue but new features should use operator-sdk patterns.

## Affected Components

- **All new operators**: Must use operator-sdk
- **Existing operators**: May migrate opportunistically
- **Component operators**: machine-config-operator, cluster-network-operator, etc.

## Compliance

**Required** for:
- New operators in openshift/* GitHub org
- Operators contributing to OpenShift payload

**Optional** for:
- Community operators in OperatorHub
- Operators outside openshift/* org

## References

- **operator-sdk**: https://sdk.operatorframework.io/
- **controller-runtime**: https://github.com/kubernetes-sigs/controller-runtime
- **Enhancement Pattern**: [../platform/operator-patterns/controller-runtime.md](../platform/operator-patterns/controller-runtime.md)
