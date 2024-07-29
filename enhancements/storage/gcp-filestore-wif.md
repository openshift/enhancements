---
title: gcp-filestore-wif
authors:
  - "@RomanBednar"
reviewers:
  - "@dobsonj"
  - "@fbertina"
  - "@gnufied"
  - "@jsafrane"
  - "@mpatlasov"
approvers:
  - "bbennett"
api-approvers:
  - "None"
creation-date: 2024-07-30
last-updated: 2024-07-30
tracking-link:
  - "https://issues.redhat.com/browse/STOR-1988"
see-also:
  - "None"
replaces:
  - "None"
superseded-by:
  - "None"
---

# GCP Filestore Workload Identity Federation

## Summary

The GCP Filestore operator will be updated to utilize Workload Identity Federation (WIF) for secure authentication with Google Cloud services.
This enhancement involves modifications to the operator to create `CredentialsRequest` that can be processed by Cloud Credential Operator (CCO) in WIF clusters.

## Motivation

Implementing WIF for the GCP Filestore operator will allow the use of short-lived credentials, enhancing security by reducing the reliance on long-lived credentials.

### User Stories

- As an OpenShift administrator, I want the GCP Filestore driver to authenticate using WIF, so I don't need to manage long-lived credentials.
- As an OpenShift administrator, I want to configure WIF through the OpenShift console (Operator Hub), ensuring a streamlined setup process.
- As an OpenShift administrator, I want to transition from my current short term credentials configuration to officially supported WIF configuration.

### Goals

- The GCP Filestore operator should create `CredentialsRequest` resources with the new required fields in WIF clusters.
- Ensure the operator has the necessary RBAC permissions to create `CredentialsRequest`.
- Define a volume and a mount in the ClusterServiceVersion (CSV) for bound service account token.
- Update the CSV to announce WIF support.

### Non-Goals

- This enhancement does not cover creating Managed Identity infrastructure on GCP.
- Detailed role granularity for permissions in `CredentialsRequest`.
- Enabling WIF mode as a day-2 operation (might be added later as Phase 2).

## Proposal

### GCP Filestore Operator Changes

