---
title: loki-tokenized-auth-enablement
authors:
  - "@periklis"
reviewers:
  - "@cahartma"
  - "@xperimental"
  - "@JoaoBraveCoding"
  - "@btaani"
approvers:
  - "@jcantrill"
  - "@alanconway"
api-approvers:
  - "@xperimental"
creation-date: 2023-10-27
last-updated: 2023-10-27
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-6
  - https://issues.redhat.com/browse/OCPSTRAT-171 (AWS STS)
  - https://issues.redhat.com/browse/OCPSTRAT-114 (Azure WIF)
  - https://issues.redhat.com/browse/OCPSTRAT-922 (GCP WIF)
  - https://issues.redhat.com/browse/LOG-4540 (AWS & Azure)
  - https://issues.redhat.com/browse/LOG-4754 (GCP)
see-also:
  - "/enhancements/cloud-integration/tokenized-auth-enablement-operators-on-cloud.md"
replaces:
  - []
superseded-by:
  - []
---

# LokiStack Tokenized Authentication on Cloud Providers

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [x] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Public cloud providers offer services that allow authentication via short-lived tokens assigned to a limited set of privileges. Currently, OpenShift supports provisioning token-based authentication on all public cloud providers via the Cloud Credential Operator (CCO), i.e. AWS STS (4.14.0), Azure (4.14.11) and GCP (4.16.0).

This enhancement enables the Loki Operator to leverage CCO resources (i.e. `CredentialsRequest`) and configure LokiStack instances for object storage access via token-based authentication. This improves the user experience for Red Hat OpenShift Logging users in general (especially on product offerings as ROSA and ARO) and Loki Operator users in particular and makes the approach more seamless and unified compared to other Red Hat Operators (e.g. cert manager for Red Hat OpenShift, OADP operator).

## Motivation

As previously accomplished by other Red Hat and third-party operators (e.g. cert manager for Red Hat OpenShift, OADP Operator, External DNS Operator, AWS EFS CSI Driver Operator) the following proposal seeks to use CCO's `CredentialsRequest` resource in addition to the OpenShift Console OLM capabilities to detect STS-enabled clusters and manage the whole operator lifecycle.

### User Stories

* As a LokiStack administrator I want to manage token-based authentication via AWS Secure Token Service (STS) for for all LokiStack instances.
* As a LokiStack administrator I want to manage token-based authentication via Azure Workload Identity Federation (WIF) for for all LokiStack instances.
* As a LokiStack administrator I want to manage token-based authentication via Google Workload Identity (WI) for for all LokiStack instances.
* As a LokiStack administrator I want to manage token-based authentication once on operator installation.
* As a LokiStack administrator I want to manually manager operator upgrades on STS/WIF/WI managed clusters to ensure IAM roles match object storage access requirements.

### Goals

* Allow LokiStack administrators to manage LokiStack cloud provider related configuration (i.e. object storage service access) like with any other Red Hat Operator using for token-based authentication.
* Allow OpenShift administrators to provision cloud provider IAM resources upfront for the entire Red Hat OpenShift Logging product and Loki Operator in particular before installation.
* Allow Hosted Control Planes (HCP) and Red Hat OpenShift on AWS (ROSA) cluster administrators to to provision Hosted Provider IAM resources upfront for the entire Red Hat OpenShift Logging product and for the Loki Operator in particular before installation.

### Non-Goals

Automating pre-provisioned cloud providers' IAM resources so that users are left only with the Loki Operator and the LokiStack resource installation.

## Proposal

The Loki Operator supports per Cloud Provider the following annotations in its respective ClusterServiceVersion per bundle (community-openshift, openshift):

- For AWS STS: `features.operators.openshift.io/token-auth-aws: true`
- For Azure WIF: `features.operators.openshift.io/token-auth-azure: true`
- For GCP WIF: `features.operators.openshift.io/token-auth-gcp: true`

These enable the operator installation to require specific IAM resource identifiers (e.g. role, client id, tenant id, etc.). The operator subscription is populated with the identifiers and exposed in the operator container environment.

### Workflow Description

The workflow implemented by this proposal follows the flow presented in the [Tokenized Authentication Enablement for Red Hat Operators on Cloud Providers](tokenized-auth-enablement-workflow) enhancement proposal.

For the LokiStack administrator using the OpenShift Console - Operator installation under the proposed system:

1. The user goes to console and starts operator install
2. Console detects token-based (STS) cluster, and token-based-supporting operator
3. Console prompts user to create roles and supply ARN-type string
4. Console creates subscription with ARN-type string embedded as a spec.config.env
5. Operator deployment is created with ARN-type string in env
6. Operator creates `CredentialsRequest` including the ARN-type string
7. Cloud Credential Operator populates Secret based on `CredentialsRequest`
8. Operator loads Secret and makes cloud service requests

For the LokiStack admninistator using the OpenShift CLI - Operator installation under the proposed system:

- For AWS:
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

