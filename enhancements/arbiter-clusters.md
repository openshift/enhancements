---
title: arbiter-clusters
authors:
  - "@eggfoobar"
reviewers:
  - "@tjungblu"
  - "@patrickdillon"
  - "@williamcaban"
  - "@deads2k"
  - "@jerpeter1"
approvers:
  - "@tjungblu"
  - "@patrickdillon"
  - "@williamcaban"
  - "@jerpeter1"
  - "@deads2k"
api-approvers:
  - "@JoelSpeed"
creation-date: 2024-08-27
last-updated: 2024-08-27
tracking-link:
  - https://issues.redhat.com/browse/OCPEDGE-1191
see-also: []
replaces: []
superseded-by: []
---

# Support 2 Node + 1 Arbiter Node HA Cluster

## Summary

This enhancement describes an ability to install OpenShift with a control plane
that consists of 2 normal sized nodes, and 1 node that can be less powerful than
the recommended node size. This 1 arbiter node will only be running critical
components for maintaining HA to allow the arbiter node size to be as small and
as low cost as possible with in reason.

## Motivation

Customers at the edge are requiring a more economical solution for HA
deployments at the edge. They can support running 2 node clusters for redundancy
but would like the option to deploy a lower cost node as an arbiter to supply
the 3 nodes for ETCD quorum.

### User Stories

- As a solutions architect for a retail organization, I want to deploy OpenShift
  at n number of store locations at the edge with only 2 regular sized nodes and
  1 lower cost node to maintain HA and keep compute costs down.
- As a solutions architect for cloud infrastructures, I want to offer low cost
  OpenShift deployments on purpose built hardware for a 2 + 1 configuration.
- As an OpenShift cluster admin I want non-critical applications deployed to my
  2 + 1 arbiter node cluster to not be scheduled to run on the arbiter node.

### Goals

- Provide a new node arbiter role type that supports HA but is not a full master
- Support installing OpenShift with 2 regular nodes and 1 arbiter node.
- The arbiter node hardware requirement will be lower than regular nodes.

### Non-Goals

The below goals are not intended to be worked on now, but might be expansion
ideas for future features.

- Running the arbiter node offsite
- Running the arbiter node as a VM local to the cluster
- Having a single arbiter supporting multiple clusters
- Moving from 2 + 1 to conventional 3 node cluster

## Proposal

The main focus of the enhancement is to support edge deployments of individual
OpenShift HA clusters at scale, and to do so in a cost effective way. We are
proposing doing this through the incorporation of an arbiter node as a quasi
heterogenous control plane configuration. The arbiter will run the critical
components that help maintain an HA cluster, but other platform pods should not
be scheduled on the arbiter node. The arbiter node will be tainted to make sure
that only deployments that tolerate that taint are scheduled on the arbiter.

Things that we are proposing of changing.

