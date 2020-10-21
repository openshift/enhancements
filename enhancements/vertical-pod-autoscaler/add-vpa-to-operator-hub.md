---
title: vpa-available-on-operatorhub
authors:
  - "@joelsmith"
reviewers:
  - "@rphillips"
approvers:
  - "@derekwaynecarr"
creation-date: 2019-10-08
last-updated: 2020-08-17
status: implemented
---

# Make Vertical Pod Autoscaler available on OperatorHub

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions

1. Which global VPA controller options should we expose? The proposed set of options are as follows:

 	- From the [admission-controller](https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/admission-controller/main.go#L47-L58): none
	- From the [updater](https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/updater/main.go#L39-L48)  (see also [here](https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/updater/priority/update_priority_calculator.go#L40-L42)): none
	- From the [recommender](https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/recommender/main.go#L35-L52)    (see also [here](https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/recommender/logic/recommender.go#L26-L28) and    [here](https://github.com/kubernetes/autoscaler/blob/master/vertical-pod-autoscaler/pkg/recommender/routines/recommender.go#L43-L45)): `recommendation-margin-fraction`, `pod-recommendation-min-cpu-millicores` and
`pod-recommendation-min-memory-mb`

For any other options which are not configurable, the VPA Operator will be opinionated.

## Summary

The upstream Kubernetes project Vertical Pod Autoscaler (VPA) monitors pods and
recommends resource limits based upon usage patterns. These recommendations can
be used to update pods with the recommended limits either on pod restart, or
via in-place pod updates (future).

This enhancement will provide the Vertical Pod Autoscaler for OpenShift
clusters via OperatorHub to be deployed by Operator Lifecycle Manager (OLM).

## Motivation

The VPA provides one means of having resource limits that more closely match
the actual needs of a pod. With more accurate limits, a node can be better
utilized by not having excess capacity reserved for a pod that doesn't need it.
These reasons will drive the demand for VPA from end users and cluster
administrators.

Providing VPA via OLM will provide a nice installation and management
experience for end users who choose to deploy this functionality.

### Goals

- Provide an excellent Vertical Pod Autoscaler installation experience
- Deploy working VPA controllers
- Allow for seamless automatic upgrades to future versions of VPA
- Provide means of controlling VPA configuration parameters

### Non-Goals

- ???

## Proposal

The following specific changes are proposed.

1. Create a `ClusterServiceVersion` manifest for OLM use in properly deploying
   [/openshift/vertical-pod-autoscaler-operator]
2. Create a `CustomResourceDefinition` for creation of instances the VPA
   controllers. Custom resources of this type will be consumed the Vertical Pod
   Autoscaler Operator to deploy the VPA controllers.
3. Create a default instance of the custom resource for the VPA controllers if
   one does not already exist so that a deployed operator results in running
   VPA controllers.

### User Stories

Generally speaking, the proposed capability provides a means of improving
resource utilization on nodes in the cluster. This section describes two
possible use cases.

#### Workloads scoped with insufficient resources

> As an OpenShift user, I want my pod to be able to reserve more CPU or
> memory if, over time, its requirements change.

When a workload's resource requirements increase (perhaps because of a change
in a utilization pattern), the end user may want the resource requirements to be
adjusted. For example, without the proposed capability, a pod using too much memory will
be terminated. The user may prefer that his pod limits instead be increased
to match the resource needs and thus be able to run during periods of high
demand.

#### Clusters with reserved, unused CPU and Memory

> As an OpenShift administrator, I want my cluster to be better utilized so that I
> can run my workloads with fewer nodes.

If workloads reserve many more CPU resources than they need, the excess CPU
resource will go unused. The proposed capability will monitor what such
workloads are actually using and can adjust the resource requirements down so
that the extra capacity is available to other workloads, thus allowing for
more efficient use of resources.

#### VPA Controllers to Run on Worker Nodes

> As an OpenShift customer running in a managed environment, I want to be able
> to use VPA on my cluster even though I can't schedule workloads on infra or
> master nodes.

The VPA operator and the controllers it deploys should have the ability to be
configured with node selectors so they will be scheduled on the nodes of the
customer's choice.

### Risks and Mitigations

We may find that the VPA Updater or the VPA Admission Controller are not
sufficiently stable for a GA release. For example, if running them causes
managed workloads to flap or be improperly resource-constrained, then
the Updater and Admission Controller might need to be disabled.

As a mitigation for this scenario, we will provide an easy way to run VPA
in recommendation-only mode.  In this mode, only the VPA Recommender
controller will run, which does not alter workloads in any way.

## Design Details

### Test Plan

The VPA Operator will include e2e tests which:

1. Install and run the VPA operator
2. Install and run the VPA controllers

Individual tests will then create a test namespace where they:

1. Create a workload which should be monitored and modified by the VPA
2. Create a VPA custom resource which references the workload
3. Ensure that the VPA acts upon the workload by scaling it up or down
    as appropriate

This individual tests will include:

* Scale up based upon CPU usage
* Scale down based upon CPU usage
* Scale up based upon memory usage
* Scale down based upon memory usage

### Graduation Criteria

#### Tech Preview -> GA 

- More testing, including upgrade and downgrade tests and improved e2e tests
- Sufficient time for feedback

### Upgrade / Downgrade Strategy

Upgrades and Downgrades will performed by OLM.

### Version Skew Strategy

VPA has few dependencies on any components beyond the core components, and is
not currently dependent upon any specific version.

Future: when any such dependencies are introduced, the operator will be made to
detect versions of components the functionality depends upon so that it can
configure controllers to enable or disable features that require certain
versions. For example, we may have the operator disable the in-place update
feature unless Kubelets with support for the feature are detected.

## Implementation History

No major milestones have been reached yet.

## Drawbacks

Until the Kubelet supports in-place adjustment of pod resource requirements,
the VPA must restart a workload for the limit adjustments to take effect. Some
customers may consider this workflow to be too disruptive and may choose not to
deploy the VPA.

## Alternatives

Many workloads support horizontal scaling and could use the Horizontal Pod
Autoscaler (HPA) instead to meet their scalability goals. Not every workload is
easily parallelized and there are cases where the only alternative is to
manually monitor and adjust workload resource requirements.
