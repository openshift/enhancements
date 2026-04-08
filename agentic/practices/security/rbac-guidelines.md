# RBAC Guidelines

**Category**: Engineering Practice  
**Applies To**: All OpenShift components  
**Last Updated**: 2026-04-08  

## Overview

Role-Based Access Control (RBAC) principles and patterns for OpenShift operators and controllers.

## Principle: Least Privilege

Grant only the minimum permissions required for functionality.

| Bad | Good |
|-----|------|
| `cluster-admin` | Specific resources and verbs |
| `ClusterRole` for everything | `Role` when namespace-scoped works |
| `verbs: ["*"]` | Explicit verb list |
| All resources | Only resources actually used |

## ServiceAccount Design

### One ServiceAccount Per Component

```yaml
# Bad: Shared ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: shared-sa
  namespace: openshift-operator
---
# Multiple components use same SA (overprivileged)

# Good: Separate ServiceAccounts
apiVersion: v1
kind: ServiceAccount
metadata:
  name: controller-sa
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: webhook-sa
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: cleanup-sa
```

### ServiceAccount Naming

```
<component>-<function>

Examples:
- machine-config-daemon
- cluster-version-operator
- console-operator-webhook
```

## Role vs ClusterRole

| Use Role | Use ClusterRole |
|----------|-----------------|
| Namespace-scoped resources | Cluster-scoped resources |
| Single namespace access | Multi-namespace access |
| Tenant workloads | Platform operators |

### Example: Role

```yaml
# Operator managing resources in its own namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: my-operator-role
  namespace: openshift-my-operator
rules:
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: [""]
  resources: ["configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
```

### Example: ClusterRole

```yaml
# Operator managing cluster-wide resources
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: machine-config-operator
rules:
- apiGroups: ["machineconfiguration.openshift.io"]
  resources: ["machineconfigs", "machineconfigpools"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "list", "watch", "update", "patch"]
```

## Verb Permissions

| Verb | Purpose | Risk Level |
|------|---------|------------|
| `get` | Read single resource | Low |
| `list` | Read all resources | Low |
| `watch` | Subscribe to changes | Low |
| `create` | Create new resources | Medium |
| `update` | Modify existing resources | Medium |
| `patch` | Partial updates | Medium |
| `delete` | Remove resources | High |
| `deletecollection` | Bulk delete | Very High |
| `*` | All verbs | **Never use** |

### Minimal Verb Sets

**Read-only**:
```yaml
verbs: ["get", "list", "watch"]
```

**Standard controller**:
```yaml
verbs: ["get", "list", "watch", "create", "update", "patch"]
# Avoid delete unless necessary
```

**With cleanup**:
```yaml
verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
```

## Resource Permissions

### Scope to Specific Resources

```yaml
# Bad: Too broad
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]

# Good: Specific
rules:
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "watch", "update"]
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
  resourceNames: ["my-specific-configmap"]  # Even more restrictive
```

### SubResources

```yaml
# Status updates require separate permission
rules:
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources/finalizers"]
  verbs: ["update"]
```

## Dangerous Permissions

### Never Grant These

```yaml
# NEVER: Full cluster admin
roleRef:
  kind: ClusterRole
  name: cluster-admin

# NEVER: Privilege escalation
rules:
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["clusterroles", "clusterrolebindings"]
  verbs: ["escalate", "bind"]

# NEVER: Impersonation (unless absolutely required)
rules:
- apiGroups: [""]
  resources: ["users", "groups", "serviceaccounts"]
  verbs: ["impersonate"]

# NEVER: All resources
rules:
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
```

### Use With Caution

```yaml
# Nodes (critical infrastructure)
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["delete"]  # Only for cluster-api

# Namespaces (tenant isolation)
- apiGroups: [""]
  resources: ["namespaces"]
  verbs: ["delete"]  # Only for cleanup controllers

# CRDs (API changes)
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: ["create", "delete"]  # Only for operators managing CRDs
```

## RoleBinding vs ClusterRoleBinding

### RoleBinding

```yaml
# Grants Role permissions in specific namespace
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-operator-binding
  namespace: openshift-my-operator
subjects:
- kind: ServiceAccount
  name: my-operator
  namespace: openshift-my-operator
roleRef:
  kind: Role
  name: my-operator-role
  apiGroup: rbac.authorization.k8s.io
```

