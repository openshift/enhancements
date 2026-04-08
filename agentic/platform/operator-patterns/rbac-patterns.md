# RBAC Patterns

**Category**: Platform Pattern  
**Applies To**: All OpenShift operators  
**Last Updated**: 2026-04-08  

## Overview

Role-Based Access Control (RBAC) patterns for operators following least privilege principle. Operators use ServiceAccounts with specific Roles/ClusterRoles.

## Principle: Least Privilege

Grant only the minimum permissions required for operation. Prefer:
- Namespaced Roles over ClusterRoles
- Specific resources over wildcards
- Specific verbs over `*`

## Standard Pattern

### ServiceAccount

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-operator
  namespace: openshift-my-operator
```

### ClusterRole (Cluster-Scoped Operator)

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-operator
rules:
# Resources operator manages
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Status subresource (separate permission)
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources/status"]
  verbs: ["get", "update", "patch"]

# Resources operator reads
- apiGroups: [""]
  resources: ["configmaps", "secrets"]
  verbs: ["get", "list", "watch"]

# Resources operator creates/manages
- apiGroups: ["apps"]
  resources: ["deployments", "daemonsets"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]

# Cluster-level resources
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "list", "watch"]
```

### ClusterRoleBinding

```yaml
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
  namespace: openshift-my-operator
```

## Permission Scopes

| Scope | Use When | Example |
|-------|----------|---------|
| ClusterRole + ClusterRoleBinding | Cluster-wide operator | CVO, MCO, Machine API |
| ClusterRole + RoleBinding | Namespaced operator reading cluster resources | App operator reading Nodes |
| Role + RoleBinding | Namespace-scoped only | Single-namespace app operator |

## Common Permission Patterns

### Node Management

```yaml
# Reading nodes (common)
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["get", "list", "watch"]

# Modifying nodes (rare - only MCO, Machine API)
- apiGroups: [""]
  resources: ["nodes"]
  verbs: ["patch", "update"]

# Draining nodes (kubelet-serving-ca-operator)
- apiGroups: [""]
  resources: ["pods/eviction"]
  verbs: ["create"]
```

### ConfigMap/Secret Access

```yaml
# Read any ConfigMap/Secret in operator namespace
- apiGroups: [""]
  resources: ["configmaps", "secrets"]
  verbs: ["get", "list", "watch"]
  # Better: scope to specific names using resourceNames

# Scoped to specific resources
- apiGroups: [""]
  resources: ["secrets"]
  resourceNames: ["my-operator-tls"]
  verbs: ["get"]
```

### Status Subresource

```yaml
# Always separate status permissions
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]

- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources/status"]
  verbs: ["get", "update", "patch"]
```

### ClusterOperator Status

```yaml
# Report operator status to CVO
- apiGroups: ["config.openshift.io"]
  resources: ["clusteroperators"]
  verbs: ["get", "list", "watch"]

- apiGroups: ["config.openshift.io"]
  resources: ["clusteroperators/status"]
  verbs: ["update", "patch"]
```

### Leader Election

```yaml
# Lease-based (recommended)
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "create", "update"]

# ConfigMap-based (legacy)
- apiGroups: [""]
  resources: ["configmaps"]
  resourceNames: ["my-operator-lock"]
  verbs: ["get", "update"]
```

## Anti-Patterns

### ❌ Wildcard Permissions

```yaml
# DON'T: Too broad
- apiGroups: ["*"]
  resources: ["*"]
  verbs: ["*"]
```

### ❌ Unnecessary Cluster Admin

```yaml
# DON'T: Escalates to cluster-admin
- apiGroups: ["rbac.authorization.k8s.io"]
  resources: ["clusterroles", "clusterrolebindings"]
  verbs: ["create", "update", "patch", "delete"]
```

### ❌ Overly Broad Secrets Access

```yaml
# DON'T: Access all secrets cluster-wide
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get", "list", "watch"]
  # Without namespace or resourceNames scope
```

## Deployment Integration

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-operator
  namespace: openshift-my-operator
spec:
  template:
    spec:
      serviceAccountName: my-operator  # Links to ServiceAccount
      containers:
      - name: operator
        image: my-operator:latest
```

## Debugging RBAC Issues

```bash
# Check ServiceAccount
oc get sa my-operator -n openshift-my-operator

# Check ClusterRole
oc get clusterrole my-operator -o yaml

# Check ClusterRoleBinding
oc get clusterrolebinding my-operator -o yaml

# Test permissions
oc auth can-i get nodes --as=system:serviceaccount:openshift-my-operator:my-operator

# Check what ServiceAccount can do
oc policy who-can get nodes

# Describe all permissions for ServiceAccount
oc describe clusterrole my-operator
```

## Examples in Components

| Component | Scope | Key Permissions |
|-----------|-------|----------------|
| machine-config-operator | Cluster | Nodes (update), MachineConfigs, MachineConfigPools |
| cluster-network-operator | Cluster | NetworkAttachmentDefinitions, Network, Nodes |
| machine-api-operator | Cluster | Machines, MachineSets, Nodes, Pods/eviction |
| cluster-version-operator | Cluster | ClusterOperators, ClusterVersion, all operators |
| ingress-operator | Cluster | IngressControllers, Services, Routes |

## Security Considerations

1. **Audit logs**: RBAC denials appear in audit logs
2. **Namespace isolation**: Use namespaced roles when possible
3. **Rotate ServiceAccount tokens**: Automatic with projected volumes
4. **Monitor permission usage**: Detect unused broad permissions
5. **Review regularly**: Permissions drift over time

## References

- **K8s RBAC**: https://kubernetes.io/docs/reference/access-authn-authz/rbac/
- **Security Guidelines**: [rbac-guidelines.md](../../practices/security/rbac-guidelines.md)
- **ServiceAccount**: https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/
