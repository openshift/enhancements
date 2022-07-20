---
title: kubernetes-for-device-edge
authors:
  - dhellmann
  - fzdarsky
  - derekwaynecarr
reviewers:
  - "@copejon, MicroShift contributor"
  - "@deads2k, control plane expert"
  - "@fzdarsky, MicroShift architect"
  - "@ggiguash, MicroShift contributor"
  - "@jogeo, MicroShift contributor"
  - "@mangelajo, MicroShift contributor"
  - "@oglok, MicroShift contributor"
  - "@sallyom, MicroShift contributor"
approvers:
  - "@derekwaynecarr"
api-approvers:
  - None
creation-date: 2022-07-12
last-updated: 2022-07-12
tracking-link:
  - https://issues.redhat.com/browse/OCPPLAN-9080
  - https://issues.redhat.com/browse/USHIFT-50
see-also:
  - https://github.com/openshift/microshift/blob/main/docs/design.md
---

# Kubernetes for Device Edge

## Summary

This enhancement describes MicroShift, which provides an essential
container orchestration runtime compatible with Kubernetes and
OpenShift built for Internet-of-things (IoT) and Edge computing
scenarios that are constrained in CPU, memory, and network bandwidth.  The container
orchestration runtime is binary compatible with OpenShift Container
Platform, but it is not 100% API resource compatible.  It has a strict
subset of OpenShift API resources pertinent for IoT and Edge computing
scenarios with a strong bias to retaining traditional runtime security
controls like SecurityContextConstraints and SELinux.

## Motivation

MicroShift addresses customer use cases with low-resource,
field-deployed edge devices (SBCs, SoCs) requiring a minimal K8s
container orchestration layer.

MicroShift is targeting a class of devices like Xilinx ZYNQ
UltraScale+, fitlet2, NVIDIA Jetson TX2, or Intel NUCs that are
pervasive in edge deployments. These cost a few hundred USD, have all
necessary accelerators and I/O on-board, but are typically not
extensible and are highly constrained on every resource, e.g.:

* CPU: Low-power ARM or Intel Atom CPU, 2-4 cores, 1.5GHz clock
* memory: 2-16GB RAM
* storage: e.g. SATA3 @ 6Gb/s, 10kIOPS, 1ms (vs. NVMe @ 32Gb/s,
  500kIOPS, 10µs)
* network: Less bandwidth and less reliable than data center-based
  servers, including being completely disconnected for extended
  periods. Likely 1Gb/s NICs, potentially even LTE/4G connections at
  5-10Mbps, instead of 10/25/40Gb/s NICs

These use cases are not being addressed by RHEL or OpenShift
today. RHEL doesn’t include Kubernetes. OpenShift is fundamentally
designed for a cloud-centric and server-centric world, with
operational models and assumptions about environmental constraints
that are very different from field-deployed devices. Even single-node
OpenShift deployments target traditional multi-socket data center
computing platforms, not the small edge devices targeted by
MicroShift.

### User Stories

An application developer builds applications.

An application deployer packages and distributes those applications.

A device administrator manages the lifecycle of a device running the
applications.

1. As an application developer, I can design my application deployment
   so that it works with MicroShift and all other topologies of
   OpenShift in a consistent way.
2. As an application deployer, I can package MicroShift, the
   applications, and any other platform components into a "system image" that
   is written onto many devices by the manufacturer.
3. As an application deployer, I can package MicroShift, the
   applications, and any other platform components into a "system image" that
   I install myself.
4. As a device administrator, I can release an updated system image over the
   network to upgrade devices already in the field using minimal
   network bandwidth.
5. As a device administrator, I can specify application-specific logic
   with which the device will verify a successful upgrade and roll
   back the upgrade automatically if verification fails.
6. As a device administrator, I have control over the operating system
   running under MicroShift and can install kernel drivers, other
   applications, agents, etc. easily.

### Goals

* Describe an approach to providing a version of Kubernetes compatible
  with the OpenShift APIs that it includes and that meets the
  constraints described in the Motivation section.
* Identify the minimal set of APIs that will be included and the
  components needed to support them.
* Design a system that can work autonomously, without requiring
  external orchestration tools for deployment.
* Treat MicroShift as an application running on RHEL, with the minimum
  privileges possible.
* Design a system that can be configured by someone familiar with
  Linux, without Kubernetes expertise.
