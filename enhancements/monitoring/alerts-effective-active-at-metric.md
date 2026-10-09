---
title: alerts-effective-active-at-metric
authors:
  - "@sradco"
reviewers:
  - "@jan--f"
  - "@jgbernalp"
approvers:
  - "@jan--f"
  - "@jgbernalp"
api-approvers:
  - None
creation-date: 2026-01-11
last-updated: 2026-01-11
status: review
tracking-link:
  - "https://issues.redhat.com/browse/CNV-74336"
see-also:
  - "/enhancements/monitoring/alert-overrides.md"
  - "/enhancements/monitoring/alerts-ui-management.md"
  - "/enhancements/monitoring/multi-cluster-alerts-ui-management.md"
replaces:
  - ""
superseded-by:
  - ""
---

# Alerts Effective ActiveAt Metric

## Summary

Introduce a Prometheus/Thanos‑queryable metric that reports pending and firing alerts with labels after user/platform relabeling is applied, including whether an alert is currently silenced. The metric value is the alert activation timestamp in Unix epoch seconds. This enables consistent, post‑relabel fleet and single‑cluster queries of relabeled alerts over time.

## Motivation

Today, the standard `ALERTS` series expose alert state but have value `1` and reflect labels before OpenShift alert relabeling is effectively consumed by downstreams. UIs and fleet queries need the alert activation time and the effective (post‑relabel) labels to:
- Order incidents by first‑seen time, not sample scrape time.
- Aggregate consistently across clusters and components after relabeling.
- Group alerts based on their start time (Incident detection improvement).

Providing a single series that encodes post‑relabel labels, `activeAt`, and current silenced status allows efficient monitoring in both single and multi-cluster use cases.
It can be used in dashboards to monitor the clusters and components health over time, the `Top Firing Alerts` and `Most Impacted Components` across clusters,  and asses the clusters stability over time.

### User Stories

- As a cluster admin, I want to sort and group alerts by when they actually started, so that I can triage incidents by impact and duration.
- As a cluster admin, I want to aggregate alerts across clusters by component, severity, and relabeled ownership, so that I can calculate the cluster and components health accurately.
- As an SRE, I want to see the alert firing alerts over time and see if I have a new issue that needs addressing or if there is an issue the fluctuates and also needs addressing.
- As a console user, I want the UI to reflect updated labels after `AlertRelabelConfig`, so that views, filters, and saved searches remain accurate.
- As a fleet operator, I want a single series to power cross‑cluster dashboards without joining multiple APIs, so that performance and reliability scale with fleet size. For example a top firing alerts and most impact components across clusters dashboards.

### Goals

- Provide a new metric that:
  - Includes pending and firing alerts.
  - Carries labels after alert relabeling is applied.
  - Uses the alert activation time (`activeAt`) as the metric value (epoch seconds).
  - Includes a `silenced` label.
  - Include all alert labels by defaulta, but provide configuration to restrict labels (allowlist or denylist) when needed.

### Non-Goals



## Proposal

Add a new gauge series exposed to Prometheus/Thanos to CMO:

- Name: `alerts_effective_active_at_timestamp_seconds`
- Type: Gauge
- One sample per alert instance in `pending` or `firing` state.
- The alert value equals Unix epoch seconds of the time the alert entered the `pending` state (activation time).
- Labels reflect the effective labels after configured relabeling has been applied.
- When Alertmanager is enabled and reachable, a `silenced` label shows whether the alert currently matches an active silence.

Post‑relabel:
- The metric’s labels MUST reflect the alert labels after `AlertRelabelConfig` are applied at rule evaluation time.

Production source:
- Implemented as a controller/exporter in the monitoring stack (co‑located with the rule evaluator: Prometheus or Thanos Ruler) that:
  1) Reads evaluated alerts and their `activeAt` from the `/api/v1/rules` endpoint (or an internal interface) where rule evaluation has already applied relabeling.
  2) Optionally queries Alertmanager (cluster service) for active alerts or silences to compute a `silenced` flag per alert instance. Two supported strategies:
     - Fetch `/api/v2/alerts` and read `status.silenced`.
     - Or fetch `/api/v2/silences` and apply matcher evaluation locally to the alert label set.
  3) Emits `alerts_effective_active_at_timestamp_seconds` with the selected labels (including `silenced` when enrichment is enabled) and the `activeAt` epoch seconds as the gauge value.
  4) Optionally enforces label filtering (allowlist or denylist) per configuration to cap cardinality or remove sensitive labels.

Evaluator read path details:
- Read from the evaluator’s rules API: GET `<prometheus-or-thanos-ruler>/api/v1/rules`, then:
  - Keep only entries where `rule.type == "alerting"`.
  - For each `rule.alerts[]` take alerts with `state in {"pending","firing"}`.
  - Use `alerts[i].activeAt` for the start timestamp.
- Post‑relabel labels:
  - For Platform alerts, Apply the `AlertRelabelConfig` logic to each alert’s label set. Treat a relabel “drop” as excluding the alert from output.
- Silenced alerts (optional):
  - Fetch from Alertmanager the silenced alerts and mark matching alerts as silenced.

### Workflow Description

Actors:
- cluster‑monitoring‑operator (CMO)
- Prometheus/Thanos Ruler
- alerts‑effective exporter
- Thanos Querier
- Console / external clients

Flow (single cluster):
1. Rule evaluation runs in Prometheus/Thanos Ruler. Relabeling is applied (per `AlertRelabelConfig`).
2. The alerts‑effective exporter scrapes `/api/v1/rules` and extracts active alerts with their effective labels and `activeAt`.
3. If Alertmanager is enabled and reachable, the exporter determines a `silenced` flag for each alert (via `/api/v2/alerts` or `/api/v2/silences`).
4. The exporter publishes `alerts_effective_active_at_timestamp_seconds{...} = activeAt_epoch`.
5. Thanos Querier exposes the metric to the console and external clients.
6. Console queries can sort by `alerts_effective_active_at_timestamp_seconds` and filter/group by the effective labels, including `silenced`.

