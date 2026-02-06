---
title: http01_challenge_proxy
authors:
  - "@sebrandon1"
reviewers:
  - "@chiragkyal"
  - "@bharath-b-rh"
  - "@mytreya-rh"
approvers:
  - "@mytreya-rh"
  - "@tgeer"
api-approvers:
  - "@tgeer"
creation-date: 2025-03-28
last-updated: 2026-01-26
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/CNF-18992
see-also:
  - enhancements/cert-manager/istio-csr-controller.md
---

# HTTP01 Challenge Proxy for Cert Manager

![HTTP01 Challenge Proxy Diagram](http01_challenge.png)

## Summary

For baremetal platforms only.  Provide a way for cert-manager to complete http01 challenges against API endpoints (such as api.cluster.example.com) similar to the way it handles certificate challenges for other OpenShift Ingress endpoints.

## Motivation

Cert manager can be used to issue certificates for the OpenShift Container Platform (OCP) endpoints (e.g., console, downloads, oauth) using an external ACME Certificate Authority (CA). These endpoints are exposed via the OpenShift Ingress (`*.apps.cluster.example.com`), and this is a supported and functional configuration today.

However, cluster administrators often want to use Cert Manager to issue custom certificates for the API endpoint (`api.cluster.example.com`). Unlike other endpoints, this API endpoint is not exposed via the OpenShift Ingress. Depending on the OCP topology (e.g., SNO, MNO, Compact), it is exposed directly on the node or via a keepalive VIP.

This lack of management by the OpenShift Ingress introduces challenges in obtaining certificates using an external ACME CA. While this challenge exists on all platforms, cloud providers typically offer DNS01 integrations that provide alternative certificate acquisition methods, making this solution primarily beneficial for baremetal environments.

The gap arises due to how the ACME HTTP01 challenge works. The following scenarios illustrate the challenges:

1. **Standard Clusters**: The API VIP is hosted on the control plane nodes which do not host an OpenShift Router. The http01 challenge, which is directed at the API VIP (the IP where `api.cluster.example.com` DNS resolves), will not hit an OpenShift Router and thus not reach the challenge response pod started by Cert Manager.
2. **Compact Clusters**: The node hosting the API VIP may also host an OpenShift Router. If no router is present on the node hosting the VIP, the challenge will fail.
3. **SNO (Single Node OpenShift)**: The same nodes host both the ingress and API components. Both FQDNs (`api` and wildcard) resolve to the same IP, making the challenge feasible.

To address this gap, a small proxy was developed. This proxy runs on the cluster as a DaemonSet (control plane nodes) and then adds iptables rules to the nodes and ensures that connections reaching the API on port 80 are redirected to the OpenShift Ingress Routers. The proxy implementation creates a reverse proxy to the apps VIP and uses `nftables` to redirect traffic from `API:80` to `PROXY:8888`.

