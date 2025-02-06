---
title: gateway-api-crd-life-cycle-management
authors:
  - "@Miciah"
  - "@shaneutt"
reviewers:
  - "@shaneutt" # Gateway API maintainer.
  - "@dgn" # OSSM maintainer.
  - "@JoelSpeed" # SME for a similar issue and solution in CAPI.
  - "@knobunc" # Staff engineer for Networking, involved in past discussions.
approvers:
  - "@JoelSpeed"
  - "@knobunc"
api-approvers:
  - None
creation-date: 2025-01-22
last-updated: 2025-01-27
tracking-link:
  - https://issues.redhat.com/browse/NE-1946
see-also:
  - "/enhancements/ingress/gateway-api-with-cluster-ingress-operator.md"
---


# Gateway API CRD Life-Cycle Management

This enhancement describes how the [Cluster Ingress Operator (CIO)] manages
Gateway API Custom Resource Definitions (CRDs).

[Cluster Ingress Operator (CIO)]:https://github.com/openshift/cluster-ingress-operator

## Summary

A key goal of Gateway API is to nurture a diverse ecosystem of implementations.
These implementations, including OpenShift's, all rely on the same Gateway API
CRDs.  To these ends, OpenShift must manage the life-cycle of these CRDs in a
way that enables Red Hat to support the product, minimizes friction for the
end-user, provides consistency, and avoids causing conflicts with other
implementations that rely on these CRDs.

## Motivation

Gateway API is an evolving API, and its CRDs change over time.  For core
OpenShift, we must ensure that we have a version of the CRDs that is compatible
with our product, and we must handle upgrading these CRDs for our own needs.  We
must also avoid unnecessarily causing friction for Red Hat layered products,
partner products, and other third-party implementations.

### User Stories

#### Using OpenShift's Gateway API implementation on a new OpenShift cluster

As a cluster-admin, I want to install a new OpenShift 4.19 cluster, and then use
OpenShift's Gateway API implementation to configure ingress to workload on this
cluster without having to deploy and then manage the lifecycle of any CRDs
thereafter.

#### Using OpenShift's Gateway API implementation on an upgraded cluster

As a cluster-admin, I want to upgrade my cluster from OpenShift 4.18 (which
*doesn't* manage the Gateway API CRDs' life-cycle) to OpenShift 4.19 (which
*does*). If a user on my cluster or I myself had Gateway API CRDs installed
previously and was managing their lifecycle, I need OpenShift to explicitly
confirm consent from me to take over the management of these CRDs to avoid
disruptions to existing workloads.

#### Using a third-party Gateway API implementation

As a cluster-admin, I want to install a third-party Gateway API implementation
on my OpenShift 4.19 cluster, and use the third-party implementation without
any interference from the first-party implementation. Relatedly I want to be
able to utilize both the first-party and any third-party solution alongside
each other simultaneously and independently without any interference between the
two.

#### Future OpenShift upgrades

As a cluster-admin, I want to upgrade my cluster from OpenShift 4.19 (which
manages the Gateway API CRDs' life-cycle) to OpenShift 4.20 (which includes a
newer version of these CRDs).

### Goals

- Ensure Gateway API CRDs are installed on new and upgraded OpenShift 4.19 clusters.
- Ensure the installed Gateway API CRDs are compatible with OpenShift's needs.
- Protect against bad updates or removals of the Gateway API CRDs.
- Detect and warn if an incompatible version of the CRDs is installed.
- Protect users against "unknown fields"/"dead fields" (defined below).
- Provide a method to transfer ownership of previously existing CRDs to when upgrading from 4.18 to 4.19.

### Non-Goals

- _Automatically_ replace incompatible CRDs that some other agent installed.
- Provide an explicit override for the cluster-admin to take CRD ownership.
- Solve CRD life-cycle management in OLM.
- Solve OLM subscription management.
- Solve CRD life-cycle management for OSSM or Istio resources.

## Proposal

### CRD Life-cycle management

OpenShift's [Cluster Ingress Operator (CIO)] will manage the _entire_
life-cycle of the Gateway API CRDs from here onward. The CIO will be packaged
along with a [Validating Admission Policy (VAP)] to block updates from sources
other than the CIO, and will hold the CRDs at a specific `$CRD_VERSION`. In
effect, this means that the **Gateway API resources will now be treated like a
core API.**

