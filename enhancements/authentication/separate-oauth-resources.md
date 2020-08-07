---
title: Separate-OAuth-API-Resources
authors:
  - "@deads2k"
reviewers:
  - "@sttts"
approvers:
  - "@derekwaynecarr"
creation-date: 2019-10-16
last-updated: 2020-08-07
status: implementable
see-also:
replaces:
superseded-by:
---

# Separate OAuth API-Resources

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions [optional]

## Summary

The API resources used by the OpenShift OAuth server will be moved to their own `oauth-apiserver` repository.
The oauth-apiserver binary will be managed by the existing authentication-operator.

## Motivation

The OpenShift OAuth server requires several openshift API resources in order to function.
Because the OAuth server enables access to the kube-apiserver, it needs to be able to run separately from the existing
set of API resources that are focused on enabling a developer workflow.
This narrowing of scope expands the deployment options for the OpenShift OAuth server.

### Goals

1. Make it possible to run the OpenShift OAuth server without installing all the developer focused API Resources.

### Non-Goals

1. Move routes out of the OpenShift API Server.

## Proposal

1. Create a new repo called `oauth-apiserver` that produces a binary called `oauth-apiserver`.
2. Add the following API groups to the new repo
   1. oauth - core oauth types
   2. user - core types used by oauth and authentication
3. Update the authentication operator to install the new `oauth-apiserver`, but not wire the apiservice.
4. Add a new `ManagingOAuthAPIServer` field to authentication's operator status field. Setting this field to true will cause `cluster-openshift-apiserver-operator` to step down.
4. Add the code to the `cluster-openshift-apiserver-operator` tolerate apiservice management by the `cluster-authentication-operator`
(indicated by an annotation written on the apiservice) if the `cluster-authentication-operator`'s `ManagingOAuthAPIServer` field is set to true.
5. Update the `cluster-authentication-operator` to wire the apiservices.
6. Remove the registration from the `cluster-openshift-apiserver-operator`.
7. Remove the serving code from the `openshift-apiserver`.

If we stop at any intermediate step, we are still safe to ship.

### User Stories [optional]

#### Story 1

#### Story 2

### Risks and Mitigations

The proposal is safe to ship at any point with clearly working upgrade/downgrade.

## Design Details

### Test Plan

If the system works, we did it.

##### Removing a deprecated feature

See upgrade/downgrade.

### Upgrade / Downgrade Strategy

This is an interesting situation.  We want to hand control of the API over to a new process.  This is possible to do,
however, to do it in a single release requires coordination.

1. Add code in 4.5 to the `cluster-openshift-apiserver-operator` that prevents it from managing an apiservice if it is "claimed"
by the `cluster-authentication-operator` via an annotation and if the `cluster-authentication-operator`'s `ManagingOAuthAPIServer` field is set to true.
2. Remove that code in 4.7.
3. Add code to `cluster-authentication-operator` to manage the apiservices in 4.6.
4. In 4.7, the `openshift-apiserver` can remove the code serving.

In an upgrade (from 4.5 to 4.6) case, the `cluster-openshift-apiserver-operator` will yield once the `cluster-authentication-operator` picks up the serving.
Because the apiservice never goes unavailable, and the old `openshift-apiserver` continues serving until the new server
starts, clients should not see any disruption.


In a downgrade (from 4.6 to 4.5) case, the `cluster-openshift-apiserver-operator` will take over once the `cluster-authentication-operator`s `ManagingOAuthAPIServer` field is set to false.
Because the apiservice never goes unavailable, and the old `openshift-apiserver` continues serving until the new server
starts, clients should not see any disruption.

### Version Skew Strategy

See the upgrade/downgrade strategy.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
