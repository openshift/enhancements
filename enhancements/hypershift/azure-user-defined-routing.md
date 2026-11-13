---
title: azure-user-defined-routing
authors:
  - "@twolff"
reviewers:
  - "@jparril"
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-10-27
last-updated: 2026-10-27
tracking-link:
  - https://redhat.atlassian.net/browse/ARO-23477
replaces: []
superseded-by: []
---

# Azure User Defined Routing (UDR) Outbound Type

## Summary

Add `UserDefinedRouting` as a second value for a new `outboundType` field on
`HostedCluster.spec.platform.azure`. When set, HyperShift stops attaching
worker NICs to the cluster Load Balancer's outbound backend pool, and the
HyperShift CLI's self-managed infra path stops creating the Load Balancer's
outbound rule. The customer becomes responsible for egress (NAT Gateway,
virtual appliance, etc.) on their subnets. `LoadBalancer` stays the default
and unchanged. `outboundType` is immutable once set.

## Motivation

The Load Balancer's outbound rule enforces a static SNAT port allocation
(1,024 ports/node, 64,000/IP) that caps a cluster at ~62 nodes per outbound
IP. Attaching a NAT Gateway doesn't lift this cap — the outbound rule still
owns the egress path until it's removed. Customers who need to scale past
that limit, want NAT Gateway zone redundancy, or already run their own
egress controls have no way off the Load Balancer today. Upstream OpenShift
has supported `UserDefinedRouting` on Azure since 4.21/4.22.

The field already exists end-to-end in the ARM API and Clusters Service
model but is validated to accept only `LoadBalancer` and is never forwarded
to HyperShift, which has no such field and hardcodes Load Balancer behavior
in the CCM config, the CAPZ `AzureCluster` spec, and the CLI's infra path.
This proposal closes the HyperShift gap; CS and RP need follow-on changes
(see Implementation Details).

### User Stories

- As a customer with existing egress controls, I want cluster egress to go
  through my own infrastructure, not an ARO-managed Load Balancer.
- As a `hcp create cluster azure` user, I want `--outbound-type
  UserDefinedRouting` so created infra never gets an outbound rule.
- As a customer scaling past ~62 nodes per outbound IP, I want to attach my
  own NAT Gateway and select `UserDefinedRouting` so the Load Balancer's
  outbound rule doesn't cap me.

### Goals

- Add `OutboundType` (`LoadBalancer` default, `UserDefinedRouting`) to
  `AzurePlatformSpec`.
- Under `UserDefinedRouting`: leave `NodeOutboundLB` unset so CAPZ doesn't
  attach worker NICs to a backend pool; CLI infra path omits the outbound
  rule (Load Balancer is still created, for inbound Services).
- Leave `LoadBalancer` behavior unchanged.
- Keep CCM `DisableOutboundSNAT: true` unconditional in both cases, so a
  `Service` of type `LoadBalancer` can never become an accidental egress
  path through an ARO-owned IP.

### Non-Goals

- ARO-HCP provisions no egress infrastructure under UDR — fully customer-owned.
- No managed NAT Gateway enum values; that's a separate, deferred phase (ARO-27719).
- No day-2 mutability between `LoadBalancer` and `UserDefinedRouting` — field
  is immutable once set.
- No pre-flight validation of subnet egress config; misconfiguration surfaces
  as node provisioning failures.
- Clusters Service and RP changes are required follow-on, not designed here.
- Karpenter/`AKSNodeClass` egress is out of scope — doesn't go through CAPZ.

## Proposal

### Workflow Description

1. Cluster creator sets `outboundType: UserDefinedRouting` at create time,
   having already attached egress to the target subnet(s).
2. `reconcileAzureCluster` leaves `AzureCluster.Spec.NetworkSpec.NodeOutboundLB`
   unset; CAPZ creates worker NICs with no backend-pool reference.
