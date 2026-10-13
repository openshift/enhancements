---
title: control-plane-upgradeable-condition
authors:
  - "@apahim"
reviewers:
  - "@csrwng"
  - "@alvaroaleman"
  - "@celebdor, for networking aspects, please look at CNO/OVN skew implications"
  - "@jcpowermac, for cloud-provider aspects, please look at CCO interaction"
  - "@jsafrane, for storage aspects, please look at cluster-storage-operator and csi-snapshot-controller-operator classification. Confirmed CP-scoped: cluster-storage-operator is a pure pass-through from per-platform CSI driver operators via ClusterCSIDriver CR conditions; no PV/PVC/Node/CSINode watches."
  - "@jmencak, for NTO aspects, please look at node-tuning classification. Confirmed CP-scoped: node-tuning never sets Upgradeable on ClusterOperator (only Available/Progressing/Degraded). PerformanceProfile CR has its own Upgradeable but it does not flow to ClusterOperator."
  - "@joelanford, for OLM aspects: confirm that operator-lifecycle-manager's Upgradeable condition aggregates user-installed OperatorCondition signals, validating the exclusion rationale. Review whether OLM's own platform health could be disaggregated from user-operator signals in a future enhancement."
approvers:
  - "@csrwng"
api-approvers:
  - "@joelspeed"
creation-date: 2026-07-02
last-updated: 2026-07-03
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/HPSTRAT-202
see-also:
  - "/enhancements/hypershift/hypershift-control-plane-version-status.md"
---

# Control Plane Upgradeable Condition: Separating CP and DP Upgrade Readiness Signals

## Summary

This enhancement adds a new `ControlPlaneUpgradeable` status condition to `HostedControlPlane` and `HostedCluster` that reflects the aggregated `Upgradeable` status of only the control-plane-side ClusterOperators, that is, operators that run on the management cluster and write their ClusterOperator objects to the guest cluster via kubeconfig. The existing `ClusterVersionUpgradeable` condition remains unchanged and continues to reflect the CVO's full aggregation of all ClusterOperators (control plane, data plane, and user-installed). The `isUpgrading` function in the HostedCluster controller is updated to prefer `ControlPlaneUpgradeable` for y-stream upgrade gating when available, so that data-plane operators can no longer block control plane upgrades.

## Glossary

- **Control-plane-side operators**: Operators whose code runs as Deployments in the HCP namespace on the management cluster but that write their own `ClusterOperator` objects to the guest cluster via a kubeconfig. Examples: CNO (`network`), dns-operator (`dns`), ingress-operator (`ingress`), cluster-storage-operator (`storage`).
- **HCCO (HostedClusterConfigOperator)**: A controller that runs inside the control-plane-operator on the management cluster. It bridges the management and guest clusters by reading and writing resources in the guest cluster via a kubeconfig. It is responsible for creating synthetic ClusterOperator stubs and, with this enhancement, for computing the `ControlPlaneUpgradeable` condition.
- **HCCO-stubbed operators**: A subset of control-plane-side operators for which the HCCO (see above) creates synthetic `ClusterOperator` objects in the guest with hardcoded `Upgradeable=True`. These are: `kube-apiserver`, `kube-controller-manager`, `kube-scheduler`, `openshift-apiserver`, `openshift-controller-manager`, `operator-lifecycle-manager-packageserver`. These stubs are defined in `hypershift/control-plane-operator/hostedclusterconfigoperator/controllers/resources/clusteroperators.go` in the `clusterOperators()` function.
- **Data-plane operators**: Operators deployed by the CVO into the guest cluster that run on worker nodes. Examples: `monitoring` (cluster-monitoring-operator), `console` (console-operator), `authentication` (cluster-authentication-operator).
- **User-installed operators**: Operators installed via OLM by the cluster user. They run on worker nodes in the guest and can set `Upgradeable=False` via `OperatorCondition` CRs.
- **OLM (management-side but excluded)**: The olm-operator and catalog-operator run as v2 deployments on the management cluster, making them architecturally "control-plane-side." However, OLM aggregates user-installed operator `OperatorCondition` signals onto its own `operator-lifecycle-manager` ClusterOperator `Upgradeable` condition, mixing user-operator state with its own platform health. Because of this mixed signal, OLM is excluded from the CP set (see "Excluded from CP set" in the Proposal section).
- **CVO**: The ClusterVersion Operator, which aggregates `Upgradeable` conditions from all ClusterOperators in the guest cluster into a single `Upgradeable` condition on the `ClusterVersion` resource.
- **`controlPlaneRelease`**: An optional field on `HostedClusterSpec` (`spec.controlPlaneRelease`) that allows setting a different release image for management-side components only, while `spec.release` governs the data plane. It bypasses the Upgradeable check because the guest cluster version is unaffected. See the Problem section for why this is insufficient for y-stream upgrades.
- **CP**: Control plane. Used as shorthand for "control-plane-side" throughout this document.
- **DP**: Data plane. Used as shorthand for "data-plane-side" throughout this document.

## Motivation

### The Problem

Today, the `ClusterVersionUpgradeable` condition on `HostedCluster` is a single boolean that mixes signals from three fundamentally different sources:

1. **Control-plane-side operators** (management cluster), which are operated by the service provider and represent infrastructure the provider controls.
2. **Data-plane operators** (guest worker nodes), which run on customer-owned nodes and may be affected by node health, version lag, or customer configuration.
3. **User-installed OLM operators** (guest worker nodes), which are arbitrary third-party operators that can set `Upgradeable=False` via their `OperatorCondition` CR for any reason.

When this mixed signal is `False`, the `isUpgrading` function blocks y-stream control plane upgrades. Service providers must then apply the `hypershift.openshift.io/force-upgrade-to` annotation to override the block. However, this override is a blunt instrument: it bypasses **all** Upgradeable signals, including legitimate warnings from control-plane-side operators (e.g., CNO detecting a network configuration incompatible with the target version).

This creates a no-win situation for service providers:

- **Without `force-upgrade-to`**: A customer-installed OLM operator or a degraded data-plane operator can indefinitely block management-side control plane upgrades, including CVE patches and EOL transitions.
- **With `force-upgrade-to`**: All safety signals are bypassed, including control-plane operator warnings that the provider should investigate before upgrading.

`spec.controlPlaneRelease` provides a partial workaround: it allows patching management-side components to a different release image while keeping the data plane at `spec.release`, bypassing the Upgradeable check because the guest cluster version is unaffected. However, this only covers z-stream management-side patches where the data plane can remain untouched. It does not help when a y-stream upgrade is required (e.g., EOL transitions, version support boundaries) or when the upgrade requires coupled control-plane and data-plane version changes.

### User Stories

