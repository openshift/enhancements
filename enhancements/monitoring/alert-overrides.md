---
title: alert-overrides
authors:
  - "@bison"
reviewers:
  - "@JoelSpeed"
  - "@openshift/openshift-team-monitoring"
approvers:
  - "@JoelSpeed"
  - "@simonpasquier"
api-approvers:
  - "@deads2k"
  - "@JoelSpeed"
creation-date: 2022-02-08
last-updated: 2022-05-11
tracking-link:
  - "https://issues.redhat.com/browse/MON-1985"
status: implementable
---

# User-Defined Alerting and Relabeling Rules

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The core OpenShift components ship a large number of Prometheus alerting rules.
A 4.10-nightly cluster on AWS currently has 175 alerts defined by default, and
enabling additional add-ons increases that number.  Still, users have repeatedly
asked for a supported method of adding additional custom rules, adjusting
thresholds, or adding and modifying labels on existing platform alerting rules
-- for example, changing severity labels.

These modifications are currently not possible, because the alerts are managed
by operators and cannot be modified, and the addition of custom rules is
explicitly unsupported at the moment.  This proposal outlines a solution for
allowing additional user-defined rules and relabeling operations on existing
platform alerting rules.

## Motivation

OpenShift is an opinionated platform, and the default alerting rules have of
course been carefully crafted to match what we think the majority of users need.
Nevertheless, users have repeatedly asked for the ability to modify alerting
rules to adjust thresholds, or to simply add and modify labels to aide in
routing alert notifications, as well as a supported method of adding additional
alerting rules.

Users currently have the ability to add additional alerting rules from a technical
standpoint -- by simply creating new `PrometheusRule` objects -- but this is
[explicitly unsupported][1] at the moment.  The reason being that these resources
are handled directly by the prometheus-operator, making it difficult to perform
OpenShift specific defaulting and validation.  A concrete example being that there
is no way to prevent users from creating new recording rules that may overwhelm
the platform Prometheus instance.

The goal of this proposal is to enable the addition of custom alerting rules,
and limited ability to override labeling of existing platform alerting rules via
resources under the direct control of the cluster-monitoring-operator.

### User Stories

- As an OpenShift user, I want to change the severity of an existing alert,
  choosing between, e.g. from `Warning` to `Critical`.
- As an OpenShift user, I want to add labels to alerts to aide in routing and
  triage.
- As an OpenShift user, I want a supported way to add custom alerts using
  metrics from components in the `openshift-*` namespaces.
- As an OpenShift developer, I want the alerting rules shipped by my component
  to be evaluated and forwarded to telemetry even when overridden by a user.

### Goals

- Give users a supported and documented method to add additional alerting rules,
  in a way that allows the cluster-monitoring-operator to provide its own
  defaulting, validation, telemetry, etc.
- Allow users to apply relabeling operations to alerting rules shipped by the
  platform.
- Resources shipped by operators remain unmodified.  No action needed from
  OpenShift developers.

### Non-Goals

- Only alerting rules are available for modification.  Not recording rules or
  other monitoring configuration.
- This phase of the enhancement does not address modifying alerting rule
  expressions or thresholds.  This may be addressed by a later enhancement,
  possibly with something similar to the [time series as alert thresholds][2]
  pattern.
  - Users still have the option of creating a new `AlertingRule` resource based
    on an existing platform rule, making their modifications there, and
    silencing the original alert.

## Proposal

### Workflow Description

The cluster-monitoring-operator will be extended with two new resources types:
`AlertingRule` and `AlertRelabelConfig`, both in the `monitoring.openshift.io`
group.

The new `AlertingRule` type will allow additional user-defined alerting rules,
but without the ability to create recording rules provided by the existing
`PrometheusRule` resource type. Example:

```yaml
---
apiVersion: monitoring.openshift.io/v1alpha1
kind: AlertingRule
metadata:
  name: custom-alerts
  namespace: openshift-monitoring
spec:
  groups:
  - name: CustomGroup
    rules:
    - alert: CustomAlert
      expr: "vector(1)"
      for: 15m
      labels:
        custom: "true"
```

A new controller in the cluster-monitoring-operator will generate
`PrometheusRule` resources based on the `AlertingRule` instances in the
`openshift-monitoring` namespace.  These will be consumed by the platform
prometheus-operator instance like any other `PrometheusRule`.  All user-defined
alerts will be labeled identifying them as such -- i.e. with a static
`openshift_io_user_alert=true` label.

