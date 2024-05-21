---
title: istio-csr-controller
authors:
  - "@bhb"
reviewers:
  - "@tgeer" ## reviewer for cert-manager component
approvers:
  - "@tgeer" ## approver for cert-manager component
api-approvers:
  - "@tgeer" ## approver for cert-manager component
creation-date: 2024-01-22
last-updated: 2024-05-13
tracking-link:
  - https://issues.redhat.com/browse/CM-234
see-also:
  - NA
replaces:
  - NA
superseded-by:
  - NA
---

# Extend cert-manager-operator to manage istio-csr.

## Summary

This enhancement describes the proposal to extend cert-manager-operator to deploy and manage the `istio-csr`
solution. `OpenShift Service Mesh` facilitates the security of both intra-cluster and external service
communications through mTLS. And istio-csr acts as an agent to obtain and renew certificates that
facilitate mTLS from the `cert-manager`.

## Motivation

cert-manager enables obtaining certificates from [various](https://docs.openshift.com/container-platform/4.15/security/cert_manager_operator/index.html#cert-manager-issuer-types_cert-manager-operator-about)
certificate authorities and also ensures the validity of certificates by renewing them before they expire.
`OpenShift Service Mesh` secures intra-cluster and external service connections with TLS encryption. `cert-manager`
can be used to obtain the required certificates through custom resources `issuer` and `clusterissuer`. `istio-csr`
is an agent that simplifies the process of obtaining the certificates from the `cert-manager` for
`OpenShift Service Mesh`. `cert-manager-operator` manages `cert-manager` and extending the operator to manage
`istio-csr` will help the users to use all the solutions mentioned above effectively and easily.

### User Stories

- As an OpenShift user, I want to have an option to dynamically enable `istio-csr`, so that it can be used only
  when required.
- As an OpenShift user, I want to have an option to dynamically configure `istio-csr`, so that only the required
  features can be enabled.

### Goals

- `cert-manager-operator` to be extended to manage `istio-csr` along with currently managed `cert-manager`.
- New custom resource `istiocsr` to be made available to enable and configure deployment.

### Non-Goals

- `istio-csr` must be enabled and configured before installing `OpenShift Service Mesh`, which is a
  limitation of `OpenShift Service Mesh`.
- Downgrading OpenShift cluster from `istio-csr` supported version to unsupported version is not supported.

## Proposal

`istio-csr` will be managed by `cert-manager-operator` which can be enabled and configured by a user as and
when required, using new custom resource which will be made available for configuring `istio-csr`.

A new controller will be added to `cert-manager-operator` to manage and maintain the `istio-csr` deployment in
desired state which can be monitored by user through the status sub-resource of the new custom resource.

`istio-csr` controller will make use of static manifest templates for creating below resources to
successfully deploy `istio-csr` agent.
* `certificate.cert-manager.io`
* `clusterissuer.cert-manager.io`
* `issuer.cert-manager.io`
* `clusterrole.rbac.authorization.k8s.io`
* `clusterrolebinding.rbac.authorization.k8s.io`
* `deployment`
* `service`
* `servicemonitor.monitoring.coreos.com`
* `namespace`
* `role.rbac.authorization.k8s.io`
* `rolebinding.rbac.authorization.k8s.io`
* `serviceaccount`

`istio-csr` CR object will be a singleton object with the name `default` and an OpenShift user will have to
create the same for enabling the deployment. `istio-csr` should be deployed in the same namespace as that of
the `OpenShift Service Mesh` control plane.

When `istiocsr` custom resource is created but `issuerref` is not provided operator will create a
`ClusterIssuer` in the `cert-manager` namespace since cert-manager controller observes CAs created in its own
namespace. And using the ClusterIssuer an Issuer will be created in the istio-csr's namespace for signing the
certificates requested by `OpenShift Service Mesh`.

An OpenShift user could update any configurations in the spec section of the API, except for below
configurations which are immutable and once set cannot be modified.
- namespace:
  Namespace where the `istio-csr` agent will be deployed, will also be used for creating other resources
  required for agent to be functional and the same namespace will be used for creating the `issuer` which
  is used for signing the certificate requests. And allowing to change this config will disrupt the
  functionality of `OpenShift Service Mesh` also deployed in the same namespace.
- issuerRef:
  IssuerRef is the reference to the certificate authority used for signing the certificates requested by
  `OpenShift Service Mesh`. And allowing to change this config will create issues to renewing the existing
  certificates.

Configurations made available in the spec of `istio-csr` custom resource are passed as process arguments and
updating these configurations would cause new rollout to happen.

When an OpenShift user deletes istio-csr CR object `istio-csr` agent deployment will be deleted and all the
other resources created will also be cleaned up.

A fork of [upstream](https://github.com/cert-manager/istio-csr) `istio-csr` will be created [downstream](https://github.com/openshift)
for better version management.

### Workflow Description

- An OpenShift user enables `istio-csr` by creating the new custom resource made available for configuring
  `istio-csr`.
- `istio-csr` controller based on the configuration in CR
  - Creates below `Issuers` when user does not provide `Issuer Reference` to be used by `istio-csr` for
    obtaining the certificates.
    - Creates an `ClusterIssuer` in `cert-manager` namespace which acts as root CA for obtaining the
      CA certificate for `istio-csr` use case.
    - Creates an `Issuer` in `istio-system` namespace which is an intermediate CA signed by `ClusterIssuer`
      root CA.
  - Deploys istio-csr agent in `istio-system` namespace.

### API Extensions

Below new API `istiocsrs.operator.openshift.io` is introduced for managing istio-csr.
```golang
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// IstioCSR describes a managed istio-csr agent for the cluster. The
// controller can service requests to obtain certificates from the configured
// issuer.
//
// When an IstioCSR is created, a new deployment is created which manages the
// istio-csr agent and keeps it in the desired state.
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default'",message="istiocsr is a singleton, .metadata.name must be 'default'"
type IstioCSR struct {
	metav1.TypeMeta `json:",inline"`

	// metadata is the standard object's metadata.
	// More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec is the specification of the desired behavior of the IstioCSR.
	Spec IstioCSRSpec `json:"spec,omitempty"`

	// status is the most recently observed status of the IstioCSR.
	Status IstioCSRStatus `json:"status,omitempty"`
}

// IstioCSRSpec is the specification of the desired behavior of the IstioCSR.
type IstioCSRSpec struct {
	apiv1.OperatorSpec `json:",inline"`

	// ControllerConfig is for configuring istio-csr operator behavior.
	ControllerConfig `json:"controllerConfig,omitempty"`

	// ApplicationConfig is for configuring istio-csr operand behavior.
	ApplicationConfig `json:"applicationConfig,omitempty"`
}

// ControllerConfig is for configuring istio-csr operator behavior.
type ControllerConfig struct {
	// Namespace where the istio control-plane is running.
	// Same namespace will be used for deploying istio-csr agent.
	// +kubebuilder:default:="istio-system"
	// +kubebuilder:validation:XValidation:rule="oldSelf == '' || self == oldSelf",message="namespace is immutable once set"
	// +optional
	Namespace string `json:"namespace,omitempty"`

	// Volumes is for defining the additional volumes for mounting custom
	// root CAs.
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`

	// VolumeMounts is for defining the additional volumes mounts for mounting
	// custom root CAs.
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`

	// Resources is for defining the resource requirements.
	// ref: https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`

	// Affinity is for setting scheduling affinity rules.
	// ref: https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations is for setting the pod tolerations.
	// ref: https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// NodeSelector is for defining the scheduling criteria using node labels.
	// ref: https://kubernetes.io/docs/concepts/configuration/assign-pod-node/
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Envs is for defining the list of environment variables to set in the container.
	// +optional
	Envs []corev1.EnvVar `json:"envs,omitempty"`

	// Labels is for setting additional labels on the resources created.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// ApplicationConfig is for configuring istio-csr operand behavior.
type ApplicationConfig struct {
	// LogLevel is for setting verbosity of istio-csr logging.
	// Supported log levels: 1-5.
	// +kubebuilder:default:=1
	// +optional
	LogLevel int32 `json:"logLevel,omitempty"`

	// ConfigmapNamespaceSelector is for restricting the creation of ConfigMaps with
	// root CA certificates by istio-csr in selected namespaces. When unset, ConfigMap
	// is created in all namespaces.
	// Example: maistra.io/member-of=istio-system to set namespaces registered with
	// istio.
	// +optional
	ConfigmapNamespaceSelector string `json:"configmapNamespaceSelector,omitempty"`

	// MetricsConfig is for configuring Prometheus metrics behavior.
	// +optional
	MetricsConfig `json:"metricsConfig,omitempty"`

	// CertManagerConfig is for configuring cert-manager specifics.
	// +optional
	CertManagerConfig `json:"certManagerConfig,omitempty"`

	// TLSConfig is for configuring certificate specifics.
	// +optional
	TLSConfig `json:"tlsConfig,omitempty"`

	// ServerConfig is for configuring the server endpoint used by istio
	// for obtaining the certificates.
	// +optional
	ServerConfig `json:"serverConfig,omitempty"`

	// IstioConfig is for configuring the istio specifics.
	IstioConfig `json:"istioConfig,omitempty"`
}

// MetricsConfig is for configuring Prometheus metrics behavior.
type MetricsConfig struct {
	// Port for exposing Prometheus metrics on 0.0.0.0 on path '/metrics'.
	// +kubebuilder:default:="9402"
	// +optional
	Port string `json:"port,omitempty"`

	// MetricServiceConfig is for confuring the metrics server specifics.
	// +optional
	MetricServiceConfig `json:"metricServiceConfig,omitempty"`
}

// MetricServiceConfig is for confuring the metrics server specifics.
type MetricServiceConfig struct {
	// Enabled when set will create a Service resource to expose metrics endpoint.
	// +kubebuilder:default:=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// ServiceType of the service resource created metrics.
	// +kubebuilder:default:="ClusterIP"
	// +optional
	ServiceType corev1.ServiceType `json:"serviceType,omitempty"`

	// MetricsServiceMonitorConfig is for configuring the service monitor specifics
	// for the metrics service.
	// +optional
	MetricsServiceMonitorConfig `json:"metricsServiceMonitorConfig,omitempty"`
}

// MetricsServiceMonitorConfig is for configuring the service monitor specifics
// for the metrics service.
type MetricsServiceMonitorConfig struct {
	// Enabled when set creates Prometheus ServiceMonitor resource for approver-policy.
	// +kubebuilder:default:=false
	// +optional
	Enabled bool `json:"enabled,omitempty"`

	// PrometheusInstance name to set the value for the "prometheus" label on the ServiceMonitor.
	// +kubebuilder:default:="default"
	// +optional
	PrometheusInstance string `json:"prometheusInstance,omitempty"`

	// Interval defines periodicity the Prometheus will scrape for metrics.
	// +kubebuilder:default:="10s"
	// +optional
	Interval time.Duration `json:"interval,omitempty"`

	// ScrapeTimeout defines timeout on each metric probe request.
	// +kubebuilder:default:="5s"
	// +optional
	ScrapeTimeout time.Duration `json:"scrapeTimeout,omitempty"`

	// Labels to add to the ServiceMonitor resource.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`
}

// CertManagerConfig is for configuring cert-manager specifics.
type CertManagerConfig struct {
	// IssuerRef contains details to the referenced object used for
	// obtaining the certificates.
	// When unset operator will create Issuer in the configured istio's
	// namespace.
	// +kubebuilder:validation:XValidation:rule="oldSelf == '' || self == oldSelf",message="issuerRef is immutable once set"
	// +optional
	IssuerRef certmanagerv1.ObjectReference `json:"issuerRef,omitempty"`
}

// TLSConfig is for configuring certificate specifics.
type TLSConfig struct {
	// TrustDomain is the cluster's trust domain.
	// +kubebuilder:default:="cluster.local"
	// +optional
	TrustDomain string `json:"trustDomain,omitempty"`

	// RootCAFile is for setting the file location containing the root CA which is
	// present in the configured IssuerRef. File should be made available using the
	// Volume and VolumeMount options.
	// +optional
	RootCAFile string `json:"rootCAFile,omitempty"`

	// CertificateDNSNames contains DNS names to be added to the certificate SAN.
	// +kubebuilder:default:=["cert-manager-istio-csr.istio-system.svc"]
	// +optional
	CertificateDNSNames []string `json:"certificateDNSNames,omitempty"`

	// IstiodCertificateDuration is the istio's certificate validity period.
	// +kubebuilder:default:="1h"
	// +optional
	IstiodCertificateDuration time.Duration `json:"istiodCertificateDuration,omitempty"`

	// IstiodCertificateRenewBefore is the ahead time to renew the istio's certificate before
	// expiry.
	// +kubebuilder:default:="30m"
	// +optional
	IstiodCertificateRenewBefore time.Duration `json:"istiodCertificateRenewBefore,omitempty"`

	// IstiodPrivateKeySize is the key size to be for RSAKey.
	// +kubebuilder:default:=2048
	// +optional
	IstiodPrivateKeySize int32 `json:"istiodPrivateKeySize,omitempty"`
}

// ServerConfig is for configuring the server endpoint used by istio
// for ontaining the certificates.
type ServerConfig struct {
	// Address to serve istio-csr gRPC service.
	// +kubebuilder:default:="0.0.0.0"
	// +optional
	Address string `json:"address,omitempty"`

	// Port to serve istio-csr gRPC service.
	// +kubebuilder:default:="6443"
	// +optional
	Port string `json:"port,omitempty"`

	// CertificateKeySize is the server's serving certificate's key size.
	// +kubebuilder:default:=2048
	// +optional
	CertificateKeySize int32 `json:"certificateKeySize,omitempty"`

	// SignatureAlgorithm is the signature algorithm to use when generating
	// private keys. At present only RSA and ECDSA are supported.
	// +kubebuilder:default:="RSA"
	// +kubebuilder:validation:Enum:="RSA";"ECDSA"
	// +optional
	SignatureAlgorithm string `json:"signatureAlgorithm,omitempty"`

	// MaxCertificateDuration is the maximum validity duration that can be
	// requested for a certificate.
	// +kubebuilder:default:="1h"
	// +optional
	MaxCertificateDuration time.Duration `json:"certificateDuration,omitempty"`
}

// IstioConfig is for configuring the istio specifics.
type IstioConfig struct {
	// Revisions are the istio revisions that are currently installed in the cluster.
	// Changing this field will modify the DNS names that will be requested for
	// the istiod certificate.
	// +kubebuilder:default:=["default"]
	// +optional
	Revisions []string `json:"revisions,omitempty"`
}

// IstioCSRStatus is the most recently observed status of the IstioCSR.
type IstioCSRStatus struct {
	apiv1.OperatorStatus `json:",inline"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

None

#### Standalone Clusters

None

#### Single-node Deployments or MicroShift

None

### Implementation Details/Notes/Constraints

None

### Risks and Mitigations

An OpenShift administrator configuring istio-csr CR object could configure insecure certificate
signature algorithm, certificate key size or certificate validity to be too long which could
cause vulnerability.
  - These configurations could be validated and can be overridden with default values.

### Drawbacks

None

## Design Details

## Open Questions [optional]

None

## Test Plan

- Enable istio-csr through cert-manager-operator CR and check the behavior
  with default istio-csr configuration.
- Enable istio-csr by creating the istio-csr CR object with permutations of
  configurations and validate the behavior.
- Upgrade/downgrade testing
  - Scenarios mentioned in the section Upgrade / Downgrade Strategy has expected behavior.
  - Sufficient time for feedback from the QE.
- The feature is available by default and does not have any specific featureGate defined.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Feature available for end-to-end usage.
- Complete end user documentation.
- UTs and e2e tests are present.
- Gather feedback from the users.

### Tech Preview -> GA

N/A. This feature is for Dev Preview, until decided for GA.

### Removing a deprecated feature

None.

## Upgrade / Downgrade Strategy

On upgrade:
- cert-manager-operator will have functionality to enable istio-csr agent and based
  on the administrator configuration, agent will be deployed and available for usage
  with Service Mesh.
- Enabling istio-csr when Service Mesh was already deployed before upgrade is not
  [supported](https://cert-manager.io/docs/usage/istio-csr/#installing-istio-csr-after-istio).

## Version Skew Strategy

Istio-csr will be supported for OpenShift Service Mesh Operator 2.4+, Istio v1.10+ and cert-manager v1.3+.

## Operational Aspects of API Extensions

Below static manifests are used for creating `istio-csr` required resources when enabled.
1. A ClusterIssuer for istio-csr which will act as the Root CA certificate. Creating an ClusterIssuer in
   `cert-manager` namespace since `cert-manager` will look for referenced secrets in its own namespace.
```yaml
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: istio-csr-selfsigned-issuer
  namespace: cert-manager
spec:
  selfSigned: {}
---
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: istio-csr-root-ca
  namespace: cert-manager
spec:
  isCA: true
  duration: 21600h # 900d
  secretName: istio-csr-root-ca
  commonName: istio-csr-root-ca
  issuerRef:
    name: istio-csr-selfsigned-issuer
    kind: Issuer
    group: cert-manager.io
---
apiVersion: cert-manager.io/v1
kind: ClusterIssuer
metadata:
  name: istio-csr-cluster-issuer
spec:
  ca:
    secretName: istio-csr-root-ca
```

2. An intermediate CA signed by the Root CA in the previous step, which will be referenced in istio-csr CR
   for obtaining the certificates for `OpenShift Service Mesh`.
```yaml
apiVersion: cert-manager.io/v1
kind: Certificate
metadata:
  name: istio-csr-ca
  namespace: istio-system
spec:
  isCA: true
  duration: 21600h
  secretName: istio-csr-ca
  commonName: istio-csr-ca
  issuerRef:
    name: istio-csr-cluster-issuer
    kind: ClusterIssuer
    group: cert-manager.io
---
apiVersion: cert-manager.io/v1
kind: Issuer
metadata:
  name: istio-csr-issuer
  namespace: istio-system
spec:
  ca:
    secretName: istio-csr-ca
```

3. Resources required for deploying istio-csr agent.
   1. `istio-system` namespace, where all the istio-csr required resources will be created.
      ```yaml
      apiVersion: v1
      kind: Namespace
      metadata:
        name: istio-system
      ```

   2. Service for creating istio-csr grpc server, for serving CertificateRequests endpoint.
      ```yaml
      apiVersion: v1
      kind: Service
      metadata:
        name: cert-manager-istio-csr
        namespace: istio-system
        labels:
          app: cert-manager-istio-csr
      spec:
        type: ClusterIP
        ports:
        - port: 443
          targetPort: 6443
          protocol: TCP
          name: grpc
        selector:
          app: cert-manager-istio-csr
      ```

   3. Service for supporting metrics.
      ```yaml
      apiVersion: v1
      kind: Service
      metadata:
        name: cert-manager-istio-csr-metrics
        namespace: istio-system
        labels:
          app: cert-manager-istio-csr
      spec:
        type: ClusterIP
        ports:
        - port: 9402
          targetPort: 9402
          protocol: TCP
          name: metrics
        selector:
          app: cert-manager-istio-csr
      ```

   4. ServiceMonitor resource to monitor and collect metrics of istio-csr.
      ```yaml
      apiVersion: monitoring.coreos.com/v1
      kind: ServiceMonitor
      metadata:
        name: cert-manager-istio-csr
        namespace: istio-system
        labels:
          app: cert-manager-istio-csr
          prometheus: default
      spec:
        jobLabel: cert-manager-istio-csr
        selector:
          matchLabels:
            app: cert-manager-istio-csr-metrics
        namespaceSelector:
          matchNames:
            - cert-manager
        endpoints:
        - targetPort: 9402
          path: "/metrics"
          interval: 10s
          scrapeTimeout: 5s
      ```

   5. ServiceAccount required by the agent, which will be associated with the required ClusterRole and
      Roles in next step.
      ```yaml
      apiVersion: v1
      kind: ServiceAccount
      metadata:
        labels:
          app: cert-manager-istio-csr
        name: cert-manager-istio-csr
        namespace: istio-system
      ```

   6. ClusterRoles and Roles required by istio-csr.
      ```yaml
      ## required for creating ConfigMap with the CA in selected namespaces
      ## by istio-csr agent.
      kind: ClusterRole
      apiVersion: rbac.authorization.k8s.io/v1
      metadata:
        name: cert-manager-istio-csr
        labels:
          app: cert-manager-istio-csr
      rules:
      - apiGroups:
        - ""
        resources:
        - "configmaps"
        verbs: ["get", "list", "create", "update", "watch"]
      - apiGroups:
        - ""
        resources:
        - "namespaces"
        verbs: ["get", "list", "watch"]
      - apiGroups:
        - "authentication.k8s.io"
        resources:
        - "tokenreviews"
        verbs:
        - "create"
      
      ```
      
      ```yaml
      kind: ClusterRoleBinding
      apiVersion: rbac.authorization.k8s.io/v1
      metadata:
        labels:
          app: cert-manager-istio-csr
        name: cert-manager-istio-csr
      roleRef:
        apiGroup: rbac.authorization.k8s.io
        kind: ClusterRole
        name: cert-manager-istio-csr
      subjects:
      - kind: ServiceAccount
        name: cert-manager-istio-csr
        namespace: istio-system
      ```

      ```yaml
      ## required for creating CertificateRequests.
      kind: Role
      apiVersion: rbac.authorization.k8s.io/v1
      metadata:
        name: cert-manager-istio-csr
        namespace: istio-system
        labels:
          app: cert-manager-istio-csr
      rules:
      - apiGroups:
        - "cert-manager.io"
        resources:
        - "certificaterequests"
        verbs:
        - "get"
        - "list"
        - "create"
        - "update"
        - "delete"
        - "watch"
      - apiGroups: [""]
        resources: ["events"]
        verbs: ["create"]
      ```
      
      ```yaml
      kind: RoleBinding
      apiVersion: rbac.authorization.k8s.io/v1
      metadata:
        name: cert-manager-istio-csr
        namespace: istio-system
        labels:
          app: cert-manager-istio-csr
      roleRef:
        apiGroup: rbac.authorization.k8s.io
        kind: Role
        name: cert-manager-istio-csr
      subjects:
      - kind: ServiceAccount
        name: cert-manager-istio-csr
        namespace: istio-system
      ```
      
      ```yaml
      kind: Role
      apiVersion: rbac.authorization.k8s.io/v1
      metadata:
        labels:
          app: cert-manager-istio-csr
        name: cert-manager-istio-csr-leases
        namespace: istio-system
      rules:
      - apiGroups:
        - "coordination.k8s.io"
        resources:
        - "leases"
        verbs:
        - "get"
        - "create"
        - "update"
        - "watch"
        - "list"
      ```
      
      ```yaml
      kind: RoleBinding
      apiVersion: rbac.authorization.k8s.io/v1
      metadata:
        name: cert-manager-istio-csr-leases
        namespace: istio-system
        labels:
          app: cert-manager-istio-csr
      roleRef:
        apiGroup: rbac.authorization.k8s.io
        kind: Role
        name: cert-manager-istio-csr-leases
      subjects:
      - kind: ServiceAccount
        name: cert-manager-istio-csr
        namespace: istio-system
      ```

   7. Certificate required by the istiod.
      ```yaml
      ## certificate required by the istiod.
      apiVersion: cert-manager.io/v1
      kind: Certificate
      metadata:
        name: istiod
        namespace: istio-system
        labels:
          app: cert-manager-istio-csr
      spec:
        commonName: istiod.istio-system.svc
        dnsNames:
        - istiod-basic.istio-system.svc
        uris:
        - spiffe://cluster.local/ns/istio-system/sa/istiod-service-account
        secretName: istiod-tls
        duration: 1h
        renewBefore: 30m
        privateKey:
          rotationPolicy: Always
          algorithm: RSA
          size: 2048
        revisionHistoryLimit: 1
        issuerRef:
          name: istio-csr-issuer
          kind: Issuer
          group: cert-manager.io
      ```
      
   8. istio-csr deployment.
      ```yaml
      apiVersion: apps/v1
      kind: Deployment
      metadata:
        name: cert-manager-istio-csr
        namespace: istio-system
        labels:
          app: cert-manager-istio-csr
      spec:
        replicas: 1
        selector:
          matchLabels:
            app: cert-manager-istio-csr
        template:
          metadata:
            labels:
              app: cert-manager-istio-csr
          spec:
            serviceAccountName: cert-manager-istio-csr
            containers:
            - name: cert-manager-istio-csr
              image: "quay.io/jetstack/cert-manager-istio-csr:v0.7.1"
              imagePullPolicy: IfNotPresent
              ports:
              - containerPort: 6443
              - containerPort: 9402
              readinessProbe:
                httpGet:
                  port: 6060
                  path: /readyz
                initialDelaySeconds: 3
                periodSeconds: 7
              command: ["cert-manager-istio-csr"]
              args:
                - "--log-level=1"
                - "--metrics-port=9402"
                - "--readiness-probe-path=/readyz"
                - "--readiness-probe-port=6060"
      
                # cert-manager
                - "--certificate-namespace=istio-system"
                - "--issuer-group=cert-manager.io"
                - "--issuer-kind=Issuer"
                - "--issuer-name=istio-csr-issuer"
                - "--preserve-certificate-requests=false"
      
                # AdditionalAnnotations
      
                # tls
                - "--root-ca-file=/var/run/secrets/istio-csr/ca.crt"
                - "--serving-certificate-dns-names=cert-manager-istio-csr.istio-system.svc"
                - "--serving-certificate-duration=1h"
                - "--serving-certificate-key-size=2048"
                - "--serving-signature-algorithm=RSA"
                - "--trust-domain=cluster.local"
      
                # server
                - "--max-client-certificate-duration=1h"
                - "--serving-address=0.0.0.0:6443"
      
                # controller
                - "--configmap-namespace-selector=maistra.io/member-of=istio-system"
                - "--leader-election-namespace=istio-system"
              volumeMounts:
                - mountPath: /var/run/secrets/istio-csr
                  name: root-ca
            volumes:
            - name: root-ca
              secret:
                secretName: istio-csr-ca
      ```

## Support Procedures

None

## Alternatives

Instead of having `cert-manager-operator` manage `istio-csr`, having `istio-csr` itself as a product was
considered. But since `istio-csr` is an agent dependent on `cert-manager` for obtaining the certificates
and the configurations supported are specific to service mesh, approach described in this proposal was
considered to be logical.

## Infrastructure Needed [optional]

None.
