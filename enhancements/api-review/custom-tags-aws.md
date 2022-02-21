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
  - @tjungblu
approvers:
  - @tjungblu
creation-date: 2021-03-24
last-updated: 2022-02-10
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

- Allow admin, compliance, and security teams to keep track of assets and objects created by OpenShift,
   both at install time and during continuous operation (Day 2)
- In a Managed OpenShift environment such as Red Hat OpenShift on AWS (ROSA), allow easy differentiation
   between Red Hat-managed AWS objects and customer-managed objects
- *Allow for the restriction of permissions granted to Red Hat in an AWS account by AWS resource tags.
   For example, see https://issues.redhat.com/browse/SDE-1146 - "IAM users and roles can only operate on resources with specific tags"*

### Goals

- The administrator or service (in the case of Managed OpenShift) installing OpenShift can pass an arbitrary
   list of user-defined tags to the OpenShift Installer, and everything created by the installer and all other
   bootstrapped components will apply those tags to all resources created in AWS, for the life of the cluster, and where supported by AWS.
- User-define tags can be applied at creation time, in an atomic operation.
- User-defined tags can be updated post creation.

### Non-Goals

- To reduce initial scope, the proposal does not apply for clouds other than AWS. There will be no actions taken to prohibit that later.

### Limitations

- User-defined tags for PV volumes cannot be updated but only set during creation  of volume using `--extra-tags`.Present CSI spec does not support `UpdateVolume` to update volume metadata.
   Restarting csi driver to initiate multiple `CreateVolume` will depend on underlying storage layer idempotency implementation for the operation. It is recommended to use a interface from CSI spec which enforces idempotency guarantees for volume metadata updates.
   In case of AWS EC2  `CreateVolume`, returns `IdempotentParameterMismatch` error when identical request is not found for the same client token.
   Also, the CSI spec `https://github.com/container-storage-interface/spec/blob/master/spec.md` mentions grpc error code
   `6 ALREADY_EXISTS` for `a volume corresponding to the specified volume name already exists but is incompatible with the specified capacity_range, volume_capabilities, parameters, accessibility_requirements or volume_content_source`.
- User-defined tags cannot be updated for vpc, security groups, elb, route53, subnet resources as there is no operator managing the resources post installation.
   User-defined tags cannot be updated to an AWS resource which is not managed by an operator in openshift-* namespace.
- User-defined tags can be updated on the following AWS resources.
  1. EC2 instances for master and worker nodes.
  2. Image registry.
  3. Ingress LB.
  4. IAM credentials by CCO in mint mode of operation.

## Proposal

- Existing `experimentalPropagateUserTags` will be renamed to `propagateUserTags` in `.platform.aws` of install config to indicate that the user tags should be applied to AWS
resources created by in-cluster operators.\
  `experimentalPropagateUserTags` field will be set for deprecation.

  If `propagateUserTags` is set to true, install validation will fail if there is any tag that starts with `kubernetes.io` or `openshift.io`.

- Add a new field `resourceTags` to `.spec.platformSpec.aws` of the `Infrastructure.config.openshift.io` type. Tags included in the
  `resourceTags` field will be applied to new resources created for the cluster. The `resourceTags` field will be populated by the installer only if the `propagateUserTags` field is true.

  `.spec.platformSpec.aws` is a mutable field and `.status.platformStatus.aws` is immutable.
  `.status.platformStatus.aws` will have older version tags defined and is required for upgrade case.

- All operators that create AWS resources (ingress, cloud credential, storage, image registry, machine-api) will apply these tags to all AWS resources they create.\
  Operator must update the AWS resource within (5 minutes + cloud provide API response time).\
  Operator must update AWS resource to match requested user-defined tags provided in `.spec.platformSpec.aws` (or `.status.platformStatus.aws`).\
  Operator must consider `.status.platformStatus.aws` when AWS resource is created. `.status.platformStatus.aws` can be ignored for user-defined tag update/delete.

### Create tags scenarios
`resourceTags` that are specified in  `.spec.platformSpec.aws` of the Infrastructure resource will merge with user-defined tags in an individual component.

