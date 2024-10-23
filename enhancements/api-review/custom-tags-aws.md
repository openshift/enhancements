---
title: apply-user-defined-tags-to-all-aws-resources-created-by-openshift
authors:
  - "@gregsheremeta"
  - "@tgeer"
reviewers:
  - @patrickdillon
  - @sdodson
  - @jerpeter1
  - @joelspeed
  - @Miciah
  - @sinnykumari
  - @dmage
  - @staebler
  - @tkashem
  - @tjungblu
approvers:
  - @sdodson
  - @jerpeter1
  - @bparees
creation-date: 2021-03-24
last-updated: 2022-06-16
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

Note: The work is carried out as part of 
- https://issues.redhat.com/browse/RFE-1101
- https://issues.redhat.com/browse/RFE-2012
- https://issues.redhat.com/browse/RFE-3677

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
- Tags can be updated day2 on teh AWS resources.

### Non-Goals

- To reduce initial scope, in case of standalone OpenShift, tags are applied only at creation time and not reconciled. If an administrator manually
changes the tags stored in the infrastructure resource, behavior is undefined. 
- To reduce initial scope, for hosted control plane deployments, we are not implementing this for clouds other than AWS. We will not take any actions
to prohibit that later.

## Proposal

New `propagateUserTags` field added to `.platform.aws` of install config to indicate that the user tags should be applied to AWS
resources created by in-cluster operators.

If `propagateUserTags` is true, install validation will fail if there is any tag that starts with `kubernetes.io` or `openshift.io`.

Add a new field `resourceTags` to `.status.aws` of the `infrastructure.config.openshift.io` type. Tags included in the
`resourceTags` field will be applied to new resources created for the cluster. The `resourceTags` field will be populated by the installer only if the `experimentalPropagateUserTags` field is true.

Note existing unchanged behavior: The installer will apply these tags to all AWS resources it creates with terraform (e.g. bootstrap and master EC2 instances) from the install config, not from infrastructure status, and regardless if the propagation option is set.

All operators that create AWS resources (ingress, cloud credential, storage, image registry, machine-api)
will apply these tags to all AWS resources they create.

userTags that are specified in the infrastructure resource will merge with userTags specified in a kube resource. In the case where a userTag is specified in the infrastructure resource and there is a tag with the same key specified in a kube resource, the value from the kube resource will have precedence and be used.

The userTags field is intended to be set at install time and is considered immutable. Components that respect this field must only ever add tags that they retrieve from this field to cloud resources, they must never remove tags from the existing underlying cloud resource even if the tags are removed from this field(despite it being immutable).

In case of hosted control plane deployments, the userTags field is updated with latest updates requested by user. The merge logic gives higher precedence to userTags when there is duplicate tag found on the AWS resource.

If the userTags field is changed post-install, all AWS resources created and managed by in-cluster and RedHat supported operators will be reconciled. Non-redhat supported operators may reconcile the change and change tags on the AWS resource or may ignore the change. However, if tags are removed from userTags, the tag will not be removed from the AWS resource.

For the resources created and managed by hosted control plane, cluster api provider for aws reconciles the user tags on AWS resources. The hosted control plane updates the `infrastructure.config.openshift.io` resource to reflect new tags in `resourceTags`. The OpenShift operators, both core and non-core (managed by RedHat), reconcile the respective AWS resources created and managed by them. 
Given that, there is no universal controller to update all resources created by OpenShift, the day2 updates of tags is not supported for standalone OpenShift deployments.
### User Stories

1. https://issues.redhat.com/browse/SDE-1146 - IAM users and roles can only operate on resources with specific tags
As a security-conscious ROSA customer, I want to restrict the permissions granted to Red Hat in my AWS account by using
AWS resource tags.

2. https://issues.redhat.com/browse/OCPSTRAT-787 - As a user of ROSA with HCP, I want to add and update tags on all AWS resources created by OpenShift, given that there is security limitation on direct access to update to AWS resources.

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

- The new status field won't be populated since it is only populated by the installer and that can't have happened if the cluster was installed from a prior version. Components that consume the new field should take no action since they will see no additional tags.

On downgrade:

The status/spec field may remain populated, components may or may not continue to tag newly created resources with the additional tags depending on whether or not a given component still has logic to respect the status tags, after the downgrade.

### Version Skew Strategy

## Implementation History

## Drawbacks

If a customer decides they do not want these tags applied to new resources, there is no clean way to address that need:

1. they would have to get help from support to edit the status field
2. they would have to manually remove the undesired tags from any existing resources

## Alternatives

