---
title: infrastructure-driven-cluster-dns-and-load-balancing
authors:
  - "@eggfoobar"
reviewers:
  - "@joelspeed" # Infrastructure API and cluster-config-operator integration
  - "@mkowalski" # On-prem networking, cloud DNS, and CNO reconciliation
  - "@cybertron" # Runtime networking and legacy API compatibility
  - "@pmtk" # SNO DNS-only behavior and OCPSTRAT-3609 integration
  - "@team-mco" # MCO rendering and node rollout
  - "@zaneb" # Platform integration, bootstrap, and provisioning isolation
  - "@sadasu" # Installer configuration and bootstrap
  - "@jaypoulz"
  # Assisted/ABI and image-based-install reviewer coverage remains to be confirmed.
approvers:
  - "@jerpeter"
api-approvers:
  - "@joelspeed"
creation-date: 2026-09-04
last-updated: 2026-11-10
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3602
  - https://redhat.atlassian.net/browse/OCPEDGE-2902
see-also:
  - "/enhancements/topologies/mutable-topology.md"
  - "/enhancements/network/external-lb-vips.md"
  - "/enhancements/network/on-prem-mutable-vips.md"
  - "/enhancements/network/baremetal-networking.md"
  - "/enhancements/cloud-integration/infrastructure-external-platform-type.md"
replaces: []
superseded-by: []
---

# Infrastructure-Driven Cluster DNS and Load Balancing

## Summary

This enhancement lets administrators configure infrastructure DNS and
API/ingress load balancing independently of platform identity. Shared
configuration on the Infrastructure singleton selects networking ownership
without changing platform or machine provisioning. The initial implementation
serves platform None Single Node OpenShift (SNO): DNS-only is the new-install
default when the feature is enabled, and managed load balancing is an explicit
installation-time or post-installation choice. The Machine Config Operator (MCO)
applies this policy using the existing infrastructure networking components. The
same configuration can support future mutable topologies and additional
platforms without adding another copy of networking fields under each platform
discriminator.

## Motivation

Platform identity currently selects several distinct concerns: provider
integration, machine provisioning, and infrastructure networking. Administrators
who manage their own machines may nevertheless need OpenShift-managed DNS or API
and ingress load balancing. Selecting a different platform solely to obtain
networking entangles these concerns and does not provide a path for existing
None clusters.

There are two immediate needs. First, SNO infrastructure DNS differs by
installation method: standard None can use external DNS, while Assisted
Installer and Agent-based Installer (ABI) conditionally generate an on-node
dnsmasq configuration. The [SNO DNS-only proposal][sno-dns-design] standardizes
this using MCO-managed CoreDNS and discovered node addresses, without VIPs or
load-balancer pods. Second, an administrator of an existing None cluster needs
to be able to move API and ingress LB ownership to OpenShift without
reinstalling or changing platform. An install-only option cannot serve
already-deployed clusters. An explicit managed-LB selection uses reserved
API/ingress VIPs and starts the stack, including on an unchanged SNO. It does
not add node failover to a single-node deployment.

Decoupling this choice from platform and topology also prepares for mutable
topologies. A cluster can establish managed API and ingress VIPs before adding
nodes and retain the same ownership and VIPs as its backends grow. Networking
configuration does not initiate node addition or a topology transition; those
operations remain in the [mutable-topology
enhancement](topologies/mutable-topology.md).

Adding another platform-specific networking object would repeat the duplication
already present in on-prem APIs. This proposal instead introduces shared intent
and an explicit compatibility boundary. Platform-agnostic configuration does not
mean platform-independent implementation: cloud LB addresses and on-prem VIPs
have different address sources, bootstrap dependencies, and resolver
integration.

Infrastructure DNS here is node-level resolution for cluster API, internal API,
and application names, not DNS Operator-managed pod and Service DNS.
Authoritative external DNS and resolution for off-cluster clients remain
separate responsibilities.

### API and ingress terminology

In this proposal, an **endpoint** is an API or default-ingress network entry
point, identified by its DNS name, resolved IP address, and service port. The
term is limited to the following entry points:

| Entry point | DNS name | Purpose |
| --- | --- | --- |
| API | `api.<cluster>.<base-domain>` | Client access to the Kubernetes API |
| Internal API | `api-int.<cluster>.<base-domain>` | Kubernetes API access for nodes and cluster components |
| Default ingress | Application hostnames under `*.apps.<cluster>.<base-domain>` | Access to application routes through the default ingress routers |

In the proposed None/on-prem path, managed infrastructure DNS resolves these
names to discovered node addresses for DNS-only SNO or configured VIPs for
managed LB. The `api` and `api-int` names share the API VIP list, while
application names use the ingress VIP list. DNS selects the address a client
uses; VIP management and the API/ingress serving components provide access
through that address.

**Managed endpoints** means OpenShift manages those API and ingress VIPs and the
components serving traffic through them. **Endpoint health** means API requests
and application routing work through the selected addresses. Address publication
alone does not establish that health.

The proposed `status.networking.endpoints` object reports only resolved API and
ingress IP addresses. It does not contain Kubernetes `Endpoints` or
`EndpointSlice` resources, individual workload Service addresses, or backend
node membership. Machine Config Server (MCS) access on port 22623 is verified
separately and is not another address list in this object.

### User Stories

- As a None SNO cluster creator, I want consistent infrastructure DNS without
  reserving VIPs or running load-balancer components, so that my single-node
  installation has a small, predictable networking footprint.
- As a cluster creator, I want an explicit managed-DNS opt-out, so that I can
  retain my installation method's supported legacy DNS behavior.
- As an administrator of an existing None SNO cluster, I want to select
  OpenShift-managed API and ingress load balancing without changing platform or
  reinstalling the cluster.
- As an administrator planning a supported topology change, I want to retain
  configured networking ownership and API/ingress addresses while nodes are
  added, so that growth does not require another networking API or a platform
  migration.
- As an existing on-prem administrator, I want my configuration and supported
  operations preserved during API evolution, so that an upgrade does not
  unexpectedly change DNS or LB ownership.
- As an OpenShift developer, I want reusable networking configuration, so that
  enabling another supported platform does not require duplicating the same DNS
  and LB API fields.
- As a fleet administrator, I want to distinguish configuration rejection,
  rollout in progress, rollout failure, and API/ingress access failure, so that
  I can recover a networking change without guessing which configuration is
  active.

### Goals

- Separate infrastructure DNS and API/ingress LB ownership from platform
  identity.
- Provide DNS-only None SNO with a new-install managed-DNS default when its
  feature is enabled and an explicit installation opt-out.
- Allow an explicit managed-LB selection on None SNO at installation or after
  installation, preserving administrator ownership when not selected.
