---
title: rationalizing-configuration
authors:
  - "@dhellmann"
  - "@mrunalp"
  - "@browsell"
  - "@MarSik"
  - "@fromanirh"
reviewers:
  - "@deads2k"
  - "@sttts"
  - maintainers of PAO
approvers:
  - "@markmc"
  - "@derekwaynecarr"
creation-date: 2021-04-26
status: implementable
see-also:
  - "/enhancements/management-workload-partitioning.md"
---

# Rationalizing Configuration of Workload Partitioning

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The initial iteration of workload partitioning focused on a short path
to a minimum viable implementation. This enhancement describes the
loose ends for preparing the feature for GA at a high level, and
explains the set of other design documents that need to be written
separately during the next iteration.

## Motivation

The management workload partitioning feature introduced in OCP 4.8
enables pod workloads to be pinned to the kubelet reserved cpuset. The
feature is enabled at install time and requires the reserved set of
CPUs as input. That configuration is currently passed in via a
manually created manifest containing a machine config resource. The
set of CPUs used for management workload partitioning has to align
with the reserved set configured by the performance-addon-operator
(PAO) during day 2. For OCP 4.8, the onus is on the user to ensure
these align, there is no interlock.

The current implementation is prone to user error, because it has two
entities configuring and managing the same data. This design document
describes phase 2 of the implementation plan for workload partitioning
to improve the user experience when configuring the feature.

### Goals

1. Describe the remaining work at a high level for architectural
   review and planning.
2. Document the division of responsibility between workload management
   in components in the OpenShift release payload and PAO.
3. List the other enhancements that need to be written, and their
   scopes.

### Non-Goals

1. Resolve all of the details for each of the next steps. We're not
   going to talk a lot about names or implementation details here.

## Proposal

### API

The initial implementation is limited to single-node deployments. To
support multi-node clusters, we need to solve a couple of concurrency
problems. The first is providing a way for the admission hook
responsible for mutating pod definitions to reliably know when the
feature should be enabled. The admission hook cannot store state on
its own, so we need a new API to indicate when the feature is
enabled.

We eventually want to extend the partitioning feature to support
non-management workloads to include workload types defined by cluster
admins. For now, however, we want to keep the interface as simple as
possible.  Therefore, the new API will be read-only, and only used so
that the admission hook (and other consumers) can tell when workload
partitioning is active.

The new API will need a controller to manage it. We propose to add a
controller to the existing cluster-config-operator, to avoid adding
the overhead of another image, pod, etc. to the cluster.

We propose that the API be owned by core OpenShift, and defined in the
`openshift/api` repository. A separate enhancement will be written to
describe the API and controller in more detail.

### Owner for Enablement

Workload management needs two node configuration steps which overlap
with work PAO is doing today:

* Set reserved CPUs in kubelet config and enable the CPU manager
  static policy
* Set systemd affinity to match the reserved CPU set

PAO already has logic for managing CPU pools and configuring kubelet
and CRI-O. Since it already does a lot of what is needed to enable
workload partitioning, we propose to extend it to handle the
additional configuration currently being done manually by users,
including the machine config with settings for kubelet and CRI-O.

The effect of this decision is that the easiest way to use the
workload partitioning feature will be through the
performance-addon-operator. This means that the implementation of the
feature is split across a few components, and not all of them are
delivered in all clusters, but it avoids duplicating a lot of the work
already done in the PAO.

Workload partitioning is currently enabled as part of installing a
cluster. This provides predictable behavior from the beginning of the
life-cycle, and avoids extra reboots required to enable it in a
running cluster (an important consideration in environments with short
maintenance windows). To continue to support enabling the feature this
way, and to simplify the configuration, we propose to extend the PAO
with a `render` command, similar to the one in other operators, to
have it generate manifests for the OpenShift installer to consume.

### Future Work

1. Write an enhancement to describe the API the admission hook will
   use to determine when workload partitioning is enabled.
2. Write an enhancement to describe the changes in
   performance-addon-operator to manage the kubelet and CRI-O
   configuration when workload partitioning is enabled, including the
   `render` command.
3. Write an enhancement to describe an API for enabling workload
   partitioning in an existing cluster.

### User Stories

N/A

### Risks and Mitigations

There is some risk of shipping a feature with part of the
implementation in the OpenShift release payload but the enabling tool
delivered separately. PAO is considered to be a "core" OpenShift
component, even though it is delivered separately. There is not a
separate SKU for it, for example. The PAO team has been working
closely with the node team on the implementation of this feature, so
we do not anticipate any issues delivering the finished work in this
way.

## Design Details

### Test Plan

The other enhancements will provide details of the test plan(s)
needed.

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History

- https://issues.redhat.com/browse/OCPPLAN-6065
- https://issues.redhat.com/browse/CNF-2084

## Drawbacks

None

## Alternatives

We could move more of the PAO implementation into the release payload,
so that users who want the workload partitioning feature do not need
another component. This would either duplicate a lot of the existing
PAO work or make future deliver of PAO updates more complicated by
tying them to OpenShift releases.
