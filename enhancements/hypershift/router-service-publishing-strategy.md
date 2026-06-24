---
title: router-service-publishing-strategy
tracking-link:
  - https://issues.redhat.com/browse/CNTRLPLANE-3527
authors:
  - "@vsolanki12"
reviewers:
  - "@csrwng"
  - "@jparrill"
approvers:
  - "@csrwng"
creation-date: 2026-05-28
api-approvers:
  - "@JoelSpeed"
last-updated: 2026-05-28
status: provisional
see-also:
  - https://issues.redhat.com/browse/OCPBUGS-77856
replaces: []
superseded-by: []
---

# Router Service Publishing Strategy

## Summary

Add a new `Router` entry to the `spec.services[]` API in `HostedCluster` to allow
operators to control how the HCP (HostedControlPlane) private router Service is exposed
on the management cluster. Today the router Service is unconditionally created as
`LoadBalancer`, which blocks non-cloud platforms (Agent, KubeVirt) that lack
cloud load-balancer controllers.

This enhancement extends the existing `ServicePublishingStrategyMapping` pattern —
already used for `APIServer`, `Konnectivity`, `OAuthServer`, and `Ignition`
— to cover the private router, giving operators explicit control over the
service type and external address used to reach HCP-managed routes.

## Motivation

### What the HCP Router Does Today

When a HostedCluster uses the `Route` publishing strategy for the Kubernetes API
server, HyperShift deploys a dedicated HAProxy router in the HCP namespace. This
per-control-plane router serves as the single ingress point for all control plane
routes, including:

- **Kubernetes API Server (KAS)** — the primary cluster API endpoint
- **OAuth Server** — authentication and token issuance
- **Konnectivity Server** — tunneled connectivity between control plane and data plane
- **Ignition Server** — node bootstrap configuration delivery

The HCP router is fronted by a LoadBalancer Service, giving each hosted control plane
its own dedicated load balancer with a unique external address. All routes in the HCP
namespace are labeled and served exclusively by this router, rather than the management
cluster's shared ingress controller.

### Why Customers Choose Route Publishing

Self-managed HyperShift customers choose the `Route` publishing strategy because it
provides **per-control-plane network isolation**:

- **Private clusters** — On cloud platforms with private endpoint access, the HCP
  router provides an internal load balancer that keeps control plane traffic off the
  public internet. Each hosted cluster gets its own isolated network path.

- **On-premises and bare-metal deployments** — Customers running HyperShift on
  Agent or KubeVirt platforms use Route publishing because `LoadBalancer`
  publishing (which provisions cloud-native LBs) is not available. Route publishing
  is the natural choice for these environments where the customer manages their own
  network infrastructure.

- **Multi-tenant isolation** — Each hosted control plane gets its own dedicated
  router, ensuring that traffic for one tenant's control plane does not traverse the
  management cluster's shared ingress.

### The Problem

The HCP router's LoadBalancer Service is created unconditionally, regardless of the
management cluster's platform. On platforms that lack a cloud load-balancer
controller — bare-metal Agent or KubeVirt — the Service stays in `Pending`
state indefinitely. This blocks the entire hosted cluster installation because:

1. **Control plane routes have no ingress status** — Route status depends on the
   router Service's external hostname. Without a provisioned load balancer, the
   hostname is empty, so all control plane routes (KAS, OAuth, Konnectivity,
   Ignition) remain unreachable.

2. **KAS service resolution stalls** — The KAS status check waits for the router
   Service to be provisioned, preventing the API server endpoint from being
   advertised to the data plane.

3. **No API-level control** — Unlike every other control plane service (`APIServer`,
   `Konnectivity`, `OAuthServer`, `Ignition`), the router has no entry in
   `spec.services[]`. Operators on non-cloud platforms have no way to specify an
   alternative exposure mechanism like `NodePort` with an explicit address.

This is the exact scenario where Route publishing should work best — on-premises
customers who want per-control-plane isolation and own their DNS — but the hardcoded
LoadBalancer assumption prevents it.

### Background

OCPBUGS-77856 identified this issue. PR #8439 provided an initial fix by auto-detecting
non-cloud platforms at the CPO level and creating the router Service as `NodePort`
instead of `LoadBalancer`. During review, it was identified that the `host` value for
NodePort services was set to `ClusterIP`, which is only reachable within the management
cluster. External consumers (data plane nodes, external clients) cannot use a ClusterIP
to reach the router.

This enhancement addresses that gap by letting the operator provide an externally
reachable address through the API, the same way `NodePort` publishing works for
APIServer and other services today. PR #8439 has been closed in favor of this
API-driven approach.

