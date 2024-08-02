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
  - "2024-08-02"
last-updated: 
  - "2024-08-02"
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/OCPSTRAT-979"
see-also:
  - "ARO HCP Authentication Strategy at HCP and Data Plane - https://docs.google.com/document/d/1Z7N2LAnRlgSgrFjjl2absOnkGFsI2TMcbwaW_CA1qek/edit#heading=h.bupciudrwmna"
replaces:
superseded-by:
---

# Add HyperShift Override Environment Variable for Azure Managed Service Identity

## Summary

This enhancement proposes introducing an environment variable in the image registry, ingress, cloud network config, 
and storage operators. This variable would allow overriding the Azure authentication strategy used by these operators to 
leverage Azure managed service identity (MSI), regardless of the underlying cloud configuration.

## Motivation

In Azure Red Hat OpenShift (ARO) Hosted Control Plane (HCP), operators running in the control plane need to 
authenticate using Azure managed service identities to communicate with cloud services. In contrast, the same operators 
running on the data plane/guest cluster side use workload identity authentication. 

### User Stories

* [Support MSI authentication in cluster-ingress-operator](https://issues.redhat.com/browse/NE-1504)
* [Support MSI authentication in cloud-network-config-controller](https://issues.redhat.com/browse/SDN-4450)
* [Support MSI authentication in cluster-storage-operator](https://issues.redhat.com/browse/STOR-1748)
* [Support MSI authentication in image-registry](https://issues.redhat.com/browse/IR-460)

### Goals

* Agreement from ingress, image registry, network, and storage representatives on a standard approach to authenticate with MSI for ARO HCP

### Non-Goals

* Implementing MSI for image registry, ingress, cloud network config, and storage operators outside the override.

## Proposal

We propose setting an environment variable, AZURE_MSI_AUTHENTICATION, upon deployment of image registry, ingress, cloud 
network config, and storage operators in the control plane of an ARO HCP cluster. This variable will be checked by each 
operator; if set, it will override the default authentication mechanism, using a managed identity to authenticate with 
Azure cloud services instead.

For operators with operands that they manage in the control plane, the operator would be responsible for propagating the 
environment variable to those operands (if the operands need cloud access).

### Workflow Description

* HostedCluster control plane operator will set AZURE_MSI_AUTHENTICATION on deployment of image registry, ingress, cloud network config, and storage operators
* When each operator is configuring the Azure authentication type, if the AZURE_MSI_AUTHENTICATION is set, the operator will ignore any other Azure cloud configuration and use MSI

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
