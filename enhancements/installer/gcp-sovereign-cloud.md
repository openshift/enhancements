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
last-updated: 2026-07-07
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3006
  - https://redhat.atlassian.net/browse/CORS-4519
see-also:
  - "/enhancements/installer/gcp-private-clusters.md"
  - "/enhancements/installer/gcp-ipi-shared-vpc.md"
  - "/enhancements/installer/aws-eusc-partition.md"
  - "/enhancements/installer/gcp-kms-key-encryption.md"
replaces: []
superseded-by: []
---

# GCP Universe Domain Support

## Summary

This enhancement adds GCP **universe domain** support to OpenShift, enabling installation and operation on any GCP environment that uses a non-default universe domain. The primary use case is GCP sovereign cloud environments (such as Google Cloud Dedicated), but the implementation is general-purpose and works with any universe domain.

Universe domain support can be added transparently via existing GCP SDK calls. By determining the universe domain from the credentials, installing and operating OpenShift on regions with alternate universe domains can happen transparently, where the only variable is specifying the region.

## Motivation

By aligning our GCP SDK usage patterns with those recommended by GCP, we unlock the ability for users to install and operate OpenShift in sovereign cloud environments. Government agencies and regulated industries require GCP environments with data residency, regulatory compliance (such as GDPR and national security regulations), and operational sovereignty. Google Cloud Dedicated addresses these needs, but OpenShift must support non-default universe domains for it to function in those environments.

### User Stories

As a cluster administrator, I want to install OpenShift on any GCP environment (public GCP, Google Cloud Dedicated, or future isolated GCP environments) so that I can deploy to the appropriate GCP infrastructure for my organization's compliance, security, and operational requirements.

### Goals

- Support any GCP universe domain (current and future)
- All components detect universe domain from GCP credentials automatically
- Maintain backward compatibility with public GCP (default universe domain)
- Support standard installer features (IPI, private clusters, shared VPC) regardless of universe domain
- Update RHCOS stream metadata to support universe domain
- Capture universe domain in Infrastructure CR for observability (informational only)

### Non-Goals

- Automatic universe domain detection from sources other than credentials (explicit configuration in credentials required)
- Cross-universe-domain networking or federation
- Converting existing clusters to different universe domains (new installations only)
- Universe domain-specific RHCOS image builds (images use standard build process)

## Proposal

Add a new `universeDomain` field to the GCP platform configuration that specifies the GCP universe domain. Based on this field, the installer and all cluster components will:

1. Configure all GCP SDK clients with `option.WithUniverseDomain()` 
2. Validate regions and zones dynamically using GCP APIs
3. Propagate universe domain to cluster components via Infrastructure CR
4. Support any current or future GCP sovereign cloud without code changes

The universe domain is passed directly to the GCP SDK, matching how Google's own tooling (gcloud CLI) handles sovereign clouds. This approach is future-proof and requires no code changes when Google adds new sovereign cloud regions.

### Workflow Description

**Universe Domain Detection Flow:**

1. User provides GCP credentials containing `universe_domain` field:
   ```json
   {
     "type": "service_account",
     "project_id": "eu0:my-project",
     "universe_domain": "apis-berlin-build0.goog",
     ...
   }
   ```

2. Installer detects universe domain from credentials and configures all GCP SDK clients using `option.WithCredentialsJSON()`

3. Infrastructure CR is populated with detected universe domain (informational only)

4. All cluster operators detect universe domain from their mounted credentials and configure GCP SDK clients accordingly

5. Cluster operates entirely within the specified universe domain

3. Proceed with installation using internal-only endpoints

### API Extensions

This enhancement adds a new field to the Infrastructure CR for observability. No install-config changes are required - universe domain is detected from GCP credentials.

#### Infrastructure CR Changes (Upstream openshift/api)

Add `universeDomain` field to `config/v1/types_infrastructure.go`:

```go
// GCPPlatformStatus holds the current status of the Google Cloud Platform infrastructure provider.
type GCPPlatformStatus struct {
    // ... existing fields ...
    
    // UniverseDomain is the GCP universe domain detected from credentials.
    // Populated by the installer for informational/observability purposes.
    // Components should NOT read this field - they should detect universe domain
    // from their own GCP credentials.
    // 
    // When empty, standard public GCP (googleapis.com) is in use.
    // 
    // +optional
    UniverseDomain string `json:"universeDomain,omitempty"`
}
```

