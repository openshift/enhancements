---
title: insights-config-api
authors:
  - "@tremes"
reviewers:
  - "@bparees"
  - "@deads2k"
  - "@mfojtik"
  - "@wking"
  - "@soltysh"
approvers:
  - "@bparees"
  - "@deads2k"
  - "@mfojtik"
  - "@soltysh"
api-approvers:
  - "@deads2k"
  - "@mfojtik"
  - "@soltysh"
creation-date: 2022-02-16
last-updated: 2022-05-27
status: implementable
tracking-link: 
  - "https://issues.redhat.com/browse/CCXDEV-6852"
see-also:
  - "https://issues.redhat.com/browse/CCXDEV-6852"
  - "https://issues.redhat.com/browse/CCX-195"
---

# Define and expose Insights configuration API

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [x] User-facing documentation is created/updated in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement introduces new Insights API resources. The new API will introduce new configuration options (including an option to force the data gathering as well as to disable it completely) as well as status reporting of the Insights operator. 

## Motivation

The main motivation for this new API is to allow users to refresh (or force) the gathering of the Insights data (see the [User stories](#user-stories) section for more details) as well as to disable the gathering completely. This new API will allows us to report various Insights status reports (including the Insights analysis report that is now exposed via Prometheus metrics).

### Goals

The goal of this enhancement is to propose a basic structure for a new Insights configuration API and define new configuration options. This includes following:

- Define a new configuration option providing a way to trigger on demand Insights gathering
- Define a new configuration option providing a way to include/exclude particular Insights data gatherers
- Provide Insights status & reporting in the status field in the new API

### Non-Goals

- Move the majority of the current configuration options to a new configuration API - Insights has quite a lot of configuration options
  and it does not make sense to expose everything in the new configuration API 
- Removing the original configuration via `support` secret

## Proposal

The proposal is to introduce new Insights `CustomResourceDefinition` types in the OpenShift API providing a way to configure the Insights operator and to report its status. 
The `support` secret configuration still takes precedence over the new config API. 

The proposal for the new `insights.config.openshift.io` resource is:

```go
type Insights struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds user settable values for configuration
	// +kubebuilder:validation:Required
	// +required
	Spec InsightsSpec `json:"spec"`
	// status holds observed values from the cluster. They may not be overridden.
	// +optional
	Status InsightsStatus `json:"status"`
}

type InsightsSpec struct {
	// GatherConfig spec attribute includes all the configuration options related to
	// gathering of the Insights archive and its uploading to the ingress.
	GatherConfig *GatherConfig `json:"gatherConfig,omitempty"`
}

type InsightsStatus struct {
}

type GatherConfig struct {
	// DataPolicy allows user to enable additional global obfuscation of the IP addresses and base domain
	// in the Insights archive data.
	// +kubebuilder:default=NoPolicy
	DataPolicy DataPolicy `json:"dataPolicy"`
	// ForceGatherReason enables user to force Insights data gathering by setting a new reason.
	// When there is some gathering in the progress then it is interrupted.
	// When all the gatherers are deactivated by the `DisabledGatherers`, nothing happens.
	ForceGatherReason string `json:"forceGatherReason"`
	// List of gatherers to be excluded from the gathering. All the gatherers can be disabled by providing "all" value.
	// If all the gatherers are disabled, the Insights operator does not gather any data.
	// The particular gatherers IDs can be found at https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md.
	// An example of disabling gatherers looks like this: `disabledGatherers: ["clusterconfig/machine_configs", "workloads/workload_info"]`
	DisabledGatherers []string `json:"disabledGatherers"`
}

const (
	// No data obfuscation
	NoPolicy DataPolicy = "NoPolicy"
	// IP addresses and cluster domain name is obfuscated
	IPsAndClusterDomainPolicy DataPolicy = "IPsAndClusterDomainPolicy"
)

type DataPolicy string
```
The proposal for the new `insights.operator.openshift.io` resource is:

```go
type Insights struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	// spec is the specification of the desired behavior of the Insights
	// +kubebuilder:validation:Required
	// +required
	Spec InsightsSpec `json:"spec"`

	// status is the most recently observed status of the Insights operator
	// +optional
	Status InsightsStatus `json:"status"`
}

type InsightsSpec struct {
	OperatorSpec `json:",inline"`
}

type InsightsStatus struct {
	OperatorStatus `json:",inline"`
	// GatheringStatus provides basic information about the last Insights gathering.
	GatheringStatus *GatheringStatus `json:"gatheringStatus,omitempty"`
	// ReportStatus provides general Insights analysis results.
	ReportStatus *ReportStatus `json:"reportStatus,omitempty"`
}

type GatheringStatus struct {
	// LastGatherTime is the last time when Insights gathering finished.
	LastGatherTime metav1.Time `json:"lastGatherTime,omitempty"`
	// LastGatherReason provides last known reason of gathering. This is helpful
	// especially when gathering was forced by user
	LastGatherReason string `json:"lastGatherReason,omitempty"`
	// StartGatherTime is the time when gathering started. The value is 0
	// when there is no gathering in progress.
	StartGatherTime metav1.Time `json:"startGatherTime,omitempty"`
	// List of active gatherers (and their statuses) in the last gathering.
	GathererStatuses []GathererStatus `json:"gathererStatuses,omitempty"`
}

type ReportStatus struct {
	// Number of active Insights healthchecks with low severity
	LowHealthChecksCount int `json:"low"`
	// Number of active Insights healthchecks with moderate severity
	ModerateHealthChecksCount int `json:"moderate"`
	// Number of active Insights healthchecks with important severity
	ImportantHealthChecksCount int `json:"important"`
	// Number of active Insights healthchecks with critical severity
	CriticalHealthChecksCount int `json:"critical"`
	// TotalCount is the count of all active Insights healthchecks
	TotalHealthChecksCount int `json:"total"`
}

type GathererStatus struct {
	// Name is the name of the gatherer.
	Name string `json:"name"`
	// GathererConditions provide details on the status of each gatherer.
	GathererConditions []GathererCondition `json:"conditions"`
	// DurationMillisecond represents the time spent gathering.
	DurationMillisecond int64 `json:"durationMillisecond"`
}

type GathererCondition struct {
	// Type of the gatherer condition
	Type GathererConditionType `json:"type"`
	// Status is last known status of the particular gatherer condition
	Status corev1.ConditionStatus `json:"status"`
	// Messages is an optional attribute that provides error and warning messages
	// from the gatherer
	Messages []string `json:"messages,omitempty"`
	// Last time the condition transit from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// Reason for the condition's last transition.
	Reason string `json:"reason,omitempty" protobuf:"bytes,5,opt,name=reason"`
}

// GathererConditionType is a valid value for GathererCondition.Type
type GathererConditionType string

const (
	// Gatherer was successful withnout any error reported
	Successful GathererConditionType = "Successful"
	// Gatherer was disabled by user
	Disabled GathererConditionType = "Disabled"
	// Gatherer failed to run
	Failed GathererConditionType = "Failed"
	// Gatherer was running, but there were some errors
	Warning GathererConditionType = "Warning"
)
```
### Workflow Description

See the [User Stories](#user-stories) and other sections.

### User Stories
#### Force gathering - refresh Insights status
As an OCP user/cluster-admin I would like to be able to trigger on demand gathering (there were some active Insights health checks in the cluster, which should be fixed and I want to refresh the Insights status). Steps:
- the user will set a new value of `ForceGatherReason`
- if all the gatherers are disabled (`disabledGatherers: ["all"]`), nothing happens
- if there is some gathering in progress (`StartGatherTime` is not nil in the `GatheringStatus` section) then it will be interrupted and the new gathering will be triggered
- if there is no gathering in progress (`StartGatherTime` is nil in the `GatheringStatus` section) then the new gathering will be triggered almost immediately
- user can check the `StartGatherTime` or the `LastGatheringTime` attribute in the `GatheringStatus` section to check if the new gathering already finished
- when the gathering is finished then the `LastGatherReason` and `LastGatherTime` in the `GatheringStatus` are updated

#### Disable Insights gathering completely
As an OCP user/cluster-admin I would like to disable Insights gathering completely. Steps:
- Set the `DisabledGatherers` to `["all"]` and the gathering will be turned off - i.e no new data will be gathered

#### Exposing latest Insights status
As an OCP user/cluster-admin I would like to see the latest Insights status. The status is represented by the `InsightsStatus` and it is only updated by the operator.

#### Excluding particular gatherers
As an OCP user/cluster-admin I would like to exclude particular gatherers. Steps:
- Set the `DisabledGatherers` with the list of gatherers to be excluded/disabled - e.g `DisabledGatherers: ["clusterconfig/config_maps", "clusterconfig/node_logs"]` 

### API Extensions

This is adding new Insights resource in the `config.openshift.io/v1` and in the `operator.openshift.io/v1` API resources. The proposal of the structure of the new API is mentioned in the [Proposal](#Proposal) section.

### Risks and Mitigations

Risk: Insights operator is not able to read the new config API.

Mitigation: The Insights operator is marked as degraded in the corresponding clusteroperator condition.

Risk: Somebody/something is repeatedly updating the `ForceGatherReason` attribute and thus forcing gathering. There is a risk that 
the cluster would upload data too often, but this assumes that the "repeating interval" allows uploading (if it's forced too quickly then there will be no upload, because the gathering is not over).

Mitigation: Maybe we should introduce some limit on the number of forced gatherings within 1 hour, for example. 

## Design Details

### On demand data gathering

The on demand data gathering can happen almost immediately when there is no active gathering in the progress, but when there is some in progress 
then it will be interrupted and the new gathering will be triggered. 

The forced gathering is defined as follows: 
- force it - e.g using a `ForceGatherReason` attribute where the user would provide some string as reason (mentioned in the [Proposal](#proposal)). 
  - pros
    - providing a reason (e.g “Refresh Insights”) is simpler for configuration than providing a time
    - gathering would be triggered almost immediately
    - there is already similar precedent with `ForceRedeploymentReason` when redeploying static pods
  - cons
    - need to be able to interrupt gathering (if any in progress)

### Open Questions

- ~~Can the API define default values? How does it work or how is it implemented?~~ This is done by using `// +kubebuilder:default=`
- ~~Should the new time of forced gathering (if any defined by user) set a new time for the existing gathering interval? If I force gathering at time A, will the new automatic gathering be in time A + interval?~~ - yes. It makes sense to update the last known time of gathering so that the regular gathering does not happen immediately after the forced gathering. 
- Should the `ForceGatherReason` attribute have its own configuration (given the fact that it's transient config) or reuse the existing one? - For now we decided to use the existing `GatherConfig` configuration. 

### Test Plan

The new config API will require updates of the existing Insights operator testsuite. The testsuite will be extended to cover the new API and respective attributes. 
Following behavior will need to be tested:
- `support` secret config takes precedence over the new config API
- forced gathering works as expected
- disabling some gatherers works as expected and diasbling all will disable the gathering completely

### Graduation Criteria

The plan is to introduce the first version of the new API behind the `TechPreviewNoUpgrade` feature gate. We would like to
keep the original way of configuring the Insights operator (that is via `support` secret), because it is problematic to move every existing configuration option to the new configuration API and the existing `support` secret serves basically as a configuration API. 

#### Dev Preview -> Tech Preview

The original way of configuring the Insights is still available. If there is a conflict, in other words, if there is some configuration option defined in the `support` secret as well as in the new configuration API then the value from the `support` secret takes precedence. 

#### Tech Preview -> GA

The `TechPreviewNoUpgrade` feature gate requirement is removed. The behavior defined above in the [Dev Preview -> Tech Preview](#dev-preview---tech-preview) section is still true. 

Other than that we would like to:
- get some time to gather feedback and some user experience
- ensure stability
- do more e2e testing

#### Removing a deprecated feature

The existing configuration via `support` secret should be still available for backward compatibility reasons. The reason of the `support` secret existence is the option of using basic authentication for the Ingress service, which Insights operator uses as the upload endpoint. 

### Upgrade / Downgrade Strategy

No update of the configuration is required. The existing configuration in the `support` secret continues to work. When there is an existing `insights.config.openshift.io` kind in a cluster after upgrade, then the user can optionally use the new configuration API. 
There can be a conflict when a configuration option is defined in the `support` secret as well as in the configuration API. The configuration in the `support` secret takes precedence in this case. 
If you downgrade to a cluster without the Insights API then the `support` secret configuration will continue to work as before, or it must be created (and possibly configured accordingly)). 

### Version Skew Strategy

### Operational Aspects of API Extensions

This API is provided exclusively for the Insights operator and there will be no other component depending on this new API. Current configobserver in the Insights operator runs every 5 minutes to check the `support` secret values.
The new API attributes will have a watch (using an informer).

#### Failure Modes

As noted in the [Risk and Mitigations](#risks-and-mitigations) section, the only possible failure is probably when the Insights operator
cannot communicate with the config API.

#### Support Procedures

The new config API will require a new OCP documentation chapter describing the new way of the Insights configuration 
and the new way of the Insights status reporting.

## Implementation History

As noted in the [Graduation criteria](#graduation-criteria) section, the first API version will be behind the `TechPreviewNoUpgrade` feature gate. 

### Drawbacks

The original reason for having the config in the `support` secret was the basic authentication option to communicate with the Ingress service. 
Currently the preferred authentication option is to use the token provided in the `pull-secret` secret, 
but the basic authentication option might be still valid for some use cases (e.g testing with Ingress service in the staging environment) so we should preserve this option. This is described more in the [Graduation](#graduation-criteria) and [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy) sections.

## Alternatives

The alternative is to keep all the original configuration options in the `support` secret, but this is not very suitable for the new configuration options (`ForceGatherReason` and `DisabledGatherers`) introduced in this enhancement and it does not allow to expose the Insights status in the API.

The alternative to forcing the gathering is to schedule it. The alternative could look like following:

- schedule it - e.g using something like the `ScheduleGatheringAt` attribute where the user would provide some time in the future.
  - Pros 
    - no need to interrupt gathering in progress
    - we could probably use the `RateLimiting` queue and add a new gathering to the queue with the `AddAfter` method. Otherwise we would need to define some waiting time (when there’s already some gathering in progress) and right now it’s difficult to estimate an average gathering time since the gatherers run in parallel
  - Cons
    - user has to define the scheduling time correctly in the future
    - gathering doesn’t happen immediately