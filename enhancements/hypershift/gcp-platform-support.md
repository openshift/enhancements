---
title: hypershift-gcp-platform-support
authors:
  - "@ckandaga"
  - "@cveiga"
  - "@apahim"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-01-05
last-updated: 2026-01-05
tracking-link:
  - https://issues.redhat.com/browse/GCP-75
see-also:
  - "/enhancements/hypershift/networking/networking.md"
  - "/enhancements/cloud-integration/azure/azure-workload-identity.md"
---

# Google Cloud Platform Support for HyperShift

## Summary

This enhancement adds Google Cloud Platform (GCP) as a supported platform for HyperShift hosted clusters. The implementation enables users to deploy OpenShift hosted control planes on GCP infrastructure, leveraging Cluster API Provider GCP (CAPG) for infrastructure management, Workload Identity Federation (WIF) for secure keyless authentication, and Private Service Connect (PSC) for private networking between management and guest clusters.

## Motivation

HyperShift currently supports AWS, Azure, and KubeVirt as platforms for hosted control planes. Adding GCP support expands the options available to customers who operate in Google Cloud environments, enabling them to benefit from HyperShift's cost-effective and efficient hosted control plane model.

GCP is a major cloud provider with a significant customer base. Many organizations operate hybrid or multi-cloud environments that include GCP, and they require the ability to run OpenShift hosted clusters on GCP infrastructure while maintaining consistency with their existing HyperShift deployments on other platforms.

### User Stories

- As a cluster administrator using GCP, I want to deploy HyperShift hosted clusters on GCP infrastructure so that I can reduce control plane costs and simplify cluster management in my GCP environment.

- As a platform engineer, I want to use Workload Identity Federation for HyperShift clusters on GCP so that I can avoid managing long-lived service account keys and improve security posture.

- As a security-conscious operator, I want to deploy private HyperShift clusters on GCP using Private Service Connect so that control plane traffic remains within private networks and is not exposed to the public internet.

- As a DevOps engineer, I want CLI commands to create and destroy GCP infrastructure for HyperShift so that I can automate cluster lifecycle management.

- As a cluster administrator, I want to configure GCP-specific NodePool settings (machine type, zones, disk configuration) so that I can optimize worker node specifications for my workloads.

- As a HyperShift engineer, I want hypershift to validate my GCP hosted-cluster configuration so that I can identify and resolve access issues early in the process.

### Goals

- Enable deployment of HyperShift hosted clusters on GCP infrastructure
- Implement secure, keyless authentication using GCP Workload Identity Federation
- Support private cluster deployments using GCP Private Service Connect
- Provide CLI commands for creating and destroying GCP infrastructure and IAM resources
- Support GCP-specific NodePool configuration for worker nodes
- Integrate with external-dns for GCP Cloud DNS management
- Follow established HyperShift patterns from AWS and Azure implementations

### Non-Goals

- Supporting GCP as a management cluster platform (this enhancement covers GCP as a guest cluster platform only)
- Implementing GCP-specific storage drivers (CSI driver support will be addressed separately)
- Supporting legacy service account key-based authentication (WIF is mandatory)
- Providing migration paths from existing self-managed OpenShift clusters on GCP
- Machine API integration (CAPG handles machine management)

## Proposal

This enhancement adds GCP as a new platform type in HyperShift, following the established patterns from AWS and Azure implementations. The implementation consists of several key components:

### GCP Platform Foundation

Establishes the essential foundation for GCP platform integration:

**Platform Registration:**
- Define GCP as a supported platform type in the HyperShift API
- Create feature flag (`GCPPlatform`) to gate GCP-HCP changes
- Integrate GCP as a supported platform in the controller framework
- Validate GCP hosted-cluster configuration

**Workload Identity Federation:**
- Use GCP Workload Identity Federation to allow HyperShift components to assume GCP identities without long-lived keys
- Define mapping between Kubernetes service accounts and GCP IAM roles
- Bootstrap and manage identity pools, providers, and trust relationships

