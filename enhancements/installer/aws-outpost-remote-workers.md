---
title: aws-outposts-remote-workers
authors:
  - "@nirarg"
  - "@pkliczewski"
reviewers:
  - "@sdodson"
  - "@JoelSpeed"
  - "@Miciah"
  - "@alebedev87"
  - "@patrickdillon"
  - "@jsafrane"
  - "@damdo"
approvers:
  - "@sdodson"
api-approvers:
  - None
creation-date: 2022-10-11
last-updated: 2022-10-11
tracking-link:
  - https://issues.redhat.com/browse/OCPPLAN-9617
  - https://issues.redhat.com/browse/ECOPROJECT-866
see-also: []
replaces: []
superseded-by: []
---


# Remote workers on AWS outpost

## Summary

Running workload closer to end users or leveraging data locality seems to be
important for many customers. That is why AWS came up with fully [managed
service](https://docs.aws.amazon.com/outposts/latest/userguide/how-outposts-works.html)
that extends AWS infrastructure and run on customer's premises. This enhancement
proposes extending how we deploy OCP on AWS platform so it is possible to run on
AWS Outpost.

The work is divided into several phases:
- Phase 0: Research. Understand what is needed to deploy OCP on AWS Outpost.
  No code changes needed and we only provide [documentation](https://github.com/openshift/openshift-docs/pull/53265) on how to customize
  generated manifests and list known limitations.
- Phase 1: Improve cluster deployment on AWS Ouptost by implementing necessary
  changes and provide mitigations to know limitations.
- Phase 2: Add OCP on AWS Outpost support to ROSA for regular clusters and HyperShift.

## Motivation

Users might want to use OCP on AWS Outpost running in their own data center.
AWS Outposts enables customers to build and run applications on premises using
the same programming interfaces as in AWS Regions, while using local compute
and storage resources for lower latency and local data processing needs.

### User Stories

Phase 0:
- As a cluster admin I want to follow OCP [documentation](https://github.com/openshift/openshift-docs/pull/53265) to deploy OCP on AWS Outpost
  where my master nodes are running in the regular AWS region and workers in AWS Outpost.

Phase 1:
- As a cluster admin I want to use an OCP installer to deploy OCP on AWS Outpost.
  (Cluster deployment topology depends on mitigating lack of support for NLB)

Phase 2:
- As a cluster admin I want to use ROSA cli or UI to deploy OCP/Hypershift on AWS Outpost.
  (OCP deployment topology similar as in Phase 1)


### Goals

Phase 0:
- Admin can follow [documentation](https://github.com/openshift/openshift-docs/pull/53265) to customize installer generated manifests to deploy
  OCP on AWS Outpost
- As post installation step admin can configure ALB on AWS Outpost as workload ingress
- Admin can check list of limitations or not supported features

Phase 1:
- Admin can use OCP installer to deploy OCP on AWS Outpost
- After installation only gp2 storageclass is available and ingress is configured

Phase 2:
- Admin can use ROSA cli or UI to deploy OCP/Hypershift on AWS Outpost

### Non-Goals

We do not intend to implement AWS Outpost specific features in OCP as well us we
do not plan to handle long lasting networking issues between regular AWS region andan outpost.

## Proposal

Phase 0:
We want to provide [documented](https://github.com/openshift/openshift-docs/pull/53265) manual procedure on how to deploy an OCP cluster in a regular
AWS region with worker nodes running in AWS Outpost. The user needs to create VPC and
subnets before the installation and provide them in the install-config to prevent VPC
creation by IPI installer. The subnets need to be created both in the regular region and
in the outpost (tagged as "kubernetes.io/cluster/<non-cluster-name>": "owned"). In the
install-config only AWS region subnet needs to be used so NLB is created for the
apiserver to use. During manifest stage it is needed to change workers' machineSet
to use subnets created in the outpost. As the last modification the user needs to
update MTU for the network provider due to AWS Outpost supporting only 1300 bytes. Once the
cluster is deployed the user needs to install AWS load balancer operator and configure
ALB ingress.

Phase 1:
Separate enhacement needed

Phase 2:
Separate enhacement needed

### Workflow Description

Phase 0:
In order to start the cluster installation process the user needs to follow steps to
configure AWS account and AWS Outpost instance types. Next the user needs to create
VPC with subnets for both the AWS region and the outpost. The user needs to create
cloud formation template with AWS outpost details and use aws client to run it.
Next the user needs to create an install-config in the specified directory and modify it
with AWS details according to [documentation](https://github.com/openshift/openshift-docs/pull/53265). Once it is ready the user needs to
create manifests. MachineSet needs to be updated with AWS outpost subnet information
and MTU. Now we can create the cluster and after the installation we need to install
AWS load balancer operator and configure ALB based ingress.

Phase 1:
Separate enhacement needed

Phase 2:
Separate enhacement needed

#### Variation [optional]

N/A

### API Extensions

N/A - no api extensions are being introduced in this proposal

### Implementation Details/Notes/Constraints [optional]

NLB is not supported by AWS outpost and it is needed for apiserver to work correctly.
That is why in Phase 0 we have decided to run master nodes in the regular AWS region and only
workers in AWS outpost where we can configure ALB for workload ingress.

### Risks and Mitigations

Administrator may want to use other storage types than gp2 (supported by outpost).
In Phase 0 we will make gp2 storageclass as default and later make sure that gp3
storageClass is not created.

### Drawbacks

- AWS Outpost was designed not to handle connectivity issues between regular
  AWS region and an outpost rack.

## Design Details

### Open Questions [optional]

TBD

### Test Plan

We do not plan to modify the test plan created for AWS platform. We may only need to
modify which tests will be run due to AWS Outpost limitation in relation to regular
AWS region.

### Graduation Criteria

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

At the moment we investigated and documented all manifest modifications needed to
deploy OCP on AWS outpost.

## Alternatives

No known alternatives to proposed solution.

## Infrastructure Needed [optional]

AWS donated an Outpost rack available in our data center so we can develop, test and
support the changes.