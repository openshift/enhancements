---
title: microshift-cluster-to-cluster-connectivity
authors:
  - '@pmtk'
reviewers:
  - '@pacevedom'
  - "@eslutsky"
  - "@copejon"
  - '@agullon'
  - '@vthapar, Networking Expert'
approvers:
  - "@jerpeter1"
api-approvers:
  - None
creation-date: 2026-03-30
last-updated: 2026-05-06
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
5. One-way connectivity — C2CC requires bidirectional
   configuration. Both clusters must configure each
   other as remote peers. One-way setups (only one
   side configured) are not supported.

## Proposal

C2CC adds the following components:

1. **Configuration & Validation** — A `c2cc` section in
   the MicroShift config defining `remoteClusters[]`,
   each with `nextHop`, `clusterNetwork`,
   `serviceNetwork`, and optional `domain`.

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
   clusterToCluster:
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
   clusterToCluster:
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
   and service CIDRs must be added to the trusted zone:

   ```bash
   # On Cluster A — trust Cluster B's pod and service CIDRs
   sudo firewall-cmd --permanent --zone=trusted \
     --add-source=10.45.0.0/16
   sudo firewall-cmd --permanent --zone=trusted \
     --add-source=10.46.0.0/16
   sudo firewall-cmd --reload
   ```

   ```bash
   # On Cluster B — trust Cluster A's pod and service CIDRs
   sudo firewall-cmd --permanent --zone=trusted \
     --add-source=10.42.0.0/16
   sudo firewall-cmd --permanent --zone=trusted \
     --add-source=10.43.0.0/16
   sudo firewall-cmd --reload
   ```

   Note: do not add the remote host IP to the trusted
   zone unless IPSec is configured (IPSec IKE negotiation
   requires host-to-host UDP 500/4500 traffic). Omitting
   the host IP prevents host-originated traffic from
   reaching remote pods at the firewall level.

   Firewall configuration is intentionally manual.
   MicroShift does not automate firewall changes because
   edge deployments often have site-specific firewall
   policies managed by external tooling or locked-down
   by organizational policy. Automating firewall rules
   could conflict with these constraints and create
   hard-to-diagnose security issues. Misconfigured
   firewalls are a common failure mode — if cross-cluster
   connectivity does not work, verifying that remote pod
   and service CIDRs are in the trusted zone should be
   the first troubleshooting step.

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

When routing table IDs are at their default values,
the user removes the `c2cc` section from the config
and restarts MicroShift. On startup, the C2CC
controller detects that C2CC is no longer enabled and
performs best-effort cleanup of all C2CC-owned state:
OVN routes, node SNAT annotations, nftables
masquerade bypass rules, Linux kernel routes and
rules, CoreDNS server blocks (removed by template
re-rendering without C2CC blocks), and the status CR.

If the user has overridden routing table IDs, cleanup
is a two-stage process: first, remove the
`remoteClusters` entries while keeping the table ID
overrides, then restart MicroShift to clean up
routes from the configured tables. Afterwards, remove
the entire `c2cc` section and restart again.

### API Extensions

One new CRD: **C2CCStatus** — a status-only resource
reporting per-remote-cluster connectivity state (route
status, health, errors, last reconciliation timestamp).
Updated by the C2CC controller; does not modify existing
resources.

**RBAC**: The C2CCStatus CR contains cluster topology
information (remote CIDRs, next-hop addresses, health
status). Write access is restricted to the C2CC
controller's service account. Read access is granted
to `system:authenticated` (any authenticated user) via
a ClusterRole, consistent with other MicroShift status
resources. Cluster administrators who need tighter
access control can override this with a custom
ClusterRoleBinding.

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
Users should account for MTU implications when using
IPSec.

**Resource scaling with N remote clusters**: Each
remote cluster adds a constant number of managed
resources:
- OVN static routes: 1 per remote CIDR (typically
  2–4 per remote cluster: pod + service, per IP
  family)
