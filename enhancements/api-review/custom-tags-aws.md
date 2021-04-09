---
title: apply-user-defined-tags-to-all-aws-resources-created-by-openshift
authors:
  - "@gregsheremeta"
reviewers:
  - @decarr @bparees
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
have the ability to apply user defined tags to all resources created by OpenShift in AWS.

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
   bootstrapped components will apply those tags to all resources created in AWS, for the life of the cluster.
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

Installer will apply these tags to all AWS resources it creates with terraform (e.g. bootstrap and master EC2 instances) (already exists)

All operators that create AWS resources (ingress, cloud credential, storage, image registry, machine-api)
will apply these tags to all AWS resources they create.

userTags that are specified in the infrastructure resource will merge with userTags specified in an individual component. In the case where a userTag is specified in the infrastructure resource and there is a tag with the same key specified in an individual component, the value from the individual component will have precedence and be used.

### User Stories

https://issues.redhat.com/browse/SDE-1146 - IAM users and roles can only operate on resources with specific tags
As a security-conscious ROSA customer, I want to restrict the permissions granted to Red Hat in my AWS account by using
AWS resource tags.

### Risks and Mitigations

The userTags field is intended to be set at install time and immutable. If the userTags field is changed post-install, there is no guarantee about how an in-cluster operator will respond to the change. Some operators may reconcile the change and change tags on the AWS resource. Some operators may ignore the change. If tags are removed from userTags, the tag may or may not be removed from the AWS resource.


## Design Details

### Test Plan

TBD

### Graduation Criteria

TBD

### Upgrade / Downgrade Strategy

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Good question ^. TBD

### Version Skew Strategy

TBD

## Implementation History

## Drawbacks

TBD

## Alternatives

TBD

## Infrastructure Needed [optional]

TBD
