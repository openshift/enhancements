---
title: Secure OAuth Resource Storage
authors:
  - "@sttts"
reviewers:
  - "@mfojtik"
  - "@deads2k"
  - "@stlaz"
  - "@marun"
  - "@polynomial"
approvers:
  - "@mfojtik"
  - "@Anandnatraj"
creation-date: 2020-03-30
last-updated: 2020-06-29
status: provisional
see-also:
  - "/enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
replaces:
superseded-by:
---

# Secure OAuth Resource Storage

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

1. Do we really need token read access for users. This enhancement allows us to add that with reasonable effort,
   but we should consider also either some imperative logout subresource, or an endpoint for the oauth server. This
   will be simpler to implement, but lack the `oc get oauthaccesstokens` ability for normal users.

## Summary

OAuthAccessTokens and OAuthAuthorizeToken use the object name for the security
sensitive token information. Hence, encrypting the value only in etcd means that
the token itself is stored as plain text in the etcd database and in encrypted
etcd backups, i.e. unprotected.

This enhancement is about the migration to a different storage format where the
token object name is insensitive and can be therefore stored in plain text without
risk.

While changing the storage format, we will prepare for different token formats in
the future by introducing a prefix syntax. The prefix will be used by the
authenticator to distinguish between the old and new verification semantics.

## Motivation

Etcd encryption without encrypted oauth access and authorize token is of limited
value. While these tokens expire after a day usually (but this can be and is often
customized by administrators to much longer times), the token from a fresh backup,
the output of etcdctl or even stack traces or trace information in the apiserver
 logs can be stolen and used to act as oauth authenticated user in the cluster.

A side-effect of non-sensitive object names is that these can be exposed via the
API to other parties than the owner of the token in the session using the token.
This is helpful to create an API that allows a user to list his token objects,
e.g. to implement a feature to log out all "other" tokens in a REST-ful way.

### Goals

- Change the `oauthaccesstokens` and `oauthauthorizetokens` storage format and
  schema such that the etcd keys alone (which are not encrypted when encryption
  at REST is enabled) are not sensitive.
- Make a "log out all other sessions" feature possible in the future.

### Non-Goals

- Design the "log out all other sessions" feature.

## Proposal

We prefix oauth access and oauth authorize token names with a prefix:

- no prefix for old tokens,
- `"sha256~"` for new tokens. The name of the token objects is the sha256 hash
  of the actual token (not decoded, but plain base64) encoded as base64. The bearer
  token will also carry the prefix, but have the token in plain text.

In v4.(n-1) and in v4.n:
- we add support to verify sha256 tokens.

In v4.n:
- we create tokens of the new format.

In v4.(n+1):

- we might remove the support to verify old tokens from the kube-apiserver authenticator.
  Note: this is a breaking API change as the user is able to add tokens manually without restriction. We would restrict it to sha256 tokens.
- we might add a feature to allow logouts of all other sessions.

Note: the tilde is a valid character in oauth tokens used as bearer tokens (according to
[RFC6750](https://tools.ietf.org/html/rfc6750#section-2.1)), and it works as character in URL paths. Colon,
which we used in an earlier iteration of this enhancement is not valid for the former.

### User Stories

#### Story 1

As an attacker who got access to an etcd backup, but not the encryption keys I do
**not** get access to oauth tokens.

#### Story 2

Using an old oc (pre v4.n-1) I want to log into a v4.n or v4.n-1 (after downgrade)
cluster.

#### Story 3

With a token created in v4.n-1 I want to access a v4.n or v4.n+1 cluster.

#### Story 4

With a token created in v4.n I want to access a v4.n-1 cluster after downgrade.

#### Story 5

With a token created in v4.n-1 I do not expect to access a v4.n+1 cluster, i.e. the token will be invalid and I have to relogin.

#### Non-goal Story 6

(should be feasible, but a non-goal to design & implement) As a user I can use the API via oc or console to log out other sessions.

#### Non-goal Story 7

(should be feasible, but a non-goal to design & implement) As a user I can list my other oauth sessions, including information like client name and expiration.

### Implementation Details/Notes/Constraints

#### Authenticator

The oauth authenticator in kube-apiserver today does a GET look-up of the passed
oauth token against the API server (with a caching layer in-front).

In v4.n and v4.n-1, it will be extended:

- if the passed token in the request is prefixed with `sha256~` the token value
  is hashed and the hashed value is looked up to find the oauthaccesstoken
  object.

#### oauth-server

The oauth-server creates access and authorize tokens. In v4.n and v4.n-1 (z-stream)
it will prefix the token with `sha256~` and the `registrystorage` package will use
`sha256~<hashed-token>` as object names. Old (unprefixed) tokens will be stored
and read with the plain text name in order to allow disruption-free upgrade.

- `oauth-server/pkg/osinserver/registrystorage`:
  - in `LoadAuthorize`, `SaveAuthorize`,
    `RemoveAuthorize`, `SaveAccess`, `LoadAccess` will hash the passed token before using
    it as an object name if it is prefixed.
  - `convertFromAuthorizeToken` is extended with a token parameter, used for the field
    `Code` instead of the object name.
  - `convertToAuthorizeToken` will hash the `Code` field and use that as the object name if the code is prefixed.
  - `convertFromAccessToken` is extended with a token parameter, used for the field `AccessToken`
    instead of the object name.
  - `convertToAccessToken` will hash the `AccessToken` field and use that as the object name if it is prefixed.

- `oauth-server/pkg/server/tokenrequest/tokenrequest.go`:
  - in `displayTokenPost` the `accessData.AccessToken` field is hashed before passing
    it to the client `Get` function if the token is prefixed.

- `oauth-server/pkg/osinserver/tokengen.go`:
  - in v4.n, we will prefix the returned token strings for access and authorize tokens.
  - in v4.n-1 z-stream, we keep the old prefix-less token generation code.

In `convertToAccessToken` the code setting `OAuthAccessToken.AuthorizeToken` is
commented out today. Hence, that value is not set and used at all through-out the
system. We will deprecate that field. If we need it in the future, we would rather
store the hashed and prefixed authorize token there, and make that clear from the
(new) field name.

#### Logout and Read Access for Users

Intentionally left empty. To be filled in the future.

### Risks and Mitigations

### Test Plan

- Old tokens with new kube-apiserver after v4.(n-1)->v4.n upgrade have to keep working.
- New tokens with an old kube-apiserver after a v4.n->v4.(n-1) downgrade will keep working.

### Upgrade / Downgrade Strategy

- New kube-apiserver will authorize clients with old tokens (43 characters long) without prefix.
- Old kube-apiserver of v4.(n-1) will be updated via z-stream release to handle `sha256~` prefixed tokens.
- Bearer tokens with `sha256~` prefix are handled transparently by client-go clients.
  The clients don't introspect tokens and the token format is not documented and
  no stable API.

## Implementation History

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.
