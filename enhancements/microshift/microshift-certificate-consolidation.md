---
title: microshift-certificate-consolidation
authors:
  - "@eslutsky"
reviewers:
  - "@pacevedom"
  - "@copejon"
  - "@pmtk"
approvers:
  - "@pacevedom"
  - "@pmtk"
api-approvers:
  - "None"
creation-date: 2026-07-29
last-updated: 2026-09-10
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-2900
see-also:
  - "/enhancements/microshift/microshift-apiserver-certs.md"
  - "/enhancements/microshift/microshift-certificate-rotation.md"
replaces:
  - N/A
superseded-by:
  - N/A
---

# MicroShift Certificate Authority Consolidation

## Summary

This enhancement consolidates MicroShift's CA hierarchy from 12 CAs to 6
to conform to ProdSec guidance for single-node deployments: two new
consolidated CAs (client-ca, serving-ca), one renamed CA (peer-ca, formerly
etcd-signer), one retained standalone CA (aggregator-signer), and two
unchanged CAs (service-ca, ingress-ca). The change is scoped exclusively to
CAs — it only affects which CA signs each internally-generated certificate,
not the certificates themselves or their content.

## Motivation

MicroShift's current PKI inherits 12 CAs from OpenShift's multi-node
architecture, where separate CAs enable independent trust domains across nodes —
for example, one node's kubelet does not need to trust another node's API server
client certificate. On a single-node device, however, all certificates are
consumed by processes on the same host, making trust domain separation
meaningless. ProdSec guidance for single-node deployments formalizes this:
reduce the CA count to the minimum needed.

Importantly, consolidating CAs only changes **which CA signs** each
internally-generated certificate — the leaf certificates, their SANs, and
their usage remain unchanged.

The current structure also creates maintenance burden: each CA has its own
validity lifecycle, its own trust bundle membership, and its own renewal
logic. Consolidation simplifies certificate renewal (OCPSTRAT-2899), reduces
the number of files on disk, and makes the PKI easier to reason about for
both developers and field engineers.

This enhancement co-ships with OCPSTRAT-2899 (Controlled Certificate and CA Renewal).
OCPSTRAT-2899 introduces a PKI inventory abstraction that catalogs all managed
certificates by role and tracks parent-child relationships between CAs and their
issued certificates, decoupling the `microshift certs` CLI from the specific
on-disk layout. This enhancement updates that inventory to reflect the new 6-CA
hierarchy, so that `microshift certs status`, `microshift certs renew --serving`,
and `microshift certs renew --ca` operate correctly without any layout-specific
changes to the CLI code.

### User Stories

* As a MicroShift platform engineer, I want fewer CAs to manage so that
  certificate renewal and troubleshooting are simpler.
* As a MicroShift operator, I want the certificate layout to reflect the
  single-node deployment model so that I can understand and verify the PKI
  without deep OpenShift knowledge.
* As a MicroShift developer, I want a simpler `certSetup()` function so that
  future changes (controlled cert renewal, multi-node) are easier to implement.
* As a security reviewer, I want the PKI to follow upstream Kubernetes
  recommendations (client CA, serving CA, etcd CA) so that audits are
  straightforward.

### Goals

* Reduce the number of managed CAs from 12 to 6.
* Consolidate the 3 KAS serving certificates into 1 SAN-based certificate.
* Preserve all leaf certificate identities (CN/O fields) so that Kubernetes
  RBAC authorization is unaffected.
* Provide an automatic upgrade path that requires a single service restart/interruption.
* Support atomic rollback via greenboot/ostree.

### Non-Goals

* Changing the service-ca operator or its behavior in any way.
* Preserving a customer-replaced ingress CA across the 5.1 upgrade: ingress-ca
  is regenerated during migration. Customers who have replaced the default
  ingress CA with a custom certificate must re-provision it after upgrading
  to 5.1. See Alternatives for rationale.
* Implementing the `microshift certs` CLI subcommands, PKI inventory abstraction,
  or configurable certificate validity — those are covered by OCPSTRAT-2899,
  which co-ships with this enhancement.
