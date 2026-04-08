# Leader Election Pattern

**Category**: Platform Pattern  
**Applies To**: All multi-replica operators  
**Last Updated**: 2026-04-08  

## Overview

Leader election ensures only one replica of an operator is active at a time, preventing concurrent reconciliation and race conditions.

## Why Leader Election

**Problem**: Multiple operator replicas reconciling simultaneously causes:
- Race conditions on resource updates
- Duplicate work (e.g., multiple node reboots)
- Conflicting decisions
- Resource thrashing

**Solution**: One leader performs reconciliation; others stand by ready to take over.

## Implementation

### controller-runtime Integration

```go
import (
    ctrl "sigs.k8s.io/controller-runtime"
    "sigs.k8s.io/controller-runtime/pkg/manager"
)

func main() {
    mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
        LeaderElection:          true,
        LeaderElectionID:        "my-operator.openshift.io",
        LeaderElectionNamespace: "openshift-my-operator",
    })
    if err != nil {
        log.Fatal(err)
    }
    
    // Setup controllers
    if err := (&MyReconciler{}).SetupWithManager(mgr); err != nil {
        log.Fatal(err)
    }
    
    // Start manager - blocks until leader
    if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
        log.Fatal(err)
    }
}
```

### library-go Pattern

```go
import (
    "github.com/openshift/library-go/pkg/operator/leaderelection"
)

func main() {
    leaderElectionConfig := leaderelection.LeaderElectionDefaulting(
        configv1.LeaderElection{
            Disable: false,
        },
        "openshift-my-operator",
        "my-operator",
    )
    
    leaderelection.RunOrDie(ctx, leaderElectionConfig, func(ctx context.Context) {
        // This code runs only when elected leader
        startOperator(ctx)
    })
}
```

## How It Works

```
Pod 1 (Leader)           ConfigMap Lock           Pod 2 (Standby)
     |                         |                         |
     |---> Acquire Lock ------>|                         |
     |         (wins)           |<---- Try Lock ---------|
     |                          |      (fails)           |
     |---> Renew Lock --------->|                         |
     |    (every 10s)           |                         |
     |                          |                         |
     X (crashes)                |                         |
                                |<---- Try Lock ---------|
                                |      (wins!)           |
                                |<----- Renew Lock ------|
```

## Configuration

### Lease-based (Recommended)

```yaml
# Creates Lease object for coordination
apiVersion: coordination.k8s.io/v1
kind: Lease
metadata:
  name: my-operator
  namespace: openshift-my-operator
spec:
  holderIdentity: "my-operator-7d5f8b9c-abc123"
  leaseDurationSeconds: 15
  acquireTime: "2026-04-08T10:00:00Z"
  renewTime: "2026-04-08T10:00:10Z"
```

### ConfigMap-based (Legacy)

```yaml
# Older approach using ConfigMap annotations
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-operator-lock
  namespace: openshift-my-operator
  annotations:
    control-plane.alpha.kubernetes.io/leader: '{"holderIdentity":"pod-1",...}'
```

## Timing Parameters

| Parameter | Default | Purpose |
|-----------|---------|---------|
| LeaseDuration | 15s | How long leader holds lease |
| RenewDeadline | 10s | Leader must renew before this |
| RetryPeriod | 2s | How often non-leaders retry |

**Important**: `RenewDeadline < LeaseDuration`

## Leader Election ID

Use unique, stable identifier:

```go
// Good: domain-based
LeaderElectionID: "machine-config-operator.openshift.io"

// Bad: generic
LeaderElectionID: "lock"
```

## Best Practices

1. **Always enable for multi-replica**: Prevent split-brain scenarios
2. **Use Lease objects**: More efficient than ConfigMap locks
3. **Set appropriate timeouts**: Balance failover speed vs stability
4. **Namespace-scoped**: Use operator's namespace for lock
5. **Monitor leadership**: Expose metrics for current leader
6. **Graceful shutdown**: Release lock on SIGTERM

## HA Deployment

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: my-operator
  namespace: openshift-my-operator
spec:
  replicas: 3  # HA setup
  selector:
    matchLabels:
      app: my-operator
  template:
    metadata:
      labels:
        app: my-operator
    spec:
      containers:
      - name: operator
        image: my-operator:latest
        # Leader election handled by controller-runtime
      serviceAccountName: my-operator
      # Anti-affinity recommended for true HA
      affinity:
        podAntiAffinity:
          preferredDuringSchedulingIgnoredDuringExecution:
          - weight: 100
            podAffinityTerm:
              labelSelector:
                matchLabels:
                  app: my-operator
              topologyKey: kubernetes.io/hostname
```

## RBAC Requirements

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: my-operator-leader-election
  namespace: openshift-my-operator
rules:
- apiGroups: ["coordination.k8s.io"]
  resources: ["leases"]
  verbs: ["get", "create", "update"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: my-operator-leader-election
  namespace: openshift-my-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: my-operator-leader-election
subjects:
- kind: ServiceAccount
  name: my-operator
  namespace: openshift-my-operator
```

## Debugging

```bash
# Check current leader
oc get lease -n openshift-my-operator my-operator -o yaml

# Check which pod is leader
oc get lease -n openshift-my-operator my-operator \
  -o jsonpath='{.spec.holderIdentity}'

# Watch leader changes
oc get lease -n openshift-my-operator my-operator -w

# Check operator logs for election events
oc logs -n openshift-my-operator deployment/my-operator | grep -i leader
```

## Metrics

Expose leader status:

```go
import "github.com/prometheus/client_golang/prometheus"

var isLeaderGauge = prometheus.NewGauge(prometheus.GaugeOpts{
    Name: "operator_is_leader",
    Help: "1 if this instance is the leader, 0 otherwise",
})

func init() {
    prometheus.MustRegister(isLeaderGauge)
}

// Update when leadership changes
func onElected() {
    isLeaderGauge.Set(1)
}

func onLost() {
    isLeaderGauge.Set(0)
}
```

## Examples in Components

| Component | Replicas | Lease Name | Notes |
|-----------|----------|------------|-------|
| machine-config-operator | 1 | N/A | Single replica, no election needed |
| cluster-network-operator | 1 | N/A | Single replica |
| machine-api-operator | 3 | machine-api-controllers | HA deployment with leader election |
| cluster-autoscaler | 2 | cluster-autoscaler | HA with leader election |

## Common Issues

1. **Lock contention**: Multiple operators fighting for lock (check RBAC)
2. **Slow failover**: Increase retry frequency for faster takeover
3. **Lease expiry**: Leader taking too long to renew (tune RenewDeadline)
4. **Split brain**: Two leaders (misconfigured election ID)

## References

- **controller-runtime**: https://github.com/kubernetes-sigs/controller-runtime
- **library-go**: https://github.com/openshift/library-go/pkg/operator/leaderelection
- **K8s Lease**: https://kubernetes.io/docs/reference/kubernetes-api/cluster-resources/lease-v1/
