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
operator into MicroShift in order to enable users to configure VFs in a more
convenient manner.

## Motivation

Currently, SR-IOV can be used with MicroShift by manually configuring VFs
(Virtual Functions) on a PF (Physical Function) on OS level and using multus to
map the VFs to pods. This approach is tedious and non-idiomatic. By using the
SR-IOV network operator, users can configure VFs in a declarative way by
specifying and deploying a NetworkNodePolicy CRD.

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
- Manifests will be based on existing manifests for OpenShift SR-IOV operator.
  Possible changes may include optimizations in regards to CPU/memory usage (to
  be discussed).

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

```
network-resources-injector
Requests:
  cpu:     10m
  memory:  50Mi

network-webhook
Requests:
  cpu:     10m
  memory:  50Mi

sriov-device-plugin
Requests:
  cpu:     10m
  memory:  50Mi

sriov-network-config-daemon
Requests:
  cpu:     100m
  memory:  100Mi

sriov-metrics-exporter
Requests:
  cpu:        100m
  memory:     100Mi
Requests:
  cpu:     10m
  memory:  20Mi

sriov-network-operator
Requests:
  cpu:     100m
  memory:  100Mi
```

This is significant, and will need to be addressed in the implementation.
Possible solution could be merging some of the pods. Some of the pods (e.g. the
metrics exporter) are optional and could be left out. To be further discussed. 

When running without the webhook and metrics exporter, the deployment consists
of only 3 pods:

```
sriov-device-plugin
    Requests:
      cpu:     10m
      memory:  50Mi

sriov-network-config-daemon
    Requests:
      cpu:     100m
      memory:  100Mi

sriov-network-operator
    Requests:
      cpu:     100m
      memory:  100Mi
```

Further discussion is required to determine whether this is acceptable, or needs
to be lowered even more by other means.

### Drawbacks

As noted in the previous section, the full operator deployment has significant
hardware requirements. Configuring VFs manually and using the SR-IOV network
device plugin is more lightweight, but also tedious. Running a stripped down
version of the operator (without the metrics exporter and webhook) is also a
possible alternative.

## Alternatives (Not Implemented)

See "Drawbacks".

## Open Questions [optional]

1. The CPU/memory usage is high and needs to be addressed, either by running
   multiple containers in a pod, or by not running some of them. See above.

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
