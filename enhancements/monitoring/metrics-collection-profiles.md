---
title: metrics-collection-profiles
authors:
  - @JoaoBraveCoding
reviewers:
  - @openshift/openshift-team-monitoring
approvers:
  - TBD
api-approvers: "None"
creation-date: 2022-12-06
last-updated: 2023-07-24
tracking-link:
  - https://issues.redhat.com/browse/MON-2483
  - https://issues.redhat.com/browse/MON-3043
---

# Metrics collection profiles

## Terms

monitors - refers to the CRDs ServiceMonitor, PodMonitor and Probe from Prometheus Operator;

users - refers to end-users of OpenShift who manage an OpenShift installation i.e cluster-admins;

developers - refers to OpenShift developers that build the platform i.e. RedHat associates and OpenSource contributors;


## Summary

The core OpenShift components ship a large number of metrics. A 4.12-nightly
cluster on AWS (3 control plane nodes + 3 worker nodes) currently produces
around 350,000 unique timeseries, and adding optional operators increases that
number. Users have repeatedly asked for a supported method of making Prometheus
consume less memory and CPU, either by increasing the scraping interval or by
scraping fewer targets.

These modifications are currently not possible, because the OpenShift operators
manage their monitors (either directly or via the cluster-version-operator) and
any manual modification would be reverted. This proposal outlines a solution for
allowing users to control the amount of data being collected which can meet
their needs.

## Motivation

OpenShift is an opinionated platform, and the default set of metrics has of
course been crafted to match what we think the majority of users might need.
Nevertheless, users have repeatedly asked for the ability to reduce the amount
of memory consumed by Prometheus either by lowering the Prometheus scrape
intervals or by modifying monitors.

Users currently can not control the aformentioned monitors scraped by Prometheus
since some of the metrics collected are essential for other parts of the system
to function properly: recording rules, alerting rules, console dashboards, and
Red Hat Telemetry. Users also are not allowed to tune the interval at which
Prometheus scrapes targets as this again can have unforeseen results that can
hinder the platform: a low scrape interval value may overwhelm the platform
Prometheus instance while a high interval value may render some of the default
alerts ineffective.

The goal of this proposal is to allow users to pick their desired level of
scraping while limiting the impact this might have on the platform, via
resources under the control of the cluster-monitoring-operator and other
platform operators.

