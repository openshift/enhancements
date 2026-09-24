---
title: simplified-olm-token-auth
authors:
  - "@jstuever"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "TBD"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "TBD"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "None"
creation-date: 2025-06-27
last-updated: 2025-06-27
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "TBD"
see-also:
  - []
replaces:
  - "enhancements/cloud-integration/tokenized-auth-enablement-operators-on-cloud.md"
superseded-by:
  - []
---
# Simplified OLM token authentication

## Summary

OpenShift previously gained the ability to use short-lived-tokens for authentication to various cloud providers. The core operators were included in this effort and are already capable of doing so. Operators managed by the Operator Lifecycle Manager (OLM) can benefit from this integration as well. Several have already done so using a previously defined process. It includes steps where the operator creates a CredentialRequest and then waits for the Cloud Credential Operator (CCO) to do little more than translate that into a secret. This enhancement is to simplify this by removing CCO from the process and having the OLM operators generate the secrets directly.

## Motivation

The intent presented in the original enhancement was to unify the process across operators so users of several of them have the same experience and similar steps to perform. This enhancement does not change that. Nor does it remove CCO from the process altogether; it is still valuable to use the `ccoctl` binary as part of the process. What this enhancement proposes is to remove CCO's in-cluster role of translating the OLM CredentialRequests into secrets.

One reason CCO was placed into the workflow was to provide consolidated logic for creating the token enabled secrets. In theory, this reduces the overall effort by providing shared code which enables future changes to happen in a single place. In practice, this appears to have increased the up-front effort for each operator while also adding continual maintenance requirements without providing the expected long term savings. Instead of creating a relatively simple secret (available in k8s libraries), these operators now have to create a credentialRequest. This forces CCO to be a dependency of the operator (the credentialRequest definition currently lives in the CCO repo). In addition, the interface between these operators and CCO adds several more points of failure. This increases the effort required from end users, support, and engineers to understand why these things break when they do. This is a significant increase in known effort with the hopes of reducing hypothetical future effort. By removing CCO from this part of the framework, we reduce the known effort significantly.

One of the hypothetical situations presented was in the case where the format of a secret changes. In this scenario, CCO would presumably be the only place where this change would need to take place. In practice, an operator only needs the new format if it itself has changed to do so. This actually highlights a compatibility nightmare created by the current framework. If CCO starts creating the secrets in a new format, what happens to the OLM operators that are still using the old? Would we update the operator to understand both formats? Would we update CCO to learn from the operator which format it needs? How would the operator relay that requirement to CCO? Either way, it appears the OLM operator would need additional changes in order to handle this situation. As a result, the desired benefit is not realized. By having the operator manage the secret directly, we guarantee that it is compatible with itself and enable it to move freely through this space unimpeded by CCO.

Another hypothetical situation presented was to enable CCO to validate the credential specified in the credentialRequest prior to creating the secret. This is a good idea, in theory, because it allows the workflow to fail early and provide the end user with a clear reason, if they know where to look. In practice, this requires CCO to have permissions in the cloud provider that it would not otherwise need. CCO currently requires no permissions in the cloud provider (though, we currently still provision an account with mint mode level permissions). Customers who are choosing to use STS authentication are doing so, in part, to minimize security risks. Having additional permission requirements goes against this. Realizing this benefit would be at the expense of security. Removing CCO from this part of the process will enable us to remove these permission requirements in the future.

CCO was designed to take no actions when in manual mode. This changed when it was modified to handle OLM operators' credentialRequests. All manual mode clusters now use the mint-mode execution path with injected logic to handle these special use cases. This has caused unintended consequences resulting in several bugs. The resolution of some of these bugs will require significant refactoring of CCO. By removing CCO from this part of the framework, we reduce the work required to resolve these bugs.

CCO became a required component for clusters using short-term-token authentication when the OLM integration was introduced. This causes additional resource requirements where they would otherwise not be needed such as hypershift and single-node clusters using short-term-token authentication. By removing CCO from this part of the framework, we remove this requirement and enable reduction in resources in these environments.

### User Stories

