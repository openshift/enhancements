---
title: ignition-system-re-architecture
authors:
  - "@muraee"
reviewers:
  - "@csrwng, HyperShift expertise, for the CPO->HO ownership move and NodePool controller changes"
  - "TBD, managed services (ROSA/ARO) expertise, for the scaling and migration model"
approvers:
  - "@csrwng"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-09-15
last-updated: 2026-09-15
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-3052
see-also:
  - "/enhancements/hypershift/predictable-nodepool-rollout-control.md"
replaces:
superseded-by:
---

# Ignition System Re-Architecture for Hosted Control Planes

## Summary

The HyperShift ignition system today spans three controllers plus an HTTP
server that coordinate through a single Kubernetes Secret written by four
different actors and read by a fifth. This makes the system hard to reason
about, debug, and extend, and — most importantly — it cannot scale
horizontally: the payload generator and HTTP server share an in-pod cache, so
running multiple replicas means every replica pulls release images (registry
DoS) and updates Secrets (KAS DoS). This enhancement re-architects the system
around a new `IgnitionPayload` CRD that cleanly separates *payload generation*
(expensive, singleton, leader-elected) from *payload serving* (cheap,
stateless, horizontally scaled), unifies all config parsing/validation/hashing
in a single `PayloadController`, moves the ignition server binary from the
Control Plane Operator (CPO) to the HyperShift Operator (HO) to eliminate
coordination skew, and makes ignition state observable via
`kubectl get ignitionpayload`. The primary requirement (OCPSTRAT-3052) is the
generation/serving split; the CRD, config unification, and ownership move are
architectural cleanup that lands together to avoid touching the ignition path
across multiple releases.

## Motivation

The current ignition system (~3,420 non-test lines) has accumulated structural
problems that block scaling and slow down every change to worker-node
bootstrapping:

- **Cannot scale horizontally.** The `TokenSecretReconciler` and the HTTP
  server share an in-memory cache inside the same pod. Running N replicas means
  all N pull release images and update Secrets, converting a scale-out into a
  registry/KAS denial-of-service. This is the subject of OCPSTRAT-3052.
- **Fragile coordination.** Annotations act as an implicit protocol between
  controllers, and a single token Secret carries inputs, auth credentials,
  status, rotation state, and lifecycle signals all at once.
- **Unnecessary lifecycle machinery.** Token rotation, a dedicated janitor
  controller, dual secrets per config version, and platform-specific cleanup
  hacks for AWS/KubeVirt all exist to work around the Secret-as-database model.
- **Poor observability.** Understanding ignition state requires decoding Secret
  data keys and annotations; there is no `kubectl get` view.
- **HO/CPO coordination skew.** The ignition server is a CPO subcommand tied to
  the OCP release, while the NodePool controller ships in the HO and releases
  independently. Any coordination change requires dual code paths gated on CPO
  capabilities.

### User Stories

* As a managed-services SRE operating many hosted clusters, I want the ignition
  serving tier to scale horizontally, so that a burst of node provisioning does
  not overwhelm the registry or the management-cluster API server.
* As a HyperShift developer, I want a single component that owns config
  parsing, validation, hashing, and payload generation, so that I can reason
  about, debug, and extend the ignition path without tracing a Secret shared by
  four actors across two namespaces.
* As an autoscaling (Karpenter) integrator, I want to request an ignition
  payload by creating a first-class resource, so that just-in-time nodes get a
  payload without fabricating a throwaway in-memory NodePool.
* As an SRE, I want to run `kubectl get ignitionpayload` and see each
  consumer's ignition lifecycle — current hash, ready, reached, previous
  version — so that I can diagnose a stuck rollout without decoding Secret data.
* As an SRE responsible for operating this at scale, I want payload generation
  and serving to be observable through status conditions and metrics and to
  self-heal (leader failover on pod loss, cold cache rehydrate on restart), so
  that I can monitor fleet health and remediate without manual payload surgery.

### Goals

- Payload **serving** scales horizontally (N stateless replicas) while payload
  **generation** remains a single active worker, so that in steady state exactly
  one release-image pull occurs per config version regardless of replica count
  (a leader failover before `PayloadStore.Put` can cost one extra pull).
- Ignition state for every consumer is observable at a glance via
  `kubectl get ignitionpayload` and status conditions.
- Management-side changes (e.g. HAProxy image bumps) never trigger fleet-wide
  node rollouts, while newly provisioned and scaled-up nodes still boot with
  current content — preserving and building on the guarantee from
  [predictable-nodepool-rollout-control](./predictable-nodepool-rollout-control.md).
- Payload generation is a first-class service that any controller can consume
  by creating a resource, without depending on the NodePool type.
- The system sheds lifecycle machinery — token rotation, the janitor
  controller, and annotation-based protocols — reducing the number of moving
  parts and failure modes operators must understand.

### Non-Goals

- Changing the MCO subprocess pipeline that extracts and runs
  machine-config-operator binaries to generate payloads; simplifying it
  requires MCO-team collaboration and is out of scope.
- Removing the standalone HAProxy proxy Deployment in v1. It remains the
  request-serving isolation boundary; retiring it is a v2 change contingent on a
  KAS-free serving tier and security sign-off.
- Migrating the in-place upgrade path (HCCO `InPlaceUpgrader`) off the legacy
  token Secret. A byte-compatible compatibility write is retained so no HCCO
  change is required; moving it to the PayloadStore is a later, release-tied
  step.
