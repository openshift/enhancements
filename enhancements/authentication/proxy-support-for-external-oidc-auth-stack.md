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
last-updated: 2026-10-22
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
to External OIDC authentication. It enables the OAuth API server's External OIDC
webhook authenticator and OpenShift Console to reach external identity providers,
OIDC distributed claim endpoints,
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
the integrated OAuth server and Cluster Authentication Operator (CAO), deferring
External OIDC and HyperShift support.
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

- Extend component-scoped proxy support to the webhook's External OIDC discovery,
  JWKS retrieval, distributed claim resolution, and external claim
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

CAO renders proxy environment variables
into the External OIDC OAuth API server Deployment, synchronizes and mounts the
optional CA bundle, and passes its path through `trustedCAFilePath` in the
generated OAuth API server configuration. The webhook uses this configuration
for its outbound requests. The TokenReview connection from
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

Apply the proxy before the OIDC provider that needs it so the webhook and Console
can reach the provider when they first perform discovery. If the provider is
configured first, runtime discovery or login can fail transiently until the proxy
configuration is reconciled.

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
5. CAO reconciles the webhook Deployment with the proxy environment variables and
   optional CA mount, and sets `trustedCAFilePath` in the generated OAuth API server
   configuration to the mounted bundle's path. Console Operator reconciles the
   corresponding `auth.proxy` section in the Console configuration file and CA
   mount for Console's OIDC clients from its own operator resource.
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
5. The Control Plane Operator (CPO) configures the HCP-side OAuth API server
   Deployment so its egress applies the component proxy and passes the optional
   mounted CA bundle's path through `trustedCAFilePath`. Because this egress is
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
  `trustedCAFilePath` pointing at a mounted generic CA bundle trusted for outbound
  HTTPS requests and connections to HTTPS proxies. This enhancement supplies the
  component proxy CA through that field; proxy URLs themselves are passed as
  environment variables. See
  [OAuth API Server Deployment and Trust](#oauth-api-server-deployment-and-trust).
- Console Operator renders `Console.spec.authProxy` into an `auth.proxy` block in
  Console's generated `console-config.yaml`, holding the proxy URLs, resolved
  `noProxy` list, and a `trustedCAFilePath` path.

For example, the generated webhook configuration includes the following fields,
alongside its existing `jwt` configuration. The path is illustrative and must
match the operator-managed CA mount:

```yaml
apiVersion: authentication.openshift.io/v1alpha1
kind: AuthenticationConfiguration
trustedCAFilePath: /var/auth-proxy-ca/ca-bundle.crt
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
      - ".cluster.local"
      - ".cluster.local."
      - ".svc"
      - ".svc."
      - "127.0.0.1"
      - "172.30.0.1"              # Kubernetes service IP, when the operator sees one
      - idp.internal.example.com
      - localhost
      - localhost.
    trustedCAFilePath: /var/auth-proxy-ca/ca-bundle.crt
```

Console Operator expands `noProxy` with the internal defaults, then sorts the
result and removes duplicates. It also translates the
`trustedCA` ConfigMap reference into the mounted bundle's `trustedCAFilePath`.
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

On HyperShift, CPO reconciles the webhook Deployment and its generated
configuration. CPO resolves the proxy from
`HostedControlPlane.spec.operatorConfiguration.authentication.proxy`, falling back to
the hosted cluster's cluster-wide proxy when that field is absent, and never to the
management cluster's own proxy.

The running webhook egresses to the IdP **through konnectivity into the guest
network**. That
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
HyperShift

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
                                                                                  |
                                                       in --no-proxy / internal: |--> in-guest endpoint (direct)
                                                       otherwise:                |--> component proxy --> external IdP
                                                                                      (resolved + reached in guest net)

  Console (guest worker node)  --auth.proxy transport--------------------------------> component proxy --> external IdP
```

Two independent decisions compose in the sidecar: Layer 1 (`--https-proxy`/`--no-proxy`)
chooses whether to insert the proxy hop; Layer 2 (the konnectivity dialer) tunnels
every resulting TCP dial into the guest network by default. So both the webhook (via
the tunnel) and Console (natively) egress from the **same guest network**, and one
guest-reachable proxy serves both.

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
- Verify the webhook and guest Console independently: successful webhook
  authentication does not prove Console connectivity, and vice versa. See the
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
| CAO | Resolves the component proxy; injects env + mounts CA on the webhook Deployment and sets `trustedCAFilePath` in its configuration (standalone). |
| oauth-apiserver (webhook) | Reads the generic outbound CA bundle through `trustedCAFilePath` for HTTPS endpoint trust, plus the proxy's own certificate on standalone only; outbound clients honor the proxy; on HyperShift egress flows through the new sidecar, which holds that proxy trust instead. |
| CPO | Builds the webhook Deployment including the konnectivity sidecar flags and both CA mounts; sets `trustedCAFilePath` in the webhook configuration (HyperShift). |
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
and `127.0.0.1`, plus the Kubernetes service IP. The operator, not the operand,
supplies that last entry: each operator reads `KUBERNETES_SERVICE_HOST` from its
own process environment and bakes the resulting value into the rendered bypass
list. Operands do not resolve it at runtime.

Use these defaults consistently in the webhook's effective bypass rules and
Console's `auth.proxy.noProxy`. Updating CAO's shared resolver also expands the
integrated authentication path's component-proxy
defaults; no other change to that flow is intended. Do not add these entries to
the cluster-wide fallback: preserve its supplied `NO_PROXY` values and Console's
global proxy environment unchanged.

Regarding the cluster-wide fallback and the component proxy, they act at different layers
for Console, and both are always present. Console Operator has always rendered the
cluster-wide proxy from `proxy.config.openshift.io/cluster` as `HTTP_PROXY`,
`HTTPS_PROXY`, and `NO_PROXY` **environment variables on the Console Deployment**,
which is the environment-based fallback described under
[Console Login and Token Refresh](#console-login-and-token-refresh). Those
variables remain set when `auth.proxy` is present, because they still govern
Console's unrelated clients. Console must therefore install an explicit proxy
selector on its OIDC clients whenever `auth.proxy` is present.

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

#### Operator Reconciliation and Configuration Generation

CAO reconciles the webhook Deployment and generated configuration when provider
configuration, effective proxy settings, or CA references change. For the CA
bundle, it synchronizes and mounts the referenced ConfigMap and sets
`trustedCAFilePath` to the mounted file's path. CAO does not contact issuer or
claim-source endpoints before generating the OAuth API server configuration.
The OAuth API server loads the bundle and performs outbound discovery, JWKS, and
claim-source requests using the configured proxy and trust.

CPO performs the corresponding configuration and mount reconciliation for hosted
control planes, including the konnectivity sidecar settings. Endpoint connectivity
is exercised by the running webhook through the guest network, as described under
[Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths).

#### OAuth API Server Deployment and Trust

The webhook's egress differs by topology, because on HyperShift it runs in the
management cluster and must egress through konnectivity (see
[Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths)):

- **Standalone.** CAO supplies the effective `HTTP_PROXY`, `HTTPS_PROXY`, and
  `NO_PROXY` values directly to the External OIDC container (including when
  `trustedCA` is omitted or the cluster-wide fallback applies). When `trustedCA` is
  set, CAO synchronizes the referenced ConfigMap from `openshift-config` into
  `openshift-oauth-apiserver` and mounts it read-only; the generated webhook
  configuration references the mounted bundle through `trustedCAFilePath`.
- **HyperShift.** CPO does not set the external proxy as container env — that env
  already points at the konnectivity sidecar. Instead it configures the webhook pod's
  `konnectivity-https-proxy` sidecar with the component proxy (`--https-proxy`,
  `--http-proxy`, `--no-proxy`). When `trustedCA` is configured, CPO mounts the bundle
  in both the sidecar and webhook container, referencing it through `trustedCAFilePath`
  in the webhook configuration. The webhook never validates the component proxy's own
  certificate on this topology: it reaches the sidecar over plaintext loopback, and the
  sidecar is what connects to the proxy. Its copy of the bundle covers only TLS
  interception, where the proxy presents a replacement endpoint certificate
  end-to-end through the CONNECT tunnel. The bundle is opaque, so CPO cannot tell
  which role a given certificate serves and mounts the same bundle in both places;
  against a non-intercepting proxy the webhook's copy simply goes unused. The two
  containers validate different certificates as described below.

The webhook's existing HTTP clients already honor proxy environment variables,
so proxy URLs do not require new fields in its generated configuration file.

`trustedCAFilePath` is a generic outbound trust input: its CA bundle is trusted
for outbound HTTPS endpoint requests, whether direct or proxied, and connections
to HTTPS proxies. It can contain endpoint CAs, proxy server CAs, and interception
CAs. This enhancement populates it with the component proxy CA bundle. Issuer and
external-source CA configuration continue to describe trust for those endpoints;
the generic bundle supplies additional trust alongside that configuration.

##### TLS Connections and Certificate Validation

For proxied HTTPS endpoint requests on HyperShift, the sidecar connects to the
external proxy, while the webhook establishes TLS to the endpoint through
the CONNECT tunnel. The sidecar forwards that endpoint TLS traffic without
decrypting it. If the external proxy intercepts TLS, it presents a replacement
endpoint certificate to the webhook and establishes a separate TLS connection to
the endpoint.

The term "proxy CA" covers two distinct roles:

- A **proxy server CA** signs the proxy's own certificate, for example for
  `proxy.example.com`. Whichever component opens the connection to an HTTPS proxy
  needs this trust: the webhook itself on standalone, the sidecar on HyperShift.
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
On HyperShift this splits cleanly across the two containers, which is why both
mounts exist: the sidecar's copy covers an HTTPS proxy's own certificate, which
the webhook never sees, and the webhook's copy covers interception, which the
sidecar never sees because it forwards the tunnel without decrypting it.
Mounting the bundle only on the sidecar is therefore insufficient for TLS
interception: the webhook must also load it through `trustedCAFilePath`.

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
including implicit defaults, and an optional `trustedCAFilePath`. The existing
top-level `proxy` section configures plugin reverse proxies and is not reused.

Console applies `auth.proxy` only to its OIDC discovery, JWKS, code-exchange, and
refresh clients, using the same proxy-matching semantics as the webhook. This
works with or without custom issuer or proxy CAs. Changing Console's global proxy
environment would also reroute unrelated traffic, so authentication uses its own
configuration while other clients retain their existing routing and trust.

Console Operator emits `auth.proxy` only when the component proxy is configured
with at least one of `httpProxy` or `httpsProxy`, the authentication type is
`oidc`, and its [feature prerequisites](#feature-gates) are satisfied:

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
operator and HCCO synchronization. CAO/CPO maintain the CA mount and
`trustedCAFilePath` in the generated webhook configuration.

Console watches its mounted proxy CA file; the standalone webhook watches the
generic outbound CA bundle referenced by `trustedCAFilePath`. They apply valid
trust updates atomically to the clients using those bundles without restarting.
New TLS connections must use the updated trust. Invalid updates are reported
while retaining the last successfully loaded trust bundle; certificate
verification must not be disabled. By reported we mean logging reload errors,
because not much more can be trivially done in the operand. The operator should
perform the checks necessary to ensure the ConfigMap is valid before mounting it
for the operator and degrade on any issues found so that the issue becomes visible.

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
can break token validation, Console login, or refresh. Successful runtime issuer
discovery does not prove that every endpoint is reachable. Administrators must
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
On HyperShift, webhook requests also depend on konnectivity, guest DNS,
worker-network availability, and the webhook's sidecar. OIDC authentication can fail while the management-side API remains
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
in the [hosted test plan](#hosted-clusters).

**Proxy credential leakage.**
Credentials embedded in proxy URLs are stored in operator resources and, on
HyperShift, HostedCluster and HostedControlPlane resources. They are propagated
to the standalone webhook's environment, the hosted konnectivity sidecar's
command-line arguments, and Console's generated ConfigMap, not protected by a
Secret reference. Restrict access to these resources and Pod specifications,
and redact credentials from diagnostics, including sidecar debug logs. This
limitation is inherited from the existing proxy API; adding a separate credential
Secret reference is outside this proposal.

The new Console field has the same exposure: `Console.spec.authProxy.httpProxy`
and `httpsProxy` are plain-text strings on the `Console` CRD, and Console
Operator copies them verbatim into the generated `console-config.yaml` ConfigMap
in `openshift-console`, readable by anyone who can read either object. Operands
need to log their effective proxy — support cannot otherwise tell a component
proxy from the cluster-wide fallback — so they must strip userinfo from the URL
before logging it, rather than omitting the log line.

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

- Identify an API approver for the HyperShift additions and confirm the shared
  `operatorConfiguration.authentication.proxy` placement on HostedCluster and
  HostedControlPlane, its validation, and the management-side feature gate.
- Confirm the [webhook egress integration](#webhook-egress-and-connection-paths)
  with HyperShift maintainers, including compatibility with existing SOCKS5
  egress and the [CA-triggered webhook rollouts](#configuration-updates-and-ca-reload).

## Test Plan

Tests are grouped by authentication path. Each section starts with the
end-to-end flows, which span an operator and its operand and are the primary
coverage, and then lists the component-scoped tests underneath so each owning
party sees its own scope in one place. Unit tests cover pure functions with
easily asserted inputs and outputs; integration tests run the operator sync loops
against a test API server and assert the resulting cluster state. The operands
may add additional unit or integration tests of their own, but the operator tests
cover configuration generation and the end-to-end flows cover observable behavior.

### Test Infrastructure

End-to-end coverage builds on the two existing Origin suites in
`test/extended/authentication`. The integrated-authentication component proxy
tests supply a forward proxy with HTTP and HTTPS listeners and a self-signed CA,
the `trustedCA` ConfigMap helpers, and the save/restore of cluster authentication
state. The External OIDC suite supplies Keycloak, the helpers that switch the
cluster to `Type: OIDC`, the break-glass certificate kubeconfig, and an external
claims endpoint. Tests run serial and disruptive, like their sources.

### Token Validation Through the External OIDC Webhook

**End-to-end flows** spanning CAO and the webhook container in the oauth-apiserver
Pod:

- Setting `spec.proxy` on `authentication.operator.openshift.io/cluster` makes
  token validation succeed with the webhook's provider requests observed on the
  proxy. Run this with an HTTPS proxy and `trustedCA`, and with a plain HTTP
  proxy without one.
- `noProxy` containing the issuer host bypasses the proxy, and authentication
  still succeeds once the proxy is no longer reachable.
- A missing `trustedCA` ConfigMap takes the authentication cluster operator
  Degraded with an actionable message; creating the ConfigMap clears the condition
  and authentication works.
- Removing `spec.proxy` restores the previous behavior, with the component proxy
  confirmed to be out of the path.
- Rotating the proxy's serving certificate and the contents of the referenced
  `trustedCA` ConfigMap lets token validation succeed again over the rotated
  proxy, with the operator resettling promptly.
- With `ExternalOIDCExternalClaimsSourcing` enabled, claim retrieval from the
  suite's external claims endpoint also goes through the proxy.

**Cluster Authentication Operator — unit tests.**

- Proxy resolution: the component proxy used in full when configured and gated,
  cluster-wide fallback otherwise, and no per-field inheritance.
- Generated webhook configuration and container environment: `trustedCAFilePath`
  set only when `trustedCA` is configured, the resolved `HTTP_PROXY`,
  `HTTPS_PROXY`, and `NO_PROXY` values including the implicit `noProxy` defaults
  and their trailing-dot forms, and removal of all of it when the component proxy
  is removed.

**Cluster Authentication Operator — integration tests.**

- Synchronizing the referenced `trustedCA` ConfigMap from `openshift-config` into
  `openshift-oauth-apiserver`, mounting it, and cleaning up when the proxy is
  removed.
- The [update contract](#configuration-updates-and-ca-reload): proxy and CA
  reference changes alter the Deployment Pod template; CA content changes only
  replace the mounted file.

### Console Login

**End-to-end flows** spanning Console Operator and Console:

- Setting `spec.authProxy` with a `trustedCA` bundle on
  `console.operator.openshift.io/cluster` sends Console's OIDC requests
  (discovery, JWKS, token endpoint) through the proxy, with login succeeding and
  the console cluster operator and the Console entry in `status.oidcClients`
  staying healthy.
- A missing `trustedCA` ConfigMap takes the console cluster operator Degraded, in
  the same way as the existing OIDC issuer CA lookup; creating the ConfigMap
  clears the condition and login works.
- Rotating the proxy's serving certificate and the referenced `trustedCA`
  ConfigMap contents lets Console's OIDC requests succeed again over the rotated
  proxy.
- Removing the field restores the previous behavior, with the component proxy
  confirmed to be out of the path.

**Console Operator — unit tests.**

- Resolution: the field applies only to `Type: OIDC`, and integrated OAuth
  clusters are unaffected.
- `console-config.yaml` generation: the `auth.proxy` block with resolved
  `noProxy` and `trustedCAFilePath`, and removal when the proxy is removed.

**Console Operator — integration tests.**

- Synchronizing the referenced `trustedCA` ConfigMap into `openshift-console`,
  mounting it separately from issuer trust, and cleaning up on removal.
- The [update contract](#configuration-updates-and-ca-reload): proxy and CA
  reference changes alter the Console Deployment; CA content changes do not.

### Hosted Clusters

Hosted coverage lands with the hosted implementation.

**End-to-end flows** spanning the HyperShift operator, CPO, HCCO, and both
operands. Rerun both sets of flows above on a hosted cluster, and additionally
verify:

- Configuration and CA propagation through the full chain into the HCP and guest
  `openshift-config` and `openshift-console` namespaces.
- Webhook egress through the sidecar: the webhook reaches the provider from the
  guest network, in-cluster endpoints stay on the konnectivity tunnel, and a proxy
  reachable only from the guest network works.
- A proxy CA content change rolls out the OAuth API server Deployment while guest
  Console reloads without a rollout.
- Isolation between two hosted clusters, with the guest cluster-wide Proxy and
  NodePool rollout hashes unchanged.
- The [version-skew guarantees](#version-skew-strategy) with separate
  `spec.controlPlaneRelease` and `spec.release` selections.

**HyperShift operator — unit tests.**

- Validation of the optional HostedCluster and HostedControlPlane fields,
  including feature-gated admission and serialization compatibility.
- Propagation of the HostedCluster block to the HostedControlPlane.

**CPO and HCCO — unit and integration tests.**

- The `konnectivity-https-proxy` sidecar arguments derived from the HCP block.
- CPO reconciles the webhook configuration and the CA mounts in both the sidecar
  and the webhook container; removal cleans all of it up.
- HCCO writes only the proxy field on the CVO-seeded guest
  `console.operator.openshift.io/cluster` resource, plus its CA copy, using
  destination-local references.
- A CA bundle content change triggers an OAuth API server Deployment rollout,
  because the sidecar does not hot-reload trust.

### Regression and Upgrade Coverage

Keep coverage for integrated OAuth clusters and for External OIDC clusters that
do not configure a component proxy. Standard upgrade jobs cover the feature
disabled. Feature-enabled upgrade and rollback scenarios run in development jobs
under `TechPreviewNoUpgrade`, which is not a promise of supported Tech Preview
upgrades; release upgrade coverage is added before GA.

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
CAO and CPO must generate `trustedCAFilePath` only for webhook images that
understand it. Console Operator must likewise pair its generated CA
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
the contents of referenced ConfigMaps. Operators check referenced configuration
and reconcile it into operand configuration and CA mounts, reporting
configuration/synchronization failures through their existing status mechanisms.
Endpoint requests are performed by the operands. Proxy outages or invalid trust can impair OIDC
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
  `openshift-oauth-apiserver` and `openshift-console`. After a proxy CA rotation,
  the operand logs are the only place a rejected bundle appears: a healthy
  `console` cluster operator means the ConfigMap synchronized, not that Console
  loaded it. The operands log their effective proxy with credentials redacted at
  startup; use that to confirm which proxy is actually in the path.
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
