---
title: introducing-hcp-from-hub-cli
authors:
  - "@yiraeChristineKim"
reviewers:
  - "@csrwng, HyperShift CLI and product-cli core paths (OWNERS core-approvers)"
  - "@enxebre, HyperShift / product-cli (OWNERS core-approvers)"
  - "@sjenning, HyperShift / product-cli (OWNERS core-approvers)"
  - "@muraee, product-cli (OWNERS core-reviewers)"
  - "@bryan-cox, product-cli (OWNERS core-reviewers)"
  - "@cblecker, product-cli (OWNERS core-reviewers)"
  - "@jparrill, product-cli (OWNERS core-reviewers)"
  - "@devguyio, product-cli (OWNERS core-approvers / core-reviewers)"
  - "@sdminonne, product-cli (OWNERS core-reviewers)"
  - "@clebs, product-cli (OWNERS core-reviewers)"
  - "@Nirshal, product-cli (OWNERS core-reviewers)"
  - "@ironcladlou, product-cli (OWNERS core-reviewers)"
  - "@nunnatsa, kubevirt platform (OWNERS kubevirt-reviewers; Dev Preview platform)"
  - "@orenc1, kubevirt platform (OWNERS kubevirt-reviewers; Dev Preview platform)"
  - "@awels, kubevirt platform (OWNERS kubevirt-reviewers; Dev Preview platform)"
  - "@akalenyu, kubevirt platform (OWNERS kubevirt-reviewers; Dev Preview platform)"
  - "@qinqon, kubevirt platform (OWNERS kubevirt-reviewers; Dev Preview platform)"
  - TBD, hypershift-addon-operator / HCP proxy (ACM/MCE) expertise
approvers:
  - "@csrwng"
api-approvers:
  - None
creation-date: 2026-07-31
last-updated: 2026-08-21
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/ACM-37265
see-also:
replaces:
superseded-by:
---

# Introducing `hcp from-hub` in the HCP CLI

## Summary

This enhancement defines how to **introduce** a new `hcp from-hub` subcommand
in the **HCP CLI** (`hcp`), letting a user create, edit, and delete
HostedClusters on a hosting `ManagedCluster` from the ACM/MCE **hub**, without
direct kubeconfig access to the hosting cluster. All requests flow through the
`hypershift-addon-operator` HCP proxy (`hcp.ocm.io/v1alpha1`) to the hosting
cluster.

The `hcp from-hub` subcommand does **not** exist yet. This document is the
design for its initial introduction and for how it should reuse — safely — the
existing `product-cli` / `hcp create cluster` core design for rendering and
validation. That core design assumes local hosting-cluster kubeconfig access via
`util.GetClient()` / `util.GetConfig()`. In a from-hub flow, those helpers
resolve to `$KUBECONFIG` — the **hub**, not the hosting cluster — unless the
from-hub implementation explicitly avoids them. If implemented naively, version
checks, release-image defaulting, existence/architecture validation, and Agent
API-server-address resolution would run against the hub instead of the hosting
cluster, and some rendered resources (Roles, ConfigMaps) would be dropped
before ever reaching the hosting cluster. Similar pitfalls apply to planned
`from-hub edit` and `from-hub delete` flows if they decode or update proxy HTTP
responses incorrectly, or call `util.GetClient()` for hosting-cluster mutations.

