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
  - "none"
creation-date: 2026-01-07
last-updated: 2026-11-19
tracking-link:
  - "https://issues.redhat.com/browse/CNV-75208"
---
# Alert Rule Classification Mapping

See the [Terminology](#terminology) section for a quick reference on the alerting concepts used in this proposal (alerting rules, alert instances, stack definitions) and the label and enrichment fields referenced throughout.

## Summary
This enhancement defines how OpenShift monitoring assigns each alerting rule (and the alerts it emits) a stable, user-visible classification: a `component` and an impact `layer`.

It documents the defaulting and fallback behavior, how users can override classification through AlertRelabelConfig (ARC) custom resources, and how the Alerts API exposes the effective values in a Prometheus compatible response for UI filtering and display.

This design is part of the [Alerts UI Management](https://github.com/openshift/enhancements/pull/1822) enhancement proposal (the link targets the open PR and will resolve to `enhancements/monitoring/alerts-ui-management.md` once that proposal is merged).

## Background
This proposal builds on existing OpenShift practices that use alert metadata for higher-level views and health computations.

### OpenShift Virtualization operator health labels
OpenShift Virtualization alerting rules already use labels to express alert ownership and alert impact. These conventions have proven useful for calculating operator health and for presenting alerts with consistent metadata.

Common labels include:

- `kubernetes_operator_part_of`: identifies the higher-level product or organization that owns the alert (for example `kubevirt`)
- `kubernetes_operator_component`: identifies the specific operator or sub-component that reports the alert (for example `hyperconverged-cluster-operator`, `kubevirt`)
- `operator_health_impact`: indicates the impact of a firing alert on the operator health calculation (values such as `critical`, `warning`, `none`)

This enhancement generalizes the same idea for all alerting rules. It defines a consistent, user-visible `(component, layer)` classification and a single override mechanism that UIs can rely on for filtering, grouping, and health rollups.

### Cluster Health Analyzer
This approach has also been accepted and implemented in `cluster-health-analyzer`. The incident detection and component mapping features classify firing alerts using alert metadata and matchers, then group related alerts into higher-level incidents. Aligning with that behavior reduces drift between alert classification used for incident detection and alert classification surfaced in the UI.

## Motivation
- Enable computation of component and cluster aggregated level health to help the UI prioritize clusters needing attention, highlight failing components, and speed troubleshooting in single and multi-cluster views.
- Provide alert with clear context, which is not possible today. Today this can only be deduced by the alert name, namespace, etc, which are not easy to sort and aggregated by to figure out what component has an issue and related alerts.

## Proposal

### User Stories
- As a multi-cluster admin, I will be able to see which clusters require my attention drill down by impacted component and scope of impact to the specific related alerts.
- As a namespace owner, I can see only the clusters with alerts that impact my namespaces, drill down to see which components are impacted, and then the specific related alerts.
- As an operator developer, if I want to set the impacted component and scope of impact for my alerts, I can do so in my alert definitions and they will take precedence over heuristics created by observability developers.
- As an observability developer, I need to create heuristics for setting impacted component and scope of impact so that every alert in the cluster has them and they are as accurate as possible.
- As a cluster admin, I will be able to update an alerting rule’s impacted component and/or scope of impact through the UI and have it override all other predefined settings for alerts emitted by that rule.
- As a cluster Admin, I want platform alerts to be treated as `cluster` impact `layer` by default, so global issues are clearly surfaced.
- As namespace owner, I want workload alerts to be treated as namespace-scoped impact `layer` by default so I can filter and troubleshoot within my project scope.

### Goals

High level goal of this feature is:
- Adding a `component` and impact `layer` mapping for each alerting rule and the alert instances it emits, so that we can group alerts by them and calculate cluster and component health.
- Let users update classification for an alerting rule when needed. This persists as an override keyed by `openshift_io_alert_rule_id` and affects alerts emitted by that rule.

1. Define allowed impact `layer` values classification rules and default behavior for user workload monitoring alert rules and alerts.
2. Define how to create a consistent mapping across all alert rules types.
3. Specify API enrichment fields for alerts and rules, and expected UI filters/columns, while maintaining compatibility with Prometheus/Thanos schemas.

### Non-Goals
- Changing upstream Prometheus/Thanos APIs or schemas.
- Redefining platform vs user source detection beyond what is documented here.
- Enforcing a specific UI, this defines the model that UIs should follow.

### Workflow Description
1) The Classification Component (console backend plugin) watches PrometheusRules and alert relabel configs, keeps an in-memory registry of relabeled alerting rule definitions and their computed classification defaults.
2) Users express intent with overrides stored as `AlertRelabelConfig` (ARC) custom resources in the appropriate monitoring namespace.
For platform rules, ARCs are stored in `openshift-monitoring`.
Classification overrides are not supported for rules in the user-defined stack: the API rejects them,
because CMO does not reconcile ARCs in `openshift-user-workload-monitoring`.
When CMO adds that support, the ARC target namespace for user-defined rules will be `openshift-user-workload-monitoring`.
The Cluster Monitoring Operator (CMO) reconciles ARCs into the Prometheus relabel pipeline,
so overrides take effect as relabel configs applied by Prometheus.
3) The API serves the effective classification to the Console (relabeled rules merged with classifier heuristics and source-based defaults).
4) Defaults are not persisted. Only user overrides are persisted as ARC CRs. When the override matches the original value (no effective change), the ARC is deleted.
5) The UI displays and filters by the effective classification.

### API Extensions
- The implementation uses the existing `AlertRelabelConfig` (ARC) CRD from `monitoring.openshift.io/v1` for persisting classification overrides. No new CRDs are introduced.
- The console backend exposes/extends alerting APIs used by the UI:
  - GET `/api/v1/alerting/alerts`: Prometheus-compatible response with additive enrichment JSON fields per alert instance: `alertRuleId`, `alertComponent`, `alertLayer`. These are top-level fields on the alert object, separate from `labels`.
  - GET `/api/v1/alerting/rules`: read path that surfaces a unified, post-relabel view of rules and their effective classification.
    Rule labels include the relabeled classification labels (`openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`)
    and provenance labels (`openshift_io_alert_rule_id`, `openshift_io_prometheus_rule_namespace`, `openshift_io_prometheus_rule_name`,
    `openshift_io_alerting_rule_name` when the rule is owned by an AlertingRule CR, and `openshift_io_rule_managed_by` /
    `openshift_io_relabel_config_managed_by` for GitOps- or operator-managed rules).
    Rules with the same `openshift_io_alert_rule_id` across groups are deduplicated.
    Rules disabled by relabel configs (Drop actions) remain discoverable so the UI can offer restore:
    every alerting rule in the response carries an `alertingRuleEnabled` boolean that reports whether
    the current relabel configuration retains the rule's labels (including `alertname`).
    Rules targeted by Drop actions are returned with `alertingRuleEnabled: false`
    (Prometheus keeps evaluating the rule; its alert instances are suppressed from the alerts response),
    and the UI restores them with PATCH `alertingRuleEnabled: true`.
    Note: alerts nested within rules in this endpoint do **not** carry the additive enrichment fields
    (`alertRuleId`, `alertComponent`, etc.) — classification is conveyed through the parent rule's `labels` map.
    Alert labels within rules do have relabel configs applied.

  - PATCH `/api/v1/alerting/rules/{ruleId}`: Update an alert rule. Supports label updates, classification overrides, and drop/restore (enable/disable).
    - Input: JSON object with optional fields:
      - `labels`: map of label key to value. A `null` or empty-string value removes the label. Omitted keys are left unchanged.
      - `classification`: nested object with three-state semantics per field (omitted = no change, `null` = clear override, string = set override):
        - `openshift_io_alert_rule_component`
        - `openshift_io_alert_rule_layer`
        - `openshift_io_alert_rule_component_from`
        - `openshift_io_alert_rule_layer_from`
      - `alertingRuleEnabled`: optional boolean for drop/restore. Cannot be combined with `labels` or `classification` in the same request.
    - Behavior: resolve `{ruleId}` to its PrometheusRule using the in-memory relabeled rules registry.
      Only platform rules are supported: the ARC is created or updated in `openshift-monitoring`
      (classification updates for user-defined rules are rejected; see user-workload limitations below).
      Create or update an ARC named `arc-<sanitized-pr-name>-<short-hash>` containing relabel configs
      that match the rule by its original labels and apply label overrides keyed by the rule ID.
      Updates are rejected for GitOps-managed rules (detected via owner references and managed-by labels).
    - Reset: setting a classification field to `null` removes that override label. If all overrides are cleared and the effective labels match the originals, the ARC is deleted.
    - Scope: classification updates create ARCs rather than modifying the PrometheusRule directly.
  - PATCH `/api/v1/alerting/rules`: Bulk update. Accepts `ruleIds` array and applies the same classification/label/toggle logic per rule.
    - Response: `{ "rules": [ { "id": "...", "statusCode": 204 }, ... ] }` with per-rule status codes.
  - DELETE `/api/v1/alerting/rules/{ruleId}`: Delete a single alert rule by ID.
  - DELETE `/api/v1/alerting/rules`: Bulk delete user-defined alert rules.

Example PATCH payload:

```json
{
  "classification": {
    "openshift_io_alert_rule_component": "kube-apiserver",
    "openshift_io_alert_rule_layer": "cluster"
  }
}
```

Example PATCH payload with dynamic mapping:

```json
{
  "classification": {
    "openshift_io_alert_rule_component_from": "name",
    "openshift_io_alert_rule_layer": "cluster"
  }
}
```

Example PATCH response (single rule):

```json
{
  "id": "rid_abc123...",
  "statusCode": 204
}
```

Example alerts response enrichment:

```json
{
  "labels": {
    "alertname": "ClusterOperatorDown",
    "openshift_io_alert_rule_id": "rid_abc123...",
    "openshift_io_alert_source": "platform",
    "name": "kube-apiserver"
  },
  "state": "firing",
  "alertRuleId": "rid_abc123...",
  "alertComponent": "kube-apiserver",
  "alertLayer": "cluster"
}
```

RBAC (high level):
- Platform stack: classification overrides create ARCs in `openshift-monitoring`. Only users with write access to ARCs in that namespace can update classification for platform rules.
- User workload monitoring: classification overrides are not supported. CMO does not reconcile ARCs in the `openshift-user-workload-monitoring` namespace, so the API rejects classification updates for user-defined rules until CMO adds support for it.

