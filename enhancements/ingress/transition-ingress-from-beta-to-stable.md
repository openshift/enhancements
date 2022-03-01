---
title: transition-ingress-from-beta-to-stable
authors:
  - "@Miciah"
reviewers:
  - "@candita"
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@miheer"
  - "@rfredette"
  - "@sgreene570"
approvers:
  - "@danehans"
  - "@frobware"
  - "@knobunc"
creation-date: 2021-03-09
last-updated: 2022-03-01
status: implemented
see-also:
replaces:
superseded-by:
---

# Transitioning Ingress API from Beta to Stable

This enhancement transitions OpenShift fully onto the stable
`networking.k8s.io/v1` API version of the Ingress API.

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement updates the
[ingress-to-route](https://github.com/openshift/openshift-controller-manager/blob/master/pkg/route/ingress/ingress.go)
controller to use the stable `networking.k8s.io/v1` version of the Ingress API.
Although the API server already supports the v1 API version without this
enhancement, the ingress-to-route controller requires changes to transition from
using the v1beta1 client to using the v1 client and to support new v1 features.
These new features include the `spec.pathType` and `spec.ingressClassType`
fields as well as the associated IngressClass API.  This enhancement does *not*
extend the Route API to accommodate new features in the Ingress API; these are
only supported to the extent that they are compatible with the Route API.

## Motivation

Some users want to use new features in the `networking.k8s.io/v1` API version,
and Kubernetes 1.22 will remove the `networking.k8s.io/v1beta1` API version.

The IngressClass API is of particular utility.  With this API, an administrator
can specify which Ingresses OpenShift should publish and which Ingresses
OpenShift should ignore, on the assumption that some third-party ingress
controller publishes them.  This is particularly important with respect to
status updates, as the Ingress API does not provide a way to differentiate
status published by one ingress controller from status published by another
ingress controller, which has the consequence that OpenShift's updates to an
Ingress's status can interfere with the operation of third-party ingress
controllers or other controllers that read the Ingress's status.  Thus
supporting this API is important for accommodating third-party ingress
controllers in OpenShift clusters.

The following table lists new API fields in Ingress v1 and the status of their
support with the implementation of this enhancement:

| Field                                          | Supported     |
|------------------------------------------------|---------------|
| `spec.defaultBackend`                          | No            |
| `spec.ingressClassName`                        | Yes[^1]       |
| `spec.rules[*].http.paths[*].backend.service`  | Yes           |
| `spec.rules[*].http.paths[*].backend.resource` | No            |
| `spec.rules[*].http.paths[*].pathType`         | Partially[^2] |


### Goals

1. Honor `spec.ingressClassName`.
2. Prepare for the removal of the v1beta1 API version.

### Non-Goals

1. Extend the Route API to support new Ingress features.
2. Implement `spec.rules[*].http.paths[*].pathType: Exact`.
3. Implement new functionality (such as regular expression matching) for `spec.rules[*].http.paths[*].pathType: ImplementationSpecific`.
4. Implement `spec.defaultBackend`.
5. Enable Ingresses to use backends that are not Services.

## Proposal

First, the ingress-to-route controller is updated to use the v1 client libraries
and API types.

Next, the ingress-to-route controller is extended to check for the
`kubernetes.io/ingress.class` annotation and `spec.ingressClassName` field on
Ingresses.  If either of these is set, the ingress-to-route controller checks if
the specified IngressClass exists and, if it does, what its `spec.controller`
field value is.  If the Ingress specifies an IngressClass that does not exist or
that does not specify `openshift.io/ingress-to-route` for `spec.controller`, the
ingress-to-route controller ignores the Ingress.

Additionally, the ingress-to-route controller is extended to check the value of
`spec.rules[*].http.paths[*].pathType` on Ingresses.  If an Ingress rule
specifies the value "Exact" for this field, the ingress-to-route controller
ignores the rule.

Finally, the ingress operator is extended with a new controller that creates an
IngressClass for each IngressController.  When creating the IngressClass for the
default IngressController, the operator annotates the IngressClass with the
`ingressclass.kubernetes.io/is-default-class` annotation if no other
IngressClass already has that annotation.  Note that the operator does not add
the annotation when reconciling already created IngressClass objects as the
administrator may intentionally have configured the cluster to have no default
IngressClass.

### Validation

This enhancement does not add any new APIs, so no additional validation is
required.

### User Stories

#### As an application developer, I have an Ingress and a third-party ingress controller, and I want OpenShift to ignore this Ingress so that only my third-party ingress controller exposes it

To satisfy this use-case, the user can specify `spec.ingressClassName` on the
Ingress:

```console
oc -n my-project patch ingresses/my-ingress --type=merge --patch='{"spec":{"ingressClassName":"my-ingress"}}'
```

Optionally, the administrator can create an IngressClass with the name that the
Ingress specifies, although the API does not strictly require that an actual
IngressClass object by the specified name exist.  If an IngressClass does not
exist, or if it does not specify `spec.controller:
openshift.io/ingress-to-route`, then OpenShift will ignore Ingresses that
specify that IngressClass.  This means that OpenShift Router will not expose
such Ingresses and will not update their statuses.

#### As a cluster administrator, I have a third-party ingress controller, and I want my ingress controller (and *not* OpenShift's) to expose Ingresses by default

To satisfy this use-case, the cluster administrator can create an IngressClass
for the third-party ingress controller.  Then the administrator can add the
`ingressclass.kubernetes.io/is-default-class` annotation to this IngressClass:

```console
oc annotate ingressclasses/my-ingress ingressclass.kubernetes.io/is-default-class=true
```

As long as the IngressClass has this annotation, any Ingresses that are created
without specifying `spec.ingressClassName` will have their
`spec.ingressClassName` fields set to the IngressClass's name, and if this
IngressClass does not specify `spec.controller: openshift.io/ingress-to-route`,
then OpenShift ignore these Ingresses.

### Implementation Details

Implementing this enhancement requires changes in the following repositories:

* openshift/api
* openshift/cluster-ingress-operator
* openshift/openshift-controller-manager

The only API addition is the definition of a IngressClass controller name for
OpenShift Router: `openshift.io/ingress-to-route`.

The ingress-to-route controller is modified to use the `networking.k8s.io/v1`
client and API definitions and to respect `spec.ingressClassName` as described
above.

The ingress operator is modified to create and manage an IngressClass for each
IngressController.

As follow-up work, we are considering modifying the ingress operator to list all
Ingresses and Routes in the cluster and publish a metric for Routes that were
created for Ingresses that OpenShift no longer manages.  This metric could be
used in alerting rules.  The following alerting rules would be added to the
ingress operator (see "Risks and Mitigations" for more context as to the purpose
of these alerts):

* An alert for Routes that were created from Ingresses that OpenShift is no longer managing.
* An alert for Ingresses older than 1 day that do not specify `spec.ingressClassName`.

### Risks and Mitigations

The v1 API specifies that ingress controllers should ignore Ingresses that do
not specify any IngressClass.  However, because the Ingress API is significantly
older than the IngressClass API, many users are likely to have Ingresses that do
not specify `spec.ingressClassName` but that the users nevertheless did intend
for OpenShift to expose.  It is impossible to determine reliably what a user's
intent is for an Ingress that does not specify `spec.ingressClassName`.

Not exposing Ingresses that did not specify `spec.ingressClassName` would thus
pose a risk of breaking existing applications.  As OpenShift's behavior before
this enhancement was to expose all Ingresses, OpenShift's behavior after this
enhancement continues to be to expose any Ingress that does not specify
`spec.ingressClassName`, so as to maximize backwards compatibility within the
constraints of the API.

This behavior of exposing Ingresses that do not specify `spec.ingressClassName`
poses a different risk that OpenShift may expose Ingresses that users did not
mean to expose.  However, this is not a new risk as OpenShift already exposed
these Ingresses before this enhancement, and moreover, the Ingress API
specification explicitly states that ingress controllers "should" (not "must")
ignore Ingresses that do not specify `spec.ingressClassName`.  Thus this risk is
somewhat mitigated by long-standing circumstances and existing documentation.
Furthermore, if the cluster administrator does not want OpenShift to expose
Ingresses that do not explicitly specify an OpenShift-owned IngressClass, then
the administrator has the option of creating a custom IngressClass and
annotating it with `ingressclass.kubernetes.io/is-default-class=true`.

Finally, it is possible that a user could have created an Ingress with some
nonempty value for `spec.ingressClassName` that did not match an OpenShift
IngressClass object, and nevertheless intended for OpenShift to expose this
Ingress.  Again, it is impossible to determine reliably what a user's intent was
in such a scenario, but as OpenShift exposed such an Ingress before this
enhancement, changing this behavior could break existing applications.

To mitigate this last risk, the ingress-to-route controller does not remove
Routes that earlier versions of OpenShift created for Ingresses that specify
`spec.ingressClassName`.  Thus these Routes will continue to be in effect.
However, after this enhancement, OpenShift does not update such Routes and does
not recreate them if the user deletes them.  As follow-up work to this
enhancement, we are considering adding alerts in case any Routes existed in this
state, so that the administrator would know that the Routes needed to be
deleted, or the Ingress modified to specify an appropriate IngressClass so that
OpenShift would once again reconcile the Routes.

The following table summarizes the above scenarios:

| For Ingresses that...                     | After this enhancement, OpenShift will...                                                      |
|:------------------------------------------|:-----------------------------------------------------------------------------------------------|
| ...do not specify `spec.ingressClassName` | ...continue to translate these Ingresses to Routes.                                            |
| ...specify an OpenShift IngressClass      | ...continue to translate these Ingresses to Routes.                                            |
| ...specify a third-party IngressClass     | ...leave previously created Routes intact but not reconcile them, and possibly raise an alert. |

## Design Details

### Test Plan

The ingress-to-route controller has extensive unit test coverage; for this
enhancement, existing test cases are modified to verify correct behavior for the
various possible values of the `pathType` field, and test cases are added to
verify the expected behavior with respect to the `ingressClassName` field when
set to specify OpenShift's default ingress controller or set to specify a
third-party ingress controller.

The ingress API itself already has extensive unit tests for validation, as well
as end-to-end tests for the `IngressClass` API, upstream.

### Graduation Criteria

N/A.

### Upgrade / Downgrade Strategy

The API server already migrates Ingresses defined using the v1beta1 APIs to the
v1 API.

On upgrade, the ingress-to-route controller may stop managing some Routes; the
new alerts mentioned in the "Implementation Details" and "Risks and Mitigations"
sections, if implemented, would bring these Routes to the cluster
administrator's attention in case they required it.  On downgrade, OpenShift
would resume managing the same Routes.

### Version Skew Strategy

N/A.

## Implementation History

* 2015-09-19, the Ingress API was added with group version `experimental/v1` (https://github.com/kubernetes/kubernetes/pull/14175).
* 2015-09-24, leading up to Kubernetes 1.0, the `experimental/v1` API group version was renamed to `experimental/v1alpha1` (https://github.com/kubernetes/kubernetes/pull/14156).
* 2015-10-10, leading up to Kubernetes 1.2, Ingress graduated from `experimental/v1alpha1` to `extensions/v1beta1` (https://github.com/kubernetes/kubernetes/pull/15409).
* 2017-01-19, leading up to OpenShift 3.5, Ingress `extensions/v1beta1` was implemented in OpenShift Router (https://github.com/openshift/origin/pull/12416).
* 2018-04-03, leading up to OpenShift 3.10, the OpenShift Router implementation of Ingress was replaced with the ingress-to-route controller (https://github.com/openshift/origin/pull/18658).
* 2019-02-21, leading up to Kubernetes 1.14, the `networking.k8s.io/v1beta1` version of the Ingress API was added (https://github.com/kubernetes/kubernetes/pull/74057).
* 2020-03-01, the IngressClass API was added with the `networking.k8s.io/v1beta1` API version, and the `spec.ingressClassName` field was added to the Ingress `networking.k8s.io/v1beta1` API version  (https://github.com/kubernetes/kubernetes/pull/88509).
* 2020-03-03, leading up to Kubernetes 1.18, the `spec.pathType` field was added to the Ingress `networking.k8s.io/v1beta1` API version (https://github.com/kubernetes/kubernetes/pull/88587).
* 2020-04-20, leading up to OpenShift 4.5, the ingress-to-route controller was updated to use the Ingress `networking.k8s.io/v1beta1` API version, albeit without implementing the new `spec.pathType` or `spec.ingressClassName` fields (https://github.com/openshift/openshift-controller-manager/pull/83).
* 2020-06-17, the Ingress API graduated to `networking.k8s.io/v1` (https://github.com/kubernetes/kubernetes/pull/89778).
* 2020-06-25, leading up to Kubernetes 1.19, the v1beta1 versions of the Ingress API were deprecated (https://github.com/kubernetes/kubernetes/pull/92484).
* 2021-04-05, the `openshift.io/ingress-to-route` ingressclass controller name was defined in openshift/api (https://github.com/openshift/api/pull/873).
* 2021-04-06, logic was added to the ingress operator to create IngressClass objects corresponding to IngressControllers (https://github.com/openshift/cluster-ingress-operator/pull/574).
* 2021-04-08, for OpenShift 4.8, the ingress-to-route controller was updated to respect IngressClass (https://github.com/openshift/openshift-controller-manager/pull/172).

## Alternatives

An alternative to supporting the `networking.k8s.io/v1` API version would be to
continue using the `networking.k8s.io/v1beta1` API version, which would require
a carry patch in openshift/kubernetes once upstream removes support for the
`networking.k8s.io/v1beta1` version.  Carrying such a patch would be highly
burdensome, and postponing support for features in the `networking.k8s.io/v1`
API and improved support for third-party ingress controllers would be highly
undesirable to users.

An alternative to exposing Ingresses that *do not* specify
`spec.ingressClassName` would be not to expose them but instead to rely on a
release note to prompt cluster administrators to verify that Ingresses specified
`spec.ingressClassName` as needed prior to upgrade.  This alternative would pose
an unknown but likely high risk of breaking existing users.

An alternative to exposing Ingresses that *do* specify `spec.ingressClassName`
would be not to expose them (i.e., delete existing associated Routes).  This
alternative would pose an unknown but likely moderate to high risk of breaking
existing users.

Instead of modifying the ingress operator to create IngressClass objects, an
alternative would be to leave defining IngressClasses entirely up to the cluster
administrator.  However this would be slightly less convenient for
administrators, could result in greater configuration variation among clusters,
and could discourage users from using the IngressClass API.

[^1]: The ingress-to-route controller generates Route objects even for Ingresses
    with empty `spec.ingressClassName`, which is allowed but discouraged in the
    API definitions.

[^2]: `Prefix` and `ImplementationSpecific` are implemented. `Exact` is not.
