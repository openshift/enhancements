---
title: kms-shim
authors:
  - "@ardaguclu"
  - "@flavianmissi"
reviewers:
  - "@ibihim"
  - "@sjenning"
  - "@tkashem"
  - "@derekwaynecarr"
approvers:
  - "@benluddy"
api-approvers:
  - "None"
creation-date: 2025-12-04
last-updated: 2025-12-04
tracking-link:
  - "https://issues.redhat.com/browse/OCPSTRAT-108"  # TP feature
  - "https://issues.redhat.com/browse/OCPSTRAT-1638" # GA feature
see-also:
  - "enhancements/kube-apiserver/kms-encryption-foundations.md"
  - "enhancements/kube-apiserver/kms-plugin-management.md"
  - "enhancements/kube-apiserver/kms-migration-recovery.md"
replaces:
  - ""
superseded-by:
  - ""
---

# KMS Shim for User-Managed KMS Plugins

## Summary

Provide a lightweight KMS shim architecture that enables users to deploy and manage their own KMS plugins (AWS, Vault, Thales, etc.) while OpenShift handles the complexity of Unix socket communication required by the Kubernetes KMS v2 API. OpenShift provides a socket proxy container image that users deploy alongside their KMS plugins to translate between network communication (used by the shim in API server pods) and Unix socket communication (required by standard KMS v2 plugins). This creates a clear support boundary, reduces Red Hat's support burden, and allows users to deploy KMS plugins anywhere (in-cluster, external infrastructure, or separate clusters) and update them independently of OpenShift's release cycle.

## Motivation

