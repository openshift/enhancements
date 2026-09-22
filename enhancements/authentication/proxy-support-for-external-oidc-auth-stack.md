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
(CAO), the OAuth API server's External OIDC webhook authenticator, and OpenShift
Console to reach external identity providers, OIDC distributed claim endpoints,
and configured external claim sources through a proxy, without requiring a
cluster-wide egress proxy. The feature targets standalone OpenShift and HyperShift.

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

The proposed proxy support covers both Console login and webhook token
validation. Their outbound dependencies are:

| Component | Outbound communication |
| --- | --- |
| CAO | Fetches the issuer's discovery document while validating External OIDC configuration. |
| Console | Fetches OIDC discovery metadata and JWKS, exchanges authorization codes for tokens, and refreshes tokens to maintain login sessions. |
| OAuth API server | Fetches OIDC discovery metadata and retrieves or refreshes the issuer's JWKS signing keys for JWT verification. |
| OAuth API server, when a token references supported [OIDC distributed claims](https://openid.net/specs/openid-connect-core-1_0.html#AggregatedDistributedClaims) | Fetches claims from endpoints referenced in the token and discovers the returned JWT's issuer and signing keys for verification. |
| OAuth API server, when external claim sources are configured | Fetches additional identity information from the configured source URLs. |
| OAuth API server, when a claim source uses client credentials | Obtains access tokens from the configured token endpoint to authenticate requests to that source. |

The webhook validates a token that the caller already obtained. Console performs
the server-side OAuth2/OIDC login flow and needs proxy access independently of the
webhook; otherwise, API token validation can succeed while Console login fails.
The user's browser must still reach the provider's authorization endpoint and the
Console callback URL. Configuring the browser's network access, CLI login helpers,
or user-managed OIDC clients is outside this enhancement's scope.

### User Stories

- As a cluster administrator, I want External OIDC authentication to reach my
  identity provider through an authentication-specific proxy so that I can use
  external identities without configuring a cluster-wide proxy.
- As a cluster administrator, I want configured external claim sources to use
  that proxy so that group and other identity information remains available in a
  restricted network.
- As a cluster administrator, I want Console login and token refresh to use the
  authentication proxy so that users can access Console without a cluster-wide
  proxy.

### Goals

- Extend component-scoped proxy support to External OIDC issuer validation,
  discovery, JWKS retrieval, distributed claim resolution, and external claim
  sourcing.
- Support Console's server-side External OIDC login and token refresh using the
  same authentication proxy configuration.
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
- Proxy configuration for end-user browsers, CLI login helpers, user-managed OIDC
  clients, or unrelated cluster components. Console's server-side OIDC client is
  in scope.

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

Console Operator also consumes the authentication proxy configuration and supplies
the effective proxy settings and trust to Console's External OIDC login clients.
It owns reconciliation of the Console Deployment and its configuration; CAO
continues to own the webhook Deployment.

The standalone webhook implementation requires `ExternalOIDC`,
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
   external claim sources as needed, and the Console OIDC client for Console login.
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
   variables and optional CA mount. Console Operator reconciles the corresponding
   proxy settings and trust for Console's OIDC clients.
5. For Console login, the browser follows the authorization redirect to the
   provider and returns to Console's callback. Console uses the effective proxy
   for discovery, authorization-code exchange, JWKS retrieval, and subsequent
   token refresh. The browser's requests use its own network configuration.
6. When a user presents an external OIDC token to kube-apiserver, kube-apiserver
   calls the TokenReview webhook. The webhook uses issuer discovery and JWKS data
   to validate the token, resolves supported distributed claims referenced by the
   token, and retrieves configured external claims. These outbound requests use
   the effective proxy before the webhook returns the authentication result.

Changing the webhook's proxy environment variables rolls out its Deployment.
Updating only the contents of its referenced proxy CA bundle is picked up through
the mounted file without a webhook Deployment rollout. Console's update behavior
is discussed below. Removing `spec.proxy` restores the cluster-wide proxy
configuration, if configured, or direct connectivity for both components.

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
distribution, including how Console receives the authentication proxy settings and
trust. HyperShift is in scope for the feature; the standalone design below does
not by itself implement hosted support.

#### Standalone Clusters

CAO runs in `openshift-authentication-operator` and manages the External OIDC
webhook Deployment in `openshift-oauth-apiserver`. It reads the operator
`Authentication` resource and user-provided ConfigMaps from the same cluster.
The standalone reconciliation and runtime behavior are described below.

Console Operator manages the Console Deployment and its configuration in
`openshift-console`, including the External OIDC client configuration and trust.

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
- Otherwise, use the cluster-wide proxy configuration. CAO obtains this from its
  process environment; Console Operator reads the status of
  `proxy.config.openshift.io/cluster`. If no proxy is configured, use direct
  connectivity.

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
claim source URLs, client-credentials token endpoints, and Console's discovery,
JWKS, and token endpoints independently. Bypassing an internal discovery endpoint
does not automatically bypass a different JWKS or token endpoint host.
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

#### Console Login and Token Refresh

Console is an External OIDC client. Its authentication HTTP clients perform
discovery, authorization-code exchange, ID token verification using JWKS, and
refresh-token requests. These requests must use the same effective authentication
proxy and bypass rules as the webhook. The browser's authorization and logout
redirects remain browser-side requests.

Console already honors `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` in its
authentication transports. Console Operator currently populates these variables
from the cluster-wide Proxy status and separately synchronizes the issuer CA.
It does not consume the authentication operator's `spec.proxy`, so configuring
only CAO and the webhook is insufficient for Console login.

Console Operator must watch `authentication.operator.openshift.io/cluster` and
the referenced proxy CA ConfigMap, apply the component proxy feature gates and
resolution rules, and reconcile the resulting settings and trust into Console.
This requires adding the operator Authentication informer and read permissions.
When `trustedCA` is specified, the operator must synchronize the bundle into
`openshift-console` and make it available to Console's authentication clients
alongside issuer trust. Changes and removal of the component configuration must
be reconciled, including fallback to the cluster-wide proxy.

TODO: Finalize the Console configuration mechanism and CA update behavior with
Console reviewers. Process-wide proxy variables also affect Console's other HTTP
clients; decide whether to use them or pass settings specifically to its OIDC
transports. Define whether proxy CA changes reload those transports or roll out
Console. The webhook's guarantee of CA rotation without restart does not by itself
provide that behavior for Console.

### Risks and Mitigations

The following risks extend those described in the
[integrated authentication proxy enhancement](proxy-support-for-integrated-auth-stack.md#risks-and-mitigations)
to External OIDC token validation and Console login.

**Cluster lockout from invalid proxy configuration.**
A wrong proxy URL, missing or invalid CA bundle, or incorrect `noProxy` entries
can prevent the webhook and Console from reaching required endpoints. This can
break token validation, Console login, or token refresh and lock out users who
rely on External OIDC. CAO's issuer discovery validation provides early feedback,
but successful discovery does not prove that JWKS, claim, and token endpoints are
reachable from the operands. Validation and diagnostic conditions are not a
guarantee against lockout.

Administrators must retain and test an independent administrative access path
using a kubeconfig with a valid client certificate and appropriate RBAC, as
described in the
[External OIDC authentication disruption guidance](direct-external-oidc-provider.md#authentication-disruptions).
This bypasses the External OIDC webhook and does not depend on Console login.
Recovery consists of correcting the proxy configuration or CA bundle, or removing
`spec.proxy` to restore the cluster-wide proxy or direct connectivity. Removal
only restores authentication if that fallback can reach the required endpoints;
it is not an automatic recovery guarantee.

**Proxy as a trusted intermediary.**
A TLS-intercepting proxy trusted by the authentication clients can observe
authorization codes, tokens, client credentials, and identity information.
Administrators must trust the proxy and any CA added through `trustedCA`, retain
HTTPS and certificate verification for external endpoints, and understand the
implications of allowing TLS interception. An ordinary HTTPS CONNECT tunnel does
not itself expose the encrypted request contents to the proxy.

**Network dependency in the authentication path.**
The proxy adds latency and an availability dependency to discovery, signing-key
refresh, claim retrieval, and Console token requests. An outage can prevent
authentication even when the identity provider is healthy. Existing caches may
delay some failures but are not a recovery mechanism. Proxy availability and
capacity are the administrator's responsibility. Connectivity failures must be
visible through operator diagnostics and operand logs; requests must not silently
bypass a configured proxy on failure.

**Proxy credential leakage.**
Credentials embedded in proxy URLs are stored in the operator resource and
propagated into the webhook's Pod environment, not protected by a Secret
reference. Access to these resources must be restricted, and diagnostics must
avoid logging credentials. This limitation is inherited from the existing proxy
API; adding a separate credential Secret reference is outside this proposal.

**Debugging complexity from dual proxy sources.**
When both component-scoped and cluster-wide proxies exist, diagnosing failures
requires checking the effective settings in CAO, the webhook, and Console.
Document the precedence rules: the component configuration replaces the
cluster-wide configuration in full; absence of both means direct connectivity.
Troubleshooting must also account for each endpoint's `NO_PROXY` match and for
the separate endpoint and proxy CA bundles.

**Authentication disruption during rollout.**
Proxy environment changes roll out the webhook Deployment and can briefly disrupt
authentication on single-replica deployments, including SNO. Administrators should
plan these changes with recovery access available. Proxy CA content updates are
hot-reloaded by the webhook without a rollout, reducing disruption during CA
rotation. Console's CA update behavior remains subject to the Console integration
design and must not be assumed to have the same no-restart guarantee.

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
  issuer validation, and synchronization of proxy settings and trust for the
  webhook and Console.
- Complete the Console integration design with Console reviewers, including
  configuration delivery, the scope of proxy settings within the Console process,
  and proxy CA update behavior.
- Close two gaps in the current standalone implementation: render proxy
  environment variables even when `trustedCA` is absent, and propagate the
  component proxy CA and its updates to external claim source and
  client-credentials HTTP clients.

## Test Plan

Verify a complete Console login when the provider is reachable from Console only
through the authentication proxy, with no cluster-wide proxy configured. Cover
discovery, authorization-code exchange, JWKS retrieval, token refresh, and the
resulting API requests authenticated by the webhook. Also cover `NO_PROXY`,
system-trusted and custom proxy CAs, proxy and CA updates, and removal of the
component proxy with cluster-wide fallback or direct connectivity. Ensure the
browser can independently reach the authorization endpoint and Console callback.

Include a token whose configured groups claim is supplied through OIDC distributed
claims, with no external claim sources configured. Verify proxy routing and
`NO_PROXY` bypass for the claim endpoint and for discovery and JWKS retrieval of
the returned JWT's issuer, including when these use hosts different from the
original issuer. Cover custom proxy CA trust and CA rotation without a webhook
restart for these requests.

Exercise unreachable proxy URLs, invalid or missing proxy CA bundles, and
incorrect `NO_PROXY` entries. Verify that failures are diagnosable, requests do
not bypass the configured proxy on failure, and administrative client-certificate
access remains available. Using that access, correct or remove `spec.proxy` and
verify recovery of webhook authentication and Console login with a reachable
cluster-wide fallback or direct path.

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