- Reducing raw line count. The value here is architectural clarity, horizontal
  scaling, and observability; net lines rise during the coexistence window and
  fall only after the frozen old module is deleted at end-of-life (EOL).

## Proposal

Introduce a new namespaced `IgnitionPayload` CRD (one per consumer request,
living in the hosted control plane (HCP) namespace) that carries the
consumer's inputs in `spec` and the generator's results in `status`. Around it:

1. **Split generation from serving into two Deployments.** A leader-elected
   `ignition-payload-controller` (2 replicas, one active) runs the MCO pipeline
   and writes payloads; a stateless `ignition-server` (3 replicas, all active)
   serves `GET /ignition`. Payload bytes cross the pod boundary through a
   `PayloadStore` (v1 Secret-backed, v2 database-backed), never through the CRD.
2. **Unify all config logic in the PayloadController.** Config splitting,
   defaulting/validating, both hashes (payload-identity and rollout), and the
   MCO pipeline all live in one component. The NodePool controller is reduced to
   copying config sources into the HCP namespace and classifying them, with no
   parse/validate/hash/MCO logic.
3. **Move the ignition server from the CPO to the HO.** The server ships in the
   same binary as the NodePool controller, eliminating HO/CPO coordination skew
   and the dual code paths gated on CPO capabilities.
4. **Detect rollouts downstream of validation.** The PayloadController — the
   only component that has validated the config — computes the rollout hash and
   advances `status.current.generation`, so an invalid config can never trigger
   a rollout that has no payload to serve.

The change deliberately lands as one unit: the cross-pod `PayloadStore` and the
two-Deployment split are the shared foundation regardless, a CPO-only phase
would build on top of the token-Secret coordination this redesign exists to
remove, and the migration rides a mandatory N->N+1 upgrade window either way.

### Workflow Description

**NodePool controller** (the primary *consumer*) runs in the management
cluster as part of the HyperShift Operator. **PayloadController** (the
*generator*) and **ignition-server** (the *server*) run in the HCP namespace.
**Karpenter** is a second consumer. A **worker node** fetches its ignition
config at boot.

Starting state: a NodePool exists and references user/core/NTO configs.

1. The NodePool controller creates or updates an `IgnitionPayload` CR (with a
   finalizer) in the HCP namespace and enumerates the config sources in
   `spec.rolloutConfigRefs` (user/core/NTO) or `spec.mgmtConfigRefs` (the
   apiserver-HAProxy config). Sources that do not already live in the HCP
   namespace it projects there as CR-owned ConfigMaps — the user configs (copied
   from the clusters namespace) and the HAProxy config it authors — while the
   core and NTO ConfigMaps already reside in the HCP namespace and are referenced
   in place with no CR owner reference. It sets scalar inputs (`releaseImage`,
   `pullSecretName`, `additionalTrustBundle`, `osStream`) and the
   rollout-relevant global-config subset (`rolloutGlobalConfig`). It never
   opens a ConfigMap's contents and computes no hash.
2. The PayloadController (leader) reacts to the CR or to a change in any
   referenced ConfigMap. It reads each referenced ConfigMap, splits it into
   individual manifests, and defaults + validates each. If validation fails it
   sets `PayloadGenerated: False` and stops — no hash, no token, no rollout.
3. Over the validated config it computes the payload-identity hash (whole
   config) and the rollout hash (`rolloutConfigRefs` config + version +
   pullSecret + trustBundle + `rolloutGlobalConfig` + osStream). Before pulling
   the release image it calls `PayloadStore.FindByIdentity` for this CR and the
   payload-identity hash; on a hit it reuses that entry and skips the pull,
   otherwise it runs the MCO pipeline (which pulls the release image).
4. If the rollout hash changed, it moves `status.current` to `status.previous` —
   deleting from the store any token already occupying `status.previous`
   (delete-on-evict), so at most two tokens (current + previous) are ever live
   per CR even when rollouts overtake each other (A->B->C) — mints a new UUID
   token, calls `PayloadStore.Put(newToken, payload)`, and sets `status.current`
   with the new hashes, token, and `generation + 1`. Otherwise
   (management-side/cloud-config change only) it refreshes the bytes behind the
   *current* token and updates only `status.current.configHash`.
5. When `status.current.generation` advances, the NodePool controller creates a
   userdata Secret embedding `status.current.token` and re-points the
   MachineDeployment; CAPI provisions new machines with the new payload.
6. A booting node calls `GET /ignition` with `Authorization: Bearer <token>`.
   The ignition-server serves the payload from its per-pod local cache; on a
   cache miss (e.g. a token published before the replica's informer has hydrated
   it) it reads through to `PayloadStore.Get`, serves the result, and populates
   the local cache — so there is no window between `PayloadStore.Put` and
   informer propagation in which a valid token returns HTTP 511. On the first
   successful serve of the *current* token (`served token ==
   status.current.token`) it sets `IgnitionReached: True` on the CR via one
   idempotent status update; serving a `status.previous` token during drain does
   not flip the condition, so a node still booting on the old generation cannot
   mark the new one reached.
7. When a rollout drains (old MachineDeployment scaled to 0), the NodePool
   controller deletes the retired userdata Secret and advances
   `spec.retiredGeneration`. The PayloadController frees the old token from the
   store and clears `status.previous`.
