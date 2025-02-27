---
title: gcp-external-account-auth
authors:
  - "@cahartma"
reviewers:
  - "@alanconway"
  - "@jcantrill"
  - "@Clee2691"
  - "@JoaoBraveCoding"
  - "@xperimental"
approvers:
  - "@jcantrill"
  - "@alanconway"
api-approvers: 
  - "@jcantrill"
creation-date: 2025-02-27
last-updated: 2024-02-27
tracking-link:
  - "https://issues.redhat.com/browse/LOG-3577"
see-also:
  - "https://issues.redhat.com/browse/LOG-3275"
  - "https://issues.redhat.com/browse/LOG-6762"
replaces: []
superseded-by: []
---

# GCP Workload Identity Federation (WIF) Support in OpenShift Logging

This enhancement enables secure, short-lived authentication for GCP services in OpenShift Logging using Workload Identity Federation

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement adds support for Workload Identity Federation (WIF) to the existing GCP 
authentication mechanism in Vector. With this update, Vector will be able to authenticate 
using external_account credentials when running in an OpenShift environment, enabling OIDC-based 
authentication with short-lived access tokens instead of static credentials.

## Motivation

Current GCP authentication in Vector relies on service account credentials, which require 
static key management. WIF eliminates the need for long-lived credentials, enabling more secure, 
short-lived authentication via OpenShift service account tokens.

### User Stories

* As an OpenShift administrator, I want Vector to authenticate with GCP using Workload Identity Federation, 
removing the need for static service account keys. 
* As an engineer, I want Vector to detect the correct authentication method based on the credentials file 
format, utilizing the correct credentials chain form the sdk, and ensuring compatibility with 
both service accounts and external accounts (WIF).


### Goals

* Modify the GCP module in vector to support external_account credentials. 
* Introduce new structs (ExternalCredentials, CredentialsType) to handle the updated credential format. 
* Serialize the configuration file into usable data 
* Implement the full OIDC-based authentication flow. 
* Ensure compatibility with existing service account authentication


### Non-Goals

* Modifying the OpenShift gcp output authentication model. 
* Supporting authentication methods outside of Google Cloud.


## Proposal

To integrate WIF, Vector’s authentication logic will be updated to support external_account 
credentials in addition to the existing service_account credentials. The process will:

* Determine the authentication type by reading the credentials JSON file. 
* Extract the OpenShift Service Account token from the projected volume. 
* Exchange the OpenShift token for a Google Identity Token using Google STS. 
* Exchange the Identity Token for a short-lived Access Token via Google IAM impersonation. 
* Ensure the existing service account authentication remains functional.

### Workflow Description
### API Extensions
The existing credentials key and secret works for our purpose, although we should aim to eventually  
align with our cloudwatch and future Azure specs which are most likely going to be configMaps:
```yaml
 googleCloudLogging:
   authentication:
     credentials:
       key: google-ext-account-creds.json
       secretName: gcp-secret
   logId : my-gcp
...
```
### Topology Considerations
#### Hypershift / Hosted Control Planes
#### Standalone Clusters
#### Single-node Deployments or MicroShift
### Implementation Details/Notes/Constraints

1. Modify Authentication to Support external_account  
   * Update authentication logic to check for external_account type.
   * Add a CredentialsType struct to differentiate between service_account and external_account.
2. Introduce ExternalCredentials Struct   
   * Create a new struct to handle external_account authentication fields.
   * Ensure compatibility with Google’s OIDC-based credential format.
3. Implement the WIF Authentication Flow   
   * Step 1: Read the OpenShift service account token from the path specified in the credentials file   
   * Step 2: Exchange the OpenShift token for a Google Identity Token via STS.   
   * Step 3: Exchange the Identity Token for an Access Token via IAM impersonation.  
4. Ensure Compatibility with service_account Type  
   * The existing authentication flow for service_account remains unchanged.

#### Credentials File Example
(external_account type)
```json
{
  "type": "external_account",
  "audience": "//iam.googleapis.com/...<workload_identity_pool_provider>",
  "token_url": "https://sts.googleapis.com/v1/token",
  "service_account_impersonation_url": "https://iamcredentials......iam.gserviceaccount.com:generateAccessToken",
  "credential_source": { 
    "file": "/var/run/ocp-collector/serviceaccount/token"
  }
}

```

#### OIDC Request Examples

##### How Is The Bound Token Used in GCP Workload Identity Federation (WIF)?
1. Pod gets the Bound Service Account Token
   * This token is created automatically by OpenShift and mapped to the service account assigned to the pod.
2. The OpenShift Pod sends this token to GCP STS  
   * This is Step 1 of the WIF authentication process.
   * The token is sent to Google's Security Token Service (STS) API: https://sts.googleapis.com/v1/token
and is included in the subject_token field of the next request.   
3. GCP STS validates the OpenShift token and exchanges it for an OIDC token
   * This step verifies that the OpenShift cluster is a trusted identity provider for GCP.
   * The result is a Google Identity Token (OIDC token).
4. The Identity Token is then exchanged for a Google Access Token using IAM impersonation
   * The access token allows access to GCP resources.

##### Example: How The Token Is Used in WIF Authentication
When sending the OpenShift "bound" service account token to Google's STS, the payload looks like:
```json
{
  "grant_type": "urn:ietf:params:oauth:grant-type:token-exchange",
  "requested_token_type": "urn:ietf:params:oauth:token-type:access_token",
  "subject_token_type": "urn:ietf:params:oauth:token-type:jwt",
  "subject_token": "<OpenShift Bound Service Account Token>",
  "audience": "//iam.googleapis.com/projects/PROJECT_ID/locations/global/workloadIdentityPools/POOL_ID/providers/PROVIDER_ID"
}
```
When sending to OIDC token, the endpoint is the `service_account_impersonation_url`, the header includes
the `identity_token` from the previous step as a Bearer token, and the payload looks like:
```json
{
  "delegates": [],
  "scope": ["https://www.googleapis.com/auth/logging.write"],
  "lifetime": "3600s"
}
```
This call requests a GCP access token using the Identity Token

### Risks and Mitigations

This change involves configuration of authentication and is high-risk.
The mitigation is to provide clear validation checks and logs for troubleshooting auth issues

### Drawbacks
## Test Plan

* Ensure credentials file and OpenShift service account token are correctly read. 
* Verify STS request returns a valid Google Identity Token. 
* Verify impersonation request returns a valid Google Access Token. 
* Ensure existing service_account authentication remains functional. 
* Confirm Stackdriver logs are sent successfully using the new access token. 
* Create unit tests for get_credentials_type, fetch_bound_service_account_token, 
fetch_identity_token_from_sts, and fetch_impersonated_token.


## Alternatives

Wait for vector to swap out their gcp auth library or wait for existing lib to be updated

## WIP
- https://github.com/ViaQ/vector/tree/gcp-wif-progress

## Graduation Criteria
### Dev Preview -> Tech Preview
### Tech Preview -> GA
### Removing a deprecated feature
## Upgrade / Downgrade Strategy
## Version Skew Strategy
## Operational Aspects of API Extensions
## Support Procedures