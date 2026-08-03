---
title: hypershift-ingress-default-certificate
authors:
  - "@deads2k"
reviewers:
  - "@csrwng, for HyperShift control plane architecture"
  - "@JoelSpeed, for API review"
approvers:
  - "@csrwng"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-08-03
last-updated: 2026-08-03
status: implementable
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3499
see-also:
  - "/enhancements/hypershift/api-driven-azure-topology-and-private-connectivity.md"
replaces:
  - N/A
superseded-by:
  - N/A
---

# Custom Default Ingress Certificate for HyperShift

## Summary

This enhancement adds a `defaultCertificate` field to `IngressOperatorSpec` in
the HyperShift HostedCluster and HostedControlPlane APIs. This allows managed-service operators (e.g. ARO, ROSA) to specify a
custom TLS certificate for the default ingress controller through the
management-plane API, instead of relying on the auto-generated wildcard
certificate or requiring day-2 guest-cluster mutations that are continuously
overwritten by reconciliation.

## Motivation

Today, HyperShift's HostedCluster ConfigOperator (HCCO) hardcodes the
IngressController's `defaultCertificate` to always point at a HyperShift-generated
wildcard TLS certificate. The certificate flow is:

1. The Control Plane Operator (CPO) generates an `ingress-crt` Secret in the
   control plane namespace.
2. HCCO copies this Secret's data into a `default-ingress-cert` Secret in the
   guest cluster's `openshift-ingress` namespace.
3. The IngressController CR references `default-ingress-cert` as its
   `spec.defaultCertificate`.

Because the `default-ingress-cert` Secret is continuously reconciled by HCCO,
any direct edits to it in the guest cluster are overwritten. There is no
first-class API field on the HostedCluster or HostedControlPlane to override
this behavior.

Managed-service operators (ARO, ROSA) need to supply their own TLS
certificates — signed by a trusted public CA — for customer-facing ingress
endpoints. Without a management-plane API for this,
they must either:

- Patch the guest cluster directly (which gets reverted by reconciliation), or
- Fork or modify HyperShift internals.

Neither approach is supportable or sustainable.

### User Stories

#### Story 1: Managed-Service Operator (ARO/ROSA)

As a managed-service operator (ARO or ROSA), I want to specify a custom TLS
certificate for a hosted cluster's default ingress controller via the
HostedCluster API, so that customer-facing routes are served with a certificate
signed by a trusted public or corporate CA without requiring day-2
guest-cluster modifications that get overwritten.

#### Story 2: Managed-Service Certificate Provisioning

As a managed-service operator (ARO or ROSA), I want to supply a TLS
certificate for the default ingress controller at cluster creation time through
the management plane, so that all customer routes immediately use the correct
certificate without manual post-deployment steps.

#### Story 3: Certificate Rotation

As a managed-service operator, I want to rotate the default ingress certificate
by updating the source Secret in the HostedCluster namespace, so that the new
certificate automatically propagates to the guest cluster without downtime or
manual intervention.

#### Story 4: Operations Monitoring

As an operations engineer, I want to verify that the custom certificate is
correctly propagated to the guest cluster through observable ConfigMaps (e.g.
`observed-default-ingress-cert`), so that I can confirm TLS configuration
correctness without directly accessing the guest cluster.

### Goals

1. Provide a first-class, optional `defaultCertificate` field on
   `IngressOperatorSpec` in the HostedCluster/HostedControlPlane API that
   references a TLS Secret in the HostedCluster namespace.
2. Automatically sync the referenced TLS Secret from the HostedCluster namespace
   through the control plane namespace into the guest cluster's
   `openshift-ingress` namespace.
3. Preserve backward compatibility: when the field is not set, the existing
   CPO-generated wildcard certificate is used.
4. Support certificate rotation by detecting changes to the source Secret and
   propagating the updated certificate data through the sync chain.
5. Enable managed-service operators to configure ingress certificates entirely
   through the management plane.

### Non-Goals