1. **CredentialsRequest Creation**:
    - The GCP Filestore operator will create `CredentialsRequest` resource and populate new API fields if needed: [openshift/cloud-credential-operator#708](https://github.com/openshift/cloud-credential-operator/pull/708).
    - Credentials request controller hook has to be updated to handle the new fields and populate them if all environment variables are set, in that case `spec.providerSpec.PredefinedRoles` field should not be set.
    - Optionally, we can set `spec.cloudTokenPath` to `/var/run/secrets/openshift/serviceaccount/token` instead of relying on CCO defaults to make sure the secret created by CCO references the correct token path.
    - Example of `CredentialsRequest` resource with WIF enabled:
```yaml
apiVersion: cloudcredential.openshift.io/v1
kind: CredentialsRequest
metadata:
   name: openshift-gcp-filestore-csi-driver-operator
   namespace: openshift-cloud-credential-operator
   annotations:
      include.release.openshift.io/self-managed-high-availability: "true"
      include.release.openshift.io/single-node-developer: "true"
spec:
   cloudTokenPath: /var/run/secrets/openshift/serviceaccount/token
   serviceAccountNames:
      - gcp-filestore-csi-driver-operator
      - gcp-filestore-csi-driver-controller-sa
   secretRef:
      name: gcp-filestore-cloud-credentials
      namespace: ${NAMESPACE}
   providerSpec:
      apiVersion: cloudcredential.openshift.io/v1
      kind: GCPProviderSpec
      poolID: ${POOL_ID}
      providerID: ${PROVIDER_ID}
      serviceAccountEmail: ${SERVICE_ACCOUNT_EMAIL}
      audience: //iam.googleapis.com/projects/${PROJECT_NUMBER}/locations/global/workloadIdentityPools/${POOL_ID}/providers/${PROVIDER_ID}
      skipServiceCheck: false
```


2. **Field Values**:
    - The values for `PoolID`, `ProviderID` and `ServiceAccountEmail` fields will be input by users via the OpenShift console and set in the `Subscription` spec configuration by OLM: [openshift/console#14086](https://github.com/openshift/console/pull/14086).
    - Mapping of the field names:

      | Dashboard name        | CR field name                         | Env var name          |
      |-----------------------|---------------------------------------|-----------------------|
      | GCP Tenant ID         | spec.providerSpec.poolID              | POOL_ID               |
      | GCP Provider ID       | spec.providerSpec.providerID          | PROVIDER_ID           |
      | Service Account Email | spec.providerSpec.serviceAccountEmail | SERVICE_ACCOUNT_EMAIL |
   
    - The value for `Audience` has to be created based on `PROJECT_NUMBER`, `POOL_ID`, `PROVIDER_ID` environment variables, and has to validate against regex check defined by CCO: [openshift/cloud-credential-operator#708](https://github.com/openshift/cloud-credential-operator/pull/708).


3. **RBAC Permissions**:
    - Ensure the operator has the necessary RBAC permissions to create `CredentialsRequest` (Document: [Google Doc](https://docs.google.com/document/d/1iFNpyycby_rOY1wUew-yl3uPWlE00krTgr9XHDZOTNo/edit#heading=h.vzrrddcdj5gj)).
    - This is a CSV update only.


4. **Volume and Mount Definition**:
    - Define the required volume and mount for bound service account token in the CSV (Document: [Google Doc](https://docs.google.com/document/d/1iFNpyycby_rOY1wUew-yl3uPWlE00krTgr9XHDZOTNo/edit#heading=h.vzrrddcdj5gj)).
    - This is a CSV update only.


5. **WIF Support Announcement**:
    - Update the CSV to include the annotation: `metadata.annotations.features.operators.openshift.io/token-auth-gcp: "true"` (Document: [Google Doc](https://docs.google.com/document/d/1iFNpyycby_rOY1wUew-yl3uPWlE00krTgr9XHDZOTNo/edit#heading=h.vzrrddcdj5gj)).
    - This is a CSV update only.


6. **Handling Secrets**:
    - Ensure the GCP Filestore operator waits for the secret to be created by CCO and provide meaningful error messages if the secret is not available.


### Workflow Description

1. User locates GCP Filestore in the OperatorHub and clicks the operator tile.
2. User clicks "Install" and is presented with installation options.
3. The form presented to user includes fields for the new `CredentialsRequest` values - `ServiceAccountEmail`, `PoolID` and `ProviderID`.
4. User fills in the required fields and clicks "Install".
5. OLM places the values into the `Subscription` spec configuration which are injected into GCP Filestore Operator pods as environment variables - `PROJECT_NUMBER`, `POOL_ID`, `PROVIDER_ID`, `SERVICE_ACCOUNT_EMAIL`
6. The operator creates `CredentialsRequest` resource using the provided values.
7. CCO processes the `CredentialsRequest` and creates a secret with the necessary credentials.
8. GCP Filestore driver uses the secret to authenticate with GCP services with short-lived credentials.

### API Extensions

- No modifications to existing storage resources.

### Topology Considerations

#### Hypershift / Hosted Control Planes

- Hosted control planes are currently not supported on GCP.

#### Standalone Clusters

- The change is relevant for standalone clusters.

#### Single-node Deployments or MicroShift

- Resource consumption and performance impact should be minimal.

### Implementation Details/Notes/Constraints

- None.

### Risks and Mitigations

- Dependence on console changes for inputting necessary field values.
- Ensure thorough testing to avoid issues related to secret handling.
- Security and UX reviews by relevant teams.

### Drawbacks

- Additional complexity in managing WIF credentials.
- At the time of writing we don't have any official documentation on how to configure infrastructure WIF for GCP Filestore neither any automation available.

## Open Questions [optional]

- Are there any additional security concerns with using WIF for GCP Filestore?
- How will the operator handle scenarios where the necessary field values are not provided?
- Can we estimate and approve the impact on resource consumption with GCP Filestore defaulting to min. size of 1TiB per volume?

## Test Plan

- Develop end-to-end tests to verify that the GCP Filestore operator correctly creates and uses WIF credentials.
- Make sure we have a CI job to validate the end-to-end functionality of WIF with the GCP Filestore operator.

## Graduation Criteria

### Dev Preview -> Tech Preview

- No Dev Preview phase planned.
- No Tech Preview phase planned.

### Tech Preview -> GA

- Functionality is tested and verified by Quality Engineering.
- Automated tests are in place and passing steadily.
- Documentation is updated and available.

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

- Upgrade supported as usual via OLM.
- Downgrades not supported via OLM.

## Version Skew Strategy

- Not applicable - OLM based operator.

## Operational Aspects of API Extensions

- Not applicable - no storage API extensions.

## Support Procedures

- No additional support procedures required.

## Alternatives

- Similar to other operators like Azure or AWS we decide on using WIF configuration based on environment variables
availability in the operator pod only and reflect the values in CredentialsRequest object; CCO then creates a secret
with the necessary credentials (WIF configuration). Alternatively we could detect if cluster is running in manual
credentials mode similar to what CCO does and provide a warning (or error) to the user that WIF would not be configured
if some of the required environment variables are not set. Checking if the cluster is in manual credentials mode
with WIF enabled can be done by inspecting cloud credential and authentication resources:

If `"Manual"` is set in `cloudcredential/cluster` resource we proceed to check authentication.
```
$ oc -n openshift-cluster-csi-drivers get cloudcredential/cluster -o json | jq '.spec.credentialsMode'
"Manual"
```

If `serviceAccountIssuer` is not `""` we can assume that cluster is in manual credentials mode.
```
$ oc get authentication/cluster -o json | jq '.spec.serviceAccountIssuer'
"https://storage.googleapis.com/mycluster-01-oidc"
```

## Infrastructure Needed [optional]

- GCP Cloud access with sufficient permissions to create service accounts and roles for WIF cluster installation.
