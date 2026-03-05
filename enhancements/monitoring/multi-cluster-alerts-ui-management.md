---
title: multi-cluster-alerts-ui-managment
authors:
  - "@sradco"
reviewers:
  - "@jan--f"
  - "@jgbernalp"
approvers:
  - "@jan--f"
  - "@jgbernalp"
api-approvers:
  - TBD
creation-date: 2026-01-12
last-updated: 2026-03-04
tracking-link:
  - "https://issues.redhat.com/browse/CNV-46597"
  - "https://issues.redhat.com/browse/CNV-62972"
---
# Managing Alerts in the Multi-Cluster Console

## Table of Contents

- [Summary](#summary)
- [Motivation](#motivation)
  - [Problem Statement](#problem-statement)
  - [User Stories](#user-stories)
- [Goals](#goals)
- [Proposed Features](#proposed-features)
- [Non-Goals](#non-goals)
- [Related Enhancement Proposals](#related-enhancement-proposals)
- [Proposal](#proposal)
  - [Architecture](#architecture)
  - [Alerts Enrichment Pipeline](#alerts-enrichment-pipeline)
    - [Data sources](#data-sources)
    - [Enrichment caches](#enrichment-caches)
    - [Enrichment steps](#enrichment-steps)
    - [Prerequisites](#prerequisites)
    - [Label mapping across data sources](#label-mapping-across-data-sources)
  - [Silence Sync Controller](#silence-sync-controller)
  - [Hub Rule Management](#hub-rule-management)
    - [Current hub rule storage](#current-hub-rule-storage)
    - [Hub rule ownership model](#hub-rule-ownership-model)
    - [Hub rule tiers in the API](#hub-rule-tiers-in-the-api)
    - [Hub rule disable mechanism](#hub-rule-disable-mechanism)
    - [Hub alert classification labels](#hub-alert-classification-labels)
- [Fleet Health Heatmap & Filtering](#fleet-health-heatmap--filtering)
  - [Fleet landing page](#fleet-landing-page)
  - [Backend data for the Heatmap](#backend-data-for-the-heatmap)
  - [Proposed UI in Multi-Cluster Console](#proposed-ui-in-multi-cluster-console)
  - [Additional Points to Consider](#additional-points-to-consider)
  - [Feature Prioritization](#feature-prioritization)
- [API Endpoints](#api-endpoints)
  - [Hub Alerts API extension](#hub-alerts-api-extension)
  - [Batch Operations API](#batch-operations-api)
- [Data Model](#data-model)
- [Metrics and Recording Rules](#metrics-and-recording-rules)
- [Migration](#migration)
- [GitOps / Argo CD Compliance](#gitops--argo-cd-compliance)
- [Pain Points Addressed by this Design](#pain-points-addressed-by-this-design)
- [Risks & Mitigations](#risks--mitigations)
- [Open Questions](#open-questions)
- [Test Plan](#test-plan)
- [Graduation Criteria](#graduation-criteria)
- [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy)
- [Version Skew Strategy](#version-skew-strategy)
- [Operational Aspects of API Extensions](#operational-aspects-of-api-extensions)
- [Support Procedures](#support-procedures)

## Summary
Introduce a centralized, multi‑cluster alerting experience on the hub cluster that leverages the Single‑Cluster Alerting API for fleet‑wide visibility and management.
The UX follows a funnel: a Fleet Health Heatmap for quick at‑a‑glance multi-clusters health status, drill‑down into per‑cluster components health  and drill-down to the specific component alerts, and unified management of alert rules via the Alerting API.

This proposal reuses the new Alerting API for read and update paths and extends it for multi‑cluster operations where needed (such as managing hub alerts).

## Motivation
- While it's possible to customize built‑in alerting rules on individual clusters (see [alert overrides](https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/alert-overrides.md)), doing so consistently across many clusters is cumbersome and error‑prone. It requires templating and applying per‑cluster YAML, coordinating rollouts, and there is no fleet‑level validation or preview. Built‑in rules and alerts also remain visible per‑cluster after overrides, leading to inconsistent UX across the fleet.
- Operational teams need a fleet‑aware console and API to define, target, and audit alerting rules and silences across scopes such as fleet‑wide (hub), groups of clusters, or individual clusters, so they do not have to repeat the same action on each cluster.
- A unified multi‑cluster interface should enable creating, cloning, and disabling rules or setting silences across selected clusters, viewing aggregated and per‑cluster firing status, resolving precedence between global and local overrides, and preserving intended behavior through cluster lifecycle events and upgrades.

### Problem Statement
Fleet administrators struggle with generic per‑cluster alerting rules that create cross‑cluster noise, lack fleet‑level context, and are difficult to standardize and target by cluster labels or sets, making consistent thresholds, severity, and routing across many clusters error‑prone.

### User Stories

1. **Fleet overview and drill‑down**
   - As a Platform Admin, I want to see the health status of my clusters, specifically for my “Production” clusters to quickly identify where critical alerts are firing, and have a way to quickly drill down to see the impacted components and only then to the specif relevant alerts.

2. **Create Global (hub) alert**
   - As an SRE, I want to define a Global alert that evaluates on the hub (MCOA Thanos Ruler) over federated data and routes to the appropriate global receiver.

3. **Batch‑apply a rule to selected clusters**
   - As an Ops Lead, I want to apply or update a specific alert rule and deploy it across a list of specific clusters (that I can easily search for by their names, labels, versions, etc.) in one action, without visiting each cluster UI.

4. **View global vs cluster‑local alerts**
   - As an SRE, I want to distinguish alerts running on the hub (global scope) from those running on a specific cluster and navigate between them seamlessly.

## Goals

The primary goal is to provide a comprehensive alerting management UI that directly addresses the problems identified through user feedback, research, and competitive analysis.
The proposed features are intended to reduce alerts noise and improve the overall user experience for monitoring and responding to issues, including surfacing prioritized next actions based on aggregated cluster and component health, so users can address the most impactful issues first.

1. Provide a Fleet Clusters health visualization to inspect clusters status at a glance, with filtering and grouping by labels (such as name, health, region, provider) and optional weighted priority (such as node count, pods count, VMs count, CPU count, alerts count).
2. Support batch operations to apply, update, or delete alerting rules across a selected set of clusters.
3. Aggregate and display alert rules and alert instances across the fleet with post‑relabel context, like in the single cluster.
4. Improve correctness and performance by reusing the Single‑Cluster Alerting API and extend it where necessary, such as Hub alerts.
5. Manage Global alerting rules on the hub (MCOA Thanos Ruler) and local rules on selected clusters from a unified UI.
6. Optionally propagate selected cluster labels to Prometheus `external_labels` to enable label‑based routing. - Not MVP
7. Enforce access control consistent with hub/cluster RBAC and ensure safe multi‑cluster operations via the console backend.
8. Keep GitOps compliance: generated resources remain declarative and read‑only when owned by GitOps apps.

## Proposed Features

#### User Interface
The user interface will be redesigned, with a new **Observe > Alerting** page that highlights new grouping and components functionality.

#### Alerts Tab
The multi‑cluster Alerts page mirrors the single‑cluster Alerts page for familiarity and consistency. The key difference is the addition of a **Cluster** column (and scope) so users can see and filter alerts per cluster alongside the existing fields.

#### Management Tab
The Management tab mirrors the single‑cluster design and capabilities for familiarity. The multi‑cluster differences are:

- The list aggregates alerting rules from all managed clusters and groups them by alert rule definition (alert name plus its full label set) to provide a unified view.
- Users can create, update, delete, enable, and disable alerting rules (subject to rule type and RBAC) and apply those changes to a selected set of clusters via the Alerting API.
- Managing hub (global) alerts is supported in the same workflow.

All other interaction patterns remain consistent with the single‑cluster experience.

## Non‑Goals
- Deep RBAC beyond native Kubernetes permissions.
- Operators reacting to user modifications (operator code remains unchanged).

## Related Enhancement Proposals
- https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/alert-overrides.md
- https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/user-workload-monitoring.md

## Proposal

### Architecture

Key flows:
- UI authenticates and calls the console backend, which invokes the Unified Alerting API with the user’s identity and RBAC context.
- Hub (Global) scope: hub alerting rules and silences are read and written via the Unified Alerting API. For MVP, the API reads and writes hub rules directly in the existing ConfigMaps (`thanos-ruler-default-rules` for operator-managed defaults, `thanos-ruler-custom-rules` for user-created custom rules) that ThanosRuler already watches. A future iteration introduces a `HubAlertingRule` CRD as the single source of truth with a reconciler generating ConfigMaps (see Hub Rule Management).
- Cluster‑scoped operations: alerting rule and silence definitions are stored on the spoke clusters and are read/written via the Unified Alerting API through the ManagedClusterProxy on the hub to each target cluster’s APIs.
- Alerts read path: `GET /hub/alerts` uses the hub Alertmanager as the primary source for alert instances (near real-time, ~30s-1min latency). Spoke Prometheus forwards alerts to the hub Alertmanager via `additionalAlertmanagerConfigs` (injected by the MCOA endpoint operator). Each spoke alert carries a `managed_cluster` label and ARC-applied labels when the rule has been classified. Hub alerts from ThanosRuler also arrive at the hub Alertmanager. The endpoint enriches alerts with rule metadata from a Rule Metadata Cache. Spoke silence state is natively available on hub Alertmanager via a new silence sync controller that replicates spoke silences (see Alerts Enrichment Pipeline).
- Firing alerts ingestion: managed clusters forward alerts to the hub Alertmanager (near real-time, primary source for the alerts page) AND produce the `alerts_effective_active_at_timestamp_seconds` metric on spoke Prometheus, which is federated to hub Thanos via the metrics-collector (~5 min interval). The `alerts_effective_*` metric carries post-ARC labels and `alertstate=silenced` for silenced alerts, and is used for recording rules and aggregated health views (heatmap, component tables) where ~5 min latency is acceptable.
- Batch endpoints: preview (dry‑run) and apply changes to a selected set of clusters. Responses include per‑cluster status, errors, and summaries.
- Rule identity and grouping: read paths aggregate rule definitions by alert name plus full label set, and aggregate alert instances with post‑relabel context for a consistent fleet view.
- GitOps ownership: when resources are owned by GitOps, the API treats them as read‑only and surfaces guidance rather than mutating in‑cluster resources.
- Conflict/drift handling: server‑side validation with optimistic concurrency (resourceVersion) and idempotent apply. The API reports conflicts and drift in per‑cluster results.
- Silences: support hub‑scoped and cluster‑scoped silences. Label‑based selectors are forwarded to the appropriate Alertmanager(s).
- Silences scope policy: Hub-initiated silences for alerts originating on spokes are created on the respective spoke Alertmanager(s) via ManagedClusterProxy. Hub-scoped alerts are silenced on the hub Alertmanager only. A silence sync controller replicates spoke Alertmanager silences to hub Alertmanager with `managed_cluster` matcher scoping, so hub AM natively suppresses spoke alerts -- no read-time merge or Silence Cache is needed.

Rationale for server‑side aggregation and routing:
- Provides a canonical, post-relabel effective view and mediates validated updates to `PrometheusRule`, `AlertingRule` and `AlertRelabelConfig` across many clusters, and `HubAlertingRule` CRDs on the hub.
- Enforces consistent RBAC, ownership checks (GitOps), and conflict handling while reducing client fan‑out.
- Reuses Single‑Cluster Alerting API semantics to minimize duplication and ease maintenance; extends with batch and preview for fleet operations.

### Alerts Enrichment Pipeline

The `GET /hub/alerts` endpoint must assemble a complete, post-relabel view of all alerts across the fleet from multiple data sources. No single source has all the information needed.

#### Data sources

Two data sources carry alert instances to the hub. Each serves a different purpose based on its latency and label coverage:

1. **Hub Alertmanager** (near real-time, ~30s-1min latency) — spoke Prometheus sends alerts to the hub Alertmanager via `additionalAlertmanagerConfigs` (injected by the MCOA endpoint operator). These alerts carry:
   - `managed_cluster` label (cluster name, from spoke `external_labels`)
   - ARC-applied labels (`openshift_io_alert_rule_id`, component, layer, severity overrides) for rules that have been classified or modified via the Alerting API
   - ARC-dropped (disabled) alerts never fire and never reach hub Alertmanager — correct behavior
   - Spoke-local silences are replicated to hub Alertmanager by the silence sync controller (with `managed_cluster` matcher scoping), so hub AM natively suppresses spoke-silenced alerts
   - **Used for:** `GET /hub/alerts` (real-time alerts page)

2. **`alerts_effective_active_at_timestamp_seconds` metric on hub Thanos** (~5 min latency, metrics-collector federation interval) — a new spoke-side metric that represents the final effective alert state after ARC relabeling is applied. Federated to hub Thanos via the metrics-collector. Key properties:
   - Carries post-ARC labels: `openshift_io_alert_rule_id`, component, layer, severity (after overrides), and all routing-relevant labels
   - Disabled alerts (ARC `action: Drop`) are absent — they never fire
   - Silenced alerts are present with `alertstate=silenced`
   - The value equals the alert `activeAt` timestamp
   - Retains all original alert labels (no label dropping) — preserves full context for queries, drill-down, and matching
   - On hub Thanos: gets the `cluster` label (from MCOA addon write relabel configs) for cluster identification
   - **Used for:** recording rules (fleet health heatmap, component health aggregation) and historical alert queries. Not used for the real-time alerts page.

**Primary source for `GET /hub/alerts`:** Hub Alertmanager. It receives alerts in near real-time from spoke Prometheus (on every evaluation cycle). A silence sync controller replicates spoke Alertmanager silences to hub AM with `managed_cluster` matcher scoping, so hub AM natively reflects spoke silence state without a read-time cache.

**Source for recording rules and aggregated health:** `alerts_effective_active_at_timestamp_seconds` on hub Thanos. The ~5 min federation latency is acceptable for heatmap health counts and pre-aggregated component tables. This metric carries post-ARC labels and silence state (`alertstate=silenced`) that the standard ALERTS metric lacks, enabling component-based recording rules and silence-aware health counts.

#### Enrichment caches

The hub console backend maintains two caches to avoid fan-out on every request:

- **Rule Metadata Cache** (per cluster, TTL 5 min): populated by calling each spoke's Alerting API (`GET /alerting/rules`) via ManagedClusterProxy. The spoke API already returns fully relabeled rules with all metadata (alertRuleId, source, component, layer, managedBy, disabled, prometheusRuleName). Indexed by `alertRuleId` for direct lookup and by `alertname` for fuzzy matching. Hub rules are read from the hub rule ConfigMaps. Warmed on startup.
- **Cluster Registry Cache** (watch-based): populated from ManagedCluster resources on the hub. Provides cluster names, labels, status, and proxy endpoints.

#### Enrichment steps

1. **Fetch alert instances** from hub Alertmanager (`GET /api/v2/alerts`). Each alert carries `alertname`, `severity`, `namespace`, `managed_cluster` (for spoke alerts), and ARC-applied labels (for rules that have been modified). Hub-scoped alerts from ThanosRuler have no `managed_cluster` label.

2. **Classify by scope**: alerts with `managed_cluster` are spoke alerts; alerts without are hub alerts. Hub Alertmanager uses `status.state = "suppressed"` for both silenced and inhibited alerts. The `status.silencedBy` array distinguishes them: non-empty means silenced (map to `state=silenced`), empty with non-empty `status.inhibitedBy` means inhibited (map to `state=inhibited` or treat as silenced depending on UI requirements).

3. **Enrich with rule metadata** from the Rule Metadata Cache:
   - For alerts with `openshift_io_alert_rule_id` label (rules that have an ARC with an id stamp): direct O(1) lookup by alertRuleId in the cache. Adds: `source` (platform/user), `prometheusRuleName`, `managedBy`.
   - For alerts without `openshift_io_alert_rule_id` (unmodified platform rules -- the majority): match by `alertname` + `managed_cluster` in the cache. For platform rules, `alertname` is typically unique within a cluster. If multiple rules match, score by label intersection and pick the best match. Adds: `alertRuleId`, `source`, `component`, `layer`, `managedBy`.
   - For hub alerts: look up in hub rule definitions by alertname. Adds: `source=hub`, component, managedBy (operator for defaults, gitops/unmanaged for custom).
   - Cache miss: return alert with partial enrichment (no classification); trigger async cache refresh.

4. **Spoke silence state** is already reflected by hub Alertmanager. The silence sync controller replicates spoke silences to hub AM with `managed_cluster` matcher scoping, so spoke alerts that are silenced on the spoke appear as `state=suppressed` (mapped to `silenced`) in the hub AM response. No additional read-time matching is needed.

5. **Filter, sort, paginate** based on query parameters (state, severity, cluster, namespace, component, source, alertname, arbitrary label filters).

#### Prerequisites

- **Spoke Alerting API (monitoring-plugin)**: The Rule Metadata Cache depends on each spoke exposing the Single-Cluster Alerting API (`GET /alerting/rules`) via the monitoring-plugin. This requires the monitoring-plugin to be deployed on all managed clusters. The monitoring-plugin is available from OpenShift 4.18+. Spokes running older versions will not have rule metadata in the cache; their alerts will be returned with partial enrichment (no classification labels, no `alertRuleId`).
- **ManagedClusterProxy**: Required for the hub console backend to reach spoke Alerting APIs and spoke Alertmanagers. Must be enabled on all managed clusters.
- **MCOA endpoint operator**: Must be configured to inject `additionalAlertmanagerConfigs` on spokes (existing behavior) for alerts to reach hub Alertmanager.

#### Label mapping across data sources

See the full label topology table in the Data Model section. Key points for enrichment:
- Hub Alertmanager has `managed_cluster` and ARC-applied labels (`openshift_io_alert_rule_id`, component, layer) -- primary source for the real-time alerts page.
- `alerts_effective_active_at_timestamp_seconds` on hub Thanos has `cluster` and ARC-applied labels plus `alertstate` -- primary source for recording rules and aggregated health views (not for real-time alerts due to ~5 min federation latency).
- The `cluster` label on hub Thanos and the `managed_cluster` label on hub Alertmanager both identify the cluster by name (same value, different key).
- ARC-applied labels are only present on alerts whose rules have an ARC id stamp (rules modified via the Alerting API).

### Silence Sync Controller

A silence sync controller replicates spoke Alertmanager silences to hub Alertmanager so that hub AM natively suppresses spoke-silenced alerts without a read-time cache in the console backend.

#### Deployment

The controller runs on the hub as a single deployment in `open-cluster-management-observability`. It uses the Cluster Registry Cache (ManagedCluster watch) to discover spoke clusters and their ManagedClusterProxy endpoints.

#### Sync mechanism

The controller periodically polls each spoke Alertmanager (`GET /api/v2/silences` via ManagedClusterProxy) and reconciles the state on hub AM:

- **Create**: when a new active silence is found on a spoke, the controller creates a replica on hub AM. The replica includes all original matchers plus an additional `managed_cluster=<cluster-name>` matcher to scope it to that spoke's alerts. A label or annotation `sync.source=<cluster-name>/<silence-id>` is added to the hub silence comment for traceability and to prevent conflicts with user-created hub silences.
- **Update**: if a spoke silence's `endsAt` is extended or matchers change, the controller expires the old hub replica and creates a new one.
- **Expire/Delete**: when a spoke silence expires or is deleted, the controller expires the corresponding hub replica. Expired silences on hub AM are cleaned up by Alertmanager's built-in GC.

#### Polling interval and consistency

The controller polls each spoke on a configurable interval (default 30s). Between polls, there is a window where a newly created or expired spoke silence is not yet reflected on hub AM. This is acceptable because:
- New silences: the alert may appear as firing for up to 30s on the hub alerts page before being suppressed. Spoke Alertmanager is already suppressing notifications immediately.
- Expired silences: the hub may continue suppressing the alert for up to 30s after the spoke silence expires. No missed notifications — spoke AM resumes notifications immediately.

#### Spoke unreachable

When a spoke is unreachable, the controller retains existing hub replicas for that spoke (stale but safe — alerts remain suppressed). After a configurable staleness TTL (default 10 min), the controller marks replicas as potentially stale but does not automatically expire them to avoid false firing alerts during transient connectivity issues. The `GET /hub/alerts` response includes a `silenceStateStale` flag for clusters whose silence state may be outdated.

#### Conflict avoidance

Hub replicas are identifiable by the `sync.source` tag in the silence comment. The controller only manages silences that carry this tag. User-created silences on hub AM (without the tag) are left untouched. This prevents conflicts between user-created hub silences and controller-managed replicas.

When a user creates a silence for a spoke alert via the API (`POST /hub/silences` with `scope: cluster:<name>`), the API creates it on the spoke AM. The sync controller then picks it up on the next poll and creates a hub replica. The user sees the silence reflected on both the spoke and the hub alerts page.

### Hub Rule Management

Hub alerting rules are evaluated by MCOA ThanosRuler over federated data from hub Thanos. ThanosRuler uses ConfigMap-based rule files, not PrometheusRule CRDs.

#### Current hub rule storage

| ConfigMap | Key | Content | Ownership |
|-----------|-----|---------|-----------|
| `thanos-ruler-default-rules` | `default_rules.yaml` | Default alerts (KubePersistentVolumeFillingUp, ViolatedPolicyReport, Thanos health) and recording rules (cluster aggregations for Grafana). Always present. | MCO operator — read-only, operator-managed. The operator overwrites on reconciliation. |
| `thanos-ruler-custom-rules` | `custom_rules.yaml` | User-defined custom alerting and recording rules. Created on demand. MCO controller watches for updates. | User-managed (direct ConfigMap edit) or GitOps-managed (ArgoCD manages the ConfigMap). |

#### Hub rule ownership model

Unlike spoke rules (where each PrometheusRule CRD has its own labels, annotations, and ownerReferences for per-rule ownership detection), hub custom rules are packed into a single ConfigMap with no per-rule metadata. This creates a mismatch with the single-cluster Alerting API's ownership model.

**MVP approach — direct ConfigMap manipulation:**

For MVP, the Alerting API reads and writes hub rules directly in the existing ConfigMaps. No new CRDs or MCO operator changes are required.

- `GET /hub/rules` parses both ConfigMaps: `thanos-ruler-default-rules` (operator defaults) and `thanos-ruler-custom-rules` (user-created). Each rule group entry is parsed, an `alertRuleId` is computed, and the response includes source, classification, and `managedBy`. The `alertRuleId` hash inputs for hub rules are: ConfigMap name + ConfigMap key + rule group name + alertname + sorted label key-value pairs. This ensures stable, reproducible IDs across API calls and is consistent with the single-cluster approach (which uses PrometheusRule CRD name + namespace + group + alertname + labels).
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
- The reconciler generates ConfigMap YAML from enabled CRDs and writes both ConfigMaps
- `GET /hub/rules` lists CRDs — one uniform source, no ConfigMap parsing
- Per-rule disable: `spec.enabled: false` excludes the rule from the ConfigMap while keeping it visible in the API
- Per-rule GitOps: individual CRDs can have ArgoCD annotations
- Per-rule optimistic concurrency: each CRD has its own `resourceVersion`
- Requires MCO operator change to adopt the CRD for default rules

#### Hub rule tiers in the API

**MVP (ConfigMap-based):**

| Tier | Source | `managedBy` | Create | Update | Disable | Delete |
|------|--------|-------------|--------|--------|---------|--------|
| Default hub rules | `thanos-ruler-default-rules` CM | `operator` | N/A | Blocked | Not supported (use silence) | Blocked |
| Custom hub rules (unmanaged) | `thanos-ruler-custom-rules` CM | `""` | `POST /hub/rules` | `PATCH /hub/rules/{id}` | Not supported (use silence or delete) | `DELETE /hub/rules/{id}` |
| Custom hub rules (GitOps) | `thanos-ruler-custom-rules` CM with ArgoCD annotations | `gitops` | N/A | Blocked — guidance to edit in Git | Blocked | Blocked |

**Future (CRD-based):**

| Tier | Source | `managedBy` | Create | Update | Disable | Delete |
|------|--------|-------------|--------|--------|---------|--------|
| Default hub rules | `HubAlertingRule` CRD (operator-owned) | `operator` | N/A (operator creates) | Blocked | Set `enabled: false` | Blocked |
| Custom hub rules (unmanaged) | `HubAlertingRule` CRD | `""` | `POST /hub/rules` | `PATCH /hub/rules/{id}` | Set `enabled: false` | `DELETE /hub/rules/{id}` |
| Custom hub rules (GitOps) | `HubAlertingRule` CRD with ArgoCD annotations | `gitops` | N/A | Blocked — guidance to edit in Git | Blocked | Blocked |

#### Hub rule disable mechanism

Hub ThanosRuler has no ARC (AlertRelabelConfig) pipeline. The disable mechanism differs from single-cluster:
- On spokes: ARC `action: Drop` prevents the alert from firing while keeping the rule definition visible in the API.
- On hub (MVP): no per-rule disable. Removing a rule from `thanos-ruler-custom-rules` deletes it. Users can silence individual alerts via the hub Alertmanager as a workaround. Default hub rules cannot be modified — use silences.
- On hub (future CRD): setting `spec.enabled: false` on the CRD removes the rule from the generated ConfigMap. ThanosRuler stops evaluating it. The rule definition remains visible in the API via the CRD.

#### Hub alert classification labels

On spokes, ARCs inject classification labels (`openshift_io_alert_rule_id`, `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`) into alerts before they reach any Alertmanager. Since `alert_relabel_configs` are global in Prometheus, these labels are present on alerts sent to hub AM via `additionalAlertmanagerConfigs` as well. Hub ThanosRuler has no ARC pipeline, so hub alerts arrive at hub AM with only the labels explicitly defined in the rule.

**Operator default rules (`thanos-ruler-default-rules`):**

The MCO operator owns these rules and overwrites them on every reconciliation. Adding classification labels directly to the rule definitions would be overwritten. For MVP, the console backend maintains a static mapping of known default rule alertnames to their classification (`component`, `layer`). This is acceptable because the default rule set is small and changes only across operator upgrades.

For a future iteration, support for `alertRelabelConfigs` on ThanosRuler should be added so that the MCO operator can configure classification labels for default rules via relabeling — the same mechanism used for the user-defined monitoring stack. This has already been requested for the user-defined monitoring stack as well.

**User-created custom rules (`thanos-ruler-custom-rules`):**

When users create hub rules via `POST /hub/rules`, the API accepts `component` and `layer` as optional metadata and writes them as labels in the Prometheus rule definition inside the ConfigMap — the same convention used for single-cluster user-defined rules. ThanosRuler evaluates the rule and the resulting alert carries these labels natively to hub AM. Users are expected to include classification labels in their rule definitions, consistent with how it is done on single-cluster.

## Fleet Health Heatmap & Filtering

### Fleet landing page
- Fleet Health Heatmap as the primary entry point.
- Visualization:
  - Default: equal‑sized grid for quick scanning.
  - Weighted mode: Treemap where tile size reflects cluster scale, such as node count or alert volume.
  - Grouping: nest tiles by common labels, such as `region` or `provider`, to visualize domains.
  - Color: strict Red/Yellow/Green by alert severity/impact.
  - View mode: toggle button to switch between Heatmap and Table views. The table lists clusters with the same filters/grouping and shows health/status columns.
- Filtering:
  - PatternFly filter toolbar: Name, Labels (such as `env=prod`), and Health status.
  - Saved searches: persist user filter sets, such as “My Prod Clusters”.
- Hub tile (The hub cluster tile in the heatmap/table):
  - Treat the Hub (MCOA) as a first‑class tile, such as “Global Platform”.
  - Click to drill into global alerts, consistent with per‑cluster interaction.

### Backend data for the Heatmap
- Aggregated health metric (recording rule) deployed to spokes and federated to hub:
  - Metric: `acm:cluster:health:critical_count`
  - Definition: counts firing alerts with `severity=critical` and `impact=cluster`
  - Flow: Spoke Prometheus → MCOA Federation → Hub UI

### Proposed UI in Multi‑Cluster Console

See additional details in the [UX Design- Alerts management](https://docs.google.com/document/d/1bB7kg-W2lLq85Dmy530STMUWJFlNPFvg08Sayc-RwK8/edit?usp=sharing)

- **Management List**: show all alerting rules. Filter and sort by cluster, name, severity, namespace, status, and labels. Saved searches.
![Alerting -> Management](assets/multi-cluster-alerts-management-ui.png)

- **Clusters Health View**:
  - Fleet landing page with a Heatmap to visualize multi‑cluster health at a glance.
  - Group clusters by common labels such as region, cloud provider, severity, or other labels to understand domain health.
  - Size tiles by different dimensions to reflect scale or impact, including number of nodes, number of pods, number of VMs, number of alerts, Total CPU Cores, Total Memory.
  - Includes two summary tables below the Heatmap:
    - “Top Firing Alerts” – aggregates the most active alerts across the fleet with counts and affected clusters.
    - “Most Impacted Components” – aggregates alert impact by component and shows health breakdown per component across clusters.
  - Example screens:
![Clusters Health – Size By](assets/multi-cluster-alerts-management-ui-heatmap1.png)
![Clusters Health – Group By](assets/multi-cluster-alerts-management-ui-heatmap2.png)
![Clusters Health – Hover with Components Health](assets/multi-cluster-alerts-management-ui-heatmap3.png)
![Clusters Health – Table View](assets/multi-cluster-alerts-management-ui-heatmap4.png)
![Clusters Health – Summary Tables](assets/multi-cluster-alerts-management-ui-heatmap5.png)

- **Alerts View**: show current firing or pending instances, silence status, and relabel context. Filter and sort by cluster, name, severity, namespace, status, and labels. Saved searches.
![Alerting -> Management](assets/multi-cluster-alerts-management-ui-alerts-page.png)

- **Alerts View (Grouped by Component)**: same page grouped by components. Shows component health for each component across clusters.
![Alerting -> Management - Components](assets/multi-cluster-alerts-management-ui-component-aggregation.png)

- **Create/Edit Alerting Rule Form**: fields for Alert Name, Summary, Description, Duration, Severity, Labels, Annotations (runbook links), Impact group & Component labels and the list of clusters to apply the alert rule to, with filtering based on the clusters names, labels, versions.

- **Bulk create/edit Alerting rules labels Form**: list common labels, Add/remove alert labels.
- **Silences List**: define matchers, duration, comment - Keep

### Additional Points to Consider

**RBAC (Multi‑cluster)**
- Read and update permissions follow hub and cluster RBAC:
  - Fleet‑scoped read is constrained by the user’s access to the hub resources and per‑cluster access, such as `ManagedCluster.view` and project or namespace access on spokes.
  - Rule updates and silences are only enabled for clusters where the user has the required permissions. Actions are automatically disabled for clusters without write access.
  - GitOps‑owned resources are treated as read‑only. The UI surfaces ownership and recommended GitOps changes.
  - Scoping options include Global (hub), Selected clusters, or Single cluster. The UI reflects the effective scope before apply.

**Terminology Alignments**
- Use consistent multi‑cluster terms:
  - Cluster vs. Namespace
  - Global (hub) alert vs. Cluster‑local alert
  - Alerting rule (definition) vs. Alert (instance)
  - Impact group (Cluster/Namespace/Compute) and Component

**Alertmanager Notifications Improvements**
- Future improvements could include multi‑cluster receivers and routing by cluster labels, such as region or team, notifications by impact group and component, and team‑scoped subscriptions honoring RBAC.

**Multi‑cluster View**
- This document defines the multi‑cluster alerting experience: a centralized, aggregated view across managed clusters and the hub.
- Key multi‑cluster aspects include a centralized view, cluster‑specific context filtering, aggregated per‑cluster and per‑component health, and batch rule operations scoped to selected clusters.

### Feature Prioritization

The features are prioritized using tags: **Must-Have**, **Should-Have**, **Could-Have**, and **Won't-Have**.

**Must-Have Features:**
- Tabs changes (Clusters Health, Alerts, Management: Alerting rules, Silence rules)
- Create user-defined alerting rules
- Create Platform alerting rules
- Create hub alerts
- Advanced filtering capabilities
- Bulk actions: disable, edit labels, edit component
- Duplicate and Delete alerting rules
- Add components and layer mapping and management

**Should-Have Features:**
- Saved filters
- Alert and alerting rule side drawer
- Add "Resource" column for node alerts
- Hub UI to manage ManagedCluster label allowlist (propagation to alert labels).
- Propagate ManagedCluster labels to the clusters Prometheus config instances.

**Could-Have Features:**
- Notifications (alertmanager receivers)
- PromQL expression autocompletion and graph
- "Save as draft" wizard
- Alerting rule history
- Acknowledge alert
- Filter by triggered date/time
- Column management
- Additional alert action items (View logs, Troubleshoot, etc.)
- Generate a summary report
- Generate a dashboard
- Manage impact groups
- Alertmanager sub-tab

### API Endpoints

Base path: `/api/v1/alerting`

> Note: GET endpoints should remain compatible with upstream Thanos/Prometheus (query parameters and response schemas) wherever applicable to enable native Perses integration.

### Hub Alerts API extension

- Goal: Extend the Single-Cluster Alerting API to support hub-scoped alerting rules and surface alert instances evaluated on the hub (ThanosRuler) alongside spoke alerts aggregated at the hub Alertmanager.

- Endpoints (mirror existing single-cluster endpoints under a hub scope):
  - Alerting rules (definitions)
    - `POST   /hub/rules`              — Create a hub alerting rule (MVP: adds rule entry to `thanos-ruler-custom-rules` ConfigMap; future: creates `HubAlertingRule` CRD)
    - `GET    /hub/rules`              — List hub alerting rules (MVP: parses both `thanos-ruler-default-rules` and `thanos-ruler-custom-rules` ConfigMaps; future: reads `HubAlertingRule` CRDs)
    - `GET    /hub/rules/{ruleId}`     — Get a hub rule by id
    - `PATCH  /hub/rules/{ruleId}`     — Update a hub rule (only custom/unmanaged; blocked for operator-managed and GitOps-managed)
    - `DELETE /hub/rules/{ruleId}`     — Delete a hub rule (only custom/unmanaged)
  - Alerts (instances)
    - `GET    /hub/alerts`             — List aggregated alert instances from spoke clusters and hub (Firing / Silenced) with classification and mapping labels. Response schema matches `GET /alerting/alerts`. Backed by hub Alertmanager (primary, near real-time), enriched with rule metadata from the Rule Metadata Cache. Spoke silence state is natively available on hub AM via the silence sync controller. See the Alerts Enrichment Pipeline section for the full flow.
  - Silences
    - `GET    /hub/silences`           — List silences from hub Alertmanager (includes hub-scoped silences and spoke silence replicas created by the sync controller). Optionally filter by `cluster` to return only silences scoped to a specific spoke.
    - `POST   /hub/silences`           — Create a silence. The request includes a `scope` field: `hub` targets hub Alertmanager directly (for hub-scoped alerts); `cluster:<name>` targets the specified spoke Alertmanager via ManagedClusterProxy (the sync controller will replicate it back to hub AM). Request body matches the Alertmanager silence schema (matchers, startsAt, endsAt, comment, createdBy).
    - `DELETE /hub/silences/{id}`      — Expire a silence. For hub-scoped silences, expires on hub Alertmanager. For spoke-scoped silences, expires on the spoke Alertmanager via ManagedClusterProxy (the sync controller removes the replica from hub AM).

### Batch Operations API

Batch endpoints enable applying rule changes or silences across multiple clusters in a single request.

- Endpoints:
  - `POST   /hub/rules/batch/preview` — Dry-run: validate a rule change against a set of target clusters without applying. Returns per-cluster validation results (success, conflict, RBAC denial, GitOps-blocked).
  - `POST   /hub/rules/batch/apply`   — Apply a rule change (create, update, disable, delete) to a set of target clusters. The request specifies the rule definition and a list of target clusters (by name or label selector). The API fans out to each target cluster's Alerting API via ManagedClusterProxy. Returns a batch response with per-cluster status.
  - `POST   /hub/silences/batch`      — Create the same silence on multiple spoke Alertmanagers. The request specifies silence matchers, duration, and a list of target clusters.

- Batch response schema:
  - `summary`: total, succeeded, failed, skipped counts.
  - `results[]`: per-cluster entries with `cluster`, `status` (success | failed | skipped | denied), `error` (if failed), `ruleId` (if created).
  - Partial success is expected — the API applies to all reachable clusters and reports failures individually.
  - Concurrency: fan-out uses bounded concurrency (configurable, default 10) with per-cluster timeouts. Long-running batches are processed synchronously within a single HTTP request for MVP; async job-based execution is a future consideration.

#### Implementation impact (MCO adoption)
- Current behavior in `multicluster-observability-operator`: managed clusters forward alerts to the hub Alertmanager (via `additionalAlertmanagerConfigs` injected by the MCOA endpoint operator). The metrics-collector also federates the `ALERTS` metric to hub Thanos but strips the `managed_cluster` label.
- Required changes to adopt this design:
  - Collect the `alerts_effective_active_at_timestamp_seconds` metric from the spoke clusters. This metric will obtain the post-ARC, post-silence effective alert state and produces the metric for spoke Prometheus to scrape. The metrics-collector federates it to hub Thanos alongside existing metrics. This metric is used for recording rules and aggregated health views (heatmap, component tables), NOT for the real-time alerts page, due to the 5 min collection interval.
  - The console backend queries hub Alertmanager for `GET /hub/alerts` (primary source, near real-time). Alerts carry `managed_cluster` and ARC-applied labels. The endpoint enriches alerts with rule metadata from the Rule Metadata Cache and applies RBAC filtering. Spoke silence state is natively available on hub AM via the silence sync controller (no read-time merge needed).
  - For hub rule management (MVP): the Alerting API reads and writes directly to the existing `thanos-ruler-custom-rules` and `thanos-ruler-default-rules` ConfigMaps. No MCO operator changes needed. ThanosRuler picks up changes via its existing ConfigMap watch. A future iteration introduces a `HubAlertingRule` CRD with a reconciler for per-rule ownership, disable, and optimistic concurrency (see Hub Rule Management and Open Questions).

- Semantics:
  - Request/response schemas are identical to the single-cluster Alerting API and include `ruleId`, `labels`, `annotations`, `mapping` (component/impactGroup), `source` (`platform`|`user`|`hub`), `managedBy` (`operator`|`gitops`|`""`), and `disabled` flags.
  - Filters mirror the single-cluster API (`name`, `group`, `component`, `severity`, `state`, `source`, `triggered_since`, arbitrary label filters) plus multi-cluster filters (`cluster`, `cluster_labels`).
  - For `GET /hub/alerts`, the `cluster` filter selects alerts by `managed_cluster` label. Omitting it returns alerts from all clusters plus hub alerts.
  - Read paths keep Prometheus/Thanos compatibility where applicable.

### Data Model

#### Label topology across the stack

Understanding which labels are available at each data sink is critical for designing recording rules and API enrichment.

| Label | Spoke Alertmanager | Hub Alertmanager | Hub Thanos (ALERTS metric) | Hub Thanos (`alerts_effective_*` metric) |
|-------|--------------------|-----------------|---------------------------|------------------------------------------|
| `managed_cluster` | YES (via external_labels) | YES (via additionalAlertmanagerConfigs) | NO (stripped by metrics-collector) | NO (stripped by metrics-collector) |
| `cluster` | NO | NO | YES (MCOA addon write relabel) | YES (MCOA addon write relabel) |
| `clusterID` | NO | NO | YES (MCOA addon write relabel) | YES (MCOA addon write relabel) |
| `openshift_io_alert_rule_id` | YES (for rules with ARC id stamp) | YES (for rules with ARC id stamp) | NO | YES (post-ARC) |
| `openshift_io_alert_rule_component` | YES (for classified rules) | YES (for classified rules) | NO | YES (post-ARC) |
| `openshift_io_alert_rule_layer` | YES (for classified rules) | YES (for classified rules) | NO | YES (post-ARC) |
| Disabled alerts | absent (ARC Drop) | absent (ARC Drop) | present (no ARC in metric pipeline) | absent (ARC Drop) |
| Silenced alerts | suppressed state | suppressed (hub silences + spoke silences via sync controller) | present (no silence awareness) | present with `alertstate=silenced` |

Key implications:
- **Hub Alertmanager** is the primary source for `GET /hub/alerts` (real-time alerts page) — it has `managed_cluster` for cluster identification, ARC-applied labels for classification, and receives alerts in near real-time. Spoke silences are replicated to hub AM by the silence sync controller with `managed_cluster` matcher scoping, so hub AM natively reflects spoke silence state.
- **`alerts_effective_active_at_timestamp_seconds` on hub Thanos** is the source for recording rules and aggregated health views — it has `cluster` for identification, ARC-applied labels for classification, silence state as `alertstate=silenced`, and excludes disabled alerts. Not used for the real-time alerts page (~5 min federation latency).
- **Hub Thanos ALERTS metric** is not used directly — it lacks ARC labels and silence/disable awareness. Superseded by the `alerts_effective_*` metric for aggregation use cases.
- **ARC-applied labels** (id stamp, component, layer) are only present on alerts whose rules have been modified via the Alerting API. Unmodified platform rules produce alerts without these labels, requiring alertname-based matching for enrichment.

### Metrics and Recording Rules

Recording rules must be defined on the data source where the required labels are available.

#### Spoke-side alert metric (federated to hub Thanos via metrics-collector)

- **`alerts_effective_active_at_timestamp_seconds`**: a new spoke-side metric representing the final effective alert state after ARC relabeling. The value equals the alert's `activeAt` timestamp. This metric is the data source for recording rules and aggregated health views on the hub (not for the real-time alerts page — hub Alertmanager is the primary source for that).
  - Generated on each spoke from the post-ARC alert notification pipeline (the source of truth for relabeled alert state)
  - Carries post-ARC labels: `alertname`, `severity` (after overrides), `namespace`, `openshift_io_alert_rule_id`, `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`, and other routing-relevant labels
  - Retains all original alert labels (no label dropping) — preserves full context for queries, drill-down, and matching. Cardinality note: alerts with high-cardinality labels (e.g., `pod`, `container`, `instance`) will produce one time series per unique label combination per firing alert. At fleet scale (many clusters, many alerts), this may generate significant series volume on hub Thanos. Monitor `alerts_effective_*` series count per cluster and consider targeted label dropping in the metrics-collector allowlist if cardinality becomes a concern.
  - Disabled alerts (ARC `action: Drop`) are absent — they never fire
  - Silenced alerts are present with `alertstate=silenced` — the metric includes both firing and silenced alerts, enabling the UI to filter by state
  - Federated to hub Thanos via the metrics-collector. On hub Thanos, the `cluster` label is added by MCOA addon write relabel configs.
  - **Open:** How is this metric generated? Options include: (a) a sidecar/exporter that reads from spoke Alertmanager API and produces the metric, (b) a recording rule combined with a mechanism that applies ARC relabeling to the ALERTS metric. Option (a) is more natural since only the Alertmanager knows the combined post-ARC + post-silence state.

#### Spoke-side recording rules (federated to hub Thanos via metrics-collector)

These recording rules run on spoke Prometheus and are federated to hub Thanos. The `cluster` label is added by MCOA addon write relabeling on the hub side. The recording rules are deployed to spokes as PrometheusRule CRDs by the MCOA endpoint operator (similar to how it deploys other spoke-side configuration). The endpoint operator ensures the rules are present on every managed cluster and updates them on operator upgrade.

- Aggregated health: `acm:cluster:health:critical_count` -- counts firing alerts with `severity=critical` and `impact=cluster`. Powers the Fleet Health Heatmap. With `alerts_effective_active_at_timestamp_seconds` available, this recording rule can be defined over that metric to get correct post-ARC severity and exclude silenced/disabled alerts.
- Component health: `acm:component:health:severity_count` -- counts firing alerts grouped by `component` and `severity`. Drives the "Most Impacted Components" table. With `alerts_effective_active_at_timestamp_seconds`, this recording rule can group by `openshift_io_alert_rule_component` since the metric carries post-ARC labels.

#### Hub-side recording rules (on ThanosRuler, querying hub Thanos)

These rules run on hub ThanosRuler over federated data. They have access to the `cluster` and `clusterID` labels, and when querying the `alerts_effective_*` metric, also have access to ARC-applied labels.

- Hub alerts health: `acm:hub:health:severity_count` -- counts hub-evaluated alerts grouped by `severity`. Default mapping labels are `impact=cluster` and `component=hub`.
- Fleet component health (new): `acm:fleet:component:health:severity_count` -- aggregates `alerts_effective_active_at_timestamp_seconds` across all clusters by `cluster`, `openshift_io_alert_rule_component`, and `severity`. Provides pre-aggregated component health for the "Most Impacted Components" table without per-request computation.

#### Recording rules design constraint (partially resolved)

The `alerts_effective_active_at_timestamp_seconds` metric resolves the main constraint: ARC-applied labels ARE available on this metric because it is generated from the post-ARC alert notification pipeline. Recording rules that use this metric CAN group by `openshift_io_alert_rule_component` and `openshift_io_alert_rule_layer`.

The original ALERTS metric (produced by Prometheus rule evaluation) still does NOT carry ARC-applied labels. Recording rules that use the ALERTS metric directly cannot group by component/layer.

### Migration
- Existing alerting rules should have an indication of missing recommended labels in the UI.

### GitOps / Argo CD Compliance
- All generated `PrometheusRule` and `AlertRelabelConfig` resources remain declarative and suitable for committing to Git

### Workflow Description

## Pain Points Addressed by this Design
- **Lack of prioritization in flat alert lists:** Without consistent impact scope (cluster/namespace/compute) and component metadata, alerts cannot be ranked by blast radius or ownership, forcing operators to scan long lists. Standardized `impact` and `component` labels enable fleet‑level grouping, priority cues, ownership‑based routing, and aggregated cluster/component health to surface what to address first.
- **Alert Noise and Data Overload:** Grouping, advanced filters, and saved filters will help reduce noise and the need for repetitive filtering.
- **Missed Alarms or Missing Data:** Users will be able to create flexible alert definitions directly in the UI to monitor any data type, configure notifications, and link a runbook.

## Pain Points Not Directly Addressed


## Risks & Mitigations
- **Fleet‑scale performance**: batch operations can fan out to many clusters. Apply concurrency limits and backpressure. Use per‑cluster timeouts and partial‑success reporting. Support resume or retry for long‑running batches. Paginate or stream large rule and alert lists.
- **Scope and precedence complexity**: hub vs cluster rule precedence must be deterministic. Provide dry‑run or preview, clear scope indicators, and UI guardrails. Document and enforce a single precedence policy in the API.
- **Connectivity or proxy reliability**: outages or high latency in the ManagedClusterProxy on the hub may disrupt writes and reads. Use retries with jitter, per‑cluster backoff, circuit breakers, and a degraded read mode backed by cached data.
- **Aggregation freshness at scale**: fleet caches can become stale. Define SLAs and TTLs. Provide on‑demand refresh for critical views, and progressively load details. Fall back to sampled or approximate aggregates for heatmaps when needed.
- **Label propagation and data minimization**: restrict `external_labels` to an allowlist of safe ManagedCluster labels. Validate size and cardinality, and perform periodic audits to avoid sensitive data leakage.
- **RBAC and ownership across clusters**: enforce hub and per‑cluster RBAC. Treat GitOps‑owned resources as read‑only. Return per‑cluster denial reasons in batch results.
- **Drift and consistency**: detect and surface drift between platform rules and relabel configs on spokes. Provide conflict reporting and optional reconciliation guidance.
- **Cache refresh fan-out at scale**: the Rule Metadata Cache refreshes by calling each spoke's Alerting API every 5 min, and the silence sync controller polls each spoke's Alertmanager every 30s. At 1000 clusters, this produces ~3.3 rule-metadata calls/sec and ~33 silence polls/sec sustained. Use bounded concurrency (e.g., 20 parallel requests), staggered refresh intervals (jitter), and prioritize clusters with recent changes. Monitor cache refresh latency and spoke API error rates.
- **`alerts_effective_*` metric cardinality**: retaining all original alert labels (no dropping) can produce high series volume at fleet scale. Monitor per-cluster series counts for this metric. If cardinality becomes a concern, consider targeted label dropping in the metrics-collector allowlist for high-cardinality labels (e.g., `pod`, `container`, `instance`).

## Graduation Criteria
- **Tech Preview**: Scope, UX, and release `Graduation Criteria` will be defined with the Observability UI team. This enhancement provides the backend APIs and behaviors they consume.
- **GA**: Finalize scope and UX with the Observability UI team. Deliver multi‑namespace filtering, full test coverage, and complete docs.

## Open Questions
 - **ManagedCluster label propagation:** The hub offers ManagedCluster labels (allowlist) that, during federation or at read time, add cluster labels to the alerts, which is needed for efficient notifications routing.
  Spokes do not currently support propagating ManagedCluster labels into alert labels. Should spoke-level propagation be added in a later phase (non-MVP)? If yes, define privacy/cardinality constraints and how precedence/deduplication would work if both hub and spokes can attach labels.

 - **Hub rule storage — future CRD evolution:** MVP uses direct ConfigMap manipulation for hub rules (no MCO operator changes). Two options for the future CRD-based iteration:
   - *Option A (recommended):* `HubAlertingRule` CRD is the single source of truth for ALL hub rules. The MCO operator creates CRDs for its default rules (with `managedBy: operator` and ownerReferences) instead of writing ConfigMaps directly. The reconciler generates both ConfigMaps from CRDs. `GET /hub/rules` reads CRDs only — one uniform source, one code path, per-rule ownership and disable semantics for all rules. Requires MCO operator change.
   - *Option B:* `HubAlertingRule` CRD is used only for custom (user/GitOps) rules. Default rules remain in `thanos-ruler-default-rules` ConfigMap, written directly by the MCO operator as today. `GET /hub/rules` reads from two sources: CRDs for custom rules + ConfigMap parsing for defaults. No MCO operator change, but two code paths and no per-rule disable for default rules.
   - Decision depends on MCO team willingness to adopt the CRD for operator-managed default rules. Should be resolved during design review with MCO team.

 - **Spoke silence visibility on hub — silence sync controller vs. Silence Cache:** Two options for making spoke-local silences visible in `GET /hub/alerts`:
   - *Option A (current design):* A silence sync controller (deployed on the hub or per-spoke) watches spoke Alertmanager silences and creates scoped replicas on hub Alertmanager with `managed_cluster` matcher scoping to prevent cross-cluster interference. Hub AM then natively suppresses spoke alerts — no read-time cache or matching needed. Trade-off: requires a new controller component, lifecycle sync (create/update/expire/delete must be mirrored), potential conflicts if hub users also create silences for spoke alerts directly on hub AM.
   - *Option B:* The console backend maintains a Silence Cache (per cluster, TTL 30s) by querying each spoke's Alertmanager (`GET /api/v2/silences`) via ManagedClusterProxy. At read time, silence matchers are matched against alert labels to determine silence state. Simpler to implement (code only, no new controller), degrades gracefully when a spoke is unreachable (stale cache with flag). Trade-off: fan-out queries to all spoke AMs every 30s; silence matching logic in the console backend.

 - **`alerts_effective_active_at_timestamp_seconds` collection interval:** The metrics-collector currently federates spoke metrics to hub Thanos at ~5 min intervals (single global interval, no per-metric configuration). This latency is acceptable for recording rules and aggregated health views (heatmap, component tables). If a faster collection interval (~30s) becomes feasible for this metric, it could improve heatmap and component health freshness. The metrics-collector does not currently support per-metric intervals; possible approaches include (a) a separate lightweight collector instance per spoke for this metric, (b) per-metric interval support in the metrics-collector (code change). Needs investigation with the MCOA team.

## Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Existing URLs must remain operational across upgrades.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

## Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.