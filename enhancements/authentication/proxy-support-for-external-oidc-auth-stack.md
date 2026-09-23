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

That enhancement adds `Authentication.spec.proxy` (`operator.openshift.io/v1`) for
the integrated OAuth server and CAO, deferring External OIDC and HyperShift support.
This enhancement extends that work to External OIDC on standalone and hosted
control planes.

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
The user's browser must still reach Console, its callback URL, the provider's
authorization endpoint, and the provider's logout endpoint when configured.
These requests use the browser's own network settings, not Console's proxy.
Configuring the browser's network access, CLI login helpers, or user-managed OIDC
clients is outside this enhancement's scope.

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
  same authentication proxy configuration without changing unrelated Console
  traffic.
- Reuse the existing standalone authentication proxy configuration and its
  precedence over the cluster-wide proxy.
- Support proxy CA rotation without restarting the webhook or Console solely for
  CA content changes.
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

Console Operator also consumes the authentication proxy configuration from
`authentication.operator.openshift.io/cluster` and supplies
the effective proxy settings and trust through Console's existing configuration
file. Console applies them only to its External OIDC HTTP clients, leaving its
global proxy environment and unrelated clients unchanged. Console Operator owns
reconciliation of the Console Deployment and its configuration; CAO continues to
own the webhook Deployment.

### Feature Gates

The feature applies in External OIDC mode
(`authentication.config.openshift.io/cluster.spec.type: OIDC`):

| Consumer | Required gates |
| --- | --- |
| CAO and the External OIDC webhook | `ExternalOIDC`, `ExternalOIDCExternalClaimsSourcing`, `AuthenticationComponentProxy`, `AuthenticationComponentProxyExternalOIDC` |
| Console Operator | `ExternalOIDC`, `AuthenticationComponentProxy`, `AuthenticationComponentProxyExternalOIDC` |

The new `AuthenticationComponentProxyExternalOIDC` gate extends component proxy
support to both paths; no separate Console gate is introduced. Console Operator
checks the gates and configures the operand, which does not evaluate them itself.

`ExternalOIDCExternalClaimsSourcing` currently selects the webhook architecture
even without configured external claim sources. Console's outbound clients do
not depend on that gate, but the complete flow in this proposal requires the
webhook. If its prerequisite enhancement separates webhook selection from claim
sourcing, only the webhook-selection gate will be required for that path; the
claim-sourcing gate will apply when external sources are used.

### Workflow Description

#### Standalone OpenShift

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
   `auth.proxy` configuration and CA mount for Console's OIDC clients.
5. For Console login, the browser follows the authorization redirect to the
   provider and returns to Console's callback. Console uses the effective proxy
   for its server-side OIDC requests, including subsequent token refresh.
6. When a user presents an external OIDC token to kube-apiserver, kube-apiserver
   calls the TokenReview webhook, which validates the token and retrieves any
   required claims using the effective proxy.

