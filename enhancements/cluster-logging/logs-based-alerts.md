---
title: logs-based-alerts
authors:
  - "@periklis"
reviewers:
  - "@alanconway"
  - "@xperimental"
  - "@jcantrill"
  - "@simonpasquier"
  - "@fpetkovski"
approvers:
  - "@alanconway"
  - "@simonpasquier"
  - "@fpetkovski"
api-approvers:
  - "@alanconway"
creation-date: "2022-05-16"
last-updated: "2022-05-19"
tracking-link:
  - https://issues.redhat.com/browse/LOG-2510
see-also: []
replaces: []
superseded-by: []
---

# Logs-based Alerts

## Summary

This document proposes a solution and a delivery plan for pushing logs-based alerts from OpenShift Logging Loki to Cluster-Monitoring AlertManager. It complements the existing [user-workload monitoring stack][uwm-docs] and [alerting stack][alerting-docs], enabling a full self-service experience for platform and workload logs-based monitoring.

## Motivation

Since OpenShift 4.6, application owners can configure alerting rules based on metrics themselves as described in [User Workload Monitoring][user-workload-monitoring-enhancement] (UWM)
enhancement. The rules are defined as `PrometheusRule` resources and can be based on platform and/or application metrics.

To expand the alerting capabilities on logs as an observability signal, cluster admins and application owners should be able to configure alerting rules as described in the [Loki Rules][loki-rules-docs] docs and in the [Loki Operator Ruler][loki-operator-ruler-support] upstream enhancement.

[AlertingRule][alertingrule-crd] CRD fullfills the requirement to define alerting rules for Loki similar to `PrometheusRule`.

[RulerConfig][rulerconfig-crd] CRD fullfills the requirement to connect the Loki Ruler component to notify a list of Prometheus AlertManager hosts on firing alerts.

### User Stories

* As an application owner, I want to use AlertingRule custom resources to notify AlertManager on firing alerts from my workload logs.
* As a cluster admin, I want to use AlertingRule custom resources to notify AlertManager on firing alerts from the platform logs.
* As a cluster admin, I want to use AlertingRule custom resources to notify AlertManager on firing alerts from the audit logs.

### Goals

* Application Owners can configure alert notifications for application alerts based on workload logs.
* Cluster admins can configure alert notifications for platform alerts based on platform logs.
* Cluster admins can configure alert notifications for platform alerts based on audit logs.

### Non-Goals

* Specific alerting support on the OCP console for Logs-based alerts. This should be seamless in the existing alerting UIs.
* Recording rules to compile metrics from logs.

## Proposal

We plan to leverage the `AlertingRule` and `RulerConfig` custom resource definitions already exposed by the Loki Operator so that:
- cluster admins can configure how the platform logs can notify on alertworthy platform and/or audit logs.
- application owners can configure how the workload logs can notify on alertworthy logs.

### Workflow Description

#### Logs-based Alerts for Application Owners

1. The application owner creates an AlertingRule custom resource on a workload project, e.g.

```yaml
apiVersion: loki.grafana.com/v1beta1
kind: AlertingRule
metadata:
  name: my-workload-alerts
  namespace: my-workload-ns
  labels:
    openshift.io/cluster-monitoring: "true"
spec:
  tenantID: application
  groups:
    - name: MyApplication
      rules:
        - alert: MyApplicationHighPercentageError
          expr: |
            sum(rate({kubernetes_namespace_name="my-workload-ns", kubernetes_pod_name=~"my-workload.*"} |= "error" [1m])) by (job)
              /
            sum(rate({kubernetes_namespace_name="my-workload-ns", kuberentes_pod_name=~"my-workload.*"}[1m])) by (job)
              > 100
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: My Application High Errors.
            description: My application throws a high amount of error logs.
```

2. The application owner can only define AlertingRules custom resources in project with granted access to them and only for the tenant `application`.
3. The application owner can inspect firing alerts from the above rule in Developer > Observe > Alerts.

#### Logs-based Alerts for Cluster Admins

1. The cluster admin creates an AlertingRule custom resource on a platform project, e.g.

