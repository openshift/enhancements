---
title: multi-cluster-hub-rule-management
authors:
  - "@sradco"
reviewers:
  - "@jan--f"
  - "@jgbernalp"
  - "@simonpasquier"
approvers:
  - "@jan--f"
  - "@jgbernalp"
api-approvers:
  - TBD
creation-date: 2026-03-18
last-updated: 2026-03-18
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

## Table of Contents

- [Summary](#summary)
- [Hub Rule Management](#hub-rule-management)
  - [Current hub rule storage](#current-hub-rule-storage)
  - [Hub rule ownership model](#hub-rule-ownership-model)
  - [Hub rule tiers in the API](#hub-rule-tiers-in-the-api)
  - [Hub rule disable mechanism](#hub-rule-disable-mechanism)
  - [Hub alert classification labels](#hub-alert-classification-labels)
- [API Endpoints](#api-endpoints)
- [GitOps / Argo CD Compliance](#gitops--argo-cd-compliance)
- [Risks & Mitigations](#risks--mitigations)
- [Open Questions](#open-questions)

## Summary

Hub alerting rules are evaluated by MCOA ThanosRuler over federated data from hub Thanos. They produce alerts that are sent to the hub Alertmanager alongside spoke-forwarded alerts. This proposal defines the API and storage model for managing these rules, covering both the MVP approach (direct ConfigMap manipulation) and the future CRD-based approach.

## Hub Rule Management

Hub alerting rules are evaluated by MCOA ThanosRuler over federated data from hub Thanos. ThanosRuler uses ConfigMap-based rule files, not PrometheusRule CRDs.

### Current hub rule storage

| ConfigMap | Key | Content | Ownership |
|-----------|-----|---------|-----------|
| `thanos-ruler-default-rules` | `default_rules.yaml` | Default alerts (KubePersistentVolumeFillingUp, ViolatedPolicyReport, Thanos health) and recording rules (cluster aggregations for Grafana). Always present. | MCO operator — read-only, operator-managed. The operator overwrites on reconciliation. |
| `thanos-ruler-custom-rules` | `custom_rules.yaml` | User-defined custom alerting and recording rules. Created on demand. MCO controller watches for updates. | User-managed (direct ConfigMap edit) or GitOps-managed (ArgoCD manages the ConfigMap). |

### Hub rule ownership model

Unlike spoke rules (where each PrometheusRule CRD has its own labels, annotations, and ownerReferences for per-rule ownership detection), hub custom rules are packed into a single ConfigMap with no per-rule metadata. This creates a mismatch with the single-cluster Alerting API's ownership model.

**MVP approach — direct ConfigMap manipulation:**

For MVP, the Alerting API reads and writes hub rules directly in the existing ConfigMaps. No new CRDs or MCO operator changes are required.

- `GET /hub/rules` parses both ConfigMaps: `thanos-ruler-default-rules` (operator defaults) and `thanos-ruler-custom-rules` (user-created). Each rule group entry is parsed, an `alertRuleId` is computed, and the response includes source, classification, and `managedBy`. The `alertRuleId` hash inputs for hub rules are: `kind` (alert/record) + alertname + PromQL expression + `for` duration + sorted business label key-value pairs (excluding `openshift_io_*` and `alertname`). This is consistent with the single-cluster approach defined in the [alert-rule-classification-mapping](alert-rule-classification-mapping.md) enhancement, where the canonical payload is `kind + name + expr + for + labels_block` and the ID is `rid_<base64url(SHA256(canonical_payload))>`. Annotations are excluded from the hash, so annotation-only changes do not alter the rule ID.
- `POST /hub/rules` adds a new rule group entry to `thanos-ruler-custom-rules` ConfigMap YAML.
- `PATCH /hub/rules/{ruleId}` updates a rule entry in `thanos-ruler-custom-rules`. Blocked for rules in `thanos-ruler-default-rules` (operator-managed).
- `DELETE /hub/rules/{ruleId}` removes a rule entry from `thanos-ruler-custom-rules`. Blocked for defaults.
- ThanosRuler picks up ConfigMap changes via its existing watch — unchanged.
- Ownership detection: rules in `thanos-ruler-default-rules` are always `managedBy: operator`. Rules in `thanos-ruler-custom-rules` are `managedBy: ""` (unmanaged) unless the ConfigMap has ArgoCD annotations, in which case all custom rules are `managedBy: gitops` (ConfigMap-level granularity only).
- Optimistic concurrency: use the ConfigMap `resourceVersion` on update. This is per-ConfigMap, not per-rule — concurrent edits to different rules in the same ConfigMap will conflict. Acceptable for MVP given low expected write volume.

MVP limitations:
- No per-rule GitOps ownership — the entire `thanos-ruler-custom-rules` ConfigMap is either GitOps-managed or not.
- No per-rule disable — Users can silence individual alert.
- Concurrent writes to different custom rules may conflict on ConfigMap `resourceVersion`.

**Future iteration — `HubAlertingRule` CRD as single source of truth:**

To address MVP limitations, introduce a `HubAlertingRule` CRD in `open-cluster-management-observability`. All hub rules — both operator defaults and user-created custom rules — are represented as CRDs. A reconciler watches these CRDs and generates the ConfigMaps that ThanosRuler reads.

- The MCO operator creates `HubAlertingRule` CRDs for its default rules (with `managedBy: operator` label and ownerReferences), replacing direct ConfigMap writes
- `POST /hub/rules` creates a `HubAlertingRule` CRD for custom rules
- The reconciler generates ConfigMap YAML from CRDs and writes both ConfigMaps
- `GET /hub/rules` lists CRDs — one uniform source, no ConfigMap parsing
- Per-rule GitOps: individual CRDs can have ArgoCD annotations
- Per-rule optimistic concurrency: each CRD has its own `resourceVersion`
- Requires MCO operator change to adopt the CRD for default rules

### Hub rule tiers in the API

**MVP (ConfigMap-based):**

| Tier | Source | `managedBy` | Create | Update | Delete |
|------|--------|-------------|--------|--------|--------|
| Default hub rules | `thanos-ruler-default-rules` CM | `operator` | N/A | Blocked | Blocked |
| Custom hub rules (unmanaged) | `thanos-ruler-custom-rules` CM | `""` | `POST /hub/rules` | `PATCH /hub/rules/{id}` | `DELETE /hub/rules/{id}` |
| Custom hub rules (GitOps) | `thanos-ruler-custom-rules` CM with ArgoCD annotations | `gitops` | N/A | Blocked — guidance to edit in Git | Blocked |

**Future (CRD-based):**

| Tier | Source | `managedBy` | Create | Update | Delete |
|------|--------|-------------|--------|--------|--------|
| Default hub rules | `HubAlertingRule` CRD (operator-owned) | `operator` | N/A (operator creates) | Blocked | Blocked |
| Custom hub rules (unmanaged) | `HubAlertingRule` CRD | `""` | `POST /hub/rules` | `PATCH /hub/rules/{id}` | `DELETE /hub/rules/{id}` |
| Custom hub rules (GitOps) | `HubAlertingRule` CRD with ArgoCD annotations | `gitops` | N/A | Blocked — guidance to edit in Git | Blocked |

### Hub rule disable mechanism

Hub ThanosRuler has no ARC (AlertRelabelConfig) pipeline, so there is no equivalent of the spoke-side `action: Drop` mechanism. Disabling individual hub rules is not supported. Instead:

- To suppress notifications from a hub alert, users create a **silence** on the hub Alertmanager targeting that alert (already supported today via the Alertmanager API and UI).
- To replace a default hub rule's behavior, users create a **custom hub rule** with the desired expression/thresholds via `POST /hub/rules` and silence the original default alert.
- Custom hub rules that are no longer needed can be **deleted** via `DELETE /hub/rules/{ruleId}`.

### Hub alert classification labels

Classification labels (`openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`) exist primarily for **UI grouping and filtering** — they enable the console to organize alerts by component and impact layer across the fleet. They are not consumed by Alertmanager for routing or notification purposes.

On spokes, classification works at two levels (see the [alert-rule-classification-mapping](alert-rule-classification-mapping.md) enhancement for full detail):
1. **ARC-stamped labels on alerts**: When a rule is modified via the Alerting API (e.g., classification override, severity change), an `AlertRelabelConfig` CR is created. Prometheus applies the ARC's relabel configs globally before dispatching alerts to any Alertmanager, so these labels (`openshift_io_alert_rule_id`, `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`) are present on alerts sent to hub AM via `additionalAlertmanagerConfigs`. Only rules with ARC overrides (or explicit classification labels in their definition) have these labels on the alert.
2. **Server-side classifier enrichment**: The monitoring-plugin backend computes `alertComponent` and `alertLayer` as additive enrichment fields for ALL rules using classifier matchers, rule-scoped default labels, and ARC overrides. These fields are returned by the Alerting API (`GET /alerting/rules`, `GET /alerting/alerts`) but are not labels on the alert itself.

The hub's Rule Metadata Cache captures the spoke's pre-computed classification (from source 2) for all rules, enabling enrichment even for alerts without ARC labels. Hub ThanosRuler has no ARC pipeline, so hub alerts arrive at hub AM with only the labels explicitly defined in the rule.

**Operator default rules (`thanos-ruler-default-rules`):**

The MCO operator owns these rules and overwrites them on every reconciliation. Adding classification labels directly to the rule definitions would be overwritten. For MVP, the console backend maintains a static mapping of known default rule alertnames to their classification (`component`, `layer`). This is acceptable because the default rule set is small and changes only across operator upgrades.

For a future iteration, support for `alertRelabelConfigs` on ThanosRuler should be added so that the MCO operator can configure classification labels for default rules via relabeling — the same mechanism used for the user-defined monitoring stack. This has already been requested for the user-defined monitoring stack as well.

**User-created custom rules (`thanos-ruler-custom-rules`):**

When users create hub rules via `POST /hub/rules`, the API accepts `component` and `layer` as optional metadata and writes them as labels in the Prometheus rule definition inside the ConfigMap — the same convention used for single-cluster user-defined rules. ThanosRuler evaluates the rule and the resulting alert carries these labels natively to hub AM. Users are expected to include classification labels in their rule definitions, consistent with how it is done on single-cluster.

## API Endpoints

Base path: `/api/v1/alerting`

> Note: GET endpoints should remain compatible with upstream Thanos/Prometheus (query parameters and response schemas) wherever applicable to enable native Perses integration.

- Alerting rules (definitions)
  - `POST   /hub/rules`              — Create a hub alerting rule (MVP: adds rule entry to `thanos-ruler-custom-rules` ConfigMap; future: creates `HubAlertingRule` CRD)
  - `GET    /hub/rules`              — List hub alerting rules (MVP: parses both `thanos-ruler-default-rules` and `thanos-ruler-custom-rules` ConfigMaps; future: reads `HubAlertingRule` CRDs)
  - `GET    /hub/rules/{ruleId}`     — Get a hub rule by id
  - `PATCH  /hub/rules/{ruleId}`     — Update a hub rule (only custom/unmanaged; blocked for operator-managed and GitOps-managed)
  - `DELETE /hub/rules/{ruleId}`     — Delete a hub rule (only custom/unmanaged)

- Semantics:
  - Request/response schemas are identical to the single-cluster Alerting API and include `ruleId`, `labels`, `annotations`, `mapping` (component/impactGroup), `source` (`platform`|`user`|`hub`), `managedBy` (`operator`|`gitops`|`""`), and `disabled` flags.
  - Read paths keep Prometheus/Thanos compatibility where applicable.

#### Implementation impact (MCO adoption)

- For hub rule management (MVP): the Alerting API reads and writes directly to the existing `thanos-ruler-custom-rules` and `thanos-ruler-default-rules` ConfigMaps. No MCO operator changes needed. ThanosRuler picks up changes via its existing ConfigMap watch.
- A future iteration introduces a `HubAlertingRule` CRD with a reconciler for per-rule ownership and optimistic concurrency (see Open Questions).

## GitOps / Argo CD Compliance

- All generated `PrometheusRule` and `AlertRelabelConfig` resources remain declarative and suitable for committing to Git.
- Hub rules in `thanos-ruler-custom-rules` are treated as GitOps-managed when the ConfigMap has ArgoCD annotations. In this case, the API treats them as read-only and surfaces guidance to edit in Git.
- Future `HubAlertingRule` CRDs support per-rule ArgoCD annotations for fine-grained GitOps ownership.

## Risks & Mitigations

- **Scope and precedence complexity**: hub vs cluster rule precedence must be deterministic. Provide clear scope indicators, a confirmation step (using cached data) before batch apply, and UI guardrails. Document and enforce a single precedence policy in the API.
- **RBAC and ownership across clusters**: enforce hub and per-cluster RBAC. Treat GitOps-owned resources as read-only. Return per-cluster denial reasons in batch results.
- **Concurrent ConfigMap writes (MVP)**: concurrent writes to different custom rules may conflict on ConfigMap `resourceVersion`. Acceptable for MVP given low expected write volume. The future CRD-based approach eliminates this issue.

## Open Questions

- **Hub rule storage — future CRD evolution:** MVP uses direct ConfigMap manipulation for hub rules (no MCO operator changes). Two options for the future CRD-based iteration:
  - *Option A (recommended):* `HubAlertingRule` CRD is the single source of truth for ALL hub rules. The MCO operator creates CRDs for its default rules (with `managedBy: operator` and ownerReferences) instead of writing ConfigMaps directly. The reconciler generates both ConfigMaps from CRDs. `GET /hub/rules` reads CRDs only — one uniform source, one code path, per-rule ownership and disable semantics for all rules. Requires MCO operator change.
  - *Option B:* `HubAlertingRule` CRD is used only for custom (user/GitOps) rules. Default rules remain in `thanos-ruler-default-rules` ConfigMap, written directly by the MCO operator as today. `GET /hub/rules` reads from two sources: CRDs for custom rules + ConfigMap parsing for defaults. No MCO operator change, but two code paths and no per-rule disable for default rules.
  - Decision depends on MCO team willingness to adopt the CRD for operator-managed default rules. Should be resolved during design review with MCO team.

- **Hub rule disable mechanism:** The current design does not support disabling individual hub rules — users silence hub alerts and create custom replacement rules if needed. Is a dedicated per-rule disable mechanism (e.g., `spec.enabled: false` on a future `HubAlertingRule` CRD) actually needed, or is silence + custom rule creation sufficient for all use cases?
