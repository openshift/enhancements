---
title: managed-ingress-dns-for-aws-hosted-control-planes
authors:
  - "@typeid"
reviewers:
  - "@csrwng"
  - "@enxebre"
  - "@joshbranham"
approvers:
  - "@csrwng"
  - "@enxebre"
api-approvers:
  - "@enxebre"
  - "@csrwng"
creation-date: 2026-08-14
last-updated: 2026-08-20
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/RFE-9235
see-also:
  - "/enhancements/hypershift/hypershift-gcp-platform-support.md"
---

# Managed Ingress DNS for AWS Hosted Control Planes

## Summary

Enable the HyperShift Control Plane Operator (CPO) to create and reconcile Route53 DNS zones for AWS hosted control planes, removing the requirement that all DNS zones be pre-created externally. This is an opt-in feature controlled by a new `managedDNS` field on `AWSPlatformSpec`, gated by the `AWSManagedDNS` feature gate, with no behavioral change for existing clusters. In shared VPC scenarios, the scope is limited to the public ingress zone (created in the cluster account) — the private ingress zone and `.hypershift.local` zone must be pre-created by the VPC owner.

## Motivation

Today, deploying an AWS hosted control plane requires three Route53 hosted zones to be pre-created externally and passed as zone IDs in the HostedCluster spec:

1. A private `.hypershift.local` zone for internal VPC endpoint connectivity
2. A public ingress zone for customer-facing DNS records and certificate validation
3. A private ingress zone for VPC-internal DNS resolution

On Azure and GCP, the CPO already creates the `.hypershift.local` zone itself. On GCP, the CPO also creates all three zone types plus ACME DNS01 challenge delegation records (see [Implementation Details](#implementation-detailsnotesconstraints) for a detailed explanation of the ACME challenge delegation pattern). AWS is the only platform that requires all zones to be pre-created externally.

This is a gap for the ROSA HyperFleet architecture, where the goal is to minimize customer account access from the cluster lifecycle component. HyperShift already holds customer credentials and manages customer-account infrastructure (VPC endpoints, security groups, Route53 records) — owning the zones those records live in is a natural extension. Today, zone creation and record management are split across different systems, making it harder to reason about failures and lifecycle.

### User Stories

- As a ROSA HyperFleet operator, I want HyperShift to manage customer-account DNS zones so that customers get a working ingress and console with a valid certificate on day 1, without requiring a separate component with customer AWS credentials for DNS management.

- As an SRE, I want DNS zone creation failures surfaced as status conditions on the HostedCluster so that I can quickly diagnose ingress DNS and certificate issues.

### Goals

- Enable opt-in managed DNS zone creation on AWS via `spec.platform.aws.managedDNS`, covering the `.hypershift.local` zone and public/private ingress zones for standard clusters, and the public ingress zone only for shared VPC clusters (without `managedDNS`, current behavior is unchanged)
- Support configurable service-side DNS delegation (ACME CNAME + NS records) for certificate generation
- Surface created zone IDs and nameservers in HostedCluster status
- Provide a status condition for DNS zone health to aid troubleshooting
- Maintain full backwards compatibility with existing ROSA clusters

### Non-Goals

- Base domain allocation (multi-tenant concern handled by the consuming platform)
- Modifying existing behavior for clusters that do not set `managedDNS`

## Proposal

This enhancement introduces two changes to the CPO, both gated behind `spec.platform.aws.managedDNS`:

1. **Auto-create `.hypershift.local` zone** when the zone does not already exist, in the PrivateLink controller (see [Change 1](#change-1-auto-create-hypershiftlocal-zone-gated-by-manageddns)).
2. **Create and reconcile public/private ingress DNS zones** with optional ACME delegation and NS record management, in the HCP reconciler (see [Change 2](#change-2-managed-ingress-dns-zones-gated-by-manageddns)).

### Workflow Description

**cluster lifecycle component** (e.g., ROSA HyperFleet's hyperfleet-operator or ROSA HCP's clusters service) is the platform service responsible for creating and managing HostedClusters.

**cluster administrator** is the human operator who monitors cluster health and troubleshoots issues.

1. The cluster lifecycle component creates a HostedCluster CR on the Management Cluster with `spec.platform.aws.managedDNS` configured and a `BaseDomain`.
2. The HyperShift hypershift-operator deep-copies `spec.platform` to the HostedControlPlane. The CPO reconciles the HCP and detects the `managedDNS` configuration.
3. The CPO creates a `.hypershift.local` private Route53 zone associated with the guest cluster's VPC (if the zone does not already exist — see Change 1). Shared VPC: see Change 1 for scoping differences.
4. The CPO creates public and private ingress Route53 zones using `{managedDNS.ingressDomainPrefix}.{baseDomainPrefix}.{baseDomain}` as the zone domain name (default prefix: `in`). The private ingress zone is associated with the guest cluster's VPC so that in-cluster DNS resolution works. Shared VPC: only the public ingress zone is created (see Change 1).
5. If delegation is configured, the CPO creates an ACME DNS01 challenge delegation CNAME record in the public ingress zone.
6. If `delegation.nsDelegation` is `ExternalDNS`, the CPO creates a `DNSEndpoint` CR (external-dns CRD, owned by the HCP via `SetControllerReference`) in the control plane namespace containing NS delegation records for the public ingress zone. external-dns picks up the CR and creates the NS records in the parent zone. If `nsDelegation` is `Manual`, the CPO skips `DNSEndpoint` creation — the consuming platform handles NS delegation using nameservers from HostedCluster status.
7. The CPO populates `hcp.Status.Platform.AWS.DNSZones` with zone IDs and nameservers. The hypershift-operator propagates this to `hcluster.Status.Platform.AWS.DNSZones` via the existing `hcp.Status.Platform → hcluster.Status.Platform` copy.
8. The CPO sets the `AWSManagedDNSAvailable` condition on the HostedControlPlane. If delegation is configured, the CPO performs a live `net.LookupNS()` check and keeps the condition `False` with reason `NSDelegationPending` until NS delegation resolves. Once confirmed (or if no delegation is configured), the condition is set to `True`. The hypershift-operator propagates this condition to the HostedCluster.
9. The CPO overrides `dns.Spec.PublicZone` and `dns.Spec.PrivateZone` in the guest cluster's `dns.config` with the managed ingress zone IDs, so the OpenShift ingress operator creates wildcard DNS records (`*.apps`) in the managed zones.

If ingress DNS zone creation fails (steps 4–8), the `AWSManagedDNSAvailable` condition is set to `False` with the error message. The cluster administrator can check this condition on the HostedCluster to diagnose and resolve issues (e.g., missing IAM permissions). The controller retries on the next reconciliation cycle, throttled to avoid Route53 rate limits (see Rate Limiting section). `.hypershift.local` zone failures (step 3) are reported through the existing `AWSEndpointAvailable` condition on the `AWSEndpointService` resource.

### Change 1: Auto-create `.hypershift.local` zone (gated by `managedDNS`)

Every `Private` or `PublicAndPrivate` AWS hosted control plane requires a `<clusterName>.hypershift.local` private Route53 zone associated with the guest cluster's VPC for internal endpoint DNS resolution.

When `spec.platform.aws.managedDNS` is set, the PrivateLink controller auto-creates this zone if it does not already exist. The controller discovers the zone through an in-memory cache, the persisted `AWSEndpointServiceStatus.DNSZoneID`, and a `ListHostedZones` lookup. If none of these yields an existing zone, the controller creates one. Without `managedDNS`, the zone must be pre-created externally (current behavior, unchanged).

In shared VPC scenarios, the VPC owner's account operates under a more restrictive permission model — the shared VPC IAM policies are intentionally scoped to the minimum required operations. Extending these policies with zone-management permissions (`CreateHostedZone`, `DeleteHostedZone`) would widen that trust boundary. Therefore, in shared VPC scenarios the `.hypershift.local` zone and the private ingress zone must be pre-created by the VPC owner, as they are today. `SharedVPC.LocalZoneID` remains required even when `managedDNS` is set. The scope of `managedDNS` in shared VPC is limited to the public ingress zone, which is created in the cluster account using the CPO's own credentials (not the `SharedVPC.RolesRef.IngressARN` role), along with the ACME challenge CNAME and NS delegation.

The controller tracks zones it created via `AWSEndpointServiceStatus.ManagedLocalZone` (set to `"Managed"`) and only deletes zones carrying this marker during cleanup. Gating auto-create behind `managedDNS` is pragmatic: any consumer that wants managed ingress DNS also wants managed local zones, and it avoids adding new IAM requirements to clusters that do not opt in.

### Change 2: Managed ingress DNS zones (gated by `managedDNS`)

When `spec.platform.aws.managedDNS` is set on the HostedCluster, the CPO creates and reconciles public and private Route53 ingress zones in the customer's AWS account (shared VPC: public only — see Change 1).

**Without delegation** (`managedDNS.delegation` is nil): The CPO creates zones using `{ingressDomainPrefix}.{baseDomainPrefix}.{baseDomain}` as the zone name (default prefix `in`). No ACME CNAME or DNSEndpoint is created. The consuming platform is responsible for all delegation and certificate management.

**With delegation** (`managedDNS.delegation` is set): The ingress zones use `managedDNS.ingressDomainPrefix` on the cluster domain (`{ingressDomainPrefix}.{baseDomainPrefix}.{baseDomain}`, default prefix `in`), keeping the cluster domain itself in the parent zone. The prefix creates a DNS delegation boundary that allows the service provider to complete ACME DNS01 challenges and provision TLS certificates for the customer's ingress domain without needing write access to the customer's zone (see [Implementation Details](#implementation-detailsnotesconstraints) for a detailed explanation).

The controller creates an ACME DNS01 challenge delegation CNAME record in the public ingress zone, enabling service-side certificate generation for the customer-delegated zone.

For NS delegation, the `nsDelegation` field controls how NS records are created in the parent zone:

- **`ExternalDNS`**: The CPO creates a `DNSEndpoint` CR in the control plane namespace. external-dns must be configured with `--source=crd` to watch for `DNSEndpoint` resources. The `DNSEndpoint` CRD (`externaldns.k8s.io/v1alpha1`) is bundled with `hypershift install` and installed on all management clusters. external-dns itself is only deployed when `--external-dns-provider` is set (AWS or GCP). If the CRD is missing (e.g., manual deletion), the `AWSManagedDNSAvailable` condition is set to `False` with an error message indicating the CRD is unavailable.
- **`Manual`**: The CPO skips `DNSEndpoint` creation. The consuming platform reads nameservers from `hcluster.Status.Platform.AWS.DNSZones` and creates NS delegation records in the parent zone. The consuming platform must verify NS delegation is complete before initiating ACME certificate challenges.

### API Extensions

All changes are additive and non-breaking. New spec fields are gated by the `AWSManagedDNS` feature gate (see Graduation Criteria).

**New spec types** in `api/hypershift/v1beta1/aws.go`:

```go
// AWSManagedDNSSpec configures CPO-managed Route53 DNS zones.
// When set, the CPO creates the .hypershift.local private zone and
// public/private ingress Route53 zones using ingressDomainPrefix to form
// the zone domain name. Delegation (ACME CNAME + NS records) is configured
// separately via the delegation field.
// +openshift:enable:FeatureGate=AWSManagedDNS
type AWSManagedDNSSpec struct {
    // ingressDomainPrefix is the subdomain prefix for ingress DNS zones.
    // Zones are created as {prefix}.{baseDomainPrefix}.{baseDomain}.
    // When delegation is configured, the prefix creates a DNS delegation boundary
    // that separates the ingress zone from the cluster domain, enabling ACME
    // challenge CNAME delegation back to the parent zone.
    // +optional
    // +default="in"
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=63
    // +kubebuilder:validation:Pattern=`^[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
    IngressDomainPrefix string `json:"ingressDomainPrefix,omitempty"`

    // delegation configures service-side DNS delegation for certificate generation.
    // When set, the CPO creates an ACME DNS01 challenge CNAME in the public ingress
    // zone and handles NS delegation based on the nsDelegation mode.
    // When absent (zero value), only zones are created and the consuming platform handles
    // delegation and certificate management.
    // +optional
    Delegation AWSManagedDNSDelegationSpec `json:"delegation,omitzero,omitempty"`
}

// AWSManagedDNSDelegationSpec configures service-side DNS delegation for
// certificate generation. When set, the CPO creates an ACME DNS01 challenge
// CNAME in the public ingress zone pointing back to the parent zone, and
// handles NS delegation based on the nsDelegation mode.
type AWSManagedDNSDelegationSpec struct {
    // nsDelegation specifies how NS delegation records are created in the parent zone.
    // "ExternalDNS": the CPO creates a DNSEndpoint CR in the control plane namespace;
    // external-dns creates NS records in the parent zone.
    // "Manual": the consuming platform handles NS delegation using nameservers
    // reported in HostedCluster status.
    // +required
    // +kubebuilder:validation:Enum=ExternalDNS;Manual
    NSDelegation NSDelegationMode `json:"nsDelegation,omitempty"`
}

// NSDelegationMode specifies how NS delegation is performed.
// +kubebuilder:validation:Enum=ExternalDNS;Manual
type NSDelegationMode string

const (
    NSDelegationExternalDNS NSDelegationMode = "ExternalDNS"
    NSDelegationManual      NSDelegationMode = "Manual"
)
```

**Extended `AWSPlatformSpec`:**

```go
type AWSPlatformSpec struct {
    // ... existing fields ...

    // managedDNS configures CPO-managed Route53 DNS zones for this cluster.
    // +optional
    // +openshift:enable:FeatureGate=AWSManagedDNS
    ManagedDNS *AWSManagedDNSSpec `json:"managedDNS,omitempty"`
}
```

**New status type** in `api/hypershift/v1beta1/aws.go`:

```go
// AWSDNSZoneType defines the purpose of a managed DNS zone.
// +kubebuilder:validation:Enum=PublicIngress;PrivateIngress
type AWSDNSZoneType string

const (
    PublicIngressZone  AWSDNSZoneType = "PublicIngress"
    PrivateIngressZone AWSDNSZoneType = "PrivateIngress"
)

type AWSDNSZoneStatus struct {
    // zoneID is the Route53 hosted zone ID.
    // +required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=32
    ZoneID string `json:"zoneID,omitempty"`

    // zoneType indicates the purpose of the zone.
    // +required
    ZoneType AWSDNSZoneType `json:"zoneType,omitempty"`

    // name is the DNS name of the hosted zone.
    // +required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=253
    Name string `json:"name,omitempty"`

    // nameServers are the authoritative name servers for this zone.
    // Used for NS delegation when nsDelegation is Manual.
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:MaxItems=10
    NameServers []string `json:"nameServers,omitempty"`
}
```

**Mutual exclusivity with `DNSSpec` zone IDs:**

`managedDNS` and the existing `DNSSpec.publicZoneID` / `DNSSpec.privateZoneID` fields both feed into the guest cluster's `dns.config.PublicZone` / `PrivateZone`. When `managedDNS` is set, the CPO creates the ingress zones and overrides `dns.config` with the managed zone IDs, making any user-provided zone IDs for the same purpose redundant. The following CEL rules enforce mutual exclusivity at admission time:

- `managedDNS` is always mutually exclusive with `publicZoneID`: the CPO always creates the public ingress zone when `managedDNS` is set (including shared VPC).
- `managedDNS` without `SharedVPC` is mutually exclusive with `privateZoneID`: the CPO creates the private ingress zone for standard clusters.
- `managedDNS` with `SharedVPC` allows `privateZoneID`: the VPC owner pre-creates the private ingress zone and provides the zone ID via `privateZoneID`. The CPO only manages the public zone.

```go
// On HostedClusterSpec (cross-field validation):
// +kubebuilder:validation:XValidation:rule="!has(self.platform.aws) || !has(self.platform.aws.managedDNS) || self.dns.publicZoneID == ''",message="publicZoneID and managedDNS are mutually exclusive: managedDNS creates the public ingress zone"
// +kubebuilder:validation:XValidation:rule="!has(self.platform.aws) || !has(self.platform.aws.managedDNS) || has(self.platform.aws.sharedVPC) || self.dns.privateZoneID == ''",message="privateZoneID and managedDNS are mutually exclusive for non-shared-VPC clusters: managedDNS creates the private ingress zone"
```

The `.hypershift.local` zone has no conflict: `SharedVPC.LocalZoneID` is always required when SharedVPC is set (regardless of `managedDNS`), and without SharedVPC the PrivateLink controller auto-creates the local zone with no user-provided zone ID to conflict with.

**Status ownership:** The `.hypershift.local` zone is tracked exclusively in `AWSEndpointServiceStatus.DNSZoneID` (managed by the PrivateLink controller). The `hcp.Status.Platform.AWS.DNSZones` array contains only ingress zones (managed by the HCP reconciler) to maintain single-writer ownership per status field. When `managedDNS` is set on a standard cluster, `PublicIngress` and `PrivateIngress` appear in `DNSZones`. In shared VPC scenarios, only `PublicIngress` appears (see Change 1).

**Extended `AWSPlatformStatus`:**

```go
type AWSPlatformStatus struct {
    DefaultWorkerSecurityGroupID string              `json:"defaultWorkerSecurityGroupID,omitempty"`
    // dnsZones contains DNS zone information for zones managed by the
    // control plane operator. This is the source of truth for zone IDs
    // used during cleanup and for consuming platform NS delegation.
    // +optional
    // +listType=map
    // +listMapKey=zoneType
    // +kubebuilder:validation:MaxItems=2
    DNSZones []AWSDNSZoneStatus `json:"dnsZones,omitempty"`
}
```

**New condition type:**

```go
// AWSManagedDNSAvailable indicates whether the managed DNS configuration
// has been successfully created. Only set when spec.platform.aws.managedDNS
// is configured.
AWSManagedDNSAvailable ConditionType = "AWSManagedDNSAvailable"
```

Status propagation: the CPO sets `hcp.Status.Platform.AWS.DNSZones` on the HostedControlPlane. The hypershift-operator already deep-copies `hcp.Status.Platform` to `hcluster.Status.Platform` and propagates conditions from HCP to HC, so no additional propagation logic is needed.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specifically for the HyperShift/HCP topology. It only affects the AWS platform. GCP already creates all three zone types (`.hypershift.local`, public ingress, private ingress) in the CPO. Azure creates the `.hypershift.local` zone but not ingress zones. The change runs within the CPO (per-HCP deployment in the control plane namespace on the management cluster). The CPO's Role in the control plane namespace includes RBAC for `externaldns.k8s.io/dnsendpoints` so the CPO can create `DNSEndpoint` CRs when `nsDelegation` is `ExternalDNS`. The RBAC is always present and is a no-op when not used.

#### Standalone Clusters

Not applicable. This feature is specific to hosted control planes.

#### Single-node Deployments or MicroShift

Not applicable.

#### OpenShift Kubernetes Engine

Not applicable. This feature does not depend on OKE-excluded capabilities.

### Implementation Details/Notes/Constraints

**Why the ingress domain prefix creates a delegation boundary (ACME DNS01 challenge delegation):**

The ingress zones are created with a configurable prefix (default `in`) on the cluster domain: `{prefix}.{baseDomainPrefix}.{baseDomain}`. This prefix is not just a naming convention — it creates a DNS delegation boundary that is essential for the ACME challenge pattern to work.

The DNS topology has two zones in different accounts:

1. **Parent zone** (service provider account): Contains `api.`, `oauth.`, `_acme-challenge.` records for the cluster, plus an NS record delegating `{prefix}.{baseDomainPrefix}.{baseDomain}` to the customer's zone.
2. **Customer zone** (`{prefix}.{baseDomainPrefix}.{baseDomain}`): Contains `*.apps.` wildcard records plus an ACME challenge CNAME.

The CNAME in the customer zone delegates ACME DNS01 challenges back to the parent zone:

- From: `_acme-challenge.apps.{prefix}.{baseDomainPrefix}.{baseDomain}` (in customer zone)
- To: `_acme-challenge.{baseDomainPrefix}.{baseDomain}` (in parent zone — no prefix)

This works because the NS delegation boundary is at `{prefix}.{baseDomainPrefix}.{baseDomain}`. The CNAME target (without the prefix) falls outside this boundary and resolves in the parent zone, where the service provider's ACME solver writes TXT records. Without the prefix, both the ACME source and target would be in the same zone, making the CNAME delegation impossible.

**Example (ROSA HyperFleet):**

Given a cluster with `BaseDomainPrefix` = `mycluster` and `BaseDomain` = `a1b2.0.us-east-1.rosa.openshiftapps.com`:

```text
Zone shard (service provider account):
  api.mycluster.a1b2.0.us-east-1.rosa.openshiftapps.com                     (A/CNAME)
  oauth.mycluster.a1b2.0.us-east-1.rosa.openshiftapps.com                   (A/CNAME)
  _acme-challenge.mycluster.a1b2.0.us-east-1.rosa.openshiftapps.com         (TXT)
  in.mycluster.a1b2.0.us-east-1.rosa.openshiftapps.com                      (NS → customer zone)

Customer public ingress zone (in.mycluster.a1b2.0.us-east-1.rosa.openshiftapps.com):
  *.apps.in.mycluster.a1b2.0.us-east-1.rosa.openshiftapps.com               (A → ingress LB)
  _acme-challenge.apps.in.mycluster.a1b2.0.us-east-1.rosa.openshiftapps.com
      → CNAME → _acme-challenge.mycluster.a1b2.0.us-east-1.rosa.openshiftapps.com
```

The CNAME target has no `in.` prefix, so it resolves in the zone shard — outside the NS delegation. The consuming platform writes TXT records there for ACME validation.

**Resource tagging:**

All managed Route53 zones are tagged with `hcp.Spec.Platform.AWS.ResourceTags` via `ChangeTagsForResource` after creation. This is consistent with how the PrivateLink controller tags VPC endpoints and security groups. Tagging failures are logged but do not block zone creation.

**dns.config population:**

When `managedDNS` is set, the CPO overrides `dns.Spec.PublicZone` and `dns.Spec.PrivateZone` in the guest cluster's `dns.config` with the managed ingress zone IDs from `hcp.Status.Platform.AWS.DNSZones`. This is how the OpenShift ingress operator discovers the correct zones and creates wildcard DNS records (`*.apps`) for ingress. Without this override, the ingress operator would not know about the managed zones. The CPO only populates `dns.Spec.PublicZone` and `dns.Spec.PrivateZone` after the managed ingress zone IDs are available in `hcp.Status.Platform.AWS.DNSZones`. If the zone IDs are not yet populated (zones still being created), the override is a no-op and the ingress operator does not receive zone configuration until the next successful reconciliation. This ensures the ingress operator does not attempt to create records in zones that do not yet exist (shared VPC: only `PublicZone` is overridden — see Change 1).

**NS delegation verification:**

When delegation is configured, the CPO performs a live `net.LookupNS()` check for the ingress zone domain to verify that NS delegation is active. Until delegation resolves, `AWSManagedDNSAvailable` remains `False` with reason `NSDelegationPending`. This provides actionable status — the consuming platform can distinguish between "zones created, waiting for delegation" and "zone creation failed."

**IAM permissions:**

The CPO needs additional Route53 permissions beyond its current set. The existing `ROSAControlPlaneOperatorPolicy` already grants `route53:ListHostedZones`, `route53:ListResourceRecordSets`, and `route53:ChangeResourceRecordSets` (scoped to `*.hypershift.local`). New permissions required:

| Action                             | Notes                                            |
| ---------------------------------- | ------------------------------------------------ |
| `route53:CreateHostedZone`         | Zone creation                                    |
| `route53:GetHostedZone`            | Zone verification and NS record retrieval        |
| `route53:DeleteHostedZone`         | Zone cleanup on cluster deletion                 |
| `route53:ChangeTagsForResource`    | Tagging zones with `ResourceTags`                |

Additionally, the existing `route53:ChangeResourceRecordSets` statement is scoped to `*.hypershift.local` record names. This must be widened to also cover ACME CNAME records in the ingress zone and record cleanup before zone deletion.

The following managed policies must be updated to include these permissions:

- **`ROSAControlPlaneOperatorPolicy`** ([managed-cluster-config](https://github.com/openshift/managed-cluster-config/blob/master/resources/sts/hypershift/openshift_hcp_control_plane_operator_credentials_policy.json)): Updated for all ROSA HCP clusters, since the entire fleet is expected to migrate to managed DNS.

**Route53 rate limiting:**

Route53 APIs are easily throttled (5 req/s per account). The two controllers use different throttle strategies matching their reconciliation patterns:

- **Ingress zones (HCP reconciler):** The HCP reconciler reconciles frequently due to status changes from other controllers, so the ingress DNS function uses an in-memory timestamp to gate Route53 calls. When `AWSManagedDNSAvailable` is `True`, it short-circuits unless 5 minutes have elapsed (periodic re-verification). When the condition is not yet `True` (initial setup or error), a 30-second minimum interval prevents hot loops from status-update-triggered reconciles.
- **`.hypershift.local` zone (PrivateLink controller):** Uses the existing PrivateLink throttle pattern: if AWS returns a `ThrottlingException`, `isAWSThrottleError()` detects it and the reconciler requeues after a 2-minute delay (`throttleRequeueDelay`).

In both cases, throttle errors are surfaced in the relevant condition message for visibility.

**external-dns configuration:**

The `--source=crd` and `--managed-record-types=A,AAAA,CNAME,NS` flags are added to the external-dns deployment for all AWS management clusters, not only those with `managedDNS` configured. This is necessary because external-dns is deployed once per management cluster, not per HostedCluster. When no `DNSEndpoint` CRs exist, the additional CRD source adds an idle watch with negligible overhead. The ClusterRole for external-dns is also extended with read/update permissions for `externaldns.k8s.io/dnsendpoints`.

**Zone cleanup:**

On HostedCluster deletion, the HCP reconciler handles ingress zone cleanup. It deletes the `DNSEndpoint` CR (if created), drains all non-SOA/NS records from each managed ingress zone, then deletes the zones. Zone IDs are read from `hcp.Status.Platform.AWS.DNSZones` on the reconciled resource directly. The `.hypershift.local` zone is cleaned up separately by the PrivateLink controller via `AWSEndpointServiceStatus` (shared VPC: only the public ingress zone is cleaned up — see Change 1).

### Risks and Mitigations

**Risk: Orphaned zones on failed cleanup**

If the controller cannot delete zones during HostedCluster deletion (e.g., IAM permissions revoked, or HCP already deleted before cleanup runs), ingress zones may be orphaned in the customer's account. The zone names follow a predictable pattern (`{ingressDomainPrefix}.{baseDomainPrefix}.{baseDomain}`) and can be identified for manual cleanup. The `.hypershift.local` zone is tracked in `AWSEndpointServiceStatus` and is cleaned up independently.

**Risk: Race between zone creation and certificate issuance**

The consuming platform may attempt ACME DNS01 challenges before the ingress zone and ACME CNAME are created. Mitigation: the `AWSManagedDNSAvailable` condition signals when DNS is ready. When delegation is configured, the condition remains `False` with `NSDelegationPending` until NS delegation is confirmed via live DNS lookup. The consuming platform should gate certificate requests on this condition.

### Drawbacks

This adds Route53 zone lifecycle management to the CPO, increasing its operational surface. However, the CPO already manages Route53 records and VPC endpoints, and this aligns AWS with what GCP already does. The spec field gate ensures no impact on clusters that don't opt in.

## Alternatives (Not Implemented)

**External DNS management by the cluster lifecycle component (current approach):**

The current approach requires a separate component with customer AWS credentials to pre-create zones. This works for the centralized ROSA architecture where the cluster lifecycle component has customer credentials, but in ROSA HyperFleet the goal is to minimize customer account access from the lifecycle component and keep DNS management with HyperShift, which already manages customer-account infrastructure.

**NS delegation exclusively by the cluster lifecycle component:**

Instead of the CPO supporting `nsDelegation: ExternalDNS`, the consuming platform could always handle NS delegation itself (`Manual` mode) by reading nameservers from HostedCluster status. This is supported as the `Manual` mode, but as the exclusive approach it adds a round-trip through the management layer for every cluster. When external-dns is available on the Management Cluster, `ExternalDNS` mode is preferred as it completes the delegation chain directly.

**No external-dns integration in the CPO:**

The CPO could limit its scope to zone creation, CNAME setup, and status reporting — without creating `DNSEndpoint` CRs. This is effectively `managedDNS: {}` (zones-only mode) or `delegation` with `nsDelegation: Manual`. The API supports this as a first-class mode for consumers that don't have external-dns or prefer to handle delegation themselves.

## Open Questions

None at this time.

## Test Plan

- **Unit tests** with mock Route53 client covering:
  - `managedDNS` absent → no-op (backwards compatibility)
  - Zones-only mode (no delegation) → zones created with default prefix (in.{baseDomainPrefix}.{baseDomain}), no CNAME or DNSEndpoint
  - Delegation mode → zones created with prefix, ACME CNAME created
  - `nsDelegation: ExternalDNS` → `DNSEndpoint` CR created with correct NS records
  - `nsDelegation: Manual` → no `DNSEndpoint` CR created
  - Custom `ingressDomainPrefix` → zone naming uses custom prefix
  - ACME CNAME record derivation from prefixed ingress zone domain
  - Idempotent zone creation (zone already exists → reuse)
  - Cleanup: records drained before zone deletion, tag-based fallback when HCP unavailable
  - Status condition set on success and failure
  - NS delegation verification via `net.LookupNS()` (mocked)
  - Route53 throttle handling → condition set to False, requeue after backoff
  - `.hypershift.local` auto-create when `managedDNS` is set and zone does not exist
  - `.hypershift.local` NOT auto-created when `managedDNS` is absent (backwards compatibility)
  - Shared VPC + `managedDNS` → only public ingress zone created and deleted
- **Envtest** for CEL validation on new spec and status types
- **E2e** testing via the existing `e2e-aws` suite with `managedDNS` configured

## Graduation Criteria

The `AWSManagedDNS` feature gate controls the availability of the `managedDNS` spec field on `AWSPlatformSpec`.

### Dev Preview -> Tech Preview

The feature gate is enabled for `TechPreviewNoUpgrade`. The `managedDNS` field is only present in the CRD when the management cluster runs with this feature set.

### Tech Preview -> GA

Promote to `Default` feature set after validation in production HyperFleet environments. At that point, the `managedDNS` field becomes available on all management clusters.

### Removing a deprecated feature

N/A — this is a new feature with no deprecation considerations.

## Upgrade / Downgrade Strategy

**Upgrade (N -> N+1):**

No action required. The new spec and status fields are additive — old controllers ignore them. The feature gate means no behavioral change unless the consuming platform explicitly configures `managedDNS`. Existing clusters without `managedDNS` continue to work identically.

**Downgrade (N+1 -> N):**

If a cluster was created with managed DNS and the CPO is downgraded to a version without this feature, the zones will persist in the customer's Route53 account but will no longer be reconciled. The zones remain functional (DNS records still resolve), but will not be cleaned up on cluster deletion. Manual cleanup using the predictable zone naming pattern (`{ingressDomainPrefix}.{baseDomainPrefix}.{baseDomain}`) would be needed.

The status fields populated by the new CPO will be ignored by the old CPO and the old hypershift-operator (unknown fields in status are preserved by Kubernetes but not acted upon). The `managedDNS` spec field will be stripped from the stored object on any subsequent update (including unrelated field changes) if the old CRD schema does not include the field, because Kubernetes validates and prunes unknown fields on write. The zones remain functional in Route53, but the CPO configuration is effectively lost from the Kubernetes object. Re-upgrading to a version with managed DNS support would require re-applying the `managedDNS` spec field.

## Version Skew Strategy

The hypershift-operator must include the managed DNS types to copy spec/status and render CPO RBAC. The `AWSManagedDNS` feature gate ensures the `managedDNS` field is only present in the CRD when the management cluster runs a version that includes this feature.

The CPO is OCP version bound, so it can be at a lower version than the hypershift-operator. An older CPO that does not include managed DNS support will ignore the `managedDNS` field. The consuming platform should only set `managedDNS` on clusters running an OCP version whose CPO includes this feature.

## Operational Aspects of API Extensions

This enhancement adds new spec and status fields to existing CRDs and a new condition type. No new CRDs, webhooks, or aggregated API servers are introduced.

- All zone creation (`.hypershift.local`, ingress public, ingress private), CNAME setup, and `DNSEndpoint` CR creation only occur when `managedDNS` is set. Clusters without `managedDNS` are unaffected. Failure to create the `.hypershift.local` zone is surfaced via the `AWSEndpointAvailable` condition on the `AWSEndpointService` resource and CPO logs.
- The `AWSManagedDNSAvailable` condition is only set when `managedDNS` is configured. It provides a clear signal for operational monitoring. When `False`, the message field contains the specific error (Route53 API error, missing CRD, NS delegation pending, etc.).
- All zone and record creation is idempotent — subsequent reconciliation cycles verify zones and records exist via stored zone IDs before creating. If a managed zone is deleted out-of-band (e.g., the customer deletes the Route53 zone directly), the controller detects the `NoSuchHostedZone` error on the next reconciliation via `verifyOrCreateZone`, clears the stored zone ID, and recreates the zone. Note that a recreated zone will have new nameservers, so any existing NS delegation records in the parent zone will become stale. For `nsDelegation: ExternalDNS`, the `DNSEndpoint` CR is updated on the next reconciliation with the new nameservers, and external-dns propagates the change. For `nsDelegation: Manual`, the consuming platform must read the updated nameservers from HostedCluster status and update delegation records.
- Route53 rate limiting: ingress zones use in-memory timestamps (30s minimum between attempts, 5-minute re-verify when healthy); `.hypershift.local` zone uses the PrivateLink controller's existing throttle-and-requeue pattern. See the Rate Limiting section in Implementation Details.

## Support Procedures

- **Detection:** For managed ingress DNS failures, check the `AWSManagedDNSAvailable` condition on the HostedCluster. If `False`, the message contains the error (e.g., `AccessDenied`, `ThrottlingException`, `NSDelegationPending`, `DNSEndpoint CRD not installed`). For `.hypershift.local` zone failures, check the `AWSEndpointAvailable` condition on the `AWSEndpointService` resource and CPO logs.
- **Diagnosis:** Verify the CPO IAM role has the required Route53 permissions. For `NSDelegationPending`, verify that external-dns is running and the parent zone has the NS records (if using `ExternalDNS` mode) or that the consuming platform has created them (if using `Manual` mode). Review CPO logs for Route53 API errors.
- **Recovery:** DNS zone creation is idempotent — fixing the underlying issue (e.g., IAM permissions) and waiting for the next reconciliation cycle will resolve the problem automatically.

## Infrastructure Needed

No new infrastructure needed. The feature uses the existing Route53 API and IAM roles already available to the CPO.
