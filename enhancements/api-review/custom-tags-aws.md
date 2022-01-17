---
title: apply-user-defined-tags-to-all-aws-resources-created-by-openshift
authors:
  - "@gregsheremeta"
  - "@tgeer"
reviewers:
  - @joelspeed
  - @Miciah
  - @sinnykumari
  - @dmage
  - @staebler
approvers:
  - @tjungblu

creation-date: 2021-03-24
last-updated: 2022-01-07
status: implementable
---

# Apply user defined tags to all AWS resources created by OpenShift

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes a proposal to allow an administrator of OpenShift to
have the ability to apply user defined tags to many resources created by OpenShift in AWS.

Note: this enhancement is slightly retroactive. Work has already begun on this. See
- https://github.com/openshift/api/pull/864
- https://github.com/openshift/cluster-ingress-operator/pull/578
- https://github.com/openshift/api/pull/1064/

## Motivation

Motivations include but are not limited to:

- allow admin, compliance, and security teams to keep track of assets and objects created by OpenShift,
   both at install time and during continuous operation (Day 2)
- in a Managed OpenShift environment such as Red Hat OpenShift on AWS (ROSA), allow easy differentiation
   between Red Hat-managed AWS objects and customer-managed objects
- *allow for the restriction of permissions granted to Red Hat in an AWS account by AWS resource tags.
   For example, see https://issues.redhat.com/browse/SDE-1146 - "IAM users and roles can only operate on resources with specific tags"*

### Goals

- the administrator or service (in the case of Managed OpenShift) installing OpenShift can pass an arbitrary
   list of user-defined tags to the OpenShift Installer, and everything created by the installer and all other
   bootstrapped components will apply those tags to all resources created in AWS, for the life of the cluster, and where supported by AWS.
- tags can be applied at creation time, in an atomic operation.
- tags can be updated post creation.

### Non-Goals

- to reduce initial scope, we are not implementing this for clouds other than AWS. We will not take any actions
   to prohibit that later.

### Limitations

- tags for PV volumes cannot be updated but only set during creation  of volume using `--extra-tags`.Present CSI spec does not support `UpdateVolume` to change the tags on the volume.

## Proposal

New `propagateUserTags` field added to `.platform.aws` of install config to indicate that the user tags should be applied to AWS
resources created by in-cluster operators.

`propagateUserTags` field is by default set to true.
`experimentalPropagateUserTags` field will be set for deprecation.

If `propagateUserTags` is not set to false, install validation will fail if there is any tag that starts with `kubernetes.io` or `openshift.io`.

Add a new field `resourceTags` to `.spec.platformSpec.aws` of the `Infrastructure.config.openshift.io` type. Tags included in the
`resourceTags` field will be applied to new resources created for the cluster. The `resourceTags` field will be populated by the installer only if the `propagateUserTags` field is not set to false.

`.spec.platformSpec.aws` is a mutable field and `.status.platformStatus.aws` is immutable.

Note existing unchanged behavior: The installer will apply these tags to all AWS resources it creates with terraform (e.g. bootstrap and master EC2 instances) from the install config, not from infrastructure status, and regardless if the propagation option is set.

All operators that create AWS resources (ingress, cloud credential, storage, image registry, machine-api)
will apply these tags to all AWS resources they create.

userTags that are specified in the Infrastructure resource will merge with userTags specified in an individual component. In the case where a userTag is specified in the Infrastructure resource and there is a tag with the same key specified in an individual component, the value from the individual component will have precedence and be used.

Users can update the `resourceTags` by editing `.spec.platformSpec.aws` of the `Infrastructure.config.openshift.io` type. New addition of tags are always appended. Any update in the existing tags, which are added by installer or  by edit in `resourceTags`, will replace the previous tag value.
On update of `resourceTags`, AWS resource is not created or restarted.

`.status.platformStatus.aws.resourceTags` reflects the present set of userTags. In case when the userTags are created by installer or newly added in Infrastructure resource is updated on the individual component directly using external tools, the value from Infrastructure resource will have the precedence.

The precedence helps to maintain creator/updator tool (in-case of external tool usage) remains same for user-defined tags which are created correspondingly.

The userTags field is intended to be set at install time and updatable (not allowed to delete). Components that respect this field must only ever add tags that they retrieve from this field to cloud resources, they must never remove tags from the existing underlying cloud resource even if the tags are removed from this field.

If the userTags field is changed post-install, there is no guarantee about how an in-cluster operator will respond to the change. Some operators may reconcile the change and change tags on the AWS resource. Some operators may ignore the change. However, if tags are removed from userTags, the tag will not be removed from the AWS resource.

