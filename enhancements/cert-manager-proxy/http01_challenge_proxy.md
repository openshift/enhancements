---
title: http01_challenge_proxy
authors:
  - "@sebrandon1"
reviewers:
  - "@JoelSpeed"
  - "@everettraven"
approvers:
  - "@everettraven"
  - "@JoelSpeed"
  - "@benluddy"
  - "@p0lyn0mial"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-03-28
last-updated: 2025-03-28
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/CNF-13731
---

# HTTP01 Challenge Proxy for Cert Manager

![HTTP01 Challenge Proxy Diagram](http01_challenge.png)

## Summary

For baremetal platforms only.  Provide a way for cert-manager to complete http01 challenges against API endpoints (such as api.cluster.example.com) similar to the way it handles certificate challenges for other OpenShift Ingress endpoints.

## Motivation

Cert manager can be used to issue certificates for the OpenShift Container Platform (OCP) endpoints (e.g., console, downloads, oauth) using an external ACME Certificate Authority (CA). These endpoints are exposed via the OpenShift Ingress (`*.apps.cluster.example.com`), and this is a supported and functional configuration today.

However, cluster administrators often want to use Cert Manager to issue custom certificates for the API endpoint (`api.cluster.example.com`). Unlike other endpoints, this API endpoint is not exposed via the OpenShift Ingress. Depending on the OCP topology (e.g., SNO, MNO, Compact), it is exposed directly on the node or via a keepalive VIP. This lack of management by the OpenShift Ingress introduces challenges in obtaining certificates using an external ACME CA. While this challenge exists on all platforms, cloud providers typically offer DNS01 integrations that provide alternative certificate acquisition methods, making this solution primarily beneficial for baremetal environments.

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

The HTTP01 Challenge Proxy will be implemented as a **core component of the OpenShift payload** and managed by an existing cluster operator, specifically the **cluster-kube-apiserver-operator**, which already manages API endpoint configuration and lifecycle.

**Deployment Architecture:**
- **Core Integration**: This feature is part of the core OpenShift distribution, not an optional OLM-installable operator
- **Operator Management**: The cluster-kube-apiserver-operator will deploy and manage the proxy DaemonSet
- **Conditional Deployment**: The proxy is only deployed when `APIServer.spec.http01ChallengeProxy.mode` is set to `DefaultDeployment` or `CustomDeployment`
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

