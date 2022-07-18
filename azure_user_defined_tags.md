---
title: apply-user-defined-tags-to-all-azure-resources-created-by-openShift
authors:
  - "@bhb"
reviewers:
  - 
approvers:
  - 
api-approvers:
  - 
creation-date: 2022-07-12
last-updated: 2022-07-12
tracking-link:
  - https://issues.redhat.com/browse/RFE-2017
  - https://issues.redhat.com/browse/CFE-492
  - https://issues.redhat.com/browse/CFEPLAN-59
see-also:
  - 
replaces:
  - 
superseded-by:
  - 
---

# Apply user defined tags to all Azure resources created by OpenShift

## Summary

This enhancement describes the proposal to allow an administrator of Openshift to 
have the ability to apply user defined tags to those resources created by Openshift 
in Azure.

## Motivation

Motivations include but are not limited to:

- Allow admin, compliance, and security teams to keep track of assets and objects 
  created by OpenShift, both at install time and during continuous operation (Day 2)

### User Stories

- As an openshift administrator, I want to have tags added to all resources created 
  in Azure by Openshift on Day 1.
- As an openshift administrator, I want to restrict access granted to Openshift specific account.
- As an openshift administrator, I should be able to create new tags or modify existing tags.

### Goals

- The administrator or service (in the case of Managed OpenShift) installing OpenShift 
  can pass an arbitrary list of user-defined tags to the OpenShift installer, and 
  the installer and all the bootstrapped components will apply these tags to all 
  resources created in Azure, for the life of the cluster, and where supported by Azure.
- Tags must be applied at creation time, in an atomic operation. It isn't acceptable 
  to create an object and, after some period of time, apply the tags post-creation.

### Non-Goals

- To reduce initial scope, support for deleting the tags is not supported.

## Proposal

A tag of the form `kubernetes.io/cluster/<cluster_name>:owned` will be added to every
resource created by Openshift to enable administrator to differentiate the resources
created for Openshift cluster. An administrator is not allowed to add or modify the tag 
having the prefix `kubernetes.io` in the name. 

New `userTags` field will be added to `platform.azure` of install-config for the user 
to define the tags to be added to the resources created by installer and in-cluster operators.

If `platform.azure.userTags` of install-config has any tag defined same will be added 
to all the azure resources created by Openshift except, when the tag validation fails 
due to any of below conditions <br>
1. A tag name can have a maximum of 128 characters <br>
   (Note: Tag name has a limit of 512 characters for all resources except for 
    storage accounts, which has a limit of 128 characters and hence tag name 
    length is restricted to 128 characters for every resource required by Openshift) <br>
2. A tag value has a limit of 256 characters. <br>
3. A tag name cannot contain `<, >, %, &, \, ?, /, #, :, whitespace` characters and 
   should not start with a number. <br>
   (Note: DNS zones, Traffic, Front Door resources does not support tag with spaces, 
    special/unicode characters or starting with number, hence these are added as 
    constraints to every other azure resource required by Openshift as well.) <br>
