---
title: external-oidc-multiple-idp-support
authors:
  - everettraven
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - liouk # Original author of the ExternalOIDC feature for OpenShift
  - jhadvig # Console SME
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - sjenning
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - JoelSpeed
creation-date: 2025-09-30
last-updated: 2025-09-30
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CNTRLPLANE-1458
see-also:
  - "/enhancements/authentication/direct-external-oidc-provider.md"
replaces:
  - none
superseded-by:
  - none
---

# External OIDC Multiple IdP Support

## Summary

Allow users to configure more than one active OIDC identity provider when using the BYO External OIDC feature.

## Motivation

### User Stories

- As a cluster administrator, I would like to enable multiple different active identity providers so that subsets of my cluster users can use different login methods.

### Goals

- Add support for configuring more than one active external OIDC provider.

### Non-Goals

- Enabling "user profiles" that would allow a user to switch between "profiles" logged in using a different IdP.

## Proposal

### Workflow Description

#### Roles / Definitions
- **Cluster Administrator**: Person(s) responsible for configuring and managing a cluster.
- **Identity Provider**: An external, trusted entity for managing user identities and authentication. Examples are Microsoft's Entra ID, Okta's Auth0, Keycloak, etc.
- **Cluster User Identity**: The "identity" of a user of the cluster, containing information used to make authorization decisions.

#### Workflows

##### Scenario 1: Configuring more than one external identity provider
An OpenShift/HyperShift customer is using two external identity providers, Keycloak and Microsoft EntraID, to manage user authentication in their various organizations.
A Cluster Administrator would like to make it possible for all employees in the various organizations to authenticate with the cluster using the identity provider their organizations use for day-to-day
operations / org specific systems.

To configure the OpenShift Kubernetes API server to use both of these external identity providers, a Cluster Administrator updates the `authentications.config.openshift.io/cluster` resource
like so:
```yaml
apiVersion: config.openshift.io/v1
kind: Authentication
metadata:
  name: cluster
spec:
  type: OIDC
  oidcProviders:
  - name: 'orgA-keycloak'
    issuer:
      audiences:
      - openshift-console
      - oc-cli
      issuerCertificateAuthority:
        name: orgA-keycloak-oidc-ca
      issuerURL: https://orgA-keycloak.apps.example.com/realms/master
    claimMappings:
      username:
        claim: email
        prefixPolicy: Prefix
        prefix:
          prefixString: 'orgA:'
    oidcClients:
    - clientID: oc-cli 
      componentName: cli
      componentNamespace: openshift-console
    - clientID: openshift-console
      clientSecret:
        name: orgA-console-secret
      componentName: console
      componentNamespace: openshift-console
  - name: 'orgB-entraid'
    issuer:
      audiences:
      - openshift-console
      - oc-cli
      issuerCertificateAuthority:
        name: orgB-entraid-oidc-ca
      issuerURL: https://login.microsoftonline.com/{tenantID}/v2.0
    claimMappings:
      username:
        claim: email
        prefixPolicy: Prefix
        prefix:
          prefixString: 'orgB:'
    oidcClients:
    - clientID: oc-cli 
      componentName: cli
      componentNamespace: openshift-console
    - clientID: openshift-console
      clientSecret:
        name: orgB-console-secret
      componentName: console
      componentNamespace: openshift-console
```

To configure the same on HyperShift, a Cluster Administrator creates a `HostedCluster` resource like so:
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
      - name: 'orgA-keycloak'
        issuer:
          audiences:
          - openshift-console
          - oc-cli
          issuerCertificateAuthority:
            name: orgA-keycloak-oidc-ca
          issuerURL: https://orgA-keycloak.apps.example.com/realms/master
        claimMappings:
          username:
            claim: email
            prefixPolicy: Prefix
            prefix:
              prefixString: 'orgA:'
        oidcClients:
        - clientID: oc-cli 
          componentName: cli
          componentNamespace: openshift-console
        - clientID: openshift-console
          clientSecret:
            name: orgA-console-secret
          componentName: console
          componentNamespace: openshift-console
      - name: 'orgB-entraid'
        issuer:
          audiences:
          - openshift-console
          - oc-cli
          issuerCertificateAuthority:
            name: orgB-entraid-oidc-ca
          issuerURL: https://login.microsoftonline.com/{tenantID}/v2.0
        claimMappings:
          username:
            claim: email
            prefixPolicy: Prefix
            prefix:
              prefixString: 'orgB:'
        oidcClients:
        - clientID: oc-cli 
          componentName: cli
          componentNamespace: openshift-console
        - clientID: openshift-console
          clientSecret:
            name: orgB-console-secret
          componentName: console
          componentNamespace: openshift-console