- Preserve existing cluster behavior and supported platform-specific operations
  while introducing the shared API.
- Make complete, accepted networking configuration start reconciliation without
  a separate activation command or required topology change.
- Define how future topology transitions consume networking readiness while
  retaining separate node, etcd, and topology lifecycle responsibilities.
- Use the shared schema for External on-prem networking as well as None.
- Make additional platform adoption possible without requiring a new copy of the
  common configuration or replacing the initial API.

### Non-Goals

- Enabling managed load balancing by default on SNO.
- Implementing node addition, etcd scaling, or topology-transition
  orchestration.
- Changing platform identity, deprecating None, or migrating None to External.
- Enabling machine provisioning, Ironic, Metal3, BMO, or CBO through networking
  configuration, or changing workload Service load-balancer behavior.
- Universal platform enablement, new cloud-LB implementations, or migration of
  every platform's DNS implementation in the first delivery.
- Enabling the shared DNS/LB modes for fresh multi-node None installations in
  the initial delivery; existing administrator-managed HA networking is
  preserved.
- External runtime enablement without its own support, installer, and
  integration review.
- General active-DNS teardown, reverse LB ownership, arbitrary VIP replacement,
  address allocation, or authoritative external-DNS management.
- New BGP, arbiter, hosted-control-plane, or MicroShift support. Existing
  platform modes remain subject to compatibility preservation.

## Proposal

Extend the Infrastructure singleton with shared networking configuration outside
the platform-specific spec/status unions. Keep platform identity for provider
integration and retain provider-specific inputs where their semantics differ.
The shared model separates DNS policy, API/ingress LB ownership, and resolved
addresses. None is the first implementation target, not a separate networking
API shape. External on-prem networking uses these same fields; its runtime and
installer qualification are coordinated with the [External networking
proposal][external-networking].

Introduce a common interpretation of accepted configuration for consumers.
Legacy-only clusters continue to derive effective configuration from their
existing fields and defaults. Explicit shared configuration is used only after
validation and acceptance for a supported scenario. MCO renders from that
effective configuration, not from arbitrary unaccepted spec changes.

| Scenario | DNS and API/ingress access | Scope |
| --- | --- | --- |
| New None SNO with the feature enabled | Managed DNS uses discovered node addresses; LB is user-managed, with no VIP requirement | Initial installation default, aligned with 3609 |
| None SNO with explicit managed-LB selection | Dedicated API/ingress VIPs and keepalived/HAProxy, including while the cluster remains single-node | Initial installation-time and post-installation workflow |
| None SNO after a supported transition to three-node compact | Retains selected networking ownership and VIPs; runtime components update their backends | Future mutable-topology integration using the same configuration |
| None without shared networking configuration | Existing administrator/installer-owned behavior | Preserved; no automatic upgrade adoption |
| Existing on-prem platforms | Existing defaults, VIP ownership, DNS behavior, and supported mutations | Compatibility requirement |
| External | Shared networking fields outside the External platform object | Schema alignment now; runtime qualification with the External proposal |
| Cloud cluster-hosted DNS | Existing provider-owned LBs and their discovered addresses | Preserved; cloud mappings require separate integration work |

DNS and LB have separate selectors. The initial None scenarios and the remaining
combination decision are:

| Infrastructure DNS | Load balancer | Behavior |
| --- | --- | --- |
| `UserManaged` | `UserManaged` | Explicit installation opt-out; retain supported administrator/installer networking |
| `ClusterManaged` | `UserManaged` | DNS-only SNO; the feature-enabled new-install default |
| `ClusterManaged` | `OpenShiftManagedDefault` | Explicit managed DNS and API/ingress VIPs |
| `UserManaged` | `OpenShiftManagedDefault` | Open design question: whether to support managed VIPs while infrastructure DNS remains administrator-owned |

The last row requires agreement on resolver and startup dependencies before its
validation and support policy are finalized. Separate fields do not establish
that all four combinations are supported, nor establish a permanent requirement
that managed LB must use managed DNS. External authoritative DNS for off-cluster
clients remains administrator-owned in either managed-DNS scenario.

The configuration flow is:

```text
Legacy platform configuration       Explicit shared networking intent
          |                                      |
    compatibility mapping                 validate and accept
          |                                      |
          +-------- effective networking --------+
                              |
                    MCO rendering and rollout
                              |
                    DNS / API and ingress VIPs
                              |
                   networking readiness
            (also consumed by future topology changes)
```

This is a logical contract, not a new operator or API object per stage.
Bootstrap supplies validated initial inputs before in-cluster controllers are
available. Acceptance starts reconciliation. Rollout and health checks determine
when it is complete; there is no accepted configuration waiting for a separate
activation signal.

### Workflow Description

The **cluster creator** selects installation policy. The **administrator**
prepares site networking, requests supported changes, and verifies API and
ingress access. The **Cluster Network Operator (CNO)** extends its existing
Infrastructure controller to validate new intent, publish accepted networking,
and report readiness. **MCO** owns node configuration and rollout. The
**cluster-config-operator (CCO) topology controller** consumes networking
readiness when needed by a topology transition; it does not enable or accept
networking on CNO's behalf.

#### New None SNO installation

1. The creator selects the applicable feature configuration and DNS policy. When
   the DNS feature is enabled, omission selects the new managed-DNS installation
   default; explicit opt-out selects the installer's legacy path.
2. The installer resolves that choice into explicit initial configuration and
   supplies consistent bootstrap inputs. It does not default absent policy on
   existing clusters through a global API default.
3. With DNS-only selected, MCO renders the 3609 node-address DNS path and
   resolver assets, without VIPs, keepalived, or HAProxy. Assisted/ABI does not
   generate a competing dnsmasq configuration for this path.
4. With opt-out, standard None retains external DNS requirements and
   Assisted/ABI retains its applicable conditional legacy behavior.
5. In-cluster reconciliation continues the resolved bootstrap policy.

The installer adds `networking.infrastructure`, separate from `platform` and
pod/Service networking. Its `dns` and `loadBalancer` policies map to
`Infrastructure.spec.networking`. These install-config excerpts show the
feature-enabled DNS-only default and an explicit managed-LB selection:

```yaml
platform:
  none: {}
networking:
  infrastructure:
    dns:
      management: ClusterManaged
    loadBalancer:
      type: UserManaged
```

```yaml
platform:
  none: {}
networking:
  machineNetwork:
    - cidr: 192.0.2.0/24
  infrastructure:
    dns:
      management: ClusterManaged
    loadBalancer:
      type: OpenShiftManagedDefault
      openShiftManagedDefault:
        apiServerInternalIPs:
          - 192.0.2.10
        ingressIPs:
          - 192.0.2.11
```

