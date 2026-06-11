---
title: gcp-sovereign-cloud
authors:
  - "@barbacbd"
reviewers:
  - "@rochacbruno"
  - "@patrickdillon"
approvers:
  - "@patrickdillon"
  - "@sadasu"
api-approvers:
  - "@jspeed"
creation-date: 2026-04-15
last-updated: 2026-04-15
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3006
see-also:
  - "/enhancements/installer/gcp-private-clusters.md"
  - "/enhancements/installer/gcp-ipi-shared-vpc.md"
  - "/enhancements/installer/aws-eusc-partition.md"
  - "/enhancements/installer/gcp-kms-key-encryption.md"
replaces: []
superseded-by: []
---

# GCP Sovereign Cloud Support

## Summary

This enhancement enables OpenShift to be installed on GCP Sovereign Cloud environments (such as Google Cloud Germany and other sovereign cloud offerings). Sovereign clouds are isolated GCP environments designed for government and regulated industries with specific data residency, security, and compliance requirements.

The key technical differentiator for sovereign clouds is the **universe domain** - a GCP SDK configuration that routes API calls to the appropriate sovereign cloud environment. For example:
- Public GCP uses universe domain `googleapis.com` (default)
- Google Cloud Germany uses universe domain `apis-berlin-build0.goog`

This enhancement adds a `universeDomain` field to the GCP platform configuration. When set, the installer and all cluster components configure their GCP SDK clients with `option.WithUniverseDomain()`, allowing them to operate in sovereign cloud environments. This approach:
- Matches Google's own tooling (gcloud CLI) for sovereign cloud access
- Works with any current or future GCP sovereign cloud without code changes
- Requires all components that call GCP APIs to read the universe domain from the Infrastructure CR

Currently, OpenShift components assume standard public GCP and do not configure universe domain, making them incompatible with sovereign cloud environments.

## Motivation

### Business Drivers

Government agencies and regulated industries (defense, healthcare, finance) in various countries require cloud infrastructure that meets strict data sovereignty, security, and compliance requirements including:
- **Data Residency**: Data must remain within specific geographic boundaries
- **Regulatory Compliance**: GDPR, national security regulations, industry-specific compliance frameworks
- **Operational Sovereignty**: Infrastructure operated by local entities or under specific jurisdictional control
- **Security Isolation**: Physical and logical separation from public cloud infrastructure

GCP Sovereign Cloud offerings address these requirements but are fundamentally incompatible with the current OpenShift installer implementation.

### Current Blocker

The OpenShift installer has multiple hardcoded assumptions about standard GCP that prevent installation on sovereign clouds:

1. **Hardcoded API Domains**: Service endpoints assume `*.googleapis.com` domain
2. **Service Name Validation**: Validates against public GCP service names
3. **Region Lists**: Hardcoded region validation against public GCP regions only
4. **Project Format**: Does not recognize sovereign cloud project naming conventions (e.g., `eu0:<project-name>`)

Without this enhancement, customers with sovereign cloud requirements cannot deploy OpenShift on GCP and must either:
- Use alternative cloud providers (reducing customer choice)
- Seek expensive exemptions from compliance requirements (often not feasible)
- Deploy on-premises (losing cloud benefits)

### User Stories

As a cluster administrator, I want to install OpenShift to GCP sovereign cloud regions (such as Google Cloud Germany) in order to meet data sovereignty, residency, and regulatory compliance requirements for government agencies, regulated industries, and organizations with strict data localization mandates.

### Goals

- Enable installation of OpenShift on GCP sovereign cloud environments
- Support multiple sovereign cloud types (Germany, other future sovereign clouds)
- Maintain backward compatibility with standard public GCP installations
- Provide clear configuration mechanism to specify sovereign cloud environment
- Support standard installer features (IPI, private clusters, shared VPC) on sovereign clouds
- Validate region and service availability specific to each sovereign cloud
- Document sovereign cloud-specific requirements and limitations

### Non-Goals

- Supporting every possible sovereign cloud variant in the initial implementation (focus on Germany as reference implementation)
- Automatic detection of sovereign cloud environment (explicit configuration required)
- Cross-sovereign-cloud networking or federation
- Sovereign cloud-specific RHCOS image builds (images must be pre-loaded by Red Hat or partners)
- Converting existing public GCP clusters to sovereign cloud (new installations only)
- Supporting GCP China or other specialized regional offerings not classified as sovereign clouds
- UPI (User Provisioned Infrastructure) mode in initial implementation

## Proposal

Add a new `universeDomain` field to the GCP platform configuration that specifies the GCP universe domain. Based on this field, the installer and all cluster components will:

1. Configure all GCP SDK clients with `option.WithUniverseDomain()` 
2. Validate regions and zones dynamically using GCP APIs
3. Propagate universe domain to cluster components via Infrastructure CR
4. Support any current or future GCP sovereign cloud without code changes

The universe domain is passed directly to the GCP SDK, matching how Google's own tooling (gcloud CLI) handles sovereign clouds. This approach is future-proof and requires no code changes when Google adds new sovereign cloud regions.

### Workflow Description

#### Installation Flow

**cluster administrator** is responsible for configuring access to the sovereign cloud environment and deploying the cluster.

1. The cluster administrator obtains access credentials for the GCP sovereign cloud environment (e.g., Google Cloud Germany):
   ```bash
   # Authenticate to sovereign cloud
   gcloud auth login --project eu0:my-project
   ```

2. The cluster administrator creates a sovereign cloud project or uses an existing one with proper naming convention:
   ```
   Project ID: eu0:openshift-cluster-project
   ```

3. The cluster administrator creates an install-config.yaml specifying the sovereign cloud's universe domain:
   ```yaml
   apiVersion: v1
   baseDomain: example.de
   metadata:
     name: my-cluster
   platform:
     gcp:
       projectID: eu0:my-project
       region: europe-west3  # Valid Germany sovereign cloud region
       universeDomain: apis-berlin-build0.goog  # Google Cloud Germany universe domain
   pullSecret: '{"auths": ...}'
   ```

4. The installer validates the install-config:
   - Recognizes non-default universe domain
   - Validates project ID format (may warn if not matching expected pattern)
   - Validates region availability via GCP API
   - Configures all GCP SDK clients with `option.WithUniverseDomain("apis-berlin-build0.goog")`

5. The installer generates manifests with sovereign cloud-specific configuration:
   - CAPI GCP cluster manifest includes universe domain configuration
   - Infrastructure CR includes `universeDomain` in both spec and status
   - Machine manifests use GCP SDK clients configured with universe domain

6. The installer provisions infrastructure using sovereign cloud APIs:
   - All GCP SDK clients initialized with `option.WithUniverseDomain("apis-berlin-build0.goog")`
   - Creates VPC, subnets, firewall rules via sovereign cloud Compute API
   - Creates service accounts and IAM bindings via sovereign cloud IAM API
   - Creates DNS records via sovereign cloud DNS API
   - Creates GCS bucket for ignition via sovereign cloud Storage API

7. Bootstrap process completes using sovereign cloud resources:
   - Bootstrap VM launches in sovereign cloud
   - Control plane nodes launch and form cluster
   - Cloud controller manager reads universe domain from Infrastructure CR and configures SDK clients

8. Cluster operators initialize with sovereign cloud awareness:
   - Machine API operator reads universe domain from Infrastructure CR
   - Cluster image registry operator creates GCS bucket using sovereign cloud APIs
   - Ingress operator creates DNS records using sovereign cloud APIs
   - All operators configure GCP SDK clients with the universe domain from Infrastructure CR

9. The cluster is fully operational on the sovereign cloud environment

#### Variation: Private Cluster Installation

For customers requiring private, air-gapped installations:

1. Create install-config with both sovereign cloud and private cluster settings:
   ```yaml
   platform:
     gcp:
       universeDomain: apis-berlin-build0.goog
       projectID: eu0:my-project
       region: europe-west3
   publish: Internal
   ```

2. Configure private service connect or VPN access to sovereign cloud APIs

3. Proceed with installation using internal-only endpoints

#### Error Handling

**Invalid Universe Domain Format**

1. User specifies a malformed universe domain in install-config
2. Installer validation fails with clear error:
   ```
   platform.gcp.universeDomain: Invalid value: "invalid domain!": must be a valid domain name
   ```
3. User corrects the domain format and retries

**Unknown Universe Domain (Warning)**

1. User specifies a universe domain that is not in the known sovereign cloud list
2. Installer validation warns but allows installation:
   ```
   Warning: platform.gcp.universeDomain: "custom-domain.goog" is not a recognized GCP sovereign cloud.
   Known sovereign cloud universe domains:
     - "apis-berlin-build0.goog" (Google Cloud Germany)
   Proceeding with installation - ensure this value is correct.
   ```
3. User can proceed if they're confident the domain is correct

**Region Not Available in Universe**

