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
last-updated: 2026-07-17
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

To address this gap, a MachineConfig-based solution was developed. The cert-manager-operator creates a MachineConfig that deploys nftables DNAT rules and a systemd oneshot service on control plane master nodes.
These rules use DNAT (Destination Network Address Translation) to forward TCP port 80 traffic arriving at the API VIP directly to the Ingress VIP, allowing the OpenShift Router to serve the ACME HTTP01 challenge response. MASQUERADE (SNAT) rules handle return traffic routing.

This enhancement aims to provide a robust solution for managing certificates for the API endpoint, particularly for baremetal customers using non cloud-ready DNS servers. Cloud providers typically offer cloud provider DNS services that integrate directly with cert-manager for DNS01 challenges, but baremetal environments often lack these integrations and rely on HTTP01 challenges instead.

### User Stories

1. **As a cluster administrator**, I want to manage custom certificates for the API endpoint (`api.cluster.example.com`) using an external ACME CA, so that I can ensure secure communication for my cluster's API.
2. **As a cluster administrator on a baremetal platform**, I want a reliable solution to handle HTTP01 challenges for the API endpoint, even when the endpoint is not managed by OpenShift Ingress, so that I can avoid manual workarounds.
3. **As a developer**, I want the HTTP01 challenge feature to be deployed via a single CR with no additional container images or DaemonSets, so that I can easily integrate it into my existing cluster setup.

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
- **Day-2 Feature**: This is an optional feature that can be enabled as a day-2 operation after cert-manager-operator installation
- **Conditional Deployment**: The MachineConfig is only deployed when an `HTTP01Proxy` CR is created with `mode` set to `DefaultDeployment`
- **Platform Targeting**: Limited to baremetal platforms where HTTP01 challenges are needed for API certificates

**Operational Responsibilities:**
- Create and manage a MachineConfig that deploys nftables DNAT+MASQUERADE rules and a systemd oneshot service on control plane master nodes
- **Traffic Forwarding**: Forward all TCP port 80 traffic destined for the API VIP to the Ingress VIP using kernel-level DNAT. This is acceptable because nothing else listens on the API VIP on port 80 in a standard OpenShift deployment
- Use nftables PREROUTING DNAT to rewrite the destination from API VIP to Ingress VIP, and POSTROUTING MASQUERADE for return traffic routing
- Add iptables FORWARD rules to allow forwarded packets through OpenShift's FORWARD chain (which uses iptables-nft with `policy DROP`)
- Handle lifecycle management via MachineConfig creation, update, and deletion, with the MCO (Machine Config Operator) managing rollout to master nodes

The MachineConfig-based DNAT rules ensure compatibility with various OCP topologies, including SNO, MNO, and Compact clusters, addressing the challenges of HTTP01 validation for the API endpoint.

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
// When an HTTP01Proxy is created, a MachineConfig is deployed to configure nftables DNAT rules on control plane master nodes.
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
type HTTP01ProxySpec struct {
    // mode controls whether the HTTP01 challenge DNAT rules are active and how they should be deployed.
    // DefaultDeployment enables the feature with default configuration.
    // +kubebuilder:validation:Enum=DefaultDeployment
    // +required
    Mode string `json:"mode"`
}

