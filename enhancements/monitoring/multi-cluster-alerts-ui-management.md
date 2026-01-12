---
title: multi-cluster-alerts-ui-managment
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
creation-date: 2026-01-12
last-updated: 2026-03-18
tracking-link:
  - "https://issues.redhat.com/browse/CNV-46597"
  - "https://issues.redhat.com/browse/CNV-62972"
---
# Managing Alerts in the Multi-Cluster Console

## Table of Contents

- [Summary](#summary)
- [Child Proposals](#child-proposals)
- [Motivation](#motivation)
  - [Problem Statement](#problem-statement)
  - [User Stories](#user-stories)
- [Goals](#goals)
- [Proposed Features](#proposed-features)
- [Non-Goals](#non-goals)
- [Related Enhancement Proposals](#related-enhancement-proposals)
- [Current State](#current-state)
  - [Existing alert forwarding infrastructure](#existing-alert-forwarding-infrastructure)
  - [Alert data storage and lifecycle](#alert-data-storage-and-lifecycle)
  - [Existing ACM alerting developer preview](#existing-acm-alerting-developer-preview)
  - [Gaps in the current state](#gaps-in-the-current-state)
- [Proposal](#proposal)
  - [Architecture](#architecture)
  - [Alerts Enrichment Pipeline (summary)](#alerts-enrichment-pipeline-summary)
  - [Silence Sync Controller (summary)](#silence-sync-controller-summary)
  - [Hub Rule Management (summary)](#hub-rule-management-summary)
  - [Batch Operations (summary)](#batch-operations-summary)
  - [Hub Alertmanager as Centralized Notification Hub (summary)](#hub-alertmanager-as-centralized-notification-hub-summary)
- [Fleet Health Heatmap & Filtering](#fleet-health-heatmap--filtering)
  - [Fleet landing page](#fleet-landing-page)
  - [Backend data for the Heatmap](#backend-data-for-the-heatmap)
  - [Proposed UI in Multi-Cluster Console](#proposed-ui-in-multi-cluster-console)
  - [Additional Points to Consider](#additional-points-to-consider)
  - [Feature Prioritization](#feature-prioritization)
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

## Child Proposals

This umbrella proposal is split into focused child documents for detailed design:

| Part | Document | Scope |
|------|----------|-------|
| 1 | [Alert Visualization (Read Path)](multi-cluster-alerts-visualization.md) | `GET /hub/alerts` enrichment pipeline, data sources, caches, label topology, data model, metrics, recording rules, historical alert views |
| 2 | [Hub Rule Management](multi-cluster-hub-rule-management.md) | Hub alerting rule CRUD, ConfigMap-based MVP, future `HubAlertingRule` CRD, classification labels, ownership model |
| 3 | [Batch Operations](multi-cluster-batch-operations.md) | `POST /hub/rules/batch/apply`, `POST /hub/silences/batch`, preview/confirmation flow, batch response schema |
| 4 | [Silence Management](multi-cluster-silence-management.md) | Silence sync controller, hub/spoke silence API, hub AM as centralized notification hub |

The sections below provide summaries with cross-references to the child documents. The Current State, Motivation, Goals, Fleet Health Heatmap, UI, and operational sections remain in this parent document.

## Motivation
- While it's possible to customize built‑in alerting rules on individual clusters (see [alert overrides](https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/alert-overrides.md)), doing so consistently across many clusters is cumbersome and error‑prone. It requires templating and applying per‑cluster YAML, coordinating rollouts, and there is no fleet‑level validation or preview. Built‑in rules and alerts also remain visible per‑cluster after overrides, leading to inconsistent UX across the fleet.
- Operational teams need a fleet‑aware console and API to define, target, and audit alerting rules and silences across scopes such as fleet‑wide (hub), groups of clusters, or individual clusters, so they do not have to repeat the same action on each cluster.
- A unified multi‑cluster interface should enable creating, cloning, and disabling rules or setting silences across selected clusters, viewing aggregated and per‑cluster firing status, resolving precedence between global and local overrides, and preserving intended behavior through cluster lifecycle events and upgrades.

### Problem Statement
Fleet administrators struggle with generic per‑cluster alerting rules that create cross‑cluster noise, lack fleet‑level context, and are difficult to standardize and target by cluster labels or sets, making consistent thresholds, severity, and routing across many clusters error‑prone.

### User Stories

1. **Fleet overview and drill‑down**
   - As a fleet Admin, I want to see the health status of my clusters, specifically for my “Production” clusters to quickly identify where critical alerts are firing, and have a way to quickly drill down to see the impacted components and only then to the specif relevant alerts.

2. **Create and manage Global (hub) alert**
   - As an SRE, I want to define and manage a Global alert that evaluates on the hub (MCOA Thanos Ruler) over federated data and routes to the appropriate global receiver.

3. **Batch‑apply a rule to selected clusters**
   - As an Ops Lead, I want to apply or update a specific alert rule and deploy it across a list of specific clusters (that I can easily search for by their names, labels, versions, etc.) in one action, without visiting each cluster UI.

4. **View global vs cluster‑local alerts**
   - As an SRE, I want to distinguish alerts running on the hub (global scope) from those running on a specific cluster and navigate between them seamlessly.

5. **Manage all types of alerting rule across selected spoke clusters**
   - As a fleet Admin, I want to create, update, disable, and delete alerting rules — both platform and user-defined — across a selected set of clusters in one workflow, so I can maintain consistent rule sets across my fleet without repeating the same action on each cluster.

6. **Bulk update labels across clusters**
   - As a Platform Admin, I want to update labels — such as severity, component, layer, or custom routing labels — for a set of alerting rules across selected clusters in one action, so alert routing, escalation, and classification are consistent without repeating the change on each cluster.

7. **Namespace‑scoped fleet view**
   - As a Namespace Owner, I want to filter the fleet alerts and rules views to only show what affects my namespace(s) across clusters, so I can focus on my workloads without being overwhelmed by cluster-wide noise.

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
- [Alert Overrides](https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/alert-overrides.md)
- [User Workload Monitoring](https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/user-workload-monitoring.md)
- [Alerts UI Management (Single-Cluster)](alerts-ui-management.md)
- [Alert Rule Classification Mapping](alert-rule-classification-mapping.md)

## Current State

This section describes the existing multi-cluster alerting infrastructure that this proposal builds upon.

### Existing alert forwarding infrastructure

Managed clusters already forward alerts to the hub Alertmanager via the Multi-Cluster Observability Addon (MCOA). The flow is:

1. **Hub side**: The MCO placement rule controller generates a `hub-info-secret` containing the hub Alertmanager URL (discovered from the `alertmanager` Route in `open-cluster-management-observability`) and router CA. This secret is distributed to each spoke cluster via ManifestWork.

2. **Spoke side**: The MCOA endpoint operator (`observabilityaddon_controller`) reads `hub-info-secret` and injects `additionalAlertmanagerConfigs` into the spoke's `cluster-monitoring-config` ConfigMap (and `user-workload-monitoring-config` for user workload monitoring). This configuration tells spoke Prometheus to forward all firing alerts to the hub Alertmanager. The endpoint operator also adds a `managed_cluster` external label (set to the spoke cluster name) so hub AM can identify the originating cluster.

3. **Alert delivery**: Spoke Prometheus sends alerts to hub AM on every evaluation cycle (~30s-1min). Since `alert_relabel_configs` from ARCs are applied globally in Prometheus before alerts are dispatched to any Alertmanager, spoke alerts arrive at hub AM with post-ARC labels (including `openshift_io_alert_rule_id`, component, layer, and severity overrides for rules that have been classified).

4. **Hub ThanosRuler**: ThanosRuler evaluates rules over federated metrics in hub Thanos and sends its alerts to the same hub Alertmanager (via internal service DNS: `observability-alertmanager-{i}.alertmanager-operated.open-cluster-management-observability.svc:9095`).

5. **Hub AM Route**: Hub Alertmanager is exposed via an OpenShift Route (`alertmanager`) at `/api/v2`, fronted by an OAuth proxy on port 9095.

### Alert data storage and lifecycle

Alert data lives in two fundamentally different stores on the hub, each serving a different purpose:

**Hub Alertmanager (transient, in-memory)**

Hub AM holds active alert instances in memory, backed by a local PVC for silences and notification state. It is not a persistent store — resolved alerts are dropped after a configurable grace period (`resolve_timeout: 5m`). Hub AM can answer "what is firing right now?" but cannot answer "what was firing yesterday?" Once an alert resolves and the grace period passes, it is gone from hub AM.

The hub AM configuration defaults to a `null` receiver (no notifications sent). However, the AM config Secret uses a `skip-creation-if-exist: "true"` annotation, so users can customize receivers after initial deployment without the operator overwriting their changes. This enables hub AM to serve as a centralized notification hub for spoke alerts (see [Hub Alertmanager as Centralized Notification Hub](#hub-alertmanager-as-centralized-notification-hub)).

**Hub Thanos (persistent, S3 object storage)**

Metrics collected from spokes (including the `ALERTS` metric, which is in the default allowlist) are federated to hub Thanos via the metrics-collector and stored in S3-compatible object storage. This data is persistent and subject to Thanos retention and compaction policies. It supports historical queries via PromQL.

However, the `ALERTS` metric in hub Thanos has significant limitations compared to hub AM:
- **No ARC-applied labels**: The `ALERTS` metric is produced by Prometheus rule evaluation, before ARCs are applied. It lacks `openshift_io_alert_rule_id`, `openshift_io_alert_rule_component`, and `openshift_io_alert_rule_layer`.
- **No silence awareness**: Silenced alerts still appear as `alertstate="firing"` in the `ALERTS` metric — Prometheus does not know about Alertmanager silences.
- **`managed_cluster` is stripped**: The metrics-collector strips the `managed_cluster` label during federation. Only the `cluster` label (added by MCOA addon write relabel configs) is available on hub Thanos.
- **No disabled alert awareness**: ARC-dropped alerts never fire, so they are absent from `ALERTS`, but there is no way to distinguish "never fired" from "disabled by ARC."

These limitations are why this proposal introduces the `alerts_effective_active_at_timestamp_seconds` metric — it carries post-ARC labels, silence state (`alertstate=silenced`), and excludes disabled alerts, providing the enriched historical view that the raw `ALERTS` metric cannot.

### Existing ACM alerting developer preview

A [developer preview of the Multi-Cluster Alerting UI](https://developers.redhat.com/articles/2025/03/27/new-multi-cluster-alerting-ui-developer-preview) is available in the ACM console, built on the monitoring-plugin with the `acm-alerting` feature flag. It provides:

- **Proxy servers**: When `acm-alerting` is enabled, the monitoring-plugin starts two additional proxy servers alongside its main server:
  - **Alertmanager proxy** on port 9444 — proxies to the hub AM URL (configured via `--alertmanager` flag or `UIPlugin` CR)
  - **Thanos Querier proxy** on port 9445 — proxies to the `rbac-query-proxy` (configured via `--thanos-querier` flag or `UIPlugin` CR)
- **Frontend paths**: The console frontend accesses these proxies via:
  - `ALERTMANAGER_PROXY_PATH` (`/api/proxy/plugin/monitoring-console-plugin/alertmanager-proxy`) — for alerts and silences (`/api/v2/alerts`, `/api/v2/silences`)
  - `PROMETHEUS_PROXY_PATH` (`/api/proxy/plugin/monitoring-console-plugin/thanos-proxy`) — for PromQL queries and rules
- **UIPlugin CR**: The Cluster Observability Operator (COO) provides a `UIPlugin` custom resource to configure the hub AM and Thanos Querier URLs.
- **Current scope**: The developer preview focuses on displaying ThanosRuler-evaluated alerts (hub rules). Forwarded spoke alerts are present in hub AM but lack enrichment context.

### Gaps in the current state

The existing infrastructure forwards spoke alerts to hub AM and provides basic proxy access, but the following gaps prevent a production-ready multi-cluster alerting experience:

1. **No alert enrichment**: The proxy returns raw hub AM data. Alerts lack classification context (component, layer) for spokes that haven't been classified via the Alerting API, and there is no rule metadata (alertRuleId, source, managedBy, disabled state).
2. **No spoke silence visibility on hub**: Spoke-local silences are not reflected on hub AM. An alert silenced on a spoke still appears as firing on the hub alerts page.
3. **No hub rule management**: No API to create, update, or delete ThanosRuler rules from the UI.
4. **No batch operations**: No fleet-wide rule apply, silence, or label update.
5. **No historical alert views with enrichment**: The `ALERTS` metric in Thanos lacks ARC labels and silence state, making enriched alert history impossible without the new `alerts_effective_*` metric.
6. **No centralized notification management UI**: Users can manually configure hub AM receivers but there is no UI support for managing notification routing for spoke alerts.

## Proposal

### Architecture

Key flows:
- UI authenticates and calls the console backend, which invokes the Unified Alerting API with the user’s identity and RBAC context.
- Hub (Global) scope: hub alerting rules and silences are read and written via the Unified Alerting API. For MVP, the API reads and writes hub rules directly in the existing ConfigMaps (`thanos-ruler-default-rules` for operator-managed defaults, `thanos-ruler-custom-rules` for user-created custom rules) that ThanosRuler already watches. A future iteration introduces a `HubAlertingRule` CRD as the single source of truth with a reconciler generating ConfigMaps (see Hub Rule Management).
- Cluster‑scoped operations: alerting rule and silence definitions are stored on the spoke clusters and are read/written via the Unified Alerting API through the ManagedClusterProxy on the hub to each target cluster’s APIs.
- Alerts read path: `GET /hub/alerts` uses the hub Alertmanager as the primary source for alert instances (near real-time, ~30s-1min latency). Spoke Prometheus forwards alerts to the hub Alertmanager via `additionalAlertmanagerConfigs` (injected by the MCOA endpoint operator). Each spoke alert carries a `managed_cluster` label and ARC-applied labels when the rule has been classified. Hub alerts from ThanosRuler also arrive at the hub Alertmanager. The endpoint enriches alerts with rule metadata from a Rule Metadata Cache. Spoke silence state is natively available on hub Alertmanager via a new silence sync controller that replicates spoke silences (see Alerts Enrichment Pipeline; approach not yet finalized — see Open Questions).
- Firing alerts ingestion: managed clusters forward alerts to the hub Alertmanager (near real-time, primary source for the alerts page) AND produce the `alerts_effective_active_at_timestamp_seconds` metric on spoke Prometheus, which is federated to hub Thanos via the metrics-collector (~5 min interval). The `alerts_effective_*` metric carries post-ARC labels and `alertstate=silenced` for silenced alerts, and is used for recording rules and aggregated health views (heatmap, component tables) where ~5 min latency is acceptable.
- Batch endpoints: apply changes to a selected set of clusters. The UI shows a confirmation step using cached data (Rule Metadata Cache, Cluster Registry Cache) before applying, surfacing target clusters, GitOps-blocked or RBAC-denied clusters, and create-vs-update intent. Responses include per‑cluster status, errors, and summaries.
- Rule identity and grouping: read paths aggregate rule definitions by alert name plus full label set, and aggregate alert instances with post‑relabel context for a consistent fleet view.
- GitOps ownership: when resources are owned by GitOps, the API treats them as read‑only and surfaces guidance rather than mutating in‑cluster resources.
- Conflict/drift handling: server‑side validation with optimistic concurrency (resourceVersion) and idempotent apply. The API reports conflicts and drift in per‑cluster results.
- Silences: support hub‑scoped and cluster‑scoped silences. Label‑based selectors are forwarded to the appropriate Alertmanager(s).
- Silences scope policy: Hub-initiated silences for alerts originating on spokes are created on the respective spoke Alertmanager(s) via ManagedClusterProxy. Hub-scoped alerts are silenced on the hub Alertmanager only. The current design uses a silence sync controller that replicates spoke Alertmanager silences to hub Alertmanager with `managed_cluster` matcher scoping, so hub AM natively suppresses spoke alerts (approach not yet finalized — see Open Questions).

Relationship to the existing ACM alerting proxy:
- The existing developer preview uses direct proxy pass-through to hub AM (port 9444) and Thanos Querier (port 9445). The frontend queries hub AM's `/api/v2/alerts` and `/api/v2/silences` directly via the proxy, receiving raw, unenriched data.
- This proposal introduces server-side enrichment endpoints (`GET /hub/alerts`, `GET /hub/rules`, `GET /hub/silences`) that replace the direct proxy for the alerts and rules pages. These endpoints query hub AM internally, enrich the response with rule metadata and classification, and return a unified view.
- The existing AM and Thanos proxies remain available for operations that do not need enrichment (e.g., direct PromQL queries, raw silence operations during migration). The frontend transitions from proxy paths to the new API endpoints as features are delivered.
- The new endpoints are registered on the same monitoring-plugin server (extending the existing `/api/v1/alerting` router) and reuse the existing `management.Client` interface, error handling, and query parameter parsing.

Rationale for server‑side aggregation and routing:
- Provides a canonical, post-relabel effective view and mediates validated updates to `PrometheusRule`, `AlertingRule` and `AlertRelabelConfig` across many clusters, and `HubAlertingRule` CRDs on the hub.
- Enforces consistent RBAC, ownership checks (GitOps), and conflict handling while reducing client fan‑out.
- Reuses Single‑Cluster Alerting API semantics to minimize duplication and ease maintenance; extends with batch apply for fleet operations (with a UI confirmation step using cached data before applying).

### Alerts Enrichment Pipeline (summary)

> Full detail: [Alert Visualization (Read Path)](multi-cluster-alerts-visualization.md)

The `GET /hub/alerts` endpoint assembles a complete, post-relabel view of all alerts across the fleet. Two data sources feed alerts to the hub:

1. **Hub Alertmanager** (near real-time, ~30s-1min) — primary source for the real-time alerts page. Spoke Prometheus forwards alerts with `managed_cluster` label and ARC-applied labels. Spoke silences are replicated by the silence sync controller.
2. **`alerts_effective_active_at_timestamp_seconds` on hub Thanos** (~5 min latency) — source for recording rules, aggregated health views, and historical queries. Carries post-ARC labels and `alertstate=silenced`.

The console backend maintains two caches (Rule Metadata Cache, Cluster Registry Cache) to avoid fan-out on every request. Enrichment steps: fetch from hub AM → classify by scope → enrich with rule metadata → filter/sort/paginate.

**Prerequisites**: monitoring-plugin on spokes (OCP 4.18+), ManagedClusterProxy, MCOA endpoint operator.

**Historical views**: Hub AM is transient — historical alert queries must use metrics from hub Thanos (S3). See the child document for the full data source comparison table.

### Silence Sync Controller (summary)

> Full detail: [Silence Management](multi-cluster-silence-management.md)

A silence sync controller replicates spoke Alertmanager silences to hub Alertmanager so that hub AM natively suppresses spoke-silenced alerts. The controller runs on the hub, polls each spoke AM every 30s via ManagedClusterProxy, and creates scoped replicas on hub AM with a `managed_cluster` matcher to prevent cross-cluster interference. Replicas are tagged with `sync.source` to avoid conflicts with user-created hub silences. **Note:** this is one of two proposed approaches — see Open Questions for the alternative (Silence Cache).

### Hub Rule Management (summary)

> Full detail: [Hub Rule Management](multi-cluster-hub-rule-management.md)

Hub alerting rules are evaluated by MCOA ThanosRuler over federated data from hub Thanos. ThanosRuler uses ConfigMap-based rule files (`thanos-ruler-default-rules` for operator defaults, `thanos-ruler-custom-rules` for user rules), not PrometheusRule CRDs.

**MVP**: The API reads and writes directly to existing ConfigMaps. No new CRDs or MCO operator changes required. Ownership is detected at ConfigMap level (operator vs unmanaged vs GitOps).

**Future**: A `HubAlertingRule` CRD provides per-rule ownership, GitOps annotations, and optimistic concurrency.

**Classification labels**: For operator default rules, a static mapping is used (MVP); for user-created rules, `component` and `layer` are written as labels in the rule definition. Future: `alertRelabelConfigs` support on ThanosRuler.

### Hub Alertmanager as Centralized Notification Hub (summary)

> Full detail: [Silence Management](multi-cluster-silence-management.md#hub-alertmanager-as-centralized-notification-hub)

Hub AM receives alerts from all spokes and ThanosRuler. It defaults to a `null` receiver but users can customize receivers (the config Secret uses `skip-creation-if-exist: "true"`). This enables hub AM as a centralized notification hub — configure receivers once on the hub instead of on each spoke. The `managed_cluster` label enables cluster-based routing. The silence sync controller is essential for notification consistency: spoke silences must be replicated to hub AM so both local and centralized notifications are suppressed.

### Batch Operations (summary)

> Full detail: [Batch Operations](multi-cluster-batch-operations.md)

Batch endpoints enable applying rule changes or silences across multiple clusters in a single request:

- `POST /hub/rules/batch/apply` — fans out a rule change to target clusters via ManagedClusterProxy
- `POST /hub/silences/batch` — creates the same silence on multiple spoke Alertmanagers

The UI shows a confirmation step using cached data (Rule Metadata Cache, Cluster Registry Cache) before applying, surfacing target clusters, GitOps-blocked or RBAC-denied clusters, and create-vs-update intent. Responses include per-cluster status with partial success support.

### API Endpoints

> Detailed endpoint specifications are in the child proposals:
> - Alerts and rules: [Alert Visualization](multi-cluster-alerts-visualization.md#api-endpoints) and [Hub Rule Management](multi-cluster-hub-rule-management.md#api-endpoints)
> - Silences: [Silence Management](multi-cluster-silence-management.md#api-endpoints)
> - Batch operations: [Batch Operations](multi-cluster-batch-operations.md#api-endpoints)

Base path: `/api/v1/alerting`

All endpoints extend the existing Single-Cluster Alerting API in the monitoring-plugin. Request/response schemas are identical to the single-cluster API and include `ruleId`, `labels`, `annotations`, `mapping` (component/impactGroup), `source`, `managedBy`, and `disabled` flags. Filters mirror the single-cluster API plus multi-cluster filters (`cluster`, `cluster_labels`). GET endpoints remain compatible with upstream Thanos/Prometheus where applicable.

Summary of hub-scoped endpoints:

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/hub/alerts` | GET | Aggregated alert instances (enriched) |
| `/hub/rules` | GET/POST | List / create hub alerting rules |
| `/hub/rules/{ruleId}` | GET/PATCH/DELETE | Get / update / delete a hub rule |
| `/hub/silences` | GET/POST | List / create silences (hub or spoke scope) |
| `/hub/silences/{id}` | DELETE | Expire a silence |
| `/hub/rules/batch/apply` | POST | Batch rule change across clusters |
| `/hub/silences/batch` | POST | Batch silence across clusters |

#### Implementation impact (MCO adoption)

- The metrics-collector must federate the new `alerts_effective_active_at_timestamp_seconds` metric from spokes to hub Thanos (for recording rules and aggregated health views).
- The console backend queries hub Alertmanager for `GET /hub/alerts` (primary, near real-time) and enriches with rule metadata.
- Hub rule management (MVP) reads/writes existing ConfigMaps directly — no MCO operator changes needed.

### Data Model and Metrics

> Full detail: [Alert Visualization — Data Model](multi-cluster-alerts-visualization.md#data-model) and [Alert Visualization — Metrics and Recording Rules](multi-cluster-alerts-visualization.md#metrics-and-recording-rules)

Key points:

- **Label topology**: Hub AM has `managed_cluster` and ARC-applied labels. Hub Thanos has `cluster`/`clusterID` and (for `alerts_effective_*`) ARC-applied labels plus `alertstate`.
- **`alerts_effective_active_at_timestamp_seconds`**: New spoke-side metric carrying post-ARC labels and silence state. Federated to hub Thanos for recording rules and aggregated health views.
- **Recording rules**: Spoke-side (`acm:cluster:health:critical_count`, `acm:component:health:severity_count`) and hub-side (`acm:hub:health:severity_count`, `acm:fleet:component:health:severity_count`) recording rules drive the heatmap and component tables.

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
- Hub AM can serve as a centralized notification hub for spoke alerts. Users can configure receivers (Slack, PagerDuty, email) on hub AM and route notifications by `managed_cluster` label — enabling fleet-wide notification management from a single configuration point instead of configuring receivers on each spoke individually.
- The hub AM config Secret uses `skip-creation-if-exist: "true"`, so user customizations are preserved across operator reconciliation.
- Future UI improvements could include managing hub AM receivers and routes from the console, multi‑cluster routing by cluster labels (region, team), notifications by impact group and component, and team‑scoped subscriptions honoring RBAC.
- The silence sync controller is essential for notification consistency: spoke silences must be replicated to hub AM so that both spoke-local and hub-centralized notifications are suppressed for silenced alerts.

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

**Should-Have Features (continued):**
- Hub AM notification receiver configuration — users can already configure receivers manually; a UI for managing hub AM receivers and routing rules for spoke alerts would reduce the barrier to centralized notification management.

**Could-Have Features:**
- Advanced notification routing UI (route by cluster labels, component, impact group)
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
- **Scope and precedence complexity**: hub vs cluster rule precedence must be deterministic. Provide clear scope indicators, a confirmation step (using cached data) before batch apply, and UI guardrails. Document and enforce a single precedence policy in the API.
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

 - **Spoke-silenced alerts on the hub:** When a spoke alert is silenced locally, should it appear as silenced on the hub alerts view, or should silenced spoke alerts not be forwarded to the hub at all? Options: (a) silence sync controller replicates spoke silences to hub AM so they appear as suppressed on the hub (current design), (b) spoke Prometheus stops sending silenced alerts to hub AM entirely, (c) hub shows all spoke alerts regardless and silence state is spoke-local only. This affects the UX for users who expect spoke silence state to be visible on the hub without separate hub-side action.

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