**CLI Infrastructure Commands:**
- `hypershift create infra gcp` / `hypershift destroy infra gcp` for network infrastructure
- `hypershift create iam gcp` / `hypershift destroy iam gcp` for IAM resources
- `hypershift create cluster gcp` / `hypershift destroy cluster gcp` for cluster lifecycle

### Private Service Connect Infrastructure

Enables secure connectivity between management and customer projects using GCP Private Service Connect:

**PSC API Types:**
- `GCPResourceReference`: Name-based resource references for GCP (MaxLength=255)
- `GCPEndpointAccessType`: Enum with Public, PublicAndPrivate, Private values
- `GCPNetworkConfig`: Management and customer VPC configuration
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

### Hosted Cluster Deployment Configuration

Configuration and deployment items for full hosted cluster creation:

- Dedicated namespace for each HostedCluster CR
- SSH key management for hosted clusters
- DNS zone/subzone configuration
- Control plane component deployment configuration
- Extension-apiserver-authentication RBAC permissions
- Let's Encrypt wildcard certificate management

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
- Architecture validation (AMD64 support)

### Workflow Description

**Cluster Administrator** is a human user responsible for deploying hosted clusters.

#### Prerequisites

1. A HyperShift-enabled management cluster exists (can be on any supported platform)
2. A GCP project with appropriate quotas and API enablement
3. User has sufficient GCP IAM permissions

#### Infrastructure Setup

1. The administrator runs `hypershift create infra gcp` to provision networking infrastructure:
   ```bash
   hypershift create infra gcp \
     --project-id my-project \
     --region us-central1 \
     --infra-id my-infra
   ```
   This creates:
   - VPC network (`{infraID}-network`)
   - Subnet (`{infraID}-subnet`)
   - Cloud Router (`{infraID}-router`)
   - Cloud NAT (`{infraID}-nat`)

2. The administrator runs `hypershift create iam gcp` to provision IAM resources:
   ```bash
   hypershift create iam gcp \
     --project-id my-project \
     --infra-id my-infra \
     --oidc-storage-provider-s3-bucket-name my-oidc-bucket
   ```
   This creates:
   - Workload Identity Pool
   - OIDC Provider
   - Service accounts for control plane components
   - IAM role bindings

#### Cluster Creation

3. The administrator runs `hypershift create cluster gcp`:
   ```bash
   hypershift create cluster gcp \
     --name my-cluster \
     --infra-id my-infra \
     --project-id my-project \
     --region us-central1 \
     --network-name my-infra-network \
     --subnet-name my-infra-subnet \
     --workload-identity-provider projects/{project}/locations/global/workloadIdentityPools/{pool}/providers/{provider}
   ```

4. The HyperShift operator creates the hosted control plane:
   - Validates GCP configuration
   - Deploys CAPG controllers
   - Creates GCPCluster resource
   - Provisions control plane components with WIF credentials
   - For private clusters: Creates PSC Service Attachments

5. The control-plane-operator (for private clusters):
   - Observes forwarding rules and ILB IPs
   - Creates PSC endpoints in customer VPC
   - Sets up DNS zones and records

#### NodePool Creation

6. The administrator creates NodePools to provision worker nodes:
   ```yaml
   apiVersion: hypershift.openshift.io/v1beta1
   kind: NodePool
   metadata:
     name: my-nodepool
     namespace: clusters
   spec:
     clusterName: my-cluster
     replicas: 3
     platform:
       type: GCP
       gcp:
         instanceType: n2-standard-4
         zone: us-central1-a
         rootVolume:
           sizeGiB: 128
           type: pd-ssd
         serviceAccount:
           email: my-sa@my-project.iam.gserviceaccount.com
   ```

7. CAPG provisions GCE instances as worker nodes
8. Workers join the hosted cluster via Konnectivity

#### Private Cluster Workflow