1. **Guest-cluster-initiated certificate overrides**: This enhancement does not
   provide a mechanism for guest cluster users to override the default ingress
   certificate independently of the management plane. The management plane
   remains the source of truth.
2. **Per-route certificate management**: This enhancement covers only the
   *default* ingress certificate. Per-route TLS configuration remains the
   responsibility of route owners.
3. **Certificate generation or renewal**: This enhancement does not generate or
   auto-renew certificates. Users must supply and manage their own TLS
   Secrets.
4. **Wildcard certificate customization**: This does not change how the CPO
   generates the fallback wildcard certificate.
5. **Standalone cluster support**: This feature targets HyperShift-managed
   clusters only. Standalone clusters already support configuring
   `spec.defaultCertificate` directly on the IngressController CR.

## Proposal

Add an optional `DefaultCertificate` field of type
`IngressDefaultCertificateReference` to `IngressOperatorSpec` in the
HyperShift v1beta1 API. When set, the referenced TLS Secret in the
HostedCluster namespace is synced through the control plane namespace into the
guest cluster's `openshift-ingress` namespace as the `default-ingress-cert`
Secret. The IngressController CR continues to reference the same
`default-ingress-cert` Secret name — only the data source changes.

The sync chain is:

1. **HostedCluster namespace** (management cluster): User creates a TLS Secret
   containing `tls.crt` and `tls.key`.
2. **HyperShift Operator**: `reconcileIngressDefaultCertSync` validates that
   `tls.crt` and `tls.key` exist in the source Secret, then copies the Secret
   data to the control plane namespace.
3. **HCCO** (`resources.go`): Copies the synced certificate data into the
   `default-ingress-cert` Secret in the guest cluster's `openshift-ingress`
   namespace, replacing the CPO-generated wildcard certificate data.
4. **ManagedCA Observer**: Syncs the observed CA back to the management cluster
   as `observed-default-ingress-cert` ConfigMap for observability.

When the field is not set, the existing behavior is preserved: the CPO-generated
wildcard certificate is used as the default ingress certificate.

### Workflow Description

**Managed-service operator** is a human user or automation system (e.g. ARO or
ROSA service infrastructure) responsible for managing HostedClusters.

**HyperShift operator** is the controller running in the management cluster
that reconciles HostedCluster resources.

**HCCO** (HostedCluster ConfigOperator) is the controller running in the
control plane namespace that reconciles guest cluster resources.

#### Initial Setup

1. The managed-service operator creates a TLS Secret in the
   HostedCluster namespace containing `tls.crt` (certificate chain) and
   `tls.key` (private key):

   ```yaml
   apiVersion: v1
   kind: Secret
   metadata:
     name: my-custom-ingress-cert
     namespace: clusters
   type: kubernetes.io/tls
   data:
     tls.crt: <base64-encoded-certificate-chain>
     tls.key: <base64-encoded-private-key>
   ```

2. The managed-service operator sets the `defaultCertificate` field on
   the HostedCluster's `spec.configuration.ingress`:

   ```yaml
   apiVersion: hypershift.openshift.io/v1beta1
   kind: HostedCluster
   metadata:
     name: my-cluster
     namespace: clusters
   spec:
     configuration:
       ingress:
         defaultCertificate:
           name: my-custom-ingress-cert
   ```

3. The HyperShift operator detects the `defaultCertificate` reference,
   validates that the referenced Secret exists and contains `tls.crt` and
   `tls.key`, and syncs the Secret data into the control plane namespace.

4. HCCO detects the synced certificate data and writes it into the
   `default-ingress-cert` Secret in the guest cluster's `openshift-ingress`
   namespace.

5. The openshift-ingress-operator detects the updated `default-ingress-cert`
   Secret and configures the router to use the new certificate.

6. The ManagedCA Observer syncs the observed certificate back to the
   management cluster as `observed-default-ingress-cert`.

#### Certificate Rotation

