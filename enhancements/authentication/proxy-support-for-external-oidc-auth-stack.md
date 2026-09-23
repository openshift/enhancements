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
  - "None" # Assignment is pending; the proposed HyperShift API requires review.
creation-date: 2026-09-10
last-updated: 2026-09-23
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

The dependency is on the webhook authentication architecture, not on configuring
an external claim source. If the prerequisite enhancement separates webhook
selection from external claim sourcing, this feature will require the webhook
gate; the claim-sourcing gate will only be needed when using external sources.

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

Console similarly receives an optional proxy CA file path in its operator-managed
operand configuration. This is not an additional administrator-facing proxy API.

For HyperShift, this proposal adds an optional `proxy` field under
`HostedCluster.spec.operatorConfiguration.openShiftOAuthAPIServer`, copied to the
corresponding `HostedControlPlane` field:

```yaml
spec:
  operatorConfiguration:
    openShiftOAuthAPIServer:
      proxy:
        httpsProxy: http://proxy.example.com:3128
        noProxy:
          - idp.internal.example.com
        trustedCA:
          name: auth-proxy-ca
```

The field mirrors the values, validation, and replacement semantics of
`operatorv1.AuthenticationProxyConfig`. Its `trustedCA` reference resolves in the
HostedCluster namespace, rather than `openshift-config`; the ConfigMap must
contain `ca-bundle.crt`. The effective configuration also applies to CPO's issuer
validation and is mirrored into the guest authentication operator resource for
Console to consume. It is not a separate proxy configuration for each consumer.

`spec.configuration.authentication` embeds `configv1.AuthenticationSpec`, not the
operator API that owns `spec.proxy`. It therefore does not inherit this field.
The existing `spec.configuration.proxy` remains the cluster-wide proxy and is not
repurposed as an authentication-specific setting.

The new HostedCluster and HostedControlPlane fields require HyperShift API review,
including the proposed placement now that Console also consumes the settings,
reference namespace semantics, CEL validation, and feature-gate annotations.
The management-cluster API gate must be coordinated with the hosted cluster's
component-proxy and External OIDC gates; enabling one does not enable the other.
The existing `AuthenticationComponentProxy` API gate must also be available in
the hosted cluster profile so HCCO can publish the guest operator configuration.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hosted support builds on the External OIDC webhook topology supplied by the
External Claims Sourcing enhancement. That prerequisite owns deploying the OAuth
API server in `external-oidc` mode, generating its authentication configuration,
and configuring kube-apiserver to call its TokenReview endpoint. This enhancement
adds proxy resolution and trust distribution to that path.

The HostedCluster is the source of truth. Administrators create the proxy CA
ConfigMap in its namespace and set the component proxy field described above.
The HyperShift operator copies the configuration and referenced CA into the
HostedControlPlane and its namespace. Configuration-reference discovery must be
extended to include the new reference under `operatorConfiguration`.

| Component | Responsibility |
| --- | --- |
| HyperShift operator | Reconcile the HostedCluster input into the HostedControlPlane and synchronize the proxy CA into the HCP namespace. |
| Control Plane Operator (CPO) | Validate issuer discovery using a proxy-aware client, configure the HCP-side OAuth API server's proxy environment and CA mount, and report reconciliation failures. |
| Hosted Cluster Config Operator (HCCO) | Publish the component settings into the guest `authentication.operator.openshift.io/cluster` and copy the proxy CA into guest `openshift-config` for Console Operator. |
| Console Operator | Consume the guest configuration and reconcile Console's proxy settings and trust in `openshift-console`, as on standalone clusters. |

CPO resolves the component proxy from the HostedControlPlane, falling back to its
cluster-wide proxy configuration when the component field is absent. It must not
use the management cluster's proxy as a tenant's component configuration. Issuer
validation uses an explicitly configured HTTP client, not CPO-wide environment
variables. The standalone CAO reconciliation controller does not run in the hosted
control plane; its bootstrap render invocation does not provide this validation.

