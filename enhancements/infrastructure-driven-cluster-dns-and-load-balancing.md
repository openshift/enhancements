---
title: infrastructure-driven-cluster-dns-and-load-balancing
authors:
  - "@eggfoobar"
reviewers:
  - "@joelspeed" # Infrastructure API and cluster-config-operator integration
  - "@mkowalski" # On-prem networking, cloud DNS, and CNO reconciliation
  - "@cybertron" # Runtime networking and legacy API compatibility
  - "@pmtk" # SNO DNS-only behavior and OCPSTRAT-3609 integration
  - "@sinnykumari" # MCO rendering and node rollout
  - "@zaneb" # Platform integration, bootstrap, and provisioning isolation
  - "@patrickdillon" # Installer configuration and bootstrap
  - "@jaypoulz"
  # Assisted/ABI and image-based-install reviewer coverage remains to be confirmed.
approvers:
  - "@jerpeter"
api-approvers:
  - "@joelspeed"
creation-date: 2026-09-04
last-updated: 2026-10-13
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

This enhancement separates infrastructure DNS and cluster-managed endpoint
configuration from platform identity through shared Infrastructure
configuration. It provides a common model for DNS-only platform None Single Node
OpenShift (SNO) and managed API and ingress endpoints for the multi-node outcome
of a topology transition. The Machine Config Operator (MCO) applies accepted
networking configuration using the existing infrastructure networking
components, without changing platform identity or enabling machine provisioning.
Existing platform-specific configuration remains compatible while the shared
model is introduced, allowing additional platforms to adopt it without repeating
the same configuration beneath each platform discriminator.

## Motivation

Platform identity currently selects several distinct concerns: provider
integration, machine provisioning, and infrastructure networking. Administrators
who manage their own machines may nevertheless need OpenShift-managed DNS or
endpoints. Selecting a different platform solely to obtain networking entangles
these concerns and does not provide a path for existing None clusters.

There are two immediate needs. First, SNO infrastructure DNS differs by
installation method: standard None can use external DNS, while Assisted
Installer and Agent-based Installer (ABI) conditionally generate an on-node
dnsmasq configuration. The [SNO DNS-only proposal][sno-dns-design] standardizes
this using MCO-managed CoreDNS and discovered node addresses, without VIPs or
load-balancer pods. Second, an existing None SNO that grows into a multi-node
cluster needs a choice of endpoint ownership for that outcome. An install-only
option cannot serve already-deployed clusters.

The second need does not justify a full load balancer on an unchanged SNO.
Ordinary SNO remains DNS-only when managed DNS is selected. Managed-LB
activation is coordinated with the topology transition; topology orchestration
itself remains in the [mutable-topology
enhancement](topologies/mutable-topology.md).

Adding another platform-specific networking object would repeat the duplication
already present in on-prem APIs. This proposal instead introduces shared intent
and an explicit compatibility boundary. Platform-agnostic configuration does not
mean platform-independent implementation: cloud-provided endpoints and on-prem
VIPs have different address sources, bootstrap dependencies, and resolver
integration.

Infrastructure DNS here is node-level resolution for cluster API, internal API,
and application names, not DNS Operator-managed pod and Service DNS.
Authoritative external DNS and resolution for off-cluster clients remain
separate responsibilities.

### User Stories

- As a None SNO cluster creator, I want consistent infrastructure DNS without
  reserving VIPs or running load-balancer components, so that my single-node
  installation has a small, predictable networking footprint.
- As a cluster creator, I want an explicit managed-DNS opt-out, so that I can
  retain my installation method's supported legacy DNS behavior.
- As an administrator of an existing None SNO, I want to prepare managed
  networking for a supported multi-node transition, so that the resulting
  cluster can own its endpoints without a platform migration or reinstall.
- As an existing on-prem administrator, I want my configuration and supported
  operations preserved during API evolution, so that an upgrade does not
  unexpectedly change DNS or endpoint ownership.
- As a platform integrator, I want reusable networking configuration, so that
  enabling another supported platform does not require duplicating the same DNS
  and LB API fields.
- As a fleet administrator, I want to distinguish configuration rejection,
  pending activation, rollout failure, and endpoint failure, so that I can
  recover a networking change without guessing which configuration is active.

