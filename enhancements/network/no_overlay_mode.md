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
last-updated: 2025-12-17
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CORENET-6133
see-also:
  - https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md

---

# No-overlay mode

## Summary

This enhancement describes how the no-overlay mode for ovn-kubernetes integrates in openshift. The feature allows pods in selected networks to communicate using the underlay network, without the overhead of Geneve encapsulation that we use to build the overlay network. The no-overlay mode is described in detail in an OVN-Kubernetes upstream enhancement: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md . This document outlines the necessary API changes, the interaction with the Cluster Network Operator (CNO) and our test plan for this feature.

## Motivation

The motivations for this feature are to be found in the original upstream enhancement: https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md


### User Stories
See the upstream OVN-Kubernetes enhancement for detailed user stories:
https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#user-storiesuse-cases

### Goals
See the upstream OVN-Kubernetes enhancement for detailed goals:
https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#goals

**Current platform limitation:**
BGP is only supported on Bare Metal clusters. Since no-overlay mode requires BGP, it shares this limitation for now. As BGP support expands to more platforms, so will no-overlay mode.

### Non-Goals
See the upstream OVN-Kubernetes enhancement for detailed non-goals:
https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#non-goals

## Proposal

<!-- This section should explain what the proposal actually is. Enumerate -->
<!-- *all* of the proposed changes at a *high level*, including all of the -->
<!-- components that need to be modified and how they will be -->
<!-- different. Include the reason for each choice in the design and -->
<!-- implementation that is proposed here. -->

<!-- To keep this section succinct, document the details like API field -->
<!-- changes, new images, and other implementation details in the -->
<!-- **Implementation Details** section and record the reasons for not -->
<!-- choosing alternatives in the **Alternatives** section at the end of -->
<!-- the document. -->

The no-overlay feature largely leverages the existing BGP functionality in OVN-Kubernetes and only needs few API changes.
The feature can be applied to:
- the default network, at cluster installation time
- Cluster User Defined Networks (CUDNs) with Layer3 topology, at run time