Managing native KMS plugins for multiple providers (AWS, Vault, Azure, GCP, Thales) places significant burden on Red Hat:
- Expertise required in 5+ external KMS systems
- Integration with provider-specific authentication (IAM, AppRole, certificates, PKCS#11)
- Support escalations requiring coordination with external vendors
- Plugin updates tied to OpenShift release cycle
- Security vulnerabilities in plugins require emergency releases

Additionally, users cannot easily update KMS plugins to fix CVEs or bugs without waiting for OpenShift updates.

A shim-based architecture allows users to deploy their own KMS plugins while OpenShift provides only the Unix socket adapter, creating a clear support boundary and reducing complexity.

### User Stories

* As a cluster admin, I want to deploy standard upstream KMS plugins (AWS, Vault, Azure, GCP, Thales) without modification, so that I can use well-tested community implementations
* As a cluster admin, I want to deploy my KMS plugin anywhere (in-cluster, on external infrastructure, or in a separate cluster), so that I have architectural flexibility
* As a cluster admin, I want OpenShift to provide the socket proxy component, so that I don't need to write network-to-Unix-socket translation code myself
* As a cluster admin, I want to update my KMS plugin independently of OpenShift releases, so that I can fix security vulnerabilities or bugs without waiting for a new OpenShift version
* As a cluster admin, I want OpenShift to handle key rotation orchestration, so that I don't need to understand OpenShift's encryption controller internals
* As a Red Hat support engineer, I want a clear support boundary between OpenShift-managed components (shim + socket proxy images) and user-managed components (plugin deployment and configuration), so that I can efficiently troubleshoot issues without needing deep expertise in every KMS provider

### Goals

* Enable users to deploy **unmodified upstream KMS v2 plugins** (no custom builds or forks required)
* Provide socket proxy container image that users deploy alongside their plugins
* Support flexible deployment models: in-cluster, external infrastructure, or separate clusters
* Provide lightweight shim and socket proxy components that handle Unix socket ↔ network translation
* Reduce Red Hat's support scope to shim and socket proxy images (not plugin deployment, configuration, or KMS provider integration)
* Allow user-managed plugins to be deployed and updated independently of OpenShift
* Maintain full automation of key rotation and migration (no user intervention required for KEK rotation)
* Support all KMS providers that implement the Kubernetes KMS v2 API

### Non-Goals

* Managing user KMS plugin deployment or lifecycle (user deploys plugin + socket proxy)
* Providing KMS plugin images or implementations (only socket proxy image)
* Automatic injection or mutation of user plugin pods
* Prescribing how users deploy socket proxy (sidecar, separate pod, external - all valid)
* Authentication between shim and socket proxy (out of scope for Tech Preview)
* Custom or non-standard KMS plugin interfaces (only KMS v2 API supported)
* Performance optimization beyond minimal overhead (network hops are acceptable tradeoff)

## Proposal

Deploy a two-component architecture that enables user-managed KMS plugins while maintaining compatibility with the Kubernetes KMS v2 Unix socket API:

1. **KMS Shim** (OpenShift-managed sidecar in API server pods): Translates Unix socket → HTTP/gRPC network calls
2. **Socket Proxy** (OpenShift-provided image, user-deployed): Translates HTTP/gRPC network calls → Unix socket

This architecture solves the **SELinux MCS isolation problem** that prevents different pods from sharing Unix sockets via hostPath, while allowing users to deploy standard upstream KMS v2 plugins without modification. Users deploy the socket proxy alongside their plugin using OpenShift's provided container image, giving them full control over the deployment architecture (in-cluster, external, or hybrid).

**Complete Architecture:**
```
┌──────────────────────────────────────────────────────────┐
│ Control Plane: API Server Pod (OpenShift-managed)       │
│ SELinux MCS: s0:c333,c444                                │
│                                                          │
│  ┌─────────────────┐         ┌────────────────────┐     │
│  │ API Server      │  Unix   │ KMS Shim           │     │
│  │ Container       │ Socket  │ Sidecar            │     │
│  │                 │◄───────►│                    │─────┼──┐
│  │ kube-apiserver/ │ emptyDir│ Unix→HTTP          │     │  │
│  │ openshift-api/  │ (same   │ Intelligent routing│     │  │
│  │ oauth-api       │  MCS)   │                    │     │  │
│  └─────────────────┘         └────────────────────┘     │  │
└──────────────────────────────────────────────────────────┘  │
                                                              │
                  HTTP/gRPC to user-configured endpoint       │
           (Kubernetes Service, external URL, or IP address)  │
                                                              │
┌──────────────────────────────────────────────────────────┐  │
│ User-Deployed Plugin (flexible architecture)            │  │
│ User controls: location, pod layout, networking         │◄─┘
│ SELinux MCS: s0:c111,c222 (if in-cluster)                │
│                                                          │
│  ┌────────────────────┐       ┌────────────────────┐    │
│  │ Socket Proxy       │ Unix  │ User's KMS Plugin  │    │
│  │ (user deploys      │Socket │ Container          │    │
│  │  OpenShift image)  │◄─────►│ (User-managed)     │    │
│  │ HTTP→Unix          │       │                    │    │
│  │                    │ (user │ Standard upstream  │    │
│  │ Listens: :8080     │ choice│ KMS v2 plugin      │    │
│  │ Forwards: unix://  │ how to│ (unmodified)       │    │
│  │                    │ share)│                    │    │
│  └────────────────────┘       └────────────────────┘    │
│                                         │                │
└─────────────────────────────────────────┼────────────────┘
                                          │
                                          ▼
                                External KMS Provider
                                (AWS KMS, Vault, Thales, etc.)

Alternative: Plugin + Socket Proxy on External Infrastructure
┌──────────────────────────────────────────────────────────┐
│ External VM / Separate Cluster / Cloud Provider         │
│                                                          │
│  Socket Proxy + Plugin                                  │
│  Exposed via: Load Balancer / Ingress / VPN             │
│  Example: https://kms.company.com:8080                  │
└──────────────────────────────────────────────────────────┘
```

**Key Innovation 1: Intelligent Routing in Shim**

The shim maintains multiple endpoint configurations and routes requests intelligently based on operation type:

- **Encrypt requests**: Always sent to the **primary endpoint** (the `endpoint` field)
- **Decrypt requests**: Try **primary endpoint** first, then fall back to **additional endpoints** (the `additionalEndpoints` field) if decryption fails
- **Status requests**: Always sent to the **primary endpoint**

**Terminology:**
- **Primary endpoint** = `spec.encryption.kms.external.endpoint` = Used for new encryptions and status checks
- **Additional endpoints** = `spec.encryption.kms.external.additionalEndpoints` = Used only for decryption fallback during KEK change migrations

This eliminates the need for multiple shim instances during KEK changes, making KEK changes as transparent to users as KEK rotations.

**Key Innovation 2: User-Controlled Deployment with OpenShift-Provided Components**

OpenShift provides the socket proxy container image, and users deploy it however they choose:
- **In-cluster as sidecar**: User deploys plugin + socket proxy in same pod (simplest)
- **In-cluster as separate pods**: User deploys plugin and socket proxy in separate pods (if they prefer)
- **External infrastructure**: User deploys plugin + socket proxy on VMs, separate clusters, or cloud provider infrastructure
- **Hybrid**: Any combination of the above

Users have full control over deployment architecture, networking, and resource management.

**Responsibilities:**

**OpenShift Provides:**
- **Shim container image**: Sidecar for API server pods, translates Unix socket → HTTP/gRPC
- **Socket proxy container image**: Translates HTTP/gRPC → Unix socket for standard KMS v2 plugins
- **Operators**: Inject shim into API server pods, validate endpoint reachability
- **Monitoring**: Metrics specifications for shim and socket proxy
- **Documentation**: Example deployment YAMLs for common KMS providers
- **Lifecycle**: Manage shim and socket proxy images and updates via release payload

**User Deploys and Manages:**
- KMS plugin pod(s) running standard upstream KMS v2 plugin
- Socket proxy container (using OpenShift-provided image) deployed with plugin
- Networking configuration (Service, Ingress, Load Balancer, etc.) to expose socket proxy endpoint
- Plugin configuration (Vault address, AWS credentials, key IDs, etc.)
- KMS provider-specific authentication (Vault tokens, AWS IAM, certificates)
- Plugin and socket proxy updates independently
- Additional plugin instances when changing KEKs (deploy new plugin + socket proxy, configure endpoint)

### Workflow Description

#### Roles

**cluster admin** is a human user responsible for configuring and maintaining the cluster.

**KMS Shim** is an OpenShift-managed sidecar container in API server pods that translates Unix socket calls to HTTP/gRPC network calls.

**Socket Proxy** is a user-deployed container (using OpenShift-provided image) that translates HTTP/gRPC network calls to Unix socket calls.

**KMS Plugin** is a user-deployed standard upstream plugin (AWS, Vault, Azure, GCP, Thales) implementing the Kubernetes KMS v2 Unix socket API.

**External KMS** is the cloud or on-premises Key Management Service (AWS KMS, HashiCorp Vault, Thales HSM).

#### Initial KMS Configuration

1. The cluster admin deploys their KMS plugin with socket proxy (example: in-cluster sidecar pattern):
   ```yaml
   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: vault-kms-plugin
     namespace: kms-plugins
   spec:
     replicas: 3
     selector:
       matchLabels:
         app: vault-kms
     template:
       metadata:
         labels:
           app: vault-kms
       spec:
         containers:
         # User's plugin (standard upstream, unmodified)
         - name: vault-kms
           image: registry.k8s.io/kms/vault-kms-plugin:v0.4.0
           args:
           - --socket=/socket/kms.sock
           - --vault-addr=https://vault.company.com
           volumeMounts:
           - name: socket
             mountPath: /socket

         # OpenShift-provided socket proxy (user deploys it)
         - name: socket-proxy
           image: registry.redhat.io/openshift4/kms-socket-proxy:v4.17
           args:
           - --listen-addr=:8080
           - --socket-path=/socket/kms.sock
           ports:
           - containerPort: 8080
           volumeMounts:
           - name: socket
             mountPath: /socket

         volumes:
         - name: socket
           emptyDir: {}
   ---
   apiVersion: v1
   kind: Service
   metadata:
     name: vault-kms-plugin
     namespace: kms-plugins
   spec:
     selector:
       app: vault-kms
     ports:
     - port: 8080
       targetPort: 8080
   ```

2. The cluster admin updates the APIServer configuration with the socket proxy endpoint:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: APIServer
   metadata:
     name: cluster
   spec:
     encryption:
       type: KMS
       kms:
         type: External
         external:
           endpoint: http://vault-kms-plugin.kms-plugins.svc:8080
   ```

3. API server operators detect the configuration change
4. Operators validate endpoint is reachable (health check + KMS Status call)
5. Operators inject KMS shim sidecars into API server pods
6. Operators configure shim to forward to `http://vault-kms-plugin.kms-plugins.svc:8080`
7. Shim creates Unix socket for API server (e.g., `/var/run/kmsplugin/kms-abc123.sock`)
8. Encryption controllers detect new KMS configuration and begin encryption

**What the user deployed**: Plugin + socket proxy + Service (full control)
**What OpenShift manages**: Shim sidecar in API server pods, endpoint validation

#### KEK Rotation (Key Materials Change)

1. External KMS rotates key materials (e.g., AWS KMS creates new version)
2. User's plugin detects rotation and returns new `key_id` via Status gRPC call
3. OpenShift's keyController polls shim Status endpoint
4. Shim forwards Status call to user plugin
5. keyController detects `key_id` change
6. keyController triggers data migration (re-encrypt with new key)
7. **User does nothing** - same plugin instance handles both old and new key internally

#### KEK Change (Switching to Different Key)

1. Cluster admin creates KEK in new KMS or different key in same KMS
2. Cluster admin deploys second instance of plugin + socket proxy with new key configuration:
   ```bash
   kubectl apply -f vault-kms-plugin-new.yaml  # Includes plugin + socket proxy + Service
   ```
   This creates a second endpoint (e.g., `http://vault-kms-new.kms-plugins.svc:8080`)

3. Cluster admin updates APIServer config with both endpoints:
   ```yaml
   spec:
     encryption:
       type: KMS
       kms:
         type: External
         external:
           endpoint: http://vault-kms-new.kms-plugins.svc:8080
           additionalEndpoints:
           - http://vault-kms-old.kms-plugins.svc:8080
   ```

4. Operators detect configuration change and update shim configuration
5. **Same shim instance** now routes to multiple endpoints:
   - Encrypt requests → **primary endpoint** (`endpoint` field = new key)
   - Decrypt requests → try **primary endpoint** first, fall back to **additional endpoints** (`additionalEndpoints` = old key) if decryption fails
6. Migration proceeds automatically:
   - API server reads old secrets, shim routes decrypt to **additional endpoints** (old key)
   - API server re-encrypts, shim routes encrypt to **primary endpoint** (new key)
7. Once migration completes, cluster admin removes `additionalEndpoints` from config
8. Shim stops routing to **additional endpoints**, old plugin deployment can be deleted

**User experience:** Similar to KEK rotation - just update config once, OpenShift handles the rest

### API Extensions

This enhancement does not introduce new CRDs or API extensions. It extends the existing `config.openshift.io/v1/APIServer` resource with new fields for external KMS plugin configuration.

**API Changes:**

```go
// In config/v1/types_kmsencryption.go

// KMSProviderType defines the supported KMS provider types.
//
// Currently only "External" is supported. Future versions may add
// OpenShift-managed provider types.
//
// +kubebuilder:validation:Enum=External
type KMSProviderType string

const (
    // ExternalKMSProvider indicates user-managed KMS plugins.
    // Users deploy their own KMS plugin + OpenShift-provided socket proxy.
    ExternalKMSProvider KMSProviderType = "External"
)

// KMSConfig defines the configuration for KMS encryption.
// The structure is extensible to support future OpenShift-managed KMS providers
// alongside the current user-managed External provider type.
//
// +kubebuilder:validation:XValidation:rule="self.type == 'External' ? has(self.external) : !has(self.external)",message="external config is required when type is External, and forbidden otherwise"
// +union
type KMSConfig struct {
    // type defines the KMS provider type.
    //
    // Currently only "External" is supported, which enables user-managed KMS plugins.
    // Future versions may add OpenShift-managed provider types (e.g., "AWS", "Vault")
    // where OpenShift handles plugin deployment and lifecycle.
    //
    // +unionDiscriminator
    // +kubebuilder:validation:Required
    Type KMSProviderType `json:"type"`

    // external defines the configuration for user-managed KMS plugins.
    // Required when type is "External".
    //
    // +unionMember
    // +optional
    External *ExternalKMSConfig `json:"external,omitempty"`
}

// ExternalKMSConfig defines the configuration for user-managed KMS plugins.
// Users deploy standard upstream KMS v2 plugins with the OpenShift-provided
// socket proxy container, and configure the socket proxy endpoint here.
//
// The shim routes requests intelligently based on operation type:
// - Encrypt requests: Always sent to the primary endpoint (endpoint field)
// - Decrypt requests: Try primary endpoint first, fall back to additionalEndpoints if decryption fails
// - Status requests: Always sent to the primary endpoint (endpoint field)
type ExternalKMSConfig struct {
    // endpoint specifies the primary network address where the socket proxy is listening.
    // This endpoint is used for:
    // - All new encryption operations (Encrypt requests)
    // - Plugin health and key rotation detection (Status requests)
    // - Decryption attempts (tried first before falling back to additionalEndpoints)
    //
    // The socket proxy must be deployed by the user alongside their KMS plugin.
    //
    // This can be:
    // - Kubernetes Service: http://vault-kms-plugin.kms-plugins.svc:8080
    // - External URL: https://kms.company.com:8080
    // - IP address: http://10.0.1.50:8080
    //
    // The endpoint must be reachable from API server pods and respond to
    // KMS v2 API calls (forwarded by the socket proxy to the actual plugin).
    //
    // Example: "http://vault-kms-plugin.kms-plugins.svc:8080"
    //
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:Pattern=`^https?://[a-zA-Z0-9]([a-zA-Z0-9.-]*[a-zA-Z0-9])?(:[0-9]+)?(/.*)?$`
    Endpoint string `json:"endpoint"`

    // additionalEndpoints specifies additional socket proxy endpoints used during
    // KEK change operations for decryption fallback.
    //
    // These endpoints are ONLY used for Decrypt requests when the primary endpoint
    // fails to decrypt data. This happens when data was encrypted with a previous
    // KMS key that the primary endpoint doesn't have access to.
    //
    // The shim tries the primary endpoint first for all Decrypt requests. If
    // decryption fails (because data was encrypted with an old key), the shim
    // tries each endpoint in additionalEndpoints until one succeeds.
    //
    // Encrypt and Status requests NEVER use additionalEndpoints - they always
    // use the primary endpoint.
    //
    // These should be removed after migration to the new key completes (when all
    // data has been re-encrypted with the new key).
    //
    // Example use case: Migrating from one KMS key to another. User deploys
    // a second instance of their plugin with the new key, sets that as the
    // primary endpoint, and adds the old plugin endpoint here. Both run
    // simultaneously until all data is re-encrypted with the new key.
    //
    // +optional
    // +kubebuilder:validation:MaxItems=10
    AdditionalEndpoints []string `json:"additionalEndpoints,omitempty"`
}
```

**Example Configuration:**

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  encryption:
    type: KMS
    kms:
      type: External
      external:
        endpoint: http://vault-kms-plugin.kms-plugins.svc:8080
```

