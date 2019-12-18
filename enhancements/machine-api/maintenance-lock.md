---
title: maintenance-lock
authors:
  - "@beekhof"
reviewers:
  - "@bison"
  - "@enxebre"
  - "@michaelgugino"
  - "@derekwaynecarr"
  - "@crawford"
  - "@eparis"
  - "@rphillips"
  - "@runcom"
approvers:
  - "@bison"
  - "@enxebre"
  - "@michaelgugino"
  - "@derekwaynecarr"
  - "@crawford"
  - "@eparis"
  - "@rphillips"
  - "@runcom"
creation-date: 2019-12-06
last-updated: 2019-12-09
status: provisional
see-also:
  - https://github.com/kubernetes/enhancements/pull/1080
  - https://github.com/kubernetes/enhancements/pull/1411
replaces:
  - https://github.com/kubevirt/node-maintenance-operator
superseded-by:
  - NA
---

# Maintenance Lock

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

None

## Summary

There is a need for a common place to request and watch for nodes to be put
into maintenance.  Whether maintenance mode is required to perform triage or
hardware replacement, a standardized way to represent it allows system and
layered components to delay activities that would inhibit those activities and
for UIs/consoles to communicate status to admins.

On baremetal, there is a need to perform additional tasks, such as draining the
node, when maintenance is requested.  However web UIs do not have access to the
`oc` command, preventing them from invoking a Go based drain library.  This
forces admins to use the `oc` command to trigger a client-side cordon and/or
drain of nodes, as well as to construct additional commands to determine
progress, wait for completion, triage failures, and unwind the process once the
maintenance activity is completed.

## Motivation

Badly timed updates can make triage an even longer process, but poorly timed
power events could corrupt the machine and represent a health and safety risk.

Without first-class support from the server, the manual steps needed to perform
equivalent functionality may lack consistency between admins, present
opportunities for mistakes, and are not reflected in the UI/consoles for other
parties to base informed decisions.

All these aspects are especially important in Telco environments where there
are multiple levels of admins, across different physical locations, managing
thousands of clusters.

In such an environment, the admin needs to find identify the cluster a
problematic node belongs to as well as locate and use the corresponding
`kube.config` and/or pass `oc` the correct cluster name for every call.  Not
impossible but certainly error prone.

### Goals

- A mechanism for signaling that a node is in a maintenance state

- A metal3 specific operator that performs relevant actions (eg. cordon +
  drain) when maintenance is requested

- Changes to existing components to disable or delay functions that would
  affect the node, such as:
  - Delay upgrades
  - Disable health checks

- Expose the ability to display and control the maintenance state via the UI

- Expose the ability to query and control the maintenance state programatically
  via a console


### Non-Goals

- To agree on a single set of actions that should be performed across all
  platforms.

## Proposal

Utilize the existing `Lease` built-in API in the API group
`coordination.k8s.io` to define a new per-node `Lease` object with Name equal
to Node name in a newly created dedicated namespace. [Further
details](https://github.com/kubernetes/enhancements/pull/1411)

Components performing a invasive and/or destructive actions much aquire the
per-node lease in advance to avoid software updates, reboots, or hardware power
events while another component is performing a similar action.

Invasive actions include anything that modifes the node's operating system or
it's configuration (eg. MCO/MCD).  Destructive actions include anything that
result in the node being deleted (autoscaling, machine healthcheck) or change
power state.

On metal3 based deployments, a `lifecycle.openshift.io/maintenance` node
annotation will be created for UIs and consoles to use to put a node into
maintenance.

Additionally, we define a new Metal3 Maintenance Operator that:

- watches for the `lifecycle.openshift.io/maintenance` annotation to be added
- acquires the per-node lease
- cordons the node
- uses the drain library to move workloads elsewhere
- reboots the machine at the end of the maintenance window

In the future, cloud platforms may create a different controller to peform
additional tasks based on the annotation if the need arises.

To exit maintenance mode, the admin can programatically remove the annotation
or use a UI/console.  This signals to the Metal3 Maintenance Operator that:

- any in-flight drain operations should be cancelled,
- the the node should be uncordoned,
- the node rebooted,
- the lease released

### User Stories [optional]

#### Story 1

As a server component, I want to put a node into maintenance, so that I can
inhibit machine healthchecks while writing a new CoreOS image to disk.

#### Story 2

As cloud admin, I want to put a node into maintenance, so that it drains and I
can see workloads start elsewhere before deleting it.

#### Story 3

As baremetal admin, I want to put a node into maintenance, so that I can
inhibit power events while I'm replacing hardware.

#### Story 4

As baremetal admin, I want to put a node into maintenance, so that I can
inhibit reboots and software changes while I'm triaging an issue.

### Implementation Details/Notes/Constraints [optional]

None

### Risks and Mitigations

UX for maintenance mode has previously shipped for baremetal as part of OCP 4.3

RBAC rules will be needed to ensure that only specific profiles are able to
create/remove the maintenance annotation, least it be used as a DoS attack
vector.

## Design Details

### Test Plan

TBA

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

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- Remove feature gate
- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

### Version Skew Strategy

The use of an annotation as the signalling mechanism prevents most types of
version skew.

It is possible that some coordination may be required between the platform
specific component and any APIs it makes use of.

## Implementation History

06-Dec-2019 - Initial version

## Drawbacks

TBA

## Alternatives

TBA

## Infrastructure Needed [optional]

None
