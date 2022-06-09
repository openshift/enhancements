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
  - @tkashem
  - @tjungblu
approvers:
  - @tjungblu
creation-date: 2021-03-24
last-updated: 2022-03-10
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
- Tags must be applied at creation time, in an atomic operation. It isn't acceptable to create an object and,
after some period of time, apply the tags post-creation.

### Non-Goals

- To reduce initial scope, tags are applied only at creation time and not reconciled. If an administrator manually
changes the tags stored in the infrastructure resource, behavior is undefined. See below.
- To reduce initial scope, we are not implementing this for clouds other than AWS. We will not take any actions
to prohibit that later.

## Proposal

### Existing design details

New `experimentalPropagateUserTags` field added to `.platform.aws` of install config to indicate that the user tags should be applied to AWS
resources created by in-cluster operators.

If `experimentalPropagateUserTags` is true, install validation will fail if there is any tag that starts with `kubernetes.io` or `openshift.io`.

Add a new field `resourceTags` to `.status.aws` of the `infrastructure.config.openshift.io` type. Tags included in the
`resourceTags` field will be applied to new resources created for the cluster. The `resourceTags` field will be populated by the installer only if the `experimentalPropagateUserTags` field is true.

Note existing unchanged behavior: The installer will apply these tags to all AWS resources it creates with terraform (e.g. bootstrap and master EC2 instances) from the install config, not from infrastructure status, and regardless if the propagation option is set.

All operators that create AWS resources (ingress, cloud credential, storage, image registry, machine-api)
will apply these tags to all AWS resources they create.

userTags that are specified in the infrastructure resource will merge with userTags specified in a kube resource. In the case where a userTag is specified in the infrastructure resource and there is a tag with the same key specified in a kube resource, the value from the kube resource will have precedence and be used.

The userTags field is intended to be set at install time and is considered immutable. Components that respect this field must only ever add tags that they retrieve from this field to cloud resources, they must never remove tags from the existing underlying cloud resource even if the tags are removed from this field(despite it being immutable).

If the userTags field is changed post-install, there is no guarantee about how an in-cluster operator will respond to the change. Some operators may reconcile the change and change tags on the AWS resource. Some operators may ignore the change. However, if tags are removed from userTags, the tag will not be removed from the AWS resource.

### Update design details

The `experimentalPropagateUserTags` field is changed to `propagateUserTags` for GA version.

### User Stories

https://issues.redhat.com/browse/SDE-1146 - IAM users and roles can only operate on resources with specific tags
As a security-conscious ROSA customer, I want to restrict the permissions granted to Red Hat in my AWS account by using
AWS resource tags.

### API Extensions

```yaml
apiVersion: apiextensions.k8s.io/v1
  kind: CustomResourceDefinition
  name: installconfigs.install.openshift.io
  spec:
    versions:
    - name: v1
      schema:
        openAPIV3Schema:
          description: InstallConfig is the configuration for an OpenShift install.
          properties:
            platform:
              aws:
                propagateUserTags:
                description: PropagateUserTags is a flag that directs in-cluster operators to include the specified
                            user tags in the tags of the AWS resources that the operators create.
                type: boolean
```

### Risks and Mitigations

## Design Details

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

- Upgrade/downgrade testing
- Sufficient time for feedback
- Available by default
- Stress testing for scaling and tag update scenarios

#### Removing a deprecated feature

This enhancement updates `experimentalPropagateUserTags` field.

### Upgrade / Downgrade Strategy

On upgrade:

The new status field won't be populated since it is only populated by the installer and that can't have happened if the cluster was installed from a prior version. Components that consume the new field should take no action since they will see no additional tags.

On downgrade:

The status/spec field may remain populated, components may or may not continue to tag newly created resources with the additional tags depending on whether or not a given component still has logic to respect the status tags, after the downgrade.

### Version Skew Strategy

## Implementation History

## Drawbacks

If a customer decides they do not want these tags applied to new resources, there is no clean way to address that need:

1. they would have to get help from support to edit the status field
2. they would have to manually remove the undesired tags from any existing resources

In the future we will want to introduce a spec field where a customer can specify and edit these tags, which will be reflected into status when changed, and we will expect consuming components to reconcile changes to the set of tags by applying net new tags to existing resources (but they will still not remove tags that are dropped from the list of tags).

## Alternatives

Proposal for managing user-defined tags in day2 operations is discussed in the following sections.