**Example with KEK change (two endpoints during migration):**

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  encryption:
    type: KMS
    kms:
      type: External
      external:
        endpoint: http://vault-kms-new.kms-plugins.svc:8080
        additionalEndpoints:
        - http://vault-kms-old.kms-plugins.svc:8080
```

**Example with external deployment:**

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  encryption:
    type: KMS
    kms:
      type: External
      external:
        endpoint: https://kms.company.com:8080
```

#### Future Extensibility

The API structure is designed to support future OpenShift-managed KMS provider types alongside the current user-managed External type. In future enhancements, the API could be extended as follows:

```go
// +kubebuilder:validation:Enum=External;AWS;Vault;Thales
type KMSProviderType string

const (
    ExternalKMSProvider KMSProviderType = "External"
    AWSKMSProvider      KMSProviderType = "AWS"      // Future: OpenShift-managed AWS KMS
    VaultKMSProvider    KMSProviderType = "Vault"    // Future: OpenShift-managed Vault KMS
    ThalesKMSProvider   KMSProviderType = "Thales"   // Future: OpenShift-managed Thales KMS
)

// +kubebuilder:validation:XValidation:rule="self.type == 'External' ? has(self.external) : !has(self.external)",message="external config required when type is External"
// +kubebuilder:validation:XValidation:rule="self.type == 'AWS' ? has(self.aws) : !has(self.aws)",message="aws config required when type is AWS"
// +union
type KMSConfig struct {
    Type     KMSProviderType     `json:"type"`
    External *ExternalKMSConfig  `json:"external,omitempty"`
    AWS      *AWSKMSConfig       `json:"aws,omitempty"`      // Future
    Vault    *VaultKMSConfig     `json:"vault,omitempty"`    // Future
    Thales   *ThalesKMSConfig    `json:"thales,omitempty"`   // Future
}
```

This extensible design allows future enhancements to add OpenShift-managed KMS providers (where OpenShift deploys and manages the KMS plugin lifecycle) without breaking the existing External provider or requiring API versioning changes. The union discriminator pattern ensures only the appropriate provider configuration is set for each type.

### Topology Considerations

#### Hypershift / Hosted Control Planes

