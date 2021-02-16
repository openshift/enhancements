---
title: spot-instances
authors:
  - "@JoelSpeed"
  - "@enxebre"
reviewers:
  - "@enxebre"
  - "@bison"
  - "@michaelgugino"
approvers:
  - "@enxebre"
  - "@bison"
  - "@michaelgugino"
creation-date: 2020-02-04
last-updated: 2020-06-26
status: implemented
see-also:
replaces:
superseded-by:
---

# Spot Instances

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Enable OCP users to leverage cheaper, non-guaranteed instances to back Machine API Machines.

## Motivation

Allow users to cut costs of running OCP clusters on cloud providers by moving interruptible workloads onto non-guaranteed instances.

### Goals

- Provide sophisticated provider-specific automation for running Machines on non-guaranteed instances

- Utilise as much of the existing Machine API as possible

- Ensure graceful shutdown of pods is attempted on non-guaranteed instances

### Non-Goals

- Any logic for choosing instances types based on availability from the cloud provider
- A one to one map for each provider available mechanism for deploying spot instances, e.g aws fleet.

## Proposal

To provide a consistent behaviour using non-guaranteed instances (Spot on AWS and Azure, Preepmtible on GCP)
across cloud providers, we must define a common behaviour based on the common features across each provider.

Based on the research on [non-guaranteed instances](#non-guaranteed-instances),
the following requirements for integration will work for each of AWS, Azure and GCP:

- Required configuration for enabling spot/preemptible instances should be added to the ProviderSpec
  - No configuration should be required outside of this scope
  - This enforces consistency across MachineSets, all Machines a MachineSet creates will either be Spot/Preemptible or On-Demand

- A Machine should be paired 1:1 with an instance on the cloud provider
  - If the instance is preempted/terminated, the cloud actuator should not replace it
  - If the instance is preempted/terminated, the cloud provider should not replace it

- The actuator is responsible for creation of the instance only and should not attempt to remediate problems

- The actuator should not attempt to verify that an instance can be created before attempting to create the instance
  - If the cloud provider does not have capacity, the Machine Health Checker can (given required MHC) remove the Machine after a period.
    MachineSet will ensure the correct number of Machines are created.

- If the Spot request cannot be satisfied when a Machine is created, the Machine will be marked as failed.
  This Machine would be remediated by an MHC if present.

- The actuator should label Machines as interruptible if they are spot/preemptible
  - This will allow termination handlers to be deployed to only spot/preemptible instances
  - The `machine.openshift.io/interruptible-instance` label will be set on `the Machine.Spec.Lables` if the instance is spot/preemptible

- Graceful termination of nodes should be provided by observing termination notices

### Implementation Details

#### Termination handler design

To enable graceful termination of workloads running on non-guaranteed instances,

#### Cloud Provider Implementation Specifics

##### AWS

###### Launching AWS instances

To launch an instance as a Spot instance on AWS, a [SpotMarketOptions](https://docs.aws.amazon.com/sdk-for-go/api/service/ec2/#SpotMarketOptions)
needs to be added to the `RunInstancesInput`. Within this there are 3 options that matter:

- InstanceInterruptionBehaviour (default: terminate): This must be set to `terminate` otherwise the SpotInstanceType cannot be `one-time`

- SpotInstanceType (default: one-time): This must be set to `one-time` to ensure that each Machine only creates on EC2 instance and that the spot request is

- MaxPrice (default: On-Demand price): This can be **optionally** set to a string representation of the hourly maximum spot price.
  If not set, the option will default to the On-Demand price of the EC2 instance type requested

The only option from this that needs exposing to the user from this is the `MaxPrice`, this option should be in an optional struct, if the struct is not nil,
then spot instances should be used, if the MaxPrice is set, this should be used instead of the default On-Demand price.

```go
type SpotMarketOptions struct {
  MaxPrice *string `json:”maxPrice,omitempty”`
}

type AWSMachineProviderConfig struct {
  ...

  SpotMarketOptions *SpotMarketOptions `json:”spotMarketOptions,omitempty”`
}
```

Once the instance is launched, the Machine will be labelled as an `interruptible-instance`
if the instance `InstanceLifecycle` field is set to `spot`.

###### Termination Notices
Termination notices on AWS are provided via the EC2 metadata service up to 2 minutes before an instance is due to be preempted.

A method should be implemented within the AWS actuator package that satisfies the requirements of the [Termination Pod](#termination-pod).
It will start a go routine to poll the EC2 metadata service and return a channel used to signify the instance is being terminated.
As soon as the polling loop receives an `OK` response (a non-terminated instance returns a `404` response),
it will close the channel allowing the [Termination Pod](#termination-pod) logic to trigger graceful draining.

##### GCP

###### Launching GCP instances

To launch an instance as Preemptible on GCP, the `Preemptible` field must be set:

```go
&compute.Instance{
  ...
  Scheduling: &comput.Scheduling{
    ...
    Preemptible: true,
  },
}
```

Therefore, to make the choice up to the user, this field should be added to the `GCPMachineProviderSpec`:

```go
type GCPMachineProviderSpec struct {
  ...
  Preemptible bool `json:”preemptible”`
}
```

Once the instance is launched, the Machine will be labelled as an `interruptible-instance`
if the instance `Scheduling.Preepmtible` field is set to `true`.

##### Azure

###### Launching Instances

To launch a VM as a Spot VM on Azure, the following 3 options need to be set within the [VirtualMachineProperties](https://github.com/Azure/azure-sdk-for-go/blob/8d7ac6eb6a149f992df6f0392eebf48544e2564a/services/compute/mgmt/2019-07-01/compute/models.go#L10274-L10309)
when the instance is created:

- Priority: This must be set to `Spot` to request a Spot VM

- Eviction Policy: This has two options, `Deallocate` or `Delete`. Only `Deallocate` is valid when using Spot VMs and as such, this must be set to `Deallocate`.

- BillingProfile (default: -1) : This is a struct containing a single field, `MaxPrice`.
  This is a float representation of the maximum price the user wishes to pay for their VM.
  This defaults to -1 which makes the maximum price the On-Demand price for the instance type.
  This also means the instance will never be evicted for price reasons as Azure caps Spot Market prices at the On-Demand price.

The only option that a user needs to interact with is the `MaxPrice` field within the `BillingProfile`, other fields only have 1 valid choice and as such can be inferred.
Similar to AWS, we can make an optional struct for SpotVMOptions, which, if present, implies the priority is `Spot`.

```go
type SpotVMOptions struct {
  MaxPrice *float64 `json:”maxPrice,omitempty”`
}

type AzureMachineProviderSpec struct {
  ...

  SpotVMOptions *SpotVMOptions `json:”spotVMOptions,omitempty”`
}
```

Once the instance is launched, the Machine will be labelled as an `interruptible-instance`
if the instance `Priority` field is set to `spot`.

###### Deallocation
Since Spot VMs are not deleted when they are preempted and instead are deallocated,
we will need to monitor for deallocated instances and manually cleanup the leftover resources (VM, disks, networking).
A MachineHealthCheck on Spot VMs should be capable of noticing that the machine has stopped and initiating a delete on the VM by the Machine actuator.

### Risks and Mitigations

#### Control-Plane instances

At present, if a control-plane node is terminated, manual intervention is required to replace the node.
Therefore, running control-plane machines on top of spot instances,
where the likelihood of being terminated is increased, should be forbidden.

This risk will be documented and it will be strongly advised that users do not attempt to create control-plane instances on spot instances.

#### Spot instances and Autoscaling

The Kubernetes Cluster Autoscaler, deployed to OpenShift clusters, is currently unaware of differences
between Spot instances and on-demand instances.

If, while the cloud provider has no capacity, or the bid price is too low for AWS/Azure,
the autoscaler were to try and scale a MachineSet backed by spot instances,
new Machines would be created, but instances on the cloud provider would not manifest and the Machines would enter a `Failed` state.
If a MachineHealthCheck were deployed, these Machines would be considered unhealthy and would be deleted, creating a new Machine in its place.

With or without an MHC, the scale request would never manifest in new compute capacity joining the cluster.
By default, after a half hour period, the autoscaler considers the MachineSet to be out of sync and rescales it to remove the `Failed` instances.
This period can be reduced by setting the `--max-node-provision-time` flag,
but it will always be twice as long as this flag's value.
At this point, it deems the original unschedulable Pods as unschedulable again and may attempt to rescale the same MachineSet.

Based on the [working implementation](https://github.com/kubernetes/autoscaler/pull/2235/files) of Spot instances in the AWS autoscaler provider,
if the autoscaler Machine API implementation were to provide fake provider IDs for Machines that have failed ([example](https://github.com/JoelSpeed/autoscaler/commit/11ebd1ffdadebbb20d2fac9aae30646b4f47dfa9)),
the autoscaler would deem these Machines as having unregistered nodes and, by default, after a 15 minute period,
would request these unregistered nodes be deleted, mark the MachineSet as unhealthy and attempt to scale an alternative MachineSet (This period can be reduced by setting the `--max-node-provision-time` flag).

By providing fake IDs, the autoscaler can track cloud instances that never got a node and apply it's health checking and backoff mechanisms for the node groups as it normally would have.

This, while still not perfect, is preferable to the current state of the autoscaler,
and brings the behaviour in line with that of the [AWS autoscaler implementation](https://github.com/kubernetes/autoscaler/pull/2235).
With this patch, assuming there is a on-demand based MachineSet to fall back to,
this would be tried as a backup once the spot MachineSet has failed.

##### Autoscaler scaling decisions

The autoscaler uses a scheduling algorithm to check which node groups are suitable for scaling.
If users wish to mix their nodes between on-demand and spot instances,
they can use [node affinity](https://kubernetes.io/docs/concepts/configuration/assign-pod-node/#node-affinity)
to ensure that some workloads would be scheduled on the on-demand instances and others on the spot instances.

When making scaling decisions, the autoscaler will take this affinity into consideration and will only scale
up nodes that match the requirements of the upcoming pods.

#### Machine Health Checks on Failed Machines

If there are any issues creating a instance with the provider, for example the request being rejected because the spot bid is too low,
the Machine controller will mark the Machine as `Failed`.
If a MachineHealthCheck matches this Machine, upon reconciling the MachineHealthCheck,
it will determine that the `Failed` machine is unhealthy and delete it immediately.
Assuming this Machine is part of a MachineSet, a new Machine will be created and,
given then price has not changed, will also be Marked as `Failed`.

This will create a hot loop where the MachineHealthCheck controller continually deletes `Failed` Machines,
triggering the `MachineSet` controller to create new `Failed` machines.

To mitigate this, the MachineHealthCheck controller should implement back-off or delays similar to that in the Cluster Autoscaler,
so that if a MachineHealthCheck has remediated a Machine recently, it should not attempt to remediate any Machines for a short period.

## Design Details

### Test Plan

### Graduation Criteria

#### Examples

##### Dev Preview -> Tech Preview

##### Tech Preview -> GA

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

Completely delegate the spot instance/feet behaviour to the provider in the fashion of [MachinePool](https://github.com/kubernetes-sigs/cluster-api/pull/1703).
This would be a more disruptive change since the current OCP design expects a one to one relation between machine resources, provider instances and kubernetes nodes.
This is particularly necessary for a kubelet to authenticate itself against the [machine approver](https://github.com/openshift/cluster-machine-approver) and join the cluster.

## Future work

### Termination Notices for GCP and Azure
Each cloud provider provides a pre-warning when it is going to terminate an instance.
In the first iteration of adding spot instance support, these will not be observed on GCP and Azure and workloads will be interrupted and rescheduled.

In the future, these notices should be used to mark Machines for deletion.
This will trigger the Machine controller to start gracefully moving workloads off of the affected machine and reduce the impact of the interruption.

## Related Research

### Non-Guaranteed instances

Behaviour of non-guaranteed instances varies from provider to provider.
With each provider offering different ways to create the instances and different guarantees for the instances.
Each of the following sections details how non-guaranteed instances works for each provider.

#### AWS Spot Instances

Amazon’s Spot instances are available to customers via three different mechanisms.
Each mechanism requires the user to set a maximum price (a bid) they are willing to pay for the instances and,
until either no-capacity is left, or the market price exceeds their bid, the user will retain access to the machine.

##### Spot backed Autoscaling Groups

Spot backed Autoscaling groups are identical to other Autoscaling groups, other than that they use Spot instances instead of On-Demand instances.

Since Autoscaling Groups are not currently support within the Machine API,
adding support for Spot backed Autoscaling Groups would require larger changes to the API and possibly the introduction of a new type
(eg. [MachinePool](https://github.com/kubernetes-sigs/cluster-api/pull/1703)).

##### Spot Fleet

Spot Fleets are similar to Spot backed Autoscaling Groups, but they differ in that there is no dedicated instance type for the group.
They can launch both On-Demand and Spot instances from a range of instance types available based on the market prices and the bid put forward by the user.

Similarly to Spot Back Autoscaling groups, there is currently no analogous type within the Machine API and as such,
implementing support for Spot Fleets would require something akin to a [MachinePool](https://github.com/kubernetes-sigs/cluster-api/pull/1703).

##### Singular Spot Instances
Singular Spot instances are created using the same API as singular On-Demand instances.
By providing a single additional parameter, the API will instead launch a Spot Instance.

Given that the Machine API currently implements Machine’s by using singular On-Demand instances,
adding singular Spot Instance support via this mechanism should be trivial.

##### Other AWS Spot features of note

###### Stop/Hibernate

Instead of terminating an instance when it is being interrupted,
Spot instances can be “stopped” or “hibernated” so that they can resume their workloads when new capacity becomes available.

Using this feature would contradict the functionality of the Machine Health Check remediation of failed nodes.
In cloud environments, it is expected that if a node is being switched off or taken away, a new one will replace it.
This option should not be made available to users to avoid conflicts with other systems within OCP.

###### Termination Notices for AWS Spot

Amazon provides a 2 minute notice of termination for Spot instances via it’s instance metadata service.
Each instance can poll the metadata service to see if it has been marked for termination.
There are [existing solutions](https://github.com/kube-aws/kube-spot-termination-notice-handler)
that run Daemonsets on Spot instances to gracefully drain workloads when the termination notice is given.
This is something that should be provided as part of the spot instance availability within Machine API.

###### Persistent Requests

Persistent requests allow users to ask that a Spot instance, once terminated, be replace by another instance when new capacity is available.

Using this feature would break assumptions in Machine API since the instance ID for the Machine would change during its lifecycle.
The usage of this feature should be explicitly forbidden so that we do not break existing assumptions.

#### GCP Preemptible instances

GCP’s Preemptible instances are available to customers via two mechanisms.
For each, the instances are available at a fixed price and will be made available to users whenever there is capacity.

##### Instance Groups

GCP Instance Groups can leverage Preemptible instances by modifying the instance template and setting preemptible option.

There currently is no analogous type to Instance Groups within the Machine API, however they could be modelled by something like a [MachinePool](https://github.com/kubernetes-sigs/cluster-api/pull/1703).

##### Single Instance

GCP Single Instances can run on Preemptible instances given the launch request specifies the preemptible option.

Given that the Machine API currently implements Machine’s by using single instances, adding singular Preemptible Instance support via this mechanism should be trivial.

##### Limitations of Preemptible

###### 24 Hour limitation

Preemptible instance will, if not already, be terminated after 24 hours.
This means that the instances will be cycled regularly and as such, good handling of shutdown events should be implemented.

###### Shutdown warning
GCP gives a 30 second warning for termination of Preemptible instances.
This signal comes via an ACPI G2 soft-off signal to the machine, which, could be intercepted to start a graceful termination of pods on the machine.
There are [existing projects](https://github.com/GoogleCloudPlatform/k8s-node-termination-handler) that already do this.

Alternatively, GCP provides a [metadata service](https://cloud.google.com/compute/docs/instances/create-start-preemptible-instance#detecting_if_an_instance_was_preempted)
that is accessible from the instances themselves that allows the instance to determine whether or not it has been preempted.
This is similar to what is provided by AWS and Azure and should be used to allow the termination handler implementation
to be consistent across the three providers.

In the case that the node is reaching its 24 hour termination mark,
it may be safer to preempt this warning and shut down the node before the 30s shut down signal.

#### Azure Spot VMs

Azure recently announced Spot VMs as a replacement for their Low-Priority VMs which were in customer preview through the latter half of 2019.
Spot VMs work in a similar manner to AWS Spot Instances. A maximum price is set on the instance when it is created, and, until that price is reached,
the instance will be given to you and you will be charged the market rate. Should the price go above your maximum price, the instance will be preempted.

Spot VMs are available in two forms in Azure.

##### Scale Sets

Scale sets include support for Spot VMs by indicating when created, that they should be backed by Spot VMs.
At this point, a eviction policy should be set and a maximum price you wish to pay.
Alternatively, you can also choose to only be preempted in the case that there are capacity constraints,
in which case, you will pay whatever the market rate is, but will be preempted less often.

There currently is no analogous type to Scale Sets within the Machine API, however they could be modelled by something like a [MachinePool](https://github.com/kubernetes-sigs/cluster-api/pull/1703).

##### Single Instances
Azure supports Spot VMs on single VM instances by indicating when created, that the VM should be a Spot VM.
At this point, a eviction policy should be set and a maximum price you wish to pay.
Alternatively, you can also choose to only be preempted in the case that there are capacity constraints,
in which case, you will pay whatever the market rate is, but will be preempted less often.

Given that the Machine API currently implements Machine’s by using single instances, adding singular Spot VM support via this mechanism should be trivial.

##### Important Spot VM notes

###### Termination Notices for Azure Spot

Azure uses their Scheduled Events API to notify Spot VMs that they are due to be preempted.
This is a similar service to the AWS metadata service that each machine can poll to see events for itself.
Azure only gives 30 seconds warning for nodes being preempted though.

A Daemonset solution similar to the AWS termination handlers could be implemented to provide graceful shutdown with Azure Spot VMs.

###### Eviction Policies

Azure Spot VMs support two types of eviction policy:

- Deallocate: This stops the VM but keeps disks and networking ready to be restarted.
  In this state, VMs maintain usage of the CPU quota and as such, are effectively just paused or hibernating.
  This is the *only* supported eviction policy for Single Instance Spot VMs.

- Delete: This deletes the VM and all associated disks and networking when the node is preempted.
  This is *only* supported on Scale Sets backed by Spot VMs.
