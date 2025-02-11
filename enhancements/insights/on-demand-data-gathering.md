---
title: on-demand-data-gathering
authors:
  - "@tremes"
reviewers: 
  - "@deads2k"
  - "@JoelSpeed"
approvers:
  - "@deads2k"
  - "@JoelSpeed"
api-approvers: 
  - "@JoelSpeed"
creation-date: 2022-11-04
last-updated: 2023-01-16
tracking-link: 
  - https://issues.redhat.com/browse/CCXDEV-8854
  - https://issues.redhat.com/browse/CCX-195
  - https://issues.redhat.com/browse/CCXDEV-9980
see-also:
  - "[Insights Config API EP](../insights/insights-config-api.md)"
---

# Introduce the option to run Insights data gathering on demand

This proposal discusses and suggests possible options for OpenShift users to run Insights data gathering on demand.  

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created/updated in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The Insights operator periodically gathers cluster data and sends it to Red Hat. The data is analyzed and the corresponding Insights analysis result is provided to the cluster. The OpenShift user can then see the number of active recommendations and go to the Insights advisor for
more information. Right now, there is no way for a user to run Insights data gathering on demand - e.g when some of the Insights recommendations have been resolved.  

## Motivation

The motivation is to allow users (cluster admins) to run Insights data gathering on demand and thus enable to refresh the last Insights status in the cluster. 

### User Stories
- As an OCP user/cluster-admin I would like to trigger on-demand gathering so that I can refresh the current Insights status of my cluster and I don't have to wait 2 hours or delete the Insights operator pod. (e.g there were some active Insights health checks in the cluster, which should be fixed and I want to refresh the Insights status)
- As an ACM (Advanced Cluster Manager) user/admin I would like to externally trigger on-demand gathering so that I can refresh the Insight status of my cluster (or multiple clusters I administrate)

### Goals

The goal is to provide OpenShift users/admins an easy way to run and trigger Insights data gathering on demand and refresh the Insights status of their cluster/s. 
The way of triggering the new data gathering should be simple enough to e.g create a new button in the ACM (advanced cluster manager) refreshing the Insights status of the given cluster. 

### Non-Goals

- Change data gather frequency.
- Change gathered data. 
- Change operator status reporting (including metrics). This should work as previously.

## Proposal

The enhancement proposal introduces a new Insights `CustomResourceDefinition` type in the OpenShift API that will serve as the definition and trigger to a new on-demand data gathering. 
This new CRD will build on and extend the existing `GatherConfig` (see the [Insights config API enhancement](../insights/insights-config-api.md)). The idea is that with each new CR instance of this resource, new data gathering will happen. 
The CR will be a clusterscoped resource. The clusterscoped resource eliminates problems with inactive CRs (because they were created in the wrong namespace) and also eliminates potential problem of potentially sensitive data being placed in the wrong namespace. 

The proposal for the new `datagather.insights.openshift.io` resource is:

```go
type DataGather struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds user settable values for configuration
	// +kubebuilder:validation:Required
	Spec DataGatherSpec `json:"spec"`
	// status holds observed values from the cluster. They may not be overridden.
	// +optional
	Status DataGatherStatus `json:"status"`
}

type DataGatherSpec struct {
	// dataPolicy allows user to enable additional global obfuscation of the IP addresses and base domain
	// in the Insights archive data. Valid values are "ClearText" and "ObfuscateNetworking".
	// When set to ClearText the data is not obfuscated.
	// When set to ObfuscateNetworking the IP addresses and the cluster domain name are obfuscated.
	// When omitted, this means no opinion and the platform is left to choose a reasonable default, which is subject to change over time.
	// The current default is ClearText.
	// +optional
	DataPolicy DataPolicy `json:"dataPolicy"`
	// gatherersConfig is a list of gatherers configurations.
	// The particular gatherers IDs can be found at https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md.
	// Run the following command to get the names of last active gatherers:
	// "oc get insightsoperators.operator.openshift.io cluster -o json | jq '.status.gatherStatus.gatherers[].name'"
	// +optional
	GatherersConfig []GathererConfig `json:"gatherersConfig"`
}

const (
	// No data obfuscation
	NoPolicy DataPolicy = "ClearText"
	// IP addresses and cluster domain name are obfuscated
	ObfuscateNetworking DataPolicy = "ObfuscateNetworking"
	// Data gathering is running
	Running DataGatherState = "Running"
	// Data gathering is completed
	Completed DataGatherState = "Completed"
	// Data gathering failed
	Failed DataGatherState = "Failed"
	// Data gathering is pending
	Pending DataGatherState = "Pending"
	// Gatherer state marked as disabled, which means that the gatherer will not run.
	Disabled GathererState = "Disabled"
	// Gatherer state marked as enabled, which means that the gatherer will run.
	Enabled GathererState = "Enabled"
)

// dataPolicy declares valid data policy types
// +kubebuilder:validation:Enum="";ClearText;ObfuscateNetworking
type DataPolicy string

// state declares valid gatherer state types.
// +kubebuilder:validation:Enum="";Enabled;Disabled
type GathererState string

// gathererConfig allows to configure specific gatherers
type GathererConfig struct {
	// name is the name of specific gatherer
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// state allows you to configure specific gatherer. Valid values are "Enabled", "Disabled" and omitted.
	// When omitted, this means no opinion and the platform is left to choose a reasonable default.
	// The current default is Enabled.
	// +optional
	State GathererState `json:"state"`
}

// dataGatherState declares valid gathering state types
// +kubebuilder:validation:Optional
// +kubebuilder:validation:Enum=Running;Completed;Failed;Pending
// +kubebuilder:validation:XValidation:rule="!(oldSelf == 'Running' && self == 'Pending') && !(oldSelf == 'Completed' && self == 'Pending') && !(oldSelf == 'Failed' && self == 'Pending') && !(oldSelf == 'Completed' && self == 'Running') && !(oldSelf == 'Failed' && self == 'Running')", message="state cannot be changed backwards"
type DataGatherState string

// +kubebuilder:validation:XValidation:rule="(!has(oldSelf.insightsRequestID) || has(self.insightsRequestID)) && (!has(oldSelf.startTime) || has(self.startTime)) && (!has(oldSelf.finishTime) || has(self.finishTime)) && (!has(oldSelf.dataGatherState) || has(self.dataGatherState))",message="cannot remove attributes from status"
// +kubebuilder:validation:Optional
type DataGatherStatus struct {
	// conditions provide details on the status of the gatherer job.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []metav1.Condition `json:"conditions" patchStrategy:"merge" patchMergeKey:"type"`
	// dataGatherState reflects the current state of the data gathering process.
	// +optional
	State DataGatherState `json:"dataGatherState,omitempty"`
	// gatherers is a list of active gatherers (and their statuses) in the last gathering.
	// +listType=map
	// +listMapKey=name
	// +optional
	Gatherers []GathererStatus `json:"gatherers,omitempty"`
	// startTime is the time when Insights data gathering started.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="startTime is immutable once set"
	// +optional
	StartTime metav1.Time `json:"startTime,omitempty"`
	// finishTime is the time when Insights data gathering finished.
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="finishTime is immutable once set"
	// +optional
	FinishTime metav1.Time `json:"finishTime,omitempty"`
	// relatedObjects is a list of resources which are useful when debugging or inspecting the data
	// gathering Pod
	// +optional
	RelatedObjects []ObjectReference `json:"relatedObjects,omitempty"`
	// insightsRequestID is an Insights request ID to track the status of the
	// Insights analysis (in console.redhat.com processing pipeline) for the corresponding Insights data archive.
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="insightsRequestID is immutable once set"
	// +kubebuilder:validation:Optional
	// +optional
	InsightsRequestID string `json:"insightsRequestID,omitempty"`
	// insightsReport provides general Insights analysis results.
	// When omitted, this means no data gathering has taken place yet or the
	// corresponding Insights analysis (identified by "insightsRequestID") is not available.
	// +optional
	InsightsReport InsightsReport `json:"insightsReport,omitempty"`
}

// gathererStatus represents information about a particular
// data gatherer.
type GathererStatus struct {
	// conditions provide details on the status of each gatherer.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=type
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Conditions []metav1.Condition `json:"conditions" patchStrategy:"merge" patchMergeKey:"type"`
	// name is the name of the gatherer.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=256
	// +kubebuilder:validation:MinLength=5
	Name string `json:"name"`
	// lastGatherDuration represents the time spent gathering.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Pattern="^([1-9][0-9]*(\\.[0-9]+)?(ns|us|µs|ms|s|m|h))+$"
	LastGatherDuration metav1.Duration `json:"lastGatherDuration"`
}

// insightsReport provides Insights health check report based on the most
// recently sent Insights data.
type InsightsReport struct {
	// downloadedAt is the time when the last Insights report was downloaded.
	// An empty value means that there has not been any Insights report downloaded yet and
	// it usually appears in disconnected clusters (or clusters when the Insights data gathering is disabled).
	// +optional
	DownloadedAt metav1.Time `json:"downloadedAt,omitempty"`
	// healthChecks provides basic information about active Insights health checks
	// in a cluster.
	// +listType=atomic
	// +optional
	HealthChecks []HealthCheck `json:"healthChecks,omitempty"`
	// uri provides the URL link from which the report was downloaded.
	// +kubebuilder:validation:Pattern=`^https:\/\/\S+`
	// +optional
	URI string `json:"uri,omitempty"`
}

// healthCheck represents an Insights health check attributes.
type HealthCheck struct {
	// description provides basic description of the healtcheck.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MaxLength=2048
	// +kubebuilder:validation:MinLength=10
	Description string `json:"description"`
	// totalRisk of the healthcheck. Indicator of the total risk posed
	// by the detected issue; combination of impact and likelihood. The values can be from 1 to 4,
	// and the higher the number, the more important the issue.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4
	TotalRisk int32 `json:"totalRisk"`
	// advisorURI provides the URL link to the Insights Advisor.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https:\/\/\S+`
	AdvisorURI string `json:"advisorURI"`
	// state determines what the current state of the health check is.
	// Health check is enabled by default and can be disabled
	// by the user in the Insights advisor user interface.
	// +kubebuilder:validation:Required
	State HealthCheckState `json:"state"`
}

