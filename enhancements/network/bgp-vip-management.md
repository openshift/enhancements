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
last-updated: 2026-07-09
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

The advertisement model of the first iteration is health-gated ECMP: every
node whose local backend passes the kube-vip health check advertises the VIP,
and external routers distribute traffic across the advertising nodes with
equal-cost multipath. A node whose backend fails withdraws its own path within
one health-check interval, independently of the other nodes. Implementation
experience showed this is the natural behavior of kube-vip's Routing Table
Mode (the leader-election Lease does not gate the route reconciliation loop)
and that it works well in practice; a single-advertiser (active/passive) mode
would require an additional kube-vip change and is not part of this iteration.

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

8. Single-advertiser (active/passive) VIP ownership. Health-gated ECMP --
   every node with a healthy local backend advertises the VIP -- is the
   native behavior of kube-vip's Routing Table Mode and is the model of this
   first iteration. Restricting advertisement to a single elected node would
   require leadership-gating kube-vip's route reconciliation loop and is
   deferred until a concrete need for single-advertiser semantics appears.

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
  (`NET_ADMIN`, `NET_RAW`, `SYS_ADMIN`, `NET_BIND_SERVICE`). The frr-k8s
  static pod manifest includes an FRR config renderer init container
  ahead of the FRR daemon containers. It is analogous to the
  `keepalived-monitor` sidecar that `baremetal-runtimecfg` runs in the
  keepalived static pod: it renders the node-specific `frr.conf` at startup
  by discovering the local node's hostname and primary IP, resolving the
  correct BGP peer list from a per-node mapping file, and writing the
  rendered config to a shared `emptyDir` volume for the FRR daemon. See the
  "Runtime FRR Configuration Rendering" section in Implementation Details
  for the full mechanism.

  MCO currently renders keepalived static pod manifests for on-prem platforms
  (baremetal, openstack, vsphere, nutanix) via `appendManifestsByPlatform()`
  in `pkg/operator/bootstrap.go`. MCO will be modified to conditionally render
  either the keepalived manifests (default) or the kube-vip + frr-k8s
  manifests, based on whether BGP VIP management is enabled. When BGP VIP
  management is active, MCO will skip rendering keepalived manifests and
  instead render the kube-vip and frr-k8s static pod manifests along with the
  bootstrap `frr.conf`. This is a required change in the MCO codebase.

  The static pod has two variants. The **bootstrap variant is FRR-only**:
  the runtimecfg config-render init container, a file-copy init container
  for the FRR `daemons`/`vtysh.conf` files, and the FRR daemon container.
  The frr-k8s controller, reloader and status containers are omitted during
  bootstrap -- there is no Node object, no frr-k8s CRDs and no metrics
  certificates at that point, and the CRD configuration path is unused. The
  **day-2 variant runs the full pod** (controller, FRR, reloader,
  frr-status) on control plane nodes only; the controller and frr-status
  containers mount the node kubeconfig, and frr-status is passed the mirror
  pod name (`--pod-name=frr-k8s-<nodeName>`). The metrics exporter is
  omitted from the static pod for now (static pods have no certificate
  provisioning path); metrics delivery will be revisited for the Tech
  Preview observability criteria. Peer data reaches MCO from the
  installer-generated `bgp-vip-config` ConfigMap: the bootstrap render
  reads the ConfigMap manifest from the installer asset directory, and
  day-2 the MCO operator syncs its `config.json` payload (validated and
  compacted) into a feature-gated internal API field,
  `ControllerConfigSpec.BGPVIPPeersJSON`, from which the template
  controller writes the node peer file. A missing or empty ConfigMap on a
  cluster with BGP VIP management active degrades the operator rather than
  blanking the peer file on nodes.

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

  When per-host BGP peers are configured (via `hosts[].bgpPeers` in
  `install-config.yaml`), the bootstrap FRR configuration is not a single
  static file delivered identically to all nodes. Instead, the installer
  generates a Go template (`frr.conf.tmpl`) and a per-node peer mapping
  file (`frr-peers.json`) keyed by hostname. An FRR config renderer
  sidecar in the frr-k8s static pod renders the node-specific `frr.conf`
  at startup by discovering the local node's hostname and primary IP,
  resolving the correct peer list from the mapping, and writing the
  rendered config to a shared volume. This follows the same two-phase
  rendering architecture used by the `keepalived-monitor` sidecar in
  `baremetal-runtimecfg`. See the "Runtime FRR Configuration Rendering"
  section in Implementation Details for the full mechanism.

