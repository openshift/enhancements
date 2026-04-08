# CustomResourceDefinitions (CRDs)

**Type**: Kubernetes Extension Concept  
**Last Updated**: 2026-04-08  

## Overview

CRDs extend the Kubernetes API by defining custom resource types with schemas, validation, and versioning.

## Basic CRD

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: machines.machine.openshift.io
spec:
  group: machine.openshift.io
  names:
    kind: Machine
    plural: machines
    singular: machine
    shortNames:
    - ma
  scope: Namespaced  # or Cluster
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              providerID:
                type: string
          status:
            type: object
            properties:
              phase:
                type: string
```

## Using Custom Resources

```yaml
apiVersion: machine.openshift.io/v1
kind: Machine
metadata:
  name: worker-1
  namespace: openshift-machine-api
spec:
  providerID: aws:///us-west-2a/i-1234567890
status:
  phase: Running
```

## Validation

```yaml
schema:
  openAPIV3Schema:
    type: object
    required: ["spec"]
    properties:
      spec:
        type: object
        required: ["replicas"]
        properties:
          replicas:
            type: integer
            minimum: 1
            maximum: 10
          mode:
            type: string
            enum: ["auto", "manual"]
```

## Subresources

### Status Subresource

```yaml
versions:
- name: v1
  subresources:
    status: {}  # Enables /status endpoint
```

**Effect**: `spec` and `status` updated separately, `status.observedGeneration` tracking

### Scale Subresource

```yaml
subresources:
  scale:
    specReplicasPath: .spec.replicas
    statusReplicasPath: .status.replicas
    labelSelectorPath: .status.selector
```

**Effect**: Enables `kubectl scale`

## Multi-Version CRDs

```yaml
versions:
- name: v1
  served: true
  storage: true
  schema: {...}
- name: v1beta1
  served: true
  storage: false
  schema: {...}
  deprecated: true
  deprecationWarning: "v1beta1 is deprecated, use v1"
```

## Printer Columns

```yaml
versions:
- name: v1
  additionalPrinterColumns:
  - name: Phase
    type: string
    jsonPath: .status.phase
  - name: Age
    type: date
    jsonPath: .metadata.creationTimestamp
```

**Effect**: Custom columns in `kubectl get`

## Controller Pattern

```go
// Watch CRD
controller.NewControllerManagedBy(mgr).
    For(&machineapi.Machine{}).
    Owns(&corev1.Pod{}).
    Complete(reconciler)

// Reconcile
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    machine := &machineapi.Machine{}
    err := r.Get(ctx, req.NamespacedName, machine)
    // Reconcile logic
}
```

## References

- **Kubernetes CRDs**: https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definitions/
- **Controller Runtime**: [../../platform/operator-patterns/controller-runtime.md](../../platform/operator-patterns/controller-runtime.md)
