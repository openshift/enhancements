---
title: multi-cluster-alerts-visualization
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
  - "/enhancements/monitoring/multi-cluster-alert-metrics-recording-rules.md"
  - "/enhancements/monitoring/alerts-ui-management.md"
  - "/enhancements/monitoring/alert-rule-classification-mapping.md"
---
# Multi-Cluster Alert Visualization (Read Path)

This proposal covers the read path for multi-cluster alert visualization: displaying spoke and hub alerts in a unified, enriched view on the hub console. It is Part 1 of the [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) umbrella proposal.

For shared context on the existing alert forwarding infrastructure, alert data storage lifecycle, and the ACM alerting developer preview, see the [Current State](multi-cluster-alerts-ui-management.md#current-state) section of the parent proposal.

## Summary

The `GET /hub/alerts` endpoint assembles a complete, post-relabel view of all alerts across the fleet. It uses hub Alertmanager as the primary real-time data source, classifies alerts locally using the hub-side classifier (`matcher.go`), enriches with rule metadata from the Rule Metadata Cache, and returns a unified view with classification, scope (hub vs spoke), and silence state.

For the data pipeline powering aggregated health views (heatmap, component tables) and historical alert queries, see [Alert Metrics and Recording Rules](multi-cluster-alert-metrics-recording-rules.md).

## Alerts Enrichment Pipeline

The `GET /hub/alerts` endpoint must assemble a complete, post-relabel view of all alerts across the fleet from multiple data sources. No single source has all the information needed.

### Data sources

Two data sources carry alert instances to the hub. Each serves a different purpose based on its latency and label coverage:

1. **Hub Alertmanager** (near real-time, ~30s-1min latency) — spoke Prometheus sends alerts to the hub Alertmanager via `additionalAlertmanagerConfigs` (injected by the MCOA endpoint operator). These alerts carry:
   - `managed_cluster` label (cluster name, from spoke `external_labels`)
   - ARC-applied labels (`openshift_io_alert_rule_id`, component, layer, severity overrides) on alerts whose rules have an associated AlertRelabelConfig (created via the Alerting API). Note: ARCs do not change the alerting rules evaluated by Prometheus — they only modify the alert labels sent to Alertmanager.
   - ARC-dropped (disabled) alerts never fire and never reach hub Alertmanager — correct behavior
   - Spoke-local silences are replicated to hub Alertmanager by the silence sync controller (with `managed_cluster` matcher scoping), so hub AM natively suppresses spoke-silenced alerts (approach not yet finalized — see [Silence Management](multi-cluster-silence-management.md))
   - **Used for:** `GET /hub/alerts` (real-time alerts page)

2. **`alerts_effective_active_at_timestamp_seconds` metric on hub Thanos** (~5 min latency, metrics-collector federation interval) — a new spoke-side metric representing the final effective alert state after ARC relabeling. Carries post-ARC labels, excludes disabled alerts, includes silence state. **Used for:** recording rules and historical alert queries (not the real-time alerts page). See [Alert Metrics and Recording Rules](multi-cluster-alert-metrics-recording-rules.md) for full details.

### Classification and caches

**Hub-side classifier**: The hub's monitoring-plugin runs the same `matcher.go` classifier logic as each spoke. Given an alert's `alertname` and labels, it computes `alertComponent` and `alertLayer` locally — no fan-out, no cache needed for classification. This is the same classifier that powers the single-cluster Alerting API (see [alert-rule-classification-mapping](alert-rule-classification-mapping.md)).

The hub console backend maintains two caches for metadata that the classifier cannot compute locally:

- **Rule Metadata Cache** (per cluster, TTL 5 min): populated by calling each spoke's Alerting API (`GET /alerting/rules`) via ManagedClusterProxy. Provides per-rule metadata: `alertRuleId`, `source` (platform/user), `managedBy` (operator/gitops/unmanaged), `prometheusRuleName`, and `disabled` state. Indexed by `alertRuleId` for direct lookup and by `alertname` for fuzzy matching. Hub rules are read from the hub `PrometheusRule` CRDs. Warmed on startup.
- **Cluster Registry Cache** (watch-based): populated from ManagedCluster resources on the hub. Provides cluster names, labels, status, and proxy endpoints.

### Enrichment steps

1. **Fetch alert instances** from hub Alertmanager (`GET /api/v2/alerts`). Each alert carries `alertname`, `severity`, `namespace`, `managed_cluster` (for spoke alerts), and ARC-applied labels (for alerts with an associated ARC). Hub-scoped alerts from ThanosRuler have no `managed_cluster` label.

2. **Classify by scope**: alerts with `managed_cluster` are spoke alerts; alerts without are hub alerts. Hub Alertmanager uses `status.state = "suppressed"` for both silenced and inhibited alerts. The `status.silencedBy` array distinguishes them: non-empty means silenced (map to `state=silenced`), empty with non-empty `status.inhibitedBy` means inhibited (map to `state=inhibited` or treat as silenced depending on UI requirements).

3. **Classify alerts** using the hub-side classifier (`matcher.go`). The classifier computes `alertComponent` and `alertLayer` locally from the alert's `alertname` and labels — the same logic used in the single-cluster Alerting API. This runs on the hub with no fan-out to spokes.

4. **Enrich with rule metadata** from the Rule Metadata Cache:
   - For alerts with `openshift_io_alert_rule_id` label (rules that have an ARC id stamp): direct O(1) lookup by alertRuleId in the cache. Adds enrichment fields: `alertRuleId`, `source` (platform/user), `prometheusRuleName`, `managedBy`.
   - For alerts without `openshift_io_alert_rule_id` (unmodified platform rules — the majority): match by `alertname` + `managed_cluster` in the cache. For platform rules, `alertname` is typically unique within a cluster. If multiple rules match, score by label intersection and pick the best match. Adds enrichment fields: `alertRuleId`, `source`, `managedBy`.
   - For hub alerts: look up in hub rule definitions by alertname. Adds enrichment fields: `alertRuleId`, `source=hub`, `managedBy` (operator for defaults, gitops/unmanaged for custom).
   - Cache miss: return alert with classification (from the classifier) but without rule metadata; trigger async cache refresh.

5. **Spoke silence state** is already reflected by hub Alertmanager. The silence sync controller replicates spoke silences to hub AM with `managed_cluster` matcher scoping, so spoke alerts that are silenced on the spoke appear as `state=suppressed` (mapped to `silenced`) in the hub AM response. No additional read-time matching is needed.

6. **Filter, sort, paginate** based on query parameters (state, severity, cluster, namespace, component, source, alertname, arbitrary label filters).

### Prerequisites

- **Spoke Alerting API (monitoring-plugin)**: The Rule Metadata Cache depends on each spoke exposing the Single-Cluster Alerting API (`GET /alerting/rules`) via the monitoring-plugin for rule metadata (`alertRuleId`, `source`, `managedBy`, `prometheusRuleName`, `disabled`). This requires the monitoring-plugin to be deployed on all managed clusters. The monitoring-plugin is available from OpenShift 4.18+. Spokes running older versions will still get classification (from the hub-side classifier) but will lack rule metadata (`alertRuleId`, `source`, `managedBy`).
- **ManagedClusterProxy**: Required for the hub console backend to reach spoke Alerting APIs and spoke Alertmanagers. Must be enabled on all managed clusters.
- **MCOA endpoint operator**: Must be configured to inject `additionalAlertmanagerConfigs` on spokes (existing behavior) for alerts to reach hub Alertmanager.

### Historical alert views

Hub Alertmanager is a transient, point-in-time store — it can only answer "what is firing right now?" For historical or over-time alert views, the UI queries metrics stored in hub Thanos. For MVP, the UI focuses on the real-time alerts page (hub AM). See [Alert Metrics and Recording Rules — Historical Alert Views](multi-cluster-alert-metrics-recording-rules.md#historical-alert-views) for the full data source mapping.

## API Endpoints

Base path: `/api/v1/alerting`

> Note: GET endpoints should remain compatible with upstream Thanos/Prometheus (query parameters and response schemas) wherever applicable to enable native Perses integration.

- Alerts (instances)
  - `GET    /hub/alerts`             — List alert instances from spoke clusters and hub (Firing / Silenced) with classification and rule metadata. Response schema matches `GET /alerting/alerts`. See [Alerts Enrichment Pipeline](#alerts-enrichment-pipeline) for the full data flow.

- Semantics:
  - Request/response schemas are identical to the single-cluster Alerting API and include `ruleId`, `labels`, `annotations`, `mapping` (component/impactGroup), `source` (`platform`|`user`|`hub`), `managedBy` (`operator`|`gitops`|`""`), and `disabled` flags.
  - Filters mirror the single-cluster API (`name`, `group`, `component`, `severity`, `state`, `source`, `triggered_since`, arbitrary label filters) plus multi-cluster filters (`cluster`, `cluster_labels`).
  - For `GET /hub/alerts`, the `cluster` filter selects alerts by `managed_cluster` label. Omitting it returns alerts from all clusters plus hub alerts.
  - Read paths keep Prometheus/Thanos compatibility where applicable.

#### Implementation impact (MCO adoption)
- Current behavior: managed clusters forward alerts to hub Alertmanager (via `additionalAlertmanagerConfigs`). The metrics-collector federates the `ALERTS` metric to hub Thanos.
- Required changes:
  - Add `alerts_effective_active_at_timestamp_seconds` to the metrics-collector federation allowlist so it is federated to hub Thanos (see [Data sources](#data-sources) for metric details).
  - Deploy the hub console backend with the Alerting API enrichment pipeline (see [Enrichment steps](#enrichment-steps)).

## Data Model

### Label topology across the stack

Understanding which labels are available at each data sink is critical for designing recording rules and API enrichment.

| Label | Spoke Alertmanager | Hub Alertmanager | Hub Thanos (ALERTS metric) | Hub Thanos (`alerts_effective_*` metric) |
|-------|--------------------|-----------------|---------------------------|------------------------------------------|
| `managed_cluster` | YES (via external_labels) | YES (via additionalAlertmanagerConfigs) | NO (stripped by metrics-collector) | NO (stripped by metrics-collector) |
| `cluster` | NO | NO | YES (MCOA addon write relabel) | YES (MCOA addon write relabel) |
| `clusterID` | NO | NO | YES (MCOA addon write relabel) | YES (MCOA addon write relabel) |
| `openshift_io_alert_rule_id` | YES (for rules with ARC id stamp) | YES (for rules with ARC id stamp) | NO | YES (post-ARC) |
| `openshift_io_alert_rule_component` | YES (for rules with ARC override or explicit label in rule definition) | YES (for rules with ARC override or explicit label in rule definition) | NO | YES (post-ARC) |
| `openshift_io_alert_rule_layer` | YES (for rules with ARC override or explicit label in rule definition) | YES (for rules with ARC override or explicit label in rule definition) | NO | YES (post-ARC) |
| Disabled alerts | absent (ARC Drop) | absent (ARC Drop) | present (no ARC in metric pipeline) | absent (ARC Drop) |
| Silenced alerts | suppressed state | suppressed (hub silences + spoke silences via sync controller) | present (no silence awareness) | present with `alertstate=silenced` |

Key implications:
- **Hub Alertmanager** is the primary source for `GET /hub/alerts` (real-time alerts page) — it has `managed_cluster` for cluster identification, ARC-applied labels for classification, and receives alerts in near real-time. Spoke silences are replicated to hub AM by the silence sync controller with `managed_cluster` matcher scoping, so hub AM natively reflects spoke silence state.
- **`alerts_effective_active_at_timestamp_seconds` on hub Thanos** is the source for recording rules and aggregated health views — it has `cluster` for identification, ARC-applied labels for classification, silence state as `alertstate=silenced`, and excludes disabled alerts. Not used for the real-time alerts page (~5 min federation latency).
- **Hub Thanos ALERTS metric** is not used directly — it lacks ARC labels and silence/disable awareness. Superseded by the `alerts_effective_*` metric for aggregation use cases.
- **ARC-applied labels** are only present on alerts with an ARC override or explicit rule labels. The majority of platform alerts lack them. The hub-side classifier computes classification for ALL alerts locally (see [Classification and caches](#classification-and-caches)).

## Metrics and Recording Rules

> Full detail: [Alert Metrics and Recording Rules](multi-cluster-alert-metrics-recording-rules.md)

The `alerts_effective_active_at_timestamp_seconds` metric and the recording rules defined over it (spoke-side and hub-side) are covered in the dedicated child proposal. They power the Fleet Health Heatmap, component health tables, and historical alert views.

## Risks & Mitigations

- **Aggregation freshness at scale**: fleet caches can become stale. Define SLAs and TTLs. Provide on‑demand refresh for critical views, and progressively load details. Fall back to sampled or approximate aggregates for heatmaps when needed.
- **Connectivity or proxy reliability**: outages or high latency in the ManagedClusterProxy on the hub may disrupt reads. Use retries with jitter, per‑cluster backoff, circuit breakers, and a degraded read mode backed by cached data.
- **Cache refresh fan-out at scale**: the Rule Metadata Cache refreshes by calling each spoke's Alerting API every 5 min for rule metadata (alertRuleId, source, managedBy). Classification (component/layer) is computed locally by the hub-side classifier and does not require fan-out. At 1000 clusters, rule metadata refresh produces ~3.3 calls/sec sustained. Use bounded concurrency (e.g., 20 parallel requests), staggered refresh intervals (jitter), and prioritize clusters with recent changes. Monitor cache refresh latency and spoke API error rates.

