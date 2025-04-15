---
title: adding-uid-and-extra-claim-mapping-configuration-options-for-external-oidc
authors:
  - everettraven
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - liouk # Original author of the ExternalOIDC feature for OpenShift
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - sjenning
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - JoelSpeed
creation-date: 2025-04-14
last-updated: 2025-04-14
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CNTRLPLANE-127
see-also:
  - "/enhancements/authentication/direct-external-oidc-provider.md"
replaces:
  - none
superseded-by:
  - none
---

# Adding `uid` and `extra` claim mapping configuration options for external OIDC

## Summary

Adds the `uid` and `extra` fields to the `authentications.config.openshift.io` CRD under the `.spec.oidcProviders[].claimMappings` field.
Adding these configuration options brings us closer to parity with the configuration options of the upstream structured authentication configuration claim mapping options
as outlined in https://kubernetes.io/docs/reference/access-authn-authz/authentication/#using-authentication-configuration.
This enables usage of functionality not previously possible with OpenShift and HyperShift's existing configuration options such as custom authorizers
that may rely on these values being set.

## Motivation

### User Stories

* As an OpenShift/HyperShift cluster administration, I want to configure the UID and Extra values assigned to a cluster user identity based
on the authentication token used, so that I can use a combination of an external OIDC provider and third-party authorizers to enforce
finer-grained access control for my cluster(s).

### Goals

- Add support for configuring the UID of a cluster user identity, when using an external OIDC-compatible identity provider, based on JWT token claims, using both exact claim names and CEL expressions.
- Add support for configuring the Extra values of a cluster user identity, when using an external OIDC-compatible identity provider, based on JWT token claims, using both exact claim names and CEL expressions.

### Non-Goals

- Adding support to existing claim mapping configurations to use CEL expressions.
- Adding support for any additional missing authentication configuration options from upstream.
- Changing how HyperShift communicates validation failures.

## Proposal

### Workflow Description

#### Roles / Definitions
- **Cluster Administrator**: Person(s) responsible for configuring and managing a cluster.
- **Identity Provider**: An external, trusted entity for managing user identities and authentication. Examples are Microsoft's Entra ID, Okta's Auth0, Keycloak, etc.
- **Cluster User Identity**: The "identity" of a user of the cluster, containing information used to make authorization decisions.

#### Workflows

##### Scenario 1: Configuring the UID of cluster user identities
An OpenShift/HyperShift customer is using an external Identity Provider to manage user authentication in their organization. A Cluster Administrator
would like to use this existing authentication layer for authenticating users to their clusters. They have already configured this using the existing
external OIDC workflows, but would like the cluster user identities to include a UID based on the claims their Identity Provider includes in the issued
JWT authentication tokens.

To configure the UID of a cluster user identity using a specific claim value on OpenShift, a Cluster Administrator updates the `authentications.config.openshift.io/cluster` resource
to populate the claim mapping like so:
```yaml
apiVersion: config.openshift.io/v1
kind: Authentication
metadata:
  name: cluster
spec:
  type: OIDC
  oidcProviders:
  - name: foo
    claimMappings:
      uid:
       claim: email
```

To configure the UID of a cluster user identity using a CEL expression on OpenShift, a Cluster Administrator updates the `authentications.config.openshift.io/cluster` resource like so:
```yaml
apiVersion: config.openshift.io/v1
kind: Authentication
metadata:
  name: cluster
spec:
  type: OIDC
  oidcProviders:
  - name: foo
    claimMappings:
      uid:
       expression: 'claims.email'
```

To configure the UID of a cluster user identity using a specific claim value on HyperShift, a Cluster Administrator creates a `HostedCluster` resource to populate the claim mapping like so:
```yaml
apiVersion: hypershift.openshift.io/v1alpha1
kind: HostedCluster
metadata:
  name: example
  namespace: clusters
spec:
  configuration:
    authentication:
      type: OIDC
      oidcProviders:
      - name: foo
        claimMappings:
          uid:
            claim: email
```

To configure the UID of a cluster user identity using a CEL expression on HyperShift, a Cluster Administrator creates a `HostedCluster` resource to populate the claim mapping like so:

```yaml
apiVersion: hypershift.openshift.io/v1alpha1
kind: HostedCluster
metadata:
  name: example
  namespace: clusters
spec:
  configuration:
    authentication:
      type: OIDC
      oidcProviders:
      - name: foo
        claimMappings:
          uid:
            expression: claims.email
```