* Changing **leaf** certificate validity periods or renewal thresholds.
  CA validity is discussed in Implementation Details; three 1-year client CAs
  are consolidated into a 10-year `client-ca` as an explicit scope change
  (see "CA validity change" below).
* Modifying the `certchains` builder framework API itself.
* Changing the service account signing key mechanism.

## Proposal

### Current State

MicroShift maintains 12 CAs organized into three categories:

**Client certificate signers (5 CAs):**
| CA | Leaf certificates signed |
|----|--------------------------|
| kube-control-plane-signer | kube-controller-manager, kube-scheduler, cluster-policy-controller, route-controller-manager |
| kube-apiserver-to-kubelet-signer | kube-apiserver-to-kubelet-client, metrics-server-kubelet-client |
| admin-kubeconfig-signer | admin-kubeconfig-client, openshift-observability-client |
| kubelet-csr-signer-signer → kube-csr-signer (sub-CA) | kubelet-client, kubelet-server |
| aggregator-signer | aggregator-client *(listed here for completeness; retained as a standalone CA in the target state — see below)* |

**Serving certificate signers (3 CAs):**
| CA | Leaf certificates signed |
|----|--------------------------|
| kube-apiserver-external-signer | kube-external-serving |
| kube-apiserver-localhost-signer | kube-apiserver-localhost-serving |
| kube-apiserver-service-network-signer | kube-apiserver-service-network-serving |

**Peer certificate signer (1 CA):**
| CA | Leaf certificates signed |
|----|--------------------------|
| etcd-signer | apiserver-etcd-client, etcd-peer, etcd-serving |

**Unchanged signers (2 CAs):**
| CA | Leaf certificates signed | Reason unchanged |
|----|--------------------------|------------------|
| service-ca | route-controller-manager-serving | Pod-managed CA with its own renewal mechanism; dynamically signs workload service certs at runtime |
| ingress-ca | router-default-serving | Customer-replaceable; must remain independently configurable |

Note: kubelet-csr-signer-signer contains a sub-CA (kube-csr-signer) making it
effectively 2 CAs, for a total of 12 (11 root + 1 sub-CA).

### Target State

**New CAs:**

`client-ca` replaces 4 signer entries — kube-control-plane-signer,
kube-apiserver-to-kubelet-signer, admin-kubeconfig-signer, and the
two-level kubelet-csr-signer-signer/kube-csr-signer pair (a root signer
and its sub-CA, counting as 2 CA objects) — for a total of 5 CA objects
eliminated — and signs client certificates:

| Leaf certificate | CN | O (groups) |
|------------------|----|------------|
| kube-controller-manager | system:kube-controller-manager | — |
| kube-scheduler | system:kube-scheduler | — |
| cluster-policy-controller | system:kube-controller-manager | — |
| route-controller-manager | system:serviceaccount:openshift-route-controller-manager:route-controller-manager-sa | — |
| kube-apiserver-to-kubelet-client | system:kube-apiserver | kube-master |
| metrics-server-kubelet-client | system:metrics-server | — |
| admin-kubeconfig-client | system:admin | system:masters |
| openshift-observability-client | openshift-observability-client | — |
| kubelet-client | system:node:\<nodename\> | system:nodes |

`aggregator-signer` is retained as a standalone CA. The kube-apiserver uses
its signed certificate when proxying requests to aggregated APIs; Kubernetes
validates these via the `requestheader-client-ca-file` configuration, which
is separate from the general `client-ca-file`. Merging aggregator-signer into
client-ca would allow any client certificate to satisfy requestheader
validation, enabling impersonation of the apiserver in aggregated API calls:

| Leaf certificate | CN | O (groups) |
|------------------|----|------------|
| aggregator-client | system:openshift-aggregator | — |

`serving-ca` replaces 3 CAs (kube-apiserver-external-signer,
kube-apiserver-localhost-signer, kube-apiserver-service-network-signer) and
signs serving certificates:

| Leaf certificate | SANs |
|------------------|------|
| kube-apiserver-serving | kubernetes, kubernetes.default, kubernetes.default.svc, kubernetes.default.svc.cluster.local, openshift, openshift.default, openshift.default.svc, openshift.default.svc.cluster.local, api.\<basedomain\>, api-int.\<basedomain\>, \<advertise-address\>, \<service-ip\>, localhost, 127.0.0.1, ::1, \<hostname\>, \<node-ip\>, \<subject-alt-names\> |
| kubelet-server | \<hostname\>, \<node-ip\> |

`peer-ca` replaces etcd-signer and signs all etcd peer, serving, and client
certificates. The rename aligns with the ProdSec conceptual model; the CA
remains isolated because etcd manages its own TLS verification independently
of Kubernetes — it cannot be merged into `client-ca` or `serving-ca` without
conflating Kubernetes RBAC trust with etcd's internal trust domain:

| Leaf certificate | Role |
|------------------|------|
| apiserver-etcd-client | kube-apiserver client cert for etcd |
| etcd-peer | etcd peer-to-peer TLS |
| etcd-serving | etcd server TLS |

**Unchanged CAs:** service-ca, ingress-ca, and aggregator-signer remain identical to the current layout.

### Certificate Identity Preservation

All leaf certificate Common Name (CN) and Organization (O) fields remain
identical to their current values. Kubernetes RBAC determines identity from
these fields, not from which CA signed the certificate. Changing the parent
CA has zero impact on authorization.

### KAS Serving Certificate Consolidation

Currently, the KAS uses 3 separate serving certificates for external,
localhost, and service-network access, each signed by a different CA. The
KAS `dynamiccertificates` package selects which certificate to serve based
on SNI or destination IP matching.

With consolidation, a single serving certificate contains all SANs from the
3 old certificates. Since all connections to the KAS will match this single
certificate regardless of the destination IP or hostname used, the
`dynamiccertificates` selection logic continues to work — it simply always
matches the same certificate.

### Workflow Description

**Fresh install:**
1. MicroShift starts and calls `initCerts()`.
2. `certSetup()` builds the new 6-CA hierarchy.
3. All leaf certs, kubeconfigs, and trust bundles are generated.
4. MicroShift starts normally.

**Upgrade from pre-consolidation version:**
1. New MicroShift binary starts and calls `initCerts()`.
2. Migration logic detects the old CA directory layout by checking for the
   existence of `<datadir>/certs/kube-control-plane-signer/`.
3. The entire `certs/` directory is renamed to
   `certs.backup.<version>.<timestamp>/`. This `os.Rename` is atomic on the
   same filesystem — no partial rename state is observable. No dedicated
   migration marker file is needed: the absence of `certs/kube-control-plane-signer/`
   on the next startup is the migration detection signal.
4. The migration recreates the `certs/` directory and copies `service-ca`
   back into it from the backup, so that existing in-cluster service-serving
   Secrets remain trusted until the service-ca controller reissues them.
   `ingress-ca` is **not** restored; it is regenerated fresh (see Non-Goals).
5. `certSetup()` finds the new `certs/` directory (with `service-ca` already
   restored) and generates all remaining CAs and leaf certificates under the
   new 6-CA hierarchy.
6. Kubeconfigs are regenerated with the new serving-ca as the trust anchor.
7. On ostree/bootc deployments, all pods restart as part of the upgrade
   and pick up the updated CA bundle via their projected volumes. This is
   existing MicroShift upgrade behavior, not new to this enhancement.

**Rollback (greenboot/ostree):**
1. If greenboot detects an unhealthy system after upgrade, it triggers an
   atomic rollback to the previous OS commit.
2. The old MicroShift binary starts and finds the new-layout `certs/`
   directory (containing `client-ca/`, `serving-ca/`, `peer-ca/`, etc.)
   left behind by the upgrade. It does not recognize the new directory names.
