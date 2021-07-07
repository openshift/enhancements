---
title: console-expose-cni-type
authors:
- "@mariomac"
  reviewers:
- "@abhat"
- "@jotak"
  approvers:
- "@abhat"
  creation-date: 2021-07-07
  last-updated: 2021-07-07
  status: provisional
---

# Console: expose CNI type

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The Console requires getting visibility about the CNI type (`OpenShiftSDN` or `KubernetesOVN`)
to show/hide some fields that might apply to one CNI type but not to the other.

## Motivation

[In the new network policy creation forms, some fields may depend on the type
of cluster network type](https://issues.redhat.com/browse/NETOBSERV-16).
For example, `OpenShiftSDN` neither supports egress
network policies nor ingress exceptions. The related fields should be only visible
when the cluster network is `KubernetesOVN`.

[According to a current proof of concept](https://github.com/mariomac/console/pull/1),
it is possible to get the CNI type from the console by means of the Kubernetes API.
However, this would limit the visibility of the CNI type to the `Administrator` user.

### Goals

* For any user in the console, the CNI type can be retrieved (`OpenShiftSDN` or `KubernetesOVN`).

### Non-Goals

* Implementing the actual required changes in the UI.

## Proposal

**TODO** check if the SDN team has any component common to `OpenShiftSDN` or `KubernetesOVN` that
could expose such value.

### User Stories

* As an Openshift Console developer, I would like to get visibility about the CNI type
  that runs in the customer cluster.
  
### Implementation Details/Notes/Constraints [optional]

To be decided.

### Risks and Mitigations

To be evaluated.

## Design Details

### Open Questions [optional]

N/A

### Test Plan

To be decided.

### Graduation Criteria

To be decided.

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

As long as this feature depends on a stable and documented Openshift API feature,
there are no dependencies that might cause such version skew.

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

* [Fetch the K8s API directly from the console](https://github.com/mariomac/console/pull/1).
  However, this capability would be available only to administrator users.
  
* Expose the CNI type in the Console backend. However, this would require adding new
  permissions to the console itself.

## Infrastructure Needed [optional]

N/A