HCCO owns only the mirrored proxy field and the managed CA copy, preserving other
fields in the guest operator resource. It reconciles additions, updates, and
removal, with appropriate read/write RBAC and a distinct managed CA name to avoid
overwriting a user ConfigMap. The mirrored `trustedCA` reference uses that managed
name. Console Operator then copies that CA into
`openshift-console`. CPO continues to consume HCP-local inputs and does not depend
on reading the asynchronously reconciled guest copy. Administrators change the
HostedCluster source rather than editing these managed copies.

CA content updates propagate through each copy. The webhook reloads its mounted
bundle without a rollout; Console Operator rolls out Console for CA changes.
Proxy URL or bypass-list changes roll out the affected operands. The component
settings must not be added to the guest cluster-wide Proxy, ignition, or NodePool
configuration and must not trigger a NodePool rollout.

The proxy and endpoints must be reachable from both the management-cluster control
plane and the guest Console. Service DNS names and IPs refer to the caller's
cluster, so a guest-local endpoint is not automatically reachable from the HCP.
The internal URL and `NO_PROXY` rules below apply separately in each network.

#### Standalone Clusters

CAO runs in `openshift-authentication-operator` and manages the External OIDC
webhook Deployment in `openshift-oauth-apiserver`. It reads the operator
`Authentication` resource and user-provided ConfigMaps from the same cluster.
The standalone reconciliation and runtime behavior are described below.

Console Operator manages the Console Deployment and its configuration in
`openshift-console`, including the External OIDC client configuration and trust.

#### Single-node Deployments or MicroShift

This proposal adds no new workload to SNO deployments. Existing authentication
components perform proxy resolution and CA loading. Single-replica webhook or
Console deployments can experience a brief authentication disruption during
configuration rollouts, as described under Risks and Mitigations.

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
the implementation must add the proxy CA and its reload handling to each client,
not just to the discovery and JWKS client.

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

The proposed implementation reuses Console's existing `HTTP_PROXY`, `HTTPS_PROXY`,
and `NO_PROXY` environment variables. Console Operator replaces their cluster-wide
values with the resolved component settings only when the required gates and
External OIDC mode are active. These are process-wide settings: other Console
HTTP clients that honor them also use the authentication proxy. Administrators
must account for those destinations in proxy policy and bypass rules. Other
components' proxy environments are not changed.

The proxy CA is mounted separately and supplied through an optional file path in
Console's generated configuration. Console's authentication client construction
must append it to the applicable issuer trust, preserving the existing system-root
behavior when no issuer CA is specified. Supplying a proxy CA must work both with
and without a custom issuer CA and for discovery, JWKS, code exchange, and refresh.

Console Operator includes proxy CA content changes in its Deployment rollout
trigger, following its existing CA configuration update pattern. Proxy settings,
CA reference changes, and CA content rotation therefore roll out Console. Hot
reload without a restart is required for the webhook, not for Console. These
Console-specific choices require Console maintainer review.

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
rotation. Console rolls out for proxy CA content changes as well as proxy setting
changes and does not have the same no-restart guarantee.

### Drawbacks

As in the integrated-authentication proposal, this extends component-specific
proxy configuration rather than introducing a general per-component framework.
Authentication has a particular need for external connectivity on otherwise
disconnected clusters; extending the same model to unrelated operators would
require a separate enhancement.

The proxy introduces another availability dependency and a second configuration
source to troubleshoot. Supporting CAO, the webhook, Console, and hosted control
planes also requires consistent precedence, trust, and update behavior across
multiple controllers. Console's process-wide proxy settings affect more than its
OIDC clients, and its CA rotation requires a rollout. These trade-offs avoid a
separate proxy API for each authentication consumer but increase testing and
operational complexity.

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

- HyperShift API review must confirm the proposed
  `operatorConfiguration.openShiftOAuthAPIServer.proxy` placement for settings also
  consumed by CPO and Console, and assign an API approver.
