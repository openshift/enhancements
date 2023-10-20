--- 
title: mco-state-reporting
authors:
  - "@cdoern"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@sinnykumari" # MCO
  - "@yuqi-zhang" # MCO
approvers:
  - "@sinnykumari"
  - "@yuqi-zhang"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@JoelSpeed"
creation-date: 2023-10-24
last-updated: 2023-10-24
tracking-link:
  - https://issues.redhat.com/browse/MCO-452
see-also:
replaces:
superseded-by:
---

# Track MCO State Related to Everyday Operations.

## Summary

This enhancement describes how MCO component progression should be aggregated into the machineconfiguration operator/v1 object. The goal here is to allow customers and the MCO team to decipher the our processes in a more verbose way, speeding up the debugging process and allowing for better customer engagement.

## Motivation

The MCO has "day to day" operations on its MachineConfigController, MachineConfigDaemon, and MachineConfigOperator pods. Tracking these daily progression statuses is just as important as the upgrade status. However, this information does not belong in the upgrade-progression datatype. Currently, only the MCO team has the skillset to dive into the code and figure out what "phase" of the MCO causes a non-upgrade related error. The goal of this work is to make that information accessible and understandable to an everyday audience.


### User Stories

* As a <role>, I want to <take some action> so that I can <accomplish a


### Goals

* Have API load be as minimal as possible but augment the proper objects as needed.
* Add State reporting to the operator machineconfiguration object, and add the MCO logic for updating these structs.
* aggregate as much MCO related data into easily accessible places as possible.

### Non-Goals


## Proposal

Create a Datatype for tracking Operator Component Progression in the MCO.

We would augment the existing MachineConfigurationStatus with a similar system as the one above but instead of tracking upgrades per pool it would track a variety of states per component of the MCO. This would all live in `oc get machineconfiguration` and be managed by the MCO. So rather than multiple CRs relating machineconfigpool nodes we would have multiple machineconfiguration CRs for each component of the MCO: the MCC, MCD and MCO pods. Unlike other operators which usually have a singular operator level CR, the MCO would benefit from 3 given the unique function of the MachineConfigController Daemon and Operator. Conglomerating them into a singular object would confuse those viewing the data. Separating the data into 3 objects would allow for ease of use given that the most common flow would be `oc get machineconfiguration` followed by `oc describe machineconfiguration/<resource_name>`. This flow ensures no resource gets forgotten about or confused for another. The types I imagine would look like this:

```go

package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +genclient:nonNamespaced
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MachineConfiguration provides information to configure an operator to manage Machine Configuration.
//
// Compatibility level 1: Stable within a major release for a minimum of 12 months or 3 minor releases (whichever is longer).
// +openshift:compatibility-gen:level=1
type MachineConfiguration struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata"`

	// spec is the specification of the desired behavior of the Machine Config Operator
	// +kubebuilder:validation:Required
	Spec MachineConfigurationSpec `json:"spec"`

	// status is the most recently observed status of the Machine Config Operator
	// +optional
	Status MachineConfigurationStatus `json:"status"`
}

type MachineConfigurationSpec struct {
	StaticPodOperatorSpec `json:",inline"`

	// Mode describes if we are talking about this object in cluster or during bootstrap
	// +kubebuilder:validation:Required
	// +required
	Mode MCOOperationMode `json:"mode"`
}

type MachineConfigurationComponent struct {
	// name represents the full name of this component
	Name string `json:"name"`
	// conditions is the most recent state reporting for each component
	// +listType=map
	// +listMapKey=type
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions" patchStrategy:"merge" patchMergeKey:"type"`
}

type MachineConfigurationStatus struct {
	StaticPodOperatorStatus `json:",inline"`
	// daemon describes the most recent progression of the MCD pods
	// +kubebuilder:validation:Required
	// +required
	Daemon MachineConfigurationComponent `json:"daemon"`
	// controller describes the most recent progression of the MCC pods
	// +kubebuilder:validation:Required
	// +required
	Controller MachineConfigurationComponent `json:"controller"`
	// operator describes the most recent progression of the MCO pod
	// +kubebuilder:validation:Required
	// +required
	Operator MachineConfigurationComponent `json:"operator"`
	// mostRecentError is populated if the State reports an error.
	MostRecentError string `json:"mostRecentError"`
	// health reports the overall status of the MCO given its Progress
	Health MachineConfigOperatorHealthEnum `json:"health"`
}

type MachineConfigOperatorHealthEnum string

const (
	// healthy describes an operator that is functioning properly
	Healthy MachineConfigOperatorHealthEnum = "Healthy"
	// unknown describes  an operator  who's health is unknown
	Unknown MachineConfigOperatorHealthEnum = "Unknown"
	// unhealthy describes an operator  that is not functioning properly
	UnHealthy MachineConfigOperatorHealthEnum = "Unhealthy"
)

// StateProgress is each possible state for the components of the MCO
type StateProgress string

