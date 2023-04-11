---
title: priority-class-dynamic-adjustment

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

This proposal details the plan for enabling the HyperShift Operator to adjust the priority class of master control plane components to be higher if the customer is using more worker nodes in their cluster. 

## Motivation
The goal of this effort is to add support for the HyperShift Operator to ensure minimal disruption for customer control plane workloads that generate higher revenue. Priority classes on the control plane workloads can be dynamically adjusted based on the number of worker nodes associated to the HC.
This helps ensure that control plane pods of higher priority clusters don't get evicted/rescheduled before the pods of a lower priority cluster. Dynamically adjusting priority classes is something IBM Cloud has been doing to maintain and operate their Kubernetes/OpenShift service at scale.

### User Stories
- As a Service Provider, I want to ensure reliability of our service offering for our paying customers.
- As a Service Provider, I want to have priority classes dynamically adjusted based on the amount of worker nodes being used by a cluster.
- As a Service Provider, I want the ability to override priority classes if needed for debugging or any other operational tasks.
- As a Service Consumer, I want to have confidence in the reliability of the service so my applications are not disrupted.
- As IBM, I want to ensure priority class adjustments are supported for HyperShift-based clusters to maintain our operation of Kubernetes/OpenShift offerings at scale.

### Goals
- Define how new priority classes are created on the mangement cluster.
  - Option 1: Default sets to be rendered by HyperShift CLI tool at install time.
  - Option 2: User-provided by management cluster admins.
- Expose priority class name being used for a given HC to integrate with external systems.
  - e.g. IBM adjusts resource requests and limits based on priority class being set.
- Define how HO will determine "priority" for a given HC.
  - e.g. IBM determines this based on customer investment (cluster size/worker node count).
- Ensure dynamic priority class allocation is available by default
  - Clients should be able to opt-out of employing the features offered through this enhancement.  
- Define sensible priority class recommendations.
  - ETCD should always have a higher priority class to account for unwarranted evictions.

### Non-Goals
- HO will not adjust any pod resource request and limits.
- Priority class adjustments to a lower priority value will not be supported.

## Proposal
HyperShift currently creates a static set of default priority classes. We propose to add support for dynamically adjusting priority classes for the underlying control plane workloads. This will help to ensure the HCs with higher level of usage/investment will be of more importance when scheduling workloads and mitigate preemptions. 
IBM's implementation of priority classes involves modifying the priority class of the master control plane components to be higher for customers using more worker nodes in their cluster. This behavior can be beneficial in HyperShift in general, to help ensure that clusters using less worker nodes get evicted/rescheduled before a cluster that has more worker nodes--a higher priority class.

The proposed default sets of priority classes rendered by HyperShift CLI at install time may look like:
```yaml
- apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-xs
    value: 1000000
    globalDefault: false
    description: "Used on master control plane resources for clusters with 1-10 workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-etcd-xs
    value: 1500000
    globalDefault: false
    description: "Used on master ETCD resources for clusters with 1-10 workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-s
    value: 2000000
    globalDefault: false
    description: "Used on master control plane resources for clusters with 11-25 workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-etcd-s
    value: 2500000
    globalDefault: false
    description: "Used on master ETCD resources for clusters with 11-25 workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-m
    value: 3000000
    globalDefault: false
    description: "Used on master control plane resources for clusters with 26-50 workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-etcd-m
    value: 3500000
    globalDefault: false
    description: "Used on master ETCD resources for clusters with 26-50 workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-l
    value: 4000000
    globalDefault: false
    description: "Used on master control plane resources for clusters with 51-100 workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-etcd-l
    value: 4500000
    globalDefault: false
    description: "Used on master ETCD resources for clusters with 51-100 workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-xl
    value: 5000000
    globalDefault: false
    description: "Used on master control plane resources for clusters with 101-300 workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-etcd-xl
    value: 5500000
    globalDefault: false
    description: "Used on master ETCD resources for clusters with 101-300 workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-xxl
    value: 6000000
    globalDefault: false
    description: "Used on master control plane resources for clusters with 300+ workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-etcd-xxl
    value: 6500000
    globalDefault: false
    description: "Used on master ETCD resources for clusters with 300+ workers."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-dedicated
    value: 7000000
    globalDefault: false
    description: "Used on master control plane resources for dedicated clusters."
  - apiVersion: scheduling.k8s.io/v1
    kind: PriorityClass
    metadata:
      name: hypershift-etcd-dedicated
    value: 7500000
    globalDefault: false
    description: "Used on master ETCD resources for dedicated clusters."
```

