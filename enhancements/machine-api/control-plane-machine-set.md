---
title: control-plane-machine-set-controller
authors:
  - "@JoelSpeed"
reviewers:
  - "@jewzaam" - service delivery asks
  - "@elmiko" - cluster infrastructure review
  - "@enxebre" - authored previews works
  - "@jstuever" - installer review
  - "@staebler" - installer review
  - "@jeana-redhat" - product docs review
approvers:
  - "@sttts" - impacts on control plane availability
  - "@soltysh" - impacts on control plane availability
  - "@tkashem" - impacts on etcd
  - "@hasbro17" - impacts on etcd
  - "@sdodson" - impacts on cluster lifecycle
api-approvers:
  - "@deads2k"
creation-date: 2022-01-11
last-updated: 2022-02-07
tracking-link:
  - TBD
replaces:
  - "[/enhancements/machine-api/control-plane-machinesets.md](https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/control-plane-machinesets.md)"
  - "https://github.com/openshift/enhancements/pull/278"
  - "https://github.com/openshift/enhancements/pull/292"
---

# Control Plane Machine Set Controller

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

To enable automated management of vertical scaling and replacement of Control Plane Machines, this proposal introduces
a new resource and controller that will manage Control Plane Machine instances.

## Motivation

As OpenShift adoption increases and our managed services offerings become more popular, the manual effort required to
scale Control Plane Machines as clusters grow or shrink is becoming a significant drain on the SRE Platform team
managing OpenShift Dedicated (OSD), Red Hat OpenShift on AWS (ROSA), and Azure Red Hat OpenShift (ARO).

The procedure to resize a Control Plane today is lengthy and very involved. It takes a significant amount of time for
an OpenShift expert to perform. We also document this process for our end users, however due to the complexity of the
procedure, there is discomfort in recommending this procedure to end users who may not be as familiar with the product.

To ensure the long term usability of OpenShift, we must provide an automated way for users to scale their Control Plane
as and when their cluster needs additional capacity.

As HyperShift adoptions grows, HyperShift will solve the same issue by running hosted Control Planes within management
clusters. However, HyperShift is not a suitable product for all OpenShift requirements (for example the first management
cluster) and as such, we see that HA clusters will continue to be used and we must solve this problem to allow the
continued adoption of HA clusters.

### Goals

* Provide a "best practices" approach to declarative management of Control Plane Machines
* Allow users to "1-click" scale their Control Plane to large or smaller instances
* Allow the adoption of existing Control Planes into this new mechanism
* Provide a safe mechanism to perform the sensitive operation of scaling the Control Plane and provide adequate
  feedback to end users about progress of the operation
* Allow users to opt-out of Control Plane management should our mechanism not fit their needs
* Allow users to customise rolling update strategies based on the needs of their environments

### Non-Goals

* Allow horizontal scaling of the Control Plane (this may be required in the future, but today is considered
  unsupported by OpenShift)
* Management of the etcd cluster state (the etcd operator will handle this separately)
* Automatic adoption of existing clusters (we will provide instructions to allow users to opt-in for existing cluster)
* Management of anything that falls out of the scope of Machine API (e.g. management of load balancers in front of
  Control Plane Machine - only load balancer membership is managed by Machine API)

## Proposal

A new CRD `ControlPlaneMachineSet` will be introduced into OpenShift and a respective `control-plane-machine-set-operator`
will be introduced as a new second level operator within OpenShift to perform the operations described in
this proposal.

The new CRD will define the specification for managing (creating, adopting, updating) Machines for the Control Plane.
The operator will be responsible for ensuring the desired number of Control Plane Machines are present within the
cluster as well as providing update mechanisms akin to those seen in a StatefulSets and Deployments to allow rollouts
of updated configuration to the Control Plane Machines.

### User Stories

- As a cluster administrator of OpenShift, I would like to be able to safely and automatically vertically resize my
  control plane as and when the demand on the control plane changes
- As a cluster administrator of OpenShift, I would like to be able to automatically recover failed Control Plane
  Machines (eg those that have been removed by the cloud provider, or those failing MachineHealthChecks)
- As a cluster administrator of OpenShift, I would like to be able to make changes to the configuration of the
  underlying hardware of my control plane and have these changes safely applied using immutable hardware concepts
- As a cluster administrator of OpenShift, I would like to be able to control rollout of changes to my Control Plane
  Machines such that I can test changes with a single replica before applying the change to all replicas
- (Future work) As a cluster administrator of OpenShift with restricted hardware capacity, I would like to be able to   
  scale down my control plane before adding new Control Plane Machines with newer configuration, notably, my
  environment does not have capacity to add additional Machines during updates

### API Extensions

We will introduce a new `ControlPlaneMachineSet` CRD to the `machine.openshift.io/v1` API group. It will be based
on the spec and status structures defined below.

