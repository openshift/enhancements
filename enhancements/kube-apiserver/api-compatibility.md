---
title: neat-enhancement-idea
authors:
  - "@sanchezl"
reviewers:
  - "@deads2k"
  - "@sttts"
approvers:
  - TBD
creation-date: 2020-03-01 
last-updated: 2020-03-03
status: implementable
---

# API Compatibility Level

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The `Summary` section is incredibly important for producing high quality user-focused documentation such as release
notes or a development roadmap. It should be possible to collect this information before implementation begins in order
to avoid requiring implementors to split their attention between writing release notes and implementing the feature
itself.

A good summary is probably at least a paragraph in length.

## Motivation

This section is for explicitly listing the motivation, goals and non-goals of this proposal. Describe why the change is
important and the benefits to users.

### Goals

* OpenShift Compatibility level is specified for APIs added by Openshift.

### Non-Goals

* Deprecated API Removal - deprecated APIs should use the upstream `prerelease-lifecycle-gen` tags and generator to mark an API as deprecated.

## Proposal

### New Generator

Introduce `+openshift-compatibility-gen` generator that processes tags added to API sources to:

1. Verifying compatibility level is appropriate for the API version. 
2. Generate methods on the API type to provide the meta-data to interested parties.
3. Add compatibility level godoc comments to the API type that will show up in the generated OpenShift Container Platform documentation.

#### Generator Tags

* `+openshift:compatibility-gen:level=n` Specify a compatibility level of 1, 2, 3, or 4.
* `+openshift:compatibility-gen:internal` Flag as in internal API.

#### Tag Validation

The generator will validate that the compatibility level is appropriate for the API version, unless the type is tagged as internal. 

| Compatibility Level | API Version                            |
| ------------------- | -------------------------------------- |
| 1                   | Must be GA (e.g. `v1`, `v2`, etc...)   |
| 2                   | Must be Pre-release (e.g. `v1beta1`)   |
| 3                   | Any                                    |
| 4                   | Must be experimental (e.g. `v1alpha1`) |

#### Generated Methods

The a `CompatibilityLevel() int` and `Internal() bool` methods will be generated for each API type in a `zz_generated.openshift_compatibility.go` file.

#### Internal Types

Internal types must be tagged with the `+openshift:compatibility-gen:internal` tag. The generator does not require a `+openshift:compatibility-gen:level=n` tag if the internal tag is specified. If the level is specified, it is not validated.

##### Internal Types with a Custom Resource Definition

Internal Types that are exposed via a CRD should be also be tagged as `openshift:compatibility-gen:level=4` regardless of API version so that the compatibility level comment can be generated and appear in the OpenShift Container Platform documentation.

##### Non-confirming API Versions

APIs with non-conforming API versions must be tagged as internal and must not be exposed via a CRD. 

### Upstream APIs

`k8s.io` APIs will need to be patched to include compatibility information in the OCP documentation.

### User Stories

Detail the things that people will be able to do if this is implemented. Include as much detail as possible so that
people can understand the "how" of the system. The goal here is to make this feel real for users without getting bogged
down.

Include a story on how this proposal will be operationalized:  lifecycled, monitored and remediated at scale.

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints [optional]

#### Verify Task

A `verify-compatibility` recipe added to the `Makefile` will ensure that any API types added to the `api` repository contain the needed 

### Risks and Mitigations

## Design Details

### Open Questions [optional]


### Test Plan

e2e 

### Graduation Criteria



### Upgrade / Downgrade Strategy



### Version Skew Strategy



## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to highlight and record other possible approaches
to delivering the value proposed by an enhancement.