## What motivates

Motivations include but are not limited to:

- Allow admin, compliance, and security teams to keep track of assets and objects created by OpenShift,
both at install time and during continuous operation (Day 2)
- In a Managed OpenShift environment such as Red Hat OpenShift on AWS (ROSA), allow easy differentiation
between Red Hat-managed AWS objects and customer-managed objects
- *Allow for the restriction of permissions granted to Red Hat in an AWS account by AWS resource tags.
For example, see https://issues.redhat.com/browse/SDE-1146 - "IAM users and roles can only operate on resources with specific tags"*

### Identified goal

- The administrator or service (in the case of Managed OpenShift) installing OpenShift can pass an arbitrary
list of user-defined tags to the OpenShift Installer, and everything created by the installer and all other
bootstrapped components will apply those tags to all resources created in AWS, for the life of the cluster, and where supported by AWS.
- User-defined tags can be applied at creation time, as an atomic operation.
- User-defined tags can be updated post creation.

### Identified non-goal

- To reduce initial scope, the proposal does not apply for clouds other than AWS. There will be no actions taken to prohibit that later.

### Identified Limitations

- User-defined tags for PV volumes cannot be updated but only set during creation of volumes using `--extra-tags`. Present CSI spec does not support `UpdateVolume` to update volume metadata.
Restarting csi driver to initiate multiple `CreateVolume` will depend on underlying storage layer idempotency implementation for the operation. It is recommended to use an interface from CSI spec which enforces idempotency guarantees for volume metadata updates.
In case of AWS EC2, `CreateVolume` returns `IdempotentParameterMismatch` error when identical request is not found for the same client token.
Also, the CSI spec `https://github.com/container-storage-interface/spec/blob/master/spec.md` mentions grpc error code
`6 ALREADY_EXISTS` for `a volume corresponding to the specified volume name already exists but is incompatible with the specified capacity_range, volume_capabilities, parameters, accessibility_requirements or volume_content_source`.
- User-defined tags cannot be updated for vpc, security groups, elb(not managed by in-cluster operators), route53, subnet resources as there is no operator managing the resources post installation.
User-defined tags cannot be updated to an AWS resource which is not managed by an operator. In this proposal, the changes proposed and developed will be part of openshift-* namespace. External operators are not in scope.
- User-defined tags can be updated on the following AWS resources.
1. EC2 instances for master and worker nodes.
2. Image registry.
3. Ingress LB.
4. IAM credentials by CCO in mint mode of operation.
- OpenShift is bound by the AWS resource that allows the fewest number of tags. An AWS S3 bucket can have at most 10 tags.

## Proposed design

### New proposal to support update of user-defined tags (or userTags)

- `experimentalPropagateUserTags` field will be set for deprecation in this proposal version.
`experimentalPropagateUserTags` will be changed to a `*bool`, where an omitted value means that the user tags will be propagated to the cluster.

The install validation will fail if there is any tag that starts with `kubernetes.io` or `openshift.io` in `userTags`. In the existing design, kubernetes.io and openshift.io namespaces are not allowed as part of user-defined tags.

The `resourceTags` field in `.spec.platformSpec.aws` will be populated by the installer using the entries from `userTags` field.

`.spec.platformSpec.aws` is a mutable field and `.status.platformStatus.aws` is immutable.
`.status.platformStatus.aws` will have older version tags defined and is required for upgrade case.

- All operators that create AWS resources (ingress, cloud credential, storage, image registry, machine-api) will apply these tags to all AWS resources they create.
Operator must update AWS resource to match requested user-defined tags provided in `.spec.platformSpec.aws` (or `.status.platformStatus.aws`).

Operator must consider `.status.platformStatus.aws` to support upgrade scenarios.

#### Create tags scenarios
`resourceTags` that are specified in  `.spec.platformSpec.aws` of the Infrastructure resource will merge with user-defined tags in a kube resource.

In the case where a user-defined tag is specified in the Infrastructure resource and
1) There is already a tag with the same key present for AWS resource, the value from the AWS resource will be replaced.

For example,

Existing tag for AWS resource = `key_infra1 = value_comp1`

New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value_infra1`

Action = Existing tag for AWS resource is updated to reflect new value.

Final tag set to AWS resource = `key_infra1 = value_infra1`

Event action = An event is generated to notify user about the request status (success/failure) to update tags for the AWS resource.

2) There is no tag with same key present, new tag is created for the AWS resource. In case of limit reached, a validation error is generated.

For example,

New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value_infra1`

