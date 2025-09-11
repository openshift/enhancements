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

  - Add API fields to enable/disable network policies for cert-manager components and allow user customization.
  - Implement a default-deny policy for all pods in the operator namespace (user-configurable) and operand namespaces (`cert-manager` and user-configurable namespace for istio-csr) when network policies are enabled.
  - Define specific ingress and egress rules for the `cert-manager` operator pod to allow essential communication.
  - Define baseline ingress and egress rules for each `cert-manager` component (`cert-manager`, `webhook`, `cainjector`) with deny-all default and user-configurable network policies via the API.
  - Define baseline ingress and egress rules for the `istio-csr` component (deny-all with metrics access), with user-configurable network policies via the API for additional access requirements.
  - Ensure that metrics collection for all components remains functional.
  - Ensure the API server can communicate with the `cert-manager-webhook` for admission control.
  - Provide backward compatibility by making network policies opt-in via the `DefaultNetworkPolicy` field.

### Non-Goals

  - This enhancement does not propose creating a generic, cluster-wide policy management solution. The policies are specific to `cert-manager`.
  - We are not introducing AdminNetworkPolicy at this stage, as standard NetworkPolicy objects are sufficient for this scope and can be managed directly by the operator.
  - Automatically detecting and configuring istio-csr gRPC service ports and client access patterns is not in scope. Users must configure network policies via the API based on their specific istio-csr deployment configuration.

## Proposal

The proposal is to extend the `CertManager` and `IstioCSR` custom resources with new API fields to enable and configure network policies. The `cert-manager-operator` will create and manage `NetworkPolicy` objects across all managed namespaces based on these API configurations. For cert-manager, network policies are opt-in via the `DefaultNetworkPolicy` field for backward compatibility. For istio-csr, network policies are enabled by default. The strategy is to first apply a default-deny policy and then allow users to configure additional rules via the API.

### Workflow Description

1.  **API-Driven Configuration:** Users configure network policies through the `CertManager` and `IstioCSR` custom resource specifications:
    - For cert-manager: Set `DefaultNetworkPolicy: "enabled"` and optionally provide custom `NetworkPolicy` rules
    - For istio-csr: Provide custom `NetworkPolicy` rules (network policies are enabled by default)

2.  **Default Deny:** When network policies are enabled, the operator will create baseline `NetworkPolicy` objects that deny all traffic for the respective components. This ensures that no traffic is allowed unless explicitly permitted.

3.  **Operator Policies:** For the operator's deployment namespace, the operator will create default policies to:

      * **Allow Egress to API Server:** Permit outgoing traffic from the operator pod to the Kubernetes API server on port 6443/TCP.
      * **Allow Ingress for Metrics:** Permit incoming traffic to the operator pod on port 8443/TCP for Prometheus metrics scraping.

4.  **Cert-Manager Operand Policies:** When `DefaultNetworkPolicy` is enabled, the operator will create baseline policies for each component:

      * **Default policies include:**
          * **API Server Egress:** For all components to communicate with the Kubernetes API server
          * **Metrics Ingress:** For all components to expose metrics on port 9402/TCP
          * **Webhook Ingress:** For the webhook component to receive admission requests on port 10250/TCP

      * **User-configurable policies:** Users can specify additional or custom rules via the `NetworkPolicy` field in the `CertManager` spec. If not provided, cert-manager components will have deny-all policies (which will prevent proper operation without user configuration).

5.  **Istio-CSR Policies:** The operator will create baseline policies for istio-csr:

      * **Default policies include:**
          * **API Server Egress:** For communication with the Kubernetes API server on port 6443/TCP
          * **Metrics Ingress:** For exposing metrics on port 9402/TCP

      * **User-required policies:** Users must specify additional rules via the `NetworkPolicy` field in the `IstioCSR` spec for gRPC service access and other requirements. If not provided, istio-csr will have deny-all policies that prevent proper operation.

### Implementation Details/Notes/Constraints

The implementation will involve extending the existing APIs and creating `NetworkPolicy` objects based on the user's API configuration. The operator will manage these policies according to the specifications provided in the custom resources.

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

#### Cert-Manager Operand Namespace Policies

When `DefaultNetworkPolicy` is set to "enabled", the operator will create baseline policies for cert-manager components. Users can provide additional or custom policies via the `NetworkPolicy` field in the `CertManager` spec.

1.  **Baseline Deny-All Policy:** Applied when `DefaultNetworkPolicy` is enabled.

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

