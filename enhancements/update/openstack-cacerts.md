---
title: openstack-cacert
authors:
  - "@stephenfin"
reviewers:
  - "TBD"
approvers:
  - "TBD"
api-approvers:
  - "None"
creation-date: 2025-03-19
last-updated: 2025-03-20
tracking-link:
  - https://issues.redhat.com/browse/OSASINFRA-3729
see-also:
  - "None"
replaces:
  - "None"
superseded-by:
  - "None"
---

# OpenStack CA Cert migration

## Summary

This enhancement proposes an update strategy for migrating the location of the
CA cert used for clusters deployed on OpenStack using self-signed certs.

## Motivation

OpenStack is frequently used as an on-prem cloud solution, and these
deployments often use self-signed certificates. Certificates expire and need to
be rotated when they do, however, there is currently no easy way to rotate
these as they live in multiple places. In addition, the current mechanisms we
have for storing these certificates, namely a CCM-specific config map and
copies of same, are not compatible with OpenStack-specific components like the
OpenStack CAPI Provider, CAPO, and OpenStack Resource Controller (ORC), which
both expect the CA cert to be provided in the credential secret.

In [OSASINFRA-3729](https://issues.redhat.com/browse/OSASINFRA-3729), we have
proposed resolving both issues by making the CA cert part of the credential
secret managed by Cloud Credential Operator (CCO). This has the added ability
to allows us to simplify a number of operators, removing a split between
standalone and hypershift deployments. However, because of CCM's role in
bootstrapping in OpenShift, we cannot rely on CCO-managed credentials (tl;dr:
CCM comes up before CCO does and needs to talk to the cloud so that it can get
instances to deploy pods on). This means we cannot remove the CA cert from the
CCM-specific config map. However, we do not want CA cert rotation to be a
multi-step process.

The dumb solution is to introduce a controller that will either copy the CA
cert from the credential secret to the CCM config map or vice versa. However,
this poses an issue for tooling. If we copy from the credential secret to the
CCM config map, we break hypothetical tooling that is updating the config map,
since all changes to this config map will be overridden. If we copy from the
CCM config map to the credential secret, we prevent new tooling being written
that targets the credential secret since, again, any changes will be
overwritten. We need to find a solution that allow existing tooling to continue
to work for some transition period, while also allowing tooling that uses the
new location to be written.

### User Stories

* As an OpenShift administrator, I want to be able to rotate the CA cert used
  for my cloud as easily as I can rotate my credentials.
* As an OpenShift administrator, I want to continue using my existing CA cert
  rotation scripts for a transition period.

### Goals

* Provide a clear upgrade path for migrating from the "old" way to the "new"
  way that doesn't break existing tooling

### Non-Goals

* Re-litigate having CCO manage the CA cert for us rather than using
  `additionalTrustBundle` and `additionalTrustBundlePolicy = Always`.

  In brief, this approach doesn't address any of our issues for CAPO and ORC,
  would have an even larger upgrade impact, and has a variety of other
  shortcomings that led to current CCM config map-based approach we used.

## Proposal

This enhance proposes to handle upgrades using a syncing controller than adds
hash annotations on the root credential secret and CCM config map. The initial
logic, implemented e.g. via an init container, will work like so:

* If neither the CA cert field of the secret (new location) nor the CA cert
  field of the config map (old location) are populated, do nothing.
* If only the old location is populated, compute a hash of the value and then
  copy the value to the new location. Once copied, set an annotation in both
  places corresponding to the hash.
* If only the new location is populated, compute a hash of the value and then
  copy the value to the old location. Once copied, set an annotation in both
  places corresponding to the hash. **NOTE** This should never happen as the
  installer will always populate the old location when a CA cert is required
  due to the aforementioned bootstrapping issues. However, we will handle it
  just in case.

Once this initial step has completed, the controller will continuously run and
do the following on ever reconciliation loop:

1. Read the CA cert and stored hash from both locations.
2. Compare the hashes:
   * If the hashes match, no action is required.
   * If the hashes match one source (a) but not the other (b), this indicates
     that the value in the other location (b) was changed and we need to
     recalculate the hashes and update (a).
    * If the hashes don't match either source, it could mean that the user
      updated both locations concurrently. In this case, we set the controller
      to degraded and do not overwrite anything.

### Workflow Description

Given a cluster administrator and a working standalone cluster for which the 
administrator is responsible.

**Cluster administrator** is a human user responsible for managing the cluster.

1. The cluster administrator chooses to update their CA cert. They run their
   existing tooling and everything just works (TM).
2. At a later date, the cluster administrator reads the release notes and
   realises that they need to update their tooling. They do so and then run the
   new tooling. Everything continues to just work (TM).

### API Extensions

None.

### Topology Considerations

#### Hypershift / Hosted Control Planes

None. Hypershift does not CCO. We can handle syncing these in the Hypershift
Operator.

#### Standalone Clusters

None. Changes may be necessary to external tooling.

#### Single-node Deployments or MicroShift

Same as standalone clusters.

### Implementation Details/Notes/Constraints

None

### Risks and Mitigations

The risk of getting this wrong is that we break existing tooling. We are not
aware of such tooling, but we're proposing this enhancment out of an abundance
of caution.

### Drawbacks

No drawbacks are known.

## Open Questions [optional]

No open questions.

## Test Plan

* Manual testing.

## Graduation Criteria

### Dev Preview -> Tech Preview

Not applicable.

### Tech Preview -> GA

Not applicable.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

Not applicable.

## Version Skew Strategy

Not applicable.

## Operational Aspects of API Extensions

Not applicable.

## Support Procedures

Not applicable.

## Alternatives

### Require admin ack during upgrades

We treat this change like an API change and note it in the release notes, then
require the admin to patch the `openshift-config / admin-acks` config map.

## Infrastructure Needed [optional]

No new infrastructure is needed.