The new `AlertRelabelConfig` type will allow relabeling operations on alerts.
All `AlertRelabelConfig` instances will be collected in order to build a single
secret with additional alert relabel configs used by the prometheus-operator.
Example of changing an alert severity from `none` to `critical`:

```yaml
---
apiVersion: monitoring.openshift.io/v1alpha1
kind: AlertRelabelConfig
metadata:
  labels:
    app.kubernetes.io/instance: k8s
  name: watchdog
  namespace: openshift-monitoring
spec:
  configs:
  - sourceLabels: [alertname, severity]
    regex: "Watchdog;none"
    targetLabel: severity
    replacement: critical
    action: replace
```

A new controller in the cluster-monitoring-operator will combine all
`AlertRelabelConfig` instances in the `openshift-monitoring` namespace into a
single secret, concatenating each resource in lexigraphical order by name.  This
secret will be referenced in the `additionalAlertRelabelConfigs` parameter for
the platform Prometheus instance.

The `AlertRelabelConfig` controller may optionally disable certain actions like
`drop` to prevent users disabling critical platform alerts.  Silences already
exist as a method of ignoring unwanted alerts.

Note that alert relabel operations only affect the alerts just before they are
sent to Alertmanager -- the changes are not reflected in the Prometheus alerts
API.  That means that currently dropped alerts will still be shown in the
OpenShift console, though they won't be forwarded to Alertmanager.  It also
means modifications to alerts, for example changed severities, are currently not
reflected in the console.  This may necessitate some UI/UX work in the console.

### API Extensions

We will be adding two new CRDs for alerting rules and relabeling:

- `alertingrule.monitoring.openshift.io/v1alpha1`
- `alertrelabelconfig.monitoring.openshift.io/v1alpha1`

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// NOTE: The AlertingRule type is a direct copy of the upstream PrometheusRule
// type from prometheus-operator.  The only difference at the moment is that we
// don't allow recording rules in OpenShift.  All rules must be alerting rules,
// but outside of that restriction, each AlertingRule will result in a 1:1 alike
// PrometheusRule object being created.
//
// See the upstream docs here:
// - https://github.com/prometheus-operator/prometheus-operator/blob/main/Documentation/api.md

// AlertingRule represents a set of user-defined Prometheus rule groups containing
// alerting rules -- recording rules are not allowed.
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
// +genclient
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status
type AlertingRule struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec describes the desired state of this AlertingRule object.
	Spec AlertingRuleSpec `json:"spec"`

	// status describes the current state of this AlertOverrides object.
	//
	// +optional
	Status AlertingRuleStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// AlertingRuleList is a list of AlertingRule objects.
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
// +k8s:openapi-gen=true
type AlertingRuleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// items is a list of AlertingRule objects.
	Items []AlertingRule `json:"items"`
}

// AlertingRuleSpec is the desired state of an AlertingRule resource.
//
// +k8s:openapi-gen=true
type AlertingRuleSpec struct {
	// groups is a list of grouped alerting rules.
	//
	// +listType=map
	// +listMapKey=name
	// +kubebuilder:validation:MinItems:=1
	Groups []RuleGroup `json:"groups"`
}

// RuleGroup is a list of sequentially evaluated alerting rules.
//
// +k8s:openapi-gen=true
type RuleGroup struct {
	// name is the name of the group.
	//
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// interval is how often rules in the group are evaluated.  If not specified,
	// it defaults to the global.evaluation_interval configured in Prometheus,
	// which itself defaults to 1 minute.  This is represented as a Prometheus
	// duration, for details on the format see:
	// - https://prometheus.io/docs/prometheus/latest/configuration/configuration/#duration
	//
	// +kubebuilder:validation:Pattern:="((([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?|0)"
	// +optional
	Interval string `json:"interval,omitempty"`

	// rules is a list of sequentially evaluated alerting rules.
	//
	// +kubebuilder:validation:MinItems:=1
	Rules []Rule `json:"rules"`
}

