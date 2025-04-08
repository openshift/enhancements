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
This enhancement enables secure, short-lived authentication for forwarding logs to GCP services in 
OpenShift Logging using Workload Identity Federation

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary
This enhancement extends the GCP authentication method used by the ClusterLogForwarder and the Vector collector, 
enabling Workload Identity Federation (WIF).  With this update the OpenShift Logging Admin can specify an 'external_account' 
credentials file when forwarding logs to GCP, eliminating the need for storing long-lived, static keys for authentication.

## Motivation
Using WIF for authentication is an important security feature that is becoming the standard for authentication
across the OpenShift cluster, eliminating the need for manual human intervention to store, rotate and secure 
static keys in Secrets.  The ClusterLogForwarder is currently blocked in this effort due to lack of upstream 
support for the feature in our collector. This enhancement would enable the feature in upstream Vector,
allowing the ClusterLogForwarder to be configured to successfully authenticate by using WIF.

### User Stories
* As an OpenShift administrator, I want the ClusterLogForwarder to use a credential file of type 'external_account'
to forward logs to GCP Cloud Logging service, utilizing WIF and removing the need for long-lived, static keys.
* As an engineer, I want the Vector collector to authenticate using an 'external_account' credential file type 
configured in the ClusterLogForwarder, successfully forwarding logs to GCP, while ensuring compatibility with 
both service_account and external_account (WIF) credentials file types.

### Goals
* Modify the GCP module in upstream Vector to support 'external_account' credentials. 
  * Get the PR approved and merged into upstream.
* Implement the updated GCP module into the OpenShift build of Vector.
* Enable the feature in ClusterLogForwarder for the GCP output so that collector can authenticate and 
forward logs to GCP using WIF
* Ensure compatibility with existing key-based GCP 'service_account' authentication

### Non-Goals
* Modifying the ClusterLogForwarder Spec or OpenShift Observability API Model. 
* Supporting GCP authentication methods other than credentials files of type 'service_account' or 'external_account'.

## Proposal
Vector lacks an official SDK for their Rust Language and the current library is not well maintained. To 
integrate WIF, Vector’s GCP authentication module will need to be extended to support 'external_account' 
credential file type. 

The Vector collector will:

* Determine the authentication type by reading the credentials JSON file. 
* Extract the OpenShift Service Account token from the local volume. 
* Exchange the OpenShift token for a Google Identity Token using Google STS. 
* Exchange the Identity Token for a short-lived Access Token via Google IAM impersonation. 
* Use the resulting Access Token in the log-forwarding request to GCP services.

The ClusterLogForwarder will:

* Read the 'type' from the configured credentials file, specified in a secret by the user.
* Conditionally project the service account token if the type is 'external_account'
* Create the collector configuration in the same way regardless of type, pointing to the path of the secret
as the 'credentials_path'.

### Workflow Description
None
### API Extensions
No changes are necessary.  The existing `credentials` spec containing `key` and `secretName` can be used:
```yaml
 googleCloudLogging:
   authentication:
     credentials:
       key: google-credentials-file.json
       secretName: gcp-secret
```

### Topology Considerations
#### Hypershift / Hosted Control Planes
#### Standalone Clusters
#### Single-node Deployments or MicroShift
### Implementation Details/Notes/Constraints
The credentials file is created by the OpenShift Admin, and configured in a Secret to be used by the collector (example spec above).

#### Credentials File Examples
The current `service_account` type contains sensitive GCP Service Account keys and the data is used directly to make requests: 
```json
{
   "type": "service_account",
   "project_id": "<gcp-project-id>",
   "private_key_id": "9faaexamplee4123456abcd51b82cf65",
   "private_key": "-----BEGIN PRIVATE KEY-----\n ... \n-----END PRIVATE KEY-----\n",
   "client_email": "atest@gcp-project-id.iam.gserviceaccount.com",
   "client_id": "<client-id>",
   "auth_uri": "https://accounts.google.com/o/oauth2/auth",
   "token_uri": "https://oauth2.googleapis.com/token"
}
```
The new `external_account` credentials file type contains no sensitive information and is 
used in the token-exchange process to create short-lived access tokens (WIF):
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
**Important:** The `credential_source.file` is the path to the bound service account token, projected into the pod 
by the ClusterLogForwarder.  The Logging Admin has no control or input into this path and the value is **required** 
to match our chosen implementation path (TBD).

### Risks and Mitigations 
* To complete the enhancement, a change to upstream Vector is necessary and is high-risk because:  
  * The current Rust library in use is 'goauth' and it is not well maintained.
  * The upstream feature request has not been implemented in several years, likely because an SDK for GCP
    does not exist for the Rust language. 
  * We lack experience with Rust and modifying the Vector project.
  * Authentication features are always high-risk.
* The best way to mitigate risk is to have the PR approved by the upstream community.
  * This combined with our feature testing should provide confirmation that the solution is viable

Note: There is low risk in the ClusterLogForwarder implementation, mainly because the authentication 
logic is abstracted by the collector, but also because we have existing outputs using this method. A typical 
OpenShift Logging release cycle and functional testing should handle any concerns with implementation. 

### Drawbacks
None

## Test Plan 
* Create unit tests for: 
  * parsing the credential type from the secret
  * conditionally projecting the service account token
* Functional tests to ensure validation handles missing 'type' or otherwise invalid credentials file 
* User testing to confirm the (bound) service account token is projected into the collector, when forwarded to GCP Cloud Logging
  and the Secret contains a credentials file of type 'external_account'
* User testing to confirm there are no errors, the collector is authenticated and logs are forwarded to GCP Cloud Logging.
* User testing of token-renewal after one hour, ensuring the collector is able to reload the latest token.

## Alternatives (Not Implemented)
* Build our own authentication 'side-car' service that handles the authentication process outside the collector, 
managing the WIF token-exchange process and maintaining a valid access token on behalf of the collector.
* Wait longer for vector to swap out their entire GCP 'goauth' library, or wait for library to be updated then 
pulled into upstream vector.  

## Graduation Criteria
### Dev Preview -> Tech Preview
### Tech Preview -> GA
### Removing a deprecated feature
## Upgrade / Downgrade Strategy
## Version Skew Strategy
## Operational Aspects of API Extensions
## Support Procedures
