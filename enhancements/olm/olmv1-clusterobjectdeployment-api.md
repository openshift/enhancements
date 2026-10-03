---
title: olm-orchestration-layer-redesign
authors:
  - "@joelanford"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-07-09
last-updated: 2026-07-09
tracking-link:
  - TBD
status: provisional
see-also:
  - "https://github.com/joelanford/orb-operator/blob/main/ADR.md"
  - "https://github.com/openshift/enhancements/pull/2054"
---

# OLMv1 Orchestration Layer Redesign: ClusterObjectDeployment and ClusterObjectSet API Changes

## Summary

This enhancement proposes two changes to the operator-controller project:

1. **Introduce a new ClusterObjectDeployment API and controller** that acts as a mutable orchestration resource between ClusterExtension and ClusterObjectSet, analogous to the relationship between Deployment and ReplicaSet. The ClusterObjectDeployment controller manages template-driven revision stamping, lifecycle transitions, and history pruning.

2. **Redesign the ClusterObjectSet API and controller** to support generic revision chaining via a `group` field, inline per-object assertions (replacing spec-level progressionProbes with selectors), a single `Available` status condition, richer phase-level status reporting, and a `completedAt` timestamp.

Together, these changes decompose the current monolithic responsibilities of the ClusterExtension and ClusterObjectSet controllers into three clean layers: resolution (ClusterExtension), orchestration (ClusterObjectDeployment), and content application (ClusterObjectSet).

## Motivation

The current operator-controller architecture has two layers: ClusterExtension creates and manages ClusterObjectSet resources directly. This conflates several concerns within the ClusterExtension controller:

- **Resolution** (catalog queries, version constraints, channel tracking)
- **Deployment orchestration** (template hashing, revision numbering, creating new revisions, archiving old ones, pruning history)
- **Status aggregation** (mapping COS conditions back to the extension)

The ClusterObjectSet controller is also tightly coupled to the ClusterExtension workflow. It uses labels (`olm.operatorframework.io/owner-name`) and annotations (`olm.operatorframework.io/service-account-name`) rather than generic spec fields to identify revision chains and configure behavior. Its progressionProbe system uses selectors to match probes to objects, which adds complexity relative to colocating assertions with the objects they check.

These design choices limit reusability: other controllers or users cannot easily create and manage ClusterObjectSets independently of the ClusterExtension workflow.

### User Stories

* As an operator-controller developer, I want the ClusterExtension controller to focus exclusively on catalog resolution and intent mapping so that the codebase is easier to reason about, test, and extend.
* As a platform engineer, I want to use ClusterObjectDeployment and ClusterObjectSet directly (without ClusterExtension) to manage phased rollouts of arbitrary Kubernetes resources with readiness gates, so that I can leverage the orchestration layer for non-catalog use cases.
* As a cluster administrator, I want clear, per-phase status reporting on my extension's rollout so that I can quickly identify which phase and which specific objects are blocking progress.
* As an operator author, I want to define readiness assertions directly on each object (including CEL expressions) rather than writing spec-level probes with selectors, so that the relationship between an object and its readiness criteria is immediately obvious.

### Goals

1. Introduce ClusterObjectDeployment as the orchestration layer between ClusterExtension and ClusterObjectSet.
2. Redesign ClusterObjectSet to be independently usable by any controller or user, with revision chain membership expressed through spec fields (`group`, `revision`) rather than labels.
3. Move readiness assertions from spec-level progressionProbes (with selectors) to inline per-object assertions, and add CEL expression support.
4. Simplify COS status to a single `Available` condition with richer phase-level reporting via `observedPhases`. Move the `Progressing` condition up to ClusterObjectDeployment, where rollout progress is tracked across revisions.
5. Maintain the phased rollout, collision protection, and revision transition semantics that exist today.

### Non-Goals

