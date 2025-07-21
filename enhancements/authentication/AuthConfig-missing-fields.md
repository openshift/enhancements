---
title: AuthConfig-missing-fields

authors:
  - "@ShazaAldawamneh"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@everettraven" 
  - "@ibihim"
  - "@liouk"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@sjenning"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@JoelSpeed"
  - "@everettraven"
creation-date: 2025-03-03
last-updated: 2025-03-03
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/CNTRLPLANE-127"
see-also:
  - "https://issues.redhat.com/browse/OCPSTRAT-306"
replaces:
  - ""
superseded-by:
  - ""
---
# Add Missing Authentication Configuration Fields to OpenShift API

## Summary

This enhancement proposal adds missing authentication fields to the structured authentication configuration in the OpenShift API, improving flexibility, security, and interoperability with identity providers.  

Key additions include:  
- **Issuer fields** (`DiscoveryURL`, `AudienceMatchPolicy`) for advanced OIDC configuration.  
- **ClaimValidationRules** to enable advanced token validation via CEL expressions.  
- **UserValidationRules** to enforce security policies on usernames and groups.  

These changes enhance identity validation, support complex authentication setups, strengthen multi-tenancy, and improve RBAC enforcement. For reference, these updates align with Kubernetes' authentication configuration model: [Kubernetes Authentication Configuration](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#using-authentication-configuration).  

## Motivation  

The current OpenShift authentication API lacks key fields necessary for organizations that require advanced OIDC configurations, fine-grained identity control, and stronger security enforcement. By adding missing fields such as **Issuer configurations** (`DiscoveryURL`, `AudienceMatchPolicy`), **ClaimValidationRules**, and **UserValidationRules**, this enhancement addresses critical gaps in authentication flexibility, security, and multi-tenancy support.

### User Stories  

- **As a customer**, I want to configure the `DiscoveryURL` and `AudienceMatchPolicy` in OpenShift so that my OIDC provider’s metadata is correctly accessed, and tokens are validated for the correct audience—even in complex networking setups or multi-cluster environments.  

- **As a security engineer**, I want to use **ClaimValidationRules** with **CEL expressions** to enforce advanced token validation logic (e.g., checking token expiration or validating multiple claims). Additionally, I want to implement **UserValidationRules** to prevent the use of reserved system usernames and groups, reducing security risks and preventing privilege escalation.  

### Goals  

1. Enable administrators to configure and validate advanced authentication settings, including `DiscoveryURL`, `AudienceMatchPolicy`, `UID`, `Extra` claims, `ClaimValidationRules`, and `UserValidationRules`.  
2. Support flexible claim mapping, multi-cluster authentication, and integration with external identity providers.  
3. Strengthen security by enforcing advanced validation rules and identity policies to ensure proper access control.  

### Non-Goals  

- This enhancement does **not** introduce new authentication mechanisms beyond OIDC.  

## Proposal

This proposal introduces missing authentication fields to the OpenShift API by modifying the `authentications.config.openshift.io` CustomResourceDefinition (CRD) and updating relevant components. These changes will improve support for advanced OIDC configurations, identity customization, and security enforcement.

### Changes to `authentications.config.openshift.io` CRD  

To enhance authentication flexibility and security, the following fields will be added to the CRD:  

- **Issuer Configuration**  
  - `.spec.oidcProviders[].issuer.discoveryURL`: Allows specifying a custom OIDC discovery endpoint.  
  - `.spec.oidcProviders[].issuer.audienceMatchPolicy`: Enables flexible audience validation rules.  

- **Claim and User Validation Rules**  
  - `.spec.oidcProviders[].claimValidationRules.expression`: Supports CEL-based validation of claims.  
  - `.spec.oidcProviders[].userValidationRules`: Enforces security policies on usernames and groups.  

### Components to Be Updated  

To ensure the new authentication fields are processed and applied, the following components need modifications:  

1. **Cluster Authentication Operator**  
   - Today, the Cluster Authentication Operator is responsible for generating the authentication configuration by pulling settings from the `authentication.config.openshift.io` CRD and writing the corresponding configuration files used by the `kube-apiserver`.  
    - This logic will be updated to ensure that the new fields (`discoveryURL`, `audienceMatchPolicy`, and validation rules) are correctly extracted from the CRD and included in the generated authentication configuration passed to the `kube-apiserver`.

