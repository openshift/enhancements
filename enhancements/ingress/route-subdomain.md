---
title: route-subdomain
authors:
  - "@Miciah"
reviewers:
  - "@alebedev87"
  - "@smarterclayton"
  - "@gcs278"
approvers:
  - "@frobware"
api-approvers:
  - "@Miciah"
creation-date: 2022-02-02
last-updated: 2022-02-02
tracking-link:
  - "https://issues.redhat.com/browse/NE-700"
see-also:
replaces:
superseded-by:
---

# Route Subdomain


## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement implements the `spec.subdomain` field of the OpenShift Route
API.  When `spec.host` is specified or `spec.subdomain` is omitted on a Route,
then the Route API behaves the same as before this enhancement.  When
`spec.host` is omitted and `spec.subdomain` is specified, the host name of the
Route depends on the domain of the IngressController that exposes the Route.  If
multiple IngressControllers expose a Route that specifies `spec.subdomain`, then
the Route has a distinct host name for each IngressController that exposes it.
This is particularly useful with sharding where the Route is exposed on multiple
shards.  It is also useful generally in situations where the user wants to
specify the subdomain part of the Route's host name but allow the
IngressController to set the domain part.

## Motivation

OpenShift router is designed to accommodate multiple router deployments running
within a single cluster.  Within a cluster with multiple router deployments,
each Route may be exposed by a single router deployment, or it may be exposed by
multiple router deployments.  The former is useful for sharding, that is,
partitioning Routes for isolation or performance.  The latter (having each
Route exposed by multiple routers) is useful for redundancy or for enabling
multiple ingress paths for the same application, such as having an internal
router deployment for in-cluster clients and an externally exposed router
deployment for clients outside the cluster.

The absence of support for the `spec.subdomain` API complicates both use-cases.
Without support for `spec.subdomain`, the `spec.host` field must be specified.
If the user does not specify a value when the Route is created, the API fills in
a default value using the cluster ingress domain.  Having `spec.host` default to
the cluster ingress domain complicates sharding because it puts the onus on the
user to specify `spec.host` with a domain that matches the domain of the shard
to which the Route will belong.  The defaulting also complicates the use-case
of having the same Route exposed on multiple router deployments because the
Route's host will match the domain of only one of those router deployments.

The `spec.subdomain` field solves this problem by allowing `spec.host` to be
omitted and a subdomain to be specified.  Then, each router deployment that
exposes the Route constructs a host name from the subdomain specified in
`spec.subdomain` and the router deployment's domain.  The router deployment
reports this host name in the Route's status: each router deployment adds an
entry to `status.ingress[]` with the host name that the router deployment has
assigned to the Route.

By implementing the `spec.subdomain` field, it will be possible for a Route to
have one or more host names determined by the router deployment that exposes the
Route, thus simplifying sharding and other use-cases.

### Goals

* An application administrator can create a Route that is exposed by one or more
  IngressControllers, and each of the IngressControllers will expose the Route
  under the IngressController's domain.
* An application administrator can create a Route with a so-called "vanity"
  (custom) subdomain without needing to specify a domain.
* When a user creates a Route with a nonempty value for `spec.subdomain`, the
  API does not set a value for the Route's `spec.host`.
* When an IngressController admits a Route with an empty value for `spec.host`
  and a nonempty value for `spec.subdomain`, the IngressController exposes the
  Route using a host name comprising the Route's subdomain and the
  IngressController's domain.

### Non-Goals

* This enhancement does not configure labels or label selectors for sharding.
* This enhancement does not add any new API fields.
* This enhancement does not provide any templating mechanism for host names.
* This enhancement does not allow one Route to have multiple host names for the
  same router deployment.

## Proposal

Years before this enhancement was written, the `spec.subdomain` field was added
to the Route API but left unimplemented.  This enhancement implements support
for `spec.subdomain` in the OpenShift API server and OpenShift router so that
users can specify `spec.subdomain` instead of specifying `spec.host` to get a
Route that assumes the particular domain of any and all IngressControllers that
expose it.  For example, if a router with the domain "bar.tld" admits a Route
with subdomain "foo", the router exposes the Route using the host name
"foo.bar.tld", and if a second router with the domain "baz.tld" admits the same
Route, then this second router exposes the Route using the host name
"foo.baz.tld".