1. This proposal does not change the ClusterExtension API or its resolution behavior. ClusterExtension continues to resolve catalogs and express user intent; it simply targets a ClusterObjectDeployment instead of directly creating ClusterObjectSets.
2. This proposal assumes that the COD and COS controllers operate with cluster-admin permissions, as proposed in [openshift/enhancements#2054](https://github.com/openshift/enhancements/pull/2054). ServiceAccount-scoped permission models for managed object application are out of scope for this proposal.
3. This proposal does not add namespace-scoped variants (ObjectDeployment, ObjectSet). Those can follow once the cluster-scoped APIs are stable.

## Proposal

### Architecture Change

The current two-layer architecture:

```
ClusterExtension ──creates──> ClusterObjectSet (rev 1, 2, 3…)
                                 │
                                 └──manages──> cluster objects
```

Becomes a three-layer architecture:

```
ClusterExtension ──manages──> ClusterObjectDeployment
                                 │
                                 └──stamps out──> ClusterObjectSet (rev 1, 2, 3…)
                                                     │
                                                     └──manages──> cluster objects
```

**ClusterExtension** resolves catalogs and updates the ClusterObjectDeployment template when the resolved bundle changes. It no longer creates ClusterObjectSets, tracks revision numbers, or manages archival.

**ClusterObjectDeployment (COD)** is a new mutable resource analogous to a Kubernetes Deployment. It holds a template for ClusterObjectSets. When the template content changes (detected by SHA-256 hash), the COD controller stamps out a new COS. It manages the lifecycle of its COS resources: archiving predecessors once the latest revision is available, and pruning archived revisions beyond the configured history limit.

**ClusterObjectSet (COS)** is an immutable revision resource analogous to a ReplicaSet. It declares the objects to manage, organized into ordered phases with inline per-object assertions. COSs with the same `group` form a revision chain. The COS controller handles object application via server-side apply, ownership handoffs between revisions in the same group, and teardown on archival.

### Workflow Description

**Cluster administrator** installs an extension by creating a ClusterExtension.

**ClusterExtension controller** resolves the extension from catalogs and creates or updates a ClusterObjectDeployment with the resolved bundle content in its template.

**ClusterObjectDeployment controller** detects the template change, stamps out a new ClusterObjectSet with the next revision number, and reports aggregate availability in its status.

**ClusterObjectSet controller** reconciles the new revision, applying objects phase-by-phase with readiness gates between phases. It coordinates ownership handoffs with sibling revisions in the same group.

**ClusterObjectDeployment controller** observes the new revision become available, archives predecessor revisions, and prunes history beyond the retention limit.

1. The cluster administrator creates a ClusterExtension specifying a package and version constraints.
2. The ClusterExtension controller resolves the package against available ClusterCatalogs and creates a ClusterObjectDeployment whose `template.spec.phases` contain the resolved bundle's objects. Caller-specific metadata (package name, bundle version, image reference) is attached as labels and annotations on `template.metadata`.
3. The ClusterObjectDeployment controller hashes the template, determines the next revision number, and creates a ClusterObjectSet named `{cod-name}-{revision}` with `group` set to the COD name, `revision` set to the computed number, and `lifecycleState: Active`.
4. The ClusterObjectSet controller begins reconciling the new revision. For each phase, it applies all objects via server-side apply and evaluates their inline assertions. It does not advance to the next phase until all objects in the current phase pass their assertions.
5. During the transition, both the old and new COS revisions are active. The COS controller coordinates: objects common to both revisions transfer ownership from the old to the new. Objects present only in the old revision remain under the old revision's ownership.
6. When all phases complete, the COS controller sets `Available=True` and records `completedAt`. The COD controller observes this and patches older active revisions to `lifecycleState: Archived`.
7. Archived COSs tear down any objects still under their exclusive ownership (in reverse phase order), then remove their finalizer.
8. The COD controller deletes archived COSs beyond the `revisionHistoryLimit`.

### API Extensions

#### New API: ClusterObjectDeployment

```go
// ClusterObjectDeployment declares a set of Kubernetes objects that should be
// applied to the cluster and kept in the desired state. The controller creates
// ClusterObjectSet resources to track each unique template snapshot and
// manages their lifecycle automatically.
type ClusterObjectDeployment struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`
    Spec              ClusterObjectDeploymentSpec   `json:"spec"`
    Status            ClusterObjectDeploymentStatus `json:"status,omitempty"`
}

type ClusterObjectDeploymentSpec struct {
    // revisionHistoryLimit is the maximum number of archived
    // ClusterObjectSet resources to retain. When omitted, the platform
    // chooses a reasonable default. Set to 0 to disable revision
    // history entirely.
    RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

    // template defines the ClusterObjectSet that the controller will
    // create whenever the template content changes.
    Template ClusterObjectDeploymentTemplate `json:"template"`
}