For private clusters using Private Service Connect:

1. Administrator sets endpoint access to `Private` in HostedCluster spec
2. `GCPPrivateServiceObserver` monitors forwarding rules and discovers ILB IPs
3. `GCPPrivateServiceConnectReconciler` creates Service Attachments in management VPC
4. PSC Endpoint controller creates endpoints in customer VPC with reserved static IPs
5. DNS controller creates zones and records pointing to PSC endpoint IPs
6. All control plane traffic flows through PSC, remaining within private networks

### API Extensions

#### New Types (api/hypershift/v1beta1/gcp.go)

**GCPPlatformSpec:**
```go
type GCPPlatformSpec struct {
    // Project is the GCP project where the cluster will be created
    Project string `json:"project"`
    // Region is the GCP region for the cluster
    Region string `json:"region"`
    // NetworkConfig contains networking configuration
    NetworkConfig GCPNetworkConfig `json:"networkConfig"`
    // WorkloadIdentityConfig contains WIF configuration
    WorkloadIdentityConfig GCPWorkloadIdentityConfig `json:"workloadIdentityConfig"`
    // ResourceLabels are labels applied to GCP resources
    ResourceLabels []GCPResourceLabel `json:"resourceLabels,omitempty"`
}
```

**GCPWorkloadIdentityConfig:**
```go
type GCPWorkloadIdentityConfig struct {
    // WorkloadIdentityProviderURI is the URI of the WIF provider
    WorkloadIdentityProviderURI string `json:"workloadIdentityProviderURI"`
    // ServiceAccounts references the secret containing service account mappings
    ServiceAccounts GCPServiceAccountsRef `json:"serviceAccounts"`
    // ProjectNumber is the GCP project number
    ProjectNumber string `json:"projectNumber"`
}
```

**GCPNodePoolPlatform:**
```go
type GCPNodePoolPlatform struct {
    // InstanceType is the GCE machine type
    InstanceType string `json:"instanceType"`
    // Zone is the GCE zone for node placement
    Zone string `json:"zone"`
    // RootVolume configures the boot disk
    RootVolume GCPRootVolume `json:"rootVolume,omitempty"`
    // ServiceAccount configures the GCE service account
    ServiceAccount GCPServiceAccount `json:"serviceAccount,omitempty"`
    // NetworkTags are applied to instances for firewall rules
    NetworkTags []string `json:"networkTags,omitempty"`
    // Labels are applied to GCE instances
    Labels []GCPResourceLabel `json:"labels,omitempty"`
    // Preemptible indicates use of preemptible instances
    Preemptible bool `json:"preemptible,omitempty"`
}
```

#### New CRDs

**GCPPrivateServiceConnect** (hypershift.openshift.io/v1alpha1):
- Manages PSC lifecycle between management and customer projects
- Spec: ForwardingRuleName, ConsumerAcceptList, NATSubnets
- Status: ServiceAttachmentName, ServiceAttachmentURI, EndpointIP, DNSZoneName, Conditions

#### Feature Gate

GCP platform support is gated behind the `GCPPlatform` feature gate and initially requires `--tech-preview-no-upgrade` for deployment.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specifically for HyperShift and defines how hosted control planes operate with GCP as the guest cluster platform:

- Control plane components run in the management cluster namespace
- CAPG controllers are deployed per hosted cluster
- WIF provides authentication for all GCP API interactions
- PSC enables private connectivity between management and guest clusters
- The management cluster can be on any supported platform (AWS, Azure, etc.)

#### Standalone Clusters

This enhancement does not affect standalone OpenShift clusters. GCP support for standalone clusters is handled by the existing installer and machine-api-provider-gcp.

#### Single-node Deployments or MicroShift

This enhancement does not apply to SNO or MicroShift deployments.

### Implementation Details/Notes/Constraints

#### CAPG Version

The implementation uses CAPG v1.10.x which provides:
- GCPCluster and GCPMachine resources
- Workload Identity Federation support
- Multi-zone deployment capability

