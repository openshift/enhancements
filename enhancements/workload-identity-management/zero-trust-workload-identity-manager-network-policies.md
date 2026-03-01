---
title: zero-trust-workload-identity-manager-network-policies
authors:
  - "@PillaiManish"
reviewers:
  - "@tgeer" ## reviewer for ZTWIM component
approvers:
  - "@tgeer" ## approver for ZTWIM component
api-approvers:
  - "@tgeer" ## approver for ZTWIM component
creation-date: 2025-10-08
last-updated: 2025-10-08
tracking-link:
  - https://issues.redhat.com/browse/SPIRE-212
see-also:
  - "/enhancements/cert-manager/cert-manager-network-policies.md"
replaces:
  - NA
superseded-by:
  - NA
---

# Network Policies for Zero Trust Workload Identity Manager Operator and Operands

## Summary

This document proposes the implementation of specific, fine-grained Kubernetes NetworkPolicy objects for the `zero-trust-workload-identity-manager` operator and its operands. Both the operator and operands (including `spire-server`, `spire-agent`, `spiffe-csi-driver`, and `spire-oidc-discovery-provider`) run in the same namespace (`zero-trust-workload-identity-manager`). Currently, the operator and its components run without network restrictions, posing a potential security risk. By defining explicit ingress and egress rules, we can enforce the principle of least privilege, securing the managed namespace and ensuring that components only communicate with necessary services like the Kubernetes API server, SPIRE components, and Prometheus. The network policies will be deployed automatically as part of the operator installation.

## Motivation

In a multi-tenant or security-conscious environment, it is crucial to enforce network segregation to limit the potential impact of a compromised pod. The `zero-trust-workload-identity-manager` operator and its components are critical for workload identity management within the cluster, handling sensitive SPIFFE/SPIRE operations, but they lack any network traffic filtering or validation. Applying network policies is a standard security best practice that utilizes the platform's own capabilities to secure platform workloads. This enhancement ensures that the ZTWIM components are not an unintended attack vector and aligns with zero trust security principles.

### User Stories

- As an administrator, I want to ensure that ZTWIM components are secure and cannot communicate with unrelated workloads, so I can trust them in a production environment.
- As a security engineer, I need to verify that all ZTWIM pods have a default-deny policy and only allow traffic that is explicitly required for their function.
- As an SRE, I need to ensure that monitoring tools like Prometheus can still scrape metrics from ZTWIM components even after restrictive policies are applied.
- As a ZTWIM user, I need assurance that applying security policies will not break core functionalities like SPIFFE identity issuance, SPIRE agent attestation, or workload API access.

### Goals

- Implement default network policies for all ZTWIM components to secure network traffic.
- Apply a default-deny policy for all pods in the `zero-trust-workload-identity-manager` namespace.
- Define specific ingress and egress rules for the ZTWIM operator pod to allow essential communication. Define baseline ingress and egress rules for each ZTWIM component (`spire-server`, `spire-agent`, `spiffe-csi-driver`, `spire-oidc-discovery-provider`) based on traffic analysis.
- Ensure that metrics collection for all components remains functional.
- Ensure the API server can communicate with SPIRE webhook components for admission control.
- Deploy network policies automatically with no user configuration required.

### Non-Goals

- This enhancement does not propose creating a generic, cluster-wide policy management solution. The policies are specific to ZTWIM.
- We are not introducing AdminNetworkPolicy at this stage, as standard NetworkPolicy objects are sufficient for this scope.
- This enhancement does not include user-configurable network policies. All policies are predefined based on traffic analysis.
- This enhancement does not cover network policies for workloads consuming SPIFFE identities, only for ZTWIM infrastructure components.

## Proposal

### Workflow Description

1. **Default Policy Deployment:** Operator's Network policies are deployed automatically as part of the operator installation with no user configuration required while operand's network policies are applied during operand's installation.

2. **Default Deny:** Baseline `NetworkPolicy` objects deny all traffic for each component, ensuring that no traffic is allowed unless explicitly permitted by additional policies.

3. **Operator Policies:** The operator's network policies are included in the deployment and cover:
    * **Egress to API Server:** Permit outgoing traffic to the Kubernetes API server on port 6443/TCP
    * **Ingress for Metrics:** Permit incoming traffic on port 8443/TCP for Prometheus metrics scraping from `openshift-monitoring` namespace

4. **SPIRE Server Policies:** Default policies for the SPIRE server component include:
    * **Ingress from SPIRE Agents:** Allow SPIRE agents to connect on port 8081/TCP
    * **Webhook Ingress:** Allow webhook traffic on port 9443/TCP for SPIRE controller manager
    * **Metrics Ingress:** Allow metrics collection on port 9402/TCP and port 8082/TCP for SPIRE controller manager from monitoring namespace
    * **API Server Egress:** Allow communication with Kubernetes API server
    * **Federation:** Allow egress-ingress communication over federation port 8443/TCP
    * **DNS Egress:** Allow DNS resolution for SPIRE controller manager

