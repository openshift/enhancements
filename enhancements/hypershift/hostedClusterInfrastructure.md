---
title: hostedClusterInfrastructure

authors:
  - "@jnpacker"
  
reviewers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
  - "@alvaroaleman"
  - "@derekwaynecarr"
  - "@imain"
  - "@yuqi-zhang"
  
approvers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
  - "@alvaroaleman"
  - "@derekwaynecarr"
  - "@imain"

api-approvers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
  - "@alvaroaleman"
  - "@derekwaynecarr"
  - "@imain"

tracking-link:
  - https://issues.redhat.com/browse/ACM-1554

creation-date: 2022-08-08
last-updated: 2022-08-08
---

# Hosted Cluster Infrastructure

## Summary
This enhnacement was born out of the Untangling HypershiftDeployment, HostedCluster and NodePool document (see glossary). With the deprecation (removal) of HypershiftDeployment for GA, ACM/MCE requires a means to track and manage Infra for Cloud Providers for its primary user Cluster Administrator (multi-service cluster administrator).
HostedClusterInfrastructure would be used to provision or track Cloud Provider infrastructure objects that can be used when provisioning HostedClusters using those providers.

## Glossary
- Untangling HypershiftDeployment, HostedCluster and NodePool Google document - [Google Document - Option D](https://docs.google.com/document/d/1VabhUQa_uZlWO6mf9UHS5lGtnH_X915cih4ARJbNibc/edit#heading=h.iuee7tji8zz4)

## Motivation
ACM and MCE primary Cloud Provider use cases already provision these resources and manage these resources, this provides feature parity with Standalone flows that ACM/MCE Cluster Administrator's (including infrastructure administrator's) are used to.  This also allows to additional opporunities:
  1. Infrastructure can be pre-created using an ACM/MCE Cloud Provider credential or by pre-populating the Custom Resource with values and applying it to the ACM/MCE hub. These can then be consumed by the ACM/MCE UI or CLI, and is a nice option, as the AWS credential does not need to be exposed to the user creating the HostedCluster.
  2. You can re-use the HostedClusterInfrastructure resource between HostedClusters. This is especially useful in Azure, where it takes longer to create infrastructure then the hosted control plane.

### User Stories
- As an Cluster Service Provider (infrastructure administrator) I want to create one or more HostedClusterInfrastructure resources for my team to consume.
  
  This approach pre-creates the HostedClusterInfrastructure before the consumer creates the HostedCluster and NodePool resources. This approach has a few advantage for ACM/MCE use cases:
  * The first advantage is that an ACM/MCE Cluster Administrator (infrastructure administrator) can create the infrastructure resources using either their account or a service account stored in the ACM/MCE Cloud Provider credential (AWS or Azure cred).  
  The result HostedClusterInfrastructure object can be given get/list verbs to the consumer who wants to create a cluster. 
  This consumer would then only need CRUD for HostedCluster and NodePool and be able to use the console to pick a HostedClusterInfrastructure resource and then create a HostedCluster.  No AWS credential needs to be passed to the consumer.  
  * The second is that for slow cloud providers like Azure, the infrastructure can be pre-created to improve cluster deployment time (in dev, test, and pipeline scenarios). 
  It also seems possible to re-use the infrastructure between clusters, so treating HostedClusterInfrastructure as an entity outside the hostedCluster promotes scenarios of re-use (saves time, especially in Azure. Most likely a dev or pipeline user case).
  * The third is that the HostedClusterInfrastructure YAML can be committed to Git and applied using ArgoCD, Flux or ACM Subscriptions to create a gitOps flow for infrastructure creation and deletion.
  * The fourth is to apply HostedClusterInfrastructure when the CLI or manual infrastructure has already been
   partially or fully completed. The HCI controller will find all the resources(using the Infra-ID tag) and update the values in the HCI customer resource status. This allows the HCI custom resource to be used after a CLI 
   `create infra aws` command, as well as to find and complete any manual configuration directly in AWS. Example: I create the VPC and appropriately tag it with the Infra-ID, once I create the HCI custom resource, the HCI controller will create and connect all the remaining AWS resources.
  
