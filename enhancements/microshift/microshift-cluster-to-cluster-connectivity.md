---
title: microshift-cluster-to-cluster-connectivity
authors:
  - '@pmtk'
reviewers:
  - '@pacevedom'
  - "@eslutsky"
  - "@copejon"
  - '@agullon'
  - 'TBD, Networking Expert'
approvers:
  - "@jerpeter1"
api-approvers:
  - None
creation-date: 2026-03-30
last-updated: 2026-04-14
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-2898
see-also: []
replaces: []
superseded-by: []
---

# MicroShift Cluster-to-Cluster Connectivity (C2CC)

## Summary

This enhancement enables cross-cluster Pod-to-Pod
and Pod-to-Service (ClusterIP and headless, via DNS)
communication between independent MicroShift instances.
C2CC uses OVN static routes for overlay-to-underlay
routing, Linux kernel policy routing for host-level
forwarding, SNAT bypass for source pod IP preservation,
and CoreDNS forwarding for cross-cluster service
discovery. It targets edge deployments where multiple
MicroShift clusters on the same network segment (or
reachable via routable next-hops) need direct workload
communication.

## Motivation

MicroShift is deployed on edge devices — for example, in
factory floors, retail locations, or vehicles — where each
device runs its own independent single-node MicroShift
cluster and workloads on one device need to consume
services running on a neighboring device. Today, this
requires relying on external solutions that add
operational complexity, complicate upgrades, and
introduce fragile dependencies.

C2CC replaces these with a built-in, declarative
mechanism to configure cross-cluster networking
directly through the MicroShift config file.

### User Stories

* As a MicroShift user, I want to declare remote cluster
  CIDRs and next-hop addresses in my config file so that
  pods can communicate with pods and services on remote
  clusters without manual route management.

* As a MicroShift user, I want to reach services on a
  remote cluster using DNS names (e.g.,
  `myservice.mynamespace.svc.remote-cluster.local`).

* As a MicroShift user, I want cross-cluster traffic to
  preserve source pod IPs so that NetworkPolicies on the
  remote cluster can enforce access control based on
  originating pod identity.

### Goals

1. Pod-to-Pod, Pod-to-Service (ClusterIP via DNS),
   and Pod-to-Service (headless via DNS) communication
   between MicroShift clusters with non-overlapping
   CIDRs.
2. Declarative configuration via the MicroShift config
   file with validation.
3. Source pod IP preservation by bypassing SNAT for
   C2CC traffic.
4. IPv4, IPv6, and dual-stack support.
5. Cross-cluster DNS service discovery with configurable
   domain names.
6. Per-remote-cluster health and route status reporting
   via a status CR.
7. Controller resilience to OVN-K restarts, DB wipes,
   MicroShift service restarts, host reboots, 
   and firewall reloads.

### Non-Goals

1. Automatic peer discovery — remote clusters must be
   explicitly configured.
2. IPSec or WireGuard tunnel management — C2CC provides
   routing; encryption is user-managed but documentation
   will be provided.
3. Multi-tenancy or per-namespace routing — C2CC
   operates at the cluster network level.
4. Overlapping CIDRs — clusters must use distinct
   network ranges.

## Proposal

C2CC adds the following components:

1. **Configuration & Validation** — A `c2cc` section in
   the MicroShift config defining `remoteClusters[]`,
   each with `nextHop`, `clusterNetwork`,
   `serviceNetwork`, and optional `domain`.
   It may include an option to disable DNS caching.

2. **Route Manager Controller** — Maintains OVN static
   routes on the Gateway Router, nftables masquerade
   bypass rules and node annotations for source IP
   preservation, and Linux kernel routes in dedicated
   routing tables.

3. **CoreDNS Cross-Cluster DNS** — Server block
   injection for each remote cluster with domain
   rewrite and forwarding to the remote cluster's DNS.

4. **Status CR & Healthcheck** — Reports per-remote-cluster
   route state, data-plane reachability, health, latency,
   and errors. Healthcheck and latency measurement use a
   pod deployed on each cluster to verify end-to-end
   connectivity through the full C2CC path.

### Workflow Description

**MicroShift user** is a human user responsible for
configuring, operating, and deploying workloads on
MicroShift clusters.

#### Initial Setup