**Important**: This field is **informational only**. Components must detect universe domain from their credentials, not from this CR.

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
| **Installer** | openshift/installer | Detect universe domain from credentials; configure all GCP SDK clients; update RHCOS stream metadata | Medium |
| **OpenShift API** | openshift/api | Add `universeDomain` to Infrastructure CR GCPPlatformStatus | Low |
| **Cloud Controller Manager (CCM)** | kubernetes/cloud-provider-gcp | Detect universe domain from credentials; configure all SDK clients | Medium |
| **Machine API Operator (MAPI)** | openshift/machine-api-operator | Detect universe domain from credentials; pass to GCP actuator | Medium |
| **Machine API Provider GCP** | openshift/machine-api-provider-gcp | Configure GCP SDK clients with universe domain from credentials | Medium |
| **Cluster Ingress Operator** | openshift/cluster-ingress-operator | Detect universe domain from credentials for DNS operations | Low |
| **Cluster Image Registry Operator** | openshift/cluster-image-registry-operator | Detect universe domain from credentials for GCS operations | Low |
| **GCP PD CSI Driver** | kubernetes-sigs/gcp-compute-persistent-disk-csi-driver | Configure SDK with universe domain | Medium (Upstream) |

#### Detailed Component Requirements

##### 1. OpenShift Installer
**Changes**: 
- Detect universe domain from GCP credentials
- Update all GCP client creation to use `option.WithCredentialsJSON()`
- Populate Infrastructure CR with detected universe domain (informational)
- Update RHCOS stream metadata to include universe domain for image lookups

**RHCOS Stream Metadata**:
Universe domain must be included in RHCOS stream metadata so the installer can locate RHCOS images in the correct universe domain. This requires updates to the RHCOS release metadata structure.

**Example locations**:
- `pkg/asset/installconfig/gcp/client.go` - Client initialization
- `pkg/asset/cluster/gcp/gcp.go` - Cluster creation  
- `pkg/infrastructure/gcp/*` - Infrastructure provisioning
- RHCOS stream metadata (details TBD with Patrick Dillon)

##### 2. Cloud Controller Manager (CCM)
**Changes**:
- Detect universe domain from GCP credentials
- Configure all GCP SDK clients with `option.WithCredentialsJSON()`
- Used for: Load balancer creation, node management

##### 3. Machine API Operator (MAPI)
**Changes**:
- Detect universe domain from GCP credentials file
- Pass universe domain to GCP actuator for machine lifecycle operations

##### 4. Cluster Ingress Operator  
**Changes**:
- Detect universe domain from GCP credentials file
- Configure DNS API client with universe domain

##### 5. Cluster Image Registry Operator
**Changes**:
- Detect universe domain from GCP credentials file
- Configure GCS client with universe domain for bucket operations

##### 6. GCP PD CSI Driver
**Changes** (Upstream dependency):
- Detect universe domain from GCP credentials
- May need upstream CSI driver enhancement if not already supported

#### Component Discovery Pattern

All components should detect universe domain directly from GCP credentials:

```go
import (
    "encoding/json"
    "google.golang.org/api/option"
)

type serviceAccountCredentials struct {
    Type           string `json:"type"`
    ProjectID      string `json:"project_id"`
    UniverseDomain string `json:"universe_domain,omitempty"`
    // ... other fields
}

func createGCPClient(ctx context.Context, credentialsJSON []byte) (*compute.Service, error) {
    // Universe domain is automatically detected by GCP SDK from credentials
    // No need to parse and extract manually
    return compute.NewService(ctx, option.WithCredentialsJSON(credentialsJSON))
}
```

The GCP SDK automatically detects the universe domain from the credentials file and configures the client appropriately. No explicit `WithUniverseDomain()` call is needed when using `WithCredentialsJSON()` - the SDK handles this internally.

#### Testing Requirements

Each component must verify:
1. ✅ Works with default universe domain (public GCP) - existing CI
2. ✅ Works with non-default universe domain (sovereign cloud) - new test coverage
3. ✅ Handles missing/empty universe domain gracefully (defaults to public)
4. ✅ Provides clear error messages when sovereign cloud APIs are unavailable

#### Upstream Dependencies

**GCP PD CSI Driver** - Required for storage:
- Investigate current universe domain support
- File enhancement if needed: https://github.com/kubernetes-sigs/gcp-compute-persistent-disk-csi-driver

### Implementation Details/Notes/Constraints

#### GCP SDK Client Initialization

