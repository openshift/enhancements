---
title: gateway-api-gateway-customization
authors:
  - "@gcs278"
reviewers:
  - "@Miciah"
  - "@candita"
  - "@rikatz"
approvers:
  - "@Miciah"
api-approvers:
  - TBD
creation-date: 2026-09-01
last-updated: 2026-09-03
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/NE-2698
see-also:
  - "/enhancements/ingress/gateway-api-with-cluster-ingress-operator.md"
  - "/enhancements/ingress/gateway-api-crd-management-mode.md"
replaces: []
superseded-by: []
---

# Gateway API Gateway Customization

## Summary

This enhancement introduces `GatewayParameters`, a new cluster-scoped
CRD in the `operator.openshift.io` API group, and establishes the first
OpenShift-native configuration layer for Gateway API infrastructure
customization. A `GatewayClass` references a `GatewayParameters`
instance via `spec.parametersRef`, and the Cluster Ingress Operator
(CIO) reconciles it into the implementation-specific configuration —
today, the OSSM GatewayClass defaults ConfigMap. The API is
implementation-agnostic: the OpenShift types abstract the underlying
mechanism so that future changes to the Gateway API implementation do
not require API changes or user migrations.

This EP establishes the plumbing and implements two concrete use cases:
ClusterIP service type and `externalTrafficPolicy: Local`. The
`GatewayParameters` CRD is designed to be extended with additional
fields (resource requests, node placement, proxy configuration) in
follow-on EPs without breaking changes to the GatewayClass or Gateway
resources.

## Motivation

OpenShift currently has no supported way to customize how a Gateway API
implementation provisions the Kubernetes Service, proxy Deployment, or
proxy configuration for a GatewayClass. OpenShift ships Gateway API via
OSSM, which supports customization through implementation-specific
mechanisms (GatewayClass defaults ConfigMap, alpha annotations), but
these are undocumented, unsupported for end users, and tied to Istio
internals. There is no OpenShift API that:

- Provides a stable, validated interface for Gateway infrastructure
  customization
- Abstracts implementation details so customizations survive Gateway
  API implementation changes
- Integrates with CIO to derive platform-specific configuration
  (cloud LB annotations, OVN settings) automatically

As a result, when a user creates a Gateway using the `openshift-default`
GatewayClass, the implementation always provisions an external
LoadBalancer service, and there is no supported path to change this.
Specific gaps that generate support exceptions today:

- ClusterIP service type for bare-metal deployments fronted by an OCP
  Route, where no hardware or software load balancer is available
- `externalTrafficPolicy: Local` for zone-aware or BGP-based topologies
  where cross-zone hops must be avoided and source IP must be preserved

Users who need these configurations must use the Istio ClusterIP alpha
annotation or manually patch the Service — both fragile approaches that
are overwritten on reconciliation and unsupported for production use.

In an ideal world, all Gateway infrastructure customization would be
standardized upstream in the Gateway API specification. The upstream
community is actively working toward some of these use cases, and where
upstream standards mature, OpenShift will align with them. However, some
customizations are unlikely to ever be standardized upstream — they are
platform-specific, operational concerns that implementations intentionally
leave to operators. For both categories, users need a supported solution
today. This EP provides a downstream OpenShift API that bridges the gap,
designed to minimize migration cost where upstream standards eventually
emerge.

### User Stories

#### Story 1: ClusterIP Gateway on Bare Metal