type ClusterObjectDeploymentTemplate struct {
    // metadata contains labels and annotations propagated to each
    // ClusterObjectSet created from this template. This is the seam
    // between the resolution layer and the orchestration layer.
    Metadata ClusterObjectDeploymentTemplateMetadata `json:"metadata,omitempty"`

    // spec defines the phases, objects, and collision protection
    // settings for each revision created from this template.
    Spec ClusterObjectDeploymentTemplateSpec `json:"spec"`
}

type ClusterObjectDeploymentTemplateSpec struct {
    // collisionProtection sets the default collision protection for
    // all phases and objects. Default is "Prevent".
    CollisionProtection *CollisionProtection `json:"collisionProtection,omitempty"`

    // phases is the ordered list of phases (1–20). Applied
    // sequentially; the controller does not advance to the next phase
    // until all objects in the current phase satisfy their assertions.
    Phases []Phase `json:"phases"`
}

type ClusterObjectDeploymentStatus struct {
    // conditions: the "Available" condition indicates whether the
    // active revision's managed objects satisfy their assertions.
    // The "Progressing" condition indicates whether a rollout is
    // in progress (e.g. a new revision has been created but has
    // not yet become available, or old revisions are being archived).
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // activeRevisions holds the currently active (non-archived)
    // ClusterObjectSet resources.
    ActiveRevisions []ClusterObjectSetStatusSummary `json:"activeRevisions,omitempty"`
}
```

COD condition logic:

**Available:**
- 0 active revisions: `Status=False, Reason=Unavailable`
- 1 active revision with `Available=True`: `Status=True, Reason=Available`
- 1 active revision without `Available=True`: `Status=False, Reason=Unavailable`
- Multiple active revisions: `Status=Unknown, Reason=Progressing`

**Progressing:**
- A new COS has been created but is not yet available: `Status=True, Reason=RollingOut`
- Multiple active revisions (old revision not yet archived): `Status=True, Reason=RollingOut`
- Archived revisions are being torn down or pruned: `Status=True, Reason=FinalizingRevisions`
- Single active revision is available, no pending work: `Status=False, Reason=Idle`

#### Modified API: ClusterObjectSet

Key changes from the current ClusterObjectSet API:

| Aspect | Current | Proposed |
|---|---|---|
| Revision chain identity | Labels (`olm.operatorframework.io/owner-name`) | Spec field: `group` (label-safe, max 52 chars) |
| Revision number | `int64`, tied to ClusterExtension | `uint32`, generic within a group |
| Readiness checks | Spec-level `progressionProbes` with selectors | Inline `assertions` per object |
| CEL support | Not available | `celExpression` assertion type |
| Status conditions | `Progressing`, `Available`, `Succeeded` (3 conditions) | `Available` (1 condition) |
| Phase status | `observedPhases` with content digests only | `observedPhases` with content digests, `status`, `error`, `incompleteObjects` |
| Completion tracking | Inferred from `Succeeded` condition | Explicit `completedAt` timestamp |

```go
type ClusterObjectSetSpec struct {
    // group is a label-safe identifier linking related revisions.
    // All revisions sharing the same group form an ordered sequence.
    Group string `json:"group"`

    // revision is the monotonically increasing sequence number
    // within the group. The first revision is 1.
    Revision uint32 `json:"revision"`

    // lifecycleState controls whether this revision is actively
    // reconciling. "Active" or "Archived". Cannot un-archive.
    LifecycleState LifecycleState `json:"lifecycleState"`

    // Inline: collisionProtection and phases
    ClusterObjectDeploymentTemplateSpec `json:",inline"`
}

type ClusterObjectSetStatus struct {
    // conditions: the "Available" condition indicates whether all
    // managed objects satisfy their assertions.
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // completedAt is the timestamp when all phases first completed
    // successfully. Set once and never cleared.
    CompletedAt *metav1.Time `json:"completedAt,omitempty"`

    // observedPhases reports the observed state of each phase.
    ObservedPhases []ObservedPhase `json:"observedPhases,omitempty"`
}

