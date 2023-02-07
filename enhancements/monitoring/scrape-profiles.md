---
title: scrape-profiles
authors:
  - @JoaoBraveCoding
reviewers:
  - @openshift/openshift-team-monitoring
approvers:
  - TBD
api-approvers: "None"
creation-date: 2022-12-06
last-updated: 2022-12-20
tracking-link:
  - https://issues.redhat.com/browse/MON-2483
---

# Scrape profiles

## Summary

The core OpenShift components ship a large number of metrics. A 4.12-nightly
cluster on AWS (3 control plane nodes + 3 worker nodes) currently produces
around 350,000 unique timeseries, and adding optional operators increases that
number. Users have repeatedly asked for a supported method of making Prometheus
consume less memory and CPU, either by increasing the scraping interval or by
scraping fewer targets.

These modifications are currently not possible, because the OpenShift operators
manage their service/pod monitors (either directly or via the cluster version
operator) and any manual modification would be reverted. This proposal outlines
a solution for allowing users to control the amount of data being collected
which can meet their needs.

## Motivation

OpenShift is an opinionated platform, and the default set of metrics has of
course been crafted to match what we think the majority of users might need.
Nevertheless, users have repeatedly asked for the ability to reduce the amount
of memory consumed by Prometheus either by lowering the Prometheus scrape
intervals or by modifying ServiceMonitors.

Users currently can not control the ServiceMonitors scraped by Prometheus since
some of the metrics collected are essential for other parts of the system to
function properly: alerting rules, console dashboards, resource metrics API,
horizontal/vertical pod autoscaling and Red Hat Telemetry. Users also are not
allowed to tune the interval at which Prometheus scrapes targets as this again
can have unforeseen results that can hinder the platform: a low scrape interval
value may overwhelm the platform Prometheus instance while a high interval value
may render some of the default alerts ineffective.

The goal of this proposal is to allow users to pick their desired level of
scraping while limiting the impact this might have on the platform, via
resources under the control of the cluster-monitoring-operator and other
platform operators.

