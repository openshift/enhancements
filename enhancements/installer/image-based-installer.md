---
title: image-based-installer
authors:
  - "@mresvanis"
  - "@eranco74"
reviewers:
  - "@patrickdillon"
  - "@romfreiman"
approvers:
  - "@zaneb"
api-approvers:
  - None
creation-date: 2024-04-30
last-updated: 2024-04-30
tracking-link:
  - https://issues.redhat.com/browse/MGMT-17600
see-also:
  - "/enhancements/agent-installer/agent-based-installer.md"
replaces: N/A
superseded-by: N/A
---

# Image-based Installer

## Summary

The Image-based Installer is an installation method for on-premise single-node
OpenShift (SNO) clusters, that will use a bootable, installer ISO and a
configuration ISO running on the hosts that are to become SNO clusters.
The user will generate each ISO using a command-line tool. The first ISO will
contain components (such as the [lifecycle-agent](https://github.com/openshift-kni/lifecycle-agent) operator)
and a [seed image](https://github.com/openshift-kni/lifecycle-agent/blob/main/docs/seed-image-generation.md).
The seed image is an [OCI image](https://github.com/opencontainers/image-spec/blob/main/spec.md)
generated from a SNO system provisioned with the target OpenShift version and is installed onto a target SNO
as a new [ostree stateroot](https://ostreedev.github.io/ostree/deployment/#stateroot-aka-osname-group-of-deployments-that-share-var).
The latter includes, among other files, the `/var`, `/etc` (with specific
exclusions) and `/ostree/repo` directories, which contain the target OpenShift
version and most of its configuration, amounting approximately to just over 1GB
in size. The second ISO will contain the site specific configuration data (e.g.
the cluster name, domain and crypto objects), which need to be set up per cluster
and are derived mainly from the OpenShift installer [install config](https://github.com/openshift/installer/tree/release-4.15/pkg/asset/installconfig).

## Motivation

The primary motivation for relocatable SNO is the fast deployment of single-node OpenShift.
Telecommunications providers continue to deploy OpenShift at the Far Edge. The
acceleration of this adoption and the nature of existing Telecommunication
infrastructure and processes drive the need to improve OpenShift provisioning
speed at the Far Edge site and the simplicity of preparation and deployment of
Far Edge clusters, at scale.

The Image-based Installer provides users with such speed and simplicity, but it
currently needs the [multicluster engine](https://docs.openshift.com/container-platform/4.15/architecture/mce-overview-ocp.html)
and/or the [Image-based Install operator](https://github.com/openshift/image-based-install-operator)
to generate the required installation and configuration artifacts. We would like
to enable users to generate the latter intuitively and independently, using
their own automation or even manual intervention to boot the host.

### User Stories

- As a user in a disconnected environment with no existing management cluster,
  I want to deploy a single-node OpenShift cluster using a [seed image](https://github.com/openshift-kni/lifecycle-agent/blob/main/docs/seed-image-generation.md)
  and my own automation for provisioning.

### Goals

- Install clusters with single-node topology.
- Install clusters in fully disconnected environments.
- Perform reproducible cluster builds from configuration artifacts.
- Require no machines in the cluster environment other than the one to be the
  single node of the cluster.
- Be agnostic to the tools used to provision machines, so that users can
  leverage their own tooling and provisioning.

### Non-Goals

- Replace any other OpenShift installation method in any capacity.
- Generate image formats other than ISO.
- Automate booting of the ISO image on the machines.
- Support installation configurations for cloud-based platforms.

## Proposal

A command-line tool will enable users to build a single custom RHCOS seed image
in ISO format, containing the components needed to provision multiple
single-node OpenShift clusters from that single ISO and multiple site
configuration ISO images, one per cluster to be installed.

The command-line tool will download the base RHCOS ISO, create an [Ignition](https://coreos.github.io/ignition/)
file with generic configuration data (i.e. configuration that is going to be
included in all clusters to be installed with that ISO) and generate an
image-based installation ISO. The Ignition file will configure the live ISO such
that once the machine is booted with the latter, it will install RHCOS to the
installation disk, mount the installation disk, restore the single-node
OpenShift from the [seed image](https://github.com/openshift-kni/lifecycle-agent/blob/main/docs/seed-image-generation.md)
and optionally precache all release container images under the
`/var/lib/containers` directory.

The installation ISO approach is very similar to what is already implemented by
the functionality of the [Agent-based Installer](/enhancements/agent-installer-agent-based-installer.md))
Although the Image-based Installer proposed here differs from the OpenShift
Agent-based Installer in several key aspects:

- while the Agent-based Installer may offer flexibility and versatility in certain scenarios,
  it may not meet the stringent time constraints and requirements of far-edge deployments
  in the telecommunications industry due to the inherently long installation process, 
  exacerbated by low bandwidth and high packet latency.
- with the Agent-based Installer all cluster configuration needs to be provided upfront
  during the generation of the ISO image, while with the Image-based Installer the cluster 
  configuration is provided in an additional step.

The Image-based Installer offers key advantages, where fast and reliable
deployment at the edge is crucial. By generating ISO images containing all
the necessary components, the Image-based Installer significantly accelerates
deployment times. Moreover, unlike the Agent-based Installer, the image-based
approach allows for cluster configuration to be supplied upon deployment at the
edge, rather than during the initial ISO generation process. This flexibility
enables operators to use a single generic image for installing multiple
clusters, streamlining the deployment process and reducing the need for multiple
customized ISO images.

The OpenShift installer will support generating a configuration ISO with all the
site specific configuration data for the cluster to be installed provided as
input. The configuration ISO contents are the following:
* ClusterInfo (cluster name, base domain, hostname, nodeIP)
* SSH authorized_keys
* Pull Secret
* Extra Manifests
* Generated keys and certs (compatible the generated admin kubeconfig)
* Static networking config

The site specific configuration data will be generated according to information
provided in the `install-config.yaml` and the manifests provided in the
installation directory as input. To complete the installation at the edge site:
- the cluster configuration for the edge location can be delivered by copying
  the config ISO content onto the node and placing it under `/opt/openshift/cluster-configuration/`.
- the cluster configuration can also be delivered using an attached ISO, a
  systemd service running on the host pre-installed Image-based Installer will
  mount that ISO (identified by a known label) and copy the cluster configuration
  to `/opt/openshift/cluster-configuration/`.
- the cluster configuration data on the disk will be used to configure the
  cluster and allow OCP to start successfully.

### Workflow Description

TBD

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

The Image-based Installer targets single-node OpenShift deployments.

### Implementation Details/Notes/Constraints

Since we must allow users to provision hosts themselves, either manually or
using automated tooling of their choice, the ISO format offers the widest range
of compatibility. Building a single ISO to boot multiple hosts makes it
considerably easier for the user to manage. The additional site configuration
ISO is necessary for configuring each cluster securely and independently.

The user, before running the Image-based Installer, must generate a [seed image](https://github.com/openshift-kni/lifecycle-agent/blob/main/docs/seed-image-generation.md)
via the [Lifecycle Agent SeedGenerator Custom Resouce (CR)](https://github.com/openshift-kni/lifecycle-agent/blob/main/docs/seed-image-generation.md).
The prerequisites to generating a seed image are the following:

- an already provisioned single-node OpenShift cluster (seed SNO).
   - The CPU topology of that host must align with the target host(s), i.e. they
     should have the same number of cores.
- the [Lifecycle Agent](https://github.com/openshift-kni/lifecycle-agent/tree/main)
  operator must be installed on the seed SNO.

### Risks and Mitigations

N/A

### Drawbacks

N/A

## Open Questions [optional]

- Should the command-line tool that generates the installation ISO be a subcommand
  of the OpenShift installer, or a standalone binary?

  Having the functionality provided by the command-line tool in the OpenShift
  installer would be a natural addition to the latter, as the former refers to
  the provisioning of single-node OpenShift clusters and generates the
  required installation artifacts in the same way as the
  [Agent-based Installer](/enhancements/agent-installer/agent-based-installer.md).

- Should the command-line tool that generates the configuration ISO be a subcommand
  of the OpenShift installer, or a standalone binary?

  Having the functionality provided by the command-line tool in the OpenShift
  installer would be a natural addition to the latter, as the former refers to
  the provisioning of single-node OpenShift clusters and consumes the OpenShift
  installer `install-config.yaml`. In addition, it generates the required
  installation artifacts in the same way as the
  [Agent-based Installer](/enhancements/agent-installer/agent-based-installer.md).

## Test Plan

The Image-based Installer will be covered by end-to-end testing using virtual
machines (in a baremetal configuration), automated by some variation on the
metal platform [dev-scripts](https://github.com/openshift-metal3/dev-scripts/#readme).
This is similar to the testing of the Agent-based Installer, the baremetal IPI
and assisted installation flows.

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

TBD

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

TBD

## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A

## Alternatives

### Downloading the installation ISO from Assisted Installer SaaS

The Assisted Installer SaaS could be used to generate and serve the image-based
installation ISO, which would potentially enhance the user experience, compared
to configuring and executing a command-line tool. In addition, even in a fully
disconnected environment, the user could generate the ISO via the Assisted
Installer service and then carry it over to the disconnected environment.

However, for the Assisted Installer service this would be a completely new
flow, as the image-based installation ISO is not generated per cluster, but for
multiple SNO clusters with similar underlying hardware (due to the requirements
of the Image-based Installer flow) and its configuration input is different than
what is currently supported.

In addition, a local command-line tool used in a disconnected environment will
be able to connect to the registry serving the seed OCI image and verify
it, whereas the Assisted Installer service cannot support this.

### Building the installation ISO in the Lifecycle Agent Operator

The Lifecycle Agent Operator could be used to generate and serve the image-based
installation ISO, as this is the component used to generate the seed OCI
image, which is the basis of the Image-based Installer flow.

However, the following reasons constitute a separate command-line tool a better
fit:

- the seed OCI image is not only used for the Image-based Installer, but
  also for the Image-based Upgrade flow, supported by different user personas.
- the same seed OCI image can be used to generate multiple image-based
  installation ISOs (e.g. with different installation disks).
- the user persona to generate the image-based installation ISO must have a
  running OpenShift cluster, generate the ISO via the Lifecycle Agent
  Operator, download the ISO and move it around. Howerver, given an already
  generated seed OCI image, which can be generated earlier by another user
  persona, we can facilitate this user persona by not requiring a running
  OpenShift cluster and the Lifecycle Agent Operator for the installation ISO
  generation.
- having 2 (installation ISO and configuration ISO) out of 3 (the 3rd is the
  seed OCI image used by both the Image-based Upgrade and the Image-based
  Installer) Image-based Installer artifacts generated by the same command-line
  tool simplifies the user experience.

## Infrastructure Needed [optional]

N/A
