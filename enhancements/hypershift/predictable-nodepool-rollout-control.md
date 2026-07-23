---
title: predictable-nodepool-rollout-control
authors:
  - "@csrwng"
reviewers:
  - "@enxebre, HyperShift expertise"
  - "@muraee, HyperShift expertise"
  - "@joshbranham, managed services expertise"
  - "@typeid, managed services expertise"
approvers:
  - "@enxebre"
api-approvers:
  - None
creation-date: 2026-06-15
last-updated: 2026-06-16
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-3298
status: provisional
see-also:
  - "/enhancements/hypershift/node-lifecycle.md"
replaces:
superseded-by:
---

# Predictable NodePool Rollout Control for Hosted Control Planes

## Summary

Today, the HyperShift NodePool controller computes a single hash over the
entire rendered node ignition configuration — including management-side image
references such as HAProxy — to decide when to trigger a Replace or InPlace
rollout of worker nodes. Any change to any input, no matter how minor, produces
a new hash and triggers a full rolling replacement of every node in the
NodePool. This has caused production incidents where automated image-updater PRs
or registry-override changes triggered unexpected fleet-wide node rollouts.

This enhancement decouples the rollout-triggering hash from management-side
implementation details by introducing a *rollout hash* derived only from
customer-facing spec inputs. Management-side changes do not trigger rollouts,
and both existing and scale-up nodes retain the current configuration until the
next spec-driven rollout.

This is a deliberate relaxation of the current behavior, not a bug fix: the
omitted inputs (HAProxy image, registry overrides, computed config defaults) do
influence the ignition payload content, and excluding them from the rollout hash
means existing and scale-up nodes will run with stale management-side
configuration until the next spec-driven rollout. This trade-off is acceptable
because the previous management-side configuration remains functional — existing
nodes continue to operate correctly — and the operational cost of unexpected
fleet-wide node replacements far outweighs the benefit of immediate propagation
of non-customer-facing changes.

## Motivation

### User Stories

* As a managed service operator (ROSA HCP, ARO HCP), I want NodePool worker
  node rollouts to be triggered only by explicit customer-facing spec changes
  (release image upgrades, proxy configuration, user-provided MachineConfigs),
  so that automated management-plane image bumps do not cause unplanned downtime
  for my tenants.

* As a self-managed HyperShift administrator, I want visibility into whether a
  pending configuration change will trigger a node rollout or is a non-disruptive
  management-side update, so that I can plan maintenance windows accordingly.

* As a service reliability engineer, I want the HyperShift operator upgrade to
  be safe for existing NodePools — it must not trigger rollouts when the new
  operator version is deployed, so that I can upgrade the control plane without
  disrupting hosted cluster workloads.

* As an SRE monitoring fleet health, I want conditions on the NodePool that
  distinguish between customer-initiated rollouts and service-provider-initiated
  configuration changes, so that I can attribute node replacements to the correct
  change category.

### Goals

