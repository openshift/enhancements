# Pod

**Type**: Kubernetes Core Concept  
**Last Updated**: 2026-04-08  

## Overview

Pod is the smallest deployable unit in Kubernetes - a group of one or more containers with shared storage and network.

## Key Concepts

- **Containers**: One or more containers running together
- **Shared Network**: All containers share IP address and ports (localhost communication)
- **Shared Storage**: Volumes mounted into containers
- **Ephemeral**: Pods are mortal, don't retain state

## Lifecycle

| Phase | Meaning |
|-------|---------|
| Pending | Waiting for scheduling or image pull |
| Running | At least one container running |
| Succeeded | All containers completed successfully |
| Failed | At least one container failed |
| Unknown | Cannot determine state |

## Common Patterns

### Single Container

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: simple-pod
spec:
  containers:
  - name: app
    image: myapp:v1
    ports:
    - containerPort: 8080
```

### Init Containers

```yaml
spec:
  initContainers:
  - name: init
    image: busybox
    command: ['sh', '-c', 'setup-script']
  containers:
  - name: app
    image: myapp
```

**Use case**: Setup before main container starts

### Sidecar Pattern

```yaml
spec:
  containers:
  - name: app
    image: myapp
  - name: log-forwarder
    image: fluentd
```

**Use case**: Additional functionality alongside main container

### Resource Limits

```yaml
spec:
  containers:
  - name: app
    resources:
      requests:
        memory: "64Mi"
        cpu: "250m"
      limits:
        memory: "128Mi"
        cpu: "500m"
```

## Health Checks

```yaml
spec:
  containers:
  - name: app
    livenessProbe:
      httpGet:
        path: /healthz
        port: 8080
      initialDelaySeconds: 15
      periodSeconds: 20
    readinessProbe:
      httpGet:
        path: /ready
        port: 8080
      initialDelaySeconds: 5
      periodSeconds: 10
```

## Related Concepts

- **Deployment**: Manages Pods via ReplicaSets
- **Service**: Exposes Pods ([service.md](./service.md))
- **Node**: Where Pods run ([node.md](./node.md))

## References

- **Kubernetes Pods**: https://kubernetes.io/docs/concepts/workloads/pods/
