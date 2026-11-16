---
title: proxy-support-for-external-oidc-auth-stack
authors:
  - "@tchap"
  - "@wouldgo"
reviewers:
  - "@liouk" # The author of the original External OIDC EP, to review the whole EP.
  - "@everettraven" # The reviewer of the Proxy Support for the Integrated Auth Stack EP; to review the whole EP.
  - "@csrwng" # To review the HyperShift part.
  - "@jhadvig" # To review Console-related changes.
approvers:
  - "@benluddy"
api-approvers:
  - "@everettraven" # For the Console Operator changes.
  - "@csrwng" # For HyperShift API changes.
creation-date: 2026-09-10
last-updated: 2026-10-23
status: provisional
tracking-link:
  - "https://redhat.atlassian.net/browse/OCPSTRAT-3721"
see-also:
  - "/enhancements/authentication/direct-external-oidc-provider.md"
  - "/enhancements/authentication/external-oidc-as-webhook.md"
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

[External OIDC as a Webhook](external-oidc-as-webhook.md)
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
- Support proxy CA rotation without restarting Console or the webhook.
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
hosted feature gates must be enabled, and the proxy must be reachable from both
the management-cluster network, where the webhook egresses, and the
guest/worker-node network, where guest Console egresses, as described under
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
   Deployment, setting the component proxy as `HTTP_PROXY`, `HTTPS_PROXY`, and
   `NO_PROXY` environment variables on the webhook container, mounting the optional
   proxy CA, and passing the mounted bundle's path through `trustedCAFilePath`. The
   webhook egresses directly from the management cluster, as kube-apiserver did
   when it validated External OIDC tokens itself. See
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

#### Feature Gates

A short refresher on where feature gates are set and how they propagate; the
design below depends on it.

A standalone cluster has one feature set,
`featuregate.config.openshift.io/cluster`, and one payload. A hosted cluster has
two independent feature sets, neither of which enables the other: the HyperShift
operator's own, selected per management cluster at `hypershift install` time,
and the hosted cluster's `spec.configuration.featureGate.featureSet`, selected
by the tenant and covering control-plane and guest components alike. The hosted
cluster's gates reach CPO as a `FeatureGate` resource rendered into a ConfigMap
in the HCP namespace rather than through a guest API read.

Gates also decide which API fields exist, because CRD manifests are generated
per feature set and per cluster profile: writing a field whose gate is off is
silently pruned rather than rejected. The profile is not configurable — CVO
applies only the manifests annotated for the profile it was started with,
`self-managed-high-availability` on standalone and `ibm-cloud-managed` for a
hosted cluster — so a gate restricted to one profile is unavailable on the other
topology.

```text
Standalone — one cluster, one payload

  featuregate.config.openshift.io/cluster .spec.featureSet
      |      ^-- feature set chosen here
      v
  cluster release payload      profile: self-managed-high-availability
      |      ^-- fixed by how the cluster was installed, not configurable
      |--> CRD variants the API server serves (incl. the Console proxy field)
      '--> gate list seen by CAO, Console Operator, and the webhook


HyperShift — two clusters, up to two payloads, two independent feature sets

  MANAGEMENT CLUSTER
    hypershift install [--tech-preview-no-upgrade]
      |      ^-- HyperShift operator feature set chosen here
      |--> management-side HyperShift operator gate
      '--> HostedCluster/HCP CRD variant, by feature-set annotation

    HostedCluster .spec.configuration.featureGate.featureSet
      |      ^-- hosted feature set: one knob, control plane and guest alike
      |
      |--(a) featuregate-generator Job in the HCP namespace
      |        renderer image   : control-plane payload
      |        --payload-version: guest payload (spec.release)
      |        --cluster-profile: ibm-cloud-managed
      |        => ConfigMap in the HCP namespace, read by CPO
      |
      '--(b) hosted CVO in the HCP namespace applies the guest payload
               |
               v
  GUEST CLUSTER                profile: ibm-cloud-managed
      |--> CRD variants served (incl. the Console proxy field)
      '--> gate list seen by Console Operator
```

##### Gates This Proposal Requires

The feature applies when `authentication.config.openshift.io/cluster` has
`spec.type: OIDC`, with these gates enabled on the cluster whose configuration
each consumer reads:

| Consumer | Topology | Required gates |
| --- | --- | --- |
| CAO and the External OIDC webhook | standalone | `ExternalOIDC`, `ExternalOIDCAsWebhook`, `AuthenticationComponentProxy`, `AuthenticationComponentProxyExternalOIDC` |
| CPO and the External OIDC webhook | HyperShift | `ExternalOIDC`, `ExternalOIDCAsWebhook`, `AuthenticationComponentProxyExternalOIDC` |
| Console Operator | both | `ExternalOIDC`, `AuthenticationComponentProxyExternalOIDC` |

`AuthenticationComponentProxyExternalOIDC` is new. It extends component proxy
support to both paths and gates the new Console operator field. The gate is
enabled in `TechPreviewNoUpgrade` while the feature is in Tech Preview and adds
`Default` at GA. Separately, it declares no cluster-profile restriction, so the
Console field appears in the `ibm-cloud-managed` CRD variants as well as the
self-managed ones — standalone and hosted clusters gain it in the same release,
with no HyperShift-specific enablement in `openshift/api`.

`ExternalOIDCAsWebhook` selects the webhook architecture and is required for the
token-validation path even without external claim sources. This proposal depends
on it, so it must reach `Default` no later than the gate above.