// HTTP01ProxyStatus is the most recently observed status of the HTTP01Proxy.
type HTTP01ProxyStatus struct {
    // conditions holds information about the current state of the HTTP01 proxy MachineConfig deployment.
    // +patchMergeKey=type
    // +patchStrategy=merge
    // +listType=map
    // +listMapKey=type
    Conditions []metav1.Condition `json:"conditions,omitempty"`
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
```

**Status Conditions:**

```yaml
status:
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2025-05-12T00:00:00Z"
      reason: "MachineConfigApplied"
      message: "DNAT MachineConfig has been applied to master nodes"
    - type: Degraded
      status: "False"
      lastTransitionTime: "2025-05-12T00:00:00Z"
      reason: "MachineConfigHealthy"
      message: "MachineConfig is deployed and nftables DNAT rules are active"
```

This design ensures that:
- Only one HTTP01Proxy configuration can exist per namespace (enforced by the singleton validation on the CR name)
- The cert-manager-operator manages this configuration following the same pattern as IstioCSR
- The feature is optional and can be enabled/disabled as a day-2 operation

### Implementation Details/Notes/Constraints

- The **cert-manager-operator** will be responsible for creating and managing a MachineConfig named `98-nftables-crtmgr-http01-dnat` on control plane master nodes.
- **Controller Location**: The controller is implemented in `pkg/controller/http01proxy/` following the pattern of the istio-csr-controller.
- **MachineConfig Rendering**: The controller renders the MachineConfig resource programmatically using Go templates in `pkg/controller/http01proxy/machineconfig.go`. No static manifest files in `bindata/` are required.
- **Deployment Conditions**: The operator creates the MachineConfig only when:
  - An `HTTP01Proxy` CR named `default` exists in the cert-manager-operator namespace with `mode` set to `DefaultDeployment`
  - The cluster platform is BareMetal with distinct API and Ingress VIPs
- **Singleton Enforcement**: Only one HTTP01Proxy configuration per namespace is supported, enforced by CEL validation on the CR name (must be `default`).
- The implementation relies on `nftables` for traffic redirection, which must be supported and enabled on the cluster nodes.

**MachineConfig Contents:**

The MachineConfig delivers two components to master nodes via Ignition:

1. **nftables config file** at `/etc/sysconfig/nftables-crtmgr-http01.conf`:
   ```text
   table inet crtmgr_http01_dnat
   delete table inet crtmgr_http01_dnat
   table inet crtmgr_http01_dnat {
       chain prerouting {
           type nat hook prerouting priority 0;
           ip daddr <API_VIP> tcp dport 80 dnat ip to <INGRESS_VIP>:80
       }
       chain postrouting {
           type nat hook postrouting priority 100;
           ip daddr <INGRESS_VIP> tcp dport 80 masquerade
       }
   }
   ```

2. **Systemd oneshot service** (`crtmgr-http01-dnat.service`):
   - At startup: enables `net.ipv4.ip_forward` via sysctl, loads the nftables config, and inserts iptables FORWARD rules (idempotent via `-C` check before `-I` insert)
   - At stop: deletes the nftables table and removes the iptables FORWARD rules
   - Uses `RemainAfterExit=yes` to keep the rules active

**Why iptables FORWARD rules alongside nftables NAT:** OpenShift's FORWARD chain uses iptables-nft with `policy DROP`. The kernel evaluates both iptables-nft and native nftables hooks, so both must ACCEPT for forwarded packets to pass. Two FORWARD rules are added:
- `-p tcp -d <INGRESS_VIP>/32 --dport 80 -j ACCEPT` — allows new TCP connections to port 80 on the Ingress VIP
- `-m conntrack --ctstate RELATED,ESTABLISHED -j ACCEPT` — allows return traffic for established connections

**MachineConfig Rollout:** The MCO (Machine Config Operator) handles rolling out the MachineConfig to master nodes. This involves a rolling reboot of master nodes, managed by the MCO with proper drain/cordon to maintain cluster availability.

#### nftables Presence and Management

- `nftables` is always present as part of the RHCOS payload. Baremetal and AWS (at least) OCP clusters install `nftables` tables, chains, and rules by default.
- The `nftables` systemd unit is disabled by default, but the netfilter subsystem is active and can be configured via the `nft` CLI/API without enabling the systemd unit.
- OVN-Kubernetes relies on netfilter (iptables/nftables) for features like UDN, Egress, and Services.
- If a user disables or removes `nftables`, this is considered an explicit user-driven action and is not managed or expected by OpenShift.
- If `nftables` is not present or is disabled, the DNAT rules will not function as intended. The systemd service will fail to load the nftables config, and the operator should surface a `Degraded` status condition on the HTTP01Proxy CR.
- Based on current RHCOS and OCP design, it is not possible to remove the underlying netfilter subsystem, so the feature can reliably depend on its presence unless a user takes explicit unsupported action.

### Design Details

**Scope**: The DNAT rules **only** affect external HTTP traffic (port 80) directed to the API VIP that resolves `api.cluster.example.com`. Internal cluster communication, service discovery, and HTTPS traffic (port 443) are completely unaffected.

- **Operator Integration**: The cert-manager-operator will manage the complete lifecycle of the HTTP01 DNAT MachineConfig, including creation, updates, and removal, following the same pattern as the istio-csr-controller.
- **MachineConfig Deployment**: The operator creates a MachineConfig that configures nftables DNAT rules on master nodes. The MCO handles rolling out the configuration with node reboots.
- **Traffic Redirection**: nftables PREROUTING DNAT forwards all TCP port 80 traffic destined for the API VIP to the Ingress VIP. POSTROUTING MASQUERADE handles return traffic routing.
- **Security**: All HTTP traffic (port 80) to the API VIP is forwarded to the Ingress VIP. This is acceptable because nothing else listens on the API VIP on port 80 in a standard OpenShift deployment.
  - **ACME Traffic**: HTTP01 challenge requests arriving at `<API_VIP>:80` are forwarded to the Ingress VIP, where the OpenShift Router serves the challenge response from the cert-manager challenge pod.
  - **Non-ACME Traffic**: Any other HTTP traffic to the API VIP on port 80 is also forwarded. In standard OpenShift deployments, no services listen on API VIP port 80, so this has no practical impact.
  - **HTTPS Traffic**: All HTTPS traffic (port 443) to the API endpoint continues to function normally and is completely unaffected by the DNAT rules.
- **Monitoring**: The controller logs reconciliation events. On the nodes, `journalctl -u crtmgr-http01-dnat.service` shows nftables rule loading status. The nftables table can be inspected with `nft list table inet crtmgr_http01_dnat`.
- **Configuration Management**: The operator watches the HTTP01Proxy CR for configuration changes and reconciles the MachineConfig state accordingly. It also watches `infrastructure/cluster` for VIP changes, invalidating its platform cache and re-rendering the MachineConfig if VIPs change.

### Drawbacks

1. **Dependency on nftables**: The solution relies on `nftables`, which may not be available or enabled on all environments.
2. **Node Reboot Required**: MachineConfig rollout requires MCO-managed node reboots. Applying or removing the MachineConfig will cause master nodes to be rebooted, which introduces temporary disruption during rollout.
3. **Node-Level Network Modification**: The solution modifies node-level network configuration via MachineConfig, adding nftables rules and a systemd service. While lightweight, this introduces kernel-level packet forwarding that must be coordinated with other networking components.
4. **No Path-Based Filtering**: Unlike a reverse proxy approach, the DNAT rules forward all port 80 traffic to the API VIP, not just ACME challenge paths. This is acceptable in standard deployments where nothing else listens on API VIP port 80.

## Alternatives

### DaemonSet + Reverse Proxy (Superseded)

The original implementation design used a DaemonSet running a Go reverse proxy binary on control plane nodes. The proxy listened on port 8888 with `hostNetwork: true`, and nftables `redirect` rules sent API_VIP:80 traffic to localhost:8888.
The proxy performed path-based filtering, only forwarding requests matching `/.well-known/acme-challenge/*` to the Ingress VIP, and returning 400 Bad Request for all other paths.

This approach was superseded by the MachineConfig-based DNAT approach because:
1. **Operational simplicity**: No container image, DaemonSet, ServiceAccount, ClusterRole, ClusterRoleBinding, SCC RoleBinding, or NetworkPolicies to manage
2. **Smaller attack surface**: No pods running with hostNetwork and privileged SCC
3. **Reliability**: Kernel-level packet forwarding is more robust than a userspace proxy
4. **Maintenance burden**: Eliminates a separate container image build and lifecycle
5. **Resource efficiency**: No additional pods consuming CPU/memory on control plane nodes

The tradeoff is that the MachineConfig approach does not perform path-based filtering (all port 80 traffic to API VIP is forwarded) and requires node reboots for deployment/removal. Both tradeoffs were deemed acceptable for the target use case.

- [Original Proxy Code Repository](https://github.com/mvazquezc/cert-mgr-http01-proxy/tree/main)
- [Original Deployment Manifest](https://github.com/mvazquezc/cert-mgr-http01-proxy/blob/main/manifests/deploy-in-ocp.yaml)

### Other Alternatives (Not Implemented)

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

1. **MachineConfig Failure or Removal**: If the MachineConfig is removed or fails to apply, the nftables DNAT rules will not be present on the master nodes, and HTTP01 challenges for the API endpoint will fail. This does **not** prevent users from accessing their cluster, but may result in the API certificate expiring if not renewed.
   - **User Impact**:
     - End-users and system components can still access the API endpoint, but may encounter certificate warnings or errors if the certificate is expired or invalid.
     - Users may need to accept insecure connections (e.g., use `--insecure-skip-tls-verify` with `oc` or `kubectl`) until the certificate is renewed.
     - Automated systems or integrations that require valid certificates may fail or refuse to connect.
   - **Remediation/Workaround**:
     - Re-create the `HTTP01Proxy` CR, which will trigger the controller to re-create the MachineConfig. The MCO will roll the configuration out to master nodes (requires reboot).
     - If the API certificate has expired, use insecure connection flags to access the cluster and perform remediation.
     - Monitor for certificate expiry and configure alerts to notify administrators before expiry occurs.
   - **Detection**: Configure alerts for expiring certificates. Warning alerts should be triggered when certificates are close to expiry, and critical alerts when certificates have expired. This allows administrators to take action before cluster API access is impacted.

2. **Traffic Interference**: The DNAT rules forward all TCP port 80 traffic destined for the API VIP. In standard OpenShift deployments, nothing listens on API VIP port 80, so this is not a concern. However, if custom services are configured to listen on API VIP port 80, they would be affected.
   **Mitigation**: The API VIP port 80 is not used by any standard OpenShift component. Users should verify no custom services use this port before enabling the feature.

3. **Node Reboot Impact**: Deploying or removing the MachineConfig requires MCO-managed node reboots. On a 3-node control plane, this results in rolling reboots of all master nodes. **Mitigation**: MCO handles rolling reboots with proper drain/cordon, ensuring cluster availability during rollout.

4. **Dual iptables/nftables Coordination**: The solution requires iptables FORWARD rules alongside nftables NAT rules because OpenShift's FORWARD chain uses iptables-nft with DROP policy. Changes to OpenShift's networking stack could affect this. **Mitigation**: The iptables FORWARD rules are narrowly scoped and the systemd service includes proper cleanup on stop.

**General Mitigation**: For all risks above, administrators can disable the HTTP01 challenge feature entirely by deleting the `HTTP01Proxy` CR. The controller's finalizer will delete the MachineConfig, and the MCO will remove the nftables rules and systemd service from master nodes (requires reboot). HTTP01 challenges for the API endpoint will no longer be possible until re-enabled.

### Implementation History

- **2025-03-28**: Enhancement proposal created.
- **2026-01-26**: Updated to use cert-manager-operator CRD approach following istio-csr-controller pattern.
- **2026-07-17**: Updated to MachineConfig-based DNAT approach, replacing DaemonSet + reverse proxy design. Implementation in [openshift/cert-manager-operator#459](https://github.com/openshift/cert-manager-operator/pull/459).

### References

- [Cert Manager Expansion JIRA Epic](https://issues.redhat.com/browse/CNF-18992)
- [ACME HTTP01 Challenge](https://letsencrypt.org/docs/challenge-types/#http-01-challenge)
- [HTTP01 Proxy MachineConfig Implementation (PR #459)](https://github.com/openshift/cert-manager-operator/pull/459)
- [Istio-CSR Controller Enhancement](istio-csr-controller.md)
- [OpenShift MachineConfig Documentation](https://docs.openshift.com/container-platform/latest/post_installation_configuration/machine-configuration-tasks.html)

### Workflow Description

1. Cert Manager initiates an HTTP01 challenge for the API endpoint (`api.cluster.example.com`).
2. The ACME CA sends an HTTP request to `http://api.cluster.example.com:80/.well-known/acme-challenge/<token>`. DNS resolves this to the API VIP.
3. **DNAT Redirection**: The packet arrives at a master node (where the API VIP is hosted by keepalived). nftables PREROUTING rules rewrite the packet destination from API VIP to Ingress VIP. POSTROUTING MASQUERADE rules rewrite the source address for return traffic. iptables FORWARD rules allow the forwarded packets through the kernel's FORWARD chain.
4. The traffic arrives at the OpenShift Ingress Router on the Ingress VIP, which serves the challenge response from the cert-manager challenge pod.
5. The ACME CA validates the challenge and issues the certificate for the API endpoint.

### Topology Considerations

- **Standard Clusters**: The API VIP is hosted on control plane nodes. The DNAT rules ensure that HTTP01 challenges are forwarded to the Ingress VIP and served by the OpenShift Ingress Routers.
- **Compact Clusters**: The DNAT rules handle scenarios where the API VIP node may or may not host an OpenShift Router, ensuring consistent challenge redirection.
- **SNO (Single Node OpenShift)**: The DNAT rules are not strictly required in this topology, as the API and wildcard FQDNs resolve to the same IP. However, the feature can still be deployed for consistency.

#### Hypershift / Hosted Control Planes

This enhancement does not directly apply to Hypershift deployments, as the API endpoint management in Hypershift differs from baremetal environments. However, the DNAT approach could be adapted for similar use cases in Hypershift if needed.

#### Standalone Clusters

For standalone clusters, the DNAT rules ensure that HTTP01 challenges for the API endpoint are forwarded to the Ingress VIP and served by the OpenShift Ingress Routers, regardless of whether the API VIP node hosts a router.

#### Single-node Deployments or MicroShift

In SNO or MicroShift deployments, the DNAT rules are not strictly required, as the API and wildcard FQDNs resolve to the same IP. The controller handles MicroShift gracefully — if the `Infrastructure` or `MachineConfig` CRDs are not available, the controller sets a `Degraded` status condition with an informational message rather than failing.

#### OpenShift Kubernetes Engine

OKE clusters will have full support for the HTTP01 challenge DNAT feature.

## Test Plan

1. **Unit Tests**: Validate MachineConfig rendering, platform discovery, and controller reconciliation logic. The implementation includes unit tests covering MachineConfig template rendering with various VIP combinations, platform validation (BareMetal-only, distinct VIPs required), and controller lifecycle (create, update, delete, finalizer cleanup).
2. **Integration Tests**: Deploy the `HTTP01Proxy` CR on a baremetal test cluster, verify the MachineConfig is created with correct nftables rules, and confirm HTTP01 challenges for the API endpoint succeed end-to-end.
3. **Performance Tests**: Verify minimal performance impact; the solution operates at the kernel level with no userspace proxy overhead.
4. **Topology Tests**: Verify platform validation rejects non-BareMetal clusters with appropriate `Degraded` status condition. Test on Standard Clusters, Compact Clusters, and SNO environments.
5. **Cleanup Tests**: Verify MachineConfig cleanup on CR deletion (finalizer-based cleanup removes MachineConfig).
6. **MicroShift Compatibility**: Verify the controller handles missing `Infrastructure` and `MachineConfig` CRDs gracefully with appropriate status conditions.

## Graduation Criteria

### Dev Preview -> Tech Preview

- The feature is implemented and tested in development environments.
- Documentation is available for deploying and configuring the HTTP01Proxy CR.

### Tech Preview -> GA

- The feature is deployed in production environments and successfully handles HTTP01 challenges for various OCP topologies.
- Performance and reliability meet production-grade requirements.

### Removing a deprecated feature

This enhancement does not deprecate any existing features.

## Upgrade / Downgrade Strategy

- The MachineConfig is managed declaratively by the controller. On operator upgrade, the controller re-renders the MachineConfig template and compares the spec with the existing MachineConfig. If changes are detected (e.g., VIP changes, template updates), the MachineConfig is updated and the MCO will perform a rolling reboot of master nodes.
- If no changes are detected, the MachineConfig is left unchanged and no reboots occur.
- **Downgrade**: If the operator is downgraded to a version that does not include the HTTP01Proxy controller, the MachineConfig will remain on the cluster. Administrators should delete the `HTTP01Proxy` CR before downgrading to trigger finalizer-based cleanup of the MachineConfig.
- The nftables rules and systemd service templates should maintain backwards compatibility to minimize upgrade risks.

## Version Skew Strategy

The MachineConfig rendered by the controller is self-contained (nftables config and systemd unit). Version skew between the operator and the MCO is handled by the MCO's standard reconciliation. Any changes to the DNAT rules or systemd service template will be documented to ensure compatibility with older cluster versions.

## Operational Aspects of API Extensions

- **Monitoring**: The controller logs reconciliation events (MachineConfig creation, updates, deletion). On master nodes, `journalctl -u crtmgr-http01-dnat.service` shows nftables rule loading status. The nftables table can be inspected with `nft list table inet crtmgr_http01_dnat`.
- **Resource Usage**: Minimal. The solution uses kernel-level nftables rules with no userspace proxy process consuming ongoing CPU or memory.
- **Failure Recovery**: The controller monitors the MachineConfig existence and spec. The MCO reports MachineConfig rollout status via MachineConfigPool conditions.

### Recovery Procedures

If the MachineConfig fails to apply (e.g., MCO degraded or MachineConfigPool stuck), HTTP01 challenges for the API endpoint will fail, and certificate renewal will not complete. This may result in the API certificate expiring, which could impact cluster operations.

**Recovery steps:**
- Check MachineConfigPool status: `oc get machineconfigpool master` for degraded conditions.
- Cluster administrators can disable the feature by deleting the `HTTP01Proxy` CR. The controller's finalizer will delete the MachineConfig, and the MCO will remove the nftables rules and systemd service from master nodes.
- If the API certificate has expired, recovery may require connecting to the cluster's API server while ignoring certificate validation errors (e.g., using `--insecure-skip-tls-verify` with `oc` or `kubectl`).
- After resolving the underlying MCO or node issue, re-create the `HTTP01Proxy` CR to restore HTTP01 challenge functionality.
- The HTTP01Proxy CR status conditions surface clear error messages to help identify degraded states.

## Support Procedures

### Detecting Failure Modes

- **Symptoms**:
  - Cert Manager HTTP01 challenges for the API endpoint (`api.cluster.example.com`) fail to complete.
  - Certificates for the API endpoint are not issued or renewed.
  - MachineConfig `98-nftables-crtmgr-http01-dnat` is missing or the MachineConfigPool reports degraded.
  - The HTTP01Proxy CR status shows `Degraded` condition.
  - On master nodes, the systemd service `crtmgr-http01-dnat.service` is not active (check via `systemctl status crtmgr-http01-dnat.service`).
  - The nftables table `crtmgr_http01_dnat` is not present on master nodes (check via `nft list tables`).
  - The API server logs may show failed ACME challenge requests or timeouts.

- **Metrics/Alerts**:
  - MachineConfigPool conditions and MCO alerts indicate MachineConfig deployment status.
  - Certificate expiry alerts (standard cert-manager) indicate challenge failures.
  - Alerts can be configured for MachineConfigPool degradation.

### Disabling the API Extension

- **How to disable**:
  - Delete the `HTTP01Proxy` CR from the cert-manager-operator namespace.
  - The controller's finalizer will delete the MachineConfig. The MCO will remove the nftables rules and systemd service from master nodes (requires reboot).
- **Consequences**:
  - HTTP01 challenges for the API endpoint will not be possible.
  - Certificates for the API endpoint will not be issued or renewed.
  - If the API certificate expires, cluster API access may be impacted until a valid certificate is restored.

### Impact on Existing, Running Workloads

**Important**: The DNAT rules only affect external HTTP access (port 80) via the API VIP that resolves `api.cluster.example.com`. Internal cluster communication (e.g., operator pods, system components, pod-to-API server communication via `kubernetes.default.svc.cluster.local` or internal service IPs) is completely unaffected.

- Existing workloads and API traffic will continue to function as long as the API certificate is valid.
- If the external API certificate expires and is not renewed, external clients may fail to connect to the API server due to certificate errors.
- No direct impact on running pods or services, as they typically use internal DNS endpoints and service discovery.
- Internal cluster operations (operators, controllers, etc.) continue to function normally as they do not use the external API endpoint.

### Impact on Newly Created Workloads

- New certificate requests for the external API endpoint (`api.cluster.example.com`) will fail.
- External tools or workloads that specifically require a valid external API certificate for integration may fail to initialize.
- Internal cluster operations and pod scheduling continue normally as they use internal service discovery.

### Graceful Failure and Recovery

- The DNAT rules are not on the critical path for existing API traffic; they only affect HTTP01 challenge completion on port 80.
- When the MachineConfig is re-applied, Cert Manager can retry failed HTTP01 challenges and resume certificate issuance/renewal.
- No risk of data loss or cluster inconsistency; functionality resumes when the MachineConfig is re-deployed and active.
- If the API certificate has expired, recovery may require connecting with `--insecure-skip-tls-verify` until a new certificate is issued.

### Listing Resources

To list all resources created for the HTTP01 proxy deployment:
```bash
oc get http01proxy default -n cert-manager-operator
oc get machineconfig 98-nftables-crtmgr-http01-dnat
oc get machineconfigpool master
```
