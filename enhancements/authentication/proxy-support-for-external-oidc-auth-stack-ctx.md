## Information Sources

- ./proxy-support-for-external-oidc-auth-stack.md, see-also section.
- https://redhat.atlassian.net/browse/OCPSTRAT-3721

## Implementation Notes

- `validateCACert` in `cluster-authentication-operator/pkg/controllers/externaloidc/generation/oauthapiserver/generate.go` is also affected.
- The generator validation tests were updated for the `ProxyResolver` argument and pass with:
  `go test -mod=vendor ./pkg/controllers/externaloidc/generation/oauthapiserver`.

## Open Questions

- Currently, we also support `kube-apiserver` direct auth. There is no simple way to set `kube-apiserver` to proxy on the relevant requests.
  Component-proxy support is deliberately limited to the OAuth API server External Claims Sourcing path; do not add it to the kube-apiserver path.
  `AuthenticationComponentProxyExternalOIDC` therefore requires `AuthenticationComponentProxy`, `ExternalOIDC`, and `ExternalOIDCExternalClaimsSourcing`.

## Implementation Status and TODOs

1. (DONE) The External OIDC controller must watch the component-proxy source.

   It uses the resolver during reconciliation, but only watches `config.openshift.io/Authentication` and ConfigMaps—not `operator.openshift.io/Authentication`, where the component proxy lives. A change to
   `spec.proxy` would therefore not revalidate the issuer or trigger reconciliation. Add `proxyResolver.Informer()` to the controller’s informer set at `pkg/controllers/externaloidc/externaloidc_controller.go:75`.

2. (DONE) The MOM/offline apply path needs the External OIDC proxy gate enabled.

   Its resolver checks `AuthenticationComponentProxyExternalOIDC` (`pkg/operator/replacement_starter.go:342`), but the static gate configuration marks only the older `AuthenticationComponentProxy` gate as disabled
   (`pkg/operator/replacement_starter.go:144`). Thus apply-configuration will not exercise the new External-OIDC-specific proxy behavior.

3. (DONE) The External OIDC OAuth API server deployment receives the effective proxy configuration.

   `OAuthAPIServerWorkload` resolves proxy settings and renders `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` on the External OIDC OAuth API server
   container. This applies the effective configuration rather than relying on the API server to discover the cluster-wide proxy itself: when the
   component-proxy gates are not enabled or no component proxy is configured, resolution falls back to the authentication operator's cluster-wide proxy
   environment.

   For the External OIDC deployment, which retrieves issuer discovery and JWKS data, the workload also synchronizes the configured `TrustedCA` ConfigMap
   from `openshift-config` to `openshift-oauth-apiserver` and mounts the synchronized copy. The API server watches the mounted CA file, so CA content
   updates are hot-reloaded and do not require a deployment rollout. Proxy environment changes alter the PodSpec and therefore roll out the deployment.

   The component-proxy informer is included in the workload controller inputs so changes to `Authentication.spec.proxy` enqueue reconciliation. Shared
   environment-variable rendering lives in `pkg/controllers/common/deploymentutil`, matching the regular OAuth server's ordering and behavior.

   Unit coverage includes proxy injection and fallback plus trusted-CA synchronization, mounting, and its initial ConfigMap-not-found retry behavior.

4. (DONE, intentionally unsupported) Kube-apiserver direct auth is not part of this feature.

   Without External Claims Sourcing, CAO continues to generate `auth-config.json` for kube-apiserver, but does not pass a component-proxy resolver into
   that generator or validate issuer discovery through that proxy. Kube-apiserver—not this operator—does the runtime discovery/JWKS retrieval, and its
   traffic cannot be configured from this component's `Authentication.spec.proxy`. Do not claim component-proxy support for that mode.