This enhancement catalogs those design requirements in the [Implementation
Details](#implementation-detailsnotesconstraints) section below and defines how
to ship from-hub correctly on first introduction — keeping the proxy-based
trust model (never falling back to a hosting-cluster/`ManagedCluster` admin
kubeconfig as the default) by (1) exposing only an **allow-list** of arguments
from the core `hcp create cluster` CLI (all other inherited flags hidden and
rejected), (2) adding a small set of HCP proxy metadata/passthrough endpoints
so hosting-cluster facts (operator version, supported OCP versions,
non-HC/NodePool/Secret objects, finalizer removal) can be read/written through
the proxy instead of a local kubeconfig, and (3) implementing from-hub-specific
create/edit/delete client logic that never silently targets the hub when the
intent is to act on the hosting cluster.

This enhancement does **not** aim to bring `hcp from-hub` to full parity with
`hcp create cluster` (direct hosting-cluster kubeconfig access). The from-hub
CLI is a **core, hub-only subset** of the HCP CLI: we will **gradually**
introduce features there as proxy endpoints and client work are merged and
released, but only via an explicit **allow-list** of core CLI arguments — not
as a wholesale copy of the direct CLI flag set. For **additional or advanced
features** beyond that allow-list, users who have hosting-cluster access should
use the standard `hcp` CLI with a hosting-cluster kubeconfig instead of
expecting full from-hub coverage.

**Dev Preview** ships **`aws` and `kubevirt` only**; other platforms are added
in later phases.

## Motivation

Many ACM/MCE hub operators need a way to manage HostedClusters on managed
hosting clusters from the hub alone: they are granted `managedcluster:admin`
via the hub's RBAC/impersonation model, but do not have — and by design should
not need — a hosting-cluster admin kubeconfig. Today they must use the
`hcp` CLI with a hosting-cluster kubeconfig, or other tooling that grants
equivalent access. This enhancement introduces **`hcp from-hub`** in the HCP
CLI to fill that gap through the HCP proxy.

Because the implementation will reuse much of the existing `hcp create cluster`
core design, the main risk is **silent incorrectness**: if from-hub calls
`util.GetClient()` / `util.GetConfig()` without special handling, validation and
defaulting would run against the **hub** while the user believes they are
acting on the **hosting** cluster. That is worse than not shipping the feature
at all — for example, version checks and release-image defaulting could appear
to succeed against the hub's HyperShift Operator while the hosting cluster's
operator cannot reconcile the resulting `HostedCluster`, and RBAC `Role`s /
trust-bundle `ConfigMap`s referenced by the `HostedCluster` might never be
created on the hosting cluster even though create returned successfully.

This enhancement exists to define the correct from-hub design **before**
implementation, so the first shipped version fails closed rather than silently
wrong.

### User Stories

* As an ACM/MCE hub operator without hosting-cluster kubeconfig access, I want
  `hcp from-hub create` to validate against the **hosting** cluster's
  HyperShift Operator version and supported OpenShift versions, so that a
  successful create does not silently produce a `HostedCluster` the hosting
  cluster's operator cannot reconcile.
* As a cluster administrator using `hcp from-hub create agent`, I want the
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
2. Every object a platform's `GenerateResources()` renders (not just
   `HostedCluster`/`NodePool`/`Secret`) is created on the hosting cluster
   through the proxy.
3. **`from-hub` exposes an allow-list of core CLI arguments only.** It reuses
   `hcp create cluster` rendering/validation code, but only flags on the
   from-hub allow-list are shown in `--help` and accepted; any other inherited
   core CLI flag is rejected with a clear error.
4. `hcp from-hub edit` decodes the proxy GET response and sends a **PUT** with
   the full updated `HostedCluster`. `hcp from-hub delete` decodes the proxy
   GET response correctly.
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

1. **Full parity with `hcp create cluster` via `hcp from-hub`.** The from-hub
   CLI reuses much of the direct `hcp create cluster` core design, but we are
   not going to fully support every flag, platform, and workflow through the
   hub/proxy path. Only a **core** subset is in scope for from-hub; additional
   features are introduced **gradually** as proxy endpoints and client work
   land, not as a wholesale port of the direct CLI surface.
2. Redesigning the HCP proxy's overall authentication/authorization model
   (hub identity, `clusterview.open-cluster-management.io`, impersonation via
   cluster-proxy). That model is out of scope and treated as correct for the
   from-hub trust boundary.
3. Adding broad new user-facing functionality to `hcp from-hub` beyond the
   Dev Preview platform set (`aws`, `kubevirt`). Other platforms (e.g.
   `agent`) and new flags beyond the initial core flows are deferred to later
   phases or to `hcp` with hosting-cluster kubeconfig.
4. Changing `hcp create cluster` (direct hosting-cluster kubeconfig flow).
   `util.GetClient()` / `util.GetConfig()` remain correct for that flow; this
   enhancement only constrains how from-hub may call into shared core code.
5. A general "management cluster view" abstraction usable outside HyperShift
   CLI. The proxy endpoints proposed here are scoped to the specific facts
   `from-hub create`/`edit`/`delete` needs.

## Proposal

Introduce `hcp from-hub` on top of the existing HCP proxy architecture:

```
hcp CLI → hub kube-apiserver → HCP proxy (hcp.ocm.io/v1alpha1) → cluster-proxy → hosting cluster
```

