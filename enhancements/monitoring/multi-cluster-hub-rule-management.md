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
last-updated: 2026-04-05
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

> **Prerequisite — MCO migration required:** This proposal depends on MCO migrating from the Observatorium-based ThanosRuler deployment to prometheus-operator's `ThanosRuler` CR with `ruleSelector`. Until that migration is complete, hub rule management via the Alerting API cannot ship. See [Open Questions](#open-questions) for timeline considerations.

## Hub Rule Management

Hub alerting rules are evaluated by MCOA ThanosRuler over federated data from hub Thanos. Today, ThanosRuler is deployed via the Observatorium operator, which only supports ConfigMap-based rule files. This proposal requires MCO to adopt prometheus-operator's `ThanosRuler` CRD, which natively supports selecting `PrometheusRule` CRDs via `ruleSelector` — the same pattern used by the `Prometheus` CR on spokes. This provides a consistent approach between single-cluster and multi-cluster rule management.

### Current hub rule storage

| ConfigMap | Key | Content | Ownership |
|-----------|-----|---------|-----------|
| `thanos-ruler-default-rules` | `default_rules.yaml` | Default alerts (KubePersistentVolumeFillingUp, ViolatedPolicyReport, Thanos health) and recording rules (cluster aggregations for Grafana). Always present. | MCO operator — read-only, operator-managed. The operator overwrites on reconciliation. |
| `thanos-ruler-custom-rules` | `custom_rules.yaml` | User-defined custom alerting and recording rules. Created on demand. MCO controller watches for updates. | User-managed (direct ConfigMap edit) or GitOps-managed (ArgoCD manages the ConfigMap). |

### Hub rule ownership model

The API uses `PrometheusRule` CRDs — the same CRD used on spoke clusters. This reuses the same API code and CRD patterns as the single-cluster Alerting API and provides per-rule ownership, GitOps annotations, and optimistic concurrency.

**Prerequisite — MCO adopts prometheus-operator `ThanosRuler` CR:**

MCO migrates from Observatorium-based ThanosRuler to prometheus-operator's `ThanosRuler` CR with `ruleSelector`. This enables ThanosRuler to read `PrometheusRule` CRDs directly — no ConfigMap bridge needed.

- The MCO operator creates `PrometheusRule` CRDs for default rules (with `managedBy: operator` label and ownerReferences) instead of writing ConfigMaps directly
- `POST /hub/rules` creates a `PrometheusRule` CRD in `open-cluster-management-observability` for custom rules
- `GET /hub/rules` lists `PrometheusRule` CRDs — one uniform source, consistent with the single-cluster API
- `PATCH /hub/rules/{ruleId}` updates a `PrometheusRule` CRD. Blocked for operator default rules.
- `DELETE /hub/rules/{ruleId}` deletes a `PrometheusRule` CRD. Blocked for defaults.
- The `alertRuleId` hash inputs for hub rules are: `kind` (alert/record) + alertname + PromQL expression + `for` duration + sorted business label key-value pairs (excluding `openshift_io_*` and `alertname`). This is consistent with the single-cluster approach defined in the [alert-rule-classification-mapping](alert-rule-classification-mapping.md) enhancement, where the canonical payload is `kind + name + expr + for + labels_block` and the ID is `rid_<base64url(SHA256(canonical_payload))>`. Annotations are excluded from the hash, so annotation-only changes do not alter the rule ID.
- Ownership detection: `PrometheusRule` CRDs use standard per-resource labels and annotations (`managedBy: operator` with ownerReferences for defaults, `managedBy: ""` for unmanaged, ArgoCD annotations for GitOps-managed).
- Per-rule optimistic concurrency: each `PrometheusRule` CRD has its own `resourceVersion`.
- Per-rule GitOps: individual `PrometheusRule` CRDs can have ArgoCD annotations.

### Hub rule tiers in the API

| Tier | Source | `managedBy` | Create | Update | Delete |
|------|--------|-------------|--------|--------|--------|
| Default hub rules | `PrometheusRule` CRD (operator-owned) | `operator` | N/A (operator creates) | Blocked | Blocked |
| Custom hub rules (unmanaged) | `PrometheusRule` CRD | `""` | `POST /hub/rules` | `PATCH /hub/rules/{id}` | `DELETE /hub/rules/{id}` |
| Custom hub rules (GitOps) | `PrometheusRule` CRD with ArgoCD annotations | `gitops` | N/A | Blocked — guidance to edit in Git | Blocked |

### Hub rule disable mechanism

Hub ThanosRuler has no ARC (AlertRelabelConfig) pipeline, so there is no equivalent of the spoke-side `action: Drop` mechanism. Disabling individual hub rules is not supported. Instead:

- To suppress notifications from a hub alert, users create a **silence** on the hub Alertmanager targeting that alert (already supported today via the Alertmanager API and UI).
- To replace a default hub rule's behavior, users create a **custom hub rule** with the desired expression/thresholds via `POST /hub/rules` and silence the original default alert.
- Custom hub rules that are no longer needed can be **deleted** via `DELETE /hub/rules/{ruleId}`.

### Hub alert classification labels

Classification labels (`openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`) exist primarily for **UI grouping and filtering** — they enable the console to organize alerts by component and impact layer across the fleet. They are not consumed by Alertmanager for routing or notification purposes.