3. The old binary detects that its expected signer directories (e.g.
   `kube-control-plane-signer/`) are absent and falls back to its normal
   cert-regeneration path, recreating the old layout from scratch.
4. Services start normally with the regenerated old-layout PKI.
5. No manual intervention required. Note: certificate-state rollback is
   recovery through regeneration — it is not atomic. Greenboot atomically
   rolls back the OS deployment, but cert material is rebuilt on first
   startup of the old binary.

**Backup cleanup:**
The `certs.backup.<version>.<timestamp>/` directory is left on disk and not
automatically deleted. The timestamp suffix (Unix seconds at migration time)
avoids name collisions on an upgrade → rollback → re-upgrade cycle, where
the same version number would otherwise overwrite a prior backup. Documentation
will advise operators to remove old backups after confirming an upgrade is
stable. Edge devices have sufficient disk capacity for one backup.

### CA Bundle Changes

| Bundle file | Current contents | New contents |
|-------------|-----------------|--------------|
| `ca-bundle/client-ca.crt` | 5 CA certs concatenated | single client-ca cert |
| `ca-bundle/kubelet-ca.crt` | kubelet-csr-signer CA | client-ca cert |
| `ca-bundle/kubelet-serving-ca.crt` | kubelet-csr-signer CA | serving-ca cert |
| `ca-bundle/service-account-token-ca.crt` | 3 KAS serving CAs | serving-ca cert |
| `ca-bundle/ca-bundle.crt` | all serving CAs | serving-ca + peer-ca certs |

The consolidated CAs are published as Kubernetes Secrets in the `kube-system`
namespace under their new names: `client-ca`, `serving-ca`, `peer-ca`.
The retained CAs (`aggregator-signer`, `service-ca`, `ingress-ca`) and the
`service-account-key` Secret keep their existing names. The old per-signer
Secrets (`kube-control-plane-signer`, `kube-apiserver-external-signer`,
`etcd-signer`, etc.) are removed. The `add-node` command is updated to
consume the new Secret names via the same `CertificateAuthorityResources`
list used by the CA controller.

### API Extensions

N/A — this enhancement does not modify any Kubernetes API resources, CRDs,
webhooks, or aggregated API servers.

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A — MicroShift does not participate in Hypershift topologies.

#### Standalone Clusters

N/A — this enhancement is specific to MicroShift.

#### Single-node Deployments or MicroShift

This enhancement is designed specifically for MicroShift's single-node edge
deployment model. The consolidation is safe precisely because all certificate
consumers run on the same node. The change reduces resource consumption
slightly (fewer files on disk, fewer CA key pairs to generate at startup).

#### OpenShift Kubernetes Engine (OKE)

N/A — MicroShift does not depend on features excluded from OKE.

### Implementation Details/Notes/Constraints

N/A

### PKI Inventory Integration

OCPSTRAT-2899 introduces a PKI inventory that catalogs all managed certificates
by role (CA, serving, client, peer) and tracks parent-child signing relationships.
This enhancement updates that inventory from the current 12-CA layout to the new
6-CA layout:

| Role | Old entries | New entries |
|------|-------------|-------------|
| Rotatable CAs (consolidated) | kube-control-plane-signer, kube-apiserver-to-kubelet-signer, admin-kubeconfig-signer, kubelet-csr-signer-signer, kube-csr-signer (sub-CA), kube-apiserver-external-signer, kube-apiserver-localhost-signer, kube-apiserver-service-network-signer | client-ca, serving-ca |
| Rotatable CAs (renamed) | etcd-signer | peer-ca |
| Rotatable CAs (retained) | aggregator-signer | aggregator-signer (unchanged) |
| Fixed CAs | service-ca, ingress-ca | service-ca, ingress-ca (unchanged) |
| Serving certs | kube-external-serving, kube-apiserver-localhost-serving, kube-apiserver-service-network-serving | kube-apiserver-serving (single SAN-based cert) |

The inventory abstraction ensures that `microshift certs renew --ca` cascades
renewal to all descendant certificates correctly under the new hierarchy without
requiring changes to the CLI code.