##### Scenario 2: Configuring the Extra values of cluster user identities
An OpenShift/HyperShift customer is using an external Identity Provider to manage user authentication in their organization. A Cluster Administrator
would like to use this existing authentication layer for authenticating users to their clusters. They have already configured this using the existing
external OIDC workflows, but would like the cluster user identities to include Extra values based on custom claims they have configured their
Identity Provider to include in the issued JWT authentication tokens. Specifically, they are using the `team` and `role` claims. They are using a
third-party authorizer to implement access control guardrails based on the combination of the team a user is part of and the role they are given.

To configure the Extra values of a cluster user identity value on OpenShift, a Cluster Administrator updates the `authentications.config.openshift.io/cluster` resource
to populate the claim mapping like so:
```yaml
apiVersion: config.openshift.io/v1
kind: Authentication
metadata:
  name: cluster
spec:
  type: OIDC
  oidcProviders:
  - name: foo
    claimMappings:
      extra:
      - key: 'myorg.io/team'
        valueExpression: claims.team
      - key: 'myorg.io/role'
        valueExpression: claims.role
```

To configure the Extra values of a cluster user identity on HyperShift, a Cluster Administrator creates a `HostedCluster` resource to populate the claim mapping like so:
```yaml
apiVersion: hypershift.openshift.io/v1alpha1
kind: HostedCluster
metadata:
  name: example
  namespace: clusters
spec:
  configuration:
    authentication:
      type: OIDC
      oidcProviders:
      - name: foo
        claimMappings:
          extra:
          - key: 'myorg.io/team'
            valueExpression: claims.team
          - key: 'myorg.io/role'
            valueExpression: claims.role
```

### API Extensions

Modifications will be made to the `authentications.config.openshift.io` CustomResourceDefinition to add the `uid` and `extra` fields under the existing `.spec.oidcProviders[].claimMappings` field.
These new fields will be optional.

Because HyperShift's `HostedCluster` and `HostedControlPlane` resources [inline the `spec` of the `Authentication` resource](https://github.com/openshift/hypershift/blob/0aae1fe62b6bfd678334bc140d2f52e3daa8eb0e/api/hypershift/v1beta1/hostedcluster_types.go#L1612-L1615), they will inherently be modified as well.

The exact changes that will be made to the existing `authentications.config.openshift.io` CRD are outlined in https://github.com/openshift/api/pull/2234

#### Adding the `uid` field
The `uid` field will be added as an optional field.
When not set, it will default to using the `sub` claim.

It will have 2 subfields:
- `uid.claim` - a string value to specify the claim whose exact value should be used as the cluster user identity's UID.
- `uid.expression` - a CEL expression that results in the string value that should be used as the cluster user identity's UID.

It will have the same validations enforced, at admission time where possible, as the Kubernetes API server enforces:
- https://github.com/kubernetes/kubernetes/blob/b15dfce6cbd0d5bbbcd6172cf7e2082f4d31055e/staging/src/k8s.io/apiserver/pkg/apis/apiserver/validation/validation.go#L327-L340
- CEL expression compilation will be enforced on HyperShift CRDs at runtime. On OpenShift, an admission webhook or plugin will be created to reject CEL expressions that won't compile at admission time.

Some additional validations may be enforced, unique to OpenShift such as:
- Requiring either `uid.claim` or `uid.expression` to be set when the `uid` field is specified.
- Enforcing minimum and maximum length restrictions

In practice, configuring these options would look something like:
```yaml
type: OIDC
oidcProviders:
- name: foo
  claimMappings:
    uid:
     claim: email
```
or

```yaml
type: OIDC
oidcProviders:
- name: foo
  claimMappings:
    uid:
     expression: claims.email
```

**NOTE:** These examples intentionally only include the `spec` fields that would need to be set to represent how it would look across all 3 APIs that would be modified.

#### Adding the `extra` field
The `extra` field will be added as an optional field.

It will have 2 subfields:
- `extra.key` - a string value to specify the key to be used for the extra attribute on the cluster user identity.
- `extra.valueExpression` - a CEL expression that results in the string value that should be used as the value for the extra attribute.

