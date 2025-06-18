---
title: openstack-for-power-vc-ipi
authors:
  - "@mjturek"
reviewers:
  - "@sherine-k"
  - "@prb112"
  - "@clnperez"
  - "@Prashanth684"

approvers:
  - "@patrickdillon"
 
api-approvers:
 - "@tbd"

creation-date: 2025-06-18
last-updated: 2025-09-23
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/MULTIARCH-5358
see-also:
  - enhancements/installer/ibm-cloud-for-power-vs-ipi.md
---

# openstack-for-power-vc-ipi

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for tech preview and GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This document describes how the [OpenStack](https://github.com/kubernetes-sigs/cluster-api-provider-openstack) provider can be leveraged to provide infrastructure to deploy
OpenShift on the [Power Virtualization Center](https://www.ibm.com/docs/en/powervc/2.1.1?topic=power-virtualization-center-apis) offering. Power Virtualization Center, or PowerVC, is an IBM
offering that provides virtualization management for their on-premise datacenters. Because PowerVC
is based on OpenStack, the new PowerVC platform heavily uses the same paths in the installer that
the OpenStack platform uses. The goal of this enhancement is to add a new powervc platform to the OpenShift
installer, that essentially serves as a shim provider of the existing openstack platform. This new
platform will acount for the differences between powervc and openstack to enable deployment of OpenShift to a PowerVC environment.

## Motivation

- Current deployments of OpenShift on Power on premise are user-provisioned, agent based, or use the assisted installer.
    - While effective, they require a lot of user intervention to deploy.
    - Automation around these features is not supported.
    - Scaling is more difficult when deploying via these methods.
- Customers are searching for a simplified method of deploying OpenShift into their existing PowerVC environments.
- IPI provides an opinionated and simplified deployment method.
    - Because PowerVC is built on top of OpenStack, we can use the OpenStack IPI platform nearly as-is.
- Clusters deployed with IPI are equipped with an easy way to scale after deployment.
    - Simplified path to incorporating the Machine API.
    - Manually scaling is as easy as updating a manifest
    - Event driven automation provides a powerful way to scale as needed.
- OpenStack IPI platform cannot be used as is, because it requires several OpenStack features that are not available in PowerVC today
    - Neutron allowed-address pairs
    - Neutron native DHCP networks
    - Security groups
    - Glance image save
- Create a platform that leverages the existsing OpenStack IPI platform, but accounts for the differences that PowerVC has.

### Goals

- Enable cluster-admins to install an OpenShift cluster on PowerVC infrastructure.
- Create a "shim" platform around the existing OpenStack platform.
    - Modularize the infrastructure differences so they are isolated from the openstack provider

### Non-Goals

- Replace or use the IBM Cloud PowerVS provider. 
- Implement an entirely new provider.
    - Changes will be made to the installer, but we will call out to the same components as the OpenStack platform does.

## Proposal

To leverage the PowerVC platform, we will make use of the [OpenStack Cluster API Provider](https://github.com/openshift/cluster-api-provider-openstack), or CAPO.
The OpenShift Installer provisions VMs in a specified PowerVC environment. These VMs will serve as bootstrap,
control plane, and compute nodes.

### Workflow Description
The workflow is:
1. The installer authenticates to PowerVC in the same manner as the OpenStack provider does today (clouds.yaml)
2. The installer imports a boot image to PowerVC from IBM Cloud Object Storage. This is the same image used by the PowerVS provider.
3. The installer will not specify any security groups to CAPO.
4. The installer follows the same flow as the OpenStack provider when it uses UserManaged loadbalancers.
5. Because PowerVC does not have native DHCP support, PowerVC expects that a DHCP service is set up by the user to serve the PowerVC on-prem network.

### API Extensions
None

### User Stories
- A user of this installer would like to see a cluster running in their PowerVC environment quickly after kicking off
the installer for a default size (3 control plane and 3 compute) cluster.
- A user would like a scaling solution for their OpenShift cluster.
- A user scales up the compute nodes in the OpenShift cluster, creating new VMs in PowerVC.
- A user scales down the comptue nodes in the OpenShift cluster, creating new VMs in PowerVC.-

### Topology Considerations
None

#### Hypershift / Hosted Control Planes
None

#### Standalone Clusters
None

#### Single-node Deployments or MicroShift
None

### Implementation Details/Notes/Constraints

While we can reuse most of what the openstack platform does as-is,
there are core differences that we'll need to address in the installer.
PowerVC handles images in a very specific way, so we'll need to perform
some extra steps to make the images bootable. Additionally, we'll need
to use swift to host ignition as glance is configured in a way that
we cannot guarantee the file can be downloaded.

### Risks and Mitigations

While the additional functionality will not pose a risk to other
teams, it will expand what is tested and supported by the IBM team. We are
confident that it will fit in our existing team so the risk is minimal.

### Drawbacks

There will be some prerequisites expected from the user, however this
is not unheard of in the installer.
 
### Implementation Details/Notes/Constraints
We will need to work around some features that the existing OpenStack
platform uses that we do not support. For example, we'll need to avoid
using security groups. We've proven this can be done.

#### Features used by the OpenStack Provider that are Not Supported in PowerVC 
1. Security Groups
2. Allowed Address Pairs (Initial load balancing)
3. Native DHCP networks

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
sshKey: ''
```

### Networking
Because PowerVC does not have native DHCP network support, we are requiring admins set up their own DHCP network service before installing.

#### Load Balancing
The OpenStack provider uses the "allowed-address-pairs" feature of Neutron for initial loadbalancing. Because PowerVC lacks this feature, we expect admins to
use the UserManaged loadbalancers option that the OpenStack provider also supports.


#### DNS
This will be handled in the same fashion as the OpenStack provider.

### Red Hat Enterprise Linux CoreOS (RHCOS) OVA
The RHCOS build pipeline publishes an OVA disk image to the IBM Cloud Object Storage (COS) buckets in each
region. We will import from the same sources used by PowerVC, transforming it into an installable, bootable
image. The cluster-api-provider will later reference this same boot image to add workers.

### Open Questions


## Test Plan

Multi-Arch currently tests results by running upgrade and e2e tests with the OpenShift Prow CI environment. Our test
plan would extend this to include these IPI scenarios. Additionally, new documentation will be needed for PowerVC,
and the test plan will incorporate review and validation of this.

### CI Plan
Currently, Multi-Arch CI for Power connects to dedicated hardware provided by IBM and provisions clusters using IPI for
libvirt, which isn't a supported customer configuration.

We will create prow jobs that uses dedicated hardware that hosts a PowerVC cluster. Initially we expect to only need
two concurrent clusters but as the number of releases with PowerVC support increases, this will expand to nine clusters
at a time.

## Graduation Criteria

- Our initial target is to work towards a `Tech Preview`, then graduate towards future milestones. This is expected to
take a total of two releases but could be more or less depending on how quickly we meet the requirements enumerated in
the graduation criteria.
- `Tech Preview` will graduate to `GA` in OpenShift 4.22
- Our current target for `Tech Preview` is OpenShift 4.21, but this will be reevaluated as work progresses. Commitments
are tracked in the Multi-Arch program sign-off documents for each release, which is created as part of Quarterly
Planning.
- [Deprecation policy][deprecation-policy]

[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

### Dev Preview -> Tech Preview

- [ ] Full functionality / ability to utilize the enhancement end to end
- [ ] End-user documentation
- [ ] CI e2e jobs created to cover installation against this platform
- [ ] Plan defined & support engaged to gather feedback from users

### Tech Preview -> GA
- [ ] CI has been upgraded to cover testing of modifications to the installer (PR validation)
- [ ] CI "soak" for to ensure a stable baseline
- [ ] Final optimizations for install time
- [ ] Expanded testing (upgrade, downgrade, scale, load/performance)
- [ ] Documentation is finalized

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

All upgrade and downgrade expectations are the same for existing IPI deployments.

## Version Skew Strategy

This should be the same as other IPI deployments.

## Operational Aspects of API Extensions
N/a

#### Failure Modes
N/a

## Support Procedures
N/a

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

## Alternatives (Not Implemented)

We can continue to rely on our UPI playbooks but this is not ideal as they are not supported. We can also point users to
our assisted installer support for PowerVC, but we believe users desire a more automated solution in this environment.

Adding code paths specific to PowerVC to the OpenStack provider was explored. For example, we tried adding the ability
to skip security group creation when targetting PowerVC. The OpenStack team was not open to accepting this as they didn't want
to imply that all OpenStack clusters should consider not using security groups. In the end, using the existing Openstack
platform and having additional options like disabling security group creation was not favored because we do not want to expose this as
an option that is available for users outside of PowerVC. We decided that the best way to isolate these changes would
be to create a shim provider that handled the differences in environment while still leveraging the existing OpenStack IPI platform.

Using the external platform was another option that we explored. The intent of the external platform is to allow for the usage
of custom providers without fully being a part of the installer. PowerVC is unique in that it is a variant of
OpenStack. Because it uses CAPO directly, it would not fit the intended usage of the external platform.

## Infrastructure Needed

