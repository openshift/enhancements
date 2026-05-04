# ClusterOperator

**Category**: OpenShift Platform API  
**API Group**: config.openshift.io/v1  
**Last Updated**: 2026-04-28  

## Overview

ClusterOperator represents the status of a platform component. Every core OpenShift component reports its health via a ClusterOperator resource.

**Purpose**: Centralized health monitoring and upgrade orchestration.

## Key Fields

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterOperator
metadata:
  name: kube-apiserver
spec: {}  # ClusterOperator has no spec
status:
  conditions:
  - type: Available
    status: "True"
    reason: AsExpected
    message: "All 3 pods are ready"
    lastTransitionTime: "2026-04-28T10:00:00Z"
    lastHeartbeatTime: "2026-04-28T10:05:00Z"
  - type: Progressing
    status: "False"
    reason: AsExpected
    message: "Desired state reached"
  - type: Degraded
    status: "False"
    reason: AsExpected
    message: "Component is healthy"
  - type: Upgradeable
    status: "True"
    reason: AsExpected
    message: "Ready for upgrade"
  versions:
  - name: operator
    version: 4.16.0
  - name: kube-apiserver
    version: 1.29.5
  relatedObjects:
  - group: ""
    resource: namespaces
    name: openshift-kube-apiserver
  - group: apps
    resource: deployments
    namespace: openshift-kube-apiserver
    name: kube-apiserver
```

## Key Concepts

- **Conditions**: Available, Progressing, Degraded, Upgradeable
- **Versions**: Operator version and operand versions
- **RelatedObjects**: Resources managed by this operator
- **Read-Only**: ClusterOperator has no spec (status only)

## Required Conditions

| Condition | Meaning | True When | False When |
|-----------|---------|-----------|------------|
| **Available** | Component functional | Pods ready, API serving | Pods not ready, API down |
| **Progressing** | Reconciliation in progress | Upgrade/rollout active | Desired state reached |
| **Degraded** | Impaired functionality | Partial failure | Fully healthy |
| **Upgradeable** | Safe to upgrade | No blockers | Manual intervention needed |

See [status-conditions.md](../../platform/operator-patterns/status-conditions.md) for detailed semantics.

## Lifecycle

```
Component start:
  1. Create ClusterOperator (if not exists)
  2. Set all conditions (Available, Progressing, Degraded)
  3. Update heartbeat every ~60s

Normal operation:
  1. Update conditions when state changes
  2. Update heartbeat regularly

Upgrade:
  1. CVO updates operator image
  2. Operator sets Progressing=True
  3. Operator rolls out new workload
  4. Operator sets Progressing=False, updates versions
```

## Monitoring

```bash
# View all ClusterOperators
oc get clusteroperators

# View specific operator
oc get clusteroperator kube-apiserver -o yaml

# Check degraded operators
oc get co -o json | jq '.items[] | select(.status.conditions[] | select(.type=="Degraded" and .status=="True")) | .metadata.name'

# Check upgrade progress
oc get co -o json | jq '.items[] | select(.status.conditions[] | select(.type=="Progressing" and .status=="True")) | .metadata.name'
```

## Creating ClusterOperator

```go
import (
    configv1 "github.com/openshift/api/config/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func ensureClusterOperator(ctx context.Context, client client.Client, name string) error {
    co := &configv1.ClusterOperator{
        ObjectMeta: metav1.ObjectMeta{
            Name: name,
        },
    }
    
    _, err := controllerutil.CreateOrUpdate(ctx, client, co, func() error {
        // Set conditions
        setCondition(&co.Status.Conditions,
            configv1.OperatorAvailable,
            configv1.ConditionTrue,
            "AsExpected",
            "Component is available")
        
        // Set versions
        co.Status.Versions = []configv1.OperandVersion{
            {Name: "operator", Version: os.Getenv("RELEASE_VERSION")},
        }
        
        // Set related objects
        co.Status.RelatedObjects = []configv1.ObjectReference{
            {
                Group:    "",
                Resource: "namespaces",
                Name:     "my-operator-namespace",
            },
        }
        
        return nil
    })
    
    return err
}
```

## Versions

```yaml
status:
  versions:
  - name: operator
    version: 4.16.0
  - name: etcd
    version: 3.5.12
```

**Operator version**: Version of the operator itself  
**Operand versions**: Versions of managed components

## RelatedObjects

```yaml
status:
  relatedObjects:
  - group: ""
    resource: namespaces
    name: openshift-kube-apiserver
  - group: apps
    resource: deployments
    namespace: openshift-kube-apiserver
    name: kube-apiserver
```

**Purpose**: Help must-gather and debugging tools find related resources.

## Alerts

```promql
# Alert on unavailable component
ALERT ClusterOperatorUnavailable
  IF cluster_operator_conditions{type="Available", status="False"} == 1
  FOR 15m
  
# Alert on degraded component
ALERT ClusterOperatorDegraded
  IF cluster_operator_conditions{type="Degraded", status="True"} == 1
  FOR 15m

# Alert on stale heartbeat
ALERT ClusterOperatorStale
  IF (time() - cluster_operator_conditions_last_heartbeat) > 300
  FOR 5m
```

## Common ClusterOperators

| Name | Component | Purpose |
|------|-----------|---------|
| kube-apiserver | Kubernetes API | API server availability |
| etcd | etcd cluster | Data store health |
| kube-controller-manager | KCM | Controller health |
| machine-api | Machine API | Node provisioning |
| cluster-network-operator | Networking | SDN/OVN health |
| cluster-version | CVO | Upgrade orchestration |

## Best Practices

1. **Always set all conditions**: Available, Progressing, Degraded must always be present

2. **Update heartbeat regularly**: Every 60s even if nothing changed
   ```go
   setCondition(&co.Status.Conditions, type, status, reason, message)
   // Updates LastHeartbeatTime automatically
   ```

3. **Use Upgradeable to block**: Set to False when upgrade would break
   ```yaml
   - type: Upgradeable
     status: "False"
     reason: UnsupportedConfiguration
     message: "Manual intervention required"
   ```

4. **Set RelatedObjects**: Help debugging
   ```yaml
   relatedObjects:
   - resource: namespaces
     name: my-operator-namespace
   ```

## Antipatterns

❌ **Missing conditions**: Not setting Available/Progressing/Degraded  
❌ **Stale heartbeat**: Not updating LastHeartbeatTime  
❌ **Vague messages**: "Error" instead of "Pod xyz crash looping: OOMKilled"  
❌ **Wrong Available semantics**: Setting False during normal upgrade

## References

- **API**: `oc explain clusteroperator`
- **Source**: [github.com/openshift/api/config/v1](https://github.com/openshift/api/blob/master/config/v1/types_cluster_operator.go)
- **Dev Guide**: [ClusterOperator Guidelines](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/clusteroperator.md)
- **Pattern**: [status-conditions.md](../../platform/operator-patterns/status-conditions.md)
