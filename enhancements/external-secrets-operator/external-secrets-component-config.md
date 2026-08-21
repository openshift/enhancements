---
title: external-secrets-component-config
authors:
  - "@sbhor"
  - "@bharath-b-rh"
reviewers:
  - "@mytreya-rh"
approvers:
  - "@mytreya-rh"
api-approvers:
  - "@mytreya-rh"
creation-date: 2025-12-1
last-updated: 2026-08-21
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-2419
  - https://redhat.atlassian.net/browse/OCPSTRAT-3454
  - https://redhat.atlassian.net/browse/RFE-7842
  - https://redhat.atlassian.net/browse/RFE-8685
  - https://redhat.atlassian.net/browse/ESO-266
  - https://redhat.atlassian.net/browse/ESO-417
  - https://redhat.atlassian.net/browse/ESO-532
  - https://redhat.atlassian.net/browse/ESO-533
see-also: []
replaces: []
superseded-by: []
---

# Component Configuration for external-secrets Operator

## Summary

The External Secrets Operator for Red Hat OpenShift provides limited configuration options via its `ExternalSecretsConfig` API, constraining user customization. This enhancement proposes extending the `ExternalSecretsConfig` API to allow comprehensive customization of the external-secrets deployment. The extended configuration options—including annotations, environment variables, and deployment/pod specifications will be available for all core components (Controller, Webhook, CertController, BitwardenSDKServer). This change provides administrators with greater control over the resource management and operational parameters of each component.

This enhancement further adds a first-class **`replicas`** field on per-component `deploymentConfigs` so GitOps-managed clusters can tune high availability without unsupported workarounds. It also introduces an `advancedOverrides` escape hatch on per-component configuration. This field applies a strategic merge patch to the component `Deployment`, covering scheduling knobs not yet available as first-class fields — for example pod anti-affinity, topology spread constraints, and core controller concurrency via `--concurrent`. Invalid patches cause the operator to set a `Degraded` condition. API godoc and product documentation warn administrators that `advancedOverrides` can overwrite first-class CRD settings and must not be used to add or modify containers, initContainers, or ports.

This enhancement proposal also adds support for injecting a custom PKI CA bundle into the `external-secrets` operand **core controller** pod via a `ConfigMap` reference in the `ExternalSecretsConfig` custom resource, enabling the controller to verify **TLS** (HTTPS) connections to external secret management systems (such as IBM/Thycotic Secret Server or HashiCorp Vault) without depending on cluster-wide proxy configuration. The operator mounts the referenced `ConfigMap` under `/etc/pki/tls/user-certs` and sets `SSL_CERT_DIR` so Go trusts both that directory and the default system location without replacing `/etc/pki/tls/certs`. Administrators may populate the `ConfigMap` manually or with projects such as `cert-manager`.

## Motivation

Administrators often need to control core operational parameters, lifecycle settings, and custom metadata without directly modifying the underlying operator-managed resources. Currently, any manual changes made directly to the operand resources or other component specifications are immediately overwritten by the operator, making persistent customization impossible. This hardening forces users to accept default settings that may not be optimal for their workloads. This proposal resolves this issue by providing a dedicated, supported configuration path through `ExternalSecretsConfig`, granting administrators the necessary flexibility to fine-tune essential specifications like revisionHistoryLimit, add crucial environment variables, and apply additional metadata for seamless and efficient integration into complex cluster environments.

Administrators also frequently run external secret management systems (for example IBM Secret Server, Thycotic, HashiCorp Vault) that use certificates signed by external PKI; the CA certificates must be available to the `external-secrets` controller for **TLS server certificate verification** on outbound HTTPS. On OpenShift, the Cluster Network Operator (CNO) injects the merged trusted CA bundle into `ConfigMaps` that carry the label **`config.openshift.io/inject-trusted-cabundle: "true"`**. That mechanism is wired to the cluster **`Proxy`** object: administrators distribute user-configured CA certificates cluster-wide by setting `Proxy.spec.trustedCA` (and related proxy fields when they use an HTTP/HTTPS proxy). Asking administrators to edit the `Proxy` CR solely to attach a CA bundle when they do not use an HTTP/HTTPS proxy is a poor fit for clusters that otherwise do not operate `Proxy`. Some `external-secrets` providers expose per-store CA options, but not all do, and repeating configuration across many stores increases maintenance overhead. This enhancement extends `ExternalSecretsConfig` with an operator-local `trustedCABundle` for controller-wide trust when that model is appropriate.

A single controller replica results in a reconciliation gap during node failures or pod evictions, until Kubernetes reschedules the pod. So a supported **`replicas`** field with proper **leader election** is needed for HA. Until first-class API fields cover every Deployment knob (affinity, topology spread, concurrency, etc.), an **`advancedOverrides`** escape hatch lets administrators apply a strategic merge patch to a component Deployment under explicit Degraded semantics. For example, large-scale deployments needing higher reconcile parallelism (`--concurrent`) can use `advancedOverrides` to tune the core controller container args.

### User Stories

- As an OpenShift administrator, I want to configure deployment lifecycle properties (e.g., revisionHistoryLimit) for external-secrets operand components using the `ExternalSecretsConfig` API so that I can control rollback behavior and optimize cluster resource consumption.
- As an OpenShift administrator, I want to apply configuration overrides to individual external-secrets components (Controller, Webhook, etc.) so that I can set component-specific environment variables or other operational parameters as needed.
- As an OpenShift Administrator, I want to define custom metadata (like annotations or labels) on the external-secrets component deployments via the `ExternalSecretsConfig` API so that the deployments correctly integrate with cluster policy tools, monitoring systems (e.g., Prometheus), and internal tooling without being overwritten.
- As an OpenShift Administrator, I need to set custom environment variables for specific components (e.g., the Controller) so that I can configure component behavior at runtime or securely integrate the operand with necessary external services.
- As an OpenShift Administrator, I want to reference a `ConfigMap` of custom CA bundle in `ExternalSecretsConfig`, so that the `external-secrets` controller can sync secrets from external secret management systems over **TLS** (HTTPS) using enterprise or private PKI.
- As a platform engineer, I want to configure controller TLS trust without touching the Proxy CR when our cluster has no HTTP/HTTPS proxy, so that we do not misuse or hollow out a cluster-wide object just to ship a PEM bundle.
- As a security engineer, I want custom roots added without replacing the container system trust store, so that the controller still trusts public CAs (for example cloud secret managers) while also trusting internal enterprise CAs.
- As a platform engineer, I want an explicit experimental escape hatch to strategic-merge-patch a component `Deployment` `spec` for knobs that are not yet first-class (for example pod anti-affinity), accepting that invalid patches mark the operand **Degraded**.
- As a site reliability engineer, I want Degraded conditions and events on ExternalSecretsConfig when advancedOverrides fails, so that I can detect and remediate misconfiguration without tailing pod logs.

