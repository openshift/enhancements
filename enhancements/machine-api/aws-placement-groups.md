---
title: aws-placement-groups
authors:
  - "@JoelSpeed"
reviewers:
  - "@sdodson"
  - "@deads2k"
approvers:
  - "@sdodson"
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
  - "@deads2k"
  - "@Miciah"
creation-date: 2022-01-05
last-updated: 2022-01-05
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - TBD
---

# AWS Placement Groups

## Summary

This enhancement describes the process of integrating [AWS Placement Groups](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/placement-groups.html#placement-groups-partition)
into the OpenShift Machine API.

Placement Groups allow users to define how groups of Machines should be placed within the Availability Zone.
For example, they can be clustered together within the same network spine, or separated for redundancy.

## Motivation

By adding support for Placement Groups within Machine API, end users will have finer control over the topology of their
machines and will be able to leverage the benefits that different placement group strategies bring.

### Goals

- Allow users of Machine API to create a MachineSet on AWS with either a Cluster, Partition or Spread Placement Group
- Allow usage of existing Placement Groups across 1 or more MachineSet
  - To allow using different instance types within a placement group
  - To allow using the same placement group across multiple availability zones
  - To allow control of machines/workloads across partitions of a Partition placement group
- Create Placement Groups based on user configuration when they do not exist
- Remove Placement Groups when they are no longer required
- Ensure Placement Groups can be removed during the cluster deprovisioning process
- Allow appropriate usage of Partition Placement Groups across multiple MachineSets.

### Non-Goals

- Allowing this feature to be leveraged during installation (day 0)
  - Users will be able to modify worker MachineSets after manifest creation if required but this will not be exposed as
    a part of the installation configuration before manifest generation
- Support for Placement Groups for Control Plane Machines
  - Users may replace their control plane at a later date using future automation if they wish to have their control
    plane within a placement group
- Support for moving Machines between placement groups

## Proposal

We will introduce a new CRD, `AWSPlacementGroup` within the `machine.openshift.io/v1` API group and a new
object reference within the `Placement` section of the AWS Machine Provider Config which will allow users
to specify which placement group to create instances within.

The placement group must exist before a Machine can be created, the new CRD will need to be present as a prerequisite
to creating the new Machines.

We will also allow end users to specify which partition their instances should be created within when appropriate.

### User Stories

- As an OpenShift cluster administrator, I would like to use a spread placement group to ensure that my critical
  workloads are unlikely to suffer from hardware failures simultaneously.
- As an operator of High-Performance Computing workloads on OpenShift, I would like to use a cluster placement group to
  ensure that there is
  minimal latency between instances within my cluster, allowing maximal throughput of my workloads.
- As an operator of a large scale distributed and replicated data store, I would like to use a partition placement
  group to ensure that different shards of my data are replicated within different fault domains within the same
  availability zone.
- As an OpenShift cluster administrator, I would like unneeded placement groups to be cleaned up so that I am not
  polluting my environment with resources that are no longer in use.

### API Extensions

The new fields for this enhancement will be split between the `Placement` struct within the `AWSMachineProviderConfig`
in the Machine v1beta1 API group and a new CRD in the Machine v1 API group.

The changes to the `AWSMachineProviderConfig` outlined below:

```golang
type Placement struct {
  // Existing Fields: Region, AvailabilityZone and Tenancy
  ...

  // Group specifies a reference to an AWSPlacementGroup resource to create the Machine within.
	// If the group specified does not exist, the Machine will not be created and will enter the failed phase.
	// +optional
	Group LocalAWSPlacementGroupReference `json:"group,omitempty"`

	// PartitionNumber specifies the numbered partition in which instances should be launched.
	// It is recommended to only use this value if multiple MachineSets share
	// a single Placement Group, in which case, each MachineSet should represent an individual partition number.
	// If unset, when a Partition placement group is used, AWS will attempt to
	// distribute instances evenly between partitions.
	// If PartitionNumber is set when used with a non Partition type Placement Group, this will be considered an error.
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Maximum:=7
	// +optional
	PartitionNumber int32 `json:"number,omitempty"`
}

// LocalAWSPlacementGroupReference contains enough information to let you locate the
// referenced AWSPlacementGroup inside the same namespace.
// +structType=atomic
type LocalAWSPlacementGroupReference struct {
	// Name of the AWSPlacementGroup.
	// +kubebuilder:validation:=Required
	Name string `json:"name"`
}
```

The additions for the new CRD are outline below:

```golang
type AWSPlacementGroupSpec struct {
	// AWSPlacementGroupManagementSpec defines the configuration for a managed or unmanaged placement group.
	// +kubebuilder:validation:Required
	ManagementSpec AWSPlacementGroupManagementSpec `json:"managementSpec"`

	// CredentialsSecret is a reference to the secret with AWS credentials. The secret must reside in the same namespace
	// as the AWSPlacementGroup resource. Otherwise, the controller will leverage the EC2 instance assigned IAM Role,
	// in OpenShift this will always be the Control Plane Machine IAM Role.
	// +optional
	CredentialsSecret *LocalSecretReference `json:"credentialsSecret,omitempty"`
}

// AWSPlacementGroupManagementSpec defines the configuration for a managed or unmanaged placement group.
// +union
type AWSPlacementGroupManagementSpec struct {
	// ManagementState determines whether the placement group is expected
	// to be managed by this CRD or whether it is user managed.
	// A managed placement group may be moved to unmanaged, however an unmanaged
	// group may not be moved back to managed.
	// +kubebuilder:validation:Required
	// +unionDiscriminator
	ManagementState ManagementState `json:"managementState"`

	// Managed defines the configuration for the placement groups to be created.
	// Updates to the configuration will not be observed as placement groups are immutable
	// after creation.
	// +optional
	Managed *ManagedAWSPlacementGroup `json:"managed,omitempty"`
}

// AWSPlacementGroupType represents the valid values for the Placement GroupType field.
type AWSPlacementGroupType string

const (
	// AWSClusterPlacementGroupType is the "Cluster" placement group type.
	// Cluster placement groups place instances close together to improve network latency and throughput.
	AWSClusterPlacementGroupType AWSPlacementGroupType = "Cluster"
	// AWSPartitionPlacementGroupType is the "Partition" placement group type.
	// Partition placement groups reduce the likelihood of hardware failures
	// disrupting your application's availability.
	// Partition placement groups are recommended for use with large scale
	// distributed and replicated workloads.
	AWSPartitionPlacementGroupType AWSPlacementGroupType = "Partition"
	// AWSSpreadPlacementGroupType is the "Spread" placement group type.
	// Spread placement groups place instances on distinct racks within the availability
	// zone. This ensures instances each have their own networking and power source
	// for maximum hardware fault tolerance.
	// Spread placement groups are recommended for a small number of critical instances
	// which must be kept separate from one another.
	// Using a Spread placement group imposes a limit of seven instances within
	// the placement group within a single availability zone.
	AWSSpreadPlacementGroupType AWSPlacementGroupType = "Spread"
)

// ManagedAWSPlacementGroup is a discriminated union of placement group configuration.
// +union
type ManagedAWSPlacementGroup struct {
	// GroupType specifies the type of AWS placement group to use for this Machine.
	// This parameter is only used when a Machine is being created and the named
	// placement group does not exist.
	// Valid values are "Cluster", "Partition", "Spread".
	// This value is required and, in case a placement group already exists, will be
	// validated against the existing placement group.
	// Note: If the value of this field is "Spread", Machines created within the group
	// may no have placement.tenancy set
	// to "dedicated".
	// +kubebuilder:validation:Enum:="Cluster";"Partition";"Spread"
	// +unionDiscriminator
	// +kubebuilder:validation:Required
	GroupType AWSPlacementGroupType `json:"groupType,omitempty"`

	// Partition defines the configuration of a partition placement group.
	// +optional
	Partition *AWSPartitionPlacement `json:"partition,omitempty"`
}

// AWSPartitionPlacement defines the configuration for partition placement groups.
type AWSPartitionPlacement struct {
	// Count specifies the number of partitions for a Partition placement
	// group. This value is only observed when creating a placement group and
	// only when the `groupType` is set to `Partition`.
	// Note the partition count of a placement group cannot be changed after creation.
	// If unset, AWS will provide a default partition count.
	// This default is currently 2.
	// Note: When using more than 2 partitions, the "dedicated" tenancy option on Machines
	// created within the group is unavailable.
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Maximum:=7
	// +optional
	Count int32 `json:"count,omitempty"`
}

type AWSPlacementGroupStatus struct {
	// Conditions represents the observations of the AWSPlacementGroup's current state.
	// Known .status.conditions.type are: Ready, Deleting
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// ExpiresAt identifies when the observed configuration is valid until.
	// The observed configuration should not be trusted if this time has passed.
	// The AWSPlacementGroup controller will attempt to update the status before it expires.
	// +optional
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`

	// Replicas counts how many AWS EC2 instances are present within the placement group.
	// Note: This is a pointer to be able to distinguish between an empty placement group
	// and the status having not yet been observed.
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`

	// ManagementState determines whether the placement group is expected
	// to be managed by this CRD or whether it is user managed.
	// A managed placement group may be moved to unmanaged, however an unmanaged
	// group may not be moved back to managed.
	// This value is owned by the controller and may differ from the spec in cases
	// when a user attempts to manage a previously unmanaged placement group.
	// +optional
	ManagementState ManagementState `json:"managementState,omitempty"`

	// ObservedConfiguration represents the configuration present on the placement group on AWS.
	// +optional
	ObservedConfiguration ManagedAWSPlacementGroup `json:"observedConfiguration,omitempty"`
}
```

### Implementation Details/Notes/Constraints

#### Placement Group Strategies

AWS offers three different placement group strategies.
Each strategy is designed with a different use case in mind and each has its own caveats.
The details of each are fleshed out below.

##### Cluster Placement Groups

A Cluster placement group is a grouping of instances within a single Availability zone.
Cluster placement groups are used for workloads that require higher per-flow throughput limits for TCP/IP traffic and
as such, are placed in the same high-bisection bandwidth segment of the network.

Cluster placement groups are recommended for applications that benefit from low network latency, high network
throughput, or both.
In particular, we expect that these will be used in conjunction with EFA network interfaces in HPC applications.

When using Cluster placement groups, AWS recommends to launch all instances as the same instance type and if
possible, to launch instances simultaneously to prevent the likelihood that capacity limits are reached within the
placement group.

Note: Machine API launches instances sequentially and is therefore unable to meet the recommendation set by AWS.
Detail about this can be found in the [Risks and Mitigations](#adding-new-instances-to-placement-groups-may-cause-capacity-errors)
section below.


###### Limitations of Cluster Placement Groups

- The following instance types are supported:
  - Current generation instances, except for burstable performance instances (for example, T2) and Mac1 instances.
  - The following previous generation instances: `A1`, `C3`, `cc2.8xlarge`, `cr1.8xlarge`, `G2`, `hs1.8xlarge`, `I2`,
    and `R3`.
- A cluster placement group can't span multiple Availability Zones.
- You can launch multiple instance types into a cluster placement group. However, this reduces the likelihood that the
  required capacity will be available for your launch to succeed. We recommend using the same instance type for all
  instances in a cluster placement group.

##### Spread Placement Groups

A Spread placement group is a grouping of instances all placed in distinct racks within the datacenter.
This ensures that each instance within the group has its own network and power source, maximising resilience to
hardware failures.

Spread placement groups are recommended for launching a small number of instances that must be kept separate, eg a
cluster of etcd instances.

As Spread placement groups keep instances in different racks, they can be expanded over time and can also mix instance
types without detriment.

Spread placement groups have a limited capacity. You can only launch seven instances within a Spread placement group
within a single availability zone.

###### Limitations of Spread Placement Groups

- Spread placement groups are not supported for AWS Dedicated Instances.

##### Partition Placement Groups

A Partition placement group divides your instances into logical segments called partitions.
Each partition is assigned a set of racks within the Availability Zone, each rack in turn having its own network and
power source.
No rack within the AZ will appear in more than one partition in the placement group.

Users may either create instances and have them spread automatically across partitions or they may allocate instances
to a particular partition.
Partitions are identified numerically, indexed from 1.

Partition placement groups are recommended for deploying large distributed and replicated workloads across distinct
racks.

Partition placement groups are limited to seven partitions per Availability Zone.

When using a Partition placement group, if users wish to control the placement of their workloads within Partitions,
they will need to create multiple MachineSets and assign each one to a partition.
Once they have assigned different MachineSets to partitions, they will be able to use Node affinity to assign the
workloads to the Nodes created by the Machines within the MachineSet by selecting based on the MachineSets' unique
labels.

For example, across three MachineSets, the Placement would vary only by the partition number:

```yaml
providerSpec:
  value:
    placement:
      region: us-east-1
      availabilityZone: us-east-1a
      group:
        name: my-partition-placement-group
      partitionNumber: 1 | 2 | 3 # This value will vary across the three MachineSets, either 1, 2 or 3
```

###### Limitations of Partition Placement Groups

- A partition placement group with AWS Dedicated Instances can have a maximum of two partitions.
  - If a user were to share a placement group between multiple MachineSets, which were a mixture
    of dedicated and non-dedicated instances, the dedicated instance limitations would apply to
    all MachineSets sharing the placement group.

#### Creation of Placement Groups

To enable users to create Placement Groups from within their OpenShift clusters, we will allow them to create the new
`AWSPlacementGroup` custom resource and reference this from the `placement.group.name` field on their MachineSet.
The referenced placement group will then be used for the placement of new Machines created by the MachineSet.

Within the `AWSPlacementGroup`, we will allow the user to specify the type of the placement group to create, either
Cluster, Spread or Partition, as well as, in the case of Partition, the number of partitions to be created.
The placement group name will match the name of the `AWSPlacementGroup`. Names must be unique for placement groups in
each AWS account and for the CR within Kubernetes, as such, we will only consider `AWSPlacementGroups` within the
`openshift-machine-api` namespace, as we presently do for Machines.

The type of placement group will be a required value on the `AWSPlacementGroup`. Due to the differences in behaviour
between the different placement group strategies, we do not believe there is a sensible default choice and as such, the
group type will be required when creating a placement group.

Should the user not define the number of partitions, we will omit this value in the request and leave AWS to default
the value.
Currently the default behaviour is to create 2 partitions if otherwise unspecified.

When creating the placement group, we will specify the `Name` tag as well as the standard cluster ID tag that is used
to identify resources belonging to a particular cluster.

As group names must be unique within a region, we should not fall foul of eventual consistency issues leading to
leaking of placement groups as we can do with EC2 instances.

When creating a new placement group, user defined tags from the Infrastructure object will be propagated onto the new
placement group. These tags will only be added on create and will not be reconciled in the future should the user
defined tag list be changed.

If the placement group is created successfully, the Ready condition will be set true. If there are any issues in
creating the placement group, the Ready condition will turn false and detail the error that occurred.

#### Removal of Placement Groups

To ensure we leave user accounts clean, we should remove any placement group that is no longer required.
If a user deletes a managed `AWSPlacementGroup`, we will remove it from AWS provided it has no instances registered
within it. To enable this clean up logic a finalizer will need to be added to each `AWSPlacementGroup` to allow the
controller to remove the placement group from AWS.

Should the controller be unable to remove the placement group (for example if it is not empty), it will not remove the
finalizer and instead, will set a `Deleting` condition on the placement group detailing the reason why it cannot remove
the placement group.

We will not delete unmanaged groups, it is up to the end user to remove any unmanaged placement group from AWS.

#### Reconciliation of placement groups

Whether the `AWSPlacementGroup` is managed or unmanaged, the controller will periodically check AWS for the
configuration of the placement group and reflect this in the status of the object. Since placement groups are immutable
in AWS after creation, we should not need to update this configuration after create.
Importantly, the controller will check that the observed configuration matches the desired configuration and set the
Ready condition based on this.
If the configuration does not match, Ready will be set false and will prevent the Machine controller from creating
new Machines within the placement group.

To ensure the data is not stale, we will reconcile `AWSPlacementGroup` resources whenever a Machine event occurs which
references the `AWSPlacementGroup`. This should ensure we have up to date information for the Machine controller to
observe while creating the new Machines.

#### General limitations of Placement Groups

- You can create a maximum of 500 placement groups per account in each Region.
- The name that you specify for a placement group must be unique within your AWS account for the Region.
- An instance can be launched in one placement group at a time; it cannot span multiple placement groups.
- You cannot launch AWS Dedicated Hosts in placement groups.

#### Installer changes

We do not plan to add Placement group configuration to the installer machine pool configuration.
Should users wish to launch the day 0 worker machines within a placement group, they will be able to modify the
installer generated machineset manifests before the cluster is created.
We do not intend to support placement groups for control plane machines at all at this point.

We will however need to update the installer cluster deprovisioning logic, as currently it does not know how to remove
a placement group if one is present within the cluster when the destroy operation is started.

If a user were to create an instance that is not managed by OpenShift within a placement group that is managed by
OpenShift, this will block the installer deprovision until such an instance is removed. This issue is known and is
already present within the installer in examples such as when a user creates an instance, not managed by OpenShift,  within a VPC that is managed by OpenShift.

#### Additional permissions

As this enhancement introduces new API calls to the Machine API, we will need to introduce 3 new permissions within the
CredentialsRequest.

```yaml
- ec2:DescribePlacementGroups
- ec2:CreatePlacementGroup
- ec2:DeletePlacementGroup
```

Existing mechanisms should allow these to be added and not cause issues for end users.
On upgrade, the CCO will block until users of manual mode, have acknowledged that they have included the new
permissions.
Users of mint mode will automatically have their IAM roles updated by the CCO.

### Risks and Mitigations

#### Adding new instances to placement groups may cause capacity errors

It is recommended for certain placement group strategies to launch instances using a single launch request. This
enables AWS to find the appropriate capacity within the datacenter to launch all of the required instances.

OpenShift does not support launching multiple instances at once and as such, we are exposed to adding instances to
placement groups one by one.
This increases the likelihood that we will exhaust the capacity within a placement group as we cannot advise AWS ahead
of time how many instances we need to fit within each placement group.

We currently have no way to mitigate this within Machine API.

If and when capacity limits are reached, Machines will enter a failed phase and an error message will be displayed
detailing that there was no capacity for the instance to be created.
Users may delete the Machine and attempt to recreate it later.

If using the Cluster Autoscaler, the Cluster Autoscaler will detect the failed launch and will, after some period (15
minutes by default), attempt to scale a different group of instances to fulfil the capacity gap, if it is able to.

We recommend that users leverage multiple MachineSets, each with their own Placement Group to minimise the likelihood
of exhausting the capacity of the Placement Group.
For example, in batch scenarios, users may wish to create a MachineSet per group of workloads. By separating each job
into their own Placement Group, the number of instances per Placement Group may be kept small while keeping the
benefits of grouping the workload placement in a cluster.

Having spoken with AWS about the possible limitations, an engineer responded with:

> The maximum number of instances in a placement group with a cluster strategy mainly depends on how many EC2 instances
are available and close together in the same availability zone. In the past, I have been able to use more than 1,000
c5.24xlarge instances in the same placement group with a cluster strategy.

This implies that this risk may not actually be that great and that users may not hit the issues we have proposed here.

#### Spread placement groups limit MachineSets to seven instances

Currently, a MachineSet is limited to acting across a single Availability Zone within AWS.
As a Spread placement group has a limit of seven instances per AZ, this then limits each MachineSet to 7 instances when
using a Spread placement group.

This may cause issues for autoscaling if users do not configure their autoscaling limits correctly.
For example, a user may specify their MachineAutoscaler to 8 instances, even though we know it will be impossible for
the 8th instance to launch.
In this scenario, were the autoscaler to scale the MachineSet to 8 instances, the final instance would never launch and
eventually the autoscaler should (dependent on configuration) scale the MachineSet back down and, if possible, attempt
a different MachineSet.

Unfortunately, there isn't currently a good way to signal to the autoscaler that this limitation exists, nor is there a
good way to signal back to the end user that the reason for the launch failures is the seven instance limit.

Using existing webhooks that inspect the Machine provider configuration, we can warn users if they create a MachineSet
with a Spread placement group and attempt to scale it over seven instances.
This however is not fool-proof, as the user may not have set the placement group type, or may have set it incorrectly
and be using an existing placement group not created by MAPI.
In this scenario the warning would not make sense, for this reason, we cannot make the warning a blocking error.

We may also wish to extend the Cluster Autoscaler Operator to inspect MachineAutoscalers and their targets.
If any MachineAutoscaler targeted a MachineSet with a spread placement group, it could warn if the maximum replica
count is set higher than 7. This however is non-trivial as there are currently no webhooks for the Cluster Autoscaler
Operator and we would have to teach it to inspect the different cloud provider specs in different ways, creating a
dependency on the different cloud provider codebases.

## Design Details

### Open Questions

N/A

### Test Plan

We will add a new test to the OpenShift Machine API E2E test suite that runs against both the Machine API Operator and
Machine API Provider AWS repositories.

As there should be little to no difference with regards to the different placement groups from the Machine API
perspective, we will only test one of the placement group types.

The test will:
- Create a new MachineSet with 2 replicas and a Partition Placement Group
- Ensure that the Machine instances are created successfully
- Remove the first Machine by scaling the MachineSet to 1 replica and wait for its removal
- Check there are no issue with the first Machine removal
- Check that the second Machine is still Running as expected
- Remove the MachineSet and wait for it the remaining Machine to be removed
- Delete and ensure that the Placement group has been removed

This will ensure that we have the appropriate permissions to perform the required actions for placement groups.

### Graduation Criteria

As with other features of this nature, this will become GA from the initial release. There will be no graduation
criteria.

#### Dev Preview -> Tech Preview

N/A as per the above.

#### Tech Preview -> GA

N/A as per the above.

#### Removing a deprecated feature

This enhancement does not describe the removal of a deprecated feature.

### Upgrade / Downgrade Strategy

As this is a new feature, we do not expect any issues during upgrades.
There will be additional permissions required by the Machine API provider AWS, but existing Cloud Credential Operator
upgrade gating should handle this in manual mode where it may cause issues.

On downgrade, if a user had created an instance in a placement group, the instance will remain functional. There should
be no immediate impact on the running cluster.
The only issue that may arise is that the Machine API provider will no longer know about the placement group that it
created.
In this case, when the last Machine is removed, we will orphan the placement group, it is expected that the end user
will be responsible for removing this in this scenario.

### Version Skew Strategy

There are no cross component dependencies and as such, we do not expect any cross version issues.

### Operational Aspects of API Extensions

We do not expect any need for operation of the API extensions introduced in this enhancement.

#### Failure Modes

The only known failure mode is when limits are reached for the number of instances within the placement groups. This is
already discussed in the Risks and Mitigations section.

This failure will prevent adding additional compute capacity to the cluster and in turn, will result in potentially
pending workloads.
As this feature is only supported on worker nodes, we do not expect any detrimental impact on the control plane.

#### Support Procedures

Based on the above, users will need to observe existing alerting from the Machine API which signals when workloads are
not able to schedule and when Machines are not provisioning correctly.

## Implementation History

Not yet available.

## Future Implementation

As the OpenShift Control Plane represents a suitable candidate for a Spread Placement Group, it would make sense in the
future to implement Placement Group support within the installer to allow users to opt-in (or even default to) running
the control plane instances within a Spread Placement Group.

The benefits of the Spread Placement Group are only valuable when there are multiple Control Plane Machines within a
single AZ which is not a common deployment model within OpenShift.
Typically an OpenShift cluster has 3 Control Plane Machines and every AWS region (so far) has at least 3 Availability
Zones in which the Control Planes are spread out.
Spreading Control Plane Machines over multiple Availability Zones provides better redundancy guarantees than using a
Placement Group within a single Availability Zone.

## Drawbacks

We do not currently plan to implement this feature within the installer. This means that it cannot be leveraged for
control plane machines and as such, there will be discrepancies between the capability of control plane and worker
machines.
This may cause confusion for users as they may wish to use the feature for the control plane instances as well.
To mitigate this, users may, as a day 2 operation, replace their control plane instances using the already documented
recovery procedures.

## Alternatives

There is no alternative feature within AWS that allows users to place groups of instances in different ways. If we wish
to allow users to have control over the placement of their instances on AWS, we must support placement groups.

If we wanted to mitigate the issues around adding instances 1x1 to placement groups, we would need to introduce a new
Machine concept that represented multiple Machines at once.
This would be a major overhaul of Machine management within OpenShift and is not something we are willing to take on at
present.

### Embedding the placement group configuration within MachineSets

We previously considered creating placement groups directly from the Machine controller by embedding the configuration
for them within the Machine providerSpec. However, this could cause non-deterministic behaviour and as such was
scrapped.

In a case where an end user creates two MachineSets with differing placement group configuration but the same placement
group name, the design detailed that the first reconciled Machine would create the placement group based on its
configuration. Any later Machine (from the second MachineSet) would then enter a failed state because its configuration
differed from the configuration on AWS. This is undesirable and as such, we must concretely define each placement group
as its own resource.

## Infrastructure Needed

No new infrastructure will be required to fulfil this enhancement.