1. The user plans non-overlapping CIDRs for each
   cluster (e.g., Cluster A uses the defaults:
   `10.42.0.0/16` pods, `10.43.0.0/16` services;
   Cluster B: `10.45.0.0/16` pods, `10.46.0.0/16`
   services).

2. On Cluster B, the user overrides the default cluster
   and service networks in the MicroShift config so they
   do not overlap with Cluster A:

   ```yaml
   # Cluster B config — override default subnets
   network:
     clusterNetwork:
       - 10.45.0.0/16
     serviceNetwork:
       - 10.46.0.0/16
   ```

3. On each cluster, the user adds a `c2cc` section
   to the MicroShift config file (or a drop-in)
   pointing at the remote cluster:

   ```yaml
   # Cluster A config
   c2cc:
     remoteClusters:
       - nextHop: "192.168.122.101"
         clusterNetwork: 
            - "10.45.0.0/16"
         serviceNetwork: 
            - "10.46.0.0/16"
         domain: "cluster-b.remote"
   ```

   ```yaml
   # Cluster B config
   c2cc:
     remoteClusters:
       - nextHop: "192.168.122.100"
         clusterNetwork:
            - "10.42.0.0/16"
         serviceNetwork:
            - "10.43.0.0/16"
         domain: "cluster-a.remote"
   ```

4. On each host, the user configures the firewall to
   allow cross-cluster traffic. The remote cluster's pod
   and service CIDRs must be added to the trusted zone,
   along with the remote host IP:

   ```bash
   # On Cluster A — trust Cluster B's networks and host
   sudo firewall-cmd --permanent --zone=trusted \
     --add-source=10.45.0.0/16
   sudo firewall-cmd --permanent --zone=trusted \
     --add-source=10.46.0.0/16
   sudo firewall-cmd --permanent --zone=trusted \
     --add-source=192.168.122.101/32
   sudo firewall-cmd --reload
   ```

   ```bash
   # On Cluster B — trust Cluster A's networks and host
   sudo firewall-cmd --permanent --zone=trusted \
     --add-source=10.42.0.0/16
   sudo firewall-cmd --permanent --zone=trusted \
     --add-source=10.43.0.0/16
   sudo firewall-cmd --permanent --zone=trusted \
     --add-source=192.168.122.100/32
   sudo firewall-cmd --reload
   ```

5. The user restarts MicroShift on each host.

6. MicroShift validates the configuration. If validation
   fails, MicroShift logs errors and does not start.

7. The C2CC controller reconciles OVN routes, SNAT
   policies, kernel routes, and CoreDNS config.

8. The user verifies connectivity:
   ```bash
   oc get c2ccstatus
   ```

#### Config Removal

1. The user removes the `c2cc` section from the
   config and restarts MicroShift.
2. The controller cleans up all C2CC-owned OVN routes,
   SNAT bypass state (node annotations, nftables
   rules, service routing tables), kernel routes
   (table 200), CoreDNS server blocks, and status CR.

### API Extensions

One new CRD: **C2CCStatus** — a status-only resource
reporting per-remote-cluster connectivity state (route
status, health, errors, last reconciliation timestamp).
Updated by the C2CC controller; does not modify existing
resources.

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

This enhancement is designed specifically for MicroShift.
The C2CC controller is lightweight (negligible CPU,
under 1 MB memory for typical deployments). Cross-cluster
traffic travels as plain IP on the underlay network.
Users should account for MTU implications when using IPSec.

#### OpenShift Kubernetes Engine

N/A

### Implementation Details/Notes/Constraints

**Configuration**:
- `C2CC` struct with `RemoteClusters []RemoteCluster`
  (`NextHop`, `ClusterNetwork`, `ServiceNetwork`,
  `Domain`)
- Validation: CIDR format, local↔remote overlap,
  remote↔remote overlap, routing loops, mask bounds

**Route Manager Controller**:
- Persistent libovsdb NBDB connection with reconnect
  and full resync on DB wipe
- Small OVN NB models: either generated from
  `ovn-nb.ovsschema` via `libovsdb/cmd/modelgen` or
  handwritten (importing the model package would import
  many other ovn-kubernetes packages)
- Route ownership via ExternalIDs (`microshift-c2cc`
  owner tag)
- Event-driven reconciliation with periodic fallback:
  the controller subscribes to change notifications for
  each managed subsystem and reconciles immediately when
  external modifications are detected (e.g., OVN-K
  flushing nftables chains, routes being deleted). A
  periodic fallback ticker covers subsystems without
  event APIs (IP rules) and acts as a safety net.
  Subscriptions used:
  - OVN routes: libovsdb `Monitor()` on the
    `LogicalRouterStaticRoute` table
  - Linux kernel routes: `netlink.RouteSubscribe()`
    for table 200
  - nftables rules: netlink `NFNLGRP_NFTABLES`
    subscription for chain flush detection
  - Node annotation: Kubernetes `Watch()` on the local
    Node object
  - IP rules: no subscription API available — covered
    by periodic fallback

**SNAT Bypass**: Three-layer approach to preserve pod
source IPs end-to-end:
1. **nftables masquerade bypass** (egress / sending
   side) — inserts `ip daddr <remote CIDR> return`
   rules at the top of OVN-K's
   `ovn-kube-pod-subnet-masq` nftables chain. Without
   this, OVN-K masquerades all outbound pod traffic to
   the node's underlay IP, destroying the original pod
   source before it even leaves the host. Rules are
   tagged with a `c2cc-no-masq` comment. OVN-K flushes
   this chain on startup and during its own
   reconciliation, destroying all external rules. C2CC
   detects this via nftables netlink event subscription
   (`NFNLGRP_NFTABLES`) and re-inserts rules
   immediately.
2. **Node annotation** (cooperative API) — sets
   `k8s.ovn.org/node-ingress-snat-exclude-subnets`
   on the Node object. OVN-K reads this annotation
   and internally handles both management port SNAT
   exclusions (via the `mgmtport-no-snat-subnets` nft
   set) and Gateway Router SNAT match modifications.
   This is the same cooperative API used by Submariner,
   avoiding direct GR SNAT entry manipulation and the
   resulting reconciler conflicts with OVN-K.
3. **Service traffic via management port** (inbound
   service traffic) — routes inbound cross-cluster
   service traffic through the management port
   (`ovn-k8s-mp0`) instead of `br-ex` using
   source-based policy routing rules (dedicated
   routing table 201). This ensures load balancing is
   handled by the node's logical switch (DNAT only)
   rather than the Gateway Router (which adds implicit
   SNAT to the join switch IP). Only traffic from
   remote cluster sources is rerouted; local service
   traffic is unaffected.

**Linux Kernel Routes**: Dedicated routing tables
isolated from the main table. Remote cluster CIDRs are
routed to the remote node via the physical NIC. A
separate table routes local service CIDRs via the
management port, activated only for traffic from remote
cluster sources.

**CoreDNS Integration**: Per-remote-cluster server
block with domain rewrite and forwarding to the remote
DNS IP (10th IP in service CIDR).

**Healthcheck Pod**: The C2CC controller deploys a
lightweight probe Pod in a dedicated namespace when 
C2CC is active. Each cluster runs its own probe Pod.
The local probe pod sends
requests to the remote probe Pod, validating
the full C2CC data path end-to-end (pod → OVN overlay
→ GR → underlay → remote GR → remote overlay → remote
pod). This catches failures at any layer — OVN routes,
SNAT bypass, kernel routes — that host-level probes
(like ICMP ping between nodes) would miss. Latency is
measured from probe RTT (min, max, avg, stddev over a
rolling window). The controller removes the probe pod
when C2CC config is removed.

### Risks and Mitigations

**No mutual authentication**: Any host reachable at the
configured nextHop is implicitly trusted. IPSec
documentation will be provided for encryption and
authentication.

**IPSec MTU overhead**: IPSec encapsulation reduces the
effective MTU, which can cause packet drops. MTU
requirements will be documented.

**Half-configured state**: If any subsystem (e.g., kernel
routes) fails while others succeed, the controller
reverts all changes for that remote cluster to avoid
partial connectivity. The status CR reports the failure.

### Drawbacks

- Static configuration does not scale beyond small
  deployments without external automation.
- No overlapping CIDR support; requires upfront
  planning.
- Tightly coupled to OVN-K internals.

## Alternatives (Not Implemented)

- **OVN Interconnect (OVN-IC)** — Requires central
  IC-NB/IC-SB databases and extends L2 between
  clusters via transit switches, neither of which
  is desirable for independent edge deployments.
