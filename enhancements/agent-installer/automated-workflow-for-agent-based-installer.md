---
title: automated-workflow-for-agent-based-installer
authors:
  - "@dhellmann"
reviewers:
  - "@avishayt"
  - "@celebdor"
  - "@lranjbar"
  - "@mhrivnak"
  - "@patrickdillon"
  - "@pawanpinjarkar"
  - "@rwsu"
approvers:
  - "@zaneb"
api-approvers:
  - None
creation-date: 2022-03-17
last-updated: 2022-03-17
tracking-link:
  - https://issues.redhat.com/browse/AGENT-10
see-also:
  - "/enhancements/agent-installer/agent-based-installer.md"
  - "/enhancements/installer/single-node-installation-bootstrap-in-place.md"
  - "/enhancements/network/baremetal-ipi-network-configuration.md"
  - "https://github.com/openshift/enhancements/pull/1034"
  - "/enhancements/network/baremetal-ipi-network-configuration.md"
replaces: N/A
superseded-by: N/A
---

# Automated Workflow for Agent-based Installer

## Summary

"Cluster 0" deployments, for the first cluster in an environment, are
unique because they occur in situations where there may not be a lot
of other infrastructure to support long-running services normally
associated with automated provisioning for on-premises use
cases. Nevertheless, users and partners want to automate these
deployments. The assisted installer GUI provides an excellent user
experience for deploying connected clusters. This enhancement covers
the *user-provided automation* use case for disconnected clusters.

## Motivation

### Goals

- The cloud-hosted assisted installer service can be used to deploy in
  fully connected on-premises environments and its REST API can be
  used to automate those deployments. In this enhancement, we need to
  describe a workflow for automated deployment of OpenShift clusters
  for fully disconnected use cases.
- We want to focus on the "cluster 0" use case, where there is no
  cluster to host a kubernetes API or other service. This
  differentiates this workflow from something like ACM or Hive or
  Hypershift, which may be used for multi-cluster provisioning or
  management in the same environment.
- We want to minimize the amount of pre-deployment work that an admin
  has to do before they can start taking steps to deploy their
  cluster.
- Many of our users and partners want to own the overall orchestration
  for deployment, of which the OpenShift installer is only a small
  part of a larger process. To support that goal, we want to be
  agnostic to the tools used to provision (virtual) machines so that
  users can use their own (possibly pre-existing) tooling for
  provisioning.  This lets us support the widest variety of hardware
  and allows the user to work with tools that are already familiar to
  them. This approach also differentiates this workflow from the
  existing installer-provisioned infrastructure workflow, which has
  automation built into the installer.
- We want to minimize the extra hardware requirements for the process,
  by taking steps to eliminate the host needed to run a REST API
  service or act as a hypervisor for a bootstrap VM, for example.
- We want to install clusters with [single-node, compact, and
  high-availability
  topologies](/enhancements/single-node/cluster-high-availability-mode-api.md).
- We want to support install configurations with platform `none`,
  `baremetal`, or `vsphere`.
- We want to avoid inventing completely new ways to describe OpenShift
  clusters. Therefore we will reuse existing inputs to the installer
  and to tools such as the zero-touch provisioning (ZTP) system in
  multi-cluster engine (MCE).
- On-premises deployments do not always have DHCP or other remote
  services available to configure host networking. They also
  frequently use more complex network configurations such as bonded
  NICs and tagged VLANs. We therefore need to support configuration of
  per-host network settings while installing.
- On-premises deployments, especially on bare metal, may need to take
  extra steps to fully utilize storage inside each host or visible to
  a host from a SAN. We therefore need to support configuration of
  per-host storage, including fibre channel settings such as
  multipath.
- We want to retain the OpenShift installer's ability to customize a
  deployment with arbitrary manifests.

### Non-Goals

- A user-driven workflow will be described in a separate enhancement.
- A user interface for collecting complex inputs such as per-host
  network settings before running the installation automatically is
  left to a future enhancement.
- Changes to the release image mirroring process are out of scope for
  this enhancement.
- Eliminating the requirement for a separate image registry to host
  the mirrored OpenShift image is left to a future enhancement.
- Describing a mechanism to add and remove nodes from the cluster after
  it is installed is left to a future enhancement.
- Generating image formats other than ISO is left to a future
  enhancement.
- Booting hosts via PXE is left to a future enhancement.
- Creating minimal ISO images with a separate rootfs is left to a
  future enhancement.
- We are not currently considering replacing the IPI installer or
  assisted installer SaaS.
- Describing an end-to-end solution for users without their own
  orchestration tool is left to a future enhancement.