5. **SPIRE Agent Policies:** Default policies for SPIRE agent components include:
    * **Egress to SPIRE Server:** Allow connection to SPIRE server on port 8081/TCP
    * **Metrics Ingress:** Allow metrics collection on port 9402/TCP from monitoring namespace
    * **API Server Egress:** Allow communication with Kubernetes API server for node attestation
    * **DNS Egress:** Allow DNS resolution for service discovery

6. **SPIFFE CSI Driver Policies:** Default policies for CSI driver include:
    * No specific network policies required - communicates via Unix sockets with SPIRE agent

7. **OIDC Discovery Provider Policies:** Default policies for OIDC provider include:
    * **HTTPS Ingress:** Allow client connections on port 8443/TCP

### Implementation Details/Notes/Constraints

The implementation will involve creating default `NetworkPolicy` objects that are deployed automatically with ZTWIM components. No API extensions are required as the policies will be static. The operator will deploy operand network policies as part of the standard component deployment, while the operator's own network policies will be handled by the OLM bundle.

#### ZTWIM Component Network Policies

The following network policies are deployed automatically for each component. All policies apply to the `zero-trust-workload-identity-manager` namespace.

1. **Baseline Deny-All Policy:** Applied to all pods in the namespace.

   ```yaml
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: deny-all-traffic
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector: {}
     policyTypes:
     - Ingress
     - Egress
   ```

2. **Operator Controller Policies:**

   ```yaml
   # Operator controller egress to API server
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-operator-api-server-egress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        name: zero-trust-workload-identity-manager
     policyTypes:
     - Egress
     egress:
     - ports:
       - protocol: TCP
         port: 6443
   ---
   # Operator controller metrics ingress
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-operator-metrics-ingress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        name: zero-trust-workload-identity-manager
     policyTypes:
     - Ingress
     ingress:
     - from:
       - namespaceSelector:
           matchLabels:
             name: openshift-monitoring
       ports:
       - protocol: TCP
         port: 8443
   ```

3. **SPIRE Server Policies:**

   ```yaml
   # SPIRE Server ingress from agents
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-spire-server-agent-ingress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        app.kubernetes.io/name: spire-server
     policyTypes:
     - Ingress
     ingress:
     # Allow SPIRE agents to connect
      - ports:
        - protocol: TCP
          port: 8081
   ---
   # SPIRE Server webhook ingress
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-spire-server-webhook-ingress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        app.kubernetes.io/name: spire-server
     policyTypes:
     - Ingress
     ingress:
     # Allow webhook traffic 
     - ports:
       - protocol: TCP
         port: 9443
   ---
   # SPIRE Server metrics ingress
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-spire-server-metrics-ingress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        app.kubernetes.io/name: spire-server
     policyTypes:
     - Ingress
     ingress:
     - from:
       - namespaceSelector:
           matchLabels:
             name: openshift-monitoring
       ports:
       - protocol: TCP
         port: 9402  # SPIRE server metrics
       - protocol: TCP
         port: 8082  # SPIRE controller manager metrics
   ---
   # SPIRE Server egress to API server
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-spire-server-api-egress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        app.kubernetes.io/name: spire-server
     policyTypes:
     - Egress
     egress:
     - ports:
       - protocol: TCP
         port: 6443
   ---
   # SPIRE Server federation
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-spire-server-federation
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        app.kubernetes.io/name: spire-server
     policyTypes:
     - Ingress
     - Egress
     ingress:
     # Allow federation traffic 
     - ports:
       - protocol: TCP
         port: 8443
     egress:
     - ports:
       - protocol: TCP
         port: 8443
   ---
   # SPIRE Server egress to DNS for controller manager
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-spire-server-dns-egress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        app.kubernetes.io/name: spire-server
     policyTypes:
     - Egress
     egress:
     - to:
       - namespaceSelector:
           matchLabels:
             kubernetes.io/metadata.name: openshift-dns
         podSelector:
           matchLabels:
             dns.operator.openshift.io/daemonset-dns: default
       ports:
       - protocol: TCP
         port: 5353
       - protocol: UDP
         port: 5353
   ```

4. **SPIRE Agent Policies:**

   ```yaml
   # SPIRE Agent egress to server
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-spire-agent-server-egress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        app.kubernetes.io/name: spire-agent
     policyTypes:
     - Egress
     egress:
     # Allow connection to SPIRE server
     - ports:
       - protocol: TCP
         port: 8081
   ---
   # SPIRE Agent metrics ingress
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-spire-agent-metrics-ingress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        app.kubernetes.io/name: spire-agent
     policyTypes:
     - Ingress
     ingress:
     - from:
       - namespaceSelector:
           matchLabels:
             name: openshift-monitoring
       ports:
       - protocol: TCP
         port: 9402
   ---
   # SPIRE Agent egress to API server
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-spire-agent-api-egress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
        app.kubernetes.io/name: spire-agent
     policyTypes:
     - Egress
     egress:
     - ports:
       - protocol: TCP
         port: 6443
   ```