- **Submariner** — Feature-rich but higher resource
  consumption, and being a third-party solution
  introduces additional maintenance burden.
- **OVN-Kubernetes + Route Advertisement** — Depends
  on BGP infrastructure, which may be too heavy for
  edge deployments.

## Open Questions

1. **Remote cluster count limits**: What is the
   practical upper bound? Should we limit that? Up
   to how many interconnected clusters should we test?

2. **Healthcheck pod discovery**: How does the local
   probe pod discover the remote probe pod's IP? DNS
   is not reliable for this since the user will be
   able to override CoreDNS configuration completely.
   Maybe we can hardcode Cluster IP like the CoreDNS?

3. **Routing table ID**: Are tables 200 and 201 safe to hardcode,
   or should they be configurable to avoid conflicts with
   other software on the host (e.g., Libreswan,
   NetworkManager)?

## Test Plan

Requires multi-VM test infrastructure: two VMs
with independent MicroShift configs.

**Functional**: Pod-to-Pod (IP), Pod-to-ClusterIP-Service
(DNS), and Pod-to-Headless-Service (DNS) in both
directions; IPv4, IPv6, dual-stack; config removal
cleanup. All cross-cluster service access is via DNS
(`<svc>.<ns>.svc.<remote-domain>`). ClusterIP Services
exercise the service routing path (table 201); headless
Services (`clusterIP: None`) resolve directly to pod IPs,
exercising the pod routing path (table 200).

**Resilience**: MicroShift restart, host reboot, network
loss, OVN-K restart, firewall reload, OVN NB DB wipe.

**Route Stability**: C2CC routes persist across OVN-K
reconciliation cycles.

**DNS**: Negative caching, multiple remote cluster
domain isolation.

**IPSec**: Libreswan setup, ESP verification, MTU
validation with double encapsulation.

**Upgrade**: Verify C2CC connectivity survives a
MicroShift upgrade on one or both clusters.

**Cross-version / Cross-OS**: Validate connectivity
between hosts running different RHEL versions (e.g.,
RHEL 9 and RHEL 10) and different MicroShift versions
(max version difference is between EUS releases).

## Graduation Criteria

### Dev Preview -> Tech Preview

- Documentation
- Core functionality works end-to-end for 2 clusters
- Status CR reports accurate state
- E2E tests cover functional and basic resilience
- IPv4 and IPv6 validated
- All resilience scenarios pass
- IPSec validated end-to-end

### Tech Preview -> GA

TBD

### Removing a deprecated feature

No features to be deprecated

## Upgrade / Downgrade Strategy

Upgrade: C2CC is disabled by default and only
activates when the user adds a `c2cc` section to the
MicroShift config.

Downgrades are not supported on MicroShift, only roll backs.
If user rolls back to a version without C2CC (without prior 
reconfiguration), the host level routes will persist.

## Version Skew Strategy

C2CC operates within a single MicroShift instance.
Cross-cluster version skew is handled by the fact that
C2CC operates at the routing level (OVN static routes +
kernel routes), which is version-independent.

## Operational Aspects of API Extensions

The C2CCStatus CRD is status-only — no webhooks or
finalizers. Updated on each reconciliation cycle.

**Failure modes:**
- NBDB connection lost: libovsdb reconnects
  automatically, monitor subscription triggers full
  resync on reconnect, status reports unhealthy until
  recovery.
- OVN-K restart / nftables flush: detected via netlink
  event subscription, rules re-inserted within seconds.
- Kernel route deletion: detected via
  `RouteSubscribe()`, routes re-created immediately.
- Kernel route failure: OVN routes exist but traffic
  cannot exit overlay, status reports degraded.
- CoreDNS failure: IP connectivity works but DNS does
  not, status reports degraded.

## Support Procedures

**Detecting issues**: `oc get c2ccstatus`, `journalctl
-u microshift --grep c2cc`.

**Diagnosing**: Check OVN routes (`ovn-nbctl
lr-route-list`), kernel routes (`ip route show table
200`), node annotations, nftables rules, CoreDNS
config.

**Disabling**: Remove `c2cc` config section and
restart. Emergency: `ip route flush table 200`.

## Infrastructure Needed

- Test Harness updates to accomodate multiple scenarios
  with VMs running different configs.
