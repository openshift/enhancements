---
title: mcd-reboot-dictionary
authors:
  - "@beekhof"
reviewers:
  - "@crawford"
  - "@eparis"
  - "@rphillips"
  - "@runcom"
approvers:
  - "@crawford"
  - "@eparis"
  - "@rphillips"
  - "@runcom"
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

# MCD Reboot Dictionary

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

- Should we allow the whitelist to apply recursively to a directory, or make
  use of pattern matching?

- Is there any way to mitigate the version skew problem?

- Should the parts of the configuration that do and don't require reboot be
  part of user facing documentation?


## Summary

A node's configuration is managed by the MCO.  When the configuation is changed
by the system or an admin, the MCO and MCD write a new version of the file to
disk and reboot the system to ensure it is applied.

This proposal is to create a dictionary that defines a whitelist of file paths
for which alternative application strategies should be used.  


## Motivation

Always rebooting a node is reasonable for cloud based systems, since they can
be rebooted very quickly.  Additionally, it is the only way for kernel
parameter changes to take effect.

However on baremetal systems, rebooting a node takes significantly longer (in
the order of minutes) and the machine configuration includes files that need
either no action to take effect, or can be more quickly applied by restarting a
systemd service.

### Goals

- Creation of a minimally invasive solution that can be delivered in the 
  short-term, allowing time for a more comprehensive approach to be developed
- A system defined whitelist of file paths that do not trigger a reboot
- Changes to systemd service configurations are applied by restarting the service
- Specific files can be updated with no action (eg. ssh keys)
- Any file not covered by the whitelist triggers a reboot

### Non-Goals

- Allowing the admin to specify additional paths or strategies
- Allowing different behaviour based on which option or part of the file has
  been updated

## Proposal

The proposal calls for a whitelist that MCD uses to decide which action to take
after updating a configuration file for the MCO.  To prevent the admin from
tampering with the whitelist, it will be included as part of the MCD image as a
static file in yaml format.

The information could be baked into the the MCD itself, but assuming we treat it
as data, the file will be a list of entries containing:

- filename
- path
- action to perform
- action specific data (eg. the service name, timeout)

After the MCD writes out a configuation change, it will consult the whitelist
before deciding if a reboot is required. If the filename is present, has a
valid action and any associated data, the action will be performed.  Otherwise,
a reboot will be performed as in prior 4.x versions.

If the action results in an error, or takes too long, a reboot will be
performed as in prior 4.x versions.


### User Stories [optional]

Detail the things that people will be able to do if this is implemented.
Include as much detail as possible so that people can understand the "how" of
the system. The goal here is to make this feel real for users without getting
bogged down.

#### Story 1

As a telco admin, I want the system to use the least invasive method for
applying configuration file updates, to reduce periods of degraded system and
application availability.

#### Story 2

None

### Implementation Details/Notes/Constraints [optional]

This is an interim solution designed to quickly address a market need and
gather data for a long term approach.

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

- Missing and empty whitelist
- Changes to files not on the whitelist
- Changes to files on the whitelist requiring no action
- Changes to files on the whitelist requiring a service restart
- Changes to files on the whitelist with invalid actions/data
- Changes to files on the whitelist with valid actions/data but the action fails or takes too long
- Large whitelist performance

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

If the MCD does not contain this feature, or only knows about the old
whitelist, then the first change that affects a new entry will trigger a reboot
(which can not be a regression) and subsequent changes will be acted on with
the new list.  Any changes affecting entries that have been removed will have
the old behaviour (also not a regression).

If the MCD knows about a newer whitelist than the rest of the system, then
there is a risk that a reboot will not occur for a version of the component
that still requires it.  This could be mitigated by ensuring the adding a file
to the whitelist always happens N versions after any engineering work that was
required to make it possible.

## Implementation History

- Initial version: 15-12-2019

## Drawbacks 

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

- Improve the MCD's logic for handling changes to CRDs.  This may require a
  lot of things that need to be plumbed through in the MCO and may not be 
  possible in the short term.

- Define a higher level syntax that would contain additional information about
  the action a field needs in order for any changes to be applied; and was be
  able to be transformed into the appropriate Ignition format.  This would be a
  much larger body of work that would not be possible to complete in the short 
  term.


## Infrastructure Needed [optional]

None