* (New) As the cloud credential operator team, I want to remove the cloud credential operator from the OLM operator short-lived-token integration in order to:
  * reduce the complexity of cloud credential operator when in manual mode
  * resolve multiple bugs introduced when the short-lived-token code was added to the operator
  * enable future efforts to remove all permission requirements form the cloud credential operator when in manual mode
* (New) As an OLM operator team: I want to remove the cloud credential operator from the OLM operator short-lived-token integration process in order to:
  * remove the requirement to maintain CCO as a dependency in my operator
  * reduce the effort required to implement and support this integration
* (New) As a cluster admin / support engineer, I want to simplify the short-lived-token integration for OLM Operators in order to:
  * reduce the effort required to support this configuration.

* As a cluster admin, I want to know which OLM Operators are safe to install because they will not be interacting with the Cloud Provider on a cluster that only supports short-lived-token authentication with the Cloud Provider
* As a cluster admin, I want to know which OLM Operators support short-lived-token authentication for my cloud, so that I can provide token-based access for cloud resources for them
* As a cluster admin of a cluster using short-lived-token cloud auth, I want to know what's required to install and upgrade OLM  Operators whenever those operators manage resources that authenticate against my cloud so they can function properly
* As a cluster admin, I want the experience of short-lived-token authentication to be as similar as possible from one Cloud Provider to the other, so that I can minimize cloud Specific knowledge and focus more on OpenShift.
* As an Operator developer, I want to have a standard framework to define short-lived-token authentication requirements and consume them, per supported cloud, so that my operator will work on short-lived-token authentication configured clusters.
* As an Operator Hub browser, I want to know which operators support short-lived-token cloud auth and on which clouds so I can see only a filtered list of operators that will work on the given cluster.
* As an Operator Hub browser, I want to be informed / reminded in the UI that the cluster only supports short-lived-token authentication with the cloud provider, so that I don't confuse the cluster with one that will try to mint long lived credentials.
* As an Operator Hub browser, I want to be able to easily provide what's required to the OLM operators I install through the UI.
* As the HyperShift team, where CCO is not installed so the only supported authentication mode is via short-lived-token authentication, I want the Red Hat branded operators that must reach the Cloud Provider API, to be enabled to work with short-lived-token credentials in a consistent, and automated fashion so that customer can use those operators as easily as possible, driving the use of layered products.

### Goals

(New) Redesign and document OLM process for short-lived-token authentication to remove the process of creating a credential request, replacing it with the direct creation of the necessary secret(s).

(New) Refactor existing OLM operators using the prior process by removing the creation of the credential request and replacing it with direct creation of the necessary secret(s).

(New) Deprecate and then remove the previously inserted mint mode functionality from the cloud credential operator while in manual mode.

Allow OLM installed operators to access cloud provider resources as seamlessly as possible when being installed, used and updated on STS enabled clusters.

While providing the above goals, allow for multi-tenancy. In this sense multi-tenancy means that an operator may enable its operands to communicate with the cloud provider instead of communicating with the cloud itself. Operands would use the same set of credentials as the operator. The operator will need to maintain its own logic to minimize conflicts when sharing credentials with operands.

Operator authors have a way to notify, guide, and assist OLM Operator admins in providing the required cloud provider credentials matched to their permission needs for install and update.

Ideally, a solution here will work in both HyperShift (short-lived-token always) and non-HyperShift(but short-lived-token-enabled) clusters.

### Non-Goals

In-payload/CVO-managed operators are out of scope.

Bring Your Own Credentials (BYOC) where an operator can manage and distribute credentials to its operands are out of scope.

Sharing a set of credentials between all operands to aid multi-tenancy.

## Proposal

(New) The proposal of this enhancement is to have the OLM operators that require short-lived-token integration to authenticate with the cloud provider(s) create the secret(s) for their components directly, thereby removing any current or future need to have these operators create a credentialsRequest. This includes refactoring the existing integrated operators to remove the credentialRequest logic in favor of creating the secret(s) directly. It also includes deprecating the cloud credential operator's call into mint mode functionality while in manual mode. And, finally, removing the STS functionality from the cloud credential operator altogether at a later release.

