---
title: openstack-availibility-zones
authors:
  - "@egarcia"
  - "@pprenetti"
reviewers:
  - "@mandre"
  - "@fedosin"
approvers:
creation-date: 2020-07-15
last-updated: 2020-07-15
status: implementable
---

# OpenStack Availability Zones

## Release Signoff Checklist

- [x]  Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

- Will the node's AZ be used for its volume? Will this effect the behavior of the cluster?
- Can persistent volume claims be made against volumes that are in a different AZs?

## Summary

The installer should automatically discover all availability zones (AZs) and distribute the control plane and compute nodes across them to maximize availibility by default. To accomodate use cases with AZ restrictions, the installer should also allow users to specify a set of them for each machinepool.

## Motivation

### Goals

- Enable the discovery of AZs in OpenStack
- Spread out control plane and machine nodes across AZs to maximize availibility as a default behavior
- Add an option to restrict a machine pool to a set of AZs in the installer

### Non-Goals

### Proposal

### User Stories

#### Day 2 Additional Machinepool

As a user, I want to be able to add Machinepools on a different AZ then the one the installer is currently installed on in order to increase the availibility of my cluster, and to re-distribute the load.

#### Install time Machinepool AZ customization

As a user, I want to customize which AZs each Machinepool can be installed onto when the cluster is first installed.

#### Default AZ discovery and HA installation

When I install OpenShift on an openstack cluster that has multiple AZs, I want the installer to discover the available AZs and install the cluster across all of them in a way that is highly available without taking away from the user experience.

## Implementation Details/Notes/Constraints

This comes down to the following core features:
1. allowing custom AZs for Machinepools
2. enabling the installer to lookup AZs
3. creating a default HA deployment across discovered AZs

### Custom AZs for Machinepools

This feature would allow users to specify a list of AZs that the installer should create nodes on for a given Machinepool. It will attempt to evenly distribute load across all listed AZs. The user interface should match the [AWS implementation](https://github.com/openshift/installer/blob/master/docs/user/aws/customization.md#machine-pools), for the best user experience across platforms.

### AZ lookup

The installer should have the ability to discover AZs in a given OpenStack cloud and cache this information. This will be useful for validating the AZs passed to the Machinepools, and for selecting which AZs to install onto by default. This would not require any changes to the user interface, and should take nothing more than an API query.

### Default HA Deployment

When the installer is run without explicit AZs provided, it should reference the AZs it discovered and spread the install out over them. Users that wish to narrow the scope of this can specify custom AZs as noted above.

### Risks and Mitigations

- If persistent volume claims cannot be made against volumes in different AZs, then that would be a huge user experience issue, and could delay or block the feature.

## Design Details

### Test Plan

We will test this with the standard suite of unit tests and are also equipped to test in QE. 

### Graduation Criteria

*This enhancement will follow standard graduation criteria.*

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Upgrade / Downgrade Strategy

Not applicable

### Version Skew Strategy

Not applicable

## Implementation History

## Drawbacks

OpenStack is a very customizable cloud platform, and so AZs can be across the datacenter or across ths ocean. As a result, spreading out installs across AZs could lead to adverse performance and behaviors.

## Alternatives

Not applicable