---
title: aws-efa-networking
authors:
  - "@JoelSpeed"
reviewers:
  - "@elmiko"
  - "@deads2k"
  - "@sdodson"
  - "@danwinship"
approvers:
  - "@deads2k"
  - "@sdodson"
  - "@knobunc"
api-approvers:
  - "@deads2k"
creation-date: 2021-12-15
last-updated: 2021-12-15
tracking-link:
  - https://issues.redhat.com/browse/OCPCLOUD-1353
---

# AWS EFA Networking

## Summary

AWS Elastic Fabric Adapter (EFA) is a feature within EC2 that allows for network adapters that provide high throughput
for HPC applications communicating via MPI to be attached to certain instance types. With the presence of these network
adapters and some additional drivers, workloads can take advantage of greatly improved network performance between
nodes within the same AWS availability zone.

## Motivation

We have customers already using this feature of EC2 within OpenShift UPI clusters. They have taken on the responsibility
of making sure the prerequisites are fulfilled and have manually created instances with the attached adapters.
However, there is a desire to enable this feature to be used in conjunction with existing autoscaling features within
OpenShift. To enable this, we must provide support for the feature within the Machine API.

### Goals

- Enable Non Control Plane Machines to attach EFA network interfaces as their primary network interface
- Enable autoscaling of Machines that use EFA network interfaces
- Ensure that RHCOS contains the required kernel module (efa) to support the new network interface type
- Enable a mixture of MachineSet types for both EFA and non-EFA enabled instances

### Non-Goals

- Support and/or configuration of the prerequisite requirements for using EFA
  - Configuration of security groups to allow the inter-node traffic
  - Deployment of any Libfabric/MPI software prerequisites
  - Configuration of Huge Pages
- Allowing day 1 worker Machine configuration for EFA
- Allowing Control Plane Machines to be configured for EFA
- Support and/or configuration of other HPC related features in AWS
  - Eg. Placing Machines into placement groups

## Proposal

Introduce a new field to the AWS Machine ProviderSpec to allow users to opt-in to using the EFA network interface type.
Existing behaviour will be maintained by defaulting the interface type to the existing standard interface type, known
within AWS as `interface`.

### User Stories

- As a user of OpenShift, I want to be able to autoscale new compute capacity for my high performance networking
workloads so that I do not have to manually create new EC2 instances when my cluster reaches capacity.

### API Extensions

The following new field will be added to the AWSMachineProviderConfig struct.
The interface will be an enum allowing two possible values, `EFA` and `ENA`.
When no value is specified, we will use the `ENA` type to maintain backwards compatibility.

`EFA` will map to the AWS `efa` value. `ENA` will map to the AWS `interface` value.

```go
type AWSMachineProviderConfig struct {
  // Existing fields will not be modified
  ...

  // NetworkInterfaceType specifies the type of network interface to be used for the primary
  // network interface.
  // Valid values are "ENA", "EFA", and omitted, which means no opinion and the platform
  // chooses a good default which may change over time.
  // The current default value is "ENA".
  // Please visit https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/efa.html to learn more
  // about the AWS Elastic Fabric Adapter interface option.
  // +kubebuilder:validation:Enum:="ENA";"EFA"
  // +optional
  NetworkInterfaceType AWSNetworkInterfaceType `json:"networkInterfaceType,omitempty"`
}

// AWSNetworkInterfaceType defines the network interface type of the the
// AWS EC2 network interface.
type AWSNetworkInterfaceType string

const (
	// AWSENANetworkInterfaceType is the default network interface type,
	// the EC2 Elastic Network Adapter commonly used with EC2 instances.
	// This should be used for standard network operations.
	AWSENANetworkInterfaceType AWSNetworkInterfaceType = "interface"
	// AWSEFANetworkInterfaceType is the Elastic Fabric Adapter network interface type.
	AWSEFANetworkInterfaceType AWSNetworkInterfaceType = "efa"
)
```