- **Transition to CRD-based configuration**: The frr-k8s configuration
  lifecycle has three distinct phases, each with a clear owner:

  1. **Bootstrap (owner: installer + MCO):** The installer generates the
     initial `frr.conf` and the frr-k8s static pod manifest. MCO renders
     these into MachineConfig resources and places them on nodes. During this
     phase, frr-k8s runs entirely from the static `frr.conf` on disk. No
     API server or CRDs are involved.

  2. **Handover (owner: CNO):** Once the API server is available and frr-k8s
     CRDs are registered, CNO performs the transition:
     - CNO creates a `FRRConfiguration` CR (named with a `bgp-vip-` prefix
       and labeled with `app.kubernetes.io/managed-by: cluster-network-operator`)
       that carries the BGP **sessions** from the bootstrap configuration:
       neighbors, passwords, BFD profiles.
     - VIP **advertisement is deliberately not expressed through the CRD
       surface**. `FRRConfiguration` `prefixes`/`toAdvertise` render as
       unconditional `network` statements, which would advertise the VIPs
       regardless of kube-vip's health gate in routing table 198
       (implementation experience: this steered ECMP traffic to nodes with
       failed backends). Instead, the CR's `spec.raw.rawConfig` reproduces
       the bootstrap advertisement semantics: `ip import-table 198` (zebra
       only tracks non-main kernel tables when instructed), per-address-
       family `redistribute table-direct 198` filtered through the VIP
       route-maps and prefix-lists, and high-sequence permit entries
       appended to frr-k8s's generated per-neighbor `<peer-address>-out`
       route-maps. The latter is needed because frr-k8s renders deny-any
       outbound prefix-lists when `toAdvertise` is absent; a prefix-list
       deny is a route-map no-match, so evaluation falls through to the
       appended permits -- egress opens exactly for the VIP prefixes and
       everything else remains implicitly denied. An upstream frr-k8s
       feature for advertising redistributed routes would remove this raw
       route-map coupling.
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
       (Static-config removal is future work; in the validated
       implementation the static file and the CRD-based configuration
       coexist safely -- the FRR daemon loads the static file at startup and
       the controller-rendered configuration takes over on its first
       reconcile.)
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
  those same nodes. When BGP VIP management is active, CNO renders the
  frr-k8s DaemonSet with required node-affinity that excludes control plane
  nodes by role (`node-role.kubernetes.io/master` `DoesNotExist`): masters
  run the static pod, workers run the DaemonSet. Implementation experience
  ruled out label-based approaches (a node label set by the static pod):
  NodeRestriction denies node-credentialed writes of such labels, and
  DaemonSet scheduling races the labeling on fresh nodes.
  The single frr-k8s instance is shared by all consumers (VIP advertisement,
  MetalLB, OVN-Kubernetes route advertisements) via additive
  `FRRConfiguration` CRs.

#### 2. kube-vip -- Routing Table Mode Deployment

kube-vip is deployed as **two separate static pods** on each control plane
node, one for the API VIP and one for the Ingress VIP. Each instance manages
a single VIP address, since kube-vip's `address` environment variable accepts
only one IP address -- there is no upstream support for multiple control plane
VIPs in a single kube-vip instance. As with frr-k8s, the installer generates
the initial manifests for bootstrap, and MCO owns the manifests post-bootstrap
via MachineConfig resources for control plane nodes.

**`kube-vip-api.yaml` -- API VIP (deployed from bootstrap):**

- Configured with `cp_enable=true`, `address=<api-vip>`,
  `vip_leaderelection=true`, `vip_routingtable=true`,
  `k8s_config_file=/etc/kubernetes/kubeconfig` and
  `kubernetes_addr=https://localhost:6443`. The last two direct kube-vip's
  Kubernetes client at the node kubeconfig and the *local* API server:
  Lease traffic must not depend on the very VIP kube-vip manages, or
  leader-election renewal deadlocks when the VIP moves (for example at
  bootstrap teardown).
- Runs a continuous backend health check loop: it periodically probes the
  local kube-apiserver (via the Kubernetes API discovery endpoint on
  `localhost:6443`) and maintains the API VIP `/32` route in Linux routing
  table 198 via netlink only while the backend is healthy. If the local
  kube-apiserver becomes unreachable, the route is removed until it
  recovers. Every node with a healthy local API server advertises the VIP
  (health-gated ECMP); the leader-election Lease does not gate the route
  reconciliation loop.
- `vip_cleanroutingtable: "true"` is enabled to clean stale routes at
  startup.
- This manifest is generated by the installer and placed in
  `/etc/kubernetes/manifests/` on the bootstrap node and each control plane
  node from the earliest bootstrap phase.

**`kube-vip-ingress.yaml` -- Ingress VIP (deployed post-bootstrap):**

- Configured identically to the API VIP instance but with
  `address=<ingress-vip>` and a different Lease name.
- Uses kube-vip's **configurable HTTP health check** (upstream since
  kube-vip/kube-vip#1604, see "Downstream kube-vip Changes" section below)
  instead of the default Kubernetes API check. Configured with
  `control_plane_health_check_address=http://localhost:1936/healthz` to
  probe the local OpenShift router's health endpoint -- the same check
  keepalived's `chk_ingress` script uses today. (An earlier revision of
  this document pointed at port 29445; that is baremetal-runtimecfg's
  API-haproxy monitor and is the wrong signal.) The route in table 198
  exists only while the local ingress controller (router) is healthy. If
  the router check fails, kube-vip removes the ingress VIP route, causing
  frr-k8s to withdraw the BGP advertisement for this node; nodes with a
  healthy router keep advertising. This is functionally equivalent to
  keepalived's `vrrp_script` mechanism.
- This manifest is **not** present during bootstrap. The ingress VIP is not
  needed until the ingress controller is operational (a day-2 concern). CNO
  deploys the `kube-vip-ingress.yaml` static pod manifest via MCO
  MachineConfig update once the cluster is fully operational and the
  ingress controller is running.