2. **Hypershift Control Plane Operator**  
   - Today, the Hypershift Control Plane Operator is responsible for generating and managing authentication configurations for hosted control planes, ensuring consistency across managed clusters.  
   - This logic will be updated to support the new fields (`discoveryURL`, `audienceMatchPolicy`, and validation rules) so that authentication policies remain aligned across hosted control planes. The operator will extract these configurations from the `authentication.config.openshift.io` CRD and ensure they are correctly propagated to the hosted control plane's authentication setup.  

### Current vs. Updated Behavior  

- **Today:** OpenShift lacks support for custom OIDC discovery URLs, flexible audience validation, and CEL-based validation.

- **After Enhancement:** These new fields allow administrators to configure advanced authentication settings, improving multi-tenancy, security, and compatibility with external identity providers.  

These enhancements will ensure OpenShift provides a more robust and configurable authentication experience. Detailed implementation specifics will be covered in the **Implementation Details** section.

### Workflow Description

This section describes how users will configure and utilize the newly added authentication fields in OpenShift. The workflow outlines user roles, required actions, and expected outcomes for each field.

### User Roles

- **Cluster Administrator**: Responsible for configuring authentication settings in OpenShift and ensuring that authentication policies and validations enforce security best practices.


### General Workflow

The process for configuring the newly added fields follows a similar pattern for all fields:

1. **Modify the Authentication Configuration**  
   - The administrator updates the `authentications.config.openshift.io` CRD with the new field values.
   - Example: Adding a `DiscoveryURL` for an OIDC provider.
   
2. **Apply the Configuration**  
   - The updated configuration is applied to the cluster.
   - OpenShift's `cluster-authentication-operator` processes the changes and propagates them to relevant components (e.g., `kube-apiserver`).

3. **Validation and Enforcement**  
   - The cluster authentication system ensures that the provided values are correctly interpreted and enforced.
   - If `ClaimValidationRules` or `UserValidationRules` are configured, OpenShift evaluates authentication requests against them.

4. **Authentication Flow Execution**  
   - When a user authenticates, OpenShift uses the configured values to validate identity claims, apply rules, and determine user permissions.


### Use Case Examples

#### Precondition
A customer has a subscription for an identity provider (IdP) that they would like their employees to use for authenticating with their OpenShift clusters. The IdP provides identity services but may have certain non-standard configurations that require customization within OpenShift.

#### 1. Configuring a Non-Standard OIDC Discovery URL  
**Scenario:**  
A company is using an external identity provider to authenticate users across multiple services, including OpenShift. However, this IdP does not follow the standard OIDC discovery URL format. To integrate OpenShift with this provider, the cluster administrator must manually specify the custom discovery URL in OpenShift’s authentication configuration.  

**Steps for the Cluster Administrator:**  
1. Update the `Authentication` CRD to include the custom `discoveryURL`:  
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: Authentication
   metadata:
     name: cluster
   spec:
     oidcProviders:
       - issuer:
           discoveryURL: "https://custom-idp.example.com/.well-known/openid-configuration"
    ```

#### 2. Configuring `AudienceMatchPolicy` for Token Validation
**Scenario:**  
A cluster administrator needs to configure OpenShift to validate the audience (`aud`) claim in JWT tokens using a flexible matching policy.

**Steps:**
1. Update the authentication CRD with `AudienceMatchPolicy`:
```yaml
  apiVersion: config.openshift.io/v1
  kind: Authentication
  metadata:
    name: cluster
  spec:
    type: OIDC
    oidcProviders:
    - name: foo
      issuer:
        audienceMatchPolicy: MatchAny
```

#### 5. Enforcing Token Expiration Limits (Claim Validation)  
**Scenario:**  
A security-conscious organization wants to ensure that all OIDC tokens used for authentication have a maximum expiration time of 24 hours.  

**Steps:**  
1. Update the authentication CRD to enforce a maximum token expiration time:  
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: Authentication
   metadata:
     name: cluster
   spec:
    type: OIDC
    oidcProviders:
      claimValidationRules:
     - expression: 'claims.exp - claims.nbf <= 86400'
        message: total token lifetime must not exceed 24 hours
  ```

#### 6. Restricting Reserved Usernames (User Validation)  
**Scenario:**  
An OpenShift administrator wants to prevent users from being created with reserved system prefixes, such as `system:`, to avoid conflicts with system users.  