1. The managed-service operator updates the TLS Secret referenced by
   `defaultCertificate` with new `tls.crt` and `tls.key` data.

2. The HyperShift operator detects the Secret change and re-syncs the updated
   data into the control plane namespace.

3. HCCO propagates the updated data to the guest cluster.

4. The router picks up the new certificate. No restart or reconfiguration of
   the HostedCluster resource is required.

#### Fallback (Field Not Set)

1. When `defaultCertificate` is not set (nil or omitted), the existing behavior
   is preserved: the CPO-generated wildcard certificate is used as the source
   for the `default-ingress-cert` Secret.

### API Extensions

This enhancement adds the following API extension to the HyperShift v1beta1 API
(`api/hypershift/v1beta1/operator.go`):

```go
// IngressOperatorSpec specifies configuration for the ingress controller.
type IngressOperatorSpec struct {
    // ... existing fields ...

    // defaultCertificate is an optional reference to a TLS Secret in the
    // HostedCluster namespace. When set, the referenced Secret's tls.crt
    // and tls.key are used as the default certificate for the ingress
    // controller, replacing the auto-generated wildcard certificate.
    // +optional
    DefaultCertificate *IngressDefaultCertificateReference `json:"defaultCertificate,omitempty"`
}

// IngressDefaultCertificateReference references a TLS Secret by name.
type IngressDefaultCertificateReference struct {
    // name is the name of a kubernetes.io/tls Secret in the HostedCluster
    // namespace. The Secret must contain tls.crt and tls.key entries.
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=253
    Name string `json:"name"`
}
```

CEL validation ensures the `name` field is non-empty when the
`defaultCertificate` object is present. The field uses `omitempty` and
`omitzero` serialization tags so that N-1 HyperShift versions safely ignore
the new field during upgrades.

This extension does not modify the behavior of any existing resources owned by
other teams. The IngressController CR in the guest cluster continues to
reference the same `default-ingress-cert` Secret name; only the data source
changes.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is **exclusively designed for HyperShift / Hosted Control
Planes**. It adds an API field to the HostedCluster and HostedControlPlane
resources and modifies the certificate sync chain between the management
cluster and guest cluster.

Components affected:

- **Management cluster (HyperShift Operator)**: New reconciliation function
  `reconcileIngressDefaultCertSync` in the HostedCluster controller syncs the
  referenced TLS Secret from the HostedCluster namespace to the control plane
  namespace.
- **Control plane namespace (HCCO)**: The `resources.go` controller reads the
  synced certificate data and writes it into the guest cluster's
  `default-ingress-cert` Secret.
- **Control plane namespace (ManagedCA Observer)**: Syncs observed certificate
  data back to the management cluster for observability.

No guest cluster components are modified. The IngressController and router
operate on the existing `default-ingress-cert` Secret as before.

#### Standalone Clusters

This change is not relevant for standalone clusters. Standalone clusters already
support configuring `spec.defaultCertificate` directly on the IngressController
CR. This enhancement addresses the HyperShift-specific gap where that
IngressController field is continuously reconciled by HCCO.

#### Single-node Deployments or MicroShift

This proposal does not affect single-node OpenShift (SNO) or MicroShift
deployments. The API change is scoped to the HyperShift HostedCluster /
HostedControlPlane API, which is not used by SNO or MicroShift. There is no
impact on resource consumption for these deployment topologies.

#### OpenShift Kubernetes Engine

This proposal does not depend on features excluded from the OpenShift
Kubernetes Engine (OKE) product offering. The ingress controller and TLS
certificate handling are part of the base platform available in both OCP and
OKE. However, HyperShift itself may have separate OKE eligibility
considerations that are outside the scope of this enhancement.

### Implementation Details/Notes/Constraints

#### Sync Chain Architecture

The implementation follows the established HyperShift pattern for syncing
resources from the management cluster to the guest cluster:

1. **Source validation**: The HyperShift Operator validates that the referenced
   Secret exists and contains both `tls.crt` and `tls.key` keys before
   attempting to sync. This provides early feedback if the Secret is malformed.