8. On teardown, the creating consumer removes its consumer finalizer and deletes
   the CR; the PayloadController then frees all remaining store tokens and removes
   its store-cleanup finalizer, after which only the CR-owned projections (the
   user and HAProxy ConfigMaps) cascade-delete via their owner references — the
   referenced-in-place core and NTO ConfigMaps carry no owner reference and are
   left untouched.

```mermaid
sequenceDiagram
    participant NP as NodePool controller (consumer)
    participant CR as IgnitionPayload CR
    participant PC as PayloadController (leader)
    participant PS as PayloadStore
    participant SRV as ignition-server (xN)
    participant Node as Worker node (ignition)

    NP->>CR: Create/update spec + project config ConfigMaps
    PC->>CR: Watch CR + referenced ConfigMaps
    PC->>PC: Split + validate configs, compute both hashes
    alt rollout hash changed
        PC->>PS: Put(newToken, payload)
        PC->>CR: status.current = {hashes, token, gen+1}
        NP->>NP: Create userdata Secret + re-point MachineDeployment
    else management/cloud-config change only
        PC->>PS: Put(current token, refreshed payload)
        PC->>CR: update status.current.configHash only
    end
    Node->>SRV: GET /ignition (Bearer token)
    SRV->>PS: hydrate local cache
    SRV-->>Node: serve payload
    SRV->>CR: set IgnitionReached=True (one-shot)
    NP->>CR: advance spec.retiredGeneration after drain
    PC->>PS: Delete(previous token); clear status.previous
```

### API Extensions

This enhancement adds a new CRD and a finalizer:

1. **New CRD: `IgnitionPayload`** (group `hypershift.openshift.io`), namespaced,
   living in the HCP namespace. One resource per consumer request (one per
   NodePool for the NodePool controller; Karpenter creates its own on demand).
   The CRD is **consumer-agnostic** — it carries no back-reference to a NodePool
   so that non-NodePool consumers can use it. Payload bytes do **not** live in
   status; the token is the key into the `PayloadStore`. The type is defined
   here (v1alpha1, group `hypershift.openshift.io`):

   ```go
   // IgnitionPayload is one consumer's ignition payload request: the consumer's
   // inputs in spec, the generator's results in status. One resource exists per
   // consumer request (one per NodePool for the NodePool controller; Karpenter
   // creates its own on demand). The type is consumer-agnostic — it carries no
   // back-reference to a NodePool so non-NodePool consumers can use it.
   //
   // +kubebuilder:object:root=true
   // +kubebuilder:subresource:status
   // +kubebuilder:resource:path=ignitionpayloads,shortName=ignpayload,scope=Namespaced
   // +kubebuilder:printcolumn:name="Generation",type=integer,JSONPath=`.status.current.generation`
   // +kubebuilder:printcolumn:name="Generated",type=string,JSONPath=`.status.conditions[?(@.type=="PayloadGenerated")].status`
   // +kubebuilder:printcolumn:name="Reached",type=string,JSONPath=`.status.conditions[?(@.type=="IgnitionReached")].status`
   type IgnitionPayload struct {
   	metav1.TypeMeta   `json:",inline"`
   	metav1.ObjectMeta `json:"metadata,omitempty"`

   	// spec is written by the consumer and describes the desired payload inputs.
   	// +required
   	Spec IgnitionPayloadSpec `json:"spec"`

   	// status is written by the PayloadController and reports generation and
   	// rollout progress.
   	// +optional
   	Status IgnitionPayloadStatus `json:"status,omitempty"`
   }

   // IgnitionPayloadSpec is written entirely by the consumer; the PayloadController
   // treats it as read-only input. The consumer declares inputs and classifies
   // config sources — it never opens a ConfigMap's contents and computes no hash.
   type IgnitionPayloadSpec struct {
   	// releaseImage is the pullspec of the OCP release whose
   	// machine-config-server binaries render the payload. The PayloadController
   	// resolves it to an immutable digest before pulling it.
   	// +required
   	// +kubebuilder:validation:MinLength=1
   	ReleaseImage string `json:"releaseImage"`

   	// pullSecretName is the name of a Secret in the CR's namespace holding the
   	// registry pull secret used to fetch the release image and embedded in the
   	// payload.
   	// +required
   	// +kubebuilder:validation:MinLength=1
   	PullSecretName string `json:"pullSecretName"`

   	// additionalTrustBundle optionally references a ConfigMap in the CR's
   	// namespace holding a PEM CA bundle for booting nodes to trust.
   	// +optional
   	AdditionalTrustBundle *corev1.LocalObjectReference `json:"additionalTrustBundle,omitempty"`

   	// osStream selects the RHEL OS stream (e.g. "rhel-9") the payload targets.
   	// +optional
   	OSStream string `json:"osStream,omitempty"`

   	// rolloutGlobalConfig carries the rollout-relevant subset of the hosted
   	// cluster's global configuration, canonicalized by the consumer. It is a
   	// rollout-hash input; see predictable-nodepool-rollout-control (#8698).
   	// +optional
   	RolloutGlobalConfig string `json:"rolloutGlobalConfig,omitempty"`

   	// rolloutConfigRefs lists ConfigMaps in the CR's namespace whose contents
   	// are rollout-relevant (user, core, and NTO machine configs). A change to
   	// any of them can advance the rollout hash and trigger a node rollout.
   	// +optional
   	// +listType=map
   	// +listMapKey=name
   	RolloutConfigRefs []corev1.LocalObjectReference `json:"rolloutConfigRefs,omitempty"`

   	// mgmtConfigRefs lists ConfigMaps in the CR's namespace whose contents are
   	// management-side only (the apiserver-HAProxy config). A change to them
   	// refreshes the payload behind the current token without a rollout.
   	// +optional
   	// +listType=map
   	// +listMapKey=name
   	MgmtConfigRefs []corev1.LocalObjectReference `json:"mgmtConfigRefs,omitempty"`

   	// retiredGeneration is a level-triggered signal that the payload of the
   	// given generation has drained (its nodes are gone) and its store token may
   	// be freed. The PayloadController deletes store entries at or below this
   	// generation except the one backing status.current.
   	// +optional
   	// +kubebuilder:validation:Minimum=0
   	RetiredGeneration int64 `json:"retiredGeneration,omitempty"`
   }

   // IgnitionPayloadStatus has two writers with disjoint field ownership. The
   // PayloadController owns current, previous, and the PayloadGenerated
   // condition; the serving tier owns only the IgnitionReached condition and
   // sets it through a field-scoped, conflict-retried patch (see the Status
   // ownership contract in Implementation Details). No writer replaces the whole
   // status.
   type IgnitionPayloadStatus struct {
   	// current describes the payload for the latest validated, generated config.
   	// +optional
   	Current *PayloadReference `json:"current,omitempty"`

   	// previous describes the immediately prior payload, retained during a
   	// rollout so in-flight boots on the old token are served until they drain.
   	// It is serving/observability state, not the cleanup mechanism: tokens are
   	// reclaimed by delete-on-evict and the store-cleanup finalizer, not by
   	// tracking every generation here.
   	// +optional
   	Previous *PayloadReference `json:"previous,omitempty"`

   	// conditions reports generation and rollout progress. Known types:
   	// "PayloadGenerated" (the latest config produced a payload) and
   	// "IgnitionReached" (a node has fetched status.current's token; written by the
   	// serving tier under the Status ownership contract).
   	// +optional
   	// +listType=map
   	// +listMapKey=type
   	Conditions []metav1.Condition `json:"conditions,omitempty"`
   }

   // PayloadReference identifies one generated payload version and its store key.
   type PayloadReference struct {
   	// configHash is the payload-identity hash over the whole validated config.
   	// It also labels the payload's store entry, making generation idempotent
   	// across leader failover.
   	// +required
   	ConfigHash string `json:"configHash"`

   	// rolloutHash is the hash over the rollout-relevant inputs. A change here
   	// advances generation and triggers a node rollout.
   	// +required
   	RolloutHash string `json:"rolloutHash"`

   	// token is an opaque, non-derivable UUID: the key into the PayloadStore for
   	// this version. It is a capability to fetch the payload, not the payload
   	// itself (bytes never live in status), and it becomes unusable the instant
   	// its store entry is deleted (after which GET /ignition returns HTTP 511).
   	// Its authorization model for GET /ignition is unchanged from today's
   	// ignition server.
   	// +required
   	Token string `json:"token"`

   	// generation is a monotonically increasing counter the consumer watches to
   	// execute a rollout. It advances only when rolloutHash changes.
   	// +required
   	// +kubebuilder:validation:Minimum=0
   	Generation int64 `json:"generation"`
   }
   ```

