---
title: bound-service-account-tokens
authors:
  - "@marun"
reviewers:
  - "@deads2k"
  - "@mfojtik"
  - "@stlaz"
  - "@sttts"
approvers:
  - "@deads2k"
  - "@sttts"
creation-date: 2019-11-28
last-updated: 2020-01-21
status: implementable
see-also:
  - "https://github.com/kubernetes/community/blob/master/contributors/design-proposals/auth/bound-service-account-tokens.md"
  - "https://docs.google.com/document/d/1XcOsEv4jO9P1QQHn-tOnC80oMyCm85hGA6LqHRfjTgo/edit?ts=5ddb86c1"
  - "https://aws.amazon.com/blogs/opensource/introducing-fine-grained-iam-roles-service-accounts/"
  - "https://thenewstack.io/no-more-forever-tokens-changes-in-identity-management-for-kubernetes/"
  - "https://jpweber.io/blog/a-look-at-tokenrequest-api/"
replaces:
superseded-by:
---

# Bound Service Account Tokens

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

- Is it necessary/desirable to allow `service-account-max-token-expiration` to be configured?

- If I were a customer with multiple clusters and wanted AWS IAM integration with all of
  them, would I want to use a different issuer for each of them or reuse the same issuer?

- If reusing the same issuer, would I want to share the bound token keypair across clusters
  or supply the public keys of multiple keypairs?

## Summary

Enable the optional use of bound service account tokens [via volume
projection](https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#service-account-token-volume-projection)
and the [TokenRequest
API](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.16/#tokenrequest-v1-authentication-k8s-io).

## Motivation

3rd party IAM components such as the [AWS pod identity
webhook](https://github.com/aws/amazon-eks-pod-identity-webhook) require bound tokens to
be able to identify pods without the use of workarounds such as proxies. It is also
generally desirable to support bound tokens to limit the scope of permissions (and
therefore risk of compromise) for a given service account token.

### Goals

1. A bound token can be acquired via the TokenRequest API.
2. A pod can request a bound service account token via a projected volume.
3. The public key used to validate bound tokens can be retrieved for use with 3rd party IAM
  components.

### Non-Goals

1. Integrate or ship the aws pod identity webhook
2. Convert existing token-consuming components to use bound tokens

## Proposal

- Support configuring the options (via `KubeAPIServerConfig`) that will enable bound
  tokens.
  - The `TokenRequest` and `TokenRequestProjection` feature gates are enabled by default
    in kube > 1.12, but are only configured by the apiserver if the following options are
    provided:
    - service-account-signing-key-file
      - Operator should set this to the path of the private key it manages
    - service-account-issuer
      - Operator should default this to `https://kubernetes.default.svc` and it should be possible to
        override it. When it is overridden, tokens from the previous issuer will no longer
        validate and it may be necessary to restart affected pods.
    - api-audiences
      - Operator should set this to the issuer.
      - Bound tokens submitted to the apiserver must specify one or more of the audiences
        configured here. If a request for a bound token does not specify an audience, the
        audience will be defaulted to the issuer.
  - Enabling bound tokens will satisfy goals #1 and #2.

- The apiserver operator should manage a keypair used to sign and verify bound tokens
  - An RSA keypair will be used. The trust model for digital signatures (via JWT) is
    different than for securing web communciation, and no explicit provision for expiry
    or revocation should be necessary.
  - The keypair needs to be distinct from the keypair used for legacy service account
    tokens. Reusing the legacy token keypair would prevent token validation since tokens
    are first validated by key and then by content. If the key were to validate, the
    subsequent content check would fail due to differences in the content of legacy and
    bound tokens.
  - The keypair should be written to a secret in the `openshift-kube-apiserver` namespace
  - The public key should be added to (rather than replaced in) a configmap in the
    `openshift-kube-apiserver` namespace.
    - Public keys will be added to the configmap with keys of the form
      `service-account-xxx.pub`, where `xxx` is incremented for uniqueness.
    - This configmap can be used to source the public keys needed by 3rd party components
      to verify bound tokens issued by the apiserver, satisfying goal #3.
    - The path of the mounted configmap should be included in
      `KubeAPIServerConfig.ServiceAccountPublicKeyFiles` to configure the apiserver. The
      path will be automatically translated into the list of filenames in that path
      during conversion from openshift configuration to apiserver configuration.
    - If it is necessary to invalidate previous tokens, deletion of the configmap will
      ensure recreation with only the current public key.
    - The configmap will be automatically copied to `openshift-config-managed` to expose
      the public keys to consumers other than the operator.
  - In the event that the keypair secret is deleted, a new keypair will be generated but
    tokens signed by the previous private key will continue to validate since the
    corresponding public key will still be used to validate bound tokens.

### Implementation Details/Notes/Constraints

Bound tokens are not currently required until bootstrap is complete, so it should be
reasonable to delay support of bound tokens until the post-bootstrap phase.

### Risks and Mitigations

Configuring a cluster to support `TokenRequest` and `TokenRequestProjection` should have
no impact on the usage of legacy tokens. Existing service account tokens will continue to
work as before, and bound tokens will be supported for those that explicitly request
them. Bound tokens only replace legacy tokens if the `BoundServiceAccountTokenVolume`
feature gate is enabled. and it is disabled by default. Existing tests passing should be
a good indication that legacy token usage is unaffected.

## Design Details

### Test Plan

There is already comprehensive test coverage of the bound token feature in upstream
kube. It should therefore be reasonable to limit test coverage to indications that bound
token usage has been properly configured:
 - Validating that a bound token can be requested via the TokenRequest API
 - Validating that a pod can request a bound token via volume projection

The key management proposed by this enhancement will also need to be tested:
 - The absence of the bound token secret should result in creation of the secret with a new keypair
 - The absence of the bound token configmap should result in creation of the configmap
   populated with the public key of the current keypair

It probably makes sense to manually verify integration with the AWS pod identity webhook,
since this is the primary motivation for this enhancement. It may make sense to delegate
this testing to those that will be deploying the webhook for customers.

### Graduation Criteria

Being delivered as GA in 4.4

### Upgrade / Downgrade Strategy

The change as proposed is additive-only, so upgrading will enable bound tokens and
downgrading would disable them.

### Version Skew Strategy

N/A

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

Other integration options for 3rd party IAM exist, but the complexity involved in
deploying and maintaining them is reportedly considerable.