### Goals

- Separate infrastructure DNS and endpoint ownership from platform identity.
- Provide DNS-only None SNO with a new-install managed-DNS default when its
  feature is enabled and an explicit installation opt-out.
- Provide an explicit managed-endpoint path for the None multi-node transition
  outcome, preserving the administrator-managed path when not selected.
- Preserve existing cluster behavior and supported platform-specific operations
  while introducing the shared API.
- Establish a networking/topology integration contract with clear acceptance,
  activation, rollout, and recovery responsibilities.
- Make additional platform adoption possible without requiring a new copy of the
  common configuration or replacing the initial API.

### Non-Goals

- Running the full managed-LB stack on an ordinary, unchanged SNO.
- Implementing node addition, etcd scaling, or topology-transition
  orchestration.
- Changing platform identity, deprecating None, or migrating None to External.
- Enabling machine provisioning, Ironic, Metal3, BMO, or CBO through networking
  configuration, or changing workload Service load-balancer behavior.
- Universal platform enablement, new cloud-LB implementations, or migration of
  every platform's DNS implementation in the first delivery.
- Generic fresh multi-node None enablement or External enablement without their
  own support, installer, and integration review.
- General active-DNS teardown, reverse LB ownership, arbitrary VIP replacement,
  address allocation, or authoritative external-DNS management.
- New BGP, arbiter, hosted-control-plane, or MicroShift support. Existing
  platform modes remain subject to compatibility preservation.

## Proposal

Extend the Infrastructure singleton with shared networking configuration outside
the platform-specific spec/status unions. Keep platform identity for provider
integration and retain provider-specific inputs where their semantics differ.
The shared model separates DNS policy, endpoint ownership, and resolved
addresses instead of exposing a single "enable baremetal-runtimecfg" switch.

Introduce a common interpretation of accepted configuration for consumers.
Legacy-only clusters continue to derive effective configuration from their
existing fields and defaults. Explicit shared configuration is used only after
validation and acceptance for a supported scenario. MCO renders from that
effective configuration, not from arbitrary unaccepted spec changes.

| Scenario                                    | DNS and endpoints                                                                         | Scope                                                       |
| ------------------------------------------- | ----------------------------------------------------------------------------------------- | ----------------------------------------------------------- |
| Ordinary None SNO                           | Managed DNS uses discovered node addresses; no managed LB or VIP requirement              | Initial new behavior, aligned with 3609                     |
| None SNO preparing to grow                  | Networking intent may be prepared; preparation alone does not start the LB                | Initial workflow; activation boundary requires joint review |
| Eligible None multi-node transition outcome | Explicit managed-LB intent selects dedicated VIPs and the on-prem keepalived/HAProxy path | Initial managed-endpoint use case                           |
| None without managed-networking selection   | Existing administrator/installer-owned behavior                                           | Preserved                                                   |
| Existing on-prem platforms                  | Existing defaults, VIP ownership, DNS behavior, and supported mutations                   | Compatibility requirement, not automatic adoption           |
| External                                    | Reuse the shared contract when enablement is separately agreed                            | Aligned extension, not a platform-migration prerequisite    |
| Cloud cluster-hosted DNS                    | Cloud LB ownership and discovered endpoints remain distinct from on-prem VIP management   | Preserve existing behavior; convergence is staged           |

The configuration flow is:

```text
Legacy platform configuration       Explicit shared networking intent
          |                                      |
    compatibility mapping                 validate and accept
          |                                      |
          +-------- effective networking --------+
                              |
                  activation eligibility
            (including topology integration)
                              |
                    MCO rendering and rollout
                              |
               DNS / eligible managed endpoints
```

This is a logical contract, not a new operator or API object per stage.
Bootstrap supplies validated initial inputs before in-cluster controllers are
available. Configuration acceptance, activation eligibility, completed rollout,
and healthy endpoints are distinct facts.

### Workflow Description

