# API Reference

**Purpose**: Pointers to authoritative API documentation  
**Last Updated**: 2026-04-29  

## Primary API Sources

### oc explain (Authoritative)

```bash
# View resource schema
oc explain <resource>

# Examples:
oc explain pod
oc explain clusteroperator
oc explain machine

# View specific field
oc explain pod.spec.containers

# Show all fields recursively
oc explain pod --recursive
```

**Why use `oc explain`?**
- Always up-to-date with cluster version
- Shows exact schema (required fields, types, validation)
- Matches what API server validates

### GitHub API Repository

[github.com/openshift/api](https://github.com/openshift/api)

**Structure**:
```
api/
├── config/v1/              # Platform config APIs
│   ├── types_cluster_operator.go
│   └── types_cluster_version.go
├── machine/v1beta1/        # Machine API
├── operator/v1/            # Operator APIs
└── ...
```

### List All APIs

```bash
# All resource types
oc api-resources

# OpenShift-specific resources
oc api-resources | grep openshift

# By API group
oc api-resources --api-group=config.openshift.io
```

## Core API Groups

| API Group | Purpose | Example Resources |
|-----------|---------|------------------|
| `config.openshift.io` | Platform configuration | ClusterOperator, ClusterVersion |
| `machine.openshift.io` | Node lifecycle | Machine, MachineSet |
| `machineconfiguration.openshift.io` | Node config | MachineConfig, MachineConfigPool |
| `operator.openshift.io` | Operator configs | KubeAPIServer, Etcd |
| `apps` | Kubernetes workloads | Deployment, StatefulSet |
| `core` (v1) | Kubernetes primitives | Pod, Service, ConfigMap |

## API Discovery

### Find API Group for Resource

```bash
# Get API group
oc api-resources | grep <resource-name>

# Example:
$ oc api-resources | grep clusteroperator
clusteroperators     co       config.openshift.io/v1    false   ClusterOperator
```

### Get Full Resource Schema

```bash
# Get as YAML
oc get crd <resource-plural>.<group> -o yaml

# Example:
oc get crd clusteroperators.config.openshift.io -o yaml
```

## Documentation Locations

### Kubernetes APIs

- **Upstream Docs**: [kubernetes.io/docs/reference/kubernetes-api/](https://kubernetes.io/docs/reference/kubernetes-api/)
- **Source**: [github.com/kubernetes/api](https://github.com/kubernetes/api)

### OpenShift APIs

- **Source**: [github.com/openshift/api](https://github.com/openshift/api)
- **Product Docs**: [docs.openshift.com](https://docs.openshift.com/container-platform/latest/)

## Common Queries

### View CRD Schema

```bash
oc get crd myresources.example.com -o jsonpath='{.spec.versions[0].schema.openAPIV3Schema}' | jq
```

### List API Versions

```bash
oc api-versions
```

### Get Resource from Specific API Version

```bash
oc get clusteroperator kube-apiserver -o yaml --show-managed-fields=false
```

## Anti-Staleness Strategy

**Don't include** (gets stale):
- ❌ Full API field documentation
- ❌ All available resources list
- ❌ Every API group

**Do include** (stays current):
- ✅ How to discover APIs (`oc explain`, `oc api-resources`)
- ✅ Core stable API groups only
- ✅ Links to authoritative sources

## Related

- **Glossary**: [glossary.md](glossary.md) - Core terminology
- **Domain Concepts**: [../domain/](../domain/) - Key API documentation
- **CRD Guide**: [../domain/kubernetes/crds.md](../domain/kubernetes/crds.md)
