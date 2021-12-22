---
title: multi-tenant-alerting
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

# multi-tenant-alerting

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
`"openshift.io/user-monitoring: false"` label on the namespace).
* Cluster admins should be able to opt-out from supporting `AlertmanagerConfig`
resources from user namespaces.

### Non-Goals

* Additional support for silencing user alerts (it is already supported by UWM in the OCP console).
* Specific integration in the OCP Console exposing the configuration of alert notifications.
* Support the configuration of alert notifications for platform alerts (e.g.
alerts originating from namespaces with the `openshift.io/cluster-monitoring: "true"`
label).
* Share alert receivers and routes across tenants.
* Deploy an additional UWM Alertmanager.

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
resource has been reconciled on the target Alertmanager so that I am confident
that I receive alert notifications.

#### Story 4

As a OpenShift cluster admin, I want to allow some of my users to
create/update/delete AlertmanagerConfig custom resources and leverage the
platform Alertmanager cluster so that I don't have to configure alert routing
on their behalf.

#### Story 5

As an OpenShift cluster admin, I want to distinguish between platform and user
alerts so that my Alertmanager configuration can reliably handle all platform alerts.

#### Story 6

As an OpenShift cluster admin, I want to exclude certain user namespaces from
modifying the Plaform Alertmanager configuration so that I can recover in case
of breakage or bad behavior.

#### Story 7

As an OpenShift cluster admin, I don't want to support AlertmanagerConfig
resources for application owners so that the configuration of the Platform
Alertmanager cluster is completely under my control.

#### Story 8

As a UWM admin, I don't want to send user alerts to the Platform Alertmanager
cluster because these alerts are managed by an external system (off-cluster Alertmanager for
instance).

### API Extensions

This enhancement proposal leverages the `AlertmanagerConfig` CRD which is
exposed by the `coreos.monitoring.com/v1alpha1` API group and defined by the
upstream Prometheus operator.

The cluster monitoring operator deploys a `ValidatingWebhookConfiguration`
resource to check the validity of `AlertmanagerConfig` resources for things
that can't be enforced by the OpenAPI specification. In particular, the
AlertmanagerConfig's `Route` struct is a recursive type which isn't supported
right now by [controller-tools][controller-tools-issue]).

The validating webhook points to
the prometheus operator's service in the `openshift-user-workload-monitoring`
namespace (path: `/admission-alertmanagerconfigs/validate`).

### Implementation Details/Notes/Constraints

#### Deployment models

##### Leveraging the Platform Alertmanager

In this model, no additional Alertmanager is deployed and the user alerts are
forwarded to the existing Platform Alertmanager. This is matching the current
model.

The `Alertmanager` custom resource defines 2 LabelSelector fields
(`alertmanagerConfigSelector` and `alertmanagerConfigNamespaceSelector`) to
select which `AlertmanagerConfig` resources should be reconciled by the
Prometheus operator and from which namespace(s). Before this proposal, the
Plaform Alertmanager resource defines the 2 selectors as null, meaning that it
doesn't select any `AlertmanagerConfig` resources.

We propose to add a new boolean field `enableUserAlertmanagerConfig` to the
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
    alertmanager:
      enableUserAlertmanagerConfig: true
```

When `enableUserAlertmanagerConfig` is true, the cluster monitoring operator
configures the Platform Alertmanager to reconcile `AlertmanagerConfig`
resources from user namespaces as follows.

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
namespaces with the `openshift.io/user-monitoring: "false"` label. It allows
application owners to opt out completely from UWM in case they deploy and run
their own monitoring infrastructure (for instance with the [Monitoring Stack
operator][monitoring-stack-operator]).

In addition, the cluster admins can exclude specific user namespace(s) from UWM
with the new `excludeUserNamespaces` field.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-monitoring-config
  namespace: openshift-monitoring
data:
  config.yaml: |-
    enableUserWorkload: true
    alertmanager:
      enableUserAlertmanagerConfig: true
    excludeUserNamespaces: [foo,bar]
```

##### Dedicated UWM Alertmanager