2. **Two-hop sync**: The Secret data is synced in two hops — first from the
   HostedCluster namespace to the control plane namespace (by the HyperShift
   Operator), then from the control plane namespace to the guest cluster (by
   HCCO). This matches the existing architecture for other synced resources.

3. **Data replacement, not reference change**: The IngressController CR in the
   guest cluster continues to reference `default-ingress-cert` by name. Only
   the data within that Secret changes. This minimizes blast radius and avoids
   modifying the IngressController reconciliation logic.

4. **Observability via CA Observer**: The ManagedCA Observer already watches
   for certificate changes in the guest cluster. When the custom certificate
   is propagated, the observer syncs the corresponding CA data back to the
   management cluster as `observed-default-ingress-cert` ConfigMap.

#### Prior Art

- The `endpointPublishingStrategy` field is already exposed on
  `IngressOperatorSpec` in the same way, establishing precedent for ingress
  configuration through the HostedCluster API.
- HOSTEDCP-898 (closed October 2024) was an earlier request for the same
  capability.
- ARO-26911 and ARO-28183 are downstream dependencies that require this
  feature for Azure Red Hat OpenShift.

#### Serialization Compatibility

The implementation includes serialization round-trip tests that verify an N-1
HyperShift version (which does not know about the `defaultCertificate` field)
can safely unmarshal and re-marshal a HostedCluster resource without losing
other fields. The `omitempty` and `omitzero` tags ensure the field is not
included in serialized output when not set.

### Risks and Mitigations

**Risk: Invalid or expired certificate supplied by user.**
Mitigation: The HyperShift Operator validates that `tls.crt` and `tls.key`
exist in the referenced Secret. However, it does not validate certificate
expiry or chain completeness. This is consistent with how standalone OpenShift
handles user-supplied certificates — the responsibility for certificate
validity lies with the managed-service operator. Monitoring and alerting for certificate
expiry should be handled by the user's certificate management tooling.

**Risk: Secret deleted after being referenced.**
Mitigation: If the referenced Secret is deleted, the sync chain will not have
new data to propagate. The existing certificate in the guest cluster remains in
place until the Secret is recreated or the `defaultCertificate` field is
cleared. The HyperShift Operator should set a condition indicating the
referenced Secret is missing.

**Risk: Privilege escalation via Secret access.**
Mitigation: The referenced Secret must exist in the same HostedCluster
namespace. The HyperShift Operator already has RBAC access to Secrets in this
namespace. No additional RBAC grants are required. Guest cluster users cannot
influence which Secret is referenced.

**Risk: Reconciliation race conditions.**
Mitigation: The sync chain uses the same reconciliation patterns as other
HyperShift-synced resources. Kubernetes' optimistic concurrency (resource
versions) prevents data corruption from concurrent updates.

### Drawbacks

- **Additional API surface**: Adding a new optional field to the HostedCluster
  API increases the API surface that must be maintained, documented, and
  tested. However, this field follows established patterns (similar to
  `endpointPublishingStrategy`) and the maintenance burden is minimal.

- **No certificate validation**: The implementation validates Secret structure
  (presence of `tls.crt` and `tls.key`) but does not validate certificate
  content (expiry, chain completeness, key match). This is a deliberate
  trade-off to avoid scope creep and to remain consistent with how standalone
  OpenShift handles user-supplied certificates.

- **Management-plane-only control**: Guest cluster users cannot override the
  default ingress certificate independently. This is by design for
  managed-service scenarios (ARO, ROSA) where the management plane is the
  authoritative source.

## Alternatives (Not Implemented)

### Alternative 1: Guest-Cluster-Side Configuration

Allow guest cluster users to configure the default ingress certificate
directly on the IngressController CR, with HCCO respecting (not overwriting)
user-supplied values.

