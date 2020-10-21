---
title: packet-ipi
authors:
  - "@displague"
  - TBD
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-08-13
last-updated: 2020-08-13
status: provisional
---

# Packet IPI

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria
- [ ] User-facing documentation is created in
  [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

[Support for OpenShift 4](https://github.com/RedHatSI/openshift-packet-deploy)
on Packet was initially provided through the user provisioned (UPI) workflow.

This enhancement proposes adding tooling and documentation to help users deploy
OpenShift 4 on Packet using the installer provisioned infrastructure (IPI)
workflow.

## Motivation

Users expect OpenShift to be available through multiple cloud providers. Those
who want to deploy OpenShift on single-tenant cloud infrastructure in facilities
where high levels of interconnection are possible, may wish to take advantage of
Packet's bare metal and virtual networking infrastructure.

We can help these users simplify the process of installing OpenShift on Packet
by introducing installer-provisioned options that take advantage of various
models of compute, memory, GPU, or storage classes of devices, hardware and IP
reservations, virtual networks, and fast direct-attached storage.

Currently, users who want to [deploy OpenShift on
Packet](https://www.packet.com/solutions/openshift/) must follow the [OpenShift
via Terraform on
Packet][https://github.com/RedHatSI/openshift-packet-deploy/blob/master/terraform/README.md]
UPI instructions. This process is not streamlined and can not be integrated into
a simple experience like the [Try OpenShift 4](https://www.openshift.com/try)
workflow.

### Goals

The main goal of the Packet IPI is to provide users with an easier path to
running Red Hat OpenShift 4 on a dynamically provisioned, horizontally scalable,
bare metal architecture in [Packet data
centers](https://www.packet.com/developers/docs/getting-started/datacenters/).

As a first step, we would add the Packet IPI to the installer codebase. With
accompanying documentation, this makes a the Packet option available through the
CLI. This would enable users to deploy OpenShift with IPI in a way that is very
similar to UPI but simplifies the process.

Following other IPI installers, this first step would include

- Making Packet IPI documentation available here:
  <https://github.com/openshift/installer/blob/master/docs/user/packet/install_ipi.md>
- Adapt Packet's sig-lifecyclce ClusterAPI v1alpha2 for use as an OpenShift's
  ClusterAPI v1beta1 Machine driver
- Prepare the Terraform code and Packet types necessary for an IPI installer
- Making a CI job executing the provisioning scripts to test the IPI installer

### Non-Goals

It is outside the scope of this enhancement to provide details about the
installation and infrastructure elements that have been highlighted in [Open
Questions](#open-questions) and [Implementation
Details/Notes/Constraints](#implementation-details-notes-constraints), even for
those elements that are required (e.g. DNS).

When open questions are explored and resolved, they will be added to this
enhancement request or they will be surfaced in additional enhancement requests,
as needed.

## Proposal

- Define and implement Packet types in the openshift/installer
  - High level variables should include:
    - [API Key](https://www.packet.com/developers/docs/API/) (required)
    - [Project
      ID](https://www.packet.com/developers/docs/getting-started/support/organizations-and-users/)
      (required) (Options to create a project could be presented)
  - Boostrap node variables include:
    - Device plan (required, defaulting to minimum required)
    - Facility (required)
    - Virtual Network (optional, defaulted to a new virtual network)
  - Control plane variables rely on boostrap variables and add:
    - Device plan (defaulting to provide a good experience)
  - `Machine`/`MachineSet` fields can be initially based on the v1alpha3
    sig-cluster-lifecycle Packet Cluster API Provider specifications
    ([Machines](https://github.com/kubernetes-sigs/cluster-api-provider-packet/blob/v0.3.2/api/v1alpha3/packetmachine_types.go#L36-L57)),
    but will likely diverge quickly.
    - [Device plan](https://www.packet.com/developers/docs/servers/) (required,
      documentation should use values that provide a good experience)
    - [Facility](https://www.packet.com/developers/docs/getting-started/datacenters/)
      (required)
    - Future:
      - Virtual Networks - [Native
        VLAN](https://www.packet.com/developers/docs/network/advanced/native-vlan/)
        These are not immediately necessary as [Packet Projects within a
        facility share a
        10.x.x.x/25](https://www.packet.com/developers/docs/network/basic/standard-ips/).
      - IP Reservations - [Reserved public IP
        blocks](https://www.packet.com/developers/docs/network/basic/ip-allocation/)
      - Hardware Reservations - [Reserved hardware
        profiles](https://www.packet.com/developers/docs/getting-started/deployment-options/reserved-hardware/)
      - Spot Market Requests - [Support for node pools with a min and max bid
        value](https://www.packet.com/developers/docs/getting-started/deployment-options/spot-market/)
- Write documentation to help users use and understand the Packet installer, to include:
  - Packet usage basics ([accounts and API
    keys](https://www.packet.com/developers/docs/API/))
  - Packet resource requirements and options
  - OpenShift Installer options
  - Non-Packet components that build on the experience (DNS configuration)
- Setup the CI job to have a running test suite

### Implementation Details/Notes/Constraints

- RHCOS must be made available and maintained.

  Images available for provisioning may be custom or official. Official images
  can be hosted outside of Packet, such as ContainerLinux and RancherOS. More
  details area available here: <https://github.com/packethost/packet-images>
  Custom Images can be hosted on git, as described here:
  - <https://www.packet.com/developers/docs/servers/operating-systems/custom-images/>
  - <https://www.packet.com/developers/changelog/custom-image-handling/>
  - Custom images are limited to single filesystem deployments without extended
    attributes (SELinux), raw-disks are not supported.
- It may be possible to boot using the [Packet Custom iPXE
  configuration](https://www.packet.com/developers/docs/servers/operating-systems/custom-ipxe/).

  As described at <https://ipxe.org/howto/rh_san>, it may be possible to use the
  [recommended RHCOS bare metal
  images](https://docs.openshift.com/container-platform/4.5/installing/installing_bare_metal/installing-bare-metal.html#registry-configuring-storage-baremetal_installing-bare-metal)
  ([mirror](https://mirror.openshift.com/pub/openshift-v4/dependencies/rhcos/4.5/)).
  For this to be viable, Packet would need to provide a mirror or cache. Packet
  currently provides some mirroring on `mirrors.{facility}.packet.net` and
  caching on `install.{facility}.packet.net`, anycast DNS is available for the
  latter.

  A generic RHCOS Ignition URL could instruct nodes booted on Packet to
  configure their network using the EC2-like [metadata
  service](https://www.packet.com/developers/docs/servers/key-features/metadata/).

- Packet does not offer a managed DNS solution. Route53, CloudFlare, or RFC 2136
  (DNS Update), and NS record based zone forwarding are considerations. Some
  existing IPI drivers ([such as
  `baremetal`](https://github.com/openshift/installer/blob/master/docs/user/metal/install_ipi.md))
  have similar limitations.
- Packet does not offer a loadbalancer service, but does [offer the networking
  features](https://www.packet.com/developers/docs/network/) that makes this
  possible at the device level.
- [Packet CCM currently offers
  LoadBalancing](https://github.com/packethost/packet-ccm#load-balancers) by
  creating and maintaining a MetalLB deployment within the cluster.  This may be
  impacted by <https://github.com/openshift/enhancements/pull/356>.
- [Packet
  CSI](https://github.com/packethost/csi-packet#kubernetes-container-storage-interface-csi-plugin-for-packet)
  currently requires a minimum of 100GB per volume. Additionally, the [Packet
  EBS service](https://www.packet.com/developers/docs/storage/ebs/) is available
  in a limiting subset of the available facilities.
- Both Public and Private IPv4 and IPv6 addresses made available for each node.
  Private and VLAN Networks can be restricted at the project level (default) or
  node by node. While custom ASNs and BGP routing features are supported, these
  and other networking features will not be exposed through the IPI.
  <https://www.packet.com/cloud/network/>
- Openshift will run on RHCOS directly on bare metal without the need for a
  VM layer.

### Risks and Mitigations

The [official Packet
ClusterAPI](https://github.com/kubernetes-sigs/cluster-api-provider-packet),
[CCM](https://github.com/packethost/packet-ccm), and
[CSI](https://github.com/packethost/csi-packet) drivers are all relatively new
and may make undertake substantial design changes.  This project will either
need to adopt and maintain the current releases, or adopt newer releases
developed alongside this project.

The CI will need to assure that Packet resources are cleanly released after
failed and successful builds.  Packet is currently used in the bare metal
provisioning tests, so there is some prior art available.  If the same account
is used for testing the IPI, it may need account quota increases.

## Design Details

### Open Questions

- How will the API be LoadBalanced?
  - Will this be a task left up to the user (and added documentation)?
  - Will extra devices be dedicated to this purpose?
  - Will DNS round-robin suffice?
  - Can Kube-vip be used for this purpose?
  - Can the API be loadbalanced by the service Loadbalancer?
    [openshift/enhancements#459](https://github.com/openshift/enhancements/pull/459)
- Is the Packet CCM necessary?
  - One of the Packet CCM functions is to provider LoadBalancer support through a (CCM) managed deployment of MetalLB. Enabling LB support is optional.
    - If enabled, who is responsible for supporting the MetalLB installation?
    - If not enabled, how do we take advantage of [Service Load Balancers for On
      Premise
      Infrastructure](https://github.com/openshift/enhancements/pull/356) for
      use with Packet (and Packet BGP features)?
  - Can [https://github.com/plunder-app/kube-vip#56](plunder-app/kube-vip)
    stand-in for LoadBalancer service on Packet while disabling the packet-ccm's
    MetalLB approach to LoadBalancing?
  - Aside from loadbalancing, are other CCM responsibilities critical? These
    include Kubernetes node annotations and Packet API hints that a "device"
    (and therefor node) has been deleted.
- Is the Packet CSI necessary? If not, how will the Openshift image registry be
  created.
- Can the sig-cluster-lifecycle ClusterAPI and OpenShift MachineAPI converge to
  prevent maintaining two similar but different ClusterAPI implementations?
  - This is not likely to happen any time soon. The models are very different at
    this point. We can reuse a lot of the upstream Packet provider verbatim but
    the source of configuration will have to remain embedded in the Machine for
    the foreseeable future.
- How will the RHCOS image be served?
- How will Ignition be started and configure nodes appropriately? After
  pre-boot, Packet's DHCP offers only the primary IPv4 address. The metadata
  service (offered on a public in-facility address) should be used. Afterburn
  has [support for the Packet metadata
  service](https://github.com/coreos/afterburn/blob/master/src/providers/packet/mod.rs).
- Should VLANs (other than the Project-based private network) be used? Should
  these be opinionated or configurable? Are they needed in the first release
  with Packet IPI support?
- What (if any) additional responsibilities can the bootstrap node adopt?
- Are any aspects of the [Assisted Installer
  flow](https://github.com/openshift/enhancements/blob/master/enhancements/installer/connected-assisted-installer.md)
  for bare metal clusters reusable and helpful for the Packet IPI install flow?

### Test Plan

We will use the existing IPI platforms, including such as `baremetal` as
inspiration for our testing strategy:

- A new e2e job will be created
- At the moment, we think unit tests will probably not be necessary. However, we
  will cover any required changes to the existing codebase with appropriate
  tests.

[Packet is currently used by the `openshift/release`
project](https://github.com/openshift/release/tree/a7d6f893020b367ecf59b50477943d5c3d57273b/ci-operator/step-registry/baremetalds/packet)
for `baremetal` testing.

### Graduation Criteria

The proposal is to follow a graduation process based on the existence of a CI
running suite with end-to-end jobs. We will evaluate its feedback along with
feedback from QE's and testers.

We consider the following as part of the necessary steps:

- CI jobs present and regularly scheduled.
- IPI document published in the OpenShift repo.
- End to end jobs are stable and passing and evaluated with the same criteria as
  comparable IPI providers.
- Developers of the team have successfully used the IPI to deploy on Packet
  following the documented procedure.

## Implementation History

Significant milestones in the life cycle of a proposal should be tracked in
`Implementation History`.

## Drawbacks

The IPI implementation is provisioned on bare metal which faces resource
availability limitations beyond those in VM environment. CI, QE, documentation,
and tests will need to be generous when defining provisioning times. Tests and
documentation should also be made flexible about facilities and device model
choices to avoid physical limitations.

## Alternatives

People not using the IPI workflow can follow the [Packet
UPI](https://github.com/RedHatSI/openshift-packet-deploy/blob/master/terraform/README.md)
document. This requires more manual work and the necessary knowledge to identify
Packet specific parts without any automation help.

Users may also follow along with the [Deploying OpenShift 4.4 on
Packet](https://www.openshift.com/blog/deploying-openshift-4.4-on-packet) blog
post, but there is no automation provided.

ProjectID is being defined as a high level variable. Arguably, OrganizationID
could take this position, as this represents the billing account. OrganizationID
is inferred by ProjectID in the current proposal. In a variation where
Organization is required at install, ProjectID could become a required or
optional (inherited) property for each cluster. Keep in mind, Projects are one
way to share a private network between nodes.

## Infrastructure Needed

As has been demonstrated in the Packet UPI, users will need access to a Packet
Project, API Key, and no less than 4 servers. One of these will be used
temporarily for bootstrapping while the other 3 represent the control plane
(optionally acting as compute nodes). The fourth node, used for bootstrapping
can be removed once the cluster is installed.

In the UPI driver, an additional server was used as a bastion node for
installation needs. This [bastion host served the ignition file and RHCOS
image](https://github.com/RedHatSI/openshift-packet-deploy/tree/master/terraform/modules/bastion/templates)
over HTTP and acted as a DHCP/iPXE host for the control-plane nodes. This IPI
implementation will seek to avoid the need for such a node through use of the
CSI driver and a hosted RHCOS image.
