---
title: cluster-high-availability-mode-api
authors:
  - "@dhellmann"
  - "@eranco"
  - "@mrunalp"
  - "@romfreiman"
reviewers:
  - TBD
  - "@markmc"
  - "@deads2k"
  - "@wking"
  - "@hexfusion"
approvers:
  - TBD
  - "@eparis"
  - "@derekwaynecarr"
creation-date: 2020-12-02
last-updated: 2020-12-02
status: implementable
see-also:
  - "/enhancements/single-node-production-edge-cluster-profile.md"
  - "/enhancements/update/cluster-profiles.md"
---

# Cluster High-availability Mode API

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

As we add more deployment topologies for OpenShift, we need a way to
communicate expectations to the operators managing the cluster. This
enhancement describes an API for telling operators how hard to work to
provide high-availability of their operands.

## Motivation

We want to avoid having cluster profiles use different manifests to
configure operators managed by the `cluster-version-operator`, and
instead rely on in-cluster APIs. This approach will make the same
information available to operators managed by either the
`cluster-version-operator` or `operator-lifecycle-manager`.

We do not want to expose the cluster profile name directly to most of
the operators, because we do not want to have to update all of the
operators each time we add a new profile.

We want the setting to be global to the cluster, rather than using
operator-specific APIs, because that simplifies the integration with
the installer and allows it to be applied consistently in all
operators that need them.

### Goals

- Expose high-availability expectations to operators running in a
  cluster so they can configure their operands appropriately.

### Non-Goals

- Expose cluster profile names to operators.
- Provide a mutable API for changing the high-availability mode (at
  least for now).
- Predict other capability settings that we might need to expose.
- Define a more generic "capabilities" API.

## Proposal

### Cluster Operators

