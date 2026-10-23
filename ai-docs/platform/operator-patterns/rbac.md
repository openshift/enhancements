# RBAC Patterns

**Category**: Operator Pattern
**Last Updated**: 2026-10-23

---

## Overview

Role-Based Access Control (RBAC) in OpenShift extends Kubernetes RBAC with additional roles, SCCs, and project-scoped bindings. Operators must follow least-privilege principles for their ServiceAccounts.

## RBAC Components

| Resource | Scope | Purpose |
|----------|-------|---------|
| `ClusterRole` | Cluster-wide | Define permissions across all namespaces |
| `ClusterRoleBinding` | Cluster-wide | Bind ClusterRole to subjects |
| `Role` | Namespace | Define permissions within a namespace |
| `RoleBinding` | Namespace | Bind Role or ClusterRole within a namespace |

## Operator RBAC Conventions

1. **Separate ServiceAccounts** for operator and operand workloads
2. **Minimize cluster-scoped permissions** — prefer namespace-scoped where possible
3. **Use RBAC markers** in controller-runtime projects: `// +kubebuilder:rbac:groups=...`
4. **Document RBAC requirements** in the operator's ClusterServiceVersion (CSV) for OLM-managed operators
5. **Never use `cluster-admin`** for operator ServiceAccounts

## CVO-Managed Operators

For operators managed by the Cluster Version Operator:
- RBAC manifests live in the operator's `manifests/` directory
- The CVO applies these during installation and upgrades
- Changes to RBAC require bumping the operator image in the release payload

## OLM-Managed Operators

For OLM-managed operators:
- RBAC is declared in the CSV (`spec.install.spec.permissions` and `spec.install.spec.clusterPermissions`)
- OLM creates and manages the ServiceAccount, Roles, and Bindings
- RBAC changes require a new CSV version

## Auditing RBAC

```bash
# List all ClusterRoleBindings for a ServiceAccount
oc get clusterrolebindings -o json | jq '.items[] | select(.subjects[]?.name == "my-operator")'

# Check what a ServiceAccount can do
oc auth can-i --list --as=system:serviceaccount:my-namespace:my-operator
```

## Platform Documentation

For operator development conventions, see [dev-guide/operators.md](../../../dev-guide/operators.md). For security practices, see [practices/security/](../../practices/security/).
