---
title: cluster-autoscaler-integration
authors:
  - "@JoelSpeed"
reviewers:
  - "@enxebre"
  - "@elmiko"
  - "@derekwaynecarr"
  - "@mhrivnak"
approvers:
- "@enxebre"
- "@elmiko"
creation-date: 2020-06-01
last-updated: 2020-06-01
status: implemented
see-also:
  - "/enhancements/machine-api/cluster-autoscaler-operator.md"  
replaces:
superseded-by:
---

# Cluster Autoscaler Integration

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The [Cluster Autoscaler](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler) is a tool that users
can deploy to a cluster to enable the automatic provisioning of extra resources when workloads exceed the cluster's
capacity.
It can also be configured to scale down resources if they are surplus to current requirement.

In normal operation, a user would configure the Cluster Autoscaler to talk directly to their cloud provider and create
new instances by expanding a nodegroup, such as an Autoscaling Group on AWS EC2.
Because OpenShift uses a cloud provider agnostic machine API for compute management, OpenShift clusters do not have
nodegroups in the way that the Cluster Autoscaler expects.

Across the broader Kubernetes ecosystem, it has been identified that having an infrastructure agnostic cluster
autoscaler node group provider is a recurring challenge.
This is evidenced by the OpenShift Machine API, SAP Gardener, and Kubernetes-SIGS [Cluster API](https://github.com/kubernetes-sigs/cluster-api) efforts.
The only mechanism today for using the existing Cluster Autoscaler requires clients to either use the built-in
infrastructure providers, or fork/extend the mainline repo (as many cloud providers do for their commercial integrations)
to introduce their own extension.
This proposal tracks the work that was done to reduce the cost of fork/extend style extension in a downstream implementation.

## Motivation

### Goals

- Enable automatic expansion of OpenShift cluster resources when capacity is exceeded
- Enable automatic contraction of OpenShift cluster resources when capacity is under-utilised
- Provide a cloud agnostic autoscaler integration that will work with different machine API scalable resources, eg. MachineSet
- Mitigate the need to fork/extend by supporting a call-out mechanism and taking that upstream

### Non-Goals

- Install the Cluster Autoscaler on OpenShift clusters, this will be handled by the Cluster Autoscaler Operator
- Add support for MachineDeployments to OpenShift, though the Cluster Autoscaler will support them

## Proposal

Create a new Cloud Provider integration within the cluster-autoscaler called `clusterapi`.

This integration will be implemented in the form of a new Cloud Provider within the Cluster Autoscaler.
The Cloud Provider will use MachineSets and MachineDeployments that have opted in to autoscaling to provide the
Cluster Autoscaler with information about the resources within a cluster and the resources available for scaling.

MachineSets and MachineDeployments will be opted-in to autoscaling if they are annotated with two annotations, which
denote the maximum and minimum sizes that the autoscaler should scale the Node group within, eg:
```yaml
metadata:
  annotations:
    - "machine.openshift.io/cluster-api-autoscaler-node-group-max-size": "6"
    - "machine.openshift.io/cluster-api-autoscaler-node-group-min-size": "1"
```

### User Stories

#### Story 1

As a user of OpenShift, I would like to be able to autoscale workloads on my cluster as required by my users, and have
the cluster automatically expand to meet the new capacity requirements

#### Story 2

As a user of OpenShift, if the workloads on my cluster reduce leaving unused capacity, I would like my cluster to
automatically scale back to reduce cost and wasted resources.

#### Story 3

As a developer of OpenShift, I would like an agnostic integration that specifies a minimal API contract for autoscaling
so that it is compatible with multiple versions and implementations of the machine API simultaneously to reduce the
burden of fork/extend style development.

### Implementation Details

This integration will satisfy the [CloudProvider](https://github.com/kubernetes/autoscaler/blob/a00bf59159bda6d70e86741764802d11e7d253f4/cluster-autoscaler/cloudprovider/cloud_provider.go#L48-L91)
interface by mapping the [NodeGroup](https://github.com/kubernetes/autoscaler/blob/a00bf59159bda6d70e86741764802d11e7d253f4/cluster-autoscaler/cloudprovider/cloud_provider.go#L103-L170)
concept to MachineSets and MachineDeployments that opt-in to autoscaling.

#### Opting in to Autoscaling

In order for a MachineSet or MachineDeployment to be considered for autoscaling, the autoscaler integration will need
autoscaling bounds for the NodeGroup, ie. it will need to know the maximum and minimum size that the MachineSet/
MachineDeployment should be.

This information will be provided to the Cluster Autoscaler by way of annotations on the MachineSet/MachineDeployment, eg:
```yaml
metadata:
  annotations:
    - "machine.openshift.io/cluster-api-autoscaler-node-group-max-size": "6"
    - "machine.openshift.io/cluster-api-autoscaler-node-group-min-size": "1"
```

#### Provider

The Provider will implement the [CloudProvider](https://github.com/kubernetes/autoscaler/blob/a00bf59159bda6d70e86741764802d11e7d253f4/cluster-autoscaler/cloudprovider/cloud_provider.go#L48-L91)
interface that is required to integrate Machine API with the cluster autoscaler.

It will essentially wrap the Machine Controller which is responsible for maintaining the state of the cluster from an
autoscaling perspective.

#### Machine Controller

The Machine Controller will be responsible for coordinating the underlying MachineSets and MachineDeployments to provide
a uniform interface to the core of the Cluster Autoscaler. It will be responsible for:
- Maintaining a cache of the state of the target cluster resources (Machines, MachineSets, MachienDeployments, Nodes)
- Providing a list of "Cloud Provider" instances to the core autoscaler logic
- Building and providing NodeGroups to the core autoscaler based on the state of the cluster
- Matching Cloud Provider IDs to Nodes

#### NodeGroups

Two forms of NodeGroup will be implemented in this integration, one will be backed by MachineSet objects, the other
will be backed by MachineDeployment objects.

Since MachineDeployments do not exist in OpenShift's Machine API, they will be able to be disabled for clusters that
do not support them.

Each NodeGroup will implement the [NodeGroup](https://github.com/kubernetes/autoscaler/blob/a00bf59159bda6d70e86741764802d11e7d253f4/cluster-autoscaler/cloudprovider/cloud_provider.go#L103-L170)
interface from Cluster Autoscaler.

We will not be supporting autoprovisioning of NodeGroups, so we will not implement the optional `Create` or `Delete` methods.
However we will enable scaling from zero, for which we will need to implement the `TemplateNodeInfo` method which will
provide information for what a Node would look like from a NodeGroup which does not have any Nodes in the cluster presently.

Since there is a lot of common functionality between the two groups, we can abstract the specific implementation details
into a `scalableResource` interface and keep a single `NodeGroup` implementation with an underlying `ScalableResource`.

```Go
type nodegroup struct {
  machineController *machineController
  scalableResource scalableResource
}

func (ng *nodegroup) Name() string {
	return ng.scalableResource.Name()
}

func (ng *nodegroup) Namespace() string {
	return ng.scalableResource.Namespace()
}

func (ng *nodegroup) MinSize() int {
	return ng.scalableResource.MinSize()
}

func (ng *nodegroup) MaxSize() int {
	return ng.scalableResource.MaxSize()
}

func (ng *nodegroup) TargetSize() (int, error) {
	...
}

func (ng *nodegroup) IncreaseSize(delta int) error {
	...
}

func (ng *nodegroup) DeleteNodes(nodes []*corev1.Node) error {
  // Mark each machine for deletion, then scale down the scalableResource by the required number of nodes
	...
}

func (ng *nodegroup) DecreaseTargetSize(delta int) error {
	...
}

func (ng *nodegroup) Id() string {
	return ng.scalableResource.ID()
}

func (ng *nodegroup) Debug() string {
  ...
}

func (ng *nodegroup) Nodes() ([]cloudprovider.Instance, error) {
  // Gather a list of "cloud provider" instances, effectively a list of ProviderIDs from Machines
  ...
}

func (ng *nodegroup) TemplateNodeInfo() (*schedulernodeinfo.NodeInfo, error) {
  // Construct a NodeInfo that looks like what a Node from this NodeGroup would look like
  // Eg capacity, labels and taints
  ...
}

func (ng *nodegroup) Exist() bool {
  // Not supporting dynamic NodeGroup provisioning
	return true
}

func (ng *nodegroup) Create() (cloudprovider.NodeGroup, error) {
  // Not supporting dynamic NodeGroup provisioning
	return nil, cloudprovider.ErrAlreadyExist
}

func (ng *nodegroup) Delete() error {
  // Not supporting dynamic NodeGroup provisioning
	return cloudprovider.ErrNotImplemented
}

func (ng *nodegroup) Autoprovisioned() bool {
  // Not supporting dynamic NodeGroup provisioning
	return false
}

```

#### Scalable Resource

A `scalableResource` will be the lowest level interface that will represent either a MachineSet or MachineDeployment.
It will be used by the `nodegroup` to retrieve information about, or manipulate the underlying MachineSet/MachineDeployment.

```Go
// scalableResource is a resource that can be scaled up and down by
// adjusting its replica count field.
type scalableResource interface {
	// Id returns an unique identifier of the resource
	ID() string

	// MaxSize returns maximum size of the resource
	MaxSize() int

	// MinSize returns minimum size of the resource
	MinSize() int

	// Name returns the name of the resource
	Name() string

	// Namespace returns the namespace the resource is in
	Namespace() string

	// Nodes returns a list of all machines that already have or should become nodes that belong to this
	// resource
	Nodes() ([]string, error)

	// SetSize() sets the replica count of the resource
	SetSize(nreplicas int32) error

	// Replicas returns the current replica count of the resource
	Replicas() (int32, error)

	// MarkMachineForDeletion marks machine for deletion
	MarkMachineForDeletion(machine *Machine) error

	// UnmarkMachineForDeletion unmarks machine for deletion
	UnmarkMachineForDeletion(machine *Machine) error

  // The following methods will be used to create a template Node for scaling from Zero
	Labels() map[string]string
	Taints() []apiv1.Taint
	CanScaleFromZero() bool
	InstanceCPUCapacity() (resource.Quantity, error)
	InstanceMemoryCapacity() (resource.Quantity, error)
	InstanceGPUCapacity() (resource.Quantity, error)
	InstanceMaxPodsCapacity() (resource.Quantity, error)
}
```

##### Marking Machines for deletion

To ensure that the MachineSet or MachineDeployment scales down the desired Node, when asked to scale down,
the `scalableResource` will apply `"machine.openshift.io/cluster-api-delete-machine"` as a label to the Machine that
has been flagged for termination. It is expected that the MachineSet/MachineDeployment controller should terminate
this instance before any other when it is scaled down.

#### Agnostic API

For the provider to support multiple machine API implementations e.g machine.openshift.io, SAP Gardener, cluster.x-k8s.io,
we will implement an internal API which is expected to be a minimal subset of existing machine API versions.
By implementing this way, we will reduce the maintenance burden when we make changes to the Machine API, and,
once contributed to the upstream autoscaler project, we will also reduce the effort required when rebasing the
OpenShift fork.

Assuming an implementation of the machine API supports this minimal internal API, it will be supported by the Cluster
Autoscaler integration.

##### Minimal Machine API

The following structure definition outlines the minimal `Machine` object that will be required for the autoscaler to
perform scaling decisions:

```Go
// Machine is the Schema for the machines API
type Machine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSpec   `json:"spec,omitempty"`
	Status MachineStatus `json:"status,omitempty"`
}

// MachineSpec defines the desired state of Machine
type MachineSpec struct {
  // ObjectMeta is required for Labels which are copied onto the Node.
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Taints is the full, authoritative list of taints to apply to the corresponding Node.
	Taints []corev1.Taint `json:"taints,omitempty"`

	// ProviderID is the identification ID of the machine provided by the provider.
	ProviderID *string `json:"providerID,omitempty"`
}

// MachineStatus defines the observed state of Machine
type MachineStatus struct {
	// NodeRef will point to the corresponding Node if it exists.
	NodeRef *corev1.ObjectReference `json:"nodeRef,omitempty"`

	// ErrorMessage will be set in the event that there is a terminal problem reconciling the Machine.
	ErrorMessage *string `json:"errorMessage,omitempty"`
}
```

##### Minimal MachineSet API

The following structure definition outlines the minimal `MachineSet` object that will be required for the autoscaler to
perform scaling decisions:

```Go
// MachineSet is the internal autoscaler Schema for machineSets
type MachineSet struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineSetSpec   `json:"spec,omitempty"`
	Status MachineSetStatus `json:"status,omitempty"`
}

// MachineSetSpec is the internal autoscaler Schema for MachineSetSpec
type MachineSetSpec struct {
	// Replicas is the number of desired replicas.
	Replicas *int32 `json:"replicas,omitempty"`

	// MinReadySeconds is the minimum number of seconds for which a newly created machine should be ready.
	MinReadySeconds int32 `json:"minReadySeconds,omitempty"`

	// Selector is a label query over machines that should match the replica count.
	Selector metav1.LabelSelector `json:"selector"`

	// Template is the object that describes the machine that will be created if insufficient replicas are detected.
	Template MachineTemplateSpec `json:"template,omitempty"`
}

// MachineTemplateSpec is the internal autoscaler Schema for MachineTemplateSpec
type MachineTemplateSpec struct {
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// Specification of the desired behavior of the machine.
	Spec MachineSpec `json:"spec,omitempty"`
}

// MachineSetStatus is the internal autoscaler Schema for MachineSetStatus
type MachineSetStatus struct {
	// Replicas is the most recently observed number of replicas.
	Replicas int32 `json:"replicas"`
}
```


##### Minimal MachineDeployment API

The following structure definition outlines the minimal `MachineDeployment` object that will be required for the autoscaler to
perform scaling decisions:

```Go
// MachineDeployment is the internal autoscaler Schema for MachineDeployment
type MachineDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineDeploymentSpec   `json:"spec,omitempty"`
	Status MachineDeploymentStatus `json:"status,omitempty"`
}

// MachineDeploymentSpec is the internal autoscaler Schema for MachineDeploymentSpec
type MachineDeploymentSpec struct {
	// Number of desired machines. Defaults to 1.
	Replicas *int32 `json:"replicas,omitempty"`

	// Label selector for machines.
	Selector metav1.LabelSelector `json:"selector"`

	// Template describes the machines that will be created.
  // Defined within the MachineSet API.
	Template MachineTemplateSpec `json:"template"`
}

// MachineDeploymentStatus is the internal autoscaler Schema for MachineDeploymentStatus
type MachineDeploymentStatus struct {
	// Total number of non-terminated machines targeted by this deployment (their labels match the selector).
	Replicas int32 `json:"replicas,omitempty"`
}
```

### Risks and Mitigations

## Design Details

### Test Plan

E2E testing will be added to the [cluster-api-actuator-pkg](https://github.com/openshift/cluster-api-actuator-pkg/)
to provide basic smoke testings of scale up and scale down operations using the new provider.

### Graduation Criteria

#### Examples

##### Dev Preview -> Tech Preview

##### Tech Preview -> GA

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

This is a new feature so upgrading will not be affected as autoscaling will be disabled by default.
Downgrading once enabled may leave unused annotations on MachineSets/MachineDeployments but will have no adverse effect
on clusters.

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed

- An OpenShift fork of the [kubernetes/autoscaler](https://github.com/kubernetes/autoscaler) respository.
