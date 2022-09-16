---
title: trust-chain-separation
authors:
  - stlaz
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - ibihim
  - deads2k
  - alvaroaleman
  - csrwng
approvers:
  - deads2k
  - alvaroaleman
  - csrwng
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/AUTH-311
see-also:
replaces:
superseded-by:
---

# Trust Chain Separation

## Summary

At the time of writing the Hypershift is generating most of its certificates
by using a single trust-all root CA. This enhancement describes how and why
such a bundle should be split in order to assure workload portability between
OpenShift-based products.

## Motivation

The OpenShift certificate trust domain separation has been serving well
to prevent bugs of extended trust for client certificate authentication,
and also to prevent implicit trust to servers that the client should not
attempt connecting to in the first place.

Having a single signer for most certificates is suboptimal as it opens
window for bugs with unexpected trusts where a server/client grabs the
global CA bundle and misuses it as this likely "just works" for them.
It is therefore possible to build a number of solutions which would be
considered "stable". As that effectively makes the CA bundle an "API",
it is later hard to split the trust to be more fine-grained as chipping
away threads of trust breaks the solutions which now rely on the global
trust bundle.

The intention is to perform at least a minimal split of the client certificate
signers from the root CA bundle in order to prevent unwanted application
authentications and to assure portability of user workload among OpenShift
platform flavors.

### User Stories

1. I would like to be able to run my workloads on any OpenShift-like platform.
My workloads allow components to authenticate using client certificates based
on a trust bundle that I am able to retrieve from the cluster.

2. I don't want my users to have access to any CA bundle that would allow them
to trust a random certificate from the cluster for client certificate authentication.

### Goals

1. Separate client trust from the global root CA so that client
certificates are being minted by CAs outside of the global trust
and these CAs are separate so that they create logical trust domains.

2. Make sure that it is possible to supply the following certificates for
the kube-apiserver:
    - client certificate to trust for a worst-case break-glass scenario
    - CA and client certificate to trust for API aggregation
    - serving certificate for a given hostname to enable SNI for external domains

### Non-Goals

1. Separate the root CA completely so that the trust chains resemble those
of standalone OCP

## Proposal

The global trust bundle should not contain any CA certificate of a
signer that is used to sign client certificates in the platform. The bundles
for each separate client certificate trust chain will be grouped the same
as the OpenShift platform does it and as documented in the
[TLS documentation](https://github.com/openshift/api/tree/master/tls/docs).

The CA certificates and the certificates signed by these CAs are
subject to certificate rotation and the Hypershift control-plane needs
to be able to adjust to the dynamic nature these certificates.

### Workflow Description

By default, user workloads don't receive any CAs that they could
use to authenticate incoming TLS client certificate authentication.
In order to retrieve the recommended trust bundle, they would use the
"kube-system/extension-apiserver-authentication" config map that's
commonly used for this purpose in the Kubernetes codebase.

The "kube-system/extension-apiserver-authentication" contains the bundles
described in [KAS total CA bundle](https://github.com/openshift/api/blob/master/tls/docs/kube-apiserver%20Client%20Certificates/README.md#kube-apiserver-total-client-ca-locations)
and [aggregated-front-proxy CA](https://github.com/openshift/api/tree/master/tls/docs/Aggregated%20API%20Server%20Certificates#aggregator-front-proxy-ca)
in its "client-ca-file" and "requestheader-client-ca-file" data fields, respectively.

### API Extensions

This enhancement does not propose any API extensions.

### Risks and Mitigations

Managing rotations of multiple certificate trust chains might be a bit more
complex but with the right amount of automated tests this should not be an
issue.

### Drawbacks

There are currently no known drawbacks.

## Design Details

### Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

### Test Plan

The correct way to test certificates and their rotations is to keep the rotation
period as low as possible and see that the cluster is still performing as expected
with no trust being broken.

### Graduation Criteria

The feature should make it into the product before the first stable version.

#### Dev Preview -> Tech Preview

Nothing here

#### Tech Preview -> GA

Nothing here

#### Removing a deprecated feature

Nothing here

### Upgrade / Downgrade Strategy

Nothing here

### Version Skew Strategy

Nothing here

### Operational Aspects of API Extensions

Nothing here

#### Failure Modes

Two failure modes come to mind:
1. controllers handling certificate rotation fail to perform and the certificates expire
2. CA or client certificate keys get leaked.

This enhancement directly reduces the impact of either of these failures to just certificate
authentication.

#### Support Procedures

Failure mode 1:
- components can no longer authenticate using client certificates
- possible symptoms: control-plane controllers don't perform any expected actions,
  kubelet fails to report node status, aggregated API is inaccessible

Failure mode 2:
- very hard to detect
- possible symptoms: increased activity of highly-privileged users in the clusters

Either of the above would require manual intervention in either troubleshooting the
failing controller or manually rotating the certificate/key pairs in question.

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