Flow (fleet/HyperShift):
1. Each managed cluster exposes the series.
2. Thanos in the hub aggregates series. Fleet dashboards query a single metric with consistent post‑relabel labels.

### API Extensions

None.

### Topology Considerations

#### Hypershift / Hosted Control Planes
Supported: the exporter co‑locates with the rule evaluator. Series include `cluster` external label to distinguish guest clusters.

#### Standalone Clusters
Supported as above.

#### Single-node Deployments or MicroShift
Lightweight exporter footprint, cardinality guardrails apply. No special handling beyond resource requests.

#### OpenShift Kubernetes Engine
No differences expected. Relies on standard monitoring stack components present in OKE.

### Implementation Details/Notes/Constraints

- Component form: prefer a sidecar container next to Prometheus/Thanos Ruler to minimize network hops and authorization complexity. Alternative: a small deployment with RBAC to read rules.
- Label allowlist:
  - Default: include all labels from alert instances (no filtering).
  - Optional: configure allowlist or denylist via CMO configuration to limit labels for performance/compliance.
- Alertmanager (AM):
  - Disabled by default in Dev/Tech Preview if we want to minimize moving parts. It can be enabled via CMO config.
  - Configure in‑cluster AM service URL and auth, cache responses and set request budgets.
  - If AM is unavailable or times out, exporter sets `silenced="false"` and emits a self‑metric for enrichment failures.
- Cardinality controls:
  - When label filtering is enabled, drop labels not in the allowlist (or drop those in the denylist).
- Metric help:
  - `help="Activation time (Unix epoch seconds) for pending and firing alerts after relabeling has been applied."`

### Risks and Mitigations

- Cardinality blow‑up:
  - Default includes all labels, like the Prometheus ALERTS metric. Admins can enable allowlist/denylist to restrict labels in sensitive or large environments.
  - Document the allowlist/denylist usage and provide examples for filtering configuration.
- Consistency with Alertmanager routing:
  - Ensure source of truth is the post‑relabel alert state from the evaluator.
  - Document that downstream Alertmanager relabeling (if any) may diverge in advanced configurations.
- Performance:
  - The exporter reads `/api/v1/rules`. This should have limited overhead when scraping at rule interval with caching.
  - Enforce concurrency limits and timeouts and add a self‑monitoring SLI on exporter latency.
- Alertmanager availability:
  - Enrichment is optional. If AM is missing it does not block metric emission.
  - Expose an SLI/alert when AM is enabled but failing and define UI fallback behavior (treat `silenced="false"` when enrichment is unavailable).

### Drawbacks

- Adds a small component and another scrape target to the stack.
- Slight duplication of information already available via HTTP APIs, but provides better query‑time ergonomics and fleet scalability.

## Alternatives (Not Implemented)

- Derive `activeAt` from `ALERTS` using PromQL functions:
  - Not feasible to reconstruct true activation time reliably. `ALERTS` value is `1`, and sample timestamps reflect scrape time, not `activeAt`.
- Use Alertmanager `/api/v2/alerts` as the primary source:
  - We deliberately avoid using AM as the primary source for `activeAt` and labels, since AM might be missing. AM is only used to enrich `silenced` status when configured.

## Open Questions [optional]

1. Do we also need an `inhibited` dimension similar to `silenced` for parity with Alertmanager status?

## Test Plan

- Unit tests for exporter label allowlist, cardinality controls, and timestamp conversion.
- E2E: create synthetic alerts with relabeling. verify:
  - Labels on the metric match post‑relabel labels.
  - Values equal the `activeAt` of the alert (± scrape skew).
  - Only `pending` and `firing` states are present with correct `state` label.
- E2E (silenced): create an active silence that matches a known alert. verify that  `silenced="true"` when AM is enabled and reachable. verify default `silenced="false"` when disabled or AM unavailable.
- Scalability: run perf test with 5k alerts and verify exporter CPU/memory and scrape latencies remain within SLOs.
- Hypershift: validation in CI suite that series contain `cluster` label and aggregate correctly through Thanos.

## Graduation Criteria
### Dev Preview -> Tech Preview
- End‑to‑end availability of the series in single‑cluster deployments.
- Documentation of label allowlist and cardinality guidance.
- Basic e2e and perf tests in CI.

### Tech Preview -> GA
- Fleet validation with HyperShift and Thanos aggregation.
- More robust tests (upgrade, skew).
- Enabled by default. Guarded by performance SLOs and alerting on exporter health.
- User‑facing documentation in `openshift-docs`.

### Removing a deprecated feature
- Announce deprecation and support policy of the existing feature.
- Deprecate the feature and document safe removal/rollback steps.

## Upgrade / Downgrade Strategy

- Introduce component and metric behind a feature gate in CMO configuration (TechPreview feature set).
- Upgrades preserve configuration. If disabled, the series disappears without impacting existing queries (guard with fallbacks in UI).

## Version Skew Strategy

- No API dependencies. Exporter follows the rule evaluator version.
- Thanos can aggregate across clusters regardless of exporter minor version, as the metric name and label schema are stable.

## Operational Aspects of API Extensions

Not applicable.

## Support Procedures

- Detect issues via:
  - Exporter health metrics and readiness/liveness probes.
  - Alert on exporter scrape timeouts and series absence.
- Disable by turning off the feature gate in CMO config, safe to remove with no persistent state.

## Infrastructure Needed [optional]

None beyond the small exporter sidecar/deployment added to the monitoring stack.

