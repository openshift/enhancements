---
title: hypershift-gcp-platform-support
authors:
  - "@ckandaga"
  - "@cveiga"
  - "@apahim"
  - "@billmvt"
reviewers:
  - "@csrwng"
  - "@muraee"
  - "@devguyio"
approvers:
  - "@csrwng"
  - "@muraee"
  - "@devguyio"
api-approvers:
  - "@csrwng"
  - "@muraee"
  - "@devguyio"
creation-date: 2026-01-05
last-updated: 2026-01-14
tracking-link:
  - https://issues.redhat.com/browse/GCP-75
see-also:
  - "/enhancements/hypershift/networking/networking.md"
  - "/enhancements/cloud-integration/azure/azure-workload-identity.md"
---

# Google Cloud Platform Support for HyperShift

## Summary

This enhancement adds Google Cloud Platform (GCP) as a supported platform for HyperShift hosted clusters. The implementation enables users to deploy OpenShift hosted clusters on GCP infrastructure, leveraging Cluster API Provider GCP (CAPG) for infrastructure management, Workload Identity Federation (WIF) for secure keyless authentication, and Private Service Connect (PSC) for private networking between management and hosted clusters.

## Motivation

HyperShift supports a variety of cloud and on-premise platforms. Adding GCP support expands the options available to customers who operate in Google Cloud environments, enabling them to benefit from HyperShift's cost-effective and efficient hosted control plane model.

GCP is a major cloud provider with a significant customer base. Many organizations operate hybrid or multi-cloud environments that include GCP, and they require the ability to run OpenShift hosted clusters on GCP infrastructure while maintaining consistency with their existing HyperShift deployments on other platforms.

### User Stories

- As a cluster administrator using GCP, I want to deploy HyperShift hosted clusters on GCP infrastructure so that I can reduce control plane costs and simplify cluster management in my GCP environment.

- As a platform engineer, I want to use Workload Identity Federation for HyperShift hosted clusters on GCP so that I can avoid managing long-lived service account keys and improve security posture.

- As a security-conscious operator, I want to deploy HyperShift hosted clusters on GCP using Private Service Connect so that control plane traffic remains within private networks.

- As a DevOps engineer, I want CLI commands to create and destroy GCP infrastructure for HyperShift so that I can automate hosted cluster lifecycle management.

- As a cluster administrator, I want to configure GCP-specific NodePool settings (machine type, zones, disk configuration) so that I can optimize worker node specifications for my workloads.

### Goals

- Enable deployment of HyperShift hosted clusters on GCP infrastructure
- Support GKE as a management cluster platform for running HyperShift
- Implement secure, keyless authentication using GCP Workload Identity Federation
- Support secure, private control plane traffic using GCP Private Service Connect
- Provide CLI commands for creating and destroying GCP infrastructure and IAM resources
- Support GCP-specific NodePool configuration for worker nodes
- Integrate with external-dns for GCP Cloud DNS management
- Follow established HyperShift patterns from AWS and Azure implementations

### Non-Goals

- Supporting legacy service account key-based authentication (WIF is mandatory)
- Windows worker nodes (may be added in a future enhancement)

## Proposal

This enhancement adds GCP as a new platform type in HyperShift, following the established patterns from AWS and Azure implementations. The implementation consists of several key components:

### GCP Platform Foundation

Establishes the essential foundation for GCP platform integration:

**Platform Registration:**
- Define GCP as a supported platform type in the HyperShift API
- Create feature flag (`GCPPlatform`) to gate GCP-HCP changes
- Integrate GCP as a supported platform in the controller framework

**Workload Identity Federation:**
- Use GCP Workload Identity Federation to allow HyperShift components to authenticate to GCP APIs without long-lived keys
- Define mapping between Kubernetes service accounts and GCP service accounts with necessary IAM roles
- Bootstrap and manage identity pools, providers, and trust relationships

**CLI Infrastructure Commands:**
- `hypershift create infra gcp` / `hypershift destroy infra gcp` for network infrastructure
- `hypershift create iam gcp` / `hypershift destroy iam gcp` for IAM resources
- `hypershift create cluster gcp` / `hypershift destroy cluster gcp` for cluster lifecycle