In Hypershift, the shim runs in the management cluster alongside the hosted control plane's API servers. User-deployed KMS plugins must also run in the management cluster and be network-accessible from the shim sidecars.

No fundamental differences from standalone clusters, but users must deploy plugins in the management cluster's appropriate namespace.

#### Standalone Clusters

This is the primary deployment model. Shim sidecars run in each API server pod, forwarding to user-deployed plugins in any namespace.

#### Single-node Deployments or MicroShift

**Resource Impact:**
- Each API server pod adds one shim sidecar (~30MB memory, minimal CPU)
- Total: 3 shim sidecars for 3 API servers
- Additional network latency: ~1-5ms per KMS operation (local cluster network)

MicroShift may adopt this approach if KMS encryption is desired, using file-based configuration instead of APIServer CR.

### Implementation Details/Notes/Constraints

#### Shim Implementation

The shim is a lightweight Go binary implementing:

**Core Functionality:**
- **Unix Socket Server**: Implements KMS v2 gRPC API, listens on Unix socket
- **HTTP/gRPC Client**: Forwards requests to configured endpoints
- **Intelligent Routing**: Routes requests based on operation type with fallback on failure
- **Configuration**: Reads endpoints from environment variables or config file
- **Health Checks**: Exposes `/healthz` endpoint for liveness/readiness probes

**Code Structure:**
```
pkg/kmsshim/
├── server.go         # Unix socket gRPC server
├── client.go         # HTTP/gRPC client to external plugins
├── router.go         # Intelligent routing logic (NEW)
├── config.go         # Configuration loading (multiple endpoints)
└── metrics.go        # Prometheus metrics
```

**Key Methods with Intelligent Routing:**
```go
type Shim struct {
    primaryEndpoint     string      // Primary endpoint (from spec.encryption.kms.external.endpoint)
    additionalEndpoints []string    // Additional endpoints (from spec.encryption.kms.external.additionalEndpoints)
    clients             map[string]*KMSClient  // Endpoint → client mapping
}

// Encrypt always uses the primary endpoint
func (s *Shim) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
    client := s.clients[s.primaryEndpoint]
    return client.Encrypt(ctx, req)
}

// Decrypt tries primary endpoint first, falls back to additional endpoints
func (s *Shim) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
    // Try primary endpoint first (most likely to succeed - most data encrypted with current key)
    client := s.clients[s.primaryEndpoint]
    resp, err := client.Decrypt(ctx, req)
    if err == nil {
        return resp, nil
    }

    // If primary endpoint fails, try additional endpoints
    // This handles data encrypted with previous keys during KEK change
    for _, endpoint := range s.additionalEndpoints {
        client := s.clients[endpoint]
        resp, err := client.Decrypt(ctx, req)
        if err == nil {
            // Successfully decrypted with an additional endpoint (old key)
            return resp, nil
        }
    }

    return nil, fmt.Errorf("decryption failed with all configured endpoints (primary + %d additional)", len(s.additionalEndpoints))
}

// Status always uses the primary endpoint
func (s *Shim) Status(ctx context.Context, req *StatusRequest) (*StatusResponse, error) {
    client := s.clients[s.primaryEndpoint]
    return client.Status(ctx, req)
}
```

**Why This Works:**

The shim uses a **try-first-then-fallback** approach for decryption:

1. **Try primary endpoint first** (most data encrypted with current key, so most likely to succeed)
2. **Fall back to additional endpoints** if primary decryption fails (data encrypted with old keys during migration)
3. **Return successful decryption** from whichever endpoint works

This approach is simple and works because:
- The primary endpoint handles the new key (most common case)
- Additional endpoints only needed during KEK change (temporary)
- Each plugin knows which keys it has access to and will succeed/fail accordingly
- No need to maintain key_id → endpoint mapping in the shim
- No need for multiple shim instances or complex socket path management

The Kubernetes KMS v2 API includes `key_id` in both EncryptResponse and DecryptRequest, but the shim doesn't need to inspect or match it - the plugins themselves handle key identification.

#### Socket Proxy Implementation

The socket proxy is a lightweight Go binary that translates between network and Unix socket communication:

**Core Functionality:**
- **HTTP/gRPC Server**: Listens on TCP port 8080 for connections from shim
- **Unix Socket Client**: Forwards requests to user's KMS plugin via Unix socket
- **Bidirectional Translation**: HTTP/gRPC ↔ Unix socket for all KMS v2 API methods
- **Health Checks**: Exposes `/healthz` endpoint for liveness/readiness probes

**Code Structure:**
```
pkg/socketproxy/
├── server.go         # HTTP/gRPC server listening on :8080
├── client.go         # Unix socket client to KMS plugin
├── translator.go     # Protocol translation logic
└── health.go         # Health check endpoint
```

**Key Methods:**
```go
type SocketProxy struct {
    listenAddr    string      // :8080
    socketPath    string      // /socket/kms.sock
    grpcServer    *grpc.Server
    socketClient  *KMSPluginClient
}

// Encrypt translates HTTP/gRPC → Unix socket
func (p *SocketProxy) Encrypt(ctx context.Context, req *EncryptRequest) (*EncryptResponse, error) {
    return p.socketClient.Encrypt(ctx, req)
}

// Decrypt translates HTTP/gRPC → Unix socket
func (p *SocketProxy) Decrypt(ctx context.Context, req *DecryptRequest) (*DecryptResponse, error) {
    return p.socketClient.Decrypt(ctx, req)
}

// Status translates HTTP/gRPC → Unix socket
func (p *SocketProxy) Status(ctx context.Context, req *StatusRequest) (*StatusResponse, error) {
    return p.socketClient.Status(ctx, req)
}
```

#### Endpoint Validation

Before injecting the shim sidecar, operators validate that the configured endpoint is reachable and responds to KMS v2 API calls. This ensures the user has correctly deployed their plugin + socket proxy.

**Validation Steps:**

```go
func (c *Controller) validateKMSEndpoint(ctx context.Context, endpoint string) error {
    // 1. Health check the socket proxy
    healthURL := endpoint + "/healthz"
    resp, err := http.Get(healthURL)
    if err != nil {
        return fmt.Errorf("socket proxy health check failed at %s: %w", healthURL, err)
    }
    if resp.StatusCode != 200 {
        return fmt.Errorf("socket proxy unhealthy at %s: status %d", healthURL, resp.StatusCode)
    }

    // 2. Validate KMS v2 API (Status call via gRPC over HTTP/2)
    conn, err := grpc.Dial(endpoint, grpc.WithInsecure())
    if err != nil {
        return fmt.Errorf("failed to connect to KMS plugin at %s: %w", endpoint, err)
    }
    defer conn.Close()

    client := kmsv2.NewKeyManagementServiceClient(conn)
    _, err = client.Status(ctx, &kmsv2.StatusRequest{})
    if err != nil {
        return fmt.Errorf("KMS plugin Status call failed at %s: %w", endpoint, err)
    }

    return nil
}
```

**Operator Behavior:**

- If validation succeeds: Inject shim, enable encryption
- If validation fails: Set condition `KMSPluginAvailable=False` with error details
- Operators periodically retry validation (every 60 seconds)

**Operator Conditions:**

```yaml
status:
  conditions:
  - type: KMSPluginAvailable
    status: "True"
    reason: PluginHealthy
    message: "KMS plugin endpoint http://vault-kms.kms-plugins.svc:8080 is reachable and healthy"

  # If validation fails:
  - type: KMSPluginAvailable
    status: "False"
    reason: EndpointUnreachable
    message: "KMS plugin endpoint http://vault-kms.kms-plugins.svc:8080 is not reachable. Please verify the plugin and socket proxy are deployed and the endpoint is correct."
```

#### Shim Sidecar Injection

Operators inject the shim sidecar into API server pods after endpoint validation succeeds:

**kube-apiserver-operator:**
- Modifies static pod manifest in `targetconfigcontroller.managePods()`
- Adds shim container with endpoint URL from APIServer CR (`external.endpoint`)
- Configures shim with `additionalEndpoints` if specified
- Uses hostPath volume for socket

**openshift-apiserver-operator:**
- Modifies deployment spec in `workload.manageOpenShiftAPIServerDeployment_v311_00_to_latest()`
- Adds shim container with endpoint configuration
- Uses emptyDir volume for socket isolation

**authentication-operator (oauth-apiserver):**
- Modifies deployment spec in `workload.syncDeployment()`
- Adds shim container with endpoint configuration
- Uses emptyDir volume for socket isolation

#### Socket Path Generation

Socket paths are calculated based on configuration hash:

```go
socketPath := fmt.Sprintf("/var/run/kmsplugin/kms-%s.sock", configHash)
```

Where `configHash` is computed from:
- KMS provider type (External)
- Primary endpoint URL

**Note:** Unlike native plugins, the socket path does NOT change during KEK changes because the same shim instance handles routing to multiple endpoints internally. This simplifies the EncryptionConfiguration and eliminates the need for multi-socket management.

#### Configuration Updates During KEK Change

When `additionalEndpoints` is added or modified:

1. Operators detect configuration change in APIServer CR
2. Operators validate new endpoints are reachable
3. Operators update shim configuration with additional endpoints
4. **Same shim instance** is updated in-place (no pod restart required)
5. Shim reloads configuration, adds new endpoint clients
6. EncryptionConfiguration remains unchanged (same socket path)
7. Migration proceeds automatically with shim routing decrypt requests appropriately

**Simplified flow compared to native plugins:**
- ✅ One shim instance (not two)
- ✅ One socket path (not two)
- ✅ No EncryptionConfiguration changes needed
- ✅ Configuration update instead of pod rollout
- ✅ User experience similar to KEK rotation
- ✅ Users deploy second plugin instance and update APIServer CR - operators handle shim configuration

#### Error Handling

**Shim cannot reach plugin Service:**
- Shim returns gRPC error to API server
- Error message includes Service name and reason (DNS failure, connection refused, timeout)
- Metrics incremented: `kms_shim_forward_errors_total{service="...", reason="..."}`

**Socket proxy returns error:**
- Shim forwards error to API server unchanged
- Metrics incremented: `kms_shim_plugin_errors_total{service="..."}`

**Plugin pod unavailable:**
- Socket proxy loses connection to plugin Unix socket
- Socket proxy returns error to shim
- API server retries operations (cached DEKs continue working for reads)

**Shim container crash:**
- Kubernetes restarts shim (standard pod restart policy)
- API server retries KMS operations
- Temporary encryption/decryption failures until shim recovers

**Socket proxy container crash:**
- Kubernetes restarts socket proxy in plugin pod
- Brief interruption in plugin availability
- Shim retries connection to Service

#### Metrics and Monitoring

**Shim Metrics:**

The shim exposes Prometheus metrics for monitoring plugin communication:

```
# Request forwarding metrics
kms_shim_requests_total{operation="encrypt|decrypt|status", service="..."}
kms_shim_request_duration_seconds{operation="...", service="..."}
kms_shim_forward_errors_total{service="...", reason="dns|connection|timeout"}

# Plugin health metrics
kms_shim_plugin_errors_total{service="...", error_code="..."}
kms_shim_plugin_healthy{service="..."}  # 1 = healthy, 0 = unhealthy
```

**Socket Proxy Metrics:**

The socket proxy exposes metrics for monitoring plugin health:

```
# Request translation metrics
socket_proxy_requests_total{operation="encrypt|decrypt|status"}
socket_proxy_request_duration_seconds{operation="..."}
socket_proxy_socket_errors_total{reason="connection_refused|timeout"}

# Plugin socket health
socket_proxy_plugin_connected{plugin="..."}  # 1 = connected, 0 = disconnected
```

Operators expose both shim and socket proxy metrics via existing monitoring infrastructure.

### Risks and Mitigations

#### Risk: Network Latency Impact

**Risk:** Extra network hop (API server → shim → socket proxy → plugin) adds latency to every encryption operation.

**Impact:**
- Estimated +2-8ms per operation for in-cluster network (two network hops)
- Secret creation, ConfigMap updates affected

**Mitigation:**
- Document expected performance impact
- Measure latency in testing, establish SLOs
- Recommend users deploy plugins in same namespace as control plane for minimal network distance
- Future: Add caching layer in shim if performance becomes issue
- Network overhead acceptable tradeoff for SELinux compliance and operator simplicity

#### Risk: Shim Cannot Reach Endpoint

**Risk:** Network policy, DNS failure, endpoint misconfiguration, or user deployment issues prevent shim from reaching socket proxy.

**Impact:**
- Encryption/decryption operations fail
- Cluster cannot create/read secrets
- Partial or complete cluster unavailability

**Mitigation:**
- Pre-flight validation: Operators verify endpoint is reachable before injecting shim
- Validation includes health check + KMS Status call
- Clear error messages in operator conditions when validation fails
- Documentation with troubleshooting guide for common network issues
- Example YAMLs reduce user deployment errors
- User can test endpoint connectivity before configuring APIServer CR

#### Risk: User Deployment Errors

**Risk:** Users incorrectly deploy plugin + socket proxy (wrong socket path, missing Service, port mismatch, etc.)

**Impact:**
- Endpoint validation fails
- KMS encryption cannot be enabled
- User frustration and support burden

**Mitigation:**
- Comprehensive documentation with copy-paste example YAMLs
- Examples for common deployment patterns (in-cluster sidecar, external, etc.)
- Validation errors provide specific guidance (e.g., "health check failed at http://... - verify socket proxy is running")
- Socket proxy exposes health endpoint for easy testing
- Troubleshooting guide with common mistakes

#### Risk: Support Boundary Confusion

**Risk:** Users unclear whether issues are in OpenShift components (shim/socket proxy images) or their deployment.

**Impact:**
- Inefficient support escalations
- User frustration

**Mitigation:**
- Clear documentation: "Red Hat supports shim and socket proxy **images only**, not user deployments or plugin configuration"
- Validation errors clearly indicate what failed (endpoint unreachable vs plugin returning errors)
- Shim logs clearly indicate forwarding success/failure
- Socket proxy logs indicate plugin socket connection status
- Metrics distinguish between shim errors, socket proxy errors, and plugin errors
- Troubleshooting guide helps users diagnose their deployment issues independently

#### Risk: Endpoint URL Flexibility Creates Complexity

