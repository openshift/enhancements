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

This enhancement enables OpenShift to be installed on GCP Sovereign Cloud environments (such as Google Cloud Germany and other sovereign cloud offerings). Sovereign clouds are isolated GCP environments designed for government and regulated industries with specific data residency, security, and compliance requirements. Unlike standard public GCP, sovereign clouds use:
- Different API endpoint domains (e.g., `cloud.google.de` instead of `googleapis.com`)
- Limited regional availability
- Specialized project naming conventions (e.g., `eu0:<project-name>`)
- Potentially different service availability and naming

Currently, the OpenShift installer assumes standard public GCP infrastructure with hardcoded endpoint domains and service names, making it incompatible with sovereign cloud environments.

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

#### Story 1: Government Agency Deployment

As a cloud architect for a European government agency, I need to deploy OpenShift on Google Cloud Germany (a sovereign cloud) to meet GDPR and national data residency requirements, so that I can run containerized government services while maintaining compliance with data sovereignty regulations.

#### Story 2: Regulated Financial Institution

As a platform engineer at a financial institution in a country with strict data localization laws, I need to install OpenShift on a GCP sovereign cloud that guarantees data never leaves the country's borders, so that I can modernize our application infrastructure without violating banking regulations.

#### Story 3: Defense Contractor

As a DevOps engineer for a defense contractor, I need to deploy OpenShift on a government-certified sovereign cloud environment with enhanced security controls and operational sovereignty, so that I can run classified workloads on cloud infrastructure approved for sensitive government work.

#### Story 4: Healthcare Provider

As a system administrator for a healthcare provider, I need OpenShift running on a sovereign cloud that meets healthcare data protection regulations and provides data residency guarantees, so that I can deploy patient data processing applications in a compliant cloud environment.

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

Add a new `cloudEnvironment` field to the GCP platform configuration that specifies the target cloud environment. Based on this field, the installer will:

1. Use appropriate API endpoint domains for the sovereign cloud
2. Validate regions and zones against sovereign cloud-specific availability
3. Configure CAPI providers with sovereign cloud endpoints
4. Adjust service validation to match sovereign cloud service availability
5. Handle sovereign cloud-specific project naming conventions

The implementation will use a cloud environment enumeration with extensible architecture to support additional sovereign clouds in the future.

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

3. The cluster administrator creates an install-config.yaml specifying the sovereign cloud environment:
   ```yaml
   apiVersion: v1
   baseDomain: example.de
   metadata:
     name: my-cluster
   platform:
     gcp:
       projectID: eu0:my-project
       region: europe-west3  # Valid Germany sovereign cloud region
       cloudEnvironment: germany-sovereign
   pullSecret: '{"auths": ...}'
   ```

4. The installer validates the install-config:
   - Recognizes `cloudEnvironment: germany-sovereign`
   - Validates project ID format matches sovereign cloud pattern (`eu0:*`)
   - Validates region is available in Germany sovereign cloud
   - Configures API endpoints for `*.cloud.google.de` domain

5. The installer generates manifests with sovereign cloud-specific configuration:
   - CAPI GCP cluster manifest includes endpoint overrides
   - Infrastructure CR includes cloud environment specification
   - Machine manifests reference correct API endpoints

6. The installer provisions infrastructure using sovereign cloud APIs:
   - Creates VPC, subnets, firewall rules via sovereign cloud Compute API
   - Creates service accounts and IAM bindings via sovereign cloud IAM API
   - Creates DNS records via sovereign cloud DNS API
   - Creates GCS bucket for ignition via sovereign cloud Storage API

7. Bootstrap process completes using sovereign cloud resources:
   - Bootstrap VM launches in sovereign cloud
   - Control plane nodes launch and form cluster
   - Cloud controller manager configured with sovereign cloud endpoints

8. Cluster operators initialize with sovereign cloud awareness:
   - Machine API operator uses sovereign cloud endpoints
   - Cluster image registry operator creates GCS bucket in sovereign cloud
   - Ingress operator creates load balancers in sovereign cloud