1. Platform-owned configuration changes MUST NOT trigger worker node Replace
   rollouts. Today, the following platform-owned inputs can cause unintended
   rollouts:
   - **HAProxy data plane image**: For shared ingress clusters (ROSA HCP, ARO
     HCP), the kube-apiserver-proxy static pod image comes from the operator's
     `IMAGE_SHARED_INGRESS_HAPROXY` env var rather than the NodePool's release
     payload. This is a historical artifact from early shared ingress
     bootstrapping — there is no reason the image cannot come from the payload
     now. For non-shared-ingress (self-hosted) clusters, the image already comes
     from the NodePool's release payload (`haproxy-router` component).
   - **Registry overrides applied to the HAProxy image**: The
     `--registry-overrides` flag on the management cluster rewrites the image
     reference embedded in the ignition payload, even though data plane CRI-O
     handles mirroring natively
     ([OCPBUGS-86415](https://issues.redhat.com/browse/OCPBUGS-86415)).
   - **`config.openshift.io` computed defaults**: The `globalConfigString()`
     function reconciles proxy and image configs with platform-specific defaults
     (e.g., `Status.NoProxy` entries like network CIDRs, `169.254.169.254` for
     AWS). If operator code changes these defaults, the serialized config changes
     and triggers a rollout even though the user's spec did not change.
2. Customer-facing spec changes (release upgrades, proxy config, user
   MachineConfigs, pull secret, trust bundle) MUST continue to trigger rollouts
   as they do today.
3. The feature MUST be deployable without causing rollouts on existing clusters
   (safe upgrade path).
4. Operators SHOULD have visibility into what changes are pending vs. applied on
   nodes via NodePool status conditions.

### Non-Goals

1. **In-place config refresh for existing nodes.** When management-side
   configuration changes are excluded from triggering a Replace rollout, existing
   worker nodes retain their previous configuration until the next spec-driven
   rollout. Delivering non-disruptive configuration changes to existing nodes
   without node replacement is tracked separately as
   [OCPSTRAT-3299](https://issues.redhat.com/browse/OCPSTRAT-3299).
2. **Changing the ignition payload content.** When a spec-driven rollout occurs,
   the full rendered ignition configuration — including management-side images —
   is used for payload generation. Only the *rollout decision* and *secret
   lifecycle* change.

## Proposal

Introduce a second hash function (`RolloutHash`) on the `ConfigGenerator` that
hashes only spec-driven inputs, excluding management-side content such as the
HAProxy static pod manifest. The rollout decision in both the Replace path
(`propagateVersionAndTemplate`) and InPlace path (`reconcileMachineSet`) switches
from comparing user-data secret names (which embed the full hash) to comparing
rollout hashes tracked via a new NodePool annotation. A new `ConfigUpdatePending`
condition provides visibility into management-side changes that are queued but
not yet applied to existing nodes.

### Workflow Description

**service provider** is a human or automated system responsible for operating
the management cluster and upgrading the HyperShift operator.

**cluster administrator** is a human user responsible for managing a hosted
cluster's NodePool configuration.

#### Normal operation (no rollout)

1. An automated image-updater bumps the HAProxy image digest in the HyperShift
   operator deployment.
2. The operator reconciles all NodePools. The `RolloutHashWithoutVersion()` does
   NOT change, because HAProxy content is excluded from the rollout hash.
3. Because the rollout hash is unchanged, no new token or user-data secrets are
   created. The existing secrets remain valid, and the MachineDeployment
   continues to reference them.
4. The `ConfigUpdatePending` condition transitions to `True` with reason
   `ManagementConfigDrift`, indicating that the management-side configuration
   has changed but no rollout has been triggered.
5. No MachineDeployment or MachineSet spec change occurs. Existing nodes remain
   undisturbed. Scale-up continues to work using the existing user-data secret,
   and new nodes receive the same configuration as existing nodes.

#### Spec-driven rollout

1. The cluster administrator updates `NodePool.spec.config` to add a
   MachineConfig (e.g., a custom chrony configuration).
2. The rollout hash changes. The controller creates new token and user-data
   secrets with the updated ignition payload. The previous token secret receives
   an expiration timestamp (2-hour TTL), and the previous user-data secret is
   deleted (the MachineDeployment will be updated to reference the new one).
3. The controller detects that `RolloutHashWithoutVersion()` differs from the
   `nodePoolCurrentRolloutConfig` annotation.
4. The MachineDeployment spec is updated with the new version and
   `DataSecretName`, triggering a CAPI rolling Replace.
5. When the rollout completes (`MachineDeploymentComplete()`), the controller
   updates the `nodePoolCurrentRolloutConfig` annotation to the new rollout hash.
6. The `ConfigUpdatePending` condition transitions to `False`.

#### Operator upgrade (safe migration)

1. The service provider upgrades the HyperShift operator to a version that
   includes this feature.
2. On first reconcile, the controller checks for the
   `nodePoolCurrentRolloutConfig` annotation. It is absent on existing
   NodePools.
3. The controller seeds the annotation with the current computed rollout hash.
   Because both the target and stored values are identical, no rollout is
   triggered.
4. Subsequent reconciles use the annotation-based comparison for rollout
   decisions.

#### Mid-rollout management-side config change

1. A spec-driven rollout is in progress — the MachineDeployment is rolling,
   `UpdatingConfig` condition is `True`.
2. While the rollout is running, a management-side change occurs (e.g., HAProxy
   image bump).
3. The rollout hash has NOT changed (management-side content is excluded), so no
   new rollout is triggered and no new secrets are created.
4. The in-progress rollout continues using the existing token/user-data secrets.
5. When the rollout completes, the `nodePoolCurrentRolloutConfig` annotation is
   updated to the current rollout hash. The nodes that were replaced have the
   ignition payload that was generated at the start of the rollout — they do NOT
   automatically pick up the mid-rollout management-side change.
6. The `ConfigUpdatePending` condition may transition to `True` if the
   management-side change means the current payload differs from what
   newly-created nodes would get on a fresh provision.

Note: a mid-rollout *spec-driven* config change is existing behavior unchanged
by this enhancement. CAPI handles this via the MachineDeployment's rolling
update strategy — the new desired state supersedes the in-progress one, and CAPI
continues rolling until all machines match the latest template.

### API Extensions

This enhancement does not modify CRDs or add webhooks. It introduces:

1. **A new NodePool annotation** for tracking rollout state:

   ```
   hypershift.openshift.io/nodePoolCurrentRolloutConfig: "<hash>"
   ```

   This annotation is controller-managed (not user-settable) and tracks the
   rollout hash that was last applied to the MachineDeployment or MachineSet.

2. **A new NodePool status condition** for visibility into pending config drift:

   ```yaml
   status:
     conditions:
     - type: ConfigUpdatePending
       status: "True"
       reason: ManagementConfigDrift
       message: "Management-side configuration has changed; new nodes will
         receive the updated payload. Existing nodes retain the previous
         configuration until the next spec-driven rollout."
   ```

   When the full payload hash differs from what existing nodes have but the
   rollout hash has not changed, this condition is `True`. It transitions to
   `False` after a spec-driven rollout completes or when there is no drift.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is exclusively for HyperShift. It modifies the NodePool
controller, which runs in the management cluster. No changes are required in the
hosted control plane or guest cluster components.

#### Standalone Clusters

Not applicable. Standalone clusters do not use NodePool or the HyperShift
NodePool controller.

#### Single-node Deployments or MicroShift

Not applicable.

#### OpenShift Kubernetes Engine

Not applicable. This change is internal to the HyperShift operator and does not
depend on features excluded from OKE.

### Implementation Details/Notes/Constraints

#### Two-hash architecture

The `ConfigGenerator` produces two categories of hash values:

| Hash | Category | Inputs | Used for |
|------|----------|--------|----------|
| `Hash()` / `HashWithoutVersion()` | **Non-rollout** | Full MCO config including HAProxy, pull secret name, additional trust bundle name, reconciled global config (with computed defaults) | User-data secret naming, payload generation |
| `RolloutHash()` / `RolloutHashWithoutVersion()` | **Rollout** | MCO config *excluding* HAProxy, pull secret name, additional trust bundle name, user-set global config (proxy spec, image spec — without computed defaults) | Rollout decisions |

The "non-rollout" vs "rollout" categorization is extensible: if new
management-side content is added in the future, it goes into the non-rollout
hash only.

The implementation adds two rollout-specific fields to the `rolloutConfig`
struct:

1. A `rolloutMcoRawConfig` field, computed by a new `parseWithoutHaproxy()`
   method that calls the existing parsing logic with an empty haproxy config
   string.
2. A `rolloutGlobalConfig` field, computed by serializing only the user-set spec
   fields (`HostedCluster.Spec.Configuration.Proxy`,
   `HostedCluster.Spec.Configuration.Image`) without reconciling
   platform-specific defaults:

```go
func (cg *ConfigGenerator) parse(configs []corev1.ConfigMap) (string, error) {
    return cg.doParse(configs, cg.haproxyRawConfig)
}

func (cg *ConfigGenerator) parseWithoutHaproxy(configs []corev1.ConfigMap) (string, error) {
    return cg.doParse(configs, "")
}
```

```go
func rolloutGlobalConfigString(hcluster *hyperv1.HostedCluster) string {
    // Hash only user-set spec fields, not reconciled defaults.
    proxyBytes, _ := json.Marshal(hcluster.Spec.Configuration.Proxy)
    imageBytes, _ := json.Marshal(hcluster.Spec.Configuration.Image)
    return string(proxyBytes) + string(imageBytes)
}
```

This ensures that user changes to proxy or image config trigger rollouts
(correct), while operator code changes to computed defaults (e.g., new
`NoProxy` entries) do not (also correct). The full reconciled config continues
to be used for payload generation via the non-rollout hash.

#### Rollout decision change

In `propagateVersionAndTemplate()` (Replace path) and `reconcileMachineSet()`
(InPlace path), the rollout trigger changes from:

```
userDataSecret.Name != machineDeployment.Spec.Template.Spec.Bootstrap.DataSecretName
```

to:

```go
versionChanged := targetVersion != ptr.Deref(machineDeployment.Spec.Template.Spec.Version, "")
configChanged := currentRolloutConfigHash != "" && targetRolloutConfigHash != currentRolloutConfigHash

if versionChanged || configChanged {
    targetDataSecretName := userDataSecret.Name
    currentDataSecretName := ptr.Deref(machineDeployment.Spec.Template.Spec.Bootstrap.DataSecretName, "")

    if versionChanged || targetDataSecretName != currentDataSecretName {
        // Update MachineDeployment spec
        specUpdated = true
    }
}
```

The inner `targetDataSecretName != currentDataSecretName` guard prevents a
deadlock: if the rollout hash annotation says a rollout is needed but the
MachineDeployment already has the correct spec values, returning `true` would
cause the caller to skip `reconcileMachineDeploymentStatus` — which is where
the annotation gets updated to mark completion. Without this guard, the rollout
would appear stuck indefinitely.

#### Annotation lifecycle

| Event | Annotation action |
|-------|-------------------|
| First reconcile after operator upgrade (annotation absent) | Seed with current `RolloutHashWithoutVersion()` — no rollout |
| Spec-driven change detected | MachineDeployment/MachineSet updated; annotation unchanged until rollout completes |
| `MachineDeploymentComplete()` or `machineSetInPlaceRolloutIsComplete()` | Annotation updated to new `RolloutHashWithoutVersion()` |
| Management-side change only | Annotation unchanged — no rollout |

The `currentRolloutConfigHash != ""` guard in the rollout decision ensures that
if the annotation is somehow absent (e.g., manually deleted), the controller
re-seeds it on the next reconcile rather than triggering a rollout.

#### Token secret and payload cache lifecycle

The rollout hash governs not only the rollout decision but also token and
user-data secret lifecycle:

- **Rollout hash unchanged (management-side change only):** No new token or
  user-data secrets are created. `cleanupOutdated()` is not called. The existing
  secrets remain valid, and the MachineDeployment continues to reference them.
  Scale-up nodes receive the same configuration as existing nodes.

- **Rollout hash changed (spec-driven change):** New token and user-data secrets
  are created with the updated ignition payload. The previous token secret
  receives an expiration timestamp (2-hour TTL via
  `IgnitionServerTokenExpirationTimestampAnnotation`), and the previous user-data
  secret is deleted. The MachineDeployment is updated to reference the new
  user-data secret, triggering a rollout.

This is critical for correctness: if the controller created new secrets on every
`Hash()` change (including management-side), the old user-data secret would be
deleted while the MachineDeployment still referenced it, breaking scale-up
operations.

#### Backward compatibility

The existing annotations `nodePoolAnnotationCurrentConfig` and
`nodePoolAnnotationCurrentConfigVersion` continue to be maintained alongside
the new `nodePoolAnnotationCurrentRolloutConfig` annotation. This ensures
backward compatibility during the transition period and preserves any external
tooling that reads the existing annotations.

#### ConfigUpdatePending condition

A new status condition `ConfigUpdatePending` provides observability into
management-side configuration drift:

```go
const NodePoolConfigUpdatePendingConditionType = "ConfigUpdatePending"
```

The condition is set when the full payload hash (`Hash()`) differs from the
last-applied payload but the rollout hash has not changed. This tells operators
that new nodes will receive an updated ignition payload, but existing nodes
retain their previous configuration.

The condition message indicates the source of the drift:

- `ManagementConfigDrift`: A management-side image or configuration changed.
  No customer action is required.
- When a spec-driven rollout is in progress, this condition is `False` (the
  `UpdatingConfig` condition covers that case).

### Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| Changing hash function triggers rollout on upgrade | Service-wide node replacement | New annotation key with first-reconcile seeding ensures safe migration. The controller seeds the annotation before reaching any rollout decision code. |
| Stale config on scale-up nodes | New nodes get old management-side image | Acceptable — old image still works. When only management-side content changes, no new secrets are created, so scale-up nodes receive the same configuration as existing nodes. The next spec-driven rollout creates new secrets with the latest full payload, bringing all nodes up to date. |
| New rollout-triggering input not added to RolloutHash | Spec change does not trigger rollout | CI tests validate that spec-driven changes produce rollout hash changes. The allowlist approach (excluding only HAProxy) makes omissions explicit and reviewable. |
| Deadlock when rollout hash differs but spec already matches | NodePool appears stuck in updating state | The inner `targetDataSecretName != currentDataSecretName` guard in `propagateVersionAndTemplate` prevents returning `specUpdated=true` when no actual spec change occurred, ensuring `reconcileMachineDeploymentStatus` runs and updates the annotation. |

### Drawbacks

The two-hash architecture adds complexity to the `ConfigGenerator`. Engineers
modifying the ignition config pipeline must understand that there are now two
separate hashes with different inputs, and must decide whether a new input
should be included in the rollout hash.

The `parseWithoutHaproxy()` approach is tied to the current HAProxy-specific
code path. If additional management-side content is added in the future (beyond
HAProxy), the exclusion mechanism will need to be extended.

## Alternatives (Not Implemented)

### Allowlist approach for rollout inputs

Instead of excluding HAProxy from the hash, explicitly list only the inputs that
should trigger a rollout (user configs, release version, pull secret, trust
bundle, global config). This was considered but rejected because:

1. It requires duplicating the serialization logic for each input category.
2. It is fragile — adding a new rollout-relevant input requires updating the
   allowlist, and missing one silently breaks rollout detection.
3. The current approach (exclude HAProxy from the existing parsing pipeline) is
   simpler and leverages the existing `parse()` infrastructure.

### Annotation on MachineDeployment instead of NodePool

Tracking the rollout hash on the MachineDeployment rather than the NodePool was
considered. This was rejected because:

1. The NodePool is the user-facing API object, and conditions/annotations on it
   are more discoverable.
2. The existing `nodePoolAnnotationCurrentConfig` pattern already uses NodePool
   annotations.
3. MachineDeployments are implementation details that may change across
   platforms.

## Open Questions

1. Should the `ConfigUpdatePending` condition distinguish between different
   types of management-side changes (image bumps vs. registry overrides vs.
   other internal config changes)?

## Test Plan

### Unit Tests

- **`TestRolloutHash`**: Verify that `RolloutHash()` changes for spec-driven
  inputs (user MachineConfigs, pull secret, trust bundle, global config, release
  version) and remains stable when only HAProxy content changes.
- **`TestPropagateVersionAndTemplate`**: Verify that the Replace path correctly
  triggers rollouts for version and config changes, and does NOT trigger
  rollouts for management-side changes. Includes a deadlock regression test
  verifying that when the rollout hash differs but the MachineDeployment already
  has correct values, the function returns `false`.
- **`TestIsUpdatingConfig`**: Verify that `isUpdatingConfig()` reads the new
  `nodePoolCurrentRolloutConfig` annotation and returns `false` when the
  annotation is absent.
- **`TestUpdatingConfigCondition`**: Verify that the `UpdatingConfig` condition
  uses `RolloutHashWithoutVersion()` and the new annotation.

### E2E Tests

Three e2e tests validate the end-to-end behavior:

1. **Management-side change does not trigger rollout**: Patches the
   `NodePoolHAProxyImageAnnotation` with a dummy digest and verifies over 2
   minutes that the rollout hash annotation is unchanged, the `UpdatingConfig`
   condition remains `False`, and no nodes are replaced.

2. **Spec-driven change triggers rollout**: Creates a new NodePool with 1
   replica, adds a MachineConfig to `spec.config`, and verifies that the rollout
   hash annotation changes, the `UpdatingConfig` condition transitions to
   `True` then back to `False`, and the node is replaced with updated
   configuration.

3. **Operator upgrade does not trigger rollout**: Verifies that the default
   NodePool has the `nodePoolCurrentRolloutConfig` annotation seeded by the
   controller, that the annotation value is stable across reconciles, and that
   no rollout is triggered.

### CI Integration

The `e2e-aws-upgrade-hypershift-operator` CI job validates that the operator
upgrade path does not trigger spurious rollouts.

## Graduation Criteria

### Dev Preview -> Tech Preview

- All unit tests and e2e tests pass.
- Feature is deployed and validated on at least one managed service environment
  (ROSA HCP or ARO HCP).

### Tech Preview -> GA

- Feature has been running in production for at least one release cycle without
  regressions.
- `ConfigUpdatePending` condition is documented and surfaced in monitoring
  dashboards.
- Backported to all supported HyperShift versions.
- User-facing documentation created in
  [openshift-docs](https://github.com/openshift/openshift-docs/).

### Removing a deprecated feature

Not applicable. This enhancement does not deprecate any existing feature. The
existing `nodePoolAnnotationCurrentConfig` and
`nodePoolAnnotationCurrentConfigVersion` annotations are maintained for backward
compatibility and are not deprecated at this time.

## Upgrade / Downgrade Strategy

### Upgrade

On upgrade to a version containing this feature:

1. The NodePool controller detects that the `nodePoolCurrentRolloutConfig`
   annotation is absent on existing NodePools.
2. The controller seeds the annotation with the computed
   `RolloutHashWithoutVersion()` on the first reconcile.
3. Because the seeded value matches the computed value, no rollout is triggered.
4. The existing `nodePoolAnnotationCurrentConfig` and
   `nodePoolAnnotationCurrentConfigVersion` annotations continue to be
   maintained for backward compatibility.

No user action is required. The upgrade is transparent.

### Downgrade

On downgrade to a version without this feature:

1. The `nodePoolCurrentRolloutConfig` annotation is ignored by the older
   controller (it reads `nodePoolAnnotationCurrentConfig` instead).
2. The older controller resumes using `DataSecretName` comparison for rollout
   decisions.
3. If the full hash changed during the period when the new controller was
   running (e.g., due to a management-side image bump that was suppressed), the
   downgraded controller may trigger a rollout on first reconcile. This is
   acceptable because the older behavior triggers rollouts for all hash changes.

No manual cleanup is required. The `nodePoolCurrentRolloutConfig` annotation
is harmless on older versions.

## Version Skew Strategy

This feature is entirely within the HyperShift operator (management cluster).
There is no version skew concern with guest cluster components, kubelets, or
other node-level components. The NodePool controller is the sole producer and
consumer of the `nodePoolCurrentRolloutConfig` annotation.

## Operational Aspects of API Extensions

This enhancement does not add CRDs, webhooks, or aggregated API servers.

The new `ConfigUpdatePending` condition is a standard NodePool status condition
and does not affect API throughput or availability. It is updated as part of the
existing NodePool reconciliation loop.

### Failure Modes

- **Annotation accidentally deleted**: The controller re-seeds it on the next
  reconcile without triggering a rollout (the `currentRolloutConfigHash != ""`
  guard prevents rollout when the annotation is absent).
- **Rollout hash computation error**: Falls through to the existing behavior —
  the full hash comparison via `DataSecretName` still produces a valid
  user-data secret, so nodes are never left without a valid ignition payload.

## Support Procedures

### Detecting unexpected rollouts

If a NodePool triggers an unexpected rollout after this feature is deployed:

1. Check the `nodePoolCurrentRolloutConfig` annotation value before and after
   the rollout:
   ```
   oc get nodepool <name> -n <namespace> -o jsonpath='{.metadata.annotations.hypershift\.openshift\.io/nodePoolCurrentRolloutConfig}'
   ```

2. Check the `UpdatingConfig` condition:
   ```
   oc get nodepool <name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="UpdatingConfig")]}'
   ```

3. Check the `ConfigUpdatePending` condition to see if management-side drift
   was detected:
   ```
   oc get nodepool <name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="ConfigUpdatePending")]}'
   ```

4. Check the NodePool controller logs for rollout-related messages:
   ```
   oc logs -n hypershift deployment/operator -c operator | grep -E "Starting (version|config) (update|upgrade)"
   ```

### Verifying the feature is active

The `nodePoolCurrentRolloutConfig` annotation is present on all NodePools when
the feature is active. If the annotation is absent, the controller has not yet
reconciled the NodePool since the operator upgrade.
