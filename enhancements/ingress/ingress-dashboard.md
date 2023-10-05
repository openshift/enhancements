---
title: NETOBSERV-1052: Deployment of a monitoring dashboard based on ingress operator metrics
authors:
  - "@jotak"
  - "@OlivierCazade"
reviewers:
  - "@Miciah"
  - "@candita"
approvers:
  - "@Miciah"
  - "@candita"
api-approvers:
  - "@deads2k"
creation-date: 2023-06-28
last-updated: 2023-06-28
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-139"
  - "https://issues.redhat.com/browse/NETOBSERV-1052"
see-also:
replaces:
superseded-by:
---

# Ingress dashboard creation for the OpenShift Console

## Release Signoff Checklist

- [ ] Enhancement is `implementable`.
- [ ] Design details are appropriately documented from clear requirements.
- [ ] Test plan is defined.
- [ ] graduation criteria for dev preview, tech preview, GA
- [ ] User-Facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/).

## Summary

The goal is to add a new dashboard in the OpenShift Console (in menu “Observe” > “Dashboards”), dedicated to metrics related to Ingress.
Such dashboard are deployed through a configmap, a new controller will be added to the ingress operator to manage this configmap.

Ingress components, such as HAProxy, already provide some metrics that are exposed and collected by Prometheus / Cluster Monitoring. Administrators should be able to get a consolidated view, using a subset of these metrics, to get a quick overview of the cluster state. This enhancement proposal is part of a wider initiative to improve the observability of networking components (cf https://issues.redhat.com/browse/OCPSTRAT-139).

## Motivation

While ingress related metrics already exist and are accessible in the OpenShift Console (via the menu “Observe” > Metrics”), there is no consolidated view presenting a summary of them. There are existing dashboards in other areas (such as etcd, compute resources, etc.), but networking today is less represented there, despite its importance for monitoring and troubleshooting.
Metrics such as HAProxy error rates, or latencies, can be made more visible by promoting them in a dashboard.

In addition, product management has shown interest in providing and making more visible some cluster-wide statistics such as the number of routes and shards in use.

More details on the new dashboard content is provided below.

### Goals

1. Design and implement a new dashboard using a selection of metrics listed below.
2. Update the Ingress Operator to deploy this dashboard configmap, making it accessible for the Cluster Monitoring stack.

### Non-Goals

- This work is not intended to provide a comprehensive set of metrics all at once. Instead, the intent is to "start small", setting in place all the mechanisms in code, and iterate later based on feedback to add or amend elements in the dashboard without any changes outside of the dashboard json definition file.
- This enhancement does not include any new metric creation or exposition: only already available metrics are considered. If discussions lead to consider the creation of new metrics, this could be the purpose of a follow-up enhancement.

### User Stories

> As a cluster administrator, I want to get a quick overview of general cluster statistics, such as the number of routes or shards in use.
> As a cluster administrator, I want to get a quick insight in incoming traffic statistics, such as on latency and HTTP errors.

## Proposal

To make a new dashboard discoverable by Cluster Monitoring, a `ConfigMap` needs to be created in the namespace `openshift-config-managed`, containing a static dashboard definition in Grafana format (JSON). The dashboard datasource has to be Cluster Monitoring's Prometheus.

The Ingress Operator is responsible for creating and reconciling this `ConfigMap`. We assume all metrics used in the dashboard are present unconditionally, which allows us to create a static dashboard unconditionally as well.
The Ingress Operator would embed a static and full json dashboard.
If the operator detect any change between the deployed dashboard and the embedded one, the deployed dashboard would be replaced totally by the embedded one.

### Dashboard content

At the top, a summary row presenting global cluster statistics as text panels:

- Total current byte rate in (aggregated across all routes/shards): _sum(rate(haproxy_server_bytes_in_total[1m]))_
- Total current byte rate out (aggregated across all routes/shards): _sum(rate(haproxy_server_bytes_out_total[1m]))_
- Total current number of routes: _count(count(haproxy_server_up == 1) by (route))_
- Total current number of ingress controllers: _count(count(haproxy_server_up == 1) by (pod))_

Below this top summary, more detailed time-series panels. Each of these panel come in two flavours: aggregated per route, and aggregated per controller instance.

- Byte rate in, per route or per controller instance: _sum(rate(haproxy_server_bytes_in_total[1m])) by (route)_ or _sum(rate(haproxy_server_bytes_in_total[1m])) by (pod)_

- Byte rate out, per route or per controller instance: _sum(rate(haproxy_server_bytes_out_total[1m])) by (route)_ or _sum(rate(haproxy_server_bytes_out_total[1m])) by (pod)_

- Response error rate, per route or per controller instance: _sum(irate(haproxy_server_response_errors_total[180s])) by (route)_ or _sum(irate(haproxy_server_response_errors_total[180s])) by (pod)_

- Average response latency, per route or per controller instance: _avg(haproxy_server_http_average_response_latency_milliseconds != 0) by (route)_ or _avg(haproxy_server_http_average_response_latency_milliseconds != 0) by (pod)_

### Workflow Description

A cluster administrator will be able to view this dashboard from the OpenShift Console, in the _Administrator_ view, under _Observe_ > _Dashboards_ menu.
Several dashboards are already listed there, e.g:

- `Kubernetes / Compute Resources / Pod` (tag: `kubernetes-mixin`)
- `Kubernetes / Compute Resources / Workload` (tag: `kubernetes-mixin`)
- `Kubernetes / Networking / Cluster` (tag: `kubernetes-mixin`)
- `Node Exporter / USE Method / Cluster` (tag: `node-exporter-mixin`)
- `Node Exporter / USE Method / Node` (tag: `node-exporter-mixin`)

The new ingress dashboard will be listed there, as:
- `Networking / Ingress` (tag: `networking-mixin`)

This "Networking" category can potentially be used for other Network-related dashboards, such as for OVN, NetObserv, etc.

Clicking on this dashboard will open it, showing time-series charts such as cluster ingress stats and HAProxy metrics.
On each chart, an "Inspect" link allows to view that metrics from the _Metrics_ page, which allows to customize the query (e.g: modifying the label filters, the grouping, etc.) and view the result directly.
Note that editing a query from the _Metrics_ page does not affect dashboards displayed in the _Dashboards_ page.
These behaviours are already implemented in the Console and do not necessitate any change.

### API Extensions

No planned change on the API.

### Implementation Details / Notes / Constraints

The new `ConfigMap` installed in `openshift-config-managed` should be named `grafana-dashboard-ingress` (The `grafana-dashboard-` prefix is common for all such dashboards).

It needs to be labelled with `console.openshift.io/dashboard: "true"`.

Dashboards have a `title` field as part of their Grafana JSON model, which is displayed in the OpenShift Console where dashboards are listed.
Here `title` should be `Networking / Ingress`.

Dashboards should also have a tag that identifies their supplier in the OpenShift Console; existing tags are: `kubernetes-mixin`, `node-exporter-mixin`, `prometheus-mixin`, `etcd-mixin`.
A new tag named `networking-mixin` should be used for this new dashboard. This tag aims to group all dashboards related to networking, such as also OVN dashboards and NetObserv.
This tag is directly set in the static JSON definition of the dashboard. No more action is required for tag creation.

A typical procedure to design and create the dashboard is to use Grafana for designing purpose, then export the dashboard as JSON and save it as an asset in the target repository. Then, it can be embedded in the built artifact using `go:embed`, and injected into a `ConfigMap`. Example:

```golang
//go:embed dashboard.json
var dashboardEmbed string

func buildDashboard() *corev1.ConfigMap {
	configMap := corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "grafana-dashboard-ingress",
			Namespace: "openshift-config-managed",
			Labels: map[string]string{
				"console.openshift.io/dashboard": "true",
			},
		},
		Data: map[string]string{
			"dashboard.json": dashboardEmbed,
		},
	}
	return &configMap
}
```

This is achieved by a new controller added to the operator, in charge of reconciling the dashboard. This controller watches the Infrastructure object which it is bound to, and the generated configmap.

The controller should deploy this configmap in the `openshift-config-managed`. Any configmap deployed in this namespace with the `console.openshift.io/dashboard` label will be automatically picked by the monitoring operator and deployed in the OpenShift Console. The monitoring stack is responsible for querying the metrics as defined in the dashboard.

When the Ingress operator is upgraded to a new version, if this upgrade brings changes to the dashboard, the existing ConfigMap will be overwritten through reconciliation.

### Risks and Mitigations

### Drawbacks

## Design Details

### Test Plan

Unit test

- Verify that the embedded JSON is parseable (to avoid unintentional errors while manually editing the JSON).

E2E Tests

There are two scenarios depending of the cluster network topology

1. Verify that the cluster network topology is not external
2. Verify that the new ConfigMap dashboard was created.
3. Delete the Configmap
4. Verify that the ConfigMap dashboard is recreated.
5. Modify the Configmap json
6. Verify that the ConfigMap is reinitialized to the right value.

1. Verify that the cluster network topology is external
2. Verify that the ConfigMap was not deployed

### Graduation Criteria

This enhancement does not require graduation milestones.

#### Dev Preview -> Tech Preview

N/A; This feature will go directly to GA.

#### Tech Preview -> GA

N/A; This feature will go directly to GA.

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Upgrading from a previous release must install the new dashboard without requiring any intervention.

On next upgrades, if the dashboard already exists and if the new version brings changes to the dashboard, the existing one will be overwritten with the new one.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

#### Failure Modes

N/A

#### Support Procedures

## Implementation History

## Alternatives
