---
title: microshift-sriov-operator
authors:
  - vanhalenar
reviewers:
  - "@pacevedom"
  - "@pmtk"
  - "@ggiguash"
approvers:
  - "@jerpeter1"
api-approvers:
  - None
creation-date: 2025-09-19
last-updated: 2025-09-19
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2329
---

# MicroShift SR-IOV operator integration

## Summary

The following enhancement proposes integrating the OpenShift SR-IOV network
operator into MicroShift in order to enable users to configure Virtual Functions
in a more convenient manner.

## Motivation

Currently, SR-IOV can be used with MicroShift by manually configuring
[VFs](https://docs.kernel.org/PCI/pci-iov-howto.html) (Virtual Functions) on a
[PF](https://docs.kernel.org/PCI/pci-iov-howto.html) (Physical Function) on OS
level and using multus to map the VFs to pods. This approach is tedious and
non-idiomatic. By using the SR-IOV network operator, users can configure VFs in
a declarative way by specifying and deploying a NetworkNodePolicy CRD.

### User Stories

As a MicroShift admin, I want to configure VFs on PFs in a declarative manner.

As a MicroShift admin, I don't want to configure VFs manually on OS level, I
want to use higher level tools.

### Goals

- Enable users to create VFs declaratively, instead of manually configuring
  them, by providing the OpenShift SR-IOV network operator as an optional RPM. 
- Provide a simple smoke test to check that the pods were deployed correctly,
  and that the VFs can be assigned.

### Non-Goals

- Provide a full test of all SR-IOV functions. The assumption is that OpenShift
  testing of SR-IOV covers the MicroShift use case as well.

## Proposal

- Provide SR-IOV network operator for MicroShift as an optional RPM containing
  required manifests that will be applied upon MicroShift starting. 
- Manifests will be derived from the existing manifests for OpenShift SR-IOV
  operator. Possible changes may include optimizations in regards to CPU/memory
  usage.

### Workflow Description

1. User installs `microshift-sriov` RPM and restarts MicroShift service
2. SR-IOV resources are deployed in `sriov-network-operator` namespace
3. User prepares NetworkNodePolicy CRD specifying their desired VF configuration
   based on their available hardware.
4. User deploys said NetworkNodePolicy CRD.
5. The specified configuration of VFs is created.
6. User deploys an SriovNetwork CR, which references the NetworkNodePolicy. The
   operator then generates a NetworkAttachmentDefinition CR and the VFs are
   available to the pods.

### API Extensions

`microshift-sriov` will introduce following CRDs:

- SRIOVNetwork
- OVSNetwork
- SriovNetworkNodeState
- SriovNetworkNodePolicy

Details on these CRDs can be found here: https://github.com/openshift/sriov-network-operator

### Topology Considerations

#### Hypershift / Hosted Control Planes

Enhancement is MicroShift specific.

#### Standalone Clusters

Enhancement is MicroShift specific.

#### Single-node Deployments or MicroShift

Enhancement is MicroShift specific.

### Implementation Details/Notes/Constraints

See "Proposal".

### Risks and Mitigations

The operator, when deployed, creates 6 pods, with the following memory/CPU limits:

| Pod Name                     | CPU Requests  | Memory Requests  |
|------------------------------|---------------|------------------|
| sriov-device-plugin          | 10m           | 50Mi             |
| sriov-network-config-daemon  | 100m          | 100Mi            |
| sriov-network-operator       | 100m          | 100Mi            |
| network-resources-injector   | 10m           | 50Mi             |
| network-webhook              | 10m           | 50Mi             |
| sriov-metrics-exporter       | 10m + 100m    | 20Mi + 100Mi     |

This is significant, and will need to be addressed in the implementation.
Possible solution could be merging some of the pods. Some of the pods (e.g. the
metrics exporter) are optional and could be left out.

When running without the webhook and metrics exporter, the deployment consists
of only 3 pods:


| Pod Name                     | CPU Requests  | Memory Requests  |
|------------------------------|---------------|------------------|
| sriov-device-plugin          | 10m           | 50Mi             |
| sriov-network-config-daemon  | 100m          | 100Mi            |
| sriov-network-operator       | 100m          | 100Mi            |


### Drawbacks

As noted in the previous section, the full operator deployment has significant
hardware requirements. Configuring VFs manually and using the SR-IOV network
device plugin is more lightweight, but also tedious. Running a stripped down
version of the operator (without the metrics exporter and webhook) is also a
possible alternative.

The [webhook](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/networking_operators/sr-iov-operator#about-sr-iov-operator-admission-control-webhook_configuring-sriov-operator), along with the [resources injector](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/networking_operators/sr-iov-operator#about-network-resource-injector_configuring-sriov-operator) provide the following validation and mutation functionality:
- Validation of the SriovNetworkNodePolicy CR when it is created or updated.
- Mutation of the SriovNetworkNodePolicy CR by setting the default value for the
  priority and deviceType fields when the CR is created or updated. 
- Mutation of resource requests and limits in a pod specification to add an
  SR-IOV resource name according to an SR-IOV network attachment definition
  annotation.
- Mutation of a pod specification with a Downward API volume to expose pod
  annotations, labels, and huge pages requests and limits.

Losing the webhook (and resources injector) would mean losing this
functionality, leaving more responsibility to the user.

## Alternatives (Not Implemented)

A good alternative to using the operator would be using the SR-IOV network
device plugin and CNI directly. This would be easier on the resources, and a
customer is already using this approach successfully. On the other hand, this is
also more complicated for users, and could be more difficult to support. In the
future, this could be a better solution, if we find the operator to be too
resource hungry.

## Open Questions [optional]

1. The CPU/memory usage is high when running a full deployment (with metrics
   exporter and resources injector). For now, we can leave them out in order to
   save resources. If users require them later, it will need to be addressed.

## Test Plan

`microshift-sriov` will be tested in MicroShift test harness using QEMU/KVM and
the `igb` driver, which emulates an SR-IOV compatible Intel 82576 Network
Adapter. This way, no special hardware is needed for testing. Currently, the
plan is to only implement a simple smoke test, not a full test of all SR-IOV
features; the assumption being that the OpenShift SR-IOV operator tests cover
the MicroShift use case.

## Graduation Criteria

`microshift-sriov` is targeted to be GA next release.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

Both MicroShift and `microshift-sriov` will be built from the same spec file and
their versions will be matched. It is expected they are updated together.

## Version Skew Strategy

As the `microshift-sriov` package and MicroShift will be built from the same
.spec file, no skew should be introduced.

## Operational Aspects of API Extensions

## Support Procedures

OpenShift SR-IOV network operator support procedures are to be followed.

## Infrastructure Needed [optional]

N/A
