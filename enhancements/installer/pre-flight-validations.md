---
title: pre-flight-validations
authors:
  - @mandre
  - @pierreprinetti
reviewers:
  - EmilienM
  - Fedosin
  - LalatenduMohanty
  - abhinavdahiya
  - iamemilio
  - stbenjam
approvers:
  - TBD
creation-date: 2020-05-18
last-updated: 2020-12-17
status: implementable
---

# Pre-flight validations

## Release Signoff Checklist

- [v] Enhancement is `implementable`
- [v] Design details are appropriately documented from clear requirements
- [v] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [x] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

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
- required cloud services are available
- required storage performance

The pre-flight validations should not alter the target infrastructure nor leave
behind any new resource.

### Non-Goals

Implementing every useful validation we can think of is out-of-scope:
- The potential number of validations is huge
- We expect more validations to be added over time as new issues are
  discovered.

## Proposal

This is where we get down to the nitty gritty of what the proposal actually is.

### User Stories

#### On-premise deployments

As an OpenShift administrator installing a new OpenShift cluster, I want the
installation process to fail early when the requirements are not met for a
successful installation. In such a case, I also want clear and actionable error
messages right in the Installer output.

#### CI debugging

As an OpenShift developer, I would like to rapidly identify failures caused by
transient environmental issues.

### Implementation Details/Notes/Constraints

The validations must leave the environment unaltered.

The framework should allow implementing checks common to all platforms as well
as per-platform checks.

We do not envision the need for the users to write additional validations. As
a consequence, the validations do not need to be loaded on startup and will be
compiled into the `openshift-install` binary. This may be revisited later.

#### Enabling the validations

The validations will run automatically, right after the `install-config.yaml`
syntax validation.

#### Reporting errors

A failed validation typically causes the installer to fail early and not
proceed with the deployment of OpenShift.

The installer will report all found failures at once, and will not stop on the
first validation error.

Validation failures should clearly indicate the root cause of an error, and if
applicable, suggest a solution to it.

### Risks and Mitigations

Depending on the number and the nature of the validations, the installation
time might end up increasing noticeably. Because the goal is to fail early,
time-consuming validations should seldom be considered for inclusion.

Since pre-flight validations are only run at install time, and not on cluster
upgrade/downgrade, caution should be used when migrating existing validations
to this "pre-flight" framework.

## Design Details

### Test Plan

The code for the framework and the validations will have unit tests.

In addition, we will enable the validations checks in CI in order to exercise
them and potentially highlight issues with the underlying CI infrastructure.

## Upgrade / Downgrade Strategy

Pre-flight validations are only run when installing a new OpenShift cluster.

## Implementation History

The pre-flight validation framework is implemented in OpenShift v4.6. New
validations should be added in new releases, to increase the coverage of
existing requirements and to cover new requirements.

## Drawbacks

Running the validations will increase the time it takes to run the installer.

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