```

### API Extensions

The only API extension related change here will be to change the validation that exists today on the `authentications.config.openshift.io` resources `spec.oidcProviders`
field that limits the maximum length of this list to a size of 1.

Ref: https://github.com/openshift/api/blob/7f245291a17ac0bd31cf8ba08530c3355b86dbea/config/v1/types_authentication.go#L91 

We would update the existing validation to use feature-gated validations to enforce a limit of 64 entries when a new feature gate, `ExternalOIDCMultipleIdPs`, is enabled.
For example:
```diff
- // +kubebuilder:validation:MaxItems:=1
+ // +openshift:validation:FeatureGateAwareMaxItems:featureGate="",maxItems=1
+ // +openshift:validation:FeatureGateAwareMaxItems:featureGate="ExternalOIDCMultipleIdPs",maxItems=64
```

**Why 64?** This is what the upstream Structured Authentication Configuration limit is: https://github.com/kubernetes/kubernetes/blob/cffecaac55698b4f364b0be2ba92f5fd69431cb6/staging/src/k8s.io/apiserver/pkg/apis/apiserver/validation/validation.go#L51-L58

Beyond this change, there will be additional validations introduced to ensure that:

- `name` values are unique
- `issuer.issuerURL` values are unique
- `issuer.discoveryURL` values, once implemented, are unique. This field is in the process of being added as part of the work outlined in https://github.com/openshift/enhancements/blob/master/enhancements/authentication/AuthConfig-missing-fields.md

Adding these new validations should not be breaking in nature as users have never been able to specify more than a singular entry.

### Topology Considerations

#### Hypershift / Hosted Control Planes

No special considerations, but will be supported.

#### Standalone Clusters

No special considerations, but all standalone OpenShift platforms will be supported.

#### Single-node Deployments or MicroShift

>How does this proposal affect the resource consumption of a
>single-node OpenShift deployment (SNO), CPU and memory?

Allowing more IdP configurations to exist will likely have some compute and memory impacts depending on how many IdPs
get configured. The more that are configured, the larger the space taken up when stored in etcd. Additionally, there
are fields where CEL expressions can be specified and are compiled at admission time. Adding many complex CEL
expressions across multiple IdP entries may have an impact in the speed in which the admission request is processed.

Similarly, more IdP configurations to loop through to validate a token the KAS has received may consume additional CPU cycles.

**MicroShift**: As far as I am aware, MicroShift does not have a configurable authentication layer and relies on KubeConfigs only. No impact.

### Implementation Details/Notes/Constraints

Because the existing `authentications.config.openshift.io` CRD is already `v1` _and_ the existing `ExternalOIDC` feature-gate
is enabled by default on HyperShift, a new feature-gate will be added to properly go through the feature promotion cycle.

This new feature-gate will be named `ExternalOIDCMultipleIdPs`.

The majority of API-related work will be focused on properly updating the validations for the `spec.oidcProviders` field and children fields
to account for things like uniqueness constraints that are enforced by the Kubernetes API server in https://github.com/kubernetes/kubernetes/blob/cffecaac55698b4f364b0be2ba92f5fd69431cb6/staging/src/k8s.io/apiserver/pkg/apis/apiserver/validation/validation.go#L47

OpenShift components that we anticipate will "just work":
- cluster-authentication-operator
- cluster-kube-apiserver-operator

HyperShift components that we anticipate will "just work":
- control-plane-operator

The above components have their implementation logic written such that they should not need any business logic updates to handle
more than one entry in the list of OIDC providers.

#### Console

On the other hand, the Console and console-operator have been written with the expectation of a singular OIDC provider being present.

For example, an instance of the Console can only be configured with a singular issuer URL, client ID, and client secret as seen by:
https://github.com/openshift/console/blob/main/cmd/bridge/config/auth/authoptions.go#L28-L45

This restriction results in the console-operator also doing things like only returning the first client configuration
instance it sees for itself in the set of configured providers:
https://github.com/openshift/console-operator/blob/ca22e61b677ad21da5060fab7d447292c4d01afe/pkg/console/subresource/authentication/cluster.go#L26

In order to fully support the use of multiple OIDC providers we will need to make the appropriate updates to both Console and console-operator 
to appropriately account for more than one provider.

As of writing this section, this includes but may not be limited to:
- Overhauling the `oidcSetupController` in https://github.com/openshift/console-operator/blob/main/pkg/console/controllers/oidcsetup/oidcsetup.go to support multiple OIDC client configurations.
- Updating the console to support configuration of multiple OIDC client configurations
- Adding new UI views to allow users to select which identity provider they would like to authenticate to the cluster with. When external OIDC providers are not configured, this is typically
handled by the IntegratedOAuth server so we may be able to re-use an existing template here.

##### Console changes

###### Configuration Options

In order to support communication with multiple identity providers, a new configuration flag, `--user-auth-oidc-providers-file`, is proposed.
When specified, it will read the identity provider configurations from a file with a shape of:
```yaml
apiVersion: console.openshift.io/v1alpha1
kind: ConsoleExternalOIDCProviders
providers:
  - name: "providerOne"
    issuer:
      url: "https://my.issuer.com"
      caFile: "path/to/ca.crt"
    client:
      id: "console"
      secretFile: "path/to/clientSecret"
    scopes:
      - "oidc"
      - "email"