- nftables rules: 1 masquerade bypass rule per
  remote CIDR
- Linux kernel routes: 1 per remote CIDR in table
  200, plus 1 per local service CIDR in table 201
  (shared across all remotes)
- ip rules: 1 per remote CIDR (source-based policy
  routing for table 201)
- CoreDNS server blocks: 1 per remote cluster with
  a `domain` configured
- Node annotation: single annotation value listing
  all remote CIDRs (comma-separated)

The reconcile loop iterates all subsystems
sequentially. Each subsystem performs an idempotent
diff (desired vs. actual state). With N remotes, the
per-cycle cost grows linearly but remains negligible
for expected deployments (under 10 remote clusters).
The periodic reconcile interval (10s) is independent
of N.

The practical upper bound for remote cluster count is
an open question (see Open Questions). Resource
consumption is not expected to be the limiting factor;
operational complexity of managing N×(N-1) config
entries is the more likely constraint.

#### OpenShift Kubernetes Engine

N/A

### Implementation Details/Notes/Constraints

**Configuration**:
- `C2CC` struct with `RemoteClusters []RemoteCluster`
- Validation: CIDR format, local↔remote overlap,
  remote↔remote overlap, routing loops, mask bounds,
  IP family consistency, host IP containment

Schema for `RemoteCluster`:

| Field | Type | Required | Default | Constraints |
|-------|------|----------|---------|-------------|
| `nextHop` | string (IP) | yes | — | Valid IPv4 or IPv6 address. Must not equal local node IP. Must not duplicate another remote's nextHop. Must not be contained in any configured CIDR. |
| `clusterNetwork` | []string (CIDR) | yes | — | At least one entry. Valid CIDR notation. Min mask: /8 (IPv4), /32 (IPv6). Max one IPv4 and one IPv6 entry (dual-stack). Must not overlap with local or other remote CIDRs. Must not contain any host interface IP. |
| `serviceNetwork` | []string (CIDR) | yes | — | Same constraints as `clusterNetwork`. Must have same cardinality as `clusterNetwork` with matching IP families at each index. |
| `domain` | string (DNS name) | no | `""` (no DNS forwarding) | Must be a valid DNS-1123 subdomain. Must not duplicate another remote's domain. When set, CoreDNS server blocks are generated for cross-cluster DNS forwarding. |

Schema for `C2CC` (top-level fields beside
`remoteClusters`):

| Field | Type | Required | Default | Constraints |
|-------|------|----------|---------|-------------|
| `routeTable` | int | no | `200` | Linux routing table ID for remote cluster pod/service CIDR routes. Must not conflict with system tables (0, 253, 254, 255) or `serviceRouteTable`. |
| `serviceRouteTable` | int | no | `201` | Linux routing table ID for service traffic rerouting via the management port. Must not conflict with system tables or `routeTable`. |

C2CC is disabled when `remoteClusters` is empty or
absent. Requires OVN-Kubernetes CNI (`network.cniPlugin`
must be `""` or `"ovnk"`). The table ID fields allow
avoiding conflicts with other software on the host
(e.g., Libreswan, NetworkManager) that may also use
custom routing tables.

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

**CoreDNS Integration**: MicroShift manages the CoreDNS
Corefile via a ConfigMap template
(`assets/components/openshift-dns/dns/configmap.yaml`).
When C2CC is enabled and remote clusters have a `domain`
configured, the DNS controller renders per-remote-cluster
server blocks into the Corefile template at startup via
the `C2CCDNSBlocks` template variable. The
`RenderC2CCDNSBlocks()` function in `pkg/config/c2cc.go`
generates a server block for each remote cluster that has
a non-empty `Domain`. Each block performs domain rewrite
(`.remote-domain` → `.cluster.local`) and forwards to the
remote cluster's DNS IP (10th IP of the remote
`serviceNetwork[0]`, computed during config validation).
Example output:
```
other-cluster.local:5353 {
    bufsize 1232
    errors
    log . {
        class error
    }
    rewrite stop name suffix .other-cluster.local .cluster.local answer auto
    forward . 10.46.0.10
    cache 10 {
        denial 9984 10
    }
}
```
The server blocks are rendered once during the DNS
component startup. Changes to the C2CC config require
a MicroShift restart to take effect in CoreDNS. Because
the blocks are part of the template rendering pipeline,
they do not conflict with CoreDNS reconciliation — the
same template is used on every render cycle.

