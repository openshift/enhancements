---
title: introducing-hcp-from-hub-cli
authors:
  - "@yiraeChristineKim"
reviewers:
  - "@csrwng"     # HyperShift CLI / product-cli core (OWNERS core-approvers)
  - "@enxebre"    # HyperShift / product-cli (OWNERS core-approvers)
  - "@sjenning"   # HyperShift / product-cli (OWNERS core-approvers)
  - "@muraee"     # product-cli (OWNERS core-reviewers)
  - "@bryan-cox"  # product-cli (OWNERS core-reviewers)
  - "@cblecker"   # product-cli (OWNERS core-reviewers)
  - "@jparrill"   # product-cli (OWNERS core-reviewers)
  - "@devguyio"   # product-cli (OWNERS core-approvers / core-reviewers)
  - "@sdminonne"  # product-cli (OWNERS core-reviewers)
  - "@clebs"      # product-cli (OWNERS core-reviewers)
  - "@Nirshal"    # product-cli (OWNERS core-reviewers)
  - "@ironcladlou" # product-cli (OWNERS core-reviewers)
  - "@nunnatsa"   # kubevirt platform (OWNERS kubevirt-reviewers; Dev/Tech Preview platform)
  - "@orenc1"     # kubevirt platform (OWNERS kubevirt-reviewers; Dev/Tech Preview platform)
  - "@awels"      # kubevirt platform (OWNERS kubevirt-reviewers; Dev/Tech Preview platform)
  - "@akalenyu"   # kubevirt platform (OWNERS kubevirt-reviewers; Dev/Tech Preview platform)
  - "@qinqon"     # kubevirt platform (OWNERS kubevirt-reviewers; Dev/Tech Preview platform)
  # TODO: add an ACM/MCE hypershift-addon-operator / HCP-proxy reviewer
approvers:
  - "@csrwng"
api-approvers:
  - None
creation-date: 2026-07-31
last-updated: 2026-08-27
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/ACM-37265
see-also:
replaces:
superseded-by:
---

# Introducing `hcp from-hub` in the HCP CLI

## Summary

This enhancement defines how to **introduce** `hcp from-hub` in the HCP CLI: create,
edit, and delete HostedClusters on a hosting `ManagedCluster` from the ACM/MCE
**hub** without direct hosting-cluster kubeconfig. All hosting-cluster reads and
writes go through the `hypershift-addon-operator` HCP proxy
(`hcp.ocm.io/v1alpha1`).