**Risk:** Users can deploy plugins anywhere (in-cluster, external, hybrid), making troubleshooting more complex.

**Impact:**
- Support cannot assume deployment architecture
- Network troubleshooting varies by architecture
- More documentation needed to cover all scenarios

**Mitigation:**
- Document common deployment patterns clearly
- Validation logic works regardless of architecture (just checks endpoint reachability)
- Troubleshooting guide organized by symptom, not architecture
- Trade-off justified by architectural flexibility users gain

### Drawbacks

1. **Performance overhead**: Two network hops (shim → socket proxy → plugin) add latency compared to native plugins
2. **User deployment responsibility**: Users must deploy plugin + socket proxy + Service/networking themselves
3. **Configuration complexity**: Users must understand how to deploy containers, create Services, and configure networking
4. **Support complexity**: Three-layer architecture (shim, socket proxy, plugin) creates more troubleshooting surface area
5. **Deployment errors**: Users can misconfigure socket path, port, or networking
6. **Socket proxy failure mode**: If socket proxy crashes, entire plugin becomes unavailable until restart
7. **No automatic updates**: Users must manually update socket proxy image when OpenShift releases new version (though updates are backward compatible)

## Alternatives (Not Implemented)

### Alternative 1: OpenShift Manages Native Plugins

**Approach:** OpenShift operators deploy provider-specific native plugins as sidecars.

