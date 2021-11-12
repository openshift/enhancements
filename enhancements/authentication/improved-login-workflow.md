---
title: oc-authorization-grant-login
authors:
  - "@arjunrn"
reviewers:
  - "@stlaz"
  - "@s-urbaniak"
  - "@soltysh"
approvers:
  - "@soltysh"
  - "@mfojtik"
creation-date: 2021-11-08
last-updated: 2021-11-08
status: provisional
---

# oc Authorization Grant Login

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes a new workflow for the `oc login` command which requires fewer logins and no copy-pasting of
sensitive credentials. The new workflow is based on implementing some functionality in
the [oauth-server](https://github.com/openshift/oauth-server) and [oc](https://github.com/openshift/oc) as specified
in [RFC-8252](https://datatracker.ietf.org/doc/html/rfc8252).

## Motivation

The current CLI login process involves multiple interactive steps where the user has to enter credentials multiple times
in the browser. This process can be simplified so that the `oc` tool can be used to login directly without having to
first login to the cluster console, the OAuth server and finally copying the login command. The `oc` tool can be
modified to act as a public client to the Openshift OAuth server and received an access token
through [Authorization Code Grant](https://datatracker.ietf.org/doc/html/rfc6749#section-1.3) flow
and [PKCE](https://datatracker.ietf.org/doc/html/rfc7636).

### Goals

Provide a single-step login in `oc` for identity providers that don't post HTTP challenges.

### Non-Goals

None

## Proposal

### User Stories

__Story 1__ As a user I want to be able to login to the cluster with `oc` and authenticating/providing credentials at
most once.

__Story 2__ As a user I want the login without having to copy and paste credentials in plain-text.

### API Extensions

None

### Implementation Details [optional]

#### _oc login_ command

The oc login command will have a new flag `--browser` which indicates to `oc` that it should use the OAuth2
Authorization Code Grant Flow in a browser to login. Currently, when a user enters `oc login $CLUSTER_API_URL`, `oc`
attempts to login on the cluster if the cluster has identity provider which
supports [challenge based authentication](https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/WWW-Authenticate).
The new login flow cannot be a fallback if challenge based authentication is unavailable because this may break existing
workflows where the user expects `oc` to login with the supplied credentials or for `oc` to prompt for credentials. For
example, if a user runs `oc login` in a script on a remote headless machine, with the assumption that challenge based
authentication is available, launching a browser would not be a reasonable fallback. Consequently, a new flag which
indicates to `oc` to start the browser login is required.

[RFC-8252](https://www.rfc-editor.org/rfc/rfc8252.html) described how native apps(any app which does not run completely
in a browser) can safely authenticate the user against a know OAuth server. `oc` is such an application and any changes
will be done as prescribed by the RFC.

#### OAuth server

The internal OAuth server uses [OSIN](https://github.com/openshift/osin) as a dependency to provide OAuth2 Authorized
Code Grant Flow for a registered client to fetch tokens. It also has support for PKCE(Proof Key for Code Exchange) as
specified in [RFC-7636](https://datatracker.ietf.org/doc/html/rfc7636). _RFC-7636_ is an enhancement for public clients,
i.e. clients which cannot hold client credentials to fetch an access token securely. Similarly `oc` is also a public
client because it cannot be distributed with client credentials to retrieve the access token and PKCE is designed for
use cases like this one.

The OSIN library will have to be modified so that when the redirect URI is a loopback address of the
form `http://127.0.0.1:{port}/{path}` or `http://[::1]:{port}/{path}` the redirect URL verification should ignore the
port value. This is required because one particular port may not always be available on all machines where `oc` is run.
This change has been prescribed as per [RFC-8253](https://datatracker.ietf.org/doc/html/rfc8252#section-7.3)

#### cluster-authentication-operator

The [cluster-authentication-operator](https://github.com/openshift/cluster-authentication-operator) provisions the
default [OAuthClient](https://docs.openshift.com/container-platform/4.9/rest_api/oauth_apis/oauthclient-oauth-openshift-io-v1.html)
resources. It already provisions a client for `oc` but sets the `respondWithChallenges` to _true_ which is unsuitable
for OAuth Grant Flow. Therefore, another client is required where the `respondWithChallenges` is set to `false` which
should also be provisioned by the operator.

### Risks and Mitigations

1. There are no changes to existing behavior for users of `oc`. Previous login workflows continue working with the same
   behavior as in previous versions.
2. The new login workflow in `oc` will need to listen on a port for the callback from the OAuth server. This port cannot
   be fixed because the port may be in-use on the user's machine and the user may not have the permissions to resolve
   this. Instead, the user should be able to specify an alternate port which is free on the system. If the user does not
   specify a port `oc` can pick any available port at random.
4. Some OSes under certain configurations might prevent the callback server from listening on the loopback interface. In
   this case the server would not start and `oc` would print a message that it cannot listen on that interface.
5. If `127.0.0.1` on the user's machine is not the machine itself but has been compromised and points to another
   location then the attacker can obtain the security code. But this code cannot be exchanged for a token because the
   attacker does not have access to the challenge key which is only in memory on the users machine.

## Design Details

### Open Questions

### Test Plan

The modifications in _osin_ will have corresponding unit tests. No end-to-end test will be added here.

Because this login workflow relies on a browser being launched and some user interaction in the browser it is not
trivial to implement an end-to-end test which tests the entire workflow. However, the existing unit tests for the `oc
login` command can be extended to verify the following scenarios:

1. The browser is launched with the correct path and parameters.
2. If the browser cannot be launched then a suitable error message is displayed.
3. When the callback is received oc processes the callback and shuts down the server listening for callback even in
   error cases.

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal should keep this high-level with a
focus on what signals will be looked at to determine graduation.

Consider the following in developing the graduation criteria for this enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to
the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning), or by redefining
what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions

[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**Examples**: These are generalized examples to consider, in addition to the
aforementioned [maturity levels][maturity-levels].

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

**For non-optional features moving to GA, the graduation criteria must include end to end tests.**

#### Removing a deprecated feature

NA

### Upgrade / Downgrade Strategy

NA

### Version Skew Strategy

#### Compatibility of new oc login flow with previous versions of OCP

When the new login flow is used `oc` will open a new browser window which will make a request on the `/authorize`
endpoint. The client-id needs to be passed here, and it will be the name of the new `OAuthClient` resource created by
the `cluster-authentication-operator`.

_Newer oc with older API server_: In older versions of OCP where the `cluster-authentication-operator` has not created
this new client the request to the OAuth server will fail because the client ID is invalid. The redirect to the endpoint
on loopback will not happen and an error message will be displayed in the browser window.

_Older oc with newer API server_: The new client does not change existing behavior and older `oc` clients will continue
to work as they have previously.

### Operational Aspects of API Extensions

None

#### Failure Modes

1. `cluster-authentication-operator` If the operator cannot provision the new `OAuthClient` it will transition to
   the `Degraded` state. Cluster administrators or the API team will then have to determine why this occurred. However,
   the operator currently provisions 2 other clients in the same fashion and a failure here due to misconfiguration
   would already be visible.
2. `oc` If the new login mechanism does not work users will still be able to login with the existing workflow through
   the console.

#### Support Procedures

NA

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

TBD

## Alternatives

None

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new subproject, repos requested, github
details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources started right away.
