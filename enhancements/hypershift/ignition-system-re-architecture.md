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
status: implementable
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
   advances `status.current.rolloutGeneration`, so an invalid config can never trigger
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
   `spec.rolloutConfigMaps` (user/core/NTO) or `spec.managementConfigMaps` (the
   apiserver-HAProxy config). Sources that do not already live in the HCP
   namespace it projects there as CR-owned ConfigMaps — the user configs (copied
   from the clusters namespace), the HAProxy config, and the canonicalized
   rollout-relevant global-config subset it authors and references via
   `spec.rolloutGlobalConfig` — while the core and NTO ConfigMaps already reside
   in the HCP namespace and are referenced in place with no CR owner reference. It
   sets the direct inputs (`releaseImage`, `pullSecret`,
   `additionalTrustBundle`, `osImageStream`). For any rollout-relevant input it keeps in
   `managementConfigMaps` rather than the rendered payload (today, the API-server endpoint
   identity), it writes a canonical summary into `spec.additionalRolloutInputs` so that input still
   rolls existing nodes. It never opens a ConfigMap's contents and computes neither the
   payload nor the rollout hash.
2. The PayloadController (leader) reacts to the CR or to a change in any
   referenced ConfigMap, the pull-secret Secret named by `spec.pullSecret.name`,
   or the trust-bundle ConfigMap named by `spec.additionalTrustBundle` — all of
   which it watches. It reads each referenced ConfigMap, splits it into
   individual manifests, and defaults + validates each. If validation fails it
   sets `PayloadGenerated: False` and stops — no hash, no token, no rollout.
3. Over the validated config it computes the payload-identity hash (`configHash`) and the
   rollout hash. The payload-identity hash covers everything embedded in the
   rendered payload — including the pull-secret and trust-bundle *contents* — so
   an in-place rotation of either produces a new `configHash` and the generator
   never serves stale bytes. The rollout hash instead uses the
   *references* (`rolloutConfigMaps` config + version + `pullSecret` +
   `additionalTrustBundle` name + `rolloutGlobalConfig` config + osImageStream +
   `additionalRolloutInputs`), preserving parity with today's NodePool
   `ConfigGenerator.RolloutHash()` (whose `endpointConfig` term the consumer now supplies
   through `additionalRolloutInputs`, and which hashes
   `pullSecret`/`additionalTrustBundle`, not their contents) so an
   in-place credential/CA rotation does not roll the fleet; instead it refreshes
   the current rollout's bytes under Policy A (step 4), so scale-up/replacement
   nodes boot with current credentials while existing nodes are untouched.
   Before pulling the release image it reads the current `(owner, rolloutHash)`
   store entry; if its recorded `configHash` already matches, it reuses those bytes
   and skips the pull, otherwise it runs the MCO pipeline (which pulls the release image).
4. If the rollout hash changed, it moves `status.current` to `status.previous` —
   deleting from the store any payload already occupying `status.previous`
   (delete-on-evict), so at most two payloads (current + previous) are ever live
   per CR even when rollouts overtake each other (A->B->C) — mints a new signed,
   audience-bound, expiring token, calls `PayloadStore.Put` keyed by the new `(owner, rolloutHash)`, **authors
   the pointer-ignition userdata Secret** (named `<ignitionpayload-name>-<rolloutHash>`,
   embedding the token plus the ignition endpoint and CA — see Node authentication and token
   handling), and sets
   `status.current` with the new hashes, `rolloutGeneration + 1`, and the
   `userDataSecretName`. **The token itself is never written to status.** Otherwise
   (management-side/cloud-config change only) it refreshes the bytes behind the
   *current* token, refreshes that userdata Secret's content in place, and updates only
   `status.current.configHash`. On the first reconcile after the hash *formula* itself
   changes (`status.rolloutHashVersion` older than the binary's constant) it rebaselines
   instead of rolling: it rewrites `status.current.rolloutHash` to the new-formula value
   and updates the version without advancing `rolloutGeneration` (see Version Skew
   Strategy).
5. When `status.current.rolloutGeneration` advances, the NodePool controller points the
   MachineDeployment's bootstrap `DataSecretName` at `status.current.userDataSecretName`;
   CAPI provisions new machines with the new payload. The consumer never mints userdata or
   handles the token — it only references the generator-authored Secret by name.
6. A booting node calls `GET /ignition` with `Authorization: Bearer <token>` (the token it
   was provisioned with in userdata). The ignition-server serves the payload from its
   per-pod local cache; on a cache miss it reads through to `PayloadStore.Get`, serves the
   result, and populates the cache — so there is no window between `PayloadStore.Put` and
   informer propagation in which a valid token returns HTTP 511. On the first successful
   serve of the *current* rollout's payload (the served payload's rollout identity matches
   `status.current.rolloutHash`) it sets `IgnitionReached: True` on the CR via one
   idempotent status update; serving a `status.previous` payload during drain does not flip
   the condition, so a node still booting on the old generation cannot mark the new one
   reached.
7. When a rollout drains (old MachineDeployment scaled to 0), the NodePool controller
   advances `spec.retiredRolloutGeneration`. The PayloadController then frees the old payload
   from the store, deletes the retired userdata Secret, and clears `status.previous` only
   when `status.previous.rolloutGeneration <= spec.retiredRolloutGeneration` — so if rollouts
   overtake each other (A->B->C), an earlier generation's (A) drain signal does not clear a
   still-draining `status.previous` (B), keeping this step consistent with the
   `retiredRolloutGeneration` contract in the CRD spec.
8. On teardown, the creating consumer removes its consumer finalizer and deletes
   the CR; the PayloadController then frees all remaining store entries and removes
   its store-cleanup finalizer, after which the CR-owned objects (the user and HAProxy
   ConfigMaps and the generator-authored userdata Secrets) cascade-delete via their owner
   references — the referenced-in-place core and NTO ConfigMaps carry no owner reference and
   are left untouched.

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
        PC->>PS: Put(owner, rolloutHash, payload)
        PC->>PC: author userdata Secret (name = <np>-rolloutHash, embeds token)
        PC->>CR: status.current = {hashes, gen+1, userDataSecretName}
        NP->>NP: point MachineDeployment at userDataSecretName
    else management/cloud-config change only
        PC->>PS: Put(owner, rolloutHash, refreshed payload)
        PC->>PC: refresh userdata content
        PC->>CR: update status.current.configHash only
    end
    Node->>SRV: GET /ignition (Bearer token)
    SRV->>PS: hydrate local cache
    SRV-->>Node: serve payload
    SRV->>CR: set IgnitionReached=True (one-shot)
    NP->>CR: advance spec.retiredRolloutGeneration after drain
    PC->>PS: Delete(owner, previous rolloutHash); delete previous userdata; clear status.previous