### Private Service Connect Infrastructure

Enables secure connectivity between management and customer projects using GCP Private Service Connect:

**PSC API Types:**
- `GCPResourceReference`: Name-based resource references for GCP (MaxLength=63)
- `GCPEndpointAccessType`: Enum with PublicAndPrivate, Private values
- `GCPNetworkConfig`: Customer VPC configuration
- `GCPPrivateServiceConnect`: CRD for PSC lifecycle management

**PSC Controllers:**
- `GCPPrivateServiceObserver` in control-plane-operator: Monitors forwarding rules for load balancer IPs
- `GCPPrivateServiceConnectReconciler` in hypershift-operator: Creates Service Attachments in management VPC
- PSC Endpoint controller in control-plane-operator: Creates endpoints in customer VPCs

**Supporting Infrastructure:**
- Private Router Service support with Internal Load Balancer
- Network Policy support for GCP clusters
- WIF integration for cross-project authentication

**DNS Management:**
Three DNS zones created after PSC endpoints are ready:
1. `{cluster}.hypershift.local` (private) for internal services
2. `in.{clusterDNSZoneBaseDomain}` (public) for ACME challenge delegation
3. `in.{clusterDNSZoneBaseDomain}` (private) for VPC-internal ingress

### NodePool Support

Full integration with Cluster API Provider GCP (CAPG) for worker node management:

**CAPG Integration:**
- Deploy and manage CAPG controllers per hosted cluster
- Watch GCPMachineTemplate resources (dynamic and static modes)
- Register CAPG types in controller scheme
- Resource cleanup for GCP resources

**NodePool Management:**
- Create NodePools with platform.type: GCP
- Support all major configuration options:
  - Machine types (e.g., `n2-standard-4`)
  - Zone placement
  - Boot disk configuration (size, type, encryption)
  - Service accounts and OAuth scopes
  - Network tags for firewall rules
  - Resource labels (RFC1035 format)
  - Preemptible instances
- Image discovery with release payload integration
- GCPMachineTemplate generation
- Architecture support (AMD64 and ARM64)

### Workflow Description

GCP hosted cluster creation follows the established HyperShift workflow:

1. **Infrastructure setup**: `hypershift create infra gcp` provisions VPC, subnets, Cloud Router, and Cloud NAT
2. **IAM setup**: `hypershift create iam gcp` provisions Workload Identity Pool, OIDC provider, and service accounts
3. **Cluster creation**: `hypershift create cluster gcp` creates the hosted cluster
4. **NodePool creation**: Standard NodePool resources with `platform.type: GCP`

For private clusters, PSC Service Attachments and endpoints are automatically provisioned, with DNS zones created once connectivity is established.

Detailed CLI usage follows the patterns documented for existing platforms.

### API Extensions

#### HostedCluster Platform Types (api/hypershift/v1beta1/gcp.go)

**GCPPlatformSpec** - Top-level GCP configuration for HostedCluster:
- `project` (required, immutable): GCP project ID (6-30 chars, lowercase alphanumeric and hyphens)
- `region` (required, immutable): GCP region (e.g., `us-central1`)
- `networkConfig` (required): VPC and PSC subnet configuration
- `endpointAccess` (optional): `Private` (default) or `PublicAndPrivate`
- `resourceLabels` (optional): Labels applied to GCP resources (max 60)
- `workloadIdentity` (required, immutable): WIF configuration

**GCPNetworkConfig** - VPC configuration:
- `network`: VPC network name reference
- `privateServiceConnectSubnet`: Subnet for PSC endpoints

**GCPWorkloadIdentityConfig** - Workload Identity Federation configuration:
- `projectNumber`: Numeric GCP project identifier
- `poolID`: Workload Identity Pool ID (4-32 chars)
- `providerID`: OIDC Provider ID within the pool
- `serviceAccountsEmails`: Service account emails for controllers

