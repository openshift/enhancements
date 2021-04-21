---
title: Allow-unordered-supported-OpenShift-versions-in-label
authors:
  - "@bentito"
reviewers:
  - "@gallettilance"
  - "@bparees"
  - "@yashvardhannanavati"
  - "@twaugh"
  - "@lcarva"
  - "@amisstea"
approvers:
  - "@twaugh"
creation-date: 2021-04-20
last-updated: 2021-04-20
status: implementable
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# Allow unordered supported OpenShift versions in label

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] Developer documentation is created in [delivery-docs](https://docs.engineering.redhat.com/display/CFC/Delivery)

## Summary

As part of an Operator's bundle definition it may specify the OpenShift version, or versions, it supports. The label, `com.redhat.openshift.versions` allows for this specification. Currently, this may be enforced at several points in the build pipeline (pyxis, IIB). This enhancement seeks to specify that bundle authors may list versions v4.5 or v4.6 in either order if providing a list of supported
OpenShift versions. This enhancement also seeks to sunset this behavior, by only allowing these two specific versions as a list. Going forward, authors would specify a single version, or range, of OpenShift the bundle is supported on. There would no longer be a comma separated list allowed.


## Motivation

Both pyxis and IIB check this `versions` label as part of building operators in the pipeline, this enhancement seeks to define the currently accepted syntax for this label as well as define future usage, that is, sunsetting comma separated labels entirely.

### Goals

- Allow `v4.5,v4.6` or `v4.6,v4.5` as a value for `com.redhat.openshift.versions`.
- Disallow all other uses of commas in this field.

### Non-Goals

- Changes to range specification, for instance, `v4.5-v4.7` is still an expected way to specify a range of OpenShift versions the bundle is supported on.

## Proposal

Create a new common routine that can be shared code between IIB and Pyxis to validate and process the value for `com.redhat.openshift.versions`. Valid values will include `v4.5,v4.6` or `v4.6,v4.5`. All other uses of commas will be disallowed, throw error on processing the bundle. Whitespace will be stripped and ignored before evaluation. These changes will effectively sunset this usage in 
OpenShift 4.7+

No new documentation is needed, as this is already the stated behavior here: https://docs.engineering.redhat.com/display/CFC/Delivery . The Delivery doc will need to be updated to remove the mention of the need for ordering the version fields, as this enhancement will alleviate that limitation.

### User Stories

This work is already mentioned and tracked in the following:

https://projects.engineering.redhat.com/browse/ISV-150

https://projects.engineering.redhat.com/browse/ISV-151

https://issues.redhat.com/browse/PORTENABLE-36


### Risks and Mitigations

This will allow for previous specification of comma separated fields in pre-existing bundles. It will allow for sunsetting of this specification from 4.7 onwards. By creating common routines for both tools that validate bundles in the build pipeline it will provide better maintainability with a single place in code to enforce the validation.

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Unit tests that submit a range of valid and non-valid strings for the label value, exercising all code paths, should be sufficient.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The validations proposed in this enhancement may fail some existing bundles, for instance there is at least one bundle that has: `v4.4,v4.5,v4.6,v4.7` as a value for this field.

## Alternatives

Since this proposal is to add a validation and enforcement for a desired behavior, allowing for a limited current use of comma-separated versions, and preventing it going forward, there really is no alternative way to accomplish this.

