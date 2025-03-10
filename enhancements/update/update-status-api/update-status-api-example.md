# Update Status API Example

## Example Data

- Example `UpdateStatus`: [4.15.0-ec2-early-us.yaml](4.15.0-ec2-early-us.yaml)
- Example `oc adm upgrade status` output: [4.15.0-ec2-early.output](4.15.0-ec2-early.output)
- Example `oc adm upgrade status --detailed=all` output: [4.15.0-ec2-early.detailed-output](4.15.0-ec2-early.detailed-output)

## High-level Structure

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
metadata:
  creationTimestamp: null
  name: cluster
spec: {}
status:
  controlPlane:
    conditions: [USC-owned summarization of the control plane update state]
    informers:
    - insights: [CPI informer-owned list of active insights about control plane]
      name: cpi
    # Potentially more informers with their insights about control plane
    poolResource:
      group: machineconfiguration.openshift.io
      name: master
      resource: machineconfigpools
    resource:
      group: config.openshift.io
      name: version
      resource: clusterversions
  workerPools:
  - conditions: [USC-owned summarization of this worker pool update state]
    informers:
    - insights: [nodes-informer informer-owned list of active insights about this worker pool]
      name: nodes-informer
    # Potentially more informers with their insights about this worker pool
    name: worker
    resource:
      group: machineconfiguration.openshift.io
      name: worker
      resource: machineconfigpools
  # Potentially more worker pools -> informers -> insights
```

The `UpdateStatus` is essentially a tree container for insights, where the tree structure helps to separate information
about different parts of the cluster, coming from potentially different sources. The hierarchy is basically:

- Abstract thing in the cluster (e.g. control plane, worker pool)
  - Entities that want to report something about the abstract thing (=informers)
    - The actual information (=insights)

There have been concerns about the size of the singleton resource, which I partially share, and I think it would be
reasonable to split it following some dimension, basically on this spectrum:

1. Every insight is a separate CR (fine-grained)
2. Each abstract thing is a separate CR (coarse-grained) with all insights for it
3. Each informer maintains a CR with all insights for all things it reports about
4. UpdateStatus singleton CR (current)

I am leaning towards (2) currently, because it addresses scaling concerns reasonably well, while avoiding the API
fragmentation issue that Status API is trying to solve, and from which (1) is partially suffering too. I think it is
easier to incrementally break things down (go from (4) in the direction of (1)) than to merge them back together, so I
think (4) is a good starting point but not necessarily the final form. The benefit of a single CR is that clients can
very easily process it, basically the idea was that it any client could just process all insights it cares about with
some form of a visitor pattern, and everything in a single CRD makes it easier to maintain consistent reporting patterns
like names, contracts and semantics.

## Insights and how they support `oc adm upgrade status`

The `UpdateStatus` API was directly designed to enable the `oc adm upgrade status` command prototype from 4.16 which was
very well received and even in its TechPreview state gained a lot of adoption in Red Hat.

### Control Plane (except nodes)

Two insight types are relevant here: `ClusterVersion` status insight and `ClusterOperator` status insight.

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
spec: {}
status:
  controlPlane:
    informers:
    - name: cpi
      insights:
      - type: ClusterVersion
        uid: cv-version
        clusterVersion: {see below}
      - type: ClusterOperator
        uid: co-config-operator
        clusterOperator: {see below}
      - type: ClusterOperator
        uid: co-etcd
        clusterOperator: {see below}
      - type: ClusterOperator
        uid: co-kube-apiserver
        clusterOperator: {see below}
      # type: ClusterOperator item for every Cluster Operator
```

These insights back the following part of the `oc adm upgrade status` output:

```
= Control Plane =
Assessment:      Progressing
Target Version:  4.15.0-ec.2 (from incomplete 4.14.1)
Updating:        etcd, kube-apiserver
Completion:      3% (1 operators updated, 2 updating, 30 waiting)
Duration:        1m29s (Est. Time Remaining: 1h25m)
Operator Health: 33 Healthy
```

#### ClusterVersion Status Insight

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
spec: {}
status:
  controlPlane:
    informers:
    - name: cpi
      insights:
      - type: ClusterVersion
        uid: cv-version
        clusterVersion:
          assessment: Progressing
          completion: 3
          conditions:
          - reason: ClusterVersionProgressing
            status: "True"
            type: Updating
          estimatedCompletedAt: "2025-02-04T14:07:01Z"
          resource:
            group: config.openshift.io
            name: version
            resource: clusterversions
          startedAt: "2025-02-04T12:40:31Z"
          versions:
            previous:
              metadata:
              - key: Partial
              version: 4.14.1
            target:
              version: 4.15.0-ec.2
