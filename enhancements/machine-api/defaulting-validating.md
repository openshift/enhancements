---
title: Defaulting and validating machine API providerSpec
authors:
  - "@enxebre"
reviewers:
  - "@JoelSpeed"
  - "@elmiko"
  - "@alexander-demichev"
  - "@michaelgugino"
  - "@Danil-Grigorev"
approvers:
  - "@JoelSpeed"
  - "@elmiko"
  - "@alexander-demichev"
  - "@michaelgugino"
  - "@Danil-Grigorev"
creation-date: 2020-06-08
last-updated: 2020-06-08
status: implementable
see-also:
replaces:
superseded-by:
---

# Defaulting/Validation for Machine API ProviderSpec


## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Today the machine API embeds multiple particular cloud provider APIs as a single raw extension within the machine resource.

This results in a lack of first class support for manipulating these APIs from a Kubernetes point of view, particularly in a lack of defaulting and validating capabilities.

This proposal outlines a mechanism to enable defaulting and validation on the particular providers API before the input is persisted to etcd.

## Motivation

Having multiple APIs as a raw extension is brittle as it allows bad user input to pass initial validation only to fail when the changes are applied.
Failing earlier and communicating it meaningfully is better. In this line we want to ensure that API defaulting and validation happens before a resource input is persisted to disk to dramatically improve the UX for manipulating resources under the `machine.openshift.io`  group.

### Goals

- Enable defaulting and validating on the particular providerSpec for the Machine and MachineSet resources.

### Non-Goals

- Introduce API breaking changes.
- Externalise the providerSpec and expose a new CRD.
- Flesh out the fields and details for each particular provider defaults and validation. That can be carefully discussed in each individual PR and updated back to this proposal in retrospective.

## Proposal

Today the machine API embeds the particular cloud provider API as a raw extension.

```go
// ProviderSpec defines the configuration to use during node creation.
type ProviderSpec struct {
  Value *runtime.RawExtension `json:"value,omitempty"`
}
```

This proposes to run both a `MutatingWebhookConfiguration` and a `ValidatingWebhookConfiguration` over the machine resource to let each provider define their API defaulting and validation.

### User Stories

#### Story 1

As an operator I want the product to communicate meaningfully when my input is bad before it gets persisted so I can fix it and save time.

### Implementation Details/Notes/Constraints

#### Plumbing
- The Machine API Operator (MAO) will expose both a `MutatingWebhookConfiguration` and a `ValidatingWebhookConfiguration` to the Cluster Version Operator (CVO).

- The MAO will expose to the CVO a `Service` referenced in the webhook config that will resolve to the `Endpoint` where the webhook is running.

- The webhook server can be managed by the same manager that manages
  the MachineSet controller. This is a convenient place so it will let
  us manage single `MutatingWebhookConfiguration` and
  `ValidatingWebhookConfiguration` resources that can be extended with
  multiple `paths` for other machine API resources, e.g `MachineSet`,
  `machineHealthCheck`, etc. All being reachable by the same `Service`
  resolving to the same `Endpoint`.

- The serving certs and CA bundle for the webhook server will be managed by the https://github.com/openshift/service-ca-operator via `service.beta.openshift.io/serving-cert-secret-name:` and `"service.beta.openshift.io/inject-cabundle": "true"` annotations.

#### Implementation

Since most of the machine API controllers rely on controller-runtime we can leverage it to define the webhook server as well.
We could implement `webhook.Defaulter` and `webhook.Validator` interfaces as in https://book.kubebuilder.io/cronjob-tutorial/webhook-implementation.html.

However we need a more granular level of customisation which is not satisfied by those interfaces. Particularly we need to know the provider we are running on (to run the appropriate defaulting/validation) and the clusterID (to satisfy some defaults).
To that end we can implement the [Handler interface](https://godoc.org/github.com/kubernetes-sigs/controller-runtime/pkg/webhook/admission#Handler) enabling us to infer the values above at the time of instantiating the webhook rather than on each request at runtime.

### Risks and Mitigations

As we introduce stronger opinions on defaults and validation we might break automated consumers that were relying on faulty behaviour.
New defaults and validation needs to be reviewed carefully for each particular resource.

## Design Details

### Test Plan

Extensive unit testing to cover defaulting and validation business logic.
e2e testing on the Machine API test suite to validate we reject invalid input.

### Graduation Criteria


### Upgrade / Downgrade Strategy

The webhook will start gating upcoming machine API requests as it runs with new payload.
Existing functional machines should remain untouched. Either because we choose to disable defaulting for update requests or because our defaults should not apply to non-required fields.

### Version Skew Strategy


## Implementation History

[AWS implementation](https://github.com/openshift/machine-api-operator/pull/601)

## Drawbacks


## Alternatives

- Introduce a new CRD for the `providerSpec`. Externalise it from the machine object. Introduce an `objectRef` in the machine object to reference `providerSpec` resources. This would let the `providerSpec` become a natural extension of the Kubernetes API. Although we still might need a webhook for clever defaulting/validation we'd get for free all the features coming from [structural schemes](https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/20190425-structural-openapi.md)

The above is still desirable but more of an orthogonal long term goal. Once we go that route it means API breaking changes will be disruptive for users.

In order to improve the current UX with defaults and validation, this alternative is not a requirement but a complement.

- Instead of running the webhook server from one of the cross provider
  controllers i.e MachineSet we could provide some facility library
  and let each particular provider to manage it. This split would
  increase complexity and would only let us enable the
  `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration`
  once every single provider implementation have udpated their machine
  controller to run the webhook server to satisfy the `Endpoint` for
  the `Service`.

To avoid the above we could also move the responsability of managing the `MutatingWebhookConfiguration` and `ValidatingWebhookConfiguration` down to the provider machine controller.

That would mean we'd need an additional webhook config, service and server for the other machine API resources e.g MachineSet, machineHealthCheck or tie their lifecycle to the particular machine controllers as well which makes it impractical.

Therefore we favour simplicity and control by defining, plumbing and running the webhook for all providers in the Machine API Operator repo.
