---
title: generate-ImageContentSourcePolicy-scoped-to-a-registry
authors:
  - "@mhrivnak"
  - "@sacharya"
reviewers:
  - "@mgoldboi"
  - "@robszumski"
  - "@beekhof"
  - "@ecordell"
  - "@eparis"
  - "@markmc"
approvers:
  - TBD
creation-date: 2020-06-10
last-updated: 2020-06-10
status: implementable
---

# Generate ImageContentSourcePolicy scoped to a registry

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]


## Summary

The `oc adm catalog mirror` command [generates an
ImageContentSourcePolicy](https://docs.openshift.com/container-platform/4.4/operators/olm-restricted-networks.html#olm-updating-operator-catalog-image_olm-restricted-networks)
that maps the original container image repository to a new location where it
will be mirrored, typically inside a disconnected environment. When a new or
modified ICSP is applied to a cluster, it is converted to a config file for
cri-o and placed onto each Node. The process of placing the config file on a
Node includes rebooting that Node.

Today, the `oc adm catalog mirror` command generates an ICSP where each entry
is specific to a repository. For example, it would map
`registry.somevendor.io/cloud/burrito-db` to
`mirror.internal.customer.com/cloud/burrito-db`. This enhancement proposes to
start generating ICSPs at the registry scope by default. Using the same
example, `registry.somevendor.io` would map to `mirror.internal.customer.com`.

Having a widely-scoped ICSP reduces the number of times the ICSP might need to
change in the future, and thus reduces the number of times a cluster needs to
reboot all of its Nodes.

## Motivation

Today, any time a customer mirrors a new operator into their environment, that
will result in a new entry in their ICSP, and thus require a cluster reboot. It
is also possible that any time the customer mirrors updates for the operators
they already use, if any one of those updates references a container image from
a new location (this applies to any image related to the operator or its
operand), that also may result in a change to the ICSP and thus a reboot.

Some clusters may take a long time to reboot, and some customers may be averse
to rebooting more often than is necessary. Bare metal clusters in particular
may take up to 15 minutes to reboot each Node, not including the time it takes
to drain workloads. This enhancement enables a customer to mirror operators
into their local registry as often as they want without inducing a reboot, so
long as they are mirroring content from a registry that they have mirrored from
in the past.

### Goals

* Enable a customer to add an operator to their local mirror without causing their cluster to reboot.
* Enable a customer to mirror updates to operators they already use without causing their cluster to reboot.

### Non-Goals

* Eliminate the need to reboot a cluster when an ImageContentSourcePolicy changes or is created.

## Proposal

Change the `oc adm mirror` command so that the ImageContentSourcePolicy it
outputs is scoped to the registry level. It will not include any path
information.

Optionally a command-line flag could be used to enable today's behavior of
scoping ICSP entries to a specific repository, in case that behavior is
desirable to someone.

### User Stories

#### Story 1

As a user with a cluster in a disconnected network, I can mirror optional
operators into my local registry, and my cluster will only need to reboot Nodes
the first time I mirror content from any particular remote registry.

#### Story 2

As a user with a cluster in a disconnected network, I can mirror updates to
optional operators that I already have installed without causing my cluster to
reboot.

### Implementation Details/Notes/Constraints [optional]

This is an example of a registry-scoped ICSP:

```
apiVersion: operator.openshift.io/v1alpha1
kind: ImageContentSourcePolicy
metadata:
  name: redhat-operators
spec:
  repositoryDigestMirrors:
  - mirrors:
    - local.registry:5000
    source: registry.redhat.io
```

The above ICSP was tested on OCP 4.4, and it resulted in an entry as shown
below in
[`/etc/containers/registries.conf`](https://github.com/containers/image/blob/8051f86/docs/containers-registries.conf.5.md)
on each Node:

```
[[registry]]
  prefix = ""
  location = "registry.redhat.io"
  mirror-by-digest-only = true

  [[registry.mirror]]
    location = "local.registry:5000"
```

An operator was then installed, and its image was observed being pulled from
`local.registry:5000`.

### Risks and Mitigations

No new risks are introduced. This enhancement proposes to use an existing API
in a slightly different way.

## Design Details

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
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

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

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

This change is safe for existing clusters that previously have mirrored
optional operators. The first time they run the new implementation of `oc adm
catalog mirror` and apply the resulting ICSP, it will definitely be different,
so it will cause a reboot. That reboot may have happened anyway.

If a user downgrades and starts generating old-style ICSPs again, that will
likewise continue to be safe.

### Version Skew Strategy

Current OCP 4.4 clusters already support the API usage that is proposed in this
enhancement. There is no version skew concern.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

If a user was depending on `registries.conf` to prevent a cluster from
accessing specific container images in the local registry, this change would
not support that usage. But that user could still manage their own ICSP and
ignore the one generated by the `oc adm catalog mirror` command. Also there are
other, arguably better, ways to restrict access to specific repositories in a
registry.

## Alternatives

To support the goal of rebooting less often, we could instead enhance the
machine-config-operator to identify when an ICSP change is purely additive and
not reboot the cluster in that case. That is a worthwhile alternative to
pursue, but it is not mutually exclusive with this enhancement. This
enhancement could be implemented and tested with less complexity, and thus
could likely be delivered to users sooner. This enhancement also has the
advantage of simplifying the contents of `registries.conf` and the sum of all
ICSPs, which itself is a good thing for manageability.

## Infrastructure Needed [optional]

None.