type ObservedPhase struct {
    Name   string `json:"name"`
    Status PhaseStatus `json:"status"`

    // digest is the content digest of the phase's resolved objects
    // at first successful resolution, in the format "<algorithm>:<hex>".
    // Immutable once set. Used to detect if referenced Secret content
    // was replaced after the COS was created, enforcing the
    // immutability contract for ref-based object storage.
    Digest string `json:"digest,omitempty"`

    Error             string         `json:"error,omitempty"`
    IncompleteObjects []ObjectStatus `json:"incompleteObjects,omitempty"`
}
```

#### Per-Object Assertions (replacing progressionProbes)

The current `progressionProbes` system defines probes at the spec level with selectors to match objects by GroupKind or label. The proposed design colocates assertions directly with each object:

```go
type PhaseObject struct {
    // Exactly one of object or ref must be set.

    // object is an optional inline Kubernetes resource to manage.
    Object *unstructured.Unstructured `json:"object,omitempty"`

    // ref is an optional reference to a Secret that holds the
    // serialized object manifest. The Secret must be immutable.
    Ref *ObjectSourceRef `json:"ref,omitempty"`

    // collisionProtection overrides the phase/spec-level setting.
    CollisionProtection *CollisionProtection `json:"collisionProtection,omitempty"`

    // assertions define conditions that must be met before this
    // object is considered available. When omitted, the object is
    // available immediately after successful apply.
    Assertions []Assertion `json:"assertions,omitempty"`
}

