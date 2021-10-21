---
title: client-cert-base-metric-scraping
authors:
  - "@deads2k"
reviewers:
  - "@sur"
approvers:
  - "@sur"
creation-date: 2021-03-18
last-updated: 2021-03-18
status: implementable
see-also:
replaces:
superseded-by:
---

# Client Cert Base Metrics Scraping

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Monitoring needs to be reliable and is the very useful when trying to debug clusters in an already degraded state.
We want to ensure that metrics scraping can always work if the scraper can reach the target, even if the kube-apiserver
is unavailable or unreachable.
To do this, we will combine a local authorizer (already merged in many binaries and the rbac-proxy) and client-cert based
authentication to have a fully local authentication and authorization path for scraper targets.

## Motivation

If networking (or part of networking) is down and a scraper target cannot reach the kube-apiserver to verify a token
and a subjectaccessreview, then the metrics scraper can be rejected.
The subjectaccessreview (authorization) is already largely addressed, but service account tokens are still used
for scraping targets.
Tokens require an external network call that we can avoid by using client certificates.
Gathering metrics, especially client metrics, from partially functionally clusters helps narrow the search area between
kube-apiserver, etcd, kubelet, and SDN considerably.

In addition, this will significantly reduce the load on the kube-apiserver.
We have observed in the CI cluster that token and subject access reviews are a significant percentage of all kube-apiserver traffic.

### Goals

1. Gather all possible metrics even when targets cannot reach the kube-apiserver.
2. Avoid sending valid kube-apiserver credentials to everyone who can create a servicemonitor.

### Non-Goals

1. Force all servicemonitor targets to change at the same time.

## Proposal

1. The monitoriting operator creates a client cert/key pair valid against the kube-apiserver.
   There is a CSR signer name specifically for this.
2. The kube-controller-manager operator runs an approver that allows monitoring operator SA to get signed certificate for
   The existing scraper identity.
3. The monitoring operator puts the client cert/key pair into a secret.
4. The monitoring operator keeps these credentials up to date (the signer is only good for 30 days or so).
5. The metrics scraper mounts the client cert/key pair and ensures that it is hot-reloadable or builds a process suicider.
   There is library-go code to suicide on file changes.
6. A participating metrics scraper target, can read the in-cluster client cert CA bundle and create an authenticator
   using it.
   The default delegated authentication stack does this and on openshift, the configmap is exposed to system:authenticated.
7. A servicemonitor grows a way to specify using this client cert.
   Technically this is optional.
   If you find it too difficult, just *always* use the client-cert for scraping since the secrets never leave disk.
   Those targets which don't support it will simply ignore the client-cert.
8. Profit.
   Now we can collect metrics for at least
   1. openshift-apiserver
   2. openshift-controller-manager
   3. kube-scheduler
   4. all kubelets
   5. node-exporter
   6. all control plane operators
   7. kube-controller-manager
   8. etcd
   These are the ones I've updated already and we'll be able to get more.

### User Stories

#### Story 1

I want developers to quickly solve my problems.

View the metrics that will now be collected.

#### Story 2

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

## Design Details

### Open Questions [optional]

### Test Plan

### Upgrade / Downgrade Strategy

The CSR API has been at v1 and able to support our needs for multiple releases.
The servicemonitors don't actually have to change in 4.9, they could change in 4.10 and we could still use this new capability in 4.9.

### Version Skew Strategy

Since service monitor's don't have to change, we don't need risk destabilizing changes.

## Drawbacks

I have to stop here to make dinner.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