```

**Consumers other than the NodePool controller.** The workflow above is written
for the NodePool controller, but the CRD contract is consumer-agnostic and every
consumer follows the same shape: create the `IgnitionPayload` (adding its own
consumer finalizer), watch `status.current.rolloutGeneration`, and point the machines
it provisions at `status.current.userDataSecretName` (the generator authors that Secret;
consumers never handle the token). The difference is rollout execution. The NodePool
controller runs a *coordinated* rollout (MachineDeployment re-point, drain watch,
`retiredRolloutGeneration`). Karpenter — and external-provider integrations that want a
payload without a NodePool — provision nodes just-in-time and do **not** run a
coordinated rollout: they always point new nodes at the *current* `userDataSecretName`, let
their own node lifecycle replace older nodes, and advance `spec.retiredRolloutGeneration`
as they observe a generation's nodes disappear (or leave it at zero and rely on
delete-on-evict plus the store-cleanup finalizer for reclamation). On teardown
each consumer removes its own consumer finalizer and deletes its CR; the
PayloadController's store-cleanup finalizer then frees the tokens regardless of
which consumer drove the delete. The concrete Karpenter integration is elaborated
in the Karpenter-decoupling effort and is out of scope here beyond this contract.

### API Extensions

This enhancement adds a new CRD and a finalizer:

1. **New CRD: `IgnitionPayload`** (group `hypershift.openshift.io`), namespaced,
   living in the HCP namespace. One resource per consumer request (one per
   NodePool for the NodePool controller; Karpenter creates its own on demand).
   The CRD is **consumer-agnostic** — it carries no back-reference to a NodePool
   so that non-NodePool consumers can use it. Neither payload bytes nor the auth
   token live in status; status references the generator-authored userdata Secret by
   name (`userDataSecretName`), and the token is the key into the `PayloadStore` that
   lives only in that Secret and the store (see Node authentication and token handling).
   The type is defined
   here (v1beta1, group `hypershift.openshift.io`). Note: The API was placed in
   v1beta1 instead of v1alpha1 to align with the existing v1beta1 API group
   convention in HyperShift (see api/AGENTS.md); it is gated behind the
   `IgnitionPayloadSystem` feature gate (TechPreviewNoUpgrade) and the CRD installs
   only under that feature set (see the Feature Gate section).

   ```go
   // IgnitionPayload is one consumer's ignition payload request: the consumer's
   // inputs in spec, the generator's results in status. One resource exists per
   // consumer request (one per NodePool for the NodePool controller; Karpenter
   // creates its own on demand). The type is consumer-agnostic — it carries no
   // back-reference to a NodePool so non-NodePool consumers can use it.
   //
   // +genclient
   // +kubebuilder:object:root=true
   // +kubebuilder:subresource:status
   // +kubebuilder:resource:path=ignitionpayloads,shortName=ignpayload,scope=Namespaced
   // +kubebuilder:printcolumn:name="RolloutGeneration",type=integer,JSONPath=`.status.current.rolloutGeneration`
   // +kubebuilder:printcolumn:name="Generated",type=string,JSONPath=`.status.conditions[?(@.type=="PayloadGenerated")].status`
   // +kubebuilder:printcolumn:name="Reached",type=string,JSONPath=`.status.conditions[?(@.type=="IgnitionReached")].status`
   // +openshift:enable:FeatureGate=IgnitionPayloadSystem
   type IgnitionPayload struct {
   	metav1.TypeMeta `json:",inline"`

   	// metadata is the standard object metadata.
   	// +optional
   	metav1.ObjectMeta `json:"metadata,omitempty"`

   	// spec is written by the consumer and describes the desired payload inputs.
   	// +required
   	Spec IgnitionPayloadSpec `json:"spec,omitzero"`

   	// status is written by the PayloadController and reports generation and
   	// rollout progress.
   	// +optional
   	Status IgnitionPayloadStatus `json:"status,omitzero"`
   }

   // Condition types reported in IgnitionPayloadStatus.Conditions.
   const (
   	// IgnitionPayloadPayloadGeneratedCondition is set by the PayloadController; its
   	// observedGeneration tracks the spec generation last processed, so consumers can
   	// tell whether status.current reflects the latest spec.
   	IgnitionPayloadPayloadGeneratedCondition = "PayloadGenerated"
   	// IgnitionPayloadIgnitionReachedCondition is set by the serving tier when a
   	// booting node has fetched (reached) its ignition payload.
   	IgnitionPayloadIgnitionReachedCondition = "IgnitionReached"
   )

   // IgnitionPayloadSpec is written entirely by the consumer; the PayloadController
   // treats it as read-only input. The consumer declares inputs and classifies
   // config sources — it never opens a ConfigMap's contents and computes no hash.
   //
   // +kubebuilder:validation:XValidation:rule="!has(oldSelf.retiredRolloutGeneration) || (has(self.retiredRolloutGeneration) && self.retiredRolloutGeneration >= oldSelf.retiredRolloutGeneration)",message="retiredRolloutGeneration cannot decrease or be removed"
   // +kubebuilder:validation:XValidation:rule="!has(self.rolloutConfigMaps) || !has(self.managementConfigMaps) || self.rolloutConfigMaps.all(r, !self.managementConfigMaps.exists(m, m.name == r.name))",message="a ConfigMap cannot be listed in both rolloutConfigMaps and managementConfigMaps"
   type IgnitionPayloadSpec struct {
   	// releaseImage is the pullspec of the OCP release whose
   	// machine-config-server binaries render the payload. The PayloadController
   	// resolves it to an immutable digest before pulling it.
   	// +required
   	// +kubebuilder:validation:MinLength=1
   	// +kubebuilder:validation:MaxLength=253
   	// +kubebuilder:validation:XValidation:rule=`self.matches('^(\\w+\\S+)$')`,message="releaseImage must start with a word character (letters, digits, or underscores) and contain no whitespace"
   	ReleaseImage string `json:"releaseImage,omitempty"`

   	// pullSecret references the pull-secret Secret in the IgnitionPayload's namespace
   	// (conventionally named "pull-secret") that the generator reads to fetch the release
   	// image and embed in the payload. This field is required and immutable: change the pull
   	// secret in place (via the global pull secret mechanism) rather than updating this
   	// reference, so it never triggers a node rollout.
   	// +required
   	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="pullSecret is immutable; update the pull secret in place via the global pull secret mechanism"
   	PullSecret SecretReference `json:"pullSecret,omitzero"`

   	// additionalTrustBundle optionally references a ConfigMap in the IgnitionPayload's namespace
   	// holding additional CA certificates (PEM, read from the "ca-bundle.crt" key) for
   	// booting nodes to trust.
   	// +optional
   	AdditionalTrustBundle ConfigMapReference `json:"additionalTrustBundle,omitzero"`

   	// osImageStream selects the RHEL OS stream the payload targets. Valid values are "rhel-9"
   	// (RHEL 9 boot images) and "rhel-10" (RHEL 10 boot images). When omitted, the stream
   	// defaults to the one implied by the release version (rhel-9 for OCP < 5.0, rhel-10 for
   	// OCP >= 5.0). rhel-9 is the legacy stream and is expected to be retired as supported
   	// releases move to RHEL 10.
   	// +optional
   	OSImageStream OSImageStreamReference `json:"osImageStream,omitzero"`

   	// rolloutGlobalConfig references an IgnitionPayload-owned ConfigMap in the
   	// IgnitionPayload's namespace holding the rollout-relevant subset of the hosted
   	// cluster's global configuration, canonicalized and authored by the consumer. The
   	// content is read from the "config" key and feeds the rollout hash; a ConfigMap
   	// missing that key contributes nothing. See predictable-nodepool-rollout-control (#8698).
   	// +optional
   	RolloutGlobalConfig ConfigMapReference `json:"rolloutGlobalConfig,omitzero"`

   	// additionalRolloutInputs is an opaque, consumer-authored summary of rollout-relevant
   	// inputs, folded into the rollout hash only (never into configHash). Changing it advances
   	// rolloutGeneration, so running nodes roll. It is the consumer's lever to roll running
   	// nodes for a change the rollout hash would otherwise miss — typically a managementConfigMaps
   	// change, which on its own only refreshes the payload bytes (updating configHash) for new
   	// nodes without rolling the running ones. Today the NodePool controller writes the API-server
   	// endpoint identity (access mode, advertise address, port), so an endpoint change both
   	// refreshes the payload via the HAProxy config in managementConfigMaps (which updates
   	// configHash) and rolls running nodes via this field. It stays out of configHash so configHash
   	// remains the payload's byte identity — a re-roll onto unchanged bytes does not make the
   	// generator re-pull the release image. Leave empty when no out-of-band rollout trigger applies.
   	// +optional
   	// +kubebuilder:validation:MaxLength=1024
   	// +kubebuilder:validation:XValidation:rule="self.matches('^[!-~]*$')",message="additionalRolloutInputs must contain only printable ASCII characters and no whitespace, so the rollout hash stays deterministic"
   	AdditionalRolloutInputs string `json:"additionalRolloutInputs,omitempty"`

   	// rolloutConfigMaps lists ConfigMaps in the IgnitionPayload's namespace whose contents
   	// are rollout-relevant (user, core, and NTO machine configs). Each referenced ConfigMap
   	// provides its content under the "config" key as multi-document YAML of MachineConfig
   	// manifests; a ConfigMap missing that key contributes nothing. A change to any of them
   	// can advance the rollout hash and trigger a node rollout.
   	// +optional
   	// +listType=map
   	// +listMapKey=name
   	// +kubebuilder:validation:MinItems=1
   	// +kubebuilder:validation:MaxItems=100
   	RolloutConfigMaps []ConfigMapReference `json:"rolloutConfigMaps,omitempty"`

   	// managementConfigMaps lists ConfigMaps in the IgnitionPayload's namespace whose contents are
   	// management-side only (the apiserver-HAProxy config). Each referenced ConfigMap provides
   	// its content under the "config" key as multi-document YAML of MachineConfig manifests;
   	// a ConfigMap missing that key contributes nothing. A change to them refreshes the payload
   	// behind the current token without advancing rolloutGeneration, so it does not trigger a
   	// rollout. A name here must not also appear in rolloutConfigMaps. When a management
   	// ConfigMap nonetheless holds inputs that must roll existing nodes on change (e.g. the
   	// endpoint address/port/access mode inside the apiserver-HAProxy config), the consumer
   	// surfaces just those inputs via spec.additionalRolloutInputs; the ConfigMap itself stays here so
   	// image-only bumps still refresh the payload without rolling.
   	// +optional
   	// +listType=map
   	// +listMapKey=name
   	// +kubebuilder:validation:MinItems=1
   	// +kubebuilder:validation:MaxItems=100
   	ManagementConfigMaps []ConfigMapReference `json:"managementConfigMaps,omitempty"`

   	// retiredRolloutGeneration releases the previous payload early. At most two payloads are
   	// retained — current and previous — and a new rollout always evicts the old previous
   	// regardless of this field. Set it to the highest rolloutGeneration whose nodes have all
   	// been replaced to free previous before the next rollout; the current payload is always
   	// kept. It only ever increases.
   	// +optional
   	// +kubebuilder:validation:Minimum=1
   	RetiredRolloutGeneration int64 `json:"retiredRolloutGeneration,omitempty"`
   }

   // ConfigMapReference references a ConfigMap by name in the IgnitionPayload's namespace.
   type ConfigMapReference struct {
   	// name is the name of a ConfigMap in the same namespace as this resource. It must
   	// be a valid DNS-1123 subdomain.
   	// +required
   	// +kubebuilder:validation:MinLength=1
   	// +kubebuilder:validation:MaxLength=253
   	// +kubebuilder:validation:XValidation:rule="self.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?(\\\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*$')",message="name must consist only of lowercase alphanumeric characters, hyphens, and periods. Each period-separated segment must start and end with an alphanumeric character."
   	Name string `json:"name,omitempty"`
   }

   // IgnitionPayloadStatus has two writers. The PayloadController owns current,
   // previous, and the PayloadGenerated condition, and resets IgnitionReached to False
   // when rolloutGeneration advances; the serving tier sets IgnitionReached to True
   // through a field-scoped, conflict-retried patch once a node fetches the current
   // rollout's payload (see the Status ownership contract in Implementation Details). No writer replaces the
   // whole status. The invariants between current and previous are enforced at admission
   // by CEL rather than only in the controller.
   //
   // +kubebuilder:validation:MinProperties=1
   // +kubebuilder:validation:XValidation:rule="!has(self.previous) || has(self.current)",message="previous requires current"
   // +kubebuilder:validation:XValidation:rule="!has(self.previous) || !has(self.current) || self.previous.rolloutGeneration < self.current.rolloutGeneration",message="previous.rolloutGeneration must be lower than current.rolloutGeneration"
   // +kubebuilder:validation:XValidation:rule="!has(oldSelf.current) || (has(self.current) && self.current.rolloutGeneration >= oldSelf.current.rolloutGeneration)",message="current.rolloutGeneration cannot decrease and current cannot be removed"
   type IgnitionPayloadStatus struct {
   	// conditions reports generation and rollout progress. Known types:
   	// "PayloadGenerated" (the latest config produced a payload; its observedGeneration
   	// tracks the spec generation last processed, so consumers can tell whether
   	// status.current reflects the latest spec) and "IgnitionReached" (False after each new
   	// rollout until a node has fetched the current payload, then True; the PayloadController
   	// resets it on rollout and the serving tier sets it on fetch).
   	// +optional
   	// +listType=map
   	// +listMapKey=type
   	// +kubebuilder:validation:MinItems=1
   	// +kubebuilder:validation:MaxItems=100
   	Conditions []metav1.Condition `json:"conditions,omitempty"`

   	// current describes the payload for the latest validated, generated config.
   	// +optional
   	Current PayloadReference `json:"current,omitzero"`

   	// previous describes the immediately prior payload, retained during a
   	// rollout so in-flight boots on the previous payload are served until they drain.
   	// It is serving/observability state, not the cleanup mechanism: payloads are
   	// reclaimed by delete-on-evict and the store-cleanup finalizer, not by
   	// tracking every generation here.
   	// +optional
   	Previous PayloadReference `json:"previous,omitzero"`

   	// rolloutHashVersion identifies the formula version used to compute
   	// current.rolloutHash. The PayloadController bumps it when it changes which
   	// inputs feed the rollout hash; on that reconcile it rewrites
   	// current.rolloutHash to the new-formula baseline without advancing
   	// current.rolloutGeneration, migrating the fleet in place instead of rolling every
   	// node (see Version Skew Strategy).
   	// +optional
   	// +kubebuilder:validation:Minimum=1
   	RolloutHashVersion int64 `json:"rolloutHashVersion,omitempty"`
   }

   // PayloadReference identifies one generated payload version and its store key.
   type PayloadReference struct {
   	// configHash is the payload-identity hash over the whole validated config.
   	// It also labels the payload's store entry, making generation idempotent
   	// across leader failover.
   	// +required
   	// +kubebuilder:validation:MinLength=1
   	// +kubebuilder:validation:MaxLength=64
   	ConfigHash string `json:"configHash,omitempty"`

   	// rolloutHash is the hash over the rollout-relevant inputs. A change here
   	// advances rolloutGeneration and triggers a node rollout.
   	// +required
   	// +kubebuilder:validation:MinLength=1
   	// +kubebuilder:validation:MaxLength=64
   	RolloutHash string `json:"rolloutHash,omitempty"`

   	// userDataSecretName is the name of the generator-authored pointer-ignition Secret in
   	// the IgnitionPayload's namespace, formatted `<ignitionpayload-name>-<rolloutHash>`. The
   	// consumer points its machines' bootstrap dataSecretName at it; it is a non-secret
   	// reference. The bearer token that fetches the payload lives only inside that Secret and
   	// the PayloadStore, never in status (see Node authentication and token handling).
   	// +required
   	// +kubebuilder:validation:MinLength=1
   	// +kubebuilder:validation:MaxLength=253
   	UserDataSecretName string `json:"userDataSecretName,omitempty"`

   	// rolloutGeneration is a monotonically increasing counter the consumer watches to
   	// execute a rollout. It advances only when rolloutHash changes (a management-only
   	// payload refresh does not advance it); each advance mints a new token, while a refresh
   	// re-issues the token with a fresh expiry under the same rollout identity.
   	// +required
   	// +kubebuilder:validation:Minimum=1
   	RolloutGeneration int64 `json:"rolloutGeneration,omitempty"`
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
   controller, drives its deletion. Once both finalizers clear, the CR's owned objects
   — the projected user/HAProxy ConfigMaps and the generator-authored userdata Secrets,
   all in the HCP namespace with a same-namespace owner reference to the CR — cascade-delete.
   (The userdata Secrets can be CR-owned precisely because the generator authors them in the
   HCP namespace; the NodePool controller, in the management namespace, could not own them
   cross-namespace — one reason generator authorship is cleaner.)

This enhancement also changes the behaviour of the HyperShift NodePool
controller and the ignition server, but it does not modify any CRDs owned by
other teams or any core OpenShift API resources. It does not add admission,
conversion, or mutation webhooks, nor an aggregated API server.

### Feature Gate

The whole re-architecture is gated behind a single **management-cluster** feature
gate, `IgnitionPayloadSystem`, enabled for `TechPreviewNoUpgrade` and **off by
default**. The gate is a HyperShift management
feature gate (configured on the management cluster / HyperShift operator), not a
hosted-cluster feature gate, and it is **fleet-wide**: there is no per-HostedCluster
opt-in. This matches how the HyperShift operator deploys the split
ignition-payload-controller + ignition-payload-server (and proxy) from the operator
image, and how the NodePool controller selects the IgnitionPayload path instead of
the legacy ignition-server — both are all-or-nothing for the management cluster.

The gate controls two things in lockstep:

- **Behavior:** when the gate is off, the operator deploys the legacy ignition
  server and the NodePool controller uses the legacy token-Secret path unchanged;
  when it is on, it deploys the split controller/server and the NodePool controller
  consumes `IgnitionPayload`. Flipping the gate is intended to be a no-op for
  already-running clusters (Policy A — see Version Skew Strategy); it does not by
  itself roll the fleet. The gate is the only switch for the consumer path and the
  new-component deploy; standing down the *legacy* CPO ignition server is a separate,
  per-HostedControlPlane step the HO drives via `DisableIgnitionServerAnnotation` (see
  Upgrade / Downgrade Strategy).
- **CRD installation:** the `IgnitionPayload` CRD is born gated via the
  `+openshift:enable:FeatureGate=IgnitionPayloadSystem` marker and the openshift/api
  CRD feature-set machinery, so it is rendered and installed **only** under the
  `TechPreviewNoUpgrade` feature set (the only feature-set variant the HyperShift
  installer selects). There is no `-Default` variant of the CRD while the feature is in
  Tech Preview.

Promotion to GA flips the gate's default on (and graduates the CRD to the default
feature set), gated on the Tech Preview -> GA criteria below.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is exclusively about HyperShift / Hosted Control Planes; the
ignition system exists only in this topology. It affects components running in
the management cluster (the NodePool controller in the HO) and components
running in the HCP namespace (the PayloadController, ignition-server, and proxy
Deployments). The move of the ignition server from the CPO image to the HO
image changes which binary hosts these components and keeps the *shape* of the
per-HostedCluster serving topology (Route + Service + proxy) the same, though resource
names, the endpoint host, and the node-facing CA change on cutover. The guest cluster is
unaffected except
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

