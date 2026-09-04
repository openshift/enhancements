---
title: ovn-kubernetes-gateway-without-uplink
authors:
  - "@abhat"
reviewers:
  - "@tssurya"
  - "@muraee"
approvers:
  - TBD
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-09-04
last-updated: 2026-09-04
status: provisional
tracking-link:
  - https://github.com/openshift/api/pull/3009
---

# OVN-Kubernetes gateway without a physical uplink

## Summary

Allow an OpenShift cluster administrator to run OVN-Kubernetes in local gateway
mode when the external gateway bridge (`br-ex`) has no physical uplink. The
capability is opt-in through the `OVNKubernetesUplinkMode` feature gate and a
new `uplinkMode` field in the cluster Network Operator API. Existing clusters
and configurations retain the current requirement for a physical uplink.

## Motivation

OVN-Kubernetes normally expects the external gateway bridge to have a physical
uplink. Some environments intentionally use the host networking stack as the
gateway without connecting `br-ex` to a physical network interface. Examples
include isolated virtualization environments, nested deployments, development
and CI systems, and deployments where external connectivity is supplied by
host routes or another host-network integration.

OVN-Kubernetes supports allowing the gateway bridge to initialize without an
uplink, but OpenShift does not currently provide a supported API for selecting
that behavior. Consequently, these environments must patch rendered
configuration, use unsupported overrides, or cannot deploy the desired
topology.

### User Stories

* As a cluster administrator deploying OpenShift in an isolated or nested
  environment, I want local gateway mode to initialize without a physical
  `br-ex` uplink so that nodes can become ready while using host-managed
  connectivity.
* As an OpenShift Virtualization developer, I want to test virtual-machine
  networking on clusters whose gateway bridge has no physical uplink so that I
  can validate networking scenarios without requiring dedicated physical
  interfaces.
* As an OpenShift support engineer, I want the selected uplink policy to be
  represented in a supported API so that I can diagnose the cluster without
  relying on undocumented overrides.

### Goals

* Provide a supported, declarative way to allow `br-ex` to operate without a
  physical uplink.
* Restrict this behavior to OVN-Kubernetes local gateway mode.
* Preserve current behavior when the new API field is omitted.
* Introduce the behavior behind a feature gate while its supported use cases,
  upgrade behavior, and test coverage are validated.
* Surface invalid combinations through API validation and operator status.

### Non-Goals

* Changing the default OVN-Kubernetes gateway mode.
* Changing the default requirement for a physical uplink.
* Configuring host routes, physical interfaces, NetworkManager connections, or
  external connectivity on behalf of the administrator.
* Supporting an uplink-less gateway in shared gateway mode.
* Guaranteeing external connectivity when no uplink and no suitable host route
  exist.
* Changing the behavior of secondary external gateway bridges, secondary
  networks, or user-defined networks. A future per-Uplink API policy would be
  a separate API design.

## Proposal

Introduce the `OVNKubernetesUplinkMode` OpenShift feature gate. During the
initial development phase it is enabled only in `DevPreviewNoUpgrade`.

When the gate is enabled, expose `uplinkMode` under
`Network.spec.defaultNetwork.ovnKubernetesConfig.gatewayConfig` in the
`operator.openshift.io/v1` API. The field has two values:

* `Required`: require a physical uplink on the external gateway bridge.
* `Optional`: permit the external gateway bridge to initialize without a
  physical uplink.

The field is valid only when `routingViaHost` is `true`, which selects local
gateway mode. In this proposal, "external gateway bridge" means the primary
gateway bridge traditionally named `br-ex`; it does not apply to secondary
bridges. When the field is omitted, OpenShift expresses no opinion and retains
the platform default. The current default remains `Required`.

The Cluster Network Operator (CNO) reads the field and renders the corresponding
OVN-Kubernetes configuration. OVN-Kubernetes then applies the policy when
initializing the gateway bridge. CNO does not create an uplink or provide
external connectivity.

### Workflow Description

**Cluster administrator:**

1. Installs a cluster with the `DevPreviewNoUpgrade` feature set while the
   feature is in development preview.
2. Configures the cluster Network resource with local gateway mode and an
   optional uplink:

   ```yaml
   apiVersion: operator.openshift.io/v1
   kind: Network
   metadata:
     name: cluster
   spec:
     defaultNetwork:
       type: OVNKubernetes
       ovnKubernetesConfig:
         gatewayConfig:
           routingViaHost: true
           uplinkMode: Optional
   ```

3. Provides any host routes and connectivity required by workloads.
4. Observes CNO and OVN-Kubernetes status to confirm that the configuration was
   accepted and the node gateway initialized.

If the administrator sets `uplinkMode` while `routingViaHost` is false or
omitted, API validation rejects the configuration. If the field is absent,
the existing behavior is unchanged.

### API Extensions

The proposal extends `operator.openshift.io/v1.GatewayConfig`:

```go
// +kubebuilder:validation:XValidation:rule="!has(self.uplinkMode) ||
// (has(self.routingViaHost) && self.routingViaHost == true)",message="uplinkMode
// can only be set when routingViaHost is true"
type GatewayConfig struct {
    // Existing fields omitted.

    // uplinkMode controls whether the external gateway bridge (br-ex) requires
    // a physical uplink port.
    // +openshift:enable:FeatureGate=OVNKubernetesUplinkMode
    // +optional
    UplinkMode UplinkMode `json:"uplinkMode,omitempty"`
}

// +kubebuilder:validation:Enum:="Required";"Optional"
type UplinkMode string

const (
    UplinkModeRequired UplinkMode = "Required"
    UplinkModeOptional UplinkMode = "Optional"
)
```

The field is included only in feature-set-specific CRD schemas where the
`OVNKubernetesUplinkMode` gate is enabled. The API is additive. Omitting it
preserves existing behavior and serialized objects remain compatible with
older components.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The API is hosted in the guest cluster and reconciled by the hosted CNO. The
behavior applies to OVN-Kubernetes node gateways in the hosted cluster. No new
management-cluster component is introduced. Initial validation will focus on
standalone clusters; hosted-cluster support must be explicitly tested before
graduation to tech preview.

#### Standalone Clusters

Standalone clusters are the initial supported topology. All nodes selecting
this configuration must use local gateway mode. Mixed node-level uplink
policies are not introduced by this proposal because the API is cluster-wide.

#### Single-node Deployments or MicroShift

The change introduces no additional workload and has negligible CPU or memory
impact. SNO is a relevant use case and will be included in testing. MicroShift
does not consume the cluster Network Operator API, so exposing equivalent
MicroShift configuration is outside the initial scope.

#### OpenShift Kubernetes Engine

The feature changes cluster networking configuration and does not depend on
OpenShift Virtualization being installed. It can therefore apply to OpenShift
Kubernetes Engine where the same Network API and OVN-Kubernetes implementation
are present.

### Implementation Details/Notes/Constraints

The implementation spans three repositories:

1. `openshift/api`
   * Register the `OVNKubernetesUplinkMode` feature gate.
   * Gate the `GatewayConfig.uplinkMode` field.
   * Generate feature-set-specific CRD and OpenAPI artifacts.
   * Validate that `uplinkMode` is used only with local gateway mode.
2. `openshift/cluster-network-operator`
   * Read the field only when the feature gate is enabled.
   * Translate `Required` and `Optional` into OVN-Kubernetes configuration.
   * Report invalid or unsupported configurations in operator status.
3. `ovn-kubernetes/ovn-kubernetes`
   * Consume the rendered setting and allow gateway initialization without a
     physical uplink when requested.
   * Preserve the current requirement by default.

The field is mutable on day 2. CNO rolls out the changed OVN-Kubernetes node
configuration through its existing rollout mechanism. Moving from `Required`
to `Optional` does not remove an existing uplink; it only permits operation
without one. Moving from `Optional` to `Required` while a node lacks an uplink
causes that node's gateway initialization to fail until an uplink is provided.
Administrators are responsible for sequencing physical interface changes and
should expect temporary traffic disruption if they remove an uplink during the
transition.

The absence of a physical uplink does not imply that the node has external
connectivity. Administrators remain responsible for appropriate host routes and
interfaces. Documentation must make this distinction explicit.

### Risks and Mitigations

* **Risk:** An administrator selects `Optional` but provides no usable host
  route, leaving workloads without external connectivity.
  **Mitigation:** Document that the setting controls gateway initialization,
  not connectivity, and add diagnostics for missing routes where practical.
* **Risk:** Enabling the behavior outside local gateway mode creates an invalid
  topology.
  **Mitigation:** Enforce the relationship through CRD validation and CNO
  validation.
* **Risk:** Older CNO or OVN-Kubernetes revisions do not understand the field
  during version skew.
  **Mitigation:** Feature-set-specific schemas hide the field when unavailable;
  CNO must omit the rendered option unless both the gate and API value permit it.
* **Risk:** An uplink-less configuration receives less CI coverage than normal
  gateway topologies.
  **Mitigation:** Add a dedicated lane or targeted end-to-end scenario before
  tech preview graduation.

### Drawbacks

This adds another cluster-wide network configuration option and expands the
gateway test matrix. It also permits a node to report a successfully initialized
gateway even when external connectivity is intentionally absent, so operators
and documentation must clearly distinguish gateway readiness from external
reachability.

## Alternatives (Not Implemented)

### Continue using unsupported overrides

Users could patch rendered OVN-Kubernetes configuration. This bypasses API
validation, is vulnerable to reconciliation, is difficult to support, and does
not provide a stable contract.

### Infer optional-uplink behavior from the absence of an uplink

Automatic inference would make configuration mistakes indistinguishable from
intentional uplink-less deployments and could silently change existing failure
behavior. An explicit API value is safer.

### Make the behavior unconditional without a feature gate