The Infrastructure resource example to involve spec for api changes

```go
// AWSPlatformSpec holds the desired state of the Amazon Web Services Infrastructure provider.
// This only includes fields that can be modified in the cluster.
type AWSPlatformSpec struct {
    // Existing fields
    ...
    // ResourceTags is a list of additional tags to apply to AWS resources created for the cluster.
    // See https://docs.aws.amazon.com/general/latest/gr/aws_tagging.html for information on tagging AWS resources.
    // AWS supports a maximum of 50 tags per resource. OpenShift reserves 25 tags for its use, leaving 25 tags
    // available for the user.
    // While ResourceTags field is mutable, items can not be removed.
    // +kubebuilder:validation:MaxItems=25
    // +optional
    ResourceTags []AWSResourceTag `json:"resourceTags,omitempty"`
}

```

```yaml
spec:
description: AWS contains settings specific to the Amazon Web Services Infrastructure provider.
type: object
properties:
    resourceTags:
    description: ResourceTags is a list of additional tags to apply to AWS resources created for the cluster. See https://docs.aws.amazon.com/general/latest/gr/aws_tagging.html for information on tagging AWS resources.
    type: array
    maxItems: 25
    items:
        description: AWSResourceTag is a tag to apply to AWS resources created for the cluster.
        type: object
        required:
            - key
            - value
        properties:
            key:
                description: key is the key of the tag
                type: string
                maxLength: 128
                minLength: 1
                pattern: ^[0-9A-Za-z_.:/=+-@]+$
            value:
                description: value is the value of the tag. Some AWS service do not support empty values. Since tags are added to resources in many services, the length of the tag value must meet the requirements of all services.
                type: string
                maxLength: 256
                minLength: 1
                pattern: ^[0-9A-Za-z_.:/=+-@]+$
```

### User Stories

https://issues.redhat.com/browse/SDE-1146 - IAM users and roles can only operate on resources with specific tags
As a security-conscious ROSA customer, I want to restrict the permissions granted to Red Hat in my AWS account by using
AWS resource tags.

### API Extensions

This proposal edits `Infrastructure.config.openshift.io/v1` type

### Operational Aspects of API Extensions

NA

### Risks and Mitigations

#### Failure Modes

NA

#### Support Procedures

NA

## Design Details

User can add tags during installation for creation of resources with tags. Installer creates infrastructure manifests with `resourceTags` to `.spec.platformSpec.aws` of the `Infrastructure.config.openshift.io`.
Cluster config operator reconciles Infrastructure object, validates and updates `resourcesTags` from `.spec.platformSpec.aws` to  `.status.platformStatus.aws`.
Operators that operate on aws resources must merge the tags from `.spec.platformSpec.aws`, `.status.platformStatus.aws` and individual component tags.

The values from `status.platformStatus.aws` will used to only support downgrade.

In case of precedence conflict or errors, the same will be reported using `Events` by the operators.

`.status.platformStatus.aws` can be deprecated in the future versions when there is a CRD validation possible to avoid removal of userTags from `.spec.platformSpec.aws`.

### Test Plan

Update the `resourceTags` testcases to check successful update from Infrastructure resource.

### Graduation Criteria

#### Dev Preview -> Tech Preview

NA

#### Tech Preview -> GA

- Upgrade/downgrade testing
- Sufficient time for feedback
- Available by default
- Stress testing for scaling and tag update scenarios

#### Removing a deprecated feature

`experimentalPropagateUserTags` field will be set for deprecation.
`.status.platformStatus.aws` will be set for deprecation.

### Upgrade / Downgrade Strategy

On upgrade:

On upgrade from version supporting `experimentalPropagateUserTags`, openshift components that consume the new field should merge `.status.platformStatus.aws` and `.spec.platformSpec.aws`.
`.spec.platformSpec.aws` can be updated or changed which will override the values from status and a new `.status.platformStatus.aws` will be set.

On downgrade:

The status field may remain populated, components may or may not continue to tag newly created resources w/ the additional tags depending on whether or not a given component still has logic to respect the status tags, after the downgrade.

### Version Skew Strategy

TBD

## Implementation History

## Drawbacks

If a customer decides they do not want these tags applied to new resources, there is no clean way to address that need:

1. they would have to get help from support to edit the status field
2. they would have to manually remove the undesired tags from any existing resources

In spec field where a customer can specify and edit these tags, which will be reflected into status when changed, and we will expect consuming components to reconcile changes to the set of tags by applying net new tags to existing resources (but they will still not remove tags that are dropped from the list of tags).

## Alternatives

## Infrastructure Needed [optional]

