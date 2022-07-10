---
title: bootstrap-kubelet-ip
authors:
  - "@tsorya"
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
creation-date: 2022-07-10
last-updated: 2022-07-10
tracking-link:
  - https://issues.redhat.com/browse/MGMT-11102
see-also:
  - https://bugzilla.redhat.com/show_bug.cgi?id=2070318
replaces:
  - https://github.com/openshift/installer/pull/6042/files
---

# Bootstrap External IP

## Summary
When installing a new cluster with UPI, you can set machine networks that will 
be set to `networkConfig` field in the `install-config.yaml` file to configure the control plane network
interfaces for the cluster hosts. However, users cannot configure 
the machine network for the bootstrap node.

## Motivation

In environments where bootstrap machine has more than one ip address, we should be able to set which ip that 
will be configured as a hostIp in kubelet. This ip will be used in bootstrap kube-api service as advertised ip.
This will allow traffic to go through expected interface in case where we have more than one interface and kubelet on bootstrap 
choose ip different from subnet configured on masters.
The user can work around this by modifying the bootstrap ignition file; however,
this isn't a friendly experience.

### User Stories

- As a user, I want to be able to specify host ip address for bootstrap machine

### Goals

- Support cluster installation in environments with bootstrap machine with more than one ip configured
- Improve the level of customization of bootstrap networking

### Non-Goals

- Provide means for configuring every aspect of bootstrap networking

## Proposal

We will add a new field, `bootstrapNodeIP` to allow for further customization of
bootstrap host ip that should be used for kubelet configuration. 
We will use this field to set BootstrapNodeIP field in kubelet.sh.template.
### Workflow Description

The **cluster creator** will modify the `install-config.yaml` to configure
kubelet node ip for the bootstrap node.

### API Extensions

We will add a new field to the baremetal and None platform sections of the
`install-config.yaml` file called `bootstrapNodeIP`.

### Risks and Mitigations

### Drawbacks

Adding more and more fields to the already busy `install-config.yaml` can be
considered less than ideal.

## Design Details

### Test Plan

Assisted-installer tests, in each e2e scenario will test this field

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
1. We can document the specific scenario, and offer an ignition override workaround
as a possible solution in which we will override kubelet.sh in bootstrap ignition.

2. We can add service, aka nodeip-configuration, that will check machine networks in install-config 
and set kubelet node ip according to it.
This solution will adds some complexity, as a new service needed be add to bootstrap ignition 
that kubelet will depend on and this service is not needed in most scenarios.
