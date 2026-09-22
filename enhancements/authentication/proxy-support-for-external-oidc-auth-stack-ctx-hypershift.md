## Information Sources

- ./proxy-support-for-external-oidc-auth-stack-ctx-standalone.md
- https://redhat.atlassian.net/browse/CNTRLPLANE-3740
  - https://github.com/openshift/enhancements/pull/2050 (extends ./external-oidc-additional-identity-information-sources.md)
  - https://github.com/openshift/hypershift/pull/8921

## Implementation Notes

- There is no previous `AuthenticationComponentProxy` feature gate logic on HyperShift as it was only implemented on standalone.
- This context assumes the HyperShift implementation of `ExternalOIDCExternalClaimsSourcing` has landed. That work owns the External OIDC topology: detecting `externalClaimsSources`, keeping `openshift-oauth-apiserver` deployed in External OIDC mode, generating/mounting its External OIDC configuration, and configuring kube-apiserver to use its TokenReview webhook. This enhancement layers component-proxy support onto that ready path.
- HyperShift's existing `.spec.configuration.proxy` is the cluster-wide guest/data-plane proxy. It is reconciled into the guest `proxy.config.openshift.io/cluster` resource and can affect NodePool configuration. It must not be reused for an authentication-component proxy: doing so gives proxy configuration to the whole cluster and defeats the component-scoped egress objective.
- `.spec.configuration.authentication` embeds `configv1.AuthenticationSpec`, the API for `authentication.config.openshift.io/cluster`. The standalone component-proxy API is instead `operatorv1.AuthenticationSpec.Proxy`, on the distinct `operator.openshift.io/Authentication` resource. Adding a proxy field directly to `configuration.authentication` would conflate these two API ownership domains and require an upstream config API change.

### Proposed HyperShift configuration path

HyperShift already exposes `HostedCluster.spec.operatorConfiguration.openShiftOAuthAPIServer`, copied to the corresponding `HostedControlPlane` field. Today its `OpenShiftOAuthAPIServerOperatorSpec` contains only `logLevel`, which CPO uses to set the oauth-apiserver `--v` argument. It is nevertheless the configuration surface for the precise HCP Deployment that needs proxy access.

Prefer extending `OpenShiftOAuthAPIServerOperatorSpec` with an External OIDC proxy field, conceptually:

```yaml
spec:
  operatorConfiguration:
    openShiftOAuthAPIServer:
      proxy:
        httpsProxy: http://proxy.example.com:8080
        noProxy:
        - .svc
        trustedCA:
          name: auth-proxy-ca
```

The exact API type and feature-gate annotations require API review, but its semantics should mirror `operatorv1.AuthenticationProxyConfig`: when supplied it replaces the cluster-wide proxy for External OIDC oauth-apiserver traffic; when omitted it falls back to the cluster-wide proxy. The referenced CA ConfigMap follows normal HyperShift configuration-reference handling: it is created in the HostedCluster namespace and copied to the HCP namespace.

CPO should consume this HCP input when it configures the management-cluster `openshift-oauth-apiserver` Deployment. It resolves the effective proxy, renders `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` on the External OIDC OAuth API server container, and mounts a synchronized `ca-bundle.crt` when `trustedCA` is set. Proxy environment changes roll the Deployment; mounted CA-content changes should be hot-reloaded by the operand without a rollout.

HCCO may reflect the same desired proxy configuration into the guest `operator.openshift.io/Authentication/cluster` object for API parity and observability, but it should not be CPO's source of truth. CPO owns an HCP-namespace Deployment while the guest resource is reconciled asynchronously; making CPO read it would add a guest-to-control-plane dependency and unclear ownership.

### Required implementation responsibilities

This is not entirely CPO work. The components and responsibilities are:

| Component | Required change |
| --- | --- |
| HyperShift operator | Extend the API and copy the new component proxy `trustedCA` ConfigMap from the HostedCluster namespace into the HCP namespace. The completed External Claims Sourcing work owns synchronization of the ConfigMaps and Secrets referenced by `externalClaimsSources`. Since the component-proxy reference is under `OperatorConfiguration`, `api/util/configrefs` needs an equivalent path for that area. |
| CPO | Use the External Claims Sourcing implementation's predicate/configuration path for the OAuth API server. Resolve and inject the component proxy and mounted proxy CA into that Deployment. Do not recreate the completed topology/config-generator work. |
| CPO validation path | Implement the issuer discovery validation that CAO performs on standalone clusters. Existing HyperShift CPO validation generates the KAS auth configuration but does not make a network request to the issuer. The CPO Pod must therefore use the same effective component proxy and trust roots while fetching the discovery document. This is control-plane traffic, not OAuth API server traffic. |
| OAuth API server | At runtime, use its proxy environment and trust bundle for OIDC discovery/JWKS refresh and external-claim source requests. |
| HCCO (optional) | Mirror the desired component proxy into the guest `operator.openshift.io/Authentication` resource for parity; it does not own HCP-side deployment configuration. |