**GCPServiceAccountsEmails** - Service accounts for different controllers:
- `controlPlane`: GSA for Control Plane Operator (roles/storage.admin, roles/iam.serviceAccountUser)
- `nodePool`: GSA for CAPG controllers (roles/compute.instanceAdmin.v1, roles/compute.networkAdmin)

**Supporting types:**
- `GCPResourceReference`: Name-based resource reference (RFC1035 naming)
- `GCPResourceLabel`: Key-value labels for GCP resources
- `GCPEndpointAccessType`: Enum for endpoint access (Private, PublicAndPrivate)

#### NodePool Platform Types (api/hypershift/v1beta1/gcp.go - PR #7329)

**GCPNodePoolPlatform** - GCP-specific NodePool configuration:
- `machineType` (required): GCE machine type (e.g., `n2-standard-4`)
- `zone` (required): GCE zone for instance placement
- `image` (optional): Boot image (defaults to RHCOS from release payload)
- `bootDisk` (optional): Boot disk configuration
- `serviceAccount` (optional): GCE instance service account
- `resourceLabels` (optional): Additional labels for instances
- `networkTags` (optional): Network tags for firewall rules
- `provisioningModel` (optional): `Standard` (default) or `Preemptible`
- `onHostMaintenance` (optional): `MIGRATE` or `TERMINATE`

**GCPBootDisk** - Boot disk configuration:
- `diskSizeGB`: Size in GB (default 64, min 20, max 65536)
- `diskType`: `pd-standard`, `pd-ssd`, or `pd-balanced` (default)
- `encryptionKey` (optional): Customer-managed encryption key (CMEK) configuration

**GCPDiskEncryptionKey** - Customer-managed encryption key for boot disks:
- `kmsKeyName` (required): Cloud KMS key resource name (format: `projects/{project}/locations/{location}/keyRings/{keyRing}/cryptoKeys/{key}`)

**GCPNodeServiceAccount** - Instance service account:
- `email`: Service account email
- `scopes`: OAuth scopes for the service account

#### New CRD (api/hypershift/v1beta1/gcpprivateserviceconnect_types.go)

**GCPPrivateServiceConnect** - Manages PSC infrastructure lifecycle:

Spec:
- `loadBalancerIP`: IP of the Internal Load Balancer
- `forwardingRuleName`: ILB forwarding rule name
- `consumerAcceptList`: Customer projects allowed to connect
- `natSubnet`: Subnet for NAT by Service Attachment

Status:
- `serviceAttachmentName`: Created Service Attachment name
- `serviceAttachmentURI`: Full URI for customer connections
- `endpointIP`: Reserved IP for PSC endpoint
- `dnsZoneName`: Private DNS zone name
- `dnsRecords`: Created DNS A records
- `conditions`: GCPPrivateServiceConnectAvailable, GCPServiceAttachmentAvailable, GCPEndpointAvailable, GCPDNSAvailable

#### Feature Gate

GCP platform support is gated behind the `GCPPlatform` feature gate.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is HyperShift-specific. It adds GCP as a new platform type for hosted control planes, enabling deployment of OpenShift hosted clusters on GCP infrastructure with the management cluster running on GKE.

#### Standalone Clusters

Not applicable. This enhancement is specific to the HyperShift topology.

#### Single-node Deployments or MicroShift

Not applicable. This enhancement is specific to the HyperShift topology.

#### OpenShift Kubernetes Engine

Not applicable. This enhancement is specific to the HyperShift topology and does not affect OKE.

### Implementation Details/Notes/Constraints

See the Proposal and API Extensions sections above.

### Affected Components

This section enumerates the OpenShift components that may require modifications to support
GCP as a HyperShift platform. Components are organized by current work status.

#### Components with Dedicated Work Items

The following components have dedicated feature epics and are actively being addressed:

| Component                         | Epic    |
|-----------------------------------|---------|
| Cloud Network Config Controller   | GCP-282 |
| Cloud Controller Manager          | GCP-311 |
| Cluster Ingress Operator / Router | GCP-314 |
| Image Registry Operator           | GCP-315 |
| Cluster Storage Operator / CSI    | GCP-322 |

