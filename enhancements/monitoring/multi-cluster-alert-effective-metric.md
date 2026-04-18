---
title: multi-cluster-alert-effective-metric
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
creation-date: 2026-04-05
last-updated: 2026-04-09
tracking-link:
  - "https://issues.redhat.com/browse/CNV-46597"
  - "https://issues.redhat.com/browse/CNV-62972"
see-also:
  - "/enhancements/monitoring/multi-cluster-alerts-ui-management.md"
  - "/enhancements/monitoring/multi-cluster-alerts-visualization.md"
  - "/enhancements/monitoring/alerts-ui-management.md"
  - "/enhancements/monitoring/alert-rule-classification-mapping.md"
---
# Multi-Cluster Alert Effective Metric

This proposal covers the collection and hub-side enhancement of the `alerts_effective_active_at_timestamp_seconds` metric for multi-cluster use. The metric itself is being developed for the single-cluster use case (see [alerts-ui-management](alerts-ui-management.md)); this document focuses on federating it from spokes to hub Thanos and extending the pipeline to include hub-evaluated alerts. It is Part 4 of the [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) umbrella proposal.

For the real-time alert read path (`GET /alerting/alerts` on the hub), see [Alert Visualization](multi-cluster-alerts-visualization.md). For shared context on the existing infrastructure, see the [Current State](multi-cluster-alerts-ui-management.md#current-state) section of the parent proposal.

## Summary

The `alerts_effective_active_at_timestamp_seconds` metric is a spoke-side metric representing the final effective alert state after ARC relabeling and silence processing. It is being introduced in the single-cluster alerting enhancement to power enriched alert views on each cluster.

For multi-cluster use, this metric needs to be:
1. **Federated** from spokes to hub Thanos via the metrics-collector — replacing the Prometheus `ALERTS` metric as the enriched source for fleet-wide historical queries and dashboards.
2. **Extended** to include hub-evaluated alerts (from ThanosRuler), which do not originate on any spoke and are not covered by the spoke-side metric.

Note: The console UI heatmap and component health views do not depend on this metric — they are computed from real-time Hub AM alerts in the monitoring-plugin cache. This metric serves historical queries, ad-hoc PromQL analysis, and future Perses/Grafana dashboards.

## Motivation

The spoke-side `alerts_effective_*` metric is being built for the single-cluster alerting UI. On the hub, the existing `ALERTS` metric in hub Thanos lacks ARC-applied labels, silence awareness, and disable awareness — it cannot power enriched historical queries. Federating the spoke-side `alerts_effective_*` metric to hub Thanos closes this gap for spoke alerts. However, hub-evaluated alerts (from ThanosRuler) are not represented in any spoke metric — they need a separate mechanism to appear alongside spoke alerts in hub Thanos for a complete fleet-wide view.

### User Stories

1. As a platform admin, I want the spoke-side `alerts_effective_*` metric federated to hub Thanos, so I can run fleet-wide historical queries with post-ARC labels and silence state.
2. As an SRE, I want hub-evaluated alerts (from ThanosRuler) to also appear in `alerts_effective_*` on hub Thanos, so historical queries and dashboards cover the full fleet — not just spoke alerts.

### Goals

1. Federate the spoke-side `alerts_effective_active_at_timestamp_seconds` metric to hub Thanos via the metrics-collector.
2. Define how hub-evaluated alerts are included in the same metric on hub Thanos.
3. Document the data source mapping for real-time vs. historical alert views.

### Non-Goals

- Defining the `alerts_effective_*` metric itself (labels, values, generation mechanism) — this is the single-cluster enhancement scope (see [alerts-ui-management](alerts-ui-management.md)).
- Real-time alert display (see [Alert Visualization](multi-cluster-alerts-visualization.md) — uses hub Alertmanager).
- Console UI heatmap (uses real-time Hub AM alerts in the monitoring-plugin cache, not this metric).
- UI wireframes for the Fleet Health Heatmap (see [UI Design](multi-cluster-alerts-ui-design.md)).

## Proposal

### Spoke metric federation

The `alerts_effective_active_at_timestamp_seconds` metric is generated on each spoke as part of the single-cluster alerting enhancement (see [alerts-ui-management](alerts-ui-management.md)). For multi-cluster use, it must be federated to hub Thanos:

- Add `alerts_effective_active_at_timestamp_seconds` to the metrics-collector federation allowlist.
- On hub Thanos, the metric gets the `cluster` and `clusterID` labels (added by MCOA addon write relabel configs). `managed_cluster` is NOT available (stripped by metrics-collector).
- The metric carries post-ARC labels (`openshift_io_alert_rule_id`, component, layer), silence state (`alertstate=silenced`), and excludes disabled (ARC-dropped) alerts.
- **Dashboard enrichment via `acm_managed_cluster_labels`:** Dashboards use `group_left` joins with the existing `acm_managed_cluster_labels` metric (produced by MCE, already federated to hub Thanos) at query time — e.g., `label_replace(acm_managed_cluster_labels, "cluster", "$1", "name", "(.*)")`. This is preferred because `group_left` always reflects the current ManagedCluster labels — if a cluster's region changes or a new label is added, dashboards immediately pick up the new value across all historical queries. Baking labels at federation time would leave stale values on historical data. No new metric needed. See [UI Design](multi-cluster-alerts-ui-design.md#goals) for the full cluster label enrichment design.
- The standard `ALERTS` metric on hub Thanos does NOT carry ARC labels or silence/disable awareness. The federated `alerts_effective_*` metric supersedes it for enriched historical queries.

See the [Label topology across the stack](multi-cluster-alerts-visualization.md#label-topology-across-the-stack) table for which labels are available at each data sink.

### Hub alerts

Hub-evaluated alerts (from ThanosRuler) go directly to Hub Alertmanager and do not originate on any spoke. They are not covered by the spoke-side `alerts_effective_*` metric. To have a complete fleet-wide view in hub Thanos, hub alerts need their own path into this metric.

**Key differences from spoke alerts:**
- Hub alerts do not go through ARC — no relabeling gap to close.
- Hub alerts may or may not have a `cluster` label, depending on the PromQL expression.
- The only gap vs. spoke alerts is **silence awareness** — ThanosRuler produces an `ALERTS` metric, but silenced hub alerts still appear as `alertstate=firing`.

**Open design question.** Options:
- **(a) Hub-side exporter** — a sidecar on the hub reads from Hub Alertmanager API and produces `alerts_effective_*` for hub-evaluated alerts, consistent with the spoke approach. Provides silence awareness and a uniform metric across hub and spoke alerts.
- **(b) Use ThanosRuler `ALERTS` metric as-is** — hub alerts have no ARC, so the only gap is silence state. Accept this limitation for dashboards; the real-time console UI already handles hub silences correctly via Hub AM.
- **(c) Hybrid** — use ThanosRuler `ALERTS` for hub alerts in dashboards, and rely on the console UI cache for silence-accurate real-time views.

### Historical Alert Views

Hub Alertmanager is a transient, point-in-time store — it can only answer "what is firing right now?" For any historical or over-time alert view, the UI must query metrics stored in hub Thanos (S3):

| Use case | Data source | Notes |
|----------|-------------|-------|
| Real-time firing/silenced alerts | Hub Alertmanager | Near real-time (~30s-1min). Primary source for `GET /alerting/alerts` on the hub. |
| Fleet Health Heatmap, component health | Hub Alertmanager (via monitoring-plugin cache) | Real-time, computed in-process from live alert instances. No recording rules needed. |
| Alert trend over time (e.g., "when did this alert start/stop firing?") | `ALERTS` metric on hub Thanos | Historical, but lacks ARC labels and silence awareness. |
| Enriched alert history (with component, layer, silence state) | `alerts_effective_active_at_timestamp_seconds` on hub Thanos | Post-ARC labels, `alertstate=silenced`, ~5 min granularity. |

For MVP, the UI focuses on the real-time alerts page (hub AM). Historical alert views are a future enhancement that depends on the `alerts_effective_*` metric being deployed and federated.

### Recording Rules (Post-MVP)

Recording rules over `alerts_effective_*` (e.g., pre-aggregated cluster health counts, component health by severity) are a future optimization for Perses/Grafana dashboards. They are not required for the console UI, which computes health from real-time Hub AM alerts in the monitoring-plugin cache. Dashboards can query `alerts_effective_*` directly until scale requires pre-aggregation.

### Implementation Impact (MCO Adoption)

- Add `alerts_effective_active_at_timestamp_seconds` to the metrics-collector federation allowlist so it is federated to hub Thanos.
- Use the existing `acm_managed_cluster_labels` metric (produced by MCE, already federated) for `group_left` joins with `alerts_effective_*` and other federated metrics in dashboards. No new metric implementation needed.
- Determine the hub alerts approach (see [Hub alerts](#hub-alerts) open question) and implement the chosen mechanism.

### Workflow Description

### API Extensions

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

#### OpenShift Kubernetes Engine

### Implementation Details/Notes/Constraints

### Risks and Mitigations

See Risks & Mitigations below.

### Drawbacks

## Risks & Mitigations

- **Federation latency**: the metrics-collector federates at ~5 min intervals. Historical queries and dashboards reflect this latency. Acceptable for their use cases; the real-time alerts page and heatmap use hub Alertmanager instead.

## Open Questions

- **Hub alerts mechanism:** How are hub-evaluated alerts (from ThanosRuler) included in `alerts_effective_*` on hub Thanos? See [Hub alerts](#hub-alerts) for options. The main gap is silence awareness.

## Alternatives (Not Implemented)

## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