- Collecting telemetry data for deployments is left to a
  future enhancement.
- Details of securing the communication between agents and services to
  ensure only desired hosts join the cluster are left to a future
  enhancement.
- Details of a workflow for deploying in settings with DHCP assignment
  of IP addresses is left to a future enhancement.
- It is not currently a requirement that the downloadable form of the
  installer support installing multiple versions of OpenShift. We will
  address the underlying requirements behind that request in other
  ways, which will be discussed elsewhere.

## Proposal

We propose to extend the `openshift-install` binary with a new
sub-command, `agent create-image`, for generating a bootable image
containing all of the information needed to deploy a cluster.  A
command line tool avoids issues with prerequisites for deploying a
long-running service, and can be run on a low-resource host such as a
laptop or even on a host at a different site before deployment begins.

Most enterprise grade hardware includes a management controller that
can present an ISO image to a host virtually, allowing the host to
boot from the image remotely. Hypervisors such as kvm and vSphere also
support booting from ISOs. Therefore, we will concentrate on that
format and approach first. Building minimal ISOs with separate root
filesystems or images that can be used with hosts that PXE boot will
come later.

We propose to leave up to the user's orchestration tool the
responsibilities of managing and storing the images, serving them on
the network, and especially configuring hosts to boot them. This gives
users the flexibility to perform those operations in any way they
like, with whatever tool they prefer. A design for an all encompassing
approach based on tools provided entirely by Red Hat is left for a
later enhancement.

The `agent create-image` command will need file-based inputs to
describe the cluster and node-specific configuration. Using the
existing file and custom resource types allows us to use the same
version management techniques that we do for the APIs within a
cluster.  We will use the `install-config.yaml` format for
cluster-wide settings. We will use a new file, tentatively named
`agent-config.yaml`, for settings that apply to only agent-based
deployments.

