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

# CRD Based CMO

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

* Currently, the monitoring stack is configured using a configmaps. In OpenShift though the best practice is to configure operators using custom resources.


## Motivation

* The specification is well known and to a degree self-documenting
* The APIServer will validate user resources based on our specifications, so users get immediate feedback on errors instead of having to check if their config was applied and check logs.
* Many users expect to interact with operators through a CRD
* Compatible with GitOps workflows. 
* We can add [cross]validation rules to CRD fields to avoid misconfigurations
* End users get a much faster feedback loop. No more applying the config and scanning logs if things don't look right. The API server will give immediate feedback
* Organizational users (such as ACM) can manage a single resource and observe its status


### Goals

- Replace configmaps with CRD
- Smooth transition for users

## Proposal

## Design Details

To initiate the process, let's establish a feature gate that will serve as the entry point for implementing a CRD configuration approach. This strategy enables us to make incremental advancements without the immediate burden of achieving complete feature equivalence with the config map. We can commence with a the basics and progressively incorporate additional functionalities as they develop.

One proposal for a minimal DoD was:
    - Feature gate
    - CRD Initial dev https://github.com/openshift/cluster-monitoring-operator/pull/2347/
        Add controller-gen logic to makefile
        Add API to pkg/apis/cmo/v1
        Add Generated CRD: config/crd/bases/example.com_clustermonitoringoperators.yaml
        Add example CustomResource: config/examples/clustermonitoringoperator.yaml
    - Client codegen: https://github.com/openshift/cluster-monitoring-operator/pull/2369
    - Reconcile logic: https://github.com/openshift/cluster-monitoring-operator/pull/2350
    - Add decoupling Confimgap / CustomResource:
        Controller logic is strongly dependant of *manifests.Config struct.
        


### Overview

- Replace confimgaps with CRD:
  cluster-monitoring-configmap
  user-workload-monitoring-config


### Migration path

    Feature gate. 
    Switch mecanishm?
    What to do if there are both CRD and confimgap?
    Precedence CRD over configmap?
    Should we compare CRD and configmap to check differences?

### Issues

- Decoupling Confimgap / CustomResource:
        Controller logic is strongly dependant of *manifests.Config struct.
        Should we translate CR into confimap?
    
- Correct name for apiVersion? In monitoring.coreos.com/v1 are all prometheus operator components, should we create a new one?

- Refactor and clean up operator.go client.go manifests.config 


### Transition to the user

- How the user could adopt CR instead of configmap.

### Example configuration


#### CRD

apiVersion: cmo.example.com/v1
kind: ClusterMonitoring
metadata:
  name: cluster
spec:
  telemeterClient:
    enabled: true
    nodeSelector:
      kubernetes.io/os: linux
    tolerations:
      - operator: Exists
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
