---
title: openstack-baremetal-compute
authors:
  - "@tomassedovic"
  - "@pierreprinetti"
reviewers:
  - "@mandre"
  - "@adduarte"
approvers:
  - "@abhinavdahiya"
  - "@sdodson"
  - "@staebler"
creation-date: 2019-12-10
last-updated: 2021-02-23
status: implemented

---

# Openstack Baremetal Compute

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

OpenStack's [Nova][openstack-nova] service can be configured to provision
baremetal machines (BM) using the same mechanisms as the virtual machines (VM).

This enhancement will allow [Ironic][openstack-ironic]-provisioned machines to
be used as OpenShift Compute Nodes, when they are made available as Nova
flavors.

The OpenShift Control Plane will sit on VMs. The Compute Nodes will be managed
by the Machine API. By virtue of using the same tools and processes as the
fully virtualised OpenStack deployments, it will provide the same integration
with OpenStack services (e.g. storage, load balancer).

## Motivation

In heavy-loaded clusters, or when the workload is better handled with specially
purposed hardware, the functional requirement of controlling where the work is
computed might outweight the added value of virtualisation.

In OpenStack clusters where Nova transparently provisions baremetal machines,
this enhancement will ensure that OpenShift installations can use them as
Compute Nodes.

Existing OpenShift clusters running on a baremetal-enabled OpenStack will be
able to add baremetal workers by adding a new node pool as a day-two operation.

### User Stories

#### Increased performance

User wants to run the Compute Nodes on baremetal machines for increased
performance. The Control Plane sits on regular Nova instances to leverage the
flexibility of virtual machines.

In order to distribute workloads between virtual and baremetal compute nodes in
a mixed environment, OpenShift admins can assign different labels to the
OpenShift nodes and the end-user will then use them in the selectors for their
applications.

### Goals

* Enable running OpenShift MachineSets based on Nova flavors matching
  Ironic-provisioned baremetal Compute Nodes.
* Provide the same integration with the underlying OpenStack services as in the
  fully virtualised environment.
* Baremetal machines can be added and removed from a running OpenShift cluster
  using the Machine API.
* Support both `flat` and `neutron` Ironic Network Interfaces
  - https://docs.openstack.org/ironic/queens/admin/multitenancy.html

### Non-Goals

**Baremetal bootstrap and control plane nodes.**

This proposal focuses on the OpenShift compute nodes only. Deploying or
migrating the Control plane to BMs is out of scope.

**Baremetal deployments on anything other than an OpenStack tenant**