The subcommand does **not** exist yet. Implementation will reuse
`product-cli` / `hcp create cluster` rendering code, but that core path assumes
hosting-cluster access via `util.GetClient()` — which resolves to the **hub** in
a from-hub flow unless the client avoids it. The [Implementation
Details](#implementation-detailsnotesconstraints) section lists the requirements
to prevent silent hub targeting (wrong version checks, dropped Roles/ConfigMaps,
broken edit/delete decoding).

**Scope:** a **core, hub-only subset** of the HCP CLI — not full parity with
`hcp create cluster`. Only an explicit **allow-list** of core flags is exposed;
everything else is rejected until a proxy-backed implementation exists. Users who
need capabilities outside that set use `hcp` with a hosting-cluster kubeconfig.

**Dev/Tech Preview:** **`aws` and `kubevirt` only**, plus the hosting-cluster
**version-metadata proxy endpoint** and allow-listed **`--version-check`** /
**`--release-stream`** ([ACM-39227](https://redhat.atlassian.net/browse/ACM-39227),
[ACM-39228](https://redhat.atlassian.net/browse/ACM-39228)). Flags such as
`--render*`, `--wait`, and `--secret-creds` remain excluded until later phases.

## Motivation

Many ACM/MCE hub operators need a way to manage HostedClusters on managed
hosting clusters from the hub alone: they are granted `managedcluster:admin`
via the hub's RBAC/impersonation model, but do not have — and by design should
not need — a hosting-cluster admin kubeconfig. Today they must use the
`hcp` CLI with a hosting-cluster kubeconfig, or other tooling that grants
equivalent access. This enhancement introduces **`hcp from-hub`** in the HCP
CLI to fill that gap through the HCP proxy.

Because the implementation reuses `hcp create cluster` core code, the main risk
is **silent incorrectness** — validation and defaulting against the **hub** while
the user believes they are on the **hosting** cluster (e.g. version checks that
pass on the hub HO but leave a `HostedCluster` the hosting operator cannot
reconcile, or Roles/ConfigMaps never created on the hosting cluster).

This enhancement defines the correct design **before** implementation so the
first ship **fails closed** rather than silently wrong.

### User Stories

* As an ACM/MCE hub operator without hosting-cluster kubeconfig access, I want
  `hcp from-hub create` to validate against the **hosting** cluster's
  HyperShift Operator version and supported OpenShift versions, so that a
  successful create does not silently produce a `HostedCluster` the hosting
  cluster's operator cannot reconcile.
* As a cluster administrator using `hcp from-hub create aws` or
  `hcp from-hub create kubevirt` (Dev/Tech Preview platforms), I want the
  RBAC `Role` and any `ConfigMap`s (e.g. `--additional-trust-bundle`) that my
  `HostedCluster` references to actually exist on the hosting cluster after
  create, so that the HostedCluster does not get stuck reconciling against
  missing objects.
* As a hub operator, I want `from-hub create` to accept only flags on an
  explicit **allow-list** from the core CLI (plus from-hub flags such as
  `--hosting-cluster`), and reject everything else with a clear error, so that
  I am not misled by inherited flags that do not work correctly from the hub.
* As a hub operator running `hcp from-hub edit`, I want the tool to correctly
  fetch the live `HostedCluster` from the proxy and save my changes with a PUT,
  so that I do not lose edits due to response decoding mistakes.
* As an SRE debugging a stuck HostedCluster deletion, I want
  `hcp from-hub delete aws` finalizer cleanup to act on the hosting cluster's
  object (or fail loudly), so that a successful from-hub delete does not leave
  a `HostedCluster` finalizer stuck on the wrong cluster.

### Goals

1. If `from-hub` needs a fact about the **hosting** cluster, get it through the
   HCP proxy or return a clear error — never read it from the hub kubeconfig
   via `util.GetClient()` / `util.GetConfig()`.
2. For Dev/Tech Preview platforms (`aws`, `kubevirt`), every object a
   platform's `GenerateResources()` renders (not just
   `HostedCluster`/`NodePool`/`Secret`) is created on the hosting cluster
   through the proxy.
3. **`from-hub` exposes an allow-list of core CLI arguments only.** It reuses
   `hcp create cluster` rendering/validation code, but only flags on the
   from-hub allow-list are shown in `--help` and accepted; any other inherited
   core CLI flag is rejected with a clear error.
4. `hcp from-hub edit` decodes the proxy GET response and sends a **PUT** with
   the full updated `HostedCluster`, including the live
   `metadata.resourceVersion` from the GET. The proxy applies the update with
   optimistic concurrency on the hosting cluster; HTTP **409 Conflict** is
   returned to the CLI and surfaced to the user so they re-fetch and retry
   instead of overwriting newer controller or operator changes.
5. The from-hub trust model (hub identity + `managedcluster:admin` RBAC +
   impersonation through the HCP proxy) remains the default and only
   supported path for the from-hub subcommand; no code change introduces an
   implicit dependency on a hosting-cluster/`ManagedCluster` admin kubeconfig
   as the from-hub default.
6. Document and enforce a clear **core vs. additional** feature boundary:
   from-hub ships a gradually expanding **core** feature set for hub-only
   operators; capabilities outside that set are explicitly documented as
   requiring the standard `hcp` CLI with hosting-cluster kubeconfig access.

### Non-Goals

1. Full parity with `hcp create cluster` over the hub/proxy path (see Goals 6).
2. Redesigning the HCP proxy auth model (`managedcluster:admin`, impersonation).
3. Dev/Tech Preview platforms beyond **`aws` and `kubevirt`**, or GitOps / declarative
   reconciliation as a first-class workflow (imperative CLI only for initial ship).
4. Changing the direct `hcp create cluster` flow (`util.GetClient()` remains
   correct there).
5. A general management-cluster abstraction outside these from-hub facts/endpoints.

## Proposal

Introduce `hcp from-hub` on top of the existing HCP proxy architecture:

```text
hcp CLI → hub kube-apiserver → HCP proxy (hcp.ocm.io/v1alpha1) → cluster-proxy → hosting cluster
```

Implementation spans `product-cli/cmd/fromhub/` (client) and
`hypershift-addon-operator` / `hcp_proxy.go` (server). Three principles:

1. **Fail closed** — explicit core-flag **allow-list**; reject unknown inherited
   flags (see Requirement 6). Before `core.CreateCluster` render, always force
   `VersionCheck=false` and clear `ReleaseStream` so core never reads the hub;
   when `--version-check` / `--release-stream` are allow-listed (Dev/Tech
   Preview), the CLI performs hosting-cluster checks via the proxy **before**
   render instead.
2. **Proxy for hosting facts** — new or extended HCP proxy routes for metadata,
   create-time hosting validations, `ExtraObjects` create, and delete finalizer
   workflow (see [API Extensions](#api-extensions)).
3. **Correct from-hub client behavior** — shared GET decoder, PUT edit with
   `resourceVersion`, one shared `--namespace` (default `clusters`, matching
   `hcp create cluster`), no `util.GetClient()` for hosting mutations
   (Requirements A–D).

### Feature scope

| | From-hub (`hcp from-hub …`) | Direct (`hcp …` + hosting kubeconfig) |
|---|-----------------------------|----------------------------------------|
| **Operations** | Create, edit, delete via proxy. **Dev/Tech Preview:** `aws`, `kubevirt` | Full platform and flag support |
| **Flags** | Explicit **allow-list** + `--hosting-cluster` | Full `hcp create cluster` set |
| **Hosting cluster** | All reads/writes through **proxy** — never silently the hub | Direct apiserver access |

**Dev/Tech Preview allow-list — version metadata:** `--version-check` and
`--release-stream` are **in scope for Dev/Tech Preview**. They route through
the [version-metadata proxy endpoint](#api-extensions) (shipped with Dev/Tech
Preview in `hypershift-addon-operator`), not through core's hub
`util.GetClient()` path:

| Flag | Hosting-cluster fact (via proxy) | Dev/Tech Preview work |
|------|----------------------------------|------------------|
| `--version-check` | `serverVersion` — CLI build vs hosting HO | [ACM-39227](https://redhat.atlassian.net/browse/ACM-39227) |
| `--release-stream` | `supportedVersions` — OCP release default/validation | [ACM-39228](https://redhat.atlassian.net/browse/ACM-39228) |

**Post–Dev/Tech Preview exclusions:** `--wait`, `--render*`, and AWS
`--secret-creds` stay off the allow-list until a hosting-cluster-correct
implementation exists (hidden from `--help`, clear error if set). The
allow-list grows gradually; from-hub is not expected to expose the full
direct `hcp create cluster` surface.

### Workflow Description

**hub operator** is a human or automated system with `managedcluster:admin`
RBAC on a hosting `ManagedCluster` via the ACM/MCE hub, but no direct
kubeconfig access to that hosting cluster.

**hypershift-addon-operator** is the HCP proxy component running on (or
addressable from) the hub, which enforces `managedcluster:admin` and
impersonates the caller toward the hosting cluster via cluster-proxy.

#### Create (target behavior)

1. The hub operator runs `hcp from-hub create aws --hosting-cluster
   <name> ...` (Dev/Tech Preview also supports `kubevirt`; other platforms are
   rejected until a later phase).
2. When `--version-check` or `--release-stream` is set, the CLI calls the
   Dev/Tech Preview [version-metadata proxy endpoint](#api-extensions) for
   `--hosting-cluster`. The CLI never runs core's hub `validateVersion()` or
   hub `supported-versions` defaulting.
3. The proxy enforces `managedcluster:admin` and impersonates toward the
   hosting cluster before reading or writing hosting-cluster state.
4. `--version-check` compares the CLI build identity to hosting `serverVersion`
   for **this cluster only**; `--release-stream` uses hosting
   `supportedVersions`. Mismatch or invalid stream fails before apply.
5. `from-hub` always reuses `hcp create cluster` with `--render`
   (`core.CreateCluster` in render mode, `VersionCheck` forced `false`).
   Render skips core's hosting-cluster `Validate()` path
   (`validateClusterExistence`: duplicate HostedCluster name and node
   architectures), so those checks never run against the hub via
   `util.GetClient()`.
6. `from-hub` classifies rendered objects into `HostedCluster`, `NodePools`,
   `Secrets`, or `ExtraObjects`, and POSTs a `CreateRequest` to the proxy.
   The HCP proxy validates the hosting cluster (duplicate HostedCluster name,
   node architectures) via its validation endpoint before apply.
7. The proxy creates `Secrets` → `ExtraObjects` → `HostedCluster` → `NodePool`s
   on the hosting cluster, stamping `hcp.ocm.io/*` labels.

#### Edit (target behavior)

1. The hub operator runs `hcp from-hub edit my-cluster --hosting-cluster
   <name>`.
2. `from-hub edit` uses the **same shared GET decoder** as
   `from-hub delete`, correctly unwrapping the proxy's
   `{ "hostedCluster": ... }` response so edits never run against an empty
   object when the proxy wraps the body.
3. The operator edits the opened YAML and saves.
4. `from-hub edit` sends a **PUT** with the full updated `HostedCluster` to
   the proxy (`Content-Type: application/json`), including the
   `metadata.resourceVersion` returned by the GET. The proxy updates the
   hosting cluster with optimistic concurrency. If the hosting apiserver
   returns **409 Conflict** (the object changed since the GET), the CLI exits
   with an error telling the operator to re-run `from-hub edit` and merge
   their changes against the latest object.

#### Delete (target behavior)

Matches the existing `hcp delete aws` sequencing, with all hosting-cluster
mutations through the proxy (not `util.GetClient()` on the hub):

1. The hub operator runs `hcp from-hub delete my-cluster --hosting-cluster
   <name>` (Dev/Tech Preview: `aws` or `kubevirt`; AWS steps below describe the
   cloud-resource cleanup path).
2. `from-hub delete` GETs the `HostedCluster` from the proxy (same shared GET
   decoder as edit).
3. **Add the CLI finalizer first** on the hosting cluster's `HostedCluster`
   (via the proxy), matching direct `hcp delete` behavior.
4. **Start deletion** of the `HostedCluster` on the hosting cluster (via the
   proxy).
5. **Wait** until all finalizers are gone **except** the CLI finalizer the
   command added in step 3.
6. **AWS only:** destroy remaining AWS cloud resources directly (same cleanup
   `hcp delete aws` performs today) **before** removing the CLI finalizer.
7. **Remove the CLI finalizer** on the hosting cluster via the proxy
   finalizer-removal endpoint ([ACM-39226](https://redhat.atlassian.net/browse/ACM-39226))
   — not via `util.GetClient()` against the hub. Failures surface as real CLI
   errors so a stuck finalizer is visible to the operator.

### API Extensions

This enhancement does not add or modify any CRD, admission/conversion
webhook, aggregated API server, or finalizer in the core OpenShift or
HyperShift APIs (`hypershift.openshift.io`). `api-approvers` is `None` for
that reason.

It does extend the **ACM `hypershift-addon-operator` HCP proxy** (API group
`hcp.ocm.io/v1alpha1`, implemented in `pkg/manager/hcp_proxy.go`) with:

1. A metadata GET for hosting-cluster facts:

   ```text
   GET /apis/hcp.ocm.io/v1alpha1/namespaces/{ns}/version?hostingCluster={name}
   ```

   ```json
   {
     "serverVersion": "<hosting-ho-git-sha>",
     "supportedVersions": ["4.19", "4.18", "..."]
   }
   ```

2. A new field on the create/POST payload, `ExtraObjects`, carrying any
   rendered object that is not a `HostedCluster`, `NodePool`, or `Secret`:

   ```go
   type CreateRequest struct {
       HostedCluster *hypershiftv1beta1.HostedCluster `json:"hostedCluster"`
       NodePools     []*hypershiftv1beta1.NodePool     `json:"nodePools,omitempty"`
       Secrets       []corev1.Secret                   `json:"secrets,omitempty"`
       ExtraObjects  []runtime.RawExtension             `json:"extraObjects,omitempty"`
   }
   ```

   The proxy **must** validate every `ExtraObjects` entry before apply:

   - **Allow-list only:** decode only explicit supported
     `GroupVersionKind`s (initial allow-list: `Role`, `RoleBinding`,
     `ConfigMap`; extend deliberately when a platform needs a new kind).
   - **Namespace match:** every namespaced object must use the same namespace
     as the create request (`HostedCluster` namespace).
   - **Reject out of scope:** reject cluster-scoped objects and any object in
     a different namespace before processing. Do not accept arbitrary
     `RawExtension` payloads based on impersonated RBAC alone.

3. A hosting-cluster **create validation** endpoint (exact HTTP route TBD with
   `hypershift-addon-operator`, tracked in
   [ACM-42875](https://redhat.atlassian.net/browse/ACM-42875)). `from-hub`
   always renders with `core.CreateCluster` `--render`, which skips core's
   `validateClusterExistence` (duplicate HostedCluster name and node
   architectures). The proxy runs those equivalent checks against the hosting
   cluster before apply and returns a clear error on conflict or architecture
   mismatch.
4. **Proxy-backed `from-hub delete` operations** (same sequence as [Delete (target
   behavior)](#delete-target-behavior); exact HTTP routes TBD with
   `hypershift-addon-operator`, tracked in
   [ACM-39226](https://redhat.atlassian.net/browse/ACM-39226)):

   | Delete step | Proxy role |
   |-------------|------------|
   | GET `HostedCluster` | Read live object on hosting cluster (shared with edit) |
   | Add CLI finalizer | Patch hosting `HostedCluster` to add CLI finalizer **first** |
   | Start deletion | Delete (or equivalent) hosting `HostedCluster` |
   | Wait for other finalizers | Poll GET until only the CLI finalizer remains |
   | AWS resource cleanup | **No proxy route** — CLI destroys AWS resources directly (same as `hcp delete aws`) |
   | Remove CLI finalizer | Finalizer-removal call on hosting `HostedCluster` — must not use hub `util.GetClient()` |

   Steps that mutate the hosting cluster must go through the proxy with
   `managedcluster:admin` + impersonation, matching existing proxy auth.

These extensions do not change `hypershift.openshift.io` APIs. They add routes
to the existing ACM/MCE **`hypershift-addon-operator`** HCP proxy (`hcp.ocm.io/v1alpha1`)
used by `from-hub`, with the same auth as today: `managedcluster:admin` on the
hub, then impersonation to the hosting cluster via cluster-proxy.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Applies when managing HostedClusters on any hub-reachable `ManagedCluster`
(including **`local-cluster`**). Client code: `product-cli/cmd/fromhub/`.
Server code: `hypershift-addon-operator` (ACM/MCE). No effect on guest cluster
workloads — the hosting HyperShift Operator reconciles objects as if applied
directly on the hosting cluster.

#### Standalone Clusters

Not applicable. `hcp from-hub` manages HostedClusters through the ACM/MCE HCP
proxy; it does not apply to standalone OpenShift clusters.

#### Single-node Deployments or MicroShift

Not applicable. HyperShift does not support single-node or MicroShift
deployments, and this enhancement targets hub-mediated HostedCluster lifecycle
only.

#### OpenShift Kubernetes Engine

Not applicable. This enhancement is scoped to ACM/MCE hub operators managing
HyperShift HostedClusters via the HCP proxy, not to OKE standalone deployments.

### Implementation Details/Notes/Constraints

The table below is the authoritative checklist. Requirements **1–7** guard
against reusing `hcp create cluster` core code that assumes hosting-cluster
`util.GetClient()` access. Requirements **A–D** are from-hub-specific proxy
client behaviors. Wire contracts live in [API Extensions](#api-extensions).

**Cross-cutting rule:** if a step needs a hosting-cluster fact or mutation, use
the ACM `hypershift-addon-operator` proxy — never the hub kubeconfig. Before
`core.CreateCluster` render, always force `VersionCheck=false` and clear
`ReleaseStream`; hosting version/release checks run via the proxy when those
flags are allow-listed.

| # | Requirement | Side | Status |
|---|-------------|------|--------|
| 1 | `--version-check` uses hosting `serverVersion` via proxy, not hub HO | Client + Proxy | Dev/Tech Preview — [ACM-39227](https://redhat.atlassian.net/browse/ACM-39227) |
| 2 | `--release-stream` uses hosting `supportedVersions` via same endpoint, not hub ConfigMap | Client + Proxy | Dev/Tech Preview — [ACM-39228](https://redhat.atlassian.net/browse/ACM-39228) |
| 3 | Platform `Role` objects sent as `ExtraObjects`, not dropped | Client + Proxy | Dev/Tech Preview — [ACM-39216](https://redhat.atlassian.net/browse/ACM-39216) |
| 4 | Platform `ConfigMap`s (e.g. trust bundle) sent as `ExtraObjects` | Client + Proxy | Dev/Tech Preview — same as #3 |
| 5 | Reject AWS `--secret-creds` (must not read hub secrets via `util.GetClient()`) | Client | Dev/Tech Preview — excluded (no proxy alternative yet) |
| 6 | Explicit core-flag allow-list; hide/reject unknown inherited flags | Client | Dev/Tech Preview — `supportedFlags` / `unsupportedFlags` |
| 7 | `from-hub` always reuses `hcp create cluster` with `--render`, so core skips hosting-state validations (duplicate HC name, node architectures) and never calls hub `util.GetClient()`. The HCP proxy runs those validations against the hosting cluster via a dedicated endpoint before apply | Client + Proxy | Planned — [ACM-42875](https://redhat.atlassian.net/browse/ACM-42875) |
| A | `edit`/`delete` share one GET decoder (unwrap `{ "hostedCluster": ... }`) | Client | Planned |
| B | `edit` sends PUT with full `HostedCluster`, `resourceVersion`, and 409 handling | Client + Proxy | Planned |
| C | One shared `--namespace` for create/edit/delete (proxy URL + rendered manifests); default `clusters`, same as `hcp create cluster` | Client | Planned |
| D | `delete aws`: add CLI finalizer → delete → wait → AWS cleanup → remove finalizer, all via proxy | Client + Proxy | Dev/Tech Preview — [ACM-39226](https://redhat.atlassian.net/browse/ACM-39226) |

**Future platform note:** Agent-platform API-server-address resolution
(default to `api.<name>.<base-domain>`, require `--base-domain`, never resolve
from hub nodes) is not a numbered requirement here because `agent` is out of
scope for this enhancement's Dev/Tech Preview (see Non-Goal 3). Track it as a
new numbered requirement when `agent` is added to `from-hub`.

All proxy-side (`hypershift-addon-operator`) work above is tracked under the
HCP Proxy epic [ACM-37265](https://redhat.atlassian.net/browse/ACM-37265) and
lands in the ACM/MCE repository, not `openshift/hypershift`. This enhancement
does not block on that repository accepting any particular implementation —
only on the wire contract (request/response shape, auth model) described in
[API Extensions](#api-extensions), which is designed to be a strict, additive
extension of the existing proxy contract.

### Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| A hosting-cluster metadata endpoint is unavailable at runtime | `--version-check` / `--release-stream` must fail the command with a clear proxy error; must not fall back to the hub | Client always forces `VersionCheck=false`/clears `ReleaseStream` before `core.CreateCluster`; hosting checks run only via the Dev/Tech Preview proxy endpoint |
| New `ExtraObjects` passthrough lets the proxy create arbitrary object kinds on the hosting cluster | Expands the proxy's effective write surface beyond `HostedCluster`/`NodePool`/`Secret` | Proxy validates `ExtraObjects` with a strict GVK allow-list, namespace equality with the request, and rejection of cluster-scoped or cross-namespace objects before any apply |

### Drawbacks

Adding hosting-cluster metadata/passthrough endpoints to the HCP proxy grows
its API surface and puts additional implementation and maintenance burden on
`hypershift-addon-operator` (a different repository/team than
`openshift/hypershift`), rather than being something `openshift/hypershift`
can deliver unilaterally. Coordinating and landing the HCP CLI client and
proxy server changes across both repositories, and keeping this enhancement's
requirement table and Jira tickets in sync adds process overhead. The proxy
must maintain an `ExtraObjects` GVK allow-list, updated when platforms emit
new kinds.

## Alternatives (Not Implemented)

### Require hosting-cluster kubeconfig for all HostedCluster operations

Hub operators could continue to use `hcp create cluster` (and related commands)
with a hosting-cluster admin kubeconfig for every workflow `from-hub` would
cover.

This was not selected because ACM/MCE hub operators often lack direct
hosting-cluster kubeconfig access. The HCP proxy path preserves the existing
hub identity and RBAC model while still targeting the hosting cluster
correctly.

## Open Questions

1. Should the hosting-cluster version-metadata endpoint live at a dedicated
   `.../version` route, or be folded into an existing/broader hosting-cluster
   status endpoint the proxy may already expose? This is an implementation detail for `hypershift-addon-operator` maintainers to
   decide, but affects the exact client call sites in
   `product-cli/cmd/fromhub/client.go`.
2. Which additional `GroupVersionKind`s belong on the initial `ExtraObjects`
   allow-list beyond `Role`, `RoleBinding`, and `ConfigMap` for Dev/Tech Preview
   `aws`/`kubevirt`? New kinds require an explicit proxy allow-list update —
   there is no generic "RBAC-checked apply any object" mode.
3. Does the HCP proxy expose PUT for `HostedCluster` update on the existing
   from-hub edit route, or does it need a new/extended route? Confirm with
   `hypershift-addon-operator` maintainers before implementation.

## Test Plan

### Unit Tests

- `product-cli/cmd/fromhub/create_test.go`: extend `TestAWSUnsupportedFlags`
  (and its `unsupportedFlags` sibling for the core `--wait`/`--timeout`/
  `--render*` set) to verify `--version-check` and `--release-stream` route
  through the version-metadata proxy endpoint and fail clearly when the proxy
  is unavailable; verify `coreOpts.VersionCheck` and `coreOpts.ReleaseStream`
  are always forced to their disabled values before `core.CreateCluster` is
  invoked so core never checks the hub; verify create always sets `--render`
  so core's `validateClusterExistence` (duplicate name, node architectures)
  is skipped on the client (Requirement 7).
- New tests for `buildRequestFromFile`/`stampFromHubLabels` verifying that
  an unrecognized `Kind` (e.g. `Role`, `ConfigMap`, a hypothetical future
  `RoleBinding`) is appended to `CreateRequest.ExtraObjects` with the
  standard `hcp.ocm.io/*` labels applied, instead of being silently
  dropped.
- New tests for a shared GET decoder used by both `from-hub edit` and
  `from-hub delete`, covering both `{ "hostedCluster": ... }` and a raw
  `HostedCluster` body, to lock in the proxy response contract (Requirement
  A).
- New test for `edit.runEdit` verifying the update uses **PUT** with the full
  `HostedCluster` body and preserved `metadata.resourceVersion`, and that
  HTTP 409 responses are surfaced to the user (Requirement B).
- New test verifying `--namespace` has one source of truth across
  create/edit/delete (Requirement C).

### Integration / E2E Tests

- An e2e (or integration test against a local `hypershift-addon-operator` +
  kind hosting cluster, using a `--proxy-url` local-dev flow) verifying that
  `hcp from-hub create aws` and `hcp from-hub create kubevirt` result in
  expected `ExtraObjects` (e.g. Roles/ConfigMaps required by the platform)
  actually existing on the hosting cluster, not just being present in the
  rendered YAML.
- Equivalent e2e coverage for `--additional-trust-bundle`'s `user-ca-bundle`
  ConfigMap landing on the hosting cluster.
- An e2e test for `hcp from-hub create --version-check` against a hosting
  cluster running a HyperShift Operator version deliberately mismatched from
  the CLI, verifying the command fails with the hosting-cluster version in
  the error message (not the hub's).
- An e2e test that creating a HostedCluster whose name already exists on the
  hosting cluster fails via the HCP proxy validation endpoint (Requirement 7),
  not via hub `util.GetClient()`.
- An e2e test for `hcp from-hub edit` changing a field (e.g. adding an entry to
  `spec.services`) end-to-end, verifying the hosting cluster's `HostedCluster`
  matches the YAML the operator saved.
- An e2e test for `hcp from-hub delete aws` covering the full sequence: CLI
  finalizer added on the hosting cluster, other finalizers clear, AWS resource
  cleanup runs, then CLI finalizer removed via the proxy (not the hub); proxy
  failures surface as CLI errors.
- An integration test against `hypershift-addon-operator` that injects a
  mid-create failure after `HostedCluster` creation and attaches a blocking
  finalizer to an object in the compensating-cleanup path, verifying the proxy
  waits for each deletion in reverse dependency order, reports a cleanup
  timeout as a separate failure from the original create error, and does not
  proceed to delete the next dependent object until the prior one is gone.

CI for these tests spans two repositories (`openshift/hypershift` for the CLI
unit tests, and the ACM/MCE `hypershift-addon-operator` repository for the
proxy-side endpoint tests); coordinating cross-repo CI coverage for the new
endpoints is itself part of the implementation work.

## Graduation Criteria

`hcp from-hub` graduation tracks delivery of the [requirements
table](#implementation-detailsnotesconstraints) for **Dev/Tech Preview
platforms
(`aws`, `kubevirt`)**:

- [ ] Subcommand shipped: create, edit, delete for `aws` and `kubevirt` only
- [ ] Hosting-cluster correctness: no `util.GetClient()` for hosting
  validation/defaulting; proxy-backed version metadata ([ACM-39227](https://redhat.atlassian.net/browse/ACM-39227),
  [ACM-39228](https://redhat.atlassian.net/browse/ACM-39228)), `ExtraObjects`
  ([ACM-39216](https://redhat.atlassian.net/browse/ACM-39216)), delete
  finalizer ([ACM-39226](https://redhat.atlassian.net/browse/ACM-39226))
- [ ] Client requirements A–D: shared GET decoder, PUT edit with
  `resourceVersion`/409 handling, single `--namespace`, allow-list enforced
- [ ] `--base-domain` required on from-hub create; unsupported platforms/flags
  rejected cleanly
- [ ] User documentation for core from-hub scope, allow-list, and when to use
  direct `hcp` + hosting kubeconfig
- [ ] Unit and e2e tests per [Test Plan](#test-plan)

Complete when all boxes are checked and no requirements-table rows remain open.

### Dev Preview -> Tech Preview

Ship `hcp from-hub` create/edit/delete for `aws` and `kubevirt` with the
checklist above: hosting-cluster correctness through the proxy (no hub
`util.GetClient()` fallback), ExtraObjects, version-metadata flags, delete
finalizer sequence, client requirements A–D, allow-list enforcement, and
unit/e2e coverage per the [Test Plan](#test-plan). User documentation lists
supported platforms and points unsupported platforms/flags to direct `hcp`
with hosting-cluster kubeconfig.

### Tech Preview -> GA

Promote when `aws` and `kubevirt` create/edit/delete work end-to-end through
the proxy, unsupported platforms fail cleanly, documentation lists supported
platforms and points others to direct `hcp`, and any platforms/flags added
after Tech Preview meet the same hosting-cluster correctness requirements as
`aws`/`kubevirt`. GA also requires soak time with hub-operator feedback and
CLI ↔ proxy skew coverage from the [Test Plan](#test-plan).

### Removing a deprecated feature

Not applicable on initial introduction. Flags excluded from the from-hub
allow-list for Dev/Tech Preview (`--wait`, `--timeout`, `--render*`, AWS
`--secret-creds`) remain part of `hcp create cluster`'s flag set — they are
only hidden and rejected on the `from-hub` subcommand, not deprecated
platform-wide. `--version-check` and `--release-stream` are allow-listed in
Dev/Tech Preview via the version-metadata proxy endpoint.

## Upgrade / Downgrade Strategy

`hcp from-hub` is a new CLI subcommand with no persisted cluster API — the
compatibility surface is **CLI ↔ proxy version skew**:

- **Newer CLI, older proxy:** Calls to unsupported endpoints (version metadata,
  `ExtraObjects`, finalizer removal) must fail closed with "unsupported by
  proxy" — never fall back to hub `util.GetClient()`. Non-empty `ExtraObjects`
  on an unsupported proxy must be **rejected**, not silently ignored.
- **Older CLI, newer proxy:** Older CLIs never call new endpoints; behavior
  matches what that CLI version shipped with.
- **Hosting HO upgrades:** Ordinary HyperShift Operator upgrades on the hosting
  cluster do not require from-hub CLI changes; version facts come from the
  version-metadata endpoint when `--version-check` is used.

Users adopt `hcp from-hub` by upgrading `hcp` and `hypershift-addon-operator`
to versions implementing the contract in [API Extensions](#api-extensions).

## Version Skew Strategy

Not applicable beyond CLI/proxy skew above — no kubelet, CRI, or CNI concerns.
The proxy is the sole source of hosting-cluster facts for from-hub.

## Operational Aspects of API Extensions

The new HCP proxy routes are read-mostly (version/supported-versions
metadata) plus two write paths already bounded by the existing
`managedcluster:admin` + impersonation model (`ExtraObjects` create,
finalizer removal). They do not introduce new SLIs beyond the proxy's
existing ones (HTTP latency/availability of `hcp.ocm.io/v1alpha1` routes).

- **Failure mode — metadata endpoint unavailable or erroring:** `from-hub
  create --version-check`/`--release-stream` fails the command with a clear
  error identifying the proxy call that failed; it does not fall back to
  checking the hub, and it does not silently skip the check.
- **Failure mode — create apply fails partway through:** The proxy fails the
  whole create request rather than leaving a half-created HostedCluster. Apply
  order is `Secrets` → `ExtraObjects` → `HostedCluster` → `NodePool`s; if any
  step fails after earlier objects were created in the same request, the proxy
  performs **compensating cleanup** in reverse dependency order, deleting any
  objects it created for this request (`NodePool`s, then `HostedCluster`, then
  `ExtraObjects`, then `Secrets`) using ownership-aware deletion keyed on the
  standard `hcp.ocm.io/*` request labels so unrelated objects are not touched.
  Each deletion must complete (object removed from the hosting apiserver) before
  the next dependent object is deleted; the proxy polls with a bounded timeout
  and treats a timeout as a cleanup failure. If compensating cleanup itself
  fails, the proxy returns the original creation error **and** a separate
  cleanup failure to the CLI. Retries must be safe:
  the proxy uses idempotent create/resume semantics (accept existing objects
  that match the same request identity, or finish cleanup before re-applying)
  so operators do not get permanent `AlreadyExists` loops or orphaned
  resources.
- **Failure mode — finalizer-removal endpoint fails:** Surfaced to the CLI
  caller as a real, non-zero-exit error, so a stuck-terminating
  `HostedCluster` is visible to the operator running `from-hub delete`
  instead of looking like a clean delete.

Failure of any of these endpoints has no impact on HostedClusters or control
planes created through other paths — it only affects the from-hub CLI
operation in progress.

## Support Procedures

Not applicable until `hcp from-hub` ships. See the [Test Plan](#test-plan) for
validation steps during implementation and e2e testing.
