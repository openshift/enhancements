# Cluster Operators

**Category**: OpenShift Platform  
**Last Updated**: 2026-04-08  

## Overview

ClusterOperators are OpenShift's extension of the Kubernetes operator pattern. They manage platform components and report status to the Cluster Version Operator (CVO).

## ClusterOperator vs Kubernetes Operator

| Aspect | Kubernetes Operator | ClusterOperator |
|--------|-------------------|----------------|
| **Scope** | Application-level | Platform-level (cluster infrastructure) |
| **Status Reporting** | Optional | Required (Available/Progressing/Degraded/Upgradeable) |
| **Lifecycle** | Independent | Coordinated by CVO |
| **Upgrades** | Independent | Orchestrated by CVO in specific order |
| **Health Monitoring** | Application-specific | Standardized conditions |

## ClusterOperator Resource

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterOperator
metadata:
  name: machine-config
spec: {}
status:
  conditions:
  - type: Available
    status: "True"
    reason: AsExpected
    message: "All pools are updated"
    lastTransitionTime: "2026-04-08T10:00:00Z"
  - type: Progressing
    status: "False"
    reason: AsExpected
    lastTransitionTime: "2026-04-08T10:00:00Z"
  - type: Degraded
    status: "False"
    reason: AsExpected
    lastTransitionTime: "2026-04-08T10:00:00Z"
  - type: Upgradeable
    status: "True"
    reason: AsExpected
    message: "Safe to upgrade"
    lastTransitionTime: "2026-04-08T10:00:00Z"
  versions:
  - name: operator
    version: 4.15.0
  relatedObjects:
  - group: machineconfiguration.openshift.io
    resource: machineconfigs
    name: ""
  - group: machineconfiguration.openshift.io
    resource: machineconfigpools
    name: ""
```

## Standard Conditions

All ClusterOperators must report four conditions:

### Available

```yaml
type: Available
status: "True"  # Operator is functional
reason: AsExpected
message: "All components running"
```

**True**: Operator is functional, workload is running.  
**False**: Operator cannot perform its primary function.

### Progressing

```yaml
type: Progressing
status: "True"  # Currently rolling out changes
reason: RollingOut
message: "Updating 3 of 10 nodes"
```

**True**: Operator is actively making changes (upgrade, config rollout).  
**False**: Operator is at desired state.

### Degraded

```yaml
type: Degraded
status: "True"  # Something is wrong
reason: NodeUpdateFailed
message: "5 nodes failed to apply configuration"
```

**True**: Operator encountered errors, reduced functionality.  
**False**: Operator is healthy.

**Important**: An operator can be Available=True and Degraded=True simultaneously (partial functionality).

### Upgradeable

```yaml
type: Upgradeable
status: "True"  # Safe to upgrade cluster
reason: AsExpected
message: "Safe to upgrade"
```

**True**: CVO can proceed with cluster upgrade.  
**False**: Operator blocks cluster upgrade (active migration, high churn, etc.).

## Implementing ClusterOperator Status

```go
import (
    configv1 "github.com/openshift/api/config/v1"
    "github.com/openshift/library-go/pkg/operator/v1helpers"
)

func (r *Reconciler) updateClusterOperatorStatus(ctx context.Context) error {
    co := &configv1.ClusterOperator{}
    if err := r.Get(ctx, types.NamespacedName{Name: "machine-config"}, co); err != nil {
        return err
    }
    
    // Update conditions
    v1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{
        Type:               configv1.OperatorAvailable,
        Status:             configv1.ConditionTrue,
        Reason:             "AsExpected",
        Message:            "All machine config pools are updated",
        LastTransitionTime: metav1.Now(),
    })
    
    v1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{
        Type:               configv1.OperatorProgressing,
        Status:             configv1.ConditionFalse,
        Reason:             "AsExpected",
        LastTransitionTime: metav1.Now(),
    })
    
    v1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{
        Type:               configv1.OperatorDegraded,
        Status:             configv1.ConditionFalse,
        Reason:             "AsExpected",
        LastTransitionTime: metav1.Now(),
    })
    
    v1helpers.SetStatusCondition(&co.Status.Conditions, configv1.ClusterOperatorStatusCondition{
        Type:               configv1.OperatorUpgradeable,
        Status:             configv1.ConditionTrue,
        Reason:             "AsExpected",
        Message:            "Safe to upgrade",
        LastTransitionTime: metav1.Now(),
    })
    
    // Update versions
    co.Status.Versions = []configv1.OperandVersion{
        {Name: "operator", Version: "4.15.0"},
    }
    
    // Update related objects
    co.Status.RelatedObjects = []configv1.ObjectReference{
        {
            Group:    "machineconfiguration.openshift.io",
            Resource: "machineconfigs",
        },
        {
            Group:    "machineconfiguration.openshift.io",
            Resource: "machineconfigpools",
        },
    }
    
    return r.Status().Update(ctx, co)
}
```

## RBAC for ClusterOperator

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: machine-config-operator-clusteroperator
rules:
# Read ClusterOperator
- apiGroups: ["config.openshift.io"]
  resources: ["clusteroperators"]
  verbs: ["get", "list", "watch"]
# Update ClusterOperator status
- apiGroups: ["config.openshift.io"]
  resources: ["clusteroperators/status"]
  verbs: ["update", "patch"]
```

