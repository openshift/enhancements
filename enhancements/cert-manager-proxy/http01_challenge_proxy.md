---
title: http01-challenge-cert-manager-proxy
authors:
  - "@sebrandon1"
reviewers:
  - "@TrilokGeer"
  - "@swagosh"
approvers:
  - "@tkashem"
  - "@deads2k"
  - "@derekwaynecarr"
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

However, cluster administrators often want to use Cert Manager to issue custom certificates for the API endpoint (`api.cluster.example.com`). Unlike other endpoints, this API endpoint is not exposed via the OpenShift Ingress. Depending on the OCP topology (e.g., SNO, MNO, Compact), it is exposed directly on the node or via a keepalive VIP. This lack of management by the OpenShift Ingress introduces challenges in obtaining certificates using an external ACME CA.

The gap arises due to how the ACME HTTP01 challenge works. The following scenarios illustrate the challenges:

1. **Standard Clusters**: The API VIP is hosted on the control plane nodes which do not host an OpenShift Router. The http01 challenge, which is directed at the API VIP (the IP where `api.cluster.example.com` DNS resolves), will not hit an OpenShift Router and thus not reach the challenge response pod started by Cert Manager.
2. **Compact Clusters**: The node hosting the API VIP may also host an OpenShift Router. If no router is present on the node hosting the VIP, the challenge will fail.
3. **SNO (Single Node OpenShift)**: The same nodes host both the ingress and API components. Both FQDNs (`api` and wildcard) resolve to the same IP, making the challenge feasible.

To address this gap, a small proxy was developed. This proxy runs on the cluster as a DaemonSet (control plane nodes) and then adds iptables rules to the nodes and ensures that connections reaching the API on port 80 are redirected to the OpenShift Ingress Routers. The proxy implementation creates a reverse proxy to the apps VIP and uses `nftables` to redirect traffic from `API:80` to `PROXY:8888`.