4. A resource, resource-group or subscription can have a maximum of 10 tags. <br>
   (Note: Azure supports a maximum of 50 tags except for Automation, 
    Content Delivery Network, DNS resources which can have a maximum of 15 tags, hence 
    restricting the number of tags to 10 for all resources created by Openshift, with 5 
    spared for administrator's use) <br>

Add a new field `resourceTags` to `.spec.platformSpec.azure` of the 
`infrastructure.config.openshift.io` type. Tags included in the `resourceTags` field 
will be applied to new resources created for the cluster.

Add a new field `resourceTags` to `.status.platformStatus.azure` of the 
`infrastructure.config.openshift.io` type. `resourceTags` will have the information 
on the resources for which tag operation failed, updated by the respective in-cluster operator.

All operators that create Azure resources (Cluster Infrastructure ,Storage ,Node ,NetworkEdge ,
Internal Registry ,CCO) will apply these tags to all Azure resources they create.

`resourceTags` that are specified in the infrastructure resource will merge with tags 
specified in an Azure resource. In the case where `resourceTags` specified in the 
infrastructure resource and there is a tag with the same name specified in an Azure 
resource, the value from the infrastructure resource will take precedence and be updated.

The userTags field is intended to be set at install time and is considered immutable. 
Components that respect this field must only ever add tags that they retrieve from this 
field to cloud resources, they must never remove tags from the existing underlying cloud 
resource even if the tags are removed from this field(despite it being immutable).

If the userTags field is changed post-install, there is no guarantee about how an 
in-cluster operator will respond to the change. Some operators may reconcile the 
change and change tags on the Azure resource. Some operators may ignore the change. 
However, if tags are removed from userTags, the tag will not be removed from the 
Azure resource.

### Workflow Description

- An Openshift administrator requests to add required tags to all Azure resources 
  created by Openshift by adding it in `.platform.azure.userTags`
- openshift installer validates the tags defined in `.platform.azure.userTags` and 
  adds these tags to all resources created during installation and also updates 
  `.spec.platformSpec.azure.resourceTags` of the `infrastructure.config.openshift.io`
- In cluster operators refers `.spec.platformSpec.azure.resourceTags` of the 
  `infrastructure.config.openshift.io` to add tags to resources created later.
- An Openshift administrator can modify existing tags or add new tags by updating 
  `.spec.platformSpec.azure.resourceTags` field in the `infrastructure.config.openshift.io`.

#### Variation [optional]

### API Extensions
Enhancement requires below modifications to the mentioned CRDs
- Add `userTags` field to `platform.azure` of the `installconfigs.install.openshift.io`
```yaml
  apiVersion: apiextensions.k8s.io/v1
  kind: CustomResourceDefinition
  metadata:
    name: installconfigs.install.openshift.io
  spec:
    versions:
    - name: v1
      schema:
        openAPIV3Schema:
          description: InstallConfig is the configuration for an OpenShift install.
          properties:
            platform:
              description: Platform is configuration for machine pool specific
                to the platform.
              properties:
                azure:
                  description: Azure is the configuration used when installing
                    on Azure.
                  properties:
                    userTags:
                      additionalProperties:
                        type: string
                      description: UserTags additional keys and values that the installer
                        will add as tags to all resources that it creates. Resources
                        created by the cluster itself may not include these tags.
                    type: object
```

- Add `resourceTags` field to `spec.platformSpec.azure` and `platformStatus.status.azure` 
  of the `infrastructure.config.openshift.io`
```yaml
  apiVersion: apiextensions.k8s.io/v1
  kind: CustomResourceDefinition
  metadata:
    name: infrastructures.config.openshift.io
  spec:
    versions:
    - name: v1
      schema:
        openAPIV3Schema:
          description: "Infrastructure holds cluster-wide information about Infrastructure.  The canonical name is `cluster` \n Compatibility level 1: Stable within a major release for a minimum of 12 months or 3 minor releases (whichever is longer)."
          properties:
            spec:
              description: spec holds user settable values for configuration
              properties:
                platformSpec:
                  description: platformSpec holds desired information specific to the underlying infrastructure provider.
                  properties:
                    azure:
                      description: Azure contains settings specific to the Azure infrastructure provider.
                      properties:
                        resourceTags:
                          description: resourceTags is a list of additional tags to apply to Azure resources created for the cluster. See https://docs.microsoft.com/en-us/rest/api/resources/tags for information on tagging Azure resources. Azure supports a maximum of 50 tags per resource except for few. OpenShift reserves 10 tags for its use, leaving 40 tags available for the user.
                          type: array
                          maxItems: 10
                          items:
                            description: AzureResourceTag is a tag to apply to Azure resources created for the cluster.
                            type: object
                            required:
                              - key
                              - value
                            properties:
                              key:
                                description: key is the key of the tag
                                type: string
                                maxLength: 128
                                minLength: 1
                                pattern: ^[a-zA-Z][0-9A-Za-z_.=+-@]+$
                              value:
                                description: value is the value of the tag.
                                type: string
                                maxLength: 256
                                minLength: 1
                                pattern: ^[0-9A-Za-z_.=+-@]+$
            status:
              description: status holds observed values from the cluster. They may not be overridden.
              properties:
                platformStatus:
                  description: platformStatus holds status information specific to the underlying infrastructure provider.
                  properties:
                    azure:
                      description: Azure contains settings specific to the Azure infrastructure provider.
                      properties:
                        resourceTags:
                          description: resourceTags is a list of Azure resources for which tag update got failed. It contains the Azure resource name and the error encountered. 
                          type: array
                          items:
                            description: Azure resource is the name of the resource created for the cluster.
                            type: object
                            required:
                              - name
                              - error
                            properties:
                              name:
                                description: name is the name of the Azure resource.
                                type: string
                              error:
                                description: error is the reason for the tag update failure.
                                type: string
```

### Implementation Details/Notes/Constraints [optional]

What are the caveats to the implementation? What are some important details that
didn't come across above. Go in to as much detail as necessary here. This might
be a good place to talk about core concepts and how they relate.

### Risks and Mitigations

### Drawbacks
- User-defined tags cannot be updated on an Azure resource which is not managed by an
  operator. In this proposal, the changes proposed and developed will be part of
  openshift-* namespace. External operators are not in scope.
  User-defined tags can be updated on the following Azure resources.
    1. Virtual machine resources created for master and worker nodes.
    2. Storage account
    3. NetworkEdge
    4. Internal Registry
    5. CCO

- OpenShift is bound to have the common limitation for all Azure resources created
  by it and constrains other resources with the least matching limit as below
    1. Tag names cannot have `microsoft`, `azure`, `windows` prefixes which are
       reserved for Azure use.
    2. An Azure storage account has a limit of 128 characters for the tag name.
    3. An Azure DNS zone or Traffic or Front Door resource tag name cannot have spaces,
       special/unicode characters or start with a number.
    4. An Azure Automation or Content Delivery Network or DNS resource can have a
       maximum of 15 tags.

- Administrator will have to manually perform tags pertaining actions for
    1. removing the undesired tags from the required resources.
    2. update tags of the resources which are not managed by an operator.
    3. update tags of the resources for which update logic is not supported by an operator.

## Design Details
The `resourceTags` field in `spec.platformSpec.azure` will be populated by the 
installer using the entries from `userTags` field.

`infrastructure.config.openshift.io` fields `spec.platformSpec.azure` is a mutable 
and `status.platformStatus.azure` is immutable.

All operators that create Azure resources (Cluster Infrastructure ,Storage ,Node ,
NetworkEdge ,Internal Registry ,CCO) will apply these tags to all Azure resources they create.

Operators should create an event to indicate the status of tag add/modify request and
as well update the `status.platformSpec.azure` field of `infrastructure.config.openshift.io`
with required details in case of failure. 

#### Create tags scenarios
In the cases where a tag is specified in the `infrastructure.config.openshift.io` resource and
1) A tag with the same key is present in an Azure resource, the value will be replaced with
   `infrastructure.config.openshift.io` resource value

   For example, <br>
   Azure resource as a tag `key_infra = value_azure` <br>
   New tag request = `spec.platformSpec.azure.resourceTags` has `key_infra = value_infra` <br>
   Action = Azure resource tag is updated to `key_infra = value_infra` <br>
   Event action = An event is generated to notify user about the request 
   status (success/failure) to update tags for the Azure resource.