5. **OIDC Discovery Provider Policies:**

   ```yaml
   # OIDC Discovery Provider HTTPS ingress
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-oidc-provider-https-ingress
     namespace: zero-trust-workload-identity-manager
   spec:
     podSelector:
       matchLabels:
         app.kubernetes.io/name: spiffe-oidc-discovery-provider
     policyTypes:
     - Ingress
     ingress:
     # Allow HTTPS traffic from clients
     - ports:
       - protocol: TCP
         port: 8443
   ```

### API Extensions

Not applicable. This enhancement does not introduce any API extensions as network policies are deployed with default configurations based on traffic analysis.

### Topology Considerations

The proposed network policies are designed to be effective across various cluster topologies.

#### Hypershift / Hosted Control Planes

NA

#### Standalone Clusters

For standard, standalone clusters, the policies will function as described, securing traffic between the pods and the cluster's API server.

#### Single-node Deployments or MicroShift

NA

### Risks and Mitigations

* **Risk:** Policies are too restrictive and block legitimate traffic, causing ZTWIM components to fail.
    * **Mitigation:** All essential flows (API server access, inter-component communication, metrics) have been identified and explicitly allowed. The test plan includes end-to-end validation to confirm functionality.
* **Risk:** SPIRE Agent cannot reach SPIRE Server due to network policy restrictions.
    * **Mitigation:** The proposal includes specific ingress and egress rules for SPIRE agent-to-server communication on port 8081/TCP. Traffic analysis confirms this is the primary communication channel.
* **Risk:** Workload API access is blocked for applications using SPIFFE identities.
    * **Mitigation:** SPIFFE Workload API communication occurs via Unix domain sockets, which are not affected by NetworkPolicy. No network-level restrictions are needed for this communication path.
* **Risk:** Debugging becomes more difficult.
    * **Mitigation:** Failures due to network policies are observable. Connection timeouts in logs or metrics are a strong indicator. Cluster administrators can use tools like the OpenShift Network Observability Operator to visualize traffic flows and identify blocked connections.
* **Risk:** Default policies may not cover all edge cases or future requirements.
    * **Mitigation:** Future enhancements can extend policies if new requirements are identified.

### Drawbacks

The main drawback is the added complexity of managing multiple `NetworkPolicy` objects. However, this complexity is managed by the operator, not the end-user, and the security benefits significantly outweigh this drawback.

## Test Plan

* **Integration Tests:**
    1. Test ZTWIM deployment with default network policies: Verify all NetworkPolicy objects are created automatically and all components can communicate with each other and the API server.
    2. Test spiffe-csi-driver with network policies: Verify CSI driver can perform volume operations via Unix socket communication with SPIRE agent.
    3. Test SPIRE agent to server communication: Verify agents can register with the server and obtain SVIDs with network policies in place.
    4. Test OIDC discovery provider functionality: Verify OIDC provider can serve discovery documents and validate JWTs.
    5. Create a `curl` pod and confirm it **can** access the metrics endpoints (`:8443` for operator, `:9402` for SPIRE components, `:8082` for controller manager) when namespace is labeled with `name: openshift-monitoring`.
    6. Confirm the `curl` pod **cannot** access pods on non-allowed ports when policies are enabled.
    7. Test DNS resolution: Verify SPIRE server controller manager and SPIRE agent can resolve DNS names.
* **End-to-End (E2E) Tests:**
    1. Run the existing ZTWIM E2E test suite with network policies enabled to ensure no functional regression.
    2. Run workload identity issuance tests with network policies enabled to ensure SPIFFE functionality is preserved.
    3. Test complete SPIRE workflow: agent attestation, SVID issuance, JWT-SVID validation, and CSI volume mounting.

## Graduation Criteria

This feature will be delivered as GA directly, as it is a self-contained security enhancement using stable Kubernetes APIs.

* All policies are implemented and delivered with the operator.
* All tests outlined in the Test Plan are passing.
* Documentation is updated to mention the presence and purpose of the network policies.

### Dev Preview -> Tech Preview

Not applicable. This feature will be enabled by default at GA.

### Tech Preview -> GA

Not applicable. This feature will be enabled by default at GA.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

Not applicable. This enhancement introduces network policies for the first time and will be delivered as GA directly.

## Alternatives (Not Implemented)

* **Deny-All at Namespace Level:** An initial approach considered applying a single `podSelector: {}` deny-all policy to the entire namespace without specific component targeting. However, the current proposal uses targeted deny-all policies that select specific pods (e.g., `app.kubernetes.io/name: spiffe-csi-driver` for CSI driver) rather than all pods in a namespace. This approach is more explicit and ensures that the denial is clearly associated with the component it is intended to protect, while avoiding interference with other workloads that might be deployed in the same namespace.
* **Single Combined Policy:** Another alternative was to create one large `NetworkPolicy` per namespace. This was rejected in favor of multiple smaller, more focused policies (e.g., one for API server egress, one for metrics ingress, one for inter-component communication). This makes the purpose of each rule clearer and easier to manage and debug.

## Version Skew Strategy

NA

## Operational Aspects of API Extensions

Not applicable, as this enhancement does not introduce any API extensions.

## Support Procedures

NA