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
last-updated: 2026-03-18
tracking-link:
  - "https://issues.redhat.com/browse/CNV-46597"
  - "https://issues.redhat.com/browse/CNV-62972"
see-also:
  - "/enhancements/monitoring/multi-cluster-alerts-ui-management.md"
  - "/enhancements/monitoring/alerts-ui-management.md"
  - "/enhancements/monitoring/alert-rule-classification-mapping.md"
---
# Multi-Cluster Alert Visualization (Read Path)

This proposal covers the read path for multi-cluster alert visualization: displaying spoke and hub alerts in a unified, enriched view on the hub console. It is Part 1 of the [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) umbrella proposal.

For shared context on the existing alert forwarding infrastructure, alert data storage lifecycle, and the ACM alerting developer preview, see the [Current State](multi-cluster-alerts-ui-management.md#current-state) section of the parent proposal.

## Table of Contents

- [Summary](#summary)
- [Alerts Enrichment Pipeline](#alerts-enrichment-pipeline)
  - [Data sources](#data-sources)
  - [Enrichment caches](#enrichment-caches)
  - [Enrichment steps](#enrichment-steps)
  - [Prerequisites](#prerequisites)
  - [Label mapping across data sources](#label-mapping-across-data-sources)
  - [Historical alert views](#historical-alert-views)
- [API Endpoints](#api-endpoints)
- [Data Model](#data-model)
- [Metrics and Recording Rules](#metrics-and-recording-rules)
- [Risks & Mitigations](#risks--mitigations)
- [Open Questions](#open-questions)

## Summary

The `GET /hub/alerts` endpoint assembles a complete, post-relabel view of all alerts across the fleet. It uses hub Alertmanager as the primary real-time data source, enriches alerts with rule metadata from a Rule Metadata Cache, and returns a unified view with classification labels, scope (hub vs spoke), and silence state.

The `alerts_effective_active_at_timestamp_seconds` metric provides the data source for historical alert views, recording rules, and aggregated health views (heatmap, component tables) where ~5 min latency is acceptable.

## Alerts Enrichment Pipeline

The `GET /hub/alerts` endpoint must assemble a complete, post-relabel view of all alerts across the fleet from multiple data sources. No single source has all the information needed.

### Data sources

Two data sources carry alert instances to the hub. Each serves a different purpose based on its latency and label coverage:

1. **Hub Alertmanager** (near real-time, ~30s-1min latency) — spoke Prometheus sends alerts to the hub Alertmanager via `additionalAlertmanagerConfigs` (injected by the MCOA endpoint operator). These alerts carry:
   - `managed_cluster` label (cluster name, from spoke `external_labels`)
   - ARC-applied labels (`openshift_io_alert_rule_id`, component, layer, severity overrides) for rules that have been classified or modified via the Alerting API
   - ARC-dropped (disabled) alerts never fire and never reach hub Alertmanager — correct behavior
   - Spoke-local silences are replicated to hub Alertmanager by the silence sync controller (with `managed_cluster` matcher scoping), so hub AM natively suppresses spoke-silenced alerts (approach not yet finalized — see [Silence Management](multi-cluster-silence-management.md))
   - **Used for:** `GET /hub/alerts` (real-time alerts page)

2. **`alerts_effective_active_at_timestamp_seconds` metric on hub Thanos** (~5 min latency, metrics-collector federation interval) — a new spoke-side metric that represents the final effective alert state after ARC relabeling is applied. Federated to hub Thanos via the metrics-collector. Key properties:
   - Carries post-ARC labels: `openshift_io_alert_rule_id`, component, layer, severity (after overrides), and all routing-relevant labels
   - Disabled alerts (ARC `action: Drop`) are absent — they never fire
   - Silenced alerts are present with `alertstate=silenced`
   - The value equals the alert `activeAt` timestamp
   - Retains all original alert labels (no label dropping) — preserves full context for queries, drill-down, and matching
   - On hub Thanos: gets the `cluster` label (from MCOA addon write relabel configs) for cluster identification
   - **Used for:** recording rules (fleet health heatmap, component health aggregation) and historical alert queries. Not used for the real-time alerts page.

**Primary source for `GET /hub/alerts`:** Hub Alertmanager. It receives alerts in near real-time from spoke Prometheus (on every evaluation cycle). The current design uses a silence sync controller that replicates spoke Alertmanager silences to hub AM with `managed_cluster` matcher scoping, so hub AM natively reflects spoke silence state without a read-time cache (approach not yet finalized — see [Silence Management](multi-cluster-silence-management.md)).

**Source for recording rules and aggregated health:** `alerts_effective_active_at_timestamp_seconds` on hub Thanos. The ~5 min federation latency is acceptable for heatmap health counts and pre-aggregated component tables. This metric carries post-ARC labels and silence state (`alertstate=silenced`) that the standard ALERTS metric lacks, enabling component-based recording rules and silence-aware health counts.

### Enrichment caches

The hub console backend maintains two caches to avoid fan-out on every request:

- **Rule Metadata Cache** (per cluster, TTL 5 min): populated by calling each spoke's Alerting API (`GET /alerting/rules`) via ManagedClusterProxy. The spoke API already returns fully relabeled rules with all metadata (alertRuleId, source, component, layer, managedBy, disabled, prometheusRuleName). Indexed by `alertRuleId` for direct lookup and by `alertname` for fuzzy matching. Hub rules are read from the hub rule ConfigMaps. Warmed on startup.
- **Cluster Registry Cache** (watch-based): populated from ManagedCluster resources on the hub. Provides cluster names, labels, status, and proxy endpoints.

### Enrichment steps

1. **Fetch alert instances** from hub Alertmanager (`GET /api/v2/alerts`). Each alert carries `alertname`, `severity`, `namespace`, `managed_cluster` (for spoke alerts), and ARC-applied labels (for rules that have been modified). Hub-scoped alerts from ThanosRuler have no `managed_cluster` label.

2. **Classify by scope**: alerts with `managed_cluster` are spoke alerts; alerts without are hub alerts. Hub Alertmanager uses `status.state = "suppressed"` for both silenced and inhibited alerts. The `status.silencedBy` array distinguishes them: non-empty means silenced (map to `state=silenced`), empty with non-empty `status.inhibitedBy` means inhibited (map to `state=inhibited` or treat as silenced depending on UI requirements).

3. **Enrich with rule metadata** from the Rule Metadata Cache. The spoke's Alerting API (`GET /alerting/rules`) already computes classification for ALL rules — using classifier matchers, rule-scoped default labels, and ARC overrides — and returns the effective `alertComponent` and `alertLayer` as additive enrichment fields (consistent with the [alert-rule-classification-mapping](alert-rule-classification-mapping.md) enhancement). The Rule Metadata Cache captures this pre-computed classification:
   - For alerts with `openshift_io_alert_rule_id` label (rules that have an ARC id stamp): direct O(1) lookup by alertRuleId in the cache. Adds enrichment fields: `alertRuleId`, `alertComponent`, `alertLayer`, `source` (platform/user), `prometheusRuleName`, `managedBy`.
   - For alerts without `openshift_io_alert_rule_id` (unmodified platform rules — the majority): match by `alertname` + `managed_cluster` in the cache. For platform rules, `alertname` is typically unique within a cluster. If multiple rules match, score by label intersection and pick the best match. Adds enrichment fields: `alertRuleId`, `alertComponent`, `alertLayer`, `source`, `managedBy`. The classification comes from the spoke's classifier matchers (computed server-side for all rules), not from labels on the alert.
   - For hub alerts: look up in hub rule definitions by alertname. Adds enrichment fields: `alertRuleId`, `alertComponent`, `alertLayer`, `source=hub`, `managedBy` (operator for defaults, gitops/unmanaged for custom). Hub alert classification comes from the static mapping (for operator defaults) or from labels in the rule definition (for user-created rules).
   - Cache miss: return alert with partial enrichment (no classification); trigger async cache refresh.

4. **Spoke silence state** is already reflected by hub Alertmanager. The silence sync controller replicates spoke silences to hub AM with `managed_cluster` matcher scoping, so spoke alerts that are silenced on the spoke appear as `state=suppressed` (mapped to `silenced`) in the hub AM response. No additional read-time matching is needed.

5. **Filter, sort, paginate** based on query parameters (state, severity, cluster, namespace, component, source, alertname, arbitrary label filters).

### Prerequisites

- **Spoke Alerting API (monitoring-plugin)**: The Rule Metadata Cache depends on each spoke exposing the Single-Cluster Alerting API (`GET /alerting/rules`) via the monitoring-plugin. This requires the monitoring-plugin to be deployed on all managed clusters. The monitoring-plugin is available from OpenShift 4.18+. Spokes running older versions will not have rule metadata in the cache; their alerts will be returned with partial enrichment (no classification labels, no `alertRuleId`).
- **ManagedClusterProxy**: Required for the hub console backend to reach spoke Alerting APIs and spoke Alertmanagers. Must be enabled on all managed clusters.
- **MCOA endpoint operator**: Must be configured to inject `additionalAlertmanagerConfigs` on spokes (existing behavior) for alerts to reach hub Alertmanager.

### Label mapping across data sources

See the full label topology table in the Data Model section below. Key points for enrichment:
- Hub Alertmanager has `managed_cluster` and ARC-applied labels (`openshift_io_alert_rule_id`, component, layer) -- primary source for the real-time alerts page.
- `alerts_effective_active_at_timestamp_seconds` on hub Thanos has `cluster` and ARC-applied labels plus `alertstate` -- primary source for recording rules and aggregated health views (not for real-time alerts due to ~5 min federation latency).
- The `cluster` label on hub Thanos and the `managed_cluster` label on hub Alertmanager both identify the cluster by name (same value, different key).
- ARC-applied labels are only present on alerts whose rules have an ARC id stamp (rules modified via the Alerting API).

### Historical alert views

Hub Alertmanager is a transient, point-in-time store — it can only answer "what is firing right now?" For any historical or over-time alert view, the UI must query metrics stored in hub Thanos (S3):

| Use case | Data source | Notes |
|----------|-------------|-------|
| Real-time firing/silenced alerts | Hub Alertmanager | Near real-time (~30s-1min). Primary source for `GET /hub/alerts`. |
| Alert trend over time (e.g., "when did this alert start/stop firing?") | `ALERTS` metric on hub Thanos | Historical, but lacks ARC labels and silence awareness. |
| Enriched alert history (with component, layer, silence state) | `alerts_effective_active_at_timestamp_seconds` on hub Thanos | Post-ARC labels, `alertstate=silenced`, ~5 min granularity. |
| Component health over time | Recording rules based on `alerts_effective_*` | Pre-aggregated for dashboard use. |
| Fleet health heatmap history | Recording rules (`acm:cluster:health:critical_count`) | Spoke-side aggregation federated to hub. |

The raw `ALERTS` metric can serve basic "was this alert firing at time T?" queries, but it cannot support enriched views (grouping by component, filtering by silence state, excluding disabled alerts). The `alerts_effective_*` metric fills this gap. Both metrics coexist in hub Thanos — the `ALERTS` metric for backward compatibility and basic queries, and `alerts_effective_*` for enriched use cases.

For MVP, the UI focuses on the real-time alerts page (hub AM). Historical alert views are a future enhancement that depends on the `alerts_effective_*` metric being deployed and federated.

## API Endpoints

Base path: `/api/v1/alerting`

> Note: GET endpoints should remain compatible with upstream Thanos/Prometheus (query parameters and response schemas) wherever applicable to enable native Perses integration.

- Alerts (instances)
  - `GET    /hub/alerts`             — List aggregated alert instances from spoke clusters and hub (Firing / Silenced) with classification and mapping labels. Response schema matches `GET /alerting/alerts`. Backed by hub Alertmanager (primary, near real-time), enriched with rule metadata from the Rule Metadata Cache. Spoke silence state is natively available on hub AM via the silence sync controller. See the Alerts Enrichment Pipeline section for the full flow.

- Semantics:
  - Request/response schemas are identical to the single-cluster Alerting API and include `ruleId`, `labels`, `annotations`, `mapping` (component/impactGroup), `source` (`platform`|`user`|`hub`), `managedBy` (`operator`|`gitops`|`""`), and `disabled` flags.
  - Filters mirror the single-cluster API (`name`, `group`, `component`, `severity`, `state`, `source`, `triggered_since`, arbitrary label filters) plus multi-cluster filters (`cluster`, `cluster_labels`).
  - For `GET /hub/alerts`, the `cluster` filter selects alerts by `managed_cluster` label. Omitting it returns alerts from all clusters plus hub alerts.
  - Read paths keep Prometheus/Thanos compatibility where applicable.

#### Implementation impact (MCO adoption)
- Current behavior in `multicluster-observability-operator`: managed clusters forward alerts to the hub Alertmanager (via `additionalAlertmanagerConfigs` injected by the MCOA endpoint operator). The metrics-collector also federates the `ALERTS` metric to hub Thanos but strips the `managed_cluster` label.
- Required changes:
  - Collect the `alerts_effective_active_at_timestamp_seconds` metric from the spoke clusters. This metric will obtain the post-ARC, post-silence effective alert state and produces the metric for spoke Prometheus to scrape. The metrics-collector federates it to hub Thanos alongside existing metrics. This metric is used for recording rules and aggregated health views (heatmap, component tables), NOT for the real-time alerts page, due to the 5 min collection interval.
  - The console backend queries hub Alertmanager for `GET /hub/alerts` (primary source, near real-time). Alerts carry `managed_cluster` and ARC-applied labels. The endpoint enriches alerts with rule metadata from the Rule Metadata Cache and applies RBAC filtering.

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
- **ARC-applied labels** (`openshift_io_alert_rule_id`, `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`) are only present as labels on alerts whose rules have an ARC override (created via the Alerting API) or whose rule definitions explicitly include them. The majority of platform rules do not have these labels on the alert. However, the spoke's monitoring-plugin classifier computes `alertComponent` and `alertLayer` as server-side enrichment fields for ALL rules (using classifier matchers and rule-scoped defaults). The Rule Metadata Cache captures this classification for all rules, so alerts without ARC labels can still be enriched with component/layer from the cache.

## Metrics and Recording Rules

Recording rules must be defined on the data source where the required labels are available.

### Spoke-side alert metric (federated to hub Thanos via metrics-collector)

- **`alerts_effective_active_at_timestamp_seconds`**: a new spoke-side metric representing the final effective alert state after ARC relabeling. The value equals the alert's `activeAt` timestamp. This metric is the data source for recording rules and aggregated health views on the hub (not for the real-time alerts page — hub Alertmanager is the primary source for that).
  - Generated on each spoke from the post-ARC alert notification pipeline (the source of truth for relabeled alert state)
  - Carries post-ARC labels: `alertname`, `severity` (after overrides), `namespace`, `openshift_io_alert_rule_id`, `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`, and other routing-relevant labels
  - Retains all original alert labels (no label dropping) — preserves full context for queries, drill-down, and matching. Cardinality note: alerts with high-cardinality labels (e.g., `pod`, `container`, `instance`) will produce one time series per unique label combination per firing alert. At fleet scale (many clusters, many alerts), this may generate significant series volume on hub Thanos. Monitor `alerts_effective_*` series count per cluster and consider targeted label dropping in the metrics-collector allowlist if cardinality becomes a concern.
  - Disabled alerts (ARC `action: Drop`) are absent — they never fire
  - Silenced alerts are present with `alertstate=silenced` — the metric includes both firing and silenced alerts, enabling the UI to filter by state
  - Federated to hub Thanos via the metrics-collector. On hub Thanos, the `cluster` label is added by MCOA addon write relabel configs.
  - **Open:** How is this metric generated? Options include: (a) a sidecar/exporter that reads from spoke Alertmanager API and produces the metric, (b) a recording rule combined with a mechanism that applies ARC relabeling to the ALERTS metric. Option (a) is more natural since only the Alertmanager knows the combined post-ARC + post-silence state.

### Spoke-side recording rules (federated to hub Thanos via metrics-collector)

These recording rules run on spoke Prometheus and are federated to hub Thanos. The `cluster` label is added by MCOA addon write relabeling on the hub side. The recording rules are deployed to spokes as PrometheusRule CRDs by the MCOA endpoint operator (similar to how it deploys other spoke-side configuration). The endpoint operator ensures the rules are present on every managed cluster and updates them on operator upgrade.

- Aggregated health: `acm:cluster:health:critical_count` -- counts firing alerts with `severity=critical` and `impact=cluster`. Powers the Fleet Health Heatmap. With `alerts_effective_active_at_timestamp_seconds` available, this recording rule can be defined over that metric to get correct post-ARC severity and exclude silenced/disabled alerts.
- Component health: `acm:component:health:severity_count` -- counts firing alerts grouped by `component` and `severity`. Drives the "Most Impacted Components" table. With `alerts_effective_active_at_timestamp_seconds`, this recording rule can group by `openshift_io_alert_rule_component` since the metric carries post-ARC labels.

### Hub-side recording rules (on ThanosRuler, querying hub Thanos)

These rules run on hub ThanosRuler over federated data. They have access to the `cluster` and `clusterID` labels, and when querying the `alerts_effective_*` metric, also have access to ARC-applied labels.

- Hub alerts health: `acm:hub:health:severity_count` -- counts hub-evaluated alerts grouped by `severity`. Default mapping labels are `impact=cluster` and `component=hub`.
- Fleet component health (new): `acm:fleet:component:health:severity_count` -- aggregates `alerts_effective_active_at_timestamp_seconds` across all clusters by `cluster`, `openshift_io_alert_rule_component`, and `severity`. Provides pre-aggregated component health for the "Most Impacted Components" table without per-request computation.

### Recording rules design constraint (partially resolved)

The `alerts_effective_active_at_timestamp_seconds` metric resolves the main constraint: ARC-applied labels ARE available on this metric because it is generated from the post-ARC alert notification pipeline. Recording rules that use this metric CAN group by `openshift_io_alert_rule_component` and `openshift_io_alert_rule_layer`.

The original ALERTS metric (produced by Prometheus rule evaluation) still does NOT carry ARC-applied labels. Recording rules that use the ALERTS metric directly cannot group by component/layer.

## Risks & Mitigations

- **Aggregation freshness at scale**: fleet caches can become stale. Define SLAs and TTLs. Provide on‑demand refresh for critical views, and progressively load details. Fall back to sampled or approximate aggregates for heatmaps when needed.
- **Connectivity or proxy reliability**: outages or high latency in the ManagedClusterProxy on the hub may disrupt reads. Use retries with jitter, per‑cluster backoff, circuit breakers, and a degraded read mode backed by cached data.
- **Cache refresh fan-out at scale**: the Rule Metadata Cache refreshes by calling each spoke's Alerting API every 5 min. At 1000 clusters, this produces ~3.3 rule-metadata calls/sec sustained. Use bounded concurrency (e.g., 20 parallel requests), staggered refresh intervals (jitter), and prioritize clusters with recent changes. Monitor cache refresh latency and spoke API error rates.
- **`alerts_effective_*` metric cardinality**: retaining all original alert labels (no dropping) can produce high series volume at fleet scale. Monitor per-cluster series counts for this metric. If cardinality becomes a concern, consider targeted label dropping in the metrics-collector allowlist for high-cardinality labels (e.g., `pod`, `container`, `instance`).

## Open Questions

- **`alerts_effective_active_at_timestamp_seconds` collection interval:** The metrics-collector currently federates spoke metrics to hub Thanos at ~5 min intervals (single global interval, no per-metric configuration). This latency is acceptable for recording rules and aggregated health views (heatmap, component tables). If a faster collection interval (~30s) becomes feasible for this metric, it could improve heatmap and component health freshness. The metrics-collector does not currently support per-metric intervals; possible approaches include (a) a separate lightweight collector instance per spoke for this metric, (b) per-metric interval support in the metrics-collector (code change). Needs investigation with the MCOA team.