2. **Finalizers on `IgnitionPayload`.** The CR lives in the HCP namespace while
   consumers (NodePool, Karpenter) live elsewhere, so a cross-namespace ownerRef
   is not possible; lifecycle is finalizer-driven with two owners. (a) A
   **consumer finalizer** is added by whichever controller creates the CR (the
   NodePool controller, or Karpenter for its on-demand CRs); that same controller
   removes it after tearing down its side and is the one that issues the CR
   delete. (b) A **PayloadController store-cleanup finalizer** is removed only
   after the PayloadController has deleted every remaining `PayloadStore` token
   for the CR. This split keeps store reclamation consumer-agnostic, so a
   Karpenter-created CR's tokens are freed even though Karpenter, not the NodePool
   controller, drives its deletion. Once both finalizers clear, the CR's owned
   ConfigMaps cascade-delete via their owner references.

This enhancement also changes the behaviour of the HyperShift NodePool
controller and the ignition server, but it does not modify any CRDs owned by
other teams or any core OpenShift API resources. It does not add admission,
conversion, or mutation webhooks, nor an aggregated API server.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is exclusively about HyperShift / Hosted Control Planes; the
ignition system exists only in this topology. It affects components running in
the management cluster (the NodePool controller in the HO) and components
running in the HCP namespace (the PayloadController, ignition-server, and proxy
Deployments). The move of the ignition server from the CPO image to the HO
image changes which binary hosts these components but keeps the per-HostedCluster
Route/Service serving topology unchanged. The guest cluster is unaffected except
that the in-place upgrade channel (HCCO `InPlaceUpgrader`) continues to read a
byte-compatible legacy token Secret, so no guest-side change is required.

#### Standalone Clusters

Not applicable. Standalone clusters do not use NodePools or the HyperShift
ignition server; they bootstrap nodes through the Machine Config Server.

#### Single-node Deployments or MicroShift

Not applicable. The ignition server and NodePool controller are HyperShift
management-side components and are not present in SNO or MicroShift. This change
does not alter SNO or MicroShift resource consumption and adds no MicroShift
configuration surface.

#### OpenShift Kubernetes Engine

