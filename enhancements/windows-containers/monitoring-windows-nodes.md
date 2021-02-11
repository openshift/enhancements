---
title: monitoring-windows-nodes
authors:
  - "@VaishnaviHire"
  - "@PratikMahajan"
  - "@MansiKulkarni"
reviewers:
  - "@@openshift/openshift-team-windows-containers"
  - "@simonpasquier"
  - "@spadgett"
approvers:
  - "@aravindhp"
  - "@simonpasquier"
creation-date: 2021-02-08
last-updated: 2021-03-04
status: implementable
---

# Monitoring Windows Nodes

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The intent of this enhancement is to enable performance monitoring on Windows
nodes created by Windows Machine Config Operator(WMCO) in OpenShift cluster.

## Motivation

Monitoring is critical to identify issues with nodes, containers running on the
nodes. The main motivation behind this enhancement is to enable monitoring on
the Windows nodes.

### Goals

As part of this enhancement, we plan to do the following:
* Run [windows_exporter](https://github.com/prometheus-community/windows_exporter)
  as a service on Windows nodes
* Upgrade the windows_exporter on the Windows Nodes
* Leverage cluster-monitoring operator that sets up Prometheus, Alertmanager
  and other components

### Non-Goals

As part of this enhancement, we do not plan to do the following:
* Integrating windows_exporter with cluster monitoring operator

## Proposal

The main idea here is to run windows_exporter as a Windows Service and let
Prometheus instance which was provisioned as part of OpenShift install to
collect data from windows_exporter. The metrics exposed by the windows_exporter
will be used to display console graphs for Windows nodes.

### User Stories

Stories can be found within the [Enable monitoring for all Windows components epic](https://issues.redhat.com/browse/WINC-590)

## Justification

Unlike [Node exporter](https://github.com/prometheus/node_exporter) on Linux
nodes, windows_exporter cannot run as a container on the Windows nodes since
Windows container images contains a Windows Kernel and Red Hat has a policy not
to ship third party kernels for support reasons. Please refer to the [WMCO
 enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/windows-containers/windows-machine-config-operator.md#justification)
for more details.

### Risks and Mitigations

* Running `windows_exporter` as a Windows Service, posses a risk of having
  inadequate resources to run the service if the Windows node is overwhelmed
  with workload containers. This can be mitigated by leveraging [priority
   classes](https://docs.microsoft.com/en-us/windows/win32/procthread/scheduling-priorities) for
  Windows processes. This is similar to what is being done for other [Windows
  services](https://issues.redhat.com/browse/WINC-534).
  
* One of the risks with the current approach is renaming Windows metrics to
  display pod graphs. The pod metrics for Linux come from cAdvisor. However, we
  do not get same metrics from cAdvisor for Windows nodes. This becomes a
  hindrance to display pod graphs by creating custom recording rules to use same
  console queries as Linux workloads. To mitigate this, use metrics exposed by
  the windows_exporter to display pod graphs as mentioned in the [Future
   Plans](#future-plans) is required. This also requires changes in console
  queries that support OS specific metrics.

## Design Details

As we are not able to run windows_exporter as a [container](#justification)
on the Windows Node, to capture data from windows_exporter, WMCO creates a
`windows-machine-config-operator-metrics` Service without selectors and
manually defines Endpoints object for that service. The Endpoints object has
entries for the endpoints `<internal-ip>:9182/metrics`, exposed by
windows_exporter for every Windows node. Once the Service and Endpoints
object is created, WMCO ensures that a Service Monitor for `windows-machine-config-operator-metrics`
Service is running so that the Prometheus operator can discover the targets
created above to scrape Windows metrics. Following design details reflect the
current approach and future plans to enable monitoring support for Windows.

### Current State

To enable basic monitoring support for Windows node, WMCO has done the
following:

* Build and add windows_exporter binary to WMCO payload.
* Install windows_exporter on the Windows nodes and ensuring
  that it runs as a Windows service.
* Add `openshift.io/cluster-monitoring=true` label to the
 `openshift-windows-machine-config-operator` namespace so that cluster
  monitoring stack will pick up the Service Monitor created by WMCO.
* Add privileges to WMCO to create Services, Endpoints, Service Monitor in
  the `openshift-windows-machine-config-operator` namespace.
* Create a Service and Endpoints object in `openshift-windows-machine-config
-operator` namespace that point to windows_exporter endpoint. WMCO uses default
  values to define metrics endpoint, `<internal-ip>:9182/metrics`,
  exposed by windows_exporter for every Windows node. The Endpoints object
  created in the namespace consist of subsets of endpoints from all the
  Windows nodes.
* Create a Service Monitor in `openshift-windows-machine-config-operator`
  namespace for Service created above.
  
To display node graphs WMCO has done the following:

* Add custom Prometheus rules in `openshift-windows-machine-config-operator`
  namespace. The custom recording rules are created using Windows metrics
  exposed by the windows_exporter and have the same names as Linux
  recording rules. This is to make use of same console queries as Linux.
* Note that WMCO is unable to display pod graphs for the Windows Nodes
  with the current implementation. See [Risks and Mitigations](#risks-and-mitigations)
  for details.
  
### Future Plans

#### Displaying Console Graphs

* As we move forward, our plan to display monitoring graphs is to create a
 [common interface](https://issues.redhat.com/browse/WINC-530) for Windows
  and Linux recording rules. Monitoring team will define recording rules for the
  metrics that have different `metric labels` for Linux and Windows. The
  differences in `metric labels` for metrics used for Node graphs and pod graphs
  are displayed in the tables below.
  The Windows team will align the Windows recording rules with these new
  recording rules. The recording rules for Windows will be managed by
  WMCO. This set of common recording rules for monitoring will return results
  for both Linux and Windows nodes for a single query.The console queries
  currently use some raw metrics such as `node_filesystem_size_bytes`,
  `node_filesystem_free_bytes` etc. They would need to be updated to include
  the new recording rules in place of using raw metrics. This will ensure that
  we have a consistent user experience for monitoring across Linux and Windows.
* In the cases where `metric labels` are equivalent, we plan to relabel the
  Windows metrics to align with the Linux metrics.

##### Node Metrics

| Node Exporter                  | Windows Exporter                 | Label Difference                                                         |                    Metric Usage for console
|--------------------------------|----------------------------------|--------------------------------------------------------------------------|-----------------------------------------------------------------------------------------|
| node_memory_MemTotal_bytes     | windows_cs_physical_memory_bytes | -                                                                        |  WMCO renames the Windows metric to node_memory_MemTotal_bytes (no label difference)    |
| node_memory_MemAvailable_bytes | windows_memory_available_bytes   | -                                                                        |  WMCO renames the Windows metric to node_memory_MemAvailable_bytes (no label difference)|
| node_filesystem_size_bytes     | windows_logical_disk_size_bytes  | Missing Labels: (device, mountpoint, fstype) Additional label : (volume) |  WMCO and CMO create a new recording rule instance:filesystem_size_bytes:sum            |
| node_filesystem_free_bytes     | windows_logical_disk_free_bytes  | Missing Label: device, mountpoint, fstype) Additional label : (volume)   |  WMCO and CMO create a new recording rule instance:filesystem_free_bytes:sum            |
| node_cpu_seconds_total         | windows_cpu_time_total           | Missing Label : cpu Additional Label: core                               |  WMCO creates recording rule instance:node_cpu:rate:sum                                 |

##### Pod Metrics

| Kubelet metrics                        | Windows Kubelet     | Windows Exporter                                        | Label Difference                                                                                           |      Metric Usage for console
|----------------------------------------|---------------------|---------------------------------------------------------|------------------------------------------------------------------------------------------------------------|--------------------------------------------------------------------------------------|
| kubelet_running_pods                   | kubelet_running_pods| windows_container_available                             | -                                                                                                          | WMCO renames the Windows metric to kubelet_running_pods (no label difference)        |
| container_memory_working_set_bytes     | -                   | windows_container_memory_usage_private_working_set_bytes| Missing Label: (image) Additional Label: (container_id) which is equivalent of (id) for Linux              | WMCO creates a new recording rule pod:container_memory_usage_bytes:sum (used by CMO) |
| container_cpu_usage_seconds_total      | -                   | windows_container_cpu_usage_seconds_total               | Missing Label: (image, metrics_path) Additional Label:(container_id) which is equivalent of (id) for Linux | WMCO creates a new recording rule pod:container_cpu_usage:sum (used by CMO)          |
| container_fs_usage_bytes               | -                   | -                                                       |                                                                                                            |                                                                                      |
| container_network_receive_bytes_total  | -                   | windows_container_network_receive_bytes_total           | Missing Label: (image, metrics_path) Additional Label:(container_id) which is equivalent of (id) for Linux | WMCO and CMO create new recording rule pod:container_network_receive_bytes_total:sum |                                                                                  |
| container_network_transmit_bytes_total | -                   | windows_container_network_transmit_bytes_total          | Missing Label: (image, metrics_path) Additional Label:(container_id) which is equivalent of (id) for Linux | WMCO and CMO create new recording rule pod:container_network_transmit_bytes_total:sum|                                                                                 |

#### Add Grafana Dashboard for Windows Nodes

* To reduce the gaps with the current Grafana dashboard in the monitoring menu, we plan to add a dashboard as a ConfigMap for displaying the Windows Node metrics in the "openshift-config-managed" namespace with the console.openshift.io/dashboard=true label.
  A dashboard for Windows exists in the upstream [kubernetes-monitoring/kubernetes-mixin](https://github.com/kubernetes-monitoring/kubernetes-mixin/blob/master/dashboards/windows.libsonnet) repository which can be used as a reference for adding a Windows dashboard.

#### Moving towards EndpointSlices

* Since the metrics Endpoints object is managed by WMCO, we plan to replace
  Endpoints object with [EndpointSlices](https://kubernetes.io/docs/concepts/services-networking/endpoint-slices/#motivation)
  to improve performance. This can be done once the `prometheus-operator` has
  [support](https://github.com/prometheus-operator/prometheus-operator/issues/3862)
  for EndpointSlices object.
  
#### Securing windows_exporter endpoint

* Since the windows-exporter is not running as a [pod](#justification), the
  endpoint is not secure. The reason for this is when running inside a pod, we
  can use CA signer for providing TLS cert/key to the service for authentication.
  We plan to secure the endpoint by using the kubernetes apiserver's [DelegatingAuthenticationOptions](https://github.com/kubernetes/apiserver/blob/8d97c871d91c75b81b8b4c438f4dd1eaa7f35052/pkg/server/options/authentication.go#L172)
  option to verify a client certificate with the CA bundle contents available in kube-system namespace.
  This will ensure that the metrics Endpoint will be able to authenticate the requests.

#### Telemetry Rules

* We plan to ensure that for [telemetry rules](https://docs.openshift.com/container-platform/4.7/support/remote_health_monitoring/showing-data-collected-by-remote-health-monitoring.html#showing-data-collected-from-the-cluster_showing-data-collected-by-remote-health-monitoring)
  also use metrics from Windows. This can be done by renaming the Windows
  metrics to align with metrics used in telemetry rules. For e.g.
  `memory_usage_bytes:sum` rule uses `node_memory_MemTotal_bytes` that is
  defined in the Windows rules. We also need to test if the existing telemetry
  rules need to be updated similar to console queries, if they have Linux
  specific queries. For e.g rules with `job=node-exporter` attribute.

### Test Plan

The current tests ensure that WMCO checks if :
* The operator namespace, `openshift-windows-machine-config-operator`, uses
  `openshift.io/cluster-monitoring=true` label.
* Service, endpoints and Service Monitor objects are created as expected.
* Prometheus is able to collect data from  windows_exporter.
* Custom Prometheus rules return Windows data.

The test plan for [future implementation](#future-plans)
will use existing tests to test creation of windows_exporter service and
metrics Service, Endpoints and Service Monitor objects. WMCO will also be
responsible for testing Prometheus rules created for Windows. We also
plan to add tests in console repo, that test the common recording rules and
ensure that they return results for Windows.

### Graduation Criteria

We are going directly to GA as this is a basic feature for Windows monitoring.

#### Dev Preview -> Tech Preview
None

#### Tech Preview -> GA
None

#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy

* WMCO is responsible for upgrading [windows_exporter](https://github.com/prometheus-community/windows_exporter/tags)
  binary to the latest release. Downgrades are [not supported](https://github.com/operator-framework/operator-lifecycle-manager/issues/1177)
  by OLM.
  
### Version Skew Strategy

* We plan to maintain parity with the upstream [windows_exporter](https://github.com/prometheus-community/windows_exporter)

## Implementation History

v1: Initial Proposal

## Drawbacks

Running windows_exporter as a Windows service instead of running as a DaemonSet
pod makes it hard for the Prometheus to monitor Windows nodes. The
limitation of not able to run windows_exporter on Windows nodes as a pod is
because of support reasons as mentioned in the [WMCO_enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/windows-containers/windows-machine-config-operator.md#justification).

## Alternatives

None known.