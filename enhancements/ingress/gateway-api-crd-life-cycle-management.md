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
status: provisional
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
CRDs. As such, OpenShift must manage the life-cycle of these CRDs in a way that
enables Red Hat to support the product, minimizes friction for the end-user,
provides consistency, and enables other implementations that rely on these
CRDs.

## Motivation

Gateway API is an evolving API, and its CRDs change over time.  For core
OpenShift, we must ensure that we have a version of the CRDs that is compatible
with our product, and we must handle upgrading these CRDs for our own needs.  We
must also avoid unnecessarily causing friction for Red Hat layered products,
partner products, and other third-party implementations.

### User Stories

#### Using OpenShift's Gateway API implementation on a new OpenShift cluster

As a cluster-admin, I want to install a new OpenShift 4.19 cluster, and then use
OpenShift's Gateway API implementation to configure ingress to a workload on
this cluster without having to deploy and then manage the lifecycle of any CRDs
thereafter.

#### Using OpenShift's Gateway API implementation on an upgraded cluster

As a cluster-admin, I want to upgrade my cluster from OpenShift 4.18 (which
*doesn't* manage the Gateway API CRDs' life-cycle) to OpenShift 4.19 (which
*does*). If a user on my cluster or I had previously installed Gateway API
CRDs and was managing their lifecycle, then I need OpenShift to explicitly
confirm consent from me to take over the management of these CRDs to avoid
disruptions to existing workloads.

#### Using a third-party Gateway API implementation

As a cluster-admin, I want to install a third-party Gateway API implementation
on my OpenShift 4.19 cluster, and use the third-party implementation without
any interference from the first-party implementation. Relatedly, I want to be
able to utilize both the first-party and any third-party solution alongside
each other simultaneously and independently without any interference between the
two.

#### Future OpenShift upgrades

As a cluster-admin, I want to be able to receive updates to the Gateway API
resources via zstream (and major) releases to add new features and capabilities.

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

## Proposal

### CRD Life-cycle management

OpenShift's [Cluster Ingress Operator (CIO)] will manage the _entire_
life-cycle of the Gateway API CRDs from here onward. The CIO will be packaged
along with a [Validating Admission Policy (VAP)] to block updates from sources
other than the CIO, and will hold the CRDs at a specific `$CRD_VERSION`. In
effect, this means that the **Gateway API resources will now be treated like a
core API.**

[Cluster Ingress Operator (CIO)]:https://github.com/openshift/cluster-ingress-operator
[Validating Admission Policy (VAP)]:https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/

#### CRD Deployment

The CIO will now check for the presence of the CRDs on the cluster, for
which there are two scenarios:

1. The CRDs are not present, so we create them
2. The CRDs are already present, so we need to take over management

The former is extremely straightforward and the correct version of the CRDs
will be applied.

The latter situation has more complexities. We'll refer to this process as "CRD
Management Succession", and cover its implications and logic below.

#### CRD Management Succession

Taking over the management of the Gateway API CRDs from a previous entity
can be tumultuous as we will require:

* no other actors managing the CRDs from here on out
* only standard CRDs can be deployed (no experimental)
* the CRDs to be deployed at an exact version/schema
* OR the CRDs not to be present at all

We will require this by providing a pre-upgrade check in the previous release
that verifies these are true and sets `Upgradable=false` if any of them are not.

> **Note**: Upstream Gateway API unfortunately layered experimental versions of
> CRDs on top of the same [Group/Version/Kind (GVK)] as the standard ones, so
> unfortunately for initial release users will be unable to use experimental as
> we can't deliver that as a part of our supported API surface. It will be
> feasible to add a gate to enable experimental later, but because of this
> layering this will not be a great user experience and will require the cluster
> to become tainted. We are tracking and supporting [an effort] in upstream
> Gateway API to separate experimental into its own group, which we expect to
> help move towards a better overall experience for users who want experimental
> Gateway API features going forward.

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

> **Note**: Even if they force through, we will take over the CRDs once the
> upgrade completes (or go `Degraded` if that fails for some reason).

[Group/Version/Kind (GVK)]:https://book.kubebuilder.io/cronjob-tutorial/gvks.html
[an effort]:https://github.com/kubernetes-sigs/gateway-api/discussions/3497

### Workflow Description