Action = A new tag is created for AWS resource.

Final tag set to AWS resource = `key_infra1 = value_infra1`

Event action = An event is generated to notify user about the request status (success/failure) to create tags for the AWS resource.

#### Update tags scenarios
Users can update the user-defined tags by editing `.spec.platformSpec.aws` of the `Infrastructure.config.openshift.io` type.
On update of `resourceTags`, AWS resource is not created or restarted.

In the case where a user-defined tag is specified in the Infrastructure resource and
1) There is already a tag with the same key and value present for AWS resource, the tag for the AWS resource will not be updated.

For example,

Existing tag for AWS resource = `key_infra1 = value_update1`

New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value_update1`

Action = There is no update for AWS resource.

Final tag set to AWS resource = `key_infra1 = value_update1`

Event action = An event is generated to notify user about the request status (success/failure) to update tags for the AWS resource.

2) There is already a tag with the same key and different value present for AWS resource, the AWS resource will be updated with the user-defined tag value.

For example,

Existing tag for AWS resource = `key_infra1 = value_old`

New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value_update1`

Action = Existing tag for AWS resource is updated to reflect new value.

Final tag set to AWS resource = `key_infra1 = value_update1`

Event action = An event is generated to notify user about the request status (success/failure) to update tags for the AWS resource.

#### Precedence scenarios
1) User-defined tags on kube resources MUST continue to take precedence during create and update.
When there is a conflict between the user tags specified in the Infrastructure resource and the kube resource, the event generated on updates to the AWS resource tags will include details about the conflict. Note that if the conflict does not cause an update to the AWS resource tags, then the conflict will not be registered in an event.

For example,

Existing tag in `.spec.platformSpec.aws.resourceTags` has `key_infra1 = value1`

Tags specified in a kube resource updated to include a different value `key_infra1 = value2`

Action = The tag for AWS resource is updated to `key_infra1 = value2`.

Final tag set to AWS resource = `key_infra1 = value2`

Event action = The event generated to notify the user about the request status (success) to update tags for the AWS resource will include details stipulating that the user tag specified for "key_infra1" in the Infrastructure resource was superseded by the tag specified for "key_infra1" in the kube resource.

2) When a new user-defined tag is added on kube resources, it MUST override the tag in `.spec.platformSpec.aws`.
A warning is generated to inform the user about action not being applied on the list of user tags.

For example,

Existing tag in Infrastructure CR = `key_infra1 = custom_value`

New tag added in kube resource object like machine CR = `key_infra1 = value1`

Action = The tag for AWS resource is updated to `key_infra1 = value1`.

Final tag set to AWS resource = `key_infra1 = value1`

Event action = An event is generated to notify user about the request status (success) to update tags for the AWS resource. A warning also must be generated about list of user-defined tags on which action did a override.

3) `.spec.platformSpec.aws` take precedence over `.status.platformStatus.aws`. User-defined tags must be merged from `.status.platformStatus.aws` and set to AWS resource.

#### Caveats
1) User updates a resource's tag using an external tool when there is an entry in `.spec.platformSpec.aws.resourceTags` or kube resource.
The resource's tag will be reconciled by its owning operator to the value from `.spec.platformSpec.aws.resourceTags` or kube resource.
In this case, the desired value is not set immediately for AWS resource by the owning operator. There is eventual consistency maintained by the owning operator.
The time taken to reconcile the modified user-defined tag on AWS resource to desired value vary across owning operators.

For example,

Edited existing tag using external tool for AWS resource = `key_infra1 = value_comp1`

Previous tag request = `.spec.platformSpec.aws.resourceTags` or kube resource has `key_infra1 = value_infra1`

Action = Update existing tag with value from `.spec.platformSpec.aws.resourceTags`.

Final tag set to AWS resource = `key_infra1 = value_infra1`

Event action = An event is generated to notify user about the request status (success/failure) to update tags for the AWS resource.

2) User removes the user-defined tag from `.spec.platformSpec.aws.resourceTags` or kube resource.
The user-defined tag which is removed from spec will not be reconciled or managed by operators.

User can update user-defined tag key:value using external tool. The user-defined tag will not be overwritten by the operator.

For example,

Existing tag for AWS resource = `key_infra1 = value1`

New tag request = `.spec.platformSpec.aws.resourceTags` has no user-defined tag with key `key_infra1`

