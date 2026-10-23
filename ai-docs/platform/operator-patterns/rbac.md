# RBAC Patterns

**Category**: Platform Pattern  
**Applies To**: All operators and platform components  
**Last Updated**: 2026-10-23  

## Overview

Role-Based Access Control (RBAC) governs what actions service accounts can perform. OpenShift operators require carefully scoped permissions for their service accounts.

**Purpose**: Grant operators minimum necessary permissions following the principle of least privilege.

## Key Concepts

- **ServiceAccount**: Identity for operator pods
- **ClusterRole / Role**: Set of permissions (cluster-wide or namespaced)
- **ClusterRoleBinding / RoleBinding**: Grants a role to a service account
- **Aggregated ClusterRoles**: Combine permissions from multiple ClusterRoles via labels

## Permission Scoping

| Scope | Resource | When to Use |
|-------|----------|-------------|
| **ClusterRole + ClusterRoleBinding** | Cluster-wide resources (Nodes, ClusterOperators) | Operator manages cluster-scoped resources |
| **ClusterRole + RoleBinding** | Namespaced resources across multiple namespaces | Operator manages resources in specific namespaces |
| **Role + RoleBinding** | Single namespace | Operator only works in its own namespace |

## Implementation

### Kubebuilder RBAC Markers
```go
// +kubebuilder:rbac:groups=my.openshift.io,resources=myresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=my.openshift.io,resources=myresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=my.openshift.io,resources=myresources/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
```

### ClusterRole Example
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-operator
rules:
- apiGroups: ["my.openshift.io"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["my.openshift.io"]
  resources: ["myresources/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
```

## Best Practices

1. **Least Privilege**: Only grant verbs and resources the operator actually uses
2. **Status Subresource**: Separate status permissions from spec permissions
3. **No Wildcard Verbs**: Avoid `verbs: ["*"]` — enumerate needed verbs explicitly
4. **Namespace Awareness**: Use namespaced roles when cluster-wide access is unnecessary
5. **Audit Regularly**: Review RBAC rules when adding new controller logic

## Common Patterns

### Leader Election RBAC
```yaml
# Required for leader election
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

### Event Recording
```yaml
# Required to emit Kubernetes events
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

### Aggregated ClusterRoles
```yaml
# Allow view role to see custom resources
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: myresource-view
  labels:
    rbac.authorization.k8s.io/aggregate-to-view: "true"
rules:
- apiGroups: ["my.openshift.io"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch"]
```

## OpenShift Considerations

- **SCC (Security Context Constraints)**: OpenShift uses SCCs instead of PodSecurityPolicies
- **OLM-managed RBAC**: If deployed via OLM, RBAC is defined in the ClusterServiceVersion
- **Privileged Namespace**: Operators in `openshift-*` namespaces run with elevated trust

## Antipatterns

❌ **Wildcard permissions**: `verbs: ["*"]` or `resources: ["*"]` grants too much access  
❌ **cluster-admin binding**: Never bind cluster-admin to operator service accounts  
❌ **Stale permissions**: Removing controller logic without removing corresponding RBAC  
❌ **Missing status permissions**: Forgetting to grant update on `/status` subresource

## References

- **Upstream**: [Kubernetes RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- **OpenShift**: [SCC Documentation](https://docs.openshift.com/container-platform/latest/authentication/managing-security-context-constraints.html)
- **API**: [rbac.authorization.k8s.io/v1](https://pkg.go.dev/k8s.io/api/rbac/v1)
