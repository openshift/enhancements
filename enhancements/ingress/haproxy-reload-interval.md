---
title: haproxy-reload-interval
authors:
  - "@Ethany-RH"
reviewers:
  - "?"
approvers:
  - "?"
api-approvers: # necessary?
  - "?"
creation-date: 2022-07-01
last-updated: 2022-07-01
tracking-link:
  - "https://issues.redhat.com/browse/NE-586"
see-also:
replaces:
superseded-by:
---

# Reload Interval in HAProxy

## Release Signoff Checklist

- [ ] Enhancement is `implementable`.
- [ ] Design details are appropriately documented from clear requirements.
- [ ] Test plan is defined.
- [ ] graduation criteria for dev preview, tech preview, GA
- [ ] User-Facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/).

## Summary

Add an API field to configure OpenShift router's `RELOAD_INTERVAL` environment variable so that administrators can define the minimum frequency the router is allowed to reload to accept new changes.

OpenShift router currently hard-codes this reload interval to 5s. It should be possible for administrators to tune this value as necessary. Based on the processes run in the cluster and the frequency that it sees new changes, increasing the minimum interval at which the router is allowed to reload can improve its efficiency.
This proposal extends the existing IngressController API to add a tuning option for the reload interval.

## Motivation

When there is an update to a route or endpoint in the cluster, the configuration for HAProxy changes, requiring that it reload for those changes to take effect. When HAProxy reloads to generate a new process with the updated configuration, it must keep the old process running until all its connections die. As such, frequent reloading increases the rate of accumulation of HAProxy processes, particularly if it has to handle many long-lived connections. The default reload interval is set as 5 seconds, which is too low for some scenarios, so it is important that administrators can extend this time interval.

In addition, HAProxy's roundrobin balancing starts over from the first server every time HAProxy reloads. Thus, another motivating factor is that frequent reloads can cause load imbalance on a backend's servers when using the roundrobin balancing algorithm.

### Goals

1. Enable the configuration of a reload interval via the `IngressControllerSpec`, specifically via the `IngressControllerSpecTuningOptions`, with a new parameter `ReloadInterval`. `ReloadInterval` exposes OpenShift router's internal environment variable `RELOAD_INTERVAL` as an API that the cluster administrator can set.
2. Leave the default interval at 5 seconds so that we do not perturb the behavior of existing clusters, particularly during upgrades.

### Non-Goals

Propose or advise on any new value for `IngressControllerTuningOptions.ReloadInterval` because the ideal reload interval varies for many different scenarios.

### User Stories

> As a cluster administrator, I want to configure RELOAD_INTERVAL to force HAProxy to reload its configuration less frequently in response to route and endpoint updates.

The administrator can use the new API to configure a longer reload interval. For example, the following command changes the default IngressController's minimum reload interval to 15 seconds:

```shell
oc -n openshift-ingress-operator patch ingresscontrollers/default --type=merge --patch='{"spec":{"tuningOptions":{"reloadInterval":"15s"}}}'
```

## Proposal

### Workflow Description

### API Extension

Add a new field `ReloadInterval` to the IngressController API:

```go
// IngressControllerTuningOptions specifies options for tuning the performance
// of ingress controller pods
type IngressControllerTuningOptions struct {
  	...

	// reloadInterval defines the minimum interval at which the router is allowed to reload
	// to accept new changes. Increasing this value can prevent the accumulation of
	// HAProxy processes, depending on the scenario. Increasing this interval can
	// also lessen load imbalance on a backend's servers when using the roundrobin
	// balancing algorithm. Alternatively, decreasing this value may decrease latency
	// since updates to HAProxy's configuration can take effect more quickly.
	//
	// The value must be a time duration value; see <https://pkg.go.dev/time#ParseDuration>.
	// Currently, the minimum value allowed is 1s, and the maximum allowed value is
	// 120s. Minimum and maximum allowed values may change in future versions of OpenShift.
	// Note that if a duration outside of these bounds is provided, the value of reloadInterval
	// will be capped/floored and not rejected (e.g. a duration of over 120s will be capped to
	// 120s; the IngressController will not reject and replace this disallowed value with
	// the default).
	//
	// A nil or zero value for reloadInterval tells the IngressController to choose the default,
	// which is currently 5s and subject to change.
	//
	// This field expects an unsigned duration string of decimal numbers, each with optional
	// fraction and a unit suffix, e.g. "100s", "1m30s". Valid time units are "s" and "m".
	//
	// Note: Setting a value significantly larger than the default of 5s can cause latency
	// in observing updates to routes and their endpoints. HAProxy's configuration will
	// be reloaded less frequently, and newly created routes will not be served until the
	// subsequent reload.
	//
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=^(0|([0-9]+(\.[0-9]+)?(s|m))+)$
	// +kubebuilder:validation:Type:=string
	// +optional
	ReloadInterval metav1.Duration `json:"reloadInterval,omitempty"`
}
```

### Implementation Details / Notes / Constraints

To expose the `ReloadInterval` in HAProxy, the environment variable `RELOAD_INTERVAL` will be added to the environment in [desiredRouterDeployment](https://github.com/openshift/cluster-ingress-operator/blob/master/pkg/operator/controller/ingress/deployment.go).
The HAProxy template will not be modified.

### Risks and Mitigation

A risk in this proposal is that customers who set a long reload interval to decrease the potential memory usage of HAProxy instances may inadverdently create latency issues in the cluster. Setting a large value for the reload interval can cause significant latency in observing updates to routes and their endpoints. This is because HAProxy's configuration will be reloaded less frequently, and newly created routes will not be served until the subsequent reload.

To mitigate this risk, we will set a lower cap (than other interval environment variables) to limit the largest time interval that `reloadInterval` will accept. In addition, we have also included a note in the API godoc warning users of the possible risk in setting a large reload interval.

### Drawbacks

## Design Details

### Test Plan

Unit testing can validate that `desiredRouterDeployment` sets the `RELOAD_INTERVAL` environment variable correctly. Unit testing can also validate that `capReloadIntervalValue` in deployment.go sets minimum and maximum caps correctly.

E2E Tests

1. Create a new IngressController. Wait for an ingress controller pod to be deployed.

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade/Downgrade Strategy

Upgrading from a previous release that does not have `Spec.TuningOptions.ReloadInterval` will leave the field empty, which is an acceptable state. With the field empty, the default value of 5s will be used.

If `Spec.TuningOptions.ReloadInterval` is set when downgrading to a release without the field, the value will be discarded, and the ingress controller will revert to the previous default of 5s.

### Version Skew Strategy

N/A

### Operations Aspects of API Extensions

#### Failure Modes

N/A

#### Support Procedures

If the frequency of reloads compromises the performance of HAProxy, and the revert to default values does not fix it, then that is indicative of another issue.

## Implementation History

## Alternatives
