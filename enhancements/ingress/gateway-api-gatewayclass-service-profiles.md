---
title: gateway-api-gatewayclass-service-profiles
authors:
  - "@rikatz"
  - "@gcs278"
reviewers:
  - "@rikatz"
  - "@gcs278"
  - "@rhamini3"
  - "@Thealisyed"
  - "@candita"
approvers:
  - "@Miciah"
  - "@candita"
api-approvers:
  - TBD
creation-date: 2026-04-28
last-updated: 2026-09-30
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/NE-2698
see-also:
  - "/enhancements/ingress/gateway-api-with-cluster-ingress-operator.md"
  - "/enhancements/ingress/gateway-api-without-olm.md"
replaces: []
superseded-by: []
---

# OpenShift-Managed GatewayClass Service Profiles

## Summary

This enhancement proposes the creation of three new
OpenShift-managed GatewayClasses (`openshift-external`,
`openshift-internal`, and `openshift-clusterip`) that allow users
to provision Gateway instances with different service topologies.
Each GatewayClass maps to a specific service type and
configuration: external LoadBalancer, internal LoadBalancer, or
ClusterIP. The Cluster Ingress Operator (CIO) passes the service
configuration to the sail-operator library, which provisions a
GatewayClass defaults ConfigMap following the Istio GatewayClass
defaults mechanism. CIO then creates the GatewayClasses
automatically when `openshift-default` is created. This mirrors
the service customization that CIO already provides for
IngressControllers, but applied to Gateway API.

## Motivation

Today, when a user creates a Gateway with the `openshift-default`
GatewayClass, the provisioned service is always a LoadBalancer
with external scope. There is no way for users to request a
Gateway with an internal LoadBalancer or a ClusterIP service
without manually patching the service after creation, which is
fragile and not supported.

Cluster administrators need the ability to provision Gateways with
different service topologies depending on their use case: external
traffic, internal-only traffic, or cluster-internal communication.
This is a common pattern in other Kubernetes distributions, where
different GatewayClasses map to different service configurations.

OpenShift’s longer-term goal is a typed API that lets cluster admins
configure supported Service, deployment, scaling, placement, and proxy
settings. This enhancement is Phase 1: curated GatewayClass
profiles address established Service requirements without exposing
Istio configuration. Phase 2, a follow-on enhancement, can extend this approach
with an OpenShift-owned API that lets cluster admins combine supported
customization settings and specify values that fixed profiles cannot
express, such as resource requests and replica counts.

The existing `openshift-default` GatewayClass will not be modified
to preserve backward compatibility. Instead, three new
GatewayClasses will be introduced.

There is a goal to backport this feature to previous OCP versions
because this is a desired capability for existing deployments.

### User Stories

#### Story 1: External LoadBalancer Gateway

As a cluster administrator, I want to create a Gateway with an
external LoadBalancer service including platform-specific
annotations (e.g., AWS NLB annotations, health check
configuration, `externalTrafficPolicy`), so that I can expose
applications to external traffic with the correct cloud provider
configuration without manual service patching.

#### Story 2: Internal LoadBalancer Gateway

As a cluster administrator, I want to create a Gateway with an
internal LoadBalancer service, so that I can expose applications
only within my cloud provider's internal network (e.g., VPC)
using the same platform-specific annotations that CIO applies to
internal IngressControllers.

#### Story 3: ClusterIP Gateway

As a cluster administrator, I want to create a Gateway with a
ClusterIP service, so that I can use Gateway API for
cluster-internal traffic or expose the Gateway through an existing
OpenShift Route without provisioning another load balancer.

