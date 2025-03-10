---
title: Add Missing Authentication Configuration Fields to OpenShift API
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
- **ClaimMappings** (`UID`, `Extra`) for customizable user identity resolution.  
- **ClaimValidationRules** to enable advanced token validation via CEL expressions.  
- **UserValidationRules** to enforce security policies on usernames and groups.  

These changes enhance identity validation, support complex authentication setups, strengthen multi-tenancy, and improve RBAC enforcement. For reference, these updates align with Kubernetes' authentication configuration model: [Kubernetes Authentication Configuration](https://kubernetes.io/docs/reference/access-authn-authz/authentication/#using-authentication-configuration).  

## Motivation  

The current OpenShift authentication API lacks key fields necessary for organizations that require advanced OIDC configurations, fine-grained identity control, and stronger security enforcement. By adding missing fields such as **Issuer configurations** (`DiscoveryURL`, `AudienceMatchPolicy`), **ClaimMappings** (`UID`, `Extra`), **ClaimValidationRules**, and **UserValidationRules**, this enhancement addresses critical gaps in authentication flexibility, security, and multi-tenancy support.  

## User Stories  

- **As a customer**, I want to configure the `DiscoveryURL` and `AudienceMatchPolicy` in OpenShift so that my OIDC provider’s metadata is correctly accessed, and tokens are validated for the correct audience—even in complex networking setups or multi-cluster environments.  

- **As an enterprise administrator**, I want to define `UID` mappings and store extra claim data, allowing me to customize OIDC claims and uniquely identify users across multiple tenants. This ensures seamless integration with external identity providers and supports **role-based access control (RBAC)** based on custom claims.  

- **As a security engineer**, I want to use **ClaimValidationRules** with **CEL expressions** to enforce advanced token validation logic (e.g., checking token expiration or validating multiple claims). Additionally, I want to implement **UserValidationRules** to prevent the use of reserved system usernames and groups, reducing security risks and preventing privilege escalation.  

## Goals  

1. Enable administrators to configure and validate advanced authentication settings, including `DiscoveryURL`, `AudienceMatchPolicy`, `UID`, `Extra` claims, `ClaimValidationRules`, and `UserValidationRules`.  
2. Support flexible claim mapping, multi-cluster authentication, and integration with external identity providers.  
3. Strengthen security by enforcing advanced validation rules and identity policies to ensure proper access control.  

## Non-Goals  

- This enhancement does **not** introduce new authentication mechanisms beyond OIDC.  

## Proposal

This proposal introduces missing authentication fields to the OpenShift API by modifying the `authentications.config.openshift.io` CustomResourceDefinition (CRD) and updating relevant components. These changes will improve support for advanced OIDC configurations, identity customization, and security enforcement.

### Changes to `authentications.config.openshift.io` CRD  

To enhance authentication flexibility and security, the following fields will be added to the CRD:  

- **Issuer Configuration**  
  - `.spec.oidcProviders[].issuer.discoveryURL`: Allows specifying a custom OIDC discovery endpoint.  
  - `.spec.oidcProviders[].issuer.audienceMatchPolicy`: Enables flexible audience validation rules.  

- **Claim Mappings**  
  - `.spec.oidcProviders[].claimMappings.uid`: Defines a claim for user identification.  
  - `.spec.oidcProviders[].claimMappings.extra`: Allows storing additional claims for authorization purposes.  

- **Claim and User Validation Rules**  
  - `.spec.oidcProviders[].claimValidationRules.expression`: Supports CEL-based validation of claims.  
  - `.spec.oidcProviders[].userValidationRules`: Enforces security policies on usernames and groups.  

### Components to Be Updated  

To ensure the new authentication fields are processed and applied, the following components need modifications:  

1. **Cluster Authentication Operator**  
   - Today, the Cluster Authentication Operator is responsible for generating the authentication configuration by pulling settings from the `authentication.config.openshift.io` CRD and writing the corresponding configuration files used by the `kube-apiserver`.  
   - This logic will be updated to ensure that the new fields (`discoveryURL`, `audienceMatchPolicy`, claim mappings, and validation rules) are correctly extracted from the CRD and included in the generated authentication configuration passed to the `kube-apiserver`.  

2. **Hypershift Control Plane Operator**  
   - Today, the Hypershift Control Plane Operator is responsible for generating and managing authentication configurations for hosted control planes, ensuring consistency across managed clusters.  
   - This logic will be updated to support the new fields (`discoveryURL`, `audienceMatchPolicy`, claim mappings, and validation rules) so that authentication policies remain aligned across hosted control planes. The operator will extract these configurations from the `authentication.config.openshift.io` CRD and ensure they are correctly propagated to the hosted control plane's authentication setup.  

