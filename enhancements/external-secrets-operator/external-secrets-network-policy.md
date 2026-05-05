---
title: external-secrets-network-policy
authors:
  - “@sbhor”
reviewers:
  - “@tgeer”
approvers:
  - “@tgeer”
api-approvers:
  - “@tgeer”
creation-date: 2025-09-12
last-updated:  2026-05-04
tracking-link: 
  - https://issues.redhat.com/browse/ESO-165
  - https://issues.redhat.com/browse/ESO-70
  - https://redhat.atlassian.net/browse/RFE-8516
see-also:
  - NA
replaces:
  - NA
superseded-by:
  - NA
---

# Network Policies for external-secrets Operator and Operands

## Summary

This document proposes the implementation of specific, fine-grained Kubernetes NetworkPolicy objects for the external-secrets operator and its operands.The operator can be deployed in any namespace (commonly `external-secrets-operator` but user-configurable) and the operand namespace would be `external-secrets`. Currently, the operator and its components run without network restrictions, posing a potential security risk. To address this, the operator's NetworkPolicy will be shipped as part of the OLM bundle, while the operand's NetworkPolicy will be created and managed dynamically by the operator. By defining explicit ingress and egress rules, we can enforce the principle of least privilege, securing the external-secrets namespaces and ensuring that its components only communicate with necessary services like the Kubernetes API server. It also allows automatic proxy egress policy management so ESO pods on proxy-configured clusters can reach the proxy server without manual NetworkPolicy authoring, and a stable `eso-sys-`/`eso-user-` naming scheme for all operator-owned policies with a migration-aware cleanup to remove legacy unprefixed objects on upgrade.

## Motivation

In a multi-tenant or security-conscious environment, it is crucial to enforce network segregation to limit the potential impact of a compromised pod. The `external-secrets` operator and its components are critical for secret management within the cluster, but they operate with default-allow network rules. Applying network policies is a standard security best practice that utilizes the platform's own capabilities to secure platform workloads. This enhancement ensures that the `external-secrets` components are not an unintended attack vector.

### User Stories

- As an administrator, I want to ensure that `external-secrets` components are secure and cannot communicate with unrelated workloads, so I can trust them in a production environment.
- As a security engineer, I need to verify that all `external-secrets` pods have a default-deny policy and only allow traffic that is explicitly required for their function.
- As a `external-secrets` user, I need assurance that applying security policies will not break core functionalities like secret management or webhook validation.
- As an administrator, I want to configure and manage egress rules for `external-secrets` operands via the operator API or CRDs, so I can control which external services they are allowed to access.
- As an administrator on a proxy-configured cluster, I want the operator to automatically allow ESO pods to reach the proxy server, so I do not have to manually create egress NetworkPolicies after every install or upgrade.
- As an administrator who manages proxy egress manually, I want to set `spec.appConfig.proxy.networkPolicyAllowProxyEgressAll: Unmanaged` to disable the operator's automatic proxy egress policy, so I retain full control over proxy traffic rules.
- As a cluster administrator upgrading from an older release, I want the operator to clean up legacy unprefixed NetworkPolicy objects after it has applied the new prefixed ones, so there are no stale or duplicate policies left behind.

### Goals

- Implement a default-deny policy for all pods in the `external-secrets-operator` and `external-secrets` namespaces.
- Define specific ingress and egress rules for the `external-secrets` operator pod to allow essential communication.
- Define specific ingress and egress rules for `external-secrets` operand  to allow them to function correctly while blocking unnecessary traffic.
- Ensure that metrics collection for all components remains functional.
- Ensure the API server can communicate with the `webhook` for admission control.
- Automatically create a proxy-egress allow policy when an effective proxy is configured, controlled by `spec.appConfig.proxy.networkPolicyAllowProxyEgressAll` (default `Managed`).
- Introduce a stable `eso-sys-` / `eso-user-` naming scheme for all operator-managed NetworkPolicy objects to enable unambiguous ownership and safe pruning.
- Migrate legacy unprefixed NetworkPolicy names to the new scheme on upgrade and clean up stale objects without leaving gaps in coverage.

### Non-Goals

