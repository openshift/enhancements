---
title: service-ca-bundle-injection
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
last-updated: 2020-01-24
status: implemented
see-also:
  - https://github.com/openshift/service-ca-operator/pull/79 (Implementation)
replaces:
superseded-by:
---

# Support Service CA Bundle Injection

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

A service that needs to securely connect to another service protected
with a service serving cert (provided by the service CA operator)
needs to validate that cert with the service CA bundle. Supporting the
injection of this CA bundle into a number of different resource types
is intended to minimize the amount of complexity involved in sourcing
the CA bundle and configuring its use for a variety of use cases.

### Goals

- Service CA bundle injection should be supported for the following
  types:
  - `APIService`
  - `ConfigMap`
  - `CustomResourceDefinition`
  - `MutatingWebhookConfiguration`
  - `ValidatingWebhookConfiguration`

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

- Add controllers to inject the service CA bundle into supported
  resource types in response to the presence of an injection
  annotation.
  - Annotations;
    - alpha: `service.alpha.openshift.io/inject-cabundle`
    - beta:  `service.beta.openshift.io/inject-cabundle`
  - Older types may support both alpha and beta annotations.
  - Newer types added in 4.4 and above should only respond to the beta
    annotation.

- Supported resource types:
  - `APIService` (`apiregistration.k8s.io`)
    - Injection field: `spec.caBundle`
    - Supported annotations: alpha,beta
  - `ConfigMap`
    - Injection field: `data["service-ca.crt"]`
    - Supported annotations: alpha,beta
  - `CustomResourceDefinition` (`apiextensions.k8s.io`)
    - Injection field: `spec.conversion.webhook.clientConfig.caBundle`
    - Supported annotations: beta
    - Notes: Only inject if conversion with webhook strategy is configured
  - `MutatingWebhookConfiguration` (`admissionregistration.k8s.io`)
    - Injection field: `webhooks[].clientConfig.caBundle`
    - Supported annotations: beta
    - Notes: See below
  - `ValidatingWebhookConfiguration` (`admissionregistration.k8s.io`)
    - Injection field: `webhooks[].clientConfig.caBundle`
    - Supported annotations: beta
    - Notes: See below

- Injection for `{Mutating|Validating}WebhookConfiguration` resources
  (aka admission webhook configurations) is only supported for all
  webhooks.
  - Admission webhook configurations can define more than one webhook.
  - When targeted for injection, an admission webhook configuration
    will have the `CABundle` field for each webhook it defines
    populated by the current service CA bundle.
  - Admission webhook configurations that need to specify different CA
    bundles for different webhooks should not set the annotation since
    the proposed implementation is not intended to be selective.
- Example of an admission webhook configuration after injection of the
  service CA bundle:
```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  ...
webhooks:
- name: mywebhook1
  ...
  clientConfig:
    caBundle: <service ca bundle>
- name: mywebhook2
  ...
  clientConfig:
    caBundle: <service ca bundle>
```
- Example of an admission webhook configuration whose requirement to
  have the CA bundle vary between webhooks is not compatible with the
  proposed implementation:
```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  ...
webhooks:
- name: mywebhook1
  ...
  clientConfig:
    caBundle: <service ca bundle>
- name: mywebhook2
  ...
  clientConfig:
    caBundle: <other ca bundle>
```
- Example of how to separate webhooks into separate admission webhook
  configuration resources to support injection of the service CA
  bundle where required:
```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  ...
  <injection annotation>
webhooks:
- name: mywebhook1
  ...
  clientConfig:
    caBundle: <service ca bundle>
---
apiVersion: admissionregistration.k8s.io/v1
kind: MutatingWebhookConfiguration
metadata:
  ...
  <no injection annotation>
webhooks:
- name: mywebhook2
  ...
  clientConfig:
    caBundle: <other ca bundle>
```

### Risks and Mitigations

N/A

## Design Details

### Test Plan

E2E testing of bundle injection

### Graduation Criteria

Support for injecting `ConfigMap` and `APIService` is GA as of
4.1. Support for `CustomResourceDefinition`,
`MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration`
supported as GA in 4.4.

### Upgrade / Downgrade Strategy

The change as proposed is additive-only, so upgrading will enable
bundle injection and downgrading will remove the capability.

### Version Skew Strategy

N/A

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

Each agent wanting to verify a service serving cert could read the ca
bundle configmap from the operand namespace and write it to whatever
resource required it. This would likely result in duplicative effort
and the potential for errors would be multiplied by the number of
implementations.