Action = No change in tags for the AWS resource.

Final tag set to AWS resource = `key_infra1 = value1`

Event action = No event

3) Any user-defined tag set using `.spec.platformSpec.aws.resourceTags` in `Infrastructure.config.openshift.io/v1` type will affect all managed AWS resources.

4) User-defined tags are not synced from `.spec.platformSpec.aws.resourceTags` to `.status.platformStatus.aws.resourceTags` for the following reasons.

- User-defined tags in `.spec.platformSpec.aws.resourceTags` can be "removed without delete" or "update".
`.spec.platformSpec.aws.resourceTags` to `.status.platformStatus.aws.resourceTags` sync is not required,
as there will be no versioning of the user-defined tags required to override the user-defined tags in Infrastructure CR.
Instead, the user-defined tags (if supported in resource operator spec field, e.g: machine) can override the user-defined tags in Infrastructure CR.

- As `.spec.platformSpec.aws.resourceTags` has the actual values. Sync to `.status.platformStatus.aws.resourceTags` was required in earlier design to identify if the
tag on the resource was created using Infrastructure CR or external tool. Identifying the creator tool was done to restrict user from editing the same user-tag kv pair using multiple tools.
This inherently poses many scenarios of conflict which will result in user-defined tag kv pair being inconsistent across cluster when applied using Infrastructure CR. Hence, user will be confused which tool to be used to update tag.

### Stories identified

- As a security-conscious ROSA customer, I want to restrict the permissions granted to Red Hat in my AWS account by using AWS resource tags.
Red Hat applies one well-known tag to all ROSA cluster resources to identify Red Hat-managed AWS resources (e.g. “red-hat-managed=true”).
Customer can create AWS policies that restrict Red Hat access to tagged resource types based on tag (e.g. “RedHat role can only create/read/update/delete S3 buckets with a red-hat-managed=true tag”).

- As a cluster administrator of OpenShift, I would like to be able to update user-defined tags managed by OpenShift for AWS resources. User will need to update the user-defined tags to set different values during ongoing operations.
This can be supported by allowing edit on mutable `.spec.platformSpec.aws.resourceTags` field in Infrastructure CR.

- As a cluster administrator of OpenShift, I would like to be able to remove user-defined tags from `.spec.platformSpec.aws.resourceTags` without deleting for AWS resources.
This enables, user-defined tags to be modified on AWS without being overridden by OpenShift operators. The user can adopt new 3rd party component to manage tags.

- As a cluster administrator of OpenShift, I expect user-defined tags added in Infrastructure CR are reconciled and desired user-defined tags maintained on AWS resources.
The latest updates on `.spec.platformSpec.aws.resourceTags` field must be reconciled to AWS resources and desired user-defined tags maintained when being managed by OpenShift.
Any modifications which are external to OpenShift must be reconciled to desired user-defined tags from `.spec.platformSpec.aws.resourceTags`.

The following user stories are added to support existing methods of updating user-defined tags which override `.spec.platformSpec.aws.resourceTags` to avoid breaking change.
As the existing cluster might be configured with user-defined tags from kube resources, the precedence is given to user-defined tags in kube resources.
- As a cluster administrator of OpenShift, I expect user-defined tags added in kube resources override the entries in `.spec.platformSpec.aws.resourceTags`.

- As a cluster administrator of OpenShift, I expect user-defined tags added in kube resources must not be overridden by update action or by removing user-defined tags from `.spec.platformSpec.aws.resourceTags`.

### Required updates in API

This proposal edits `Infrastructure.config.openshift.io/v1` type

The Infrastructure resource example to use spec for api changes

