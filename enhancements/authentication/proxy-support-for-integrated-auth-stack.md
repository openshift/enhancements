---
title: proxy-support-for-integrated-auth-stack
authors:
  - "@tchap"
reviewers:
  - "@liouk"
  - "@everettraven"
approvers:
  - "@benluddy"
api-approvers:
  - "@everettraven"
creation-date: 2026-05-18
last-updated: 2026-05-18
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced|informational
tracking-link:
  - https://redhat.atlassian.net/browse/CNTRLPLANE-3376
see-also:
  - "/enhancements/proxy/global-cluster-egress-proxy.md"
  - "/enhancements/authentication/direct-external-oidc-provider.md"
---

# Proxy Support for Integrated Auth Stack

## Summary

This enhancement adds an optional, component-scoped proxy configuration to
the `Authentication` operator so that authentication components can reach
external identity providers (IdP) through a proxy without requiring the cluster-wide egress proxy,
enabling more restrictive network policies in disconnected environments.

## Motivation

The authentication stack holds a unique position among OpenShift control plane components.
Several components make outbound calls — the Cluster Version Operator (CVO) checks for
updates, the Insights Operator uploads telemetry, the Operator Lifecycle Manager (OLM)
fetches catalogs — but those connect to well-known Red Hat infrastructure that can be
mirrored or disabled in disconnected environments. Cloud-related operators (Cloud Credential
Operator, Machine API) call cloud provider APIs, which can be reached via private networking
(PrivateLink, Private Service Connect). The authentication stack is different: when an
external identity provider is configured, both the cluster authentication operator and the
OAuth server must reach arbitrary, customer-chosen IdP endpoints for OIDC discovery, token
exchange, user info, and group membership — on every login. These targets cannot be mirrored
and are not reachable via private networking.

```
                        ┌─────────────────────────────────────────────────────┐
                        │                   OpenShift Cluster                 │
                        │                                                     │
  User                  │  ┌───────────────────────────────────────────────┐  │
  (oc login,            │  │  Cluster Authentication Operator (CAO)        │  │
   console)             │  │  namespace: openshift-authentication-operator │  │
       │                │  │                                               │  │
       │                │  │  • Deploys and configures the other two       │  │
       │                │  │  • Validates IdP configuration ───────────────│──│──► External IdP
       │                │  │  • Syncs certificates and secrets             │  │    (OIDC discovery)
       │                │  └───────────────────────────────────────────────┘  │
       │                │                                                     │
       │                │  ┌───────────────────────────────────────────────┐  │
       ▼                │  │  OAuth Server                                 │  │
  ┌──────────┐          │  │  namespace: openshift-authentication          │  │
  │  Route   │──────────│─►│                                               │  │
  │ (ingress)│          │  │  • Hosts /login, /oauth/authorize, /callback  │  │
  └──────────┘          │  │  • Redirects user to external IdP             │  │
                        │  │  • Exchanges auth codes for tokens ───────────│──│──► External IdP
                        │  │  • Fetches user info and group membership ────│──│──► External IdP
                        │  │  • Issues OpenShift OAuth access tokens       │  │
                        │  └───────────────────────────────────────────────┘  │
                        │                                                     │
                        │  ┌───────────────────────────────────────────────┐  │
  kube-apiserver ──────►│  │  OAuth API Server                             │  │
  (token validation     │  │  namespace: openshift-oauth-apiserver         │  │
   webhook)             │  │                                               │  │
                        │  │  • Stores OAuth tokens, clients, identities   │  │
                        │  │  • Validates tokens on behalf of KAS          │  │
                        │  └───────────────────────────────────────────────┘  │
                        └─────────────────────────────────────────────────────┘
```

In a disconnected or otherwise restricted environment, this creates a tension. The cluster needs
to reach the IdP, but the only mechanism available today is the cluster-wide egress proxy, which
opens egress for all components uniformly — there is no way to grant it to only the authentication
stack. Enabling the cluster-wide proxy solely for auth means every other component also gets proxy
configuration, undermining the restricted network posture the administrator set up.

The current workarounds include:

1. **Cluster-wide proxy + restrictive proxy ACLs** -- configure the cluster-wide proxy but lock down the proxy server itself
   to only forward traffic to IdP domains. Every component receives the env vars and attempts to use the proxy,
   but only auth-related traffic is allowed through. This leaks intent and generates noise from failed proxy connections in non-auth components.

