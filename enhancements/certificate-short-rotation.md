---
title: certificate-short-rotation
authors:
  - vrutkovs
reviewers:
  - deads2k
approvers:
  - deads2k
api-approvers:
  - deads2k
creation-date: 2024-08-24
last-updated: 2024-08-24
tracking-link:
  - https://issues.redhat.com/browse/API-1688
---

# Short Rotation Period For Certificates

## Summary

Add new feature gate in DevPreview set so that components would issue certificates with shorter 
duration - hours instead of days.

## Motivation

Currently certificates are issued by Openshift with various validity durations, but at least its 15 
days. This makes testing certificate rotation in CI complicated - we have to emulate passing time 
using time skewing. This methods shows how cluster recovers after certificates have expired, but 
it doesn't help us with testing happy path when certificates rotate during standard cluster lifecycle.

Some components (i.e. cluster-kube-apiserver-operator) issue certificate with shorter lifetime in 
development branch. This requires us to revert this change every time we branch for new release.
This also doesn't help us in CI, as it needs a similar change in the installer. 
Also, most components are not using this, so we end up with some certificates valid for hours but  
most would be valid for days.

Since the change to revert this setting requires manual pull request, there is chance that this 
setting will leak into supported releases.

This enhancement describes a new feature gate, which would enable this feature for all components 
and ensure that stable releases don't have it accidentally enabled as it uses FeatureGates.

### User Stories

> As an Openshift developer, I want to have a setting for component to issue shorter living 
> certificates so that I could verify that certificate rotation doesn't cause issues

Note that this lacks any customer userstories - this is a developer-only feature, customers are 
not expected to use it

### Goals

* Create a new FeatureGate in DevPreview featureset
* Each component can decide the new duration for certificates separately.
* Create e2e tests enabling this featuregate and checking that certificate rotate correctly
* Run e2e periodically to ensure cluster with this featuregate is functional

### Non-Goals

* Change validity duration for existing certificates

## Proposal

Update components to read enabled FeatureGates and update certificate issuing code in all OpenShift 
components.

The featuregate would make components generate certificates which have shorter duration - hours 
instead of days, so that we could verify that most certificates can be rotated within duration of 
e2e test. This would allow developers to verify that certificates get rotated without breaking 
cluster features. Standard minimal conformance test should be sufficient, however component teams 
may want to suggest additional tests to run after rotation is performed.

Component developers would be able to decide the duration of the certificates - and collect a list 
of certificates which should not be rotated. Some signers are meant to last 
"indefinitely" (10 years) to support cluster features, i.e. CSR signer is not meant to 
expire so that new nodes could join.

Identified components required changes:
* installer
* cluster-kube-apiserver-operator
* cluster-kube-controller-manager-operator
* cluster-etcd-operator
* cluster-network-operator
* service-ca-operator
* OLM


### Workflow Description

N/A

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

Not applicable to MicroShift - it doesn't issue certificates via operators

### Implementation Details/Notes/Constraints


### Risks and Mitigations


### Drawbacks


## Open Questions [optional]


## Test Plan

End to end testing this feature would:
* enable ShortCertificateRotation featuregate
* observe the cluster for 8 hours
* run minimal testsuite to ensure that main cluster functions are not affected

## Graduation Criteria

This featuregate is not meant to be graduated - its intended to be developer-only setting

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA
N/A

### Removing a deprecated feature


## Upgrade / Downgrade Strategy

Setting DevPreview is permanent - there is no way to upgrade or downgrade the cluster.

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

This setting is unsupported

## Alternatives
