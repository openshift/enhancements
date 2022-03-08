---
title: worker-latency-profile
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
creation-date: 2021-12-07
last-updated: 2021-12-07
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

To make sure the Openshift cluster keeps running optimally where network latency between the control plane and the worker nodes may not always be at its best (e.g. Edge cases), we can tweak the frequency of the status updates done by the `Kubelet` and the corresponding reaction times of the `Kube Controller Manager` and `Kube API Server`.

## Motivation

To accommodate the higher network latency (such as Edge cases) cluster may experience, we need to adjust how frequently the `Kubelet` updates its status and how components such as `Kube Controller Manager` and `Kube API Server`react to those status updates.

The main motivation of this enhancement is to allow setting relevant arguments for these critical components in a more controlled manner instead of letting the users directly modify them manually which will make the cluster unsupported. This will also eliminate any room for manual errors that could lead to unscheduled downtime.

### Goals

* Enable Openshift cluster to fine tune the reliability in medium to high latency scenarios by specifying simple `WorkerLatencyProfile` at day-0 as well as day-2.

### Non-Goals

* Modify the existing `Kubelet`, `Kube Controller Manager` or `Kube API Server` code in any way.
* Allow any kind of different latency scenarios between masters

### User Stories

* User wants to fine tune the cluster reliability for their node latency scenario while making sure they are protected from setting parameters that could potentially break the cluster.

## Proposal

