---
title: connected-assisted-installer
authors:
  - "@avishayt"
  - "@hardys"
  - "@dhellmann"
reviewers:
  - "@beekhof"
  - "@deads2k"
  - "@hexfusion"
  - "@mhrivnak"
approvers:
  - "@crawford"
  - "@abhinavdahiya"
  - "@eparis"
creation-date: 2020-06-09
last-updated: 2020-06-10
status: implementable
see-also:
  - "/enhancements/baremetal/minimise-baremetal-footprint.md"
---

# Assisted Installer for Connected Environments

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes changes in and around the installer to
assist with deployment on user-provisioned infrastructure. The use
cases are primarily relevant for bare metal, but in the future may be
applicable to cloud users who are running an installer UI directly
instead of using a front-end such as `cloud.redhat.com` or the UI
provided by their cloud vendor.

## Motivation

The target user is someone wanting to deploy OpenShift, especially on
bare metal, with as little up-front infrastructure dependencies as
possible. This person has access to server hardware and wants to run
workloads quickly. They do not necessarily have the administrative
privileges to create private VLANs, configure DHCP/PXE servers, or
manage other aspects of the infrastructure surrounding the hardware
where the cluster will run. If they do have the required privileges,
they may not want to delegate them to the OpenShift installer for an
installer-provisioned infrastructure installation, preferring instead
to use their existing tools and processes for some or all of that
configuration. They are willing to accept that the cluster they build
may not have all of the infrastructure automation features
immediately, but that by taking additional steps they will be able to
add those features later.

### Goals

- Make initial deployment of usable and supportable clusters simpler.
- Move more infrastructure configuration from day 1 to day 2.
- Support connected on-premise deployments.
- Support existing infrastructure automation features, especially for
  day 2 cluster management and scale-out.

### Non-Goals

- Because the initial focus is on bare metal, this enhancement does
  not exhaustively cover variations needed to offer similar features
  on other platforms (such as changes to image formats, the way a host
  boots, etc.). It is desirable to support those platforms, but that
  work will be described separately.
- Environments with restricted networks where hosts cannot reach the internet unimpeded
  ("disconnected" or "air-gapped") will require more work to support
  this installation workflow than simply packaging the hosted solution
  built to support fully connected environments. The work to support
  disconnected environments will be covered by a future enhancement.
- Replace the existing OpenShift installer.
- Describe how these workflows would work for multi-cluster
  deployments managed with Hive or ACM.

## Proposal

There are several separate changes to enable the assisted installer
workflows, including a GUI front-end for the installer, a cloud-based
orchestration service, and changes to the installer and bootstrapping
process.

The process starts when the user goes to an "assisted installer"
application running on `cloud.redhat.com`, enters details needed by
the installer (OpenShift version, ssh keys, proxy settings, etc.), and
then downloads a live RHCOS ISO image with the software and settings
they need to complete the installation locally.

The user then boots the live ISO on each host they want to be part of
the cluster (control plane and workers). They can do this by hand
using thumb drives, by attaching the ISO using virtual media support
in the BMC of the host, or any other way they choose.

When the ISO boots, it starts an agent that communicates with the REST
API for the assisted installer service running on `cloud.redhat.com`
to receive instructions. The agent registers the host with the
service, using the user's pull secret embedded in the ISO's Ignition config
to authenticate. The agent identifies itself based on the serial
number from the host it is running on. Communication always flows from
agent to service via HTTPS so that firewalls and proxies work as
expected.

Each host agent periodically asks the service what tasks to perform,
and the service replies with a list of commands and arguments. A
command can be to:

1. Return hardware information for its host
2. Return L2 and L3 connectivity information between its host and the
   other hosts (the IPs and MAC addresses of the other hosts are
   passed as arguments)
3. Begin the installation of its host (arguments include the host's
   role, boot device, etc.). The agent executes different installation
   logic depending on its role (bootstrap-master, master, or worker).

The agent posts the results for the command back to the
service. During the actual installation, the agents post progress.

As agents report to the assisted installer, their hosts appear in the
UI and the user is given an opportunity to examine the hardware
details reported and to set the role and cluster of each host.

