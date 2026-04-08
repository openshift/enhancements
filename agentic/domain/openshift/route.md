# Route

**Type**: OpenShift Platform API  
**API Group**: `route.openshift.io/v1`  
**Last Updated**: 2026-04-08  

## Overview

Route exposes a Service at a hostname for external access. OpenShift alternative to Kubernetes Ingress.

## Basic Route

```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: my-route
  namespace: my-app
spec:
  host: myapp.apps.cluster.example.com
  to:
    kind: Service
    name: my-service
  port:
    targetPort: 8080
```

## TLS Termination

### Edge Termination

```yaml
spec:
  tls:
    termination: edge
    insecureEdgeTerminationPolicy: Redirect
```

**TLS terminated at router**, HTTP to backend

### Passthrough Termination

```yaml
spec:
  tls:
    termination: passthrough
```

**TLS terminated at backend** (encrypted end-to-end)

### Re-encrypt Termination

```yaml
spec:
  tls:
    termination: reencrypt
    destinationCACertificate: <CA-cert>
```

**TLS terminated at router**, re-encrypted to backend

## Custom Domains

```yaml
spec:
  host: www.example.com  # Custom domain
  tls:
    termination: edge
    certificate: <cert>
    key: <key>
    caCertificate: <ca>
```

## Route vs Ingress

| Feature | Route | Ingress |
|---------|-------|---------|
| **Origin** | OpenShift | Kubernetes |
| **TLS modes** | Edge, passthrough, re-encrypt | Edge only |
| **Wildcard** | Yes | Limited |
| **Load balancing** | Round-robin, least-conn | Depends on controller |

## References

- **OpenShift Routes**: https://docs.openshift.com/container-platform/latest/networking/routes/route-configuration.html
- **Service**: [../kubernetes/service.md](../kubernetes/service.md)