2. **Cluster-wide proxy + network policy** -- enable the cluster-wide proxy, then use `NetworkPolicy` or `EgressFirewall`
   (OVN-Kubernetes) to restrict which pods can reach the proxy endpoint. Only auth pods get network-level egress to the proxy;
   everything else is blocked. This works but is fragile -- every component is configured with proxy settings, and a separate mechanism prevents most of them from using it.

3. **Internal IdP federation** -- deploy an internal identity provider (e.g., Keycloak) that federates to the external IdP, and only allow
   the internal IdP to reach the outside network. This adds significant operational overhead: a full identity service to deploy, maintain,
   and upgrade, with its own HA, certificates, and storage requirements. It also doubles auth latency.

4. **Egress sidecar** -- inject a sidecar proxy (e.g., Envoy) into the OAuth Server pod to intercept outbound IdP traffic and forward it
   through a network path permitted to egress, avoiding proxy environment variables entirely. This requires deploying, configuring, and
   maintaining the sidecar (TLS origination, routing rules, health checks) and managing its lifecycle across upgrades. It also does not cover
   the operator's own outbound calls (OIDC discovery during config observation), which run in a separate pod — requiring either a second
   sidecar or a cluster-level solution like a service mesh, adding complexity disproportionate to the problem.

5. **Manual env var injection** -- skip the cluster-wide proxy API entirely and patch the OAuth Server and operator deployments directly with proxy env vars.
   This is unsupported, breaks on upgrades when the CVO reconciles the deployments, and doesn't survive operator-managed redeployments.

These workarounds are rather complicated for the value they bring,
hence the need to add the option to add component-scoped proxy to authentication only.

### User Stories

1. As an OpenShift cluster administrator, I want to configure proxy settings scoped to authentication components
   so that I can connect to external identity providers without opening cluster-wide egress.

### Goals

- Configure proxy settings scoped to authentication components, avoiding cluster-wide egress.
- Allow authentication flows (login, token exchange, group resolution) to function with external IdPs in disconnected or restricted clusters.
- Maintain compatibility with existing authentication providers (OIDC, OAuth-based providers).

### Non-Goals

- Per-identity-provider proxy configuration. Proxy settings apply to all authentication components uniformly.
- Modifications to the cluster-wide proxy API (`proxy.config.openshift.io/v1`).
- A generalized per-component proxy framework. This is scoped to authentication only; extending to other operators would require a separate enhancement.
- HyperShift (Hosted Control Planes) support. HyperShift manages authentication proxy requirements through its own mechanisms and is excluded from the initial scope.
- Proxy support for the OAuth API Server's External OIDC mode. External OIDC is gated behind TechPreviewNoUpgrade and its egress is limited to the `oauth-apiserver external-oidc` subcommand. Proxy support for that code path can be added when External OIDC matures; it is expected to reuse the same `spec.proxy` configuration rather than introducing a separate field.
- Proxy configuration for user-managed LDAP group sync CronJobs. These are not managed by the authentication operator and must be configured independently by the administrator.

## Proposal

To achieve the given goals, the following is proposed:

- The `Authentication` operator (`operator.openshift.io/v1`) spec is extended to contain
  proxy configuration, including support for a custom trusted certificate authority (CA) certificate bundle.
  The API mirrors the cluster-wide proxy configuration for consistency.
- Cluster Authentication Operator (CAO) is extended to use the component-scoped proxy configuration
  when `Authentication.spec.proxy` is set. This overrides the cluster-wide proxy configuration and is applied to
  all affected components:
  1. CAO uses the proxy configuration and trusted CA when validating IdP endpoints during config observation.
  2. The proxy configuration and trusted CA are passed to oauth-server as environment variables and a mounted CA bundle respectively.

### Workflow Description

The configuration workflow:

1. The cluster administrator specifies the `proxy` field in the `Authentication` spec:
   ```yaml
   apiVersion: operator.openshift.io/v1
   kind: Authentication
   metadata:
     name: cluster
   spec:
     managementState: Managed
     proxy:
       httpProxy: "http://proxy.corp.example.com:3128"
       httpsProxy: "http://proxy.corp.example.com:3128"
       trustedCA:
         name: "auth-proxy-ca-bundle"
   ```
2. `spec.proxy` is picked up by the operator, causing it to:
   1. re-run IdP validation (e.g., OIDC discovery) using the proxy,
   2. re-validate proxy connectivity via the proxy validation controller,
   3. re-deploy `oauth-server` using new environment variable values and trusted CA bundle.
      This includes synchronizing the given `trustedCA` ConfigMap into `openshift-authentication` namespace.
      This step must also be repeated every time the `trustedCA` ConfigMap content is modified.

