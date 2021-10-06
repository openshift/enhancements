---
title: windows-machine-config-operator
authors:
  - "@ravisantoshgudimetla"
  - "@aravindhp"
reviewers:
  - "@sdodson"
  - "@derekwaynecarr"
  - "@openshift/openshift-team-cloud"
  - "@openshift/openshift-team-mco"
approvers:
  - "@sdodson"
  - "@derekwaynecarr"
creation-date: 2020-05-28
last-updated: 2020-05-28
status: implementable
replaces:
  - "/enhancements/windows-containers/ansible-dev-preview.md"
---

# Windows Machine Config Operator

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The intent of this enhancement is to allow a cluster administrator to add a
Windows compute node as a day 2 operation with a prescribed configuration to an
installer provisioned OpenShift 4.6 cluster and enable scheduling of Windows
workloads. The targeted platforms for 4.6 are vSphere, Azure and AWS. The
cluster has to be configured with Hybrid OVN networking.

## Motivation

The main motivation behind this enhancement is to satisfy customer
requirement of being able to run Windows workloads on OpenShift clusters.

### Goals

As part of this enhancement we plan to do the following:
* Provide workflows for adding and removing Windows instances provisioned using
  the Machine API to OpenShift clusters
* Perform all the required steps within the VM for it to be added to the cluster
  as an OpenShift worker node and removed from it.

### Non-Goals

As part of this enhancement we do not plan to support:
* Installing the container runtime in the Windows node
* Upgrades (This will be addressed in a separate enhancement)
  * Windows operating system upgrades
  * Node management like k8s compute component upgrades and reboots
* OpenShift Builds
* OpenShift DeploymentConfigs

## Proposal

The two main pieces that will allow us to achieve our goals is the Windows
Machine Config operator (WMCO) and a Windows Machine Config Bootstrapper (WMCB)
binary. WMCO will collect information from the cluster and transfer it to the
Windows node. The WMCB will then perform the necessary steps on the node for it
to join the cluster. [WMCB](https://github.com/openshift/windows-machine-config-bootstrapper)
has already been developed as part of the Windows Containers
[dev preview](https://github.com/openshift/windows-machine-config-bootstrapper/blob/master/tools/ansible/docs/ocp-4-4-with-windows-server.md).

### Justification

The reason we are using a WMCB binary rather than a container image is
that all Windows container images are packaged with a Windows kernel and Red Hat
has a policy to not ship 3rd party kernels for support reasons.

### Design Details

The WMCO image will be packaged with the following binaries:
* Kubelet
* CNI plugins
* Hybrid-overlay
* WMCB
* Kube-proxy

WMCO will be published to OperatorHub as a Red Hat operator. The way the cluster
admin will enable Windows workloads on their cluster would be to first install
WMCO. WMCO expects the cluster admin to create a predetermined Secret in its
namespace containing a private key that will be used to interact with the
Windows instance. WMCO will check for this Secret during boot up time and create
a user data Secret which the cluster admin can reference in the Windows
MachineSet that they create as described below. WMCO will populate the user data
secret with a public key that corresponds to the private key. In addition it
will have all the pre-requisite steps needed for setting the VM up for key based
SSH connections.

When the cluster admin wants to bring up Windows worker nodes, they will create
a [MachineSet](https://github.com/openshift/machine-api-operator/blob/master/config/machineset.yaml)
where they reference a Windows OS image which has the Docker container runtime
add-on enabled. They will also specify the user data secret created by WMCO as
described above. WMCO will watch for Machine objects with a special label that
indicate that they map to Windows VMs. Once the Machine object is in the
["provisioned" phase](https://github.com/openshift/enhancements/blob/master/enhancements/machine-api/machine-instance-lifecycle.md),
WMCO will connect to the associated VM to transfer the binaries and perform the
following steps for it to join the cluster as a worker node:
* Transfer the required binaries
* Configure the kubelet service using WMCB
* Run the hybrid-overlay-node.exe binary
* Configure the kubelet service for CNI using WMCB
* Configure kube-proxy

When the cluster admin wants to scale up or down the number of Windows workers,
they will edit the (replicas) https://github.com/openshift/machine-api-operator/blob/master/config/machineset.yaml#L11
field in the Windows Machine Set spec.

### User Stories

The user stories are collected using the links below:
* [Windows Containers Productization Epic](https://issues.redhat.com/browse/WINC-182)
* [Machine API migration](https://issues.redhat.com/issues/?filter=12347681)
* [Filter out "worker" windows node from being tagged by the MCC](https://issues.redhat.com/browse/GRPA-2213)

### Risks and Mitigations

* The main risk is that OVN Hybrid networking will not GA in 4.6. In that
  scenario a third-party networking solution that has been validated for all the
  targeted platforms will need to be explored.
* Linux workloads that have blanket tolerations has the potential to land on a
  Windows node and fail. The mitigation for this is to document this clearly in
  the official Windows Containers documentations. Other OpenShift operator
  developers should also ensure that they are not using blanket toleration.

### Test Plan

The WMCO repository has already been integrated with Prow CI. We are running
e2e tests for every PR that is opened. These e2e tests involve bringing up a
cluster on all supported cloud providers, instantiating Windows nodes and
running the following tests:
* Basic node validation
* East west networking between Linux and Windows workloads
* North south networking

### Graduation Criteria

This enhancement will start as GA.

### Upgrade / Downgrade Strategy

This is an involved topic and we will be opening a separate enhancement
detailing it.

### Version Skew Strategy

We plan to maintain kubelet major version parity with the Linux counterpart.


## Implementation History

We already have a working [prototype](https://github.com/openshift/windows-machine-config-operator)
in place where use a custom CRD and the Windows Node Installer library to enable Windows workloads.

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

Instead of using the MachineSet as the point of entry to enable Windows
workloads, we can introduce a Windows specific CRD to do the same as we have
done in the prototype.

## Infrastructure Needed

We will need vSphere development and CI environments to test our code in
addition to the AWS and Azure which we already have.
