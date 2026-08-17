# ClusterOperator

**Category**: OpenShift Platform API  
**API Group**: config.openshift.io/v1  
**Last Updated**: 2026-05-26
**Scope**: All form factors (see HCP note below)

## Overview

ClusterOperator represents the status of a platform component. Every core OpenShift component reports its health via a ClusterOperator resource.

**Purpose**: Centralized health monitoring and upgrade orchestration.

**⚠️ Form Factor Note**: In **Hypershift/HCP**, ClusterOperators exist in **both** clusters:
- **Guest cluster**: Platform components serving the hosted cluster (same as standalone)
- **Management cluster**: HyperShift platform components managing hosted control planes
- The CVO in the **guest** cluster watches ClusterOperators in the guest cluster

See [hypershift-control-plane-version-status.md](../../../enhancements/hypershift/hypershift-control-plane-version-status.md) for authoritative details on CVO placement in HCP.

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
  - type: Progressing
    status: "False"
    reason: AsExpected
    message: "Desired state reached"
    lastTransitionTime: "2026-04-28T10:00:00Z"
  - type: Degraded
    status: "False"
    reason: AsExpected
    message: "Component is healthy"
    lastTransitionTime: "2026-04-28T10:00:00Z"
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

## Optional Conditions

| Condition | Meaning | True When | False When |
|-----------|---------|-----------|------------|
| **Upgradeable** | Safe to upgrade | No blockers | Manual intervention needed |
| **EvaluationConditionsDetected** | Detects impact of invasive changes | Deprecated/risky config detected | No issues detected |

**Note**: `Upgradeable` defaults to allowing upgrades when missing, True, or Unknown — only `False` blocks minor upgrades. `EvaluationConditionsDetected` is set by individual operators on their own ClusterOperator resources to report the results of detection logic for invasive changes; the CVO monitors it generically for metrics but does not exclusively manage it.

See [status-conditions.md](../../platform/operator-patterns/status-conditions.md) for detailed semantics.

## Lifecycle

```
Component start:
  1. Create ClusterOperator (if not exists)
  2. Set all conditions (Available, Progressing, Degraded)

Normal operation:
  1. Update conditions when state changes
  2. Update lastTransitionTime only when condition status changes

Upgrade:
  1. CVO updates operator image
  2. Operator reconciles and detects version change
  3. Operator sets Progressing=True (if rollout needed)
  4. Operator rolls out new workload (if applicable)
  5. Operator sets Progressing=False, updates versions

Note: Some upgrades only require image updates without rollout (Progressing may stay False).
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

**Two-step process** — ClusterOperator has a status subresource (`+kubebuilder:subresource:status`), so the resource and its status must be updated via separate API calls:

1. Create/update the ClusterOperator resource via the main endpoint (`controllerutil.CreateOrUpdate` or `client.Create`)
2. Update status (conditions, versions, relatedObjects) via `client.Status().Update()` or `client.Status().Patch()`

**Key types**: `configv1.ClusterOperator`, `configv1.OperatorAvailable`, `configv1.ConditionTrue`, `configv1.OperandVersion`, `configv1.ObjectReference` (all in `github.com/openshift/api/config/v1`)

**Caution**: Status mutations inside a `CreateOrUpdate` mutate function are silently discarded by the API server. Always use `Status().Update()` for status changes.

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
cluster_operator_conditions{condition="Available"} == 0

# Alert on degraded component
cluster_operator_conditions{condition="Degraded"} == 1
```

**Note**: The metric uses numeric values (0 = False, 1 = True) and `condition` label (not `type`). See [CVO metrics documentation](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/metrics.md) for full metric definition.

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

2. **Use Upgradeable to block**: Set to False when upgrade would break
   ```yaml
   - type: Upgradeable
     status: "False"
     reason: UnsupportedConfiguration
     message: "Manual intervention required"
   ```

3. **Set RelatedObjects**: Help debugging
   ```yaml
   relatedObjects:
   - resource: namespaces
     name: my-operator-namespace
   ```

## Antipatterns

❌ **Missing conditions**: Not setting Available/Progressing/Degraded  
❌ **Vague messages**: "Error" instead of "Pod xyz crash looping: OOMKilled"  
❌ **Wrong Available semantics**: Setting False during normal upgrade  
❌ **Unnecessary updates**: Updating conditions when nothing has changed (wasteful reconciliation)

## References

- **API**: `oc explain clusteroperator`
- **Source**: [github.com/openshift/api/config/v1](https://github.com/openshift/api/blob/master/config/v1/types_cluster_operator.go)
- **Dev Guide**: [ClusterOperator Guidelines](https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/clusteroperator.md)
- **Pattern**: [status-conditions.md](../../platform/operator-patterns/status-conditions.md)
