---
title: nutanix-ahv-ipi
authors:
  - "@vnephologist"
reviewers:
  - "@makentenza"
  - "@fabianofranz"
  - "@elmiko"
approvers:
  - "@makentenza"
  - "@fabianofranz"
creation-date: 2021-09-15
last-updated: 2021-09-24
status: provisional
---

# OpenShift Installer Provisioned Infrastructure (IPI) on Nutanix AOS (AHV)

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes adding support for OpenShift 4 Installer Provisioned Infrastructure (IPI) on Nutanix AOS infrastructure with Nutanix AHV hypervisor. It describes the necessary enhancements, tooling and documentation to enable this capability.

## Motivation

Red Hat and Nutanix have recently [announced](https://www.redhat.com/en/about/press-releases/red-hat-and-nutanix-announce-strategic-partnership-deliver-open-hybrid-multicloud-solutions) a strategic partnership to deliver open hybrid multicloud solutions. For OpenShift this means that it ​​becomes the preferred enterprise full-stack Kubernetes solution on Nutanix Cloud Platform with AHV.

Today, OpenShift can be installed on the Nutanix AOS platform using the OpenShift platform [agnostic installer](https://github.com/nutanix/openshift/tree/main/docs/install/manual), but mutual customers have also requested a fully automated, integrated installer built using OpenShift 4's infrastructure level automation capabilities.
This is particularly appealing for customers deploying in "hybrid cloud" scenarios where OpenShift should be deployed using common mechanisms across different clouds.

### Goals

The primary goal of Nutanix AHV IPI is to provide users with an easier path to running OpenShift 4 on Nutanix AOS with AHV.

To achieve that goal, we will:

- Enhance the installer, adding Nutanix AOS options.

- Prepare Nutanix infrastructure using the [Nutanix Terraform (TF) provider](https://github.com/nutanix/terraform-provider-nutanix) for provisioning.

- Integrate with the Nutanix machine API provider [to-do] and enhance requisite operators to enable cluster functionality.

- Provide Nutanix AHV IPI user documentation [here](https://github.com/openshift/installer/tree/master/docs/user/nutanix/).

- Provide CI artifacts required to test the Nutanix AHV IPI installer.

### Non-Goals

None at this time.

## Proposal

- Implement installer CLI prompts to build a default/minimal `install-config.yaml` for Nutanix AHV:

  ```shell
  ? SSH Public Key /Users/someone/.ssh/id_rsa.pub
  ? Platform nutanix
  ? Prism Central pc01.ocp.nutanix.com
  ? Prism Central Username admin@ocp.nutanix.com
  ? Prism Central Password [? for help] **********
  ? Cluster (Prism Element) nx01.ocp.nutanix.com
  ? Container default-container-70593
  ? Subnet vm-network
  ? Virtual IP Address for API 192.168.1.5
  ? Virtual IP Address for Ingress 192.168.1.6
  ? Base Domain ocp.nutanix.com 
  ? Cluster Name ocp01
  ? Pull Secret [? for help] ****
  ```

- Implement Nutanix specific platform installer customizations:
  
  **Platform**
  
  - prismCentral (required string): Name or IP address of the Nutanix Prism Central instance to target.
  - prismCentralUser (required string): Username for Prism Central access (Cluster Admin).
  - prismCentralPass (required string): Password for Prism central access.
  - cluster (required string): Nutanix cluster (Prism Element) where the OpenShift cluster will be created.
  - container (required string): Nutanix container name for OS disk install.
  - subnet (required string): Nutanix subnet name (IPAM or DHCP Enabled).
  
- Provide documentation (in similar format to other supported IPI platforms) to help users use and customize the IPI installer on Nutanix:
  - Prerequisite instructions prior to invoking installer
  - Description of installer options and customizations as they apply to Nutanix
  - Post installation instructions on further customizations a user may wish to apply

### User Stories

Story 1
As an OpenShift consumer, I want to quickly, with minimal input and default options, be able to use the OpenShift installer to create and destroy an OpenShift 4 cluster on Nutanix AOS with AHV.

Story 2
As an OpenShift consumer, I want to be able to use the OpenShift installer with customizations (i.e. enhanced security) to create and destroy an OpenShift 4 cluster on Nutanix AOS with AHV.

Story 3
As an OpenShift platform, I want OpenShift on Nutanix to be maintained and released like other supported OpenShift platforms and covered by CI tests.

### Implementation Details/Notes/Constraints

- Nutanix AHV supports VM guest customization via [cloud-init Config Drive v2](https://cloudinit.readthedocs.io/en/latest/topics/datasources/configdrive.html).

- It will be assumed the customer utilizes a subnet configured with Nutanix IPAM or will provide a properly configured DHCP server that is available on the configured L2 network.

- Nutanix does not provide networking services like DNS or load balancing. With this in mind, we require the ability to automate DNS and load balancing services internal to the cluster. A solution similar to the baremetal-networking enhancement can be utilized.

### Risks and Mitigations

- The Nutanix Cluster API provider is a work-in-progress.  It is currently being tracked internally at Nutanix and expected to be open sourced.

## Design Details

- Cloud Credential Operator (CCO) will operate in manual mode. Possible migration to passthrough mode in future enhancement.

- Cloud Controller Manager (CCM) is not currently implemented. Node lifecycle handled via MAPI until standard AOS/AHV CCM is developed.

- CSI Operator already publised [here](https://catalog.redhat.com/software/containers/nutanix/nutanix-csi-operator/60dfad1364002f2e99660a86). Installer to deploy CSI and provision PV for use by Internal Registry.

- Internal Registry to utilize Nutanix Volumes (block) storage by default. Post-install configuration on Nutanix Files or Objects to be documented.

### Open Questions

1. How should VM specifications be determined or specified?

1. Are there any minimum host requirements to enforce?

1. Should the default install attempt to locate control plane VMs on different physical nodes?

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
- Test engineers have successfully followed the documented IPI instructions to deploy OpenShift 4 on Nutanix.

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

Nutanix infrastructure will be needed to support CI E2E testing.
