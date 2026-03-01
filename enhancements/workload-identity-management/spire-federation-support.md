---
title: spire-federation-support
authors:
  - "@rausingh-rh"
reviewers:
  - "@tgeer"
approvers:
  - "@tgeer"
api-approvers:
  - "@tgeer"
creation-date: 2025-10-16
last-updated: 2025-10-16
tracking-link:
  - https://issues.redhat.com/browse/SPIRE-211
---

# SPIRE Federation Support for Zero Trust Workload Identity Manager

## Summary

This enhancement adds native support for SPIRE Federation in the Zero Trust Workload Identity Manager operator, enabling secure cross-cluster workload communication. The operator will manage configuration to add federation support, managing lifecycle of trust bundle endpoint, and support federation across N clusters (where N is limited to a configurable maximum).

## Motivation

Organizations deploying workloads across multiple OpenShift clusters need secure service-to-service communication. SPIRE Federation enables workloads in one cluster to authenticate and communicate with workloads in another cluster using cryptographically verified identities (SVIDs). By adding federation support to the operator, we enable declarative, automated, and auditable federation management.

### User Stories

* As an OpenShift cluster administrator, I want to enable SPIRE federation by setting `spec.federation.bundleEndpoint` in the SpireServer CR, so that the operator automatically creates a Service (port 8443) and Route exposing the federation endpoint.

* As an OpenShift cluster administrator, I want to specify federated trust domains in `spec.federation.federatesWith[]` with `bundleEndpointUrl` and `endpointSpiffeId`, so that the operator generates the SPIRE `server.conf` with `federation.federates_with` configuration and triggers StatefulSet rolling updates.

* As an OpenShift security engineer, I want to choose between `https_spiffe` (default, SPIFFE authentication) and `https_web` (Web PKI) profiles for `spec.federation.bundleEndpoint.profile`, so that I can align federation security with organizational certificate policies and authentication requirements.

* As an SRE, I want SpireServer status conditions `ConfigurationValid` and `RouteAvailable` with timestamps and error messages, so that I can quickly diagnose federation or route configuration issues.

* As an OpenShift cluster administrator, I want the operator to handle federation configuration removal gracefully when `spec.federation` is removed, so that cross-cluster communication stops cleanly while intra-cluster workloads continue functioning without manual resource cleanup.

* As an application developer, I want to create ClusterSPIFFEID resources with `spec.federatesWith[]` after federation is configured, so that my workloads automatically receive SVIDs capable of authenticating against federated trust domains without understanding the underlying SPIRE federation mechanics.

* As an OpenShift security engineer, I want clear documentation that initial trust bundle bootstrapping requires manual efforts, so that I understand the security model and the one-time manual step required to establish federation trust.

### Goals

1. Enable declarative federation configuration through SpireServer CR API
2. Automate federation endpoint exposure (Service, Route creation)
3. Support both `https_spiffe` and `https_web` bundle endpoint profiles
4. Support federation across N clusters (configurable limit)
5. Provide clear validation and error messages for federation misconfiguration
6. Document the manual trust bundle bootstrapping process
7. Ensure federation configuration is included in StatefulSet pod spec for proper restarts on configuration changes

### Non-Goals

1. **Automatic trust bundle bootstrapping** - Initial trust bundle exchange cannot be fully automated without compromising security. Users must manually bootstrap trust bundles.
2. **Federation with unlimited clusters** - We impose a configurable limit to prevent performance degradation.
3. **Automatic ClusterFederatedTrustDomain creation** - Users must create `ClusterFederatedTrustDomain` resources on each cluster for each federation relationship to enable automatic bundle rotation and initial trust bundle exchange.
4. **Custom CA certificate management for https_web profile** - For `https_web` profile, users must provide valid certificates via Secrets or use ACME.
5. **Dynamic trust domain changes** - Changing trust domains in federated clusters requires recreating the federation from scratch. This is a SPIRE limitation, not an operator limitation. This is not supported as part of this enhancement proposal.

## Proposal

This proposal adds federation configuration to the `SpireServerSpec` API and extends the spire-server controller to:

1. Generate federation configuration in spire-server ConfigMap
2. Create/update a Service to expose port 8443 for federation endpoint
3. Create/update an OpenShift Route to expose the federation endpoint externally
4. Update the StatefulSet to expose port 8443
5. Validate federation configuration at admission time

The operator will NOT automate trust bundle bootstrapping. Users must still perform initial trust bundle exchange.

### Workflow Description

