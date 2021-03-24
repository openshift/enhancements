---
title: apply-user-defined-tags-to-all-aws-resources-created-by-openshift
authors:
  - "@gregsheremeta"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-03-24
last-updated: 2021-03-24
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
   both at install time during continuous operation (Day 2)
 - in a Managed OpenShift environment such as Red Hat OpenShift on AWS (ROSA), allow easy differentiation
   between Red Hat-managed AWS objects and customer-managed objects
 - *allow for the restriction of permissions granted to Red Hat in an AWS account by AWS resource tags.
   For example, see SDE-1146 - "IAM users and roles can only operate on resources with specific tags"*

### Goals

 - the administrator or service (in the case of Managed OpenShift) installing OpenShift can pass an arbitrary
   list of user-defined tags to the OpenShift Installer, and everything created by the installer and all other
   bootstrapped components will apply those tags to all resources created in AWS, for the lift of the cluster.
 - Installer writes the tags to the infrastructure resource so that Day 2 operations can know what tags to apply
   to AWS resources for the life of the cluster.
 - tags must be applied at creation time, in an atomic operation. It isn't acceptable to create an object and,
   after some period of time, apply the tags post-creation.

### Non-Goals

 - to reduce initial scope, tags are applied only at creation time and not reconciled. If an administrator manually
   changes the tags stores in the infrastructure resource, pre-existing AWS resources are not updated. Only newly-
   created AWS resources would get the updated tags.
 - to reduce initial scope, we're not implementing this for clouds other than AWS. We shouldn't take any actions to
   prohibit that later, though.

## Proposal

Add a new field `userTags` to `.spec.aws` of the `infrastructure.config.openshift.io` type. Tags included in the
`userTags` field will be applied to new resources created for the cluster.

Installer will apply these tags to all AWS resources it creates with terraform (e.g. bootstrap and master EC2 instances)

All operators that create AWS resources (ingress, cloud credential, storage, image registry, machine-api)
will apply these tags to all AWS resources they create.

### User Stories

SDE-1146 - IAM users and roles can only operate on resources with specific tags
As a security-conscious ROSA customer, I want to restrict the permissions granted to Red Hat in my AWS account by using
AWS resource tags.

### Implementation Details/Notes/Constraints [optional]

Some components, like machine-api and image registry, already have some support to allow adminstrators to
push tags into EC2 instance and S3 buckets, respectively.

In the case of machine-api, a user has to specify tags on the MachineSet as part of the Day 2 operation.

In the case of image registry, a user has to pre-configure an S3 bucket with the required tags, and use the
image registry's bring-your-own-bucket feature.

### Risks and Mitigations

To reduce initial scope, tags are applied only at creation time and not reconciled. This behavior is atypical in
Kubernetes and tends to cause confusion for users.

## Design Details

### Open Questions [optional]

 - Since machine-api and image registry already have some support to allow adminstrators to
   push tags into EC2 instance and S3 buckets, respectively, should these be out of scope? Or can we agree
   that it's a nicer user experience to specify the tags one time, at install time, and have everything just
   work after that. Our intention is to change the contract of user tags to state that all resources created
   for the cluster are tagged with the user-provided tags, putting the user-provided tags on equal footing with
   the kubernetes owned tag. If we have to carve out exceptions from there that the user needs to remember to
   fill out themselves, then it dilutes that contract.

 - How are upgrades handled? Is this an install-time only feature that requires a fresh cluster to adopt, or
   can we somehow enable upgrade users to adopt it?

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