It will have the same validations enforced, at admission time where possible, as the Kubernetes API server enforces:
- https://github.com/kubernetes/kubernetes/blob/master/staging/src/k8s.io/apiserver/pkg/apis/apiserver/validation/validation.go#L345-L384
- CEL expression compilation will be enforced on HyperShift CRDs at runtime. On OpenShift, an admission webhook or plugin will be created to reject CEL expressions that won't compile at admission time.

Some additional validations may be enforced, unique to OpenShift such as:
- Enforcing minimum and maximum length restrictions
- Adding additional reserved domains and subdomains for OpenShift

In practice, configuring these options would look something like:
```yaml
type: OIDC
oidcProviders:
- name: foo
  claimMappings:
    extra:
    - key: example.org/foo
      valueExpression: claims.foo
```

**NOTE:** This example intentionally only includes the `spec` fields that would need to be set to represent how it would look across all 3 APIs that would be modified.

### Topology Considerations

#### Hypershift / Hosted Control Planes

##### Failure Mode Communications
Admission time validations will fail upon attempting to create a `HostedCluster` with invalid values specified for the new configuration options.
This will be enforced through the OpenAPI schema validations that exist on the CRD.

Runtime validation failure will result in status conditions being applied to the resources that could not be successfully reconciled due to invalid configuration options.
The Hypershift Operator and the Control Plane Operator have existing configuration validation logic that populates the `ValidConfiguration` status condition for `HostedCluster`
resources and the `ValidHostedControlPlaneConfiguration` status condition for `HostedControlPlane` resources. Any failed runtime validations will result in these
conditions being set to a status of `False` and a message containing the errors. Reconciliation is halted if configurations are invalid.

Links to source code where validation conditions exist today for each operator:
- Hypershift Operator
    - https://github.com/openshift/hypershift/blob/e513976922d8ccd5571fb10cd46a99677b676f6e/hypershift-operator/controllers/hostedcluster/hostedcluster_controller.go#L906-L922
    - https://github.com/openshift/hypershift/blob/e513976922d8ccd5571fb10cd46a99677b676f6e/hypershift-operator/controllers/hostedcluster/hostedcluster_controller.go#L1187-L1217
- Control Plane Operator
    - https://github.com/openshift/hypershift/blob/e513976922d8ccd5571fb10cd46a99677b676f6e/control-plane-operator/controllers/hostedcontrolplane/hostedcontrolplane_controller.go#L447-L463
    - https://github.com/openshift/hypershift/blob/e513976922d8ccd5571fb10cd46a99677b676f6e/control-plane-operator/controllers/hostedcontrolplane/hostedcontrolplane_controller.go#L803-L810

#### Standalone Clusters

#### Single-node Deployments or MicroShift

>How does this proposal affect the resource consumption of a
>single-node OpenShift deployment (SNO), CPU and memory?

Adding new fields that need to persist across multiple representations add minimal memory overhead.
The additional validations associated with the new fields add some, likely negligible, CPU overhead.

**MicroShift**: As far as I am aware, MicroShift does not have a configurable authentication layer and relies on Kubeconfigs only. No impact.

### Implementation Details/Notes/Constraints

Because the existing `authentications.config.openshift.io` CRD is already `v1` _and_ the existing `ExternalOIDC` feature-gate
is enabled by default on HyperShift, a new feature-gate will be added to properly go through the `TechPreviewNoUpgrade` --> `Default`
feature promotion cycle.

This new feature-gate will be named `ExternalOIDCWithUIDAndExtraClaimMappings`.

Existing implementations will need to be updated to recognize this new feature-gate and perform the appropriate translations
from OpenShift/HyperShift specific resources --> kube-apiserver recognized configuration.

### Risks and Mitigations

1. Collision with system identities that were previously impossible.
    - To my knowledge, there are no system identities in which collision will occur by allowing configuration of the UID and Extra attributes of a user identity.
2. Collision with existing reserved extra attributes that were previously impossible.
    - We will not allow users to configure extra attributes where the key uses a domain or sub-domain reserved for Kubernetes or OpenShift. Specifically, `openshift.io`, `*.openshift.io`, `kubernetes.io`, `k8s.io`, `*.kubernetes.io`, and `*.k8s.io` domains and sub-domains are not allowed in the key.

3. HyperShift only: Providing CEL expressions that won't compile will result in an invalid configuration and will prevent successful roll out of hosted clusters. This can impact future configuration changes and upgrades from rolling out successfully until the CEL expressions are fixed. There is currently no way around this for now. In the future, we _might_ be able to add a CEL compilation library that can be used via the `kubebuilder:validation:XValidation` marker to check for compilable CEL expressions, but that is not planned as part of this work.

