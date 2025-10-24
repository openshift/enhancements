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

To implement the functionality this enhancement provides, some changes are 
required in the MCO, the released images payload, and the CoreOS images.
The following sections describe all the required changes.

### Machine Config Pools

To provide the user with the ability to set which stream an MCP's nodes 
should use, the MCP CRD must be modified to introduce a few fields:

- `spec.osImageStream`: To set the target stream the pool should use. We 
preserve the current behavior of deploying the cluster-wide OS images if 
no stream is set.
- `status.osImageStream`: To inform the user of the stream currently used 
by the pool. This field will reflect the target stream once the pool has 
finished updating to it.

The [API Extensions](#api-extensions) section describes these API changes 
in greater detail.

From the perspective of the MCP reconciliation logic, the addition of 
streams is not different from an override of both OS images in the 
MachineConfig of the associated pool. If a user sets a stream in the 
pool, the MCO takes care of picking the proper URLs to use from the 
new, internally populated, OSImageStream resource and injecting them 
as part of the MCP's MachineConfig. This internal change of the URLs 
will force the MCP to update and deploy the image on each node one by 
one.

### CoreOS and Payload Images

The scope of this enhancement is to allow the user to consume streams shipped 
as part of the payload. Therefore, all information about which streams are 
available should be contained in the payload image and the tagged OS images.

To accommodate more than one OS version and the associated stream name, the 
release build process has been updated with the following changes:

The Payload ImageStream now contains extra coreos tags for both OS and 
Extension Images to accommodate more OS versions.

Each OS image is now built with an extra label that allows the MCO to identify 
the stream to which it belongs.

- Regular OS Images: `io.coreos.oscontainerimage.osstream` pointing to the 
stream name, for example, `rhel-coreos-10`.
- Extension Images: `io.coreos.osextensionscontainerimage.osstream` pointing 
to the stream name, for example, `rhel-coreos-10`.

With those changes to the images in place, the MCO has enough information to 
build the list of available streams and determine which images should be used
for each stream.

### Machine Config OSImageStream

This new resource holds the URLs associated with each stream and is populated
by the MCO using the information from the OS image labels. The logic that 
extracts the URLs and stream names from the OS images differs depending on 
whether the cluster is bootstrapping or undergoing an update. During regular 
operation (i.e., when not bootstrapping or updating), the MCO does not make 
any changes to this resource, and its information can be safely considered 
static.

#### TBD: Add details of both OSImageStream generation scenarios

#### Streams

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