3. CCM keeps `DisableOutboundSNAT: true` unconditionally (unchanged from
   today's effective value).
4. The CLI's self-managed infra path (`cmd/infra/azure/networking.go`)
   still creates the Load Balancer but omits its outbound rule.
5. The customer's own egress mechanism provides outbound connectivity
   (image pulls, API access, private endpoints like Key Vault for KMS).

### API Extensions

- `AzurePlatformSpec.OutboundType AzureOutboundType` (`+optional`).
- `AzureOutboundType string`, enum `LoadBalancer`/`UserDefinedRouting`.
- Three CEL rules, all required: cannot remove once set; immutable once set;
  cannot set to `UserDefinedRouting` from unset (closes the
  unset→LoadBalancer→UDR bypass of immutability).
- Ungated — no feature gate.

### Topology Considerations

HyperShift/Azure-specific only. Not applicable to standalone clusters,
single-node/MicroShift, or other topologies.

### Implementation Details/Notes/Constraints

**This proposal (HyperShift):**
- `api/hypershift/v1beta1/azure.go` — field, enum, CEL rules.
- `hypershift-operator/.../platform/azure/azure.go` — conditional `NodeOutboundLB`.
- `control-plane-operator/.../cloud_controller_manager/azure/config.go` —
  `DisableOutboundSNAT: true` now explicitly unconditional (comment only).
- `cmd/infra/azure/*`, `cmd/cluster/azure/*` — `--outbound-type` flag,
  outbound-rule omission.
- Envtest CEL coverage; opt-in e2e (`test/e2e/azure_udr_test.go`, gated on
  `AZURE_UDR_SUBNET_ID`).

**Required follow-on (HyperShift must land first):**
- **Clusters Service**: set `OutboundType` on the HC CR (currently never
  set); stop creating the outbound rule + 9 egress IPs under UDR.
- **RP**: add `UserDefinedRouting` to the API enum, gated to a new API
  version with a clean rejection on older ones.

### Risks and Mitigations

- **CAPZ NIC creation fails (`InvalidResourceReference`) if `NodeOutboundLB`
  points at a nonexistent pool.** Mitigated by nil-ing it under UDR —
  verified in dev testing (zero such events, nodes reached `ready: true`).
- **Day-2 LB↔UDR mutability was tried via a relaxed CEL rule** neither 
  direction actually changes egress — LB→UDR leaves the old outbound
  rule and NIC pool membership in place; UDR→LB sets `NodeOutboundLB` but
  nothing recreates the outbound rule. Mitigation: immutable field;
  day-2 tracked as a separate future epic.
- **Karpenter/`AKSNodeClass` egress behavior is undefined** (no CAPZ path) — open.
- **Private ingress + UDR is untested**: unconfirmed whether CCM's
  unconditional `LoadBalancerName` config creates an LB anyway.

### Drawbacks

Immutability means a customer who picks wrong must recreate the cluster. This
is the scope for v1, unless day 2 updates can be cleanly worked out.

## Alternatives

- **Managed NAT Gateway** (ARO provisions/owns it, `NATGatewaySingleZone`/
  `NATGatewayMultiZone` per ARO-27719): rejected for now — ARO would chase
  the NAT Gateway API surface and relax deny assignments; NAT Gateway V2
  Private is coming and would add more churn. UDR is the building block;
  revisit managed NAT Gateway later with customer input.
- **Day-2 mutable `outboundType`**: implemented via relaxed CEL,no HyperShift 
  component owns the outbound rule post-create, so
  neither transition direction changes egress. Would need the
  hypershift-operator to continuously reconcile the outbound rule and
  existing-node pool membership — materially larger scope.

## Open Questions

- Node pool subnet handling: must customers pre-configure egress per subnet? Any ARO validation?
- Private ingress + UDR: does CCM's `LoadBalancerName` config create an unwanted LB?
- Karpenter/`AKSNodeClass` egress path ownership.
- SLI/SLO and SRE alerting for UDR egress failures — none defined yet.
- Validation scope: validate identity permissions over customer egress resources, or nothing?

## Test Plan

- Unit tests: CEL rules, CLI outbound-rule omission, CCM config.
- Envtest: 8 CEL cases (create pass/fail, invalid enum, LB→UDR rejected,
  removal rejected, unset→UDR rejected, unset→LB allowed).
- E2E: opt-in `test/e2e/azure_udr_test.go` (`AZURE_UDR_SUBNET_ID`), asserts
  `nodeOutboundLB` absent and `disableOutboundSNAT: true`. Not yet run live.
- Manual validation (2026-10-22, dev env): `NodeOutboundLB` nil under UDR,
  CCM `disableOutboundSNAT: true`, zero `InvalidResourceReference` events,
  `AzureMachine` ready, ingress LB with empty backend pool and zero
  outbound rules.

## Graduation Criteria

### Dev Preview -> Tech Preview

HyperShift API, CEL, and CAPZ/CLI changes merged; self-managed e2e passing
against a live cluster with a real NAT Gateway, proving node `Ready` and
egress through the customer's IP.

### Tech Preview -> GA

Clusters Service sets `OutboundType` and stops creating the outbound rule +
9 egress IPs under UDR; RP exposes the gated enum value.

### Removing a deprecated feature

N/A — additive only.

## Upgrade / Downgrade Strategy

New, optional field defaulting to empty (`LoadBalancer`). Existing clusters
unaffected on upgrade. No downgrade concern.

## Version Skew Strategy

None beyond standard HyperShift API versioning — a control plane without
this field behaves as today (`LoadBalancer`).

## Operational Aspects of API Extensions

Validated field on an existing CRD; no new CRDs or feature gate. CEL runs
at admission; failure is a rejected API request.

## Support Procedures

Under UDR, ARO-HCP owns no egress resource, so there's nothing ARO-side to
inspect on failure — root cause is the customer's NAT Gateway/route
table/appliance config. Primary symptom: node image-pull failure. No
SLI/SLO or alerting defined yet.

## Infrastructure Needed [optional]

None beyond existing HyperShift/ARO-HCP Azure CI and dev tooling.