For managed LB, the installer populates the Infrastructure configuration's
`machineNetworks` from the existing `networking.machineNetwork[].cidr` field;
the install-config does not duplicate that list. This explicit selection renders
the managed-LB assets as well as DNS. To opt out of managed DNS, the creator
sets `dns.management: UserManaged` and `loadBalancer.type: UserManaged`. When an
explicit `networking.infrastructure` object is supplied, both policy selectors
are required. Omission and feature-gate defaults are defined below.

#### Existing None cluster: managed-LB opt-in

1. The administrator verifies an eligible starting state, identifies any
   required legacy DNS handoff, reserves dedicated API/ingress VIPs, checks for
   competing claimants, and establishes console access and a
   maintenance/recovery window.
2. The administrator applies complete `spec.networking` configuration selecting
   `OpenShiftManagedDefault` and its VIP/network inputs. CRD schema and CEL
   validate field structure, configuration relationships, and update rules. CNO
   checks legacy DNS ownership and handoff eligibility, conflicting
   configuration owners, and consumer compatibility before accepting the
   request.
3. CNO publishes the accepted configuration and starts reconciliation. A valid
   SNO request does not wait for additional nodes, three-member etcd, or a
   topology change. Unsupported requests report rejection and retain the last
   accepted networking rather than becoming dormant prepared configuration.
4. MCO renders the requested stack. Startup uses the existing API access path;
   DNS must not switch to an unserved VIP that the LB itself needs to start.
   VIPs become available and managed DNS adopts their addresses as part of the
   automatic rollout. The existing administrator access path is retained until
   API and ingress access through the new VIPs is verified.
5. CNO reports completion using `status.networking.conditions[type=Ready]` after
   matching MCO rollout and DNS/API/ingress health checks. The administrator
   also verifies external-client access and MCS reachability and certificates
   before changing external DNS or retiring the old access path.

Platform identity and machine-provisioning ownership remain unchanged. Applying
the complete request is the deployment trigger; acceptance and rollout are
asynchronous, not a second administrator activation step.

#### Future mutable-topology integration

The shared networking object remains unchanged as a None SNO grows to a
three-node compact cluster. For a managed-LB workflow, enable and verify the LB
while still SNO, then add nodes using the working API VIP. Runtime components
discover the additional backends without changing ownership or replacing the
VIPs. Node-address DNS-only remains a SNO mode; a growth workflow must establish
suitable managed or administrator-provided API and ingress access first.

The topology controller retains its own admission checks: added nodes must be
Ready and etcd must have the healthy voting membership required by the target
topology. These are prerequisites for the topology change, not for enabling LB
on SNO. Transition completion waits for topology-related MCO rollout and working
API and ingress access; when managed networking is selected, its current
`Ready=True` result is part of that check. A prior SNO readiness result alone
does not prove the expanded cluster is healthy.

An administrator-managed transition continues to use its existing networking and
does not wait for unrequested managed VIPs. Neither desired topology alone nor
node addition changes networking ownership. This integration defines how the
networking capability can serve the separate mutable-topology work; it does not
implement that transition or qualify new topologies by itself.

#### Legacy configuration and DNS adoption

An upgrade does not automatically migrate a cluster to this feature. Existing
on-prem settings are interpreted without automatically rewriting user spec.
Adoption of the shared API is distinct from changing active networking and is
allowed only for mappings with reviewed ownership and coexistence rules.

For Assisted DNS, omitting a manifest on a new install does not remove an
already-installed MachineConfig. MCO and Assisted must define handoff of the
generated dnsmasq configuration, service, dispatcher, and resolver files.
Unknown customizations require an eligibility decision, not deletion of
arbitrary user MachineConfigs. Image-based installations require their own
inventory and handoff review; Assisted/ABI findings are not sufficient evidence.

#### Rejection and partial failure

Reject requests that violate schema, configuration relationships, or permitted
update rules at admission. Requests rejected by CNO's cluster-dependent checks
remain persisted and correctable or withdrawable; the last accepted
configuration is retained. Once accepted, configuration is reconciled until
rollout succeeds or an actionable failure is reported. Reconciliation repairs
the accepted state rather than silently reversing ownership. Admission rejects
removal of accepted configuration or a change from managed to user-managed
ownership, including deletion of a parent object. A rejected, never-accepted
request remains correctable or withdrawable. Disabling a gate is not a teardown
or recovery mechanism. Networking rollout and topology progress are reported
separately.

### API Extensions

Extend `config.openshift.io/v1` Infrastructure named `cluster` with
`spec.networking` and `status.networking`, outside `platformSpec` and
`platformStatus`. This proposal uses the following types and serialized fields.

#### Desired configuration

`spec.networking` is an optional `InfrastructureNetworkingSpec`. When present,
both `dns` and `loadBalancer` are required; an empty object is invalid.

```yaml
spec:
  # Optional InfrastructureNetworkingSpec; dns and loadBalancer are required when set.
  networking:
    dns: # InfrastructureDNS
      # Required: ClusterManaged (MCO-owned) or UserManaged (administrator/installer-owned).
      management: ClusterManaged
    loadBalancer: # InfrastructureLoadBalancer discriminated union
      # Required PlatformLoadBalancerType: UserManaged or OpenShiftManagedDefault.
      type: OpenShiftManagedDefault
      # Required with OpenShiftManagedDefault; forbidden with UserManaged.
      openShiftManagedDefault: # OpenShiftManagedDefaultNetworking
        # Required atomic []IP: dedicated VIPs serving both api and api-int.
        apiServerInternalIPs: [192.0.2.10]
        # Required atomic []IP: dedicated ingress VIPs.
        ingressIPs: [192.0.2.11]
        # Required atomic []CIDR: machine networks containing the VIPs.
        machineNetworks: [192.0.2.0/24]
```

Each VIP list contains one or two addresses, with at most one address per IP
family. API and ingress lists use matching families and order. Machine networks
contain one to 32 unique CIDRs. Validation checks address syntax, family
consistency, and VIP membership in the configured networks. Site administrators
reserve the addresses and verify that they are unused; API validation cannot
establish exclusive ownership on the physical network.

The new managed-LB path uses keepalived/HAProxy. Existing BGP modes and legacy
VIP mutation rules are unaffected. DNS-only needs neither VIPs nor
machine-network inputs. Discovered node addresses are runtime observations, not
reserved VIPs stored in desired configuration.

`UserManaged` has the same ownership meaning in both selectors: OpenShift does
not manage that infrastructure service through this API. On SNO, a user-managed
LB selection does not require an external appliance; clients can use the node
address. DNS Operator-managed pod and Service DNS is unaffected. Existing
`DNSRecordsType` controls record generation, not ownership of the DNS service
and resolver configuration, so it is not an alias for `dns.management`.

