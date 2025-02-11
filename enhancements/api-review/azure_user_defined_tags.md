---
title: azure_user_defined_tags
authors:
  - "@bhb"
reviewers:
  - "@patrickdillon" ## reviewer for installer component
  - "@JoelSpeed" ## reviewer for api and machine-api-provider-azure components
  - "@flavianmissi" ## reviewer for cluster-image-registry-operator component
  - "@Miciah" ## reviewer for cluster-ingress-operator component
approvers:
  - "@jerpeter1" ## approver for CFE
api-approvers:
  - "@JoelSpeed" ## approver for api component
creation-date: 2022-07-12
last-updated: 2023-06-06
tracking-link:
  - https://issues.redhat.com/browse/OCPPLAN-8155
  - https://issues.redhat.com/browse/CORS-2249
see-also:
  - "enhancements/api-review/custom-tags-aws.md"
replaces:
  - N/A
superseded-by:
  - N/A
---

# Apply user defined tags to all Azure resources created by OpenShift

## Summary

This enhancement describes the proposal to allow an administrator of Openshift to 
have the ability to apply user defined tags to those resources created by Openshift 
in Azure.

## Motivation

Motivations include but are not limited to:

- Allow admin, compliance, and security teams to keep track of assets and objects 
  created by OpenShift in Azure.

### User Stories

- As an openshift administrator, I want to have tags added to all resources created 
  in Azure by Openshift, so that I can restrict access granted to an OpenShift specific account.

### Goals

- The administrator or service (in the case of Managed OpenShift) installing OpenShift 
  can configure a list of up to 10 user-defined tags in the OpenShift installer generated
  install config, which is referred and applied by the installer and the in-cluster operators
  on the the Azure resources during cluster creation.
- Tags must be applied at creation time, in an atomic operation. It isn't acceptable 
  to create an object and to apply tags post resource creation.

### Non-Goals

- Management(update/delete) of resource tags post creation of cluster is out of scope.

## Proposal

A tag of the form `kubernetes.io_cluster.<cluster_id>:owned` will be added to every
resource created by Openshift to enable administrator to differentiate the resources
created for Openshift cluster. An administrator is not allowed to add or modify the tag 
having the prefix `kubernetes.io` or `openshift.io` in the name. The same tag can be 
found applied to other cloud platform resources which supports tagging for ex: AWS.

New `userTags` field will be added to `platform.azure` of install-config for the user 
to define the tags to be added to the resources created by installer and in-cluster operators.

If `platform.azure.userTags` in the install-config has any tags defined, then these tags 
will be added to all the azure resources created by OpenShift, provided all the defined 
tags meet all the below conditions or else the cluster creation will fail.
1. A tag name can have a maximum of 128 characters.
    - Tag name has a limit of 512 characters for all resources except for 
    storage accounts, which has a limit of 128 characters and hence tag name 
    length is restricted to 128 characters on every resource required by Openshift.
2. A tag name must begin with a letter, end with a letter, number or underscore, and
   can contain only letters, numbers, underscores, periods, or hyphens.
    - DNS zones, Traffic, Front Door resources does not support tag with spaces, 
    special/unicode characters or starting with number, hence these are added as 
    constraints on every other Azure resource required by Openshift as well.
3. A tag value can have a maximum of 256 characters.
4. A resource, resource-group or subscription, user can configure a maximum of 10 tags
   through Openshift. 
    - Azure supports a maximum of 50 tags except for Automation, Content Delivery Network,
    DNS resources which can have a maximum of 15 tags, hence restricting the number of 
    user defined tags to 10 and 5 for Openshift's internal use, for all the resources 
    created by Openshift.

All in-cluster operators that create Azure resources (Cluster Infrastructure ,Storage ,Node ,NetworkEdge , Internal Registry) will apply these tags during resource creation.

The userTags field is intended to be set at install time and is considered immutable. 
Components that respect this field must only ever add tags that they retrieve from this 
field to cloud resources, they must never remove tags from the existing underlying cloud 
resource even if the tags are removed from this field(despite it being immutable).