- For Azure:
```yaml
kind: Subscription
metadata:
 name: ...
spec:
  config:
    env:
    - name: CLIENT_ID
      value: "<azure client id>"
    - name: TENANT_ID
      value: "<azure tenant id>"
    - name: SUBSCRIPTION_ID
      value: "<azure subscription id>"
    - name: REGION
      value: "centralus"
```

- For GCP: TBD along 4.16.

For the Loki Operator author team:
- Add code to create a `CredentialsRequest` per `LokiStack` resource.
- Add IAM-identifier-type fields to the `CredentialsRequest` resources.
- Add eventing to report status on a CR to indicate lacking STS credentials for fully operational deploy or update.

#### AWS STS Support

The Loki Operator will use the `ROLEARN` environment variable to create a `CredentialsRequest` resource in the same namespace as the LokiStack resource. The `ROLEARN` value is placed in the `spec.providerSpec.stsIAMRoleARN` field. The `CredentialsRequest` resource references both LokiStack service-account names, the main one which is the same as the LokiStack resource name and the ruler service-account. The LokiStack administrator is required to provide an AWS Region via the AWS object storage secret.

```yaml
apiVersion: cloudcredential.openshift.io/v1
kind: CredentialsRequest
metadata:
  name: logging-loki
  namespace: openshift-logging
spec:
  cloudTokenPath: /var/run/secrets/storage/serviceaccount/token
  providerSpec:
    apiVersion: cloudcredential.openshift.io/v1
    kind: AWSProviderSpec
    statementEntries:
    - action:
      - s3:ListBucket
      - s3:PutObject
      - s3:GetObject
      - s3:DeleteObject
      effect: Allow
      resource: arn:aws:s3:*:*:*
    stsIAMRoleARN: arn:aws:iam::${AWS_ACCOUNT_ID}:role/openshift-logging-loki-operator-controller-manager
  secretRef:
    name: logging-loki-managed-credentials
    namespace: openshift-logging
  serviceAccountNames:
  - logging-loki
  - logging-loki-ruler
```

#### Azure WIF Support

The Loki Operator will use the provided environment variables (`CLIENT_ID`, `TENANT_ID`, `SUBSCRIPTION_ID`) to create a `CredentialsRequest` resource in the same namespace as the LokiStack resource. Optionally, a region can be provided as well (using `REGION`). The values will be used in the `CredentialsRequest` fields `spec.providerSpec.{AzureClientID,AzureTenantID,AzureSubscriptionID,AzureRegion}` respectively. If the region is not provided, then the operator will fall back to using `centralus`, as the region is only used in the `CredentialsRequest` and not when using the object storage.

```yaml
apiVersion: cloudcredential.openshift.io/v1
kind: CredentialsRequest
metadata:
  name: logging-loki
  namespace: openshift-logging
spec:
  cloudTokenPath: /var/run/secrets/storage/serviceaccount/token
  providerSpec:
    apiVersion: cloudcredential.openshift.io/v1
    kind: AzureProviderSpec
    AzureClientID: clientid
    AzureTenantID: tenantid
    AzureRegion: centralus
    AzureSubscriptionID: subscriptionid
  secretRef:
    name: logging-loki-managed-credentials
    namespace: openshift-logging
  serviceAccountNames:
  - logging-loki
  - logging-loki-ruler
```

#### GCP WIF Support

GCP WIF support will be supported only in the form of the upstream support as per CCO missing respectable bits.

### API Extensions

The present proposal introduces a new optional field in the `ObjectStorageSecretSpec` namely `CredentialsMode`. If the user does not provide a value the `CredentialsMode` is automatically detected either from the provided secret fields or the operator environment variables (latter applies only in OpenShift cluters.). The selected or detected `CredentialsMode` is populated in addition in the `status.storage.credentialMode` field.

```go
/ CredentialMode represents the type of authentication used for accessing the object storage.
//
// +kubebuilder:validation:Enum=static;token;token-cco
type CredentialMode string

const (
    // CredentialModeStatic represents the usage of static, long-lived credentials stored in a Secret.
    // This is the default authentication mode and available for all supported object storage types.
    CredentialModeStatic CredentialMode = "static"
    // CredentialModeToken represents the usage of short-lived tokens retrieved from a credential source.
    // In this mode the static configuration does not contain credentials needed for the object storage.
    // Instead, they are generated during runtime using a service, which allows for shorter-lived credentials and
    // much more granular control. This authentication mode is not supported for all object storage types.
    CredentialModeToken CredentialMode = "token"
    // CredentialModeTokenCCO represents the usage of short-lived tokens retrieved from a credential source.
    // This mode is similar to CredentialModeToken, but instead of having a user-configured credential source,
    // it is configured by the environment and the operator relies on the Cloud Credential Operator to provide
    // a secret. This mode is only supported for certain object storage types in certain runtime environments.
    CredentialModeTokenCCO CredentialMode = "token-cco"
)

// ObjectStorageSecretSpec is a secret reference containing name only, no namespace.
type ObjectStorageSecretSpec struct {
...
    // CredentialMode can be used to set the desired credential mode for authenticating with the object storage.
    // If this is not set, then the operator tries to infer the credential mode from the provided secret and its
    // own configuration.
    //
    // +optional
    // +kubebuilder:validation:Optional
    CredentialMode CredentialMode `json:"credentialMode,omitempty"`
}
```
The purpose of the `CredentialMode` is to override the detected credentials type from object storage secrets or the operator environment variables. Latter is only supported on AWS-STS/Azure-WIF managed OpenShift clusters, where the operator is using CredetialMode `token-cco` by default. However, the user might want to use `static` to store logs for example to Minio on the same cluster instead to AWS S3.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Not applicable here.

