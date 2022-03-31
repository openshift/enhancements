---
title: alerting-consistency
authors:
  - "@michaelgugino"
  - "@bison"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2021-02-03
last-updated: 2021-09-09
status: implementable
---

# Alerting Consistency

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Clear and actionable alerts are a key component of a smooth operational
experience.  Ensuring we have clear and concise guidelines for developers and
administrators creating new alerts for OpenShift will result in a better
experience for end users.  This proposal aims to outline the collective wisdom
of the OpenShift developer and wider monitoring communities in a way which can
then be translated into official documentation.

## Motivation

There seems to be consensus that the signal-to-noise ratio of alerts in
OpenShift could be improved, and alert fatigue for administrators thereby
reduced.  There should be clear guidance aligning critical, warning, and info
alert severities with expected outcomes across components.  There must be agreed
upon acceptance criteria for new critical alerts introduced in OpenShift.

### Goals

* Define acceptance criteria for new critical alerts.
* Define practical guidelines for writing alerts of each severity.
* Translate this into clear developer documentation with examples.
* Enforce acceptance criteria for critical alerts in the OpenShift test suite.

### Non-Goals

* Define needs for specific alerts.
* Implement user-defined alerts.

## Proposal

### User Stories

#### Story 1

As an OpenShift developer, I need to ensure that my alerts are appropriately
tuned to allow end-user operational efficiency.

#### Story 2

As an SRE, I need alerts to be informative, actionable and thoroughly
documented.

### Risks and Mitigations

As the primary goal of this proposal is to collect feedback to be translated
into developer documentation, there isn't much risk.  The primary source of risk
is friction introduced by enforcing acceptance criteria for critical alerts in
shared OpenShift test suites.  This could be an issue for teams already shipping
critical alerts, but can be mitigated by providing case by case exceptions until
the monitoring team can reach out and provide guidance on correcting any issues.
This also provides a concrete process by which we can audit existing critical
alerts across components, and bring them into compliance one by one.

## Design Details

### Recommended Reading

A list of references on good alerting practices:

* [Google SRE Book - Monitoring Distributed Systems][7]
* [Prometheus Alerting Documentation][8]
* [Alerting for Distributed Systems][9]

### Alert Ownership

Individual teams are responsible for writing and maintaining alerting rules for
their components, i.e. their operators and operands.  The monitoring team is
available for consulting.  Frequently asked questions and broadly applicable
patterns should be added to the developer documentation this proposal aims to
result in.

Teams should also take into consideration how their components interact with
existing monitoring and alerting.  As an example, if your operator deploys a
service which creates one or more `PersistentVolume` resources, and these
volumes are expected to be mostly full as part of normal operation, it's likely
that this will cause unnecessary `KubePersistentVolumeFillingUp` alerts to fire.
You should work with the monitoring team to find a solution to avoid triggering
these alerts if they are not actionable.

### Style Guide

* Alert names MUST be CamelCase, e.g.: `PrometheusRuleFailures`
* Alert names SHOULD be prefixed with a component, e.g.: `AlertmanagerFailedReload`
  * There may be exceptions for some broadly scoped alerts, e.g.: `TargetDown`
* Alerts MUST include a `severity` label indicating the alert's urgency.
  * Valid severities are: `critical`, `warning`, or `info` — see below for
    guidelines on writing alerts of each severity.
* Alerts MUST include `summary` and `description` annotations.
  * Think of `summary` as the first line of a commit message, or an email
    subject line.  It should be brief but informative.  The `description` is the
    longer, more detailed explanation of the alert.
* Alerts SHOULD include a `namespace` label indicating the source of the alert.
  * Many alerts will include this by virtue of the fact that their PromQL
    expressions result in a namespace label.  Others may require a static
    namespace label — see for example, the [KubeCPUOvercommit][1] alert.
* All critical alerts MUST include a `runbook_url` annotation.
  * Runbook style documentation for resolving critical alerts is required.
    These runbooks are reviewed by OpenShift SREs and currently live in the
    [openshift/runbooks][2] repository.
* Operator Alerts are RECOMMENDED to include `kubernetes_operator_part_of` label
  indicating the operator name the alert is related to.
* Operator Alerts are RECOMMENDED to include `kubernetes_operator_component` label
  indicating the operator component name that the alert is related to.

### Critical Alerts

TL/DR: For alerting current and impending disaster situations.  These alerts
page an SRE.  The situation should warrant waking someone in the middle of the
night.

Timeline:  ~5 minutes.

Reserve critical level alerts only for reporting conditions that may lead to
loss of data or inability to deliver service for the cluster as a whole.
Failures of most individual components should not trigger critical level alerts,
unless they would result in either of those conditions. Configure critical level
alerts so they fire before the situation becomes irrecoverable. Expect users to
be notified of a critical alert within a short period of time after it fires so
they can respond with corrective action quickly.

Example critical alert: [KubeAPIDown][3]

```yaml
- alert: KubeAPIDown
  annotations:
    summary: Target disappeared from Prometheus target discovery.
    description: KubeAPI has disappeared from Prometheus target discovery.
    runbook_url: https://github.com/openshift/runbooks/blob/master/alerts/cluster-monitoring-operator/KubeAPIDown.md
  expr: |
    absent(up{job="apiserver"} == 1)
  for: 15m
  labels:
    severity: critical
```

