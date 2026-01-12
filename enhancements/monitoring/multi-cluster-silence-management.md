---
title: multi-cluster-silence-management
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
---
# Multi-Cluster Silence Management

This proposal covers silence management across the multi-cluster fleet: making spoke silences visible on the hub, managing hub-scoped and spoke-scoped silences via a unified API, and the relationship between silences and hub Alertmanager as a centralized notification hub. It is Part 4 of the [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) umbrella proposal.

For shared context on the existing alert forwarding infrastructure, alert data storage lifecycle, and the ACM alerting developer preview, see the [Current State](multi-cluster-alerts-ui-management.md#current-state) section of the parent proposal.

## Summary

Spoke clusters maintain their own Alertmanager silences locally. Without replication, spoke-silenced alerts appear as firing on the hub alerts page, creating an inconsistent view. This proposal introduces a silence sync controller that replicates spoke silences to hub Alertmanager, and a unified API for managing silences across scopes. The silence sync controller is also essential for notification consistency when hub AM is used as a centralized notification hub.

**Note:** The silence sync controller is one of two proposed approaches — see Open Questions for the alternative (Silence Cache). The design has not yet been finalized.

## Silence Sync Controller

A silence sync controller replicates spoke Alertmanager silences to hub Alertmanager so that hub AM natively suppresses spoke-silenced alerts without a read-time cache in the console backend.

### Deployment

The controller runs on the hub as a single deployment in `open-cluster-management-observability`. It uses the Cluster Registry Cache (ManagedCluster watch) to discover spoke clusters and their ManagedClusterProxy endpoints.

### Sync mechanism

The controller periodically polls each spoke Alertmanager (`GET /api/v2/silences` via ManagedClusterProxy) and reconciles the state on hub AM:

- **Create**: when a new active silence is found on a spoke, the controller creates a replica on hub AM. The replica includes all original matchers plus an additional `managed_cluster=<cluster-name>` matcher to scope it to that spoke's alerts. A label or annotation `sync.source=<cluster-name>/<silence-id>` is added to the hub silence comment for traceability and to prevent conflicts with user-created hub silences.
- **Update**: if a spoke silence's `endsAt` is extended or matchers change, the controller expires the old hub replica and creates a new one.
- **Expire/Delete**: when a spoke silence expires or is deleted, the controller expires the corresponding hub replica. Expired silences on hub AM are cleaned up by Alertmanager's built-in GC.

### Polling interval and consistency

The controller polls each spoke on a configurable interval (default 30s). Between polls, there is a window where a newly created or expired spoke silence is not yet reflected on hub AM. This is acceptable because:
- New silences: the alert may appear as firing for up to 30s on the hub alerts page before being suppressed. Spoke Alertmanager is already suppressing notifications immediately.
- Expired silences: the hub may continue suppressing the alert for up to 30s after the spoke silence expires. No missed notifications — spoke AM resumes notifications immediately.

### Spoke unreachable

When a spoke is unreachable, the controller retains existing hub replicas for that spoke (stale but safe — alerts remain suppressed). After a configurable staleness TTL (default 10 min), the controller marks replicas as potentially stale but does not automatically expire them to avoid false firing alerts during transient connectivity issues. The `GET /hub/alerts` response includes a `silenceStateStale` flag for clusters whose silence state may be outdated.

### Conflict avoidance

Hub replicas are identifiable by the `sync.source` tag in the silence comment. The controller only manages silences that carry this tag. User-created silences on hub AM (without the tag) are left untouched. This prevents conflicts between user-created hub silences and controller-managed replicas.

When a user creates a silence for a spoke alert via the API (`POST /hub/silences` with `scope: cluster:<name>`), the API creates it on the spoke AM. The sync controller then picks it up on the next poll and creates a hub replica. The user sees the silence reflected on both the spoke and the hub alerts page.

## API Endpoints

Base path: `/api/v1/alerting`

> Note: GET endpoints should remain compatible with upstream Thanos/Prometheus (query parameters and response schemas) wherever applicable to enable native Perses integration.

- Silences
  - `GET    /hub/silences`           — List silences from hub Alertmanager (includes hub-scoped silences and spoke silence replicas created by the sync controller). Optionally filter by `cluster` to return only silences scoped to a specific spoke.
  - `POST   /hub/silences`           — Create a silence. The request includes a `scope` field: `hub` targets hub Alertmanager directly (for hub-scoped alerts); `cluster:<name>` targets the specified spoke Alertmanager via ManagedClusterProxy (the sync controller will replicate it back to hub AM). Request body matches the Alertmanager silence schema (matchers, startsAt, endsAt, comment, createdBy).
  - `DELETE /hub/silences/{id}`      — Expire a silence. In Alertmanager, deleting a silence means expiring it (setting its `endsAt` to now) — silences are never physically removed, only expired. Similarly, updating a silence is implemented as expiring the current silence and creating a new one. For hub-scoped silences, expires on hub Alertmanager. For spoke-scoped silences, expires on the spoke Alertmanager via ManagedClusterProxy (the sync controller removes the expired replica from hub AM on the next poll).

- Silences scope policy:
  - Hub-initiated silences for alerts originating on spokes are created on the respective spoke Alertmanager(s) via ManagedClusterProxy. Hub-scoped alerts are silenced on the hub Alertmanager only.
  - The silence sync controller replicates spoke Alertmanager silences to hub Alertmanager with `managed_cluster` matcher scoping, so hub AM natively suppresses spoke alerts.

## Hub Alertmanager as Centralized Notification Hub

Hub Alertmanager receives alerts from all spoke clusters (via `additionalAlertmanagerConfigs`) and from ThanosRuler. By default, hub AM is configured with a `null` receiver — it accepts and stores alert state but does not send notifications. However, users can customize the hub AM configuration to add real notification receivers (Slack, PagerDuty, email, webhooks, etc.) and routing rules.

This makes hub AM a potential **centralized notification hub** for the fleet: instead of configuring receivers on each individual spoke cluster, users can configure them once on hub AM and receive notifications about alerts from all managed clusters in a single place. The `managed_cluster` label on spoke alerts enables routing by cluster (e.g., production clusters to PagerDuty, dev clusters to Slack).

**Spoke Alertmanager disabled topology:** ACM supports disabling Alertmanager on spoke clusters so that all alert notifications are managed exclusively at the hub level. When spoke AM is disabled, spoke Prometheus still forwards alerts to hub AM via `additionalAlertmanagerConfigs`, but there is no spoke AM to create silences on or poll for silence sync. In this topology:
- The silence sync controller skips spokes with disabled AM (no AM to poll)
- Silences for spoke alerts are created directly on hub AM (no spoke-side routing needed)
- Hub AM is the sole notification endpoint — simplifying notification management but requiring all silence and routing configuration to be hub-centric

**Implications for silence management:**

When hub AM is used for notifications, silences on hub AM have real notification impact — not just UI display. This strengthens the rationale for the silence sync controller:
- A spoke-local silence that is not replicated to hub AM would suppress the alert on the spoke's notification pipeline but NOT on the hub's notification pipeline, resulting in duplicate or unwanted notifications from the hub.
- The silence sync controller ensures consistency: a silence created on a spoke is replicated to hub AM with `managed_cluster` matcher scoping, suppressing both the spoke's local notifications and the hub's centralized notifications.

**Current state and future work:**

- The hub AM config Secret uses a `skip-creation-if-exist: "true"` annotation, so the MCO operator creates the default `null` config only on initial deployment and does not overwrite user customizations.
- Users can manually edit the `alertmanager-config` Secret in `open-cluster-management-observability` to add receivers and routes.
- A future UI for managing hub AM notification routing (receivers, routes, inhibition rules) is listed as a "Could-Have" feature. For MVP, users configure receivers manually or via GitOps.

## Risks & Mitigations

- **Silence sync controller fan-out**: the controller polls each spoke's Alertmanager every 30s. At 1000 clusters, this produces ~33 silence polls/sec sustained. Use bounded concurrency (e.g., 20 parallel requests), staggered refresh intervals (jitter), and prioritize clusters with recent changes. Monitor silence sync latency and spoke AM error rates.
- **Silence state inconsistency window**: between sync polls (~30s), new or expired silences may not be reflected on hub AM. Acceptable because spoke AM handles notifications immediately; the hub UI delay is bounded and well-understood.
- **Spoke unreachable**: stale silence replicas are retained to avoid false firing alerts. The `silenceStateStale` flag in the API response signals outdated silence state to the UI.
- **Conflicts with user-created hub silences**: the `sync.source` tag distinguishes controller-managed replicas from user-created silences. The controller never modifies or deletes silences without the tag.
- **CRDT consistency within spoke AM clusters**: Alertmanager uses [Conflict-free replicated data types (CRDTs)](https://en.wikipedia.org/wiki/Conflict-free_replicated_data_type) to replicate silences across instances within a single AM cluster, favoring availability over consistency. This means two AM instances in the same spoke may not have a fully consistent silence state at any given moment. The sync controller polls one spoke AM instance, so it may see a slightly stale view. This is an inherent property of Alertmanager's gossip protocol and adds another layer of eventual consistency to the silence replication chain (spoke AM gossip → sync controller poll → hub AM replica).

## Open Questions

- **Spoke silence visibility on hub — silence sync controller vs. Silence Cache:** Two options for making spoke-local silences visible in `GET /hub/alerts`:
  - *Option A (current design):* A silence sync controller (deployed on the hub) watches spoke Alertmanager silences and creates scoped replicas on hub Alertmanager with `managed_cluster` matcher scoping to prevent cross-cluster interference. Hub AM then natively suppresses spoke alerts — no read-time cache or matching needed. Trade-off: requires a new controller component, lifecycle sync (create/update/expire/delete must be mirrored), potential conflicts if hub users also create silences for spoke alerts directly on hub AM.
  - *Option B:* The console backend maintains a Silence Cache (per cluster, TTL 30s) by querying each spoke's Alertmanager (`GET /api/v2/silences`) via ManagedClusterProxy. At read time, silence matchers are matched against alert labels to determine silence state. Simpler to implement (code only, no new controller), degrades gracefully when a spoke is unreachable (stale cache with flag). Trade-off: fan-out queries to all spoke AMs every 30s; silence matching logic in the console backend.

- **Spoke-silenced alerts on the hub:** When a spoke alert is silenced locally, should it appear as silenced on the hub alerts view, or should silenced spoke alerts not be forwarded to the hub at all? Options: (a) silence sync controller replicates spoke silences to hub AM so they appear as suppressed on the hub (current design), (b) spoke Prometheus stops sending silenced alerts to hub AM entirely, (c) hub shows all spoke alerts regardless and silence state is spoke-local only. This affects the UX for users who expect spoke silence state to be visible on the hub without separate hub-side action.