### Implementation Details/Notes/Constraints

To enable Machine API to create EFA enabled instances, the above API field will need to be added to the AWS Machine
ProviderConfig. Once that field is added, if set by a user, we will set the interface to the correct value while
creating new instances.

```go
  if machineProviderConfig.NetworkInterfaceType != "" {
		networkInterfaces[0].InterfaceType = aws.String(string(machineProviderConfig.NetworkInterfaceType))
	}
```

It is important to note that OpenShift only supports a single network interface on instances created by Machine API
today, as such, the EFA network interface will be the primary network interface for the instance.
AWS also only supports EFA on the primary network interface.

#### What is EFA?

To quote the [AWS documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/efa.html):

> An Elastic Fabric Adapter (EFA) is a network device that you can attach to your Amazon EC2 instance to accelerate
  High Performance Computing (HPC) and machine learning applications. EFA enables you to achieve the application
  performance of an on-premises HPC cluster, with the scalability, flexibility, and elasticity provided by the AWS
  Cloud.
>
> EFA provides lower and more consistent latency and higher throughput than the TCP transport traditionally used in
  cloud-based HPC systems. It enhances the performance of inter-instance communication that is critical for scaling HPC
  and machine learning applications. It is optimized to work on the existing AWS network infrastructure and it can
  scale depending on application requirements.

### Risks and Mitigations

#### This feature cannot be leveraged for control plane instances

Due to limitations of the libraries that the installer uses to create control plane machines, there is currently no
way in which we can enable the control plane machines to be created day 0 with the EFA network interface type.

As this feature is only intended to be used with specific high performance workloads, which should not be running on
control plane hosts, we do not expect end users to require EFA network interfaces for their control plane machines.

#### Users will have to configure their own security groups

To allow the feature to work, the security groups that contain the Machines must be configured to allow all traffic
between the hosts within the security group. This is not configured by default on an IPI cluster.
Users must manually make this change via the AWS console/cli.
This prerequisite should be documented as a manual step to enable the feature.

Additionally, as we have future plans to manage security groups for IPI clusters, these manual changes to the security
groups may be difficult to adopt into the future management system. However, there is nothing to stop users of IPI
clusters making changes such as this already, so this kind of change must be considered for any adoption plans already.

Additionally, IPI users are free to add additional security groups to their clusters post install and specify these
additional groups within their Machine specs. We have no control over this today and again, this is something that must
be planned for in the future security group management process.
It is worth noting that today we do not guarantee the shape of security groups created by the installer and as such,
any user automating changes to such security groups is already exposing themself to potential compatibility issues with
future versions of OCP.

#### Users will have to configure Huge Pages

Huge Pages is a prerequisite requirement for deploying MPI workloads which leverage the EFA network interface.
It is expected that users will be responsible for configuring this within their OpenShift clusters post installation.

