---
title: zero-trust-workload-identity-manager
authors:
  - "@anirudhAgniRedhat"
reviewers:
  - "@tgeer"
approvers:
  - "@tgeer"
api-approvers:
  - "@tgeer"
creation-date: 2025-04-3
last-updated: 2025-04-3
tracking-link:
  - https://issues.redhat.com/browse/SPIRE-2
---

# Zero Trust Workload Identity Manager

## Summary

This enhancement describes the proposal to introduce a `Zero-Trust-Workload-Identity-Manager` for OpenShift, leveraging `SPIFFE` and `SPIRE` to provide a comprehensive identity management solution for distributed systems. SPIFFE/SPIRE provides a standardized approach to workload identity, enabling secure service-to-service communication in dynamic and heterogeneous environments. OpenShift will extend its security capabilities by integrating SPIFFE/SPIRE through a dedicated operator that manages workload identities across the cluster.


The solution introduces a identity management framework designed to address the complex security challenges of modern distributed systems. Key capabilities include implementing SPIFFE identity standards, automating SPIRE agent and server deployment, providing dynamic workload identity provisioning, ensuring verifiable identities for workloads, and supporting automatic certificate rotation and management.



## Motivation
Customers deploying OpenShift require a zero-trust workload identity solution that goes beyond static credentials and perimeter-based security models. Traditional approaches, such as long-lived certificates or manual secret injection, struggle to secure dynamic microservices architectures where workloads scale, migrate, and communicate across clusters. These methods introduce operational complexity, increase the risk of credential leakage, and fail to provide granular, cryptographically verifiable authentication for ephemeral workloads.  

The `Zero-Trust Workload Identity Manager` addresses these challenges by integrating `SPIFFE/SPIRE` natively into OpenShift. This provides automated issuance of short-lived, cryptographically signed identities (SVIDs) to workloads, replacing error-prone manual processes with dynamic identity lifecycle management. By enforcing mutual TLS (mTLS) by default and enabling fine-grained trust boundaries, organizations can secure service-to-service communication in multi-tenant or hybrid environments while reducing operational overhead and aligning with zero-trust security mandates.

### User Stories

- As a openshift cluster administrator, I want to deploy SPIRE components via an operator so that workload identities are managed automatically across the cluster.
- As a openshift cluster administrator, I want to configure trust domains via CRDs so that I can enforce secure isolation between teams or environments.
- As an application developer, I want SPIFFE identities injected into my workloads via the CSI Driver so that I avoid manual certificate management.
- As a security engineer, I want workloads to use short-lived, auto-rotated SVIDs so that mTLS is enforced by default for zero-trust security.
- As a cluster administrator, I want SPIRE metrics in OpenShift Monitoring so that I can proactively resolve identity-related issues.


### Goals

- Automate SPIRE server/agent deployment and lifecycle management via an operator.
- Dynamically provision SPIFFE identities (SVIDs) to workloads using the CSI Driver.
- Enable trust domain configuration through CRDs for secure multi-tenant/multi-cluster isolation.
- Ensure SVIDs have a TTL with automatic rotation to enforce credentials freshness.
- Integrate SPIRE health and performance metrics into OpenShift Monitoring.
- Maintain high availability for SPIRE servers and resilience for agents during failures.  

### Non-Goals  
- Replace existing auth systems (e.g., Service Accounts, OAuth).  
- Manage non-workload identities (users).  
- Enforce network-level security policies.  
- Support non-SPIFFE systems or legacy PKI.  
- Establish cross-cluster trust without explicit configuration.  
- Provide dedicated UI/CLI tools in initial release.

## Proposal
A new zero-trust-workload-identity-manager operator will manage the deployment and lifecycle of SPIRE components (server, agents) and the SPIFFE CSI Driver to provide workload identities based on the SPIFFE standard. A singleton TrustDomain Custom Resource (CR) will configure trust boundaries, while WorkloadAttestation CRs define attestation policies for workloads.

### Key Components
- Operator-Managed SPIRE Components
- `SPIRE Server` as a `StatefulSet` for high availability.
- `SPIRE Agents` as a `DaemonSet` (one per node).
- `SPIFFE CSI Driver` as a DaemonSet to inject workload identities (`SVIDs`) into pods.
- Resources `(RBAC, ServiceAccount, ClusterRole, etc.)` are created from static manifest templates.

The operator will create and manage the following resources to deploy SPIRE and SPIFFE components, Please refer `Implementation Details/Notes/Constraints` section for more details:

1. Core Infrastructure
    - Namespaces:
        - `spire-system` (management plane)
        - `spire-server` (SPIRE server components)
        - `zero-trust-workload-identity-manager` (shared resources)

    - ServiceAccounts:
        - `spire-agent`
        - `spire-server`
        - `spire-spiffe-csi-driver`
        - `spire-spiffe-oidc-discovery-provider`

2. Configuration
    - ConfigMaps:

        - `spire-agent`, `spire-server` (component configurations)
        - `spire-spiffe-oidc-discovery-provider` (OIDC provider settings)
        - `spire-bundle` (trust bundle for certificates)

3. RBAC & Security
    - ClusterRoles:
        - `spire-agent` (read pods/nodes)
        - `spire-controller-manager` (manage SPIRE CRDs)
        - `spire-server` (token reviews, node/pod access)

    - ClusterRoleBindings:
        - Bind roles to SPIRE ServiceAccounts (e.g., `spire-agent`, `spire-server`).

    - SecurityContextConstraints (SCCs):
        - `spire-spiffe-csi-driver` (privileged CSI driver)
        - `spire-agent` (host access for agents)