The **cluster creator** selects installation policy. The **administrator**
prepares site networking, requests supported changes, and verifies endpoints.
The **networking acceptance controller** validates new intent and preserves
accepted networking. The **topology controller** owns topology decisions under
its separate enhancement. **MCO** owns node configuration and rollout. The
placement of networking acceptance in CNO or cluster-config-operator (CCO)
remains an API/component ownership decision.

#### New None SNO installation

1. The creator selects the applicable feature configuration and DNS policy. When
   the DNS feature is enabled, omission selects the new managed-DNS installation
   default; explicit opt-out selects the installer's legacy path.
2. The installer resolves that choice into explicit initial configuration and
   supplies consistent bootstrap inputs. It does not default absent policy on
   existing clusters through a global API default.
3. In managed mode, MCO renders the 3609 node-address DNS path and resolver
   assets, without VIPs, keepalived, or HAProxy. Assisted/ABI does not generate
   a competing dnsmasq configuration for this path.
4. With opt-out, standard None retains external DNS requirements and
   Assisted/ABI retains its applicable conditional legacy behavior.
5. In-cluster reconciliation continues the resolved bootstrap policy.

DNS selection is exposed through normal installation configuration. The exact
installer field and bootstrap/status mapping are reviewed with the API and 3609
owners; a custom manifest is not the only intended opt-out interface.

#### Existing None cluster and topology-associated activation

1. The administrator verifies an eligible starting state, resolves any legacy
   DNS handoff, reserves dedicated API/ingress VIPs, checks for competing
   claimants, and establishes console access and a maintenance/recovery window.
2. The administrator prepares complete networking intent. Validation checks
   syntax, address families, networks, supported combinations, and lifecycle
   eligibility. An invalid candidate cannot change active DNS or endpoints.
3. The networking and topology controllers establish the reviewed activation
   boundary. Merely requesting a topology or setting VIP fields does not start
   the stack. Preparation while still SNO is distinct from activation.
4. Once eligible, the accepted active configuration drives MCO to use VIP-based
   DNS answers and the on-prem LB assets. Nodes reconcile through normal MCO
   rollout; this publication does not make file changes or cutover atomic.
5. Verify API access with the expected hostname and CA, ingress routes, DNS and
   upstream forwarding, and MCS reachability/certificates separately. External
   DNS cutover follows verified endpoint availability.
6. Networking readiness contributes to the transition completion contract. The
   resulting multi-node cluster retains platform None and its existing
   machine-provisioning ownership.

The precise interleaving is an open cross-enhancement decision, not a complete
runbook. The current mutable-topology design relies on administrator-provided
networking while nodes join and become ready. This design must either preserve
that path through cutover or explicitly revise the dependency ordering with the
topology owners. Waiting for readiness that itself needs the new endpoints would
deadlock; activating from desired topology alone would be premature. Any
necessary temporary single-backend LB state must be explicitly bounded and
validated, not treated as support for steady-state SNO load balancing.

#### Legacy configuration and DNS adoption

An upgrade with no shared-policy request preserves effective behavior. Existing
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

Reject schema-invalid requests at admission. Rejected persisted intent retains
the last accepted configuration and remains correctable or withdrawable.
Prepared but ineligible networking retains active DNS and endpoint ownership.
After activation, reconciliation repairs the accepted state instead of silently
reversing ownership. Removing fields or disabling a gate is not a DNS/LB
teardown or recovery mechanism. Transition and rollout failures expose their
separate progress and recovery responsibilities.

### API Extensions

This extends `config.openshift.io/v1` Infrastructure named `cluster`, with
shared desired and accepted/effective networking configuration outside
`platformSpec` and `platformStatus`. Exact field names, types, serialization,
and reporting fields remain API-review items. The contract and examples below
show the proposed semantics and one candidate serialization, not an existing or
applicable API schema.

| Concept                                  | Contract                                                                                                                                                               |
| ---------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| DNS policy                               | Select infrastructure DNS ownership independently of LB ownership; omission preserves legacy interpretation on existing clusters                                       |
| LB ownership                             | Express whether the on-prem endpoint stack is requested; retain existing ownership vocabulary where semantics match, without treating cloud-managed LB as user-managed |
| Requested addresses/networks             | Administrator-provided dedicated VIPs and validation inputs for the on-prem managed-LB path; not required for node-address DNS-only mode                               |
| Resolved endpoints                       | Distinguish API, internal API, and ingress; distinguish discovered node addresses, reserved VIPs, and provider-produced addresses                                      |
| Accepted policy and active configuration | Identify validated intent and the configuration authorized for rendering; represent pending eligibility without premature activation                                   |
| Runtime readiness                        | Report rollout and verified endpoint health separately from configuration acceptance                                                                                   |

