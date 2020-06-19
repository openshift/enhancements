---
title: configuring-webhook-token-authenticators
authors:
  - "@stlaz"
reviewers:
  - "@sttts"
  - "@deads2k"
approvers:
  - "@deads2k"
  - "@sttts"
  - "@mfojtik"
creation-date: 2019-12-10
last-updated: 2019-12-10
status: implementable
see-also:
  - "[oauth-apiserver enhancement](https://github.com/openshift/enhancements/pull/75)"
replaces:
  - ""
superseded-by:
  - ""
---

# Configuring Webhook Token Authenticators

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

## Summary

To access the API with a token issued by a 3rd party, the kubernetes API server allows to set a
webhook authenticator that uses the `tokenreviews.authentication.k8s.io` API to check validity
of the user-supplied token. The aim of this enhancement is to expose this configuration and to
use it for authentication with the integrated oauth-server.

## Motivation

Using the standard kube-apiserver extension mechanism will allow external integrations like Keycloak or
any other token-based authenticator using the Kubernetes authentication stack to integrate in a standard way.

Webhook Token Authentication is a kube-apiserver configuration feature that can be used to
configure an external authenticator which supports Kubernetes' token reviews (see
[Webhook Token Authentication](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#webhook-token-authentication) for more details).
By implementing this enhancement, we would significantly simplify any drop-in replacements
for the integrated oauth-server, which might be helpful in cases where we would like to
delegate authentication to a different OpenShift control plane, or direct authentication to an
identity provider capable of handling the Kubernetes token reviews, such as Keycloak.

### Goals

1. Make it possible to configure an endpoint capable of performing kube token reviews
2. Modify cluster-authentication-operator so that it configures authentication via the OpenShift OAuth
   server by using the `webhookTokenAuthenticator` field instead of the current patching of the
   kube-apiserver.

### Non-Goals

1. provide a partial authentication operator to any external authenticator.  Once the standard oauth-server
   is disabled, authentication configuration is *entirely* the responsibility of the new authentication stack.
2. allow both the built-in oauth-server and another authentication stack to be enabled at the same time.

## Proposal

### Implementation

#### API

`authentications.config.openshift.io/v1` already contains the `spec.webhookTokenAuthenticators`. It
expects an array of references to secrets, but upstream only allows a single authenticator. This field
will therefore be deprecated in favour of a new one, `spec.webhookTokenAuthenticator`.

The secret reference which gets set in this field has to be validated. The referenced secret will be
expected to contain a `kubeconfig` key with a valid kubeconfig configuration for the webhook. In 4.n,
it will only be possible to configure a single element for the `wehbhookTokenAuthenticator` field
and the referenced kubeconfigs will only be allowed to use the `-data` forms (e.g.
`certificate-authority-data` is allowed whereas `certificate-authority` is not) of their fields to
configure certificate authority, client certificate and key.

This path is compatible with kubernetes as it is possible to directly map the referenced
secret's contents to files and specify these in the kube-apiserver configuration.

#### Backend - kube-apiserver-operator

1. Observes any changes in the `webhookTokenAuthenticator` field (and changes to the referenced secret)
   of the `authentication/cluster` resource and performs validation of the reference in that field.
2. It pushes the webhook authenticator, iff it passes the validation to the `webhook-authenticator`
   secret in `openshift-kube-apiserver` namespace.
3. If the webhook authenticator does not pass validation, the operator keeps the previous state.
4. Redeploys the kube-apiserver static pods while specifying the new
   mounted file path in the `apiServerArguments.authentication-token-webhook-config-file`

#### Backend - Integrated OAuth Server Token Authentication

1. cluster-authentication-operator observes `authentication.config/cluster` `type` field
2. if set to "IntegratedOAuth", it creates an `openshift-config/webhook-authentication-integrated-oauth`
  secret and adds it to the `webhookTokenAuthenticator` field of the `authentication.config/cluster`
  resource. If the `type` field is set to any other value, the operator will perform no action to the
  `webhookTokenAuthenticator` field.
  - the above secret contains details that point kube-apiserver to the oauth-apiserver for token reviews
3. during an authentication flow, kube-apiserver sends `tokenreviews.authentication.k8s.io` object to the
  oauth-apiserver for review
4. oauth-apiserver sends a response accordingly to Kubernetes requirements, on success it adds the
  `system:authenticated:oauth` group to the returned details about the groups of the authenticated user
  - `system:authenticated:oauth` provides users with the ability to handle projects via the self-provisioner
    cluster role
  - similarly, if any integrator would like their users to have project privileges, they need to add this
    specific group in their token review responses.

### Common Gotchas When Providing Your Own Authentication Webhook

1. `oauth-proxy` relies on the _/.well-known/oauth-authorization-server_ endpoint
   and if the integrated oauth-server is turned off and the `spec.oauthMetadata` field
   in `authentications.config/cluster` is not set properly, things like Grafana
   and Prometheus won't be accessible
2. ServiceAccounts may not be usable as OAuth clients anymore, which would result in
  breaking authentication for 3rd party components that rely on this functionality
  (e.g the Jenkins plugin integrating with OpenShift, but also Grafana and Prometheus
  that also use service accounts as OAuth clients for oauth-proxy)

It is the sole responsibility of the replacement component to make sure all of these work.

## Design Details

### Test Plan

Since we will be making the cluster-authentication-operator use the `webhookTokenAuthenticator`
(in combination with `oauthMetadata`, which already works) to configure authentication using
the integrated OAuth server, we will just re-use all the tests that we have already.

### Upgrade / Downgrade Strategy

To keep authentication working even during upgrades, we'll keep our kube-apiserver
authenticator patch code in place for 4.n version (the first version with the new wiring),
but will disable it if any webhook authenticators are set. The authenticator patching code
should then be removed in 4.(n+1) version.

The cluster-authentication-operator in 4.n waits for the oauth-apiserver deployment
to be healthy, and after that happens, it will create the `openshift-config/webhook-authentication-integrated-oauth`
secret and configure the `webhookTokenAuthenticator` field of the `authentications.config/cluster`.

Since the kubernetes-apiserver of version 4.(n-1) does not honor webhook authenticator configuration,
downgrade should not cause any issues.

### Version Skew Strategy

See the upgrade/downgrade strategy.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

An alternative to having users create their own kubeconfigs is to have a CRD that takes
care of any copy-pasting that would otherwise have to be done manually

```
apiVersion: config.openshift.io/v1
kind: TokenAuthenticator
spec:
  ca: CM reference
  server: string (valid https:// URL)
  clientCertificate: CM reference
  clientKey: Secret reference
```