- This enhancement does not propose creating a generic, cluster-wide policy management solution. The policies are specific to `external-secrets`.
- We are not introducing AdminNetworkPolicy at this stage, as standard NetworkPolicy objects are sufficient for this scope and can be managed directly by the operator.
- `spec.appConfig.proxy.networkPolicyAllowProxyEgressAll` does not affect non-proxy clusters — `getProxyConfiguration()` returns nil there and no proxy policy is ever created regardless of the field value.

## Proposal

The proposal is to create and manage `NetworkPolicy` objects for both the operator and its operands. The NetworkPolicy for operands will be created and managed by the `external-secrets-operator`, while the operator’s own NetworkPolicy will be shipped as part of the `OLM bundle`. The strategy is to first apply a default-deny policy and then layer more specific allow policies for required traffic flows.

### Workflow Description

1.  **Default Deny:** For each managed namespace (`external-secrets-operator` and `external-secrets`), the operator will create a baseline `NetworkPolicy` that selects all pods and applies a full ingress and egress deny. This ensures that no traffic is allowed unless explicitly permitted by another policy.

2.  **Operator Policies:** For the `external-secrets-operator` namespace, the operator will create policies to:

    * **Allow Egress to API Server:** Permit outgoing traffic from the operator pod to the Kubernetes API server on port 6443/TCP. This is critical for the operator to manage resources and reconcile its state.

3.  **Operand Policies:** For the `external-secrets` namespace, which contains the operands, the operator will create policies for each component:

    * **For the `external-secrets` controller pod (`app: external-secrets`):**

        * **Allow Egress to API Server:** Permit egress to the API server on port 6443/TCP for its core reconciliation loops.

    * **For the `external-secrets-webhook` pod (`app: external-secrets`):**

        * **Allow Egress to API Server:** Permit egress to the API server on port 6443/TCP.
        * **Allow Ingress from API Server:** Permit ingress on the webhook port (10250/TCP) to receive admission review requests from the Kubernetes API server.

    * **For the `external-secrets-cert-controller` pod (`app: external-secrets`):**

        * **Allow Egress to API Server:** Permit egress to the API server on port 6443/TCP so it can inject CA data into other resources. 
        * Note: The `cert-controller` is an optional component. It is only created if `cert-manager` is **not** enabled. If `cert-manager` is enabled, this component and its policies must be cleaned up.

    * **For the `external-secrets-bitwarden-sever` pod (`app: external-secrets`):**

        * **Allow Egress to API Server:** Permit egress to the API server on port 6443/TCP so it can store the secrets fetched from external `Bitwarden Secrets Manager` into a Kubernetes Secret resource. 
        * **Allow Ingress from Core Controller:** Permit ingress from the `external-secrets` controller pod for communication with the Bitwarden server.
        * Note: The Bitwarden server is an optional integration. It is only created if explicitly enabled by user configuration.* User has to additionally create a allow NetworkPolicy to interact with the external `Bitwarden Secret Manager`.

4.  **Proxy Egress Policy (conditional):** After applying the static operand policies, the reconciler evaluates whether an automatic proxy egress allow policy should be created in the `external-secrets` namespace. The policy is created only when **both** conditions hold:

    * `spec.appConfig.proxy.networkPolicyAllowProxyEgressAll` is `Managed` (the default).
    * `getProxyConfiguration()` returns a non-nil result - i.e., an effective proxy is actually configured on the cluster.

    When created, this policy allows all ESO pods to reach the proxy server on its configured port. When either condition is not met (`Unmanaged`, or no proxy configured), the policy is not created; if one was previously created, it is removed on the next reconcile.

5.  **Naming scheme:** All operator-owned Kubernetes NetworkPolicy objects use a stable name prefix so the operator can unambiguously identify and prune them:

    * Static system policies (bindata): `eso-sys-<stable-name>` (e.g., `eso-sys-deny-all-traffic`).
    * Programmatically-built system policies : also use the `eso-sys-<stable-name>` prefix (e.g., `eso-sys-proxy-egress-core`). These are assembled in Go at reconcile time because they embed runtime data (e.g., the proxy port from `getProxyConfiguration()`).
    * User-defined policies from `spec.networkPolicies[]`: the operator prepends `eso-user-` to the logical name in the CR. The user writes `name: allow-external-secrets-egress`; the resulting Kubernetes object is named `eso-user-allow-external-secrets-egress`.

