---
title: baremetal-optional-mdns
authors:
  - "@cybertron"
reviewers:
  - "@celebdor"
approvers:
  - TBD
creation-date: 2019-11-21
last-updated: 2019-11-21
status: implementable
---

# Baremetal Optional mDNS

Make deployment of the baremetal mDNS services optional to support environments
where multicast is not allowed and alternate solutions for providing internal
DNS records are used.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

1. This may be problematic to test because it requires an external provider
   for DNS records. It's possible we can add support for creating those
   records in openshift-metal3/dev-scripts.
1. It's not clear at this time exactly what combination of mDNS services
   will be required. We may only be able to remove mDNS from the workers, but
   still need it on the masters. The current design is flexible enough to
   allow us to handle any combination, however.

## Summary

We have recently had a new use case presented to us that involves a networking
environment where multicast traffic is not allowed. This is problematic for the
existing mDNS-based internal DNS system that is used for baremetal deployments.
However, the targeted environment will have the capability to provide the
necessary DNS records from an external DNS system, so we should be able to
just disable the mDNS services in such an environment.

## Motivation

This enables the deployment of OpenShift on baremetal in environments where
multicast is not supported.

### Goals

Make it possible to deploy OpenShift on baremetal without the mDNS services
enabled.

### Non-Goals

An alternate method for automatically managing the necessary DNS records.
In environments where mDNS is disabled, the deployer will be responsible for
providing the necessary DNS records themselves.

## Proposal

Add a new configuration option to the baremetal install-config called
`useMDNS`. This option will take a string value that specifies where the mDNS
services should be deployed. Initially, this is expected to be one of
"all", "none", or "master". This value will be populated into the baremetal
platform status and consumed by machine-config-operator to determine which
baremetal infrastructure services it should deploy on the nodes.

### User Stories [optional]

#### Story 1

As a customer deploying OpenShift on baremetal in a networking environment
where multicast is not allowed, I want to be able to disable the mDNS services
that won't function there and provide DNS records myself.

### Risks and Mitigations

When deployers choose to use DNS records not managed by the OpenShift install,
it adds more pre-requisites for a successful deployment and increases the
potential for mistakes. The expectation is that this deployment model will only
be used in environments where the external DNS records are automated in some
way that limits the potential for mismatches.

## Design Details

### Test Plan

Baremetal end-to-end testing is still a WIP so it is difficult to come up
with a definite plan for this, but it should be possible to configure a test
job that provides the necessary external DNS records to allow the mDNS services
to be disabled and exercise this functionality.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

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

This should have no impact on upgrades to existing deployments. They will
continue to function as they did before.

To disable mDNS services after an upgrade to the version where this exists,
I believe it would be necessary to set the appropriate value in the baremetal
platform status and then manually remove the static pods for the mDNS services.

### Version Skew Strategy

After upgrade, the machine-config-operator will be looking for this new value
in the platform status. We will need to make sure it is there before MCO runs,
or add logic to MCO to handle it not being present yet.

Having the value present with an older version of MCO will just result in the
same behavior as before. MCO will need to be upgrade before any changes to the
setting will take effect, but it will otherwise work fine.

## Implementation History

## Drawbacks

## Alternatives

We could just leave the mDNS services enabled, even though they won't function
correctly. This is not preferred because it is likely to be confusing to have
services deployed that can't be used.
