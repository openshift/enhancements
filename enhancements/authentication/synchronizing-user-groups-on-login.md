---
title: synchronizing-user-groups-on-login
authors:
  - "@stlaz"
reviewers:
  - "@sur"
  - "@slaskawi"
approvers:
  - "@deads2k"
  - "@sttts"
  - "@sur"
  - "@stlaz"
creation-date: 2021-05-22
last-updated: 2021-07-07
status: implementable
see-also:
replaces:
superseded-by:
---

# Synchronizing User Groups on Login

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

OpenShift should be capable of synchronizing user groups from a 3rd party
identity provider upon a user logging in to their OpenShift cluster.

## Motivation

In the real world, OpenShift is not the owner of user identities but these
are stored in a 3rd party identity provider system. When OpenShift retrieves
such an identity, it usually only gathers information about the user's unique
identification in that IdP and optionally a username that's preferred for the
user if it's different from its identifier.

It is a common practice to group users in logical groups on the identity provider
side and sometimes it would make sense to bring the groups over to the OpenShift
cluster as well. The identity providers oftentimes allow a protocol-specific path
to retrieve group information, which is something OpenShift should leverage in those
cases where it is desired.

### Goals

- allow retrieving user's groups from a 3rd party OIDC identity provider into
  OpenShift when such a user logs in

### Non-Goals

- implement group synchronization for an identity provider type other than
  OIDC
- keep the groups retrieved by OpenShift from a 3rd party identity provider
  synchronized between user logins
- for a given session, only have the user appear in the groups synchronized
  from the identity provider associated with this session

## Proposal

### User Stories

#### Story 1

I am an administrator of an OpenShift cluster that uses identities from
my company's identity provider. The users of the identity provider are
organized in a complex and well-though-out group structure. I would like
OpenShift to respect the group structure so that I can assign privileges
to the users on OpenShift side according to their group membership in the
identity provider.

### Implementation Details

#### OIDC Group Retrieval

Neither the OIDC core specification nor JWT RFC mention a standard claim that would serve for user group
storage and so none of the standard scopes define accessing it. While this could work without
a need for any further configuration, we may need a way to specify extra scopes, which can
already be done in the oauth-server configuration by specifying `extraScopes` in the identity
provider configuration (`oauth.config.openshift.io/cluster`).