// healthCheckState provides information about the status of the
// health check (for example, the health check may be marked as disabled by the user).
// +kubebuilder:validation:Enum:=Enabled;Disabled
type HealthCheckState string

const (
	// enabled marks the health check as enabled
	HealthCheckEnabled HealthCheckState = "Enabled"
	// disabled marks the health check as disabled
	HealthCheckDisabled HealthCheckState = "Disabled"
)

// ObjectReference contains enough information to let you inspect or modify the referred object.
type ObjectReference struct {
	// group is the API Group of the Resource.
	// Enter empty string for the core group.
	// This value should consist of only lowercase alphanumeric characters, hyphens and periods.
	// Example: "", "apps", "build.openshift.io", etc.
	// +kubebuilder:validation:Pattern:="^$|^[a-z0-9]([-a-z0-9]*[a-z0-9])?(.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$"
	// +kubebuilder:validation:Required
	Group string `json:"group"`
	// resource is the type that is being referenced.
	// It is normally the plural form of the resource kind in lowercase.
	// This value should consist of only lowercase alphanumeric characters and hyphens.
	// Example: "deployments", "deploymentconfigs", "pods", etc.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern:="^[a-z0-9]([-a-z0-9]*[a-z0-9])?$"
	Resource string `json:"resource"`
	// name of the referent.
	// +kubebuilder:validation:Required
	Name string `json:"name"`
	// namespace of the referent.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// DataGatherList is a collection of items
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
type DataGatherList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`
	Items           []DataGather `json:"items"`
}
```

### Workflow Description

1. User/cluster admin creates a new CR instance of the `datagather.insights.openshift.io` CRD. 
2. The Insights operator is informed (using informers) about the new CR instance and spawns a new data gathering in a new container. 
3. The new container process gathers Insights data based on the configuration provided in the CR instance and is responsible for uploading the gathered Insights archive and checking that the archive was processed correctly in the console.redhat.com (the API documentation is available at https://console.redhat.com/docs/api/insights-results-aggregator/v2). 
4. The original Insights operator's pod is responsible for downloading the last known Insights analysis report (from console.redhat.com)
   based on latest successfully finished gathering job and updating the corresponding values in the already existing `insightsoperator.operator.openshift.io` resource.

This scenario brings few questions and things to be discussed. Please see the following sections [Implementation details](#implementation-detailsnotesconstraints), [Risk mitigations](#risks-and-mitigations) and [Open questions](#open-questions)

### API Extensions

This is adding new Insights CRD in the `insights.openshift.io/v1alpha1` . The proposal of the structure of the new CRD is mentioned in the [Proposal](#Proposal) section.

### Implementation Details/Notes/Constraints

#### Current behavior
The Insights operator currently runs and gathers the data in a single container. 
Uploading of new archives, pruning old archives as well as downloading the last Insights analysis report (this is always per archive) is implemented in this one container/binary. 
When there is a successful upload (to the console.redhat.com) of the archive then there is some short delay (waiting for the processing in console.redhat.com pipelines - see the [API documentation](https://console.redhat.com/docs/api/insights-results-aggregator/v2)) and then there is a request to get the last Insights analysis report per given cluster. 
Finally the status section of the `insightsoperator.operator.openshift.io` CR is updated. Note that there is a retry logic in case of failed upload and after some time (and number of attempts) the Insights operator is marked as degraded.

#### Updates required to allow on-demand data gathering
The plan is to refactor the existing periodic data gathering to run in a separate Pod/Job. The existing Insights operator pod will still exist, but it would periodically do the following:
- check the `insightsdatagather.config.openshift.io` CR to read current user configuration
- create a new `datagather.insights.openshift.io` CR based on the values from the above
- spawn a new `job` to gather the Insights data, upload it to `console.redhat.com` and check that the archive was processed in console.redhat.com (this requires a new API endpoint to be implemented in https://console.redhat.com/docs/api/insights-results-aggregator/v2).
  The new API endpoint will be identified by the cluster ID and will require `insightsRequestID` parameter. The endpoint will be queried with some delay (to allow processing)  
- status attribute of the `datagather.insights.openshift.io` is updated within the job execution
- wait for the job completion and if the processing part was successful then download the last Insights analysis report
- check that the Insights analysis report ID is equal to the gathering job request ID. If the IDs are equal then update the corresponding job status with `AnalysisAvailable=True` condition and continue with the following step below, otherwise update the corresponding job status with `AnalysisAvailable=False` condition and do nothing 
- based on the status of the last `datagather.insights.openshift.io` CR update the `insightsoperator.operator.openshift.io` CR and also the clusteroperator conditions
- prune the job definitions and `datagather.insights.openshift.io` CRs older than 24 hours. This time attribute will not be exposed in the configuration options by default, but it can be overriden.  

The above steps are illustrated in the following diagram: 

![data gathering](./on-demand%20gathering%20diagram.png "new data gathering")
  
The periodically created jobs will use `generateName` attribute with value `periodic`. To start on-demand data gathering, the user must create a new CR with a unique name.

### Risks and Mitigations

Risk: Inconsistent Insights analysis results. Assume following situation:
- periodic gathering is running in the Insights operator (let's call it gather A)
- user runs a new on demand data gathering (let's call it gather B) e.g 30 seconds after gather A
- gather A and B finish at essentially the same time
- the external processing service (in the console.redhat.com) provides only one recent Insights analysis result, and there is no guarantee which (of the two) it is.
- this can be very confusing, because user running gather B can get result of the gather A (e.g there could be different gatherers enabled, which would produce different results)

Mitigation: The gathering job will only check whether the corresponding archive was successfully processed or not (it will not download the full Insights analysis report every time). 
This requires a new API endpoint to be implemented in the CCX processing data pipeline ([API documentation here](https://console.redhat.com/docs/api/insights-results-aggregator/v2)). This request is tracked in https://issues.redhat.com/browse/CCXDEV-9980. 
The original requirement for the external processing was to provide the most recent Insights analysis and recommendations in the HAC (Hybrid Application Console) and ACM (Advanced Cluster Manager) and there is no use case for maintaining a history of Insights recommmedations/analyses in the cluster, plus it would take considerable effort to enable such a history. 

Risk: Problematic access to archives of gathered data (e.g for inspecting the data). The archives are currently stored in a container. This is problematic because once the Pod/job is finished, the container can no longer be accessed. 

Mitigation: Introduce a new config option for the Insights operator. This option allows users to specify a volume name and path that can then be  mounted to a job, and the job will also store the archives on the mounted volume. This will be a global configuration option, which means that all gathering jobs will store archives on this volume. 

### Drawbacks

- Very frequent data gathering in a cluster can produce a significant load on the processing part in the console.redhat.com and can overwhelm the processing pipeline. 
Theoretically, we could limit the number of user-trigerred gathering over time, but this problem can happen today when something is deleting the Insights operator pod on a regular basis with appropriate timing (so that data can be uploaded). Some kind of rate limiting should be implemented on "console.redhat.com" side.  
- When the newly spawned container is failing to upload the data then it can be running for some longer time (couple of hours). 


## Design Details

### Open Questions
- ~~How to solve the problem with only one Insights analysis result available in a cluster? How to avoid the "uncertainty" of the Insights analysis results described in the [Risk and Mitigations](#risks-and-mitigations) section?~~ 
  The gathering jobs will only check the status of the corresponding Insights analysis via the new API endpoint (See the section [Updates required to allow on-demand data gathering](#updates-required-to-allow-on-demand-data-gathering)) using the `insightsRequestID` attribute. 
  The Insights analysis report will be downloaded by the operator when a gathering job is finished. The request ID of the Insights analysis will be compared to the corresponding job request ID and the job `AnalysisAvailable` condition will be updated based on this comparison. 
- ~~Should the newly created gathering process update the clusteroperator conditions (e.g it fails to upload the data, should it update the `Degraded` status as it is today)? This can probably be racy too or it can lead to temporary and inconsistent conditions state.~~ 
  Yes. The scenario is described in the [Updates required to allow on-demand data gathering](#updates-required-to-allow-on-demand-data-gathering) section.
- ~~What all is required to spawn an extra container? Do we need a new image and a new binary (note that the Insights operator currently includes some additional features - such as pulling SCA certificates - that are not required for a new container/process)?~~ I think we can use the existing image and binary and introduce a new command.
- ~~Is some clean-up of the old `datagather.insights.openshift.io` CRs (and completed containers/pods) required? I think it would make sense to keep them only for 24h.~~ The CRs older than 24 hours will be removed by the Insights operator. This time option will be configurable via the operator configuration. 

### Test Plan

This will require significant updates to the existing Insights operator testsuite. The tests will probably need to setup a persistent volume to easily consume the gathered archive. Following scenarios will need to be tested:

- creation of a new `datagather.insights.openshift.io` CR triggers a new on-demand gathering
- the status of the `datagather.insights.openshift.io` CR is updated correctly and propagated to the `insightsoperator.operator.openshift.io` CR
- error cases (related to the failing upload or download of the data) lead to same clusteroperator conditions as before
- old jobs and CRs are pruned
- Prometheus metrics (registered by the operator) work as previously

### Graduation Criteria
The plan is to introduce the first version of the new API behind the `TechPreviewNoUpgrade` feature gate and the new API in version `v1alpha1`. 

#### Dev Preview -> Tech Preview

When `TechPreviewNoUpgrade` feature is not enabled, the Insights operator works and reports in the same way. 
When `TechPreviewNoUpgrade` feature is enabled then the operator runs the periodical data gathering in a new Pod as a job and the status reporting follows the new approach with the new `datagather.insights.openshift.io` CR. 
User can create a new `datagather.insights.openshift.io` CR to trigger on-demand data gathering.  

#### Tech Preview -> GA

The `TechPreviewNoUpgrade` feature gate requirement is removed. The Insights operator runs periodical data gathering in a separate Pod as a job and works with the new `datagather.insights.openshift.io` CRD. The old jobs and CRs are periodically removed. 

Other than that we would like to:
- get some time to gather feedback and some user experience
- ensure stability
- do more e2e testing

#### Removing a deprecated feature

This on-demand data gathering feature can be deprecated, but it will require changes in the Insights operator code base. 

### Upgrade / Downgrade Strategy

No special upgrade or downgrade strategy should be needed. The new `datagather.insights.openshift.io` CRD will be introduced with an upgrade to given version (allowing users to run on demand Insights data gathering). 
When a cluster is degraded, the `datagather.insights.openshift.io` CRD and corresponding CRs will remain in the cluster, but will not be watched by any controller. There should be no bad impacts on the cluster in this scenario. 

### Version Skew Strategy

### Operational Aspects of API Extensions

This API is provided exclusively for the Insights operator and there will be no other component depending on this new API. This new API will require the removal of the existing `insightsdatagather.config.openshift.io` API from the `TechPreviewNoUpgrade` so that the operator can retrieve the user configuration and propagate it to the new `datagather.insights.openshift.io` CR. 

#### Failure Modes

This API should not be considered as critical part of the OCP control plane. Any problems will need to be addressed by the CCX team (Insights operator subteam). There can be following:
- the operator fails to prune old jobs (and CRs) and this can lead to growing number of pods in the `openshift-insights` namespace
- the gathering job fails to upload the data and will be running couple of hours. This should not be a problem, but the job must always complete (error/failed state is acceptable and the job back-off limit will be set to 0). 

#### Support Procedures

As mentioned in the previous section, the main problem may be that we are not getting data from OCP clusters. 
This should not happen, but in some cases there are authentication failures and the operator fails to upload the data. 
This can manifest as a few gathering pods in an error state. This status is exposed in the clusteroperator conditions as well (and the operator is marked as degraded in case of upload failures). It is always recommended to check the logs of the operator as well as the gathering pods. 
The data gathering can be disabled by the configuration (see the [config API enhancement](./insights-config-api.md)).  

## Implementation History

As noted in the [Graduation criteria](#graduation-criteria) section, the first API version will be behind the `TechPreviewNoUpgrade` feature gate.

## Alternatives

- Current alternative is to delete the Insights operator Pod and let the replication controller start a new Pod that will run new data gathering almost immediately. 
- ~~Run the on-demand gathering within an existing container. This approach would use very similar `datagather.insights.openshift.io` CR (as mentioned above) to run the on-demand Insights data gathering, but the operator wouldn't spawn any new container.~~ 
  ~~It would run the data gathering within an existing container and it would interrupt any gathering in progress (if any). The new CR wouldn't have any status and the status will be reported to the `insightsoperator.operator.openshift.io` CR.~~ 
  This was discussed during the review of the enhancement proposal and it was agreed that creating new pods is better for observability and status reporting. 

