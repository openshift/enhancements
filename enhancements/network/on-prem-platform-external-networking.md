---
title: on-prem-platform-external-networking
authors:
  - "@emy"
reviewers:
  - "@cybertron, for on-prem networking (keepalived/haproxy/coredns static pods) impact"
  - "@TBD, for installer integration of the External platform"
approvers:
  - "@TBD"
api-approvers:
  - "@TBD"
creation-date: 2026-09-25
last-updated: 2026-10-30
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OPNET-771
replaces: []
superseded-by: []
---

# External Platform On-Prem Networking

## Summary

The generic `External` platform type lets deployers integrate their own
provider components with OpenShift. Today `External` does not expose the
on-prem networking primitives (self-hosted API/Ingress load balancing, internal
DNS, and machine networks) that other platforms like BareMetal, vSphere or
OpenStack rely on. This enhancement extends `infrastructure.config.openshift.io`
`ExternalPlatformStatus` with the same on-prem networking surface: `apiServerInternalIPs`,
`ingressIPs`, `loadBalancer`, `dnsRecordsType`, and `machineNetworks` so an
`External` cluster can run with OpenShift-managed on-prem networking or defer to
a user-managed load balancer and externally-provided DNS. The work is gated
behind the `ExternalPlatformOnPrem` feature gate.

## Motivation

Deployers who use the `External` platform to run OpenShift on infrastructure
without a native cloud load balancer or DNS currently have no supported way to
enable the on-prem networking stack (keepalived/haproxy/CoreDNS static pods) or
to declare that they will supply their own load balancer and DNS. This forces
them to fork platform behavior or misuse another platform type. Bringing the
established on-prem fields to `External` gives these deployers the same choices
already available on BareMetal, vSphere, and OpenStack.

### User Stories

* As a deployer integrating the `External` platform on infrastructure without a
  native load balancer, I want to opt in to OpenShift's self-hosted on-prem load
  balancing by configuring API and Ingress VIPs, so that OpenShift serves
  cluster traffic without my having to supply an external load balancer.
* As a deployer who already supplies my own load balancer and DNS (the current
  `External` behavior), I want that behavior to remain the default and to be
  explicitly representable (`loadBalancer.type: UserManaged`, `dnsRecordsType:
  External`), so that enabling this feature never causes OpenShift to deploy
  on-prem networking static pods I did not ask for.
* As a cluster administrator, I want `machineNetworks` recorded in the
  infrastructure status so that components can resolve the node networks on an
  `External` cluster the same way they do on other on-prem platforms.

### Goals

- Add a net-new capability: allow an `External` cluster to opt in to
  OpenShift-managed on-prem load balancing and internal DNS
  (keepalived/haproxy/CoreDNS static pods), which `External` cannot do today.
- Preserve the current `External` behavior (user-managed load balancer and
  externally provided DNS) as the default, and make it explicitly representable
  in the API.
- Provide an `install-config` interface so deployers can select the behavior at
  install time (the installer owns these status fields).
- Reuse the existing on-prem fields rather than inventing new
  ones.

### Non-Goals

- Changing on-prem networking behavior for existing platforms (BareMetal,
  vSphere, OpenStack, Nutanix).
- Changing the default behavior of existing `External` clusters: clusters that
  do not opt in keep running exactly as they do today.
- Graduating the feature beyond Tech Preview in this proposal.

## Proposal

Extend `ExternalPlatformStatus` in `config/v1` with the on-prem networking fields
already present on other platforms, all gated by the `ExternalPlatformOnPrem`
feature gate:

- `apiServerInternalIPs` and `ingressIPs`: the VIPs for OpenShift's self-hosted
  load balancer, at most one IPv4 and one IPv6 address each.
- `loadBalancer.type`: `OpenShiftManagedDefault` (deploy the API/Ingress static
  pods) or `UserManaged` (do not deploy them; the deployer supplies the load
  balancer). Unlike the other on-prem platforms, `External` defaults to
  `UserManaged`: `External` does not deploy on-prem networking today, so
  defaulting to `OpenShiftManagedDefault` would change the behavior of existing
  clusters when the feature gate is enabled. The self-hosted path is opt-in — a
  deployer selects `OpenShiftManagedDefault` and sets the VIPs. The field is
  immutable once set. Consuming components deploy the on-prem static pods only
  when `loadBalancer.type` is `OpenShiftManagedDefault` and the VIPs are set,
  never on the basis of a defaulted value alone.