## Terminology

### Concepts

- **Alerting rule**: An alert definition. It includes a PromQL expression and an optional `for` duration and labels and annotations. The rule triggers alerts when the expression returns data. It is defined within a group of a `PrometheusRule` resource which is evaluated by Prometheus or Thanos Ruler. For additional details, see [Alerting rules](https://prometheus.io/docs/prometheus/latest/configuration/alerting_rules/).
- **Alerts**: Runtime instances generated by an alerting rule when its condition is met.
- **Platform monitoring stack**: The cluster monitoring stack in the `openshift-monitoring` namespace, managed by the Cluster Monitoring operator. It is meant to collect metrics and evaluate rules from Red Hat certified operators. For additional details, see [Monitoring stack architecture](https://docs.redhat.com/en/documentation/monitoring_stack_for_red_hat_openshift/4.20/html/about_monitoring/monitoring-stack-architecture#understanding-the-monitoring-stack_monitoring-stack-architecture).
- **User-defined monitoring stack**: The user monitoring stack for user workloads. It is meant to collect metrics and evaluate rules from applications in user namespaces. For additional details, see [Monitoring stack architecture](https://docs.redhat.com/en/documentation/monitoring_stack_for_red_hat_openshift/4.20/html/about_monitoring/monitoring-stack-architecture#understanding-the-monitoring-stack_monitoring-stack-architecture).

#### Labels and enrichment fields

- **rule id**: `openshift_io_alert_rule_id`, stable identifier for correlating alerting rules and the alerts they emit.
  The ID is computed as `rid_<base64url(SHA256(canonical_payload))>` where the canonical payload is derived
  from the rule's identity (kind, name) and spec (expr, for, business labels).
  Annotations and `openshift_io_*` system labels are excluded from the hash.
  For admin-authored rules, the backend persists the computed ID as a label on the relabeled rule
  via relabel configs and treats it as the primary correlation key.
- **source**: `openshift_io_alert_source`, source label for the rule or alert. Allowed values: `platform`, `user`. Determined by whether the PrometheusRule's namespace has the `openshift.io/cluster-monitoring=true` label.
- **component**: `openshift_io_alert_rule_component`, rule-scoped default component for the rule. The component represents the impacted service, subsystem, or resource area (e.g., `kube-apiserver`, `etcd`, `compute`, a team). The Alerts API returns the effective value for each alert instance as `alertComponent` (an additive JSON field, not a label).
- **layer**: `openshift_io_alert_rule_layer`, rule-scoped default layer for the rule. Allowed values: `cluster`, `namespace`. The Alerts API returns the effective value for each alert instance as `alertLayer` (an additive JSON field, not a label).
- **component_from**: `openshift_io_alert_rule_component_from`, optional label on a rule. When present, the backend derives the effective alert-instance component from the alert label named by this value at request time (e.g., `component_from=name` → component = alert's `name` label).
- **layer_from**: `openshift_io_alert_rule_layer_from`, optional label on a rule. When present, the backend derives the effective alert-instance layer from the alert label named by this value at request time.
- **classification_managed_by**: `openshift_io_alert_rule_classification_managed_by`, provenance label set to `monitoring-plugin` on rules whose classification was set via the API.
- For admin-authored alerting rules (platform and user-defined stacks), the rule definition should include `openshift_io_alert_rule_component` and `openshift_io_alert_rule_layer` labels when possible. This encodes the intended defaults directly on the rule and reduces reliance on matcher heuristics and post-hoc overrides.

## Mapping Logic
### Primary Mapping (Classifier)
- The backend uses matchers to compute a `(layer, component)` tuple from rule/alert labels.
- We try to stay aligned with the matchers and behavior implemented by `cluster-health-analyzer` that already uses the `layer`, `component` for `incident detection` feature.
  - The matcher approach should remain aligned. The layer values used by `cluster-health-analyzer` are not the same as the impact layer values in this proposal.
- Typical mappings:
  - Core control-plane components → `layer=cluster`, `component=<cp-subsystem>`
  - Node/compute-related → `layer=cluster` and `component=compute`.
  - Namespace-level alerts → `layer=namespace`

#### How classifier heuristics are authored
- The classifier is implemented as an **ordered list of matcher functions** in `pkg/alertcomponent/matcher.go`. **First match wins** and determines the effective `(layer, component)`.
- The matcher evaluation order is:
  1. **CVO alerts**: `ClusterOperatorDown`, `ClusterOperatorDegraded` → `layer=cluster`, `component=<labels.name>` (or `version` if `name` is absent). This enables per-alert-instance component mapping.
  2. **KubeVirt operator**: Matches alerts with `kubernetes_operator_part_of=kubevirt`, excluding `kubernetes_operator_component=cnv-observability` (the CNV observability stack is classified by its own matchers). Sub-classifies based on `kubernetes_operator_component` and `operator_health_impact`:
     - VM-scoped alerts (`operator_health_impact=none`, `kubernetes_operator_component=kubevirt`) → `layer=namespace`, `component=openshift-virtualization-virtual-machine`
     - Operator-scoped alerts → `layer=cluster`, `component=openshift-virtualization-operator`
  3. **Compute (node) alerts**: A curated list of node-related alert names (e.g., `KubeNodeNotReady`, `NodeFilesystemSpaceFillingUp`, `NodeMemHigh`, `MCDRebootError`, etc.) → `layer=cluster`, `component=compute`.
  4. **Core component matchers**: Namespace-based and alert-name-based matchers for well-known platform components. All map to `layer=cluster`. Examples:
     - `etcd` (namespaces: `openshift-etcd`, `openshift-etcd-operator`)
     - `kube-apiserver` (namespaces: `openshift-kube-apiserver`, `openshift-kube-apiserver-operator`)
     - `machine-config` (namespace + specific alert names like `HighOverallControlPlaneMemory`)
     - `version` (namespaces: `openshift-cluster-version`, `openshift-version-operator`, plus alert names `ClusterNotUpgradeable`, `UpdateAvailable`)
     - And ~20 more platform components (dns, authentication, ingress, monitoring, network, storage, etc.)
  5. **Workload matchers**: Namespace-based and label-based matchers for well-known workload areas. All map to `layer=namespace`. Examples:
     - `openshift-compliance`, `openshift-logging`, `openshift-gitops`
     - `quay` (matched by `container` label values)
     - `Argo` (matched by regex on alertname `^Argo`)
  6. **Fallback**: when nothing matches → `component=others`, layer derived from source.
- Adding or updating heuristics is a code change in the monitoring plugin (`pkg/alertcomponent/matcher.go`), and should remain aligned with `cluster-health-analyzer` to avoid drift between backend and UI behavior.

Rule scoped default classification labels:
- If an alerting rule includes `openshift_io_alert_rule_component` and or `openshift_io_alert_rule_layer`, the backend uses those values as the default `component` and `layer` for alerts emitted by that rule, unless a user override replaces them.

### Precedence and matching semantics
The backend computes the effective classification using this order, highest priority first:
1) Explicit user overrides persisted as ARCs and set through the PATCH API, recognizable by the `openshift_io_alert_rule_classification_managed_by=monitoring-plugin` provenance label.
   A static override (`openshift_io_alert_rule_component` / `openshift_io_alert_rule_layer`) wins over every other mechanism for the field it sets: dynamic `_from` resolution (step 2) and built-in per-instance enrichment (step 3) are skipped for that field, so an admin override always takes effect.
   For example, a static component override on `ClusterOperatorDown` is not replaced by the CVO `labels.name` enrichment.
   A user-set `_from` label is itself an explicit override and resolves that field dynamically from the chosen alert label.
2) Dynamic `_from` labels on the relabeled rule (`openshift_io_alert_rule_component_from`, `openshift_io_alert_rule_layer_from`): these reference an alert label whose runtime value becomes the classification. Takes precedence over static classification.
3) CVO-style per-alert-instance classification: for known alert families (e.g., `ClusterOperatorDown`, `ClusterOperatorDegraded`), the backend computes component from alert labels (e.g., `labels.name`) at request time, overriding any static rule classification. Skipped for fields covered by an explicit user override.
4) Rule-scoped default labels on the relabeled rule: `openshift_io_alert_rule_component`, `openshift_io_alert_rule_layer`. These may reflect either the original rule labels or user overrides applied via ARC relabeling.
5) Classifier-based mapping (matchers): heuristic matching against rule/alert labels.
6) Fallback: `component=others`, derive `layer` from source (platform → `cluster`, user → `namespace`).

