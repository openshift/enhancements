## Information Sources

- ./proxy-support-for-external-oidc-auth-stack.md, see-also section.
- https://redhat.atlassian.net/browse/OCPSTRAT-3721

## Implementation Notes

- `validateCACert` in `cluster-authentication-operator/pkg/controllers/externaloidc/generation/oauthapiserver/generate.go` is also affected.

## Open Questions

- Currently, we also support `kube-apiserver` direct auth. There is no simple way to set `kube-apiserver` to proxy on the relevant requests.
  This basically means `AuthenticationComponentProxyExternalOIDC` can only be enabled when `AuthenticationComponentProxy`, `ExternalOIDC` and `ExternalOIDCExternalClaimsSourcing` are enabled.

```
  1. (DONE) The External OIDC controller must watch the component-proxy source.

     It uses the resolver during reconciliation, but only watches config.openshift.io/Authentication and ConfigMaps—not operator.openshift.io/Authentication, where the component proxy lives. A change to
     spec.proxy would therefore not revalidate the issuer or trigger reconciliation. Add proxyResolver.Informer() to the controller’s informer set at pkg/controllers/externaloidc/externaloidc_controller.go:75.

  2. (DONE) The MOM/offline apply path needs the External OIDC proxy gate enabled.

     Its resolver checks AuthenticationComponentProxyExternalOIDC (pkg/operator/replacement_starter.go:342), but the static gate configuration marks only the older AuthenticationComponentProxy gate as disabled
     (pkg/operator/replacement_starter.go:144). Thus apply-configuration will not exercise the new External-OIDC-specific proxy behavior.

  3. In ExternalOIDCExternalClaimsSourcing mode, the OAuth API server deployment itself needs proxy configuration.

     It is an active External OIDC consumer: it receives the generated auth configuration and will need to reach the issuer/JWKS endpoint. The generated deployment has neither HTTP_PROXY / HTTPS_PROXY / NO_PROXY
     nor the component proxy CA (bindata/oauth-apiserver/externaloidc-deploy.yaml:38). The normal OAuth server already handles both.

     That means wiring a resolver into OAuthAPIServerWorkload, injecting the environment variables, syncing/mounting the configured TrustedCA, and triggering a rollout when either changes. The deployment sync
     today only hashes the ordinary trusted-ca-bundle (pkg/operator/workload/sync_openshift_oauth_apiserver.go:391).

  4. Kube-apiserver mode needs coordination outside this operator.

     Without External Claims Sourcing, this operator writes auth-config.json for kube-apiserver. Kube-apiserver—not this operator—does discovery/JWKS retrieval. The component proxy is stored on
     operator.openshift.io/Authentication, so writing a proxy-aware validation request here does not configure kube-apiserver’s runtime traffic. Supporting a component-specific proxy end-to-end in that mode needs
     kube-apiserver-operator/API support; it cannot be completed solely in this repository.

```