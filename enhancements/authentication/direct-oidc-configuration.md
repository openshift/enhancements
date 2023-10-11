---
title: direct-oidc-configuration
authors:
  - stlaz
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - deads2k
  - sjenning
  - tkashem
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - deads2k
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - deads2k
creation-date: 2023-10-10
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/HOSTEDCP-1240"
see-also:
  - "/enhancements/authentication/direct-oidc-study/study-oidc-in-openshift.md"
  - "https://github.com/kubernetes/enhancements/commits/master/keps/sig-auth/3331-structured-authentication-configuration"
replaces:
  - "/enhancements/authentication/unsupported-direct-use-of-oidc.md"
superseded-by: []
---

# Direct OIDC Configuraition

## Summary

This enhancement describes how to allow configuring OIDC directly as the main
source of identities in an OpenShift cluster.

## Motivation

In vanilla Kubernetes, it is possible to configure a remote OIDC provider as the
source of identities. This has made it possible to create solutions that are based
around it. However, these are incompatible with the current OpenShift authentication
stack.

We want to both enable the aforementioned solutions, and we want to make it possible
for users to be able to use their OIDC providers' features to their full extent.

### User Stories

* I am a cluster administrator and I want to be able to use the full potential
  of my OIDC provider in my OpenShift clusters
* As an OpenShift user, I want to be able to use ID tokens issued by our company's
  OIDC provider to query the OpenShift API

### Goals

- Provide an API that exposes kube-apiserver OIDC configuration with possible
  expansions to accommodate recent "Structured Authentication Configuration"
  Kubernetes enhancement.

### Non-Goals

- Allow using OIDC directly and keep the original OpenShift authentication
  stack working.

## Proposal

### Workflow Description

1. a cluster gets created
2. a cluster administrator requests OAuth client credentials from
   their company's identity team
3. the cluster administrator configures the cluster with the OAuth
   client credentials for use with their company's OIDC provider via
   a configuration API provided by OpenShift
4. users interacting with the cluster can now use the company's OIDC
   provider credentials for authentication

#### Variation and form factor considerations

New API is introduced that could be used across the various flavours of OpenShift
to configure direct use of OIDC credentials when interacting with OpenShift clusters.

The API proposal also includes validation of the configuration that should be
implementable on any platform.

The implementation aims on the standalone OCP as that's the most complex
platform so far. Any derived platform can adopt the pieces that apply to it.

### API Extensions

`authentications.config.openshift.io` is the API that should be getting extended.

Currently, setting the value of `Type` to anything other than `IntegratedOAuth`,
or an empty string that semantically matches `IntegratedOAuth`, is unsupported.

A new value for the `Type` field is introduced - `OIDC`. Setting this value
allows the user to configure a new field: "OIDCProviders".

```go
type Authentication struct {
    ...
    // OIDCProviders are OIDC identity providers that can issue tokens
    // for this cluster
    // Can only be set if "Type" is set to "OIDC".
    //
    // At most one provider can be configured.
    //
    // +listType=atomic
    // +kubebuilder:validation:MaxItems=1
    OIDCProviders []OIDCProvider `json:"oidcProviders,omitempty"`
    ...
}

type OIDCProvider struct {
    // Issuer describes atributes of the OIDC token issuer
    //
    // +kubebuilder:validation:Required
    // +required
    Issuer TokenIssuer  `json:"issuer"`

    // ClaimMappings describes rules on how to transform information from an
    // ID token into a cluster identity
    ClaimMappings TokenClaimMappings `json:"claimMappings"`
}

type TokenIssuer struct {
    // URL is the serving URL of the token issuer.
    // Must use the https:// scheme.
    //
    // +kubebuilder:validation:Required
    // +required
    URL string  `json:"url"`

    // Audiences is an array of audiences that the token was issued for.
    // Valid tokens must include at least one of these values in their
    // "aud" claim.
    // Must be set to exactly one value.
    //
    // +listType=set
    // +kubebuilder:validation:Required
    // +kubebuilder:validation:MaxItems=1
    // +required
    Audiences []string `json:"audiences"`

    // CertificateAuthority is a reference to a config map in the
    // configuration namespace. The .data of the configMap must contain
    // the "ca.crt" key.
    CertificateAuthority ConfigMapNameReference  `json:"certificateAuthority,omitempty"`
}

type TokenClaimMappings struct {
    // Username is a name of the claim that should be used to construct
    // usernames for the cluster identity.
    //
    // By default, claims other than `email` will be prefixed with the issuer URL to
    // prevent naming clashes with other plugins.
    //
    // Set `username.prefix` to "-" to disable prefixing
    //
    // Default value: "sub"
    Username PrefixedTokenClaimMapping `json:"username,omitempty"`

    // Groups is a name of the claim that should be used to construct
    // groups for the cluster identity.
    // The referenced claim must use array of strings values.
    Groups PrefixedTokenClaimMapping  `json:"groups,omitempty"`
}

type TokenClaimMapping struct {
    // Claim is a JWT token claim to be used
    //
    // +kubebuilder:validation:Required
    // +required
    Claim string  `json:"claim"`
}

type PrefixedTokenClaimMapping struct {
    TokenClaimMapping `json:",inline"`

    // Prefix is a string to prefix the value of the token in the result of the
    // claim mapping
    Prefix string  `json:"prefix,omitempty"`
}
```