**Rejected because**: This breaks the management-plane-as-source-of-truth model
that HyperShift and managed services rely on. It would also require significant
changes to HCCO's reconciliation logic to detect and preserve user-supplied
values, introducing complexity and potential for drift between management-plane
intent and guest-cluster state.

### Alternative 2: Annotation-Based Configuration

Use an annotation on the HostedCluster resource to reference the certificate
Secret, rather than a first-class API field.

**Rejected because**: Annotations are not validated, not documented in API
schemas, and not discoverable through `kubectl explain`. A first-class API
field provides type safety, CEL validation, and proper API documentation. The
`endpointPublishingStrategy` precedent demonstrates that ingress configuration
belongs as a structured field.

### Alternative 3: ConfigMap-Based Certificate Delivery

Deliver the certificate via a ConfigMap instead of a Secret.

**Rejected because**: TLS private keys are sensitive data that should be stored
in Secrets, not ConfigMaps. Using a ConfigMap would violate Kubernetes security
conventions and could expose private keys to unauthorized parties through
ConfigMap-watching tools.

## Open Questions [optional]

1. **Removal/revert behavior**: The current E2E tests do not cover the case
   where `defaultCertificate` is set and then removed (reverted to nil). The
   expected behavior is that the system reverts to using the CPO-generated
   wildcard certificate, but this path needs explicit testing.

2. **Status reporting**: Should the HostedCluster status include a condition
   reflecting the state of the certificate sync (e.g. `IngressCertificateSynced`)?
   The current implementation relies on existing reconciliation health reporting.

## Test Plan

The test plan covers the following areas:

### Unit Tests

- Serialization round-trip tests verifying that an N-1 HyperShift version can
  safely ignore the new `defaultCertificate` field.
- Validation tests for the `IngressDefaultCertificateReference` type (CEL
  validation, `MinLength`/`MaxLength` constraints).

### E2E Tests

The E2E test suite (implemented in PR openshift/hypershift#9132) covers:

1. **Certificate provisioning**: Create a self-signed CA and issue a TLS
   certificate. Set `defaultCertificate` on the HostedCluster. Verify that the
   certificate data appears byte-for-byte in the guest cluster's
   `default-ingress-cert` Secret in the `openshift-ingress` namespace.

2. **TLS handshake verification**: Create a canary Route in the guest cluster
   and perform a TLS handshake against it, validating that the presented
   certificate matches the custom certificate (not the default wildcard).

3. **Certificate rotation**: Update the source Secret with a new certificate
   and key. Verify that the updated certificate propagates to the guest cluster
   and is presented by the router.

4. **Observability**: Verify that the `observed-default-ingress-cert` ConfigMap
   is synced back to the management cluster and contains the expected CA data.

### Known Test Gaps

- **Removal/revert**: The E2E suite does not currently test removing the
  `defaultCertificate` field after it has been set. This should be added
  before GA promotion.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to set `defaultCertificate` on a HostedCluster and have the
  certificate propagated to the guest cluster's default ingress controller.
- E2E test coverage for provisioning, TLS handshake, and rotation.
- API stability sufficient for early adopters.
- Documentation for managed-service operators (ARO, ROSA).

### Tech Preview -> GA

- Removal/revert test coverage (clearing `defaultCertificate` reverts to
  wildcard certificate).
- Soak time in ARO/ROSA staging environments.
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/).
- Confirmation from ARO and ROSA teams that the feature meets their
  requirements (ARO-26911, ARO-28183).
- Load testing with certificate rotation under high route counts.

### Removing a deprecated feature

Not applicable. This is a new feature with no deprecated predecessor.

## Upgrade / Downgrade Strategy

### Upgrade (N to N+1)

No action is required from existing clusters on upgrade. The new
`defaultCertificate` field is optional and defaults to nil. Existing clusters
that do not set this field continue to use the CPO-generated wildcard
certificate without any behavior change.

Clusters that adopt the new field after upgrade will begin using the custom
certificate on the next reconciliation cycle.

### Downgrade (N+1 to N)

