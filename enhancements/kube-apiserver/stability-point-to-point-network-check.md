---
title: point-to-point-network-check
authors:
  - "@deads2k"
  - "@sanchezl"
reviewers:
approvers:
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
replaces:
superseded-by:
---

# Stability: Point to Point Network Check

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance, 
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this? 

## Summary

There are a few key network connections that are important to the health of the cluster.
We can create a CRD to store the the last success time for each point-to-point check.
By watching for explicit failures and lack of updates, we can produce a picture of when a point-to-point check failed.
This information could be correlated to e2e failures, alerts, events, logs, or other information feeds.

## Motivation

This attempts to identify failures of particular connections in a black-box manner that is independent of SDN, cloud,
or cluster ingress.

This enhancement compliments existing metrics reported by REST client instrumentation. This enhancement derives
additional value by:
* REST calls can fail for a variety of reasons beyond simple DNS lookup and reachability of the target endpoint.
* Limiting scope to DNS lookup and TCP connect to target endpoint. REST client can fail for a variety of other reasons.
* Providing insights into target endpoints that are not accessed a REST client.
* Detecting and reporting issues independently of monitoring, which might itself not be available.
* Providing insights during a rollout.
* Providing status that can be consumed by an operator.

### Goals

1. be independent of SDN, cloud, or cluster ingress
2. Identify connection failure between each kas and each oas endpoint
3. Identify connection failure between each kas and each etcd
4. Identify connection failure between each kas and oas service IP
5. Identify connection failure between each oas and each kas host IP
6. Identify connection failure between each oas and each kas endpoint IP
7. Identify connection failure between each oas and kas service IP
8. Identify connection failure between each oas and each etcd

### Non-Goals

## Proposal

### 1. One instance of a new `PodNetworkConnectivityCheck` CR per point to point connection.

A point to point connection is between a source pod and a target endpoint.

Component operators will create a `PodNetworkConnectivityCheck` resource per point-to-point connection. If an operator 
is not managing the source pod directly, but via a workload controller (such as via a deployment or daemonset), the 
operator will observe the workload resources to determine the list of pods and create a `PodNetworkConnectivityCheck` 
resource for each target endpoint per pod. Operators that directly manage a source pod (such as operators that manage 
static-pods) will create `PodNetworkConnectivityCheck` resources for each target endpoint when creating the source pod.

### 2. Detailed status, probably some kind of latency information.

The `PodNetworkConnectivityCheck` will maintain a log of the actions performed during a check, recording if the actions
were successful, a reason for success or failure, latency of performing the action and some human readable message.

**LogEntry:**
```go
// LogEntry records events
type LogEntry struct {
	// Start time of check action.
	Start metav1.Time `json:"time,omitempty"`
	// Status indicates if the action performed by the check was successful or not.
	Success bool `json:"status"`
	// Reason for status in a machine readable format.
	Reason LogEntryReason `json:"reason"`
	// Message explaining status in a human readable format.
	Message string `json:"message"`
	// Latency records how long the action mentioned in the entry took.
	Latency time.Duration `json:"latency"`
}
``` 

The start and end time of detected outages will be logged separately from the individual check actions.

### 3. It should be possible to clean up garbage after a couple hours of inactivity.

A controller to prune `PodNetworkConnectivityCheck` resources, will be provided for inclusion into source pod operators.

A `PodNetworkConnectivityCheck` resource would be pruned after 48 hours of inactivity.

Tools updating `PodNetworkConnectivityCheck` log entries should simultaneously delete the oldest log entry to ensure
there are never more that 20 entries.

### 4. Don't delete aggressively, on upgrades, we have different endpoints

Source pods managed by workload controllers will have new `PodNetworkConnectivityCheck` resources to go along with their
new names. The old `PodNetworkConnectivityCheck` resource will eventually go stale and be cleaned up by the provided
pruning controller. 

Directly managed pods whose names do not change (such as static pods) will share `PodNetworkConnectivityCheck` resources
across revisions and upgrades.

### 5. Create a binary that can take multiple destinations paired with instances to write to.

### 6. Use that binary in containers included in each kas and oas (or other interesting) pod.

The binary will:

1. List all `PodNetworkConnectivityCheck` resource in the same namespace, where `spec.sourcePodName` matches the pod the
   binary is running in.

2. Perform each check found.

3. Append entries to the `status.successes` and `status.failures` as needed.

4. Append entries to `status.outages` as needed.

5. Update `status.conditions` as needed.

6. If unable to update the resource, retry updates at a later time.

7. Repeat evey minute.

### User Stories [optional]

Detail the things that people will be able to do if this is implemented.
Include as much detail as possible so that people can understand the "how" of
the system. The goal here is to make this feel real for users without getting
bogged down.

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

#### RelatedEndpoint Custom Resource

