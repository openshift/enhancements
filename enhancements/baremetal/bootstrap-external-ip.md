---
title: bootstrap-external-ip
authors:
  - "@honza"
reviewers:
  - "@hardys"
  - "@dtantsur"
  - "@dhellmann"
  - "@patrickdillon"
approvers:
  - "@hardys"
  - "@dtantsur"
  - "@dhellmann"
  - "@patrickdillon"
api-approvers:
  - None
creation-date: 2022-03-23
last-updated: 2022-03-23
tracking-link:
  - https://issues.redhat.com/browse/METAL-175
see-also:
  - https://bugzilla.redhat.com/show_bug.cgi?id=2048600#c1
replaces:
superseded-by:
---

# Bootstrap External IP

## Summary

When installing a bare metal IPI cluster, you can use the `networkConfig` field
in the `install-config.yaml` file to configure the control plane network
interfaces for the cluster hosts.  However, currently, you cannot configure the
bootstrap VM networking using the same means.

## Motivation

In environments where no DHCP server is running, the bootstrap VM will not get
an IP address on the control plane NIC causing cluster installation to fail.
The user can work around this by modifying the bootstrap ignition file; however,
this isn't a friendly experience.

### User Stories

- As a user, I want to be able to deploy an IPI bare metal cluster in an
  environment where no DHCP cluster is running
- As a user, I want to specify a static IP address for the bootstrap VM

### Goals

- Support cluster installation in environments without a running DHCP server
- Improve the level of customization of bootstrap VM networking
- Improve user-friendliness when specifying a static IP for the bootstrap VM

### Non-Goals

- Provide means for configuring every aspect of bootstrap networking

## Proposal

We will add two new fields, `bootstrapExternalStaticIP` and
`bootstrapExternalStaticGateway`, to allow for further customization of
bootstrap networking.  We will use these fields to generate static configuration
in the installer.

### Workflow Description

The **cluster creator** will modify the `install-config.yaml` to configure
static networking for the bootstrap node.

### API Extensions

We will add two new fields to the baremetal platform section of the
`install-config.yaml` file called `bootstrapExternalStaticIP`, and
`bootstrapExternalStaticGateway`.  This is similar to the existing
`bootstrapProvisioningIP` field.

### Risks and Mitigations

### Drawbacks

Adding more and more fields to the already busy `install-config.yaml` can be
considered less than ideal.

## Design Details

### Test Plan

There is work in progress to add end-to-end tests to exercise the static IP
scenario.

### Graduation Criteria

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
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

N/A

## Alternatives

We can document the specific scenario, and offer the ignition-based workaround
as a possible solution.
