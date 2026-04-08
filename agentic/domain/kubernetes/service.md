# Service

**Type**: Kubernetes Core Concept  
**Last Updated**: 2026-04-08  

## Overview

Service provides stable network endpoint for accessing Pods with load balancing and service discovery.

## Key Concepts

- **Stable IP**: ClusterIP that doesn't change
- **Load Balancing**: Distributes traffic across Pod replicas
- **Service Discovery**: DNS name (my-svc.my-namespace.svc.cluster.local)
- **Port Mapping**: Map service port to container port

## Service Types

| Type | Purpose | Access |
|------|---------|--------|
| **ClusterIP** | Internal cluster access | Cluster-internal only |
| **NodePort** | External access via node IP | <NodeIP>:<NodePort> |
| **LoadBalancer** | Cloud load balancer | External IP from cloud |
| **ExternalName** | DNS CNAME | External service alias |

## Examples

### ClusterIP (Default)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
spec:
  type: ClusterIP
  selector:
    app: myapp
  ports:
  - port: 80        # Service port
    targetPort: 8080 # Container port
```

Access: `http://my-service.my-namespace.svc.cluster.local`

### NodePort

```yaml
spec:
  type: NodePort
  ports:
  - port: 80
    targetPort: 8080
    nodePort: 30080  # 30000-32767
```

Access: `http://<node-ip>:30080`

### Headless Service

```yaml
spec:
  clusterIP: None  # No load balancing
  selector:
    app: myapp
```

**Use case**: StatefulSets, direct Pod access (DNS returns Pod IPs)

## Service Discovery

### DNS

```bash
# Same namespace
curl http://my-service

# Cross-namespace
curl http://my-service.other-namespace

# FQDN
curl http://my-service.my-namespace.svc.cluster.local
```

### Environment Variables

```bash
MY_SERVICE_SERVICE_HOST=10.96.0.10
MY_SERVICE_SERVICE_PORT=80
```

## Session Affinity

```yaml
spec:
  sessionAffinity: ClientIP
  sessionAffinityConfig:
    clientIP:
      timeoutSeconds: 3600
```

**Use case**: Sticky sessions (same client → same Pod)

## References

- **Kubernetes Services**: https://kubernetes.io/docs/concepts/services-networking/service/
- **OpenShift Routes**: [../openshift/route.md](../openshift/route.md)
