---
title: cert-manager-network-policies
authors:
  - "@manpilla"
reviewers:
  - "@tgeer" ## reviewer for cert-manager component
approvers:
  - "@tgeer" ## approver for cert-manager component
api-approvers:
  - "@tgeer" ## approver for cert-manager component
creation-date: 2025-07-03
last-updated: 2025-01-21
tracking-link:
  - https://issues.redhat.com/browse/CM-525
see-also:
  - NA
replaces:
  - NA
superseded-by:
  - NA
---

# Network Policies for cert-manager Operator and Operands

## Summary

This document proposes the implementation of specific, fine-grained Kubernetes NetworkPolicy objects for the `cert-manager` operator and its operands. The operator can be deployed in any namespace (commonly `cert-manager-operator` but user-configurable), while the operands (including `cert-manager`, `webhook`, `cainjector`, and `istio-csr`) run in their respective operand namespaces (`cert-manager` for core components and user-configurable namespace for `istio-csr`). Currently, the operator and its components run without network restrictions, posing a potential security risk. By defining explicit ingress and egress rules, we can enforce the principle of least privilege, securing all managed namespaces and ensuring that components only communicate with necessary services like the Kubernetes API server and Prometheus.

## Motivation

In a multi-tenant or security-conscious environment, it is crucial to enforce network segregation to limit the potential impact of a compromised pod. The `cert-manager` operator and its components are critical for certificate management within the cluster, but they lack any network traffic filtering or validation. Applying network policies is a standard security best practice that utilizes the platform's own capabilities to secure platform workloads. This enhancement ensures that the `cert-manager` components are not an unintended attack vector.

### User Stories

  - As an administrator, I want to ensure that `cert-manager` components are secure and cannot communicate with unrelated workloads, so I can trust them in a production environment.
  - As a security engineer, I need to verify that all `cert-manager` pods have a default-deny policy and only allow traffic that is explicitly required for their function.
  - As an SRE, I need to ensure that monitoring tools like Prometheus can still scrape metrics from `cert-manager` components even after restrictive policies are applied.
  - As a `cert-manager` user, I need assurance that applying security policies will not break core functionalities like certificate issuance or webhook validation.

### Goals

  - Implement a default-deny policy for all pods in the operator namespace (user-configurable) and operand namespaces (`cert-manager` and user-configurable namespace for istio-csr).
  - Define specific ingress and egress rules for the `cert-manager` operator pod to allow essential communication.
  - Define specific ingress and egress rules for each `cert-manager` component (`cert-manager`, `webhook`, `cainjector`) to allow them to function correctly while blocking unnecessary traffic.
  - Define baseline ingress and egress rules for the `istio-csr` component (deny-all with metrics access), with additional configuration required by users based on their specific deployment requirements.
  - Ensure that metrics collection for all components remains functional.
  - Ensure the API server can communicate with the `cert-manager-webhook` for admission control.

### Non-Goals

  - This enhancement does not propose creating a generic, cluster-wide policy management solution. The policies are specific to `cert-manager`.
  - We are not introducing AdminNetworkPolicy at this stage, as standard NetworkPolicy objects are sufficient for this scope and can be managed directly by the operator.
  - Creating a user-facing option to disable these policies is not in scope, as they represent a baseline security posture.
  - Providing a user-configurable mechanism to customize the broad egress rule for the `cert-manager` controller is not in scope for this initial implementation. Users who require more restrictive policies can supplement these baseline policies with additional NetworkPolicy objects.
  - Automatically detecting and configuring istio-csr gRPC service ports and client access patterns is not in scope. Users must manually configure additional NetworkPolicy rules based on their specific istio-csr deployment configuration.

## Proposal

The proposal is to have the `cert-manager-operator` create and manage a set of `NetworkPolicy` objects across all managed namespaces: the operator's deployment namespace for the operator itself, `cert-manager` for the core operands, and the user-configured namespace for the `istio-csr` operand. The strategy is to first apply a default-deny policy and then layer more specific `allow` policies for required traffic flows.

### Workflow Description