If a cluster using `defaultCertificate` is downgraded to a version that does
not recognize the field:

- The N version's HyperShift Operator will ignore the unknown
  `defaultCertificate` field (verified by serialization round-trip tests).
- The sync chain for the custom certificate will stop operating.
- HCCO will revert to using the CPO-generated wildcard certificate as the
  data source for `default-ingress-cert`.
- The ingress controller will begin serving the wildcard certificate again.

This is a graceful degradation — the cluster remains functional with the
default wildcard certificate. Managed-service operators should be aware that their custom
certificate will no longer be served after downgrade.

## Version Skew Strategy

During an upgrade, the HyperShift Operator and HCCO may temporarily be at
different versions:

- **New Operator, old HCCO**: The Operator syncs the custom certificate to the
  control plane namespace, but the old HCCO does not read the custom
  certificate field. The guest cluster continues using the wildcard
  certificate until HCCO is upgraded. This is safe — the certificate will be
  propagated once HCCO catches up.

- **Old Operator, new HCCO**: The old Operator does not know about
  `defaultCertificate` and does not sync the custom certificate. The new HCCO
  falls back to the CPO-generated wildcard certificate (existing behavior).
  This is also safe.

In both cases, the worst outcome is temporary use of the wildcard certificate.
There is no risk of serving a partial or corrupted certificate.

## Operational Aspects of API Extensions

This enhancement adds a single optional field to the existing HostedCluster
CRD. The operational impact is minimal:

- **No new webhooks**: The field uses CEL validation markers on the CRD schema.
  No admission or conversion webhooks are introduced.
- **No new CRDs**: No new Custom Resource Definitions are created.
- **No new finalizers**: No finalizers are added to any resources.
- **Existing SLI impact**: Negligible. The additional reconciliation work
  (syncing one Secret) is comparable to existing Secret sync operations in
  HyperShift. No measurable impact on API throughput or latency.
- **Failure mode**: If the referenced Secret is missing or malformed, the
  HyperShift Operator's reconciliation will log an error and the existing
  certificate remains in place. The HostedCluster does not become degraded.

## Support Procedures

**Symptom: Custom certificate not appearing on guest cluster routes.**

Diagnosis:
1. Verify the source Secret exists in the HostedCluster namespace and contains
   `tls.crt` and `tls.key`:
   ```
   oc get secret <secret-name> -n <hosted-cluster-namespace> -o jsonpath='{.data}'
   ```
2. Check the HyperShift Operator logs for errors related to certificate sync:
   ```
   oc logs -n hypershift deployment/operator -c operator | grep -i "ingress.*cert"
   ```
3. Verify the certificate was synced to the control plane namespace:
   ```
   oc get secret default-ingress-cert -n <control-plane-namespace> -o jsonpath='{.data}'
   ```
4. Verify the certificate reached the guest cluster:
   ```
   oc --kubeconfig=<guest-kubeconfig> get secret default-ingress-cert -n openshift-ingress -o jsonpath='{.data}'
   ```
5. Check the `observed-default-ingress-cert` ConfigMap on the management
   cluster for CA observability data.

**Symptom: Certificate rotation not taking effect.**

Diagnosis:
1. Confirm the source Secret was actually updated (check `resourceVersion`).
2. Check HyperShift Operator reconciliation logs for the HostedCluster.
3. Verify HCCO logs in the control plane namespace for Secret sync activity.
4. The router may take up to 60 seconds to reload after the Secret is updated.

**Disabling the feature:**

Remove the `defaultCertificate` field from the HostedCluster spec. The system
will revert to using the CPO-generated wildcard certificate on the next
reconciliation cycle. No data loss occurs. Running workloads are unaffected —
only the TLS certificate presented by the router changes.

## Infrastructure Needed [optional]

No new infrastructure is needed. The feature uses existing HyperShift CI
infrastructure for E2E testing. The E2E tests create self-signed certificates
at test time and do not require external certificate authority infrastructure.