With this enhancement, the OpenShift API server is modified not to set a default
value for `spec.host` if `spec.subdomain` is specified, and the ingress operator
is modified to configure router deployments to use the IngressController's
`status.domain` as the router deployment's domain.  OpenShift router is modified
to use this domain along with the Route's subdomain to compose a default host
name for any Route that does not specify `spec.host`; the router then reports
this host name in the Route's `status.ingress[*].host` field.  For example, a
router named "foo" with domain "baz.tld" that admits a Route with subdomain
"bar" will set `status.ingress["foo"].host` to "bar.baz.tld".

### Validation

The OpenShift API server is modified to validate that the `spec.subdomain` field
is a valid DNS subdomain as defined in RFC 1123: The value must not exceed 253
characters and must comprise a period-delimited sequence of labels where each
label does not exceed 63 characters and does begin with an alphanumeric
character and contain only alphanumeric characters, possibly with non-initial
and non-final dashes.  Previously, the OpenShift API server performed no
validation on the field; however, because the field was unused before this
enhancement, the stricter validation can be justified as not breaking any
previously supported use-case; cf. "Risks and Mitigations" below.

The OpenShift API server is also modified not to set a default value for
`spec.host` if `spec.subdomain` has a nonempty value.  This change enables each
router deployment to set a host, which is reported in the Route's
`status.ingress[*].host` field.

### User Stories

#### As an application developer, I want to create a Route with an explicit host name

Users can still specify `spec.host`, same as before this enhancement.

#### As an application developer, I want to create a Route and have a default host name set using the cluster ingress domain

Users can omit `spec.host` and `spec.subdomain`, and the API will set a default
host name based on the cluster ingress domain, same as before this enhancement.

#### As an application developer, I want to create a Route and have a default host name set using the cluster ingress domain and a subdomain of my choosing

Users can omit `spec.host` and specify `spec.subdomain`, in which case the API
will leave `spec.host` unset, and the router deployment for the default
IngressController will use a host name based on the cluster ingress domain and
the user-specified subdomain.  For example, a user could specify a Route as
follows:

```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: hello-openshift
  namespace: hello-openshift
spec:
  # host left unspecified.
  subdomain: hello
  # ...
```

Because the subdomain field is specified, the API will not set a default value
for the host field.  Instead, the default IngressController's router deployment
will use a host name based on the specified subdomain and the cluster ingress
domain.  The router will add an entry to the Route's status to indicate the
exact host name:

```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: hello-openshift
  namespace: hello-openshift
spec:
  # ...
  subdomain: hello
  # ...
status:
  ingress:
  - conditions:
    - lastTransitionTime: # ...
      status: "True"
      type: Admitted
    host: hello.apps.mycluster.com
    routerCanonicalHostname: router-default.apps.mycluster.com
    routerName: default
    # ...
```

#### As an application developer, I want to create a Route and have it exposed by multiple router deployments, using their respective domains

Users can omit `spec.host` and specify `spec.subdomain`, in which case the API
will leave `spec.host` unset, and each router deployment that exposes the Route
will do so using the router deployment's domain and the Route's subdomain.  For
example, suppose the cluster administrator has two IngressController: one named
"default" with domain `apps.mycluster.com` and one named "internal" with
domain `apps-internal.mycluster.com`.  The application developer can then
specify a Route as follows:

```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: hello-openshift
  namespace: hello-openshift
spec:
  # host left unspecified.
  subdomain: hello
  # ...
```

Because the subdomain field is specified, the API will not set a default value
for the host field.  Instead, **each** router deployment will add an entry to
the Route's status to indicate the host name under which that particular router
deployment exposes the Route:

```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  name: hello-openshift
  namespace: hello-openshift
spec:
  # ...
  subdomain: hello
  # ...
status:
  ingress:
  - conditions:
    - lastTransitionTime: # ...
      status: "True"
      type: Admitted
    host: hello.apps-internal.mycluster.com
    routerCanonicalHostname: router-internal.apps-internal.mycluster.com
    routerName: internal
    # ...
  - conditions:
    - lastTransitionTime: # ...
      status: "True"
      type: Admitted
    host: hello.apps.mycluster.com
    routerCanonicalHostname: router-default.apps.mycluster.com
    routerName: default
    # ...
```

#### As an application developer, I want to create a Route and have a default host name set based on the shard that exposes the Route

Users can omit `spec.host` and specify `spec.subdomain`, in which case the API
will leave `spec.host` unset, and each router deployment that exposes the Route
will do so using the router deployment's domain and the Route's subdomain.  For
example, suppose the cluster administrator has created the following
IngressController:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: shard1
  namespace: openshift-ingress-operator
spec:
  domain: shard1.apps.mycluster.com
  routeSelector:
    matchLabels:
      shard: shard1
```

The application developer specifies a Route as follows:

```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  labels:
    shard: shard1
  name: hello-openshift
  namespace: hello-openshift
spec:
  spec:
    # host left unspecified.
    subdomain: hello
  # ...
```

Because the subdomain field is specified, the API will not set a default value
for the host field.  Instead, the router deployment associated with the "shard1"
IngressController will add an entry to the Route's status to indicate the host
name under which the router exposes the Route:

```yaml
apiVersion: route.openshift.io/v1
kind: Route
metadata:
  labels:
    shard: shard1
  name: hello-openshift
  namespace: hello-openshift
spec:
  # ...
  subdomain: hello
  # ...
status:
  ingress:
  - conditions:
    - lastTransitionTime: # ...
      status: "True"
      type: Admitted
    host: hello.shard1.apps.mycluster.com
    routerCanonicalHostname: router-shard1.shard1.apps.mycluster.com
    routerName: shard1
    # ...