#### Standalone Clusters

Not applicable here.

#### Single-node Deployments or MicroShift

Not applicable here.

### Implementation Details/Notes/Constraints [optional]

The Loki Operator will generate the Loki configuration needed to operate with tokenized authentication based on the credential secret created by the reconciliation of the `CredentialRequest` resource. This Loki configuration will change depending on the Cloud Provider as follows.

__Note__: The LokiStack components requiring access to object storage are: `ingester`, `querier`, `index-gateway`, `compactor` and `ruler`.

#### AWS STS Support

For AWS assuming the following credentials secret provided by CCO:

```yaml
apiVersion: v1
stringData:
  credentials: |-
    [default]
    role_arn = <role ARN>
    web_identity_token_file = <path to mounted service account token such /var/run/secrets/storage/serviceaccount/token>
kind: Secret
metadata:
  name: logging-loki-aws-credentials
  namespace: openshift-logging
type: Opaque
```

Each Loki container will gain two new environment variables:

```yaml
containers:
  - args:
    - -target=ingester
    - -config.file=/etc/loki/config/config.yaml
    - -runtime-config.file=/etc/loki/config/runtime-config.yaml
    - -config.expand-env=true
    env:
    - name: AWS_SDK_LOAD_CONFIG
      value: true
    - name: AWS_SHARED_CREDENTIALS_FILE
      value: /etc/storage/token-auth/credentials
```

In addition the `LokiStack` ConfigMap `logging-loki-config` needs to provide an `s3_storage_config` as below:

```yaml
common:
  storage:
    s3:
      s3: "s3://<region>/<bucketnames>"
      s3forcepathstyle: false
```

__Note:__ The above configuration is a workaround as suggested by the [grafana/loki#5437](grafana-loki-issue-5437) issue.

#### Azure WIF Support

For Azure assuming the following credentials secret provided by CCO:

```yaml
apiVersion: v1
stringData:
  azure_client_id: <client id>
  azure_federated_token_file: <path to mounted service account token such as /var/run/secrets/storage/serviceaccount/token>
  azure_region: <region>
  azure_subscription_id: <subscription id>
  azure_tenant_id: <tenant id>
kind: Secret
metadata:
  name: logging-loki-azure-credentials
  namespace: openshift-logging
type: Opaque
```

Each Loki container will gain the following environment variables:

```yaml
containers:
  - args:
    - -target=ingester
    - -config.file=/etc/loki/config/config.yaml
    - -runtime-config.file=/etc/loki/config/runtime-config.yaml
    - -config.expand-env=true
    env:
    - name: AZURE_CLIENT_ID
      value: <client id>
    - name: AZURE_TENANT_ID
      value: <tenant id>
    - name: AZURE_SUBSCRIPTION_ID
      value: <subscription id>
    - name: AZURE_FEDERATED_TOKEN_FILE
      value: /var/run/secrets/storage/serviceaccount/token
```

#### GCP WIF Support

GCP WIF support will be supported only in the form of the upstream support as per CCO missing respectable bits.

### Risks and Mitigations

### Drawbacks

## Design Details

The main design is described in the upstream enhancement prospoal here: https://github.com/grafana/loki/pull/11060

## Test Plan

The [loki-operator](https://github.com/grafana/loki) includes a framework based on [counterfeiter](github.com/maxbrunsfeld/counterfeiter) to create simple and useful fake client to test configuration in unit tests.

Testing of the reconciliation of a `CredentialsRequest` custom resource will be based upon the same technique where the custom resource describes the inputs and the generated manifests the outputs.

## Graduation Criteria

### Dev Preview -> Tech Preview

Not applicable here.

### Tech Preview -> GA

Not applicable here.

### Removing a deprecated feature

Not applicable here.

## Upgrade / Downgrade Strategy

The workflow does not support any automatic upgrades as the CCO capabilities on the OpenShift OLM Console are required upon installation time only.

## Version Skew Strategy

Not applicable here.

## Operational Aspects of API Extensions

Not applicable here.

### Failure Modes

The LokiStack resource will support another degraded error condition if the CredentialsRequest is not returning a valid secret.

## Support Procedures

## Implementation History

| Release | Description              |
| ---     | ---                      |
| 5.9.0   | **GA** - Initial release |

## Alternatives

Not applicable here.

## Infrastructure Needed [optional]

[tokenized-auth-enablement-workflow]:https://github.com/openshift/enhancements/blob/master/enhancements/cloud-integration/tokenized-auth-enablement-operators-on-cloud.md#workflow-description
[grafana-loki-issue-5437]:https://github.com/grafana/loki/issues/5437#issuecomment-1158862015