The workflow in this case is an upgrade process. From the _user_ perspective the
CRDs will be fully managed via the platform from here on out, so they only need
to interface with the upgrade workflow on the condition that their cluster had
previously installed Gateway API CRDs. The workflow consists of the pre-upgrade
checks and the post-upgrade checks.

### Pre-upgrade

1. In the CIO a pre-upgrade check verifies CRD presence
  * IF the CRDs are present
    * an admingate is created requiring acknowledgement of CRD succession
    * UNTIL the schema exactly matches the version we provide we set `Upgradable=false`
2. Once CRDs are not present OR are an exact match we set `Upgradable=true`

> **Note**: A **cluster-admin** is required for these steps.

> **Note**: The logic for these lives in in the previous release (4.18), but
> does not need to be carried forward to future releases as other logic exists
> there to handle Gateway API CRD state (see below).

### Post-upgrade

> **Note**: The logic for these lives in in the new release (4.19) and onward.

1. The CIO is hereafter deployed alongside its CRD protection [Validating Admission Policy (VAP)]
2. The CIO constantly checks for the presence of the CRDs
  * IF the CRDs are present
    * UNTIL the CRD schema matches what is expected, the CIO upgrades them
    * IF the upgrade fails `Degraded` status is set
  * ELSE the CRDs are deployed by the CIO

> **Note**: If we reach `Degraded` its expected that some tampering has
> occurred (e.g. a cluster-admin has for some reason destroyed our VAP and
> manually changed the CRDs). For the initial release we will simply require
> manual intervention (support) to fix this as we can't guess too well at the
> original intent behind the change. In future iterations we may consider more
> solutions if this becomes a common problem for some reason.

[Validating Admission Policy (VAP)]:https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/

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

The Cluster Ingress Operator (CIO) will be used to deploy and manage the
lifecycle of the Gateway API CRDs for applicable clusters. CRDs and their
management are not particularly resource intensive.

The CIO covers Single Node Deployments. MicroShift however, does not run the
CIO has its own design for supporting Gateway API (see the [MicroShift Gateway
API Support Enhancement]).

[MicroShift Gateway API Support Enhancement]:../microshift/gateway-api-support.md

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
[gateway-api#3624], which goes into details about the effects on Gateway API
implementations, and what can generally go wrong. Since this problem does not
have any standards or solutions defined in upstream at the time of writing we
will address this by pinning to one specific version of the Gateway API CRDs
that is tested and vetted by us to ensure "dead fields" will not cause harm to
users of the corresponding version of our first-party implementation. When the
upstream community provides standards and solution around this we will implement
that solution.

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
of the platform version, will be lost. Third party implementations will not be
able to bring their own API changes.

> **Note**: The API has stabilized, so it's important to note that there is
> little expectation of any rapid change.

> **Note**: While third party implementations can not bring their own API
> changes directly, if the need for more flexibility arises we may consider
> adding functionality in future iterations that enables the platform to more
> dynamically set the version deployed in the cluster.

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

## Open Questions [optional]

> This is where to call out areas of the design that require closure before deciding
> to implement the design.  For instance,
>  > 1. This requires exposing previously private resources which contain sensitive
>   information.  Can we do this?

_TBD_

## Test Plan

> **Note:** *Section not required until targeted at a release.*
> 
> Consider the following in developing a test plan for this enhancement:
> - Will there be e2e and integration tests, in addition to unit tests?
> - How will it be tested in isolation vs with other components?
> - What additional testing is necessary to support managed OpenShift service-based offerings?
> 
> No need to outline all of the test cases, just the general strategy. Anything
> that would count as tricky in the implementation and anything particularly
> challenging to test should be called out.
> 
> All code is expected to have adequate tests (eventually with coverage
> expectations).

The Ingress Operator will have E2E tests to simulate the user stories outlined
in this EP:

- Delete the CRDs; verify that the operator re-installs them.
- Install incompatible CRDs with `metadata.managedFields` set to indicate that the operator *did not* install them; verify that the operator reports the appropriate `Degraded` status.
- Install older CRDs with `metadata.managedFields` set to indicate that an older version of the operator *did* install them; verify that the operator updates them.

_TBD_

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

## Version Skew Strategy

> How will the component handle version skew with other components?
> What are the guarantees? Make sure this is in the test plan.
> 
> Consider the following in developing a version skew strategy for this
> enhancement:
> - During an upgrade, we will always have skew among components, how will this impact your work?
> - Does this enhancement involve coordinating behavior in the control plane and
>   in the kubelet? How does an n-2 kubelet without this feature available behave
>   when this feature is used?
> - Will any other components on the node change? For example, changes to CSI, CRI
>   or CNI may require updating that component before the kubelet.

_TBD: Do we describe version skew with layered products here?_

## Operational Aspects of API Extensions

> Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
> admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
> especially how they impact the OCP system architecture and operational aspects.
> 
> - For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
>   Indicators) an administrator or support can use to determine the health of the API extensions
> 
>   Examples (metrics, alerts, operator conditions)
>   - authentication-operator condition `APIServerDegraded=False`
>   - authentication-operator condition `APIServerAvailable=True`
>   - openshift-authentication/oauth-apiserver deployment and pods health
> 
> - What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
>   API availability)
> 
>   Examples:
>   - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
>   - Fails creation of ConfigMap in the system when the webhook is not available.
>   - Adds a dependency on the SDN service network for all resources, risking API availability in case
>     of SDN issues.
>   - Expected use-cases require less than 1000 instances of the CRD, not impacting
>     general API throughput.
> 
> - How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
>   automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
>   this enhancement)
> 
> - Describe the possible failure modes of the API extensions.
> - Describe how a failure or behaviour of the extension will impact the overall cluster health
>   (e.g. which kube-controller-manager functionality will stop working), especially regarding
>   stability, availability, performance and security.
> - Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
>   and add them as reviewers to this enhancement.