The `infrastructure.config.openshift.io` [informational
resource](https://docs.openshift.com/container-platform/4.6/installing/install_config/customizations.html#informational-resources_customizations),
is expanded to include two new fields to describe the
high-availability mode desired for cluster operators running on the
cluster through "topology" values for the control plane and
infrastructure, separately.

The `controlPlaneTopology` field will express the expectations for
operands that normally run on control nodes.

The `infrastructureTopology` field will use an enum to express the
expectations for operands that normally run on infrastructure nodes.

Possible values are `HighlyAvailable` and `SingleReplica`. The default is
`HighlyAvailable`, which represents the behavior operators have today
in a "normal" cluster. The `SingleReplica` setting will be used in
single-node deployments (developer and production), and operators
should not configure their operands for highly-available
operation.

For this first pass, the user will not have direct control over the
settings. Instead, the installer will set the `controlPlaneTopology`
and `infrastructureTopology` status fields based on the replica counts
for the cluster when it is created.

When the control plane replica count is `< 3`, the
`controlPlaneTopology` is set to `SingleReplica`. Otherwise it is set to
`HighlyAvailable`.

When worker replica count is `0`, the control plane nodes are
also configured as workers. Therefore the `infrastructureTopology`
value will be the same as the `controlPlaneTopology` value.  When the
worker replica count is `1`, the `infrastructureTopology` is set to
`SingleReplica`.  Otherwise it is set to `HighlyAvailable`.

A future enhancement may add support for an `External` topology,
indicating that the operand is running somewhere other than a node in
the cluster, such as in containers on another cluster. This work is
planned, but not covered by this enhancement.

A future enhancement may add support for an `ActivePassive` topology,
indicating that the deployment is participating in active-passive
high-availability group with another cluster or clusters. This idea is
an example, and is not yet planned or covered by this enhancement.

The new cluster-wide settings will supersede existing unsupported
APIs, such as the flags to disable high-availability requirements for
`cluster-etcd-operator` and `cluster-authentication-operator`.

If other *supported* APIs for configuring resource use exist, they
take precedence over the new cluster-wide APIs. The cluster-wide APIs
should be treated as a default for cases where operator-specific
values are not set, a replacement for unsupported APIs, and a way to
avoid adding new operator-specific APIs.

### Operator Lifecycle Manager Operators

Operators managed by OLM will look at the `infrastructure` API if they
need to configure their operand differently based on the
high-availability mode.

### User Stories

1. As an author of an operator, I want to make my operator adapt to
   clusters with different topologies so that more users can benefit
   from using the operator.
2. As the designer of a cluster profile, I want to express the intent
   of the profile so that operators can adapt to the new cluster type.

### Implementation Details/Notes/Constraints

We will extend the infrastructure API struct, add the new enums, and
update the CRD in the `openshift/api` repository.

We will add logic to the installer to use the profile name to create a
manifest to set up the singleton of the `infrastructure` API resource
with the appropriate settings for both fields. For example, a
single-node production deployment would have `controlPlaneTopology`
and `infrastructureTopology` both set to `SingleReplica`.

### Risks and Mitigations

#### Limited understanding of new requirements

This work is based on very early understanding of the requirements, so
it would be easy to define cluster capabilities or topologies that are
not needed or that do not let us express the intent in a way that the
implementation in individual operators can use. By scoping the initial
implementation to only focus on the high-availability behavior and
leaving other potential parameters to later iterations, we give
ourselves more time to consider the design.

We could also use operator-specific APIs, but that is likely to
require more work to ensure they are all set properly (see the
alternatives list below).

#### Scaling single-node clusters up

If a single-node deployment scales the control plane up, then it would
be reasonable to expect the relevant operators to scale their operands
as well. That implies the topology APIs need to be tied to the one
that scales the control plane, somehow. More work is needed to
understand that link and the implications.

In the case where a single-node deployment gains some worker nodes, no
change would be expected in the control plane operands because those
worker nodes should not run the control plane components.

*How will security be reviewed and by whom? How will UX be reviewed and by whom?*

## Design Details

### Open Questions

1.

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

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

### Upgrade / Downgrade Strategy

#### Immutable API

While the API is immutable, updating to a new version (upgrade or
downgrade) is safe because nothing will change the values in the
`status` block of the resource.

#### Mutable API (future work)

Upgrading the API to add new fields is safe because old consumers will
ignore them until they are also upgraded.

Adding new values to enum fields is not safe, because there is a
window of time between when the API itself is updated and when the
operators using the API are updated. To avoid issues during that time,
the controller that manages the infrastructure API will wait for the
`ClusterVersion` resource to have its progressing condition set to
false, before it copies settings from the `spec` fields to the
`status` used by consumers. This will ensure that the other operators
will be upgraded and start consuming the old settings. Then, when all
of the other operators are updated, the settings in the `status`
fields will be updated and the other operators will re-configure their
operands.

For a downgrade, the administrator may need to manually update the
`infrastructure` API to change settings that use new values not
supported in the older version.

### Version Skew Strategy

Clusters deployed using 4.7 or earlier will not have the new fields in
the `infrastructure` API so the value in the struct will be
empty. Operators that need the high-availability mode should treat an
empty string the same as `HighlyAvailable`.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

### Define a new Capabilities API

The initial proposal was to create a new informational API to express
cluster-wide "capabilities". That approach was rejected for several
reasons:

1. In configurations where there is an external control plane, the
   `infrastructure` API is already being expressed inside the
   cluster. Adding a new API would require similar work.
2. The new API was seen as collecting unrelated settings into a
   generic object.

In addition to these objections, a new API is more work than adding a
field (or a few fields) to the existing API, because it requires more
code to be updated, including having the RBAC rules configured to
allow the service accounts to see it.

### Use a "high availability mode" flag

Another early draft of this enhancement defined a
`highAvailabilityMode` enum field with values `Full` and `None`. This
approach was considered to be too narrowly focused on the
high-availability mode and to inaccurately express the desired
topology, so that it could not support the notion of an external
control plane.

### Expose the cluster profile name directly

We could add a `clusterProfile` field to an existing or new
configuration API. This would require each operator to interpret each
new profile, which would make maintaining profiles even more work than
it is now. It may also encourage us to create more profiles, when we
have stated elsewhere that our goal is to have as few as possible.

### Use the cluster profile to have different Deployments for operators

We could use different manifests to have the cluster-version-operator
deploy operators with different configuration (via command line
arguments or environment variables). This would give us a way to
configure operators differently for each profile, but those settings
would be less discoverable and we would not be able to offer users a
way to change them at runtime (if we ever want to do that).

### Recognize HA/non-HA configuration based on control plane node count

Operators could look at the number of control plane nodes and use that
information to determine whether HA is enabled (3 or more nodes) or
not (1 node).

This would require giving the operator read access to Node resources,
and would be subject to race conditions as the control plane
forms. For example, the cluster-etcd-operator [requires 3 control
plane
nodes](https://github.com/openshift/cluster-etcd-operator/blob/98590e6ecfe282735c4eff01432ae40b29f81202/pkg/etcdenvvar/etcd_env.go#L72)
to exist before deploying its operand. If it counted nodes, it could
not tell the difference between a cluster starting to form with 1
node, then more, or a cluster that is only ever going to include 1
node.

Looking at nodes also will not work in clusters using an external
control plane, since there will not be any special control plane
nodes.

### Use operator-specific APIs to manage the behaviors

The cluster-etcd-operator and cluster-authentication-operator both
include flags to enter an unsupported non-HA operating mode. We could
change those APIs to make the flags supported, and add similar flags
to other components, such as cluster-ingress-operator and
machine-config-operator. This would offer more flexibility, since each
component could be configured separately. However, these settings are
needed during cluster bootstrapping, so this approach would tie the
installer to the APIs for each of the operators so they could be
configured separately.

Complexity will grow further if we add capabilities for managing the
resource consumption of operators and their operands, since we risk
having different ways to express those constraints in different APIs.

### Manage upgrades by tracking settings in each consuming operator

When we make the API mutable we will need to protect against older
components encountering unknown enum values during an upgrade.
Anything that use the `capabilities` API could copy the settings into
its own API resource. If the enum value is unknown, it wouldn't be
copied and the existing configuration would remain in place until the
newer version of the component is deployed. This approach would
require similar logic in each component, which wouldn't be complicated
to implement but which would nonetheless be easy to overlook when
implementing a feature.

### Configure OLM-managed operators without the Infrastructure API

The argument can be made that operators managed by OLM should not be
required to respond to an OpenShift-specific API. If that argument is
accepted, the OLM should read the value of `highAvailabilityMode` and
pass it to the operators it manages in some other way. Two approaches
were identified and rejected:

1. Have OLM add an environment variable with the value to the
   Deployments for all operators it manages, similar to how the proxy
   settings are configured today.

   The proxy environment variables are standardized, but this new
   variable would be specific to OLM. It is possible that even an
   apparently unique name could collide with a variable already used
   by an operator. Using environment variables also effectively makes
   those an API defined by OLM.

2. Have OLM define its own kubernetes API for these settings, and
   populate it based on the infrastructure API.

   Operators would still need to respond to the API, but it would be
   defined by OLM rather than OpenShift, making it more portable
   across distributions. On the other hand, OLM itself would still
   need to use the OpenShift API to configure the new API resource.
