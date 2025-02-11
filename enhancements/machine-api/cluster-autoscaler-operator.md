---
title: cluster-autoscaler-operator
authors:
  - "@JoelSpeed"
reviewers:
  - "@enxebre"
  - "@elmiko"
  - "@wking"
  - "@jeremyeder"
approvers:
  - "@enxebre"
creation-date: 2020-05-19
last-updated: 2023-02-03
status: implemented
see-also:
replaces:
superseded-by:
---

# Cluster Autoscaler Operator

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Provide a declarative way for users to enable autoscaling of compute resources by providing the Cluster Autoscaler
as a Deployment with opinionated default options and a gated set of flags.

Provide a way for users to opt individual MachineSets into autoscaling and to set boundaries for the size of the
MachineSets that they wish to autoscale.

## Motivation

### Goals

- Enable users to create Cluster Autoscaler with opinionated defaults
- Remove operational burden for users to allow autoscaling to be up and running quickly
- Enable users to autoscale MachineSets
- Enable users to set a maximum size of their cluster
- Enable users to set maximum and minimum sizes for each of their MachineSets

### Non-Goals

- Integrate the Cluster Autoscaler with Machine API (This will be a separate proposal)
- Enable autoscaling by default for new OpenShift clusters
- Provide options for autoscaling across multiple MachineSets

## Proposal

A new controller will be developed to manage the lifecycle of the Cluster Autoscaler application and to manage
assigning size limits to MachineSets to enable the Cluster Autoscaler to autoscale desired MachineSets.

OpenShift cluster administrators will be able to specify configuration for the Cluster Autoscaler using a `ClusterAutoscaler`
custom resource. This custom resource provides the user with the options to configure cluster wide resource limits,
eg. a max number of CPUs over all instances, and other Cluster Autoscaler parameters such as the configuration for
node draining, eg the Pod graceful termination period.

To opt particular `MachineSets` into autoscaling, administrators will create `MachineAutoscaler` custom resources.
These resources will reference and provide scaling limits for a single `MachineSet`.
The Cluster Autoscaler Operator will use these `MachineAutoscalers` to configure the Cluster Autoscaler to autoscale
the referenced `MachineSet`.

### User Stories

#### Story 1

As a user of OpenShift, it should be easy to enable autoscaling of compute resources so that my cluster will respond to
increased usage without manual intervention.

#### Story 2

As a user of OpenShift, I should be able to specify which groups of Machines should be autoscaled and which should not.

#### Story 3

As a user of OpenShift, I should be able to limit the size of my cluster when it is autoscaling so that I do not overload
the Control Plane and so that I can cap the costs of my cluster.

#### Story 4

As a user of OpenShift, I should be able to specify a maximum and minimum size for any group of Machines being scaled
so that I can ensure I have a minimum capacity available and that I do not over commit to a single pool of Machines.

### Implementation Details

The controller will consist of two control loops, one for a new `ClusterAutoscaler` CRD, and the second for a new
`MachineAutoscaler` CRD.

This controller will be deployed and managed by the Cluster Version Operator and will become a second level operator.

#### ClusterAutoscaler controller

The `ClusterAutoscaler` controller is responsible for managing Cluster Autoscaler Deployments in the `openshift-machine-api` namespace.
It will also be responsible for ensuring that the relevant resources for providing monitoring information will also be deployed.

If a `ClusterAutoscaler` resource exists, the controller will perform the following logic:
- Fetch the `ClusterAutoscaler` custom resource
- Validate the options specified on the `ClusterAutoscaler` specification
  - Should the validation fail, log an error and requeue the custom resource
- Check for a Deployment for the `ClusterAutoscaler` custom resource
  - A Deployment named `cluster-autoscaler-<name>` would be expected, where `<name>` is the name of the `ClusterAutoscaler` resource
- Ensure monitoring resources are set up for the Cluster Autoscaler deployment
  - This includes a `Service`, `ServiceMonitor` and `PrometheusRule`
- If the Deployment does not exist, create it
- If the Deployment exists, ensure the Pod specification matches the desired configuration from the `ClusterAutoscaler` resource

#### ClusterAutoscaler Admission Webhook

The `ClusterAutoscaler` admission webhook will be responsible for validating the configuration of `ClusterAutoscaler`
resources as they are created or updated.

It will perform the following validation:
- Check the `ClusterAutoscaler` name
  - Only one `ClusterAutoscaler` will be allowed per cluster to prevent multiple Cluster Autoscaler deployments from
    conflicting with each other
  - By default, the only allowed name will be `default`
- Check that the resource limits are valid
  - Eg maximum Node count limit is greater than 0
- Check that the scale down options are valid
  - Eg the duration strings are correctly formatted
- Check that the string values for GPU resource limit types are valid
  - These must follow the [Kubernetes rules for valid label values][labels-syntax] as they will be
    used as label values to identify Nodes with these resources by the Cluster Autoscaler.
  - This validation will only produce a warning for users so as not to break previously valid ClusterAutoscaler objects.
    (this validation is being added after the initial release of the operator)