```

### API Extensions

This enhancement uses the existing `spec.subdomain` field of the Route API.  It
does not modify the structure or semantics of the API; it merely implements
existing semantics that have been specified as optional and were previously
unimplemented.

### Risks and Mitigations

The Route API's `spec.subdomain` field has existed since OpenShift 4.1 as an
explicitly optional field; it has simply not been implemented before OpenShift
4.11.  Users may already be using manifests that specify `spec.subdomain` on
clusters running older OpenShift releases.  When these manifests are
instantiated on a cluster where `spec.subdomain` is implemented, the resulting
Route will behave differently.  To mitigate confusion, this enhancement will
include a release note advising users of that change.

Users can proactively check for manifests that specify `spec.subdomain` prior to
upgrading to a version of OpenShift that supports `spec.subdomain`:

```shell
grep -e subdomain: -- manifests/*.yaml
```

Users and admins can also proactively check for instantiated Routes that specify
`spec.subdomain` on a running cluster:

```shell
oc get routes --output=go-template --template='{{range .items}}{{if .spec.subdomain}}{{printf "%s %s %s\n" .metadata.namespace .metadata.name .spec.subdomain}}{{end}}{{end}}' --all-namespaces
```

However, per the Route API's existing semantics, a Route object that has already
had `spec.host` defaulted on an older OpenShift cluster will continue to be
served using the same host name, and `spec.subdomain` will be ignored; the
ultimate purpose of checking for `spec.subdomain` on instantiated Route objects
is to identify Routes that may have been created from manifests that specify
`spec.subdomain`.

## Design Details

### Open Questions

Do `oc get` or `oc describe` need changes to print Routes' host names when
`spec.host` is omitted?

Is it safe to change existing core platform components' Routes to use
`spec.subdomain`, or would doing so break compatibility for use-cases built on
assumptions regarding API defaulting?  For example, as of OpenShift 4.10, the
OpenShift console operator creates a Route for the console without specifying
`spec.host` or `spec.subdomain`, and so the API defaults `spec.host`; if some
use-case assumes that the console Route is exposed using the same host name on
every router deployment, then changing the console operator to specify
`spec.subdomain` could break this use-case.

### Test Plan

This enhancement involves changes to the ingress operator, the OpenShift API
server's validation logic, and OpenShift router itself.  The change in the
ingress operator involves a minor change to how the operator configures router
deployments; this change can be verified using unit tests.  The change in the
validation and router can be verified using an integration test in
openshift/origin.  For example, an integration test could do the following:

1. Deploy two router deployments with distinct domains.
2. Create four Routes:
   - One Route that omits both `spec.host` and `spec.subdomain`.
   - One Route that omits `spec.host` and specifies `spec.subdomain`.
   - One Route that specifies `spec.host` and omits `spec.subdomain`.
   - One Route that specifies both `spec.host` and `spec.subdomain`.
3. Verify that each router deployment admits each Route and puts the expected host in the Route's status.
   - For a Route that omits both `spec.host` and `spec.subdomain`, the API should set a default value for `spec.host`, and the router should expose the Route using that host name.
   - For a Route that specifies `spec.host`, with or without `spec.subdomain`, the router should expose the Route using that host name.
   - For a Route that omits `spec.host` and specifies `spec.subdomain`, the router should leave `spec.host` empty, and the router should expose the Route using a host name composed from the Route's subdomain and the router's domain.

### Graduation Criteria

The Route API, including the `spec.subdomain` field, is already GA but optional
to implement, so the API and implementation do not require graduation
milestones.

#### Dev Preview -> Tech Preview

N/A.  This feature will go directly to GA.

#### Tech Preview -> GA

N/A.  This feature will go directly to GA.

#### Removing a deprecated feature

N/A.  We do not plan to deprecate this feature.

### Upgrade / Downgrade Strategy

On clusters running older OpenShift releases, the OpenShift API server will have
already set a default value for `spec.host` on every Route.  On upgrade, this
value will continue to be used.

Suppose an administrator upgrades a cluster to an OpenShift version that has
this enhancement, users create Routes that specify `spec.subdomain`, and then
the administrator downgrades the cluster to an OpenShift version that doesn't
have this enhancement; in this scenario, Routes that specify `spec.subdomain`
and not `spec.host` will have `spec.host` defaulted by the downgraded OpenShift
API server, and any router deployments that expose the Route will do so using
the newly defaulted host name.  Although this behavior is allowed by the API's
documented semantics, it may be confusing to users, so it should be noted in a
release note (cf.  "Risks and Mitigations").

### Version Skew Strategy

If a cluster has an older ingress operator and a newer OpenShift router
deployment, the operator will not configure the router deployment to enable
`spec.subdomain`; in this case, the router deployment will behave the same as an
older router deployment.

If a cluster has an older OpenShift API server and a newer OpenShift router
deployment, the API will set a default `spec.host` on Routes that don't specify
it; in this scenario, the router deployment will again behave the same as an
older router deployment.

If a cluster has a newer OpenShift API server and an older OpenShift router
deployment and a user creates a Route that omits `spec.host` and specifies
`spec.subdomain`, the API may leave `spec.host` empty, and the old router
deployment may itself set a default host name for the Route.  Note that the
router does **not** update the Route to persist any default host name that the
router itself sets.  This means that such a Route that is created on a partially
upgraded or downgraded cluster might use an unexpected host name until the
router deployment is upgraded.

Although this behavior technically wouldn't violate the API's documented
semantics, it could be confusing; the mitigation is to check for Routes that
specify `spec.subdomain` before upgrading or downgrading to or from an OpenShift
version that implements `spec.subdomain` (cf.  "Risks and Mitigations").

### Operational Aspects of API Extensions

Any users (human or software) of the Route API must not assume that `spec.host`
will be set.  The `spec.host` field specifies a single host name by which the
user **desires** a Route be exposed.  The Route's status reports the host name
(or possibly multiple host names) by which the Route is **actually** exposed.
If a user needs to know the host name under which a Route is exposed, the user
must look at the Route's status.

Some existing users of the Route API ignore the documented semantics and
incorrectly read `spec.host` instead of checking the status for the actual host
name (or names).  These users should be corrected lest they take inappropriate
action for a Route that omits `spec.host` and specifies `spec.subdomain`.

For example, external-dns is known to use `spec.host` when creating a DNS record
for a Route (cf. [the definition of `endpointsFromOcpRoute` in the external-dns
source](https://github.com/kubernetes-sigs/external-dns/blob/ced50bf0f5b5d3f754994166fa499028f07c2bff/source/openshift_route.go#L245-L246).
This user should be corrected to check the status to find the entry for some
given router deployment and then use the host name from that status entry.

#### Failure Modes

Behavior is unchanged for Routes that specify `spec.host`.  This includes
existing platform Routes; this enhancement does not impact them.  As described
in the "Version Skew Strategy" and "Upgrade / Downgrade Strategy" sections
(q.v.), users may experience confusing behavior if they attempt to use
`spec.subdomain` on a partially upgraded cluster.

If the user observes unexpected behavior, the user can check a Route's
`spec.host`, `spec.subdomain`, and `status.ingress` fields to determine whether
a given router has admitted the Route and by what host name the Route can be
expected to be available.

Further investigation may involve checking router pods' logs using `oc -n
openshift-ingress logs -c router deploy/router-<name of IngressController>` or
examination of the router pod's `haproxy.config` and map files.

The Network Edge team would be responsible for helping to diagnose and resolve
issues stemming from this enhancement.

#### Support Procedures

To diagnose issues with a Route, check its spec and status.  For example, the
following Route specifies a host name in its `spec.host` field and is exposed by
the "default" router deployment using that host name:

```shell
oc -n <namespace> get routes/<name> -o yaml
```

```yaml
spec:
  host: myapp-mynamespace.apps.mycluster.com
status:
  ingress:
  - conditions:
    - lastTransitionTime: # ...
      status: "True"
      type: Admitted
    host: myapp-mynamespace.apps.mycluster.com
    routerCanonicalHostname: router-default.apps.mycluster.com
    routerName: default
    # ...
```

If `spec.host` is set but the user did not specify it when creating the Route,
then most likely the API set the value when it admitted the Route.  To verify,
the user can delete and recreate the Route and observe whether the API sets a
default value for `spec.host`.

If `status.ingress` does not have an entry for a router deployment, or if it has
an entry for which the "Admitted" status condition does not have status "True",
then the router deployment is not exposing the Route.  In the case of a
validation error, the status condition will indicate the cause of the error.  If
the status has an unexpected host name in the `host` field, then the Route's
spec may need to be updated, or the Route may need to be recreated.

Validation errors may also appear in the router pods' logs; use `oc -n
openshift-ingress logs -c router deploy/router-<name of IngressController>` to
check the logs for errors.

In addition to reporting the host name in the Route's status, the router should
also write the Route's host name in one or more HAProxy map files in each pod of
the router deployment, depending on the Route type:

- The host name of a cleartext (non-TLS) Route should appear in `os_http_be.map`.
- The host name of an edge-terminated TLS or reencrypt Route should appear in `os_edge_reencrypt_be.map`.
- The host name of a passthrough TLS Route should appear in both `os_tcp_be.map` and `os_sni_passthrough.map`.

If the expected host name is not written to the appropriate map files, this may
indicate an issue with the Route or OpenShift router warranting a Bugzilla
report.

## Implementation History

The `spec.subdomain` field was added to the Route API in OpenShift 4.1 and will
be implemented in a future release.

* 2019-03-12, [openshift/router#19](https://github.com/openshift/router/pull/19)
  was opened to add support for `spec.subdomain`; this PR was never merged.
* 2019-03-29,
  [openshift/api#250](https://github.com/openshift/api/pull/250/commits/2f62ae36ce75ab7b3ef08b1909600f7de6cb5e5b)
  was merged, adding the `spec.subdomain` API field.
* 2019-04-02,
  [openshift/api#275](https://github.com/openshift/api/pull/275/commits/6579faa0eba6b427e408c5fea12c837cd127d7fd)
  was merged, reverting #250 to unblock a rebase.
* 2019-04-22,
  [openshift/api#291](https://github.com/openshift/api/pull/291/commits/52b90890b812dfa3d9ef12d7a0fde0fe4113cb86)
  was merged, restoring the new API field.
* 2021-03-30,
  [openshift-apiserver#194](https://github.com/openshift/openshift-apiserver/pull/194)
  was opened to prevent the API server from defaulting `spec.host` when
  `spec.subdomain` is specified; this PR was never merged.
* 2021-11-02,
  [openshift/router#357](https://github.com/openshift/router/pull/357) was
  opened, superseding #19;
  [openshift/openshift-apiserver#254](https://github.com/openshift/openshift-apiserver/pull/254)
  was opened, superseding
  [openshift-apiserver#194](https://github.com/openshift/openshift-apiserver/pull/194);
  and
  [openshift/cluster-ingress-operator#674](https://github.com/openshift/cluster-ingress-operator/pull/674)
  was opened to configure the domain in router deployments.
* 2022-02-02, this enhancement was filed.

## Drawbacks

Implementing `spec.subdomain` expands the set of features that would need to be
defined and implemented in any successor to the Route API and OpenShift router.
In particular, [Gateway API's HTTPRoute
kind](https://github.com/kubernetes-sigs/gateway-api/blob/master/apis/v1alpha2/httproute_types.go)
defines no equivalent API field; this gap could impede users from migrating from
the Route API to Gateway API.

## Alternatives

### Require explicit `spec.host` and multiple Routes per domain (the status quo)

For the use-case in which a user wants a custom subdomain and the cluster
ingress domain, the user (human or software) can manually look up the cluster
ingress domain or the default IngressController's domain and specify an explicit
value for `spec.host` with the desired subdomain and the correct domain.
Compared to using `spec.subdomain`, this alternative puts unnecessary
operational burden on users.

For the use-case in which the user wants multiple router deployments to expose a
Route using each router's respective domain, the user would need to create
multiple Routes: one per router deployment, explicitly specifying the
appropriate domain for each Route in `spec.host`.  Again, this alternative puts
unnecessary operational burden on users.

### Define a mutating webhook to set `spec.host`

For the sharding use-case, a custom mutating webhook or other controller could
set `spec.host` using the appropriate domain based on the domain of the router
deployment corresponding to the shard to which the Route belongs.  An advantage
to this approach is that the controller could handle other aspects of sharding
as well, such as automatically assign Routes to shards based on capacity or
other factors (cf. [RFE-1467](https://issues.redhat.com/browse/RFE-1467) for an
exploration of this idea).  Disadvantages of this approach include the
additional development and operations effort required to implement and manage
such a controller as well as the fundamental limitation that a Route could still
only have a single host name and thus could not be simultaneously be exposed by
multiple shards with different domains.

### Default `spec.subdomain` instead of defaulting `spec.host`

A possible modification to this enhancement would be to change the OpenShift API
server not to default `spec.host` on Routes at all, but instead to default
`spec.subdomain`.  This modification would be consistent with the existing Route
API contract (which does not dictate the API defaulting behavior), and it would
have several advantages.  In particular, it would facilitate the transition from
using `spec.host` to using `spec.subdomain` for Routes that currently rely on
the API to default `spec.host`.  For example, the OpenShift console operator
creates a Route for the console without specifying `spec.host`, leaving it to
the API to set a default host name; changing the API to default `spec.subdomain`
instead of `spec.host` would cause the console's Route to use `spec.subdomain`
on new clusters with no change to the console operator itself.  The result would
be that on newly installed clusters, the console's Route would assume the domain
of any router deployment that exposed the Route.

The disadvantage to defaulting `spec.subdomain` instead of `spec.host` is that
it would make the enhancement no longer an opt-in feature; that is, while the
enhancement without this modification would have no effect for users who didn't
specify `spec.subdomain` on Routes, this modification would change the behavior
for users who omitted both `spec.subdomain` and `spec.host`.  There is a risk
that this change would surprise users and cause problems for use-cases that are
built on assumptions about API defaulting.