#### Accepted configuration and completion

CNO owns `status.networking`. MCO consumes the accepted configuration as soon as
it is published; there is no separate `active` snapshot or topology activation
flag.

Admission, CNO acceptance, and runtime readiness have separate responsibilities:

| Stage | Checks | Outcome |
| --- | --- | --- |
| Admission | Required fields, enums/unions, list bounds, IP/CIDR syntax and relationships, and supported-mode and lifecycle rules expressible from the Infrastructure object | Invalid writes are rejected before persistence; admission alone does not publish a configuration for MCO |
| CNO acceptance | Recognized legacy DNS assets and ownership, a supported handoff without competing configuration owners, and participating consumers' compatibility with the selected mode | Passing prerequisites publishes the accepted configuration; a rejected candidate leaves the previous accepted configuration intact |
| Runtime readiness | MCO rendering and rollout, followed by health checks for the selected DNS, API, and ingress paths | `Ready=True` reports successful rollout and health verification; progress or failure is reported separately from configuration validity |

CNO inspects other cluster resources, including installed DNS MachineConfigs and
consumer compatibility observations, that Infrastructure schema/CEL rules cannot
inspect. For an adoption path, CNO verifies that the installed assets have a
recognized owner and supported handoff before publishing a new render target. It
does not require the old DNS service to be removed before acceptance; the
handoff preserves working access through rollout.

`Accepted` establishes eligibility to apply the configuration, not that the
resulting networking already works. New VIPs do not need to respond before
acceptance or activation. Once accepted, temporary consumer unavailability,
rollout failure, or failed network health checks are reported through `Ready`;
they do not retroactively make the request schema-invalid. Neither admission nor
CNO acceptance proves physical-network VIP reservation or external-client
reachability; those remain administrator preparation and verification steps.

```yaml
status:
  # CNO-owned status, illustrated with a DNS-only SNO configuration.
  networking:
    # Optional AcceptedInfrastructureNetworking; absent before first acceptance.
    accepted:
      generation: 12 # int64: source generation of the last accepted configuration.
      # Validated InfrastructureNetworkingSpec consumed by MCO; same shape as spec.networking.
      configuration:
        dns:
          management: ClusterManaged
        loadBalancer:
          type: UserManaged
    # InfrastructureNetworkingEndpoints: resolved addresses, not health or backend membership.
    endpoints:
      apiServerInternalIPs: [192.0.2.20] # Atomic []IP shared by api and api-int.
      ingressIPs: [192.0.2.20] # Atomic []IP for default ingress application names.
    # []metav1.Condition, map keyed by type: Accepted and Ready.
    conditions:
      - type: Accepted
        status: "True"
        observedGeneration: 12
        reason: ValidConfiguration
        message: Networking configuration accepted for reconciliation.
        lastTransitionTime: "2026-11-09T12:00:00Z"
      - type: Ready
        status: "False"
        observedGeneration: 12
        reason: RolloutInProgress
        message: Waiting for MCO to apply the accepted networking configuration.
        lastTransitionTime: "2026-11-09T12:00:00Z"
```

For DNS-only SNO, resolved targets are discovered node addresses. For managed
LB, they are the configured VIPs. Publishing these targets is not a health
assertion. The None/on-prem API does not introduce separate public and internal
API VIP lists: both names use `apiServerInternalIPs`.

Existing cloud DNS has separate provider-discovered API, internal-API, and
ingress addresses. Those fields stay on their existing paths until a cloud
mapping is designed. A later mapping must preserve that distinction and its
larger address lists, rather than force cloud LB addresses into the on-prem
managed-LB branch or label a cloud-owned LB `UserManaged`.

CNO reports:

- `Accepted=True` when the admitted configuration has passed CNO's
  cluster-dependent prerequisite checks and has been published for
  reconciliation. `Accepted=False` reports a rejected candidate and leaves the
  previous accepted configuration intact; its reason and message identify the
  blocking ownership, handoff, or compatibility requirement.
- `Ready=False` while rendering, rollout, or health verification is incomplete,
  or when the current candidate is rejected.
- `Ready=True` when MCO has applied the accepted configuration on all affected
  nodes and the selected DNS and managed API/ingress paths pass health checks.
  `Ready` is updated when that health changes.

Success requires `Accepted=True` and `Ready=True`, with both conditions'
`observedGeneration` and `accepted.generation` matching the current
Infrastructure generation. A condition for an older generation is not success
for a new request. A generation change unrelated to networking can reuse the
same rendered configuration after re-evaluation; it need not force a node
rollout. Existing MCO/MachineConfigPool status from a previous configuration is
not sufficient evidence for a changed networking request.

CNO correlates MCO's rendered configuration and rollout with the accepted
policy, then checks managed infrastructure DNS resolution/forwarding, API
reachability through the VIP using the expected hostname and CA, and ingress
routing through the configured ingress VIP. Checks apply only to the services
selected for cluster management. The reporting integration must distinguish
rollout failure from API/ingress access failure. It does not certify off-cluster
client paths or external authoritative DNS; those remain administrator
verification steps.

#### Infrastructure YAML

##### Example 1: DNS-only None SNO

The installer writes the feature-enabled new-install default explicitly. The
same policy can be selected on an existing eligible cluster after its legacy DNS
handoff.

```yaml
apiVersion: config.openshift.io/v1
kind: Infrastructure
metadata:
  name: cluster
spec:
  platformSpec:
    type: None
  networking:
    dns:
      management: ClusterManaged
    loadBalancer:
      type: UserManaged
```

MCO renders CoreDNS and resolver assets using node discovery, without keepalived
or HAProxy.

##### Example 2: Explicit installation-time DNS opt-out

```yaml
apiVersion: config.openshift.io/v1
kind: Infrastructure
metadata:
  name: cluster
spec:
  platformSpec:
    type: None
  networking:
    dns:
      management: UserManaged
    loadBalancer:
      type: UserManaged
```

Standard None retains its external DNS requirements; Assisted/ABI retains its
applicable conditional legacy DNS behavior. This policy selects installation
behavior; it does not remove an already-active managed resolver.

##### Example 3: Request managed endpoints on an existing None SNO

Applying this configuration requests deployment immediately. No topology field
changes and no separate activation step are needed.

```yaml
apiVersion: config.openshift.io/v1
kind: Infrastructure
metadata:
  name: cluster
spec:
  platformSpec:
    type: None
  networking:
    dns:
      management: ClusterManaged
    loadBalancer:
      type: OpenShiftManagedDefault
      openShiftManagedDefault:
        apiServerInternalIPs:
          - 192.0.2.10
        ingressIPs:
          - 192.0.2.11
        machineNetworks:
          - 192.0.2.0/24
```