### Drawbacks

As with anything providing users flexibility in configurations, there may be scenarios that customers have that we have not thought of or tested.
Users may expect that we support _any_ configurations possible, even if explicitly stated otherwise.
Making this change means that we inherently are choosing to provide some level of support for these cases.

## Alternatives (Not Implemented)

### Do Nothing
Doing nothing would not allow users to effectively configure their authentication and authorization flows as flexibly as upstream Kubernetes
or as offered by other vendors. Inflexibility in this configuration means locking customers into a particular authentication and authorization
pattern. This is not beneficial to customers that have a wide range of needs, especially when it comes to having existing authentication and authorization
flows they would like to use across multiple systems in their organization.

## Open Questions [optional]

N/A

## Test Plan

In general, the test plan is to have:
- Unit tests in all components that implement the business logic associated with this feature.
- E2E tests in all components that implement the business logic associated with this feature.
- Periodic jobs that run with a configuration that is expected to be fairly common in place, ensuring that the configuration of these new fields don't break other components.
- Integration tests for all API changes associated with this feature.

## Graduation Criteria

### Pre-requisites
There is currently no existing testing in place for the `ExternalOIDC` feature.
In order for this feature to graduate and implement tests, tests must first be retroactively
implemented for the original `ExternalOIDC` feature.

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

OpenShift:
- Unit testing
- Integration testing
- E2E testing
- Periodic testing
- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

HyperShift:
- Unit testing
- Integration testing
- E2E testing
- Periodic testing
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

This is an opt-in feature upon upgrading. When opting in, a user will:
- Configure the `HostedCluster` at creation/update time for HyperShift
- Configure the `Authentication` resource after upgrading for OpenShift

When performing a downgrade, a user may need to manually remove the new configuration options they have configured as a previous release
will not support these configuration options.

## Version Skew Strategy

The cluster-authentication-operator in OpenShift can effectively handle the version skew of the kube-apiserver during
an upgrade. The functionality exposed by the feature has existed on the kube-apiserver for some time, meaning that we shouldn't
encounter a state during an upgrade where this feature is configured and the configuration is not recognized by the Kubernetes API server.

For more information on how the existing `ExternalOIDC` feature, that this changes layers on top of, handles the version skew see https://github.com/openshift/enhancements/blob/master/enhancements/authentication/direct-external-oidc-provider.md#version-skew-strategy

## Operational Aspects of API Extensions

Potential failure modes are:
- Admission time failure for OpenShift and HyperShift
- Runtime failures for HyperShift

Admission time failures will be made immediately clear to users on create/update requests for the resources.

Runtime failures on HyperShift will be presented to users in the same way HyperShift currently presents runtime validation errors to users.

Failure of the extension on HyperShift will prevent successful rollout of a hosted cluster.
Failure of the extension on OpenShift should not cause negative impacts to the cluster as no configuration changes roll out until the configuration provided is deemed valid.
In the event an invalid configuration makes its way through on OpenShift, there is a break-glass workaround of using a Kubeconfig to authenticate with the kube-apiserver.

If the admission plugin/webhook on OpenShift is down, there will be runtime validations in place as a back-up to prevent uncompileable CEL expressions from being used.
If the runtime validations find problems on OpenShift, the cluster-authentication-operator will go `Degraded`.

## Support Procedures

If the admission webhook for OpenShift is not running, the cluster-authentication-operator will set a `Degraded` status condition. Additionally, the kube-apiserver will
output logs that signal the admission webhook could not be contacted. A must-gather can be used to diagnose.

Users will need to provide error messages they receive for admission time validation failures when submitting bug reports.

For runtime validation failures, status conditions will be populated.

To disable this feature, disable the `ExternalOIDCWithUIDAndExtraClaimMappings` feature-gate. Disabling this feature means that users will no longer be able to configure
the `uid` and `extra` attributes for cluster user identities. If they have authorization flows in place that rely on these attributes for making decisions, users may have a degraded
experience and access controls may not be enforced as expected.

If managed services needs to make updates to a Hypershift guest cluster, user-defined configurations must be valid. This is a pre-existing issue and will not be solved as part of
this work. Separate work should be done to improve how guest clusters can be managed in tandem with user-defined configurations.

## Infrastructure Needed [optional]
N/A
