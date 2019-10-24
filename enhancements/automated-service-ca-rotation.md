---
title: Automated Service CA Rotation
authors:
  - "@marun"
reviewers:
  - "@deads2k"
  - "@mfojtik"
  - "@mrogers950"
  - "@stlaz"
  - "@sttts"
approvers:
  - TBD
creation-date: 2019-10-23
last-updated: 2019-10-23
status: implementable
see-also:
  - "https://docs.google.com/document/d/1VqrIERs30M9EyPaqcO43wcSXHXvfe_Ao2W0ThDIohlM/edit"
replaces:
superseded-by:
---

# automated-service-ca-rotation

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Enable automated rotation of the service signing CA and ensure that clients
(using an operator-provided CA bundle to verify trust) and servers (serving with
an operator-provided certificate) will be able to continue communicating without
undue disruption.

## Motivation

Service serving certificates are automatically renewed by the service CA
controller. However, the service CA signer is not. The service CA is expired
after 1 year, and once that expiration has passed clients will not be able to
validate server certs, leading to service disruption. Recovering from this
currently requires manual intervention (force rotation of the CA by deleting the
CA secret).

Since 4.1 shipped without automated rotation, operators depending on the service
CA operator will break without manual triggering of forced rotation via deletion
of the signing secret. Since manually-triggered rotation won't provide a grace
period where the old and new CAs are trusted, it is likely to induce a brief
period of system instability while servers and clients converge on the use of the
new trust chain.

### Goals

- The signing CA should be automatically rotated before expiry

- Rotation should prevent trust breakage between consumers of the operator as
  long as the pre-rotation signing CA remains valid. The following should be
  true:
  - A serving cert, from the new CA, validates against the pre-rotation CA
    bundle.
  - A serving cert, from the old CA, validates against the post-rotation CA
    bundle.

- After rotation:
  - Serving cert secrets should be updated with serving certs generated from the new CA.
  - Configmaps annotated for CA bundle injection should be updated with the new CA bundle.
  - API services annotated for CA bundle injection should be updated with the new CA bundle.

- Support forced rotation
  - Rotation triggered by an API change rather than the validity bounds of the CA

- Support forced rotation with a custom validity bounds to support emergency rotation
  - i.e. support minimizing the grace period for trusting the pre-rotation CA

### Non-Goals

- Support the maintenance of trust beyond the new CA and the previous CA.

- Support forced rotation using an external CA and signing key. This could be
  useful for admins to plug in their own intermediate CA and have all serving
  certs issued by it, and also for future integration efforts.