Implementation spans new client code under `product-cli/cmd/fromhub/` and new
or extended server-side routes in `hypershift-addon-operator`'s
`pkg/manager/hcp_proxy.go`. Three design principles apply from the first
commit:

1. **Fail closed instead of silently wrong.** `from-hub create` reuses core
   CLI code but does **not** expose the full `hcp create cluster` flag set.
   Instead it maintains an explicit **allow-list** of supported arguments
   (from-hub-specific flags such as `--hosting-cluster`, plus a small subset
   of core create flags that work correctly through the proxy). Any inherited
   core CLI flag not on the allow-list is hidden from `--help` and rejected
   if set — for example `--wait`, `--timeout`, `--render*`, AWS
   `--secret-creds`, and `--version-check` until a hosting-cluster-correct
   proxy-backed implementation exists (see Requirements 1, 6, and 9 below).
   The allow-list grows **gradually** as proxy support lands; it is not a
   one-time copy of the full core CLI surface.
2. **Add small, purpose-built HCP proxy endpoints** so hosting-cluster facts
   that shared core code would otherwise fetch via `util.GetClient()` can be
   read from, or applied to, the **hosting** cluster through the existing
   proxy trust boundary:
   - a metadata GET returning the hosting cluster's HyperShift Operator
     version and supported-OCP-versions list (Requirements 1 and 2 / W1
     and W2),
   - generic passthrough of non-`HostedCluster`/`NodePool`/`Secret` objects
     (`ExtraObjects`) on create, so Roles/ConfigMaps are created on the
     hosting cluster (Requirements 4 and 5 / W4),
   - a finalizer-removal endpoint for `from-hub delete aws` cleanup
     (Requirement D below).
3. **Implement from-hub client logic correctly from the start**, independent
   of cluster targeting: shared GET-response decoding for `edit`/`delete`,
   `edit` sending a PUT with the full updated `HostedCluster`, a single source
   of truth for `--namespace`, and no use of `util.GetClient()` for
   hosting-cluster mutations (Requirements A–C below).

#### Feature scope: core from-hub vs. additional via hosting kubeconfig

The from-hub CLI is intentionally a **subset** of the HCP CLI, not a
line-for-line reimplementation of every `hcp create cluster` capability over
the hub/proxy path.

| Category | From-hub (`hcp from-hub …`) | Direct hosting access (`hcp …` + hosting kubeconfig) |
|----------|-----------------------------|------------------------------------------------------|
| **What users can do** | Create, edit, and delete HostedClusters from the hub (through the proxy). **Dev Preview:** `aws` and `kubevirt` only | Same, with full platform and flag support |
| **Which flags are allowed** | Small **allow-list** from core CLI + from-hub flags (e.g. `--hosting-cluster`); all others rejected | Full `hcp create cluster` flag set |
| **Must talk to the hosting cluster** | All reads/writes for the hosting cluster go through the **proxy** — never silently use the hub | Already talks to the hosting cluster directly |
| **Not in from-hub (initially)** | Flags/workloads outside the allow-list until proxy support exists | Use this path instead (e.g. `--wait`, `--render*`, `--secret-creds`) |

**Must talk to the hosting cluster** is not a separate command — it is the rule that
makes from-hub trustworthy. Concretely, the first release must:

- Check version and supported OCP versions on the **hosting** cluster (via proxy)
- Create Roles and ConfigMaps on the **hosting** cluster, not drop them
- Edit with GET + PUT through the proxy (parse the proxy JSON response correctly)
- Run delete finalizer cleanup on the **hosting** cluster (via proxy)

If from-hub reused core CLI code without these rules, it would often touch the **hub**
by mistake while the user thinks they are managing the hosting cluster.

**Rollout model:**

1. **Dev Preview (initial ship):** Introduce `hcp from-hub` with create, edit,
   and delete for **`aws` and `kubevirt` only**. Other platforms (e.g. `agent`)
   are rejected with a clear error until added in a later phase. Ship with a
   minimal core CLI **allow-list**; reject any inherited flag not on that list.
2. **Gradually:** Add platforms and flags to the allow-list only when there is
   a hosting-cluster-correct proxy implementation — not by silently reusing
   direct-CLI code paths against the hub.
