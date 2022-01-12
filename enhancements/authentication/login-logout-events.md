---
title: Login Logout Auditing
authors:
  - "@s-urbaniak"
reviewers:
  - "@mfojtik"
  - "@deads2k"
  - "@stlaz"
  - "@sttts"
approvers:
- "@Anandnatraj"
- "@mfojtik"
creation-date: 2021-09-10
last-updated: 2021-09-10
status: implementable
see-also:
  - "/enhancements/kube-apiserver/audit-policy.md"
---

# Logging login, login failure and logout events

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement describes capturing login, login failure, and logout events in the OpenShift authentication subsystems.

Currently, login and logout events can only be derived from inspecting audit logs. These include creation or deletion of `oauthaccesstokens` resources as described in the API server [Audit Policy Configuration](https://github.com/openshift/enhancements/blob/master/enhancements/kube-apiserver/audit-policy.md#logging-of-token-with-secure-oauth-storage).

This approach has a couple of drawbacks:
1. Login events are captured via creation events of `oauthaccesstokens` in audit logs. This includes successful login events only, but not all login failures. Currently the only login failure that can be deduced from the audit logs is the inability of exchanging the oauthauthorizetoken for an oauthaccesstoken as we log creation of both of these objects.
2. Logout events are captured via deletion events of `oauthaccesstokens` in audit logs. Here only references to the sha256 based access token are present in audit logs, but no additional information about the underlying username who logged out.

## Motivation

### Goals

1. Add capabilities in oauth-server to create audit-like logs. Gate fine grained audit log events via a simple profile configuration mechanism.
2. Extend oauth-server to capture login and login failure events.
3. Extend oauth-apiserver to add user information when deleting `oauthaccesstoken` objects.
4. Extend must-gather/gather_audit_logs to fetch oauth-server audit logs.

### Non-Goals

1. Hook into the existing API server [policy configuration](https://github.com/openshift/enhancements/blob/master/enhancements/kube-apiserver/audit-policy.md).
2. Provide a fully featured audit subsystem in oauth-server compliant with API server policy framework.
3. Rate limiting of logging login/logout events.

## Proposal

We propose to add an `audit` struct to `oauth.config.openshift.io/v1` with a `profile` field which configures audit logging granularity.

```go
// OAuthSpec contains desired cluster auth configuration
type OAuthSpec struct {
	// identityProviders is an ordered list of ways for a user to identify themselves.
	// When this list is empty, no identities are provisioned for users.
	// +optional
	// +listType=atomic
	IdentityProviders []IdentityProvider `json:"identityProviders,omitempty"`

	// tokenConfig contains options for authorization and access tokens
	TokenConfig TokenConfig `json:"tokenConfig"`

	// templates allow you to customize pages like the login page.
	// +optional
	Templates OAuthTemplates `json:"templates"`
	// audit specifies what should be audited in the context of OAuthServer.
	// +optional
	// +kubebuilder:default:={"profile":"WriteLoginEvents"}
	Audit OAuthAudit `json:"audit"`
}

// OAuthAudit specifies the Audit profile in use.
type OAuthAudit struct {
	// profile is a simple drop in profile type that can be turned off by
	// setting it to "None" or it can be turned on by setting it to
	// "WriteLoginEvents".
	// +kubebuilder:default:="WriteLoginEvents"
	Profile OAuthAuditProfileType `json:"profile,omitempty"`
}

// OAuthAuditProfileType defines a simple audit profile, which can turn OAuth
// authentication audit logging on or off.
// +kubebuilder:validation:Enum=None;WriteLoginEvents
type OAuthAuditProfileType string

const (
	// "None" disables audit logs.
	OAuthNoneAuditProfileType AuditProfileType = "None"

	// "WriteLoginEvents" logs login and login failure events.
	// This is the default.
	OAuthWriteLoginEventsProfileType AuditProfileType = "WriteLoginEvents"
)
```

We propose to add additional information that helps to identify login and login failures to the audit log.

In case that the authentication happens through the `oauth-server`, we suggest to add:

- `authentication.openshift.io/username`, which is the username for the authentication attempt.
- `authentication.openshift.io/decision`, which is an enum that can be `allow`, `deny` or `error`.

An audit event for an unsuccesful authentication event would look like so:

```JavaScript
{
  "kind": "Event",
  "apiVersion": "audit.k8s.io/v1",
  "level": "RequestResponse",
  "auditID": "${ auditID }",
  "stage": "ResponseComplete",
  "requestURI": "${ requestURI }",
  "verb": "get",
  "user": {
    "username": "system:anonymous",
    "groups": [
      "system:unauthenticated"
    ]
  },
  "sourceIPs": [
    "${ sourceIP1 }"
  ],
  "userAgent": "Go-http-client/1.1",
  "responseStatus": {
    "metadata": {},
    "message": "Authentication failed, attempted: basic",
    "code": 401
  },
  "requestReceivedTimestamp": "2021-11-29T13:32:05.798968Z",
  "stageTimestamp": "2021-11-29T13:32:05.805280Z",
  "annotations": {
    "authorization.k8s.io/decision": "allow",
    "authorization.k8s.io/reason": "",
    "authentication.openshift.io/username": "kostrows",
    "authentication.openshift.io/decision": "deny",
  }
}
```

An audit event for a successful authentication event would look like so:

```JavaScript
{
  "kind": "Event",
  "apiVersion": "audit.k8s.io/v1",
  "level": "RequestResponse",
  "auditID": "${ auditID }",
  "stage": "ResponseComplete",
  "requestURI": "${ requestURI }",
  "verb": "get",
  "user": {
    "username": "system:anonymous",
    "groups": [
      "system:unauthenticated"
    ]
  },
  "sourceIPs": [
    "${ sourceIP1 }"
  ],
  "userAgent": "Go-http-client/1.1",
  "responseStatus": {
    "metadata": {},
    "code": 302
  },
  "requestReceivedTimestamp": "2021-11-29T13:26:53.395635Z",
  "stageTimestamp": "2021-11-29T13:26:53.550445Z",
  "annotations": {
    "authorization.k8s.io/decision": "allow",
    "authorization.k8s.io/reason": "",
    "authentication.openshift.io/username": "kostrows",
    "authentication.openshift.io/decision": "allow",
  }
}
```

An audit event for an authentication event that failed in the process would look like so:

```JavaScript
{
  "kind": "Event",
  "apiVersion": "audit.k8s.io/v1",
  "level": "RequestResponse",
  "auditID": "${ auditID }",
  "stage": "ResponseComplete",
  "requestURI": "${ requestURI }",
  "verb": "get",
  "user": {
    "username": "system:anonymous",
    "groups": [
      "system:unauthenticated"
    ]
  },
  "sourceIPs": [
    "${ sourceIP1 }"
  ],
  "userAgent": "Go-http-client/1.1",
  "responseStatus": {
    "metadata": {},
    "message": "Authentication failed, attempted: basic",
    "code": 400
  },
  "requestReceivedTimestamp": "2021-11-29T13:32:05.798968Z",
  "stageTimestamp": "2021-11-29T13:32:05.805280Z",
  "annotations": {
    "authorization.k8s.io/decision": "allow",
    "authorization.k8s.io/reason": "",
    "authentication.openshift.io/username": "kostrows",
    "authentication.openshift.io/decision": "error",
  }
}
```

### User Stories

#### As an administrator I want to inspect successful login and login failure attempts

Use `oc adm must-gather -- /usr/bin/gather_audit_logs` and inspect oauth-server audit logs.

#### As an administrator I want to inspect logout events and the deleted user

Use `oc adm must-gather -- /usr/bin/gather_audit_logs` and inspect apiserver audit logs for deleted `oauthaccesstoken` resources.
Audit log entries include the deleted user.

#### As an administrator I want to inspect the source IPs of the client triggering an authentication event

Use `oc adm must-gather -- /usr/bin/gather_audit_logs` and inspect oauth-server audit logs for auth events and inspect the IP within the event.

### API Extensions

We introduce a new property called `audit` to the `oauth.config.openshift.io` type.

This enhancement is not expected to affect any other team or compoment outside of auth.

### Operational Aspects of API Extensions

There is no explicit operational involvement expected. The property is optional and set by default to log login events.

It could be turned off by an admin, by setting the `audit.profile` to `None`.

#### Failure Modes

N/A

#### Support Procedures

If the `audit` can't be set in the CR, support would need to file a bugzilla.
If there are no audit events for login or login failures, verify that the `audit` values is set in the `oauth` CR.
If the `audit` is set to `profile: "WriteLoginEvents"` and there are no audit events for the oauth-server after the login or login failure events, support would need to file a bugzilla.
If there is no returning of logout events, something seems to be off with the logging of the deletion of the `oauthaccesstoken` and needs to be investigated in that direction. The logging of the logout event was not added, just extended in this enhancement.

### Risks and Mitigations

There are risks in quality and quantity. In quality means that there could be audit events that are non compliant to be logged. In quantity means amount of traffic (DoS mitigation) and information to store (disk capacity).

#### DoS mitigation

To prevent flooding audit event logs we rely on the existing default apiserver handler chain which is used to serve oauth-server http resources.

Currently, the default configuration of apiserver's `MaxRequestsInFlight` (i.e. GET requests) and `MaxMutatingRequestsInFlight` (i.e. POST requests) are used as part of the [default configuration](https://github.com/openshift/oauth-server/blob/961295a3151da3fff9daa5ea0ab8c0bf92a7d7e6/vendor/k8s.io/apiserver/pkg/server/config.go#L330-L331) to protect against audit event flooding.

#### Disk Capacity

The configuration of the logging is up to the customer and can be looked up at [cluster logging documentation of OpenShift](https://docs.openshift.com/container-platform/4.9/logging/cluster-logging-release-notes.html).

There are options to set up a suitable retention for a given set of resources. Classic strategies for logging are e.g. to rotate files for a certain time or until they hit a certain size and then get compressed.

## Design Details

### Successful Login and Login failure events

To create successful login and login failure audit events differ slightly depending on the underlying identity provider. There are the following cases to consider:

1. **Password-based identity providers**

All password-based identity providers are using a [central location](https://github.com/openshift/oauth-server/blob/690499e76a0b242adb5ccc73f23cccc8a50b8788/pkg/server/login/login.go#L173) where login events are handled. Both login success and failure events will be emitted here.

2. **External OAuth identity providers**

For login events external OAuth providers must invoke oauth-server callback handler to finalize authorization using [code grant flow](https://datatracker.ietf.org/doc/html/rfc6749#section-4.1).

Login success will be emitted upon successful code exchange between the oauth-server and the OIDC IdP in the [central OAuth callback handler](https://github.com/openshift/oauth-server/blob/822478f88514a80f053d9f65cd689d981b6ea4fd/pkg/oauth/external/handler.go#L149-L157).

Login failure however cannot be easily detected as there is no guarantee that external oauth providers will invoke callback in the case of a login failure. Hence, only best-effort login failure events can be emitted, if the external OAuth provider provides an `error` response in the [central `HandleRequest` handler](https://github.com/openshift/oauth-server/blob/513a8bb5b6cbd5e8faacbed523eb861aa29a674b/vendor/github.com/RangelReale/osincli/authorize.go#L84).

3. **External OAuth identity providers used as challengers**

External oauth identity providers used as challengers use a [central location](https://github.com/openshift/oauth-server/blob/822478f88514a80f053d9f65cd689d981b6ea4fd/pkg/oauth/external/handler.go#L118) where login and login failures audit events will be emitted.

4. **Request Header identity provider**

For this case an established trust to the external login proxy exists via means of client certificate trust. This implies that all authentication requests, are assumed to have passed successful the authentication in the external 3rd party proxy.

Thus, successful Login events can easily be constructed by creating audit events in the [central request authentication method](https://github.com/openshift/oauth-server/blob/4e63f0f350f28171edc4ae2553c7b852e65efbc6/pkg/authenticator/request/headerrequest/requestheader.go#L34).

Login failure events, however never cause a request header authentication request to be generated in the 3rd party proxy, thus those events cannot be detected in oauth-server for this use case.

### Logout events

#### oauth-server

Logout events are not visible to oauth-server as the infrastructure in OpenShift does not have a central logout handler mechanism. Instead we have to rely on audit events against deletion of `oauthaccesstoken` resources.

#### oauth-apiserver

The deletion of `oauthaccesstoken` is logged within the `oauth-apiserver`. Currently, only a `Status` object is created in an deletion event entry of the following form:

```json
{
  "kind": "Event",
  "apiVersion": "audit.k8s.io/v1",
  "level": "RequestResponse",
...
  "verb": "delete",
...
  "responseObject": {
    "kind": "Status",
    "apiVersion": "v1",
    "metadata": {},
    "status": "Success",
    "details": {
      "name": "sha256~sometoken",
      "group": "oauth.openshift.io",
      "kind": "oauthaccesstokens",
      "uid": "444845f0-1348-4ab1-95be-d0644a6bd11c"
    }
  },
...
}
```

To log the complete object we have to set `ReturnDeletedObject: true` on the oauth-apiserver REST Store: https://github.com/openshift/oauth-apiserver/blob/6e0f92194d5a25728c826fefb2d99c5e88ebb5e5/pkg/oauth/apiserver/registry/oauthaccesstoken/etcd/etcd.go#L30-L46.

This will allow to introspect the deleted user:

```json
{
   "kind": "Event",
   "apiVersion": "audit.k8s.io/v1",
   "level": "RequestResponse",
...
    "verb": "delete",
...
    "responseObject": {
      "kind": "OAuthAccessToken",
      "apiVersion": "oauth.openshift.io/v1",
      "metadata": {
...
        "userName": "exampleuser",
      },
      "userUID": "123",
      "authorizeToken": "sha256~sometoken"
    },
...
  }
```

### Protecting sensitive data

The request URI in the OAuth2 flows contain a lot of information and therefore values of query parameters that are not an allow list will be masked. The allow list will be hardcoded and won't be configurable.

For some OAuth2 flows, which are not regarded as best pratice, the exposed information could enable an attacker to try to impersonate a user.

If it would be possible to configure that setting, an attacker could try to add sensible information to the allow list.

### Verbosity

The audit events are based on the audit framework given by kubernetes. The configuration for the verbosity should be set on the `ResponseComplete` stage and the `Metadata` level.

The `username` is present on `RequestReceived`, but the decision happens during the response itself. So the logging stage is set to `ResponseComplete`.

The focus is on who tried to authenticate and did it work, which is answered on `Metadata` level. It is not necessary and probably not compliant to log the body.

### Event construction

In all cases above an [Info](https://github.com/openshift/oauth-server/blob/961295a3151da3fff9daa5ea0ab8c0bf92a7d7e6/vendor/k8s.io/apiserver/pkg/authentication/user/user.go#L20) entity is available with user information which can be used to fill audit event data.

### Integration in must-gather

The existing `oc adm must-gather -- /usr/bin/gather_audit_logs` tool will also fetch oauth-server audit logs.

### Graduation Criteria

This feature is planned for OpenShift 4.10.

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

As we are introducing a new property with default values, there are not expected risks for upgrading.

Downgrading would not cause any data loss or inconsistencies. The oauth-apiserver will just prune the fields in the request. The data itself will stay in etcd and will not be rewritten.

### Version Skew Strategy

All components will be developed with the concern of version skew in mind. So they will handle the missing of the `Audit` property.

### Test Plan

Ideally we test our feature with:

- unit tests,
- e2e tests in origin and
- tests at must-gather.

#### E2E testing steps

In order to test the audit logging as mentioned in the proposal:

**Verify audit logging for login**

1. Create a cluster with OpenShift installed on it.
2. Verify that the version deployed contains this enhancement.
3. Verify that the custom resource `oauths.config.openshift.io` exists and contains the properties:

```yaml
audit:
    profile: WriteLoginEvents
```

4. Create Login failures and Login.
5. Collect the audit logs [how to view audit logs](https://docs.openshift.com/container-platform/4.9/security/audit-log-view.html#nodes-nodes-audit-log-basic-viewing_audit-log-view)
6. Verify that the gathered logs contain logs from `oauth-server`.
7. Verify that the events caused in `4.` are logged.

**Verify audit logging can be turned off for login**

1. Create a cluster with OpenShift installed on it.
2. Verify that the version deployed contains this enhancement.
3. Verify that the custom resource `OAuth` exists and contains the properties:

```yaml
audit:
    profile: WriteLoginEvents
```
4. Edit the yaml and replace `WriteLoginEvents` with `None`.
5. Create Login failures and Login events.
6. Collect the audit logs [how to view audit logs](https://docs.openshift.com/container-platform/4.9/security/audit-log-view.html#nodes-nodes-audit-log-basic-viewing_audit-log-view)
7. Verify that there is no audit logging from the `oaut-server`.

**Verify audit logging for logout**

1. Create logout event.
2. Collect the audit logs [how to view audit logs](https://docs.openshift.com/container-platform/4.9/security/audit-log-view.html#nodes-nodes-audit-log-basic-viewing_audit-log-view)
3. Verify that the gathered logs contain logs from `oauth-apiserver`.
4. Verify that the logout event is logged and contains more details like user id and IP of the client that triggered the logout.


## Implementation History

N/A

## Drawbacks

We create more log entries, which increases the space that the logs will occupy.

## Alternatives

The alternative is to keep on relying on `oauthaccesstoken` audit events with the aforementioned drawbacks.