#### Actors
- **Platform Administrator**: Responsible for configuring federation across clusters
- **Zero Trust Workload Identity Manager Operator**: Manages SPIRE infrastructure
- **SPIRE Server**: Provides federation endpoints and manages trust bundles
- **SPIRE Controller Manager**: Reconciles ClusterFederatedTrustDomain resources
- **Application Developer**: Deploys workloads that use federation

#### Initial Federation Setup (Two Clusters)

```mermaid
sequenceDiagram
    participant Admin as Platform Administrator
    participant CRA as SpireServer CR (Cluster A)
    participant OpA as Operator (Cluster A)
    participant SSA as SPIRE Server (Cluster A)
    participant CRB as SpireServer CR (Cluster B)
    participant OpB as Operator (Cluster B)
    participant SSB as SPIRE Server (Cluster B)
    participant SCMA as SPIRE Controller Manager (Cluster A)
    participant SCMB as SPIRE Controller Manager (Cluster B)

    Note over Admin,SCMB: Phase 1: Configure Federation Endpoints

    Admin->>CRA: Update with federation config<br/>(bundleEndpoint + federatesWith)
    CRA->>OpA: Reconcile triggered
    OpA->>OpA: Validate federation config
    OpA->>OpA: Generate ConfigMap with<br/>federation configuration
    OpA->>OpA: Create/Update Service<br/>(port 8443)
    OpA->>OpA: Create/Update Route<br/>(passthrough/reencrypt TLS)
    OpA->>OpA: Update StatefulSet<br/>(expose port 8443)
    OpA->>SSA: Rolling restart with<br/>new configuration
    SSA-->>OpA: Federation endpoint active

    Admin->>CRB: Update with federation config<br/>(bundleEndpoint + federatesWith)
    CRB->>OpB: Reconcile triggered
    OpB->>OpB: Validate federation config
    OpB->>OpB: Generate ConfigMap with<br/>federation configuration
    OpB->>OpB: Create/Update Service<br/>(port 8443)
    OpB->>OpB: Create/Update Route<br/>(passthrough/reencrypt TLS)
    OpB->>OpB: Update StatefulSet<br/>(expose port 8443)
    OpB->>SSB: Rolling restart with<br/>new configuration
    SSB-->>OpB: Federation endpoint active

    Note over Admin,SCMB: Phase 2: Manual Trust Bundle Bootstrapping and Automatic Bundle Rotation

    Admin->>SSB: Extract trust bundle<br/>(federation endpoint response)
    Admin->>SSA: Extract trust bundle<br/>(federation endpoint response)
    
    Admin->>SCMA: Create ClusterFederatedTrustDomain with cluster B's trust bundle in trustDomainBundle field<br/>(for Cluster B)
    SCMA->>SSA: Register federation relationship
    SSA->>SSB: Fetch bundle via federation endpoint
    SSB-->>SSA: Return current bundle
    SCMA->>SCMA: Schedule next refresh<br/>(every 5 minutes - refresh_hint)

    Admin->>SCMB: Create ClusterFederatedTrustDomain with cluster A's trust bundle in trustDomainBundle field<br/>(for Cluster A)
    SCMB->>SSB: Register federation relationship
    SSB->>SSA: Fetch bundle via federation endpoint
    SSA-->>SSB: Return current bundle
    SCMB->>SCMB: Schedule next refresh<br/>(every 5 minutes - refresh_hint)

    Note over Admin,SCMB: Phase 3: Deploy Federated Workloads

    Admin->>SCMA: Create ClusterSPIFFEID<br/>(with federatesWith)
    SCMA->>SSA: Register SPIFFE ID with<br/>federation capability
    Admin->>SCMB: Create ClusterSPIFFEID<br/>(with federatesWith)
    SCMB->>SSB: Register SPIFFE ID with<br/>federation capability
    Admin->>SCMB: Deploy workload
    
    Note over SSA,SSB: Ongoing: Automatic Bundle Rotation (every 5 min)
    
    loop Every 5 minutes
        SSA->>SSB: Fetch updated bundle
        SSB-->>SSA: Return bundle
        SSB->>SSA: Fetch updated bundle
        SSA-->>SSB: Return bundle
    end
```

