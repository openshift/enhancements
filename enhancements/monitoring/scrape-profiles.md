---
title: scrape-profiles
authors:
  - @JoaoBraveCoding
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @openshift/openshift-team-monitoring
approvers:
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: 2022-12-06
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/MON-2483
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# Scrape profiles

## Summary

The core OpenShift components ship a large number of metrics. A 4.12-nightly
cluster on AWS currently produces around 350K series by default, and enabling
additional add-ons increases that number. Users have repeatedly asked for a supported
method of making Prometheus consume less memory, either by increasing the scraping
timeout (which is a direction we would not like to follow) or by scraping fewer targets 
-- for example modifying the ServiceMonitors to drop metrics undesired to users.

These modifications are currently not possible, because the service monitors deployed to OCP
are managed by operators and cannot be modified.
This proposal outlines a solution for allowing users to set a level of scraping aligned with their needs.

## Motivation

OpenShift is an opinionated platform, and the default set of metrics has of course 
been carefully crafted to match what we think the majority of users need. Nevertheless,
users have repeatedly asked for the ability to reduce the amount of memory consumed by
Prometheus either by removing ServiceMonitors or lowering the Prometheus scrape intervals.

Users currently can not control the ServiceMonitors scraped by Prometheus since some of the
metrics collected are essential for other parts of the system to function properly such as the
console, HPA and alerts. Users are also not allowed to tune the cadence at which Prometheus
scrapes targets as this again can have unforeseen results that can hinder the platform, a
very low cadence may overwhelm the platform Prometheus instance a very high cadence may render
some of the default alerts completely unproductive.

The goal of this proposal is to allow users to pick their desired level of scraping while limiting
the impact this might have on the platform, via resources under the control of the
cluster-monitoring-operator and other platform operators.


### User Stories

- As an OpenShift user, I want to lower the amount of memory consumed by Prometheus in a supported way, so I can choose between different scrape profiles, e.g `full` or `operational`.
- As an OpenShift developer, I want a supported way to collect a subset of the metrics exported by my operator depending on the distribution.

### Goals

- Give users a supported and documented method to pick the amount of metrics being consumed by the platform, in a way that allows the cluster-monitoring-operator and other operators to provide their own defaulting.
- Other components of the cluster (console, HPA, alerts) should not be negatively affected by this change.
- Openshift developers may optionaly adhere to this change, developers that do not adhere should not be impacted. 

### Non-Goals

- Dynamically adjusting metrics scraped at runtime based on heuristics.

## Proposal

This enhancement leverages label selectors to allow cluster-monitoring-operator to select the appropriate monitors ([Pod|Service]Monitor) for a given scrape profile. 

Given this, the following steps would be necessary to implement this enhancement:

- cluster-monitoring-operator has to be updated to support the new field logic;
- OCP component that would like to implement scrape profiles would have to provide X monitors. Where X is the number of profiles that we would support.

### Workflow Description

The cluster-monitoring-operator (CMO) will be extended with a new field, `scrapeProfile` under `prometheusK8s` this new field will then allow users to configure their desired scrape profile while preserving the platform functionality (console, HPA and alerts).

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-monitoring-config
  namespace: openshift-monitoring
data:
  config.yaml: |
    prometheusK8s:
      scrapeProfile: full 
```

The different profiles would be pre-defined by us. Once profile is selected CMO then populates the main Prometheus CR to select resources that implement the requested profile -- using [pod|service]MonitorSelector and the label `monitoring.openshift.io/scrape-profile` -- this way Prometheus would select monitors that have the label set to the respective profile or monitors that do not have the label set at all. So monitors will be picked from two sets: a monitor with the profile label and the requested label value and all monitors without the profile label present (additionally to the current namespace selector).

Afterwards this then up to monitors to implement the scrape profiles. Without any change to the monitor, even after setting a profile in the CMO config, things should work as they did before. When a monitor owner wants to implement scrape profiles, they needs to provide monitors for all profiles and no unlabeled monitors. If a profile label (`monitoring.openshift.io/scrape-profile`) is not used, then a monitor will not be scraped at all for any given profile.

In the beginning the goal is to support 2 profiles:

- `full` (same as today)
- `operational` (only collect metrics necessary for alerts, recording rules, telemetry and dashboards)

When the cluster admin enables the `operational` profile, the k8s Prometheus resource would be configured accordingly:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: Prometheus
metadata:
  name: k8s
  namespace: openshift-monitoring
spec:
  serviceMonitorSelector:
    matchExpressions:
    - key: monitoring.openshift.io/scrape-profile
      operator: NotIn
      values:
      - "full"
```
 
An OpenShift developer that would want to support the scrape profiles feature would need to provision 2 monitors for each profile (in this example 1 service monitor per profile).

```yaml
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    monitoring.openshift.io/scrape-profile: full
  name: foo-full
  namespace: openshift-bar
spec:
  endpoints:
    port: metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: foo
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    monitoring.openshift.io/scrape-profile: operational
  name: foo-operational
  namespace: openshift-bar
spec:
  endpoints:
    port: metrics
  selector:
    matchLabels:
      app.kubernetes.io/name: foo
  metricRelabelings:
  - sourceLabels: [__name__]
    action: keep
    regex: "requests_total|requests_failed_total"
 ```

#### Variation [optional]

NA

### API Extensions

- Addition of a new field to the `cluster-monitoring-config` ConfigMap

```go
type PrometheusK8sConfig struct {
  // Defines the scraping profile that will be enforced on the platform
  // prometheus instance. Possible values are full and operational.
  //
  // +kubebuilder:validation:Enum=full;operational
  // +optional
	ScrapeProfile string `json:"scrapeProfile,omitempty"`
}
```

### Implementation Details/Notes/Constraints [optional]

Each OpenShift team will be responsible for generating the different monitors, this work might not be trivial as there might be a dependecy on an operator metrics that the team supporting it is not aware. To try and help teams with this effort we would provide a tool that given a monitor it would generate the remaining monitors for the different profiles (in this case just `operational`). Undortunately for this the tool would need to have access to a fresh up to date, instalation of OpenShift.

### Risks and Mitigations

- How are monitors supposed to be kept up to date? In 4.12 a metric that wasn't being used in an alert in 4.11 is now required, how is this procedure happen?
  - The tool we provide would run in CI and ensure are ServiceMonitors are up to date

- A new profile is added, what will happen to operators that had implemented scrape profiles but did not implement the latest profile.
  - TBD

### Drawbacks

- Keeping monitors up to date.

## Design Details

### Open Questions [optional]

TBD

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

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

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

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
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

### Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

#### Failure Modes

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

#### Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Implementation History

Initial proofs-of-concept:

- https://github.com/openshift/cluster-monitoring-operator/pull/1785

## Alternatives

TBD

## Infrastructure Needed [optional]

TBD
