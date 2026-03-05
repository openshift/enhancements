---
title: aws-eusc-partition
authors:
  - @patrickdillon
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @tthvo
approvers: # This should be a single approver. The role of the approver is to raise important questions, ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval. Team leads and staff engineers often make good approvers.
  - @sadasu
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None". Once your EP is published, ask in #forum-api-review to be assigned an API approver.
  - @everettraven
creation-date: 2026-03-03
last-updated: 2026-03-03
status: implementable
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CORS-4239
see-also:
replaces:
superseded-by:
---

# AWS European Sovereign Cloud (EUSC) Partition

## Summary

AWS has introduced a new partition (a separate collection of regions) called
European Sovereign Cloud. The data centers comprising this partition are isolated
entirely within the European geography. OpenShift should support this partition
with feature parity to the standard `aws` partition, with any identified differences
documented.

Ideally partitions and regions would be a transparent consideration, utilizing the AWS
API to determine validity. Certain limitations, specficially RHCOS AMI publication,
openshift/API regex-based validations, and pending adoption of v2 of the aws sdk by
many components need to be addressed to make progress toward that goal.

## Motivation

We want to improve the user experience of installing to EUSC in order to increase
adoption of OpenShift, specifically for users and businesses looking for
geographical isolation of data.

### User Stories

* As an administrator, I want to be able to install a fully functgional OpenShift cluster
to a region in the EUSC.

### Goals

Add support for EUSC.

### Non-Goals

In the future, we should make the addition of any future partitions
and regions transparent, but we're not there yet.

## Proposal

* Publish RHCOS AMI in the only region in EUSC, until then users
can BYO AMI, which is existing functionality in the installer
* Add `aws-eusc` to the hard-coded lists of AWS partitions in components such as
the installer and openshift/api validation regexes.
* Installer will automatically populate service endpoints when users
install to the EUSC region for any operators that have not upgraded
to aws-sdk-go-v2 (and are currently using V1, which is past EOL)  

### Workflow Description

Users should be able to install to the EUSC partition just like any region
in the standard `aws` commercial partition, by supplying valid credentials
(as specified by the aws sdk) and a EUSC region in the install config.

Currently, users need to supply their own AMI and service endpoints, so
for illustration the install config looks like:

```yaml
platform:
  aws:
    region: eusc-de-east-1
    defaultMachinePlatform:
      # Build and use a custom AMI as public RHCOS AMI is not available in this region
      amiID: ami-1234567890
    serviceEndpoints:
    - name: ec2
      url: https://ec2.eusc-de-east-1.amazonaws.eu
    - name: elasticloadbalancing
      url: https://elasticloadbalancing.eusc-de-east-1.amazonaws.eu
    - name: s3
      url: https://s3.eusc-de-east-1.amazonaws.eu
    - name: route53
      url: https://route53.amazonaws.eu
    - name: iam
      url: https://iam.eusc-de-east-1.amazonaws.eu
    - name: sts
      url: https://sts.eusc-de-east-1.amazonaws.eu
    - name: tagging
      url: https://tagging.eusc-de-east-1.amazonaws.eu
```

Once an AMI is published in the region, and the installer is updated to
automatically populate these endpoints when `eusc-de-east-1` is set
(not to mention operators update to aws-sdk-go-v2), the install config
will be simply:

```yaml
platform:
  aws:
    region: eusc-de-east-1
  ```

### API Extensions

Regex validation in openshift/api will be updated to include `aws-eusc`.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hypershift will gain the same benefit from updating the regexes. The installer
cannot populate the service endpoints for hypershift, so hypershift will need
to depend on users supplying the endpoints until cluster operators are updated.

#### Standalone Clusters

Already covered above.

#### Single-node Deployments or MicroShift

n/a

#### OpenShift Kubernetes Engine

n/a

### Implementation Details/Notes/Constraints


### Risks and Mitigations

None

### Drawbacks

None

## Alternatives (Not Implemented)

N/A

## Open Questions [optional]

None

## Test Plan

New cluster profile is created for EUSC partition. Presubmits and periodics will
be created for the initial region. Standard QE testing patterns will be applied
for additional regions.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end

### Tech Preview -> GA

- Document & validate any feature disparity between standard commercial partition & EUSC 

### Removing a deprecated feature

n/a

## Upgrade / Downgrade Strategy

n/a


## Version Skew Strategy

n/a

## Operational Aspects of API Extensions

n/a

## Support Procedures

SOP for any region

## Infrastructure Needed [optional]

PGE Cloud Ops has supplied relevant AWS accounts.