1. User specifies a region not available in the sovereign cloud
2. Validation fails with error from GCP API:
   ```
   platform.gcp.region: Invalid value: "us-central1": region is not available in the specified universe domain.
   Available regions can be queried via: gcloud compute regions list
   ```
3. User selects a valid region for their sovereign cloud

**Authentication Failure**

1. User's credentials don't have access to the specified universe domain
2. Installer fails during credential validation:
   ```
   Failed to authenticate to GCP universe domain "apis-berlin-build0.goog": credentials may not have access to sovereign cloud.
   Ensure you have authenticated using: gcloud auth login --project <project-id>
   ```
3. User authenticates with proper credentials for the sovereign cloud

### API Extensions

This enhancement modifies the install-config API and adds a new field to the Infrastructure CR.

#### Install Config Changes

Add to `pkg/types/gcp/platform.go`:

```go
// Platform stores all the global configuration that all machinesets use.
type Platform struct {
    // ... existing fields ...
    
    // UniverseDomain specifies the GCP universe domain for API endpoints.
    // When empty, uses the default "googleapis.com" (standard public GCP).
    // For sovereign clouds, set to the sovereign cloud's universe domain.
    // 
    // Examples:
    //   - "" or "googleapis.com" - Standard public GCP (default)
    //   - "apis-berlin-build0.goog" - Google Cloud Germany sovereign cloud
    // 
    // This value is passed directly to the GCP SDK via option.WithUniverseDomain().
    // See: https://cloud.google.com/docs/authentication#universe_domain
    // 
    // +optional
    // +kubebuilder:validation:MaxLength=253
    // +kubebuilder:validation:Pattern=`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
    UniverseDomain string `json:"universeDomain,omitempty"`
}

// GetUniverseDomain returns the universe domain, defaulting to googleapis.com if empty
func (p *Platform) GetUniverseDomain() string {
    if p.UniverseDomain == "" {
        return "googleapis.com"
    }
    return p.UniverseDomain
}

// IsPublicCloud returns true if using standard public GCP
func (p *Platform) IsPublicCloud() bool {
    return p.UniverseDomain == "" || p.UniverseDomain == "googleapis.com"
}
```

#### Infrastructure CR Changes (Upstream openshift/api)

Add to `config/v1/types_infrastructure.go`:

```go
// GCPPlatformSpec holds the desired state of the Google Cloud Platform infrastructure provider.
// This only includes fields that can be modified in the cluster.
type GCPPlatformSpec struct {
    // ... existing fields ...
    
    // UniverseDomain specifies the GCP universe domain for API endpoints.
    // When empty, assumes standard public GCP (googleapis.com).
    // For sovereign clouds, this should be set to the sovereign cloud's universe domain
    // (e.g., "apis-berlin-build0.goog" for Google Cloud Germany).
    // 
    // This field is immutable and should match the value from the install-config.
    // 
    // +optional
    // +kubebuilder:validation:MaxLength=253
    // +kubebuilder:validation:Pattern=`^([a-z0-9]([a-z0-9-]*[a-z0-9])?\.)*[a-z0-9]([a-z0-9-]*[a-z0-9])?$`
    UniverseDomain string `json:"universeDomain,omitempty"`
}