**Steps:**  
1. Update the authentication CRD to enforce username restrictions:  
   ```yaml
  apiVersion: config.openshift.io/v1
  kind: Authentication
  metadata:
    name: cluster
  spec:
    type: OIDC
    oidcProviders:
      userValidationRules:
      - expression: "!user.username.startsWith('system:')" # the expression will evaluate to true, so validation will succeed.
          message: 'username cannot used reserved system: prefix'
  ```

### API Extensions
To facilitate the configuration and validation of token claims, token issuers, and user validation rules, the existing `authentications.config.openshift.io` CRD is extended with new fields and structures. These extensions provide enhanced flexibility in token validation, token claim mappings, issuer configuration, and user validation. The proposed changes introduce new fields that allow administrators to define custom authentication behaviors.  
The proposed API changes have been submitted in [openshift/api#2245](https://github.com/openshift/api/pull/2245). Below are examples of how users will configure these new fields:  


#### TokenIssuer  
##### DiscoveryURL
- **What it does:**  
  The `discoveryURL` field specifies the OpenID Connect (OIDC) discovery endpoint, which provides metadata about the identity provider. It allows OpenShift to automatically retrieve issuer details, supported authentication methods, and token endpoints.  

- **Why users would set it:**  
  This field is optional and typically only needed when integrating OpenShift authentication with an external OIDC provider that does not support the standard OIDC discovery endpoint. Setting this ensures proper token validation in those specific cases.

- **Constraints:**  
  - The value must be a valid HTTPS URL.  
  - The `discoveryURL` must be different from the `issuer.url`.  
  - If this field is misconfigured, authentication may fail, preventing user logins.  

**Example:**  
```yaml
   apiVersion: config.openshift.io/v1
   kind: Authentication
   metadata:
     name: cluster
   spec:
    type: OIDC
    oidcProviders:
    - issuer:
      discoveryURL: https://discovery.example.com/.well-known/openid-configuration
```

##### AudienceMatchPolicy
- **What it does:**  
  The `audienceMatchPolicy` field controls how OpenShift matches the audience (`aud`) claim in ID tokens issued by the OIDC provider.  

- **Why users would set it:**  
  This ensures that tokens are only accepted if they are intended for OpenShift, preventing replay attacks or misuse of tokens from other services.  

- **Constraints:**  
  - Supported values:  
    - `MatchAny`: Accepts any audience value present in the configured `audiences` list.  
  - If multiple audiences are defined, `MatchAny` must be used.  

**Example:**  
```yaml
apiVersion: config.openshift.io/v1
kind: Authentication
metadata:
  name: cluster
spec:
  type: OIDC
  oidcProviders:
  - name: foo
    issuer:
      audienceMatchPolicy: MatchAny
```

### TokenClaimValidationRule
- **What it does:**  
  Defines validation rules for token claims using Common Expression Language (CEL).  

- **Why users would set it:**  
  Enforces security policies on token claims, such as expiration times.  

- **Constraints:**  
  - The CEL expression must be valid and evaluate to `true` for authentication to succeed.  
  - Misconfigured rules may prevent valid users from logging in.  

**Example:**  
```yaml
   apiVersion: config.openshift.io/v1
   kind: Authentication
   metadata:
     name: cluster
   spec:
    type: OIDC
    oidcProviders:
      claimValidationRules:
      - expression: 'claims.exp - claims.nbf <= 86400'
        message: total token lifetime must not exceed 24 hours
```

### TokenUserValidationRule
- **What it does:**  
  Defines rules to validate the user object created from an authenticated token.  

- **Why users would set it:**  
  Ensures that users comply with security policies, such as preventing reserved prefixes in usernames.  

- **Constraints:**  
  - The CEL expression must return `true` for authentication to proceed.  
  - Invalid configurations may block valid users from accessing the cluster.  

**Example:**  
```yaml
  apiVersion: config.openshift.io/v1
  kind: Authentication
  metadata:
    name: cluster
  spec:
    type: OIDC
    oidcProviders:
      userValidationRules:
      - expression: "!user.username.startsWith('system:')" # the expression will evaluate to true, so validation will succeed.
        message: 'username cannot used reserved system: prefix'
  ```

### Topology Considerations

#### Hypershift / Hosted Control Planes

To support this change in HyperShift, updates are required in the logic responsible for generating the structured authentication configuration for the kube-apiserver.  

Currently, this logic is defined in the following file:  
🔗 [auth.go](https://github.com/openshift/hypershift/blob/433f8c99016ca7a13c5587d5629d7975e134b54a/control-plane-operator/controllers/hostedcontrolplane/kas/auth.go#L39-L97).
Similar to the `cluster-authentication-operator`, this implementation needs to be modified to map the new fields introduced in the `authentications.config.openshift.io` CRD to Kubernetes structured authentication configuration types. This ensures that the additional authentication settings are properly propagated within HyperShift environments.  

##### Impact on Management and Guest Clusters  
There does not appear to be any impact beyond what already changes when the existing OIDC functionality is enabled. The modifications are limited to mapping new authentication fields, and they do not introduce any structural changes that would affect components running in either the management cluster or guest cluster. 

#### Standalone Clusters

**Is the change relevant for standalone clusters?**

This change is relevant for standalone clusters, as we are making modifications to the `cluster-authentication-operator`. This operator is responsible for generating the authentication configuration and ensuring the integration of external OIDC providers. These updates will affect how the operator interacts with the configuration, particularly regarding the new fields in the API.

The following updates will be necessary for standalone clusters:

1. **Changes to `cluster-authentication-operator` Code:**

The [generateAuthConfig](https://github.com/liouk/cluster-authentication-operator/blob/cc82f462af153c188c2e717ea4a8d19933b7d381/pkg/controllers/externaloidc/externaloidc_controller.go#L148) method of the ExternalOIDCController is used to map OIDC provider configurations in the authentication.config.openshift.io custom resource to the structured authentication configuration format that the Kubernetes API server understands. This method will be updated to include logic to map the new fields introduced in the authentication.config.openshift.io CRD to their existing counterparts in the Kubernetes structured authentication configuration types.

2. **Testing:**

   The tests in [externaloidc_controller_test.go](https://github.com/openshift/cluster-authentication-operator/blob/master/pkg/controllers/externaloidc/externaloidc_controller_test.go) will need to be adjusted. In particular, the test case [TestExternalOIDCController_sync](https://github.com/openshift/cluster-authentication-operator/blob/eb6de2ecd5097a3146e330ea24b0e66029ae5152/pkg/controllers/externaloidc/externaloidc_controller_test.go#L212) will be updated to reflect the changes in the controller logic. We will also add additional test cases to validate the integration of the new fields and ensure that the operator processes them correctly.

By making these updates, we ensure that standalone clusters can utilize the new authentication configurations, including custom OIDC providers and claim mappings, for enhanced authentication control and validation.

#### Single-node Deployments or MicroShift

**Impact on Single-node OpenShift (SNO) Resource Consumption:**  
This proposal introduces additional API fields that will be stored in etcd and cached in specific components. However, the impact on CPU and memory consumption is expected to be negligible, as these changes primarily involve storing and retrieving small amounts of configuration data. No significant increase in resource utilization is anticipated.

**Impact on MicroShift:**  
MicroShift does not have a built-in authentication stack or configurable authentication layer. Instead, it relies on `kubeconfig` files for access control, which are generated at startup and used to authenticate API requests.  

Given this, there are no anticipated impacts from the proposed changes, as MicroShift does not currently support authentication configuration options. However, if MicroShift's stance on authentication evolves in the future, further evaluation may be required.  

### Implementation Details/Notes/Constraints

N/A

### Risks and Mitigations

#### Security Risks  
Adding new authentication-related API fields could allow cluster administrators to misconfigure their authentication layer leading to security vulnerabilities. The new API fields themselves don't introduce any security risk.

- **Mitigation:**  
  - Ensure we have robust admission and runtime validations to ensure that misconfigurations are prevented as much as possible prior to rolling out the authentication layer changes.

### Drawbacks
One potential argument against implementing this enhancement is that achieving parity with the upstream Kubernetes configuration will result in supporting any configuration that a user can configure on a standard Kubernetes cluster.

This could lead to situations where customers implement configurations that have not been tested or fully validated, and they may expect support for configurations outside the scope of our tested configurations, unless we establish explicit supportability guidelines for this feature.

However, despite these concerns, this drawback is not significant enough to prevent the proposed changes. The benefits of offering greater flexibility and enabling customers to integrate their existing Identity Provider (IdP) infrastructure with OpenShift's authentication layer far outweigh the potential downsides.

## Test Plan
For this enhancement, we will expand on the existing OIDC test suite. The focus will be on adding tests to verify the proper functionality of the new configuration options introduced by the new API fields. These tests will confirm that the integration of these new options aligns correctly with the existing authentication functionality and performs as expected. Additionally, we will ensure that the new fields are thoroughly tested across common configurations and scenarios to guarantee reliable behavior.

## Graduation Criteria

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA

Given that these changes are additions to the ExternalOIDC feature API, which is currently in Tech Preview for standalone OpenShift, the general approach is as follows:

- We will introduce a new feature gate called `ExternalOIDCWithNewAuthConfigFields`

To graduate from Tech Preview (TP) to GA, the following criteria must be met:

- User-facing documentation exists.
- Time for early adopter feedback in Tech Preview.
- Additional testing will be conducted based on early adopter and anticipated user feedback, particularly for desired configurations.

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

## Upgrade Considerations

When upgrading from a version of OpenShift that does not include these API changes to a version that does, the upgrade process should be seamless. The new API fields will be introduced, but they will not affect existing configurations unless explicitly used. No manual intervention is required before the upgrade, and clusters should continue functioning as expected without any disruptions.
After upgrading, customers can begin leveraging the new API fields by updating the Authentication resource to configure token claim mappings, validation rules, and user validation settings.

## Downgrade Considerations

If a customer downgrades from a version of OpenShift where these API changes are present to a version where they are not, any configurations using the new fields will no longer be recognized. This could lead to unexpected behavior or validation errors. To ensure a smooth downgrade, customers should remove any references to the new API fields before initiating the downgrade process.

## Version Skew Strategy

Since this enhancement builds upon the existing OIDC functionality, it will follow the same [version skew strategy established for the original OIDC feature ](https://github.com/openshift/enhancements/blob/master/enhancements/authentication/direct-external-oidc-provider.md#version-skew-strategy). These changes do not introduce any new version skew concerns beyond those already considered in the initial implementation of OIDC support. 
As a result, there are no additional compatibility risks or upgrade constraints beyond what has already been accounted for in the existing OIDC version skew strategy.

## Operational Aspects of API Extensions

This enhancement builds upon the existing OIDC functionality and will follow the same operational practices defined in the earlier OIDC enhancements. In particular, it inherits the ART considerations outlined in the [adding UID and extra claim mapping configuration enhancement](https://github.com/openshift/enhancements/blob/master/enhancements/authentication/adding-uid-and-extra-claim-mapping-configuration-options-for-external-oidc.md#operational-aspects-of-api-extensions).

## Impact of Misconfiguration on Users

If users configure the new API fields incorrectly, authentication may break, causing the kube-apiserver to become inaccessible. This could prevent users from logging in or interacting with the cluster.

## Mitigation Strategy

We plan to add a combination of admission time and runtime checks to prevent invalid configurations from being applied. These checks will ensure that only properly formatted and functional configurations are accepted before they are rolled out to the kube-apiserver.

If the external OIDC authentication layer becomes misconfigured or non-functional, users will still be able to access the cluster using a kubeconfig as a break-glass scenario. This ensures that cluster administrators can regain access and correct any issues without being locked out.

## How Users Will Be Informed of a Prevented Misconfiguration

If an invalid configuration is rejected, users will receive a clear error message explaining what went wrong. Additionally, the cluster-authentication-operator's `clusteroperator` conditions (e.g., `AuthenticationDegraded=True`) and logs will indicate issues related to authentication misconfigurations.

## Support Procedures

This enhancement builds upon the existing OIDC support infrastructure. As with previous enhancements such as [adding UID and extra claim mapping configuration options for external OIDC](https://github.com/openshift/enhancements/blob/master/enhancements/authentication/adding-uid-and-extra-claim-mapping-configuration-options-for-external-oidc.md#support-procedures), supportability is ensured through existing tools and workflows.

### Logging and Errors  

The authentication configuration is consumed by the **kube-apiserver (KAS)** pods but produced by the **Cluster Authentication Operator (CAO)**. The CAO is responsible for performing as many validations on the configuration as possible before generating the final authentication configuration that will be consumed by the KAS. This means that in case of a bad configuration, the CAO will move its status to **Degraded** and log any errors encountered.  

It is noteworthy that the CAO performs a full validation on any provided **authentication settings**, including **OIDC provider configurations, audience claims, and token mappings**. These validations help detect misconfigurations early in the process, ensuring that authentication failures do not propagate to the cluster.  

Nevertheless, in case of authentication problems that are not logged on the CAO side, one should consult the **KAS logs** and rollout progress as well. Additionally, the **KAS-operator logs** will reveal any problems related to syncing the ConfigMap, creating a revisioned authentication configuration, generating static files, or enabling the required KAS CLI arguments.  

## Alternatives (Not Implemented)

n/a

## Infrastructure Needed [optional]

N/A