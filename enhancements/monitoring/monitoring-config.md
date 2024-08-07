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
* We can add [cross]validation rules to CRD fields to avoid misconfigurations
* End users get a much faster feedback loop. No more applying the config and scanning logs if things don't look right. The API server will give immediate feedback
* CRDs supports multiple actors managing the same resource which is a key property for the Observability service of Advanced Cluster Management.

### Goals

- Replace the existing ConfigMaps with CRDs.
- Automated and friction-less upgrade for users.

## Proposal

### Overview

- Replace confimgaps with CRD:
  cluster-monitoring-configmap
  user-workload-monitoring-config

  pkg/manifests/config.go

```
  type Config struct {
	Images                               *Images `json:"-"`
	RemoteWrite                          bool    `json:"-"`
	CollectionProfilesFeatureGateEnabled bool    `json:"-"`

	ClusterMonitoringConfiguration *ClusterMonitoringConfiguration `json:"-"`
	UserWorkloadConfiguration      *UserWorkloadConfiguration      `json:"-"`
}
```

### Migration path

- Feature gate. 
- Move each component to openshift/api/config
- Create two CRD's:
  * ClusterMonitoringConfiguration
  * UserWorkloadConfiguration
  The reason for having two different CRDs is to address the needs of two distinct roles who manage them. In managed OpenShift clusters (OSD/ROSA), the platform SREs are responsible for the cluster (platform) monitoring stack, while the platform users (e.g., customers) handle the user-defined monitoring stack.º
- Review/improve/modify every configmap component to create a compliant openshif/api type. Fix erros, clean old things, improve names and config.
- Switch mecanishm
- Both ConfigMap and CRD will coexist for a while until the transition is complete.
- CRD and ConfigMap will be merged into a single structure, this structure will config CMO
- Extensive testing will be required to validate the new behavior.
- If there are different fields in CRD and ConfigMap, the field in the ConfigMap will take precedence.
- Some fields in CRD and ConfigMap diverge due to API requirements. Helper functions will be needed to resolve this.
- The end user will be able to choose to use ConfigMap or CRD at any time, keeping in mind that the fields in the CRD will gradually expand.


### Issues

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