// GCPPlatformStatus holds the current status of the Google Cloud Platform infrastructure provider.
type GCPPlatformStatus struct {
    // ... existing fields ...
    
    // UniverseDomain is the GCP universe domain in use.
    // When empty, standard public GCP (googleapis.com) is in use.
    // 
    // +optional
    UniverseDomain string `json:"universeDomain,omitempty"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is not immediately applicable to Hypershift because:
- Hypershift management clusters typically run on public GCP
- Guest clusters on sovereign clouds would require the management cluster to access sovereign cloud APIs
- Cross-cloud API access may be restricted by sovereign cloud isolation requirements

**Future Consideration**: A separate enhancement would be needed to support:
- Hypershift management clusters on sovereign clouds
- Hypershift guest clusters on sovereign clouds from public cloud management clusters (if technically and legally permissible)

#### Standalone Clusters

This enhancement is fully applicable to standalone IPI clusters on GCP sovereign clouds. All standard IPI features should work:
- Standard installations
- Private clusters
- Shared VPC
- Custom networks

#### Single-node Deployments or MicroShift

**Single-Node OpenShift (SNO)**:
Fully supported. SNO installations on sovereign clouds work identically to multi-node clusters with the same cloud environment configuration.

**MicroShift**:
Not applicable. MicroShift does not use the OpenShift installer.

#### OpenShift Kubernetes Engine

OKE deployments that use the installer on GCP would support sovereign clouds with this enhancement. All limitations and requirements apply equally to OKE.

### Component Support Requirements

All OpenShift components that interact with GCP APIs must be updated to support universe domain. This section documents the scope of changes required.

#### Components Requiring Updates

| Component | Repository | Changes Required | Complexity |
|-----------|------------|------------------|------------|
| **Installer** | openshift/installer | Add `universeDomain` field to install-config API; pass to all GCP SDK clients | Medium |
| **OpenShift API** | openshift/api | Add `universeDomain` to Infrastructure CR GCPPlatformSpec/Status | Low |
| **Cloud Controller Manager (CCM)** | kubernetes/cloud-provider-gcp | Read universe domain from Infrastructure CR; pass to all SDK clients | Medium |
| **Machine API Operator (MAPI)** | openshift/machine-api-operator | Read universe domain from Infrastructure CR; pass to GCP actuator | Medium |
| **Cluster API Provider GCP (CAPG)** | kubernetes-sigs/cluster-api-provider-gcp | Read universe domain from Infrastructure CR; configure all SDK clients | High (Upstream) |
| **Cluster Ingress Operator** | openshift/cluster-ingress-operator | Read universe domain for DNS record management | Low |
| **Cluster Image Registry Operator** | openshift/cluster-image-registry-operator | Read universe domain for GCS bucket operations | Low |
| **GCP PD CSI Driver** | kubernetes-sigs/gcp-compute-persistent-disk-csi-driver | Configure SDK with universe domain | Medium (Upstream) |

#### Detailed Component Requirements

##### 1. OpenShift Installer
**Changes**: 
- Add `UniverseDomain string` field to `pkg/types/gcp/platform.go`
- Update all GCP client creation to pass universe domain via `option.WithUniverseDomain()`
- Set universe domain in Infrastructure CR during cluster creation
- Update validation to handle non-default universe domains

**Example locations**:
- `pkg/asset/installconfig/gcp/client.go` - Client initialization
- `pkg/asset/cluster/gcp/gcp.go` - Cluster creation
- `pkg/infrastructure/gcp/*` - Infrastructure provisioning

##### 2. Cloud Controller Manager (CCM)
**Changes**:
- Read `spec.platformSpec.gcp.universeDomain` from Infrastructure CR
- Pass universe domain to all GCP SDK clients (compute, storage)
- Used for: Load balancer creation, node management

**Impact**: Without this, load balancer services won't work in sovereign clouds.

##### 3. Machine API Operator (MAPI)
**Changes**:
- Read universe domain from Infrastructure CR
- Pass to GCP actuator when creating/updating/deleting machines
- Used for: Machine lifecycle management

**Impact**: Without this, machine scaling and management won't work.

##### 4. Cluster API Provider GCP (CAPG)
**Changes** (Upstream dependency):
- Add universe domain support to CAPG controller
- Read from Infrastructure CR or cluster configuration
- Configure all GCP SDK clients with universe domain

**Impact**: Without this, Day 2 machine management via CAPI won't work.
**Status**: Requires upstream enhancement in CAPG project.

##### 5. Cluster Ingress Operator  
**Changes**:
- Read universe domain from Infrastructure CR
- Use for DNS API operations (creating DNS records for routes)

**Impact**: Without this, DNS record creation for routes won't work.

##### 6. Cluster Image Registry Operator
**Changes**:
- Read universe domain from Infrastructure CR
- Use for GCS operations (bucket creation, object storage)

**Impact**: Without this, integrated registry won't work.

##### 7. GCP PD CSI Driver
**Changes** (Upstream dependency):
- Configure GCP SDK with universe domain
- May need enhancement in upstream CSI driver

**Impact**: Without this, persistent volume provisioning won't work.
**Status**: Requires investigation of upstream CSI driver support.

#### Component Discovery Pattern

All components should follow this pattern to read universe domain:

```go
import (
    configv1 "github.com/openshift/api/config/v1"
    "google.golang.org/api/option"
)

func getUniverseDomain(ctx context.Context, client ctrlclient.Client) (string, error) {
    infra := &configv1.Infrastructure{}
    if err := client.Get(ctx, types.NamespacedName{Name: "cluster"}, infra); err != nil {
        return "", err
    }
    
    if infra.Status.PlatformStatus == nil ||
       infra.Status.PlatformStatus.GCP == nil {
        return "googleapis.com", nil // Default
    }
    
    universeDomain := infra.Status.PlatformStatus.GCP.UniverseDomain
    if universeDomain == "" {
        return "googleapis.com", nil // Default
    }
    
    return universeDomain, nil
}

func createGCPClient(ctx context.Context, universeDomain string) (*compute.Service, error) {
    opts := []option.ClientOption{}
    if universeDomain != "" && universeDomain != "googleapis.com" {
        opts = append(opts, option.WithUniverseDomain(universeDomain))
    }
    return compute.NewService(ctx, opts...)
}
```

#### Testing Requirements

Each component must verify:
1. ✅ Works with default universe domain (public GCP) - existing CI
2. ✅ Works with non-default universe domain (sovereign cloud) - new test coverage
3. ✅ Handles missing/empty universe domain gracefully (defaults to public)
4. ✅ Provides clear error messages when sovereign cloud APIs are unavailable

#### Upstream Dependencies

**Critical Path**:
1. **CAPG** - Required for Day 2 operations
   - File enhancement: https://github.com/kubernetes-sigs/cluster-api-provider-gcp
   - Estimate: 1-2 releases

2. **GCP PD CSI Driver** - Required for storage
   - Investigate current universe domain support
   - File enhancement if needed: https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver

### Implementation Details/Notes/Constraints

#### GCP SDK Client Initialization

The key implementation change is initializing all GCP SDK clients with the universe domain.

**Current Implementation** (typical pattern across codebase):
```go
import (
    compute "google.golang.org/api/compute/v1"
)

func createComputeClient(ctx context.Context) (*compute.Service, error) {
    return compute.NewService(ctx)
}
```

**New Implementation**:
```go
import (
    compute "google.golang.org/api/compute/v1"
    "google.golang.org/api/option"
)

func createComputeClient(ctx context.Context, universeDomain string) (*compute.Service, error) {
    opts := []option.ClientOption{}
    
    // Add universe domain if not default
    if universeDomain != "" && universeDomain != "googleapis.com" {
        opts = append(opts, option.WithUniverseDomain(universeDomain))
    }
    
    return compute.NewService(ctx, opts...)
}
```

**This pattern applies to all GCP SDK clients:**
- `compute.NewService()`
- `storage.NewClient()`
- `dns.NewService()`
- `iam.NewService()`
- `cloudresourcemanager.NewService()`

**Example for installer** (`pkg/asset/installconfig/gcp/client.go`):
```go
type Client struct {
    universeDomain string
}

func NewClient(ctx context.Context, universeDomain string) (*Client, error) {
    return &Client{
        universeDomain: universeDomain,
    }, nil
}

func (c *Client) GetComputeService(ctx context.Context) (*compute.Service, error) {
    opts := []option.ClientOption{}
    if c.universeDomain != "" && c.universeDomain != "googleapis.com" {
        opts = append(opts, option.WithUniverseDomain(c.universeDomain))
    }
    return compute.NewService(ctx, opts...)
}
```

#### Service Validation

Service validation is handled automatically by the GCP SDK when configured with the universe domain. The SDK will attempt to call APIs within the specified universe domain and fail if services are unavailable.

**Current Implementation** (`pkg/asset/installconfig/gcp/validation.go`):
```go
requiredServices := []string{
    "compute.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "dns.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "serviceusage.googleapis.com",
}
```

**New Implementation**:
The GCP SDK handles service naming automatically when using `WithUniverseDomain()`. Service names remain the same (e.g., `compute`, `dns`, `iam`), but the SDK routes requests to the correct universe domain.

```go
func validateGCPServices(ctx context.Context, client *Client) error {
    // The SDK automatically uses the correct endpoints based on universe domain
    // No need to change service names
    
    // Example: Test that compute API is accessible
    computeSvc, err := client.GetComputeService(ctx)
    if err != nil {
        return fmt.Errorf("failed to create compute client for universe domain %s: %w", 
            client.universeDomain, err)
    }
    
    // Test API access
    _, err = computeSvc.Regions.List(projectID).Do()
    if err != nil {
        return fmt.Errorf("failed to access compute API in universe domain %s: %w",
            client.universeDomain, err)
    }
    
    return nil
}
```

**Note**: Some services may not be available in sovereign clouds. The installer should gracefully handle service unavailability and provide clear error messages indicating which OCP features are unavailable.

#### Region and Zone Validation

Region validation should be **dynamic** using the GCP API rather than hardcoded lists. This ensures that:
1. New regions become available automatically without code changes
2. Validation is accurate for each universe domain
3. No backports are needed when regions are added

**Implementation Strategy**:
```go
func validateRegion(ctx context.Context, region string, client *Client) error {
    computeSvc, err := client.GetComputeService(ctx)
    if err != nil {
        return fmt.Errorf("failed to create compute client: %w", err)
    }
    
    // Fetch regions from API (this automatically uses the correct universe domain)
    regionsList, err := computeSvc.Regions.List(client.projectID).Do()
    if err != nil {
        return fmt.Errorf("failed to list regions: %w", err)
    }
    
    // Check if specified region exists
    for _, r := range regionsList.Items {
        if r.Name == region {
            return nil
        }
    }
    
    // Build available regions list for error message
    available := []string{}
    for _, r := range regionsList.Items {
        available = append(available, r.Name)
    }
    
    return fmt.Errorf("region %s is not available in universe domain %s. Available regions: %v",
        region, client.universeDomain, available)
}
```

**Fallback for Offline/Air-gapped Scenarios**:
If API access fails during validation, provide helpful error message:
```go
if err != nil {
    return fmt.Errorf("cannot validate region %s: unable to query GCP API: %w. "+
        "For sovereign clouds, ensure region is valid for universe domain %s",
        region, client.universeDomain, err)
}
```

**Benefits of Dynamic Validation**:
- ✅ No hardcoded region lists to maintain
- ✅ No backports needed when regions are added
- ✅ Accurate for each universe domain
- ✅ User gets real-time region availability

#### CAPI Provider GCP (CAPG) Configuration

CAPG needs to use the universe domain when creating GCP SDK clients. The universe domain must be propagated from the Infrastructure CR to CAPG.

**Approach 1: Infrastructure CR Annotation** (Recommended)
Add the universe domain to Infrastructure CR, which CAPG can read:

```go
// In installer, when creating Infrastructure CR
func (p *Provider) createInfrastructureCR() error {
    infra := &configv1.Infrastructure{
        Spec: configv1.InfrastructureSpec{
            PlatformSpec: configv1.PlatformSpec{
                Type: configv1.GCPPlatformType,
                GCP: &configv1.GCPPlatformSpec{
                    UniverseDomain: p.platform.UniverseDomain,
                },
            },
        },
        Status: configv1.InfrastructureStatus{
            PlatformStatus: &configv1.PlatformStatus{
                Type: configv1.GCPPlatformType,
                GCP: &configv1.GCPPlatformStatus{
                    UniverseDomain: p.platform.UniverseDomain,
                },
            },
        },
    }
    // ...
}
```

**CAPG Changes Required**:
CAPG needs to read `universeDomain` from Infrastructure CR and pass it to all GCP SDK client initializations:

```go
// In CAPG controller
func (r *Reconciler) getGCPClient(ctx context.Context) error {
    // Read Infrastructure CR
    infra := &configv1.Infrastructure{}
    if err := r.Get(ctx, types.NamespacedName{Name: "cluster"}, infra); err != nil {
        return err
    }
    
    universeDomain := ""
    if infra.Status.PlatformStatus.GCP != nil {
        universeDomain = infra.Status.PlatformStatus.GCP.UniverseDomain
    }
    
    // Create compute client with universe domain
    opts := []option.ClientOption{}
    if universeDomain != "" && universeDomain != "googleapis.com" {
        opts = append(opts, option.WithUniverseDomain(universeDomain))
    }
    
    computeSvc, err := compute.NewService(ctx, opts...)
    // ...
}
```

**Note**: This requires upstream changes to CAPG. Need to file enhancement request with CAPG maintainers.

#### Project ID Format Validation

Sovereign clouds may use different project ID formats. For example:
- Public GCP: `my-project-123`
- Google Cloud Germany: `eu0:my-project-123` (prefix indicates the sovereign cloud)

**Implementation Strategy**:
Rather than enforcing strict project ID patterns based on universe domain (which could be fragile), we should:
1. Attempt to use the provided project ID
2. Let GCP API validation fail if the project ID is invalid
3. Provide helpful error messages if authentication or project access fails

```go
func validateProjectID(ctx context.Context, projectID string, universeDomain string, client *Client) error {
    // Try to access the project via GCP API
    // This validates both format and accessibility
    resourceSvc, err := client.GetResourceManagerService(ctx)
    if err != nil {
        return fmt.Errorf("failed to create resource manager client: %w", err)
    }
    
    project, err := resourceSvc.Projects.Get(projectID).Do()
    if err != nil {
        return fmt.Errorf("failed to access project %s in universe domain %s: %w. "+
            "Ensure the project exists and you have appropriate permissions", 
            projectID, universeDomain, err)
    }
    
    logrus.Infof("Successfully validated project: %s (name: %s)", project.ProjectId, project.Name)
    return nil
}
```

**Benefits**:
- Works for any sovereign cloud project ID format
- Validates actual project access, not just format
- No hardcoded assumptions about project naming conventions

#### Authentication and Credentials

**Question**: Are different credentials needed for sovereign clouds?

**Answer**: Likely yes, but the format may be the same (service account JSON key). The key differences:
1. Service accounts are created in the sovereign cloud project
2. Auth endpoints may be different (e.g., `https://oauth2.cloud.google.de/token`)
3. Scopes may be the same or slightly different

**Implementation** (`pkg/asset/installconfig/gcp/session.go`):
```go
func GetSession(ctx context.Context, cloudEnv CloudEnvironment) (*Session, error) {
    // Load credentials (same process for public and sovereign)
    creds, err := loadCredentials(ctx)
    if err != nil {
        return nil, err
    }
    
    // Configure client with appropriate endpoints
    opts := []option.ClientOption{
        option.WithCredentials(creds),
    }
    
    // Add endpoint overrides for sovereign cloud
    if cloudEnv == GermanySovereignCloud {
        opts = append(opts, 
            option.WithEndpoint("https://compute.cloud.google.de"),
            // Add other service endpoints as needed
        )
    }
    
    // Create clients with options
    computeClient, err := compute.NewService(ctx, opts...)
    if err != nil {
        return nil, err
    }
    
    return &Session{
        ComputeClient: computeClient,
        // ... other clients
    }, nil
}
```

#### Shared VPC Support

**Question**: Will shared VPC be supported?

**Answer**: Yes, with the same caveats as public GCP:
- Shared VPC host project must be in the same sovereign cloud
- Network project ID must use the same sovereign cloud format
- Regional constraints apply (shared VPC network must be in regions available to sovereign cloud)

**Implementation**: No special changes needed beyond ensuring project ID validation accepts sovereign cloud format for network project.

#### Cloud Provider Configuration

**Location**: `pkg/asset/manifests/gcp/cloudproviderconfig.go`

The in-cluster cloud provider config needs to be aware of the cloud environment:

```go
func GenerateCloudProviderConfig(platform *gcp.Platform) string {
    config := &cloudProviderConfig{
        ProjectID:   platform.ProjectID,
        NetworkName: platform.Network,
        SubnetworkName: platform.ComputeSubnet,
        // Add cloud environment info for in-cluster components
        CloudEnvironment: string(platform.CloudEnvironment),
    }
    
    return marshalConfig(config)
}
```

This ensures in-cluster components (Machine API, cloud controller manager) use the correct endpoints.

#### RHCOS Images

**Question**: How are RHCOS images handled in sovereign clouds?

**Answer**: 
- RHCOS images must be pre-loaded into the sovereign cloud by Red Hat or partners
- Images are stored in a sovereign cloud project (not the public `rhcos-cloud` project)
- Users must specify custom image project in install-config

**Implementation**:
```yaml
platform:
  gcp:
    cloudEnvironment: germany-sovereign
    projectID: eu0:my-project
    region: europe-west3
    defaultMachinePlatform:
      osImage:
        project: eu0:rhcos-images-germany  # Sovereign cloud RHCOS image project
        name: rhcos-48-x86-64
```

**Documentation Note**: Must clearly document that sovereign cloud installations require specifying custom RHCOS image projects.

### Risks and Mitigations

#### Risk: Incomplete Sovereign Cloud Documentation

**Risk**: Google Cloud sovereign cloud offerings may have limited or incomplete documentation, making it difficult to determine exact API endpoints, service names, and feature availability.

**Mitigation**:
- Partner closely with Google Cloud to obtain sovereign cloud-specific documentation
- Implement initial support for Germany sovereign cloud as reference implementation
- Create extensible architecture that can adapt as more information becomes available
- Document known limitations and unknowns clearly
- Provide fallback behavior for undocumented scenarios

#### Risk: Service Availability Differences

**Risk**: Sovereign clouds may not have all the services available in public GCP, or services may behave differently, causing installation failures.

**Mitigation**:
- Implement service availability validation specific to each sovereign cloud
- Make service validation cloud-environment-aware with appropriate fallbacks
- Document required vs. optional services for each cloud environment
- Provide clear error messages when required services are unavailable
- Consider degrading gracefully for optional service unavailability

#### Risk: Divergent API Behavior

**Risk**: Sovereign cloud APIs may have subtle differences in behavior compared to public GCP APIs, causing unexpected failures.

**Mitigation**:
- Extensive testing in actual sovereign cloud environments
- Partner with Google Cloud for API compatibility validation
- Implement defensive coding with proper error handling
- Add cloud environment-specific integration tests
- Document any known API differences

#### Risk: Region and Zone Limitations

**Risk**: Sovereign clouds have limited regional availability, which may impact multi-zone HA configurations or specific customer requirements.

**Mitigation**:
- Clearly document available regions for each sovereign cloud
- Validate region selection at install-config time with clear error messages
- Ensure installer works with the limited zone availability in sovereign clouds
- Document any HA limitations due to reduced zone availability

#### Risk: Credential and Authentication Differences

**Risk**: Authentication mechanisms or credential formats may differ between public GCP and sovereign clouds.

**Mitigation**:
- Test with actual sovereign cloud credentials
- Document any credential format differences
- Implement flexible credential loading that works across cloud environments
- Provide clear error messages for authentication failures
- Work with Google Cloud to ensure credential compatibility

#### Risk: CAPI Provider Compatibility

**Risk**: CAPG (Cluster API Provider GCP) may not fully support sovereign cloud endpoints or may require upstream changes.

**Mitigation**:
- Analyze CAPG endpoint override capabilities
- Contribute upstream to CAPG if enhancements are needed
- Implement workarounds in installer if CAPG limitations exist
- Document CAPG version requirements for sovereign cloud support
- Consider forking CAPG temporarily if critical upstream changes are delayed

#### Risk: Upgrade Path Complexity

**Risk**: Clusters cannot migrate between cloud environments (e.g., public to sovereign), and upgrades may be complex if cloud environment affects core infrastructure.

**Mitigation**:
- Clearly document that cloud environment cannot be changed post-installation
- Ensure cloud environment is immutable in Infrastructure CR
- Validate upgrade paths thoroughly in sovereign cloud environments
- Document any sovereign cloud-specific upgrade considerations

### Drawbacks

1. **Increased Complexity**: Supporting multiple cloud environments adds complexity to codebase, validation, and testing.

2. **Limited Testing**: Sovereign cloud environments may have limited availability for CI/CD testing, making it harder to validate changes.

3. **Documentation Burden**: Each sovereign cloud has its own quirks and requirements that must be documented separately.

4. **Maintenance Overhead**: Each new sovereign cloud offering requires analysis, implementation, and ongoing maintenance.

5. **Feature Parity**: Sovereign clouds may lag behind public GCP in feature availability, limiting what OpenShift features can be offered.

6. **Upstream Dependencies**: CAPG and other upstream components may need changes, creating coordination overhead.

## Alternatives (Not Implemented)

### Alternative 1: Create Separate Platform Type

Instead of adding `cloudEnvironment` to GCP platform, create a new platform type (e.g., `platform.gcpSovereign`).

**Pros**:
- Clear separation between public and sovereign cloud configurations
- No risk of accidentally mixing configurations
- Easier to have completely different validation logic

**Cons**:
- Massive code duplication between GCP and GCP Sovereign implementations
- Harder to maintain feature parity
- Confusing for users ("is this GCP or not?")
- Doesn't scale well to multiple sovereign clouds (need a new platform type for each?)
- Inconsistent with other cloud providers (AWS has single platform for multiple partitions)

**Rejected because**: The AWS partition support pattern (single platform, environment field) is more maintainable and scales better to multiple sovereign clouds.

### Alternative 2: Rely Solely on Custom Endpoints (PSC)

Use the existing Private Service Connect (PSC) endpoint override mechanism instead of adding cloud environment awareness.

**Pros**:
- No new API fields needed
- Reuses existing functionality

**Cons**:
- PSC only supports one endpoint override per installation
- Doesn't address service name differences
- Doesn't help with region validation
- Doesn't support project ID format differences
- Very user-unfriendly (requires knowing exact endpoint URLs for each service)
- Doesn't scale to multiple sovereign clouds with different requirements

**Rejected because**: PSC is too limited and requires users to have deep knowledge of sovereign cloud internals. A first-class cloud environment field provides better UX.

### Alternative 3: Automatic Cloud Environment Detection

Automatically detect sovereign cloud based on project ID format or API endpoint responses.

**Pros**:
- No configuration needed from user
- Simpler install-config

**Cons**:
- Fragile (detection logic could break if formats change)
- Unclear error messages when detection fails
- Doesn't handle edge cases (e.g., migrated projects with unexpected formats)
- Makes debugging harder (implicit rather than explicit configuration)
- Increases code complexity with detection heuristics

**Rejected because**: Explicit configuration is more robust and easier to debug than automatic detection.

### Alternative 4: Post-Installation Cloud Environment Configuration

Support only public GCP installation initially, then allow cloud environment to be changed post-installation.

**Pros**:
- Simpler initial implementation
- Phased rollout of feature

**Cons**:
- Cloud environment is fundamentally immutable (can't change API endpoints for existing infrastructure)
- Would require full cluster recreation to change environment
- Misleading to users if the feature doesn't actually work
- Doesn't solve the core problem (still can't install on sovereign cloud)

**Rejected because**: This doesn't address the actual requirement and would provide false hope to users.

## Open Questions

1. **What are the exact API endpoint formats for Germany Sovereign Cloud?**
   - Need official documentation from Google Cloud
   - May vary by service
   - Proposed format: `https://{service}.{region}.cloud.google.de/`

2. **What services are available in Germany Sovereign Cloud?**
   - Is `serviceusage.googleapis.com` available?
   - Are all required services available?
   - Need comprehensive service availability matrix

3. **How does authentication work for sovereign clouds?**
   - Same service account JSON format?
   - Different auth endpoints?
   - Different token scopes?

4. **What is the RHCOS image distribution model?**
   - Does Red Hat maintain an image project in sovereign clouds?
   - Do customers need to upload their own images?
   - What is the image update process?

5. **Are there other GCP sovereign cloud offerings beyond Germany?**
   - Timeline for additional sovereign clouds?
   - How similar/different are they from Germany sovereign cloud?

6. **Does CAPG fully support endpoint overrides for all required services?**
   - What CAPG version is needed?
   - Are upstream changes required?

7. **What regions are available in Germany Sovereign Cloud?**
   - Confirmed list of regions and zones
   - Roadmap for new region availability

8. **Is Shared VPC supported in sovereign clouds?**
   - Any differences from public GCP?
   - Cross-project constraints?

9. **What compliance certifications does Germany Sovereign Cloud have?**
   - Helps with documentation and customer communication
   - May affect feature availability

10. **Is there a sandbox/test environment for Germany Sovereign Cloud?**
    - Critical for CI/CD and testing
    - Access requirements for development team

## GCD-Specific Constraints and Requirements

Based on analysis of Google Cloud Dedicated (GCD) in Germany TPC (Trusted Partner Cloud) differences documentation, the following constraints and requirements must be addressed in the implementation:

### Load Balancing Constraints

#### Available Load Balancer Types

GCD has a **single region** (Germany) with multiple zones, which means **only regional load balancers** are available:

**Supported in GCD:**
- Regional internal Application Load Balancer
- Regional external Application Load Balancer  
- Regional internal proxy Network Load Balancer
- Regional external proxy Network Load Balancer
- Internal passthrough Network Load Balancer
- External passthrough Network Load Balancer

**NOT Available in GCD:**
- Global load balancers (any type)
- Classic load balancers
- Multi-region load balancing
- Cross-region failover

#### Load Balancer Resource Constraints

**Available:**
- Regional IP addresses
- Regional backend services
- Regional forwarding rules
- Regional target proxies
- Regional URL maps
- Legacy global HTTP health checks (for target pool-based external passthrough NLBs)
- Firewall rules (always global)

**NOT Available:**
- Global versions of the above resources (except firewall rules and legacy health checks)
- Backend buckets referencing Cloud Storage
- Serverless NEGs (Cloud Run, Cloud Run functions, App Engine not available in GCD)
- Global internet NEGs

#### Networking Constraints

**Network Service Tiers:**
- Standard Tier is NOT available in GCD
- All load balancers and resources must use **Premium Tier only**

**VPC Networks:**
- Single region means auto mode networks contain only one subnet
- No default network created automatically - must be created manually if needed
- Service load balancing policies NOT available
- In-flight balancing mode NOT available
- Custom metrics balancing mode NOT available
- Zonal affinity for internal passthrough NLBs NOT available

**Interconnect:**
- Partner Interconnect NOT available
- Cross-Cloud Interconnect NOT available

#### SSL/TLS Certificate Constraints

**NOT Available:**
- Certificate Manager certificates and certificate maps
- Compute Engine Google-managed certificates (both global and regional)
- Compute Engine self-managed certificates (global only)

**Available:**
- Self-managed regional SSL certificates only

**Impact:** Applications must manage their own certificates and use regional certificate resources.

#### Security and Integration Constraints

**NOT Available:**
- Authorization policies
- Frontend and backend mTLS
- Global SSL policies
- Cloud CDN
- Media CDN
- Certificate Manager
- Service Extensions
- Network Intelligence Center
- Cloud Service Mesh

**Available:**
- Cloud Armor (see Cloud Armor documentation for GCD-specific features)

### DNS Constraints

#### Available DNS Zone Types

**Private DNS Zones (Available):**
- DNS forwarding zones ✓
- DNS peering zones ✓
- Managed reverse lookup zones ✓
- Service Directory zones ✓

**All DNS server policy configurations available ✓**

**All response policy zone configurations supported ✓**

#### NOT Available

- **Public DNS zones** - GCD only supports private DNS zones
- **Reverse DNS lookup** of public IPv4 and IPv6 addresses
- Multi-region DNS features

**Impact:** For clusters requiring public DNS resolution, an external DNS provider must be used. Internal cluster DNS can use private zones.

### Implementation Requirements

#### cluster-api-provider-gcp (CAPG) Changes Required

**Current State:**
- CAPG creates **Global External Proxy Load Balancer** by default for external API server access
  - Uses global health checks (`createOrGetHealthCheck`)
  - Uses global backend services (`createOrGetBackendService`)
  - Uses global target TCP proxy
  - Uses global addresses (`createOrGetAddress`)
  - Uses global forwarding rules (`createOrGetForwardingRule`)

**Required Changes for GCD:**
- External load balancer must use **regional resources** instead of global
- Must use regional health checks (`createOrGetRegionalHealthCheck`)
- Must use regional backend services (`createOrGetRegionalBackendService`)
- Must use regional addresses (already available as `createOrGetInternalAddress`)
- Must use regional forwarding rules (`createOrGetRegionalForwardingRule`)
- Must use regional target TCP proxy (may need to be created)

**Code Location:**
- `/cloud/services/compute/loadbalancers/reconcile.go` - Main reconciliation logic
- Line 190: `createExternalLoadBalancer` function creates global resources
- Line 258: `createInternalLoadBalancer` function creates regional resources (reference implementation)

**Implementation Approach:**
1. Detect cloud environment (GCD vs public GCP) via `cloudEnvironment` field
2. When `cloudEnvironment: germany-sovereign`, modify `createExternalLoadBalancer` to use regional resource methods
3. Reuse existing regional resource methods from `createInternalLoadBalancer` implementation
4. Ensure endpoint configuration points to `cloud.google.de` domain

**CAPG Endpoint Override Support:**
- CAPG already supports endpoint overrides via `ServiceEndpoints` in GCPCluster spec
- `ComputeServiceEndpoint`, `ContainerServiceEndpoint`, `IAMServiceEndpoint`, `ResourceManagerServiceEndpoint`
- Installer already uses this mechanism for PSC endpoints (line 171-178 of `pkg/asset/manifests/gcp/cluster.go`)

##### Detailed Load Balancer Creation Changes in cluster-api-provider-gcp

**Problem Statement:**

The current CAPG implementation in `/cloud/services/compute/loadbalancers/reconcile.go` hardcodes the creation of **Global** load balancer resources for external API server access. GCD does not support global resources - all load balancers must be regional. This requires significant changes to the load balancer creation logic.

**Current External Load Balancer Creation Flow:**

From `createExternalLoadBalancer()` function (line 190-256):

```go
func (s *Service) createExternalLoadBalancer(ctx context.Context, lbType infrav1.LoadBalancerType, instancegroups []*compute.InstanceGroup) error {
    name := infrav1.APIServerRoleTagValue
    
    // 1. Create GLOBAL health check
    healthcheck, err := s.createOrGetHealthCheck(ctx, name)  // Line 193 - GLOBAL
    
    // 2. Create GLOBAL backend service  
    backendsvc, err := s.createOrGetBackendService(ctx, name, mode, instancegroups, healthcheck)  // Line 206 - GLOBAL
    
    // 3. Create GLOBAL target TCP proxy
    target, err := s.createOrGetTargetTCPProxy(ctx, backendsvc)  // Line 213 - GLOBAL
    
    // 4. Create GLOBAL address
    addr, err := s.createOrGetAddress(ctx, name)  // Line 219 - GLOBAL
    
    // 5. Create GLOBAL forwarding rule
    forwardingrule, err := s.createOrGetForwardingRule(ctx, name, target, addr)  // Line 229 - GLOBAL
}
```

**Key Issue:** Each of these functions uses `meta.GlobalKey()` to create global resources:

- Line 364: `key := meta.GlobalKey(healthcheckSpec.Name)` in `createOrGetHealthCheck`
- Line 429: `key := meta.GlobalKey(backendSpec.Name)` in `createOrGetBackendService`
- Global target TCP proxy creation
- Line 600: `key := meta.GlobalKey(addressSpec.Name)` in `createOrGetAddress`
- Line 736: `key := meta.GlobalKey(forwardingRuleSpec.Name)` in `createOrGetForwardingRule`

**Existing Regional Implementation Reference:**

CAPG already implements regional load balancers for the internal API server in `createInternalLoadBalancer()` (line 258-296):

```go
func (s *Service) createInternalLoadBalancer(ctx context.Context, name string, lbType infrav1.LoadBalancerType, instancegroups []*compute.InstanceGroup) error {
    // 1. Create REGIONAL health check
    healthcheck, err := s.createOrGetRegionalHealthCheck(ctx, name)  // Line 261 - REGIONAL
    
    // 2. Create REGIONAL backend service
    backendsvc, err := s.createOrGetRegionalBackendService(ctx, name, instancegroups, healthcheck)  // Line 268 - REGIONAL
    
    // 3. Create REGIONAL address
    addr, err := s.createOrGetInternalAddress(ctx, name)  // Line 280 - REGIONAL
    
    // 4. Create REGIONAL forwarding rule (passthrough - no target proxy)
    forwardingrule, err := s.createOrGetRegionalForwardingRule(ctx, name, backendsvc, addr)  // Line 289 - REGIONAL
}
```

**Critical Difference:** Internal LB uses passthrough mode (no target proxy), but external API server requires a proxy load balancer with a target TCP proxy.

**Required Changes:**

**Option 1: Add Regional External LB Path (Recommended)**

Add a new function `createRegionalExternalLoadBalancer()` that mirrors `createExternalLoadBalancer()` but uses regional resources:

```go
func (s *Service) createRegionalExternalLoadBalancer(ctx context.Context, lbType infrav1.LoadBalancerType, instancegroups []*compute.InstanceGroup) error {
    name := infrav1.APIServerRoleTagValue
    
    // 1. Create REGIONAL health check (already exists)
    healthcheck, err := s.createOrGetRegionalHealthCheck(ctx, name)
    
    // 2. Create REGIONAL backend service (already exists)
    mode := loadBalancingModeUtilization
    if lbType == infrav1.InternalExternal {
        mode = loadBalancingModeConnection
    }
    backendsvc, err := s.createOrGetRegionalBackendService(ctx, name, instancegroups, healthcheck)
    
    // 3. Create REGIONAL target TCP proxy (NEW - needs implementation)
    target, err := s.createOrGetRegionalTargetTCPProxy(ctx, backendsvc)
    
    // 4. Create REGIONAL address (already exists as createOrGetInternalAddress, may need rename)
    addr, err := s.createOrGetRegionalAddress(ctx, name)
    
    // 5. Create REGIONAL forwarding rule to target proxy (may need modification)
    forwardingrule, err := s.createOrGetRegionalForwardingRuleWithProxy(ctx, name, target, addr)
    
    // Set endpoint and network references
    endpoint := s.scope.ControlPlaneEndpoint()
    endpoint.Host = addr.Address
    s.scope.SetControlPlaneEndpoint(endpoint)
    
    return nil
}
```

**New Functions Needed:**

1. **`createOrGetRegionalTargetTCPProxy()`** - Does not currently exist
   - Create regional target TCP proxy resource
   - Use `meta.RegionalKey(name, s.scope.Region())` instead of `meta.GlobalKey(name)`
   - Call `s.regionaltargetproxies.Insert()` (may need to add this interface)

2. **`createOrGetRegionalAddress()`** - Rename/adapt existing `createOrGetInternalAddress()`
   - Already exists but named for "internal" use
   - May need to support external regional addresses vs internal subnet addresses

3. **`createOrGetRegionalForwardingRuleWithProxy()`** - Modify existing regional forwarding rule
   - Existing `createOrGetRegionalForwardingRule()` takes backend service for passthrough
   - Need version that takes target proxy for proxy load balancer
   - Set `Target` field to target proxy SelfLink instead of `BackendService`

**Changes to Service Interface:**

The `Service` struct needs access to regional target proxy operations:

```go
type Service struct {
    // ... existing fields ...
    
    // Add regional target TCP proxy interface
    regionaltargetproxies TargetTCPProxies  // NEW FIELD
}
```

**Changes to Reconcile Logic:**

Modify the `Reconcile()` function (line 50-81) to choose between global and regional based on cloud environment:

```go
func (s *Service) Reconcile(ctx context.Context) error {
    instancegroups, err := s.createOrGetInstanceGroups(ctx)
    if err != nil {
        return err
    }
    
    lbSpec := s.scope.LoadBalancer()
    lbType := ptr.Deref(lbSpec.LoadBalancerType, infrav1.External)
    
    // Determine if sovereign cloud (regional-only)
    isSovereignCloud := s.scope.IsSovereignCloud() // NEW METHOD NEEDED
    
    if lbType == infrav1.External || lbType == infrav1.InternalExternal {
        if isSovereignCloud {
            // Use regional external LB for sovereign clouds
            if err = s.createRegionalExternalLoadBalancer(ctx, lbType, instancegroups); err != nil {
                return err
            }
        } else {
            // Use global external LB for public GCP
            if err = s.createExternalLoadBalancer(ctx, lbType, instancegroups); err != nil {
                return err
            }
        }
    }
    
    if lbType == infrav1.Internal || lbType == infrav1.InternalExternal {
        // Internal LB is always regional
        name := infrav1.InternalRoleTagValue
        if lbSpec.InternalLoadBalancer != nil {
            name = ptr.Deref(lbSpec.InternalLoadBalancer.Name, infrav1.InternalRoleTagValue)
        }
        if err = s.createInternalLoadBalancer(ctx, name, lbType, instancegroups); err != nil {
            return err
        }
    }
    
    return nil
}
```

**Cloud Environment Detection:**

Add method to scope to detect sovereign cloud:

```go
// In cloud/scope/cluster.go

func (s *ClusterScope) IsSovereignCloud() bool {
    // Check GCPCluster spec for cloud environment indicator
    // This could be from:
    // 1. A new field in GCPCluster spec: CloudEnvironment
    // 2. ServiceEndpoints configuration (if using cloud.google.de)
    // 3. A label or annotation
    
    if s.GCPCluster.Spec.ServiceEndpoints != nil {
        if strings.Contains(s.GCPCluster.Spec.ServiceEndpoints.ComputeServiceEndpoint, "cloud.google.de") {
            return true
        }
    }
    
    return false
}
```

**Option 2: Modify Existing Functions to be Environment-Aware (Alternative)**

Instead of creating new functions, modify existing functions to create regional or global resources based on cloud environment:

```go
func (s *Service) createOrGetHealthCheck(ctx context.Context, lbname string) (*compute.HealthCheck, error) {
    healthcheckSpec := s.scope.HealthCheckSpec(lbname)
    
    if s.scope.IsSovereignCloud() {
        // Use regional key and regional API
        healthcheckSpec.Region = s.scope.Region()
        key := meta.RegionalKey(healthcheckSpec.Name, s.scope.Region())
        return s.regionalhealthchecks.Get(ctx, key)
        // ... create if not found using s.regionalhealthchecks.Insert()
    } else {
        // Use global key and global API
        key := meta.GlobalKey(healthcheckSpec.Name)
        return s.healthchecks.Get(ctx, key)
        // ... create if not found using s.healthchecks.Insert()
    }
}
```

**Recommendation:** Option 1 is preferred because:
- Clearer separation of concerns
- Easier to test and validate
- Less risk of breaking existing public GCP functionality
- More maintainable as differences accumulate

**Additional Considerations:**

1. **Deletion Logic:** Must also update deletion functions:
   - `deleteExternalLoadBalancer()` (line 112-152)
   - Must detect cloud environment and delete regional resources for sovereign clouds

2. **Dual Stack Support:** 
   - GCD may have different IPv6 support
   - IPv6 address creation (line 237-250) may need adaptation

3. **Load Balancing Mode:**
   - Internal LB uses `loadBalancingModeConnection` (required for proxy LBs)
   - External LB uses `loadBalancingModeUtilization` by default
   - For `InternalExternal` type, must use `Connection` mode (line 202-205)

4. **Network Tier:**
   - All GCD resources use Premium Tier automatically
   - No need to specify tier, but verify no errors if tier is set

**Testing Requirements:**

1. Create test cluster with sovereign cloud configuration
2. Verify regional external LB is created (not global)
3. Verify all components (health check, backend service, target proxy, address, forwarding rule) are regional
4. Verify API endpoint is accessible via regional LB
5. Verify control plane endpoint is correctly set
6. Test deletion properly removes regional resources
7. Test dual stack if supported in GCD

#### cluster-ingress-operator Changes Required

**Current State:**
- Ingress operator sets Kubernetes Service annotations for GCP:
  - `cloud.google.com/load-balancer-type: Internal` (for internal LBs)
  - `networking.gke.io/internal-load-balancer-allow-global-access` (for global access)
- Load balancer creation is handled by Kubernetes cloud provider based on Service annotations
- Operator does not directly create GCP load balancer resources

**Required Changes for GCD:**
- Remove or adapt `networking.gke.io/internal-load-balancer-allow-global-access` annotation
  - Global access feature may not be available or may work differently in single-region GCD
- Ensure load balancer type annotations are compatible with regional-only LBs
- Verify health check configurations are compatible with GCD LB constraints
  - GCP LBs are set to "3 fail @ 8s interval, 1 healthy" (line 555 of `load_balancer_service.go`)

**Code Location:**
- `/pkg/operator/controller/ingress/load_balancer_service.go`
  - Line 118: `gcpLBTypeAnnotation = "cloud.google.com/load-balancer-type"`
  - Line 122: `GCPGlobalAccessAnnotation = "networking.gke.io/internal-load-balancer-allow-global-access"`
  - Line 435-440: GCP Global Access annotation handling

**Testing Requirements:**
- Verify ingress load balancers are created as regional external Application Load Balancers
- Confirm health checks work correctly
- Test internal ingress with regional internal Application Load Balancers

#### installer Changes Required

**Current State:**
- Installer sets `LoadBalancerType` in CAPG GCPCluster spec:
  - `InternalExternal` for external publishing (creates both external and internal LBs)
  - `Internal` for internal publishing (creates internal LB only)
- Supports custom service endpoints via `ServiceEndpoints` field for PSC

**Required Changes for GCD:**
1. **Service Endpoint Configuration:**
   - When `cloudEnvironment: germany-sovereign`, set all service endpoints to use `cloud.google.de` domain
   - Format: `https://{service}.{region}.cloud.google.de/` (to be confirmed with GCD documentation)
   - Update endpoint generation logic in `pkg/asset/manifests/gcp/cluster.go` (lines 171-178)

2. **Load Balancer Type Validation:**
   - Ensure load balancer types are compatible with regional-only constraints
   - External publishing must create regional external LB, not global

3. **Region and Zone Validation:**
   - Update region validation to accept GCD-specific regions
   - Available regions: `europe-west3` (Frankfurt), potentially `europe-west4` (Netherlands)
   - Must validate against GCD-specific region list, not public GCP regions

4. **Service Validation:**
   - Update required services list for GCD
   - Service names may differ: `compute.cloud.google.de` vs `compute.googleapis.com`
   - Some services may not be available (e.g., `serviceusage.googleapis.com`)

5. **Network Configuration:**
   - Document that no default network is created
   - Provide guidance on creating auto mode network named "default" if needed
   - All networks automatically use Premium Tier (no tier selection needed)

6. **DNS Configuration:**
   - Validate that only private DNS zones are used for GCD installations
   - Block or warn if public DNS zone configuration is attempted
   - Document requirement for external DNS provider for public-facing DNS

**Code Locations:**
- `pkg/asset/manifests/gcp/cluster.go` - CAPG cluster manifest generation
- `pkg/asset/installconfig/gcp/validation.go` - Region and service validation
- `pkg/types/gcp/validation/platform.go` - Platform configuration validation

#### Required Updates Summary

**High Priority (Blockers for GCD Support):**
1. ✅ Add `cloudEnvironment` field to install-config API and Infrastructure CR
2. ✅ Implement service endpoint override for GCD domains
3. ⚠️ **CRITICAL: Modify CAPG to create regional external load balancers for GCD** (currently creates global)
4. ✅ Update region validation for GCD-specific regions
5. ✅ Update service validation for GCD-specific services

**Medium Priority (Required for Full Functionality):**
1. Update cluster-ingress-operator to handle regional-only LB constraints
2. Document SSL certificate management for self-managed regional certificates
3. Validate and document Premium Tier networking behavior
4. Create migration guides for workloads using unsupported features

**Low Priority (Documentation and Guidance):**
1. Document DNS limitations (private zones only)
2. Document missing features and alternatives (CDN, Certificate Manager, etc.)
3. Create troubleshooting guides for GCD-specific issues
4. Document Cloud Armor integration specifics for GCD

### Updated Open Questions with Answers

Based on TPC differences documentation analysis:

**ANSWERED:**

1. **What regions are available in Germany Sovereign Cloud?**
   - **Answer:** Single region in Germany with multiple zones
   - Primary region: `europe-west3` (Frankfurt)
   - Possibly: `europe-west4` (Netherlands)
   - **Action:** Confirm exact region codes and zone availability with Google

2. **What services are available in Germany Sovereign Cloud?**
   - **Answer:** Limited service availability compared to public GCP
   - Confirmed NOT available: Cloud CDN, Media CDN, Certificate Manager, Service Extensions, Network Intelligence Center, Cloud Service Mesh, serverless platforms (Cloud Run, Functions, App Engine)
   - Service names use different domain: `*.cloud.google.de` vs `*.googleapis.com`
   - **Action:** Get comprehensive service availability matrix from Google

3. **What are the exact API endpoint formats for Germany Sovereign Cloud?**
   - **Answer:** Different domain structure
   - Public GCP: `https://{service}-{region}.p.googleapis.com/`
   - GCD: Likely `https://{service}.{region}.cloud.google.de/` or `https://{service}.cloud.google.de/`
   - **Action:** Confirm exact format with Google Cloud Germany documentation

4. **Does CAPG fully support endpoint overrides for all required services?**
   - **Answer:** Partial support
   - CAPG supports: `ComputeServiceEndpoint`, `ContainerServiceEndpoint`, `IAMServiceEndpoint`, `ResourceManagerServiceEndpoint`
   - **Issue:** CAPG currently hardcoded to create global LB resources for external API
   - **Action:** Modify CAPG to support regional external load balancers based on cloud environment

**STILL OPEN:**

1. **How does authentication work for sovereign clouds?**
   - Same service account JSON format?
   - Different auth endpoints (e.g., `https://oauth2.cloud.google.de/token`)?
   - Different token scopes?

2. **What is the RHCOS image distribution model?**
   - Does Red Hat maintain an image project in sovereign clouds?
   - Do customers need to upload their own images?
   - What is the image update process?

3. **Are there other GCP sovereign cloud offerings beyond Germany?**
   - Timeline for additional sovereign clouds?
   - How similar/different are they from Germany sovereign cloud?

4. **Is Shared VPC supported in sovereign clouds?**
   - Appears to be supported based on TPC docs
   - Any cross-project constraints specific to GCD?
   - **Action:** Validate shared VPC functionality in test environment

5. **What compliance certifications does Germany Sovereign Cloud have?**
   - Helps with documentation and customer communication
   - May affect feature availability

6. **Is there a sandbox/test environment for Germany Sovereign Cloud?**
   - Critical for CI/CD and testing
   - Access requirements for development team

## Test Plan

### Unit Tests

1. **Cloud environment validation tests** (`pkg/types/gcp/validation/platform_test.go`):
   - Valid cloud environment values accepted
   - Invalid cloud environment values rejected
   - Project ID format validation per cloud environment
   - Region validation per cloud environment

2. **Endpoint generation tests** (`pkg/asset/installconfig/gcp/services_test.go`):
   - Correct endpoints generated for public cloud
   - Correct endpoints generated for Germany sovereign cloud
   - Correct endpoints generated for unknown cloud (fallback to public)

3. **Service name resolution tests**:
   - Correct service names for each cloud environment
   - Service availability validation per cloud

4. **Client initialization tests**:
   - Correct endpoint configuration per cloud environment
   - Authentication works for each cloud type

### Integration Tests

1. **Install-config validation integration test**:
   - Create install-config with Germany sovereign cloud
   - Validation passes with correct project ID format
   - Validation fails with incorrect region for sovereign cloud
   - Validation fails with public GCP project ID format

2. **Manifest generation integration test**:
   - Generate manifests for Germany sovereign cloud installation
   - Verify CAPI GCPCluster has correct endpoint configuration
   - Verify Infrastructure CR has correct cloud environment
   - Verify cloud provider config has cloud environment specified

3. **Mock API integration test**:
   - Test installer logic against mock sovereign cloud APIs
   - Verify correct endpoint calls
   - Verify correct authentication flow

### End-to-End Tests

1. **Germany Sovereign Cloud full installation** (requires actual access):
   - Create service account in Germany sovereign cloud
   - Create install-config with `cloudEnvironment: germany-sovereign`
   - Run full cluster installation
   - Verify all infrastructure created correctly
   - Verify cluster is functional
   - Verify in-cluster operators work correctly
   - Verify machinesets can scale
   - Test cluster upgrade

2. **Private cluster on sovereign cloud**:
   - Install with `publish: Internal` on Germany sovereign cloud
   - Verify private cluster functionality
   - Verify no public endpoints created

3. **Shared VPC on sovereign cloud**:
   - Pre-create shared VPC in sovereign cloud host project
   - Install cluster using shared VPC
   - Verify networking works correctly

4. **Backward compatibility test**:
   - Install cluster without specifying cloud environment
   - Verify defaults to public GCP
   - Verify installation completes successfully

### Manual Testing

1. Test with different region configurations for Germany sovereign cloud
2. Test with malformed project IDs
3. Test with credentials from wrong cloud environment
4. Test error messages for all failure scenarios
5. Verify documentation accuracy with actual installation

### CI/CD Considerations

**Challenge**: Sovereign cloud environments may not be accessible from public CI/CD systems.

**Mitigation**:
- Implement comprehensive unit and mock integration tests
- Establish periodic manual E2E testing process
- Consider dedicated CI infrastructure in sovereign cloud (if technically possible)
- Use nightly or weekly testing cadence for E2E tests
- Partner with customers or Google Cloud for testing access

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to install OpenShift on Germany Sovereign Cloud end-to-end
- End user documentation available for Germany sovereign cloud
- Sufficient unit and integration test coverage
- At least one E2E test run on actual Germany sovereign cloud environment
- Known limitations clearly documented
- Support for standard features (IPI installation) validated
- Gather feedback from early adopters with sovereign cloud access
- CAPG compatibility validated

### Tech Preview -> GA

- Multiple successful customer installations on Germany sovereign cloud
- Sufficient time for feedback (minimum 2 releases in Tech Preview)
- Support for key features validated:
  - Private clusters
  - Shared VPC
  - Standard machine scaling
  - Cluster upgrades
- Comprehensive testing coverage including:
  - Regular E2E testing on Germany sovereign cloud
  - Upgrade testing
  - Scale testing
- User-facing documentation in openshift-docs
- Support procedures and runbooks created
- Performance validated as equivalent to public GCP
- Clear SLA commitments for sovereign cloud support

**For GA, the following must be demonstrated:**
- Production-ready installations with customer success stories
- No known critical bugs specific to sovereign cloud
- Support team trained on sovereign cloud troubleshooting
- Monitoring and alerting work correctly on sovereign cloud
- Backup/restore procedures validated
- Disaster recovery procedures documented and tested

### Adding Additional Sovereign Clouds

Once Germany sovereign cloud is GA, adding additional sovereign clouds follows this process:

1. Add new CloudEnvironment constant
2. Implement endpoint mapping for new cloud
3. Add region validation for new cloud
4. Add unit tests for new cloud
5. Run E2E tests on new cloud
6. Update documentation
7. Release as Tech Preview
8. Graduate to GA after validation period

### Removing a deprecated feature

This enhancement does not deprecate or remove any existing features. The `cloudEnvironment` field is a new addition that defaults to public GCP when unset, maintaining full backward compatibility with existing installations.

## Upgrade / Downgrade Strategy

### Upgrade

**Upgrading TO a release with this feature:**
- Existing public GCP clusters are unaffected
- Cloud environment defaults to public GCP (empty string)
- No changes needed to existing install-configs
- New installations can opt-in to sovereign cloud

**Upgrading FROM a sovereign cloud cluster:**
- Cloud environment remains set in Infrastructure CR
- All components continue using sovereign cloud endpoints
- Upgrade process identical to public GCP upgrades
- No special handling needed during upgrade

**Important**: Cloud environment is immutable. Once set during installation, it cannot be changed.

### Downgrade

**Downgrading TO a release without this feature:**
- Sovereign cloud clusters would break (endpoints not configured)
- **DO NOT SUPPORT DOWNGRADE** from releases with sovereign cloud to releases without
- Document clearly that sovereign cloud clusters require minimum version

**Mitigation**: Set minimum cluster version for sovereign cloud installations. Prevent downgrades below this version.

## Version Skew Strategy

This enhancement has minimal version skew concerns:

1. **Installer vs Cluster**: The installer runs once. Version skew does not apply.

2. **In-Cluster Components**: All in-cluster components (Machine API, cloud controller manager) receive cloud environment configuration via:
   - Infrastructure CR (cloud environment field)
   - Cloud provider config (includes cloud environment)
   - Machine manifests (include cloud environment context)

3. **CAPG Version**: Must ensure CAPG version supports endpoint overrides. Document minimum CAPG version.

4. **Component Updates**: If in-cluster component is updated independently, it reads cloud environment from Infrastructure CR, ensuring compatibility.

5. **Control Plane vs Workers**: No version skew issues as all nodes use the same cloud environment.

## Operational Aspects of API Extensions

This enhancement adds a field to the Infrastructure CR spec and status but does not add webhooks, finalizers, or other API extensions.

### Infrastructure CR Impact

**Field Addition**: 
- `spec.platformSpec.gcp.cloudEnvironment` (optional)
- `status.platformStatus.gcp.cloudEnvironment` (optional)

**Impact on existing systems**:
- No impact on API throughput or availability
- Field is optional and backward compatible
- No validation webhooks required (validation in installer)
- No finalizers or blocking operations

**Failure Modes**:
- If cloud environment not set: Defaults to public GCP (safe)
- If invalid cloud environment set: Installation fails with clear error (fail-fast)
- If cloud environment set but components don't support it: Components would use public endpoints, likely failing API calls (detected during installation)

## Support Procedures

### Detecting Failures

**Symptom**: Installer fails with "connection refused" or "unknown host" errors

**Detection**:
- Error messages reference `googleapis.com` when expecting `cloud.google.de`
- DNS resolution failures for sovereign cloud endpoints
- Logs show: "Failed to connect to https://compute-europe-west3.p.googleapis.com"

**Resolution**:
1. Verify `cloudEnvironment` is set correctly in install-config.yaml
2. Verify network access to sovereign cloud endpoints
3. Verify credentials are for sovereign cloud project
4. Check project ID format matches cloud environment

**Symptom**: Validation fails with "region not available"

**Detection**:
- Error message: "region 'us-central1' not available in germany-sovereign cloud"
- Region is valid for public GCP but not for sovereign cloud

**Resolution**:
1. Check available regions for the sovereign cloud
2. Update install-config.yaml with valid sovereign cloud region
3. Refer to documentation for region availability

**Symptom**: Service validation fails

**Detection**:
- Error message: "Required service not available: serviceusage.googleapis.com"
- Service name uses public GCP format when expecting sovereign cloud format

**Resolution**:
1. Verify cloud environment is set correctly
2. Check service availability in sovereign cloud documentation
3. If service is genuinely unavailable, check if installation can proceed without it

**Symptom**: Authentication failures

**Detection**:
- Error message: "oauth2: cannot fetch token"
- Error message: "invalid_grant" or similar OAuth errors
- Logs show auth endpoint mismatch

**Resolution**:
1. Verify credentials are for correct cloud environment
2. Verify service account exists in sovereign cloud project
3. Check credential file format
4. Verify auth endpoints are correct for sovereign cloud

### Disabling the Feature

To disable sovereign cloud support for a specific installation:
1. Remove or comment out `cloudEnvironment` field in install-config.yaml
2. Installation will default to public GCP

For existing clusters:
- Cloud environment cannot be changed post-installation
- Cluster must be destroyed and recreated to change cloud environment

### Graceful Degradation

If sovereign cloud configuration fails:
- Installation fails fast during validation (before creating any infrastructure)
- Clear error messages guide users to resolution
- No partially-created infrastructure in sovereign cloud
- Users can correct configuration and retry

## Infrastructure Needed

### Testing Infrastructure

1. **Access to Germany Sovereign Cloud**:
   - Test project in Germany sovereign cloud
   - Service accounts with necessary permissions
   - Budget allocation for test clusters
   - VPN or network access to sovereign cloud (if required for private clusters)

2. **CI/CD Pipeline**:
   - If sovereign cloud accessible from CI: Add sovereign cloud test jobs
   - If not accessible: Manual testing process with periodic E2E runs
   - Dedicated testing schedule (weekly/monthly for E2E tests)

3. **RHCOS Images**:
   - Process to upload RHCOS images to sovereign cloud
   - Automation for image updates
   - Image project in sovereign cloud

### Development Infrastructure

1. **Development Access**:
   - Development team needs access to sovereign cloud for testing
   - Documentation on how to obtain sovereign cloud access
   - Development project for experimentation

2. **Documentation Repository**:
   - Space in openshift-docs for sovereign cloud installation guides
   - Troubleshooting guides
   - Known limitations documentation

3. **Partnership with Google Cloud**:
   - Technical liaison for sovereign cloud questions
   - Access to sovereign cloud documentation
   - Support for testing and validation

No new GitHub repositories or subprojects needed, but collaboration with:
- CAPG upstream project
- openshift/api repository
- cluster-image-registry-operator
- machine-api-operator

### Customer Communication

1. **Tech Preview Announcement**:
   - Clear communication about sovereign cloud support availability
   - Known limitations and requirements
   - How to provide feedback

2. **Support Documentation**:
   - Dedicated support articles for sovereign cloud
   - Escalation paths for sovereign cloud-specific issues
   - Training for support engineers on sovereign cloud differences
