---
title: priority-class-override

authors:
  - "@a-dsouza"
  - "@hasueki"
  
reviewers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"

approvers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"

api-approvers:
  - None

tracking-link:
  - https://github.com/openshift/hypershift/issues/1041

creation-date: 2022-09-23
last-updated: 2023-04-10
---

# Priority Classes

## Summary

This proposal details the plan for enabling the HyperShift Operator to override the default priority classes set by HyperShift.

## Motivation
The goal of this effort is to add support for the Hypershift operator to work with any custom priority classes provided they conform to certain requirements. Currently, HyperShift is programmed to work with a set of default priority classes. This effort will add support for the HyperShift Operator to override the default priority classes to ones provided and specified by management cluster admins.

### User Stories
- As a Service Provider, I want the ability to override priority classes if needed for debugging or any other operational tasks.
- As a Service Provider, I want to ensure the reliability of our service offering for our paying customers.
- As a Service Provider, I want to be able to provide specific value/name overrides for the default priority classes utilized by HyperShift.
- As a Service Consumer, I want to have confidence in the reliability of the service so my applications are not disrupted.
- As IBM, I want to ensure priority class adjustments are supported for HyperShift-based clusters to maintain our operation of Kubernetes/OpenShift offerings at scale.

### Goals
- Define how new priority classes are created/provided on the management cluster--user-provided by management cluster admins.
- Define which control plane components will benefit from priority class adjustments.
- Ensure priority class override is available by default.
- Define sensible priority class recommendations.
  - ETCD should always have a higher priority class to account for unwarranted evictions.

### Non-Goals
- HO will not adjust any pod resource request and limits.
- There will be no dynamic adjustment of priority classes included with this phase--this will strictly be propagated through manual actions.
- Overriding the `hypershift-operator` priority class will not apply to this enhancement. 


## Proposal
HyperShift currently creates a static set of default priority classes. This enhancement will add support for providing custom priority classes for the underlying control plane workloads. By utilizing HC annotations, we avoid any API additions for this enhancement at this time. 
The decision to utilize override priority classes can be made at any time by adding the annotation and providing a value for the priority class name. This will effectively propagate the provided priority class name to the various components depending on the annotation. 
Priority classes provided as an override must conform to the specifications listed in the [Implementation Details/Notes/Constraints](#Implementation-details/Notes/Constraints) section of this enhancement document.

### Workflow Description
TBD
### API Extensions
None

### Implementation Details/Notes/Constraints
This effort will largely focus on Hypershift's ability to override priority classes for underlying workloads.
- The desired override priority classes must exist on the kube management cluster. Some details may be different based on management cluster use cases.
  - Priority classes will be defined by unique `Name` and `Value` fields. These may need to be read by external microservices, so the naming convention is important here. The `Name` must follow these general guidelines:
    - contain no more than 253 characters
    - contain only lowercase alphanumeric characters, '-' or '.'
    - start with an alphanumeric character
    - end with an alphanumeric character
    - cannot be prefixed with `system-`
  - The integer `Value` of the priority class can be any 32-bit integer value smaller than or equal to 1 billion. Larger numbers are reserved for critical system Pods that should not normally be preempted or evicted.
    - Values are relatively assigned to priority classes based on management cluster, but it’s up to the service provider to determine values. These values are not universally set and can differ between different customer environments.
- The HC will have three new user-provided annotations for the three priority classes currently used by HyperShift control planes:
  - `hypershift.openshift.io/control-plane-priority-class`
  - `hypershift.openshift.io/api-critical-priority-class`
  - `hypershift.openshift.io/etcd-priority-class`
- The annotations expect the name of the override priority class to be provided as the value. 
- Override criteria:
  - Adding the `hypershift.openshift.io/control-plane-priority-class` annotation in the HC metadata will override any components with the `hypershift-control-plane` priority class.
  - Adding the `hypershift.openshift.io/api-critical-priority-class` annotation in the HC metadata will override any components with the `hypershift-api-critical` priority class.
  - Adding the `hypershift.openshift.io/etcd-priority-class` annotation in the HC metadata will override any components with the `hypershift-etcd` priority class.
- Technical implementation details may resemble:
  - The `hostedcluster_controller` will reconcile the specified override priority classes from the HC annotations and mirror them for the HCP resource.
  - The CPO will reconcile the new override priority classes from the HCP spec, and propagate them to the various components determined by the override criteria.
  - The `DeploymentConfig` struct will be used to propagate changes of priority classes.

### maxSurge
At the moment, we are mitigating a Kubernetes rollout bug that can take down the service by setting `maxSurge` to `0`. Due to the bug, we experience an outage if a pod is restarted for any reason during a rollout, and since overriding priority classes initiates a rollout, it could potentially lead to an outage. 

As a temporary workaround, we've adjusted `maxSurge` values for all HA control plane deployments to `0`.
PR: https://github.com/openshift/hypershift/issues/1568

We'll need to re-evaluate if this needs to be reverted if/when the Kubernetes bug is resolved. 
Kubernetes issue: https://github.com/kubernetes/kubernetes/issues/108266

### multus-admission-controller
Introduced to the control plane in v4.12, the `multus-admission-controller` is a deployment that's within scope of this enhancement. Currently, the `multus-admission-controller` is managed by the `cluster-network-operator`, which also sets its priority class. Since `multus` isn't managed by HyperShift, it's not possible to override the priority class since CNO will squash any changes made.
An issue has been opened against CNO to resolve this blocker: https://issues.redhat.com/browse/OCPBUGS-7942.



### Risks and Mitigations
N/A.
### Drawbacks
N/A.

## Design Details

### Open Questions [optional]
TBD
### Test Plan
TBD
### Graduation Criteria
TBD
#### Dev Preview -> Tech Preview
TBD
#### Tech Preview -> GA
TBD
#### Removing a deprecated feature
TBD
### Upgrade / Downgrade Strategy
TBD
### Version Skew Strategy
TBD
### Operational Aspects of API Extensions
TBD
#### Failure Modes
TBD
#### Support Procedures
TBD
## Implementation History
TBD
## Alternatives
TBD
## Infrastructure Needed [optional]
TBD