Not applicable in the sense that this change is internal to the HyperShift
operator and does not depend on any feature excluded from the OKE product
offering. Where HyperShift is offered, the re-architecture applies transparently.

### Implementation Details/Notes/Constraints

**Component layout (reduced from 4 to 3, all in the HO binary).** The
`TokenSecretReconciler`, the secret janitor, and the CPO ignition server
component are eliminated. The PayloadController (leader-elected, 2 replicas) and
ignition-server (stateless, 3 replicas) become two separate Deployments in the
HCP namespace, wired via the existing CPOv2 component framework the HO already
uses to deploy CAPI.

**Ownership move (package extraction).** `local_ignitionprovider.go` and
`cmd/start.go` import a handful of CPO packages (`hostedcontrolplane/common`,
`imageprovider`, `manifests`, `support/releaseinfo`). Before the move these are
extracted into a shared package importable by both binaries, avoiding an HO->CPO
import cycle. Payload *generation* is version-agnostic in function — the MCO
binaries are extracted from the release image at runtime and version-dependent
behavior branches on the release-image version, not the CPO build — so the same
code produces correct payloads for every OCP version in the fleet.

**Config resolution: one owner.** The NodePool controller copies each config
source into the HCP namespace as its own ConfigMap (owned by the CR) and
classifies it into `rolloutConfigRefs` vs `mgmtConfigRefs`. The PayloadController
reads exactly the ConfigMaps named in those lists — a self-describing input set
with no label query — gathers them in a deterministic (sorted) order, splits
multi-document YAML, validates each manifest, and computes both hashes over the
validated output. Per-ConfigMap projection (rather than one concatenated bundle)
keeps every object within the etcd size limit its source already respects and
preserves provenance for validation errors. Core and NTO ConfigMaps already
reside in the HCP namespace and are referenced in place (a mapped `Watches`,
ported from the existing `enqueueNodePoolsForConfig` dispatcher); user and
HAProxy ConfigMaps are CR-owned (a plain `Owns`).

**PayloadStore.** The token-keyed core (`Put`/`Get`/`Delete`) already
exists in today's ignition server, backed by an in-pod `ExpiringCache`. Because
the token is an opaque UUID, two flows must address entries without holding a
token: idempotent generation has to find an existing entry by payload identity,
and the store-cleanup finalizer has to reach every entry for a CR (including one
an interrupted generation left unreferenced by status). The interface therefore
adds two lookups — `FindByIdentity(owner, identityHash)` and `ListByOwner(owner)`
— resolved against labels the entries already carry, so they are
backend-independent rather than assuming a Secret query. v1 swaps
the implementation for a `SecretBackedStore`: the generator writes each payload
to a Secret in the HCP namespace labeled with its owning CR and payload-identity
hash, and server replicas hydrate a per-pod
local cache (emptyDir/memory) via an informer; there the two lookups are
label-selector `List` calls. The local cache is an
optimization, not a correctness dependency: the `/ignition` handler reads
through to `PayloadStore.Get` on a miss, so a token is servable as soon as `Put`
returns, before the informer propagates it. On a management-side refresh (Policy
A) the generator overwrites the store entry for the *current* token in place;
the informer delivers an update event and each replica replaces its cached bytes
for that token (last-write-wins), so a replacement node served by any replica
gets the current content. v2 swaps in a
`DatabaseBackedStore` keyed by token, removing the etcd object-size cap on
payloads without changing the CRD contract or the serving hot path; there the
same two lookups are indexed queries on `(owner, identityHash)` and `owner`.

**Idempotent generation across failover.** Each store entry is labeled with its
owning CR and the payload-identity hash of the content it holds. Before pulling
the release image, the leader calls `FindByIdentity` for the target
payload-identity hash and reuses any existing entry instead of regenerating.
Leader election already guarantees a single active generator, so this labeling
closes the remaining gap: a failover that interrupts an in-flight generation
costs at most one repeated pull for that config version (the new leader may
re-pull only if the crash preceded `PayloadStore.Put`), and the steady-state
guarantee is exactly one pull per config version at any replica count. If such a
crash lands after `Put` but before the `status` write and the config then
advances, the entry it wrote is left unreferenced by `status`; it is not swept
mid-life — it is reused if that identity recurs, and otherwise reclaimed at CR
teardown when the store-cleanup finalizer deletes everything `ListByOwner`
returns.

**Rollout detection lives with generation.** Because a rollout must not be
triggered for a config that cannot produce a payload, rollout detection is
placed downstream of validation, in the PayloadController. It emits a new token
and advances `status.current.generation` only when the rollout hash changes; the
NodePool controller reacts to that generation bump to execute the rollout
(userdata Secret, MachineDeployment re-point, drain watch, `retiredGeneration`).
Payload refresh follows **Policy A**: the token is a rollout identity, and
management-side/cloud-config changes refresh the bytes behind the current token
so scale-up/replacement nodes always boot with current content while existing
nodes are untouched until they roll.