A possible follow-up implementation might include being able to
[request specific claims](https://openid.net/specs/openid-connect-core-1_0.html#ClaimsParameter)
from the UserInfo endpoint or from an `id_token`, which would allow for a more fine-grained
configuration to reach the group claim.

#### API

The `OpenIDClaims` config API structure shall be expanded by a new `Groups`
field:

```golang
// OpenIDClaims contains a list of OpenID claims to use when authenticating with an OpenID identity provider
type OpenIDClaims struct {
    ...

    // groups is the list of claims whose values should be converted into OpenShift groups
    // that will include the user
    // +optional
    Groups []string `json:"groups,omitempty"`
}
```

The group object (`groups.user.openshift.io.com/v1`) should be annotated with
`oauth.openshift.io/idp.<idpname>: "synced"`
where `<idpname>` marks the identity provider name from the configuration. It should
also get an `oauth.openshift.io/generated: true` if it hasn't previously existed.

#### oauth-server

When the OIDC provider has group synchronization configured, the oauth-server
must attempt to retrieve the group claim (or claims) from the `id_token` it
obtains from the provider after a successful authentication, and should do
the same for the JSON data retrieved from the OIDC's UserInfo endpoint
should the OIDC be exposing this endpoint in its OIDC discovery data. The
UserInfo response must be authoritative in that case.

Once the server has the user's groups information, it will look up the
OpenShift groups the user appears in, and it will either add to/remove from/create
the necessary groups for the user. The user should only be removed from the groups
that were created for that specific identity provider (as per the annotation
from the [API section](#api)). If such a group ends up being empty, it should
be removed if it contains the `oauth.openshift.io/generated: true` annotation.

When a group that would otherwise be created on sync already exists, the
oauth-server should only add the user among the group members and add an
`oauth.openshift.io/idp.<idpname>` annotation to mark users of that IdP appear
in that group.

This implementation is not solving an issue when a user can be sourced from
multiple identity providers when `mappingMethod: add` is configured for these
providers. Such a user would get group memberships in all the groups from
all matching identity providers. An alternative implementation will be
discussed in the [Alternatives section](#alternatives).

### Risks and Mitigations

Given that the group synchronization only happens on logins, there is a time
window during which the synchronization of groups between the identity provider
and OpenShift is not guaranteed. This would typically be an issue when someone's
group membership gets revoked.

In order to force OpenShift group membership synchronization on group membership
revocation on the identity provider side, one might:

1. revoke all the OAuthAccessTokens for a given user by removing them, leveraging
   a `userName` field selector, thus forcing the user to relogin:

```bash
       oc delete oauthaccesstoken --field-selector=userName="<username>"
```

2. remove the user from the OpenShift group manually

Users can also be added to groups in the IdP, in which case they are more
likely to willingly cause a resync by `oc login` themselves.

## Design Details

### Open Questions

### Test Plan

The authentication operator already has a way to configure Keycloak and
communicate with its API. Use that to add test users and groups to test:

1. user can still log in when groups are configured but no group
   claims are received
2. on login, user gets added to a previously non-existent group when
   the group appears in the configured claims
3. on login, user gets added to an existing group that appears in the
   retrieved group claims
4. on login, user gets removed from any existing group that was created
   for the given identity provider (per annotation) when the group
   no longer appears among those retrieved from the OIDC


### Follow-up Work

To synchronize groups in a time-based manner, an additional controller in the
oauth-server would be necessary. OIDC does not provide a standardized group
membership and reverse group membership lookups. That means that such a controller
would have to cache the access tokens (and possibly refresh tokens) retrieved
from the OIDC after a successful user login, and use them to query the UserInfo
endpoint. This could be troublesome, however, because:

1. the access token we get from the OIDC might be shortlived, if there are no
   refresh tokens we can't synchronize the groups for the given user for too long
2. we might end up generating a lot of traffic to the OIDC provider
3. the OIDC may not expose/implement the UserInfo endpoint

### Graduation Criteria

The feature should be considered mature once it lands.

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

There are two drawbacks to the current solution:

1. the groups go out of sync between logins
- This is quite a common problem with all systems that receive delegated identities.
  Third party vendors don't usually provide a standardized watch channel that could be
  effectively and reliably observed, and so most systems either only synchronize on user
  login or attempt to perform group retrievals in regular intervals, but real real-time
  synchronization is usually impossible.

2. the groups are assigned to a user, not an `<identity provider:user>` touple
- Some configurations allow a user to be sourced by multiple identity providers, where
  each of the providers might have different groups configured. The implemenentation
  described in this enhancement does not distinguish the groups when the user attempts
  to use their token and just uses all the OpenShift groups the user appears in.

## Alternatives

### oauth-server alternative

The OAuthAccessToken, User and Groups are the three things that the current OpenShift
authenticator retrieves during token validation.

To solve the issue of the user only appearing in the groups of
`union(current identity provider groups, OpenShift groups)`, we could take an
alternative approach and only synchronize the groups into the OAuthAccessToken for
the given user and session, since, as noted above, the access token is one of the
three objects introspected during authentication, and is the only one of the three
that is sure to describe the current session.

Having the groups stored within the access token would likely make group
synchronization in between logins (a possible future expansion) harder to perform
because to assure the groups in the token actually come from the given identity provider,
the field should be immutable (as all the other OAuthAccessToken fields are).

### Using vanilla Kubernetes OIDC config

Another alternative approach involves using the [Kubernetes OIDC authenticator](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#configuring-the-api-server)
directly to retrieve identities. This way the OpenShift authentication stack does
not get involved at all. This means that:

- No `OAuthAuthorizeToken`/`OAuthAccessToken` get created.
- No `Identity`/`User`/`Groups` object get created.
- Neither `ServiceAccount` or `OAuthClient` and the credentials derived from them
  will be respected when applications attempt to retrieve user's credentials from
  the identity provider. That is, unless the 3rd party identity provider does not
  implement their own integration with OpenShift, which is unlikely.
- A third-party identity provider is not likely to know about the OAuthAccessToken
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
  the `authentication.openshift.io/cluster` resource so that these clients are
  redirected to the correct URLs.

  - Alternatively, this configuration may stay the same, but in that case a user
    would have to configure an OIDC identity provider in the `oauth.openshift.io/cluster`
    resource so that they are able to use it. This would lead to an inconsistent
    behavior where logins with an ID token retrieved from the OIDC identity provider
    would have the OIDC provider groups, but logins with username/password in
    `oc`, `oauth-proxy`, OpenShift web console would not + the behavior would
    likely differ in all the other points outlined in this section.
