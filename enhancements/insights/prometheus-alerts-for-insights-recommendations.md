---
title: prometheus-alerts-for-insights-recommendations
authors:
  - "@natiiix"
reviewers:
  - "@tremes"
  - "@inecas"
  - "@bparees"
  - "@simonpasquier"
  - "@sdodson"
  - "@wking"
approvers:
  - "@tremes"
  - "@bparees"
api-approvers: # in case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers)
creation-date: 2022-02-14
last-updated: 2022-03-31
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CCXDEV-6653
see-also:
replaces:
superseded-by:
---

# Insights Operator Prometheus Alerts for Insights Recommendations

## Summary

Insights recommendations are triggered based on data in the archives
periodically uploaded by the Insights Operator. A report about these
recommendations is available through the Insights Smart Proxy, and it is
currently fetched by the Insights Operator after each successful archive
upload in order for the Insights Operator to update Prometheus metrics
containing the number of currently active recommendations in each severity
category.

This enhancement aims to extend this functionality by not only making the
number of recommendations available, but also providing specific information
about each of the recommendations via Prometheus alerts. The currently
suggested approach expects one info-level alert per active Insights
recommendation, containing its ID, human-readable name, severity, and a link
to detailed information about the recommendation and its remediation steps.

## Motivation

Users are currently not being immediately notified about newly active Insights
recommendations. If they do not check the Insights tab periodically, these
recommendations may go unnoticed for a long time during which the underlying
issue escalates instead of being resolved. Making the recommendations available
through the Prometheus alerts makes it easier for users to define how they want
to receive information about newly active recommendations (via an email, Slack
message, etc.).

### Goals

- New Prometheus metric with labels containing aforementioned properties of
  the recommendation that will indicate if the recommendation is currently active.
- New Prometheus alert based on this metric, defined using a template that fills
  the information from labels into the description of the alert.
- The user should be able to find out when was a certain recommendation active
  by querying the Prometheus metric in the OCP console.
- Configuration option allowing the user to disable/enable this functionality.
  Since this feature consumes resources on both sides (cluster and Smart Proxy),
  it should only be enabled on clusters where the user actually wants to use it.
  (It should be disabled by default, unless the user explicitly enables it.)
- The enabling should be done using the same approach as the rest of the
  Insights Operator configuration (i.e., currently using the `support` secret).

### Non-Goals

- Insights Operator fetching additional data, from the Smart Proxy or any other source.
- Modifications or additions to the user notification systems (the goal is to
  utilize the existing notification systems based on Prometheus alerts - most
  notably Alertmanager).

## Proposal

### User Stories

As a user, I want to be immediately notified about a recommendation that has
become active on my cluster without having to periodically check the Insights
tab. I want to receive notifications through my preferred channel (e.g., Slack,
email) configured via Alertmanager when a recommendation becomes active, and
another one when it becomes inactive again.

### Why should this be implemented in Insights Operator?

The Insights Operator is already fetching the relevant data from the Smart Proxy,
and managing Prometheus metrics based on this data. Therefore, it makes sense
for additional information about recommendations to be implemented alongside
what already exists.

### API Extensions

N/A

### Implementation Details/Notes/Constraints [optional]

Please note that IO refers to the Insights Operator in this context.

0. IO is enabled and periodically uploads archives to the Ingress service, which
   are then processed by the CCX data pipeline. (This is the existing behavior,
   which will not have to be modified in any way.)
0. After a successful archive upload, IO waits for the report to become
   available, fetches it from the Smart Proxy and processes it. (This is an
   extension of the current behavior, where we only look at the number of
   active recommendations per severity level.)
0. Prometheus metrics indicating the status of recommendations being active are
   set accordingly. Only metrics for active recommendations are reported during
   the metric collection. If a recommendation is no longer active, the metric
   will not be collected, and the in-cluster Prometheus is expected to delete
   it, which in turns should cancel the corresponding alert.
0. The metric will have labels containing the ID, severity (based on the total
   risk property), a human-readable name, and a link to a detailed description
   of the recommendation and its remediation steps (Insights Advisor URL
   generated based on the cluster ID and rule ID by IO) and its value will be
   set to the timestamp from the latest report with the recommendation active.
1. There will be a generic/templated info-level alert present on the cluster,
   which will present the user with brief information about the hitting
   recommendation (its human-readable name and severity),
   and the link for details and remediation steps.

It is worth pointing out that if any key part of the Prometheus stack is not
running as expected on the cluster, then this feature will not work. Prometheus
manages both the metrics and the alerts, so without it, no alerts can be
generated by IO.

An example of what the Insights recommendation metric may look like:
```text
insights_recommendation_active{rule_id="ccx_rules_ocp.external.rules.empty_prometheus_db_volume.report",error_key="PROMETHEUS_DB_VOLUME_IS_EMPTY",severity="moderate",description="Prometheus metrics data will be lost when the Prometheus pod is restarted or recreated",info_url="http://example.com/"} 1646416494
```

**Unhealthy Insights Operator scenario:**

- If something goes wrong with IO, we would not want the Insights alerts to
  remain hanging on the cluster indefinitely.
- The alert should only fire if IO has been healthy at any point in the past 10
  minutes. Healthy, in this context, means that there is no major error
  happening (crashlooping, degraded operator status, etc.). A temporary error
  state should not cause the alerts to be disabled.