```yaml
apiVersion: loki.grafana.com/v1beta1
kind: AlertingRule
metadata:
  name: my-operator-alerts
  namespace: openshift-operators-redhat
  labels:
    openshift.io/cluster-monitoring: "true"
spec:
  tenantID: infrastructure
  groups:
    - name: MyOperator
      rules:
        - alert: MyOperatorHighPercentageError
          expr: |
            sum(rate({kubernetes_namespace_name="openshift-operators-redhat", kubernetes_pod_name=~"my-operator.*"} |= "error" [1m])) by (job)
              /
            sum(rate({kubernetes_namespace_name="openshift-operators-redhat", kuberentes_pod_name=~"my-operator.*"}[1m])) by (job)
              > 100
          for: 5m
          labels:
            severity: warning
          annotations:
            summary: My Operator High Errors.
            description: My operator throws a high amount of error logs.
```

2. The cluster admin can define AlertingRule custom resources all projects for tenants `application`, `infrastructure` and `audit`
3. The cluster admin can inspect firing alerts from the above rule in Administrator > Observe > Alerting > Alerts.

### API Extensions

As mentioned above this document proposes to leverage two existing CRDs namely `AlertingRule` and `RulerConfig` provided by the Loki Operator.

### Implementation Details/Notes/Constraints

Since we don't introduce or modify any existing custom resources, the following section provides an overview of how the Loki Ruler configuration in form of a `RulerConfig` custom resource is leveraged to connect the Loki Ruler component with Cluster Monitoring AlertManager.

__Note:__ The following defaults for the `RulerConfig` custom resource is applied only for tenant mode `openshift-logging` (See for modes more info [here][lokistack-tenant-modes])

1. To connect the loki ruler with the ClusterMonitoring hosts we need to default the RulerConfig, i.e:

```yaml
apiVersion: loki.grafana.com/v1beta1
kind: RulerConfig
metadata:
  name: lokistack-dev
spec:
  alertmanager:
    enableV2: true
    endpoints:
      - "https://alertmanager-operated.openshift-monitoring.svc:9095"
```

2. To autheticate and access the alertmanager host the loki ruler service account requires RBAC rules as follows:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: lokistack-dev-ruler
rules:
- apiGroups:
  - monitoring.coreos.com
  resourceNames:
  - non-existant
  resources:
  - alertmanagers
  verbs:
  - patch
```

and a namespaced-scoped binding in `openshift-logging`:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: lokistack-dev-ruler
  namespace: openshift-logging
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: lokistack-dev-ruler
subjects:
- kind: ServiceAccount
  name: lokistack-dev-ruler
  namespace: openshift-logging
```

3. In addition the ruler component requires specific TLS configuration for its alertmanager client as follows:

```yaml
spec:
  containers:
  - name: ruler
    args:
    - "-ruler.alertmanager-client.tls-ca-path=/var/run/ca/service-ca.crt"
    - "-ruler.alertmanager-client.tls-server-name=alertmanager-main.openshift-monitoring.svc"
    - "-ruler.alertmanager-client.credentials-file=/var/run/secrets/kubernetes.io/serviceaccount/token"
```

__Note:__ The `service-ca.crt` is mounted into the ruler container via a ConfigMap using the `service.beta.openshift.io/inject-cabundle: true` provided by the [ServiceCAOperator][service-ca-operator-docs]

### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?


## Design Details

### Open Questions [optional]

1. How can we use the existing the OpenShift Console and Thanos-Querier instaces to make logs-based alerts from the Loki Rules API?
2. How can we leverage [multi-tenant alerting][multi-tenant-alerting] capabilities of UWM? What is required from Loki side?

### Test Plan

TBD

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

- 2022-05-19: : Initial draft proposal

## Alternatives

N/A

[user-workload-monitoring-enhancement]: https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/user-workload-monitoring.md
[uwm-docs]: https://docs.openshift.com/container-platform/4.10/monitoring/enabling-monitoring-for-user-defined-projects.html
[alerting-docs]: https://docs.openshift.com/container-platform/4.10/monitoring/managing-alerts.html
[multi-tenant-alerting]: https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/multi-tenant-alerting.md
[loki-rules-docs]: https://grafana.com/docs/loki/latest/rules/#alerting-rules
[loki-operator-ruler-support]: https://github.com/grafana/loki/blob/main/operator/docs/enhancements/ruler_support.md
[lokistack-tenant-modes]: https://github.com/openshift/enhancements/blob/master/enhancements/cluster-logging/loki-storage.md
[alertingrule-crd]: https://github.com/grafana/loki/blob/main/operator/docs/enhancements/ruler_support.md#alertingrule-definition
[rulerconfig-crd]: https://github.com/grafana/loki/blob/main/operator/docs/enhancements/ruler_support.md#rulerconfig-definition
[service-ca-operator-docs]: https://docs.openshift.com/container-platform/4.10/security/certificate_types_descriptions/service-ca-certificates.html
