---
title: alert-routing
authors:
  - "@simonpasquier"
reviewers:
  - "@openshift/openshift-team-monitoring"
approvers:
  - TBD
  - "@openshift/openshift-team-monitoring"
creation-date: 2021-10-11
last-updated: 2021-10-11
status: provisional
see-also:
  - "/enhancements/monitoring/user-workload-monitoring.md"
---

# alert-routing-for-user-workload monitoring

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This document describes a solution that allows OpenShift users to route alert
notifications without cluster admin intervention. It complements the existing
[user-workload monitoring stack][uwm-docs], enabling a full self-service experience for
workload monitoring.

## Motivation

Since OpenShift 4.6, application owners can collect metrics from their
applications and configure alerting rules by themselves as described in the
[User Workload Monitoring][user-workload-monitoring-enhancement] (UWM)
enhancement. The rules are defined as `PrometheusRule` resources and can be
based on platform and/or application metrics. They are evaluated by the Thanos
ruler instances (by default) or the Prometheus instances running in the
`openshift-user-workload` namespace.

When a user alert fires, the Thanos Ruler (or UWM Prometheus) sends it to the
Plaform Alertmanager cluster (deployed in the `openshift-monitoring` namespace)
where it gets aggregated and dispatched to the correct destination (page, chat,
email, ticket, ...).

The configuration of Alertmanager is done via a single configuration file that
only cluster admins have permissions to modify. If the cluster is shared by
multiple tenants and each tenant has different requirements to receive their
notifications then each tenant needs to ask and wait for the cluster admins to
adjust the Alertmanager configuration.

To streamline the process and avoid cluster admins being the bottleneck,
application owners should be able to configure alert routing and notification
receivers in the Plaform Alertmanager without cluster admin intervention.