### Goals

- Provide a declarative API for specifying deployment lifecycle overrides for each component via `ExternalSecretsConfig`.
- Provide a declarative API for adding custom annotations globally to all resources created for the `external-secrets` operand via `ExternalSecretsConfig`.
- Provide a declarative API for specifying custom environment variables uniquely for each component via `ExternalSecretsConfig`.
- Allow optional, supported injection of a user-supplied CA bundle so the `external-secrets` **core controller** can verify **TLS** to external HTTPS backends (enterprise PKI, private CAs).
- Automatically mount the referenced ConfigMap into the ESO core controller pod at `/etc/pki/tls/user-certs`, without overriding the system trust store at `/etc/pki/tls/certs`.
- Existing proxy-based CA bundle injection behavior (CNO-managed) is preserved unchanged and can coexist with new user configured CA bundle.
- Provide a declarative `replicas` field on per-component `deploymentConfigs` for every operand component (`ExternalSecretsCoreController`, `Webhook`, `CertController`, `BitwardenSDKServer`).
- Provide `advancedOverrides` (`runtime.RawExtension`) on per-component configuration as a strategic merge patch of that component's `Deployment`, with **Degraded** (`UserConfigurationError`) status on invalid or broken patch application.

### Non-Goals

- Exhaustive validation of individual configured values (e.g., validating that an environment variable value is semantically correct). Users should consult upstream documentation. Only basic structural validation (non-empty strings, list length limits, numeric bounds on `replicas`) will be performed.
- Setting resource limits (CPU/memory requests/limits) as dedicated API fields on a per-component basis. Resource limits are not available as first-class fields in this proposal; however, they can be configured via `advancedOverrides`.
- Applying the user configured CA bundle to webhook or unrelated sidecars unless a follow-up explicitly requires it.
- Automatic CA certificate rotation or lifecycle management. The operator mounts the ConfigMap as-is; certificate updates are the cluster administrator's responsibility.
- Supporting ConfigMaps from namespaces other than the `external-secrets` operand namespace (`external-secrets`), as Kubernetes does not allow pods to mount ConfigMaps from other namespaces.
- Guaranteeing that `advancedOverrides` patches produce a supportable configuration. The administrator is responsible for providing correct patch data.
- While advancedOverrides permits field-level flexibility, core operator-managed resource specifications are explicitly protected and cannot be mutated or overridden through this mechanism.


## Proposal

Extend the ExternalSecretsConfig API with:
1. A new `annotations` field for adding custom annotations globally to Deployments and Pod templates.
2. A new `componentConfigs` field for per-component deployment lifecycle overrides (`revisionHistoryLimit`, `replicas`, `overrideEnv`) and the `advancedOverrides` escape hatch.
3. A new `trustedCABundle` field for adding trusted CA bundle.

**For trusted CA bundle**

This proposal extends the `ExternalSecretsConfig` API with a new optional field `trustedCABundle` of type `ConfigMapKeyReference` under `spec.controllerConfig` (a **single** optional object, not a list). When set, the operator will:

- Validate that the referenced `ConfigMap` and key exist in the ESO operand namespace and that the value parses as **valid PEM-encoded CA data**.
  - If `optional: true` and the `ConfigMap` or key is **missing**, skip trusted-bundle mounting **without** error.
  - If the key is present but contains **no valid PEM**, set **Degraded** and do **not** roll out a broken trust configuration (**regardless** of `optional`).
- Mount the `ConfigMap` as a volume into the `external-secrets` core controller pod at `/etc/pki/tls/user-certs`.
- Set the `SSL_CERT_DIR` environment variable on the core controller container to `/etc/pki/tls/certs:/etc/pki/tls/user-certs`, causing Go's `crypto/x509` `loadSystemRoots()` to load certificates from both the system trust store and the custom CA directory into the same trust pool.

The custom path (`/etc/pki/tls/user-certs`) is intentionally separate from the system path (`/etc/pki/tls/certs`) to avoid overriding system CAs, which would break TLS connectivity to public services.

Both proxy-based CA injection (CNO-managed, at `/etc/pki/tls/certs`) and `trustedCABundle` injection can coexist; when both apply, `SSL_CERT_DIR` includes both inputs.

**`ConfigMap` with the CNO injection label:** If the `ConfigMap` referenced by `trustedCABundle` is labeled with `config.openshift.io/inject-trusted-cabundle: "true"`, operator **skips** mounting that reference for `trustedCABundle`.

**Interaction with `overrideEnv`:** The operator owns **`SSL_CERT_DIR`** (and, when applicable, **`SSL_CERT_FILE`**) on the **External Secrets core controller** for proxy/CNO trust and for **`trustedCABundle`** injection. **`overrideEnv`** therefore **must not** set **`SSL_CERT_DIR`** or **`SSL_CERT_FILE`** on **any** operand component: the **`ExternalSecretsConfig`** CRD extends the existing **`overrideEnv`** CEL rule so the API server **rejects** those names up front (same pattern as reserved prefixes such as `KUBERNETES_`). No runtime “ignore vs reject” choice is required for a valid CR.
| Situation | Expected behaviour |
|-----------|---------------------|
| `optional: false` (default) and missing `ConfigMap` or key | **Degraded**; do not patch the controller `Deployment` until valid. |
| `optional: true` and missing `ConfigMap` or key | Skip user bundle; no error for the missing reference alone. |
| Present key with **invalid PEM** | **Degraded** regardless of `optional`. |
| Referenced `ConfigMap` has **`config.openshift.io/inject-trusted-cabundle: "true"`** | A `ConfigMap` is already created, when proxy is configured, and its contents are mounted at `/etc/pki/tls/certs` path. Mounting it again under `/etc/pki/tls/user-certs` would be a duplicate. The operator **skips** the trustedCABundle volume mount. |

**For `replicas` (all components)**

Adds `spec.controllerConfig.componentConfigs[].deploymentConfigs.replicas` to set that component's Deployment replica count:

- **Unset:** defaults to `1` (same default as the Kubernetes Deployment resource).
- **Set:** applies the given replica count on every reconcile.
- CRD validation: minimum `1`, maximum `10`.

**For `advancedOverrides` (per component)**

Adds `spec.controllerConfig.componentConfigs[].advancedOverrides` (`runtime.RawExtension`). The value is applied as a **strategic merge patch** to the component's `Deployment`.


**Allowed `advancedOverrides` paths** :

- `spec.template.spec.affinity`
- `spec.template.spec.tolerations`
- `spec.template.spec.nodeSelector`
- `spec.template.spec.topologySpreadConstraints`
- `spec.template.spec.containers[*].args` (for operand flags such as `--concurrent`, `--client-burst`, `--client-qps`)
- `spec.template.spec.containers[*].resources`

 Adding a container, renaming a container, or setting any other container field is rejected.

