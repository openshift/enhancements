---
title: automated-service-ca-rotation
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
last-updated: 2020-02-17
status: implemented
see-also:
  - "https://docs.google.com/document/d/1VqrIERs30M9EyPaqcO43wcSXHXvfe_Ao2W0ThDIohlM/edit"
replaces:
superseded-by:
---

# Automated Service CA Rotation

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [x] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

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

- The duration of the service CA should be extended from its current value of
  12 months to 26 months and the minimum CA duration should be extended to 13
  months. These values ensure that pods will be guaranteed to be restarted in
  a cluster supported by Red Hat before the expiry of the pre-rotation CA.
  - Pods in the OCP control plane automatically refresh key material
    (certificates and CA bundles) without pod restart.
  - For all other pods, the timing of upgrades is key to determining CA
    duration:
    - Services that do not refresh key material automatically must be
      restarted after CA rotation.
    - Upgrades restart all pods, so ensuring that an upgrade takes place
      after rotation and prior to expiry of the pre-rotation CA guarantees
      that services will always be using valid key material.
  - Computing CA duration:
    - The maximum interval, *I*, between upgrades of a cluster supported by
      Red Hat is 12 months.
      - At most 3 OCP releases are supported at one time, and 3 releases are
        expected in a given year.
      - In the event that an LTS release strategy is chosen, a supported
        cluster would still be expected to be upgraded at least once per year
        if only with a patch release.
    - When the minimum remaining CA duration, *M*, is reached, automatic
      rotation will be triggered. *M* must be greater than *I* to ensure that
      an upgrade occurs before the expiry of the pre-rotation CA.
    - Let the interval between creation of a CA and its rotation be *R*.
      - *R* must be greater than *I* to ensure that an upgrade occurs before
        the expiry of the pre-rotation CA.
      - *R* must be greater than or equal to the minimum remaining CA
        duration, *M*, to ensure that an upgrade occurs before a subsequent
        rotation.
        - Since cross-signing is only supported between the current and
          previous CAs, rotating before all pods have been restarted to use key
          material from the current CA risks breaking trust with key material
          issued by the previous CA.
    - The interval between creation and rotation, *R*, can be computed as the
      total duration *D* less the minimum remaining duration *M*:
      - *R* = *D* - *M*
    - Reordering to solve for *D*:
      - *D* = *R* + *M*
    - Since each of *R* and *M* must be greater than *I*:
      - *D* = *R* + *M* > 2 \* *I*
    - Since *R* >= *M* should be true, simplify to *R* = *M*:
      - *D* = 2 \* *R* > 2 \* *I*
    - Substitute *I* = 12:
      - *D* = 2 \* *R* > 24
    - Picking *R* = *M* = 13 will satisfy the relation, resulting in *D* = 26
  - Worst-case timelines with old and new values for CA duration:
    - Let minimum duration *M* = 6 months, total duration *D* = 12 months:
      - T+0m  - Cluster installed with new CA or existing CA is rotated (CA-1)
      - T+6m  - Automated rotation replaces CA-1 with CA-2 when CA-1 duration < 6m
      - T+12m - Cluster is upgraded and all pods are restarted
      - T+12m - CA-1 expires. If cluster was not upgraded before this
                happens, services using the old key material may
                break.
    - Let minimum duration *M* = 13 months, total duration *D* = 26 months:
      - T+0m  - Cluster installed with new CA or existing CA is rotated (CA-1)
      - T+12m - Cluster is upgraded and all pods are restarted
      - T+13m - Automated rotation replaces CA-1 with CA-2 when CA-1 duration < 13m
      - T+24m - Cluster is upgraded and all pods are restarted
      - T+26m - CA-1 expires. No impact because of the restart at time of upgrade
  - Since all clusters that do not support automated rotation will have CAs with
    a 12 month total duration, the remaining CA duration for those clusters will
    always be less than the minimum upgrade interval of 12 months. Rotation will
    be triggered when upgrading to a release that supports automated CA rotation
    and the cluster administrator should manually restart all pods.
    - Without this intervention, pod restart before expiry of the pre-rotation
      CA cannot be guaranteed.
    - The requirement to manually restart pods after a cluster is upgraded to
      a release that supports automated CA rotation is a one-time thing. All
      subsequent upgrades and rotations will ensure restart before expiry
      without manual intervention.
    - The requirement to manually restart pods after upgrading to a release
      that supports automated rotation will need to be communicated via
      documentation.
      - A Prometheus alert doesn't seem like a viable option except for the
        current pending release (4.4) due to the requirement to key it off of
        an existing metric, and the general policy of not backporting metrics
        and alert additions to previous releases of OCP.

