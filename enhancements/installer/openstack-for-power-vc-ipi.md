---
title: openstack-for-power-vc-ipi
authors:
  - "@mjturek"
reviewers:
  - "@sherine-k"
  - "@prb112"

approvers:
  - "@tbd"
 
creation-date: 2025-06-18
last-updated: 2025-06-18
status: implementable
see-also:
  - enhancements/installer/ibm-cloud-for-power-vs-ipi.md
---

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for tech preview and GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This document describes how the [OpenStack](https://github.com/kubernetes-sigs/cluster-api-provider-openstack) provider can be leveraged to provide infrastructure to deploy
OpenShift on the [Power Virtualization Center][powervc-website] offering. Power Virtualization Center, or PowerVC, is an IBM
offering that provides virtualization management for their on-premise datacenters. Because PowerVC
is based on OpenStack, we intend to heavily use the existing OpenStack provider by using the same paths in the installer that
The goal of this is to extend the OpenShift installer to enable
deployment to a PowerVC environment.

## Motivation

- Current deployments of OpenShift on Power on premise use UPI or the Assisted Intaller.
-- While effective, they require a lot of user intervention to deploy.
-- Scaling is more difficult when deploying via these methods.
- Customers are searching for a simplified method of deploying OpenShift into their existing PowerVC environments.
- IPI provides an opinionated and simplified deployment method.
-- Because it is built on top of OpenStack, we can use the provider nearly as-is.
- IPI provides an easy way to scale after deployment.

### Goals

- Provide a way to install OpenShift on PowerVC infrastructure using the OpenShift installer's OpenStack provider.

### Non-Goals

- Public cloud IPI. This is already implemented by the PowerVS provider. 

## Proposal

To leverage the PowerVC platform, we will make use of the [OpenStack Cluster API Provider][capo-website], or CAPO.
The OpenShift Installer will provision VMs in a specified PowerVC environment. These VMs will serve as bootstrap,
control plane, and compute nodes.

The expected workflow would be as follows:
1. The installer will authenticate to PowerVC in the same manner as the OpenStack provider does today (clouds.yaml)
2. The installer will import a boot image to PowerVC from IBM Cloud Object Storage. This is the same image used by the
PowerVS provider.
3. The installer will follow the same flow as the OpenStack provider.
4. TODO(mjturek): Describe the DHCP service challenges.
5. ROUGH: The cluster will come up without load balancers, and users will add load balancers

### User Stories

- A user of this installer would like to see a cluster running in their PowerVC environment quickly after kicking off
the installer for a default size (3 master and 2 worker) cluster.
- A user would like a scaling solution for their OpenShift cluster.

### API Extensions
None

### Implementation Details/Notes/Constraints

#### Features used by the OpenStack Provider that are Not Supported in PowerVC 
1. Security Groups
2. Allowed Address Pairs

### Risks and Mitigations

TODO

## Design Details

Basic Walkthrough:
1. The user creates a DHCP network in PowerVC.
2. The user prepares the clouds.yaml with access information to their PowerVC environment. 
3. The installer uploads an OVA from IBM Cloud COS and makes it bootable.
4. The installer hands the install over to CAPO.


[Power VS Reference Doc][power-vs-reference-doc]

### Installation Configuration
Here is a sample of an **install-config.yaml** for a PowerVC installation.
```yaml
apiVersion: v1
baseDomain: example.com
compute:
- architecture: ppc64le
  hyperthreading: Enabled
  name: worker
  platform:
    powervc:
      zones:
        - e980
  replicas: 3
controlPlane:
  architecture: ppc64le
  hyperthreading: Enabled
  name: master
  platform:
    powervc:
      zones:
        - e980
  replicas: 3
metadata:
  creationTimestamp: null
  name: powervc-cluster 
networking:
  clusterNetwork:
  - cidr: 10.128.0.0/14
    hostPrefix: 23
  machineNetwork:
  - cidr: 10.20.176.0/20
  networkType: OVNKubernetes
  serviceNetwork:
  - 172.30.0.0/16
platform:
  powervc:
    loadBalancer:
      type: UserManaged
    apiVIPs:
    - 10.20.184.56
    cloud: powervc
    defaultMachinePlatform:
      type: project-name
    ingressVIPs:
    - 10.20.184.56
    machinesSubnet: 8f895eaa-d54f-4c2e-8f00-e1c5eeb51aa1
publish: External
pullSecret: ''
sshKey: '
```

### Networking
TODO: Call out DHCP prerequisites

#### Load Balancing
TODO: Call out LB post deploy tasks
#### DNS
TODO: Investigate how OpenStack handles this

### Red Hat Enterprise Linux CoreOS (RHCOS) OVA
The RHCOS build pipeline publishes an OVA disk image to the IBM Cloud Object Storage (COS) buckets in each
supported region. We will import the image to the desired Power VS instance, transforming it into an installable
boot image when it does so. The cluster-api-provider will later reference this same boot image to add workers.

### Open Questions

#### CI Plan
Currently, Multi-Arch CI for Power connects to dedicated hardware provided by IBM and provisions clusters using IPI for
libvirt, which isn't a supported customer configuration.

We will create prow jobs that uses dedicated hardware that hosts a PowerVC cluster.

### Test Plan

Multi-Arch currently tests results by running upgrade and e2e tests with the OpenShift Prow CI environment. Our test
plan would extend this to include these IPI scenarios. Additionally, new documentation will be needed for PowerVC,
and the test plan will incorporate review and validation of this.

### Graduation Criteria

- Our initial target is to work towards a `Tech Preview`, then graduate towards future milestones. This is expected to
take a total of two releases but could be more or less depending on how quickly we meet the requirements enumerated in
the graduation criteria.
- `Tech Preview` will graduate to `GA` in OpenShift
- Our current target for `Tech Preview` is OpenShift 4.21, but this will be reevaluated as work progresses. Commitments
are tracked in the Multi-Arch program sign-off documents for each release, which is created as part of Quarterly
Planning.
- [Deprecation policy][deprecation-policy]

[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

#### Dev Preview -> Tech Preview
In 4.20 we intend to have an implementation behind a feature gate.

#### Tech Preview Requirements

- [ ] Full functionality / ability to utilize the enhancement end to end
- [ ] End-user documentation
- [ ] CI e2e jobs created to cover installation against this platform
- [ ] Plan defined & support engaged to gather feedback from users

#### Tech Preview -> GA
- [ ] CI has been upgraded to cover testing of modifications to the installer (PR validation)
- [ ] CI "soak" for to ensure a stable baseline
- [ ] Final optimizations for install time
- [ ] Expanded testing (upgrade, downgrade, scale, load/performance)
- [ ] Documentation is finalized

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

All upgrade and downgrade expectations are the same for existing IPI deployments.

### Version Skew Strategy

This should be the same as other IPI deployments.

### Operational Aspects of API Extensions
N/a

#### Failure Modes
N/a

#### Support Procedures
N/a

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

## Alternatives (Not Implemented)

We can continue to rely on our UPI playbooks but this is not ideal as they are not supported. We can also point users to
our assisted installer support for PowerVC, but we believe users desire a more automated solution in this environment.

## Infrastructure Needed