**Healthcheck Pod**: The C2CC controller deploys a
lightweight probe Pod and a ClusterIP Service with a
well-known ClusterIP in a dedicated namespace when
C2CC is active. Each cluster runs its own probe Pod.
The local C2CC controller discovers the remote probe
by computing the remote's well-known healthcheck
ClusterIP from the remote cluster's `serviceNetwork`
(already present in the C2CC config), avoiding any
dependency on DNS or additional discovery mechanisms.
The probe validates the full C2CC data path end-to-end
(pod → OVN overlay → GR → underlay → remote GR →
remote overlay → remote pod). This catches failures at
any layer — OVN routes, SNAT bypass, kernel routes —
that host-level probes (like ICMP ping between nodes)
would miss. Latency is measured from probe RTT (min,
max, avg, stddev over a rolling window). The
controller removes the probe Pod and Service when C2CC
config is removed.

**NetworkPolicy for inbound cross-cluster traffic**:
C2CC does not create any NetworkPolicy resources.
Allowing or restricting ingress from remote cluster
pod CIDRs is the user's responsibility. Users who
deploy NetworkPolicies with default-deny ingress must
add explicit rules to allow traffic from remote pod
CIDRs in each namespace that should be reachable
cross-cluster. Because SNAT is bypassed, remote
traffic arrives with the original pod source IP,
so standard `ipBlock` selectors in NetworkPolicy
work correctly for cross-cluster access control.

**Host-to-Pod traffic prevention**: C2CC is designed
for pod-to-pod and pod-to-service communication only.
Traffic originating from a host (whether the configured
nextHop or any other host on the subnet) should not
reach pods on a remote cluster. C2CC does not prevent
host-to-pod traffic by itself — Kubernetes
NetworkPolicies are namespace-scoped and cannot
reliably cover all workloads without introducing
isolation side-effects that break local traffic.
Host-to-pod prevention requires IPSec: when configured
with transport or tunnel mode between cluster subnets,
only authenticated peers can exchange traffic, and any
unauthenticated host on the subnet is rejected at the
network layer. The firewall configuration also
contributes by trusting only remote pod and service
CIDRs, not host IPs.

### Risks and Mitigations

**No mutual authentication**: Any host reachable at the
configured nextHop is implicitly trusted. Without
IPSec, the threat model is: any host on the same L2
segment (or any host that can route to the local node)
can spoof the nextHop IP address and inject traffic
into pods on the cluster. Because C2CC bypasses SNAT,
spoofed traffic would arrive with an attacker-chosen
source IP, potentially bypassing NetworkPolicies that
allow traffic from remote pod CIDRs. IPSec is strongly
recommended for production deployments to provide both
encryption and mutual authentication between cluster
nodes. Documentation will cover Libreswan
configuration with shunt policies
(`failureshunt=drop`, `negotiationshunt=drop`) and
nftables `meta ipsec missing` rules to prevent
plaintext traffic.

**IPSec MTU overhead**: C2CC itself does not adjust MTU
settings — pod MTU is determined by the OVN-Kubernetes
CNI based on the configured Geneve overlay overhead.
The default OVN Geneve overhead is 100 bytes (on a
1500-byte physical MTU, pod MTU is 1400). When IPSec
is added, the additional encapsulation overhead depends
on the mode and cipher suite: ESP with AES-GCM-256 in
transport mode adds ~54–74 bytes, tunnel mode adds
~73–93 bytes (including the outer IP header). The user
must ensure that the physical network MTU accommodates
the combined overhead (Geneve + IPSec). If the physical
MTU cannot be increased, the user should reduce the OVN
MTU via MicroShift configuration to prevent
fragmentation and packet drops. MTU sizing guidance and
a calculation reference will be included in
documentation.

