---
title: machine-health-check-pause-on-cluster-upgrade

authors:
  - @slintes

reviewers:
  - @JoelSpeed
  - @michaelgugino
  - @beekhof

approvers:
  - @JoelSpeed
  - @michaelgugino
  - @beekhof

creation-date: 2021-07-14  
last-updated: 2021-07-14  
status: implementable  

see-also:
  - "enhancements/machine-api/machine-health-checking.md"
---

# Pause MachineHealthChecks During Cluster Upgrades

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Automatically prevent unwanted remediation during cluster upgrades caused by temporarily unhealthy nodes.

## Motivation

During cluster upgrades nodes get temporarily unhealthy, which might trigger remediation when a machineHealthCheck
resource is targeting those nodes, and they don't get healthy before the configured timeout. This remediation delays
the cluster upgrade significantly especially on bare metal clusters, and causes needless interruption of customer
workloads while they are transferred to other nodes.

### Goals

- Automatically pause all machineHealthChecks during cluster upgrades
- Automatically unpause all machineHealthChecks when cluster upgrade finished

### Non-Goals

tbd

## Proposal

Introduce a new controller, which watches the clusterVersion resource, and pauses all machineHealthChecks when the
clusterVersion status indicates an ongoing cluster upgrade. The controller will unpause the machineHealthChecks when the
clusterVersion status indicates that the upgrade process finished.

### User Stories

- As a cluster admin I don't want unneeded reboots of nodes during cluster upgrades
- As a cluster admin I don't want to manually pause machineHealthChecks during cluster upgrades
- As a user I don't want my workloads to be transferred to new nodes more often than needed during cluster upgrades

### Implementation Details/Notes/Constraints [optional]

The clusterVersion object has several conditions in its status. Example:

```yaml
  status:
    conditions:
    - lastTransitionTime: "2021-07-05T20:53:02Z"
      message: Done applying 4.9.0-0.nightly-2021-07-05-092650
      status: "True"
      type: Available
    - lastTransitionTime: "2021-07-06T12:03:02Z"
      status: "False"
      type: Failing
    - lastTransitionTime: "2021-07-05T20:53:02Z"
      message: Cluster version is 4.9.0-0.nightly-2021-07-05-092650
      status: "False"
      type: Progressing
    - lastTransitionTime: "2021-07-05T20:05:15Z"
      message: 'Unable to retrieve available updates: currently reconciling cluster
        version 4.9.0-0.nightly-2021-07-05-092650 not found in the "stable-4.8" channel'
      reason: VersionNotFound
      status: "False"
      type: RetrievedUpdates
    ...
```

The machineHealthCheck supports pausing by setting an annotation on it:

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineHealthCheck
metadata:
  name: example
  namespace: openshift-machine-api
  annotations:
    cluster.x-k8s.io/paused: ""
  ...
```

The new controller will add the pause annotation on the machineHealthChecks when the clusterVersion's `Progressing`
condition switches to `"True"`. When that condition switches back to `"False"`, the controller will remove the
annotation.

The new controller will be implemented in the `openshift/machine-api-operator` repository, and be deployed in the
`machine-api-controllers` pod, both alongside the machine-healthcheck-controller.

### Risks and Mitigations

Nothing we are aware of today

## Design Details

### Open Questions [optional]

1. Should admins be able to enable / disable this feature? (IMHO no, I don't see a usecase when this has disadvantages)

2. Today there is no way to coordinate multiple users of the pause feature of machineHealthChecks. The value of the
annotation is supposed to be empty. This means that the annotation might already be set by someone else when the new
controller want to set it as well. That introduces the question what to do in this case after cluster upgrade: remove
the annotation, or keep it? In the latter case: how to keep track whether the controller set the annotation or not (e.g.
in another annotation?)

### Test Plan

TODO (need to check existing e2e tests, and if we extend them with a test case for this, and how much extra load to CI
it would add)

### Graduation Criteria

TODO I have no good idea yet
- if this is alpha, beta or stable (do we need this at all, it doesn't introduce a new API)?
- if this is Dev Preview, Tech Preview or GA?

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

The new controller lives in the machine-api-operator image so the upgrades will be driven by the CVO which will fetch
the right image version as usual.
The controller does not offer any API or configuration. The controller can be replaced with an older or newer
version without side effects or risks.

### Version Skew Strategy

This controller has some expectation to other APIs, which are the clusterVersion status conditions, and the
machineHealthCheck pause annotation. In case any of those changes their API (new condition name or annotaton key), the
controller will stop to work. This will catched in CI.

## Implementation History

## Drawbacks

## Alternatives

- Not having this feature at all: cluster admins could pause machineHealthChecks manually, but that is unneeded work,
  and too easy to forget.
- Implement this in the machine healthcheck controller: this would be an obviuos choice on a first look. But since the
  clusterVersion resource is Openshift specific, it would further diverge Openshift machine API and upstream Cluster
  API. The goal is to align them over time.

## Infrastructure Needed [optional]

n/a
