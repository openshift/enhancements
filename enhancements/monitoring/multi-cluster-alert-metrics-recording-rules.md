---
title: multi-cluster-alert-metrics-recording-rules
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
creation-date: 2026-04-05
last-updated: 2026-04-05
tracking-link:
  - "https://issues.redhat.com/browse/CNV-46597"
  - "https://issues.redhat.com/browse/CNV-62972"
see-also:
  - "/enhancements/monitoring/multi-cluster-alerts-ui-management.md"
  - "/enhancements/monitoring/multi-cluster-alerts-visualization.md"
  - "/enhancements/monitoring/alerts-ui-management.md"
  - "/enhancements/monitoring/alert-rule-classification-mapping.md"
---
# Multi-Cluster Alert Metrics and Recording Rules

This proposal covers the data pipeline for aggregated health views and historical alert queries: the `alerts_effective_active_at_timestamp_seconds` metric, spoke-side and hub-side recording rules, and the historical alert views they enable. It is Part 4 of the [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) umbrella proposal.

For the real-time alert read path (`GET /hub/alerts`), see [Alert Visualization](multi-cluster-alerts-visualization.md). For shared context on the existing infrastructure, see the [Current State](multi-cluster-alerts-ui-management.md#current-state) section of the parent proposal.

## Summary

The real-time alerts page uses hub Alertmanager as its primary source (see [Alert Visualization](multi-cluster-alerts-visualization.md)). For aggregated health views (Fleet Health Heatmap, component health tables) and historical alert queries, a separate data pipeline is needed — one that carries post-ARC labels, silence state, and is stored long-term in hub Thanos.

The `alerts_effective_active_at_timestamp_seconds` metric fills this gap. It is a new spoke-side metric representing the final effective alert state after ARC relabeling. Recording rules defined over this metric produce pre-aggregated health counts for dashboards.

## `alerts_effective_active_at_timestamp_seconds` Metric

A new spoke-side metric representing the final effective alert state after ARC relabeling is applied. Federated to hub Thanos via the metrics-collector. Key properties:

- Generated on each spoke from the post-ARC alert notification pipeline (the source of truth for relabeled alert state)
- The value equals the alert's `activeAt` timestamp
- Carries post-ARC labels: `openshift_io_alert_rule_id`, `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`, `alertname`, `severity` (after overrides), `namespace`, and all routing-relevant labels
- Retains all original alert labels (no label dropping) — preserves full context for queries, drill-down, and matching
- Disabled alerts (ARC `action: Drop`) are absent — they never fire
- Silenced alerts are present with `alertstate=silenced` — the metric includes both firing and silenced alerts, enabling the UI to filter by state
- On hub Thanos: gets the `cluster` label (from MCOA addon write relabel configs) for cluster identification

**Cardinality note:** Alerts with high-cardinality labels (e.g., `pod`, `container`, `instance`) will produce one time series per unique label combination per firing alert. At fleet scale (many clusters, many alerts), this may generate significant series volume on hub Thanos. Monitor `alerts_effective_*` series count per cluster and consider targeted label dropping in the metrics-collector allowlist if cardinality becomes a concern.

### How is this metric generated?

**Open design question.** Options include:
- **(a) Sidecar/exporter** that reads from spoke Alertmanager API and produces the metric for Prometheus to scrape. More natural since only the Alertmanager knows the combined post-ARC + post-silence state.
- **(b) Recording rule** combined with a mechanism that applies ARC relabeling to the ALERTS metric. Less natural — the ALERTS metric does not carry ARC labels or silence state.

Option (a) is the preferred approach.

### Label availability

See the [Label topology across the stack](multi-cluster-alerts-visualization.md#label-topology-across-the-stack) table for which labels are available at each data sink. Key points for this metric:

- On hub Thanos, `cluster` and `clusterID` are available (added by MCOA addon write relabel configs). `managed_cluster` is NOT available (stripped by metrics-collector).
- ARC-applied labels (`openshift_io_alert_rule_id`, component, layer) ARE available — this metric is generated from the post-ARC pipeline.
- The standard `ALERTS` metric on hub Thanos does NOT carry ARC labels or silence/disable awareness. It is superseded by `alerts_effective_*` for aggregation use cases.

## Historical Alert Views

Hub Alertmanager is a transient, point-in-time store — it can only answer "what is firing right now?" For any historical or over-time alert view, the UI must query metrics stored in hub Thanos (S3):

| Use case | Data source | Notes |
|----------|-------------|-------|
| Real-time firing/silenced alerts | Hub Alertmanager | Near real-time (~30s-1min). Primary source for `GET /hub/alerts`. |
| Alert trend over time (e.g., "when did this alert start/stop firing?") | `ALERTS` metric on hub Thanos | Historical, but lacks ARC labels and silence awareness. |
| Enriched alert history (with component, layer, silence state) | `alerts_effective_active_at_timestamp_seconds` on hub Thanos | Post-ARC labels, `alertstate=silenced`, ~5 min granularity. |
| Component health over time | Recording rules based on `alerts_effective_*` | Pre-aggregated for dashboard use. |
| Fleet health heatmap history | Recording rules (`acm:cluster:health:critical_count`) | Spoke-side aggregation federated to hub. |

For MVP, the UI focuses on the real-time alerts page (hub AM). Historical alert views are a future enhancement that depends on the `alerts_effective_*` metric being deployed and federated.

## Recording Rules

Recording rules must be defined on the data source where the required labels are available.

### Spoke-side recording rules (federated to hub Thanos via metrics-collector)

These recording rules run on spoke Prometheus and are federated to hub Thanos. The `cluster` label is added by MCOA addon write relabeling on the hub side. The recording rules are deployed to spokes as PrometheusRule CRDs by the MCOA endpoint operator (similar to how it deploys other spoke-side configuration). The endpoint operator ensures the rules are present on every managed cluster and updates them on operator upgrade.

- Aggregated health: `acm:cluster:health:critical_count` -- counts firing alerts with `severity=critical` and `impact=cluster`. Powers the Fleet Health Heatmap. With `alerts_effective_active_at_timestamp_seconds` available, this recording rule can be defined over that metric to get correct post-ARC severity and exclude silenced/disabled alerts.
- Component health: `acm:component:health:severity_count` -- counts firing alerts grouped by `component` and `severity`. Drives the "Most Impacted Components" table. With `alerts_effective_active_at_timestamp_seconds`, this recording rule can group by `openshift_io_alert_rule_component` since the metric carries post-ARC labels.

### Hub-side recording rules (on ThanosRuler, querying hub Thanos)

These rules run on hub ThanosRuler over federated data. They have access to the `cluster` and `clusterID` labels, and when querying the `alerts_effective_*` metric, also have access to ARC-applied labels.

- Hub alerts health: `acm:hub:health:severity_count` -- counts hub-evaluated alerts grouped by `severity`. Default mapping labels are `impact=cluster` and `component=hub`.
- Fleet component health (new): `acm:fleet:component:health:severity_count` -- aggregates `alerts_effective_active_at_timestamp_seconds` across all clusters by `cluster`, `openshift_io_alert_rule_component`, and `severity`. Provides pre-aggregated component health for the "Most Impacted Components" table without per-request computation.

## Implementation Impact (MCO Adoption)

- Add `alerts_effective_active_at_timestamp_seconds` to the metrics-collector federation allowlist so it is federated to hub Thanos.
- Deploy spoke-side recording rules (`acm:cluster:health:*`, `acm:component:health:*`) as PrometheusRule CRDs via the MCOA endpoint operator.
- Deploy hub-side recording rules (`acm:hub:health:*`, `acm:fleet:component:health:*`) as PrometheusRule CRDs for ThanosRuler.

## Risks & Mitigations

- **`alerts_effective_*` metric cardinality**: retaining all original alert labels (no dropping) can produce high series volume at fleet scale. Monitor per-cluster series counts for this metric. If cardinality becomes a concern, consider targeted label dropping in the metrics-collector allowlist for high-cardinality labels (e.g., `pod`, `container`, `instance`).
- **Federation latency**: the metrics-collector federates at ~5 min intervals. Aggregated health views (heatmap, component tables) reflect this latency. Acceptable for dashboard use; the real-time alerts page uses hub Alertmanager instead.

## Open Questions

- **Metric generation mechanism:** How is `alerts_effective_active_at_timestamp_seconds` generated on each spoke? See [options above](#how-is-this-metric-generated). Needs design investigation.
- **Collection interval:** The metrics-collector currently federates spoke metrics to hub Thanos at ~5 min intervals (single global interval, no per-metric configuration). This latency is acceptable for recording rules and aggregated health views (heatmap, component tables). If a faster collection interval (~30s) becomes feasible for this metric, it could improve heatmap and component health freshness. The metrics-collector does not currently support per-metric intervals; possible approaches include (a) a separate lightweight collector instance per spoke for this metric, (b) per-metric interval support in the metrics-collector (code change). Needs investigation with the MCOA team.