If the userTags field is changed post-install, there is no guarantee about how an 
in-cluster operator will respond to the change. Operators may reconcile the 
change by adding or updating tags on the Azure resource. Operators may ignore the change. 
However, if a tag is removed from `userTags`, the tag must not be removed from the 
Azure resource.

### Workflow Description

- An Openshift administrator requests to add required tags to all Azure resources 
  created by Openshift by adding it in `.platform.azure.userTags`
- openshift installer validates the tags defined in `.platform.azure.userTags` and 
  adds these tags to all resources created during installation and also updates 
  `.status.platformStatus.azure.resourceTags` of the `infrastructure.config.openshift.io`
- Cluster operators refer to `.status.platformStatus.azure.resourceTags` of the 
  `infrastructure.config.openshift.io` object to add tags to resources created later.

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
        properties:
          platform:
            properties:
              azure:
                properties:
                  userTags:
                    additionalProperties:
                      type: string
                    description: UserTags has additional keys and values that the installer
                      will add as tags to all resources that it creates. Resources
                      created by the cluster itself may not include these tags.
                  type: object
```

- Add `resourceTags` field to `status.platformStatus.azure` 
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
        properties:
          status:
            properties:
              platformStatus:
                properties:
                  azure:
                    properties:
                      resourceTags:
                        description: resourceTags is a list of additional tags to apply to Azure
                        resources created for the cluster. See
                        https://docs.microsoft.com/en-us/rest/api/resources/tags for information
                        on tagging Azure resources. Due to limitations on Automation,
                        Content Delivery Network, DNS Azure resources, a maximum of 15 tags may be
                        applied. OpenShift reserves 5 tags for internal use, allowing 10 tags for
                        user configuration.
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
                              description: key is the key part of the tag. A tag key can have a maximum of
                              128 characters and cannot be empty. Key must begin with a letter, end with a
                              letter, number or underscore, and must contain only alphanumeric characters
                              and the following special characters `_ . -`.
                              maxLength: 128
                              minLength: 1
                              pattern: ^[a-zA-Z]([0-9A-Za-z_.-]*[0-9A-Za-z_])?$
                            value:
                              description: value is the value part of the tag. A tag value can have a maximum
                              of 256 characters and cannot be empty. Value must contain only alphanumeric
                              characters and the following special characters `_ + , - . / : ; < = > ? @`.
                              type: string
                              maxLength: 256
                              minLength: 1
                              pattern: ^[0-9A-Za-z_.=+-@]+$
```

### Implementation Details/Notes/Constraints [optional]
Add a new field `resourceTags` to `.status.platformStatus.azure` of the 
`infrastructure.config.openshift.io` type. Tags included in the `resourceTags` field 
will be applied to new resources created for the cluster by the in-cluster operators.

The `resourceTags` field in `status.platformStatus.azure` of `infrastructure.config.openshift.io`
will be populated by the installer using the entries from `platform.azure.userTags` field of `install-config`.

`status.platformStatus.azure` of `infrastructure.config.openshift.io` is immutable.

All operators that create Azure resources will apply these tags to all Azure 
resources they create. List of in-cluster operators managing cloud resources
could vary across platform types, example for AWS there are additional operators
like aws-ebs-csi-driver-operator, aws-efs-csi-driver-operator to manage specific
resources.

| Operator | Resources created by the operator |
| -------- | ----------------------------- |
| cluster-image-registry-operator | Storage Account |
| cluster-ingress-operator | DNS records |
| machine-api-provider-azure | Application Security Group, Availability Set, Group, Load Balancer, Public IP Address, Route, Network Security Group, Virtual Machine Extension, Virtual Interface, Virtual Machine, Virtual Network. |

Below list of terraform Azure APIs to create resources should be updated to add user
defined tags and as well the openshift default tag in the installer component.
`azurerm_resource_group, azurerm_image, azurerm_lb, azurerm_network_security_group, azurerm_storage_account, azurerm_user_assigned_identity, azurerm_virtual_network, azurerm_linux_virtual_machine, azurerm_network_interface, azurerm_dns_cname_record`

API update example:
A local variable should be defined, which merges the default tag and the user
defined Azure tags, which should be referenced in the Azure resource APIs.
``` terraform
locals {
  tags = merge(
    {
      "kubernetes.io_cluster.${var.cluster_id}" = "owned"
    },
    var.azure_extra_tags,
  )
}

resource "azurerm_resource_group" "main" {
  tags     = local.tags
}
```