This supports patterns such as the ClusterIP Gateway behind an
OpenShift Route documented by
[Models-as-a-Service](https://opendatahub-io.github.io/models-as-a-service/v2.0.1/configuration-and-management/gateway-patterns/).

#### Story 4: Operations at Scale

As a platform engineer managing multiple clusters, I want the
GatewayClass-based service customization to be declarative and
automated by CIO, so that I can rely on consistent service
configurations across clusters without manual intervention, and
monitor the provisioned Gateways through existing telemetry.

### Goals

- Customize service creation based on GatewayClass name. Create
  three new GatewayClasses: `openshift-external`,
  `openshift-internal`, and `openshift-clusterip`.
- Define new GatewayClasses so existing environments will not
  break. The `openshift-default` GatewayClass will not be
  modified.
- Define what each GatewayClass represents:
  - `openshift-external`: mirrors CIO service provisioning for
    external LoadBalancers, including platform-specific
    annotations (e.g., AWS NLB annotations, health check
    settings), `externalTrafficPolicy`, and the OVN
    `local-with-fallback` annotation when applicable. See
    [Service Configuration Details](#service-configuration-details)
    for the full list.
  - `openshift-internal`: same approach but for internal
    LoadBalancers, including the platform-specific internal
    annotation (e.g.,
    `service.beta.kubernetes.io/aws-load-balancer-internal` on
    AWS, `cloud.google.com/load-balancer-type: Internal` on GCP).
  - `openshift-clusterip`: service type ClusterIP, no
    LoadBalancer provisioned.
- Reserve the `openshift-*` naming prefix for OpenShift
  GatewayClasses (requires `controllerName:
  openshift.io/gateway-controller/v1`).
- Provisioning any of these GatewayClasses with the OpenShift
  controller name kicks the provisioning of Istio/OSSM (same as
  today with `openshift-default`).
- Existing telemetry already covers these new GatewayClasses
  since we collect `controllerName` to determine OCP Gateway API
  usage.
- Reuse as much as possible the existing CIO service
  customization functions (from IngressController service
  provisioning) for building the GatewayClass defaults, rather
  than creating new logic from scratch.
- Support backporting to previous OCP versions.
- Support the Models-as-a-Service pattern of exposing a ClusterIP
  Gateway through an OpenShift Route, using existing ingress
  infrastructure without provisioning an additional load balancer.
- Establish a foundation for a future OpenShift API that lets
  administrators combine supported GatewayClass customization settings.

### Non-Goals

- Fan out many GatewayClasses for every service customization
  permutation. We will stick with 3 new GatewayClasses. Any
  further customization should be discussed separately.
- Expose customization of Gateway deployment options (nodeSelector, replicas,
  resource limits, etc.). These will be addressed by the customization
  API in a follow-on enhancement.
- Support NodePort service type as a dedicated GatewayClass. This
  is a stretch goal and the approach needs discussion (e.g., a
  ClusterIP GatewayClass with a manually created NodePort
  service).
- Support or block users directly referencing Istio patch ConfigMaps through
  `Gateway.spec.infrastructure.parametersRef`.

## Proposal

The Cluster Ingress Operator will be extended to recognize three
new GatewayClass names (`openshift-external`,
`openshift-internal`, `openshift-clusterip`) in addition to the
existing `openshift-default`. When a user creates one of these
GatewayClasses with `controllerName:
openshift.io/gateway-controller/v1`, CIO will:

1. Provision Istio/OSSM the same way it does for
   `openshift-default`.
2. Pass the service configuration as `json.RawMessage` to the
   sail-operator library, which provisions a GatewayClass
   defaults ConfigMap following the Istio mechanism documented at
   https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/#gatewayclass-defaults.
3. Create the GatewayClass resources (if they do not already
   exist) with the correct `controllerName`.

This approach is consistent with how CIO already provisions
automatic HPA for Gateways managed by CIO.

One ConfigMap is created per GatewayClass (not per Gateway
instance). All Gateways referencing a given GatewayClass share
the same default configuration.

OpenShift may add or revise profile defaults to address defects,
security issues, or platform requirements. Changes must preserve the
profile's documented Service type and exposure scope and be evaluated
for compatibility with existing Gateways. Changes that cannot preserve
compatibility require a separate profile or an explicit migration strategy.

Phase 1 adds predefined Service configurations and the proxy configuration
required to support them through class-default ConfigMaps; it does not
introduce a customization CRD or generate
`Telemetry` or `EnvoyFilter` resources. GatewayClass selection hides the
configuration mechanism, allowing a future customization API to use
other implementation resources without exposing them to users.

CIO automatically creates the three new GatewayClasses when
`openshift-default` is created (or during upgrade). This keeps
the Gateway API enablement workflow unchanged: the cluster
administrator creates `openshift-default`, and CIO provisions
the additional classes as part of the enablement process. On bare-metal
clusters, CIO omits `openshift-internal` as described below.

While Gateway API remains enabled, CIO recreates deleted profile
GatewayClasses and restores any missing defaults ConfigMaps.

### Workflow Description

**cluster administrator** is a human user responsible for managing
the cluster and Gateway infrastructure.

**application developer** is a human user responsible for
deploying applications and creating routes.

1. The cluster administrator creates the `openshift-default`
   GatewayClass with `spec.controllerName:
   openshift.io/gateway-controller/v1` (existing enablement
   workflow, unchanged).
2. CIO detects `openshift-default` and provisions Istio/OSSM
   (same as today).
3. The sail-operator library (or Sail Operator, depending on the
   approach) creates the GatewayClass defaults ConfigMap for
   each new class, with the appropriate service type and
   platform-specific annotations. CIO passes the required
   configuration as `json.RawMessage`, the same way it does for
   HPA provisioning.
4. CIO verifies each ConfigMap and its class-selection label before
   creating the class, helping prevent unintended exposure or
   misconfiguration.
5. After verification, CIO creates the `openshift-external`, `openshift-internal`,
   and `openshift-clusterip` GatewayClasses (if they do not
   already exist) with the same `controllerName`, omitting
   `openshift-internal` on bare-metal clusters.
6. The cluster administrator creates a Gateway referencing one
   of the available GatewayClasses (e.g.,
   `gatewayClassName: openshift-external`).
7. Istio provisions the Gateway with an Envoy deployment and a
   service matching the defaults from the ConfigMap.
8. CIO manages DNS for the Gateway listeners (same as today,
   except for `openshift-clusterip` which gets no DNS).
9. The application developer creates an HTTPRoute attached to
   the Gateway.

The same workflow applies for `openshift-internal` (internal
LoadBalancer) and `openshift-clusterip` (ClusterIP, no
LoadBalancer, no DNS).

```mermaid
sequenceDiagram
    participant Admin as Cluster Admin
    participant CIO as cluster-ingress-operator
    participant Sail as sail-operator library
    participant Istio as Istio/OSSM
    participant Envoy as Envoy Proxy
    participant Cloud as Cloud Provider

    Admin->>CIO: Create GatewayClass (openshift-default)
    CIO->>Istio: Provision Istio (if needed)
    CIO->>Sail: Pass json.RawMessage with service config
    Sail->>Sail: Create ConfigMap per GatewayClass
    Note over Sail: external: LB + platform annotations<br/>internal: LB + internal annotations<br/>clusterip: ClusterIP, no LB
    CIO->>CIO: Create openshift-external,<br/>openshift-internal, openshift-clusterip
    Note over CIO: Omit openshift-internal on bare metal

    Admin->>Istio: Create Gateway (class: openshift-external)
    Istio->>Envoy: Deploy Envoy proxy
    Istio->>Cloud: Create Service (per ConfigMap defaults)
    CIO->>Cloud: Create DNS records for listeners

    Note over Admin,Cloud: No DNS for openshift-clusterip
```

### API Extensions

This enhancement introduces a ValidatingAdmissionPolicy (VAP)
that enforces naming and ownership rules for GatewayClasses using
the OpenShift controller name.

#### ValidatingAdmissionPolicy for GatewayClass Naming

A ValidatingAdmissionPolicy will be created to enforce the
following rules:

1. **OpenShift controller name requires `openshift-` prefix**: If
   a GatewayClass specifies `controllerName:
   openshift.io/gateway-controller/v1`, its name must be prefixed
   with `openshift-`. A GatewayClass with the OpenShift controller
   name but without the `openshift-` prefix will be rejected.

2. **`openshift-` prefix is reserved**: The `openshift-` prefix
   for GatewayClass names is reserved for OpenShift-managed
   classes. A GatewayClass with the `openshift-` prefix but a
   different `controllerName` will be rejected.

3. **Allowlisted names only**: Only the following GatewayClass
   names are allowed when using the OpenShift controller name:
   - `openshift-default`
   - `openshift-external`
   - `openshift-internal`
   - `openshift-clusterip`

   Any other `openshift-*` name will be rejected.

The ValidatingAdmissionPolicyBinding must set
`validationActions: [Deny]` (hard error, blocks creation) to
prevent misconfiguration. Warnings are not sufficient because
a misconfigured GatewayClass with the `openshift-*` prefix
would be silently ignored by CIO, leading to user confusion.

Example VAP (simplified):

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: gatewayclass-openshift-naming
spec:
  failurePolicy: Fail
  matchConstraints:
    resourceRules:
    - apiGroups:
      - gateway.networking.k8s.io
      apiVersions:
      - v1
      operations:
      - CREATE
      resources:
      - gatewayclasses
  validations:
  - expression: >-
      !(object.spec.controllerName ==
        'openshift.io/gateway-controller/v1' &&
        !object.metadata.name.startsWith('openshift-'))
    message: >-
      GatewayClasses with controllerName
      'openshift.io/gateway-controller/v1' must have a name
      prefixed with 'openshift-'.
  - expression: >-
      !(object.metadata.name.startsWith('openshift-') &&
        object.spec.controllerName !=
        'openshift.io/gateway-controller/v1')
    message: >-
      The 'openshift-' prefix is reserved for OpenShift-managed
      GatewayClasses and requires controllerName
      'openshift.io/gateway-controller/v1'.
  - expression: >-
      !(object.spec.controllerName ==
        'openshift.io/gateway-controller/v1' &&
        !(object.metadata.name in ['openshift-default',
          'openshift-external', 'openshift-internal',
          'openshift-clusterip']))
    message: >-
      Only the following GatewayClass names are allowed with
      the OpenShift controller: openshift-default,
      openshift-external, openshift-internal,
      openshift-clusterip.
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: gatewayclass-openshift-naming
spec:
  policyName: gatewayclass-openshift-naming
  validationActions: [Deny]
```

The VAP and its corresponding ValidatingAdmissionPolicyBinding
will be managed by CIO and deployed when the GatewayAPI feature
is enabled.

The new GatewayClasses are instances of the existing
`gateway.networking.k8s.io/v1` GatewayClass resource. The
ConfigMaps created by CIO are standard Kubernetes ConfigMaps.

#### GatewayClass Customization Status

CIO reports a `Customized` condition on each new GatewayClass.
It checks that the expected class-default ConfigMap exists in
`openshift-ingress` and has the
`gateway.istio.io/defaults-for-class: <gatewayclass-name>` label.

- `True / Configured`: the expected ConfigMap and label are present.
- `False / ConfigurationMissing`: the expected ConfigMap is absent or
  its class-selection label is missing or incorrect.
- `Unknown / Pending`: CIO has not yet checked the configuration.

The condition message identifies the ConfigMap by namespace and name
so users can inspect it. The ConfigMap is managed implementation output,
not a supported customization interface.

Example status for `openshift-external`:

```yaml
status:
  conditions:
    - type: Customized
      status: "True"
      reason: Configured
      message: >-
        ConfigMap
        openshift-ingress/openshift-gatewayclass-external-config
        is present and associated with GatewayClass openshift-external.
        Inspect the ConfigMap for configuration details.
```

This condition verifies configuration presence and association; it does
not indicate that Istio has applied the configuration to individual
Gateways. The condition can extend to resources generated by the
Phase 2 customization API.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement applies to Hypershift with no additional
considerations beyond existing Gateway API support. The
GatewayClass defaults ConfigMap and Gateway provisioning happen
on the guest cluster where CIO and Istio run. Platform-specific
annotations in the ConfigMap must match the guest cluster's
infrastructure platform.

#### Standalone Clusters

This enhancement is directly applicable to standalone clusters.
The platform-specific annotations in the GatewayClass defaults
ConfigMap will be derived from the cluster's infrastructure
platform, the same way CIO derives annotations for
IngressController services.

#### Bare-metal Clusters

`openshift-clusterip` requires no load balancer integration.
`openshift-external` requires a configured LoadBalancer implementation,
such as MetalLB; address allocation and network reachability depend on
that implementation's configuration.

CIO does not create `openshift-internal` on bare-metal clusters.
Unlike supported cloud platforms, CIO has no mechanism to select
internal-only exposure on bare metal. Providing this profile requires
a defined integration with the load balancer's address pools and
network configuration.

#### Single-node Deployments or MicroShift

No additional resource consumption beyond what a Gateway already
requires. The `openshift-clusterip` GatewayClass is useful for
single-node deployments where external LoadBalancers may not be
available.

MicroShift has its own Gateway API support and does not use CIO,
so this enhancement does not directly affect MicroShift.

#### OpenShift Kubernetes Engine

This enhancement works on OKE clusters the same way as on OCP
clusters, provided Gateway API is available (which depends on the
OLM/Helm-based Istio installation being available on OKE, as
addressed by the gateway-api-without-olm enhancement).

### Implementation Details/Notes/Constraints

#### Service Configuration Details

`openshift-default` retains its existing external LoadBalancer behavior,
including `externalTrafficPolicy: Cluster`. In contrast,
`openshift-external` uses CIO's platform-specific Service configuration:
`Local` on most platforms to preserve source IPs, but `Cluster` on IBM
Cloud and Power VS. Selecting `openshift-external` is therefore an opt-in
to different traffic handling, not merely an alias for `openshift-default`.

The GatewayClass defaults ConfigMap must apply the platform-specific
Service defaults that CIO uses for IngressControllers, excluding
user-specified customizations. The following summarizes these defaults,
based on the existing load balancer service
provisioning code in `cluster-ingress-operator`:

**Common to all platforms (external LoadBalancer):**
- `externalTrafficPolicy: Local` on all platforms except IBM Cloud and
  Power VS, which use `Cluster`.
- `traffic-policy.network.alpha.openshift.io/local-with-fallback: ""`
  annotation when `externalTrafficPolicy: Local` is set (OVN
  local-with-fallback support)

**AWS:**
- External: NLB type
  (`service.beta.kubernetes.io/aws-load-balancer-type: nlb`),
  health check annotations (interval `10`, timeout `4`,
  unhealthy threshold `2`, healthy threshold `2`)
- Internal: adds
  `service.beta.kubernetes.io/aws-load-balancer-internal: "true"`
- NLB protocol: sets
  `service.beta.kubernetes.io/aws-load-balancer-target-group-attributes: "preserve_client_ip.enabled=false,proxy_protocol_v2.enabled=true"`
  to avoid the hairpin connection failures described in
  [OCPBUGS-63219](https://redhat.atlassian.net/browse/OCPBUGS-63219).
  The GatewayClass defaults ConfigMap also patches the generated Deployment's
  `spec.template.metadata.annotations` with
  `proxy.istio.io/config: '{"gatewayTopology":{"proxyProtocol":{}}}'`
  so Istio configures Envoy to receive PROXY protocol. This applies to
  both new AWS LoadBalancer profiles; `openshift-default` remains unchanged.
  See [Istio PROXY protocol configuration](https://istio.io/latest/docs/ops/configuration/traffic-management/network-topologies/#proxy-protocol).
- Resource tags: propagates cluster-defined tags from
  `Infrastructure.status.platformStatus.aws.resourceTags`, when present,
  through `service.beta.kubernetes.io/aws-load-balancer-additional-resource-tags`
  as comma-separated `key=value` pairs, following existing
  IngressController behavior. See
  [AWS resource tagging enhancement](../api-review/custom-tags-aws.md).
- Dual-stack: sets `ipFamilyPolicy: RequireDualStack` and orders
  `ipFamilies` to match the cluster's primary address family, following
  existing IngressController behavior. See
  [AWS dual-stack enhancement](aws-dual-stack-support-for-ingresscontrollers.md).

**Azure:**
- Internal:
  `service.beta.kubernetes.io/azure-load-balancer-internal: "true"`

**GCP:**
- Internal:
  `cloud.google.com/load-balancer-type: "Internal"` and
  `networking.gke.io/internal-load-balancer-allow-global-access: "false"`
  (same-region client access, following the IngressController default).

**IBM Cloud / Power VS:**
- External:
  `service.kubernetes.io/ibm-load-balancer-cloud-provider-ip-type: "public"`
- Internal:
  `service.kubernetes.io/ibm-load-balancer-cloud-provider-ip-type: "private"`
- `externalTrafficPolicy: Cluster` (IBM-specific)

**OpenStack:**
- Internal:
  `service.beta.kubernetes.io/openstack-internal-load-balancer: "true"`

**ClusterIP class:** no LoadBalancer annotations, service type
is `ClusterIP`.

The implementation must reuse the same annotation-derivation
logic that CIO already uses for IngressController services to
ensure consistency.

#### Out-of-scope Service Configurations

Phase 1 does not support the following settings, which require inputs
beyond the predefined profiles:

**Common:**

- Load balancer source ranges.

**AWS:**

- Explicit subnet selection.
- Elastic IP allocations.
- Custom security groups.
- Classic LoadBalancer selection and connection idle timeout.

**GCP:**

- Internal LoadBalancer global access.

**IBM Cloud / Power VS:**

- Optional PROXY protocol.

**OpenStack:**

- Floating IP selection.

These may be considered in the follow-on customization API.

#### Code Changes Required

1. **GatewayClass recognition**: Extend the
   gatewayclass-controller to recognize the three new GatewayClass
   names in addition to `openshift-default`. All four classes use
   the same `controllerName:
   openshift.io/gateway-controller/v1`.

2. **ConfigMap provisioning**: The sail-operator library (or Sail
   Operator) provisions the GatewayClass defaults ConfigMap,
   following the mechanism defined at
   https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/#gatewayclass-defaults.
   CIO passes the required service configuration as
   `json.RawMessage` to the sail-operator library, the same
   pattern used for HPA provisioning today.
   Each ConfigMap is placed in Istio's root namespace
   (`openshift-ingress`) and labeled
   `gateway.istio.io/defaults-for-class: <gatewayclass-name>`.

3. **Platform-specific annotations**: CIO derives the
   platform-specific service annotations from the cluster
   infrastructure, reusing the same code path as IngressController
   service provisioning.

4. **DNS management**: For `openshift-external` and
   `openshift-internal`, CIO manages DNS the same way as for
   `openshift-default`. For `openshift-clusterip`, no DNS records
   are created since there is no external endpoint.

5. **Reserved naming**: The `openshift-*` prefix is reserved for
   OpenShift-managed GatewayClasses. CIO should reject or ignore
   GatewayClasses with this prefix that do not use the correct
   `controllerName`.

6. **Customization status**: CIO watches the class-default ConfigMaps
   and reconciles `Customized` on each new GatewayClass based
   on ConfigMap presence and its class-selection label, preserving
   conditions owned by other controllers.

This enhancement does not require a new feature gate. It is a CIO
behavioral change that extends existing Gateway API support.

#### Path to Phase 2

The follow-on enhancement would introduce a typed OpenShift customization
CRD targeting a GatewayClass. The predefined profiles could remain as
base defaults, with cluster admins using the API to override supported
defaults and specify additional settings, such as proxy replicas and
resource requests and limits.

CIO would combine the profile defaults with the declared customization
and translate the resulting configuration into implementation resources.
The `Customized` condition could still be leveraged for
configuration discoverability.

### Risks and Mitigations

**Risk**: Missing or incorrect profile configuration could expose a
Gateway intended to be internal-only or cluster-only.

**Mitigation**: Verify ConfigMap presence and association before creating
each GatewayClass, reconcile missing configuration, report failures
through `Customized`, and test the resulting Service configuration.
Presence checks do not verify that Istio applied the patch correctly,
and reconciliation does not eliminate the window after ConfigMap deletion.

**Risk**: Users create GatewayClasses with `openshift-*` names
but incorrect `controllerName`.

**Mitigation**: CIO only acts on GatewayClasses with both a
recognized name and the correct `controllerName`. Unrecognized
combinations are ignored.

**Risk**: Platform-specific annotations diverge from what CIO
applies to IngressController services.

**Mitigation**: Reuse the same annotation-derivation code that
CIO uses for IngressController services.

**Risk**: `externalTrafficPolicy: Local` changes load balancer traffic
distribution. With MetalLB BGP, only nodes with local Service endpoints
advertise the address, and uneven proxy placement can produce uneven
per-pod traffic. See [MetalLB traffic policies](https://metallb.io/usage/#traffic-policies).

**Mitigation**: Validate traffic distribution and advertisement changes
during proxy rollout and endpoint loss on supported MetalLB/network
combinations. The `local-with-fallback` annotation allows forwarding to
another node when no local endpoint exists; it does not control MetalLB
advertisements or replace this validation. This enhancement does not
make proxy readiness depend on application backend health.

**Risk**: The Istio GatewayClass defaults ConfigMap mechanism
changes or is removed in a future Istio version.

**Mitigation**: The ConfigMap mechanism is documented and
supported by Istio. Monitor upstream changes and adapt if needed.

### Drawbacks

- Adds three GatewayClasses that users must understand and choose from.
  Supporting more configuration combinations could increase this
  complexity and the maintenance burden.
- Once introduced, profile defaults become a compatibility commitment.
  Changes are limited to those demonstrated to preserve existing
  customer behavior.
- Configuration applies to every Gateway using a class. Per-Gateway
  differences require separate GatewayClasses.
- Gateways reference a class, not its configuration. Troubleshooting may
  require inspecting GatewayClass status and the generated configuration.
- Depends on Istio's ConfigMap-based GatewayClass defaults mechanism.
  If that mechanism changes or is removed, the implementation must adapt.

## Open Questions

1. Should NodePort be supported as a stretch goal? If so, should
   it be a separate GatewayClass, or should users create an
   `openshift-clusterip` Gateway and manually create a NodePort
   Service? The proposed approach is ClusterIP + a manual NodePort
   Service, with documented instructions for users.

2. Are there other opinionated Envoy defaults we should change when
   introducing these profiles?

3. Should a ValidatingAdmissionPolicy protect operator-managed
   GatewayClass defaults ConfigMaps from user modification and deletion,
   while allowing operator reconciliation and cleanup?

## Alternatives (Not Implemented)

### Alternative 1: Modify openshift-default Behavior

Instead of creating new GatewayClasses, modify
`openshift-default` to accept configuration parameters that
control service type.

**Reason Not Chosen**: This would break existing environments
where `openshift-default` always provisions an external
LoadBalancer. The GatewayClass-per-topology approach is the
standard pattern in Gateway API (used by other implementations)
and keeps each class self-contained.

### Alternative 2: Per-Gateway Annotations

Allow users to annotate Gateway resources with desired service
type and platform annotations.

**Reason Not Chosen**: Annotations are not validated and are
error-prone. The GatewayClass approach provides a clear contract
between the platform and the user about what service topology
will be provisioned.

### Alternative 3: Direct Istio Patch ConfigMap Generation

CIO could generate curated Istio patch ConfigMaps that users reference
through `Gateway.spec.infrastructure.parametersRef`. This provides
explicit, per-Gateway customization.
The referenced ConfigMap must be in the Gateway's namespace, requiring
copies of shared profiles when Gateways span namespaces.

**Reason Not Chosen**: Users would reference implementation-specific
resources, making migration to another Gateway implementation more
difficult. This attachment model also limits customization to settings
supported by Istio’s ConfigMap patches; it cannot accommodate other
resources, such as `Telemetry` or `EnvoyFilter`. GatewayClass profiles
hide those implementation details and provide a foundation for broader
customization.

## Test Plan

<!-- TODO: Tests must include the following labels per
dev-guide/feature-zero-to-hero.md:
- [Jira:"Networking / cluster-ingress-operator"] for the
  component
- Appropriate test type labels like [Suite:...], [Serial],
  [Slow], or [Disruptive] as needed
Reference dev-guide/test-conventions.md for details. -->

Testing will cover the following scenarios:

1. Create `openshift-default` and verify CIO automatically
   creates `openshift-external`, `openshift-internal`, and
   `openshift-clusterip` GatewayClasses with the correct
   ConfigMap for each. Verify that bare-metal clusters omit
   `openshift-internal`.
2. Create a Gateway with each new GatewayClass and verify the
   provisioned service has the correct type (LoadBalancer
   external, LoadBalancer internal, ClusterIP), annotations, and
   platform-specific `externalTrafficPolicy`.
3. Verify that `openshift-default` behavior is unchanged
   (regression test), including `externalTrafficPolicy: Cluster`.
4. Verify that DNS records are created for `openshift-external`
   and `openshift-internal` but not for `openshift-clusterip`.
5. Verify that deleting a profile GatewayClass results in its
   recreation, with the associated defaults ConfigMap present
   before the class is recreated.
6. Test on multiple platforms (AWS, Azure, GCP, vSphere) to
   verify platform-specific annotations are correct.
7. Test upgrade and downgrade scenarios.
8. Verify `Customized` reflects ConfigMap presence and the
   class-selection label, identifies the ConfigMap in its message,
   and recovers after the ConfigMap or label is restored.
9. Verify end-to-end connectivity through a Gateway and HTTPRoute
   for each profile, using an external client, a private-network
   client, or an in-cluster client as appropriate.

## Graduation Criteria

<!-- TODO: Refer to dev-guide/feature-zero-to-hero.md for
promotion requirements: minimum 5 tests, 7 runs per week, 14
runs per supported platform, 95% pass rate, and tests running
on all supported platforms (AWS, Azure, GCP, vSphere, Baremetal
with various network stacks). -->

### Dev Preview -> Tech Preview

N/A. This feature targets direct inclusion as part of the
existing Gateway API support, which is already GA in 4.19.

### Tech Preview -> GA

- All test scenarios pass consistently across supported
  platforms.
- Platform-specific annotation coverage verified for AWS, Azure,
  GCP, and vSphere.
- Documentation created in openshift-docs covering the new
  GatewayClasses and their use cases.
- Backport plan finalized.

### Removing a deprecated feature

N/A.

## Upgrade / Downgrade Strategy

### Upgrade

Clusters upgrading to OCP 5.1 where `openshift-default` already
exists will have the three new GatewayClasses automatically
created by CIO during the upgrade, except that bare-metal clusters omit
`openshift-internal`. The `openshift-default`
GatewayClass continues to work as before.

This enhancement does not schedule deprecation or removal of the static
profiles or their ConfigMaps. Any transition to the Phase 2 API must
define its own lifecycle and migration plan; introducing that API alone
does not require users to change existing Gateways.

### Downgrade

The downgrade behavior for Gateways using the new GatewayClasses
needs to be defined. Considerations include:

- Gateways created with `openshift-external`,
  `openshift-internal`, or `openshift-clusterip` GatewayClasses
  will remain on the cluster after downgrade, but the older CIO
  version will not recognize them.
- The GatewayClass defaults ConfigMap will not be managed by the
  older CIO. The ConfigMap may be left orphaned, or it may be
  garbage collected depending on owner references.
- Existing Gateway services may lose their customized
  configuration on the next Istio reconciliation if the ConfigMap
  is removed.
- Both scenarios (Gateways staying with degraded behavior, or
  Gateways being cleaned up by Istio) need to be tested during
  implementation to determine the actual behavior and define the
  supported downgrade procedure.

## Version Skew Strategy

No new version skew concerns. CIO and Istio versions remain
synchronized through the OCP release. The ConfigMap-based
GatewayClass defaults mechanism is handled entirely by Istio,
and CIO only creates the ConfigMap.

## Operational Aspects of API Extensions

This enhancement introduces a ValidatingAdmissionPolicy (VAP)
for GatewayClass naming enforcement.

- The VAP only applies to GatewayClass CREATE operations.
  GatewayClass `name` and `controllerName` are immutable after
  creation, so UPDATE validation is not needed.
- The VAP uses CEL expressions evaluated in-process by the API
  server. There is no external webhook, no network dependency,
  and no additional latency beyond the CEL evaluation itself.
- If the VAP is removed or misconfigured, GatewayClasses with
  non-standard names could be created. CIO would ignore them
  (it only acts on recognized names), but the naming convention
  would not be enforced.
- The NID (Networking, Ingress, and DNS) team is responsible for
  the VAP and should be contacted for escalation.

### Failure Modes

- If CIO fails to create the GatewayClass defaults ConfigMap,
  Gateways referencing that class will be provisioned with
  Istio's default behavior (LoadBalancer, no platform
  annotations). CIO should set a condition on the GatewayClass
  to indicate the failure.
- If the ConfigMap is deleted manually, CIO should recreate it
  on the next reconciliation.
- If the ValidatingAdmissionPolicy is deleted, naming
  enforcement is lost but Gateway provisioning continues to
  work. CIO should recreate the VAP on the next reconciliation.

## Support Procedures

Check the GatewayClass defaults ConfigMap exists:
```bash
oc -n openshift-ingress get configmap \
  -l gateway.istio.io/defaults-for-class
```

Check the ValidatingAdmissionPolicy is in place:
```bash
oc get validatingadmissionpolicy \
  gatewayclass-openshift-naming
```

Check CIO logs for GatewayClass reconciliation:
```bash
oc -n openshift-ingress-operator logs \
  deployment/ingress-operator | grep -i gatewayclass
```

Verify the service created for a Gateway has the expected type
and annotations:
```bash
oc -n <gateway-namespace> get svc <gateway-name> -o yaml
```

## Infrastructure Needed

No new infrastructure required.