1. **Configure Federation**: Admin updates SpireServer CR on both clusters with `federation.bundleEndpoint` and `federation.federatesWith` configuration
    ```yaml
   apiVersion: operator.openshift.io/v1alpha1
   kind: SpireServer
   metadata:
     name: cluster
   spec:
     trustDomain: apps.cluster-a.example.com
     clusterName: cluster-a
     # ... existing fields ...
     federation:
       bundleEndpoint:
         profile: https_spiffe
       federatesWith:
       - trustDomain: apps.cluster-b.example.com
         bundleEndpointUrl: https://spire-server-federation-zero-trust-workload-identity-manager.apps.cluster-b.example.com
         bundleEndpointProfile: https_spiffe
         endpointSpiffeId: spiffe://apps.cluster-b.example.com/spire/server
   ```

2. **Operator Reconciliation**: 
   - Operator validates federation configuration
   - Updates ConfigMap with federation settings
   - Creates/updates Service exposing federation endpoint port (8443)
   - Creates/updates Route exposing federation endpoint externally
   - Updates StatefulSet to expose federation endpoint port
   - Triggers pod restart with new configuration 

### API Extensions

This enhancement adds new fields to the existing `SpireServerSpec` API:

```go
type SpireServerSpec struct {
  // ... existing fields ...

  // Federation configures SPIRE federation endpoints and relationships
  // +kubebuilder:validation:Optional
  Federation *FederationConfig `json:"federation,omitempty"`
}

// FederationConfig defines federation bundle endpoint and federated trust domains
type FederationConfig struct {
	// bundleEndpoint configures this cluster's federation bundle endpoint
	// +kubebuilder:validation:Required
	BundleEndpoint BundleEndpointConfig `json:"bundleEndpoint"`

	// federatesWith lists trust domains this cluster federates with
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=50
	FederatesWith []FederatesWithConfig `json:"federatesWith,omitempty"`

	// managedRoute enables or disables automatic Route creation for the federation endpoint
	// "true": Allows automatic exposure of federation endpoint through a managed OpenShift Route.
	// "false": Allows administrators to manually configure exposure using custom OpenShift Routes or ingress, offering more control over routing behavior.
	// +kubebuilder:default:="true"
	// +kubebuilder:validation:Enum:="true";"false"
	// +kubebuilder:validation:Optional
	ManagedRoute string `json:"managedRoute,omitempty"`
}

// BundleEndpointConfig configures how this cluster exposes its federation bundle
// The federation endpoint is exposed on 0.0.0.0:8443
// +kubebuilder:validation:XValidation:rule="self.profile == 'https_web' ? has(self.httpsWeb) : true",message="httpsWeb is required when profile is https_web"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.profile) || oldSelf.profile == self.profile",message="profile is immutable and cannot be changed once set"
type BundleEndpointConfig struct {
	// profile is the bundle endpoint authentication profile
	// +kubebuilder:validation:Enum=https_spiffe;https_web
	// +kubebuilder:default=https_spiffe
	Profile BundleEndpointProfile `json:"profile"`

	// refreshHint is the hint for bundle refresh interval in seconds
	// +kubebuilder:validation:Minimum=60
	// +kubebuilder:validation:Maximum=3600
	// +kubebuilder:default=300
	RefreshHint int32 `json:"refreshHint,omitempty"`

	// httpsWeb configures the https_web profile (required if profile is https_web)
	// +kubebuilder:validation:Optional
	HttpsWeb *HttpsWebConfig `json:"httpsWeb,omitempty"`
}

// BundleEndpointProfile represents the authentication profile for bundle endpoint
// +kubebuilder:validation:Enum=https_spiffe;https_web
type BundleEndpointProfile string

const (
	// HttpsSpiffeProfile uses SPIFFE authentication (default)
	HttpsSpiffeProfile BundleEndpointProfile = "https_spiffe"

	// HttpsWebProfile uses Web PKI (X.509 certificates from public CA)
	HttpsWebProfile BundleEndpointProfile = "https_web"
)

// HttpsWebConfig configures https_web profile authentication
// +kubebuilder:validation:XValidation:rule="(has(self.acme) && !has(self.servingCert)) || (!has(self.acme) && has(self.servingCert))",message="exactly one of acme or servingCert must be set"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.acme) || has(self.acme)",message="cannot switch from acme to servingCert configuration"
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.servingCert) || has(self.servingCert)",message="cannot switch from servingCert to acme configuration"
type HttpsWebConfig struct {
	// acme configures automatic certificate management using ACME protocol
	// Mutually exclusive with servingCert
	// +kubebuilder:validation:Optional
	Acme *AcmeConfig `json:"acme,omitempty"`

	// servingCert configures certificate from a Kubernetes Secret
	// Mutually exclusive with acme
	// +kubebuilder:validation:Optional
	ServingCert *ServingCertConfig `json:"servingCert,omitempty"`
}

// AcmeConfig configures ACME certificate provisioning
type AcmeConfig struct {
	// directoryUrl is the ACME directory URL (e.g., Let's Encrypt)
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https://.*`
	DirectoryUrl string `json:"directoryUrl"`

	// domainName is the domain name for the certificate
	// +kubebuilder:validation:Required
	DomainName string `json:"domainName"`

	// email for ACME account registration
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-zA-Z0-9][a-zA-Z0-9._%+-]*[a-zA-Z0-9]@[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?(\.[a-zA-Z0-9]([a-zA-Z0-9-]*[a-zA-Z0-9])?)*\.[a-zA-Z]{2,}$`
	Email string `json:"email"`

	// tosAccepted indicates acceptance of Terms of Service
	// +kubebuilder:default:="false"
	// +kubebuilder:validation:Enum:="true";"false"
	// +kubebuilder:validation:Optional
	TosAccepted string `json:"tosAccepted,omitempty"`
}

// ServingCertConfig configures TLS certificates for the federation endpoint.
// The service CA certificate is always used for internal communication from the Route to the
// SPIRE server pod. For external communication from clients to the Route, the certificate is
// controlled by ExternalSecretRef.
type ServingCertConfig struct {
	// fileSyncInterval is how often to check for certificate updates (seconds)
	// +kubebuilder:validation:Minimum=3600
	// +kubebuilder:validation:Maximum=7776000
	// +kubebuilder:default=86400
	FileSyncInterval int32 `json:"fileSyncInterval,omitempty"`

	// externalSecretRef is a reference to an externally managed secret that contains
	// the TLS certificate for the SPIRE server federation Route host. The secret must
	// be in the same namespace where the operator and operands are deployed and must
	// contain tls.crt and tls.key fields. The OpenShift Ingress Operator will read
	// this secret to configure the route's TLS certificate.
	// +kubebuilder:validation:Optional
	ExternalSecretRef string `json:"externalSecretRef,omitempty"`
}

// FederatesWithConfig represents a remote trust domain to federate with
// +kubebuilder:validation:XValidation:rule="self.bundleEndpointProfile == 'https_spiffe' ? has(self.endpointSpiffeId) && self.endpointSpiffeId != '' : true",message="endpointSpiffeId is required when bundleEndpointProfile is https_spiffe"
type FederatesWithConfig struct {
	// trustDomain is the federated trust domain name
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^[a-z0-9._-]{1,255}$`
	TrustDomain string `json:"trustDomain"`

	// bundleEndpointUrl is the URL of the remote federation endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Pattern=`^https://.*`
	BundleEndpointUrl string `json:"bundleEndpointUrl"`

	// bundleEndpointProfile is the authentication profile of the remote endpoint
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=https_spiffe;https_web
	BundleEndpointProfile BundleEndpointProfile `json:"bundleEndpointProfile"`

	// endpointSpiffeId is required for https_spiffe profile
	// +kubebuilder:validation:Optional
	// +kubebuilder:validation:Pattern=`^spiffe://.*`
	EndpointSpiffeId string `json:"endpointSpiffeId,omitempty"`
}

