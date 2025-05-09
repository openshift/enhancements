---
title: Clusters on Managed Dedicated Hosts in AWS
authors:
  - "@faermanj"
  - "@rvanderp3"
reviewers:
  - "@nrb" # CAPA/MAPI
  - "@patrickdillon" # installer 
  - "@makentenza" # Product Manager
approvers: 
  - "@patrickdillon"
creation-date: 2025-05-09
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@JoelSpeed"
last-updated: 2025-05-09
tracking-link: 
  - https://issues.redhat.com/browse/SPLAT-2207
see-also: {}
replaces: {}
superseded-by: {}
---

# Clusters on Managed Dedicated Hosts in AWS

## Summary

This enhancement proposal outlines the work required to enable OpenShift to manage dedicated hosts on AWS. 

## Motivation

### User Stories

1. As an administrator, I want openshift to allocate and release hosts as necessary, according to my configurations and limits.

### Goals

- Enable OpenShift to manage dedicated AWS hosts.

### Non-Goals


## Proposal


### Workflow Description

### API Extensions

Add fields for host configurations and limits.

### Topology Considerations


#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints


### Risks and Mitigations


### Drawbacks


## Alternatives (Not Implemented)


## Open Questions [optional]

## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

N/A

## Infrastructure Needed

- Dedicated host(s) for unit testing, e2e, and development purposes.