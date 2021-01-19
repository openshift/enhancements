---
title: allowing-uri-scheme-in-oidc-sub-claims
authors:
  - "@stlaz"
reviewers:
  - "@stts"
  - "@deads2k"
approvers:
  - "@stts"
  - "@deads2k"
creation-date: 2020-01-19
last-updated: 2020-01-19
status: implementable
see-also:
  -
replaces:
  -
superseded-by:
  -
---

# Allowing URI Scheme in OIDC `sub` claims

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Make the oauth-server capable of setting up identities for OIDC identity
providers that use the URI scheme in the `sub` claim of their ID tokens.

## Motivation

Per the [OIDC specification](https://openid.net/specs/openid-connect-core-1_0-final.html#IDToken)
of the ID token and its more thorough details described in
[RFC7519 - JSON Web Token (JWT)](https://tools.ietf.org/html/rfc7519#section-4.1.2),
the `sub` claim may be represented as both a string or a URI. However, URI by
definition may contain characters such as `/` and `:` that are hard to digest
for the `identity.oauth.openshift.io` API, which prevents integration with OIDCs
that make use of this URI scheme. However, an identity provider that follows all
specifications should still be usable as such for OpenShift to successfully retrieve
identities from.

### Goals

1. Make it possible for users of the aforementioned identity providers to log in
   to OpenShift successfully.

### Non-Goals

None.

## Problem Summary

With OpenShift 4.x, the oauth-server no longer allows [configuring the claim
of ID tokens](https://github.com/openshift/api/blob/670ac3fc997c2f1d19b8c29ef04f70d6e3d4a59e/osin/v1/types.go#L341)
retrieved from the OIDC provider to use as the unique identifier
for the given user. This identifier is later used to construct the name of an
`Identity` object that describes the mapping between the identity provider the
user is a member of, and the user.

The name of the `Identity` object is constructed by using the following pattern:

```text
<identity provider name from config>:<unique user identifier from the identity provider>
```

This naming is further validated so that neither the username, nor the identity provider
name may contain the `:` character, and, according to Kubernetes naming schemes,
neither can also contain the `/` sign, yet both of these commonly appear as a
part of URI/URL schemes.

## Proposal

Due to the API limitations mentioned in [Problem Summary](#problem-summary), a
generic `sub` claim cannot be safely used in order to create an `Identity` object.
It is apparent that these will need to be encoded in a certain way. Simply encoding
a `sub` claim won't do as that would break authentication for any OIDC user
that had its `Identity` created prior to the version containing this fix.

Proposal: if a `sub` claim of an ID token contains either `:` or `/`, url-encode
the value of the claim and create the new identity object like so:

```yaml
apiVersion: user.openshift.io/v1
kind: Identity
metadata:
  name: <identity provider name>:url:<url-encoded sub claim value>
providerName: <identity provider name>
providerUserName: <url-encoded sub claim value>
user:
  name: <preferred username || url-encoded sub claim value>
  uid: <as usual>
```

Notice the naming scheme change where the `Identity` object name now consists of
three sections instead of previous two. The additional section simply contains
`url` to mark the encoding scheme used in the third section of the name.

The username of the user object also needs to be an encoded `sub` claim value if
a preferred username is not configured for the identity provider.

### User Stories

#### Story 1

I am a user of an OpenID Connect identity provider and I would like to log in to
OpenShift.

### Implementation Details

#### oauth-server

When creating a user and an identity object, the oauth-server will have to check
whether the username it retrieved from the third-party authentication logic contains
either of `:` or `/`, and if it does, follow the naming scheme from [Proposal](#proposal),
that is, url-encode the username for `User` objects for the object name, and
url-encode the username and prepend it with `url:` to use as a username in the
`Identity` object naming scheme.

#### oauth-apiserver

Validation of the names of the `Identity` objects need to be loosened in order
to allow three sections separated by the `:` character, where the only allowed
value for the second section is `url`.

### Risks and Mitigations

When a cluster with existing `Identity` objects of the new naming schemes is downgraded,
logging in with OIDC providers using URI scheme is their ID tokens' `sub` claims
will be broken again.

It is also possible that some OpenShift users already started using their OIDC
providers that were broken due to this, with the use of `unsupportedConfigOverrides`.
While we generally do not care about these cases, it is fair to say that the
proposed schema should allow for a scripted conversion of `Identity` and `User`
objects for them.

## Design Details

### Test Plan

Configure an OIDC provider that uses URL scheme for the `sub` claim in its
ID tokens (like Keycloak in some configurations), set it up as an OIDC identity
provider for an OpenShift cluster, and attempt to log in as a user of this
OIDC provider.

### Upgrade / Downgrade Strategy

No special strategy is considered for upgrades/downgrades.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