```

```
= Control Plane =
Assessment:      Progressing
```
- `.assessment`

```
Target Version:  4.15.0-ec.2 (from incomplete 4.14.1)
```
- `.versions.target.version` and `.versions.previous.version` where "incomplete" comes from the `metadata` on the previous version

```
Completion:      3% (1 operators updated, 2 updating, 30 waiting)
```
- `.completion`, remaining part of the line comes from ClusterOperator insights

```
Duration:        1m29s (Est. Time Remaining: 1h25m)
```
- `.startedAt` and `.estimatedCompletedAt`

Additionally, the `Updating` condition allows USC to populate the `controlPlane.conditions`-level `Updating` condition, which allows
the clients to quickly detect whether the control plane is updating.

#### ClusterOperator Status Insight

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
spec: {}
status:
  controlPlane:
    informers:
    - name: cpi
      insights:
      - type: ClusterOperator
        uid: co-config-operator
        clusterOperator:
          name: config-operator
          conditions:
          - reason: AsExpected
            status: "True"
            type: Healthy
          - reason: Updated
            status: "False"
            type: Updating
          resource:
            group: config.openshift.io
            name: config-operator
            resource: clusteroperators
      - type: ClusterOperator
        uid: co-etcd
        clusterOperator:
          name: etcd
          conditions:
          - reason: AsExpected
            status: "True"
            type: Healthy
          - message: 'NodeInstallerProgressing: 2 nodes are at revision 33; 1 nodes are at revision 34'
            reason: NodeInstaller
            status: "True"
            type: Updating
          resource:
            group: config.openshift.io
            name: etcd
            resource: clusteroperators
      - type: ClusterOperator
        uid: co-kube-apiserver
        clusterOperator:
          name: kube-apiserver
          conditions:
          - reason: AsExpected
            status: "True"
            type: Healthy
          - message: 'NodeInstallerProgressing: 3 nodes are at revision 274; 0 nodes have achieved new revision 276'
            reason: NodeInstaller
            status: "True"
            type: Updating
          resource:
            group: config.openshift.io
            name: kube-apiserver
            resource: clusteroperators
      - type: ClusterOperator
        uid: co-0
        clusterOperator:
          conditions:
          - lastTransitionTime: "2025-02-04T12:42:00Z"
            message: ""
            reason: AsExpected
            status: "True"
            type: Healthy
          - lastTransitionTime: null
            message: ""
            reason: Pending
            status: "False"
            type: Updating
          name: "0"
          resource:
            group: config.openshift.io
            name: "0"
            resource: clusteroperators
      # type: ClusterOperator item for every Cluster Operator
```

```
Updating:        etcd, kube-apiserver
```
- `.clusterOperator.name` for every operator with `.clusterOperator.conditions.type: Updating` with a `True` status


```
Completion:      3% (1 operators updated, 2 updating, 30 waiting)
```
- Updated operators is the count of `.clusterOperator.conditions.type: Updating` with a `False` status and `Updated` reason
- Updating operators is the count of `.clusterOperator.conditions.type: Updating` with a `True` status
- Waiting operators is the count of `.clusterOperator.conditions.type: Updating` with a `False` status and `Pending` reason

```
Operator Health: 33 Healthy
```
- Healthy operators is the count of `.clusterOperator.conditions.type: Healthy` with a `True` status
- Reason allows to distinguish some detailed states like Available or Degraded

The informer that produces these insights is expected to encode interpretation logic, it is not required to just blindly
copy information from the source APIs. For example, this is the layer where we can implement the "if a ClusterOperator is
degraded for just few seconds now, it can still be considered healthy" loosening to reduce noise. Similarly, right now
we do not have a good data source for the "COs that are updating right now". By abstracting this behind a `Updating=True`
condition owned by the informer, we can improve this behind the scene from the current implementation that basically
considers any CO with `Progressing=True` while not already flipped its `operator` version, to a potentially better data
source with we would still need to implement.

### Control Plane (Nodes)

Two insight types are relevant here: `MachineControlPool` status insight and `Node` status insight. Both are also used
for the worker pool section.

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
spec: {}
status:
  controlPlane:
    informers:
    - name: cpi
      insights:
      - type: machineConfigPool
        uid: mcp-master
        machineConfigPool: {see below}
      - type: Node
        uid: node-master-1
        node: {see below}
      - type: Node
        uid: node-master-2
        node: {see below}
      - type: Node
        uid: node-master-3
        node: {see below}
