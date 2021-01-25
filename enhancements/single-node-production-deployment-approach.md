---
title: single-node-production-deployment-approach
authors:
  - "@dhellmann"
  - "@mrunalp"
  - "@romfreiman"
  - "@markmc"
reviewers:
  - "@MarSik"
  - "@abhinavdahiya"
  - "@browsell"
  - "@celebdor"
  - "@cgwalters"
  - "@crawford"
  - "@eranco74"
  - "@hexfusion"
  - "@itamarh"
  - "@jwforres"
  - "@kikisdeliveryservice"
  - "@marun"
  - "@s-urbaniak"
  - "@sinnykumari"
  - "@soltysh"
  - "@sronanrh"
  - "@sudhaponnaganti"
  - TBD, probably all leads
approvers:
  - "@derekwaynecarr"
  - "@eparis"
  - "@markmc"
  - "@smarterclayton"
creation-date: 2020-12-10
last-updated: 2020-12-10
status: implementable
see-also:
  - "/enhancements/update/cluster-profiles.md"
  - "/enhancements/single-node-developer-cluster-profile.md"
  - https://github.com/openshift/enhancements/pull/302
  - https://github.com/openshift/enhancements/pull/414
  - https://github.com/openshift/enhancements/pull/440
  - https://github.com/openshift/enhancements/pull/504
replaces:
  - https://github.com/openshift/enhancements/pull/504
---

# Single-node Production Deployment Approach

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes the approach to deploying single-node
production OpenShift instances without using a cluster profile.

A cluster deployed in this way will differ from the default
`self-managed-highly-available` cluster profile in several significant
ways:

* The single node serves as both the cluster’s control plane and as a
  worker node.
* Many operators will be configured to reduce the footprint of their
  operands, such as by running fewer replicas.
* In-place upgrades will not be supported by the first iteration of
  single-node deployments.

One example of this use case is seen in telecommunication service
providers implementation of a Radio Access Network (RAN). This use case
is discussed in more detail below.

