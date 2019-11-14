---
title: windows-containers-productization
authors:
  - "@ravisantoshgudimetla"
  - "@aravindhp"
reviewers:
  - "@crawford"
  - "@sdodson"
approvers:
  - "@crawford"
  - "@sdodson"
creation-date: 2019-08-30
last-updated: 2019-09-03
status: implementable
---

# Windows Containers Productization

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The intent of this enhancement is to allow a cluster administrator to add a
Windows compute node with a prescribed configuration to an OpenShift cluster as a
day 2 operation and enable scheduling of Windows workloads.

## Motivation

The main motivation behind this enhancement is to satisfy customer
requirement of being able to run Windows workloads on OpenShift clusters.

### Goals

As part of this enhancement we plan to do the following:
* Provide workflows for installing and upgrading OpenShift compute components
(kubelet, OVN, and the Windows Machine Config Bootstrapper) on user-provided
Windows machines
* Perform all the required steps within the node for it to be an OpenShift
compute node
* Administrator initiated upgrade of all OpenShift related components
(kubelet, OVN and Windows Machine Config Bootstrapper) in the node

### Non-Goals

As part of this enhancement we do not plan to support:
* Windows node provisioning or de-provisioning
* Installing the container runtime in the Windows node
* Windows operating system upgrades
* Node management like reboots and node draining
* OpenShift Builds
* Network configuration
  * The details of this will be worked out as part of the enhancement
    proposal for "OVN Plumbing for Hybrid Linux+Windows Clusters GA"

## Proposal

The two main pieces that will allow us to achieve our goals is a Windows Scale
Up (WSU) Ansible playbook and a Windows Machine Config Bootstrapper (WMCB)
binary. The Ansible playbook will collect information from the cluster and
transfer it to the Windows node. The WMCB will then perform the necessary
steps on the node for it to join the cluster.

### Justification

The reason for having an Ansible playbook and on-node executable split is to
have a consistent user experience with "Bring your own RHEL". It also allows
us to be future proof when we move to an operator workflow. Please read the
[alternatives](#Alternatives) section.

The reason we are using a binary rather than a container image is
that all Windows container images contains a Windows kernel and Red Hat has a
policy to not ship 3rd party kernels for support reasons.

### Implementation Details

#### Windows Scale Up (WSU)

The Windows Scale Up is an Ansible playbook that has the follow prerequisites:
* Needs to run on a Linux system
* Needs to be able to access the cluster where the Windows compute will be added
* Needs to be able to access the Windows node

The inputs that the WSU playbook requires are:
* Windows node credentials
* Kubelet download location
* Worker Ignition
* WMCB download location

The actions that the WSU will perform are:
* Check if the container runtime is present on the Windows node
* Download and copy the kubelet to the Windows node
* Extract the worker Ignition from the cluster and copy it to the Windows node
* Download the WMCB, copy it to the Windows node and execute it

#### Windows Machine Config Bootstrapper (WMCB)

The Windows Machine Config Bootstrapper executable has the following
prerequisites:
* Only supports Windows x86-64 nodes (Windows Server 2019)
* Can only run on Windows nodes

The inputs that the WMCB requires are:
* kubelet location on the local disk
* Worker Ignition location on the local disk

The actions that the WMCB will perform are:
* Install / upgrade and configure the kubelet
* Parse the worker Ignition and extract the bootstrap kubeconfig and the
kubelet configuration
  * We are not using the Ignition file during the node booting stage
* Launch the kubelet as a Windows service
* Check if the kubelet is running
* Exit

### Risks and Mitigations

The main risk with this proposal is the dependency on Microsoft to publish a
downstream version of the kubelet. If this does not happen we would have to
use the upstream version of the kubelet which will result in Red Hat being
responsible for security and other fixes.

The other risk is "OVN Plumbing for Hybrid Linux+Windows Clusters GA" not being
delivered in time for integration. The mitigation for that would be to use 3rd
party networking components but that might have support implications that would
need to be resolved.

## Design Details

### Test Plan

We plan to have all the repositories associated with this effort fully
integrated with Prow CI and run e2e tests for every PR that is opened. These
e2e tests will involve bringing up a cluster on all supported cloud providers,
instantiating a Windows node and running workloads on it. We also plan to add
blocking tests to the nightly runs. We do not plan to use CI to test upgrade and
downgrade workflows in the 4.3 timeframe.

### Graduation Criteria

This enhancement will start as GA

### Upgrade / Downgrade Strategy

We will support upgrades of the node components by publishing a new release
of the WSU Ansible playbook. An older release of the playbook can be used to
downgrade.

### Version Skew Strategy

We plan to maintain kubelet major version parity with the Linux counterpart. In
the case of a major version change, the user will have to manually upgrade the
Windows compute node using the Ansible playbook.

## Implementation History

v1: Initial proposal

## Drawbacks

The Microsoft container ecosystem is not fully mature or on par with Linux
containers. For one we can only support Windows process containers that
restricts workloads that are runnable on Windows Server 2019 as there is a tight
kernel version coupling with Windows containers. The coupling is slightly
loosened if we use Hyper-V containers but that is not a viable option yet with
all cloud providers as it requires nested virtualization support. Given these
limitations customers could potentially get a degraded experience with Windows
workloads when compared to Linux workloads on the cluster.

## Alternatives

An alternate approach to this design is to follow the operator approach where in
the user will first install an operator from OperatorHub. The operator will
introduce a CRD that will take similar inputs that the Ansible playbook does.
It will then ensure that the WMCB is installed and launched on the cluster. The
WMCB could also potentially become Windows Machine Config Daemon (WMCD) that
supports a run once option. While it is a daemon it would run as a Windows
service constantly reconciling with a config object on the cluster to ensure the
node is in the desired state.

We opted not to go with this approach given the time frame and all the unknowns
present in this project.

## Infrastructure Needed

We plan to house the WSU Ansible playbook in the openshift/openshift-ansible
repo. We will have a separate repository for WMCB.
