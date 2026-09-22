---
title: proxy-support-for-external-oidc-auth-stack
authors:
  - "@tchap"
  - "@wouldgo"
reviewers:
  - "@liouk" # The author of the original External OIDC EP, to review the whole EP.
approvers:
  - "@benluddy"
api-approvers:
  - "None"
creation-date: 2026-09-10
last-updated: 2026-09-22
status: provisional
tracking-link:
  - "https://redhat.atlassian.net/browse/OCPSTRAT-3721"
see-also:
  - "/enhancements/authentication/direct-external-oidc-provider.md"
  - "/enhancements/authentication/external-oidc-additional-identity-information-sources.md"
  - "/enhancements/authentication/proxy-support-for-integrated-auth-stack.md"
replaces:
superseded-by:
---

# Proxy Support for External OIDC Auth Stack

## Summary

This enhancement extends the component-scoped proxy support introduced by
[Proxy Support for Integrated Auth Stack](proxy-support-for-integrated-auth-stack.md)
to External OIDC authentication. It enables the Cluster Authentication Operator
(CAO) and the OAuth API server's External OIDC webhook authenticator to reach
external identity providers, OIDC distributed claim endpoints, and configured
external claim sources through a proxy, without requiring a cluster-wide egress
proxy. The feature targets standalone OpenShift and HyperShift.

**Note: HyperShift part is largely missing for now.**

## Motivation

