---
title: machine-config-pool-update-surge-and-nodedraintimeout
authors:
  - jupierce
reviewers: 
  - TBD
approvers: 
  - TBD
api-approvers: 
  - TBD
creation-date: 2024-04-29
last-updated: 2024-04-29
tracking-link:
  - TBD
see-also:
  - https://github.com/openshift/enhancements/pull/1571
---

# MachineConfigPool Update Surge and NodeDrainTimeout

## Summary

Add `MaxSurge` and `NodeDrainTimeout` semantics to `MachineConfigPool` to improve the predictability 
of standalone OpenShift cluster updates. `MaxSurge` allows clusters to scale above configured replica
counts during an update -- helping to ensure worker node capacity is available for drained workloads. 
`NodeDrainTimeout` limits the amount of time an update operation will block waiting for a potentially 
stalled drain to succeed -- helping to ensure that updates can proceed (by incurring disruption) even 
in the presence of poorly configured workloads.

## Motivation

During a typical worker node update for an OpenShift cluster, it is necessary to "cordon" nodes (prevent new pods from being scheduled on a node)
and "drain" them (attempt to migrate workloads by rescheduling its pods onto uncordoned nodes). Workers generally need to be rebooted during a
cluster update and draining nodes is best practice before rebooting them. If they were not drained first, pods running
on a node targeted by the update process could be terminated with no other viable pods on the cluster to 
handle the workload. This outcome can cause a disruption in the service the terminated pod was attempt to provide. For example,
an incoming web request may not be routable to a pod for a given Kubernetes service - resulting in errors being returned
to the consumers of that service.

With appropriate cluster management, node draining can be used to ensure that sufficient pods are running to satisfy workload requirements
at all times - even during updates. "Appropriate cluster management," though, is a multi-faceted challenge involving considerations
from pod replicas to cluster topology.

### Managing Worker Node Capacity

One aspect of this challenge is ensuring that, while a node is being drained, there is sufficient worker node capacity (CPU/memory/other
resources) available for new pods to take the place of old pods from the node being drained. Consider the reductive example of 
a static cluster with a single worker node. If there is an attempt to drain the node in this example, there is no additional worker 
node capacity available to schedule new pods to replace the pods being drained. This can result in a stalled drain -- one that 
does not terminate until there is external intervention.

Stalled drains create a frustrating experience for operations teams as they require analysis and intervention. They can also
make it impossible to predict when an update will complete -- complicating work schedules and communications. There are a number of reasons 
drains can stall, but simple lack of spare worker node capacity is a common one. The reason is that running an over-provisioned cluster
(i.e. with more worker nodes than actually required for the steady state workloads) is not always cost-effective. Running
an extra node 24x7 simply to ensure capacity for the short period while another node is being drained is inefficient.

One cost-effective approach to ensure capacity is called "surging". With a surge strategy, during an update, the platform
is permitted to bring additional worker nodes online, to accommodate workloads being drained from existing nodes. After an 
update concludes, the surged nodes are scaled down and the cluster resumes its steady state.

