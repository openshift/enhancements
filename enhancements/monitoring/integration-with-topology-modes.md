---
title: monitoring-topology-mode
authors:
  - "@lilic"
reviewers:
  - "monitoring-team"
  - "???"
approvers:
  - "monitoring-team"
creation-date: 2021-03-01
last-updated: 2021-03-01
status: implementable
see-also:
  - "/enhancements/cluster-high-availability-mode-api.md"
---

# Monitoring Stack on Single Replica Topology Mode


## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Monitoring stack needs to react based on the newly added single replica topology mode. Plan is to detect whenever
cluster is in single replica topology mode and reduce the monitoring stack footprint by adjusting platform Prometheus
instance to be non Highly Available.

## Motivation

In the 4.8 time frame, OpenShift will introduce a new capabilities field in the Infrastructure API that will inform
operators about the deployment topology. The immediate goal is for operators to adjust the deployment of their operands
depending on whether the infrastructure is HA (at least 3 nodes) or not (single node).

### Goals

- Reduce monitoring footprint by lowering number of replicas

### Non-Goals

- User workload stack or its Prometheus instance is not adjusted
- Reduce memory footprint
- Address CRC requirements (though it might help)
- Tests from monitoring team

## Proposal

To adjust to the single replica mode, in cluster-monitoring-operator we detect the topology mode based on the "cluster"
named instance of `Infrastructure`, if that mode is `SingleReplicaTopologyMode`, we only spin up one instance of
platform Prometheus for that cluster.

### Risks and Mitigations

There are risks that this is not enough to reduce the actual footprint of the monitoring stack. The other thing is that
as a result of this monitoring is no longer highly available but there is no workaround for that as its a single node
cluster.

## Design Details

### Test Plan

N/A Its a non goal for monitoring team, apart from manual testing.

### Graduation Criteria

N/A

### Upgrade / Downgrade Strategy

Upgrades are a non goal right now.

### Version Skew Strategy

N/A

## Implementation History

## Drawbacks


## Alternatives

Skip installing monitoring stack is one alternative, but this is harder to do due to monitoring stack being a core
component of OpenShift, and our CRDs being used by so many other components.
