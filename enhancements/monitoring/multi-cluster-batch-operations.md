---
title: multi-cluster-batch-operations
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
---
# Multi-Cluster Batch Operations

This proposal covers batch operations for applying rule changes and silences across multiple managed clusters in a single request. It is Part 3 of the [Multi-Cluster Alerts UI Management](multi-cluster-alerts-ui-management.md) umbrella proposal.

For shared context on the existing alert forwarding infrastructure, alert data storage lifecycle, and the ACM alerting developer preview, see the [Current State](multi-cluster-alerts-ui-management.md#current-state) section of the parent proposal.

## Table of Contents

- [Summary](#summary)
  - [Relationship to GitOps and ACM Policies](#relationship-to-gitops-and-acm-policies)
- [User Stories](#user-stories)
- [API Endpoints](#api-endpoints)
  - [Batch rules apply](#batch-rules-apply)
  - [Batch silences](#batch-silences)
  - [Preview / confirmation](#preview--confirmation)
  - [Batch response schema](#batch-response-schema)
- [Risks & Mitigations](#risks--mitigations)

## Summary

Fleet administrators need to apply the same rule change or silence to many clusters at once without visiting each cluster individually. Batch endpoints fan out to each target cluster's existing Single-Cluster Alerting API via ManagedClusterProxy. The UI shows a confirmation step (using cached data from the Rule Metadata Cache and Cluster Registry Cache) before applying, and the API returns per-cluster results with partial success support.

### Relationship to GitOps and ACM Policies

Batch operations via the API are not the only way to apply alert rules across clusters. Two existing mechanisms serve similar goals:

- **GitOps (ArgoCD / OpenShift GitOps)**: Teams can define `PrometheusRule` and `AlertRelabelConfig` resources in Git and use ArgoCD ApplicationSets to deploy them to sets of clusters. This is the preferred approach for teams with mature GitOps workflows — it provides version control, audit trail, drift detection, and rollback. The batch API respects GitOps ownership: clusters whose alert resources are managed by ArgoCD are flagged in the confirmation step and skipped during apply, with guidance to make the change in Git instead.

- **ACM Policies**: ACM's Policy framework can enforce that specific `PrometheusRule` or `AlertRelabelConfig` resources exist on clusters matching a `Placement` selector. Policies provide compliance reporting and remediation. Like GitOps, policy-managed resources are declarative and should not be overwritten by imperative batch operations.

**Where batch operations add value alongside these approaches:**

- **Ad-hoc and exploratory changes**: Not all teams use GitOps or policies for alerting rules. For teams that manage alert configurations imperatively (unmanaged rules), the batch API provides a fast, UI-driven way to apply changes without writing YAML or setting up ArgoCD ApplicationSets.
- **Silences**: Neither GitOps nor ACM policies are well-suited for managing Alertmanager silences, which are ephemeral, time-bounded, and stored in Alertmanager's in-memory state (not as Kubernetes resources). Batch silence operations (`POST /hub/silences/batch`) fill this gap.
- **Visibility and confirmation**: Even when GitOps or policies handle deployment, the batch API's cached preview step (Rule Metadata Cache + Cluster Registry Cache) provides a unified view of which clusters have a rule, which are GitOps-managed, and which are unmanaged — helping administrators understand fleet state before deciding where to make changes and through which mechanism.

## User Stories

1. **Batch-apply a rule to selected clusters** — As an Ops Lead, I want to apply or update a specific alert rule and deploy it across a list of specific clusters (that I can easily search for by their names, labels, versions, etc.) in one action, without visiting each cluster UI.

2. **Manage all types of alerting rule across selected spoke clusters** — As a fleet Admin, I want to create, update, disable, and delete alerting rules — both platform and user-defined — across a selected set of clusters in one workflow, so I can maintain consistent rule sets across my fleet without repeating the same action on each cluster.

3. **Bulk update labels across clusters** — As a Platform Admin, I want to update labels — such as severity, component, layer, or custom routing labels — for a set of alerting rules across selected clusters in one action, so alert routing, escalation, and classification are consistent without repeating the change on each cluster.

## API Endpoints

Base path: `/api/v1/alerting`

> Note: GET endpoints should remain compatible with upstream Thanos/Prometheus (query parameters and response schemas) wherever applicable to enable native Perses integration.

### Batch rules apply

- `POST /hub/rules/batch/apply` — Apply a rule change (create, update, disable, delete) to a set of target clusters.
  - The request specifies the rule definition and a list of target clusters (by name or label selector).
  - The API fans out to each target cluster's Alerting API via ManagedClusterProxy.
  - Returns a batch response with per-cluster status.

### Batch silences

- `POST /hub/silences/batch` — Create the same silence on multiple spoke Alertmanagers.
  - The request specifies silence matchers, duration, and a list of target clusters.
  - The API fans out to each target cluster's Alertmanager via ManagedClusterProxy.
  - Returns a batch response with per-cluster status.

### Preview / confirmation

There is no separate `batch/preview` endpoint. Instead, the UI shows a confirmation step before applying batch changes. The confirmation dialog uses data already available from the Rule Metadata Cache and Cluster Registry Cache to display:

- The list of target clusters that will receive the change.
- Clusters that will be **skipped** because their resources are GitOps-managed (detected from ArgoCD annotations in cached rule metadata).
- Clusters where the user **lacks write permissions** (detected from RBAC information available on the hub).
- Whether the target rule already exists on each cluster (from cached rule definitions), so the user can see which clusters get a create vs. an update.

This cached-data approach is fast (no fan-out to spokes) and provides sufficient confidence for the user to proceed. The actual `batch/apply` response then reports the definitive per-cluster results — including any conflicts, validation errors, or transient failures that cached data could not predict. A dedicated dry-run endpoint was considered but rejected because it would require the same fan-out cost as the actual apply, and its results would be immediately stale (TOCTOU race between preview and apply).

### Batch response schema

- `summary`: total, succeeded, failed, skipped counts.
- `results[]`: per-cluster entries with `cluster`, `status` (success | failed | skipped | denied), `error` (if failed), `ruleId` (if created).
- Partial success is expected — the API applies to all reachable clusters and reports failures individually.
- Concurrency: fan-out uses bounded concurrency (configurable, default 10) with per-cluster timeouts. Long-running batches are processed synchronously within a single HTTP request for MVP; async job-based execution is a future consideration.

## Risks & Mitigations

- **Fleet-scale performance**: batch operations can fan out to many clusters. Apply concurrency limits and backpressure. Use per-cluster timeouts and partial-success reporting. Support resume or retry for long-running batches. Paginate or stream large rule and alert lists.
- **Connectivity or proxy reliability**: outages or high latency in the ManagedClusterProxy on the hub may disrupt writes. Use retries with jitter, per-cluster backoff, circuit breakers, and a degraded read mode backed by cached data.
- **RBAC and ownership across clusters**: enforce hub and per-cluster RBAC. Treat GitOps-owned resources as read-only. Return per-cluster denial reasons in batch results.
- **Drift and consistency**: detect and surface drift between platform rules and relabel configs on spokes. Provide conflict reporting and optional reconciliation guidance.
