---
title: add-conversion-webhook-for-ICSP
authors:
  - "@QiWang19"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-08-09
last-updated: 2021-08-09
status: implementable
see-also:
  - "/enhancements/api-review/add-repositoryMirrors-spec.md"
  - https://docs.openshift.com/container-platform/4.8/operators/understanding/olm/olm-webhooks.html
  - https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/#webhook-conversion
---

# Add conversion webhook for ImageContentSourcePolicy version conversion

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The ImageContentSourcePolicy CRD needs a conversion webhook from
operator.openshift.io/v1alpha1 to operator.openshift.io/v1alpha2 for
adding new spec `repositoryMirrors` and deprecating `repositoryDigestMirrors`.

## Motivation

With the new spec `repositoryMirrors`( [add-repositoryMirrors-spec.md](enhancements/api-review/add-repositoryMirrors-spec.md)) adding to ImageContentSourcePolicy CRD, the functionality of `repositoryDigestMirrors` will be satisfied by `repositoryMirrors`. The `repositoryDigestMirrors` should
be deprecated so we don't have to keep duplicated APIs.

The above API change motivates the version conversion from
operator.openshift.io/v1alpha1 to operator.openshift.io/v1alpha2.
For safe and gentle version conversion, the conversion webhook will be
the conversion strategy.

### Goals

Use conversion webhook for ImageContentSourcePolicy to migrate user from operator.openshift.io/v1alpha1 to operator.openshift.io/v1alpha2.

### Non-Goals

## Proposal

Create and deploy a conversion webhook server for different custom resources in [MCO](https://github.com/openshift/machine-config-operator). 
Need to implement the conversion policy between different `ImageContentSourcePolicy` versions.

Update the ImageContentSourcePolicy CustomResourceDefinition to include the v1alpha2 version,
set conversion webhook as conversion strategy, and configure CustomResourceDefinition to call the webhook.
So it is safe for some clients to use the old version while others use the new version.

Operator Lifecycle Manager (OLM) can manage the lifecycle of this webhooks when they are shipped alongside operator.

### User Stories

#### As a user that currently use v1alpha1 ImageContentSourcePolicy, I would like to upgrade to v1alpine2 and config the repositoryMirrors spec 

The conversion webhook will automatically convert the
ImageContentSourcePolicies to operator.openshift.io/v1alpha2.
The user can use `oc edit imagecontentsourcepolicy.operator.openshift.io` to configure `repositoryMirrors`.

### Implementation Details/Notes/Constraints [optional]

#### Create and deploy the conversion webhook server

#### Define the conversion policy

Implement function `convertImageContentSourcePolicyCRD` defines the
conversion policy between different versions.

```go
// example: https://github.com/kubernetes/kubernetes/blob/v1.15.0/test/images/crd-conversion-webhook/converter/example_converter.go#L29-L80
func convertImageContentSourcePolicyCRD(Object *ImageContentSourcePolicy, toVersion string) (*ImageContentSourcePolicy, metav1.Status)
```

#### Configure CustomResourceDefinition to use conversion webhooks

Update the [ImageContentSourcePolicyCRD](https://github.com/openshift/api/blob/master/operator/v1alpha1/0000_10_config-operator_01_imagecontentsourcepolicy.crd.yaml).

```yml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  ...
  name: imagecontentsourcepolicies.operator.openshift.io
spec:
  group: operator.openshift.io
  ...
  versions:
    - name: v1alpha1
    ...
    # Add new version v1alpha2 and the schema
    - name: v1alpha2
      schema:
        openAPIV3Schema:
          description: ...
          type: object
          required:
            - spec
          properties:
            apiVersion:
              type: string
            kind:
              type: string
            metadata:
              type: object
            spec:
              type: object
              properties:
                repositoryMirrors:
                  type: array
                  items:
                    type: object
                    required:
                      - source
                    properties:
                      mirrors:
                        type: array
                        items:
                          type: string
                      source:
                        type: string
                      allowMirrorByTags:
                        type: bool
      served: true
      storage: false
      subresources:
        status: {}
    conversion:
      # a Webhook strategy instruct API server to call an external webhook for any conversion between custom resources.
      strategy: Webhook
      webhook:
        # conversionReviewVersions indicates what ConversionReview versions are understood/preferred by the webhook.
        # The first version in the list understood by the API server is sent to the webhook.
        # The webhook must respond with a ConversionReview object in the same version it received.
        conversionReviewVersions: ["v1alpha1","v1alpha2"]
        clientConfig:
          service:
            namespace: default
            name: example-conversion-webhook-server
            path: /crdconvert
```

### Risks and Mitigations

## Design Details

### Open Questions [optional]

### Test Plan

After adding the conversion webhook, apply the ImageContentSourcePolicy v1alpha1 version and check the CR is automatically converted into v1alpha2 schema. 

**Note:** *Section not required until targeted at a release.*

### Graduation Criteria

GA in openshift will be looked at to
determine graduation.

**Note:** *Section not required until targeted at a release.*

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

the downgrade will be affected.
Users upgraded and configured the `repositoryMirrors` spec with webhook will presumably have their CRI-O configurations clobbered and break their workflows after they downgrade to a version that lacks support for conversion webhook.

### Version Skew Strategy

Upgrade skew will not impact this feature. The MCO does not require skew check.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks


## Alternatives

## Infrastructure Needed [optional]

