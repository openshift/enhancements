---
title: ingress-component-route-labels
authors:
  - "@jhadvig"
  - "@Leo6Leo"
reviewers:
  - "@saschagrunert"
  - "@everettraven"
approvers:
  - "@spadgett"
  - "@everettraven"
api-approvers:
  - "@saschagrunert"
creation-date: 2026-05-28
last-updated: 2026-06-02
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/CONSOLE-5163
see-also:
  - "https://github.com/openshift/api/pull/2845"
---

# Ingress Component Route Labels

## Summary

This enhancement covers the **API change only**: adding a `labels` field to `ComponentRouteSpec` in the `Ingress` config API (`config.openshift.io/v1`). This enables cluster administrators to specify labels on component-managed routes so that IngressControllers using route selectors can manage them for route sharding scenarios. Operator-side consumption of this field is tracked separately per operator.

## Motivation

### User Stories

- As a cluster administrator running multiple IngressControllers for internal and external traffic, I want to add labels to the console route so it is served by a specific IngressController shard rather than the default.
- As a cluster administrator, I want to configure route labels for component routes through the Ingress cluster config so that labels persist across operator reconciliations without requiring manual re-application after each operator sync.

### Goals

- Allow cluster administrators to specify labels on `componentRoutes` entries in the Ingress config API.
- Labels are applied to the corresponding routes by the operators that manage them.
- Labels are validated against Kubernetes label conventions at admission time.

### Non-Goals

- This proposal covers the API change only. Operator-side consumption (reading and applying labels to routes) is tracked and implemented separately per operator.
- This proposal does not add labels to `ComponentRouteStatus`. Labels are configuration input, not status output. If an operator that supports the `labels` field encounters an error applying labels, it reports the error through its existing status conditions. If an operator does not yet support the field, the labels are silently ignored.
- This proposal does not create or manage IngressControllers. Administrators must configure IngressController route selectors separately.

## Proposal

Add an optional `labels` field of type `map[string]LabelValue` to the existing `ComponentRouteSpec` struct in `config/v1/types_ingress.go`. `LabelValue` is a string type alias with CEL validation enforcing Kubernetes label value conventions. The field is gated behind the `IngressComponentRouteLabels` FeatureGate, initially enabled in `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade`.

### Validation

- `+mapType=granular` for proper strategic merge patch behavior, allowing individual label keys to be added or removed without replacing the entire map.
- `MinProperties=1` prevents semantically empty `labels: {}`. When the field is omitted, no additional labels are applied.
- `MaxProperties=8` bounds the map size. Route sharding typically needs 1-2 labels; 8 provides a ceiling for CEL cost estimation.
- Key validation uses a CEL rule with `format.qualifiedName()` (a [Kubernetes CEL standard library function](https://kubernetes.io/docs/reference/using-api/cel/#kubernetes-cel-libraries)) to enforce label key conventions.
- Value validation is enforced by the `LabelValue` type, a new validated `string` alias defined in `config/v1/types_ingress.go`. It uses `format.labelValue()` (also a Kubernetes CEL standard library function) to enforce label value conventions.
- Keys with `kubernetes.io/` and `k8s.io/` reserved prefixes are rejected, as these are reserved for Kubernetes system use.

### Label Conflict and Removal Behavior

Labels specified in `componentRoutes` are additive. The consuming operator applies them to the route alongside any labels the operator itself manages. If an administrator specifies a label key that the operator also sets, the administrator-specified value takes precedence — the operator must treat `componentRoutes.labels` as the authoritative source for those keys. Operators must not overwrite administrator-specified labels during reconciliation.

When an administrator removes a key from the map or removes the `labels` field entirely, the operator removes those labels from the route on the next reconciliation and restores any operator-managed defaults for those keys.

### Example

```yaml
apiVersion: config.openshift.io/v1
kind: Ingress
metadata:
  name: cluster
spec:
  componentRoutes:
  - name: console-2
    namespace: openshift-console
    hostname: console.internal.corp.example.com
    labels:
      ingress: shard-console2
  - name: console-3
    namespace: openshift-console
    hostname: console.private.corp.example.com
    labels:
      ingress: shard-console3
```

### Workflow Description

**Cluster administrator** is a human user responsible for managing the cluster's ingress infrastructure, including IngressControllers and component route configuration.

**Component operator** is a cluster operator (e.g., console-operator, authentication-operator) responsible for creating and reconciling a component route.

**Starting state:** A cluster with at least two IngressControllers. One is the default; the other has a `routeSelector` configured to manage routes with specific labels (e.g., `ingress: shard-console2`).

1. The cluster administrator identifies which component route (e.g., the console) should be served by a non-default IngressController.
2. The cluster administrator edits the Ingress cluster config (`oc edit ingress.config.openshift.io cluster`) and adds labels to the desired `componentRoutes` entry.
3. The API server validates the labels against the CRD schema (CEL rules and OpenAPI constraints) and rejects the update if validation fails. No separate admission webhook is involved.
4. The component operator detects the change to the Ingress config during its next reconciliation loop.
5. The component operator applies the specified labels to the managed route, merging them with any operator-managed labels.
6. The IngressController with a matching `routeSelector` picks up the route. The route becomes accessible through that IngressController's endpoints.
7. If the administrator later changes or removes labels, the route may move to a different IngressController during the next reconciliation, potentially causing brief unavailability while DNS and router state converge.

#### Failure Cases

- If the component operator has not been updated to support the `labels` field, the labels are silently ignored. The route remains on its current IngressController. The operator's status conditions do not report an error because the field is unknown to the operator.
- If the administrator specifies labels that do not match any IngressController's `routeSelector`, the route becomes unmanaged and inaccessible. The administrator must correct the labels or adjust an IngressController's `routeSelector`.

### API Extensions

Adds one new type and one new field to `config/v1/types_ingress.go`:

```go
// LabelValue is a validated string type for Kubernetes label values,
// defined in config/v1/types_ingress.go.
// +kubebuilder:validation:MaxLength=63
// +kubebuilder:validation:XValidation:rule="!format.labelValue().validate(self).hasValue()"
type LabelValue string

type ComponentRouteSpec struct {
    // ... existing fields ...

    // +openshift:enable:FeatureGate=IngressComponentRouteLabels
    // +optional
    // +mapType=granular
    // +kubebuilder:validation:MinProperties=1
    // +kubebuilder:validation:MaxProperties=8
    Labels map[string]LabelValue `json:"labels,omitempty"`
}
```

`format.qualifiedName()` and `format.labelValue()` are [Kubernetes CEL standard library functions](https://kubernetes.io/docs/reference/using-api/cel/#kubernetes-cel-libraries) provided by the API server, not defined locally. CEL rules on the `Labels` map enforce `format.qualifiedName()` for keys and reject `kubernetes.io/` and `k8s.io/` reserved prefixes. See [openshift/api#2845](https://github.com/openshift/api/pull/2845) for full validation rules and generated manifests.

No new CRDs, webhooks, finalizers, or aggregated API servers are introduced.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No special considerations. The Ingress config is a cluster-scoped resource managed identically in Hypershift hosted clusters. The operators that reconcile component routes run in the management cluster and apply labels to routes in the hosted cluster the same way they do in standalone clusters.

#### Standalone Clusters

Primary target topology. No special considerations.

#### Single-node Deployments or MicroShift

No special considerations for single-node deployments. Route labels work the same regardless of node count.

MicroShift does not use the OpenShift Ingress config API and does not run IngressControllers with route selectors, so this feature is not applicable to MicroShift.

#### OpenShift Kubernetes Engine

This feature modifies the Ingress config API (`config.openshift.io/v1`), which is available in both OCP and OKE. Route sharding via IngressController route selectors is an existing OKE capability. This enhancement does not depend on any OCP-only components and is expected to work in OKE without restrictions.

### Implementation Details/Notes/Constraints

- The field is behind the `IngressComponentRouteLabels` FeatureGate, initially enabled in `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade`. The field only appears in the CRD schema when the FeatureGate is enabled.
- The following operators manage component routes and must be updated to read and apply labels from `ComponentRouteSpec`:
  - `console-operator` — manages the `console` and `downloads` routes in `openshift-console`
  - `authentication-operator` — manages the `oauth-openshift` route in `openshift-authentication`
- The `+mapType=granular` annotation ensures individual label keys can be added or removed via strategic merge patch without replacing the entire map.
- Operators must apply labels during their normal reconciliation loop. Labels from `componentRoutes` take precedence over any operator-managed labels with the same key.

### Risks and Mitigations

- **Risk:** Changing labels on a component route may cause it to move between IngressController shards, potentially causing brief unavailability.
  - **Mitigation:** The godoc explicitly warns about this behavior. The feature is gated behind TechPreview for initial adoption.

- **Risk:** Operators that manage component routes may not immediately support the new labels field.
  - **Mitigation:** The labels field is optional. Operators that do not read the field will simply not apply labels, maintaining current behavior.

- **Risk:** Specifying labels that do not match any IngressController's `routeSelector` causes the component route to become unmanaged and inaccessible.
  - **Mitigation:** This is an existing operational risk with route sharding. Documentation and release notes will emphasize that labels must match an IngressController's `routeSelector`. Future work may add status conditions to report when a route is not admitted by any IngressController.

- **Risk:** On downgrade to a version without the `labels` field, labels already applied to routes are not automatically removed. The route retains the labels and continues to be managed by whichever IngressController's `routeSelector` matches, which is the expected behavior. No data integrity risk exists because the labels are metadata on routes, not configuration state.

### Drawbacks

- Adding a new field to a stable API increases the surface area that must be maintained indefinitely. Once GA, the `labels` field cannot be removed without a deprecation cycle.
- The feature introduces an indirect dependency between Ingress config and IngressController configuration. Misconfigured labels may silently cause routes to become unreachable if no IngressController's `routeSelector` matches, which can be difficult to diagnose.
- Each consuming operator must be individually updated to read and apply labels. Until all operators are updated, the feature works inconsistently across components, which may confuse administrators.

## Alternatives (Not Implemented)

1. **Annotations instead of labels.** Route annotations could carry sharding hints. However, IngressController `routeSelector` operates on labels, not annotations, so annotations would require changes to the IngressController itself. Labels align with the existing route selection mechanism.

2. **Modify IngressController routeSelector to match component routes by name/namespace.** Instead of adding labels to routes, IngressControllers could be configured to select routes by name or namespace. This would avoid modifying the Ingress config API but would tightly couple IngressController configuration to specific component route names, reducing flexibility.

3. **Dedicated ComponentRouteSharding CRD.** A separate CRD could manage the mapping between component routes and IngressController shards. This would provide more flexibility but introduces unnecessary complexity for a use case that is well served by a single optional field on an existing API.

## Open Questions

1. Are there additional operators beyond `console-operator` and `authentication-operator` that manage component routes and need to be updated?
2. Should a status condition be added to report when a route's labels do not match any IngressController's `routeSelector`? This would improve diagnosability but requires changes to the ingress-operator, which is out of scope for this proposal.

## Test Plan

### Unit Tests

CRD validation tests in `config/v1/tests/ingresses.config.openshift.io/IngressComponentRouteLabels.yaml` cover:

- **Happy paths:** single label, multiple labels, DNS-prefixed keys, max-length values (63 characters), empty string values (valid per Kubernetes label spec).
- **Negative paths:** invalid keys, invalid values (starting with dash, e.g., `-starts-with-dash`), reserved prefixes (`kubernetes.io/`, `k8s.io/`), key name part over 63 characters, more than 8 labels.
- **Non-reserved prefixes:** keys with non-reserved prefixes (e.g., `openshift.io/my-label`) are accepted, confirming the reserved prefix check is not overly broad.
- **Update scenarios:** add labels, change label values, remove labels (labels present to no labels).
- **DeepCopy:** validated by the repo-wide `TestRoundTripTypesWithoutProtobuf` test.

### E2E Tests

E2E tests will be added when consuming operators are updated to apply labels. These will verify:

- Labels specified in the Ingress config appear on the managed routes.
- Changing labels on a component route causes the route to be readmitted by the correct IngressController.
- Removing labels from a component route causes the operator to remove those labels from the route on the next reconciliation.

## Graduation Criteria

### Dev Preview -> Tech Preview

- API field defined and validated.
- CRD validation tests passing.
- At least one consuming operator (console-operator) applies labels from the spec.

### Tech Preview -> GA

- All consuming operators that manage component routes apply labels.
- E2E tests confirm labels are propagated to routes.
- No critical bugs reported during TechPreview.
- User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/) with route sharding examples showing how to use `componentRoutes.labels` with IngressController `routeSelector`.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

- **Upgrade:** The field is additive and optional. Existing clusters without labels continue to work unchanged.
- **Downgrade:** The `labels` key remains in etcd (unknown fields are preserved) but is ignored by the older API server and operators. Routes retain whatever labels were last applied. On re-upgrade, the preserved values become active again.

## Version Skew Strategy

The CVO updates CRDs before operators. The `labels` field becomes available in the schema before operators are updated to read it. This is safe because the field is optional — labels are stored in the Ingress config and applied once the operator catches up. There is no window where setting labels causes an error or data loss.

## Operational Aspects of API Extensions

No new CRDs, admission webhooks, conversion webhooks, aggregated API servers, or finalizers are introduced.

### Failure Modes

- If invalid labels are specified, CRD validation rejects the update at admission time. The API server returns a validation error and the Ingress config is not modified.
- If valid labels are specified but the consuming operator does not support them, the labels are silently ignored. The route continues to function on its current IngressController. There is no degradation of existing functionality.
- If the consuming operator crashes mid-reconciliation after partially applying labels, the next reconciliation will complete the label application. Operators are expected to be idempotent.
- If labels cause a route to not match any IngressController's `routeSelector`, the route becomes unadmitted. Traffic to that component's hostname will fail until the labels are corrected or an IngressController's `routeSelector` is updated.

### Impact on Existing SLIs

- No impact on API throughput. The field adds a small amount of data to an existing resource that is infrequently updated.
- No new admission webhooks or controllers. Validation is handled by existing CRD schema validation at the API server level.
- No new metrics are introduced by this enhancement. Operators that apply labels use their existing reconciliation metrics and status conditions to report success or failure.

### Escalation

- For issues related to CRD validation or the Ingress config API: the API team (`#forum-api-review`).
- For issues related to label application on routes: the team owning the consuming operator (e.g., console team for console routes, authentication team for OAuth routes).
- For issues related to route admission by IngressControllers: the ingress team.

## Support Procedures

- To check if labels are configured:
  ```
  oc get ingress.config.openshift.io cluster -o jsonpath='{.spec.componentRoutes[*].labels}'
  ```
- To verify labels are applied to a route:
  ```
  oc get route <route-name> -n <namespace> -o jsonpath='{.metadata.labels}'
  ```
  Compare with the labels configured in the Ingress config. If they differ, the consuming operator may not support the `labels` field yet, or may not have reconciled.
- To check if a route is admitted by an IngressController:
  ```
  oc get route <route-name> -n <namespace> -o jsonpath='{.status.ingress}'
  ```
  An empty or missing `status.ingress` indicates no IngressController has admitted the route.
- To check operator status for reconciliation issues:
  ```
  oc get clusteroperator console -o jsonpath='{.status.conditions}'
  ```
- To disable the feature: remove the `IngressComponentRouteLabels` FeatureGate (only possible in TechPreview). Existing labels on routes are not automatically removed.