- As a **service provider (ROSA/ARO/GCP)**, I want control plane y-stream upgrades to proceed when only data-plane or user-installed operators report `Upgradeable=False`, so that customer actions on worker nodes cannot block my control plane lifecycle management.
- As a **service provider**, I want to be alerted when a control-plane-side operator (e.g., CNO, ingress-operator) reports `Upgradeable=False`, so that I can investigate and resolve configuration issues before upgrading the control plane.
- As a **service provider**, I want to stop routinely applying `force-upgrade-to` on every y-stream upgrade, so that I retain the safety net of control-plane operator Upgradeable signals.
- As a **cluster administrator** (self-managed), I want to see separate signals for control-plane and data-plane upgrade readiness, so that I can distinguish between "the control plane is not ready" and "worker nodes need attention."
- As an **SRE**, I want fleet-wide visibility into which clusters have control-plane-specific upgrade blockers vs. data-plane-only blockers, so that I can prioritize remediation efforts accurately.

### Goals

- Add a `ControlPlaneUpgradeable` condition to `HostedControlPlane` and `HostedCluster` status that aggregates `Upgradeable` only from control-plane-side ClusterOperators.
- Update the `isUpgrading` function to use `ControlPlaneUpgradeable` for y-stream upgrade gating when the condition is available, falling back to `ClusterVersionUpgradeable` for backward compatibility with older CPOs.
- Maintain the existing `ClusterVersionUpgradeable` condition unchanged, preserving the full CVO aggregation for observability.

### Non-Goals

- Modifying the CVO's `Upgradeable` condition aggregation logic. The CVO continues to aggregate all ClusterOperators as it does today.
- Adding a complementary `DataPlaneUpgradeable` condition. The existing `ClusterVersionUpgradeable` already provides the full-aggregation signal; a data-plane-only condition can be derived by comparing the two conditions if needed.
- Changing the behavior of the `force-upgrade-to` annotation or `controlPlaneRelease` override. These remain available as escape hatches.
- Defining which specific ClusterOperator Upgradeable reasons are "safe to ignore." This enhancement separates the signal by source; the interpretation of individual operator conditions remains the provider's responsibility.
- Deprecating `ClusterVersionUpgradeable` as a gating signal. Both conditions are intended to be permanent: `ClusterVersionUpgradeable` for full observability, `ControlPlaneUpgradeable` for CP-specific gating.

## Proposal

### New Condition: `ControlPlaneUpgradeable`

A new condition type `ControlPlaneUpgradeable` is added to the HyperShift API. It is set on both `HostedControlPlane` and `HostedCluster` status, following the same bubble-up pattern as existing CVO conditions.

The condition aggregates the `Upgradeable` condition from a known set of control-plane-side ClusterOperators in the guest cluster:

**Operator classification rationale:**

| ClusterOperator | Component | Runs On | Upgradeable Reflects | Classification |
|----------------|-----------|---------|---------------------|---------------|
| `network` | CNO | Management cluster | Network config compatibility with target version (CP-scoped) | CP set |
| `dns` | dns-operator | Management cluster | DNS config (CP-scoped) | CP set |
| `ingress` | ingress-operator | Management cluster | Ingress config (CP-scoped) | CP set |
| `storage` | cluster-storage-operator | Management cluster | Pass-through from per-platform CSI driver operators via ClusterCSIDriver CR conditions; no PV/PVC/Node/CSINode watches (CP-scoped) | CP set |
| `image-registry` | cluster-image-registry-operator | Management cluster | Never sets Upgradeable (only sets Available/Progressing/Degraded); absent condition treated as upgradeable per CVO convention (CP-scoped) | CP set |
| `node-tuning` | NTO | Management cluster | Never sets Upgradeable on ClusterOperator (only sets Available/Progressing/Degraded); PerformanceProfile CR has its own Upgradeable but it does not flow to ClusterOperator; absent condition treated as upgradeable per CVO convention (CP-scoped) | CP set |
| `csi-snapshot-controller` | csi-snapshot-controller-operator | Management cluster | CSI snapshot config (CP-scoped) | CP set |
| `cloud-credential` | CCO | Management cluster | Root credential secret existence and manual-mode upgrade annotation; does not inspect CredentialsRequest objects or per-component credential health (CP-scoped) | CP set |
| `kube-storage-version-migrator` | kube-storage-version-migrator | Management cluster | Migration state (CP-scoped) | CP set |
| `openshift-apiserver` | HCCO stub | N/A (synthetic) | Always True (hardcoded) | HCCO-stubbed |
| `openshift-controller-manager` | HCCO stub | N/A (synthetic) | Always True (hardcoded) | HCCO-stubbed |
| `kube-apiserver` | HCCO stub | N/A (synthetic) | Always True (hardcoded) | HCCO-stubbed |
| `kube-controller-manager` | HCCO stub | N/A (synthetic) | Always True (hardcoded) | HCCO-stubbed |
| `kube-scheduler` | HCCO stub | N/A (synthetic) | Always True (hardcoded) | HCCO-stubbed |
| `operator-lifecycle-manager-packageserver` | HCCO stub | N/A (synthetic) | Always True (hardcoded) | HCCO-stubbed |
| `operator-lifecycle-manager` | olm-operator | Management cluster | Mixed: aggregates user OperatorCondition signals | Excluded |
| `operator-lifecycle-manager-catalog` | catalog-operator | Management cluster | Mixed: same as olm-operator | Excluded |

**Excluded from CP set (mixed signal):**
- `operator-lifecycle-manager` (olm-operator): OLM runs as a v2 deployment on the management cluster, making it architecturally "control-plane-side." However, OLM aggregates user-installed operator `OperatorCondition` signals onto its own ClusterOperator `Upgradeable` condition. Including OLM in the CP set would mean user-installed operators can still block `ControlPlaneUpgradeable`, undermining the primary goal of this enhancement. Excluding OLM means a genuine OLM platform issue would not block `ControlPlaneUpgradeable`; this trade-off is accepted because OLM platform issues are rare and the `ClusterVersionUpgradeable` condition still reflects them for observability.
- `operator-lifecycle-manager-catalog` (catalog-operator): same rationale as `operator-lifecycle-manager`.

**Operational guidance for OLM exclusion:** When `ClusterVersionUpgradeable=False` and `ControlPlaneUpgradeable=True`, service providers should inspect the `operator-lifecycle-manager` ClusterOperator's `Upgradeable` condition message to determine whether the block is user-operator-driven or an OLM platform issue. A future enhancement should consider disaggregating OLM's own platform health from user-operator signals.

**Management-side v2 components that do not write ClusterOperator objects:**
- `openshift-route-controller-manager`: functionality is subsumed by the HCCO-stubbed `openshift-controller-manager` ClusterOperator.
- `cluster-policy-controller`: does not write a separate ClusterOperator object; its status is reflected through other mechanisms.
- `openshift-oauth-apiserver`: does not write a separate ClusterOperator object in HyperShift topology.