2) A tag with same key is not present and is a new tag to be added for an Aure 
   resource, but maximum tag limit is reached or any other error encountered
   should be notified to user.

   For example, <br>
   New tag request = `.spec.platformSpec.azure.resourceTags` has `key_infra = value_infra` <br>
   Action = A new tag to be added for an Azure resource. <br>
   Final tag set to Azure resource = `key_infra = value_infra` <br>
   Event action = An event is generated to notify user about the request 
   status (success/failure) to create tags for the Azure resource.

#### Update tags scenarios
Users can update the user-defined tags by editing `.spec.platformSpec.azure` of the 
`infrastructure.config.openshift.io` resource. On update of `resourceTags`, 
Azure resource is not created or restarted.

In the case where a tag is updated in the `infrastructure.config.openshift.io` resource and
1) A tag with the same key and value is present in an Azure resource, no update is made.

   For example,<br>
   Existing tag for Azure resource = `key_infra = value_update` <br>
   New tag request = `.spec.platformSpec.azure.resourceTags` has `key_infra = value_update` <br>
   Action = There is no update for Azure resource. <br>
   Final tag set to Azure resource = `key_infra = value_update`

2) A tag with the same key but different value is present in an Azure resource, the Azure 
   resource will be updated with the new value.

   For example, <br>
   Existing tag for Azure resource = `key_infra = value` <br>
   New tag request = `.spec.platformSpec.azure.resourceTags` has `key_infra = value_new` <br>
   Action = Existing tag for Azure resource is updated to reflect new value. <br>
   Final tag set to Azure resource = `key_infra = value_new` <br>
   Event action = An event is generated to notify user about the request 
   status (success/failure) to update tags for the Azure resource.

