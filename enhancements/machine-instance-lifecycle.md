---
title: Machine/Instance lifecycle
authors:
  - @enxebre
reviewers:
  - @derekwaynecarr
  - @michaelgugino
  - @bison
  - @mrunalp
approvers:
  - @derekwaynecarr
  - @michaelgugino
  - @bison
  - @mrunalp

creation-date: 2019-09-09
last-updated: 2019-09-09
status: implementable
see-also:
replaces:
superseded-by:
---

# Machine/Instance lifecycle

## Release Signoff Checklist

 - [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs


## Summary

Enable unified semantics across any provider to represent the lifecycle of instances backed by machines resources as phases.

## Motivation

Provide the most similar user experience for the machine API across providers.

### Goals

- Provide semantics to convey the lifecycle of machines in a unified manner across any provider.

- Prevent actuators from creating more than one instance for a machine resource during its lifetime.

### Non-Goals

- Introduce breaking changes in the interface for actuators.

## Proposal

- As a user I want to understand at a glance where a machine is at its lifecycle regardless of the cloud.

- As a platform I want to provide the most similar user experience across providers and hide provider specific details.

- As a dev I want to enforce the machine API invariants and reduce the surface of arbitrary specific provider decisions.

This proposes to use unified phases across any provider to convey the lifecycle of machines by leveraging the [existing API](https://github.com/openshift/cluster-api/blob/openshift-4.2-cluster-api-0.1.0/pkg/apis/machine/v1beta1/machine_types.go#L174).

### Implementation Details

#### Phases

The phase of a Machine is a simple, high-level summary of where the machine is in its lifecycle. In the same vein of [pods](https://kubernetes.io/docs/concepts/workloads/pods/pod-lifecycle/#pod-phase) and [cluster API Upstream](https://github.com/kubernetes-sigs/cluster-api/blob/master/docs/proposals/20190610-machine-states-preboot-bootstrapping.md)

MachineSets or other upper level controllers might choose to leverage signaled phases for weighing when choosing machines for scaling down or ignoring failed machines to satisfy replica count.

The phase will be exposed to kubectl additionalPrinterColumns so users can understand the current state of the world for machines at a glance.

The machine controller will set the right phase when its communication flow with the actuator interface meets the following criteria:

##### Provisioning

- Exists() is False.

- Machine has **no** providerID/address.

##### Provisioned

- Exists() is True.

- Machine has **no** status.nodeRef.

##### Running

- Exists() is True.

- Machine has status.nodeRef.

##### Deleting

- Machine has a DeletionTimestamp.

##### Failed

- Create() returns a permanentError type or Exists() is False and machine has a providerID/address.

#### Actuator invariants

- Once the nodeRef is set, it must never be mutated/deleted.

- Once the providerID is set, it must never mutated/deleted.

##### Create()

- It must set providerID and IP addresses.

- It must return a permanentError type for known unrecoverable errors, e.g invalid cloud input or insufficient quota.

##### Update()

- It must not modify cloud infrastructure.

- It should reconcile machine.Status with cloud values.

#### Errors

- A machine entering a "Failed" phase should set the machine.Status.ErrorMessage.

- The machine.Status.ErrorMessage might be bubbled up to the machineSet.Status.ErrorMessage for easier visibility.


#### [Implementation example](https://github.com/enxebre/cluster-api-provider-azure/commit/ef9a0dd68918eb6ca4af50765b07bcae4d309aaf)

### Risks and Mitigations

For it to happen in a non disruptive manner for actuators this proposal is intentionally keeping the surface change small by not breaking the create()/exists()/update() actuators interface. 

Once this is settled we might consider to simplify the interface.

## Design Details

### Test Plan

Changes will be test driven via the current e2e machine API suite https://github.com/openshift/cluster-api-actuator-pkg/tree/8250b456dec7b2fb06c591738518de1265e84a2c/pkg/e2e.

### Graduation Criteria

### Upgrade / Downgrade Strategy

Operators must revendor the lastest openshift/cluster-api version. The actuator machine controller image is set in the machine-api-operator repo https://github.com/openshift/machine-api-operator/blob/474e14e4965a8c5e6788417c851ccc7fad1acb3a/install/0000_30_machine-api-operator_01_images.configmap.yaml so the upgrades will be driven by the CVO which will fetch the right image version as usual.


### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed
