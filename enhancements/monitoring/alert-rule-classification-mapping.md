---
title: alert-rule-classification-mapping
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
creation-date: 2026-01-07
last-updated: 2026-01-21
tracking-link:
  - ""
---
# Alert Rule Classification Mapping

## Summary
This enhancement defines how OpenShift monitoring assigns each alerting rule (and the alerts it emits) a stable, user-visible classification: a `component` and an impact `layer`(See [Terminology](#terminology) section for definitions).

It documents the defaulting and fallback behavior, how users can override classification through centrally stored ConfigMaps, and how the Alerts API exposes the effective values in a Prometheus compatible response for UI filtering and display.

This design is part of enhancement: [Alerts UI Management](alerts-ui-management.md).

## Motivation
- Enable computation of component and cluster aggregated level health to help the UI prioritize clusters needing attention, highlight failing components, and speed troubleshooting in single and multi-cluster views.
- Provide alert with clear context, which is not possible today. Today this can only be deduced by the alert name, namespace, etc, which are not easy to sort and aggregated by to figure out what component has an issue and related alerts.

## Proposal

### User Stories
- As a multi-cluster admin, I will be able to see which clusters require my attention drill down by impacted component and scope of impact to the specific related alerts.
- As a namespace owner, I can see only the clusters with alerts that impact my namespaces, drill down to see which components are impacted, and then the specific related alerts.
- As an operator developer, if I want to set the impacted component and scope of impact for my alerts, I can do so in my alert definitions and they will take precedence over heuristics created by observability developers.
- As an observability developer, I need to create heuristics for setting impacted component and scope of impact so that every alert in the cluster has them and they are as accurate as possible.
- As a cluster admin or a user that has permissions to update the alerts defined in my namespaces, I will be able to update an alert’s impacted component and/or scope of impact through the UI and have it override all other predefined settings.
- As a cluster Admin, I want platform alerts to be treated as `cluster` impact `layer` by default, so global issues are clearly surfaced.
- As namespace owner, I want workload alerts to be treated as namespace-scoped impact `layer` by default so I can filter and troubleshoot within my project scope.

### Goals

High level goal of this feature is:
  - Adding a `component` and impact `layer` mapping for each alert and alert rule, so that we can group alerts by them and be able to calculate Cluster and Components health.
  - Let users to update them for individual alert rules and alerts, if needed.

1. Define allowed impact `layer` values classification rules and default behavior for user workload monitoring alert rules and alerts.
2. Define how to create a consistent mapping across all alert rules types.
3. Specify API enrichment fields for alerts and rules, and expected UI filters/columns, while maintaining compatibility with Prometheus/Thanos schemas.

### Non-Goals
- Changing upstream Prometheus/Thanos APIs or schemas.
- Redefining platform vs user source detection beyond what is documented here.
- Enforcing a specific UI, this defines the model that UIs should follow.

### Workflow Description
1) The Classification Component (console backend plugin) watches PrometheusRules and keeps an in-memory registry of alerting rule definitions and their computed classification defaults.
2) Users express intent with overrides stored in the monitoring plugin namespace, sharded by rule namespace, and merged on top of defaults in memory.
3) The API serves the effective classification to the Console (defaults merged with overrides).
4) Defaults are not persisted. Only user overrides are persisted as ConfigMaps.
5) The UI displays and filters by the effective classification.

### API Extensions
- No new Kubernetes API extensions (no new CRDs/webhooks/aggregated API servers).
- The console backend exposes/extends alerting APIs used by the UI:
  - GET `/api/v1/alerting/alerts`: Prometheus-compatible response with additive fields `openshift_io_alert_rule_id`, `openshift_io_alert_component`, `openshift_io_alert_layer`.
  - GET `/api/v1/alerting/rules`: read path that surfaces a unified, post-relabel view of rules and their effective classification.
    - Additive fields for each rule:
      - `classification`: nested object:
        - `openshift_io_alert_rule_component`: effective component for the rule (after applying overrides and defaults)
        - `openshift_io_alert_rule_layer`: effective layer for the rule (after applying overrides and defaults)
        - `classificationSource: default|user`: where the effective classification came from (computed default vs user override)

  - PATCH `/api/v1/alerting/rules/{ruleId}`: Update classification overrides for one `openshift_io_alert_rule_id`.
    - Input: a nested object:
      - `classification.openshift_io_alert_rule_component`
      - `classification.openshift_io_alert_rule_layer`
    - Behavior: resolve `{ruleId}` to its PrometheusRule namespace using the in-memory registry, persist changes into the shard `alert-classification-overrides-<rule-namespace>` (keyed by base64url(openshift_io_alert_rule_id)) in the plugin namespace using ConfigMap `resourceVersion`, and update the in-memory cache.
    - Reset: setting either field to `null` removes that override for `{ruleId}`, causing the effective value to fall back to defaults.
  - PATCH `/api/v1/alerting/rules`: Bulk update classification overrides using the existing bulk rules PATCH API.
    - Behavior: for each `ruleId`, apply the same persistence and cache update logic described above.

Example payloads:

```json
{ "classification": { "openshift_io_alert_rule_component": "kube-apiserver", "openshift_io_alert_rule_layer": "cluster" } }
```

Response record example:

```json
{
  "openshift_io_alert_rule_id": "ClusterOperatorDown;da08af39c9dec02d06b765938de86c34cfda26ea9dad87e80851aaa9cc92eb53",
  "namespace": "openshift-monitoring",
  "classification": {
    "openshift_io_alert_rule_component": "kube-apiserver",
    "openshift_io_alert_rule_layer": "cluster",
    "classificationSource": "default"
  },
  "openshift_io_alert_source": "platform"
}
```

RBAC (high level):
- Platform stack: only users who can update alerting definitions in the platform stack should be allowed to update classification for platform `openshift_io_alert_rule_id`s.
- User workload monitoring: only users with edit permissions in the workload namespace containing the PrometheusRule should be allowed to update classification for `openshift_io_alert_rule_id`s in that namespace.

## Terminology
- Component: Logical owner of the alert or rule (e.g., `kube-apiserver`, `etcd`, a namespace, a team).
- Layer: Impact scope. Allowed values: `cluster`, `namespace`.
- source: Origin of the rule/alert. Either `platform` (cluster monitoring stack) or `user` (User workload monitoring).
- Platform stack: The `openshift-monitoring` stack managed by Red Hat–supported operators. Deploys a Prometheus instance by default for every OpenShift cluster.
- User stack (User workload monitoring): User monitoring stack for application namespaces. Deploys a second Prometheus instance, if enabled in the Cluster Monitoring Operator(CMO) settings.
- Platform / User alert and alert rules -  Alerts and alert rules are considered `platform` if they belong to the `Platform stack`. Otherwise they are `user`.

## Mapping Logic
### Primary Mapping (Classifier)
- The backend uses matchers to compute a `(layer, component)` tuple from rule/alert labels.
- We try to stay aligned with the matchers and behavior implemented by `cluster-health-analyzer` that already uses the `layer`, `component` for `incident detection` feature.
- Typical mappings:
  - Core control-plane components → `layer=cluster`, `component=<cp-subsystem>`
  - Node/compute-related → `layer=cluster` and `component=compute`.
  - Namespace-level alerts → `layer=namespace`

#### How classifier heuristics are authored (high level)
- The classifier is implemented as an **ordered list of matchers**. **First match wins** and determines the effective values.
- Matchers are intentionally grouped from most-specific to most-general:
  - **Special-cases first**: alert families that require per-alert-instance classification (for example, where the effective component depends on runtime alert labels) are evaluated before general rules.
  - **General mappings next**: stable mappings for well-known platform and workload areas.
  - **Fallback last**: when nothing matches, we apply the documented fallback behavior.
- Adding or updating heuristics is a code change in the monitoring plugin (see `monitoring-plugin-machadovilaca/pkg/alertcomponent/matcher.go`), and should remain aligned with `cluster-health-analyzer` to avoid drift between backend and UI behavior.

Rule scoped default classification labels:
- If an alerting rule includes `openshift_io_alert_rule_component` and or `openshift_io_alert_rule_layer`, the backend uses those values as the default `component` and `layer` for alerts emitted by that rule, unless a user override replaces them.

### Precedence and matching semantics
The backend should compute the effective classification using this order, highest priority first:
1) Explicit user override fields: `openshift_io_alert_rule_component` and `openshift_io_alert_rule_layer` stored for this `openshift_io_alert_rule_id` in the centralized shard.
2) Rule-scoped default labels on the alerting rule: `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`.
3) Classifier-based mapping (matchers).
4) Fallback: `component=other`, derive `layer` from source (platform → cluster, user → namespace).

Non-MVP behavior (dynamic mapping via `*_from` and per-alert-instance overrides via `overridesByMatch`) is specified in the "Non-MVP / Future Work" section below.

### Non-MVP / Future Work
#### Dynamic mapping configuration (`*_from`)
- If `openshift_io_alert_rule_component_from` is present in the override entry (stored in the shard for the matching `openshift_io_alert_rule_id`), the backend derives the effective alert instance `openshift_io_alert_component` from the specified alert label key at request time. Initial allowed values: `name` and `component`.
- If `openshift_io_alert_rule_layer_from` is present in the override entry (stored in the shard for the matching `openshift_io_alert_rule_id`), the backend derives the effective alert instance `openshift_io_alert_layer` from the specified alert label key at request time. Initial allowed values: `layer`.
- Invalid values are ignored and normal mapping continues.

#### Per-alert-instance overrides (`overridesByMatch`)
- If `overridesByMatch` is present in the override entry, the backend may override the effective alert instance `openshift_io_alert_component` and or `openshift_io_alert_layer` for specific alert instances emitted by the same rule.
- Matching semantics:
  - Exact match only, `match` is a map of label key to value.
  - AND across keys, all pairs must be present on the alert.
  - First match wins, evaluate entries in order.

#### Correlation-based enrichment (Korrel8r)
In the future, we may be able to enhance classification by using signal-correlation tooling such as `korrel8r` to associate an alert with related Kubernetes resources and context (for example, the owning workload or operator). This could improve accuracy for alerts that do not carry enough information for reliable classification via static matchers alone.

This would be an **optional enhancement** and should not replace deterministic matcher-based behavior in the baseline design. Any such integration would need to consider performance, caching, and availability (classification must remain fast and predictable even when correlation is unavailable).

### Alert-specific dynamic classification (examples: CVO alerts)
Some alert families cannot be classified purely from the alerting rule definition because the component is derived from runtime alert labels that vary per alert instance. A common example is Cluster Version Operator alerts where the component is derived from the alert label `name` which identifies the ClusterOperator.

For these alerts, the backend should compute component and layer per alert instance using alert labels, even if the underlying rule has a static classification.

Example logic:
- If `alertname` is `ClusterOperatorDown` or `ClusterOperatorDegraded`
  - `layer = cluster`
  - `component = <labels.name>`, and if `name` is missing then use `component = version`

This matches the cluster-health-analyzer approach and enables “dynamic” per alert component mapping without requiring users to split rules.

### Fallback Mapping (When component is unknown)
If the classifier returns an empty component or `Others`:
- `component = other`
- `layer` is derived from `source`:
  - `platform` → `cluster`
  - `user` → `namespace`

Notes:
- Generated values are always one of `cluster|namespace`.


### Source Determination
- For rules: a rule is considered `platform` if it belongs to the cluster monitoring namespace (`openshift-monitoring`). Otherwise it is `user`.
- For alerts: considered `platform` when either:
  - `openshift_io_alert_source == platform`, or
  - `prometheus` label is prefixed with `openshift-monitoring/`.
  Otherwise `user`.

## Persistence and Overrides
### In-memory cache
- The Classification Component maintains an in-memory cache of:
  - PrometheusRules and their alerting rule definitions
  - computed default classification per `openshift_io_alert_rule_id`
  - merged user overrides from the namespace shard

### On demand user overrides (sharded in plugin namespace)
- Users only create an override ConfigMap when they need to change defaults.
- Name: `alert-classification-overrides-<rule-namespace>`
- Namespace: monitoring plugin namespace
- Storage schema:
  - Key: base64url(`<openshift_io_alert_rule_id>`)
  - Value: JSON object containing one or both of:
    - `openshift_io_alert_rule_component: <string>`
    - `openshift_io_alert_rule_layer: <cluster|namespace>`
    - `alertName: <string>` (optional, informational)
    - `prometheusRuleName: <string>` (optional, informational)
    - `prometheusRuleNamespace: <string>` (optional, informational)

Concurrency:
- The backend updates each shard using Kubernetes optimistic locking by reading and writing the ConfigMap `resourceVersion`.
- Optimistic locking means the backend updates a specific ConfigMap version, and retries if another writer updated it concurrently.

Example:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: alert-classification-overrides-openshift-monitoring
  namespace: mp-dev-ns
data:
  Q2x1c3Rlck9wZXJhdG9yRG93bjtkYTA4YWYzOWM5ZGVjMDJkMDZiNzY1OTM4ZGU4NmMzNGNmZGEyNmVhOWRhZDg3ZTgwODUxYWFhOWNjOTJlYjUz: '{"alertName":"ClusterOperatorDown","prometheusRuleName":"cluster-version","prometheusRuleNamespace":"openshift-monitoring","classification":{"openshift_io_alert_rule_component":"kube-apiserver","openshift_io_alert_rule_layer":"cluster"}}'
  U29tZVdvcmtsb2FkQWxlcnQ7MmVkZDE3OTA1MTNjNWExOWYzOTFlYTEyMTQ1YWU2MTYyNTBjNTM5OTAwMDM5MWJhZTk0MDFiNDc4ZmYyZGE1YQ: '{"alertName":"SomeWorkloadAlert","prometheusRuleName":"user-alerts","prometheusRuleNamespace":"my-alerts-ns","classification":{"openshift_io_alert_rule_component":"team-a","openshift_io_alert_rule_layer":"namespace"}}'
```

### User Overrides
- Users may override classification by creating or updating the shard in the plugin namespace.
- Validation:
  - `openshift_io_alert_rule_component`: non-empty, 1–253 chars, `[A-Za-z0-9._-]`, must start/end alphanumeric.
  - `openshift_io_alert_rule_layer`: one of `cluster|namespace`.
- Invalid overrides are ignored and the effective value falls back to defaults.
- Unknown `openshift_io_alert_rule_id` keys are ignored.

## Alerts API Enrichment
- Endpoint aligns with Prometheus `/api/v1/alerts` and adds fields (additive):
  - `openshift_io_alert_rule_id`
  - `openshift_io_alert_component`
  - `openshift_io_alert_layer`
- Classification for alerts is computed by correlating alerts to relabeled rules and using the effective rule classification as a default. For alert families that require dynamic classification (for example CVO alerts), the backend computes `component` and `layer` per alert instance from alert labels and uses that result. When correlation fails, the fallback mapping above applies and derives `layer` from `source`.

Mechanisms to achieve dynamic classification for specific alerts:
- Backend runtime mapping: compute component and layer from alert labels at request time, for example CVO alerts using `name`.
- Dynamic classification is implemented in the backend mapping logic, not via relabeling.

Notes:
- When opt-in dynamic mapping is configured on a rule, the backend can derive effective values per alert instance and populate `openshift_io_alert_component` and `openshift_io_alert_layer` for that alert.
- The backend should not add unprefixed `component` or `layer` labels to alerts, to avoid clashing with existing user labels. Use the prefixed `openshift_io_alert_component` and `openshift_io_alert_layer` fields.

## UI Alignment
- Columns for both Alerts and Alerting Rules should include `Layer` and `Component`.
- Filters should include `Layer (cluster|namespace)` and `Source (platform|user)`.
- Creation/edit flows should allow choosing `layer` from the allowed set. `component` free-form (validated).
- An admin-facing “Manage layers” section can describe the meaning of layers:
  - Cluster: control plane, cluster-wide components (API server, etcd, network, …)
  - Namespace: workloads and components scoped to a project/namespace

### Implementation Details/Notes/Constraints
- Classification is computed server-side using matchers and maintained in an in-memory cache. Users may persist overrides in namespace shards with validation.
- Alerts are enriched additively (Prometheus-compatible), correlating to relabeled rules where possible and applying source-based defaults on fallback.
- No new CRDs or aggregated API servers are introduced; standard RBAC applies.

### Topology Considerations
#### Hypershift / Hosted Control Planes
The Classification Component runs within the Hosted Cluster (Guest). It classifies alerts originating from the Hosted Cluster (User Workloads + Guest Control Plane artifacts). It does not access or classify alerts from the Management Cluster (Hypershift infrastructure).

#### Standalone Clusters

#### Single-node Deployments or MicroShift

#### OpenShift Kubernetes Engine

## Upgrade / Downgrade Strategy
- User overrides remain intact. only invalid values are annotated with `errors`.

## Test Plan (High Level)
- Unit tests:
  - Unknown component fallback for user rules → `layer=namespace`, `component=<rule ns>`.
  - Unknown component fallback for platform rules → `layer=cluster`, `component=<rule ns>`.
  - Valid overrides are merged. Invalid overrides are recorded in `errors` and ignored.
  - Signature annotation stored and updated deterministically.
- Integration/e2e (as available):
  - ConfigMap creation/update on rule changes.
  - Alerts API includes additive fields and respects relabel configs.

### Risks and Mitigations
- Misclassification by classifier: mitigated by clear overrides and validation paths.
- Drift between docs and implementation: mitigated by this enhancement and regular verification in tests.
- Client assumptions about additional `layer` values: documented allowed set and guidance to pass through unknown values without interpretation.

### Drawbacks
- Classifier rules require maintenance as platform components evolve.

## Alternatives (Not Implemented)
- Setting the labels with alertRelabelConfig CR for all alerts, except for operator alerts in User workload monitoring.
- Introduce a dedicated classification CRD (adds operational overhead with limited benefit).
- Compute classification only in the UI (duplicates logic, hard to validate).

## Graduation Criteria

### Dev Preview -> Tech Preview
- End-to-end classification (compute classification, persist, enrich) with unit tests and docs.
- UI consumes `component`/`layer` for display and filtering.

### Tech Preview -> GA
- Full test coverage (upgrade/downgrade/scale).
- Stable defaulting across supported topologies (standalone, Hypershift, SNO/MicroShift).

### Removing a deprecated feature
- If the classifier or persistence format changes, document migration and keep backward compatibility for one minor release.

## Version Skew Strategy
- Server-side enrichment ensures older/newer UIs receive consistent fields. Unknown `layer` values must be passed through and displayed as-is.

## Operational Aspects of API Extensions
- No new API extensions are introduced. OwnerReferences ensure GC of ConfigMaps. Failures surface in controller logs.

## Support Procedures
- Verify `alert-classification-overrides-<rule-namespace>` ConfigMaps and validate their entries.
- Check controller logs for validation failures.
- Confirm alert `prometheus` or `openshift_io_alert_source` labels for source detection.
## Open Questions

- Where should we store the classification override ConfigMaps?
  - Current implementation: store overrides in the monitoring plugin namespace
    (MONITORING_PLUGIN_NAMESPACE), sharded by rule namespace.
  - Option A: store the override ConfigMap in the PrometheusRule namespace it
    applies to (per-namespace storage). This requires write RBAC in each target
    namespace and may not be acceptable for a console backend plugin.
  - Option B: store overrides in a fixed "control" namespace (e.g.,
    openshift-monitoring). This reduces RBAC scope but centralizes writes into a
    privileged namespace and still requires careful authorization.

- When `component classification is unknown, what should the fallback component value be?
  - Option A: `other` for both stacks.
  - Option B: `namespace` for user workload monitoring, `other` for platform.
  - Option C: always `namespace`.

