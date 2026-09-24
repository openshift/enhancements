---
title: microshift-certificate-consolidation
authors:
  - "@eslutsky"
reviewers:
  - "@fzdarsky, MicroShift architect"
  - "@ggiguash, MicroShift contributor"
  - "@stlaz, Security specialist"
  - "@pmtk, MicroShift contributor"
  - "@copejon, MicroShift contributor"
approvers:
  - "@dhellmann"
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

This enhancement consolidates MicroShift's CA hierarchy from 12 CAs to 5
(3 new, 2 unchanged) to conform to ProdSec guidance for single-node
deployments. The change is scoped exclusively to CAs — it only affects which
CA signs each internally-generated certificate, not the certificates
themselves or their content.

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
on-disk layout. This enhancement updates that inventory to reflect the new 5-CA
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

* Reduce the number of managed CAs from 12 to 5.
* Consolidate the 3 KAS serving certificates into 1 SAN-based certificate.
* Preserve all leaf certificate identities (CN/O fields) so that Kubernetes
  RBAC authorization is unaffected.
* Provide a zero-downtime upgrade path from the old to the new layout.
* Support atomic rollback via greenboot/ostree.

### Non-Goals

* Changing the service-ca or ingress-ca in any way.
* Implementing the `microshift certs` CLI subcommands, PKI inventory abstraction,
  or configurable certificate validity — those are covered by OCPSTRAT-2899,
  which co-ships with this enhancement.
* Changing certificate validity periods or renewal thresholds.
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
| aggregator-signer | aggregator-client |

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

`client-ca` replaces 5 CAs (kube-control-plane-signer,
kube-apiserver-to-kubelet-signer, admin-kubeconfig-signer,
kubelet-csr-signer-signer/kube-csr-signer, aggregator-signer) and signs all
client certificates:

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

**Unchanged CAs:** service-ca and ingress-ca remain identical.

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
2. `certSetup()` builds the new 5-CA hierarchy.
3. All leaf certs, kubeconfigs, and trust bundles are generated.
4. MicroShift starts normally.

**Upgrade from pre-consolidation version:**
1. New MicroShift binary starts and calls `initCerts()`.
2. Migration logic detects the old CA directory layout by checking for the
   existence of `<datadir>/certs/kube-control-plane-signer/`.
3. The entire `certs/` directory is renamed to
   `certs.backup.<version>/`.
4. `certSetup()` finds no certs directory, generates everything fresh with
   the new hierarchy.
5. Kubeconfigs are regenerated with the new serving-ca as the trust anchor.
6. All pods restart and receive new service account tokens.

**Rollback (greenboot/ostree):**
1. If greenboot detects an unhealthy system after upgrade, it triggers an
   atomic rollback to the previous OS commit.
2. The old MicroShift binary starts, finds no `certs/` directory (the backup
   has a different name).
3. Existing behavior: MicroShift regenerates all certs from scratch with the
   old hierarchy.
4. Clean rollback with no manual intervention required.

**Backup cleanup:**
The `certs.backup.<version>/` directory is left on disk and not
automatically deleted. Documentation will advise operators to remove it after
confirming the upgrade is stable. Edge devices have sufficient disk capacity
for one backup.

### CA Bundle Changes

| Bundle file | Current contents | New contents |
|-------------|-----------------|--------------|
| `ca-bundle/client-ca.crt` | 5 CA certs concatenated | single client-ca cert |
| `ca-bundle/kubelet-ca.crt` | kubelet-csr-signer CA | client-ca cert |
| `ca-bundle/kubelet-serving-ca.crt` | kubelet-csr-signer CA | serving-ca cert |
| `ca-bundle/service-account-token-ca.crt` | 3 KAS serving CAs | client-ca cert |
| `ca-bundle/ca-bundle.crt` | all serving CAs | serving-ca + peer-ca certs |

ConfigMaps and Secrets exposed to Kubernetes maintain the same names and
namespaces. Consumer-side configuration does not change.

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
5-CA layout:

| Role | Old entries | New entries |
|------|-------------|-------------|
| Rotatable CAs | kube-control-plane-signer, kube-apiserver-to-kubelet-signer, admin-kubeconfig-signer, kubelet-csr-signer-signer, kube-csr-signer (sub-CA), kube-apiserver-external-signer, kube-apiserver-localhost-signer, kube-apiserver-service-network-signer | client-ca, serving-ca |
| Rotatable CAs (renamed) | etcd-signer | peer-ca |
| Fixed CAs | service-ca, ingress-ca | service-ca, ingress-ca (unchanged) |
| Serving certs | kube-external-serving, kube-apiserver-localhost-serving, kube-apiserver-service-network-serving | kube-apiserver-serving (single SAN-based cert) |