* The option to set `WorkerLatencyProfile` will have to reside in a centralized location. [Recently proposed Node object](https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/mco-cgroupsv2-support.md#api-extensions) in `config/v1/types_node.go` would be the ideal place to define API extensions necessary for `WorkerLatencyProfile`.
* [Machine Config Operator (MCO)](https://github.com/openshift/machine-config-operator) sets the appropriate value of the `Kubelet` flag `--node-status-update-frequency`
* [Kubernetes Controller Manager operator (KCMO)](https://github.com/openshift/cluster-kube-controller-manager-operator) sets the appropriate value of the `Kube Controller Manager` flag `--node-monitor-grace-period`
* [Kubernetes API Server Operator (KASO)](https://github.com/openshift/cluster-kube-apiserver-operator) sets the appropriate value of the `Kube API Server` flags `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds`

## Design Details

### API Extensions

The [existing proposed node object](https://github.com/openshift/enhancements/blob/master/enhancements/machine-config/mco-cgroupsv2-support.md#api-extensions) needs to be updated to include `NodeStatus` to track the progress of `WorkerLatencyProfile` rollout.

```go
type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// +required
	Spec NodeSpec `json:"spec"`

  // status holds observed values.
	// +optional
	Status NodeStatus `json:"status"`
}

type NodeSpec struct {
  WorkerLatencyProfile WorkerLatencyProfileType `json:"workerLatencyProfile,omitempty"`

  // an eventual additional option might be crun in the future. This explains
  //   why a new struct may be necessary
  // CgroupMode CgroupMode `json:"cgroupMode,omitempty"`
  //
  // CrunEnabled bool ...
}

type NodeStatus struct {
  // WorkerLatencyProfileStatus provides the current status of WorkerLatencyProfile
  WorkerLatencyProfileStatus WorkerLatencyProfileStatus `json:"workerLatencyProfileStatus,omitempty"`
}

type WorkerLatencyProfileType string

const (
    // Medium Kubelet Update Frequency (heart-beat) and Average Reaction Time to unresponsive Node
    MediumUpdateAverageReaction WorkerLatencyProfileType = "MediumUpdateAverageReaction"

    // Low Kubelet Update Frequency (heart-beat) and Slow Reaction Time to unresponsive Node
    LowUpdateSlowReaction WorkerLatencyProfileType = "LowUpdateSlowReaction"

    // Default values of relavent Kubelet, Kube Controller Manager and Kube API Server
    Default WorkerLatencyProfileType = "Default"
)

// WorkerLatencyProfileStatus provides status information about the WorkerLatencyProfile rollout
type WorkerLatencyProfileStatus struct {
  // conditions describes the state of the WorkerLatencyProfile and related components
  // (Kubelet or Controller Manager or Kube API Server)
  // +patchMergeKey=type
  // +patchStrategy=merge
  // +optional
  Conditions []WorkerLatencyStatusCondition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

  // relatedObjects is a list of objects that are "interesting" or related to this WorkerLatencyProfile. e.g. KubeletConfig object used for updating Kubelet arguments
  // +optional
  RelatedObjects []ObjectReference `json:"relatedObjects,omitempty"`
}

// WorkerLatencyStatusConditionType is an aspect of WorkerLatencyProfile state.
type WorkerLatencyStatusConditionType string

const (
  // Progressing indicates that the updates to component (Kubelet or Controller
  // Manager or Kube API Server) is actively rolling out, propagating changes to the
  // respective arguments.
  WorkerLatencyProfileProgressing WorkerLatencyStatusConditionType = "Progressing"

  // Complete indicates whether the component (Kubelet or Controller Manager or Kube API Server)
  // is successfully updated the respective arguments.
  WorkerLatencyProfileComplete WorkerLatencyStatusConditionType = "Complete"

  // Degraded indicates that the component (Kubelet or Controller Manager or Kube API Server)
  // does not reach the state 'Complete' over a period of time
  // resulting in either a lower quality or absence of service.
  // If the component enters in this state, "Default" WorkerLatencyProfileType
  // rollout will be initiated to restore the respective default arguments of all
  // components.
  WorkerLatencyProfileDegraded WorkerLatencyStatusConditionType = "Degraded"
)

type WorkerLatencyStatusConditionOwner string

const (
  // Machine Config Operator will update condition status by setting this as owner
  MachineConfigOperator WorkerLatencyStatusConditionOwner = "MachineConfigOperator"

  // Kube Controller Manager Operator will update condition status  by setting this as owner
  KubeControllerManagerOperator WorkerLatencyStatusConditionOwner = "KubeControllerManagerOperator"

  // Kube API Server Operator will update condition status by setting this as owner
  KubeAPIServerOperator WorkerLatencyStatusConditionOwner = "KubeAPIServerOperator"
)

type WorkerLatencyStatusCondition struct {
	// Owner specifies the operator that is updating this condition
	// +kubebuilder:validation:Required
	// +required
	Owner WorkerLatencyStatusConditionOwner string `json:"owner"`

	// type specifies the aspect reported by this condition.
	// +kubebuilder:validation:Required
	// +required
	Type WorkerLatencyStatusConditionType `json:"type"`

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

### Operational Aspects of API Extensions


#### Default Update And Default Reaction
OpenShift ships with the following default configuration which works in most cases.

By default `Kubelet` updates it's status every 10 seconds (`--node-status-update-frequency`), while `Kube Controller Manager` checks the statuses of `Kubelet` every 5 seconds (`--node-monitor-period`).
Before considering the `Kubelet` unhealthy `Kube Controller Manager` will wait for 40 seconds (`--node-monitor-grace-period`) to hear from the `Kubelet`. Once the `Kubelet` is considered unhealthy the node is given `node.kubernetes.io/not-ready` or `node.kubernetes.io/unreachable` taints.
Pods with `NoExecute` taint will get executed as per [tolerationSeconds](https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/#taint-based-evictions), but pods without such taint will get evicted in 300 seconds (`--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` settings of the `Kube API Server`)

| Component               | Flag Name                                | Flag Value  |
| -----------             | -----------                              | ----------- |
| Kubelet                 | --node-status-update-frequency           |    10s      |
| Kube Controller Manager | --node-monitor-grace-period              |    40s      |
| Kube API Server         | --default-not-ready-toleration-seconds   |    300      |
| Kube API Server         | --default-unreachable-toleration-seconds |    300      |


#### Medium Update And Average Reaction

While the default configuration works in most cases, sometimes the worker nodes may find themselves in a network with slightly higher than usual latency.

In this scenario, we will reduce the frequency of `Kubelet` updates to every 20 seconds (`--node-status-update-frequency`) and change the node monitor grace period of the `Kube Controller Manager` to 2 minutes (`--node-monitor-grace-period`). `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of the Kube API Server will be set to 60 seconds.

`Kube Controller Manager` will wait for 2 minutes to consider unhealthy status for the node and since `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of the Kube API Server are set to 60 seconds the total time will be 3 minutes before eviction process starts.


| Component               | Flag Name                                | Flag Value  |
| -----------             | -----------                              | ----------- |
| Kubelet                 | --node-status-update-frequency           |    20s      |
| Kube Controller Manager | --node-monitor-grace-period              |    2m       |
| Kube API Server         | --default-not-ready-toleration-seconds   |    60       |
| Kube API Server         | --default-unreachable-toleration-seconds |    60       |

#### Low Update and Slow reaction

Worker nodes may find themselves in a network with extremely high latency and/or bad reliability.
In this scenario, we will reduce the frequency of `Kubelet` updates further to every 1 minute (`--node-status-update-frequency`) and change the node monitor grade period of the `Kube Controller Manager` to every 5 minutes (`--node-monitor-grace-period`). `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of the Kube API Server are set to 60 seconds.

`Kube Controller Manager` will wait for 5 minutes to consider unhealthy status for the node and since `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of the `Kube API Server` are set to 60 seconds the total time will be 6 minutes before eviction process starts.


| Component               | Flag Name                                | Flag Value  |
| -----------             | -----------                              | ----------- |
| Kubelet                 | --node-status-update-frequency           |    1m       |
| Kube Controller Manager | --node-monitor-grace-period              |    5m       |
| Kube API Server         | --default-not-ready-toleration-seconds   |    60       |
| Kube API Server         | --default-unreachable-toleration-seconds |    60       |


#### Failure Modes
1. In case of failure `WorkerLatencyProfileStatus` should point towards failed component(s)


### Cluster Stability Analysis

Let's try to understand the effect of applying various worker latency profiles on the overall stability of the cluster.

Kubelet will try to make [5 post attempts](https://github.com/kubernetes/kubernetes/blob/release-1.23/pkg/kubelet/kubelet.go#L129) to update it's status. `--node-status-update-frequency` determines the frequency of attemps to update status.

So, there will be 5 * `--node-status-update-frequency` attempts to set a status of node.

Meanwhile, `Kube Controller Manager` will consider node unhealthy after `--node-monitor-grace-period`. Finally, pods will then be rescheduled based on the Taint Based Eviction timers set on them individually, or the `Kube API Server's` global timers:`--default-not-ready-toleration-seconds` & `--default-unreachable-toleration-seconds.`

Here are various plausible worker latency profile transitions and the respective individual states the cluster _might_ go through while applying them.

#### Default -> MediumUpdateAverageReaction

| Kubelet Status Update Frequency | KCM Node Monitor Grade Period| KAS NotReady/Unreachable Toleration | Notes
| -----------             | -----------                              | ----------- | -----------  |
| 10s                 | 40s           |    300s       |   Default Profile (Initial state)
| 10s                 | 40s           |    60s       |    No major impact on cluster health
| 10s                 | 2m            |    300s       |  No major impact on cluster health
| 10s                 | 2m           |     60s       | No major impact on cluster health
| 20s                 | 2m           |     300s       | No major impact on cluster health
| 20s                 | 40s           |     60s       |  Kubelet will get only 2 chances to set the Status. **Higher chances of node incorrectly getting NotReady/Unreachable taint.**
| 20s                 | 40s           |     300s       | Kubelet will get only 2 chances to set the Status. **Higher chances of node incorrectly getting NotReady/Unreachable taint.**
| 20s                 | 2m           |     60s       | MediumUpdateAverageReaction Profile (Final state)


#### Default -> LowUpdateSlowReaction

Because of the extremely high probability of node incorrectly getting NotReady/Unreachable taint, this transition is only allowed at day-0.

| Kubelet Status Update Frequency | KCM Node Monitor Grade Period| KAS NotReady/Unreachable Toleration | Notes
| -----------             | -----------                              | ----------- | -----------  |
| 10s                 | 40s           |    300s       |   Default Profile (Initial state)
| 10s                 | 40s           |    60s       |    No major impact on cluster health
| 10s                 | 5m            |    300s       |  No major impact on cluster health
| 10s                 | 5m           |     60s       | No major impact on cluster health
| 1m                 | 5m           |     300s       | No major impact on cluster health
| 1m                 | 40s           |     60s       |  Kubelet will NOT get any chance to set the Status. **Node will incorrectly get NotReady/Unreachable taint.**
| 1m                 | 40s           |     300s       | Kubelet will NOT get any chance to set the Status. **Node will incorrectly get NotReady/Unreachable taint.**
| 1m                 | 5m           |     60s       | LowUpdateSlowReaction Profile (Final state)

For day-2 operation this transtion will result in **Default -> MediumUpdateAverageReaction -> LowUpdateSlowReaction** to minimize the impact on the stability of the cluster.


#### MediumUpdateAverageReaction -> Default
| Kubelet Status Update Frequency | KCM Node Monitor Grade Period| KAS NotReady/Unreachable Toleration | Notes |
| -----------             | -----------                              | ----------- | -----------                              |
| 20s                 | 2m           |     60s       | MediumUpdateAverageReaction Profile (Initial state)
| 20s                 | 2m           |     300s       | No major impact on cluster health
| 20s                 | 40s           |     60s       |  Kubelet will get only 2 chances to set the Status. **Higher chances of node incorrectly getting NotReady/Unreachable taint.**
| 20s                 | 40s           |     300s       |  Kubelet will get only 2 chances to set the Status. **Higher chances of node incorrectly getting NotReady/Unreachable taint.**
| 10s                 | 40s           |    60s       | No major impact on cluster health
| 10s                 | 2m            |    300s       | No major impact on cluster health
| 10s                 | 2m           |     60s       | No major impact on cluster health
| 10s                 | 40s           |    300s       | Default Profile (Final state)

#### MediumUpdateAverageReaction -> LowUpdateSlowReaction
| Kubelet Status Update Frequency | KCM Node Monitor Grade Period| KAS NotReady/Unreachable Toleration | Notes
| -----------             | -----------                              | ----------- | -----------                              |
| 20s                 | 2m           |     60s       | MediumUpdateAverageReaction Profile (Initial state)
| 20s                 | 5m           |     60s       | No major impact on cluster health
| 1m                 | 2m           |     60s       | Kubelet will get only 2 chances to set the Status. **Higher chances of node incorrectly getting NotReady/Unreachable taint.**
| 1m                 | 5m           |    60s       | LowUpdateSlowReaction (Final state)


#### LowUpdateSlowReaction -> MediumUpdateAverageReaction
| Kubelet Status Update Frequency | KCM Node Monitor Grade Period| KAS NotReady/Unreachable Toleration | Notes
| -----------             | -----------                              | ----------- | ----------- |
| 1m                 | 5m           |    60s       | LowUpdateSlowReaction (Initial state)
| 1m                 | 2m           |     60s       | Kubelet will get only 2 chances to set the Status. **Higher chances of node incorrectly getting NotReady/Unreachable taint.**
| 20s                 | 5m           |     60s       | No major impact on cluster health
| 20s                 | 2m           |     60s       | MediumUpdateAverageReaction Profile (Final state)


#### LowUpdateSlowReaction -> Default

Because of the extremely high probability of node incorrectly getting NotReady/Unreachable taint, this transition is only allowed at day-0.

| Kubelet Status Update Frequency | KCM Node Monitor Grade Period| KAS NotReady/Unreachable Toleration | Notes
| -----------             | -----------                              | ----------- | -----------  |
| 1m                 | 5m           |     60s       | LowUpdateSlowReaction Profile (Initial state)
| 1m                 | 5m           |     300s       | No major impact on cluster health
| 1m                 | 40s           |     60s       |  Kubelet will NOT get any chance to set the Status. **Node will incorrectly get NotReady/Unreachable taint.**
| 1m                 | 40s           |     300s       | Kubelet will NOT get any chance to set the Status. **Node will incorrectly get NotReady/Unreachable taint.**
| 10s                 | 40s           |    60s       |    No major impact on cluster health
| 10s                 | 5m            |    300s       |  No major impact on cluster health
| 10s                 | 5m           |     60s       | No major impact on cluster health
| 10s                 | 40s           |    300s       |   Default Profile (Final state)

For day-2 operation this transtion will result in **LowUpdateSlowReaction -> MediumUpdateAverageReaction -> Default** to to minimize the impact on the stability of the cluster.

#### Support Procedures

### Test Plan

Testing should be thoroughly done at all levels, including unit, end-to-end, and integration.

### Graduation Criteria

#### Dev Preview -> Tech Preview

`WorkerLatencyProfile` will be dev preview on its initial release. Internal and customer
usage will be critical to gather information on bugs and enhancements to the
underlying subsystem.

Graduation requirements to Tech Preview are:

* No regressions after applying the `WorkerLatencyProfile` in bringing up `Kubelet`, `Kube Controller Manager` and `Kube API Server`
* No major imapct on the cluster stability while applying the `WorkerLatencyProfile`, especially nodes should not incorrectly get NotReady/Unreachable taint for prolonged period except for the described disruptions as described in the previous section during rollout.
* No performance issues - PSAP and QE teams will be asked to test their suites for regressions after applying different `WorkerLatencyProfiles`.

#### Tech Preview -> GA

With sufficient internal testing and customer feedback the feature will graduate
to Tech Preview.

Graduation requirements to GA:
* Internal stakeholders are using `WorkerLatencyProfile` without issue
* Tech Preview Graduation requirements are still good
* Add CI jobs with cluster set to `MediumUpdateAverageReaction` and `LowUpdateSlowReaction` profiles
* For CI jobs with cluster set to `MediumUpdateAverageReaction` and `LowUpdateSlowReaction` profiles pass percentage is similar or better than the OpenShift current values which is nothing but `Default` latency profile.


### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?

  It's important to highlight that if the cluster is using `MediumUpdateAverageReaction` or `LowUpdateSlowReaction` profile AND if any of the 3 operators (MCO, KCMO, and KASO) involved staying back at the version where they are not aware of the `WorkerLatencyProfile` then the respective component they manage will not get updated with relevant arguments.
  This will reduce the cluster stability and can lead to unexpected behavior.

- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?

  No.

- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

  No


#### Removing a deprecated feature

N/A

### Risks and Mitigations

1. Any bug in [MCO](https://github.com/openshift/machine-config-operator), [KCMO](https://github.com/openshift/cluster-kube-controller-manager-operator) or [KASO](https://github.com/openshift/cluster-kube-apiserver-operator) in setting the appropiate values for the respective flags might put the cluster at risk.

2. We are changing `--node-monitor-grace-period` argument of the `Kube Controller Manager` and `--default-not-ready-toleration-seconds` and `--default-unreachable-toleration-seconds` of arguments the `Kube API Server`.
Although we are targeting only worker nodes with this enhancement, both `Kube Controller Manager` and `Kube API Server` do not allow us to set those respective arguments only for the subset of the available nodes.
i.e. They are applied cluster wide including master nodes. We are only proposing `WorkerLatencyProfile` with this enhancement that slow down updates from the Kubelet on the worker nodes and the corresponding reaction time from the control plane.
So even though 100% reliable network connections amongst master nodes is assumed, when the user applies `WorkerLatencyProfile` there will be delays evicting the pods running on the master nodes than their usual default values.

### Upgrade / Downgrade Strategy

1. Since this feature is controlled using the `KubeletConfig` and `ConfigObserver`, upgrade/downgrade strategies applicable for the `KubeletConfig` and `ConfigObserver` are applicable here too.
2. During downgrade, if the cluster is using `MediumUpdateAverageReaction` or `LowUpdateSlowReaction` profile AND if it will get downgraded the version where it is not aware of the `WorkerLatencyProfile` then the operators MCO, KCMO, and KASO will end up eventually overriding the relevant values to their defaults.

## Drawbacks

## Alternatives

Since we don't want users to manually modify the arguments for the components involved, there isn't any viable alternative that could safely work.


## Implementation History


