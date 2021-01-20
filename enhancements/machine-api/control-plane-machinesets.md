---
title: control-plane-machinesets
authors:
  - "@michaelgugino"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-01-19
last-updated: 2021-01-19
status: implementable
---

# Control Plane MachineSets

<!-- toc -->
- [Release Signoff Checklist](#release-signoff-checklist)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
    - [Story 1](#story-1)
    - [Story 2](#story-2)
  - [Implementation Details/Notes/Constraints [optional]](#implementation-detailsnotesconstraints-optional)
  - [Risks and Mitigations](#risks-and-mitigations)
    - [Risk: Users might delete these new machinesets](#risk-users-might-delete-these-new-machinesets)
    - [Risk: Users might delete too many control plane hosts carelessly](#risk-users-might-delete-too-many-control-plane-hosts-carelessly)
    - [Risk: Conflicts with user-created operators or automation](#risk-conflicts-with-user-created-operators-or-automation)
    - [Risk: Some clusters won't successfully automatically adopt machines](#risk-some-clusters-wont-successfully-automatically-adopt-machines)
    - [Risk: Some users might scale the machinesets](#risk-some-users-might-scale-the-machinesets)
- [Design Details](#design-details)
  - [machine-api API modifications](#machine-api-api-modifications)
  - [New Controllers](#new-controllers)
  - [MachineSet deletion webhooks](#machineset-deletion-webhooks)
  - [Open Questions [optional]](#open-questions-optional)
    - [Should we modify the installer?](#should-we-modify-the-installer)
  - [Test Plan](#test-plan)
  - [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
<!-- /toc -->

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement documents the intent to adopt existing control plane machines
into machinesets for all clusters utilizing the machine-api.

## Motivation

Currently, control plane hosts are deployed as individual machines without
a machineset or other controller to assist in replacement operations.

### Goals

1. Adopt existing control plane machines into machinesets
1. Add logic to ensure users don't break themselves with the new machinesets

### Non-Goals

1. Perform any in-place updates, such as vertical resizing, to a running
machine.
1. Enable or prevent horizontal control plane scaling.
1. Determine or specify the exact method of the etcd-operator to allow etcd
members to automatically be joined and removed from the cluster.
1. Add new API objects

## Proposal

### User Stories

#### Story 1
As a cluster administrator, when I need to replace a control plane machine, I
want to be able to simply delete the existing one and automatically get a
replacement that is identical.

#### Story 2
As a cluster administrator, when I need to change the attributes of a machine,
I want to be able to simply update the machineset, delete the existing machine,
and automatically get an updated machine.

### Implementation Details/Notes/Constraints [optional]

This feature requires the ability of the etcd-operator to safely add and remove
etcd quorum members automatically without intervention of the machine-api.

### Risks and Mitigations

Worst case scenario is total destruction of a running cluster, since we'll be
targeting control plane hosts.

#### Risk: Users might delete these new machinesets
We'll use webhooks to prevent this.  These webhooks are automatically synced
by our operator and cannot be easily overridden.

#### Risk: Users might delete too many control plane hosts carelessly
Now that users have these machinesets, they might be less cautious when deleting
control plane machines, expecting that automated controls will prevent bad
things from happening.

We'll implement machine deletion lifecycle hooks https://github.com/kubernetes-sigs/cluster-api/pull/3132
to prevent a machine from being deleted in the cloud prior to its replacement
coming online.

Additionally, we still have etcd-quroum-guard in place, which utilizes PBDs
to prevent removal of more than one control plane machine at a time.

#### Risk: Conflicts with user-created operators or automation
It may be necessary to modify the existing machine objects to add labels
so they can be successfully adopted into the machinesets via match-labels.

This update operation may conflict with any automation users might have and
result in dueling update operations.

We'll ensure we don't mutate any existing labels on a machine and only add
new ones.  These labels will be properly namespaced.  We'll detect if users
have already adopted the machines into machinesets and stop if any conflicts
are detected.

#### Risk: Some clusters won't successfully automatically adopt machines
Building on the previous risk, if for any reason our adoption logic cannot
create machinesets for one or more control plane hosts, we should signal
degrade=True on our operator.  This will inform users that changes must be
made to the cluster prior to performing additional upgrades.

#### Risk: Some users might scale the machinesets
Inevitably, some users will attempt to scale the machinesets.  This proposal
allows for this operation as the etcd-operator will be in charge of ensuring
the proper amount of etcd members are added or removed to the cluster.

For example, if a user scales all control plane machinesets to zero, etcd's
configured PDBs would disallow draining on 2 of the machines.  This would
prevent all machines from being removed from the cluster.  This would have the
same effect as deleting each control plane machine individual prior to this
feature being implemented.

If a user attempts to scale any individual machineset to a higher number of
replicas, resulting in more than 3, or an even number of replicas, the
etcd-operator should not add these new machines to etcd-quorum.  This would
potentially allow 'warm' control plane machines to exist, allowing the
etcd-operator to quickly add/remove members from quroum from the expanded
control plane pool.

In the future, if we desire to allow the control plane to scale to 1 replica,
this implementation will not need modifications to allow that usecase.  etcd
and any other operators will need to determine how to scale their components,
and users may scale down the cluster as they see fit.

## Design Details

### machine-api API modifications
We'll need to implement lifecycle hooks to coordinate replacement of an
given control plane machine.  This consists of a small amount of common
controller logic and annotations, no API fields.

We'll need to add a field to machineSet.status to indicate the last time
an individual machineset's status.AvailableReplicas was added to facilitate
the functionality provided by the lifecycle hook.  This is a single additional
API field in the status subresource.

No other API changes are needed.

### New Controllers
We'll need a new controller to adopt existing machines into machineSets.  This
controller will be responsible for updating the machine objects as well as
creating the new machineSets.  This controller should have pre-flight checks
to ensure we don't break existing users.

We'll need a controller to add and remove the lifecycle hooks on the control
plane machines.

Both of these controllers are/should be platform independent, and may run in the
same container as the existing machine-controller.  This will allow us to reuse
existing managers/informs to reduce API load.

### MachineSet deletion webhooks
We'll use webhooks to prevent users from deleting the machinesets we create.
Users should never need to do this, and doing so will result in the owned
machine objects being deleted as well.  There is not a better way to prevent
outright deletion of an object other than webhooks.

We'll need to account for the fact that users might have created or will
create machinesets that appear to the webhook controller to be control plane
webhook hooks.  The users might desire to remove these newly created machinesets
after some time.  We should allow functionality, such as an override annotation
to allow any particular machineset to be delete.  This might also be necessary
in an extreme case (unforeseen bug) to remediate a problem with a machineset
we created automatically.

### Open Questions [optional]

#### Should we modify the installer?
In future releases, should a machine-api controller continue to create
machinesets and adopt the machines created by the installer, or should we
modify the installer to create the desired machinesets for new clusters and
perform adoptions as a 1-time operation for a given release.

This question is not a blocker, we can decide the update the installer or not
later as the adoption code will work in all scenarios.

### Test Plan

We will need to thoroughly plan and test potential failure modes.  We need
a plan to collect metrics and proactively support users when control plane
machines cannot be adopted.

### Upgrade / Downgrade Strategy

No special upgrade logic is needed.

For downgrades, we will not delete the machinesets we created, but the
webhooks and new controllers will not be running to prevent bad user actions.

## Drawbacks

Some users might not want this behavior at all.

## Alternatives

Previous proposals:
* https://github.com/openshift/enhancements/pull/278
* https://github.com/openshift/enhancements/pull/292
