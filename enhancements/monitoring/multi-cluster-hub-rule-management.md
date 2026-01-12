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
last-updated: 2026-04-09
tracking-link:
  - "https://issues.redhat.com/browse/CNV-46597"
  - "https://issues.redhat.com/browse/CNV-62972"
see-also:
  - "/enhancements/monitoring/multi-cluster-alerts-ui-management.md"
  - "/enhancements/monitoring/alerts-ui-management.md"
  - "/enhancements/monitoring/alert-rule-classification-mapping.md"
---
# Multi-Cluster Hub Rule Management

This proposal covers the management of hub alerting rules — creating, updating, disabling, and deleting rules evaluated by ThanosRuler on the hub over federated metrics. It is Part 2 of the [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) umbrella proposal.

For shared context on the existing alert forwarding infrastructure, alert data storage lifecycle, and the ACM alerting developer preview, see the [Current State](multi-cluster-alerts-ui-management.md#current-state) section of the parent proposal.

## Summary

Hub alerting rules are evaluated by MCOA ThanosRuler over federated data from hub Thanos. They produce alerts that are sent to the hub Alertmanager alongside spoke-forwarded alerts. This proposal defines the API and storage model for managing these rules using `PrometheusRule` CRDs — the same CRD used on spoke clusters — requiring MCO to adopt prometheus-operator's `ThanosRuler` CR with `ruleSelector`.

The hub monitoring-plugin reuses the **same single-cluster Alerting API** (`/api/v1/alerting/*`) — there are no separate hub-specific endpoints. On the hub, the monitoring-plugin is configured to point to the hub Alertmanager, which receives both spoke-forwarded alerts and ThanosRuler alerts. The enrichment pipeline adds spoke rule metadata (from each spoke's Alerting API via ManagedClusterProxy) to produce a unified, classified fleet view through the same API surface.

> **Prerequisite — MCO migration required:** This proposal depends on MCO migrating from the Observatorium-based ThanosRuler deployment to prometheus-operator's `ThanosRuler` CR with `ruleSelector`. Until that migration is complete, hub rule management via the Alerting API cannot ship. See [Open Questions](#open-questions) for timeline considerations.

## Motivation

Fleet administrators need to define alerting rules that evaluate over federated data from multiple clusters on the hub's ThanosRuler. Today, hub rules are managed via raw ConfigMap edits with no API — inconsistent with the single-cluster experience where rules are managed as `PrometheusRule` CRDs via the Alerting API.

### User Stories

1. As an SRE, I want to create a global alerting rule on the hub that evaluates over federated metrics from all clusters, so I can detect fleet-wide conditions (e.g., more than N clusters have storage filling up).
2. As an admin, I want to view, update, and delete custom hub alerting rules via the same API and UI used for single-cluster rules, so I have a consistent management experience.
3. As an admin, I want hub alerting rules to be managed as `PrometheusRule` CRDs — the same resource type used on spoke clusters — so the management model is consistent across hub and spokes.

### Goals

1. Provide a CRUD API for hub alerting rules using `PrometheusRule` CRDs — consistent with the single-cluster Alerting API.
2. Define a clear ownership model: operator-managed defaults (read-only), user-created custom rules, and GitOps-managed rules.
3. Support hub alert classification labels for UI grouping and filtering.

### Non-Goals

- Spoke rule management (this document covers hub rules only; spoke rules are managed via the existing single-cluster Alerting API proxied through ManagedClusterProxy).
- Hub rule disable mechanism (users silence hub alerts or create replacement rules; see [Open Questions](#open-questions)).
- Notification routing configuration for hub Alertmanager.

## Proposal

### Hub Rule Management

Hub alerting rules are evaluated by MCOA ThanosRuler over federated data from hub Thanos. Today, ThanosRuler is deployed via the Observatorium operator, which only supports ConfigMap-based rule files. This proposal requires MCO to adopt prometheus-operator's `ThanosRuler` CRD, which natively supports selecting `PrometheusRule` CRDs via `ruleSelector` — the same pattern used by the `Prometheus` CR on spokes. This provides a consistent approach between single-cluster and multi-cluster rule management.

#### Current hub rule storage

| ConfigMap | Key | Content | Ownership |
|-----------|-----|---------|-----------|
| `thanos-ruler-default-rules` | `default_rules.yaml` | Default alerts (KubePersistentVolumeFillingUp, ViolatedPolicyReport, Thanos health) and recording rules (cluster aggregations for Grafana). Always present. | MCO operator — read-only, operator-managed. The operator overwrites on reconciliation. |
| `thanos-ruler-custom-rules` | `custom_rules.yaml` | User-defined custom alerting and recording rules. Created on demand. MCO controller watches for updates. | User-managed (direct ConfigMap edit) or GitOps-managed (ArgoCD manages the ConfigMap). |

#### Default hub alerts: cluster not reporting

Two related alerts already exist in the hub default rules (`thanos-ruler-hub-metrics-rules` ConfigMap) and use `acm_managed_cluster_labels` as the authoritative cluster inventory:

- **`ManagedClusterMetricsMissing`** — detects managed clusters whose observability addon is available but no metrics are being received. Per-cluster alert (has `cluster` label), appears in the Fleet Health Heatmap.
- **`ObservabilityHubInventoryMissing`** — detects when `acm_managed_cluster_labels` itself is absent, indicating MCE's metrics pipeline is broken.

When MCO adopts prometheus-operator's `ThanosRuler` CR, both move from ConfigMap entries to operator-managed `PrometheusRule` CRDs (`managedBy: operator`).

#### Hub rule ownership model

The API uses `PrometheusRule` CRDs — the same CRD used on spoke clusters. This reuses the same API code and CRD patterns as the single-cluster Alerting API and provides per-rule ownership, GitOps annotations, and optimistic concurrency.

**Prerequisite — MCO adopts prometheus-operator `ThanosRuler` CR:**

MCO migrates from Observatorium-based ThanosRuler to prometheus-operator's `ThanosRuler` CR with `ruleSelector`. This enables ThanosRuler to read `PrometheusRule` CRDs directly — no ConfigMap bridge needed.

- The MCO operator creates `PrometheusRule` CRDs for default rules (with `managedBy: operator` label and ownerReferences) instead of writing ConfigMaps directly
- `POST /alerting/rules` creates a `PrometheusRule` CRD in `open-cluster-management-observability` for custom hub rules
- `GET /alerting/rules` lists hub `PrometheusRule` CRDs (operator defaults + user custom) and, via the Rule Metadata Cache, spoke rule metadata — providing a unified fleet view through the same API
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

Hub ThanosRuler has no ARC (AlertRelabelConfig) pipeline, so there is no equivalent of the spoke-side `action: Drop` mechanism. Disabling individual hub rules is not supported. Instead:

- To suppress notifications from a hub alert, users create a **silence** on the hub Alertmanager targeting that alert (already supported today via the Alertmanager API and UI).
- To replace a default hub rule's behavior, users create a **custom hub rule** with the desired expression/thresholds via `POST /alerting/rules` and silence the original default alert.
- Custom hub rules that are no longer needed can be **deleted** via `DELETE /alerting/rules`.

#### Hub alert classification labels

Classification labels (`openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`) exist primarily for **UI grouping and filtering** — they enable the console to organize alerts by component and impact layer across the fleet. They are not consumed by Alertmanager for routing or notification purposes.

On spokes, classification works at two levels (see the [alert-rule-classification-mapping](alert-rule-classification-mapping.md) enhancement for full detail):
1. **ARC-stamped labels on alerts**: When a classification override or severity change is applied via the Alerting API, an `AlertRelabelConfig` CR is created. ARCs do not change the alerting rules evaluated by Prometheus — they modify the alert labels sent to Alertmanager. Prometheus applies ARC relabel configs globally before dispatching alerts to any Alertmanager, so these labels (`openshift_io_alert_rule_id`, `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`) are present on alerts sent to hub AM via `additionalAlertmanagerConfigs`. Only alerts with an associated ARC (or explicit classification labels in their rule definition) have these labels.
2. **Server-side classifier enrichment**: The monitoring-plugin backend computes `alertComponent` and `alertLayer` as additive enrichment fields for ALL rules using classifier matchers, rule-scoped default labels, and ARC overrides. These fields are returned by the Alerting API (`GET /alerting/rules`, `GET /alerting/alerts`) but are not labels on the alert itself.

The hub-side classifier (`matcher.go`) computes classification for hub alerts locally using the same logic as spokes. Hub ThanosRuler has no ARC pipeline, so hub alerts arrive at hub AM with only the labels explicitly defined in the rule.

**Operator default rules (`thanos-ruler-default-rules`):**

The MCO operator owns these rules and overwrites them on every reconciliation. Adding classification labels directly to the rule definitions would be overwritten. For MVP, the console backend maintains a static mapping of known default rule alertnames to their classification (`component`, `layer`). This is acceptable because the default rule set is small and changes only across operator upgrades.

For a future iteration, support for `alertRelabelConfigs` on ThanosRuler should be added so that the MCO operator can configure classification labels for default rules via relabeling — the same mechanism used for the user-defined monitoring stack. This has already been requested for the user-defined monitoring stack as well.

**User-created custom rules (`thanos-ruler-custom-rules`):**

When users create hub rules via `POST /alerting/rules`, the API accepts `component` and `layer` as optional metadata and writes them as labels in the `PrometheusRule` CRD's rule definition — the same convention used for single-cluster user-defined rules. ThanosRuler evaluates the rule and the resulting alert carries these labels natively to hub AM. Users are expected to include classification labels in their rule definitions, consistent with how it is done on single-cluster.

### API Endpoints — Unified Single-Cluster API

Base path: `/api/v1/alerting`

> Note: GET endpoints should remain compatible with upstream Thanos/Prometheus (query parameters and response schemas) wherever applicable to enable native Perses integration.

The hub monitoring-plugin reuses the **same Alerting API** used on single clusters. The monitoring-plugin on the hub is configured to point to the hub Alertmanager as its Alertmanager backend. This means:

- **`GET /alerting/alerts`** returns alerts from hub Alertmanager — which includes both spoke-forwarded alerts (with `managed_cluster` label) and hub ThanosRuler alerts. The enrichment pipeline (classifier + Rule Metadata Cache) adds classification and rule metadata from spoke clusters, exactly as described in the [Alert Visualization](multi-cluster-alerts-visualization.md) proposal.
- **`GET /alerting/rules`** returns hub alerting rules from `PrometheusRule` CRDs in the hub namespace — both operator-owned defaults and user-created custom rules. Additionally, the Rule Metadata Cache provides rule metadata from spoke clusters (fetched via `GET /alerting/rules` on each spoke through ManagedClusterProxy), so the hub can return a unified rule view across the fleet.
- **`POST /alerting/rules`** creates a hub alerting rule (creates a `PrometheusRule` CRD in the hub namespace).
- **`PATCH /alerting/rules`** updates a hub rule (only custom/unmanaged; blocked for operator-managed and GitOps-managed).
- **`DELETE /alerting/rules`** deletes a hub rule (only custom/unmanaged).

Multi-cluster extensions are handled via query parameters on the same endpoints — for example, a `cluster` filter on `GET /alerting/alerts` selects alerts by `managed_cluster` label. Omitting it returns alerts from all clusters plus hub alerts.

- Semantics:
  - Request/response schemas are identical to the single-cluster Alerting API and include `ruleId`, `labels`, `annotations`, `mapping` (component/impactGroup), `source` (`platform`|`user`|`hub`), `managedBy` (`operator`|`gitops`|`""`), and `disabled` flags.
  - For spoke alerts, the enrichment pipeline adds rule metadata (`alertRuleId`, `source`, `managedBy`, `prometheusRuleName`) from the Rule Metadata Cache — populated by calling each spoke's Alerting API via ManagedClusterProxy.
  - Read paths keep Prometheus/Thanos compatibility where applicable.

This approach has several advantages:
1. **No separate API surface** — the same endpoints, schemas, and client code work for single-cluster and multi-cluster. The console frontend calls the same API regardless of scope.
2. **Consistency** — the hub monitoring-plugin behaves like any spoke monitoring-plugin, just backed by hub AM which aggregates all alerts.
3. **Natural enrichment** — the hub instance adds spoke rule metadata as additive enrichment fields on the same response schema.

#### Implementation impact (MCO adoption)

This proposal requires MCO to migrate from the Observatorium-based ThanosRuler deployment to prometheus-operator's `ThanosRuler` CR with `ruleSelector` for `PrometheusRule` CRDs. As part of this migration, the MCO operator creates `PrometheusRule` CRDs for default rules (instead of writing ConfigMaps directly). The Alerting API then operates entirely on `PrometheusRule` CRDs — no ConfigMap parsing needed.

#### Implementation impact (monitoring-plugin on hub)

The hub monitoring-plugin is deployed with the same Alerting API code as on spokes, configured to use hub Alertmanager as its backend. The key differences in the hub deployment:

1. **Alertmanager backend**: points to hub AM (via the `--alertmanager` flag or `UIPlugin` CR), which receives spoke-forwarded alerts and ThanosRuler alerts.
2. **PrometheusRule target namespace**: operates on `PrometheusRule` CRDs in `open-cluster-management-observability` (hub namespace) for hub rule CRUD.
3. **Rule Metadata Cache**: additionally fetches rule metadata from spoke clusters via ManagedClusterProxy (`GET /alerting/rules` on each spoke) to enrich hub alerts with spoke rule context.
4. **Classifier**: the hub-side classifier (`matcher.go`) computes classification for all alerts locally — same logic as spokes, no fan-out needed.
5. **Multi-cluster filters**: the same API accepts additional query parameters (`cluster`, `cluster_labels`) for filtering by `managed_cluster` label and cluster metadata.

### GitOps / Argo CD Compliance

- All generated `PrometheusRule` and `AlertRelabelConfig` resources remain declarative and suitable for committing to Git.
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
- **MCO migration dependency**: this proposal requires MCO to adopt prometheus-operator's `ThanosRuler` CR. Timeline depends on MCO team capacity. Hub rule management cannot ship until this migration is complete.

## Open Questions

- **MCO migration timeline:** This proposal requires MCO to adopt prometheus-operator's `ThanosRuler` CR with `ruleSelector`. Timeline depends on MCO team capacity and willingness to migrate from the Observatorium-based deployment. Should be resolved during design review with MCO team.

- **Hub rule disable mechanism:** The current design does not support disabling individual hub rules — users silence hub alerts and create custom replacement rules instead. Should we also support `alertRelabelConfigs` on ThanosRuler to enable ARC-like relabeling and `Drop` capabilities for hub alerts — the same mechanism used on spokes? This would provide a consistent disable/relabel model across hub and spokes, but requires ThanosRuler to support `alertRelabelConfigs` (which may need upstream work or MCO support). Alternatively, silencing may be sufficient for hub alerts given the small default rule set. Needs design discussion.

## Alternatives (Not Implemented)

TBD.

## Test Plan

TBD.

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