As a cluster administrator running on bare metal without a hardware
load balancer, I want to create a GatewayClass that provisions Gateways
with a ClusterIP service so that I can front the Gateway with an OCP
Route using the existing HAProxy ingress infrastructure. This pattern is
documented publicly by the Red Hat AI platform:
[ClusterIP Gateway with OpenShift Route (re-encrypt)](https://opendatahub-io.github.io/models-as-a-service/v2.0.1/configuration-and-management/gateway-patterns/#clusterip-gateway-with-openshift-route-re-encrypt).

#### Story 2: Cost-Efficient Gateway on Clusters with an Existing Load Balancer

As a cluster administrator whose cluster already has an IngressController
backed by a cloud load balancer, I want to create a GatewayClass that
provisions Gateways with a ClusterIP service so that I can expose Gateway
traffic through the existing OCP Route and HAProxy infrastructure rather
than provisioning a second cloud load balancer, reducing cost while
accepting the additional hop through the IngressController.

#### Story 3: Zone-Aware External Gateway with ETP Local

As a cluster administrator running a cluster across multiple
availability zones with BGP-based networking, I want to configure
`externalTrafficPolicy: Local` on my GatewayClass so that traffic
arriving in a given zone is served by the proxy pod in that same zone,
avoiding cross-zone hops and preserving source IP.


### Goals

- Provide a supported, stable OpenShift API for customizing the service
  topology of a GatewayClass (LoadBalancer, NodePort, ClusterIP) and
  endpoint traffic policy, using field names that mirror the Kubernetes
  Service API.
- Where CIO can derive platform-specific configuration from the user's
  expressed intent, it does so automatically — administrators express
  what they want, not how to achieve it on a given platform.
- The API is implementation-agnostic and upgrade-safe: customizations
  survive Gateway API implementation changes without user intervention.
- The `openshift-default` GatewayClass is unchanged.
- Establish an extensible API foundation for follow-on customization
  (resource requests, node placement) without breaking changes.

### Non-Goals

- Customizing the `openshift-default` GatewayClass.
- Exposing the OSSM GatewayClass defaults ConfigMap (`gateway.istio.io/defaults-for-class`)
  as a supported API for end users. It is an implementation detail owned
  and managed exclusively by CIO.
- Using or supporting the Istio ClusterIP alpha annotation
  (`networking.istio.io/service-type`) as a supported mechanism for
  service type customization. This annotation is an unsupported private
  API and is superseded by this enhancement.
- Automatically deriving cloud provider internal/external LB annotations.
  Administrators set these directly on the GatewayClass or Gateway.
- Per-Gateway resource overrides. `GatewayParameters` configures
  class-level defaults shared by all Gateways referencing the class.
- Gateway API implementation configuration (logging, control plane
  settings).
- `trafficDistribution` on the Service. When `externalTrafficPolicy: Local`
  is set it is superseded; for ClusterIP traffic to the Gateway it is
  a no-op for the primary use cases.
- HPA configuration (deferred to a follow-on).
- Node placement and tolerations (deferred to a follow-on).

## Proposal

A new cluster-scoped CRD `GatewayParameters`
(`operator.openshift.io/v1alpha1`) is introduced. A cluster
administrator creates a `GatewayClass` with `spec.parametersRef`
pointing to a `GatewayParameters` instance, and creates the
`GatewayParameters` CR to configure the desired service topology.

CIO watches GatewayClasses whose `spec.controllerName` matches the
OpenShift controller name. For any such GatewayClass with a
`spec.parametersRef` pointing to a `GatewayParameters` CR, CIO:

1. Reads the `GatewayParameters` CR.
2. Creates or updates a ConfigMap in the `openshift-ingress` namespace
   with the label `gateway.istio.io/defaults-for-class: <gatewayclass-name>`.
   OSSM reads this ConfigMap to apply service type and ETP to all
   Gateways referencing the class.
3. When `externalTrafficPolicy: Local` is set, automatically adds the
   OVN `local-with-fallback` annotation on applicable platforms.
4. Manages DNS when `service.type` is `LoadBalancer`.

CIO never mutates the user's `Gateway` or `GatewayClass` resources.

### Workflow Description

#### ClusterIP Gateway (bare-metal / OCP Route topology)

1. The cluster administrator creates a `GatewayParameters` CR:

   ```yaml
   apiVersion: operator.openshift.io/v1alpha1
   kind: GatewayParameters
   metadata:
     name: clusterip-params
   spec:
     service:
       type: ClusterIP
   ```

2. The cluster administrator creates a `GatewayClass` referencing it:

   ```yaml
   apiVersion: gateway.networking.k8s.io/v1
   kind: GatewayClass
   metadata:
     name: openshift-clusterip
   spec:
     controllerName: openshift.io/gateway-controller/v1
     parametersRef:
       group: operator.openshift.io
       kind: GatewayParameters
       name: clusterip-params
   ```

3. CIO creates a ConfigMap in `openshift-ingress`:

   ```yaml
   metadata:
     labels:
       gateway.istio.io/defaults-for-class: openshift-clusterip
   data:
     service: |
       spec:
         type: ClusterIP
   ```

4. The cluster administrator creates a Gateway referencing
   `openshift-clusterip`. OSSM provisions an Envoy Deployment and a
   ClusterIP Service. No cloud LB is created, no DNS is managed.

5. The cluster administrator creates an OCP Route pointing at the
   ClusterIP Service to expose the Gateway externally via HAProxy
   (see [ClusterIP Gateway with OpenShift Route (re-encrypt)](https://opendatahub-io.github.io/models-as-a-service/v2.0.1/configuration-and-management/gateway-patterns/#clusterip-gateway-with-openshift-route-re-encrypt)).

#### Zone-Aware LoadBalancer with ETP Local

1. The cluster administrator creates a `GatewayParameters` CR:

   ```yaml
   apiVersion: operator.openshift.io/v1alpha1
   kind: GatewayParameters
   metadata:
     name: zone-aware-params
   spec:
     service:
       type: LoadBalancer
       externalTrafficPolicy: LocalWithFallback
   ```

2. The cluster administrator creates a GatewayClass referencing it
   with `spec.parametersRef` (same pattern as above).

3. CIO creates the ConfigMap with `service.type: LoadBalancer`,
   `externalTrafficPolicy: Local`, and the OVN `local-with-fallback`
   annotation on applicable platforms.

4. Gateways referencing this class get a LoadBalancer Service with ETP
   Local. CIO manages DNS.

   For an internal LoadBalancer, the administrator annotates the
   GatewayClass or Gateway directly with the platform-specific annotation
   (e.g. `service.beta.kubernetes.io/aws-load-balancer-internal: "true"`
   on AWS). These annotations propagate to the provisioned Service.

### API Extensions

#### `GatewayParameters` CRD

```go
// GatewayParameters configures the infrastructure provisioned by the
// OpenShift ingress operator for a GatewayClass that references it via
// spec.parametersRef.
//
// This CRD is designed to be extended to support Gateway-level
// customization in a future release, pending upstream plumbing support
// for non-mutating per-Gateway configuration injection.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +openshift:enable:FeatureGate=GatewayClassParameters
type GatewayParameters struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   GatewayParametersSpec   `json:"spec"`
    Status GatewayParametersStatus `json:"status,omitempty"`
}

// GatewayParametersStatus is the observed state of a GatewayParameters resource.
type GatewayParametersStatus struct {
    // observedGeneration is the most recent generation observed by the operator.
    // +optional
    ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

type GatewayParametersSpec struct {
    // service configures the Kubernetes Service provisioned for Gateways
    // referencing this GatewayClass. Fields mirror the corresponding
    // Kubernetes Service spec fields.
    //
    // +optional
    Service *GatewayServiceParameters `json:"service,omitempty"`

}

// GatewayServiceParameters mirrors selected Kubernetes Service spec fields,
// allowing administrators to configure how the Gateway API implementation
// provisions the backing Service for a GatewayClass.
type GatewayServiceParameters struct {
    // type specifies the type of Kubernetes Service to provision.
    // Mirrors the Kubernetes Service spec.type field.
    //
    // LoadBalancer provisions a cloud or hardware load balancer.
    // DNS is managed automatically by the ingress operator.
    //
    // NodePort provisions a NodePort Service. No DNS is managed.
    // The administrator is responsible for configuring an external
    // load balancer. When externalTrafficPolicy is LocalWithFallback,
    // the external load balancer MUST health-check nodes via
    // Service.spec.healthCheckNodePort.
    //
    // ClusterIP provisions a ClusterIP Service accessible only within
    // the cluster. No DNS is managed. Useful for fronting with an OCP
    // Route on bare-metal clusters without a hardware load balancer.
    //
    // When omitted, defaults to LoadBalancer.
    //
    // +optional
    // +kubebuilder:validation:Enum=LoadBalancer;NodePort;ClusterIP
    Type *corev1.ServiceType `json:"type,omitempty"`

    // externalTrafficPolicy specifies how external traffic is routed to
    // Gateway proxy pods. Only meaningful when type is LoadBalancer or
    // NodePort.
    //
    // LocalWithFallback sets Kubernetes externalTrafficPolicy to Local,
    // preserving source IP and preferring the local node to avoid
    // cross-zone hops. Traffic is not dropped on nodes without a local
    // proxy pod. This is the recommended value for most deployments.
    //
    // Cluster routes traffic to any proxy pod (with SNAT). Source IP
    // is not preserved but load distribution is even regardless of pod
    // placement.
    //
    // Defaults to LocalWithFallback.
    //
    // +optional
    // +kubebuilder:default=LocalWithFallback
    // +kubebuilder:validation:Enum=LocalWithFallback;Cluster
    ExternalTrafficPolicy *GatewayExternalTrafficPolicy `json:"externalTrafficPolicy,omitempty"`
}

// GatewayExternalTrafficPolicy specifies how external traffic is routed
// to Gateway proxy pods.
// +kubebuilder:validation:Enum=LocalWithFallback;Cluster
type GatewayExternalTrafficPolicy string

const (
    // GatewayExternalTrafficPolicyLocalWithFallback sets
    // externalTrafficPolicy: Local on the backing Service and, on
    // applicable platforms, adds the OVN local-with-fallback annotation
    // to prevent traffic drops when no local proxy pod is present.
    GatewayExternalTrafficPolicyLocalWithFallback GatewayExternalTrafficPolicy = "LocalWithFallback"

    // GatewayExternalTrafficPolicyCluster routes traffic to any proxy
    // pod in the cluster (with SNAT). Source IP is not preserved.
    GatewayExternalTrafficPolicyCluster GatewayExternalTrafficPolicy = "Cluster"
)
```

#### ValidatingAdmissionPolicy

The existing VAP for GatewayClass naming enforces two symmetric rules:

1. A GatewayClass with `controllerName: openshift.io/gateway-controller/v1`
   MUST have a name prefixed with `openshift-`.
2. A GatewayClass with the `openshift-` name prefix MUST use
   `controllerName: openshift.io/gateway-controller/v1`.

The previous hardcoded allowlist of four names is removed. The
`openshift-` prefix is reserved for OpenShift-controller-managed classes.
Unmanaged GatewayClasses (no OpenShift controllerName) may use any name.

#### DNS Management

| `service.type`  | CIO manages DNS |
|-----------------|-----------------|
| `LoadBalancer`  | Yes             |
| `NodePort`      | No              |
| `ClusterIP`     | No              |

### Future API Extensions

The following fields are planned for follow-on EPs and are **not** part
of this EP. They are enumerated here to confirm the `GatewayParameters`
CRD design accommodates them without breaking changes:

- **Internal LoadBalancer**: A first-class `scope: Internal` field (or
  equivalent) that causes CIO to automatically derive and apply the
  correct cloud provider internal LB annotation for the cluster's
  platform. Today users set this annotation directly on the GatewayClass
  or Gateway.

- **`spec.deployment.resources`** (`corev1.ResourceRequirements`): Configure
  CPU and memory requests/limits for the gateway proxy containers.

- **`spec.deployment.nodePlacement`**: Node selectors, tolerations, and
  affinity rules for the proxy Deployment.

- **Gateway-level reuse**: `GatewayParameters` is designed as a potential
  `gateway.spec.infrastructure.parametersRef` target in a future release,
  enabling per-Gateway overrides without a new CRD. This requires either
  upstream plumbing support or OSSM native support for reading the CRD,
  and is explicitly out of scope for this EP.

### Implementation Details

#### OSSM ConfigMap Mechanism

CIO creates one ConfigMap per GatewayClass in the `openshift-ingress`
namespace, labeled `gateway.istio.io/defaults-for-class: <gatewayclass-name>`.
OSSM reads this label to apply the defaults to all Gateways referencing
that class. This mechanism requires OSSM >= 3.2.4 or >= 3.3.1.

The ConfigMap is owned by CIO and must not be manually edited. Direct
use of this ConfigMap as a customization mechanism by end users is
explicitly unsupported and may be overwritten at any time.

#### Platform Annotation Derivation

When `externalTrafficPolicy` is `LocalWithFallback`, CIO automatically
sets the OVN annotation `traffic-policy.network.alpha.openshift.io/local-with-fallback: ""`
on the provisioned Service on applicable platforms. This prevents traffic
from being dropped on nodes without a local proxy pod during rolling
updates or uneven pod scheduling, making `LocalWithFallback` safer than
bare Kubernetes `externalTrafficPolicy: Local`.

All other service annotations (e.g. cloud provider internal/external LB
annotations) are the administrator's responsibility and should be set
directly on the GatewayClass or Gateway resource, where they propagate
to the provisioned Service.

#### Default ExternalTrafficPolicy and Interaction with Global Patch

CIO already manages a global Sail Helm `values.gatewayClasses` patch
that applies service and deployment configuration to all CIO-managed
GatewayClasses. When the `GatewayClassParameters` feature gate is enabled, CIO sets
`externalTrafficPolicy: Local` and the OVN `local-with-fallback`
annotation in that global patch, making `LocalWithFallback` the default
for all CIO-managed GatewayClasses — including `openshift-default` —
without requiring a per-class ConfigMap for the common case. The global
default change and the opt-out mechanism (`GatewayParameters` CRD) are
introduced atomically under the same feature gate so that the default
is never changed without an opt-out being available.

The per-class `gateway.istio.io/defaults-for-class` ConfigMap (created
by CIO when a `GatewayParameters` CR is present) takes precedence over
the global patch for that specific class. A `GatewayParameters` CR with
`externalTrafficPolicy: Cluster` therefore produces a per-class ConfigMap
that overrides the global `LocalWithFallback` default for that class only.

This interaction must be confirmed against the OSSM/Sail implementation
to ensure per-class ConfigMap precedence over the global patch is
guaranteed.

#### Deletion Semantics

When a `GatewayParameters` CR is deleted, CIO removes the defaults
ConfigMap but does not delete the `GatewayClass`. Deleting a GatewayClass
while Gateways reference it would orphan running workloads. CIO sets a
condition on the referencing `GatewayClass` status (`Accepted: False`,
reason: `InvalidParameters`) to notify the administrator.

#### Feature Gate

Gated behind `GatewayClassParameters` in `TechPreviewNoUpgrade`.

### Risks and Mitigations

**Risk**: OSSM version dependency for the defaults ConfigMap mechanism.

**Mitigation**: CIO checks the OSSM version at reconciliation time and
sets a condition on the referencing `GatewayClass` status with a clear
message if the version requirement is not met.

**Risk**: `externalTrafficPolicy: Local` with MetalLB BGP can cause
traffic disruption if gateway pods are not spread across all nodes —
MetalLB withdraws BGP route advertisements from nodes without a local
proxy pod, so pod scheduling changes cause route flaps.

**Mitigation**: The field is opt-in. Documentation explicitly covers the
MetalLB interaction and the requirement for even pod distribution.
Future `nodePlacement` support (follow-on) will allow operators to
ensure pods are spread across all BGP-advertising nodes.

**Risk**: ETP Local is not appropriate as a default for existing
GatewayClasses because it could silently break customers who have
been advised by support to rely on ETP Cluster + MetalLB. Changing
an existing GatewayClass default requires a new GatewayClass
(controllerName is immutable).

**Mitigation**: `openshift-default` is unchanged. ETP Local is only
set when explicitly configured in `GatewayParameters`.

## Alternatives

### Abstracted API (IngressController-Style)

An earlier draft of this EP used a discriminated union abstraction
modelled on the IngressController `EndpointPublishingStrategy`:

```yaml
spec:
  endpointPublishingStrategy:
    type: LoadBalancerService
    loadBalancer:
      scope: External
      endpointTrafficPolicy: Local
```

This was rejected in favour of mirroring Kubernetes field names because:

- Administrators who know Kubernetes already know `service.type: ClusterIP`.
  A translation layer (`ClusterIPService`, `endpointTrafficPolicy`) adds
  cognitive overhead with no benefit.
- Mirroring stable Kubernetes API field names means the OpenShift API
  is less likely to need changes as Kubernetes evolves.
- The abstraction required an `Internal`/`External` scope discriminator
  that has no Kubernetes equivalent, forcing a hybrid of mirrored and
  invented fields in the same struct.
- Other Gateway API implementations (e.g. Envoy Gateway) use the
  mirroring approach for the same reasons.

Note: `externalTrafficPolicy` field values are OpenShift-specific
(`LocalWithFallback`, `Cluster`) rather than pure Kubernetes passthroughs,
because the OpenShift value carries additional platform behaviour (OVN
local-with-fallback) that plain Kubernetes `Local` does not. The field
name mirrors Kubernetes; the values make the delivered behaviour explicit.

### Extending the `Ingress` Singleton

Adding `spec.gatewayAPI.customGatewayClasses[]` to the existing
`operator.openshift.io/v1alpha1` `Ingress` singleton was considered.
This was rejected because:

- It conflates two concerns: CRD/controller lifecycle management
  (existing `managementMode` field) with GatewayClass infrastructure
  configuration.
- It requires CIO to own and create GatewayClasses on behalf of users,
  rather than users creating their own GatewayClasses with CIO acting
  only as a configuration reconciler.
- The `parametersRef` pattern is the Gateway API's designed extension
  point for this purpose and is already used by other implementations
  (e.g. Envoy Gateway's `EnvoyProxy` CRD).

### Hardcoded GatewayClasses

[A prior proposal](https://github.com/openshift/enhancements/pull/1990)
created fixed GatewayClasses (`openshift-external`, `openshift-internal`,
`openshift-clusterip`). This was rejected because it requires a code
change for each new configuration combination and creates a permanent
VAP allowlist maintenance burden.

### Wait for Upstream Standardization

For ClusterIP specifically, [GEP-5093 (Routability)](https://gateway-api.sigs.k8s.io/geps/gep-5093/)
proposes a standard upstream mechanism for configuring service type in
the Gateway API spec. If adopted and GA'd upstream, OpenShift could
align with it rather than maintaining a downstream API.

This alternative was rejected for the near term because GEP-5093 is in
early stages, upstream GA is likely 3–5 years away, and OpenShift
adoption would follow after that. Users need a supported solution now.
This EP is designed so that if a future upstream standard for ClusterIP
emerges, the migration path is a GatewayClass or GatewayParameters change
rather than application-level changes. Other customizations covered by
this EP (ETP, platform annotation derivation) are unlikely to ever be
standardized upstream.

### Istio ClusterIP Alpha Annotation as Interim Solution

The annotation `networking.istio.io/service-type: ClusterIP` on a
`Gateway` resource can configure a ClusterIP service today. An interim
approach of officially supporting this annotation until GEP-5093 reaches
GA was considered.

This was rejected because: the annotation is an undocumented private
Istio API that can change or be removed at any OSSM version without
notice; it gives CIO no visibility into the configured service type and
therefore cannot integrate with DNS management, condition reporting, or
future platform annotation derivation; and it does not compose with ETP
configuration. Supporting it officially would set a precedent of exposing
implementation internals as a supported API surface, which contradicts
the goal of an implementation-agnostic OpenShift configuration layer.

## Open Questions

1. **Per-class ConfigMap precedence**: Does the `gateway.istio.io/defaults-for-class`
   ConfigMap take precedence over the global `values.gatewayClasses` patch
   in all OSSM versions that support this feature? This must be confirmed
   before the opt-out path can be relied upon.

2. **GatewayClass ownership**: Should CIO require the GatewayClass to
   use the OpenShift controllerName before reconciling a referenced
   `GatewayParameters`, or reconcile for any GatewayClass that points
   to a `GatewayParameters` CR?

## Test Plan

- `[OCPFeatureGate:GatewayClassParameters]` label on all tests
- Unit tests: ConfigMap content derivation, platform annotation selection
- E2E: `ClusterIPService` with OCP Route fronting on bare-metal/vSphere
- E2E: `LoadBalancerService` External/Internal on AWS, Azure, GCP
- E2E: `endpointTrafficPolicy: Local` — source IP preservation, health
  check NodePort set
- Upgrade: `openshift-default` unaffected after upgrade

## Graduation Criteria

### Dev Preview → Tech Preview

- E2E tests on AWS and bare-metal
- OSSM version check implemented
- Documentation draft

### Tech Preview → GA

- E2E on all supported platforms
- Upgrade/downgrade tested
- openshift-docs complete

## Upgrade / Downgrade Strategy

**Upgrade**: No existing GatewayClass or Gateway is modified.
`openshift-default` behavior is unchanged.

**Downgrade**: CIO stops reconciling `GatewayParameters` CRs but does
not delete previously created ConfigMaps or GatewayClasses. Those
resources remain functional until an administrator manually removes them.

## Version Skew Strategy

CIO checks the OSSM version at reconciliation time and reports a
condition rather than creating ConfigMaps an older OSSM cannot consume.

## Infrastructure Needed

None. CIO already has the sail-operator library integration for ConfigMap
management. The new CRD is registered in `openshift/api`.
