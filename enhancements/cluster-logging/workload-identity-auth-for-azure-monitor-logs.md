---
title: workload-identity-auth-for-azure-monitor-logs
authors:
  - "@calee"
reviewers:
  - "@jcantrill"
  - "@alanconway"
  - "@cahartma"
approvers:
  - "@jcantrill"
  - "@alanconway"
api-approvers:
  - "@jcantrill"
  - "@alanconway"
creation-date: 2025-04-30
last-updated: 2025-05-29
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/LOG-4782
---

# Workload Identity Auth for Azure Monitor Logs

## Release Sign-off Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

[Azure Monitor Logs](https://learn.microsoft.com/en-us/azure/azure-monitor/logs/data-platform-logs) is a comprehensive service provided by Microsoft Azure that enables the collection, analysis, and actioning of telemetry data across various Azure and on-premises resources.

This proposal enhances the `ClusterLogForwarder` API by introducing a new output type for Azure Monitor Logs, transitioning from the deprecated [Data Collector API](https://learn.microsoft.com/en-us/previous-versions/azure/azure-monitor/logs/data-collector-api) to the new [Log Ingestion API](https://learn.microsoft.com/en-us/azure/azure-monitor/logs/logs-ingestion-api-overview). The implementation
will include secure, short-lived authentication using federated tokens with [Microsoft Entra Workload Identity (WID)](https://learn.microsoft.com/en-us/entra/workload-id/workload-identities-overview), as well as long-lived credentials utilizing client secrets. This proposal will leverage the pending upstream Vector PR, [azure_logs_ingestion](https://github.com/vectordotdev/vector/pull/22912) feature.

## Motivation

The current Azure Monitor Logs integration relies on long-lived credentials via [shared_key](https://learn.microsoft.com/en-us/previous-versions/azure/azure-monitor/logs/data-collector-api?tabs=powershell#authorization) and a deprecated API posing potential security risks. Adopting WID will enhance security by providing short-lived credential access to Azure Monitor Logs and eliminate the dependency
on this deprecated, soon-to-be-retired API.

### User Stories

- As an administrator, I want to be able to forward logs from my OpenShift cluster to Azure Monitor Logs using federated tokens, thus removing the need for long-lived, static credentials.
- As a developer, I want to continue to forward logs from my OpenShift cluster to Azure Monitor Logs when the data collector API is retired.
- As an administrator, I want continue forwarding logs to Azure Monitor Logs with long-lived credentials utilizing the Log Ingestion API.

### Goals

- Extend the ClusterLogging Operator's API with a new Azure output type utilizing the Log Ingestion API.
- Allow authentication using short-lived federated token credentials with the Log Ingestion API.
- Allow authentication using static long-lived credentials with the Log Ingestion API.

### Non-Goals

## Proposal

To realize the goals of this enhancement:

- Switch to the new Azure Log Ingestion sink once it is implemented in upstream Vector.
  - See #1 in [implementation details](#implementation-detailsnotesconstraints) section.
- Update upstream Vector's Rust [Azure Identity](https://github.com/Azure/azure-sdk-for-rust/tree/main/sdk/identity/azure_identity) client library to `v0.23.0`.
  - See #2 in [implementation details](#implementation-detailsnotesconstraints) section.
- Extend upstream Vector's Azure Log Ingestion sink to accept additional configuration for workload identity authentication and client secrets.
- Add a new output type to `ClusterLogForwarder` that supports Azure's Log Ingestion API.
  - Include workload identity authentication.
  - Include long-lived credentials through `client secrets`.

### Workflow Description

The Vector collector will do the following for workload identity authentication:

1. Determine the authentication type to be workload identity when a token path is provided.
2. Retrieve the OpenShift service account token from the local volume.
3. Exchange the OpenShift token with Microsoft identity platform for a short-lived access token.
4. Use the access token in the log forwarding request to Azure Monitor Logs.

The Vector collector will do the following for credential secret authentication:

1. Determine the authentication type to be client secret when a client secret is provided.
2. Request an access token from the Microsoft identity platform using the client secret.
3. Use the access token in the log forwarding request to Azure Monitor Logs.


The Vector collector will do the following if both workload identity and credential secret authentication is defined:

1. Create a credentials chain with precedence for workload identity followed by the credential secret.
2. Retrieve the OpenShift service account token from the local volume.
3. Credential Exchange (in order):
    - **Service Account Token**: Exchange the OpenShift token with Microsoft identity platform for a short-lived access token.
    - **Credential Secret**: Request an access token from the Microsoft identity platform using the client secret.
4. Use the access token in the log forwarding request to Azure Monitor Logs.

The ClusterLogForwarder will:

1. Conditionally project the service account token if workload identity authentication is configured.
2. Create the collector configuration with the required fields for the Log Ingestion API, including either the path to the projected service account token or the client secret.

### Proposed API

#### New `AzureLogIngestion` output type

Output

```Go
// New output type of AzureLogIngestion which can live side by side with the AzureMonitor output
type AzureLogIngestion struct {
  // The Data Collection Endpoint (DCE) or Data Collection Rule (DCR) logs ingestion endpoint URL.
  //
  // https://learn.microsoft.com/en-us/azure/azure-monitor/logs/logs-ingestion-api-overview#endpoint
  //
  // +kubebuilder:validation:Required
  // +kubebuilder:validation:XValidation:rule="isURL(self)", message="invalid URL"
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Url",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  URL string `json:"url"`

  // The Data Collection Rule's (DCR) Immutable ID
  //
  // A unique identifier for the data collection rule. This property and its value are automatically created when the DCR is created.
  //
  // https://learn.microsoft.com/en-us/azure/azure-monitor/data-collection/data-collection-rule-structure#properties
  //
  // +kubebuilder:validation:Required
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="DCR Immutable ID",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  DcrImmutableId string `json:"dcrImmutableId"`

  // The stream in the Data Collection Rule (DCR) that should handle the custom data
  // 
  // https://learn.microsoft.com/en-us/azure/azure-monitor/data-collection/data-collection-rule-structure#input-streams
  //
  // +kubebuilder:validation:Required
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Stream Name",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  StreamName string `json:"streamName"`

  // Authentication sets credentials for authenticating the requests.
  //
  // +kubebuilder:validation:Required
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Authentication Options",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  Authentication *AzureLogIngestionAuth `json:"authentication"`

  // Tuning specs tuning for the output
  //
  // +kubebuilder:validation:Optional
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Tuning Options"
  Tuning *BaseOutputTuningSpec `json:"tuning,omitempty"`
}
```

Authentication

```Go
type AzureLogIngestionAuth struct {
  // ClientId points to the secret containing the client ID used for authentication.
  // 
  // This is the application ID that's assigned to your app. You can find this information in the portal where you registered your app.
  //
  // +kubebuilder:validation:Required
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Secret with Client ID",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  ClientId *SecretReference `json:"clientId"`

  // TenantId points to the secret containing the tenant ID used for authentication.
  //
  // The directory tenant the application plans to operate against, in GUID or domain-name format.
  //
  // https://learn.microsoft.com/en-us/azure/azure-monitor/data-collection/data-collection-rule-structure#properties
  //
  // +kubebuilder:validation:Required
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Secret with Tenant ID",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  TenantId *SecretReference `json:"tenantId"`

  // Token specifies a bearer token to be used for authenticating requests. If both Token and ClientSecret are defined, the Token will be tried first before the ClientSecret when making requests.
  //
  // https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow#third-case-access-token-request-with-a-federated-credential
  //
  // +kubebuilder:validation:Optional
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Bearer Token"
  Token *BearerToken `json:"token,omitempty"`

  // ClientSecret points to the secret containing the client secret.
  // A client secret is a secret string that the application uses to prove its identity when requesting a token. Also can be referred to as application password. If both Token and ClientSecret are defined, the Token will be tried first before the ClientSecret when making requests.
  //
  // https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow#first-case-access-token-request-with-a-shared-secret
  //
  // +kubebuilder:validation:Optional
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Secret with client secret"
  ClientSecret *SecretReference `json:"clientSecret,omitempty"`
}
```

- The `TenantId` and `ClientId` can be retrieved from the [CCO utility secret](#cco-utility-secret) generated when OpenShift is configured for [workload identity for Azure](https://github.com/openshift/cloud-credential-operator/blob/9c3346aea5a7f9a38713c09d11605b8ee825446c/docs/azure_workload_identity.md).

#### Additional configuration fields for upstream `Vector`'s Log Ingestion Sink API

```Rust
pub struct AzureLogsIngestionConfig {
  /// The [Client Secret][client_secret] for authentication.
  /// A secret string that the application uses to prove its identity when requesting a token. Also can be referred to as application password.
  ///
  /// [client_secret]: https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow#first-case-access-token-request-with-a-shared-secret
  #[configurable(metadata(docs::examples = "qWgdYAmab0YSkuL1qKv5bPX"))]
  pub client_secret: Option<String>,

  /// The [Federated Token File Path][federated_token_file_path] pointing to a federated token for authentication.
  ///
  /// [token_path]: https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow#third-case-access-token-request-with-a-federated-credential
  #[configurable(metadata(docs::examples = "/path/to/my/token"))]
  pub federated_token_file_path: Option<String>,

  /// The [Tenant ID][tenant_id] for authentication.
  ///
  /// The directory tenant the application plans to operate against, in GUID or domain-name format.
  ///
  /// [tenant_id]: https://learn.microsoft.com/en-us/azure/azure-monitor/data-collection/data-collection-rule-structure#properties
  #[configurable(metadata(docs::examples = "11111111-2222-3333-4444-555555555555"))]
  pub tenant_id: String,

  /// The [Client ID][client_id] for authentication.
  ///
  /// The client ID is the application ID that's assigned to your app. You can find this information in the portal where you registered your app.
  ///
  /// [client_id]: https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow#third-case-access-token-request-with-a-federated-credential
  #[configurable(metadata(docs::examples = "11111111-2222-3333-4444-555555555555"))]
  pub client_id: String,
}
```

### Implementation Details/Notes/Constraints

1. Relies on [this upstream vector PR](https://github.com/vectordotdev/vector/pull/22912) to implement the Azure Log Ingestion sink utilizing the Log Ingestion API. This is a separate sink from the data collector API. We can support both APIs while transitioning.
2. Current `master` branch of [upstream Vector](https://github.com/vectordotdev/vector), `>=v0.46.1`, as of 04/29/2025, utilizes `azure_identity@v0.21.0` which relies solely on environment variables for workload identity credentials and will not be sufficient when forwarding logs to multiple different Azure sinks. The aforementioned PR relies on `azure_identity@v0.21.0`.
    - `azure_identity@v0.23.0` allows for setting `client_id`, `tenant_id`, etc. for authentication.
    - [Workload Identity Credentials azure_identity@v0.23.0 SDK Ref](https://github.com/Azure/azure-sdk-for-rust/blob/azure_identity%400.23.0/sdk/identity/azure_identity/src/credentials/workload_identity_credentials.rs)
    - `v0.23.0` can also utilize the [ChainedTokenCredential](https://github.com/Azure/azure-sdk-for-rust/blob/azure_identity%400.23.0/sdk/identity/azure_identity/src/chained_token_credential.rs) struct which provides a user-configurable `TokenCredential` authentication flow for applications.
3. Additional fields will be required in sink configuration in upstream Vector's API. See [proposed API](#proposed-api) above.
4. A separate output type, `AzureLogIngestion`, will be added to work alongside the existing `AzureMonitor` output. When Microsoft retires the  Data Collector API, the `AzureMonitor` sink can also be deprecated and retired.
5. Long lived credentials will be implemented via `client secret` in the `AzureLogIngestion` output in the CLO API.

#### Constraints

- As of `v6.2`, CLO relies on `v0.37.1` of OpenShift Vector. OpenShift's Vector will have to be upgraded; however, the upgrade is currently blocked by the Rust version for RHEL.

#### CCO Utility Secret

After the creation of a managed identity using the CCO utility, the following secret is created and can be used for authentication:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: azure-test-secret
  namespace: openshift-logging
type: Opaque
stringData:
  azure_client_id: 11111111-2222-3333-4444-555555555555
  azure_tenant_id: 11111111-2222-3333-4444-555555555555
  azure_region: westus
  azure_subscription_id: 11111111-2222-3333-4444-555555555555
  azure_federated_token_file: /var/run/secrets/openshift/serviceaccount/token
```
- The `azure_federated_token_file` cannot be used because the CLO projects the service account token to a custom path. (e.g `/var/run/ocp-collector/serviceaccount/token`).

### Open Questions

1. Do we also want to implement long-lived credential support using the Log Ingestion API?
    - We will most likely implement long-lived tokens at the same time in the form of client secrets.
2. Do we want to start deprecating the fields for the HTTP data collector API?
    - The addition of a distinct output type will allow for easier deprecation of the existing API.

## Test Plan

- Manual E2E tests will require access to Azure accounts and an OpenShift cluster configured to use Azure's workload identity.

## Alternatives (Not Implemented)

### Vector's Azure Identity Rust SDK

- Consider adding a patch to `azure_identity` crate to allow setting `client_id`, `tenant_id`, etc. instead of relying on environment variables for workload identity credentials until Vector updates its crate version. See [implementation details](#implementation-detailsnotesconstraints).

### Risks and Mitigations

- As of `v6.2`, CLO relies on `v0.37.1` of OpenShift Vector. OpenShift's Vector will have to be upgraded; however, the upgrade is currently blocked by the Rust version for RHEL.

### Drawbacks

## Design Details

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Implementation History

### API Extensions

### Topology Considerations

#### Hypershift / Hosted Control Planes

#### Standalone Clusters

#### Single-node Deployments or MicroShift
