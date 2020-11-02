---
title: allow-olm-operators-to-request-a-namespace-to-be-installed-into-and-enable-cluster-monitoring-on-that-namespace
authors:
  - "@awgreene"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2019-12-11
last-updated: 2020-11-02
status: implemented
---

# allow-olm-operators-to-request-a-namespace-to-be-installed-into-and-enable-cluster-monitoring-on-that-namespace

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA

## Summary and Motivation

The primary purpose of this enhancement is to enable [Operator-Lifecycle-Manager (OLM)](https://github.com/operator-framework/operator-lifecycle-manager) managed operators to easily integrate with the [Prometheus Operator](https://github.com/coreos/prometheus-operator) on OpenShift and Vanilla Kubernetes Clusters.
Additionally, this enhancement focuses on enabling OLM managed operators to record metrics with the [OpenShift Monitoring](https://github.com/openshift/cluster-monitoring-operator) Prometheus Operator present on all OpenShift Clusters.

OLM will support tight integration with the Prometheus Operator by allowing operator authors to package `ServiceMonitor` and `PrometheusRule` objects within their [Operator Bundle](https://github.com/operator-framework/operator-registry/blob/master/docs/design/operator-bundle.md) and will manage these resources alongside the lifecycle of the operator.

OLM will also enable an operator author to supply [OpenShift Console](https://github.com/openshift/console) with the following information, which is needed to integrate with OpenShift's Monitoring services:

- A suggested namespace that the operator should be deployed to.
- An indicator denoting that the operator exposes metrics that should be scraped by prometheus.

### Goals

- Provide OLM managed operators with a first class way of packaging `ServiceMonitors` and `PrometheusRules` resources as a part of their Operator Bundle.
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

OLM will then install the `ServiceMonitors` and `PrometheusRules` resources as part of an `InstallPlan`.

> Note: If a bundle includes a `ServiceMonitor` or `PrometheusRule` resource the CSV should add a dependency to each CRD so OLM can install the prometheus operator on vanilla kubernetes clusters.

On vanilla Kubernetes clusters there is no way for OLM to identify which Prometheus Operator ServiceAccount should be granted the appropriate RBAC privileges to view the `ServiceMonitor` and `PrometheusRule` resource events in the namespace that the operator is deployed in.
As such, the cluster admin will need to configure the Prometheus Operator to watch for events in the correct namespace.

### OpenShift Monitoring Prometheus Operator Support

#### Requirements

Once OLM offers bundle support for the `ServiceMonitor` and `PrometheusRule` resources, a sanctioned set of OLM managed operators should be able to expose metrics that are collected by the OpenShift Monitoring Prometheus Instance.
A number of requirements must be met for an application's metrics to be identified as cluster monitoring workload before it is scraped by the OpenShift Monitoring Prometheus Operator.

##### Namespace Requirements

The application must be deployed in a namespace that:

- Is prefixed with `openshift-`
- Is labeled with `openshift.io/cluster-monitoring=true`

##### ServiceMonitor Requirements

A `ServiceMonitor` object must be created in the namespace mentioned above and point to the application's metrics endpoint.

>Note: This requirement is already fulfilled with OLM bundle support for Prometheus resources.

##### RBAC Requirements

A role and rolebinding must be created that provides the OpenShift Monitoring `prometheus-k8s` ServiceAccount with the appropriate RBAC privileges to discover the `Services` in your operator's namespace. An example of the required RBAC can be seen below:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: prometheus-k8s
  namespace: placeholder
rules:
- apiGroups:
  - ""
  resources:
  - services
  - endpoints
  - pods
  verbs:
  - get
  - list
  - watch
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: prometheus-k8s
  namespace: placeholder
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: prometheus-k8s
subjects:
- kind: ServiceAccount
  name: 'prometheus-k8s',
  namespace: 'openshift-monitoring',
```

#### Fulfilling Namespace and RBAC Requirements

The OpenShift Monitoring Requirements mentioned above can be met in one of two ways:

- By using annotations that instructs OpenShift Console to generate the required Namespace and RBAC Requirements
- By packaging the RBAC [mentioned above](#####rbac-requirements) within the Operator Bundle and having a cluster admin install the operator in a namespace that is both prefixed with `openshift-` and has the `openshift.io/cluster-monitoring=true` label.

##### Fulfilling the Namespace and RBAC Requirements via Console

OLM  [building-your-csv](https://github.com/operator-framework/operator-lifecycle-manager/blob/master/doc/design/building-your-csv.md) documentation will be updated to describe two new supported annotations:

- The `operatorframework.io/suggested-namespace` annotation.
When the `operatorframework.io/suggested-namespace` annotation is present the UI will highlight the suggested namespace when installing the operator.
OLM itself will not require that the operator is deployed in the namespace defined by the `operatorframework.io/suggested-namespace` annotation.
- The `operatorframework.io/cluster-monitoring=true` annotation.
When this annotation is set to `true`, the OpenShift Console will update the namespace that the operator is being deployed to with the `openshift.io/cluster-monitoring=true` label.
When this annotation is present, the UI will update the OpenShift Monitoring `prometheus-k8s` ServiceAccount with the [required RBAC](#####rbac-requirements) privileges for the given namespace as well, allowing operators to be scraped by the OpenShift Monitoring Prometheus instance.

#### Risks and Mitigations

If OLM allows any operator to report metrics to the OpenShift
Monitoring Prometheus Instance, there is a chance that the prometheus
instance could be overloaded or the integrity of the data it scraps
could be jeopardized. In an effort to minimize this risk, the number
of operators that are granted permission to include metrics will be
highly regulated. Operators must not be added to officially supported
CatalogSources without first being reviewed.

Additionally, operators that introduce metrics must be added to an e2e test that ensures the metrics cardinality are under appropriate limits.

## Implementation History

- 2019/12/11: Proposal created
- 2019/12/12: Proposal updated based on feedback
- 2019/12/13: Proposal updated based on feedback
- 2019/12/16: Proposal updated based on feedback
- 2019/12/19: Proposal updated based on feedback
- 2020/10/20: Proposal updated to include steps to have an operator report metrics when installed via the command line