In the case where a user-defined tag is specified in the Infrastructure resource and
1) There is already user tag with the same key present for AWS resource, the value from the AWS resource will be replaced.\
   For example,\
   Existing tag for AWS resource = `key_infra1 = value_comp1`\
   New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value_infra1`\
   Action = Existing tag for AWS resource is updated to reflect new value.\
   Final tag set to AWS resource = `key_infra1 = value_infra1`\
   Event action = An event is generated to notify user about the action status (success/failure) to update tags for the AWS resource.

2) There is no tag with same key present, new user-defined tag is created for the AWS resource. In case of limit reached, a validation error is generated.\
   For example,\
   Existing tag for AWS resource = `key_infra1 = value_comp1`\
   New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value_infra1`\
   Action = A new tag is created for AWS resource.\
   Final tag set to AWS resource = `key_infra1 = value_infra1`\
   Event action = An event is generated to notify user about the action status (success/failure) to create tags for the AWS resource.

### Update tags scenarios
Users can update the user-defined tags by editing `.spec.platformSpec.aws` of the `Infrastructure.config.openshift.io` type. New addition of tags are always appended.
On update of `resourceTags`, AWS resource is not created or restarted.

In the case where a user-defined tag is specified in the Infrastructure resource and
1) There is already user-defined tag with the same key and value present for AWS resource, the user-define tag value for the AWS resource will not be updated.\
   For example,\
   Existing tag for AWS resource = `key_infra1 = value_update1`\
   New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value_update1`\
   Action = There is no update for AWS resource.\
   Final tag set to AWS resource = `key_infra1 = value_update1`\
   Event action = An event is generated to notify user about the action status (success/failure) to update tags for the AWS resource.

2) There is already user-defined tag with the same key and different value present for AWS resource, the user-define tag value for the AWS resource will be updated.\
   For example,\
   Existing tag for AWS resource = `key_infra1 = value_old`\
   New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value_update1`\
   Action = Existing tag for AWS resource is updated to reflect new value.\
   Final tag set to AWS resource = `key_infra1 = value_update1`\
   Event action = An event is generated to notify user about the action status (success/failure) to update tags for the AWS resource.

3) The new user-defined tag request has empty string `""`, the user-defined tag value for the AWS resource will be updated. Please refer to the `Delete Scenarios` for more details on empty value string handling.\
   For example,\
   Existing tag for AWS resource = `key_infra1 = value_old`\
   New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 =`\
   Action = Existing tag for AWS resource is updated to reflect new value.\
   Final tag set to AWS resource = `key_infra1 = value_update1`\
   Event action = An event is generated to notify user about the action status (success/failure) to update tags for the AWS resource. A warning also must be generated about user-defined tag being marked for deletion.

### Delete tags scenarios
User-defined tags are deleted when the user sets the user-defined tag value to empty string in `.spec.platformSpec.aws.resourceTags`.
Also refer to `Precedence` scenario where delete of user-defined tag is not allowed to delete.\
For example,\
Existing tag for AWS resource = `key_infra1 = value_old`\
New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 =`\
Action = Existing tag for AWS resource is deleted.\
Final tag set to AWS resource = deleted\
Event action = An event is generated to notify user about the action status (success/failure) to delete tags for the AWS resource.