With the availablity of the above priority classes, HO can be enhanced to use and override priority classes being on set on the underlying control plane workloads/pods.

### Workflow Description
TBD
### API Extensions
None

### Implementation Details/Notes/Constraints
This enhancement will focus on bolstering HyperShift's capability by introducing dynamic adjustment of priority classes based on the workload's size. This will directly be tied to the HC's total node count.
- Add proposed default sets of priority classes to the cmd/install assets for HyperShift Operator, and enable the utilization of these custom priority classes using a CLI flag. 
  - Priority classes will be defined by unique `Name` and `Value` fields. These may need to be read by external microservices, so the naming convention is important here. The `Name` must follow these general guidelines:
    - contain no more than 253 characters
    - contain only lowercase alphanumeric characters, '-' or '.'
    - start with an alphanumeric character
    - end with an alphanumeric character
    - cannot be prefixed with `system-`
  - The integer `Value` of the priority class can be any 32-bit integer value smaller than or equal to 1 billion. Larger numbers are reserved for critical system Pods that should not normally be preempted or evicted.
    - Values are relatively assigned to priority classes based on management cluster, but it’s up to service provider to determine values. These values are not universally set and can differ between different customer enviroments.  
- In the case of user-provided priority classes with dynamic adjustment, we will need a way to determine the priority class hierarchy. This could either be a simple naming convention or there may be a need for an annotation/label to be attached to said priority classes. 
- User-provided prirority classes for dynamic adjustmnet must mimic the structure of the default priority classes provided in the Proposal section of this document. 

- Proposed logic for HO to calculate total node count for a specific HC:
  - Within the Nodepool controller's reconcile functions, list all the nodepools. 
  - Filter through nodepools to gather those associated with the hosted cluster in question. 
  - Calculate the total node count for the cluster and override the priority class based on it will be added to `hypershift-operator/controllers/util`. 
    - Assign a priority class by comparing the node count against the predetermined upper bounds of priority classes. 
    - Override the priority class specification of the HC.
  - A change of the priority class specification in the HC will be propogated to the HCP.
  - The HyperShift Operator and control plane operator will consequently work to propogate the change in priority class to all the applicable components for the workload.

#### Dedicated clusters
We need to ascertain the usage of the `*-dedicated` priority classes. These are applied to special-case customer clusters, and when this is applied, we essentially ignore/remove resource requests and limits for control plane components. We have two options for this iplementation:

- If this isn't something we'd like to be implemented universally, make this particular functionality platform specific to the IBM platform type.
- If we'd like this to be implemented universally, we can implement it as it is, albeit with a few changes to the naming conventions used. 

### maxSurge
At the moment, we are mitigating a Kubernetes rollout bug that can take down the service by setting `maxSurge` to `0`. Due to the bug, we experience an outage if a pod is restarted for any reason during a rollout, and since overriding priority classes initiates a rollout, it could potentially lead to an outage. 

As a temporary workaround, we've adjusted `maxSurge` values for all HA control plane deployments to `0`.
PR: https://github.com/openshift/hypershift/issues/1568

We'll need to re-evaluate if this needs to be reverted if/when the Kubernetes bug is resolved. 
Kubernetes issue: https://github.com/kubernetes/kubernetes/issues/108266

### multus-admission-controller
Introduced to the control plane in v4.12, the `multus-admission-controller` is a deployment that's within scope of this enhancement. Currently, the `multus-admission-controller` is managed by the `cluster-network-operator`, which also sets its priority class. Since `multus` isn't managed by HyperShift, it's not possible to override the priority class since CNO will squash and changes made.
An issue has been opened against CNO to resolve this blocker: https://issues.redhat.com/browse/OCPBUGS-7942.
This dedicated priority class will have a higher `Value` than any other priority class.

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