The prerequisite External Claims Sourcing implementation supplies the separate oauth-apiserver External OIDC config generator. The existing `kas.GenerateAuthConfig` remains the direct-KAS structured-authentication generator and has no representation for `externalClaimsSources`. Proxy work should consume the prerequisite's established OAuth API server mode, generated-config ConfigMap, and mount location rather than extending `kas.GenerateAuthConfig`.

The two CAs have separate purposes and must both be available where needed: the issuer/source CA validates TLS to the IdP or claim endpoint; the component proxy `trustedCA` validates TLS to an HTTPS or TLS-intercepting proxy. CPO's validation client needs the applicable trust roots as well as the resolved proxy. The oauth-apiserver needs the generated issuer/source CA configuration plus the mounted proxy CA path.

HyperShift does contain an `authentication-operator render` init container in the kube-apiserver Pod, but it only renders bootstrap manifests. It is not a running CAO reconciliation controller. In addition, CPO removes the cluster-authentication-operator deployment manifest from the hosted payload. Consequently, the standalone CAO External OIDC controller's discovery validation does not run in a hosted control plane today. CPO must implement or reuse equivalent validation; otherwise issuer configuration errors are discovered only later by the OAuth API server at runtime.

### References

- https://github.com/openshift/hypershift/blob/38be02d5866ef573c8930e5ca0aebf2e7b1c8f17/vendor/github.com/openshift/hypershift/api/hypershift/v1beta1/hostedcluster_types.go#L2884

## HyperShift Notes

### Architecture overview

HyperShift separates the public desired-state API from the management-cluster control plane that realizes it:

```text
Management cluster

HostedCluster (HC)                         HostedControlPlane (HCP)
in the user namespace                      in a dedicated HCP namespace
─────────────────────                      ───────────────────────────
Public desired cluster declaration  ───►   Internal control-plane declaration
                                           and its Pods, Services, Secrets,
                                           ConfigMaps, and RBAC
```

The HyperShift operator watches a `HostedCluster`, creates its corresponding `HostedControlPlane`, and copies user-referenced ConfigMaps and Secrets from the HostedCluster namespace to the HCP namespace. Pods can mount only same-namespace objects, so the copy provides both namespace isolation and a local mountable input for control-plane workloads.

```text
HostedCluster namespace                   HCP namespace
  HostedCluster/my-cluster                  HostedControlPlane/my-cluster
  user ConfigMap/auth-proxy-ca  ───────►    copied ConfigMap/auth-proxy-ca
                                             kube-apiserver, etcd, OAuth API server,
                                             CPO and HCCO Pods
```

**CPO** (Control Plane Operator) runs once per HCP in the HCP namespace. It manages the physical control-plane workloads in the management cluster: Deployments, Services, certificates, volumes, and Pod configuration. It is the owner of the OAuth API server Deployment and therefore configures its proxy environment and CA mount.

**HCCO** (Hosted Cluster Config Operator) is a separate control-plane-side workload. It connects to the guest cluster through kube-apiserver and reconciles guest-visible configuration/resources, including `config.openshift.io/Authentication`, `config.openshift.io/Proxy`, pull secrets, and MachineConfigs. It cannot directly configure an HCP-namespace Pod.

```text
HostedCluster spec → HostedControlPlane spec → CPO  → HCP-side Pods
                                             └→ HCCO → guest-cluster APIs
```

- There are no usual operators. Operands are managed by Control Plane Operator.
- In HyperShift, external OIDC is configured via the `HostedCluster` resource (`.spec.configuration.authentication`), unlike standalone clusters where the admin configures `authentication.config.openshift.io/cluster` directly.
- CPO needs a proxy configuration derived from the `HostedCluster`, but it should extend the existing `.spec.operatorConfiguration.openShiftOAuthAPIServer` component configuration, not `.spec.configuration.authentication`.