We will use manifests containing
[NMStateConfig](https://github.com/openshift/assisted-service/blob/master/api/v1beta1/nmstate_config_types.go)
resources to specify static network settings for the hosts. Although
there are similar fields in `install-config.yaml`, they are only
available when the platform setting is `baremetal`, and the
agent-based installer must support other platforms.

We anticipate using
[Butane](https://github.com/coreos/butane/blob/main/docs/config-openshift-v4_10.md)
file in the future for specifying storage settings, although that
decision is subject to change in a future enhancement.

The `agent create-image` command will produce a single image as output, to
save storage space and reduce management complexity. The assisted
installer service can already generate images with boot scripts that
recognize which host is booting and apply host-specific network
settings and roles (such as running different services) accordingly,
and we will reuse that technique (and code).

The image generated by the `agent create-image` command will be an RHCOS
live ISO with embedded Ignition config containing systemd units to
start the assisted service, agent, network, etc. running in containers
pulled from the OpenShift release payload. Using RHCOS ensures that
the same OS that will run OpenShift boots on the hosts. Using
containers in the release payload means that when the payload is
mirrored all of the tools needed to run the agent based deployment
workflows will be present in the mirror.  More details about the
various services follow below.

The image generated by the `agent create-image` command will also
contain the data in the input `install-config.yaml`,
`agent-config.yaml`, and other files. That data may be converted to
another format (we anticipate using the ZTP CRDs, since those map to
the assisted-service API calls). The intermediate format will be
considered another layer of control for the cluster creator, just as
the manifests written by the `create manifests` sub-command are. The
cluster creator will be able to generate those intermediate files with
the sub-command `agent create-manifests`.

The image generated by the `agent create-image` command will also
contain one-time generated information, such as the InfraEnv ID needed
by the agents to join a cluster deployment and REST API credentials
needed for the agents and other clients to communicate securely with
the assisted service.

One host, designated as "node0", will be identified by the user
through its IP address and configured to run the assisted service and
a client that uses the assisted service REST API to automate the
deployment. Because we must support single-node and compact
deployments, the node selected as node0 must always be configured as a
control plane node. Selecting other control plane nodes may be left to
the user.

All hosts will run the assisted agent, configured to talk to the
service on node0.

When all of the hosts are booted with the image, the client on node0
will use the assisted service to trigger the deployment of the cluster
(including applicable pre-flight validations) without further
interaction from the user.

The `agent create-image` command will produce as output credentials to
access the cluster that is about to be created. This allows a client,
such as a tool monitoring deployment progress, to use the cluster's
API as soon as the API service is available.

During deployment, the third-party orchestration tool will need a way
to monitor progress and discover errors. The assisted service
read-only API endpoints running on node0 will report that data. In the
event of failure, a read-only endpoint in that service will be
available to download any logs that have been collected.

We will be taking advantage of the assisted service's ability to run
the bootstrap services "in place" to avoid having a separate bootstrap
VM. Bootstrap will run on node0, where the assisted service runs, to
ensure that the service and its API are available for as long as
possible.

When bootstrapping completes and node0 reboots, progress monitoring
will need to shift to the OpenShift APIs (`ClusterVersion` and
`ClusterOperator`). We will extend the `openshift-install` binary with
a new sub-command, `agent wait-for install-complete`, to use both APIs for monitoring
and [progress
reporting](https://github.com/openshift/assisted-service/blob/master/docs/enhancements/installation-progress-bar.md)
in that case, including reporting when steps like validation fail and
the cluster deployment is never going to succeed. *Users will still be
able to write their own monitoring logic using the assisted service
and OpenShift APIs directly, but they will have the option of using
the command line tool.*

For the fully automated workflow, the steps a user might normally
perform such as approving agents for installation or triggering the
installation will be handled by a new client running as a service on
node0. The client will read the user's inputs from files included in
the image and interact with the assisted-service REST API on localhost
to drive the deployment. For the purposes of this document, we will
refer to that service as a single entity called `create-cluster`,
although in practice we may divide some of the steps into separate
tools.

### Actors in the Workflow

**infrastructure admin** is a human user responsible for infrastructure
within the data center, such as an image registry, networking, etc.

**cluster creator** is a human user responsible for deploying a cluster.

**orchestration tool** is intended to be a software component provided
by the end user for managing the overall deployment process, including
any steps for which the installer is not responsible. Those steps may
also be performed by **cluster creator** if no software component is
available.

**installer** is the OpenShift installer binary, `openshift-install`.

**node0** is one of the hosts that will join the cluster. It runs the
assisted service and a client that uses the assisted service REST API
to automate the deployment.

### Steps for Deploying a Disconnected Cluster

1. The infrastructure admin launches an image registry visible to the
   hosts that will form the cluster and to a host that has internet
   access so the release image can be downloaded.
2. The cluster creator obtains the `oc` command line tool and uses `oc`
   to copy the OpenShift release image into the local image registry
   in one of the usual ways with [`oc adm
   mirror`](https://docs.openshift.com/container-platform/4.10/installing/disconnected_install/installing-mirroring-installation-images.html)
   or [the `oc mirror`
   plugin](https://docs.openshift.com/container-platform/4.10/installing/disconnected_install/installing-mirroring-disconnected.html).
3. The cluster creator obtains the OpenShift installer in one of [the
   usual
   ways](https://docs.openshift.com/container-platform/4.9/installing/installing_bare_metal/installing-bare-metal.html#installation-obtaining-installer_installing-bare-metal).
4. The cluster creator triggers their orchestration tool to run the
   installer.
5. The orchestration tool (or cluster creator) prepares an
   `install-config.yaml`, including `NMState` content to configure the
   static network settings for the hosts.
6. The orchestration tool runs `openshift-install agent create-image`.
7. The installer extracts a copy of the RHCOS ISO from the OpenShift
   release payload.
8. The installer combines the inputs provided by the user with
   generated information like the InfraEnv ID and REST API credentials
   to produce an Ignition config.
9. The installer combines the Ignition config and the RHCOS ISO to
   create the bootable image configured to run the assisted service and/or
   agent based on the host that boots the image and writes the new
   image and cluster credentials to disk.
10. The orchestration tool copies the ISO to a location that the hosts
    or VMs can boot from it. For bare metal, that will generally be a
    web server visible to the management controllers in the bare metal
    hosts (because the management controllers are unlikely to be on
    the same network as the external interfaces for the cluster
    hosts). For vSphere or other hypervisors, that may be a filesystem
    on the hypervisor host.
11. The orchestration tool configures the host to boot the ISO. For
    bare metal, that means configuring the baseboard management
    controller (BMC) in each host to expose the ISO through the
    virtual media interface. For vSphere or other hypervisors, similar
    settings of the VM need to be adjusted.
12. The orchestration tool boots the hosts.
13. The orchestration tool uses `openshift-install agent wait-for install-complete` to
    watch the deployment progress and wait for it to complete.
14. On each host, the startup script in the image selects the correct
    network settings based on the MAC addresses visible and applies
    the network settings for each interface.
15. On node0, the startup script in the image recognizes that it is on
    node0 and launches the assisted service, passing generated
    one-time-use data such as the InfraEnv ID and REST API
    credentials. It also starts the create-cluster service to drive
    the deployment.
16. On node0, the create-cluster service waits for the assisted
    service to start, then creates an InfraEnv using the API.
17. In parallel
    - On node0, the create-cluster service waits for the number of
      agents registered with the service to match the expected value
      based on the input from the user.
    - On all nodes (including node0), the startup script in the image
      launches the assisted agent with the InfraEnv ID and configured
      to connect to the assisted service on node0.
18. All of the agents register their hosts with the assisted service.
19. The assisted service performs its standard validations (network
    connectivity, storage space, RAM, etc.) for all of the hosts.
20. On node0, the create-cluster service sees that all known agents
    have registered and the hosts have passed validation. It uses the
    assisted service API to trigger the deployment to proceed.
21. The assisted service performs the cluster deployment in its usual
    way, except that the nodes will use the ISOs they booted from to
    install RHCOS instead of fetching a new image from the assisted
    service.
22. The `openshift-install agent wait-for install-complete` process sees the OpenShift
    API become available and starts using it to watch the cluster
    complete deployment, combining the information provided by
    `ClusterVersion` and `ClusterOperator` resources with the assisted
    service progress API.
23. On node0, when all of the other hosts have been deployed and
    bootstrapping is complete, the agent reboots node0.
24. node0 boots from its internal storage and joins the cluster as a
    control plane node.
25. The `openshift-install agent wait-for install-complete` process loses
    the connection to the assisted service REST API and starts relying
    entirely on the OpenShift API for data.
26. The `openshift-install agent wait-for install-complete` process sees that the
    `ClusterVersion` API in the cluster shows that the deployment has
    completed, and reports success then exits.
27. The orchestration tool may take steps to clean up (removing the
    ISO, disconnecting it from the boot sequence for the VMs or BMCs,
    etc.).

#### Variation for Assisted Service Failing to Start

If the assisted service fails to start on node0, then the agents will
not be able to communicate with it. The agents will log error messages
to systemd logs and the console including the URL of the service they
are trying to contact so that the cluster creator can discover the
source of the error.

#### Variation for Agent Failing to Register

If any of the expected agents fail to register with the assisted
service, deployment will not progress. The `agent wait-for install-complete` command
will time out after a suitable period (duration to be determined),
report that deployment has failed, and exit with an error code so that
the calling orchestration tool can handle the error.

Customers integrating directly with the assisted service REST API are
responsible for defining their own timeout, recognizing it, and
handling it accordingly.

Logs for the agent that has failed to register will not be collected
automatically, since the agent is not communicating with the
service. Agent logs in systemd and to the console may be useful for
debugging the failure.

#### Variation for Host Failing Validation

If any of the expected agents fail validation after registering with
the assisted service, deployment will not progress. The `agent
wait-for install-complete` command will recognize the error condition and report
it. After all expected agents have completed validation, the `agent
wait-for install-complete` command will exit with an error code.

Customers integrating directly with the assisted service REST API are
responsible for recognizing this error state themselves and handling
it accordingly.

Logs for the agents that have failed validation will be collected
automatically and downloaded by the `agent wait-for install-complete` command.

### User Stories

- As an OpenShift infrastructure owner, I need to be able to integrate
  the installation of my first on-premises OpenShift cluster with my
  automation flows and tools.
- As an OpenShift infrastructure owner, I must be able to provide the
  definition of the cluster I want to deploy as input to the tool.
- As an OpenShift Infrastructure owner, I must be able to retrieve the
  validation errors from an in-progress or failed deployment in a
  programmatic way.
- As an OpenShift Infrastructure owner, I must be able to get the
  events and progress of the installation in a programmatic way.
- As an OpenShift Infrastructure owner, I must be able to retrieve the
  kubeconfig and OpenShift Console URL for my new cluster in a
  programmatic way.

### API Extensions

This enhancement describes changes to the installer, but will not
introduce any in-cluster API changes via CRDs, webhooks, or aggregated
API servers.

### Implementation Details/Notes/Constraints [optional]

We already have several ways to describe OpenShift clusters for our
different installation workflows, and want to avoid inventing a new
one. Therefore we will reuse existing inputs relevant to on-premises
deployment, borrowing from the IPI installer and other tools such as
the zero-touch provisioning (ZTP) system in multi-cluster engine
(MCE).

Users often do not want to store the credentials for the baseboard
management controllers (BMC) of their hosts inside of the cluster they
are deploying. Therefore we cannot rely on them to give us those
credentials during installation, and we must support tools outside of
OpenShift managing the hosts' boot process. This places more of the
burden of designing the end-to-end automated workflow on users or our
solution architects.

### Risks and Mitigations

The inputs to building an on-premises cluster, especially the per-host
static network settings, can be complex. Manually creating the inputs
to the installer provides many opportunities to introduce errors. For
the automated use case, we assume that users are likely to mitigate
this risk by also automating production of the input files using
templates or file generators.

We are investigating the options for including basic static validation
of the network settings in the installer. Doing so requires access
golang bindings to a Rust library for nmstate, which may make it
problematic to distribute a single build artifact. Handling the build
complications and actual validation is left to a future enhancement.

Other mitigation approaches, such as a GUI for constructing the files,
are left to future enhancements.

The NMStateConfig API is part of the ZTP layer, and could be changed
independently of the installer, potentially breaking compatibility
between the 2 tools. The risk of this happening is low, because the
API is a relatively simple wrapper for nmstate config content that is
not parsed directly by ZTP. If the API is changed, the new version
will need to be backwards-compatible for ZTP users, which should allow
us to bring the installer up to date without significant impact.

## Design Details

### Open Questions [optional]

1. What do we do with the installation logs?

   - Write them to disk on each host in a way that allows someone
     logging in to find them or allows a Pod to mount the directory
     they are in and `cat` them to be collected via the OpenShift API.
   - Expect the orchestration tool to download them before allowing
     deployment to continue.
   - Can we take advantage of or replicate the installer's [bootstrap
     service
     records](https://github.com/openshift/installer/blob/6d778f911e79afad8ba2ff4301eda5b5cf4d8e9e/docs/dev/bootstrap_services.md)
     behavior?

2. Is the workflow different for single-node? Do we need to start the
   service and go through the orchestration steps for one node? If we
   don't, how do we handle the validations?

### Test Plan

We will implement CI jobs to verify deployment of clusters in all
three topologies (single-node, compact, and HA) using VMs simulating
bare metal hosts. We will configure the jobs so that they run for
changes in the dev-scripts, installer, assisted-service, and
assisted-agent repositories to ensure that regressions are not
introduced.

We will implement end-to-end CI jobs to exercise the
parallel/serial/upgrade suites running periodically as release
informers/blockers to ensure that clusters created by the agent-based
installer meet the expectations of all OpenShift clusters.

We will use unit and functional tests on the installer repository to
test the image generator using different types of inputs.

### Graduation Criteria

We will use a fork or branch of the installer to prototype this work,
and then merge it into the installer repository protected by a feature
flag. The minimum viable version of the new feature will be exposed as
tech preview (deployment failures may not receive immediate attention,
but clusters successfully deployed using the tool will be fully
supported).

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- The tech preview version of the new installer may only work in
  connected settings
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- Must support deploying in fully disconnected settings
- Automated end-to-end tests
- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Does not apply for the installer.

### Version Skew Strategy

Does not apply for the installer.

### Operational Aspects of API Extensions

No API changes.

#### Failure Modes

No API changes.

#### Support Procedures

No API changes.

## Implementation History

A prototype of the command to create bootable images is available in
the `fleeting` repository:
https://github.com/openshift-agent-team/fleeting

## Drawbacks

N/A

## Alternatives

The image generator could produce one image per host so that the
settings inside each image are less complex. This approach was
rejected because it introduces more complexity for the user to manage
the images properly and raises the storage requirements, which can be
especially significant for large clusters.

We could use ZTP API resources in manifests as inputs to the installer
for generating the image, but that would make the inputs for the
installer command line tool different for different sub-commands or
operating modes. It would also mean that a user of the installer might
have to create many separate input files, or structure one
multi-document YAML file correctly. Having separate API resource types
for different parts of the input data makes sense in the kubernetes
API because the controllers that read those APIs may be managing many
different clusters. This tool will only be creating one cluster at a
time.

We could use the
[siteconfig](https://github.com/openshift-kni/cnf-features-deploy/tree/master/ztp/siteconfig-generator)
file format as input to the installer for generating the image, but in
addition to making the inputs for the installer command line tool
different for different sub-commands, that file format is a complex
API owned by the ZTP layer above OpenShift, so having the installer in
the base layer depend on it would make managing versions and changes
awkward.

## Infrastructure Needed [optional]

Test resources for running VMs to simulate bare metal (i.e., not
simple cloud instances) are needed. We should be able to use the same
resources that the bare metal IPI tests use.

We will add builds of images for at least assisted-service and
assisted-agent to CI promotion and the OpenShift release payload. We
may need to include the image for assisted-ui as well.