4. SPIRE Server
    - StatefulSet:
        - `spire-server` (with persistent storage via spire-data PVC).

    - Services:
        - `spire-server` (gRPC endpoint for agent communication).

    - Volume Claims:
        - `spire-data` (1Gi persistent volume for server state).

5. SPIRE Agents
    - DaemonSet:
        - `spire-agent` (deployed per node for workload attestation).

    - Host Access:
        - Runs with `hostPID:true` and `hostNetwork:true` for node-level visibility.

6. SPIFFE CSI Driver
    - DaemonSet:

        - `spire-spiffe-csi-driver` (injects SVIDs into pods via CSI volumes).

    - CSIDriver:
        - `csi.spiffe.io` (supports ephemeral volumes for workload identities).

7. Networking
    - Ingresses:
        - Exposes `OIDC` discovery endpoint (oidc-discovery.apps...).

    - Services:
        - `spire-spiffe-oidc-discovery-provider` (OIDC HTTP service).

Each of the resource created for `zero-trust-workload-identity-manager` will have below set of labels added.
* `app: zero-trust-workload-identity-manager`
* `app.kubernetes.io/name: zero-trust-workload-identity-manager`
* `app.kubernetes.io/instance: zero-trust-workload-identity-manager`
* `app.kubernetes.io/version: "v0.1.0"`
* `app.kubernetes.io/managed-by: zero-trust-workload-identity-manager`
* `app.kubernetes.io/part-of: zero-trust-workload-identity-manager`

