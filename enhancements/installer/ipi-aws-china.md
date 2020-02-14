---
title: ipi-install-aws-china
authors:
  - "@wanghaoran1988"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-02-14
last-updated: 2020-02-14
status: implementable
---

# IPI install on AWS China

## Release Sign off Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

We have customers want to install OpenShift on AWS China, but currently
OpenShift installer doesn't show up the AWS China regions `cn-north-1` and `cn-northwest-1`.
These two regions are isolated from other global regions and RHCOS AMI pushed to global cannot
be used in AWS China, [ARNs and api endpoint](https://docs.amazonaws.cn/en_us/aws/latest/userguide/endpoints-arns.html) are different.

Similar to other global regions, we should support these two regions for customer who want deploy
OpenShift on AWS China.

## Motivation

### Goals

* OpenShift installer support IPI install on AWS China Regions.
* CI job executing testings on AWS China regions.

### Non-Goals

* It's not a goal to detail how to request and setup a AWS account in AWS China.
* It's not a goal to detail how to do UPI install.
* It's not a goal to support all AWS service endpoints that are not part of the SDK for AWS China Regions.

## Proposal

In order to support install OpenShift on AWS China, we need:

* Setup a public AWS China Account to host RHCOS AMIs.
* Push RHCOS AMIs to AWS China account, and share them to public.
* OpenShift installer support AWS China Regions.
* All OCP components using AWS apis should use AWS China api endpoints.

### Implementation Details/Notes/Constraints

#### Setup AWS China Account

The AMIs in global regions are not useable in AWS China regions, we need setup an AWS China Account to host our RHCOS AMIs, so that installer can use them to setup the cluster.

#### Push RHCOS AMIs to AWS China account, and share them to public.

Currently, we have CI jobs push the AMIs to public regions, after the AWS China account setup is ready, we should make our CI job start push our AMIs to AWS China regions, and share them to all accounts in AWS China regions.

#### OpenShift installer support AWS China regions

The OpenShift installer should be able to use the AMIs that pushed to AWS China regions to provision clusters, and use the correct api endpoints and ARNs, Notable difference for AWS China:

* AWS resources ARNs in China regions are prefixed with "arn:arn-cn"
* Ec2 service endpoint is "ec2.amazonaws.com.cn", it's already in SDK, we should configure correctly to use this.
* Route53 currently is not GA, and AWS SDK is missing the api endpoint, we should hard code the api endpoint "route53.amazonaws.com.cn" or "api.route53.cn" in AWS China, the two both works at the moment, but first one is preferred.

#### Cloud credential operator support AWS China regions

Cloud credential operator will create AWS client and use IAM service to validate the permission for provided AWS credential, to support AWS China, we should setup the SDK correctly so it can use IAM api endpoint "iam.amazonaws.com.cn" for AWS China regions.

#### Ingress operator support AWS China regions

Ingress operator use route53 service to update related DNS records, Route53 currently is not GA in AWS China, and AWS SDK is missing the api endpoint, we should hard code the api endpoint "route53.amazonaws.com.cn" or "api.route53.cn" in AWS China, the two both works at the moment, but first one is preferred.

When we using `resourcetaggingapi` to query the DNS zones, we need setup the region correctly or it will return no results.

### Risks and Mitigations

TODO

## Design Details

### Test Plan

We should have our AWS Account setup and shared out for dev teams, so we can start testing, and our testing CI should include one AWS China Region, and run the installer and e2e tests in AWS China account.

### Graduation Criteria

This enhancement will follow standard graduation criteria.

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

Not applicable

### Version Skew Strategy

Not applicable

## Implementation History

## Drawbacks

None

## Alternatives

None
