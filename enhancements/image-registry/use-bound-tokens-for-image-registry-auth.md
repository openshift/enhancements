---
title: use-bound-tokens-for-image-registry-auth
authors:
  - @sanchezl
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @deads2k
  - @adambkaplan
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - @deads2k
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - @deads2k
creation-date: 2023-02-13
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/API-1644
see-also:
  - TBD
replaces:
  - TBD
superseded-by:
  - TBD
---

# Use Bound Tokens for Integrated Image Registry Authentication

## Summary

Use bound service account tokens to generate image pull secrets for pulling from the integrated
image registry. Instead of creating the secrets needed to create long-lived service account tokens,
bound service account tokens are generated directly via the TokenRequest API. Using the TokenRequest
API will reduce the number of secrets and improve the security posture of a cluster.

## Motivation

With the release of OpenShift 4.11, the upstream Kubernetes feature gate
`LegacyServiceAccountTokenNoAutoGeneration` was enabled. This feature gate was anticipated by
OpenShift administrators as a means to halt the automatic generation of service account token
secrets for every service account in the cluster. However, despite the feature gate being enabled, a
service account token secret containing a long-lived token was still generated for every service
account in the cluster. As long-lived tokens represent a higher risk to information security than
time-bound tokens, it would be ideal minimize thier use in a cluster.

### User Stories

- As an OpenShift administrator, I do not want legacy, long-lived service account tokens to be
  auto-generated in my cluster for the purposes of authenticating with the integrated image
  registry, so that my cluster can benefit from the improved security posture of using bound tokens.

### Goals

- Managed image pull secrets, generated for authenticating with the integrated image registry, will
  use bound service account tokens.
- Legacy service account token secrets, for use in creating the image pull secrets for the
  integrated image registry, will not be generated.
- Make it easy to find and delete legacy service account token secrets.

### Non-Goals

- Automatically migrating a managed image pull secret's authentication from a long-lived to bound
  token.
- Automatically cleaning up previously generated service account token secrets.

## Proposal

### Workflow Description

#### New Cluster

There are no steps to take on a new cluster install. If the `ImageRegistry` capability is enabled,
and the integrated image registry is enabled, the image pull secrets will be managed by the
controller without any explicit interactions by the cluster administrator.

```gherkin
Scenario: New cluster created
  Given a freshly install cluster
  Then no legacy service account token secrets are generated
  And the managed image pull secrets generated contain bound tokens.
```

#### Upgraded Cluster

When upgrading a cluster where the `ImageRegistry` capability is enabled, and the integrated image
registry is enabled, existing legacy managed token secrets and image pull secrets will not be
automatically migrated.

```gherkin=
Background:
    Given an existing cluster version N-1
    And the ImageRegistry capability is enabled
    And the integrated image registry is enabled
    
Scenario: Existing managed secrets
    And the cluster is upgrated to version N
    Then no legacy managed service account tokens are deleted
    And no legacy managed integrated image registry pull secrets are updated

Scenario: New service accounts created after upgrade
   When the cluster is upgrated to version N
   And a new service account is created
   Then a new managed integrated image pull secret will be generated
   And the auth values will be bound service account tokens
   And no legacy service account token secrets will be generated
```

##### Migrating Legacy Image Pull Secrets

Deleting an existing legacy managed service account token secret or image pull secret will result in
the generation of a new managed image pull secret generated with a bound service account token.

```gherkin
Background:
    Given an existing cluster version N-1
    And the ImageRegistry capability is enabled
    And the integrated image registry is enabled
    And the cluster is upgrated to version N
    
Scenario: Deleting a legacy managed image pull secret
    When a legacy managed image pull secret is deleted
    Then the corresponding legacy managed service account token secret will be deleted
    And the managed integrated image pull secret will be re-generated
    And the auth values will be bound service account tokens

Scenario: Deleting a legacy managed service account token secret
    When a legacy managed service account token secret is deleted
    Then the corresponding legacy managed image pull secret will be deleted
    And the managed integrated image pull secret will be re-generated
    And the auth values will be bound service account tokens

```

### API Extensions

#### Annotations

- Service Accounts
  - `openshift.io/internal-registry-pull-secret-ref` references managed image pull secret
- Image Pull Secrets
  - `openshift.io/internal-registry-auth-token.service-acount` references service account for image
    pull secret.
  - `openshift.io/internal-registry-auth-token.binding` set to `legacy` or `bound`

#### Labels

- Service Account Token Secrets (Legacy)
  - `openshift.io/legacy-token` handy selector.

#### Finalizers

- Image Pull Secrets
  - `openshift.io/legacy-token` to ensure legacy service account token secret are also deleted.

### Risks and Mitigations

There is a risk that a existing OCP user is using the service account API tokens for other purposes.
We attempt to mitigate this somewhat by forcing a user to explicitly delete an existing legacy
service account API token secret before the controller takes over and starts managing the image pull
secret without the legacy service account API token.

### Drawbacks

## Design Details

### Controller Loops

- Service Account Controller:

  1. Add `openshift.io/internal-registry-pull-secret-ref` annotation to every service account.

     - Initially set to legacy managed image pull secret name if it exist, otherwise a new name is
       generated.

  2. Creates managed image pull secret if it does not exist.

     - Initialized with `openshift.io/internal-registry-auth-token.service-acount` annotation.
     - Initialized with empty auth data. The managed image pull secret controller will fill in
       later.

  3. Adds the managed image pull secret to `ServiceAccount.ImagePullSecrets` field.

     - This is only done once the managed image pull secret has auth data, so it is not used
       beforhand.

  4. Cleans up legacy managed image pull secret from the `ServiceAccount.Secrets` field.

     - We do not expect image pull secrets to be mounted into a pod.

- Image Pull Secret Controller

  1. Initialized with `openshift.io/internal-registry-auth-token.binding=bound` annotation.
  2. Generates a `dockercfg` format image pull authentication data if:

     - Data is corrupt.
     - List of registries is incorrect.
     - Service account token has expired, or is about to expire.
       - Token is considered "about to expire" if expiration is less than 11 minutes away.
     - Service account token signer has changed.
       - Service account signer found in `bound-service-account-signing-key` secret in the
         `openshift-kube-apiserver` namespace.

- Legacy Service Account Token Secret Controller

  1. Add `openshift.io/legacy-token` label to the secret.

     - This enables administrators to quickly find all existing legacy managed service account
       tokens.

- Legacy Image Pull Secret Controller

  1. Add `openshift.io/internal-registry-auth-token.binding=legacy` annotation.
  2. Add `openshift.io/legacy-token` finalizer.
  3. On deletion of legacy image pull secret:
     1. Delete corresponding legacy managed service account token secret.
     2. Remove `openshift.io/legacy-token` finalizer.

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

## Open Questions

1. What is the source of truth for the bound service account token signer certificate?

## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in
  [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

### Upgrade

On upgrade, see [above](#upgraded-cluster).

### Downgrade

Preliminary reviews suggests the need to patch the previous version such that it recovers the
original functionality after a downgraded.

## Version Skew Strategy

## Operational Aspects of API Extensions

## Failure Modes

## Support Procedures

## Implementation History

## Alternatives

## Infrastructure Needed \[optional\]
