---
title: bgp-vip-management
authors:
  - "@mkowalski"
reviewers:
  - "@cybertron, On-prem networking aspects"
  - "@jcaamano, OVN-Kubernetes"
  - "@fedepaol, MetalLB/frr-k8s"
approvers:
  - "@bbennett, Networking staff engineer"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-04-23
last-updated: 2026-04-23
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OPNET-595
see-also:
  - "/enhancements/network/bgp-overview.md"
  - "/enhancements/network/bgp-ovn-kubernetes.md"
  - "/enhancements/network/on-prem-service-load-balancers.md"
---

# BGP-Based VIP Management for On-Premise OpenShift Clusters

## Summary

This enhancement proposes replacing the current keepalived/VRRP-based Virtual IP
(VIP) management for on-premise OpenShift clusters with a BGP-based approach
using kube-vip in Routing Table Mode and frr-k8s (FRR-K8s). kube-vip will manage
API and Ingress VIP addresses by writing routes into a dedicated Linux routing
table, while frr-k8s -- deployed as a static pod during bootstrap -- will read
those routes and advertise them to external BGP peers. This approach eliminates
the L2 domain dependency inherent in keepalived and provides faster failover
through BFD integration. The
OpenShift installer API (`install-config.yaml`) will be extended to accept BGP
peering configuration at installation time for both API and Ingress VIPs.

## Motivation

OpenShift on-premise clusters currently rely on keepalived (VRRP) to manage API
and Ingress VIPs. This approach has several fundamental limitations:

- **L2 domain requirement**: All control plane nodes managing a VIP must reside
  on the same L2 network segment, since VRRP uses gratuitous ARP (IPv4) or NDP
  (IPv6) to announce VIP ownership changes. This constrains network topology
  choices and prevents deployments across L3 boundaries.

- **Active/passive only**: VRRP elects a single master that owns the VIP.
  Traffic cannot be distributed across multiple nodes, leading to uneven load
  and underutilization of available control plane capacity.

- **Slow failover**: VRRP failover depends on advertisement timers (typically
  1-3 seconds), which is too slow for latency-sensitive workloads and can cause
  visible disruption to API consumers during control plane node failures.

- **No integration with external routing infrastructure**: keepalived operates
  purely at L2 and cannot signal VIP location changes to upstream routers or
  load balancers, requiring static route configuration or additional
  infrastructure.

The primary goal of this enhancement is to eliminate the L2 domain requirement
by introducing BGP-based VIP advertisement. This is the most critical
limitation for customers deploying OpenShift across routed L3 networks, spine-
and-leaf fabrics, or multi-rack environments where L2 adjacency between control
plane nodes is not feasible. Faster failover (via BFD) and integration with
external routing infrastructure are additional benefits delivered in this first
iteration.

ECMP-based load distribution across multiple nodes will be enabled in a
subsequent iteration once the foundational BGP VIP infrastructure is proven and
operational experience is gained. The first iteration retains an active/passive
model via leader election, which is functionally equivalent to keepalived in
this regard but operates at L3 rather than L2.

### User Stories

* As a cluster administrator deploying OpenShift on bare metal in a routed L3
  network, I want to use BGP to advertise my API and Ingress VIPs so that I am
  not constrained to having all control plane nodes on the same L2 segment.

* As a cluster administrator, I want to configure BGP peering for my API and
  Ingress VIPs at installation time via `install-config.yaml` so that the
  cluster is reachable via BGP from the moment it is bootstrapped.

* As a network engineer, I want the OpenShift cluster to advertise its VIPs via
  BGP with standard attributes (ASN, communities, password authentication) so
  that I can integrate it into my existing BGP fabric using my standard
  operational procedures.

* As a site reliability engineer, I want sub-second VIP failover via BFD-backed
  BGP sessions so that API and Ingress availability is maintained during node
  failures with minimal disruption.

* As a platform operator managing multiple OpenShift clusters, I want to
  monitor the health of BGP VIP advertisements using standard BGP tooling
  (FRRNodeState CRs, `vtysh` on nodes, BGP session metrics) so that I can
  detect and troubleshoot VIP reachability issues.

### Goals

1. Provide a BGP-based alternative to keepalived for managing API and Ingress
   VIPs on bare metal OpenShift clusters.

2. Enable VIP advertisement via BGP from the earliest bootstrap phase, before
   the Kubernetes API server is available, by deploying frr-k8s as a static pod.

3. Extend the OpenShift installer API (`install-config.yaml`) to accept BGP
   peering configuration for API and Ingress VIPs at installation time.

4. Ensure that OVN-Kubernetes operates correctly when frr-k8s runs as a static
   pod, maintaining compatibility with the existing BGP integration work (see
   [bgp-ovn-kubernetes.md](/enhancements/network/bgp-ovn-kubernetes.md)).

5. Support BFD-backed fast failover for VIPs.

6. Support dual-stack (IPv4 and IPv6) VIP advertisement from the first
   iteration. The BGP configuration must support both address families
   simultaneously.

7. Provide day-2 observability for VIP BGP sessions through FRRNodeState CRs,
   BGP session metrics, and standard FRR diagnostic tools.

### Non-Goals

1. Replacing MetalLB for `type=LoadBalancer` Service IP advertisement. MetalLB
   remains the supported solution for Service load balancer IPs.

2. Advertising pod network or service network routes via BGP. This is covered
   by the [OVN-Kubernetes BGP Integration](/enhancements/network/bgp-ovn-kubernetes.md)
   enhancement.

3. Supporting BGP-based VIP management on cloud platforms (AWS, Azure, GCP).
   Cloud platforms have their own load balancer services for VIP management.

4. Supporting on-prem platforms other than bare metal in the first iteration.
   Platforms such as vSphere, OpenStack, and `platform: none` also use
   keepalived for VIP management and could benefit from BGP-based VIPs.
   Support for these platforms may be added in future iterations once the
   bare metal implementation is proven.

5. Supporting routing protocols other than BGP (e.g., OSPF, IS-IS).

6. Supporting kube-vip in ARP or BGP mode. Only the Routing Table Mode is
   in scope, as it cleanly separates VIP management from route advertisement.

7. Removing keepalived support. The existing keepalived-based VIP management
   will remain as the default and will continue to be supported. BGP-based VIP
   management is an opt-in alternative.

8. ECMP (Equal Cost Multi Path) load distribution for VIPs. The first iteration
   uses leader-elected, single-node VIP ownership (active/passive). ECMP
   support, where multiple nodes simultaneously advertise the same VIP for load
   distribution, is deferred to a future enhancement.

## Proposal

The proposed architecture introduces two new static pod components on control
plane nodes and modifies several existing components:

### Architecture Overview

The solution is composed of three cooperating components on each control plane
node:

1. **kube-vip (Routing Table Mode)**: Runs as a static pod. Watches the
   Kubernetes API for control plane and service VIP state. When a VIP is
   assigned to the local node (via leader election), kube-vip writes a `/32`
   (or `/128` for IPv6) route for the VIP address into
   a dedicated Linux routing table (table ID `198` by default) using netlink
   with protocol ID `248`. kube-vip does *not* run any routing protocol itself;
   it only manages routing table entries.

2. **frr-k8s (static pod)**: Runs as a static pod alongside kube-vip. An FRR
   instance inside frr-k8s is configured to read routes from routing table
   `198` (the table managed by kube-vip) and advertise them to configured BGP
   peers. frr-k8s uses the `FRRConfiguration` CRD for configuration; however,
   during bootstrap when the API server is not yet available, a static FRR
   configuration file will be used, generated by the installer from the
   `install-config.yaml` BGP peering parameters.

