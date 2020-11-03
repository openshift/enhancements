---
title: fcc-support-in-mc
authors:
  - "@LorbusChris"
reviewers:
  - TBD
  - "@crawford"
  - "@ashcrow"
  - "@cgwalters"
  - "@runcom"
approvers:
  - TBD
creation-date: 2020-08-31
last-updated: 2020-08-31
status: provisional|**implementable**|implemented|deferred|rejected|withdrawn|replaced
see-also:
  - "./ignition-spec-dual-support.md"  
---

# Fedora CoreOS Config support in MachineConfig

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Proposal to add support for Fedora CoreOS Config (FCC) in the Config field of MachineConfig (MC)
resources.

The Config field is of the `runtime.RawExtension` type and can therefore contain arbitrary data.
The machine-config-operator (MCO) currently supports parsing the RawExtension contents
for Ignition spec v2.2, v3.0 and v3.1 configuration. Internally, the configuration from
all MachineConfig objects are translated to spec v3.1 and merged into a rendered MC
which represents the canonical state the MCO enforces.
FCC is more human friendly to write and read than Ignition config, and can be transpiled
to Ignition spec v3.1 config safely with the Fedora CoreOS Config Transpiler (FCCT). Support for
FCCs can be added by adding the FCCT parser to the RawExtension processing chain.

## Motivation

Writing MachineConfig objects by hand is currently difficult due to the fact the Ignition
configuration it contains is machine-friendly JSON data, which makes it hard for humans to
read it and write it manually.

FCCs are YAML encoded and more easily readable and writable for humans, which will improve
the ergonomics of working with MachineConfig objects.

### Goals

The goal is to improve UX for cluster admins and to make writing MCs more ergonomic by allowing
the MC's `Config` field to contain YAML-encoded FCCs.
A MachineConfig object that contains FCC will be rendered and merged into the canonical
configuration that the MCO manages, like any other MC object on the cluster.

### Non-Goals

- Introduction of a distinct type for the rendered MachineConfig that represents the current canonical
configuration (currently spec v3.1 Ignition config) is not a goal of this enhancement.

## Proposal

### User Stories

#### Story 1

As a cluster admin,
I want to be able to write machine configuration manually in the human-friendly FCC
format specification which contains useful shorthands and sugar for generating
spec v3 Ignition configuration.

### Implementation Details/Notes/Constraints

The `Config` RawExtension field of a MachineConfig resource is currently parsed for
Ignition spec v2.2, v3.0 and v3.1 configuration.
FCCT is already a dependency of the MCO as its template controller uses FCCs for 
templating the files the MCO writes.
Adding the logic to check MCs for FCC contents does not require large code changes:
https://github.com/openshift/machine-config-operator/pull/1980


### Risks and Mitigations

- Downgrades: Using FCCs in a cluster that does not yet have this feature will not be possible

## Design Details

### Open Questions [optional]

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
    - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
    - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

#### Examples

These are generalized examples to consider, in addition to the aforementioned [maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.
