---
title: Improving Supportability of API Webhooks
authors:
- "@sttts"
reviewers:
- "@mfojtik"
- "@tkashem"
- "@p0lynomial"
- "@deads2k"
approvers:
- "@mfojtik"
creation-date: 2021-09-03
last-updated: 2021-10-06
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# Improving Supportability of API webhooks

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in
      [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

OpenShift supports the API extensions that upstream Kubernetes has built into the kube-apiserver.
This especially involves webhooks for admission and for CRD conversion. This enhancement is about
measures to increase supportability of clusters that use official Red Hat provided add-ons (like
Istio, kubevirt or Open Policy Agent), but also 3rd-party add-ons (like Vault, or the community
variants of previously mentioned software).

API extension webhooks can have negative impact on a cluster, from performance, over stability and
security to availability. Customer case escalations of early adopters have shown that these concerns
are very real and supportability is at risk.

A formal tainting mechanism with influence on the support status of a cluster is not desired by the
OpenShift organization.

Hence, we propose:
- a number of **alerts** to warn about unreliability of webhooks or performance problems
- a number of **informational alerts** notifying the admin about 3rd-party software that installs
  webhooks on critical resources
- a non-centralized "soft" **approval process** (similar to the use of `k8s.io` group names in CRDs
  in upstream) for webhooks hosted in `openshift-*` namespaces for core resources
- an **extension of OpenShift enhancement template** about webhooks, their impact to the cluster,
  possible debugging strategies and supportability concerns
- a **monitoring dashboard** to watch the latency of individual webhooks and total latency for all
  webhooks per resource.
- to **skip 3rd-party admission webhooks on runlevel=0 namespaces** by default, with an explicit
  opt-in per admission webhook to bypass the restriction.

This enhancement is about admission webhook and conversion webhook. They have an overlap of
characteristic, but also distinct threat for a cluster.

## Motivation

As seen in escalations of early adopters of new technology on-top of OpenShift like service meshes
or policy engines like OPA, we know that clusters with webhooks installed for core resources like
pods or `SubjectAccessReview` are at risk and a supportability problem. The triage is hard or
impossible if these are installed, and often the webhooks misbehavour is even the root cause for the
problems. Examples are:

https://bugzilla.redhat.com/show_bug.cgi?id=1998436
https://bugzilla.redhat.com/show_bug.cgi?id=1998245

Hence, we have learned that API extension webhooks can have negative impact on
- the **performance** of a cluster because API requests involve additional network round-trips to
  another service, or a webhook with side-effect causes a reevaluation of the admission chain, or
  intermediate requests like `SubjectAccessReviews` require yet another webhook call.
- the **stability** of a cluster because API requests can fail either because the webhook
  intentionally rejects requests, or the webhook request fails for infrastructure reasons (and
  failure policy `Fail` is used), or the webhook times out requests (because it is overloaded or for
  infrastructure reasons)
- the **security** of a cluster because API requests can include security sensitive information like
  tokens or secrets. The webhook will see these in plain-text and hence become a security risk for
  the cluster.
- the **availability** of a cluster because API requests that fail can render resource-agnostic
  controllers like garbage collection, namespace controller or resource quota disfunctional for all
  resources in a cluster, not only those the webhook is about.

### Goals

- allow the customer, support and engineering in escalations to notice misbehaving webhooks
- make the customer aware about risks of installing (multiple) add-ons that hook deeply into core
  components
- make Red Hat teams aware early about the risk of hooking into core resources and make them design
  their architecture with minimal impact
- establish an Red Hat approval process for webhooks hooking into core resources
- establish a culture of "webhook impact budget" in the development process.

### Non-Goals

- tainting a cluster with effect on the support status
- forbidding webhooks for OpenShift add-ons
- out-of-scope: proposing alerts for kube-controller-manager, which can stop from doing its work
  (GC, namespace controller, quota) with unstable webhooks (especially conversion webhooks)
- out-of-scope: creating alerts for aggregated apiservers

## Proposal

### User Stories

1. As a cluster-admin I want to be warned if core resource webhooks risk the cluster health.
2. As a cluster-admin I want to be warned if webhooks are slow and risk the user-experience or even
   the availability of non-core resources (CRDs).
3. As a cluster-admin I want to be notified and acknowledge the risk of core resource webhooks that
   are being installed.
4. As a development organization, we want to develop architectures that do not harm or minimally
   harm the stability, performance, availability and security of OpenShift.
5. As a development organization, we don't want to stop innovation and hence allow a decentralized
   development process of OpenShift add-ons, but establish a culture where best-practices are
   communicated (e.g. in architecture reviews) and applied.

### Existing Metrics

Upstream Kubernetes provides the following webhook specific metrics:

- histogram `webhook_admission_duration_seconds(name, type, operation, rejected)` for per-webhook latency
- histogram `controller_admission_duration_seconds(type, operation, rejected)` for the summed up webhook latencies
- counter `webhook_rejection_count(name, type, operation, error_type, rejection_code)` for
  per-webhook rejections, which includes connection errors and explicit rejection responses.

Here,
- the type is `admit` or `validating`,
- the operation is the http verb (`CREATE`, `UPDATE`, `DELETE`, `CONNECT`),
- the error type is `calling_webhook_error`, `apiserver_internal_error` or `no_error` (with
  rejection code the http result code).

The rejection count does not give us information about the error rate.

Neither of the metrics gives us direct information about the GroupVersionResource, only about the
admission registration object name in `webhook_admission_duration_seconds` and by that one can infer
the affect resources. Notably, `controller_admission_duration_seconds` is not split by resource type
in any way and hence just a cluster-global metric, and with that mostly useless.

### Critical Alerts

Based on the existing metrics from above, we will add the following critical alerts:

- alert about latency per webhook `>1s` (`0.5s` for critical resources)
- alert about failure count > 1% (both conversion and admission)

We would like to have

- alert about total webhook latency per resource >1s (0.5 for critical resources)

but likely cannot implement that because of the lack of resource label (see section above). We have
to discuss alternatives (new labels or new metric) upstream.

Critical resources is a static set of resources that are critical to the operation of the cluster
and likely bottlenecks for scalability and stability if admission is installed on them
unconditionally (labelled with "performance" in the [Critical OCP API Resource
spreadsheet](https://docs.google.com/spreadsheets/d/18A-bGQTcXrCB_sHAHBtzmsz0ma9fvFBtt5PY-VZ8T-Y)):

- endpoints
- events
- pods
- resourcequotas
- apirequestcounts
- endpointslices
- clusterresourcequotas.

### Degraded Operator

The kube-apiserver operator will go degraded

- if webhooks for virtual resources (SubjectAccessReview, ...) exist
- if webhooks's service does not exist, or has no endpoints.

Virtual resources are those that are not persisted in etcd (labelled as "virtual" in the
[Critical OCP API Resource spreadsheet](https://docs.google.com/spreadsheets/d/18A-bGQTcXrCB_sHAHBtzmsz0ma9fvFBtt5PY-VZ8T-Y))):

- bindings
- `*`reviews.

We are considering to make kube-controller-manager operator go degraded in parallel because
garbage collection and other dynamic controllers will stop working when not all resources are
readable because of missing CRD conversion webhook services. This happened many times in
customer clusters in the past, and today can only be diagnosed by looking into kube-apiserver logs.
But connecting to dots between broken garbage collection and a missing service is hard.

Note: OLM today does intentionally not delete CRDs on operator uninstall. With this enhancement in place
every cluster that has a dangling webhooks service reference will degrade. Hence, a `olm uninstall <operator-with-CRD-conversion>`
will break the cluster. Also note that the cluster is already broken in that situation. So to degrade
is the correct behaviour, not a regression.

### Informational Alerts

- webhook installed for core resources that are not whitelisted (via soft approval; restricted to
  openshift-`*` namespaces)
- webhook installed for security-sensitive resources  (TokenReviews, oauth tokens+clients) that are
  not whitelisted (including non-openshift-`*` components).

  This alert is not expressible with the metrics we have. We have to find a way either via
  artificial metrics from the operator or by moving to some condition instead.

Core resources are those shipped in a stock OCP cluster.

Security sensitive resources (labelled with "security" in the [Critical OCP API Resource
spreadsheet](https://docs.google.com/spreadsheets/d/18A-bGQTcXrCB_sHAHBtzmsz0ma9fvFBtt5PY-VZ8T-Y)):

- secrets
- serviceaccounts
- mutatingwebhookconfigurations
- validatingwebhookconfigurations
- tokenreviews
- certificatesigningrequests
- credentialsrequests
- oauthaccesstokens
- oauthauthorizetokens
- oauthclientauthorizations
- oauthclients
- useroauthaccesstokens
- routes.

### "Soft" Approval Process

For RH-internal add-ons (those whose service lives in `openshift-*` namespaces) we add a
```yaml
webhook-approved.openshift.io: https://github.com/openshift/enhancement/pull/<pr>
```
annotation on CRDs with webhook conversion and admission registrations objects (similarly to
`api-approved.openshift.io`). This will work as a white-list marker to hook into core
resources. These webhooks are excluded from the informational alerts for core resources of the
previous section. No other action is taken beyond the informational alert i.e. to stop those
webhooks to work.

### Extension of OpenShift Enhancement Template

We had a section to the [OpenShift enhancement template]
(https://github.com/openshift/enhancements/blob/master/guidelines/enhancement_template.md) about API
extensions:

```yaml
### API Extensions

- name the API extensions (webhooks, aggregated API servers) this enhancement adds or modifies
- does this enhancement modify the behaviour of existing resource and if yes how

  Examples:
  - adds a finalizer to namespaces. Namespace cannot be deleted without our controller running.
  - restricts the label format for objects to X.
  - defaults field Y on object kind Z.
- what are the SLIs (Service Level Indicators) an operator can use to determine the health of
  the API extensions

  Examples: metrics, alerts, operator conditions
- which impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - add 1s to every pod update in the system
  - fails creation of ConfigMap in the system when the webhook is not availiable
  - adds a dependency on the SDN service network for all resources, risking API availability in case of SDN issues
- how is the upper impact to be measured and when (e.g. every release by QE, or automatically in CI)
  and by whom (e.g. perf team; name the responsible person and let them review this enhancement)

#### Failure Modes

- describe the possible failure modes of the API extensions
- describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- describe which teams are probably called out in case of escalation with one of the failure modes
  and add them as reviewer to the enhancement.

#### Support Procedures

describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)
- disable the API extension
  - which consequences does it have on the cluster health?
  - which consequences does it have on existing, running workloads?
  - which consequences does it have for newly created workloads?
  - does functionality fail gracefully and will resume work when re-enabled without risking
    consistency?
```

We add `api-approver: <name>` to the top-level section and mandate in the enhancement process that
an API approver must be named there and has to approve these enhancements in addition to the usual
enhancement approver. The people who are API approvers in the OpenShift organization are defined by
the enhancement process.

### Monitoring Dashboard

We add new metric figures to the API dashboard in the console:

1. per webhook (both conversion and admission) latency
1. per webhook (both conversion and admission) failure rate
1. per webhook (both conversion and admission) rejection rate

We would like to add the following:

1. per resource (both conversion and admission) total on average
1. per resource (both conversion and admission) total failure rate (of any webhook in the chain)

but without suggesting new labels or a new metrics we won't have this information.

Out of scope: aggregated API server metrics.

### Skip 3rd-party Admission Webhooks on runlevel=0 Namespaces

Admission webhooks today can configure a [namespace selector](https://github.com/kubernetes/api/blob/7036ead253974a37c7aca010f305593a486a253e/admissionregistration/v1/types.go#L263)
to restrict the namespaced object the webhook applies to. We will implicitly add logic to exclude
namespaces with `openshift.io/run-level: "0"` label. A kube-apiserver admission plugin (no webhook,
but via carry patch in `openshift-kube-apiserver` package) will add
```yaml
labelSelector:
  matchExpressions:
  - key: openshift.io/run-level
    operator: NotIn
    values:
    - "0"
```
to the registration object.

Note: this might potentially break reconciliation loops like Helm, CVO or OLM to hot-loop
(repeatedly trying to apply the manifest). Proper reconciler logic has to cope with mutating
admission, e.g. by using server-side-apply or by comparing the in-cluster manifest with the
previously created object. Hence, this hot-looping would be considered a bug in those tools
and would need fixing.

For OpenShift specific add-ons which are aware of the OpenShift specific topology of the control-plane
(like static pods and self-hosting) we provide an opt-in mechanism to apply to namespaces with
`openshift.io/run-level: "0"`. For that the admission registration object must carry an annotation
```yaml
webhook.openshift.io/run-level-0-opt-in: "true"
```

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For example, consider
both security and how this will impact the larger OKD ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Core Resources

Core resources are those shipped in a stock OCP cluster, specified through
1. `api-approved.openshift.io`-annotated CRDs enforced in openshift/api signaling they are part of
   OCP,
3. a hard-coded list of API groups on-top of 1.

Completeness of that list is verified through an origin e2e test.

### Open Questions [optional]

This is where to call out areas of the design that require closure before deciding to implement the
design.  For instance, > 1. This requires exposing previously private resources which contain
sensitive information.  Can we do this?

### Test Plan

- e2e with

    1. a broken conversion webhook
       - with non-existing service
       - with failure rate of 10%
       - without endpoints
    2. a broken admission webhook
       - with non-existing service
       - with failure rate of 10%
       - without endpoints
    3. admission webhooks on
       - virtual resources
       - security-sensitive resources
       - with and without webhook approval annotation

  in both cases verify that the expected alerts fire eventually and that the operator goes degraded.
- e2e with a failing, failure policy pod webhook in `openshift-kube-apiserver-operator` and
  `openshift-kube-apiserver namespaces` checking that pods in these namespace can still be created.


### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal should keep
this high-level with a focus on what signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc
definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning), or by
redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is
accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**Examples**: These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

**For non-optional features moving to GA, the graduation criteria must include end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this is in the test
plan.

Consider the following in developing an upgrade/downgrade strategy for this enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing cluster required to
  make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing cluster required to
  make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and workloads during upgrades. Ensure the
  components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a minor release
  stream without being required to pass through intermediate versions - i.e. `x.y.N->x.y.N+2` should
  work without requiring `x.y.N->x.y.N+1` as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade steps. So, for example, it
  is acceptable to require a user running 4.3 to upgrade to 4.5 with a `4.3->4.4` step followed by a
  `4.4->4.5` step.
- While an upgrade is in progress, new component versions should continue to operate correctly in
  concert with older component versions (aka "version skew"). For example, if a node is down, and an
  operator is rolling out a daemonset, the old and new daemonset pods must continue to work
  correctly even while the cluster remains in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is misbehaving, it should be
  possible for the user to rollback to `N`. It is acceptable to require some documented manual steps
  in order to fully restore the downgraded cluster to its previous state. Examples of acceptable
  steps include:
  - Deleting any CVO-managed resources added by the new version. The CVO does not currently delete
    resources that no longer exist in the target version.

### Version Skew Strategy

How will the component handle version skew with other components?  What are the guarantees? Make
sure this is in the test plan.

Consider the following in developing a version skew strategy for this enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and in the kubelet? How
  does an n-2 kubelet without this feature available behave when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI or CNI may require
  updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to highlight and record other
possible approaches to delivering the value proposed by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new subproject, repos
requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources started right away.
