---
title: compact-clusters
authors:
  - "@smarterclayton"
reviewers:
  - "@derekwaynecarr"
  - "@ironcladlou"
  - "@crawford"
  - "@hexfusion"
approvers:
  - "@derekwaynecarr"
creation-date: "2019-09-26"
last-updated: "2019-09-26"
status: implementable
see-also:
replaces:
superseded-by:
---

# Compact Clusters

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

OpenShift is intended to run in a wide variety of environments. Over the years
we have refined and reduced the default required footprint - first by removing
the hard requirement for infrastructure nodes in 4.1, and second by preparing
for 3-node clusters in 4.2 with the introduction of the masterSchedulable flag.
OpenShift should continue to take steps to reduce the default footprint and
incentivize improvements that allow it to work with smaller 3-node and eventually
one node clusters.

## Motivation

This proposal covers the majority of near term improvements to OpenShift to
allow it to fit within smaller footprints.

Our near term goal is to continue to drive down the control plane and node
footprint and make three node clusters a viable production deployment strategy
on both cloud and metal, as well as exert engineering pressure on reducing our default
resource usage in terms of CPU and memory to fit on smaller machines.

At the same time, it should be easy on cloud providers to deploy 3-node clusters,
and since we prefer to test as our users will run, our CI environments should test
that way as well.

### Goals

* Ensure 3-node clusters work correctly in cloud and metal enviroments in UPI and IPI
* Enforce guard rails via testing that prevent regressions in 3-node environments
* Describe the prerequisites to support single-node clusters in a future release
* Clarify the supported behavior for single-node today
* Identify future improvements that may be required for more resource constrained environments
* Incentivize end users to leverage our dynamic compute and autoscaling capabilities in cloud environments

### Non-Goals

* Supporting any topology other than three masters for production clusters at this time

## Proposal

### User Stories

#### Story 1

As a user of OpenShift, I should be able to install a 3-node cluster (no workers)
on cloud environments via the IPI or UPI installation paths and have a fully conformant
OpenShift cluster (all features enabled).

#### Story 2

As a developer of OpenShift, I should be able to run smaller clusters that are more
resource efficient for iterative testing while still being conformant.

#### Story 3

As a developer of OpenShift, I should be exposed to bugs and failures caused by
assumptions about cluster size or the topology of cluster early, so that my feature
additions do not break small cluster topologies.

#### Story 4

As an admin of an OpenShift cluster deployed onto 3-nodes, I should be able to easily
transition to a larger cluster and make the control plane unschedulable.

### Risks and Mitigations

This proposal is low risk because it depends only on already approved upstream
functionality and is already supportable on bare-metal.

## Design Details

### Enabling three-node clusters in the cloud

OpenShift 4.3 should pass a full e2e run on a 3-node cluster in the cloud.

