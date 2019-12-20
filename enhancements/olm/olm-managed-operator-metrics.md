---
title: allow-olm-operators-to-request-a-namespace-to-be-installed-into-and-enable-cluster-monitoring-on-that-namespace
authors:
  - "@awgreene"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2019-12-11
last-updated: 2019-12-19
status: implementable
---

# allow-olm-operators-to-request-a-namespace-to-be-installed-into-and-enable-cluster-monitoring-on-that-namespace

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA

## Summary and Motivation

The primary purpose of this enhancement is to propose a series of changes that would enable [Operator-Lifecycle-Manager (OLM)](https://github.com/operator-framework/operator-lifecycle-manager) managed operators to report metrics to the [OpenShift Monitoring](https://github.com/openshift/cluster-monitoring-operator) [Prometheus Operator](https://github.com/coreos/prometheus-operator).

First, OLM will enable operators to interact with a Prometheus Operator deployed on vanilla Kubernetes clusters. This support will be implemented by allowing OLM managed operators to package `ServiceMonitor` and `PrometheusRule` objects within their bundle. OLM will then manage these resources alongside the lifecycle of the operator, as OLM currently does with a number of other resources.

Second, OLM will define how an operator author can:

- Suggest a namespace that the operator should be deployed to
- Specify that the operator should be scraped by the OpenShift Monitoring Prometheus Operator present on all OpenShift clusters.

This information will be consumed by [OpenShift Console UI](https://github.com/openshift/console) to integrate the operator into the OpenShift Monitoring Prometheus Operator.

### Goals

- Provide OLM managed operators with a first class way of creating `ServiceMonitors` and `PrometheusRules` resources to interact with the Prometheus Operator
- Enable a set of Red Hat approved Operators to integrate with the OpenShift Monitoring Prometheus Operator

### Non-Goals

- Provide generic metrics about OLM managed operators
- Own the process to integrate with Telemeter
- Support operand metric reporting

## Proposal

### Generic Prometheus Operator Support

#### OLM support for new Prometheus objects in bundles

The bundle object currently has the following format:

```bash
$ tree
bundle
    ├── manifests
    │   ├── example.crd.yaml
    │   ├── example.csv.yaml
    │   └── example.rbac.yaml
    └── metadata
        └── annotations.yaml
```

Notice that OLM understands how to create and manage CSVs, CRDs, and RBAC resources. This enhancement proposes adding support for Prometheus `ServiceMonitors` and `PrometheusRules` resources:

```bash
$ tree
bundle
    ├── manifests
    │   ├── example.crd.yaml
    │   ├── example.csv.yaml
    │   ├── example.prometheusrule.yaml
    │   ├── example.rbac.yaml
    │   └── example.servicemonitor.yaml
    └── metadata
        └── annotations.yaml
```

OLM would then need to be updated to identify the `ServiceMonitors` and `PrometheusRules` resources and deploy them as part of an `InstallPlan`. There is no way for OLM to identify which Prometheus Operator ServiceAccount should be granted the appropriate RBAC privileges to view the `ServiceMonitor` and `PrometheusRule` resource events in the namespace that the operator is being deployed to. As such, the cluster admin will need to configure the Prometheus Operator to watch for events in the correct namespace.

### OpenShift Monitoring Prometheus Operator Support

#### Requirements

Once OLM offers bundle support for the `ServiceMonitor` and `PrometheusRule` resources, a sanctioned set of OLM managed operators should be able to report metrics to the OpenShift Monitoring Prometheus Operator.

A number of requirements must be met for an application's metrics to be identified as cluster monitoring workload before it is scraped by the OpenShift Monitoring Prometheus Operator.

##### Namespace Requirements

The application must be deployed in a namespace that:

- Is prefixed with `openshift-`
- Is labeled with `openshift.io/cluster-monitoring=true`

##### ServiceMonitor Requirements

A `ServiceMonitor` object must be created in the namespace mentioned above and point to the application's metrics endpoint.

>Note: This requirement is already fulfilled with OLM bundle support for Prometheus resources.

##### RBAC Requirements

A role/rolebinding must be created that provides the Prometheus Operator ServiceAccount with the appropriate RBAC privileges to discover the `ServiceMonitor` and `PrometheusRule` resources in the newly created namespace.

#### Fulfilling Namespace and RBAC Requirements

OLM itself will not be responsible for fulfilling the Namespace and RBAC requirements. Instead, OLM will define how operator authors can provide OpenShift Console with the information required to fulfill these requirements.

OLM [building-your-csv](https://github.com/operator-framework/operator-lifecycle-manager/blob/master/doc/design/building-your-csv.md) documentation will be updated to describe two new supported annotations:

- The `operatorframework.io/suggested-namespace` annotation. When the `operatorframework.io/suggested-namespace` annotation is present the UI will highlight the suggested namespace when installing the operator. OLM itself will not require that the operator is deployed in the namespace defined by the `operatorframework.io/suggested-namespace` annotation.

- The `operatorframework.io/cluster-monitoring=true` annotation. When this annotation is set to `true`, the OpenShift Console will update the namespace that the operator is being deployed to with the `openshift.io/cluster-monitoring=true` label. When this annotation is present, the UI will update the OpenShift Monitoring Prometheus Operator ServiceAccount with the appropriate RBAC privileges for the given namespace as well, allowing operators to be scraped by the OpenShift Monitoring Prometheus Operator.

>Note: There is no work required by the OLM team to implement this support.

#### Risks and Mitigations

If OLM allows any operator to report metrics to the OpenShift Monitoring Prometheus Instance, there is a chance that the prometheus instance could be overloaded or the integrity of the data it scraps could be jeopardized. In an effort to minimize this risk, the number of operators that are granted permission to include metrics will be highly regulated. Operators must not be added to officially supported CatalogSources without first being reviewed.

Additionally, operators that introduce metrics must be added to an e2e test that ensures the metrics cardinality are under appropriate limits.

## Implementation History

- 2019/12/11: Proposal created
- 2019/12/12: Proposal updated based on feedback
- 2019/12/13: Proposal updated based on feedback
- 2019/12/16: Proposal updated based on feedback
- 2019/12/19: Proposal updated based on feedback