The assisted installer orchestrates a set of validations on all
hosts. It ensures there is full L2 and L3 connectivity between all of
the hosts, that the hosts all meet minimum hardware requirements, and
that the API and ingress VIPs are on the same machine network.

The discovered hardware and networking details are combined with the
results of the validation to derive defaults for the machine network
CIDR, the API VIP, and other network configuration settings for the
hosts.

When enough hosts are configured, the assisted installer application
replies to the agent on each host with the instructions it needs to
take part in forming the cluster. The assisted installer application
selects one host to run the bootstrap services used during
installation, and the other hosts are told to write an RHCOS image to
disk and set up ignition to fetch configuration from the
machine-config-operator in the usual way.

During installation, progress and error information is reported to the
assisted installer application on `cloud.redhat.com` so it can be
shown in the UI.

### Integration with Existing Bare Metal Infrastructure Management Tools

Clusters built using the assisted installer workflow use the same
"baremetal" platform setting as clusters built with
installer-provisioned infrastructure. The cluster runs metal3, without
PXE booting support.

BareMetalHosts created by the assisted installer workflow do not have
BMC credentials set. This means that power-based fencing is not
available for the associated nodes until the user provides the BMC
details.

### User Stories

#### Story 1

As a cluster deployer, I want to install OpenShift on a small set of
hosts without having to make configuration changes to my network or
obtain administrator access to infrastructure so I can experiment
before committing to a full production-quality setup.

#### Story 2

As a cluster deployer, I want to install OpenShift on a large number
of hosts using my existing provisioning tools to automate launching
the installer so I can adapt my existing admin processes and
infrastructure tools instead of replacing them.

#### Story 3

As a cluster deployer, I want to install a production-ready OpenShift
cluster without committing to delegating all infrastructure control to
the installer or to the cluster, so I can adapt my existing admin
processes and infrastructure management tools instead of replacing
them.

#### Story 4

As a cluster hardware administrator, I want to enable power control
for the hosts that make up my running cluster so I can use features
like fencing and failure remediation.

### Implementation Details/Notes/Constraints

Much of the work described by this enhancement already exists as a
proof-of-concept implementation. Some aspects will need to change as
part of moving from PoC to product. At the very least, the code will
need to be moved into more a suitable GitHub org.

The agent discussed in this design is different from the
`ironic-python-agent` used by Ironic in the current
installer-provisioned infrastructure implementation.

### Risks and Mitigations

The current implementation relies on
[minimise-baremetal-footprint](https://github.com/openshift/enhancements/pull/361). If
that approach cannot be supported, users can proceed by providing an
extra host (4 hosts to build a 3 node cluster, 6 hosts to build a 5
node cluster, etc.).

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

This work is all about building clusters on day 1. After the cluster
is running, it should be possible to upgrade or downgrade it like any
other cluster.

### Version Skew Strategy

The assisted installer and agent need to know enough about the
installer version to construct its inputs correctly. This is a
development-time skew, for the most part, and the service that builds
the live ISOs with the assisted installer components should be able to
adjust the version of the assisted installer to match the version of
OpenShift, if necessary.

## Implementation History

### Proof of Concept (June, 2020)

* https://github.com/filanov/bm-inventory : The REST service
* https://github.com/ori-amizur/introspector : Gathers hardware and
  connectivity info on a host
* https://github.com/oshercc/coreos_installation_iso : Creates the
  RHCOS ISO - runs as a k8s job by bm-inventory
* https://github.com/oshercc/ignition-manifests-and-kubeconfig-generate :
  Script that generates ignition manifests and kubeconfig - runs as a
  k8s job by bm-inventory
* https://github.com/tsorya/test-infra : Called by
  openshift-metal3/dev-scripts to end up with a cluster of VMs like in
  dev-scripts, but using the assisted installer
* https://github.com/eranco74/assisted-installer.git : The actual
  installer code that runs on the hosts

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

The telco/edge bare metal team is working on support for automating
virtual media and dropping the need for a separate provisioning
network. Using the results will still require the user to understand
how to tell the installer the BMC type and credentials and to ensure
each host has an IP provided by an outside DHCP server. Hardware
support for automating virtual media is not consistent between
vendors.

## Infrastructure Needed [optional]

The existing code (see "Proof of Concept" above) will need to be moved
into an official GitHub organization.