9. The cluster is fully operational on the sovereign cloud environment

#### Variation: Private Cluster Installation

For customers requiring private, air-gapped installations:

1. Create install-config with both sovereign cloud and private cluster settings:
   ```yaml
   platform:
     gcp:
       cloudEnvironment: germany-sovereign
       projectID: eu0:my-project
       region: europe-west3
   publish: Internal
   ```

2. Configure private service connect or VPN access to sovereign cloud APIs

3. Proceed with installation using internal-only endpoints

#### Error Handling

**Invalid Cloud Environment Specified**

1. User specifies an unsupported cloud environment in install-config
2. Installer validation fails with clear error:
   ```
   platform.gcp.cloudEnvironment: Invalid value: "invalid-cloud": supported values are "", "germany-sovereign"
   ```
3. User corrects the configuration and retries

**Region Not Available in Sovereign Cloud**

1. User specifies a public GCP region for a sovereign cloud installation
2. Validation fails with error:
   ```
   platform.gcp.region: Invalid value: "us-central1": region is not available in germany-sovereign cloud environment. Available regions: europe-west3, europe-west4
   ```
3. User selects a valid sovereign cloud region

**Project ID Format Mismatch**

1. User specifies standard project ID for sovereign cloud
2. Validation warns:
   ```
   Warning: platform.gcp.projectID: "my-project" does not match expected sovereign cloud format "eu0:<name>". Verify this is correct.
   ```
3. User confirms or corrects project ID

**Service Not Available in Sovereign Cloud**

1. Installer attempts to enable a service not available in sovereign cloud
2. Validation fails with error:
   ```
   platform.gcp: Service "serviceusage.googleapis.com" is not available in germany-sovereign cloud environment
   ```
3. Installer skips optional service or fails if service is required

### API Extensions

This enhancement modifies the install-config API and adds a new field to the Infrastructure CR.

#### Install Config Changes

Add to `pkg/types/gcp/platform.go`:

```go
// CloudEnvironment specifies the GCP cloud environment type
type CloudEnvironment string

const (
    // PublicCloud is the standard public GCP cloud (default)
    PublicCloud CloudEnvironment = ""
    
    // GermanySovereignCloud is the Google Cloud Germany sovereign cloud
    GermanySovereignCloud CloudEnvironment = "germany-sovereign"
    
    // Additional sovereign clouds can be added here as they become available
)

// Platform stores all the global configuration that all machinesets use.
type Platform struct {
    // ... existing fields ...
    
    // CloudEnvironment specifies the target GCP cloud environment.
    // When empty or "public", uses standard public GCP (googleapis.com domain).
    // When set to a sovereign cloud value (e.g., "germany-sovereign"), uses
    // sovereign cloud-specific API endpoints and configurations.
    // +optional
    CloudEnvironment CloudEnvironment `json:"cloudEnvironment,omitempty"`
}

// GetBaseDomain returns the base API domain for the cloud environment
func (p *Platform) GetBaseDomain() string {
    switch p.CloudEnvironment {
    case GermanySovereignCloud:
        return "cloud.google.de"
    case PublicCloud, "":
        return "googleapis.com"
    default:
        // Unknown environment, use public cloud default
        return "googleapis.com"
    }
}

// GetProjectIDFormat returns the expected project ID format for validation
func (p *Platform) GetProjectIDFormat() string {
    switch p.CloudEnvironment {
    case GermanySovereignCloud:
        return "eu0:<project-name>"
    case PublicCloud, "":
        return "<project-id>"
    default:
        return "<project-id>"
    }
}
```

#### Infrastructure CR Changes (Upstream openshift/api)

Add to `config/v1/types_infrastructure.go`:

```go
// GCPPlatformSpec holds the desired state of the Google Cloud Platform infrastructure provider.
// This only includes fields that can be modified in the cluster.
type GCPPlatformSpec struct {
    // ... existing fields ...
    
    // CloudEnvironment specifies the GCP cloud environment type.
    // When empty, assumes standard public GCP.
    // +optional
    CloudEnvironment GCPCloudEnvironment `json:"cloudEnvironment,omitempty"`
}

// GCPPlatformStatus holds the current status of the Google Cloud Platform infrastructure provider.
type GCPPlatformStatus struct {
    // ... existing fields ...
    
    // CloudEnvironment specifies the GCP cloud environment in use.
    // +optional
    CloudEnvironment GCPCloudEnvironment `json:"cloudEnvironment,omitempty"`
}

// GCPCloudEnvironment is the type for GCP cloud environment
type GCPCloudEnvironment string

const (
    // GCPPublicCloud is standard public GCP
    GCPPublicCloud GCPCloudEnvironment = ""
    
    // GCPGermanySovereignCloud is Google Cloud Germany
    GCPGermanySovereignCloud GCPCloudEnvironment = "germany-sovereign"
)
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

### Implementation Details/Notes/Constraints

#### Service Endpoint Management

**Current Implementation** (`pkg/asset/installconfig/gcp/services.go`):
```go
func CreateServiceEndpoint(endpointName string, service ServiceNameGCP) string {
    baseEndpoint := fmt.Sprintf("https://%s-%s.p.googleapis.com/", string(service), endpointName)
    return baseEndpoint
}
```

**New Implementation**:
```go
func CreateServiceEndpoint(endpointName string, service ServiceNameGCP, cloudEnv CloudEnvironment) string {
    switch cloudEnv {
    case GermanySovereignCloud:
        // Germany sovereign cloud uses different endpoint pattern
        // Format: https://{service}.{region}.cloud.google.de/
        return fmt.Sprintf("https://%s.%s.cloud.google.de/", string(service), endpointName)
    case PublicCloud, "":
        // Standard public GCP
        return fmt.Sprintf("https://%s-%s.p.googleapis.com/", string(service), endpointName)
    default:
        // Unknown environment, use public cloud default
        return fmt.Sprintf("https://%s-%s.p.googleapis.com/", string(service), endpointName)
    }
}
```

**Note**: The exact endpoint format for Germany sovereign cloud may differ. This needs verification with Google Cloud Germany documentation.

#### Service Validation

**Current Implementation** (`pkg/asset/installconfig/gcp/validation.go`):
```go
requiredServices := sets.NewString(
    "compute.googleapis.com",
    "cloudresourcemanager.googleapis.com",
    "dns.googleapis.com",
    "iam.googleapis.com",
    "iamcredentials.googleapis.com",
    "serviceusage.googleapis.com"
)
```

**New Implementation**:
```go
func getRequiredServices(cloudEnv CloudEnvironment) sets.String {
    switch cloudEnv {
    case GermanySovereignCloud:
        // Germany sovereign cloud may have different service names
        // or may not have all services (e.g., serviceusage.googleapis.com may not exist)
        return sets.NewString(
            "compute.cloud.google.de",
            "cloudresourcemanager.cloud.google.de",
            "dns.cloud.google.de",
            "iam.cloud.google.de",
            "iamcredentials.cloud.google.de",
            // serviceusage may not be available in sovereign cloud
        )
    case PublicCloud, "":
        // Standard public GCP services
        return sets.NewString(
            "compute.googleapis.com",
            "cloudresourcemanager.googleapis.com",
            "dns.googleapis.com",
            "iam.googleapis.com",
            "iamcredentials.googleapis.com",
            "serviceusage.googleapis.com",
        )
    default:
        // Unknown, use public cloud
        return sets.NewString(
            "compute.googleapis.com",
            "cloudresourcemanager.googleapis.com",
            "dns.googleapis.com",
            "iam.googleapis.com",
            "iamcredentials.googleapis.com",
            "serviceusage.googleapis.com",
        )
    }
}
```

**Important**: Service availability and naming in sovereign clouds needs to be confirmed with Google Cloud documentation for each sovereign cloud offering.

#### Region and Zone Validation

**Current Implementation** (`pkg/types/gcp/validation/platform.go`):
- Hardcoded map of 42 public GCP regions
- Validates against this static list

**New Implementation**:
```go
// Cloud environment-specific region lists
var sovereignCloudRegions = map[CloudEnvironment][]string{
    GermanySovereignCloud: {
        "europe-west3",  // Frankfurt
        "europe-west4",  // Netherlands (if available)
        // Add other Germany sovereign cloud regions
    },
    // Additional sovereign clouds can be added here
}