```

These insights back the following part of the `oc adm upgrade status` output:

```
Control Plane Nodes
NAME                                        ASSESSMENT   PHASE     VERSION   EST   MESSAGE
ip-10-0-30-217.us-east-2.compute.internal   Outdated     Pending   4.14.1    ?     
ip-10-0-53-40.us-east-2.compute.internal    Outdated     Pending   4.14.1    ?     
ip-10-0-92-180.us-east-2.compute.internal   Outdated     Pending   4.14.1    ?     
```

#### MachinePoolConfig Status Insight

MCP status insight is actually not used by `oc adm upgrade status` currently, because it carries summary information
that is not that useful for control plane.

```
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
spec: {}
status:
  controlPlane:
    informers:
    - name: cpi
      insights:
      - machineConfigPool:
          name: master
          assessment: Pending
          completion: 0
          conditions:
          - lastTransitionTime: null
            message: ""
            reason: Pending
            status: "False"
            type: Updating
          resource:
            group: machineconfiguration.openshift.io
            name: master
            resource: machineconfigpools
          scopeType: ControlPlane
          summaries:
          - count: 3
            type: Total
          - count: 3
            type: Available
          - count: 0
            type: Progressing
          - count: 3
            type: Outdated
          - count: 0
            type: Draining
          - count: 0
            type: Excluded
          - count: 0
            type: Degraded
```

### Node Status Insight

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
spec: {}
status:
  controlPlane:
    informers:
    - name: cpi
      insights:
      - type: Node
        uid: node-cp-1
        node:
          name: ip-10-0-30-217.us-east-2.compute.internal
          conditions:
          - message: ""
            reason: Pending
            status: "False"
            type: Updating
          - reason: AsExpected
            status: "False"
            type: Degraded
          - message: ""
            reason: AsExpected
            status: "True"
            type: Available
          poolResource:
            group: machineconfiguration.openshift.io
            name: master
            resource: machineconfigpools
          resource:
            group: core
            name: ip-10-0-30-217.us-east-2.compute.internal
            resource: nodes
          scopeType: ControlPlane
          version: 4.14.1
      - type: Node
        uid: node-cp-2
        node:
          name: ip-10-0-53-40.us-east-2.compute.internal
          conditions:
          - message: ""
            reason: Pending
            status: "False"
            type: Updating
          - message: ""
            reason: AsExpected
            status: "False"
            type: Degraded
          - message: ""
            reason: AsExpected
            status: "True"
            type: Available
          poolResource:
            group: machineconfiguration.openshift.io
            name: master
            resource: machineconfigpools
          resource:
            group: core
            name: ip-10-0-53-40.us-east-2.compute.internal
            resource: nodes
          scopeType: ControlPlane
          version: 4.14.1
      - type: Node
        uid: node-cp-3
        node:
          name: ip-10-0-92-180.us-east-2.compute.internal
          conditions:
          - message: ""
            reason: Pending
            status: "False"
            type: Updating
          - message: ""
            reason: AsExpected
            status: "False"
            type: Degraded
          - message: ""
            reason: AsExpected
            status: "True"
            type: Available
          poolResource:
            group: machineconfiguration.openshift.io
            name: master
            resource: machineconfigpools
          resource:
            group: core
            name: ip-10-0-92-180.us-east-2.compute.internal
            resource: nodes
          scopeType: ControlPlane
          version: 4.14.1
```

```
Control Plane Nodes
NAME                                        ASSESSMENT   PHASE     VERSION   EST   MESSAGE
ip-10-0-30-217.us-east-2.compute.internal   Outdated     Pending   4.14.1    ?     
```
- Each node insight is a row in the node table, with `.name`, `.version`, `.estToComplete` (missing here, interpreted as `?`) and `.message` (also emtpy, populated only when there is something interesting to say).
- Assessment and phase are client-side interpretations of the condition states and reasons, which felt more correct and robust than implementing them through string state machine-like Enums. I would expect the presentation to adapt to the API, rather than the other way around.
- There is an opportunity to include the new NodeUpdateStatus API data in the Node insight, but was not done yet.

### Worker Pools

Two insight types are relevant here: `MachineControlPool` status insight and `Node` status insights. Each item under `workerPools` should contain one `MachineControlPool` insight and multiple `Node` insights (and Health insight, when something is worth reporting).

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
spec: {}
status:
  workerPools:
  - name: worker
    resource:
      group: machineconfiguration.openshift.io
      name: worker
      resource: machineconfigpools
    informers:
    - name: nodes
      insights:
      - type: machineConfigPool
        uid: mcp-worker
        machineConfigPool: {see below}
      - type: Node
        uid: node-worker-1
        node: {see below}
      - type: Node
        uid: node-worker-2
        node: {see below}
      - type: Node
        uid: node-worker-3
        node: {see below}
  # Potentially more worker pools -> informers -> insights