Furthermore, to assess the viability of the metrics collection profile feature
the monitoring team performed a detailed analysis of its impact in an OpenShift
cluster. The analysis consisted of a test where an OpenShift cluster would run
two replicas of the OpenShift Prometheus instance but each replica would be
configured to a different metrics collection profile (`full`, `minimal`). Then
we would trigger a workload using
[kube-burner](https://github.com/cloud-bulldozer/kube-burner) and at the end of
2 hours, we evaluated the results. We concluded that in terms of resource usage,
given the results obtained we can confidently state that the feature is quite
valuable given the reduction of CPU by ~21% and memory by ~33% when comparing
the `minimal` to the `full` profile. More details can be consulted in this
[document](https://docs.google.com/document/d/1MA-HTJQ_X7y_bwpJS2IPbGmC4qDyIMp25jisr34X2F4/edit?usp=sharing).

Moreover, through Telemetry, we collect for each cluster the top 3 Prometheus
jobs that generate the biggest amount of samples. With this data, we know that
for OpenShift 4.11 the 5 components most often reported as the biggest producers
are: the Kubernetes API servers, the Kubernetes schedulers, kube-state-metrics,
kubelet and the network daemon.


### User Stories

- As a user, I want to lower the amount of resources consumed by Prometheus in a
  supported way, so I can configure the clusters metrics collection profiles to
  `minimal`.
- As a developer, I want a supported way to collect a subset of the metrics
  exported by my operator and operands, while still collecting necessary metrics
  for alerts, visualization of key indicators and Telemetry.
- As a developer of a component (that does not yet implement a profile), I want to
  extract metrics needed to implement said profile, based on the assets I
  provide, or the ones gathered from the cluster based on a group of target
  selectors, and a plug-in relabel configuration to apply within the monitor.
- As a component owner (that does not, or only partially implements a profile),
  I want to get information about any monitors that are not yet implemented for
  any of the supported profiles that are offered.
- As a component owner (that implements a profile), I want to verify if all the
  profile metrics are present in the cluster, and which of the profile monitors
  are affected if not. Also, I want additional information to narrow down where
  these metrics are exactly being used.

### Goals

- Give users a supported and documented method to pick the amount of metrics
  being collected by the platform, in a way that allows
  cluster-monitoring-operator and other operators to provide their own
  defaulting.
- Other components of the cluster (alerts, console, HPA and VPA and telemetry)
  should not be negatively affected by this feature.
- Developers may optionally adhere to this feature, those that do not adhere
  should not be impacted.

### Non-Goals

- Dynamically adjusting metrics scraped at runtime based on heuristics.
- Allowing users to explicitly set a list or a regex of metrics that they would
want to be included in a given profile.
- Support the addition of profiles other than `full` and `minimal`.

## Proposal

This enhancement leverages label selectors to allow cluster-monitoring-operator
to select the appropriate monitors for a given metrics collection profile. 

Given this, the following steps would be necessary to implement this
enhancement:

- expose a configuration option in the cluster-monitoring-operator ConfigMap
  that allows selecting the metrics collection profile;
- OCP components that would like to implement metrics collection profiles would
  have to provide at least 1 monitor for each supported metrics collection
  profile.

### Workflow Description

The cluster-monitoring-operator (CMO) will be extended with a new field,
`collectionProfile` under `prometheusK8s`. This new field will then allow users
to configure their desired metrics collection profile while preserving the
platform functionality (alerts, console, HPA and VPA and telemetry).

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-monitoring-config
  namespace: openshift-monitoring
data:
  config.yaml: |
    prometheusK8s:
      collectionProfile: full 
```

The different profile names would be pre-defined by the OpenShift monitoring
team. Once a profile is selected CMO then updates the platform Prometheus CR to
select monitors as the union of the 2 sets:
- monitors with the 
  `monitoring.openshift.io/collection-profile: <selected profile>` label.
- monitors without the `monitoring.openshift.io/collection-profile` profile
  label present, to retain the default behaviour (for components that didn't
  opt-in for metrics collection profile).

OpenShift teams can decide if they want to adopt this feature. Without any
change to a monitor, if a user picks a profile in the CMO config, things
will work as they did before. When an OpenShift team wants to implement
metrics collection profiles, they need to provide monitors for all profiles,
making sure they do not provide monitors without the profile label. If a team
implements metrics collection profiles they must ensure that all their monitors
have the label `monitoring.openshift.io/collection-profile` set with the
appropriate value, otherwise their component might end up being double scraped
or not scraped at all.

The goal is to support 2 profiles:

- `full` (same as today)
- `minimal` (only collect metrics necessary for recording rules, alerts,
  dashboards, HPA and VPA and telemetry)

When the cluster admin enables the `minimal` profile, the Prometheus
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
    - key: monitoring.openshift.io/collection-profile
      operator: NotIn
      values:
      - "full"
```
 
An OpenShift team that wants to support the metrics collection profiles feature
would need to provide 2 monitors for each profile (in this example 1
ServiceMonitor per profile).

```yaml
---
apiVersion: monitoring.coreos.com/v1
kind: ServiceMonitor
metadata:
  labels:
    k8s-app: telemeter-client
    monitoring.openshift.io/collection-profile: full
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
    monitoring.openshift.io/collection-profile: minimal
  name: telemeter-client-minimal
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

Note: 
- the `metricRelabeling`s section keeps only two metrics, while the rest is
dropped.
- the metrics in the `keep` section were obtained with the help of a script that
  parsed all Alerts, PrometheusRules and Console dashboards to determine what
  metrics were actually being used.

Finally, a team that adopts the metrics collection profile feature should also 
add themselves and their component to the list under 
[Infrastruture needed](#infrastructure-needed-optional)

#### CLI Utility

Incorporating the CLI utility into the workflow in order to better leverage this
feature offers the following functionalities (based on the various options
offered):
* Metric extraction: Metrics can be extracted from the rule file, as well as the
  cluster based on the target selectors provided, while allow-listing a set of
  metrics, if need be, that will always be included within the metrics regex of
  the [generated relabel
  configuration](https://github.com/rexagod/cpv/blob/40acb00abedd25bcd9cebfbc19c897766470cb96/internal/profiles/minimal_extractor.go#L231).
* Cardinality statistics: Output the cardinality of every single metric that was
  included in the aforementioned relabel configuration, i.e., every single
  metric that was extracted using any of the three methods.
* Profile validation: Verify if the set of metrics used within the range of
  monitors that were used to collectively implement a profile (for different
  components) are all available at the given endpoint, and output the queries,
  rules, groups, and profile monitors that the absence of these metrics affects.
* Implementation status: Get the profile implementation status in terms of which
  monitors need to be created in order to completely implement that profile, for
  every single default profile monitor to eventually have a corresponding custom
  monitor that implements said profile.

#### Variation [optional]

NA

### API Extensions

- Addition of a new field to the `cluster-monitoring-config` ConfigMap

```go
type PrometheusK8sConfig struct {
	// Defines the metrics collection profile that Prometheus uses to collect
	// metrics from the platform components. Supported values are `full` or
	// `minimal`. In the `full` profile (default), Prometheus collects all
	// metrics that are exposed by the platform components. In the `minimal`
	// profile, Prometheus only collects metrics necessary for the default
	// platform alerts, recording rules, telemetry and console dashboards.
	CollectionProfile CollectionProfile `json:"collectionProfile,omitempty"`
}
```

### Implementation Details/Notes/Constraints

Each OpenShift team that wants to adopt this feature will be responsible for
providing the different monitors. This work is not trivial. Dependencies between
operators and their metrics exist. This makes it difficult for developers to
determine whether a given metric can be excluded from the `minimal` profile or
not. To aid teams with this effort the monitoring team will provide:
- a CLI tool that offers a suite of operation to make it easier for developers
  to utilize all aspects of this feature into their component's workflow.
- an origin/CI test that validates for all Alerts and PrometheusRules that the
  metrics used by them are present in the `keep` expression of the
  monitor for the `minimal` profile


### Risks and Mitigations

- How are monitors supposed to be kept up to date? In 4.12 a metric that wasn't
  being used in an alert is now required, how does the monitor responsible for
  that metric gets updated?
  - The origin/CI test mentioned in the previous section will fail if there is a
    resource (Alerts/PrometheusRules/Dashboards) using a metric which is not
    present in the monitor in question;

- What happens if a user provides an invalid value for a metrics collection profile?
  - CMO will reconcile and validate that the value supplied is invalid and it
    will report Degraded=False and fail reconciliation.

- Should we add future profiles? How would we validate such profiles?
  - Our current validation strategy with only two profiles is quite linear,
    however, things start becoming more complex and hard to mainain as we
    introduce new profiles to the mix. 
  - Some of the things to consider if new profiles are introduce are:
      - How would we validate such profile?
      - How would we ensure teams that adopted metrics collection profiles
        implement the new profile?
      - How would we aid developers implementing the new profile?

### Drawbacks

- Extra CI cycles

## Design Details

### Open Questions

### Test Plan

- Unit tests in CMO to validate that the correct monitors are being selected
- E2E tests in CMO to validate that everything works correctly 
- For the `minimal` profile, origin/CI test to validate that every metric used
in a resource (Alerts/PrometheusRules/Dashboards) exist in the `keep` expression
of a minimal monitors.

### Graduation Criteria

- Released as TechPreview: the default being `full`, it
shouldn't impact operations.

- Plan to GA: this would officially support the "minimal"
profile out-of-the-box and removes the earlier-imposed
TechPreview gate. PTAL at the section below for more details.

#### Tech Preview -> GA

- [Automation to update metrics in collection profiles](https://issues.redhat.com/browse/MON-3106)
- [Telemetry signal for collection profile usage](https://issues.redhat.com/browse/MON-3231)
- [Enhancement proposal for metrics collection profile](https://issues.redhat.com/browse/MON-2692)
- [CLI tool to facilitate implementation of metrics collection profiles for OCP components](https://issues.redhat.com/browse/MON-2694)
- [Drop TechPreview gate on collection profiles](https://issues.redhat.com/browse/MON-3215)
- [origin/CI tool to validate collection profiles](https://issues.redhat.com/browse/MON-3105)
- [User facing documentation created in openshift-docs](https://issues.redhat.com/browse/OBSDOCS-330)

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

- If metrics collection profiles is accepted and is released to 4.13 then we
  must backport the new monitors selectors to 4.12. The reason being that when
  an upgrade occurs and the new monitors are deployed, we will not want the 4.12
  Prometheus-Operator to process all the new monitors as this would configure
  the Prometheus instances to start double scraping the OpenShift components
  that implemented the metrics collection profiles feature. Done in
  [cluster-monitoring-operator#2047](https://github.com/openshift/cluster-monitoring-operator/pull/2047)
- Once we backport the new monitors selectors upgrades and
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
  p-o to modify the prometheus config according to a metrics collection profile.
  - This container would perform an analysis on what metrics were being used.
    Then it would provide prometheus operator with this list.
  - Prometheus-operator with the list provided by this new component would know
    what scraping targets it could change to keep certain metrics.
- Recently Azure also added support for metrics collection profiles:
  - [Azure
    Docs](https://learn.microsoft.com/en-us/azure/azure-monitor/essentials/prometheus-metrics-scrape-configuration-minimal)
  -  https://github.com/Azure/prometheus-collector
  - In their approach they also have [hardcoded](https://github.com/Azure/prometheus-collector/blob/66ed1a5a27781d7e7e3bb1771b11f1da25ffa79c/otelcollector/configmapparser/tomlparser-default-targets-metrics-keep-list.rb#L28)
  set of metrics that are only consumed when the minimal profile is enabled.
  However, customer are also able to extend this minimal profile with regexes to
  include metrics which might be interesting to them.

## Infrastructure Needed [optional]

### Adopted metrics collection profiles

Add the team and the component that will to adopt metrics collection profiles
and implementation status. Possible implementation status: 
- considering
- implementation in progress
- implemented

| Team            | Component          | Implementation Status      |
|-----------------|--------------------|----------------------------|
| Monitoring Team | kubelet            | Implemented                |
| etcd Team       | etcd               | Implemented                |
| Monitoring Team | kube-state-metrics | Implemented                |
| Monitoring Team | node-exporter      | Implemented                |
| Monitoring Team | prometheus-adapter | Implemented                |