`AuthenticationComponentProxy` appears only in the standalone row. It gates
CAO's use of `Authentication.spec.proxy` and is declared
[`inClusterProfile(SelfManaged)`](https://github.com/openshift/api/blob/24b2699a9b32/features/features.go#L347-L353),
so by the profile rule above it is unavailable on HyperShift. Neither hosted
consumer needs it: CPO resolves the proxy from the HostedControlPlane field, and
Console reads its own operator resource.

On HyperShift this table is necessary but not sufficient, because propagation
also depends on the management-side gate below.

##### HyperShift Feature Gating

- The new `spec.operatorConfiguration.authentication` block on HostedCluster and
  HostedControlPlane is gated by a management-side HyperShift operator feature,
  declared alongside its siblings in `hypershift-operator/featuregate` and
  enabled by the operator's own feature set rather than by any hosted-cluster
  configuration. The HyperShift operator evaluates it before propagating
  anything to the HostedControlPlane, so without it neither the webhook nor
  Console ever sees a component proxy.
- The guest `console.operator.openshift.io/cluster` proxy field is served
  wherever `AuthenticationComponentProxyExternalOIDC` is enabled, on the
  Hypershift profile as on SelfManaged, so no cluster-profile change in
  `openshift/api` is needed at any phase.

During preview the administrator must enable both HyperShift's management-side
preview feature set (`hypershift install --tech-preview-no-upgrade` upstream)
and the tenant's
`HostedCluster.spec.configuration.featureGate.featureSet: TechPreviewNoUpgrade`
([independent settings](https://hypershift.pages.dev/how-to/feature-gates/)). At
GA both gates are enabled by default and neither opt-in remains necessary.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hosted support builds on the External OIDC webhook topology supplied by the
External OIDC as a Webhook enhancement. That prerequisite owns deploying the OAuth
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
the management cluster's proxy when that field is absent, exactly as
kube-apiserver already egresses today. The hosted cluster's
`spec.configuration.proxy` is not part of that resolution: it describes egress
from the guest network, which the webhook does not use.

The webhook Pod carries no konnectivity sidecar. The running webhook egresses to
the IdP **directly from the management cluster**, applying the component proxy
through standard proxy environment variables on its container. This keeps the
connection path compatible with the one kube-apiserver used when it validated
External OIDC tokens itself, and it does not depend on worker nodes, konnectivity,
or guest DNS. That connection path is detailed under
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
    |-- Mounted on the webhook container (proxy server and TLS interception trust)
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
webhook reach the IdP from different networks on HyperShift, because the webhook runs
in the management cluster while Console runs on guest worker nodes.

On standalone, both operands egress directly through the component proxy:

```text
Standalone (single cluster)

  webhook (oauth-apiserver)    --HTTP(S)_PROXY env-------->  component proxy  -->  external IdP
  Console (openshift-console)  --auth.proxy transport----->  component proxy  -->  external IdP
```

On HyperShift, both operands also egress directly, each from the network it runs in:

```text
HyperShift

  management cluster: HCP namespace
  +-------------------------------------------+
  | oauth-apiserver pod (External OIDC mode)  |
  |   webhook container                       |
  |     HTTP(S)_PROXY = <component proxy>     |
  |     NO_PROXY      = <in-HCP + component>  |--> component proxy --> external IdP
  |     (proxy CA mounted)                    |    (resolved + reached in management net)
  +-------------------------------------------+

  guest / worker-node network
  Console (guest worker node)  --auth.proxy transport--> component proxy --> external IdP
                                                         (resolved + reached in guest net)
```

The webhook Pod has no konnectivity sidecar, and its IdP traffic does not enter the
guest network. This is the path kube-apiserver used when it validated External OIDC
tokens itself, so moving validation into the webhook does not change where that
traffic originates. It also means hosted login works as soon as the hosted control
plane is available, before any worker node joins, and keeps working if every worker
node is removed.

Consequences:

- The component proxy must be reachable and DNS-resolvable **from the
  management-cluster network**, for the webhook, and **from the guest/worker-node
  network**, for Console. One proxy reachable from both networks serves both
  consumers.
- The IdP and claim-source endpoints the webhook calls must be reachable from the
  management-cluster network, either through the component proxy or directly when
  bypassed. Endpoints reachable only from inside the guest network, such as a guest
  Service, are not reachable by the webhook, as they were not by kube-apiserver.
- The webhook's `NO_PROXY` must keep the in-HCP endpoints it already bypasses
  (kube-apiserver, etcd, and the audit webhook) so they are never sent to the
  component proxy; see [Proxy Resolution](#proxy-resolution).
- The component proxy takes precedence over the management cluster's proxy for the
  webhook pod when set. It does not change the egress of any other management-side
  component.
- Verify the webhook and guest Console independently: they egress from different
  networks, so successful webhook authentication does not prove Console
  connectivity, and vice versa. See the
  [`NO_PROXY` rules](#no_proxy-and-internal-endpoint-urls).

##### Component Inventory

Covering both topologies, the feature breaks down as follows.

New components and artifacts:

| Added | Topology | Purpose |
| --- | --- | --- |
| Proxy field on `operatorv1.Console` | both | Console's own login-proxy configuration. |
| `operatorConfiguration.authentication.proxy` on HostedCluster/HostedControlPlane | HyperShift | Single hosted entry point that fans out to webhook and Console. |
| HCCO reconciler for the guest Console proxy | HyperShift | Writes the guest Console proxy field + guest `openshift-config` CA copy, verifying the written field persisted ([Cross-Cluster Write Skew](#cross-cluster-write-skew)). |

Existing components whose behavior changes:

| Affected | Change |
| --- | --- |
| CAO | Resolves the component proxy; injects env + mounts CA on the webhook Deployment and sets `trustedCAFilePath` in its configuration (standalone). |
| oauth-apiserver (webhook) | Reads the generic outbound CA bundle through `trustedCAFilePath` for HTTPS endpoint and HTTPS proxy trust; outbound clients honor the proxy environment on both topologies. |
| CPO | Resolves the component proxy; sets the proxy env and mounts the CA on the webhook Deployment and sets `trustedCAFilePath` in its configuration (HyperShift). |
| Console Operator | Reads the Console operator proxy field; renders `auth.proxy` and mounts the CA. |
| Console | Applies `auth.proxy` to its OIDC clients only. |
| HyperShift operator | Validates the control-plane and guest release targets and the gates on both sides, blocking propagation of the proxy configuration through the HostedCluster's `ValidProxyConfiguration` condition when either is unsupported ([Hosted Support Validation](#hosted-support-validation)); copies `operatorConfiguration` HC→HCP; synchronizes the referenced CA HC→HCP. |

Configuration that must hold for it to work:

- Standalone: the proxy is set on **both** the Authentication and Console operator
  resources.
- HyperShift: the proxy is set once on the HostedCluster; it must be reachable from the
  **management-cluster network** for the webhook and from the **guest network** for
  Console; the webhook's `NO_PROXY` keeps the in-HCP endpoints; and the component proxy
  takes precedence over the management cluster's proxy for the webhook pod.
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
list. Operands do not resolve it at runtime. This applies to CAO and also to Console
Operator, which runs in the guest on HyperShift, but not to CPO: it runs on the
management cluster.

On HyperShift, the webhook runs on the management cluster and egresses from
there, so CPO resolves its proxy differently:

- When the component proxy is set, CPO uses it in full as the webhook
  container's `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY`. The bypass list is
  the component `noProxy`, the hostname defaults above, and the in-HCP endpoints
  the webhook already bypasses today (the kube-apiserver Service, the etcd
  client host, and the audit webhook Service). Nothing is inherited from the
  management cluster's proxy, per the no-inheritance rule above.
- Otherwise, CPO keeps the webhook's current behavior and applies the management
  cluster's proxy from its own process environment, the same values it already
  passes to kube-apiserver, extended with the same in-HCP endpoints. If no
  management cluster proxy is configured, the webhook connects directly.

The hosted cluster's `spec.configuration.proxy` is never used for the webhook:
it describes egress from the guest network. Bypassed destinations are dialed
directly from the management-cluster network.

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
match the endpoint's TLS certificate. On HyperShift the webhook's caller namespace
is the HCP namespace on the management cluster, so internal hostnames there
resolve against management-cluster DNS, not guest DNS, while Console's resolve
against guest DNS.

#### Operator Reconciliation and Configuration Generation

CAO reconciles the webhook Deployment and generated configuration when provider
configuration, effective proxy settings, or CA references change. For the CA
bundle, it synchronizes and mounts the referenced ConfigMap and sets
`trustedCAFilePath` to the mounted file's path. CAO does not contact issuer or
claim-source endpoints before generating the OAuth API server configuration.
The OAuth API server loads the bundle and performs outbound discovery, JWKS, and
claim-source requests using the configured proxy and trust.

CPO performs the corresponding environment, configuration, and mount
reconciliation for hosted control planes. Endpoint connectivity is exercised by
the running webhook from the management-cluster network, as described under
[Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths).

CAO and CPO generate the webhook's authentication configuration through the same
shared library-go generator. Adding `trustedCAFilePath` is therefore a single
change in that generator rather than two parallel ones, even though each operator
resolves the proxy and mounts the bundle itself.

#### OAuth API Server Deployment and Trust

Both operators supply the proxy to the webhook the same way; only the operator and
the source of the fallback differ by topology (see
[Proxy Resolution](#proxy-resolution)):

- **Standalone.** CAO supplies the effective `HTTP_PROXY`, `HTTPS_PROXY`, and
  `NO_PROXY` values directly to the External OIDC container (including when
  `trustedCA` is omitted or the cluster-wide fallback applies). When `trustedCA` is
  set, CAO synchronizes the referenced ConfigMap from `openshift-config` into
  `openshift-oauth-apiserver` and mounts it read-only; the generated webhook
  configuration references the mounted bundle through `trustedCAFilePath`.
- **HyperShift.** CPO supplies the effective `HTTP_PROXY`, `HTTPS_PROXY`, and
  `NO_PROXY` values directly to the External OIDC container, including when the
  management cluster proxy fallback applies. When `trustedCA` is set, CPO mounts the
  HyperShift operator's copy in the HCP namespace read-only and references it
  through `trustedCAFilePath` in the generated webhook configuration. The webhook
  Pod has no konnectivity sidecar, so the webhook itself connects to the component
  proxy and validates both the proxy's own certificate and, under TLS
  interception, the replacement endpoint certificates, exactly as on standalone.

The webhook's existing HTTP clients already honor proxy environment variables,
so proxy URLs do not require new fields in its generated configuration file.

`trustedCAFilePath` is a generic outbound trust input: its CA bundle is trusted
for outbound HTTPS endpoint requests, whether direct or proxied, and connections
to HTTPS proxies. It can contain endpoint CAs, proxy server CAs, and interception
CAs. This enhancement populates it with the component proxy CA bundle. Issuer and
external-source CA configuration continue to describe trust for those endpoints;
the generic bundle supplies additional trust alongside that configuration.

##### TLS Connections and Certificate Validation

For proxied HTTPS endpoint requests, on either topology, the webhook connects to
the external proxy and then establishes TLS to the endpoint through the CONNECT
tunnel. If the external proxy intercepts TLS, it presents a replacement endpoint
certificate to the webhook and establishes a separate TLS connection to the
endpoint.

The term "proxy CA" covers two distinct roles:

- A **proxy server CA** signs the proxy's own certificate, for example for
  `proxy.example.com`. Whichever component opens the connection to an HTTPS proxy
  needs this trust: the webhook itself, on both topologies.
- An **interception CA** signs replacement endpoint certificates, for example for
  `idp.example.com`. The webhook accepts these only if their certificate chain
  leads to a CA it trusts. The proxy must independently validate the real
  endpoint's certificate; the webhook does not see that original chain.

These roles may use the same CA or different CAs. An HTTP proxy can use an
interception CA even though its own connection has no TLS server certificate.

| External proxy behavior | Proxy connection | Endpoint connection |
| --- | --- | --- |
| HTTP proxy, no TLS interception | No certificate. | Endpoint certificate using normal endpoint trust. |
| HTTPS proxy, no TLS interception | Proxy certificate using the proxy server CA. | Endpoint certificate using normal endpoint trust. |
| HTTP proxy with TLS interception | No certificate. | Replacement endpoint certificate using the proxy's interception CA. |
| HTTPS proxy with TLS interception | Proxy certificate using the proxy server CA. | Replacement endpoint certificate using the proxy's interception CA. |

HTTP versus HTTPS here refers to the proxy URL scheme, not the endpoint scheme or
the `httpsProxy` field name. If the required CAs are not already trusted, the
`trustedCA` bundle must contain their public certificates.
The webhook validates both connections, so both roles must be covered by the one
bundle it loads through `trustedCAFilePath`.

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

#### Proxy Configuration Validation

The [integrated authentication proxy enhancement](proxy-support-for-integrated-auth-stack.md)
extended CAO's proxy configuration controller to check, through the effective
proxy, that the OAuth route and the configured identity providers are reachable.
That controller is skipped once External OIDC is in use, and Console Operator has
no equivalent, so neither operator can report a misconfigured proxy for this
feature; the failure surfaces only as failed authentication.

The controller and its `noProxy` matching therefore move into library-go as a
reusable proxy configuration checker, and both operators are refactored to use
it. The checker takes the effective proxy, the CA bundles to trust, and the
endpoints to probe, and keeps the existing semantics: proxy-level failures, such
as a refused connection to the proxy, a failed TLS handshake with it, or an
unreadable CA bundle, set the operator `Degraded`, while an unreachable endpoint
behind a working proxy emits a Warning event, since an external IdP can be down
for reasons unrelated to the proxy. Endpoint probes run when the effective
configuration changes rather than on every resync.

- CAO keeps its integrated OAuth checks and, under External OIDC, probes each
  provider's discovery endpoint through the webhook's effective proxy and trust.
- Console Operator probes the discovery endpoint of the issuer its OIDC client
  uses, through Console's effective `auth.proxy` settings and trust.

The checks run from the operator Pods, not the operands. On standalone both
operators share the control-plane network with their operands, and on
HyperShift Console Operator shares the guest network with Console, so a failed
check is a reliable signal; a passing check does not prove every endpoint
reachable from every replica. The hosted webhook is not covered. HyperShift has
no proxy reachability checks for any component today — its proxy validation
covers only the CA bundle, through `ValidProxyConfiguration`. CPO shares the
management-cluster network with the webhook, so such a check could be added
later, but this proposal does not add one; hosted webhook failures remain
visible in its logs, as described under
[Diagnosing Hosted Clusters](#diagnosing-hosted-clusters).

#### Configuration Updates and CA Reload

Proxy settings and CA changes follow this update contract:

| Change | Operand behavior |
| --- | --- |
| Proxy settings, including bypass rules, or CA references/mounts | Roll out the affected Deployment. |
| Contents of an already-referenced proxy CA bundle: Console or the webhook, on either topology | Reload trust without a rollout. |
| Component proxy removal or gate disablement | Reconcile away the component settings and dedicated CA input, restoring the fallback proxy (the cluster-wide proxy on standalone, the management cluster's proxy for the hosted webhook) or direct connectivity. |

CAO and Console Operator synchronize source CA changes to the mounted copies;
the [hosted distribution path](#hypershift--hosted-control-planes) adds HyperShift
operator and HCCO synchronization. CAO/CPO maintain the CA mount and
`trustedCAFilePath` in the generated webhook configuration. CAO, CPO, and
Console Operator publish a newly introduced CA path only after the CA mount has
rolled out to every replica, as described under
[Asynchronous Reconciliation Hazards](#asynchronous-reconciliation-hazards).

Console watches its mounted proxy CA file; the webhook watches the generic
outbound CA bundle referenced by `trustedCAFilePath`, on both topologies. They apply valid
trust updates atomically to the clients using those bundles without restarting.
New TLS connections must use the updated trust. Invalid updates are reported
while retaining the last successfully loaded trust bundle; certificate
verification must not be disabled. By reported we mean logging reload errors,
because not much more can be trivially done in the operand. The operator should
perform the checks necessary to ensure the ConfigMap is valid before mounting it
for the operator and degrade on any issues found so that the issue becomes visible.

On HyperShift, CPO mounts the CA ConfigMap as a volume rather than through
`subPath`, so that content updates reach running webhook Pods, and excludes it
from the Pod template's config hash, as it already does for the webhook's
authentication configuration, so a content change does not roll out the
Deployment.

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
Administrators own proxy availability and capacity. Failures must be observable
— the [proxy configuration checks](#proxy-configuration-validation) report
proxy-level misconfiguration through operator conditions —
and requests must not silently bypass a configured proxy on failure.
On HyperShift, the webhook and Console egress from different networks, so the
component proxy is a dependency of both the management-cluster and the guest
network. A proxy reachable only from the guest network leaves hosted token
validation failing while Console login may still work, and vice versa; this must
be documented so administrators verify reachability from both networks. The
webhook does not depend on konnectivity, guest DNS, or worker nodes. When the
component proxy is configured, the management cluster's own proxy is never
substituted for the tenant's.

**Partial application across management and guest clusters.**
The hosted webhook and guest Console reconcile asynchronously and can temporarily
use different proxy settings or CA bundles. Configuration and CA synchronization
failures must be reported through the responsible operators' existing status
conditions. Unsupported version combinations follow the
[Version Skew Strategy](#version-skew-strategy). For routine CA rotation,
distribute a bundle containing both old and new CAs and wait for both the hosted
webhook and Console to reload it before switching proxy certificates. Remove the old CA only after verifying both authentication paths.

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
to the webhook's environment on both topologies and to Console's generated
ConfigMap, not protected by a Secret reference. Restrict access to these
resources and Pod specifications, and redact credentials from diagnostics. This
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
single-replica deployments, including SNO. Plan these changes with recovery
access available.

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

### Hosted Cluster-Wide Proxy as the Hosted Webhook Fallback

On HyperShift, when no component proxy is configured, the webhook could fall back to
the hosted cluster's `spec.configuration.proxy`, mirroring standalone, where the
cluster-wide proxy is the fallback. The proposal instead falls back to the management
cluster's proxy, for the following reasons:

- **Continuity with kube-apiserver.** Before the webhook architecture,
  kube-apiserver retrieved discovery and JWKS from the management cluster through
  the management cluster's proxy. Keeping that fallback means a hosted cluster
  that moves token validation to the webhook without configuring a component
  proxy reaches its IdP exactly as before. Switching to a different proxy at that
  point could break IdP reachability during the migration.
- **Network reachability.** The hosted cluster-wide proxy describes egress from
  the guest network and is commonly reachable only from there. The webhook runs
  and egresses on the management cluster, so it cannot assume that proxy is
  reachable.
- **A single rule for direct egress.** Without a component proxy, the webhook
  follows the management cluster's network configuration, as other control-plane
  components that dial out directly do. With one, the component proxy replaces
  that configuration in full, with no field-level mixing between the two.

The trade-off is that the management cluster's proxy is controlled by the
management cluster administrator, not the hosted cluster administrator. A hosted
cluster whose IdP is not reachable through it must configure the component proxy,
which is the case this enhancement exists for and no worse than kube-apiserver's
previous behavior.

## Open Questions [optional]

- Agree the two minimum supported releases used by
  [Hosted Support Validation](#hosted-support-validation) — the control-plane
  floor with the HyperShift team, and the guest floor with the Console team —
  before the feature is introduced.

## Test Plan

Tests are grouped by authentication path. Each section starts with the
end-to-end flows, which span an operator and its operand and are the primary
coverage, and then lists the component-scoped tests underneath so each owning
party sees its own scope in one place. Unit tests cover pure functions with
easily asserted inputs and outputs; integration tests run the operator sync loops
against fake clientsets and assert the resulting objects. The operands
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
- A configuration the running webhook cannot parse does not tear down a working
  authenticator: the replica keeps serving token validation with its last
  successfully loaded configuration, as
  [Asynchronous Reconciliation Hazards](#asynchronous-reconciliation-hazards)
  assumes.
- A proxy URL carrying credentials still authenticates, and the webhook's
  effective-proxy log line has the userinfo stripped.
- An unreachable proxy URL takes the authentication cluster operator Degraded
  through the [proxy configuration checks](#proxy-configuration-validation), and
  an unreachable issuer behind a working proxy emits a Warning event instead.
- A token referencing [distributed claims](https://openid.net/specs/openid-connect-core-1_0.html#AggregatedDistributedClaims)
  authenticates with the resolved claim, with the claim endpoint request and the
  discovery and JWKS requests verifying its response observed on the proxy.
- With `ExternalOIDCExternalClaimsSourcing` enabled, retrieval from a configured
  external claim source goes through the proxy, and for a source using client
  credentials, so does the access-token request to its token endpoint.

**Cluster Authentication Operator — unit tests.**

- Proxy resolution: the component proxy used in full when configured and gated,
  cluster-wide fallback otherwise, and no per-field inheritance.
- Generated webhook configuration and container environment: `trustedCAFilePath`
  set only when `trustedCA` is configured, the resolved `HTTP_PROXY`,
  `HTTPS_PROXY`, and `NO_PROXY` values including the implicit `noProxy` defaults
  and their trailing-dot forms, and removal of all of it when the component proxy
  is removed.
- [Proxy configuration checks](#proxy-configuration-validation) under External
  OIDC: the providers' discovery endpoints are probed through the webhook's
  effective proxy and trust, and the integrated OAuth checks are unchanged.

**library-go — unit tests.**

- The shared [proxy configuration checker](#proxy-configuration-validation):
  `noProxy` matching, the mapping of proxy-level failures to `Degraded` and of
  unreachable endpoints to Warning events, and probing only when the effective
  configuration changes.

**Cluster Authentication Operator — integration tests.**

- Synchronizing the referenced `trustedCA` ConfigMap from `openshift-config` into
  `openshift-oauth-apiserver`, mounting it, and cleaning up when the proxy is
  removed.
- The [update contract](#configuration-updates-and-ca-reload): proxy and CA
  reference changes alter the Deployment Pod template; CA content changes only
  replace the mounted file; and disabling the gate reconciles the component
  settings and CA mount away, like removing the proxy does.
- [Staged CA introduction](#asynchronous-reconciliation-hazards): the generated
  webhook configuration gains `trustedCAFilePath` only once every replica runs a
  template with the CA mount, and loses it before the mount is removed.

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
- A proxy URL carrying credentials still allows login, and Console's
  effective-proxy log line has the userinfo stripped.
- An unreachable proxy URL takes the console cluster operator Degraded through
  the [proxy configuration checks](#proxy-configuration-validation), and an
  unreachable issuer behind a working proxy emits a Warning event instead.
- Token refresh: with a short access-token lifetime, Console refreshes the
  session's tokens through the proxy, the refresh request is observed on it, and
  the session stays valid without a new login.

**Console Operator — unit tests.**

- Resolution: the field applies only to `Type: OIDC`, and integrated OAuth
  clusters are unaffected.
- `console-config.yaml` generation: the `auth.proxy` block with resolved
  `noProxy` and `trustedCAFilePath`, and removal when the proxy is removed.
- Console's global `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` environment
  variables keep their cluster-wide values while `auth.proxy` is present, so
  non-OIDC clients retain their existing routing
  ([Proxy Resolution](#proxy-resolution)).
- [Proxy configuration checks](#proxy-configuration-validation): the Console
  OIDC client's issuer discovery endpoint is probed through Console's effective
  `auth.proxy` settings and trust.

**Console Operator — integration tests.**

- Synchronizing the referenced `trustedCA` ConfigMap into `openshift-console`,
  mounting it separately from issuer trust, and cleaning up on removal.
- The [update contract](#configuration-updates-and-ca-reload): proxy and CA
  reference changes alter the Console Deployment; CA content changes do not; and
  disabling the gate reconciles the `auth.proxy` block and CA mount away.
- [Staged CA introduction](#asynchronous-reconciliation-hazards): the CA mount
  rolls out first while the published configuration, including the whole
  `auth.proxy` block, stays unchanged; the configuration is published only once
  every replica runs the mounted template; an interrupted reconciliation resumes
  without publishing early; and removal publishes the configuration without the
  path before dropping the mount.

### Hosted Clusters

Hosted coverage lands with the hosted implementation.

**End-to-end flows** spanning the HyperShift operator, CPO, HCCO, and both
operands. Rerun both sets of flows above on a hosted cluster, and additionally
verify:

- Configuration and CA propagation through the full chain into the HCP and guest
  `openshift-config` and `openshift-console` namespaces.
- Webhook egress from the management cluster: the webhook's provider requests are
  observed on the component proxy arriving from the management-cluster network,
  in-HCP endpoints (kube-apiserver, etcd, the audit webhook) are never sent to the
  proxy, and the webhook Pod carries no konnectivity sidecar.
- Login without workers: with the hosted control plane up and no worker nodes
  joined, token validation through the component proxy succeeds.
- Fallback: with no component proxy, the webhook uses the management cluster's
  proxy, matching kube-apiserver, and ignores the hosted cluster's
  `spec.configuration.proxy`.
- An HTTPS proxy and an intercepting proxy: with the proxy server CA and the
  interception CA in `trustedCA`, token validation succeeds.
- A proxy CA content change is picked up by both the webhook and guest Console
  without a rollout.
- Isolation between two hosted clusters, with the guest cluster-wide Proxy and
  NodePool rollout hashes unchanged.
- The [version-skew guarantees](#version-skew-strategy) with separate
  `spec.controlPlaneRelease` and `spec.release` selections: a guest payload whose
  resolved gates do not enable the feature blocks the entire configuration,
  leaving both the webhook and Console on their previous proxy behavior rather
  than proxying the webhook alone.

**HyperShift operator — unit tests.**

- Validation of the optional HostedCluster and HostedControlPlane fields,
  including feature-gated admission and serialization compatibility.
- Propagation of the HostedCluster block to the HostedControlPlane.
- [Release validation](#hosted-support-validation): a control-plane target below
  the minimum, a guest target below the minimum, and a hosted feature set that
  does not enable the gate each set the HostedCluster's
  `ValidProxyConfiguration` condition to `False` with a message naming the
  failing target, leave the HostedControlPlane proxy block unwritten, and leave
  `ValidConfiguration` and the rest of HostedCluster reconciliation unaffected.
- `ValidProxyConfiguration` aggregates both checks: an invalid cluster-wide
  proxy CA bundle and an unsupported component proxy each set it `False` on
  their own, and when both fail the condition reports both.
- An unresolvable or unparseable release image sets the condition `Unknown` and
  retries rather than blocking permanently or propagating.
- A prerelease build of the minimum supported release — an engineering or
  release candidate — passes both floors rather than being rejected for sorting
  below its own final version.
- Install-level: an operator installed without the preview feature set serves a
  HostedCluster CRD variant without the `operatorConfiguration.authentication`
  block, so a HostedCluster carrying a proxy block is pruned on write. This is
  the one hosted failure with no in-product detection
  ([Hosted Support Validation](#hosted-support-validation)); the test pins the
  CRD variant's contents rather than the pruning behavior itself.

**CPO and HCCO — unit and integration tests.**

- The webhook container's `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` derived from
  the HCP block, including that the component proxy takes precedence over the
  management cluster's proxy without inheriting from it, that the management
  cluster's proxy is used when the component block is absent, that the hosted
  cluster's `spec.configuration.proxy` is never used, and that `NO_PROXY` always
  carries the in-HCP endpoints and the hostname defaults.
- CPO reconciles the webhook configuration and the CA mount on the webhook
  container; removal cleans all of it up.
- [Staged CA introduction](#asynchronous-reconciliation-hazards): CPO adds the
  webhook's CA mount only once the HyperShift operator's copy lands in the HCP
  namespace, publishes `trustedCAFilePath` only once every webhook replica runs a
  template with the mount, and removes it before the mount.
- HCCO writes only the proxy field on the CVO-seeded guest
  `console.operator.openshift.io/cluster` resource, plus its CA copy, using
  destination-local references.
- HCCO read-back: against a guest whose CRD prunes the proxy field, HCCO reports
  through its own `ConfigOperatorReconciliationSucceeded` condition rather than
  treating the write as successful, and does not touch
  `ValidHostedControlPlaneConfiguration`, which CPO owns.
- The proxy CA ConfigMap is excluded from the webhook Pod template's config hash,
  so a CA bundle content change does not trigger a rollout.

### Lockout and Recovery

The [primary mitigation](#risks-and-mitigations) for a misconfigured proxy is
independent administrative access, so the recovery path is exercised rather than
assumed. Using the suite's break-glass certificate kubeconfig:

- A proxy URL that cannot be reached breaks token validation and Console login,
  while client-certificate and service-account authentication keep working.
- Correcting the proxy at its source restores both paths, and removing the
  component proxy restores the previous cluster-wide or direct behavior.
- On hosted clusters the same flow runs against the HostedCluster and its source
  CA ConfigMap on the management cluster, confirming that the guest-side copies
  reconcile back without editing generated resources.

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
and hosted topologies must be covered.

### Dev Preview -> Tech Preview

The feature is proposed to ship directly as Tech Preview behind
`AuthenticationComponentProxyExternalOIDC`, with the prerequisite gates already
enabled in `TechPreviewNoUpgrade`.

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

On standalone clusters, enable component-proxy use only after the participating
operators and operands meet the
[version-skew requirements](#version-skew-strategy); the feature gate and this
sequencing are the only controls. On HyperShift the HyperShift operator
enforces the equivalent check itself against both release targets and blocks an
unsupported configuration, as described under
[Hosted Support Validation](#hosted-support-validation), so upgrading
management-side API/operator support before using the new field is advisory
rather than load-bearing. Operators reconcile compatible images and generated
configuration through normal rollouts; no authentication-data migration is
introduced.

On both topologies, configuring the component proxy during an upgrade can leave
consumers temporarily on different paths: replicas that predate the feature keep
their previous routing until replaced, and on HyperShift the control plane can
apply the configuration before the guest has upgraded (see
[Hosted Support Validation](#hosted-support-validation)). Retain the previous
route — the cluster-wide proxy or the existing egress allowance — until the
upgrade has completed and both webhook token authentication and a fresh Console
login have been verified through the component proxy.

`TechPreviewNoUpgrade` retains its existing upgrade restrictions. This enhancement
does not introduce a supported downgrade path. For development rollback testing,
first establish a working cluster-wide proxy or direct path, remove the component
configuration, and let the operators reconcile compatible operand configuration
before reverting to versions that do not support it. On HyperShift, make the
change on the HostedCluster and retain the independent administrative access
described in [Support Procedures](#support-procedures).

## Version Skew Strategy

Unlike the preceding proposal, this change spans multiple operators and operands.
New operands must continue to accept configuration that omits the new inputs,
retaining Console's existing environment-based fallback when `auth.proxy` is
absent. Mixed old and new replicas during rollout must retain a working
configuration until replaced.

### Co-Versioning Guarantees

No operator performs a per-image capability check before generating the new
configuration. What prevents an operator from configuring an operand that cannot
understand it is co-versioning plus the [feature gates](#feature-gates): each
operator ships in the same release payload as the operand it configures.

| Operator | Operand | Shared payload |
| --- | --- | --- |
| CAO | webhook (oauth-apiserver) | the standalone cluster's release |
| CPO | webhook | `spec.controlPlaneRelease`, else `spec.release` |
| HCCO | guest Console resource | `spec.controlPlaneRelease`, else `spec.release` |
| Console Operator | Console | `spec.release` |

As the [External Claims Sourcing enhancement](external-oidc-additional-identity-information-sources.md)
establishes for the same components, the CPO minor version always corresponds to
the OCP minor version it operates on, and the only supported skew is at the
Z-release level. The feature gate alone is therefore sufficient to stop an
operator generating configuration the deployed operand cannot parse, and no
version-aware generation logic is needed inside CAO, CPO, or Console Operator.
The residual exposure is a rollout window, not a supported configuration.

The two operands behave differently inside that window, so the hazard is
one-sided:

- **The webhook decodes strictly**, so a replica predating `trustedCAFilePath`
  rejects a configuration containing it rather than ignoring the field. The
  consequences are covered under
  [Asynchronous Reconciliation Hazards](#asynchronous-reconciliation-hazards).
- **Console tolerates unknown fields.** Console decodes `console-config.yaml`
  with a non-strict YAML unmarshaller
  ([`pkg/serverconfig/config.go`](https://github.com/openshift/console/blob/master/pkg/serverconfig/config.go)),
  so an image that predates `auth.proxy` ignores the block and starts normally.
  Console Operator therefore needs no image pairing logic, and the requirement
  that mixed replicas retain a working configuration holds structurally rather
  than by mitigation. That is configuration compatibility, not authentication
  availability: such a replica still reaches the IdP through its previous
  routing — the cluster-wide proxy or direct connectivity — so logins it serves
  fail if the IdP is reachable only through the component proxy, until the
  rollout replaces it. Because that routing is what was in effect before the
  component proxy was configured, the window regresses nothing on its own; it
  becomes an outage only if the previous route is removed while old replicas
  remain, which the [upgrade guidance](#upgrade--downgrade-strategy) advises
  against.

### Asynchronous Reconciliation Hazards

Two hazards remain that co-versioning does not resolve, because the generated
configuration, the CA mount, and the running replica are reconciled by three
independent, asynchronous paths:

- **Schema skew.** The webhook decodes its configuration strictly, so a replica
  running an image that predates `trustedCAFilePath` rejects a configuration
  containing it outright rather than ignoring the unrecognized field. Because
  CAO/CPO and the webhook image ship in the same release payload, this window
  exists only transiently during a rollout. It is also cushioned by the
  webhook's reload behavior: a failed reload keeps the last successfully
  loaded authenticator rather than tearing it down, so an already-running
  replica tolerates a configuration it cannot yet parse. A replica performing
  its first configuration load has no prior authenticator to fall back on, so
  the only residual exposure is a replica created from a stale, pre-upgrade
  template racing the operand rollout that is already replacing it.
- **Resource-ordering skew.** Even once the image understands
  `trustedCAFilePath`, the operator must not publish the path while any Pod
  lacks the CA mount. ConfigMap synchronization, the Deployment's volume mount,
  and generated-configuration distribution are reconciled independently, and
  both operands load the referenced bundle when they start: a Pod from the
  previous template that restarts after the path is published reads the new
  configuration, fails to load the missing file, and crash-loops until the
  rollout replaces it. For the webhook, which every External OIDC token
  validation depends on, that is lost capacity in the authentication path.

  CAO and Console Operator therefore roll out a newly introduced proxy CA in two
  stages:

  1. Once the CA copy exists in the operand's namespace, the operator adds the
     CA volume and mount to the Deployment while keeping the previously
     published configuration, and waits until every replica runs a template
     with the mount: the Deployment's observed generation is current and all
     replicas are updated. It waits on the rollout, not on authentication
     health, since the configuration has not changed yet.
  2. Only then does it publish the configuration referencing the CA. The
     webhook reloads its configuration in place, so this stage needs no
     rollout. Console reads its configuration only at startup, so the new
     configuration rolls the Console Deployment a second time.

  Console Operator holds back the entire `auth.proxy` change during the first
  stage, not only `trustedCAFilePath`: publishing the proxy URL alone would point
  new Pods at a proxy whose certificate they cannot yet verify. The staging
  applies only when a CA is being introduced; once the mount exists, later
  changes, including CA content updates, follow the normal
  [update contract](#configuration-updates-and-ca-reload). Removal needs no
  staging: the configuration stops referencing the path first, after which the
  mount is no longer read and can be removed.

  CPO applies the same two stages on HyperShift to the webhook's
  `trustedCAFilePath`. Its first stage starts one hop later than CAO's: the
  proxy CA ConfigMap in the HCP namespace is written by the HyperShift operator
  copying the bundle HC→HCP, so CPO waits on that synchronization before adding
  the mount (see the [CA distribution path](#hypershift--hosted-control-planes)).
  CPO excludes the webhook configuration ConfigMap from the Pod template's
  config hash, so as on standalone the second stage needs no rollout and the
  webhook reloads the published configuration in place.

### Hosted Support Validation

HyperShift's management cluster OpenShift version and HyperShift Operator version
are distinct from the releases supplying the hosted cluster's components.
By default, `HostedCluster.spec.release` supplies both hosted control-plane and
guest components. When set, `spec.controlPlaneRelease` selects a separate payload
for management-side components, including CPO, HCCO, and OAuth API server; Console
Operator and Console continue to come from `spec.release` in the guest cluster.

Feature sets and release payloads are orthogonal, and the distinction matters
for the rest of this section. A hosted cluster has exactly one feature set,
`spec.configuration.featureGate.featureSet`, covering control-plane and guest
components alike, so the [gates](#feature-gates) the webhook and Console require
cannot be configured apart. Releases are chosen per target and nothing compares
them. The feature set decides whether a gate is on; the payload decides whether
the field and the code behind it exist at all, and a feature set can only enable
gates the payload interpreting it knows about. Feature-set parity is therefore
structural, while release parity is not — which is why the validation below is
built on release floors and treats gates as the secondary check.

Gate agreement across the split is not automatic either. The hosted cluster's
effective gate list is rendered management-side by a job in the HCP namespace
and published as a ConfigMap that CPO reads, and that render mixes payloads: the
renderer image comes from the control-plane payload, while the payload version
it evaluates comes from `spec.release` (it also renders for the Hypershift
cluster profile, which is why the SelfManaged-only
`AuthenticationComponentProxy` is unavailable there). The guest resolves its own
list independently from `spec.release` through its CVO and the per-feature-set
CRD manifests. With `spec.controlPlaneRelease` set, the two sides can therefore
disagree about whether a gate exists at all — a newer control-plane payload can
report this proposal's gate enabled to CPO while the guest payload has never
heard of it.

Within supported HyperShift version combinations, newer Console components with
older authentication components, or the reverse, must preserve existing behavior.
Skew can prevent use of the shared component proxy but must not itself break
authentication or degrade a healthy cluster. Until all consumers support the
feature, retain the existing cluster-wide proxy or direct connectivity. A
configuration blocked by the validation below is not a Degraded cluster: the
hosted cluster keeps its previous proxy behavior.

Acceptance of the new HostedCluster field or enablement of the management-side
gate alone does not establish end-to-end support, because the two payloads are
selected independently and `spec.controlPlaneRelease` is unconstrained: the API
performs no comparison against `spec.release`. Each payload supplies different
consumers of this feature:

| Target | Resolved from | Supplies |
| --- | --- | --- |
| Control plane | `spec.controlPlaneRelease.image`, else `spec.release.image` | CPO, HCCO, and the OAuth API server |
| Guest | `spec.release.image` | Console Operator, Console, and the `console.operator.openshift.io` CRD that must serve the proxy field |

Before propagating `operatorConfiguration.authentication.proxy` to the
HostedControlPlane, the HyperShift operator validates both targets against a
minimum supported release, and confirms the gates permit the configuration on
both sides: the management-side gate, and the hosted feature set, since the
guest Console proxy field is only served while the gate is enabled (see
[HyperShift Feature Gating](#hypershift-feature-gating)). The gate half of that
check is load-bearing only while the gates remain preview-scoped; once both reach
their default feature sets at GA it is satisfied on every cluster and the release
floors become the operative check. Unsupported releases
or gates set the HostedCluster's existing `ValidProxyConfiguration` condition to
`False` with an `AuthenticationProxyUnsupported` reason naming the target that
fell short, and **block propagation** of
`operatorConfiguration.authentication.proxy`, so no part of the proxy
configuration is applied. The rest of the HostedCluster continues to reconcile:
unlike `ValidConfiguration`, a `False` `ValidProxyConfiguration` does not halt
HostedCluster reconciliation, so an unsupported proxy configuration withholds
only itself rather than freezing unrelated changes. Release metadata lookup or
version parsing failures set the condition `Unknown` and retry rather than
reporting the payload unsupported.

`ValidProxyConfiguration` today reports only the validity of the cluster-wide
proxy CA bundle (`spec.configuration.proxy.trustedCA`, reason
`ProxyCABundleInvalid`). This proposal extends it to cover the component proxy as
well: the condition is `True` only when both checks pass, and when either fails
it carries the reason and message of the failing check — both, if both fail — so
neither result masks the other.

A third version participates but cannot be validated here: the HyperShift
operator's own. It is installed independently of both payloads, and
`hypershift install` selects which CRD variant to apply by feature-set
annotation. An operator predating the feature — or a current one installed
without the preview feature set while the field is still preview-scoped — therefore
serves a HostedCluster CRD without `operatorConfiguration.authentication`, and
the API server prunes the administrator's proxy block on write. The apply
succeeds, `ValidProxyConfiguration` stays `True`, and the configuration is simply
absent, because the component that would report the loss is the one that is too
old or ungated. No condition can cover this case. It is the inverse of the
guest-side pruning in [Cross-Cluster Write Skew](#cross-cluster-write-skew),
which HCCO can detect precisely because a newer component performs the write;
here the administrator or support must confirm the field persisted on the
HostedCluster, as [Diagnosing Hosted Clusters](#diagnosing-hosted-clusters)
describes.

This follows HyperShift's established pattern for feature-level support
checks — a minimum release constant evaluated during reconciliation and
surfaced through an existing HostedCluster condition — and the validation the
[External Claims Sourcing enhancement](external-oidc-additional-identity-information-sources.md)
defines for the control-plane target, extended to the guest target because this
feature has a guest-side consumer that one does not. It reports through the
proxy-specific `ValidProxyConfiguration` rather than the general
`ValidConfiguration` so that a proxy-only problem blocks only the proxy
configuration, and it introduces no new condition type.

Each floor is a constant in the HyperShift operator. The version is not read
from a field: `spec.release.image` and `spec.controlPlaneRelease.image` are
pullspecs, so the operator resolves each through the release provider — a
cached registry lookup — and parses the resulting release version before
comparing. When `spec.controlPlaneRelease` is unset both targets resolve to the
same image, so the second check adds no lookup in the default topology. The
comparison must ignore the prerelease segment, so that an engineering or
release candidate of the release introducing support is not rejected for
sorting below its own final version. The two minimum releases are determined
before the feature is introduced.

The two targets fail differently, which is why both are checked. A guest
payload below the floor would otherwise leave the webhook proxied while Console
falls back to the cluster-wide proxy or direct connectivity — a split that
persists until the payload is updated. A control-plane payload below the floor
configures neither consumer, since CPO and HCCO both come from it, so the
result is inert rather than half-applied; but nothing inside that payload can
report it, because neither operator knows the feature exists. The guest floor
additionally covers the divergent gate resolution described above, which no
feature-set check can: a control-plane payload that knows the gate can report it
enabled to CPO regardless of what the guest payload understands. The HyperShift
operator is versioned independently of both payloads and is therefore the only
component able to detect any of these cases before anything is applied.

Blocking on the guest target means a hosted cluster whose control plane
supports the feature but whose guest payload does not gets nothing, including
the webhook proxying that would have worked on its own. That is deliberate: a
half-applied proxy leaves token validation working while Console login fails in
exactly the restricted networks this feature targets, and the
[graduation criteria](#tech-preview---ga) already treat webhook-only support as
insufficient.

The validation evaluates the *desired* releases named by the pullspecs, not the
versions currently running, and does not wait for an upgrade to complete.
Starting an upgrade updates `spec.release` immediately, so a proxy
configuration set during an upgrade into the first supporting release passes
validation and propagates while the rollout is still in progress. The control
plane converges first: CPO proxies the webhook and HCCO writes the guest Console
field, while the guest CVO may still be running the previous Console Operator,
CRD, and Console pods. Until the guest finishes upgrading, the configuration is
pruned or ignored on the guest side as described under
[Cross-Cluster Write Skew](#cross-cluster-write-skew) and
[Co-Versioning Guarantees](#co-versioning-guarantees), and Console keeps its
previous routing. This window is bounded by the rollout converging rather than
by the validation, which only rules out targets that will never support the
feature; a stalled guest upgrade extends it. Requiring the last completed
version in `status.version.history` to meet the floor as well would close it,
at the cost of deferring the configuration for the whole upgrade; since the
window regresses nothing while the previous route is retained, this proposal
documents it instead.

Responsibility therefore differs by topology. On HyperShift the HyperShift
operator enforces this, and the
[upgrade guidance](#upgrade--downgrade-strategy) is advisory. On standalone
clusters there is no equivalent gatekeeper — CAO and Console Operator co-ship
with their operands — so the feature gate and the administrator's own
sequencing remain the only controls.

### Cross-Cluster Write Skew

One skew path crosses the cluster boundary and would otherwise defeat the
guarantee above. HCCO comes from the control-plane payload but writes
`console.operator.openshift.io/cluster` in the guest cluster, whose CRD schema
comes from `spec.release`. If the guest payload predates the Console proxy
field, or — while the gate is still preview-scoped — its feature set does not
enable the gate that serves it, the API server prunes the field: HCCO's write
appears to succeed and the setting silently disappears. The resulting state —
webhook proxied, Console falling back to the cluster-wide proxy or direct
connectivity — persists for as long as the mismatch does.

Two layers address this:

- **Prevention.** The [guest release floor](#hosted-support-validation) above
  stops the configuration before anything is applied. It covers both causes,
  since a payload below the floor is also the one whose CRD will not serve the
  field. This is the guarantee.
- **Detection.** After writing the guest Console resource, HCCO re-reads it and
  confirms the proxy field persisted. That re-read must bypass HCCO's guest
  cache, which backs its default client and is filtered to objects labeled
  [`hypershift.openshift.io/managed=true`](https://github.com/openshift/hypershift/blob/e19f174168d9d4ab6cabb63f9d1bca374c7b3edc/support/labelenforcingclient/labelenforcer.go#L14-L17):
  a cached read immediately after the write can return stale content and report
  a successful write as pruned, flapping the condition below. HCCO already
  maintains an uncached client for checks of this kind. If the field did not
  persist, HCCO treats this as a reconciliation error rather than reporting
  success, surfacing it through its own
  `ConfigOperatorReconciliationSucceeded` condition on the
  HostedControlPlane. It must not report through
  `ValidHostedControlPlaneConfiguration`: CPO sets that condition on every
  reconcile from its own validation and would overwrite HCCO's result. This is a
  backstop for state that changed after validation passed — the guest payload or
  its feature set moving underneath an already propagated configuration.

Support should treat a blocked `ValidProxyConfiguration` on the HostedCluster as the
expected signal for an unsupported payload, and an HCCO read-back failure as an
indication that something changed underneath an already-validated
configuration.

HCP and guest reconciliation remain asynchronous; this proposal adds no aggregate
condition confirming proxy and CA propagation across both paths, relying instead
on the existing `ValidProxyConfiguration` and
`ConfigOperatorReconciliationSucceeded` conditions described above. No new kubelet
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
is unavailable. On HyperShift, recovery also requires independent
management-cluster access with permission to update the HostedCluster and its
source CA ConfigMap; guest-cluster credentials alone cannot repair those
authoritative inputs.

Two properties shape every procedure below. The webhook and Console are separate
paths with separate configuration, trust, and egress, so a working `oc` token
proves nothing about Console login, and vice versa. And a healthy operator means
the operator reconciled, not that the operand loaded the result: both operands
retain their last good state when a reload fails — the webhook keeps its
previously loaded authenticator ([Asynchronous Reconciliation
Hazards](#asynchronous-reconciliation-hazards)) — so working authentication does
not prove the current configuration is in effect. Confirm the operand side in
operand logs.

### Diagnosing Standalone Clusters

- Inspect `oc get clusteroperator authentication console` and the detailed
  conditions on the operator `Authentication` and `Console` resources. A missing
  or malformed `trustedCA` ConfigMap degrades the responsible operator before the
  bundle is mounted
  ([Configuration Updates and CA Reload](#configuration-updates-and-ca-reload)).
- Check events and operator logs in `openshift-authentication-operator` and
  `openshift-console-operator`, plus webhook and Console logs in
  `openshift-oauth-apiserver` and `openshift-console`. A bundle that passes
  operator validation but is rejected or incomplete at load time appears only in
  the operand logs: a healthy `console` cluster operator means the ConfigMap
  synchronized, not that Console loaded it. The operands log their effective proxy
  with credentials redacted; use that to confirm which proxy is actually in the
  path.
- Compare the configured component proxy, cluster-wide fallback, rendered operand
  configuration, CA references, and mounted CA copies. Inspect the webhook
  container's proxy environment and Console's generated `auth.proxy` block
  separately; Console's global environment still describes its cluster-wide
  fallback, not an active authentication-specific override. A generated webhook
  configuration without `trustedCAFilePath` while `trustedCA` is set means CAO is
  still rolling out the CA mount and has not published the path yet
  ([resource-ordering skew](#asynchronous-reconciliation-hazards)); that resolves
  itself once the rollout completes and is not missing trust.
- Verify the [required gates](#feature-gates) and that kube-apiserver uses the
  External OIDC webhook path.
- Check discovery, JWKS, claim-source, and token endpoints independently. Confirm
  that `NO_PROXY` matches the URL hostname, the endpoint resolves from the caller's
  network, and the correct issuer/source and proxy CAs are available. Distinguish
  proxy connectivity errors from endpoint TLS errors and invalid tokens.

### Diagnosing Hosted Clusters

The checks above apply unchanged to the guest Console, which runs in
`openshift-console` on a worker node and is configured by Console Operator from
its own guest resource exactly as on standalone. The webhook differs: it runs in
the HCP namespace on the management cluster and egresses directly from there.

- **First confirm the proxy block is actually stored on the HostedCluster.** If
  the HyperShift operator predates the feature, or was installed without the
  preview feature set while the field is still preview-scoped, its HostedCluster
  CRD does not carry the field and the API server pruned it on write without an
  error. Every condition below then reports a cluster that simply has no
  component proxy configured ([Hosted Support
  Validation](#hosted-support-validation)).
- **Then read the HostedCluster conditions.** `ValidProxyConfiguration` set to
  `False` with an `AuthenticationProxyUnsupported` reason naming a release target
  is the expected signal for an unsupported payload: the HyperShift operator
  blocked propagation of the proxy configuration, none of it was applied, and the
  cluster keeps its previous proxy behavior while the rest of the HostedCluster
  continues to reconcile
  ([Hosted Support Validation](#hosted-support-validation)). `Unknown` means
  release lookup or version parsing failed and is being retried. Before GA, also
  confirm both gate sides — the management-side HyperShift gate and the hosted
  `featureGate.featureSet`; once the gates reach their default feature sets, the
  release floors are the only remaining constraint
  ([HyperShift Feature Gating](#hypershift-feature-gating)).
- **Read the webhook container's proxy environment as the effective proxy.** CPO
  writes the component proxy, or the management cluster's proxy when none is
  configured, as `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` on the webhook
  container. `NO_PROXY` must still cover the in-HCP endpoints. Failures reaching
  the proxy appear in the webhook logs.
- **Expect webhook egress to originate from the management-cluster network.** The
  proxy, and any bypassed IdP or claim-source endpoint, must be reachable and
  DNS-resolvable from the HCP namespace, independently of the guest network that
  Console egresses from
  ([Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths)).
  A proxy reachable only from worker nodes breaks hosted token validation while
  Console login may still work.
- **Walk the CA chain hop by hop:** the source ConfigMap in the HostedCluster's
  namespace, the HyperShift operator's copy in the HCP namespace, the HCCO copy in
  guest `openshift-config`, and Console Operator's copy in `openshift-console`
  ([distribution path](#hypershift--hosted-control-planes)). A content update that
  stalls at any hop leaves the remaining hops on the old bundle. Trust reload
  errors appear in the webhook logs, and a failed reload keeps the previously
  loaded bundle.
- **If the guest `console.operator.openshift.io/cluster` proxy field is absent
  while the HostedCluster has one set,** the guest API server pruned HCCO's write
  because the guest payload or its feature set no longer serves the field. The
  webhook is then proxied while Console falls back to the cluster-wide proxy or
  direct connectivity. HCCO reports the failed read-back through
  `ConfigOperatorReconciliationSucceeded` on the HostedControlPlane, which bubbles
  up to the HostedCluster; it deliberately does not use
  `ValidHostedControlPlaneConfiguration`, which CPO overwrites on every reconcile
  ([Cross-Cluster Write Skew](#cross-cluster-write-skew)).
- Check HyperShift operator, CPO, and HCCO logs, and check management-cluster and
  guest connectivity separately. Trace the configuration from the HostedCluster
  source; editing a generated HostedControlPlane field, guest resource, or
  Deployment is not a persistent fix.

### Recovery

Correct the proxy URL, bypass list, or referenced CA at the source. Alternatively,
remove the component proxy to restore a verified working cluster-wide proxy or
direct path; on hosted clusters the webhook then falls back to the management
cluster's proxy, or direct connectivity when none is configured, as
kube-apiserver does. On standalone
clusters the sources are `authentication.operator.openshift.io/cluster` (webhook)
and `console.operator.openshift.io/cluster` (Console login); on hosted clusters
both derive from
`HostedCluster.spec.operatorConfiguration.authentication.proxy`. Do not disable
TLS verification or remove the TokenReview webhook as a proxy workaround.
Reconciliation resumes after the configuration is fixed; verify both API token
authentication and a fresh Console login/refresh, not just cleared operator
conditions.