- As an ACM/MCE Cluster Administrator (infrastructure administrator), I want to choose an ACM/MCE Cloud Provider credential and deploy a Cloud Provider HostedCluster from start to finish.

  * This works nicely with the CLI, as it serializes the steps. The CLI creates the resources and renders (and could apply) the HostedClusterInfrastructure, then creates and deploys the HostedCluster and NodePool. 
   In the strict CLI flow, the additional capability is minimal, but when you combine the console in ACM/MCE, the destruction of a HostedCluster can now be completed from the Console, where currently, you could destroy the HostedCluster, 
   but the infrastructure objects would remain until you ran a CLI command or performed all the manual deletion steps in AWS/Azure.
  * For the Console, GitOps flow, or from YAML,  when HostedClusterInfrastructure, HostedCluster & NodePool are created simultaneously we would need to enhance HostedCluster and NodePool controllers.  We would want to add an ObjectRef or annotation to identify the HostedClusterInfrastructure. 
  Then these controllers would pull their platform infrastructure values from HostedClusterInfrastructure. (We would need to decide if we want to populate the values into HC and NP or just read them from HostedClusterInfrastructure.  
  It might be possible to ObjectRef from the HostedClusterInfrastructure to the HostedCluster (and nodePool), but we would still need the HC and NP controllers to either pull the values or wait for the HostedClusterInfrasrtucture controller to transfer the values.
  
- As a Cluster Consumer, I want to choose an available HostedClusterInfrastructure resource and have a HostedCluster deploy, without knowing or having to provide an AWS credential

  * This use case was described under the pre-create approaches above.

### Goals
- Provide a consumable API for HostedClusterInfrastructure that satisfies existing ACM/MCE use cases and expected user stories(flows)
- Consistency between CLI, Console and GitOps (YAML flows). Including HostedClusterInfrastructure allows Console and Git flows to interact with Infrastructure, similar to what the CLI does. It also allows the infrastructure to be provisioned via CLI, but removed from the Console.

### Non-Goals
- The consumer usecase also compliments the cluster create templating flow

## Proposal
### API
[HostedClusterInfrastructure](https://github.com/jnpacker/hypershift/blob/acm-1554/api/v1alpha1/hostedclusterinfrastructure_types.go) is the consumer facing API exposed for Cloud Infrastructure creation and destroy.
Its reason to exist is to preserve Red Hat Advanced Cluster Management user stories and allows our ability to satisfy consumer needs and evolve at our own pace based on user feedback.

#### Implementation
The HostedClusterInfrastructure controller is meant to be limited interface, that leverages the same CLI flows (so they are always compatable) to create, destroy and evaluate (spike) Cloud Provider infrastructure resources when the Cloud Provider credential is available.

When a new HostedClusterInfrastructure is created:
- The HostedClusterInfrastructure controller will reconcile
  - First it will determine if a Cloud Provider credential is present
    - If Present:
      - It will create the infrastructure where needed and validate the infrastructure where it exists
    - If Not Present
      - The resource just contains user provided values that can be consumed by the UI or CLI
- CLI will be able to create or render the HostedClusterInfrastructure resource
- CLI will be able to consume the HostedClusterInfrastructure resource
- User may create/apply the resource with all values
- Delete HostedClusterInfrastructure the controller will reconcile
  - First it will determine if a Cloud Provider credential is present
    - If Present:
      - It will delete all the infrastructure resources found in the cr before allowing the cr to be removed
    - If Not Present
      - The cr will just be deleted

### CLI
Supported CLI capabilities:
- Create a HostedClusterInfrastructure using an AWS/Azure credential or secret
- Render a HostedClusterInfrastructure using an AWS/Azure credential or secret (for Git or manual create/apply)
- Consume an existing HostedClusterInfrastructure when creating a HostedCluster and NodePool resource
- If connected to the Hosting Cluster(Management Cluster) When creating infrastructure apply the HCI custom resource as part of the creation process

#### Console
- Allow creation of infrastructure via the HCI cr
- Allow deletion of infrastructure via the HCI cr
- Allow Admin to pre-populate infrastructure for use by a consumer who just creates the HostedCluster and NodePool cr's
- (spike) Report status of resources in HCI
### Workflow Description
The [HostedClusterInfrastructure API](https://github.com/jnpacker/hypershift/blob/acm-1554/api/v1alpha1/hostedclusterinfrastructure_types.go) 
is the entrypoint and consumer facing API for any human or automation to interact with cloud provider infrastructure.  This works in tandem with the hypershift CLI, but is neither is required to create HostedClusters if you have the appropriate values.

### API Extensions
N/A.
### Risks and Mitigations
N/A.
### Drawbacks
N/A.
### Test Plan
N/A.
#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
The following features are aimed to be supported and go through e2e automated testing before GA:
- Create Infra
- Destroy Infra
- (Spike) Verify infra

#### Removing a deprecated feature
- This will align with ACM 2.7/MCE 2.2, where the HypershiftDeployment CRD will be removed
### Upgrade / Downgrade Strategy
N/A.
### Version Skew Strategy
N/A.
### Operational Aspects of API Extensions
N/A.
#### Failure Modes
#### Support Procedures

## Alternatives
* HypershiftDeployment custom resource moved to openshift/hypershift which encompasses Infrastructure, HostedCluster and NodePool (currently exists)
* Leave things as is, which does not give a consistent console experience for create and destroy, it also leaves no easy path to perform GitOps to cover the infrastructure components.


## Design Details
* Some initial work on the CRD and the `hypershift` CLI to create a HostedClusterInfrasrtucture CR when provisioning infra resources in AWS.
[Git Commit](https://github.com/jnpacker/hypershift/commit/300958f60f1cd30cabb1c25a4bf8acdb13708836)

### Graduation Criteria

## Implementation History
The initial version of this doc represents implementation as delivered via MCE tech preview(ACM contains MCE).