The tag of the form `kubernetes.io_cluster.<cluster_id>:owned` added by openshift, where
cluster_id is a string generated by concatenating user inputted cluster name and a random 
string will be limited to a maximum length of 27 characters by trimming long cluster name 
to 21 characters.

#### Caveats
1. Updating or removing resource tags added by Openshift, using an external interface
   may or may not be reconciled by the operator managing the resource.
2. Updating tags of individual resources is not supported and any tag present in 
   `.status.platformStatus.azure.resourceTags` of `infrastructure.config.openshift.io/v1` resource 
   will result in adding tags to all Openshift managed Azure resources. 

### Risks and Mitigations

### Drawbacks
- User-defined tags cannot be updated on an Azure resource which is not managed by an
  operator. In this proposal, the changes proposed and developed will be part of
  openshift-* namespace. External operators are not in the scope.
  User-defined tags can be updated on the following Azure resources.
  - ResourceGroup
  - Storage Account
  - DNS Zones
  - DNS Records
  - ApplicationSecurityGroup
  - AvailabilitySet
  - Group
  - LoadBalancer
  - PublicIPAddress
  - RouteTable
  - SecurityGroup
  - VirtualMachineExtension
  - NetworkInterface
  - VirtualMachine
  - VirtualNetwork

- Because tags in `userTags` must be applied to all Azure resources that OpenShift creates,
  tags must fit all the constraints of the various Azure resources as described below:
    1. Tag names cannot have `microsoft`, `azure`, `windows` prefixes which are
       reserved for Azure use.
    2. An Azure storage account has a limit of 128 characters for the tag name.
    3. An Azure DNS zone or Traffic or Front Door resource tag name cannot have spaces,
       special/unicode characters or start with a number.
    4. An Azure Automation or Content Delivery Network or DNS resource can have a
       maximum of 15 tags.

- Administrator will have to manually perform below tags pertaining actions
    1. removing the undesired tags from the required resources.
    2. update tag values of the required resources.
    3. update tags of the resources which are not managed by an operator.
    4. update tags of the resources for which update logic is not supported by an operator.

## Design Details

### Open Questions

### Test Plan
- Upgrade/downgrade testing
- Sufficient time for feedback
- Available by default
- Stress testing for scaling and tag update scenarios

### Graduation Criteria
#### Dev Preview -> Tech Preview
- Feature available for end-to-end usage.
- Complete end user documentation.
- UTs and e2e tests are present.
- Gather feedback from the users.

#### Tech Preview -> GA
- Sufficient time for feedback.
- Available by default.
- More testing (scale, perf).
- Incorporate feedback from the users on the TechPreview content.

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

On upgrade:
- Scenario 1: Upgrade to OpenShift version having support for adding tags.
  The new status field won't be populated since it is only populated by the
  installer and that can't have happened if the cluster was installed from a
  prior version. Components that consume the new field should take no action
  since there won't be any tags to apply.
- Scenario 2: Upgrade from OpenShift version having support for adding tags to higher:
  Cluster operators that add tags to Azure resources created for the cluster
  should refer to the tag fields and take action. For any new resource created
  post-upgrade, the operator managing the resource will add the user-defined tags
  to the resource. But the same does not apply to already existing resources,
  components may or may not update the resources with the user defined tags.

On downgrade:
- Scenario 1: Cluster installed with OpenShift version not having support for adding
  tags, upgraded to a version supporting tags and later downgraded to installed version.
  The new status field won't be populated since it is only populated by the
  installer and that can't have happened if the cluster was installed from a
  earlier version and upgraded to version having support for tags and downgrade will
  have no impact with the tags functionality too.
- Scenario 2: Cluster installed with OpenShift version having support for adding tags,
  upgraded to higher version and later downgraded to a lower version supporting tags.
  The status field may remain populated, components may or may not continue to tag 
  newly created resources with the additional tags depending on whether given component 
  sill has the logic to add tags post downgrade. 

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Alternatives

## Infrastructure Needed [optional]