> **Note**: the `$CRD_VERSION` selected for any OCP release version will be
> based on a corresponding release of [OpenShift Service Mesh (OSSM)] which is
> the implementation of Gateway API we will be using for first party Gateway
> API support on the cluster.

[Cluster Ingress Operator (CIO)]:https://github.com/openshift/cluster-ingress-operator
[Validating Admission Policy (VAP)]:https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/
[OpenShift Service Mesh (OSSM)]:https://github.com/openshift-service-mesh

#### CRD Deployment

The CIO will now check for the presence of the CRDs on the cluster, for
which there are two scenarios:

1. The CRDs are not present, so we create them
2. The CRDs are already present, so we need to take over management

The former is extremely straightforward and the correct version of the CRDs
will be applied.

The latter situation has more complexities. We'll refer to this process as "CRD
Management Succession", and cover it's implications and logic below.

#### CRD Management Succession

Taking over the management of the Gateway API CRDs from a previous entity
can be turmultuous as we will require:

* no other actors managing the CRDs from here on out
* only standard CRDs can be deployed (no experimental)
* the CRDs to be deployed at an exact version/schema
* OR the CRDs not to be present at all

We will require this by providing a pre-upgrade check in the previous release
that verifies these are true and sets `Upgradable=false` if any of them are not.

> **Note**: Upstream Gateway API unfortunately layered experimental versions of
> CRDs on top of the same GVK as the standard ones, so unfortunately for initial
> release users will be unable to use experimental as we can't deliver that as
> a part of our supported API surface. It will be feasible to add a gate to
> enable experimental later, but because of this layering this will not be a
> great user experience and will require the cluster to become tainted. We are
> tracking and supporting [an effort] in upstream Gateway API to separate
> experimental into its own group, which we expect to help move towards a better
> overall experience for users who want experimental Gateway API features going
> forward.

**We simply can not anticipate all of the negative effects** succession will
have on existing implementations if the cluster admin forces through the upgrade
check. As an extra precaution for users forcing upgrades precipitously, we will
provide an admin gate to both provide an extra warning and gather consent from
the cluster admin to take over the management of the CRDs. This will be
accompanied by detailed documentation on what the admin should check and how to
fufill the checks, which will also be linked from the description provided in
the admin gate.

The admin will be responsible for ensuring the safety of succession. If they
force through the pre-upgrade check AND the admin gate, the admin gate will be
left behind to help aid investigation of the cause of problems when the upgrade
goes wrong.

[an effort]:https://github.com/kubernetes-sigs/gateway-api/discussions/3497

### Workflow Description

