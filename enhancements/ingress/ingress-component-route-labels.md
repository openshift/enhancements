---
title: ingress-component-route-labels
authors:
  - "@jhadvig"
  - "@Leo6Leo"
reviewers:
  - "@saschagrunert"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-05-28
last-updated: 2026-05-28
tracking-link:
  - https://redhat.atlassian.net/browse/CONSOLE-5163
---

# Ingress Component Route Labels

## Summary

Add a `labels` field to `ComponentRouteSpec` in the `Ingress` config API (`config.openshift.io/v1`). This enables cluster administrators to specify labels on component-managed routes (e.g., console, OAuth) so that IngressControllers using route selectors can manage them for route sharding scenarios.

## Motivation

### User Stories

- As a cluster administrator running multiple IngressControllers for internal and external traffic, I want to add labels to the console route so it is served by a specific IngressController shard rather than the default.
- As a cluster administrator, I want to configure route labels for component routes through the Ingress cluster config rather than manually patching routes, which would be overwritten by the managing operator.

### Goals

- Allow cluster administrators to specify labels on `componentRoutes` entries in the Ingress config API.
- Labels are applied to the corresponding routes by the operators that manage them.
- Labels are validated against Kubernetes label conventions at admission time.

### Non-Goals

- This proposal does not add labels to `ComponentRouteStatus`. Labels are configuration input; status conditions already report errors if labels cannot be applied.
- This proposal does not create or manage IngressControllers. Administrators must configure IngressController route selectors separately.
- This proposal does not modify how operators reconcile routes beyond applying the specified labels.

## Proposal

Add an optional `labels` field of type `map[string]string` to the existing `ComponentRouteSpec` struct in `config/v1/types_ingress.go`. The field is gated behind the `IngressComponentRouteLabels` FeatureGate, initially enabled in TechPreview.

### Validation

- `+mapType=granular` for proper strategic merge patch behavior.
- `MinProperties=1` prevents semantically empty `labels: {}`.
- `MaxProperties=8` bounds the map size. Route sharding typically needs 1-2 labels; 8 is generous while providing a ceiling for CEL cost estimation.
- Key validation uses `format.qualifiedName()` to enforce Kubernetes label key conventions.
- Value validation uses `format.labelValue()` to enforce Kubernetes label value conventions.
- Keys with `kubernetes.io/` and `k8s.io/` reserved prefixes are rejected.

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

## Workflow Description

1. Cluster administrator configures an IngressController with a `routeSelector` that matches specific labels.
2. Administrator edits the Ingress cluster config to add labels to one or more `componentRoutes` entries.
3. The operator managing the component route (e.g., console-operator) reconciles the route and applies the specified labels.
4. The IngressController picks up the route based on its label selector.
5. If labels are later changed, the route may be reassigned to a different IngressController.

## API Extensions

Adds one field to an existing stable API type:

```go
type ComponentRouteSpec struct {
    // ... existing fields ...

    // labels defines additional labels to be applied to the route created
    // for the component.
    // +openshift:enable:FeatureGate=IngressComponentRouteLabels
    // +optional
    // +mapType=granular
    Labels map[string]string `json:"labels,omitempty"`
}
```

No new CRDs, webhooks, finalizers, or aggregated API servers are introduced.

## Topology Considerations

### Hypershift

No special considerations. The Ingress config is a cluster-scoped resource managed identically in Hypershift hosted clusters.

### Standalone Clusters

Primary target topology. No special considerations.

### Single-node Deployments (Compact Clusters)

No special considerations. Route labels work the same regardless of node count.

### MicroShift

Not applicable. MicroShift does not use the OpenShift Ingress config API.

## Implementation Details/Notes/Constraints

- The field is behind the `IngressComponentRouteLabels` FeatureGate, initially enabled in `TechPreviewNoUpgrade` and `DevPreviewNoUpgrade`.
- Consuming operators (console-operator, authentication-operator, etc.) must be updated to read and apply the labels from `ComponentRouteSpec` to their managed routes. This work is tracked separately.
- The `+mapType=granular` annotation ensures individual label keys can be added or removed via strategic merge patch without replacing the entire map.

## Risks and Mitigations

- **Risk:** Changing labels on a component route may cause it to move between IngressController shards, potentially causing brief unavailability.
  - **Mitigation:** The godoc explicitly warns about this behavior. The feature is gated behind TechPreview for initial adoption.

- **Risk:** Operators that manage component routes may not immediately support the new labels field.
  - **Mitigation:** The labels field is optional. Operators that do not read the field will simply not apply labels, maintaining current behavior.

## Test Plan

### Unit Tests

CRD validation tests in `config/v1/tests/ingresses.config.openshift.io/IngressComponentRouteLabels.yaml` cover:

- **Happy paths:** single label, multiple labels, DNS-prefixed keys, max-length keys/values, empty string values.
- **Negative paths:** invalid keys, invalid values (starting with dash), reserved prefixes (`kubernetes.io/`, `k8s.io/`), key name over 63 characters, more than 8 labels.
- **Update scenarios:** add labels, change label values, remove labels.
- **DeepCopy:** validated by the repo-wide `TestRoundTripTypesWithoutProtobuf` test.

### E2E Tests

E2E tests will be added when consuming operators are updated to apply labels. These will verify that labels specified in the Ingress config appear on the managed routes.

## Graduation Criteria

### Dev Preview -> Tech Preview

- API field defined and validated.
- CRD validation tests passing.
- At least one consuming operator (console-operator) applies labels from the spec.

### Tech Preview -> GA

- All consuming operators that manage component routes apply labels.
- E2E tests confirm labels are propagated to routes.
- No critical bugs reported during TechPreview.
- Documentation updated with route sharding examples.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

- **Upgrade:** The field is additive and optional. Existing clusters without labels continue to work unchanged. The field only appears in the CRD when the FeatureGate is enabled.
- **Downgrade:** If the cluster is downgraded to a version without this field, the labels key in the Ingress config will be ignored. Routes retain whatever labels were last applied; they are not automatically removed.

## Version Skew Strategy

During upgrades, the control plane may be at a newer version than operators. If the Ingress config contains labels but the operator has not been updated, the operator will ignore the field. This is safe because the field is optional and additive.

## Operational Aspects of API Extensions

### Failure Modes

- If invalid labels are specified, CRD validation rejects the update at admission time.
- If valid labels are specified but the consuming operator does not support them, the labels are silently ignored.

### Support Procedures

- To check if labels are configured: `oc get ingress.config.openshift.io cluster -o jsonpath='{.spec.componentRoutes[*].labels}'`
- To disable the feature: remove the `IngressComponentRouteLabels` FeatureGate (only possible in TechPreview).