```

This new configuration flag will only be able to be set when the existing `--user-auth` flag is set to `oidc`.
It will not be able to be set alongside the existing `--user-auth-oidc-*` flags.

###### Provider Selection Login UI

Additionally, Console will be updated to now have a native login UI for selecting which provider a user can authenticate with.

We will be working with UXD and the Console team to ensure that the new web page meets UX guidelines and requirements.

Previously this logic was the responsibility of the OpenShift OAuth Server, so there is some prior art we can build off of here.

##### Console Operator Changes

The console operator controllers responsible for reacting to the external OIDC configuration changes in the `authentications.config.openshift.io` resource will
be updated to read more than a single provider, generating the new configuration file for each provider it is configured as a client
for.

Once it has generated the new configuration file, it will be written to a `ConfigMap` that will be mounted to the console deployment.
The deployment will be updated to set the `--user-auth-oidc-providers-file` flag to point to the path the `ConfigMap` is mounted.

---

All changes to these components will be done behind the new feature gate to ensure we are not accidentally introducing regressions.

### Risks and Mitigations

Risk: Many IdPs with complex CEL expressions may cause admission / cluster-authentication-operator performance
degradation as we validate that they are valid CEL expressions before handing them off to the Kubernetes API Server

Mitigation: We already limit the size of the CEL expression that can be specified to a reasonable length to prevent excessive compile times based on complexity.
We could ratchet this further, but the likelihood of getting to this bad of a state is unlikely.

Risk: Many IdPs may result in authentication against the Kubernetes API server taking longer to complete due to it needing to iterate through the
list of configured OIDC providers.

Mitigation: Limit to 64 providers. This was sufficient for upstream Kubernetes and should be sufficient for us as well. 

### Drawbacks

As with anything providing users flexibility in configurations, there may be scenarios that customers have that we have not thought of or tested.
Users may expect that we support _any_ configurations possible, even if explicitly stated otherwise.
Making this change means that we inherently are choosing to provide some level of support for these cases.

## Alternatives (Not Implemented)

### Do Nothing

Doing nothing would force customers to decide which single IdP they need to use for cluster authentication.
There is a clear need for larger enterprises and multi-tenant style clusters to be able to specify multiple IdPs.

## Open Questions [optional]

- What changes need to happen to support the user UI workflow of selecting an OIDC provider to log in with?
- Today, the console is configured with OAuth2 client information through command line flags. How should this evolve to support a world where Console may be a client against _multiple_ providers?

## Test Plan

In general, the test plan is to have:
- Unit tests in all components that implement the business logic associated with this feature.
- E2E tests in all components that implement the business logic associated with this feature.
- Periodic jobs that run with a configuration that is expected to be fairly common in place, ensuring that the configuration of these new fields don't break other components.
- Integration tests for all API changes associated with this feature.

## Graduation Criteria

### Pre-requisites

N/A

### Dev Preview -> Tech Preview

The `ExternalOIDCMultipleIdPs` feature gate will be introduced under DevPreviewNoUpgrade to prevent any potential regressions
in TechPreviewNoUpgrade Component Readiness.

Promotion of the feature gate from DevPreviewNoUpgrade to TechPreviewNoUpgrade
will be gated on at least one test being implemented in Standalone OpenShift
and run on a single platform.

This is to attempt to identify as early as possible any potential regressions
to the OpenShift Console that may need to be addressed as part of this work.

### Tech Preview -> GA

OpenShift:
- Unit testing, where appropriate
- Integration testing
- E2E testing
- Periodic testing
- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

HyperShift:
- Unit testing, where appropriate
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
In the event an invalid configuration makes its way through on OpenShift, there is a break-glass workaround of using a KubeConfig to authenticate with the kube-apiserver.

## Support Procedures

Support procedures for this feature do not differ from the existing support procedures for the ExternalOIDC feature.

## Infrastructure Needed [optional]
N/A