**Component layout (reduced from 4 to 3 — the new PayloadController and
ignition-server run in the HO binary; the HAProxy proxy is unchanged).** The
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
classifies it into `rolloutConfigMaps` vs `managementConfigMaps`. The PayloadController
reads exactly the ConfigMaps named in those lists — a self-describing input set
with no label query — gathers them in a deterministic (sorted) order, splits
multi-document YAML, validates each manifest, and computes both hashes over the
validated output. Per-ConfigMap projection (rather than one concatenated bundle)
keeps every object within the etcd size limit its source already respects and
preserves provenance for validation errors. Core and NTO ConfigMaps already
reside in the HCP namespace and are referenced in place (a mapped `Watches`,
ported from the existing `enqueueNodePoolsForConfig` dispatcher); user and
HAProxy ConfigMaps are CR-owned (a plain `Owns`).

**PayloadStore.** The store maps a **rollout identity** (`owner` + `rolloutHash`) to the
current payload bytes for that rollout; it does **not** store or key on the token. The interface
is `Put(owner, rolloutHash, configHash, payload)` / `Get(owner, rolloutHash)` /
`Delete(owner, rolloutHash)`, plus `ListByOwner(owner)` for cleanup. The signed token the node
presents carries the rollout identity as claims, so the `/ignition` handler validates the
token's signature, audience, and expiry, extracts `(owner, rolloutHash)`, and resolves the
payload by identity — the token is never stored, and re-issuing a fresh-expiry token for the
same rollout resolves the same entry, so periodic re-issue (see Node authentication and token
handling) costs nothing in the store. `(owner, rolloutHash)` also identifies the CR, so the
handler can patch `IgnitionReached` without a separate lookup. Each entry records the
`configHash` of the bytes it holds, for idempotent generation and observability.

