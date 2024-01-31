---
title: microshift-route-admission-policy
authors:
  - "@pacevedom"
reviewers:
  - "@eslutsky"
  - "@copejon"
  - "@ggiguash"
  - "@pmtk"
  - "@pliurh"
  - "@jerpeter1"
  - "@Miciah"
approvers:
  - "@dhellmann"
api-approvers:
  - None
creation-date: 2024-01-30
last-updated: 2024-02-01
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1067
see-also:
  - "/enhancements/ingress/openshift-route-admission-policy.md"
---

# MicroShift route admission policy
## Summary
OpenShift defaults to not allow routes in multiple namespaces use the same
hostname, and MicroShift inherits that default.

Ever since OpenShift 4 this has been possible to configure, and MicroShift
should allow that too to accommodate additional use cases.

## Motivation
Users who compose applications from multiple sources may deploy those to
different namespaces, albeit want to expose them using the same hostname and
different path.

### User Stories
As a MicroShift admin, I want to configure MicroShift to support combining
routes from multiple namespaces.

### Goals
- Allow the use of the same host in routes from different namespaces.

### Non-Goals
N/A

## Proposal
The proposal follows all the changes that were done in the original
[enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/openshift-route-admission-policy.md).

This includes the exposure of a new configuration option to pass on to the
router to allow cross-namespace host routes.

MicroShift will default to disable the namespace ownership checks, contrary to
OpenShift. MicroShift is not meant to be multi-tenant and hold several
unrelated applications that need protection between them.

MicroShift is also focused on single-node deployments, and runs without an
external load balancer and in environments without access to deep DNS
integration. The primary means of accessing the apps on the MicroShift host
will be the single hostname of that host.

### Workflow Description

### API Extensions
To allow configuring the router a new option is exposed through the MicroShift
configuration file:
```yaml
router:
  routerAdmissionPolicy:
    namespaceOwnership: <Strict|InterNamespaceAllowed> # Defaults to InterNamespaceAllowed.
```

In order to keep configuration as close to OpenShift, the option and its values
are the same.

When set to `Strict` the router will not allow routes in different namespaces
to claim the same host.
When set to `InterNamespaceAllowed` the router will allow routes in different
namespaces to claim different paths of the same host. This is the default
value, as MicroShift's typical use cases are not multi-tenant and this is the
most flexible value.

### Risks and Mitigations
N/A

### Drawbacks
N/A

## Design Details
See original [enhancement](https://github.com/openshift/enhancements/enhancements/ingress/openshift-route-admission-policy.md)
for more details.

### Test Plan
Same as original [enhancement](https://github.com/openshift/enhancements/enhancements/ingress/openshift-route-admission-policy.md).
These tests will be included as part of the automatic tests in CI.

### Graduation Criteria
#### Dev Preview -> Tech Preview
- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA
- Sufficient time for feedback
- Available by default
- User facing documentation created

#### Removing a deprecated feature
N/A

### Upgrade / Downgrade Strategy
N/A

### Version Skew Strategy
N/A

### Operational Aspects of API Extensions
N/A

#### Failure Modes
N/A

#### Support Procedures
N/A

## Implementation History
N/A

## Alternatives
N/A
