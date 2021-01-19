---
title: mcd-domain-specific-application-of-config-changes
authors:
  - "@beekhof"
reviewers:
  - "@crawford"
  - "@eparis"
  - "@rphillips"
  - "@runcom"
  - "@yuqi-zhang"
  - "@kikisdeliveryservice"
  - "@sinnykumari"
  - "@ericavonb"
approvers:
  - "@crawford"
  - "@eparis"
  - "@rphillips"
  - "@runcom"
creation-date: 2019-12-15
last-updated: 2020-06-18
status: provisional
see-also:
  - NA
replaces:
  - NA
superseded-by:
  - NA
---

# Domain specific application of config updates

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

- Should the parts of the configuration that do and don't require reboot be
  part of user facing documentation?

## Summary

A node's configuration is managed by the MCO.  When the configuation is changed
by the system or an admin, the MCO causes the MCD to write a new version of the
file to disk and reboot the system to ensure it is applied.

This proposal is to allow drain and reboot to be optional and independant
actions, to implement the ability to restart a systemd service, and to add a
function to MCD that compares two MachineConfigs and can decide which actions
are needed to apply it.


## Motivation

The current MCO mechanism for applying config changes, is to reboot the machine.

While it is expected and normal that any cluster node can reboot at any time,
clusters have a practical limit to the number of full cluster reboots they can
achieve in a given time period.

This means we need to weigh the correctness and simplicity of using full cluster
reboots, against their frequency and user impact.

In environments where the limit is generally lower, such as bare metal, the
balance can be different - creating a need to find alternative ways to apply
configuration changes for some scenarios.

### Goals

Provide a mechanism for avoiding a full cluster reboot in domain specific
scenarios where engineering has verified that the configuration change:

1. can be proven to be safely and immediately applied by some other mechanism,
1. is not combined with other changes that do require a reboot
1. has a demonstrable negative user impact when applied via reboot

### Non-Goals

1. Removing all reboots from the system
1. Allowing the admin to configure additional domains or strategies for applying
   configuration updates

## Proposal

1. Add a function to MCD that compares two MachineConfigs, and outputs an
   ordered list of known actions that are needed to apply it
1. Modify any parts of MCD that assume drain and reboot always happens after
   writing to disk
1. Modify MCD to perform only the list of actions produced in step 1

The full list of post-write actions required for this feature to be useful are:

- drain
- reboot
- restart a named systemd service

If the action results in an error, or takes too long, a reboot will be
performed as in prior 4.x versions.

### User Stories

The user stories below are intended to be illustrative of the different types of
services we need to build capabilities for.

#### Story 1

As an administrator of a disconnected bare metal cluster, I want the ability to
update the image catalog mirror[1] without potentially causing a full cluster
reboot, so that I can obtain the latest fixes quickly and without needing to be
concerned about the cumulative disruption to the cluster.

[1] As well as the ICSP associated with it, which is sensitive to the addition,
subtraction, or renaming of images.

#### Story 2

As an adminstrator of a large bare metal cluster, I want the addition or removal
of SSH keys to happen without a full cluster reboot, so that the credentials are
usable (or revoked) in a timeframe comparable with traditional environments not
hours.

#### Story 3

As an adminstrator of a bare metal cluster that takes a day to reboot, I want
the system to use the least invasive method for applying configuration changes,
so that I have periods of relative stability where I can accomodate the
maintenance windows of less enlightened vendors.

#### Story 4

As an admin, I want to continue to force reboots for network configuration
updates and changes to kernel flags, so that they are applied in a timely
manner.


### Implementation Details/Notes/Constraints

How the function determines what constitutes a unit of configuration or "domain"
for the purposes of comparision and output is left for later discussion.

Possibilities include basing the results on:
- which file(s) the configuration changes end up in,
- whether specific keys have or have not changed,
- a combination of both

### Risks and Mitigations

The risks are of false positives (an update that requires a reboot does not
trigger one), and false negatives (an update that does not require a reboot
triggers one anyway).  While the latter may be unexpected by an admin that knows
of this feature, at worst it is a performance issue and cannot be considered a
regression over the current behaviour.

False negatives can be mitigated by defaulting to the current behaviour
(reboot), preventing the admin from tampering with the list of domains with
exceptions, and only adding new domains after Engineering and QE validate that
the changes are safe.

## Design Details

### Test Plan

Since the default and most common action for applying a configuration change
will remain draining and rebooting the cluster nodes, we use the term "exception"
below to mean deviation from this approach.

- Unit tests for the comparision function that cover
  - Identical, missing, and invalid MachineConfigs
  - Changes to domains for which no exception exists
  - Changes to only domains for which a 'do nothing' exception exists
  - Changes to only domains for which a 'service restart' exception exists
  - Changes to only domains for which a 'drain + service restart' exception exists
  - Changes to domains for which exceptions exists, combined with changes which require reboot
  - Changes to only domains for which an 'do nothing' exception exists
- Performance testing of the comparison function
- Unit tests for handling unknown/invalid actions from the comparision function
- Unit tests for handling actions that fail or takes too long
- Testing that covers failures during the write phase
- Testing that covers on-disk changes detected during the write phase

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- Sufficient test coverage
- Support for handling ICSP changes

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

As per the current MCD strategy.

### Version Skew Strategy

If the MCD does not contain this feature, or has an outdated view of how an
change needs to be applied, then the first change that affects a new entry will
trigger a reboot (which can not be a regression) and subsequent changes will be
acted on with the new information.  Any changes affecting entries that have been
removed will have the old behaviour (also not a regression).

If the MCD relies on capabilities that the rest of the system doesn't have yet,
then there is a risk that a reboot will not occur for a version of the component
that still requires it.  This could be mitigated through proper error handling
and/or ensuring that the making an exception for a new domain always happens N
versions after any engineering work that was required to make it possible.

## Implementation History

- Initial version: 15-12-2019
- Updated: 18-06-2020

## Drawbacks

The risks primarily fall into 4 categories:

- configuration: that domains could be be added to the comparision function even
  though they require a reboot in unidentified scenarios,

- behavioural: that some config changes will result in a reboot, others not, and
  that admins cannot determine which in advance

- support: that the implementation could introduce bugs that prevent necessary
  reboots from being triggered

- tech debt: that the implementation may need to be replaced if a more holistic
  approach is identified

## Alternatives

- Improve the MCD's logic for handling changes to CRDs.  This may require a
  lot of things that need to be plumbed through in the MCO and may not be
  possible in the short term.

- Define a higher level syntax that would contain additional information about
  the action a field needs in order for any changes to be applied; and was be
  able to be transformed into the appropriate Ignition format.  This would be a
  much larger body of work that would not be possible to complete in the short
  term.

## Infrastructure Needed

None