#### Caveats
1) User updates Azure resource's tag using an external interface which is present in 
   `.spec.platformSpec.azure.resourceTags`, tag will be reconciled by the owning operator 
   with the value from the `.spec.platformSpec.azure.resourceTags`. Reconciliation does not 
   happen set immediately for Azure resource by the owning operator. There is eventual 
   consistency maintained by the owning operator. The time taken to reconcile the modified 
   tag on Azure resource to desired value vary across owning operators.

   For example, <br>
   Edited existing tag using external tool for Azure resource = `key_infra = value_tool` <br>
   Previous tag request = `.spec.platformSpec.azure.resourceTags` or resource has `key_infra = value` <br>
   Action = Update existing tag with value from `.spec.platformSpec.azure.resourceTags`. <br>
   Final tag set to Azure resource = `key_infra1 = value` <br>
   Event action = An event is generated to notify user about the request 
   status (success/failure) to update tags for the Azure resource.

2) User removes the user-defined tag from `.spec.platformSpec.azure.resourceTags` or Azure resource.
   The user-defined tag which is removed from spec will not be reconciled or managed by operators.
   User can update tag using an external interface. The user-defined tag will not be modified 
   by the operator.

   For example, <br>
   Existing tag for Azure resource = `key_infra = value` <br>
   New tag request = `.spec.platformSpec.azure.resourceTags` has no user-defined tag with key `key_infra` <br>
   Action = No change in tags for the Azure resource. <br>
   Final tag set to Azure resource = `key_infra1 = value` <br>

3) Updating tags of individual resources is not supported and any tag present in 
   `.spec.platformSpec.azure.resourceTags` of `infrastructure.config.openshift.io/v1` resource 
   will result in updating tags of all Openshift managed Azure resources. 

### Open Questions

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

On upgrade:
- Cluster operators that update the tags of Azure resources created for cluster 
  should refer the new fields and take action. 

On downgrade:
- The status/spec field may remain populated, components may or may not continue 
  to tag newly created resources with the additional tags depending on whether or 
  not a given component still has logic to respect the status tags, after the downgrade.

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Alternatives
Alternate or extended proposal is to have a dedicated controller for managing 
the cloud infrastructure as desired by the user by supporting but not limited
to adding or modifying tags of the cloud resources created by Openshift.

### Motivation
Motivations for having a dedicated cloud-infra-controller include but are not limited to 
- A single controller which adds/modifies tags of the all resources created by 
  Openshift and updates the status of the operation in `infrastructure.config.openshift.io`
  instead of multiple controllers doing so.
- Having single controller will solve the issue of tags not getting added to those resources
  created by installer and not being managed by any operator during tag update or cluster 
  upgrade scenario(from feature unsupported version).

### Design Details
- The dedicated controller will watch for changes in the `infrastructure.config.openshift.io`
  resource and acts only when there is a change to `spec.platformSpec.azure` field.
- Controller queries for all the resources which has the tag 
  `kubernetes.io/cluster/<cluster_name>:owned` and updates each resource with the new 
  requested changes.
- In case cluster is upgraded from a release without Azure tag implementation (query for 
  resources with `kubernetes.io/cluster/<cluster_name>:owned` tag yields just resource group
  type), controller queries for the resources having cluster name in the name tag and 
  updates each matching resource with the new requested changes.
- In-cluster operators should not watch for changes to `spec.platformSpec.azure` field of 
  `infrastructure.config.openshift.io` and should continue with current functionality of
  creating requested resources, but include additional functionality to add user defined tags
  and the `kubernetes.io/cluster/<cluster_name>:owned` default tag to created resource.
  
### Caveats
- Few Azure resources such as disk, storage account, DNS zones and records name might not
  match either `kubernetes.io/cluster/<cluster_name>:owned` or cluster name in the name tag
  and will result in tagging inconsistency, which will be minimal compared not to having 
  aformentioned dedicated controller for managing tags of Azure resources.
## Infrastructure Needed [optional]