Note: user overrides stored as ARCs are applied during Prometheus relabeling (step 4), so they appear as labels on the relabeled rule and take precedence over classifier heuristics (step 5). The `openshift_io_alert_rule_classification_managed_by` provenance label distinguishes them from rule-authored labels and is what gives them priority over `_from` labels and CVO enrichment (step 1).

### Dynamic mapping configuration (`*_from`) — Implemented
- If `openshift_io_alert_rule_component_from` is present on the relabeled rule (set via ARC override or directly on the rule), the backend derives the effective alert-instance component from the specified alert label key at request time. The value must be a valid Prometheus label name (`^[A-Za-z_][A-Za-z0-9_]*$`). The resolved value must pass component validation.
- If `openshift_io_alert_rule_layer_from` is present on the relabeled rule, the backend derives the effective alert-instance layer from the specified alert label key at request time. The resolved value must be one of `cluster|namespace`.
- `_from` takes precedence over static classification labels authored on the same rule. An explicit static user override set through the PATCH API is higher precedence than `_from` and suppresses it for that field (see [precedence](#precedence-and-matching-semantics)).
- Invalid `_from` values are ignored and the static classification is used.
- Users can set `_from` labels via the PATCH API classification payload alongside static component/layer values.

### Future Work

#### Per-alert-instance overrides (`overridesByMatch`)
- If `overridesByMatch` is present in the override entry, the backend may override the effective alert instance component and layer for specific alert instances emitted by the same rule.
- Matching semantics:
  - Exact match only, `match` is a map of label key to value.
  - AND across keys, all pairs must be present on the alert.
  - First match wins, evaluate entries in order.

#### Correlation-based enrichment (Korrel8r)
In the future, we may be able to enhance classification by using signal-correlation tooling such as `korrel8r` to associate an alert with related Kubernetes resources and context (for example, the owning workload or operator). This could improve accuracy for alerts that do not carry enough information for reliable classification using static matchers alone.

This would be an **optional enhancement** and should not replace deterministic matcher-based behavior in the baseline design. Any such integration would need to consider performance, caching, and availability (classification must remain fast and predictable even when correlation is unavailable).

### Alert-specific dynamic classification (examples: CVO alerts)
Some alert families cannot be classified purely from the alerting rule definition because the component is derived from runtime alert labels that vary per alert instance. A common example is Cluster Version Operator alerts where the component is derived from the alert label `name` which identifies the ClusterOperator.

For these alerts, the backend computes component and layer per alert instance using alert labels, overriding any static rule classification. An explicit user override set through the PATCH API takes precedence over this built-in enrichment for the fields it sets (see [precedence](#precedence-and-matching-semantics)).
This enrichment is implemented in both paths: the classifier matchers (rule-level) and the alerts enrichment path (alert-level).

Implementation:
- The classifier matchers evaluate CVO alerts first (highest priority). At the rule level, this matches by alertname.
- At the alert enrichment level, `classifyCvoAlert` checks each alert: if `alertname` is `ClusterOperatorDown` or `ClusterOperatorDegraded`, it sets `layer = cluster` and `component = <labels.name>` (or `version` if `name` is absent). This runs after rule-based classification and overrides it.
- Additionally, the generic `_from` mechanism can achieve similar behavior for other alert families: setting `openshift_io_alert_rule_component_from=name` on a rule causes the backend to derive component from the alert's `name` label at request time.

This matches the cluster-health-analyzer approach and enables dynamic per-alert component mapping without requiring users to split rules.

### Fallback Mapping (When component is unknown)
If the classifier returns an empty component or `others`:
- `component = others`
- `layer` is derived from source using `deriveLayerFromSource`:
  - `openshift_io_alert_source=platform` → `cluster`
  - `openshift_io_prometheus_rule_namespace=openshift-monitoring` → `cluster`
  - `prometheus` label prefixed with `openshift-monitoring/` → `cluster`
  - Otherwise → `namespace`

Notes:
- Generated layer values are always one of `cluster|namespace`.
- Fallback is applied both in rule-level classification (`classifyFromRule`) and alert-level classification (`classifyFromAlertLabels`).


### Source Determination
- Platform detection is based on the `openshift.io/cluster-monitoring=true` namespace label, not a fixed list. The backend watches namespaces via an informer and maintains a dynamic set of cluster-monitoring namespaces.
- For rules: a rule is considered `platform` when its PrometheusRule resides in a namespace with the `openshift.io/cluster-monitoring=true` label. The Alerts API exposes this as `openshift_io_alert_source=platform`.
- For alerts: the backend correlates the alert to its base rule (via subset label matching or rule ID lookup) and derives source from the rule's PrometheusRule namespace. When correlation fails, the backend falls back to:
  - `openshift_io_alert_source == platform` label on the alert, or
  - `openshift_io_prometheus_rule_namespace == openshift-monitoring`, or
  - `prometheus` label prefixed with `openshift-monitoring/`.
  Otherwise `user`.

## Persistence and Overrides
### In-memory cache (relabeled rules registry)
- The Classification Component maintains an in-memory cache of relabeled rules via the `relabeledRulesManager`:
  - Watches all `PrometheusRule` CRs via an informer.
  - Watches the `alert-relabel-configs` Secret in `openshift-monitoring` for relabel configs (the reconciled output of all ARCs).
  - Applies relabel configs to rules from cluster-monitoring namespaces, producing a post-relabel view of each rule with enriched labels (`openshift_io_alert_rule_id`, `openshift_io_prometheus_rule_namespace`, `openshift_io_prometheus_rule_name`, and any classification overrides).
  - Stores relabeled rules keyed by `openshift_io_alert_rule_id`. Duplicate IDs are detected and logged; the later rule is skipped.
  - Resyncs every 15 minutes with exponential backoff on errors.

### On demand user overrides (AlertRelabelConfig CRs)
- Users create overrides through the PATCH API. The backend persists them as `AlertRelabelConfig` (ARC) custom resources.
- ARC namespace:
  - Platform rules: `openshift-monitoring`
  - User-defined rules: not supported. The API rejects classification updates for user-defined rules because CMO does not reconcile ARCs in `openshift-user-workload-monitoring`. When CMO adds support, the target namespace will be `openshift-user-workload-monitoring`.
- ARC naming: `arc-<sanitized-prometheusrule-name>-<short-hash-of-rule-id>` (DNS-safe, deterministic).
- Each ARC is scoped to a single rule ID and shared across all label changes (classification, severity, etc.) for that rule.
- ARC metadata:
  - Labels: `monitoring.openshift.io/prometheusrule-name`, `monitoring.openshift.io/alertname`
  - Annotations: `monitoring.openshift.io/alertRuleId`

ARC structure:
- The ARC contains an ordered list of `RelabelConfig` entries:
  1. A matcher rule that identifies the target alert by its original labels and assigns the `openshift_io_alert_rule_id` label. The matcher uses `alertname` plus **all** of the rule's business labels
     (excluding the `namespace` label and `openshift_io_*` system labels), sorted alphabetically into an anchored regex, so the override is scoped to the target rule rather than to every rule sharing the alert name.
  2. One `Replace` action per label override, keyed by `openshift_io_alert_rule_id`, setting the target label to the desired value. Relabel matchers operate on alert labels only, so `expr` and `for` cannot participate in the matcher:
     two rules that share alertname and business labels but differ only in expression cannot be distinguished by relabeling.
  3. Optionally, preserved `Drop` actions for disabled rules.
- Ambiguity rejection: before creating or updating an ARC, the backend verifies that the matcher (alertname + business labels) uniquely identifies the target rule among platform rules in cluster-monitoring namespaces.
  If another rule produces alerts with the same label set, the PATCH request is rejected with a validation error and the user is directed to differentiate the rules first (for example, by adding a distinguishing business label), so an override never silently applies to multiple rules.
- The Cluster Monitoring Operator (CMO) reconciles ARCs into the Prometheus relabel pipeline. The plugin reads the reconciled output from the `alert-relabel-configs` Secret to build the relabeled rules cache.

Concurrency:
- The backend reads and updates ARC CRs using standard Kubernetes optimistic concurrency (resource version on the ARC object).

Example ARC:
```yaml
apiVersion: monitoring.openshift.io/v1
kind: AlertRelabelConfig
metadata:
  name: arc-cluster-version-a1b2c3d4e5f6
  namespace: openshift-monitoring
  labels:
    monitoring.openshift.io/prometheusrule-name: cluster-version
    monitoring.openshift.io/alertname: ClusterOperatorDown
  annotations:
    monitoring.openshift.io/alertRuleId: "rid_abc123..."
spec:
  configs:
    # Matcher: alertname plus all business labels of the target rule
    # (namespace and openshift_io_* labels are excluded), anchored.
    - sourceLabels: [alertname, severity]
      regex: "^ClusterOperatorDown;critical$"
      targetLabel: openshift_io_alert_rule_id
      replacement: "rid_abc123..."
      action: Replace
    - sourceLabels: [openshift_io_alert_rule_id]
      regex: "rid_abc123\\.\\.\\."
      targetLabel: openshift_io_alert_rule_component
      replacement: kube-apiserver
      action: Replace
    - sourceLabels: [openshift_io_alert_rule_id]
      regex: "rid_abc123\\.\\.\\."
      targetLabel: openshift_io_alert_rule_layer
      replacement: cluster
      action: Replace
    - sourceLabels: [openshift_io_alert_rule_id]
      regex: "rid_abc123\\.\\.\\."
      targetLabel: openshift_io_alert_rule_classification_managed_by
      replacement: monitoring-plugin
      action: Replace
```

### User Overrides
- Users override classification through the PATCH API, which creates or updates ARC CRs.
- Validation:
  - `openshift_io_alert_rule_component`: non-empty, 1–253 chars, `^[A-Za-z0-9]([A-Za-z0-9_.-]*[A-Za-z0-9])?$`, must start/end alphanumeric.
  - `openshift_io_alert_rule_layer`: one of `cluster|namespace` (case-insensitive).
  - `openshift_io_alert_rule_component_from` and `openshift_io_alert_rule_layer_from`: must be a valid Prometheus label name (`^[A-Za-z_][A-Za-z0-9_]*$`).
- Invalid values are rejected by the API with a validation error.
- Reset: setting a field to `null` in the PATCH payload clears that override. If all overrides are cleared (effective labels match originals), the ARC is deleted.

### Rule id computation and stability
- The rule ID is computed as `rid_<base64url(SHA256(canonical_payload))>` where:
  - Canonical payload = `kind \n---\n name \n---\n expr \n---\n for \n---\n labels_block`
  - `kind`: `alert` or `record`
  - `name`: the alert or record name
  - `expr`: whitespace-normalized PromQL expression
  - `for`: trimmed duration string (empty if unset)
  - `labels_block`: business labels (excluding `openshift_io_*` and `alertname`) sorted alphabetically, joined as `key=value\n`. Empty values and invalid label names are dropped.
- Annotations are excluded from the hash, so annotation-only changes do not alter the rule ID.
- The computed ID is persisted as the `openshift_io_alert_rule_id` label on the relabeled rule (via relabel configs in the `alert-relabel-configs` Secret).

### Rule id changes and existing overrides
- Overrides (ARCs) reference the rule ID via their relabel config matcher and annotation.
- Changes to rule spec fields included in the hash (name, expr, for, business labels) result in a new computed ID. Existing ARC overrides keyed by the old ID will not match the new rule.
- Annotation-only edits and `openshift_io_*` label changes do not alter the rule ID.
- A copied rule computes a new ID (assuming at least one hash input differs). If the copy is byte-identical, it produces the same ID; the relabeled rules cache detects this and logs a warning.
- Stale-override gating: the effective classification view is keyed on the **current** computed rule ID, not on the alert labels an ARC originally matched. On every relabel sync the backend recomputes each rule's ID from its current spec,
  and an ARC's `Replace` entries are honored only when the ARC's recorded rule ID equals that current ID.
- As a result, when a rule is edited out of band (direct PrometheusRule edits) and its ID changes, the stale ARC immediately stops applying its replacement in the API view instead of continuing to apply until cleanup.
  Alerts may still carry the stale `openshift_io_alert_rule_id` label through the Prometheus relabel pipeline until the ARC is deleted; API-side correlation and classification are unaffected because they use the authoritative computed IDs.
- Orphaned ARCs are garbage-collected: when a relabel sync no longer finds a rule whose current computed ID matches an ARC's recorded ID, the ARC is annotated with `monitoring.openshift.io/orphaned-since` and deleted after a grace period (one resync interval, 15 minutes).
  GitOps-managed ARCs are never deleted automatically; they remain excluded from the effective view once stale and their cleanup is left to the user.

### Rule id uniqueness and collisions
- The rule id must be unique across all rules returned by the Alerting API. If two different rules share the same `openshift_io_alert_rule_id`, overrides and UI actions become ambiguous.
- Collision prevention:
  - When creating a new rule using the Alerting API, generate a new id and reject create requests that specify an id already used by a different rule.
  - For GitOps-managed rules, the backend should not mutate the rule id label. If duplicates are detected, surface a warning and require users to fix the manifests in Git by regenerating the id on the copied rule.
- Collision handling:
  - If duplicates are detected at runtime, the backend should treat those rules as conflicting and not apply user overrides keyed by that id, to avoid accidental cross-application.

## Alerts API Enrichment
- The GET `/api/v1/alerting/alerts` endpoint returns Prometheus-compatible alert objects with additive JSON fields (not additional labels):
  - `alertRuleId`: the effective `openshift_io_alert_rule_id`
  - `alertComponent`: the effective component for this alert instance
  - `alertLayer`: the effective layer for this alert instance
- The PrometheusRule name and namespace that define the alert are not repeated as alert fields; they are available on the parent rule's labels (`openshift_io_prometheus_rule_name`, `openshift_io_prometheus_rule_namespace`) via the rules endpoint and the rule ID correlation.
- The `openshift_io_alert_rule_id` and `openshift_io_alert_source` are also added to the alert `labels` map for filtering.
- Classification for alerts is computed by:
  1. Correlating the alert to its base relabeled rule via subset label matching (preferring rules with more matching labels).
  2. If correlation succeeds, classifying from the relabeled rule labels (which include any ARC overrides).
  3. Applying CVO-style per-instance classification (overrides static classification for known alert families, unless an explicit user override covers the field).
  4. Applying dynamic `_from` classification (rule labels point to alert labels for runtime resolution), unless an explicit static user override covers the field.
  5. When correlation fails, classifying directly from alert labels using the classifier matchers and fallback.
- For platform alerts (non-user-source), relabel configs from the `alert-relabel-configs` Secret are applied to alert labels before classification.

Notes:
- The backend adds enrichment as dedicated JSON fields (`alertComponent`, `alertLayer`) rather than adding unprefixed `component` or `layer` labels, to avoid clashing with existing user labels.

## UI Alignment
- Columns for both Alerts and Alerting Rules should include `Layer` and `Component`.
- Filters should include `Layer (cluster|namespace)` and `Source (platform|user)`.
- Creation/edit flows should allow choosing `layer` from the allowed set. `component` free-form (validated).
- An admin-facing “Manage layers” section can describe the meaning of layers:
  - Cluster: control plane, cluster-wide components (API server, etcd, network, …)
  - Namespace: workloads and components scoped to a project/namespace

### Implementation Details/Notes/Constraints
- Classification is computed server-side using matchers and maintained in an in-memory relabeled rules cache. User overrides are persisted as AlertRelabelConfig (ARC) CRs, which the Cluster Monitoring Operator reconciles into the Prometheus relabel pipeline.
- Alerts are enriched additively (Prometheus-compatible), correlating to relabeled rules where possible and applying source-based defaults on fallback. Enrichment fields are additive JSON fields (`alertRuleId`, `alertComponent`, `alertLayer`), not additional labels (except for `openshift_io_alert_rule_id` and `openshift_io_alert_source` which are added to labels for filtering).
- The implementation uses the existing `AlertRelabelConfig` CRD from `monitoring.openshift.io/v1`. No new CRDs or aggregated API servers are introduced. Standard RBAC applies.
- Alert-to-rule correlation uses subset label matching with preference for the most specific match (rule with most matching labels). When subset matching fails, the backend falls back to rule ID lookup in the relabeled rules cache.
- Rules with the same `openshift_io_alert_rule_id` across Prometheus groups are deduplicated in the rules response. Duplicate IDs in the relabeled rules cache are detected and warned.

### Topology Considerations
#### Hypershift / Hosted Control Planes
The Classification Component runs within the Hosted Cluster (Guest). It classifies alerts originating from the Hosted Cluster (User Workloads + Guest Control Plane artifacts). It does not access or classify alerts from the Management Cluster (Hypershift infrastructure).

#### Standalone Clusters

#### Single-node Deployments or MicroShift

#### OpenShift Kubernetes Engine

## Upgrade / Downgrade Strategy
- User overrides stored as ARC CRs remain intact across upgrades. ARCs are standard Kubernetes resources and survive cluster upgrades.
- On downgrade, ARC CRs remain in the cluster but the monitoring plugin may not process them if the classification feature is not present in the older version. The ARCs do not cause harm; CMO continues to reconcile them into relabel configs.

## Test Plan
- Unit tests:
  - Rule ID computation: deterministic across runs, stable under annotation changes, changes when spec fields change.
  - Classifier matchers: CVO alerts, KubeVirt operator, compute/node alerts, core components, workload components, and fallback behavior.
  - Unknown component fallback for user rules → `layer=namespace`, `component=others`.
  - Unknown component fallback for platform rules → `layer=cluster`, `component=others`.
  - Source determination: platform vs user based on namespace `openshift.io/cluster-monitoring=true` label.
  - Rule-scoped default labels override classifier results.
  - Dynamic `_from` classification: component and layer derived from alert labels at request time.
  - CVO alert per-instance classification: component from `name` label, fallback to `version`.
  - Validation: component regex, layer allowed values, `_from` Prometheus label name format.
  - ARC creation, update, and deletion for classification overrides.
  - Three-state PATCH semantics: omitted (no change), null (clear), string (set).
  - User override precedence: a static override set via the PATCH API beats `_from` resolution and CVO per-instance enrichment for the field it sets.
  - ARC matcher ambiguity: override requests are rejected when two platform rules share alertname and business labels.
  - Stale ARC gating: ARC relabel entries are ignored when the ARC's recorded rule ID no longer matches the rule's current computed ID.
  - Alert-to-rule correlation via subset matching with most-specific-match preference.
  - Rule deduplication by `openshift_io_alert_rule_id`.
- Integration/e2e (as available):
  - ARC creation/update on classification override via PATCH API.
  - Alerts API includes additive enrichment fields (`alertRuleId`, `alertComponent`, `alertLayer`) and respects relabel configs.
  - Rules API returns relabeled labels including classification and provenance.
  - Bulk update propagates classification overrides to multiple rules.

### Risks and Mitigations
- Misclassification by classifier: mitigated by clear overrides and validation paths.
- Drift between docs and implementation: mitigated by this enhancement and regular verification in tests.
- Client assumptions about additional `layer` values: documented allowed set and guidance to pass through unknown values without interpretation.

### Drawbacks
- Classifier rules require maintenance as platform components evolve.

## Alternatives (Not Implemented)
- **ConfigMap-based overrides** sharded by rule namespace in the plugin namespace: initially proposed but replaced with ARC-based persistence. ARCs leverage the existing CMO reconciliation pipeline and integrate directly with the Prometheus relabel mechanism.
- **Dedicated classification CRD**: adds operational overhead with limited benefit over using the existing ARC CRD.
- **Compute classification only in the UI**: duplicates logic, hard to validate, and does not support multi-consumer scenarios.

## Graduation Criteria

### Dev Preview -> Tech Preview
- End-to-end classification (compute classification via matchers, persist overrides via ARCs, enrich alerts and rules API) with unit tests and docs.
- Dynamic `_from` classification functional for per-alert-instance component mapping.
- UI consumes `alertComponent`/`alertLayer` for display and filtering.

### Tech Preview -> GA
- Full test coverage (upgrade/downgrade/scale).
- Stable defaulting across supported topologies (standalone, Hypershift, SNO/MicroShift).

### Removing a deprecated feature
- If the classifier or persistence format changes, document migration and keep backward compatibility for one minor release.

## Version Skew Strategy
- Server-side enrichment ensures older/newer UIs receive consistent fields. Unknown `layer` values must be passed through and displayed as-is.

## Operational Aspects of API Extensions
- No new API extensions are introduced beyond usage of the existing `AlertRelabelConfig` CRD. ARC CRs are standard Kubernetes resources managed through the OpenShift monitoring API. The CMO reconciles ARCs into the Prometheus configuration.

## Support Procedures
- Verify `AlertRelabelConfig` CRs in `openshift-monitoring`. (ARCs in `openshift-user-workload-monitoring` are not supported today.) Use labels (`monitoring.openshift.io/prometheusrule-name`, `monitoring.openshift.io/alertname`) and annotations (`monitoring.openshift.io/alertRuleId`) to identify which rule an ARC applies to.
- Check the `alert-relabel-configs` Secret in `openshift-monitoring` to see the reconciled relabel configs.
- Check plugin logs for classification, relabeled rules sync, and ARC errors.
- Confirm alert `openshift_io_alert_source` labels for source detection. Source is derived from the PrometheusRule namespace's `openshift.io/cluster-monitoring=true` label.

## Resolved Questions

- **Where should we store classification overrides?**
  - Resolved: overrides are stored as `AlertRelabelConfig` (ARC) CRs in the monitoring namespaces (`openshift-monitoring` for platform). For user-defined rules, ARCs are not supported: CMO does not reconcile ARCs in `openshift-user-workload-monitoring`, so the API rejects classification updates for user-defined rules until CMO adds support. This leverages the existing ARC CRD and CMO reconciliation pipeline, avoiding custom ConfigMap sharding.

- **When component classification is unknown, what should the fallback component value be?**
  - Resolved: `others` for both stacks. Layer is derived from source (`platform` → `cluster`, `user` → `namespace`).