- **Proxy Code**: [GitHub Repository](https://github.com/mvazquezc/cert-mgr-http01-proxy/tree/main)
- **Deployment Manifest**: [Manifest Link](https://github.com/mvazquezc/cert-mgr-http01-proxy/blob/main/manifests/deploy-in-ocp.yaml)

This enhancement aims to provide a robust solution for managing certificates for the API endpoint, particularly for baremetal customers using non cloud-ready DNS servers. Cloud providers typically offer cloud provider DNS services that integrate directly with cert-manager for DNS01 challenges, but baremetal environments often lack these integrations and rely on HTTP01 challenges instead.

### User Stories

1. **As a cluster administrator**, I want to manage custom certificates for the API endpoint (`api.cluster.example.com`) using an external ACME CA, so that I can ensure secure communication for my cluster's API.
2. **As a cluster administrator on a baremetal platform**, I want a reliable solution to handle HTTP01 challenges for the API endpoint, even when the endpoint is not managed by OpenShift Ingress, so that I can avoid manual workarounds.
3. **As a developer**, I want a simple deployment mechanism for the HTTP01 challenge proxy, so that I can easily integrate it into my existing cluster setup.

### Goals

- Provide a reliable mechanism for Cert Manager to complete HTTP01 challenges for the API endpoint (`api.cluster.example.com`) in baremetal environments.
- Ensure compatibility with various OpenShift topologies, including Standard Clusters, Compact Clusters, and SNO.
- Low operational complexity to secure communication with a cluster's API endpoint.

### Non-Goals

- This enhancement does not aim to replace or modify the existing OpenShift Ingress functionality.
- It does not provide support for non-HTTP01 challenge types (e.g., DNS-01).
- It does not address certificate management for endpoints other than the API endpoint (`api.cluster.example.com`).
- It does not provide a solution for environments where `nftables` is not supported.

## Proposal

The HTTP01 Challenge Proxy will be implemented as a controller within the **cert-manager-operator**, following the pattern established by the istio-csr-controller. The feature will be managed via a new CRD `http01proxies.operator.openshift.io`.

**Deployment Architecture:**
- **Operator Integration**: This feature is managed by the cert-manager-operator, not as part of the core OpenShift payload
- **Day-2 Feature**: The proxy is an optional feature that can be enabled as a day-2 operation after cert-manager-operator installation
- **Conditional Deployment**: The proxy is only deployed when an `HTTP01Proxy` CR is created with `mode` set to `DefaultDeployment` or `CustomDeployment`
- **Platform Targeting**: Limited to baremetal platforms where HTTP01 challenges are needed for API certificates

**Operational Responsibilities:**
- Deploy and manage a DaemonSet running on control plane nodes that may host the API VIP
- **Traffic Filtering**: Only redirect HTTP traffic destined for `<API_VIP>/.well-known/acme-challenge/*` paths (ACME HTTP01 challenge traffic)
- **Path Validation**: Use path-based filtering rather than SNI (Server Name Indication) to identify HTTP01 challenge requests
- Redirect filtered HTTP traffic from the API endpoint (`api.cluster.example.com`) on port 80 to the OpenShift Ingress Routers
- Use `nftables` for traffic redirection from `API:80` to `PROXY:8888`
- Handle lifecycle management, upgrades, and configuration of the proxy

The proxy will ensure compatibility with various OCP topologies, including SNO, MNO, and Compact clusters, addressing the challenges of HTTP01 validation for the API endpoint.

### API Extensions

A new CRD `http01proxies.operator.openshift.io` will be added to the cert-manager-operator API, following the pattern established by `istiocsrs.operator.openshift.io`. This approach keeps the feature within the cert-manager-operator domain and allows it to be deployed as an optional day-2 feature.

**API Definition in api/operator/v1alpha1/:**

```go
// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=http01proxies,scope=Namespaced,categories={cert-manager-operator},shortName=http01proxy

// HTTP01Proxy describes the configuration for the HTTP01 challenge proxy
// that redirects traffic from the API endpoint on port 80 to ingress routers.
// This enables cert-manager to perform HTTP01 ACME challenges for API endpoint certificates.
// The name must be `default` to make HTTP01Proxy a singleton.
//
// When an HTTP01Proxy is created, the proxy DaemonSet is deployed on control plane nodes.
//
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'default'",message="http01proxy is a singleton, .metadata.name must be 'default'"
// +operator-sdk:csv:customresourcedefinitions:displayName="HTTP01Proxy"
type HTTP01Proxy struct {
    metav1.TypeMeta `json:",inline"`

    // metadata is the standard object's metadata.
    // More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
    metav1.ObjectMeta `json:"metadata,omitempty"`

    // spec is the specification of the desired behavior of the HTTP01Proxy.
    // +kubebuilder:validation:Required
    // +required
    Spec HTTP01ProxySpec `json:"spec"`

    // status is the most recently observed status of the HTTP01Proxy.
    // +kubebuilder:validation:Optional
    // +optional
    Status HTTP01ProxyStatus `json:"status,omitempty"`
}

// HTTP01ProxySpec is the specification of the desired behavior of the HTTP01Proxy.
// +kubebuilder:validation:XValidation:rule="self.mode == 'CustomDeployment' ? has(self.customDeployment) : !has(self.customDeployment)",message="customDeployment is required when mode is CustomDeployment and forbidden otherwise"
type HTTP01ProxySpec struct {
    // mode controls whether the HTTP01 challenge proxy is active and how it should be deployed.
    // DefaultDeployment enables the proxy with default configuration.
    // CustomDeployment enables the proxy with user-specified configuration.
    // +kubebuilder:validation:Enum=DefaultDeployment;CustomDeployment
    // +required
    Mode string `json:"mode"`

    // customDeployment contains configuration options when mode is CustomDeployment.
    // This field is only valid when mode is CustomDeployment.
    // +optional
    CustomDeployment *HTTP01ProxyCustomDeploymentSpec `json:"customDeployment,omitempty"`
}

// HTTP01ProxyCustomDeploymentSpec contains configuration for custom proxy deployment.
type HTTP01ProxyCustomDeploymentSpec struct {
    // internalPort specifies the internal port used by the proxy service.
    // Valid values are 1024-65535.
    // +kubebuilder:validation:Minimum=1024
    // +kubebuilder:validation:Maximum=65535
    // +kubebuilder:default=8888
    // +optional
    InternalPort int32 `json:"internalPort,omitempty"`
}

// HTTP01ProxyStatus is the most recently observed status of the HTTP01Proxy.
type HTTP01ProxyStatus struct {
    // conditions holds information about the current state of the HTTP01 proxy deployment.
    // +patchMergeKey=type
    // +patchStrategy=merge
    // +listType=map
    // +listMapKey=type
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // proxyImage is the name of the image and the tag used for deploying the proxy.
    ProxyImage string `json:"proxyImage,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// HTTP01ProxyList is a list of HTTP01Proxy objects.
type HTTP01ProxyList struct {
    metav1.TypeMeta `json:",inline"`

    // metadata is the standard list's metadata.
    // More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#metadata
    metav1.ListMeta `json:"metadata"`
    Items           []HTTP01Proxy `json:"items"`
}
```

**Example Configuration:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: HTTP01Proxy
metadata:
  name: default
  namespace: cert-manager-operator
spec:
  mode: DefaultDeployment
---
apiVersion: operator.openshift.io/v1alpha1
kind: HTTP01Proxy
metadata:
  name: default
  namespace: cert-manager-operator
spec:
  mode: CustomDeployment
  customDeployment:
    internalPort: 8888
```

**Status Conditions:**

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2025-05-12T00:00:00Z"
      reason: "ProxyDeployed"
      message: "HTTP01 Challenge Proxy is ready"
    - type: Degraded
      status: "False"
      lastTransitionTime: "2025-05-12T00:00:00Z"
      reason: "ProxyHealthy"
      message: "All proxy pods are healthy"
```

This design ensures that:
- Only one proxy configuration can exist per namespace (enforced by the singleton validation on the CR name)
- The cert-manager-operator manages this configuration following the same pattern as IstioCSR
- The feature is optional and can be enabled/disabled as a day-2 operation

### Implementation Details/Notes/Constraints

- The **cert-manager-operator** will be responsible for deploying and managing the proxy as a DaemonSet on control plane nodes that may host the API VIP.
- **Controller Location**: The controller will be implemented in `pkg/controller/http01proxy/` following the pattern of the istio-csr-controller.
- **Static Manifests**: Static manifest templates will be stored in `bindata/http01-proxy-deployment/` and compiled into bindata via `make update-bindata`.
- **Deployment Conditions**: The operator deploys the proxy DaemonSet only when:
  - An `HTTP01Proxy` CR named `default` exists in the cert-manager-operator namespace with `mode` set to `DefaultDeployment` or `CustomDeployment`
  - The operator validates that nftables/netfilter subsystem is available on target nodes
- **Singleton Enforcement**: Only one proxy configuration per namespace is supported, enforced by CEL validation on the CR name (must be `default`).
- The implementation relies on `nftables` for traffic redirection, which must be supported and enabled on the cluster nodes.
- The [demo deployment manifest](https://github.com/mvazquezc/cert-mgr-http01-proxy/blob/main/manifests/deploy-in-ocp.yaml) for the proxy is available.
- An example implementation can be found in this [repository](https://github.com/mvazquezc/cert-mgr-http01-proxy/tree/main).
- The proxy will listen on a configurable port (default: 8888) for HTTP01 challenge traffic. The port can be set via the HTTP01Proxy CR's `customDeployment.internalPort` field to avoid conflicts with other workloads that may require port 8888 on the host.
- 8888 was chosen as a reasonable default because it is commonly unused, but clusters with a conflict can override this value in the HTTP01Proxy configuration.
- The [host port registry](https://github.com/openshift/enhancements/blob/master/dev-guide/host-port-registry.md) should be updated to reflect the use of port 80 on apiServer nodes when this feature is enabled, to avoid conflicts and ensure proper documentation of port usage.
- The priority (order) of the `nftables` entries relative to other services should be coordinated with the OpenShift networking team to ensure it follows established precedent and does not interfere with other networking rules.
- **Host Network Requirement**: The proxy DaemonSet must use `hostNetwork: true` because nftables rules can only redirect traffic between addresses within the same network namespace. Since the API VIP resides on the host network interface, the proxy listener must also be in the host network namespace for the traffic redirection to function.

#### nftables Presence and Management

- `nftables` is always present as part of the RHCOS payload. Baremetal and AWS (at least) OCP clusters install `nftables` tables, chains, and rules by default.
- The `nftables` systemd unit is disabled by default, but the netfilter subsystem is active and can be configured via the `nft` CLI/API without enabling the systemd unit.
- OVN-Kubernetes relies on netfilter (iptables/nftables) for features like UDN, Egress, and Services.
- If a user disables or removes `nftables`, this is considered an explicit user-driven action and is not managed or expected by OpenShift.
- If `nftables` is not present or is disabled, the proxy will not function as intended. Detection of this condition should be implemented (e.g., by checking for the presence of the `nft` binary and ability to apply rules), and the operator should surface a clear error or degraded status.
- Based on current RHCOS and OCP design, it is not possible to remove the underlying netfilter subsystem, so the feature can reliably depend on its presence unless a user takes explicit unsupported action.

### Design Details

**Scope**: This proxy **only** affects external HTTP traffic (port 80) directed to the API VIP that resolves `api.cluster.example.com`. Internal cluster communication, service discovery, and HTTPS traffic (port 443) are completely unaffected.

- **Operator Integration**: The cert-manager-operator will manage the complete lifecycle of the HTTP01 challenge proxy, including deployment, configuration, upgrades, and removal, following the same pattern as the istio-csr-controller.
- **Proxy Deployment**: The operator will deploy the proxy feature as a DaemonSet on control plane nodes. The daemonset will implement nftable rules via pods that run to completion.
- **Traffic Redirection**: This will use `nftables` rules to redirect incoming external HTTP traffic on `API:80` to `PROXY:8888`.
- **Security**: The proxy implements selective traffic filtering, only handling external HTTP requests with paths matching `/.well-known/acme-challenge/*` destined for the API VIP.
  - **ACME Traffic**: Requests to `<API_VIP>/.well-known/acme-challenge/*` are redirected to the ingress routers for certificate validation.
    - **Valid ACME Data**: Properly formatted challenge requests are forwarded to cert-manager challenge pods for validation.
    - **Invalid ACME Data**: Malformed challenge requests to the correct path are still forwarded to cert-manager, which handles the validation failure (same behavior as without the proxy).
  - **Non-ACME Traffic**: HTTP requests to the API VIP on port 80 with other paths receive a **400 Bad Request** response with the message "Only /.well-known/acme-challenge/* is allowed".
  - **HTTPS Traffic**: All HTTPS traffic (port 443) to the API endpoint continues to function normally and is unaffected by the proxy.
- **Monitoring**: Logs and metrics will be exposed to help administrators monitor the proxy's behavior and troubleshoot issues.
- **Configuration Management**: The operator will watch the HTTP01Proxy CR for configuration changes and reconcile the proxy state accordingly.

### Drawbacks

1. **Dependency on nftables**: The solution relies on `nftables`, which may not be available or enabled on all environments.
2. **Additional Resource Usage**: Running the proxy as a DaemonSet introduces additional resource usage on the cluster nodes while the proxy pod is applying its nftable rules.
3. **Complexity**: The solution adds another component to the cluster, which may increase operational complexity.

## Alternatives (Not Implemented)

**Important Note**: The alternatives listed below address **certificate management at scale** (how to deploy and manage cert-manager across multiple clusters), which is a different problem than what this enhancement solves.

**All of these alternatives would still encounter the same fundamental gap** that the HTTP01 Challenge Proxy addresses: **the inability to complete HTTP01 challenges for API endpoints that are not exposed via OpenShift Ingress.**

Regardless of which certificate management approach is chosen (centralized, distributed, hub-and-spoke, etc.), the underlying HTTP01 challenge mechanism for API endpoints would still fail without the traffic redirection solution provided by this enhancement. **The HTTP01 proxy is complementary to these approaches, not competing with them.**

The alternatives were actually implemented if you look through the presentation [slides](https://docs.google.com/presentation/d/1mJ1pnsPiEwb-U5lHwhM2UkyRmkkLeYxj3cfE4F7dOx0/edit#slide=id.g547716335e_0_260) but the approaches are all listed below.

1. **RHACM Manages Cert Manager Deployment**: RHACM (Red Hat Advanced Cluster Management) manages the deployment of Cert Manager and certificates on the spokes using Policies. Each managed cluster runs its own Cert Manager instance. This approach decentralizes certificate management but requires Cert Manager to be deployed and maintained on each spoke cluster.

2. **Single Addon on the Hub**: A single addon runs on the hub and watches the spoke clusters' APIs for `Certificate` and `CertificateRequest` related events. When these APIs are created, updated, or deleted in the spoke, the addon syncs the contents back and forth between the hub and the spokes. This approach centralizes management but introduces additional complexity in syncing data.

3. **Cert Manager Controller per Spoke**: A Cert Manager controller is configured for each spoke cluster on the hub. These controllers run in the spoke cluster namespace and are configured to use the spoke's `system:admin` kubeconfig. This approach allows centralized control but requires managing multiple controllers on the hub.

4. **Single Cert Manager Controller on the Hub**: A single Cert Manager controller runs on the hub. Certificates and `CertificateRequests` for each spoke cluster are created with data known beforehand (e.g., API, Ingress, CNFs). The resulting secrets are synced to the spokes via RHACM Policies. This approach simplifies the deployment but requires pre-configured data for each spoke.

More information about the investigation can be found in the [certificate management investigation slides](https://docs.google.com/presentation/d/1mJ1pnsPiEwb-U5lHwhM2UkyRmkkLeYxj3cfE4F7dOx0/edit#slide=id.g547716335e_0_260).

### Risks and Mitigations

1. **Proxy Failure**: If the proxy fails, HTTP01 challenges for the API endpoint will not succeed. This does **not** prevent users from accessing their cluster, but may result in the API certificate expiring if not renewed.
   - **User Impact**:
     - End-users and system components can still access the API endpoint, but may encounter certificate warnings or errors if the certificate is expired or invalid.
     - Users may need to accept insecure connections (e.g., use `--insecure-skip-tls-verify` with `oc` or `kubectl`) until the certificate is renewed.
     - Automated systems or integrations that require valid certificates may fail or refuse to connect.
   - **Remediation/Workaround**:
     - Restore the proxy DaemonSet and ensure it is healthy so Cert Manager can retry and complete HTTP01 challenges.
     - If the API certificate has expired, use insecure connection flags to access the cluster and perform remediation.
     - Monitor for certificate expiry and configure alerts to notify administrators before expiry occurs.
   - **Detection**: Configure alerts for expiring certificates. Warning alerts should be triggered when certificates are close to expiry, and critical alerts when certificates have expired. This allows administrators to take action before cluster API access is impacted.

2. **Traffic Interference**: The proxy could inadvertently interfere with other traffic. Mitigation: Carefully scope the proxy's functionality to only handle HTTP01 challenge traffic.

**General Mitigation**: For both risks above, administrators can disable the HTTP01 challenge proxy entirely by deleting the `HTTP01Proxy` CR. This will disable the proxy functionality and revert to standard API endpoint behavior, though HTTP01 challenges for the API endpoint will no longer be possible.

### Implementation History

- **2025-03-28**: Enhancement proposal created.
- **2026-01-26**: Updated to use cert-manager-operator CRD approach following istio-csr-controller pattern.

### References

- [Cert Manager Expansion JIRA Epic](https://issues.redhat.com/browse/CNF-18992)
- [ACME HTTP01 Challenge](https://letsencrypt.org/docs/challenge-types/#http-01-challenge)
- [Proxy Code Repository](https://github.com/mvazquezc/cert-mgr-http01-proxy/tree/main)
- [Deployment Manifest](https://github.com/mvazquezc/cert-mgr-http01-proxy/blob/main/manifests/deploy-in-ocp.yaml)
- [Istio-CSR Controller Enhancement](istio-csr-controller.md)

### Workflow Description

1. Cert Manager initiates an HTTP01 challenge for the API endpoint (`api.cluster.example.com`).
2. The HTTP01 challenge request is directed to the API VIP on port 80 with the path `/.well-known/acme-challenge/<token>`.
3. **Traffic Identification**: The HTTP01 Challenge Proxy identifies this as ACME challenge traffic by examining the destination path (`/.well-known/acme-challenge/*`) rather than using SNI validation.
4. **Selective Redirection**: Only traffic matching the ACME challenge path pattern is intercepted using `nftables` and redirected to the proxy pod on port 8888. Other HTTP requests to the API VIP on port 80 receive a **400 Bad Request** response.
5. The proxy pod forwards the filtered request to the OpenShift Ingress Router, which serves the challenge response from the Cert Manager challenge pod.
6. The ACME CA validates the challenge and issues the certificate for the API endpoint.

### Topology Considerations

- **Standard Clusters**: The API VIP is hosted on control plane nodes. The proxy ensures that HTTP01 challenges are redirected to the OpenShift Ingress Routers.
- **Compact Clusters**: The proxy handles scenarios where the API VIP node may or may not host an OpenShift Router, ensuring consistent challenge redirection.
- **SNO (Single Node OpenShift)**: The proxy is not strictly required in this topology, as the API and wildcard FQDNs resolve to the same IP. However, it can still be deployed for consistency.

#### Hypershift / Hosted Control Planes

This enhancement does not directly apply to Hypershift deployments, as the API endpoint management in Hypershift differs from baremetal environments. However, the proxy's design could be adapted for similar use cases in Hypershift if needed.

#### Standalone Clusters

For standalone clusters, the proxy ensures that HTTP01 challenges for the API endpoint are redirected to the OpenShift Ingress Routers, regardless of whether the API VIP node hosts a router.

#### Single-node Deployments or MicroShift

In SNO or MicroShift deployments, the proxy is not strictly required, as the API and wildcard FQDNs resolve to the same IP. However, deploying the proxy ensures consistency and simplifies certificate management.

#### OpenShift Kubernetes Engine

OKE clusters will have full support for the HTTP01 challenge proxy feature.

## Test Plan

1. **Unit Tests**: Validate the proxy's functionality in isolation, including traffic redirection and error handling.
2. **Integration Tests**: Deploy the proxy in a test cluster and verify that HTTP01 challenges for the API endpoint succeed.
3. **Performance Tests**: Measure the proxy's impact on cluster performance and resource usage.
4. **Topology Tests**: Test the proxy in Standard Clusters, Compact Clusters, and SNO environments to ensure compatibility.

## Graduation Criteria

### Dev Preview -> Tech Preview

- The proxy is implemented and tested in development environments.
- Documentation is available for deploying and configuring the proxy.

### Tech Preview -> GA

- The proxy is deployed in production environments and successfully handles HTTP01 challenges for various OCP topologies.
- Performance and reliability meet production-grade requirements.

### Removing a deprecated feature

This enhancement does not deprecate any existing features.

## Upgrade / Downgrade Strategy

- Updated versions of the proxy can be applied to the cluster similar to initial deployment.
- The proxy DaemonSet must use a `Recreate` update strategy to ensure that only one instance of the proxy runs per node at any time, as the proxy listens on a fixed port (`8888`). This prevents port collisions during upgrades.
- Rolling upgrades are not supported due to the singleton nature of the proxy per node; the `Recreate` policy ensures the old pod is terminated before the new one starts.
- If an upgrade fails midway, administrators should roll back to the previous working DaemonSet image or manifest. The cluster will not have a running proxy until the DaemonSet is restored, so HTTP01 challenges will fail during this window.
- The proxy code should maintain backwards compatibility for nftables rules and configuration to minimize upgrade risks.

## Version Skew Strategy

Any changes to the proxy's behavior will be documented to ensure compatibility with older cluster versions.

## Operational Aspects of API Extensions

- **Monitoring**: Logs and metrics will be exposed to help administrators monitor the proxy's behavior and troubleshoot issues.
- **Resource Usage**: The proxy's resource requirements will be minimal, as it only handles HTTP01 challenge traffic.
- **Failure Recovery**: Health checks will ensure that the proxy is running correctly, and failed pods will be automatically restarted.

### Recovery Procedures

If the proxy DaemonSet enters a `CrashLoopBackOff` state, HTTP01 challenges for the API endpoint will fail, and certificate renewal will not complete. This may result in the API certificate expiring, which could impact cluster operations.

**Recovery steps:**
- Cluster administrators can disable the proxy by deleting the `HTTP01Proxy` CR. The operator will then clean up the proxy DaemonSet and related resources.
- If the API certificate has expired, recovery may require connecting to the cluster's API server while ignoring certificate validation errors (e.g., using `--insecure-skip-tls-verify` with `oc` or `kubectl`).
- After resolving the issue (e.g., fixing the DaemonSet, node configuration, or proxy image), re-create the `HTTP01Proxy` CR to restore HTTP01 challenge functionality.
- The proxy should surface clear status and error messages to help identify and resolve CrashLoopBackOff or degraded states.

## Support Procedures

### Detecting Failure Modes

- **Symptoms**:
  - Cert Manager HTTP01 challenges for the API endpoint (`api.cluster.example.com`) fail to complete.
  - Certificates for the API endpoint are not issued or renewed.
  - The proxy DaemonSet pods are in `CrashLoopBackOff` or `Error` state.
  - Events in the `cert-manager-operator` namespace indicate pod failures.
  - The HTTP01Proxy CR status shows `Degraded` condition.
  - Logs from the proxy pod show errors related to `nftables` or port binding.
  - The API server logs may show failed ACME challenge requests or timeouts.

- **Metrics/Alerts**:
  - Custom metrics (if implemented) such as `http01_proxy_up` or `http01_proxy_errors_total` may indicate proxy health.
  - Alerts can be configured for DaemonSet unavailability or excessive restarts.

### Disabling the API Extension

- **How to disable**:
  - Delete the `HTTP01Proxy` CR from the cert-manager-operator namespace.
  - The operator will automatically clean up the proxy DaemonSet and related resources.
- **Consequences**:
  - HTTP01 challenges for the API endpoint will not be possible.
  - Certificates for the API endpoint will not be issued or renewed.
  - If the API certificate expires, cluster API access may be impacted until a valid certificate is restored.

### Impact on Existing, Running Workloads

**Important**: The proxy only affects external API access via the public FQDN (`api.cluster.example.com`) on port 80. Internal cluster communication (e.g., operator pods, system components, pod-to-API server communication via `kubernetes.default.svc.cluster.local` or internal service IPs) is completely unaffected.

- Existing workloads and API traffic will continue to function as long as the API certificate is valid.
- If the external API certificate expires and is not renewed, external clients may fail to connect to the API server due to certificate errors.
- No direct impact on running pods or services, as they typically use internal DNS endpoints and service discovery.
- Internal cluster operations (operators, controllers, etc.) continue to function normally as they do not use the external API endpoint.

### Impact on Newly Created Workloads

- New certificate requests for the external API endpoint (`api.cluster.example.com`) will fail.
- External tools or workloads that specifically require a valid external API certificate for integration may fail to initialize.
- Internal cluster operations and pod scheduling continue normally as they use internal service discovery.

### Graceful Failure and Recovery

- The proxy is not on the critical path for existing API traffic; it only affects HTTP01 challenge completion.
- When the proxy is restored, Cert Manager can retry failed HTTP01 challenges and resume certificate issuance/renewal.
- No risk of data loss or cluster inconsistency; functionality resumes when the proxy is re-enabled and healthy.
- If the API certificate has expired, recovery may require connecting with `--insecure-skip-tls-verify` until a new certificate is issued.

### Listing Resources

To list all resources created for the HTTP01 proxy deployment:
```bash
oc get DaemonSets,Services,ServiceAccounts,ClusterRoles,ClusterRoleBindings -l "app=cert-manager-http01-proxy" -n cert-manager-operator
```