The approach described here is an alternative to using a cluster
profile, as was detailed in [#504](https://github.com/openshift/enhancements/pull/504).
With a few changes to existing operators that would also be needed for a
profile-based approach, and with the high-availability mode API
described in [#555](https://github.com/openshift/enhancements/pull/555), it is possible to produce single-node deployments
without introducing a new cluster profile.

## Motivation

The benefits of the cloud native approach to developing and deploying
applications is increasingly being adopted in the context of edge
computing. Here we see that as the the distance between an site and the
central management hub grows, the number of servers at the site tends to
shrink. The most distant sites typically have physical space for 1 server.

We are seeing an emerging pattern in which some infrastructure
providers and application owners desire:

1. A consistent deployment approach for their workloads across these
   disparate environments.
1. That the edge sites can operate independently from the central
   management hub.

And so, these users who have adopted Kubernetes at their their central
management sites wish to have independent Kubernetes clusters at the
more remote sites.

Of the several options explored for supporting the use of Kubernetes
patterns for managing workloads at these sites (see the alternatives
listed blow) a single-node deployment of OpenShift is the best way to
give users a consistent experience across all of their sites.

### Radio Access Network (RAN) Use Case

In the context of telcocommunications service providers' 5G Radio Access
Networks, it is increasingly common to see "cloud native" implementations
of the 5G Distributed Unit (DU) component. Due to latency constraints,
this DU component needs to be deployed very close to the radio antenna for
which it is responsible. In practice, this can mean running this
component on anything from a single server at the base of a remote cell
tower or in a datacenter-like environment serving several base stations.

A hypothetical DU example is an unusually resource-intensive workload,
requiring 20 dedicated cores, 24 GiB of RAM consumed as huge pages,
multiple SR-IOV NICs carrying several Gbps of traffic each, and
specialized accelerator devices. The node hosting this workload must
run a realtime
kernel, be carefully tuned to ensure low-latency requirements
can be met, and be configured to support features like Precision Timing
Protocol (PTP).

One crucial detail of this use case is the "cloud" hosting this workload
is expected to be "autonomous" such that it can continue operating with
its existing configuration and running the existing workload, even when
any centralized management functionality is unavailable.

### Goals

* This enhancement describes an approach for deploying OpenShift in
  single-node configurations for production use in environments with
  "reasonably significant" memory, storage, and compute resources.
* Clusters built in this way should pass most Kubernetes and OpenShift
  conformance and functional end-to-end tests. Any functional tests
  that must be skipped due to differences from a multi-node deployment
  will be documented.
* Operators running on single-node production clusters should do
  whatever they normally do, as closely as possible, including
  checking the health of their operands and making (potentially
  disruptive) changes to the cluster.

### Non-Goals

* This enhancement does not address single-node deployments in
  highly-constrained environments such as Internet-of-things devices
  or personal computers.
* This enhancement does not address "developer" use cases. See the
  [single-node-developer-profile](https://github.com/openshift/enhancements/pull/302)
  enhancement.
* This enhancement does not address high-availability for single-node
  deployments. It explicitly does not assume zero downtime of the
  kubernetes API or the workloads running on the cluster when
  operators change the configuration of the cluster.
* This enhancement does not address in-place upgrades. Upgrades will
  initially only be achieved by redeploying the machine and its
  workload. In-place upgrades will be addressed as a separate feature
  in another enhancement as follow-up work building on this iteration.
* This enhancement does not presume that (more) operators are modified
  to "render" manifests while the cluster is being built so that the
  operators do not have to be run in the cluster.
* This enhancement does not attempt to describe a way to "pre-build"
  deployment images, either generically or customized for a user.
* This enhancement does not address the removal of the bootstrap VM,
  although single-node clusters would benefit from that work, which
  will be described in a separate enhancement.

## Proposal

After the [capabilities API is
introduced](https://github.com/openshift/enhancements/pull/555), all
teams developing OpenShift components will need to consider how their
components should be configured when deployed and used when the
high-availability capability is disabled.

In the telco RAN use case, high-availability is typically achieved by
having multiple sites provide coverage to overlapping geographic
areas. Therefore, use of cluster-based high-availability features will
be limited (for example, by running a single API service).

Failures in edge deployments are frequently resolved by re-imaging or
physically replacing the entire host. Combining this fact with the
previous observation about the approach to providing highly-available
services lets us draw the conclusion that in-place upgrades do not
need to be supported by the first iteration of this
configuration. In-place upgrades will be addressed as a separate
feature in another enhancement as follow-up work building on this
iteration.

In a multi-node cluster, operators may safely trigger rolling reboots
or restarts of the control plane services without taking the cluster
offline. In a single-node deployment, any such restart would take some
or all of the services offline. The deployment or upgrade of a cluster
may include several host reboots, clustered together in time, until
the operators stabilize. Users should wait for this period of
instability before installing their workloads.

The single-node configuration of OpenShift will be sufficiently
generic to cater to a variety of edge computing use cases. As such,
OpenShift's usual cluster configuration mechanisms will be favored
where there is a likelihood that there will be edge computing use case
with differing requirements.  For example, there is no expectation
that single-node deployments will use the real-time kernel by
default -- this will continue to be a `MachineConfig` choice as per
[enhancements/support-for-realtime-kernel](https://github.com/openshift/enhancements/blob/master/enhancements/support-for-realtime-kernel.md). *See open questions*

### User Stories

#### As a user, I can deploy OpenShift in a supported single-node configuration

A user will be able to run the OpenShift installer to create a single-node
deployment, with some limitations (see non-goals above). The user
will not require special support exceptions to receive technical assistance
for the features supported by the configuration.

### Implementation Details/Notes/Constraints

Some OpenShift components require a minimum of 2 or 3 nodes. The new
cluster capabilities API will be used to configure these components as
appropriate for a single node.

#### installer

We will add logic to the installer to set the new
`highAvailabilityMode` status field of the `infrastructure` API
resource. The will value be set to `Full` by default and `None` when
the number of replicas for the control plane less than `3`. See the [cluster
high-availability mode
API](https://github.com/openshift/enhancements/pull/555) enhancement
for details.

#### machine-config-operator

The machine-config-operator includes a check in code that the number
of control plane nodes is 3. This code needs to be updated to look at
the new high-availability mode capability API to determine the number
of expected nodes to allow the operator to complete its work, for
example to enable the realtime kernel on a single node with the
appropriate performance profile settings.

The machine-config-operator tries to drain a node before applying
local changes and rebooting. In a true single-node deployment, there
is nowhere for drained pods to be rescheduled. In a deployment with a
single-node control plane and additional workers, some nodes could
potentially be drained, but there is no way for the MCO to know if the
workers are fully utilized. When the high-availability mode is `None`
the step needs to be skipped. Ideally we will be able to rely on the
[graceful shutdown support in
kubelet](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/2000-graceful-node-shutdown)
to stop all workloads safely as part of the reboot. That feature is
alpha in kubernetes 1.20 and disabled by default, so we will need to
add a feature gate to enable it.

#### cluster-authentication-operator

By default, the `cluster-authentication-operator` will not deploy
oauth-apiserver and oauth-server without a minimum of 3 control plane
nodes. This can be change by enabling
`useUnsupportedUnsafeNonHANonProductionUnstableOAuthServer`. The
operator will need to be updated to honor the new capabilities API
instead. Health checks for these components will also have to be
updated to allow a single replica.

#### cluster-etcd-operator

By default, `cluster-etcd-operator` will not deploy the etcd cluster
without minimum of 3 master nodes. This can be changed by enabling
`useUnsupportedUnsafeNonHANonProductionUnstableEtcd`. The operator
will need to be updated to honor the new capabilities API instead.

`etcd-quorum-guard` is managed by the cluster-version-operator, which
means it has a statically configured Deployment. The operator will
need to be updated to manage the `etcd-quorum-guard` so that it can
support changing the replica count or disabling the guard entirely,
depending on the best approach to honor the high-availability mode
setting in the capabilities API.

#### cluster-ingress-operator

By default, the `cluster-ingress-operator` deploys the router with 2
replicas. On a single node one will fail to start and the ingress will
show as degraded. The operator will need to be updated to honor the
new capabilities API to change the replica count to 1 when the
high-availability mode is none.

#### cluster-machine-approver

The operator should disable automatic machine approval. (See risks
section below.)

#### operator-lifecycle-manager

Operators managed by the `operator-lifecycle-manager` will look at the
`infrastructure` API if they need to configure their operand
differently based on the high-availability mode.

#### console-operator

Console currently deploys two replicas of both the console itself and
the downloads container. The console deployment uses affinity
configuration to ensure spreading. On a single node, having multiple
replicas with affinity configuration will cause the operator to report
as degraded.

The configuration for the downloads container will need to move from a
static manifest to be managed by the `console-operator`. The operator
will also need to look at the `infrastructure` API to determine how to
configure both deployments, using a replica count of 1 and omitting
the affinity rules for single-node environments.

### Risks and Mitigations

*How will security be reviewed and by whom? How will UX be reviewed and by whom?*

#### Control plane rollouts

kube-apiserver is periodically reconfigured by its operator in
response to changes in cluster state (e.g. weekly rotation of etcd
encryption keys). When this occurs, the apiserver may be inaccessible
for up to 2 minutes. Workloads that do not depend on the apiserver
would be expected to run without issue during this interval. Workloads
that do depend on apiserver availability would need to be resilient to
these events. OpenShift core components are already resilient in this
way.

#### Disruptive Configuration Changes

Single-node deployments of OpenShift necessarily will be prone to
disruption when `MachineConfig` changes are applied, especially if the
change triggers a system reboot. When this occurs, workloads will be
inaccessible for as long as it takes the host to completely reboot and
for OpenShift to restart. Users choosing single-node deployments need
to be aware of the trade-off in resiliency that comes with using fewer
resources, so we will need to ensure our user-facing documentation
provides adequate warning.

#### Cluster Machine Approver

Auto-approval of certificate signing requests requires 2 sources of
truth to avoid security attacks like
[kubeletmein](https://github.com/openshift/machine-config-operator/issues/731). In
single-node deployments we do not have a second source of truth (there
is no Machine and no other way to confirm the Node), so certificate
signing requests cannot be automatically approved from within the
cluster. We can disable the machine-approver-operator. An outside tool
must be used to approve any certificate signing requests instead.

#### Lack of high-availability

In a single-node configuration OpenShift would have limited ability to
provide resiliency features. We will need to clearly communicate these
limitations to customers choosing to use single-node deployments.

#### Failure to reboot

It is possible that a single node may fail to reboot due to an issue
with configuration, hardware, or environment. These single-node
deployments will be managed by external tools, and as with other
failures related to providing resilient operation, host boot failures
will need to be detected and mitigated by those tools, rather than the
local OpenShift instance.

#### Reboots for reconfiguration

Some reconfiguration operations will unavoidably cause the node to
reboot. These tasks will need to be scheduled during maintenance
windows.  Ideally we will be able to rely on the [graceful shutdown
support in
kubelet](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/2000-graceful-node-shutdown)
to stop all workloads safely as part of the reboot.

Users can pause the `MachineConfigPool` to avoid unexpected reboots,
although this does also prevent configuration updates that require
changing the `MachineConfig`.

## Design Details

### Open Questions

1. Telco workloads typically require special network setups
   for a host to boot, including bonded interfaces, access to multiple
   VLANs, and static IPs. How do we anticipate configuring those?
2. How do we hand off ownership of the `etcd-quorum-guard` Deployment
   from the `cluster-version-operator` to the `cluster-etcd-operator`?
3. How does a user or workload operator know when the cluster has
   reached a steady state after initial installation so that it is
   safe to deploy a workload? Is the `ClusterVersion` resource
   sufficient? What about disruptions caused by operators such as the
   `performance-addon-operator` or `node-tuning-operator`, which are
   likely to trigger reboots but are not part of installing or
   upgrading the base OpenShift version?
4. Do we need to programmatically block upgrading single-node
   deployments until they are supported or is it sufficient to rely on
   the documentation and policy that we do not support upgrading dev
   and tech-preview deployments generally?

### Test Plan

In order to claim full support for this configuration, we must have CI
coverage informing the release. An end-to-end job using a single-node
deployment and running an appropriate subset of the standard OpenShift
tests will be created and configured to block accepting release images
unless it passes.

That end-to-end job should also be run against pull requests for the
operators and other components that are most affected by the new
configuration, such as the etcd and auth operators.

There are 2 new CI jobs for testing the results. The
e2e-metal-single-node-live-iso job
(https://github.com/openshift/release/pull/14552) tests using the
bootstrap-in-place approach described in
https://github.com/openshift/enhancements/pull/565 on Packet and
e2e-aws-single-node (https://github.com/openshift/release/pull/14556)
uses AWS to test single-node deployments with a bootstrap VM.

### Graduation Criteria

#### Dev Preview

- Ability to deploy an instance of OpenShift on a single host
- Single-node deployments pass the identified conformance test suite(s)

#### Dev Preview -> Tech Preview

- Gather feedback from users rather than just developers
- Multi-cluster management support for creating single-node
  deployments. The work for that is well outside of the scope of this
  enhancement.

#### Tech Preview -> GA

- Additional time for customer feedback
- Available by default

### Upgrade / Downgrade Strategy

In-place upgrades and downgrades will not be supported for this first
iteration, and will be addressed as a separate feature in another
enhancement. Upgrades will initially only be achieved by redeploying
the machine and its workload.

### Version Skew Strategy

With only one node and no in-place upgrade, there will be no
version skew.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

1. Single-node deployments will not have many of the high-availability
   features that OpenShift users have come to rely on. We will need to
   communicate the relevant limitations and the approaches to deal
   with them clearly.

## Alternatives

### Single-node deployments using a cluster profile

The cluster profile system is purposefully designed to require extra
effort when defining a new profile, to acknowledge the ongoing
maintenance burden. This enhancement is an alternative to the original
proposal to use a profile in
[#504](https://github.com/openshift/enhancements/pull/504).

### Single-node deployments based on static pods

[Enhancement proposal
302](https://github.com/openshift/enhancements/pull/302) describes an
approach for creating the manifests to run a set of static pods to run
the cluster control plane, instead of using operators.

[Enhancement proposal
440](https://github.com/openshift/enhancements/pull/440) builds on 302
and describes another approach for creating a single-node deployment
by having the installer create an Ignition configuration file to
define static pods for the control plane services.

Either approach may be useful for more constrained environments, but
the effort involved - including the effort required to make any
relevant optional software available for this deployment type -
is not obviously worth the resource savings in the less
resource-contrained environments addressed this proposal.

### "Remote workers" for widely-dispersed clusters

A "remote worker" approach - where the worker nodes are separated
from the control plane by significant (physical or network
topological) distance - is appealing because it has the benefit
of reducing the per-site control plane overhead demanded by
autonomous edge clusters.

However, there are drawbacks related to what happens
when those worker nodes lose communication with the cluster control
plane. The most significant problem is that if a node reboots while
it has lost communication with the control plane, it does not
restart any pods it was previously running until communication
is restored.

It's tempting to imagine that this limitation could be addressed
by running the end-user workloads using static pods, but the same
approach would also be needed for per-node control plane components
managed by cluster operators. It would be a major endeavour to
get to the point that all required components - and the workloads
themselves - could all be deployed using static pods that have
no need to communicate with the control plane API.

### Multi-node clusters running on physically smaller hardware

Using blade form-factor servers it could be possible to have more than
one physical server fit in the space currently planned for a single
server, which would allow for multi-node deployments. However, the
specialized hardware involved, especially for telco carrier-grade
networking, make blade servers inadequate for these use cases.

### Containers, but not kubernetes

Workloads that are available in a containerized form factor could be
deployed in a standalone server running a container runtime such as
podman, without the Kubernetes layer on top. However, this would mean
the edge deployments would use different techniques, tools, and
potentially container images, than the centralized sites running the
same workloads on Kubernetes clusters. The extra complexity of having
multiple deployment scenarios is undesirable.