2.  **Default Allow Policies:** The operator creates these baseline policies when `DefaultNetworkPolicy` is enabled:

    ```yaml
    # API Server egress for all components
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-api-server-egress
      namespace: cert-manager
    spec:
      podSelector:
        matchLabels:
          app.kubernetes.io/name: cert-manager
      policyTypes:
      - Egress
      egress:
      - ports:
        - protocol: TCP
          port: 6443
    ---
    # Metrics ingress for all components
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-metrics-ingress
      namespace: cert-manager
    spec:
      podSelector:
        matchLabels:
          app.kubernetes.io/name: cert-manager
      policyTypes:
      - Ingress
      ingress:
      - ports:
        - protocol: TCP
          port: 9402
    ---
    # Webhook ingress for admission control
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-webhook-ingress
      namespace: cert-manager
    spec:
      podSelector:
        matchLabels:
          app: webhook
      policyTypes:
      - Ingress
      ingress:
      - ports:
        - protocol: TCP
          port: 10250
    ```

3.  **User-Configurable Policies:** Users must configure additional policies via the API for cert-manager controller egress (to communicate with external issuers). Example user configuration:

    ```yaml
    apiVersion: operator.openshift.io/v1alpha1
    kind: CertManager
    metadata:
      name: cluster
    spec:
      defaultNetworkPolicy: "enabled"
      networkPolicy:
        metadata:
          name: allow-cert-manager-controller-egress
          namespace: cert-manager
        spec:
          podSelector:
            matchLabels:
              app: cert-manager
          policyTypes:
          - Egress
          egress:
          - {} # Allow all egress for external issuers communication
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

2.  **Default Baseline Policies:** The operator creates these baseline policies for istio-csr:

    ```yaml
    # API Server egress
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-istio-csr-api-server-egress
      namespace: istio-system  # Replace with actual istio-csr deployment namespace
    spec:
      podSelector:
        matchLabels:
          app: cert-manager-istio-csr
      policyTypes:
      - Egress
      egress:
      - ports:
        - protocol: TCP
          port: 6443 # API server access for reconciliation and leader election
    ---
    # Metrics ingress
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: allow-istio-csr-metrics-ingress
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
          port: 9402  # Metrics port
    ```

3.  **User-Required Configuration via API:** Users must configure additional policies via the `NetworkPolicy` field in the `IstioCSR` spec. Example configuration:

    ```yaml
    apiVersion: operator.openshift.io/v1alpha1
    kind: IstioCSR
    metadata:
      name: cluster
    spec:
      networkPolicy:
        metadata:
          name: allow-istio-csr-grpc-service
          namespace: istio-system  # Replace with actual deployment namespace
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
            # Add appropriate 'from' selectors based on your Istio components
    ```

### API Extensions

This enhancement introduces new fields to the existing `CertManager` and `IstioCSR` custom resources to support network policy configuration.

#### CertManager API Changes

The `CertManagerSpec` will be extended with two new fields:

```go
type CertManagerSpec struct {
    // ... existing fields ...

    // DefaultNetworkPolicy enables or disables the default network policy for cert-manager components.
    // When set to "enabled", the operator will create default network policies to secure
    // communication between cert-manager controller, webhook, and cainjector components.
    // When set to "disabled" or empty, no default network policies are created.
    // Valid values are: "enabled", "disabled", or empty (default: disabled).
    //
    // +kubebuilder:validation:Optional
    // +kubebuilder:validation:Enum=enabled;disabled
    // +optional
    DefaultNetworkPolicy string `json:"defaultNetworkPolicy,omitempty"`

    // NetworkPolicy specifies the network policy configuration to be applied to cert-manager
    // pods/operands when DefaultNetworkPolicy is enabled. By default, enabling network policies
    // creates a deny-all policy that blocks all network traffic to and from cert-manager components.
    // Use this field to provide the necessary network policy rules that allow required traffic
    // for cert-manager to function properly (e.g., API server communication, webhook traffic, etc.).
    //
    // This field is only effective when DefaultNetworkPolicy is set to "enabled".
    // If DefaultNetworkPolicy is enabled but this field is not provided, cert-manager
    // components will be isolated with deny-all network policies.
    //
    // +kubebuilder:validation:Optional
    // +optional
    NetworkPolicy *v1.NetworkPolicy `json:"networkPolicy,omitempty"`
}
```

#### IstioCSR API Changes

The `IstioCSRSpec` will be extended with a new field:

```go
type IstioCSRSpec struct {
    // ... existing fields ...

    // NetworkPolicy specifies the network policy configuration to be applied to istio-csr
    // pods. By default, network policies are enabled with a deny-all approach that blocks
    // all network traffic to and from istio-csr components.
    //
    // If this field is not provided, istio-csr components will be isolated with deny-all
    // network policies, which will prevent proper operation.
    //
    // +kubebuilder:validation:Optional
    // +optional
    NetworkPolicy *v1.NetworkPolicy `json:"networkPolicy,omitempty"`
}
```

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
    1.  Test with `DefaultNetworkPolicy: "disabled"` (default): Verify no NetworkPolicy objects are created and cert-manager functions normally.
    2.  Test with `DefaultNetworkPolicy: "enabled"` but no custom `NetworkPolicy` specified: Verify baseline policies are created and cert-manager controller cannot communicate with external issuers (expected behavior).
    3.  Test with `DefaultNetworkPolicy: "enabled"` and custom `NetworkPolicy` for cert-manager controller egress: Verify cert-manager can communicate with external issuers.
    4.  Test istio-csr with no `NetworkPolicy` specified: Verify baseline policies are created and gRPC service is blocked (expected behavior).
    5.  Test istio-csr with custom `NetworkPolicy` for gRPC service: Verify istio-csr can receive certificate signing requests.
    6.  Create a `curl` pod and confirm it **can** access the metrics endpoints (`:8443` for operator, `:9402` for operands) when policies are enabled.
    7.  Confirm the `curl` pod **cannot** access pods on non-allowed ports when policies are enabled.
  * **End-to-End (E2E) Tests:**
    1.  Run the existing `cert-manager` E2E test suite with `DefaultNetworkPolicy: "disabled"` to ensure no regression.
    2.  Configure proper NetworkPolicy settings via the API and run the cert-manager E2E test suite with network policies enabled.
    3.  Configure proper NetworkPolicy settings for istio-csr via the API and run the istio-csr E2E test suite with network policies enabled.

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

  * **Upgrade:** On upgrade, the new API fields will be available but network policies remain disabled by default (`DefaultNetworkPolicy: "disabled"`). This ensures backward compatibility. Users must explicitly enable network policies and configure them appropriately.
  * **Downgrade:** If a user downgrades to a version of the operator that is not aware of the new API fields:
    - The API fields will be ignored by the older operator version
    - Any existing `NetworkPolicy` objects created by the newer operator will be orphaned
    - Users should disable network policies (`DefaultNetworkPolicy: "disabled"`) and remove custom `NetworkPolicy` configurations before downgrading to prevent orphaned policies from blocking traffic

## Alternatives (Not Implemented)

  * **Deny-All at Namespace Level:** An initial approach considered applying a single `podSelector: {}` deny-all policy to the entire namespace without specific component targeting. However, the current proposal uses targeted deny-all policies that select specific pods (e.g., `app: cert-manager-istio-csr` for istio-csr) rather than all pods in a namespace. This approach is more explicit and ensures that the denial is clearly associated with the component it is intended to protect, while avoiding interference with other workloads that might be deployed in the same namespace.
  * **Single Combined Policy:** Another alternative was to create one large `NetworkPolicy` per namespace. This was rejected in favor of multiple smaller, more focused policies (e.g., one for API server egress, one for metrics ingress). This makes the purpose of each rule clearer and easier to manage and debug.

## Version Skew Strategy

This enhancement only involves adding `NetworkPolicy` resources, which are managed by the `cert-manager-operator`. There are no version skew concerns with other components, as the operator's version will be tied to the policies it deploys. The Kubernetes API for `NetworkPolicy` is stable.

## Operational Aspects of API Extensions

Not applicable, as this enhancement does not introduce any API extensions.

## Support Procedures

Support personnel debugging potential network policy issues should follow these steps:

1.  **Check API Configuration:**
    - Verify `CertManager` resource: `oc get certmanager cluster -o yaml` and check `defaultNetworkPolicy` and `networkPolicy` fields
    - Verify `IstioCSR` resource: `oc get istiocsr cluster -o yaml` and check `networkPolicy` field

2.  **Verify NetworkPolicy Objects:**
    - Check if NetworkPolicy objects exist: `oc get networkpolicy -n <namespace>`
    - Compare actual policies with expected configuration from the API specs

3.  **Troubleshoot Connectivity Issues:**
    - Check pod logs for connection timeout errors
    - Use the OpenShift Network Observability Operator to visualize traffic flows
    - Verify that required NetworkPolicy rules are configured via the API

4.  **Common Issues:**
    - **Cert-manager controller cannot reach external issuers:** Check if custom `NetworkPolicy` is configured in `CertManager` spec for controller egress
    - **Istio-csr gRPC service not accessible:** Check if custom `NetworkPolicy` is configured in `IstioCSR` spec for gRPC ingress
    - **Metrics not accessible:** Verify baseline policies are applied correctly