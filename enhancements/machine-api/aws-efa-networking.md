---
title: aws-efa-networking
authors:
  - "@JoelSpeed"
reviewers:
  - "@elmiko"
  - "@deads2k"
  - TBD
approvers:
  - TBD
api-approvers:
  - "@deads2k"
creation-date: 2021-12-15
last-updated: 2021-12-15
tracking-link:
  - https://issues.redhat.com/browse/OCPCLOUD-1353
---

# AWS EFA Networking

## Summary

AWS Elastic-Fabric-Adapter (EFA) is a feature within EC2 that allows for high throughput network adapters to be
attached to certain instance types. With the presence of these network adapters and some additional drivers,
workloads can take advantage of greatly improved network performance between nodes within the same AWS availability
zone.

## Motivation

We have customers already using this feature of EC2 within OpenShift UPI clusters. They have taken on the responsibility
of making sure the prerequisites are fulfilled and have manually created instances with the attached adapters.
However, there is a desire to enable this feature to be used in conjunction with existing autoscaling features within
OpenShift. To enable this, we must provide support for the feature within the Machine API.

### Goals

- Enable worker Machines to attach EFA network interfaces as their primary network interface
- Enable autoscaling of Machines that leverage high performance network interfaces

### Non-Goals

- Support and or configuration of the prerequisite requirements for using EFA
  - Configuration of security groups to allow the inter-node traffic
  - Deployment of any Libfabric/MPI software prerequisites

## Proposal

Introduce a new field to the AWS Machine ProviderSpec to allow users to opt-in to using the EFA network interface type.
Existing behaviour will be maintained by defaulting the interface type to the existing standard interface type, known
within AWS as `interface`.

### User Stories

- As a user of OpenShift, I want to be able to autoscale new compute capacity for my high performance networking
workloads so that I do not have to manually create new EC2 instances when my cluster reaches capacity.

### API Extensions

The following new field will be added to the AWSMachineProviderConfig struct.
The interface will be an enum allowing two possible values, `efa` and `interface`.
When no value is specified, we will use the `interface` type to maintain backwards compatibility.

```go
type AWSMachineProviderConfig struct {
  // Existing fields will not be modified
  ...

  // NetworkInterfaceType specifies the type of network interface to be used for the primary
  // network interface.
  // Valid values are "interface", "efa", and omitted, which means no opinion and the platform
  // chooses a good default which may change over time.
  // The current default value is "interface".
  // Please visit https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/efa.html to learn more
  // about the AWS Elastic Fabric Adapter interface option.
  // +kubebuilder:validation:Enum:="interface";"efa"
  // +optional
  NetworkInterfaceType AWSNetworkInterfaceType `json:"networkInterfaceType,omitempty"`
}

// AWSNetworkInterfaceType defines the network interface type of the the
// AWS EC2 network interface.
type AWSNetworkInterfaceType string

const (
	// AWSInterfaceNetworkInterfaceType is the default network interface type.
	// This should be used for standard network operations.
	AWSInterfaceNetworkInterfaceType AWSNetworkInterfaceType = "interface"
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

#### Using EFA requires additional software

The EFA network interface types require special drivers to enable them to be used.
Fortunately, the RHEL kernel already includes the relevant kernel modules to allow the adapter to be used transparently
as a standard network adapter within user space. This means that no additional configuration is required within RHCOS
to allow EFA instances to be used as regular OpenShift nodes, existing networking is unaffected.

To take advantage of the EFA performance benefits, additional libfabric software must be deployed.
This software is currently [distributed by AWS](https://github.com/aws-samples/aws-efa-eks/blob/main/manifest/efa-k8s-device-plugin.yml)
and it will be up to the end user to deploy this software within their clusters.
Red Hat will not initially support the libfabric driver.

#### Only a subset of instance types are supported with EFA network interfaces

Only a small subset of AWS instance types (eg. m5dn.24xlarge, m5dn.metal) are supported for use with EFA interfaces.
If a user were to specify an EFA interface type with a non supported instance (eg. m5.xlarge), then the instance will
fail to create. In this scenario, AWS rejects the RunInstance call from Machine API which causes Machine API to mark the
Machine as `Failed`. An error message will be added to the Machine status to signal to the user that their configuration
was invalid.

> error launching instance: EFA interfaces are not supported on m5.2xlarge

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

In the future, if/when we introduce full support for this feature, we will need to introduce specific testing to check
that the workloads can leverage the EFA feature and that their networking throughput is increased when leveraging the
feature.

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

## Infrastructure Needed

N/A
