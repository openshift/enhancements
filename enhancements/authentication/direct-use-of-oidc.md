---
title: direct-use-of-oidc
authors:
  - "@stlaz"
reviewers:
  - "@s-urbaniak"
  - "@slaskawi"
  - "@deads2k"
approvers:
  - "@deads2k"
api-approvers: []
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link: []
see-also: []
replaces: []
superseded-by: []
---

# Direct use of OIDC as an identity provider

## Summary

When users authenticate to OpenShift, the authentication goes through the `oauth-server`
which serves as middle-man actor that unifies access to multiple kinds of
identity providers, such as LDAP, OIDC providers, providers returning HTTP Basic
Authentication challenges, and similar.

The above differs from vanilla Kubernetes where authentication is dealt directly
by the `kube-apiserver`. This has limitations especially when it comes to being
able to login in a uniform manner. On the other hand, `kubectl`, the binary for
accessing Kubernetes APIs, has evolved through the years to simplify the login
process to Kubernetes clusters, and communities built useful toolings around
the binary.

## Motivation

Due to the `oauth-server` being the authentication middle man minting
OpenShift-specific access tokens, it is impossible to use tokens minted by a
3rd party OIDC provider directly to authenticate to OpenShift clusters as such
a token is forwarded directly to the `kube-apiserver` and even if it weren't and
went directly to the `oauth-server`, there exists no such mechanism to for the
`oauth-server` to mint an OpenShift-specific access token based on an access or
ID token from a 3rd party OIDC provider - not to mention that a client should not
forward its access/ID tokens to a different entity.

For the reasons above, it makes sense to allow configuring OIDC-related flags of
the `kube-apiserver` directly in order to fully leverage capabilities of the
community-built tools around OIDC-related authentication. However, there are some
OpenShift specifics that one needs to bear in mind should they want to use their
own OIDC provider.

### Goals

- allow directly configuring OIDC-related flags in `kube-apiserver`, guarded
  by a  `TechPreviewNoUpgrade` feature gate
- describe expectations that an OIDC provider needs to fulfill in order for its
  integration with OpenShift to be seamless

### Non-Goals

- focus on a closer integration with any specific OIDC provider

## Proposal

### User Stories

1. I am an OIDC provider developer and I would like to try out how my product integrates
   with OpenShift

### API Extensions

There shall be a new `TechPreviewNoUpgrade` `Feature` called `KubeOIDCConfiguration`.
This feature gate will make a `cluster-kube-apiserver-operator`'s observer respect
the configuration fields from `authentication.config.openshift.io/cluster` which
will then be turned in `kube-apiserver` configuration flags.

The `authentication.config.openshift.io` CRD gets a new struct field, `KubeOIDC`:

```go
type AuthenticationSpec struct {
  ...
  KubeOIDC OIDCKubeConfig `json:"kubeOIDC,omitempty"`
  ...
}

type OIDCKubeConfig struct {
    // IssuerURL - The URL of the OpenID issuer, only HTTPS scheme will be
    // accepted. If set, it will be used to verify the OIDC JSON Web Token (JWT).
    IssuerURL string `json:"issuerURL,omitempty"`

    // ClientID - The client ID for the OpenID Connect client, must be set
    // if IssuerURL is set.
    ClientID string `json:"clientID,omitempty"`

    // RequiredClaims - key=value pairs that describes a required claims in the ID
    // Token. If set, the claims are verified to be present in the ID Token with
    // matching values
    RequiredClaims map[string]string `json:"requiredClaims,omitempty"`

    // UsernameClaim - The OpenID claim to use as the user name. Note that claims
    // other than the default ('sub') is not guaranteed to be unique and immutable.
    // This flag is experimental, please see the authentication documentation for
    // further details.
    // TODO: what is the authentication documentation?
    UsernameClaim string `json:"usernameClaim,omitempty"`

    // UsernamePrefix - If provided, all usernames will be prefixed with this
    // value. If not provided, username claims other than 'email' are prefixed
    // by the issuer URL to avoid clashes. To skip any prefixing, provide the
    // value '-'.
    UsernamePrefix string `json:"usernamePrefix,omitempty"`

    // GroupsClaim - If provided, the name of a custom OpenID Connect claim for
    // specifying user groups. The claim value is expected to be a string or
    // array of strings. This flag is experimental, please see the authentication
    // documentation for further details.
    GroupsClaim string `json:"groupsClaim,omitempty"`

    // GroupsPrefix - If provided, all groups will be prefixed with this value to
    // prevent conflicts with other authentication strategies.
    GroupsPrefix string `json:"groupsPrefix,omitempty"`

    // SigningAlgorithms - List of allowed JOSE asymmetric signing
    // algorithms. JWTs with a supported 'alg' header values are:
    // RS256, RS384, RS512, ES256, ES384, ES512, PS256, PS384, PS512.
    // Values are defined by RFC 7518 https://tools.ietf.org/html/rfc7518#section-3.1.
    //
    // Default value: RS256
    // TODO: define an enum type for this?
    SigningAlgorithms []string `json:"signingAlgorithms,omitempty"`
    
    // CAFile - If set, the OpenID server's certificate will be verified by one
    // of the authorities in the CAFile, otherwise the host's root CA
    // set will be used.
    CAFile ConfigMapNameReference `json:"caFile,omitempty"`
}

```