```go
// Package v1alpha1 is an API version in the controlplane.operator.openshift.io group
package v1alpha1

import (
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodNetworkConnectivityCheck
type PodNetworkConnectivityCheck struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   PodNetworkConnectivityCheckSpec   `json:"spec"`
	Status PodNetworkConnectivityCheckStatus `json:"status"`
}

type PodNetworkConnectivityCheckSpec struct {
	// SourcePod names the pod from which the condition will be checked
	SourcePod string `json:"sourcePod"`
	// EndpointAddress to check. A TCP address of the form host:port. Note that
	// if host is a DNS name, then the check would fail if the DNS name cannot
	// be resolved. Specify an IP address for host to bypass DNS name lookup.
	TargetEndpoint string `json:"targetEndpoint"`
}

type PodNetworkConnectivityCheckStatus struct {
	// Successes contains logs successful check actions
	Successes []LogEntry `json:"successes"`
	// Failures contains logs of unsuccessful check actions
	Failures []LogEntry `json:"failures"`
	// Outages contains logs of time periods of outages
	Outages []OutageEntry `json:"outages"`
	// Conditions summarize the status of the check
	Conditions []metav1.ConditionStatus `json:"conditions"`
}

// LogEntry records events
type LogEntry struct {
	// Start time of check action.
	Start metav1.Time `json:"time,omitempty"`
	// Status indicates if the action performed by the check was successful or not.
	Success bool `json:"status"`
	// Reason for status in a machine readable format.
	Reason LogEntryReason `json:"reason"`
	// Message explaining status in a human readable format.
	Message string `json:"message"`
	// Latency records how long the action mentioned in the entry took.
	Latency time.Duration `json:"latency"`
}

// OutageEntry records time period of an outage
type OutageEntry struct {
	// Start of outage detected
	Start time.Time
	// End of outage detected
	End time.Time
}

type PodNetworkConnectivityCheckCondition struct {
	// Type of the condition
	Type PodNetworkConnectivityCheckConditionType `json:"type"`
	// Status of the condition
	Status metav1.ConditionStatus `json:"status"`
	// machine readable reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// human readable message indicating details about last transition.
	Message string `json:"message,omitempty"`
	// Last time the condition transit from one status to another.
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
}

type LogEntryReason string

type PodNetworkConnectivityCheckConditionType string

const (
	Reachable    PodNetworkConnectivityCheckConditionType = "Reachable"
	DNSDone      LogEntryReason                           = "DNSDone"
	DNSError     LogEntryReason                           = "DNSError"
	ConnectDone  LogEntryReason                           = "ConnectDone"
	ConnectError LogEntryReason                           = "ConnectError"
)

```


#### Point-to-point Network Check Tool

Runs in a container in the interested pod. Executes check and updates `PodNetworkConnectivityCheck` resources for the pod.

Usage: 

```
p2pnc check tcp-endpoint

``` 


### Risks and Mitigations

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom? How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

None

### Examples

#### Example PodNetworkConnectivityCheck instance

```yaml
kind: PodNetworkConnectivityCheck
version: network.openshift.io/v1alpha1
metadata:
    name: kas-10-0-137-239-r7-to-etcd-10-0-148-99
    namespace: openshift-kube-apiserver
spec:
    sourcePodName: kube-apiserver-ip-10-0-135-158.us-west-1.compute.internal
    targetEndpoint: 10.0.134.23:2379
status:
    conditions:
      - type: Reachable
        status: False
        message: "Failed connect to 10.0.134.23:2379; No route to host"
    outages:
      - start: '2020-04-20T14:11:18Z'
      - start: '2020-03-20T12:10:00Z'
        end: '2020-03-20T12:12:00Z'
    successes:
      - time: '2020-04-20T13:11:18Z'
        success: true
        reason: ConnectDone
        message: "Connected to 10.0.134.23:2379"
        duration: "200ms"
      - time: '2020-04-20T12:11:18Z'
        success: true
        reason: ConnectDone
        message: "Connected to 10.0.134.23:2379"
        duration: "200ms"
    failures:
      - time: '2020-04-20T14:11:18Z'
        success: false
        reason: ConnectError
        message: "Failed connect to 10.0.134.23:2379; No route to host"
        latency: "208ms"

```

#### Example PodNetworkConnectivityCheck instance with DNS
```yaml
kind: PodNetworkConnectivityCheck
version: network.openshift.io/v1alpha1
metadata:
    name: apiserver-db674d48d-cxj9p-to-etcd
    namespace: openshift-apiserver
spec:
    sourcePodName: apiserver-db674d48d-cxj9p
    targetEndpoint: etcd.openshift-etcd.svc:2379
status:
    conditions:
      - type: Reachable
        status: true
        message: "Connection to etcd.openshift-etcd.svc:2379 established."
    successes:
      - time: '2020-04-20T12:11:18Z'
        success: true
        reason: ConnectDone
        message: "Connection to etcd.openshift-etcd.svc:2379 established."
        duration: "200ms"
      - time: '2020-04-20T13:11:18Z'
        success: true
        reason: DNSDone
        message: "etcd.openshift-etcd.sv resolved to 10.0.140.67"
        duration: "200ms"
```

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