In some environments where cluster admins and UWM admins are different personas
(e.g. OSD), it might not be acceptable for cluster admins to let users
configure the Platform Alertmanager with `AlertmanagerConfig` resources because:
* User configurations may break the Alertmanager configuration.
* Processing of user alerts may slow down the alert notification pipeline.
* Cluster admins don't want to deal with delivery errors for user notifications.

At the same time, application owners want to configure their alert
notifications without requesting external intervention.

In this case, UWM admins have the possibility to deploy a dedicated
Alertmanager. The configuration options are equivalent to the options
exposed for the Platform Alertmanager and live under the `alertmanager` key
in the UWM configmap.

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: user-workload-monitoring-config
  namespace: openshift-user-workload-monitoring
data:
  config.yaml: |-
    alertmanager:
      enabled: true
      enableUserAlertmanagerConfig: true
      logLevel: info
      nodeSelector: {...}
      tolerations: [...]
      resources: {...}
      volumeClaimTemplate: {...}
    prometheus: {}
    thanosRuler: {}
```

When `enableUserAlertmanagerConfig` is true, the UWM Alertmanager is
automatically configured to reconcile `AlertmanagerConfig` resources from all
user namespaces (just like for UWM service/pod monitors and rules). Again
namespaces with the `openshift.io/user-monitoring: false` label are
excluded.

When the UWM Alertmanager is enabled:
* The Platform Alertmanager is configured to not reconcile
  `AlertmanagerConfig` resources from user namespaces.
* The UWM Prometheus and Thanos Ruler send alerts to
  the UWM Alertmanager only.

The UWM admins are responsible for provisioning the root configuration of the
UWM Alertmanager in the
`openshift-user-workload-monitoring/alertmanager-user-workload` secret.


| User alert destination | User notifications managed by | `enableUserAlertmanagerConfig` | `alertmanager` (UWM) | `additionalAlertmanagerConfigs` (UWM) |
|------------------------|-------------------------------|:------------------------------:|:--------------------:|:-------------------------------------:|
| Platform Alertmanager | Cluster admins | false | empty | empty |
| Platform Alertmanager<br/>External Alertmanager(s) | Cluster admins | false | empty | not empty |
| Platform Alertmanager | Application owners | true | empty | empty |
| UWM Alertmanager | UWM admins | &lt;any&gt; | {enabled: true, enableUserAlertmanagerConfig: false} | empty |
| UWM Alertmanager | Application owners | &lt;any&gt; | {enabled: true, enableUserAlertmanagerConfig: true} | empty |
| UWM Alertmanager<br/>External Alertmanager(s) | UWM admins | &lt;any&gt; | {enabled: true, enableUserAlertmanagerConfig: false} | not empty |
| UWM Alertmanager<br/>External Alertmanager(s) | Application owners | &lt;any&gt; | {enabled: true, enableUserAlertmanagerConfig: true} | not empty |


#### Distinction between platform and user alerts

It is important that platform alerts can be clearly distinguished from user
alerts because cluster admins want to ensure that:
1. all alerts originating from platform components are dispatched to at least one default receiver which is owned by the admin team.
2. they aren't notified about any user alert and focus on platform alerts.

To this effect, CMO configures the Platform Prometheus instances with an alert
relabeling configuration adding the `openshift_io_alert_source="platform"`
label:

```yaml
alerting:
  relabel_configs:
  - target_label: openshift_io_alert_source
    action: replace
    replacement: platform
```

The Alertmanager configuration can leverage the label's value to make the
correct decision in the alert routing tree. For instance, the following
configuration sends all user alerts which haven't been processed by a previous
entry to an empty receiver.

```yaml
route:
  receiver: default-platform-receiver
  routes:
  - ...
  - matchers: ['openshift_io_alert_source!="platform"']
    receiver: default-user-receiver
receivers:
- name: default-platform-receiver
  ...
