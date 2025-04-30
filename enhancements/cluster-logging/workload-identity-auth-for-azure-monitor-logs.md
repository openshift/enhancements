---
title: workload-identity-auth-for-azure-monitor-logs
authors:
  - "@calee"
reviewers:
  - "@jcantrill"
  - "@alanconway"
approvers:
  - "@jcantrill"
  - "@alanconway"
api-approvers:
  - "@jcantrill"
  - "@alanconway"
creation-date: 2025-04-30
last-updated: 2025-05-07
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/LOG-4782
---

# Workload Identity Auth for Azure Monitor Logs

## Release Sign-off Checklist

- [ X ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ X ] Test plan is defined
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

[Azure Monitor Logs](https://learn.microsoft.com/en-us/azure/azure-monitor/logs/data-platform-logs) is a comprehensive service provided by Microsoft Azure that enables the collection, analysis, and actioning of telemetry data across various Azure and on-premises resources.

This proposal enhances the Azure Monitor Logs integration by implementing secure, short-lived authentication using federated tokens with [Microsoft Entra Workload Identity (WID)](https://learn.microsoft.com/en-us/entra/workload-id/workload-identities-overview). This update will leverage the pending upstream Vector PR, [azure_logs_ingestion feature](https://github.com/vectordotdev/vector/pull/22912), which will utilize the new [Log Ingestion API](https://learn.microsoft.com/en-us/azure/azure-monitor/logs/logs-ingestion-api-overview).

## Motivation

The current Azure Monitor Logs integration relies on long-lived credentials ([shared_key](https://learn.microsoft.com/en-us/previous-versions/azure/azure-monitor/logs/data-collector-api?tabs=powershell#authorization)) and a deprecated [API](https://learn.microsoft.com/en-us/previous-versions/azure/azure-monitor/logs/data-collector-api), which poses potential security risks. Adopting WID will enhance security by providing short-lived credential access to Azure Monitor Logs and eliminate the dependency on this deprecated, soon-to-be-retired API.

### User Stories

- As an administrator, I want to be able to forward logs from my OpenShift cluster to Azure Monitor Logs using federated tokens, removing the need for long-lived, static credentials.

### Goals

- Enable the ClusterLogging Operator's Azure Monitor Logs sink to authenticate using short-lived federated token credentials.
- Enable the ClusterLogging Operator's Azure Monitor Logs sink to authenticate using static long-lived credentials with the Log Ingestion API.

### Non-Goals

## Proposal

To realize the goals of this enhancement:

- Switch over to the new Azure Log Ingestion sink when implemented in upstream Vector.
  - See #1 in [implementation details](#implementation-detailsnotesconstraints) section.
- Update upstream Vector's rust [Azure Identity](https://github.com/Azure/azure-sdk-for-rust/tree/main/sdk/identity/azure_identity) client library to `v0.23.0`.
  - See #2 in [implementation details](#implementation-detailsnotesconstraints) section.
- Extend upstream Vector's Azure Log Ingestion sink to accept additional configuration for workload identity authentication.
- Extend the ClusterLogForwarder’s Azure Monitor integration to support the required fields of the Log Ingestion API, including additional authentication fields for workload identity.

### Workflow Description

The Vector collector will:

1. Determine the authentication type using a configurable field (`credential_kind`).
2. Retrieve the Openshift Service Account token from the local volume.
3. Exchange the Openshift token with Microsoft identity platform for a short-lived access token.
4. Use the access token in the log forwarding request to Azure Monitor Logs.

The ClusterLogForwarder will:

1. Determine which authentication method to use based on a configurable field on the `azureMonitorAuthentication`.
2. Conditionally project the service account token if the authentication type is set to `workloadIdentity`.
3. Create the collector configuration with required fields for the Log Ingestion API along with the path to the projected service account token.

### Proposed API

#### Additional configuration fields for the `azureMonitor` output type to the `ClusterLogForwarder` API

Output

```Go
type AzureMonitor struct {
  ...// Keep rest of options

  // The Data Collection Endpoint (DCE) or Data Collection Rule (DCR) logs ingestion endpoint URL.
  //
  // https://learn.microsoft.com/en-us/azure/azure-monitor/logs/logs-ingestion-api-overview#endpoint
  //
  // +kubebuilder:validation:Optional
  // +kubebuilder:validation:XValidation:rule="isURL(self)", message="invalid URL"
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Url",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  URL string `json:"url,omitempty"`

  // The Data Collection Rule's (DCR) Immutable ID
  //
  // A unique identifier for the data collection rule. This property and its value are automatically created when the DCR is created.
  //
  // +kubebuilder:validation:Optional
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="DCR Immutable ID",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  DcrImmutableId string `json:"dcrImmutableId,omitempty"`

  // The stream in the Data Collection Rule (DCR) that should handle the custom data
  // 
  // https://learn.microsoft.com/en-us/azure/azure-monitor/data-collection/data-collection-rule-structure#input-streams
  //
  // +kubebuilder:validation:Optional
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Stream Name",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  StreamName string `json:"streamName,omitempty"`
}
```

Authentication

```Go
type AzureMonitorAuthentication struct {
  // Type is the type of Azure authentication to configure.
  //
  // Valid types are:
  //   1. sharedKey
  //   2. workloadIdentity
  //
  // +kubebuilder:validation:Required
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Authentication Type"
  Type AzureAuthType `json:"type"`

  // WorkloadIdentity
  //
  // +nullable
  // +kubebuilder:validation:Optional
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Workload Identity"
  WorkloadIdentity *AzureWorkloadIdentity `json:"workloadIdentity,omitempty"`

  // SharedKey points to the secret containing the shared key used for authenticating requests.
  //
  // +nullable
  // +kubebuilder:validation:Optional
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Shared Key"
  SharedKey *AzureSharedKey `json:"sharedKey,omitempty"`
}
```

```Go
type AzureSharedKey struct {
  // SharedKey points to the secret containing the shared key used for authenticating requests.
  //
  // +nullable
  // +kubebuilder:validation:Required
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Secret with Shared Key"
  SharedKey SecretReference `json:"sharedKey"`
}
```

```Go
type AzureWorkloadIdentity struct {
  // Token specifies a bearer token to be used for authenticating requests.
  //
  // +nullable
  // +kubebuilder:validation:Required
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Bearer Token"
  Token BearerToken `json:"token"`

  // ClientId points to the secret containing the client ID used for authentication.
  // 
  // https://learn.microsoft.com/en-us/azure/azure-monitor/data-collection/data-collection-rule-structure#input-streams
  //
  // +kubebuilder:validation:Required
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Secret with Client ID",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  ClientId SecretReference `json:"clientId"`

  // TenantId points to the secret containing the tenant ID used for authentication.
  // 
  // https://learn.microsoft.com/en-us/azure/azure-monitor/data-collection/data-collection-rule-structure#input-streams
  //
  // +kubebuilder:validation:Required
  // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Secret with Tenant ID",xDescriptors={"urn:alm:descriptor:com.tectonic.ui:text"}
  TenantId SecretReference `json:"tenantId"`
}
```

- The `TenantId` and `ClientId` can be retrieved from the [CCO utility secret](#cco-utility-secret) generated when Openshift is configured for [workload identity for Azure](https://github.com/openshift/cloud-credential-operator/blob/9c3346aea5a7f9a38713c09d11605b8ee825446c/docs/azure_workload_identity.md).

#### Additional configuration fields needed for upstream `Vector`'s Log Ingestion Sink API

```Rust
pub struct AzureLogsIngestionConfig {
  /// The [Federated Token File Path][federated_token_file_path] pointing to a federated token for authentication.
  ///
  /// [token_path]: https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow#third-case-access-token-request-with-a-federated-credential
  #[configurable(metadata(docs::examples = "/path/to/my/token"))]
  pub federated_token_file_path: Option<String>,

  /// The [Tenant ID][tenant_id] for authentication.
  ///
  /// [tenant_id]: https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow#third-case-access-token-request-with-a-federated-credential
  #[configurable(metadata(docs::examples = "11111111-2222-3333-4444-555555555555"))]
  pub tenant_id: Option<String>,

  /// The [Client ID][client_id] for authentication.
  ///
  /// [client_id]: https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow#third-case-access-token-request-with-a-federated-credential
  #[configurable(metadata(docs::examples = "11111111-2222-3333-4444-555555555555"))]
  pub client_id: Option<String>,
}
```

### Implementation Details/Notes/Constraints

1. Relies on [this upstream vector PR](https://github.com/vectordotdev/vector/pull/22912) to implement the Azure Log Ingestion sink utilizing the Log Ingestion API. This is a separate sink from the data collector API. We can support both APIs while transitioning.
2. Current `master` branch of [upstream Vector](https://github.com/vectordotdev/vector), `>=v0.46.1`, as of 04/29/2025, utilizes `azure_identity@v0.21.0` which relies soley on environment variables for workload identity credentials and will not be sufficient when forwarding to multiple different Azure sinks. The PR above relies on `azure_identity@v0.21.0`.
    - `azure_identity@v0.23.0` allows for setting `client_id`, `tenant_id`, etc. for authentication.
    - [Workload Identity Credentials azure_identity@v0.23.0 SDK Ref](https://github.com/Azure/azure-sdk-for-rust/blob/azure_identity%400.23.0/sdk/identity/azure_identity/src/credentials/workload_identity_credentials.rs)
    - `v0.23.0` can also utilize the [ChainedTokenCredential](https://github.com/Azure/azure-sdk-for-rust/blob/azure_identity%400.23.0/sdk/identity/azure_identity/src/chained_token_credential.rs) struct which provides a user-configurable `TokenCredential` authentication flow for applications.
3. The `customer_id` and `log_type` fields can now optional.
4. `Shared_key` field can now be optional with option to choose type of authentication.
5. Additional fields will be required in sink configuration in Vector's API. See [proposed API](#proposed-api) above.

#### Constraints

- CLO as of `v6.2` relies on `v0.37.0` of Openshift Vector. Openshift's Vector will have to be upgraded however, upgrade is currently blocked by rust toolchain's version for RHEL.  

#### CCO Utility Secret

After creation of a managed identity using the CCO utility. The following secret is created and can be used for authentication:

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
- The `azure_federated_token_file` cannot be used because the CLO projects the service account token in a custom path. (e.g `/var/run/ocp-collector/serviceaccount/token`).

### Open Questions

1. Do we also want to implement long-lived credential support using the Log Ingestion API?
2. Do we want to start deprecating the fields for the HTTP data collector API?

### Test Plan

- Manual E2E tests: Need access to Azure accounts along with an Openshift cluster configured to use Azure's workload identity.

## Alternatives (Not Implemented)

### Vector's Azure Identity Rust SDK

- Add a patch to `azure_identity` crate to allow setting `client_id`, `tenant_id`, etc. instead of relying on environment variables for workload identity credentials until Vector updates the crate version. See [implementation details](#implementation-detailsnotesconstraints).

### Risks and Mitigations

### Drawbacks

## Design Details

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

### API Extensions