3. **OVN-Kubernetes**: Continues to manage pod networking. When frr-k8s runs as
   a static pod, OVN-Kubernetes must detect this deployment model and avoid
   deploying a conflicting frr-k8s DaemonSet instance. OVN-Kubernetes route
   import (listening on netlink for BGP-learned routes) continues to work
   unchanged, as it is agnostic to whether frr-k8s runs as a static pod or
   DaemonSet.

```
                                    External BGP Peer
                                    (ToR Switch / Router)
                                           ^
                                           | BGP session
                                           | (eBGP or iBGP)
                                           |
                    +----------------------+----------------------+
                    |              Control Plane Node              |
                    |                                              |
                    |  +-------------+       +------------------+  |
                    |  |  kube-vip   |       |    frr-k8s       |  |
                    |  | (static pod)|       |   (static pod)   |  |
                    |  |             |       |                  |  |
                    |  | Writes VIP  |       | Imports table 198|  |
                    |  | routes to   |       | via zebra;       |  |
                    |  | table 198   |       | advertises VIPs  |  |
                    |  | via netlink |       | via BGP to peers |  |
                    |  +------+------+       +---+---------+----+  |
                    |         |                  |         |        |
                    |         v                  |         |        |
                    |  +------+------+           |         |        |
                    |  | Linux       |   imports |         |        |
                    |  | Routing     |<----------+         |        |
                    |  | Table 198   |                     |        |
                    |  +-------------+                     |        |
                    |                            installs  |        |
                    |                         BGP-learned  |        |
                    |                          routes into  |        |
                    |  +------------------+   main/VRF tbl |        |
                    |  | OVN-Kubernetes   |<---------------+        |
                    |  | (reads routes    |                         |
                    |  |  with proto BGP  |                         |
                    |  |  from main/VRF   |                         |
                    |  |  tables via      |                         |
                    |  |  netlink)        |                         |
                    |  +------------------+                         |
                    +-----------------------------------------------+
```

In this architecture, data flows as follows:

1. kube-vip writes VIP `/32` (or `/128`) routes into Linux routing table 198
   via netlink.
2. frr-k8s (zebra) imports routes from table 198 via `ip import-table 198`
   and BGP redistributes them to external peers via `redistribute table-direct
   198`.
3. For BGP-learned routes from external peers (inbound), FRR installs them
   into the main kernel routing table or VRF-specific tables with protocol
   `RTPROT_BGP`.
4. OVN-Kubernetes monitors the main/VRF kernel routing tables via netlink,
   filtering for routes with protocol `RTPROT_BGP` (not table 198), and
   programs them into OVN logical routers. OVN-Kubernetes has no direct
   interaction with kube-vip's table 198.

### Component Changes

#### 1. frr-k8s (openshift/frr) -- Static Pod Deployment

frr-k8s must be modified to support deployment as a static pod. This is
required because during the OpenShift bootstrap sequence, the Kubernetes API
server is not yet available, so regular Pods (including DaemonSets) cannot be
scheduled. Static pods are managed directly by the kubelet from manifest files
in `/etc/kubernetes/manifests/`.

Changes required:

- **Static pod manifest generation**: The installer will generate the initial
  static pod manifest for frr-k8s and place it in
  `/etc/kubernetes/manifests/` on the bootstrap node and initial control plane
  nodes. Post-bootstrap, the Machine Config Operator (MCO) owns these
  manifests. MCO will render updated MachineConfig resources containing the
  frr-k8s static pod manifest for control plane nodes, ensuring the manifests
  are updated during cluster upgrades and placed on any new control plane
  nodes (scale-up or replacement). The manifest will configure frr-k8s with
  host networking (`hostNetwork: true`) and the required security capabilities
  (`NET_ADMIN`, `NET_RAW`, `SYS_ADMIN`, `NET_BIND_SERVICE`).

  MCO currently renders keepalived static pod manifests for on-prem platforms
  (baremetal, openstack, vsphere, nutanix) via `appendManifestsByPlatform()`
  in `pkg/operator/bootstrap.go`. MCO will be modified to conditionally render
  either the keepalived manifests (default) or the kube-vip + frr-k8s
  manifests, based on whether BGP VIP management is enabled. When BGP VIP
  management is active, MCO will skip rendering keepalived manifests and
  instead render the kube-vip and frr-k8s static pod manifests along with the
  bootstrap `frr.conf`. This is a required change in the MCO codebase.