```go
// ControlPlaneMachineSet represents the configuration of the ControlPlaneMachineSet.
type ControlPlaneMachineSetSpec struct {
	// Replicas defines how many Control Plane Machines should be
	// created by this ControlPlaneMachineSet.
	// This field is immutable and cannot be changed after cluster
	// installation.
	// The ControlPlaneMachineSet only operates with 3 or 5 node control planes,
	// 3 and 5 are the only valid values for this field.
	// +kubebuilder:validation:Enum:=3;5
	// +kubebuilder:default:=3
	// +kubebuilder:validation:Required
	Replicas *int32 `json:"replicas"`

	// Strategy defines how the ControlPlaneMachineSet will update
	// Machines when it detects a change to the ProviderSpec.
	// +kubebuilder:default:={type: RollingUpdate}
	// +optional
	Strategy ControlPlaneMachineSetStrategy `json:"strategy,omitempty"`

	// Label selector for Machines. Existing Machines selected by this
	// selector will be the ones affected by this ControlPlaneMachineSet.
	// It must match the template's labels.
	// This field is considered immutable after creation of the resource.
	// +kubebuilder:validation:Required
	Selector metav1.LabelSelector `json:"selector"`

	// Template describes the Control Plane Machines that will be created
	// by this ControlPlaneMachineSet.
	// +kubebuilder:validation:Required
	Template ControlPlaneMachineSetTemplate `json:"template"`
}

// ControlPlaneMachineSetTemplate is a template used by the ControlPlaneMachineSet
// to create the Machines that it will manage in the future.
// +union
// + ---
// + This struct is a discriminated union which allows users to select the type of Machine
// + that the ControlPlaneMachineSet should create and manage.
// + For now, the only supported type is the OpenShift Machine API Machine, but in the future
// + we plan to expand this to allow other Machine types such as Cluster API Machines or a
// + future version of the Machine API Machine.
type ControlPlaneMachineSetTemplate struct {
  // MachineType determines the type of Machines that should be managed by the ControlPlaneMachineSet.
	// Currently, the only valid value is machines_v1beta1_machine_openshift_io.
	// +unionDiscriminator
	// +kubebuilder:validation:Required
	MachineType ControlPlaneMachineSetMachineType `json:"machineType"`

	// OpenShiftMachineV1Beta1Machine defines the template for creating Machines
	// from the v1beta1.machine.openshift.io API group.
	// +kubebuilder:validation:Required
	OpenShiftMachineV1Beta1Machine *OpenShiftMachineV1Beta1MachineTemplate `json:"machines_v1beta1_machine_openshift_io,omitempty"`
}

// ControlPlaneMachineSetMachineType is a enumeration of valid Machine types
// supported by the ControlPlaneMachineSet.
// +kubebuilder:validation:Enum:=machines_v1beta1_machine_openshift_io
type ControlPlaneMachineSetMachineType string

const (
	// OpenShiftMachineV1Beta1MachineType is the OpenShift Machine API v1beta1 Machine type.
	OpenShiftMachineV1Beta1MachineType ControlPlaneMachineSetMachineType = "machines_v1beta1_machine_openshift_io"
)

// OpenShiftMachineV1Beta1MachineTemplate is a template for the ControlPlaneMachineSet to create
// Machines from the v1beta1.machine.openshift.io API group.
type OpenShiftMachineV1Beta1MachineTemplate struct {
	// FailureDomains is the list of failure domains (sometimes called
	// availability zones) in which the ControlPlaneMachineSet should balance
	// the Control Plane Machines.
	// This will be merged into the ProviderSpec given in the template.
	// This field is optional on platforms that do not require placement
	// information, eg OpenStack.
	// +optional
	FailureDomains FailureDomains `json:"failureDomains,omitempty"`

	// ObjectMeta is the standard object metadata
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	// Labels are required to match the ControlPlaneMachineSet selector.
	// +kubebuilder:validation:Required
	ObjectMeta ControlPlaneMachineSetTemplateObjectMeta `json:"metadata"`

	// Spec contains the desired configuration of the Control Plane Machines.
	// The ProviderSpec within contains platform specific details
	// for creating the Control Plane Machines.
	// The ProviderSe should be complete apart from the platform specific
	// failure domain field. This will be overriden when the Machines
	// are created based on the FailureDomains field.
	// +kubebuilder:validation:Required
	Spec machinev1beta1.MachineSpec `json:"spec"`
}

// ControlPlaneMachineSetTemplateObjectMeta is a subset of the metav1.ObjectMeta struct.
// It allows users to specify labels and annotations that will be copied onto Machines
// created from this template.
type ControlPlaneMachineSetTemplateObjectMeta struct {
	// Map of string keys and values that can be used to organize and categorize
	// (scope and select) objects. May match selectors of replication controllers
	// and services.
	// More info: http://kubernetes.io/docs/user-guide/labels
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Annotations is an unstructured key value map stored with a resource that may be
	// set by external tools to store and retrieve arbitrary metadata. They are not
	// queryable and should be preserved when modifying objects.
	// More info: http://kubernetes.io/docs/user-guide/annotations
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// ControlPlaneMachineSetStrategy defines the strategy for applying updates to the
// Control Plane Machines managed by the ControlPlaneMachineSet.
type ControlPlaneMachineSetStrategy struct {
	// Type defines the type of update strategy that should be
	// used when updating Machines owned by the ControlPlaneMachineSet.
	// Valid values are "RollingUpdate" and "OnDelete".
	// The current default value is "RollingUpdate".
	// +kubebuilder:default:="RollingUpdate"
	// +kubebuilder:validation:Enum:="RollingUpdate";"OnDelete"
	// +optional
	Type ControlPlaneMachineSetStrategyType `json:"type,omitempty"`

	// This is left as a struct to allow future rolling update
	// strategy configuration to be added later.
}

// ControlPlaneMachineSetStrategyType is an enumeration of different update strategies
// for the Control Plane Machines.
type ControlPlaneMachineSetStrategyType string

const (
	// RollingUpdate is the default update strategy type for a
	// ControlPlaneMachineSet. This will cause the ControlPlaneMachineSet to
	// first create a new Machine and wait for this to be Ready
	// before removing the Machine chosen for replacement.
	RollingUpdate ControlPlaneMachineSetStrategyType = "RollingUpdate"

	// Recreate causes the ControlPlaneMachineSet controller to first
	// remove a ControlPlaneMachine before creating its
	// replacement. This allows for scenarios with limited capacity
	// such as baremetal environments where additional capacity to
	// perform rolling updates is not available.
	Recreate ControlPlaneMachineSetStrategyType = "Recreate"

	// OnDelete causes the ControlPlaneMachineSet to only replace a
	// Machine once it has been marked for deletion. This strategy
	// makes the rollout of updated specifications into a manual
	// process. This allows users to test new configuration on
	// a single Machine without forcing the rollout of all of their
	// Control Plane Machines.
	OnDelete ControlPlaneMachineSetStrategyType = "OnDelete"
)

// FailureDomain represents the different configurations required to spread Machines
// across failure domains on different platforms.
// +union
type FailureDomains struct {
	// Platform identifies the platform for which the FailureDomain represents
	// +unionDiscriminator
	// +optional
	Platform configv1.PlatformType `json:"platform,omitempty"`

	// AWS configures failure domain information for the AWS platform
	// +optional
	AWS *[]AWSFailureDomain `json:"aws,omitempty"`

	// Azure configures failure domain information for the Azure platform
	// +optional
	Azure *[]AzureFailureDomain `json:"azure,omitempty"`

	// GCP configures failure domain information for the GCP platform
	// +optional
	GCP *[]GCPFailureDomain `json:"gcp,omitempty"`

	// OpenStack configures failure domain information for the OpenStack platform
	// +optional
	OpenStack *[]OpenStackFailureDomain `json:"openstack,omitempty"`
}

// AWSFailureDomain configures failure domain information for the AWS platform
// +kubebuilder:validation:MinProperties:=1
type AWSFailureDomain struct {
	// Subnet is a reference to the subnet to use for this instance.
	// If no subnet reference is provided, the Machine will be created in the first
	// subnet returned by AWS when listing subnets within the provided availability zone.
	// +optional
	Subnet *AWSResourceReference `json:"subnet,omitempty"`

	// Placement configures the placement information for this instance
	// +optional
	Placement AWSFailureDomainPlacement `json:"placement,omitempty"`
}

// AWSFailureDomainPlacement configures the placement information for the AWSFailureDomain
type AWSFailureDomainPlacement struct {
	// AvailabilityZone is the availability zone of the instance
	// +kubebuilder:validation:Required
	AvailabilityZone string `json:"availabilityZone"`
}

// AzureFailureDomain configures failure domain information for the Azure platform
type AzureFailureDomain struct {
	// Availability Zone for the virtual machine.
	// If nil, the virtual machine should be deployed to no zone
	// +kubebuilder:validation:Required
	Zone string `json:"zone"`
}

// GCPFailureDomain configures failure domain information for the GCP platform
type GCPFailureDomain struct {
	// Zone is the zone in which the GCP machine provider will create the VM.
	// +kubebuilder:validation:Required
	Zone string `json:"zone"`
}

// OpenStackFailureDomain configures failure domain information for the OpenStack platform
type OpenStackFailureDomain struct {
	// The availability zone from which to launch the server.
	// +kubebuilder:validation:Required
	AvailabilityZone string `json:"availabilityZone"`
}

// ControlPlaneMachineSetStatus represents the status of the ControlPlaneMachineSet CRD.
type ControlPlaneMachineSetStatus struct {
	// Conditions represents the observations of the ControlPlaneMachineSet's current state.
	// Known .status.conditions.type are: (TODO)
	// TODO: Identify different condition types/reasons that will be needed.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ObservedGeneration is the most recent generation observed for this
	// ControlPlaneMachineSet. It corresponds to the ControlPlaneMachineSets's generation,
	// which is updated on mutation by the API Server.
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// Replicas is the number of Control Plane Machines created by the
	// ControlPlaneMachineSet controller.
	// Note that during update operations this value may differ from the
	// desired replica count.
	// +optional
	Replicas int32 `json:"replicas,omitempty"`

	// ReadyReplicas is the number of Control Plane Machines created by the
	// ControlPlaneMachineSet controller which are ready.
	// +optional
	ReadyReplicas int32 `json:"readyReplicas,omitempty"`

	// UpdatedReplicas is the number of non-terminated Control Plane Machines
	// created by the ControlPlaneMachineSet controller that have the desired
	// provider spec.
	// +optional
	UpdatedReplicas int32 `json:"updatedReplicas,omitempty"`

	// UnavailableReplicas is the number of Control Plane Machines that are
	// still required before the ControlPlaneMachineSet reaches the desired
	// available capacity. When this value is non-zero, the number of
	// ReadyReplicas is less than the desired Replicas.
	// +optional
	UnavailableReplicas int32 `json:"unavailableReplicas,omitempty"`
}
```