## CVO Integration

The Cluster Version Operator (CVO) uses ClusterOperator status to:

1. **Monitor platform health**: Aggregate all operator statuses
2. **Coordinate upgrades**: Wait for Progressing=False before next operator
3. **Block unsafe upgrades**: Respect Upgradeable=False
4. **Report cluster status**: Surface issues to users via `oc get clusteroperators`

### CVO Upgrade Flow

```
CVO: Start upgrade to 4.15.0
  ↓
CVO: Update kube-apiserver
  ↓
kube-apiserver ClusterOperator: Progressing=True
  ↓
kube-apiserver ClusterOperator: Available=True, Progressing=False
  ↓
CVO: Proceed to next operator (machine-config)
  ↓
machine-config ClusterOperator: Check Upgradeable=True
  ↓
CVO: Update machine-config-operator
  ...
```

## Monitoring ClusterOperators

```bash
# List all ClusterOperators
oc get clusteroperators

# Expected output:
# NAME                 VERSION   AVAILABLE   PROGRESSING   DEGRADED   SINCE
# machine-config       4.15.0    True        False         False      10m
# network              4.15.0    True        False         False      10m

# Check specific operator
oc get clusteroperator machine-config -o yaml

# Watch for changes
oc get clusteroperators -w

# Check degraded operators
oc get clusteroperators | grep -v "True.*False.*False"
```

## Common ClusterOperators

| Name | Purpose | Critical |
|------|---------|----------|
| **kube-apiserver** | Kubernetes API server | ✅ Yes |
| **kube-controller-manager** | Core K8s controllers | ✅ Yes |
| **kube-scheduler** | Pod scheduling | ✅ Yes |
| **etcd** | Cluster state storage | ✅ Yes |
| **machine-config** | Node configuration | ✅ Yes |
| **network** | Cluster networking (SDN/OVN) | ✅ Yes |
| **ingress** | Cluster ingress/routing | ✅ Yes |
| **authentication** | Cluster authentication | ✅ Yes |
| **console** | Web console | ❌ No |
| **monitoring** | Prometheus/Grafana | ❌ No |
| **logging** | Log aggregation | ❌ No |

## Best Practices

1. **Always set all four conditions**: Available, Progressing, Degraded, Upgradeable
2. **Update regularly**: Refresh status every reconciliation loop
3. **Meaningful messages**: Explain why operator is degraded/not upgradeable
4. **Set versions**: Report operator version in `.status.versions`
5. **List related objects**: Help users discover managed resources
6. **Accurate transitions**: Only change condition when state actually changes
7. **Block upgrades carefully**: Upgradeable=False should be temporary

## Debugging Issues

```bash
# Check condition history
oc describe clusteroperator machine-config

# View related resources
oc get clusteroperator machine-config -o jsonpath='{.status.relatedObjects[*]}'

# Check operator logs
oc logs -n openshift-machine-config-operator deployment/machine-config-operator

# Compare versions
oc get clusteroperators -o custom-columns=NAME:.metadata.name,VERSION:.status.versions[0].version
```

## Examples in Components

| Component | ClusterOperator Name | Key Status Logic |
|-----------|---------------------|------------------|
| machine-config-operator | machine-config | Aggregates MachineConfigPool status |
| cluster-network-operator | network | Reports CNI health (SDN/OVN) |
| machine-api-operator | machine-api | Reports Machine provisioning status |
| cluster-version-operator | cluster-version | Self-reports CVO status |
| ingress-operator | ingress | Reports IngressController health |

## References

- **ClusterOperator API**: [clusteroperator.md](../../domain/openshift/clusteroperator.md)
- **Status Conditions**: [status-conditions.md](../operator-patterns/status-conditions.md)
- **CVO**: [operator-lifecycle.md](./operator-lifecycle.md)
- **Upgrade Strategies**: [upgrade-strategies.md](../operator-patterns/upgrade-strategies.md)
