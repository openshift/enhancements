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
any interference from the OpenShift implementation. Relatedly, I want to be
able to utilize both the first-party and any third-party solution alongside
each other simultaneously and independently without any interference between the
two.

#### Future OpenShift upgrades

As a cluster-admin, I want to be able to receive updates to the Gateway API
resources via zstream (and major) releases to add new features and capabilities.

### Goals

- Establish the owner of Gateway API CRDs as OpenShift Cluster Ingress Operator (CIO).
- Prevent upgrades on OpenShift Clusters that have incompatible Gateway API CRDs already installed.
- Ensure Gateway API CRDs are installed on new and upgraded OpenShift 4.19 clusters.
- Ensure the installed Gateway API CRDs are compatible with OpenShift's needs.
- Protect against bad updates or removals of the Gateway API CRDs.
- Detect and warn if an incompatible version of the CRDs is installed.
- Protect users against "unknown fields"/"dead fields" (defined below).
- Provide a method to transfer ownership of previously existing CRDs to OpenShift Ingress Cluster Operator when upgrading from 4.18 to 4.19.

### Non-Goals

- Provide an explicit override for the cluster-admin to take CRD ownership.
- Solve CRD life-cycle management in OLM.
- Solve OLM subscription management.

## Proposal

### CRD Life-cycle management

OpenShift's [Cluster Ingress Operator (CIO)] will manage the _entire_
life-cycle of the Gateway API CRDs from here onward. The CIO will be packaged
along with a [Validating Admission Policy (VAP)] to block updates from sources
other than the CIO, and will hold the CRDs at a specific version. In
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
can be tumultuous as we will require the following:

* no other actors to be managing the CRDs from here on out,
* AND only standard CRDs to be deployed (no experimental),
* AND the CRDs to be deployed at an exact version/schema;
* OR the CRDs not to be present at all.

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

**We simply cannot anticipate all of the negative effects** succession will
have on existing implementations if the cluster admin forces through the upgrade
check. As an extra precaution for users forcing upgrades precipitously, we will
provide an admin gate to both provide an extra warning and gather consent from
the cluster admin to take over the management of the CRDs. This will be
accompanied by detailed documentation on what the admin should check and how to
fufill the checks, which will also be linked from the description provided in
the admin gate. An "unsupported config override" will be used to flag
exceptions.

The admin will be responsible for ensuring the safety of succession. If they
force through the pre-upgrade check AND the admin gate, the admin gate will be
left behind to help aid investigation of the cause of problems when the upgrade
goes wrong.

> **Note**: Even if they force through, we will take over the CRDs once the
> upgrade completes (or go `Degraded` if that fails for some reason) unless
> they are in "unsupported config override" mode.

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
    * IF there are unexpected CRDs (because the VAP was bypassed), the operator reports `Degraded` and logs.
    * UNTIL the CRD schema matches what is expected, the CIO upgrades the otherwise expected CRDs
    * IF the upgrade fails persistently, `Degraded` status is set
  * ELSE the CRDs are deployed by the CIO

> **Note**: If we reach `Degraded`, it's expected that some tampering must have
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

The CIO covers Single Node Deployments for some operations, but not CIO on MicroShift.  MicroShift has its own design for supporting Gateway API (see the [MicroShift Gateway
API Support Enhancement]).

[MicroShift Gateway API Support Enhancement]:../microshift/gateway-api-support.md

### Implementation Details/Notes/Constraints

The Gateway API CRDs will be hosted as YAML manifests alongside several
manifests the Cluster Ingress Operator (CIO) hosts, and will be deployed
via the relevant controller logic.

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
upstream community provides standards and solution around this we will evaluate
and possibly implement that solution.

> **Warning**: For a third party implementation, that does not support the
> version of our Gateway API CRDs, it may be possible for a user to specify a
> field, that has no effect immediately, but takes effect once the
> implementation is later upgraded. This could be confusing for end users, or
> could break their ingress during the upgrade, or it could even lead the user
> to incorrectly believe that their their endpoints are secure where the dead
> field is security related. Notably third party implementations do not have a
> great way to protect themselves from dead fields until [gateway-api#3624] is
> resolved. Until that is resolved, they will have to come up with a custom
> solution. We will provide documentation specific to third party Gateway API
> integrations which highlights this problem and suggests that their
> implementations (e.g. API clients, controllers, etc) should be checking and
> validating the JSON schema of what is actually provided by the API versus
> the schema they expect and are aware of.

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

### Dev Preview -> Tech Preview

This stage covers basic management for the Gateway API CRDs behind the feature
gate, but without the [Validating Admission Policy (VAP)], allowing them to be
disabled and for someone else to deploy and manage them.

[Validating Admission Policy (VAP)]:https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/

### Tech Preview -> GA

This stage covers the CRD version bump, the [Validating Admission Policy (VAP)]
is added (blocking anyone but the platform from deploying or managing Gateway
API CRDs), and the pre-upgrade and post-upgrade checks to ensure CRD management
succession are in place in the Cluster Ingress Operator (CIO).

[Validating Admission Policy (VAP)]:https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/

### Dev Preview -> Tech Preview

N/A.

### Tech Preview -> GA

- End-to-end tests in the Ingress Operator.
- Upgrade testing with conflicting CRDs.
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

The previous technical preview code for managing Gateway API CRDs will be
removed in favor of the new logic in the CIO.

## Version Skew Strategy

> **Note**:see operational aspects of API extensions below.

## Upgrade / Downgrade Strategy

Upgrading is straightforward as Gateway API is stable and will not be making
any backwards incompatible changes in major versions from here on out.

Downgrading involves the user performing the downgrade, and then applying the
previous versions of the CRDs to the cluster which they had previously.
Therefore cluster admins who had the CRDs previously installed need to take
backups or record their Gateway API CRD resources prior to the upgrade in order
for a downgrade to be successful.

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
the mismatch and will overwrite to the intended version.

## Alternatives

### Use a fleet evaluation condition to detect clusters with incompatible CRDs

Another option considered was to add a [fleet evaluation condition](../../dev-guide/cluster-fleet-evaluation.md) to tell
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