### Implementation Details/Notes/Constraints

The ControlPlaneMachineSet controller aims to act similarly to the Kubernetes StatefulSet controller.
It will take the desired configuration, the `ProviderSpec`, and ensure that an appropriate number of Machines exist
within the cluster which match this specification.

It will be introduced as a new second level operator so that, if there are issues with its operation, it may report
this via a `ClusterOperator` and prevent upgrades or further disruption until the issues have been rectified.
Due to the nature of the actions performed by this controller, we believe it is important that it have its own
`ClusterOperator` in which to report its current status.

The `ControlPlaneMachineSet` CRD will be limited to a singleton within a standard OpenShift HA cluster, the only
allowed name will be `cluster`. This matches other high level CRD concepts such as the `Infrastructure` object.
The operator will operate solely on the `openshift-machine-api` namespace as with other Machine API components.
Only `ControlPlaneMachineSets` in this namespace will be reconciled.
The resource must be namespaced to ensure compatibility with future OpenShift projects such as Cluster API and
centralised machine management patterns.

The behaviour of such a controller is complex, and as such, various features of the controller and scenarios are
outlined in the details below.

#### Desired number of Machines

At present, the only source of the installed control plane size within OpenShift clusters exists within the
`cluster-config-v1` ConfigMap in the `kube-system` namespace.
This ConfigMap has been deprecated for some time and as such, should not be used for scale in new projects.

Due to this limitation, we will need to have a desired replicas field within the ControlPlaneMachineSet controller.
As we are not planning to tackle horizontal scaling of the control plane initially, we will implement a validating
webhook to deny changes to this value once set.

For new clusters, the installer will create (timeline TBD) the `ControlPlaneMachineSet` resource and will set the value
based on the install configuration.
For existing clusters, we will need to validate during creation of the `ControlPlaneMachineSet` resource that the
number of existing Control Plane Machines matches the replica count set within the `ControlPlaneMachineSet`.
This will prevent end users from attempting to horizontally scale their control plane during creation of the
`ControlPlaneMachineSet` resource.

In the future, once we have identified risks and issues with horizontal scale, and mitigated those, we will remove the
immutability restriction on the replica field to allow horizontal scaling of the control plane between 3 and 5 replicas
as per current support policy for control plane sizing.

#### Selecting Machines

The ControlPlaneMachineSet operator will use the selector defined within the CRD
to find Machines which it should consider to be within the set of control plane Machines it is responsible for managing.

This set should be the complete control plane and there should be a 1:1 mapping of Machines in this set to the control
plane nodes within the cluster.

If there are any control plane nodes (identified by the node role) which do not have a Machine, the operator will mark
itself degraded as this is an unexpected state.
No further action (ie creating/deleting Control PlaneMachines, any rollouts or updates that need to be applied) will be
taken until the unknown node has either been removed from the cluster or a Machine has been manually created for it.

#### Providing high availability/fault tolerance within the control plane

Typically within OpenShift, Machines are created within a MachineSet.
MachineSets have a single `ProviderSpec` which defines the configuration for the Machines created by the MachineSet.
The failure domain (sometimes called availability zone) is a part of this provider spec and as such, defines that all
Machines within the Machineset share a failure domain.

This is undesirable for Control Plane Machines as we wish to have them spread across multiple availability zones to
reduce the likelihood of datacenter level faults degrading the control plane.
To this end, the failure domains for the control plane will be set on the `ControlPlaneMachineSet` spec directly.

When creating Machines, the `ControlPlaneMachineSet` controller will balance the Machines across these failure domains
by injecting the desired failure domain for the new Machine into the provider spec based on the platform specific
failure domain field.

##### Failures domains

The `FailureDomains` field is expected to be populated by the user/installer based on the topology of the Control Plane
Machines. For example, we expect on AWS that this will contain a list of availability zones within a single region.
Note, we are explicitly not expecting users to add different regions to the the `FailureDomains` as we do not support
running OpenShift across multiple regions.

Note that the `FailureDomains` field is only supported on certain platforms, currently; AWS, Azure, GCP and OpenStack;
other platforms may be supported in the future.

The users will be allowed to override a small amount of configuration for the `providerSpec` based on the configuration
required to spread Machines across different failure domains.
For example, on AWS, both the `availabilityZone` and `subnets` differ depending on which failure domain is configured,
on other platforms, eg Azure, GCP or OpenStack, only one field is required to vary the failure domain, in which case,
this is all that will be allowed.

The overrides will be injected into the given `providerSpec` before creating the Machines as part of the balancing logic
within the `ControlPlaneMachineSet` operator.

As an example, a user on AWS may set their `FailureDomains` as:

```yaml
failureDomains:
  aws:
  - placement:
      availabilityZone: us-east-1a
    subnet:
      filters:
      - name: "tag:Name"
        values:
        - "my-cluster-subnet-1a"
  - placement:
      availabilityZone: us-east-1b
      subnet:
        filters:
        - name: "tag:Name"
          values:
          - "my-cluster-subnet-1b"
```

A user on Azure may set their `FailureDomains` as:

```yaml
failureDomains:
  azure:
  - zone: us-central-1
  - zone: us-central-2
```

###### Failure Domains on vSphere

As of OpenShift 4.10, there is no concept of a zone within vSphere and as such we would expect that the `FailureDomains`
would be omitted for vSphere.

However, [there is future work](https://github.com/openshift/enhancements/pull/918) to include zone support for vSphere
within 4.11. Once this is implemented, the Machine API provider for vSphere will understand the concept of zones,
defined within the `Infrastructure` resource.
The `ControlPlaneMachineSet` will then be able to balance Machines across the different zones by setting the new zone
information within the provider specs as it does on other platforms.

#### Ensuring Machines match the desired state

To ensure Machines are up to date with the desired configuration, the `ControlPlaneMachineSet` controller will leverage
a similar pattern to the workload controllers.
It will hash the template and compare the hashed result with the spec of the Machines within the managed set.
As we expect the failure domain to vary across the managed Machines, this will need to be omitted before the hash can
be calculated.

Should any Machine not match the desired hash, it will be updated based on the chosen update strategy.

##### The RollingUpdate update strategy

The RollingUpdate strategy will be the default. It mirrors the RollingUpdate strategy familiar to users from
Deployments. When a change is required to be rolled out, the Machine controller will create a new Control Plane
Machine, wait for this to become ready, and then remove the old Machine. It will repeat this process until all Machines
are up to date.
During this process the etcd membership will be protected by the mechanism described in a [separate enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/protecting-etcd-quorum-during-control-plane-scaling.md),
in particular it isn't essential that the Control Plane Machines are updated in a rolling fashion for etcd sakes, though
to avoid potential issues with the spread of etcd members across failure domains during update, the
`ControlPlaneMachineSet` will perform a rolling update domain by domain.

At first, we will not allow any configuration of the RollingUpdate (as you might see in a Deployment) and will pin the
surge to 1 replica. We may wish to change this later once we have more data about the stability of this operator and
the etcd protection mechanism it relies on.

We expect this strategy to be used in most applications of the `ControlPlaneMachineSet`, though it is not appropriate
in all environments. For example, we expect this strategy to not be used in environments where capacity is very limited
and a surge of any control plane capacity is unavailable.

##### The Recreate update strategy (FUTURE WORK)

Note: This strategy is not planned for implementation in the initial phase of this project. There are a number of
open questions that need to be ironed out and the use case is not immediately required with OpenShift.
The details of the strategy are left here as a basis of the future work.

The Recreate strategy mirrors the Recreate strategy familiar to users from Deployments.
When a change is required to be rolled out, the Machine controller will first remove a Control Plane Machine, wait for
its removal, and then create a new Control Plane Machine.

This strategy is intended to be used only in very resource constrained environments where there is no capacity to
introduce an extra temporary Control Plane Machine (preventing the RollingUpdate strategy).

At present, when using this strategy, the updates will need manual intervention. The etcd protection design will
prevent the Control Plane Machine from being removed until a replacement for the etcd member has been introduced into
the cluster. To allow for this use case, the end user can manually remove the etcd protection (via removing the Machine
deletion hook on the deleted Machine) allowing the rollout to proceed, the etcd operator will not re-add the
protection if the Machine is already marked for deletion.

This strategy introduces risk into the update of the Machines. While the old Machine is being drained/removed and the
new Machine is being provisioned, etcd quorum is at risk as a member has been removed from the cluster. In most
circumstances this would leave the cluster with just 2 etcd members.
This poses a similar risk to the etcd cluster as is present during a cluster upgrade when the Machine Config Daemon
reboots the Control Plane Machines, however, in this case, the duration of the member being down is expected to be
much longer than in the update process. For example baremetal clusters can take over an hour to reprovision a host.

There are a number of open questions related to this update strategy:
- Are we comfortable offering this strategy given the risks that it poses?
- What alternative can we provide to satisfy resource constrained environments if we do not implement this strategy?
- Should we teach the etcd operator to remove the protection mechanism when the `ControlPlaneMachineSet` is configured
  with this strategy?
- To minimise risk, should we allow this strategy only on certain platforms? (Eg disallow the strategy on public clouds)
- Do we want to name the update strategy in a way that highlights the risks associated, eg `RecreateUnsupported`?

##### The OnDelete update strategy

The OnDelete strategy mirrors the OnDelete strategy familiar to users from StatefulSets.
When a change is required, any logic for rolling out the Machine will be paused until the `ControlPlaneMachineSet`
controller observes a deleted Machine.
When a Machine is marked for deletion, it will create a new Machine based on the current spec.
It will then proceed with the replacement as normal.

This strategy will allow for explicit control for end users to decide when their Control Plane Machines are updated.
In particular it will also allow them to have different specs across their control plane.
While in the normal case this is discouraged, this could be used for short periods to test new configuration before
rolling the updated configuration to all Control Plane Machines.

Note that the OnDelete strategy is similar to the RollingUpdate strategy in that it needs to be able to add new
Machines to the cluster to function. It can be thought of as a slower, more controlled RollingUpdate strategy where the
user decides when each Control Plane Machine is replaced, and marks it for replacement by deleting it.
Otherwise the replacement is identical to the RollingUpdate strategy.

#### Protecting etcd quorum during update operations

With the introduction of the [etcd protection enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/protecting-etcd-quorum-during-control-plane-scaling.md), the
ControlPlaneMachineSet does not need to observe anything etcd related during scaling operations.
In particular, it is the etcd operators responsibility to add Machine deletion hooks to prevent Control Plane Machines
from being removed until they are no longer needed.
When the `ControlPlaneMachineSet` controller observes that a newly created Machine is ready, it will delete the old
Control Plane Machine signalling to the etcd operator to switch the membership of the etcd cluster between the old and
new instances.
Once the membership has been switched, the Machine deletion hook will be removed on the old Machine, allowing it to be
removed by the Machine controller in the normal way.

#### Removing/disabling the ControlPlaneMachineSet

As some users may want to remove or disable the ControlPlaneMachineSet, a finalizer will be placed on the
`ControlPlaneMachineSet` to allow the controller to ensure a safe removal of the ControlPlaneMachineSet, while leaving
the Control Plane Machines in place.

Notably it will need to ensure that there are no owner references on the Machines pointing to the ControlPlaneMachineSet
instances. This will prevent the garbage collector from removing the Control Plane Machines when the
`ControlPlaneMachineSet` is deleted.

If users later wish to reinstall the `ControlPlaneMachineSet`, they are free to do so.

Note: Owner references will be added to the Control Plane Machines to identify to other components that a controller is
managing the state of these Machines. This allows other systems such as the MachineHealthCheck to identify that if they
were to make an action on the Machine, the Control Plane Machine Set Operator will react to that action.

#### Installing a ControlPlaneMachineSet within an existing cluster

When adding a ControlPlaneMachineSet to a existing cluster, the end user will need to define the
`ControlPlaneMachineSet` resource by copying the existing Control Plane Machine ProviderSpecs.
Once this is copied, they should remove the failure domain and add the desired failure domains to the `FailureDomains`
field within the ControlPlaneMachineSet spec.

To ensure adding a `ControlPlaneMachineSet` to the cluster is safe, we will need to ensure via a validating webhook
that the replica count defined in the spec is consistent with the actual size of the control plane within the cluster.
We will also validate that the failure domains align with those within the cluster already, this will prevent users
from accidentally migrating their Control Plane Machines from multiple availability zones to a single zone.

If no Control Plane Machines exist, or they are in a non-Running state, the operator will report degraded until this
issue is resolved. This creates a dependency for the `ControlPlaneMachineSet` operator on the Machine API. It will be
required to run at a higher run-level than Machine API.

This in turn means, that in a UPI (where typically Machine objects do not exist) or misconfigured clusters, a user
adding a `ControlPlaneMachineSet` will result in a degraded cluster.
Users will need to remove the invalid `ControlPlaneMachineSet` resource, or manually add correctly configured Control
Plane Machines to their cluster to restore their cluster to a healthy state.

We do not recommend that UPI users attempt to adopt their control plane instances into Machine API due to the
likelihood that the resulting spec does not match the existing Machines. Instead, we recommend that UPI users initially
go through the process of replacing their control plane instances with Machines by creating new Control Plane Machines,
allowing Machine API to create the instances, and then removing their old, manually created control plane instance.
This effectively means migrating their entire control plane onto Machine API before the `ControlPlaneMachineSet` will
take over the management.
We enforce this recommendation so that users can be confident in the Machine `providerSpec` that they have configured
and that Machine API will be able to create valid Machines from the spec before they then start using our automation.

##### Why not to populate the spec for the customer

One idea posed, is to allow the `ControlPlaneMachineSet` to be automatically populated if the spec is left blank when it
is created. This would improve the UX by allowing customers not to have to worry about the spec and allow it to be
inferred from the existing Machines within the cluster. It would also make it easier for managed services to adopt
`ControlPlaneMachineSets` throughout the fleet without having to manually check each cluster.

There are however a few concerns about this idea which means we are planning to not implement this (at least for the
first iteration of the project):
- It may encourage users to attempt to use `ControlPlaneMachineSets` with UPI clusters which are incorrectly configured
  - Some users have Machines in their cluster, that aren't actually configured correctly, therefore we would be creating
    an incorrectly configured `ControlPlaneMachineSet` for them, it is then unclear who is at fault for this and we
    could end up with an increased number of support tickets due to these misconfigurations
  - As an example, this is particularly prevalent in UPI clusters where users can forget to remove the Machine manifests
    from the install manifest directory, this has been the source of many bugs where users have had Machines stuck in
    Provisioning for an extended period. Inferring the specs from these Machines would certainly make the
    `ControlPlaneMachineSet` invalid.
- Some users may have made out of band changes to their Control Plane Machines which are not reflected within the
  Machine specs
  - We document the procedure within product docs of replacing a Control Plane Machine, though it is a manual process
    that doesn't involve using the Machine API. Within the process we instruct users to resize their VM within the
    cloud, and then update their Machine `providerSpec` to match the changes in the cloud. We suspect that there are
    a number of users who have made this change, or other similar changes that are not reflected in the `providerSpec`
    and as such, inferring the configuration from these Machines would create an incorrect spec.

If we do implement some adoption process in the future, we should also include a `paused` field within the spec, set to
`true` by the adoption logic, that prevents the `ControlPlaneMachineSet` from taking any action until someone has
reviewed the inferred spec and marked `paused: false`. This would allow a sanity check to be enforced before the
`ControlPlaneMachineSet` takes any actions based on the inferred spec.

An alternative way to populate the spec could be to build a plugin for `oc` which would inspect the Control Plane
Machines and print out a `ControlPlaneMachineSet` for the customer to create. This would also allow the customer to
inspect the resource before they create it within the cluster and would make it a very conscious decision on their part
to create the `ControlPlaneMachineSet`.

#### Naming of Control Plane Machines owned by the ControlPlaneMachineSet

In an IPI OpenShift cluster, the Control Plane Machines are named after the cluster ID followed by an index, for example
`my-openshift-cluster-abcde-master-0`. As we will need to create additional Machines during replacement operations, we
cannot reuse the names of the existing Machines. Instead we will generate a random, 5 character string, and add this
before the index of the Machine. For example, Machines created by a `ControlPlaneMachineSet` may have a name such as
`my-openshift-cluster-abcde-master-fghij-0`. This should make it clear to the end user which Machine we are attempting
to replace with the newer Machine. As we typically spread Machines across multiple availability zones, we will keep the
indexes consistent with the availability zone that they were originally assigned.

As users can today replace their Machines with differently named Machines (eg as part of a recovery process), we do not
expect other components to be relying on the index of the Machines for any function within OpenShift, but we may
discover during testing something that is currently unknown to us. If we discover issues with other components relying
on a particular naming scheme for the Control Plane Machines, we will need to solve this issue within the other
component.

#### Delivery via the core OpenShift payload

This new operator will form part of the core OpenShift release payload for clusters installed on/upgrading to OpenShift
4.11. While there is a movement within OpenShift to stop adding new components to the release payload, we believe that
this new operator should be added to the core for the following reasons:

- The scaling operations enabled by this operator are often required when there is an imminent cluster failure due to
  an overwhelmed control plane. Having the `ControlPlaneMachineSet` already installed means the users will be able to
  scale their control plane without first having to learn that this project exists or installing/configuring it.
- By having this installed by the installer, there is a lower likelihood of issues where configuration within the
  `ControlPlaneMachineSet` is invalid, such that it wouldn't be able to create new Machines when required.
  The installer already has logic for creating MachineSets, which will form a basis of the installation of the
  `ControlPlaneMachineSet`.
- Having this installed by default will make it easier for managed services to manage. The managed services team intend
  to leverage this operator across thousands of clusters. Installing the operator and configuring it correctly for each
  new cluster is a significant amount of work for them to automate. Including this within the installer would make this
  easier to rollout to new clusters in the fleet.
- The operator adds a lot of value for recovery of failed Control Plane Machines in clusters using Machine API. If the
  operator is installed by default, users are more likely to be able to correctly create new Machines during the
  cluster recovery process.

This operator however is not critical to the OpenShift cluster, and while we believe this should be installed by
default, it should be be made optional via the install time [component-selection](https://github.com/openshift/enhancements/blob/master/enhancements/installer/component-selection.md)
mechanism currently being implemented.

#### How does this new operator fit within the Cluster API landscape

Within Cluster API, a concept exists known as a Control Plane Provider. This component, currently with a single upstream
reference implementation based on KubeADM, is intended to instantiate and manage a control plane within for the
Kubernetes guest cluster.

The Control Plane Provider is responsible not only for creating the infrastructure for the Control Plane Machines but
also etcd and the control plane Kubernetes components (API server, Controller Manager, Scheduler, Cloud Controller
Manager). Within OpenShift, various different operators implement the management and responsibility of these components,
however, to date we do not have a Machine Infrastructure operator that fits this role.

In a future iteration of the `ControlPlaneMachineSet`, we could use it to satisfy the Cluster API Control Plane Provider
contract and fill the role within OpenShift clusters running on CAPI. To ensure that this is a possibility, we are
planning to make the `ControlPlaneMachineSet` compatible, as much as possible, with the
[CRD contract](https://cluster-api.sigs.k8s.io/developer/architecture/controllers/control-plane.html#crd-contracts)
for the Control Plane Provider in Cluster API.
Importantly, we are designing the CRD API with the intention of making it API compatible in the future without making
any breaking changes or needing to bump the API version of the `ControlPlaneMachineSet`.
Notably, an API restriction imposed by making this resource compatible with Cluster API is that the resource MUST be
Namespaced, and not Cluster scoped.

The notable exception to this, is that because Cluster API uses separate resources for Machine templates, and Machine
API embeds these directly within the spec, we will follow the Machine API convention in the first iteration of this
CRD and may evolve it at a later date by adding the additional fields required to satisfy the Machine template within
Cluster API.

The `template` within the `spec` is designed such that we can add additional supported machine types in the future.
We will add a new struct `machines.<version>.cluster-x.k8s.io` to the discriminated union to allow users to specify
the template for creating Cluster API machines via the `ControlPlaneMachineSet`.
The templates for different machine types are mutually exclusive and as such, users will only be able to set one
template type at a time.

We are planning to synchronise and convert resources from Machine API to Cluster API resources as part of our Cluster
API proof of concept. When converting resources, such as `MachineSets` and `Machines`, we will also convert the
`ControlPlaneMachineSet` in the same manor. Users will be able to select whether they want the Machine API or Cluster
API version of the resource to be authoritative, and therefore which should have controllers operate on it.

The Control Plane Provider in Cluster API is also responsible for creating and maintaining a Kubeconfig file that the
Cluster API components can use to manage resources within the guest cluster. Within the OpenShift Cluster API Technical
Preview, we are handling this Kubeconfig generation with a separate component, we will continue to do this even if
we make this new operator satisfy the Control Plane Provider contract.

Alternatively, instead of making the `ControlPlaneMachineSet` satisfy the Control Plane Provider contract, we may
introduce another CRD to act as a proxy, gathering information from the other components within the OpenShift cluster
to satisfy the requirements of the Control Plane Provider contract. Further investigation will be required in the future
to determine how exactly we want to handle this compatibility.

#### Interaction with Machine Health Check

In OpenShift, customers may choose to use MachineHealthCheck resources to remove failed Machines from their clusters
and have them replaced automatically by a MachineSet. MachineHealthCheck requires that a Machine has an Owner Reference
before it will remove the Machine, this prevents the removal of a Machine that is not going to be replaced.

As the Control Plane Machines will now be owned by the `ControlPlaneMachineSet`, MachineHealthChecks will now be
compatible with Control Plane Machines. This we believe to be safe due to the design of the `ControlPlaneMachineSet`
operator and the protection mechanism being implemented to protect etcd quorum. If at any point a Machine is deleted,
the protection system will ensure no Machine is actually removed until a replacement has been brought in to replace it
within the etcd cluster.

We expect that if a user wants automated remediation for their Control Plane Machines, they will configure a
MachineHealthCheck to point to the Control Plane Machines, but we will not configure this for them.

Notably, if a user does wish to use a MachineHealthCheck with the Control Plane Machines, we advise them to configure
the MachineHealthCheck just to observe Control Plane Machines and to have the `maxUnhealthy` field set to 1.
These recommendations will ensure that if more than one Control Plane Machine appears unhealthy at once, that the
MachineHealthCheck will take no action on the Machines. It is likely that if more than one Control Plane Machine appears
unhealthy that either the etcd cluster is degraded, or a scaling operation is taking place presently to replace a
failed Machine.

#### Management of Control Plane load balancers

In an OpenShift cluster, in general, there is a concept of an internal and external load balancer in front of the
Kubernetes API. These load balancers are created by the installer on various cloud providers but are later considered
for the most part to be unmanaged.

To enable Control Plane Machine replacement, Machine API handles adding and removing Control Plane Machines from these
load balancers on appropriate platforms (eg AWS, Azure and GCP). Therefore, when a customer today replaces their Control
Plane Machine, they do not need to worry about the load balancer attachment as this is automated for them.

As this is already a part of the Machine management directly, the `ControlPlaneMachineSet` does not need to be concerned
about the load balancer management itself.

On other platforms where virtual load balancers are employed (via Keepalived and HAProxy), such as vSphere or OpenStack,
the Kubernetes API load balancing is all in cluster and therefore is not required to be modified during a Control Plane
Machine replacement.

On platforms that do not yet support load balancer management (eg IBM and Alibaba), this will need to be implemented in
a similar manner to that of AWS, Azure and GCP before these platforms can be supported by `ControlPlaneMachineSets`.

### Risks and Mitigations

#### etcd quorum must be preserved throughout scaling operations

As we are planning to scale up/down Control Plane Machines in an automated fashion, scaling operations will inevitably
effect the stability of the etcd cluster.
To prevent disruption, we have an [existing mechanism](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/protecting-etcd-quorum-during-control-plane-scaling.md) that was
designed to allow the etcd operator to protect etcd quorum without other systems, such as Machine API, having any
knowledge of the state of the etcd cluster.

The protection mechanism is designed so that, even if a Machine is deleted, nothing will happen to the Machine (ie no
drain, no removal on the cloud provider) until the etcd operator allows the removal to proceed.
This prevents any data loss or quorum loss as the Machine will remain within the cluster until the etcd operator is
confident that it no longer needs the Machine.

The only time etcd quorum may be at risk is during the Recreate update strategy, this is highlighted in more detail
above.

#### Users may delete Control Plane Machines manually

If a user were to delete the Control Plane Machines using `oc`, `kubectl` or some other API call, the  
`ControlPlaneMachineSet` operator is designed in such a way that this should not pose a risk to the cluster.

The etcd protection mechanism will prevent removal of the Machines until they are no longer required for the etcd
quorum. The `ControlPlaneMachineSet` operator will, one by one, add new Control Plane Machines based on the existing
spec and wait for these to join the cluster. Once the new Machines have joined, they will replace the deleted Machines
as normal with the process outlined earlier in this document.

#### Machine Config may change during a rollout

There may be occasions where the Machine Config operator attempts to rollout new Machine Config during a Control Plane
scaling operation. We do not believe this will cause issue, but it may extend the time taken for the scaling operation
to take place.

While a scaling operation is in progress, etcd quorum is protected by the protection mechanism mentioned above as well
as the etcd quorum guard. The quorum guard will ensure that at most one etcd instance is interrupted at any time.
If the Machine Config operator needs to rollout an update, it will proceed in the usual manner while the etcd learner
process (part of scaling up a new etcd member) may suffer a delay due to the restart of the Control Plane Machines.

#### A user deletes the ControlPlaneMachineSet resource

Users will be allowed to remove `ControlPlaneMachineSet` resources should they so desire. This shouldn't pose a risk to
the control plane as the `ControlPlaneMachineSet` will orphan Machines it manages before it is removed from the
cluster.
More detail is available in the [Removing/disabling the ControlPlaneMachineSet](#Removingdisabling-the-ControlPlane)
notes.

#### The ControlPlaneMachineSet spec may not match the existing Machines

It is likely that in a number of scenarios, the spec attached to the `ControlPlaneMachineSet` for creating new Machines
may not match that of the existing Machines. In this case, we expect the `ControlPlaneMachineSet` operator will attempt
a rolling update
when it is created. Apart from refreshing the Machines potentially needlessly, this shouldn't have a negative effect on
the cluster assuming the new configuration is valid. If it is invalid, the cluster will degrade as the new Machines
created by the `ControlPlaneMachineSet` will fail to launch.

We have designed this operator with IPI style clusters in mind. We expect that the Machine objects within the cluster
should match the actual hosts on the infrastructure provider. In IPI clusters this is ensured by the installer.
Users may have, out of band, then modified these instances, but unfortunately we have no way to deal with this at
present. In these scenarios, when the Machines are replaced, they may not exactly match the previous iteration and
therefore may cause issues for the customer.
If a customer wishes to mitigate the chances of issues occurring, they may wish to manually control the roll out using
the OnDelete update strategy.

## Design Details

### Open Questions

Some open questions currently exist, but are called out in the Recreate Update Strategy notes.

1. How do we wish to introduce this feature into the cluster, should it be a Technical Preview in the first instance?

### Test Plan

As this project operates solely on resources within the Kubernetes environment, we will aim to leverage the controller
runtime [envtest](https://book.kubebuilder.io/reference/envtest.html) project to write an extensive integration test
suite for the main functionality of the new operator.

In particular, tests should include:
- Behaviour when there is no `ControlPlaneMachineSet` within the cluster
- Behaviour when a new `ControlPlaneMachineSet` is created
  - What happens with regard to the adoption of existing Control Plane Machines
  - What happens if the configuration differs from the existing Machines within the cluster (per strategy)
  - How does the ClusterOperator status present during the adoption
  - What happens if the name is wrong or there are multiple `ControlPlaneMachineSets`
  - How does the status get updated during this process
- Behaviour during a configuration change
  - How does the RollingUpdate strategy behave
  - How does the OnDelete strategy behave
  - How does the status get updated during rollouts
- Behaviour when a `ControlPlaneMachineSet` is removed
  - The effect on the Machines (removing owner references etc)
  - Removal of the finalizer

Notably, these tests will involve a large amount of simulation as we won't have a running Machine controller, nor will
we have an etcd operator adding Machine deletion hooks. The functions of these two operators will be simulated to
exercise the `ControlPlaneMachineSet` operator based on the behaviours that the operator relies on.

As this isn't a true E2E test, we will also duplicate a number of the above tests into an E2E suite that can run on a
real cluster. This E2E suite has the potential to be very disruptive, we expect that a sufficiently well written
integration suite should prevent this however.

We will ensure that the origin E2E suite is updated to include tests for this component to prevent regressions in either
the etcd operator or Machine API ecosystem from breaking the functionality provided by this operator.

### Graduation Criteria

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

This enhancement does not describe removing a feature of OpenShift, this section is not applicable.

### Upgrade / Downgrade Strategy

When the new operator is introduced into the cluster, it will not operate until a `ControlPlaneMachineSet` resource has
been created. This means we do not need to worry about upgrades during the introduction of this resource.

### Version Skew Strategy

The `ControlPlaneMachineSet` operator relies on the Machine API. This is a stable API within OpenShift and we are not
expecting changes that will cause version skew issues over the next handful of releases.

### Operational Aspects of API Extensions

We will introduce a new `ClusterOperator` for the `ControlPlaneMachineSet` operator. Within this we will introduce
conditions (TBD) which describe the state of the operator and the control plane which it manages.

This new `ClusterOperator` will be the key resource from which support should discover information if they believe the
`ControlPlaneMachineSet` to be degraded.

There should be no effect to existing operators or supportability of other components due to this enhancement, in
particular, this is strictly adding new functionality on top of an existing API, we do not expect it to impact the
functionality of other APIs.

#### Failure Modes

- A new Machine fails to scale up
  - In this scenario, we expect the operator to turn degraded to signal that there is an issue with the Control Plane
    Machines
  - We expect that the Machine will contain information identifying the issue with the launch request, which can be
    rectified by the user before they then delete the Machine, once deleted, the ControlPlaneMachineSet will attempt the
    scale up again
  - This process should be familiar from dealing with existing Failed Machines
  - A MachineHealthCheck targeting Control Plane Machines could automatically resolve this issue
- The `ControlPlaneMachineSet` webhook is not operational
  - This will prevent creation and update of ControlPlaneMachineSets
  - We expect these operations to be very infrequent within a cluster once established, so the impact should be
    minimal
  - The failure policy will be to fail closed, as such, when the webhook is down, no update operations will succeed
  - The Kube API server operator already detects failing webhooks and as such will identify the issue early
  - The ControlPlaneMachineSet is not a critical cluster operator. Everything done by the operator described here can be
    done manually to the machine(set) objects.

#### Support Procedures

TBD

## Implementation History

There is not current implementation of this enhancement, however, it will depend on the
[etcd quorum protection mechanism](https://github.com/openshift/enhancements/blob/master/enhancements/etcd/protecting-etcd-quorum-during-control-plane-scaling.md) which is currently being
implemented.

## Drawbacks

- Introduction of new CRDs complicates the user experience
  - Users will have to learn about the new CRD and how it can be used
  - Counter: The concepts are familiar from existing apps resources and CRDs
- We are making it easier for customers to put their clusters at risk
  - If a scale up fails, this will then need manual intervention, the cluster will become degraded at this point
  - Counter: The etcd protection mechanism should mean that the cluster is never at risk of losing quorum, clusters
    should be recoverable
- We will need confidence in the project before we can ship it
  - We need to decide how to ship the project, will we ship it as GA or TP in the first instance?

## Alternatives

Previous iterations of this enhancement linked in the metadata describe alternative ways we could implement this
feature.

### Layering MachineSets

There has been previous discussion about the use of MachineSets to create the Machines for the Control Plane.
In previous iterations of this enhancement they have been recommendations to either create one MachineSet per
availability zone or to have some `ControlPlaneMachineSet` like CRD create MachineSets and leverage these to create the
Machines.
This proposal deliberately omits MachineSets due to concern over the risks and drawbacks that leveraging MachineSets
poses in this situation.

Exposing MachineSets within this mechanism exposes risk in a number of ways:

- Users have the ability to scale the MachineSet
  - We do not intend to support horizontal scaling initially, there's no easy way to prevent users scaling the
    Control Plane MachineSets while not affecting workload MachineSets
  - If a user were to scale up the MachineSet, something would need to sit on top to scale the MachineSet back to the
    supported control plane replicas count. It is then difficult to ensure that the correct Machine is removed from
    the cluster without jeopardising the etcd quorum. If a user attempted to use GitOps to manage the MachineSet,
    this could cause issues as the higher level controller battles to scale the MachineSet.
- If no higher level object exists, difficult to ensure consistency across MachineSets
  - We promote to users that their control plane should be consistent across availability zones, with separate
    MachineSets, there is nothing to prevent users modifying the MachineSets between zones and having major
    differences. Collating these differences for support issues could prove difficult. Users may also spread their
    Machines in an undesirable manner.
  - Users will have the ability to have inconsistency while using the OnDelete update strategy with
    `ControlPlaneMachineSets`, but this should be easy to track due to the updated replicas count within the
    `ControlPlaneMachineSet` status. We may want to degrade the operator if there are discrepancies to encourage users
    to keep their control plane consistent.
- Users can delete intermediary MachineSets
  - In this case, if a user were to delete the Control Plane MachineSet, it is hard to define a safe way to leave the
    Control Plane Machine(s) behind without having to have very specific knowledge baked into the MachineSet
    controller
  - It becomes easier for users to mismanage their Control Plane Machines and put their cluster at risk
  - Previous enhancements have discussed the use of webhooks to prevent the deletion of Control Plane MachineSets,
    though these are not foolproof. The removal design of the `ControlPlaneMachineSet` (being operator based) should be
    more reliable than a webhook.

### StatefulMachineSet

The proposed `ControlPlaneMachineSet` is reasonably similar to a mixture between a `StatefulSet` and a `MachineSet`.
The `ControlPlaneMachineSet` is targeted specifically at managing Control Plane Machines but we could also create a
more generic `StatefulMachineSet` that covers this use case and others. The implementation would be very similar, though
would most likely not have its own `ClusterOperator` to report status.

To understand why we don't think a more generic `StatfeulMachineSet` is worth implementing, we must look at the promises
that `StatefulSets` make and why users would want to use them.

- Stable, unique network identifies: Users have applications that require stable IP addresses, eg. something like etcd
- Stable, persistent storage: Users must reattach applications to the same storage disk as previously used when the
  workload is rescheduled
- Ordered, graceful deployment, scaling and rolling updates: Different applications must be updated in a certain and
  controlled way

In most Kubernetes environments and cloud applications, the IP address of the host does not matter and is not required
to be stable. The application layer networking means that the host IP address is insignificant to the functionality of
the cluster. The only use case we can consider where static IPs over a given set of hosts are required would be in a
scenario where you have an external load balancer that requires reconfiguration if the host IPs change. However, for
this example, it would most likely be more cloud-native to implement an operator that could reconfigure the load
balancer on changes rather than trying to keep the IP addresses of the hosts static. Additionally, there are already
projects tackling IPAM within Kubernetes which may resolve this issue without having to make pets of the Machines.

For storage, we expect most users to use persistent volumes which can be, in most environments attached to multiple
hosts, whether that be abstracted away as a cloud provider service (eg AWS EFS) or as an iSCSI storage network Within
a datacenter. In certain applications these network storage provisions may not be suitable however and you may need
access to a local disk or volume. In cloud environments this doesn't apply, in virtualized environments the local
volume would be represented as a persistent volume that is only able to be attached to VMs on a certain physical host,
and in bare-metal environments, you would need to schedule to a single host, in which case, existing pod scheduling
mechanisms would ensure this scheduling provided the Machine has some persistent labelling. In the bare metal case,
this is already achieved through the hardware inventories provided by Metal3.

For graceful ordered deployments, this isn't typically a property of the host but the applications running on top of
them. If we want to provide users with the ability to apply updates to their Machines, we are likely better implementing
a `MachineDeployment` concept similar to that of the Cluster API project. This allows automated updates by creating new
Machines, as described within this document but does treat the Machines with any special consideration.
When OpenShift 4 was conceived, the `MachineDeployment` concept was originally tabled because, although we promote
immutable infrastructure within OpenShift, our OS level is updated automatically through the Machine Config Operator
system. The two ideas would work together, but we didn't want to force users to redeploy every Machine to benefit from
updates and so the value of the `MachineDeployment` was diminished.

Aside from the above arguments, there are other higher level reasons we don't feel that a generic `StatefulMachineSet`
is a valuable addition to OpenShift.
- The concept of stateful Machines goes against the cattle not pets concept on which the rest of Machine API has been
  built
- Machines in OpenShift don't, in the majority of cases, have any state and, we shouldn't promote them to have state.
  The state is handled at the application layer by adding additional abstractions such as persistent volumes.
- A lot of the scenarios we could think of for having a stateful group of Machines, could also be solved by using a set
  of well defined `MachineSets`.
- To our knowledge, no customers have asked for a `StatefulMachineSet`
- For the Control Plane case, we want additional monitoring on top of what a `StatefulMachineSet` might provide:
  - The ability to track the Control Plane infrastructure state via a Cluster Operator and to block upgrades if the
    Control Plane infrastructure is degraded for any reason
  - The ability to restrict there to being a single source of truth for the Control Plane infrastructure definition
  - Restricting the replica count of the set to a supported number for the Control Plane within OpenShift
  - Ensuring that users aren't putting themselves in unsupported scenarios by trying to create additional Control Plane
    Machines
  - Ensuring Control Plane Machines are not removed when/if the `ControlPlaneMachineSet` is deleted

## Infrastructure Needed

For a clean separation of code, we will introduce the new operator in a new repository,
openshift/cluster-control-plane-machine-set-operator.