Each instance has its own independent Kubernetes Lease for leader election,
so the API VIP and Ingress VIP can reside on different nodes simultaneously.

Both kube-vip instances require:
- `hostNetwork: true` for netlink access
- `NET_ADMIN` and `NET_RAW` capabilities
- Access to the kubeconfig file (`/etc/kubernetes/admin.conf` or
  `/etc/kubernetes/super-admin.conf` on Kubernetes >= 1.29)

#### 3. OVN-Kubernetes (openshift/ovn-kubernetes) -- Static Pod Compatibility

OVN-Kubernetes must be aware that frr-k8s may be deployed as a static pod
rather than a DaemonSet managed by CNO. The key changes:

- **Static pod / DaemonSet separation**: OVN-Kubernetes's route
  advertisement features consume the frr-k8s DaemonSet that CNO deploys.
  Under BGP VIP management the DaemonSet carries node-affinity excluding
  control plane nodes by role (see the frr-k8s section above), so on
  masters the static pod is the single frr-k8s instance serving all
  consumers, and on workers it is the DaemonSet.

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

The types above (`BGPPeerConfig`, `BGPVIPConfig`) are defined as reusable Go
types in a shared package (e.g., `installer/pkg/types/onprem/` or a common
types package), not inside the baremetal-specific `platform.go`. Each on-prem
platform struct that supports BGP VIP management references these shared types
by embedding or aliasing them. For the first iteration, only the baremetal
`Platform` struct in `installer/pkg/types/baremetal/platform.go` exposes
`bgpVIPConfig`. When future platforms (vSphere, OpenStack, Nutanix) gain BGP
VIP support, they add their own `bgpVIPConfig` field referencing the same
shared types -- following the established precedent where `apiVIPs`,
`ingressVIPs`, and `PlatformLoadBalancerType` are structurally identical
across platforms but defined independently in each platform's types.

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

**Cross-platform considerations:** The `hosts[].bgpPeers` override is
inherently baremetal-specific because only the baremetal platform has a
`hosts[]` array in `install-config.yaml`. Other on-prem platforms (vSphere,
OpenStack, Nutanix) do not have a per-host concept at install time -- vSphere
uses `failureDomains[]`, OpenStack uses subnet references, and
`platform: none` has no per-host configuration mechanism at all.

There is no elegant way to express per-node BGP peer overrides in a
platform-independent manner at install time. The OpenShift `install-config.yaml`
API uses discriminated unions where each platform is an independent Go struct,
and there is no shared "on-prem platform" base type or mixin. This is the same
pattern followed by `apiVIPs`, `ingressVIPs`, and `loadBalancer.type`, which
are structurally identical across platforms but defined independently in each
platform's types.

For platforms without `hosts[]`, per-node BGP peer overrides are handled
**post-bootstrap via `FRRConfiguration` CRs with `nodeSelector`**, which is
the day-2 steady-state mechanism for all platforms regardless. The
`FRRConfiguration` CRD natively supports `nodeSelector`, so no new API is
required. This means non-baremetal platforms will use the global
`bgpVIPConfig.peers` for all nodes during bootstrap, with per-node
differentiation available only after the API server is operational and
`FRRConfiguration` CRs can be created. For most non-baremetal deployments
this is acceptable because per-node peering differences are less common when
nodes are not physically distributed across distinct ToR switches.

The per-host peer override works as follows:

- **Bootstrap phase:** MCO delivers a single, identical MachineConfig to all
  master nodes containing a Go template (`frr.conf.tmpl`) and a JSON peer
  mapping file (`frr-peers.json`) keyed by hostname. The installer generates
  the peer mapping from `install-config.yaml`: for each host with
  `host.bgpPeers` set, the mapping entry uses the host-specific peers;
  hosts without a per-host override fall back to the global
  `bgpVIPConfig.peers`. The frr-k8s static pod includes an FRR config
  renderer sidecar (analogous to the `keepalived-monitor` sidecar in
  `baremetal-runtimecfg`) that discovers the local node's hostname and
  primary IP at startup, resolves the correct peer list from the mapping,
  renders the final `frr.conf`, and writes it to a shared `emptyDir` volume
  for the FRR daemon. This follows the same two-phase rendering architecture
  (MCO delivers template, sidecar renders per-node config at runtime) used
  by keepalived today. On baremetal IPI, hostname is deterministic: the
  `hosts[].name` field becomes the RHCOS hostname through the BareMetalHost
  provisioning flow. See the "Runtime FRR Configuration Rendering" section
  in Implementation Details for the full mechanism.

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
    // Immutable once set.
    // +openshift:enable:FeatureGate=BGPBasedVIPManagement
    // +optional
    VIPManagement string `json:"vipManagement,omitempty"`
}
```

`vipManagement` is the single API-level signal for whether the cluster uses
BGP or keepalived for VIP management (implemented in openshift/api#2923,
installer-set, immutable in the first iteration). An earlier draft also
carried a `BGPVIPStatus` struct here echoing `localASN` and the peer
addresses; it was dropped: status must not restate configuration, the
useful observation - per-node, per-peer session health - lives in frr-k8s's
`BGPSessionState`/`FRRNodeState` resources (plus the metrics and alerts
required for Tech Preview), and populating it would have made CNO a second
writer to Infrastructure status.

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
   - A `kube-vip-api.yaml` static pod manifest configured for Routing Table
     Mode with the API VIP address. No ingress VIP manifest is generated at
     this stage -- the ingress VIP is a post-bootstrap concern.
   - A frr-k8s static pod manifest with a bootstrap `frr.conf` that
     configures BGP peering using the parameters from `install-config.yaml` and
     imports routes from kernel table 198.
   - Both manifests are placed in `/etc/kubernetes/manifests/` on the bootstrap
     node and subsequently on each control plane node.

5. The bootstrap node starts. The kubelet launches the kube-vip-api and
   frr-k8s static pods. kube-vip connects to the local bootstrap
   kube-apiserver, its backend health check passes, and it writes the API
   VIP route to table 198 via netlink. frr-k8s establishes BGP sessions
   with the configured peers using the static `frr.conf` and begins
   advertising the API VIP route.

6. The API VIP becomes reachable via BGP before the API server starts. The
   kube-apiserver starts and becomes accessible at the API VIP.

7. The remaining control plane nodes are provisioned. Each receives the same
   static pod manifests. As each node's local kube-apiserver becomes
   healthy, its kube-vip adds the API VIP route to table 198 and its
   frr-k8s advertises it -- the external peers see equal-cost paths to
   every healthy control plane node.

8. Once the cluster is fully operational, CNO detects the BGP VIP configuration
   and creates the corresponding `FRRConfiguration` CRs to formalize the
   bootstrap configuration. CNO also deploys the `kube-vip-ingress.yaml`
   static pod manifest via MCO MachineConfig update. This manifest is
   configured with
   `control_plane_health_check_address=http://localhost:1936/healthz`
   so that the ingress VIP route is only written to table 198 on nodes where
   the ingress controller (router) is healthy.

#### Failover Workflow

1. A control plane node advertising the API VIP fails.
2. If BFD is enabled, the external peer detects the failure within
   milliseconds and withdraws that node's path; otherwise the BGP hold
   timer expires. Independently, if only the node's backend (rather than
   the node itself) fails, kube-vip's health check removes the route from
   table 198 within one check interval and frr-k8s withdraws the
   advertisement.
3. The remaining healthy nodes were already advertising the VIP; external
   peers simply drop the failed path from the ECMP set. No leadership
   transfer or re-election is on the failover path.
4. When the node (or its backend) recovers, its advertisement returns and
   the peers re-add the path.

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

- **ControllerConfigSpec.BGPVIPPeersJSON (openshift/api,
  machineconfiguration/v1)**: a feature-gated, optional string field on
  MCO's internal ControllerConfig API carrying the `bgp-vip-config`
  ConfigMap's `config.json` payload (validated and compacted by the MCO
  operator) so the template controller can render the node peer file. Not
  a user-facing API; only populated when BGP VIP management is active.
  **Dev Preview only**: the serialized-JSON form exists to let the payload
  schema bake without API commitments. It is replaced for Tech Preview by
  the structured `BGPVIPConfig` CRD described below.