6.  **Migration and prune:** `cleanupMigratedNetworkPolicies()` runs only after the apply step fully succeeds. On the first reconcile after upgrade (no `migration-complete` annotation on the CR), it lists all NetworkPolicies in the namespace by label (`app.kubernetes.io/managed-by` + `app.kubernetes.io/part-of`) and by name, diffs against the already-applied desired set, and deletes any whose name is not in the desired set — this covers legacy unprefixed names as well as any stale user NPs. Once all deletions succeed, a `migration-complete` annotation is written to the `ExternalSecretsConfig` CR using the `fieldOwner` option. On subsequent reconciles the full deletion loop is skipped, but the label-based list and diff still runs every reconcile to catch newly stale entries (e.g. a user removes an NP from the CR). This function will be removed after 3 releases, by which point every cluster will have been reconciled at least once under the new naming scheme.

      
### Implementation Details/Notes/Constraints

The implementation will involve extending the existing APIs and creating `NetworkPolicy` objects based on the user's API configuration. The operator will manage these policies according to the specifications provided in the custom resources.

#### Operator Namespace (`external-secrets-operator`) Policies

1.  **Deny All Traffic:**

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: deny-all-traffic
      namespace: external-secrets-operator
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
         namespace: external-secrets-operator
       spec:
         podSelector:
           matchLabels:
             app: external-secrets-operator
         policyTypes:
         - Ingress
         - Egress
         egress:
         - ports:
           - protocol: TCP
             port: 6443 # Required: Kubernetes API server
         ingress:
         # Optional: expose metrics (8443 and 8080 based on user configuration)
          - from:
            - namespaceSelector:
                matchLabels:
                  name: openshift-monitoring
          - ports:
            - protocol: TCP
              port: 8443
            - protocol: TCP
              port: 8080
     ```

#### Operand Namespace (`external-secrets`) Policies

The policies for the operand namespace will be structured similarly, with a deny-all policy followed by specific allow policies for each component, targeting them via their `app` label (`external-secrets`). Each will have egress allowed to the API server with the webhook additionally allowing ingress on port 10250 for admission control.

1.  **Deny All Traffic:** A baseline policy will deny all traffic in the `external-secrets` namespace.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: eso-sys-deny-all-traffic
      namespace: external-secrets
      labels:
        app.kubernetes.io/managed-by: external-secrets-operator
    spec:
      podSelector: {}
      policyTypes:
      - Ingress
      - Egress
    ```

2.  **Allow `external-secrets` Controller Traffic:** This policy allows the main controller to talk to API-server.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: eso-sys-allow-api-server-egress
      namespace: external-secrets
      labels:
        app.kubernetes.io/managed-by: external-secrets-operator
    spec:
      podSelector:
        matchLabels:
          app.kubernetes.io/name: external-secrets
      policyTypes:
      - Egress
      egress:
        - ports:
            - protocol: TCP
              port: 6443
    ```

3. **Allow `external-secrets-webhook` Controller Traffic:** This policy allows the API Server Access for Outbound communication to Kubernetes API server (port 6443) for resource reconciliation and Webhook Admission Control Inbound traffic from API server to webhook (port 10250) for resource validation.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: eso-sys-allow-api-server-egress-for-webhook
      namespace: external-secrets
      labels:
        app.kubernetes.io/managed-by: external-secrets-operator
    spec:
      podSelector:
        matchLabels:
          app.kubernetes.io/name: external-secrets-webhook
      policyTypes:
      - Egress
      - Ingress
      egress:
        - ports:
            - protocol: TCP
              port: 6443
      ingress:
        - ports:
            - protocol: TCP
              port: 10250
    ```

