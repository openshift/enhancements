---
title: dns-management-state
authors:
  - "@rfredette"
reviewers:
  - "@miciah"
  - "@danehans"
  - "@frobware"
  - "@sgreene570"
  - "@knobunc"
  - "@miheer"
  - "@candita"
approvers:
  - "@miciah"
  - "@frobware"
  - "@danehans"
  - "@knobunc"
creation-date: 2021-02-09
last-updated: 2021-02-16
status: provisional
see-also:
replaces:
superseded-by:
---

# `managementState` for DNS Operator

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

When diagnosing a DNS issue, sometimes it is helpful or even necessary to
disable the DNS operator and patch the CoreDNS daemonset. Currently, this
requires disabling the cluster version operator (CVO) as well so that the CVO
doesn't re-enable the DNS operator.

The DNS operator API should provide a `managementState` field, which will
prevent the DNS operator from overwriting fields in managed deployments and
daemonsets when the `managementState` field set to `Unmanaged`

## Motivation

### Goals

Provide a method to disable the DNS operator without disabling the CVO.

### Non-Goals

- Do not support the `Force` option for `managementState` that is provided for
  other operators. The `Force` state overrides other upgrade-blocking checks,
  and can lead to unsuccessful upgrades.
- Do not support the `Removed` option for `managementState` that is provided
  for other operators. The `Removed` state means that the operator is actively
  removing itself, which should not be done for DNS.

## Proposal

Several other operators, including the apiserver, scheduler, storage, network,
and console operators already implement the `managementState` field in order to
disable the operator without needing to disabling the CVO. I propose that the
DNS operator utilize the same field for consistency.

The DNS operator spec shall include the `managementState` field. The field will
accept two values:
- `Managed`: The DNS operator will manage CoreDNS configuration.
- `Unmanaged`: The DNS operator will not manage CoreDNS configuration.

### User Stories

#### Story 1

As a developer, I want to test a configuration change to see if it fixes an
issue in CoreDNS. I need to stop the DNS operator from overwriting the fix, so
I set `managementState` to `Unmanaged`.

#### Story 2

As a cluster admin, I have reported an issue with CoreDNS, but until a fix is
released, I need a workaround. I set the DNS operator's `managementState` field
to `Unmanaged`, then apply the workaround.

### Implementation Details/Notes/Constraints [optional]

TODO

### Risks and Mitigations

By adding the `managementState` field, it becomes easier for cluster admins to
put cluster DNS into an unsupported, potentially broken state. In order to
mitigate this, it is important to prominently state that setting
`managementState` to `Unmanaged` is inherently unsupported.

## Design Details

### Test Plan

- set `managementState` to `Unmanaged`. Change various fields (TODO: be more
  specific) in the CoreDNS daemonset, and wait until reconciliation is
  complete, then verify that the changed field(s) were not reverted.
- while the DNS operator's `managmentState` field is set to `Unmanaged`, modify
  various fields (TODO: be more specific) to non-standard values. Change the
  DNS operator's `managementState` field to `Managed`, and verify that the
  fields are returned to their standard values.
- Attempt to set `managementState` to a value other than `Managed` or
  `Unmanaged`. Verify that the change is rejected.

### Graduation Criteria

TODO

### Upgrade / Downgrade Strategy

When upgrading from 4.7 where `managementState` is not supported to 4.8,
`managementState` will be left unset, which should be treated as being in a
`Managed` configuration.

In future upgrades, having `managementState` set to `Unmanaged` could put DNS
in a state that would break on upgrade. As such, upgrades should be blocked
until DNS is returned to the `Managed` state.

### Version Skew Strategy

The DNS operator will accept a spec with `managementState` unset, and will
treat an empty `managementState` the same as if `managementState` is set to
`Managed`.

## Drawbacks

TODO

## Alternatives

TODO