Rather than creating a new CRD, the HTTP01 Challenge Proxy configuration will be added as a new field to the existing **APIServer** CRD in the [openshift/api](https://github.com/openshift/api) repo. This approach aligns with the pattern used by other API server features like audit, encryption, and TLS configuration.

The configuration will be added to the `APIServerSpec` struct and managed by the cluster-kube-apiserver-operator as part of its normal reconciliation loop. Since the APIServer CRD is already a singleton (named "cluster"), no additional enforcement mechanisms are needed.

**API Changes to config/v1/types_apiserver.go:**

```go
type APIServerSpec struct {
    // ... existing fields ...

    // http01ChallengeProxy contains configuration for the HTTP01 challenge proxy
    // that redirects traffic from the API endpoint on port 80 to ingress routers.
    // This enables cert-manager to perform HTTP01 ACME challenges for API endpoint certificates.
    // +optional
    HTTP01ChallengeProxy *HTTP01ChallengeProxySpec `json:"http01ChallengeProxy,omitempty,omitzero"`
}

// +union
// +kubebuilder:validation:XValidation:rule="self.mode == 'CustomDeployment' ? has(self.customDeployment) : !has(self.customDeployment)",message="customDeployment is required when mode is CustomDeployment and forbidden otherwise"
type HTTP01ChallengeProxySpec struct {
    // mode controls whether the HTTP01 challenge proxy is active and how it should be deployed.
    // DefaultDeployment enables the proxy with default configuration.
    // CustomDeployment enables the proxy with user-specified configuration.
    // +kubebuilder:validation:Enum=DefaultDeployment;CustomDeployment
    // +required
    // +unionDiscriminator
    Mode string `json:"mode"`

    // customDeployment contains configuration options when mode is CustomDeployment.
    // This field is only valid when mode is CustomDeployment.
    // +optional
    // +unionMember
    CustomDeployment *HTTP01ChallengeProxyCustomDeploymentSpec `json:"customDeployment,omitempty,omitzero"`
}

type HTTP01ChallengeProxyCustomDeploymentSpec struct {
    // internalPort specifies the internal port used by the proxy service.
    // Valid values are 1024-65535.
    // +kubebuilder:validation:Minimum=1024
    // +kubebuilder:validation:Maximum=65535
    // +required
    InternalPort int32 `json:"internalPort"`
}
```

**API Design Notes:**

This discriminated union approach provides several benefits:
- **Future extensibility**: Additional deployment modes can be added (e.g., "HighAvailabilityDeployment")
- **Mode-specific configuration**: Each mode can have its own configuration options
- **Clear API semantics**: Configuration is only allowed under specific modes of operation
- **Required mode**: Since the parent field is optional, the mode field is required when the parent is specified

**Example Configuration:**

```yaml
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  # existing fields...
  http01ChallengeProxy:
    mode: DefaultDeployment
---
apiVersion: config.openshift.io/v1
kind: APIServer
metadata:
  name: cluster
spec:
  # existing fields...
  http01ChallengeProxy:
    mode: CustomDeployment
    customDeployment:
      internalPort: 8888
status:
  # existing status fields...
  conditions:
    - type: HTTP01ChallengeProxyReady
      status: "True"
      lastTransitionTime: "2025-05-12T00:00:00Z"
      reason: "Initialized"
      message: "HTTP01ChallengeProxy is ready"
```

This design ensures that:
- Only one proxy configuration can exist cluster-wide (enforced by the singleton nature of the APIServer CRD)
- The cluster-kube-apiserver-operator manages this configuration as part of the existing APIServer resource

**Note on Validation**: Unlike designs that use separate CRDs, this approach does not require a ValidatingAdmissionPolicy (VAP) to enforce singleton behavior. The APIServer CRD is inherently a cluster singleton resource (there is exactly one APIServer resource named "cluster" per cluster), which naturally prevents multiple configurations from existing.

### Implementation Details/Notes/Constraints

- The **cluster-kube-apiserver-operator** will be responsible for deploying and managing the proxy as a DaemonSet on control plane nodes that may host the API VIP.
- **Deployment Conditions**: The operator deploys the proxy DaemonSet only when:
  - `APIServer.spec.http01ChallengeProxy.mode` is set to `DefaultDeployment` or `CustomDeployment` in the cluster's APIServer configuration
  - The cluster is running on a baremetal or supported platform (not cloud platforms with integrated DNS01 support)
  - The operator validates that nftables/netfilter subsystem is available on target nodes
- **Component Integration**: The operator will integrate the proxy lifecycle with existing API server management, ensuring consistent behavior and upgrade/downgrade strategies.
- **Singleton Enforcement**: Only one proxy configuration per cluster is supported, enforced by the singleton nature of the APIServer CRD.
- **MCO Integration**: The cluster-kube-apiserver-operator handles MachineConfig creation for nftables configuration. Since this feature will only be available in OCP 4.20+, the operator can safely assume modern MCO capabilities and configure the nftables service to restart rather than reboot nodes when `/etc/sysconfig/nftables.conf` is modified.
- The implementation relies on `nftables` for traffic redirection, which must be supported and enabled on the cluster nodes.
- The demo deployment manifest for the proxy is available [here](https://github.com/mvazquezc/cert-mgr-http01-proxy/blob/main/manifests/deploy-in-ocp.yaml).
- An example implementation can be found in this [repository](https://github.com/mvazquezc/cert-mgr-http01-proxy/tree/main).
- The proxy will listen on a configurable port (default: 8888) for HTTP01 challenge traffic. The port can be set via the APIServer CR's `http01ChallengeProxy.customDeployment.internalPort` field to avoid conflicts with other workloads that may require port 8888 on the host.
- 8888 was chosen as a reasonable default because it is commonly unused, but clusters with a conflict can override this value in the APIServer configuration.
- The [host port registry](https://github.com/openshift/enhancements/blob/master/dev-guide/host-port-registry.md) should be updated to reflect the use of port 80 on apiServer nodes when this feature is enabled, to avoid conflicts and ensure proper documentation of port usage.
- The priority (order) of the `nftables` entries relative to other services should be coordinated with the OpenShift networking team to ensure it follows established precedent and does not interfere with other networking rules.
- **Host Network Requirement**: The proxy DaemonSet must use `hostNetwork: true` because nftables rules can only redirect traffic between addresses within the same network namespace. Since the API VIP resides on the host network interface, the proxy listener must also be in the host network namespace for the traffic redirection to function. Running the proxy in a CNI network would make it unreachable from the nftables rules that redirect API VIP traffic.

#### nftables Presence and Management

- `nftables` is always present as part of the RHCOS payload. Baremetal and AWS (at least) OCP clusters install `nftables` tables, chains, and rules by default.
- The `nftables` systemd unit is disabled by default, but the netfilter subsystem is active and can be configured via the `nft` CLI/API without enabling the systemd unit.
- OVN-Kubernetes relies on netfilter (iptables/nftables) for features like UDN, Egress, and Services.
- The Machine Config Operator (MCO) does not manage `nftables` directly, but users can explicitly disable or modify `nftables` via MachineConfig if desired.
- If a user disables or removes `nftables`, this is considered an explicit user-driven action and is not managed or expected by OpenShift.
- If `nftables` is not present or is disabled, the proxy will not function as intended. Detection of this condition should be implemented (e.g., by checking for the presence of the `nft` binary and ability to apply rules), and the operator should surface a clear error or degraded status.
- Based on current RHCOS and OCP design, it is not possible to remove the underlying netfilter subsystem, so the feature can reliably depend on its presence unless a user takes explicit unsupported action.

### Design Details

**Scope**: This proxy **only** affects external HTTP traffic (port 80) directed to the API VIP that resolves `api.cluster.example.com`. Internal cluster communication, service discovery, and HTTPS traffic (port 443) are completely unaffected.

- **Operator Integration**: The cluster-kube-apiserver-operator will manage the complete lifecycle of the HTTP01 challenge proxy, including deployment, configuration, upgrades, and removal.
- **Proxy Deployment**: The operator will deploy the proxy feature as a DaemonSet on control plane nodes. The daemonset will implement nftable rules via pods that run to completion.
- **Traffic Redirection**: This will use `nftables` rules to redirect incoming external HTTP traffic on `API:80` to `PROXY:8888`.
- **Security**: The proxy implements selective traffic filtering, only handling external HTTP requests with paths matching `/.well-known/acme-challenge/*` destined for the API VIP. 
  - **ACME Traffic**: Requests to `<API_VIP>/.well-known/acme-challenge/*` are redirected to the ingress routers for certificate validation.
    - **Valid ACME Data**: Properly formatted challenge requests are forwarded to cert-manager challenge pods for validation.
    - **Invalid ACME Data**: Malformed challenge requests to the correct path are still forwarded to cert-manager, which handles the validation failure (same behavior as without the proxy).
  - **Non-ACME Traffic**: HTTP requests to the API VIP on port 80 with other paths receive a **400 Bad Request** response with the message "Only /.well-known/acme-challenge/* is allowed".
  - **HTTPS Traffic**: All HTTPS traffic (port 443) to the API endpoint continues to function normally and is unaffected by the proxy.
- **Monitoring**: Logs and metrics will be exposed to help administrators monitor the proxy's behavior and troubleshoot issues.
- **Configuration Management**: The operator will watch the APIServer CR for `http01ChallengeProxy` configuration changes and reconcile the proxy state accordingly.

### Drawbacks

1. **Dependency on nftables**: The solution relies on `nftables`, which may not be available or enabled on all environments.
2. **Additional Resource Usage**: Running the proxy as a DaemonSet introduces additional resource usage on the cluster nodes while the proxy pod is applying its nftable rules.
3. **Complexity**: The solution adds another component to the cluster, which may increase operational complexity.

## Alternatives (Not Implemented)

**Important Note**: The alternatives listed below address **certificate management at scale** (how to deploy and manage cert-manager across multiple clusters), which is a different problem than what this enhancement solves. **All of these alternatives would still encounter the same fundamental gap** that the HTTP01 Challenge Proxy addresses: **the inability to complete HTTP01 challenges for API endpoints that are not exposed via OpenShift Ingress**.

Regardless of which certificate management approach is chosen (centralized, distributed, hub-and-spoke, etc.), the underlying HTTP01 challenge mechanism for API endpoints would still fail without the traffic redirection solution provided by this enhancement. **The HTTP01 proxy is complementary to these approaches, not competing with them.**

The alternatives were actually implemented if you look through the presentation [slides](https://docs.google.com/presentation/d/1mJ1pnsPiEwb-U5lHwhM2UkyRmkkLeYxj3cfE4F7dOx0/edit#slide=id.g547716335e_0_260) but the approaches are all listed below.

1. **RHACM Manages Cert Manager Deployment**: RHACM (Red Hat Advanced Cluster Management) manages the deployment of Cert Manager and certificates on the spokes using Policies. Each managed cluster runs its own Cert Manager instance. This approach decentralizes certificate management but requires Cert Manager to be deployed and maintained on each spoke cluster.

2. **Single Addon on the Hub**: A single addon runs on the hub and watches the spoke clusters' APIs for `Certificate` and `CertificateRequest` related events. When these APIs are created, updated, or deleted in the spoke, the addon syncs the contents back and forth between the hub and the spokes. This approach centralizes management but introduces additional complexity in syncing data.

3. **Cert Manager Controller per Spoke**: A Cert Manager controller is configured for each spoke cluster on the hub. These controllers run in the spoke cluster namespace and are configured to use the spoke’s `system:admin` kubeconfig. This approach allows centralized control but requires managing multiple controllers on the hub.

4. **Single Cert Manager Controller on the Hub**: A single Cert Manager controller runs on the hub. Certificates and `CertificateRequests` for each spoke cluster are created with data known beforehand (e.g., API, Ingress, CNFs). The resulting secrets are synced to the spokes via RHACM Policies. This approach simplifies the deployment but requires pre-configured data for each spoke.

More information about the investigation can be found [here](https://docs.google.com/presentation/d/1mJ1pnsPiEwb-U5lHwhM2UkyRmkkLeYxj3cfE4F7dOx0/edit#slide=id.g547716335e_0_260).

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

**General Mitigation**: For both risks above, administrators can disable the HTTP01 challenge proxy entirely by removing the `http01ChallengeProxy` configuration from the APIServer CR or setting the `mode` to an empty string. This will disable the proxy functionality and revert to standard API endpoint behavior, though HTTP01 challenges for the API endpoint will no longer be possible.

### Implementation History

- **2025-03-28**: Enhancement proposal created.

### References

- [Cert Manager Expansion JIRA Epic](https://issues.redhat.com/browse/CNF-13731)
- [ACME HTTP01 Challenge](https://letsencrypt.org/docs/challenge-types/#http-01-challenge)
- [Proxy Code Repository](https://github.com/mvazquezc/cert-mgr-http01-proxy/tree/main)
- [Deployment Manifest](https://github.com/mvazquezc/cert-mgr-http01-proxy/blob/main/manifests/deploy-in-ocp.yaml)

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

#### Recovery Procedures

If the proxy DaemonSet enters a `CrashLoopBackOff` state, HTTP01 challenges for the API endpoint will fail, and certificate renewal will not complete. This may result in the API certificate expiring, which could impact cluster operations.

**Recovery steps:**
- Cluster administrators can disable the proxy by updating the APIServer CR to remove the `http01ChallengeProxy` configuration or set the `mode` to an empty string. Deleting the DaemonSet directly will not work as the cluster operator will recreate it.
- If the API certificate has expired, recovery may require connecting to the cluster's API server while ignoring certificate validation errors (e.g., using `--insecure-skip-tls-verify` with `oc` or `kubectl`).
- After resolving the issue (e.g., fixing the DaemonSet, node configuration, or proxy image), re-deploy the proxy to restore HTTP01 challenge functionality.
- The proxy should surface clear status and error messages to help identify and resolve CrashLoopBackOff or degraded states.

## Support Procedures

### Detecting Failure Modes

- **Symptoms**: 
  - Cert Manager HTTP01 challenges for the API endpoint (`api.cluster.example.com`) fail to complete.
  - Certificates for the API endpoint are not issued or renewed.
  - The proxy DaemonSet pods are in `CrashLoopBackOff` or `Error` state.
  - Events in the `openshift-cert-manager` or relevant namespace indicate pod failures.
  - The operator managing the proxy (if any) reports a degraded or error status.
  - Logs from the proxy pod show errors related to `nftables` or port binding.
  - The API server logs may show failed ACME challenge requests or timeouts.

- **Metrics/Alerts**:
  - Custom metrics (if implemented) such as `cert_manager_proxy_up` or `cert_manager_proxy_errors_total` may indicate proxy health.
  - Alerts can be configured for DaemonSet unavailability or excessive restarts.

### Disabling the API Extension

- **How to disable**: 
  - Remove or scale down the proxy DaemonSet.
  - Update the APIServer CR to set `http01ChallengeProxy.mode` to an empty string or remove the `http01ChallengeProxy` section entirely.
  - Remove any MachineConfig or configuration that enables the proxy.
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
