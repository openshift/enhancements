# RBAC Patterns

**Category**: Platform Pattern  
**Applies To**: All Operators  
**Last Updated**: 2026-04-29  

## Overview

Role-Based Access Control (RBAC) restricts access to Kubernetes resources. Operators need appropriate permissions to manage resources, and should follow least-privilege principles.

**Pattern**: ServiceAccount + Role/ClusterRole + RoleBinding/ClusterRoleBinding

## Key Concepts

- **ServiceAccount**: Identity for pods
- **Role**: Namespaced permissions
- **ClusterRole**: Cluster-wide permissions
- **RoleBinding**: Grants Role to ServiceAccount (namespaced)
- **ClusterRoleBinding**: Grants ClusterRole to ServiceAccount (cluster-wide)

## RBAC Resources

| Resource | Scope | Use Case |
|----------|-------|----------|
| **Role** | Namespace | Permissions for namespaced resources in one namespace |
| **ClusterRole** | Cluster | Permissions for cluster-scoped or all-namespace resources |
| **RoleBinding** | Namespace | Grant Role to users/groups/ServiceAccounts |
| **ClusterRoleBinding** | Cluster | Grant ClusterRole to users/groups/ServiceAccounts |

## Operator RBAC Pattern

```yaml
# ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-operator
  namespace: my-operator-namespace

---
# ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-operator
rules:
# Watch and manage custom resources
- apiGroups: ["example.com"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Update status subresource
- apiGroups: ["example.com"]
  resources: ["myresources/status"]
  verbs: ["get", "update", "patch"]

# Manage deployments (operands)
- apiGroups: ["apps"]
  resources: ["deployments"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Read configmaps
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]

---
# ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: my-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: my-operator
subjects:
- kind: ServiceAccount
  name: my-operator
  namespace: my-operator-namespace
```

## Best Practices

1. **Least Privilege**: Grant only required permissions
   ```yaml
   # ✅ Good: Specific resources and verbs
   - apiGroups: ["apps"]
     resources: ["deployments"]
     verbs: ["get", "list", "watch", "update"]
   
   # ❌ Bad: Wildcard permissions
   - apiGroups: ["*"]
     resources: ["*"]
     verbs: ["*"]
   ```

2. **Separate Status Permissions**: Use status subresource
   ```yaml
   # Spec updates (user-facing)
   - apiGroups: ["example.com"]
     resources: ["myresources"]
     verbs: ["update", "patch"]
   
   # Status updates (controller-only)
   - apiGroups: ["example.com"]
     resources: ["myresources/status"]
     verbs: ["update", "patch"]
   ```

3. **Use ClusterRole for CRDs**: Even if namespaced, CRD management needs ClusterRole
   ```yaml
   - apiGroups: ["apiextensions.k8s.io"]
     resources: ["customresourcedefinitions"]
     verbs: ["get", "list", "watch"]  # Usually read-only
   ```

4. **Scope Appropriately**: Use Role for namespace-specific operators
   ```yaml
   # Namespace-scoped operator (single namespace)
   kind: Role  # Not ClusterRole
   
   # Multi-namespace operator
   kind: ClusterRole
   ```

## Common Permission Sets

### CRD Management

```yaml
rules:
# Watch CRD
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: ["get", "list", "watch"]

# Manage custom resources
- apiGroups: ["example.com"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Update status
- apiGroups: ["example.com"]
  resources: ["myresources/status"]
  verbs: ["get", "update", "patch"]
```

### Core Resources

```yaml
rules:
# Pods
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]

# Services
- apiGroups: [""]
  resources: ["services"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# ConfigMaps (read-only)
- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["get", "list", "watch"]

# Secrets (read-only, specific names)
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["my-operator-tls"]
  verbs: ["get"]
```

### Events

```yaml
rules:
# Create events for debugging
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
```

### Leader Election

```yaml
rules:
# ConfigMap-based leader election
- apiGroups: [""]
  resources: ["configmaps"]
  resourceNames: ["my-operator-leader"]
  verbs: ["get", "update", "patch"]

- apiGroups: [""]
  resources: ["configmaps"]
  verbs: ["create"]

# Lease-based leader election (preferred)
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "create", "update", "patch"]
```

## OpenShift-Specific RBAC

### Security Context Constraints (SCCs)

```yaml
# ClusterRole to use specific SCC
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-operator-scc
rules:
- apiGroups: ["security.openshift.io"]
  resources: ["securitycontextconstraints"]
  resourceNames: ["privileged"]  # Or custom SCC
  verbs: ["use"]
```

### OpenShift Config Resources

```yaml
rules:
# Read cluster config
- apiGroups: ["config.openshift.io"]
  resources: ["clusterversions", "infrastructures", "networks"]
  verbs: ["get", "list", "watch"]

# Update ClusterOperator status
- apiGroups: ["config.openshift.io"]
  resources: ["clusteroperators"]
  verbs: ["get", "list", "watch"]

- apiGroups: ["config.openshift.io"]
  resources: ["clusteroperators/status"]
  verbs: ["update", "patch"]
```

## Kubebuilder Markers

```go
// Generate RBAC manifests using kubebuilder markers

//+kubebuilder:rbac:groups=example.com,resources=myresources,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=example.com,resources=myresources/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups="",resources=events,verbs=create;patch

func (r *MyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // Controller logic
}
```

**Generate manifests**: `make manifests` creates RBAC YAML from markers.

## Debugging RBAC

### Check Permissions

```bash
# Can I create deployments?
oc auth can-i create deployments --as=system:serviceaccount:my-ns:my-sa

# What can this SA do?
oc auth can-i --list --as=system:serviceaccount:my-ns:my-sa

# View ClusterRole
oc describe clusterrole my-operator

# View bindings
oc get clusterrolebinding | grep my-operator
```

### Common Errors

**Error**: `forbidden: User "system:serviceaccount:my-ns:my-sa" cannot create resource`

**Cause**: Missing RBAC permissions

**Fix**:
1. Check ClusterRole has required verbs
2. Verify ClusterRoleBinding references correct SA
3. Ensure SA exists

## Examples in Components

| Component | RBAC Pattern | Notes |
|-----------|-------------|-------|
| machine-api-operator | ClusterRole for Machines | Cluster-scoped resources |
| kube-apiserver | ClusterRole + privileged SCC | Needs elevated permissions |
| cluster-network-operator | ClusterRole for network config | Cluster-wide networking |

## Antipatterns

❌ **Wildcard permissions**: `apiGroups: ["*"]`, `resources: ["*"]`  
❌ **Overly broad verbs**: `verbs: ["*"]` instead of specific verbs  
❌ **No status subresource**: Mixing spec and status permissions  
❌ **Hardcoded namespace**: ClusterRoleBinding with hardcoded namespace  
❌ **Secrets without resourceNames**: Allowing read of all secrets

## References

- **Kubernetes**: [RBAC](https://kubernetes.io/docs/reference/access-authn-authz/rbac/)
- **OpenShift**: [RBAC Guide](https://docs.openshift.com/container-platform/latest/authentication/using-rbac.html)
- **SCCs**: [Security Context Constraints](https://docs.openshift.com/container-platform/latest/authentication/managing-security-context-constraints.html)
- **Kubebuilder**: [RBAC Markers](https://book.kubebuilder.io/reference/markers/rbac.html)