4. **Allow `external-secrets-cert-controller` Traffic:** This policy allows the cert-controller API Server Access for outbound communication to Kubernetes API server (port 6443/TCP) for certificate lifecycle management.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: eso-sys-allow-api-server-egress-for-cert-controller
      namespace: external-secrets
      labels:
        app.kubernetes.io/managed-by: external-secrets-operator
    spec:
      podSelector:
        matchLabels:
          app.kubernetes.io/name: external-secrets-cert-controller
      policyTypes:
      - Egress
      egress:
        - ports:
            - protocol: TCP
              port: 6443
    ```

5. **Allow `external-secrets-bitwarden-server` Traffic:** This policy permits the Bitwarden server to communicate with the Kubernetes API server (port 6443/TCP) for secret synchronization, and to receive inbound traffic from the core controller for integration workflows.

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: eso-sys-allow-api-server-egress-for-bitwarden-server
      namespace: external-secrets
      labels:
        app.kubernetes.io/managed-by: external-secrets-operator
    spec:
      podSelector:
        matchLabels:
          app.kubernetes.io/name: external-secrets-bitwarden-server
      policyTypes:
      - Ingress
      - Egress
      ingress:
       # Allow External Secrets Controller to communicate with Bitwarden SDK Server
        - ports: 
            - protocol: TCP
              port: 9998
      # Allow access to Kubernetes API server
      egress:
        - ports:
            - protocol: TCP
              port: 6443
    ```  
6. **User-Configurable Policies:** Users configure additional policies via the `ExternalSecretsConfig` custom resource to set `external-secrets` controller egress allow policy to communicate with external providers. The operator prepends `eso-user-` to the logical name in the CR. The user writes `name: allow-external-secrets-egress`; the resulting Kubernetes object is named `eso-user-allow-external-secrets-egress`.
Example user configuration:

    ```yaml
    apiVersion: operator.openshift.io/v1alpha1
    kind: ExternalSecretsConfig
    metadata:
      name: cluster
    spec:
      appConfig:
        proxy:
          networkPolicyAllowProxyEgressAll: Managed   # default; set Unmanaged to manage proxy egress yourself
      networkPolicies:
        - name: allow-external-secrets-egress    # K8s object: eso-user-allow-external-secrets-egress
          componentName: CoreController
          policyTypes:
          - Egress
          egress:
          - {} # Allow all egress for external issuers communication
    ```  

7. **Proxy Egress Policy (auto-created, conditional):** This policy is built **programmatically in the reconciler** - it is not a static bindata manifest because the proxy port is runtime data obtained from `getProxyConfiguration()`.

    The policy is applied only when **both** conditions hold:

    * `spec.appConfig.proxy.networkPolicyAllowProxyEgressAll` is `Managed` (the default).
    * `getProxyConfiguration()` returns a non-nil result - i.e., an effective proxy is actually configured on the cluster.

    If either condition is not met, this policy is not created (or removed if previously present).

    Illustrative structure (port value filled in at runtime):

    ```yaml
    apiVersion: networking.k8s.io/v1
    kind: NetworkPolicy
    metadata:
      name: eso-sys-proxy-egress-core
      namespace: external-secrets
      labels:
        app.kubernetes.io/managed-by: external-secrets-operator
    spec:
      podSelector:
        matchLabels:
          app.kubernetes.io/name: external-secrets
      policyTypes:
        - Egress
      egress:
        - ports:
            - protocol: TCP
              port: <proxy-port>   # set at reconcile time from getProxyConfiguration()
    ```



### API Extensions

This enhancement introduces new fields to the existing `ExternalSecretsConfig` custom resources to support network policy configuration.