There are other efforts that provide OpenShift on the bare metal. See
[Metal³][metal3-io] and the [Baremetal installers](#baremetal-ipi--upi).

These projects allow deploying OpenShift in an environment where all the nodes
are physical and there is no OpenStack or other cloud layer in-between.

In contrast, this proposal uses the existing OpenStack provider and extends it
to allow adding baremetal workers if the cloud supports it.

**Attaching floating IPs to the BM workers.**

Attaching floating IPs to the BM workers is out of scope of this enhancement.

## Proposal

This enhancement is less about adding a new logical piece, than it is about
uncovering unknowns in a configuration that might work out-of-the-box in
selected environments.

Therefore, the implementation consists of:
* identifying a reference architecture
* anticipating probable issues
* testing and fixing errors

### Characteristics of the reference architecture

Prerequisites:

* The OpenStack cloud runs the [Bare Metal (ironic)][openstack-ironic] service
  to provide baremetal management
* The OpenStack [Compute (nova)][openstack-nova] and [Networking
  (neutron)][openstack-neutron] services are configured to serve bare metal
  nodes using the Nova API
* All of the additional complexity inherent to baremetal is handled by the IaaS
  cloud and its administrators:
  * Enrolling new hardware and discovering its components
  * Wiping the disks between deployments
  * Booting the system up from the desired image
  * Networking
* The choice between deploying a virtual or baremetal worker is done by
  selecting an appropriate Nova flavor and Neutron network
* Both virtual and baremetal machines consume the same image from glance
* The installer will have no insight into the Ironic networking (in particular,
  the installer will have no knowledge whether the network interface is `flat`
  or `neutron`)
  * All it will do is create a node with the user-specified flavor and
      network

**We target two use-cases:**

1. Install on the installer-provisioned network, with BM workers
2. Install on a preexisting network, with BM workers

#### Case 1. Install on the installer-provisioned network, with BM workers

**Description:**

The installer provisions a network and deploys on it both the Control plane VMs
and the Compute node BMs.

**Notes:**

Booting BMs in a virtual network requires specific hardware; this case is a
simplified one and is not expected to be the most represented in real-world
installations.

**Prerequisites for the user:**

* Being able to provision BM nodes on a tenant network
* Ironic needs to be able to listen and PXE-boot nodes in freshly created
  networks

**Expected development work:**

* Adapt the wait-for install-complete timeout, either in code or in
  documentation, in order to accomodate for the BM booting times.
* Adapt the Certificate-Signing-Request timeouts to the BM booting times.
* Document how to use:
  * document the prerequisites (see above)
  * set the Ironic flavor in `install-config.yaml`

#### Case 2. Install on a preexisting network, with BM workers

**Description:**

The installer does not create any virtual network. Instead, it deploys both the
Control plane VMs and the Compute node BMs onto a pre-existing network.

**Notes:**

This case leverages [Provider Networks and Custom
Subnets](openstack-customer-provided-subnets.md). The entire cluster is
installed on the same pre-existing network where the BMs boot.

**Prerequisites for the user:**

* Being able to provision VMs nodes on the preexisting subnet where BMs run.

**Expected development work:**

* Adapt the wait-for install-complete timeout, either in code or in
  documentation, in order to accomodate for the BM booting times.
* Adapt the Certificate-Signing-Request timeouts to the BM booting times.
* Document how to use:
  * set the appropriate Ironic-backed Nova flavor in `install-config.yaml`

### Implementation work

#### Adapt the wait-for install-complete timeout

_Needed for cases 1 and 2_

The last step of the Installer is to check for the availability of the cluster
and report a failed installation if a timeout is exceeded. However, due to the
longer boot times of BM nodes, the installation will take longer than on VMs.
For this reason, the timeout is likely to be hit regularly when installing on
BMs, also for clusters that would converge to a healthy state over time.

To work around the Installer-wide timer, it is recommended to run
`openshift-install wait-for install-complete` as many times as needed.

#### Handle VM to BM networking differences

For every OpenStack worker VM, [CAPO][capo] always creates a Neutron port with
the following configuration:

* Security Groups
* Allowed Address Pairs for the [Ingress VRRP IP
  address][openstack-provider-vip]

The worker node (Nova server) is requested to use this port rather than
creating a new one.

Depending on the ML2 driver these options may not be be available. We expect
that in such cases the network traffic will not be blocked and the deployment
will function as expected.

We may however, need to add a more complex error recovery and fallback whereby
we try to recreate the port without these features or possibly even create a
server without adding an explicit port.

In that case, we should not introduce any new fields to the provider `spec`
field in the `Machine` object. Instead, the code should either recover from
errors or do the right thing if the SG/Address Pair fields are empty.

#### Add the necessary CI infrastructure

The [OpenStack cloud we use for CI][moc] does not currently provide Ironic and
baremetal machines. A considerable portion if this effort will be to ensure
proper testing.

See the [Test Plan](#test-plan) section for more details.

### Probable issues

* When attaching and detaching, the expected timings may need to be adjusted to
  reflect the different boot and shutdown latencies of physical machines

### Risks and Mitigations

#### Certificate-Signing-Request timeouts

In order to be authorised to join the cluster, new workers are expected to
issue a CSR within a predefined timeout. This timeout might not always be
compatible with the bare metal booting times.

#### Development and Testing Environments

None of the OpenStack environments we currently use have access to Ironic or
provide baremetal nodes and we don't have the opportunity to add them easily.

This poses a significant risk to both timely development (dev environment) as
well as support of the feature (CI).

We propose several options in the [Test Plan](#test-plan) section below.

#### Uncertainty about OpenStack's extra networking resources

The OpenShift Installer as well as [cluster-api-provider-openstack][capo]
currently create a port with the following properties set for every node:

* Security groups
* Attached to the network/subnet created during the installation
* Allowed Address pairs set for the VRRP IP addresses defined by the installer
* (optional) network trunk

Some of these options (e.g. security groups) are only supported for certain ML2
drivers. Others (attaching to a given tenant network) only work for the
`neutron` Ironic networking interface but not for the `flat` one.

It is unclear what happens when the installer tries to set any of these in
environments whehere they aren't supported. If that results in error, we may
have to detect these situations, pass them to [CAPO][capo] and modify how we
create the node and port resources.

This can be mitigated by not using [CAPO][capo] for the baremetal nodes.
Instead, the OpenShift administrator would create them manually, pass the
correct Ignition configuration and approve the certificate signing requests
(CSRs) explicitly.

This manual process of adding compute nodes is described in the [OpenStack
user-provisioned infrastructure (UPI) document][openstack-upi] and should work
here as well.

## Design Details

### Test Plan

The OpenStack cloud running CI does not currently support Ironic and bare metal
machines.

Similarly, switching to a different OpenStack provider or building our own
cluster are longer-term solutions that we cannot rely on.

As this is an issue faced by other OpenStack bare metal-related projects such
as Ironic and TripleO, we are going to utilise a similar solution: have the CI
job build out the OpenStack deployment required.

* The CI job will create virtual machines serving as the OpenStack nodes
* It will also create virtual machines serving as the bare metal nodes
* It will install OpenStack on top of the OpenStack virtual nodes
* It will configure the OpenStack to provide bare metal nodes via
  virtualbmc/sushy-tools
* It will run the OpenShift installation on top of that OpenStack

### Graduation Criteria

This enhancement will follow standard graduation criteria.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- Community Testing
- Sufficient time for feedback
- Downgrade and scale testing are not relevant to this enhancement

### Upgrade / Downgrade Strategy

This proposal should have no effect on existing OpenShift clusters running on
OpenStack. We do not plan any changes to the VM-based OpenStack deployments.

However, if an existing deployment runs on an OpenStack cloud that supports
creating bare metal machines, it will be possible to add a bare metal worker
machine pool to it.

Once registered, the bare metal nodes can have workloads migrated to them using
standard OpenShift tools and processes.

## Implementation History

This section will track the pull requests implementing the bare metal compute
feature.

## Drawbacks

This proposal will not support all conceivable configurations of bare metal
integration. In particular, running or migrating to a bare metal control plane
is out of scope.

Relying on the Nova -> Ironic passthrough (as opposed to talking to Ironic
directly) means some of the Ironic-specific information is lost. This allows a
simpler implementation and suits the installer-provisioned processes better.

### Storage

Cinder-provided Ceph storage is not expected to work on most Ironic
deployments. CSI Manila driver is expected to be a requirement for mounting
volumes in such cases.

## Alternatives

Being able to run at least some workloads on baremetal nodes is highly
desirable for certain users. It should be possible to achieve that even without
an explicit support:

### Baremetal IPI / UPI

Support for an installer-provisioned infrastructure (IPI) deployments where all
the nodes are baremetal is currently being developed. Baremetal-based
user-provisioned infrastructure (UPI) deployments are supported today.

Both approaches are all-or-nothing. Every workload running on the OpenShift
cluster will run on a baremetal node. This may get wasteful and expensive.

They also do not provide a simple way of integrating with the OpenStack
services - e.g. using the Object storage as the backing storage for the
OpenShift Image Registry, Block storage for Persistent Volumes, exposing Load
Balancer as a Service to the end users, etc.

The spirit of this enhancement is to provide bare metal capabilities to
OpenStack-based deployments without losing any of the existing integration.
This will be more difficult when using the bare metal provider.

### OpenStack UPI

It should be possible to add baremetal compute nodes manually to an existing
deployment.

The process should be similar to adding workers in the OpenStack
user-provisioned case. Rather than using cluster-api-provider-openstack, the
baremetal machines will be configured and booted up manually, loading the
worker Ignition configuration from the cluster's machine-config-server.

This should work both with a full-on UPI as well as simply adding additional
baremetal nodes to an IPI-deployed cluster.

[openstack-nova]: https://docs.openstack.org/nova "OpenStack Compute (nova)"
[openstack-ironic]: https://docs.openstack.org/ironic "OpenStack BareMetal (Ironic)"
[openstack-neutron]: https://docs.openstack.org/neutron "OpenStack Networking (Neutron)"
[metal3-io]: https://metal3.io/
[baremetal-provider]: https://github.com/openshift/installer/tree/master/docs/user/metal
[openstack-upi]: https://github.com/openshift/installer/blob/master/docs/user/openstack/install_upi.md
[capo]: https://github.com/openshift/cluster-api-provider-openstack
[openstack-provider-vip]: https://github.com/openshift/installer/blob/master/docs/design/openstack/networking-infrastructure.md#virtual-ips
[moc]: https://massopen.cloud/
[openshift-customization]: https://github.com/openshift/installer/blob/master/docs/user/customization.md#platform-customization "OpenShift Installer docs: Platform Customization"
