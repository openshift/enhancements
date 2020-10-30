---
title: single-node-production-edge-cluster-profile
authors:
  - "@dhellmann"
  - "@eranco"
  - "@romfreiman"
  - "@markmc"
reviewers:
  - TBD, probably all leads
approvers:
  - "@derekwaynecarr"
  - "@smarterclayton"
creation-date: 2020-10-15
last-updated: 2020-10-15
status: implementable
see-also:
  - "/enhancements/update/cluster-profiles.md"
  - "/enhancements/single-node-developer-cluster-profile.md"
  - https://github.com/openshift/enhancements/pull/302
  - https://github.com/openshift/enhancements/pull/414
  - https://github.com/openshift/enhancements/pull/440
---

# Single-node Production Edge Cluster Profile

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Add a new `single-node-production-edge` [cluster
profile](https://github.com/openshift/enhancements/blob/master/enhancements/update/cluster-profiles.md)
for *production use* in "edge" deployments on servers that are not
considered to be resource-constrained.

A cluster deployed using this profile will differ from the default
`self-managed-highly-available` cluster profile in several significant
ways:

* The single node serves as both the cluster’s control plane and as a
  worker node.
* Many operators will be configured to reduce the footprint of their
  operands, such as by running fewer replicas.
* In-place upgrades will not be supported by the first iteration of
  this cluster profile.

One example of this use case is seen in telecommunication service
providers implementation of a Radio Access Network (RAN). This use case
is discussed in more detail below.

## Motivation

The benefits of the cloud native approach to developing and deploying
applications is increasingly being adopted in the context of edge
computing. Here we see that as the the distance between an site and the
central management hub grows, the number of servers at the site tends to
shrink. The most distant sites typically have physical space for 1 server.

We are seeing an emerging pattern in which some infrastructure providers and application
owners desire:

1. A consistent deployment approach for their workloads across these
   disparate environments.
3. That the edge sites can operate independantly from the central
   management hub.

And so, these users who have adopted Kubernetes at their their central
management sites wish to have independent Kubernetes clusters at the
more remote sites.

Of the several options explored for supporting the use of Kubernetes
patterns for managing workloads at these sites (see the alternatives
listed blow) a single-node deployment profile of OpenShift is the best
way to give users a consistent experience across all of their sites.

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
* Clusters built using the `single-node-production-edge` profile
  should pass most Kubernetes and OpenShift conformance end-to-end
  tests. Any tests that must be skipped due to differences from a
  multi-node deployment will be documented.

### Non-Goals

* This enhancement does not address single-node deployments in
  highly-constrained environments such as Internet-of-things devices
  or personal computers.
* This enhancement does not address "developer" use cases. See the
  [single-node-developer-profile](https://github.com/openshift/enhancements/pull/302)
  enhancement.
* This enhancement does not address high-availability for single-node
  deployments.
* This enhancement does not address in-place upgrades for this first
  iteration. Upgrades will initially only be achieved by redeploying
  the machine and its workload.
* This enhancement does not attempt to describe a way to "pre-build"
  deployment images, either generically or customized for a user.
* This enhancement does not address the removal of the bootstrap VM,
  although single-node clusters would benefit from that work, which
  will be described in a separate enhancement.

## Proposal

After the profile is introduced, all teams developing OpenShift
components will need to consider how their components should be
configured when deployed and used in the `single-node-production-edge`
deployments.

Although the environment is assumed to have significant resources, it
is important to dedicate most of them to end-user workloads, rather
than cluster control plane or monitoring. Therefore, the cluster
profile will configure telemetry and logging to forward data, instead
of collecting it locally.

Edge deployments are typically part of a large fleet that is managed
automatically rather than one at a time. Therefore the console will
not be deployed by this profile.

The profile describes single-node, all-in-one, deployments, so there
is no need to support provisioning additional workers. The
machine-api-operator and cluster-baremetal-operator will not be
included in clusters using this profile. Remediation (by rebooting or
reprovisioning the host) will be handled by an orchestration tool
running outside of the node.

In the telco RAN use case, high-availability is typically achieved by
having multiple sites provide coverage to overlapping geographic
areas. Therefore, use of cluster-based high-availability features will
be limited (for example, by running a single API service).

Failures in edge deployments are frequently resolved by re-imaging or
physically replacing the entire host. Combining this fact with the
previous observation about the approach to providing highly-available
services lets us draw the conclusion that in-place upgrades do not
need to be supported by the first iteration of this cluster profile.

The cluster profile will be sufficiently generic to cater to a variety
of edge computing use cases. As such, OpenShift's usual cluster
configuration mechanisms will be favored where there is a likelihood
that there will be edge computing use case with differing requirements.
For example, there is no expectation that this new profile will use
the real-time kernel by default - this will continue to be a
`MachineConfig` choice as per
[enhancements/support-for-realtime-kernel](https://github.com/openshift/enhancements/blob/master/enhancements/support-for-realtime-kernel.md). *See open questions*

### User Stories

#### As a user, I can deploy OpenShift in a supported single-node configuration

A user will be able to run the OpenShift installer to create a single-node
deployment, with some limitations (see non-goals above). The user
will not require special support exceptions to receive technical assistance
for the features supported by the configuration.

### Implementation Details/Notes/Constraints

Some OpenShift components (such as Etcd and Ingress) require
a minimum of 2 or 3 nodes. The `single-node-production-edge`
cluster profile will configure these components as appropriate
for a single node.

When we are deploying a cluster with the `single-node-production-edge`
cluster profile, the relevant operators should support a non-HA
configuration that makes the correct adjustments to the deployment
(e.g., `cluster-ingress-operator` should deploy a single router,
`cluster-etcd-operator` should deploy the `etcd-member` [without
waiting for 3 master
nodes](https://github.com/openshift/cluster-etcd-operator/blob/98590e6ecfe282735c4eff01432ae40b29f81202/pkg/etcdenvvar/etcd_env.go#L72))

In addition, some components are not relevant for this cluster profile
(e.g. console, cluster-autoscaler, keepalived for ingressVIP and
apiVIP) and shouldn't be deployed at all.

#### cluster-etcd-operator

By default, `cluster-etcd-operator` will not deploy the etcd cluster
without minimum of 3 master nodes. This can be changed by enabling
`useUnsupportedUnsafeNonHANonProductionUnstableEtcd`.

```shell
# allow etcd-operator to start the etcd cluster without minimum of 3 master nodes
oc patch etcd cluster --type=merge -p="$(cat <<- EOF

 spec:
   unsupportedConfigOverrides:
     useUnsupportedUnsafeNonHANonProductionUnstableEtcd: true
EOF
)"
```

Even with the unsupported feature flag, `etcd-quorum-guard` still
requires 3 nodes due to its replica count. The `etcd-quorum-guard`
Deployment is managed by the `cluster-verison-operator`, so it needs
to be marked as unmanaged before it can be scaled down.

```shell
# tell the cluster-version-operator not to manage etcd-quorum-guard
oc patch clusterversion/version --type='merge' -p "$(cat <<- EOF
 spec:
    overrides:
      - group: apps/v1
        kind: Deployment
        name: etcd-quorum-guard
        namespace: openshift-machine-config-operator
        unmanaged: true
EOF
)"

# scale down etcd-quorum-guard
oc scale --replicas=1 deployment/etcd-quorum-guard -n openshift-etcd
```

#### cluster-authentication-operator

By default, the `cluster-authentication-operator` will not deploy
`OAuthServer` without minimum of 3 master nodes. This can be change by
enabling `useUnsupportedUnsafeNonHANonProductionUnstableOAuthServer`.

```shell
# allow cluster-authentication-operator to deploy OAuthServer without minimum of 3 master nodes
oc patch authentications.operator.openshift.io cluster --type=merge -p="$(cat <<- EOF

 spec:
   managementState: "Managed"
   unsupportedConfigOverrides:
     useUnsupportedUnsafeNonHANonProductionUnstableOAuthServer: true
EOF
)"
```

#### cluster-ingress-operator

By default, the `cluster-ingress-operator` deploys the router with 2
replicas. On a single node one will fail to start and the ingress will
show as degraded.

```shell
# patch ingress operator to run a single router pod
oc patch -n openshift-ingress-operator ingresscontroller/default --type=merge --patch '{"spec":{"replicas": 1}}'
```

#### machine-config-operator

The machine-config-operator includes a check in code that the number
of control plane nodes is 3. Removing this or changing the minimum is
required to allow the operator to complete its work, for example to
enable the realtime kernel on a single node with the appropriate
performance profile settings.

### Risks and Mitigations

*What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.*

*How will security be reviewed and by whom? How will UX be reviewed and by whom?*

## Design Details

### Open Questions

1. Telco workloads frequently require a realtime kernel. How will a
   user specify whether to use the realtime or regular kernel? Should
   we assume they always want the realtime version?
2. Similarly, telco workloads typically require special network setups
   for a host to boot, including bonded interfaces, access to multiple
   VLANs, and static IPs. How do we anticipate configuring those?
3. The machine-config-operator works by (almost always) rebooting a
   host.  Is that going to be OK in these single-node deployments?
   MCO is used by the performance-addon-operator and the
   network-tuning-operator to apply the computed host OS and kernel
   tuning values.  It is also used to allocate hugepages.  Do we want
   it to run in a different mode where reboots are not performed?

### Test Plan

In order to claim full support for this configuration, we must have
CI coverage informing the release. An end-to-end job using the profile
and running an appropriate subset of the standard OpenShift tests
will be created and configured to block accepting release images
unless it passes.

That end-to-end job should also be run against pull requests for
the operators and other components that are most affected by the new
profile, such as the etcd and auth operators.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
    - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
    - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

#### Examples

These are generalized examples to consider, in addition to the aforementioned [maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

In-place upgrades and downgrades will not be supported for this first
iteration. Upgrades will initially only be achieved by redeploying
the machine and its workload.

### Version Skew Strategy

With only one node and no in-place upgrade, there will be no
version skew.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

1. Clusters built using this profile will not have many of the high-availability
   features that OpenShift users have come to rely on. We will need to communicate
   the relevant limitations and the approaches to deal with them clearly.

## Alternatives

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

Workloads that are available in a containerized form factor could
be deployed in a standalone server running a container runtime,
without the Kubernetes layer on top. However, this would mean the
edge deployments would use different techniques, tools, and
potentially container images, than the centralized sites running
the same workloads on Kubernetes clusters. The extra complexity of
having multiple deployment scenarios is undesirable.
