# Webhooks Pattern

**Category**: Platform Pattern  
**Applies To**: Operators with CRD validation, mutation, or conversion needs  
**Last Updated**: 2026-10-23  

## Overview

Admission webhooks intercept API requests to validate or mutate resources before persistence. OpenShift operators use webhooks for enforcing invariants and setting defaults.

**Purpose**: Enforce API constraints and set defaults that cannot be expressed in OpenAPI schemas alone.

## Key Concepts

- **ValidatingWebhook**: Rejects invalid resources (runs after mutation)
- **MutatingWebhook**: Sets defaults and normalizes resources (runs before validation)
- **Conversion Webhook**: Converts between API versions (rarely used in OpenShift)
- **FailurePolicy**: `Fail` (reject on webhook error) or `Ignore` (allow on error)

## Webhook Types

| Type | When It Runs | Purpose | Side Effects |
|------|-------------|---------|--------------|
| **Mutating** | Before validation | Set defaults, inject fields | Modifies the resource |
| **Validating** | After mutation | Enforce invariants | Rejects or allows |
| **Conversion** | On version mismatch | Convert between API versions | Transforms resource |

## Implementation

### Validating Webhook
```go
import (
    "context"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

// +kubebuilder:webhook:path=/validate-myresource,mutating=false,failurePolicy=fail,groups=my.openshift.io,resources=myresources,verbs=create;update,versions=v1,name=vmyresource.my.openshift.io

func (v *MyResourceValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
    obj := &myv1.MyResource{}
    if err := v.decoder.Decode(req, obj); err != nil {
        return admission.Denied(err.Error())
    }

    // Validate invariants
    if obj.Spec.Replicas < 0 {
        return admission.Denied("replicas must be non-negative")
    }

    return admission.Allowed("")
}
```

### Mutating Webhook
```go
// +kubebuilder:webhook:path=/mutate-myresource,mutating=true,failurePolicy=fail,groups=my.openshift.io,resources=myresources,verbs=create;update,versions=v1,name=mmyresource.my.openshift.io

func (m *MyResourceDefaulter) Handle(ctx context.Context, req admission.Request) admission.Response {
    obj := &myv1.MyResource{}
    if err := m.decoder.Decode(req, obj); err != nil {
        return admission.Denied(err.Error())
    }

    // Set defaults — only when Replicas was not explicitly provided.
    // Using a pointer (*int32) for Replicas distinguishes "not set" (nil)
    // from "explicitly set to zero" so users can intentionally scale to 0.
    // With a plain int, Replicas == 0 is ambiguous:
    //   obj.Spec.Replicas == 0  ← could be "user wants zero" OR "field was omitted"
    if obj.Spec.Replicas == nil {
        one := int32(1)
        obj.Spec.Replicas = &one
    }

    marshalled, _ := json.Marshal(obj)
    return admission.PatchResponseFromRaw(req.Object.Raw, marshalled)
}
```

## Best Practices

1. **FailurePolicy=Fail** for validating webhooks (safety first)
2. **Idempotent Mutations**: Running the mutating webhook twice must produce the same result
3. **Fast Execution**: Webhooks add latency to every API call; keep them under 100ms
4. **No External Dependencies**: Webhook should not call external services during admission
5. **Scope Narrowly**: Only intercept the resources and operations you need

## Webhook Configuration

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: myresource-validation
webhooks:
- name: validate.myresource.openshift.io
  clientConfig:
    service:
      name: my-operator-webhook
      namespace: openshift-my-operator
      path: /validate-myresource
  rules:
  - apiGroups: ["my.openshift.io"]
    resources: ["myresources"]
    operations: ["CREATE", "UPDATE"]
  failurePolicy: Fail
  sideEffects: None
```

## OpenShift Considerations

- **Certificate Management**: Use `service.beta.openshift.io/serving-cert-secret-name` annotation for automatic TLS
- **Conversion Webhooks**: OpenShift generally avoids them — prefer API version transitions across releases
- **Webhook Availability**: Must handle webhook pod restarts gracefully during upgrades

## References

- **Upstream**: [Kubernetes Admission Controllers](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/)
- **API**: [admissionregistration.k8s.io/v1](https://pkg.go.dev/k8s.io/api/admissionregistration/v1)
- **Pattern**: Related to [status-conditions.md](./status-conditions.md) for reporting webhook health