Any other path — including `spec.replicas` (use the first-class `deploymentConfigs.replicas` field), `spec.revisionHistoryLimit`, `spec.selector`, `spec.template.metadata`, `spec.template.spec.serviceAccountName` / `serviceAccount`, `spec.template.spec.securityContext`, `spec.template.spec.imagePullSecrets`, `spec.template.spec.automountServiceAccountToken`, `spec.template.spec.volumes`, `spec.template.spec.restartPolicy`, `spec.template.spec.initContainers`, `spec.template.spec.ephemeralContainers`, and container `env` / `envFrom` / `lifecycle` / `name` / `ports` / `securityContext` / `volumeMounts` / `image` / `command` — is rejected with **Degraded** (`UserConfigurationError`).

- **Apply order** per component Deployment:
  1. Start from the operator baseline (bindata).
  2. Apply operator-managed settings (image, args, trusted CA mounts, labels, annotations, `revisionHistoryLimit`, `overrideEnv`, `replicas`).
  3. Check `advancedOverrides` against the allowlist above; if any path is not allowed, set **Degraded** (`UserConfigurationError`) and skip the patch.
  4. Strategic-merge-patch `Deployment` with the allowed overrides.
- **Error handling:** if the patch is not valid YAML, the merge fails, or the API server rejects the result, the operator sets **Degraded** (`UserConfigurationError`, e.g. `AdvancedOverridesInvalid` / `AdvancedOverridesApplyFailed`) and does not leave the operand in an inconsistent state.

### Workflow Description

```mermaid
sequenceDiagram
    actor Admin as Cluster Administrator
    participant CR as ExternalSecretsConfig CR
    participant Op as ESO Operator
    participant CM as ConfigMap<br/>(trusted-ca-bundle)
    participant Deps as Component Deployments
    participant Pod as Core Controller Pod

    Admin->>CR: Update controllerConfig:<br/>annotations / componentConfig / trustedCABundle
    Op->>CR: Watch → Reconcile

    rect rgb(0, 110, 75)
        Note over Op,Deps: 1 — Global Annotations
        Op->>Op: Validate keys<br/>(reject reserved prefixes)
        Op->>Deps: Merge and apply annotations to all<br/>Deployment + Pod template metadata
    end

    rect rgb(0, 80, 155)
        Note over Op,Deps: 2 — Component Config
        Op->>Op: Validate componentName enum,<br/>uniqueness, reserved env var names (CEL)
        Op->>Deps: Set revisionHistoryLimit on<br/>matched component Deployment
        Op->>Deps: Merge overrideEnv into<br/>matched component container spec
        Op->>Deps: Set replicas from<br/>deploymentConfigs on each component
        Op->>Deps: Enable leader election on core<br/>controller only when replicas > 1
    end

    rect rgb(120, 60, 140)
        Note over Op,Deps: 2b — advancedOverrides (per component)
        Op->>Op: Reject paths not on the allowlist<br/>(see Allowed advancedOverrides paths)
        alt disallowed path or invalid patch
            Op-->>CR: Set Degraded condition
        else allowed patch
            Op->>Deps: Strategic merge patch on<br/>allowed Deployment paths
            Op->>Deps: Re-assert protected fields<br/>and owned nested lists
        end
    end

    rect rgb(155, 95, 0)
        Note over Op,Pod: 3 — Trusted CA Bundle (core controller only)
        Op->>CM: Fetch referenced ConfigMap + key
        alt missing and optional: false
            Op-->>CR: Set Degraded condition<br/>(TrustedCABundleConfigMapNotFound /<br/>TrustedCABundleKeyNotFound)
        else ConfigMap labeled inject-trusted-cabundle: true
            Note over Op: Bundle already mounted by CNO/proxy<br/>path at /etc/pki/tls/certs — skip<br/>duplicate mount
            Op-->>Admin: Emit informational event
        else invalid PEM (regardless of optional)
            Op-->>CR: Set Degraded condition<br/>(InvalidPEMData)
        else valid PEM
            Op->>Deps: Patch core controller Deployment:<br/>add volume → /etc/pki/tls/user-certs<br/>set SSL_CERT_DIR on controller container
            Deps->>Pod: Rolling update
            Pod->>Pod: Loads system CAs (/etc/pki/tls/certs)<br/>+ enterprise CAs (/etc/pki/tls/user-certs)<br/>via SSL_CERT_DIR
            Pod-->>Admin: TLS to enterprise secret stores succeeds
        end
    end

    Deps-->>Admin: Rolling updates complete for<br/>annotations + componentConfig changes
```

**For Global Annotations:**

1. **User Configuration:** Administrator updates the `ExternalSecretsConfig` CR with the `controllerConfig.annotations` field containing custom key-value pairs.
2. **Validation:** The operator validates that annotation keys and values conform to Kubernetes annotation constraints.
3. **Reconciliation:** The operator merges user-specified annotations with any default annotations. User annotations take precedence in case of conflicts. Annotations are applied to both the Deployment metadata and Pod template metadata for all components.
4. **Rollout:** Kubernetes detects the annotation changes and performs updates as needed.

**For Component Configuration:**

1. **User Configuration:** Administrator updates the `ExternalSecretsConfig` CR, utilizing the `componentConfigs` list to specify configuration entries for a component (Controller, Webhook, etc.). This includes deployment-level overrides via `DeploymentConfig` (`revisionHistoryLimit`, `replicas`) and custom environment variables via `overrideEnv`.
2. **Validation:** It verifies the `componentName` against the allowed enum values and enforces uniqueness across the list. It strictly validates the `DeploymentConfig` field using the provided Kubernetes CEL validation rules, ensuring every entry uses the specified format. For `overrideEnv`, it validates that environment variable names and values conform to Kubernetes conventions, and that **reserved names** (including **`SSL_CERT_DIR`** and **`SSL_CERT_FILE`**, plus the existing prefix rules) are rejected by CEL on the CRD.
3. **Reconciliation:** The operator applies the `deploymentConfigs` values (e.g., `revisionHistoryLimit`, `replicas`) directly to the component's underlying Kubernetes Deployment resource spec. For `overrideEnv`, the operator merges user-specified environment variables with default variables, with user values taking precedence in case of conflicts. For the core controller, `--enable-leader-election=true` is set only when `replicas > 1`.
4. **Rollout:** Kubernetes detects the change in the Deployment's spec and performs a rolling update, applying the new setting to the component.

**For trusted CA bundle**

1. **User Configuration:** Administrator updates the `ExternalSecretsConfig` CR, setting the optional **`trustedCABundle`** object (`name`, `key`, `optional`) to reference a `ConfigMap` in the operand namespace.
2. **Validation:** Operator validates the referenced `ConfigMap` and key per the error-handling table in **For trusted CA bundle** above (including invalid PEM → **Degraded** regardless of `optional`).
3. **Reconciliation:** Operator updates the `external-secrets` **core controller** `Deployment` to add:
   - A volume referencing the `ConfigMap`.
   - A `volumeMount` at `/etc/pki/tls/user-certs` on the core controller container.
   - `SSL_CERT_DIR=/etc/pki/tls/certs:/etc/pki/tls/user-certs` on the core controller container.
4. **Rollout:** Kubernetes performs a rolling update when the `Deployment` spec changes.

**For `replicas`**

1. **User Configuration:** Administrator sets `spec.controllerConfig.componentConfigs[].deploymentConfigs.replicas` on the singleton `ExternalSecretsConfig` named `cluster` for one or more components.
2. **Validation:** API server enforces numeric bounds via CRD validation.
3. **Reconciliation:** For each component `Deployment` with a matching `componentConfigs` entry, the operator:
   - Sets `spec.replicas` from `deploymentConfigs.replicas` when present (else default `1`).
   - For `ExternalSecretsCoreController` only, sets `--enable-leader-election=true` when `replicas > 1`.
4. **Rollout:** Deployment rolling update. For the core controller with `replicas > 1`, standby pods acquire the lease on failover.

**For `advancedOverrides`**

1. **User Configuration:** Administrator adds a `componentConfigs` entry with `advancedOverrides` containing a partial `Deployment` manifest (for example container args for `--concurrent`, `--client-burst`, and `--client-qps`).
2. **Validation:** At reconcile time the operator rejects any path that is not on the **Allowed `advancedOverrides` paths** list and checks that the patch can be merged cleanly.
3. **Reconciliation:** The operator applies supported API fields first, then strategic-merge-patches the Deployment with the allowed overrides. If the patch targets a disallowed path, is invalid, or fails to apply, the operator sets **Degraded** (`UserConfigurationError`) and skips the patch. When core-controller `replicas > 1`, `--enable-leader-election=true` is kept on the core controller args.
4. **Rollout:** Standard Deployment rollout for the affected component.

### Implementation Details/Notes/Constraints

This enhancement extends the `ExternalSecretsConfig` for **operand customization**: administrators set **global annotations** on managed `Deployment`s and pod templates, **per-component** knobs (`deploymentConfigs` such as `revisionHistoryLimit` and `replicas`, plus `overrideEnv` and `advancedOverrides`), and optionally **`trustedCABundle`** for core-controller TLS trust—without editing generated workloads by hand.

- **Operand namespace:** `trustedCABundle` references must resolve to a `ConfigMap` in the **operand** namespace (fixed to `external-secrets`).
- **Volume semantics:** Mount the trusted CA `ConfigMap` as a **directory** volume on the core controller. Go's trust loading expects PEM files under the directories listed in `SSL_CERT_DIR`; the operator should surface a predictable filename such as **`ca-bundle.crt`** under `/etc/pki/tls/user-certs`. When the `ConfigMap` **key** is not already named `ca-bundle.crt`, use the volume's **`items`** list to **project** that key to the path `ca-bundle.crt` inside the mount (same pattern as other operators that must normalize key names):

  ```yaml
  volumes:
    - name: user-trusted-ca-bundle
      configMap:
        name: trusted-ca-bundle
        items:
          - key: ca-chain.crt
            path: ca-bundle.crt
  ```

  With this, the controller sees `/etc/pki/tls/user-certs/ca-bundle.crt` regardless of the original key name in the `ConfigMap`.

- **Proxy/CNO path unchanged:** Existing behaviour that ties the CNO-injected bundle to cluster `Proxy` configuration remains; `trustedCABundle` is additive for controller-local trust.

### API Extensions

