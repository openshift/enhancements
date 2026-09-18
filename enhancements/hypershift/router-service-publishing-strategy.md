---
title: router-service-publishing-strategy
tracking-link:
  - https://issues.redhat.com/browse/CNTRLPLANE-3527
authors:
  - "@vsolanki12"
reviewers:
  - "@csrwng"
  - "@jparrill"
  - "@muraee"
approvers:
  - "@csrwng"
creation-date: 2026-05-28
api-approvers:
  - "@JoelSpeed"
last-updated: 2026-08-13
status: provisional
see-also:
  - https://issues.redhat.com/browse/OCPBUGS-77856
replaces: []
superseded-by: []
---

# Router Publishing Strategy

## Summary

Add a new `spec.routerPublishing` field to the `HostedCluster` API to give operators
explicit control over how the HCP (HostedControlPlane) private router Service is
exposed on the management cluster. Today the router Service is unconditionally created
as `LoadBalancer`, which blocks non-cloud platforms (Agent, KubeVirt) that lack cloud
load-balancer controllers.

When `spec.routerPublishing` is set, a dedicated HCP router is deployed and exposed as
specified. All control plane services are published through this router using the
`Route` strategy — their per-service hostnames are configured under
`spec.routerPublishing.services`. The existing `spec.services[]` field is mutually
exclusive with `spec.routerPublishing`.

When `spec.routerPublishing` is not set, current behavior is preserved — `spec.services[]`
controls everything, and the router is derived from the combination of strategies and
endpoint access mode.

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

3. **No API-level control** — The router is shared infrastructure that implements
   the `Route` publishing strategy for all control plane services, yet it has no
   dedicated configuration surface. Operators on non-cloud platforms have no way to
   specify an alternative exposure mechanism like `NodePort` with an explicit address.

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
reachable address through the API. PR #8439 has been closed in favor of this
API-driven approach.

### Why a top-level field instead of `spec.services[]`

An earlier revision of this enhancement proposed adding `Router` as a new entry in
`spec.services[]`. Review feedback identified several problems with that approach:

1. **Semantic mismatch.** `spec.services[]` maps user-facing services to how they are
   exposed. The router is not a user-facing service — it is shared infrastructure (a
   single HAProxy deployment) that implements the `Route` strategy for all other
   services via SNI routing.

2. **Immutability.** `spec.services[]` is immutable after creation. If an operator did
   not add a `Router` entry at creation time, they cannot add it later without
   recreating the cluster.

3. **Lifecycle ambiguity.** The router only deploys under certain conditions
   (`UseHCPRouter` — private clusters, KAS using Route+hostname, etc.). A `Router`
   ServiceType could be configured when the router would not exist, with no clean way
   to validate that at admission time since it depends on a combination of platform,
   endpoint access mode, and other services' strategies.

4. **Redundant configuration.** When all services use Route (required on most platforms
   for OAuth/Konnectivity/Ignition), adding a `Router` entry adds no information — the
   router would be deployed anyway. The only new information is how to expose it.

The `spec.routerPublishing` field resolves all of these: it explicitly declares the
intent to deploy and expose a dedicated router, can be designed with mutable semantics,
and centralizes both router exposure and per-service hostnames in one place.

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

1. Add `spec.routerPublishing` as a new top-level field on `HostedCluster` to control
   how the HCP private router is exposed, mutually exclusive with `spec.services[]`.
2. Allow operators to specify `NodePort` exposure with an explicit `hostname` for
   the router Service, providing an externally reachable address for non-cloud platforms.
3. Centralize per-service Route hostnames (KAS, OAuth, Konnectivity, Ignition) under
   `spec.routerPublishing.services` instead of requiring separate `spec.services[]` entries.
4. Maintain backward compatibility — existing HostedClusters using `spec.services[]`
   continue to work unchanged.
