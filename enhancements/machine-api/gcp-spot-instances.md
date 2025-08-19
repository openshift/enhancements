---
title: gcp-spot-instances
authors:
  - "@de1987"
  - "@germanparente"
reviewers:
  - "@damdo"
  - "@sub-mod"
approvers:
  - "@damdo"
  - "@sub-mod"
creation-date: 2025-08-15
last-updated: 2025-08-15
status: draft
see-also:
replaces:
superseded-by:
---

# GCP Spot Instances

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement should enable OCP users to choose GCP spot instances for the Machine API.

## Motivation

At the current Machine API version, the user can only choose legacy preemptible instances on GCP. These instances are limited to 24 hours, and they're terminated. By using GCP spot instances, there is no time limit.

This comes from the feature request [RFE-3563](https://issues.redhat.com/browse/RFE-3563).


### Goals

- Support spot instances as an alternative to the legacy preemptible instances on GCP

- Utilize the existing mechanisms that are managing the legacy preemptible instances


### Non-Goals

- The configuration field `ProviderSpec.provisioningModel` should only allow `SPOT` option

- Do not allow set `ProviderSpec.provisioningModel` as `SPOT` and the `Preemptible` as `true` at the same time.

- The configuration field `ProviderSpec.provisioningModel` is optional


## Proposal

To support GCP spot instances, the features and mechanisms should use the previous implementation from other cloud providers and GCP itself, which currently only supports the legacy preemptible instances.

GCP spot instances are also preemptible instances as explained in the [GCP Preemptible Instances docs](https://cloud.google.com/compute/docs/instances/preemptible). GCP has two types of preemptible instances: the legacy and the spot instances. The difference between them are the new features and the time limit.

The mechanisms should already be implemented for the legacy preemptible instances. With that said, the GCP spot instances implementation should use or be based on the existing code implementation to simplify the feature. As a checklist let's use the old requirements to make sure everything will be working as expected for this feature, which are:

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
  - The `machine.openshift.io/interruptible-instance` label will be set on the `Machine.Spec.Labels` if the instance is spot/preemptible

- Graceful termination of nodes should be provided by observing termination notices

### Implementation Details

#### Termination handler

The termination handler is already implemented and it checks if the instance has the label `interruptible-instance`. If it does, the handler will make sure the instance will have a graceful termination.

Both GCP instance types, legacy preemptible and spot, have the same termination process based on the [Spot instances termination docs](https://cloud.google.com/compute/docs/instances/spot#preemption-process) and [Legacy preemptible instances termination docs](https://cloud.google.com/compute/docs/instances/preemptible#preemption-process). This means no changes are required in the termination handler, besides the instance having the label `interruptible-instance`.


#### Implementation Specifics


###### Launching GCP spot instances

To launch a spot instance on GCP, the `ProvisioningModel` field must be set:

```go
&compute.Instance{
  ...
  Scheduling: &compute.Scheduling{
    ...
    ProvisioningModel: r.providerSpec.ProvisioningModel,
  },
}
```

Therefore, to make the choice up to the user, this field should be added to the `GCPMachineProviderSpec`:

```go
type GCPMachineProviderSpec struct {
  ...
  ProvisioningModel GCPProvisioningModelType `json:”provisioningModel”`
}
```

Once the instance is launched, the Machine will be labelled as an `interruptible-instance`
if the instance `Scheduling.ProvisioningModel` field is set to `SPOT`.


### Risks and Mitigations


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


## Related Research


### Limitations of Preemptible

#### Legacy preemptible instance limitation

Legacy preemptible instances will, if not already, be terminated after 24 hours.
This means that the instances will be cycled regularly and as such, good handling of shutdown events should be implemented.

#### Shutdown warning
GCP gives a 30 second warning for termination of Preemptible instances.
This signal comes via an ACPI G2 soft-off signal to the machine, which, could be intercepted to start a graceful termination of pods on the machine.
There are [existing projects](https://github.com/GoogleCloudPlatform/k8s-node-termination-handler) that already do this.

Alternatively, GCP provides a [metadata service](https://cloud.google.com/compute/docs/instances/create-start-preemptible-instance#detecting_if_an_instance_was_preempted)
that is accessible from the instances themselves that allows the instance to determine whether or not it has been preempted.
This is similar to what is provided by AWS and Azure and should be used to allow the termination handler implementation
to be consistent across the three providers.

In the case that the node is reaching its 24 hour termination mark,
it may be safer to preempt this warning and shut down the node before the 30s shut down signal.
