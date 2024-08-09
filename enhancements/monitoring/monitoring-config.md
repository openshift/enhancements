---
title: Configuration CRD for the Cluster Monitoring Operator
authors:
  - "@marioferh"
  - "@danielmellado"
  - "@jan--f"
  - "@moadz"
  - "simonpasquier"
reviewers:
  - "@jan--f"
  - "@moadz"
  - "simonpasquier"
approvers:
  - "@jan--f"
  - "@moadz"
  - "simonpasquier"
  - "@openshift/openshift-team-monitoring"
creation-date: 2024-04-26
last-updated: 2024-04-26
status: provisional
---

# CRD ClusterMonitoring

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

* Currently, the OCP monitoring stacks are configured using ConfigMaps (1 for platform monitoring and 1 for user-defined monitoring). In OpenShift though the best practice is to configure operators using custom resources.


## Motivation

* The specification is well known and to a degree self-documenting.
* The Kubernetes API Server validate custom resources based on their API specification, so users get immediate feedback on errors instead of checking the ClusterOperator object and the cluster monitoring operator's logs. Custom expression language (CEL) even allows to write complex validation logic, including cross-field tests.
* Many users expect to interact with operators through a CRD.
* Custom resources play better with GitOps workflows. 
* CRDs supports multiple actors managing the same resource which is a key property for the Observability service of Advanced Cluster Management.

### Goals

- Replace the existing ConfigMaps with CRDs.
- Automated and friction-less upgrade for users.

## Proposal

### Overview

Currently in CMO a config map provides a way to inject configuration data into pods. There are two configmaps for the different stacks:

    cluster-monitoring-configmap: Default platform monitoring components. A set of platform monitoring components are installed in the openshift-monitoring project by default during an OpenShift Container Platform installation. This provides monitoring for core cluster components including Kubernetes services. The default monitoring stack also enables remote health monitoring for clusters.

    user-workload-monitoring-config: Components for monitoring user-defined projects. After optionally enabling monitoring for user-defined projects, additional monitoring components are installed in the openshift-user-workload-monitoring project. This provides monitoring for user-defined projects. These components are illustrated in the User section in the following diagram.


Two distinct CRDs are necessary because they are managed by different personas with specific roles and responsibilities:

    - UWM admins: manage the configuration of the UWM components (edit permissions on the openshift-user-workload-monitoring/user-workload-monitoring-config configmap).
    - Cluster admins: manage the configuration of the Platform monitoring components.

In managed OpenShift clusters like OSD/ROSA, two separate CRDs are necessary because platform SREs manage the cluster's platform monitoring stack, while customers manage the user-defined monitoring stack. This separation ensures that each group maintains control over their specific monitoring configurations, reducing conflicts and enhancing system management.

[More info](https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/multi-tenant-alerting.md)


- Replace confimgaps with CRD:
  
```
  type Config struct {
	Images                               *Images `json:"-"`
	RemoteWrite                          bool    `json:"-"`
	CollectionProfilesFeatureGateEnabled bool    `json:"-"`

	ClusterMonitoringConfiguration *ClusterMonitoringConfiguration `json:"-"`
	UserWorkloadConfiguration      *UserWorkloadConfiguration      `json:"-"`
}
```

We will strive to maintain the previous structure as much as possible while adapting it to OpenShift API conventions. 