5. Design the new field with partially mutable semantics: `type` is immutable after
   creation (matching `spec.services[]` behavior), but `hostname`, `port`, and
   per-service hostnames are mutable to allow address changes without cluster recreation.

### Non-Goals

1. Decoupling endpoint exposure decisions from `HostedCluster.spec.platform.type`
   entirely — that is a separate, broader concern.
2. Deprecating `spec.services[]` — both configuration modes coexist.
3. Supporting `S3` or `None` publishing types for the router (only `LoadBalancer`
   and `NodePort` are meaningful).
4. Modifying how the hypershift-operator selects the default publishing strategy
   based on platform type — that is a separate concern.
5. Changing how cloud platforms expose the router (AWS NLB, GCP ILB, Azure LB
   behaviors remain unchanged).

## Proposal

### Workflow Description

1. Operator creates HostedCluster with `spec.routerPublishing` set (no `spec.services[]`)
2. HO validates: `spec.routerPublishing` and `spec.services[]` are mutually exclusive
3. HO generates the HCP with router publishing configuration
4. CPO deploys the dedicated HCP router, exposed as specified (NodePort or LoadBalancer)
5. CPO derives Route strategy for all control plane services from `spec.routerPublishing`
6. `reconcileRouterServiceStatus()` populates route status using the configured hostname
7. KAS resolves through the router's external address
8. Cluster installation completes normally

### API Extensions

#### Schema change to `spec.services[]`

Today `spec.services[]` is required in the `HostedCluster` CRD schema. With this
enhancement, it must become optional (`+optional`) so that router-only clusters can
omit it entirely. The "at least one of `services` or `routerPublishing`" constraint
is enforced by CEL, not the OpenAPI required list. Envtest coverage must verify that
a HostedCluster with only `routerPublishing` (no `services[]`) is accepted.

#### New types

Add the following to `api/hypershift/v1beta1/hostedcluster_types.go`:

```go
// In HostedClusterSpec
type HostedClusterSpec struct {
    // ...existing fields...

    // RouterPublishing configures the dedicated HCP router exposure.
    // Mutually exclusive with Services[].
    // +optional
    RouterPublishing *RouterPublishing `json:"routerPublishing,omitempty"`
}
```

