---
title: multi-cluster-alerts-ui-design
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
last-updated: 2026-04-12
tracking-link:
  - "https://issues.redhat.com/browse/CNV-46597"
  - "https://issues.redhat.com/browse/CNV-62972"
see-also:
  - "/enhancements/monitoring/multi-cluster-alerts-ui-management.md"
  - "/enhancements/monitoring/multi-cluster-alert-effective-metric.md"
  - "/enhancements/monitoring/alerts-ui-management.md"
---
# Multi-Cluster Alerts UI Design

This proposal covers the UI design for the multi-cluster alerting experience: the Fleet Health Heatmap, alert and rule views, wireframes, and feature prioritization. It is Part 5 of the [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) umbrella proposal.

For the backend data model and API details, see the child proposals linked from the [parent document](multi-cluster-alerts-ui-management.md#child-proposals).

## Summary

This proposal covers the UI design for the multi-cluster alerting experience.

## Motivation

### User Stories

See [parent proposal User Stories](multi-cluster-alerts-ui-management.md#user-stories).

### Goals

The primary goal is to provide a comprehensive alerting management UI that directly addresses the problems identified through user feedback, research, and competitive analysis.
The proposed features are intended to reduce alerts noise and improve the overall user experience for monitoring and responding to issues, including surfacing prioritized next actions based on aggregated cluster and component health, so users can address the most impactful issues first.

1. Provide a Fleet Clusters health visualization showing all managed clusters (healthy and unhealthy) to inspect status at a glance, with filtering and grouping by labels (such as name, health, region, provider) and optional weighted priority (such as node count, pods count, VMs count, CPU count, alerts count).
2. Aggregate and display alert rules and alert instances across the fleet with post-relabel context, like in the single cluster.
3. Improve correctness and performance by reusing the **same Single-Cluster Alerting API** on the hub — the monitoring-plugin on the hub points to hub Alertmanager and enriches with spoke rule metadata, requiring no separate hub-specific endpoints.
4. Manage Global alerting rules on the hub (MCOA Thanos Ruler) and local rules on selected clusters from a unified UI.
5. Enrich alerts and metrics with ManagedCluster labels (e.g., `region`, `env`, `availability_zone`, `provider`) for filtering, grouping, notification routing, and dashboards. Three complementary mechanisms, all MVP:
   - **API-time enrichment:** the monitoring-plugin on the hub looks up the alert's `managed_cluster`, fetches the ManagedCluster resource from the Cluster Registry Cache, and attaches selected labels to the API response. This enables UI filtering and grouping by cluster metadata.
   - **Hub AM relabeling for routing:** a relabel config controller watches ManagedCluster resources and generates hub AM `alert_relabel_configs` that inject cluster labels onto alerts by matching `managed_cluster`. This makes the labels available to AM routing rules, enabling cluster-aware notification routing (e.g., route `region=us-east-1` alerts to a specific receiver). Required because AM routing can only match on labels present on the alert itself.
   - **`acm_managed_cluster_labels` for dashboards:** the existing `acm_managed_cluster_labels` metric (produced by MCE's `clusterlifecycle-state-metrics`, already federated to hub Thanos) exposes ManagedCluster labels including `cloud`, `vendor`, `openshiftVersion`, and user-defined labels. Dashboards join it via `group_left` at PromQL query time — e.g., `label_replace(acm_managed_cluster_labels, "cluster", "$1", "name", "(.*)")`.
   - All three mechanisms share a single allowlist controlling which ManagedCluster labels are exposed.

### Non-Goals

Backend API design and platform topology-specific deployment details are out of scope for this document; see the linked parent and sibling enhancements.

## Proposal

## Tab Overview

The user interface introduces a new **Observe > Alerting** page with grouping and component-based functionality.

#### Alerts Tab
The multi-cluster Alerts page mirrors the single-cluster Alerts page for familiarity and consistency. The key difference is the addition of a **Cluster** column (and scope) so users can see and filter alerts per cluster alongside the existing fields.

#### Management Tab
The Management tab mirrors the single-cluster design and capabilities for familiarity. The multi-cluster differences are:

- The list aggregates alerting rules from all managed clusters and groups them by alert rule definition (alert name plus its full label set) to provide a unified view.
- Users can create, update, delete, enable, and disable alerting rules (subject to rule type and RBAC) and apply those changes to a selected set of clusters via the Alerting API.
- Managing hub (global) alerts is supported in the same workflow.

All other interaction patterns remain consistent with the single-cluster experience.

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
  - Saved searches: persist user filter sets, such as "My Prod Clusters".
- Hub tile (The hub cluster tile in the heatmap/table):
  - Treat the Hub (MCOA) as a first‑class tile, such as "Global Platform".
  - Click to drill into global alerts, consistent with per-cluster interaction.

### Backend data for the Heatmap
- The heatmap is driven by **real-time alerts from Hub Alertmanager**, aggregated in the monitoring-plugin cache. For each managed cluster (from the Cluster Registry Cache), the cache computes health status by checking whether any cluster-health-impacting alerts (e.g., `severity=critical`, `impact=cluster`) are currently firing.

## Proposed UI in Multi‑Cluster Console

See additional details in the [UX Design- Alerts management](https://docs.google.com/document/d/1bB7kg-W2lLq85Dmy530STMUWJFlNPFvg08Sayc-RwK8/edit?usp=sharing)

- **Management List**: show all alerting rules. Filter and sort by cluster, name, severity, namespace, status, and labels. Saved searches.
![Alerting -> Management](assets/multi-cluster-alerts-management-ui.png)

- **Clusters Health View**:
  - Fleet landing page with a Heatmap to visualize multi‑cluster health at a glance.
  - Group clusters by common labels such as region, cloud provider, severity, or other labels to understand domain health.
  - Size tiles by different dimensions to reflect scale or impact, including number of nodes, number of pods, number of VMs, number of alerts, Total CPU Cores, Total Memory.
  - Includes two summary tables below the Heatmap:
    - "Top Firing Alerts" – aggregates the most active alerts across the fleet with counts and affected clusters.
    - "Most Impacted Components" – aggregates alert impact by component and shows health breakdown per component across clusters.
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

### Workflow Description

Described in the sections above (tabs, heatmap, alerts and management views, and forms). Detailed end-to-end workflows align with the single-cluster Observe > Alerting patterns extended for fleet scope.

### API Extensions

See [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) and related enhancements for Alerting API and hub integration; this design document consumes those contracts.

### Topology Considerations

#### Hypershift / Hosted Control Planes

UI assumes managed clusters are addressable via the multi-cluster console and Alerting API; any Hypershift-specific constraints follow hub/spoke connectivity and RBAC from the parent proposal.

#### Standalone Clusters

Same UI patterns apply where the cluster is a spoke in the fleet; standalone-only deployments are not the primary target of this enhancement.

#### Single-node Deployments or MicroShift

Visualization and filtering remain valid at smaller scale; resource-weighted views may be less differentiated.

#### OpenShift Kubernetes Engine

Feature availability follows platform support for MCOA and the Alerting API as documented in the parent proposal.

### Implementation Details/Notes/Constraints

Implementation is owned by the Observability UI and multi-cluster console teams in coordination with MCOA; wireframes and assets in this document are the primary UI constraints.

### Risks and Mitigations

- **UX complexity at fleet scale**: Mitigate via progressive disclosure, saved filters, and table mode alongside the heatmap.
- **API or metric drift**: Mitigate by reusing single-cluster API patterns and documenting dependencies in the parent enhancement.

### Drawbacks

Additional UI surface area and ongoing maintenance for parity with single-cluster alerting as APIs evolve.

## Feature Prioritization

The features are prioritized using tags: **Must-Have**, **Should-Have**, **Could-Have**, and **Won't-Have**.

**Must-Have Features:**
- Tabs changes (Clusters Health, Alerts, Management: Alerting rules, Silence rules)
- Create user-defined alerting rules
- Create Platform alerting rules
- Create hub alerts
- Hub AM silence CRUD — create, list, and expire silences on hub Alertmanager for both hub alerts and spoke-forwarded alerts. Already implemented in the monitoring-plugin via the Alertmanager proxy.
- Advanced filtering capabilities
- Bulk actions: disable, edit labels, edit component
- Duplicate and Delete alerting rules
- Add components and layer mapping and management
- ManagedCluster label enrichment at three levels: API responses (UI filtering/grouping), alerts in Alertmanager (notification routing via relabel config controller), and the existing `acm_managed_cluster_labels` metric (dashboards via `group_left` joins — always reflects current labels, even on historical queries). A shared allowlist controls which labels are exposed for API and AM enrichment.

**Should-Have Features:**
- Saved filters
- Alert and alerting rule side drawer
- Add "Resource" column for node alerts

**Should-Have Features (continued):**
- `SilenceRule` CRD + controller — declarative silence management with spoke AM propagation, spoke silence visibility on hub, GitOps support, and drift repair. Addresses MVP limitations where spoke-silenced alerts appear as firing on the hub and hub silences don't propagate to spokes.
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

## Open Questions

- **Hub alerts in the Fleet Health Heatmap and cluster health views:** Hub alerts (evaluated by ThanosRuler over federated data) fall into two categories with different UI implications:
  - *Per-cluster hub alerts* — alerts whose PromQL expression produces results with a `cluster` label (e.g., `KubePersistentVolumeFillingUp` aggregated per cluster). These alerts can be attributed to specific clusters and should contribute to the respective cluster's health tile in the heatmap, just like spoke-forwarded alerts.
  - *Fleet-wide hub alerts* — alerts that aggregate across clusters without a per-cluster breakdown (e.g., "more than N clusters not reporting"). These cannot be shown per cluster. They should be surfaced on the hub tile ("Global Platform") and in the alerts list with a "Hub" scope, but they do not contribute to any individual cluster's health.
  - The UI needs to distinguish these two cases based on the presence of the `cluster` label on the alert. Design the heatmap, component health tables, and drill-down flows to handle both.

- **Healthy clusters in the Fleet Health Heatmap:** The heatmap is driven by real-time alerts from Hub Alertmanager, aggregated in the monitoring-plugin cache. The cluster list comes from the Cluster Registry Cache (ManagedCluster resources), not from alerts. For each registered cluster, the cache checks whether any cluster-health-impacting alerts exist; if none, the cluster gets a green tile with full details (name, labels, node count, etc.). This ensures healthy clusters are always visible and the heatmap represents the full fleet, not just clusters with problems. Additionally, the existing `acm_managed_cluster_labels` metric (produced by MCE, already federated to hub Thanos) can be used in dashboard panels to enumerate all managed clusters and their metadata — enabling green tiles and fleet-complete views even in PromQL-driven dashboards. The same metric is already used by the existing `ManagedClusterMetricsMissing` alert to detect non-reporting clusters (see [Hub Rule Management — cluster not reporting](multi-cluster-hub-rule-management.md#proposed-new-default-hub-alert-cluster-not-reporting)).

- **Per-cluster alerting infrastructure configuration UI:** Several alerting infrastructure settings affect how alerts, metrics, silences, and notifications flow between spoke and hub. Today these require manual ConfigMap/Secret edits or GitOps — there is no UI surface for them. The alerting management UI should provide a unified configuration panel (e.g., accessible from the cluster tile or Management tab) that groups these settings:
  1. **Notification topology** — whether a cluster uses spoke-local AM notifications, hub-only notifications, or both. ACM already supports disabling spoke Alertmanagers. This affects silence routing (hub AM vs spoke AM).
  2. **ManagedCluster label enrichment** — cluster labels (e.g., `region`, `env`, `provider`, `availability_zone`) are available on the hub via ManagedCluster resources. They are enriched at three levels, all MVP:
     - **API read-time enrichment:** the monitoring-plugin looks up the alert's `managed_cluster`, fetches the ManagedCluster resource from the Cluster Registry Cache, and attaches selected labels to the API response for UI filtering and grouping.
     - **Hub AM relabeling for routing:** a relabel config controller watches ManagedCluster resources and generates hub AM `alert_relabel_configs` that inject cluster labels onto alerts by matching `managed_cluster`. Required for AM notification routing — routing rules can only match on labels present on the alert. The controller keeps relabel configs in sync as clusters are added, removed, or relabeled.
     - **`acm_managed_cluster_labels` for dashboards:** the existing `acm_managed_cluster_labels` metric (produced by MCE, already federated to hub Thanos) exposes ManagedCluster labels. Dashboards use `group_left` joins at query time — e.g., `label_replace(acm_managed_cluster_labels, "cluster", "$1", "name", "(.*)")`. Always reflects current labels even on historical queries. No new metric needed.
     - All three share a single allowlist to control which labels are exposed and avoid sensitive data leakage.
  3. **Metrics federation allowlist** — which additional metrics to federate from spoke to hub Thanos for custom hub rules. Today requires editing the metrics-collector allowlist ConfigMap (see [Hub Rule Management — Open Questions](multi-cluster-hub-rule-management.md#open-questions)).
  4. **Alert forwarding to hub AM** — enable/disable spoke-to-hub alert forwarding per cluster. Today controlled by the MCOA endpoint operator injecting `additionalAlertmanagerConfigs`, with no UI toggle.
  - Needs UX design: scope (per-cluster, per-cluster-group, or fleet-wide defaults), placement in the UI, and interaction with existing MCOA operator configuration.

## Alternatives (Not Implemented)

Alternatives (e.g., console plugins only, or hub-only aggregation without spoke-local views) are discussed in the parent [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) proposal.

## Test Plan

Test planning is coordinated with the Observability UI team: unit and integration tests for console flows, and end-to-end validation against a multi-cluster test environment with MCOA and representative topologies.

## Graduation Criteria
- **Tech Preview**: Scope, UX, and release `Graduation Criteria` will be defined with the Observability UI team. This enhancement provides the backend APIs and behaviors they consume.
- **GA**: Finalize scope and UX with the Observability UI team. Deliver multi-namespace filtering, full test coverage, and complete docs.

### Dev Preview -> Tech Preview

Criteria will be aligned with the Observability UI team as the implementation lands; includes stable UX for core fleet alerting views behind preview flags if applicable.

### Tech Preview -> GA

Matches the GA bullet above: finalized scope and UX, multi-namespace filtering, full test coverage, and complete documentation.

### Removing a deprecated feature

Follow OpenShift console deprecation policy: announce in release notes, maintain compatibility for the stated window, then remove UI entry points and dead code in coordination with API lifecycle in the parent enhancements.

## Upgrade / Downgrade Strategy

UI changes ship with the console and MCOA release trains; upgrades pick up new views and API versions together. Downgrade may hide new UI if the console version rolls back while leaving data plane behavior to the operators documented in sibling enhancements.

## Version Skew Strategy

The multi-cluster console targets supported skew between hub and spokes per ACM/OpenShift guidance; UI calls the Alerting API version negotiated by the backend—see the parent proposal for API compatibility expectations.

## Operational Aspects of API Extensions

Operational runbooks (SLOs, troubleshooting) for alerting APIs and federation belong in [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) and operator docs; operators use the same APIs the UI exercises.

## Support Procedures

Support follows standard OpenShift/ACM procedures: gather must-gather, verify MCOA and managed cluster connectivity, and escalate to Observability UI or platform teams per component ownership documented in the parent proposal.