- Detect when the current CA certificate has less than the minimum validity
  duration and generate a new CA.
  - The new CA should have the same `Subject.CommonName` as the current CA to
    ensure that trust chaining is possible.
  - Servers using a serving cert provided by the operator and clients using a ca
    bundle provided by the operator will have until the original expiry of the
    pre-rotation CA to refresh without breaking trust.
    - It's not possible to extend the trust past the original expiry of the
      pre-rotation CA because an unrefreshed participant - a client or server
      using key material with the original expiry - is the limiting factor.

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
  - If the expiry of the current CA is less than the minimum validity duration,
    the new intermediate certificate should be set to expire at the minimum
    validity duration from the time of rotation to ensure that refreshed clients
    will be able to trust unrefreshed servers for that interval.

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
year. Instead, the rotation schema provides a 13 month window when both the old
and new CAs will be trusted. Within that 13 month window, it is critical that
services that consume serving certs and clients that consume CA bundles start
using the serving certs and CA bundle provided by the new CA. For some consumers
this may happen automatically (e.g. via restart on change or hitless
rotation). Others may need to be manually restarted.

OpenShift 4.1 Beta 5 - the first 4.1 beta candidate provided to external
audiences - was made available on May 14, 2019. Without automatic or manual
rotation, the control plane of a cluster installed on that date would start
failing a year later - May 14, 2020 - due to the expiry of the service CA
generated at the time of cluster deployment.

- A 4.1, 4.2 or 4.3 cluster should ideally be upgraded to 4.4 (or a z-stream
patch release that supports rotation) in advance of the 1 year anniversary of
deployment to enable the seamless automated rotation proposed by this
enhancement.

- A release note for any release that supports automated rotation should indicate
the need to restart services after automated CA rotation that are not capable of
automatically picking up changes to service cert secrets or ca bundle
configmaps. To avoid downtime, such services need to be restarted in advance of
the expiry of the service CA created at deployment (i.e. before the 1 year
anniversary of cluster deployment).

- If an upgrade to a release supporting automated rotation were not possible
before the expiry of the service CA, manual rotation (by deleting the signing
secret) could be performed. Manual rotation would not be seamless, however, due
to the lack of a grace period for services and clients to converge on the new
trust chain.

- Testing in advance of service CA rotation whether consumers of CA artifacts are
  refreshing in a timely manner is complicated:
  - Unrefreshed clients and servers would be able to communicate without error
    with both refreshed and unrefreshed clients and servers until the expiry of
    the pre-rotation CA.
  - Detecting clients that do not refresh would require starting a cluster with
    automatic rotation disabled and provisioned with a near-expiry service
    CA. This would allow a subsequent forced rotation to flush out the failure of
    unrefreshed clients when the pre-rotation CA had expired.
  - Without knowing that a given deployment or replicaset was automatically
    responding to a serving cert being regenerated or a new ca bundle being
    injected, a pod that references a serving cert secret or ca bundle configmap
    could simply be deleted if it was started before the most recent rotation. This
    would ensure that the pod was restarted with the current state of the service
    ca artifacts it was consuming.

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
  - https://github.com/openshift/service-ca-operator/blob/master/test/util/rotate.go#L22

- Unit testing should be used to validate the logic of the rotation method.
  - https://github.com/openshift/service-ca-operator/blob/master/pkg/operator/rotate_test.go#L176

- E2E testing should be used to validate the rotation method in combination with
  the required API interaction.
  - https://github.com/openshift/service-ca-operator/blob/master/test/e2e/e2e_test.go#L1174
  - https://github.com/openshift/service-ca-operator/blob/master/test/e2e/e2e_test.go#L1182

- Exploratory testing should be performed to determine if rotation induces
  unexpected behavior in other components.
  - Being able to induce rotation on demand would be necessary, e.g. via
    - API support for forced rotation
    - A tool that renewed the CA with a validity bounds more than half-expired

### Graduation Criteria

Being delivered as GA in 4.4.

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
