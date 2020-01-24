---
title: service-ca-bundle-injection-for-admission-webhooks
authors:
  - "@marun"
reviewers:
  - "@deads2k"
  - "@sttts"
  - "@stlaz"
approvers:
  - "@deads2k"
  - "@sttts"
creation-date: 2020-01-23
last-updated: 2020-01-23
status: implementable
see-also:
  - https://github.com/openshift/service-ca-operator/pull/79 (Implementation)
replaces:
superseded-by:
---

# Support Service CA Bundle Injection for Admission Webhooks

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Admission webhooks can secure their endpoints automatically with a
serving cert provisioned by the service CA operator, but the CA bundle
needed to verify that cert must be manually added to an admission
webhook configuration resource (i.e. ). The service CA operator should be
updated to support injection of the CA bundle for admission webhook
configurations.

## Motivation

A survey of operators that configure admission webhooks that use
serving certs determined that the quality of injection varied (not all
were compatible with CA rotation) and that there was unnecessary
duplication of effort. Implementing this facility in the service ca
operator would ensure that all operators (and user workloads) had a
simple and well-tested option.

### Goals

- Service CA bundle injection is supported for both
`MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration`
admission webhook configuration types.

### Non-Goals

- Supporting CA bundle injection to a subset of webhooks defined in an
  admission webhook configuration resource.
  - Allowing selective injection would likely increase the complexity
    of implementation and there is no clear indication that this
    capability is required.
  - Webhooks in one configuration object are all independent and
    therefore configuration can be split into multiple resources if
    difference CAs are necessary.

## Proposal

- Add a new bundle injection controller for `MutatingWebhookConfiguration`
- Add a new bundle injection controller for `ValidatingWebhookConfiguration`
- The new controllers will ensure that both types of admission webhook
  configurations will have all their CABundle fields populated by the
  current service CA bundle when they are found to have one of the
  injection annotations (`service.beta.openshift.io/inject-cabundle`
  or `service.alpha.openshift.io/inject-cabundle`)
  - Admission webhook configurations needing to specify different CA
    bundles for different webhooks should not set the annotation since
    the proposed implementation is not intended to be selective.

### Risks and Mitigations

N/A

## Design Details

### Test Plan

E2E testing of bundle injection

### Graduation Criteria

Being delivered as GA in 4.4

### Upgrade / Downgrade Strategy

The change as proposed is additive-only, so upgrading will enable
bundle injection for admission webhooks and downgrading will remove
the capability.

### Version Skew Strategy

N/A

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

Avoid implementing for 4.4 in the interests of implementing support
for injecting the service CA bundle to a subset of webhooks defined in
an admission webhook configuration.
