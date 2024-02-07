---
title: replace-prometheus-adapter-with-metrics-server
authors:
  - "@slashpai"
reviewers:
  - "@simonpasquier"
  - "@dgrisonnet"
  - "@mansikulkarni96"
  - "@joelsmith"
  - "@openshift/openshift-team-monitoring"
approvers: 
  - "@simonpasquier"
  - "@dgrisonnet"
  - "@openshift/openshift-team-monitoring"
api-approvers:
  - "@deads2k"
creation-date: 2023-06-20
last-updated: 2023-11-24
tracking-link:
  - https://issues.redhat.com/browse/MON-3153
---

# Replace prometheus-adapter with metrics-server

## Summary

This proposal suggests replacing the [Prometheus Adapter](https://github.com/kubernetes-sigs/prometheus-adapter) currently used for implementing the resource [metrics API](https://github.com/kubernetes/metrics) in OpenShift with the [Metrics Server](https://github.com/kubernetes-sigs/metrics-server).

## Motivation

For vertical and horizontal pod autoscaling, a resource metrics API implementation is necessary. Currently, OpenShift relies on the Prometheus stack and Prometheus adapter managed by the Cluster Monitoring Operator (CMO).

The reasons to move from prometheus-adapter to Metrics Server are:

1. **Upcoming Archival of prometheus-adapter:** The Prometheus Adapter project will soon be archived due to difficulties in maintaining the project, making it less desirable for long-term usage.

1. **Simplified and more robust Monitoring architecture:** By replacing Prometheus Adapter with Metrics Server, we can reduce the complexity and maintenance overhead of the monitoring system. Since `metrics-server` does not rely on Prometheus, the resources metrics API would remain functional even when the Prometheus service is down.

1. **Lightweight Solution:** Deploying and configuring Prometheus Adapter requires additional effort compared to Metrics Server. Prometheus Adapter is tightly coupled to Prometheus and this creates a bottleneck for allowing monitoring as optional feature. See [MON-3152](https://issues.redhat.com/browse/MON-3152).

1. **Better Accuracy:**  By default, the Metrics Server takes into account the timestamps of metric samples, allowing for accurate tracking of resource utilization over time. This means you can rely on the Metrics Server to provide a more precise understanding of how resources are being utilized by your applications.

1. **Scability:** Starting from `v0.5.0` Metrics Server comes with default resource requests that guarantee good performance for most cluster configurations and it continues to do [scale testing](https://github.com/kubernetes-sigs/metrics-server/blob/077a462cd660beef705afb0a4a14a707ef39c4f2/FAQ.md#how-large-can-clusters-be) which is not the case for Prometheus Adapter.

### User Stories

#### Story 1

* In order to provide [optional built-in monitoring](https://issues.redhat.com/browse/MON-3152) feature we need to provide a way to support HPA and VPA without a Prometheus-based solution.

#### Story 2

* As a cluster admin, I seek a lightweight and efficient solution with improved accuracy for monitoring resource metrics within my cluster.

#### Story 3

* As a cluster admin, I want a simplified monitoring architecture in OpenShift, so that I can reduce the complexity and maintenance overhead of the monitoring system.

### Goals

* Replace Prometheus Adapter with Metrics Server while maintaining feature parity with the existing stack 
* Ensure no performance regression occurs when replacing Prometheus Adapter with Metrics Server
* Improved scale up time for autoscaling

### Non-Goals

* Allow optional cluster monitoring stack
* Moving `openshift-monitoring/cluster-monitoring-config` config-map to a Custom Resource

## Proposal

The proposal is to replace the Prometheus Adapter with the Metrics Server for monitoring resource metrics in OpenShift.

### Workflow Description

The workflow for using the Metrics Server in OpenShift for monitoring resource metrics would involve the following steps:

1. User enables `TechPreviewNoUpgrade` featureset

    ```yaml
    apiVersion: config.openshift.io/v1
    kind: FeatureGate
    metadata:
      name: cluster
    spec:
      featureSet: TechPreviewNoUpgrade
    ```

    With this configuration, the cluster monitoring operator should:
    1. Deploy the `metrics-server` resources in the `openshift-monitoring` namespace
    1. Wait for the metrics-server task to be complete which involves the metrics-server deployment and service being ready.
    1. Update `APIService` resource to point to `metrics-server` API instead of `prometheus-adapter` API
    1. Delete `prometheus-adapter` resources

### API Extensions

No new API extensions are required. For tech preview, the only update required
is the addition of a `MetricsServer` feature gate to the OpenShift API
`TechPreviewNoUpgrade` feature set. See [Design Details](#design-details) for
the specifics.

### Implementation Details/Notes/Constraints

#### Impact for Hosted Control Planes (HyperShift)

1. The existing resource metrics API implementation, prometheus-adapter, is deployed on the guest cluster's data plane by CMO which is itself deployed by the Cluster Version operator. The same will be true for the Metrics server because it requires kubelet scraping. Consequently, we don't anticipate any effects on Hypershift as part of this enhancement.

#### Impact for Windows nodes

1. We took help from the OCP Windows team to verify that no impact on Windows nodes in the cluster and metrics for Windows nodes are reported correctly. Both `oc adm top pods` and `oc adm top nodes` commands work as expected, even for the win-webserver pods. More details in [MON-3514](https://issues.redhat.com/browse/MON-3514).

#### Configuration/Enablement

Initially the Metrics Server must be explicitly enabled by the user by configuring the `TechPreviewNoUpgrade` feature set. 

#### Resource Impact

Once metrics-server is enabled, by default 2 replicas of `metrics-server` instances are being deployed for multi-nodes clusters and 1 for SNO clusters in the openshift-monitoring namespace.

Once metrics-server is fully functional (metrics-server deployment in Ready state) we will remove `prometheus-adapter` resources fully.

### Risks and Mitigations

With prometheus-adapter only Prometheus scrapes kubelet, but with metrics-server you need both Prometheus and metrics-server to scrape the kubelet in order to also get container metrics in Prometheus.

We compared cpu and memory usage of metric-server vs prometheus-adapter on cluster-bot's cluster in [MON-3210](https://issues.redhat.com/browse/MON-3210) which didn't see significant difference when prometheus-adapter was replaced with metrics-server.

We need to do performance testing on a larger OpenShift cluster (more than 100 nodes) and more than 70 pods on per node to measure the performance of Metrics Server. This is based on scalability table mentioned in [metrics-server](https://github.com/kubernetes-sigs/metrics-server#scaling). [MON-3328](https://issues.redhat.com/browse/MON-3328) is created for doing this.

### Drawbacks

N/A


## Design Details

We will add a new feature gate `MetricsServer` which will be enabled by default in the `TechPreviewNoUpgrade` feature set.

For the cluster monitoring operator, we propose to add a type `metricsServer` to the `openshift-montoring/cluster-monitoring-config` configmap defining the
metrics-server settings such as pod placement and resource requirements.

  ```go
  type ClusterMonitoringConfiguration struct {
    ...
    // `MetricsServerConfig` defines settings for the MetricsServer component.
    MetricsServerConfig *MetricsServerConfig `json:"metricsServer,omitempty"`
  }

  // The `MetricsServerConfig` resource defines settings for the MetricsServer component.
  type MetricsServerConfig struct {
    // Defines the audit configuration used by the Metrics Server instance.
    Audit *Audit `json:"audit,omitempty"`
    // Defines the nodes on which the pods are scheduled.
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`
    // Defines tolerations for the pods.
    Tolerations []v1.Toleration `json:"tolerations,omitempty"`
    // Defines resource requests and limits for the Metrics Server container.
    Resources *v1.ResourceRequirements `json:"resources,omitempty"`
    // Defines a pod's topology spread constraints.
    TopologySpreadConstraints []v1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
  }
  ```

The cluster monitoring operator will check the presence of the `MetricsServer` FeatureGate: if present, it will configure the Metrics
Server resources in `openshift-monitoring` namespace and tears down the Prometheus Adapter resources once Metrics Server is fully functional. We will use the Prometheus Adapter settings to configure the Metrics server when the upgrade
happens to avoid unpleasant surprises for users.

### Test Plan

The proposed solution will be tested with two suites:

* Unit tests - running independently of each other and without side effects
* End to end tests - in the same repository as the cluster-monitoring-operator. These tests will be executed against an actual Kubernetes cluster and are intended to verify end-to-end functionality of the entire system.

E2E Test will test following:
* Enables metrics-server
* Verifies that the resource metrics API is working
* During the test, it will ensure that there's no disruption of the resource metrics API.

TechPreview jobs will be configured for the cluster monitoring operator and metrics-server repositories.

### Graduation Criteria

The main rationale behind adopting a phased approach is to do:
- More testing
- Conduct Peformance testing on larger cluster
- Sufficient time for feedback

* 4.15 TechPreview
  * Notice in the release notes that prometheus-adapter is deprecated (should be repeated on every future release).
* 4.16 GA, the `MetricsServer` feature gate will be enabled in the default feature set.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

More testing (upgrade, downgrade, scale).
* Sufficient time for feedback
* Conduct load testing.

#### Removing a deprecated feature

We will add a notice in the `4.15` release notes that `prometheus-adapter` is deprecated. The information should be repeated until Metrics server becomes GA.

In the OCP release following the GA graduation of Metrics server, we will remove the prometheus-adapter support from the CMO code base. We will also drop the  `MetricsServer` [FeatureGate](https://github.com/openshift/api/blob/b86761094ee3d5aa3a94a521ba50081de65a6522/config/v1/types_feature.go#L185) from the `TechPreviewNoUpgrade` FeatureSet.

### Upgrade / Downgrade Strategy

When promoting the feature gate to GA, we will verify that CMO is able to deal with OCP downgrade. We will not look for 100% availability of the resource metrics API during the downgrade though.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

`metrics-server` will be deployed in HA mode (2 replicas by default). We will define Pod AntiAffinity Rule in `metrics-server` Deployment spec and create `PodDisruptionBudget` config to meet HA criteria.

`KubeAggregatedAPIErrors` and `KubeAggregatedAPIDown` alerts already exist to monitor the `APIService`. Additionaly metrics-server exposes [internal metrics](https://github.com/kubernetes-sigs/metrics-server/blob/796fc0f832c1ac444c44f88a952be87524456e07/pkg/server/metrics.go#L29-L46). A few of the metrics that can be useful:


1. `metrics_server_manager_tick_duration_seconds` : The total time spent collecting and storing metrics in seconds

1. `metrics_server_kubelet_request_duration_seconds` : Duration of requests to Kubelet API in seconds.

1. `metrics_server_kubelet_request_total` : Number of requests sent to Kubelet API

1. `metrics_server_storage_points` : Number of metrics points stored in memory.

We will add alerting rules to detect the following conditions:

* The collect time exceeds the metrics resolution (e.g. 15 seconds) which would decrease the accuracy.

* The Metrics server fails to scrape metrics from some kubelet targets.
These alerting rules will have a `warning` severity.

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

N/A

## Alternatives

1. The alternative would be creating a standalone Metrics Server operator that would work in conjuntion with CMO.
But for the foreseeable future we’ll have CMO in the picture, but with a reduced profile.
So CMO deploying metrics-server and managing is a better option since CMO manages prometheus-adapter and 
it will already have info to delete those while switching to metrics-server. Having independent operators
could make migration trickier as we need a way for both operators to collaborate. It also avoids the overhead
to maintain and manage another operator and we can utilize existing test framework available in CMO to do
e2e testing as well.

1. Another approach involves deploying the metrics-server through CRD-based configuration. Given that all other configurations are provided via the `openshift-monitoring/cluster-monitoring-config` Config Map, this seems to be a more organized approach for the sake of clarity.

However, in the context of the [MON-1100](https://issues.redhat.com/browse/MON-1100), it could be viewed as an alternative method for defining metrics-server configurations.