Furthermore to assess the viability of the scrape profile feature the monitoring
team performed a detailed analysis of its impact in an OpenShift cluster. The
analysis consisted in a test where an OpenShift cluster would run three replicas
of the OpenShift Prometheus instance but each replica would be configured to a
different scrape profile (`full`, `minimal`). Then we would trigger a
workload using kube-burner and at the end of 2 hours, we evaluated the results.
We concluded that in terms of resource usage, given the results obtained we can
confidently state that the feature is quite valuable given the reduction of CPU
by ~21% and memory by ~33% when comparing the `minimal` to the `full` profile.
More detail can be consulted in this
[document](https://docs.google.com/document/d/1MA-HTJQ_X7y_bwpJS2IPbGmC4qDyIMp25jisr34X2F4/edit?usp=sharing)

Moreover, through Telemetry, we collect for each cluster the top 3 Prometheus
jobs that generate the biggest amount of samples. With this data, we know that
for OpenShift 4.11 the 5 components most often reported as the biggest producers
are: the Kubernetes API servers, the Kubernetes schedulers, kube-state-metrics,
kubelet and the network daemon.


### User Stories

- As an OpenShift cluster administrator, I want to lower the amount of resources
  consumed by Prometheus in a supported way, so I can choose between different
  scrape profiles, e.g `full` or `minimal`.
- As an OpenShift developer, I want a supported way to collect a subset of the
  metrics exported by my operator and operands.

### Goals

- Give users a supported and documented method to pick the amount of metrics
  being collected by the platform, in a way that allows
  cluster-monitoring-operator and other operators to provide their own
  defaulting.
- Other components of the cluster (console, HPA, alerts) should not be
  negatively affected by this change.
- Openshift developers may optionally adhere to this change, developers that do
  not adhere should not be impacted.

### Non-Goals

- Dynamically adjusting metrics scraped at runtime based on heuristics.
- Allowing users to explicitly set a list or a regex of metrics that they would
want to be included in a given profile.

## Proposal

This enhancement leverages label selectors to allow cluster-monitoring-operator
to select the appropriate monitors ([Pod|Service]Monitor) for a given scrape
profile. 

Given this, the following steps would be necessary to implement this
enhancement:

- expose a configuration option in the cluster-monitoring-operator ConfigMap
  that allows selecting the scrape profile;
- OCP components that would like to implement scrape profiles would have to
  provide X monitors. Where X is the number of scrape profiles that we would
  support.

### Workflow Description

The cluster-monitoring-operator (CMO) will be extended with a new field,
`scrapeProfile` under `prometheusK8s`. This new field will then allow users to
configure their desired scrape profile while preserving the platform
functionality (console, HPA and alerts).

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

The different profiles would be pre-defined by the OpenShift monitoring team.
Once a profile is selected CMO then updates the platform Prometheus CR to select
pod and service monitors as the union of the 2 sets:
- pod and service monitors with the 
  `monitoring.openshift.io/scrape-profile: <selected profile>` label.
- pod and service monitors without the 
  `monitoring.openshift.io/scrape-profile` profile label present.

OpenShift teams can decide if they wanted to adopt this feature. Without any
change to a ServiceMonitor, if a user picks a profile in the CMO config, things
should work as they did before. When an OpenShift team wants to implement scrape
profiles, they need to provide monitors for all profiles, making sure they do
not provide monitors without the profile label. If a team implements scrape
profiles they must ensure that all their monitors have the label
`monitoring.openshift.io/scrape-profile` set with the appropriate value,
otherwise their component might end up being double scraped or not scraped at
all.

In the beginning the goal is to support 2 profiles:

- `full` (same as today)
- `minimal` (only collect metrics necessary for alerts, recording rules,
  telemetry and dashboards)

When the cluster admin enables the `minimal` profile, the k8s Prometheus
resource would be configured accordingly:

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
 
An OpenShift team that wants to support the scrape profiles feature would need
to provide 2 monitors for each profile (in this example 1 service monitor per
profile).

```yaml
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    k8s-app: telemeter-client
    monitoring.openshift.io/scrape-profile: full
  name: telemeter-client
  namespace: openshift-monitoring
spec:
  endpoints:
  - bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    interval: 30s
    port: https
    scheme: https
    tlsConfig:
      <...>
  jobLabel: k8s-app
  selector:
    matchLabels:
      k8s-app: telemeter-client
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    k8s-app: telemeter-client
    monitoring.openshift.io/scrape-profile: minimal
  name: telemeter-client
  namespace: openshift-monitoring
spec:
  endpoints:
  - bearerTokenFile: /var/run/secrets/kubernetes.io/serviceaccount/token
    interval: 30s
    port: https
    scheme: https
    tlsConfig:
      <...>
  jobLabel: k8s-app
  selector:
    matchLabels:
      k8s-app: telemeter-client
  metricRelabelings:
  - sourceLabels: [__name__]
    action: keep
    regex: "federate_samples|federate_filtered_samples"
 ```

Note: the `metricRelabeling` section only keeping two metrics, while the rest is
dropped.

Finally, a team that adopts the scrape profile feature should also add 
themselves and their component to the list under 
[Infrastruture needed](#infrastructure-needed-optional)

#### Variation [optional]

NA

### API Extensions

- Addition of a new field to the `cluster-monitoring-config` ConfigMap

```go
type PrometheusK8sConfig struct {
  // Defines the scraping profile that will be enforced on the platform
  // prometheus instance. Possible values are full and minimal.
	ScrapeProfile string `json:"scrapeProfile,omitempty"`
}
```

### Implementation Details/Notes/Constraints

Each OpenShift team that wants to adopt this feature will be responsible for
providing the different monitors. This work is not trivial. Dependencies between
operators and their metrics exist. This makes it difficult for developers to
determine whether a given metric must be added to the "minimal" profile or not.
To aid teams with this problem we will provide a tool that would consume a
monitor and generate the "minimal" monitor for that operator. However for this,
the tool would need to have access to an up to date, installation of OpenShift
in order to query the Prometheus instance running in the cluster.

### Risks and Mitigations

- How are monitors supposed to be kept up to date? In 4.12 a metric that wasn't
  being used in an alert is now required, how does the monitor responsible for
  that metric gets updated?
  - The tool we provide would run in CI and ensure are ServiceMonitors are up to
    date;

- A new profile is added, what will happen to operators that had implemented
  scrape profiles but did not implement the latest profile.
  - The monitoring team will use the list under 
  [Infrastruture needed](#infrastructure-needed-optional) to help the developers
  with the addoption of the new scrape profile.

### Drawbacks

- Extra CI cycles

## Design Details

### Open Questions

- Should we add future profiles? 
- How will ensure that all metrics used by the console are still present?
- What happens if CMO pick-up an unsupported scrape value? (dicussion on
  https://github.com/openshift/enhancements/pull/1298#discussion_r1072266853)

### Test Plan

- Unit tests in CMO to validate that the correct monitors are being selected
- E2E tests in CMO to validate that everything works correctly Unsure on this
one... (- Testing in openshift/CI to validate that every metrics being used
exists in the cluster?)

### Graduation Criteria

Plan to release as TechPreview as the first step: the default being `full`, it shouldn't impact operations.

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

- If scrape profiles is accepted and is released to 4.13 then we must backport
  the new [Service|Pod]Monitors selectors to 4.12. The reason being that when an
  upgrade occurs and the new [Service|Pod]Monitors are deployed, we will not
  want the 4.12 Prometheus-Operator to process all the new [Service|Pod]Monitors
  as this would configure the Prometheus instances to start double scraping the
  OpenShift components that implemented the scrape profiles feature.
- Once we backport the new [Service|Pod]Monitors selectors upgrades and
  downgrades are not expected to present a significant challenge.

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

- Make CMO injecting metric relabelling for all service monitors based on the
  rules being deployed, but this is not a good idea because: 
  - CMO acting behind the back of other operators isn't probably a very good
    idea (less explicit, more brittle);
  - it's likely to be less efficient because the relabeling configs would be
    very complex and expensive in terms of processing.
  - Hypershift implements a [similar
    strategy](https://github.com/openshift/hypershift/pull/1294), but again this
    would only work for service monitors that CMO deploys directly
- Let users configure themselves the Prometheus scraping interval. This solution
  was discarded because:
  - Users might not be fully aware of the impact that changing this interval
    might have. Too low of an interval and users might overwhelm Prometheus and
    exhaust its memory. In contrast, too long of an interval might render the
    default alerts ineffective;
  - It's not advisable to increase the scrape interval to more than 2 minutes.
    Staleness occurs at 5 minutes (by default) which is would cause gaps in
    graphs/dashboards;
  - Many upstream dashboards were built while having the 30 second interval in
    mind;
  - Scrapes can fail, but the user might not be mindful of this and set a high
    scrape interval;
- Add a seperate container to prometheus-operator (p-o) that would be used by
  p-o to modify the prometheus config according to a scrape profile.
  - This container would perform an analysis on what metrics were being used.
    Then it would provide prometheus operator with this list.
  - Prometheus-operator with the list provided by this new component would know
    what scraping targets it could change to keep certain metrics.
- Recently Azure also added support for scrape profiles:
  - [Azure
    Docs](https://learn.microsoft.com/en-us/azure/azure-monitor/essentials/prometheus-metrics-scrape-configuration-minimal)
  -  https://github.com/Azure/prometheus-collector
  - In their approach they also have [hardcoded](https://github.com/Azure/prometheus-collector/blob/66ed1a5a27781d7e7e3bb1771b11f1da25ffa79c/otelcollector/configmapparser/tomlparser-default-targets-metrics-keep-list.rb#L28)
  set of metrics that are only consussumed when the minimal profile is enabled.
  However, customer are also able to extend this minimal profile with regexes to
  include metrics which might be interesting to them.

## Infrastructure Needed [optional]

### Adopted Scrape Profiles

Add the team and the component that will to adopt scrape profiles and
implementation status.
Possible implementation status: 
- considering
- implementation in progress
- implemented

| Team            | Component          | Implementation Status      |
|-----------------|--------------------|----------------------------|
| Monitoring Team | kubelet            | Implementation in progress |
| Monitoring Team | etcd               | Implementation in progress |
| Monitoring Team | kube-state-metrics | Implementation in progress |
| Monitoring Team | node-exporter      | Implementation in progress |
| Monitoring Team | prometheus-adapter | Implementation in progress |