#### MachineAutoscaler controller

The `MachineAutoscaler` controller is responsible for managing annotations on MachineSets based on the desired state
captured in `MachineAutoscaler` custom resources.

If a `MachineAutoscaler` resource exists,  the controller will perform the following logic:
- Fetch the `MachineAutoscaler` custom resource
- If the status contains a previous target reference, check this matches the current target reference
  - If the target has changed, perform clean up logic to remove annotations and owner references from previous target
- Validate the options specified on the `MachineAutoscaler` specification
  - Should the validation fail, log and error and request the custom resource
- Fetch the `MachineSet` specified in the target reference
- Ensure the `MachineSet` has an owner reference pointing to the `MachineAutoscaler` resource
- Ensure the `MachineAutoscaler` has a `finalizer` to allow it to clean up owner references and annotations should it be deleted
- Ensure the Cluster Autoscaler annotations are set and match the current desired state of the `MachineAutoscaler` resource
  - If adding the annotations, ensure that the `MachineSet` has set a GPU accelerator label in its `.spec.template.spec.metadata.labels`,
    if not a warning event will be emitted to describe potential issues that can occur without this label, and a link to a related KCS
    article providing more details.

#### MachineAutoscaler Admission Webhook

The `MachineAutoscaler` admission webhook will be responsible for validating the configuration of the `MachineAutoscaler`
resources as they are created or updated.

It will perform the following validation:
- Check that the maximum and minimum replica counts are non-negative
- Check the the maximum replica count is greater than the minimum replica count

#### Telemetry

The Cluster Autoscaler Operator will be responsible for configuring telemetry for Cluster Autoscaler's that is has created.
It will manage a `Service`, `ServiceMonitor` and `PrometheusRule` for each Cluster Autoscaler Deployment.

The following alerts will be provided by default:

```yaml
- alert: ClusterAutoscalerUnschedulablePods
  expr:  "cluster_autoscaler_unschedulable_pods_count{service=\"<service-name>\"} > 0"
  for:   20m
  labels:
    severity: warning
  annotations:
    message: "Cluster Autoscaler has {{ $value }} unschedulable pods"
- alert: ClusterAutoscalerNotSafeToScale
  expr: "cluster_autoscaler_cluster_safe_to_autoscale{service=\"<service-name>\"} != 1"
  for: 15m
  labels:
    severity: warning
  annotations:
    message: "Cluster Autoscaler is reporting that the cluster is not ready for scaling"
```

#### API Changes

##### ClusterAutoscaler

A new CRD will be introduced called `ClusterAutoscaler`.
The specification of the `ClusterAutoscaler` CRD will provide users with options that allow them to configure the
options of the Cluster Autoscaler.

Notably this will be a subset of the full options limited to the options detailed in the specification below.
Omitted options will be set to sensible defaults (Eg log level).

```go
// ClusterAutoscalerSpec defines the desired state of ClusterAutoscaler
type ClusterAutoscalerSpec struct {
	// Constraints of autoscaling resources
	ResourceLimits *ResourceLimits `json:"resourceLimits,omitempty"`

	// Configuration of scale down operation
	ScaleDown *ScaleDownConfig `json:"scaleDown,omitempty"`

	// Gives pods graceful termination time before scaling down
	MaxPodGracePeriod *int32 `json:"maxPodGracePeriod,omitempty"`

	// To allow users to schedule "best-effort" pods, which shouldn't trigger
	// Cluster Autoscaler actions, but only run when there are spare resources available,
	// More info: https://github.com/kubernetes/autoscaler/blob/master/cluster-autoscaler/FAQ.md#how-does-cluster-autoscaler-work-with-pod-priority-and-preemption
	PodPriorityThreshold *int32 `json:"podPriorityThreshold,omitempty"`

	// BalanceSimilarNodeGroups enables/disables the
	// `--balance-similar-node-groups` cluster-autocaler feature.
	// This feature will automatically identify node groups with
	// the same instance type and the same set of labels and try
	// to keep the respective sizes of those node groups balanced.
	BalanceSimilarNodeGroups *bool `json:"balanceSimilarNodeGroups,omitempty"`

	// Enables/Disables `--ignore-daemonsets-utilization` CA feature flag.
  // Should CA ignore DaemonSet pods when calculating resource utilization for scaling down. false by default
	IgnoreDaemonsetsUtilization *bool `json:"ignoreDaemonsetsUtilization,omitempty"`

	// Enables/Disables `--skip-nodes-with-local-storage` CA feature flag.
  // If true cluster autoscaler will never delete nodes with pods with local storage,
  // e.g. EmptyDir or HostPath. true by default at autoscaler
	SkipNodesWithLocalStorage *bool `json:"skipNodesWithLocalStorage,omitempty"`
}

type ResourceLimits struct {
	// Maximum number of nodes in all node groups.
	// Cluster autoscaler will not grow the cluster beyond this number.
	MaxNodesTotal *int32 `json:"maxNodesTotal,omitempty"`

	// Minimum and maximum number of cores in cluster, in the format <min>:<max>.
	// Cluster autoscaler will not scale the cluster beyond these numbers.
	Cores *ResourceRange `json:"cores,omitempty"`

	// Minimum and maximum number of gigabytes of memory in cluster, in the format <min>:<max>.
	// Cluster autoscaler will not scale the cluster beyond these numbers.
	Memory *ResourceRange `json:"memory,omitempty"`

	// Minimum and maximum number of different GPUs in cluster, in the format <gpu_type>:<min>:<max>.
	// Cluster autoscaler will not scale the cluster beyond these numbers. Can be passed multiple times.
	GPUS []GPULimit `json:"gpus,omitempty"`
}

type GPULimit struct {
	Type string `json:"type"`

	Min int32 `json:"min"`
	Max int32 `json:"max"`
}

type ResourceRange struct {
	Min int32 `json:"min"`
	Max int32 `json:"max"`
}

type ScaleDownConfig struct {
	// Should CA scale down the cluster
	Enabled bool `json:"enabled"`

	// How long after scale up that scale down evaluation resumes
	DelayAfterAdd *string `json:"delayAfterAdd,omitempty"`

	// How long after node deletion that scale down evaluation resumes, defaults to scan-interval
	DelayAfterDelete *string `json:"delayAfterDelete,omitempty"`

	// How long after scale down failure that scale down evaluation resumes
	DelayAfterFailure *string `json:"delayAfterFailure,omitempty"`

	// How long a node should be unneeded before it is eligible for scale down
	UnneededTime *string `json:"unneededTime,omitempty"`
}
```