OpenShift components currently assume the default GCP universe domain (`googleapis.com`). GCP SDK clients are often initialized without using `WithCredentialsJSON()`, which means the universe domain is not detected from credentials, API calls are hardcoded to `googleapis.com` endpoints, and non-default universe domains (like Google Cloud Dedicated's `apis-berlin-build0.goog`) fail. The key implementation change is initializing all GCP SDK clients with the universe domain.

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

GCP Sovereign Cloud (specifically Google Cloud Dedicated/GCD) requires a different authentication method compared to public GCP. This is a critical implementation detail that affects all components interacting with GCP APIs.

##### Authentication Method Differences

**Public GCP:**
- Uses standard OAuth2 authentication flow
- Service account credentials exchange for OAuth2 access tokens
- Token endpoint: `https://oauth2.googleapis.com/token`
- Credentials include standard OAuth2 scopes
- GCP SDK automatically handles token refresh

**GCD (Google Cloud Dedicated):**
- Uses **self-signed JWT authentication** via `WithCredentialsJSON`
- Service account credentials include universe domain information
- No token exchange - credentials contain the JWT directly
- Universe domain is detected from the credentials file
- Authentication works entirely within the sovereign cloud boundary

##### Why Self-Signed JWT for GCD?

Self-signed JWT authentication is required for GCD because:
1. **No external token endpoint**: GCD sovereign clouds don't have access to public OAuth2 endpoints
2. **Compliance requirements**: Token exchange with external endpoints would violate data sovereignty
3. **Reduced latency**: No round-trip to token endpoint for every API call
4. **Simpler trust model**: Credentials are validated directly by sovereign cloud APIs

##### Implementation Pattern for Cluster Operators

All cluster operators that interact with GCP APIs must use the correct authentication method based on the universe domain.

**Standard Pattern** (works for both public GCP and GCD):

```go
import (
    "google.golang.org/api/compute/v1"
    "google.golang.org/api/option"
)

// createGCPClient creates a GCP SDK client - universe domain detected automatically from credentials
func createGCPClient(ctx context.Context, credentialsJSON []byte) (*compute.Service, error) {
    // SDK automatically detects universe domain from credentials and configures appropriately
    return compute.NewService(ctx, option.WithCredentialsJSON(credentialsJSON))
}
```

**Key Points:**
- ✅ **Use `option.WithCredentialsJSON(credentialsJSON)`** - SDK auto-detects universe domain from the credentials file
- ✅ **No manual universe domain extraction needed** - SDK handles this internally
- ✅ **No explicit `WithUniverseDomain()` call needed** - automatically configured from credentials
- ❌ **Do NOT manually parse credentials to extract universe domain** - SDK does this for you

**GCD Credentials Example:**
```json
{
  "type": "service_account",
  "project_id": "eu0:my-project",
  "private_key_id": "...",
  "private_key": "...",
  "client_email": "my-sa@project.eu0.iam.gserviceaccount.com",
  "client_id": "...",
  "auth_uri": "https://accounts.google.com/o/oauth2/auth",
  "token_uri": "https://oauth2.apis-berlin-build0.goog/token",
  "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
  "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/...",
  "universe_domain": "apis-berlin-build0.goog"
}
```

**Note:** The `token_uri` in GCD service account keys (e.g., `oauth2.apis-berlin-build0.goog`) is a non-existent endpoint. Any tool using `gcloud auth activate-service-account` or `option.WithCredentials()` will fail because it tries to hit this unresolvable token URI. The installer solves this by using `option.WithCredentialsJSON()`, which bypasses the token exchange and uses self-signed JWT authentication instead.

##### Component Implementation Pattern

All components detect universe domain from GCP credentials and configure SDK clients automatically.

**Standard Pattern:**
```go
// Read credentials from mounted secret
credentialsJSON, err := ioutil.ReadFile("/etc/cloud-credentials/credentials.json")

// SDK automatically detects universe domain and configures authentication
computeClient, err := compute.NewService(ctx, option.WithCredentialsJSON(credentialsJSON))
```

The SDK reads `universe_domain` from credentials and automatically:
- Configures the correct API endpoints
- Uses self-signed JWT authentication for non-default universe domains
- Uses standard OAuth2 for public GCP (`googleapis.com`)

This pattern applies to all GCP SDK clients: `compute.NewService()`, `storage.NewClient()`, `dns.NewService()`, `iam.NewService()`, `cloudresourcemanager.NewService()`

**Verified Components** (CORS-4519):
- cloud-provider-gcp
- cluster-ingress-operator
- machine-api-operator
- machine-api-provider-gcp
- cluster-image-registry-operator
- image-registry
- gcp-pd-csi-driver
- cloud-credential-operator
- cluster-cloud-controller-manager-operator
- cloud-network-config-controller

##### Cloud Provider Configuration

**Location**: `pkg/asset/manifests/gcp/cloudproviderconfig.go`

The in-cluster cloud provider config needs to include universe domain so that cluster operators can authenticate to the correct GCP environment.

```go
func GenerateCloudProviderConfig(platform *gcp.Platform) string {
    config := &cloudProviderConfig{
        ProjectID:      platform.ProjectID,
        NetworkName:    platform.Network,
        SubnetworkName: platform.ComputeSubnet,
        UniverseDomain: platform.UniverseDomain,
    }
    return marshalConfig(config)
}
```

This configuration is consumed by in-cluster operators (Machine API, Cloud Controller Manager, Cloud Credential Operator) who combine it with their mounted credentials for authentication.

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
    region: u-germany-northeast1
    defaultMachinePlatform:
      osImage:
        project: eu0:rhcos-images-germany  # Sovereign cloud RHCOS image project
        name: rhcos-48-x86-64
```

**Documentation Note**: Must clearly document that sovereign cloud installations require specifying custom RHCOS image projects.

### Risks and Mitigations

#### Risk: Service Availability Differences

**Risk**: Sovereign clouds may not have all services available in public GCP, causing installation failures.

**Mitigation**: Implement service availability validation specific to each sovereign cloud. Document required vs. optional services. Provide clear error messages when required services are unavailable.

#### Risk: Divergent API Behavior

**Risk**: Sovereign cloud APIs may have subtle differences in behavior compared to public GCP APIs.

**Mitigation**: Extensive testing in actual sovereign cloud environments. Partner with Google Cloud for API compatibility validation. Document any known API differences.

#### Risk: Universe Domain Immutability

**Risk**: Clusters cannot migrate between universe domains (e.g., public to sovereign) post-installation.

**Mitigation**: Universe domain is immutable in Infrastructure CR. Clearly document that it cannot be changed after installation.

### Drawbacks

- Supporting multiple universe domains adds complexity to codebase, validation, and testing
- Sovereign cloud environments may have limited availability for CI/CD testing
- Sovereign clouds may lag behind public GCP in feature availability

## Alternatives (Not Implemented)

### Alternative 1: Separate Platform Type

Create a new platform type (e.g., `platform.gcpSovereign`) instead of detecting universe domain from credentials.

**Rejected because**: Massive code duplication. AWS partition support pattern (single platform, environment detection) is more maintainable and scales better.

### Alternative 2: Manual Universe Domain Configuration

Add `universeDomain` field to install-config instead of detecting from credentials.

**Rejected because**: Credentials already contain universe domain. Requiring users to specify it twice is redundant and error-prone. Credentials-based detection follows GCP SDK best practices.

## Open Questions

1. **RHCOS Stream Metadata Structure**
   - Exact structure for including universe domain in RHCOS stream metadata (pending discussion with Patrick Dillon)

## GCD-Specific Constraints and Requirements

Google Cloud Dedicated (GCD) has specific constraints that differ from public GCP. These constraints from Google's GCD TPC (Trusted Partner Cloud) documentation affect OpenShift installation and operations:

### Key GCD Constraints

**Machine Types:**
- GCD only supports C3, M3, and A3 Edge series machine types. Common public GCP types such as N2, E2, and T2A are not available.
- The installer's default machine types (`e2-custom-6-16384` for workers, `e2-standard` for masters) will not work in GCD. Defaults must be overridden to use supported machine types (e.g., `c3-standard-4`).

**Load Balancing:**
- Only regional load balancers supported (no global load balancers of any type)
- All networking resources must use Premium Tier (Standard Tier not available)
- Regional resources only: IP addresses, backend services, forwarding rules, target proxies

**SSL/TLS Certificates:**
- Self-managed regional SSL certificates only
- Certificate Manager not available
- Google-managed certificates not available
- Applications must manage their own certificates

**DNS:**
- Private DNS zones only (public DNS zones not supported)
- External DNS provider required for public-facing DNS resolution
- All private DNS zone types supported (forwarding, peering, reverse lookup, Service Directory)

**Networking:**
- Single region deployment with multiple availability zones
  - No multi-region HA is possible; all cluster resources must reside in a single region
  - Boskos lease naming must account for the single-region constraint when managing CI test clusters
- No default VPC network created automatically
- Partner Interconnect and Cross-Cloud Interconnect not available
- C3 machines require GVNIC guest OS feature enabled on images. Any custom image must be created with the option `guest-os-features=GVNIC`

**Missing Cloud Services:**
- Serverless: Cloud Run, Cloud Functions, App Engine
- CDN: Cloud CDN, Media CDN
- Security: Certificate Manager, Service Extensions, mTLS policies
- Observability: Network Intelligence Center, Cloud Service Mesh

**Disk Types:**
- Hyperdisk Balanced is the only disk type currently supported by both OpenShift and GCD. Common public GCP disk types such as `pd-ssd` and `pd-standard` are not available.
- This is relevant to the installer's default machine configuration and storage provisioning, which must be updated to use Hyperdisk Balanced.

**Authentication:**
- Self-signed JWT authentication via `WithCredentialsJSON` (not standard OAuth2)
- Universe domain detected from credentials file
- All cluster operators must configure GCP SDK clients with universe domain

**Impact on OpenShift:** Load balancers created by installer and cluster operators must use regional resources. External DNS provider required for public ingress. Self-managed certificates required for ingress TLS.

See [Google Cloud Dedicated documentation](https://cloud.google.com/docs/security/gcd) for complete constraints.

### Component Implementation Requirements

**Installer:**
- Configure service endpoints for `apis-berlin-build0.goog` domain when universe domain is set
- Validate regions against GCD-available regions
- Configure all GCP SDK clients with `WithUniverseDomain`

**All Cluster Operators:**
- Read GCP credentials from mounted secret (`/etc/cloud-credentials/credentials.json`)
- Use `option.WithCredentialsJSON(credentialsJSON)` when creating GCP SDK clients
- GCP SDK automatically detects universe domain from credentials and configures:
  - Correct API endpoints (e.g., `apis-berlin-build0.goog` vs `googleapis.com`)
  - Appropriate authentication method (self-signed JWT for GCD, OAuth2 for public GCP)

**No Infrastructure CR coordination needed** - each component independently detects universe domain from its credentials.

## Test Plan

**Primary Test**: Install a private cluster on Google Cloud Dedicated (GCD) using the authentication methods detailed in this enhancement.

**Success Criteria**:
- Installer detects universe domain from GCD credentials
- All components authenticate using self-signed JWT via `WithCredentialsJSON`
- Cluster installation completes successfully
- In-cluster operators function correctly with GCD APIs
- Cluster can be upgraded

**Note**: Testing requires access to a GCD environment.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Successful installation on Google Cloud Dedicated (GCD) using universe domain detection from credentials
- All components authenticate using self-signed JWT via `WithCredentialsJSON` as documented in this enhancement
- End user documentation for universe domain support
- Sufficient unit and integration test coverage
- At least one E2E test run on actual GCD environment
- Known limitations clearly documented
- CSI driver compatibility validated

### Tech Preview -> GA

- Multiple successful customer installations on GCD using universe domain authentication methods
- Sufficient time for feedback (minimum 2 releases in Tech Preview)
- Support for key features validated on GCD:
  - Private clusters
  - Standard machine scaling
  - Cluster upgrades
- Comprehensive testing coverage:
  - Regular E2E testing on GCD
  - Upgrade testing
  - Scale testing
- User-facing documentation in openshift-docs
- Support procedures and runbooks created
- No known critical bugs specific to universe domain support

### Removing a deprecated feature

This enhancement does not deprecate or remove any existing features. Universe domain detection is automatic from credentials, maintaining full backward compatibility.

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

3. **CSI Driver Version**: Must ensure CSI driver version supports universe domain.

4. **Component Updates**: If in-cluster component is updated independently, it reads cloud environment from Infrastructure CR, ensuring compatibility.

5. **Control Plane vs Workers**: No version skew issues as all nodes use the same cloud environment.

## Operational Aspects of API Extensions

This enhancement adds a field to the Infrastructure CR spec and status but does not add webhooks, finalizers, or other API extensions.

### Infrastructure CR Impact

**Field Addition**: 
- `status.platformStatus.gcp.universeDomain` (optional, informational only)

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

Common failure modes:
- **Authentication failures**: Verify credentials contain correct `universe_domain` field and use self-signed JWT authentication as documented in this enhancement
- **API connection errors**: Verify network access to sovereign cloud API endpoints specified in credentials
- **Service unavailability**: Check service availability in sovereign cloud documentation

Universe domain is immutable post-installation. Clusters cannot be migrated between universe domains.

Detailed troubleshooting procedures will be documented in support runbooks.

## Infrastructure Needed

### Testing Infrastructure

1. **Access to Germany Sovereign Cloud**:
   - Test project in Germany sovereign cloud
   - Service accounts with necessary permissions
   - Budget allocation for test clusters
   - VPN or network access to sovereign cloud (if required for private clusters)

2. **CI/CD Pipeline**:
   - A Bastion Host is currently required for CI access to the sovereign cloud environment
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
- GCP PD CSI driver upstream project
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