Node discovery is not represented by storing a node IP as a pretend VIP.
Provider-discovered endpoint addresses are not copied into administrator intent
as though they were reserved VIPs. Unknown/not-yet-available addresses must not
be interpreted as a request to disable DNS. The shared model must preserve cloud
API/internal-API distinctions, multiple addresses, and private-cluster cases
without imposing the on-prem VIP-list cardinality globally.

DNS-only is valid without managed LB. The initial None managed-LB path uses
managed DNS and VIP-based answers, but that requirement is not imposed on every
existing platform combination. Existing `DNSRecordsType` controls record
generation and is not a complete DNS-service/resolver enable switch.

#### Illustrative Infrastructure YAML

The examples use `spec.networking` for desired policy and `status.networking`
for controller-owned acceptance and active configuration. Both are siblings of
the existing platform-specific fields, not members of a new `none` or `external`
union. These are proposed field names for API review; the examples are not
manifests to apply to an existing release. Unrelated Infrastructure fields are
omitted, and all addresses are documentation examples.

In this candidate shape:

- `dns.management: ClusterManaged` selects the MCO-managed infrastructure DNS
  path. `Unmanaged` opts out of that path at installation, retaining the
  installer's supported legacy behavior. It does not mean that every DNS service
  is absent or that a running resolver should be removed.
- `loadBalancer.type` reuses the existing `UserManaged` and
  `OpenShiftManagedDefault` vocabulary for these None/on-prem scenarios.
  Dedicated VIP and network inputs belong to `openShiftManagedDefault`, not to
  DNS policy or a globally required endpoint list.
- `accepted` retains the last validated policy and its source generation;
  `active` identifies the configuration authorized for MCO rendering and its
  resolved endpoints. Publishing `active` is not proof of completed rollout or
  endpoint health. Generation values refer to the source Infrastructure spec,
  not a count of node-address discoveries or status updates.

The acceptance owner, status structure, conditions, and representation of
legacy/shared authority remain review items. The examples illustrate the
prepared-versus-active distinction without choosing a topology activation signal
or assigning ownership of unrelated Infrastructure status.

##### Example 1: DNS-only None SNO

The installer resolves a new eligible SNO's managed-DNS selection to explicit
policy. The same policy represents an existing cluster only after any required
DNS adoption checks and legacy resolver handoff. No VIP or machine-network input
is needed for node-address DNS.

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

MCO renders CoreDNS and the corresponding resolver assets, using discovered node
addresses, without keepalived or HAProxy. In this SNO example, `UserManaged`
means no OpenShift-managed LB; it does not require an external LB appliance in
front of the single node. The node IP is not entered as a VIP.

##### Example 2: Explicit installation-time DNS opt-out

This is the Infrastructure policy produced from an explicit installer opt-out,
not a replacement for exposing that choice in normal installation configuration.

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
      management: Unmanaged
    loadBalancer:
      type: UserManaged
```

Standard None retains its external DNS requirements; Assisted/ABI retains its
applicable conditional legacy DNS behavior. DNS Operator-managed pod and Service
DNS is unchanged. Applying this choice to an already-active managed resolver is
not a supported teardown workflow. On an existing cluster, omitting `networking`
preserves legacy interpretation; omission must not be globally defaulted to
either of these explicit examples.

##### Example 3: Prepared managed endpoints, with DNS-only still active

The administrator reserves separate API and ingress VIPs for the multi-node
transition outcome and requests `OpenShiftManagedDefault`. Here the request has
been validated, but the reviewed activation prerequisites are not yet met. The
cluster is still SNO, with node address `192.0.2.20` and DNS-only active.

The spec is administrator intent. The generation and status below illustrate a
controller-observed response, not fields for an administrator to write.

```yaml
apiVersion: config.openshift.io/v1
kind: Infrastructure
metadata:
  name: cluster
  generation: 12
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
status:
  platformStatus:
    type: None
  networking:
    observedGeneration: 12
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
    active:
      generation: 11
      configuration:
        dns:
          management: ClusterManaged
        loadBalancer:
          type: UserManaged
      endpoints:
        api:
          - 192.0.2.20
        apiInternal:
          - 192.0.2.20
        ingress:
          - 192.0.2.20