**OperatorHub and Console changes**: Allow for input from user of additional fields during install depending on the Cloud Provider which will result in ENV variables on the Subscription object that are required in the Secrets created by the Operator. Setting the Subscription config ENV will allow UX for the information needed by the operator in order to create the necessary secrets while not having to change the Subscription API. CLI Users will need to add the following to their subscription:

AWS
```yaml
kind: Subscription
metadata:
 name: ...
spec:
  config:
    env:
    - name: ROLEARN
      value: "<role ARN >"
```

Show in OperatorHub that the cluster is in a mode that supports short-lived-token authentication by reading the `.spec.serviceAccountIssuer` from the Authentication CR, `.status.platformStatus.type` from the Infrastructure CR, `.spec.credentialsMode` from the CloudCredentials CR.

Show that the operator is enabled for short-lived-token use by reading the [CSV](https://olm.operatorframework.io/docs/concepts/crds/clusterserviceversion/) annotation `features.operators.openshift.io/token-auth-aws` provided by the operator author.

Subscriptions to these types of operators will be manual by default in the UI. This is to ensure that these operators don't automatically get upgraded without first having the admin verify the permissions required by the next version and making the requisite changes if needed prior to upgrade.

**Operator team changes**: Follow new guidelines for allowing for the operator to work on short-lived-token auth enabled cluster. New guidelines would include the following to use when OLM has detected cluster is using time-based tokens:

- Add a bundle annotation claiming token-based authentication support:
    ```yaml
    apiVersion: operators.coreos.com/v1alpha1
    kind: ClusterServiceVersion
    metadata:
        annotations:
            features.operators.openshift.io/token-auth-aws: "true"
            ...
    ```
- Add a script in the operator description, per supported cloud provider, to help set up the correct role
- When OLM starts the operator with the cloud specific env. variable(s)
  - Create the necessary secrets to include the cloud provider specific fields as per the env values.
    - AWS:
      - `role_arn`
      - `web_identity_token_file`
    - Azure:
      - `azure_client_id`
      - `azure_federated_token_file`
      - `azure_region`
      - `azure_subscription_id`
      - `azure_tenant_id`
    - GCP:
      - `audience`
      - `service_account_impersonation_url`
      - `credential_source.file`
  - Wait for the Secret to be created.
    - Expect the Secret to take some time to create, do not crash and report an error when it takes too long.

### Workflow Description

Operator installation under the proposed system:
1. user goes to console and starts operator install
1. console detects token-based (STS) cluster, and token-based-supporting operator
1. console prompts user to create roles+supply cloud provider specific inputs
1. console creates subscription with the cloud provider specific inputs embedded as a spec.config.env
1. operator deployment is created with the cloud provider specific inputs in env
1. operator creates `secret` including the cloud provider specific inputs
1. operator loads Secret, makes cloud service requests the secret to be used by the operator's components for authentication to the cloud.

### API Extensions

Deprecate the following components from the api:
- CredentialsRequestSpec.CloudTokenPath
- AWSProviderSpec.STSIAMRoleARN
- AzureProviderSpec.AzureClientID
- AzureProviderSpec.AzureRegion
- AzureProviderSpec.AzureSubscriptionID
- AzureProviderSpec.AzureTenantID
- GCPProviderSpec.ServiceAccountEmail
- GCPProviderSpec.Audience

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

### Risks and Mitigations

### Drawbacks

## Alternatives (Not Implemented)

## Open Questions [optional]

## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

The ability for OLM operators to use short-term-token integration already exists. Each operator should use TechPreviewNoUpgrade when introducing the new feature or migrating from the old method to the new.

### Tech Preview -> GA

Each operator is expected to graduate to GA within the same release upon successfully demonstrating e2e test pass rate meets or exceeds OCP baselines.

### Removing a deprecated feature

The deprecated API fields will be noted as deprecated. They can continue to exist until the next major version, or be removed at a future time.

The deprecated functionality in the CCO operator will be removed in a future release only after all of the OLM operators are no longer relying on it to generate the secrets.

## Upgrade / Downgrade Strategy

## Version Skew Strategy

## Operational Aspects of API Extensions

There should be none. The deprecated fields can continue to exist even though they are no longer being used.

## Support Procedures

## Infrastructure Needed [optional]
