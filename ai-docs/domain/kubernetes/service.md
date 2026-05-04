# Service

**Category**: Kubernetes Core API  
**API Group**: core/v1  
**Last Updated**: 2026-04-29  

## Overview

Service provides stable networking for pods. It acts as a load balancer and service discovery mechanism.

**Key Principle**: Pods are ephemeral, Services are stable.

## Key Fields

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-service
  namespace: default
spec:
  type: ClusterIP  # ClusterIP, NodePort, LoadBalancer
  selector:
    app: my-app
  ports:
  - name: http
    protocol: TCP
    port: 80        # Service port
    targetPort: 8080 # Pod port
  sessionAffinity: None
status:
  loadBalancer: {}
```

## Service Types

| Type | Accessibility | Use Case |
|------|--------------|----------|
| **ClusterIP** | Internal only | Inter-service communication |
| **NodePort** | External (node IP:port) | Development, testing |
| **LoadBalancer** | External (cloud LB) | Production external access |
| **ExternalName** | DNS CNAME | External service alias |

## ClusterIP (Default)

**Purpose**: Internal service discovery

```yaml
apiVersion: v1
kind: Service
metadata:
  name: backend
spec:
  type: ClusterIP
  selector:
    app: backend
  ports:
  - port: 80
    targetPort: 8080
```

**Access**: `http://backend.default.svc.cluster.local:80`

## NodePort

**Purpose**: Expose service on each node's IP

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-nodeport
spec:
  type: NodePort
  selector:
    app: my-app
  ports:
  - port: 80
    targetPort: 8080
    nodePort: 30080  # Optional (auto-assigned if omitted)
```

**Access**: `http://<node-ip>:30080`

## LoadBalancer

**Purpose**: Cloud provider load balancer

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-lb
spec:
  type: LoadBalancer
  selector:
    app: my-app
  ports:
  - port: 80
    targetPort: 8080
```

**Status**:
```yaml
status:
  loadBalancer:
    ingress:
    - ip: 203.0.113.5  # External IP
```

## Selectors and Endpoints

**Service selects pods by labels**:

```yaml
# Service
spec:
  selector:
    app: my-app
    version: v1

# Matching Pod
metadata:
  labels:
    app: my-app
    version: v1
```

**Endpoints (auto-created)**:
```bash
$ oc get endpoints my-service
NAME         ENDPOINTS                     AGE
my-service   10.128.0.5:8080,10.128.0.6:8080   5m
```

## Headless Service

**Purpose**: Direct pod-to-pod DNS (no load balancing)

```yaml
apiVersion: v1
kind: Service
metadata:
  name: my-headless
spec:
  clusterIP: None  # Headless
  selector:
    app: my-app
  ports:
  - port: 80
    targetPort: 8080
```

**DNS**: Returns pod IPs, not service IP
```bash
nslookup my-headless.default.svc.cluster.local
# Returns: 10.128.0.5, 10.128.0.6 (pod IPs)
```

## Session Affinity

```yaml
spec:
  sessionAffinity: ClientIP
  sessionAffinityConfig:
    clientIP:
      timeoutSeconds: 3600
```

**Effect**: Same client IP always routed to same pod

## Multiple Ports

```yaml
spec:
  ports:
  - name: http
    port: 80
    targetPort: 8080
  - name: https
    port: 443
    targetPort: 8443
```

## Named Ports

```yaml
# Pod
spec:
  containers:
  - name: app
    ports:
    - name: http
      containerPort: 8080

# Service (references named port)
spec:
  ports:
  - port: 80
    targetPort: http  # Port name, not number
```

## ExternalName

**Purpose**: Alias for external service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: external-db
spec:
  type: ExternalName
  externalName: db.example.com
```

**Access**: `external-db.default.svc.cluster.local` → `db.example.com`

## Service Discovery

### DNS

```bash
# Same namespace
curl http://my-service

# Different namespace
curl http://my-service.other-namespace

# Fully qualified
curl http://my-service.other-namespace.svc.cluster.local
```

### Environment Variables

```bash
# Kubernetes injects for services in same namespace
MY_SERVICE_SERVICE_HOST=10.96.0.5
MY_SERVICE_SERVICE_PORT=80
```

## Common Patterns

### Internal API Service

```yaml
apiVersion: v1
kind: Service
metadata:
  name: api
  namespace: my-app
spec:
  type: ClusterIP
  selector:
    component: api
  ports:
  - port: 8080
```

### External Load Balancer

```yaml
apiVersion: v1
kind: Service
metadata:
  name: frontend
  annotations:
    service.beta.kubernetes.io/aws-load-balancer-type: nlb
spec:
  type: LoadBalancer
  selector:
    component: frontend
  ports:
  - port: 80
    targetPort: 8080
```

### StatefulSet Headless Service

```yaml
# Headless service for StatefulSet
apiVersion: v1
kind: Service
metadata:
  name: mysql
spec:
  clusterIP: None
  selector:
    app: mysql
  ports:
  - port: 3306
```

**DNS**: Each pod gets stable DNS: `mysql-0.mysql.default.svc.cluster.local`

## Debugging

```bash
# View service
oc get svc my-service

# Check endpoints
oc get endpoints my-service

# Describe (shows events)
oc describe svc my-service

# Test connectivity
oc run test --rm -it --image=busybox -- wget -O- http://my-service

# Check DNS
oc run test --rm -it --image=busybox -- nslookup my-service
```

## Common Issues

### No Endpoints

```bash
$ oc get endpoints my-service
NAME         ENDPOINTS   AGE
my-service   <none>      5m
```

**Causes**:
- Selector doesn't match any pods
- Pods not ready (readiness probe failing)
- Pods in different namespace

**Fix**: Check selector and pod labels match

### Service Not Accessible

**Checks**:
1. Endpoints exist? `oc get endpoints`
2. Pods ready? `oc get pods`
3. Network policy blocking? `oc get networkpolicy`
4. Firewall rules? (cloud provider)

## OpenShift-Specific

### Routes (External Access)

```yaml
# OpenShift Route (Layer 7 load balancing)
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: my-route
spec:
  to:
    kind: Service
    name: my-service
  port:
    targetPort: http
  tls:
    termination: edge
```

**Access**: `https://my-route-default.apps.cluster.example.com`

## Best Practices

1. **Use ClusterIP for internal**: Don't expose unnecessarily
2. **Name ports**: Enables changing port numbers without updating service
3. **Use Routes for external**: OpenShift Routes > NodePort for production
4. **Set readiness probe**: Unhealthy pods excluded from endpoints
5. **Use headless for StatefulSets**: Stable DNS per pod

## Antipatterns

❌ **NodePort for production**: Use LoadBalancer or Route instead  
❌ **Selector matching all pods**: Too broad, wrong pods selected  
❌ **No port names**: Hard to understand multi-port services  
❌ **Hardcoded IPs**: Use DNS names instead  
❌ **LoadBalancer for internal**: Expensive, use ClusterIP

## References

- **API**: `oc explain service`
- **Docs**: [Kubernetes Services](https://kubernetes.io/docs/concepts/services-networking/service/)
- **OpenShift**: [Routes](https://docs.openshift.com/container-platform/latest/networking/routes/route-configuration.html)
- **Related**: [pod.md](./pod.md)
