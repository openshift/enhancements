---
title: haproxy-thread-tuning
authors:
  - "@rfredette"
reviewers:
  - "@Miciah"
  - "@danehans"
  - "@frobware"
  - "@sgreene"
  - "@knobunc"
  - "@miheer"
  - "@candita"
approvers:
  - "@knobunc"
  - "@miciah"
  - "@frobware"
  - "@danehans"
creation-date: 2021-03-24
last-updated: 2021-03-25
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced|informational
see-also:
replaces:
superseded-by:
---

# HAProxy Thread Tuning

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal is to allow the cluster administrator to configure the number of
connection handling threads within router pods.

## Motivation

As the number of routes and the volume of traffic through a cluster increases,
eventually the router pod will reach a limit to how many connections it can
handle. In order to increase the maximum number of connections, administrators
can split the routes into multiple shards, each shard being handled by a
separate router.  More router pods can also be deployed without sharding,
allowing more connections to be handled, but this comes with more overhead, as
each router must separately verify the health of its backends.  These redundant
health checks increase the volume of traffic passing through the cluster, and
can cancel out much of the performance gain from deploying more router pods.

In OpenShift 3.x, users could configure routers to increase the number of
threads that HAProxy spawns, allowing a single router to handle more
connections without additional health check overhead. This document proposes to
allow users to employ a similar strategy, and provide a field within the
IngressController API to specify the number of threads allocated within router
pods

### Goals

Provide an API for configuring the number of threads handling connections in
router pods

### Non-Goals

Expose additional performance tuning parameters available within HAProxy

## Proposal

Add the subresource `threading` to the IngressController API. It will currently
contain one field, `count`.

```go
type IngressControllerSpec struct {
	// ...
	// Existing fields
	// ...

	// threading defines parameters for configuring threading options within
	// routers created under this IngressController. See specific threading
	// fields for their respective definitions and default values.
	//
	// +optional
	Threading IngressControllerThreading `json:"threading,omitempty"`
}

type IngressControllerThreading struct {
	// count defines the number of threads created per router pod. Creating
	// more threads allows each router pod to handle more connections, at the
	// cost of more system resources used. If this field is empty, the
	// IngressController will use a default value of 4 threads.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Minimum=4
	// +optional
	Count int `json:"count,omitempty"`
}
```

When unset, the ingress operator will provision routers with 4 threads,
matching the existing behavior in OpenShift 4.7.

It would be possible to implement this field as a simple integer within the
`IngressControllerSpec` as `spec.threadCount` or something similar, however
there are other tunable threading performance options that HAProxy exposes, and
it is possible that the ingress controller will be updated to support those
fields at a later date. If any of those fields are added, it would be
preferable to include those fields within a single subresource. In order to
make that potential upgrade less painful, the `spec.threading` subresource will
be added with only one field, `spec.threading.count`.

### User Stories

#### Story 1

> As a cluster administrator, I want to increase the amount of incoming
> connections my cluster can handle without configuring IngressController
> sharding

The administrator can patch their existing ingress controller to increase the
number of router threads:

```sh
$ oc patch ingresscontroller/<controller-name> --type=merge -p '{"spec": {"threading": {"count": <new-thread-count>} } }'
```

New router pods will be rolled out with the updated thread count.

### Risks and Mitigations

#### Increased Resource Usage

It's possible that an administrator could configure routers to have enough
threads that other pods on the same node could become resource starved when the
router pod is under full load. In order to mitigate this risk, the resources
requested in the router deployment should scale with the number of threads
configured.

This still presents a problem of overestimating the amount of resources
required when the router pod is under lower load, which could cause the node to
be underutilized during low or moderate traffic load. As such, some amount of
scale testing needs to be done before the appropriate resource request scaling
factor can be determined.

#### Maximum Connections Doesn't Scale Up With Additional Threads

At this time, the `spec.threading.count` field has no maximum set, but there is
a limit on how many open sockets a single process can have and HAProxy also has
a maximum number of connections allowed. Because of this, there is an upper
bound to the additional performance gained by adding more threads to HAProxy.
If the cluster administrator still requires more incoming connection handling
ability, they will have to split the routes across multiple IngressController
shards.

## Design Details

### Test Plan

#### Test 1
1. Create an IngressController with `spec.threading.count` unset. Wait for
   a router pod to be deployed.
2. Verify the router has the environment variable `"ROUTER_THREADS"` set to 4.
3. Patch the IngressController to set `spec.threading.count` to 7. Wait for
   the router pod to be updated.
4. Verify the router has the environment variable `"ROUTER_THREADS"` set to 7.
5. Patch the IngressController to remove `spec.threading.count`. Wait for the
   router pod to be updated.
6. Verify the router has the environment variable `"ROUTER_THREADS"` set to 4.

### Graduation Criteria

TODO

### Upgrade / Downgrade Strategy

TODO

### Version Skew Strategy

TODO

## Implementation History

## Drawbacks

## Alternatives

[IngressController sharding](https://docs.openshift.com/container-platform/4.7/networking/configuring_ingress_cluster_traffic/configuring-ingress-cluster-traffic-ingress-controller.html#nw-ingress-sharding-route-labels_configuring-ingress-cluster-traffic-ingress-controller)