**Note:** Only Layer3 CUDNs support no-overlay mode. See the [upstream enhancement](https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#non-goals) for details.

For each network we are going to need a `transport` parameter that takes `geneve` (default) or `no-overlay`.
Then if `transport` is set to `no-overlay`, we need the following parameters to configure the no-overlay mode:
- `outboundSNAT`:
  - `Enabled`: apply source NAT to egress traffic, allowing only the node IP to be exposed, which is today's expected behaviour unless EgressIP is used;
  - `Disabled`: do not apply any SNAT to egress traffic, thus exposing the pod subnet outside the cluster. This is the same behaviour we have today for a BGP-advertised network.
- `routing`:
  - `Managed`: delegate to OVN-Kubernetes the configuration of the BGP fabric. OVN-Kubernetes will configure the FRR instance on each node to set up an internal BGP fabric without requiring external BGP routers.
  - `Unmanaged`: use the FRRConfig and RouteAdvertisements provided by the cluster administrator to implement the no-overlay mode

For CUDNs these parameters are to be added to the CUDN CRD and can be configured by the cluster administrator when creating a CUDN. For the default network, these parameters must be input by the cluster administrator at installation time and passed over to ovn-kubernetes by the Cluster Network Operator.

There are two global parameters specific to the way that the no-overlay mode is to be implemented when `NoOverlayOptions.routing`=`Managed`, affecting the generated BGP configuration:
- `asNumber`: (optional) the Autonomous System (AS) number to be used in the generated FRRConfig for the default VRF. When omitted, this defaults to 64512. This value should be aligned with any other FRRConfiguration resources added by the cluster administrator.
- `bgpTopology`:
  - `FullMesh`: every node deploys a BGP router, thus forming a BGP full mesh.
  <!-- - `routeReflector`: every master node will run a BGP route reflector, in order to reduce the number of BGP connections in the cluster; this is particularly useful for large clusters. -->

The resulting FRRConfig and RouteAdvertisements will be generated by OVN-Kubernetes and are described in detail in the upstream enhancement.
In this enhancement we are going to detail the workflow necessary for a cluster admin to enable this feature in Openshift and we are going to define how the Openshift API is to be extended to include these new parameters, which will be passed by CNO to OVN-Kubernetes at installation time.

### MTU Considerations

Networks operating in no-overlay mode use the provider network MTU directly, as there is no encapsulation overhead. This means pods on no-overlay networks can use the full MTU of the underlying physical network infrastructure, unlike overlay networks which require a reduced MTU to accommodate encapsulation headers.

**Default network MTU:** The MTU for the default network is specified in `operator.openshift.io/v1 Network` at installation time. For example, if CNO sees that the provider network MTU is 1500 bytes and the user doesn't specify an explicit MTU:
- In **overlay mode**, the cluster network MTU will be set to 1400 bytes (1500 - 100 bytes for Geneve encapsulation overhead)
- In **no-overlay mode**, the cluster network MTU will be set to 1500 bytes (the full provider network MTU)

If the user specifies an explicit MTU in no-overlay mode, CNO validates that the configured MTU does not exceed the host MTU. If the specified MTU is greater than the host MTU, CNO will report a degraded status with an error message indicating the invalid MTU configuration.

**CUDN MTU:** The MTU for CUDNs is specified in the `spec.network.layer3.mtu` field of the ClusterUserDefinedNetwork CRD. OVN-Kubernetes validates that the configured MTU does not exceed the maximum allowed based on the CUDN's transport mode (host MTU for no-overlay, host MTU - 100 for Geneve). CNO does not provide the host MTU value to OVN-Kubernetes; OVN-Kubernetes discovers it directly from the host.

### Workflow Description
<!-- No-overlay mode can be enabled at cluster installation time (day 0) or at run time (day 2) depending on which networks (default vs CUDNs) are to be affected. -->


In a nutshell, the necessary steps to enable no-overlay are the following:
- enable `TechPreviewNoUpgrade` feature gate on Openshift at installation time, as long as no-overlay mode is in tech preview.
- enable BGP for the cluster in `operator.openshift.io/v1 Network`:
  - `spec.additionalRoutingCapabilities.providers`: `FRR`
  - `spec.defaultNetwork.ovnKubernetesConfig.routeAdvertisements`: `Enabled`
- enable no-overlay mode for the default network (if desired) in `operator.openshift.io/v1 Network`and configure it with the per-network parameters:
  - `outboundSNAT`: `Enabled`,`Disabled`
  - `routing`: `Managed`, `Unmanaged`
- provide the necessary manifests for the `Unmanaged` routing scenario, if that's the preferred routing mode for either the default network (as configured above) or for CUDNs (created later on, after cluster installation):
  - FRRConfig CR
  - RouteAdvertisements CR
- configure parameters for the `Managed` scenario, if that's the preferred routing mode for either the default network or for CUDNs:
  - `asNumber`: int (optional) - the BGP AS number to use for the default VRF. When omitted, this defaults to 64512.
  - `bgpTopology`: `FullMesh` (required) - defines the BGP peering topology. Currently only `FullMesh` is supported, where every node deploys a BGP router and peers with all other nodes. Future enhancements may add support for route reflector topologies.
- create CUDNs with `transport`: `NoOverlay` and the desired `noOverlayOptions`.

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
  - `outboundSNAT`: `Enabled`,`Disabled`
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
      defaultNetworkTransport: NoOverlay
      defaultNetworkNoOverlayOptions:
        outboundSNAT: Disabled
        routing: Unmanaged
    type: OVNKubernetes
```

The cluster administrator should also provide in the `manifests` folder the necessary manifests for the `Unmanaged` scenario:
  - FRRConfiguration CR (defines the BGP peering with the external router)
  - RouteAdvertisements CR (defines which routes to advertise)

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
            prefixes:
            - prefix: 10.128.0.0/14  # Cluster pod subnet
              ge: 23  # Must match the cluster's hostPrefix
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

For more details on the upstream implementation, see the OVN-Kubernetes enhancement:
https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#enable-no-overlay-mode-for-the-default-network-with-unmanaged-routing

If there are errors in setting up no-overlay mode for the default network (e.g., missing or invalid RouteAdvertisements CR), OVN-Kubernetes will report an event and log error messages.

#### No-overlay for the default network with managed routing
In `operator.openshift.io/v1 Network` the cluster administrator should enable no-overlay mode for the default network and configure it with the necessary per-network parameters:
  - `outboundSNAT`: `Enabled`,`Disabled`
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
      defaultNetworkTransport: NoOverlay
      defaultNetworkNoOverlayOptions:
        outboundSNAT: Enabled
        routing: Managed
      bgpManagedConfig:
        asNumber: 64512
        bgpTopology: FullMesh
    type: OVNKubernetes
```
OVN-Kubernetes will take care of generating the necessary FRRConfig and RouteAdvertisements CRs.

If there are errors in setting up no-overlay mode for the default network (e.g., missing or invalid RouteAdvertisements CR), OVN-Kubernetes will report an event and log error messages.

#### No-overlay for CUDNs with unmanaged routing
The cluster administrator should enable BGP either on day 0 or on day 2, as described in the prerequisite section. Along with enabling BGP, the cluster administrator should provide at the same time the two manifests that define the external BGP speakers:
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
            prefixes:
            - prefix: 10.10.0.0/16  # CUDN pod subnet
              ge: 24  # Must match the CUDN's hostSubnet
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
          network: blue  # Matches the CUDN below
  nodeSelector: {}
```

##### Example: External BGP Router Configuration (FRR)

The external BGP router configuration is similar to the default network case. Example FRR configuration:

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

On day 2, the cluster administrator can define CUDNs with `transport`:`NoOverlay` and further configure the no-overlay options within the CUDN CR. The CUDN must have a label that matches the `networkSelector` in the RouteAdvertisements CR. For instance:
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
    noOverlayOptions:
      outboundSNAT: "Disabled"
      routing: "Unmanaged"
status:
  conditions:
  - type: TransportAccepted
    status: "True"
    reason: "NoOverlayTransportAccepted"
    message: "No-overlay transport has been configured."
```

Any error in setting up no-overlay mode for a CUDN will be reflected in the status conditions of the CUDN.

#### No-overlay for CUDNs with managed routing
The cluster administrator should enable BGP either on day 0 or on day 2, as described in the prerequisite section. In order to configure managed routing, `BGPManagedConfig` options should also be set in `operator.openshift.io/v1 Network` either on day 0 or on day 2. If done on day 2, an example of the patch operation could be:

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
OVN-Kubernetes will take care of generating the necessary FRRConfig and RouteAdvertisements CRs.

On day 2, the cluster administrator can define CUDNs with `transport`:`noOverlay` and further configure the no-overlay options within the CUDN CR. For instance:
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
    noOverlayOptions:
      outboundSNAT: "Disabled"
      routing: "Managed"
status:
  conditions:
  - type: TransportAccepted
    status: "True"
    reason: "NoOverlayTransportAccepted"
    message: "No-overlay transport has been configured."
```

Any error in setting up no-overlay mode for a CUDN will be reflected in the status conditions of the CUDN.


#### Further considerations

<!-- The cluster administrator is expected to provide the OCP configuration and necessary manifests to the OpenShift installer, which then deploys them as part of the cluster installation process. -->

It's important to note that there can be networks in no-overlay mode running in `Managed` mode, and networks in no-overlay mode running in `Unmanaged` mode, coexisting in the same cluster.

On day 1, during installation, CNO then propagate the configuration parameters to the ovn-kubernetes components (ovnkube-control-plane and ovnkube-node) via the existing configmap. OVN-Kubernetes will then be responsible for implementing no-overlay mode according to the provided parameters; in particular, for `Managed` mode, OVN-Kubernetes will generate the necessary FRRConfig and RouteAdvertisements based on the provided parameters.

Extra care must be taken to ensure that the total time taken by CNO to deploy the network is not significantly increased by the additional steps necessary to configure no-overlay mode. The time taken by the network to converge should be well within the time window allocated to CNO by the Cluster Version Operator (CVO).

<!-- Changes to the no-overlay mode configuration after the initial cluster installation are not currently supported and are therefore forbidden by the newly introduced API. -->

When a network is created in no-overlay mode, be it the default network or a CUDN, it is not possible to switch it to overlay mode. The opposite is also true: a network created in overlay mode cannot be switched to no-overlay mode. In particular:
- For the default network, the feature can only be enabled at cluster installation time and cannot be disabled afterwards.
- For CUDNs, the cluster administrator can choose between overlay and no-overlay mode at creation time, in the CUDN spec. For an existing CUDN, the mode cannot be changed unless the CUDN is deleted and recreated with the desired overlay / no-overlay mode.


### API Extensions

The following changes have been added to the operator network configuration (`operator/v1/types_network.go`) in the `openshift/api` repository:

#### OVNKubernetesConfig struct additions

```go
// Maintainer note for NoOverlayMode feature (TechPreview):
// When NoOverlayMode graduates to GA, add '+kubebuilder:default=Geneve' to the DefaultNetworkTransport
// field so the default is visible in the CRD schema and applied by the API server automatically.
// Currently CNO handles the default (treating omitted as Geneve) because the field is feature-gated
// and existing ungated tests don't expect this field in outputs.

// ovnKubernetesConfig contains the configuration parameters for networks
// using the ovn-kubernetes network project
// +openshift:validation:FeatureGateAwareXValidation:featureGate=NoOverlayMode,rule="!has(self.defaultNetworkTransport) || self.defaultNetworkTransport != 'NoOverlay' || has(self.defaultNetworkNoOverlayOptions)",message="defaultNetworkNoOverlayOptions is required when defaultNetworkTransport is NoOverlay"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=NoOverlayMode,rule="!has(self.defaultNetworkNoOverlayOptions) || self.defaultNetworkNoOverlayOptions.routing != 'Managed' || has(self.bgpManagedConfig)",message="bgpManagedConfig is required when defaultNetworkNoOverlayOptions.routing is Managed"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=NoOverlayMode,rule="!has(oldSelf.defaultNetworkTransport) || oldSelf.defaultNetworkTransport == '' || has(self.defaultNetworkTransport)",message="defaultNetworkTransport cannot be removed once set to a non-empty value"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=NoOverlayMode,rule="!has(oldSelf.defaultNetworkNoOverlayOptions) || has(self.defaultNetworkNoOverlayOptions)",message="defaultNetworkNoOverlayOptions cannot be removed once set"
type OVNKubernetesConfig struct {
	// ... existing fields ...

	// defaultNetworkTransport describes the transport protocol for east-west traffic for the default network.
	// Allowed values are "NoOverlay" and "Geneve".
	// When set to "NoOverlay", the default network operates in no-overlay mode.
	// When set to "Geneve", the default network uses Geneve overlay.
	// When omitted, this means the user has no opinion and the platform chooses a reasonable default which is subject to change over time.
	// The current default is "Geneve".
	// +openshift:enable:FeatureGate=NoOverlayMode
	// +kubebuilder:validation:Enum=NoOverlay;Geneve
	// +kubebuilder:validation:XValidation:rule="oldSelf == '' || self == oldSelf",message="defaultNetworkTransport is immutable once set"
	// +optional
	DefaultNetworkTransport TransportOption `json:"defaultNetworkTransport,omitempty"`

	// defaultNetworkNoOverlayOptions contains configuration for no-overlay mode for the default network.
	// It is required when DefaultNetworkTransport is "NoOverlay".
	// When omitted, this means the user does not configure no-overlay mode options.
	// +openshift:enable:FeatureGate=NoOverlayMode
	// +kubebuilder:validation:XValidation:rule="!oldSelf.hasValue() || self == oldSelf.value()",message="defaultNetworkNoOverlayOptions is immutable once set",optionalOldSelf=true
	// +optional
	DefaultNetworkNoOverlayOptions NoOverlayOptions `json:"defaultNetworkNoOverlayOptions,omitzero,omitempty"`

	// bgpManagedConfig configures the BGP properties for networks (default network or CUDNs)
	// in no-overlay mode that specify routing="Managed" in their NoOverlayOptions.
	// It is required when DefaultNetworkNoOverlayOptions.Routing is set to "Managed".
	// When omitted, this means the user does not configure BGP for managed routing.
	// +openshift:enable:FeatureGate=NoOverlayMode
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="bgpManagedConfig field is immutable"
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

// NoOverlayOptions contains configuration options for networks operating in no-overlay mode.
type NoOverlayOptions struct {
	// outboundSNAT defines the SNAT behavior for outbound traffic from pods.
	// Allowed values are "Enabled" and "Disabled".
	// When set to "Enabled", SNAT is performed on outbound traffic from pods.
	// When set to "Disabled", SNAT is not performed and pod IPs are preserved in outbound traffic.
	// This field is required when the network operates in no-overlay mode.
	// +kubebuilder:validation:Enum=Enabled;Disabled
	// +required
	OutboundSNAT SNATOption `json:"outboundSNAT,omitempty"`

	// routing specifies whether the pod network routing is managed by OVN-Kubernetes or users.
	// Allowed values are "Managed" and "Unmanaged".
	// When set to "Managed", OVN-Kubernetes manages the pod network routing configuration through BGP.
	// When set to "Unmanaged", users are responsible for configuring the pod network routing.
	// This field is required when the network operates in no-overlay mode.
	// +kubebuilder:validation:Enum=Managed;Unmanaged
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
	// +kubebuilder:default=64512
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

No-overlay mode support for Hypershift / Hosted Control Planes will be deferred to a future enhancement. The initial implementation will focus on standalone clusters only.

**Considerations for future Hypershift support:**
- The control plane runs in the management cluster while data plane (worker nodes) runs in the hosted cluster
- BGP peering would need to occur between hosted cluster nodes, not involving the management cluster
- FRR would run on hosted cluster nodes only
- The HostedControlPlane API would need to be extended in a similar way to the Network operator API
- Additional testing would be required to ensure BGP route propagation works correctly within hosted cluster boundary

This topic requires further discussion with the Hypershift team and will be addressed as an update to this enhancement.
#### Standalone Clusters

No special considerations for standalone clusters.

#### Single-node Deployments or MicroShift
<!-- How does this proposal affect the resource consumption of a -->
<!-- single-node OpenShift deployment (SNO), CPU and memory? -->

<!-- How does this proposal affect MicroShift? For example, if the proposal -->
<!-- adds configuration options through API resources, should any of those -->
<!-- behaviors also be exposed to MicroShift admins through the -->
<!-- configuration file for MicroShift? -->

The goal being to not encapsulate traffic between any two cluster nodes, there is no use case for this feature in true single-node deployments. For deployments with a single-node control plane and multiple worker nodes, the feature is usable in the same way as standalone clusters.

### Implementation Details/Notes/Constraints

#### openshift/api Validation

The `openshift/api` repository defines the `operator.openshift.io/v1 Network` CRD schema, which includes:
- No-overlay configuration fields for the default network (`defaultNetworkTransport`, `defaultNetworkNoOverlayOptions`)
- BGP managed configuration (`bgpManagedConfig`) that applies to both the default network and CUDNs when using managed routing

CEL (Common Expression Language) validation rules are used to reject invalid configurations at the API server level. These rules enforce:

- `defaultNetworkNoOverlayOptions` is required when `defaultNetworkTransport` is set to `NoOverlay`
- `bgpManagedConfig` is required when `defaultNetworkNoOverlayOptions.routing` is set to `Managed`
- Immutability of `defaultNetworkTransport`, `defaultNetworkNoOverlayOptions`, and `bgpManagedConfig` fields after initial configuration

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
2. For `Managed` routing: generating FRRConfig and RouteAdvertisements CRs based on the cluster topology
3. For `Unmanaged` routing: relying on user-provided FRRConfig and RouteAdvertisements CRs
4. Coordinating with FRR-Kubernetes to establish BGP peerings and advertise pod subnets

#### Day-0 vs Day-2 Configuration

- **Default network**: Must be configured at installation time (day-0) via manifests provided to the OpenShift installer. The configuration is immutable after installation.
- **CUDNs**: Can be configured at run time (day-2) when creating the CUDN. The transport mode is immutable after CUDN creation.

For detailed implementation specifics, refer to the upstream OVN-Kubernetes enhancement:
https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md

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

The following OVN-Kubernetes features are not supported when operating in no-overlay mode:

- EgressIP
- EgressService
- Multicast
- IPsec

For more details on feature compatibility, refer to the upstream OVN-Kubernetes enhancement:
https://github.com/ovn-kubernetes/ovn-kubernetes/blob/deaf3c549936ee975d80d7b9447ede652165e69a/docs/okeps/okep-5259-no-overlay.md#feature-compatibility

## Alternatives (Not Implemented)

N/A

## Open Questions [optional]
N/A

## Test Plan

<!-- **Note:** *Section not required until targeted at a release.* -->

<!-- Consider the following in developing a test plan for this enhancement: -->
<!-- - Will there be e2e and integration tests, in addition to unit tests? -->
<!-- - How will it be tested in isolation vs with other components? -->
<!-- - What additional testing is necessary to support managed OpenShift service-based offerings? -->

<!-- No need to outline all of the test cases, just the general strategy. Anything -->
<!-- that would count as tricky in the implementation and anything particularly -->
<!-- challenging to test should be called out. -->

<!-- All code is expected to have adequate tests (eventually with coverage -->
<!-- expectations). -->

E2E tests and QE tests should ensure that the no-overlay mode works as expected in all supported configurations and that we fully support:
- conformance kubernetes tests on the default network in no-overlay mode
- ovn-kubernetes E2E tests, excluding features that are not supported in no-overlay mode (i.e. EgressService, EgressIP, Multicast, IPSEC), for the default network and for CUDNs in no-overlay mode.

### E2E tests

We already have two dual stack CI lanes for BGP, one for shared gateway and one for local gateway.
- e2e-metal-ipi-ovn-dualstack-bgp
- e2e-metal-ipi-ovn-dualstack-bgp-local-gw

These two lanes will be extended to cover the no-overlay mode in the unmanaged scenario for CUDNs. Since a CUDN can be added after cluster installation, we can extend the two existing CI lanes to enable the no-overlay feature gate at installation time and then create a CUDN in no-overlay mode and test its connectivity east-west and north-south. CUDNs can be created on the fly with `outboundSnat`=`Enabled` or `outboundSnat`=`Disabled`, so both cases can be covered in the same CI lane.
This would give us a good coverage of the no-overlay mode for CUDNs in the unmanaged scenario for both gateway modes.

Enabling the feature gate at installation time in the two existing BGP lanes above and running e2e tests on them for no-overlay mode is a reasonable trade-off that will help us minimize the number of CI lanes and reduce CI costs without affecting the existing CI coverage for the BGP feature.

We can test the default network in two new separate CI lanes that we create specifically for the managed scenario, where we can test no-overlay mode with the full mesh topology. We can have one lane run ovn-kubernetes in shared gateway mode and the other in local gateway mode. In both lanes the default network is in no-overlay mode and we can create CUDNs in no-overlay mode on the fly, with `outboundSNAT`=`Enabled` and then `outboundSNAT`=`Disabled`, thus covering both cases in the same lane:
- e2e-metal-ipi-ovn-dualstack-shared-gw-no-overlay-managed-full-mesh-techpreview
- e2e-metal-ipi-ovn-dualstack-local-gw-no-overlay-managed-full-mesh-techpreview

To cover different `outboundSNAT` configurations for the default network, one lane can configure `outboundSNAT`=`Enabled` and the other `outboundSNAT`=`Disabled`.

### QE Testing
When testing a managed full mesh topology, we should pay some special attention to resource consumption as we scale up the cluster. The number of links in a full mesh topology grows as N*(N-1)/2, where N is the number of nodes in the cluster. We need to ensure that ovn-kubernetes and FRR can handle the number of BGP connections in a large cluster without excessive resource consumption.
In our scale testing we should explicitly test for two separate aspects:
- cluster node scale up functionally: does adding a new node to a cluster already in no-overlay mode work as expected? Is the new node correctly configured and does it join the BGP mesh?
- cluster scale performance wise: in our scale testing we have good coverage for clusters of size: small (24 nodes), medium (120 nodes), large (250 nodes); x-large (500 nodes) clusters are less frequently tested. We should aim to have coverage for no-overlay mode for at least medium-size clusters (120 nodes) and verify that a full mesh topology is sustainable at this scale.

## Graduation Criteria

<!-- **Note:** *Section not required until targeted at a release.* -->

<!-- Define graduation milestones. -->

<!-- These may be defined in terms of API maturity, or as something else. Initial proposal -->
<!-- should keep this high-level with a focus on what signals will be looked at to -->
<!-- determine graduation. -->

<!-- Consider the following in developing the graduation criteria for this -->
<!-- enhancement: -->

<!-- - Maturity levels -->
<!--   - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels] -->
<!--   - `Dev Preview`, `Tech Preview`, `GA` in OpenShift -->
<!-- - [Deprecation policy][deprecation-policy] -->

<!-- Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning), -->
<!-- or by redefining what graduation means. -->

<!-- In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed. -->

<!-- [maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions -->
<!-- [deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/ -->

<!-- **If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/), -->
<!-- please be sure to include in the graduation criteria.** -->

<!-- **Examples**: These are generalized examples to consider, in addition -->
<!-- to the aforementioned [maturity levels][maturity-levels]. -->

We are going to develop this feature in two phases:
- phase 1: we will support no-overlay mode in the unmanaged configuration: an FRRConfig and a RouteAdvertisements to configure the BGP topology are provided by the cluster administrator.
- phase 2: we will add support for the managed configuration: the FRRConfig and RouteAdvertisements are generated by OVN-Kubernetes based on the new API parameters.

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
- Add `+kubebuilder:default=Geneve` to `defaultNetworkTransport` field so that the default value is visible in the CRD schema and applied automatically by the API server (omitted during Tech Preview to avoid breaking existing ungated tests; CNO handles the default during Tech Preview)

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

<!-- If applicable, how will the component be upgraded and downgraded? Make sure this -->
<!-- is in the test plan. -->

<!-- Consider the following in developing an upgrade/downgrade strategy for this -->
<!-- enhancement: -->
<!-- - What changes (in invocations, configurations, API use, etc.) is an existing -->
<!--   cluster required to make on upgrade in order to keep previous behavior? -->
<!-- - What changes (in invocations, configurations, API use, etc.) is an existing -->
<!--   cluster required to make on upgrade in order to make use of the enhancement? -->

<!-- Upgrade expectations: -->
<!-- - Each component should remain available for user requests and -->
<!--   workloads during upgrades. Ensure the components leverage best practices in handling [voluntary -->
<!--   disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to -->
<!--   this should be identified and discussed here. -->
<!-- - Micro version upgrades - users should be able to skip forward versions within a -->
<!--   minor release stream without being required to pass through intermediate -->
<!--   versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1` -->
<!--   as an intermediate step. -->
<!-- - Minor version upgrades - you only need to support `x.N->x.N+1` upgrade -->
<!--   steps. So, for example, it is acceptable to require a user running 4.3 to -->
<!--   upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step. -->
<!-- - While an upgrade is in progress, new component versions should -->
<!--   continue to operate correctly in concert with older component -->
<!--   versions (aka "version skew"). For example, if a node is down, and -->
<!--   an operator is rolling out a daemonset, the old and new daemonset -->
<!--   pods must continue to work correctly even while the cluster remains -->
<!--   in this partially upgraded state for some time. -->

<!-- Downgrade expectations: -->
<!-- - If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is -->
<!--   misbehaving, it should be possible for the user to rollback to `N`. It is -->
<!--   acceptable to require some documented manual steps in order to fully restore -->
<!--   the downgraded cluster to its previous state. Examples of acceptable steps -->
<!--   include: -->
<!--   - Deleting any CVO-managed resources added by the new version. The -->
<!--     CVO does not currently delete resources that no longer exist in -->
<!--     the target version. -->

There are no special concerns about upgrades, since the feature can only be turned on for the default network at installation time and for CUDNs at the time of creation of the affected CUDNs. Configuration changes should not be allowed by CNO during upgrades.
QA coverage will include testing the upgrade of a cluster that already runs in no-overlay mode.

## Version Skew Strategy

### Component Version Skew

During cluster upgrades, there will be version skew between ovnkube-control-plane (running on control plane nodes) and ovnkube-node (running on worker nodes). This enhancement handles version skew as follows:

**Configuration Immutability:**
- No-overlay mode configuration is immutable, preventing changes during upgrades
- CNO will reject any attempts to modify `defaultNetworkTransport`, `defaultNetworkNoOverlayOptions`, or `bgpManagedConfig` during upgrades
- This ensures consistent behavior across all component versions during the upgrade window

**Testing:**
- E2E tests will include upgrade scenarios from version N to N+1
- Version skew between ovnkube-control-plane and ovnkube-node will be tested as part of standard rolling update procedures

## Operational Aspects of API Extensions

<!-- Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs, -->
<!-- admission and conversion webhooks, aggregated API servers, finalizers) here in detail, -->
<!-- especially how they impact the OCP system architecture and operational aspects. -->

<!-- - For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level -->
<!--   Indicators) an administrator or support can use to determine the health of the API extensions -->

<!--   Examples (metrics, alerts, operator conditions) -->
<!--   - authentication-operator condition `APIServerDegraded=False` -->
<!--   - authentication-operator condition `APIServerAvailable=True` -->
<!--   - openshift-authentication/oauth-apiserver deployment and pods health -->

<!-- - What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput, -->
<!--   API availability) -->

<!--   Examples: -->
<!--   - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average. -->
<!--   - Fails creation of ConfigMap in the system when the webhook is not available. -->
<!--   - Adds a dependency on the SDN service network for all resources, risking API availability in case -->
<!--     of SDN issues. -->
<!--   - Expected use-cases require less than 1000 instances of the CRD, not impacting -->
<!--     general API throughput. -->

<!-- - How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or -->
<!--   automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review -->
<!--   this enhancement) -->

<!-- - Describe the possible failure modes of the API extensions. -->
<!-- - Describe how a failure or behaviour of the extension will impact the overall cluster health -->
<!--   (e.g. which kube-controller-manager functionality will stop working), especially regarding -->
<!--   stability, availability, performance and security. -->
<!-- - Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes -->
<!--   and add them as reviewers to this enhancement. -->

## Support Procedures

<!-- Describe how to -->
<!-- - detect the failure modes in a support situation, describe possible symptoms (events, metrics, -->
<!--   alerts, which log output in which component) -->

<!--   Examples: -->
<!--   - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz". -->
<!--   - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed". -->
<!--   - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")` -->
<!--     will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire. -->

<!-- - disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`) -->

<!--   - What consequences does it have on the cluster health? -->

<!--     Examples: -->
<!--     - Garbage collection in kube-controller-manager will stop working. -->
<!--     - Quota will be wrongly computed. -->
<!--     - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data. -->
<!--       Disabling the conversion webhook will break garbage collection. -->

<!--   - What consequences does it have on existing, running workloads? -->

<!--     Examples: -->
<!--     - New namespaces won't get the finalizer "xyz" and hence might leak resource X -->
<!--       when deleted. -->
<!--     - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod -->
<!--       communication after some minutes. -->

<!--   - What consequences does it have for newly created workloads? -->

<!--     Examples: -->
<!--     - New pods in namespace with Istio support will not get sidecars injected, breaking -->
<!--       their networking. -->

<!-- - Does functionality fail gracefully and will work resume when re-enabled without risking -->
<!--   consistency? -->

<!--   Examples: -->
<!--   - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence -->
<!--     will not block the creation or updates on objects when it fails. When the -->
<!--     webhook comes back online, there is a controller reconciling all objects, applying -->
<!--     labels that were not applied during admission webhook downtime. -->
<!--   - Namespaces deletion will not delete all objects in etcd, leading to zombie -->
<!--     objects when another namespace with the same name is created. -->

## Infrastructure Needed [optional]

N/A
