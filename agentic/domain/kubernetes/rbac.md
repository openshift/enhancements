# RBAC (Role-Based Access Control)

**Type**: Kubernetes Core Concept  
**Last Updated**: 2026-04-08  

## Overview

RBAC controls access to Kubernetes resources using Roles and RoleBindings.

## Core Concepts

| Resource | Scope | Purpose |
|----------|-------|---------|
| **Role** | Namespace | Permissions within namespace |
| **ClusterRole** | Cluster | Permissions cluster-wide |
| **RoleBinding** | Namespace | Grants Role to subjects |
| **ClusterRoleBinding** | Cluster | Grants ClusterRole to subjects |

## Role Example

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: pod-reader
  namespace: default
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]
```

## ClusterRole Example

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: secret-reader
rules:
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list"]
```

## RoleBinding

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: read-pods
  namespace: default
subjects:
- kind: ServiceAccount
  name: my-sa
  namespace: default
roleRef:
  kind: Role
  name: pod-reader
  apiGroup: rbac.authorization.k8s.io
```

## Verbs

| Verb | Meaning |
|------|---------|
| get | Read single resource |
| list | Read all resources |
| watch | Watch for changes |
| create | Create new resources |
| update | Modify resources |
| patch | Partial updates |
| delete | Remove resources |
| deletecollection | Bulk delete |

## Testing Permissions

```bash
# Can I?
kubectl auth can-i create pods

# Can service account?
kubectl auth can-i get secrets --as=system:serviceaccount:default:my-sa

# List all permissions
kubectl auth can-i --list
```

## References

- **Kubernetes RBAC**: https://kubernetes.io/docs/reference/access-authn-authz/rbac/
- **RBAC Guidelines**: [../../practices/security/rbac-guidelines.md](../../practices/security/rbac-guidelines.md)
