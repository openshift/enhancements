---
title: arbiter-clusters
authors:
  - "@eggfoobar"
reviewers:
  - "@tjungblu"
  - "@patrickdillon"
  - "@racedo"
  - "@deads2k"
  - "@jerpeter1"
approvers:
  - "@tjungblu"
  - "@patrickdillon"
  - "@racedo"
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
- As an OpenShift cluster admin I want to be able to allow deployments with
  proper tolerations or explicitly defined node in the `spec` to be able to be
  scheduled on the arbiter node.

### Goals

- Provide a new arbiter node role type that achieves HA but does not act as a
  full master node.
- HA for a 2+1 arbiter node should match the HA guarantees of a 3 Node Cluster
  deployment.
- Support installing OpenShift with 2 master nodes and 1 arbiter node.
- The arbiter node hardware requirements will be lower than regular nodes in
  both cost and performance. Customers can use devices on the market from OEMs
  like Dell that supply an all in one unit with 2 compute and 1 lower powered
  compute for this deployment scenario.
- Moving from 2 + 1 to a conventional 3 node cluster

### Non-Goals

The below goals are not intended to be worked on now, but might be expansion
ideas for future features.

- Running the arbiter node offsite.
- Running a virtualized arbiter node on the same cluster.
- Having a single arbiter supporting multiple clusters.

## Proposal

The main focus of the enhancement is to support edge deployments of individual
OpenShift HA clusters at scale, and to do so in a cost effective way. We are
proposing doing this through the incorporation of an arbiter node as a quasi
heterogenous control plane configuration. The arbiter will run the critical
components that help maintain an HA cluster, but other platform pods should not
be scheduled on the arbiter node. The arbiter node will be tainted to make sure
that only deployments that tolerate that taint are scheduled on the arbiter.

Functionality that we are proposing to change:

- Update MCO MachinePool Validation Webhook to support `master/arbiter`
  configuration.
  - Currently MCO blocks custom machine pools that are paired with the `master`
    role, we will need to update this to explicitly support the `arbiter` role
    only. This will allow the arbiter to inherit the same machine configs as the
    `master`.
- We will add support to the OCP installer to provide a way of setting up the
  initial manifests, taints, and node roles.
  - We will need to support a path for customers to indicate the desire for a
    2+1 arbiter install configuration.
  - This will also be used to apply the taint to the machineset manifest.
- Alter Cluster ETCD Operator (CEO) to be aware of the arbiter node role type
  and allow it to treat it as if it were a master node.
  - We will need CEO to create an ETCD member on the arbiter node to allow
    quarum to happen
- Update the tolerations of any critical or desired component that should be
  running on the arbiter node.

### Workflow Description

#### For Cloud Installs

1. The user creates an `install-config.yaml`.
2. The user defines the `install-config.controlPlane` field with `3` replicas.
3. The user then enters the new field `install-config.controlPlane.arbiterNode`
   and sets it to `true`
4. The user generates the manifests with this install config via the
   `openshift-install create manifests`
5. The installer creates a new `arbiter` MachineSet with a replica of 1 and
   reduces the default control plane replicas to `2`
6. With the flag `arbiterNode` in the install config, the installer adds the
   `node-role.kubernetes.io/arbiter: ""` to the machine object.
7. The installer applies the taint to the arbiter MachineSet
8. The user can make any alterations to the node machine type to use less
   powerful machines.
9. The user then begins the install via `openshift-install create cluster`

#### For Baremetal Installs

1. The user creates an `install-config.yaml` like normal.
2. The user defines the `install-config.controlPlane` field with `3` replicas.
3. The user then enters the new field `install-config.controlPlane.arbiterNode`
   and sets it to `true`
4. The user then enters the machine information for `platform.baremetal` and
   identifies one of the nodes as a role `arbiter`
5. With the flag `arbiterNode` in the install config, the installer adds the
   `node-role.kubernetes.io/arbiter: ""` to the machine object.
6. The user then begins the install via `openshift-install create cluster`

#### During Install

1. The CEO will watch for new masters and the arbiter role
2. CEO will create the operand for the etcd deployments that have tolerations
   for the arbiter
3. Operators that have tolerations for the arbiter should be scheduled on the
   node
4. The install should proceed as normal

### API Extensions

The `installConfig` will include a `install-config.controlPlane.arbiterNode`
bool flag to denote arbiter infra structure.

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
members, we should provide proper guidance so that the arbiter node must meet
minimum requirements for ETCD to function properly. ([ETCD
Recommendations](https://docs.openshift.com/container-platform/4.16/scalability_and_performance/recommended-performance-scale-practices/recommended-etcd-practices.html))

### Drawbacks

A few drawbacks we have is that we will be creating a new variant of OpenShift
that implements a new unique way of doing HA for kubernetes. This does mean an
increase in the test matrix and all together a different type of tests since the
addition of an arbiter node will require different validation scenarios for
failover.

## Open Questions [optional]

1. In the future it might be desired to add another master and convert to a
   compact cluster, do we want to support changing ControlPlaneTopology field
   after the fact?

## Test Plan

- We will create a CI lane to validate install and fail over scenarios such as
  loosing a master or swaping out an arbiter node.
- CI lane for e2e conformance testing, tests that explicitly test 3 node masters
  will need to be altered to accommodate the different topology.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A

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
to use a device that is lower power and is cheaper as the arbiter. This method
would still run most of the OCP overhead on the arbiter node.

## Infrastructure Needed [optional]

N/A
