---
title: aws-dedicated-hosts
authors:
  - "@faermanj"
reviewers:
  - "@nrb"
  - "@mtulio"
  - "@rvanderp3"
# approvers:
#  - "@TODO"
creation-date: 2025-05-05
last-updated: 2025-05-05
status: provisional
see-also:
  - ""
replaces:
  - ""
superseded-by:
  - ""
---

# Support for Dedicated Hosts on AWS

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

AWS EC2 lets customers allocate [dedicated hosts](https://aws.amazon.com/ec2/dedicated-hosts/) and control placement of instances on them. This feature is usually desirable for licensing compliance or security concerns.
This enhancement adds support for dedicated hosts on OpenShift so that existing dedicated hosts can be used in clusters.

## Motivation

This enhancement is necessary so that any workload that must run on dedicated hosts can be migrated to OpenShift.

### Goals

1. Provision instances in the host identified by the customer.

### Non-Goals

1. Automatically allocate and release dedicated hosts.

## Proposal

1. Add support for dedicated hosts on upstream cluster-api-provider-aws
1. Integrate with installer with corresponding tests and documentation

### Implementation Details/Notes/Constraints [optional]

This implementation should take pre-allocated host ids and pass it to the EC2 RunInstances API, optionally with host affinity setting.
Once that is working, we may consider adding automatic host allocation and release.

### Risks and Mitigations

Ensure that resource pruners (upstream and internal) are able to collect dedicated hosts that might leak from tests.

### Test Plan

Add a dedicated-host test case to CAPA E2E suite.

### Graduation Criteria

#### Examples

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- Sufficient time for feedback
- Available by default

## Infrastructure Needed [optional]

- Dedicated Hosts on EC2