_TBD: Do we need to describe anything here?_

## Support Procedures

### Conflicting CRDs

If the Ingress Operator detects the presence of a conflicting version of the
Gateway API CRDs, it updates the ingress clusteroperator to report a `Degraded`
status condition with status `True` and a message explaining the situation:

_TBD: Insert example output from `oc get clusteroperators/ingress -o yaml`._

In this situation, the cluster-admin is expected to verify that workload would
not be broken by handing life-cycle management of the CRDs over to the Ingress
Operator:

_TBD: Insert `oc` command to make the CRD ownership transition._

Then the Ingress Operator takes ownership and updates the CRDs:

_TBD: Insert example `oc get clusteroperators` and `oc get crds` commands._

### Overriding the Ingress Operator

_TBD: Should we describe how to turn off the Ingress Operator so that the
cluster-admin can override the CRDs, or describe how Server-Side Apply enables
the cluster-admin to take over the CRDs?_

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

### Admission webhook

An admission webhook could be implemented for the Gateway API CRDs to prevent
writes to any field that is in the CRD but isn't implemented by our Gateway API
implementation.

Note that using a webhook for this purpose would run into consistency issues and
race conditions because the webhook would need to cross-validate multiple
resources.  Specifically, the webhook would need to check which gatewayclasses
specified our controller name; then the webhook would check *only* resources
(gateways, httproutes, etc.) associated with those gatewayclasses for fields
that our controller would not recognize.  Consistency issues could arise, for
example, if an object were created and subsequently updated to reference a
gatewayclass, or if a gatewayclass were created (or its controller name were
updated) after resources that referenced that gatewayclass by name had already
been created.

_TBD: Fill in details._

We conclude that this can be further evaluated and, if appropriate, implemented
post-GA if the need arises to allow newer CRD versions than the version that our
Gateway API implementation recognizes.

### Provide an API for explicitly overriding CRD life-cycle management

Inspired by CAPI's mechanism.  This could be useful in a procedure for upgrading
a cluster with cluster-admin-owned CRDs to a cluster with operator-managed CRDs.

_TBD: Fill in details._

### Validate and allow a range of CRD versions

As a way to offer more flexibility for third-party implementations, we
considered defining a range of allowed CRD versions.  On the one hand, this
approach has the minor advantage of avoiding dead fields unless the
cluster-admin *needs* a newer CRD that has dead fields.

On the other hand, allowing a range adds complexity, requires more testing, and
still must be constrained to avoid security-problematic dead fields.  This added
complexity has questionable value and could delay the feature.  Therefore we
conclude that it is best to pin the CRDs to a specific version for at least the
initial GA release of the Gateway API feature.

### Package CRDs as operator manifests that Cluster Version Operator owns

_TBD_

## Infrastructure Needed [optional]

No new infrastructure is required for this enhancement.