#### API Validation

An operator handling the API described in the [API Extensions section](#api-extensions)
should report a degraded condition if it fails to validate the values set. The
operator MUST NOT configure its operand as long as the configuration is considered
invalid.

Validation rules for the API:
1. The `OIDCProviders` field can only be set iff the `Type` field is set to the `OIDC`
   value
2. Only a single `OIDCProvider` can ever be set. This requirement can be lifted
   once upstream allows setting multiple OIDC IdPs in its structured authentication
   configuration.
3. The `OIDCProvider.Issuer.URL` MUST always use the "https://" scheme
4. Only a single audience can ever be set in `OIDCProvider.Issuer.Audiences`. This
   requirement can be lifted once upstream allows setting multiple audiences
   in its structured authentication configuration.
5. If `OIDCProvider.Issuer.CertificateAuthority` is set, it MUST refer an existing
   configMap that contains the `ca.crt` key in its `.data` field.

### Implementation Details

The following implementation describes how to implement the feature in a standalone
OCP. The configuration API might be different in other derived platforms, but aside
from that all the other implementation details are binding for all such platforms.

This feature gets configured by setting `Type: OIDC` in the `authentications.config.openshift.io/cluster`
resource. This is a sign for the operator handling the cluster authentication stack
to remove the OpenShift integrated authentication by removing the integrated
oauth-server and the oauth-apiserver.

The removal of the oauth-apiserver means that the `user.openshift.io` and
`oauth.openshift.io` APIs will be unavailable. As this would leave dangling objects
in etcd, all objects of these APIs should be removed prior to the API removal.

In an initial implementation, this removal shall be manual and performed by the user.
For GA, the operator handling the oauth-apiserver deployment should turn the API
server into a mode that refuses to accept any new CREATE requests on these APIs,
after which the objects get removed by the operator maintaining the oauth-apiserver.

> [!NOTE]
> TODO: will this break the Projects API?

The platform MUST provide means of administrative user authentication even without
the oauth-server. In case of standalone OpenShift, this is provided by the
`system:admin` certificate/key pair present in the installation kubeconfig.

Once `Type: OIDC` is set and the configuration passes validation, the operator
configuring the kube-apiserver converts the API values into appropriate flags.
In later versions, when the underlying kube-apiserver implements [a stable structured
authentication configuration](https://github.com/kubernetes/enhancements/tree/master/keps/sig-auth/3331-structured-authentication-configuration),
the values from the API get transformed into this new structure.

#### Hypershift

Hypershift is not prepared today for Tech Preview features and does not appear
to have a way to mark cluster support to be limited as such. However, this feature
in its first iteration will not be suitable for production.

The Hypershift team needs to introduce feature lifecycle into their architecture.

### Risks and Mitigations

There will be a lot of components broken from the start. To see the list of components
that is going to break, read the "Observed issues with direct OIDC configuration" section
of the [Unsupported direct use of OIDC as an identity provider](./unsupported-direct-use-of-oidc.md)
document.

Each of these failures needs to be addressed ad-hoc. Additionally, these are
only the components we know of. There is a good chance there are third-party
components that rely on the way OpenShift authentication stack worked until now.
These will have to read the `authentication/cluster` resource in order to figure
out how the authentication stack is configured.

### Drawbacks

TODO:

## Design Details

### Test Plan

End to end tests are a MUST for the feature. The tests should check:
- no kube-apiserver rollout is performed as long as the configuration is invalid
- the operator handling the configuration API clearly reports failing validation
- oauth-server and oauth-apiserver are not present when OIDC is configured
- user can authenticate to the OpenShift API with their ID tokens

We may need manual testing for the following:
- web console and oauth-proxy work with the new authentication stack

### Graduation Criteria

#### Dev Preview -> Tech Preview

A new `KubeAPIServerDirectOIDCConfig` feature gate is introduced. This feature
gate unlocks the controller configuring the kube-apiserver from reacting to the
new OIDC API.

The OAuth and User API group objects must be removed by hand by the users prior
to configuring the direct OIDC identity provider.

Clusters using this feature are prevented from upgrading among major (.y) versions.

Unit tests are implemented for all new functionality.

#### Tech Preview -> GA

- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)


#### Removing a deprecated feature

Irrelevant here.

**------------- WIP: I finished here -------------**

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

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