- If the user intentionally disables IO, the alerts should never fire. If IO is
  disabled while some of the Insights alerts are firing, the alerts should
  immediately stop firing (the in-cluster will detect that IO is not active
  during its period scraping, which will remove the Insights metrics from its
  database).
- If IO was disabled by an accident, the user will be notified via the existing
  `InsightsDisabled` alert, which is defined as a part of the Insights Operator.

**Unhealthy Smart Proxy scenario:**

If the processing of a successfully uploaded archive takes unexpectedly long,
the Insights Operator will have the `InsightsDownloadDegraded` cluster operator
condition. This would most likely be detected from the code that performs the
Insights report fetching. If it is not possible to get an up-to-date report
after 10 minutes since the upload, the condition will be set. Once the report
becomes available, the condition will be reset.

An alert based on this condition will be added to indicate a delay in the
processing of the uploaded archive, so that the user is aware of potential
outdated information in the Insights recommendation alerts.

Furthermore, the Insights recommendation metrics will use a timestamp as their
value to indicate the time when the recommendation was last reported as active
(the timestamp of the most recent report that contained this recommendation).

### Risks and Mitigations

- Disabling and re-enabling an alert (e.g, due to IO or Smart Proxy outage)
  can trigger user-defined actions, even though there has not been any actual
  change to the status of the recommendation. This could be very annoying if
  it were to happen often or repeatedly over a short period of time.
- The URL to detailed information about an active Insights recommendation will
  be hard-coded into the alert template or the Insights Operator. If the
  endpoint moves elsewhere, the URL will be broken and an updated version of the
  alert template will have to be backported. The process can take several
  months, and there is no guarantee that the user will perform the update.

## Design Details

### Open Questions [optional]

1. ~~Which operator conditions should fall under the unhealthy Insights Operator
   state?~~ This does not have to be explicitly specified because the alerts
   will automatically be disabled when the Insights Operator stops working
   correctly because the reporting of the relevant metrics will stop.
1. ~~How long should IO wait before considering a report to be no longer valid
   (outdated)?~~ The data about active Insights recommendations are valid for 8
   hours since the timestamp contained in the report.
1. ~~If uploads keep failing, how and when will the alerts be disabled? (because
   the current design assumes that the metrics are only modified after receiving
   a report after a successful upload)~~ The alerts will depend on the metrics,
   so once the metrics stop being reported to the in-cluster Prometheus, the
   alerts will be automatically disabled. This will happen when the Insights
   recommendation data become outdated (see above).

### Test Plan

Due to its nature, this feature will probably be best tested using e2e tests
that involve other components. The Prometheus stack will be necessary to
evaluate the metrics and alerts.

The e2e tests for default functionality could work as follows:
- break some property of the cluster,
- have the Insights Operator gather data about it,
- upload the Insights archive,
- allow the CCX data pipeline to process the archive,
- check the resulting metrics/alerts.

To test the alert behavior when Insights are disabled:
- make sure Insights are enabled,
- cause a report including at least one recommendation to be generated,
- verify that the Insights alert is created,
- disable Insights,
- ensure that the latest report still includes at least one recommendation,
- verify that the Insights alert is not present.

Another test could check that restarting the Insights Operator pod will not
cause the alerts to be re-triggered (to be disabled when IO is terminated and
re-enabled when IO comes back online). If it happened, it could trigger
used-defined actions, even though the state of the Insights recommendation has
not changed.

Testing the `InsightsDownloadDegraded` condition:
- let the Insights Operator upload an archive while the Smart Proxy is down,
- wait 10 minutes after the upload,
- IO should have the `InsightsDownloadDegraded` condition set,
- check that the Insights report delay alert is created (see the "Unhealthy
  Smart Proxy scenario" section for details).

### Graduation Criteria

N/A

This feature does not introduce a new API.

There will first be a PoC to verify viability of the feature, and then the next
step will be the full implementation ready for release.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Downgrading to an older version will remove the alert template definition, if
the version is from before the introduction of this feature.

Upgrading to a newer version may result in the alert template being updated.

In general, this feature only adds new alerts to the cluster, which can be then
processed into various notifications. If the alerts are removed or modified, it
will not break other components. At worst, it will break user-defined actions
based on these alerts, but that should have no impact on the cluster.

### Version Skew Strategy

This feature will be shipped as a part of the Insights Operator, which is its
only version-sensitive dependency. While it relies on the Prometheus stack, it
does not depend on a specific version of it.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

If there are active Insights recommendations, but no alerts are firing, check
the status of the Prometheus stack and the Insights Operator. If both of them
are enabled, running, and not in a degraded state, then the alerts should be
generated. If the Smart Proxy is not working, then the Insights recommendations
would not be available through other interfaces (i.e., Insights Advisor) either.

If an Insights recommendation is acting suspiciously (e.g., being active when it
should not be, or coming on and off repeatedly without any change to cluster
configuration), a member of the CCX team should be notified. Conversely, if a
recommendation should be active but is not, then the status of the Insights
Operator and the CCX pipeline should be verified.

## Implementation History

N/A

## Drawbacks

Please see the risks mentioned above.

This feature, when enabled, could result in potentially confusing outdated
alerts being presented to the user, especially during unusual events (e.g.,
cluster upgrades).

## Alternatives

Other notification approaches are currently being developed, but all of them
either focus on a different aspect or seem to be aimed at a more distant future,
whereas this feature could likely be implemented and available in production
relatively soon.

## Infrastructure Needed [optional]

N/A
