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
scenarios that are both CPU and memory constrained.  The container
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

* CPU: ARM Cortex or Intel Atom class CPU, 2-4 cores, 1.5GHz clock
* memory: 2-16GB RAM
* storage: e.g. SATA3 @ 6Gb/s, 10kIOPS, 1ms (vs. NVMe @ 32Gb/s,
  500kIOPS, 10µs)
* network: Less than data center-based servers. Likely 1Gb/s NICs,
  potentially even LTE/4G connections at 5-10Mbps, instead of
  10/25/40Gb/s NICs

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
   applications, and any other platform components into an image that
   is pre-installed on many devices by the manufacturer.
3. As an application deployer, I can package MicroShift, the
   applications, and any other platform components into an image that
   I install myself.
4. As an device administrator, I can release an updated image over the
   network to upgrade devices already in the field using minimal
   network bandwidth.
5. As an device administrator, I can rely on the system to rollback to
   a good state if an upgrade fails.
6. As an device administrator, I have control over the operating
   system running under MicroShift.

### Goals

* Describe an approach to providing a version of Kubernetes compatible
  with the OpenShift APIs that it includes and that meets the
  constraints described in the Motivation section.
* Identify the minimal set of APIs that will be included and the
  components needed to support them.
* Design a system that can work autonomously, without requiring
  external orchestration tools for deployment.
* Treat MicroShift as an application running on RHEL, with minimal
  privileges.
* Design a system that can be configured by someone familiar with
  Linux, without Kubernetes expertise.
* Follow the [MicroShift design
  principles](https://github.com/openshift/microshift/blob/main/docs/design.md)

### Non-Goals

* We are not trying to fit all of OpenShift into the target hardware
  platforms.
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

Many of the OpenShift APIs are not relevant for edge device use
cases. We are starting with the absolute minimal set for now because
it will be easier to add more APIs later than it will be to remove
anything. In addition to the standard APIs included in core
Kubernetes, the OpenShift APIs supported will be:

* route.openshift.io/v1 for Routes
* security.openshift.io/v1 for SecurityContextConstraints
* authorization.openshift.io/v1 for RBAC APIs such as Role,
  RoleBinding, etc.

In addition to those APIs, we will enable two OpenShift controllers
related to Ingress:

* openshift.io/ingress-ip
* openshift.io/ingress-to-route

MicroShift is built downstream of OpenShift. The versions of the
embedded components are selected by looking at an OpenShift release
image, finding the git SHAs for the direct dependencies, resolving the
secondary dependencies, and vendoring the results into the MicroShift
git repository. The version of the containerized components are taken
from the same release image and the image references are built into
the MicroShift binary.

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
OpenShift is. Instead, we will use a new configuration file for the
control plane (`/etc/microshift/config.yaml`) with [limited
configuration
options](https://github.com/openshift/microshift/blob/main/pkg/config/config.go). The
node-side configuration will be done via the standard kubelet
configuration file.

Other operating system services will be configured outside of
MicroShift using their standard configuration files. There is no
machine-config-operator.

Because MicroShift will be treated as a single application, we will
not expose separate health checks or implement per-component restart
features. If any embedded component is unhealthy, the application will
be treated as unhealthy and be restarted.

### Workflow Description

#### Deploying

Application deployers will use RHEL for Edge tools such as Image
Builder to construct full system images containing RHEL, MicroShift,
agents, custom operating system configuration, data prerequisites, and
optionally application images. Those system images will be installed
on the target devices either individually for development/testing or
via manufacturer imaging processes for large-scale production
deployments. RHEL for Edge uses ostree-based images, and an [example
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

### API Extensions

MicroShift does not add any APIs to OpenShift.

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

If an upgrade fails, the RHEL for Edge greenboot feature will allow
the system to reboot back into the older ostree.

Building a single binary means that secondary dependencies of
components need to be kept compatible. This is especially problematic
during a Kubernetes rebase in Openhift, but applies to many other
dependencies as well. We are working with the control plane team to
find ways to mitigate this risk, including moving some code out of the
OpenShift API server into controllers that would not depend on
Kubernetes at all.

Building a different form factor downstream of OpenShift means it is
possible for behaviors introduced in the dependencies to cause issues
within MicroShift that are not caught upstream. After the basic rebase
automation is working, we will be trying to construct a CI job that
can be run on pull requests to the repositories containing MicroShift
dependencies to test upstream code changes using the MicroShift
end-to-end test job.

The support lifetime of MicroShift is the same as the version of
OpenShift on which it is based, therefore it will be critical to
automate the component update process as much as possible to ensure
that MicroShift stays as closely up to date as it can. The team is
developing a [rebase
script](https://github.com/openshift/microshift/blob/main/scripts/rebase.sh)
which will be used by an automated job to propose update pull
requests. We anticipate periods of time when OpenShift is itself
rebasing to a new version of Kubernetes where MicroShift's
dependencies are not compatible and cannot be updated. We will need to
work with the rebase team to ensure those periods of time are as short
as possible.

Because MicroShift will run as only a single node, for application
availability we will recommend running two single-node instances that
deploy a common application in active/active or active/passive mode
and then using existing tools to support failover between those states
when either host is unable to provide availability.

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

### Test Plan

We will run the relevant Kubernetes and OpenShift compliance tests in
CI.

We will have end-to-end CI tests for many deployment scenarios.

We will have manual and automated QE for other scenarios.

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

### RHEL

Using non-containerized deployment options on RHEL may be appropriate
for some cases, but does not solve the case where a customer is using
multiple variations of OpenShift and wants to construct their
application deployment independently of the cluster footprint.

### podman

Podman is another tool for deploying container-based workloads onto
RHEL systems. It does not support the Kubernetes and OpenShift APIs
that users will be used to seeing in single-node and full OpenShift
clusters, and users have asked us to provide a way to have a
consistent deployment experience for all topologies and sizes.

### Single-node OpenShift

Single-node OpenShift uses far more resources than are available in
the field edge devices where MicroShift will be used. [Work is being
done](https://github.com/openshift/enhancements/blob/master/enhancements/workload-partitioning/management-workload-partitioning.md)
to address the resource consumption, but it is primarily focused on
CPU utilization and not RAM or over-the-wire upgrade costs.

### Separate Binaries

Running the different control plane components as separate binaries,
instead of compiling them into one, would be easier to support as
dependencies drift apart. However, go binaries are large and splitting
them apart would increase RAM consumption far beyond the allowance.

## Infrastructure Needed [optional]

[openshift/microshift](https://github.com/openshift/microshift) has
been created.
