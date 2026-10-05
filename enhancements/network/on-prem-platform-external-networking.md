---
title: external-platform-on-prem-networking
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
last-updated: 2026-10-05
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
`ingressIPs`, `loadBalancer`, `dnsRecordsType`, and `machineNetworks` `so an
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
  native load balancer, I want to configure API and Ingress VIPs so that
  OpenShift's self-hosted load balancing serves cluster traffic.
* As a deployer of an `External` cluster, I want to declare that I provide my own
  load balancer and DNS (`loadBalancer.type: UserManaged`, `dnsRecordsType:
  External`) so that OpenShift does not deploy its on-prem networking static pods.
* As a cluster administrator, I want `machineNetworks` recorded in the
  infrastructure status so that components can resolve the node networks on an
  `External` cluster the same way they do on other on-prem platforms.

### Goals

- Allow an `External` cluster to run with OpenShift-managed on-prem load
  balancing and internal DNS.
- Allow an `External` cluster to run with a user managed load balancer and
  externally provided DNS.
- Reuse the existing on-prem fields rather than inventing new
  ones.

### Non-Goals

- Changing on-prem networking behavior for existing platforms (BareMetal,
  vSphere, OpenStack, Nutanix).
- Defining the installer UX; only the API surface and its semantics are in scope
  here.
- Graduating the feature beyond Tech Preview in this proposal.

## Proposal

Extend `ExternalPlatformStatus` in `config/v1` with the on-prem networking fields
already present on other platforms, all gated by the `ExternalPlatformOnPrem`
feature gate:

- `apiServerInternalIPs` and `ingressIPs`: the VIPs for OpenShift's self-hosted
  load balancer, at most one IPv4 and one IPv6 address each.
- `loadBalancer.type`: `OpenShiftManagedDefault` (deploy the API/Ingress static
  pods) or `UserManaged` (do not deploy them; the deployer supplies the load
  balancer). Defaults to `OpenShiftManagedDefault` and is immutable once set.
- `dnsRecordsType`: `Internal` (internal DNS provides api/api-int/ingress
  records) or `External` (records provided outside the cluster). Only settable to
  `External` when `loadBalancer.type` is `UserManaged`.
- `machineNetworks`: the node networks, mirroring the other on-prem platforms.

### Workflow Description

**deployer** is the person responsible for standing up the PlatformType
`External` cluster.

1. The deployer chooses whether OpenShift or they provide load
   balancing and DNS.
2. For OpenShift-managed networking, the deployer sets `apiServerInternalIPs`,
   `ingressIPs`, and leaves `loadBalancer.type` as `OpenShiftManagedDefault`;
   the on-prem static pods are deployed.
3. For user-managed networking, the deployer sets `loadBalancer.type:
   UserManaged` and optionally `dnsRecordsType: External`, configures their own
   load balancer and DNS. OpenShift does not deploy the on-prem
   static pods.

### API Extensions

Modifies the existing `infrastructures.config.openshift.io` CRD (status
subresource, `ExternalPlatformStatus`). New fields: `apiServerInternalIPs`,
`ingressIPs`, `loadBalancer` (with `type`), `dnsRecordsType`, and
`machineNetworks`. All are gated by `ExternalPlatformOnPrem` and are `status`
fields owned by the installer/infrastructure. Validation:

- `apiServerInternalIPs` / `ingressIPs`: `MaxItems=2`, IP format, at most one
  address per IP family (CEL).
- `loadBalancer.type`: enum, defaulted, immutable once set (CEL).
- `dnsRecordsType`: only `External` when `loadBalancer.type` is `UserManaged`
  (CEL, defined at the `ExternalPlatformStatus` level).
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

- Which components beyond the MCO on-prem static pods need to learn about the
  `External` platform for these fields to take effect end to end?

## Test Plan

CRD-level validation is covered by the integration test suite in
`config/v1/tests` (enum, immutability, IP-family, and cross-field
`dnsRecordsType`/`loadBalancer` rules). End-to-end coverage of the on-prem static
pods on `External` is added ahead of GA.

## Graduation Criteria

### Dev Preview -> Tech Preview

- API available behind `ExternalPlatformOnPrem` with validation tests.
- Ability to bring up an `External` cluster with OpenShift-managed and
  user-managed networking end to end.

### Tech Preview -> GA

- Upgrade/downgrade and scale testing.
- User-facing documentation in openshift-docs.
- Feature enabled by default.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

The new fields are status fields gated by a feature gate. Clusters that do not
set them keep their current behavior. When downgrading, the fields are ignored by
older components because `loadBalancer.type` is immutable once set. Existing
clusters retain their chosen behavior.

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

