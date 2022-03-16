---
title: agent-based-installer
authors:
  - "@zaneb"
reviewers:
  - "@avishayt"
  - "@celebdor"
  - "@hardys"
  - "@lranjbar"
  - "@mhrivnak"
  - "@patrickdillon"
  - "@pawanpinjarkar"
  - "@romfreiman"
  - "@rwsu"
  - "@sdodson"
approvers:
  - "@dhellmann"
creation-date: 2022-03-03
last-updated: 2022-03-14
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/AGENT-9
---

# Agent-based Installer

## Summary

The agent-based installer is an installation method for on-premises clusters,
that will use a bootable, ephemeral installer image running on the hosts that
are to become part of the cluster. The user will generate the image using a
command-line tool. The image itself will contain components (such as
[assisted-service](https://github.com/openshift/assisted-service/#readme) and
[assisted-agent](https://github.com/openshift/assisted-installer-agent#readme))
of the Assisted and Multi-Cluster Engine (MCE) installation methods. The
installer will be usable in both a fully-automated workflow, where
configuration is provided upfront, and an interactive workflow where the user
can interact with a GUI similar to the hosted Assisted installation service.

## Motivation

Every on-premises user has a first cluster ("Cluster 0"), and many of them will
be installed in a disconnected environment. While powerful tools exist for
creating multiple on-premises clusters (Hive, HyperShift, MCE), they all run in
a cluster themselves, which must somehow be provisioned without these tools.

Each of the current installation options has limitations that make it
unpalatable to a subset of users. IPI is highly opinionated, and suffers from
providing limited visibility of configuration problems in on-premises
environments. UPI exposes users to the full complexity of OpenShift with little
guidance. The (hosted) Assisted service is not available for IPv6-only (until
Quay.io supports IPv6) or disconnected environments. For some users, none of
these options is optimal.

On-premises users need a simple way to deploy their first cluster, even in a
disconnected environment, using their own automation (or even manual
intervention) to boot hosts.

### Goals

- Install clusters in fully-disconnected environments.
- Make use of assisted installation components to pre-validate the installation
  environment.
- Perform reproducible cluster builds from configuration artifacts, as well as
  interactive installation.
- Require no machines in the cluster environment other than those that are to
  become part of the cluster.
- Install clusters with single-node, compact, and high-availability topologies.
- Be agnostic to the tools used to provision (virtual) machines so that users
  can use their own (possibly pre-existing) tooling for provisioning.
- Support install configurations with platform `none`, `baremetal`, or
  `vsphere`.
- Run on either Linux (any distro) or MacOS, in either x86-64 or arm64
  architectures.
- Install to machines with either x86-64 or arm64 architectures, independent of
  the architecture the CLI tool runs on.

### Non-Goals

- Replace the IPI installer in any capacity.
- Automate booting of the image on (virtual) machines.
- Support install configurations for cloud-based platforms.
- Provide an interactive API to drive automated deployments.
- Operate in disconnected environments without the need to separately mirror
  the release payload to a registry that is usable from the installed cluster.
- Provide a mechanism to add and remove nodes from the cluster after it is
  installed.
- Generate image formats other than ISO.
- Create minimal ISO images with a separate rootfs.
- Run on Windows.

## Proposal

A command-line tool will enable users to build a custom RHCOS image, initially
in ISO format, containing the components needed to install.

A prerequisite for disconnected users is that the release image, and possibly
an extra image containing installer components, is mirrored to a local registry
that is accessible from the network environment where the cluster is to be
deployed. The command-line tool will
[retrieve](https://github.com/openshift/machine-os-images/) the base ISO from
the container registry, and add an Ignition file to produce a cluster-specific
install ISO. Users can then boot this image on the hosts that are to become
part of the cluster using any method they choose - this could be existing
automation that they have or manually booting using physical or virtual media.
Container images required for installation will be pulled from the same
registry as the base ISO, as will the OpenShift release that is being
installed. One host (hereafter referred to as "Node 0") will be selected to run
the assisted-service. All hosts will run the assisted agent. A single ISO will
be used for all hosts in the cluster.

In the automated workflow, all configuration data for the cluster to be
installed (including the expected number of hosts) will be provided as input to
the command-line tool at the time the ISO is generated. Users will (optionally)
be able to supply custom manifests to augment the initial configuration, in
addition to data required for installation such as the network config.
Installation will proceed automatically once the requisite hosts have
registered with Node 0 and passed validations. An API running on Node 0 will
enable automated tools to monitor progress, detect errors, and retrieve logs
until the OpenShift control plane is available.

In the interactive flow, the ISO will be generated without collecting
cluster-specific data, or with only enough data to configure networking so that
the hosts will be able to retrieve container images from the registry and
communicate with each other once they are booted. Users will interact with
assisted-service on Node 0 to define and install the cluster. Unlike the hosted
service, each instance of assisted-service will work with only a single
cluster.

### User Stories

- As a user in a disconnected environment with no existing management cluster,
  I want to deploy a cluster using my own automation for provisioning.
- As a user in a disconnected environment with no existing management cluster,
  I want to interactively deploy a cluster and receive immediate feedback about
  configuration errors and problems.
- As a user in a disconnected environment with no existing management cluster,
  I want to deploy a cluster to a vSphere where I have permissions only to set
  the boot image for one or more existing VMs.
- As a user in a disconnected environment with no existing management cluster
  and strict security protocols, I  want to deploy a cluster using only
  physical media and access to the host consoles via a KVM switch.

### API Extensions

N/A

### Implementation Details/Notes/Constraints [optional]

We must allow the user to provision hosts themselves, either manually or using
automated tooling of their choice. The ISO format offers the widest range of
compatibility. If necessary, the user can decompose the ISO into a minimal
version, or into PXE artifacts, using `coreos-installer`.

Building a single ISO to boot all of the hosts makes it considerably easier for
the user to manage. In some PXE environments this may even be compulsory. It
also avoids the need for large amounts of storage.

We must be able to install without access to any compute resource other than
the machines that are to join the cluster. Thus, all run-time activities (that
is, those that must take place in the same network environment as the eventual
cluster) must happen within the ISO image once it is booted.

The location of the release image will be customisable to allow for
disconnected installation. Disconnected and IPv6-only users will be required to
mirror the release image to a local registry that is accessible both from the
environment where the ISO is generated and the environment the cluster will be
deployed in. This could, but need not, be set up using
[oc-mirror](https://github.com/openshift/oc-mirror#readme). The agent-based
installer itself will not attempt to automate this aspect of the deployment.

The components of the agent-based installer itself may be included in the
release payload, or in a separate operator payload that is also mirrored to the
local registry.

No additional components will be added to the installed cluster, other than
those explicitly requested by the user. In particular, the agent-based
installer will **not** automatically add MCE for day 2 scale out of the
cluster.

Further details will be discussed in future enhancements.

### Risks and Mitigations

The agent-based installer adds yet *another* method of installing OpenShift,
and overlaps some other installation methods in applicability. Thus, in some
cases it will contribute to even more decisions the user has to make before
getting started.

No mechanism currently exists for monitoring progress of an assisted
installation in which the assisted-service is stopped before the installation
is complete (as it must in this case, when Node 0 reboots to join the cluster).
It will have to be created. There may be a gap in debug coverage as Node 0 is
installed.

## Design Details

### Open Questions [optional]

- Should the command-line tool be a subcommand of openshift-installer, or a
  standalone binary?
- Will the containers for the assisted components be retrieved from the same
  release payload, or will we require the user to do additional mirroring (e.g.
  of MCE) as well?

### Test Plan

The agent-based installer will be covered by end-to-end testing using virtual
machines (in a baremetal-like configuration), automated by some variation on
the metal platform
[dev-scripts](https://github.com/openshift-metal3/dev-scripts/#readme). This is
similar to the testing employed on both the baremetal IPI and assisted
installation flows.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TBD

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in
`Implementation History`.

## Drawbacks

In disconnected environments, users will still need to mirror the release
payload to an accessible local registry, which must be long-running (i.e. it
continues to be available once the cluster is up).

Since the installation tools cannot continue running to the completion of
building the cluster (Node 0 must eventually be rebooted to join the cluster),
it will necessarily not be possible to use a single API to track the entire
process of installation.

Despite the fact that RHCOS is an operating system designed exclusively to run
OpenShift, users will still need to wait for OpenShift to be *installed* into
it after boot. A total of three boots (which are very slow on baremetal due to
RAM testing at startup) will still be required for each host to come up as an
OpenShift node.

## Alternatives

### Run assisted-service outside of the cluster hosts

Rather than running assisted-service on one of the hosts that is to become part
of the cluster, we could require the user to provide somewhere else to run it
in the same network environment, in either a Podman pod on a RHEL host or in a
virtual machine. This avoids the complexity of running the installation
components on a host that will itself have to be rebooted and come up as a
member of the cluster control plane.

However, there exist environments where the boot image is transported in on
physical media and the user has no access to any other compute capacity within
the same network environment. This architecture would preclude supporting such
sites at any point in the future.

### Expand from a single-node cluster

Instead of doing an 'installation' as such, we could provide a pre-built
Single-Node OpenShift image with the Multi-Cluster Engine (and potentially a
registry with a mirror of the release image) installed. Users could then use
MCE to expand the cluster to the desired size.

However, scaling etcd from 1 to multiple nodes carries a significant risk of
data loss (unlike scaling from 3 to 5 or more nodes). While this is likely
acceptable during installation, it is not at any other time and there are thus
no plans to implement this scenario in the etcd operator.

### Deliver the assisted components separately

Rather than delivering the assisted components as part of the release image, we
could ship them separately and have the ISO pull them either directly from MCE
or from an operator in the OLM catalog. In disconnected environments, this
would require the user to perform an extra step to mirror an additional image
obtained in a different way from the release image.

This would also permit installation of older versions of OpenShift, at least
going back to 4.10 (when the CoreOS ISO was added to the release payload).
However, since the CoreOS ISO is delivered in the release payload, it would
also mean that the agent-based installer would need to be tested end-to-end
against multiple supported versions of CoreOS.

## Infrastructure Needed [optional]

Git repositories exist in the `openshift` org for the assisted components, but
they are currently not delivered as part of the OpenShift product. We will need
to create delivery repos for the relevant containers.

The CLI tool itself may require a new git repo and delivery mechanisms, unless
the functionality is incorporated into the existing `openshift-installer` tool.
