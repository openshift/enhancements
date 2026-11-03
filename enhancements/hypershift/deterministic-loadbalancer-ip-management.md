---
title: deterministic-loadbalancer-ip-management
authors:
  - "@jparrill"
reviewers:
  - "@csrwng"
  - "@sdminonne"
  - "@danmace"
  - "@muraee"
  - "@JoelSpeed"
approvers:
  - "@csrwng"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-09-29
last-updated: 2026-10-26
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2653
see-also:
  - https://issues.redhat.com/browse/RFE-7201
  - https://issues.redhat.com/browse/RFE-8602
  - https://issues.redhat.com/browse/RFE-5029
  - /enhancements/hypershift/kubelet-haproxy-routing-bare-metal.md
replaces: []
superseded-by: []
---

# Deterministic LoadBalancer IP Management for Hosted Control Planes

## Summary

This enhancement extends `LoadBalancerPublishingStrategy` inside `spec.services`
with two provider-agnostic fields for deterministic control of LoadBalancer IP
assignment on the kube-apiserver Service. Scope is **IP-only** — hostname/DNS
indirection is deferred as future work:

- `loadBalancerClass` — standard Kubernetes `Service.spec.loadBalancerClass`
  (GA since K8s 1.24) to select which LB controller provisions the Service.
- `serviceAnnotations` (`map[string]string`) — opaque annotation passthrough to the
  KAS Service. The user provides the exact annotations their LB provider expects.
  HyperShift never learns vendor-specific semantics.

A CEL-based allowlist restricts annotations to known LB provider
prefixes. Adding support for a new provider requires adding its prefix to
the allowlist — a one-line type change requiring a CRD bump, not a structural
API change.

The existing `spec.services` deep-equality immutability rule
(`self.services == oldSelf.services`) is unchanged. All fields — including
the new ones — are immutable after creation.

## Motivation

Multiple enterprise accounts require deterministic LoadBalancer IP assignment for
Hosted Control Planes. The primary use case is Telco/RAN environments where
firewall rules and DNS records must be pre-configured before cluster creation.

Currently, `LoadBalancerPublishingStrategy` only exposes `Hostname` — there is no
mechanism to control IP address pool selection (MetalLB IPAddressPool,
CiliumLoadBalancerIPPool, F5 IPAM labels), specify well-known IP addresses, or
configure provider-specific LoadBalancer settings.

### User Stories

- As a platform owner deploying Hosted Control Planes into a regulated or
  otherwise network-restricted environment, I want the kube-apiserver's external
  IP to be known before the cluster is provisioned, so that firewall rules, DNS
  records, and security allowlists can be approved in advance and cluster
  onboarding is not blocked.

- As an administrator of a management cluster that runs more than one LoadBalancer
  controller, I want to choose which controller provisions the kube-apiserver
  Service, so that the API endpoint is served by the intended provider.

- As a HyperShift user, I want the system to only accept annotation keys from
  known LB providers, so that misconfigured or unsupported annotations
  are rejected at admission time. HyperShift enforces this via a CEL-based
  annotation allowlist baked into the CRD — adding a new provider requires a
  CRD bump with the new prefix.

### Goals

- Allow Day 1 deterministic IP/pool assignment for kube-apiserver LB across all
  LB providers.
- Support controller selection for management clusters with multiple LB controllers.
- Enforce an allowlist: only known providers are accepted by default.
- Maintain backward compatibility: clusters without the new fields behave identically.

### Non-Goals

- **Per-provider typed API structs.** The provider-agnostic approach delegates
  provider semantics to the user via annotations. HyperShift does not validate
  provider-specific annotation values.