### Precedence scenarios
1) User-defined tags on local objects MUST continue to take precedence during create and update. User-defined tags found on local objects must not be deleted by methods described in `Delete tags scenarios`.
   A warning is generated to inform the user about action not being applied on the list of user tags.

   For example,\
   Existing tag in local object like machine CRD = `key_infra1 = custom_value`\
   New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value1`\
   Action = The tag for AWS resource is maintained to `key_infra1 = custom_value`.\
   Final tag set to AWS resource = `key_infra1 = value_update1`\
   Event action = An event is generated to notify user about the action status (success/failure) to update tags for the AWS resource. A warning also must be generated about list of user-defined tags on which action is not applicable.

2) `.spec.platformSpec.aws` take precedence over `.status.platformStatus.aws`. User-defined tags must be merged from `.status.platformStatus.aws` and set to AWS resource.
   On delete, user-defined must also be removed from `.status.platformStatus.aws`, if present.

### Caveats
1) User updates the user-defined tag from using external tools when there is an entry in `.spec.platformSpec.aws.resourceTags`
   The user-defined tag which is updated from spec, will be reconciled by operators to set value from `.spec.platformSpec.aws.resourceTags`.
   The user-defined tag will be overwritten with value from `.spec.platformSpec.aws.resourceTags` when there is an update to `.spec.platformSpec.aws.resourceTags` section as a whole.

   User must handle inconsistencies in `.spec.platformSpec.aws.resourceTags` and user-defined tag value for AWS resource when using multiple tools to manage tags.

   For example,\
   Edited existing tag using external tool for AWS resource = `key_infra1 = value_comp1`\
   Previous tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value_infra1`\
   Action = Update existing tag with value from `.spec.platformSpec.aws.resourceTags`.\
   Final tag set to AWS resource = `key_infra1 = value_infra1`\
   Event action = An event is generated to notify user about the action status (success/failure) to update tags for the AWS resource.

2) User deletes the user-defined tag from `.spec.platformSpec.aws.resourceTags`
   The user-defined tag which is removed from spec, will not be reconciled or managed by operators.\
   User can update user-defined tag key:value using external tool. The user-defined tag will not be overwritten.

   For example,\
   Existing tag for AWS resource = `key_infra1 = value1`\
   New tag request = `.spec.platformSpec.aws.resourceTags` has no user-defined tag with key `key_infra1`\
   Action = No change in existing user-defined tag.\
   Final tag set to AWS resource = `key_infra1 = value1`\
   Event action = No event

3) User sets user-defined tag to delete which is created using external tools.\
   There is no validation check involved for creator of user-defined tag. Any user-defined tag added by user in `.spec.platformSpec.aws.resourceTags` is considered for create/update/delete accordingly.

4) Any user-defined tag set using `.spec.platformSpec.aws.resourceTags` in `Infrastructure.config.openshift.io/v1` type has scope limited to cluster-level.

The Infrastructure resource example to use spec for api changes

```go
// AWSPlatformSpec holds the desired state of the Amazon Web Services Infrastructure provider.
// This only includes fields that can be modified in the cluster.
type AWSPlatformSpec struct {
    // Existing fields
    ...
    // ResourceTags is a list of additional tags to apply to AWS resources created for the cluster.
    // See https://docs.aws.amazon.com/general/latest/gr/aws_tagging.html for information on tagging AWS resources.
    // AWS supports a maximum of 10 tags per resource. OpenShift reserves 5 tags for its use, leaving 5 tags
    // available for the user.
    // While ResourceTags field is mutable, items can not be removed.
    // +kubebuilder:validation:MaxItems=10
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
    maxItems: 10
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
Operators that operate on aws resources must consider the tags from `.spec.platformSpec.aws`, `.status.platformStatus.aws` and existing AWS resource tags.

The values from `status.platformStatus.aws` will used to only support older version.

`.status.platformStatus.aws` will be deprecated in the future versions.

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
`.spec.platformSpec.aws` can be updated or changed which will override the values from status.

For installer configuration compatibility, configuration with `experimentalPropagateUserTags` should be supported.

On downgrade:

The status field may remain populated, components may or may not continue to tag newly created resources with the additional tags depending on whether or not a given component still has logic to respect the status tags, after the downgrade.

`experimentalPropagateUserTags` field should be generated in installer configuration to support lower version installers.

### Version Skew Strategy

## Implementation History

## Drawbacks

If a user decides that they do not want these tags applied to new resources, there is no clean way to address that need:

1. User would have to get help from support to edit the status field for older version which uses `experimentalPropagateUserTags`.
2. User would have to manually remove the undesired user-defined tags from some existing resources even though user-defined tags can be marked for deletion.

## Alternatives

## Infrastructure Needed [optional]