When the proxy configuration is removed, it causes the same process to happen, just not using any proxy,
or using the cluster-wide proxy when that is specified.

The user authentication workflow:

1. A cluster end user uses `oc login` to log into the cluster.
2. They authenticate with the associated IdP.
3. When the auth callback is received by `oauth-server`, it does server-side token exchange,
   it gets user info and group membership. The configured proxy is used for all these server-side calls.
4. A new OpenShift access token is created via `oauth-apiserver` and returned to the user.

### API Extensions

The `Authentication` operator (`operator.openshift.io/v1`) spec needs to be extended to accommodate the proxy settings.

`AuthenticationProxyConfig` mimics the cluster-wide proxy configuration for consistency.

```golang
type AuthenticationSpec struct {
	OperatorSpec `json:",inline"`

	// proxy configures proxy settings for authentication components
	// (the OAuth server and the cluster authentication operator).
	// When set, these values are used for authentication components,
	// overriding the cluster-wide proxy (proxy.config.openshift.io/cluster).
	// No per-field inheritance from the cluster-wide proxy occurs.
	// When omitted (nil), the cluster-wide proxy is used, preserving
	// existing behavior.
	// +optional
	Proxy *AuthenticationProxyConfig `json:"proxy,omitempty"`
}

// AuthenticationProxyConfig holds proxy configuration scoped to
// authentication components (the OAuth server and the cluster
// authentication operator).
type AuthenticationProxyConfig struct {
	// httpProxy is the URL of the proxy for HTTP requests.
	// An empty string means no HTTP proxy is used.
	// +required
	HTTPProxy *string `json:"httpProxy"`

	// httpsProxy is the URL of the proxy for HTTPS requests.
	// An empty string means no HTTPS proxy is used.
	// +required
	HTTPSProxy *string `json:"httpsProxy"`

	// trustedCA is a reference to a ConfigMap in the openshift-config
	// namespace containing a CA certificate bundle under the key
	// "ca-bundle.crt". This bundle is appended to the system trust store
	// used by authentication components for proxy TLS connections.
	// When omitted, only the system trust store is used.
	// +optional
	TrustedCA configv1.ConfigMapNameReference `json:"trustedCA,omitempty"`
}
```

`httpProxy` and `httpsProxy` are `*string` with `+required`, so CRD schema validation
ensures both are always present when `spec.proxy` is set — there is no partial
configuration state and proxying can't get disabled by accident by forgetting to fill in a field.

No CRD-level URL format validation is applied, following the same approach as the cluster-wide proxy
(`config.openshift.io/v1 Proxy`), which also accepts free-form strings. URL format and `trustedCA`
content are validated at runtime by the proxy validation controller, which sets `Degraded` on
invalid configuration.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift is out of scope for this proposal.
The CAO does not run in HyperShift (it is excluded from the CVO payload); instead, the
control-plane-operator manages auth components directly. Auth components already have
proxy-aware transport plumbing for OIDC discovery, but it is wired to the cluster-wide
proxy (`HostedCluster.spec.configuration.proxy`), not a component-scoped one. Future
HyperShift support would require reading a component-scoped proxy field and passing it
into the existing transport code (which includes konnectivity sidecars for outbound
proxy connectivity), but as of now there is no request for a similar feature in HyperShift.

#### Standalone Clusters

Yes, this is applicable to standalone clusters.

#### Single-node Deployments or MicroShift

> How does this proposal affect the resource consumption of a
single-node OpenShift deployment (SNO), CPU and memory?

This does not add any additional overhead besides network latency for the one extra proxy hop.

> How does this proposal affect MicroShift? For example, if the proposal
adds configuration options through API resources, should any of those
behaviors also be exposed to MicroShift admins through the
configuration file for MicroShift?

The auth stack is not present on MicroShift.

#### OpenShift Kubernetes Engine

Not affected.

### Implementation Details/Notes/Constraints

#### Proxy Resolution

The `httpProxy` and `httpsProxy` fields are required when the `proxy` object is present.
This is enforced by CRD schema validation, preventing partial configurations where an
administrator sets `spec.proxy` but forgets to specify the proxy URLs.

Resolution follows three states:

1. `spec.proxy` set with non-empty values — use component-scoped proxy, overriding any cluster-wide proxy.
2. `spec.proxy` set with empty strings (`httpProxy: ""`, `httpsProxy: ""`) — explicitly no proxy for auth components, even if a cluster-wide proxy is configured.
3. `spec.proxy` absent (`nil`) — fall back to the cluster-wide proxy (`proxy.config.openshift.io/cluster`) as today.

