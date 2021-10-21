---
title: External Control Plane Topology
authors:
  - "@csrwng"
reviewers:
  - "@derekwaynecarr"
  - "@ironcladlou"
  - "@enxebre"
  - "@sjenning"
  - "@sttts"
  - "@deads2k"
  - "@mfojtik"
  - "@s-urbaniak"
  - "@spadgett"
  - "@dmage"
  - "@Miciah"
approvers:
  - "@derekwaynecarr"
  - "@smarterclayton"
creation-date: 2021-04-19
last-updated: 2021-04-19
status: implementable
see-also:
  - "/enhancements/update/ibm-public-cloud-support.md"
  - "/enhancements/single-node/cluster-high-availability-mode-api.md"
---

# External Control Plane Topology

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

External control plane support was introduced in OCP 4 with the support
for the [IBM Cloud Managed service](https://github.com/openshift/enhancements/blob/master/enhancements/update/ibm-public-cloud-support.md) (ROKS). At the time, this was the only platform that
was run with an external control plane. It was sufficient to use a platform type of
`IBMCloud` to distinguish it from other OCP installations with traditional
control plane topology.

More recently, an orthogonal field was [added to the Infrastructure resource](https://github.com/openshift/enhancements/blob/master/enhancements/single-node/cluster-high-availability-mode-api.md) to indicate
the type of control plane topology. Current supported values for the `controlPlaneTopology` field
are `HighlyAvailable` and `SingleReplica`.

This enhancement proposes adding a third possible value to the control plane topology
field: `External`. A value of `External` in this field indicates that control plane components
such as Etcd, Kube API server, Kube Controller Manager, and Kube Scheduler are running outside
the cluster and are not visible as pods inside the cluster.

## Motivation

Whether the control plane is external or not should not be tied to the platform that the cluster
is running on. IBM Cloud will soon support IPI/UPI installation. Thus, having a platform of `IBMCloud`
will not imply an external control plane. Hypershift will bring support for external control planes
to existing platforms such as AWS.

### Goals

- Allow expressing a new type of control plane topology in the Infrastructure resource inside an
  OCP cluster.

- Provide operators/components that change their behavior based on whether the control plane is external or
  self hosted, a clear indicator of what mode they're running in.

### Non-Goals

- Provide a design for running OCP with an external control plane.

- Describe how the `controlPlaneTopology` field will be set for an external control plane deployment.

## Proposal

- Add `External` as an additional option to the `TopologyMode` type for `Infrastructure`

### User Stories

As a platform provider I can set a control plane topology of External to signal OCP components to adjust their
behavior accordingly.


### Implementation Details

Currently `External` only makes sense as a mode for the ControlPlaneTopology, not the InfrastructureTopology, but
both use the same type (`TopologyMode`).

The Hypershift or IBM ROKS installer is responsible for setting this value when bootstrapping a new hosted control
plane OpenShift cluster.

#### Component Impact

Existing components such as Console and Monitoring that use platform type of `IBMCloud` to modify their behavior
for external control planes would need to switch to `controlPlaneTopology` to determine whether the control plane
is external.

### Risks and Mitigations


## Design Details

### Test Plan

IBM ROKS will need to be regression tested by IBM Cloud QE. There is no impact to mainline OCP.

### Graduation Criteria
#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
#### Removing a deprecated feature
### Upgrade / Downgrade Strategy
### Version Skew Strategy

## Implementation History

## Drawbacks

None

## Alternatives

None