```go
// ComponentConfig defines configuration overrides for a specific external-secrets component.
type ComponentConfig struct {
	// componentName identifies which external-secrets component this configuration applies to.
	// Valid component names: ExternalSecretsCoreController, Webhook, CertController, BitwardenSDKServer.
	// +kubebuilder:validation:Enum:=ExternalSecretsCoreController;Webhook;CertController;BitwardenSDKServer
	// +required
	//nolint:kubeapilinter // ComponentName is a listMapKey and must not have omitempty for proper patch identification
	ComponentName ComponentName `json:"componentName"`

	// deploymentConfigs specifies overrides for the Kubernetes Deployment resource of this component.
	// +optional
	DeploymentConfigs *DeploymentConfig `json:"deploymentConfigs,omitempty"`

    // overrideEnv allows setting custom environment variables for the component's container. These environment variables are merged with the default environment variables set by the operator. User-specified variables take precedence in case of conflicts.
    // Environment variables starting with KUBERNETES_, or EXTERNAL_SECRETS_ are reserved and cannot be overridden. HOSTNAME, SSL_CERT_DIR and SSL_CERT_FILE are also reserved.
    // +kubebuilder:validation:Optional
    // +kubebuilder:validation:XValidation:rule="self.all(e, !['KUBERNETES_', 'EXTERNAL_SECRETS_'].exists(p, e.name.startsWith(p)) && e.name != 'HOSTNAME' && e.name != 'SSL_CERT_DIR' && e.name != 'SSL_CERT_FILE')",message="Environment variable names starting with 'KUBERNETES_' or 'EXTERNAL_SECRETS_' are reserved; 'HOSTNAME', 'SSL_CERT_DIR', and 'SSL_CERT_FILE' are also reserved exact names."
    // +optional
    OverrideEnv []corev1.EnvVar `json:"overrideEnv,omitempty"`

	// advancedOverrides applies raw patches on top of the final operator generated Deployment spec.
	// WARNING: DO NOT USE UNLESS YOU KNOW EXACTLY WHAT YOU ARE DOING.
	// This field can overwrite your own first-class CRD settings. You must NOT use this
	// field to add or modify containers, initContainers, or ports, as doing so breaks
	// the structural integrity of the operand and will fail deployment reconciliation.
	// Only the allowlisted paths documented in this enhancement are applied.
	// +kubebuilder:validation:Optional
	// +kubebuilder:pruning:PreserveUnknownFields
	AdvancedOverrides *runtime.RawExtension `json:"advancedOverrides,omitempty"`
}

// ControllerConfig is for specifying the configurations for the controller to use while installing the `external-secrets` operand and the plugins.
type ControllerConfig struct {
	// annotations are for adding custom annotations to all the resources created for external-secrets deployment.
	// The annotations are merged with any default annotations set by the operator. User-specified annotations take precedence over defaults in case of conflicts.
	// Annotation keys containing domains `kubernetes.io/`, `openshift.io/`, `cert-manager.io/` or `k8s.io/` (including subdomains like `*.kubernetes.io/`) are not allowed.
	// +kubebuilder:validation:XValidation:rule="self.all(key, key.matches('^([a-z0-9]([-a-z0-9]*[a-z0-9])?(\\\\.[a-z0-9]([-a-z0-9]*[a-z0-9])?)*\\\\/)?([A-Za-z0-9][-A-Za-z0-9_.]*)?[A-Za-z0-9]$'))",message="Annotation keys must consist of alphanumeric characters, '-', '_' or '.', starting and ending with alphanumeric, with an optional lowercase DNS subdomain prefix and '/' (e.g., 'my-key' or 'example.com/my-key')"
	// +kubebuilder:validation:XValidation:rule="self.all(key, !key.contains('/') || key.split('/')[0].size() <= 253)",message="Annotation key prefix (DNS subdomain) must be no more than 253 characters"
	// +kubebuilder:validation:XValidation:rule="self.all(key, key.contains('/') ? key.split('/')[1].size() <= 63 : key.size() <= 63)",message="Annotation key name part must be no more than 63 characters"
	// +kubebuilder:validation:XValidation:rule="self.all(key, !key.matches('^([^/]*\\\\.)?(kubernetes\\\\.io|k8s\\\\.io|openshift\\\\.io)/'))",message="Annotation keys containing reserved domains 'kubernetes.io/', 'openshift.io/', 'k8s.io/' (including subdomains like '*.kubernetes.io/') are not allowed"
	// +kubebuilder:validation:XValidation:rule="self.all(key, !key.matches('^(cert-manager\\\\.io)/'))",message="Annotation keys containing reserved domain 'cert-manager.io/' are not allowed"
	// +kubebuilder:validation:MinProperties=0
	// +kubebuilder:validation:MaxProperties=20
	// +optional
	Annotations map[string]string `json:"annotations,omitempty"`

	// networkPolicies specifies the list of network policy configurations to be applied to external-secrets pods.
	// Already shipped; see external-secrets-network-policy.md.
	// +kubebuilder:validation:MinItems:=0
	// +kubebuilder:validation:MaxItems:=50
	// +listType=map
	// +listMapKey=name
	// +listMapKey=componentName
	// +optional
	NetworkPolicies []NetworkPolicy `json:"networkPolicies,omitempty"`

	// componentConfigs allows specifying deployment-level configuration overrides for individual external-secrets components. This field enables fine-grained control over deployment settings for each component independently.
	// Each component can only have one configuration entry.
	// +kubebuilder:validation:MinItems:=0
	// +kubebuilder:validation:MaxItems:=4
	// +listType=map
	// +listMapKey=componentName
	// +optional
	ComponentConfigs []ComponentConfig `json:"componentConfigs,omitempty"`

	// TrustedCABundle references a ConfigMap containing the CA certificates
	// required to verify external TLS endpoints (e.g., Proxies, External Secret Management Systems).
    // +optional
	TrustedCABundle *ConfigMapKeyReference `json:"trustedCABundle,omitempty"`
}

// DeploymentConfig defines configuration overrides for a Kubernetes Deployment resource.
type DeploymentConfig struct {
	// revisionHistoryLimit specifies the number of old ReplicaSets to retain for rollback purposes.
	// This allows rolling back to previous deployment versions using 'kubectl rollout undo'.
	// Must be at least 1 to ensure rollback capability. Maximum value is 50 to limit resource usage.
	// If not specified, defaults to 10.
	// +kubebuilder:default:=10
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=50
	// +optional
	RevisionHistoryLimit *int32 `json:"revisionHistoryLimit,omitempty"`

	// replicas sets the desired replica count for this component's Deployment.
	// When omitted, defaults to 1. For ExternalSecretsCoreController, replicas > 1 enables --enable-leader-election.
	// +kubebuilder:default:=1
	// +kubebuilder:validation:Minimum:=1
	// +kubebuilder:validation:Maximum:=10
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
}

// ConfigMapKeyReference refers to a specific key within a ConfigMap.
type ConfigMapKeyReference struct {
    // name of the ConfigMap resource being referred to.
    // +kubebuilder:validation:MinLength:=1
    // +kubebuilder:validation:MaxLength:=253
    // +kubebuilder:validation:Required
    Name string `json:"name"`

    // key is the specific key in the ConfigMap to be utilized.
    // If not specified, defaults to "ca-bundle.crt".
    // +kubebuilder:validation:MinLength:=1
    // +kubebuilder:validation:MaxLength:=253
    // +kubebuilder:validation:Pattern:=^[-._a-zA-Z0-9]+$
    // +kubebuilder:default:="ca-bundle.crt"
    // +kubebuilder:validation:Optional
    Key string `json:"key,omitempty"`

    // optional specifies whether the ConfigMap or its key must be defined.
    // If true and the ConfigMap or key is missing, the operator will skip
    // the dependent logic instead of erroring.
    // +kubebuilder:default:=false
    // +kubebuilder:validation:Optional
    Optional *bool `json:"optional,omitempty"`
}
```

#### Example User Configuration

**Configure RevisionHistoryLimit for the Controller:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    componentConfigs:
      - componentName: ExternalSecretsCoreController
        deploymentConfigs:
          revisionHistoryLimit: 5
```

**Add custom annotations (applied to all components):**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    annotations:
      "example.com/custom-annotation": "my-value"
```

**Set custom environment variables for a component:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    componentConfigs:
      - componentName: ExternalSecretsCoreController
        overrideEnv:
          - name: GOMAXPROCS
            value: "4"
```

**Configure component replicas:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    componentConfigs:
      - componentName: ExternalSecretsCoreController
        deploymentConfigs:
          replicas: 2
      - componentName: Webhook
        deploymentConfigs:
          replicas: 2
```

**Use `advancedOverrides` to set `--concurrent`, `--client-burst`, and `--client-qps`:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    componentConfigs:
      - componentName: ExternalSecretsCoreController
        advancedOverrides:
          spec:
            template:
              spec:
                containers:
                  - name: external-secrets
                    args:
                      - --concurrent=20
                      - --client-burst=200
                      - --client-qps=100
```

**Configure `trustedCABundle`:**

1. The ConfigMap exists in the external-secrets namespace containing the trusted CA certificates:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: trusted-ca-bundle
  namespace: external-secrets
data:
  ca-bundle.crt: |
    -----BEGIN CERTIFICATE-----
    MIIDXTCCAkWgAwIBAgIJAJC1HiIAZAiUMA...
    -----END CERTIFICATE-----
    -----BEGIN CERTIFICATE-----
    MIIDfTCCAmWgAwIBAgIBATANBgkqhkiG...
    -----END CERTIFICATE-----
```