- `dnsRecordsType`: `Internal` (internal DNS provides api/api-int/ingress
  records) or `External` (records provided outside the cluster). Only settable to
  `External` when `loadBalancer.type` is `UserManaged`. Defaults to `External`,
  consistent with the `UserManaged` load-balancer default and with the fact that
  `External` runs no in-cluster DNS today. The `External` default applies to the
  user-managed path. A deployer opting in to the OpenShift-managed path
  (`loadBalancer.type: OpenShiftManagedDefault`) must set `dnsRecordsType:
  Internal`, since `External` is only valid with `UserManaged`.
- `machineNetworks`: the node networks, mirroring the other on-prem platforms.

### Workflow Description

**deployer** is the person responsible for standing up the PlatformType
`External` cluster.

1. The deployer chooses whether OpenShift or they provide load
   balancing and DNS. With no on-prem fields set, OpenShift provides none,
   matching today's `External` behavior.
2. For OpenShift-managed networking, the deployer opts in by setting
   `loadBalancer.type: OpenShiftManagedDefault` and `dnsRecordsType: Internal`;
   the on-prem static pods are deployed.
3. For user-managed networking (the default), the deployer leaves
   `loadBalancer.type` as `UserManaged` and optionally sets `dnsRecordsType:
   External`, and configures their own load balancer and DNS. OpenShift does not
   deploy the on-prem static pods. When `dnsRecordsType: External` is used, the
   deployer must pre-create the cluster DNS records (see below) before the
   cluster is brought up.

### DNS Records

Which DNS records the deployer must provide depends on `dnsRecordsType`, and
mirrors the behavior of the other on-prem platforms (BareMetal, vSphere,
OpenStack):

- `dnsRecordsType: Internal` (used with the opt-in OpenShift-managed path): the
  OpenShift-managed CoreDNS static pod serves the `api`, `api-int`, and `*.apps`
  records internally, pointing at the configured VIPs (`apiServerInternalIPs` /
  `ingressIPs`). The deployer's external/upstream DNS only needs the records
  required by clients outside the cluster — `api.<base_domain>` and
  `*.apps.<base_domain>`. The `api-int` record is internal-only and is served by
  CoreDNS. The deployer doesn't need to create it externally.
- `dnsRecordsType: External` (the default; only valid with `loadBalancer.type:
  UserManaged`):
  CoreDNS does not serve any of these records. The deployer must pre-create all
  of them in their own DNS, pointing at their user-managed load balancer:
  `api.<base_domain>`, `api-int.<base_domain>`, and `*.apps.<base_domain>`.

#### DualStack

DualStack `External` clusters are supported: `apiServerInternalIPs` and
`ingressIPs` each accept at most one IPv4 and one IPv6 address, and the CoreDNS
Corefile already renders both IP families (it emits an `A`/`AAAA` record for the
primary family and a second record for the secondary family when a second
address is present). No additional mechanism is required.

For a DualStack cluster with `dnsRecordsType: External`, the deployer must create
both an `A` and an `AAAA` record for each of `api`, `api-int`, and `*.apps`.
With `dnsRecordsType: Internal`, CoreDNS emits the per-family records
automatically from the configured VIPs.

### API Extensions

Modifies the existing `infrastructures.config.openshift.io` CRD (status
subresource, `ExternalPlatformStatus`). New fields: `apiServerInternalIPs`,
`ingressIPs`, `loadBalancer` (with `type`), `dnsRecordsType`, and
`machineNetworks`. All are gated by `ExternalPlatformOnPrem` and are `status`
fields owned by the installer/infrastructure. Validation:

- `apiServerInternalIPs` / `ingressIPs`: `MaxItems=2`, IP format, at most one
  address per IP family (CEL).
- `loadBalancer.type`: enum, immutable once set (CEL). Defaulted to
  `UserManaged` for `External` (differs from the `OpenShiftManagedDefault` used
  by other on-prem platforms) to preserve existing behavior.
- `dnsRecordsType`: defaulted to `External`; only `External` when
  `loadBalancer.type` is `UserManaged` (CEL, defined at the
  `ExternalPlatformStatus` level).
- `machineNetworks`: `MaxItems=32`, unique entries (CEL).

### Topology Considerations

#### Hypershift / Hosted Control Planes

Not applicable. On-prem load balancing and DNS static pods are a standalone
control-plane concern. `External` on-prem fields are not consumed by hosted
control planes.

#### Standalone Clusters

This is the primary and only target topology.

#### Single-node Deployments or MicroShift

SNO with the `External` platform can use these fields the same way other on-prem
SNO deployments do. No additional resource impact beyond the existing on-prem
static pods. MicroShift does not use the infrastructure CRD on-prem networking
stack so it is unaffected.