```go
   // ComponentName represents the different external-secrets components that can have network policies applied.
    type ComponentName string
    
    const (
        // CoreController represents the external-secrets component"
        CoreController ComponentName = "ExternalSecretsCoreController"
        
        // BitwardenSDKServer represents the bitwarden-sdk-server component" 
		BitwardenSDKServer ComponentName = "BitwardenSDKServer"
		
    )

    // NetworkPolicy represents a custom network policy configuration for operator-managed components.
    // It includes a name for identification and the network policy rules to be enforced.
    type NetworkPolicy struct {
        // Name is the logical identifier for this network policy entry.
        // The operator prepends "eso-user-" to this value when creating the Kubernetes
        // NetworkPolicy object (e.g. "allow-egress" becomes "eso-user-allow-egress").
        // +kubebuilder:validation:Required
        // +required
        Name string `json:"name"`
		
		// +kubebuilder:validation:Enum:=CoreController;BitwardenSDKServer
		// +kubebuilder:validation:Required
		ComponentName ComponentName `json:"componentName"`
		
        // egress is a list of egress rules to be applied to the selected pods. Outgoing traffic
        // is allowed if there are no NetworkPolicies selecting the pod (and cluster policy
        // otherwise allows the traffic), OR if the traffic matches at least one egress rule
        // across all of the NetworkPolicy objects whose podSelector matches the pod. If
        // this field is empty then this NetworkPolicy limits all outgoing traffic (and serves
        // solely to ensure that the pods it selects are isolated by default).
		// The operator will automatically handle ingress rules based on the current running ports.
        // +optional
        // +listType=atomic
        Egress []networkingv1.NetworkPolicyEgressRule `json:"egress,omitempty" protobuf:"bytes,3,rep,name=egress"`
  }

    // NetworkPolicyEgressManagement controls whether the operator manages the proxy egress NetworkPolicy.
    type NetworkPolicyEgressManagement string

    const (
        // NetworkPolicyEgressManaged means the operator automatically creates and reconciles
        // the eso-sys-proxy-egress-core NetworkPolicy when a proxy is configured.
        NetworkPolicyEgressManaged NetworkPolicyEgressManagement = "Managed"

        // NetworkPolicyEgressUnmanaged means the user is responsible for proxy egress;
        // the operator does not create, update, or delete the proxy egress NetworkPolicy.
        NetworkPolicyEgressUnmanaged NetworkPolicyEgressManagement = "Unmanaged"
    )

    // ProxyConfig is extended to include the proxy egress NetworkPolicy management field.
    type ProxyConfig struct {
        // ... existing httpProxy, httpsProxy, noProxy fields ...

        // networkPolicyAllowProxyEgressAll controls whether the operator automatically
        // creates and manages the "eso-sys-proxy-egress-core" NetworkPolicy that allows
        // all ESO pods to reach the cluster proxy server.
        // On clusters without a proxy configured, this field has no effect regardless of value.
        // +kubebuilder:validation:Enum=Managed;Unmanaged
        // +kubebuilder:default=Managed
        // +optional
        NetworkPolicyAllowProxyEgressAll NetworkPolicyEgressManagement `json:"networkPolicyAllowProxyEgressAll,omitempty"`
    }

    type ExternalSecretsConfigSpec struct {

        // NetworkPolicies specifies the list of network policy configurations
        // to be applied to external-secrets pods.
        //
        // Each entry allows specifying a name for the generated NetworkPolicy object,
        // along with its full Kubernetes NetworkPolicy definition.
        // The operator prepends "eso-user-" to the provided name when creating the Kubernetes object.
        //
        // If this field is not provided, external-secrets components will be isolated
        // with deny-all network policies, which will prevent proper operation.
        //
        // +kubebuilder:validation:Optional
        // +optional
        NetworkPolicies []NetworkPolicy `json:"networkPolicies,omitempty"`
    
    }
```

### Topology Considerations

The proposed network policies are designed to be effective across various cluster topologies.

#### Hypershift / Hosted Control Planes

In a Hypershift environment, the `external-secrets` operator and operands run in the hosted cluster. The policies correctly target the in-cluster API server endpoint for egress traffic. No special configuration is required.

#### Standalone Clusters

For standard, standalone clusters, the policies will function as described, securing traffic between the pods and the cluster's API server.

#### OpenShift Kubernetes Engine

None

#### Single-node Deployments or MicroShift

The network policies are fully compatible with single-node and MicroShift deployments. They will enforce the same principle of the least privilege, regardless of the cluster's scale.

### Risks and Mitigations

* **Risk:** Policies are too restrictive and block legitimate traffic, causing `external-secrets` to fail.
    * **Mitigation:** The proposed policies are based on traffic analysis using network observability tools. All essential flows (API server access, webhooks, metrics) have been identified and explicitly allowed. The test plan includes end-to-end validation to confirm functionality.
