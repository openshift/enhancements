---
title: control-plane-machineset-vertical-scaling
authors:
  - "@bergmannf"
reviewers:
  - "@JoelSpeed"
approvers:
  - "@JoelSpeed"
api-approvers: 
  - None
creation-date: 2023-10-30
last-updated: 2023-10-30
tracking-link:
  - https://issues.redhat.com/browse/OSD-15261
see-also: []
replaces: []
superseded-by: []
---

# Control plane machineset automatic vertical scaling

## Summary

To allow control plane nodes to remain at an adequate size for its cluster, this
proposal introduces new configuration for the control plane machineset operator
to allow it to make automated scaling decisions for control plane node sizing.

## Motivation

During normal operation of an OpenShift cluster, as worker node count and
workloads increase, it becomes necessary to increase resources available to the
control plane nodes.

During SRE operations of the managed OpenShift product, there was already a
trigger indentified, when these increases are required: right now these triggers
are based on cluster load over the last 8 hours (details care available in [this
PrometheusAlert](https://github.com/openshift/managed-cluster-config/blob/master/deploy/sre-prometheus/100-control-plane-resizing.PrometheusRule.yaml#L93C1-L93C1)
that is used in OSD clusters). In addition to the average load our [recommended
control plane
practices](https://docs.openshift.com/container-platform/4.13/scalability_and_performance/recommended-performance-scale-practices/recommended-control-plane-practices.html)
specify control plane sizes based on worker nodes.

Instead of requiring the adjustments to be performed by hand, the operator
should be able to automatically determine if a control plane node size change is
required.

On the other hand, it should also be possible to determine that control plane
nodes have become too large for a cluster, because load has decreased. In this
case the operator should also be able to automatically decide to scale the
control plane down again.

With the automatic increase of the control plane size, cluster performance can
remain high, while automated decrease of the size will ensure that spend for
users in a cloud environment is only has high as needed.

### User Stories

* As an OpenShift administrator, I want to know that the platform will always
  have enough resources available to it's control plane nodes without manual
  intervention.
* As an OpenShift administrator, I want to know that the platform can reduce
  spending on control plane nodes if they are oversized for the current cluster
  load.
* As an OpenShift administrator, I want to be able to control when sizing up and
  down occurs, to adjust it to workloads on my cluster that might be burstable
  to prevent the automatic scaling from occurring to often.

### Goals

* Allow automatic vertical scaling of control plane nodes.
* Allow users to configure the sizes that can be chosen for automatic scaling.
* Allow users to configure the thresholds when scaling will occur for CPU and
  memory usage.
* Allow users to configure how often checks for automatic scaling should be run.
* Allow users to configure how long after a scale up or scale down, no more
  scaling should be performed.
* Allow users to completely disable scale up.
* Allow users to completely disable scale down.

### Non-Goals

* Horizontal scaling of control plane nodes

Horizontal scaling of control plane nodes is not part of this enhancement,
because increasing the count of control plane nodes must take into account
special cases like [etcd
quorum](https://etcd.io/docs/v3.5/faq/#why-an-odd-number-of-cluster-members) and
[etcd performance](https://cloud.redhat.com/blog/a-guide-to-etcd).

As the trade offs in case of increasing control plane node number might not
always be desired, and there is no operator that can scale the control plane
horizontally automatically this is out of scope for this enhancement.

## Proposal

New configuration and a reconcile loop for the
`control-plane-machine-set-operator` will be introduced that will monitor
resource usage of the control plane nodes and automatically modify the
`ControlPlaneMachineSet` if scaling the nodes is determined to be required.

The new configuration will specify if scaling up and down is enabled and allow
configuring when to trigger scaling. It will also allow configuring which
machine types are to be used for scaling, putting an upper and lower limit on
possible automatic scaling.

### Workflow Description

1. The OpenShift adminstrator creates a valid
   `control-plane-machine-set-autoscaling` `CR` in the `openshift-machine-api`
   namespace (or the respective namespace `control-plane-machineset-operator` is
   running in), to configure the automatic vertical scaling.

### API Extensions

This proposal requires a new custom resource to configure when and how
automatically scale the control plane by modifying the control plane machinset.

### Implementation Details/Notes/Constraints [optional]

The implementation for this feature, requires a data source to gather current
CPU and memory usage for nodes, as well as available resources on the nodes.

To gather available resource information the default
[Node](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#node-v1-core)
API endpoint can be used.

Using the [horizontal pod
autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)
as a reference, they are using the [metrics
API](https://kubernetes.io/docs/tasks/debug/debug-cluster/resource-metrics-pipeline/)
to gather performance statistics for nodes. 

Using the metrics API has the least requirements as it does not expect a running
Prometheus monitoring stack. Using Prometheus generated data could make the
implementation easier with respect to data aggregation.

Data aggregation will be required using the metrics API, as decisions about
scaling nodes up and down, must not be performed based on performance usage at a
single point in time. Instead the operator will have to gather and store usage
data for a certain period of time, to decide if scaling is required.

As an example with a configured `timeWindow` (see
[configuration](#control-plane-machineset-operator-configuration)) of 30 minutes
for scaling up, the operator has to keep querying the metrics API and averaging
values until enough data has been aggregated to know the load average for the
last 30 minutes.

If the metrics API returns it's own window of ~60 second aggregations, the
operators will have to retrieve 30 values (one per minute), before any scaling
decision should be made.

#### Hypershift

Hypershift does not use `ControlPlaneMachineSet` to manage the control-planes
for hosted clusters, as those are not running on their own virtual machines.

Because of this, no special handling should be required.

### Risks and Mitigations

#### Risk 1: Higher costs when scaling up

Scaling up will increase costs for running the cluster, as bigger nodes will
incur higher costs from the cloud provider.

To mitigate unbounded scaling the user can setup the possible instance sizes
available for scaling - once the highest size has been scaled to, no more
scaling will occur.

#### Risk 2: Cluster instability due to scaling down

Scaling down might also impact the clusters ability to accommodate a workload if
resource usage was uncharacteristically low for the detection period of scaling.
This turns into an issue, if the cluster still would have required the bigger
control plane nodes.

To mitigate this issue the scale down behavior should be disabled by default and
require an explicit opt-in by the cluster administrator.

#### Risk 3: Control plane node churn

Automatically scaling up and down control planes could impact cluster
reliability and availability, if performed too often.

As such the additional logic must ensure that there is a grace period between
changes to the machine size, during which no more scaling will be performed.

### Drawbacks

Some users might not want this feature and rather opt to scale control planes
manually.

## Design Details

### Control Plane MachinSet operator configuration

As automatic scaling will require adjustments to certain aspects of its logic,
the implementation should implement a way for administrators of an OpenShift
cluster to configure the specifics of how and when scaling will occur.

This configuration will be performed using a new custom resource
`ControlPlaneMachineSetAutoscaling`.

The **required** configurations will include the following properties:
- `machineConfiguration`: this YAML object copies the [
  configuration](https://docs.openshift.com/container-platform/4.13/machine_management/control_plane_machine_management/cpmso-configuration.html)
  from controlplane machinset operator to configure the cloud platform's
  machinesetup with a machinetype and concrete values that must be adjusted. The
  implementation should use multiple [discriminated
  unions](https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md#discriminated-unions).
  One will specify the `machineType` used:
  - `machineType`: currently always `machines_v1beta1_machine_openshift_io`, but
  should be able to support [cluster API](https://cluster-api.sigs.k8s.io/) in
  the future. 
    - `machines_v1beta1_machine_openshift_io`: specifies options for the
    machines API based provisioner.
      - `machineConfigurations`: specifies the concrete specifications for the
        machines.
  
  The `machineConfiguration` will use a discriminated union again to dispatch
  for the cloud provider. The dispatch key will be `platform`. To start the
  supported cloud providers must include:
  - `Azure` and configuration for `vmSize`
  - `GCP` and configuration for `machineType`
  - `AWS` and configuration for `instanceType`
  - `vSphere` and configuration for `memoryMiB`, `numCPUs` and `diskGiB`.
  
  Each element that denotes an instance configuration also has to include a
  `weight` property to specify the order in which scaling should occur.

The **optional** configurations will include the following properties:

- `syncPeriod`: how often will the operator check resource usage to see if scaling
  should be performed.
- `scaleUp`: defines configuration for scaling up. This includes the following
  sub elements:
  - `stabilizationWindow`: duration to wait after a scale up before another
    scaling operation can occur.
  - `selectPolicy`: `next` to select the next bigger instance size.
  - `timeWindow`: the time CPU and memory usage have to be above the
    thresholds, before triggering a scale up event.
  - `thresholds`: sets up the thresholds, which will trigger a scale down if
    load is higher:
    - `cpuLoadAverage`: a percentage of averge CPU load.
    - `memoryUsage`: a percentage of average memory usage.
- `scaleDown`: defines configuration for scaling up. This includes the following
  sub elements:
  - `stabilizationWindow`: duration to wait after a scale down before another
    scaling operation can occur.
  - `selectPolicy`: `next` to select the next smaller instance size.
  - `timeWindow`: the time CPU and memory usage have to be above the
    thresholds, before triggering a scale up event.
  - `thresholds`: sets up the thresholds, which will trigger a scale down if
    load is lower:
    - `cpuLoadAverage`: a percentage of averge CPU load.
    - `memoryUsage`: a percentage of average memory usage.

A complete custom resources instance in YAML format could then look like this:

```yaml
controlplaneautoscaling:
  machineConfiguration:
    machineType: machines_v1beta1_machine_openshift_io
    machines_v1beta1_machine_openshift_io:
      machineConfigurations:
        platform: AWS
        aws:
        - weight: 1
          instanceSize: r4.large
        - weight: 2
          instanceSize: r4.xlarge
  syncPeriod: [duration as specified in golang: e.g. 5s, 15s]
  scaleUp:
    stabilizationWindow: [duration as specified in golang: e.g. 5s, 15s]
    selectPolicy: "next"
    timeWindow: [duration as specified in golang: e.g. 5m, 15m]
    thresholds:
      cpuLoadAverage: [0-100 as percentage]
      memoryUsage: [0-100 as percentage]
  scaleDown:
    stabilizationWindow: [duration as specified in golang: e.g. 5s, 15s]
    selectPolicy: "next"
    timeWindow: [duration as specified in golang: e.g. 5m, 15m]
    thresholds:
      cpuLoadAverage: [0-100 as percentage]
      memoryUsage: [0-100 as percentage]
```

### Data gathering

As specified the operator should not require access to data from operators that
might not be installed in the cluster (e.g. `prometheus`).

Instead the operator should gather the required data via the [metrics
API](https://kubernetes.io/docs/tasks/debug/debug-cluster/resource-metrics-pipeline/)
that is also used for the horizontal and vertical pod autoscalers.

Data available from the metrics API includes:

- CPU usage in `cores`.
- Memory usage in a SI units.
- The time window used to aggregate the above mentioned data.

To gather data about the available CPU cores and memory available to control
plane nodes, the operator should use the kubernetes
[Node](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#node-v1-core)
API.

As the window returned by metrics API is likely shorter than required window for
scaling decisions, the operator should gather its own average to allow making
the scaling decision based on the metrics data.

### Workflow for automatic scaling

Automatic scaling should be triggered the same as [horizontal pod
autoscaling](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/) -
that runs periodically to check if scaling the control-plane machines is
required.

In addition the implementation has to ensure that scaling decision are not based
on single point-in-time values to prevent scaling up or down based on usage
spikes.

Instead a running average (or similar) should be used to decide about scaling.

If for any reason metrics can not be gathered, the operator must remain
available and only the autoscaling should be disabled / non-functional.

### Additional RBAC permissions

The operator will require access to the following resources to gather the data:

- Read access for `/apis/metrics.k8s.io/v1beta1/nodes`.
- Read access for `/api/v1/nodes` (this is already in place for OSD OpenShift
  clusters, but has to be verified for a OCP OpenShift installations).

### Open Questions

1. Data gathering via metrics API requires calculating its own average again,
   while a TSDB like Prometheus could provide this information directly. Should
   the operator allow different data gathering strategies, so Prometheus could
   be used instead of the metrics API.

### Test Plan

Testing must be thorough to ensure no unnecessary scaling occurs automatically,
to prevent service interruption and higher costs.

- E2E tests could schedule pods using the `stress` program and low scaling
  windows to trigger the automatic behavior.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Upgrade / Downgrade Strategy

Upgrade and downgrade strategy will remain the same for the operator.

### Version Skew Strategy

Version strategy will remain the same for the operator.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

Upgrade and downgrade strategy will remain the same for the operator.

### Version Skew Strategy

Version strategy will remain the same for the operator.

### Operational Aspects of API Extensions

This proposal does not add API extensions.

#### Failure Modes

This proposal does not add API extensions.

#### Support Procedures

If automatic scaling does not perform according to expectations it should be
disabled.

As this is a quality of life feature it should not impact cluster availability,
unless manual scaling of control plane nodes is not performed when required.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

Instead of implementing automatic vertical scaling in the
`control-plane-machinset-operator` it could be implemented in a new operator
that could react to specific Prometheus alerts (e.g.
[ControlPlaneNeedsResizingSRE](https://github.com/openshift/managed-cluster-config/blob/master/deploy/sre-prometheus/100-control-plane-resizing.PrometheusRule.yaml#L93C1-L93C1)).