```

MCO continues to consume `active`, not the prepared VIPs in spec or `accepted`.
It therefore retains node-address DNS and does not start keepalived/HAProxy.
Acceptance does not prove that the VIPs are unused, announce them, or change
external DNS. The administrator still verifies site-network prerequisites.

##### Example 4: Managed endpoints authorized for activation

After the jointly defined topology/networking activation boundary is satisfied,
the controller publishes the following active configuration for the request
above. This status-only excerpt omits the unchanged spec and accepted snapshot;
it is not an administrator patch or a topology-transition command.

```yaml
status:
  platformStatus:
    type: None
  networking:
    observedGeneration: 12
    active:
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
        api:
          - 192.0.2.10
        apiInternal:
          - 192.0.2.10
        ingress:
          - 192.0.2.11
```

This authorizes VIP-based DNS and the keepalived/HAProxy path for MCO rollout;
it does not assert that all nodes have applied it or that endpoints are healthy.
The transition's managed multi-node outcome requires those additional checks.
The exact activation boundary, including whether a bounded temporary
single-backend state is necessary, remains a cross-enhancement decision.
Platform identity remains None throughout.

API and internal-API addresses are equal for this on-prem example but remain
distinct in the resolved representation. A later cloud mapping must preserve
provider-owned load balancers, independently discovered address lists, and
private-cluster semantics; it must not manufacture `UserManaged` ownership or
populate `openShiftManagedDefault` from cloud endpoint observations. Likewise,
shared placement permits future External reuse without making a platform change
part of this workflow. These examples do not select a public endpoint-source
enum or enable those additional platform paths.

#### Coexistence and authority

Use a compatibility interpretation of legacy inputs first; do not require
universal field migration to introduce the shared model. Preserve legacy field
types, defaults, feature gates, and supported mutability during coexistence.

- Legacy-only configuration remains authoritative for legacy-origin behavior.
- Shared-only configuration is accepted only for supported scenarios and when
  its consumers are ready.
- Matching legacy/shared representations require an explicit adoption boundary
  and one authority per adopted configuration group, not two active writers.
- Conflicting representations block new acceptance and report the conflict; they
  do not silently select the newest write or tear down accepted services.
- If old readers require compatibility projections, give each projection one
  writer and a defined direction. Do not introduce a bidirectional sync loop.

Read-side normalization is the initial compatibility approach. Whether any
status projections or explicit adoption marker are needed remains contingent on
the complete reader/writer inventory. Public-cloud paths whose semantics do not
yet map safely remain on their existing implementation; they are not silently
converted to on-prem networking.

Validation covers optional-parent deletion as well as leaf changes. Invalid
desired values must remain correctable; preserving accepted configuration does
not mean freezing a rejected first write. Existing on-prem primary/secondary VIP
rules remain intact. The new None lifecycle does not grant arbitrary VIP
replacement or restrict already-supported operations on other platforms.

#### Feature gating and defaults

Feature gates control API/component availability, not durable administrator
intent. Align DNS exposure with 3609 and define any separate managed-LB gate
with API owners. Do not invent a gate name or declare a release default here.
The managed-DNS default applies to new eligible None SNO installations when its
feature is enabled. Existing absent or gate-only configuration preserves its
behavior. Managed LB always requires explicit networking intent and activation
eligibility; an upgrade, broad FeatureSet, or topology change alone is not an
opt-in. Gate removal is not an active-service teardown procedure.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The initial workflow applies to standalone clusters whose nodes host these
networking assets. Hosted control planes have a different management/guest
boundary and are not newly enabled. Shared API placement alone must not cause
guest or management nodes to render the standalone stack.

#### Standalone Clusters

The new endpoint workflow targets the None SNO-to-three-node compact outcome.
Topology controllers retain node/topology lifecycle responsibilities; the
networking contract supplies the explicitly selected endpoint behavior.
Compatibility tests cover existing standalone platforms. Generic fresh None
multi-node and External enablement require separate scope and qualification
decisions, not merely matching a platform-neutral schema.

#### Single-node Deployments or MicroShift

Ordinary SNO uses the DNS-only path without managed-LB resource overhead.
Measure DNS CPU/memory, startup, upstream refresh, and full-node reboot
behavior. Measure any transition-only networking overhead separately. No
measured values are claimed. Cluster-component containers must declare CPU and
memory requests and should not set limits under [resource
conventions][resource-conventions].

SNO rollout or reboot can interrupt the only control plane and workloads. A
temporary VIP on one node provides no failover. An unproven one-backend runtime
path cannot be assumed safe merely because templates render.

MicroShift does not use this Infrastructure/MCO contract; no configuration
change is included. Image-based SNO installation is a distinct research and
integration path, not an implicitly qualified transition source. The current
mutable-topology design excludes image-based installations.

#### OpenShift Kubernetes Engine

No new optional OLM operator is introduced.

### Implementation Details/Notes/Constraints

#### API, acceptance, and topology integration

openshift/api defines the shared contract, validation, and exposure. CNO already
has [on-prem VIP reconciliation][cno-sync], but its assumptions about accepted
VIPs do not provide None's initial configuration or DNS-only state. CNO and CCO
must agree one networking acceptance writer, field-scoped updates, and the
handoff to topology eligibility. CCO here means cluster-config-operator, not
cloud-credential-operator. This does not assign ownership of all Infrastructure
status to the networking controller.

The compatibility inventory includes installer initialization, controller
writes, MCO/ControllerConfig conversion, certificate/endpoint consumers, and
legacy update paths. Keep platform fields for actual provider integration; do
not remove every platform check merely to call the design platform agnostic.

#### MCO and baremetal-runtimecfg

MCO consumes accepted effective networking and selects DNS-only versus the
eligible on-prem stack. The [current renderer][mco-render] excludes None from
on-prem paths and has a separate cloud alternate-DNS path. Adding None to one
platform list is insufficient: templates, resolver assets, endpoint helpers,
bootstrap, and certificates must use consistent decisions.

runtimecfg supplies init/sidecar commands inside independently rendered CoreDNS,
keepalived, and HAProxy pods; it is not a standalone service toggle. The 3609
DNS-only path supplies node discovery without VIPs. Its address and
upstream-resolver refresh contract must be agreed with its SME.

The current unicast [keepalived monitor rejects updates with fewer than two
backends][runtime-guard]. This remains an integration constraint, not a mandate
to implement standalone SNO LB. Revisit it if activation ordering requires a
temporary single-backend state. HAProxy handles the API path; MCS port 22623 and
its certificates must be verified separately. Ingress is verified through its
VIP and router path, not described as another HAProxy backend by assumption.

#### Installer, Assisted/ABI, and image-based installation

The installer resolves DNS default/opt-out and supplies bootstrap inputs that
match steady-state semantics. Bootstrap cannot depend on an in-cluster
controller that requires the DNS being configured. Assisted/ABI selects or omits
its conditional DNS assets from the same effective policy. Its service database
is not a Day-2 Infrastructure API.

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
- **Premature activation or dependency deadlock:** review node joining, accepted
  topology, backend availability, endpoint readiness, and external access
  ordering together; do not substitute node count for the contract.
- **Resolver conflicts or bootstrap failure:** single resolver ownership,
  explicit legacy handoff, and distinct cloud/bootstrap integration tests.
- **VIP conflicts and exposure:** site administrators reserve addresses and
  verify reachability; schema validation does not prove an address is unused.
- **Access loss:** retain a recovery path, require a maintenance window where
  disruptive, and verify endpoints before external DNS cutover.
- **Scope/maintenance expansion:** qualify each new platform/mode explicitly; a
  general API is not universal enablement or an unnamed maintenance owner.

Networking and MCO reviewers assess host networking, privileges, mounts,
resolver ownership, and endpoint exposure. API, installer, and platform owners
review defaulting, compatibility, and error UX. Retain existing API/MCS TLS and
certificate behavior; no independent TLS policy or new admission-service
dependency is proposed.

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

On-prem fields share substantial structure, but cloud endpoint discovery and
resolver/bootstrap behavior differ. A universal rewrite would enlarge risk and
delay the initial use case. Preserve legacy paths and converge only with
reviewed semantic mappings and evidence.

### Keep a feature gate or user MachineConfig as the configuration API

A gate controls availability, and a MachineConfig changes node files; neither
provides a durable cross-component ownership and compatibility contract. Reuse
MCO rather than introduce a separate LB operator.

### Enable full load balancing on ordinary SNO

This consumes resources without adding node failover and is not needed to
standardize DNS. Prepare networking intent separately and activate the stack
only within the reviewed multi-node transition workflow.

## Open Questions

1. What shared spec/status shape, endpoint-source representation, and
   accepted-versus-active reporting do API owners recommend?
2. Which component accepts networking, owns compatibility mappings/projections,
   and reports conflicts? How do legacy writers behave after explicit adoption?
3. At what transition boundary may LB activation occur? How are existing access,
   node joining, desired/accepted topology, DNS cutover, and completion ordered?
   Is a temporary single-backend state required at all?
4. Which legacy on-prem groups adopt the shared API first, and which remain
   read-compatible only? What evidence permits later deprecation? No number of
   releases or removal date is assumed.
5. What installer/bootstrap representation, gates, and refresh behavior align
   with 3609 while preserving gate-only and explicitly opted-out clusters?
6. Which Assisted and image-based starting states are eligible for adoption, and
   who owns their resolver-asset handoff and recovery?
7. How does External enablement reuse the contract, and which cloud differences
   require retained provider-specific configuration? Who owns each integration?
8. What initial address-family, resource, operational, and OKE qualification
   matrix is agreed by the component, product, and QE owners?

## Test Plan

Use source/rendering fixtures before disruptive integration tests. Cover legacy
and shared inputs, absence and defaults, matching/conflicting representations,
rejected-input correction, optional-parent deletion, and concurrent writers.
Prove supported legacy VIP mutations and cloud endpoint distinctions survive
normalization. Unknown cloud endpoints must not become fabricated VIPs or a
request to disable an active resolver.

Test new None DNS-only installation, explicit opt-out, existing gate-only DNS,
and each advertised legacy adoption path. Assert no LB assets on ordinary SNO or
from prepared-but-ineligible intent. Test address/upstream refresh, startup
dependencies, restart/reboot recovery, and resource cost.

Integration with mutable topology verifies the reviewed activation ordering, DNS
and external-access cutover, API and ingress traffic, separate MCS access,
backend changes, failure/retry, and the managed-endpoint multi-node outcome.
Test preservation of administrator-managed networking when no opt-in exists. The
platform and provisioning configuration remain unchanged; topology is allowed to
change through its designated controller.

Include interrupted upgrades, mixed consumers, and legacy-render non-regression
for each affected platform. Test any compatibility projection against old
readers. Image-based transitions, External enablement, and new cloud behavior
require their own agreed qualification rather than implied coverage here. No
POC, conformance, or payload results are claimed by this proposal.

## Graduation Criteria

### Entering Dev Preview

- API and compatibility semantics, acceptance ownership, and topology
  integration are reviewed, with unsupported combinations rejected.
- DNS-only bootstrap/default/opt-out and legacy preservation have evidence.
- The explicit None managed-endpoint workflow works end to end with defined
  recovery; any necessary transient backend state is validated.
- Operators expose acceptance, activation, and rollout failures distinctly.

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
- End-to-end, conformance, upgrade, and disruption tests cover the supported
  DNS-only and transition-associated endpoint workflows.
- Feature graduation preserves explicit policy and legacy behavior.
- User feedback, support procedures, and resource expectations are addressed.

### Removing a deprecated feature

This enhancement does not remove None or existing networking fields on a fixed
schedule. Any deprecation requires a migration path, identified consumers,
documented support policy, and separate review before removal. Supporting two
representations temporarily is not an API-removal deadline.

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

Inventory installer/bootstrap, Infrastructure writers, MCO, certificate/endpoint
consumers, runtimecfg, and topology consumers. Test old readers with legacy
inputs and any proposed projections, and new readers with old writers. Do not
accept modes that an older active consumer would misinterpret.

Unknown/missing policy must not activate new defaults or remove active services.
Controllers preserve field ownership and accepted state through restarts and
conflicts. A shared logical model does not require every component to upgrade
atomically; the coexistence policy must work throughout the supported skew.

## Operational Aspects of API Extensions

Configuration remains a cluster singleton with infrequent writes. No per-node
API fleet, aggregated API server, or network-dependent admission service is
introduced. Report the candidate generation, accepted configuration authority,
pending activation, and actionable rejection reasons using reviewed status
mechanisms. MCO/MachineConfigPool status reports node rollout; topology status
reports transition progress; endpoint checks establish runtime health.

An acceptance-controller outage blocks new acceptance but does not remove
rendered services. A rollout, DNS, or VIP failure can still interrupt API
access, image pulls, and workloads. Networking, MCO, topology, and affected
installer/platform teams share escalation according to the failed stage.

QE and component owners measure acceptance/rollout duration, DNS resolution,
endpoint availability, disruption, and resource consumption. Metric names and
alert thresholds require implementation review; customer addresses must not be
used as telemetry labels.

## Support Procedures

Establish the authoritative legacy/shared configuration and the last accepted
active state before changing anything. Inspect Infrastructure, feature
configuration, acceptance/activation reporting, topology progress, MCO
conditions, and MachineConfigPool state.

- **Conflicting or rejected input:** correct the unaccepted request or resolve
  its source conflict. Do not clear active fields to force reconciliation.
- **Waiting for activation:** inspect the agreed topology/backend prerequisites
  and retained access path; do not manufacture topology status to start LB.
- **DNS failure:** inspect the selected implementation, resolver owner, port 53,
  node/provider addresses, upstreams, manifests, image availability, and logs.
  Use the installation-specific handoff procedure for legacy assets.
- **Endpoint failure:** inspect VIP ownership and API/ingress paths, test API
  hostname/CA, and check MCS port 22623 and certificates independently.
- **Partial rollout/transition:** repair the failed stage and reconcile from
  accepted state; follow the topology design for node/etcd failure handling. No
  automatic networking reversal or topology recovery is promised here.
- **Disablement request:** installation opt-out is not active-DNS teardown.
  Deleting policy or disabling gates is not a supported removal procedure.

Retain console access for failures that prevent API-based repair. Existing
workloads may be interrupted by node reboot or endpoint/DNS failure; new
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
[mco-render]:
    https://github.com/openshift/machine-config-operator/blob/3f863f437e3af4d975300927b3ca7e2bf7dbbb29/pkg/controller/template/render.go
[cno-sync]:
    https://github.com/openshift/cluster-network-operator/blob/069ef080db6b20f6704e4dc0c07d48937cef0ef8/pkg/controller/infrastructureconfig/sync_vips.go
[runtime-guard]:
    https://github.com/openshift/baremetal-runtimecfg/blob/3057d9978db74dcd5012a293d37d571b524c4e46/pkg/monitor/dynkeepalived.go#L133-L145
[cloud-api]:
    https://github.com/openshift/api/blob/38053d30e7e728362c4c6df1e402f6f7eb37d299/config/v1/types_infrastructure.go#L929-L1000
[cloud-mco]:
    https://github.com/openshift/machine-config-operator/blob/3f863f437e3af4d975300927b3ca7e2bf7dbbb29/templates/common/cloud-platform-alt-dns/files/coredns.yaml
[cloud-resolver]:
    https://github.com/openshift/machine-config-operator/blob/3f863f437e3af4d975300927b3ca7e2bf7dbbb29/templates/common/cloud-platform-alt-dns/files/usr-local-bin-update-dns-server.yaml
[resource-conventions]:
    https://github.com/openshift/enhancements/blob/2cf7bf953506ba2a463848e59c8bc574e340406d/CONVENTIONS.md#resources-and-limits