- Adding a new topology to the [OCP/API Control Plane
  Topology](https://github.com/openshift/api/blob/69df64132c911e9eb0176e9697f451c13457e294/config/v1/types_infrastructure.go#L103)
  - This type of change should have an authoritative flag that indicates layout
    of the control plane, this information would be valuable for operator
    developers so no inference is required.
- We will add support to the OCP installer to provide a way of setting up the
  initial manifests and the ControlPlaneTopology field.
  - We will need to support a path for customers to indicate the desire for a 2
    - 1 arbiter install configuration.
  - This will also be used to apply the taint to the machineset manifest.
- Alter CEO to be aware of the arbiter node role type and allow it to treat it
  as if it were a master node.
  - We will need CEO to create an ETCD member on the arbiter node to allow
    quarom to happen
- Update the tolerations of any critical or desired component that should be
  running on the arbiter node.

### Workflow Description

#### For Cloud Installs

1. User sits down at the computer.
2. The user creates an `install-config.yaml` like normal.
3. The user defines the `install-config.controlPlane` field with `3` replicas.
4. The user then enters the new field `install-config.controlPlane.arbiterNode`
   and sets it to `true`
5. The user generates the manifests with this install config via the
   `openshift-install create manifests`
6. With the flag `arbiterNode` in the install config, the installer adds the
   `ControlPlaneTopology: ArbiterHighlyAvailable` to the infrastructure config
   object.
7. The installer creates a new `arbiter` MachineSet with a replica of 1 and
   reduces the default control plane replicas to `2`
8. The installer applies the new node role and taint to the arbiter MachineSet
9. The user can make any alterations to the node machine type to use less
   powerful machines.
10. The user then begins the install via `openshift-install create cluster`

#### For Baremetal Installs

1. User sits down at the computer.
2. The user creates an `install-config.yaml` like normal.
3. The user defines the `install-config.controlPlane` field with `3` replicas.
4. The user then enters the new field `install-config.controlPlane.arbiterNode`
   and sets it to `true`
5. The user then enters the machine information for `platform.baremetal` and
   identifies one of the nodes as a role `arbiter`
6. With the flag `arbiterNode` in the install config, the installer adds the
   `ControlPlaneTopology: ArbiterHighlyAvailable` to the infrastructure config
   object.
7. The user then begins the install via `openshift-install create cluster`

#### During Install

1. The CEO will watch for new masters and the arbiter role
2. CEO will create the operand for the etcd deployments that have tolerations
   for the arbiter
3. Operators that have tolerations for the arbiter should be scheduled on the
   node
4. The install should proceed as normal

### API Extensions

The `config.infrastructure.controlPlaneTopology` enum will be extended to
include `ArbiterHighlyAvailable`

### Topology Considerations

#### Hypershift / Hosted Control Planes

At the time being there is no impact on Hypershift since this edge deployment
will require running the control plane.

#### Standalone Clusters

This change is relevant to standalone deployments of OpenShift at the edge or
datacenters. This enhancement specifically deals with this type of deployment.

#### Single-node Deployments or MicroShift

This change does not effect Single Node or MicroShift.

### Implementation Details/Notes/Constraints

Currently there are some behavior unknowns, we will need to put out a POC to
validate some of the desires in this proposal. In it's current version this
proposal is not exhaustive but will be filled out as we implement these goals.

We currently expect this feature to mainly be used by `baremetal` installs, or
specialized hardware that is built to take advantage of this type of
configuration. In the current design we make two paths for cloud and baremetal
installs in the installer. However, the cloud install is primarily for testing,
this might mean that we simplify the installer changes if we are the only ones
using cloud installs, since we can simply alter the manifests in the pipeline
with out needing to change the installer.

### Risks and Mitigations

The main risk in this enhancement is that because we are treating one of the
master nodes in a 3 node cluster as an arbiter, we are explicitly evicting
processes that would otherwise be a normal supported upstream configuration such
as a compact cluster. We run the risk of new components being critical to HA not
containing the proper tolerations for running on the arbiter node. One of the
mitigations we can take against that is to make sure we are testing installs and
updates.

Another risk we run is customers using an arbiter node with improper disk speeds
below that recommended for etcd, since etcd is sensitive to latency between
members, we should provide proper guidance so that the arbiter node doesn't
become a bottleneck for etcd.

### Drawbacks

A few drawbacks we have is that we will be creating a new variant of OpenShift
that implements a new unique way of doing HA for kubernetes. This does mean an
increase in the test matrix and all together a different type of tests since

## Open Questions [optional]

1. In the future it might be desired to add another master and convert to a
   compact cluster, do we want to support changing ControlPlaneTopology field
   after the fact?

## Test Plan

WIP

- Running e2e test would be preferred but might prove to be tricky due to the
  asymmetry in the control plane
- We need a strategy for validating install and test failures

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in
  [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

WIP

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

WIP

## Support Procedures

WIP

## Alternatives

We originally had tried using the pre-existing features in OCP, such as setting
a node as NoSchedule to avoid customer workloads going on the arbiter node.
While this whole worked as expected, the problem we faced is that the desire is
to use a very lower powered and cheap device as the arbiter, this method would
still run a lot of the overhead on the arbiter node.

## Infrastructure Needed [optional]

N/A
