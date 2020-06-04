---
title: pre-flight-validations
authors:
  - "@mandre"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-05-18
last-updated: 2020-06-04
status: implementable
---

# Pre-flight validations

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

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
  DNS to reach the cloud's endpoints
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

The validations must leave the environment unaltered.

The framework should allow implementing checks common to all platforms as well
as per-platform checks.

We do not envision the need for the users to write additional validations. As
a consequence, the validations do not need to be loaded on startup and will be
compiled into the `openshift-install` binary. This may be revisited later.

#### Enabling the validations

The validations will be split into two groups: the core validations (including
all current validations) and the extra validations.

Core validations are the ones we already know. They run every time, as it is
done today.

The extra validations will be enabled on demand via a flag when running the
installer.

In the future we may also consider adding in the form of a separate command or
flag a way to perform all validations and check that the environment is
suitable without actually performing a deployment.

#### Reporting errors

A failed validation typically causes the installer to fail early and not
proceed with the deployment of OpenShift. While this is fine when the
validation identifies a missing hard requirement, there are cases where the
environment doesn't match the recommendation and we may want the installer to
go on with the deployment still.

In that case, the validation can be marked as optional, meaning failure of the
validation will not stop the deployment.

The installer will report all found failures at once, and will not stop on the
first validation error.

Validation failure should result in actionable action, for example failure
message could provide pointers on how to fix the error.

#### Pre-provision a node

Node with the master flavor on the user provisioned network:
- pull container images
- validate networking and cloud connectivity
- run benchmarking tools, for example `fio` for storage

Then report back to the installer.

### Risks and Mitigations

Due to their idempotent nature, there is no identified risk in running the
validations.

However, the deployment time will increase when running more validations. For
this reason, The installer will either enable the new validations by default
(have the validation in the core group) if the overhead is found to be minimal,
or include the validation in the extra validations group and provide a flag for
enabling them.

There will be no changes to the existing flows.

## Design Details

### Test Plan

The code for the framework and the validations will have unit tests.

In addition, we will enable the validations checks in CI in order to exercise
them and potentially highlight issues with the underlying CI infrastructure.

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

Only perform input validation as it is done today and rely on runtime errors to
troubleshoot deployment issues.

The pre-flight validations can give a good indication whether a deployment has
a chance to succeed at a given time, however they can't catch all potential
issues. Any change in the environment invalidates the previous validation
results. That is why it is important to also report runtime errors in way that
is easy to understand.

The pre-flight validations are not mutually exclusive with [improved
debuggability](https://github.com/openshift/enhancements/pull/328) of the
deployment errors but the two are instead complementary.