#### Workload Identity Federation Requirements

WIF configuration requires:
- A Workload Identity Pool in the GCP project
- An OIDC provider configured with the hosted cluster's OIDC endpoint
- Service accounts with appropriate IAM roles:
  - Control Plane Operator: `roles/compute.admin`, `roles/iam.serviceAccountUser`
  - Node Pool Manager: `roles/compute.instanceAdmin.v1`
  - Image Registry: `roles/storage.objectAdmin`
  - Ingress: `roles/dns.admin`
  - Cloud Controller Manager: `roles/compute.networkViewer`

#### Networking Architecture

GCP hosted clusters use the same networking model as AWS:
- Konnectivity for control plane to node communication
- HAProxy on nodes for kubernetes.default.svc routing
- OVN for pod networking

For private clusters:
- Internal Load Balancers expose control plane services
- Private Service Connect provides cross-VPC connectivity
- NAT subnets with PSC purpose for Service Attachments
- Firewall rules control traffic between components

#### Resource Labeling

GCP resources are labeled following RFC1035 conventions:
- Labels use lowercase letters, numbers, and hyphens
- No underscores (differs from Kubernetes labels which use RFC1123)
- Reserved prefixes (`goog`) are blocked per GCP standards
- Maximum 63 characters per label value

#### GCP API Dependencies

Required GCP APIs:
- Compute Engine API
- Cloud DNS API
- IAM API
- Cloud Resource Manager API
- Service Networking API (for PSC)

### Risks and Mitigations

#### Risk: WIF Configuration Complexity

**Mitigation**: The `hypershift create iam gcp` command automates the creation of all required IAM resources including the Workload Identity Pool, OIDC provider, and service accounts. Comprehensive validation identifies access issues early.

#### Risk: Private Service Connect Limitations

**Mitigation**: PSC has limits on connections per Service Attachment (default 10, max 10,000). Documentation will clearly state these limits and provide guidance for large deployments. The implementation supports ConsumerAcceptList for access control.

#### Risk: CAPG API Stability

**Mitigation**: CAPG v1.10 is a stable release. The implementation uses well-established CAPI patterns and will track CAPG releases for compatibility.

#### Risk: GCP API Rate Limiting

**Mitigation**: Controllers implement appropriate retry logic and exponential backoff for GCP API interactions. Resource reconciliation is designed to be idempotent.

#### Risk: Cross-Project Authentication

**Mitigation**: WIF enables secure cross-project operations without credential sharing. The implementation uses impersonation patterns for customer project operations.

### Drawbacks

- Adds maintenance burden for another cloud platform
- CAPG dependency adds complexity to the deployment
- WIF requirement may be unfamiliar to users accustomed to service account keys
- PSC has regional limitations and connection limits
- GCP naming conventions (RFC1035 for some resources) differ from Kubernetes conventions

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

1. Should GCP support include integration with GKE for the management cluster, or is it limited to self-managed OpenShift management clusters?

2. What is the upgrade path from Tech Preview to GA, and how will existing clusters be migrated if API changes are required?

3. Should we support Confidential VMs for hosted cluster workers in a future iteration?

4. How should multi-region deployments be handled for disaster recovery scenarios?

## Test Plan

### Unit Tests

- GCP API type validation (RFC1035 labels, service account emails, project IDs)
- Platform detection logic
- Resource name generation
- WIF credential file building
- PSC status management

### Integration Tests

- CAPG controller deployment and scheme registration
- GCPCluster reconciliation
- NodePool machine template generation
- GCPPrivateServiceConnect status updates
- CLI command execution

### E2E Tests

- Full cluster lifecycle (create, scale, destroy) on GCP
- Private cluster deployment with PSC
- NodePool operations (create, scale, delete)
- Upgrade testing
- WIF credential rotation

### CI Infrastructure

