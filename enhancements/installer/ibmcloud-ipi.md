---
title: openshift-ipi-on-ibmcloud
authors:
  - "@jeffnowicki"
  - "@BobbyRadford"
reviewers:
  - @staebler
approvers:
  - @staebler
creation-date: 2021-05-03
last-updated: yyyy-mm-dd
status: implementable
---

# OpenShift Installer Provisioned Infrastructure (IPI) on IBM Cloud

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes adding support for OpenShift 4 Installer Provisioned Infrastructure (IPI) on IBM Cloud VPC (Gen 2) infrastructure. It describes the necessary enhancements, tooling and documentation to enable this capability.

## Motivation

Users expect OpenShift to be available through multiple cloud providers. Customers with high-availability and enhanced security requirements may wish to take advantage of [IBM Cloud VPC (Gen 2) infrastructure](https://www.ibm.com/cloud/vpc). Leveraging IPI, the process of installing OpenShift on IBM Cloud can be simplified.


### Goals

The primary goal of IBM Cloud IPI is to provide users with an easier path to running OpenShift 4 on VPC infrastructure in IBM Cloud data centers.

To achieve that goal, we will follow the pattern of other IPI supported platforms:
- Enhance the installer to survey customer for IBM Cloud options.
- Prepare IBM Cloud Terraform (TF) module for control plane provisioning. The [IBM Cloud Terraform Provider](https://github.com/IBM-Cloud/terraform-provider-ibm) will utilized.
- Integrate with IBM Cloud VPC machine API provider. The [IBM Cloud Cluster API Provider](https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud) will be utilized.
- Integrate with IBM Cloud Controller Manager (CCM) and enhance requisite operators to enable cluster functionality on IBM Cloud.
- Provide IBM Cloud IPI user documentation [here](https://github.com/openshift/installer/tree/master/docs/user/ibmcloud/).
- Provide CI artifacts required to test the IBM Cloud IPI installer.


### Non-Goals

None at this time.

## Proposal

- Implement installer CLI prompts in order to build a default/minimal `install-config.yaml` for IBM Cloud :
  ```shell
  ? SSH Public Key /Users/someone/.ssh/id_rsa.pub
  ? Platform ibmcloud
  ? Resource Group ID default (34ffb674f7c4466398dcd257a0dac58e)
  ? Region us-south
  ? RHCOS Custom Image rhcos-ibmcloud-470
  ? Base Domain ibm.foo.com (Internet Services-9h)
  ? Cluster Name test
  ? Pull Secret [? for help] ****
  ```

- Implement IBM Cloud specific platform and machine pool installer customizations
  
  **Platform**
  
  - region (required string): The IBM Cloud region where the cluster will be created.
  - cisInstanceCRN (required string): The Cloud Internet Services CRN managing the base domain DNS zone.
  - clusterOSImage (required string): The name of the RHCOS custom image to use for machines.
  - resourceGroup (optional string): The name of an existing resource group where the cluster and all required resources will be created.
  - defaultMachinePlatform (optional object): Default machine pool properties that apply to machine pools that do not define their own IBM Cloud specific properties.
  
  If one of these is specified, they ALL must be specified.
  - vpc (optional string): The name of an existing VPC network.
  - vpcResourceGroup (optional string): The name of the existing VPC's resource group.
  - subnets (optional array of strings): A list of existing subnet IDs. Leave unset and the installer will create new subnets in the VPC network on your behalf.
  
  **Machine Pool**
  - type (optional string): The VSI machine profile.
  - zones (optional array of strings): The availability zones used for machines in the pool.
  - bootVolume (optional object):
    - encryptionKeyCRN: (optional string): The CRN referencing a a Key Protect or Hyper Protect Crypto Services key to use for volume encryption. If not specified, a provider managed encryption key will be used.

- Provide documentation (in similar format to other supported IPI platforms) to help users use and customize the IPI installer on IBM Cloud:
  - Prerequisite instructions prior to invoking installer
  - Description of installer options and customizations as they apply to IBM Cloud
  - Post installation instructions on further customizations a user may wish to apply

### User Stories

Story 1
As an OpenShift consumer, I want to quickly, with minimal input and default options, be able to use the OpenShift installer to create and destroy an OpenShift 4 cluster on IBM Cloud.

Story 2
As an OpenShift consumer, I want to be able to use the OpenShift installer with customizations (i.e. enhanced security) to create and destroy an OpenShift 4 cluster on IBM Cloud.

Story 3
As an OpenShift platform, I want OpenShift on IBM Cloud to be maintained and released like other supported OpenShift platforms and covered by CI tests.


### Implementation Details/Notes/Constraints

- The default IPI-provisioned cluster on IBM Cloud will be a [single region, multizone (3 zones) cluster](https://cloud.ibm.com/docs/containers?topic=containers-ha_clusters#multizone).

- An IBM Cloud RHCOS image must be made available and maintained - ([current 4.7 image location](https://mirror.openshift.com/pub/openshift-v4/x86_64/dependencies/rhcos/4.7/latest/)). The images will be imported as custom images for IBM Cloud.

- A small custom-bootstrap.ign file is used to reference canonical bootstrap.ign file hosted in object storage to workaround 64 KB user data size limit.

- Customer will need to provide and prepare a [Cloud Internet Services (CIS)](https://www.ibm.com/cloud/cloud-internet-services/details) instance, which will provide required DNS capabilities.


### Risks and Mitigations

- Current RHCOS image minimum storage requirement of 120gb. Currently, IBM Cloud VSIs are provisioned with 100gb boot volumes. Workarounds have been explored and discussions have started for getting support for larger boot volume size options to accommodate image storage requirement.

- The IBM Cloud Provider (Kubernetes Cloud Provider) is a work-in-progress.  It is being tracked by a [Kubernetes community enhancement](https://github.com/kubernetes/enhancements/issues/671).

- The [IBM Cloud Cluster API Provider](https://github.com/kubernetes-sigs/cluster-api-provider-ibmcloud) project is a relatively new project.  The project will need to be reasonably hardened with a level of CI enabled.

## Design Details


### Open Questions

1. Is IBM Cloud VPC CCM required for IPI deliverable? Load balancer support being the obvious functionality provided by CCM (optionally enabled). Is there any other CCM functionality that is considered MVP (minimum viable product)?

1. The Cloud Credential Operator supports multiple modes of operation. For the initial IPI deliverable, what is the MVP level of support that needs to be implemented for IBM Cloud?  
> We'll plan to implement "Passthru Mode" initially and work towards a strategic implementation around "Compute Resource Identify". Ref: https://github.com/openshift/cloud-credential-operator#2-passthrough-mode

1. Is storage operator support a hard requirement for IPI? Could it be provided in a future release?
> Without it (making it a Day 2 post install operation), diminishes the UX for the customer.

1. Are unit-level tests recommended (required?). If so, is there preferred tooling / test structure? This is in addition to the required CI E2E testing.
> We will follow what other supported platforms have done. Please advise with any suggestions/tips.

### Test Plan

Will follow test plan design implemented by existing supported IPI platforms:
- A new CI test suite with E2E test jobs will be established.

### Graduation Criteria

The proposal is to follow a graduation process based on the existence of a continuous integration (CI)
suite running end-to-end (E2E) jobs. The CI suite results will be evaluated and acted on as needed.

The following list describes the key elements of the criteria:

- CI jobs are enabled and regularly scheduled.
- Current IPI documention published in the OpenShift repo.
- E2E jobs are stable and passing.  Results are evaluated with the same criteria as comparable supported IPI platform providers.
- Test engineers have successfully followed the documented IPI instructions to deploy OpenShift 4 on IBM Cloud.

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

None at this time.

## Alternatives

Currently, there is no alternative. We intend to formalize and present a UPI proposal in the near future (which would be considered an alternative to IPI). Proof-of-concept work was performed following the [UPI plaform-agnostic instructions](https://docs.openshift.com/container-platform/4.7/installing/installing_platform_agnostic/installing-platform-agnostic.html).

## Infrastructure Needed

IBM Cloud VPC infrastructure will be made available to support CI E2E testing.
