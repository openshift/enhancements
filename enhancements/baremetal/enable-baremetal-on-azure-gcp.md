---
title: enable-baremetal-on-azure-gcp
authors:
  - "@elfosardo"
reviewers:
  - "@dtantsur"
  - "@zaneb"
approvers:
  - "@dtantsur"
  - "@zaneb"
api-approvers:
  - "@dtantsur"
  - "@zaneb"
creation-date: 2023-04-28
last-updated: 2023-04-28
tracking-link: 
  - "https://issues.redhat.com/browse/METAL-421"
  - "https://issues.redhat.com/browse/METAL-435"
status: implementable
see-also:
  - "/enhancements/baremetal/baremetal-provisioning-config.md"
  - "/enhancements/baremetal/enable-baremetal-on-other-platforms.md"
  - "/enhancements/baremetal/enable-baremetal-on-aws.md"
replaces:
superseded-by:
---

# Enable baremetal on Azure and Google Cloud Platform to support centralized host management

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Baremetal Host API is available when deploying an OpenShift cluster with the baremetal platform (via the IPI or AI (Assisted Installer) workflow), with the on-premise platforms Openstack, vSphere, and None, and from the AWS cloud platform.
Expanding the possibility to deploy and manage bare metal hosts from clusters in other cloud platforms such as Azure and Google Cloud Platform (GCP) would be beneficial to customers.

## Motivation

Having the control plane on a pure cloud platform, allowing the deployment and management of bare metal workers, provides a true hybrid cloud solution.

### Goals

Support the centralized host management use case by enabling Baremetal Host API on Azure and GCP platforms, in order to allow clusters deployment when running on the Azure and GCP platforms.

### Non-Goals

* Automatically configure the connection between the Azure and GCP platforms and the user's bare metal hosts.
* Any work on the Machine API to make it functional.

## Proposal

The BMO (baremetal-operator) provides the Baremetal Host API, which is configured and managed by the CBO (Cluster Baremetal Operator).

Currently CBO checks the platform and if it is not baremetal, Openstack, vSphere, None, or AWS it will be in a "disabled" state i.e. it will
1. set status.conditions Disabled=true and
2. not read or process the Provisioning CR and thus not deploy baremetal-operator.

This proposal is to allow CBO to be enabled on Azure and GCP platforms.

To keep the testing matrix as it is, the allowed configuration options
of the Provisioning CR will be kept restricted to exactly those required
by centralized host management.

*Only spec.provisioningNetwork=Disabled mode will still be accepted in the Provisioning CR.*

If any other provisioningNetwork mode is set, the CBO webhook will refuse the change in the usual way.

Notes:

1. when the Provisioning CR is set to provisioningNetwork=Disabled mode, worker nodes would be booted via virtual media. This removes the requirement for the Provisioning Network which can be expected to be available only in Baremetal platform types.

2. documentation will need to be added to the centralized host management documentation explaining how to create and update a Provisioning CR for the Azure and GCP platforms.

### Workflow Description

#### Variation

### User Stories

#### Story 1 - Current IPI baremetal platform use case

No change.

#### Story 2 - centralized host management use case

As a user of a hub cluster that performs central infrastructure management, and optionally zero-touch provisioning, I need to provision hosts using the k8s-native API (Baremetal Hosts CR) even when the hub cluster has a platform of Azure or GCP.

### Risks and Mitigations

One concern of this implementation is that for the solution to work properly the inbound and outbound connections of the BMCs of the bare metal hosts deployed and managed in this way will be exposed externally, as they need to communicate with the control plane, hosted in the Azure or GCP platforms.
It is essential to use some vpn/tunnel-like connection between the bare metal hosts and the control plane.  
For example, a native solution commonly used in Azure is the [Azure Virtual Network (VNet)](https://learn.microsoft.com/en-us/azure/virtual-network/virtual-networks-overview), while on Google Cloud Platform is the [GCP VPC](https://cloud.google.com/vpc).  
A risk is given by the fact that we don't have a way to verify the correctness of this configuration when directly managed by the user.  
A way to mitigate this is to provide accurate documentation and guidelines.
Since the only deployment method supported by this feature is using virtual media, disabling IPMI on the hosts BMCs is also highly recommended since it's of no use.

### Drawbacks

## Design Details

### API Extensions

### Operational Aspects of API Extensions

#### Failure Modes

* Misconfiguring the Provisoning CR will end up in failure during bare metal hosts deployment. See the [Proposal section](#Proposal) on how to correctly configure the Provisoning CR, especially the provisioningNetwork parameter.
* If the BMC access is misconfigured, the enrollment of the associated bare metal hosts will not happen, or there will be failures during the inspection phase.

#### Support Procedures

### Test Plan

#### Unit Testing

We will add unit tests to confirm that cluster-baremetal-operator:
* is enabled on the Azure and GCP platforms
* will restrict functionality on the Azure and GCP platforms to ProvisioningNetork=Disabled

#### Functional Testing

QE will validate that the Azure and GCP platforms are supported to reduce the load on CI.

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

The feature was silently enabled in Openshift 4.13 and it will go to GA in Openshift 4.14.  
A test has been completed during the OCP 4.14 development phase.  
The documentation, including procedure, configuration, and prerequisites, is worked by the ACM team, that will also provide QE coverage.

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

CBO will upgrade as it currently does, this is only a minor change in functionality.

On the Azure and GCP platforms where the operator was in a disabled state, after been upgraded, it will move into an enabled state. However in all but centralized host management use cases nothing will change as there is no Provisioning CR.

### Version Skew Strategy

None required as this is not dependant on other components.

## Implementation History

These PRs enable the CBO on the Azure and GCP platforms:
* https://github.com/openshift/cluster-baremetal-operator/pull/309
* https://github.com/openshift/cluster-baremetal-operator/pull/316

## Alternatives
