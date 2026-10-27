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
last-updated: 2026-10-23
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
egress there — and, for login before the first worker node joins, also from the
management-cluster network, as described under
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
   webhook pod's existing konnectivity sidecar, not as raw proxy environment on
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
network**, via the konnectivity sidecar already present on its pod. CPO
configures that sidecar to chain to the component proxy in preference to the
hosted cluster-wide proxy. That connection path, including its behavior before
worker nodes exist, is detailed under
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
    |-- Mounted on the konnectivity sidecar (HTTPS proxy certificate trust)
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
component proxy lives. A konnectivity sidecar in the webhook pod applies
the component proxy:

```text
HyperShift

  management cluster: HCP namespace                     guest / worker-node network
  +-------------------------------------------+
  | oauth-apiserver pod (External OIDC mode)  |
  |   webhook container                       |
  |     HTTP(S)_PROXY = http://127.0.0.1:P --+|
  |   konnectivity sidecar (HTTPS mode) <----+|
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
  network** — konnectivity resolves via guest DNS and dials from a worker node — and,
  during the [day-0 window](#day-0-egress-before-worker-nodes) before the first
  worker joins, also from the **management-cluster network**.
- The sidecar's `--no-proxy` must list the in-cluster endpoints (KAS, the `token-review`
  Service, `.svc`, `.cluster.local`) so they tunnel through konnectivity directly rather
  than via the forward proxy.
- The proxy host is excluded from the sidecar's cloud-API direct-dial bypass, so that
  once the tunnel is healthy the proxy itself is reached through konnectivity (from the
  guest) rather than from the management cluster.
- The component proxy takes precedence over the hosted cluster's
  `spec.configuration.proxy` for the webhook pod when set.
- Verify the webhook and guest Console independently: successful webhook
  authentication does not prove Console connectivity, and vice versa. See the
  [`NO_PROXY` rules](#no_proxy-and-internal-endpoint-urls).

###### Day-0 Egress Before Worker Nodes

The konnectivity tunnel only has a backend for this traffic once a worker node
runs the data-plane konnectivity agent. Between the hosted control plane becoming
available and the first worker joining there is therefore a window in which
nothing can be dialed through the tunnel. The webhook's sidecar covers that window
with `--resolve-from-management-cluster-dns`: while the tunnel is unavailable it
resolves through management-cluster DNS and dials directly, reverting permanently
to the tunnel once it becomes healthy. Without it, hosted login would not work
until the first worker joined — the same reason the integrated identity-provider
path already carries this fallback.

That fallback does not bypass the component proxy. The two layers above are
ordered: the sidecar decides whether to insert the proxy hop *before* the
konnectivity dialer runs, so by dial time the address being dialed is already the
component proxy's. The fallback therefore bypasses konnectivity, not the proxy — it
dials the component proxy directly from the management-cluster network and the
proxy hop survives.

This is the only window in which webhook egress does not originate from the guest
network, and it is why the component proxy must also be reachable and
DNS-resolvable from the management-cluster network. CPO applies the same proxy
flags regardless; nothing in the configuration is conditional on the window.

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
| Konnectivity sidecar on the External-OIDC oauth-apiserver pod | Already present on the webhook pod; its proxy flags (`--https-proxy`, `--http-proxy`, `--no-proxy`) are now populated from the component proxy, and the proxy CA is mounted on it. |
| CAO | Resolves the component proxy; injects env + mounts CA on the webhook Deployment and sets `trustedCAFilePath` in its configuration (standalone). |
| oauth-apiserver (webhook) | Reads the generic outbound CA bundle through `trustedCAFilePath` for HTTPS endpoint trust, plus the proxy's own certificate on standalone only; outbound clients honor the proxy; on HyperShift egress flows through the new sidecar, which holds that proxy trust instead. |
| CPO | Builds the webhook Deployment including the konnectivity sidecar flags and both CA mounts; sets `trustedCAFilePath` in the webhook configuration (HyperShift). |
| Console Operator | Reads the Console operator proxy field; renders `auth.proxy` and mounts the CA. |
| Console | Applies `auth.proxy` to its OIDC clients only. |
| HyperShift operator | Validates the control-plane and guest release targets and the gates on both sides, blocking propagation through the HostedCluster's `ValidConfiguration` condition when either is unsupported ([Hosted Support Validation](#hosted-support-validation)); copies `operatorConfiguration` HC→HCP; synchronizes the referenced CA HC→HCP. |

Configuration that must hold for it to work:

- Standalone: the proxy is set on **both** the Authentication and Console operator
  resources.
- HyperShift: the proxy is set once on the HostedCluster; it must be reachable from the
  **guest network**, and from the **management-cluster network** for day-0 login to work
  before the first worker joins; the sidecar `--no-proxy` covers in-cluster endpoints;
  and the component proxy takes precedence over `spec.configuration.proxy` for the
  webhook pod.
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
control planes, including the konnectivity sidecar's proxy flags. Endpoint connectivity
is exercised by the running webhook through the guest network, as described under
[Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths).

CAO and CPO generate the webhook's authentication configuration through the same
shared library-go generator. Adding `trustedCAFilePath` is therefore a single
change in that generator rather than two parallel ones, even though each operator
resolves the proxy and mounts the bundle itself.

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
  konnectivity sidecar with the component proxy (`--https-proxy`,
  `--http-proxy`, `--no-proxy`), in preference to the hosted cluster's
  `spec.configuration.proxy`. When `trustedCA` is configured, CPO mounts the bundle
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

On HyperShift, the konnectivity sidecar does not hot-reload its proxy CA trust.
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
The sidecar's day-0 fallback may bypass konnectivity while the tunnel is
unavailable, but it must never bypass the component proxy: it dials that proxy
directly from the management-cluster network instead of through the tunnel, as
described under
[Day-0 Egress Before Worker Nodes](#day-0-egress-before-worker-nodes).
A hosted cluster whose proxy is reachable only from the guest network therefore
cannot authenticate until its first worker node joins, and this must be
documented rather than worked around by dropping the proxy hop. The management
cluster's own proxy is never substituted for the tenant's.

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
- Day-0 login: with the hosted control plane up and no worker nodes joined, token
  validation succeeds and the webhook's provider requests are still observed on the
  component proxy, reaching it from the management-cluster network. After the first
  worker joins, the same requests are observed arriving from the guest network.
- An intercepting proxy: with TLS interception enabled and the interception CA in
  `trustedCA`, token validation succeeds, and it fails with an actionable error
  when the webhook container's copy of the bundle is absent.
- A proxy CA content change rolls out the OAuth API server Deployment while guest
  Console reloads without a rollout.
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
  does not enable the gate each set the HostedCluster's `ValidConfiguration`
  condition to `False` with a message naming the failing target, and leave the
  HostedControlPlane block unwritten.
- An unresolvable or unparseable release image sets the condition `Unknown` and
  retries rather than blocking permanently or propagating.
- A prerelease build of the minimum supported release — an engineering or
  release candidate — passes both floors rather than being rejected for sorting
  below its own final version.

**CPO and HCCO — unit and integration tests.**

- The konnectivity sidecar's proxy arguments derived from the HCP block, including
  that the component proxy takes precedence over the hosted cluster-wide proxy, that
  the cluster-wide proxy is used when the component block is absent, and that the
  management cluster's own proxy is never used.
- CPO reconciles the webhook configuration and the CA mounts in both the sidecar
  and the webhook container; removal cleans all of it up.
- CPO defers `trustedCAFilePath` until the proxy CA ConfigMap exists in the HCP
  namespace, and publishes it once the HyperShift operator's copy lands.
- HCCO writes only the proxy field on the CVO-seeded guest
  `console.operator.openshift.io/cluster` resource, plus its CA copy, using
  destination-local references.
- HCCO read-back: against a guest whose CRD prunes the proxy field, HCCO reports
  through its own `ConfigOperatorReconciliationSucceeded` condition rather than
  treating the write as successful, and does not touch
  `ValidHostedControlPlaneConfiguration`, which CPO owns.
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
| CPO | webhook, konnectivity sidecar | `spec.controlPlaneRelease`, else `spec.release` |
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
  than by mitigation.

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
  `trustedCAFilePath`, the operator must not publish the path before the
  referenced CA bundle is actually synchronized into the operand's namespace
  and mounted into the Deployment, since ConfigMap synchronization, the
  Deployment's volume mount, and generated-configuration distribution are
  reconciled independently of one another. CAO addresses this by deferring
  `trustedCAFilePath` until it can confirm the destination ConfigMap already
  exists, rather than assuming mount provisioning keeps pace with
  configuration generation. Confirming the ConfigMap exists does not by
  itself guarantee the Deployment's pod template already mounts it as a
  volume; that is reconciled separately. But because the same workload
  controller responsible for the mount is already converging independently
  once the ConfigMap exists, any remaining gap is covered by a rollout
  already in progress rather than left open-ended.

  CPO applies the same rule on HyperShift, deferring `trustedCAFilePath` until
  the proxy CA ConfigMap exists in the HCP namespace. Its dependency chain is one
  hop longer than CAO's: that ConfigMap is written by the HyperShift operator
  copying the bundle HC→HCP, so CPO's deferral transitively waits on that
  synchronization completing. See the
  [CA distribution path](#hypershift--hosted-control-planes).

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
| Control plane | `spec.controlPlaneRelease.image`, else `spec.release.image` | CPO, HCCO, the OAuth API server and its konnectivity sidecar |
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
or gates set the HostedCluster's `ValidConfiguration` condition to `False` with
an `InvalidConfiguration` reason naming the target that fell short, and **block
propagation**, so no part of the configuration is applied. Release metadata
lookup or version parsing failures set the condition `Unknown` and retry rather
than reporting the payload unsupported.

A third version participates but cannot be validated here: the HyperShift
operator's own. It is installed independently of both payloads, and
`hypershift install` selects which CRD variant to apply by feature-set
annotation. An operator predating the feature — or a current one installed
without the preview feature set while the field is still preview-scoped — therefore
serves a HostedCluster CRD without `operatorConfiguration.authentication`, and
the API server prunes the administrator's proxy block on write. The apply
succeeds, `ValidConfiguration` stays `True`, and the configuration is simply
absent, because the component that would report the loss is the one that is too
old or ungated. No condition can cover this case. It is the inverse of the
guest-side pruning in [Cross-Cluster Write Skew](#cross-cluster-write-skew),
which HCCO can detect precisely because a newer component performs the write;
here the administrator or support must confirm the field persisted on the
HostedCluster, as [Diagnosing Hosted Clusters](#diagnosing-hosted-clusters)
describes.

This follows HyperShift's established pattern for feature-level support
checks — a minimum release constant evaluated during reconciliation and
surfaced through the existing HostedCluster validation condition — and the
validation the
[External Claims Sourcing enhancement](external-oidc-additional-identity-information-sources.md)
defines for the control-plane target, extended to the guest target because this
feature has a guest-side consumer that one does not. It introduces no new
condition type.

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

Support should treat a blocked `ValidConfiguration` on the HostedCluster as the
expected signal for an unsupported payload, and an HCCO read-back failure as an
indication that something changed underneath an already-validated
configuration.

HCP and guest reconciliation remain asynchronous; this proposal adds no aggregate
condition confirming proxy and CA propagation across both paths, relying instead
on the existing `ValidConfiguration` and
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
  still deferring the path until the destination ConfigMap exists
  ([resource-ordering skew](#asynchronous-reconciliation-hazards)); that resolves
  itself and is not missing trust.
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
the HCP namespace on the management cluster and egresses through its konnectivity
sidecar.

- **First confirm the proxy block is actually stored on the HostedCluster.** If
  the HyperShift operator predates the feature, or was installed without the
  preview feature set while the field is still preview-scoped, its HostedCluster
  CRD does not carry the field and the API server pruned it on write without an
  error. Every condition below then reports a cluster that simply has no
  component proxy configured ([Hosted Support
  Validation](#hosted-support-validation)).
- **Then read the HostedCluster conditions.** `ValidConfiguration` set to `False`
  with an `InvalidConfiguration` reason naming a release target is the expected
  signal for an unsupported payload: the HyperShift operator blocked propagation,
  nothing was applied, and the cluster keeps its previous proxy behavior
  ([Hosted Support Validation](#hosted-support-validation)). `Unknown` means
  release lookup or version parsing failed and is being retried. Before GA, also
  confirm both gate sides — the management-side HyperShift gate and the hosted
  `featureGate.featureSet`; once the gates reach their default feature sets, the
  release floors are the only remaining constraint
  ([HyperShift Feature Gating](#hypershift-feature-gating)).
- **Do not read the webhook container's proxy environment as the effective
  proxy.** CPO points that environment at the pod's konnectivity sidecar; the
  component proxy appears as the sidecar's `--https-proxy`, `--http-proxy`, and
  `--no-proxy` arguments
  ([Webhook Egress and Connection Paths](#webhook-egress-and-connection-paths)).
  Bypass problems for the webhook are sidecar `--no-proxy` problems, and that list
  must still cover the in-cluster endpoints. Failures reaching the proxy itself
  appear in sidecar logs, not webhook logs.
- **Expect webhook egress to originate from the guest network.** The proxy must be
  reachable and DNS-resolvable from a worker node. Before the first worker joins,
  the sidecar instead resolves through management-cluster DNS and dials the proxy
  directly, so a proxy reachable only from the guest network leaves hosted login
  failing until that worker arrives, then recovers on its own
  ([Day-0 Egress Before Worker Nodes](#day-0-egress-before-worker-nodes)). Rule
  that window in or out before treating the symptom as a configuration fault.
- **Walk the CA chain hop by hop:** the source ConfigMap in the HostedCluster's
  namespace, the HyperShift operator's copy in the HCP namespace, the HCCO copy in
  guest `openshift-config`, and Console Operator's copy in `openshift-console`
  ([distribution path](#hypershift--hosted-control-planes)). A content update that
  stalls at any hop leaves the remaining hops on the old bundle. Because the
  sidecar does not hot-reload, confirm the OAuth API server pod actually rolled
  after a CA rotation; an updated ConfigMap with an older pod means the sidecar
  still holds the previous trust.
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
direct path; on hosted clusters the webhook then falls back to the hosted
cluster's own cluster-wide proxy, never the management cluster's. On standalone
clusters the sources are `authentication.operator.openshift.io/cluster` (webhook)
and `console.operator.openshift.io/cluster` (Console login); on hosted clusters
both derive from
`HostedCluster.spec.operatorConfiguration.authentication.proxy`. Do not disable
TLS verification or remove the TokenReview webhook as a proxy workaround.
Reconciliation resumes after the configuration is fixed; verify both API token
authentication and a fresh Console login/refresh, not just cleared operator
conditions.

Disabling component proxy use does not delete user or workload data, but affected
users cannot submit new API requests while authentication is broken. Existing
workloads and service-account authentication do not depend on this OIDC proxy.
Redact proxy credentials, client secrets, and tokens from diagnostic output and
support attachments.