**Service account CA bundle:** The `service-account-token-ca.crt` bundle
contains the KAS serving CAs and is injected into pods as a projected volume
so they can verify the KAS TLS identity. After migration this bundle is
updated to contain `serving-ca`. On ostree/bootc deployments MicroShift
restarts all pods on upgrade, so pods receive the updated bundle
automatically. Note: service account tokens themselves are JWTs signed by an
asymmetric key pair (`service-account.key`), not by an X.509 CA; the signing
key is preserved across certificate migration so tokens remain valid.

**Certificate validity:** Two validity constants are used throughout
(`pkg/util/cryptomaterial/certinfo.go`):

| Constant | Value |
|----------|-------|
| `ShortLivedCertificateValidity` | 1 year |
| `LongLivedCertificateValidity` | 10 years |

All expiration dates are aligned to the next calendar midnight so that certs
with the same nominal validity expire on the same day (`alignValidity` in
`pkg/cmd/init.go`).

Current validity assignments (unchanged by this enhancement except where noted):

| Certificate | Type | CA validity | Leaf validity |
|-------------|------|-------------|---------------|
| kube-control-plane-signer | CA | 1 yr | — |
| kube-controller-manager, kube-scheduler, cluster-policy-controller, route-controller-manager | client | — | 1 yr |
| kube-apiserver-to-kubelet-signer | CA | 1 yr | — |
| kube-apiserver-to-kubelet-client | client | — | 1 yr |
| admin-kubeconfig-signer | CA | **10 yr** | — |
| admin-kubeconfig-client | client | — | **10 yr** |
| openshift-observability-client | client | — | 1 yr |
| kubelet-signer (root) → kube-csr-signer (sub-CA) | CA / sub-CA | 1 yr / 1 yr | — |
| kubelet-client | client | — | 1 yr |
| kubelet-server | serving | — | 1 yr |
| aggregator-signer | CA | 1 yr | — |
| aggregator-client | client | — | 1 yr |
| kube-apiserver-external-signer | CA | **10 yr** | — |
| kube-external-serving | serving | — | 1 yr |
| kube-apiserver-localhost-signer | CA | **10 yr** | — |
| kube-apiserver-localhost-serving | serving | — | 1 yr |
| kube-apiserver-service-network-signer | CA | **10 yr** | — |
| kube-apiserver-service-network-serving | serving | — | 1 yr |
| service-ca | CA | **10 yr** | — |
| route-controller-manager-serving | serving | — | 1 yr |
| ingress-ca | CA | **10 yr** | — |
| router-default-serving | serving | — | 1 yr |
| etcd-signer | CA | **10 yr** | — |
| apiserver-etcd-client | client | — | **10 yr** |
| etcd-peer, etcd-serving | peer | — | **10 yr** |

After consolidation, the 3 KAS serving CAs and 5 client CAs are replaced by
`serving-ca` and `client-ca` respectively, both using `LongLivedCertificateValidity`
(10 years). Leaf certificate validity is unchanged. The CA validity change is
an explicit scope expansion — see "CA validity change" below.

**CA validity change (scope expansion):**

Three of the old client CAs used 1-year validity (`kube-control-plane-signer`,
`kube-apiserver-to-kubelet-signer`, `kubelet-csr-signer-signer`); the fourth
(`admin-kubeconfig-signer`) was already 10 years. The consolidated `client-ca`
uses 10-year validity for all of them.

Security trade-off of the longer CA lifetime:

* **Increased blast radius:** A compromised `client-ca` private key can be
  used to sign client certificates trusted by all Kubernetes components for
  up to 10 years. Under the old model, three of those CAs were rotated
  annually, bounding key exposure to 1 year.

