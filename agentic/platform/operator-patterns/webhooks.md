# Webhooks Pattern

**Category**: Platform Pattern  
**Applies To**: Operators needing admission control  
**Last Updated**: 2026-04-08  

## Overview

Webhooks allow operators to validate or mutate resources before they're persisted in etcd. They're invoked by the Kubernetes API server during the admission control phase.

## Types

| Type | Purpose | When to Use |
|------|---------|-------------|
| Validating | Reject invalid resources | Enforce business rules, schema validation beyond CRD |
| Mutating | Modify resources before storage | Set defaults, inject sidecars, normalize fields |

## Implementation

### Validating Webhook

```go
import (
    "context"
    "net/http"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type MyResourceValidator struct {
    decoder *admission.Decoder
}

func (v *MyResourceValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
    obj := &MyResource{}
    if err := v.decoder.Decode(req, obj); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }
    
    // Validate
    if obj.Spec.Replicas < 1 {
        return admission.Denied("replicas must be >= 1")
    }
    
    if obj.Spec.Image == "" {
        return admission.Denied("image is required")
    }
    
    return admission.Allowed("")
}
```

### Mutating Webhook

```go
import "encoding/json"

func (m *MyResourceMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
    obj := &MyResource{}
    if err := m.decoder.Decode(req, obj); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }
    
    // Set defaults
    if obj.Spec.Replicas == 0 {
        obj.Spec.Replicas = 3
    }
    
    if obj.Spec.Image == "" {
        obj.Spec.Image = "registry.redhat.io/openshift4/default:latest"
    }
    
    marshaledObj, err := json.Marshal(obj)
    if err != nil {
        return admission.Errored(http.StatusInternalServerError, err)
    }
    
    return admission.PatchResponseFromRaw(req.Object.Raw, marshaledObj)
}
```

## Configuration

### ValidatingWebhookConfiguration

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: myresource-validator
webhooks:
- name: myresource.openshift.io
  clientConfig:
    service:
      name: my-operator-webhook
      namespace: openshift-my-operator
      path: /validate-myresource
  rules:
  - apiGroups: ["myapi.openshift.io"]
    apiVersions: ["v1"]
    operations: ["CREATE", "UPDATE"]
    resources: ["myresources"]
  sideEffects: None
  admissionReviewVersions: ["v1"]
  failurePolicy: Fail
  timeoutSeconds: 3
```

### MutatingWebhookConfiguration

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  name: myresource-mutator
webhooks:
- name: myresource-mutator.openshift.io
  clientConfig:
    service:
      name: my-operator-webhook
      namespace: openshift-my-operator
      path: /mutate-myresource
  rules:
  - apiGroups: ["myapi.openshift.io"]
    apiVersions: ["v1"]
    operations: ["CREATE"]
    resources: ["myresources"]
  sideEffects: None
  admissionReviewVersions: ["v1"]
  failurePolicy: Ignore
  timeoutSeconds: 2
```

## Best Practices

1. **Fail open on errors**: Use `failurePolicy: Ignore` for non-critical validation
2. **Short timeouts**: Default 10s is too long, use 2-3s for most cases
3. **Avoid side effects**: Webhooks should be idempotent and stateless
4. **Use object selectors**: Limit webhook scope to relevant objects
5. **Handle DELETE carefully**: Old object may not pass current validation rules
6. **Version compatibility**: Support multiple API versions in validation

## Common Patterns

### Version-Specific Validation

```go
func (v *Validator) Handle(ctx context.Context, req admission.Request) admission.Response {
    switch req.Kind.Version {
    case "v1":
        obj := &MyResourceV1{}
        if err := v.decoder.Decode(req, obj); err != nil {
            return admission.Errored(http.StatusBadRequest, err)
        }
        return v.validateV1(obj)
    case "v2":
        obj := &MyResourceV2{}
        if err := v.decoder.Decode(req, obj); err != nil {
            return admission.Errored(http.StatusBadRequest, err)
        }
        return v.validateV2(obj)
    default:
        return admission.Errored(http.StatusBadRequest, fmt.Errorf("unsupported version: %s", req.Kind.Version))
    }
}
```

### Namespace-Aware Mutation

```go
func (m *Mutator) Handle(ctx context.Context, req admission.Request) admission.Response {
    obj := &MyResource{}
    if err := m.decoder.Decode(req, obj); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }
    
    // Apply namespace-specific defaults
    if obj.Namespace == "openshift-monitoring" {
        obj.Spec.MonitoringEnabled = true
        obj.Spec.Retention = "30d"
    }
    
    marshaledObj, _ := json.Marshal(obj)
    return admission.PatchResponseFromRaw(req.Object.Raw, marshaledObj)
}
```

### Validating Updates Only

```go
func (v *Validator) Handle(ctx context.Context, req admission.Request) admission.Response {
    if req.Operation == admissionv1.Update {
        newObj := &MyResource{}
        oldObj := &MyResource{}
        
        v.decoder.Decode(req, newObj)
        v.decoder.DecodeRaw(req.OldObject, oldObj)
        
        // Prevent immutable field changes
        if newObj.Spec.ImmutableField != oldObj.Spec.ImmutableField {
            return admission.Denied("immutableField cannot be changed")
        }
    }
    
    return admission.Allowed("")
}
```

## Examples in Components

| Component | Webhook Type | Purpose |
|-----------|-------------|---------|
| machine-api-operator | Validating | Prevent invalid machine configurations |
| cluster-network-operator | Mutating | Inject network configuration defaults |
| console-operator | Validating | Ensure console extensions are valid |
| ingress-operator | Validating | Validate IngressController specs |

## Debugging

```bash
# Check webhook configuration
oc get validatingwebhookconfigurations
oc get mutatingwebhookconfigurations

# View webhook logs
oc logs -n openshift-my-operator deployment/my-operator-webhook

# Test webhook locally
curl -k -X POST https://localhost:9443/validate-myresource \
  -H "Content-Type: application/json" \
  -d @admission-request.json
  
# Check webhook certificate
oc get secret -n openshift-my-operator webhook-serving-cert -o yaml
```

## Common Issues

1. **Certificate expiration**: Webhooks use TLS - monitor cert expiry
2. **Timeouts**: Slow webhooks block API requests - keep them fast
3. **Infinite loops**: Mutating webhook creating new objects that trigger itself
4. **Version skew**: Webhook doesn't support all API versions
5. **Failure during upgrade**: Webhook down can block cluster operations

## References

- **K8s Admission Controllers**: https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/
- **controller-runtime Webhooks**: https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation.html
- **Cert Management**: Use cert-manager or service-ca-operator for TLS certs