The inventory abstraction ensures that `microshift certs renew --ca` cascades
renewal to all descendant certificates correctly under the new hierarchy without
requiring changes to the CLI code.

**Service account tokens:** Tokens signed by the old service-account-token
CA become invalid after migration. MicroShift restarts all pods on upgrade,
so they receive new tokens immediately. No user action required.

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
(10 years). Leaf certificate validity is unchanged.

Rotation thresholds (from `certsToRegenerate`, `pkg/cmd/init.go`):

| Cert lifetime class | Threshold to trigger renewal |
|---------------------|------------------------------|
| Short-lived (< 5 yr total) | < 7 months remaining |
| Long-lived (≥ 5 yr total) | < 18 months remaining |

**certchains framework:** No changes to the builder framework itself. The
same `NewCertificateSigner`, `WithClientCertificates`,
`WithServingCertificates`, `WithCABundle`, and `Complete` APIs are used.

### Risks and Mitigations

* **Risk: External systems caching old CA certificates.**
  Mitigation: MicroShift edge devices are typically not integrated with
  external CA trust stores. The admin kubeconfig is regenerated on upgrade.
  If operators have distributed the old CA cert to external systems, they
  must update those systems after upgrade.

* **Risk: Service account token invalidation during upgrade.**
  Mitigation: MicroShift restarts all workloads on upgrade. Pods receive
  new tokens automatically. Operators using long-lived extracted tokens
  (anti-pattern) must re-extract after upgrade.

* **Risk: Migration failure leaves no certs directory.**
  Mitigation: The `os.Rename` operation is atomic on the same filesystem.
  If it fails, the old `certs/` directory remains intact and MicroShift
  continues with the old layout. If it succeeds but `certSetup()` fails,
  greenboot triggers a rollback.

### Drawbacks

* **Divergence from OpenShift:** The consolidated CA layout differs from
  OCP's multi-CA structure. This is intentional — the ProdSec review
  concluded that the multi-CA structure serves no purpose on a single node.
  Future multi-node MicroShift deployments may need to re-introduce
  separate trust domains, but that would be a separate enhancement.

* **One-time upgrade disruption:** All certificates are regenerated, which
  means a brief period during startup where services are restarting with
  new certs. On a single-node device with MicroShift managing all
  components, this is functionally equivalent to a fresh start.

## Design Details

### Open Questions [optional]

None.

## Test Plan

**Unit tests:**
* Verify the new chain builder produces the correct 5-CA hierarchy.
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
* ProdSec sign-off on the new layout.

### Removing a deprecated feature

The old CA layout is removed in the same release. No deprecation period is
needed because:
1. The CA directories are internal implementation details, not a public API.
2. The backup-and-regenerate migration handles the transition automatically.
3. Kubernetes Secret names and namespaces are preserved.

## Upgrade / Downgrade Strategy

**Upgrade (5.0 → 5.1):**
1. New binary detects old cert layout on first startup.
2. Old `certs/` directory is atomically renamed to `certs.backup.<version>/`.
3. Fresh cert generation creates the new layout.
4. All services restart with new certificates.

**Downgrade (5.1 → 5.0 via greenboot rollback):**
1. Old binary starts, finds no `certs/` directory.
2. Existing cert regeneration logic creates the old layout from scratch.
3. Services start normally.

No manual intervention is required in either direction.

## Version Skew Strategy

N/A — MicroShift runs all components at the same version on a single node.
There is no version skew between control plane and kubelet.

## Operational Aspects of API Extensions

N/A — no API extensions are introduced.

#### Failure Modes

* **Migration rename fails (permissions, disk full):** `initCerts()` returns
  an error, MicroShift does not start. The old `certs/` directory is
  unchanged. Operator fixes the disk issue and restarts.

* **Fresh cert generation fails after migration:** `certs/` directory does
  not exist (was renamed), `certSetup()` fails. Greenboot detects unhealthy
  state and triggers rollback. Old binary regenerates certs from scratch.

## Support Procedures

* **Detecting migration occurred:** Check for `certs.backup.*` directory
  under the data directory.
* **Verifying new layout:** `ls /var/lib/microshift/certs/` should show
  `client-ca/`, `serving-ca/`, `peer-ca/`, `service-ca/`, `ingress-ca/`,
  and `ca-bundle/`.
* **Reverting manually:** Stop MicroShift, remove `certs/`, rename
  `certs.backup.<version>/` back to `certs/`, restart. Old certs will be
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
  serving-ca would confuse two distinct responsibilities. ingress-ca signs the
  default wildcard certificate (`*.apps.<domain>`) used by the router;
  customers can and do replace this with their own custom certificate for
  ingress routes, so it must remain independently configurable with its own
  lifecycle. Folding it into serving-ca would prevent customers from managing
  their own ingress TLS independently.

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