The administrator keeps the current access path available during rollout.
Acceptance starts the work; `Ready=True` indicates that rollout and verification
of DNS, API access through the API VIP, and routing through the ingress VIP have
succeeded.

##### Example 4: Reconciliation in progress

This controller-owned status excerpt illustrates an accepted request at
generation 12 while MCO is deploying it. There is one accepted configuration,
not different prepared and active policies.

```yaml
status:
  platformStatus:
    type: None
  networking:
    accepted:
      generation: 12
      configuration:
        dns:
          management: ClusterManaged
        loadBalancer:
          type: OpenShiftManagedDefault
          openShiftManagedDefault:
            apiServerInternalIPs:
              - 192.0.2.10
            ingressIPs:
              - 192.0.2.11
            machineNetworks:
              - 192.0.2.0/24
    endpoints:
      apiServerInternalIPs:
        - 192.0.2.10
      ingressIPs:
        - 192.0.2.11
    conditions:
      - type: Accepted
        status: "True"
        observedGeneration: 12
        reason: ValidConfiguration
        message: Networking configuration accepted for reconciliation.
        lastTransitionTime: "2026-11-08T12:00:00Z"
      - type: Ready
        status: "False"
        observedGeneration: 12
        reason: RolloutInProgress
        message: Waiting for MCO to apply the accepted networking configuration.
        lastTransitionTime: "2026-11-08T12:00:00Z"
```

After rollout and health verification, CNO sets `Ready=True` with reason
`NetworkingReady` for the same generation. Failure leaves `Ready=False` with the
failing stage and a diagnostic message. This status is also the networking
signal available to a future topology workflow.

#### Coexistence and authority

Use a compatibility interpretation of legacy inputs first; do not require
universal field migration to introduce the shared model. Preserve legacy field
types, defaults, feature gates, and supported mutability during coexistence.

- Legacy-only configuration remains authoritative for legacy-origin behavior.
- Shared-only configuration is accepted only for supported scenarios and when
  its consumers are ready.
- For a qualified legacy adoption path, CNO accepts a matching shared request
  and records it in `status.networking.accepted`. That acceptance transfers
  authority for the adopted networking group to the shared configuration.
- Conflicting representations block new acceptance and report the conflict; they
  do not silently select the newest write or tear down accepted services.
- If old readers require compatibility projections, give each projection one
  writer and a defined direction. Do not introduce a bidirectional sync loop.

Read-side normalization is the initial compatibility approach. CNO owns any
required shared-to-legacy status projection, and legacy writers must not compete
with it for an adopted group. The reader/writer inventory determines which
projections are necessary before each adoption path is enabled. Public-cloud
paths remain on their existing implementation until their mappings are defined.

For the new None path, accepted policy cannot be removed, including by deleting
`networking`, `dns`, `loadBalancer`, or its managed configuration branch.
User-managed ownership can move to managed ownership through a supported
adoption workflow; the reverse change is rejected. Accepted managed VIPs and
machine-network inputs are immutable in the initial delivery. Admission
validation uses the last accepted configuration as the protection boundary, so a
rejected first request can still be corrected or withdrawn. CNO's status updates
preserve that boundary during reconciliation and controller restarts.

These new-path rules do not replace existing on-prem primary/secondary VIP rules
or narrow already-supported mutations. Adopting those legacy paths needs an
equivalent mutability mapping. General active-DNS teardown, LB teardown, and
arbitrary VIP replacement require separate lifecycle designs.

#### Feature gating and defaults

The proposed feature gate is `UnifiedClusterManagedDNSAndLB`, initially exposed
in `DevPreviewNoUpgrade`. It controls the new shared API and its consumers; the
Infrastructure policy records durable administrator intent. DNS rendering reuses
the 3609 implementation. Clusters using the 3609 DNS feature without shared
configuration retain that behavior until explicit adoption; the new gate does
not become a second DNS owner.

| Context | Default or omission behavior |
| --- | --- |
| New eligible None SNO, shared feature enabled, installer policy omitted | Installer writes `ClusterManaged` DNS and `UserManaged` LB explicitly |
| New None SNO, explicit DNS opt-out | Installer writes `UserManaged` for both policies and retains its supported legacy DNS path |
| New install without the shared feature | Existing installer/3609 behavior; no shared API default is introduced |
| Existing cluster without `spec.networking` | Existing platform/installer/3609 policy remains authoritative; no upgrade adoption |
| Explicit `spec.networking` | Both policy selectors are required; there is no ambiguous partially defaulted object |
| Managed LB | Always an explicit `OpenShiftManagedDefault` selection with complete VIP/network inputs |

There is no global default that creates `spec.networking` on existing objects.
An upgrade, broad FeatureSet, or topology change alone is not an opt-in. Gate
removal is not an active-service teardown procedure. Feature-gate naming and
coordination with 3609 are part of API review; the 5.1 deliverable is design
agreement, not a commitment to an implementation release.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The initial implementation does not enable this feature for HyperShift/hosted
control planes.

The initial workflow applies to standalone clusters whose nodes host these
networking assets. Hosted control planes have a different management/guest
boundary and are not newly enabled. Shared API placement alone must not cause
guest or management nodes to render the standalone stack.

#### Standalone Clusters

The shared networking model is designed for standalone SNO and HA clusters. The
same API expresses DNS/LB ownership and managed API/ingress VIPs at installation
and afterward, independently of platform identity. Selecting managed networking
does not require a topology transition.

This EP scopes implementation and qualification to None SNO; that scope is not
an architectural restriction on HA or a separate SNO-only installation
interface. HA enablement uses the same configuration model, with qualification
of bootstrap, multi-node rollout, VIP failover, and any existing-networking
handoff. The node-address DNS-only default remains specific to SNO; HA requires
appropriate stable API and ingress addresses.

Existing supported standalone networking is preserved without automatic
shared-API migration. Future topology workflows retain networking ownership and
configured VIPs while topology controllers manage node, etcd, and topology
lifecycle. The networking contract supplies API/ingress behavior and readiness;
topology-transition orchestration remains outside this EP.

#### Single-node Deployments or MicroShift

SNO defaults to the DNS-only path when the feature is enabled. An explicit
managed-LB selection adds keepalived, HAProxy, and their supporting containers.
The default therefore avoids LB overhead for installations that do not request
it. Qualification checks the combined footprint and reboot behavior on supported
SNO hardware. Cluster-component containers must declare CPU and memory requests
and should not set limits under [resource conventions][resource-conventions].

SNO rollout or reboot can interrupt the only control plane and workloads. A VIP
on one node provides no node failover. The OpenShift runtimecfg guard that
currently blocks some single-backend updates is addressed below; it is not a
general limitation of keepalived.

