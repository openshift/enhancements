---
title: hypershift-for-multiple-architectures
authors:
  - "@jaypoulz"
  - "@mkumatag"
  - "@deepsm007"
  - "@jeffdyoung"
  - "@bryan-cox"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2022-07-08
last-updated: 2022-07-08
tracking-link:
  - [HyperShift Epic](https://issues.redhat.com/browse/MULTIARCH-2060)
see-also:
  - [Heterogeneous Enhancement](https://github.com/openshift/enhancements/pull/1014)
---
# HyperShift Across Multiple Architectures

Expanding upon our work towards OpenShift working across nodes of different
architectures, HyperShift opens up new doors for us. With HyperShift, we can
maximize cost-efficiency by running our orchestration workloads on competitively
priced instance types, regardless of architecture. Other objectives includ
enabling operators whose availability is tied to an architecture to still be
able to run regardless of the control plane architecture.

## Summary

Enabling HyperShift across Multiple Architectures starts by extending the
platforms and architectures it can currently target as compute nodes. This
enables clusters to benefit from HyperShift's scaling on x86 while still
being able to target workloads on IBM Power, IBM zSystems, and ARM.

Another driver major driver is the cost-benefit of being able to schedule
orchestration workloads on the lowest cost instance type costs in public cloud
environments. If you compare different clouds, it becomes clear that those with
ARM instances tend to price those very competitively. By enabling the HyperShift
operator on ARM, we are looking at how we can enable users to reap these cost
benefits for their workloads.

Finally, looking at the IBM Power and zSystem environments, there is huge
potential for the benefits of being able to provision new clusters quickly in
these environments, as well as the potential of being able to run hybrid
workloads with operators or technology that is not available natively on those
platforms.

## Motivation

Here we will try to expand on the summary above with examples of each.

### Goals

Summarize the specific goals of the proposal. How will we know that
this has succeeded?  A good goal describes something a user wants from
their perspective, and does not include the implementation details
from the proposal.

### User Stories

#### Control Cluster (x86) with Non-x86 Compute Nodes
As a cluster admistrator of a control cluster with HyperShift enabled, I would
like to add my IBM Power, IBM zSystem, and/or ARM compute nodes to my new
cluster so that I can run my architecture specific workloads without having to
set up another control plane with additional hardware. I want this to work
regardless of whether my additional nodes in a public cloud or not.

- This means that whereever non-x86 instances that support OpenShift are
 available (ARM on AWS, Azure, etc.; IBM Power on Power VS for IBM Cloud), I
 should be able to add them to my cluster.
- All arches should support non public cloud hardware as well.

#### Control Cluster (ARM) with Any Arch Compute Nodes
As a cluster admistrator of an ARM cluster in a public cloud environment, I
would like to provision clusters so that I can benefit from having my
orchestration pods running on cost-effective instances.

- This has major potential cost-savings for managed services, and other types of
  long-standing static infrastructure.

#### Control Cluster (Non-x86) with Any Arch Compute Nodes
As a cluster administrator of a non-x86 control cluster with HyperShift enabled,
I would like easily scale out more clusters without having to dedicate hardware
to each control plane. I would also like to be able to add
control-plane-architecture agnostic compute nodes to my environment so that I
can run operators or technologies that are only available on a subset of
architectures in my environment.

- All architectures benefit from HyperShift's scaling technology, but non-x86
  clusters also benefit majorly from being able to add in x86 nodes for
  dependencies that are only built for that architecture. This is especially
  important when you consider all of the community and partner content that
  is only available for x86.

### Non-Goals

This enchancment assumes that the control cluster where HyperShift is enabled is
fully homogeneous. It does not explore the anything related to a heterogeneous
control cluster, since that adds a third layer of architecture mismatch. Above
we talk about the architecture of the control plane, and the architecture of the
remote compute nodes. If you include heterogeneous clusters, we'd also have to
talk about the architecture of the compute node of the control cluster where the
pod-based control plane is running. This however, is the same discussion as one
would have for any operator running on a heterogeneous cluster, and is thus best
discussed and analyzed as part of the
[Heterogeneous enhancement proposal](https://github.com/openshift/enhancements/pull/1014).

## Proposal

TODO

### Workflow Description

TODO


### API Extensions

N/a

### Implementation Details/Notes/Constraints [optional]

TODO

### Risks and Mitigations

This effort is dependent on heterogeneous payloads. While this work is on track
for upcoming OpenShift releases, it should still be noted as a risk. Risk is
being mitigated by stretching the delivery timeframe for different architectures
and delivering the features in a piecewise fashion to give us more time to
ensure functionality and fully test each new platform.

### Drawbacks

The only drawback to investing heavily in this direction is that a lot of the
benefits you get from HyperShift across multiple architectures, you also get
from regular heterogeneous clusters. For example, with heterogeneous clusters,
you can run your control planes on strategically chosen instances and add
compute nodes of a different arch to run specific workloads. However, the big
differentiator for HyperShift the power to scale out, and since this is the
direction OpenShift is moving, we believe this is an important step forward to
take for all architectures.

## Design Details

### Open Questions [optional]

N/a

### Test Plan

TODO

### Graduation Criteria

Since HyperShift is already available for x86, we aren't planning on having a
separate graduation path for these enhancements. Each sub-deliverable is planned
to be GA in the release it is made available.

### Upgrade / Downgrade Strategy

N/a

### Version Skew Strategy

N/a

### Operational Aspects of API Extensions

N/a

## Implementation History

### 4.12 Targets
[ ] Support for Power remote compute nodes (bare metal and PowerVS)
[ ] Support for ARM remote compute nodes (bare metal, Azure, AWS)

### 4.13 Candidate Targets
[ ] Support for Z remote compute nodes (bare metal and any supported cloud envs if applicable)
[ ] HyperShift operator running on ARM

### 4.13 or Beyond Targets
[ ] HyperShift operator running on Power
[ ] HyperShift operator running on Z

## Alternatives

N/a

## Infrastructure Needed [optional]

N/a