> Explain how the user will use the feature. Be detailed and explicit.  Describe
> all of the actors, their roles, and the APIs or interfaces involved. Define a
> starting state and then list the steps that the user would need to go through to
> trigger the feature described in the enhancement. Optionally add a
> [mermaid](https://github.com/mermaid-js/mermaid#readme) sequence diagram.
>
> Use sub-sections to explain variations, such as for error handling,
> failure recovery, or alternative outcomes.

**cluster-admin** is a human user responsible for managing a cluster.

1. Start with a 4.18 cluster with conflicting CRDs.
2. Upgrade to 4.19.
3. Check clusteroperators, see a conflict.
4. Run some `oc` command.
5. Check the ingress clusteroperator again.  Now everything should be dandy.

### API Extensions

None.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hypershift runs the Ingress Operator on the management cluster but configured
with a kubeconfig to manage resources on the guest cluster.  This means that the
Ingress Operator manages the Gateway API CRDs on the guest cluster, the same as
on standalone clusters.

#### Standalone Clusters

For standalone clusters, the Ingress Operator manages the Gateway API CRD
life-cycle so that Gateway API can be configured and used post-install.

#### Single-node Deployments or MicroShift

The CRDs themselves use minimal resources.  Creating a GatewayClass CR can cause
the Ingress Operator to install OpenShift Service Mesh, which in turn installs
Istio and Envoy (see the [gateway-api-with-cluster-ingress-operator](gateway-api-with-cluster-ingress-operator.md)
enhancement proposal), which use considerable resources.  For Single-Node
OpenShift, the cluster-admin might be advised to pay particular attention to
OSSM's resource requirements and the cluster's resource constraints before
attempting to use Gateway API.

MicroShift does not run the Ingress Operator and has its own design for
supporting Gateway API (see the MicroShift [gateway-api-support](../microshift/gateway-api-support.md)
enhancement).

### Implementation Details/Notes/Constraints

> What are some important details that didn't come across above in the
> **Proposal**? Go in to as much detail as necessary here. This might be
> a good place to talk about core concepts and how they relate. While it is useful
> to go into the details of the code changes required, it is not necessary to show
> how the code will be rewritten in the enhancement.

### Risks and Mitigations

#### Unknown Fields

> **Note**: aka "dead fields"

The definition of the "unknown fields" problem is provided upstream in
[gateway-api#3624]. Since this problem does not have any standards or solutions
defined in upstream at the time of writing we will address this by pinning to
one specific version of the Gateway API CRDs that is tested and vetted by us to
ensure freedom from "dead fields" with the corresponding version of our
first-party implementation. When the upstream community provides standards and
solution around this we will implement that solution.

[gateway-api#3624]:https://github.com/kubernetes-sigs/gateway-api/issues/3624

### Drawbacks

> The idea is to find the best form of an argument why this enhancement should
> _not_ be implemented.
>
> What trade-offs (technical/efficiency cost, user experience, flexibility,
> supportability, etc) must be made in order to implement this? What are the reasons
> we might not want to undertake this proposal, and how do we overcome them?
>
> Does this proposal implement a behavior that's new/unique/novel? Is it poorly
> aligned with existing user expectations?  Will it be a significant maintenance
> burden?  Is it likely to be superceded by something else in the near future?

The key tradeoff is that we will start treating Gateway API effectively as if
it were a core API. This means any perceived value someone might see in being
able to manage and upgrade the Gateway API resources themselves, independently
of the platform version, will be lost.

> **Note**: This "downside" can actually be an upside for integrators, as we've
> heard from projects which have Gateway API integrations (such as OSSM) which
> appreciate having a consistent and known version of Gateway API on the
> cluster which they can always rely on being there.

Unfortunately this is not really an avoidable problem, as it is not tenable for
us to simultaneously say that we support Gateway API as a primary and fully
supported API surface for ingress traffic, and also have no control over which
version of the APIs are present, or if they are even present at all (e.g. the
cluster admin decides to delete them).

To make this situation a bit easier we do anticipate providing some updates
after the initial release. We want to eventually allow version _ranges_ in
time after we resolve the "dead fields" problem (see more in the section about
this above) which we expect to provide significantly more flexibility and take
care of many concerns that would come from this change. We are also tracking and
supporting [upstream efforts] to separate experimental APIs out into their own
group, which will provide more flexibility when users want experimental features.

[upstream efforts]:https://github.com/kubernetes-sigs/gateway-api/discussions/3497

## Test Plan

The Ingress Operator will have tests to validate the intended functionality
outlined in this EP:

- CIO Tests that verify the pre-upgrade check logic
- CIO Tests that verify the post-upgrade logic:
    - Delete the CRDs; verify that the operator re-installs them
    - Install incompatible CRDs; verify that the operator reports `Degraded`
    - Install experimental CRDs; verify that the operator reports `Degraded`
    - Verify none of the above can even be done with a present VAP

## Graduation Criteria

Note that Gateway API CRD life-cycle management will be part of the
[gateway-api-with-cluster-ingress-operator](gateway-api-with-cluster-ingress-operator.md) enhancement, which defines its
own graduation criteria.

### Dev Preview -> Tech Preview

N/A.

### Tech Preview -> GA

- End-to-end tests in the Ingress Operator.
- Upgrade testing with conflicting CRDs.
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

N/A.

## Version Skew Strategy

> **Note**:see operational aspects of API extensions below.

## Upgrade / Downgrade Strategy

> If applicable, how will the component be upgraded and downgraded? Make sure this
> is in the test plan.
>
> Consider the following in developing an upgrade/downgrade strategy for this
> enhancement:
> - What changes (in invocations, configurations, API use, etc.) is an existing
>   cluster required to make on upgrade in order to keep previous behavior?
> - What changes (in invocations, configurations, API use, etc.) is an existing
>   cluster required to make on upgrade in order to make use of the enhancement?
>
> Upgrade expectations:
> - Each component should remain available for user requests and
>   workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
>   disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
>   this should be identified and discussed here.
> - Micro version upgrades - users should be able to skip forward versions within a
>   minor release stream without being required to pass through intermediate
>   versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
>   as an intermediate step.
> - Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
>   steps. So, for example, it is acceptable to require a user running 4.3 to
>   upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
> - While an upgrade is in progress, new component versions should
>   continue to operate correctly in concert with older component
>   versions (aka "version skew"). For example, if a node is down, and
>   an operator is rolling out a daemonset, the old and new daemonset
>   pods must continue to work correctly even while the cluster remains
>   in this partially upgraded state for some time.
>
> Downgrade expectations:
> - If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
>   misbehaving, it should be possible for the user to rollback to `N`. It is
>   acceptable to require some documented manual steps in order to fully restore
>   the downgraded cluster to its previous state. Examples of acceptable steps
>   include:
>   - Deleting any CVO-managed resources added by the new version. The
>     CVO does not currently delete resources that no longer exist in
>     the target version.

## Operational Aspects of API Extensions

Other products and components that have Gateway API support will now be able to
consistently know that Gateway API will already be present on the cluster, and
which version will be present given the version of OpenShift. There will no
longer be a need for them to document having their users deploy the CRDs
manually or do any management themselves that could conflict.

We are already aware of several projects which utilize Gateway API including
(but not limited to):

* OpenShift Service Mesh
* Kuadrant
* OpenShift AI Serving

We will coordinate with these projects and others from release to release on
their needs related to Gateway API version support. We expect over time that
more flexibility with the version will eventually be needed, and we anticipate
adding ranges of support instead of specific versions to accomodate this.

## Support Procedures

### Conflicting CRDs

The pre-upgrade checks should eliminate any problems with CRD conflicts.
However it is always _technically possible_ for the admin to force through both
the pre-upgrade check AND the admin gate. If they do this the CIO will detect
the mismatching schema and report a `Degraded` status condition with status
`True` and a message explaining the problem.

In this situation the cluster-admin then has to go back and follow the upgrade
instructions regarding Gateway API CRDs correctly and fix the state on the
cluster before we can move out of degraded.

## Alternatives

### Use an admin-ack gate to block upgrades if incompatible CRDs exist

One option considered was to add logic in OpenShift 4.18's Ingress Operator to
detect conflicting Gateway API CRDs.  This logic would block upgrades from 4.18
to 4.19 if conflicting CRDs were detected.  Then 4.19's Ingress Operator could
unconditionally take ownership of the CRDs' life-cycle.

This has the advantage of providing a warning *before* upgrade.  In contrast,
the solution proposed in this enhancement allows the upgrade and then reports a
`Degraded` status condition *after* the upgrade.  However, in either case, the
cluster-admin is responsible for resolving the conflict.  Thus the admin-ack
gate adds complexity without significantly improving the user experience.

We conclude that the effort of adding an admin-ack gate isn't worth the effort.

### Use a fleet evaluation condition to detect clusters with incompatible CRDs

Another option considered was to add a [fleet evaluation condition](../dev-guide/cluster-fleet-evaluation.md) to tell
us how many clusters have conflicting CRDs already installed.  This could help
us decide whether implementing upgrade-blocking logic (such as the
aforementioned admin-ack gate) would be beneficial.  However, given time
constraints, and given that we need to handle conflicts in any case, we have
concluded that the fleet evaluation condition would not be of much benefit.

### Validate and allow a range of CRD versions

As a way to offer more flexibility for third-party implementations, we
considered defining a range of allowed CRD versions.  On the one hand, this
approach has the minor advantage of avoiding dead fields unless the
cluster-admin *needs* a newer CRD that has dead fields.

On the other hand, allowing a range adds complexity, requires more testing, and
still must be constrained to avoid security-problematic dead fields.  This added
complexity has questionable value and could delay the feature.  Therefore we
conclude that it is best to pin the CRDs to a specific version for at least the
initial GA release of the Gateway API feature, but we expect in time we will
iterate into having more flexible ranges.

### Package CRDs as operator manifests that Cluster Version Operator owns

We considered whether we would package the CRD manifests with the CVO, but this
approach has downsides in terms of how we develop management logic for the CIO
and ultimately did not appear to have enough upsides.

## Infrastructure Needed [optional]

No new infrastructure is required for this enhancement.