3. **Documentation:** New from-hub user documentation must state which
   operations and flags are supported from-hub vs. which require `hcp` with a
   hosting-cluster kubeconfig, so hub-only operators know when they need
   elevated access or a different workflow.

This is a product boundary, not a temporary workaround: we do not expect
`hcp from-hub` to ever expose the full `hcp create cluster` surface. Users
who need capabilities outside the documented from-hub core set should use the
standard `hcp` CLI against the hosting cluster.

### Workflow Description

**hub operator** is a human or automated system with `managedcluster:admin`
RBAC on a hosting `ManagedCluster` via the ACM/MCE hub, but no direct
kubeconfig access to that hosting cluster.

**hypershift-addon-operator** is the HCP proxy component running on (or
addressable from) the hub, which enforces `managedcluster:admin` and
impersonates the caller toward the hosting cluster via cluster-proxy.

#### Create (target behavior)

1. The hub operator runs `hcp from-hub create aws --hosting-cluster
   <name> ...` (Dev Preview also supports `kubevirt`; other platforms are
   rejected until a later phase).
2. Once the version-metadata proxy endpoint exists (W1), `from-hub` routes
   `--version-check` through it: it does **not** let `core.CreateCluster` run
   `validateVersion()` against `util.GetClient()` (the hub). Instead it
   issues a GET to the HCP proxy's version-metadata endpoint for
   `--hosting-cluster <name>`. Until that endpoint exists, `--version-check`
   must be rejected with a clear error.
3. The proxy checks `managedcluster:admin`, impersonates toward the hosting
   cluster, reads the hosting cluster's `supported-versions` ConfigMap, and
   returns `serverVersion` (and `supportedVersions`, reused by W2) to the CLI.
4. `from-hub` compares the CLI's own commit SHA against the returned hosting
   `serverVersion` and fails fast on mismatch — checking the cluster the user
   actually intends to create the HostedCluster on.
5. `from-hub` renders the HostedCluster/NodePool/Secret/Role/ConfigMap
   manifests locally (`core.CreateCluster` in render mode, `VersionCheck`
   forced `false` so core never touches the hub), including any object a
   platform's `GenerateResources()` emits, and classifies each rendered
   object into `HostedCluster`, `NodePools`, `Secrets`, or `ExtraObjects`.
6. `from-hub` POSTs a `CreateRequest` (including `ExtraObjects`) to the
   proxy.
7. The proxy creates `Secrets` → `ExtraObjects` (Roles/ConfigMaps, so
   dependent objects exist first) → `HostedCluster` → `NodePool`s on the
   hosting cluster, in that order, stamping the standard `hcp.ocm.io/*`
   labels.

#### Edit (target behavior)

1. The hub operator runs `hcp from-hub edit my-cluster --hosting-cluster
   <name>`.
2. `from-hub edit` uses the **same shared GET decoder** as
   `from-hub delete`, correctly unwrapping the proxy's
   `{ "hostedCluster": ... }` response so edits never run against an empty
   object when the proxy wraps the body.
3. The operator edits the opened YAML and saves.
4. `from-hub edit` sends a **PUT** with the full updated `HostedCluster` to
   the proxy (`Content-Type: application/json`), replacing the object on the
   hosting cluster.

#### Delete (target behavior)

1. The hub operator runs `hcp from-hub delete my-cluster --hosting-cluster
   <name>` for an AWS HostedCluster.
2. `from-hub delete aws`'s finalizer cleanup calls the proxy
   finalizer-removal endpoint instead of `util.GetClient()`, so the patch
   lands on the hosting cluster's `HostedCluster` object, and a failure is
   surfaced as a real error rather than being swallowed.

### API Extensions

This enhancement does not add or modify any CRD, admission/conversion
webhook, aggregated API server, or finalizer in the core OpenShift or
HyperShift APIs (`hypershift.openshift.io`). `api-approvers` is `None` for
that reason.