#### OpenShift Kubernetes Engine

No OKE-specific impact. The feature depends only on on-prem networking
components included in OKE.

### Implementation Details/Notes/Constraints

The fields intentionally mirror the shapes already used by `BareMetalPlatformStatus`
and `VSpherePlatformStatus` so the consuming components (MCO on-prem static pods,
CoreDNS) can treat `External` like other on-prem platforms with minimal change.

DualStack is supported with no extra mechanism: the VIP fields accept one IPv4
and one IPv6 address each, and the existing on-prem rendering (VIP getters and the
CoreDNS Corefile) already handles both families, emitting secondary-family records
when a second address is present.

#### Installer Interface

Because these are `status` fields owned by the installer/infrastructure, a
deployer cannot set them directly on a running cluster; the installer must
populate them. The installer therefore needs an `install-config` interface for
the `External` platform (VIPs, `loadBalancer.type`, `dnsRecordsType`,
`machineNetworks`), mirroring how BareMetal/vSphere accept these in
`install-config` and flow them through to `PlatformStatus`. The detailed
installer UX is specified in the installer repo, but the feature is not
considered complete until that interface exists — without it there is no
supported way for a deployer to opt in to the OpenShift-managed path. This is
reflected in the graduation criteria below.

#### Affected Components

The components that must learn about the `External` platform for these fields
to take effect end to end:

- cluster-network-operator (CNO): 
  CNO does not render the on-prem static pods but its `infrastructureconfig`
  controller keeps the Infrastructure CR's VIP and `machineNetworks`
  fields consistent between spec and status and validates them. Today `External` is
  excluded from the controller's on-prem platform predicate and from the
  spec/status sync logic so VIPs and machine networks set on `External` would not be
  propagated or validated. CNO needs `External` added to its on-prem platform
  predicate and its spec/status sync (mirroring the BareMetal/vSphere/OpenStack
  cases). Because `External` is also used for pure cloud-provider-external
  clusters with no self-hosted load balancer, CNO should treat `External` as
  on-prem only when the on-prem fields are actually set, rather than
  unconditionally.

### Risks and Mitigations

The main risk is behavior divergence from other on-prem platforms. Reusing the
existing fields and CEL validations mitigate this.

### Drawbacks

It adds on-prem-specific surface to a platform type meant to be generic, which
slightly broadens the `External` scope. This is outweighed by giving deployers
a supported path instead of forcing them to misuse other platform types.

## Alternatives (Not Implemented)

- Introducing a new set of `External`-only networking fields with different
  shapes: rejected to avoid divergence and duplicated validation.

## Open Questions [optional]

- None outstanding.

## Test Plan

CRD-level validation is covered by the integration test suite in
`config/v1/tests` (enum, immutability, IP-family, and cross-field
`dnsRecordsType`/`loadBalancer` rules). End-to-end coverage of the on-prem static
pods on `External` is added ahead of GA.

## Graduation Criteria

### Dev Preview -> Tech Preview

- API available behind `ExternalPlatformOnPrem` with validation tests.
- `install-config` interface available for the `External` on-prem fields so a
  deployer can select the behavior at install time.
- Ability to bring up an `External` cluster with OpenShift-managed and
  user-managed networking end to end.

### Tech Preview -> GA

- Upgrade/downgrade and scale testing.
- User-facing documentation in openshift-docs.
- Feature enabled by default.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

The new fields are status fields gated by a feature gate. Because
`loadBalancer.type` defaults to `UserManaged` and the on-prem static pods are
deployed only when `OpenShiftManagedDefault` is explicitly selected with VIPs,
enabling the feature gate on an existing `External` cluster does not change its
behavior. Clusters that do not opt in keep their current behavior. When
downgrading, the fields are ignored by older components because
`loadBalancer.type` is immutable once set. Existing clusters retain their chosen
behavior.

## Version Skew Strategy

During upgrade the consuming components tolerate the fields being absent, matching
the existing on-prem behavior. No coordination is required.

## Operational Aspects of API Extensions

This enhancement modifies a CRD. There is no expected API-throughput impact.

## Support Procedures

Failures surface through the existing on-prem networking components (keepalived,
haproxy, CoreDNS static pods managed by the MCO); detection, symptoms, and
remediation are identical to other on-prem platforms. Disabling the feature gate
prevents the fields from being honored; on a running cluster the immutable
`loadBalancer.type` preserves the already-selected behavior.

## Infrastructure Needed [optional]

None.