HyperShfit Hosted Control Planes (HCP) already support the surge concept. HyperShift `NodePools`
expose `maxUnavailable` and `maxSurge` as configurable options during updates: https://hypershift-docs.netlify.app/reference/api/#hypershift.openshift.io/v1beta1.RollingUpdate . 
Unfortunately, standalone OpenShift, which uses `MachineConfigPools`, does not. To workaround this
limitation for managed services customers, Service Delivery developed a custom Machine Update Operator (MUO)
which can surge a standalone cluster during an update (see [reserved capacity feature](https://github.com/openshift/managed-upgrade-operator/blob/a56079fda6ab4088f350b05ed007896a4cabcd97/docs/faq.md)).

### Preventing Other Stalled Drains

As previously mentioned, there are other reasons that drains can stall. For example, `PodDisruptionBudgets` can
be configured in such a way as to prevent pods from draining even if there is sufficient capacity for them
to be rescheduled on other nodes. A powerful (though blunt) tool to prevent drain stalls is to limit the amount of time
a drain operation is permitted to run before forcibly terminating pods and allowing an update to proceed. 
`NodeDrainTimeout`, in HCP's `NodePools` allows users to configure this timeout.
The Managed Update Operator also supports this feature with [`PDBForceDrainTimeout`](https://github.com/openshift/managed-upgrade-operator/blob/master/docs/faq.md).

This enhancement includes adding `NodeDrainTimeout` to `MachineConfigPools` to provide this feature in standlone 
cluster environments.

### User Stories

Implementing surge and node drain timeout support in `MachineConfigPools` can simplify cluster management for self-managed 
standalone clusters as well as managed clusters (i.e. Service Delivery can remove this customized behavior from the MUO and use
more of the core platform).

* As an Operations team managing one or more standalone clusters, I want to
  help ensure smooth updates by surging my worker node count without constantly 
  having my cluster over-provisioned.
* As an Operations team managing one or more standalone clusters, I want to 
  ensure my cluster update makes steady progress by limiting the amount of time a node drain can
  consume.
* As an engineer in the Service Delivery organization, I want to use core platform
  features instead of developing, evolving, and testing the Managed Update Operator.
* As an Operations team managing standalone and HCP based OpenShift clusters, I
  want a consistent update experience leveraging a surge strategy and/or 
  node drain timeouts regardless of the cluster profile.

### Goals

- Implement an update configuration, including `MaxSurge`, similar to HCP's `NodePool`
  in standalone OpenShift's `MachineConfigPool`. 
- Implement `NodeDrainTimeout`, similar to HCP's `NodePool`, in standalone OpenShift's `MachineConfigPool`.
- Provide a consistent update controls between standalone and HCP cluster profiles. 
- Allow Service Delivery to deprecate their MUO reserved capacity & `PDBForceDrainTimeout` features and use more of the core platform.

### Non-Goals

- Address all causes of problematic updates.
- Prevent workload disruption when `NodeDrainTImeout` is utilized.
- Fully unify the update experience for Standalone vs HCP.

## Proposal

The HyperShift HCP `NodePool` exposes a [`NodePoolManagement`](https://hypershift-docs.netlify.app/reference/api/#hypershift.openshift.io/v1beta1.NodePoolManagement)
stanza which captures traditional `MachineConfigPool` update semantics ([`MaxUnavailable`](https://docs.openshift.com/container-platform/4.14/rest_api/machine_apis/machineconfigpool-machineconfiguration-openshift-io-v1.html#spec)) 
as well as the ability to specify a `MaxSurge` preferences. HCP's `NodePool` also exposes a `NodeDrainTimeout` configuration
option.

This enhancement proposes an analog for `NodePoolManagement` and `NodeDrainTimeout` be added
to standalone OpenShift's `MachineConfigPool` custom resource.

### Workflow Description

**Cluster Lifecycle Administrator** is a human user responsible for triggering, monitoring, and 
managing all aspects of a cluster update. They are operating a standalone OpenShift cluster.

1. The cluster lifecycle administrator desires to ensure that there is sufficient worker node capacity during
   updates to handle graceful termination of pods and rescheduling of workloads.
2. They want to avoid other causes of drain stalls by limiting the amount of time permitted for any drain operation.
3. They configure worker `MachineConfigPools` on the cluster with a `MaxSurge`.
   value that will bring additional worker node capacity online for the duration of an update.
4. They configure worker `MachineConfigPools` on the cluster with a `NodeDrainTimeout` value of 30 minutes to 
   limit the amount of time non-capacity related draining issues can stall the overall update.

### API Extensions

#### API Overview
The Standalone `MachineConfigPool` custom resource is updated to include new update strategies (one of which 
supports`MaxSurge`) and `NodeDrainTimeout` semantics identical to HCP's `NodePool`.

Documentation for these configuration options can be found in HyperShift's API reference: 
- https://hypershift-docs.netlify.app/reference/api/#hypershift.openshift.io/v1beta1.NodePoolManagement exposes `MaxSurge`.
- https://hypershift-docs.netlify.app/reference/api/#hypershift.openshift.io/v1beta1.NodePoolSpec exposes `NodePoolTimeout`

Example `MachineConfigPool` including both `NodeDrainTimeout` and a `MaxSurge` setting:
```yaml
kind: MachineConfigPool
spec:
  # Existing spec fields are not shown.    

  # Adopted from NodePool to create consistency and further our goal
  # to improve the reliability of worker updates.
  nodeDrainTimeout: 10m
      
  # New policy analog to NodePool.NodePoolManagement.
  machineManagement:
    upgradeType: "Replace"
    replace:
      strategy: "RollingUpdate"
      rollingUpdate:
        maxUnavailable: 0
        maxSurge: 4
```

Like HCP, `UpgradeType` will support:
- `InPlace` where no additional nodes are brought online to support draining workloads.
- `Replace` where new nodes will be brought online with `MaxSurge` support.

#### InPlace Update Type

The `InPlace` update type is similar to `MachineConfigPools` traditional behavior where a 
user can configure the `MaxUnavailable` nodes. This approach assumes the number (or percentage)
of nodes specified by `MaxUnavailable` can be drained simultaneously with workloads finding
sufficient resources on other nodes to avoid stalled drains.

```yaml
kind: MachineConfigPool
spec:
  # Existing spec fields are not shown.    

  machineManagement:
    upgradeType: "InPlace"
    inPlace:
      maxUnavailable: 10%
```

Nodes are rebooted after they are drained.

#### Replace Update Type

The `Replace` update type removes old machines and replaces them with new instances. It supports
two strategies:
- `OnDelete` where a new machine is brought online only after the old machine is deleted.
- `RollingUpdate` which supports the `MaxSurge` option.

```yaml
kind: MachineConfigPool
spec:
  # Existing spec fields are not shown.    

  machineManagement:
    upgradeType: "Replace"
    replace:
      strategy: "RollingUpdate"
      rollingUpdate:
        maxUnavailable: 0
        maxSurge: 4
```

`MaxSurge` applies independently to each associated MachineSet. For example, if three MachineSets are associated
with a MachineConfigPool, and `MaxSurge` is set to 4, then it is possible for the cluster to surge up to 12 nodes
(4 for each of the 3 MachineSets).

### Topology Considerations

Multi-AZ (availability zone) clusters function by using one or more `MachineSets` per zone. In order for this enhancement
to work across all zones, all such `MachineSets` should be associated with a `MachineConfigPool` with well considered
values for `MaxSurge` and `NodeDrainTimeout`.

Each `MachineSet` associated with a `MachineConfigPool` will be permitted to scale by the `MaxSurge` number of nodes. 
The alternative (trying to spread a surge value evenly across `MachineSets`) is problematic. Consider a cluster with two `MachineSets`:
- machine-set-1a which creates nodes in availability zone us-east-1a.
- machine-set-1b which creates nodes in availability zone us-east-1b.

Further, assume that `MaxSurge` is set to 1 for the `MachineConfigPool` associated with these `MachineSets`.

There may be pods running on 1b nodes that can only be scheduled on 1b nodes (e.g. due to taints / affinity / 
topology constraints, machine type, etc.). If `MaxSurge` was interpreted in such a way as to only surge machine-set-1a by 1 node,
constrained pods requiring 1b nodes could not benefit from this additional capacity.

Instead, this enhancement proposes each `MachineSet` be permitted to surge up to the `MachineConfigPool` surge
value independently. 


#### Hypershift / Hosted Control Planes

N/A. Hosted Control Planes provide the model for the settings this enhancement seeks to expose in standalone clusters.

#### Standalone Clusters

The `MachineConfigPool` custom resource must be updated to expose the new semantics.  The existing `spec.maxUnavailable`
will be deprecated in favor of the more expressive `MachineManagement` stanza. 

#### Single-node Deployments or MicroShift

N/A.

### Implementation Details/Notes/Constraints

`MachineSets` or CAPI equivalents must support `MachineConfigPool` values for `MaxSurge`. When `MaxSurge` is greater
than 0, the controller can instantiate more machines than the number of specified `MachineSet` replicas. 
This must work seamlessly with all cluster autoscaler options. The cluster autoscaler may need to be aware of 
surge operations to prevent conflicting management of cluster machines.

### Risks and Mitigations

Service Delivery believes this enhancement is a key to dramatically simplifying the MUO in conjunction with
https://github.com/openshift/enhancements/pull/1571 . Without this enhancement, https://github.com/openshift/enhancements/pull/1571
may not be useful to Service Delivery without this enhancement as well. 

### Drawbacks

The primary drawback is that alternative priorities are not pursued or that the investment is not ultimately
warranted by the proposed business value.

### Removing a deprecated feature

- `MachineConfigPool.spec.maxUnavailable` will be deprecated. 

## Upgrade / Downgrade Strategy

This feature is integral to standalone updates. Preceding sections discuss its behavior.

## Version Skew Strategy

N/A.

## Operational Aspects of API Extensions

The new stanzas are specifically designed to be tools used to improve update predictability
and reliability for operations teams. Preceding sections discuss its behavior.

## Support Procedures

The machine-api-operator logs will indicate the decisions being made to actuate the new configuration
fields. If machines are scaled into the cluster during an update but are unable to successfully join
the cluster, this scenario is debugged just as if the problem occurred during normal scaling operations.

## Alternatives

1. The status quo of standalone updates and the MUO can be maintained. We can assume
   that customers impacted by the existing operational burden of drain timeouts will
   find their own solutions or migrate to HCP.
2. Aspects of the MUO could be incorporated into the OpenShift core. Unfortunately, the MUO
   is deeply integrated into SD's architecture and is not easily productized.