### ClusterRoleBinding

```yaml
# Grants ClusterRole permissions cluster-wide
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: machine-config-operator
subjects:
- kind: ServiceAccount
  name: machine-config-operator
  namespace: openshift-machine-config-operator
roleRef:
  kind: ClusterRole
  name: machine-config-operator
  apiGroup: rbac.authorization.k8s.io
```

### ClusterRole + RoleBinding

```yaml
# Use ClusterRole in specific namespace
# Useful for reusing ClusterRole definitions
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: operator-in-namespace
  namespace: user-namespace
subjects:
- kind: ServiceAccount
  name: my-operator
  namespace: openshift-my-operator
roleRef:
  kind: ClusterRole  # Reference ClusterRole
  name: my-operator-role
  apiGroup: rbac.authorization.k8s.io
# Grants permissions ONLY in user-namespace
```

## Testing RBAC

### Verify Permissions

```bash
# Check if ServiceAccount can perform action
oc auth can-i get pods --as=system:serviceaccount:openshift-my-operator:my-operator

# List all permissions for ServiceAccount
oc policy can-i --list --as=system:serviceaccount:openshift-my-operator:my-operator

# Verify minimal permissions
oc auth can-i delete nodes --as=system:serviceaccount:openshift-my-operator:my-operator
# Should return "no" unless specifically needed
```

### Unit Test RBAC

```go
func TestServiceAccountPermissions(t *testing.T) {
    // Test that SA has required permissions
    requiredPermissions := []struct {
        verb     string
        resource string
        group    string
    }{
        {"get", "deployments", "apps"},
        {"list", "deployments", "apps"},
        {"update", "deployments", "apps"},
    }
    
    for _, perm := range requiredPermissions {
        allowed := checkPermission(perm.verb, perm.resource, perm.group)
        if !allowed {
            t.Errorf("Missing permission: %s %s.%s", perm.verb, perm.resource, perm.group)
        }
    }
    
    // Test that SA doesn't have dangerous permissions
    forbidden := []struct {
        verb     string
        resource string
    }{
        {"delete", "nodes"},
        {"*", "*"},
    }
    
    for _, perm := range forbidden {
        allowed := checkPermission(perm.verb, perm.resource, "")
        if allowed {
            t.Errorf("Has forbidden permission: %s %s", perm.verb, perm.resource)
        }
    }
}
```

## Common Patterns

### Controller Pattern

```yaml
# Standard controller RBAC
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-controller
  namespace: openshift-my-operator
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-controller
rules:
# Watch custom resources
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch", "update", "patch"]
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources/status"]
  verbs: ["update", "patch"]
# Manage owned resources
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
# Read config
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]
# Record events
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

### Webhook Pattern

```yaml
# Webhook server RBAC
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-webhook
rules:
# Read resources for validation
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources"]
  verbs: ["get", "list"]
# No write permissions needed
```

### Status-Only Pattern

```yaml
# Only update status subresource
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: status-updater
rules:
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources/status"]
  verbs: ["update", "patch"]
```

## Aggregation

```yaml
# Base role
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: view-myresources
  labels:
    rbac.authorization.k8s.io/aggregate-to-view: "true"
    rbac.authorization.k8s.io/aggregate-to-edit: "true"
    rbac.authorization.k8s.io/aggregate-to-admin: "true"
rules:
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch"]

# Edit role
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: edit-myresources
  labels:
    rbac.authorization.k8s.io/aggregate-to-edit: "true"
    rbac.authorization.k8s.io/aggregate-to-admin: "true"
rules:
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources"]
  verbs: ["create", "update", "patch", "delete"]
```

## Examples by Component

| Component | ServiceAccounts | Permissions Level |
|-----------|----------------|-------------------|
| machine-config-operator | 1 (operator) | High (node management) |
| cluster-network-operator | 1 (operator) | High (network config) |
| console-operator | 2 (operator, downloads) | Medium (console resources) |

## References

- **Kubernetes RBAC**: https://kubernetes.io/docs/reference/access-authn-authz/rbac/
- **OpenShift RBAC**: https://docs.openshift.com/container-platform/latest/authentication/using-rbac.html
- **Threat Modeling**: [threat-modeling.md](./threat-modeling.md)
- **Secrets Management**: [secrets-management.md](./secrets-management.md)