No per-field inheritance from the cluster-wide proxy occurs; component-scoped proxy is all-or-nothing.

The operator always sets `NO_PROXY` to static cluster-internal defaults
(`.cluster.local`, `.svc`, `localhost`, `127.0.0.1`) when the component-scoped proxy is active.
There is no user-configurable `noProxy` field — per-IdP proxy configuration is a non-goal.
Authentication components connect to internal services via DNS names covered by `.svc` and
`.cluster.local`, so network CIDRs and the api-int hostname are not needed.

#### Operator Process

When the component-scoped proxy is configured, it is read from the API resource, not injected
as process-level environment variables. All outbound calls from the operator process that
previously relied on `http.ProxyFromEnvironment` must use a proxy-aware HTTP transport with
the resolved proxy configuration instead. When `spec.proxy.trustedCA` is set, the component
CA is loaded into the operator's certificate pool for these calls. The following operator
controllers are affected:

- **Config observation** — OIDC discovery calls during IdP validation use a proxy-aware
  transport so that discovery requests reach external IdP endpoints through the component proxy.

- **Endpoint accessibility** — the route health check hits the external OAuth route hostname,
  which in cloud environments resolves to an external load balancer. Without proxy awareness,
  this check would falsely report the OAuth server as unavailable when no cluster-wide proxy
  is configured. The route check controller is updated to use the resolved component proxy.

- **Proxy validation** — the existing proxy validation controller already tests OAuth route
  reachability through the cluster-wide proxy. It is extended to also validate the
  component-scoped proxy configuration and to test IdP endpoint connectivity on configuration
  change. Transient IdP unreachability shall emit a Warning event; the `Degraded` condition is
  reserved for proxy-level and configuration failures (connection refused, TLS handshake
  errors with the proxy itself).

#### OAuth Server

The operator injects `HTTP_PROXY`, `HTTPS_PROXY`, and `NO_PROXY` environment variables into the
OAuth Server pod spec. Changes to proxy configuration trigger a redeployment.
The OAuth Server's HTTP transports already respect proxy environment variables, so no
application-level code changes are needed in the OAuth Server itself.

When `spec.proxy.trustedCA` is set, the referenced ConfigMap is synced from `openshift-config`
to `openshift-authentication` and mounted as an additional volume in the OAuth Server pod.
The container entrypoint appends it to the system trust store after the existing system trust
copy, resulting in the OAuth Server trusting: system CAs + cluster-wide proxy CA (if any) + component proxy CA.
This follows the existing entrypoint pattern that already copies the injected system trust bundle
into the container's trust path.

CAO must watch the source ConfigMap in `openshift-config`, copy it into `openshift-authentication` on any
change and also re-deploy OAuth Server so that the updated ConfigMap is picked up.

#### LDAP Group Sync