It does extend the **HCP proxy's** existing extension API surface
(`hcp.ocm.io/v1alpha1`, served by `hypershift-addon-operator`, an ACM/MCE
component outside `openshift/hypershift`'s API group) with:

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

3. A finalizer-removal endpoint for `from-hub delete aws` cleanup (exact
   route TBD with the `hypershift-addon-operator` maintainers).

None of these change behavior of existing resources owned by other parties;
they extend the HCP proxy API (`hcp.ocm.io/v1alpha1`), which is the transport
for the new from-hub subcommand. Auth/RBAC for all new routes matches the
existing proxy routes: `managedcluster:admin` via
`clusterview.open-cluster-management.io`, then impersonation toward the
hosting cluster.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement introduces HyperShift's new `hcp from-hub` CLI subcommand and
extends the `hypershift-addon-operator` HCP proxy used to manage
HostedClusters from an ACM/MCE hub. It adds new client code under
`product-cli/cmd/fromhub/` and new server-side routes in
`hypershift-addon-operator` (a separate, ACM/MCE-owned repository). It applies
when the hosting target is any `ManagedCluster` the operator can reach through
the proxy — including **`local-cluster`** (the hub registered as its own
managed cluster in MCE). It has no effect on hosted control plane components
or guest cluster workloads — the HostedCluster, NodePool, Role, and ConfigMap
objects from-hub creates/edits/deletes are handled by the existing HyperShift
Operator reconciliation on the hosting cluster exactly as if they had been
applied directly.

### Implementation Details/Notes/Constraints

The design requirements below define what the initial `hcp from-hub`
implementation must get right. Requirements 1–10 address pitfalls from reusing
`product-cli` / `hcp create cluster` code (`core.CreateCluster`,
`core.BindOptions`, platform `Complete()` / validation) that assumes local
hosting-cluster kubeconfig access via `util.GetClient()` /
`util.GetConfig()`. Requirements A–D are from-hub-specific client behaviors
that must be implemented correctly against the HCP proxy, independent of
shared core-CLI reuse.

**Core-CLI reuse requirements (1–10):**

- **Requirement 1 — `--version-check` must not use the hub HO version:** If
  `from-hub create` calls `core.CreateCluster` without special handling, the
  CLI commit SHA would be validated against whatever cluster
  `util.GetClient()` resolves to — the hub, not the hosting cluster.
- **Requirement 2 — `--release-stream` must not default from hub supported
  versions:** Release image defaulting must not read the hub's
  `supported-versions` ConfigMap; it must use hosting-cluster facts via the
  proxy (W1/W2).
- **Requirement 3 — Agent API-server-address resolution:** Agent platform
  completion must not resolve the API server address from hub nodes; use W3
  (`api.<name>.<base-domain>`) instead.
- **Requirements 4–5 — no dropped `Role`/`ConfigMap` objects:** Objects
  rendered by a platform's `GenerateResources()` that are not
  `HostedCluster`, `NodePool`, or `Secret` must be sent to the proxy via
  `ExtraObjects` (W4), not dropped client-side.
- **Requirement 6 — AWS `--secret-creds` must be rejected on from-hub:**
  Must not read secrets from the hub via `util.GetClient()`; reject via
  `unsupportedFlags` until a proxy-backed alternative exists.
- **Requirement 9 — core CLI argument allow-list:** `from-hub create` must
  not register the full `hcp create cluster` flag set. Maintain an explicit
  allow-list of supported core CLI arguments (plus from-hub-only flags). Flags
  such as `--wait`, `--render*`, and AWS `--secret-creds` stay off the
  allow-list until a hosting-cluster-correct implementation exists; any flag
  not on the allow-list is hidden from `--help` and rejected if set.
- **Requirement 10 — hosting existence/arch checks in render mode:** Render
  mode must not skip validations that need hosting-cluster facts; use proxy
  endpoints or fail closed.

**From-hub client requirements (A–D):**

- **Requirement A — shared GET decoder for `edit`/`delete`:** Both subcommands
  must decode the proxy GET response the same way, including
  `{ "hostedCluster": ... }` wrapping.
- **Requirement B — `edit` uses PUT, not PATCH:** After the user edits the
  YAML, `from-hub edit` must send the full updated `HostedCluster` in a PUT
  request. Do not use PATCH or partial merge semantics.
- **Requirement C — single `--namespace` source of truth:** One flag binding
  shared across all from-hub subcommands.
- **Requirement D — `delete aws` finalizer cleanup via proxy:** Finalizer
  removal must not use `util.GetClient()` against the hub; use a proxy
  endpoint instead.

**Proxy-backed approaches referenced in this enhancement:**

- **W1/W2 — hosting-cluster version metadata endpoint:** Unblocks Issues 1 and
  2 by returning `serverVersion` and `supportedVersions` from the hosting
  cluster through the HCP proxy.
- **W3 — Agent API server DNS default:** Default to `api.<name>.<base-domain>`;
  `--base-domain` is required on from-hub create.
- **W4 — `ExtraObjects` passthrough:** Send Roles/ConfigMaps (and similar) to
  the proxy on create instead of dropping them client-side.

Summary of planned client-side and server-side work:

| # | Requirement | Side | Status |
|---|-------------|------|--------|
| 1 | `--version-check` uses hosting HO version via proxy | Client + Proxy | Planned — proxy endpoint ([ACM-39227](https://redhat.atlassian.net/browse/ACM-39227)); reject flag until endpoint ships |
| 2 | `--release-stream` defaults from hosting supported versions | Client + Proxy | Planned — proxy endpoint ([ACM-39228](https://redhat.atlassian.net/browse/ACM-39228)), same endpoint family as #1 |
| 3 | Agent API server address from DNS default, not hub nodes | Client | Planned — W3; require `--base-domain` on from-hub create |
| 4 | Agent `Role` created on hosting cluster | Client + Proxy | Planned — `ExtraObjects` client + proxy apply ([ACM-39216](https://redhat.atlassian.net/browse/ACM-39216)) |
| 5 | ConfigMaps (e.g. `--additional-trust-bundle`) created on hosting cluster | Client + Proxy | Planned — same `ExtraObjects` mechanism as #4 |
| 6 | AWS `--secret-creds` rejected on from-hub | Client | Planned — `unsupportedFlags` on initial ship |
| 9 | Core CLI argument allow-list (not full inherited flag set) | Client | Planned — `supportedFlags` / reject unknown inherited flags on initial ship |
| 10 | Hosting existence/arch checks via proxy or fail closed | Client + Proxy | Planned — proxy endpoint or documented gap |
| A | `edit`/`delete` share one GET decoder | Client | Planned |
| B | `edit` sends PUT with full `HostedCluster` body | Client + Proxy | Planned |
| C | `--namespace` has single source of truth | Client | Planned |
| D | `delete aws` finalizer cleanup via proxy | Client + Proxy | Planned — proxy endpoint ([ACM-39226](https://redhat.atlassian.net/browse/ACM-39226)) |

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
| A hosting-cluster metadata/passthrough endpoint is unavailable or lags behind the CLI | Flags that depend on it (`--version-check`, `--release-stream`) must fail closed or remain rejected until the endpoint ships; a naive implementation could silently check the hub instead | Client always forces `VersionCheck=false`/clears `ReleaseStream` before calling `core.CreateCluster`, regardless of endpoint availability, and treats a failed/unavailable proxy call as a hard error, never a silent skip |
| New `ExtraObjects` passthrough lets the proxy create arbitrary object kinds on the hosting cluster | Expands the proxy's effective write surface beyond `HostedCluster`/`NodePool`/`Secret` | Proxy decodes and creates `ExtraObjects` using the same impersonated, RBAC-checked identity as everything else — it does not grant privilege beyond what the caller's impersonated hosting-cluster RBAC already allows; scope decoding to a known allow-list of kinds (Role, RoleBinding, ConfigMap) rather than accepting arbitrary GroupVersionKinds |

### Drawbacks

Adding hosting-cluster metadata/passthrough endpoints to the HCP proxy grows
its API surface and puts additional implementation and maintenance burden on
`hypershift-addon-operator` (a different repository/team than
`openshift/hypershift`), rather than being something `openshift/hypershift`
can deliver unilaterally. Coordinating and landing the HCP CLI client and
proxy server changes across both repositories, and keeping this enhancement's
requirement table and Jira tickets in sync as work lands, adds process
overhead compared to a single-repo feature. The `ExtraObjects`
the proxy must maintain an allow-list of object kinds it is willing to
decode/apply, which needs to be revisited whenever a platform's
`GenerateResources()` starts emitting a new kind.

## Open Questions

1. Should the hosting-cluster version-metadata endpoint (W1/W2) live at a
   dedicated `.../version` route, or be folded into an existing/broader
   hosting-cluster status endpoint the proxy may already expose? This is an
   implementation detail for `hypershift-addon-operator` maintainers to
   decide, but affects the exact client call sites in
   `product-cli/cmd/fromhub/client.go`.
2. Should `ExtraObjects` decoding on the proxy use a strict allow-list of
   `GroupVersionKind`s (Role, RoleBinding, ConfigMap) or a broader
   "anything with an RBAC-checked apply" model? A strict allow-list is
   simpler to reason about but requires a proxy code change every time a
   platform starts emitting a new kind.
3. Does the HCP proxy expose PUT for `HostedCluster` update on the existing
   from-hub edit route, or does it need a new/extended route? Confirm with
   `hypershift-addon-operator` maintainers before implementation.
4. For Requirement 10 (hosting existence/arch checks in render mode), is a
   dedicated "does this HostedCluster already exist, and what architectures
   do the hosting/NodePool nodes have" proxy endpoint worth adding on initial
   ship, or is it acceptable to document as a gap (from-hub create will fail
   server-side later if the HostedCluster already exists)?

## Test Plan

### Unit Tests

- `product-cli/cmd/fromhub/create_test.go`: extend `TestAWSUnsupportedFlags`
  (and its `unsupportedFlags` sibling for the core `--wait`/`--timeout`/
  `--render*` set) to cover `--version-check` once it moves from "always
  rejected" to "rejected only if the hosting-cluster endpoint is
  unavailable, otherwise routed through the proxy" (W1); verify
  `coreOpts.VersionCheck` and `coreOpts.ReleaseStream` are always forced to
  their disabled values before `core.CreateCluster` is invoked, regardless
  of the flag's user-facing behavior.
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
  `HostedCluster` body (`Content-Type: application/json`), not PATCH
  (Requirement B).
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
- An e2e test for `hcp from-hub edit` changing a field (e.g. adding an entry to
  `spec.services`) end-to-end, verifying the hosting cluster's `HostedCluster`
  matches the YAML the operator saved.
- An e2e test for `hcp from-hub delete aws` verifying the finalizer is
  removed from the hosting cluster's `HostedCluster` object (via the proxy
  finalizer-removal endpoint), and that a proxy-side failure surfaces as a
  CLI error rather than being swallowed.

CI for these tests spans two repositories (`openshift/hypershift` for the CLI
unit tests, and the ACM/MCE `hypershift-addon-operator` repository for the
proxy-side endpoint tests); coordinating cross-repo CI coverage for the new
endpoints is itself part of the implementation work.

## Graduation Criteria

`hcp from-hub` is a **new HCP CLI subcommand** being introduced by this
enhancement. Graduation tracks initial delivery and subsequent expansion of
the core feature set:

- [ ] `hcp from-hub` subcommand shipped in the HCP CLI with create, edit, and
  delete for **Dev Preview platforms: `aws` and `kubevirt` only**
- [ ] `--version-check` validates against the hosting-cluster HO, not the
  hub HO (proxy endpoint: [ACM-39227](https://redhat.atlassian.net/browse/ACM-39227))
- [ ] `--release-stream` defaulting uses hosting-cluster supported versions
  (proxy endpoint: [ACM-39228](https://redhat.atlassian.net/browse/ACM-39228))
- [ ] `from-hub create` never calls `util.GetClient()` for hosting-cluster
  validation/defaulting
- [ ] `hcp from-hub create` requires `--base-domain` explicitly
- [ ] `from-hub create` uses an explicit allow-list of core CLI arguments;
  inherited flags not on the allow-list are hidden and rejected
- [ ] `from-hub create` accepts only **Dev Preview platforms (`aws`, `kubevirt`)**;
  other platforms are rejected with a clear error
- [ ] Agent from-hub create defaults API server address to
  `api.<name>.<base-domain>`, never hub nodes *(when `agent` platform is added;
  not in Dev Preview)*
- [ ] Agent `Role` and trust-bundle/proxy `ConfigMap`s are created on the
  hosting cluster (proxy endpoint: [ACM-39216](https://redhat.atlassian.net/browse/ACM-39216))
  *(platform-specific; validate for `aws`/`kubevirt` Dev Preview objects in e2e)*
- [ ] `from-hub edit`/`from-hub delete` share one GET decoder matching the
  proxy response contract
- [ ] `from-hub edit` sends a PUT with the full updated `HostedCluster`, not
  a PATCH
- [ ] `--namespace` has a single source of truth shared by all from-hub
  subcommands
- [ ] `from-hub delete aws` finalizer cleanup affects the hosting-cluster
  object, or is removed if unsupported (proxy endpoint: [ACM-39226](https://redhat.atlassian.net/browse/ACM-39226))
- [ ] User documentation published for the **core from-hub feature set**, the
  core CLI **allow-list**, gradual expansion of that allow-list, and which
  operations/flags require `hcp` with hosting-cluster kubeconfig instead of
  `hcp from-hub`

This enhancement is considered complete once all boxes above are checked and
no requirements in the [Implementation
Details](#implementation-detailsnotesconstraints) table remain open.

### Dev Preview -> Tech Preview

**Dev Preview scope:** `hcp from-hub` supports create, edit, and delete for
**`aws` and `kubevirt` HostedClusters only**. Requests for other platforms
(e.g. `agent`, `powervs`, `ibmcloud`) must fail with a clear
"platform not supported in from-hub Dev Preview" error.

Promote to **Tech Preview** when:

- [ ] `aws` and `kubevirt` create/edit/delete work end-to-end through the HCP
  proxy with hosting-cluster correctness (requirements table above)
- [ ] Unsupported platforms are rejected cleanly (not silently misrouted to the
  hub)
- [ ] Dev Preview documentation lists `aws` and `kubevirt` as the supported
  platforms and points other platforms to `hcp` with hosting-cluster kubeconfig

### Tech Preview -> GA

Promote `hcp from-hub` to GA once Tech Preview criteria are met, user
documentation is published, unit/e2e tests cover `aws` and `kubevirt`
proxy-backed create/edit/delete flows, and any additional platforms or flags
added after Dev Preview each meet the same hosting-cluster correctness
requirements.

### Removing a deprecated feature

Not applicable on initial introduction. Flags hidden or rejected on from-hub
(`--wait`, `--timeout`, `--render*`, AWS `--secret-creds`, and, until the
hosting-cluster check ships, `--version-check`) remain part of
`hcp create cluster`'s flag set — they are only hidden and rejected on the
`from-hub` subcommand specifically, because they have no correct meaning in
that flow, not deprecated platform-wide.

## Upgrade / Downgrade Strategy

`hcp from-hub` is a new CLI subcommand, not a cluster component with a
persisted API, so there is no cluster upgrade/downgrade path to reason about
in the usual sense. The relevant compatibility surface is CLI-version-to-proxy-
version skew after introduction:

- **Newer CLI, older proxy:** A `hcp` CLI calling metadata (W1/W2),
  `ExtraObjects`-aware create, or finalizer-removal endpoints against an
  `hypershift-addon-operator` that has not yet added them must fail closed
  with a clear "unsupported by proxy" error (e.g. on a 404), not silently
  fall back to hub-checking behavior.
- **Older CLI, newer proxy:** An older `hcp` CLI simply never calls endpoints
  the proxy already exposes; behavior matches whatever that CLI version
  shipped with.
- **`ExtraObjects` field addition:** Purely additive on the wire
  (`omitempty`); proxies that do not yet decode the field ignore it — no
  wire breakage, but Roles/ConfigMaps are not created until both sides support
  `ExtraObjects`.

Users adopt `hcp from-hub` by upgrading `hcp` and `hypershift-addon-operator`
to versions that implement the client and proxy contract described here.

## Version Skew Strategy

This enhancement is entirely within `hcp from-hub` (client) and the HCP
proxy (`hypershift-addon-operator`, server). There is no kubelet, CRI, or CNI
version-skew concern. The two sides communicate over the versioned
`hcp.ocm.io/v1alpha1` HTTP contract, and the proxy remains the sole source of
truth for hosting-cluster facts — the CLI never needs to reason about which
HyperShift Operator version is running on the hosting cluster except through
the version-metadata endpoint itself (W1), so ordinary HyperShift Operator
upgrades on the hosting cluster do not require any from-hub CLI change.

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
- **Failure mode — `ExtraObjects` decode/apply fails on the proxy for one
  object:** The proxy should fail the whole create request rather than
  partially create some hosting-cluster objects and not others, to avoid an
  `HostedCluster` referencing a ConfigMap/Role that was supposed to exist but
  doesn't (the exact partial-failure/rollback semantics are an open
  implementation detail for `hypershift-addon-operator`).
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