### Current vs. Updated Behavior  

- **Today:** OpenShift lacks support for custom OIDC discovery URLs, flexible audience validation, claim-based UID mapping, and CEL-based validation.  
- **After Enhancement:** These new fields allow administrators to configure advanced authentication settings, improving multi-tenancy, security, and compatibility with external identity providers.  

These enhancements will ensure OpenShift provides a more robust and configurable authentication experience. Detailed implementation specifics will be covered in the **Implementation Details** section.

### Workflow Description

This section describes how users will configure and utilize the newly added authentication fields in OpenShift. The workflow outlines user roles, required actions, and expected outcomes for each field.

### User Roles

- **Cluster Administrator**: Responsible for configuring authentication settings in OpenShift.
- **Security Engineer**: Ensures that authentication policies and validations enforce security best practices.

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
     audienceMatchPolicy: MatchAny
     ```

#### 3. Custom UID Mapping for Identity Providers
**Scenario:**  
A company needs to map the `uid` claim to a custom identifier in order to properly identify users across different OIDC providers.

**Steps:**

1. Update the authentication CRD to include the custom `uid` mapping:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: Authentication
   metadata:
     name: cluster
   spec:
     tokenClaimMappings:
       uid: claims.email  # Example of custom claim mapping
  ```

### 4. Adding Extra Claims for Role-Based Permissions
**Scenario:**  
A cluster administrator needs to map an extra claim (such as `role`) from the OIDC token for role-based access control (RBAC).

**Steps:**

1. Update the authentication CRD to include extra claims:
   ```yaml
   apiVersion: config.openshift.io/v1
   kind: Authentication
   metadata:
     name: cluster
   spec:
     tokenClaimMappings:
       extra:
         - name: example.com/role
           claimName: role
  ```

### API Extensions

To facilitate the configuration and validation of token claims, token issuers, and user validation rules, the existing `authentications.config.openshift.io` CRD is extended with new fields and structures. These extensions allow for enhanced flexibility in token validation, token claim mappings, issuer configuration, and user validation. The proposed changes aim to introduce new fields for token claim mappings, validation rules, user validation rules, and token issuer configuration, providing greater control over how authentication and token validation are managed within the system.

The CRD modifications include the following fields:

#### TokenIssuer
```go
type TokenIssuer struct {
    // discoveryURL, if specified, overrides the URL used to fetch discovery
    // information instead of using "{url}/.well-known/openid-configuration".
    // The exact value specified is used, so "/.well-known/openid-configuration"
    // must be included in discoveryURL if needed.
    // Example:
    // discoveryURL: "https://oidc.oidc-namespace/.well-known/openid-configuration"
    // discoveryURL: "https://oidc.example.com/.well-known/openid-configuration"
    DiscoveryURL string `json:"discoveryURL,omitempty"`

    // AudienceMatchPolicy controls how the "aud" claim in JWT tokens is validated.
    // It allows flexible matching of the audience value in tokens.
    // Possible values: MatchAny, MatchAll.
    AudienceMatchPolicy AudienceMatchPolicyType `json:"audienceMatchPolicy,omitempty"`
}
```
For more details on the Issuer type and its fields, see [here](https://github.com/openshift/api/blob/b8a067b12e1c404dc0f8e5dff9183ef20389318c/config/v1/types_authentication.go#L228-L252).

#### TokenClaimMappings
```go
// TokenClaimMappings provides the claim mapping configuration for token-based identities
type TokenClaimMappings struct {
    // UID claim mapping
    UID ClaimOrExpression `json:"uid,omitempty"`

    // Extra claim mappings
    Extra []ExtraMapping `json:"extra,omitempty"`
}
```
For more details on the ClaimMappings type and its fields, see [here](https://github.com/openshift/api/blob/b8a067b12e1c404dc0f8e5dff9183ef20389318c/config/v1/types_authentication.go#L254-L265)


#### TokenClaimValidationRule
```go
type TokenClaimValidationRule struct {
    // Expression allows configuring a custom validation rule based on an expression
    // This field defines a validation rule using a claim expression to evaluate the token
    Expression string `json:"expression,omitempty"`

    // Message defines a custom error message to be returned if the validation fails
    Message string `json:"message,omitempty"`
}
```
For more details on the ClaimValidationRule type and its fields, see [here](https://github.com/openshift/api/blob/b8a067b12e1c404dc0f8e5dff9183ef20389318c/config/v1/types_authentication.go#L440-L450)

#### ToeknUserValidationRule
```go
// UserValidationRule provides the configuration for a single user validation rule.
type ToeknUserValidationRule struct {
	Expression string
	Message    string
}
```
For more details on the UserValidationRule type and its fields, see [here](https://github.com/kubernetes/kubernetes/blob/75909b89201386c8a555eadc79d14fb11f91747c/staging/src/k8s.io/apiserver/pkg/apis/apiserver/types.go#L284)

### Topology Considerations

#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with
Hypershift?

See https://github.com/openshift/enhancements/blob/e044f84e9b2bafa600e6c24e35d226463c2308a5/enhancements/multi-arch/heterogeneous-architecture-clusters.md?plain=1#L282

How does it affect any of the components running in the
management cluster? How does it affect any components running split
between the management cluster and guest cluster?

#### Standalone Clusters

**Is the change relevant for standalone clusters?**

This change is relevant for standalone clusters, as we are making modifications to the `cluster-authentication-operator`. This operator is responsible for generating the authentication configuration and ensuring the integration of external OIDC providers. These updates will affect how the operator interacts with the configuration, particularly regarding the new fields in the API.

The following updates will be necessary for standalone clusters:

1. **Changes to `cluster-authentication-operator` Code:**

   The file responsible for generating the authentication configuration, [externaloidc_controller.go](http://github.com/openshift/cluster-authentication-operator/blob/eb6de2ecd5097a3146e330ea24b0e66029ae5152/pkg/controllers/externaloidc/externaloidc_controller.go#L148), will be modified to ensure that the authentication configuration uses our custom API instead of the Kubernetes API for missing fields. Specifically, the method [generateAuthConfig](https://github.com/openshift/cluster-authentication-operator/blob/eb6de2ecd5097a3146e330ea24b0e66029ae5152/pkg/controllers/externaloidc/externaloidc_controller.go#L148) will be updated to extract the new fields defined in the `authentication.config.openshift.io` CRD.

2. **Changes to the API Definition:**

   The `authentication.config.openshift.io` CRD definition will be updated to include the new fields, including the `OIDCProvider`. This change will be reflected in the API file, located at [types_authentication.go](https://github.com/openshift/api/blob/b8da3bfeaf773d9dce2ea56edc9a1cf06cfdbd80/config/v1/types_authentication.go#L195). Specifically, we will update the definition of `OIDCProvider` to handle new fields such as `discoveryURL`, `audienceMatchPolicy`, and custom claim mappings.

3. **Testing:**

   The tests in [externaloidc_controller_test.go](https://github.com/openshift/cluster-authentication-operator/blob/master/pkg/controllers/externaloidc/externaloidc_controller_test.go) will need to be adjusted. In particular, the test case [TestExternalOIDCController_sync](https://github.com/openshift/cluster-authentication-operator/blob/eb6de2ecd5097a3146e330ea24b0e66029ae5152/pkg/controllers/externaloidc/externaloidc_controller_test.go#L212) will be updated to reflect the changes in the controller logic. We will also add additional test cases to validate the integration of the new fields and ensure that the operator processes them correctly.

By making these updates, we ensure that standalone clusters can utilize the new authentication configurations, including custom OIDC providers and claim mappings, for enhanced authentication control and validation.

#### Single-node Deployments or MicroShift

**Impact on Single-node OpenShift (SNO) Resource Consumption:**  
This proposal introduces additional API fields that will be stored in etcd and cached in specific components. However, the impact on CPU and memory consumption is expected to be negligible, as these changes primarily involve storing and retrieving small amounts of configuration data. No significant increase in resource utilization is anticipated.

**Impact on MicroShift:**  
#TODO 

### Implementation Details/Notes/Constraints

What are some important details that didn't come across above in the
**Proposal**? Go in to as much detail as necessary here. This might be
a good place to talk about core concepts and how they relate. While it is useful
to go into the details of the code changes required, it is not necessary to show
how the code will be rewritten in the enhancement.

### Risks and Mitigations

#### Security Risks  
Introducing new authentication-related API fields could expose potential misconfigurations or security vulnerabilities.  
- **Mitigation:**  
  - Ensure that all authentication configurations are validated before applying them.  
  - Perform security reviews in collaboration with the OpenShift security team.  
  - Conduct penetration testing to validate that changes do not introduce vulnerabilities.  

### Drawbacks

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

## Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

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
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

## Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Alternatives

Do nothing

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