[LDAP group sync](https://docs.redhat.com/en/documentation/openshift_container_platform/4.18/html/authentication_and_authorization/ldap-syncing#ldap-auto-syncing_ldap-syncing-groups)
is typically configured as a user-managed `CronJob` that runs `oc adm groups sync`. This job is
not managed by the authentication operator, so setting `spec.proxy` will not configure proxy
settings for LDAP group sync. Administrators who rely on LDAP group sync in disconnected
environments must independently configure proxy settings for their sync jobs (e.g. by setting
`HTTP_PROXY`/`HTTPS_PROXY` environment variables on the `CronJob` pod spec). This limitation
should be documented.

### Risks and Mitigations

**Cluster lockout from invalid proxy configuration.**
A misconfigured component proxy (wrong URL, missing CA) can prevent
the OAuth Server from reaching the external IdP, locking all users out of the cluster.
The proxy validation controller tests IdP connectivity on configuration change and reports
warnings for unreachable IdPs and `Degraded` for proxy-level failures (connection refused,
TLS handshake errors), but these conditions are informational — the configuration is applied
regardless. Recovery is possible via `kubeadmin` credentials or client certificate
authentication, which bypass OAuth entirely. The risk is the same class as any IdP
misconfiguration today.

**Proxy as an untrusted intermediary.**
A proxy positioned between auth components and the IdP can observe or tamper with
authorization codes, tokens, and user info. This is the same trust model as the
cluster-wide proxy — the administrator who configures the proxy is assumed to control it.
The `trustedCA` field pins the proxy's TLS certificate, and all IdP traffic uses HTTPS,
so the proxy cannot silently intercept without a trusted CA. The security model and
implications should be documented.

**Network dependency in the authentication path.**
Adding a proxy hop introduces a new availability dependency: if the proxy is down,
all authentication fails. This is the same failure class as a cluster-wide proxy outage
and is mitigated by setting a `Degraded` condition when the proxy is unreachable, giving
administrators visibility. Proxy high availability is the administrator's responsibility
and should be documented as a prerequisite.

**Proxy credential leakage.**
Proxy credentials embedded in the URL (e.g., `http://user:pass@proxy:3128`) are stored
in the operator spec and propagated as environment variables to OAuth Server pods. This
mirrors the cluster-wide proxy's approach. Whether to add support for
`proxyCredentials` SecretNameReference for improved credential handling is listed as an open question.

**Debugging complexity from dual proxy sources.**
When both a cluster-wide proxy and a component-scoped proxy exist, diagnosing connectivity
issues requires understanding the precedence rules. The proxy validation controller reports conditions when the proxy itself is misconfigured
and emits events when IdP endpoints are unreachable through the proxy. The resolved proxy
values are visible as environment variables on the OAuth Server pod spec.
The precedence rules (component-scoped > cluster-wide > none) should be documented
clearly.

**SNO auth disruption during rollout.**
On single-node deployments, a proxy configuration change triggers an OAuth Server
redeployment, causing a brief authentication outage (same as any OAuth config change
on SNO today). This is inherent to the single-replica topology and should be documented.

### Drawbacks

The main drawback is that we are adding ad-hoc proxy configuration for one particular component.
The rationale is that the auth stack holds a rather special position in disconnected environments —
it is the only component that must reach external services for the cluster to be usable,
yet granting that access through the cluster-wide proxy opens egress for everything else.
This is explicitly not a generalized per-component proxy framework, and expanding scope to
other operators would require a separate enhancement.

## Alternatives (Not Implemented)

**Per-IDP proxy fields on `oauth.config.openshift.io`:** Proxy configuration could be specified per IdP,
but this is explicitly rejected. The goal is to have a component-scoped proxy configuration.

**Annotations on the Authentication operator resource:** Proxy settings could be stored as annotations
instead of typed spec fields. This loses CRD schema validation, discoverability via `oc explain`,
and generated documentation.

## Open Questions

1. **Should a validating admission webhook reject invalid proxy configuration?** Currently, validation
   is done by the controller at runtime (setting `Degraded` on error), matching the cluster-wide proxy
   pattern. A webhook would give immediate feedback on `oc apply`, but introduces availability concerns:
   if the webhook is unavailable, writes to the `Authentication` resource are either blocked (preventing
   recovery) or silently unvalidated. The most valuable validation (IdP connectivity through the proxy)
   is inherently async and cannot run in a webhook regardless.

## Test Plan

TBD

## Graduation Criteria

TBD

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

This change is scoped to the CAO only. Since we are adding new functionality attached to
the new and optional `Authentication.spec.proxy` field, there are no principal issues with upgrades
since nothing new happens until the cluster administrator sets the `spec.proxy` field
once the cluster is upgraded. Only then does the new functionality kick in.

Regarding downgrades, the older operator simply does not know about the `spec.proxy` field
and ignores it, so authentication falls back to the cluster-wide proxy automatically.
The field itself will eventually get pruned by the API server as unrecognized on the next
write to the resource. In case `spec.proxy.trustedCA` is set, the cluster administrator
may want to delete the ConfigMap synchronized into the `openshift-authentication` namespace,
otherwise it will be left abandoned. Just leaving it there, though, poses no issue.

## Version Skew Strategy

This change is scoped to the CAO only. Version skew doesn't apply.

## Operational Aspects of API Extensions

This enhancement adds fields to the existing `Authentication` CRD (`operator.openshift.io/v1`).
No webhooks, aggregated API servers, or finalizers are introduced. The only operational
impact is that a misconfigured proxy can block authentication — visible via the
`ProxyConfigControllerDegraded` condition and `IdPEndpointUnreachable` Warning events.

## Support Procedures

Proxy misconfiguration is detectable via:
- `oc get clusteroperator authentication` — the `Degraded` condition is set for proxy-level failures (connection refused, TLS handshake errors with the proxy itself).
- `oc get events -n openshift-authentication-operator` — `IdPEndpointUnreachable` Warning events are emitted when IdP endpoints are unreachable through the component proxy.
- Operator logs — detailed error messages for both proxy and IdP connectivity failures.

Recovery: remove `spec.proxy` from the `Authentication` resource to fall back to the
cluster-wide proxy, or correct the proxy URL / CA configuration. The `kubeadmin` and
client certificate authentication paths bypass OAuth entirely and remain available for
recovery access.