### User Stories

#### Story 1: Bare-metal operator using Agent platform

As a cluster administrator deploying HyperShift on bare-metal infrastructure using
the Agent platform, I want to specify how the HCP router Service is exposed so that
my hosted cluster can complete installation without requiring a cloud load balancer.

#### Story 2: KubeVirt platform operator

As a cluster administrator running HyperShift with KubeVirt on an on-premises
management cluster, I want to use NodePort with an explicit address for the router
Service so that HCP routes are reachable from the data plane nodes.

#### Story 3: Cloud platform operator (no change)

As a cluster administrator running HyperShift on AWS/GCP/Azure, I want the router
Service to continue using LoadBalancer by default so that my existing clusters are
not affected by this change.

### Goals

1. Add `Router` as a new `ServiceType` in `spec.services[]` alongside existing types.
2. Allow operators to specify `NodePort` publishing with an explicit `address` and
   optional `port` for the router Service, matching the existing
   `NodePortPublishingStrategy` pattern.
3. Maintain backward compatibility — existing HostedClusters without a `Router` entry
   in `spec.services[]` continue to get `LoadBalancer` behavior (current default).
4. Provide a clean, API-driven path for non-cloud platforms to expose HCP routes
   without relying on platform auto-detection heuristics.

### Non-Goals

1. Decoupling endpoint exposure decisions from `HostedCluster.spec.platform.type`
   entirely — that is a separate, broader concern.
2. Supporting `S3` or `None` publishing types for the router (only `LoadBalancer`,
   `NodePort`, and `Route` are meaningful for this service).
3. Modifying how the hypershift-operator selects the default publishing strategy
   based on platform type — that is a separate concern.
4. Changing how cloud platforms expose the router (AWS NLB, GCP ILB, Azure LB
   behaviors remain unchanged).

## Proposal

### Workflow Description

1. Operator creates HostedCluster with Router entry in spec.services[] using NodePort + address
2. HO validates the spec (existing CEL rules)
3. CPO reads the Router strategy from spec.services[]
4. CPO creates the router Service as NodePort instead of LoadBalancer
5. reconcileRouterServiceStatus() populates route status using the user-provided address
6. KAS resolves through the router's external address
7. Cluster installation completes normally

### API Extensions

#### New ServiceType constant

Add a new `Router` constant to the `ServiceType` enum in
`api/hypershift/v1beta1/hostedcluster_types.go`:

```go
// Router is the service for the HCP private router that serves
// routes for control plane services (OAuth, downloads, etc.)
Router ServiceType = "Router"
```

The `spec.services[]` field currently has `MaxItems=6` (to accommodate the 6 existing
service types including deprecated OVNSbDb and OIDC). This must be bumped to `MaxItems=7`
to allow the new `Router` entry.

No new CEL validation rules are needed beyond updating the enum membership — the
existing `ServicePublishingStrategy` validation rules already handle `NodePort`,
`LoadBalancer`, and `Route` publishing types correctly.

#### Usage in HostedCluster spec

Operators specify the router publishing strategy the same way they do for other
services:

```yaml
spec:
  services:
    - service: Router
      servicePublishingStrategy:
        type: NodePort
        nodePort:
          address: 192.168.126.10   # externally reachable management node IP
          port: 30443               # optional, auto-assigned if omitted
```

For cloud platforms, or when no `Router` entry is specified, the default behavior
is `LoadBalancer` (preserving backward compatibility).

```yaml
spec:
  services:
    - service: Router
      servicePublishingStrategy:
        type: LoadBalancer
```

> **Note:** When `LoadBalancer` is specified for the Router, specifying a `hostname`
> does not make sense. The router's purpose is to route *other* hostnames to control
> plane services — it does not need its own hostname override.

### Implementation Details/Notes/Constraints

Three code paths consume the HCP router Service and need to be updated:

#### 1. Router Service creation (`ingress/router.go`)

`ReconcileRouterService()` currently hardcodes the service type. The updated function
reads the `Router` entry from `spec.services[]` to determine the service type:

- If the strategy type is `NodePort`, create the Service as `ServiceTypeNodePort`.
- If the strategy type is `LoadBalancer` (or no entry exists), create as
  `ServiceTypeLoadBalancer` (current default behavior).
- The `svc.Spec.Type` is only set on initial creation (`svc.Spec.Type == ""`)
  to prevent flipping the type on existing clusters during upgrade.

The function signature changes to accept the `ServicePublishingStrategy`:

```go
func ReconcileRouterService(svc *corev1.Service, internal, crossZoneLoadBalancingEnabled bool,
    hcp *hyperv1.HostedControlPlane, strategy *hyperv1.ServicePublishingStrategy) error {
    // ...
    if svc.Spec.Type == "" {
        if strategy != nil && strategy.Type == hyperv1.NodePort {
            svc.Spec.Type = corev1.ServiceTypeNodePort
        } else {
            svc.Spec.Type = corev1.ServiceTypeLoadBalancer
        }
    }
}
```

#### 2. Router service status (`infra/infra.go`)

`reconcileRouterServiceStatus()` currently falls back to `svc.Spec.ClusterIP` for
NodePort services. The updated function uses the user-provided `address` from the
`NodePortPublishingStrategy`:

```go
if svc.Spec.Type == corev1.ServiceTypeNodePort {
    if strategy.NodePort != nil && strategy.NodePort.Address != "" {
        host = strategy.NodePort.Address
    }
    return
}
```

This address flows through the following path:
- `infraStatus.ExternalHCPRouterHost` / `infraStatus.InternalHCPRouterHost`
- `AdmitHCPManagedRoutes()` → `ReconcileRouteStatus()`
- `RouterCanonicalHostname` on each Route's ingress status

#### 3. KAS service status (`kas/service.go`)

`ReconcileServiceStatus()` uses the same address from the strategy when the
router Service is NodePort, instead of calling
`CollectLBMessageIfNotProvisioned()` which would block indefinitely.

### Defaults and Backward Compatibility

| Scenario | Behavior |
|----------|----------|
| Existing HC with no `Router` in `spec.services[]` | Default to `LoadBalancer` — no change |
| New HC on cloud platform | User can omit `Router` (gets LB) or explicitly set it |
| New HC on Agent/KubeVirt | User sets `Router` with `NodePort` + address |

The `Router` entry in `spec.services[]` is **optional**. When absent, the CPO defaults
to `LoadBalancer` behavior, matching the current behavior. This ensures zero disruption
for existing clusters during upgrade.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specifically designed for HyperShift. The `Router` ServiceType
is added to the `HostedCluster` API and consumed by the CPO. It enables non-cloud
management clusters (Agent, KubeVirt) to expose the HCP private router via NodePort
with an explicit address, unblocking cluster installation on platforms without cloud
load-balancer controllers.

For managed offerings (ROSA HCP, ARO HCP), no change is needed — these always run
on cloud platforms with LB support and will continue using `LoadBalancer`.

#### Standalone Clusters

Not applicable. This enhancement modifies the HyperShift `HostedCluster` API and
CPO behavior only. Standalone OpenShift clusters do not use `ServicePublishingStrategy`.

#### Single-node Deployments or MicroShift

Not applicable. This enhancement does not affect SNO resource consumption or MicroShift.
The change is scoped to HyperShift's `HostedCluster` API and CPO.

#### OpenShift Kubernetes Engine

Not applicable. This enhancement does not depend on features excluded from the OKE
product offering. It extends the existing `ServicePublishingStrategy` pattern in
HyperShift.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| User provides unreachable address | Same risk that exists today for `APIServer` NodePort. User is responsible for providing a valid, externally reachable address. Documentation will cover requirements. |
| Upgrade safety — flipping service type | The `svc.Spec.Type == ""` guard ensures existing services keep their type. API-level changes only apply to new clusters or explicit user edits. The `spec.services[]` field is immutable, so the strategy cannot change after cluster creation. |
| MaxItems increase | Bumping from 6 to 7 is safe. Existing clusters with 6 entries are unaffected. The increase only allows an additional optional entry. |
| CEL validation complexity | The new `Router` type follows the exact same validation pattern as existing service types. No new validation rules needed beyond enum membership. |

### Drawbacks

- adds a 7th service type, incrementally increasing API surface
- requires operator to manually provide an externally reachable address — no auto-detection.

## Test Plan

### Unit Tests

- `ReconcileRouterService()` respects `NodePort` strategy from `spec.services[]`
- `ReconcileRouterService()` defaults to `LoadBalancer` when no `Router` entry exists
- `ReconcileRouterService()` preserves existing service type on reconcile (upgrade safety)
- `reconcileRouterServiceStatus()` uses `NodePort.Address` when available
- `reconcileRouterServiceStatus()` returns empty host when NodePort address not configured
- `ReconcileServiceStatus()` in KAS handles NodePort router with strategy address
- Existing cloud platform tests remain passing (AWS, GCP, Azure)

### Envtest (API Validation Tests)

Following the existing `test/envtest/` patterns:

- YAML test cases for `Router` ServiceType in `spec.services[]`
- Verify `NodePort` strategy requires `nodePort.address` (existing CEL rule)
- Verify `LoadBalancer` strategy works for `Router`
- Verify invalid publishing types are rejected
- Verify `MaxItems=7` allows `Router` alongside all other service types

### Integration Tests

- Create HostedCluster on Agent platform with Router NodePort strategy
- Verify router Service is created as NodePort
- Verify route status is populated with the user-provided address
- Verify KAS service status resolves correctly

### E2E Tests

- Agent platform cluster lifecycle with Router NodePort strategy
- KubeVirt platform cluster lifecycle with Router NodePort strategy
- Verify HCP routes are accessible through the NodePort address

## Graduation Criteria

### Dev Preview -> Tech Preview

- API change merged with `Router` ServiceType
- CPO consumes the new field for service creation and status resolution
- Unit and envtest tests passing
- End user documentation for non-cloud platform setup
- Gather feedback from users on Agent and KubeVirt platforms

### Tech Preview -> GA

- E2E validation on Agent and KubeVirt platforms
- Upgrade and downgrade testing (clusters without `Router` entry continue working)
- Sufficient time for feedback (at least one release cycle of soak time)
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

Not applicable. This enhancement adds a new feature and does not deprecate any
existing functionality.

## Implementation History

- 2026-05: OCPBUGS-77856 filed — identified that router Service hardcoded as
  LoadBalancer blocks non-cloud platforms.
- 2026-05: [PR #8439](https://github.com/openshift/hypershift/pull/8439) — Initial
  fix using platform auto-detection to create NodePort on non-cloud platforms.
  Closed after review identified ClusterIP reachability limitation.
- 2026-05: [CNTRLPLANE-3527](https://issues.redhat.com/browse/CNTRLPLANE-3527) —
  Epic created for the proper API-driven solution.
- 2026-05: This enhancement proposed to provide API-driven control over router
  service publishing strategy.

## Alternatives (Not Implemented)

### 1. Platform auto-detection only (PR #8439 approach)

CPO detects non-cloud platforms and creates NodePort automatically, using ClusterIP
as the host.

**Pros**: No API change needed, works immediately.
**Cons**: ClusterIP is not externally reachable; no user control over the address;
couples behavior to platform type detection which may not cover all cases.

### 2. Infer from management cluster capabilities

Detect whether the management cluster has a cloud LB controller and choose the
service type accordingly.

**Pros**: Fully automatic, no user input required.
**Cons**: Complex detection logic; unreliable across different management cluster
configurations; doesn't solve the external address problem.

### 3. Reuse existing Route publishing type for the router

Expose the router via an OpenShift Route on the management cluster instead of a
dedicated Service.

**Pros**: Works on any management cluster with an ingress controller.
**Cons**: Creates a dependency on the management cluster's ingress controller;
adds latency through an extra proxy hop; may conflict with management cluster
routing rules.

## Open Questions

None at this time.

## Upgrade / Downgrade Strategy
- `spec.services[]` is immutable after creation, so the strategy cannot change post-install.
- **Downgrade**: older CPO ignores unknown Router entry and falls back to LB default.
- **Upgrade**: clusters without a Router entry continue with LB behavior unchanged.

## Version Skew Strategy

- **HO knows Router but CPO doesn't**: If the HostedCluster is configured with a
  `Router` service entry and the CPO does not support it, the HostedCluster should
  fail to provision with an `InvalidConfiguration` condition explaining why. This
  prevents silent misconfiguration where the operator expects NodePort behavior but
  the CPO falls back to LoadBalancer.
- **CPO knows Router but HO doesn't**: HO rejects at admission (safe, no silent misconfiguration).

## Operational Aspects of API Extensions

### Monitoring Requirements

No new metrics are required. The existing router Service health can be monitored through
standard Kubernetes service status. If the user-provided NodePort address becomes
unreachable, HCP routes will fail — this is the same operational model as `APIServer`
NodePort today.

### Failure Modes

- **Unreachable NodePort address**: HCP routes will not resolve. Operator must update the
  address or fix network connectivity. Detectable via route status and HCP conditions.
- **NodePort port conflict**: If a specific port is requested and conflicts with another
  service, the Service creation will fail. Using port `0` (auto-assign) avoids this.

## Support Procedures

For clusters using `Router` with `NodePort` strategy:

1. Verify the NodePort address is reachable from data plane nodes
2. Verify the NodePort port is accessible through any firewalls
3. Check `oc get svc router -n <hcp-namespace>` for service status
4. Check route status for `RouterCanonicalHostname` — should match the configured address

## Infrastructure Needed [optional]
None
