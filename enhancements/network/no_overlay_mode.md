---
title: no_overlay_mode
authors:
  - Riccardo Ravaioli
reviewers:
  - Peng Liu
approvers:
  - Jaime Caamaño
  - Surya Seetharaman
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - Joel Speed
creation-date: 2025-07-21
last-updated: 2026-02-05
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CORENET-6133
see-also:
  - https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md

---

# No-overlay mode

## Summary

This enhancement describes how the no-overlay mode for ovn-kubernetes integrates in OpenShift. The feature allows pods in selected networks to communicate using the underlay network, without the overhead of Geneve encapsulation that we use to build the overlay network. The no-overlay mode is described in detail in the [OVN-Kubernetes upstream enhancement](https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md). This document outlines the necessary API changes, the interaction with the Cluster Network Operator (CNO) and our test plan for this feature.

## Motivation

The motivations for this feature are to be found in the [original upstream enhancement](https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md).

### User Stories
See the [upstream OVN-Kubernetes enhancement](https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#user-storiesuse-cases) for detailed user stories.

### Goals
See the [upstream OVN-Kubernetes enhancement](https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#goals) for detailed goals.

**Current platform limitation:**
BGP is only supported on Bare Metal clusters. Since no-overlay mode requires BGP, it shares this limitation for now. As BGP support expands to more platforms, so will no-overlay mode.

### Non-Goals
See the [upstream OVN-Kubernetes enhancement](https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#non-goals) for detailed non-goals.

## Proposal

The no-overlay feature largely leverages the existing BGP functionality in OVN-Kubernetes and only needs a few API changes.
The feature can be applied to:
- the default network, at cluster installation time
- Cluster User Defined Networks (CUDNs) with Layer3 topology, at run time

**Note:** Only Layer3 CUDNs support no-overlay mode. See the [upstream enhancement](https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#non-goals) for details.

For each network we are going to need a `transport` parameter that takes `Geneve` (default) or `NoOverlay`.
Then if `transport` is set to `NoOverlay`, we need the following parameters to configure the no-overlay mode:
- `outboundSNAT`:
  - `Enabled`: apply source NAT to egress traffic, allowing only the node IP to be exposed, which is today's expected behaviour unless EgressIP is used;
  - `Disabled`: do not apply any SNAT to egress traffic, thus exposing the pod subnet outside the cluster. This is the same behaviour we have today for a BGP-advertised network.
- `routing`:
  - `Managed`: delegate to OVN-Kubernetes the configuration of the BGP fabric. OVN-Kubernetes will configure the FRR instance on each node to set up an internal BGP fabric without requiring external BGP routers.
  - `Unmanaged`: use the FRRConfiguration and RouteAdvertisements provided by the cluster administrator to implement the no-overlay mode

For CUDNs these parameters are to be added to the CUDN CRD and can be configured by the cluster administrator when creating a CUDN. For the default network, these parameters must be input by the cluster administrator at installation time and passed over to ovn-kubernetes by the Cluster Network Operator.

There are two global parameters specific to the way that the no-overlay mode is to be implemented when `NoOverlayConfig.routing`=`Managed`, affecting the generated BGP configuration:
- `asNumber` (optional): the Autonomous System (AS) number to be used in the generated FRRConfiguration for the default VRF. When omitted, this defaults to 64512. If the cluster administrator is also defining BGP configuration for the default VRF through other means (e.g., directly via FRR-K8s FRRConfiguration resources or indirectly via MetalLB), the same AS number must be used. Failure to use consistent AS numbers will cause FRR-K8s to fail when merging the FRRConfiguration resources.
- `bgpTopology`:
  - `FullMesh`: every node deploys a BGP router, thus forming a BGP full mesh.
  <!-- - `routeReflector`: every master node will run a BGP route reflector, in order to reduce the number of BGP connections in the cluster; this is particularly useful for large clusters. -->

The resulting FRRConfiguration and RouteAdvertisements will be generated by OVN-Kubernetes and are described in detail in the upstream enhancement.
In this enhancement we are going to detail the workflow necessary for a cluster admin to enable this feature in OpenShift and we are going to define how the OpenShift API is to be extended to include these new parameters, which will be passed by CNO to OVN-Kubernetes at installation time.

### MTU Considerations

Networks configured in no-overlay mode utilize the provider network's MTU directly, without any encapsulation overhead. This allows pods on a no-overlay network to leverage the full MTU of the underlying physical network. In contrast, pods on the overlay network use a reduced MTU to account for encapsulation headers.

**Default network MTU:** The MTU for the default network is specified in `operator.openshift.io/v1 Network` at installation time. For example, if CNO sees that the provider network MTU is 1500 bytes and the user doesn't specify an explicit MTU:
- In **overlay mode**, the cluster network MTU will be set to 1400 bytes (1500 - 100 bytes for Geneve encapsulation overhead)
- In **no-overlay mode**, the cluster network MTU will be set to 1500 bytes (the full provider network MTU)

If the user specifies an explicit MTU in no-overlay mode, CNO validates that the configured MTU does not exceed the host MTU. If the specified MTU is greater than the host MTU, CNO will report a degraded status with an error message indicating the invalid MTU configuration.

**CUDN MTU:** The MTU for CUDNs is specified in the `spec.network.layer3.mtu` field of the ClusterUserDefinedNetwork CRD. OVN-Kubernetes validates that the configured MTU does not exceed the maximum allowed based on the CUDN's transport mode (host MTU for no-overlay, host MTU - 100 for Geneve). If the configured MTU exceeds the allowed maximum, OVN-Kubernetes will reject the CUDN configuration and report the error in the CUDN's status conditions. CNO does not provide the host MTU value to OVN-Kubernetes; OVN-Kubernetes discovers it directly from the host.

### Workflow Description

In a nutshell, the necessary steps to enable no-overlay are the following:
- enable `TechPreviewNoUpgrade` feature gate on OpenShift at installation time, as long as no-overlay mode is in tech preview.
- enable BGP for the cluster in `operator.openshift.io/v1 Network`:
  - `spec.additionalRoutingCapabilities.providers`: `FRR`
  - `spec.defaultNetwork.ovnKubernetesConfig.routeAdvertisements`: `Enabled`
- enable no-overlay mode for the default network (if desired) in `operator.openshift.io/v1 Network` and configure it with the per-network parameters:
  - `outboundSNAT`: `Enabled` or `Disabled`
  - `routing`: `Managed`, `Unmanaged`
- provide the necessary manifests for the `Unmanaged` routing scenario, if that's the preferred routing mode for either the default network (as configured above) or for CUDNs (created later on, after cluster installation):
  - FRRConfiguration CR
  - RouteAdvertisements CR
- configure parameters for the `Managed` scenario, if that's the preferred routing mode for either the default network or for CUDNs:
  - `asNumber`: int (optional) - the BGP AS number to use for the default VRF. When omitted, this defaults to 64512.
  - `bgpTopology`: `FullMesh` (required) - defines the BGP peering topology. Currently only `FullMesh` is supported, where every node deploys a BGP router and peers with all other nodes. Future enhancements may add support for route reflector topologies.
- create CUDNs with `transport`: `NoOverlay` and the desired `noOverlay` options.

No-overlay mode should be enabled at installation time (day 0) if the default network is to be affected, whereas it can be enabled at run time (day 2) if CUDNs are to be affected. In this section we will detail the workflow for each combination of network (default, CUDN) and chosen routing (`Managed`, `Unmanaged`).

#### Prerequisite: BGP
The prerequisite for no-overlay mode regardless of how it is configured is to enable BGP for the cluster. In `operator.openshift.io/v1 Network` the cluster administrator should set:
  - `spec.additionalRoutingCapabilities.providers`: `FRR`
  - `spec.defaultNetwork.ovnKubernetesConfig.routeAdvertisements`: `Enabled`

This is to be done at installation time for the default network and can be postponed to day 2 for CUDNs.
At installation time this manifest should be provided in the `manifests` folder for the installer to apply on day 0:
```yaml
apiVersion: operator.openshift.io/v1
kind: Network
metadata:
  name: cluster
spec:
  additionalRoutingCapabilities:
    providers:
    - FRR
  defaultNetwork:
    ovnKubernetesConfig:
      routeAdvertisements: Enabled
    type: OVNKubernetes
```

On day 2 we can patch `network.operator cluster` and CNO will take care of applying this change:
```sh
$ oc patch network.operator cluster --type merge --patch \
'{
   "spec":{
      "additionalRoutingCapabilities":{
         "providers":[
            "FRR"
         ]
      },
      "defaultNetwork":{
         "ovnKubernetesConfig":{
            "routeAdvertisements":"Enabled"
         }
      }
   }
}

```
#### No-overlay for the default network with unmanaged routing

On day 0, in `operator.openshift.io/v1 Network` the cluster administrator should enable no-overlay mode for the default network and configure it with the necessary per-network parameters:
  - `outboundSNAT`: `Enabled` or `Disabled`
  - `routing`: `Unmanaged`

In the prerequisite step, the cluster administrator already created a manifest for `operator.openshift.io/v1 Network` to enable BGP and placed it in the `manifests` folder. This manifest should therefore be extended to include the aforementioned parameters for the default network. For instance:
```yaml
apiVersion: operator.openshift.io/v1
kind: Network
metadata:
  name: cluster
spec:
  additionalRoutingCapabilities:
    providers:
    - FRR
  defaultNetwork:
    ovnKubernetesConfig:
      routeAdvertisements: Enabled
      transport: NoOverlay
      noOverlayConfig:
        outboundSNAT: Enabled
        routing: Unmanaged
    type: OVNKubernetes
```

The cluster administrator should also provide in the `manifests` folder the necessary manifests for the `Unmanaged` scenario:
  - FRRConfiguration CR (defines the BGP peering with the external router)
  - RouteAdvertisements CR (defines which networks should be advertised)

In total three manifests need to be provided:
```$ tree manifests/
manifests/
├── 99-frrconfig.yaml
├── 99-operator_config.yaml
└── 99-ra.yaml
```

##### Example: FRRConfiguration CR

```yaml
apiVersion: frrk8s.metallb.io/v1beta1
kind: FRRConfiguration
metadata:
  name: external-bgp
  namespace: openshift-frr-k8s
  labels:
    network: default
spec:
  bgp:
    routers:
    - asn: 64512
      neighbors:
      - address: 192.168.111.1  # External BGP router address
        asn: 64512
        disableMP: true
        toReceive:
          allowed:
            mode: filtered
```

##### Example: RouteAdvertisements CR

```yaml
apiVersion: k8s.ovn.org/v1
kind: RouteAdvertisements
metadata:
  name: default
spec:
  advertisements:
  - PodNetwork
  frrConfigurationSelector:
    matchLabels:
      network: default
  networkSelectors:
  - networkSelectionType: DefaultNetwork  # Matches the default network only
  nodeSelector: {}
```

##### Example: External BGP Router Configuration (FRR)

The following example shows one possible configuration for an external BGP router acting as a route reflector, listening on host network 192.168.111.0/24:

```conf
router bgp 64512
 bgp router-id 192.168.111.1
 bgp cluster-id 192.168.111.1
 no bgp ebgp-requires-policy
 no bgp default ipv4-unicast

 neighbor NODES peer-group
 neighbor NODES remote-as 64512
 bgp listen range 192.168.111.0/24 peer-group NODES

 address-family ipv4 unicast
  neighbor NODES activate
  neighbor NODES route-reflector-client
 exit-address-family
```

The `bgp listen range` directive enables dynamic neighbors, allowing cluster nodes to peer automatically without individual neighbor configuration.

For more details on the upstream implementation, see the [OVN-Kubernetes enhancement](https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#enable-no-overlay-mode-for-the-default-network-with-unmanaged-routing).

If there are errors in setting up no-overlay mode for the default network (e.g., missing or invalid RouteAdvertisements CR), OVN-Kubernetes will report an event and log error messages.

#### No-overlay for the default network with managed routing
In `operator.openshift.io/v1 Network` the cluster administrator should enable no-overlay mode for the default network and configure it with the necessary per-network parameters:
  - `outboundSNAT`: `Enabled` or `Disabled`
  - `routing`: `Managed`

In addition to that, the managed routing options should also be set in `operator.openshift.io/v1 Network`: `bgpTopology` (required) and optionally `asNumber` (when omitted, this defaults to 64512).
An example of the resulting `operator.openshift.io/v1 Network` is:
```yaml
apiVersion: operator.openshift.io/v1
kind: Network
metadata:
  name: cluster
spec:
  additionalRoutingCapabilities:
    providers:
    - FRR
  defaultNetwork:
    ovnKubernetesConfig:
      routeAdvertisements: Enabled
      transport: NoOverlay
      noOverlayConfig:
        outboundSNAT: Enabled
        routing: Managed
      bgpManagedConfig:
        asNumber: 64512
        bgpTopology: FullMesh
    type: OVNKubernetes
```

In total only the manifest for the above API configuration needs to provided:
```$ tree manifests/
manifests/
├── 99-operator_config.yaml
```

OVN-Kubernetes will take care of generating the necessary FRRConfiguration and RouteAdvertisements CRs.

If there are errors in setting up no-overlay mode for the default network (e.g., missing or invalid RouteAdvertisements CR), OVN-Kubernetes will report an event and log error messages.

#### No-overlay for CUDNs with unmanaged routing
The cluster administrator should enable BGP either on day 0 or on day 2, as described in the prerequisite section. Along with enabling BGP, the cluster administrator should provide at the same time two manifests:
  - FRRConfiguration CR
  - RouteAdvertisements CR

##### Example: FRRConfiguration CR for CUDN

```yaml
apiVersion: frrk8s.metallb.io/v1beta1
kind: FRRConfiguration
metadata:
  name: blue-network
  namespace: openshift-frr-k8s
  labels:
    network: blue
spec:
  bgp:
    routers:
    - asn: 64512
      neighbors:
      - address: 192.168.111.1  # External BGP router address
        asn: 64512
        disableMP: true
        toReceive:
          allowed:
            mode: filtered
```

##### Example: RouteAdvertisements CR for CUDN

```yaml
apiVersion: k8s.ovn.org/v1
kind: RouteAdvertisements
metadata:
  name: blue-network
spec:
  advertisements:
  - PodNetwork
  frrConfigurationSelector:
    matchLabels:
      network: blue  # Matches the FRRConfiguration above
  networkSelectors:
  - networkSelectionType: ClusterUserDefinedNetwork
    clusterUserDefinedNetworkSelector:
      networkSelector:
        matchLabels:
          network: blue  # Matches the CUDN further below
  nodeSelector: {}
```

##### Example: External BGP Router Configuration (FRR)

The external BGP router configuration can be the same as in the default network case. Example FRR configuration:

```conf
router bgp 64512
 bgp router-id 192.168.111.1
 bgp cluster-id 192.168.111.1
 no bgp ebgp-requires-policy
 no bgp default ipv4-unicast

 neighbor NODES peer-group
 neighbor NODES remote-as 64512
 bgp listen range 192.168.111.0/24 peer-group NODES

 address-family ipv4 unicast
  neighbor NODES activate
  neighbor NODES route-reflector-client
 exit-address-family
```

On day 2, the cluster administrator can define Layer3 CUDNs with `transport`:`NoOverlay` and further configure the no-overlay options within the CUDN CR. The CUDN must have a label that matches the `networkSelector` in the RouteAdvertisements CR. For instance:
```yaml
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: blue
  labels:
    network: blue  # Must match the networkSelector in RouteAdvertisements
spec:
  namespaceSelector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values: ["red", "blue"]
  network:
    topology: Layer3
    layer3:
      role: Primary
      mtu: 1500
      subnets:
      - cidr: 10.10.0.0/16
        hostSubnet: 24
    transport: "NoOverlay"
    noOverlay:
      outboundSNAT: "Disabled"
      routing: "Unmanaged"
```

Any error in setting up no-overlay mode for a CUDN will be reflected in the status conditions of the CUDN.

#### No-overlay for CUDNs with managed routing
The cluster administrator should enable BGP either on day 0 or on day 2, as described in the prerequisite section. In order to configure managed routing, `bgpManagedConfig` options should also be set in `operator.openshift.io/v1 Network` either at installation time (day 0) or on day 2 and is mutable thereafter. An example of the patch operation could be:

```sh
oc patch network.operator cluster --type merge --patch '
  {
    "spec": {
      "defaultNetwork": {
        "ovnKubernetesConfig": {
          "bgpManagedConfig": {
            "asNumber": 64512,
            "bgpTopology": "FullMesh"
          }
        }
      }
    }
  }'
```
OVN-Kubernetes will take care of generating the necessary FRRConfiguration and RouteAdvertisements CRs.

On day 2, the cluster administrator can define CUDNs with `transport`:`NoOverlay` and further configure the no-overlay options within the CUDN CR. For instance:
```yaml
apiVersion: k8s.ovn.org/v1
kind: ClusterUserDefinedNetwork
metadata:
  name: my-cudn
spec:
  namespaceSelector:
    matchExpressions:
    - key: kubernetes.io/metadata.name
      operator: In
      values: ["red", "blue"]
  network:
    topology: Layer3
    layer3:
      role: Primary
      mtu: 1500
      subnets:
      - cidr: 10.10.0.0/16
        hostSubnet: 24
    transport: "NoOverlay"
    noOverlay:
      outboundSNAT: "Disabled"
      routing: "Managed"
```

Any error in setting up no-overlay mode for a CUDN will be reflected in the status conditions of the CUDN.

#### Further considerations

It's important to note that there can be networks in no-overlay mode running in `Managed` mode, and networks in no-overlay mode running in `Unmanaged` mode, coexisting in the same cluster.

On day 1, during installation, CNO propagates the configuration parameters to the ovn-kubernetes components (ovnkube-control-plane and ovnkube-node) via the existing ConfigMap. OVN-Kubernetes then implements no-overlay mode according to these parameters. In particular, in `Managed` mode, OVN-Kubernetes generates the necessary FRRConfiguration and RouteAdvertisements manifests based on the provided configuration.

Extra care must be taken to ensure that the total time taken by CNO to deploy the network is not significantly increased by the additional steps necessary to configure no-overlay mode. The time taken by the network to converge should be well within the time window allocated to CNO by the Cluster Version Operator (CVO).

When a network is created in no-overlay mode, be it the default network or a CUDN, it is not possible to switch it to overlay mode. The opposite is also true: a network created in overlay mode cannot be switched to no-overlay mode. In particular:
- For the default network, the feature can only be enabled at cluster installation time and cannot be disabled afterwards.
- For CUDNs, the cluster administrator can choose between overlay and no-overlay mode at creation time, in the CUDN spec. For an existing CUDN, the mode cannot be changed unless the CUDN is deleted and recreated with the desired overlay / no-overlay mode.

### API Extensions

The following changes are to be added to the operator network configuration (`operator/v1/types_network.go`) in the `openshift/api` repository:

#### OVNKubernetesConfig struct additions

```go
// Maintainer note for NoOverlayMode feature (TechPreview):
// When NoOverlayMode graduates to GA, add '+kubebuilder:default=Geneve' to the Transport
// field so the default is visible in the CRD schema and applied by the API server automatically.
// Currently CNO handles the default (treating omitted as Geneve) because the field is feature-gated
// and existing ungated tests don't expect this field in outputs.

// ovnKubernetesConfig contains the configuration parameters for networks
// using the ovn-kubernetes network project
// +openshift:validation:FeatureGateAwareXValidation:featureGate=NoOverlayMode,rule="has(self.noOverlayConfig) == (has(self.transport) && self.transport == 'NoOverlay')",message="noOverlayConfig must be set if and only if transport is NoOverlay"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=NoOverlayMode,rule="!has(self.noOverlayConfig) || self.noOverlayConfig.routing != 'Managed' || has(self.bgpManagedConfig)",message="bgpManagedConfig is required when noOverlayConfig.routing is Managed"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=NoOverlayMode,rule="!has(oldSelf.transport) || oldSelf.transport == '' || has(self.transport)",message="transport cannot be removed once set to a non-empty value"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=NoOverlayMode,rule="!has(oldSelf.noOverlayConfig) || has(self.noOverlayConfig)",message="noOverlayConfig cannot be removed once set"
type OVNKubernetesConfig struct {
	// ... existing fields ...

	// transport describes the transport protocol for east-west traffic
	// for the default network.
	// Allowed values are "NoOverlay" and "Geneve".
	// When set to "NoOverlay", the default network operates in no-overlay mode.
	// When set to "Geneve", the default network uses Geneve overlay.
	// When omitted, this means the user has no opinion and the platform chooses a reasonable default which is subject to change over time.
	// The current default is "Geneve".
	// This field can only be set at installation time and cannot be changed afterwards.
	// +openshift:enable:FeatureGate=NoOverlayMode
	// +kubebuilder:validation:Enum=NoOverlay;Geneve
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="transport can only be set at installation time"
	// +optional
	Transport TransportOption `json:"transport,omitempty"`

	// noOverlayConfig contains configuration for no-overlay mode.
	// This configuration applies to the default (primary) network only.
	// It is required when Transport is "NoOverlay".
	// When omitted, this means the user does not configure no-overlay mode options.
	// +openshift:enable:FeatureGate=NoOverlayMode
	// +optional
	NoOverlayConfig *NoOverlayConfig `json:"noOverlayConfig,omitempty"`

	// bgpManagedConfig configures the BGP properties for networks (default network or CUDNs)
	// in no-overlay mode that specify routing="Managed" in their NoOverlayConfig.
	// It is required when NoOverlayConfig.Routing is set to "Managed".
	// When omitted, this means the user does not configure BGP for managed routing.
	// This field can be set at installation time or on day 2, and can be modified at any time.
	// +openshift:enable:FeatureGate=NoOverlayMode
	// +optional
	BGPManagedConfig BGPManagedConfig `json:"bgpManagedConfig,omitzero,omitempty"`
}
```

#### New types

```go
// TransportOption is the type for network transport options
type TransportOption string

// SNATOption is the type for SNAT configuration options
type SNATOption string

// RoutingOption is the type for routing configuration options
type RoutingOption string

// BGPTopology is the type for BGP topology configuration
type BGPTopology string

const (
	// TransportOptionNoOverlay indicates the network operates in no-overlay mode
	TransportOptionNoOverlay TransportOption = "NoOverlay"
	// TransportOptionGeneve indicates the network uses Geneve overlay
	TransportOptionGeneve TransportOption = "Geneve"

	// SNATEnabled indicates outbound SNAT is enabled
	SNATEnabled SNATOption = "Enabled"
	// SNATDisabled indicates outbound SNAT is disabled
	SNATDisabled SNATOption = "Disabled"

	// RoutingManaged indicates routing is managed by OVN-Kubernetes
	RoutingManaged RoutingOption = "Managed"
	// RoutingUnmanaged indicates routing is managed by users
	RoutingUnmanaged RoutingOption = "Unmanaged"

	// BGPTopologyFullMesh indicates every node deploys a BGP router, forming a BGP full mesh
	BGPTopologyFullMesh BGPTopology = "FullMesh"
)

// NoOverlayConfig contains configuration options for networks operating in no-overlay mode.
type NoOverlayConfig struct {
	// outboundSNAT defines the SNAT behavior for outbound traffic from pods.
	// Allowed values are "Enabled" and "Disabled".
	// When set to "Enabled", SNAT is performed on outbound traffic from pods.
	// When set to "Disabled", SNAT is not performed and pod IPs are preserved in outbound traffic.
	// This field is required when the network operates in no-overlay mode.
	// This field can be set to any value at installation time and can be changed afterwards.
	// +kubebuilder:validation:Enum=Enabled;Disabled
	// +required
	OutboundSNAT SNATOption `json:"outboundSNAT,omitempty"`

	// routing specifies whether the pod network routing is managed by OVN-Kubernetes or users.
	// Allowed values are "Managed" and "Unmanaged".
	// When set to "Managed", OVN-Kubernetes manages the pod network routing configuration through BGP.
	// When set to "Unmanaged", users are responsible for configuring the pod network routing.
	// This field is required when the network operates in no-overlay mode.
	// This field is immutable once set.
	// +kubebuilder:validation:Enum=Managed;Unmanaged
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="routing is immutable once set"
	// +required
	Routing RoutingOption `json:"routing,omitempty"`
}

// BGPManagedConfig contains configuration options for BGP when routing is "Managed".
type BGPManagedConfig struct {
	// asNumber is the 2-byte or 4-byte Autonomous System Number (ASN)
	// to be used in the generated FRR configuration.
	// Valid values are 1 to 4294967295.
	// When omitted, this defaults to 64512.
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=4294967295
	// +default=64512
	// +optional
	ASNumber int64 `json:"asNumber,omitempty"`

	// bgpTopology defines the BGP topology to be used.
	// Allowed values are "FullMesh".
	// When set to "FullMesh", every node deploys a BGP router, forming a BGP full mesh.
	// This field is required when BGPManagedConfig is specified.
	// +kubebuilder:validation:Enum=FullMesh
	// +required
	BGPTopology BGPTopology `json:"bgpTopology,omitempty"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

No-overlay mode support for Hypershift / Hosted Control Planes will be deferred to future work. The initial implementation will focus on standalone clusters only.

**Considerations for future Hypershift support:**
- The control plane runs in the management cluster while the data plane (worker nodes) runs in the hosted cluster
- BGP peering would need to occur between hosted cluster nodes, not involving the management cluster
- FRR would run on hosted cluster nodes only
- Communication between a no-overlay network on the hosted cluster and the control plane (e.g., from a pod to the API server) should likely use SNAT to translate the source IP to the node IP when traffic is destined for the management cluster. This is necessary because the management cluster operates as a separate Kubernetes cluster and cannot route traffic back to the hosted cluster's pod subnet. However, it can reach the hosted cluster via the IP addresses of its nodes.

This topic requires further discussion with the Hypershift team and will be addressed as an update to this enhancement.

#### Standalone Clusters

No special considerations for standalone clusters.

#### Single-node Deployments or MicroShift

The goal being to not encapsulate traffic between any two cluster nodes, there is no use case for this feature in true single-node deployments. For deployments with a single-node control plane and multiple worker nodes, the feature is usable in the same way as standalone clusters.

#### OpenShift Kubernetes Engine

### Implementation Details/Notes/Constraints

#### openshift/api Validation

The `openshift/api` repository defines the `operator.openshift.io/v1 Network` CRD schema, which includes:
- No-overlay configuration fields for the default network (`transport`, `noOverlayConfig`)
- BGP managed configuration (`bgpManagedConfig`) that applies to both the default network and CUDNs when using managed routing

CEL (Common Expression Language) validation rules are used to reject invalid configurations at the API server level. These rules enforce the following:

- `transport` can only be set at installation time and cannot be changed afterwards
- `noOverlayConfig` must be set if and only if `transport` is `NoOverlay`
- `noOverlayConfig` cannot be removed once set
- `noOverlayConfig.outboundSNAT` can be set to any value at installation time and can be changed afterwards
- `noOverlayConfig.routing` is immutable once set
- `bgpManagedConfig` is required when `noOverlayConfig.routing` is set to `Managed`

This ensures that misconfigurations are caught early at admission time, before CNO or OVN-Kubernetes attempt to apply them.

#### CNO Integration

The Cluster Network Operator (CNO) is responsible for:
1. Managing and deploying the CUDN CRDs, which include no-overlay configuration fields for user-defined networks
2. Deploying all updated CRDs (CUDN CRD and `operator.openshift.io/v1 Network`) when the NoOverlayMode feature gate is enabled
3. Reading the no-overlay configuration from `operator.openshift.io/v1 Network`
4. Propagating the configuration to OVN-Kubernetes via the existing configmap mechanism

#### OVN-Kubernetes Integration

OVN-Kubernetes handles no-overlay mode by:
1. Configuring the OVN topology without overlay encapsulation for affected networks
2. For `Managed` routing: generating FRRConfiguration and RouteAdvertisements CRs based on the cluster topology
3. For `Unmanaged` routing: relying on user-provided FRRConfiguration and RouteAdvertisements CRs
4. Coordinating with FRR-Kubernetes to establish BGP peerings and advertise pod subnets

#### Day-0 vs Day-2 Configuration

- **Default network**: Must be configured at installation time (day 0) via manifests provided to the OpenShift installer. The configuration is immutable after installation except for `outboundSNAT`.
- **CUDNs**: Can be configured at run time (day 2) when creating the CUDN. The transport mode is immutable after CUDN creation.
- **bgpManagedConfig**: Can be set at installation time (day 0) or on day 2 and is mutable thereafter.

For detailed implementation specifics, refer to the [upstream OVN-Kubernetes enhancement](https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md).

#### Bootstrap Node Connectivity and outboundSNAT

The `outboundSNAT` field can be set to either `Enabled` or `Disabled` at installation time. However, extra care must be taken when installing a cluster with `outboundSNAT=Disabled`.

During installation, pods need to communicate with the kube-apiserver that runs on the bootstrap node. However, the bootstrap node is not registered as a Kubernetes Node and does not participate in BGP peering, so it has no routes to pod subnets. With `outboundSNAT=Disabled`, pod traffic preserves the original pod IP and the bootstrap node cannot route return traffic back unless specific routing is in place. With `outboundSNAT=Enabled`, traffic is SNATed to the node IP, which is routable from the bootstrap node.

When `outboundSNAT=Disabled` is used at installation time, the designated cluster gateway must be BGP-enabled and configured to accept and install BGP routes from the cluster nodes. This ensures that the bootstrap node can reach pod IPs via the cluster gateway, enabling successful bootstrap operations.

After installation, the administrator can change `outboundSNAT` at any time by patching the `network.operator cluster` resource; Cluster Network Operator will detect this change and trigger a restart of ovn-kubernetes-node pods in order to apply the new configuration value.

### Risks and Mitigations

**Risk: Scale Limitations with Full Mesh**
In `Managed` mode with full mesh topology, the number of BGP connections grows as N*(N-1)/2, potentially causing resource exhaustion in large clusters.

*Mitigation:*
- Test and document the maximum supported cluster size for full mesh topology
- Plan to implement route reflector topology as a future enhancement to support larger clusters
- Monitor FRR resource consumption in scale tests
- For large clusters, we recommend using `Unmanaged` routing mode with external route reflectors to reduce the number of BGP connections

### Drawbacks

#### Features Not Supported in No-Overlay Mode

The following OVN-Kubernetes features are not supported on networks operating in no-overlay mode:

- EgressIP
- EgressService
- Multicast
- IPsec

For more details on feature compatibility, refer to the [upstream OVN-Kubernetes enhancement](https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#feature-compatibility).

## Alternatives (Not Implemented)

N/A

## Open Questions [optional]
N/A

## Test Plan

E2E tests and QE tests should ensure that the no-overlay mode works as expected in all supported configurations and that we fully support:
- conformance kubernetes tests on the default network in no-overlay mode
- ovn-kubernetes E2E tests, excluding features that are not supported in no-overlay mode (i.e. EgressService, EgressIP, Multicast, IPsec), for the default network and for CUDNs in no-overlay mode.

### E2E tests

To minimize CI costs while ensuring comprehensive coverage, we will create two new dedicated CI lanes for no-overlay testing.

#### Lane 1: Managed mode (local gateway)

`e2e-metal-ipi-ovn-dualstack-bgp-managed-lgw`

This lane tests the managed routing scenario with local gateway mode:
- The default network is configured in no-overlay mode with managed BGP configuration
- There are CUDNs configured in no-overlay mode with managed BGP configuration and CUDNs configured in overlay mode
- It runs default network and CUDN regression tests
- It runs no-overlay specific tests (e.g. east-west no-overlay tests, `outboundSNAT` modification, etc.)

#### Lane 2: Unmanaged mode (shared gateway)

`e2e-metal-ipi-ovn-dualstack-bgp-unmanaged-sgw`

This lane tests the unmanaged routing scenario with shared gateway mode:
- The default network is configured in no-overlay mode with unmanaged BGP configuration
- There are CUDNs configured in no-overlay mode with unmanaged BGP configuration and CUDNs configured in overlay mode
- It runs default network and CUDN regression tests
- It runs BGP-specific tests
- It runs no-overlay specific tests (e.g. east-west no-overlay tests, `outboundSNAT` modification, etc.)

This two-lane approach provides coverage of both gateway modes and both routing modes while keeping the testing matrix manageable.

*Optional optimization*. To improve test coverage for `outboundSNAT`, we can run a comprehensive E2E test suite with one `outboundSNAT` value, then dynamically update the value and execute a targeted subset of tests. This approach aims to maximize coverage while avoiding excessive execution time. We will proceed only if our investigation confirms feasibility within the current CI environment.

### QE Testing
When testing a managed full mesh topology, it's crucial to monitor resource consumption as the cluster scales. The number of links in a full mesh increases according to N*(N-1)/2, where N represents the node count. We must verify that both ovn-kubernetes and FRR can manage the corresponding BGP connections in large clusters without excessive resource consumption.

Our scale testing should address two distinct areas:

- **Functional node scale-up**: Does adding a new node to an existing no-overlay cluster function correctly? We need to confirm the new node is properly configured and integrates into the BGP mesh.

- **Performance at scale**: Our current scale testing covers clusters of various sizes: small (24 nodes), medium (120 nodes), large (250 nodes), with extra-large (500 nodes) being tested less frequently. For no-overlay mode, we should ensure that at least medium-sized clusters (120 nodes) are covered to validate the sustainability of a full mesh topology at this scale.

## Graduation Criteria

We are going to develop this feature in two phases:
- phase 1: we will support no-overlay mode in the unmanaged configuration, where the FRRConfiguration and RouteAdvertisements CRs to configure the BGP topology are provided by the cluster administrator;
- phase 2: we will add support for the managed configuration, where the FRRConfiguration and RouteAdvertisements CRs are generated by OVN-Kubernetes based on the new API parameters.

Until phase 2 is complete, the feature will be considered in Tech Preview and will be enabled by a feature gate.

### Dev Preview -> Tech Preview

- Unmanaged no-overlay mode implemented
- Sufficient test coverage (E2E, QE)

### Tech Preview -> GA

- Unmanaged and managed no-overlay mode implemented
- More testing (upgrade, downgrade, scale, end to end)
- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Add `+kubebuilder:default=Geneve` to `transport` field so that the default value is visible in the CRD schema and applied automatically by the API server (omitted during Tech Preview to avoid breaking existing ungated tests; CNO handles the default during Tech Preview)

### Stretch Goal

A stretch goal for GA would be to allow changing the default network transport mode from overlay to no-overlay (and vice versa) on day 2. This requires extra considerations with regards to the cluster MTU.

The rule is: as long as `spec.defaultNetwork.ovnKubernetesConfig.mtu` + Geneve overhead (100 bytes) <= host MTU, the default network transport can be seamlessly switched between no-overlay and Geneve. When this condition does not hold (typically when using the full host MTU in no-overlay mode, that is when `spec.defaultNetwork.ovnKubernetesConfig.mtu := host MTU`), the default network MTU must be lowered through the existing [MTU migration procedure for OCP](https://github.com/openshift/enhancements/blob/67e388d2b6f14c9cc968ddc2fb1c125aae4ad78b/enhancements/network/allow-mtu-changes.md) before switching to overlay mode.

This stretch goal requires an API change to allow the `transport` field to be mutable, and a check in CNO to verify that the MTU requirements are satisfied before applying the change.

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

There are no special concerns about upgrades, since the feature can only be turned on for the default network at installation time and for CUDNs at the time of creation of the affected CUDNs. Configuration changes to immutable fields are rejected by CEL validation rules during upgrades.
QA coverage will include testing the upgrade of a cluster that already runs in no-overlay mode.

## Version Skew Strategy

### Component Version Skew

During cluster upgrades, there is version skew between ovnkube-control-plane (running on control plane nodes) and ovnkube-node (running on worker nodes). This version skew has no impact on no-overlay mode operation because future versions will be backwards compatible.

## Operational Aspects of API Extensions

No changes to metrics or telemetry are introduced by this feature. Existing metrics remain applicable.

Ongoing work on EVPN is adding metrics that will also benefit no-overlay mode:
- An overall metric for the health status of a CUDN, allowing operators to monitor CUDNs configured in no-overlay mode
- A metric that counts the number of networks configured with a particular transport, with corresponding telemetry, providing visibility into transport configuration across the cluster

## Support Procedures

## Infrastructure Needed [optional]
N/A