- name: default-user-receiver
```

Note that a similar use case was already reported in [BZ 1933239][bz-1933239].


#### Tenancy

By design, all alerts coming from UWM have a `namespace` label equal to the
`PrometheusRule` resource's namespace. The Prometheus operator relies on this
invariant to generate an Alertmanager configuration that ensures that a given
`AlertmanagerConfig` resource only matches alerts that have the same
`namespace` value. This means that an `AlertmanagerConfig` resource from
namespace `foo` only processes alerts with the `namespace="foo"` label (be it
for routing or inhibiting purposes).

Below is how the operator renders an AlertmanagerConfig resource in the final Alertmanager configuration.

```yaml
...
route:
  routes:
  # The next item is generated from an AlertmanagerConfig resource named alertmanagerconfig1 in namespace foo.
  - matchers: ['namespace="foo"']
    receiver: foo-alertmanagerconfig1-my-receiver
    continue: true
    routes:
    - ...
inhibit_rules:
# The next item is generated from an AlertmanagerConfig resource named alertmanagerconfig1 in namespace foo.
- source_matchers: ['namespace="foo"', ...]
  target_maTCHERS: ['namespace="foo"', ...]
  equal: ['namespace', ...]
receivers:
# The next item is generated from an AlertmanagerConfig resource named alertmanagerconfig1 in namespace foo.
- name: foo-alertmanagerconfig1-my-receiver
  ...
```

#### RBAC

The cluster monitoring operator ships a new cluster role
`alertmanager-config-edit` that grants all actions on `AlertmanagerConfig`
custom resources.

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

Cluster admins can bind the cluster role with a `RoleBinding` to grant
permissions to users or groups on `AlertmanagerConfig` custom resources within
a given namespace.

```bash
oc -n <namespace> adm policy add-role-to-user \
  alertmanager-config-edit <user> --role-namespace <namespace>
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

#### Disruption of Alertmanager

Even though the Prometheus operator prevents it as much as it can, it may be
possible for users to create an `AlertmanagerConfig` resource that triggers the
Prometheus operator to generate an invalid Alertmanager configuration, leading
to a potential outage of the Alertmanager cluster.

Mitigations
* If cluster admins have configured an external notification provider coupled with the always firing `Watchdog` alert, they should receive an out-of-band notification about the alerting pipeline being broken.
* The `AlertmanagerBadConfig` alert fires when Alertmanager can't reload its configuration.
* Cluster admins can exclude specific user namespaces (once the "rogue" `AlertmanagerConfig` resource(s) have been identified) to restore UWM functionality for good citizens.
* When alerts are sent to the Platform Alertmanager, cluster admins can turn off the support for `AlertmanagerConfig` in the CMO configmap so that the Platform Alertmanager cluster can process platform alerts again and the cluster admins have time to identiy the "rogue" `AlertmanagerConfig` resource(s).

#### Misconfiguration of receivers

Users may provide bad credentials for the receivers, the system receiving the
notifications might be unreachable, or the system might be unable to process
the requests. These situations would trigger the
`AlertmanagerFailedToSendAlerts` and/or `AlertmanagerClusterFailedToSendAlerts`
alerts. The cluster admins have to act on upon the alerts and understand where
the problem comes from.

Mitigations
* Detailed runbook for the `AlertmanagerFailedToSendAlerts` and `AlertmanagerClusterFailedToSendAlerts` alerts.
* Ability to use a separate Alertmanager cluster to avoid messing up with the Platform Alertmanager cluster.

#### Non-optimal Alertmanager settings

Users may use non-optimal settings for their alert notifications (such as
reevaluation of alert groups at high frequency). This may impede the
performances of Alertmanager since it would consume more resources. It can
also trigger notification failures if an exteral integration limits the number
of requests a client IP address can do.

Mitigation
* Improve the `Alertmanager` CRD to expose new fields enforcing minimum interval values for all associated `AlertmanagerConfig` resources. This would be similar to what exists at the `Prometheus` CRD level for scrape targets with `enforcedSampleLimit` for instance.

#### AlertmanagerConfig resources not being reconciled

