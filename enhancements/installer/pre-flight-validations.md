---
title: pre-flight-validations
authors:
  - "@mandre"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-05-18
last-updated: 2020-05-18
status: implementable
---

# Pre-flight validations

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

- Will the validations be run automatically on `cluster create` or will it be
  an explicit action?
- If explicit action, how will the validations be enabled? E.g. adding
  a `--dry-run` option to the installer vs. a separate subcommand or even
  a separate binary.
- When enabling the validations, will it also perform the installation or
  simply run the validations?
- Can the installer override validations that failed?
- How are failures and warnings reported to the user?

## Summary

One of the guiding principles of OCP 4 is that the installation should always
succeed. This goal is relatively easy to implement for public cloud platforms
where we can make assumptions about services being available, or performance
meeting requirements, however this is not the case with private clouds where
each cloud is unique.

It is currently not possible to tell with confidence if an installation will be
successful or not for the following platforms:
- BareMetal
- OpenStack
- oVirt
- vSphere

For this purpose, we propose to implement a framework allowing the installer to
run pre-flight validations in order to certify that all pre-requisites are met
to successfully install OpenShift in the selected environment.

## Motivation

This section is for explicitly listing the motivation, goals and non-goals of
this proposal. Describe why the change is important and the benefits to users.

### Goals

By having an automated way to identify potential issues early, before the
deployment even started, we want to reduce wasted time and resources, customer
escalations, and improve the perception of OpenShift deployments on-premise.

The installer already performs some validations:
- checks all required fields are set and that the data is in the right format
- some basic validation, such as networks do not overlap

However, this doesn't check that the environment is suitable to install OpenShift:
- pull secret is valid to fetch the container images
- the tenant has adequate quota and the flavors' specifications are within the
  recommended ranges.
- for user-provided networks, check the subnets have a DHCP server and valid
  DNS to reach the cloud's enpoints
- necessary cloud services are available
- storage performance

### Non-Goals

Implementing every useful validation we can think of is out-of-scope:
- The potential number of validations is huge
- We expect more validations to be added over time as new issues are
  discovered.

## Proposal

This is where we get down to the nitty gritty of what the proposal actually is.

### User Stories

#### On-premise deployments

As an administrator of an OpenShift cluster, I would like to verify that my
OpenStack cloud meets all the performance, service and networking requirements
necessary for a successful deployment.

#### CI debugging

As an OpenShift developer, I would like to know if the CI flakes might be
caused by transient environmental issues.

### Implementation Details/Notes/Constraints [optional]

This should allow for checks common to all platforms and per-platform checks.

We do not envision the need for the users to write additional validations. As
a consequence, the validations do not need to be loaded on startup and will be
compiled into the `openshift-install` binary. This may be revisited later.

#### Pre-provision a node

Node with the master flavor on the user provisioned network:
- pull container images
- validate networking and cloud connectivity
- run fio

### Risks and Mitigations

None.
The installer will either enable the new validation by default if the overhead
is found to be minimal, or provide an flag for enabling the validations.

There will be no changes to the existing flows.

## Design Details

### Proposed Design

In order to accomodate a number of sprawling tests, we need to develop a framework for running them. The framework must have the following garuntees:
- returns an accurate list of all validation errors discovered during run
- allows tests for the same component to be grouped into packages
- allows you to specify dependencies between test packages
  - example: a set of tests the validate openstack's quota depends on tests that validate the clouds.yaml
- does not run a package when a package that is a downstream dependency fails or returns validation errors
- run all packages in the most optimal way possible

### Test Plan

The code for the framework and the validations will have unit tests.

In addition, we will enable the validations checks in CI in order to exercise
them and potentially hightlight issues with the underlying CI infrastructure.

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

Not applicable.

### Version Skew Strategy

Not applicable.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

Running the validations would increase the time it takes to run the installer.
As a consequence, the validations may not be enabled by default.

## Alternatives

Only perform input validation as it is done now.
