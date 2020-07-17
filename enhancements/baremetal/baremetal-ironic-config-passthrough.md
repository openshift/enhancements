---
title: baremetal-ironic-config-passthrough
authors:
  - "@derekh"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-07-17
last-updated: 2020-07-17
status: provisional
see-also:
  - "https://github.com/openshift/installer/pull/3887"
  - "https://github.com/openshift/machine-api-operator/pull/645"
---

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Baremetal IPI makes use of OpenStack Ironic to provision RHCOS to each baremetal
host. Ironic is deployed in the metal3 pod and its configuration is highly opinionated.
This enhancement would provide a mechanism to pass arbitary ironic config options
into the metal3 pod in order to change how ironic and ironic-inspector are configured.

## Motivation

The current opinionated ironic works well when we have a single set of target hardware
to support, this is the ideal but hasn't been our experience. As users deploy IPI clusters
in labs, production systems, and for CI, we occasionally needs to deal with corner cases arise.

The motivation for this enhancement is to allow people to deal with their individual corner
cases without the need to insert special cases into the ironic containers. The ironic
container can stay opinionated and targeted for a specific set of environments while also
being usable where uniq envirnments require adjustments.

In addition, when a change is being considered to ironic's opinionated configurations,
deployers will be able to test this change in their deployment and make recommendations on
an the suitability of making a config update permanent in the ironic image.

### Goals

Allow users of to tweak ironic config options on the fly without the need for a new
ironic image.

### Non-Goals

Our goal is not to provide an API the ends up being commonly used with a small
subset of options. i.e. if any specific config option becomes a frequently
tweaked option for multiple environments then it should be considered a condidate
as a top-level platform configuration.

## Proposal

Add a new platform configuration "ironicExtraConf" of type map, this would consist
of keys and values to be used as config options for configuring ironic.

Each config option should be expressed as a key/value pair with the format

OS_<section>_\_<name>=<value> - where `section` and `name` are the
reprepresent the config option in ironic.conf e.g. to set a IPA ssh key and
set the number of ironic API workers

```yaml
platform:
  baremetal:
    ironicExtraConf: {"OS_PXE__PXE_APPEND_PARAMS":'nofb nomodeset vga=normal sshkey="ssh-rsa AAAA..."', "OS_API__API_WORKERS":"8"}
```

### User Stories [optional]

#### Story 1

As a developer debuging baremetal ipi CI, I want to be able to log the console
of virtual baremetal nodes while they are being provisioned, in order to do
this I need to tweak IPA kernel command line options to write to ttyS0, this
is a CI only adjustment is not suitable for real baremetal (where tty0 would
be ideal).

#### Story 2

As a Deployment Operator, I want Barametal IPI deployments to be customizable to my
hardware that features some distinctive characteristic.

### Implementation Details/Notes/Constraints [optional]

A new property "ironicExtraConf" would be added to the "provisionings.metal3.io" CRD,
this property would be a map holding environment variables to be defined in the ironic
containers.

This enhancement would make use of "Config Opts From Environment" provided by
ironic's use of oslo.config(see https://specs.openstack.org/openstack/oslo-specs/specs/rocky/config-from-environment.html).
To utilize this each of the containers running an ironic service should
export an environment variable which will then automatically be picked up
by ironic.

### Risks and Mitigations

The risk here is that Deployment Operators would start making common use
of this API without providing feedback upstream. They would not be "forced"
to push improvments into a release where others could take advantage.

To mitigate against this any passthrough API provided should be marked as
experimental and any config options set unsuported, along with suitable
message to warn the operator of the situation.

## Design Details

### Open Questions [optional]

### Test Plan

### Graduation Criteria

- As we wouldn't intend to support any options changed with this API is status
  would remain as `Tech Preview` once implemented.

##### Dev Preview -> Tech Preview

- Ability to update any ironic or ironic-inspector config option
- config option should be reflected in ironic on the bootstrap node
- config option should be reflected in ironic in the metal3 pod

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History


## Drawbacks

## Alternatives

