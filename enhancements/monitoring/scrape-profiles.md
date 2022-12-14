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
interval or by scraping fewer targets -- for example, modifying the ServiceMonitors
to drop metrics undesired to some users.

These modifications are currently not possible, because the service monitors deployed to OCP
are managed by operators and cannot be modified. This proposal outlines a solution for allowing
users to set a level of scraping aligned with their needs.

## Motivation

OpenShift is an opinionated platform, and the default set of metrics has of course 
been crafted to match what we think the majority of users might need. Nevertheless,
users have repeatedly asked for the ability to reduce the amount of memory consumed by
Prometheus either by lowering the Prometheus scrape intervals or by modifying ServiceMonitors.

Users currently can not control the ServiceMonitors scraped by Prometheus since some of the
metrics collected are essential for other parts of the system to function propperly
(console, HPA and alerts). Users also are not allowed to tune the interval at which Prometheus
scrapes targets as this again can have unforeseen results that can hinder the platform, a very
low cadence may overwhelm the platform Prometheus instance a very high interval may render some
of the default alerts ineffective.

The goal of this proposal is to allow users to pick their desired level of scraping while limiting
the impact this might have on the platform, via resources under the control of the
cluster-monitoring-operator and other platform operators.


### User Stories

- As an OpenShift user, I want to lower the amount of memory consumed by Prometheus in a supported way, so I can choose between different scrape profiles, e.g `full` or `minimal`.
- As an OpenShift developer, I want a supported way to collect a subset of the metrics exported by my operator depending on the distribution.

### Goals

- Give users a supported and documented method to pick the amount of metrics being consumed by the platform, in a way that allows cluster-monitoring-operator and other operators to provide their own defaulting.
- Other components of the cluster (console, HPA, alerts) should not be negatively affected by this change.
- Openshift developers may optionally adhere to this change, developers that do not adhere should not be impacted.

### Non-Goals

- Dynamically adjusting metrics scraped at runtime based on heuristics.

## Proposal

This enhancement leverages label selectors to allow cluster-monitoring-operator to select the appropriate monitors ([Pod|Service]Monitor) for a given scrape profile. 

Given this, the following steps would be necessary to implement this enhancement:

- cluster-monitoring-operator has to be updated to support the logic of the new field;
- OCP components that would like to implement scrape profiles would have to provide X monitors. Where X is the number of scrape profiles that we would support.

### Workflow Description

The cluster-monitoring-operator (CMO) will be extended with a new field, `scrapeProfile` under `prometheusK8s`. This new field will then allow users to configure their desired scrape profile while preserving the platform functionality (console, HPA and alerts).

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

The different profiles would be pre-defined by us. Once a profile is selected CMO then populates the main Prometheus CR to select resources that implement the requested profile -- using [pod|service]MonitorSelector and the label `monitoring.openshift.io/scrape-profile` (profile label) -- this way Prometheus would select monitors from two sets:
- monitors with the profile label and the requested label value (profile)
- monitors without the profile label present (additionally to the current namespace selector).

OpenShift teams can decide if they wanted to adopt this feature. Without any change to a ServiceMonitor, if a user picks a profile in the CMO config, things should work as they did before. When an OpenShift team wants to implement scrape profiles, they need to provide monitors for all profiles, making sure they do not provide monitors without the profile label. If a profile label is not used, then a monitor will not be scraped at all for any given profile.

In the beginning the goal is to support 2 profiles:

- `full` (same as today)
- `minimal` (only collect metrics necessary for alerts, recording rules, telemetry and dashboards)

When the cluster admin enables the `minimal` profile, the k8s Prometheus resource would be configured accordingly:

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
 
An OpenShift team that would want to support the scrape profiles feature would need to provide 2 monitors for each profile (in this example 1 service monitor per profile).

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
    monitoring.openshift.io/scrape-profile: minimal
  name: foo-minimal
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
  // prometheus instance. Possible values are full and minimal.
  //
  // +kubebuilder:validation:Enum=full;minimal
  // +optional
	ScrapeProfile string `json:"scrapeProfile,omitempty"`
}
```

### Implementation Details/Notes/Constraints [optional]

Each OpenShift team that wants to adopt this feature will be responsible for providing the different monitors. This work is not trivial. Dependencies between operators and their metrics exist. This makes it difficult for developers to determine whether a given metric must be added to the "minimal" profile or not. To aid teams with this problem we will provide a tool that would consume a monitor and generate the "minimal" monitor for that operator. However for this, the tool would need to have access to an up to date, installation of OpenShift in order to query the Prometheus instance running in the cluster.

### Risks and Mitigations

- How are monitors supposed to be kept up to date? In 4.12 a metric that wasn't being used in an alert is now required, how does the monitor responsible for that metric gets updated?
  - The tool we provide would run in CI and ensure are ServiceMonitors are up to date;

- A new profile is added, what will happen to operators that had implemented scrape profiles but did not implement the latest profile.
  - TBD. Currently, according to the description above, they would not be scraped when the new profile would be picked.

### Drawbacks

- Extra CI cycles

## Design Details

### Open Questions [optional]

- Should we add future profiles? 
- Should developers have to comply with all profiles?

### Test Plan

- Unit tests in CMO to validate that the correct monitors are being selected
- E2E tests in CMO to validate that everything works correctly
Unsure on this one... (- Testing in openshift/CI to validate that every metrics being used exists in the cluster?)

### Graduation Criteria

Plan to release as Dev Preview initially.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation
- Sufficient test coverage
- Gather feedback from users rather than just developers

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

- Upgrade and downgrade are not expected to present a significant challenge. The new field is a small layers over existing stable APIs.

### Version Skew Strategy

TBD but I don't think it applies here

### minimal Aspects of API Extensions

TBD but I don't think it applies here

#### Failure Modes

TBD but I don't think it applies here

#### Support Procedures

TBD but I don't think it applies here

## Implementation History

Initial proofs-of-concept:

- https://github.com/openshift/cluster-monitoring-operator/pull/1785

## Alternatives

- Let users configure themselves the Prometheus scraping interval. This solution was discarded since some users might not be fully aware of the impact that changing this interval migh have. Too low of an interval and users might overwelm Prometheus and exaust it's memory. In contrast, too long of an interval might render the default alerts ineffective.
- Add a seperate container to prometheus-operator (p-o) that would be used by p-o to modify the prometheus config according to a scrape profile.
  - This container would perform an analysis on what metrics were being used. Then it would provide prometheus operator with this list.
  - Prometheus-operator with the list provided by this new component would know what scraping targets it could change to keep certain metrics.

## Infrastructure Needed [optional]

None.