- **Structured BGP configuration API (Tech Preview)**: the Dev Preview
  ConfigMap and serialized-JSON ControllerConfig field are replaced by a
  single admission-validated API. Two candidate placements are under
  consideration; the decision is made with the API reviewers before Tech
  Preview implementation starts.

  **Option A - `Infrastructure.spec.platformSpec.baremetal.bgp`**: a typed
  struct on the existing Infrastructure CR spec.

  - Precedent: `BareMetalPlatformSpec` already serves day-2 editable
    on-prem networking (`apiServerInternalIPs`/`ingressIPs` for VIP
    changes), with the established spec-to-status propagation flow.
    Secret references from config-group objects are also established
    (APIServer `servingCerts`, Proxy `trustedCA`).
  - Pros: no new CRD lifecycle; co-located with `vipManagement` and the
    VIP fields; discoverable where operators already look for on-prem
    networking; bootstrap consumes the Infrastructure manifest unchanged.
  - Cons: Infrastructure is watched by nearly every operator and node
    agent, so every peer edit fans a full-object update to the fleet -
    `hostOverrides` scales with host count (multi-rack: per-host peer
    lists), making the hottest object in the cluster hotter; there is no
    feature-scoped status/conditions surface (only value mirroring into
    `platformStatus`), so the day-2 feedback loop is limited; fields in
    `config.openshift.io/v1` are permanent on arrival, while the peer
    schema is still young (it changed twice during Dev Preview).
  - The cons are mitigated by keeping the schema lean where possible and
    accepting value mirroring into `platformStatus` as the feedback
    mechanism; `hostOverrides` size at multi-rack scale is the one factor
    that could tip the decision to Option B.

  **Option B - dedicated `BGPVIPConfig` CRD
  (machineconfiguration.openshift.io, cluster-scoped, feature-gated)**,
  replacing both the `bgp-vip-config` ConfigMap and the serialized-JSON
  ControllerConfig field as the single source of BGP peer configuration:

  - The installer generates the `BGPVIPConfig` manifest from
    `install-config.yaml` (the bootstrap MCO render consumes the manifest
    file exactly as it consumes the ConfigMap manifest today, so the
    bootstrap flow is unchanged).
  - CNO and the MCO operator both watch the CR: CNO renders the
    `FRRConfiguration` from it, MCO populates an operator-internal typed
    copy on `ControllerConfigSpec` (replacing `BGPVIPPeersJSON`) for the
    template render. One schema, admission-validated, no hand-mirrored
    JSON contracts.
  - Typed shape fixes the Dev Preview payload warts: `metav1.Duration`
    for hold/keepalive times, booleans instead of `"true"` strings,
    list-map `hostOverrides`, CEL validation for ASNs and peer addresses,
    and `passwordSecretRef` (a `kubernetes.io/basic-auth` Secret
    reference) instead of an inline plaintext password. The
    `apiVIPs`/`ingressVIPs` duplication is dropped: templates read VIPs
    from the Infrastructure CR as they do for keepalived.
  - Day-2 reconfiguration becomes a first-class flow: `oc edit
    bgpvipconfig cluster` is validated at admission, both consumers react
    via watches, and the CR carries status conditions
    (`observedGeneration`, rendered/applied) so changes have a feedback
    loop. A NodeDisruptionPolicy ships alongside so peer-file updates do
    not reboot nodes (after the bootstrap-to-CRD handover the on-disk
    config only matters at early boot).
  - GA hardens the same CRD (no shape change expected): status conditions
    complete, day-2 flows covered by e2e, and the Dev Preview ConfigMap
    path removed.
  - Pros: watched by exactly two consumers (CNO, MCO operator); native
    status conditions for the day-2 feedback loop; the feature-gated CRD
    can iterate while the schema matures; room for the full multi-rack
    `hostOverrides` shape, which the GA test criteria require.
  - Cons: a new CRD lifecycle to own (shipped and reconciled via MCO's
    payload manifests); one more object to discover, mitigated by
    cross-references from the `vipManagement` field documentation.

  The current preference is Option A: it keeps the BGP configuration on
  the API object that already owns the on-prem VIP surface, reuses the
  established day-2 editing and spec-to-status flow, and adds no new CRD
  lifecycle. Option B is the fallback if API review concludes the
  Infrastructure CR should not grow this schema (the watch fan-out of
  `hostOverrides` at multi-rack scale being the main reason to make that
  call).

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

#### Implementation Experience