* Follow the [MicroShift design
  principles](https://github.com/openshift/microshift/blob/main/docs/design.md)

### Non-Goals

* We are not trying to fit all of the features of a standalone
  OpenShift deployment into the target hardware platforms.
* We are not trying to make MicroShift self-managing, so no core
  OpenShift operators will be included.
* We do not plan to support multi-node clusters.
* The details of the default CSI driver are left to a future
  enhancement.
* The details of the default CNI driver are left to a future
  enhancement.
* Use cases beyond industrial applications, such as developer
  environments, are out of scope for now.
* The design of MicroShift relies heavily on features of RHEL for Edge
  and rpm-ostree. Those features are referenced here, but their
  implementation is not described in detail.
* The details of integrating with fleet orchestration tools such as
  Multi-cluster Engine (MCE) or Advanced Cluster Manager (ACM) are
  left to a future enhancement.
* We do not want to push MicroShift use on RHEL versions where OCP is
  not supported. We do have interest from users in RHEL 9, but will
  not GA on RHEL 9 faster than OCP does.
* We do not want to push MicroShift use on hardware platforms where
  RHEL for Edge is not supported. We may use interest in MicroShift to
  drive prioritization of hardware support, but we would not support
  MicroShift on a platform where RHEL is not supported.

## Proposal

To meet the resource consumption constraints (RAM and CPU), MicroShift
is designed as an all-in-one binary for the components that are
required to launch a Kubernetes control plane, combined with
components running in containers to provide runtime services after the
control plane is up.

Today the all-in-one binary includes:

* etcd
* Kubernetes API server
* Kubernetes controller manager
* Kubernetes scheduler
* Kubelet
* Kube Proxy Server
* OpenShift API server
* OpenShift controller manager
* OpenShift SCC manager

The pod-based components include:

* CoreDNS
* service CA operator
* CSI driver
* CNI driver

To allow deployers to customize and manage the OS, MicroShift will run
on [RHEL for
Edge](https://www.redhat.com/en/resources/meet-workload-demands-edge-computing-datasheet),
which is designed for field-deployed devices and use cases.
MicroShift will take advantage of the existing RHEL for Edge work by
running as an application managed by systemd and using as few
privileges as possible. Deployers will use standard RHEL for Edge
tools such as RPM, rpm-ostree, and Image Builder to construct system
images and distribute software updates. Deployers will be responsible
for configuring the operating system components and anything that runs
on the device outside of MicroShift.

Administrators of edge devices may have little or no control over the
network to which the devices are attached, especially the DNS or DHCP
services on those networks. Edge devices are unlikely to have DNS
entries added for MicroShift. MicroShift itself must also tolerate the
host changing IP address when a DHCP lease expires or the host
reboots.

Many of the OpenShift APIs are not relevant for edge device use cases.
We are starting with the absolute minimal set for now because it will
be easier to add more APIs later than it will be to remove anything.
It is meant as a pure runtime tool, and will not be used for
build/dev-cluster use cases, therefore we will not include Builds,
BuildConfigs, etc.  MicroShift is not an interactive, multi-user
environment, therefore we do not include some of the authorization
APIs such as users, projects, etc.  It doesn't manage OS or
infrastructure, so we do not include MachineConfig and related APIs.
MicroShift is a monolith, therefore we do not include any cluster
configuration APIs.

In addition to the standard APIs included in core Kubernetes, the
OpenShift APIs supported will be:

* route.openshift.io/v1 for Routes
* security.openshift.io/v1 for SecurityContextConstraints
* authorization.openshift.io/v1 for RBAC APIs such as Role,
  RoleBinding, etc.

In addition to those APIs, we will enable two OpenShift controllers
related to Ingress:

* openshift.io/ingress-ip
* openshift.io/ingress-to-route

MicroShift is built downstream of OpenShift to ensure that it includes
the same versions of the embedded components that are present in
standalone OpenShift, allowing us to maintain binary compatibility.
The versions of the embedded components are selected by looking at an
OpenShift release image, finding the git SHAs for the direct
dependencies, resolving the secondary dependencies, and vendoring the
results into the MicroShift git repository. That process needs to be
as automatic as possible, so we want to avoid carrying any
MicroShift-specific patches that must be applied after a rebase. The
version of the containerized components are taken from the same
release image and the image references are built into the MicroShift
binary.

A great deal of effort has gone into making OpenShift 4 self-managing,
especially when it comes to multi-node deployment and upgrades. We do
not want to reinvent that work, nor do we want to recreate the
OpenShift 3 model using an outside orchestration tool.  A single-node
control plane becomes a single point of failure when worker nodes are
attached, which eliminates the high availability benefits of running
workloads on Kubernetes. Those statements taken together lead us to
conclude that MicroShift will only support deployments with 1 node
acting as both control plane and worker.

Because MicroShift does not reuse the system operators from OpenShift,
it will not be configured through API resources in the way that
OpenShift is. Instead, we will use a new configuration file to expose
all of the configuration settings that make sense for MicroShift users
to access to control the embedded components
(`/etc/microshift/config.yaml`) with [limited configuration
options](https://github.com/openshift/microshift/blob/main/pkg/config/config.go).

Other operating system services will be configured outside of
MicroShift using their standard configuration files. There is no
machine-config-operator.

Because MicroShift will be treated as a single application, we will
not expose separate health checks or implement per-component restart
features. If any embedded component is unhealthy, the application will
be treated as unhealthy and be restarted.

MicroShift embeds etcd, but will not expose many of its configuration
options. In particular, on-disk encryption will be handled at the host
level using FDE via dm-crypt/LUKS with the key stored in TPM.

### Workflow Description

#### Deploying

Application deployers will use RHEL for Edge tools such as Image
Builder to construct full system images containing RHEL, MicroShift,
agents, custom operating system configuration, data prerequisites, and
optionally application images. Those system images will be written to
the target devices either individually (via the RHEL installation
process) for development/testing or via manufacturer imaging processes
for large-scale production deployments. RHEL for Edge uses
ostree-based images, and an [example
workflow](https://github.com/openshift/microshift/blob/main/docs/rhel4edge_iso.md)
for constructing a basic image is described in the MicroShift
documentation. We are not creating a separate installer for
MicroShift.

#### Upgrading

Because the operating system is based on rpm-ostree, upgrades are
applied either by re-imaging the device, or by using the ostree update
process and rebooting into a new ostree. The ostree images can be
updated by downloading only the differences, which satisfies the
requirement to minimize over-the-wire bandwidth costs of upgrades.

An [example workflow illustrating
upgrades](https://github.com/redhat-et/microshift-demos/tree/main/ostree-demo)
is described in the MicroShift demos repository.

There is no cluster-version-operator. MicroShift is not
self-upgrading, although another agent on the host may participate in
orchestrating upgrades.

#### Configuring

MicroShift reads its configuration file once on startup. Changes to
the file will only be implemented after the application is
restarted. Most configuration changes in production settings would be
rolled out as new ostree images.

#### Deploying Applications

Applications are deployed on MicroShift using standard Kubernetes
resources such as Deployments. They can be deployed at runtime via API
calls, or embedded in the system image with the manifests loaded from
the filesystem by MicroShift on startup. By using the `IfNotPresent`
pull policy and adding images to the CRI-O cache in the system image,
it is possible to build a device that can boot and launch the
application without network access.

### API Extensions

MicroShift does not add any APIs to OpenShift.

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

If an upgrade fails, the RHEL for Edge [greenboot
feature](https://www.redhat.com/en/blog/automating-rhel-edge-image-rollback-greenboot)
will allow the system to reboot back into the older ostree. Success
and failure of an upgrade can be defined by the application developer
and device administrator to include MicroShift, the applications
running on MicroShift, or anything else running on the device.

Building a single binary means that secondary dependencies of
components need to be kept compatible. This is especially problematic
during a Kubernetes rebase in OpenShift, but applies to many other
dependencies as well. We are working with the control plane team to
find ways to mitigate this risk, including moving some code out of the
OpenShift API server into controllers that would not depend on
Kubernetes at all.

Building a different form factor downstream of the OpenShift release
image means it is possible for behaviors introduced in the
dependencies to cause issues within MicroShift that are not caught
upstream. After the basic rebase automation is working, we will be
trying to construct a CI job that can be run on pull requests to the
repositories containing MicroShift dependencies to test upstream code
changes using the MicroShift end-to-end test job.

The support lifetime of MicroShift is the same as the version of
OpenShift on which it is based, therefore it will be critical to
automate the component update process as much as possible to ensure
that MicroShift stays as closely up to date as it can. The team is
developing a [rebase
script](https://github.com/openshift/microshift/blob/main/scripts/rebase.sh)
which will be used by an automated job to propose update pull
requests. We anticipate periods of time during the development of
minor version updates when OpenShift is rebasing to a new version of
Kubernetes where MicroShift's dependencies are not compatible and
cannot be updated. We will need to work with the rebase team to ensure
those periods of time are as short as possible.

Because MicroShift will run as only a single node, for application
availability we will recommend running two single-node instances that
deploy a common application in active/active or active/passive mode
and then using existing tools to support failover between those states
when either host is unable to provide availability.  This
configuration is more reliable than a single node control plane with a
worker, because if the worker loses access to the control plane
(through power or network loss), the worker has no way to restore or
recover its state, and all workloads could be affected.

Because MicroShift does not embed the operating system, its life-cycle
is independent of the OS. The components built into or delivered with
MicroShift will be version-locked together. We will need to test
MicroShift with the specific OS versions that we intend to support,
which will necessarily increase the support matrix during the period
when both RHEL 8 and 9 are covered.

Some of the components being compiled together into a single binary
are not intended to be used that way and are not tested that way
upstream of MicroShift. There is community support for MicroShift and
other similar projects, such as [k3s](https://k3s.io), which should
help us if we encounter problems with the technical direction of those
dependencies.

### Drawbacks


## Design Details

### Open Questions [optional]

1. How can we best minimize the window of time when OCP components are
   not using the same version of Kubernetes?
2. Can we further reduce the dependencies of MicroShift by making
   changes in any of the upstream components?
3. Given the embedded image references, how will we build an
   OKD-equivalent version of MicroShift?
4. We anticipate periods of time during the development of minor
   version updates when OpenShift is rebasing to a new version of
   Kubernetes where MicroShift's dependencies are not compatible and
   cannot be updated. We also update the version of Kubernetes used in
   z-stream branches. Are we likely to have the same problem with
   those updates?

### Test Plan

We will run the relevant Kubernetes and OpenShift compliance tests in
CI.

We will have end-to-end CI tests for many deployment scenarios.

We will have manual and automated QE for other scenarios.

We will submit Kubernetes conformance results for MicroShift to the
CNCF.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to build MicroShift images and deploy them
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

The first release will have "limited availability" to a few partners
and customers.

- CSI driver selected
- CNI driver selected
- Reliable automated rebase process in place
- More testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Relative API stability

#### Full GA

- Available by default

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

See the discussion of upgrades earlier.

### Version Skew Strategy

There is no version skew for most MicroShift components because they
are built in or running from images for which the references are built
in. We expect to use the same CRI-O version distributed with each
version of OCP, although that will not be rebuilt for MicroShift.

### Operational Aspects of API Extensions

No API extensions.

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

* [openshift/microshift](https://github.com/openshift/microshift)
* [Design guidelines](https://github.com/openshift/microshift/blob/main/docs/design.md)

## Alternatives

### Yum-based RHEL

Using non-containerized deployment options on RHEL may be appropriate
for some cases, but does not solve the case where a customer is using
multiple variations of OpenShift and wants to construct their
application deployment independently of the cluster footprint.

### podman

Podman is another tool for deploying container-based workloads onto
RHEL systems. It does not support the Kubernetes and OpenShift APIs
that users will be used to seeing in single-node and full OpenShift
clusters, such as Kubernetes orchestration, services, etc.  Users have
asked us to provide a way to have a consistent deployment experience
for all OpenShift topologies and sizes.

### Single-node OpenShift

Single-node OpenShift targets server-class hardware or cloud VMs and
therefore uses far more resources than are available in the field edge
devices where MicroShift will be used. [Work is being
done](https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md)
to address the resource consumption, but it is primarily focused on
CPU utilization and not RAM or over-the-wire upgrade costs and the
resulting deployment still includes all of the OpenShift operators,
which are not wanted for MicroShift use cases.

Single-node OpenShift is designed to encapsulate management of the
underlying operating system, and does not currently support the sort
of user-built images that are needed to support edge device
scenarios. The [CoreOS
Layering](https://github.com/openshift/enhancements/blob/master/enhancements/ocp-coreos-layering.md)
work will change that.

Single-node OpenShift relies on being installed and does not support
the sort of factory imaging process using customer-built images that
is typically needed for edge devices.

Single-node OpenShift, like other standalone OpenShift configurations,
implements a roll-forward deployment model and does not support
rolling the system back when an issue is encountered during upgrades.

### Separate Binaries or Processes

Running the different control plane components as separate processes
using different binaries, instead of compiling them into one, would be
easier to support as dependencies drift apart. However, go binaries
are large and splitting them apart would increase RAM consumption far
beyond the allowance.

Separate processes would also require more complex orchestration to
start, stop, upgrade, etc. We want to avoid recreating or running all
of the logic currently in OpenShift's cluster operators.

## Infrastructure Needed [optional]

[openshift/microshift](https://github.com/openshift/microshift) has
been created.
