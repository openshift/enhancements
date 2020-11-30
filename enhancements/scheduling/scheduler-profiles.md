---
title: scheduling-profiles
authors:
  - "@damemi"
reviewers:
  - "@soltysh"
  - "@ingvagabund"
  - "@deads2k"
approvers:
  - "@soltysh"
  - "@ingvagabund"
  - "@deads2k"
creation-date: 2020-11-11
last-updated: 2020-11-11
status: provisional
see-also:
  - "https://github.com/kubernetes/enhancements/blob/master/keps/sig-scheduling/624-scheduling-framework/kep.yaml"
  - "/enhancements/kube-apiserver/audit-policy.md"
replaces:
superseded-by:
---

# Scheduling Profiles

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes a new field in the `schedulers.config.openshift.io/v1` API to allow
configuration of enabled and disabled scheduler plugins via defined profiles.

This will simplify how users configure pod scheduling beyond the default set of enabled plugins in the
default scheduler managed by the OpenShift operator. 

## Motivation

As of Kubernetes 1.19, plugin profile configuration has replaced Policy files for the scheduler.
We currently support setting scheduler predicates and priorities through the Policy API, and need
to offer similar configuration with the new API in order to maintain a level of feature parity.

In addition, the existing Policy API has been deprecated upstream (and is planned to be removed completely in 
Kubernetes 1.23, see https://github.com/kubernetes/kubernetes/issues/92143)

However, the new plugin API is more expansive than the old API and provides more configuration possibilities
than is necessary for most customers or reasonable to support. For those reasons we propose an
abstracted representation of the most common plugin configurations for users to configure.

### Goals

1. define several scheduler plugin configurations that represent the most common use-cases
2. add a `profile` API field to `schedulers.config.openshift.io/v1` to specify which predetermined
profile to use
3. enable these profiles by kube-scheduler-operator

### Non-Goals

* fine configuration of individual scheduler plugins
  * This differs from the current scheduler Policy config, which allows individual control over predicates and priorities
* default support for custom schedulers or out-of-tree plugins

Note that these non-goals are only for the default cluster scheduler managed by OpenShift, and should not imply any 
inherent platform restrictions on secondary or other custom schedulers users choose to deploy on their own.

## Proposal

This enhancement proposes a new field in `schedulers.config.openshift.io/v1` named `profile`. This field
specifies which of the following plugin configurations (as scheduler profiles to enable on the default cluster
scheduler.

Note that for any of these profiles, the definition is based on intent and the underlying implementation may change 
between releases to maintain that original intent (corresponding to any changes in the upstream scheduler's plugins).

* `LowNodeUtilization`: this profile attempts to spread pods evenly across nodes to get low resource usage per node. 
This will use the default scheduler profile as defined in the kube-scheduler's internal registry.

* `HighNodeUtilization`: this attempts to pack as many pods as possible on to as few nodes as possible, to minimize node 
count with high usage per node. This will disable `NodeResourcesLeastAllocated` and enable `NodeResourcesMostAllocated` to enable
bin-packing of workloads onto nodes.

* `NoScoring`: this is a "low latency" profile which strives for the quickest scheduling cycle by disabling all score plugins. 
Score plugins are inherently non-critical so their exclusion should be safe and provide decreased scheduling time. This 
may sacrifice better scheduling decisions for faster ones.

### User Stories [optional]

#### Story 1

As a sysadmin, I want my workloads to be packed tightly onto nodes rather than the default behavior of spreading workloads 
out among nodes. I enable the `HighUtilization` profile.

#### Story 2

As a sysadmin, I want to schedule my workloads as quickly as possible without regard for any soft preference of specific nodes. 
I enable the `NoScoring` profile to achieve this.

### Implementation Details/Notes/Constraints [optional]

The new field will be set in the operator config:
```yaml
kind: Scheduler
apiVersion: config.openshift.io/v1
spec:
  ...
  profile: Default | HighUtilization | NoScoring | ...
```

The available values will be constrained to those defined above by API validation, but there is the possibility of adding more 
options if significant use cases arise for different configurations.

### Risks and Mitigations

* Security and stability risks should be minimal, as this method of configuration is actually more restrictive than what is 
currently supported for the scheduler
* However, due to that there is a risk that this regresses from the current depth of configuration supported for customers with 
very specific needs (based on metrics gathered, there are likely very few clusters that would be impacted in this way)
* Transition from the old Policy API will provided by supporting the new Profile field alongside the old API (at least until Policy is 
removed upstream, currently targeting k8s 1.23) with the restriction that only one may be set at a time.

## Design Details

The raw scheduler config corresponding to the set value will be merged into the existing postbootstrap component config 
for the scheduler by the operator (except in the case of `Default`, where no raw config is necessary).

* `HighUtilization`: disables plugins favoring balanced/low utilization and enables plugins which favor higher utilization nodes
```yaml
apiVersion: kubescheduler.config.k8s.io/v1beta1
kind: KubeSchedulerConfiguration
...
profiles:
  - schedulerName: default-scheduler
    plugins:
      score:
        disabled:
        - name: "NodeResourcesLeastAllocated"
        enabled:
        - name: "NodeResourcesMostAllocated"
```

* `NoScoring`: disables all preScore/Score plugins for lower latency scheduling
```yaml
apiVersion: kubescheduler.config.k8s.io/v1beta1
kind: KubeSchedulerConfiguration
...
profiles:
  - schedulerName: default-scheduler
    plugins:
      preScore:
        disabled:
        - name: "*"
      score:
        disabled:
        - name: "*"
``` 

### Test Plan

**Note:** *Section not required until targeted at a release.*

Propagation of the expected profile settings can be verified through operator e2e's that check the right values are in 
the config map that is currently mounted into the scheduler. Manual testing can also verify that these values are set as 
expected through inspection of this config and logs from the scheduler.

Testing that pods are scheduled to expected nodes is outside the scope of this enhancement, as that would be more focused 
on testing of specific plugins which is difficult as the combination of plugins along with fluctuations in cluster states makes 
not all scheduling decisions strictly deterministic.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

This field should technically be considered "beta" or "tech preview" as long as the underlying upstream API is in beta. 
It should be marked as GA following the same promotion upstream (approximately upstream 1.23 -> OpenShift 4.10).

The existing Policy field should immediately be marked as deprecated (effective in 4.7) and removed in the release that 
Profiles is promoted to GA. This removal may come as a requirement in the rebase if it is also removed upstream (or require 
a carry patch to continue support for this API).

### Upgrade / Downgrade Strategy

Upgrades may be an issue only for users of the old Policy API in the version that it is removed. We will provide both 
options until the API is removed upstream (at least) for this reason. Otherwise, users will need to determine a reasonable 
equivalent for their scheduler configuration out of the available options we provide (if applicable).

Because this approach manages the raw profile values in our own operator, this should be more resilient to version skew 
in the upstream scheduler plugin definitions and steps can be taken to react to any changes in the underlying config 
before the release which would affect users.

If more profile options are added, they may not be available in earlier versions causing downgrades to assume the default 
scheduler configuration unless new profiles are backported.

### Version Skew Strategy

Many of the scheduler plugins are self-contained features, and the ones that do interact with or depend on matching features 
in other components can be easily gated through this design. In general, minor version skew between the scheduler and other 
components is not an issue.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

* This is a more restrictive configuration option for the scheduler than what is currently supported. If users have a requirement 
for very specific or unique default scheduler configs they may not be able to achieve the same settings for the cluster
* Some newer scheduler plugins have their own configurable arguments (such as `PodTopologySpread`, which provides cluster defaults). 
Users will not be able to set these parameters (however with enough demand we could provide them as additional operator spec settings).

## Alternatives

* The inclusion of a `profiles` field which behaves more similarly to the current `policy` field in the scheduler operator, with users 
providing a configmap containing the raw scheduler profiles themselves (see the [docs](https://docs.openshift.com/container-platform/4.1/nodes/scheduling/nodes-scheduler-default.html) 
on this). This has the drawbacks of maintainability and cluster stability with the tradeoff of more in-depth tuning and newer features.