Clusters in restricted networks may need to authenticate users against an external
identity provider while keeping other cluster components disconnected from external
services. Configuring the cluster-wide proxy for this purpose distributes proxy
configuration beyond the authentication components that need it. The
[integrated authentication proxy enhancement](proxy-support-for-integrated-auth-stack.md#motivation)
describes this problem and the operational cost of workarounds such as an internal
identity broker or manually patching operator-managed Deployments.

That enhancement adds `Authentication.spec.proxy` (operator.openshift.io/v1) for the integrated OAuth server and CAO,
deferring External OIDC and HyperShift support. This enhancement extends that work to External
OIDC on standalone and hosted control planes.

### Authentication Architecture

[Direct External OIDC Provider](direct-external-oidc-provider.md) allows OpenShift
to accept tokens issued by an external OIDC provider. In its original architecture,
kube-apiserver validates those tokens directly using its structured authentication
configuration.

[External OIDC Additional Identity Information Sources](external-oidc-additional-identity-information-sources.md)
introduces a different authentication path: the OAuth API server runs its
`external-oidc` subcommand as a TokenReview webhook authenticator. Kube-apiserver
delegates token validation to this webhook, which validates JWTs and can retrieve
additional identity information from configured external claim sources. The
integrated OAuth server is not used in this mode.

This webhook architecture provides the component boundary for the proposed proxy
support. Its outbound dependencies are:

| Component | Outbound communication |
| --- | --- |
| CAO | Fetches the issuer's discovery document while validating External OIDC configuration. |
| OAuth API server | Fetches OIDC discovery metadata and retrieves or refreshes the issuer's JWKS signing keys for JWT verification. |
| OAuth API server, when a token references supported [OIDC distributed claims](https://openid.net/specs/openid-connect-core-1_0.html#AggregatedDistributedClaims) | Fetches claims from endpoints referenced in the token and discovers the returned JWT's issuer and signing keys for verification. |
| OAuth API server, when external claim sources are configured | Fetches additional identity information from the configured source URLs. |
| OAuth API server, when a claim source uses client credentials | Obtains access tokens from the configured token endpoint to authenticate requests to that source. |

The webhook validates a token that the caller already obtained. It does not host
the interactive browser login flow or exchange the user's authorization code for
tokens. Client-credentials requests for external claim sourcing are a separate
outbound dependency. Proxy configuration for browsers, CLI login helpers, or
other OIDC clients is outside this component's configuration.

### User Stories

- As a cluster administrator, I want External OIDC authentication to reach my
  identity provider through an authentication-specific proxy so that I can use
  external identities without configuring a cluster-wide proxy.
- As a cluster administrator, I want configured external claim sources to use
  that proxy so that group and other identity information remains available in a
  restricted network.

### Goals

- Extend component-scoped proxy support to External OIDC issuer validation,
  discovery, JWKS retrieval, distributed claim resolution, and external claim
  sourcing.
- Reuse the existing standalone authentication proxy configuration and its
  precedence over the cluster-wide proxy.
- Support proxy CA rotation without restarting the webhook solely for CA content
  changes.
- Support standalone OpenShift and HyperShift.

### Non-Goals

- Component-scoped proxy support for the original External OIDC path in which
  kube-apiserver performs OIDC authentication directly.
- Changes to the integrated OAuth authentication flow covered by the preceding
  proxy enhancement.
- Per-provider or per-claim-source proxy settings, changes to the cluster-wide
  proxy API, or a generalized per-component proxy framework.
- Proxy configuration for end-user OIDC clients or unrelated cluster components.

## Proposal

Extend the existing authentication proxy resolution and CA distribution mechanisms
to the External OIDC webhook architecture. On standalone clusters, the
administrator continues to configure providers on
`authentication.config.openshift.io/cluster` and configures the component proxy
on the distinct `authentication.operator.openshift.io/cluster` resource.

CAO uses the effective proxy when validating issuer discovery, renders proxy
environment variables into the External OIDC OAuth API server Deployment, and
synchronizes and mounts the optional proxy CA bundle. The webhook uses this
configuration for its outbound requests. The TokenReview connection from
kube-apiserver to the webhook remains within the cluster.

The standalone implementation requires `ExternalOIDC`,
`ExternalOIDCExternalClaimsSourcing`, `AuthenticationComponentProxy`, and
`AuthenticationComponentProxyExternalOIDC`. The External Claims Sourcing gate
selects the webhook architecture even when no provider has external claim sources.
The new `AuthenticationComponentProxyExternalOIDC` gate to be added controls extending the
component proxy to this path.

TODO: Update relevant gate names once `ExternalOIDCExternalClaimsSourcing` is split.

### Workflow Description

The starting point is a standalone cluster using the External OIDC webhook
architecture, with the required feature gates enabled and an administrator-provided
proxy that can reach the required external endpoints.

1. The administrator configures OIDC providers on
   `authentication.config.openshift.io/cluster`, including issuer trust and
   external claim sources as needed.
2. If the proxy requires a custom CA, the administrator creates a ConfigMap in
   `openshift-config` containing the PEM bundle under `ca-bundle.crt`.
3. The administrator sets `spec.proxy` on the authentication operator resource:

   ```yaml
   apiVersion: operator.openshift.io/v1
   kind: Authentication
   metadata:
     name: cluster
   spec:
     managementState: Managed
     proxy:
       httpProxy: http://proxy.example.com:3128
       httpsProxy: http://proxy.example.com:3128
       noProxy:
         - idp.internal.example.com
       trustedCA:
         name: auth-proxy-ca
   ```

   The `trustedCA` reference is optional and is omitted when no additional proxy
   trust is needed. At least one of `httpProxy` or `httpsProxy` must be set.
4. CAO revalidates issuer discovery using the effective proxy and applicable CA
   certificates, and reconciles the webhook Deployment with the proxy environment
   variables and optional CA mount.
5. When a user presents an external OIDC token to kube-apiserver, kube-apiserver
   calls the TokenReview webhook. The webhook uses issuer discovery and JWKS data
   to validate the token, resolves supported distributed claims referenced by the
   token, and retrieves configured external claims. These outbound requests use
   the effective proxy before the webhook returns the authentication result.

Changing proxy environment variables rolls out the Deployment. Updating only the
contents of the referenced proxy CA bundle is picked up through the mounted file
without a Deployment rollout. Removing `spec.proxy` restores the cluster-wide
proxy configuration, if configured, or direct connectivity otherwise.

### API Extensions

The standalone design reuses the `spec.proxy` API defined in
[the integrated authentication proxy enhancement](proxy-support-for-integrated-auth-stack.md#api-extensions):
`httpProxy`, `httpsProxy`, `noProxy`, and `trustedCA`.
It extends consumption of that API to the External OIDC path without introducing
another user-facing proxy field.

The generated OAuth API server authentication configuration gains a
`proxyTrustedCA` file path for the mounted proxy bundle. This is operand
configuration managed by CAO, not a field that administrators set on
`authentication.config.openshift.io/cluster`.

TODO: Describe the HyperShift API and determine its API review requirements.

### Topology Considerations

#### Hypershift / Hosted Control Planes

TODO: Describe the hosted control plane configuration, reconciliation, and CA
distribution. HyperShift is in scope for the feature; the standalone CAO design
below does not by itself implement hosted support.

#### Standalone Clusters

CAO runs in `openshift-authentication-operator` and manages the External OIDC
webhook Deployment in `openshift-oauth-apiserver`. It reads the operator
`Authentication` resource and user-provided ConfigMaps from the same cluster.
The standalone reconciliation and runtime behavior are described below.

#### Single-node Deployments or MicroShift

This proposal does not add any additional CPU/memory overhead to SNO deployments.

The auth stack is not present on MicroShift.

#### OpenShift Kubernetes Engine

Not affected.

### Implementation Details/Notes/Constraints

#### Proxy Resolution

Reuse the resolution rules from
[the integrated authentication proxy enhancement](proxy-support-for-integrated-auth-stack.md#proxy-resolution):

- When the component proxy gates are enabled and `spec.proxy` is configured,
  use that configuration in full. Do not inherit individual fields from the
  cluster-wide proxy.
- Otherwise, use the cluster-wide proxy configuration provided through CAO's
  process environment. If no proxy is configured there, use direct connectivity.

The shared resolver adds cluster-internal bypass entries to the component
`noProxy` list, including `.cluster.local`, `.svc`, `localhost`,
`127.0.0.1`, and the Kubernetes service IP when available through
`KUBERNETES_SERVICE_HOST`. The cluster-wide fallback preserves the `NO_PROXY`
value supplied through CAO's environment.

#### `NO_PROXY` and Internal Endpoint URLs

Go matches `NO_PROXY` against the host in each request URL before DNS resolution;
it does not expand DNS search suffixes or match a hostname against its resolved
IP address. The default entries therefore cover common Service DNS names, but
administrators must add other forms used by endpoints that require direct access:

| Host in the request URL | Additional `spec.proxy.noProxy` entry |
| --- | --- |
| `oidc.oidc-namespace.svc` or `oidc.oidc-namespace.svc.cluster.local` | None; covered by the default suffixes. |
| `oidc` or `oidc.oidc-namespace` | The short hostname used in the URL. |
| A Route hostname or custom DNS alias | That hostname, or an appropriate domain suffix. |
| A Service or Pod IP other than the automatically included Kubernetes service IP | That IP, or a CIDR containing it. |

For example, `discoveryURL: https://oidc.oidc-namespace/.well-known/openid-configuration`
requires `oidc.oidc-namespace` in `spec.proxy.noProxy` to bypass the proxy.
Entries contain hosts or IPs, optionally with ports, or CIDRs, not complete URLs.
The hostname must still resolve from the caller's namespace and match the
endpoint's TLS certificate.

Apply this rule to discovery, the `jwks_uri` returned by discovery, distributed
claim endpoints and their JWT issuers' discovery and JWKS URLs, configured external
claim source URLs, and client-credentials token endpoints independently. Bypassing
an internal discovery endpoint does not automatically bypass a different JWKS host.
The incoming TokenReview connection from kube-apiserver requires no additional
bypass entry in the webhook's environment; these settings affect outbound requests.

#### Operator Reconciliation and Issuer Validation

The External OIDC controller watches the configuration `Authentication` resource,
the operator `Authentication` resource containing `spec.proxy`, and relevant
ConfigMaps. Proxy and CA changes therefore trigger configuration reconciliation
and issuer validation.

For the webhook path, the OAuth API server authentication-config generator uses
the shared proxy resolver when fetching
`<issuer URL>/.well-known/openid-configuration`, or the configured
`discoveryURL`. Its HTTP transport combines the applicable issuer trust with the
component proxy CA. CAO reads the proxy configuration for these requests rather
than changing its own process environment.

The kube-apiserver authentication-config generator does not receive the component
proxy resolver. Applying a proxy only to CAO's validation would not configure
kube-apiserver's runtime discovery and JWKS requests, so this proposal does not
claim support for that direct authentication path.

#### OAuth API Server Deployment and Trust

The workload controller watches the component proxy input and renders
`HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` from the effective configuration
onto the External OIDC container. This must also work when `trustedCA` is
omitted, including when using the cluster-wide fallback.

When `trustedCA` is set, CAO synchronizes the referenced ConfigMap from
`openshift-config` into `openshift-oauth-apiserver` as
`v4-0-config-system-auth-proxy-ca` and mounts it read-only. The generated
`auth-config.json` includes the mounted `ca-bundle.crt` path in
`proxyTrustedCA`.

Issuer and external-source CA configuration continue to describe trust for those
endpoints. The component proxy CA supplies additional trust for an HTTPS proxy or
a TLS-intercepting proxy; it does not replace endpoint configuration.

#### Webhook Runtime and CA Rotation

The webhook's discovery and JWKS HTTP client uses the proxy environment and
combines issuer trust with the mounted proxy CA. The operand watches that CA
file and rebuilds the affected HTTP transports when its contents change.
CAO propagates source ConfigMap updates to the mounted copy without including
those contents in a Deployment rollout trigger.

The upstream OIDC authenticator also resolves distributed claims referenced by
`_claim_names` and `_claim_sources` in the token. In the current implementation,
this applies to the configured groups claim when it is not already present as a
normal claim. The resolver fetches the referenced endpoint, using the supplied
`access_token` when present, and verifies the returned JWT, which can require
discovery and JWKS requests to that JWT's issuer. This path operates independently
of configured external claim sources.

Distributed claim retrieval and verification reuse the issuer HTTP client, so
they inherit its proxy environment, `NO_PROXY` behavior, and issuer and proxy CA
trust, including proxy CA reloads. No separate proxy integration is needed for
this path. The claim endpoint and its JWT issuer can use hosts different from the
original issuer; all must be reachable through the proxy or directly when matched
by `NO_PROXY`.

External claim source requests, including client-credentials token acquisition
when configured, also need the effective proxy and applicable source and proxy
trust. These use separate HTTP clients from discovery and JWKS retrieval;
consistent proxy CA handling across those clients is an implementation follow-up
noted below.

Changes to proxy environment variables or adding or removing the CA mount change
the PodSpec and trigger a rollout. CA bundle content rotation alone does not.

### Risks and Mitigations

TODO.

### Drawbacks

TODO.

## Alternatives (Not Implemented)

### Proxy Settings in the OAuth API Server Configuration File

CAO could pass proxy URLs and bypass settings through the OAuth API server's
generated configuration file, with the server applying them to each outbound HTTP
transport. Environment variables are the simplest solution that meets the
requirements: the existing transports already honor `HTTP_PROXY`, `HTTPS_PROXY`,
and `NO_PROXY` through Go's
[`http.ProxyFromEnvironment`](https://pkg.go.dev/net/http#ProxyFromEnvironment).
This covers discovery, JWKS retrieval, distributed claim resolution, external
claim sourcing, and client-credentials token requests without additional
configuration fields or proxy wiring in the server. Proxy CA trust remains
separately configured through `proxyTrustedCA`.

## Open Questions [optional]

- Complete the HyperShift design, including the configuration API, ownership of
  issuer validation, and synchronization of proxy trust.
- Close two gaps in the current standalone implementation: render proxy
  environment variables even when `trustedCA` is absent, and propagate the
  component proxy CA and its updates to external claim source and
  client-credentials HTTP clients.

## Test Plan

Include a token whose configured groups claim is supplied through OIDC distributed
claims, with no external claim sources configured. Verify proxy routing and
`NO_PROXY` bypass for the claim endpoint and for discovery and JWKS retrieval of
the returned JWT's issuer, including when these use hosts different from the
original issuer. Cover custom proxy CA trust and CA rotation without a webhook
restart for these requests.

TODO: Complete the remaining test plan.

## Graduation Criteria

TODO.

### Dev Preview -> Tech Preview

TODO.

### Tech Preview -> GA

TODO.

### Removing a deprecated feature

TODO.

## Upgrade / Downgrade Strategy

TODO.

## Version Skew Strategy

TODO.

## Operational Aspects of API Extensions

TODO.

## Support Procedures

TODO.

## Infrastructure Needed [optional]

TODO.