Refer below links for more information on the labels used
- [Guidelines for Labels and Annotations for OpenShift applications](https://github.com/redhat-developer/app-labels/blob/master/labels-annotation-for-openshift.adoc)
- [Well-Known Labels, Annotations and Taints](https://kubernetes.io/docs/reference/labels-annotations-taints/)

### Workflow Description

#### 1. **Operator Installation**  
- **Action**: Install the operator via **OpenShift OperatorHub** or CLI.  
- **Result**: The operator deploys:  
  - **SPIRE Server** (`StatefulSet`) as the certificate authority (CA).  
  - **SPIRE Agents** (`DaemonSet`) for per-node workload attestation.  
  - **SPIFFE CSI Driver** (`DaemonSet`) to inject SVIDs into pods.  
  - **SPIFFE OIDC Provider** (`Deployment`) for OIDC token issuance.  

#### 2. **Workload Deployment**  
- **Action**: Deploy a pod with labels matching a `ClusterSPIFFEID` policy (e.g., `app: secure`).  
- **Result**:  
  - The SPIRE Agent on the node detects the pod and validates:  
    1. Pod labels/annotations.  
    2. Service account and namespace.  
    3. Node identity (e.g., Kubernetes node UID).  

#### 3. **SVID Issuance**  
- **Action**: The SPIRE Server processes the attestation request.  
- **Result**:  
  - Generates a short-lived **SVID** (X.509/JWT) with a default 1-hour TTL.  
  - Signs SVIDs using its CA or delegates to an external enterprise PKI.  

#### 4. **SVID Injection**  
- **Action**: The SPIFFE CSI Driver mounts the SVID into the pod.  
- **Result**:  
  - SVIDs are stored at `/var/run/secrets/spiffe` as read-only volumes.  
  - Auto-rotated 10 minutes before expiration.  

#### 5. **Secure Communication**  
- **Action**: Workloads authenticate using SVIDs.  
- **Result**:  
  - **Mutual TLS (mTLS)**: Services validate each other’s SVIDs during TLS handshakes.  
  - **OIDC Integration**: Workloads fetch tokens for external systems (e.g., Kubernetes API).  

#### 6. **Advanced Scenarios**  
- **Federated Trust**:  
  - Define cross-cluster trust via `ClusterFederatedTrustDomains` CR.  
  - Example: Trust workloads from `cluster-a.example` in `cluster-b.example`.  
- **Static Identities**:  
  - Use `ClusterStaticEntries` to pre-register identities for system components (e.g., `kube-apiserver`).  

---

#### Visual Workflow  

```mermaid
flowchart TD
    %% User Configuration
    subgraph User Config
        A1[Admin deploys zero-trust-workload-identity-manager Operator]
        A2[Admin creates ZeroTrustWorkloadIdentityManager CR]
        A3[Operator watches ZeroTrustWorkloadIdentityManager CR via controller-runtime]
    end

    %% SPIFFE/SPIRE Infrastructure Setup
    subgraph SPIFFE/SPIRE Infra Setup
        B1[Operator deploys spire-server StatefulSet config from ZeroTrustWorkloadIdentityManager CR]
        B2[Operator deploys spire-agent DaemonSet on all nodes]
        B3[Operator deploys spiffe-csi-driver DaemonSet]
        B4[Operator optionally deploys spiffe-oidc-provider Deployment]
    end

    %% Health Management & Status Updates
    subgraph Health Management
        B5[Operator performs health checks on all Spire components]
        B6[Operator updates ZeroTrustWorkloadIdentityManager.status with component conditions]
    end

    %% Workload Identity Provisioning
    subgraph Workload Identity Provisioning
        C1[User deploys workload with annotated ServiceAccount]
        C2[Kubelet schedules Pod]
        C3[spiffe-csi-driver NodePublishVolume invoked]
        C4[spiffe-csi-driver contacts local spire-agent]
        C5[spire-agent fetches SVID from spire-server Workload API]
        C6[CSI driver writes SVID cert/key to workload volume]
    end

    %% SPIRE Registration Management
    subgraph SPIRE Registration
        D1[Operator monitors annotated ServiceAccounts & Namespaces]
        D2[Operator derives SPIFFE ID for workload]
        D3[Operator creates/updates SPIRE RegistrationEntry spire-server gRPC]
    end

    %% SVID Consumption (mTLS)
    subgraph SVID Consumption mTLS
        E1[Workload mounts SVID volume]
        E2[Workload initiates mTLS connection using SVID]
        E3[Peer workload validates SPIFFE ID using local spire-agent cache]
    end

    %% Optional OIDC Integration
    subgraph Optional OIDC Integration
        F1[spiffe-oidc-provider exposes JWKS endpoint]
        F2[Workload requests JWT-SVID from local spire-agent]
        F3[Workload uses JWT-SVID to authenticate to external services]
        F4[External service validates JWT-SVID using JWKS]
    end

    %% Connections
    A1 --> A2
    A2 --> A3
    A3 --> B1
    A3 --> D1

    B1 --> B2
    B2 --> B3
    B2 -- Optional --> B4

    B1 --> B5
    B2 --> B5
    B3 --> B5
    B4 --> B5
    B5 --> B6

    C1 --> C2
    C2 --> C3
    C3 --> C4
    C4 --> C5
    C5 --> C6
    C6 --> E1

    D1 --> D2
    D2 --> D3
    D3 --> B1

    E1 --> E2
    E2 --> E3

    C5 -- JWT-SVID Request --> F2
    B4 --> F1
    F2 --> F3
    F3 --> F4
  ```

### API Extensions

```golang
type WorkloadIdentityManager struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the specification of the desired behavior of the WorkloadIdentityManager.
	// +kubebuilder:validation:Required
	Spec WorkloadIdentityManagerSpec `json:"spec,omitempty"`

	// status is the most recently observed status of the WorkloadIdentityManager.
	Status WorkloadIdentityManagerStatus `json:"status,omitempty"`
}

type WorkloadIdentityManagerSpec struct {

	// workloadIdentityManagerConfig is for configuring the workload identity manager operands behavior.
	// +kubebuilder:validation:Required
	WorkloadIdentityManagerConfig *WorkloadIdentityManagerConfig `json:"workloadIdentityManagerConfig,omitempty"`

	// controllerConfig is for configuring the controller for setting up
	// defaults to enable spire.
	// +kubebuilder:validation:Optional
	ControllerConfig *OperatorControllerConfig `json:"controllerConfig,omitempty"`
}

// OperatorControllerConfig is for configuring the operator for setting up
// defaults to install external-secrets.
type OperatorControllerConfig struct {
	// namespace is for configuring the namespace to install the spire operand.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:="workload-identity-manager"
	Namespace string `json:"namespace,omitempty"`

	// labels to apply to all resources created for operands deployment.
	// +mapType=granular
	// +kubebuilder:validation:Optional
	Labels map[string]string `json:"labels,omitempty"`
}

// WorkloadIdentityManagerConfig is for configuring the external-secrets behavior.
type WorkloadIdentityManagerConfig struct {
	// logLevel supports value range as per [kubernetes logging guidelines](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-instrumentation/logging.md#what-method-to-use).
	// +kubebuilder:default:=1
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Maximum:=5
	// +kubebuilder:validation:Optional
	LogLevel int32 `json:"logLevel,omitempty"`

	// operatingNamespace is for restricting the external-secrets operations to provided namespace.
	// And when enabled `ClusterSecretStore` and `ClusterExternalSecret` are implicitly disabled.
	// +kubebuilder:validation:Optional
	OperatingNamespace string `json:"operatingNamespace,omitempty"`

	// trustDomain to be used for the SPIFFE identifiers
	TrustDomain string `json:"trustDomain,omitempty"`

	// bundleConfigMap is Configmap name for Spire bundle
	// +kubebuilder:default:=spire-bundle
	BundleConfigMap string `json:"bundleConfigMap"`

	// spiffeOIDCProviderConfig has config for OIDC provider
	// +kubebuilder:validation:Optional
	SpiffeOIDCProviderConfig *SpiffeOIDCProviderConfig `json:"spiffeOIDCProviderConfigMap,omitempty"`

	// spireAgentConfig has config for spire agents.
	// +kubebuilder:validation:Optional
	SpireAgentConfig *SpireAgentConfig `json:"spireAgentConfig,omitempty"`

	// spireServerConfig has config for spire server.
	// +kubebuilder:validation:Optional
	SpireServerConfig *SpireServerConfig `json:"spireServerConfig,omitempty"`

	// spiffeCSIDriverConfig has config for spiffe csi driver.
	// +kubebuilder:validation:Optional
	SpiffeCSIDriverConfig *SpiffeCSIDriverConfig `json:"spiffeCSIDriverConfig,omitempty"`

	// resources are for defining the resource requirements.
	// Cannot be updated.
	// ref: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +kubebuilder:validation:Optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// affinity is for setting scheduling affinity rules.
	// ref: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/
	// +kubebuilder:validation:Optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// tolerations are for setting the pod tolerations.
	// ref: https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/
	// +kubebuilder:validation:Optional
	// +listType=atomic
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// nodeSelector is for defining the scheduling criteria using node labels.
	// ref: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +kubebuilder:validation:Optional
	// +mapType=atomic
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

// SpiffeCSIDriverConfig defines the configuration for the Spiffe CSI Driver.
// +kubebuilder:validation:Optional
type SpiffeCSIDriverConfig struct {
	// Enabled specifies whether the Spiffe CSI Driver is enabled.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=true
	Enabled bool `json:"enabled,omitempty"`

	// AgentSocket is the path to the agent socket.
	// +kubebuilder:validation:Optional
	AgentSocket string `json:"agentSocketPath,omitempty"`

	// PluginName defines the name of the CSI plugin.
	// +kubebuilder:validation:Optional
	PluginName string `json:"pluginName,omitempty"`
}

// SpiffeOIDCProviderConfig defines the configuration for the Spiffe OIDC Provider.
// +kubebuilder:validation:Optional
type SpiffeOIDCProviderConfig struct {
	// Enabled specifies whether the OIDC provider is enabled.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=true
	Enabled bool `json:"enabled,omitempty"`

	// AgentSocket is the name of the agent socket.
	// +kubebuilder:validation:Optional
	AgentSocket string `json:"agentSocketName,omitempty"`

	// ReplicaCount is the number of replicas for the OIDC provider.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=1
	ReplicaCount int `json:"replicaCount,omitempty"`

	// JwtIssuer specifies the JWT issuer.
	// +kubebuilder:validation:Optional
	JwtIssuer string `json:"jwtIssuer,omitempty"`

	// TLS contains TLS configuration settings.
	// +kubebuilder:validation:Optional
	TLS *TLSConfig `json:"tls,omitempty"`
}

// SpireAgentConfig defines the configuration for the Spire Agent.
// +kubebuilder:validation:Optional
type SpireAgentConfig struct {
	// Enabled specifies whether the Spire Agent is enabled.
	// +kubebuilder:validation:Optional
	Enabled bool `json:"enabled,omitempty"`
}

// SpireServerConfig defines the configuration for the Spire Server.
// +kubebuilder:validation:Optional
type SpireServerConfig struct {
	// Enabled specifies whether the Spire Server is enabled.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=true
	Enabled bool `json:"enabled,omitempty"`

	// Federation contains federation configuration settings.
	// +kubebuilder:validation:Optional
	Federation *Federation `json:"federation,omitempty"`

	// CASubject contains subject information for the Spire CA.
	// +kubebuilder:validation:Optional
	CASubject *CASubject `json:"caSubject,omitempty"`
}

// Federation defines the federation configuration for Spire.
// +kubebuilder:validation:Optional
type Federation struct {
	// Enabled specifies whether federation is enabled.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=true
	Enabled bool `json:"enabled,omitempty"`

	// TLS contains TLS configuration settings for federation.
	// +kubebuilder:validation:Optional
	TLS *TLSConfig `json:"tls,omitempty"`

	// Ingress defines ingress settings for Spire federation.
	// +kubebuilder:validation:Optional
	Ingress *SpireIngress `json:"ingress,omitempty"`
}

// SpireIngress defines the ingress configuration for Spire federation.
// +kubebuilder:validation:Optional
type SpireIngress struct {
	// Enabled specifies whether ingress is enabled.
	// +kubebuilder:validation:Optional
	// +kubebuilder:default:=false
	Enabled bool `json:"enabled,omitempty"`

	// Host specifies the ingress host.
	// +kubebuilder:validation:Optional
	Host string `json:"host,omitempty"`

	// TLSSecret specifies the name of the TLS secret.
	// +kubebuilder:validation:Optional
	TlSSecret string `json:"tlsSecret,omitempty"`
}

// CASubject defines the subject information for the Spire CA.
// +kubebuilder:validation:Optional
type CASubject struct {
	// Country specifies the country for the CA.
	// +kubebuilder:validation:Optional
	Country string `json:"country,omitempty"`

	// Organization specifies the organization for the CA.
	// +kubebuilder:validation:Optional
	Organization string `json:"organization,omitempty"`

	// CommonName specifies the common name for the CA.
	// +kubebuilder:validation:Optional
	CommonName string `json:"commonName,omitempty"`
}

// TLSConfig defines the TLS configuration options.
// +kubebuilder:validation:Optional
type TLSConfig struct {
	Spire struct {
		// Enabled specifies whether Spire-managed TLS is enabled.
		// +kubebuilder:validation:Optional
		Enabled bool `json:"enabled,omitempty"`
	} `json:"spire,omitempty"`

	ExternalSecret struct {
		// Enabled specifies whether external secrets are enabled.
		// +kubebuilder:validation:Optional
		// +kubebuilder:default:=false
		Enabled bool `json:"enabled,omitempty"`

		// SecretName specifies the name of the external secret.
		// +kubebuilder:validation:Optional
		SecretName string `json:"secretName,omitempty"`
	} `json:"externalSecret,omitempty"`

	CertManager struct {
		// Enabled specifies whether Cert Manager is enabled.
		// +kubebuilder:validation:Optional
		// +kubebuilder:default:=false
		Enabled bool `json:"enabled,omitempty"`

		Issuer struct {
			// Create specifies whether an issuer should be created.
			// +kubebuilder:validation:Optional
			Create bool `json:"create,omitempty"`

			ACME struct {
				// Email specifies the email for ACME registration.
				// +kubebuilder:validation:Optional
				Email string `json:"email,omitempty"`

				// Server specifies the ACME server URL.
				// +kubebuilder:validation:Optional
				Server string `json:"server,omitempty"`

				// Solvers defines ACME solvers configuration.
				// +kubebuilder:validation:Optional
				Solvers map[string]interface{} `json:"solvers,omitempty"`
			} `json:"acme,omitempty"`
		} `json:"issuer,omitempty"`

		Certificate struct {
			// DNSNames defines the DNS names for the certificate.
			// +kubebuilder:validation:Optional
			DNSNames []string `json:"dnsNames,omitempty"`
		} `json:"certificate,omitempty"`
	} `json:"certManager,omitempty"`
}

type WorkloadIdentityManagerStatus struct {
	// Conditions is a list of conditions and their status
	// +listType=map
	// +listMapKey=type
	// +optional
	Conditions []Condition `json:"condition"`
}

// Condition is just the standard condition fields.
type Condition struct {
	// type of condition in CamelCase or in foo.example.com/CamelCase.
	// ---
	// Many .condition.type values are consistent across resources like Available, but because arbitrary conditions can be
	// useful (see .node.status.conditions), the ability to deconflict is important.
	// The regex it matches is (dns1123SubdomainFmt/)?(qualifiedNameFmt)
	// +required
	// +kubebuilder:validation:Pattern=`^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*/)?(([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9])$`
	// +kubebuilder:validation:MaxLength=316
	Type string `json:"type" protobuf:"bytes,1,opt,name=type"`

	// name of the Operand
	// +required
	Name string `json:"name" protobuf:"bytes,2,opt,name=name"`

	// status of the condition, one of True, False, Unknown.
	// +required
	// +kubebuilder:validation:Enum=True;False;Unknown
	Status ConditionStatus `json:"status"`

	// lastTransitionTime is the last time the condition transitioned from one status to another.
	// This should be when the underlying condition changed.  If that is not known, then using the time when the API field changed is acceptable.
	// +required
	// +kubebuilder:validation:Type=string
	// +kubebuilder:validation:Format=date-time
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`

	Reason string `json:"reason,omitempty"`

	Message string `json:"message,omitempty"`
}

```

### Topology Considerations

#### Hypershift / Hosted Control Planes
None.

#### Standalone Clusters
None.

#### Single-node Deployments or MicroShift
None.

### Implementation Details/Notes/Constraints
Below are the example static manifests used for creating required resources for installing `zero-trust-workload-identity-manager`.

1. ServiceAccounts

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: spire-spiffe-csi-driver
  namespace: <operand-namespace>
  labels:
    app.kubernetes.io/name: spiffe-csi-driver
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: spire-spiffe-oidc-discovery-provider
  namespace: <operand-namespace>
  labels:
    app.kubernetes.io/name: spiffe-oidc-discovery-provider
```
```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: spire-agent
  namespace: <operand-namespace>
  labels:
    app.kubernetes.io/name: spire-agent
```

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: spire-server
  namespace: <operand-namespace>
  labels:
    app.kubernetes.io/name: spire-server

```

2. ClusterRoles and Roles required by `zero-trust-workload-identity-manager`.

```yaml
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: spire-agent
rules:
  - apiGroups: [""]
    resources:
      - pods
      - nodes
      - nodes/proxy
    verbs: ["get"]
```

``` yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: spire-controller-manager
rules:
  - apiGroups: [""]
    resources: ["namespaces"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["admissionregistration.k8s.io"]
    resources: ["validatingwebhookconfigurations"]
    verbs: ["get", "list", "patch", "watch"]
  - apiGroups: [""]
    resources: ["nodes"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["endpoints"]
    verbs: ["get", "list", "watch"]
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
  - apiGroups: ["spire.spiffe.io"]
    resources: ["clusterfederatedtrustdomains"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["spire.spiffe.io"]
    resources: ["clusterfederatedtrustdomains/finalizers"]
    verbs: ["update"]
  - apiGroups: ["spire.spiffe.io"]
    resources: ["clusterfederatedtrustdomains/status"]
    verbs: ["get", "patch", "update"]
  - apiGroups: ["spire.spiffe.io"]
    resources: ["clusterspiffeids"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["spire.spiffe.io"]
    resources: ["clusterspiffeids/finalizers"]
    verbs: ["update"]
  - apiGroups: ["spire.spiffe.io"]
    resources: ["clusterspiffeids/status"]
    verbs: ["get", "patch", "update"]
  - apiGroups: ["spire.spiffe.io"]
    resources: ["clusterstaticentries"]
    verbs: ["create", "delete", "get", "list", "patch", "update", "watch"]
  - apiGroups: ["spire.spiffe.io"]
    resources: ["clusterstaticentries/finalizers"]
    verbs: ["update"]
  - apiGroups: ["spire.spiffe.io"]
    resources: ["clusterstaticentries/status"]
    verbs: ["get", "patch", "update"]
```

```yaml
kind: ClusterRole
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: spire-server
rules:
  - apiGroups: [authentication.k8s.io]
    resources: [tokenreviews]
    verbs:
      - get
      - watch
      - list
      - create
  - apiGroups: [""]
    resources: [nodes, pods]
    verbs:
      - get
      - list
```

```yaml
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: spire-agent
subjects:
  - kind: ServiceAccount
    name: spire-agent
    namespace: <operand-namespace>
roleRef:
  kind: ClusterRole
  name: spire-agent
  apiGroup: rbac.authorization.k8s.io
```

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: spire-controller-manager
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: spire-controller-manager
subjects:
- kind: ServiceAccount
  name: spire-server
  namespace: <operand-namespace>
```

```yaml
# Binds above cluster role to spire-server service account
kind: ClusterRoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: spire-server

subjects:
- kind: ServiceAccount
  name: spire-server
  namespace: <operand-namespace>
roleRef:
  kind: ClusterRole
  name: spire-server
  apiGroup: rbac.authorization.k8s.io
```

```yaml 
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: spire-controller-manager-leader-election
  namespace: <operand-namespace>
rules:
  - apiGroups: [""]
    resources: ["configmaps"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: ["coordination.k8s.io"]
    resources: ["leases"]
    verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
  - apiGroups: [""]
    resources: ["events"]
    verbs: ["create", "patch"]

```

```yaml
kind: Role
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: spire-bundle
  namespace: <operand-namespace>
rules:
  - apiGroups: [""]
    resources: [configmaps]
    resourceNames: [spire-bundle]
    verbs:
      - get
      - patch
```

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: spire-controller-manager-leader-election
  namespace: <operand-namespace>
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: spire-controller-manager-leader-election

subjects:
- kind: ServiceAccount
  name: spire-server
  namespace: <operand-namespace>

```

```yaml 
kind: RoleBinding
apiVersion: rbac.authorization.k8s.io/v1
metadata:
  name: spire-bundle
  namespace: <operand-namespace>

subjects:
- kind: ServiceAccount
  name: spire-server
  namespace: <operand-namespace>
roleRef:
  kind: Role
  name: spire-bundle
  apiGroup: rbac.authorization.k8s.io
```

3. Service for `spire` operands.
```yaml 

apiVersion: v1
kind: Service
metadata:
  name: spire-spiffe-oidc-discovery-provider
  namespace: <operand-namespace>
spec:
  type: ClusterIP
  ports:
    - name: http
      port: 80
      targetPort: http
      protocol: TCP
  selector:
    app.kubernetes.io/name: spiffe-oidc-discovery-provider
    app.kubernetes.io/instance: spire
```

```yaml 
apiVersion: v1
kind: Service
metadata:
  name: spire-controller-manager-webhook
  namespace: <operand-namespace>
  labels:
    app.kubernetes.io/name: spire-server
spec:
  type: ClusterIP
  ports:
    - name: https
      port: 443
      targetPort: https
      protocol: TCP
  selector:
    app.kubernetes.io/name: server
    app.kubernetes.io/instance: spire

```

```yaml
apiVersion: v1
kind: Service
metadata:
  name: spire-server
  namespace: <operand-namespace>
  labels:
    app.kubernetes.io/name: spire-server
spec:
  type: ClusterIP
  ports:
    - name: grpc
      port: 443
      targetPort: grpc
      protocol: TCP
  selector:
    app.kubernetes.io/name: spire-server
    app.kubernetes.io/instance: spire
```
4. DaemonSets for `spire-agents` and `spiffe-csi-driver`
```yaml
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: spire-spiffe-csi-driver
  namespace: <operand-namespace>
  labels:
    helm.sh/chart: spiffe-csi-driver-0.1.0
    app.kubernetes.io/name: spiffe-csi-driver
    app.kubernetes.io/instance: spire
    app.kubernetes.io/version: "0.2.3"
    app.kubernetes.io/managed-by: Helm
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: spiffe-csi-driver
      app.kubernetes.io/instance: spire
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
  template:
    metadata:
      labels:
        app.kubernetes.io/name: spiffe-csi-driver
        app.kubernetes.io/instance: spire
    spec:
      serviceAccountName: spire-spiffe-csi-driver
      
      initContainers:
        - name: set-context
          command:
            - chcon
            - '-Rvt'
            - container_file_t
            - spire-agent-socket/
          image: registry.access.redhat.com/ubi9:latest
          imagePullPolicy: Always
          securityContext:
            capabilities:
              drop:
                - all
            privileged: true
          volumeMounts:
            - name: spire-agent-socket-dir
              mountPath: /spire-agent-socket
          terminationMessagePolicy: File
          terminationMessagePath: /dev/termination-log
      containers:
        # This is the container which runs the SPIFFE CSI driver.
        - name: spiffe-csi-driver
          image: <SPIFFE-CSI-IMAGE-NAME>
          imagePullPolicy: IfNotPresent
          args: [
            "-workload-api-socket-dir", "/spire-agent-socket",
            "-plugin-name", "csi.spiffe.io",
            "-csi-socket-path", "/spiffe-csi/csi.sock",
          ]
          env:
            # The CSI driver needs a unique node ID. The node name can be
            # used for this purpose.
            - name: MY_NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
          volumeMounts:
            # The volume containing the SPIRE agent socket. The SPIFFE CSI
            # driver will mount this directory into containers.
            - mountPath: /spire-agent-socket
              name: spire-agent-socket-dir
              readOnly: true
            # The volume that will contain the CSI driver socket shared
            # with the kubelet and the driver registrar.
            - mountPath: /spiffe-csi
              name: spiffe-csi-socket-dir
            # The volume containing mount points for containers.
            - mountPath: /var/lib/kubelet/pods
              mountPropagation: Bidirectional
              name: mountpoint-dir
          securityContext:
            readOnlyRootFilesystem: true
            capabilities:
              drop:
                - all
            privileged: true
          resources:
            {}
        # This container runs the CSI Node Driver Registrar which takes care
        # of all the little details required to register a CSI driver with
        # the kubelet.
        - name: node-driver-registrar
          image: registry.k8s.io/sig-storage/csi-node-driver-registrar:v2.9.4
          imagePullPolicy: IfNotPresent
          args: [
            "-csi-address", "/spiffe-csi/csi.sock",
            "-kubelet-registration-path", "/var/lib/kubelet/plugins/csi.spiffe.io/csi.sock",
            "-health-port", "9809"
          ]
          volumeMounts:
            # The registrar needs access to the SPIFFE CSI driver socket
            - mountPath: /spiffe-csi
              name: spiffe-csi-socket-dir
            # The registrar needs access to the Kubelet plugin registration
            # directory
            - name: kubelet-plugin-registration-dir
              mountPath: /registration
          ports:
            - containerPort: 9809
              name: healthz
          livenessProbe:
            httpGet:
              path: /healthz
              port: healthz
            initialDelaySeconds: 5
            timeoutSeconds: 5
          resources:
            {}
      volumes:
        - name: spire-agent-socket-dir
          hostPath:
            path: /run/spire/agent-sockets
            type: DirectoryOrCreate
        # This volume is where the socket for kubelet->driver communication lives
        - name: spiffe-csi-socket-dir
          hostPath:
            path: /var/lib/kubelet/plugins/csi.spiffe.io
            type: DirectoryOrCreate
        # This volume is where the SPIFFE CSI driver mounts volumes
        - name: mountpoint-dir
          hostPath:
            path: /var/lib/kubelet/pods
            type: Directory
        # This volume is where the node-driver-registrar registers the plugin
        # with kubelet
        - name: kubelet-plugin-registration-dir
          hostPath:
            path: /var/lib/kubelet/plugins_registry
            type: Directory
```

```yaml 
apiVersion: apps/v1
kind: DaemonSet
metadata:
  name: spire-agent
  namespace: <operand-namespace>
  labels:
    app.kubernetes.io/name: spire-agent
spec:
  selector:
    matchLabels:
      app.kubernetes.io/name: agent
      app.kubernetes.io/instance: spire
      app.kubernetes.io/component: default
  updateStrategy:
    type: RollingUpdate
    rollingUpdate:
      maxUnavailable: 1
  template:
    metadata:
      annotations:
        kubectl.kubernetes.io/default-container: spire-agent
        checksum/config: 6843076b7bd6f18317742c45125cfdea234ded3126f63cc508cab7ce9bd6f505
      labels:
        app.kubernetes.io/name: agent
        app.kubernetes.io/instance: spire
        app.kubernetes.io/component: default
    spec:
      hostPID: true
      hostNetwork: true
      dnsPolicy: ClusterFirstWithHostNet
      serviceAccountName: spire-agent
      securityContext:
        {}
      
      initContainers:
        - name: ensure-alternate-names
          image: <IMAGE-NAME>
          imagePullPolicy: Always
          command: ["bash", "-xc"]
          args:
            - |
              cd /run/spire/agent-sockets
              L=`readlink socket`
              [ "x$L" != "xspire-agent.sock" ] && rm -f socket
              [ ! -L socket ] && ln -s spire-agent.sock socket
              L=`readlink api.sock`
              [ "x$L" != "xspire-agent.sock" ] && rm -f api.sock
              [ ! -L api.sock ] && ln -s spire-agent.sock api.sock
              [ -L spire-agent.sock ] && rm -f spire-agent.sock
              exit 0
          resources:
            {}
          volumeMounts:
            - name: spire-agent-socket-dir
              mountPath: /run/spire/agent-sockets
          securityContext:
            runAsUser: 0
            runAsGroup: 0
      containers:
        - name: spire-agent
          image: <SPIFFE-CSI-IMAGE-NAME>
          imagePullPolicy: IfNotPresent
          args: ["-config", "/opt/spire/conf/agent/agent.conf"]
          securityContext:
            {}
          env:
            - name: PATH
              value: "/opt/spire/bin:/bin"
            - name: MY_NODE_NAME
              valueFrom:
                fieldRef:
                  fieldPath: spec.nodeName
          ports:
            - containerPort: 9982
              name: healthz
          volumeMounts:
            - name: spire-config
              mountPath: /opt/spire/conf/agent
              readOnly: true
            - name: spire-bundle
              mountPath: /run/spire/bundle
              readOnly: true
            - name: spire-agent-socket-dir
              mountPath: /tmp/spire-agent/public
              readOnly: false
            - name: spire-token
              mountPath: /var/run/secrets/tokens
          livenessProbe:
            httpGet:
              path: /live
              port: healthz
            initialDelaySeconds: 15
            periodSeconds: 60
          readinessProbe:
            httpGet:
              path: /ready
              port: healthz
            initialDelaySeconds: 10
            periodSeconds: 30
          resources:
            {}
      volumes:
        - name: spire-config
          configMap:
            name: spire-agent
        - name: spire-agent-admin-socket-dir
          emptyDir: {}
        - name: spire-bundle
          configMap:
            name: spire-bundle
        - name: spire-token
          projected:
            sources:
            - serviceAccountToken:
                path: spire-agent
                expirationSeconds: 7200
                audience: spire-server
        - name: spire-agent-socket-dir
          hostPath:
            path: /run/spire/agent-sockets
            type: DirectoryOrCreate

```

5. Deployment for `spire-spiffe-oidc-discovery-provider`
```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: spire-spiffe-oidc-discovery-provider
  namespace: <operand-namespace>
  labels:
    app.kubernetes.io/name: spiffe-oidc-discovery-provider
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: spiffe-oidc-discovery-provider
      app.kubernetes.io/instance: spire
  template:
    metadata:
      annotations:
      labels:
        component: oidc-discovery-provider
    spec:
      serviceAccountName: spire-spiffe-oidc-discovery-provider
      securityContext:
        {}
      initContainers:
      containers:
        - name: spiffe-oidc-discovery-provider
          securityContext:
            {}
          image: <IMAGE-NAME>
          imagePullPolicy: IfNotPresent
          args:
            - -config
            - /run/spire/oidc/config/oidc-discovery-provider.conf
          ports:
            - containerPort: 8008
              name: healthz
          volumeMounts:
            - name: spiffe-workload-api
              mountPath: /spiffe-workload-api
              readOnly: true
            - name: spire-oidc-sockets
              mountPath: /run/spire/oidc-sockets
              readOnly: false
            - name: spire-oidc-config
              mountPath: /run/spire/oidc/config/oidc-discovery-provider.conf
              subPath: oidc-discovery-provider.conf
              readOnly: true
            - name: certdir
              mountPath: /certs
              readOnly: true
          readinessProbe:
            httpGet:
              path: /ready
              port: healthz
            initialDelaySeconds: 5
            periodSeconds: 5
          livenessProbe:
            httpGet:
              path: /live
              port: healthz
            initialDelaySeconds: 5
            periodSeconds: 5
          resources:
            {}
        - name: nginx
          securityContext:
            {}
          image: <IMAGE-NAME>
          imagePullPolicy: IfNotPresent
          ports:
            - containerPort: 8080
              name: http
          volumeMounts:
            - name: spire-oidc-sockets
              mountPath: /run/spire/oidc-sockets
              readOnly: true
            - name: spire-oidc-config
              mountPath: /etc/nginx/conf.d/default.conf
              subPath: default.conf
              readOnly: true
            - name: nginx-tmp
              mountPath: /tmp
              readOnly: false
          resources:
            {}
      volumes:
        - name: spiffe-workload-api
          csi:
            driver: "csi.spiffe.io"
            readOnly: true
        - name: spire-oidc-sockets
          emptyDir: {}
        - name: spire-oidc-config
          configMap:
            name: spire-spiffe-oidc-discovery-provider
        - name: nginx-tmp
          emptyDir: {}
        - name: certdir
          emptyDir: {}
```

6. StatefulSet for `spire-server`
```yaml
apiVersion: apps/v1
kind: StatefulSet
metadata:
  name: spire-server
  namespace: <operand-namespace>
  labels:
    app.kubernetes.io/name: spire-server
spec:
  replicas: 1
  serviceName: spire-server
  selector:
    matchLabels:
      app.kubernetes.io/name: spire-server
      app.kubernetes.io/instance: spire
      app.kubernetes.io/component: spire-server
  template:
    metadata:
      annotations:
        kubectl.kubernetes.io/default-container: spire-server
      labels:
        app.kubernetes.io/name: spire-server
    spec:
      serviceAccountName: spire-server
      shareProcessNamespace: true
      securityContext:
        {}
      
      containers:
        - name: spire-server
          securityContext:
            {}
          image: ghcr.io/spiffe/spire-server:1.9.6
          imagePullPolicy: IfNotPresent
          args:
            - -expandEnv
            - -config
            - /run/spire/config/server.conf
          env:
          - name: PATH
            value: "/opt/spire/bin:/bin"
          ports:
            - name: grpc
              containerPort: 8081
              protocol: TCP
            - containerPort: 8080
              name: healthz
          livenessProbe:
            httpGet:
              path: /live
              port: healthz
            failureThreshold: 2
            initialDelaySeconds: 15
            periodSeconds: 60
            timeoutSeconds: 3
          readinessProbe:
            httpGet:
              path: /ready
              port: healthz
            initialDelaySeconds: 5
            periodSeconds: 5
          resources:
            {}
          volumeMounts:
            - name: spire-server-socket
              mountPath: /tmp/spire-server/private
              readOnly: false
            - name: spire-config
              mountPath: /run/spire/config
              readOnly: true
            - name: spire-data
              mountPath: /run/spire/data
              readOnly: false
            - name: server-tmp
              mountPath: /tmp
              readOnly: false
        
        - name: spire-controller-manager
          securityContext:
            {}
          image: <IMAGE_NAME>
          imagePullPolicy: IfNotPresent
          args:
            - --config=controller-manager-config.yaml
          env:
            - name: ENABLE_WEBHOOKS
              value: "true"
          ports:
            - name: https
              containerPort: 9443
              protocol: TCP
            - containerPort: 8083
              name: healthz
          livenessProbe:
            httpGet:
              path: /healthz
              port: healthz
          readinessProbe:
            httpGet:
              path: /readyz
              port: healthz
          resources:
            {}
          volumeMounts:
            - name: spire-server-socket
              mountPath: /tmp/spire-server/private
              readOnly: true
            - name: controller-manager-config
              mountPath: /controller-manager-config.yaml
              subPath: controller-manager-config.yaml
              readOnly: true
            - name: spire-controller-manager-tmp
              mountPath: /tmp
              subPath: spire-controller-manager
              readOnly: false
      volumes:
        - name: server-tmp
          emptyDir: {}
        - name: spire-config
          configMap:
            name: spire-server
        - name: spire-server-socket
          emptyDir: {}
        - name: spire-controller-manager-tmp
          emptyDir: {}
        - name: controller-manager-config
          configMap:
            name: spire-controller-manager
  volumeClaimTemplates:
    - metadata:
        name: spire-data
      spec:
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 1Gi
```

### Risks and Mitigations

### Drawbacks
None.

## Test Plan

### Testing Strategies

- **Unit Tests**
  - Validate Operator logic
  - Test configuration parsing
  - Verify custom resource validation

- **Integration Tests**
  - Deploy identity management components
  - Validate workload identity issuance
  - Test credential rotation mechanisms

- **Performance Testing**
  - Scalability tests with multiple workloads
  - Measure identity issuance overhead
  - Benchmark credential management performance


## Graduation Criteria

### Dev Preview -> Tech Preview

- Feature available for end-to-end usage.
- Complete end user documentation.
- UTs and e2e tests are present.
- Gather feedback from the users.

### Tech Preview -> GA
N/A. This feature is for Tech Preview, until decided for GA.

### Removing a deprecated feature
None.

## Upgrade / Downgrade Strategy

## Version Skew Strategy
zero-trust-workload-indentity-manager will be supported for OpenShift versions 4.19+ as Tech-preview


## Operational Aspects of API Extensions


## Support Procedures
None.

## Alternatives
None.

## Infrastructure Needed [optional]
None