// Assertion: exactly one of the four fields must be set.
type Assertion struct {
    ConditionEqual *ConditionEqualAssertion `json:"conditionEqual,omitempty"`
    FieldsEqual    *FieldsEqualAssertion    `json:"fieldsEqual,omitempty"`
    FieldValue     *FieldValueAssertion     `json:"fieldValue,omitempty"`
    CELExpression  *CELExpressionAssertion  `json:"celExpression,omitempty"`
}
```

Benefits of inline assertions over spec-level probes with selectors:
- **Locality**: the readiness criteria for an object are defined next to the object, making the relationship immediately obvious.
- **No selector ambiguity**: there is no risk of a probe accidentally matching (or failing to match) objects due to label or GroupKind overlap.
- **Simpler validation**: assertions are validated per-object rather than requiring cross-referencing between probes and phase objects.

#### Collision Protection

Unchanged. The three-level resolution order (object > phase > spec) and the three modes (`Prevent`, `IfNoController`, `None`) remain the same.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The ClusterObjectDeployment and ClusterObjectSet controllers run in the management cluster as part of the operator-controller deployment. They manage cluster-scoped resources in the guest cluster. No unique Hypershift considerations beyond what exists today.

#### Standalone Clusters

Fully relevant. This is the primary deployment topology.

#### Single-node Deployments or MicroShift

The proposal adds a new COD controller within the existing operator-controller binary (no new processes). The COD controller adds negligible overhead — it only reconciles when template content changes and otherwise performs lightweight status aggregation. Overall resource consumption should be comparable to today.

MicroShift does not currently use OLM/operator-controller, so there is no impact.

#### OpenShift Kubernetes Engine

No dependency on features excluded from OKE.

### Implementation Details/Notes/Constraints

**COD controller responsibilities:**
1. Watch ClusterObjectDeployments and owned ClusterObjectSets.
2. Compute a SHA-256 hash of the template. If the hash differs from the latest COS's `template-hash` label, create a new COS.
3. Determine the next revision number: `max(revision across all COSs in the group) + 1`.
4. Adopt orphaned COSs in the group (those with no controller owner) by adding an owner reference.
5. Derive COD status from active (non-archived) COSs.
6. When the latest COS becomes available, archive older active COSs by patching their `lifecycleState` to `Archived`.
7. Delete archived COSs beyond `revisionHistoryLimit`.

**COS controller responsibilities:**
1. Watch ClusterObjectSets and managed objects (via dynamic watches).
2. Resolve object content: for inline objects, use directly; for `ref` entries, read the referenced immutable Secret and deserialize the manifest (auto-detecting gzip compression).
3. For active COSs: apply objects phase-by-phase via server-side apply, evaluate inline assertions, coordinate ownership with sibling revisions in the same group.
4. For archived COSs or COSs being deleted: tear down managed objects in reverse phase order, remove the finalizer when complete.
5. Report per-phase status in `observedPhases` with object-level failure messages.

**Revision chain model:**
A "chain" is defined by the tuple `(group, controllerOwnerRef.name)`. COSs in the same chain share a common group and controller owner. The COS controller uses this to identify sibling revisions for ownership handoffs. COSs with the same group but different controller owners are independent chains (this supports multiple CODs with different names creating COSs in the same group namespace, though in practice COD names its COSs with group = COD name).

**Template metadata as the extension point:**
The COD `template.metadata` field carries labels and annotations from the resolution layer (package name, bundle version, image reference, etc.) through to each COS. The COD and COS APIs never reference catalog, package, or bundle concepts. This keeps the orchestration and content layers decoupled from the resolution layer.

**Dependency on cluster-admin permissions:**
This proposal depends on [openshift/enhancements#2054](https://github.com/openshift/enhancements/pull/2054), which proposes that OLM controllers operate with cluster-admin permissions. The COD and COS controllers apply and manage arbitrary Kubernetes objects on behalf of extensions, so they require broad cluster access. The current ServiceAccount-scoped client model (where each COS carries annotations referencing a specific ServiceAccount) is not carried forward in this design.

**Immutability enforcement:**
COS spec fields (`group`, `revision`, `phases`, `collisionProtection`) are immutable after creation, enforced by CEL validation rules. Only `lifecycleState` may be updated (and only from `Active` to `Archived`). The `completedAt` status field is immutable once set. When objects are stored via `ref`, the referenced Secrets must have `immutable: true` — the COS controller validates this before reconciling and blocks with `Progressing=False, Reason=Blocked` if any referenced Secret is not immutable.

### Risks and Mitigations

**Risk: Migration from the existing Helm-based applier.**
Clusters upgrading from GA (which uses the Helm-based applier) to a version with this feature need a migration path.
*Mitigation:* The existing Helm-to-Boxcutter storage migrator will be updated to create a COD and initial COS from the Helm release state. Since the current Boxcutter/COS feature is behind a feature gate (`BoxcutterRuntime`) and has not reached GA, there is no need to support migration from the old COS format — only migration from the GA Helm-based applier matters.

**Risk: Additional API resource (COD) increases system complexity.**
Adding a new resource type means more controllers, more RBAC, and more objects in etcd.
*Mitigation:* The COD controller is lightweight — it only reconciles when templates change and otherwise performs status aggregation. The tradeoff is justified by the separation of concerns it enables: the ClusterExtension controller becomes simpler, and the orchestration layer becomes independently reusable.

**Risk: Inline assertions increase COS object size.**
Moving assertions from spec-level probes (shared across objects) to per-object inline assertions increases the size of each COS.
*Mitigation:* The maximum COS size is bounded (20 phases × 50 objects × 16 assertions). In practice, most objects have 0–2 assertions. The existing Secret-based `ref` storage mechanism continues to externalize large object manifests out of the COS, keeping COS resources well within etcd size limits even with inline assertions added.

### Drawbacks

The primary drawback is the additional layer of indirection. Debugging a rollout now requires understanding three resources (ClusterExtension → ClusterObjectDeployment → ClusterObjectSet) rather than two. This is the same tradeoff Kubernetes itself makes with Deployment → ReplicaSet → Pod: the additional layer adds complexity but provides clean separation of concerns and enables each layer to evolve independently.

Removing `progressionProbes` means that common readiness checks (e.g., "all Deployments should have `status.updatedReplicas == spec.replicas`") must be specified on each Deployment object individually rather than once at the spec level. In practice, the controller that populates the COD template (ClusterExtension controller) generates these assertions programmatically, so the repetition is an implementation detail, not a user-facing burden.

## Alternatives (Not Implemented)

### Keep the two-layer architecture

The ClusterExtension controller could continue to manage ClusterObjectSets directly. The `group` field and inline assertions could be adopted without introducing ClusterObjectDeployment.

This was rejected because it keeps deployment orchestration concerns (revision stamping, archival, pruning) in the ClusterExtension controller, preventing independent use of the orchestration layer by other controllers or users. It also means any controller that wants phased rollout semantics must reimplement the same orchestration logic.

### Use labels/annotations instead of the `group` spec field

The current COS design uses labels to identify which ClusterExtension owns a COS. This avoids adding a new spec field.

This was rejected because labels are mutable and not suitable for defining a logical grouping that the controller depends on for correctness. The `group` field is immutable and validated, providing a stronger contract. It also makes the COS API self-describing: a reader can understand which revision chain a COS belongs to by reading its spec, without needing to know which labels are semantically significant.

### Keep progressionProbes with selectors

The current selector-based probe system could be extended with CEL support rather than replaced with inline assertions.

This was rejected because selectors introduce indirection: understanding which probes apply to which objects requires mentally cross-referencing the probe selector against the phase objects. Inline assertions eliminate this indirection. The selector approach also makes it harder to validate probes at admission time (e.g., detecting a probe that selects a GroupKind not present in any phase).

## Open Questions

1. What specific changes are needed in the Helm-to-Boxcutter storage migrator to create CODs and COSs from existing Helm releases?
2. Should the COD support rollout strategies beyond "replace" (e.g., canary, blue-green)?
3. Should a purpose-built `ClusterObjectSlice` type eventually replace Secret-based refs, or is the existing Secret storage sufficient long-term?

## Test Plan

- **Unit tests** for the COD controller (template hashing, revision stamping, adoption, archival, pruning, status derivation) and the COS controller (phase progression, assertion evaluation, ownership handoffs, teardown).
- **Integration tests** using envtest to verify end-to-end workflows: initial rollout, upgrade with shared objects, upgrade with removed objects, archival, teardown, collision protection modes.
- **E2E tests** exercising the full ClusterExtension → COD → COS pipeline on a real cluster.
- **Migration tests** verifying that existing Helm-based installations are correctly migrated to COD + COS resources.

## Graduation Criteria

### Dev Preview -> Tech Preview

- ClusterObjectDeployment and redesigned ClusterObjectSet APIs are functional end-to-end.
- Inline assertions (all four types including CEL) work correctly.
- Per-phase status reporting with `observedPhases` provides actionable diagnostics.
- ClusterExtension controller updated to target COD instead of COS directly.
- Migration from existing COS installations is automated and tested.

### Tech Preview -> GA

- API stability: no breaking changes to COD or COS spec.
- Upgrade and downgrade testing.
- Load testing with realistic extension bundle sizes.
- Documentation in openshift-docs.

## Upgrade / Downgrade Strategy

**Upgrade:** The operator-controller manages its own CRDs. On upgrade from GA (Helm-based applier), the new CRDs (ClusterObjectDeployment, ClusterObjectSet) are registered and the existing Helm-to-Boxcutter storage migrator is updated to create a COD and initial COS from each existing Helm release. Migration from the current tech-preview Boxcutter COS format is not required since it has not reached GA.

**Downgrade:** Downgrade to a pre-COD GA version is not automatically supported. The Helm release records from the original migration are not kept up to date as the extension progresses through new versions, so the Helm-based applier cannot simply resume from stale release state. A downgrade would require manual intervention (e.g., uninstalling and reinstalling affected extensions).

## Version Skew Strategy

All three controllers (ClusterExtension, COD, COS) run in the same binary (operator-controller). There is no version skew between them. The CRDs and operator-controller deployment are managed by [cluster-olm-operator](https://github.com/openshift/cluster-olm-operator), which ensures they are updated together during cluster upgrades.

## Operational Aspects of API Extensions

**New CRD: ClusterObjectDeployment**
- Expected instance count: one per ClusterExtension (typically tens, not hundreds, per cluster). Negligible API throughput impact.
- Health indicator: the `Available` condition on each COD. An absent or stale condition indicates the COD controller is not reconciling.
- Failure mode: if the COD controller stops running, no new COS revisions are created on template changes, and archival/pruning stops. Existing active COS revisions continue to reconcile independently.

**Modified CRD: ClusterObjectSet**
- The `group` field replaces label-based chain identification. No change in instance count or API throughput.
- The `observedPhases` status field provides per-phase diagnostics, replacing the opaque content-digest approach with actionable status, error messages, and per-object failure details.
- Failure mode: unchanged. If the COS controller stops running, managed objects remain in their last-applied state.

**Escalation:** The PIXAA Rotational Interrupt Team is responsible for escalations about OLM.

## Support Procedures

**Detecting issues:**
- Check the COD's `Available` condition: `kubectl get clusterobjectdeployments` shows the availability reason in the printed columns.
- Check COS phase status: `kubectl get clusterobjectset <name> -o jsonpath='{.status.observedPhases}'` shows per-phase status with object-level error messages.
- Controller logs: the COD controller logs with field owner `cod-controller`; the COS controller logs with field owner `cos-controller`.

**Common issues:**
- A COS stuck in `Reconciling` with `incompleteObjects`: check the listed objects' messages for assertion failures, collision errors, or apply errors.
- Multiple active revisions (COD shows `Progressing`): the latest COS has not yet become available. Check its `observedPhases` for the blocking phase.
- Archived COS stuck with finalizer: teardown is in progress or blocked. Check the COS's `observedPhases` for `TearingDown` phases with `incompleteObjects`.

## Infrastructure Needed

No new subprojects or repositories are needed. The implementation occurs within the existing `operator-framework/operator-controller` repository.
