---
title: alert-overrides
authors:
  - "@bison"
reviewers:
  - TBD
  - "@openshift/openshift-team-monitoring"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2021-11-02
last-updated: 2021-11-02
tracking-link: https://issues.redhat.com/browse/MON-1985
status: implementable
---

# User-Defined Overrides for Platform Alerting Rules

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The core OpenShift components ship a large number of Prometheus alerting rules.
A 4.10-nightly cluster on AWS currently has 175 alerts defined by default, and
enabling additional add-ons increases that number.  Still, users have repeatedly
asked for a supported method of adding additional custom rules, and adjusting
thresholds, or adding labels and annotations to existing platform alerting
rules.

These modifications are currently not possible, because the alerts are managed
by operators and cannot be modified, and the addition of custom rules is
explicitly unsupported at the moment.  This proposal outlines a solution for
allowing additional user-defined rules and limited overrides of existing
platform alerting rules.

## Motivation

OpenShift is an opinionated platform, and the default alerting rules have of
course been carefully crafted to match what we think the majority of users need.
Nevertheless, users have repeatedly asked for the ability to modify alerting
rules to adjust thresholds, or to simply add labels and annotations to aide in
routing alert notifications, as well as a supported method of adding additional
alerting rules.

Users currently have the ability to add additional rules from a technical
standpoint -- by simply creating new `PrometheusRule` objects -- but this is
[explicitly unsupported][1] at the moment.  The reason being that these
resources are handled directly by the prometheus-operator, making it difficult
to perform OpenShift specific defaulting and validation.  A concrete example
being that there is no way to prevent users from creating new recording rules
that may overwhelm the platform Prometheus instance.

We've also encountered bugs and unexpected interactions that cause alerts to
fire erroneously.  A supported method of patching platform alerting rules
provides a more flexible method of applying temporary fixes in such cases.

The goal of this proposal is to enable the addition of custom alerting rules,
and limited ability to override existing platform alerting rules via a resource
under the direct control of the cluster-monitoring-operator.

### Goals

- Give users a single, supported, and documented place to add additional rules,
  in a way that allows the cluster-monitoring-operator to provide its own
  defaulting, validation, etc.
- Allow users to apply overrides to alerting rules shipped by the platform.
- Allow users to drop unwanted platform alerts.
- User-supplied overrides are merged with platform rules -- updates to fields
  not modified by users continue to get updates.
- Resources shipped by operators remain unmodified.  No action needed from
  OpenShift developers.

### Non-Goals

- Only alerting rules are available for modification.  Not recording rules or
  other monitoring configuration.

## Proposal

The cluster-monitoring-operator will be extended with a new controller that
watches an instance of a new resource type with the following structure:

```yaml
apiVersion: v1alpha1
kind: alertoverrides.monitoring.openshift.io
metadata:
  name: alert-overrides
  namespace: openshift-monitoring
spec:
  # List of overrides for alerts in platform namespaces.
  overrides:
  - selector: # required
      alert: "KubeAPIErrorBudgetBurn" # required
      matchLabels: # optional, but multiple matches is an error reported in status.
        severity: "critical"
        long: "1h"
    action: "patch"
    labels:
      patched: "true"

  - selector:
      alert: "KubeAPIErrorBudgetBurn"
      matchLabels: # not a unique match, this is an error.
        severity: "critical"
    action: "patch"
    labels:
      patched: "true"

  - selector:
      alert: "Watchdog"
    action: "drop"

  # List of new Prometheus alerting rules.
  rules:
  - alert: "CustomAlert"
    expr: "vector(1)"
    for: "15m"
    labels:
      custom: "true"

status:
  conditions:
  - type: "OverrideError"
    status: "True"
    reason: "MultipleMatches"
    message: "Multiple alerts found with name KubeAPIErrorBudgetBurn and labels: [severity: critical]"
    lastTransitionTime: "2021-11-26T15:42:19Z"
```

The controller maintains an index of alerting rule names to their corresponding
`PrometheusRule` objects, ignoring any in non-platform namespaces, i.e. any
namespace without the `openshift.io/cluster-monitoring=true` label.  When the
configuration changes, all `PrometheusRule` resources containing an alerting
rule with the given name are found, the rules extracted, and the static labels
are matched against the `matchLabels` map supplied by the user.  If an exact
match is found, this rule is targeted for the override.  Multiple matches is an
error condition, this override will be skipped, and an error will be reported in
the status conditions.

For each override a new alerting rule is generated with the overrides applied on
top of the existing rule, and this new rule is added to the "alert-overrides"
`PrometheusRule` object.  Additional user-supplied rules are also added here.
All generated alerts will be given an identifying label -- currently:
`monitoring_openshift_io__alert_override=true`.  This happens after applying
user-supplied overrides, meaning the user cannot modify this label.  All rules
can be validated individually using [the same code][2] the prometheus-operator
and Prometheus itself use.

This leaves the original alerting rules present and untouched.  These, as well
as any rules listed with the `drop` action, must not be forwarded to the
platform Alertmanager.  A `Secret` containing [alert_relabel_configs][3] data
for Prometheus is generated to accomplish this:

```yaml
# Alerts to be dropped are matched against name, the set of labels the user
# supplied to match on, and an empty override label.  The empty override label
# differentiates the original and generated alerting rules.

- source_labels: [alertname, severity, long, monitoring_openshift_io__alert_override]
  regex: "KubeAPIErrorBudgetBurn;critical;1h;"
  action: drop
- source_labels: [alertname, severity, monitoring_openshift_io__alert_override]
  regex: “Watchdog;none;”
  action: drop
```

Note that this prevents these alerts, whether overridden or dropped, from being
sent to Alertmanager, but they will still show up as active alerts in the
Prometheus API.  The OpenShift web console will have to be made aware of the
labeling in order to hide these by default.

The generation of a new `PrometheusRule` object for overrides, and dropping the
original alerts via relabeling is a consequence of the fact that the existing
rules are managed by various operators.  They cannot be modified directly, as
the operators would revert the changes.  Keeping the existing alerts in place,
and only preventing them from being sent to Alertmanager via relabeling also
allows us to continue to collect telemetry on whether these platform alerts are
firing -- even if a user has chose to completely "drop" them.

Summary:

- Watch for a user-defined list of overrides and new rules.
  - Processing of overrides is also triggered by `PrometheusRule` object
    changes, and changes to the set of platform namespaces.
- For each override, find the existing rule matching the name and labels, and
  apply the action specified by the user, i.e. `patch` or `drop`.
  - If the action is `patch`, generate a new rule with supplied overrides.
  - If the action is `drop`, simply add the alert to the alert relabel config.
  - If multiple alerts match the name and labels, notify the user and skip
    this override.
- Generate new `PrometheusRule` objects for any new user-defined rules.
- Generate the [alert_relabel_configs][1] to drop original alerts that have been
  replaced with a patched version, and any that were explicitly dropped.

### User Stories

- As an OpenShift user, I want to change the severity of an existing alert,
  choosing between, e.g. from `Warning` to `Critical`.
- As an OpenShift user, I want to permanently disable an alert that I know does
  not pertain to my deployment.
- As an OpenShift user, I want to add labels or annotations to alerts to aide in
  routing and triage.
- As an OpenShift user, I want to rewrite the PromQL expression or duration for
  an alert in order to adjust thresholds to suit my environment.
- As an OpenShift user, I want a supported way to add custom alerts using
  metrics from components in the `openshift-*` namespaces.

### API Extensions

We will be adding a new CRD for alert overrides. Initially:
`alertoverrides.monitoring.coreos.com/v1alpha1`

### Implementation Details/Notes/Constraints

TBD

### Risks and Mitigations

The primary risk is that this provides users with a method to permanently
modify, including to completely drop, alerting rules, which could lead to
unintentionally missed alerts in the case of user error.  Careful consideration
should be given to UX.  Information on which overridden or dropped alerts are
firing is still available in the Prometheus API.  Perhaps this can be leveraged
and surfaced in an unobtrusive way.

## Design Details

### Open Questions

TBD

### Test Plan

This should be fairly straight forward to both unit and e2e test.

### Graduation Criteria

Plan to release as Tech Preview initially.

#### Dev Preview -> Tech Preview

N/A

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

Upgrade and downgrade are not expected to present a significant challenge.  The
overridden rules are reconciled against those shipped by OpenShift in real time,
so any updates are applied as needed.  On downgrade to a version not supporting
the feature, the newly generated `PrometheusRule` resource and the `Secret`
containing the relabeling configs can simply be deleted.

The biggest concern is removal, renaming, or change in severity of OpenShift
shipped rules as this may prevent a previously overridden rule from being found.
This will be surfaced to the user in the form of status conditions on the CRD.

### Version Skew Strategy

This shouldn't be an issue as rules are reconciled against OpenShift rules and
regenerated as necessary as changes occur.

### Operational Aspects of API Extensions

The new CRD is not expected to have significant operational impact.  It is in
fact a "singleton" CRD, in that there is only expected to be a single instance
of it in the `openshift-monitoring` namespace.

#### Failure Modes

TBD

#### Support Procedures

TBD

## Implementation History

- Initial Proof-of-Concept:
  https://github.com/openshift/cluster-monitoring-operator/pull/1473

## Drawbacks

TBD

## Alternatives

- Instead of dropping alerts via relabel config, it may be possible to have the
  operator generate silences for them.
  - Pros:
    - No / fewer changes to console.  Overridden alerts show as silenced.
  - Cons:
    - Additional complexity in cluster-monitoring-operator.
    - Additional load on Alertmanager.
    - Some users, including OSD, alert when silences are active for some time.

- Users deploy overrides and/or additional alerts as standard `PrometheusRule`
  objects in `openshift-*` namespaces, and label them to indicate they are
  user-defined rules.  The operator instructs Prometheus to ignore the original
  alerts with a relabel config.
  - Pros:
    - No new `ConfigMap` or `CRD` -- uses existing types.
  - Cons:
    - A simple addition of a label requires copying the entire rule definition.
    - Increased configuration drift because of the need to copy entire rules.
    - Error prone if user doesn't correctly label rules as user-defined.

## Infrastructure Needed

None.

[1]: https://docs.openshift.com/container-platform/4.9/monitoring/configuring-the-monitoring-stack.html#support-considerations_configuring-the-monitoring-stack
[2]: https://pkg.go.dev/github.com/prometheus/prometheus/pkg/rulefmt
[3]: https://prometheus.io/docs/prometheus/latest/configuration/configuration/#alert_relabel_configs