- **Bootstrap FRR configuration**: Since `FRRConfiguration` CRDs are not
  available during bootstrap (no API server), the installer will generate a
  static FRR configuration file (`frr.conf`) from the BGP peering parameters
  in `install-config.yaml`. This file will be mounted into the frr-k8s static
  pod via a `hostPath` volume. The configuration will include:
  - Router ID (derived from the node's primary IP)
  - Local ASN
  - BGP neighbor definitions (address, remote ASN, password, BFD profile)
  - Route import from kernel table 198 (kube-vip's routing table)
  - Route filtering to only advertise VIP routes (`/32` or `/128`)

- **Transition to CRD-based configuration**: The frr-k8s configuration
  lifecycle has three distinct phases, each with a clear owner:

  1. **Bootstrap (owner: installer + MCO):** The installer generates the
     initial `frr.conf` and the frr-k8s static pod manifest. MCO renders
     these into MachineConfig resources and places them on nodes. During this
     phase, frr-k8s runs entirely from the static `frr.conf` on disk. No
     API server or CRDs are involved.

  2. **Handover (owner: CNO):** Once the API server is available and frr-k8s
     CRDs are registered, CNO performs the transition:
     - CNO creates `FRRConfiguration` CRs (named with a `bgp-vip-` prefix
       and labeled with `app.kubernetes.io/managed-by: cluster-network-operator`)
       that replicate the bootstrap BGP peering configuration.
     - CNO waits for the `FRRNodeState` CR on each node to report that the
       CRD-based configuration has been applied and BGP sessions are
       established.
     - Once verified, CNO updates the MCO MachineConfig to remove the static
       `frr.conf` override from the frr-k8s static pod volume mounts. This
       MachineConfig change must be applied without rebooting the node, since
       a reboot would disrupt the VIP and the API server. MCO supports
       non-reboot file changes for static pod configurations (similar to how
       it handles updates to other on-prem static pods like keepalived). The
       frr-k8s static pod will be restarted by the kubelet when its manifest
       changes on disk, picking up the CRD-based configuration seamlessly.
     - If verification fails (e.g., BGP sessions do not come up with the
       CRD-based config), CNO leaves the static `frr.conf` in place and
       reports a degraded condition. The static config acts as a safety net.

  3. **Steady state (owners: CNO for VIP config, OVN-K and MetalLB for their
     respective configs):** frr-k8s runs from CRD-based configuration. Each
     consumer owns its own `FRRConfiguration` CRs, identified by distinct
     name prefixes and `managed-by` labels:
     - `bgp-vip-*` owned by CNO (VIP advertisement)
     - `route-advertisements-*` owned by OVN-Kubernetes
     - `metallb-*` owned by MetalLB operator
     frr-k8s merges all applicable CRs for the node into a single FRR
     configuration. Consumers must not modify or delete CRs they do not own.

- **Single frr-k8s instance per node**: There must be exactly one frr-k8s
  instance running on each node. When frr-k8s is deployed as a static pod on
  control plane nodes, CNO must not also deploy frr-k8s as a DaemonSet on
  those same nodes. CNO will be modified during implementation to add
  detection logic (e.g., checking for a node label set by MCO when it renders
  the static pod MachineConfig, or querying the kubelet mirror pod) and skip
  DaemonSet scheduling on those nodes. This detection code does not exist in
  CNO today and will be added as part of this enhancement's implementation.
  The single frr-k8s instance is shared by all consumers (VIP advertisement,
  MetalLB, OVN-Kubernetes route advertisements) via additive
  `FRRConfiguration` CRs.

#### 2. kube-vip -- Routing Table Mode Deployment

kube-vip will be deployed as a static pod on each control plane node,
configured to use Routing Table Mode. As with frr-k8s, the installer generates
the initial manifest for bootstrap, and MCO owns the manifest post-bootstrap
via MachineConfig resources for control plane nodes. This ensures kube-vip is
updated during cluster upgrades and deployed on new control plane nodes.

In this mode:

- kube-vip participates in Kubernetes leader election to determine which node
  owns the API VIP. The same leader election mechanism is used for the Ingress
  VIP. In this first iteration, VIP ownership follows an active/passive model:
  only one node at a time holds a given VIP.
- When the local node is elected leader for a VIP, kube-vip writes a route
  for the VIP address to Linux routing table 198 via netlink.
- When leadership is lost, kube-vip removes the route from table 198.
- `vip_cleanroutingtable: "true"` is enabled to clean stale routes at startup.

kube-vip requires:
- `hostNetwork: true` for netlink access
- `NET_ADMIN` and `NET_RAW` capabilities
- Access to the kubeconfig file (`/etc/kubernetes/admin.conf` or
  `/etc/kubernetes/super-admin.conf` on Kubernetes >= 1.29)

#### 3. OVN-Kubernetes (openshift/ovn-kubernetes) -- Static Pod Compatibility

OVN-Kubernetes must be aware that frr-k8s may be deployed as a static pod
rather than a DaemonSet managed by CNO. The key changes:

- **Detection of static pod frr-k8s**: OVN-Kubernetes should detect when
  frr-k8s is already running as a static pod on a node and skip creating
  additional FRR-related pods for that node.

- **FRRConfiguration CR management**: When creating `FRRConfiguration` CRs for
  route advertisements (as described in
  [bgp-ovn-kubernetes.md](/enhancements/network/bgp-ovn-kubernetes.md)),
  OVN-Kubernetes must account for the pre-existing BGP peering configuration
  that was established during bootstrap. The CRs must be additive and not
  conflict with the VIP advertisement configuration. OVN-Kubernetes's
  RouteAdvertisements controller currently tracks ownership of
  `FRRConfiguration` CRs via annotations and may garbage-collect unrecognized
  CRs. During implementation, the VIP advertisement `FRRConfiguration` CRs
  will use distinct naming conventions and labels (e.g., prefixed with
  `bgp-vip-`) so that OVN-Kubernetes's reconciliation loop does not interfere
  with them.

- **Netlink route monitoring**: No changes needed. OVN-Kubernetes already
  monitors netlink for routes with protocol `bgp` and programs them into OVN
  logical routers. This works regardless of whether frr-k8s runs as a static
  pod or DaemonSet.

#### 4. OpenShift API (openshift/api) -- install-config.yaml Extension

The `install-config.yaml` schema will be extended to accept BGP peering
configuration for VIP advertisement. This configuration is provided at
installation time and is used by the installer to generate the static pod
manifests and FRR configuration for bootstrap.

Example `install-config.yaml` snippet:

```yaml
platform:
  baremetal:
    apiVIPs:
      - 192.168.111.5
    ingressVIPs:
      - 192.168.111.10
    bgpVIPConfig:
      localASN: 64512
      peers:
        - peerAddress: 192.168.111.1
          peerASN: 64512
          bfdEnabled: "true"
          password: "s3cret"
        - peerAddress: 192.168.111.2
          peerASN: 64512
          bfdEnabled: "true"
          password: "s3cret"
      communities:
        - "64512:100"
```

Proposed API additions to the platform-specific bare metal configuration:

```go
// BGPPeerConfig defines the configuration for a BGP peer.
type BGPPeerConfig struct {
    // peerAddress is the IP address of the BGP peer (e.g., ToR switch).
    // +kubebuilder:validation:Required
    PeerAddress string `json:"peerAddress"`

    // peerASN is the Autonomous System Number of the BGP peer.
    // Supports both 2-byte (1-65535) and 4-byte (1-4294967295) ASNs.
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=4294967295
    PeerASN int64 `json:"peerASN"`

    // password is the optional MD5 password for the BGP session.
    // This value is stored in plaintext in install-config.yaml and in the
    // generated frr.conf on the node filesystem. This is a known limitation:
    // during bootstrap, no API server is available to serve Kubernetes
    // Secrets, so a SecretReference cannot be used. The install-config.yaml
    // is treated as a sensitive artifact (it is stored as a Secret in the
    // kube-system namespace post-install), and the frr.conf file on disk is
    // readable only by root (mode 0600). Once the cluster transitions to
    // CRD-based configuration, the FRRConfiguration CRs use Kubernetes
    // Secrets for password storage (via passwordSecret), and the plaintext
    // frr.conf is removed from the node.
    // +optional
    Password string `json:"password,omitempty"`

    // port is the TCP port for the BGP session. Defaults to 179.
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=65535
    // +kubebuilder:default=179
    // +optional
    Port int32 `json:"port,omitempty"`

    // bfdEnabled configures Bi-directional Forwarding Detection for fast
    // failure detection on this peer session. Valid values are "true" and
    // "false". When omitted, the default is "false".
    // +kubebuilder:validation:Enum="true";"false"
    // +kubebuilder:default="false"
    // +optional
    BFDEnabled string `json:"bfdEnabled,omitempty"`

    // ebgpMultiHop configures multi-hop eBGP for this peer when the peer
    // is not directly connected. Valid values are "true" and "false".
    // When omitted, the default is "false".
    // +kubebuilder:validation:Enum="true";"false"
    // +kubebuilder:default="false"
    // +optional
    EBGPMultiHop string `json:"ebgpMultiHop,omitempty"`

    // holdTime is the BGP hold time for this peer. Defaults to 90s.
    // +optional
    HoldTime *metav1.Duration `json:"holdTime,omitempty"`

    // keepaliveTime is the BGP keepalive interval for this peer. Defaults to 30s.
    // +optional
    KeepaliveTime *metav1.Duration `json:"keepaliveTime,omitempty"`
}

// BGPVIPConfig configures BGP-based VIP advertisement.
type BGPVIPConfig struct {
    // localASN is the Autonomous System Number for this cluster's BGP speaker.
    // Supports both 2-byte (1-65535) and 4-byte (1-4294967295) ASNs.
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Minimum=1
    // +kubebuilder:validation:Maximum=4294967295
    LocalASN int64 `json:"localASN"`

    // peers is the list of BGP peers to advertise VIPs to.
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MinItems=1
    // +kubebuilder:validation:MaxItems=16
    // +listType=atomic
    Peers []BGPPeerConfig `json:"peers"`

    // communities is an optional list of BGP communities to attach to
    // advertised VIP routes, in the format "ASN:value" (e.g., "64512:100").
    // +optional
    // +kubebuilder:validation:MaxItems=32
    // +listType=atomic
    Communities []string `json:"communities,omitempty"`
}

```

The types above (`BGPPeerConfig`, `BGPVIPConfig`) are added to the installer's
bare metal platform types in `installer/pkg/types/baremetal/platform.go`
(the `Platform` struct), which is the type that parses `install-config.yaml`.
This is distinct from the runtime `config/v1.BareMetalPlatformSpec` in
`openshift/api` which represents the Infrastructure CR.

#### Per-Node BGP Peer Configuration

In multi-rack bare metal deployments, each control plane node typically peers
with its local ToR (Top of Rack) switch rather than a shared set of peers. To
support this, the existing `Host` struct in the installer's bare metal platform
types will be extended with an optional per-host BGP peer override:

```go
// Host stores the configuration for a bare metal host
// (installer/pkg/types/baremetal/platform.go).
type Host struct {
    // ... existing fields (Name, BMC, Role, BootMACAddress, NetworkConfig, etc.) ...

    // bgpPeers overrides the global bgpVIPConfig.peers for this specific
    // host. When set, this host will peer with the listed BGP peers instead
    // of the global peer list. This is useful in multi-rack deployments
    // where each node peers with its local ToR switch.
    // When omitted, the host uses the global bgpVIPConfig.peers.
    // +openshift:enable:FeatureGate=BGPBasedVIPManagement
    // +optional
    // +kubebuilder:validation:MaxItems=16
    // +listType=atomic
    BGPPeers []BGPPeerConfig `json:"bgpPeers,omitempty"`
}
```

The per-host peer override works as follows:

- **Bootstrap phase:** The installer generates a per-node `frr.conf` for each
  host. If `host.bgpPeers` is set, that host's `frr.conf` uses the
  host-specific peers; otherwise, it uses the global `bgpVIPConfig.peers`.
  MCO renders the correct `frr.conf` for each node via per-node MachineConfig.

- **Post-bootstrap CRD phase:** CNO creates per-node `FRRConfiguration` CRs
  with `nodeSelector` matching individual nodes, each containing the
  appropriate peer list for that node. The `FRRConfiguration` CRD natively
  supports `nodeSelector`, so per-node peering requires no frr-k8s changes.

Example `install-config.yaml` with per-node peers in a multi-rack deployment:

```yaml
platform:
  baremetal:
    apiVIPs:
      - 192.168.111.5
    ingressVIPs:
      - 192.168.111.10
    bgpVIPConfig:
      localASN: 64512
      peers:
        - peerAddress: 192.168.1.1
          peerASN: 64512
    hosts:
      - name: master-0
        role: master
        bootMACAddress: "00:aa:bb:cc:dd:01"
        bmc:
          address: "ipmi://192.168.111.101"
          username: admin
          password: password
        bgpPeers:
          - peerAddress: 192.168.1.1
            peerASN: 64512
            bfdEnabled: "true"
      - name: master-1
        role: master
        bootMACAddress: "00:aa:bb:cc:dd:02"
        bmc:
          address: "ipmi://192.168.111.102"
          username: admin
          password: password
        bgpPeers:
          - peerAddress: 192.168.2.1
            peerASN: 64512
            bfdEnabled: "true"
      - name: master-2
        role: master
        bootMACAddress: "00:aa:bb:cc:dd:03"
        bmc:
          address: "ipmi://192.168.111.103"
          username: admin
          password: password
        bgpPeers:
          - peerAddress: 192.168.3.1
            peerASN: 64512
            bfdEnabled: "true"
```

Additionally, the runtime Infrastructure CR (`infrastructure.config.openshift.io`)
will be extended to expose the BGP VIP management state in its `status` section.
This allows cluster administrators and support engineers to inspect whether
a cluster is using BGP-based VIP management via `oc get infrastructure cluster`:

```go
// BareMetalPlatformStatus holds the status of the BareMetal platform
// (config/v1.BareMetalPlatformStatus in openshift/api).
type BareMetalPlatformStatus struct {
    // ... existing fields (APIServerInternalIPs, IngressIPs, etc.) ...

    // vipManagement indicates which VIP management mechanism is active
    // on this cluster. Valid values are "Keepalived" and "BGP".
    // +openshift:enable:FeatureGate=BGPBasedVIPManagement
    // +optional
    VIPManagement string `json:"vipManagement,omitempty"`

    // bgpVIPStatus reports the observed state of BGP-based VIP management.
    // This field is only populated when vipManagement is "BGP".
    // +openshift:enable:FeatureGate=BGPBasedVIPManagement
    // +optional
    BGPVIPStatus *BGPVIPStatus `json:"bgpVIPStatus,omitempty"`
}

// BGPVIPStatus reports the observed state of BGP-based VIP management.
type BGPVIPStatus struct {
    // localASN is the Autonomous System Number configured for this cluster.
    // +optional
    LocalASN int64 `json:"localASN,omitempty"`

    // peers reports the configured BGP peer addresses.
    // +optional
    // +listType=atomic
    Peers []string `json:"peers,omitempty"`
}
```

This status is populated by CNO after the bootstrap-to-CRD handover completes,
providing a single API-level signal for whether the cluster uses BGP or
keepalived for VIP management.

```go
// Platform stores the platform-specific configuration for the
// bare metal platform (installer/pkg/types/baremetal/platform.go).
type Platform struct {
    // ... existing fields (LibvirtURI, Hosts, APIVIPs, IngressVIPs, etc.) ...

    // bgpVIPConfig configures BGP-based advertisement of the API and
    // Ingress VIPs. When set, kube-vip (Routing Table Mode) and frr-k8s
    // are deployed as static pods on control plane nodes to advertise
    // VIPs via BGP, replacing the default keepalived/VRRP mechanism.
    // +openshift:enable:FeatureGate=BGPBasedVIPManagement
    // +optional
    BGPVIPConfig *BGPVIPConfig `json:"bgpVIPConfig,omitempty"`
}
```

### Workflow Description

**cluster creator** is a human user responsible for deploying an OpenShift
cluster on bare metal infrastructure with BGP-capable network equipment.

**network engineer** is a human user responsible for the external network
infrastructure (ToR switches, routers) and BGP configuration on those devices.

#### Installation Workflow

1. The network engineer configures the ToR switches / routers with BGP peering
   sessions that will accept connections from the OpenShift control plane nodes.
   This includes configuring the peer ASN, any required route filters, and
   optionally BFD.

2. The cluster creator prepares the `install-config.yaml` with the standard
   bare metal platform configuration (API VIPs, Ingress VIPs, BMC credentials,
   etc.) and adds the `bgpVIPConfig` section specifying the local ASN, peer
   addresses, peer ASN, and optional BGP session parameters.

3. The cluster creator runs `openshift-install create cluster`.

4. The installer validates the `bgpVIPConfig` parameters and generates:
   - A kube-vip static pod manifest configured for Routing Table Mode with the
     API VIP address.
   - A frr-k8s static pod manifest with a bootstrap `frr.conf` that
     configures BGP peering using the parameters from `install-config.yaml` and
     imports routes from kernel table 198.
   - Both manifests are placed in `/etc/kubernetes/manifests/` on the bootstrap
     node and subsequently on each control plane node.

5. The bootstrap node starts. The kubelet launches the kube-vip and frr-k8s
   static pods. As the only kube-vip instance, the bootstrap node immediately
   self-elects as leader and writes the API VIP route to table 198 via
   netlink (no API server dependency). frr-k8s establishes BGP sessions with
   the configured peers using the static `frr.conf` and begins advertising
   the API VIP route.

6. The API VIP becomes reachable via BGP before the API server starts. The
   kube-apiserver starts and becomes accessible at the API VIP.

7. The remaining control plane nodes are provisioned. Each receives the same
   static pod manifests. Since the API server is already reachable via the
   VIP, kube-vip on each node participates in normal Kubernetes leader
   election; only the elected leader writes the API VIP route to table 198.

8. Once the cluster is fully operational, CNO detects the BGP VIP configuration
   and creates the corresponding `FRRConfiguration` CRs to formalize the
   bootstrap configuration. It also deploys kube-vip configuration for the
   Ingress VIP.

#### Failover Workflow

1. A control plane node holding the API VIP fails.
2. If BFD is enabled, the external peer detects the failure within milliseconds
   and withdraws the route.
3. kube-vip on remaining nodes detects the leadership vacancy and elects a new
   leader. The new leader writes the API VIP route to its local table 198.
4. frr-k8s on the new leader advertises the route to external peers.
5. External peers install the new route. Traffic converges to the new node.

#### Error Handling

- If BGP peering cannot be established during bootstrap (misconfigured peer,
  network issue), the frr-k8s pod will log the failure and FRRNodeState will
  report the session as down. The cluster installation will stall because the
  API VIP is not reachable. The cluster creator must fix the peering
  configuration and retry.

- If a BGP session flaps post-installation, frr-k8s will withdraw and
  re-advertise routes according to standard BGP behavior. BFD (if enabled) will
  accelerate failure detection. The `FRRNodeState` CR and frr-k8s metrics will
  expose the session state for monitoring.

- If BGP peering fails permanently (e.g., misconfigured external peers,
  network infrastructure failure), the API VIP becomes unreachable via BGP.
  In this scenario, the cluster admin can regain access to the cluster by
  connecting directly to any control plane node's IP address on port 6443
  (the kube-apiserver listens on all interfaces, not only on the VIP). This
  allows the admin to run `oc` or `kubectl` commands by specifying the node
  IP explicitly (e.g., `oc --server=https://<node-ip>:6443`) or via SSH to
  the node and using the local kubeconfig. From there, the admin can
  diagnose and fix the BGP configuration (e.g., update `FRRConfiguration`
  CRs, inspect frr-k8s logs, or correct external peer settings).

### API Extensions

This enhancement introduces the following API changes:

- **install-config.yaml (openshift/api)**: New `bgpVIPConfig` field under
  `platform.baremetal` as described in the API section above. This is a
  configuration-time-only field consumed by the installer.

- **FRRConfiguration CRs (frr-k8s)**: No new CRD is introduced. Existing
  `FRRConfiguration` CRDs from frr-k8s are used. The enhancement requires that
  frr-k8s CRDs are registered once the API server is available, so that CNO can
  manage `FRRConfiguration` resources for day-2 operations.

- **No new runtime CRDs for kube-vip**: kube-vip is configured entirely via
  environment variables in its static pod manifest. No CRD is required.

This enhancement does not modify the behavior of any existing API resources.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement does not apply to Hypershift. In Hypershift, the control plane
runs in a management cluster and VIPs are managed by the hosting infrastructure.
No changes are needed for Hypershift.

#### Standalone Clusters

This is the primary target for this enhancement. Standalone bare metal clusters
that currently use keepalived for VIP management can opt into BGP-based VIP
management via `install-config.yaml`.

#### Single-node Deployments or MicroShift

For single-node OpenShift (SNO), BGP-based VIP management is applicable but has
reduced utility since there is only one node to advertise the VIP. It can still
be useful for integration with external BGP fabrics. The additional resource
footprint of kube-vip and frr-k8s static pods is approximately 500 MiB memory
and 250m CPU combined (based on the existing frr-k8s DaemonSet resource
requests: ~400 MiB across the frr, controller, reloader, metrics, and
kube-rbac-proxy containers, plus kube-vip overhead). On resource-constrained
SNO deployments, this should be evaluated against the cluster's available
capacity.

This enhancement does not affect MicroShift, which does not use the OpenShift
installer or the VIP management infrastructure.

#### Disconnected / Air-Gapped Environments

This enhancement requires no special handling for disconnected environments.
The kube-vip and frr-k8s container images are included in the OpenShift release
payload. In disconnected deployments, these images are mirrored alongside all
other release payload images using the standard `oc-mirror` workflow. The
bootstrap node and control plane nodes pull the static pod images from the
mirrored registry, identical to how other static pod images (etcd,
kube-apiserver, etc.) are handled in disconnected installations.

#### OpenShift Kubernetes Engine

This enhancement is applicable to OKE on bare metal. It does not depend on any
features excluded from OKE.

### Implementation Details/Notes/Constraints

#### Feature Gate

This feature is gated behind the `BGPBasedVIPManagement` feature gate. The
feature gate controls visibility of the `bgpVIPConfig` field in
`BareMetalPlatformSpec`. When the gate is disabled, the field is not accepted
by the API and the installer rejects `install-config.yaml` files that include
it.

The feature will start in the `TechPreviewNoUpgrade` FeatureSet. This means:

- The feature is available only on clusters installed with
  `featureSet: TechPreviewNoUpgrade` in `install-config.yaml`.
- Clusters with this FeatureSet cannot be upgraded to future minor versions
  (this is a standard Tech Preview constraint).
- Once the feature graduates to GA, the feature gate will be enabled by
  default and the FeatureSet restriction will be removed.

#### Bootstrap Sequence and API Server Dependency

A key concern with VIP management is the apparent circular dependency: the API
VIP must be reachable before the API server starts, but kube-vip uses
Kubernetes leader election which requires the API server. This is resolved the
same way the existing keepalived bootstrap works -- by separating the
network-level VIP operations from the Kubernetes API dependency:

1. **No circular dependency exists.** Writing a route to kernel table 198 via
   netlink and advertising it via BGP are pure network-level operations. Neither
   requires the Kubernetes API server. This is analogous to how keepalived uses
   VRRP (a pure L2/L3 protocol) to claim the VIP independently of the API
   server.

2. **Bootstrap node (single instance):** On the bootstrap node, kube-vip is
   the only instance running. Kubernetes leader election with a single
   candidate results in immediate self-election -- kube-vip wins leadership
   without needing to contact the API server and writes the VIP route to table
   198 immediately. frr-k8s then advertises this route via BGP using the static
   `frr.conf` configuration. The API VIP becomes reachable on the network
   before the API server starts.

3. **Control plane nodes (multiple instances):** By the time the remaining
   control plane nodes boot, the API server is already reachable via the VIP
   that the bootstrap node advertised. kube-vip on these nodes can use normal
   Kubernetes leader election via the API to coordinate VIP ownership.

4. **Bootstrap teardown:** The bootstrap node runs with a higher leader
   election priority (analogous to keepalived's priority 70 vs. 40 for
   masters). When the bootstrap node is shut down, kube-vip on one of the
   control plane nodes wins the election and takes over the VIP.

This sequence mirrors the existing keepalived bootstrap flow where keepalived
starts the VRRP daemon independently of the API server, claims the VIP via
unicast VRRP, and only uses the API for dynamic peer discovery after the
cluster is operational.

#### Static Pod Startup Ordering

The kubelet starts static pods in alphabetical order by manifest filename.
To ensure frr-k8s is running and ready to advertise routes before kube-vip
writes VIP routes to table 198, the static pod manifests are named to
enforce the correct ordering:

1. `frr-k8s.yaml` -- starts first (alphabetically before `kube-vip`)
2. `kube-vip.yaml` -- starts second, after frr-k8s

This ordering ensures that by the time kube-vip writes a VIP route to table
198, frr-k8s (with zebra importing table 198) is already running and will
pick up the route for BGP advertisement. If kube-vip were to start first,
there would be a brief window where the VIP route exists in table 198 but is
not advertised via BGP. While this window is harmless (frr-k8s would
advertise the route as soon as it starts), the explicit ordering eliminates
it entirely.

#### kube-vip Routing Table Mode Details

kube-vip in Routing Table Mode operates as follows:

- All other VIP modes (ARP, BGP native) are disabled;
  only `vip_routingtable: "true"` is set.
- Routes are managed via netlink in routing table ID `198` (configurable via
  `vip_routingtableid`) with routing protocol ID `248` (configurable via
  `vip_routingtableprotocol`).
- Leader election (`vip_leaderelection: "true"`) determines which single node
  writes the VIP route. Only the elected leader has the VIP route in table 198;
  all other nodes do not, ensuring an active/passive model.

Note: kube-vip also supports an ECMP multi-homing mode (by disabling both
`vip_leaderelection` and `svc_election`), where all nodes simultaneously
advertise the VIP. This mode is **not in scope for the first iteration** of
this enhancement and is deferred to a future phase. The first iteration uses
leader-elected, single-node VIP ownership exclusively.

#### frr-k8s Bootstrap Configuration

The bootstrap `frr.conf` generated by the installer will follow this structure:

```
frr defaults traditional
hostname <node-hostname>
log syslog informational

! Import routes from kube-vip's routing table (198) into zebra's RIB.
! Without this, 'redistribute kernel' only reads the main table (254).
ip import-table 198

router bgp <localASN>
 bgp router-id <node-primary-ip>
 neighbor <peer1-ipv4-address> remote-as <peer1-asn>
 neighbor <peer1-ipv4-address> password <peer1-password>
 neighbor <peer1-ipv6-address> remote-as <peer1-asn>
 neighbor <peer1-ipv6-address> password <peer1-password>
 !
 address-family ipv4 unicast
  redistribute table-direct 198 route-map KUBE-VIP-ROUTES-V4
  neighbor <peer1-ipv4-address> activate
 exit-address-family
 !
 address-family ipv6 unicast
  redistribute table-direct 198 route-map KUBE-VIP-ROUTES-V6
  neighbor <peer1-ipv6-address> activate
 exit-address-family
!
route-map KUBE-VIP-ROUTES-V4 permit 10
 match ip address prefix-list KUBE-VIP-PREFIXES-V4
!
route-map KUBE-VIP-ROUTES-V6 permit 10
 match ipv6 address prefix-list KUBE-VIP-PREFIXES-V6
!
route-map KUBE-VIP-ROUTES-V4 deny 20
!
route-map KUBE-VIP-ROUTES-V6 deny 20
!
ip prefix-list KUBE-VIP-PREFIXES-V4 seq 10 permit <api-vip-v4>/32
ip prefix-list KUBE-VIP-PREFIXES-V4 seq 20 permit <ingress-vip-v4>/32
!
ipv6 prefix-list KUBE-VIP-PREFIXES-V6 seq 10 permit <api-vip-v6>/128
ipv6 prefix-list KUBE-VIP-PREFIXES-V6 seq 20 permit <ingress-vip-v6>/128
```

The key configuration elements are:

- **`ip import-table 198`**: Instructs zebra to import routes from kernel
  routing table 198 (where kube-vip writes VIP routes) into FRR's RIB.
  Without this directive, FRR only sees routes from the main kernel table
  (table 254) and would never observe kube-vip's routes.

- **`redistribute table-direct 198`**: Redistributes routes imported from
  table 198 into BGP, filtered through route-maps. The `table-direct`
  variant directly references the imported table rather than relying on
  `redistribute kernel` which only covers the main table.

- **Route-maps with explicit deny**: Each route-map ends with a `deny 20`
  entry to ensure that only VIP prefixes matching the prefix-lists are
  advertised. Any routes in table 198 that do not match the VIP `/32` or
  `/128` prefixes are silently dropped.

The configuration supports both IPv4 and IPv6 address families for dual-stack
deployments. When only single-stack is configured, the installer will generate
only the relevant address family block.

#### FRR Daemon Startup Options

The existing frr-k8s DaemonSet starts the BGP daemon with `-p 0` (no listening
port), since frr-k8s manages BGP configuration programmatically. For
BGP-based VIP management, the FRR daemon needs to actively peer with external
routers, which requires listening on port 179 and initiating outbound BGP
connections. The frr-k8s static pod will be configured with updated daemon
startup options (removing `-p 0`) to enable standard BGP peering. These
changes will be implemented in the frr-k8s static pod manifest and the
associated FRR daemon configuration files (`daemons`) as part of this
enhancement.

#### Static Pod Security Context

Both kube-vip and frr-k8s static pods require privileged access:

```yaml
securityContext:
  capabilities:
    add:
      - NET_ADMIN
      - NET_RAW
      - SYS_ADMIN
      - NET_BIND_SERVICE
hostNetwork: true
```

The `SYS_ADMIN` capability is required by the FRR daemon for network namespace
operations, and `NET_BIND_SERVICE` is required for binding to BGP port 179.

On OpenShift, a Security Context Constraint (SCC) permitting these capabilities
must be available. Since static pods run as part of the kubelet and are not
subject to admission webhooks, the SCC is implicitly `privileged`.

#### Integration with Existing AdditionalRoutingCapabilities API

The [bgp-ovn-kubernetes](/enhancements/network/bgp-ovn-kubernetes.md)
enhancement introduced the `AdditionalRoutingCapabilities` API in
`network.operator.openshift.io` to signal CNO to deploy FRR and frr-k8s.

When `bgpVIPConfig` is set in `install-config.yaml`, the installer will:
1. Set `additionalRoutingCapabilities.providers: ["FRR"]` in the Network CR.
2. CNO will detect that frr-k8s is already running as a static pod (via node
   annotation or presence of the static pod) and will not deploy a duplicate
   frr-k8s DaemonSet on those nodes.
3. CNO will still create the frr-k8s namespace and CRDs so that day-2
   `FRRConfiguration` management works.

#### Cross-Team Dependencies

This enhancement requires changes across multiple components owned by different
teams. The authoring team will coordinate with each team as needed during
implementation:

| Component | Repository | Team | Changes Required |
|-----------|-----------|------|------------------|
| OpenShift Installer | `openshift/installer` | Installer | `install-config.yaml` schema extension, bootstrap manifest generation, `bgpVIPConfig` validation |
| OpenShift API | `openshift/api` | API | `BGPVIPConfig` types in installer types, `BareMetalPlatformStatus` extension for Infrastructure CR |
| Machine Config Operator | `openshift/machine-config-operator` | MCO | Conditional rendering of kube-vip + frr-k8s static pod manifests instead of keepalived, MachineConfig lifecycle management |
| Cluster Network Operator | `openshift/cluster-network-operator` | Networking | Static pod frr-k8s detection logic, bootstrap-to-CRD handover, `FRRConfiguration` CR creation for VIP advertisement |
| OVN-Kubernetes | `openshift/ovn-kubernetes` | OVN | Awareness of static pod frr-k8s, avoiding DaemonSet conflicts, FRRConfiguration CR coexistence |
| frr-k8s | `openshift/frr` | MetalLB | Static pod deployment support, FRR daemon startup option changes |
| kube-vip | (new) | Networking | OpenShift-specific build, inclusion in release payload, Routing Table Mode validation |

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| BGP misconfiguration during bootstrap prevents cluster installation | The installer will validate BGP parameters. Clear error messages will guide the user. A fallback to keepalived can be documented. |
| kube-vip or frr-k8s static pod crashes during bootstrap | Both components will be configured with restart policies. The kubelet will automatically restart failed static pods. |
| Conflict between static pod frr-k8s and DaemonSet frr-k8s | CNO will detect static pod deployment and skip DaemonSet creation on affected nodes. |
| BGP session security (unauthenticated sessions) | MD5 password authentication is supported and should be recommended in documentation. |
| Route leaking -- kube-vip advertising unintended routes | The FRR configuration uses strict route-maps and prefix-lists to only advertise VIP `/32` routes. |
| Upstream kube-vip project stability | OpenShift will vendor a specific, tested version of kube-vip. The Routing Table Mode is the simplest mode with minimal moving parts. |
| FRR-K8s API gaps for bootstrap scenario | Bootstrap uses static `frr.conf` to avoid dependency on the API server. CRD-based management takes over post-bootstrap. |
| Split-brain: network partition causes two nodes to believe they hold the VIP | See detailed analysis below. |

#### Split-Brain Behavior During Network Partitions

During a network partition, two or more kube-vip instances may temporarily
believe they are the leader and write the VIP route to their local table 198.
This would cause multiple frr-k8s instances to advertise the same VIP `/32`
to external BGP peers, resulting in multiple BGP routes for the same
destination with different next-hops.

**Impact on traffic:** External routers receiving the same prefix from multiple
next-hops will treat this as ECMP and distribute traffic across the
partitioned nodes. Since the API server runs on all control plane nodes, API
requests will succeed regardless of which node receives them -- the client
reaches a valid kube-apiserver instance either way. This is a transient
condition, not a data-loss scenario.

**Resolution:** Kubernetes leader election uses Lease objects with a
`leaseDurationSeconds` timeout. When the partition heals, the Lease state
converges: one node renews the Lease and the others observe they have lost
leadership. The losing nodes remove the VIP route from their local table 198,
frr-k8s withdraws the BGP advertisement, and traffic converges back to the
single leader. The convergence time is bounded by the Lease duration (default
15 seconds) plus BGP withdrawal propagation (typically sub-second with BFD,
or up to the hold timer without BFD).

**Comparison with keepalived:** The existing keepalived/VRRP model has the
same split-brain risk during network partitions -- multiple nodes may claim
the VIP via gratuitous ARP. The behavior and resolution are analogous, with
VRRP advertisement timers playing the role of Lease duration.

### Drawbacks

- **Increased complexity**: Two additional static pods (kube-vip, frr-k8s) on
  each control plane node adds operational complexity compared to keepalived.
  Troubleshooting requires understanding of BGP concepts.

- **External infrastructure dependency**: BGP-based VIP management requires
  functional BGP peers on the external network. A misconfigured ToR switch or
  router can make VIPs unreachable. This is a shared responsibility between the
  cluster administrator and the network team.

- **Bootstrap fragility**: The bootstrap sequence becomes dependent on
  successful BGP peering. Unlike keepalived which only requires L2 connectivity,
  BGP requires correctly configured peers on both sides.

- **Not a drop-in replacement**: Users must opt in and provide BGP peering
  details. This is not a transparent upgrade from keepalived; it requires
  network infrastructure changes.

## Alternatives (Not Implemented)

### kube-vip in Native BGP Mode

kube-vip supports a built-in BGP mode using goBGP where it directly advertises
VIPs to BGP peers without requiring a separate routing daemon. This was not
selected because:

- It duplicates BGP speaker functionality that frr-k8s already provides.
- It would create conflicts with frr-k8s when OVN-Kubernetes BGP integration
  is also enabled (two BGP speakers on the same node).
- FRR (via frr-k8s) is a more mature and feature-complete BGP implementation
  than goBGP, supporting features like BFD, communities, route-maps, and VRF.

### Replacing keepalived with BGP-only (No kube-vip)

An alternative would be to have frr-k8s directly manage VIP assignment and
advertisement without kube-vip. This was not selected because:

- kube-vip provides a clean abstraction for VIP lifecycle management (leader
  election, health checking, VIP assignment) that would need to be reimplemented.
- The Routing Table Mode cleanly separates concerns: kube-vip handles VIP
  lifecycle, frr-k8s handles route advertisement.

### Using MetalLB for VIP Advertisement

MetalLB is already used in OpenShift for advertising `type=LoadBalancer` Service
IPs via BGP, and it supports both FRR and FRR-K8s backends. An alternative
would be to use MetalLB directly for VIP advertisement instead of introducing
kube-vip. This was not selected because MetalLB fundamentally cannot operate
during the OpenShift bootstrap phase:

- **MetalLB is a day-2 operator**: MetalLB is deployed via the MetalLB Operator
  through OLM (Operator Lifecycle Manager). Its components -- the `controller`
  Deployment (which handles IP address assignment from configured pools) and the
  `speaker` DaemonSet (which announces addresses via ARP/NDP or BGP) -- are
  regular Kubernetes workloads that require a functioning API server and
  scheduler. During bootstrap, neither is available.

- **No static pod support**: MetalLB has no mechanism to run as a static pod.
  Its controller requires access to Kubernetes Services and Endpoints resources
  to determine IP assignments, and the speaker requires RBAC-mediated access to
  node and service objects. These dependencies are structurally incompatible
  with the static pod model where no API server exists.

- **Bootstrap chicken-and-egg problem**: The API VIP must be reachable *before*
  the API server starts, because the API server itself is accessed via the VIP.
  MetalLB can only assign and advertise IPs for Services that already exist in
  a running cluster. It cannot bootstrap the VIP that the cluster needs to
  become operational in the first place.

- **Different abstraction level**: MetalLB manages Service LoadBalancer IPs,
  not infrastructure-level VIPs. The API and Ingress VIPs are not Kubernetes
  Services -- they are infrastructure endpoints that must exist independently
  of the cluster's workload lifecycle. kube-vip in Routing Table Mode is
  specifically designed for this infrastructure-level VIP management.

MetalLB remains the correct solution for day-2 Service LoadBalancer IP
advertisement and will continue to coexist with this enhancement, sharing the
same frr-k8s instance for BGP peering.

### Using BIRD Instead of FRR

BIRD is another routing daemon that could read routes from kube-vip's routing
table. It was not selected because frr-k8s is already an established component
in the OpenShift ecosystem with CRD-based configuration, and FRR is the
routing daemon used by MetalLB and OVN-Kubernetes BGP integration.

## Open Questions [optional]

1. When should ECMP support be introduced? What prerequisites (e.g., external
   health checking, graceful restart) must be in place before enabling
   multi-node VIP advertisement?

## Test Plan

**Note:** *Section not required until targeted at a release.*

Testing strategy will cover the following areas:

- **Unit tests**: Installer validation logic for `bgpVIPConfig` parameters.
  FRR configuration generation from `install-config.yaml` parameters.

- **Integration tests**: Static pod manifest generation and placement. frr-k8s
  static pod startup and FRR configuration loading. kube-vip routing table
  write/delete operations.

- **E2E tests**:
  - Full cluster installation with BGP-based VIP management using
    containerlab or similar to simulate BGP peers.
  - VIP failover scenarios (control plane node failure, BGP session loss).
  - BFD-backed fast failover timing validation.
  - Coexistence with MetalLB FRR-K8S mode.
  - Coexistence with OVN-Kubernetes BGP route advertisements.
  - Dual-stack (IPv4 + IPv6) VIP advertisement with both address families.

- **Scale testing**: Impact of frr-k8s static pod on bootstrap timing. BGP
  convergence time with multiple peers and multiple control plane nodes.

## Graduation Criteria

### Dev Preview -> Tech Preview

- `BGPBasedVIPManagement` feature gate available in `TechPreviewNoUpgrade`
  FeatureSet.
- Ability to install an OpenShift bare metal cluster with BGP-based VIP
  management end to end.
- Dual-stack (IPv4 + IPv6) VIP advertisement functional.
- Minimum 5 E2E tests covering installation, failover, and dual-stack
  scenarios, gated with `[OCPFeatureGate:BGPBasedVIPManagement]` test label.
- Tests running at least 7 times per week in CI.
- End user documentation for `install-config.yaml` BGP configuration.
- Metrics exposed for BGP session state monitoring.
- Symptoms-based alerts for BGP session failures.

### Tech Preview -> GA

- `BGPBasedVIPManagement` feature gate promoted to `Default` FeatureSet
  (enabled by default).
- Minimum 5 E2E tests running at least 7 times per week, across at least 14
  runs per supported platform, with a sustained 95% pass rate over the 14
  days prior to branch cut.
- Tests must cover: installation, VIP failover (with and without BFD),
  dual-stack, upgrade with MCO manifest rollout, and bootstrap-to-CRD
  handover.
- Load testing with production-representative topologies (multi-peer,
  multi-rack).
- User-facing documentation in
  [openshift-docs](https://github.com/openshift/openshift-docs/).
- Feedback from Tech Preview users incorporated.
- BFD fast failover validated under realistic failure scenarios.

### Removing a deprecated feature

This enhancement does not deprecate any existing feature. keepalived-based VIP
management remains the default.

## Upgrade / Downgrade Strategy

### Upgrade

- Existing clusters using keepalived will not be affected by this enhancement.
  No changes are required to maintain previous behavior on upgrade.
- For clusters using BGP-based VIP management, the kube-vip and frr-k8s static
  pod manifests are managed by MCO via MachineConfig resources. During a
  cluster upgrade, MCO renders updated MachineConfigs containing the new
  static pod manifests (with updated image references from the release
  payload). The machine-config-daemon rolls out these updates to control plane
  nodes following the standard MCO node drain and reboot strategy. BGP
  sessions will be briefly interrupted on each node during the rolling update;
  kube-vip leader election will migrate the VIP to a non-rebooting node,
  maintaining API availability throughout the upgrade.
- Migration from keepalived to BGP-based VIP management on an existing cluster
  is not supported in the first iteration. Cluster reinstallation is required
  to switch to BGP-based VIP management. A day-2 migration path (allowing
  in-place transition from keepalived to BGP) may be considered in a future
  enhancement.
- EUS-to-EUS upgrades (e.g., 4.14 -> 4.16 skipping 4.15) require no special
  handling. The static pod manifests are rendered by MCO from the target
  release payload, so intermediate versions are irrelevant -- MCO applies the
  final target version's manifests directly. The upgrade follows the same
  rolling node update strategy as any other MCO-managed static pod.

### Downgrade

Downgrading from BGP-based VIP management back to the keepalived model is not
supported. Once a cluster is installed with BGP-based VIP management enabled,
there is no supported path to revert to keepalived without a full cluster
reinstallation.

## Version Skew Strategy

During an upgrade:

- The static pod manifests for kube-vip and frr-k8s are managed by MCO via
  MachineConfig resources. MCO rolls out updated manifests as part of the
  standard node upgrade sequence. During a rolling upgrade, some control plane
  nodes will temporarily run the new version while others run the old version.
  This is safe because each node's kube-vip and frr-k8s operate independently
  -- they manage routes on their local node and peer with external BGP
  routers. There is no cross-node coordination between kube-vip or frr-k8s
  instances beyond Kubernetes leader election for VIP ownership.
- frr-k8s CRD versions must be compatible between the static pod frr-k8s on
  control plane nodes and any DaemonSet frr-k8s instances on worker nodes.
  Since MCO updates the static pods from the same release payload that CNO
  uses to update the DaemonSet, both will converge to the same version by the
  end of the upgrade.
- kube-vip is self-contained and does not have cross-component version
  dependencies beyond the Kubernetes API client.

## Operational Aspects of API Extensions

- The `bgpVIPConfig` field in `install-config.yaml` is consumed only at
  installation time by the installer. It does not create any runtime API
  extensions (no webhooks, no aggregated API servers, no finalizers).

- `FRRConfiguration` and `FRRNodeState` CRDs are provided by frr-k8s and are
  already covered by the
  [bgp-ovn-kubernetes](/enhancements/network/bgp-ovn-kubernetes.md)
  enhancement.

- The impact on existing SLIs is minimal:
  - No additional API server load from the install-config extension.
  - frr-k8s FRRNodeState updates are infrequent (on BGP state changes only).
  - kube-vip API watches for leader election leases are lightweight.

## Support Procedures

### Detecting Failure Modes

- **BGP session not established**: Check `FRRNodeState` CR for the node. Run
  `vtysh -c "show bgp summary"` inside the frr-k8s static pod container.
  Look for `BGP session not established` log messages from frr-k8s.

- **VIP not advertised**: Verify kube-vip has written the route to table 198:
  `ip route show table 198`. Verify frr-k8s is advertising it:
  `vtysh -c "show ip bgp"` inside the frr-k8s container. Check FRR route-map
  and prefix-list configuration.

- **VIP not reachable despite BGP session up**: Check external peer's routing
  table for the VIP route. Verify there are no conflicting route filters on the
  external peer. Check that the VIP subnet is not being filtered by the
  external peer's route policy.

- **Slow failover**: Verify BFD is enabled and operational:
  `vtysh -c "show bfd peers"`. Check BFD timers. If BFD is not enabled, BGP
  hold timer (default 90s) governs failover speed.

### Emergency Recovery When API VIP Is Unreachable

If BGP peering is permanently broken and the API VIP is not reachable, the
cluster admin can recover access by connecting directly to a control plane
node's IP:

1. SSH into a control plane node, or use `oc --server=https://<node-ip>:6443`
   with the appropriate kubeconfig to bypass the VIP.
2. Inspect frr-k8s logs: `crictl logs $(crictl ps --name frr -q)`.
3. Inspect kube-vip logs: `crictl logs $(crictl ps --name kube-vip -q)`.
4. Verify the BGP session state: `crictl exec <frr-container-id> vtysh -c "show bgp summary"`.
5. Verify routes in table 198: `ip route show table 198`.
6. Fix the root cause (update `FRRConfiguration` CRs, correct external peer
   settings, or resolve network infrastructure issues).

The kube-apiserver listens on all node interfaces (not only on the VIP), so
direct node IP access is always available as a recovery path regardless of
BGP state.

### Disabling the Feature

Reverting from BGP-based VIP management to keepalived is not supported without
a full cluster reinstallation. There is no in-place mechanism to switch back to
the keepalived model once the cluster has been installed with BGP-based VIP
management.

## Infrastructure Needed [optional]

- **kube-vip image**: An OpenShift-specific build of kube-vip must be produced
  and published to the OpenShift release image registry.

- **CI infrastructure**: BGP peer simulation infrastructure (e.g., containerlab
  with FRR-based peers) for E2E testing of BGP VIP management scenarios.

- **Bare metal CI environments**: Existing bare metal CI environments will need
  BGP-capable virtual network infrastructure for testing.
