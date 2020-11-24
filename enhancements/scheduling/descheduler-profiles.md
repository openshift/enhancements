---
title: scheduling-profiles
authors:
  - "@damemi"
reviewers:
  - "@soltysh"
  - "@ingvagabund"
approvers:
  - "@soltysh"
  - "@ingvagabund"
creation-date: 2020-11-23
last-updated: 2020-11-23
status: provisional
see-also:
  - "enhancements/scheduling/scheduler-profiles.md"
  - "/enhancements/kube-apiserver/audit-policy.md"
replaces:
superseded-by:
---

# Descheduler Profiles

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes the v1 design for configuring the [Descheduler Operator](https://github.com/openshift/cluster-kube-descheduler-operator)
via API fields which select pre-defined policy configurations, which will then be propagated by the operator to the
descheduler operand.

## Motivation

In order to promote the descheduler operator to GA we would like to define an operator
spec which allows users to easily enable and disable certain descheduling strategies.

From a support perspective, it's important to structure this spec in a way that provides
consistent, stable operation. For that reason we choose to abstract away the raw
[upstream Policy type](https://github.com/kubernetes-sigs/descheduler/#policy-and-strategies)
into predefined arrangements of options.

This will allow users to run the descheduler in ways that suit their needs while ensuring
the settings they run are reasonably maintainable by our team.

### Goals

1. define several descheduler policy profiles that serve different goals based on their enabled strategies
2. define an API field to set which profile(s) are enabled
3. implement logic in the descheduler operator to translate the spec setting to an actual policy consumed by the descheduler

### Non-Goals

* support all possible combinations of descheduler settings and strategies

## Proposal

The list of available upstream Descheduler strategies will be grouped into several
profiles. These profiles will be approximately based on similarities shared by the
strategies within them, for example strategies that deal with affinity will be grouped.

The profiles will also be generally grouped by how derivative their strategies' functions
are from core Kubernetes functionality. For example, node taints are a basic feature of
a cluster so that strategy shouldn't be grouped with LowNodeUtilization, which is a more
abstracted concept implemented by the Descheduler. This also serves the purpose of grouping
strategies by their estimated usage, as users will more likely want lower-level descheduling
configurations than complex, niche approaches. This sets a precedent for adding future
strategies into existing groups as well.

Below are the proposed initial profiles for the currently available descheduling strategies:

* `AffinityAndTaints`: enables `RemovePodsViolatingInterPodAntiAffinity`, `RemovePodsViolatingNodeAffinity`,
and `RemovePodsViolatingNodeTaints`. These are the most basic descheduling strategies and most likely the minimum for
what every user of the Descheduler will want to run. In the future, this could be split into 2 profiles (for hard vs. soft
affinity requirements).

* `TopologyAndDuplicates`: enables `RemovePodsViolatingTopologySpreadConstraint` and `RemoveDuplicates`.
These strategies are focused specifically on spreading pods evenly among nodes.

* `LifecycleAndUtilization`: enables `RemovePodsHavingTooManyRestarts`, `LowNodeUtilization`,
and `PodLifeTime`. These focus on the lifecycle of pods and nodes.

These profiles each serve distinct, unrelated functions so users will not be limited to enabling
just one. There is no risk of interference between the profiles so any combination of them can
be enabled at once.

### User Stories [optional]

#### Story 1

As a sysadmin, I want to ensure that my running pods respect node taints, affinity, and inter-pod
affinity. I enable the `AffinityAndTaints` profile to ensure this.

#### Story 2

As a sysadmin, I have a low risk of affinity and taints changing after my pods are scheduled
but I do want to ensure that they are evenly-distributed among the nodes of the cluster. I also
want to keep node utilization balanced. So I enable both `TopologyAndDuplicates` and `LifecycleAndUtilization`.

### Risks and Mitigations

* this will restrict the configuration options from what is currently available in the descheduler operator, but
since it is currently in tech preview (and the API is only beta) this should not be an issue
* this will improve stability and security by restricting config to only what we are prepared to support

## Design Details

The new field will be added to the existing operator spec:
```go
// KubeDeschedulerSpec defines the desired state of KubeDescheduler
type KubeDeschedulerSpec struct {
	operatorv1.OperatorSpec `json:",inline"`
  ...
  Profiles []DeschedulerProfile `json:"profiles"`
  ...
}

// DeschedulerProfile allows configuring the enabled strategy profiles for the descheduler
// it allows multiple profiles to be enabled at once, which will have cumulative effects on the cluster.
// +kubebuilder:validation:Enum=AffinityAndTaints;TopologyAndDuplicates;LifecycleAndUtilization
type DeschedulerProfile string

var (
	// AffinityAndTaints enables descheduling strategies that balance pods based on affinity and
	// node taint violations.
	AffinityAndTaints DeschedulerProfile = "AffinityAndTaints"

	// TopologyAndDuplicates attempts to spread pods evenly among nodes based on topology spread
	// constraints and duplicate replicas on the same node.
	TopologyAndDuplicates DeschedulerProfile = "TopologyAndDuplicates"

	// LifecycleAndUtilization attempts to balance pods based on node resource usage, pod age, and pod restarts
	LifecycleAndUtilization DeschedulerProfile = "LifecycleAndUtilization"
)
```

This approach will clearly present users with all of their options for configuration
while eliminating the chance of typos and the need for validation against input.

This is a simpler definition, but requires more validation checks and isn't as clear
to the user.

The profiles will be translated to an upstream Descheduler policy enabling them:

* `AffinityAndTaints`:
```yaml
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "RemovePodsViolatingInterPodAntiAffinity":
    enabled: true
  "RemovePodsViolatingNodeTaints":
    enabled: true
  "RemovePodsViolatingNodeAffinity":
    enabled: true
    params:
      nodeAffinityType:
      - "requiredDuringSchedulingIgnoredDuringExecution"
```

* `TopologyAndDuplicates`:
```yaml
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "RemovePodsViolatingTopologySpreadConstraint":
    enabled: true
  "RemoveDuplicates":
    enabled: true
```

* `LifecycleAndUtilization`:
```yaml
apiVersion: "descheduler/v1alpha1"
kind: "DeschedulerPolicy"
strategies:
  "PodLifeTime":
     enabled: true
     params:
       podLifeTime:
         maxPodLifeTimeSeconds: 86400 #24 hours
  "RemovePodsHavingTooManyRestarts":
     enabled: true
     params:
       podsHavingTooManyRestarts:
         podRestartThreshold: 100
         includingInitContainers: true
  "LowNodeUtilization":
     enabled: true
     params:
       nodeResourceUtilizationThresholds:
         thresholds:
           "cpu" : 20
           "memory": 20
           "pods": 20
         targetThresholds:
           "cpu" : 50
           "memory": 50
           "pods": 50
```

**Note:** The `LifecycleAndAutomation` profile contains those strategies which have the
most available parameters for users to tweak, and we must decide on sensible default values
for these parameters (the values above are taken from the upstream Descheduler readme).

### Test Plan

**Note:** *Section not required until targeted at a release.*

The test plan should remain similar to what is currently in place for the Descheduler + Operator.
Ensuring that the operator spec settings are correctly translated to a policy file that is used
by the descheduler will remain the same.

This may complicate how we test individual strategies, as while they are grouped it will be tougher
to distinctly get the expected outcome from a pod that is evictable by multiple strategies (for example,
a test environment designed to get evictions for LowUtilization may also have some pods older than PodLifeTime).
These strategies won't conflict with each other per se, they will only make setting up a specific
test environment more difficult. One option to mitigate this would be breaking up the profiles further, or even
grouping some strategies into their own profiles.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

The new field will be added to the existing v1beta1 API and targeted as a GA alternative to
the existing fields in 4.7.

The existing field will be clearly marked as deprecated and will not serve any function (it will
only be provided to support the transition of existing objects). This is acceptable as the operator
is currently only in tech preview.

When we are able to remove the v1beta1 API (in 3 releases or 9 months, whichever is longer), the v1
replacement will only have the new field.

### Upgrade / Downgrade Strategy

If the current `strategies` field stays supported, there will be no issues during upgrades or downgrades.
If it is removed, upgrading and downgrading the descheduler version will cause it to not recognize the alternative
setting. In this case, all the happens is the descheduler fails to start (and does not affect or rely on other
components).

### Version Skew Strategy

The descheduler is fairly resilient to version skew among components, relying mainly on features which are
already GA upstream.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

* This is more restrictive than the current options for configuration, and limits users to enabling
descheduling potentially with some other strategies that they don't intend.

## Alternatives

* One alternative is adding a `policy` field which would take a raw Descheduler policy config map and simply
pass that to the operand (similar to how scheduler currently works). This exposes much more combinations of
configs than we can reasonably support though, and is counter to the direction we are taking configs like these.