* **Risk:** Outgoing traffic for certificate challenges (e.g., HTTP-01, DNS-01) is blocked for the `external-secrets` controller.
    * **Mitigation:** The proposal includes a broad egress rule for the `external-secrets` controller pod (`egress: - {}`) to allow it to communicate with any external provider or service. While less specific, this is necessary for its core function. This could be refined in the future.
* **Risk:** Debugging becomes more difficult.
    * **Mitigation:** Failures due to network policies are observable. Connection timeouts in logs or metrics are a strong indicator. Cluster administrators can use tools like the OpenShift Network Observability Operator to visualize traffic flows and identify blocked connections.

### Drawbacks

The main drawback is the added complexity of managing multiple `NetworkPolicy` objects. However, this complexity is managed by the operator, not the end-user, and the security benefits significantly outweigh this drawback.

## Test Plan

* **Integration Tests:**
    1.  Deploy the operator and confirm all `NetworkPolicy` objects are created with the new `eso-sys-` prefix as expected.
    2.  Verify the operator and all operand pods are running without errors or crash loops.
    3.  Create a `curl` pod and confirm it **can** access the metrics endpoints (`:8443` for operator).
    4.  Confirm the `curl` pod **cannot** `ping` or otherwise access the pods on non-allowed ports.
    5.  Verify the webhook is accessible on port `:10250` from the API server for admission control.
    6.  Simulate an upgrade from the previous release (with legacy unprefixed policy names) and verify that `cleanupMigratedNetworkPolicies()` removes all legacy objects and leaves only `eso-sys-*` and `eso-user-*` objects.
    7.  Verify the `migration-complete` annotation is written to the `ExternalSecretsConfig` CR after a successful cleanup, and that subsequent reconciles skip the deletion loop.
  
* **Proxy Egress Tests:**
    1.  On a proxy-configured cluster with `spec.appConfig.proxy.networkPolicyAllowProxyEgressAll: Managed` (the default), verify that `eso-sys-proxy-egress-core` is created in the `external-secrets` namespace and that ESO pods can successfully reach the proxy server.
    2.  On a non-proxy cluster with `networkPolicyAllowProxyEgressAll: Managed`, verify that `eso-sys-proxy-egress-core` is **not** created (because `getProxyConfiguration()` returns nil).
    3.  On a proxy-configured cluster with `networkPolicyAllowProxyEgressAll: Unmanaged`, verify that `eso-sys-proxy-egress-core` is **not** created, and that if it was previously created it is removed on the next reconcile.
    4.  Change `networkPolicyAllowProxyEgressAll` from `Unmanaged` to `Managed` on a proxy cluster and verify the policy is re-created on the next reconcile.

* **Naming and Validation Tests:**
    1.  Create a valid entry (e.g., `name: allow-external-secrets-egress`) and confirm the resulting Kubernetes object is named `eso-user-allow-external-secrets-egress`.

* **End-to-End (E2E) Tests:**
    1.  Run the existing `external-secrets` E2E test suite with the network policies enabled.
    2.  Create an ExternalSecret resource and verify it is successfully synced, which validates the entire flow from API server → webhook → controller → external provider. This implicitly tests the webhook ingress and the controller's egress capabilities.
    3.  Test secret synchronization from various providers (AWS Secrets Manager,GCP Secret Manager, etc.) to ensure external provider connectivity works through the network policies.
    4.  Verify that SecretStore and ClusterSecretStore resources can be created and validated by the webhook.
  
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