func getValidRegions(cloudEnv CloudEnvironment) []string {
    if regions, ok := sovereignCloudRegions[cloudEnv]; ok {
        return regions
    }
    // Fall back to public GCP regions
    return publicGCPRegions
}

// Validation should also call GCP API to get actual available regions
// rather than relying solely on hardcoded lists
func validateRegion(region string, cloudEnv CloudEnvironment, client *Client) error {
    // Try to fetch regions from API
    apiRegions, err := client.GetRegions(ctx, projectID)
    if err == nil {
        // Use API-provided region list
        if !contains(apiRegions, region) {
            return fmt.Errorf("region %s not available in cloud environment %s", region, cloudEnv)
        }
        return nil
    }
    
    // Fall back to hardcoded list if API call fails
    validRegions := getValidRegions(cloudEnv)
    if !contains(validRegions, region) {
        return fmt.Errorf("region %s not in expected region list for cloud environment %s: %v", 
            region, cloudEnv, validRegions)
    }
    return nil
}
```

#### CAPI Provider Endpoint Configuration

The Cluster API Provider GCP (CAPG) needs to be configured with sovereign cloud endpoints.

**Location**: `cluster-api/providers/gcp/vendor/sigs.k8s.io/cluster-api-provider-gcp/cloud/scope/clients.go`

CAPG already supports endpoint overrides via:
- `ComputeServiceEndpoint` (line 120)
- `ContainerServiceEndpoint` (line 139)
- `IAMServiceEndpoint` (line 156)
- `ResourceManagerServiceEndpoint` (line 189)

**Implementation in Installer** (`pkg/infrastructure/gcp/clusterapi/clusterapi.go`):
```go
func (p *Provider) generateGCPCluster(cloudEnv CloudEnvironment) (*gcpv1.GCPCluster, error) {
    cluster := &gcpv1.GCPCluster{
        // ... standard cluster config ...
    }
    
    // Configure endpoints for sovereign cloud
    if cloudEnv == GermanySovereignCloud {
        cluster.Spec.AdditionalLabels["cloud-environment"] = "germany-sovereign"
        
        // Set endpoint overrides (requires CAPG API support)
        // This may need to be configured via annotations or a custom resource
        // if CAPG doesn't have direct API fields for this
    }
    
    return cluster, nil
}
```

**Note**: This may require upstream CAPG enhancements if endpoint configuration is not fully supported in CAPG API.

#### Project ID Format Validation

Sovereign clouds use different project ID formats:
- Public GCP: `my-project-123`
- Germany Sovereign Cloud: `eu0:my-project-123`

**Implementation**:
```go
func validateProjectID(projectID string, cloudEnv CloudEnvironment) error {
    switch cloudEnv {
    case GermanySovereignCloud:
        if !strings.HasPrefix(projectID, "eu0:") {
            return fmt.Errorf("Germany sovereign cloud project IDs must start with 'eu0:', got: %s", projectID)
        }
    case PublicCloud, "":
        if strings.Contains(projectID, ":") {
            return fmt.Errorf("public GCP project IDs should not contain ':', got: %s. Did you mean to set cloudEnvironment?", projectID)
        }
    }
    return nil
}
```

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