- **Proxy Code**: [GitHub Repository](https://github.com/mvazquezc/cert-mgr-http01-proxy/tree/main)
- **Deployment Manifest**: [Manifest Link](https://github.com/mvazquezc/cert-mgr-http01-proxy/blob/main/manifests/deploy-in-ocp.yaml)

This enhancement aims to provide a robust solution for managing certificates for the API endpoint in baremetal environments.

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

The HTTP01 Challenge Proxy will be implemented via DaemonSet running on the cluster. It will:

- Redirect HTTP traffic from the API endpoint (`api.cluster.example.com`) on port 80 to the OpenShift Ingress Routers.
- Use `nftables` for traffic redirection from `API:80` to `PROXY:8888`.
- Be deployed using a manifest that includes all necessary configurations.

The proxy will ensure compatibility with various OCP topologies, including SNO, MNO, and Compact clusters, addressing the challenges of HTTP01 validation for the API endpoint.

### API Extensions

A new CR type may be created and can be applied to clusters.  This new typed will be stored in the [openshift/api](https://github.com/openshift/api) repo.

Potential Example of a CR:

```
apiVersion: network.openshift.io/v1alpha1
kind: HTTP01ChallengeProxy
metadata:
  name: example-http01challengeproxy
spec:
  # Add fields here to specify the desired state of the HTTP01ChallengeProxy
  # Default port is 8888.
  internalport: 8888
status:
  conditions:
    - type: Ready
      status: "True"
      lastTransitionTime: "2025-05-12T00:00:00Z"
      reason: "Initialized"
      message: "HTTP01ChallengeProxy is ready"
```

### Implementation Details/Notes/Constraints

- The proxy will be deployed as a DaemonSet to ensure it runs on all nodes which may host the API VIP in the cluster.
- When the proxy starts, it checks which version of OCP it's running on. If it's 4.17+ it will proceed to configure MCO restart the nftables service rather than reboot the node when the file /etc/sysconfig/nftables.conf is modified. After that it creates the MC that configures the file + service.
- The implementation relies on `nftables` for traffic redirection, which must be supported and enabled on the cluster nodes.
- The demo deployment manifest for the proxy is available [here](https://github.com/mvazquezc/cert-mgr-http01-proxy/blob/main/manifests/deploy-in-ocp.yaml).
- An example implementation can be found in this [repository](https://github.com/mvazquezc/cert-mgr-http01-proxy/tree/main).
- The proxy will listen on a configurable port (default: 8888) for HTTP01 challenge traffic. The port can be set via the CR to avoid conflicts with other workloads that may require port 8888 on the host.
- 8888 was chosen as a reasonable default because it is commonly unused, but clusters with a conflict can override this value in the CR.
- The [host port registry](https://github.com/openshift/enhancements/blob/master/dev-guide/host-port-registry.md) should be updated to reflect the use of port 80 on apiServer nodes when this feature is enabled, to avoid conflicts and ensure proper documentation of port usage.
- The priority (order) of the `nftables` entries relative to other services should be coordinated with the OpenShift networking team to ensure it follows established precedent and does not interfere with other networking rules.

#### nftables Presence and Management

- `nftables` is always present as part of the RHCOS payload. Baremetal and AWS (at least) OCP clusters install `nftables` tables, chains, and rules by default.
- The `nftables` systemd unit is disabled by default, but the netfilter subsystem is active and can be configured via the `nft` CLI/API without enabling the systemd unit.
- OVN-Kubernetes relies on netfilter (iptables/nftables) for features like UDN, Egress, and Services.
- The Machine Config Operator (MCO) does not manage `nftables` directly, but users can explicitly disable or modify `nftables` via MachineConfig if desired.
- If a user disables or removes `nftables`, this is considered an explicit user-driven action and is not managed or expected by OpenShift.
- If `nftables` is not present or is disabled, the proxy will not function as intended. Detection of this condition should be implemented (e.g., by checking for the presence of the `nft` binary and ability to apply rules), and the operator should surface a clear error or degraded status.
- Based on current RHCOS and OCP design, it is not possible to remove the underlying netfilter subsystem, so the feature can reliably depend on its presence unless a user takes explicit unsupported action.

### Design Details

- **Proxy Deployment**: The proxy will be deployed using a Kubernetes DaemonSet. The daemonset will implement an nftable rule via pod that runs to completion.
- **Traffic Redirection**: This will use `nftables` rules to redirect incoming traffic on `API:80` to `PROXY:8888`.
- **Security**: The proxy will only handle HTTP traffic for the HTTP01 challenge and will not interfere with other traffic or services.
- **Monitoring**: Logs and metrics will be exposed to help administrators monitor the proxy's behavior and troubleshoot issues.

### Drawbacks

1. **Dependency on nftables**: The solution relies on `nftables`, which may not be available or enabled on all environments.
2. **Additional Resource Usage**: Running the proxy as a DaemonSet introduces additional resource usage on the cluster nodes while the proxy pod is applying its nftable rules.
3. **Complexity**: The solution adds another component to the cluster, which may increase operational complexity.

## Alternatives (Not Implemented)

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

### Implementation History

- **2025-03-28**: Enhancement proposal created.

### References

- [Cert Manager Expansion JIRA Epic](https://issues.redhat.com/browse/CNF-13731)
- [ACME HTTP01 Challenge](https://letsencrypt.org/docs/challenge-types/#http-01-challenge)
- [Proxy Code Repository](https://github.com/mvazquezc/cert-mgr-http01-proxy/tree/main)
- [Deployment Manifest](https://github.com/mvazquezc/cert-mgr-http01-proxy/blob/main/manifests/deploy-in-ocp.yaml)

### Workflow Description

1. Cert Manager initiates an HTTP01 challenge for the API endpoint (`api.cluster.example.com`).
2. The HTTP01 challenge request is directed to the API VIP on port 80.
3. The HTTP01 Challenge Proxy intercepts the traffic using `nftables` and redirects it to the proxy pod on port 8888.
4. The proxy pod forwards the request to the OpenShift Ingress Router, which serves the challenge response from the Cert Manager challenge pod.
5. The ACME CA validates the challenge and issues the certificate for the API endpoint.

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
- The proxy DaemonSet must use a `Recreate` update strategy to ensure that only one instance of the proxy runs per node at any time, as the proxy listens on a fixed port (`8888`) in the host network. This prevents port collisions during upgrades.
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
- Cluster administrators can disable or remove the proxy by deleting the DaemonSet or updating the relevant CR/manifest.
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
  - Remove or update the associated CR (if using a CRD).
  - Remove any MachineConfig or configuration that enables the proxy.
- **Consequences**:
  - HTTP01 challenges for the API endpoint will not be possible.
  - Certificates for the API endpoint will not be issued or renewed.
  - If the API certificate expires, cluster API access may be impacted until a valid certificate is restored.

### Impact on Existing, Running Workloads

- Existing workloads and API traffic will continue to function as long as the API certificate is valid.
- If the API certificate expires and is not renewed, clients may fail to connect to the API server due to certificate errors.
- No direct impact on running pods or services, unless they rely on the API endpoint with a valid certificate.

### Impact on Newly Created Workloads

- New certificate requests for the API endpoint will fail.
- New clusters or workloads that require a valid API certificate for bootstrap or integration may fail to initialize.

### Graceful Failure and Recovery

- The proxy is not on the critical path for existing API traffic; it only affects HTTP01 challenge completion.
- When the proxy is restored, Cert Manager can retry failed HTTP01 challenges and resume certificate issuance/renewal.
- No risk of data loss or cluster inconsistency; functionality resumes when the proxy is re-enabled and healthy.
- If the API certificate has expired, recovery may require connecting with `--insecure-skip-tls-verify` until a new certificate is issued.