* **Upgrade:** On upgrade, the operator applies the new `eso-sys-` prefixed NetworkPolicy objects first, then runs `cleanupMigratedNetworkPolicies()` to remove legacy unprefixed objects (e.g., `deny-all-traffic`, `allow-to-dns`, etc.). This apply-before-delete ordering ensures there is no window where a required policy is absent.

  This function runs only after the apply step fully succeeds. On each reconcile it operates as follows:
  1. Check whether a `migration-complete` annotation is present on the `ExternalSecretsConfig` CR. If the annotation is present, skip the full deletion loop entirely.
  2. If the annotation is absent, list all NetworkPolicy resources in the namespace by label (`app.kubernetes.io/managed-by: external-secrets-operator` and `app.kubernetes.io/part-of: external-secrets-operator`) and by name, and delete any whose name is not in the desired set. The legacy unprefixed policies - `deny-all-traffic`, `allow-to-dns`, `allow-api-server-egress-for-main-controller`, etc. - carry the operator labels but are no longer in the desired set, so they get cleaned up. Stale user NPs are also removed by the same diff to avoid the duplicate NPs.
  3. After all deletions succeed, write the `migration-complete` annotation onto the `ExternalSecretsConfig` CR using the `fieldOwner` option so subsequent reconciles skip the deletion loop and avoid unnecessary GETs.

  > **Note:** `cleanupMigratedNetworkPolicies()` is a temporary migration helper. It must be removed after 3 releases. By that point every cluster is assumed to have been reconciled at least once under the new naming scheme and no legacy unprefixed objects will remain.

**Desired set after migration:**

  | New K8s name | Replaces |
  |---|---|
  | `eso-sys-deny-all-traffic` | `deny-all-traffic` |
  | `eso-sys-allow-to-dns` | `allow-to-dns` |
  | `eso-sys-allow-api-server-egress-for-main-controller` | `allow-api-server-egress-for-main-controller` |
  | `eso-sys-allow-api-server-egress-for-webhook` | `allow-api-server-egress-for-webhook` |
  | `eso-sys-allow-api-server-egress-for-cert-controller` | `allow-api-server-egress-for-cert-controller` |
  | `eso-sys-allow-api-server-egress-for-bitwarden-server` | `allow-api-server-egress-for-bitwarden-server` |
  | `eso-sys-proxy-egress-core` *(conditional)* | *(new)* |
  | `eso-user-<cr-name>` | `<cr-name>` |

* **Downgrade:** If a user downgrades to a version of the operator that is not aware of network policies, the `NetworkPolicy` objects will be orphaned (left behind). Since older versions operated in a default-allow world, these leftover restrictive policies could break the installation. The downgrade documentation must instruct the user to manually delete the `NetworkPolicy` objects from the `external-secrets-operator` and `external-secrets` namespaces before downgrading.

## Alternatives (Not Implemented)

* **Deny-All at Namespace Level:** An initial approach considered applying a single `podSelector: {}` deny-all policy to the entire namespace. However, this is less explicit. Using a pod selector for each `deny` policy ensures that the denial is clearly associated with the component it is intended to protect.

* **Single Combined Policy:** Another alternative was to create one large `NetworkPolicy` per namespace. This was rejected in favor of multiple smaller, more focused policies (e.g., one for API server egress, one for metrics ingress). This makes the purpose of each rule clearer and easier to manage and debug.

## Version Skew Strategy

This enhancement only involves adding `NetworkPolicy` resources, which are managed by the `external-secrets-operator`. There are no version skew concerns with other components, as the operator's version will be tied to the policies it deploys. The Kubernetes API for `NetworkPolicy` is stable.

## Operational Aspects of API Extensions

Two new API fields are introduced:

* **`spec.appConfig.proxy.networkPolicyAllowProxyEgressAll` (enum `Managed|Unmanaged`, default `Managed`):** Controls automatic proxy egress policy creation. Operators can observe the effect of this field by checking for the presence of the `eso-sys-proxy-egress-core` NetworkPolicy in the `external-secrets` namespace. Changing to `Unmanaged` removes the policy on the next reconcile.

## Support Procedures

Support personnel debugging potential issues should first check the `NetworkPolicy` resources in the `external-secrets` and `external-secrets-operator` namespaces. They should also be aware that NetworkPolicy objects are now named with `eso-sys-` and `eso-user-` prefixes. Commands such as `oc get networkpolicy -n external-secrets` will reflect the new names after upgrade.

1.  Verify the policies exist: `oc get networkpolicy -n external-secrets`.
2.  If a pod is suspected of having network connectivity issues, check its logs for connection timeout errors.
3.  Use the OpenShift Network Observability Operator or similar tools to visualize traffic and identify any connections being dropped by the policies.