// Rule describes an alerting rule.
// See Prometheus documentation:
// - https://www.prometheus.io/docs/prometheus/latest/configuration/alerting_rules
//
// +k8s:openapi-gen=true
type Rule struct {
        // alert is the name of the alert. Must be a valid label value, i.e. 
        only
	// contain ASCII letters, numbers, and underscores.
	//
	// +kubebuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
	// +required
	Alert string `json:"alert"`

	// expr is the PromQL expression to evaluate. Every evaluation cycle this is
	// evaluated at the current time, and all resultant time series become
	// pending/firing alerts.
	//
	// +required
	Expr intstr.IntOrString `json:"expr"`

	// for is the time period after which alerts are considered firing after first
	// returning results.  Alerts which have not yet fired for long enough are
	// considered pending. This is represented as a Prometheus duration, for
	// details on the format see:
	// - https://prometheus.io/docs/prometheus/latest/configuration/configuration/#duration
	//
	// +kubebuilder:validation:Pattern:="((([0-9]+)y)?(([0-9]+)w)?(([0-9]+)d)?(([0-9]+)h)?(([0-9]+)m)?(([0-9]+)s)?(([0-9]+)ms)?|0)"
	// +optional
	For string `json:"for,omitempty"`

	// labels to add or overwrite for each alert.
	//
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// annotations to add to each alert.
	//
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`
}

// AlertingRuleStatus is the status of an AlertingRule resource.
type AlertingRuleStatus struct {
	// observedGeneration is the last generation change you've dealt with.
	//
	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`

	// prometheusRule is the generated PrometheusRule for this AlertingRule.
	//
	// +optional
	PrometheusRule PrometheusRuleRef `json:"prometheusRule,omitempty"`
}

// PrometheusRuleRef is a reference to an existing PrometheusRule object.
type PrometheusRuleRef struct {
	// name of the referent; More info: http://kubernetes.io/docs/user-guide/identifiers#names
	Name string `json:"name"`

	// kind of the referent; More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds"
	//
	// +optional
	Kind string `json:"kind,omitempty"`

	// API version of the referent.
	//
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
}

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:subresource:status

// AlertRelabelConfig defines a set of relabel configs for alerts.
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
// +k8s:openapi-gen=true
type AlertRelabelConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec describes the desired state of this AlertRelabelConfig object.
	Spec AlertRelabelConfigSpec `json:"spec"`

	// status describes the current state of this AlertRelabelConfig object.
	//
	// +optional
	Status AlertRelabelConfigStatus `json:"status,omitempty"`
}

// AlertRelabelConfigsSpec is the desired state of an AlertRelabelConfig resource.
//
// +k8s:openapi-gen=true
type AlertRelabelConfigSpec struct {
	// configs is a list of sequentially evaluated alert relabel configs.
	//
	// +kubebuilder:validation:MinItems:=1
	Configs []RelabelConfig `json:"configs"`
}

// AlertRelabelConfigStatus is the status of an AlertRelabelConfig resource.
type AlertRelabelConfigStatus struct {
	// conditions contains details on the state of the AlertRelabelConfig, may be
	// empty.
	//
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

const (
	// AlertRelabelConfigReady is the condition type indicating readiness.
	AlertRelabelConfigReady string = "Ready"
)

// AlertRelabelConfigList is a list of AlertRelabelConfigs.
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
// +k8s:openapi-gen=true
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type AlertRelabelConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	// items is a list of AlertRelabelConfigs.
	Items []*AlertRelabelConfig `json:"items"`
}

// LabelName is a valid Prometheus label name which may only contain ASCII
// letters, numbers, and underscores.
//
// +kubebuilder:validation:Pattern:="^[a-zA-Z_][a-zA-Z0-9_]*$"
type LabelName string