- Console maintainers must confirm the proposed process-wide proxy scope and
  rollout-based CA updates. An OIDC-specific transport configuration would narrow
  the scope but require additional configuration and client wiring.
- Coordinate the final webhook-selection gate with the prerequisite enhancement
  if it is split from `ExternalOIDCExternalClaimsSourcing`; do not require external
  claim sources merely to enable component-proxy support.

## Test Plan

Testing follows the integrated-authentication proposal's input validation,
authentication flow, operator health, and proxy resolution coverage, extended to
the External OIDC webhook, Console, and HyperShift.

### Input Validation and Unit Tests

Reuse the authentication proxy CRD validation tests in `openshift/api` for URL
schemes, hostnames, paths, query strings, fragments, CA reference names, list
constraints, and the requirement to supply at least one proxy URL. Add equivalent
HyperShift API tests, including feature-gated admission and serialization
compatibility for the optional HostedCluster and HostedControlPlane fields.

Unit and controller tests in CAO, OAuth API server, Console, Console Operator, and
HyperShift cover:

- Full component replacement, cluster-wide fallback, and direct connectivity,
  including partially populated proxy configurations with no field inheritance.
- Feature gates disabled, integrated OAuth mode unchanged, and the unsupported
  direct-kube-apiserver OIDC path not receiving component proxy settings.
- Proxy injection with and without `trustedCA`, informer-triggered reconciliation,
  CA reference changes, missing or invalid ConfigMaps, and removal of managed
  configuration.
- Issuer/source trust combined with proxy trust, with and without custom endpoint
  CAs, for every outbound client including client-credentials token acquisition.
- Webhook CA hot reload without a Deployment change and Console CA changes
  triggering a rollout; environment changes must roll out both operands.
- Hostname-based `NO_PROXY` matching, including Service FQDNs, short names, aliases,
  IPs, and endpoints advertised by discovery or distributed claims.

### End-to-End Authentication and Operator Health

Reuse the integrated-authentication test infrastructure: an OIDC provider such as
Keycloak and a forward proxy, with direct operand access to external test endpoints
blocked so successful requests prove proxy use. Run tests with component proxy
only, both proxy sources configured, cluster-wide fallback only, and no proxy.

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

Cover webhook authentication with no external claim sources configured, as well
as configured sources using anonymous, request-token, and client-credentials
authentication. Test CAO's discovery validation independently from runtime
discovery, JWKS refresh, and claim retrieval. Test Console's other proxy-aware
clients for regressions caused by the process-wide settings.

Verify that invalid configuration and reconciliation failures surface through the
owning operators' conditions and logs, that conditions recover after correction,
and that proxy failures can be distinguished from endpoint TLS or connectivity
failures. Do not rely solely on Deployment availability to prove authentication
works. Retain regression coverage for integrated OAuth and clusters not using the
new feature.

### Hosted Control Planes and Upgrades

Run the authentication and recovery scenarios on hosted clusters. Verify the
HostedCluster-to-HCP configuration copy, CA synchronization into the HCP and guest
namespaces, HCCO's guest operator configuration, and Console reconciliation.
Test updates, removal, missing CA references, and isolation between two hosted
clusters with different proxies. Confirm that neither component proxy nor CA
changes modify the guest cluster-wide Proxy or cause a NodePool rollout.

Exercise supported operator/payload version combinations and rolling updates of
the webhook and Console with a proxy configured. Standard upgrade jobs cover
unchanged behavior when the feature is disabled. Feature-enabled upgrade and
rollback scenarios require development test jobs while the feature is in
`TechPreviewNoUpgrade`; they are not a promise of supported Tech Preview upgrades.
Before GA, add release upgrade coverage with component proxy configuration and
CA trust retained throughout the upgrade, including control-plane/guest skew on
HyperShift and the documented disruption on single-replica deployments.

## Graduation Criteria

The feature follows the integrated-authentication proposal's Tech Preview-to-GA
path, with graduation dependent on the External OIDC webhook architecture and
component proxy API. All in-scope authentication consumers and both standalone
and hosted topologies must be covered; webhook-only support is not sufficient.