```go
// AWSPlatformSpec holds the desired state of the Amazon Web Services Infrastructure provider.
// This only includes fields that can be modified in the cluster.
type AWSPlatformSpec struct {
// Existing fields
...
// ResourceTags is a list of additional tags to apply to AWS resources created for the cluster.
// See https://docs.aws.amazon.com/general/latest/gr/aws_tagging.html for information on tagging AWS resources.
// AWS supports a minimum of 10 tags per resource. OpenShift reserves 5 tags for its use, leaving 5 tags
// available for the user.
// ResourceTags field is mutable and items can be removed.
// When a tag is removed from this list, management of the tag is stopped.
// The tag will remain in an unmanaged state on any existing resource.
// +kubebuilder:validation:MaxItems=5
// +optional
ResourceTags []AWSResourceTag `json:"resourceTags,omitempty"`
}

// Existing type
// AWSResourceTagSpec is a tag to apply to AWS resources created for the cluster.
type AWSResourceTagSpec struct {
  // key is the key of the AWS tag. The value must consist only of alphanumeric and certain special characters(_.:\/=+-@).
  // key should not start with "openshift.io" or "kubernetes.io". Both of these prefixes are reserved for use by the platform.
  // key must be no more than 128 characters in length.
  // +kubebuilder:validation:Required
  // +kubebuilder:validation:MinLength=1
  // +kubebuilder:validation:MaxLength=128
  // +kubebuilder:validation:Pattern=`^[0-9A-Za-z_.:/=+-@]+$`
  // +required
  Key string `json:"key"`
  // value is the value of the AWS tag. The value must consist only of alphanumeric and certain special characters(_.:\/=+-@).
  // value must be no more than 256 characters in length.
  // Some AWS service do not support empty values. Since tags are added to resources in many services, the
  // length of the tag value must meet the requirements of all services.
  // +kubebuilder:validation:Required
  // +kubebuilder:validation:MinLength=1
  // +kubebuilder:validation:MaxLength=256
  // +kubebuilder:validation:Pattern=`^[0-9A-Za-z_.:/=+-@]+$`
  // +required
  Value string `json:"value"`
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
    maxItems: 5
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

## Short desgin summary

User can add tags during installation for creation of resources with tags. Installer creates infrastructure manifests with `resourceTags` to `.spec.platformSpec.aws` of the `Infrastructure.config.openshift.io`.
Operators that operate on aws resources must consider the tags from `.spec.platformSpec.aws`, `.status.platformStatus.aws`, kube resource and existing AWS resource tags.

The values from `status.platformStatus.aws` will be used to only support older version.

`.status.platformStatus.aws` will be deprecated in the future versions.

### Test scenarios identified

- Upgrade/downgrade testing
- Sufficient time for feedback
- Available by default
- Stress testing for scaling and tag update scenarios

### Deprecation of fields

`experimentalPropagateUserTags` field will be set for deprecation.
`.status.platformStatus.aws` will be set for deprecation.

### Affects in upgrade and downgrade scenarios

On upgrade:

On upgrade from version supporting `experimentalPropagateUserTags`, openshift components that consume the new field should merge `.status.platformStatus.aws` and `.spec.platformSpec.aws`.
`.spec.platformSpec.aws` can be updated or changed which will override the values from status.

For installer configuration compatibility, configuration with `experimentalPropagateUserTags` should be supported.

On downgrade:

The status/spec field may remain populated, components may or may not continue to tag newly created resources with the additional tags depending on whether or not a given component still has logic to respect the status tags, after the downgrade.

## Disadvantages

If a user decides that they do not want these tags applied to new resources, there is no clean way to address that need:

1. User would have to get help from support to edit the status field for older version which uses `experimentalPropagateUserTags`.
2. User would have to manually remove the undesired user-defined tags from some existing resources.
3. User would have to manually update user-defined tags for AWS resources which are not managed by an operator.
4. User would have to manually update user-defined tags for AWS resources where update logic is not supported by an operator.

## Alternatives considered to support day2 operations

1. An operator can be designed to behave similar to how the installer's destroy behaves, namely by using the resource tagging API to search for resources with the kubernetes.io/.../cluster=owned tag.
The operator would reconcile on changes to the infrastructure CR to ensure that resources owned by the cluster have tags matching what is in the infrastructure CR.
That would get tricky when it comes to resources owned by in-cluster operators, though, particularly with tags that may have different values in the corresponding kube resources.
The operator would need to monitor multiple kube resources, infrastructure CR and multiple instances of different AWS resource types.

2. A way to delete tags was considered when the user sets the user-defined tag value to an empty string in `.spec.platformSpec.aws.resourceTags`.

For example,

Existing tag for AWS resource = `key_infra1 = value_old`

New tag request = `.spec.platformSpec.aws.resourceTags` has `key_infra1 =`

Action = Existing tag for AWS resource is deleted.

Final tag set to AWS resource = deleted

Event action = An event is generated to notify user about the request status (success/failure) to delete tags for the AWS resource.

As AWS considers empty string as a valid tag value, the behaviour becomes ambiguous from the user perspective.


## Infrastructure Needed [optional]