2. Reference the ConfigMap in the `ExternalSecretsConfig` CR

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    trustedCABundle:
      name: trusted-ca-bundle
      key: ca-bundle.crt
      optional: false
```

**Combined: annotations (global) with scale/HA, component-specific deployment config, overrideEnv, advancedOverrides, and trustedCABundle:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: ExternalSecretsConfig
metadata:
  name: cluster
spec:
  controllerConfig:
    annotations:
      "example.com/custom-annotation": "my-value"
    componentConfigs:
      - componentName: ExternalSecretsCoreController
        deploymentConfigs:
          revisionHistoryLimit: 10
          replicas: 2
        overrideEnv:
          - name: GOMAXPROCS
            value: "4"
        advancedOverrides:
          spec:
            template:
              spec:
                topologySpreadConstraints:
                  - maxSkew: 1
                    topologyKey: topology.kubernetes.io/zone
                    whenUnsatisfiable: ScheduleAnyway
                    labelSelector:
                      matchLabels:
                        app.kubernetes.io/name: external-secrets
                containers:
                  - name: external-secrets
                    args:
                      - --concurrent=20
                      - --client-burst=200
                      - --client-qps=100
      - componentName: Webhook
        deploymentConfigs:
          revisionHistoryLimit: 3
          replicas: 2
    # trusted CA certificates
    trustedCABundle:
      name: trusted-ca-bundle
      key: ca-bundle.crt
      optional: false
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

The ESO operator runs in the hosted cluster's data plane, not in the management cluster. The `ExternalSecretsConfig` CR and the trusted CA ConfigMap both reside in the hosted cluster. There are no unique considerations for Hypershift — the feature works identically to standalone clusters. The ConfigMap must be in the `external-secrets` namespace within the hosted cluster.

#### Standalone Clusters

This is the primary topology for this enhancement. The feature is fully applicable.

#### Single-node Deployments or MicroShift

**Single-node OpenShift (SNO)**: Applicable; behavior matches standalone.

**MicroShift**: External Secrets Operator is not part of the MicroShift product offering; this enhancement does not apply to MicroShift unless that changes.

### Risks and Mitigations

* **Risk:** The primary risk lies in administrators setting the RevisionHistoryLimit value too low (for example, setting it to 0 or 1). Doing so severely limits or completely eliminates the component's ability to perform quick rollbacks to previous stable versions. If a new deployment fails, recovery will be slower and more complex if there are no historical ReplicaSets to instantly switch back to.
    * **Mitigation:** strongly recommend a safe minimum value (typically between 3 and 5) to ensure operational continuity and maintain reasonable rollback capabilities.

* **Risk:** Users may accidentally override critical arguments required for proper operation.
    * **Mitigation:** The operator can protect certain critical arguments from being overridden and will log warnings if users attempt to do so.

* **Risk:** Users may override critical environment variables required for proper component operation.
    * **Mitigation:** The operator can protect certain critical environment variables from being overridden and will log warnings if users attempt to do so.

* **Risk:** Configuration changes may cause service disruption during rollout.
    * **Mitigation:** Standard Kubernetes rolling update strategies will minimize disruption. Users can control rollout behavior through the deployment's update strategy.

* **Risk:** Malicious CA injection.
    * **Mitigation:** ConfigMap must exist in `external-secrets` namespace; only cluster admins with write access to that namespace should be able to configure this, by restricting RBAC to cluster admins.

* **Risk:** Invalid PEM or mis-typed `ConfigMap` reference breaks secret sync.
    * **Mitigation:** Operator validates PEM on reconcile; sets **Degraded** with a clear message; does not roll out broken trust when `optional: false` (see error-handling table).

* **Risk:** User sets `SSL_CERT_DIR` or `SSL_CERT_FILE` in `overrideEnv`, conflicting with operator-managed trust.
    * **Mitigation:** CRD **CEL** on `overrideEnv` rejects those names at create/update (see **Interaction with `overrideEnv`**); document in user-facing docs; add API validation tests.

* **Risk:** Setting a very high `--concurrent` value via `advancedOverrides` overwhelms the API server or external providers.
    * **Mitigation:** Document that administrators should raise concurrency gradually and watch API/QPS metrics.

* **Risk:** Multiple replicas without leader election would double-reconcile the core controller.
    * **Mitigation:** When core-controller `replicas > 1`, the operator sets `--enable-leader-election=true` and keeps that flag after an allowed `advancedOverrides` args patch.

* **Risk:** `advancedOverrides` produces an unsupportable or broken Deployment.
    * **Mitigation:** Treat the field as unsupported/experimental; reject paths not on the allowlist; on patch/apply failure set **Degraded** with a descriptive message; document that users own patch correctness; prefer promoting common knobs to first-class fields later.

* **Risk:** Users add sidecars, init containers, or volumes via strategic merge on nested lists, creating unsupported or insecure runtime shape.
    * **Mitigation:** Only allowlisted paths are applied (see **Allowed `advancedOverrides` paths**). Adding or replacing `containers` / `initContainers` / `volumes` is rejected with **Degraded**.

### Drawbacks

- Increased API surface complexity for users who don't need customization.
- Potential for misconfiguration leading to operational issues.
- Administrators must create, update, and delete ConfigMap contents themselves; there is no operator-managed CA rotation beyond what Kubernetes volume updates provide.
- `advancedOverrides` is explicitly unsupported; incorrect patches may cause **Degraded** conditions that require support engagement to resolve.

## Test Plan

* **Unit Tests:**
    1. Test validation of componentName uniqueness.
    2. Test validation of `deploymentConfigs.revisionHistoryLimit` values.
    3. Test that invalid `deploymentConfigs` values are handled gracefully.
    4. Test annotation merging logic with defaults and user overrides.
    5. Test that reserved annotation prefixes are rejected.
    6. Test environment variable merging logic with defaults and user overrides.
    7. Test that reserved environment variable prefixes are rejected.
    8. Test that environment variable names conform to Kubernetes conventions.
    9. Test that `overrideEnv` CEL rejects `SSL_CERT_DIR` and `SSL_CERT_FILE` for any component.
    10. Test for all combinations of proxy/trustedCABundle being set or unset.
    11. Test PEM validation logic; assert **Degraded** for invalid PEM regardless of `optional`.
    12. Test optional field behavior for missing ConfigMap and missing key.
    13. When referenced `ConfigMap` has `config.openshift.io/inject-trusted-cabundle: "true"`, assert reconcile **skips** user-bundle mount and does **not** set **Degraded** solely for that reason.
    14. Test that `replicas` sets `Deployment.spec.replicas` on the core controller; unset defaults to `1`.
    15.Test that valid `advancedOverrides` are merged into the `Deployment` for allowlisted paths (including `--concurrent` / `--client-burst` / `--client-qps` args).s.
    16. Test that invalid `advancedOverrides` sets **Degraded** (`UserConfigurationError`).
    17. Test that first-class fields (`image`, `replicas`, `--enable-leader-election` when core `replicas > 1`) are not overridden by `advancedOverrides`.
    18. Test that `advancedOverrides` targeting a path not on the allowlist is rejected with **Degraded** (`UserConfigurationError`).
    19. Test that `--enable-leader-election` is set on the core controller only when `replicas > 1`, and is absent when `replicas` is `1`.
    20. Test that the temporary env-var args hatch (v1.1–v1.3) and `advancedOverrides` args patches produce the same operand args when both are used on overlapping versions, and that `advancedOverrides` wins after the env-var hatch is removed.

* **Integration Tests:**
    1. Deploy the operator and create an `ExternalSecretsConfig` with component configuration.
    2. Verify that `deploymentConfigs.revisionHistoryLimit` is correctly applied to the deployment's `spec.revisionHistoryLimit`.
    3. Verify that specified annotations appear on both Deployment and Pod template.
    4. Verify that specified environment variables appear in the container spec.
    5. Update the configuration and verify the deployment is updated accordingly.
    6. Remove the configuration and verify defaults are restored.
    7. Attempt to apply a configuration that fails XValidation and verify the API server rejects the resource with the appropriate error message (including `overrideEnv` containing `SSL_CERT_DIR` or `SSL_CERT_FILE`).
    8. Test annotation override behavior when user annotation conflicts with operator default.
    9. Test environment variable override behavior when user variable conflicts with operator default.
    10. Deploy with `trustedCABundle`; assert volume mount and `SSL_CERT_DIR` exist only on the core controller container; assert Degraded when reference invalid with `optional: false`; assert silent skip when `optional: true` and reference missing; assert Degraded for invalid PEM even when `optional: true`.
    11. With proxy configured and `trustedCABundle` referencing a ConfigMap labeled `config.openshift.io/inject-trusted-cabundle: "true"`, assert the operator does not add the trustedCABundle volume mount (since the bundle is already handled for the proxy path), does not set Degraded.
    12. Set `deploymentConfigs.replicas` on multiple components; assert replica counts survive multiple reconcile loops (no revert to hardcoded defaults).
    13. Apply `advancedOverrides` for allowlisted paths (affinity/topology and `--concurrent` / `--client-burst` / `--client-qps` args); assert merged into live Deployment; assert **Degraded** (`UserConfigurationError`) for malformed RawExtension; assert **Degraded** and no Deployment change when the patch targets a path not on the allowlist.
    14. On a version that still supports the env-var args hatch, set both the env-var hatch and `advancedOverrides` args; assert the resulting container args and that the hatch can be used as a fallback after downgrade (see Upgrade / Downgrade).

* **End-to-End (E2E) Tests:**
    1. Test each component type (Controller, Webhook, CertController, BitwardenSDKServer) individually.
    2. Configure `deploymentConfigs.revisionHistoryLimit` and verify old ReplicaSets are cleaned up accordingly.
    3. Configure custom environment variables and verify they are available in the running container.
    4. Verify that the operator correctly handles invalid configurations gracefully.
    5. Configure ESO to connect to an internal test secret store (self-signed cert, e.g., vault); verify secrets sync successfully after setting trustedCABundle. Verify no regression for stores using public CAs (e.g., AWS Secrets Manager).
    6. Verify proxy-based CA injection still works when both proxy and trustedCABundle are configured.
    7. Scale core controller `deploymentConfigs.replicas` to `>1`; verify leader election (single active reconciler / lease) and failover when the leader pod is deleted; verify secret sync continues. Scale webhook `replicas` independently and verify the webhook Service still serves.
## Graduation Criteria

This feature will be delivered as GA directly, as it uses stable Kubernetes APIs and provides essential operational flexibility.

* All API fields are implemented with proper validation.
* Deployment config application logic is complete (e.g., `revisionHistoryLimit`).
* Annotation merging logic is complete and applies to both Deployment and Pod template.
* Environment variable merging logic is complete and applies to the container spec.
* `replicas` is applied per component via `deploymentConfigs` and persists across reconcile.
* `advancedOverrides` strategic merge and **Degraded** failure paths are implemented for the allowlisted paths.
* Leader election is enabled for the core controller only when `replicas > 1`.
* All tests outlined in the Test Plan are passing.
* Documentation includes examples for common use cases.

### Dev Preview → Tech Preview

Not applicable. This feature will be enabled by default at GA.

### Tech Preview → GA

Not applicable. This feature will be enabled by default at GA.

### Removing a deprecated feature

The temporary operand args env-var hatch is deprecated in v1.3 and **removed in v1.4**. See **Temporary operand args env-var hatch** under Alternatives. Administrators must migrate to `advancedOverrides` during the v1.3 window.

## Upgrade / Downgrade Strategy

* **Upgrade:** On upgrade, `annotations`, `componentConfigs` (including shipped `deploymentConfigs.revisionHistoryLimit` and `overrideEnv`, plus new `replicas` and `advancedOverrides`), and `trustedCABundle` remain available. Existing installations without the new fields continue with defaults (`replicas=1`). Users can optionally add scale/HA and escape-hatch settings after upgrade. During v1.3, the temporary env-var args hatch remains available (deprecated) so administrators can migrate to `advancedOverrides`.

* **Downgrade:** If a user downgrades to a version that does not support these fields, the older operator **ignores** unknown `spec.controllerConfig` keys (or they are pruned from stored objects depending on CRD schema). Effects include:
  * **`annotations` / `componentConfigs`:** Deployments revert toward operator defaults; user annotations and `overrideEnv` from the newer schema are lost.
  * **`advancedOverrides`:** The patch payload is pruned with the newer CRD schema. Operand Deployments revert to operator defaults for scheduling and args. On a version that still supports the temporary env-var args hatch (v1.1–v1.3), administrators can restore `--concurrent` / `--client-burst` / `--client-qps` (and similar) through that hatch until they upgrade again.
  * **`replicas`:** Component Deployments return to single replica; HA tuning is lost.
  * **`trustedCABundle`:** The custom CA volume and `SSL_CERT_DIR` user path are no longer applied. TLS to backends that require enterprise-only roots may fail with `x509: certificate signed by unknown authority` until the cluster is upgraded again or trust is supplied via another supported path (for example `Proxy.spec.trustedCA` and the CNO-injected bundle when `Proxy` is in use).

  Treat downgrade across this API split as an **availability risk** for external secret sync wherever custom trust, HA, or elevated concurrency was required.

## Alternatives (Not Implemented)

* **Validating Webhook for Argument Semantics:** A validating admission webhook could be implemented to perform semantic validation of override values against upstream external-secrets component schemas. This would provide pre-flight validation of override keys and semantic values, enabling the early rejection of invalid configurations before deployment rollout, and offering user-friendly error messages. This could be reconsidered in future iterations if runtime validation failures (due to invalid values for supported keys) become a significant operational burden.

* **Cluster-Wide Proxy Object trustedCA Field:** OpenShift's `Proxy` object supports a `trustedCA` field that works independently of HTTP/HTTPS proxy settings. Modifying operator logic to always create the CNO-labeled ConfigMap (removing the proxy check) would allow CA injection via the existing CNO mechanism with minimal code changes and no API changes to ExternalSecretsConfig. This requires cluster-admin access to modify the cluster-wide Proxy object, affects all workloads cluster-wide (not just ESO), and does not support ESO-specific CA configuration. It could be the right choice when Proxy is already part of cluster operations.

* **Service Mesh (Istio/OpenShift Service Mesh):** Use Istio/OpenShift Service Mesh for mTLS and CA management. This introduces significant infrastructure overhead and complexity for a problem solved more simply at the operator level. Not all customers have or want a service mesh.

* **Per-SecretStore `caProvider` / `caBundle`:** The upstream external-secrets project supports `caBundle` and `caProvider` on individual `SecretStore` and `ClusterSecretStore` resources for some providers, but not all. It requires configuring CA references on every store, cannot express a single global controller trust bundle, and increases maintenance burden during CA rotation.

* **Temporary operand args env-var hatch (v1.1–v1.3 only, removed in v1.4):** Until `advancedOverrides` lands in v1.3, administrators need a way to override operand container args (for example `--concurrent`, `--loglevel`) without unsupported Deployment patches that the operator reverts. A **temporary, operator-level env-var escape hatch** bridges this gap and is backported to v1.1/v1.2. It is not the long-term API: `advancedOverrides` is the supported, declarative path. Administrators should migrate during the v1.3 window; the env vars are **removed in v1.4**.

| Version | Env-var escape hatch | `advancedOverrides` | Action required |
|---------|---------------------|---------------------|-----------------|
| v1.1–v1.2 | Available | Not available | Use env vars if needed. |
| v1.3 | Available (deprecated) | Available | Migrate to `advancedOverrides`. |
| v1.4+ | **Removed** | Available | Env vars no longer work; use `advancedOverrides`. |

## Version Skew Strategy

The External Secrets Operator and its operands (controller, webhook, cert-controller, BitwardenSDKServer) are delivered as a single OLM bundle and upgraded atomically. The operator image and all operand images advance together; there is no supported configuration where an older operand runs against a newer operator version or vice versa.

Because the new `annotations`, `componentConfigs` (including `deploymentConfigs.replicas` and `advancedOverrides`), and `trustedCABundle` fields are applied by the operator during reconciliation after the bundle upgrade completes, there is no window in which an operand pod would attempt to read or act on these fields independently. The fields are purely operator-consumed: the operator reads them and patches the operand `Deployment` specs accordingly.

No version skew handling is therefore required for this enhancement.

## Operational Aspects of API Extensions

The `annotations`, `componentConfigs` (including `deploymentConfigs.replicas` and `advancedOverrides`), and `trustedCABundle` API extensions follow standard Kubernetes patterns:

* **Failure Modes:**
  * Invalid configurations will be rejected by the API server validation (annotations, `componentConfigs`, CEL rules, numeric bounds on `replicas`).
  * Invalid annotation formats will be rejected at the API level.
  * Invalid environment variable names will be rejected at the API level (including **`SSL_CERT_DIR`** and **`SSL_CERT_FILE`** in `overrideEnv` per CEL, alongside existing reserved-prefix rules).
  * For **`trustedCABundle`:** If the referenced `ConfigMap` or key is missing when `optional: false`, or the key contains invalid PEM, the operator sets **`Degraded`** on `ExternalSecretsConfig`, logs a clear error, and **does not** apply a broken controller `Deployment` patch—the running controller keeps its prior trust configuration until the spec is valid. If the referenced `ConfigMap` carries `config.openshift.io/inject-trusted-cabundle: "true"`, it is already created and mounted at `/etc/pki/tls/certs` when a proxy is configured. The operator **skips** the trustedCABundle volume mount to avoid a duplicate.
  * For **`advancedOverrides`:** malformed YAML, a path that is not on the **Allowed `advancedOverrides` paths** list, or a failed strategic merge/apply sets **`Degraded`** (`UserConfigurationError`) with a descriptive reason. The operator does not apply the patch. When core-controller `replicas > 1`, `--enable-leader-election=true` is kept on the core controller args.

* **Support Procedures:** Administrators can verify the applied configuration by inspecting operand `Deployment` objects and comparing them to `ExternalSecretsConfig`. For the trusted CA path, also verify the referenced `ConfigMap`, controller pod env `SSL_CERT_DIR`, and files under `/etc/pki/tls/user-certs`. For scale/HA, verify `spec.replicas` and the leader-election lease in the operand namespace.

## Support Procedures

Support personnel debugging configuration issues should:

1. Verify the `ExternalSecretsConfig` resource (resource name is commonly `cluster`; plural resource is `externalsecretsconfigs`, short names include `esc`):
   `oc get externalsecretsconfigs cluster -o yaml`
2. Inspect status conditions (including **Degraded** reasons for `trustedCABundle` or `advancedOverrides`):
   `oc get externalsecretsconfigs cluster -o jsonpath='{.status.conditions}'`
3. Compare the operand deployment spec to the expected configuration (deployment name may vary by release; confirm in-namespace):
   `oc get deployment -n external-secrets -o yaml`
4. Verify custom annotations on Deployment and Pod template: `.metadata.annotations` and `.spec.template.metadata.annotations`.
5. Verify custom environment variables: `.spec.template.spec.containers[*].env`.
6. Verify core controller args include `--enable-leader-election=true` only when core-controller `replicas > 1`:
   `oc get deployment external-secrets -n external-secrets -o jsonpath='{.spec.template.spec.containers[0].args}'`
7. Verify replica count and ready replicas on each component Deployment; for HA, inspect the leader election lease:
   `oc get lease -n external-secrets`
8. Check pod logs for TLS errors (`x509: certificate signed by unknown authority`) or env merge issues.
9. Review events: `oc get events -n external-secrets`
10. If a pod fails to start, check the container termination message and logs.
11. If **Degraded** mentions `advancedOverrides`, treat the patch payload as user config error; validate YAML and strategic-merge semantics before escalating. If the reason indicates a disallowed path, keep only allowlisted fields (see **Allowed `advancedOverrides` paths**).