An optional field with a backward-compatible default would protect existing
clusters, but it would immediately commit OpenShift to the API contract. The
feature gate allows the API, implementation, and operational behavior to be
validated before broader exposure.

## Open Questions [optional]

* Which platforms and installation methods will be supported at tech preview?
* Can the existing CNO rollout mechanism apply day-2 transitions without a
  full ovnkube-node restart, or should a restart be part of the documented
  transition?
* Which signal should distinguish an intentionally uplink-less gateway from a
  missing uplink caused by misconfiguration?
* Should hosted clusters be included at tech preview or deferred until GA?

## Test Plan

### API and unit tests

* Verify the field is absent from Default, OKD, and Tech Preview CRD schemas
  while the gate is development preview only.
* Verify the field is present in the Dev Preview schema.
* Verify accepted values are `Required` and `Optional`.
* Verify the API rejects `uplinkMode` unless `routingViaHost` is true.
* Verify omission retains the existing default behavior.

### CNO tests

* Verify `Required`, `Optional`, and omitted values render the expected
  OVN-Kubernetes configuration.
* Verify the option is ignored or rejected when the feature gate is disabled.
* Verify reconciliation and status reporting for invalid configurations.

### End-to-end tests

* Start an OVN-Kubernetes node in local gateway mode with no physical `br-ex`
  uplink and `uplinkMode: Optional`; verify node and network operator readiness.
* Verify pod-to-pod, pod-to-service, host-to-pod, and VM connectivity that does
  not require an external uplink.
* Verify host-routed external connectivity when suitable routes are provided.
* Verify `Required` fails or reports degradation when the required uplink is
  missing.
* Verify upgrades preserve the selected policy.

## Graduation Criteria

### Dev Preview -> Tech Preview

* API, CNO, and OVN-Kubernetes implementations are complete.
* API validation and unit tests pass.
* A repeatable end-to-end test validates an uplink-less local gateway.
* Upgrade behavior and supported platforms are documented.
* CNO exposes actionable status for configuration failures.
* User-facing documentation describes prerequisites and limitations.

### Tech Preview -> GA

* At least two consecutive OpenShift minor releases of CI signal demonstrate
  stable gateway initialization and networking behavior.
* Supported standalone and hosted topologies have end-to-end coverage.
* Upgrade, downgrade, and version-skew tests pass.
* Support procedures and must-gather diagnostics are documented.
* No unresolved critical or high-severity defects remain.
* The API contract and defaulting behavior receive API approval.
* The feature gate is enabled in the Default and OKD feature sets.

### Removing a deprecated feature

Not applicable. This proposal introduces a new capability.

## Upgrade / Downgrade Strategy

On upgrade, omission of `uplinkMode` continues to select the platform default.
An explicit value is preserved in the Network resource and rendered after all
components support the gated field.

Development Preview feature sets do not support normal downgrade guarantees.
Before tech preview, downgrade testing must verify that clusters using
`Optional` either reject an unsupported downgrade or can be returned to
`Required` before downgrade. Removing the feature gate while the field is set
must produce a clear validation or reconciliation error rather than silently
changing gateway behavior.

## Version Skew Strategy

CNO is responsible for translating the API value to the OVN-Kubernetes
configuration. During an upgrade it must not render the option until the target
OVN-Kubernetes payload supports it. Older API servers do not expose the field
outside enabled feature-set schemas. Mixed node revisions must continue using a
policy understood by all nodes until the rollout reaches the required version.

## Operational Aspects of API Extensions

The Network resource is cluster-scoped and reconciled by CNO. Setting
`uplinkMode: Optional` may change whether missing physical uplinks are treated
as fatal during gateway initialization. It does not create or modify physical
interfaces and does not guarantee external connectivity.

CNO and OVN-Kubernetes logs must record the selected uplink mode. CNO status
should identify invalid mode combinations and failures to apply the requested
configuration. The selected API value and operator status are collected by
standard must-gather tooling; no new persistent data is introduced.

The feature does not add API calls proportional to cluster size and does not
change API availability objectives. Reconciliation occurs through the existing
Network resource path.

## Support Procedures

Support personnel should:

1. Inspect the cluster FeatureGate resource and confirm that
   `OVNKubernetesUplinkMode` is enabled.
2. Inspect `Network/cluster` and verify that `routingViaHost` is true when
   `uplinkMode` is set.
3. Review CNO status and logs for rendering or validation errors.
4. Review OVN-Kubernetes node logs for gateway initialization failures.
5. Inspect `br-ex`, its OVS ports, host routes, and NetworkManager state on the
   affected node.
6. Confirm whether external connectivity is expected and whether the host has a
   route that can provide it.

Recovery consists of correcting host networking, setting `uplinkMode` to
`Required` after adding a physical uplink, or removing the field to restore the
platform default.

## Infrastructure Needed [optional]

CI requires a topology in which `br-ex` can be created without a physical
uplink. A nested or virtualized environment is sufficient if it can verify node
readiness and the required pod, service, host, and virtual-machine traffic
paths.
