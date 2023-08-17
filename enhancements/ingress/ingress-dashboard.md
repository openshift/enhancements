---
title: NETOBSERV-1052: Deployment of a monitoring dashboard based on ingress operator metrics
authors:
  - "@jotak"
reviewers:
  - "@Miciah"
  - "@candita"
  - "@OlivierCazade"
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

In addition, product management has shown interest in providing and making more visible some cluster-wide statistics such as the number of routes and shards in use.

### Goals

1. Review and select a subset of the available metrics that should be included in the new dashboard. This should ideally be done with the help of the NetEdge team expertise and/or SREs.
2. Design and implement a new dashboard with these metrics.
3. Update the Ingress Operator to deploy this dashboard configmap, making it accessible for Cluster Monitoring stack.

### Non-Goals

- This work is not intended to provide a comprehensive set of metrics all at once. Instead, the intent is to "start small", setting in place all the mechanisms in code, and iterate later based on feedback to add or amend elements in the dashboard without any changes outside of the dashboard json definition file.
- This enhancement does not include any new metric creation or exposition: only already available metrics are considered. If discussions lead to consider the creation of new metrics, this could be the purpose of a follow-up enhancement.

### User Stories

> As a cluster administrator, I want to get a quick overview of general cluster statistics, such as the number of routes or shards in use.
> As a cluster administrator, I want to get a quick insight in incoming traffic statistics, such as on latency and HTTP errors.

## Proposal

To make a new dashboard discoverable by Cluster Monitoring, a `ConfigMap` needs to be created in the namespace `openshift-config-managed`, containing a static dashboard definition in Grafana format (JSON). The dashboard datasource has to be Cluster Monitoring's Prometheus.

The Ingress Operator is responsible for creating and reconciling this `ConfigMap`. We assume all metrics used in the dashboard are present unconditionally, which allows us to create a static dashboard unconditionally as well. The Ingress Operator would embed a static and full json dashboard. If the operator detect any change between the deployed dashboard and the embedded one, the deployed dashboard would be replaced totally by the embedded one.
In the event where conditional display should be introduced later (e.g. an alternative to HAProxy being implemented, resulting is different metrics to display), different approaches can be discussed: the dashboard could be dynamically amended via JSON manipulation; or several dashboards could be embedded and selectively installed.

The content of the dashboard will be further discussed and refined, including during review and verification.
As a starting point, we can focus on metrics mentioned in [NE-1059](https://issues.redhat.com/browse/NE-1059):

- Number of external connections to each HAproxy instance/pod that could be grouped by client's source IP
- Number/IP/names of backends per route
- Balancing option/algorithm used by a route
- Traffic volume (Mbps) on each ingress instance
- Refresh rate of HAproxy
- Route's backend failure count

And also global statistics such as:
- Number of routes / shards / routes per shard

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

The controller should then deploy this configmap in the `openshift-config-managed`. Any configmap deployed in this namespace with the `console.openshift.io/dashboard` label will be automatically picked by the monitoring operator and deployed in the OpenShift Console.

### Risks and Mitigations

### Drawbacks

## Design Details

### Test Plan

Unit test

- Verify that the embedded JSON is parseable and contains expected elements for a dashboard (rows, panels, etc.)

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

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

#### Failure Modes

N/A

#### Support Procedures

## Implementation History

## Alternatives