1.  **Default Deny:** For each managed namespace (operator's deployment namespace, `cert-manager`, and the istio-csr deployment namespace), the operator will create a baseline `NetworkPolicy` that selects all pods and applies a full ingress and egress deny. This ensures that no traffic is allowed unless explicitly permitted by another policy.

2.  **Operator Policies:** For the operator's deployment namespace, the operator will create policies to:

      * **Allow Egress to API Server:** Permit outgoing traffic from the operator pod to the Kubernetes API server on port 6443/TCP. This is critical for the operator to manage resources and reconcile its state.
      * **Allow Ingress for Metrics:** Permit incoming traffic to the operator pod on port 8443/TCP from any source, allowing Prometheus to scrape metrics.

3.  **Operand Policies:** For the `cert-manager` namespace, which contains the operands, the operator will create policies for each component:

      * **For the `cert-manager` controller pod (`app: cert-manager`):**

          * **Allow Egress to API Server:** Permit egress to the API server on port 6443/TCP for its core reconciliation loops.
          * **Allow Egress for Issuers:** Permit all egress traffic to allow communication with various external ACME issuers (e.g., Let's Encrypt) or other services required for certificate challenges.
          * **Allow Ingress for Metrics:** Permit ingress on its metrics port (9402/TCP).

      * **For the `cert-manager-webhook` pod (`app: webhook`):**

          * **Allow Egress to API Server:** Permit egress to the API server on port 6443/TCP.
          * **Allow Ingress from API Server:** Permit ingress on the webhook port (10250/TCP) to receive admission review requests from the Kubernetes API server.
          * **Allow Ingress for Metrics:** Permit ingress on its metrics port (9402/TCP).

      * **For the `cert-manager-cainjector` pod (`app: cainjector`):**

          * **Allow Egress to API Server:** Permit egress to the API server on port 6443/TCP so it can inject CA data into other resources.
          * **Allow Ingress for Metrics:** Permit ingress on its metrics port (9402/TCP).

4.  **Istio-CSR Policies:** For the istio-csr deployment namespace (user-configurable), the operator will create baseline policies for:

      * **For the `istio-csr` pod (`app: cert-manager-istio-csr`):**

          * **Allow Egress to API Server:** Permit egress to the API server on port 6443/TCP for its core reconciliation loops and leader election.
          * **Allow Ingress for Metrics:** Permit ingress on its metrics port (9402/TCP).
          * **Note:** Additional ingress rules for the gRPC service port (default 6443/TCP, but user-configurable) must be configured by users based on their specific istio-csr deployment configuration and requirements.

### Implementation Details/Notes/Constraints

The implementation will involve creating multiple `NetworkPolicy` YAML files, managed and applied by the operator.

#### Operator Namespace Policies

**Note:** The operator namespace is user-configurable. The examples below use `cert-manager-operator` as the namespace, but this should be replaced with the actual deployment namespace.

1.  **Deny All Traffic:**

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: deny-all-traffic
      namespace: cert-manager-operator
    spec:
      podSelector: {}
      policyTypes:
      - Ingress
      - Egress
    ```

2.  **Allow Operator Egress to API Server & Ingress for Metrics:**

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-operator-traffic
      namespace: cert-manager-operator
    spec:
      podSelector:
        matchLabels:
          name: cert-manager-operator
      policyTypes:
      - Ingress
      - Egress
      egress:
      - ports:
        - protocol: TCP
          port: 6443
      ingress:
      - ports:
        - protocol: TCP
          port: 8443
    ```

#### Operand Namespace (`cert-manager`) Policies

The policies for the operand namespace will be structured similarly, with a deny-all policy followed by specific allow policies for each component, targeting them via their `app` label (`cert-manager`, `webhook`, `cainjector`). Each will have egress allowed to the API server and ingress allowed for metrics (port 9402), with the webhook additionally allowing ingress on port 10250 for admission control.

1.  **Deny All Traffic:** A baseline policy will deny all traffic in the `cert-manager` namespace.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: deny-all-traffic
      namespace: cert-manager
    spec:
      podSelector: {}
      policyTypes:
      - Ingress
      - Egress
    ```

2.  **Allow `cert-manager` Controller Traffic:** This policy allows the main controller to talk to issuers and exposes its metrics port.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-cert-manager-controller-traffic
      namespace: cert-manager
    spec:
      podSelector:
        matchLabels:
          app: cert-manager
      policyTypes:
      - Ingress
      - Egress
      egress:
      - {} # Allow all egress for communication with external issuers
           # This broad rule is necessary as issuers can be deployed anywhere
           # and the controller needs to communicate with various external services
           # for certificate challenges (ACME, DNS providers, etc.)
      ingress:
      - ports:
        - protocol: TCP
          port: 9402
    ```

3.  **Allow `cert-manager-webhook` Traffic:** This policy allows the webhook to function and expose its metrics.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-cert-manager-webhook-traffic
      namespace: cert-manager
    spec:
      podSelector:
        matchLabels:
          app: webhook
      policyTypes:
      - Ingress
      - Egress
      egress:
      - ports:
        - protocol: TCP
          port: 6443
      ingress:
      - ports:
        - protocol: TCP
          port: 10250 # Webhook port for API server
        - protocol: TCP
          port: 9402  # Metrics port
    ```

4.  **Allow `cert-manager-cainjector` Traffic:** This policy allows the CA injector to communicate with the API server and expose its metrics.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-cert-manager-cainjector-traffic
      namespace: cert-manager
    spec:
      podSelector:
        matchLabels:
          app: cainjector
      policyTypes:
      - Ingress
      - Egress
      egress:
      - ports:
        - protocol: TCP
          port: 6443
      ingress:
      - ports:
        - protocol: TCP
          port: 9402
    ```

#### Istio-CSR Namespace Policies

**Note:** The istio-csr namespace is user-configurable. The examples below use `istio-system` as the namespace, but this should be replaced with the actual deployment namespace.

The `istio-csr` component requires specific network policies to function correctly while maintaining security. Through traffic analysis using network observability tools, the following essential traffic flows have been identified:

- **API Server Communication (Egress):** The `istio-csr` pod requires egress access to the Kubernetes API server on port 6443/TCP for leader election, resource reconciliation, and managing certificates.
- **gRPC Service (Ingress):** The `istio-csr` pod exposes a gRPC service (default port 6443/TCP, but user-configurable) to handle certificate signing requests from Istio components. **Users must configure additional NetworkPolicy rules for this based on their specific port configuration.**
- **Metrics Endpoint (Ingress):** The `istio-csr` pod exposes metrics on port 9402/TCP for monitoring by Prometheus.

1.  **Deny All Traffic:** A baseline policy will deny all traffic for `istio-csr` pods in their deployment namespace.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: deny-istio-csr-traffic
      namespace: istio-system # Replace with actual istio-csr deployment namespace
    spec:
      podSelector:
        matchLabels:
          app: cert-manager-istio-csr
      policyTypes:
      - Ingress
      - Egress
    ```

2.  **Allow `istio-csr` Baseline Traffic:** This policy provides baseline access for the istio-csr component (API server communication and metrics). **Users must add additional NetworkPolicy rules for gRPC service access based on their configuration.**

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-istio-csr-baseline-traffic
      namespace: istio-system  # Replace with actual istio-csr deployment namespace
    spec:
      podSelector:
        matchLabels:
          app: cert-manager-istio-csr
      policyTypes:
      - Ingress
      - Egress
      egress:
      - ports:
        - protocol: TCP
          port: 6443 # API server access for reconciliation and leader election
      ingress:
      - ports:
        - protocol: TCP
          port: 9402  # Metrics port
    ```

3.  **User-Required gRPC Service Policy Example:** Users must create additional policies like this based on their istio-csr gRPC service port configuration:

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-istio-csr-grpc-service
      namespace: istio-system  # Replace with actual istio-csr deployment namespace
    spec:
      podSelector:
        matchLabels:
          app: cert-manager-istio-csr
      policyTypes:
      - Ingress
      ingress:
      - ports:
        - protocol: TCP
          port: 6443  # Replace with actual configured gRPC service port
        # Add appropriate 'from' selectors based on your Istio components' labels/namespaces
    ```

### API Extensions

This enhancement does not introduce any new APIs or modify existing API structures. It exclusively uses the standard `networking.k8s.io/v1/NetworkPolicy` resource.

### Topology Considerations

The proposed network policies are designed to be effective across various cluster topologies.

#### Hypershift / Hosted Control Planes

In a Hypershift environment, the `cert-manager` operator and operands run in the hosted cluster. The policies correctly target the in-cluster API server endpoint for egress traffic. No special configuration is required.

#### Standalone Clusters

For standard, standalone clusters, the policies will function as described, securing traffic between the pods and the cluster's API server.

#### Single-node Deployments or MicroShift

The network policies are fully compatible with single-node and MicroShift deployments. They will enforce the same principle of least privilege, regardless of the cluster's scale.

### Risks and Mitigations

  * **Risk:** Policies are too restrictive and block legitimate traffic, causing `cert-manager` to fail.
      * **Mitigation:** The proposed policies are based on traffic analysis using network observability tools. All essential flows (API server access, webhooks, metrics) have been identified and explicitly allowed. The test plan includes end-to-end validation to confirm functionality.
  * **Risk:** Outgoing traffic for certificate challenges (e.g., HTTP-01, DNS-01) is blocked for the `cert-manager` controller.
      * **Mitigation:** The proposal includes a broad egress rule for the `cert-manager` controller pod (`egress: - {}`) to allow it to communicate with any external issuer or service. While less specific, this is necessary for its core function. This could be refined in the future if a mechanism to predict issuer endpoints is developed.
  * **Risk:** Debugging becomes more difficult.
      * **Mitigation:** Failures due to network policies are observable. Connection timeouts in logs or metrics are a strong indicator. Cluster administrators can use tools like the OpenShift Network Observability Operator to visualize traffic flows and identify blocked connections.
  * **Risk:** Users may not configure additional NetworkPolicy rules for istio-csr gRPC service, causing functionality issues.
      * **Mitigation:** Clear documentation will be provided with examples of required additional NetworkPolicy configurations. The operator will log warnings when istio-csr is deployed but additional ingress rules are not detected.

### Drawbacks

The main drawback is the added complexity of managing multiple `NetworkPolicy` objects. However, this complexity is managed by the operator, not the end-user, and the security benefits significantly outweigh this drawback.

## Test Plan

  * **Integration Tests:**
    1.  Deploy the operator and confirm all `NetworkPolicy` objects are created as expected in all managed namespaces (operator's deployment namespace, `cert-manager`, istio-csr deployment namespace).
    2.  Verify the operator and all operand pods (including `istio-csr`) are running without errors or crash loops.
    3.  Create a `curl` pod and confirm it **can** access the metrics endpoints (`:8443` for operator, `:9402` for operands including `istio-csr`).
    4.  Confirm the `curl` pod **cannot** `ping` or otherwise access the pods on non-allowed ports.
    5.  Verify that `istio-csr` can communicate with the API server for leader election and resource reconciliation.
    6.  Verify that `istio-csr` gRPC service is **blocked** by default and requires user-configured additional NetworkPolicy rules to function.
  * **End-to-End (E2E) Tests:**
    1.  Run the existing `cert-manager` E2E test suite with the network policies enabled.
    2.  Create a `Certificate` resource and verify it is successfully issued, which validates the entire flow from API server -> webhook -> controller -> issuer. This implicitly tests the webhook ingress and the controller's egress capabilities.
    3.  Configure additional NetworkPolicy rules for `istio-csr` gRPC service based on the deployment configuration, then run the `istio-csr` E2E test suite to ensure that certificate signing requests from Istio components are processed correctly.

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

  * **Upgrade:** On upgrade, the operator will apply the new `NetworkPolicy` objects. Since the previous version had no policies, this will be a seamless transition to a more secure state.
  * **Downgrade:** If a user downgrades to a version of the operator that is not aware of network policies, the `NetworkPolicy` objects will be orphaned (left behind). Since older versions operated in a default-allow world, these leftover restrictive policies could break the installation. The downgrade documentation must instruct the user to manually delete the `NetworkPolicy` objects from the operator's deployment namespace, `cert-manager`, and istio-csr deployment namespace before downgrading. This is only necessary if the network policies have been modified from their default configuration.

## Alternatives (Not Implemented)

  * **Deny-All at Namespace Level:** An initial approach considered applying a single `podSelector: {}` deny-all policy to the entire namespace without specific component targeting. However, the current proposal uses targeted deny-all policies that select specific pods (e.g., `app: cert-manager-istio-csr` for istio-csr) rather than all pods in a namespace. This approach is more explicit and ensures that the denial is clearly associated with the component it is intended to protect, while avoiding interference with other workloads that might be deployed in the same namespace.
  * **Single Combined Policy:** Another alternative was to create one large `NetworkPolicy` per namespace. This was rejected in favor of multiple smaller, more focused policies (e.g., one for API server egress, one for metrics ingress). This makes the purpose of each rule clearer and easier to manage and debug.

## Version Skew Strategy

This enhancement only involves adding `NetworkPolicy` resources, which are managed by the `cert-manager-operator`. There are no version skew concerns with other components, as the operator's version will be tied to the policies it deploys. The Kubernetes API for `NetworkPolicy` is stable.

## Operational Aspects of API Extensions

Not applicable, as this enhancement does not introduce any API extensions.

## Support Procedures

Support personnel debugging potential issues should first check the `NetworkPolicy` resources in the operator's deployment namespace, `cert-manager`, and istio-csr deployment namespace.

1.  Verify the policies exist in all managed namespaces: `oc get networkpolicy -n <operator-namespace>`, `oc get networkpolicy -n cert-manager`, and `oc get networkpolicy -n <istio-csr-namespace>`.
2.  If a pod is suspected of having network connectivity issues, check its logs for connection timeout errors.
3.  Use the OpenShift Network Observability Operator or similar tools to visualize traffic and identify any connections being dropped by the policies.
4.  For `istio-csr` specific issues:
   - Verify that API server communication is working for leader election.
   - Check if user has configured additional NetworkPolicy rules for the gRPC service port.
   - Verify the gRPC service port configuration matches the NetworkPolicy rules.