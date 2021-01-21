---
title: Third Pod Priority Class
authors:
  - "@lilic"
reviewers:
  - "@bparees"
  - "@bbrowning"
  - "@derekwaynecarr"
approvers:
  - "@bparees"
creation-date: 2021-21-01
last-updated: 2021-21-01
status: implemented
replaces:
superseded-by:
---

# Third Pod Priority Class 

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

By default OpenShift has two reserved priority classes for critical system pods to have guaranteed scheduling, this
proposal suggests the need for a third less critical priority class which would be used for user facing features we ship
with OpenShift, for example user workload monitoring pods, that are not system critical but should have a higher
scheduling priority than any user workloads.

## Motivation

Currently OpenShift monitoring ships with two workloads, user and cluster monitoring. These include Prometheus pods,
they both have the same priority classes set. What can and actually did happen, is that with not enough resources, user
workload Prometheus pods were scheduled in favour of the cluster monitoring Prometheus pods. This should not happen, as
cluster monitoring is system critical component, whereas user monitoring has less priority. 

### Goals

1. Describe the current state of pod priority classes in OpenShift.
2. Introduce a new third priority class to be used within OpenShift by any non critical workloads.

### Non-Goals

Out of scope of this proposal is solving user specific priority classes.

## Proposal

Priority indicates the importance of a Pod relative to other Pods. If a Pod cannot be scheduled, the scheduler tries to
preempt (evict) lower priority Pods to make scheduling of the pending Pod possible.

By default OpenShift has two reserved priority classes for critical system pods to have guaranteed scheduling:
(following is from our docs [1]):
- `system-cluster-critical` - This priority class has a value of 2000000000 (two billion) and is used with pods that are
  important for the cluster. Pods with this priority class can be evicted from a node in certain circumstances.  For
  example, pods configured with the system-node-critical priority class can take priority. However, this priority class
  does ensure guaranteed scheduling.  Examples of pods that can have this priority class are fluentd, add-on components
  like descheduler, and so forth.
- `system-node-critical` - This priority class has a value of 2000001000 and is used for all pods that should never be
  evicted from a node. Examples of pods that have this priority class are sdn-ovs, sdn, and so forth.

As mentioned before these two are not enough and this proposal wants to introduce a third priority class:
`user-critical`. This would be used by any pods that are important for user facing OpenShift features but are not deemed
system critical. Example of pods include user workload monitoring and the OpenShift Serverless control and data planes 

### Risks and Mitigations

N/A

## Design Details

New priority class would be created by the component that creates the two existing priority classes with name
`user-critical` and value 1000000000 (1 billion) to convey it is not system or node critical.

Adding a prefix of `openshift-` to the `user-critical` class name would make it clear to users that this is reserved for
user workloads that are managed by OpenShift, but it goes against the current pattern as no one of our existing classes
have a reserved prefix. To change this would require to change approx 170+ instances of the existing class names. 

After new priority class exists, user workload monitoring pods would set this class for its pods. 

## Implementation History

This is already implemented and used in OpenShift monitoring stack. It is created by the cluster-version-operator, and the
manifest is located in the cluster-monitoring-operator manifests bundle.

The resource looks like this:

```yaml
kind: PriorityClass
apiVersion: scheduling.k8s.io/v1
metadata:
  name: openshift-user-critical
  annotations:
    include.release.openshift.io/self-managed-high-availability: 'true'
    include.release.openshift.io/single-node-developer: 'true'
  managedFields:
    - manager: cluster-version-operator
value: 1000000000
description: >-
  This priority class should be used for user facing OpenShift workload pods
  only.
preemptionPolicy: PreemptLowerPriority
```

Any component can use this, as long as they are installed after the creation of the PriorityClass.

## Drawbacks

N/A

## Alternatives

Each component creates its own priority classes, this would lead to confusion from user perspective as there would be x
number of classes, but also possible problems with some lower priority components being scheduled over higher system
critical ones, if we let each component pick the value of priority.

Other one is to just not have a new priority class for the user facing OpenShift workloads, problem with that is that
users can create priority classes that would schedule this in favour of the OpenShift workloads.

[1]:
https://docs.openshift.com/container-platform/3.11/admin_guide/scheduling/priority_preemption.html#admin-guide-priority-preemption-priority-class