// RelabelConfig allows dynamic rewriting of label sets for alerts.
// See Prometheus documentation:
// - https://prometheus.io/docs/prometheus/latest/configuration/configuration/#alert_relabel_configs
// - https://prometheus.io/docs/prometheus/latest/configuration/configuration/#relabel_config
//
// +k8s:openapi-gen=true
type RelabelConfig struct {
	// sourceLabels select values from existing labels. Their content is
	// concatenated using the configured separator and matched against the
	// configured regular expression for the replace, keep, and drop actions.
	//
	// +optional
	SourceLabels []LabelName `json:"sourceLabels,omitempty"`

	// separator placed between concatenated source label values. When omitted,
	// Prometheus will use its default value of ';'.
        //
	// +optional
	Separator string `json:"separator,omitempty"`

	// targetLabel to which the resulting value is written in a replace action.
	// It is mandatory for 'replace' and 'hashmod' actions. Regex capture groups
	// are available.
	//
	// +optional
	TargetLabel string `json:"targetLabel,omitempty"`

	// regex against which the extracted value is matched. Default is: '(.*)'
	//
	// +optional
	Regex string `json:"regex,omitempty"`

	// modulus to take of the hash of the source label values.  This can be
	// combined with the 'hashmod' action to set 'target_label' to the 'modulus'
	// of a hash of the concatenated 'source_labels'.
	//
	// +optional
	Modulus uint64 `json:"modulus,omitempty"`

	// replacement value against which a regex replace is performed if the regular
	// expression matches. This is required if the action is 'replace' or
	// 'labelmap'. Regex capture groups are available. Default is: '$1'
	//
	// +optional
	Replacement string `json:"replacement,omitempty"`

	// action to perform based on regex matching. Must be one of: replace, keep,
	// drop, hashmod, labelmap, labeldrop, or labelkeep.  Default is: 'replace'
	//
	// +kubebuilder:validation:Enum=Replace;Keep;Drop;HashMod;LabelMap;LabelDrop;LabelKeep
	// +kubebuilder:default=Replace
	// +optional
	Action string `json:"action,omitempty"`
}
```

### Implementation Details/Notes/Constraints

TBD

### Risks and Mitigations

The primary risk is that relabeling is a powerful tool.  It's possible for users
to make a mistake and apply relabeling rules to unwanted alerts, including to
all alerts.  This should be warned against in documentation.  In addition, it
may be possible to provide a guided UI for common operations like simply
changing alert severity.

### Drawbacks

The main drawback of this proposal is the requirement for an API extension as 
outlined above. This is necessary since the exisiting API does not distinguish 
between recording rules and alerting rules.
We considered using the existing (non-distiguishing) API in concert with an 
admission webhook. This however was deemed to lead to a poor user experience and 
complicated documentation.

## Design Details

### Open Questions

- Should users be able to drop alerts, or are existing silences enough?

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
new resource types are mostly layers over existing stable APIs.

### Version Skew Strategy

TBD

### Operational Aspects of API Extensions

TBD, but The new CRDs are not expected to have significant operational impact.

#### Failure Modes

TBD

#### Support Procedures

TBD

## Implementation History

Initial proofs-of-concept:

- [cluster-monitoring-operator #1590](https://openshift/cluster-monitoring-operator/pull/1590)

  This is the initial PR adding the `AlertingRule` controller to c-m-o.

- [cluster-monitoring-operator #1596](https://github.com/openshift/cluster-monitoring-operator/pull/1596)

  This is the initial PR adding the `AlertRelabelConfig` controller to c-m-o.

- [prometheus-operator #4609](https://github.com/prometheus-operator/prometheus-operator/pull/4609)

  This is an attempt to introduce the `AlertRelabelConfig` type in upstream
  prometheus-operator.  If this isn't accepted upstream, it should be
  straightforward to bring downstream into c-m-o.

  *Update*: Looks like this isn't a fit for upstream.  It has been moved to
  cluster-monitoring-operator.

## Alternatives

- Primary alternative: https://github.com/openshift/enhancements/pull/958

  This is an alternative to the original alert overrides proposal, which
  involves a new CRD which matches existing alerting rules by name and labels,
  and then allows overriding any field.  The original is more flexible, but
  significantly more complex, and presents UX/UI challenges for console.

  The original proposal aims to allow overriding arbitrary fields in existing
  alerting rules. To do that, a new resource type takes a list of alert
  definitions that each have a selector to target an existing alert. That turns
  out to be complicated because the alerts are spread across a bunch of
  PrometheusRule resources, the names don't have to be unique, and outside of
  name we can only match on static labels, which aren't used very heavily except
  for setting a severity.

  This introduces a bunch of edge cases: What happens when multiple alerts
  match? Currently an error condition. What happens when the match used to be
  exact, but a new alert is introduced that also matches? Also an error
  condition, but one the user isn't likely to notice. What happens if the
  targeted alert is removed entirely? The patched alert also disappears. This
  all works technically, but the UX feels a bit odd in my opinion.

  There are also issues with integrating with the console. The overridden
  alerting rules and alerts still show up in the Prometheus API. The console
  should filter these somehow, but the original alerts must remain unchanged, so
  it now has to filter itself based on the original alerts and information from
  the overrides.

- Another alternative is to have a namespace, e.g. `openshift-user-alerts`,
  dedicated for user configuration objects.  Users can simply create any custom
  `PrometheusRule` objects in this namespace, and the operator can ensure they
  are then picked up by platform monitoring and labeled as user-defined.

  The primary disadvantage to this is that we have less control.  We can't for
  example disallow creation of recording rules with this method.

- Yet another alternative is that users create normal upstream `PrometheusRule`
  objects in any platform namespace, but must label them manually as
  user-defined.  This shares the same disadvantages as a dedicated namepsace,
  with the additional problem that users must remember to label the resources.

## Infrastructure Needed

None.

[1]: https://docs.openshift.com/container-platform/4.9/monitoring/configuring-the-monitoring-stack.html#support-considerations_configuring-the-monitoring-stack
[2]: https://www.robustperception.io/using-time-series-as-alert-thresholds
