# Node

**Type**: Kubernetes Core Concept  
**Last Updated**: 2026-04-08  

## Overview

Node is a worker machine (VM or physical) in Kubernetes that runs containerized workloads.

## Components

| Component | Purpose |
|-----------|---------|
| **kubelet** | Runs containers, reports to control plane |
| **kube-proxy** | Network proxy for Services |
| **Container runtime** | Runs containers (CRI-O, containerd) |

## Node Conditions

```yaml
status:
  conditions:
  - type: Ready
    status: "True"  # Node is healthy
  - type: MemoryPressure
    status: "False"  # Sufficient memory
  - type: DiskPressure
    status: "False"  # Sufficient disk
  - type: PIDPressure
    status: "False"  # Sufficient PIDs
  - type: NetworkUnavailable
    status: "False"  # Network configured
```

## Taints and Tolerations

### Taints (on Nodes)

```yaml
# Prevent pods from scheduling
apiVersion: v1
kind: Node
metadata:
  name: node1
spec:
  taints:
  - key: node-role.kubernetes.io/master
    effect: NoSchedule
  - key: dedicated
    value: special-workload
    effect: NoExecute
```

**Effects**:
- `NoSchedule`: Don't schedule new pods
- `PreferNoSchedule`: Avoid scheduling if possible
- `NoExecute`: Evict existing pods

### Tolerations (on Pods)

```yaml
spec:
  tolerations:
  - key: dedicated
    operator: Equal
    value: special-workload
    effect: NoExecute
```

## Node Selectors

```yaml
# Pod scheduled only on nodes with label
spec:
  nodeSelector:
    disk: ssd
    region: us-west
```

## Affinity and Anti-Affinity

```yaml
spec:
  affinity:
    nodeAffinity:
      requiredDuringSchedulingIgnoredDuringExecution:
        nodeSelectorTerms:
        - matchExpressions:
          - key: topology.kubernetes.io/zone
            operator: In
            values:
            - us-west-1a
            - us-west-1b
```

## References

- **Kubernetes Nodes**: https://kubernetes.io/docs/concepts/architecture/nodes/
- **Taints and Tolerations**: https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/
