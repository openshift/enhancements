---
title: api-removal-notifications
authors:
  - "@sanchezl"
  - "@deads2k"
reviewers:
  - "@deads2k"
  - "@sttts"
approvers:
  - TBD
creation-date: 2021-02-01
last-updated: 2021-02-17
status: provisional
---
# API Removal Notifications

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Notify customers that an API that will be removed in the next release is in use.

## Motivation

Kubernetes is being more aggressive with the removal of APIs that have been deprecated or pre-release APIs that have
reached a dead-end and will never reach GA. Additionally, the ability to re-enable a previously removed API has been
removed.

Customers should be notified that an API that will be removed in the next release is in use so that they can decide if
there is a need to take action in order to ensure success when upgrading to the next release.

### Goals

1. Notify customers that an API that will be removed in the next release is in use.
2. Notify EUS customers that an API that will be removed in the next EUS release is in use.
3. Help customers identify the workload that is using the API to be removed.

### Non-Goals

1. Automatically identify the workload using the API to be removed.
2. Prevent the customer from attempting to upgrade when an API that will be removed in the next release is in use.
3. Notify customers that an upstream API is not supported on OpenShift.

## Proposal

Fire an `APIRemovedInNextReleaseInUse` alert when an API that will be removed in the the next release in in use.

On EUS releases, also fire an `APIRemovedInNextEUSReleaseInUse` alert when an API that will be removed in the next
extended support release is in use.

Introduce a `APIRequestCount` API to track users of deprecated APIs.

Example:

```yaml
apiVersion: apiserver.openshift.io/v1
kind: APIRequestCount
metadata:
  name: certificatesigningrequests.v1beta1.certificates.k8s.io
spec:
  numberOfUsersToReport: 10
status:
  removedRelease: '1.22'
  currentHour:
    byNode:
    - byUser:
      - byVerb:
        - requestCount: 1
          verb: delete
        - requestCount: 31
          verb: get
        requestCount: 32
        userAgent: "openshift-tests/0.0.0"
        username: system:admin
      nodeName: 10.0.0.3
      requestCount: 32
    requestCount: 32
  requestsLast24h:
    byNode:
    - byUser:
      - byVerb:
        - requestCount: 1
          verb: delete
        - requestCount: 31
          verb: get
        requestCount: 32
        userAgent: "openshift-tests/0.0.0"
        username: system:admin
      nodeName: 10.0.0.3
      requestCount: 32
    requestCount: 32
        ...
```

### User Stories

#### Story 1

As an OpenShift administrator, I need visibility into which APIs that will be removed in the next release are currently
in use, so I can take any needed actions to ensure that I can upgrade to the next release.

#### Story 2

As an OpenShift administrator, I need visibility into which APIs that will be removed in the next EUS release are
currently in use, so I can take any needed actions to ensure that I can upgrade to next EUS release.

#### Story 3

As an OpenShift administrator, I need visibility into which workload is using an API that will be removed in the next
release, so I can take any needed actions to ensure that I can upgrade to the next release.

### Implementation Details/Notes/Constraints

#### Pre-release Lifecycle Background

Pre-release APIs are now required to specify `prerelease-lifecycle-gen` tags that inform the apiserver to, among other
things, maintain the `apiserver_requested_deprecated_apis` metric.

```go
// +k8s:prerelease-lifecycle-gen:introduced=1.1
// +k8s:prerelease-lifecycle-gen:deprecated=1.8
// +k8s:prerelease-lifecycle-gen:removed=1.18
// +k8s:prerelease-lifecycle-gen:replacement=apps,v1,Deployment
```

The `apiserver_requested_deprecated_apis` metrics provides the metadata needed by this enhancement to "look into the
future" and identify APIs, currently in use, that will be removed in the next release.

In order to provide the metadata needed to look forward to the next EUS release, `prerelease-lifecycle-gen` tags updated
between EUS releases will need to be back-ported to the latest EUS release.

#### Alert Expression

For the alert expression we filter the existing `apiserver_requested_deprecated_apis` series in combination with
the `apiserver_request_total` series to determine if the API to be removed in the next release is actively in use.

```text
group(apiserver_requested_deprecated_apis{removed_release="1.xx"}) by (group,version,resource)
and
(sum by(group,version,resource) (rate(apiserver_request_total[4h]))) > 0
```

Value of `removed_release`:

| OpenShift Release | removed_release | removed_release (EUS) |
| ----------------- | --------------- | --------------------- |
| 4.6 / 4.6 EUS     | 1.20            | 1.23                  |
| 4.7               | 1.21            |                       |
| 4.8               | 1.22            |                       |
| 4.9               | 1.23            |                       |
| 4.10 / 4.10 EUS   | 1.24            | TBD                   |

#### APIRequestCount Resource

Patch the apiserver such that requests to a deprecated API are logged to a `APIRequestCount` resource corresponding
to the deprecated API. This resource can help customers determine which workloads are using deprecated APIs, including
those which are a being removed in the next release.

