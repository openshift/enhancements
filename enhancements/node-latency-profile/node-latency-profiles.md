---
title: worker-latency-profiles
authors:
  - "@harche"
reviewers:
  - "@rphillips"
  - "@sttts"
  - "@soltysh"
approvers:
  - "@rphillips"
  - "@sttts"
  - "@soltysh"
creation-date: 2021-09-28
last-updated: 2021-10-19
status: implementable
see-also:
  - https://github.com/kubernetes-sigs/kubespray/blob/master/docs/kubernetes-reliability.md
  - https://github.com/Azure/aks-engine/blob/master/docs/topics/clusterdefinitions.md#controllermanagerconfig
replaces:
superseded-by:
---

# WorkerLatencyProfile

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)


## Summary

To make sure Openshift cluster keeps running optimally under various circumstances where network latency, API Server latency, etcd latency, control plane load latency, etc. may not always be at their best (e.g. Edge cases), we can tweak the frequency of the status updates done by the `Kubelet` and the corresponding reaction times of the `Controller Manager` and `Kube API Server`.

## Motivation

In order to adjust to different latencies the cluster may experience, we need to adjust how frequently the `Kubelet` updates it's status and how do components such as `Controller Manager` and `Kube API Server`react to those status updates.

In it's more rudimentary form, one can simply edit the relavent arguments to the `Kubelet`, `Controller Manager` and `Kube API Server`to achieve the desired results.
Upstream `Kubespray` project has excellent [documentation](https://github.com/kubernetes-sigs/kubespray/blob/master/docs/kubernetes-reliability.md#recommendations-for-different-cases) on relavent arguments to each of those components in different scenarios.

But the main motivation of this enhancement is to allow this updates to these critical components in a more controlled manner. Instead of letting the users directly modify those arguments, we want to make sure it happens systematically and methodically. This will eliminate any room for manual errors and would not lead to an unscheduled downtime.

### Goals

* Enable Openshift cluster to fine tune the reliability in various latency scenarios using simple `WorkerLatencyProfile`.

### Non-Goals

* Modify the existing `Kubelet`, `Controller Manager` or `Kube API Server` code in any way.
* Allow any kind of different latency scenarios between masters

### User Stories

* User wants to fine tune the cluster reliability for their node latency scenario while making sure they are protected from setting parameters that could potentially break the cluster.

## Proposal

* A new Custom Resource `WorkerLatencyProfile` should be created.
* This CR should be monitored by [MCO](https://github.com/openshift/machine-config-operator), [KCMO](https://github.com/openshift/cluster-kube-controller-manager-operator) and [CKAO](https://github.com/openshift/cluster-kube-apiserver-operator).
* `KubeletConfigController` of MCO will modify Kubelet specific arguments
* KCMO will modify `Controller Manager` specific arguments using `ConfigObserver`
* CKAO will modify `Kube API Server` specific arguments using `ConfigObserver`


### Graduation Criteria

#### Dev Preview -> Tech Preview
* Succeffully update the relavent arguments of the `Kubelet`, `Controller Manager` and `Kube API Server`
* End user documentation

#### Tech Preview -> GA
* More testing (upgrade, downgrade, scale)
* Optinally make it available during installation

#### Removing a deprecated feature

N/A

## Design Details

### API Extensions

We propose a new following [config v1 API](https://github.com/openshift/api/tree/master/config/v1) which will be consumed by the `WorkerLatencyProfile`,


```go
type WorkerLatencyProfile struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds user settable values for configuration
	// +kubebuilder:validation:Required
	// +optional
	Spec WorkerLatencyProfileSpec `json:"spec"`
	// status holds observed values from the cluster. They may not be overridden.
	// +optional
	Status WorkerLatencyProfileStatus `json:"status"`
}
```

Please refer to the section [WorkerLatencyProfile Scenarios](#workerLatencyProfile-scenarios) for more details on different scenarios.
```go
type WorkerLatencyProfileType string

const (
    // Medium Update and Average Reaction
    MUAR WorkerLatencyProfileType = "MediumUpdateAndAverageReaction"

    // Low Update and Slow Reaction
    LUSR WorkerLatencyProfileType = "LowUpdateAndSlowReaction"

    // Default values of relavent Kubelet and Controller Manager
    Default WorkerLatencyProfileType = "Default"
)
```

```go
type WorkerLatencyProfileSpec struct {
  // ProfileType determins the type of WorkerLatencyProfile to set on the cluster.
  // Only follow values "FastUpdateAndFastReaction", "MediumUpdateAndAverageReaction"
  // "LowUpdateAndSlowReaction" or "Default" are allowed. If the user does not explicitly
  // set the value, "Default" will be automatically assigned.
  ProfileType WorkerLatencyProfileType `json:"WorkerLatencyProfileType"`
}
```
```go
// WorkerLatencyProfileStatus provides information about the status of the
// WorkerLatencyProfile rollout
type WorkerLatencyProfileStatus struct {
  // conditions describes the state of the WorkerLatencyProfile and related components
  // (Kubelet or Controller Manager or Kube API Server)
  // +patchMergeKey=type
  // +patchStrategy=merge
  // +optional
  Conditions []NodeLatencyStatusCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

  // relatedObjects is a list of objects that are "interesting" or related to this WorkerLatencyProfile. e.g. KubeletConfig object used for updating Kubelet arguments
  // +optional
  RelatedObjects []ObjectReference `json:"relatedObjects,omitempty"`
}
```
```go
// NodeLatencyStatusConditionType is an aspect of WorkerLatencyProfile state.
type NodeLatencyStatusConditionType string

const (
  // Progressing indicates that the updates to component (Kubelet or Controller
  // Manager or Kube API Server) is actively rolling out, propagating changes to the
  // respective arguments.
  WorkerLatencyProfileProgressing NodeLatencyStatusConditionType = "Progressing"

  // Complete indicates whether the component (Kubelet or Controller Manager or Kube API Server)
  // is successfully updated the respective arguments.
  WorkerLatencyProfileComplete NodeLatencyStatusConditionType = "Complete"

  // Degraded indicates that the component (Kubelet or Controller Manager or Kube API Server)
  // does not reach the state 'Complete' over a period of time
  // resulting in either a lower quality or absence of service.
  // If the component enters in this state, "Default" WorkerLatencyProfileType
  // rollout will be initiated to restore the respective default arguments of all
  // components.
  WorkerLatencyProfileDegraded NodeLatencyStatusConditionType = "Degraded"
)
```
```go

type ConditionStatus string

// These are valid condition statuses. "ConditionTrue" means a resource is in the condition.
// "ConditionFalse" means a resource is not in the condition. "ConditionUnknown" means kubernetes
// can't decide if a resource is in the condition or not.
const (
	ConditionTrue    ConditionStatus = "True"
	ConditionFalse   ConditionStatus = "False"
	ConditionUnknown ConditionStatus = "Unknown"
)
```
```go

type NodeLatencyStatusCondition struct {
	// type specifies the aspect reported by this condition.
	// +kubebuilder:validation:Required
	// +required
	Type NodeLatencyStatusConditionType `json:"type"`

	// status of the condition, one of True, False, Unknown.
	// +kubebuilder:validation:Required
	// +required
	Status ConditionStatus `json:"status"`

	// lastTransitionTime is the time of the last update to the current status property.
	// +kubebuilder:validation:Required
	// +required
	LastTransitionTime metav1.Time `json:"lastTransitionTime"`

	// reason is the CamelCase reason for the condition's current status.
	// +optional
	Reason string `json:"reason,omitempty"`

	// message provides additional information about the current condition.
	// This is only to be consumed by humans.  It may contain Line Feed
	// characters (U+000A), which should be rendered as new lines.
	// +optional
	Message string `json:"message,omitempty"`
}
```
```go
// ObjectReference contains enough information to let you inspect or modify the referred object.
type ObjectReference struct {
	// group of the referent.
	// +kubebuilder:validation:Required
	// +required
	Group string `json:"group"`
	// resource of the referent.
	// +kubebuilder:validation:Required
	// +required
	Resource string `json:"resource"`
	// namespace of the referent.
	// +optional
	Namespace string `json:"namespace,omitempty"`
	// name of the referent.
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name"`
}

```
#### WorkerLatencyProfile Custom Resource


```yaml
kind: WorkerLatencyProfile
metadata:
  name: medium-update-and-average-reaction
spec:
  profileType: "MediumUpdateAndAverageReaction" # Possible values, "MediumUpdateAndAverageReaction", "LowUpdateAndSlowReaction" or "Default"
```

### Operational Aspects of API Extensions

Openshift always assumes low latency with 100% reliable network connections amongst master nodes. However, this may not be always true for the worker nodes (e.g Edge cases). In this section we will discuss how to keep the cluster healthy when the worker nodes have relatively high latency with not so reliable network connections to reach the master nodes.

When the worker nodes have slightly higher than usual latency or/and relatively worse network connection, we can tweak the frequency at which the `Kubelet` updates the status and the corresponding reaction time of the `Controller Manager` to avoid considering the node unhealthy/unreachable.
We will also need to tweak `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of the `Kube API Server` to make sure pod eviction happens in timely manner.

#### Default Update And Default Reaction
Openshift ships with following default configuration which works quite well in most cases.

By default `Kubelet` updates it's status every 10 seconds (`--node-status-update-frequency`), while `Controller Manager` checks the statuses of `Kubelet` every 5 seconds (`--node-monitor-period`).
Before considering the `Kubelet` unhealthy `Controller Manager` will wait for 40 seconds (`--node-monitor-grace-period`) to hear from the `Kubelet`. Once the `Kubelet` is considered unhealthy the node is given `node.kubernetes.io/not-ready` or `node.kubernetes.io/unreachable` taints.
Pods with `NoExecute` taint will get executed as per [tolerationSeconds](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/#taint-based-evictions), but pods without such taint will get evicted in 300 seconds (`--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` settings of the `Kube API Server`)

#### Medium Update And Average Reaction

While the default configuration works well in most cases, sometimes the worker nodes may find themselves in a network with slightly higher than usual latency and/or degraded reliability.

In this scenario, we will reduce the frequency of `Kubelet` updates to every 20 seconds (`--node-status-update-frequency`) and change the node monitor grade period of the `Controller Manager` to 2 minutes (`--node-monitor-grace-period`). `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of the Kube API Server are set to 60 seconds.

`Controller Manager` will wait for 2 minutes to consider unhealthy status for the node and since `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of the Kube API Server are set to 60 seconds the total time will be 3 minutes before eviction process starts.

#### Low Update and Slow reaction

Worker nodes may find themselves in a network with extremely high latency and/or bad reliability.
In this scenario, we will reduce the frequency of `Kubelet` updates further to every 1 minute (`--node-status-update-frequency`) and change the node monitor grade period of the `Controller Manager` to every 5 minutes (`--node-monitor-grace-period`). `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of the Kube API Server are set to 60 seconds.

`Controller Manager` will wait for 5 minutes to consider unhealthy status for the node and since `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of the Kube API Server are set to 60 seconds the total time will be 6 minutes before eviction process starts.


#### Coordination between the Kubelet, Controller Manager and Kube API Server

Since the updates to the `Kubelet`, `Controller Manager` and `Kube API Server` arguments cannot be simultaneous, we will have to take 3-phase approach.

MCO and KCMO will have necessary observers to consume `WorkerLatencyProfile`.

1. **Kubelet** -
MCO will initialize updates to the Kubelet as a first step by submitting a `KubeletConfig` with relavent value of `nodeStatusUpdateFrequency` which will be processed by existing `KubeletConfigController` within MCO.
MCO will observe the [status](https://github.com/openshift/machine-config-operator/blob/85f9f9583451cb1e4c25893598eb5d086fd7e4ba/pkg/apis/machineconfiguration.openshift.io/v1/types.go#L349) of this `KubeletConfig` and fill out `NodeLatencyStatusCondition` appropriately.

2. **Controller Manager** -
Once the Kubelet arguments are successfully updated, KCMO will start rolling out relavent updates to the arguments `--node-monitor-period` and/or `--node-monitor-grace-period` of the Controller Manager.
MCO will monitor [KubeControllerManager](https://docs.openshift.com/container-platform/4.9/rest_api/operator_apis/kubeapiserver-operator-openshift-io-v1.html) `spec.observedConfig.extendedArguments` to show `--node-monitor-period` and/or `--node-monitor-grace-period` with the values specific to the selected `WorkerLatencyProfile` and then wait for the `Progressing` condition on `status.conditions`.
Once the Controller Manager is successfully deployed with the updated arguments, MCO will update `NodeLatencyStatusCondition` appropriately to mark the completion of the update to the arguments of Controller Manager.

3. **Kube API Server** -
After the Controller Manager arguments have been updated, CKAO will start rolling out relevant updates to the arguments `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of the Kube API Server.
MCO will monitor [KubeAPIServer](https://docs.openshift.com/container-platform/4.9/rest_api/operator_apis/kubeapiserver-operator-openshift-io-v1.html)
`spec.observedConfig.apiServerArguments` to show `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` with the values specific to the selected `WorkerLatencyProfile` and then wait for the `Progressing` condition on `status.conditions`.
Once the Kube API Server is successfully deployed with the updated arguments, MCO will update `NodeLatencyStatusCondition` appropriately to mark the completion of the update to the arguments of Kube API Server.

#### Failure Modes
If the `WorkerLatencyProfile.status.conditions` shows `WorkerLatencyProfileDegraded` condition, then the rollout of the given `WorkerLatencyProfile` has failed. Failure at any stage of rolling out `WorkerLatencyProfile` should trigger a rollback by MCO to reset the arguments of Kubelet, Controller Manager and Kube API Server to their default corresponding values.
In the event that the rollback is unsuccessful, `WorkerLatencyProfile.status.conditions` should provide an indication of which component out of the Kubelet, Controller Manager and/or Kube API Server has failed and the corresponding team responsible for that component needs to handle it as a bug.
Having a failure in rolling out `WorkerLatencyProfile` and a failure in successfully rolling back to the default values will result in inconsistency in the cluster (e.g we could observe nodes gettling labelled `NotReady` when actually they could be `Ready`)

#### Support Procedures
If the `WorkerLatencyProfile.status.conditions` shows `WorkerLatencyProfileDegraded` condition then the rollout of the given `WorkerLatencyProfile` has failed. `WorkerLatencyProfile.status.conditions` should provide an indication of which component out of the Kubelet, Controller Manager and/or Kube API Server has failed.
If support wants to disable this feature, then they can delete `WorkerLatencyProfile` CRD.
### Test Plan


### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?

  This functionality only modifies the existing arguments of the `Kubelet`, `Controller Manager` and `Kube API Server`. As long as components keep the respective flags in place, version skew should not have any impact on this work.

- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?

  Yes, It does involve coordinating between the control plane and the kubelet. N-2 kubelet without this feature will still work because the kubelet argument we are using exists from beyond N-2.

- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

  No



### Risks and Mitigations

1. Any bug in the coordination between the `Kubelet`, `Controller Manager` and `Kube API Server` might put the cluster at risk.

2. If the rollback mechanism proposed in this enhancement also fails for some reason, then users can mitigate manually supplying `KubeletConfig` that could work with the `Controller Manager` and `Kube API Server`.

3. We are changing `--node-monitor-period` and/or `--node-monitor-grace-period` arguments of the Controller Manager and `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of arguments the Kube API Server.
Although we are targeting only worker nodes with this enhancement, both Controller Manager and Kube API Server do not allow us to set those respective arguments only for the subset of the available nodes.
i.e. They are applied cluster wide including master nodes. We are only proposing `WorkerLatencyProfile` with this enhancement that slow down updates from the Kubelet on the worker nodes and the corresponding reaction time from the control plane.
So even though 100% reliable network connections amongst master nodes is assumed, when the user applies `WorkerLatencyProfile` there will be delays evicting the pods running on the master nodes than their usual default values.

### Upgrade / Downgrade Strategy

Since this feature is controlled using the `KubeletConfig` and `ConfigObserver`, upgrade/downgrade strategies applicable for the `KubeletConfig` and `ConfigObserver` are applicable here too.

## Drawbacks

This solution is fairly complex due to the synchronization it requires between the control plane and the kubelet. Unfortunately, there isn't any way around this.

## Alternatives

Since we don't want users to manually modify the arguments for the components involved, there isn't any viable alternative that could safely work.

## Implementation History