On spokes, classification works at two levels (see the [alert-rule-classification-mapping](alert-rule-classification-mapping.md) enhancement for full detail):
1. **ARC-stamped labels on alerts**: When a classification override or severity change is applied via the Alerting API, an `AlertRelabelConfig` CR is created. ARCs do not change the alerting rules evaluated by Prometheus — they modify the alert labels sent to Alertmanager. Prometheus applies ARC relabel configs globally before dispatching alerts to any Alertmanager, so these labels (`openshift_io_alert_rule_id`, `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`) are present on alerts sent to hub AM via `additionalAlertmanagerConfigs`. Only alerts with an associated ARC (or explicit classification labels in their rule definition) have these labels.
2. **Server-side classifier enrichment**: The monitoring-plugin backend computes `alertComponent` and `alertLayer` as additive enrichment fields for ALL rules using classifier matchers, rule-scoped default labels, and ARC overrides. These fields are returned by the Alerting API (`GET /alerting/rules`, `GET /alerting/alerts`) but are not labels on the alert itself.

The hub-side classifier (`matcher.go`) computes classification for hub alerts locally using the same logic as spokes. Hub ThanosRuler has no ARC pipeline, so hub alerts arrive at hub AM with only the labels explicitly defined in the rule.

**Operator default rules (`thanos-ruler-default-rules`):**

The MCO operator owns these rules and overwrites them on every reconciliation. Adding classification labels directly to the rule definitions would be overwritten. For MVP, the console backend maintains a static mapping of known default rule alertnames to their classification (`component`, `layer`). This is acceptable because the default rule set is small and changes only across operator upgrades.

For a future iteration, support for `alertRelabelConfigs` on ThanosRuler should be added so that the MCO operator can configure classification labels for default rules via relabeling — the same mechanism used for the user-defined monitoring stack. This has already been requested for the user-defined monitoring stack as well.

**User-created custom rules (`thanos-ruler-custom-rules`):**

When users create hub rules via `POST /hub/rules`, the API accepts `component` and `layer` as optional metadata and writes them as labels in the `PrometheusRule` CRD's rule definition — the same convention used for single-cluster user-defined rules. ThanosRuler evaluates the rule and the resulting alert carries these labels natively to hub AM. Users are expected to include classification labels in their rule definitions, consistent with how it is done on single-cluster.

## API Endpoints

Base path: `/api/v1/alerting`

> Note: GET endpoints should remain compatible with upstream Thanos/Prometheus (query parameters and response schemas) wherever applicable to enable native Perses integration.

- Alerting rules (definitions)
  - `POST   /hub/rules`              — Create a hub alerting rule (creates a `PrometheusRule` CRD)
  - `GET    /hub/rules`              — List hub alerting rules (reads `PrometheusRule` CRDs — both operator-owned defaults and user-created custom rules)
  - `GET    /hub/rules/{ruleId}`     — Get a hub rule by id
  - `PATCH  /hub/rules/{ruleId}`     — Update a hub rule (only custom/unmanaged; blocked for operator-managed and GitOps-managed)
  - `DELETE /hub/rules/{ruleId}`     — Delete a hub rule (only custom/unmanaged)

- Semantics:
  - Request/response schemas are identical to the single-cluster Alerting API and include `ruleId`, `labels`, `annotations`, `mapping` (component/impactGroup), `source` (`platform`|`user`|`hub`), `managedBy` (`operator`|`gitops`|`""`), and `disabled` flags.
  - Read paths keep Prometheus/Thanos compatibility where applicable.

#### Implementation impact (MCO adoption)

This proposal requires MCO to migrate from the Observatorium-based ThanosRuler deployment to prometheus-operator's `ThanosRuler` CR with `ruleSelector` for `PrometheusRule` CRDs. As part of this migration, the MCO operator creates `PrometheusRule` CRDs for default rules (instead of writing ConfigMaps directly). The Alerting API then operates entirely on `PrometheusRule` CRDs — no ConfigMap parsing needed.

## GitOps / Argo CD Compliance

- All generated `PrometheusRule` and `AlertRelabelConfig` resources remain declarative and suitable for committing to Git.
- Custom hub rules are `PrometheusRule` CRDs, so per-rule ArgoCD annotations are supported for fine-grained GitOps ownership — consistent with how spoke rules are managed. GitOps-managed `PrometheusRule` CRDs are treated as read-only by the API.

## Risks & Mitigations

- **Scope and precedence complexity**: hub vs cluster rule precedence must be deterministic. Provide clear scope indicators and UI guardrails. Document and enforce a single precedence policy in the API.
- **RBAC and ownership across clusters**: enforce hub and per-cluster RBAC. Treat GitOps-owned resources as read-only.
- **MCO migration dependency**: this proposal requires MCO to adopt prometheus-operator's `ThanosRuler` CR. Timeline depends on MCO team capacity. Hub rule management cannot ship until this migration is complete.

## Open Questions

- **MCO migration timeline:** This proposal requires MCO to adopt prometheus-operator's `ThanosRuler` CR with `ruleSelector`. Timeline depends on MCO team capacity and willingness to migrate from the Observatorium-based deployment. Should be resolved during design review with MCO team.

- **Hub rule disable mechanism:** The current design does not support disabling individual hub rules — users silence hub alerts and create custom replacement rules if needed. If a dedicated per-rule disable mechanism is needed in the future, it should work from the resource's labels (consistent with Kubernetes conventions) rather than from a spec field.
