---
title: apply-user-defined-tags-to-all-aws-resources-created-by-openshift
authors:
  - "@gregsheremeta"
reviewers:
  - @decarr
  - @bparees
approvers:
  - @decarr 
creation-date: 2021-03-24
last-updated: 2021-04-09
status: implementable
---

# Apply user defined tags to all AWS resources created by OpenShift

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes a proposal to allow an administrator of OpenShift to
have the ability to apply user defined tags to many resources created by OpenShift in AWS.

Note: this enhancement is slightly retroactive. Work has already begun on this. See
- https://github.com/openshift/api/pull/864
- https://github.com/openshift/cluster-ingress-operator/pull/578

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
 - tags must be applied at creation time, in an atomic operation. It isn't acceptable to create an object and,
   after some period of time, apply the tags post-creation.

### Non-Goals

 - to reduce initial scope, tags are applied only at creation time and not reconciled. If an administrator manually
   changes the tags stored in the infrastructure resource, behavior is undefined. See below.
 - to reduce initial scope, we are not implementing this for clouds other than AWS. We will not take any actions
   to prohibit that later.

## Proposal

New `experimentalPropagateUserTags` field added to `.platform.aws` of install config to indicate that the user tags should be applied to AWS
resources created by in-cluster operators.

If `experimentalPropagateUserTags` is true, install validation will fail if there is any tag that starts with `kubernetes.io` or `openshift.io`.

Add a new field `userTags` to `.status.aws` of the `infrastructure.config.openshift.io` type. Tags included in the
`userTags` field will be applied to new resources created for the cluster. The `userTags` field will be populated by the installer only if the `experimentalPropagateUserTags` field is true.

Note existing unchanged behavior: The installer will apply these tags to all AWS resources it creates with terraform (e.g. bootstrap and master EC2 instances) from the install config, not from infrastructure status, and regardless if the propagation option is set.

All operators that create AWS resources (ingress, cloud credential, storage, image registry, machine-api)
will apply these tags to all AWS resources they create.

userTags that are specified in the infrastructure resource will merge with userTags specified in an individual component. In the case where a userTag is specified in the infrastructure resource and there is a tag with the same key specified in an individual component, the value from the individual component will have precedence and be used.

The userTags field is intended to be set at install time and is considered immutable. Components that respect this field must only ever add tags that they retrieve from this field to cloud resources, they must never remove tags from the existing underlying cloud resource even if the tags are removed from this field(despite it being immutable).

If the userTags field is changed post-install, there is no guarantee about how an in-cluster operator will respond to the change. Some operators may reconcile the change and change tags on the AWS resource. Some operators may ignore the change. However, if tags are removed from userTags, the tag will not be removed from the AWS resource.

### User Stories

https://issues.redhat.com/browse/SDE-1146 - IAM users and roles can only operate on resources with specific tags
As a security-conscious ROSA customer, I want to restrict the permissions granted to Red Hat in my AWS account by using
AWS resource tags.

### Risks and Mitigations

## Design Details

### Test Plan

TBD

### Graduation Criteria

TBD

### Upgrade / Downgrade Strategy

On upgrade:

The new status field won't be populated since it is only populated by the installer and that can't have happened if the cluster was installed from a prior version. Components that consume the new field should take no action since they will see no additional tags.

On downgrade:

The status field may remain populated, components may or may not continue to tag newly created resources w/ the additional tags depending on whether or not a given component still has logic to respect the status tags, after the downgrade.

### Version Skew Strategy

TBD

## Implementation History

## Drawbacks

If a customer decides they do not want these tags applied to new resources, there is no clean way to address that need:

1. they would have to get help from support to edit the status field
2. they would have to manually remove the undesired tags from any existing resources

In the future we will want to introduce a spec field where a customer can specify and edit these tags, which will be reflected into status when changed, and we will expect consuming components to reconcile changes to the set of tags by applying net new tags to existing resources (but they will still not remove tags that are dropped from the list of tags).

## Alternatives

TBD

## Infrastructure Needed [optional]

TBD
