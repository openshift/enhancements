---
title: direct-use-of-oidc
authors:
  - "@stlaz"
reviewers:
  - "@s-urbaniak"
  - "@liouk"
  - "@ibihim"
  - "@deads2k"
approvers:
  - "@deads2k"
  - "@s-urbaniak"
api-approvers:
  - "@deads2k"
  - "@tkashem"
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link: []
see-also:
  - [oc Authorization Grant login](./improved-login-workflow.md)
  - [Configuring OIDC directly - study](./direct-oidc-study/study-oidc-in-openshift.md)
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
able to login to different kinds of identity providers in a uniform manner.
On the other hand, `kubectl`, the binary for accessing Kubernetes APIs, has evolved
through the years to simplify the login process to Kubernetes clusters, and communities
built useful toolings around the binary.

## Motivation

As the `oauth-server` is an authentication middle-man that mints
OpenShift-specific access tokens, it is impossible to use tokens issued by a
3rd party OIDC provider directly to authenticate to OpenShift clusters. Such
a token gets forwarded directly to the `kube-apiserver` instead of the `oauth-server`,
where the latter is at least aware of the 3rd parties that should be used as identity
sources. But even if the token went directly to the `oauth-server`, there currently exists
no such mechanism for the `oauth-server` to mint an OpenShift-specific access
token based on an access or ID token from a 3rd party OIDC provider. Not to
mention that a client should not forward its access tokens to a different entity.

For the reasons above, it makes sense to allow configuring OIDC-related flags of
the `kube-apiserver` directly in order to fully leverage capabilities of the
community-built tools around OIDC-related authentication. However, there are some
OpenShift specifics that one needs to bear in mind should they want to use their
own OIDC provider.

### Goals

- describe expectations that an OIDC provider needs to fulfill in order for its
  integration with OpenShift to be seamless
- provide a guide to configure OIDC directly with a use of unsupportedConfigOverrides
- point out which OpenShift components would be affected by the `oauth-server` disappearing
  from the cluster and what they would need to do to integrate with a direct OIDC
  provider instead

### Non-Goals

- focus on a closer integration with any specific OIDC provider

## Proposal

### User Stories

1. I am an OIDC provider developer and I would like to try out how my product integrates
   with OpenShift

### Affected core functionality the auth team cares about

Throughout the time of its existence, the OpenShift authentication stack has built
several expectations about how OAuth clients work, which claims should be present
in a token and several others.

The following points are important to realize for any administrator that would decide
to configure OIDC directly in their cluster:

- Token revocation is an unsolved problem in the Kubernetes direct OIDC configuration. This
  is based on vanilla Kube mostly relying on ID tokens instead of access/refresh tokens
  for its authentication. While the revocation of the latter is [standardized](https://datatracker.ietf.org/doc/html/rfc7009),
  revocation of ID tokens is undefined.
  A very short lifetime (~5 min) must be imposed on ID tokens to keep such a system
  safe.
- Since the integrated `oauth-server` is no longer involved in the login process,
  none of the following objects get created upon login:
  - `OAuthAuthorizeToken`/`OAuthAccessToken`
  - `Identity`/`User`/`Groups`
- Neither `ServiceAccount` or `OAuthClient` and the credentials derived from them
  will be respected when applications attempt to retrieve user's credentials from
  the identity provider. That is, unless the 3rd party identity provider implements
  their own integration with OpenShift, which is unlikely.
- A third-party identity provider is not likely to know about the `OAuthAccessToken`
  scopes that OpenShift uses that some components may rely upon (e.g. `user:info`
  scope for components that only need to be able to get the user data). This means
  that the 3rd party OIDC needs to be flexible in order to allow requesting unknown
  scopes and scopes with a given prefix (for the [role scopes](https://docs.openshift.com/container-platform/4.13/authentication/tokens-scoping.html#scoping-tokens-role-scope_configuring-internal-oauth)).
- The upstream configuration for OIDC does not currently allow setting the
  `userInfo.Extra` fields of the authentication response so even if the 3rd party
  OIDC identity provider allows custom scopes, there is currently no way for the
  OpenShift-specific `scopeAuthorizer` to learn of these.
  This would be solved by the [structured OIDC configuration KEP](https://github.com/kubernetes/enhancements/issues/3331).
- Any component that relied on an OpenShift-specific retrieval of user information
  (==> `get` on `User` echo endpoint with a given accesstoken) would need to be rewritten to
  either use the OIDC token's claims or retrieve that information from the UserInfo
  endpoint of the OIDC identity provider.
- In order for `oc`, `oauth-proxy` and OpenShift web console to work with the given
  3rd party OIDC identity provider, the user must configure the `oauthMetadata` of
  the `authentication.config.openshift.io/cluster` resource so that these clients are
  redirected to the correct URLs.

### Configuring an OIDC provider directly for the kube-apiserver

The `kube-apiserver` binary already provides flags to [configure direct OIDC integration](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#openid-connect-tokens). While these are not exposed in any of the configuration
APIs, they can be supplied in the `unsupportedConfigOverrides` field, by adding them
under the `apiServerArguments` key in a `kubeapiserver.operator.openshift.io/cluster` patch such as
```json
{"spec":
    {"unsupportedConfigOverrides":
        {"apiServerArguments":
            {
              "oidc-issuer-url":["an.issuer.url"],
              "oidc-client-id":["a-client-id"],
              "oidc-ca-file":["/a/ca/file/path.crt"],
              ...
            }
        }
    }
}
```

Note that the [structured OIDC configuration KEP](https://github.com/kubernetes/enhancements/issues/3331)
introduces enhancements to the configuration but also moves the whole configuration
into a file.

**Unsupported files synchronization**<br>
The OIDC configuration is very likely to require configuring additional files to be
mounted to the kube-apiserver static pods. A typical example is the `oidc-ca-file`.

This requires synchronizing additional `configMaps`/`secrets` into static pods.
The kube-apiserver-operator should support two subfields to the `unsupportedConfigOverrides`:
- `unsupportedConfigMaps`
- `unsupportedSecrets`

These each follow the same API:
```
- name: <openshift-config ref>
  path: <cannot contain any path traversal>
  defaultMode: <file permissions>
```
where:
- `name` points to the name of the resource in the `openshift-config` namespace
- `path` specifies a subpath of the `/etc/kubernetes/unsupported` directory where
  the `configMap`/`secret` should be mounted
- `defaultMode` matches the common `volume`'s `defaultMode` semantics

**OAuth metadata**<br>
`oc login`, the web console and the `oauth-proxy` are all using the kube-apiserver's
`/.well-known/oauth-authorization-server` endpoint to discover the details about the
identity provider in the cluster.

The data served is configurable from the `authentications.config.io/cluster` object.
In order to modify the default values, users should set `Type: None` and point the
`oauthMetadata` field to a config map in the `openshift-config` namespace.

This should make the authentication operator report `Upgradeable: false`.

### Observed issues with direct OIDC configuration

Configuring OIDC provider directly in the kube-apiserver brings issues since
the OpenShift authentication stack does not consider this option.

The following section will ignore all problems already outlined in
[Affected core functionality the auth team cares about](#affected-core-functionality-the-auth-team-cares-about)
and will describe issues in the main 3 users of the authentication stack:
- web console
- `oc login`
- oauth-proxy

Each subsection will provide a proposal of how these issues could be solved if
we decided to allow direct OIDC configuration some time in the future.

The subsections consider the public-facing configuration APIs that are present
at the time of writing of this proposal.

#### Web Console

The web console makes the following assumptions:
1. the presense of the `console` client in the OAuth2/OIDC system
2. it can configure the client credentials
3. it requests the `user:full` scope
4. the ability to extract the user info from the `users.openshift.io/~` endpoint

None of the above is likely to exist in a default deployment of an OIDC provider.

On the other hand, the console uses the standard OAuth2 Code Grant to request tokens.

[Old code](https://github.com/openshift/console/blob/3e0bb0928ce09030bc3340c9639b2a1df9e0a007/cmd/bridge/main.go#L614)
exists in the console that apparently worked for a specific OIDC provider in the past
but it also makes some assumptions that are not defined in the standard and so it
is broken in different ways than the code which is used for OpenShift authentication.
Read more in the [attached study](./direct-oidc-study/study-oidc-in-openshift.md).

In order to allow proper integration with a 3rd party OIDC provider, additional
configuration API would have to exist. It would have to include:
1. a reference to the OAuth2 client credentials that the console can use
2. configuration of the scopes it should use to request authorization from the
   3rd party IdP

Additional considerations:
1. The console would likely have to forward the ID tokens it retrieves to the
   kube-apiserver to get the `userInfo` based on the kube-apiserver configuration,
   not on the configuration of the console to avoid discrepancies in console/kube-apiserver
   identity configuration.
2. From 1. we can see that the ID tokens for the console would likely have to contain
   both the `kube-apiserver` and `console` OAuth2 clients in the audience claim
3. ID tokens are not revocable which is commonly solved by these being short-lived.
   The `console` should therefore implement token refresh.

**Open question**: based on 3. we need refresh tokens for seamless web console use.
During token refresh, an ID token does not necessarily have to be returned. There
are a few options in that case:
1. Ignore the above 1. and have the console authenticate the user itself and impersonate
   it in the requests to the kube-apiserver.
2. Use the access token from the token refresh flow to retrieve claims from the
   `UserInfo` endpoint and forward those to the kube-apiserver. However, these
   are not time-limited (no `exp` claim) and do not have to contain the `iss` and `aud` claims.
3. Have the configuring user decide. Maybe ID tokens are present in token refresh
   flow. Maybe at least `iss` and `aud` are present in the `UserInfo` endpoint responses.
   The user should know.

#### oc login

`oc login`, like the web console, has many assumptions on its own:
1. existence of the `openshift-challenging-client`
2. the trust should be present either in the system bundle or in the CA bundle from the kubeconfig
3. `oc` uses the issuer with added `/oauth/token/implicit` path as the `redirect_uri` for the implicit flow
4. `oc` expects to be able to use HTTP challenge to authenticate to the authorization endpoint of the issuer

In order to get `oc login` to work, [oc Authorization Grant Login](./improved-login-workflow.md)
should get implemented. Additionally, it should be able to retrieve parameters
specific to the OIDC provider deployment, such as which OAuth2 client to use
and which scopes to request.

As an alternative, `oc login` might try to use an exec plugin supplied by the
OIDC provider directly instead of attempting something of its own. Perhaps `oc login`
would not be used for cluster login at all.

Even now, `oc` is capable of using an ID token directly if the kube-apiserver
is configured accordingly. The main issue is with retrieving the token.

#### oauth-proxy

The oauth-proxy often relies on the functionality of the oauth-server that allows
it to use its ServiceAccount as the OAuth2 client. That will no longer work, each
oauth-proxy instance will have to run with an OAuth2 client that gets created in
the OIDC.

The proxy also relies on the user echo endpoint to get the information about the
user. This will likely have to change similarly as with the web console.

Ideally, there would be API that would help to decide component operators whether
to deploy the oauth-proxy at all.

### API Extensions

None

### Implementation Details/Notes/Constraints [optional]

None

### Risks and Mitigations

The "unsupported files synchronization" introduces a way to install files to the
master nodes.

The config API that allows this is already very privileged and therefore does not
introduce any additional threat, but it is still worth pointing out.

## Design Details

### Open Questions [optional]

See the question at [Web Console](#web-console), which might also apply to oauth-proxy
in case the decision was made to fix it.

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

At its current shape and form, the enhancement only considers the use-case through
unsupported config overrides. Further graduation would only be considered if the
configuration described proves to be useful to those parties that require direct
OIDC provider integration.

#### Dev Preview -> Tech Preview

Read [Graduation Criteria](#graduation-criteria).

#### Tech Preview -> GA

Read [Graduation Criteria](#graduation-criteria).

#### Removing a deprecated feature

Irrelevant.

### Upgrade / Downgrade Strategy

Don't allow upgrades.

### Version Skew Strategy

Irrelevant

### Operational Aspects of API Extensions

TBD

#### Failure Modes

TBD

#### Support Procedures

Unsupported.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

TBD

## Alternatives

TBD

## Infrastructure Needed [optional]

Irrelevant at this point