##### MachineAutoscaler

A new CRD will be introduced called `MachineAutoscaler`.
The specification of the `MachineAutoscaler` CRD will allow users to configure a MachineSet to be autoscaled by adding
an object reference pointing to the desired MachineSet.
It will also allow the user to provide maximum and minimum boundaries for the number of Machines that each MachineSet
should be allowed to contain. Note these boundaries are inclusive.

```go
// MachineAutoscalerSpec defines the desired state of MachineAutoscaler
type MachineAutoscalerSpec struct {
	// MinReplicas constrains the minimal number of replicas of a scalable resource
	MinReplicas int32 `json:"minReplicas"`

	// MaxReplicas constrains the maximal number of replicas of a scalable resource
	MaxReplicas int32 `json:"maxReplicas"`

	// ScaleTargetRef holds reference to a scalable resource
	ScaleTargetRef CrossVersionObjectReference `json:"scaleTargetRef"`
}

// CrossVersionObjectReference identifies another object by name, API version,
// and kind.
type CrossVersionObjectReference struct {
	// APIVersion defines the versioned schema of this representation of an
	// object. Servers should convert recognized schemas to the latest internal
	// value, and may reject unrecognized values. More info:
	// http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#resources
	APIVersion string `json:"apiVersion,omitempty"`

	// Kind is a string value representing the REST resource this object
	// represents. Servers may infer this from the endpoint the client submits
	// requests to. Cannot be updated. In CamelCase. More info:
	// http://releases.k8s.io/HEAD/docs/devel/api-conventions.md#types-kinds
	Kind string `json:"kind"`

	// Name specifies a name of an object, e.g. worker-us-east-1a.
	// Scalable resources are expected to exist under a single namespace.
	Name string `json:"name"`
}
```

The `MinReplicas` and `MaxReplicas` values will be copied onto the target `MachineSet` as annotations under
`"machine.openshift.io/cluster-api-autoscaler-node-group-min-size"` and
`"machine.openshift.io/cluster-api-autoscaler-node-group-max-size"` respectively.

### Risks and Mitigations

## Design Details

### Test Plan

### Graduation Criteria

#### Examples

##### Dev Preview -> Tech Preview

##### Tech Preview -> GA

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

### Don't provide an operator

In this case, users would need to manually configure a Deployment for the Cluster Autoscaler and will need to manually
mark MachineSets with the required annotations for the Cluster Autoscaler to be able to identify which MachineSets are
allowed to be autoscaled.

Without an operator, it would be up to the user to do the following:
- Deploy appropriate RBAC and create the required service account for the Cluster Autoscaler
- Deploy telemetry resources for monitoring the state of the Cluster Autoscaler
- Configure a Deployment for the Cluster Autoscaler so that it is compatible with OpenShift and the Machine API in particular
- Manage upgrades of the Cluster Autoscaler deployment and ensure that it still functions as expected

## Infrastructure Needed

This project will be hosted in the
[openshift/cluster-autoscaler-operator](https://github.com/openshift/cluster-autoscaler-operator) repository on GitHub.

[labels-syntax]: https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#syntax-and-character-set