**Half-configured state**: If any subsystem (e.g., kernel
routes) fails while others succeed, the controller
does not roll back the successfully applied changes.
Because subsystems operate on shared OS resources
(routing tables, OVN DB, nftables), a failure
typically indicates a systemic problem rather than a
peer-specific one, and a rollback would likely fail
for the same reason. Instead, the controller marks
the affected remote cluster as degraded in the status
CR and retries the failed subsystem on the next
reconciliation cycle (every 10s or upon incoming event).

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

**NetworkPolicy**: Verify that NetworkPolicies on the
remote cluster can enforce access control based on
cross-cluster source pod IPs. A pod from Cluster A
should be able to reach a pod in a namespace where
ingress is allowed but be blocked in a namespace where
a NetworkPolicy denies ingress from Cluster A's pod
CIDR. This validates end-to-end SNAT bypass — without
source IP preservation, the remote cluster would see
the node IP and pod-level NetworkPolicies would not
match.

**Networking Regression**: Run existing MicroShift
networking test suites (networking smoke, DNS, router)
against a C2CC-configured cluster to verify that C2CC's
extra OVN routes, ip rules, nftables rules, and node
annotations do not break normal single-cluster
networking.

**Resilience**: MicroShift restart, host reboot, network
loss, OVN-K restart, firewall reload, OVN NB DB wipe.

**Route Stability**: C2CC routes persist across OVN-K
reconciliation cycles.

**DNS**: Negative caching, multiple remote cluster
domain isolation.

**IPSec**: Libreswan setup, ESP verification, MTU
validation with double encapsulation, plaintext
rejection (verify traffic is dropped — not sent in
plaintext — when IPSec SAs are absent and enforcement
policies are configured), host-to-pod rejection (curl
directly from Cluster A's host to a pod on Cluster B
— should be rejected since the host does not have
IPSec credentials and cannot establish a security
association with the remote node).

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
- Upgrade path validated across consecutive
  MicroShift releases with C2CC enabled
- Cross-version and cross-OS compatibility validated
  (RHEL 9 ↔ RHEL 10, EUS version skew)

### Tech Preview -> GA

- Scale testing with target upper bound of remote
  clusters (resource consumption profiled and
  documented)
- Customer validation from at least one Tech Preview
  adopter
- Complete troubleshooting and operational
  documentation

### Removing a deprecated feature

No features to be deprecated

## Upgrade / Downgrade Strategy

Upgrade: C2CC is disabled by default and only
activates when the user adds a `c2cc` section to the
MicroShift config.

Downgrades are not supported on MicroShift, only
rollbacks. If a user rolls back to a version without
C2CC support (without prior reconfiguration), the
following C2CC-owned state may persist:
- Linux kernel routes in routing tables 200/201 and
  associated ip rules
- nftables masquerade bypass rules in the
  `ovn-kube-pod-subnet-masq` chain
- Node annotation
  `k8s.ovn.org/node-ingress-snat-exclude-subnets`
  with C2CC CIDRs
- C2CCStatus CR

OVN static routes are cleaned up by the existing
`pre-rollback.sh` script, which wipes the OVN NB DB
as part of the standard rollback procedure. The
nftables rules and node annotation will be cleaned up
by OVN-K on its next restart or reconciliation cycle.
Linux kernel routes in dedicated tables do not
interfere with the main routing table. The C2CCStatus
CR is inert. To manually clean up remaining state,
flush the routing tables (`ip route flush table 200;
ip route flush table 201`) and delete the CR.

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
