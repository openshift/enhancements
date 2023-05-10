---
title: microshift-csi-snapshot-integration
authors:
  - @copejon
reviewers:
  - @dhellman
  - @pmtk
  - @eggfoobar
  - @pacevedom
approvers:
  - @dhellmann
api-approvers:
  - None
creation-date: 2023-05-10
last-updated: 2023-05-10
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-1140
see-also: []
---

# CSI Snapshotting Integration

## Summary

MicroShift is a small form-factor, single-node OpenShift targeting IoT and Edge Computing use cases characterized by
tight resource constraints and single-tenant workloads. See [kubernetes-for-devices-edge.md](./kubernetes-for-device-edge.md)
for more detail.

This document proposes the integration of the CSI Snapshot Controller to support backup and restore customer scenarios.  The
snapshot controller, along with the CSI external snapshot sidecar, will provide an API driven path for managing stateful
workload data.

The CSI Snapshot controller and sidecar are components of the Kubernetes CSI implementation.  OpenShift maintains downstream
versions of these images and tracks them as part of OCP releases.  The controller container is responsible for handling 
snapshot APIs

## Motivation

CSI snapshot functionality was originally excluded from the CSI driver integration in MicroShift to support the low-resource
overhead goals of the project. However, user feedback has made it clear that a supportable, robust backup/restore solution
is necessary for meeting certain user-needs.  While it is possible to run a workflow out-of-band to manage workload data,
this would be reinventing the wheel and contribute significantly to technical debt. 

### User Stories

* A device owner has deployed stateful workloads onto a MicroShift cluster.  During MicroShift runtime, the device owner
wants to create a snapshot of workload state

### Goals

* Enable an in-cluster workflow for snapshotting and restoration of cluster workload data

* Follow the [MicroShift design
  principles](https://github.com/openshift/microshift/blob/main/docs/design.md)

### Non-Goals

* Exporting data from a MicroShift cluster

* Automating event triggered backup/restore operations.

## Proposal
 
Integrate the CSI snapshot controller and sidecar into the MicroShift installation.  

### Workflow Description

Assumptions:
* An active MicroShift cluster
* An active stateful cluster workload
* Workload state is stored on a LVM Thin volume

#### Deploying

#### Upgrading

#### Configuring

#### Deploying Applications

### API Extensions

### Risks and Mitigations

### Drawbacks

## Design Details

### Open Questions [optional]

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Full GA

- Available by default

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

## Alternatives