This alert fires if no Kubernetes API server instance has reported metrics
successfully in the last 15 minutes.  This is a clear example of a critical
control-plane issue that represents a threat to the operability of the cluster
as a whole, and likely warrants paging someone.  The alert has clear summary and
description annotations, and it links to a runbook with information on
investigating and resolving the issue.

The group of critical alerts should be small, very well defined, highly
documented, polished and with a high bar set for entry.  This includes a
mandatory review of a proposed critical alert by the Red Hat SRE team.

### Warning Alerts

TL/DR: The vast majority of alerts should use the severity.  Issues at the
warning level should be addressed in a timely manner, but don't pose an
immediate threat to the operation of the cluster as a whole.

Timeline:  ~60 minutes

If your alert does not meet the criteria in "Critical Alerts" above, it belongs
to the warning level or lower.

Use warning level alerts for reporting conditions that may lead to inability to
deliver individual features of the cluster, but not service for the cluster as a
whole. Most alerts are likely to be warnings. Configure warning level alerts so
that they do not fire until components have sufficient time to try to recover
from the interruption automatically. Expect users to be notified of a warning,
but for them not to respond with corrective action immediately.

Example warning alert: [ClusterNotUpgradeable][4]

```yaml
- alert: ClusterNotUpgradeable
  annotations:
    summary: One or more cluster operators have been blocking minor version cluster upgrades for at least an hour.
    description: In most cases, you will still be able to apply patch releases.
      Reason {{ "{{ with $cluster_operator_conditions := \"cluster_operator_conditions\" | query}}{{range $value := .}}{{if and (eq (label \"name\" $value) \"version\") (eq (label \"condition\" $value) \"Upgradeable\") (eq (label \"endpoint\" $value) \"metrics\") (eq (value $value) 0.0) (ne (len (label \"reason\" $value)) 0) }}{{label \"reason\" $value}}.{{end}}{{end}}{{end}}"}}
      For more information refer to 'oc adm upgrade'{{ "{{ with $console_url := \"console_url\" | query }}{{ if ne (len (label \"url\" (first $console_url ) ) ) 0}} or {{ label \"url\" (first $console_url ) }}/settings/cluster/{{ end }}{{ end }}" }}.
    expr: |
      max by (name, condition, endpoint) (cluster_operator_conditions{name="version", condition="Upgradeable", endpoint="metrics"} == 0)
    for: 60m
    labels:
      severity: warning
```

This alert fires if one or more operators have not reported their `Upgradeable`
condition as true in more than an hour.  The alert has a clear name and
informative summary and description annotations.  The timeline is appropriate
for allowing the operator a chance to resolve the issue automatically, avoiding
the need to alert an administrator.

### Info Alerts

TL/DR: Info level alerts represent situations an administrator should be aware
of, but that don't necessarily require any action.  Use these sparingly, and
consider instead reporting this information via Kubernetes events.

Example info alert: [MultipleContainersOOMKilled][5]

```yaml
- alert: MultipleContainersOOMKilled
  annotations:
    description: Multiple containers were out of memory killed within the past
      15 minutes. There are many potential causes of OOM errors, however issues
      on a specific node or containers breaching their limits is common.
      summary: Containers are being killed due to OOM
  expr: sum(max by(namespace, container, pod) (increase(kube_pod_container_status_restarts_total[12m]))
    and max by(namespace, container, pod) (kube_pod_container_status_last_terminated_reason{reason="OOMKilled"}) == 1) > 5
  for: 15m
  labels:
    namespace: kube-system
    severity: info
```

This alert fires if multiple containers have been terminated due to out of
memory conditions in the last 15 minutes.  This is something the administrator
should be aware of, but may not require immediate action.

### Test Plan

Automated tests enforcing acceptance criteria for critical alerts, and basic
style linting will be added to the [openshift/origin][6] end-to-end test suite.
The monitoring team will work with anyone shipping existing critical alerts that
don't meet these criteria in order to resolve the issue before enabling the
tests.

## Implementation History
None

## Drawbacks

People might have rules around the existing broken alerts.  They will have to
change some of these rules.

## Alternatives

Document policies, but not make any backwards-incompatible changes to the
existing alerts and only apply the policies to new alerts.

### Graduation Criteria
None

#### Dev Preview -> Tech Preview"
None

#### Tech Preview -> GA"
None

#### Removing a deprecated feature"
None

### Upgrade / Downgrade Strategy"
None

### Version Skew Strategy"
None


[1]: https://github.com/openshift/cluster-monitoring-operator/blob/79cdf68/assets/control-plane/prometheus-rule.yaml#L235-L247
[2]: https://github.com/openshift/runbooks
[3]: https://github.com/openshift/cluster-monitoring-operator/blob/79cdf68/assets/control-plane/prometheus-rule.yaml#L412-L421
[4]: https://github.com/openshift/cluster-version-operator/blob/513a2fc/install/0000_90_cluster-version-operator_02_servicemonitor.yaml#L68-L76
[5]: https://github.com/openshift/cluster-monitoring-operator/blob/79cdf68/assets/cluster-monitoring-operator/prometheus-rule.yaml#L326-L338
[6]: https://github.com/openshift/origin
[7]: https://sre.google/sre-book/monitoring-distributed-systems/
[8]: https://prometheus.io/docs/practices/alerting/
[9]: https://www.usenix.org/sites/default/files/conference/protected-files/srecon16europe_slides_rabenstein.pdf