We [already document](https://docs.openshift.com/container-platform/4.9/post_installation_configuration/node-tasks.html#configuring-huge-pages_post-install-node-tasks)
how to configure Huge Pages within OpenShift clusters as a day 2 operation and it is expected that users of this
feature will follow the instructions linked above.

For completeness, an example configuration is included below:

```yaml
apiVersion: tuned.openshift.io/v1
kind: Tuned
metadata:
  name: hugepages
  namespace: openshift-cluster-node-tuning-operator
spec:
  profile:
  - data: |
      [main]
      summary=Boot time configuration for hugepages
      include=openshift-node
      [bootloader]
      cmdline_openshift_node_hugepages=hugepagesz=2M hugepages=10256M
    name: openshift-node-hugepages

  recommend:
  - machineConfigLabels:
      machineconfiguration.openshift.io/role: "worker"
    priority: 30
    profile: openshift-node-hugepages
```

Note, as the Huge Pages configuration is tuned to the size of the instance types leveraged within the cluster,
it is hard to predict an appropriate default. Due to this limitation, we leave this configuration as an exercise
for the end user.

A `2M` page size is the minimum requirement for the MPI operator.
The number of Huge Pages must fit within the hosts memory.

#### Using EFA requires additional software

The EFA network interface types require special drivers to enable them to be used.
Fortunately, the RHEL kernel already includes the relevant kernel modules to allow the adapter to be used transparently
as a standard network adapter within user space. This means that no additional configuration is required within RHCOS
to allow EFA instances to be used as regular OpenShift nodes, existing networking is unaffected.

To take advantage of the EFA performance benefits, additional libfabric software must be deployed.
This software is currently [distributed by AWS](https://github.com/aws-samples/aws-efa-eks/blob/main/manifest/efa-k8s-device-plugin.yml)
and it will be up to the end user to deploy this software within their clusters.
Red Hat will not initially support the libfabric driver.

Once the end user has correctly configured the software prerequisites, the Node Feature Discovery operator will
configure the Node status to show the EFA interface as an allocatble resource.
This then allows the MPI operator to start scheduling MPI workloads onto the Node.

As far as we can tell, enabling processeses to take advantage of the libfabric networking is transparent to the
standard networking stack and will not interrupt existing communication from user space.

#### Only a subset of instance types are supported with EFA network interfaces

Only a small subset of AWS instance types (eg. m5dn.24xlarge, m5dn.metal) are supported for use with EFA interfaces.
If a user were to specify an EFA interface type with a non supported instance (eg. m5.xlarge), then the instance will
fail to create. In this scenario, AWS rejects the RunInstance call from Machine API which causes Machine API to mark the
Machine as `Failed`. An error message will be added to the Machine status to signal to the user that their configuration
was invalid.

> error launching instance: EFA interfaces are not supported on m5.2xlarge

#### The Cluster Autoscaler has not been thoroughly tested with EFA instances

We have performed basic testing of the Cluster Autoscaler with MPI workloads requesting EFA interfaces.
We are now aware that the Cluster Autoscaler is aware of additional resource requests beyond the standard (CPU/Mem/GPU)
and will correctly report that an MPI workload cannot be scheduled to a node if it does not have the EFA interface as
an allocatable resource.

```bash
I1221 12:12:41.454239       1 scale_up.go:300] Pod osu-efa-test-intel-worker-1 can't be scheduled on MachineSet/openshift-machine-api/efa-test-n42zl-worker-us-east-1a, predicate checking error: Insufficient vpc.amazonaws.com/efa; predicateName=NodeResourcesFit; reasons: Insufficient vpc.amazonaws.com/efa; debugInfo=
```

When Machines with an EFA interface exist within the cluster, the Autoscaler can also correctly identify these (and
their respective MachineSets) and scale up and scale down the MachineSet as expected.

However, as we have seen with GPU autoscaling, there may be some issues that we need to fix within the autoscaler to
make sure the autoscaling is smooth and works as expected. We have not yet tested whether these issues manifest, though
these are the kind of issues we can expect given our experience with GPU resources.

##### A need for a dedicated EFA processor

As with GPUs, EFA allocatable resources are discoverd by the Node Feature Discovery operator.
This operator adds the EFA device as an allocatable resource to Nodes some time after the Node is created.

When there is a delay in adding this allocatable resource to the Node, and with certain configurations of the cluster
autoscaler, this can cause the Autoscaler to scale the Node down as it thinks that the new Node doesn't satisfy the
requirements of the unscheduled pods.

To avoid this, users can either disable scale down on the ClusterAutoscaler resource or set a sufficiently long
`delayAfterAdd` on the scale down configuration. We expect 10 minutes should be sufficient in most cases.

##### Scaling from zero is not implemented

To allow scale from zero, the Cluster Autoscaler Operator annotates MachineSets with information about the resources
the Machines within the MachineSet provide when they are created. Currently we annotate the CPU, Memory and GPU count
for instance types based on a mapping generated from the AWS API.

To enable EFA scaling from zero, we will also need to come up with a new annotation to signal to the Cluster Autoscaler
CAPI provider that the Machines within the Machineset can satisfy the EFA resource requirement.

## Design Details

### Open Questions

- What support will we provide in the future?
  - Will we be able to provide full end to end support for the required libfabric drivers?
- How can the feature be used by end users?
  - How do they signal to OpenShift that a particular workload should leverage the EFA feature?

### Test Plan

In the first phase, we are planning to just provide users the ability to create Machines with the EFA interface type.
We will run standard regression testing on the feature but we do not expect any special additional testing to be
performed.

There are two levels of testing that we can then automate within the origin test suite.

#### Smoke test for basic networking

To ensure that enabling an EFA interface does not break basic networking requirements,
we will add a test performs the following steps:

- Create a new MachineSet based on the existing MachineSets within the cluster
  - Modify the MachineSet to enable EFA networking
  - Set the MachineSet replicas to 1
- Wait for the MachineSet to create a Machine
- Wait for the Machine to become a Node
- Remove the MachineSet once the Machine has a Node
- Wait for the Machine to be removed

This process will test whether the basic Node networking is affected by the introduction of the EFA adapter.
As the EFA adapter requires a specific kernel module to provide basic networking, this will also provide signal as to
the inclusion of the `efa` kernel module with the RHCOS image.

#### Extended MPI testing

Based on the [POC project](https://github.com/kwozyman/ocp-aws-efa-poc/tree/main/manifests/),
we will deploy the Libfabric DaemonSet, MPI operator and an MPI latency job to determine whether or not the end to end
flow of creating an HPC workload is successful.

This process will rely on configuration of the preqrequisites for the feature which currently are non-trivial to
perform within the test suite. This may delay the addition of this test as we work out how to, for example, set the
correct security group configuration from within a test.

### Graduation Criteria

The addition of an API field to Machine API implies that the feature is GA from the beginning, no graduation criteria
are required.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

No features will be removed as a part of this proposal

### Upgrade / Downgrade Strategy

Existing clusters being upgraded will not be configured for the new feature and will continue to use the existing
standard interface type.

Once configured, on downgrade, the Machine API components will not know about the new fields, and as such, will ignore
the interface type field if specified. The usage of an EFA interface type will not affect removal of Machines after a
downgrade, there should be no detrimental effect of a downgrade on Machine API.
Machines created with the new interface type will be unaffected and will persist within the cluster until an
administrator decides to remove them.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

The Machine API changes are implemented within:
- https://github.com/openshift/api/pull/1065
- https://github.com/openshift/machine-api-provider-aws/pull/8

## Drawbacks

This feature is not easily usable by a generic user. There are a number of risks outlined in the risk section that
provide reasons for not implementing this feature, to summarise:
- End users must modify security group rules before they can use this feature
- End users must deploy the libfabric software onto OpenShift before they can use this feature
- We cannot supply this feature for control plane machines without replacing the control plane machines as a day 2
  operation

## Alternatives

The alternative to adding support for EFA interfaces on MAPI is to reject the RFE. In this case, the end user must use
some method outside of OpenShift to attach new instances to their clusters. This prevents the user from leveraging the
Machine API and the autoscaling integrations that we have built within the product.

### Future implementation

In the future, we may wish to implement a full `cluster-hpc-operator` to be responsible for the end to end
configuration of the prerequisites for HPC workloads. This would include configuring EFA and it's prerequisites as well
as possible other configuraiton such as placement groups.

The solutions outlined in this enhancement are currently AWS specific, however, the problems are not unique to AWS and
other platforms provide similar features which we will want to implement in the future.
A generic operator would be expected to be able to handle this feature across multiple platforms (eg Azure and GCP).

This is currently considered to be out of scope within this enhancement.

## Infrastructure Needed

N/A