const (
	// OperatorSync describes the overall process of syncing the operator regularly
	OperatorSync StateProgress = "OperatorSync"
	// OperatorSyncRenderConfig describes a machine that is creating or syncing its render config
	OperatorSyncRenderConfig StateProgress = "OperatorSyncRenderConfig"
	// OperatorSyncCustomResourceDefinitions describes the process of applying and verifying the CRDs related to the MCO
	OperatorSyncCustomResourceDefinitions StateProgress = "OperatorSyncCustomResourceDefinitions"
	// OperatorSyncConfigMaps describes the process of generating new data for and applying the configmaps the MCO manages
	OperatorSyncConfigMaps StateProgress = "OperatorSyncConfigmaps"
	// OperatorSyncMCP describes a machine that is syncing or applying its MachineConigPools
	OperatorSyncMCP StateProgress = "OperatorSyncMCP"
	// OperatorSyncMCD describes a machine that is syncing or applying Daemon related files.
	OperatorSyncMCD StateProgress = "OperatorSyncMCD"
	// OperatorSyncMCC describes a machine that is sycing or applying Controller related files
	OperatorSyncMCC StateProgress = "OperatorSyncMCC"
	// OperatorSyncMCS describes a machine that is syncing or applying server related files
	OperatorSyncMCS StateProgress = "OperatorSyncMCS"
	// OperatorSyncMCPRequired describes a machine in the process of ensuring and applying required MachineConfigPools
	OperatorSyncMCPRequired StateProgress = "OperatorSyncMCPRequired"
	// OperatorSyncKubeletConfig describes a machine that is syncing its KubeletConfig
	OperatorSyncKubeletConfig StateProgress = "OperatorSyncKubeletConfig"
	// MCCSync describes the process of syncing the MCC regularly
	MCCSync StateProgress = "SyncMCC"
	// MCCSyncMachineConfigPool describes the process of modifying a machineconfigpool in the MCC
	MCCSyncMachineConfigPool StateProgress = "SyncMCCMachineConfigPool"
	// MCCSyncRenderedMachineConfigs describes the process of generating and applying new machineconfigs
	MCCSyncRenderedMachineConfigs StateProgress = "SyncMCCRenderedMachineConfigs"
	// MCCSyncGeneratedKubeletConfig describes the process of generating and applying new kubelet machineconfigs
	MCCSyncGeneratedKubeletConfigs StateProgress = "SyncMCCGeneratedKubeletConfigs"
	// MCCSyncControllerConfig describes the process of filling out and applying a new controller config
	MCCSyncControllerConfig StateProgress = "SyncMCCControllerConfig"
	// MCCSyncContainerRuntimeConfig describes the process of generating and applying the container runtime machineconfigs
	MCCSyncContainerRuntimeConfig StateProgress = "SyncMCCContainerRuntimeConfig"
	// MCDSync describes the process of syncing the MCD regularly
	MCDSync StateProgress = "SyncMCD"
	// MCDSyncChangingStateAndReason indicates when the MCD changes the state on the node.
	MCDSyncChangingStateAndReason StateProgress = "SyncMCDChangingStateAndReason"
	// MCDSyncTriggeringUpdate indicates when the MCD tells the node it needs to update.
	MCDSyncTriggeringUpdate StateProgress = "SyncMCDTriggeringUpdate"
	// MetricSync describes the process of updating metrics and their related options
	MetricsSync StateProgress = "SyncMetrics"
	// BootstrapProgression describes processes occuring during the bootstrapping process
	BootstrapProgression StateProgress = "BootstrapProgression"
	// MachineStateErroed describes a machine that has errored during its proccess
	MachineStateErrored StateProgress = "Errored"
)

type MCOOperationMode string

const (
	// Bootstrap
	Bootstrap string = "bootstrap"
	// InCluster
	InCluster string = "inCluster"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// MachineConfigurationList is a collection of items
//
// Compatibility level 1: Stable within a major release for a minimum of 12 months or 3 minor releases (whichever is longer).
// +openshift:compatibility-gen:level=1
type MachineConfigurationList struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard list's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ListMeta `json:"metadata"`

	// Items contains the items
	Items []MachineConfiguration `json:"items"`
}


```

In these types the most important part is the MachineConfigurationStatus. Specifically the `MachineConfigurationComponent` living within the status. Each of these holds its own conditions for a component of the MCO. So, rather than having 3 seprate CRs for each component of the MCO, there is one CR that will eventually hold all user level options for the operator. This means there is one status, so seprating the status up into 3 sub-components is the clearest way to display what is going on in all pods the MCO owns.

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfiguration
metadata:
  name: machineconfigoperator
spec:
  mode: in-cluster
status:
  daemon:
    conditions:
  controller:
    conditions:
  operator:
    conditions:
```
Using this structure allows for ease of understanding both in `oc describe` as well as `oc get` where I will add additional printer columns that will outline the operator state like the following:

```console
$ oc get machineconfiguration
NAME                      OPERATORSYNC     MCCSYNC      MCDSYNC 
default                   True             False        False
```

as well as:

```console
$ oc get machineconfiguration -o wide
NAME                      OPERATORSYNC     MCCSYNC      MCDSYNC   OPERATORSYNCRENDERCONFIG OPERATORSYNCCRDS OPERATORSYNCCM OPERATORSYNCMCP OPERATORSYNCMCD....
default                   True             False        False     False                    True             False          False           False
```

Where each parent condition: OperatorSync MCCSync and MCDSync; have child conditions that only appear in -o wide. The output here is truncated for simplicity.