```

These insights back the following part of the `oc adm upgrade status` output:

```
= Worker Upgrade =

WORKER POOL   ASSESSMENT   COMPLETION   STATUS
worker        Pending      0% (0/3)     3 Available, 0 Progressing, 0 Draining

Worker Pool Nodes: worker
NAME                                        ASSESSMENT   PHASE     VERSION   EST   MESSAGE
ip-10-0-20-162.us-east-2.compute.internal   Outdated     Pending   4.14.1    ?     
ip-10-0-4-159.us-east-2.compute.internal    Outdated     Pending   4.14.1    ?     
ip-10-0-99-40.us-east-2.compute.internal    Outdated     Pending   4.14.1    ?      
```

#### MachinePoolConfig Status Insight

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
spec: {}
status:
  workerPools:
  - name: worker
    resource:
      group: machineconfiguration.openshift.io
      name: worker
      resource: machineconfigpools
    informers:
    - name: nodes
      insights:
      - type: MachineConfigPool
        uid: mcp-worker
        machineConfigPool:      
          assessment: Pending
          completion: 0
          conditions:
          - reason: Pending
            status: "False"
            type: Updating
          name: worker
          resource:
            group: machineconfiguration.openshift.io
            name: worker
            resource: machineconfigpools
          scopeType: WorkerPool
          summaries:
          - count: 3
            type: Total
          - count: 3
            type: Available
          - count: 0
            type: Progressing
          - count: 3
            type: Outdated
          - count: 0
            type: Draining
          - count: 0
            type: Excluded
          - count: 0
            type: Degraded
```

Each MCP insight (across `workerPools` items) is a row in the worker pool table:

```
WORKER POOL   ASSESSMENT   COMPLETION   STATUS
worker        Pending      0% (0/3)     3 Available, 0 Progressing, 0 Draining
``` 
- `.machineConfigPool.name`, `.assessment`, `.completion` are directly mapped to the output
- `.machineConfigPool.summaries` are used to populate the counts in the status line, some are hidden when zero

### Node Status Insight

These back the worker pool node tables,  identical to the control plane node table:

```yaml
Worker Pool Nodes: worker
NAME                                        ASSESSMENT   PHASE     VERSION   EST   MESSAGE
ip-10-0-20-162.us-east-2.compute.internal   Outdated     Pending   4.14.1    ?     
ip-10-0-4-159.us-east-2.compute.internal    Outdated     Pending   4.14.1    ?     
ip-10-0-99-40.us-east-2.compute.internal    Outdated     Pending   4.14.1    ?      
```

When there are more worker pools, there will be more worker pool node tables. The `oc adm upgrade status` default mode
limits the table size, and hides rows for not interesting nodes when the pool contains more nodes.

### Health Insights

Health insights are not displayed separately for control plane and worker pools, they are collected across whole API
and displayed in a table (there are mockups for showing control plane / worker pool in a column though):

```
= Update Health =
SINCE   LEVEL     IMPACT   MESSAGE
1m29s   Warning   None     Previous update to 4.14.1 never completed, last complete update was 4.14.0-rc.7

Run with --details=health for additional description and links to related online documentation
```
and in a detailed mode (hinted about in normal node):

```
= Update Health =
Message: Previous update to 4.14.1 never completed, last complete update was 4.14.0-rc.7
  Since:       1m29s
  Level:       Warning
  Impact:      None
  Reference:   https://docs.openshift.com/container-platform/latest/updating/troubleshooting_updates/gathering-data-cluster-update.html#gathering-clusterversion-history-cli_troubleshooting_updates
  Resources:
    clusterversions.config.openshift.io: version
  Description: Current update to 4.15.0-ec.2 was initiated while the previous update to version 4.14.1 was still in progress
```

Each health insight is basically one item in the table:

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
spec: {}
status:
  controlPlane:
    informers:
    - name: cpi
      insights:
      - type: Health
        uid: health-partial-update
        health:
          impact:
            description: Current update to 4.15.0-ec.2 was initiated while the previous update to version 4.14.1 was still in progress
            level: Warning
            summary: Previous update to 4.14.1 never completed, last complete update was 4.14.0-rc.7
            type: None
          remediation:
            reference: https://docs.openshift.com/container-platform/latest/updating/troubleshooting_updates/gathering-data-cluster-update.html#gathering-clusterversion-history-cli_troubleshooting_updates
          scope:
            type: ControlPlane
            resources:
            - group: config.openshift.io
              name: version
              resource: clusterversions
          startedAt: "2025-02-04T12:40:31Z"
```