The design in this document has been implemented across all affected
repositories and validated end to end on a dev-scripts bare metal cluster:
installation completes with the API VIP advertised via BGP from the
bootstrap phase, both VIPs are advertised with health-gated ECMP, the
CRD handover works, and the console is reachable over the BGP-routed
ingress path (35 of 36 cluster operators available; the exception was a
platform bug unrelated to this feature). The reference implementation --
a per-repository patch series, the full run ledger of the 14 validation
installs, an operational runbook and the isolated FRR reproduction -- is
available at <https://github.com/mkowalski/bgp-vip-demo>. Supporting
changes are in flight upstream: dev-scripts ToR BGP speaker support
(openshift-metal3/dev-scripts#1929) and the kube-vip kubeconfig fixes
(kube-vip/kube-vip#1627). Statements in this document marked "validated"
refer to that work.

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
VIP must be reachable before the API server starts, but kube-vip talks to a
Kubernetes API server for its health checks and Lease. This is resolved by
keeping every Kubernetes interaction strictly node-local:

1. **No circular dependency exists.** Writing a route to kernel table 198 via
   netlink and advertising it via BGP are pure network-level operations.
   kube-vip's only API dependency is on the *local* API server
   (`kubernetes_addr=https://localhost:6443` with the node kubeconfig),
   never on the VIP.

2. **Bootstrap node:** The bootstrap kube-apiserver comes up on
   `localhost:6443` without any VIP involvement. As soon as kube-vip's
   health check against it passes, kube-vip writes the VIP route to table
   198 and frr-k8s advertises it via BGP using the static `frr.conf`
   configuration. The API VIP becomes reachable on the network as soon as
   the bootstrap control plane answers -- validated during implementation.

3. **Control plane nodes:** Each master's kubelet, ignition and cluster
   join traffic reaches the API via the VIP that the bootstrap node
   advertises. Each master's kube-vip in turn health-checks its own local
   kube-apiserver and starts advertising once it is healthy, growing the
   ECMP set.

4. **Bootstrap teardown:** When the bootstrap node is destroyed, its BGP
   sessions drop and its path is withdrawn (BFD or hold timer bounds the
   detection). The masters were already advertising; no takeover handshake
   is involved -- validated during implementation: the next-hop set simply
   pivoted from the bootstrap node to the masters.

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
2. `kube-vip-api.yaml` -- starts second, after frr-k8s
3. `kube-vip-ingress.yaml` -- starts third (only present post-bootstrap,
   deployed by CNO via MCO MachineConfig update)

This ordering ensures that by the time kube-vip-api writes the API VIP
route to table 198, frr-k8s (with zebra importing table 198) is already
running and will pick up the route for BGP advertisement. The
`kube-vip-ingress.yaml` manifest is not present during bootstrap; it is
added post-bootstrap by CNO once the ingress controller is operational.

#### kube-vip Routing Table Mode Details

kube-vip in Routing Table Mode operates as follows:

- All other VIP modes (ARP, BGP native) are disabled;
  only `vip_routingtable: "true"` is set.
- Routes are managed via netlink in routing table ID `198` (configurable via
  `vip_routingtableid`) with routing protocol ID `248` (configurable via
  `vip_routingtableprotocol`).
- The backend health check loop runs on every instance and is the sole gate
  for the route: a node holds the VIP route in table 198 exactly while its
  local backend is healthy. The leader-election Lease
  (`vip_leaderelection: "true"`) is configured but does not gate the route
  reconciliation loop in Routing Table Mode -- all healthy nodes advertise
  and external peers ECMP across them (validated behavior; see the
  multi-advertiser section under Risks and Mitigations).

Note: restricting advertisement to a single elected node (active/passive)
would require leadership-gating kube-vip's route reconciliation loop. This
is **not part of the first iteration**; health-gated ECMP is the validated
model.

#### Downstream kube-vip Changes

Two areas of kube-vip needed attention; both are resolved or in flight
upstream.

**HTTP health check for the ingress VIP.** Upstream kube-vip provides a
configurable HTTP health check (`control_plane_health_check_address`,
`control_plane_health_check_timeout_seconds`), and its wiring into the
Routing Table Mode reconciliation loop merged upstream as
[kube-vip/kube-vip#1604](https://github.com/kube-vip/kube-vip/pull/1604):
on each interval the loop probes the configured URL and adds or removes the
VIP route in table 198 accordingly, falling back to the default Kubernetes
API discovery check when the address is unset. The ingress instance sets
`control_plane_health_check_address=http://localhost:1936/healthz` (the
local router health endpoint). No downstream change is required for this.

**Kubeconfig handling for static pods.** Implementation experience exposed
a real gap: kube-vip ignored the explicitly configured kubeconfig path
(`--k8sConfigPath` / `k8s_config_file`) in **both** the manager
initialization and the backend health checks, probing only hardcoded
locations (`/etc/kubernetes/admin.conf`, `$HOME/.kube/config`, in-cluster
config). OpenShift static pods have none of those -- the node kubeconfig
lives at `/etc/kubernetes/kubeconfig` -- so kube-vip could neither start
its manager nor pass a single health check. Upstream PR
[kube-vip/kube-vip#1627](https://github.com/kube-vip/kube-vip/pull/1627)
makes an explicitly configured kubeconfig take precedence in both paths
(no behavior change when the option is unset); the downstream fork carries
these commits until the PR merges. The manifests additionally set
`kubernetes_addr=https://localhost:6443` so that Lease and health traffic
targets the local API server rather than the VIP kube-vip itself manages.

The two kube-vip instances use this as follows:

| Instance | `address` | Health check | Route lifecycle |
|----------|-----------|-------------|-----------------|
| `kube-vip-api` | `<api-vip>` | Default (Kubernetes API discovery on `localhost:6443`) | Route in table 198 exists only when local kube-apiserver is healthy |
| `kube-vip-ingress` | `<ingress-vip>` | HTTP (`control_plane_health_check_address=http://localhost:1936/healthz`) | Route in table 198 exists only when the local OpenShift router is healthy |

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

#### Runtime FRR Configuration Rendering

MachineConfig is a pool-level primitive: all nodes in a MachineConfigPool
(e.g., `master`) receive the same rendered MachineConfig. There is no
per-node MachineConfig mechanism in MCO. When per-host BGP peers are
configured via `hosts[].bgpPeers`, each node needs a different `frr.conf`
with its specific peer list and router-id. This is solved using the same
two-phase rendering architecture that keepalived uses with
`baremetal-runtimecfg` today.

**Phase 1 -- MCO delivers identical files to all nodes:**

MCO renders a single MachineConfig for all master nodes containing:

- A Go template file (`/etc/frr/frr.conf.tmpl`) with per-node variables
  for the router-id and BGP neighbor definitions (e.g.,
  `{{ .RouterID }}`, `{{ range .Peers }}`).
- A JSON peer mapping file (`/etc/frr/frr-peers.json`) keyed by hostname,
  generated by the installer from `install-config.yaml`. Each entry maps
  a hostname to its BGP peer list. Hosts without a per-host override in
  `hosts[].bgpPeers` are omitted from the mapping and fall back to the
  global `bgpVIPConfig.peers` at render time.

Both files are placed on disk via MachineConfig and mounted into the
frr-k8s static pod via `hostPath` volumes.

Example `frr-peers.json` for a multi-rack deployment (the schema matches
baremetal-runtimecfg's `FRRPeerMapping` type verbatim; the installer's
`bgp-vip-config` ConfigMap `config.json` uses the same schema -- plus
`apiVIPs`/`ingressVIPs` keys that runtimecfg ignores -- and is copied
unchanged to the node peer file, so one schema is used end to end):

```json
{
  "localASN": 64512,
  "defaultPeers": [
    {"peerAddress": "192.168.1.1", "peerASN": 64512, "bfdEnabled": "false"}
  ],
  "hostOverrides": {
    "master-0": [
      {"peerAddress": "192.168.1.1", "peerASN": 64512, "bfdEnabled": "true"}
    ],
    "master-1": [
      {"peerAddress": "192.168.2.1", "peerASN": 64512, "bfdEnabled": "true"}
    ],
    "master-2": [
      {"peerAddress": "192.168.3.1", "peerASN": 64512, "bfdEnabled": "true"}
    ]
  }
}
```

**Phase 2 -- Sidecar renders per-node config at runtime:**

The frr-k8s static pod includes an FRR config renderer init container
(a one-shot run of `baremetal-runtimecfg`, analogous in role to the
`keepalived-monitor` sidecar in the keepalived static pod). At startup it:

1. Reads the Go template and peer mapping file from the `hostPath` volumes.
2. Discovers the local node's hostname via `os.Hostname()`.
3. Discovers the local node's primary IP via
   `/run/nodeip-configuration/primary-ip` (the same mechanism used by
   `baremetal-runtimecfg` for `.NonVirtualIP`).
4. Looks up the hostname in the `hostOverrides` map. If a per-host entry
   exists, uses that peer list; otherwise, falls back to `defaultPeers`.
5. Renders the Go template with the resolved peer list and router-id
   (primary IP).
6. Writes the final `frr.conf` to a shared `emptyDir` volume that the FRR
   daemon container reads at startup.

When all nodes share the same peers (no `hosts[].bgpPeers` overrides), the
sidecar still runs but the rendering is uniform -- only the router-id
(derived from each node's primary IP) differs between nodes.

**Node identity key:**

The sidecar uses the OS hostname as the lookup key into the peer mapping.
On baremetal IPI, hostname is deterministic: the `hosts[].name` field in
`install-config.yaml` becomes the BareMetalHost CR name, which becomes the
RHCOS hostname through the provisioning flow. This is the same identity
mechanism used by the `configure-ovs` per-node NMState configuration
dispatch. Future platform support (vSphere, OpenStack, `platform: none`)
may require a more robust identity resolution strategy (e.g., IP-based
matching or Kubernetes Node object lookup), since hostname assignment on
those platforms is not always controlled by the installer.

**Parallel with keepalived:**

The existing keepalived static pod uses the same architecture:

- MCO delivers an identical keepalived config template and pod manifest to
  all master nodes.
- The `keepalived-monitor` sidecar (from `baremetal-runtimecfg`) runs on
  each node, discovers the local node's IP (`.NonVirtualIP`), hostname
  (`.ShortHostname`), and peer IPs (queried from the local
  kube-apiserver), and renders the final `keepalived.conf` at runtime.
- The sidecar continuously monitors for changes (new nodes joining,
  interface changes) and re-renders the config as needed.

The FRR config renderer follows this pattern as a one-shot init
container. During bootstrap
(before the API server is available), it operates purely from local state
(hostname, primary IP, files on disk). Post-bootstrap, it could optionally
be extended to watch for configuration changes, though this is not required
for the first iteration since the CRD-based handover (described above)
takes over configuration management once the API server is available.

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

#### FRR zebra import-table fix required

FRR versions before 10.7 carry a zebra bug that breaks this design's day-2
phase: `zebra_add_import_table_entry` clears the `ZEBRA_FLAG_SELECTED` flag
on the *source* route while importing it, so any route that already exists
in the kube-vip routing table at the moment the `ip import-table` /
`redistribute table-direct` configuration is (re)applied is never
redistributed -- and a configuration reload actively de-selects previously
selected routes. Bootstrap is unaffected (kube-vip writes the route after
FRR starts, and live netlink events process correctly), but the CRD
handover re-applies the configuration while the VIP route already exists,
silently stopping advertisement. Fixed upstream by FRRouting/frr commit
`b2c17ad52` ("zebra: Do not clear selected flag on route about to be
imported", first released in FRR 10.7). OpenShift currently ships FRR
10.4.x in the frr-k8s image, so the fix must be backported to the shipped
FRR (RPM or image) as a prerequisite for this enhancement. An isolated
container reproduction and the backport are part of the reference
implementation.

#### Static pod API access (RBAC)

The day-2 static pod's controller and status containers authenticate with
the node kubeconfig (`/etc/kubernetes/kubeconfig`). Its identity is the
ServiceAccount `openshift-machine-config-operator/node-bootstrapper`
(verified on a live cluster) -- notably *not* a `system:nodes` group
identity, so the Node authorizer does not apply and plain RBAC governs
access to the frr-k8s CRs. CNO ships a ClusterRole and binding for that
subject: read (`get`/`list`/`watch`) on `frrconfigurations` and
`frrk8sconfigurations`, write on `frrnodestates` and `bgpsessionstates`
(including status), node reads, plus namespace-scoped `secrets` (BGP
session passwords) and `pods` reads in `openshift-frr-k8s`. Known
limitation to resolve before GA: the credential is shared by all nodes, so
any node can write any node's state CRs; per-node scoping requires
admission-level enforcement (for example a CEL ValidatingAdmissionPolicy),
which RBAC alone cannot express.

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
| kube-vip | (new, downstream fork) | Networking | OpenShift-specific build, inclusion in release payload, Routing Table Mode validation; carries the kubeconfig handling fixes until kube-vip/kube-vip#1627 merges |
| baremetal-runtimecfg | `openshift/baremetal-runtimecfg` | On-prem Networking | FRR config template rendering support (config renderer sidecar), per-node peer mapping resolution by hostname |

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| BGP misconfiguration during bootstrap prevents cluster installation | The installer will validate BGP parameters. Clear error messages will guide the user. A fallback to keepalived can be documented. |
| kube-vip or frr-k8s static pod crashes during bootstrap | Both components will be configured with restart policies. The kubelet will automatically restart failed static pods. |
| Conflict between static pod frr-k8s and DaemonSet frr-k8s | Under BGP VIP management, CNO renders the DaemonSet with node-affinity excluding control plane nodes by role; masters run the static pod, workers the DaemonSet. |
| BGP session security (unauthenticated sessions) | MD5 password authentication is supported and should be recommended in documentation. |
| Route leaking -- kube-vip advertising unintended routes | The FRR configuration uses strict route-maps and prefix-lists to only advertise VIP `/32` routes. |
| Upstream kube-vip project stability | OpenShift will vendor a specific, tested version of kube-vip. The Routing Table Mode is the simplest mode with minimal moving parts. |
| FRR-K8s API gaps for bootstrap scenario | Bootstrap uses static `frr.conf` to avoid dependency on the API server. CRD-based management takes over post-bootstrap. |
| FRR silently not redistributing pre-existing table routes (FRR < 10.7) | Requires the zebra fix FRRouting/frr `b2c17ad52` backported to the shipped FRR (see "FRR zebra import-table fix required"). |
| Split-brain: network partition causes two nodes to believe they hold the VIP | See detailed analysis below. |

#### Multi-Advertiser Operation and Network Partitions

In normal operation every node whose local backend is healthy advertises the
VIP: external BGP peers receive the same `/32` from multiple next-hops and
distribute traffic across them with ECMP. This is the steady-state model of
this enhancement, and the same analysis covers network partitions (where a
subset of nodes keeps advertising a VIP independently).

**API VIP traffic:** External routers ECMP across the advertising nodes.
Since the API server runs on all control plane nodes and each node's
advertisement is gated on its *local* API server health, every path in the
ECMP set terminates at a working kube-apiserver. A node whose API server
degrades withdraws its own path within one health-check interval --
validated live: during control plane rollouts the ECMP set shrank and grew
per node, with no client-visible outage.

**Ingress VIP traffic:** The ingress controller (router) may not run on
every node. The `kube-vip-ingress` health check (configured with
`control_plane_health_check_address=http://localhost:1936/healthz`)
guarantees only nodes with a healthy local router advertise the ingress
VIP -- validated live: on a compact cluster with routers on two of three
control plane nodes, exactly those two advertised. Traffic therefore only
reaches nodes with a working router, during partitions included.

**Partition healing:** No convergence protocol is needed beyond BGP itself.
When a partition heals, peers simply merge the advertisements back into one
ECMP set; when a node dies, BFD (milliseconds) or the BGP hold timer bounds
the withdrawal of its path.

**Comparison with keepalived:** The existing keepalived/VRRP model has a
genuine split-brain failure mode during partitions -- multiple nodes may
claim the VIP via gratuitous ARP, and L2 convergence is undefined. The BGP
model replaces that with well-defined router behavior (ECMP across
advertised paths), each path individually health-gated.

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
  election, health-gated route management, VIP assignment) that would need to
  be reimplemented. Note: the upstream Routing Table Mode health check is
  hardcoded to Kubernetes API discovery; a downstream enhancement adds HTTP
  endpoint probing for the ingress VIP use case (see "Downstream kube-vip
  Enhancement" section).
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

1. ~~When should ECMP support be introduced?~~ *Resolved by implementation
   experience:* health-gated ECMP is the native behavior of kube-vip's
   Routing Table Mode and is the model of the first iteration (see
   "Multi-Advertiser Operation"). The remaining open question is the
   inverse: is a single-advertiser (active/passive) option ever needed,
   and if so, kube-vip's route reconciliation loop must learn to be
   leadership-gated.

2. When BGP VIP management is extended to non-baremetal on-prem platforms
   (vSphere, OpenStack, Nutanix), should those platforms introduce a
   lightweight per-node peer override mechanism at install time (e.g., a
   platform-independent `bgpNodeOverrides[]` array keyed by expected
   hostname), or is the post-bootstrap `FRRConfiguration` CR approach with
   `nodeSelector` sufficient for per-node peer differentiation on all
   non-baremetal platforms?

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
- The structured BGP configuration API (preferred:
  `Infrastructure.spec.platformSpec.baremetal.bgp`; fallback: dedicated
  `BGPVIPConfig` CRD - see API Extensions) replaces the `bgp-vip-config`
  ConfigMap and the serialized-JSON `ControllerConfigSpec.BGPVIPPeersJSON`
  field, including `passwordSecretRef` for peer passwords.
- NodeDisruptionPolicy for the rendered peer file so day-2 BGP
  reconfiguration does not reboot nodes.

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
- Day-2 BGP reconfiguration flows covered by e2e; the Dev Preview
  ConfigMap ingestion path removed.

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
