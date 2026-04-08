# OpenShift Specifics Index

**Last Updated**: 2026-04-08  

## Overview

OpenShift-specific platform patterns and concepts that extend Kubernetes.

## Core Concepts

| Concept | Purpose | File |
|---------|---------|------|
| ClusterOperator | Operator status reporting to CVO | [cluster-operators.md](./cluster-operators.md) |
| Operator Lifecycle | Installation, upgrades, removal | [operator-lifecycle.md](./operator-lifecycle.md) |

## ClusterOperator Pattern

All OpenShift operators must:
1. Create ClusterOperator resource on startup
2. Report Available/Progressing/Degraded conditions
3. Update version information during upgrades
4. List related objects for debugging

See [cluster-operators.md](./cluster-operators.md) for details.

## Lifecycle Management

Operators follow standardized lifecycle:
- **Installation**: CVO or OLM deploys operator
- **Available**: Normal operation, reconciliation loops
- **Upgrading**: CVO coordinates version changes
- **Degraded**: Partial functionality, automatic recovery
- **Removal**: Cleanup with finalizers

See [operator-lifecycle.md](./operator-lifecycle.md) for details.

## Related Sections

- **Operator Patterns**: [../operator-patterns/](../operator-patterns/) - Controller implementation patterns
- **OpenShift Domain**: [../../domain/openshift/](../../domain/openshift/) - OpenShift API concepts
- **Kubernetes Domain**: [../../domain/kubernetes/](../../domain/kubernetes/) - Kubernetes fundamentals

## Usage

These patterns apply to all ClusterOperators in OpenShift. Component-specific details belong in component repositories, not here.

## See Also

- [Operator Patterns Index](../operator-patterns/index.md)
- [ClusterVersion](../../domain/openshift/clusterversion.md)
- [Repository Index](../../references/repo-index.md)
