---
title: multi-cluster-hub-rule-management
authors:
  - "@sradco"
reviewers:
  - "@jacobbaungard"
  - "@jgbernalp"
  - "@simonpasquier"
approvers:
  - "@jacobbaungard"
  - "@jgbernalp"
api-approvers:
  - "@jacobbaungard"
  - "@jgbernalp"
creation-date: 2026-03-18
last-updated: 2026-11-09
tracking-link:
  - "https://issues.redhat.com/browse/CNV-46597"
  - "https://issues.redhat.com/browse/CNV-62972"
see-also:
  - "/enhancements/monitoring/multi-cluster-alerts-ui-management.md"
  - "/enhancements/monitoring/alerts-ui-management.md"
  - "/enhancements/monitoring/alert-rule-classification-mapping.md"
---
# Multi-Cluster Hub Rule Management

This proposal covers the management of hub alerting rules: creating, updating, and deleting rules evaluated by ThanosRuler on the hub over federated metrics.
It is Part 2 of the [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) umbrella proposal.

For shared context on the existing alert forwarding infrastructure, alert data storage lifecycle, and the ACM alerting developer preview, see the [Current State](multi-cluster-alerts-ui-management.md#current-state) section of the parent proposal.

## Summary

Hub alerting rules are evaluated by MCOA ThanosRuler over federated data from hub Thanos.
They produce alerts that are sent to the hub Alertmanager alongside spoke-forwarded alerts.
This proposal manages only `monitoring.coreos.com/v1` `PrometheusRule` resources.
It targets MCOA's Thanos Operator stack, whose `monitoring.thanos.io/v1alpha1` `ThanosRuler` selects rules using `spec.ruleConfigSelector`.
It does not require adoption of prometheus-operator's `ThanosRuler` CR.

The hub monitoring-plugin reuses the **same single-cluster Alerting API** (`/api/v1/alerting/*`) — there are no separate hub-specific endpoints. On the hub, the monitoring-plugin is configured to point to the hub Alertmanager, which receives both spoke-forwarded alerts and ThanosRuler alerts. The enrichment pipeline adds spoke rule metadata to produce a unified, classified fleet view through the same API surface.

The hub manages spoke rules via **direct Kubernetes API** through ManagedClusterProxy as the primary path — working on any managed cluster (OCP or non-OCP) with the PrometheusRule CRD. When the spoke Alerting API is available, it is used as an optimization for enriched operations. See the [parent proposal](multi-cluster-alerts-ui-management.md#hub-centric-spoke-management-via-direct-kube-api) for details.

> **Deployment prerequisite:** The hub must use MCOA's Thanos Operator stack with PrometheusRule discovery enabled.
> The legacy Observatorium-based ruler with ConfigMap-authored rules is not a supported target for hub rule management.
> Enabling and delivering the supported ACM observability stack is an ACM responsibility, not a controller migration implemented by this feature.

## Motivation

Fleet administrators need to define alerting rules that evaluate over federated data from multiple clusters on the hub's ThanosRuler.
Legacy hub deployments use ConfigMap-authored rules.
MCOA's Thanos Operator stack already supports PrometheusRule discovery; this proposal adds the API and UI management experience for those resources.

### User Stories

1. As an SRE, I want to create a global alerting rule on the hub that evaluates over federated metrics from all clusters, so I can detect fleet-wide conditions (e.g., more than N clusters have storage filling up).
2. As an admin, I want to view, update, and delete custom hub alerting rules via the same API and UI used for single-cluster rules, so I have a consistent management experience.
3. As an admin, I want hub alerting rules to be managed as `PrometheusRule` CRDs — the same resource type used on spoke clusters — so the management model is consistent across hub and spokes.

### Goals

1. Provide a CRUD API for hub alerting rules using `PrometheusRule` CRDs — consistent with the single-cluster Alerting API.
2. Define a clear ownership model: operator-managed defaults (read-only), user-created custom rules, and GitOps-managed rules.
3. Support hub alert classification labels for UI grouping and filtering.

### Non-Goals

- Detailed spoke rule management implementation (this document covers hub rules; spoke rules are managed via direct kube API through ManagedClusterProxy, or via the spoke Alerting API when available — see the [parent proposal](multi-cluster-alerts-ui-management.md#hub-centric-spoke-management-via-direct-kube-api) for the hub-centric spoke management architecture).
- Reading, creating, updating, or deleting ConfigMap-authored alert rules, including a compatibility API or conversion of legacy rules to PrometheusRules.
- Implementing AlertRelabelConfig support for ACM predefined hub rules. ACM observability may add that support independently; it is not a dependency of this feature.
- Hub rule disable mechanism. This proposal does not add a hub ARC pipeline; users can silence hub alerts or delete custom rules.
- Notification routing configuration for hub Alertmanager.

## Proposal

### Hub Rule Management

The supported hub evaluator is Thanos Operator's `ThanosRuler`, not prometheus-operator's similarly named resource.
MCOA creates a ruler named `mcoa` in `open-cluster-management-observability`.
The relevant ruler configuration is:

```yaml
apiVersion: monitoring.thanos.io/v1alpha1
kind: ThanosRuler
metadata:
  name: mcoa
  namespace: open-cluster-management-observability
spec:
  ruleConfigSelector:
    matchLabels:
      app.kubernetes.io/part-of: multicluster-observability-addon
```

This is a configuration excerpt, not a complete ruler manifest. The Alerting API does not create or modify the ruler.
In the current MCOA implementation, this stack is enabled by the `mcoa-thanos-operator: "true"` annotation on the applicable `AddOnDeploymentConfig`.
MCOA deploys Thanos Operator with `--enable-feature=prometheus-rule`.
These are deployment prerequisites, not evidence that every ACM installation already runs this stack.

#### PrometheusRule discovery contract

The hub API creates and manages `monitoring.coreos.com/v1` `PrometheusRule` resources in the ruler's namespace,
normally `open-cluster-management-observability`. The current controller does not discover rules across namespaces.
Each managed hub PrometheusRule must carry both of these **resource metadata labels**:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
metadata:
  name: example-hub-alerts
  namespace: open-cluster-management-observability
  labels:
    app.kubernetes.io/part-of: multicluster-observability-addon
    operator.thanos.io/prometheus-rule: "true"
```

This excerpt shows the resource identity and discovery metadata; alert groups are supplied in `spec.groups`.
The first label matches MCOA's `spec.ruleConfigSelector`; the second is a required label added to the effective selector by Thanos Operator.
These are not labels on individual alert definitions.

- Create operations add both metadata labels. Updates preserve them and reject changes that would remove the rule from the supported ruler's selection.
- Hub rule listing uses the same API group, namespace, and effective selector so it does not present unrelated PrometheusRules as hub-evaluated rules.
- API validation and ownership checks apply before writes. CRUD success alone does not establish that the ruler has loaded or evaluated a rule.
- If the required stack or discovery capability is unavailable, hub rule management reports that it is unsupported rather than falling back to ConfigMap writes.

Thanos Operator converts selected PrometheusRules into operator-managed ConfigMaps containing rule files for the evaluator.
Those generated ConfigMaps are an internal implementation detail, not an API source of truth.
The Alerting API neither parses nor edits them, and it does not support legacy ConfigMap-authored rules.
This restriction concerns rule management; alerts already delivered to hub Alertmanager remain visible through the alert read path regardless of their rule storage.

Implementation references:
- [MCOA ruler construction and selector](https://github.com/stolostron/multicluster-observability-addon/blob/cfdf4ae9234f50c6a07f6cccf1955e599282eb79/internal/metrics/thanos/objects.go)
- [MCOA operator feature flags](https://github.com/stolostron/multicluster-observability-addon/blob/cfdf4ae9234f50c6a07f6cccf1955e599282eb79/internal/addon/manifests/charts/mcoa/charts/metrics/templates/thanos-operator/operator/thanos-operator-deployment.yaml)
- [PrometheusRule discovery in MCOA's pinned Thanos Operator](https://github.com/thanos-community/thanos-operator/blob/1a9a0ab18a9f/internal/controller/thanosruler_controller.go)

#### Default hub alerts: cluster not reporting

The legacy hub default rules include two related alerts that use `acm_managed_cluster_labels` as the authoritative cluster inventory:

- **`ManagedClusterMetricsMissing`** — detects managed clusters whose observability addon is available but no metrics are being received. Per-cluster alert (has `cluster` label), appears in the Fleet Health Heatmap.
- **`ObservabilityHubInventoryMissing`** — detects when `acm_managed_cluster_labels` itself is absent, indicating MCE's metrics pipeline is broken.

These predefined rules are exposed in the rule-management API only when ACM observability supplies them as PrometheusRules selected by the supported hub ruler.
The feature does not convert their existing ConfigMap definitions or assume that such conversion has already happened.
Providing predefined rules as PrometheusRules is owned by ACM observability.

#### Hub rule ownership model

The API uses `PrometheusRule` CRDs — the same CRD used on spoke clusters. This reuses the same API code and CRD patterns as the single-cluster Alerting API and provides per-rule ownership, GitOps annotations, and optimistic concurrency.

- ACM-provided default PrometheusRules are read-only and identified through operator ownership metadata (`managedBy: operator` and ownerReferences).
- `POST /alerting/rules` creates a PrometheusRule in `open-cluster-management-observability` with both discovery labels for custom hub rules.
- `GET /alerting/rules` lists hub PrometheusRules matching the discovery contract (ACM-provided defaults, where available, and custom rules).
  The Rule Metadata Cache adds spoke rule metadata to the unified fleet view. ConfigMap-authored rules are not included in this rule-management view.
- `PATCH /alerting/rules` updates a hub `PrometheusRule` CRD. Blocked for operator default rules.
- `DELETE /alerting/rules` deletes a hub `PrometheusRule` CRD. Blocked for defaults.
- The `alertRuleId` hash inputs for hub rules are: `kind` (alert/record) + alertname + PromQL expression + `for` duration + sorted business label key-value pairs (excluding `openshift_io_*` and `alertname`). This is consistent with the single-cluster approach defined in the [alert-rule-classification-mapping](alert-rule-classification-mapping.md) enhancement, where the canonical payload is `kind + name + expr + for + labels_block` and the ID is `rid_<base64url(SHA256(canonical_payload))>`. Annotations are excluded from the hash, so annotation-only changes do not alter the rule ID.
- Ownership detection: `PrometheusRule` CRDs use standard per-resource labels and annotations (`managedBy: operator` with ownerReferences for defaults, `managedBy: ""` for unmanaged, ArgoCD annotations for GitOps-managed).
- Per-rule optimistic concurrency: each `PrometheusRule` CRD has its own `resourceVersion`.
- Per-rule GitOps: individual `PrometheusRule` CRDs can have ArgoCD annotations.

#### Hub rule tiers in the API

| Tier | Source | `managedBy` | Create | Update | Delete |
|------|--------|-------------|--------|--------|--------|
| Default hub rules | `PrometheusRule` CRD (operator-owned) | `operator` | N/A (operator creates) | Blocked | Blocked |
| Custom hub rules (unmanaged) | `PrometheusRule` CRD | `""` | `POST /alerting/rules` | `PATCH /alerting/rules` | `DELETE /alerting/rules` |
| Custom hub rules (GitOps) | `PrometheusRule` CRD with ArgoCD annotations | `gitops` | N/A | Blocked — guidance to edit in Git | Blocked |

#### Hub rule disable mechanism

This feature does not implement a hub ARC (AlertRelabelConfig) pipeline or the spoke-side `action: Drop` mechanism for hub rules.
Individual hub-rule disabling is not offered by this proposal. ACM observability may independently add ARC support for its predefined hub rules;
this proposal neither implements nor depends on that work. Until supported separately:

- To suppress notifications from a hub alert, users create a **silence** on the hub Alertmanager targeting that alert (already supported today via the Alertmanager API and UI).
- To replace a default hub rule's behavior, users create a **custom hub rule** with the desired expression/thresholds via `POST /alerting/rules` and silence the original default alert.
- Custom hub rules that are no longer needed can be **deleted** via `DELETE /alerting/rules`.

#### Hub alert classification labels

Classification labels (`openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`) exist primarily for **UI grouping and filtering** — they enable the console to organize alerts by component and impact layer across the fleet. They are not consumed by Alertmanager for routing or notification purposes.

On spokes, classification works at two levels (see the [alert-rule-classification-mapping](alert-rule-classification-mapping.md) enhancement for full detail):
1. **ARC-stamped labels on alerts**: When a classification override or severity change is applied via the Alerting API, an `AlertRelabelConfig` CR is created. ARCs do not change the alerting rules evaluated by Prometheus — they modify the alert labels sent to Alertmanager. Prometheus applies ARC relabel configs globally before dispatching alerts to any Alertmanager, so these labels (`openshift_io_alert_rule_id`, `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`) are present on alerts sent to hub AM via `additionalAlertmanagerConfigs`. Only alerts with an associated ARC (or explicit classification labels in their rule definition) have these labels.
2. **Server-side classifier enrichment**: The monitoring-plugin backend computes `alertComponent` and `alertLayer` as additive enrichment fields for ALL rules using classifier matchers, rule-scoped default labels, and ARC overrides. These fields are returned by the Alerting API (`GET /alerting/rules`, `GET /alerting/alerts`) but are not labels on the alert itself.

The hub-side classifier (`matcher.go`) computes classification for hub alerts locally using the same logic as spokes.
This proposal does not add hub-side ARC processing; classification remains available without it.

**ACM predefined rules exposed as PrometheusRules:**

ACM observability owns these definitions. The Alerting API treats them as read-only and does not modify their labels.
For MVP, the console backend uses its classifier mapping of known default rule alertnames to compute component and layer metadata.
This is API enrichment and does not add labels to alerts delivered to Alertmanager.

ACM observability may add AlertRelabelConfig support for its predefined hub rules independently. That implementation and its delivery timeline are out of scope.

**User-created custom PrometheusRules:**

When users create hub rules via `POST /alerting/rules`, the API accepts `component` and `layer` as optional metadata and writes them as labels in the `PrometheusRule` CRD's rule definition — the same convention used for single-cluster user-defined rules. ThanosRuler evaluates the rule and the resulting alert carries these labels natively to hub AM. Users are expected to include classification labels in their rule definitions, consistent with how it is done on single-cluster.

### API Endpoints — Unified Single-Cluster API

Base path: `/api/v1/alerting`

> Note: GET endpoints should remain compatible with upstream Thanos/Prometheus (query parameters and response schemas) wherever applicable to enable native Perses integration.

The hub monitoring-plugin reuses the **same Alerting API** used on single clusters. The monitoring-plugin on the hub is configured to point to the hub Alertmanager as its Alertmanager backend. This means:

- **`GET /alerting/alerts`** returns alerts from hub Alertmanager — which includes both spoke-forwarded alerts (with `managed_cluster` label) and hub ThanosRuler alerts. The enrichment pipeline (classifier + Rule Metadata Cache) adds classification and rule metadata from spoke clusters, exactly as described in the [Alert Visualization](multi-cluster-alerts-visualization.md) proposal.
- **`GET /alerting/rules`** returns hub PrometheusRules matching the discovery contract, including ACM-provided defaults where available and custom rules.
  The Rule Metadata Cache adds spoke metadata, using the spoke Alerting API through ManagedClusterProxy where available, or direct PrometheusRule reads otherwise.
  Both paths feed the unified rule view; ConfigMap-authored hub rules are excluded.
- **`POST /alerting/rules`** creates a hub PrometheusRule in the supported ruler's namespace with both required metadata labels.
- **`PATCH /alerting/rules`** updates a hub rule (only custom/unmanaged; blocked for operator-managed and GitOps-managed).
- **`DELETE /alerting/rules`** deletes a hub rule (only custom/unmanaged).

Multi-cluster extensions are handled via query parameters on the same endpoints — for example, a `cluster` filter on `GET /alerting/alerts` selects alerts by `managed_cluster` label. Omitting it returns alerts from all clusters plus hub alerts.

- Semantics:
  - Request/response schemas are identical to the single-cluster Alerting API and include `ruleId`, `labels`, `annotations`, `mapping` (component/impactGroup), `source` (`platform`|`user`|`hub`), `managedBy` (`operator`|`gitops`|`""`), and `disabled` flags.
  - Shared response fields do not imply identical write capabilities. Hub rules have no disable operation in this proposal;
    the UI must not offer a disable action based solely on the presence of the `disabled` field.
  - For spoke alerts, the enrichment pipeline adds rule metadata (`alertRuleId`, `source`, `managedBy`, `prometheusRuleName`) from the Rule Metadata Cache — populated by calling each spoke's Alerting API via ManagedClusterProxy.
  - Read paths keep Prometheus/Thanos compatibility where applicable.

This approach has several advantages:
1. **No separate API surface** — the same endpoints, schemas, and client code work for single-cluster and multi-cluster. The console frontend calls the same API regardless of scope.
2. **Consistency** — the hub monitoring-plugin behaves like any spoke monitoring-plugin, just backed by hub AM which aggregates all alerts.
3. **Natural enrichment** — the hub instance adds spoke rule metadata as additive enrichment fields on the same response schema.

#### Implementation impact (ACM observability integration)

Use MCOA's existing Thanos Operator stack and its PrometheusRule discovery contract.
ACM observability owns enabling that stack and supplying predefined rules as PrometheusRules where they should appear in rule management.
The Alerting API operates entirely on PrometheusRules; it does not migrate legacy rules, manage ConfigMap rule files, or implement ARC support for ACM predefined hub rules.

#### Implementation impact (monitoring-plugin on hub)

The hub monitoring-plugin is deployed with the same Alerting API code as on spokes, configured to use hub Alertmanager as its backend. The key differences in the hub deployment:

1. **Alertmanager backend**: points to hub AM (via the `--alertmanager` flag or `UIPlugin` CR), which receives spoke-forwarded alerts and ThanosRuler alerts.
2. **PrometheusRule target**: operates on `monitoring.coreos.com/v1` PrometheusRules in `open-cluster-management-observability`,
   using both discovery labels described above for hub rule CRUD.
3. **Spoke rule management**: manages spoke `PrometheusRule` CRDs via direct kube API through ManagedClusterProxy (primary path, works on all cluster types). Uses spoke Alerting API when available for validated writes and classification cache.
4. **Rule Metadata Cache**: fetches rule metadata from spoke clusters via ManagedClusterProxy — using the spoke Alerting API where available, falling back to direct `PrometheusRule` CRD reads for older OCP and non-OCP spokes.
5. **Classifier**: the hub-side classifier (`matcher.go`) computes classification for all alerts locally — same logic as spokes, no fan-out needed.
6. **Multi-cluster filters**: the same API accepts additional query parameters (`cluster`, `cluster_labels`) for filtering by `managed_cluster` label and cluster metadata.
7. **Cluster capability detection**: probes each managed cluster to determine available features (PrometheusRule CRD, ARC CRD, Alerting API, monitoring namespace) and adapts behavior accordingly.

### GitOps / Argo CD Compliance

- Hub rule resources generated by this feature are declarative PrometheusRules suitable for committing to Git.
  Spoke ARC resources follow the single-cluster model; this proposal does not generate ARCs for ACM predefined hub rules.
- Custom hub rules are `PrometheusRule` CRDs, so per-rule ArgoCD annotations are supported for fine-grained GitOps ownership — consistent with how spoke rules are managed. GitOps-managed `PrometheusRule` CRDs are treated as read-only by the API.

### Workflow Description

TBD.

### API Extensions

TBD.

### Topology Considerations

#### Hypershift / Hosted Control Planes

TBD.

#### Standalone Clusters

TBD.

#### Single-node Deployments or MicroShift

TBD.

#### OpenShift Kubernetes Engine

TBD.

### Implementation Details/Notes/Constraints

TBD.

### Risks and Mitigations

See Risks & Mitigations below.

### Drawbacks

TBD.

## Risks & Mitigations

- **Scope and precedence complexity**: hub vs cluster rule precedence must be deterministic. Provide clear scope indicators and UI guardrails. Document and enforce a single precedence policy in the API.
- **RBAC and ownership across clusters**: enforce hub and per-cluster RBAC. Treat GitOps-owned resources as read-only.
- **Deployment and discovery mismatch**: hub rule management requires the supported MCOA Thanos Operator stack and both discovery labels.
  Detect unsupported deployments explicitly and verify that created PrometheusRules are loaded and evaluated in end-to-end tests.
- **Legacy rule visibility**: ConfigMap-authored rules are outside rule-management scope. Do not imply that the PrometheusRule list includes every legacy hub rule.
  ACM observability owns providing predefined rules as PrometheusRules.

## Open Questions

- **Supported ACM versions and deployment readiness:** Which ACM release/configuration provides the Thanos Operator stack and PrometheusRule discovery contract required here?
  Confirm with ACM observability, including availability of predefined rules as PrometheusRules. No prometheus-operator ThanosRuler migration is required by this proposal.

- **Non-OCP spoke classification persistence:** Non-OCP clusters lack the AlertRelabelConfig CRD. When the hub creates or updates classification labels on spoke rules, options are: (a) embed labels directly in PrometheusRule annotations, (b) maintain a hub-side ConfigMap mapping, (c) write labels into the PrometheusRule `labels` field. Need to decide the universal approach and whether it should also replace ARC on OCP for consistency.

- **Monitoring namespace per spoke:** Non-OCP clusters don't use `openshift-monitoring`. The hub needs to know where each spoke's monitoring stack lives. Options: ManagedCluster annotation, auto-detection via kube API, or hub-side per-cluster configuration.

- **`management.Client` backend abstraction:** The hub needs two spoke management backends: "direct kube API" (works everywhere) and "via spoke Alerting API" (optimization when available). Need to define the interface boundary and whether maintaining two backends is justified long-term.

## Alternatives (Not Implemented)

TBD.

## Test Plan

Test hub rule management against MCOA's supported Thanos Operator deployment:

- Create a hub PrometheusRule through the API and verify its API group, namespace, both discovery labels, and eventual loading and evaluation by the ruler.
- Update and delete an unmanaged rule; verify the ruler applies the changes. Reject updates that remove required discovery labels.
- Verify that rules in other namespaces or without the effective selector labels are not listed as managed hub rules.
- Verify operator-owned and GitOps-owned PrometheusRules remain read-only.
- Verify legacy rule ConfigMaps and operator-generated rule-file ConfigMaps are never parsed or modified by rule-management operations.
- Verify an unsupported hub deployment produces an explicit unsupported-capability response rather than a successful-looking write or a ConfigMap fallback.
- Verify custom rule CRUD, classification enrichment, and hub silences work without ARC support for ACM predefined rules; do not expose a hub disable action.

## Graduation Criteria

TBD.

### Dev Preview -> Tech Preview

TBD.

### Tech Preview -> GA

TBD.

### Removing a deprecated feature

TBD.

## Upgrade / Downgrade Strategy

TBD.

## Version Skew Strategy

TBD.

## Operational Aspects of API Extensions

TBD.

## Support Procedures

TBD.
