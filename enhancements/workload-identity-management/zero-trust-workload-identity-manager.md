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

Customers deploying OpenShift increasingly require a robust zero-trust approach to workload identity management that goes beyond traditional network security models. As microservices architectures become more complex, organizations need a solution that provides dynamic, cryptographically verifiable identities for their workloads, enabling secure and granular authentication across distributed systems. The current approach to identity management often relies on static, perimeter-based security mechanisms that fail to address the sophisticated security challenges of modern cloud-native environments. By implementing a Zero-Trust Workload Identity Manager, customers can achieve more comprehensive protection, reduce manual operational overhead, and ensure that every service interaction is explicitly verified and secured.

### User Stories

- As an OpenShift user, I want to have an option to dynamically deploy `spire-sever`, so that it can be used only when required by creating the custom resource.
- As an OpenShift user, I want to have an option to dynamically attest `spiffe-csi-driver` on the nodes, so that it can be used only when required by creating the custom resource.
- As an OpenShift user, I want `spire-agents` to mount workload identity tokens using the `spiffe-csi-driver` so that applications can consume workload identities securely.
- As an OpenShift user, I want `spire` to automatically issue and manage `SVIDs` for workloads so that workloads can authenticate securely.
- As an OpenShift user, I want to dynamically configure `trust domains` so that I can manage trust relationships between workloads and clusters securely.

### Goals  
- Integrate SPIFFE/SPIRE via an operator for automated workload identity lifecycle management.  
- Automate SVID issuance/rotation for workloads without manual intervention.  
- Securely provision identities to workloads via SPIFFE CSI Driver.  
- Enable dynamic trust domain configuration via CRs for multi-cluster/tenant scenarios.  
- Enforce zero-trust with mutual authentication (mTLS) for workloads.  
- Integrate with OpenShift Monitoring for observability.  

### Non-Goals  
- Replace existing auth systems (e.g., Service Accounts, OAuth).  
- Manage non-workload identities (users/machines).  
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

The operator will create and manage the following resources to deploy SPIRE and SPIFFE components:

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
        - `spire-mgmt-spire-controller-manager` (manage SPIRE CRDs)
        - `spire-mgmt-spire-server` (token reviews, node/pod access)

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

- **Security Testing**
  - Vulnerability scanning
  - Penetration testing
  - Compliance checks

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