For subsequent changes, edit the operator resource or source CA ConfigMap. See
[Configuration Updates and CA Reload](#configuration-updates-and-ca-reload) for
update and removal behavior.

#### HyperShift

The starting point is a hosted cluster using the External OIDC webhook
architecture, with Console deployed on guest worker nodes. The management-side
API/operator and both selected hosted control-plane and guest payloads must
support this feature, as described in
[Version Skew Strategy](#version-skew-strategy). The required management and
hosted feature gates must be enabled, and the shared proxy must be reachable
from both clusters as described under
[hosted networking](#hypershift--hosted-control-planes). The administrator must be
authorized to update the HostedCluster and create ConfigMaps in its namespace
on the management cluster.

1. The administrator configures External OIDC through
   `HostedCluster.spec.configuration.authentication`, including provider trust,
   external claim sources as needed, and the Console OIDC client.
2. If the proxy requires a custom CA, the administrator creates a ConfigMap in the
   **HostedCluster namespace on the management cluster**, containing the PEM
   bundle under `ca-bundle.crt`. It is not initially created in the guest's
   `openshift-config` namespace.
3. The administrator sets the proposed
   `HostedCluster.spec.operatorConfiguration.authentication.proxy` field. For
   example, the following fragment is added to the existing HostedCluster:

   ```yaml
   spec:
     operatorConfiguration:
       authentication:
         proxy:
           httpProxy: http://proxy.example.com:3128
           httpsProxy: http://proxy.example.com:3128
           noProxy:
             - idp.internal.example.com
           trustedCA:
             name: auth-proxy-ca
   ```

   As on standalone clusters, `trustedCA` is optional and at least one proxy URL
   must be set. The new hosted API is subject to the review described under
   [API Extensions](#api-extensions).
4. The HyperShift operator copies the component configuration into the
   HostedControlPlane and, when configured, synchronizes the referenced CA into
   the HCP namespace.
5. The Control Plane Operator (CPO) validates issuer discovery using the effective proxy and applicable trust,
   and configures the HCP-side OAuth API server Deployment with the proxy
   environment and optional proxy CA mount.
6. The Hosted Cluster Config Operator (HCCO) publishes the component proxy into the guest
   `authentication.operator.openshift.io/cluster` and, when configured, copies the
   proxy CA into guest `openshift-config`, adjusting the `trustedCA` reference to
   the managed copy. Console Operator consumes these inputs as on standalone
   clusters.

For updates, the administrator edits the HostedCluster proxy configuration or its
source CA ConfigMap, not the HostedControlPlane or generated guest copies. The
same [update and removal behavior](#configuration-updates-and-ca-reload) applies.

### API Extensions

The standalone design reuses the `spec.proxy` API defined in
[the integrated authentication proxy enhancement](proxy-support-for-integrated-auth-stack.md#api-extensions):
`httpProxy`, `httpsProxy`, `noProxy`, and `trustedCA`.
It extends consumption of that API to the External OIDC path without introducing
another user-facing proxy field.

Generated operand configuration gains a webhook `proxyTrustedCA` file path and
a Console `auth.proxy` block, described in the implementation sections below.
These are operator-managed fields, not additional administrator-facing APIs.

For HyperShift, this proposal adds an authentication-wide configuration block at
`HostedCluster.spec.operatorConfiguration.authentication`, with an optional
`proxy` field copied to the corresponding `HostedControlPlane` field. This is
shared authentication-stack configuration, rather than configuration under
`openShiftOAuthAPIServer` that would unexpectedly also affect Console. See the
[HyperShift workflow](#hypershift) for an example.

The field mirrors the values, validation, and replacement semantics of
`operatorv1.AuthenticationProxyConfig`. Its `trustedCA` reference resolves in the
HostedCluster namespace, rather than `openshift-config`; the ConfigMap must
contain `ca-bundle.crt`. This is one shared setting for CPO, the webhook, and
Console, not a separate proxy configuration for each consumer.

`spec.configuration.authentication` embeds `configv1.AuthenticationSpec`, not the
operator API that owns `spec.proxy`. It therefore does not inherit this field.
The existing `spec.configuration.proxy` remains the cluster-wide proxy and is not
repurposed as an authentication-specific setting.

The new HostedCluster and HostedControlPlane fields require HyperShift API review,
including the proposed authentication-wide configuration block,
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

The components serve the same hosted cluster but run in different Kubernetes
clusters: kube-apiserver, the OAuth API server, and CPO run in the HCP namespace
on the management cluster, while the hosted cluster's Console runs in
`openshift-console` on guest worker nodes. Their DNS and network reachability can
therefore differ.

The [hosted workflow](#hypershift) defines the configuration handoff between
operators. HyperShift's configuration-reference discovery must also include the
new CA reference under `operatorConfiguration`.

Console Operator requires no additional HyperShift-specific changes for this
feature. The common implementation reads the same cluster-local authentication
operator resource and proxy CA ConfigMap on both topologies. HCCO supplies these
inputs in the guest cluster; Console Operator does not access HostedCluster or
HostedControlPlane resources or the management cluster.

CPO resolves the component proxy from the HostedControlPlane, falling back to its
cluster-wide proxy configuration when the component field is absent. It must not
use the management cluster's proxy as a tenant's component configuration. Issuer
validation uses an explicitly configured HTTP client, not CPO-wide environment
variables. The standalone CAO reconciliation controller does not run in the hosted
control plane; its bootstrap render invocation does not provide this validation.

The proxy CA must reach both clusters through the following distribution path:

```text
Management cluster: HostedCluster namespace
  User-provided proxy CA ConfigMap
    |
    | HyperShift operator copies ca-bundle.crt
    v
Management cluster: HCP namespace
  Proxy CA ConfigMap
    |-- Read by CPO for issuer validation
    |-- Mounted by the OAuth API server
    |
    | HCCO copies ca-bundle.crt across clusters
    v
Guest cluster: openshift-config
  Managed proxy CA ConfigMap
    |
    | Console Operator copies ca-bundle.crt
    v
Guest cluster: openshift-console
  Proxy CA ConfigMap mounted by Console
```

ConfigMap references must resolve in each consumer's cluster and namespace.
The HCP references its local copy; HCCO rewrites the guest `trustedCA` reference
to the managed copy in `openshift-config`. No copy is needed in the guest's
`openshift-oauth-apiserver` namespace because the webhook runs in the HCP.

HCCO owns only the mirrored proxy field and the managed CA copy, preserving other
fields in the guest operator resource. It reconciles additions, updates, and
removal, with appropriate read/write RBAC and a distinct managed CA name to avoid
overwriting a user ConfigMap. CPO consumes HCP-local inputs independently of
guest synchronization.

Each synchronizing controller must watch CA content and reference changes so
rotation propagates through the entire chain, including both guest-side copies.
CPO refreshes issuer-validation trust. Operand updates follow
[Configuration Updates and CA Reload](#configuration-updates-and-ca-reload).
The component settings must not alter the guest cluster-wide Proxy, ignition,
or NodePool configuration or trigger a NodePool rollout.

The shared proxy URLs must resolve and be reachable from both HCP-side components
and guest Console; separate per-plane URLs are outside this design. Each caller
must reach its endpoints through the proxy or directly when matched by `NO_PROXY`.
Service DNS names and IPs refer to the caller's cluster, so a guest-local endpoint
is not automatically reachable from the HCP. Check the
[`NO_PROXY` rules](#no_proxy-and-internal-endpoint-urls) separately in each network;
successful CPO issuer validation does not prove Console connectivity.

#### Standalone Clusters

All inputs and operands reside in one cluster: CAO runs in
`openshift-authentication-operator` and manages the webhook in
`openshift-oauth-apiserver`; Console Operator manages Console in `openshift-console`.

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
| `oidc.oidc-namespace.svc.cluster.local.` (with a trailing dot) | That hostname or `.cluster.local.`; the undotted suffix does not match. |
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

Console's non-OIDC destinations do not need entries in this bypass list: their
routing is outside the authentication proxy's scope. This includes plugin-service
URLs ending in `.svc.cluster.local.`; trailing-dot OIDC destinations, however,
need matching entries as shown above.

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

#### Webhook HTTP Clients

The webhook's discovery and JWKS HTTP client uses the proxy environment and
combines issuer trust with the mounted proxy CA.

The upstream OIDC authenticator also resolves distributed claims referenced by
`_claim_names` and `_claim_sources` in the token. In the current implementation,
this applies to the configured groups claim when it is not already present as a
normal claim. The resolver fetches the referenced endpoint, using the supplied
`access_token` when present, and verifies the returned JWT, which can require
discovery and JWKS requests to that JWT's issuer. This path operates independently
of configured external claim sources.

Distributed claim retrieval and verification reuse the issuer HTTP client and
therefore inherit its proxy selection and trust. No separate proxy integration
is needed for this path.

External claim source requests, including client-credentials token acquisition
when configured, also need the effective proxy and applicable source and proxy
trust. These use separate HTTP clients from discovery and JWKS retrieval and
must each participate in the shared [CA reload behavior](#configuration-updates-and-ca-reload).

#### Console Login and Token Refresh

Console already honors `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` in its
authentication transports. Console Operator currently populates these variables
from the cluster-wide Proxy status and separately synchronizes the issuer CA.
It must additionally consume the authentication operator's `spec.proxy`.

Console Operator must watch `authentication.operator.openshift.io/cluster` and
the referenced proxy CA ConfigMap, apply the component proxy feature gates and
resolution rules, and reconcile the resulting settings and trust into Console.
This requires adding the operator Authentication informer and read permissions.
When `trustedCA` is specified, the operator must synchronize the bundle into
`openshift-console` and make it available to Console's authentication clients
alongside issuer trust. Changes and removal of the component configuration must
be reconciled.

Console already reads `/var/console-config/console-config.yaml`, mounted from the
`console-config` ConfigMap in `openshift-console`, through its `--config` argument.
The proposed implementation adds an optional `auth.proxy` block to that existing
file. For example, the generated configuration could contain this fragment:

```yaml
auth:
  authType: oidc
  oidcIssuer: https://idp.example.com
  proxy:
    httpsProxy: http://auth-proxy.example.com:3128
    noProxy:
      - ".svc"
      - ".cluster.local"
      - "localhost"
      - "127.0.0.1"
    trustedCAFile: /var/auth-proxy-ca/ca-bundle.crt
```

The `auth.proxy` fields and CA mount path are proposed additions. `httpProxy` and
`httpsProxy` contain the component proxy URLs, `noProxy` contains the resolved
bypass list including caller-local defaults, and `trustedCAFile` is omitted when
no additional proxy trust is configured. The existing top-level `proxy` section
configures plugin reverse proxies and is not reused for outbound authentication.
The duplicated configuration types in Console and Console Operator must be
updated together, along with generation, validation, and authentication-client
wiring.

Console constructs dedicated OIDC HTTP clients with explicit proxy selection,
using the same Go proxy-matching semantics as the webhook, rather than setting
process environment variables. These clients serve discovery, JWKS retrieval,
authorization-code exchange, and refresh, including when no custom issuer or
proxy CA is configured. The implementation must not mutate `http.DefaultClient`,
`http.DefaultTransport`, or clients shared with non-OIDC traffic. Console's global
`HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` values and unrelated clients' routing
and trust remain unchanged.

Console Operator emits `auth.proxy` only when the
[feature prerequisites](#feature-gates) are satisfied and the component proxy is
configured. The generated representation preserves the resolution semantics:

- When `auth.proxy` is absent, Console retains its existing environment-based
  cluster-wide proxy or direct-connect behavior and corresponding trust.
- When `auth.proxy` is present, the OIDC clients use it in full. Missing fields
  do not inherit environment values; for example, an omitted `httpsProxy` means
  direct HTTPS access even if the global `HTTPS_PROXY` variable is set.

The proxy CA is mounted separately and supplied through `auth.proxy.trustedCAFile`.
Console's authentication client construction must append it to the applicable
issuer trust, preserving the existing system-root behavior when no issuer CA is
specified, with or without a custom issuer CA.
The component proxy CA is not added to non-OIDC clients' trust stores.

#### Configuration Updates and CA Reload

The webhook and Console follow the same update contract:

| Change | Operand behavior |
| --- | --- |
| Proxy settings, including bypass rules, or CA references/mounts | Roll out the affected Deployment. |
| Contents of an already-referenced proxy CA bundle | Reload trust without a rollout. |
| Component proxy removal or gate disablement | Reconcile away the component settings and dedicated CA input, restoring cluster-wide proxy or direct-connect fallback. |

CAO and Console Operator synchronize source CA changes to the mounted copies;
the [hosted distribution path](#hypershift--hosted-control-planes) adds HyperShift
operator and HCCO synchronization. CA content hashes or resource versions must
not be included in rollout-triggering Pod annotations or generated configuration.
CAO/CPO must also refresh the trust used for issuer validation.

Both operands watch the mounted proxy CA file and rebuild affected transports
on valid content changes. Use normal ConfigMap volume mounts, not `subPath`, and
handle projected-volume file replacement. Reload the combined endpoint and proxy
trust atomically without mutating shared non-authentication clients. New TLS
connections must use the updated trust, and idle connections on superseded
transports must be retired. Invalid updates are reported while retaining the last
successfully loaded trust bundle; certificate verification must not be disabled.

Reloads must reach every authentication client: the webhook clients described
above and Console's discovery, JWKS, code-exchange, and refresh clients. In
Console, cached OIDC providers and JWKS verifiers retain HTTP clients, so replacing
only the client factory's return value is insufficient; those retained clients
must also use the updated transport.

Removing the component proxy restores connectivity only if the fallback can reach
the required endpoints. See [Support Procedures](#support-procedures) for recovery.

### Risks and Mitigations

The following risks extend those described in the
[integrated authentication proxy enhancement](proxy-support-for-integrated-auth-stack.md#risks-and-mitigations)
to External OIDC token validation and Console login.

**Cluster lockout from invalid proxy configuration.**
A wrong proxy URL, missing or invalid CA bundle, or incorrect `noProxy` entries
can break token validation, Console login, or refresh. Successful issuer discovery
does not prove that every runtime endpoint is reachable. Administrators must
retain and test independent client-certificate access with appropriate RBAC;
[Support Procedures](#support-procedures) describes recovery, following the
[External OIDC disruption guidance](direct-external-oidc-provider.md#authentication-disruptions).

**Proxy as a trusted intermediary.**
A TLS-intercepting proxy trusted by the authentication clients can observe
authorization codes, tokens, client credentials, and identity information.
Administrators must trust the proxy and any CA added through `trustedCA`, retain
HTTPS and certificate verification for external endpoints, and understand the
implications of allowing TLS interception. An ordinary HTTPS CONNECT tunnel does
not itself expose the encrypted request contents to the proxy.

**Network dependency in the authentication path.**
Proxy latency or outages can impair authentication even when the provider is
healthy. Existing caches may delay failures but are not a recovery mechanism.
Administrators own proxy availability and capacity. Failures must be observable,
and requests must not silently bypass a configured proxy on failure.

**Proxy credential leakage.**
Credentials embedded in proxy URLs are stored in the operator resource and
propagated into the webhook's Pod environment and Console's generated ConfigMap,
not protected by a Secret reference. Access to these resources must be restricted,
and diagnostics must avoid logging credentials. This limitation is inherited
from the existing proxy API; adding a separate credential Secret reference is
outside this proposal.

**Debugging complexity from dual proxy sources.**
When both component-scoped and cluster-wide proxies exist, diagnosing failures
requires checking effective settings, endpoint bypass matches, and separate
endpoint/proxy trust. Document the [resolution rules](#proxy-resolution) and
Console's OIDC-only scope so administrators do not mistake its global environment
for its authentication override.

**Authentication disruption during rollout.**
Changes requiring a rollout under the
[update contract](#configuration-updates-and-ca-reload) can briefly disrupt
single-replica deployments, including SNO. Plan these changes with recovery access
available.

### Drawbacks

This extends an authentication-specific API rather than a general per-component
proxy framework. It avoids separate administrator-facing settings for each
consumer, but requires coordinated configuration, trust, and lifecycle handling
across multiple repositories. Dedicated Console transports and cross-cluster CA
distribution add implementation and testing complexity beyond the preceding
integrated-authentication enhancement.

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

### Process-Wide Proxy Environment for Console

Console Operator could replace Console's global `HTTP_PROXY`, `HTTPS_PROXY`, and
`NO_PROXY` variables with the component settings. Its OIDC clients already honor
them, so this would require less authentication-client wiring. However, unrelated
clients, including Kubernetes, monitoring, and plugin-service reverse proxies and
external Helm repository clients, also use those variables. They could be routed
through an authentication proxy that does not permit their destinations or whose
CA they do not trust.

Additional bypass entries would protect some internal traffic, including
trailing-dot plugin-service names not covered by CAO's current defaults, but
`NO_PROXY` can only select direct access, not restore a different cluster-wide
proxy for unrelated external requests. OIDC-specific configuration therefore
provides the required isolation while preserving existing Console networking.

## Open Questions [optional]

- HyperShift API review must confirm the proposed
  `operatorConfiguration.authentication.proxy` placement for settings shared by
  CPO, the OAuth API server, and Console, and assign an API approver.
- Console maintainers must confirm the proposed `auth.proxy` operand schema,
  OIDC-specific transport wiring, and proxy CA hot reload behavior.
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
- The [feature-gate matrix](#feature-gates), including Console's independence from
  the claims-sourcing gate, fallback when either proxy gate is disabled, unchanged
  integrated OAuth behavior, and exclusion of direct kube-apiserver OIDC.
- Console config generation and parsing preserve the distinction between absent
  `auth.proxy` and a present block with omitted fields. Explicit OIDC settings
  must not inherit global proxy fields, even without custom issuer or proxy CAs.
  Applying, updating, and removing the block must leave the global proxy
  environment and non-OIDC clients' transports and trust unchanged.
- Proxy injection with and without `trustedCA`, informer-triggered reconciliation,
  CA reference changes, missing or invalid ConfigMaps, and removal of managed
  configuration.
- Issuer/source trust combined with proxy trust, with and without custom endpoint
  CAs, for every outbound client including client-credentials token acquisition.
- The [update contract](#configuration-updates-and-ca-reload), including which
  changes alter Pod templates, projected-volume file replacement, cached OIDC/JWKS
  clients, stale transport retirement, and last-valid-trust handling on errors.
- Hostname-based `NO_PROXY` matching, including Service FQDNs, short names, aliases,
  trailing-dot names, IPs, and endpoints advertised by discovery or distributed
  claims.

### End-to-End Authentication and Operator Health

Reuse the integrated-authentication test infrastructure: an OIDC provider such as
Keycloak and a forward proxy, with direct operand access to external test endpoints
blocked so successful requests prove proxy use. Run tests with component proxy
only, both proxy sources configured, cluster-wide fallback only, and no proxy.

Verify the complete Console flow from login through authenticated API requests,
token refresh, and logout. Check discovery, code exchange, JWKS retrieval, and
webhook authentication before and after refresh, with both system-trusted and
custom proxy CAs. Verify that logout clears the session and follows the configured
redirect using the browser's network, independently of the server-side proxy.

Verify Console traffic isolation using proxy request logs: Kubernetes API,
monitoring, and trailing-dot plugin-service requests retain direct access without
authentication bypass entries. With two proxies, OIDC requests use the component
proxy and unrelated external requests retain the cluster-wide proxy. Repeat
without a cluster-wide proxy and across component updates/removal; unrelated
clients must not gain component CA trust.

Include a token whose configured groups claim is supplied through OIDC distributed
claims, with no external claim sources configured. Verify proxy routing and
`NO_PROXY` bypass for the claim endpoint and for discovery and JWKS retrieval of
the returned JWT's issuer, including when these use hosts different from the
original issuer.

Exercise unreachable proxies, latency/timeouts, invalid or missing CA bundles,
and incorrect `NO_PROXY` entries. Verify diagnostic conditions/logs, no silent
proxy bypass, and retained client-certificate access. Use that access to correct
or remove `spec.proxy`, then verify recovery through a reachable fallback.

Cover webhook authentication with no external claim sources configured, as well
as configured sources using anonymous, request-token, and client-credentials
authentication. Test CAO's discovery validation independently from runtime
discovery, JWKS refresh, and claim retrieval.

Conditions must recover after correction and distinguish proxy failures from
endpoint TLS/connectivity errors. Successful authentication, not Pod readiness
alone, is the acceptance criterion. Retain regression coverage for integrated
OAuth and clusters not using the feature.

### CA Rotation

Rotate the proxy CA bundle and proxy certificate without replacing webhook or
Console Pods or changing their Pod templates. Exercise all outbound paths in
the [architecture table](#authentication-architecture), including Console providers
initialized before rotation, distributed claims, and external-source credentials.
Use fresh JWKS retrieval and new TLS connections, and test both adding and
removing CAs so caches or connections cannot mask stale trust. Confirm invalid
updates are reported, last-valid trust remains effective, and a subsequent valid
update loads without a restart.

### Hosted Control Planes and Upgrades

Run the authentication and recovery scenarios on hosted clusters. Verify the
HostedCluster-to-HCP configuration copy, CA synchronization into the HCP and guest
`openshift-config` and `openshift-console` namespaces, HCCO's guest operator
configuration, and Console reconciliation. Run the CA rotation scenarios through
the full distribution chain, checking destination-local references. Test updates,
removal, missing references, and isolation between two hosted clusters. Assert
that the guest cluster-wide Proxy and NodePool rollout hashes remain unchanged.

Verify the shared proxy URLs are usable from both the management-side control
plane and guest Console, including DNS resolution and `NO_PROXY` behavior in
each network. Include a case where the proxy is unreachable from only one side;
successful webhook authentication or CPO discovery validation must not substitute
for a successful Console login and refresh.

Exercise the [version-skew guarantees](#version-skew-strategy) in both directions
with separate `spec.controlPlaneRelease` and `spec.release` selections. Verify
unchanged authentication when the feature is unavailable and reporting
of unsupported proxy use without partial application. Test rolling updates with
proxy configuration and CA trust retained, including single-replica disruption.
Standard upgrade jobs cover the disabled feature; feature-enabled upgrade and
rollback scenarios use development jobs under `TechPreviewNoUpgrade`, not a
promise of supported Tech Preview upgrades. Add release upgrade coverage before GA.

## Graduation Criteria

The feature follows the integrated-authentication proposal's Tech Preview-to-GA
path, with graduation dependent on the External OIDC webhook architecture and
component proxy API. All in-scope authentication consumers and both standalone
and hosted topologies must be covered; webhook-only support is not sufficient.

### Dev Preview -> Tech Preview

The feature is proposed to ship directly as Tech Preview behind
`AuthenticationComponentProxyExternalOIDC`, with the prerequisite gates already enabled
in `TechPreviewNoUpgrade`.

### Tech Preview -> GA

- The [Test Plan](#test-plan) is implemented across all consumers and both
  topologies, including unit/controller, API validation, end-to-end, and recovery
  coverage.
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
- User documentation covers configuration, trust, OIDC-only Console proxy scope,
  hosted networking, troubleshooting, and client-certificate recovery.
- Tech Preview use provides sufficient feedback and soak time without unresolved
  authentication regressions.

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

Upgrades preserve existing behavior unless the feature is enabled and component
proxy settings are present. An existing `spec.proxy` starts applying to the new
consumers when the gate is enabled; administrators must check its endpoints and
bypass rules first.

Enable component-proxy use only after the participating operators and operands
meet the [version-skew requirements](#version-skew-strategy). On HyperShift,
upgrade management-side API/operator support before using the new field. Operators
reconcile compatible images and generated configuration through normal rollouts;
no authentication-data migration is introduced.

`TechPreviewNoUpgrade` retains its existing upgrade restrictions. This enhancement
does not introduce a supported downgrade path. For development rollback testing,
first establish a working cluster-wide proxy or direct path, remove the component
configuration, and let the operators reconcile compatible operand configuration
before reverting to versions that do not support it. On HyperShift, make the
change on the HostedCluster and retain the independent administrative access
described in [Support Procedures](#support-procedures).

## Version Skew Strategy

Unlike the preceding proposal, this change spans multiple operators and operands.
CAO and CPO must generate the optional proxy CA configuration only for webhook
images that understand it. Console Operator must likewise pair its generated CA
and `auth.proxy` configuration with a compatible Console image. New operands must
continue to accept configuration that omits the new inputs, retaining Console's
existing environment-based fallback when `auth.proxy` is absent. Mixed old and new
replicas during rollout must retain a working configuration until replaced.

HyperShift's management cluster OpenShift version and HyperShift Operator version
are distinct from the releases supplying the hosted cluster's components.
By default, `HostedCluster.spec.release` supplies both hosted control-plane and
guest components. When set, `spec.controlPlaneRelease` selects a separate payload
for management-side components, including CPO, HCCO, and OAuth API server; Console
Operator and Console continue to come from `spec.release` in the guest cluster.

Within supported HyperShift version combinations, newer Console components with
older authentication components, or the reverse, must preserve existing behavior.
Skew can prevent use of the shared component proxy but must not itself break
authentication or degrade a healthy cluster. Until all consumers support the
feature, retain the existing cluster-wide proxy or direct connectivity.

Acceptance of the new HostedCluster field or enablement of feature gates alone
does not establish end-to-end support. When the shared component proxy is
requested, the management operator must check both selected payloads for CPO,
HCCO, OAuth API server, Console Operator, and Console support, and report
unsupported use without partially applying the proxy configuration.
Management-side and hosted feature gates must also permit the configuration.

HCP and guest reconciliation are asynchronous; status must not imply that a
change is fully applied until both paths converge. No new kubelet or node API
dependencies are introduced, and existing platform skew limits remain unchanged.

## Operational Aspects of API Extensions

The [API extensions](#api-extensions) introduce no new CRD kind,
admission/conversion webhook, aggregated API server, or finalizer. The TokenReview
webhook belongs to the prerequisite External OIDC enhancement.

API admission validates the shape of proxy settings, not network reachability or
the contents of referenced ConfigMaps. Operators validate and reconcile these
runtime inputs and report configuration/synchronization failures through their
existing status mechanisms. Proxy outages or invalid trust can impair OIDC
authentication and Console login without making the Kubernetes API unavailable
to client-certificate or service-account authentication.

The authentication, Console, and HyperShift teams own escalation for their
respective paths. Existing conditions, events, and logs provide diagnostics as
described in [Support Procedures](#support-procedures); no dedicated service or
alerting system is added.

## Support Procedures

Use independent client-certificate administrative access when External OIDC login
is unavailable. On standalone clusters:

- Inspect `oc get clusteroperator authentication console` and the detailed
  conditions on the operator `Authentication` and `Console` resources.
- Check events and operator logs in `openshift-authentication-operator` and
  `openshift-console-operator`, plus webhook and Console logs in
  `openshift-oauth-apiserver` and `openshift-console`.
- Compare the configured component proxy, cluster-wide fallback, rendered operand
  configuration, CA references, and mounted CA copies. Inspect the webhook's proxy
  environment and Console's generated `auth.proxy` block separately; Console's
  global environment still describes its cluster-wide fallback, not an active
  authentication-specific override. Verify the required gates and that
  kube-apiserver uses the External OIDC webhook path.
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
