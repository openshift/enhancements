# Webhooks Pattern

**Category**: Platform Pattern  
**Applies To**: Operators with API validation/mutation needs  
**Last Updated**: 2026-04-28  

## Overview

Webhooks extend Kubernetes API server with custom validation, mutation, and conversion logic. They intercept API requests before objects are persisted to etcd.

**Types**: ValidatingWebhook, MutatingWebhook, ConversionWebhook

## Key Concepts

- **Admission Webhook**: Intercepts CREATE/UPDATE/DELETE operations
- **Validating**: Approve or reject requests (cannot modify)
- **Mutating**: Modify requests before persistence (set defaults, inject sidecars)
- **Conversion**: Convert between API versions (v1alpha1 ↔ v1beta1)
- **Fail Policy**: FailClosed (reject on error) vs FailOpen (allow on error)

## Webhook Types

| Type | Purpose | Can Modify | Use Cases |
|------|---------|------------|-----------|
| **Validating** | Enforce invariants | No | Reject invalid configs |
| **Mutating** | Set defaults, inject | Yes | Default values, sidecar injection |
| **Conversion** | API version migration | Yes | v1alpha1 ↔ v1 |

## Implementation

### Validating Webhook

```go
import (
    "k8s.io/api/admission/v1"
    "sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

type MyValidator struct {
    decoder *admission.Decoder
}

func (v *MyValidator) Handle(ctx context.Context, req admission.Request) admission.Response {
    obj := &MyResource{}
    if err := v.decoder.Decode(req, obj); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }
    
    // Validate
    if obj.Spec.Replicas > 10 {
        return admission.Denied("replicas cannot exceed 10")
    }
    
    if obj.Spec.Image == "" {
        return admission.Denied("image is required")
    }
    
    return admission.Allowed("")
}
```

### Mutating Webhook

```go
func (m *MyMutator) Handle(ctx context.Context, req admission.Request) admission.Response {
    obj := &MyResource{}
    if err := m.decoder.Decode(req, obj); err != nil {
        return admission.Errored(http.StatusBadRequest, err)
    }
    
    // Set defaults
    if obj.Spec.Replicas == 0 {
        obj.Spec.Replicas = 3
    }
    
    if obj.Spec.Strategy == "" {
        obj.Spec.Strategy = "RollingUpdate"
    }
    
    // Return patched object
    return admission.Patched("defaults applied", obj)
}
```

### Conversion Webhook

```go
func (c *MyConverter) ConvertTo(dst conversion.Hub) error {
    // Convert from this version to hub version
    dstObj := dst.(*v1.MyResource)
    dstObj.Spec.NewField = c.Spec.OldField
    return nil
}

func (c *MyConverter) ConvertFrom(src conversion.Hub) error {
    // Convert from hub version to this version
    srcObj := src.(*v1.MyResource)
    c.Spec.OldField = srcObj.Spec.NewField
    return nil
}
```

## Best Practices

1. **Fail Policies**:
   - **FailClosed** (default): Reject requests if webhook unavailable (safer)
   - **FailOpen**: Allow requests if webhook unavailable (for non-critical validation)

2. **Idempotent Mutations**: Applying mutation multiple times = same result
   ```go
   // ✅ Idempotent
   if obj.Labels == nil {
       obj.Labels = make(map[string]string)
   }
   obj.Labels["injected"] = "true"
   
   // ❌ Not idempotent
   obj.Spec.Env = append(obj.Spec.Env, newVar)
   ```

3. **Validation Order**: Mutating webhooks run before validating webhooks

4. **Namespace Selectors**: Limit webhook scope to avoid performance issues
   ```yaml
   namespaceSelector:
     matchExpressions:
     - key: admission.openshift.io/ignore
       operator: DoesNotExist
   ```

5. **Timeout**: Default 10s (can configure 1-30s)

## Common Patterns

### WebhookConfiguration

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingWebhookConfiguration
metadata:
  name: my-validator
webhooks:
- name: validate.myresource.example.com
  clientConfig:
    service:
      name: my-webhook-service
      namespace: my-operator
      path: /validate
    caBundle: <base64-ca-cert>
  rules:
  - operations: ["CREATE", "UPDATE"]
    apiGroups: ["example.com"]
    apiVersions: ["v1"]
    resources: ["myresources"]
  failurePolicy: Fail
  sideEffects: None
  admissionReviewVersions: ["v1"]
  timeoutSeconds: 10
```

### Certificate Management

```go
// Use cert-manager or controller-runtime's cert provisioning
import (
    "sigs.k8s.io/controller-runtime/pkg/webhook"
)

func main() {
    mgr, _ := ctrl.NewManager(cfg, ctrl.Options{
        CertDir: "/tmp/k8s-webhook-server/serving-certs",
    })
    
    // Certs automatically provisioned
    mgr.GetWebhookServer().Register("/validate", &webhook.Admission{
        Handler: &MyValidator{},
    })
}
```

### Validation Example: Cross-Field

```go
func validateCrossField(obj *MyResource) error {
    // Ensure replicas matches tier
    if obj.Spec.Tier == "production" && obj.Spec.Replicas < 3 {
        return fmt.Errorf("production tier requires at least 3 replicas")
    }
    
    // Ensure resources set for production
    if obj.Spec.Tier == "production" && obj.Spec.Resources == nil {
        return fmt.Errorf("production tier requires resource limits")
    }
    
    return nil
}
```

## Testing Webhooks

```go
func TestWebhook(t *testing.T) {
    decoder, _ := admission.NewDecoder(scheme)
    validator := &MyValidator{decoder: decoder}
    
    obj := &MyResource{
        Spec: MyResourceSpec{
            Replicas: 15, // Invalid
        },
    }
    
    req := admission.Request{
        AdmissionRequest: v1.AdmissionRequest{
            Object: runtime.RawExtension{Object: obj},
        },
    }
    
    resp := validator.Handle(context.TODO(), req)
    assert.False(t, resp.Allowed)
    assert.Contains(t, resp.Result.Message, "cannot exceed 10")
}
```

## Examples in Components

| Component | Webhook Type | Purpose |
|-----------|--------------|---------|
| machine-api-operator | Validating | Validate Machine/MachineSet specs |
| kube-apiserver | Mutating | Inject service account tokens |
| cluster-network-operator | Validating | Prevent invalid network config changes |

## Performance Considerations

- **Timeout**: Keep webhook logic fast (<1s ideal, 10s max)
- **Namespace Filtering**: Use selectors to reduce webhook invocations
- **Caching**: Cache expensive lookups (don't query API in every request)
- **Monitoring**: Track webhook latency and failure rates

```promql
# Webhook latency
histogram_quantile(0.99, rate(admission_webhook_request_duration_seconds_bucket[5m]))

# Webhook rejections
rate(admission_webhook_rejections_total[5m])
```

## Antipatterns

❌ **Slow webhooks**: Blocking API server for >1s  
❌ **External dependencies**: Webhook depends on external service (network partition = cluster unavailable)  
❌ **FailOpen for critical validation**: Allows invalid configs during webhook downtime  
❌ **No namespace filtering**: Webhook called for every pod/deployment in cluster  
❌ **Non-idempotent mutations**: Applying mutation twice gives different results

## References

- **Upstream**: [Admission Controllers](https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/)
- **controller-runtime**: [Webhook Guide](https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation.html)
- **OpenShift**: [Admission Plugins](https://docs.openshift.com/container-platform/latest/architecture/admission-plug-ins.html)
- **Pattern**: Implements "API-First Design" from [DESIGN_PHILOSOPHY.md](../../DESIGN_PHILOSOPHY.md)