### Implementation Details/Notes/Constraints [optional]

There will be a new observer in `cluster-kube-apiserver-operator` which is going
to observe the `KubeOIDC` field of the `authentication.config.openshift.io/cluster`
configuration custom resource:
- If the `KubeOIDCConfiguration` feature gate is
enabled, the observer is going to turn the `KubeOIDC` configuration into
`kube-apiserver` flags.
- If the `KubeOIDCConfiguration` is disabled, any previous `kube-apiserver` OIDC-related
  flags get removed from the configuration and the `KubeOIDC` field of the 
  `authentication.config.openshift.io/cluster` resource is ignored

Additionally to the above, the configmap from `authentication.config.openshift.io/cluster`
`KubeOIDC.CAFile` should get synchronized from the `openshift-config` namespace
to `openshift-kube-apiserver` namespace if the `KubeOIDCConfiguration` feature
gate is enabled.

// TODO: decide how involved should the `oauth-server` be when OIDC is configured
// to provide identities directly

### Risks and Mitigations

Throughout the time of its existence, the OpenShift authentication stack build
several expectations about how OAuth clients work, which claims should be present
in a token and several others.

Anyone attempting to integrate their OIDC provider with OpenShift needs to take
the following points into account:

- Since the integrated `oauth-server` is no longer involved in the login process,
  none of the following objects get created upon login:
  - `OAuthAuthorizeToken`/`OAuthAccessToken`
  - `Identity`/`User`/`Groups`
- Neither `ServiceAccount` or `OAuthClient` and the credentials derived from them
  will be respected when applications attempt to retrieve user's credentials from
  the identity provider. That is, unless the 3rd party identity provider implements
  their own integration with OpenShift, which is unlikely.
- A third-party identity provider is not likely to know about the `OAuthAccessToken`
  scopes that OpenShift uses that some components may rely upon (e.g. "user:info"
  scope for components that only need to be able to get the user data). This means
  that the 3rd party OIDC needs to be flexible in order to allow requesting unknown
  scopes and scopes with a given prefix (for the [role scopes](https://docs.openshift.com/container-platform/4.7/authentication/tokens-scoping.html#scoping-tokens-role-scope_configuring-internal-oauth)).
- The upstream configuration for OIDC does not currently allow setting the
  "userInfo.Extra" fields of the authentication response so even if the 3rd party
  OIDC identity provider allows custom scopes, there is currently no way for the
  OpenShift-specific `scopeAuthorizer` to learn of these.
- Any component that relied on an OpenShift-specific retrieval of user information
  (==> `get` on `User` with a given accesstoken) would need to be rewritten to
  either use the OIDC token's claims or retrieve that information from the UserInfo
  endpoint of the OIDC identity provider.
- In order for `oc`, `oauth-proxy` and OpenShift web console to work with the given
  3rd party OIDC identity provider, the user must configure the `oauthMetadata` of
  the `authentication.config.openshift.io/cluster` resource so that these clients are
  redirected to the correct URLs.

  - Alternatively, this configuration may stay the same, but in that case a user
    would have to configure an OIDC identity provider in the `oauth.openshift.io/cluster`
    resource so that they are able to use it. This would lead to an inconsistent
    behavior where logins with an ID token retrieved from the OIDC identity provider
    would have the OIDC provider groups, but logins with username/password in
    `oc`, `oauth-proxy`, OpenShift web console would not + the behavior would
    likely differ in all the other points outlined in this section.


## Design Details

### Open Questions [optional]

1. Should the `oauth-server` be completely disregarded for the login process
   once OIDC is directly configured for the `kube-apiserver`?

## FIXME: NO POINT READING FURTHER, THIS IS WHERE I FINISHED FOR NOW

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

### Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

### Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

#### Failure Modes

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

#### Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