v1 is a `SecretBackedStore`: the generator writes each payload (gzip-compressed, to stay well
under the etcd object-size cap) to a Secret in the HCP namespace named by a digest of
`(owner, rolloutHash)` — all non-secret values, so nothing sensitive appears in a resource name
or audit log, and the token (not being the key) never touches the store. The Secret carries
fixed-length digests of `owner` and `configHash` as labels (digested so selectors work past the
63-character label limit), with exact values in annotations for precise post-list matching. The
store uses a direct, **uncached** client so reads always observe the latest write regardless of
informer-cache state (and it performs writes, so a read-only API reader is insufficient).

Server replicas hydrate a per-pod local cache (emptyDir/memory) via an informer; a cache miss
reads through to `PayloadStore.Get`, so a payload is servable as soon as `Put` returns, before
the informer propagates it. On a management-side refresh (Policy A) the generator **overwrites
the current `(owner, rolloutHash)` entry in place** with the new bytes (new `configHash`); the
informer delivers an update and each replica replaces its cached bytes (last-write-wins), so a
scale-up node served by any replica gets current content with no roll. Deletions propagate the
same way: a freed entry converges to HTTP 511 on every replica within informer lag. Because an
entry is deleted only after its rollout has drained (its nodes are gone) or at CR teardown, no
live consumer depends on it during that bounded window. v2 swaps in a `DatabaseBackedStore`
keyed the same way, removing the etcd object-size cap without changing the CRD contract or the
serving hot path.