- Implement support for [hitless
rotation](https://diogomonica.com/2017/01/11/hitless-tls-certificate-rotation-in-go/)
so that consumers of serving certs and CA bundles can trivially support automated
refresh. This is already supported by
[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime/pull/421)
and the functionality could be separately extracted for operator reuse.

## Proposal

- Detect when the current CA certificate has passed 1/2 of its validity bounds
  (i.e. 6 months has passed of a 1 year validity) and generate a new CA.
  - The new CA should have the same `Subject.CommonName` as the current CA to
    ensure that trust chaining is possible.
  - This provides a 6 month grace period for consumers of serving certs and ca
    bundles to be refreshed.

- Generate an intermediate CA certificate with the same public key as the new CA
  but signed with the private key of the current CA.
  - This intermediate certificate should be included with newly-generated serving
    certificates, and serves to bridge trust between the CA bundle of an
    unrefreshed client (validating trust with the old CA bundle) and a service
    serving with a cert generated from the new CA.

- Generate an intermediate CA certificate with the same public key as the current CA
  but signed with the private key of the new CA.
  - This intermediate certificate should be included in the the new CA
    bundle along with the new CA's primary certificate, and serves to
    bridge trust between an unrefreshed server (serving with a cert
    generated from the previous CA) and a refreshed client (validating
    trust with the new CA bundle).

- Compare the `AuthorityKeyId` of a serving cert with the `SubjectKeyId` of the
  current CA to determine whether the serving cert was issued by the current CA.
  - If the serving cert is determined not to have been issued by the current CA,
    it needs to be regenerated.
  - Previously, `Subject.CommonName` of a serving cert was compared with
    `Issuer.CommonName` of the CA. Since ensuring trust between old and new CAs
    requires that `Subject.CommonName` be reused, this comparison will no longer
    be sufficient to determine lineage.
  - This requires that all new certificates - whether CA or entity - have
    `AuthorityKeyId` and `SubjectKeyId` set.
  - In addition to ensuring regeneration when `AuthorityKeyId` and `SubjectKeyId`
    are set, this comparison schema will also ensure regeneration of serving
    certs without `SubjectKeyId` set (due to being generated by a CA without
    `AuthorityKeyId` set) since an empty `SubjectKeyId` will not match a
    populated `AuthorityKeyId`.

- Provide an indication that rotation has taken place via an event.

- Allow rotation to be forced via an `unsupportedConfigOverrides`
  entry providing a non-empty string reason for the rotation that does
  not match the value for the
  `service-ca.operators.openshift.io/forced-rotation-reason`
  annotation on the signing secret.
  - Setting an annotation on the signing secret ensures that a forced
    rotation is triggered at most once per reason.
  - If a forced rotation and expiry-triggered rotation coincide, only
    the forced rotation should be performed.

### Risks and Mitigations

Automatic rotation is only effective if consumers of certs and CA bundles will
also refresh automatically before expiry of the pre-rotation CA.

The proposed rotation scheme does not extend the useful lifespan of serving certs
and CA bundles from the pre-rotation CA. Their lifespan will still be 1
year. Instead, the rotation schema provides a 6 month window when both the old
and new CAs will be trusted. Within that 6 month window, it is critical that
services that consume serving certs and clients that consume CA bundles start
using the serving certs and CA bundle provided by the new CA. For some consumers
this may happen automatically (e.g. via restart on change or hitless
rotation. Others may need to be manually restarted.

OpenShift 4.1 Beta 5 - the first 4.1 beta candidate provided to external
audiences - was made available on May 14, 2019. Without automatic or manual
rotation, the control plane of a cluster installed on that date would start
failing a year later - May 14, 2020 - due to the expiry of the service CA
generated at the time of cluster deployment.

- A 4.1 or 4.2 cluster should ideally be upgraded to 4.3 in advance of the 1 year
anniversary of deployment to enable the seamless automated rotation proposed by
this enhancement.

- A 4.3 release note should indicate the need to restart services after automated
CA rotation that are not capable of automatically picking up changes to service
cert secrets or ca bundle configmaps. To avoid downtime, such services need to be
restarted in advance of the expiry of the service CA created at deployment
(i.e. before the 1 year anniversary of 4.1 cluster deployment).

- If an upgrade to 4.3 were not possible before the expiry of the service CA,
manual rotation (by deleting the signing secret) could be performed. Manual
rotation would not be seamless, however, due to the lack of a grace period for
services and clients to converge on the new trust chain.

- Every team should confirm whether any components that interact with service
serving CAs (either by consuming a service that is signed with one, or by
offering a service signed with one) properly handle rotation of the serving cert
or CA bundle (i.e. recognize when it changes, load the new value).

## Design Details

### Test Plan

- To validate that trust will be maintained before, during, and after rotation,
  the following scenarios should be considered:
  - pre-rotation serving cert against pre-rotation CA bundle
  - post-rotation serving cert against pre-rotation CA bundle
  - pre-rotation serving cert against post-rotation CA bundle
  - post-rotation serving cert against post-rotation CA bundle

- Trust can be verified:
  - With `x509.Certificate.Verify`
    - Simpler, but not necessarily representative of real-world use.
  - By starting a server with a serving cert and providing a client calling
    that server with a CA bundle.
    - More complicated, but closer to how a golang client of the service ca
      operator would use a serving cert or a CA bundle.

- Since trust verification does not depend on interaction with a cluster, the
  code to do so could be shared between unit and e2e testing.

- Unit testing should be used to validate the logic of the rotation method.

- E2E testing should be used to validate the rotation method in combination with
  the required API interaction.

- Exploratory testing should be performed to determine if rotation induces
  unexpected behavior in other components.
  - Being able to induce rotation on demand would be necessary, e.g. via
    - API support for forced rotation
    - A tool that renewed the CA with a validity bounds more than half-expired

### Graduation Criteria

Being delivered as GA in 4.3.

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History

Automated Rotation: https://github.com/openshift/service-ca-operator/pull/73
Forced Rotation: https://github.com/openshift/service-ca-operator/pull/77

## Drawbacks

?

## Alternatives

?