```go
// Package v1 is an api version in the apiserver.openshift.io group
package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:resource:scope="Cluster"
// +kubebuilder:subresource:status
// +genclient:nonNamespaced

// APIRequestCount tracks requests made to an API. The instance name must
// be of the form `resource.version.group`, matching the resource.
type APIRequestCount struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	// spec defines the characteristics of the resource.
	// +kubebuilder:validation:Required
	// +required
	Spec APIRequestCountSpec `json:"spec"`

	// status contains the observed state of the resource.
	Status APIRequestCountStatus `json:"status,omitempty"`
}

type APIRequestCountSpec struct {

	// numberOfUsersToReport is the number of users to include in the report.
	// If unspecified or zero, the default is ten.  This is default is subject to change.
	// +kubebuilder:default:=10
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=100
	// +optional
	NumberOfUsersToReport int64 `json:"numberOfUsersToReport"`
}

// +k8s:deepcopy-gen=true
type APIRequestCountStatus struct {

	// conditions contains details of the current status of this API Resource.
	// +patchMergeKey=type
	// +patchStrategy=merge
	Conditions []metav1.Condition `json:"conditions" patchStrategy:"merge" patchMergeKey:"type"`

	// removedInRelease is when the API will be removed.
	// +kubebuilder:validation:MinLength=0
	// +kubebuilder:validation:Pattern=^[0-9][0-9]*\.[0-9][0-9]*$
	// +kubebuilder:validation:MaxLength=64
	// +optional
	RemovedInRelease string `json:"removedInRelease,omitempty"`

	// requestCount is a sum of all requestCounts across all current hours, nodes, and users.
	// +kubebuilder:validation:Minimum=0
	// +required
	RequestCount int64 `json:"requestCount"`

	// currentHour contains request history for the current hour. This is porcelain to make the API
	// easier to read by humans seeing if they addressed a problem. This field is reset on the hour.
	// +optional
	CurrentHour PerResourceAPIRequestLog `json:"currentHour"`

	// last24h contains request history for the last 24 hours, indexed by the hour, so
	// 12:00AM-12:59 is in index 0, 6am-6:59am is index 6, etc. The index of the current hour
	// is updated live and then duplicated into the requestsLastHour field.
	// +kubebuilder:validation:MaxItems=24
	// +optional
	Last24h []PerResourceAPIRequestLog `json:"last24h"`
}

// PerResourceAPIRequestLog logs request for various nodes.
type PerResourceAPIRequestLog struct {

	// byNode contains logs of requests per node.
	// +kubebuilder:validation:MaxItems=512
	// +optional
	ByNode []PerNodeAPIRequestLog `json:"byNode"`

	// requestCount is a sum of all requestCounts across nodes.
	// +kubebuilder:validation:Minimum=0
	// +required
	RequestCount int64 `json:"requestCount"`
}

// PerNodeAPIRequestLog contains logs of requests to a certain node.
type PerNodeAPIRequestLog struct {

	// nodeName where the request are being handled.
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:MaxLength=512
	// +required
	NodeName string `json:"nodeName"`

	// requestCount is a sum of all requestCounts across all users, even those outside of the top 10 users.
	// +kubebuilder:validation:Minimum=0
	// +required
	RequestCount int64 `json:"requestCount"`

	// byUser contains request details by top .spec.numberOfUsersToReport users.
	// Note that because in the case of an apiserver, restart the list of top users is determined on a best-effort basis,
	// the list might be imprecise.
	// In addition, some system users may be explicitly included in the list.
	// +kubebuilder:validation:MaxItems=500
	ByUser []PerUserAPIRequestCount `json:"byUser"`
}

// PerUserAPIRequestCount contains logs of a user's requests.
type PerUserAPIRequestCount struct {

	// userName that made the request.
	// +kubebuilder:validation:MaxLength=512
	UserName string `json:"username"`

	// userAgent that made the request.
	// The same user often has multiple binaries which connect (pods with many containers).  The different binaries
	// will have different userAgents, but the same user.  In addition, we have userAgents with version information
	// embedded and the userName isn't likely to change.
	// +kubebuilder:validation:MaxLength=1024
	UserAgent string `json:"userAgent"`

	// requestCount of requests by the user across all verbs.
	// +kubebuilder:validation:Minimum=0
	// +required
	RequestCount int64 `json:"requestCount"`

	// byVerb details by verb.
	// +kubebuilder:validation:MaxItems=10
	ByVerb []PerVerbAPIRequestCount `json:"byVerb"`
}

// PerVerbAPIRequestCount requestCounts requests by API request verb.
type PerVerbAPIRequestCount struct {

	// verb of API request (get, list, create, etc...)
	// +kubebuilder:validation:MaxLength=20
	// +required
	Verb string `json:"verb"`

	// requestCount of requests for verb.
	// +kubebuilder:validation:Minimum=0
	// +required
	RequestCount int64 `json:"requestCount"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// APIRequestCountList is a list of APIRequestCount resources.
type APIRequestCountList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []APIRequestCount `json:"items"`
}
```

### Risks and Mitigations

#### Storage Version Migration

During an EUS upgrade, where multiple "normal" releases are applied back-to-back, there is a risk that there are
persisted instances of resources that can longer be decoded from storage due to the removal of the storage encoder for a
removed version. We should either provide customers with documentation on upgrading storage versions or provide a
storage version migration tool to perform the migrations automatically.

## Design Details

### Open Questions [optional]

### Test Plan

* end-to-end test to ensure `removed_release` is bumped when the underlying Kubernetes release is bumped.

### Graduation Criteria

### Upgrade / Downgrade Strategy

#### Dev Preview -> Tech Preview
linter pass

#### Tech Preview -> GA
linter pass

#### Removing a deprecated feature
linter pass

### Version Skew Strategy

## Implementation History
linter pass

## Drawbacks
linter pass

## Alternatives
linter pass

### Console Recommendations vs. Alerts

Alerts are not ideal for these notifications as they are not necessarily immediately actionable. There is the
possibility in the future of moving from using alerts to "console recommendations", but "console recommendations" are
still in the request for enhancement stage.

### Upgradeable Cluster Operator Status

Setting `Upgradable=False` on a cluster operator status is not recommended as there are situations where the customer,
after analyzing the workloads on the cluster, might choose to ignore the removed API notifications and continue with an
upgrade to the next release.

## Infrastructure Needed [optional]
