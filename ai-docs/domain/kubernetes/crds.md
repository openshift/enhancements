# Custom Resource Definitions (CRDs)

**Category**: Kubernetes Core API  
**API Group**: apiextensions.k8s.io/v1  
**Last Updated**: 2026-04-28  

## Overview

CustomResourceDefinitions (CRDs) extend the Kubernetes API with new resource types. They allow operators to define domain-specific objects that behave like built-in Kubernetes resources.

**Pattern**: CRD + Controller = Operator

## Key Fields

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: myresources.example.com  # plural.group
spec:
  group: example.com
  names:
    kind: MyResource
    plural: myresources
    singular: myresource
    shortNames: [mr]
  scope: Namespaced  # or Cluster
  versions:
  - name: v1
    served: true
    storage: true  # One version must be storage version
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              replicas:
                type: integer
                minimum: 1
                maximum: 10
          status:
            type: object
            properties:
              observedGeneration:
                type: integer
```

## Key Concepts

- **Group**: API group (e.g., `example.com`)
- **Version**: API version (e.g., `v1`, `v1beta1`)
- **Kind**: Resource type (e.g., `MyResource`)
- **Scope**: Namespaced or Cluster
- **Schema**: OpenAPI v3 validation
- **Storage Version**: Version persisted in etcd
- **Subresources**: `/status`, `/scale` endpoints

## Schema Validation

```yaml
schema:
  openAPIV3Schema:
    type: object
    required: ["spec"]
    properties:
      spec:
        type: object
        required: ["image"]
        properties:
          image:
            type: string
            pattern: '^[a-z0-9\-\.]+/[a-z0-9\-\.]+:[a-z0-9\-\.]+$'
          replicas:
            type: integer
            minimum: 1
            maximum: 100
            default: 3
          tier:
            type: string
            enum: ["development", "staging", "production"]
```

## Versions and Conversion

### Multiple Versions

```yaml
versions:
- name: v1
  served: true
  storage: true  # Storage version
  schema:
    openAPIV3Schema: {...}
  
- name: v1beta1
  served: true
  storage: false  # Deprecated but still served
  schema:
    openAPIV3Schema: {...}
```

### Conversion Webhook

```yaml
spec:
  conversion:
    strategy: Webhook
    webhook:
      clientConfig:
        service:
          name: my-conversion-webhook
          namespace: my-operator
          path: /convert
        caBundle: <base64-ca-cert>
      conversionReviewVersions: ["v1"]
```

## Subresources

### Status Subresource

```yaml
versions:
- name: v1
  served: true
  storage: true
  subresources:
    status: {}  # Enables /status endpoint
  schema:
    openAPIV3Schema:
      type: object
      properties:
        spec: {...}
        status:
          type: object
          properties:
            conditions: {...}
```

**Benefits**:
- Separate RBAC for spec vs status
- Update status without triggering validation webhooks on spec
- ObservedGeneration pattern

### Scale Subresource

```yaml
subresources:
  scale:
    specReplicasPath: .spec.replicas
    statusReplicasPath: .status.replicas
    labelSelectorPath: .status.labelSelector
```

**Enables**: `kubectl scale myresource/foo --replicas=5`

## Printer Columns

```yaml
versions:
- name: v1
  additionalPrinterColumns:
  - name: Replicas
    type: integer
    jsonPath: .spec.replicas
  - name: Available
    type: integer
    jsonPath: .status.availableReplicas
  - name: Age
    type: date
    jsonPath: .metadata.creationTimestamp
```

**Result**:
```bash
$ kubectl get myresources
NAME    REPLICAS   AVAILABLE   AGE
foo     3          3           5m
```

## Best Practices

1. **Schema Validation**: Define comprehensive OpenAPI schema
   - Prevent invalid objects from being created
   - Better than validating in controller

2. **Status Subresource**: Always use for resources with status
   ```yaml
   subresources:
     status: {}
   ```

3. **Storage Version**: Only one version should be `storage: true`
   - This is the version stored in etcd
   - Others converted on read/write

4. **Defaulting**: Use schema defaults when possible
   ```yaml
   replicas:
     type: integer
     default: 3
   ```

5. **Short Names**: Provide kubectl shortcuts
   ```yaml
   shortNames: [co]  # kubectl get co
   ```

## Common Patterns

### Namespaced Resource

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: myresources.example.com
spec:
  group: example.com
  scope: Namespaced
  names:
    kind: MyResource
    plural: myresources
```

### Cluster-Scoped Resource

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: clusterconfigs.example.com
spec:
  group: example.com
  scope: Cluster
  names:
    kind: ClusterConfig
    plural: clusterconfigs
```

## OpenShift Examples

| CRD | Group | Purpose |
|-----|-------|---------|
| ClusterOperator | config.openshift.io | Platform component status |
| ClusterVersion | config.openshift.io | Cluster upgrade orchestration |
| Machine | machine.openshift.io | Node provisioning |
| MachineConfig | machineconfiguration.openshift.io | Node configuration |

## Validation

```bash
# View CRD
oc get crd myresources.example.com -o yaml

# Explain schema
oc explain myresource.spec

# List all CRDs
oc get crds

# View CRD instances
oc get myresources -A
```

## Antipatterns

❌ **Multiple storage versions**: Only one version should have `storage: true`  
❌ **Missing schema**: No OpenAPI validation (allows invalid objects)  
❌ **No status subresource**: Mixing spec and status updates  
❌ **Breaking schema changes**: Removing required fields in new version

## References

- **API**: `oc explain customresourcedefinition`
- **Upstream**: [Extend Kubernetes API](https://kubernetes.io/docs/concepts/extend-kubernetes/api-extension/custom-resources/)
- **OpenShift**: [github.com/openshift/api](https://github.com/openshift/api)
- **Pattern**: Implements "API-First Design" from [DESIGN_PHILOSOPHY.md](../../DESIGN_PHILOSOPHY.md)
