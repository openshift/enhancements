---
title: msi-enablement-for-aro-hcp
authors:
  - "@bryan-cox"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@enxebre" #HCP
  - "@csrwng" #HCP
  - "@kyrtapz" #CNCC
  - "@flavianmissi" #Image Registry
  - "@jsafrane" #Storage
  - "@Miciah" #Ingress
  - "@bennerv" #ARO HCP
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@enxebre"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "None"
creation-date: 
  - "2024-10-07"
last-updated:
  - "2024-10-11"
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/HOSTEDCP-1994"
see-also:
  - "ARO HCP Authentication Strategy at HCP and Data Plane - https://docs.google.com/document/d/1Z7N2LAnRlgSgrFjjl2absOnkGFsI2TMcbwaW_CA1qek/edit#heading=h.bupciudrwmna"
  - "MSI Connector Design - https://docs.google.com/document/d/1xFJSXi71bl-fpAJBr2MM1iFdUqeQnlcneAjlH8ogQxQ/edit#heading=h.8e4x3inip35u"
replaces:
  - "https://github.com/openshift/enhancements/pull/1659"
superseded-by:
---

# Enable Authenticating with Azure with Certificates using Azure SDK for Go's Generic NewDefaultAzureCredential

## Summary

This enhancement proposes enabling image registry, ingress, cloud network config, and storage operators(azure-file and 
azure-disk) to accept authenticating with Azure with certificates using Azure SDK for Go's generic function 
[NewDefaultAzureCredential](https://github.com/Azure/azure-sdk-for-go/blob/4ebe2fa68c8f9f0a0737d4569810525b4ac45834/sdk/azidentity/default_azure_credential.go#L63).

## Motivation

In production, Azure Red Hat OpenShift (ARO) Hosted Control Plane (HCP), operators running in the control plane need to
authenticate using Azure managed identities, backed by certificates, to communicate with cloud services. In the 
meantime, ARO HCP will use Service Principal, backed by certificates, for development and testing. 

In contrast, the same operators running on the data plane/guest cluster side use workload identity authentication.

### User Stories

* [Explore enable getting AzureCreds via cert using generic NewDefaultAzureCredential](https://issues.redhat.com/browse/HOSTEDCP-1994)

### Goals

* Agreement from ingress, image registry, network, and storage representatives on a standard approach to authenticate with Azure for ARO HCP.

### Non-Goals

N/A

## Proposal

We propose updating the Azure API authentication methods in image registry, ingress, cloud network config, and storage 
operators to use the using Azure SDK for Go's generic function [NewDefaultAzureCredential](https://github.com/Azure/azure-sdk-for-go/blob/4ebe2fa68c8f9f0a0737d4569810525b4ac45834/sdk/azidentity/default_azure_credential.go#L63).
This function walks through a chain of Azure authentication types, using environment variables, Instance Metadata Service (IMDS), or a file on the local filesystem to authenticate with the Azure API.  

HyperShift would pass the following environment variables - AZURE_CLIENT_ID, AZURE_TENANT_ID, and 
AZURE_CLIENT_CERTIFICATE_PATH - to its deployments of image registry and ingress on the hosted control plane. Each of 
these components would then pass these variables along to NewDefaultAzureCredential.

For storage operators (azure-file and azure-disk) on the hosted control plane, we will need pass along extra variables 
so we can mount the cert in a volume on the azure-file and azure-disk controller pod - 
ARO_HCP_SECRET_PROVIDER_CLASS_FOR_FILE and ARO_HCP_SECRET_PROVIDER_CLASS_FOR_DISK. 

For cloud-network-config-controller, HyperShift will pass AZURE_CLIENT_ID, AZURE_TENANT_ID, 
AZURE_CLIENT_CERTIFICATE_PATH, and ARO_HCP_SECRET_PROVIDER_CLASS to cluster-network-operator. cluster-network-operator 
will include AZURE_CLIENT_ID, AZURE_TENANT_ID, and AZURE_CLIENT_CERTIFICATE_PATH as environment variables to the 
deployment of cloud-network-config-controller and mount the ARO_HCP_SECRET_PROVIDER_CLASS as a volume attribute in a 
volume in the same deployment.

For each component, we should also set this environment variable to true, AZURE_CLIENT_SEND_CERTIFICATE_CHAIN. This 
enables a Microsoft internal feature called SNI (Subject Name and Issuer authentication). It essentially allows one to 
authenticate without pinning a certificate to a service principal if the certificate passed is issued & trusted by a 
specific CA. 

Unfortunately, the SDK today does not support hot-reloading the credential. While Microsoft works on getting that into 
the SDK, we could either use a library to create a generator for the credential and allow reloading in-process, or use 
the common OpenShift fsnotify +os.Exit() to restart the pod on config change.

### Workflow Description

* HostedCluster control plane operator will set AZURE_CLIENT_ID, AZURE_TENANT_ID, and AZURE_CLIENT_CERTIFICATE_PATH on deployment of image registry, ingress, cluster network operator (which will pass the value to cloud network config), and storage operators (which will pass the values to azure-file and azure disk)
* When each operator is configuring the Azure authentication type, it will call Azure SDK for Go's generic function NewDefaultAzureCredential

### API Extensions

N/A

### Topology Considerations

#### Hypershift / Hosted Control Planes

Specified above

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

### Implementation Details/Notes/Constraints

TBD

### Risks and Mitigations

TBD

### Drawbacks

TBD

## Open Questions [optional]

TBD

## Test Plan

TBD

## Graduation Criteria

TBD

### Dev Preview -> Tech Preview

TBD

### Tech Preview -> GA

TBD

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

N/A

## Alternatives

N/A

## Infrastructure Needed [optional]

N/A