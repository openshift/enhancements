---
title: multi-cluster-alerts-visualization
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
  - "/enhancements/monitoring/multi-cluster-alert-effective-metric.md"
  - "/enhancements/monitoring/alerts-ui-management.md"
  - "/enhancements/monitoring/alert-rule-classification-mapping.md"
---
# Multi-Cluster Alert Visualization (Read Path)

This proposal covers the read path for multi-cluster alert visualization: displaying spoke and hub alerts in a unified, enriched view on the hub console. It is Part 1 of the [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) umbrella proposal.

For shared context on the existing alert forwarding infrastructure, alert data storage lifecycle, and the ACM alerting developer preview, see the [Current State](multi-cluster-alerts-ui-management.md#current-state) section of the parent proposal.

## Summary

The hub monitoring-plugin reuses the **same single-cluster Alerting API** (`GET /alerting/alerts`). On the hub, the monitoring-plugin is configured to point to the hub Alertmanager as its backend, which receives both spoke-forwarded alerts (with `managed_cluster` label) and ThanosRuler alerts.

The `GET /alerting/alerts` endpoint on the hub assembles a complete, post-relabel view of all alerts across the fleet. It uses hub Alertmanager as the primary real-time data source, classifies alerts locally using the hub-side classifier (`matcher.go`), enriches with spoke rule metadata from the Rule Metadata Cache, and returns a unified view with classification, scope (hub vs spoke), and silence state — through the same API surface used on single clusters.

The Fleet Health Heatmap and component health views in the console UI are also computed from real-time Hub AM alerts, aggregated in the monitoring-plugin cache. The existing `acm_managed_cluster_labels` metric (produced by MCE, already federated to hub Thanos) provides the authoritative cluster inventory — it drives green tiles (healthy clusters with no alerts), fleet-complete dashboard views, and the `ManagedClusterMetricsMissing` alert for detecting non-reporting clusters.

## Motivation

Real-time alerts from spoke clusters arrive at the hub Alertmanager but lack classification metadata (component, layer), rule metadata (source, managedBy), and consistent silence state. Without a dedicated enrichment pipeline, the hub console cannot display a unified, classified view of fleet alerts — users would need to check each cluster individually to see the full context of each alert.

### User Stories

1. As a fleet admin, I want to see all firing and silenced alerts across all managed clusters in a single view with classification and scope, so I can identify issues without switching between cluster consoles.
2. As an SRE, I want alerts on the hub to show component and layer classification and rule metadata (source, managedBy) even for alerts from spokes, so I can prioritize and route issues effectively.
3. As a namespace owner, I want to filter hub alerts by namespace across all clusters, so I can focus on alerts affecting my workloads.
4. As a Red Hat observability developer, I need alerts on the hub to carry full rule metadata (alertRuleId, source, managedBy, prometheusRuleName) so the console can correctly enable management actions (edit, disable, silence) on the originating spoke clusters.

### Goals

1. Provide `GET /alerting/alerts` on the hub that assembles a unified, enriched view of all fleet alerts from hub Alertmanager — reusing the same single-cluster API.
2. Classify all alerts (hub and spoke) using the hub-side classifier (`matcher.go`) without fan-out to spokes.
3. Enrich alerts with rule metadata (alertRuleId, source, managedBy) via the Rule Metadata Cache.
4. Reflect silence state on the hub alerts page — hub AM silences are natively reflected (MVP); spoke silence visibility via the silence controller is Phase 2 (see [SilenceRule CRD](multi-cluster-silence-rule-crd.md)).

### Non-Goals

- Historical alert queries (see [Alert Effective Metric](multi-cluster-alert-effective-metric.md)).
- Silence management details — hub AM silence CRUD is already implemented; see [SilenceRule CRD](multi-cluster-silence-rule-crd.md) for a future improvement adding spoke propagation and declarative management.
- Write operations on alert rules (see [Hub Rule Management](multi-cluster-hub-rule-management.md)).
- UI wireframes and feature prioritization (see [UI Design](multi-cluster-alerts-ui-design.md)).

## Proposal

### Alerts Enrichment Pipeline

On the hub, `GET /alerting/alerts` must assemble a complete, post-relabel view of all alerts across the fleet from multiple data sources. No single source has all the information needed.

#### Data sources

**Hub Alertmanager** (near real-time, ~30s-1min latency) is the primary data source for `GET /alerting/alerts` on the hub and the Fleet Health Heatmap. Spoke Prometheus sends alerts to hub Alertmanager via `additionalAlertmanagerConfigs` (injected by the MCOA endpoint operator). These alerts carry:
- `managed_cluster` label (cluster name, from spoke `external_labels`)
- ARC-applied labels (`openshift_io_alert_rule_id`, component, layer, severity overrides) on alerts whose rules have an associated AlertRelabelConfig (created via the Alerting API). Note: ARCs do not change the alerting rules evaluated by Prometheus — they only modify the alert labels sent to Alertmanager.
- ARC-dropped (disabled) alerts never fire and never reach hub Alertmanager — correct behavior
- Silences created on hub AM are already natively reflected in the hub AM response. In the future, spoke-local silences could be replicated to hub AM by the silence controller (from [SilenceRule CRDs](multi-cluster-silence-rule-crd.md) with `managed_cluster` matcher scoping), so hub AM would also suppress spoke-silenced alerts

#### Classification and caches

**Hub-side classifier**: The hub's monitoring-plugin runs the same `matcher.go` classifier logic as each spoke. Given an alert's `alertname` and labels, it computes `alertComponent` and `alertLayer` locally — no fan-out, no cache needed for classification. This is the same classifier that powers the single-cluster Alerting API (see [alert-rule-classification-mapping](alert-rule-classification-mapping.md)).

The hub console backend maintains two caches for metadata that the classifier cannot compute locally:

- **Rule Metadata Cache** (per cluster, TTL 5 min): populated by calling each spoke's Alerting API (`GET /alerting/rules`) via ManagedClusterProxy. Provides per-rule metadata: `alertRuleId`, `source` (platform/user), `managedBy` (operator/gitops/unmanaged), `prometheusRuleName`, and `disabled` state. Indexed by `alertRuleId` for direct lookup and by `alertname` for fuzzy matching. Hub rules are read from the hub `PrometheusRule` CRDs. Warmed on startup.
- **Cluster Registry Cache** (watch-based): populated from ManagedCluster resources on the hub. Provides cluster names, labels, status, and proxy endpoints.

#### Enrichment steps

1. **Fetch alert instances** from hub Alertmanager (`GET /api/v2/alerts`). Each alert carries `alertname`, `severity`, `namespace`, `managed_cluster` (for spoke alerts), and ARC-applied labels (for alerts with an associated ARC). Hub-scoped alerts from ThanosRuler have no `managed_cluster` label.

2. **Classify by scope**: alerts with `managed_cluster` are spoke alerts; alerts without are hub alerts. Hub Alertmanager uses `status.state = "suppressed"` for both silenced and inhibited alerts. The `status.silencedBy` array distinguishes them: non-empty means silenced (map to `state=silenced`), empty with non-empty `status.inhibitedBy` means inhibited (map to `state=inhibited` or treat as silenced depending on UI requirements).

3. **Classify alerts** using the hub-side classifier (`matcher.go`). The classifier computes `alertComponent` and `alertLayer` locally from the alert's `alertname` and labels — the same logic used in the single-cluster Alerting API. This runs on the hub with no fan-out to spokes.

4. **Enrich with rule metadata** from the Rule Metadata Cache:
   - For alerts with `openshift_io_alert_rule_id` label (rules that have an ARC id stamp): direct O(1) lookup by alertRuleId in the cache. Adds enrichment fields: `alertRuleId`, `source` (platform/user), `prometheusRuleName`, `managedBy`.
   - For alerts without `openshift_io_alert_rule_id` (unmodified platform rules — the majority): match by `alertname` + `managed_cluster` in the cache. For platform rules, `alertname` is typically unique within a cluster. If multiple rules match, score by label intersection and pick the best match. Adds enrichment fields: `alertRuleId`, `source`, `managedBy`.
   - For hub alerts: look up in hub rule definitions by alertname. Adds enrichment fields: `alertRuleId`, `source=hub`, `managedBy` (operator for defaults, gitops/unmanaged for custom).
   - Cache miss: return alert with classification (from the classifier) but without rule metadata; trigger async cache refresh.

5. **Enrich with cluster metadata** from the Cluster Registry Cache: for spoke alerts, look up the `managed_cluster` in the Cluster Registry Cache and attach selected ManagedCluster labels (e.g., `region`, `availability_zone`, `provider`, `env`) as additive enrichment fields on the API response. An allowlist controls which labels are exposed. These fields enable UI filtering and grouping by cluster metadata. The same cluster labels are also enriched at two other levels (all MVP): a hub AM relabel config controller injects them onto alerts in Alertmanager for notification routing, and the existing `acm_managed_cluster_labels` metric (produced by MCE, already federated to hub Thanos) exposes them for `group_left` joins in dashboards (always reflects current labels, even on historical queries). All three mechanisms share a single allowlist. See [UI Design — Goals](multi-cluster-alerts-ui-design.md#goals) for details.

6. **Silence state**: for MVP, silences created on hub AM are natively reflected — alerts suppressed by hub AM silences appear as `state=suppressed` (mapped to `silenced`). Spoke-local silences are not visible on hub AM in MVP. In Phase 2, the silence controller reconciles `SilenceRule` CRDs to hub AM with `managed_cluster` matcher scoping, making spoke-silenced alerts visible as suppressed on the hub. See [SilenceRule CRD](multi-cluster-silence-rule-crd.md).

7. **Filter, sort, paginate** based on query parameters (state, severity, cluster, namespace, component, source, alertname, cluster_labels, arbitrary label filters).

#### Prerequisites

- **Spoke Alerting API (monitoring-plugin)**: The Rule Metadata Cache depends on each spoke exposing the Single-Cluster Alerting API (`GET /alerting/rules`) via the monitoring-plugin for rule metadata (`alertRuleId`, `source`, `managedBy`, `prometheusRuleName`, `disabled`). This requires the monitoring-plugin to be deployed on all managed clusters. The monitoring-plugin is available from OpenShift 4.18+. Spokes running older versions will still get classification (from the hub-side classifier) but will lack rule metadata (`alertRuleId`, `source`, `managedBy`).
- **ManagedClusterProxy**: Required for the hub console backend to reach spoke Alerting APIs and spoke Alertmanagers. Must be enabled on all managed clusters.
- **MCOA endpoint operator**: Must be configured to inject `additionalAlertmanagerConfigs` on spokes (existing behavior) for alerts to reach hub Alertmanager.

#### Historical alert views

Hub Alertmanager is a transient, point-in-time store — it can only answer "what is firing right now?" For historical or over-time alert views, the UI queries metrics stored in hub Thanos. For MVP, the UI focuses on the real-time alerts page (hub AM). See [Alert Effective Metric — Historical Alert Views](multi-cluster-alert-effective-metric.md#historical-alert-views) for the full data source mapping.

### API Endpoints — Unified Single-Cluster API

Base path: `/api/v1/alerting`

> Note: GET endpoints should remain compatible with upstream Thanos/Prometheus (query parameters and response schemas) wherever applicable to enable native Perses integration.

The hub monitoring-plugin reuses the **same Alerting API** used on single clusters. There are no separate `/hub/*` endpoints. Instead, the monitoring-plugin on the hub is configured to point to the hub Alertmanager as its backend.

- Alerts (instances)
  - `GET    /alerting/alerts`        — On the hub, returns alert instances from hub Alertmanager — which includes both spoke-forwarded alerts (with `managed_cluster` label) and hub ThanosRuler alerts. The enrichment pipeline (classifier + Rule Metadata Cache) adds classification and spoke rule metadata. See [Alerts Enrichment Pipeline](#alerts-enrichment-pipeline) for the full data flow.

- Semantics:
  - Request/response schemas are identical to the single-cluster Alerting API and include `ruleId`, `labels`, `annotations`, `mapping` (component/impactGroup), `source` (`platform`|`user`|`hub`), `managedBy` (`operator`|`gitops`|`""`), and `disabled` flags.
  - Filters mirror the single-cluster API (`name`, `group`, `component`, `severity`, `state`, `source`, `triggered_since`, arbitrary label filters) plus multi-cluster filters (`cluster`, `cluster_labels`).
  - The `cluster` filter selects alerts by `managed_cluster` label. Omitting it returns alerts from all clusters plus hub alerts.
  - Read paths keep Prometheus/Thanos compatibility where applicable.

#### Implementation impact
- Current behavior: managed clusters forward alerts to hub Alertmanager (via `additionalAlertmanagerConfigs`). The metrics-collector federates the `ALERTS` metric to hub Thanos.
- Required changes:
  - Deploy the hub monitoring-plugin with the same Alerting API code, configured to point to hub Alertmanager as its backend (`--alertmanager` flag or `UIPlugin` CR). The enrichment pipeline (classifier + Rule Metadata Cache) runs on the hub instance, adding spoke rule metadata via ManagedClusterProxy. See [Enrichment steps](#enrichment-steps).

### Data Model

#### Label topology across the stack

Understanding which labels are available at Hub Alertmanager is critical for the enrichment pipeline on the hub's `GET /alerting/alerts`.

| Label | Spoke Alertmanager | Hub Alertmanager |
|-------|--------------------|-----------------|
| `managed_cluster` | YES (via external_labels) | YES (via additionalAlertmanagerConfigs) |
| `cluster` | NO | NO |
| `clusterID` | NO | NO |
| `openshift_io_alert_rule_id` | YES (for rules with ARC id stamp) | YES (for rules with ARC id stamp) |
| `openshift_io_alert_rule_component` | YES (for rules with ARC override or explicit label in rule definition) | YES (for rules with ARC override or explicit label in rule definition) |
| `openshift_io_alert_rule_layer` | YES (for rules with ARC override or explicit label in rule definition) | YES (for rules with ARC override or explicit label in rule definition) |
| Disabled alerts | absent (ARC Drop) | absent (ARC Drop) |
| Silenced alerts | suppressed state | suppressed (hub AM silences in MVP; + spoke silences via silence controller in Phase 2) |

Key implications:
- **Hub Alertmanager** has `managed_cluster` for cluster identification, ARC-applied labels for classification, and receives alerts in near real-time. For MVP, hub AM silences are natively reflected. In Phase 2, spoke silences are reconciled to hub AM by the silence controller (from [`SilenceRule` CRDs](multi-cluster-silence-rule-crd.md)) with `managed_cluster` matcher scoping, so hub AM also reflects spoke silence state.
- **ARC-applied labels** are only present on alerts with an ARC override or explicit rule labels. The majority of platform alerts lack them. The hub-side classifier computes classification for ALL alerts locally (see [Classification and caches](#classification-and-caches)).

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

- **Aggregation freshness at scale**: fleet caches can become stale. Define SLAs and TTLs. Provide on‑demand refresh for critical views, and progressively load details. Fall back to sampled or approximate aggregates for heatmaps when needed.
- **Connectivity or proxy reliability**: outages or high latency in the ManagedClusterProxy on the hub may disrupt reads. Use retries with jitter, per‑cluster backoff, circuit breakers, and a degraded read mode backed by cached data.
- **Cache refresh fan-out at scale**: the Rule Metadata Cache refreshes by calling each spoke's Alerting API every 5 min for rule metadata (alertRuleId, source, managedBy). Classification (component/layer) is computed locally by the hub-side classifier and does not require fan-out. At 1000 clusters, rule metadata refresh produces ~3.3 calls/sec sustained. Use bounded concurrency (e.g., 20 parallel requests), staggered refresh intervals (jitter), and prioritize clusters with recent changes. Monitor cache refresh latency and spoke API error rates.

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