**Why not chosen:**
- Requires Red Hat to support 5+ external KMS systems
- Plugin updates tied to OpenShift release cycle
- Large support burden (IAM, Vault auth, PKCS#11, etc.)
- User cannot update plugins independently

**This is the current design in Enhancement B (kms-plugin-management.md).**

### Alternative 2: User Manages Plugins with hostPath (DaemonSet Approach)

**Approach:** Users deploy KMS plugins as DaemonSets on control plane nodes with hostPath volumes. Each API server (kube-apiserver, openshift-apiserver, oauth-apiserver) mounts the same hostPath directory and accesses the Unix socket directly.

**Example:**
```yaml
# User deploys:
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: vault-kms-plugin
spec:
  template:
    spec:
      nodeSelector:
        node-role.kubernetes.io/master: ""
      volumes:
      - name: socket
        hostPath:
          path: /var/run/kms-plugins/vault
      containers:
      - name: vault
        volumeMounts:
        - name: socket
          mountPath: /socket
```

**Why this doesn't work:**

**SELinux MCS (Multi-Category Security) Isolation Problem:**

OpenShift uses SELinux MCS labels to provide pod-to-pod isolation. Each pod gets a unique MCS label (e.g., `s0:c111,c222`). When a container creates a file (including Unix sockets) via hostPath, the file inherits the container's MCS label:

```bash
# On host filesystem:
$ ls -lZ /var/run/kms-plugins/vault/kms.sock
srwxrwxrwx. root root system_u:object_r:container_file_t:s0:c111,c222 kms.sock
                                                         ^^^^^^^^^^^^
                                                         DaemonSet pod's MCS label
```

When a different pod (e.g., openshift-apiserver with MCS label `s0:c333,c444`) tries to access this socket:

```bash
$ ls /var/run/kms-plugins/vault/
ls: cannot access 'kms.sock': Permission denied

# SELinux blocks because:
# - Accessing process MCS: s0:c333,c444
# - Socket file MCS:        s0:c111,c222
# - Labels don't match → DENIED
```

**This works for kube-apiserver only** because it runs as `spc_t` (super privileged container) which bypasses MCS enforcement. **It does NOT work for openshift-apiserver or oauth-apiserver** which run as normal containers with MCS enforcement.

**Additional problems:**
- Users must calculate socket paths matching OpenShift's hash algorithm
- Requires understanding of SELinux, MCS labels, and OpenShift internals
- No way to support non-privileged API servers without disabling SELinux (unacceptable)

**References:**
- [Understanding SELinux labels for container runtimes](https://opensource.com/article/18/2/selinux-labels-container-runtimes)
- [Configure a Security Context for a Pod or Container (Kubernetes)](https://kubernetes.io/docs/tasks/configure-pod-container/security-context/)
- Internal blog post: `feature-overview.txt` in this repository

### Alternative 3: No Automatic Rotation

**Approach:** Remove automatic rotation detection, make users manually trigger migration.

**Why not chosen:**
- Defeats purpose of OpenShift automation
- Users must monitor external KMS for rotations
- Loses significant value proposition
- Upstream Kubernetes already has manual approach, OpenShift should add value

### Alternative 4: Multiple Shim Instances During KEK Change

**Approach:** Deploy two separate shim instances during KEK change, each with its own socket and endpoint.

**Why not chosen:**
- Requires two socket paths in EncryptionConfiguration
- Operators must manage multiple shim sidecars and coordinate removal
- More complex than single shim with try-fallback routing
- User experience differs between KEK rotation (one shim) and KEK change (two shims)

**Note:** This was the original design before we realized a single shim instance can simply try the primary endpoint first and fall back to additional endpoints on failure, without needing to inspect or match `key_id` values.

## Open Questions

1. **Authentication between shim and external plugin:**
   - Tech Preview: No authentication (trust in-cluster network)
   - GA: Mutual TLS? ServiceAccount tokens? Shared secret?
   - Decision deferred to GA planning based on Tech Preview feedback

2. **Shim image distribution:**
   - Tech Preview: Users may need to manually specify shim image
   - GA: Include shim in OpenShift release payload?
   - Decision: Start with manual, automate for GA

3. **Endpoint discovery:**
   - Current: User provides explicit endpoint URL
   - Alternative: Convention-based (e.g., always look for service named `kms-plugin` in specific namespace)
   - Decision: Explicit endpoint for flexibility, consider convention for GA

## Test Plan

### Unit Tests

**Shim Component:**
- Shim forwarding logic (Unix socket → HTTP/gRPC)
- Intelligent routing logic:
  - Encrypt requests always use primary endpoint
  - Decrypt requests try primary endpoint first, fall back to additionalEndpoints
  - Status requests use primary endpoint
- Error handling (connection failures, endpoint unreachable)
- Configuration parsing and validation (endpoint URL format, additionalEndpoints list)
- Metrics collection (per-endpoint counters)

**Socket Proxy Component:**
- Socket proxy translation logic (HTTP/gRPC → Unix socket)
- Connection handling to plugin Unix socket
- Error handling (plugin socket unavailable, plugin errors)
- Health check endpoint functionality
- Metrics collection (translation counters, plugin connection status)

**Operator Validation Logic:**
- Endpoint reachability validation (health check + Status call)
- Validation error handling and operator condition updates
- Shim configuration with validated endpoints
- Configuration updates when endpoints change

### Integration Tests

**Shim and Socket Proxy Integration:**
- Shim deployed as sidecar in API server pod
- Socket proxy deployed by test alongside mock plugin
- End-to-end encryption/decryption through shim → socket proxy → plugin
- KEK rotation detection forwarded correctly through the chain
- Intelligent routing during KEK change (single shim, multiple endpoints)
- Configuration updates applied without pod restart

**Operator Integration:**
- Operator validates endpoint before injecting shim
- Validation failures set correct operator conditions
- Shim configured with correct endpoint URL
- Multiple plugin deployments (KEK change scenario with multiple endpoints)
- Endpoint validation retries on failure

### E2E Tests

**Full Stack Testing:**
- User deploys standard upstream KMS plugin (e.g., Vault in-cluster) using example YAML
- User deploys OpenShift-provided socket proxy alongside plugin (sidecar pattern)
- User creates Service to expose socket proxy endpoint
- User configures APIServer CR with endpoint URL
- Operators validate endpoint reachability
- Operators inject shim into API server pods with validated endpoint
- Verify data encrypted end-to-end
- Trigger KEK rotation, verify migration
- Perform KEK change with `additionalEndpoints`, verify smart routing
- Verify old plugin removal after migration completes
- Measure performance impact (latency, throughput with two network hops)

**User Experience Testing:**
- Deploy plugin without modifications (verify upstream compatibility)
- Update plugin image independently (verify plugin update without OpenShift changes)
- Deploy socket proxy in different patterns (sidecar, separate pod, external)
- Verify endpoint validation works regardless of deployment architecture

### Failure Injection Tests

**Network and Endpoint Failures:**
- Plugin endpoint unavailable (all pods down)
- Network policy blocking shim → endpoint
- DNS failure (endpoint hostname unresolvable)
- Socket proxy container crash (verify restart and recovery)
- Plugin pod deleted (verify endpoint routing failure, recovery after recreation)

**Endpoint Validation Failures:**
- Endpoint URL malformed (invalid format, port out of range)
- Endpoint unreachable (connection refused, timeout)
- Endpoint health check fails (socket proxy not responding)
- Endpoint KMS Status call fails (plugin not responding via socket proxy)
- Multiple endpoints configured, some unreachable (verify graceful degradation)

**User Deployment Errors:**
- Socket proxy deployed with wrong socket path (doesn't match plugin)
- Socket proxy port mismatch (Service port vs container port)
- Plugin and socket proxy in different pods without shared volume
- External endpoint with invalid TLS certificate
- Endpoint URL points to wrong service

**Component Failures:**
- Shim container crash and recovery
- Socket proxy loses connection to plugin socket
- Plugin returns errors (verify forwarding through proxy → shim → API server)

## Graduation Criteria

### Tech Preview Acceptance Criteria

**Core Architecture:**
- ✅ Shim implementation complete (Unix socket ↔ HTTP/gRPC forwarding with intelligent routing)
- ✅ Socket proxy implementation complete (HTTP/gRPC ↔ Unix socket translation)
- ✅ Operator integration (shim sidecar injection into API server pods)
- ✅ Endpoint validation (health check + KMS Status call)
- ✅ Operator conditions reporting endpoint validation status

**Key Rotation:**
- ✅ Basic KEK rotation working (forwarding Status calls through socket proxy)
- ✅ Smart routing during KEK change (single shim, multiple endpoints via `additionalEndpoints`)
- ✅ Configuration hot-reload (update endpoints without pod restart)

**Documentation and Feature Gate:**
- ✅ Documentation with complete example deployment YAMLs (plugin + socket proxy + Service)
- ✅ Troubleshooting guide for shim, socket proxy, and plugin issues
- ✅ Example YAMLs for common deployment patterns (in-cluster sidecar, external)
- ✅ Behind `KMSEncryptionProvider` feature gate (disabled by default)

**Compatibility:**
- ✅ Works with standard upstream KMS v2 plugins (unmodified)
- ✅ Users can update plugin images independently
- ✅ Socket proxy deployment flexible (sidecar, separate pod, external)

### Tech Preview → GA

**Production Validation:**
- ✅ Production validation with at least 2 external KMS providers (Vault, AWS plugin)
- ✅ Validation that standard upstream plugins work unmodified
- ✅ Validation of plugin updates without OpenShift changes
- ✅ Performance benchmarks and SLO definition (two-hop latency acceptable)

**Operational Readiness:**
- ✅ Authentication between shim and socket proxy (if required for security)
- ✅ Comprehensive troubleshooting documentation (three-layer architecture)
- ✅ Support runbooks for common failure scenarios:
  - Endpoint validation failures
  - User deployment misconfigurations
  - Plugin socket connection issues
- ✅ Operator conditions for endpoint validation status
- ✅ Metrics and alerts for shim and socket proxy health

**Infrastructure:**
- ✅ Shim image included in OpenShift release payload
- ✅ Socket proxy image included in OpenShift release payload
- ✅ Example deployment YAMLs published for common KMS providers

**User Feedback:**
- ✅ 6+ months of Tech Preview feedback incorporated
- ✅ User validation that manual socket proxy deployment provides needed flexibility
- ✅ User validation of endpoint URL configuration approach

## Upgrade / Downgrade Strategy

### Upgrade

**From version without shim to version with shim:**
- No user action required if not using KMS encryption
- If using native plugins (Enhancement B): Continue working, shim not deployed
- If user wants to switch to external plugins: Update APIServer config, deploy external plugin, operators deploy shim

**During upgrade:**
- Shim image updated with operator upgrade
- Existing shim sidecars restarted with new image
- No encryption downtime (rolling update)

### Downgrade

**From version with shim to version without shim:**
- If KMS encryption enabled with external plugins: **Cannot downgrade without migration**
- User must first migrate to native plugins or disable encryption
- Migration requires updating APIServer config and waiting for data re-encryption

**Procedure:**
1. Update APIServer config to use native plugin (type: AWS/Vault) or disable encryption (type: identity)
2. Wait for migration to complete
3. Downgrade OpenShift version
4. Shim code inactive (not deployed)

## Version Skew Strategy

### Operator Version Skew

During cluster upgrade, operators may be at different versions:
- kube-apiserver-operator upgraded, injects new shim version
- openshift-apiserver-operator still on old version, injects old shim or no shim

**Impact:** Some API servers have new shim, others have old shim or native plugins

**Mitigation:**
- Shim API is simple (KMS v2 forwarding), backward compatible
- Old and new shim versions can coexist
- External plugin interface unchanged (KMS v2 API stable)

### Shim vs External Plugin Skew

Shim updated but external plugin unchanged (or vice versa).

**Impact:** Minimal - KMS v2 API is stable

**Mitigation:**
- Shim forwards KMS v2 calls unchanged
- Plugin implements KMS v2 API (stable interface)
- No version coordination required

### During KEK Change

Single shim instance routing to multiple endpoints (primary endpoint + additionalEndpoints).

**Impact:** No version skew issue - same shim version handling all endpoints

**Behavior:** Smart routing within single instance, no special coordination needed

## Operational Aspects of API Extensions

This enhancement extends the `config.openshift.io/v1/APIServer` resource but does not introduce new CRDs, webhooks, or aggregated API servers.

### Service Level Indicators (SLIs)

**Shim Health:**
- Operator condition: `KMSShimDegraded=False`
- Shim container health checks (liveness, readiness)
- Metrics: `kms_shim_healthy{endpoint="..."}`

**Performance:**
- Metrics: `kms_shim_request_duration_seconds`
- Alert: `KMSShimLatencyHigh` if p99 > 100ms

### Impact on Existing SLIs

**API Availability:**
- KMS encryption adds latency to resource creation/updates
- With shim: Additional network hop (~1-5ms)
- Total expected impact: +10-50ms per encrypted resource operation

**API Throughput:**
- Shim adds minimal overhead (simple forwarding)
- Expected: <5% throughput reduction compared to native plugins

## Support Procedures

### Three-Layer Troubleshooting Model

KMS encryption uses a three-layer architecture. Troubleshooting follows this order:

1. **Shim Layer** (in API server pods) - Red Hat responsibility
2. **Socket Proxy Layer** (in user-deployed plugin pods) - Red Hat image responsibility, user deployment responsibility
3. **Plugin Layer** (in user-deployed plugin pods) - User responsibility

### Detecting Shim Issues

**Symptoms:**
- Encryption/decryption operations failing
- API server logs: "failed to call KMS plugin"
- Secrets cannot be created or read

**Diagnosis:**
```bash
# Check shim container health
oc get pods -n openshift-kube-apiserver -l app=kube-apiserver
oc logs -n openshift-kube-apiserver <pod> -c kms-shim

# Check shim metrics
oc exec -n openshift-kube-apiserver <pod> -c kms-shim -- \
  curl localhost:8080/metrics | grep kms_shim

# Check shim can reach plugin endpoint
oc exec -n openshift-kube-apiserver <pod> -c kms-shim -- \
  curl http://vault-kms-plugin.kms-plugins.svc:8080/healthz
```

**Resolution:**
- If shim cannot reach endpoint: Check network policy, DNS, endpoint configuration
- If endpoint returns errors: Proceed to socket proxy troubleshooting
- If shim crashing: Check shim logs, escalate to Red Hat if shim bug

### Detecting Socket Proxy Issues

**Symptoms:**
- Shim successfully connects to endpoint but gets errors
- Plugin pod running but encryption fails
- Socket proxy metrics show connection failures

**Diagnosis:**
```bash
# Check if socket proxy container is present in user deployment
oc get pod -n kms-plugins <plugin-pod> -o jsonpath='{.spec.containers[*].name}'
# Should show: vault-kms  socket-proxy

# Check socket proxy container health
oc logs -n kms-plugins <plugin-pod> -c socket-proxy

# Check socket proxy metrics
oc exec -n kms-plugins <plugin-pod> -c socket-proxy -- \
  curl localhost:8080/metrics | grep socket_proxy

# Check if Service was created by user
oc get svc -n kms-plugins vault-kms-plugin
```

**Resolution:**
- If socket proxy not present: User needs to add socket proxy container to their deployment (see example YAMLs)
- If socket proxy cannot reach plugin socket: Check plugin container logs, verify socket path matches in both containers
- If socket proxy crashing: Check logs, escalate to Red Hat if proxy image bug
- If Service missing: User needs to create Service (see example YAMLs)
- If port mismatch: Verify Service port matches socket proxy container port

### Detecting Plugin Issues

**Symptoms:**
- Socket proxy successfully connects to plugin socket but gets errors
- Plugin container logs show errors
- Plugin-specific functionality broken

**Diagnosis:**
```bash
# Check plugin container logs
oc logs -n kms-plugins <plugin-pod> -c vault-kms

# Check plugin socket exists
oc exec -n kms-plugins <plugin-pod> -c socket-proxy -- ls -la /socket/

# Test plugin directly (from socket proxy container)
oc exec -n kms-plugins <plugin-pod> -c socket-proxy -- \
  grpcurl -unix /socket/kms.sock kmsv2.KeyManagementService/Status
```

**Resolution:**
- Plugin errors are user responsibility (outside Red Hat support scope)
- Verify plugin configuration (Vault address, AWS credentials, etc.)
- Check plugin documentation for troubleshooting
- Contact plugin vendor for plugin-specific issues

### Detecting Endpoint Validation Failures

**Symptoms:**
- User configured endpoint in APIServer CR but encryption doesn't work
- Operator condition shows `KMSPluginAvailable=False`
- Shim not injected into API server pods

**Diagnosis:**
```bash
# Check operator conditions for validation status
oc get clusteroperator kube-apiserver -o jsonpath='{.status.conditions[?(@.type=="KMSPluginAvailable")]}'

# Check operator logs for validation errors
oc logs -n openshift-kube-apiserver-operator deployment/kube-apiserver-operator | \
  grep -i "kms.*validation"

# Manually test endpoint reachability from control plane
oc debug node/<master-node> -- curl http://vault-kms-plugin.kms-plugins.svc:8080/healthz

# Check APIServer configuration
oc get apiserver cluster -o jsonpath='{.spec.encryption.kms.external.endpoint}'
```

**Resolution:**
- If endpoint unreachable: Verify user deployed plugin + socket proxy + Service
- If health check fails: Check socket proxy logs, verify it's listening on correct port
- If KMS Status call fails: Check plugin logs, verify socket path is correct
- If endpoint URL wrong: Update APIServer CR with correct endpoint
- If network policy blocking: Update network policy to allow API server → endpoint traffic

### Support Boundary

**Red Hat supports:**
- ✅ Shim deployment and lifecycle
- ✅ Shim forwarding logic and intelligent routing
- ✅ Socket proxy **image** (not deployment)
- ✅ Socket proxy translation logic (HTTP/gRPC ↔ Unix socket)
- ✅ Endpoint validation (health check + KMS Status call)
- ✅ Socket path generation in API server pods
- ✅ Connectivity troubleshooting (shim → endpoint → socket proxy)
- ✅ Metrics and monitoring (shim + socket proxy images)

**User responsible for:**
- ❌ Plugin deployment (creating Deployment, pod, or external infrastructure)
- ❌ Socket proxy deployment (adding socket proxy container to their deployment)
- ❌ Service/networking creation (exposing socket proxy endpoint)
- ❌ Plugin configuration (Vault address, AWS credentials, key IDs)
- ❌ Plugin bugs or errors
- ❌ KMS provider configuration and credentials
- ❌ Plugin performance or reliability
- ❌ Plugin updates and version management

**Example Escalation:**
- **Issue:** "KMS encryption not working"
- **Red Hat checks:**
  1. Is shim reaching endpoint? → Yes
  2. Is socket proxy container present in user deployment? → Yes
  3. Is socket proxy reaching plugin socket? → Yes
  4. Is plugin returning errors? → Yes: Plugin error X
- **Red Hat response:** "Shim and socket proxy image are working correctly. Plugin is returning error X. This is a plugin configuration or bug issue. Please check your plugin configuration or contact plugin vendor. Verify socket path matches between plugin and socket proxy containers."

## Infrastructure Needed

**For Development:**
- Shim container image repository
- Socket proxy container image repository
- CI infrastructure to build and test shim
- CI infrastructure to build and test socket proxy
- Example deployment YAML repository (for common KMS providers)

**For Testing:**
- Mock KMS plugin for integration tests (standard KMS v2 plugin)
- Real KMS plugin instances (Vault, AWS KMS) for E2E tests
- Test environment with endpoint validation enabled
- Performance testing environment (measure two-hop latency)
- Test clusters with various network policies (validate endpoint connectivity)
- Test different deployment patterns (in-cluster sidecar, separate pods, external)