- GCP project with appropriate quotas
- Service accounts for automated testing
- Network configuration for private cluster testing
- RHCOS image access for development and testing

## Graduation Criteria

### Dev Preview -> Tech Preview

- Basic cluster creation and destruction working
- WIF authentication functional for all components
- NodePool support complete with GCPMachineTemplate generation
- CLI commands implemented (`create/destroy cluster/infra/iam gcp`)
- PSC infrastructure functional for private clusters
- Unit test coverage > 80%
- Integration tests passing

### Tech Preview -> GA

- Private cluster support with PSC fully complete
- DNS management functional with all three zone types
- E2E tests passing consistently
- Performance testing completed
- Load testing for control plane density
- User documentation in openshift-docs
- Operator conditions and metrics implemented
- Support runbooks created
- Security review completed

### Removing a deprecated feature

Not applicable - this is a new feature.

## Upgrade / Downgrade Strategy

GCP platform support is introduced as a new capability. Existing clusters on other platforms are not affected.

For GCP clusters:
- Control plane component upgrades follow standard HyperShift patterns
- CAPG version upgrades are managed by the control plane operator
- WIF configuration is immutable after cluster creation
- NodePool upgrades follow existing machine rollout strategies

Downgrade is not supported for the platform type itself. Individual component downgrades follow standard OpenShift patterns.

## Version Skew Strategy

- CAPG controllers are deployed per hosted cluster and versioned with the control plane
- Management cluster and hosted cluster can run different OpenShift versions within supported skew
- WIF configuration is validated at cluster creation time
- GCP API compatibility is maintained through the google.golang.org/api dependency

## Operational Aspects of API Extensions

### GCPPrivateServiceConnect CRD

**SLIs:**
- `hypershift_gcp_psc_reconcile_duration_seconds`: Time to reconcile PSC resources
- `hypershift_gcp_psc_service_attachment_ready`: Boolean indicating SA readiness
- `hypershift_gcp_psc_endpoint_ready`: Boolean indicating endpoint readiness

**Failure Modes:**
- GCP API unavailable: Reconciliation will retry with exponential backoff
- Quota exceeded: Condition set on resource, event emitted
- Invalid configuration: Admission webhook rejects invalid specs
- Cross-project authentication failure: WIF token refresh, detailed error logging

**Impact on Existing SLIs:**
- Minimal impact - PSC resources are per-cluster
- Expected < 100 PSC resources per management cluster

### Support Procedures

**Detecting Failures:**
- Check `GCPPrivateServiceConnect` status conditions (GCPServiceAttachmentAvailable, GCPEndpointAvailable, GCPDNSAvailable)
- Review control plane operator logs for GCP API errors
- Monitor `hypershift_gcp_*` metrics for anomalies
- Check for events on GCPPrivateServiceConnect resources

**Disabling the Extension:**
- Set `spec.platform.type` to a different platform (requires cluster recreation)
- PSC resources can be deleted manually if controller is malfunctioning

**Graceful Degradation:**
- PSC controller failures don't affect running workloads
- Control plane remains functional if PSC reconciliation fails temporarily
- Manual intervention possible for Service Attachment creation
- DNS failures are recoverable once the controller is restored

## Infrastructure Needed

- GCP project for CI testing with appropriate quotas
- Service accounts for automated testing
- VPC and subnet configuration for integration tests
- Access to GCP console for debugging and validation
- RHCOS image access for development and production

## Implementation History

The implementation is organized under the GCP-75 feature with the following epics:
- GCP-79: GCP Platform Foundation (Closed)
- GCP-81: PSC Infrastructure (In Progress)
- GCP-82: Production Readiness (Closed)
- GCP-84: Hosted Cluster Deployment Configuration (Closed)
- GCP-212: NodePool Support (In Progress)

The initial work began in October 2025 with the feature gate and API definitions. Core functionality including CLI commands, WIF support, and PSC infrastructure is substantially complete, with NodePool support and remaining PSC controllers in review.