MicroShift does not use this Infrastructure/MCO contract; no configuration
change is included. Image-based SNO installation is a distinct research and
integration path, not an implicitly qualified transition source. The current
mutable-topology design excludes image-based installations.

#### OpenShift Kubernetes Engine

No OKE/OKD-specific API or reconciliation path is required. Feature availability
remains subject to the feature-gate, maturity, and topology qualification
requirements described in this proposal.

### Implementation Details/Notes/Constraints

#### API, acceptance, and topology integration

openshift/api defines the shared types, feature-gated schema, and admission
validation, including accepted-policy retention. Extend CNO's existing
[Infrastructure controller][cno-controller] beyond its platform filter to
reconcile explicitly configured shared networking. It owns the cluster-dependent
acceptance checks described above, accepted configuration, compatibility
mappings, and networking conditions. Its existing [VIP
synchronization][cno-sync] remains available to legacy platforms with their
current mutation rules.

MCO reports rendering and rollout for the accepted policy; CNO combines those
observations with networking health checks. CCO consumes that readiness for
future topology transitions and continues to own topology status. CCO here means
cluster-config-operator, not cloud-credential-operator. Each controller uses
field-scoped updates; networking ownership does not grant CNO ownership of all
Infrastructure status. No standalone acceptance operator or admission webhook is
introduced.

The compatibility inventory includes installer initialization, controller
writes, MCO/ControllerConfig conversion, certificate and API/ingress address
consumers, and legacy update paths. Keep platform fields for actual provider
integration; do not remove every platform check merely to call the design
platform agnostic.

#### MCO and baremetal-runtimecfg

MCO consumes accepted effective networking and selects DNS-only versus the
requested on-prem stack. The [current renderer][mco-render] excludes None from
on-prem paths and has a separate cloud alternate-DNS path. Adding None to one
platform list is insufficient: templates, resolver assets, API/ingress address
helpers, bootstrap, and certificates must use consistent decisions.

runtimecfg supplies init/sidecar commands inside independently rendered CoreDNS,
keepalived, and HAProxy pods; it is not a standalone service toggle. The 3609
DNS-only path supplies node discovery without VIPs. Its address and
upstream-resolver refresh contract must be agreed with its SME.

The runtimecfg [configuration-update guard][runtime-guard] rejects fewer than
two backends only when unicast is enabled and `IS_BOOTSTRAP=no`. Update that
guard to allow the explicitly selected single-node managed-LB mode while
preserving bootstrap synchronization and rejecting an empty backend set. Test
both stable single-node operation and subsequent backend growth. This is an
OpenShift integration change, not a change to keepalived's ability to run on a
single node. Rendering and backend discovery must continue to work while added
nodes exist but the observed topology is still SingleReplica.

The current [HAProxy startup check][haproxy-startup] accesses `api-int` before
starting the proxy. For post-installation enablement, use the reachable existing
or local API path to initialize the LB, and update managed DNS to VIP targets
only after API and ingress traffic is served through those VIPs. The rollout
performs this ordering automatically; publishing new DNS answers must not make
startup depend on the inactive LB. HAProxy handles the API path; MCS port 22623
and its certificates are verified separately. Ingress uses its VIP and router
path, not an assumed HAProxy backend.

#### Installer, Assisted/ABI, and image-based installation

The installer maps `networking.infrastructure` and existing machine-network
inputs to the shared Infrastructure configuration. It validates the selected
installation mode and seeds matching accepted bootstrap configuration before CNO
is available. In-cluster CNO takes over validation and status ownership without
changing policy. Bootstrap cannot depend on an in-cluster controller that
requires the DNS being configured. Assisted/ABI selects or omits its conditional
DNS assets from the same effective policy. Its service database is not a Day-2
Infrastructure API.

MCO and Assisted jointly design installed-asset adoption. Image-based-install
maintainers must identify their own resolver/address state and mutation
constraints before an adoption path is claimed. No provisioning-controller
enablement is added as a consequence of networking selection.

#### Platform agnosticism, External, and cloud DNS

The [External platform
design](cloud-integration/infrastructure-external-platform-type.md) separates
provider integration from built-in platform implementations. External is not
simply a renamed None: its integration configuration, including external CCM
behavior, must remain intact. New External networking and existing None adoption
are distinct workflows that can use the same networking contract. Neither
requires this enhancement to implement platform migration or approve None
deprecation.

The [on-prem External networking proposal][external-networking] introduces the
same ownership and VIP concepts. This design places those inputs under shared
`spec.networking` and their observations under `status.networking`, rather than
adding another copy to `ExternalPlatformSpec` or `ExternalPlatformStatus`.
External retains its provider and CCM settings under its platform fields. Its
installer uses `networking.infrastructure` for on-prem networking, with explicit
policies and no implied adoption of the None SNO default. External and None
share the API and rendering contract; platform qualification and any existing
field compatibility remain work for the respective implementation owners.