The primary limitation blocking this in 4.2 and earlier was a limitation in
Kubernetes that prevented service load balancers from targeting the master nodes
when the node-role for masters was set. This was identified as a bug and in
Kube 1.16 we [clarified that this should be fixed by moving the functionality to
explicit labels](https://github.com/kubernetes/enhancements/issues/1143) and
[added two new alpha labels](https://github.com/kubernetes/kubernetes/pull/80238)
that made the old behavior still achievable. The new alpha feature gate `LegacyNodeRoleBehavior`
disables the old check, which would allow service load balancers to target masters.
The risk of regression is low because the code path is simple, and all usage of
the gate would be internal. We would enable this gate by default in 4.3.

The IPI and UPI install and documentation must ensure that service load balancers
can correctly target master machines if a 3-node cluster is created by ensuring
security groups are correctly targeted. It may be desirable to switch the default
ingress port policy to be NodePort to remove the need to listen on the host in
these environments in 4.3 across all appropriate clouds, but that may not be
strictly required.

In order to ensure this is successful and testable by default, some of the default
e2e-aws runs should be switched to a 3-node cluster configuration, no worker pool
defined. Tests that fail should be corrected. Resource limits that are insufficient
to handle the workload should be bumped. Any hardcoded logic that breaks in this
configuration should be fixed.

At the end of this work, we should be able to declare a 3-node 4.3 cluster in the
cloud as fully supported.

### Support for one-node control planes

OpenShift 3.x supported single node "all-in-one" clusters via both `oc cluster up`,
`openshift-ansible`, and other more opinionated configurations like pre-baked VMs.
OpenShift 4, with its different control plane strategy, intentionally diverges from
the static preconfiguration of 3.x but is geared towards highly available control
planes and upgrades. To tolerate single control-plane node upgrades we would need
to fix a number of assumptions in core operators. In addition, we wish to introduce
the cluster-etcd-operator to allow for master replacement, and the capabilities
that operator will introduce

For the 4.3 timeframe the following statements are true for single-node clusters:

* The installer allows creation of single-master control planes
* Some components may not completely roll out because they require separate nodes
  * Users may be required to disable certain operators and override their replicas count
* Upgrades may hang or pause because operators assume at least 2 control plane members are available at all times

In general it will remain possible with some work to get a single-node cluster in
4.3 but it is not supported for upgrade or production use and will continue to have
a list of known caveats.

The current rollout plan for replacement of masters is via the cluster-etcd-operator
which will dynamically manage the quorum of a cluster from single instance during
bootstrap to a full HA cluster, and updating the quorum as new masters are created.
Because we expect the operator to work in a single etcd member configuration during
bootstrap and then grow to three nodes, we must ensure single member continues to
work. That opens the door for future versions of OpenShift to run a larger and
more complete control plane on the bootstrap node, and we anticipate leveraging that
flow to enable support of single-node control planes. We would prefer not to provide
workarounds for single-node clusters in advance of that functionality.

A future release would gradually reduce restrictions on single-node clusters to
ensure all operators function correctly and to allow upgrades, possibly including
but not limited to the following work:

* A pattern for core operators that have components that require >1 instance to:
  * Only require one instance
  * Colocate both instances on the single machine
  * Survive machine reboot
* Allowing components like ingress that depend on HostNetwork to move to NodePorts
* Ensuring that when outages are required (restarting apiserver) that the duration is minimal and reliably comes back up
* Ensuring any components that check membership can successfully upgrade single instances (1 instance machine config pool can upgrade)

In the short term, we recommend users who want single machines running containers
to use RHCoS machines with podman, static pods, and system units, and Ignition to
configure those machines.  You can also create machine pools and have these remote
machines launch static pods and system units without running workload pods by
excluding normal infrastructure containers.


### Reducing resource requirements of the core cluster

The OpenShift control plane cpu, memory, and disk requirements are largely dominated
by etcd (io and memory), kube-apiserver (cpu and memory), and prometheus (cpu and memory).
Per node components scale with cluster size `O(node)` in terms of cpu and memory on
the apiservers, and components like the scheduler, controller manager, and other
core controllers scale with `O(workload)` on themselves and the apiservers. Almost all
controller/operator components have some fixed overhead in CPU and memory, and requests
made to the apiserver.

In small clusters the fixed overhead dominates and is the primary optimization target.
These overheads include:

* memory use by etcd and apiserver objects (caches) for all default objects
* memory use by controllers (caches) for all default objects
* fixed watches for controller config `O(config_types * controllers)`
* kubelet and cri-o maintenance CPU and memory costs monitoring control plane pods
* cluster monitoring CPU and memory scraping the control plane components
* components that are made highly available but which could be rescheduled if one machine goes down, such that true HA may not be necessary

Given the choice between dropping function that is valuable but expensive, OR optimizing that function to be more efficient by default, **we should prefer to optimize that valuable function first before making it optional.**

The short term wins are:

* Continue to optimize components to reduce writes per second, memory in use, and excess caching
* Investigate default usage of top components - kube-apiserver and prometheus - and fix issues
* In small clusters, consider automatically switching to single instance for prometheus AND ensure prometheus is rescheduled if machine fails

The following optimizations are unlikely to dramatically improve resource usage or platform health and are non-goals:

* Disabling many semi-optional components (like ingress or image-registry) that have low resource usage
* Running fewer instances of core Kube control plane components and adding complexity to recovery or debuggability

### Test Plan

We should test in cloud environments with 3-node clusters with machines that represent
our minimum target and ensure our e2e tests (which stress the platform control plane)
pass reliably.

We should ensure that in 3-node clusters disruptive events (losing a machine, recovering
etcd quorum) do not compromise function of the product.

We should add tests that catch regressions in resource usage.

### Graduation Criteria

This is an evolving plan, it is considered a core requirement for the product to remain
at a shippable GA quality.


### Upgrade / Downgrade Strategy

In the future, this section will be updated to describe how single master control planes
might upgrade. At the current time this is not supported.

If a user upgrades to 4.3 on a cloud, then migrates workloads to masters, then reverts
back, some function will break. Since that is an opt-in action, that is acceptable.


### Version Skew Strategy

Enabling the Kubernetes 1.16 feature gates by default for master placement will impact
a customer who ASSUMES that masters cannot use service load balancers.  However, nodes
that don't run those workloads are excluded from the load balancer automatically, so
the biggest risk is that customers have configured their scheduling policies inconsistent
with their network topology, which is already out of our control. We believe there is
no impact here for master exclusion.

All other components described here have no skew requirements.


## Implementation History


## Drawbacks

We could conceivably never support clusters in the cloud that run like our compact
on-premise clusters (which we have already started supporting for tech preview).

We may be impacted if the Kubernetes work around master placement is reverted, which
is very unlikely. The gates would be the only impact.

## Alternatives

We considered and rejected running single masters outside of RHCoS, but this loses
a large chunk of value.
