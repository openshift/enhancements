---
title: bring-your-own-windows-host
authors:
  - "@aravindhp"
reviewers:
  - "@@openshift/openshift-team-windows-containers"
approvers:
  - "@sdodson"
creation-date: 2021-01-19
last-updated: 2021-01-19
status: implementable
---

# Bring Your Own Windows Host

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The intent of this enhancement is to allow customers to add their existing
Windows instances to an OpenShift 4.7+ cluster using the Windows Machine Config
Operator (WMCO).

## Motivation

The world of Windows sees customers treating their instances more as "pets"
rather than "cattle". There is a desire from customers to be able to reuse these
"pet" Windows instances as OpenShift worker nodes, run Windows workloads and gain
similar benefits that their Linux workloads get when being managed by OpenShift.

### Goals

* Allow a customer to add an existing Windows instance running a variant of
  Windows Server 2019 (1809, 1909, 20H2 etc). The instance should have the
  Docker runtime and SSH services configured. The public key used by WMCO for
  the [IPI workflow](https://github.com/openshift/enhancements/blob/master/enhancements/windows-containers/windows-machine-config-operator.md#design-details)
  needs to be an authorized key in the instance.  Please note that these
  instances have to be connected to the same network that other Linux worker
  nodes in the cluster are connected to. These instance also have be present
  in the same cloud provider that the cluster is brought up in.
* WMCO will then perform all the required steps within the VM for it to be
  added to the cluster as an OpenShift worker node and removed from it.
* [Handle upgrades](https://github.com/openshift/enhancements/blob/master/enhancements/windows-containers/windows-machine-config-operator-upgrades.md)
  of all components installed by WMCO.

### Non-Goals

* Installing the container runtime in the Windows instance
* Managing the Windows operating system(OS) including handling of OS updates
* OpenShift Builds
* OpenShift DeploymentConfigs
* Supporting remote Windows instances connected to a different network from
  the cluster's Linux worker nodes

## Proposal

The cluster admin will create a ConfigMap called _windows-instances_ in the
_openshift-windows-machine-config-operator_ namespace. WMCO will be configured
to watch for this special ConfigMap, whose data section will have the following
layout:
```yaml
kind: ConfigMap
apiVersion: v1
metadata:
  name: windows-instances
  namespace: openshift-windows-machine-config-operator
data:
  10.1.42.1: |-
    username=Administrator
  instance.dns.com: |-
    username=core
```
WMCO will configure or de-configure instances based on the contents of the data
section and will use the [cloud-private-key]((https://github.com/openshift/enhancements/blob/master/enhancements/windows-containers/windows-machine-config-operator.md#design-details)
to communicate with them. The key value above can either be the IP address of
the instance or it's DNS name. The IP or DNS entries act as unique keys. If a
specified IP and DNS entry resolve to the same instance then it will get
prepped twice but will result in only one Windows worker.

### User Stories

#### Story 1
As an OpenShift cluster administrator I would like to add an existing Windows
instance to the cluster as a worker node.

#### Story 2
As an OpenShift cluster administrator I would like to remove an existing Windows
instance from the cluster as a worker node.

#### Story 3
As an OpenShift cluster administrator I would like to have the Kubernetes
components on Windows nodes upgraded when new Kubernetes versions or patch
versions are released.

### Risks and Mitigations

The same risk and mitigation that apply to Windows nodes added using MachineSets
to an IPI cluster applies here. In addition there is potential for a node to
become unstable if configuration fails and will require administrator
intervention to fix. There is no real mitigation here other that for us to test
this feature thoroughly in CI.

Given that customers would be reusing existing Windows instances, there is
potential for substantial system resources reserved for non-Kubernetes managed
services. However setting the [priority flags](https://issues.redhat.com/browse/WINC-534)
for k8s services will mitigate this issue.

From an implementation stand point there is potential for code complexity given
we are attempting to track the services being used in the code itself. While
this might not be an issue today given the number of services, it could get
magnified as we add to the number of services. The mitigation for this would
be to externally track the services using a Kubernetes object. This however is
beyond the scope of this enhancement and should be tackled holistically across
MachineSet initiated and BYOH nodes in a separate enhancement.

## Design Details

WMCO will include a ConfigMap controller that watches for changes to the
_windows-instances_ ConfigMap and node objects. Any change will cause WMCO to
compare the list of instances with the set of existing Windows nodes. The
comparison will be based on the instance and node's IP addresses. WMCO will
then react in the following manner:
* If a new entry is found i.e. the list of nodes does not contain one that map
  to the instances' IP address, the instance will be configured by WMCO. The
  configuration process will be identical to configuring instances added using
  a MachineSet in an IPI cluster except in addition, WMCO will also have to
  handle CSR approvals. The CSR approval process can be loosely based on the
  [Node Client CSR Approval Workflow](https://github.com/openshift/cluster-machine-approver#node-client-csr-approval-workflow),
  however there will be differences as no Machine object will be present that
  corresponds to the BYOH Windows instance.
* If the list of nodes has IP addresses that do not map to any in the list of
  instances, WMCO will perform an uninstall operation on the node. This will
  involve:
  * Drain and cordon the node
  * Stop and remove:
    * Monitoring and logging services
    * kube-proxy service
    * hybrid-overlay-node service
    * kubelet service
  * Delete the payload binaries
* If the list of nodes has IP addresses that do not map to any in the list of
  instances, and WMCO is unable to access the instance, it will assume that
  the node in question is not managed by it.

The status of these operations will be reflected by events in the
_openshift-windows-machine-config-operator_ namespace and will be associated
with the _windows-instances_ ConfigMap.

### Test Plan

The existing set of WMCO end to end tests will be run against already created
Windows instances. This can be achieved by creating a Windows Machine that does
not have the _machine.openshift.io/os-id: Windows_ label. Once this Machine has
been created the Machine API and cluster provider controllers should be
disabled to prevent them from reacting. Then the created Machine's IP can then
be added to / removed from the _windows-instances_ ConfigMap which will cause
WMCO to react.

### Graduation Criteria

This enhancement will start as GA.

### Upgrade / Downgrade Strategy

The upgrade process for BYOH nodes will be different to that of
[upgrading Windows nodes](https://github.com/openshift/enhancements/blob/master/enhancements/windows-containers/windows-machine-config-operator-upgrades.md)
added using a MachineSet in an IPI cluster. In the MachineSet case, Machines
are deleted in lieu of upgrading where as here the Windows instances will have
to be reconfigured. WMCO will handle the reconfiguration process and will
include:
* Drain and cordon the node
* Stop, copy over new binaries and update:
  * Monitoring and logging services
  * kube-proxy service
  * hybrid-overlay-node service
  * kubelet service
* Delete unused services and binaries if we move away from using any of them
* Uncordon the node

### Version Skew Strategy

WMCO goes through major version increases across OpenShift releases. Each major
version's payload binaries will have the same version as the Linux counterpart.
For example WMCO 1.0's payload contains Kubernetes v1.19 binaries and WMCO 2.0's
payload contains Kubernetes v1.20 binaries and so on. We plan to maintain N-1
kubelet major version parity with the Linux counterpart i.e. a customer can run
WMCO 1.y.z (Windows nodes will have v1.19 binaries) on an OpenShift 4.7 (v1.20)
cluster however if they upgrade to WMCO 2.y.z, the Kubernetes components on the
Windows node will be upgraded to v1.20.

## Implementation History

Initial enhancement created on Jan 19th, 2021.

## Drawbacks

The status of the configuring and de-configuring of the instances is not clearly
reflected given there is no CR associated with each instance. The cluster
administrator has to depend on the events generated by WMCO.

## Alternatives

Instead of using a ConfigMap, we can introduce a WindowsInstance CRD that maps to
each instance a cluster administrator is trying to add or remove from the cluster
as a worker node.

## Infrastructure Needed

We will need CI environments for all supported platforms to test our code.
