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

**(more non-goals I should add?)**

### User Stories

> As a cluster administrator, I want to configure RELOAD_INTERVAL to force HAProxy to reload its configuration less frequently in response to route and endpoint updates.

The administrator can use the new API to configure a longer reload interval. For example, the following command changes the default IngressController's minimum reload interval to 15 seconds:

```shell
oc -n openshift-ingress-operator patch ingresscontrollers/default --type=merge --patch='{"spec":{"tuningOptions":{"reloadInterval":"15s"}}}'
```

## Proposal

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
    // 720h (= 30 days). Minimum and maximum allowed values may change in future versions
    // of OpenShift.
    //
    // An empty reloadInterval tells the IngressController to choose the default, which
    // is currently 5s.
    //
    // This field expects an unsigned duration string of integer numbers, each with a unit suffix,
    // e.g. "300s", "1h", "2h45m". Valid time units are "s", "m", and "h".
    //
    // +kubebuilder:validation:Optional
    // +kubebuilder:validation:Pattern=^(0|([0-9]+(s|m|h))+)$
    // +kubebuilder:validation:Type:=string
    // +optional
	ReloadInterval *metav1.Duration `json:"reloadInterval,omitempty"`
}
```
### Implementation Details / Notes / Constraints

To expose the `ReloadInterval` in HAProxy, the environment variable `RELOAD_INTERVAL` will be added to the environment in [desiredRouterDeployment](https://github.com/openshift/cluster-ingress-operator/blob/master/pkg/operator/controller/ingress/deployment.go):
```go
    // desiredRouterDeployment returns the desired router deployment.
    func desiredRouterDeployment(ci *operatorv1.IngressController, ingressControllerImage string, ingressConfig *configv1.Ingress, apiConfig *configv1.APIServer, networkConfig *configv1.Network, proxyNeeded bool, haveClientCAConfigmap bool, clientCAConfigmap *corev1.ConfigMap) 
    (*appsv1.Deployment, error){
        ...
        reloadInterval := 5 * time.Second
	    if ci.Spec.TuningOptions.ReloadInterval != nil && ci.Spec.TuningOptions.ReloadInterval.Duration >= 1*time.Second {
		    reloadInterval = ci.Spec.TuningOptions.ReloadInterval.Duration
	    }
	    env = append(env, corev1.EnvVar{Name: RouterReloadIntervalEnvName, Value: strconv.Itoa(int(reloadInterval.Seconds()))})
        ...
    }
```
The HAProxy template will not be modified.

### Risks and Mitigation

### Drawbacks

## Design Details

### Open Questions

### Test Plan

### Graduation Criteria

### Upgrade/Downgrade Strategy

### Version Skew Strategy

### Operations Aspects of API Extensions

### Failure Modes

## Implementation History

## Alternatives
