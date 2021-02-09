---
title: api-removal-notifications
authors:
  - "@sanchezl"
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

Introduce a `DeprecatedAPIRequest` API to track users of deprecated APIs.

Example:

```yaml
version: api.openshift.io/v1
kind: DeprecatedAPIRequest
meta-data:
  name: 'flowschemas.v1alpha1.flowcontrol.apiserver.k8s.io'
status:
  removedRelease: '1.21'
  conditions:
    - type: UsedInPastDay
      status: True
  requestsLastHour:
    nodes:
      - nodeName: master0
        lastUpdate: '2020-01-02 05:00'
        users:
          - username: 'system:serviceaccount:openshift-cluster-version:default'
            count: 10
            requests:
              - verb: get
                count: 5
              - verb: update
                count: 3
              - verb: delete
                count: 2
  requestsLast24h:
    nodes:
      - nodename: master0
        lastUpdate: '2020-01-02 00:00'
        users:
          - username: 'system:serviceaccount:openshift-network-operator:default'
            ...
      - nodename: master1
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

#### DeprecatedAPIRequest Resource

Patch the apiserver such that requests to a deprecated API are logged to a `DeprecatedAPIRequest` resource corresponding
to the deprecated API. This resource can help customers determine which workloads are using deprecated APIs, including
those which are a being removed in the next release.

```go
// Package v1 is an api version in the api.openshift.io group
package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DeprecatedAPIRequest tracts requests made to a deprecated API. The instance name should
// be of the form `resource.version.group`, matching the deprecated resource.
type DeprecatedAPIRequest struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty" protobuf:"bytes,1,opt,name=metadata"`

	Spec   DeprecatedAPIRequestSpec   `json:"spec"`
	Status DeprecatedAPIRequestStatus `json:"status,omitempty"`
}

type DeprecatedAPIRequestSpec struct {
	// RemovedRelease is when the API will be removed.
	RemovedRelease string `json:"removedRelease"`
}

type DeprecatedAPIRequestStatus struct {

	// Conditions contains details of the current status of this API Resource.
	// +patchMergeKey=type
	// +patchStrategy=merge
	// +optional
	Conditions []metav1.Condition

	// RequestsLastHour contains request history for the current hour. This is porcelain to make the API
	// easier to read by humans seeing if they addressed a problem.
	// +optional
	RequestsLastHour RequestLog `json:"requestsLastHour"`

	// RequestsLast24h contains request history for the last 24 hours, indexed by the hour, so
	// 12:00AM-12:59 is in index 0, 6am-6:59am is index 6, etc..
	// +optional
	RequestsLast24h []RequestLog `json:"requestsLast24h"`
}

// RequestLog logs request for various nodes.
type RequestLog struct {

	// Nodes contains logs of requests per node.
	Nodes []NodeRequestLog `json:"nodes"`
}

// NodeRequestLog contains logs of requests to a certain node.
type NodeRequestLog struct {

	// NodeName where the request are being handled.
	NodeName string `json:"nodeName"`

	// LastUpdate should *always* being within the hour this is for.  This is a time indicating
	// the last moment the server is recording for, not the actual update time.
	LastUpdate metav1.Time `json:"lastUpdate"`

	// Users contains request details by top 10 users.
	Users []RequestUser `json:"users"`
}

// RequestUser contains logs of a user's requests.
type RequestUser struct {

	// UserName that made the request.
	UserName string `json:"username"`

	// Count of requests.
	Count int `json:"count"`

	// Requests details by verb.
	Requests []RequestCount `json:"requests"`
}

// RequestCount counts requests by API request verb.
type RequestCount struct {

	// Verb of API request (get, list, create, etc...)
	Verb string `json:"verb"`

	// Count of requests for verb.
	Count int `json:"count"`
}
```

#### Determining Workload

```sh
oc get deprecatedapirequests --field-selector status.removedRelease=1.21
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

### Version Skew Strategy

## Implementation History

## Drawbacks

## Alternatives

### Console Recommendations vs. Alerts

Alerts are not ideal for these notifications as they are not necessarily immediately actionable. There is the
possibility in the future of moving from using alerts to "console recommendations", but "console recommendations" are
still in the request for enhancement stage.

### Upgradeable Cluster Operator Status

Setting `Upgradable=False` on a cluster operator status is not recommended as there are situations where the customer,
after analyzing the workloads on the cluster, might choose to ignore the removed API notifications and continue with an
upgrade to the next release.

## Infrastructure Needed [optional]