```

**Validation Rules (via CEL or webhook):**

1. If `federation.bundleEndpoint.profile == "https_web"`, then `federation.bundleEndpoint.httpsWeb` must be set
2. If `httpsWeb` is set, exactly one of `acme` or `servingCert` must be specified (mutually exclusive)
3. If `federatesWith[*].bundleEndpointProfile == "https_spiffe"`, then `endpointSpiffeId` must be set
4. `federatesWith` array length must not exceed N (cluster limit)
5. `federatesWith[*].trustDomain` must not equal `spec.trustDomain` (cannot federate with self)
6. `federatesWith[*].bundleEndpointUrl` must be valid HTTPS URL

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

Full federation support. This is the primary use case.

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

#### Operator Code Changes

The operator implementation requires modifications across several components: API types to define federation structures, ConfigMap generation to include federation configuration, Service creation/updation for internal federation endpoint exposure, Route creation for external access, StatefulSet updates to expose federation ports and mount servingCerts if configured, controller reconciliation to orchestrate federation resources and manage status conditions, and validation logic to ensure configuration correctness. When `managedRoute` is set to `false`, the operator will not reconcile or manage the Route resource for the federation endpoint created by operator, allowing administrators to manually configure custom Routes or ingress solutions for more granular control.

#### SPIRE Server Configuration Output

Example generated `server.conf` with federation:

```json
{
  "server": {
    "trust_domain": "apps.cluster-a.example.com",
    // ... other server config ...
  },
  "federation": {
    "bundle_endpoint": {
      "address": "0.0.0.0",
      "port": 8443,
      "profile" : {
        "https_spiffe": {}
      },
      "refresh_hint": "300s"
    },
    "federates_with": {
      "apps.cluster-b.example.com": {
        "bundle_endpoint_url": "https://spire-server-federation-zero-trust-workload-identity-manager.apps.cluster-b.example.com",
        "bundle_endpoint_profile": {
          "https_spiffe": {
            "endpoint_spiffe_id": "spiffe://apps.cluster-b.example.com/spire/server"
          }
        }
      }
    }
  },
  // ... plugins, telemetry, etc ...
}
```

#### Constraints and Limitations

1. **Maximum Federated Clusters**: Default limit of N (configurable) clusters to prevent performance issues.
2. **Trust Bundle Bootstrapping**: Cannot be automated. Users MUST manually bootstrap trust bundles by creating `clusterFederatedTrustDomain`
3. **Certificate Management for https_web**: Users are responsible for providing valid certificates via Secrets or configuring ACME correctly. Invalid certificates will cause federation to fail.
4. **Route Termination Constraints**:
    - **https_spiffe**: MUST use passthrough route. Reencrypt will fail due to: (a) client validation expecting SPIFFE ID in URI SAN which router certificates lack, (b) client trusting only SPIRE CA not OpenShift ingress operator CA, and (c) frequent SPIRE CA rotation (~20h) making destinationCACertificate maintenance impossible.
    - **https_web with ACME**: MUST use passthrough route. Reencrypt will fail because ACME validation requires direct SNI passthrough to SPIRE server. Router TLS termination loses original SNI, causing ACME challenges to fail.
    - **https_web with servingCert**: CAN use passthrough or reencrypt (when externalSecretRef configured). Passthrough requires certificate with external DNS name. Reencrypt allows separate backend and edge certificates. When `managedRoute` is enabled, a reencrypt route will be automatically created.
5. **Federation Bundle Endpoint Immutability**: Once configured, the Federation Bundle Endpoint and its associated profile are immutable and cannot be removed or modified during Day-2 operations. API validation enforces this constraint to maintain system stability, as the endpoint serves as the trust anchor for all federated peers. To disable federation, the system must be uninstalled and reinstalled. Peer configurations (`federatesWith`) remain dynamic and can be added or removed at any time.

### Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| **Misconfigured federation breaks cross-cluster communication** | High - workloads cannot communicate across clusters | - Comprehensive validation at API admission time<br>- Clear status conditions showing federation configuration validation<br>- Detailed documentation and examples<br>- E2E tests for common scenarios |
| **Manual trust bundle bootstrapping is error-prone** | High - federation won't work without correct bootstrapping | - Detailed step-by-step documentation<br>|
| **Too many federated clusters cause performance degradation** | Medium - SPIRE server becomes slow or unstable | - Enforce maximum limit <br>- Document performance characteristics<br>- Test with maximum number of clusters |

### Drawbacks

The solution can incur performance overhead, increase troubleshooting complexity, and lead to higher resource consumption under heavy load.


## Alternatives (Not Implemented)

1. Add ingress instead of route for exposing federation endpoint

## Open Questions

1. **Should we support dynamic trust domain changes?**
   - Current proposal: No, requires complete re-federation.
   - Question: Should we detect and provide better guidance when trust domain changes?

2. **Should we support automatic failover if a federated cluster becomes unavailable?**
   - Should operator surface this information in status?
   - Should we add health checks for federated endpoints?

## Test Plan

### Unit Tests
- API validation (valid/invalid configs, edge cases)
- ConfigMap/Service/Route generation

### Integration Tests
- Controller reconciliation flow (create, update, delete resources)
- Create-only mode behavior
- Status condition updates

### E2E Tests
- Two-cluster federation setup and verification
- `N+1`th cluster addition
- Configuration updates and pod restarts

### Performance Tests
- Bundle refresh latency with varying cluster counts
- Federation endpoint load testing
- Resource usage measurement

## Graduation Criteria

The feature will directly be a part of GA release.

### Dev Preview -> Tech Preview

Not applicable.

### Tech Preview -> GA

Not applicable.


### Removing a deprecated feature

## Upgrade / Downgrade Strategy

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Infrastructure Needed [optional]
None