**Idempotent generation across failover.** Each store entry records the `configHash` of the
bytes it holds. Before pulling the release image, the leader reads the current
`(owner, rolloutHash)` entry and regenerates only when its recorded `configHash` differs from
the target — so a failover that interrupts an in-flight generation costs at most one repeated
pull (the new leader re-pulls only if the crash preceded `PayloadStore.Put`), and the
steady-state guarantee is exactly one pull per config version at any replica count. Entries no
longer named by `status.current`/`status.previous` — e.g. a rollout interrupted between `Put`
and the `status` write, or an overtaken one — are reclaimed at CR teardown, when the
store-cleanup finalizer deletes everything `ListByOwner` returns, and, as defense-in-depth, by
an inline per-CR sweep each reconcile: after resolving `status.current`/`status.previous` the
leader calls `ListByOwner` and deletes any `(owner, rolloutHash)` entry named by neither (nor
the one being generated this pass). This is not the old background `secretsJanitor`: it is
level-triggered, single-active (leader election), and scoped to the one CR already being
reconciled, so it cannot race a concurrent generator. With it, the live per-CR entry count
stays bounded to the two status-referenced rollouts plus at most the one in flight.

**Rollout detection lives with generation.** Because a rollout must not be
triggered for a config that cannot produce a payload, rollout detection is
placed downstream of validation, in the PayloadController. It mints a new token,
authors the userdata Secret, and advances `status.current.rolloutGeneration` only when the
rollout hash changes; the NodePool controller reacts to that generation bump to execute the
rollout (point the MachineDeployment at `status.current.userDataSecretName`, drain watch,
`retiredRolloutGeneration`). Payload refresh follows **Policy A**: the token is a rollout
identity, and management-side/cloud-config changes refresh the bytes behind the current
token (and that userdata Secret's content in place) so scale-up/replacement nodes always
boot with current content while existing nodes are untouched until they roll.

A classification bucket alone cannot express an input that must refresh the payload *and*
roll existing nodes: moving its ConfigMap into `rolloutConfigMaps` would make every change
to it roll (regressing the management-side guarantee for, e.g., HAProxy image bumps),
while leaving it in `managementConfigMaps` never rolls. The apiserver-HAProxy config is
exactly this case — an image bump must not roll, but an endpoint change (Public/Private,
API-server address/port) must, because that config runs on the node. The consumer resolves
this without splitting the ConfigMap: it keeps the whole config in `managementConfigMaps`
(so the payload still carries the current endpoint for new nodes) and writes a canonical
summary of only the roll-worthy inputs into `spec.additionalRolloutInputs`, which feeds the rollout
hash and nothing else. This also keeps endpoint resolution with the consumer, which has the
management-cluster access it needs (the shared-ingress Router Service lives in the
`hypershift-sharedingress` namespace, outside the HCP namespace the PayloadController is
scoped to). `additionalRolloutInputs` is generic: any future "lives in management config but must
still roll" input uses the same lever.

This work depends on and extends
[predictable-nodepool-rollout-control](./predictable-nodepool-rollout-control.md)
(PR #8698), which must merge first: its rollout-vs-management-side hash split
moves into the PayloadController, and the rollout state it holds in annotations
(`nodePoolCurrentRolloutConfig`) and the `Token.isOutdated()` gate migrate into
CRD `status` (`current.rolloutGeneration` / `current.rolloutHash`), so those
annotation/token mechanisms are not carried over.

**Status ownership.** `IgnitionPayloadStatus` has two writers rather than a single
owner. The PayloadController writes `status.current`, `status.previous`, and the
`PayloadGenerated` condition, and resets `IgnitionReached` to `False` when
`rolloutGeneration` advances. The serving tier sets `IgnitionReached` to `True`, and
does so with a field-scoped patch — Server-Side Apply under its own field manager (or a
JSON-patch targeting just that condition entry) — never a full-status replace, so
it can never clobber `current`/`previous`. That write is conditional and
conflict-retried: a replica sets `IgnitionReached: True` only while
`status.current.rolloutHash` still matches the rollout identity of the payload it served,
re-checking under optimistic concurrency and dropping the update if a retry observes the
generation has advanced. Field-scoping plus that precondition together mean concurrent
writes can neither lose the PayloadController's rollout reset nor overwrite
unrelated status fields, so no separate reporting resource or single-owner funnel
is needed.

**Node authentication and token handling.** The token is a bearer capability: whoever
holds it can `GET /ignition` and fetch a payload that embeds the pull secret and node
bootstrap credentials. Two properties contain that risk.

*Where it lives.* The token lives **only** in two Secrets in the HCP namespace — the
generator-authored userdata Secret (baked into the node at provisioning) and the PayloadStore
entry — and is **never written to `IgnitionPayload.status`**. Status carries only non-secret
identity (`configHash`, `rolloutHash`, `rolloutGeneration`) plus the `userDataSecretName`
reference. This is a deliberate least-privilege boundary: a principal that can read the CR but
not Secrets — e.g. the `hypershift-readers` ClusterRole, which grants get/list/watch on
`hypershift.openshift.io` resources but not Secrets — cannot obtain a token, and OpenShift's
`WriteRequestBodies`/`AllRequestBodies` audit profiles (which log custom-resource bodies but
keep Secrets at Metadata level) never capture it. The actors that legitimately need the token
already have HCP-namespace Secret access: the PayloadController (mints it, writes the userdata
Secret and the store entry), the NodePool controller and other consumers (reference
`userDataSecretName`, and for the InPlace compatibility write resolve the bytes via
`PayloadStore.Get(owner, status.current.rolloutHash)`), and the ignition-server (resolves a
presented token to a payload). RBAC grants those Roles only in the HCP namespace; guest-cluster
and tenant identities have no access to the CR or its backing Secrets.

*What it is.* The token is not an opaque random string but a **generator-signed token carrying
the rollout identity (`owner` + `rolloutHash`), an audience, and an expiry** (e.g. a JWT). The
serving tier holds only the public verification key, so it **validates the signature, audience,
and expiry statelessly** — rejecting forgeries and expired tokens before any lookup — then
resolves the payload by that rollout identity (the PayloadStore is keyed by it, not by the
token; see PayloadStore). This bounds a leaked token's blast radius (it expires and is scoped to
this serving tier) and lets the stateless, horizontally-scaled servers authenticate without a
shared secret.

*Keeping it fresh.* CAPI bakes the userdata token into an instance when it **creates** the
instance, which can be long after the rollout (e.g. an autoscaler scale-up reads the rollout's
userdata Secret hours later), so the token in the current — and draining-`previous` — userdata
Secret must stay valid for a *future* instance. The generator therefore **re-issues the token
with a fresh expiry on a timer** (well before expiry), and also on any payload refresh, rewriting
it **in place** into the same userdata Secret with the rollout identity (`rolloutHash`, hence
`userDataSecretName`) unchanged — so re-issue never triggers a roll and, because the store is
keyed by rollout identity, costs nothing in the store (every re-issued token resolves the same
entry). Already-booted nodes never re-fetch, so an expired token after boot is harmless — only
the *userdata* must stay fresh for not-yet-created/not-yet-booted instances. Two first-boot
realities size the parameters: clocks may be wrong before NTP (validation allows generous skew /
`nbf`), and the instance-creation→boot gap plus the re-issue cadence must fit comfortably within
the token TTL.

The standalone HAProxy Deployment remains the only externally reachable hop (see Risks). Moving
beyond a bearer token entirely — to possession-proof mTLS — is future work (see Alternatives);
it is blocked today by Ignition's static-header-only first-boot fetch.

**Legacy in-place-upgrade compatibility.** The HCCO `InPlaceUpgrader` reads a
Secret named `token-{machineSetName}-{targetConfigVersion}`, where
`targetConfigVersion` is the payload-identity hash (`status.current.configHash`)
stamped on the matching MachineSet annotation. The **NodePool controller** owns this
compatibility write — it is inherently machineSet/upgrade-type scoped (CAPI concepts absent
from the consumer-agnostic CR), so making the PayloadController the writer would force
MachineSet names and upgrade type into the CRD spec. It holds payload *bytes* (not the
token), which the consumer resolves from the store via
`PayloadStore.Get(owner, status.current.rolloutHash)` — needing
HCP-namespace Secret access, which the NodePool controller has and CR-only readers do not, so
this path does not re-introduce the token into any readable place. In the same reconcile that
reacts to a `status.current.rolloutGeneration` advance, the NodePool controller writes the
byte-compatible legacy Secret and stamps the MachineSet annotation *before* re-pointing the
MachineDeployment, so HCCO never observes the annotation without its Secret. The step is
level-triggered and idempotent: if either write fails the reconcile requeues and does not
re-point until the legacy Secret and the annotation both exist. No HCCO change is required —
the guest side keeps reading the same Secret shape it reads today.

**Feature gating.** The whole re-architecture is gated behind the management-cluster
`IgnitionPayloadSystem` feature gate (fleet-wide, enabled for TechPreviewNoUpgrade,
off by default) — see the [Feature Gate](#feature-gate) section. It
is a HyperShift management feature gate (configured on the HyperShift operator), not
an openshift/api guest feature set, and the `IgnitionPayload` CRD is born gated behind
the same gate via the openshift/api CRD feature-set machinery, so it installs only
under TechPreviewNoUpgrade. The old token-Secret path is retained as
a gate-selected path through the coexistence window (coexisting with the new path behind
the gate rather than as a fully independent module), and the migration model
(see [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)) rides the mandatory
N->N+1 upgrade. Promotion to GA flips the gate's default on and graduates the CRD to
the default feature set.

### Risks and Mitigations

- **v1 serving tier is not KAS-free.** With `SecretBackedStore`, server
  replicas need management-cluster KAS access to run the Secret informer and to
  write `IgnitionReached`. *Mitigation:* the serving tier stays behind the
  standalone HAProxy proxy Deployment — the only externally reachable hop, kept
  off KAS-connected nodes. Reviewers of this enhancement should confirm this
  boundary holds. v2 removes the payload-read KAS dependency and can move
  `IgnitionReached` off the hot path, after which the proxy can be retired with
  security sign-off.
- **Cross-tenant data boundary.** The ignition system never reaches across
  namespaces to pull in resources: the `IgnitionPayload` CR and the PayloadController
  only ever read resources in their own HCP namespace (`pullSecret`,
  `additionalTrustBundle`, and every `ConfigMapReference` resolve in-namespace), so no
  privileged controller copies a resource across namespaces on a caller's behalf.
  Making every input available in the HCP namespace is the **consumer's**
  responsibility — the NodePool controller copies the user config ConfigMaps from the
  clusters namespace into the HCP namespace. That copy moves the same data that already
  crosses the boundary today (as `mcoRawConfig` inside the token Secret) one step
  earlier; it does not introduce a new class of data to the boundary.
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
  retained legacy path (gate-branched, not fully isolated) that must keep compiling
  until EOL.

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
- **Auth token in CRD status.** An earlier revision published the per-rollout token in
  `status.current/previous.token` so consumers could read it directly. Rejected: the token
  is a bearer capability for a pull-secret-bearing payload, and CRD status is readable by
  principals that cannot read Secrets (e.g. `hypershift-readers`) and is captured by
  custom-resource audit profiles — a privilege-escalation leak. The generator now authors
  the userdata Secret and status carries only a non-secret `userDataSecretName`; the token
  lives only in Secrets (see Node authentication and token handling).
- **Possession-proof node authentication (mTLS) — future work.** The design authenticates
  nodes with a signed, audience-bound, expiring bearer token (see Node authentication and
  token handling); the stronger end state is possession-proof auth where only public material
  lives in the CR/userdata and the credential is non-replayable, keeping the serving tier
  stateless (verify against a static CA) and platform-portable. It is **blocked by Ignition
  today**: the first-boot fetch can present only static headers, not a client certificate or a
  computed/attested proof (coreos/ignition#1870). Unblocking needs either driving that upstream
  or a thin **first-boot fetch agent** (a minimal baked Ignition stub that runs an agent which
  performs the authenticated fetch and hands the payload to Ignition) — which also enables the
  **two-step bootstrap** every strong system uses (bake a low-value, short-lived credential;
  release the secret-grade payload only after a possession/identity check). Instance-identity
  attestation (AWS IID / Azure IMDS) was rejected as a primary mechanism: it is not expressible
  in static Ignition, not portable (None/agent/bare-metal have no metadata authority), and
  proves "a legit instance," not "the authorized node."

## Open Questions [optional]

1. **Policy A vs Policy B exposure.** Should payload-refresh behavior be a
   per-NodePool API field (e.g. `spec.management.payloadRefresh:
   Immediate | OnRollout`), or is fleet-wide Policy A sufficient indefinitely?
2. **v2 PayloadStore backend.** Which database backing (and its HA/backup story)
   should `DatabaseBackedStore` use, and when does v2 land relative to v1?
3. **Proxy retirement in v2.** Under what exact conditions does security sign off
   on retiring the HAProxy proxy Deployment once the serving tier is KAS-free?
4. **In-flight instances at cutover.** An instance whose userdata (endpoint, CA, token)
   was baked by the legacy server just before cutover, but that first boots just after the
   legacy server is stood down, would fetch a now-dead endpoint with the wrong node-facing
   CA. Should the legacy ignition server/Route keep serving through a bounded drain window
   after cutover, or are such pre-cutover in-flight instances expected to fail and be
   replaced? (See [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy).)

## Test Plan

<!-- TODO: Complete once targeted at a release.
Per dev-guide/feature-zero-to-hero.md and dev-guide/test-conventions.md, tests
must be appropriately labeled. Because this is gated by the HyperShift
management-side `IgnitionPayloadSystem` feature gate (not an openshift/api guest
feature gate), the standard `[OCPFeatureGate:FeatureName]` label does not apply;
tests instead run in the HyperShift e2e suite with the gate enabled. Include:
- `[Jira:"HyperShift"]` (or the appropriate component) so regressions are routed.
- Suite/behavior labels as needed: `[Serial]`, `[Slow]`, `[Disruptive]`.
Fill in the concrete cases below. -->

The general strategy:

- **Unit tests** for the PayloadController: config split + validation +
  deterministic gather order, both hashes, rollout detection (rollout-relevant
  vs management-side change), and the delete-on-evict path for rollouts that
  overtake each other (A->B->C within one rollout window).
- **Unit/integration tests** for the NodePool controller's reduced role: config
  projection, ref-list classification, `retiredRolloutGeneration` advancement,
  finalizer-driven CR delete and cascade of owned ConfigMaps.
- **Integration tests** for the `PayloadStore` (SecretBackedStore + informer
  hydration; cold rehydrate on restart; a management-side same-token refresh is
  observed through *every* ignition-server replica — last-write-wins cache
  replacement plus read-through on miss, so no replica serves stale bytes) and
  the two-Deployment leader-election split (exactly one release-image pull per
  config version at any replica count).
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
cut. This change is gated by the management-cluster `IgnitionPayloadSystem` feature
gate (see "Feature Gate" above), so graduation follows HyperShift's own conventions
and CI signals; document the concrete platform matrix and signal thresholds here when
targeting a release. -->

### Dev Preview -> Tech Preview

- The `IgnitionPayloadSystem` feature gate exists and is enabled for
  `TechPreviewNoUpgrade` (off by default); the `IgnitionPayload`
  CRD installs only under that feature set.
- Ability to use the new ignition path end to end on at least one platform.
- `PayloadGenerated` and `IgnitionReached` conditions and the
  `kubectl get ignitionpayload` view implemented.
- Sufficient unit/integration coverage; SLIs enumerated and exposed as metrics
  (generation latency, store size per NodePool, informer lag, leader failovers).
- Symptoms-based alerts drafted for stuck generation and cache staleness.

### Tech Preview -> GA

- Upgrade and scale testing, including the N->N+1 migration cutover
  and the frozen-old-module coexistence path.
- The `IgnitionPayloadSystem` gate's default flipped on and the `IgnitionPayload`
  CRD graduated to the default feature set; migration validated across supported
  platforms (including AWS and KubeVirt userdata GC behavior).
- SLO documentation and load testing of the serving tier under provisioning
  bursts.
- Backhaul of SLI telemetry.

### Removing a deprecated feature

- Announce and remove the frozen old token-Secret module and the HO's
  legacy-standdown use of `DisableIgnitionServerAnnotation` once the oldest supported
  OCP has reached the cutover release. The user-facing disable-ignition semantics of
  the annotation remain (now disabling the new stack).

## Upgrade / Downgrade Strategy

Migration is driven by the management-cluster `IgnitionPayloadSystem` feature gate
(fleet-wide, default off — see Feature Gate), **not** by per-HostedCluster OCP upgrades.
Enabling the gate on a management cluster must be a no-op for already-running nodes, so the
design makes the flip fleet-safe through adoption rather than relying on a worker roll.

**Gate off (default).** The legacy ignition path is unchanged, byte-for-byte: the CPO
ignition server runs and the NodePool controller uses the legacy token-Secret path. The new
components are not deployed and the `IgnitionPayload` CRD is not installed.

**Gate on.** When the gate is on *and* the user has not disabled ignition on the
HostedCluster, the HO deploys the generator, server, and proxy from the HO image, and the
NodePool controller — now on the gate-selected consumer path — creates an `IgnitionPayload`
CR (and projects the config ConfigMaps) for every NodePool. The **gate**, not any annotation,
selects the new consumer path and the new-component deploy; `DisableIgnitionServerAnnotation`
does one narrower job, standing down the *legacy* CPO ignition server and proxy per
HostedControlPlane. Because the HO must own that standdown signal, the annotation is **removed
from the HC->HCP annotation mirror** and written directly on the HostedControlPlane by the HO,
which reconciles it to `(user disabled ignition on the HostedCluster) OR (this
HostedControlPlane has cut over)` in both gate states — preserving today's user-driven disable
when the gate is off. The new components never key off this HCP annotation (if they did, the
HO's own standdown would tear them down); they are gated on the HO deploy decision above.

Each HostedControlPlane then cuts over independently, forward-only:

1. **Pre-cutover.** The CR exists and the generator produces payloads, but CAPI still points
   at the legacy userdata and the legacy CPO server still serves — nothing has moved.
2. **Adoption (no roll) — before standdown.** Once the new `ignition-payload-server` reports
   Available, the HO re-points `hc.Status.IgnitionEndpoint` at the new proxy Route (with the
   new node-facing CA). For each already-provisioned NodePool the consumer does **not** re-point
   CAPI: it reads the live `Bootstrap.DataSecretName`, keeps CAPI on that existing userdata
   Secret, and refreshes its *content* from the generator's canonical userdata (new endpoint,
   CA, token) in place — so `DataSecretName` is unchanged and **no roll occurs**. Existing nodes
   (which never re-read userdata) are untouched; scale-up nodes get current content, and the
   legacy server stays up as a safety net for a scale-up whose pool has not been adopted yet.
   For InPlace NodePools, adoption must **not** re-stamp the MachineSet `targetConfigVersion`,
   or HCCO would reboot the pool's nodes. The pool is marked adopted; the next genuine config
   change rolls it onto the canonical `<ignitionpayload-name>-<rolloutHash>` name.
3. **Legacy standdown.** Only after every pool on the HostedControlPlane has been adopted does
   the HO set `DisableIgnitionServerAnnotation` on the HostedControlPlane, standing down the
   legacy CPO ignition server and proxy. Ordering matters: standing down before adoption would
   leave a scale-up in the gap booting stale userdata (old endpoint/CA) against a server that
   is already gone. (Instances already *created* by CAPI but not yet booted at cutover are the
   subject of the in-flight open question.)
4. **Fleet EOL.** Once the oldest supported OCP has reached the cutover release (~2-3 releases),
   the HO removes the frozen legacy token-Secret module and its direct
   `DisableIgnitionServerAnnotation` handling.

**Other consumers at cutover.** Karpenter takes its ignition endpoint from
`hc.Status.IgnitionEndpoint`, so the re-point reaches new just-in-time nodes automatically; it
runs no coordinated adoption (it has no stable `DataSecretName` to refresh), so existing
Karpenter nodes are replaced by its own lifecycle and only in-flight instances fall under the
open question. The Agent platform carries both the ignition endpoint and the node-facing CA in
`AgentCluster.Spec.IgnitionEndpoint`; cutover updates both, already-booted agents are untouched,
and new agents fetch from the new endpoint/CA. NodePools with no automated machine management
(Platform None, IBM UPI) get a CR served by the new server but are skipped for CAPI/adoption
(they have no Machines).

The OCP N->N+1 upgrade is orthogonal: it rolls nodes as it always has, and rolled nodes simply
boot from the new path once their HostedControlPlane has cut over. The legacy token-Secret
module is retained — gate-selected and coexisting with the new path (their shared
NodePool-controller wiring is branched on the gate, not fully isolated) — only to drain
in-flight rollouts during the coexistence window.

**Downgrade.** The `IgnitionPayloadSystem` gate is a management-cluster TechPreview gate and
cutover is forward-only (the HO owns the HCP `DisableIgnitionServerAnnotation` and keeps it set
once a HostedControlPlane has cut over), so disabling the gate after a HostedControlPlane has
cut over is not a supported rollback. HyperShift
likewise does not support downgrading a HostedCluster's control plane to an earlier OCP
version. The migration is therefore forward-only; the frozen legacy module exists solely to
drain in-flight rollouts during the coexistence window (not to enable a rollback) before it is
deleted at fleet EOL.

## Version Skew Strategy

- **HO/CPO skew is intrinsic to HyperShift** — the HO spans multiple OCP
  versions at once. The redesign turns today's intra-system, four-actor
  entanglement into two independent modules: the fleet-wide `IgnitionPayloadSystem`
  gate selects the HO-side consumer path and new-component deploy, while the
  per-HostedControlPlane `DisableIgnitionServerAnnotation` (HO-owned, no longer mirrored
  from the HostedCluster) stands down the legacy CPO server once a HostedControlPlane has
  cut over — the standard way HyperShift ships breaking CPO/HO changes.
- **Generation behavior remains release-image-version-branched**, not
  CPO-build-branched, so a single HO copy produces correct payloads for every
  OCP version in the fleet.
- **No kubelet/node coordination.** The payload is version-matched via the
  release image; an n-2 kubelet is unaffected because ignition is delivered at
  boot and the node consumes a fully rendered payload.
- **Rollout-hash formula changes migrate in place, without rolling nodes.** The
  inputs feeding the rollout hash may need to evolve over time (for example,
  moving the pull secret from name-based to content-based hashing). Because the HO
  spans OCP versions, a naive formula change would make every existing CR's
  recomputed `rolloutHash` differ from its stored value on the first reconcile
  after the HO upgrade — advancing `rolloutGeneration` and rolling the entire fleet. To
  prevent this, the PayloadController carries the current formula as a hardcoded
  constant, bumped by hand in the same change that alters the rollout-hash inputs,
  and records it in `status.rolloutHashVersion` alongside the hash it produced.
  When the stored version is older than the binary's constant, the controller
  *unconditionally rebaselines*: it rewrites `status.current.rolloutHash` to the
  new-formula value over the current inputs, sets `rolloutHashVersion` to the
  constant, and does **not** advance `rolloutGeneration` — so the version bump itself
  never rolls. This is a single-controller migration: because the PayloadController
  is the sole owner of both the rollout hash and the `rolloutGeneration` counter, the
  rebaseline is one controller's decision rather than a per-NodePool reconciliation.
  - **Accepted limitation of unconditional rebaseline.** A config change that is
    visible to *both* the old and new formula, and that lands in the narrow window
    between the outgoing leader's last status write and the new leader's first
    reconcile, is absorbed into the rebaselined value without rolling; it rolls on
    the next change instead. Outside a formula bump this race is self-correcting —
    the stored hash is still pre-change, so the restarted controller detects the
    delta and rolls — but the rebaseline overwrites that pre-change baseline, which
    defeats the catch-up. Distinguishing this case from a pre-existing drift that
    must *not* retroactively roll (the name→content scenario above) would require
    the controller to recompute the stored hash under its old formula, and thus to
    retain every historical formula implementation indefinitely. Given the window
    is a single controller failover (seconds) and the change still rolls on the
    next edit, this is accepted rather than paying that cost.

## Operational Aspects of API Extensions

- **SLIs for the `IgnitionPayload` CRD.** Health is observable through per-CR
  conditions: `PayloadGenerated` (True/False + error) indicates whether the
  latest config produced a payload; `IgnitionReached` indicates whether a node
  has fetched the current generation — when the PayloadController advances
  `status.current.rolloutGeneration` for a new rollout it resets `IgnitionReached` to
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
  indicate a token miss in the serving cache — check that the MachineDeployment's bootstrap
  Secret matches `status.current.userDataSecretName`, that the token baked into that userdata
  Secret resolves to a live PayloadStore entry, and that the serving tier's
  cache has hydrated (informer lag metric / ignition-server logs). Generator
  logs (leader) show MCO-pipeline and store-write errors.
- **Disabling the extension.** The `IgnitionPayload` CRD cannot be removed
  without losing the ignition state for all consumers, and the management gate is
  fleet-wide (disabling it after a HostedControlPlane has cut over is unsupported —
  see Upgrade / Downgrade Strategy). `DisableIgnitionServerAnnotation` is **not** an
  off switch for the new system: it only stands down the legacy CPO server. The only
  way to run a HostedCluster with no ignition at all — as today — is the user-set
  HostedCluster `DisableIgnitionServerAnnotation`, which the HO honors by disabling
  both stacks; that stops node provisioning for that cluster and is not a fallback to
  the legacy path.
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
