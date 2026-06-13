# Pod

**Category**: Kubernetes Core API  
**API Group**: core/v1  
**Last Updated**: 2026-04-29  

## Overview

Pod is the smallest deployable unit in Kubernetes. It represents one or more containers running together on a node.

**Key Principle**: Pods are ephemeral - they can be deleted and recreated at any time.

## Key Fields

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: my-pod
  namespace: default
  labels:
    app: my-app
spec:
  containers:
  - name: app
    image: registry.io/my-app:v1.0
    ports:
    - containerPort: 8080
    env:
    - name: ENV_VAR
      value: "value"
    resources:
      requests:
        memory: "64Mi"
        cpu: "250m"
      limits:
        memory: "128Mi"
        cpu: "500m"
    volumeMounts:
    - name: data
      mountPath: /data
  volumes:
  - name: data
    emptyDir: {}
  restartPolicy: Always
  serviceAccountName: my-sa
status:
  phase: Running
  conditions:
  - type: Ready
    status: "True"
  podIP: 10.128.0.5
```

## Pod Lifecycle

| Phase | Meaning | Next State |
|-------|---------|------------|
| **Pending** | Waiting for scheduling | Running |
| **Running** | At least one container running | Succeeded / Failed |
| **Succeeded** | All containers exited successfully | Terminal |
| **Failed** | At least one container exited with error | Terminal |
| **Unknown** | Cannot determine state | Any |

## Container States

| State | Meaning | Reason |
|-------|---------|--------|
| **Waiting** | Not yet started | ContainerCreating, ImagePullBackOff |
| **Running** | Executing | - |
| **Terminated** | Finished or failed | Completed, Error, OOMKilled |

## Key Concepts

### Multi-Container Patterns

**Sidecar**: Helper container (logging, monitoring)
```yaml
containers:
- name: app
  image: my-app:v1
- name: log-collector
  image: log-collector:v1  # Sidecar
```

**Init Containers**: Run before main containers
```yaml
initContainers:
- name: setup
  image: setup:v1
  command: ["sh", "-c", "echo Setting up > /data/setup.txt"]
  volumeMounts:
  - name: data
    mountPath: /data
containers:
- name: app
  image: my-app:v1
  volumeMounts:
  - name: data
    mountPath: /data
```

## Resource Requests and Limits

```yaml
resources:
  requests:      # Scheduler uses for placement
    memory: "64Mi"
    cpu: "250m"
  limits:        # Hard limits enforced by kubelet
    memory: "128Mi"
    cpu: "500m"
```

**Effects**:
- **CPU limit exceeded**: Throttling
- **Memory limit exceeded**: OOMKilled

## Probes

### Liveness Probe

**Purpose**: Restart container if unhealthy

```yaml
livenessProbe:
  httpGet:
    path: /healthz
    port: 8080
  initialDelaySeconds: 30
  periodSeconds: 10
  failureThreshold: 3
```

### Readiness Probe

**Purpose**: Remove from service if not ready

```yaml
readinessProbe:
  httpGet:
    path: /ready
    port: 8080
  initialDelaySeconds: 5
  periodSeconds: 5
```

### Startup Probe

**Purpose**: Delay liveness check until app started

```yaml
startupProbe:
  httpGet:
    path: /healthz
    port: 8080
  failureThreshold: 30  # 30 * 10s = 5 minutes max startup
  periodSeconds: 10
```

## Volumes

```yaml
volumes:
# Empty directory (deleted with pod)
- name: cache
  emptyDir: {}

# ConfigMap
- name: config
  configMap:
    name: my-config

# Secret
- name: certs
  secret:
    secretName: my-certs

# PersistentVolumeClaim
- name: data
  persistentVolumeClaim:
    claimName: my-pvc
```

## Security Context

```yaml
# Pod-level
securityContext:
  runAsNonRoot: true
  seccompProfile:
    type: RuntimeDefault

# Container-level
containers:
- name: app
  securityContext:
    allowPrivilegeEscalation: false
    capabilities:
      drop:
      - ALL
    runAsUser: 1000
```

## Common Patterns

### Job Pod

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: job-pod
spec:
  restartPolicy: Never  # Don't restart on completion
  containers:
  - name: job
    image: job:v1
    command: ["./run-job.sh"]
```

### DaemonSet Pod

```yaml
# One pod per node
spec:
  tolerations:
  - key: node-role.kubernetes.io/master
    effect: NoSchedule
  hostNetwork: true  # Common for node-level ops
```

## Debugging

```bash
# View pod
oc get pod my-pod -o yaml

# Check status
oc describe pod my-pod

# View logs
oc logs my-pod

# Previous logs (if restarted)
oc logs my-pod --previous

# Multi-container pod
oc logs my-pod -c container-name

# Execute command
oc exec -it my-pod -- /bin/sh

# Debug with ephemeral container (K8s 1.25+)
oc debug pod/my-pod --image=busybox
```

## Common Issues

### ImagePullBackOff

```yaml
status:
  containerStatuses:
  - state:
      waiting:
        reason: ImagePullBackOff
        message: "Failed to pull image: unauthorized"
```

**Causes**: Wrong image, auth issues, network issues

### CrashLoopBackOff

```yaml
status:
  containerStatuses:
  - restartCount: 5
    state:
      waiting:
        reason: CrashLoopBackOff
```

**Causes**: App crashes on startup, missing config

### OOMKilled

```yaml
status:
  containerStatuses:
  - lastState:
      terminated:
        reason: OOMKilled
```

**Fix**: Increase memory limit

## Pod Conditions

```yaml
status:
  conditions:
  - type: PodScheduled
    status: "True"
  - type: Initialized
    status: "True"
  - type: ContainersReady
    status: "True"
  - type: Ready
    status: "True"
```

## Best Practices

1. **Always set resource requests**: Enables proper scheduling
2. **Use readiness probe**: Prevents routing to unhealthy pods
3. **Use liveness probe**: Automatic recovery from deadlock
4. **Non-root user**: Security best practice
5. **Immutable image tags**: Use SHA256 or semantic versions, not `latest`

## Antipatterns

❌ **No resource limits**: Can consume all node resources  
❌ **No health probes**: Broken pods stay in service  
❌ **Running as root**: Security risk  
❌ **Host network for app pods**: Breaks network isolation  
❌ **Local storage for stateful data**: Lost on pod deletion

## References

- **API**: `oc explain pod`
- **Docs**: [Kubernetes Pods](https://kubernetes.io/docs/concepts/workloads/pods/)
- **Related**: [service.md](./service.md)
