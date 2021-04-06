---
title: okd-ppc64le-builds 
authors:
  - "@mjturek"
reviewers:
  - TBD
  - "@lorbus"
  - "@vrutkovs"
approvers:
  - TBD
creation-date: 2021-03-01
last-updated: 2021-03-01
status: implementable
---

# Build OKD for ppc64le

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Currently OKD only provides builds for x86_64 based systems. This enhancement
is a proposal to build and publish OKD for ppc64le hardware alongside the
x86_64 builds. In this proposal, how, when, and where these builds will
happen will be detailed. 

## Motivation

While OpenShift is supported on ppc64le, OKD is not. This leaves a gap in how OpenShift is developed,
tested, and released on x86\_64 versus ppc64le.

### Goals

The goal is to provide builds of OKD on ppc64le to the public.

### Non-Goals

Building for other architectures is not in scope of this enhancement. That being said, any development
done should be extensible enough that adding other architectures would be simple.

## Proposal
- Build and release OKD payloads for ppc64le alongside x86_64.
- Update documentation to reflect ppc64le availability

## Design Details

### Test Plan

### Graduation Criteria

## Implementation History

## Drawbacks

Maintaining builds for multiple architectures adds complexity to both the build and release process.

## Alternatives

The only obvious alternative is to continue to not build on ppc64le.

## Infrastructure Needed

We need a place to build on ppc64le. In the [OKD on Fedora enhancement PR](https://github.com/openshift/enhancements/pull/78#issuecomment-769215628), cgwalters
suggested that we should reach out to the Fedora or CentOS communities as that is where community
ppc64le hardware is. I am not sure how much capacity we would need.
