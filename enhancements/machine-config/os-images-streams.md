---
title: machine-config-os-images-streams
authors:
  - "@pablintino"
reviewers:
  - "@yuqi-zhang"
approvers:
  - "@yuqi-zhang"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-10-24
tracking-link:
  - https://issues.redhat.com/browse/MCO-1914
see-also:
replaces:
superseded-by:
---

# MachineConfig OS Images Streams

## Summary

This enhancement allows administrators to easily assign different OS images
to specific groups of nodes using a simple "stream" identifier.

It introduces a new, optional stream field in theMCP. When this field is set,
the MCO will provision nodes in that pool using the specific OS image 
associated with that stream name.

This provides a simple, declarative way to run different OS variants within
the same cluster. This can be used to test new major OS versions 
(like RHEL 10) on a subset of nodes or to deploy specialized images,
without affecting the rest of the cluster.

## Motivation

**TBD**


### User Stories

**TBD**

### Goals

**TBD**

### Non-Goals

**TBD**

## Proposal

**TBD**

### API Extensions

**TBD**

### Topology Considerations

**TBD**

### Implementation Details/Notes/Constraints

**TBD**

### Risks and Mitigations

**TBD**

### Drawbacks

**TBD**

## Design Details

### Open Questions [optional]

None.

## Test Plan

**TBD**

## Graduation Criteria

**TBD**

### Dev Preview -> Tech Preview

**TBD**

### Tech Preview -> GA

**TBD**

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

**TBD**

## Version Skew Strategy

**TBD**

## Operational Aspects of API Extensions

#### Failure Modes

**TBD**

## Support Procedures

None.

## Implementation History

Not applicable.

## Alternatives (Not Implemented)

**TBD**
