---
title: dedicated-instances
authors:
  - "@alexander-demichev"
reviewers:
  - "@JoelSpeed"
  - "@enxebre"
approvers:
  - "@JoelSpeed"
  - "@enxebre"
creation-date: 2020-09-01
last-updated: 2020-09-01
status: provisional
---

# Dedicated instances

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Make it possible for users to create machines which run as dedicated instances. Dedicated instances are instances
that usually run on hardware that's dedicated to a single customer.

## Motivation

Some organizations need to make sure that their workloads are not hosted on the same physical hardware as others.

### Goals

- Provide automation similar to what Machine API supports for spot instances.

- Expose a field in the machine API that enables consumers to choose dedicated tenancy.

### Non-Goals

- TODO

## Proposal

In order to give users the ability to run their workloads on dedicated instances we should do the following things for AWS, GCP and Azure:

- Add ability to enable dedicated instances using Machine's provider spec.

- Validate that provider spec doesn't contain spot instances configuration and dedicated instances at the same time when it's not supported by the cloud provider. The only provider that currently supports this case is [AWS](https://aws.amazon.com/about-aws/whats-new/2017/01/amazon-ec2-spot-instances-now-support-dedicated-tenancy/#:~:text=Dedicated%20Spot%20instances%20work%20the,belong%20to%20other%20AWS%20accounts)

### Implementation Details

For each of the cloud providers that support dedicated instances the implementation will be different.

#### AWS

`Dedicated Instances are Amazon EC2 instances that run in a virtual private cloud (VPC) on hardware that's dedicated to a single customer.`. [AWS Documentation](https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/dedicated-instance.html). Each launched instance has a tenancy attribute and it can be configured similar to how we set availability zone.

```go
placement = &ec2.Placement{
  AvailabilityZone: aws.String(machineProviderConfig.Placement.AvailabilityZone),
  Tenancy: aws.String(machineProviderConfig.Placement.Tenancy)
}
```

That change will require adding `Tenancy` field to provider spec.

```go
type AWSMachineProviderConfig struct {
  // existing fields
  ...

  // Placement specifies where to create the instance in AWS
  Placement Placement `json:"placement"`
}

// Placement indicates where to create the instance in AWS
type Placement struct {
  // existing fields
  ...

  // Tenancy indicates tenant policy for instance
  // +kubebuilder:validation:Enum:=default,dedicated,host
  // +kubebuilder:default:=default
  Tenancy string
}

```

AWS provides support for spot instances on dedicated tenancy, we need to make sure that this case is also tested.
[AWS Documentation](https://aws.amazon.com/about-aws/whats-new/2017/01/amazon-ec2-spot-instances-now-support-dedicated-tenancy/#:~:text=Dedicated%20Spot%20instances%20work%20the,belong%20to%20other%20AWS%20accounts.)

#### Azure

In order to make dedicated VMs work on Azure we need to understand the concept of host groups and hosts.
[Azure documentation](https://docs.microsoft.com/en-us/azure/virtual-machines/windows/dedicated-hosts).

```text
A host group is a resource that represents a collection of dedicated hosts. You create a host group in a region and an availability zone, and add hosts to it.

A host is a resource, mapped to a physical server in an Azure data center. The physical server is allocated when the host is created. A host is created within a host group. A host has a SKU describing which VM sizes can be created. Each host can host multiple VMs, of different sizes, as long as they are from the same size series.

When creating a VM in Azure, you can select which dedicated host to use for your VM. You have full control as to which VMs are placed on your hosts.
```

The problem here are standard quotas: for host of type `DSv3-Type1` we can create only 32 VM of type `Standard_D2s_v3`(default type for worker VMs). To request a quota increase, the users are required to create a support request. This part should be well documented

The required API change is adding host name field `Host` to provider spec.

```go
type AzureMachineProviderConfig struct {
  // existing fields
  ...

  // Host name of physical server that hosts the virtual machine
  // +optional
  Host string
}
```

#### GCP

GCP requires `Node Templates` and `Node Groups` to be able to create dedicated instances. [GCP documentation](https://cloud.google.com/compute/docs/nodes/sole-tenant-nodes).

```text
Node templates

A node template is a regional resource that defines the properties of each node in a node group. 

Node groups and VM provisioning

Sole-tenant node templates define the properties of a node group, and you must create a node template before creating a node group in a Google Cloud zone. When you create a group, specify the maintenance policy for VM instances on the node group, and the number of nodes for the node group. 
A node group can have zero or more nodes; for example, you can reduce the number of nodes in a node group to zero when you don't need to run any VM instances on nodes in the group, or you can enable the node group autoscaler to manage the size of the node group automatically.
```

In order to be able to create a VM on a dedicated host we should introduce `NodeGroup` API field to provider spec.

We should document that node groups have resource capacities which limit the number of VMs, unless the node group autoscaler is enabled.

```text
type GCPMachineProviderConfig struct {
  // existing fields
  ...

  // NodeGroup name of node group that hosts the virtual machine
  // +optional
  NodeGroup string
}
```

### Risks and Mitigations

- Different quotas issues in Azure. See [this](#azure) section for more details.
- Azure and GCP will require some [configurations](#infrastructure-needed) on cloud provider side to be made before creating a dedicated instance.

#### Autoscaling

Autoscaling dedicated instances can be a problem because dedicated hosts have quotas and limits on provider side. We should provide good documentation here.

#### Limited resources for Azure and GCP

To avoid misconfiguration we should document that users are responsible for capacity of their dedicated host on Azure and node groups on GCP

### User Stories

## Design Details

### Open Questions

### Test Plan

### Graduation Criteria

#### Examples

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

The feature will go to GA without tech preview

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

PR with aws implementation https://github.com/openshift/cluster-api-provider-aws/pull/360

## Drawbacks

## Alternatives

## Infrastructure Needed

- For GCP, our CI environment should have proper `Node Template` and `Node Group` created.

- For Azure, our CI environment should have `Host Group` and `Host` created.

