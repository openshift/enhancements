---
title: control-plane-machineset-vertical-scaling
authors:
  - "@bergmannf"
reviewers:
  - "@JoelSpeed"
approvers:
  - "@JoelSpeed"
api-approvers: 
  - "@JoelSpeed"
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
proposal introduces new operator that monitors resource usage in the cluster.
Based on configured thresholds it will use the control plane machineset to make
apply automated scaling decisions for control plane node sizing.

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
   `control-plane-machine-set-autoscaler` `CR` in the `openshift-machine-api`
   namespace (or the respective namespace `control-plane-machineset-operator` is
   running in), to configure the automatic vertical scaling.

### API Extensions

This proposal requires a new custom resource to configure when and how to
automatically scale the control plane by modifying the control plane machinset.

As multiple configurations could potentially interrupt each other's work, this
API should act like a *Configuration API* (see: [API
conventions](https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md#discriminated-unions)),
but the concrete API CR instances should be scoped to the namespace the new
operator will run in.

```go
package cpmsscaling

// ControlPlaneMachineSetAutoScaler enables the user to define rules, when to
// automatically scale control plane nodes vertically in size - up or down.
type ControlPlaneMachineSetAutoScaler struct {
	// MachineConfiguration defines the possible flavors of virtual machines
	// that the scaling algorithm has available to scale either up or down.
	// +kubebuilder:validation:Required
	MachineConfiguration MachineConfiguration
	// SyncPeriod allows the user to define how often the controller will check
	// the configured triggers to see if scaling up or down is required.
	//
	// When omitted, this means the user has no opinion and the value is left to
	// the platform to choose a good default, which is subject to change over
	// time. The current default is 30m.
	SyncPeriod Duration
	// ScaleUp allows the user to define specifics around scale up decisions.
	// For example it allows the to specify possible triggers like CPU or memory
	// utilization.
	//
	// When omitted, the value will default to disabling scaling up.
	//
	// +kubebuilder:validation:Optional
	ScaleUp ScaleDefinition
	// ScaleDown allows the user to define specifics around scale down
	// decisions.
	// For example it allows the to specify possible triggers like CPU or memory
	// utilization.
	//
	// When omitted, the value will default to disabling scaling down.
	//
	// +kubebuilder:validation:Optional
	ScaleDown ScalingDefinition
}

// MachineConfiguration defines the changes to be made to the
// controlplanemachineset in case of scaling up or down. It lets the customer
// specify platform specific options to increase or decrease CPU or memory.
type MachineConfiguration struct {
	// MachineType is the union discriminator.
	// Users are expected to set this value to the name of the platform.
	// Currently the only valid value is 'machines_v1beta1_machine_openshift_io'
	// +unionDiscriminator
	// +kubebuilder:validation:Enum:="machines_v1beta1_machine_openshift_io"
	// +kubebuilder:validation:Required
	MachineType string
	// OpenShiftMachineV1Beta1Machine defines the template for creating Machines
	// from the v1beta1.machine.openshift.io API group.
	OpenShiftMachinesV1Beta1Machine MachineConfigurationMAPI
}

type MachineConfigurationMAPI struct {
	// Platform identifies the platform for which the FailureDomain represents.
	// Currently supported values are AWS, Azure, and GCP.
	// +unionDiscriminator
	// +kubebuilder:validation:Enum:="AWS","Azure","GCP","Nutanix","Openstack","Vsphere"
	// +kubebuilder:validation:Required
	Platform string
	// AWS configures machine information for the AWS platform.
	// +kubebuilder:validation:Optional
	Aws AWSMachineConfiguration
	// Azure configures machine information for the Azure platform.
	// +kubebuilder:validation:Optional
	Azure AzureMachineConfiguration
	// Gcp configures machine information for the Gcp platform.
	// +kubebuilder:validation:Optional
	Gcp GCPMachineConfiguration
	// Nutanix configures machine information for the Nutanix platform.
	// +kubebuilder:validation:Optional
	Nutanix NutanixMachineConfiguration
	// Openstack configures machine information for the Openstack platform.
	// +kubebuilder:validation:Optional
	Openstack OpenstackMachineConfiguration
	// Vsphere configures machine information for the Vsphere platform.
	// +kubebuilder:validation:Optional
	Vsphere VsphereMachineConfiguration
}

type AWSMachineConfiguration struct {
	// Weight specifies the priority ordering for using this configuration. This
	// is selected in increasing order for scaling up, or decreasing order for
	// scaling down.
	// +kubebuilder:validation:Required
	Weight uint
	// InstanceSize specifies the AWS instance size to use for this size of
	// machine.
	// +kubebuilder:validation:Required
	InstanceSize string
}

type AzureMachineConfiguration struct {
	// Weight specifies the priority ordering for using this configuration. This
	// is selected in increasing order for scaling up, or decreasing order for
	// scaling down.
	// +kubebuilder:validation:Required
	Weight uint
	// VmSize specifies the Azure instance size to use for this size of
	// machine.
	// +kubebuilder:validation:Required
	VmSize string
}

type GCPMachineConfiguration struct {
	// Weight specifies the priority ordering for using this configuration. This
	// is selected in increasing order for scaling up, or decreasing order for
	// scaling down.
	// +kubebuilder:validation:Required
	Weight uint
	// MachineType specifies the GCP instance size to use for this size of
	// machine.
	// +kubebuilder:validation:Required
	MachineType size
}

type VsphereMachineConfiguration struct {
	// Weight specifies the priority ordering for using this configuration. This
	// is selected in increasing order for scaling up, or decreasing order for
	// scaling down.
	// +kubebuilder:validation:Required
	Weight uint
	// NumCPUs specifies the amount of CPUs to use for this instance size.
	// +kubebuilder:validation:Required
	NumCPUs uint
	// MemoryMiB specifies the amount of memory to use for this instance size.
	// +kubebuilder:validation:Required
	MemoryMiB uint
}

type OpenstackMachineConfiguration struct {
	// Weight specifies the priority ordering for using this configuration. This
	// is selected in increasing order for scaling up, or decreasing order for
	// scaling down.
	// +kubebuilder:validation:Required
	Weight uint
	// Flavor specifies the flavor to use for this instance size.
	// +kubebuilder:validation:Required
	Flavor size
}

type NutanixMachineConfiguration struct {
	// Weight specifies the priority ordering for using this configuration. This
	// is selected in increasing order for scaling up, or decreasing order for
	// scaling down.
	// +kubebuilder:validation:Required
	Weight uint
	// MemorySize specifies the amount of memory to use for this instance size.
	// +kubebuilder:validation:Required
	MemorySize string
	// VcpuSockets specifies the amount of CPU sockets to use for this instance
	// size.
	// +kubebuilder:validation:Required
	VcpuSockets uint
	// VcpusPerSockets specifies the amount of virtual CPU per socket to use for
	// this instance size.
	// +kubebuilder:validation:Required
	VcpusPerSocket uint
}

type ScalingDefinition struct {
	// ScaleUpDelay let's the user define the amount of time that must pass
	// between a previous scale event and a potential new scale event. This
	// allows the user to specify a custom amount of time to allow operators to
	// settle before scaling again.
	//
	// When omitted, this means the user has no opinion and the value is left to
	// the platform to choose a good default, which is subject to change over
	// time. The current default is 120m.
	//
	// +kubebuilder:validation:Required
	ScaleUpDelay  Duration
	// SelectPolicy let's the user define the selection strategy when choosing
	// the next machine to scale up to. Right now the only supported value is
	// 'next', which will choose the next highest weight available.
	// 
	// +kubebuilder:validation:Optional
	SelectPolicy  string
	// TriggerPolicy let's the user define how many triggers must be above or
	// below their threshold, before a scale up or scale down will be triggered.
	//
	// Possible values are 'all' or 'any'.
	// 
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Enum:="all","any"
	TriggerPolicy string
	// Triggers contains the definition for triggers that will be checked to
	// decide if scaling is required or not.
	// 
	// If the triggers are empty, the type of scaling is disabled.
	// 
	// +kubebuilder:validation:Optional
	Triggers      []Trigger
}

type Trigger struct {
	// TriggerType is the union discriminator. Users are expected to set this
	// value to the type of trigger they want to use. Supported values are
	// 'cpu', 'memory', 'prometheus'.
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum:="cpu","memory","prometheus"
	TriggerType       string
	// Prometheus allows the user to specify the required fields to check a
	// metric using prometheus.
	// 
	// +kubebuilder:validation:Optional
	Prometheus        PrometheusTrigger
	// Cpu allows the user to specify the required fields to check a
	// cpu-usage based metric using the metrics API.
	// 
	// +kubebuilder:validation:Optional
	Cpu               MetricsTrigger
	// Memory allows the user to specify the required fields to check a
	// memory-usage based metric using the metrics API.
	// 
	// +kubebuilder:validation:Optional
	Memory            MetricsTrigger
	// AuthenticationRef allows the user to specify a secret that contains
	// authentication information to connect to prometheus.
	// In case of CPU and Memory triggers this value is ignored.
	//
	// +kubebuilder:validation:Optional 
	AuthenticationRef string
}

type PrometheusTrigger struct {
	// ServerAddress allows the user to specify the address of Prometheus
	// server.
	//
	// kubebuilder:validation:Required
	ServerAddress    string
	// Query allows the user to specify the query for the Prometheus metric to
	// use.
	//
	// kubebuilder:validation:Required
	Query            string
	// Threshold allows the user to specify the threshold for the Prometheus
	// metric to use.
	// 
	// In case of scale up, this is the lower bound - so if current usage is
	// higher it will trigger scaling.
	// 
	// In case of scale down, this is the upper bound - so if current usage is
	// lower it will trigger scaling.
	//
	// kubebuilder:validation:Required
	Threshold        float
	// AuthMode allows the user to specify what authentication mode to use to
	// connect to the Prometheus server.
	//
	// Supported values are "Basic", "Bearer" and "Tls".
	// 
	// kubebuilder:validation:Required
	// kubebuilder:validation:Enum:"Basic","Bearer","Tls"
	AuthMode         string
	// UnsafeSsl allows the user to specify skipping the certificate check.
	//
	// The default value is "false".
	// 
	// kubebuilder:validation:Optional
	UnsafeSsl        bool
}

type MetricsTrigger struct {
	// Value allows the user to specify the usage % of the metrics API based
	// metric that is used to make a scaling decision.
	//
	// In case of scale up, this is the lower bound - so if current usage is
	// higher it will trigger scaling.
	// 
	// In case of scale down, this is the upper bound - so if current usage is
	// lower it will trigger scaling.
	//
	// kubebuilder:validation:Required
	Value      string
	// TimeWindow alles the user to specify the amount of time to take into
	// account, when calculating the usage-average that will be compared to the
	// specified Value.
	// 
	// kubebuilder:validation:Required
	TimeWindow Duration
}
```

### Implementation Details/Notes/Constraints [optional]

The implementation for this feature, requires a data source to gather required
data that is used to make scaling decisions.

As every OpenShift cluster comes with the [metrics
API](https://kubernetes.io/docs/tasks/debug/debug-cluster/resource-metrics-pipeline/)
as well as
[prometheus](https://docs.openshift.com/container-platform/4.13/monitoring/monitoring-overview.html)
installed, both should be available as possible data sources.

Both should be usable simultaneously or in isolation - should prometheus not be
available and not configured this should not pose a problem and the metrics
based datasource should continue functioning and vice versa.

#### Using metrics API

Current CPU and memory usage for nodes, as well as available resources on the
nodes are gathered using the metrics API.

To gather available resource information in this case, the default
[Node](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.27/#node-v1-core)
API endpoint can be used.

Using the [horizontal pod
autoscaler](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/)
as a reference, they are using the to gather performance statistics for nodes.

Using the metrics API has the least requirements as it does not expect a running
Prometheus monitoring stack. 

On the other hand, data aggregation will be required using the metrics API, as
decisions about scaling nodes up and down, must not be performed based on
performance usage at a single point in time. Instead the operator will have to
gather and store usage data for a certain period of time, to decide if scaling
is required.

As an example with a configured `timeWindow` (see
[configuration](#control-plane-machineset-operator-configuration)) of 30 minutes
for scaling up, the operator has to keep querying the metrics API and averaging
values until enough data has been aggregated to know the load average for the
last 30 minutes.

If the metrics API returns it's own window of ~60 second aggregations, the
operators will have to retrieve 30 values (one per minute), before any scaling
decision should be made.

#### Using Prometheus

Using Prometheus generated data will make the implementation easier with respect
to data aggregation, as data should already have been aggregated by prometheus.

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

Initially this value should be rather conservative like 60 minutes or even more,
to give plenty of time for all operators to settle on the new nodes. The
operator can log information if the configured value seems too low, but should
still continue running.

### Drawbacks

Some users might not want this feature and rather opt to scale control planes
manually.

## Design Details

### Control Plane MachineSet Scaling operator configuration

As automatic scaling will require adjustments to certain aspects of its logic,
the implementation should implement a way for administrators of an OpenShift
cluster to configure the specifics of how and when scaling will occur.

This configuration will be performed using a new custom resource
`ControlPlaneMachineSetAutoscaler`. This resource should be a singleton, as it
should not be possible to run multiple instances of this operator at the same
time to prevent race conditions during resizing.

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
  - `scaleUpDelay`: duration to wait after a scale up before another scaling
    operation can occur.
  - `selectPolicy`: `next` to select the next bigger instance size.
  - `triggerPolicy`: `all` or `any`. Determines if scaling occurs if one or
    all triggers have to be true.
  - `triggers`: sets up the list of triggers, which will trigger a scale up. The
    configuration is based on [custom metrics autoscaling
    configuration](https://docs.openshift.com/container-platform/4.13/nodes/cma/nodes-cma-autoscaling-custom-trigger.html)
    and should be kept compatbile to make usage easier for users accustomed to
    CMA. Specifying multiple triggers, the `triggerPolicy` will determine when a
    scaleup is performed.
    An empty list of triggers disables scale up.
    - `type`: `prometheus`, `cpu` or `memory`.
    - `Prometheus`:
      - `serverAddress`: https://thanos-querier.openshift-monitoring.svc.cluster.local:9092 
      - `query`: Specifies the Prometheus query to use.
      - `threshold`: Specifies the value that triggers scaling. 
      - `authModes`: Specifies the authentication method to use. Should support
        at least *basic*, *bearer* and *tls* authentication.
      - `ignoreNullValues`: Specifies how the trigger should proceed if the Prometheus target is lost.
        - If true, the trigger continues to operate if the Prometheus target is
          lost. This is the default behavior.
        - If false, the trigger returns an error if the Prometheus target is
          lost - this will degrade the operator.
      - `unsafeSsl`: Specifies whether the certificate check should be skipped.
    - `cpu`:
      - `value`: Load average value that determines if scaling has to occur.
      - `timeWindow`: Duration over which the load average must be higher to
        trigger a scale up.
    - `memory`:
      - `value`: Load average value that determines if scaling has to occur.
      - `timeWindow`: Duration over which the load average must be higher to
        trigger a scale up.
    - `authenticationRef`:
      - `name`: Name of the
        `ControlPlaneMachineSetAutoscalingTriggerAuthentication` in the same
        namespace.
- `scaleDown`: defines configuration for scaling up. This includes the following
  sub elements:
  - `scaleDownDelay`: duration to wait after a scale down before another scaling
    operation can occur.
  - `selectPolicy`: `next` to select the next smaller instance size.
  - `triggerPolicy`: `all` or `any`. Determines if scaling occurs if one or
    all triggers have to be true.
  - `triggers`: sets up the list of triggers, which will trigger a scale down.
    For details see the `scaleUp` configuration.
    An empty list of triggers disables scale down.

A complete custom resources instance in YAML format could then look like this:

```yaml
controlplaneautoscaler:
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
    selectPolicy: "next"
    stabilizationWindow: [duration as specified in golang: e.g. 5s, 15s]
    triggerPolicy: "any"
    triggers:
      - type: "cpu"
        cpu:
          value: "80"
          timeWindow: "30m"
  scaleDown:
    selectPolicy: "next"
    stabilizationWindow: [duration as specified in golang: e.g. 5s, 15s]
    triggerPolicy: "all"
    triggers:
      - type: "cpu"
        cpu:
          value: "80"
          timeWindow: "30m"
      - type: "memory"
        memory:
          value: "90"
          timeWindow: "30m"
```

In case the user decides to use `prometheus`, authentication will be specified
in an additional custom resource
`ControlPlaneMachineSetAutoscalingTriggerAuthentication` that is based on
[CMA](https://docs.openshift.com/container-platform/4.13/nodes/cma/nodes-cma-autoscaling-custom-trigger.html#nodes-cma-autoscaling-custom-prometheus-config_nodes-cma-autoscaling-custom-trigger
) as well and specifies the following values:

- `secretTargetRef`:
  - `parameter`: type of the secret referenced: should be `bearer` for bearer
    authentication.
  - `name`: name of the secret too use.
  - `key`: key in the secret that contains the token.

### Data gathering

As specified the operator should be able to use two different data sources:
metrics API and prometheus.

#### Metrics API

The operator can gather the following data via the [metrics
API](https://kubernetes.io/docs/tasks/debug/debug-cluster/resource-metrics-pipeline/):

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

#### Prometheus

Prometheus queries should be configured to use the correct window already.

The operator has to handle a metric's data missing or authentication failure.

Both cases should use a conservative approach of not performing any actions, and
not interrupting cluster operation. However the operator must be marked as
degraded in case of authentication failure and disappearing of a metric.

### Workflow for automatic scaling

Automatic scaling should be triggered, when either of the triggers specified
matches it's conditions.

Checking for the conditions should use the same approach as [horizontal pod
autoscaling](https://kubernetes.io/docs/tasks/run-application/horizontal-pod-autoscale/):
HPA runs periodic checks, to see if scaling is required.

In addition the implementation has to ensure that scaling decision made based on
the metrics API are not based on single point-in-time values to prevent scaling
up or down based on usage spikes.

Instead a running average (or similar) should be used to decide about scaling.

If for any reason metrics can not be gathered, the operator must remain
available and only the autoscaling should be disabled / non-functional.

### Additional RBAC permissions

The operator will require access to the following resources to gather the data:

- Write access to the `ControlPlaneMachineSet` CR.
- Read access for `/apis/metrics.k8s.io/v1beta1/nodes`.
- Read access for `/api/v1/nodes` (this is already in place for OSD OpenShift
  clusters, but has to be verified for a OCP OpenShift installations).

### Open Questions

1. CMA even allow scaling to be triggered via Kafka - should this be supported
   as well?

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
