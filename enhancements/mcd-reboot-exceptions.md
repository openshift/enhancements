---
title: mcd-reboot-dictionary
authors:
  - "@beekhof"
reviewers:
  - "@crawford"
  - "@eparis"
  - "@rphillips"
approvers:
  - "@crawford"
  - "@eparis"
  - "@rphillips"
creation-date: 2019-12-15
last-updated: 2019-12-15
status: provisional
see-also:
  - NA
replaces:
  - NA
superseded-by:
  - NA
---

# Domain specific minimisation of config driven cluster reboots

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

- Should the parts of the configuration that do and don't require reboot be
  part of user facing documentation?


## Summary

A node's configuration is managed by the MCO.  When the configuation is changed
by the system or an admin, the MCO and MCD write a new version of the file to
disk and reboot the system to ensure it is applied.

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
1. Allowing the admin to configure additional domains or strategies for applying configuration updates

## Proposal

1. Add a function to MCD that compares two MachineConfigs, and outputs an ordered list of known actions that are needed to apply it
1. Modify any parts of MCD that assume drain and reboot always happens after writing to disk
1. Modify MCD to perform only the list of actions produced in step 1

The full list of actions required for this feature to be useful are:

- drain
- reboot
- restart a named systemd service

If the action results in an error, or takes too long, a reboot will be
performed as in prior 4.x versions.

### User Stories [optional]

Detail the things that people will be able to do if this is implemented.
Include as much detail as possible so that people can understand the "how" of
the system. The goal here is to make this feel real for users without getting
bogged down.

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


### Implementation Details/Notes/Constraints [optional]

How the comparision determines what constitutes a domain, whether by target file
or by configuration element, is left for later discussion.

### Risks and Mitigations

The risks are of false positives (an update that requires a reboot does not
trigger one), and false negatives (an update that does not require a reboot
triggers one anyway).  While the latter may be unexpected by an admin that
knows of this feature, at worst it is a performance issue and cannot be
considered a regression over the current behaviour.

False negatives can be mitigated by defaulting to the current behaviour
(reboot), preventing the admin from tampering with the whitelist, and only
adding entries to the whitelist after Engineering and QE validate that no
possible contents of the file could require a reboot.

## Design Details

### Test Plan

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

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- Whitelist format stability
- Sufficient test coverage
- Provisional whitelist contents

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- A supportable whitelist

##### Removing a deprecated feature

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

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