```go
// RouterPublishing configures a dedicated HCP router and how it is exposed
// on the management cluster. When set, all control plane services are
// published through this router using the Route strategy. Mutually exclusive
// with spec.services[].
type RouterPublishing struct {
    // Type specifies how the router Service is exposed on the management cluster.
    // Valid values are "LoadBalancer" and "NodePort".
    // +kubebuilder:validation:Enum=LoadBalancer;NodePort
    // +required
    Type PublishingStrategyType `json:"type"`

    // Hostname is the externally reachable address of the router.
    // Required when Type is NodePort. This must be an IPv4 address, IPv6
    // address, or a valid DNS name reachable from data plane nodes and
    // external clients.
    // +optional
    // +kubebuilder:validation:Pattern=`^([a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*|(\d{1,3}\.){3}\d{1,3}|\[?[0-9a-fA-F:]+\]?)$`
    Hostname string `json:"hostname,omitempty"`

    // Port is the NodePort to request for the router Service.
    // Only applicable when Type is NodePort. Maps to
    // svc.Spec.Ports[0].NodePort. If omitted, Kubernetes auto-assigns
    // a port from the cluster's configured node-port range.
    // If the requested port is already in use, Service creation fails
    // with a port conflict error. Port range validation is left to the
    // Kubernetes API server (respects --service-node-port-range).
    // +optional
    Port *int32 `json:"port,omitempty"`

    // Services configures per-service Route hostnames for control plane
    // services published through this router. When omitted, hostnames
    // are derived from the cluster's baseDomain using the convention
    // <service>.<baseDomain> (e.g., api.<baseDomain>).
    // +optional
    Services *RouterServiceHostnames `json:"services,omitempty"`
}

// RouterServiceHostnames holds per-service hostname overrides for control
// plane services published through the HCP router.
type RouterServiceHostnames struct {
    // APIServer is the hostname for the Kubernetes API server Route.
    // +optional
    APIServer *ServiceHostname `json:"apiServer,omitempty"`

    // OAuthServer is the hostname for the OAuth server Route.
    // +optional
    OAuthServer *ServiceHostname `json:"oAuthServer,omitempty"`

    // Konnectivity is the hostname for the Konnectivity server Route.
    // +optional
    Konnectivity *ServiceHostname `json:"konnectivity,omitempty"`

    // Ignition is the hostname for the Ignition server Route.
    // +optional
    Ignition *ServiceHostname `json:"ignition,omitempty"`
}

// ServiceHostname configures the hostname for a single service Route.
type ServiceHostname struct {
    // Hostname is the DNS name used as the Route host and SNI target.
    // Must be a valid DNS name (RFC 1123).
    // +required
    // +kubebuilder:validation:Pattern=`^[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*$`
    Hostname string `json:"hostname"`
}
```

#### HostedControlPlane transport

The CPO consumes `HostedControlPlane`, not `HostedCluster`. The `RouterPublishing`
type is shared — it appears on both `HostedClusterSpec` and `HostedControlPlaneSpec`:

```go
// In HostedControlPlaneSpec
type HostedControlPlaneSpec struct {
    // ...existing fields...

    // RouterPublishing configures the dedicated HCP router exposure.
    // Propagated from HostedCluster.spec.routerPublishing by the HO.
    // +optional
    RouterPublishing *RouterPublishing `json:"routerPublishing,omitempty"`
}
```

The HO propagates this field during HCP creation and updates:
- If `HostedCluster.spec.routerPublishing` is set, HO copies it to `HCP.spec.routerPublishing`
- If not set, `HCP.spec.routerPublishing` remains nil — CPO uses existing `spec.services[]` behavior
- Per-service hostnames that were omitted on the HostedCluster are populated by HO
  using the `<service>.<baseDomain>` convention before writing to HCP

#### Usage in HostedCluster spec

Non-cloud platform with NodePort:

```yaml
spec:
  routerPublishing:
    type: NodePort
    hostname: 192.168.126.10
    port: 30443
    services:
      apiServer:
        hostname: api.mycluster.example.com
      oAuthServer:
        hostname: oauth.mycluster.example.com
      konnectivity:
        hostname: konnectivity.mycluster.example.com
      ignition:
        hostname: ignition.mycluster.example.com
```

Cloud platform with LoadBalancer (explicit):

```yaml
spec:
  routerPublishing:
    type: LoadBalancer
    services:
      apiServer:
        hostname: api.mycluster.example.com
      oAuthServer:
        hostname: oauth.mycluster.example.com
```

When `spec.routerPublishing` is not set, existing `spec.services[]` behavior is
preserved unchanged.

#### Validation rules

CEL validation enforces the following invariants on `HostedClusterSpec`:

```go
// Mutual exclusivity: routerPublishing and services cannot both be set
// +kubebuilder:validation:XValidation:rule="!(has(self.routerPublishing) && has(self.services) && size(self.services) > 0)",message="routerPublishing and services are mutually exclusive"

// At least one must be set
// +kubebuilder:validation:XValidation:rule="has(self.routerPublishing) || (has(self.services) && size(self.services) > 0)",message="either routerPublishing or services must be specified"
```

CEL validation on `RouterPublishing`:

```go
// NodePort requires hostname
// +kubebuilder:validation:XValidation:rule="self.type != 'NodePort' || (has(self.hostname) && self.hostname != '')",message="hostname is required when type is NodePort"

// LoadBalancer must not specify port
// +kubebuilder:validation:XValidation:rule="self.type != 'LoadBalancer' || !has(self.port)",message="port is not applicable when type is LoadBalancer"

// Type is immutable after creation
// +kubebuilder:validation:XValidation:rule="self.type == oldSelf.type",message="routerPublishing.type is immutable"
```

Per-service hostname validation uses standard DNS name rules via the existing
`+kubebuilder:validation:Pattern` on the `Hostname` field in `ServiceHostname`.

**Hostname uniqueness:** HO validates that all effective service hostnames (explicit +
derived from `<service>.<baseDomain>`) are unique after defaulting is applied. Duplicate
hostnames are rejected before HCP creation with a validation error, since duplicate
Route hosts on a shared SNI router would route traffic to the wrong backend. This
validation runs in the HO reconciler (not CEL, since it depends on runtime defaulting)
and is covered by envtest cases for both duplicate-explicit and explicit-vs-derived
collision scenarios.

### Implementation Details/Notes/Constraints

#### HyperShift Operator (HO)

When `spec.routerPublishing` is set, HO translates the configuration into the
HostedControlPlane spec. All services are implicitly set to `Route` publishing —
the per-service hostnames from `spec.routerPublishing.services` become the Route
hostnames.

**Hostname defaulting:** When `spec.routerPublishing.services` is omitted or
individual service hostnames are not specified, HO derives them from the cluster's
`baseDomain` using the convention `<service>.<baseDomain>`:
- `api.<baseDomain>` for APIServer
- `oauth.<baseDomain>` for OAuthServer
- `konnectivity.<baseDomain>` for Konnectivity
- `ignition.<baseDomain>` for Ignition

This ensures all four services always have Route hostnames before the HCP is created,
regardless of whether the operator specified them explicitly.

**Router activation:** Setting `spec.routerPublishing` unconditionally deploys the
dedicated HCP router, overriding the existing `UseHCPRouter` decision logic. The
`UseHCPRouter` function is only consulted when `spec.routerPublishing` is not set
(i.e., when `spec.services[]` mode is active).

#### Control Plane Operator (CPO)

Three code paths consume the HCP router Service and need to be updated:

##### 1. Router Service creation (`ingress/router.go`)

`ReconcileRouterService()` currently hardcodes the service type. The updated function
reads the router publishing configuration to determine the service type:

- If the type is `NodePort`, create the Service as `ServiceTypeNodePort` and set
  `svc.Spec.Ports[0].NodePort` to the requested port (or leave as 0 for auto-assign).
- If the type is `LoadBalancer` (or not configured), create as
  `ServiceTypeLoadBalancer` (current default behavior).
- The `svc.Spec.Type` is only set on initial creation (`svc.Spec.Type == ""`)
  to prevent flipping the type on existing clusters during upgrade. This is consistent
  with `routerPublishing.type` being immutable after creation.
- On subsequent reconciles, mutable fields (`hostname`, `port`, per-service hostnames)
  are updated without recreating the Service.

##### 2. Router service status (`infra/infra.go`)

`reconcileRouterServiceStatus()` currently falls back to `svc.Spec.ClusterIP` for
NodePort services. The updated function uses the `hostname` from the router publishing
configuration:

```go
if svc.Spec.Type == corev1.ServiceTypeNodePort {
    if routerPublishing != nil && routerPublishing.Hostname != "" {
        host = routerPublishing.Hostname
    }
    // Resolve effective port: explicit from config, or auto-assigned from Service
    if routerPublishing != nil && routerPublishing.Port != nil {
        port = *routerPublishing.Port
    } else if len(svc.Spec.Ports) > 0 {
        port = svc.Spec.Ports[0].NodePort
    }
    return
}
```

This flows through:
- `infraStatus.ExternalHCPRouterHost` / `infraStatus.InternalHCPRouterHost` — hostname
- `infraStatus.ExternalHCPRouterPort` / `infraStatus.InternalHCPRouterPort` — effective port
- `AdmitHCPManagedRoutes()` / `ReconcileRouteStatus()`
- `RouterCanonicalHostname` on each Route's ingress status

Consumers (data plane nodes, external clients) use `hostname:port` to reach the
router. For LoadBalancer, port is the standard HTTPS port (443). For NodePort, port
is the auto-assigned or explicitly requested NodePort value. Integration tests cover
both explicit and auto-assigned port scenarios.

##### 3. KAS service status (`kas/service.go`)

`ReconcileServiceStatus()` uses the same hostname and port from the router publishing
config when the router Service is NodePort, instead of calling
`CollectLBMessageIfNotProvisioned()` which would block indefinitely.

### Defaults and Backward Compatibility

| Scenario | Behavior |
|----------|----------|
| Existing HC with `spec.services[]` (no `routerPublishing`) | No change — `spec.services[]` continues to work, router behavior derived from existing `UseHCPRouter` logic |
| New HC with `spec.routerPublishing` (no `services[]`) | Dedicated router deployed, all services use Route with derived or explicit hostnames |
| New HC with `spec.services[]` (no `routerPublishing`) | Current behavior — `spec.services[]` controls everything |
| New HC with both fields | Rejected by CEL validation |

> **Note:** Existing clusters always have `spec.services[]` set (it is currently
> required). The "at least one must be set" CEL rule does not break existing clusters —
> it only ensures new clusters choose one configuration mode. When `spec.routerPublishing`
> is not set, `spec.services[]` is the active mode, and the summary statement
> "current behavior is preserved" applies.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specifically designed for HyperShift. The `spec.routerPublishing`
field is added to the `HostedCluster` API and consumed by the CPO. It enables non-cloud
management clusters (Agent, KubeVirt) to expose the HCP private router via NodePort
with an explicit address, unblocking cluster installation on platforms without cloud
load-balancer controllers.

For managed offerings (ROSA HCP, ARO HCP), no change is needed — these always run
on cloud platforms with LB support and will continue using `spec.services[]`.

#### Standalone Clusters

Not applicable. This enhancement modifies the HyperShift `HostedCluster` API and
CPO behavior only. Standalone OpenShift clusters do not use `ServicePublishingStrategy`.

#### Single-node Deployments or MicroShift

Not applicable. This enhancement does not affect SNO resource consumption or MicroShift.
The change is scoped to HyperShift's `HostedCluster` API and CPO.

#### OpenShift Kubernetes Engine

Not applicable. This enhancement does not depend on features excluded from the OKE
product offering.

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| User provides unreachable hostname | Same risk as `APIServer` NodePort today. User is responsible for providing a valid, externally reachable address. Documentation will cover requirements. |
| Two configuration modes increase complexity | CEL mutual exclusivity prevents mixed state. Documentation will clearly guide which mode to use for each platform. |
| Upgrade safety — flipping service type | `routerPublishing.type` is immutable after creation (CEL enforced). The `svc.Spec.Type == ""` guard provides defense in depth at the CPO level. |
| Migration from `spec.services[]` to `spec.routerPublishing` | Not supported in-place due to `spec.services[]` immutability. Documented as requiring cluster recreation. |
| CRD downgrade prunes `routerPublishing` | Downgrade documentation requires migrating clusters back to `spec.services[]` before HO downgrade. HO sets `RouterPublishingSupported` condition for observability. |

### Drawbacks

- Introduces a second configuration mode for service publishing alongside `spec.services[]`.
- Requires operator to manually provide an externally reachable hostname — no auto-detection.
- Operators using `spec.services[]` cannot migrate to `spec.routerPublishing` without
  cluster recreation.

## Test Plan

### Unit Tests

- `ReconcileRouterService()` respects `NodePort` type from `routerPublishing`
- `ReconcileRouterService()` defaults to `LoadBalancer` when `routerPublishing` not set
- `ReconcileRouterService()` preserves existing service type on reconcile (upgrade safety)
- `reconcileRouterServiceStatus()` uses `routerPublishing.hostname` when available
- `reconcileRouterServiceStatus()` returns empty host when hostname not configured
- `ReconcileServiceStatus()` in KAS handles NodePort router with configured hostname
- HO correctly derives Route strategy for all services when `routerPublishing` is set
- Existing cloud platform tests remain passing (AWS, GCP, Azure)

### Envtest (API Validation Tests)

Following the existing `test/envtest/` patterns:

- YAML test cases for `spec.routerPublishing` field
- Verify mutual exclusivity: `routerPublishing` + `services[]` rejected
- Verify `NodePort` type requires `hostname`
- Verify `LoadBalancer` type works without `hostname`
- Verify per-service hostname validation
- Verify existing `spec.services[]` behavior unchanged

### Integration Tests

- Create HostedCluster on Agent platform with `routerPublishing` NodePort
- Verify router Service is created as NodePort
- Verify route status is populated with the configured hostname
- Verify KAS service status resolves correctly
- Verify all services (OAuth, Konnectivity, Ignition) use correct Route hostnames

### E2E Tests

- Agent platform cluster lifecycle with `routerPublishing` NodePort
- KubeVirt platform cluster lifecycle with `routerPublishing` NodePort
- Verify HCP routes are accessible through the configured hostname

## Graduation Criteria

### Dev Preview -> Tech Preview

- API change merged with `spec.routerPublishing` field
- CPO consumes the new field for router deployment and service creation
- HO translates `routerPublishing` into HCP spec
- Unit and envtest tests passing
- End user documentation for non-cloud platform setup
- Gather feedback from users on Agent and KubeVirt platforms

### Tech Preview -> GA

- E2E validation on Agent and KubeVirt platforms
- Upgrade and downgrade testing (clusters with `spec.services[]` continue working)
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
- 2026-05: Initial enhancement proposed adding `Router` to `spec.services[]`.
- 2026-06: csrwng reviewed and approved the `spec.services[]` approach.
- 2026-08: muraee proposed `spec.routerPublishing` as a dedicated top-level field,
  citing semantic mismatch, immutability constraints, and lifecycle ambiguity with
  `spec.services[]`. Enhancement revised to adopt this approach.

## Alternatives (Not Implemented)

### 1. Add `Router` to `spec.services[]`

Add a new `Router` ServiceType constant to the existing `spec.services[]` field,
following the same `ServicePublishingStrategyMapping` pattern used for APIServer,
OAuth, Konnectivity, and Ignition.

**Pros**: Minimal API change; reuses existing pattern; no new types needed.
**Cons**: Semantic mismatch — the router is infrastructure, not a user-facing service;
`spec.services[]` is immutable, preventing post-creation addition; requires
`MaxItems` bump; lifecycle ambiguity when router wouldn't be deployed; redundant
configuration when all services already use Route.

### 2. Platform auto-detection only (PR #8439 approach)

CPO detects non-cloud platforms and creates NodePort automatically, using ClusterIP
as the host.

**Pros**: No API change needed, works immediately.
**Cons**: ClusterIP is not externally reachable; no user control over the address;
couples behavior to platform type detection which may not cover all cases.

### 3. Infer from management cluster capabilities

Detect whether the management cluster has a cloud LB controller and choose the
service type accordingly.

**Pros**: Fully automatic, no user input required.
**Cons**: Complex detection logic; unreliable across different management cluster
configurations; doesn't solve the external address problem.

### 4. Reuse existing Route publishing type for the router

Expose the router via an OpenShift Route on the management cluster instead of a
dedicated Service.

**Pros**: Works on any management cluster with an ingress controller.
**Cons**: Creates a dependency on the management cluster's ingress controller;
adds latency through an extra proxy hop; may conflict with management cluster
routing rules.

## Open Questions

1. Should `spec.routerPublishing` support a `baseDomain` shorthand that auto-derives
   per-service hostnames (`api.<baseDomain>`, `oauth.<baseDomain>`, etc.) as a
   convenience on top of explicit per-service hostnames? This could be considered as
   a follow-up enhancement.

## Upgrade / Downgrade Strategy

- `spec.routerPublishing` is a new field — existing clusters using `spec.services[]`
  are unaffected.
- **Downgrade**: Before downgrading HO, clusters using `routerPublishing` must be
  migrated back to `spec.services[]`. The procedure is:
  1. Export the HostedCluster spec (`oc get hostedcluster <name> -o yaml`)
  2. Delete the HostedCluster (with `--cascade=orphan` to preserve data plane)
  3. Recreate with equivalent `spec.services[]` configuration
  4. Proceed with HO downgrade
  This is required because `spec.services[]` is immutable and cannot be added to an
  existing cluster that was created without it. Downgrading without migration causes
  the older CRD to prune `routerPublishing`, leaving the cluster with no publishing
  configuration. Release notes will document this procedure.
- **Upgrade**: Clusters using `spec.services[]` continue working unchanged. No
  automatic migration to `spec.routerPublishing`.

## Version Skew Strategy

The `spec.routerPublishing` field is added to the `HostedCluster` CRD as part of the
HO upgrade. The CRD update and HO binary are deployed together — there is no window
where the field exists in the CRD but HO does not understand it, or vice versa.

**CRD pruning concern:** Kubernetes prunes fields absent from a structural CRD schema.
If the CRD is rolled back to an older version that lacks `routerPublishing`, the API
server silently removes the field from stored objects on next write. This is mitigated by:
1. CRD updates are part of the HO deployment — they are upgraded and downgraded together
2. Downgrade documentation explicitly warns that clusters using `routerPublishing` must
   be migrated back to `spec.services[]` before downgrading HO
3. HO sets a `RouterPublishingSupported` condition that CPO can check

**Supported version pairs:**

| HO CRD | HO binary | CPO binary | HCP CRD | Result |
|--------|-----------|------------|---------|--------|
| Has field | Supports | Supports | Has field | Full functionality |
| Has field | Supports | Old | Has field | HO propagates to HCP. CPO ignores unknown field, falls back to LB. HO sets `InvalidConfiguration` condition. **Router-only clusters (no `spec.services[]`) will fail** — CPO has no fallback. |
| Has field | Supports | Old | Old (prunes) | Same as above, but HCP field is pruned by API server. CPO sees nil. HO detects via missing status. |
| Old (prunes) | Old | Supports | Any | HO does not populate `HCP.spec.routerPublishing`. CPO uses `spec.services[]`. No issue. |
| Old (prunes) | Old | Old | Any | Current behavior. No issue. |

**Router-only cluster risk:** A cluster created with `routerPublishing` and no
`spec.services[]` has no fallback if CPO does not support the field. HO must validate
CPO compatibility (via release version or capability annotation) before allowing
router-only cluster creation. This validation is enforced in the HO reconciler and
covered by skew tests.

## Operational Aspects of API Extensions

### Monitoring Requirements

No new metrics are required. The existing router Service health can be monitored through
standard Kubernetes service status. If the configured hostname becomes unreachable,
HCP routes will fail — this is the same operational model as `APIServer` NodePort today.

### Failure Modes

- **Unreachable hostname**: HCP routes will not resolve. Operator must update the
  hostname or fix network connectivity. Detectable via route status and HCP conditions.
- **NodePort port conflict**: If a specific port is requested and conflicts with another
  service, the Service creation will fail. Using auto-assign (omitting `port`) avoids this.

## Support Procedures

For clusters using `spec.routerPublishing` with NodePort:

1. Verify the configured hostname is reachable from data plane nodes
2. Verify the NodePort port is accessible through any firewalls
3. Check `oc get svc router -n <hcp-namespace>` for service status
4. Check route status for `RouterCanonicalHostname` — should match the configured hostname

## Infrastructure Needed [optional]
None