### Dev Preview -> Tech Preview

The feature is proposed to ship directly as Tech Preview behind
`AuthenticationComponentProxyExternalOIDC`, with the prerequisite gates enabled
in `TechPreviewNoUpgrade`. Entry requires end-to-end webhook and Console proxy
support, the hosted API and reconciliation changes, automated tests, and initial
configuration and recovery documentation.

### Tech Preview -> GA

- End-to-end behavior, proxy precedence, `NO_PROXY`, custom trust, CA rotation,
  configuration removal, and recovery are implemented and tested for standalone
  and hosted control planes.
- Unit and controller tests cover all proxy consumers; CRD integration tests cover
  standalone and hosted API validation.
- E2E tests run in presubmit and periodic CI and meet the
  [feature promotion requirements](../../dev-guide/feature-zero-to-hero.md#promotion-requirements)
  across supported platforms, architectures, network types, and topologies,
  including the required pass rate and pre-branch observation period.
- Upgrade and version-skew coverage demonstrates retained authentication with a
  configured proxy, and documents the single-replica availability limitations.
- Prerequisite features are sufficiently mature for the supported configuration;
  this feature cannot graduate while its required authentication path remains
  unsupported for GA use.
- HyperShift API and Console integration reviews are complete, and the necessary
  gates are promoted together with the applicable APIs and consumers.
- User documentation covers configuration, trust, process-wide Console scope,
  hosted networking, troubleshooting, and client-certificate recovery.
- Tech Preview use provides sufficient feedback and soak time without unresolved
  authentication regressions.

### Removing a deprecated feature

Not applicable. This proposal does not deprecate or remove an existing feature.

## Upgrade / Downgrade Strategy

The new behavior is opt-in through feature gates and optional component proxy
configuration. With the new gate disabled, existing integrated OAuth and External
OIDC behavior is preserved. With the gate enabled but no component proxy set, the
existing cluster-wide proxy or direct-connect configuration remains effective.
An existing `spec.proxy` starts applying to the newly supported consumers when
the new gate is enabled; administrators must check its endpoints and bypass rules
before enabling it.

The operators reconcile compatible operand images, proxy settings, CA mounts,
and generated configuration as part of normal rollouts. Administrators should
enable component-proxy use only after all participating operators and operands
have updated. On HyperShift, upgrade the management-side API/operator support
before using the new field and ensure the hosted release includes CPO, HCCO,
OAuth API server, and Console support. Existing single-replica rollout disruption
still applies; the proxy does not add a separate migration of authentication data.

`TechPreviewNoUpgrade` retains its existing upgrade restrictions. This enhancement
does not introduce a supported downgrade path. For development rollback testing,
first establish a working cluster-wide proxy or direct path, remove the component
configuration, and let the operators reconcile compatible operand configuration
before reverting to versions that do not support it. On HyperShift, make the
change on the HostedCluster. Retain client-certificate administrative access
throughout; simply removing the component proxy does not make an unreachable IdP
reachable.

## Version Skew Strategy

Unlike the preceding proposal, this change spans multiple operators and operands.
CAO and CPO must generate the optional proxy CA configuration only for webhook
images that understand it. Console Operator must likewise pair its generated CA
configuration with a compatible Console image. New operands must continue to
accept configuration that omits the new CA input. Mixed old and new replicas
during rollout must retain a working configuration until replaced.

HyperShift's management operator can serve hosted clusters on different releases.
Acceptance of the new HostedCluster field alone does not imply that an older
hosted release supports it. The management operator must validate the requested
feature against the selected hosted release and report unsupported combinations
instead of silently treating the field as effective. Management-side API gates
and hosted feature gates must both permit the configuration.

CPO consumes HCP-local configuration, while HCCO and Console Operator reconcile
the guest copy asynchronously. A change is not fully applied until both paths
converge; status and tests must cover this interval. There are no new kubelet or
node API dependencies, and the component setting must not alter NodePool rollout
hashes. Existing platform version-skew limits remain unchanged.

## Operational Aspects of API Extensions

Standalone clusters reuse the existing operator `Authentication` API; hosted
clusters gain optional fields on the existing HostedCluster and HostedControlPlane
APIs. No new CRD kind, admission/conversion webhook, aggregated API server, or
finalizer is introduced. The TokenReview webhook is supplied by the prerequisite
External OIDC enhancement, not by this proxy feature.

API admission validates the shape of proxy settings, not network reachability or
the contents of referenced ConfigMaps. Operators validate and reconcile these
runtime inputs and report configuration/synchronization failures through their
existing status mechanisms. Proxy outages or invalid trust can impair OIDC
authentication and Console login without making the Kubernetes API unavailable
to client-certificate or service-account authentication.

Observe authentication and Console operator conditions, hosted control plane
conditions, operand readiness, and runtime discovery, TLS, claim-retrieval, and
token-exchange errors. Successful login and token validation are the end-to-end
health checks; healthy Pods alone are insufficient. Proxy latency can add to
authentication latency, so feature tests must exercise failures and timeouts.
The authentication, Console, and HyperShift teams own escalation for their
respective reconciliation and runtime paths. No new dedicated service or alerting
system is required by this proposal.

## Support Procedures

Use independent client-certificate administrative access when External OIDC login
is unavailable. On standalone clusters:

- Inspect `oc get clusteroperator authentication console` and the detailed
  conditions on the operator `Authentication` and `Console` resources.
- Check events and operator logs in `openshift-authentication-operator` and
  `openshift-console-operator`, plus webhook and Console logs in
  `openshift-oauth-apiserver` and `openshift-console`.
- Compare the configured component proxy, cluster-wide fallback, rendered operand
  environments, CA references, and mounted CA copies. Verify the required gates
  and that kube-apiserver uses the External OIDC webhook path.
- Check discovery, JWKS, claim-source, and token endpoints independently. Confirm
  that `NO_PROXY` matches the URL hostname, the endpoint resolves from the caller's
  network, and the correct issuer/source and proxy CAs are available. Distinguish
  proxy connectivity errors from endpoint TLS errors and invalid tokens.

For HyperShift, also inspect HostedCluster and HostedControlPlane conditions,
HyperShift operator/CPO/HCCO logs, HCP-side operand settings and CA copies, and the
guest configuration received by Console Operator. Trace the configuration from the
HostedCluster source; editing a generated guest resource or Deployment is not a
persistent fix. Check management-plane and guest connectivity separately.

Correct the proxy URL, bypass list, or referenced CA at the source. Alternatively,
remove the component proxy to restore a verified working cluster-wide proxy or
direct path. On standalone clusters the source is
`authentication.operator.openshift.io/cluster`; on hosted clusters it is the
HostedCluster component proxy field. Do not disable TLS verification or remove
the TokenReview webhook as a proxy workaround. Reconciliation resumes after the
configuration is fixed; verify both API token authentication and a fresh Console
login/refresh, not just cleared operator conditions.

Disabling component proxy use does not delete user or workload data, but affected
users cannot submit new API requests while authentication is broken. Existing
workloads and service-account authentication do not depend on this OIDC proxy.
Redact proxy credentials, client secrets, and tokens from diagnostic output and
support attachments.

## Infrastructure Needed [optional]

Reuse the integrated-authentication proposal's OIDC provider and forward-proxy
test infrastructure. Extend it with distributed/configured claim endpoints,
client-credentials token acquisition, custom proxy CA rotation, and browser-driven
Console login. CI must be able to block direct operand egress while allowing the
test browser to reach the provider's authorization endpoint and Console callback.

Add the necessary presubmit and periodic coverage to the existing authentication,
Console, and HyperShift test jobs, including hosted management/guest networking.
No new production service, repository, or externally operated identity provider
is required; test services should be deployed and cleaned up by the test harness.
