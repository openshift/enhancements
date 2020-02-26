---
title: container-log-throttling
authors:
  - "@syedriko"
reviewers:
  - "@aconway"
  - "@portante"
  - "@jcantrill"
  - "@bparees"
  - "@redhatdan"
approvers:
  - "@jcantrill"
  - "@bparees"
  - "@redhatdan"
creation-date: 2020-02-23
last-updated: 2020-02-23
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
  - conmon PR [WIP: logging behavior policies and rated-limited logging](https://github.com/containers/conmon/pull/92)
  - libpod PR [WIP: Added support for log policy and log rate limit in conmon](https://github.com/containers/libpod/pull/4663)
replaces: []
superseded-by: []
---

# Container Log Throttling

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

  Container log throttling is a set of capabilities in OpenShift aimed at providing cluster administrators with means of controlling logging policies and logging throughput of individual containers or their groupings. Containers generate two log streams, stdout and stderr, that the container runtime routes to a log file on the cluster node. Stdout and stderr log streams are controlled separately.
  The following logging policies are proposed:
  1. Passthrough
     Unrestricted logging. For backward compatibility, this is the default if no policy is specified. It is an error to specify a rate limit with this policy.
  1. Ignore
     Container stdout/stderr output is not written to the log and is discarded. It is an error to specify a rate limit with this policy.
  1. Backpressure
     Container is only allowed to log as fast as rate limit allows and may occasionally block to stay within that limit. A rate limit must be specified with this policy.
  1. Drop
     Container is only allowed to log as fast as rate limit allows. The container avoids blocking, but some log output may be skipped and not written to the log to stay within the rate limit. A rate limit must be specified with this policy.

   The log stream rate limit is specified in bytes per second. A suffix of "K", "M", "G", or "T" can be added to denote kibibytes (*1024), mebibytes, and so on.

## Motivation

  Administrators of an OpenShift cluster need the proper controls/policies for logging behaviors in order to maintain a balanced system. Without these controls, the rate at which containers can emit logs can exceed the rate at which logging collectors can read them, leading to a situation where the kubelet deletes entire log files as part of log rotation before they are read by the collectors. This leads to multiple megabytes of logs being lost.

## Open Questions

1. How can we help cluster administrators identify containers that require log throttling?
1. How can groupings of containers can be expressed in OpenShift for the purposes of log throttling?

### Goals

* Ship an OpenShift release with an implementation of the container logging policies and rate limiting functionality described in this enhancement while meeting quality goals.

### Non-Goals

* It is not a goal of this enhancement to introduce control of logs from non-containerized OCP components.
* It it not a goal to introduce log throttling into all container runtimes and container and VM technologies that can be run on Kubernetes. The scope is limited to the ones necessary to enable shipping of the next version OpenShift: Kubernetes API, kubelet, cri-o, podman and conmon on Linux.

## Proposal

### User Stories

#### As a cluster administrator, I want to disable logs from a particular container or a group of containers.

#### As a cluster administrator, I want to cap the rate at which a container or a group of containers can produce logs. I want to keep all the log entries and I don't mind slowing down the containers for that.

#### As a cluster administrator, I want to cap the rate at which a container or a group of containers can produce logs. I don't want to slow down the containers and I don't mind losing some of the log entries.

### Risks and Mitigations

1. This proposal calls for modifications to core Kubernetes APIs and components and as such runs the risk of requiring substantial investment into building consensus around it in the Kubernetes community.
1. It may take a substantial amount of time for the proposed changes to make it into released versions of the corresponding components.

## Design Details

The general idea of the design is to build the container log throttling mechanism into [conmon](https://github.com/containers/conmon), the container runtime monitor, and expose the controls successively through the CRI interface and the Container core/v1 API. A POC for the conmon part of the implementation has been built, please refer to the see-also field in the header of this document.

It is envisioned that the following changes will need to be made to the Kubernetes API

### Container core/v1 API:

```go
// k8s.io/api/core/v1/types.go

// A single application container that you want to run within a pod.
type Container struct {
...
	// Policies the container runtime applies to this container's
	// log streams. LogStreams[0] controls stdout, LogStreams[1]
	// controls stderr.
	// Cannot be updated.
	// +optional
    // +patchMergeKey=logStream
	// +patchStrategy=merge
	// +listType=map
	// +listMapKey=logStream
	LogPolicies []LogStreamPolicy `json:"logPolicies,omitempty" patchStrategy:"merge" patchMergeKey:"logStream" protobuf:"bytes,23,rep,name=logPolicies"`
}

// LogStreamPolicy describes the policy container runtime enforces
// on containers' log stream, i.e. stdout or stderr.
type LogStreamPolicy struct {
	// The logging policy to apply. Supported values are 
	// passthrough, ignore, backpressure and drop.
	Policy string `json:"policy" protobuf:"bytes,1,opt,name=policy"`
	// The maximum log rate, in bytes per second.
	// Must be specified for the backpressure and drop policies.
	// Must not be specified for the passthrough and ignore policies.
	// A suffix of "K", "M", "G", or "T" can be added.
	// +optional
	RateLimit string `json:"rateLimit" protobuf:"bytes,2,opt,name=rateLimit"`
}
```

### Container internal form

```go
// kubernetes/pkg/apis/core/types.go

package core

type Container struct {
...
	// Policies the container runtime applies to this container's
	// log streams. LogPolicies[0] controls stdout, LogPolicies[1]
	// controls stderr.
	// Cannot be updated.
	// +optional
	LogPolicies []LogStreamPolicy
}

// LogStreamPolicy describes the policy container runtime enforces
// on containers' log stream, i.e. stdout or stderr.
type LogStreamPolicy struct {
	// The logging policy to apply. Supported values are 
	// passthrough, ignore, backpressure and drop.
	Policy string
	// The maximum log rate, in bytes per second.
	// Must be specified for the backpressure and drop policies.
	// Must not be specified for the passthrough and ignore policies.
	// A suffix of "K", "M", "G", or "T" can be added.
	// +optional
	RateLimit string
}
```

### CRI API
```
// cri-api/blob/master/pkg/apis/runtime/v1alpha2/api.proto

message ContainerConfig {
...
	// Policies the container runtime applies to this container's
	// log streams. The first instance controls stdout, the second
	// controls stderr.
	repeated LogStreamPolicy = 17
}

// LogStreamPolicy describes the policy container runtime enforces
// on containers' log stream, i.e. stdout or stderr.
message LogStreamPolicy {
	// The logging policy to apply. Supported values are 
	// passthrough, ignore, backpressure and drop.
	string policy = 1;
	// The maximum log rate, in bytes per second.
	// Must be specified for the backpressure and drop policies.
	// Must not be specified for the passthrough and ignore policies.
	// A suffix of "K", "M", "G", or "T" can be added.
	string rate_limit = 2;
}
```

### CRI-O

```
// cri-o/cri-o/internal/oci/container.go

package oci

type Container struct {
...
	logPolicies []LogStreamPolicy
}

// LogStreamPolicy describes the policy container runtime enforces
// on containers' log stream, i.e. stdout or stderr.
type LogStreamPolicy struct {
	// The logging policy to apply. Supported values are 
	// passthrough, ignore, backpressure and drop.
	Policy string `json:"policy"`
	// The maximum log rate, in bytes per second.
	// Must be specified for the backpressure and drop policies.
	// Must not be specified for the passthrough and ignore policies.
	// A suffix of "K", "M", "G", or "T" can be added.
	RateLimit string `json:"rateLimit"`
}
```