- **Day 2 IP migration.** Safe runtime IP address changes are deferred to a
  separate scope ([OCPSTRAT-3926](https://issues.redhat.com/browse/OCPSTRAT-3926)).
- **None platform support.** HyperShift has no "None" platform. Bare-metal
  self-managed deploys use Agent platform.
- **PowerVS / IBMCloud support.** No RFE, no standard annotation pattern. To be
  evaluated with team input.
- **OAuth or Konnectivity LB configuration.** v1 targets APIServer (public) only.
  OAuth (Azure self-managed) and Konnectivity are stretch goals, but the API
  design supports them without changes.
- **Automatic DNS management for self-managed platforms.** Self-managed platforms
  without ExternalDNS require manual DNS updates; this is documented, not automated.

## Proposal

### Overview

Two new fields under the existing
`spec.services[*].servicePublishingStrategy.loadBalancer` object:

1. **`loadBalancerClass`** (string, optional) — Maps directly to
   `Service.spec.loadBalancerClass`. Selects which LB controller provisions the
   KAS Service. Essential for management clusters with multiple LB providers
   (e.g., F5 for management + MetalLB for tenant Services). Immutable after
   creation (matches Kubernetes semantics — `loadBalancerClass` is immutable on
   Services).

2. **`serviceAnnotations`** (`map[string]string`, optional) — Passed through to
   the KAS Service as annotations. The user provides exactly the annotations
   their LB provider expects. HyperShift applies them per-key (never full-map
   overwrite). **Immutable after creation** (set once at cluster creation time,
   cannot be changed).

This approach is provider-agnostic by design:
- HyperShift never needs to learn vendor annotation schemas.
- Adding a new LB provider requires adding its prefix to the CEL allowlist —
  a one-line type change requiring a CRD bump, not a structural API change.
- The LB implementation is a management-cluster property, orthogonal to the
  guest cluster platform. An Agent-platform HC on an AWS management cluster
  has its KAS Service processed by the AWS LB controller, not MetalLB.

Because the fields live inside `LoadBalancerPublishingStrategy`, the API
struct is shared across all services (APIServer, OAuthServer, Konnectivity).
However, **Day 1 only implements annotation passthrough for KAS** in CPO.
If a user sets `serviceAnnotations` on OAuthServer or Konnectivity today,
the fields are accepted by the API but CPO does not process them — the
annotations are silently ignored. Adding passthrough for other services
requires CPO changes in a follow-up.

#### spec.services Immutability

`spec.services` is currently immutable via a single deep-equality CEL rule
(`self.services == oldSelf.services`). This rule is **unchanged** by this
enhancement. All fields — including the new `loadBalancerClass` and
`serviceAnnotations` — are immutable after creation via the existing rule.

No CEL migration, no per-field rules, no cost budget changes needed. The
immutability restriction on `serviceAnnotations` will be revisited when
Day 2 IP migration ([OCPSTRAT-3926](https://issues.redhat.com/browse/OCPSTRAT-3926))
requires mutable annotations.

### Workflow Description

#### Actors

- **Cluster administrator:** Creates or manages `HostedCluster` resources.
- **HostedCluster controller:** Mirrors HC spec to HCP, copies kubeconfig Secrets,
  mirrors HCP conditions to HC status, relays annotations HC→HCP.
- **CPO (Control Plane Operator):** Reconciles control plane components including
  KAS Service and TLS certificates.
- **LB Provider:** External controller (MetalLB, F5 CIS, AWS LB Controller, etc.)
  that processes Service annotations and assigns IPs.

#### Day 1: MetalLB Example

```yaml
apiVersion: hypershift.openshift.io/v1beta1
kind: HostedCluster
spec:
  services:
    - service: APIServer
      servicePublishingStrategy:
        type: LoadBalancer
        loadBalancer:
          loadBalancerClass: "metallb.io/metallb"
          serviceAnnotations:
            metallb.io/loadBalancerIPs: "10.1.1.100"
            metallb.io/address-pool: "production-pool"
    - service: OAuthServer
      servicePublishingStrategy:
        type: Route
    - service: Konnectivity
      servicePublishingStrategy:
        type: Route
    - service: Ignition
      servicePublishingStrategy:
        type: Route
```

1. The HostedCluster controller creates the HostedControlPlane, mirroring
   `spec.services` to HCP.
2. CPO reconciles the KAS Service:
   - Sets `Service.spec.loadBalancerClass` to `metallb.io/metallb`.
   - Applies each entry in `serviceAnnotations` as per-key annotation writes
     on the Service object.
3. MetalLB processes the annotations and assigns IP `10.1.1.100`.
4. `ReconcileServiceStatus()` reads `Service.Status.LoadBalancer.Ingress[0]`,
   propagates to kubeconfig, worker HAProxy config, etc. Standard existing flow.

#### Day 1: AWS NLB with Elastic IPs

```yaml
spec:
  services:
    - service: APIServer
      servicePublishingStrategy:
        type: LoadBalancer
        loadBalancer:
          serviceAnnotations:
            service.beta.kubernetes.io/aws-load-balancer-eip-allocations: "eipalloc-abc123,eipalloc-def456"
```

No `loadBalancerClass` needed — AWS management clusters typically have a single
LB controller (aws-cloud-controller-manager).

#### Day 1: F5 BIG-IP

```yaml
spec:
  services:
    - service: APIServer
      servicePublishingStrategy:
        type: LoadBalancer
        loadBalancer:
          loadBalancerClass: "f5.com/f5-bigip"
          serviceAnnotations:
            cis.f5.com/ip: "10.2.2.50"
            cis.f5.com/ipamLabel: "production"
```

#### Day 1: Azure Static IP

```yaml
spec:
  services:
    - service: APIServer
      servicePublishingStrategy:
        type: LoadBalancer
        loadBalancer:
          serviceAnnotations:
            service.beta.kubernetes.io/azure-load-balancer-ipv4: "20.0.0.100"
```

#### Future: Per-Service LB Configuration

Because the fields live inside `LoadBalancerPublishingStrategy`, extending to
other services requires zero API changes:

```yaml
spec:
  services:
    - service: APIServer
      servicePublishingStrategy:
        type: LoadBalancer
        loadBalancer:
          loadBalancerClass: "metallb.io/metallb"
          serviceAnnotations:
            metallb.io/loadBalancerIPs: "10.1.1.100"
    - service: OAuthServer
      servicePublishingStrategy:
        type: LoadBalancer
        loadBalancer:
          serviceAnnotations:
            metallb.io/loadBalancerIPs: "10.1.1.101"
    - service: Konnectivity
      servicePublishingStrategy:
        type: LoadBalancer
        loadBalancer:
          loadBalancerClass: "metallb.io/metallb"
          serviceAnnotations:
            metallb.io/loadBalancerIPs: "10.1.1.102"
```

#### Related: Kubelet → HAProxy Routing

An orthogonal change routes kubelet through HAProxy (`172.20.0.1`) on
bare-metal public clusters instead of connecting directly to the KAS
LoadBalancer IP. This is not a prerequisite for Day 1 but enables future
Day 2 IP migration work.

Covered by a separate enhancement:
[kubelet-haproxy-routing-bare-metal.md](kubelet-haproxy-routing-bare-metal.md)
(tracking: [CNTRLPLANE-4732](https://issues.redhat.com/browse/CNTRLPLANE-4732),
gated behind `KubeletHAProxyRouting` FeatureGate —
[CNTRLPLANE-4807](https://issues.redhat.com/browse/CNTRLPLANE-4807)).

### API Extensions

#### Modified type: LoadBalancerPublishingStrategy

```go
// LoadBalancerPublishingStrategy specifies settings used to expose a service
// as a LoadBalancer.
type LoadBalancerPublishingStrategy struct {
    // hostname is the name of the DNS record that will be created pointing
    // to the LoadBalancer.
    // +optional
    Hostname string `json:"hostname,omitempty"`

    // loadBalancerClass sets Service.spec.loadBalancerClass to select which
    // LB controller provisions the Service. Required when the management
    // cluster runs multiple LB controllers (e.g., F5 for management + MetalLB
    // for tenant workloads). Maps to the Kubernetes Service field directly.
    //
    // +optional
    LoadBalancerClass string `json:"loadBalancerClass,omitempty"`

    // serviceAnnotations is a map of annotations passed through to the
    // LoadBalancer Service. The user provides the exact annotations their LB
    // provider expects (e.g., metallb.io/loadBalancerIPs,
    // service.beta.kubernetes.io/aws-load-balancer-eip-allocations).
    //
    // Annotations are applied per-key — existing annotations set by other
    // controllers (e.g., MetalLB status annotations) are preserved.
    //
    // Only annotation keys matching known LB provider prefixes are
    // accepted (see allowlist below). Adding a new provider requires adding
    // its prefix to the CEL rule — a one-line type change requiring a CRD bump.
    //
    // Immutable after creation via the existing spec.services deep-equality
    // CEL rule. No per-field immutability rules needed.
    //
    // +optional
    // +kubebuilder:validation:MinProperties=1
    // +kubebuilder:validation:MaxProperties=10
    // +kubebuilder:validation:XValidation:rule="self.all(k, k.matches('^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\\/)?[A-Za-z0-9][-A-Za-z0-9_.]{0,62}$'))",message="annotation keys must be valid qualified names (optional prefix/name, prefix ≤ 253, name ≤ 63)"
    // +kubebuilder:validation:XValidation:rule="self.all(k, size(self[k]) <= 253)",message="annotation values must be at most 253 characters"
    // +kubebuilder:validation:XValidation:rule (see allowlist table for full rule)
    ServiceAnnotations map[string]string `json:"serviceAnnotations,omitempty"`
}
```

All fields — existing and new — are immutable after creation via the existing
`self.services == oldSelf.services` rule on `HostedClusterSpec`. No per-field
immutability rules are needed for Day 1.


#### Annotation allowlist

Only annotation keys matching known LB provider prefixes are accepted
via CEL on `serviceAnnotations`. Each prefix is matched using CEL `startsWith()`
— these are plain string prefixes, not regular expressions. Adding a new
provider is a conscious decision that includes E2E testing.

| Prefix (matched via `startsWith()`) | Provider | Example annotations |
|--------------------------------------|----------|-------------------|
| `metallb.io/` | MetalLB (v0.13+) | `loadBalancerIPs`, `address-pool` |
| `metallb.universe.tf/` | MetalLB (legacy) | `loadBalancerIPs`, `address-pool` |
| `service.beta.kubernetes.io/aws-load-balancer-eip` | AWS NLB (EIP) | `eip-allocations` |
| `service.beta.kubernetes.io/aws-load-balancer-subnet` | AWS NLB (subnet) | `subnets` |
| `service.beta.kubernetes.io/aws-load-balancer-target-group` | AWS NLB (TG) | `target-group-attributes` |
| `service.beta.kubernetes.io/azure-load-balancer-ipv` | Azure LB | `ipv4`, `ipv6` |
| `service.beta.kubernetes.io/azure-load-balancer-resource-group` | Azure LB (RG) | `resource-group` |
| `networking.gke.io/` | GCP | `load-balancer-ip-addresses` |
| `cis.f5.com/` | F5 BIG-IP CIS | `ip`, `ipamLabel` |
| `lbipam.cilium.io/` | Cilium LB IPAM | `ips`, `sharing-key` |
| `loadbalancer.openstack.org/` | OpenStack Octavia | `floating-network-id`, `floating-subnet-id` |

Annotations not matching any prefix produce a CEL validation error. Adding a
new provider requires adding its prefix to the CEL rule — a conscious decision
that includes E2E testing.

> **CI coverage note:** CI E2E tests cover MetalLB only. Other providers in the
> allowlist are included based on upstream documentation and customer RFEs, not
> HyperShift CI testing. One-time provider validation (F5, Cilium) is documented
> under Test Plan.

**Implementation order:** The initial API PR
([hypershift#9913](https://github.com/openshift/hypershift/pull/9913)) introduces
the types with `MinProperties=1`, `MaxProperties=10`, and the feature gate. The
allowlist CEL rule (`startsWith()` prefixes) and key/value size bounds are added
in a subsequent PR during implementation, once the base types are merged.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is exclusively for HyperShift. All changes are in the HyperShift
operator (management cluster) and CPO (HCP namespace). No guest cluster components
are modified.

- **Management cluster**: Modified API types on HostedCluster/HostedControlPlane.
  CPO reconciler changes for KAS Service annotation passthrough.
- **Guest cluster**: No changes. Worker HAProxy configuration is managed via
  MachineConfig/Ignition from the management cluster.
- **Split components**: The kubeconfig Secret is generated by CPO in the HCP
  namespace and copied to the HC namespace by the HC controller. No new
  cross-namespace communication needed.

#### Standalone Clusters

Not applicable. Standalone clusters manage LB Services directly.

#### Single-node Deployments or MicroShift

Not applicable. SNO and MicroShift do not use HyperShift.

#### OpenShift Kubernetes Engine

This enhancement adds optional API fields that do not affect OKE functionality.
OKE clusters using HyperShift would benefit from deterministic LB IP assignment
identically to OCP.

### Implementation Details/Notes/Constraints

#### Annotation Passthrough

CPO's `ReconcileService()` uses per-key annotation writes (`svc.Annotations[key] = value`),
never full map overwrite (confirmed at `kas/service.go:81, 86, 98`, etc.). This means annotations
set by external controllers (MetalLB `ip-allocated-from-pool`, cloud-controller-manager
status annotations) survive every reconciliation loop.

For this feature, `serviceAnnotations` entries from the APIServer's
`LoadBalancerPublishingStrategy` are applied directly as per-key writes.
No translation layer, no vendor-specific logic in CPO.

**Conflict resolution:** `serviceAnnotations` are written per key *before* CPO
writes its own platform annotations, which gives two different outcomes:

- For keys CPO does not manage (for example an annotation set by an external
  controller), the user-specified value wins — CPO rewrites it on every
  reconciliation.
- For keys CPO manages itself (`aws-load-balancer-type`,
  `aws-load-balancer-cross-zone-load-balancing-enabled`,
  `aws-load-balancer-attributes`, `aws-load-balancer-internal`,
  `external-dns.alpha.kubernetes.io/hostname`), **CPO wins**. Letting a user value
  survive on those keys would break the Service: setting
  `aws-load-balancer-internal` on the public KAS Service, for instance, would turn
  the public NLB internal and the cluster would lose its API endpoint.

In practice the second case cannot be reached: the CEL allowlist on
`serviceAnnotations` only accepts the provider prefixes listed below, and none of
them match a CPO-managed key. The precedence rule is the defence in depth that
makes the collision harmless if the allowlist ever grows.

Annotations on keys *not* in `serviceAnnotations` are left untouched.

#### Which Service receives the fields

`serviceAnnotations` carry an address, so only the Service exposing the cluster's
*primary* endpoint may request it. Requesting the same address on two Services
leaves whichever load balancer loses the race pending forever.

| Field | `Public` | `PublicAndPrivate` | `Private` |
| --- | --- | --- | --- |
| `serviceAnnotations` | `kube-apiserver` | `kube-apiserver` (removed from `kube-apiserver-private`) | `kube-apiserver-private` |
| `loadBalancerClass` | every LoadBalancer Service | every LoadBalancer Service | every LoadBalancer Service |

Because `endpointAccess` is mutable, the keys are actively *removed* from the
non-primary Service rather than merely not written, so a `Private` to
`PublicAndPrivate` transition hands the address over to the public Service instead
of having both request it.

`loadBalancerClass` selects the provisioning controller rather than an address, so
every LoadBalancer Service needs it. Kubernetes only accepts the field on create,
or when a Service becomes a LoadBalancer, and rejects it on non-LoadBalancer
Services; CPO sets it accordingly and never clears it.

#### Provider examples

The annotation map covers all known LB providers without any HyperShift code changes:

| Provider | `loadBalancerClass` | `serviceAnnotations` |
|----------|-------------------|---------------------|
| MetalLB | `metallb.io/metallb` | `metallb.io/loadBalancerIPs: "10.1.1.100"`, `metallb.io/address-pool: "prod"` |
| AWS NLB | (not needed — single provider) | `service.beta.kubernetes.io/aws-load-balancer-eip-allocations: "eipalloc-abc"` |
| Azure LB | (not needed) | `service.beta.kubernetes.io/azure-load-balancer-ipv4: "20.0.0.100"` |
| GCP | (not needed) | `networking.gke.io/load-balancer-ip-addresses: "my-ip"` |
| F5 CIS | `f5.com/f5-bigip` | `cis.f5.com/ip: "10.2.2.50"`, `cis.f5.com/ipamLabel: "prod"` |
| Cilium LB IPAM | `io.cilium/bgp-control-plane` | `lbipam.cilium.io/ips: "10.3.3.50"` |
| OpenStack | (not needed) | `loadbalancer.openstack.org/floating-network-id: "<uuid>"` |

Adding a new provider requires adding its prefix to the CEL allowlist —
a one-line type change requiring a CRD bump.

#### Controller Coordination

Existing communication channels are reused:
- **CPO → HO**: HCP status conditions mirrored to HC (`hostedcluster_controller.go:783-893`)
- **HO → CPO**: HC spec mirrored to HCP via the standard spec mirror in the
  HostedCluster controller. `spec.services` flows directly — no annotation relay
  needed.

No new annotations or relay entries needed.

#### Platform Independence

The LB configuration is inside `spec.services`, not inside platform substructs.
This reflects the reality that the LB provider is a management-cluster property,
orthogonal to the guest cluster platform:

- Agent platform on AWS management cluster → AWS NLB processes the KAS Service
- Agent platform on bare-metal with MetalLB → MetalLB processes the KAS Service
- AWS platform on AWS management cluster → AWS NLB (same provider, different reason)

The CCMs in the HCP namespace are **guest cluster CCMs** — they manage guest
resources. The KAS Service lives in the HCP namespace and is processed by
whatever LB controller runs on the management cluster itself.

### Risks and Mitigations

| Risk | Mitigation |
|------|-----------|
| **User provides invalid annotations** | LB provider ignores unknown annotations. Service stays Pending. Existing conditions surface the failure. Documented. |
| **Reconciliation stripping user annotations** | Confirmed per-key writes at `kas/service.go:81, 86, 98`. Unit tests will verify annotation preservation. |
| **Unknown provider needed** | Adding a provider prefix to the allowlist is a one-line CEL change with E2E test and CRD bump. Turnaround is one release cycle. |
| **No type safety on annotations** | Deliberate tradeoff. Allowlist constrains which providers are accepted; annotation value correctness is the user's responsibility — same as setting annotations directly on any K8s Service. |
| **Misspelled annotation key within allowed prefix** | A key like `metallb.io/loadBalancerIP` (missing trailing `s`) passes the allowlist but MetalLB ignores it — Service stays Pending with no error. Mitigated: documented per-provider annotation examples; operator can compare Service annotations against provider docs. No programmatic mitigation — HyperShift does not validate provider-specific key names. |
| **No controller for `loadBalancerClass`** | If the user specifies a `loadBalancerClass` with no matching controller on the management cluster, the Service stays Pending indefinitely. Mitigated: same failure mode as any Kubernetes Service with an unmatched `loadBalancerClass` — surfaced via existing `Available` condition on HC. |

### Drawbacks

- No type safety or `oc explain` discoverability for provider-specific fields.
  Users must know their LB provider's annotation schema. The allowlist ensures
  only known providers are accepted by default.
- Adding a new LB provider to the allowlist requires a CRD bump (one-line
  type change). This is a deliberate tradeoff for safety over convenience.

## Alternatives (Not Implemented)

### 1. Per-Provider Typed Structs

Provider-specific typed fields inside platform substructs (e.g.,
`AWSLoadBalancerConfig.EIPAllocations`). Discarded:
- Makes HyperShift maintainer of every vendor annotation schema.
- Every new LB provider requires an API change + CRD bump + release cycle.
- Conflates LB provider with guest platform — an Agent HC on AWS uses AWS NLB,
  not MetalLB.
- Internally contradictory when combined with an "unsupported annotation" escape
  hatch (typed structs for safety + untyped blob for everything else).

### 2. Pure HC Annotation (no API field)

Use only HC annotations for Day 1. No API type changes. Discarded: no
OpenAPI validation, no `oc explain` discoverability, fragile JSON strings, no
structured diff. RFE-7201 explicitly proposes API fields.

### 3. Root-Level spec.loadBalancerConfig

A new root-level struct on `HostedClusterSpec`. Discarded:
- Avoids the `spec.services` CEL rewrite but creates API debt if LB config is
  needed per-service in the future.
- Forces a `map[ServiceType]LoadBalancerConfig` or multiple root-level fields
  to support per-service configuration.
- `spec.services` is the natural home for service publishing configuration.

### 4. Automatic MGMT Cluster LB Discovery

HO uses `DetectManagementClusterCapabilities()` to auto-detect the LB provider
and select defaults. Investigated but deferred: the detection mechanism exists
(`support/capabilities/management_cluster_capabilities.go`) and adding CRD checks
is a 1-file change, but auto-detection introduces complexity around multi-provider
clusters and default selection. Can be added as an enhancement on top of this
proposal.

### 5. Typed `loadBalancerIPs` + `addressPool` Fields

Typed fields (`loadBalancerIPs []string`, `addressPool string`) with per-provider
translation in CPO. Discarded:
- `spec.loadBalancerIP` is deprecated since Kubernetes 1.24 and will be removed.
  There is no common standard for IP assignment across LB providers.
- Each provider uses different annotation keys for the same concept (MetalLB:
  `metallb.io/loadBalancerIPs`, AWS: EIP allocations, Azure: `ipv4`/`ipv6`).
  A translation layer in CPO would need to understand every provider's schema —
  the same maintenance burden as per-provider typed structs (Alternative 1).
- The annotation passthrough approach lets each provider define its own semantics
  without HyperShift acting as a translation broker.

## Future Work

### Day 2 IP Migration ([OCPSTRAT-3926](https://issues.redhat.com/browse/OCPSTRAT-3926))

Safe runtime IP address changes on running Hosted Control Planes without
requiring pod restarts or node replacement. Includes lifting the
`serviceAnnotations` immutability restriction. Separate scope.

### Automatic MGMT Cluster LB Discovery

HO's `DetectManagementClusterCapabilities()` can be extended to auto-detect MetalLB,
F5 CIS, Cilium, etc. via CRD checks. This would provide UX improvements (default
`loadBalancerClass`, validation hints) but is not required for the annotation
passthrough approach.

## Open Questions

1. Should `loadBalancerClass` immutability be enforced at the HC level (CEL) or
   inherited from the Kubernetes Service semantics (fail at Service creation)?
   Recommendation: enforce at HC level via CEL for clearer UX.

## Test Plan

### Unit Tests

- `ReconcileService()` passes `serviceAnnotations` per-key to KAS Service.
- `ReconcileService()` sets `loadBalancerClass` on KAS Service.
- CEL allowlist rejects annotations from unknown providers.
- Existing `spec.services` immutability rule still blocks changes after creation.
- Annotation preservation: external annotations survive reconciliation loops.

### E2E Tests

- Day 1: Create HC with MetalLB annotations in `serviceAnnotations`, verify
  annotations on KAS Service, verify MetalLB assigns expected IP (requires
  MetalLB in CI).
- `loadBalancerClass`: Create HC with `loadBalancerClass`, verify it's set on
  KAS Service and the correct LB controller processes it.

### Negative E2E Tests

- IP already in use: Create HC requesting an IP that is already assigned to
  another Service. Verify Service stays Pending and HC condition surfaces
  the failure.
- Invalid `loadBalancerClass`: Create HC with a `loadBalancerClass` that has
  no matching controller. Verify Service stays Pending.
- Unknown annotation prefix: Create HC with an annotation key not in the
  allowlist. Verify the CEL rule rejects the request at admission.

### Provider Validation (One-Time, Not CI)

- F5 BIG-IP VE Trial VM for end-to-end proof with F5 CIS annotations.
- Cilium LB IPAM with `lbipam.cilium.io/ips` annotation.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Day 1 annotation passthrough functional.
- `loadBalancerClass` support functional.
- Unit test coverage for annotation passthrough and CEL allowlist validation.
- Basic E2E test with MetalLB (if available in CI).
- `DeterministicLBIPSupported` condition reported by HO. The skew it detects is a
  Day 1 failure mode that exists from the first TechPreview release, so it is a
  TechPreview requirement rather than a GA one.

### Tech Preview -> GA

- CEL allowlist validation.
- Load testing with multiple concurrent HostedClusters.
- User-facing documentation in openshift-docs with per-provider examples.
- F5 one-time validation completed.
- Minimum soak time at TechPreview without regressions.

### GA

- IP assignment latency observable via `Service.Status.LoadBalancer.Ingress`
  population time.
- All E2E tests passing in CI.

### Removing a deprecated feature

Not applicable — this is a new feature.

## Upgrade / Downgrade Strategy

**Upgrade**: Existing clusters without `serviceAnnotations` or `loadBalancerClass`
continue to work identically. The new fields are optional and have no default
behavior when unset. The existing `spec.services` immutability rule is unchanged.
No data migration needed.

**Downgrade**: If a cluster was created with `serviceAnnotations`, downgrading
HO has two effects:

1. **CRD pruning**: The downgraded CRD no longer defines `loadBalancerClass` or
   `serviceAnnotations`. The API server prunes these fields from stored
   HostedCluster objects on the next write. The values are lost — they cannot
   be recovered by re-upgrading HO. Users must re-create the HC to restore
   the fields.
2. **Service annotations persist**: Annotations already written to the KAS
   Service by CPO are Kubernetes-native — they survive the operator downgrade.
   The LB provider continues to serve the configured IP.
3. **CPO must never clear `loadBalancerClass`**: On downgrade, old CPO does not
   know about `loadBalancerClass`. Since CPO never sets
   `Service.spec.loadBalancerClass` to empty (it only sets it when the field
   is present), the existing value on the Service is preserved. This is the
   correct behavior — clearing it would cause the default LB controller to
   re-provision the Service.

## Version Skew Strategy

The feature requires **both HO and CPO at the minimum version** (the release
that introduces `serviceAnnotations` and `loadBalancerClass`). HO always
upgrades before CPO (HO deploys CPO), so the only version skew scenario is:

- **New HO + Old CPO**: HO mirrors new fields to HCP correctly. Old CPO ignores
  unknown fields. No annotations are set on the Service. The user specified an
  IP or LB configuration that is **silently not honored**. This is not a
  transient upgrade window — the CPO version is pinned to the HC's release
  image, so a cluster on an older release runs the old CPO indefinitely until
  the HC itself is upgraded.

  Detection must use CPO image labels, not Service annotation comparison —
  the absence of annotations on the Service could be transient (CPO hasn't
  reconciled yet). The new CPO image adds a label
  `io.openshift.hypershift.control-plane-operator-supports-deterministic-lb`.
  HO reads it via `GetControlPlaneOperatorImageLabels()` (the same mechanism
  used for `supports-kas-custom-kubeconfig`, `restricted-psa`, etc.). When
  `serviceAnnotations` or `loadBalancerClass` are present on HC but the CPO
  image label is absent, HO sets the dedicated condition
  `DeterministicLBIPSupported=False` with reason `UnsupportedControlPlaneOperator`
  and a message directing the user to upgrade the HC release. The condition is
  informational: reconciliation continues with the fields ignored, and the
  condition clears by itself once the CPO image is upgraded.
- **Both at new version**: Feature fully functional.

### Why a dedicated condition instead of `ValidConfiguration`

An earlier revision of this enhancement specified `ValidConfiguration=False` for
this case. That does not work as a gate, and it is the wrong severity:

- The skew check can only run once the CPO image labels have been resolved, which
  happens in `CPOImageAndNamespace`. That is *after* the point where the
  reconciler blocks on `ValidConfiguration`. On the next reconcile the condition
  is recomputed from scratch by `validateConfigAndClusterCapabilities`, which has
  no access to image labels, so it is `True` again by the time the block is
  evaluated. The condition would therefore flap between `True` and `False` on
  every reconcile and never actually block anything.
- Making it block would require reporting the check as a critical step, which
  halts the rest of control plane reconciliation (the pattern used by
  `ManagedHSMVersionCheck`). That is disproportionate here: these are optional
  fields whose only failure mode is being silently ignored, and halting the whole
  control plane over them would be a far worse outcome than an unhonored address.

A dedicated, non-blocking condition surfaces the skew to the user and to
monitoring without either of those problems.

## Operational Aspects of API Extensions

- **New CRDs**: None. This modifies fields on existing `HostedCluster` and
  `HostedControlPlane` types.
- **Webhooks**: No new webhooks. All validation via CEL on CRD.
- **SLIs**: IP assignment success can be monitored via `Service.Status.LoadBalancer.Ingress`
  population time.
- **Failure mode**: If LB provider fails to assign requested IP (e.g., IP already
  in use, pool exhausted, invalid annotation), the Service stays in Pending state.
  Existing `ExternalDNSReady` and `Available` conditions on HC surface this. No
  new failure mode introduced.
- **Escalation**: HyperShift team (HCP) for reconciler issues. LB provider team
  for provider-specific IP assignment failures.

## Support Procedures

- **Detect**: If `Service.Status.LoadBalancer.Ingress` is empty after expected time,
  check Service annotations match provider expectations. Check LB provider logs for
  assignment errors.
- **Diagnose**: `oc get svc kube-apiserver -n <hcp-namespace> -o yaml` — verify
  annotations are correct and match the LB provider's expected format.
- **Disable**: not available during TechPreview, and deliberately so. Two things
  prevent it:
  - `spec.services` is immutable on every platform except IBMCloud
    (`self.platform.type != "IBMCloud" ? self.services == oldSelf.services : true`),
    so the fields cannot be removed from an existing HostedCluster at all.
  - Even where removal is possible, CPO does not clean up annotations on a spec
    change. The keys would stay on the Service and the provider would keep the
    static address instead of falling back to a dynamic one.

  The supported way to stop using a requested address during TechPreview is to
  recreate the HostedCluster without the fields. A real disable procedure belongs
  to Day 2, where `serviceAnnotations` becomes mutable and the sidecar mechanism
  exists to drain connections; it is specified there, not here.

## Infrastructure Needed

- MetalLB availability in CI for E2E testing (TBD — may require CI team coordination).
- F5 BIG-IP VE Trial license for one-time validation (free, time-limited).