```
type ClusterMonitoringConfiguration struct {
	AlertmanagerMainConfig *AlertmanagerMainConfig `json:"alertmanagerMain,omitempty"`
	UserWorkloadEnabled *bool `json:"enableUserWorkload,omitempty"`
	HTTPConfig *HTTPConfig `json:"http,omitempty"`
	K8sPrometheusAdapter *K8sPrometheusAdapter `json:"k8sPrometheusAdapter,omitempty"`
	MetricsServerConfig *MetricsServerConfig `json:"metricsServer,omitempty"`
	KubeStateMetricsConfig *KubeStateMetricsConfig `json:"kubeStateMetrics,omitempty"`
	PrometheusK8sConfig *PrometheusK8sConfig `json:"prometheusK8s,omitempty"`
	PrometheusOperatorConfig *PrometheusOperatorConfig `json:"prometheusOperator,omitempty"`
	PrometheusOperatorAdmissionWebhookConfig *PrometheusOperatorAdmissionWebhookConfig `json:"prometheusOperatorAdmissionWebhook,omitempty"`
	OpenShiftMetricsConfig *OpenShiftStateMetricsConfig `json:"openshiftStateMetrics,omitempty"`
	TelemeterClientConfig *TelemeterClientConfig `json:"telemeterClient,omitempty"`
	ThanosQuerierConfig *ThanosQuerierConfig `json:"thanosQuerier,omitempty"`
	NodeExporterConfig NodeExporterConfig `json:"nodeExporter,omitempty"`
	MonitoringPluginConfig *MonitoringPluginConfig `json:"monitoringPlugin,omitempty"`
}
```


Each component within the ConfigMap will be migrated to the OpenShift API in separate PRs. This approach allows for a thorough review, improvement, and modification of each ConfigMap component to ensure it aligns with OpenShift API standards. As part of this process, types will be modified, outdated elements will be removed and names and configurations will be refined.


### Migration path


- Switch mecanishm
- Both ConfigMap and CRD will coexist for a while until the transition is complete.
- CRD and ConfigMap will be merged into a single structure, this structure will config CMO
- Extensive testing will be required to validate the new behavior.
- If there are different fields in CRD and ConfigMap, the field in the ConfigMap will take precedence.
- Some fields in CRD and ConfigMap diverge due to API requirements. Helper functions will be needed to resolve this.
- The end user will be able to choose to use ConfigMap or CRD at any time, keeping in mind that the fields in the CRD will gradually expand.


### Open questions

- Merge CRD and configmap could be error prone and will need extra test
- Change of types and names and how handle it when there are CRD and configmap

### Transition to the user

- How the user could adopt CRD instead of configmap.

## Design Details

To initiate the process, let's establish a feature gate that will serve as the entry point for implementing a CRD configuration approach. This strategy enables us to make incremental advancements without the immediate burden of achieving complete feature equivalence with the config map. We can commence with a the basics and progressively incorporate additional functionalities as they develop.

One proposal for a minimal DoD was:
    - Feature gate in openshift/api
    - Api types moved to openshift/api
    - CRD Initial dev https://github.com/openshift/api/pull/1929
        Add controller-gen logic to makefile
        Add API to api/config/v1
        Add Generated CRD: config/v1/zz_generated.crd-manifests/0000_10_config-operator_01_clustermonitoring.crd.yaml
        Add example CustomResource: config/examples/clustermonitoringoperator.yaml
    - Client codegen 
    - Reconcile logic: https://github.com/openshift/cluster-monitoring-operator/pull/2350
    - Add decoupling Confimgap / CustomResource:
        Controller logic is strongly dependant of *manifests.Config struct.
        

### Example configuration


#### current configmap

```
apiVersion: v1
kind: ConfigMap
metadata:
  name: cluster-monitoring-config
  namespace: openshift-monitoring
data:
  config.yaml: |
    telemeterClient:
      enabled: false
    prometheusK8s:
      volumeClaimTemplate:
        metadata:
          name: prometheus-data
          annotations:
            openshift.io/cluster-monitoring-drop-pvc: "yes"
        spec:
          resources:
            requests:
              storage: 20Gi

```

### CRD

```
apiVersion: clustermonitoring.config.openshift.io
kind: ClusterMonitoring
metadata:
  name: cluster
  namespace: openshift-monitoring
spec:
  prometheusK8s:
    volumeClaimTemplate:
      metadata:
        name: prometheus-data
        annotations:
          openshift.io/cluster-monitoring-drop-pvc: "yes"
      spec:
        resources:
          requests:
            storage: 20G`
```


### Test Plan

- Unit tests for the feature
- e2e tests covering the feature

### Graduation Criteria

From Tech Preview to GA

#### Tech Preview -> GA

- Ensure feature parity with OpenShift SDN egress router

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History
