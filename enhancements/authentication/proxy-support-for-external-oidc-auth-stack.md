---
title: proxy-support-for-external-oidc-auth-stack
authors:
  - "@tchap"
  - "@wouldgo"
reviewers:
  - "@liouk" # The author of the original External OIDC EP, to review the whole EP.
  - "@everettraven" # The reviewer of the Proxy Support for the Integrated Auth Stack EP; to review the whole EP.
approvers:
  - "@benluddy"
api-approvers:
  - "@everettraven" # For the Console Operator changes.
creation-date: 2026-09-10
last-updated: 2026-10-08
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
introduces a different authentication path: the OAuth API server runs as a TokenReview webhook authenticator.
Kube-apiserver delegates token validation to this webhook, which validates JWTs and can retrieve
additional identity information from configured external claim sources. The
integrated OAuth server is not used in this mode.

Console acts as a separate OIDC client and obtains the tokens used for the user's
Console session through the authorization-code flow:

1. Console fetches the provider's discovery metadata to locate its authorization,
   token, and JWKS endpoints.
2. Console redirects the user's browser to the provider's authorization endpoint.
   After the user authenticates, the provider redirects the browser back to
   Console's callback with an authorization code.
3. Console's backend exchanges the code at the provider's token endpoint using
   its configured client ID and secret. It validates the returned ID token using
   the provider's JWKS and establishes the user's Console session.
4. For Kubernetes API requests made through Console, the backend forwards the
   user's ID token as a bearer token. Kube-apiserver authenticates it through the
   TokenReview webhook described above; Console's own token validation does not
   replace API authentication or authorization.
5. When the session needs fresh tokens and a refresh token is available, Console's
   backend calls the provider's token endpoint and validates the new ID token to
   continue the session without another browser login.

The proposed proxy support covers both Console login and webhook token
validation. Their outbound dependencies are:

| Component | Outbound communication |
| --- | --- |
| CAO | Fetches the issuer's discovery document while validating External OIDC configuration. |
| Console | Fetches OIDC discovery metadata and JWKS, exchanges authorization codes for tokens, and refreshes tokens to maintain login sessions. |
| OAuth API server | Fetches OIDC discovery metadata and retrieves or refreshes the issuer's JWKS signing keys for JWT verification. |
| OAuth API server, when a token references supported [OIDC distributed claims](https://openid.net/specs/openid-connect-core-1_0.html#AggregatedDistributedClaims) | Resolves distributed claims from token-referenced endpoints, including discovery and JWKS retrieval to verify returned JWTs. This does not require configured external claim sources. |
| OAuth API server, when external claim sources are configured | Fetches additional identity information from the configured source URLs. |
| OAuth API server, when a claim source uses client credentials | Obtains access tokens from the configured token endpoint to authenticate requests to that source. |

Console and the webhook need independent proxy access to their external endpoints;
API token validation can succeed while Console login or refresh fails.
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
- Support proxy CA rotation without restarting Console or the standalone webhook;
  hosted webhook Pods roll out to refresh sidecar trust.
- Support standalone OpenShift and HyperShift.

### Non-Goals

- Component-scoped proxy support for the original External OIDC path in which
  kube-apiserver performs OIDC authentication directly.
- Changes to the integrated OAuth authentication flow covered by the preceding
  proxy enhancement, apart from the shared bypass-default expansion described in
  [Proxy Resolution](#proxy-resolution).
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

Console configures its login proxy on its own operator API, so each operator owns
its proxy configuration. Console Operator reads a new proxy field on
`console.operator.openshift.io/cluster`, and supplies the effective proxy settings
and trust through Console's existing configuration file. Console applies them only
to its External OIDC HTTP clients, leaving its global proxy environment and
unrelated clients unchanged. Console Operator owns reconciliation of the Console
Deployment and its configuration; CAO continues to own the webhook Deployment.

Because the webhook and Console read separate operator resources, a standalone
administrator sets the proxy on both. On HyperShift, a single HostedCluster block
feeds both operators, so it is configured once; see
[Topology Considerations](#hypershift--hosted-control-planes).

### Workflow Description

Apply the proxy before the OIDC provider that needs it: otherwise the first issuer
validation fails transiently until the proxy is in place. This is a sequencing
nicety, not a hard dependency — reconciliation converges to the same result in either
order.

#### Standalone OpenShift

The starting point is a standalone cluster supporting the External OIDC webhook
architecture, with the required feature gates enabled and an administrator-provided
proxy that can reach the required external endpoints.

1. If the proxy requires a custom CA, the administrator creates a ConfigMap in
   `openshift-config` containing the PEM bundle under `ca-bundle.crt`.
2. The administrator sets `spec.proxy` on the authentication operator resource:

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
3. The administrator sets the matching proxy on the Console operator resource so
   Console login uses the same proxy. The webhook and Console read separate
   operator resources, so this configuration is set in both:

   ```yaml
   apiVersion: operator.openshift.io/v1
   kind: Console
   metadata:
     name: cluster
   spec:
     authProxy:
       httpProxy: http://proxy.example.com:3128
       httpsProxy: http://proxy.example.com:3128
       noProxy:
         - idp.internal.example.com
       trustedCA:
         name: auth-proxy-ca
   ```

   The shape of `authProxy` and `trustedCA`/URL rules match the authentication operator field above.
4. The administrator configures OIDC providers on
   `authentication.config.openshift.io/cluster`, including issuer trust and
   external claim sources as needed, and the Console OIDC client for Console login.
5. CAO validates issuer discovery using the effective proxy and applicable CA
   certificates, and reconciles the webhook Deployment with the proxy environment
   variables and optional CA mount. Console Operator reconciles the corresponding
   `auth.proxy` section in the Console configuration file and CA mount for Console's OIDC
   clients from its own operator resource.
6. For Console login, the browser follows the authorization redirect to the
   provider and returns to Console's callback. Console uses the effective proxy
   for its server-side OIDC requests, including subsequent token refresh.
7. When a user presents an external OIDC token to kube-apiserver, kube-apiserver
   calls the TokenReview webhook, which validates the token and retrieves any
   required claims using the effective proxy.

Updates and removal follow
[Configuration Updates and CA Reload](#configuration-updates-and-ca-reload).

#### HyperShift

The starting point is a hosted cluster supporting the External OIDC webhook
architecture, with Console deployed on guest worker nodes. The management-side
API/operator and both selected hosted control-plane and guest payloads must
support this feature, as described in
[Version Skew Strategy](#version-skew-strategy). The required management and
hosted feature gates must be enabled, and the proxy must be reachable from the
guest/worker-node network — both the webhook (via konnectivity) and guest Console
egress there, as described under
[Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths). The
administrator must be authorized to update the HostedCluster and create ConfigMaps
in its namespace on the management cluster.

1. If the proxy requires a custom CA, the administrator creates a ConfigMap in the
   **HostedCluster namespace on the management cluster**, containing the PEM
   bundle under `ca-bundle.crt`. It is not initially created in the guest's
   `openshift-config` namespace.
2. The administrator sets the proposed
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
3. The administrator configures External OIDC through
   `HostedCluster.spec.configuration.authentication`, including provider trust,
   external claim sources as needed, and the Console OIDC client.
4. The HyperShift operator copies the component configuration into the
   HostedControlPlane and, when configured, synchronizes the referenced CA into
   the HCP namespace.
5. The Control Plane Operator (CPO) validates issuer discovery using the effective
   proxy and applicable trust, and configures the HCP-side OAuth API server
   Deployment so its egress applies the component proxy. Because this egress is
   tunneled through konnectivity into the guest network, the proxy is applied via the
   webhook pod's `konnectivity-https-proxy` sidecar, not as raw proxy environment on
   the webhook container. The optional proxy CA is mounted in both containers. See
   [Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths).
6. The Hosted Cluster Config Operator (HCCO) fans the same block out to Console:
   it writes the proxy onto the guest `console.operator.openshift.io/cluster`
   resource and, when configured, copies the proxy CA into guest `openshift-config`,
   pointing the guest Console resource's `trustedCA` at that copy. Console Operator
   then reconciles Console's configuration and CA mount from its own resource, as on
   standalone clusters.

The single `operatorConfiguration.authentication.proxy` block therefore feeds both
operators — CPO for the webhook and HCCO for Console — so the hosted administrator
configures the proxy once, unlike the two-resource standalone case.

For updates, the administrator edits the HostedCluster proxy configuration or its
source CA ConfigMap, not the HostedControlPlane or generated guest copies. The
same [update and removal behavior](#configuration-updates-and-ca-reload) applies.

### API Extensions

The YAML examples below show the relevant fields; unrelated resource fields are
omitted. They illustrate the proposed API shape and are not complete installation
manifests.

#### Existing Authentication Operator API

Reuse `Authentication.spec.proxy` in `operator.openshift.io/v1`, defined by
[the integrated authentication proxy enhancement](proxy-support-for-integrated-auth-stack.md#api-extensions).
Its fields remain `httpProxy`, `httpsProxy`, `noProxy`, and `trustedCA`; this
proposal extends their use to the External OIDC webhook without changing the API
shape.

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

Here, `trustedCA` refers to `openshift-config/auth-proxy-ca`. The reference is
optional; when present, the ConfigMap must contain `ca-bundle.crt`.

#### New Console Operator API

Add an optional proxy field to `Console.spec` in `operator.openshift.io/v1`
(`console.operator.openshift.io/cluster`), mirroring
`operatorv1.AuthenticationProxyConfig` (`httpProxy`, `httpsProxy`, `noProxy`,
`trustedCA`) with the same validation and whole-configuration replacement
semantics. It scopes proxy egress to Console's OIDC login clients and is
independent of the authentication operator field, so each operator owns its proxy
on its own resource. The proposed field name is `authProxy` since bare `proxy` risks
confusion with Console's existing plugin reverse-proxy configuration. The field is gated by
`AuthenticationComponentProxyExternalOIDC`.

```yaml
apiVersion: operator.openshift.io/v1
kind: Console
metadata:
  name: cluster
spec:
  managementState: Managed
  authProxy:
    httpProxy: http://proxy.example.com:3128
    httpsProxy: http://proxy.example.com:3128
    noProxy:
      - idp.internal.example.com
    trustedCA:
      name: auth-proxy-ca
```

This `trustedCA` also refers to a ConfigMap in `openshift-config`, so standalone
Authentication and Console resources can reference the same bundle. On hosted
clusters, HCCO writes this field with a reference to the synchronized guest copy.

#### HyperShift API Additions

The existing `spec.configuration.authentication` configures providers and their
trust, while `spec.configuration.proxy` configures the cluster-wide proxy.
An authentication-specific proxy therefore needs a separate field. It belongs
under a single authentication-wide block, not per-operator blocks such as
`openShiftOAuthAPIServer` plus a Console block, because CPO, the webhook, and
Console share one intent; a single block lets the hosted administrator configure
it once, and HyperShift fans it out to each operator (splitting it per operator
would force artificial double-entry on one HostedCluster).

Add an optional `authentication` block containing an optional `proxy` field to
`spec.operatorConfiguration` on both resources:

| New field | Set by | `trustedCA` ConfigMap namespace |
| --- | --- | --- |
| `HostedCluster.spec.operatorConfiguration.authentication.proxy` | Administrator | HostedCluster namespace |
| `HostedControlPlane.spec.operatorConfiguration.authentication.proxy` | HyperShift operator, copied from HostedCluster | HostedControlPlane namespace |

Both proxy fields mirror `operatorv1.AuthenticationProxyConfig`, including its
validation and whole-configuration replacement semantics. The HyperShift operator
synchronizes the referenced CA and ensures the HCP reference points to that local
copy. From the HCP block, CPO configures the webhook and HCCO writes the guest
`console.operator.openshift.io/cluster` proxy field (see
[Topology Considerations](#hypershift--hosted-control-planes)). CA ConfigMaps must
contain `ca-bundle.crt`.

For example, the administrator configures the HostedCluster on the management
cluster as follows:

```yaml
apiVersion: hypershift.openshift.io/v1beta1
kind: HostedCluster
metadata:
  name: example
  namespace: clusters
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

The source CA ConfigMap is `clusters/auth-proxy-ca` on the management cluster.
The HyperShift operator generates the corresponding HostedControlPlane field,
rewriting `trustedCA.name` to the synchronized ConfigMap's name in the HCP
namespace. For an HCP in `clusters-example`, this could look like:

```yaml
apiVersion: hypershift.openshift.io/v1beta1
kind: HostedControlPlane
metadata:
  name: example
  namespace: clusters-example
spec:
  operatorConfiguration:
    authentication:
      proxy:
        httpProxy: http://proxy.example.com:3128
        httpsProxy: http://proxy.example.com:3128
        noProxy:
          - idp.internal.example.com
        trustedCA:
          name: managed-auth-proxy-ca
```

Here, `managed-auth-proxy-ca` is an illustrative name for the synchronized
ConfigMap in `clusters-example`, also on the management cluster.

#### Feature Gates

The feature applies when `authentication.config.openshift.io/cluster` has
`spec.type: OIDC`:

| Consumer | Required gates |
| --- | --- |
| CAO and the External OIDC webhook | `ExternalOIDC`, `ExternalOIDCAsWebhook`, `AuthenticationComponentProxy`, `AuthenticationComponentProxyExternalOIDC` |
| Console Operator | `ExternalOIDC`, `AuthenticationComponentProxyExternalOIDC` |

The new `AuthenticationComponentProxyExternalOIDC` gate extends component proxy
support to both paths and gates the new Console operator field. Console does not
require `AuthenticationComponentProxy`: it reads its own operator resource, not the
authentication operator field that gate controls.

`ExternalOIDCAsWebhook` selects the webhook architecture and is required for this
proposal's token-validation path even without external claim sources.

##### HyperShift Feature Gating

Hosted support requires the following gating:

- The new `spec.operatorConfiguration.authentication` block on HostedCluster and
  HostedControlPlane is gated by `AuthenticationComponentProxyExternalOIDC` on the
  management side, distinct from the tenant's OpenShift gate of the same name. The
  gate name is subject to HyperShift API review.
- The guest `console.operator.openshift.io/cluster` proxy field is gated by
  `AuthenticationComponentProxyExternalOIDC`. `TechPreviewNoUpgrade` enables that
  gate, so a hosted cluster on that feature set already serves the field; no
  cluster-profile change in `openshift/api` is needed.
- The hosted preview feature set must include the gates in the
  [gate matrix](#feature-gates): `ExternalOIDC` and
  `AuthenticationComponentProxyExternalOIDC` for the guest consumers.

To use the preview feature, the administrator must enable both HyperShift's
management-side preview feature set
(`hypershift install --tech-preview-no-upgrade` upstream) and the tenant's
`HostedCluster.spec.configuration.featureGate.featureSet: TechPreviewNoUpgrade`.
These are [independent settings](https://hypershift.pages.dev/how-to/feature-gates/);
neither enables the other.

#### Generated Operand Configuration

Besides the administrator-facing fields above, the operators render the resolved
proxy into the configuration files their operands already consume. These are
internal formats, generated and versioned with the operands and not editable by
administrators; they are listed here only to distinguish them from the API
additions:

- The webhook's generated `AuthenticationConfiguration` file gains a
  `proxyTrustedCA` path pointing at the mounted proxy CA bundle (the proxy URLs
  themselves are passed as environment variables). See
  [OAuth API Server Deployment and Trust](#oauth-api-server-deployment-and-trust).
- Console Operator renders `Console.spec.authProxy` into an `auth.proxy` block in
  Console's generated `console-config.yaml`, holding the proxy URLs, resolved
  `noProxy` list, and a `trustedCAFile` path.

For example, the generated webhook configuration includes the following fields,
alongside its existing `jwt` configuration. The path is illustrative and must
match the operator-managed CA mount:

```yaml
apiVersion: authentication.openshift.io/v1alpha1
kind: AuthenticationConfiguration
proxyTrustedCA: /var/auth-proxy-ca/ca-bundle.crt
```

For the Console operator resource shown above, Console Operator generates the
following configuration fragment when the feature prerequisites are satisfied:

```yaml
auth:
  authType: oidc
  oidcIssuer: https://idp.example.com
  proxy:
    httpProxy: http://proxy.example.com:3128
    httpsProxy: http://proxy.example.com:3128
    noProxy:
      - idp.internal.example.com
      - ".cluster.local"
      - ".cluster.local."
      - ".svc"
      - ".svc."
      - localhost
      - localhost.
      - "127.0.0.1"
      # The Kubernetes service IP is also appended when available.
    trustedCAFile: /var/auth-proxy-ca/ca-bundle.crt
```

Console Operator expands `noProxy` with the internal defaults and translates the
`trustedCA` ConfigMap reference into the mounted bundle's `trustedCAFile` path.
The path is illustrative and must match the mount it configures on the Console
Deployment. The existing `authType` and `oidcIssuer` fields are shown for context.

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
operators. HyperShift's existing configuration-reference discovery covers
`spec.configuration` and cannot discover the new CA reference under its sibling
`spec.operatorConfiguration`. Discovery and synchronization must therefore be
extended to include that reference, even when `spec.configuration` is absent.
Source CA updates and deletion/recreation must trigger reconciliation so trust
changes propagate without requiring an edit to the HostedCluster.

Console Operator reads the component proxy from its own
`console.operator.openshift.io/cluster` resource on both topologies — it needs no
topology branch and no HyperShift-specific input. On HyperShift, HCCO writes that
guest resource from the shared `operatorConfiguration.authentication.proxy` block,
exactly as HyperShift already reconciles the guest `Network` and `IngressController`
resources from `operatorConfiguration`. Console Operator does not access
HostedCluster or HostedControlPlane resources or the management cluster.

On HyperShift, CPO performs the issuer validation that CAO performs on standalone:
CAO's reconciliation controller does not run in the hosted control plane (it only
contributes a bootstrap render step there, which does not validate issuers), so that
responsibility falls to CPO. CPO resolves the proxy from
`HostedControlPlane.spec.operatorConfiguration.authentication.proxy`, falling back to
the hosted cluster's cluster-wide proxy when that field is absent, and never to the
management cluster's own proxy.

Both CPO's validation client and the running webhook egress to the IdP **through
konnectivity into the guest network**, not directly from the management cluster. That
connection path, and the sidecar that applies the proxy, are detailed under
[Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths) below.

The proxy CA must reach both clusters through the following distribution path:

```text
Management cluster: namespace containing the HostedCluster resource (HostedCluster.metadata.namespace)
  User-provided proxy CA ConfigMap
    |
    | HyperShift operator copies ca-bundle.crt
    v
Management cluster: namespace running this hosted cluster's control plane (HostedControlPlane.metadata.namespace)
  Proxy CA ConfigMap
    |-- Read by CPO for issuer validation
    |-- Mounted on the konnectivity-https-proxy sidecar (HTTPS proxy certificate trust)
    |-- Mounted on the webhook container (certificate trust for TLS interception)
    |
    | HCCO copies ca-bundle.crt across clusters
    v
Guest cluster: openshift-config
  Proxy CA ConfigMap (referenced by the guest Console resource's trustedCA)
    |
    | Console Operator copies ca-bundle.crt
    v
Guest cluster: openshift-console
  Proxy CA ConfigMap mounted by Console
```

ConfigMap references must resolve in each consumer's cluster and namespace.
The HCP references its local copy. No copy is needed in the guest's
`openshift-oauth-apiserver` namespace because the webhook runs in the HCP.

On HyperShift, HCCO writes two guest resources from the HCP block:

- the `console.operator.openshift.io/cluster` proxy field, and
- the proxy CA ConfigMap in guest `openshift-config` that the field's `trustedCA`
  references.

Both reuse patterns HyperShift already has: reconciling guest
`operator.openshift.io` resources (as it does for `Network` and `IngressController`)
and copying a CA into guest `openshift-config` (as it does for the OIDC issuer CA).
CVO seeds the guest Console resource using a create-only manifest. HCCO reconciles
proxy additions, updates, and removal, preserving other Console fields and using
a managed CA name distinct from any user ConfigMap. CPO configures the webhook
from the same HCP block, independently.

Each synchronizing controller must watch CA content and reference changes so
rotation propagates through the entire chain, including both guest-side copies.
Validation and operand updates follow
[Configuration Updates and CA Reload](#configuration-updates-and-ca-reload).

##### Webhook Egress and Connection Paths

The CA distribution above is the trust path; this is the request path. Console and the
webhook reach the IdP differently per topology, because on HyperShift the webhook runs
in the management cluster but must egress as the guest.

On standalone, both operands egress directly through the component proxy:

```text
Standalone (single cluster)

  webhook (oauth-apiserver)    --HTTP(S)_PROXY env-------->  component proxy  -->  external IdP
  Console (openshift-console)  --auth.proxy transport----->  component proxy  -->  external IdP
```

On HyperShift, guest Console egresses directly (it runs on a worker node), but the
webhook's egress is tunneled through konnectivity into the guest network, where the
component proxy lives. A `konnectivity-https-proxy` sidecar in the webhook pod applies
the component proxy:

```text
HyperShift                                         (two HCP-side mechanisms, one egress path)

  management cluster: HCP namespace                     guest / worker-node network
  +-------------------------------------------+
  | oauth-apiserver pod (External OIDC mode)  |
  |   webhook container                       |
  |     HTTP(S)_PROXY = http://127.0.0.1:P --+|
  |   konnectivity-https-proxy sidecar  <----+|
  |     Layer 1: --https-proxy=<component>,   |
  |              --no-proxy=<in-cluster>      |
  |     Layer 2: konnectivity dialer ---------+--+
  |     (CA mounted in both containers)       |  |
  +-------------------------------------------+  |
                                                 +== konnectivity tunnel ==> konnectivity agent (worker node)
  +-------------------------------------------+  |                               |
  | CPO (issuer validation)                   |  |     in --no-proxy / internal: |--> in-guest endpoint (direct)
  |   in-process konnectivity dialer ---------+--+     otherwise:                |--> component proxy --> external IdP
  |   transport.Proxy = <component proxy>     |                                       (resolved + reached in guest net)
  |   (no sidecar; certs read via Kube API)   |
  +-------------------------------------------+

  Console (guest worker node)  --auth.proxy transport--------------------------------> component proxy --> external IdP
```

Two independent decisions compose in the sidecar: Layer 1 (`--https-proxy`/`--no-proxy`)
chooses whether to insert the proxy hop; Layer 2 (the konnectivity dialer) tunnels
every resulting TCP dial into the guest network by default. So both the webhook (via
the tunnel) and Console (natively) egress from the **same guest network**, and one
guest-reachable proxy serves both.

CPO's issuer-validation client takes the same path but by a different mechanism: being
the operator, it embeds the konnectivity dialer in-process rather than using a sidecar
(see [Operator Reconciliation and Issuer Validation](#operator-reconciliation-and-issuer-validation)).
Validating over the runtime path keeps validation and runtime from diverging.

Consequences:

- The component proxy must be reachable and DNS-resolvable **from the guest/worker-node
  network**, not the management cluster — konnectivity resolves via guest DNS and dials
  from a worker node.
- The sidecar's `--no-proxy` must list the in-cluster endpoints (KAS, the `token-review`
  Service, `.svc`, `.cluster.local`) so they tunnel through konnectivity directly rather
  than via the forward proxy.
- The proxy host is excluded from the sidecar's cloud-API direct-dial bypass, so the
  proxy itself is reached through konnectivity (from the guest), not from the management
  cluster.
- The component proxy takes precedence over the hosted cluster's
  `spec.configuration.proxy` for the webhook pod when set.
- Verify each plane independently: successful CPO issuer validation does not prove guest
  Console connectivity, and vice versa. See the
  [`NO_PROXY` rules](#no_proxy-and-internal-endpoint-urls).

##### Component Inventory

Covering both topologies, the feature breaks down as follows.

New components and artifacts:

| Added | Topology | Purpose |
| --- | --- | --- |
| Proxy field on `operatorv1.Console` | both | Console's own login-proxy configuration. |
| `operatorConfiguration.authentication.proxy` on HostedCluster/HostedControlPlane | HyperShift | Single hosted entry point that fans out to webhook and Console. |
| HCCO reconciler for the guest Console proxy | HyperShift | Writes the guest Console proxy field + guest `openshift-config` CA copy. |
| `konnectivity-https-proxy` sidecar on the External-OIDC oauth-apiserver pod | HyperShift | Applies the component proxy to the webhook's konnectivity egress (new, or an HTTPS/Dual extension of the existing socks5 injection). |

Existing components whose behavior changes:

| Affected | Change |
| --- | --- |
| CAO | Resolves the component proxy; injects env + mounts CA on the webhook Deployment and validates the issuer through it (standalone). |
| oauth-apiserver (webhook) | Reads `proxyTrustedCA`; outbound clients honor the proxy and trust; on HyperShift egress flows through the new sidecar. |
| CPO | Builds the webhook Deployment including the konnectivity sidecar flags and CA mount; validates the issuer via konnectivity + the component proxy (HyperShift). |
| Console Operator | Reads the Console operator proxy field; renders `auth.proxy` and mounts the CA. |
| Console | Applies `auth.proxy` to its OIDC clients only. |
| HyperShift operator | Copies `operatorConfiguration` HC→HCP; synchronizes the referenced CA HC→HCP. |

Configuration that must hold for it to work:

- Standalone: the proxy is set on **both** the Authentication and Console operator
  resources.
- HyperShift: the proxy is set once on the HostedCluster; it must be reachable from the
  **guest network**; the sidecar `--no-proxy` covers in-cluster endpoints; and the
  component proxy takes precedence over `spec.configuration.proxy` for the webhook pod.
- All topologies: `trustedCA` ConfigMaps carry `ca-bundle.crt` and are distributed as in
  the [CA path](#hypershift--hosted-control-planes) above.

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

The details below describe the components as they run on standalone clusters.
The hosted-cluster operator handoff, cross-cluster CA distribution, and CPO/HCCO
responsibilities are covered in
[Topology Considerations](#hypershift--hosted-control-planes); the resolution,
trust, and update rules here apply equally to their hosted counterparts.

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

Extend the component proxy's implicit `noProxy` entries to include trailing-dot
forms of the existing hostname defaults. The resulting defaults are
`.cluster.local`, `.cluster.local.`, `.svc`, `.svc.`, `localhost`, `localhost.`,
and `127.0.0.1`, plus the caller's Kubernetes service IP when available through
`KUBERNETES_SERVICE_HOST`.

Use these defaults consistently in CAO/CPO issuer-validation clients, the
webhook's `NO_PROXY`, and Console's `auth.proxy.noProxy`. Updating CAO's shared
resolver also expands the integrated authentication path's component-proxy
defaults; no other change to that flow is intended. Do not add these entries to
the cluster-wide fallback: preserve its supplied `NO_PROXY` values and Console's
global proxy environment unchanged.

#### `NO_PROXY` and Internal Endpoint URLs

Go matches `NO_PROXY` against the host in each request URL before DNS resolution;
it does not expand DNS search suffixes, match a hostname against its resolved IP,
or strip a trailing dot before suffix matching. The dotted and undotted default
suffixes above therefore cover both forms of the internal hostname patterns
(`oidc.ns.svc`, `oidc.ns.svc.cluster.local`, and their trailing-dot variants).
Administrators must still bypass other forms used by endpoints requiring direct
access:

| Host in the request URL | Additional `spec.proxy.noProxy` entry |
| --- | --- |
| `oidc` or `oidc.oidc-namespace` (short name) | The short hostname used in the URL. |
| A Route hostname or custom DNS alias | That hostname, or an appropriate domain suffix. |
| A Service or Pod IP other than the Kubernetes service IP | That IP, or a CIDR containing it. |

For example, `discoveryURL: https://oidc.oidc-namespace/.well-known/openid-configuration`
requires `oidc.oidc-namespace` in `spec.proxy.noProxy`. Entries are hosts or IPs
(optionally with ports) or CIDRs, not complete URLs, and only select direct
connectivity: the hostname must still resolve from the caller's namespace and
match the endpoint's TLS certificate.

#### Operator Reconciliation and Issuer Validation

CAO reconciles the webhook configuration and revalidates issuer discovery when
the provider configuration, effective proxy settings, or relevant CA bundles
change. Its issuer-validation HTTP client uses the effective proxy settings and
combined issuer and proxy trust, leaving its process environment and unrelated
clients unchanged.

CPO provides equivalent issuer validation for hosted control planes, using the
same effective proxy settings, bypass rules, and trust as the webhook. Validation
requests must follow the webhook's runtime path through konnectivity into the
guest network so that validation reflects runtime connectivity. CPO must not use
direct management-cluster egress or its process-wide proxy environment for these
requests. See [Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths).

#### OAuth API Server Deployment and Trust

The webhook's egress differs by topology, because on HyperShift it runs in the
management cluster and must egress through konnectivity (see
[Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths)):

- **Standalone.** CAO supplies the effective `HTTP_PROXY`, `HTTPS_PROXY`, and
  `NO_PROXY` values directly to the External OIDC container (including when
  `trustedCA` is omitted or the cluster-wide fallback applies). When `trustedCA` is
  set, CAO synchronizes the referenced ConfigMap from `openshift-config` into
  `openshift-oauth-apiserver` and mounts it read-only; the generated webhook
  configuration references the mounted bundle through `proxyTrustedCA`.
- **HyperShift.** CPO does not set the external proxy as container env — that env
  already points at the konnectivity sidecar. Instead it configures the webhook pod's
  `konnectivity-https-proxy` sidecar with the component proxy (`--https-proxy`,
  `--http-proxy`, `--no-proxy`). When `trustedCA` is configured, CPO mounts the bundle
  in both the sidecar and webhook container, referencing it through `proxyTrustedCA`
  in the webhook configuration. They validate different certificates as described
  below.

The webhook's existing HTTP clients already honor proxy environment variables,
so proxy URLs do not require new fields in its generated configuration file.

Issuer and external-source CA configuration continue to describe trust for those
endpoints. The component proxy CA supplies additional trust for an HTTPS proxy or
a TLS-intercepting proxy; it does not replace endpoint configuration.

##### TLS Connections and Certificate Validation

For proxied HTTPS endpoint requests on HyperShift, the sidecar connects to the
external proxy, while the webhook establishes TLS to the endpoint through
the CONNECT tunnel. The sidecar forwards that endpoint TLS traffic without
decrypting it. If the external proxy intercepts TLS, it presents a replacement
endpoint certificate to the webhook and establishes a separate TLS connection to
the endpoint.

The term "proxy CA" covers two distinct roles:

- A **proxy server CA** signs the proxy's own certificate, for example for
  `proxy.example.com`. The sidecar uses this trust when connecting to an HTTPS
  proxy.
- An **interception CA** signs replacement endpoint certificates, for example for
  `idp.example.com`. The webhook accepts these only if their certificate chain
  leads to a CA it trusts. The proxy must independently validate the real
  endpoint's certificate; the webhook does not see that original chain.

These roles may use the same CA or different CAs. An HTTP proxy can use an
interception CA even though its own connection has no TLS server certificate.

| External proxy behavior | Sidecar validates | Webhook validates |
| --- | --- | --- |
| HTTP proxy, no TLS interception | No certificate on the proxy connection. | Endpoint certificate using normal endpoint trust. |
| HTTPS proxy, no TLS interception | Proxy certificate using the proxy server CA. | Endpoint certificate using normal endpoint trust. |
| HTTP proxy with TLS interception | No certificate on the proxy connection. | Replacement endpoint certificate using the proxy's interception CA. |
| HTTPS proxy with TLS interception | Proxy certificate using the proxy server CA. | Replacement endpoint certificate using the proxy's interception CA. |

HTTP versus HTTPS here refers to the proxy URL scheme, not the endpoint scheme or
the `httpsProxy` field name. If the required CAs are not already trusted, the
`trustedCA` bundle must contain their public certificates.
Mounting the bundle only on the sidecar is insufficient for TLS interception:
the webhook must also load it through `proxyTrustedCA`.

#### Console Login and Token Refresh

Console Operator reads the proxy from its own `console.operator.openshift.io/cluster`
resource — the [new Console operator field](#new-console-operator-api) — on both
topologies, with no topology branch. On HyperShift, HCCO populates that guest
resource (see [Topology Considerations](#hypershift--hosted-control-planes)); on
standalone the administrator sets it directly. From that field it applies the shared
feature-gate and resolution rules and supplies the component settings through a new
optional `auth.proxy` block in Console's existing `console-config.yaml`. When a proxy
CA is configured, it synchronizes the referenced bundle into `openshift-console` and
mounts it separately from issuer trust.

See [Generated Operand Configuration](#generated-operand-configuration) for an
example of the Console configuration rendered from `Console.spec.authProxy`.

The proposed block contains `httpProxy`, `httpsProxy`, the resolved `noProxy` list
including implicit defaults, and an optional `trustedCAFile` path. The existing
top-level `proxy` section configures plugin reverse proxies and is not reused.

Console applies `auth.proxy` only to its OIDC discovery, JWKS, code-exchange, and
refresh clients, using the same proxy-matching semantics as the webhook. This
works with or without custom issuer or proxy CAs. Changing Console's global proxy
environment would also reroute unrelated traffic, so authentication uses its own
configuration while other clients retain their existing routing and trust.

Console Operator emits `auth.proxy` only when the component proxy is configured
and its [feature prerequisites](#feature-gates) are satisfied:

- When `auth.proxy` is absent, Console retains its existing environment-based
  cluster-wide proxy or direct-connect behavior and corresponding trust.
- When `auth.proxy` is present, the OIDC clients use it in full. Missing fields
  do not inherit environment values; for example, an omitted `httpsProxy` means
  direct HTTPS access even if the global `HTTPS_PROXY` variable is set.

The proxy CA supplements the applicable issuer trust, preserving existing
system-root behavior when no issuer CA is specified.

#### Configuration Updates and CA Reload

Proxy settings and CA changes follow this update contract:

| Change | Operand behavior |
| --- | --- |
| Proxy settings, including bypass rules, or CA references/mounts | Roll out the affected Deployment. |
| Contents of an already-referenced proxy CA bundle: Console on either topology, or the standalone webhook | Reload trust without a rollout. |
| Contents of an already-referenced proxy CA bundle: hosted webhook | Roll out the OAuth API server Deployment to refresh sidecar trust. |
| Component proxy removal or gate disablement | Reconcile away the component settings and dedicated CA input, restoring cluster-wide proxy or direct-connect fallback. |

CAO and Console Operator synchronize source CA changes to the mounted copies;
the [hosted distribution path](#hypershift--hosted-control-planes) adds HyperShift
operator and HCCO synchronization. CAO/CPO also refresh the trust used for issuer
validation.

Console and the standalone webhook watch the mounted proxy CA file and apply
valid trust updates atomically to all affected authentication clients, without
restarting or changing unrelated clients. New TLS connections must use the updated
trust. Invalid updates are reported while retaining the last successfully loaded
trust bundle; certificate verification must not be disabled.

On HyperShift, `konnectivity-https-proxy` does not hot-reload its proxy CA trust.
CPO must therefore roll out the OAuth API server Deployment when the referenced
CA bundle's contents change. This replaces the entire Pod, restarting both the
sidecar and webhook even though the webhook itself supports CA hot reload.
Updating the mounted ConfigMap alone is insufficient. Adding hot reload to the
sidecar is outside this proposal.

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
On HyperShift, recovery also requires independent management-cluster access with
permission to update the HostedCluster and source CA ConfigMap. Guest-cluster
credentials alone cannot repair these authoritative inputs.

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
On HyperShift, issuer validation and webhook requests also depend on konnectivity,
guest DNS, and worker-network availability; webhook egress additionally depends
on its sidecar. OIDC authentication can fail while the management-side API remains
reachable. Diagnostics must distinguish these dependencies from proxy failures.
Do not fall back to direct management-cluster egress when the guest-network path
fails.

**Partial application across management and guest clusters.**
The hosted webhook and guest Console reconcile asynchronously and can temporarily
use different proxy settings or CA bundles. Configuration and CA synchronization
failures must be reported through the responsible operators' existing status
conditions. Unsupported version combinations follow the
[Version Skew Strategy](#version-skew-strategy). For routine CA rotation,
distribute a bundle containing both old and new CAs and wait for the hosted
webhook rollout and Console reload to complete before switching proxy
certificates. Remove the old CA only after verifying both authentication paths.

**Tenant configuration and trust isolation.**
Incorrect synchronization could apply one hosted cluster's proxy or CA to another
or affect management-cluster traffic. Scope configuration, CA distribution, and
reconciliation permissions to the intended hosted cluster. Tenant settings must
not change management-wide proxy configuration or trust, or unrelated clients.
Verify isolation between hosted clusters sharing a management cluster, as covered
in the [hosted test plan](#hosted-control-planes-and-upgrades).

**Proxy credential leakage.**
Credentials embedded in proxy URLs are stored in operator resources and, on
HyperShift, HostedCluster and HostedControlPlane resources. They are propagated
to the standalone webhook's environment, the hosted konnectivity sidecar's
command-line arguments, and Console's generated ConfigMap, not protected by a
Secret reference. Restrict access to these resources and Pod specifications,
and redact credentials from diagnostics, including sidecar debug logs. This
limitation is inherited from the existing proxy API; adding a separate credential
Secret reference is outside this proposal.

**Debugging complexity from dual proxy sources.**
When both component-scoped and cluster-wide proxies exist, diagnosing failures
requires checking effective settings, endpoint bypass matches, and separate
endpoint/proxy trust. Document the [resolution rules](#proxy-resolution) and
Console's OIDC-only scope so administrators do not mistake its global environment
for its authentication override.

**Authentication disruption during rollout.**
Changes requiring a rollout under the
[update contract](#configuration-updates-and-ca-reload) can briefly disrupt
single-replica deployments, including SNO. This also applies to hosted webhook
CA rotation because its sidecar requires a rollout. Plan these changes with
recovery access available.

### Drawbacks

Standalone administrators must configure the proxy separately on the
Authentication and Console operator resources. HyperShift avoids this duplication
by using one HostedCluster setting for both.

The feature requires coordinated changes across several operators and operands.
Console's OIDC-only proxy handling and HyperShift's cross-cluster CA distribution
add implementation and testing complexity. These changes support authentication
only, rather than providing a general per-component proxy solution.

## Alternatives (Not Implemented)

### Shared Versus Per-Operator Proxy Configuration

Console could reuse `Authentication.spec.proxy` instead of adding its own operator
field. This would let standalone administrators configure the authentication proxy
once, but would couple Console to another operator's API. The proposal instead
keeps each operator's configuration on its own resource, at the cost of duplicate
standalone configuration.

On HyperShift, the alternative would be to mirror that separation with independent
webhook and Console proxy blocks on HostedCluster. However, HostedCluster is
already a single resource owned by HyperShift: separate blocks would duplicate
configuration within the same resource without preserving any additional API
ownership boundary. The proposal therefore uses one shared
`operatorConfiguration.authentication.proxy` block and distributes its settings
to the webhook and Console, preserving their operator-specific configuration
without requiring the hosted administrator to enter it twice.

## Open Questions [optional]

- Confirm the proposed `Console.spec.authProxy` name, schema, and validation with
  the assigned Console API approver.
- Identify an API approver for the HyperShift additions and confirm the shared
  `operatorConfiguration.authentication.proxy` placement on HostedCluster and
  HostedControlPlane, its validation, and the management-side feature gate.
- Confirm Console's proposed `auth.proxy` configuration format, OIDC-only scope,
  and proxy CA hot-reload contract with Console maintainers.
- Confirm the [webhook egress integration](#webhook-egress-and-connection-paths)
  with HyperShift maintainers, including compatibility with existing SOCKS5
  egress and the [CA-triggered webhook rollouts](#configuration-updates-and-ca-reload).

## Test Plan

Testing follows the integrated-authentication proposal's input validation,
authentication flow, operator health, and proxy resolution coverage, extended to
the External OIDC webhook, Console, and HyperShift.

### Input Validation and Unit Tests

Reuse the authentication proxy CRD validation tests in `openshift/api` for URL
schemes, hostnames, paths, query strings, fragments, CA reference names, list
constraints, and the requirement to supply at least one proxy URL. Apply the same
validation to the new Console operator proxy field, and add equivalent HyperShift
API tests, including feature-gated admission and serialization compatibility for
the optional HostedCluster and HostedControlPlane fields.

Unit and controller tests in CAO, OAuth API server, Console, Console Operator, and
HyperShift cover:

- Full component replacement, cluster-wide fallback, and direct connectivity,
  including partially populated proxy configurations with no field inheritance.
- The [feature-gate matrix](#feature-gates), including webhook proxy support with
  `ExternalOIDCAsWebhook` enabled and `ExternalOIDCExternalClaimsSourcing` disabled,
  Console gated by `AuthenticationComponentProxyExternalOIDC` alone (independent of
  the webhook gates and of `AuthenticationComponentProxy`), fallback when the proxy
  gate is disabled, and exclusion of direct kube-apiserver OIDC. Integrated OAuth
  behavior remains unchanged apart from the shared component-proxy bypass defaults.
- Console config generation and parsing preserve the distinction between absent
  `auth.proxy` and a present block with omitted fields. Explicit OIDC settings
  must not inherit global proxy fields, even without custom issuer or proxy CAs.
  Applying, updating, and removing the block must leave the global proxy
  environment and non-OIDC clients' transports and trust unchanged.
- Proxy injection with and without `trustedCA`, informer-triggered reconciliation,
  CA reference changes, missing or invalid ConfigMaps, and removal of managed
  configuration, including HCCO reconciling the guest Console operator resource and
  its CA copy from the HCP block while co-managing only the proxy field.
- Issuer/source trust combined with proxy trust, with and without custom endpoint
  CAs, for every outbound client including client-credentials token acquisition.
- The [update contract](#configuration-updates-and-ca-reload), including which
  changes alter Pod templates, projected-volume file replacement, cached OIDC/JWKS
  clients, stale transport retirement, and last-valid-trust handling on errors.
- Hostname-based `NO_PROXY` matching, including Service FQDNs, short names, aliases,
  trailing-dot names, IPs, and endpoints advertised by discovery or distributed
  claims. Verify automatic bypass for dotted and undotted default hostname forms
  across all component-proxy consumers, including integrated authentication.
  External hosts and lookalikes such as `idp.ns.svc.cluster.local.example.com`
  must remain proxied unless explicitly bypassed. Cluster-wide fallback values
  and Console's unrelated clients must remain unchanged.

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
claims, with `ExternalOIDCExternalClaimsSourcing` disabled and no external claim
sources configured. Verify proxy routing and `NO_PROXY` bypass for the claim
endpoint and for discovery and JWKS retrieval of the returned JWT's issuer,
including when these use hosts different from the original issuer.

Exercise unreachable proxies, latency/timeouts, invalid or missing CA bundles,
and incorrect `NO_PROXY` entries. Verify diagnostic conditions/logs, no silent
proxy bypass, and retained client-certificate access. Use that access to correct
or remove `spec.proxy`, then verify recovery through a reachable fallback.

Run webhook tests with `ExternalOIDCAsWebhook` enabled. Cover authentication with
`ExternalOIDCExternalClaimsSourcing` disabled and no external claim sources, then
enable that gate to test configured sources using anonymous, request-token, and
client-credentials authentication. Test CAO's discovery validation independently
from runtime discovery, JWKS refresh, and claim retrieval.

Conditions must recover after correction and distinguish proxy failures from
endpoint TLS/connectivity errors. Successful authentication, not Pod readiness
alone, is the acceptance criterion. Retain regression coverage for integrated
OAuth and clusters not using the feature.

### CA Rotation

On standalone clusters, rotate the proxy CA bundle and proxy certificate without
replacing webhook or Console Pods or changing their Pod templates. Repeat the
no-rollout check for Console on hosted clusters; the hosted webhook rollout is
covered below. Exercise all outbound paths in
the [architecture table](#authentication-architecture), including Console providers
initialized before rotation, distributed claims, and external-source credentials.
Use fresh JWKS retrieval and new TLS connections, and test both adding and
removing CAs so caches or connections cannot mask stale trust. Confirm invalid
updates are reported, last-valid trust remains effective, and a subsequent valid
update loads without a restart.

### Hosted Control Planes and Upgrades

Run the authentication and recovery scenarios on hosted clusters. Verify the
HostedCluster-to-HCP configuration copy, CA synchronization into the HCP and guest
`openshift-config` and `openshift-console` namespaces, HCCO writing the guest
`console.operator.openshift.io/cluster` proxy field (and its referenced CA
ConfigMap) from the single `operatorConfiguration.authentication.proxy` block, and
Console reconciliation from its own resource. Verify HCCO co-manages only the proxy
field on the CVO-seeded Console resource. Run the CA rotation scenarios through the
full distribution chain, checking destination-local references. Test updates,
removal, missing references, and isolation between two hosted clusters. Assert
that the guest cluster-wide Proxy and NodePool rollout hashes remain unchanged.

Cover the [TLS validation matrix](#tls-connections-and-certificate-validation),
including different CAs for the HTTPS proxy's own certificate and its intercepted
endpoint certificates. Verify CA rotation reaches both the sidecar's proxy
connection and the webhook's endpoint connection through an OAuth API server
Deployment rollout, replacing both containers. Guest Console must reload the
updated CA without a rollout.

Verify the webhook egress specifically: the proxy is applied via the oauth-apiserver
pod's `konnectivity-https-proxy` sidecar, the webhook reaches the IdP from the guest
network through konnectivity, in-cluster endpoints (KAS, `token-review`, `.svc`) stay
on the konnectivity tunnel via `--no-proxy` rather than the forward proxy, and the
component proxy takes precedence over `spec.configuration.proxy`. Confirm the proxy
needs reachability only from the guest network: a proxy reachable from the guest but
not from the management cluster must still work for the webhook. Include a case where
the proxy is unreachable from the guest network; successful CPO discovery validation
must not substitute for a successful Console login and refresh, and vice versa.

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

HCP and guest reconciliation remain asynchronous; this proposal adds no aggregate
condition confirming proxy and CA propagation across both paths. No new kubelet
or node API dependencies are introduced, and existing platform skew limits remain
unchanged.

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
guest `console.operator.openshift.io/cluster` proxy field HCCO writes for Console.
Trace the configuration from the HostedCluster source; editing a generated guest
resource or Deployment is not a persistent fix. Check management-plane and guest
connectivity separately.

Correct the proxy URL, bypass list, or referenced CA at the source. Alternatively,
remove the component proxy to restore a verified working cluster-wide proxy or
direct path. On standalone clusters the sources are
`authentication.operator.openshift.io/cluster` (webhook) and
`console.operator.openshift.io/cluster` (Console login); on hosted clusters both
derive from the single HostedCluster component proxy field. Do not disable TLS verification or remove
the TokenReview webhook as a proxy workaround. Reconciliation resumes after the
configuration is fixed; verify both API token authentication and a fresh Console
login/refresh, not just cleared operator conditions.

Disabling component proxy use does not delete user or workload data, but affected
users cannot submit new API requests while authentication is broken. Existing
workloads and service-account authentication do not depend on this OIDC proxy.
Redact proxy credentials, client secrets, and tokens from diagnostic output and
support attachments.