The `AlertmanagerConfig` CRD implements schema validation for things that can
be modeled with the OpenAPI specification. However a
resource might still not be valid for various reasons:
* An alerting route contains a sub-route that is invalid (the `route` field has a self-reference to itself which means that it can't be validated at the API level).
* Credentials (such as API keys) are referenced by secrets, the
  operator doesn't have permissions to read the secret or the reference
  is incorrect (wrong name or key).

In such cases, the Prometheus operator discards the invalid
`AlertmanagerConfig` resource which isn't reconciled in the final Alertmanager
configuration.

The operator might also be unable to reconcile the AlertmanagerConfig resources temporiraly.

Mitigation
* The Prometheus operator exposes a validating admission webhook that prevents invalid resources.
* The Prometheus operator implements the `Status` subresource of the `AlertmanagerConfig` CRD to report whether or not the resource is reconciled or not (see [upstream issue][status-subresource-issue])
* Users can validate that alerting routing works as expected by generating "fake" alerts triggering the notification system. _Users don't have permissions on the Alertmanager API endpoint so they would have to generate fake alerts from alerting rules themselves. We could also support the ability to craft an alert from the OCP console_.

## Design Details

### Open Questions

1. How can the Dev Console support the UWM Alertmanager?

Users are able to silence alerts from the Dev Console and the console backend
assumes that the API is served by the
`alertmanager-main.openshift-monitoring.svc` service. To support the UWM
Alertmanager configuration, CMO should provide to the console operator the name
of the Alertmanager service managing the user alerts (either
`alertmanager-main.openshift-monitoring.svc` or
`alertmanager.openshift-user-workload-monitoring.svc`). Based on the presence
of the `openshift_io_alert_source` label, the console backend can decide which
Alertmanager service should be queried.

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

- The `AlertmanagerConfig` CRD is exposed as `v1beta1` API.
- More testing (upgrade, downgrade, scale).
- Sufficient time for feedback including signals from telemetry about the customer adoption (e.g. number of `AlertmanagerConfig` resources across the fleet).
- Counter-measures to avoid service degradation of the Platform Alertmanager.
- Option to deploy UWM Alertmanager with Console integration.
- Conduct load testing.

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

### Operational Aspects of API Extensions

#### Failure Modes

The validating webhook for `AlertmanagerConfig` resources is configured with
`failurePolicy: Fail`. Currently the validating webhook service is backed by a
single prometheus-operator pod so there is a risk that users can't
create/update AlertmanagerConfig resources when the pod isn't ready. We will
address this limitation upstream by allowing the deployment of a
highly-available webhook service ([issue][ha-webhook-service-issue]).

#### Support Procedures

N/A

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

N/A

## Alternatives

### Status-quo

An alternative is to keep the current status-quo and rely on cluster admins to
configure alert routing for their users. This proposal doesn't forbid this
model since cluster admins can decide to not reconcile user-defined
`AlertmanagerConfig` resources within the Platform Alertmanager.

### Don't support UWM Alertmanager

We could decide that CMO doesn't offer the ability to deploy the UWM
Alertmanager. In this case the responsibility of deploying an additional
Alertmanager is delegated to the cluster admins which would leverage
`additionalAlertmanagerConfigs` to point user alerts to this instance.

The downsides are
* Degraded user experience and overhead on the cluster admins.
* No console integration.
* The additional setup wouldn't be supported by Red Hat.

[user-workload-monitoring-enhancement]: https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/user-workload-monitoring.md
[uwm-docs]: https://docs.openshift.com/container-platform/4.8/monitoring/enabling-monitoring-for-user-defined-projects.html
[prometheus-operator]: https://prometheus-operator.dev/
[alertmanagerconfig-crd]: https://prometheus-operator.dev/docs/operator/api/#alertmanagerconfig
[unsupported-resources]: https://docs.openshift.com/container-platform/4.8/monitoring/configuring-the-monitoring-stack.html#support-considerations_configuring-the-monitoring-stack
[monitoring-stack-operator]: https://github.com/openshift/enhancements/pull/866
[bz-1933239]: https://bugzilla.redhat.com/show_bug.cgi?id=1933239
[controller-tools-issue]: https://github.com/kubernetes-sigs/controller-tools/issues/477
[ha-webhook-service-issue]: https://github.com/prometheus-operator/prometheus-operator/issues/4437
[status-subresource-issue]: https://github.com/prometheus-operator/prometheus-operator/issues/3335