#### Components Under Investigation

The following components may require modifications. Investigation is tracked under GCP-303.

| Component                       | Cloud Integration            |
|---------------------------------|------------------------------|
| Machine Config Operator         | Ignition, cloud-init         |
| Cluster Authentication Operator | OIDC                         |
| Cluster Monitoring Operator     | Metrics/alerting             |
| Kube Controller Manager         | Cloud provider               |
| Cluster Autoscaler              | Node scaling                 |
| OLM / Marketplace               | Catalog access               |
| Console Operator                | UI                           |
| DNS Operator                    | CoreDNS                      |

Note: Machine API Operator is not applicable for HyperShift as it is replaced by CAPI/CAPG.

### Risks and Mitigations

#### Private Service Connect Scalability

The PSC architecture addresses common scalability concerns:
- **NAT subnet sizing**: Each hosted cluster provisions its own NAT subnet, eliminating connection exhaustion risks
- **GCP quotas**: Each management cluster runs in its own GCP project, so quotas (1,000 service attachments, 500 forwarding rules per project) apply per management cluster rather than globally

#### Risk: GKE Autopilot Mode Compatibility

Running the management cluster on GKE in Autopilot mode may surface compatibility issues due to Autopilot's restrictions on workload configurations. Initial testing has been positive, but further validation is needed.

### Drawbacks

- Adds maintenance burden for another cloud platform, including additional CI infrastructure utilization

## Alternatives (Not Implemented)

### Service Account Keys Instead of WIF

Using traditional GCP service account keys would simplify the initial configuration but:
- Creates security risks with long-lived credentials
- Requires manual key rotation
- Doesn't align with GCP best practices
- WIF is the standard for Kubernetes workloads on GCP

### Direct GCE API Instead of CAPG

Implementing GCP support without CAPG would:
- Require significant custom controller development
- Miss out on CAPI ecosystem benefits
- Increase maintenance burden
- Diverge from the pattern used by other platforms

### VPC Peering Instead of PSC

Using VPC peering for private connectivity would:
- Create routing complexity with overlapping CIDRs
- Limit scalability (peering connection limits)
- Not provide the same level of isolation as PSC
- PSC is GCP's recommended approach for service exposure

## Open Questions

1. HCP migration: What is required to support hosted control plane migration between management clusters?

2. Are there other OpenShift components that lack GCP support when running in HyperShift topology?

## Test Plan

### Unit Tests

Unit tests in accordance with Kubernetes, OpenShift, and HyperShift standards.

### E2E Tests

E2E tests will bootstrap a management cluster on GKE, provision hosted control planes and NodePools, and exercise key GCP-specific functionality including WIF authentication and PSC connectivity.

### CI Infrastructure

Uses existing OpenShift Prow CI infrastructure.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Basic cluster creation and destruction working
- WIF authentication functional for all components
- NodePool support complete with GCPMachineTemplate generation
- CLI commands implemented (`create/destroy cluster/infra/iam gcp`)
- PSC infrastructure functional for private clusters
- E2E tests passing in Prow

### Tech Preview -> GA

- TBD

### Removing a deprecated feature

Not applicable - this is a new feature.

## Upgrade / Downgrade Strategy

GCP platform support is introduced as a new capability. Existing clusters on other platforms are not affected.

For GCP clusters:
- Control plane component upgrades follow standard HyperShift patterns
- CAPG version upgrades are managed by the control plane operator
- NodePool upgrades follow existing machine rollout strategies

## Version Skew Strategy

Nothing novel; follows standard HyperShift patterns.

## Operational Aspects of API Extensions

Nothing novel; follows standard HyperShift patterns.

## Support Procedures

Nothing novel; follows standard HyperShift patterns.

## Implementation History

The implementation is organized under the GCP-75 feature.

The initial work began in October 2025 with the feature gate and API definitions. Core functionality including CLI commands, WIF support, and PSC infrastructure is substantially complete, with NodePool support and remaining PSC controllers in review.