**Not listed (no ClusterOperator object in HyperShift):**
- `etcd`: In HyperShift, etcd is managed directly by the CPO as a StatefulSet on the management cluster. Unlike standalone OpenShift where `cluster-etcd-operator` writes an `etcd` ClusterOperator, HyperShift does not create an `etcd` ClusterOperator object in the guest cluster. There is nothing to aggregate.

The condition values:

| Status | Meaning |
|--------|---------|
| `True` | All control-plane-side ClusterOperators report `Upgradeable=True` (or have no `Upgradeable` condition, which is treated as upgradeable per CVO convention; see the `ClusterStatusConditionType` documentation in `openshift/api` at `config/v1/types_cluster_operator.go`: "The cluster-version operator will allow updates when this condition is not False, including when it is missing, True, or Unknown."). |
| `False` | At least one control-plane-side ClusterOperator reports `Upgradeable=False`. The message includes the operator name(s) and their reason/message. |
| `Unknown` | Unable to determine status (e.g., no ClusterOperator objects found yet during bootstrap, or listing failed). |

### Upgrade Gating Change

The `isUpgrading` function in the HostedCluster controller is modified to prefer `ControlPlaneUpgradeable` over `ClusterVersionUpgradeable` when it is available:

1. Check `ControlPlaneUpgradeable` on the HostedCluster status.
2. If it exists and is not `Unknown`, use it for upgrade gating.
3. If it is absent or `Unknown`, fall back to `ClusterVersionUpgradeable` (backward compatibility with older CPOs that do not report this condition).
4. The rest of the upgrade gating logic (force-upgrade annotation, `controlPlaneRelease` override, z-stream exception) remains unchanged.

This means:
- A data-plane operator or user-installed OLM operator reporting `Upgradeable=False` will cause `ClusterVersionUpgradeable=False` but will NOT block y-stream CP upgrades (because `ControlPlaneUpgradeable` is `True`).
- A control-plane operator reporting `Upgradeable=False` will cause both `ClusterVersionUpgradeable=False` AND `ControlPlaneUpgradeable=False`, and WILL block y-stream CP upgrades.

### Workflow Description

**Service provider** is the operator responsible for managing the hosted control plane lifecycle (e.g., ROSA SRE, ARO operations, GCP HCP automation).

**Cluster user** is the consumer who owns the worker nodes and may install operators via OLM.

#### Normal Upgrade (No Blockers)

1. Service provider sets `spec.release.image` to the new y-stream release.
2. The HCCO's hcpstatus controller lists ClusterOperators from the guest cluster, checks the `Upgradeable` condition on each CP-side operator, and sets `ControlPlaneUpgradeable=True` on the HCP.
3. The HostedCluster controller copies `ControlPlaneUpgradeable=True` to the HostedCluster status.
4. `isUpgrading` sees `ControlPlaneUpgradeable=True` and allows the upgrade.
5. The upgrade proceeds.

#### Data-Plane Blocker (User-Installed Operator)

1. A user-installed OLM operator sets `Upgradeable=False` on its `OperatorCondition`.
2. OLM reflects this to the `operator-lifecycle-manager` ClusterOperator's `Upgradeable` condition.
3. The CVO aggregates this and sets `ClusterVersion.Upgradeable=False`.
4. The HCCO bubbles this up: `ClusterVersionUpgradeable=False` on the HCP and HostedCluster.
5. The HCCO independently checks CP-side ClusterOperators. Because `operator-lifecycle-manager` is **excluded** from the CP set (it mixes user-operator signals with its own readiness), the user-installed operator's `Upgradeable=False` does **not** affect `ControlPlaneUpgradeable`. The condition remains `True`.
6. `isUpgrading` sees `ControlPlaneUpgradeable=True` and allows the y-stream upgrade to proceed.
7. The service provider can observe `ClusterVersionUpgradeable=False` for awareness but is not blocked.

#### Control-Plane Blocker (Legitimate Warning)

1. CNO detects a network configuration incompatible with the target y-stream and sets `Upgradeable=False` on the `network` ClusterOperator.
2. The HCCO sets `ControlPlaneUpgradeable=False` on the HCP with message: `"Control plane operators not upgradeable: network (reason: IncompatibleConfig): ..."`.
3. The HostedCluster controller copies this to HostedCluster status.
4. `isUpgrading` sees `ControlPlaneUpgradeable=False` and blocks the y-stream upgrade.
5. The service provider investigates the CNO warning, fixes the configuration, and the upgrade proceeds without needing `force-upgrade-to`.

### API Extensions

This enhancement adds a new status condition type to the existing `Conditions []metav1.Condition` field on `HostedControlPlane` and `HostedCluster`. No new CRDs, webhooks, or finalizers are introduced.

```go
// ControlPlaneUpgradeable reflects the aggregated Upgradeable status of only
// control-plane-side ClusterOperators (operators that run on the management
// cluster). Unlike ClusterVersionUpgradeable, this excludes data-plane
// operators, user-installed OLM operators, and the OLM ClusterOperators
// themselves (which mix user-operator signals with their own readiness).
ControlPlaneUpgradeable ConditionType = "ControlPlaneUpgradeable"
```

This constant is defined in `hypershift/api/hypershift/v1beta1/hostedcluster_conditions.go` alongside other HyperShift-specific condition types.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specific to Hypershift. It addresses the unique topology where control-plane operators run on the management cluster and data-plane operators run on guest worker nodes, creating a need to separate their upgrade readiness signals.

The new condition is produced by the HCCO (management cluster) and consumed by the HostedCluster controller (management cluster). No changes are made to guest-cluster components.

#### Standalone Clusters

Not applicable. In standalone clusters, there is no control-plane / data-plane split, so all operators run on the same cluster. The CVO's single `Upgradeable` condition is sufficient.

#### Single-node Deployments or MicroShift

Not applicable for standalone SNO or MicroShift. Management clusters running as SNO are a supported edge configuration for HyperShift. The additional resource cost from this enhancement is negligible: one informer watch on ~18 ClusterOperator objects and a periodic condition computation.

#### OpenShift Kubernetes Engine

Not applicable.

#### Disconnected / Air-Gapped Environments

`ControlPlaneUpgradeable` has no external dependencies because it reads ClusterOperator objects from the guest cluster via the HCCO's local informer cache on the management cluster. Disconnected environments function identically to connected environments. Individual CP-side operators may report `Upgradeable=False` for disconnected-specific reasons (e.g., a network configuration incompatible with the target version), but this is correct behavior, as the operator is legitimately signaling an issue that should be investigated before upgrading.

### Implementation Details/Notes/Constraints

#### Feature Gate

This enhancement is gated behind the `ControlPlaneUpgradeableCondition` feature gate:

- **Dev Preview (`DevPreviewNoUpgrade`)**: The HCCO computes and sets `ControlPlaneUpgradeable` on HCP status, and it is propagated to HostedCluster status. However, `isUpgrading` does **not** consume it; upgrade gating continues to use `ClusterVersionUpgradeable` only. This phase allows service providers to observe the condition and validate the CP operator classification without changing upgrade behavior.
- **Tech Preview (`TechPreviewNoUpgrade`)**: `isUpgrading` is updated to prefer `ControlPlaneUpgradeable` over `ClusterVersionUpgradeable` when available. Upgrade gating behavior changes. Validated in managed service canary environments.
- **GA (Default)**: Feature gate enabled by default. `ControlPlaneUpgradeable` is the primary y-stream upgrade gating signal for HostedClusters. **Behavioral change on GA**: When the feature gate is enabled by default, all HostedClusters on that version silently change behavior: data-plane blockers that previously blocked CP upgrades no longer do so. Service providers who relied on data-plane `Upgradeable=False` as an additional CP upgrade safety check should review their upgrade policies before the GA release. The `ClusterVersionUpgradeable` condition continues to surface data-plane blockers for observability.

Tests are labeled with `[OCPFeatureGate:ControlPlaneUpgradeableCondition]` and run in both `TechPreviewNoUpgrade` and `Default` Prow job variants.

The feature gate is registered in `openshift/api/features/features.go` with Jira component `Hypershift` and contact `@enxebre`. It is consumed at runtime via the FeatureGateAccessor pattern from library-go. The HCCO reads feature gate state from the management cluster's `featuregates.config.openshift.io/cluster` resource to determine whether to compute and set `ControlPlaneUpgradeable`. The HostedCluster controller similarly checks the feature gate to decide whether `isUpgrading` should prefer `ControlPlaneUpgradeable`. This is a runtime behavioral gate, not a CRD field gate, so the `+openshift:enable:FeatureGate=` CRD generation markers are not applicable here.

Note: `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade` feature sets prevent cluster upgrades. The transition from TechPreview to GA (feature gate enabled by default) follows the standard OpenShift feature gate lifecycle where the condition becomes unconditionally available.

#### HCCO hcpstatus Controller Changes

The HCCO's hcpstatus controller (`control-plane-operator/hostedclusterconfigoperator/controllers/hcpstatus/hcpstatus.go`) is extended to:

1. **Add a watch on ClusterOperator objects** from the guest cluster cache (cache already configured at `operator/config.go:125`; a new `source.Kind` watch with an `EnqueueRequestsFromMapFunc` mapper must be added to the controller's `Setup()` function, which currently only watches `HostedControlPlane`, `ClusterVersion`, and `Authentication`).
2. **Define a `controlPlaneOperators` set** listing the ClusterOperator names that correspond to management-side operators. This is a hardcoded `sets.Set[string]` maintained alongside the code. The set changes only when HyperShift adds or removes a management-side operator component.
3. **Aggregate per-operator Upgradeable** by listing all ClusterOperators, filtering to the CP set, and checking each one's `Upgradeable` condition (Go constant `configv1.OperatorUpgradeable`, on-wire condition type `"Upgradeable"`). A missing `Upgradeable` condition on a present ClusterOperator is treated as upgradeable (matching CVO convention). **Quorum rule**: `ControlPlaneUpgradeable` is set to `Unknown` until all operators in the `controlPlaneOperators` set (the self-reporting management-side operators, not the HCCO-stubbed operators) are present in the guest cluster. The quorum count is derived at runtime from `len(controlPlaneOperators)`, not a hardcoded constant, so it automatically adjusts when operators are added or removed from the set. HCCO-stubbed operators are excluded from quorum because the HCCO creates them and they are always present by construction. If a CP-set ClusterOperator that was previously present disappears, the HCCO logs a warning and re-evaluates quorum; if the count drops below `len(controlPlaneOperators)`, the condition transitions to `Unknown`.
4. **Set `ControlPlaneUpgradeable`** on the HCP status. The controller uses the existing RFC 6902 JSON Patch mechanism for status updates (consistent with how CVO conditions are already set on HCP status). The HCCO also logs a warning (structured field `event=uncategorized-clusteroperator`) for any ClusterOperator that is not in the CP set or the HCCO-stubbed set. Some warnings from legitimate data-plane operators are expected; the goal is to detect new management-side operators that should be added to the CP set. This warning is also exposed as a `hypershift_hcco_uncategorized_clusteroperator` counter metric with an `operator_name` label for fleet-wide visibility.

#### HostedCluster Controller Changes

The HostedCluster controller (`hypershift-operator/controllers/hostedcluster/hostedcluster_controller.go`) is extended to:

1. **Propagate `ControlPlaneUpgradeable`** from HCP to HostedCluster status as a strict copy (no message transformation). This follows the direct HCP condition copy pattern used by `setHostedClusterConditionsFromHCP()` (which copies conditions like `EtcdAvailable`, `KubeAPIServerAvailable`, etc.), not the CVO conditions map pattern (which derives conditions from CVO status like `ClusterVersionSucceeding`), because `ControlPlaneUpgradeable` is set by the HCCO rather than derived from CVO conditions. The HCP is the authoritative source for this condition; the HostedCluster copy is a convenience for fleet management tools (OCM, managed service automation) that read HostedCluster status. In case of divergence, HCP is canonical. Propagation is triggered by the existing HCP status watch; the staleness window between HCP update and HostedCluster update is sub-second in practice (standard controller-runtime watch-triggered reconciliation).
2. **Modify `isUpgrading()`** to prefer `ControlPlaneUpgradeable` over `ClusterVersionUpgradeable` when it is present and not `Unknown`. The new check replaces the existing `ClusterVersionUpgradeable` lookup at the same position in the execution flow (the `meta.FindStatusCondition` call in `isUpgrading()`). The `ForceUpgradeToAnnotation`, `ControlPlaneRelease`, and z-stream checks that follow are unchanged. Note that `isUpgrading()` is called from two sites with different consequences: (a) during the main reconcile flow, where it gates whether reconciliation proceeds with the new release, and (b) inside `isProgressing()`, where it determines the `Progressing` condition status. Because `isUpgrading()` handles the `ControlPlaneUpgradeable` fallback internally, both call sites work correctly without modification. Implementation note: verify with `grep -rn isUpgrading` that no additional call sites exist beyond the two documented here.
3. **Update `reconcile_legacy.go`**: the legacy reconcile path has a duplicate condition propagation map and its own calls to `isUpgrading()` and `isProgressing()` that must also be updated consistently. The legacy path executes for HostedClusters that have not yet migrated to the current reconcile path. Both the condition propagation map and the `isUpgrading()` implementation in the legacy path must include `ControlPlaneUpgradeable` support.

#### Downstream Impact

- **NodePool upgrades**: Unaffected. `ControlPlaneUpgradeable` gates only CP y-stream upgrades, not NodePool upgrades.
- **OCM / fleet management**: Consumers that read `ClusterVersionUpgradeable` for upgrade decisions should be aware of the new condition but are not required to change because `ClusterVersionUpgradeable` continues to reflect the full CVO aggregation unchanged.
- **Service provider automation**: Existing automation that monitors `ClusterVersionUpgradeable` to decide whether to apply `force-upgrade-to` may produce false positives once `ControlPlaneUpgradeable` is in use (data-plane-only blockers would cause `ClusterVersionUpgradeable=False` but no longer block CP upgrades). Service providers should update alerting rules to also check `ControlPlaneUpgradeable`. See "Migration from `force-upgrade-to`" in the Upgrade / Downgrade Strategy section.
- **Cincinnati / update service**: Unaffected. The CVO's `Upgradeable` condition on `ClusterVersion`, which is consumed by Cincinnati to determine upgrade edge availability, is unchanged. `ControlPlaneUpgradeable` is consumed only by the HyperShift-level upgrade gating in the HostedCluster controller, not by the CVO or Cincinnati.

#### Backward Compatibility

- An older CPO/HCCO that does not know about `ControlPlaneUpgradeable` will not set it on the HCP. The HostedCluster controller sees it as absent and falls back to `ClusterVersionUpgradeable`. No behavioral regression.
- A newer CPO writing `ControlPlaneUpgradeable` to an HCP consumed by an older hypershift-operator is harmless because the older operator ignores unknown conditions.

#### Operator List Maintenance

The hardcoded set of control-plane operators is the simplest approach. The list changes only when HyperShift adds or removes a management-side operator (which requires code changes in the CPO v2 component registration anyway). A comment in the set definition documents the categorization criteria and lists the authoritative sources (`clusteroperators.go` for stubs, `v2/assets/*/deployment.yaml` for self-reporting operators).

To guard against the set becoming stale, unit tests assert that the hardcoded CP set matches the documented list. The HCCO also logs a warning when it encounters an uncategorized ClusterOperator in the guest cluster (see HCCO changes above).

Changes to the `controlPlaneOperators` set require review from the HyperShift approver and the affected component owner. A CODEOWNERS rule on the file containing the set is required.

The v2 component framework (`control-plane-operator/controllers/hostedcontrolplane/v2/`) does not currently carry ClusterOperator metadata. Components have a `ComponentName` (deployment name) but no `ClusterOperatorName` field. Deriving the CP set from v2 component metadata is targeted as a follow-up improvement for the release after initial GA. A Jira issue will be created under HPSTRAT-202 to track this work.

#### Failure Modes

**Guest cluster API unavailable**: If the HCCO cannot list ClusterOperator objects from the guest cluster (e.g., network partition, guest API server down), the controller sets `ControlPlaneUpgradeable=Unknown`. The `isUpgrading` function treats `Unknown` as absent and falls back to `ClusterVersionUpgradeable`. This is consistent with the existing behavior for `ClusterVersionUpgradeable`, which also reads from the guest cluster. The HCCO logs errors from failed list operations. The informer cache continues to serve last-known state until it detects the connection loss, at which point the controller sets `Unknown`. No additional timeout or circuit breaker is needed beyond standard informer cache behavior.

**Stale informer cache**: If a network partition occurs between management and guest clusters, the HCCO's informer cache may contain stale ClusterOperator data. A CP-side operator could set `Upgradeable=False` after the cache went stale, but `ControlPlaneUpgradeable` would remain `True` until the cache refreshes. Informer watch connections typically detect failure within 30-60 seconds and trigger a re-list; during this window, stale data is served. This is the same staleness risk that exists today for `ClusterVersionUpgradeable` (which also reads `ClusterVersion` from the guest cluster). However, this enhancement elevates the risk: previously, stale `ClusterVersionUpgradeable=True` during a partition could allow an upgrade past any operator warning. Now, stale `ControlPlaneUpgradeable=True` specifically bypasses a CP-side operator's warning, which is the one class of signal this enhancement is designed to respect.

**Staleness mitigation**: The HCCO re-stamps `ControlPlaneUpgradeable.lastTransitionTime` on every successful ClusterOperator list (heartbeat pattern), even when the condition status does not change. The `isUpgrading` function checks that `lastTransitionTime` is within a configurable maximum age (default: 10 minutes) before trusting the condition. If the condition is older than the threshold, it is treated as `Unknown` and triggers fallback to `ClusterVersionUpgradeable`. This bounds the stale-cache risk to the informer detection window (~30-60s) plus the heartbeat interval, rather than allowing arbitrarily stale conditions. The `ControlPlaneUpgradeableUnknown` alert (15-minute threshold) provides fleet-wide detection when the HCCO recognizes the partition.

**Missing ClusterOperators during bootstrap**: During initial cluster creation, ClusterOperator objects are created incrementally. The HCCO sets `ControlPlaneUpgradeable=Unknown` until all operators in the `controlPlaneOperators` set are present in the guest cluster (see the quorum rule in the HCCO Controller Changes section above). HCCO-stubbed operators are excluded from quorum because the HCCO creates them. This prevents prematurely reporting `True` before operators have had a chance to report their status. The `Unknown` state triggers fallback to `ClusterVersionUpgradeable`.

**CP-side ClusterOperator deleted post-bootstrap**: If a CP-side ClusterOperator object is deleted from the guest cluster after bootstrap (e.g., operator uninstall, CRD corruption, accidental deletion), the quorum check detects the missing operator and transitions `ControlPlaneUpgradeable` to `Unknown`. The HCCO logs a warning (structured field `event=clusteroperator-missing`, `operator=<name>`) identifying the missing operator. The `Unknown` state triggers fallback to `ClusterVersionUpgradeable`, which is strictly more conservative.

**HCCO restart**: Standard level-driven reconciliation recomputes `ControlPlaneUpgradeable` from scratch on the first reconcile after restart. The condition may be briefly absent from HCP status during the restart window. The HostedCluster controller treats absence as fallback to `ClusterVersionUpgradeable`, so there is no behavioral gap.

**Concurrent condition updates**: The condition flows from HCCO (sets on HCP) to HostedCluster controller (copies to HostedCluster). If the HCCO updates the condition while the HostedCluster controller is mid-reconcile, the HostedCluster may briefly reflect the old value. This is benign and self-heals on the next reconcile (standard controller-runtime behavior).

**Hibernated HostedCluster**: When a HostedCluster is hibernated, the guest cluster API server is shut down and the HCCO cannot list ClusterOperator objects. The HCCO sets `ControlPlaneUpgradeable=Unknown`. When both `ControlPlaneUpgradeable` and `ClusterVersionUpgradeable` are `Unknown` or absent (as during hibernation), `isUpgrading` treats them as absent and allows the upgrade to proceed. This matches the existing behavior where a missing `ClusterVersionUpgradeable` permits upgrades (see the `isUpgrading` function in `hostedcluster_controller.go`: `if upgradeable == nil || upgradeable.Status == metav1.ConditionTrue { return true }`). Service providers initiating y-stream upgrades on hibernated HostedClusters should be aware that Upgradeable safety checks are effectively disabled in this state; the `force-upgrade-to` annotation is not required, but the upgrade proceeds without Upgradeable gating. This is unchanged from current behavior. On wake-from-hibernation, the HCCO re-lists ClusterOperators and restores the condition to its normal state.

**Paused reconciliation**: When `pauseReconciliation` is set on the HostedCluster, the hypershift-operator stops reconciling. Condition propagation from HCP to HostedCluster pauses, meaning `ControlPlaneUpgradeable` on the HostedCluster may become stale. This is expected behavior consistent with all other conditions and is inherent to the `pauseReconciliation` mechanism.

#### Metrics and Alerting

The following metrics are exposed by the hypershift-operator:

- `hypershift_hosted_cluster_control_plane_upgradeable`: gauge metric per HostedCluster with label `status` (`true`, `false`, `unknown`). Value `1` on the matching status label, `0` on the others. Labels: `namespace`, `name`, `status`. This label-based approach avoids the Prometheus anti-pattern of using sentinel values (e.g., `-1`) in gauges, which breaks aggregation functions like `sum()` and `avg()`.

The following metric is exposed by the HCCO:

- `hypershift_hcco_uncategorized_clusteroperator`: counter metric per uncategorized ClusterOperator encountered. Labels: `operator_name`. This provides fleet-wide visibility into categorization drift when new ClusterOperators appear.

The following alerts are defined:

- `ControlPlaneUpgradeableBlocked`: fires when `ControlPlaneUpgradeable=False` persists for more than 1 hour and the HostedCluster has a pending y-stream upgrade (i.e., `spec.release.image` differs from the current version). The 1-hour threshold avoids noise from transient `Upgradeable=False` during operator restarts or reconcile delays while still providing timely notification for persistent blockers.
- `ControlPlaneUpgradeableUnknown`: fires when `ControlPlaneUpgradeable=Unknown` persists for more than 15 minutes. The 15-minute threshold allows for guest cluster API temporary unavailability (e.g., API server rollout) while detecting sustained connectivity issues. Note: bootstrap can legitimately exceed 15 minutes; the alert expression includes `unless on(namespace, name) hostedcluster_phase{phase="Creating"}` to suppress during bootstrap.
- `ControlPlaneOperatorUncategorized`: fires when `hypershift_hcco_uncategorized_clusteroperator` is greater than 0 for more than 30 minutes. Indicates a new ClusterOperator has appeared that is not in the CP set or HCCO-stubbed set, and the `controlPlaneOperators` set may need updating. This prevents categorization drift from silently weakening the safety guarantee.

These metrics and alerts enable fleet-wide dashboards for service providers to distinguish CP vs DP blockers at scale (User Story 5).

### Risks and Mitigations

**Risk**: The hardcoded operator list becomes stale when new management-side operators are added.
**Mitigation**: The list is co-located with the HCCO code that creates ClusterOperator stubs. Adding a new v2 component that writes its own ClusterOperator requires code changes in the same area, making it natural to update the set. Unit tests assert the set matches the documented list. The HCCO logs a warning when it encounters an uncategorized ClusterOperator. A follow-up improvement to derive the list from v2 component metadata is targeted for the release after initial GA.

**Risk**: OLM is excluded from the CP set due to user-signal mixing (see "Excluded from CP set" in the Proposal section), which means a genuine OLM platform issue would not block `ControlPlaneUpgradeable`.
**Mitigation**: `ClusterVersionUpgradeable` still reflects the full CVO aggregation, so OLM issues remain visible for observability. OLM platform issues are rare, and the primary goal is preventing user-installed operators from blocking CP upgrades.

**Risk**: A new guest-cluster operator (deployed by CVO, running on workers) is mistakenly added to the CP set.
**Mitigation**: The set is documented with clear categorization criteria. Code review should catch miscategorization.

### Drawbacks

- Adds another condition to an already condition-rich status, increasing surface area for consumers to understand.
- The hardcoded operator list is a maintenance burden, though a small one given the low rate of change.
- Excluding OLM from the CP set means a genuine OLM platform issue would not block `ControlPlaneUpgradeable` (see "Excluded from CP set" in the Proposal section for full rationale).

## Alternatives (Not Implemented)

### Parse the CVO Upgradeable Message

The CVO's `Upgradeable=False` message includes the names of ClusterOperators that are not upgradeable. The HCCO could parse this message to extract operator names and cross-reference against the CP set, avoiding the need to list ClusterOperators individually.

This was rejected because: the CVO message format is unstructured free text, making parsing fragile and version-dependent. Listing ClusterOperators directly is more robust.

### Informational-Only Condition (No Gating Change)

Add `ControlPlaneUpgradeable` as a purely informational condition without modifying `isUpgrading`. Service providers would use it for alerting/dashboards but continue to use `force-upgrade-to` for upgrades.

This was rejected because: the primary value of the enhancement is avoiding routine use of `force-upgrade-to`. An informational-only condition helps with visibility but does not solve the core problem of data-plane operators blocking CP upgrades.

### Derive the CP Operator List from v2 Component Registration

Instead of a hardcoded set, derive the list of CP operators from the v2 component registration metadata (e.g., adding a `ClusterOperatorName` field to each component).

This was deferred because: the v2 components do not currently carry ClusterOperator metadata, and adding it would be a larger change. The hardcoded set is sufficient for the initial implementation and can be replaced later.

## Open Questions

1. Should there be a mechanism (annotation, label) to dynamically add operators to the CP set without code changes?

   Reasoning for "no": The hardcoded set is sufficient for all known consumers (ROSA, ARO, GCP HCP). Custom management-side operators that write ClusterOperator objects are not a supported pattern in HyperShift. The set changes only when HyperShift adds or removes a management-side operator component, which requires code changes in the CPO regardless. If dynamic extensibility becomes a requirement in the future, a follow-up enhancement should define an annotation-based self-declaration mechanism on ClusterOperator objects. The `hypershift_hcco_uncategorized_clusteroperator` metric provides detection of operators that may need to be added to the set.

## Test Plan

**Unit tests (HCCO hcpstatus controller):**
- All CP-side ClusterOperators report `Upgradeable=True` → `ControlPlaneUpgradeable=True`.
- One CP-side ClusterOperator reports `Upgradeable=False` → `ControlPlaneUpgradeable=False` with message naming the operator.
- A ClusterOperator not in the CP set reports `Upgradeable=False` → `ControlPlaneUpgradeable=True` (not affected).
- A CP-side ClusterOperator has no `Upgradeable` condition → treated as upgradeable (CVO convention).
- No ClusterOperator objects exist (bootstrap) → `ControlPlaneUpgradeable=Unknown`.
- Fewer than `len(controlPlaneOperators)` self-reporting CP-side ClusterOperators present → `ControlPlaneUpgradeable=Unknown` (quorum not met; HCCO-stubbed operators are excluded from quorum).
- A previously present CP-side ClusterOperator disappears → quorum re-evaluated, `ControlPlaneUpgradeable=Unknown` if count drops below `len(controlPlaneOperators)`.
- `operator-lifecycle-manager` ClusterOperator reports `Upgradeable=False` → `ControlPlaneUpgradeable=True` (OLM is excluded from CP set).
- ClusterOperator listing fails → `ControlPlaneUpgradeable=Unknown`.
- CP set assertion test: the hardcoded CP set matches the documented list.

**Unit tests (HostedCluster controller):**
- `ControlPlaneUpgradeable=True` → y-stream upgrade allowed.
- `ControlPlaneUpgradeable=False` → y-stream upgrade blocked.
- `ControlPlaneUpgradeable=Unknown` → fallback to `ClusterVersionUpgradeable`.
- `ControlPlaneUpgradeable` absent → fallback to `ClusterVersionUpgradeable` (backward compatibility with older CPO).
- `force-upgrade-to` annotation set + `ControlPlaneUpgradeable=False` → upgrade allowed (override works).
- z-stream upgrade + `ControlPlaneUpgradeable=False` → upgrade allowed (z-stream exception).
- Condition propagation from HCP to HostedCluster verified (strict copy, no transformation).

**Integration tests:**
- Create a HostedCluster, verify `ControlPlaneUpgradeable` appears on HostedCluster status after the control plane is up.
- Simulate a data-plane-only `Upgradeable=False` (e.g., via a synthetic ClusterOperator not in the CP set) and verify that y-stream upgrades are not blocked.
- Simulate a CP-side `Upgradeable=False` (e.g., via the `network` ClusterOperator) and verify that y-stream upgrades are blocked.
- Deploy with an older HCCO (no `ControlPlaneUpgradeable`) and verify the HostedCluster controller falls back to `ClusterVersionUpgradeable`.
- Simulate guest cluster API unavailability and verify `ControlPlaneUpgradeable=Unknown`.
- Verify `ControlPlaneUpgradeable` propagation and `isUpgrading` gating through the legacy reconcile path (`reconcile_legacy.go`) for HostedClusters that have not migrated to the current reconcile path.

**e2e tests:**
- Perform a y-stream upgrade on a HostedCluster with all CP operators upgradeable and verify the upgrade completes.
- A CP-side operator reports `Upgradeable=False` → verify the y-stream upgrade does not proceed (core negative-path validation).
- A data-plane-only `Upgradeable=False` does not block a y-stream CP upgrade (core use case validation, as this is the primary motivating scenario).
- Verify `ControlPlaneUpgradeable` metric is exposed and reflects the condition value.

**CI lane:**
- Tests run in the existing HyperShift CI Prow jobs (`e2e-hypershift`). Feature-gated tests labeled with `[OCPFeatureGate:ControlPlaneUpgradeableCondition]` run in both `TechPreviewNoUpgrade` and `Default` Prow job variants. Tests are skipped in `Default` jobs until the feature gate is promoted, ensuring continuous coverage after promotion.
- Periodic jobs run at least once daily per supported management-cluster platform (AWS, Azure, GCP) for at least 14 days before branch cut, per feature promotion requirements.
- Results flow into Sippy and Component Readiness via the standard HyperShift periodic jobs.

## Graduation Criteria

### Dev Preview -> Tech Preview

- `ControlPlaneUpgradeable` condition is set on HCP and HostedCluster status behind `DevPreviewNoUpgrade` feature gate.
- `isUpgrading` does **not** consume the condition yet (informational only).
- Unit and integration tests pass, covering all scenarios listed in the Test Plan.
- `hypershift_hosted_cluster_control_plane_upgradeable` metric is exposed.
- Feature gate promoted to `TechPreviewNoUpgrade`: `isUpgrading` prefers `ControlPlaneUpgradeable` when available.

### Tech Preview -> GA

- At least 5 unique test scenarios in component readiness, minimum 14 runs per supported management-cluster platform, 95% pass rate. Supported management-cluster platforms: AWS (HA, amd64), Azure (HA, amd64), GCP (HA, amd64), matching the HyperShift CI job matrix. HyperShift management clusters are supported on AWS, Azure, and GCP only. vSphere, Baremetal, and AWS Single-node are not supported as HyperShift management cluster platforms. The testing matrix reflects the supported management cluster platforms, not the full standalone OCP platform list from `feature-zero-to-hero.md`.
- New periodic jobs run at least once daily for at least 14 days before branch cut.
- Validation in managed service environments (ROSA, ARO, GCP HCP) over at least one minor release cycle, covering: successful y-stream upgrades gated by `ControlPlaneUpgradeable`, data-plane-only blockers correctly bypassed, CP-side blockers correctly enforced.
- Confirmation that the CP operator list is complete and correctly categorized.
- `ControlPlaneUpgradeableBlocked` and `ControlPlaneUpgradeableUnknown` alerts defined and validated.
- Documentation of the condition in HostedCluster API reference (openshift-docs).
- Feature gate enabled by default.

### Removing a deprecated feature

Not applicable. This enhancement introduces a new condition; no existing feature is being deprecated or removed.

## Upgrade / Downgrade Strategy

**Upgrade**: When a newer CPO/HCCO starts reporting `ControlPlaneUpgradeable`, the HostedCluster controller (if also updated) begins using it for upgrade gating. If the HostedCluster controller is not yet updated, it ignores the new condition, so there is no behavioral change.

**Downgrade**: If the CPO/HCCO is downgraded to a version that does not report `ControlPlaneUpgradeable`, the condition disappears from HCP status. When `ControlPlaneUpgradeable` is absent from HCP status, the HostedCluster controller actively removes the condition from HostedCluster status (sets it to absent) on its next reconcile. This prevents stale `ControlPlaneUpgradeable=True` from persisting on HostedCluster status after downgrade, which would mislead fleet management tools. The HostedCluster controller then falls back to `ClusterVersionUpgradeable`, restoring the previous behavior.

**Mixed version**: During a rolling upgrade of the hypershift-operator, some HostedClusters may have the new behavior while others do not. This is safe because the fallback to `ClusterVersionUpgradeable` is strictly more conservative (it blocks on both CP and DP signals).

**EUS-to-EUS upgrades**: During sequential y-stream upgrades (e.g., 4.14 → 4.15 → 4.16), `ControlPlaneUpgradeable` is re-evaluated at each intermediate step. The HCCO is upgraded first as part of the CPO (which is part of the OCP release payload), so the updated CP operator list takes effect before the upgrade gating decision for each step. A new CP operator added in 4.18 will be in the 4.18 HCCO's set and checked during the 4.17 → 4.18 step.

**Migration from `force-upgrade-to`**: Service providers currently using the `force-upgrade-to` annotation routinely for y-stream upgrades should transition to the new `ControlPlaneUpgradeable`-based gating as follows:

1. **During rollout**: Continue using `force-upgrade-to` for clusters with an older HCCO that does not report `ControlPlaneUpgradeable`. The HostedCluster controller falls back to `ClusterVersionUpgradeable` for these clusters.
2. **Determining fleet readiness**: Service providers can query fleet rollout completeness using: `count(hypershift_hosted_cluster_control_plane_upgradeable{status="true"} or hypershift_hosted_cluster_control_plane_upgradeable{status="false"})`. This returns the number of HostedClusters where the condition is present and not Unknown. Compare against total HostedCluster count to determine rollout percentage.
3. **After fleet-wide rollout**: Once all HCPs run the new HCCO, service providers can stop applying `force-upgrade-to` on every y-stream upgrade. Managed service teams (ROSA, ARO, GCP HCP) must update their automation to check for `ControlPlaneUpgradeable` condition existence before deciding whether to apply `force-upgrade-to`. The annotation remains available as an escape hatch for cases where `ControlPlaneUpgradeable=False` is a false positive.
4. **Recommended approach**: Enable in a canary fleet first. Monitor `hypershift_hosted_cluster_control_plane_upgradeable` metrics for correctness over at least one full y-stream upgrade cycle before expanding. Minimum soak period: 2 weeks in canary before fleet-wide enablement.
5. **Interaction with `force-upgrade-to`**: The `force-upgrade-to` annotation continues to bypass **all** Upgradeable checks, including `ControlPlaneUpgradeable`. This behavior is unchanged.

## Version Skew Strategy

The `ControlPlaneUpgradeable` condition is produced by the HCCO (versioned with the CPO, which is part of the OCP release payload) and consumed by the hypershift-operator. The following skew scenarios are handled:

- **New HCCO + old hypershift-operator**: The hypershift-operator ignores the unknown condition. No behavioral change.
- **Old HCCO + new hypershift-operator**: The condition is absent. `isUpgrading` falls back to `ClusterVersionUpgradeable`. No behavioral change.
- **New HCCO + new hypershift-operator**: Full functionality.

**Rolling upgrade of hypershift-operator**: The hypershift-operator uses leader election; a single leader handles all HostedCluster reconciliation at any given time. During a rolling upgrade, the old pod relinquishes leadership and the new pod acquires it. There is no period where both old and new pods reconcile the same HostedCluster simultaneously, so the upgrade gating decision does not oscillate between `ClusterVersionUpgradeable` and `ControlPlaneUpgradeable`.

## Operational Aspects of API Extensions

This enhancement adds a status condition rather than a CRD, webhook, or aggregated API server. No new admission or conversion webhooks are introduced. The condition is computed from data already available in the HCCO's informer cache (ClusterOperator objects) and adds negligible compute/memory overhead. The hcpstatus controller reconciles on ClusterOperator watch events; with ~18 ClusterOperators, this adds at most ~18 additional reconcile triggers during bootstrap and rare subsequent updates. However, the condition is operationally significant because it gates y-stream upgrade behavior. See "Metrics and Alerting" for the SLIs and alerts that monitor condition health, and "Failure Modes" for the enumerated failure scenarios and their detection/mitigation strategies.

## Support Procedures

- **Detecting blocked upgrades**: If `ControlPlaneUpgradeable=False` on the HostedCluster, the condition message names the specific CP operator(s) that are not upgradeable and their reason/message. This directly identifies the remediation target.
  ```
  oc get hostedcluster <name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="ControlPlaneUpgradeable")]}'
  ```
- **Distinguishing CP vs DP blockers**: Compare `ControlPlaneUpgradeable` and `ClusterVersionUpgradeable`. If `ControlPlaneUpgradeable=True` but `ClusterVersionUpgradeable=False`, the block is from data-plane or user-installed operators only.
  ```
  oc get hostedcluster <name> -n <namespace> -o jsonpath='{range .status.conditions[?(@.type=="ControlPlaneUpgradeable" || @.type=="ClusterVersionUpgradeable")]}{.type}: {.status} - {.message}{"\n"}{end}'
  ```
- **Condition is Unknown**: If `ControlPlaneUpgradeable=Unknown`, the HCCO cannot determine CP upgrade readiness. Check the HCCO pod logs in the HCP namespace for structured events:
  ```
  # List failures (guest API unreachable):
  oc logs -n <hcp-namespace> deployment/control-plane-operator -c control-plane-operator | grep 'event=clusteroperator-list-failed'
  # Example: {"level":"error","logger":"hcpstatus","event":"clusteroperator-list-failed","error":"connection refused","msg":"failed to list ClusterOperators from guest cluster"}

  # Missing CP-side operator (quorum not met):
  oc logs -n <hcp-namespace> deployment/control-plane-operator -c control-plane-operator | grep 'event=clusteroperator-missing'
  # Example: {"level":"warning","logger":"hcpstatus","event":"clusteroperator-missing","operator":"network","msg":"CP-side ClusterOperator not found in guest cluster"}

  # Uncategorized operator (potential stale CP set):
  oc logs -n <hcp-namespace> deployment/control-plane-operator -c control-plane-operator | grep 'event=uncategorized-clusteroperator'
  # Example: {"level":"warning","logger":"hcpstatus","event":"uncategorized-clusteroperator","operator":"new-operator","msg":"ClusterOperator not in CP or HCCO-stubbed set"}
  ```
  Common causes: guest cluster API unavailable, bootstrap in progress, HCCO pod recently restarted, CP-side ClusterOperator deleted from guest cluster.
- **Checking condition staleness**: If the condition may be stale (e.g., after a network partition), check `lastTransitionTime`:
  ```
  oc get hostedcluster <name> -n <namespace> -o jsonpath='{.status.conditions[?(@.type=="ControlPlaneUpgradeable")].lastTransitionTime}'
  ```
  The HCCO heartbeats this timestamp on every successful ClusterOperator list. If `lastTransitionTime` is older than 10 minutes, `isUpgrading` treats the condition as `Unknown` and falls back to `ClusterVersionUpgradeable`.
- **Overriding**: The `force-upgrade-to` annotation and `controlPlaneRelease` override mechanisms remain unchanged and continue to bypass all Upgradeable checks.

## Infrastructure Needed

No new infrastructure is required.