[PR #1666][prior-proposal] explored DNS/LB on None and External with duplicated
platform fields and considered moving networking out of platform configuration
as an alternative. This proposal chooses shared configuration and treats its
compatibility cost explicitly; the earlier unmerged proposal is prior art, not
an approved contract.

The existing [cloud DNS API][cloud-api] retains cloud-provided load balancers
when DNS is cluster-hosted, with separate API, internal-API, and ingress address
lists populated after LB configuration. The [cloud MCO path][cloud-mco] uses
different address arguments and [resolver setup][cloud-resolver] from the
on-prem path. This is not a field-location-only migration. Preserve provider
bootstrap/address discovery and resolver behavior; do not require user-reserved
VIPs, keepalived, or on-prem resolver mechanics for cloud DNS. Cloud convergence
can follow reviewed mappings without blocking the initial None workflow.

### Risks and Mitigations

- **Configuration divergence:** one authority per adopted group, explicit
  conflict rejection, and no implicit legacy/new last-write-wins behavior.
- **Upgrade changes networking:** preserve absence/default semantics and test
  legacy render equivalence before changing consumers.
- **Startup or topology dependency deadlock:** initialize the LB through a
  working API path, verify API/ingress access through the VIPs before DNS
  cutover, and establish usable networking before future node joining. A SNO LB
  request must not wait for the node/etcd prerequisites of a later topology
  transition.
- **Resolver conflicts or bootstrap failure:** single resolver ownership,
  explicit legacy handoff, and distinct cloud/bootstrap integration tests.
- **VIP conflicts and exposure:** site administrators reserve addresses and
  verify reachability; schema validation does not prove an address is unused.
- **Access loss:** retain a recovery path, require a maintenance window where
  disruptive, and verify API/ingress access before external DNS cutover.
- **Scope/maintenance expansion:** qualify each new platform/mode explicitly; a
  general API is not universal enablement or an unnamed maintenance owner.

Networking and MCO reviewers assess host networking, privileges, mounts,
resolver ownership, and API/ingress network exposure. API, installer, and
platform owners review defaulting, compatibility, and error UX. Retain existing
API/MCS TLS and certificate behavior; no independent TLS policy or new
admission-service dependency is proposed.

### Drawbacks

Shared configuration creates migration and compatibility work beyond a narrow
None-only union member. Old and new representations can coexist for a long
period, increasing testing and support costs. Cloud bootstrap differences limit
immediate implementation reuse even where user-facing concepts are shared.

Offering managed networking on administrator-provisioned infrastructure also
changes expectations. Clear ownership boundaries and named maintainers are
necessary; reusing existing binaries does not itself establish support.

## Alternatives (Not Implemented)

### Add None and External platform-specific networking fields

This delivers a smaller initial schema change but repeats the existing
duplication and makes each later platform a new API migration. The shared
contract is preferred, with staged implementation to bound its cost.

### Migrate None clusters to External or BareMetal first

Platform migration is a separate lifecycle problem and is not a prerequisite for
existing clusters to select networking. Installing new clusters on a different
platform does not solve existing None adoption. Networking selection must not
implicitly select provisioning or CCM behavior.

### Replace all platform networking implementations at once

On-prem fields share substantial structure, but cloud LB address discovery and
resolver/bootstrap behavior differ. A universal rewrite would enlarge risk and
delay the initial use case. Preserve legacy paths and converge only with
reviewed semantic mappings and evidence.

### Keep a feature gate or user MachineConfig as the configuration API

A gate controls availability, and a MachineConfig changes node files; neither
provides a durable cross-component ownership and compatibility contract. Reuse
MCO rather than introduce a separate LB operator.

### Couple managed-LB activation to topology changes

Accepting VIP configuration but withholding deployment until a topology change
adds another activation state and can create circular readiness dependencies.
Instead, a complete supported request starts deployment immediately. SNO can
establish stable managed API and ingress VIPs before a later topology
transition, while DNS-only remains the default for clusters that do not request
managed LB.

## Open Questions

1. Should `UserManaged` infrastructure DNS with `OpenShiftManagedDefault` LB be
   supported initially? Networking and MCO owners must resolve its resolver and
   startup dependencies before validation encodes either outcome. This is
   separate from administrator ownership of external authoritative DNS.
2. Can CNO obtain all required per-node DNS, rendered-configuration, and
   API/ingress health observations through existing interfaces, or does
   MCO/runtimecfg need additional reporting for the proposed `Ready` condition?
3. Which installed Assisted/ABI DNS configurations have a supported handoff, and
   what distinct image-based-install cases can be adopted safely? The owners
   must define recognition, asset ownership, and recovery for each case.
4. Which legacy platform integrations migrate in each release of the proposed
   three-release adoption window? Which readers need compatibility projections,
   and which cloud-specific fields must remain until separate mappings exist?
5. How should the shared feature gate be coordinated with 3609's DNS gate and
   node-address/upstream refresh implementation? The proposed new-install
   default and preservation of existing gate-only clusters remain requirements.
6. Which address families and installation methods are qualified first, and when
   can External and mutable-topology integration use the shared implementation?

## Test Plan

Use source/rendering fixtures before disruptive integration tests. Cover legacy
and shared inputs, absence and defaults, matching/conflicting representations,
rejected-input correction, optional-parent deletion, and concurrent writers.
Prove supported legacy VIP mutations and distinct cloud API/internal-API address
lists survive normalization. Unknown cloud LB addresses must not become
fabricated VIPs or a request to disable an active resolver.

Test new None DNS-only installation, explicit opt-out, existing gate-only DNS,
and each advertised legacy adoption path. Assert no LB assets without explicit
opt-in. Test installation-time and post-installation LB selection on unchanged
SNO, single-backend updates, and `Accepted`/`Ready` generation handling. Verify
that rejected candidates cannot change rendered networking, and accepted policy
cannot be removed through a parent deletion. Test address/upstream refresh,
startup ordering, restart/reboot recovery, and resource cost.

Test the three stages independently: admission rejects invalid configuration; an
admitted request with an unsupported legacy DNS owner or incompatible consumer
does not replace the accepted snapshot; an accepted request remains
`Ready=False` while rollout or health checks fail. Verify that acceptance never
depends on a new VIP responding before its serving components are deployed.

Future mutable-topology integration verifies managed networking before node
addition, continued API/ingress and MCS access during backend growth, and final
MCO rollout and API/ingress health before transition completion. Test that LB
activation does not depend on future node or etcd readiness, and that topology
changes do not implicitly opt in to managed networking. Preserve the
administrator-managed path when selected. Platform and provisioning
configuration remain unchanged; topology changes through its designated
controller.

Include interrupted upgrades, mixed consumers, and legacy-render non-regression
for each affected platform. Test any compatibility projection against old
readers. Image-based transitions, External enablement, and new cloud behavior
require their own agreed qualification rather than implied coverage here. No
POC, conformance, or payload results are claimed by this proposal.

## Graduation Criteria

### Entering Dev Preview

- API and compatibility semantics, CNO/MCO responsibilities, and the DNS/LB
  combination matrix are reviewed, with unsupported combinations rejected.
- DNS-only bootstrap/default/opt-out and legacy preservation have evidence.
- The explicit None SNO managed-LB workflow works end to end at installation and
  after installation, with single-backend and recovery tests.
- Operators expose rejection, rollout, and API/ingress access failures
  distinctly.

### Dev Preview -> Tech Preview

- Advertised starting states and address families have integration coverage.
- Old/new coexistence, partial rollout, rejected requests, and recovery are
  tested; participating consumer versions are specified.
- Resource and disruption measurements are available, with diagnostic and
  support ownership agreed.
- User-facing installation, adoption, verification, and recovery documentation
  is available in openshift-docs.

### Tech Preview -> GA

- API semantics, support matrix, defaults, and upgrade behavior are stable.
- End-to-end, conformance, upgrade, and disruption tests cover supported
  DNS-only and managed-LB workflows; additional topology integrations are
  qualified before they are advertised.
- Feature graduation preserves explicit policy and legacy behavior.
- User feedback, support procedures, and resource expectations are addressed.

### Removing a deprecated feature

Plan a three-minor-release adoption window beginning when the shared feature
reaches GA. During that window, migrate supported producers and consumers,
provide adoption documentation, and assess remaining legacy usage. New platform
integrations use the shared schema rather than add more platform-specific
networking fields.

The end of that window is a compatibility review milestone, not automatic field
deletion. Deprecation or removal requires an agreed migration path, consumer
inventory, and compliance with API compatibility policy. Legacy platforms whose
semantics have not been mapped retain their supported behavior. None deprecation
and platform migration are outside this enhancement.

## Upgrade / Downgrade Strategy

Introduce legacy-compatible interpretation before enabling new writers and
workflows. Upgrades preserve effective configuration when shared policy is
absent; no automatic takeover, spec rewrite, or DNS default is applied to an
existing cluster. New acceptance is withheld until participating consumers can
interpret the selected mode. Establish this eligibility mechanism before
implementation rather than assuming operator rollout order makes it safe.

Explicit API adoption preserves service behavior unless a separate supported
networking change is requested. Legacy projections, if needed, must remain
consistent through retries and skew. Existing cloud resolver/bootstrap paths are
not replaced just because the shared API becomes available.

Removing fields, disabling a gate, or changing platform is not rollback.
Recovery repairs the last accepted configuration and the responsible rollout or
transition. This design adds no customer payload-downgrade guarantee and does
not override preview FeatureSet upgrade restrictions.

## Version Skew Strategy

Inventory installer/bootstrap, Infrastructure writers, MCO, certificate and
API/ingress address consumers, runtimecfg, and topology consumers. Test old
readers with legacy inputs and any proposed projections, and new readers with
old writers. Do not accept modes that an older active consumer would
misinterpret.

Unknown/missing policy must not activate new defaults or remove active services.
Controllers preserve field ownership and accepted state through restarts and
conflicts. A shared logical model does not require every component to upgrade
atomically; the coexistence policy must work throughout the supported skew.

## Operational Aspects of API Extensions

Configuration remains a cluster singleton with infrequent writes. No per-node
API fleet, aggregated API server, or network-dependent admission service is
introduced. Report the candidate generation, accepted configuration authority,
and actionable rejection reasons through `status.networking`. `Accepted`
identifies the policy being reconciled; `Ready` reports matching rollout and
health checks. MCO/MachineConfigPool status explains node rollout, while
topology status reports the separate transition. A stale condition generation
cannot prove completion of a new request.

An acceptance-controller outage blocks new acceptance but does not remove
rendered services. A rollout, DNS, or VIP failure can still interrupt API
access, image pulls, and workloads. Networking, MCO, topology, and affected
installer/platform teams share escalation according to the failed stage.

QE and component owners measure acceptance/rollout duration, DNS resolution,
API/ingress availability, disruption, and resource consumption. Metric names and
alert thresholds require implementation review; customer addresses must not be
used as telemetry labels.

## Support Procedures

Establish the authoritative legacy/shared configuration and the last accepted
policy before changing anything. Inspect Infrastructure, feature configuration,
`Accepted` and `Ready` conditions and their generations, MCO conditions, and
MachineConfigPool state. Inspect topology progress separately when a transition
is underway.

- **Conflicting or rejected input:** correct the unaccepted request or resolve
  its source conflict. Do not clear active fields to force reconciliation.
- **Accepted but not ready:** inspect whether MCO is deploying the accepted
  configuration or whether DNS/API/ingress checks are failing. No topology
  status change or second activation command is needed to start a valid SNO
  request.
- **DNS failure:** inspect the selected implementation, resolver owner, port 53,
  node/provider addresses, upstreams, manifests, image availability, and logs.
  Use the installation-specific handoff procedure for legacy assets.
- **API/ingress access failure:** inspect VIP ownership and API/ingress paths,
  test API hostname/CA, and check MCS port 22623 and certificates independently.
- **Partial rollout/transition:** repair the failed stage and reconcile from
  accepted state; follow the topology design for node/etcd failure handling. No
  automatic networking reversal or topology recovery is promised here.
- **Disablement request:** installation opt-out is not active-DNS teardown.
  Deleting policy or disabling gates is not a supported removal procedure.

Retain console access for failures that prevent API-based repair. Existing
workloads may be interrupted by node reboot or API/ingress/DNS failure; new
workloads may fail to resolve names or pull images. Reconciliation resumes from
accepted state when controllers recover, without selecting new ownership.

## Infrastructure Needed

Disposable standard and Assisted/ABI None SNO environments, a compatible
multi-node transition lab, reserved VIPs, external DNS, and console recovery.
Use existing on-prem/cloud render fixtures for compatibility tests; obtain
platform-specific bootstrap and image-based evidence before claiming adoption
support. Lab execution, code changes, and payload qualification are separate
work, not results of this design document.

[sno-dns-design]:
    https://github.com/openshift/enhancements/blob/f78b50340589564f8998ed13186d1d1c38abf15b/enhancements/single-node/sno-none-platform-internal-dns.md
[prior-proposal]: https://github.com/openshift/enhancements/pull/1666
[external-networking]: https://github.com/openshift/enhancements/pull/2130
[mco-render]:
    https://github.com/openshift/machine-config-operator/blob/3f863f437e3af4d975300927b3ca7e2bf7dbbb29/pkg/controller/template/render.go
[cno-controller]:
    https://github.com/openshift/cluster-network-operator/blob/069ef080db6b20f6704e4dc0c07d48937cef0ef8/pkg/controller/infrastructureconfig/infrastructureconfig_controller.go
[cno-sync]:
    https://github.com/openshift/cluster-network-operator/blob/069ef080db6b20f6704e4dc0c07d48937cef0ef8/pkg/controller/infrastructureconfig/sync_vips.go
[runtime-guard]:
    https://github.com/openshift/baremetal-runtimecfg/blob/3057d9978db74dcd5012a293d37d571b524c4e46/pkg/monitor/dynkeepalived.go#L133-L145
[haproxy-startup]:
    https://github.com/openshift/machine-config-operator/blob/3f863f437e3af4d975300927b3ca7e2bf7dbbb29/templates/master/00-master/on-prem/files/haproxy.yaml
[cloud-api]:
    https://github.com/openshift/api/blob/38053d30e7e728362c4c6df1e402f6f7eb37d299/config/v1/types_infrastructure.go#L929-L1000
[cloud-mco]:
    https://github.com/openshift/machine-config-operator/blob/3f863f437e3af4d975300927b3ca7e2bf7dbbb29/templates/common/cloud-platform-alt-dns/files/coredns.yaml
[cloud-resolver]:
    https://github.com/openshift/machine-config-operator/blob/3f863f437e3af4d975300927b3ca7e2bf7dbbb29/templates/common/cloud-platform-alt-dns/files/usr-local-bin-update-dns-server.yaml
[resource-conventions]:
    https://github.com/openshift/enhancements/blob/2cf7bf953506ba2a463848e59c8bc574e340406d/CONVENTIONS.md#resources-and-limits