[AlertmanagerConfig][alertmanagerconfig-crd] CRD fullfills this requirement and
is supported by the [Prometheus operator][prometheus-operator] since v0.43.0
but it is explicitly called out in the OCP documentation as ["not
supported"][unsupported-resources].

### Goals

* Cluster users can configure alert notifications for applications being
monitored by the user-workload monitoring without requesting intervention from
cluster admins.
* Cluster admins can grant permissions to users and groups to manage alert
routing scoped to individual namespaces.
* Namespace owners should be able to opt-out from Alertmanager
configuration (similar to what exist for service/pod monitors and rules using the
`"openshisft.io/user-monitoring: false"` label on the namespace).
* Cluster admins should be able to opt-out from supporting `AlertmanagerConfig`
resources from user namespaces.

### Non-Goals

* Additional support for silencing user alerts (it is already supported by UWM in the OCP console).
* Specific integration in the OCP Console exposing the configuration of alert notifications.
* Support the configuration of alert notifications for platform alerts (e.g.
alerts originating from namespaces with the `openshift.io/cluster-monitoring: "true"`
label).

## Proposal

We plan to leverage the `AlertmanagerConfig` custom resource definition already
exposed by the Prometheus operator so that application owners can configure how
and where their alert notifications should be routed.

### User Stories

Personas:
* Application owners: manage a project with sufficient permissions to define monitoring resources.
* UWM admins: manage the configuration of the UWM components (edit permissions on the `openshift-user-workload-monitoring/user-workload-monitoring-config` configmap).
* Cluster admins: manage the configuration of the Platform monitoring components.

#### Story 1

As an application owner, I want to use AlertmanagerConfig custom resources so
that Alertmanager can push alert notifications for my applications to the
receiver of my choice.

#### Story 2

As an application owner, I want to use AlertmanagerConfig custom
resources so that Alertmanager can inhibit alerts based on other alerts firing
at the same time.

#### Story 3

As an application owner, I want to know if my AlertmanagerConfig custom
resource is taken into account so that I am confident that I will receive alert
notifications.

#### Story 4

As a OpenShift cluster admin, I want to allow some of my users to
create/update/delete AlertmanagerConfig custom resources and leverage the
platform Alertmanager cluster so that I don't have to configure alert routing
on their behalf.

#### Story 5

As an OpenShift cluster admin, I don't want AlertmanagerConfig resources
defined by application owners to interfere with the routing of platform alerts.

#### Story 6

As an OpenShift cluster admin, I want to exclude certain user namespaces from
modifying the Plaform Alertmanager configuration so that I can recover in case
of breakage or bad behavior.

#### Story 7

As an OpenShift cluster admin, I don't want to support AlertmanagerConfig
resources for application owners so that the configuration of the Platform
Alertmanager cluster is completely under my control.

### Story 8

As a UWM admin, I don't want to send user alerts to the Platform Alertmanager
cluster because these alerts are managed by an external system (off-cluster Alertmanager for
instance).

### Implementation Details/Notes/Constraints

The `AlertmanagerConfig` CRD is exposed by the `coreos.monitoring.com/v1alpha1` API group.

The `Alertmanager` custom resource defines 2 LabelSelector fields
(`alertmanagerConfigSelector` and `alertmanagerConfigNamespaceSelector`) to
select which `AlertmanagerConfig` resources should be reconciled by the
Prometheus operator and from which namespace(s). Before this proposal, the
Plaform Alertmanager resource defines the 2 selectors as null, meaning that it
doesn't select any `AlertmanagerConfig` resources.

We propose to add a new field `enableUserAlertmanagerConfig` to the
`openshift-montoring/cluster-monitoring-config` configmap. If
`enableUserAlertmanagerConfig` is missing, the default value is false.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-monitoring-config
  namespace: openshift-monitoring
data:
  config.yaml: |-
    enableUserWorkload: true
    enableUserAlertmanagerConfig: true
```

When `enableUserAlertmanagerConfig` is true, the cluster monitoring operator
configures the Platform Alertmanager as follows.

```yaml
apiVersion: monitoring.coreos.com/v1
kind: Alertmanager
metadata:
  name: main
  namespace: openshift-monitoring
spec:
  alertmanagerConfigSelector: {}
  alertmanagerConfigNamespaceSelector:
    matchExpressions:
    - key: openshift.io/cluster-monitoring
      operator: NotIn
      values:
      - "true"
    - key: openshift.io/user-monitoring
      operator: NotIn
      values:
      - "false"
  ...
```

To be consistent with what exists already for service/pod monitors and rules,
the Prometheus operator doesn't reconcile `AlertmanagerConfig` resources from
namespaces with the `openshift.io/user-monitoring: "false"` label.  It allows
application owners to opt out completely from UWM in case they deploy and run
their own monitoring infrastructure (for instance with the [Monitoring Stack
operator][monitoring-stack-operator]).

In addition, the cluster admins can exclude specific user namespace(s) from UWM with the new `excludeUserNamespaces` field.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-monitoring-config
  namespace: openshift-monitoring
data:
  config.yaml: |-
    enableUserWorkload: true
    enableUserAlertmanagerConfig: true
    excludeUserNamespaces: [foo,bar]
```

The UWM admins can also define that UWM alerts shouldn't be forwarded to the
Platform Alertmanager. With this capability and the existing
`additionalAlertmanagerConfigs`, it is possible to externalize the alert
routing and notifications to an external Alertmanager instance when the cluster
admins don't want to share the Plaform Alertmanager for instance.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: user-workload-monitoring-config
  namespace: openshift-user-workload-monitoring
data:
  config.yaml: |-
    thanosRuler:
      usePlatformAlertmanager: false
    prometheus:
      usePlatformAlertmanager: false
      additionalAlertmanagerConfigs: [...]
```

When this option is chosen, the OCP console can't be used to manage silences for user alerts.

### Tenancy

By design, all alerts coming from UWM have a `namespace` label equal to the
`PrometheusRule` resource's namespace. The Prometheus operator relies on this
invariant to generate an Alertmanager configuration that ensures that a given
`AlertmanagerConfig` resource only matches alerts that have the same
`namespace` value. This means that an `AlertmanagerConfig` resource from
namespace `foo` only processes alerts with the `namespace="foo"` label (be it
for routing or inhibiting purposes).

### RBAC

The cluster monitoring operator ships a new cluster role
`alertmanager-config-edit` so that cluster admins can bind it with a
`RoleBinding` to grant permissions to users or groups on `AlertmanagerConfig`
custom resources within a given namespace.

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: alertmanager-config-edit
rules:
- apiGroups:
  - monitoring.coreos.com
  resources:
  - alertmanagerconfigs
  verbs:
  - '*'
```

This role complements the existing `monitoring-edit`, `monitoring-rules-edit` and `monitoring-rules-view` roles.

#### Resource impacts

The size of the Alertmanager routing tree can grow to thousands of entries
(possibly several `AlertmanagerConfig` resources per namespace) and this may
hinder the performances of the Plaform Alertmanager (increased latency of alert
notifications for instance).

We already know from upstream users that Alertmanager can deal with many
routes. We plan to simulate environments with thousands of `AlertmanagerConfig`
resources and measure the impact on notification delivery.

### Risks and Mitigations

#### Disruption of the platform Alertmanager

Even though the Prometheus operator prevents it as much as it can, it may be
possible for users to create an `AlertmanagerConfig` resource that triggers the
Prometheus operator to generate an invalid Alertmanager configuration, leading
to a potential outage of the Platform Alertmanager cluster.

Mitigations
* The `AlertmanagerBadConfig` alert fires when Alertmanager can't reload its configuration.
* Cluster admins can turn off the support for `AlertmanagerConfig` globally so that the Platform Alertmanager cluster can process platform alerts again and the cluster admins have time to identiy the "rogue" `AlertmanagerConfig` resource(s).
* Cluster admins can exclude specific user namespaces (once the "rogue" `AlertmanagerConfig` resource(s) have been identified) to restore UWM functionality for good citizens.

#### Misconfiguration of receivers

Users may provide bad credentials for the receivers, the system receiving the
notifications might be unreachable or the system might be unable to process the requests. These
situations would trigger the `AlertmanagerFailedToSendAlerts` and/or
`AlertmanagerClusterFailedToSendAlerts` alerts. The cluster admins have to act
on upon the alerts and understand where the problem comes from.

Mitigations
* Detailed runbook for the `AlertmanagerFailedToSendAlerts` and `AlertmanagerClusterFailedToSendAlerts` alerts.
* Ability to use a separate Alertmanager cluster to avoid messing up with the platform Alertmanager cluster.

#### Non-optimal Alertmanager settings

Users may use non-optimal settings for their alert notifications (such as
reevaluation of alert groups at high frequency). This may impede the
performances of Alertmanager globally because it would consume more CPU. It can
also trigger notification failures if an exteral integration limits the number
of requests a client IP address can do.

Mitigation
* Improve the `Alertmanager` CRD to expose new fields enforcing minimum interval values for all associated `AlertmanagerConfig` resources. This would be similar to what exists at the `Prometheus` CRD level for scrape targets with `enforcedSampleLimit` for instance.

#### AlertmanagerConfig resources not being reconciled

An `AlertmanagerConfig` resource might require credentials (such as API keys)
which are referenced by secrets. If the Platform Prometheus operator doesn't
have permissions to read the secret or if the reference is incorrect (wrong
name or key), the operator doesn't reconcile the resource in the final
Alertmanager configuration.

Mitigation
* The Prometheus operator should expose a validating admission webhook that should prevent invalid configurations.
* We can implement the `Status` subresource of the `AlertmanagerConfig` CRD to report whether or not the resource is reconciled or not (with a message).
* Users can validate that alerting routing works as expected by generating "fake" alerts triggering the notification system. _Users don't have permissions on the Alertmanager API endpoint so they would have to generate fake alerts from alerting rules themselves. We could also support the ability to craft an alert from the OCP console_.

## Design Details

### Open Questions

* Should CMO allow UWM admins to deploy a separate Alertmanager cluster in the `openshift-user-workload-monitoring` namespace if the cluster admins don't want to share the Platform Alertmanager?
  * Pros
    * More flexibility.
  * Cons
    * Increased complexity.
    * Redundancy with the upcoming Monitoring Stack operator.

### Test Plan

New tests are added to the cluster monitoring operator end-to-end test suites
to validate the different user stories.

### Graduation Criteria

We plan to have the feature released Tech Preview first. We assume that the
`AlertmanagerConfig` CRD graduates to `v1beta1` at least before we consider
exposing the feature.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

- The `AlertmanagerConfig` CRD is exposed as `v1` API.
- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback including signals from telemetry about the customer adoption (e.g. number of `AlertmanagerConfig` resources across the fleet).
- Counter-measures to avoid service degradation of the Platform Alertmanager.
- Conduct load testing
- Console integration?

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

CMO continues to orchestrate and automate the deployment of all monitoring
components with the help of the Prometheus operator in this case.

By default, CMO doesn't enable for user alert routing, hence upgrading to a
OpenShift release supporting `AlertmanagerConfig` doesn't change the behavior
of the monitoring components.

### Version Skew Strategy

N/A

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

N/A

## Alternatives

An alternative is to keep the current status-quo and rely on cluster admins to
configure alert routing for their users. This proposal doesn't forbid this
model since cluster admins can decide to not reconcile user-defined
`AlertmanagerConfig` resources within the Platform Alertmanager.

[user-workload-monitoring-enhancement]: https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/user-workload-monitoring.md
[uwm-docs]: https://docs.openshift.com/container-platform/4.8/monitoring/enabling-monitoring-for-user-defined-projects.html
[prometheus-operator]: https://prometheus-operator.dev/
[alertmanagerconfig-crd]: https://prometheus-operator.dev/docs/operator/api/#alertmanagerconfig
[unsupported-resources]: https://docs.openshift.com/container-platform/4.8/monitoring/configuring-the-monitoring-stack.html#support-considerations_configuring-the-monitoring-stack
[monitoring-stack-operator]: https://github.com/openshift/enhancements/pull/866