This work depends on and extends
[predictable-nodepool-rollout-control](./predictable-nodepool-rollout-control.md)
(PR #8698), which must merge first: its rollout-vs-management-side hash split
moves into the PayloadController, and the rollout state it holds in annotations
(`nodePoolCurrentRolloutConfig`) and the `Token.isOutdated()` gate migrate into
CRD `status` (`current.generation` / `current.rolloutHash`), so those
annotation/token mechanisms are not carried over.

**Status ownership.** `IgnitionPayloadStatus` has two writers with disjoint
field ownership rather than a single owner. The PayloadController writes
`status.current`, `status.previous`, and the `PayloadGenerated` condition. The
serving tier writes only the `IgnitionReached` condition, and does so with a
field-scoped patch — Server-Side Apply under its own field manager (or a
JSON-patch targeting just that condition entry) — never a full-status replace, so
it can never clobber `current`/`previous`. That write is conditional and
conflict-retried: a replica sets `IgnitionReached: True` only while
`status.current.token` still equals the token it served, re-checking under
optimistic concurrency and dropping the update if a retry observes the generation
has advanced. Field-scoping plus the token precondition together mean concurrent
writes can neither lose the PayloadController's rollout reset nor overwrite
unrelated status fields, so no separate reporting resource or single-owner funnel
is needed.

**Legacy in-place-upgrade compatibility.** The HCCO `InPlaceUpgrader` reads a
Secret named `token-{machineSetName}-{targetConfigVersion}`, where
`targetConfigVersion` is the payload-identity hash (`status.current.configHash`)
stamped on the matching MachineSet annotation. The **NodePool controller** owns
this compatibility write — it already owns CAPI naming, the userdata Secret, and
MachineSet annotations, whereas making the PayloadController the writer would
force MachineSet names into the consumer-agnostic CRD spec. In the same reconcile
that reacts to a `status.current.generation` advance and creates the userdata
Secret, the NodePool controller also writes the byte-compatible legacy Secret and
stamps the MachineSet annotation, both *before* re-pointing the MachineDeployment,
so HCCO never observes the annotation without its Secret. The step is
level-triggered and idempotent: if either write fails the reconcile requeues and
does not re-point the MachineDeployment until the userdata Secret, the legacy
Secret, and the annotation all exist. This is why no HCCO change is required — the
guest side keeps reading the same Secret shape it reads today.

**Feature gating.** This is a HyperShift management-side operator change and is
**not** gated through the OpenShift feature-gate mechanism in
`https://github.com/openshift/api/blob/master/features/features.go` — that
mechanism gates guest-cluster/product feature sets (DevPreviewNoUpgrade /
TechPreviewNoUpgrade), not HyperShift-operator-internal architecture. Release
gating is instead achieved through the migration model (see
[Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)): the new path is
selected per-HostedCluster and the old token-Secret path is retained as a
frozen, independent module until fleet EOL. If, during review, stakeholders
prefer an explicit opt-in (e.g. a HyperShift annotation on HostedCluster to
select the new path as Tech Preview before it becomes default), that can be
layered on top of the migration model without changing the core design.

### Risks and Mitigations

- **v1 serving tier is not KAS-free.** With `SecretBackedStore`, server
  replicas need management-cluster KAS access to run the Secret informer and to
  write `IgnitionReached`. *Mitigation:* the serving tier stays behind the
  standalone HAProxy proxy Deployment — the only externally reachable hop, kept
  off KAS-connected nodes. Reviewers of this enhancement should confirm this
  boundary holds. v2 removes the payload-read KAS dependency and can move
  `IgnitionReached` off the hot path, after which the proxy can be retired with
  security sign-off.
- **Cross-tenant data boundary.** Copying user ConfigMaps into the HCP namespace
  moves the same data that already crosses the boundary today (as `mcoRawConfig`
  inside the token Secret) one step earlier; it does not introduce a new class of
  data to the boundary. Reviewers should confirm this equivalence.
- **Generator availability gap.** At a single generator replica, a pod loss
  creates a cold-start gap during which new rollouts and scale-up payload
  refreshes stall. *Mitigation:* run 2 replicas with leader election; standby
  takes over on lease expiry (seconds). Already-served nodes are unaffected.
- **Fleet-wide requeue amplification.** A core (cluster-wide) config change
  enqueues every `IgnitionPayload` CR in the namespace — the same amplification
  the NodePool controller has today. *Mitigation:* this is a known,
  pre-existing characteristic; core/NTO are referenced in place rather than
  copied per-CR to avoid stacking write amplification on the fan-out.
- **Broken management-side render reaching new nodes without a rollout signal.**
  Under Policy A a bad refresh could reach scale-up nodes. *Mitigation:* the
  PayloadController reports render health as `PayloadGenerated: False` + error
  for operators to alert on.
- **UX review.** The `kubectl get ignitionpayload` view and status conditions
  should be reviewed with SRE/managed-services stakeholders to ensure the
  surfaced columns match operational needs.

### Drawbacks

- **Net LOC rises during the coexistence window.** The frozen old token-Secret
  module is retained until fleet EOL, so the line-count benefit only materializes
  after cleanup — and the value proposition is architectural, not LOC.
- **More objects to manage.** Per-ConfigMap projection means N ConfigMaps to
  sync, watch, and garbage-collect per consumer instead of one bundle (though GC
  is automatic via owner references and the ref lists are self-describing).
- **New API surface.** A new CRD and finalizer add to the HyperShift API surface
  and require API review and long-term maintenance.
- **More pods per HCP.** Two Deployments (2 + 3 replicas) plus the proxy
  increase the per-HostedCluster pod count relative to today's single ignition
  server Deployment.
- **Coexistence complexity.** The migration introduces a branch point and a
  frozen module that must keep compiling until EOL.

## Alternatives (Not Implemented)

- **Follower-side generation.** Have the leader distribute a cheaper
  intermediate context so server replicas generate ignition locally. Rejected:
  the payload is static per config version, and any pre-final intermediate still
  needs the version-matched `machine-config-server` binary from the release
  image — so followers would still pull the image and fan compute across all
  replicas, reintroducing the exact registry/KAS DoS this design targets. The
  finished payload distributed via the PayloadStore is the cheapest complete
  context and gives better failover.
- **Phasing as a smaller CPO-only split.** Ship only the generation/serving
  split, keeping the generator CPO-internal and the token-Secret contract.
  Rejected as the default: it builds the split on top of the coordination
  mechanism this redesign removes (throwaway code), and it disturbs the ignition
  path across two releases. Phasing would win only if the DoS were a live
  production incident (it is a scaling-headroom concern), in which case a
  migration-free CPO-internal split could ship first to buy time.
- **Policy B (homogeneous NodePool) as the default.** Keep every node in a
  NodePool on byte-identical content, so management/cloud-config changes reach
  nodes only via a coordinated rollout. Deferred, not rejected: it trades
  "new nodes get current content immediately" for "no in-NodePool drift" and a
  tighter blast radius; it could be exposed per-NodePool via a future API field.
  Policy A applies fleet-wide until then.
- **Payload bytes in CRD status.** Rejected: it would cap payloads at the etcd
  object size and bloat every status read. Bytes live in the PayloadStore.

## Open Questions [optional]

1. **Policy A vs Policy B exposure.** Should payload-refresh behavior be a
   per-NodePool API field (e.g. `spec.management.payloadRefresh:
   Immediate | OnRollout`), or is fleet-wide Policy A sufficient indefinitely?
2. **v2 PayloadStore backend.** Which database backing (and its HA/backup story)
   should `DatabaseBackedStore` use, and when does v2 land relative to v1?
3. **Proxy retirement in v2.** Under what exact conditions does security sign off
   on retiring the HAProxy proxy Deployment once the serving tier is KAS-free?

## Test Plan

<!-- TODO: Complete once targeted at a release.
Per dev-guide/feature-zero-to-hero.md and dev-guide/test-conventions.md, tests
must be appropriately labeled. Because this is a HyperShift management-side
change with no openshift/api feature gate, the standard
`[OCPFeatureGate:FeatureName]` label does not apply; tests instead run in the
HyperShift e2e suite. Include:
- `[Jira:"HyperShift"]` (or the appropriate component) so regressions are routed.
- Suite/behavior labels as needed: `[Serial]`, `[Slow]`, `[Disruptive]`.
Fill in the concrete cases below. -->

The general strategy:

- **Unit tests** for the PayloadController: config split + validation +
  deterministic gather order, both hashes, rollout detection (rollout-relevant
  vs management-side change), and the delete-on-evict path for rollouts that
  overtake each other (A->B->C within one rollout window).
- **Unit/integration tests** for the NodePool controller's reduced role: config
  projection, ref-list classification, `retiredGeneration` advancement,
  finalizer-driven CR delete and cascade of owned ConfigMaps.
- **Integration tests** for the `PayloadStore` (SecretBackedStore + informer
  hydration; cold rehydrate on restart) and the two-Deployment leader-election
  split (exactly one release-image pull per config version at any replica count).
- **e2e tests**: node provisioning through `GET /ignition`, a rollout-triggering
  config change, a management-side change that must *not* roll the fleet, and
  the N->N+1 migration cutover (see Upgrade/Downgrade).
- **Managed-service coverage**: exercise scale-out of the serving tier under
  concurrent node provisioning to confirm no registry/KAS amplification.

## Graduation Criteria

<!-- TODO: Complete once targeted at a release.
For OpenShift features promoted via openshift/api, dev-guide/feature-zero-to-hero.md
requires: at least 5 tests per feature, run >=7 times/week, >=14 times per
supported platform, >=95% pass rate, on all supported platforms (AWS HA/Single,
Azure, GCP, vSphere, Baremetal IPv4/IPv6/Dual), in place >=14 days before branch
cut. Because this change ships in the HyperShift operator (no openshift/api
feature gate), graduation follows HyperShift's own conventions and CI signals;
document the concrete platform matrix and signal thresholds here when targeting
a release. -->

### Dev Preview -> Tech Preview

- Ability to use the new ignition path end to end on at least one platform.
- `PayloadGenerated` and `IgnitionReached` conditions and the
  `kubectl get ignitionpayload` view implemented.
- Sufficient unit/integration coverage; SLIs enumerated and exposed as metrics
  (generation latency, store size per NodePool, informer lag, leader failovers).
- Symptoms-based alerts drafted for stuck generation and cache staleness.

### Tech Preview -> GA

- Upgrade and scale testing, including the N->N+1 migration cutover
  and the frozen-old-module coexistence path.
- The new path enabled by default; migration validated across supported
  platforms (including AWS and KubeVirt userdata GC behavior).
- SLO documentation and load testing of the serving tier under provisioning
  bursts.
- Backhaul of SLI telemetry.

### Removing a deprecated feature

- Announce and remove the frozen old token-Secret module and
  `DisableIgnitionServerAnnotation` handling once the oldest supported OCP has
  reached the cutover release.

## Upgrade / Downgrade Strategy

Migration piggybacks on the mandatory N->N+1 OCP upgrade, which already rolls
every worker node, so there is no extra rollout and no adoption shim:

1. **HostedCluster on old CPO (OCP <= N):** the HO sets
   `DisableIgnitionServerAnnotation` on the HostedControlPlane; the old CPO stops
   managing its ignition server component; the HO deploys the ignition server
   with the HO image and uses the `IgnitionPayload` path.
2. **HostedCluster upgraded to new CPO (OCP N+1):** the new CPO has no ignition
   server code — nothing to disable. The HO is already deploying the server on
   the same CRD path. The worker roll performed by the OCP upgrade carries nodes
   onto the new path.
3. **Fleet EOL:** once the oldest supported OCP has reached the cutover release
   (~2-3 releases), the HO removes the frozen old token-Secret module and the
   `DisableIgnitionServerAnnotation` handling.

Because the cutover rides this worker roll — during which every node is replaced
and comes up on the new system from new userdata — the old token Secrets are
never read after a cluster flips and need no compatibility serving path.

No backports are required: old CPOs already recognize
`DisableIgnitionServerAnnotation`, new CPOs simply lack the component, and the HO
handles both. The old module is retained only to drain in-flight rollouts within
the upgrade window and shares no code with the new path.

**Downgrade.** HyperShift does not support downgrading a HostedCluster's control
plane to an earlier OCP version, so there is no supported N+1 -> N rollback of the
ignition path to design for. The migration is therefore forward-only: it rides
the mandatory N->N+1 upgrade, and the frozen old token-Secret module is retained
solely to drain in-flight rollouts within that upgrade window (not to enable a
rollback) before it is deleted at fleet EOL.

## Version Skew Strategy

- **HO/CPO skew is intrinsic to HyperShift** — the HO spans multiple OCP
  versions at once. The redesign turns today's intra-system, four-actor
  entanglement into two independent modules selected by a single branch point
  (`DisableIgnitionServerAnnotation`), the standard way HyperShift ships breaking
  CPO/HO changes.
- **Generation behavior remains release-image-version-branched**, not
  CPO-build-branched, so a single HO copy produces correct payloads for every
  OCP version in the fleet.
- **No kubelet/node coordination.** The payload is version-matched via the
  release image; an n-2 kubelet is unaffected because ignition is delivered at
  boot and the node consumes a fully rendered payload.

## Operational Aspects of API Extensions

- **SLIs for the `IgnitionPayload` CRD.** Health is observable through per-CR
  conditions: `PayloadGenerated` (True/False + error) indicates whether the
  latest config produced a payload; `IgnitionReached` indicates whether a node
  has fetched the current generation — when the PayloadController advances
  `status.current.generation` for a new rollout it resets `IgnitionReached` to
  `False`, and the first server replica to serve the new token flips it back to
  `True`. Fleet-level metrics should expose
  generation latency, store size per NodePool, informer lag on the serving tier,
  and leader failovers.
- **Impact on existing SLIs.** The design *reduces* the load the old system
  placed on the registry and management KAS (in steady state, one release-image
  pull per config version regardless of replica count). Expected CR count is roughly one per
  NodePool plus Karpenter's on-demand CRs per HostedCluster — well within normal
  API throughput; the main amplification (core-config change enqueuing every CR)
  is pre-existing and unchanged.
- **Measurement.** Scale and amplification behavior should be measured in
  HyperShift CI and by the managed-services perf process; name the responsible
  reviewer during review.
- **Failure modes.** (a) Generator down at single replica -> new
  rollouts/scale-up refreshes stall (mitigated by leader election). (b)
  Validation failure -> `PayloadGenerated: False`, no rollout. (c) PayloadStore
  unavailable -> serving tier serves from local cache; new payloads cannot be
  written until it recovers. (d) Finalizer stuck (consumer down) -> CR deletion
  blocked; requires the consumer to recover or manual finalizer removal.
- **Escalation.** The HyperShift team (and, for the trust boundary, the security
  team) are the likely escalation owners; add them as reviewers.

## Support Procedures

- **Detecting failure.** For a stuck rollout, inspect
  `kubectl get ignitionpayload -n <hcp-ns>` and the `PayloadGenerated` /
  `IgnitionReached` conditions; a `PayloadGenerated: False` with an error names
  the source ConfigMap that failed validation (provenance is preserved by
  per-ConfigMap projection). Nodes returning HTTP 511 on `GET /ignition`
  indicate a token miss in the serving cache — check that the token in the
  userdata Secret matches `status.current.token` and that the serving tier's
  cache has hydrated (informer lag metric / ignition-server logs). Generator
  logs (leader) show MCO-pipeline and store-write errors.
- **Disabling the extension.** The `IgnitionPayload` CRD cannot be removed
  without losing the ignition state for all consumers; disabling it would break
  node provisioning for the affected HostedClusters. The supported "off switch"
  is the migration branch point (`DisableIgnitionServerAnnotation`), which
  selects the old vs new path rather than disabling ignition entirely.
- **Graceful recovery.** The serving hot path reads a local cache, so pod
  restarts trigger a cold rehydrate (seconds) with no payload loss or
  regeneration; a leader loss is recovered on lease expiry. Payloads persist in
  the store, so a generator crash loses nothing — only brand-new config versions
  wait for the new leader. Functionality resumes consistently when components
  return.

## Infrastructure Needed [optional]

- A new `IgnitionPayload` API type added to the `openshift/hypershift` API
  packages (no new repository required).
- No new subprojects or external testing infrastructure beyond the existing
  HyperShift e2e/CI environments; managed-service scale testing uses existing
  perf tooling.