* **Mitigations specific to MicroShift's single-node edge model:**
  * The CA private key never leaves the device. There is no network-accessible
    path to exfiltrate it; an attacker needs physical or root access to the
    node — the same access required to steal any other credential on the device.
  * On-demand forced rotation is available via `microshift certs renew --ca`
    (OCPSTRAT-2899). This regenerates `client-ca` and all leaf certificates
    it signed immediately, without waiting for expiry.
  * Short-lived separate CAs provide no additional containment on a single
    node: all consumers of those CAs run on the same host, so an attacker
    with node-level access can steal any of them regardless of key lifetime.
  * ProdSec reviewed and approved this single-node CA hierarchy in March 2026

Rotation thresholds (from `certsToRegenerate`, `pkg/cmd/init.go`):

| Cert lifetime class | Threshold to trigger renewal |
|---------------------|------------------------------|
| Short-lived (< 5 yr total) | < 7 months remaining |
| Long-lived (≥ 5 yr total) | < 18 months remaining |

The validity assignments above reflect the current implementation and are
not changed by this enhancement. Configurable certificate validity periods
are tracked separately in OCPSTRAT-2899; see
[microshift-certificate-rotation.md](/enhancements/microshift/microshift-certificate-rotation.md#certificate-lifetime-configuration)
for details.

**certchains framework:** No changes to the builder framework itself. The
same `NewCertificateSigner`, `WithClientCertificates`,
`WithServingCertificates`, `WithCABundle`, and `Complete` APIs are used.

### Risks and Mitigations

* **Risk: External systems caching old CA certificates.**
  Mitigation: MicroShift edge devices are typically not integrated with
  external CA trust stores. The admin kubeconfig is regenerated on upgrade.
  If operators have distributed the old CA cert to external systems, they
  must update those systems after upgrade.

* **Risk: Service account CA bundle update during upgrade.**
  Mitigation: The `service-account-token-ca.crt` bundle changes from three
  KAS serving CAs to a single `serving-ca`. On ostree/bootc deployments,
  MicroShift restarts all workloads automatically so pods get the updated
  bundle from the projected volume. Service account tokens themselves are
  JWTs signed by a key pair and are unaffected by CA consolidation.

* **Risk: Loss of per-endpoint CA discrimination in KAS serving certificate selection.**
  The current design uses three separate serving CAs, one per endpoint
  (external, localhost, service-network). Consolidating to a single `serving-ca`
  means the KAS no longer validates incoming connections against an
  endpoint-specific CA. On a single-node device this is not a meaningful
  security boundary — all consumers run on the same host — but it is a
  departure from the multi-node model. ProdSec confirmed that per-endpoint
  CA isolation provides no meaningful security benefit in a single-node
  deployment. The `dynamiccertificates` SNI selection logic continues to
  function unchanged; it simply always matches the same SAN-based certificate.

* **Risk: Migration failure leaves a partial certs directory.**
  Mitigation: The `os.Rename` operation is atomic on the same filesystem.
  If it fails, the old `certs/` directory remains intact and MicroShift
  continues with the old layout. If it succeeds but `certSetup()` fails
  mid-way, the partial `certs/` directory is left in place. `certchains`
  writes each signer atomically and idempotently; greenboot restarts the
  service and the retry fills in the missing signers without clearing the
  partial state. If repeated restarts fail, greenboot triggers a rollback.

### Drawbacks

* **Divergence from OpenShift:** The consolidated CA layout differs from
  OCP's multi-CA structure. This is intentional — the ProdSec review
  concluded that the multi-CA structure serves no purpose on a single node.
  Future multi-node MicroShift deployments may need to re-introduce
  separate trust domains, but that would be a separate enhancement.

* **One-time service restart on upgrade:** All certificates are regenerated
  during the 5.0 → 5.1 migration. MicroShift components restart with the
  new cert material. This is an intentional interruption, not a zero-downtime
  migration. On a single-node device with MicroShift managing all components,
  the disruption is equivalent to a service restart.

## Design Details

### Open Questions [optional]

None.

## Test Plan

**Unit tests:**
* Verify the new chain builder produces the correct 6-CA hierarchy.
* Verify the consolidated KAS serving cert contains all expected SANs.
* Verify leaf cert CN/O fields match their original values.
* Verify migration detection correctly identifies old vs. new layouts.
* Verify migration creates a backup directory and removes the old certs dir.

**Integration tests (Robot Framework):**
* **Fresh install test:** Install new MicroShift version, verify cert
  directory structure matches the new layout, verify services are healthy.
* **Migration test:** Deploy old version, upgrade to new version, verify
  backup directory created, new CAs exist, old CAs absent, services healthy,
  `oc login` works.
* **Rollback test:** Upgrade, trigger greenboot rollback, verify old version
  regenerates certs and runs normally.
* **Cert renewal test:** Advance system clock past expiry, restart, verify
  certs renewed under new CAs.

**Manual verification checklist:**
* `oc get csr` — no pending CSRs stuck.
* `openssl verify` — leaf certs chain to correct CA.
* `oc login` — admin kubeconfig works.
* Workloads (pods, routes) functional after upgrade.
* Service account tokens valid (API calls from pods succeed).

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A — this is an internal infrastructure change, not a user-facing feature.
It ships as GA in 5.1.

### Tech Preview -> GA

* All unit and integration tests passing.
* Upgrade testing from 5.0 to 5.1 validated.
* Rollback testing validated with greenboot.
* ProdSec sign-off on the new layout, including the CA validity change from
  1-year to 10-year for `client-ca`.

### Removing a deprecated feature

The old CA layout is removed in the same release. No deprecation period is
needed because:
1. The CA directories are internal implementation details, not a public API.
2. The backup-and-regenerate migration handles the transition automatically.
3. The old per-signer CA Secrets are internal implementation details consumed
   only by MicroShift itself and the `add-node` command, which is updated
   in the same release.

## Upgrade / Downgrade Strategy

**Upgrade (5.0 → 5.1):**
1. New binary detects old cert layout on first startup.
2. Old `certs/` directory is atomically renamed to `certs.backup.<version>.<timestamp>/`.
3. `certs/` is recreated; `service-ca` is restored from the backup.
   `ingress-ca` is not restored — it is regenerated fresh.
4. `certSetup()` generates all remaining CAs and certs under the new hierarchy.
5. All MicroShift components restart with the new certificates.

**Downgrade (5.1 → 5.0 via greenboot rollback):**
1. Old binary starts and finds the new-layout `certs/` directory.
2. It does not recognize the new signer directories and falls back to its
   normal cert-regeneration path, creating the old layout from scratch.
3. Services start normally.

No manual intervention is required in either direction. Certificate-state
rollback is recovery through regeneration, not an atomic restore.

## Version Skew Strategy

N/A — MicroShift runs all components at the same version on a single node.
There is no version skew between control plane and kubelet.

## Operational Aspects of API Extensions

N/A — no API extensions are introduced.

#### Failure Modes

* **Migration rename fails (permissions, disk full):** `initCerts()` returns
  an error, MicroShift does not start. The old `certs/` directory is
  unchanged. Operator fixes the disk issue and restarts.

* **Fresh cert generation fails after migration:** `certSetup()` fails
  mid-way, leaving a partial `certs/` directory. `certchains` writes each
  signer atomically and idempotently — on the next startup it skips
  already-written signers and generates only the missing ones. MicroShift
  exits and greenboot restarts the service; the retry completes the
  remaining signers without clearing the partial directory. If repeated
  restarts fail, greenboot triggers a rollback and the old binary
  regenerates the old-layout certs from scratch.

## Support Procedures

* **Detecting migration occurred:** Check for `certs.backup.*` directory
  under the data directory.
* **Verifying new layout:** `ls /var/lib/microshift/certs/` should show
  `client-ca/`, `aggregator-signer/`, `serving-ca/`, `peer-ca/`,
  `service-ca/`, `ingress-ca/`, and `ca-bundle/`.
* **Reverting manually:** Stop MicroShift, remove `certs/`, rename
  `certs.backup.<version>.<timestamp>/` back to `certs/`, restart. Old certs will be
  used until the next upgrade triggers migration again.

## Implementation History

* 2026-07-29: Initial enhancement proposal.

## Alternatives (Not Implemented)

* **Bridge trust period (keep old CAs in bundles for one release cycle).**
  This would allow old and new CAs to be trusted simultaneously during a
  transition period, with old CAs pruned in 5.2. Rejected because on a
  single-node device there are no external peers that need to gradually
  transition trust. The backup-and-regenerate approach is simpler and
  achieves the same result in one step.

* **In-place re-signing (keep old CA directories, just change which CA
  signs each leaf cert).** This would preserve the directory structure
  while changing the signing relationships. Rejected because it does not
  achieve the goal of simplifying the on-disk layout and leaves dead CA
  key material on disk.

* **Consolidate service-ca and/or ingress-ca into serving-ca.**
  Rejected based on ProdSec review. service-ca runs as a separate pod in the
  openshift-service-ca namespace; it is not a static certificate but a CA
  that dynamically generates TLS certificates for workload Services annotated
  with `service.beta.openshift.io/serving-cert-secret-name`. It has its own
  lifecycle and renewal mechanism managed by the pod itself — merging it into
  serving-ca would confuse two distinct responsibilities. ingress-ca is
  regenerated during the 5.1 migration (the backup-and-regenerate approach
  does not restore it). Customers who have replaced the default ingress CA
  with a custom certificate must re-provision it after upgrading. Preserving
  ingress-ca across migration was evaluated but deferred: the migration
  restores service-ca because in-cluster service-serving Secrets would
  otherwise be immediately invalidated; ingress-ca does not have the same
  in-cluster dependency and its loss is recoverable by the operator.

* **Merge aggregator-signer into client-ca.**
  Rejected for security reasons. The kube-apiserver uses the aggregator-client
  certificate when proxying requests to aggregated APIs. Kubernetes validates
  these via `requestheader-client-ca-file`, which is configured separately
  from `client-ca-file`. Merging aggregator-signer into client-ca would
  cause any client certificate signed by client-ca to satisfy requestheader
  validation, enabling impersonation of the apiserver in aggregated API calls.
  aggregator-signer is also used by metrics packages today. The risk is
  non-zero even on a single node because the trust boundary is enforced by
  Kubernetes API machinery, not by network topology.

* **Keep etcd-signer name unchanged.**
  etcd-signer is structurally identical to the ProdSec-recommended peer-ca.
  Renaming aligns the layout with the ProdSec conceptual model and makes
  the CA's purpose self-documenting (`peer-ca` clearly signals peer
  authentication rather than an OpenShift-specific signer name). The rename
  requires updating etcd's startup configuration to point at the new
  directory path, but the certificate content and signing relationships are
  unchanged. The migration backup-and-regenerate path handles this
  transparently.

* **Merge peer-ca (etcd-signer) into client-ca or serving-ca.**
  The consolidation's core argument — that CA trust domain separation is
  meaningless on a single node because all consumers are on the same host —
  does not apply to peer-ca for three reasons:

  1. **Genuinely isolated trust domain.** etcd manages its own TLS certificate
     verification for both peer and client connections independently of
     Kubernetes. peer-ca is referenced in etcd's own startup configuration,
     not in Kubernetes component configs. Merging it into `client-ca` or
     `serving-ca` would conflate Kubernetes RBAC trust with etcd's internal
     trust domain, which etcd's own TLS stack enforces separately regardless
     of node topology.

  2. **ProdSec recommendation matches the current shape.** ProdSec explicitly
     recommended a peer-ca for etcd peer certificates. Renaming etcd-signer
     to peer-ca satisfies this recommendation without merging it into the
     Kubernetes PKI.

  3. **Unfavorable risk/reward.** Merging peer-ca requires touching etcd's
     bootstrapping and certificate configuration — a separate, higher-risk
     surface — to eliminate exactly one CA, since etcd is its only consumer.
     The security posture improvement is zero; the disruption cost is
     non-trivial.

## Infrastructure Needed [optional]

N/A
