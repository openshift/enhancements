---
title: azure-cluster-scaling-for-outbound-traffic
authors:
  - @lranjbar
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @bennerv, aro
  - @patrickdillion, installer
  - @jhixson74, installer
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - TBD
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: 2023-08-02
last-updated: 2023-08-02
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/ARO-2728
---


# Azure Cluster Scaling for Outbound Traffic

## Summary

Currently customers with OpenShift clusters in Azure experience a number of difficulties
in scaling their clusters past 62 total nodes for outbound traffic. This enhancement outlines 
proposes an approach for openshift-installer that improves customer experience and aligns with ARO.

## Motivation

The primary motivation is to define a set of default network resources to allow customers on Azure to
scale their clusters for outbound traffic without service degradation. The current default settings 
for setting `disableOutboundSNAT` and the public IPs on the load balancer are insufficent for cluster 
scaling past 62 nodes. This enhancement uses the [OpenShift recommended performance and scalability 
practices](https://docs.openshift.com/container-platform/4.13/scalability_and_performance/recommended-performance-scale-practices/recommended-infrastructure-practices.html) to help define these network resources.

*Explaination of the Customer's Current Problem with OpenShift Scaling on Azure*

Currently, customers scaling Openshift clusters with large amout of outbound traffic experience port
exhaustion beyond 62 nodes. This is because by default Azure allocates 64,000 outbound Source Network Address 
Translation (SNAT) ports per public IP address used for SNATing. The default number of ports allocated 
per virtual machine in Azure is 1024. At installation we use only one public IP for the cluster. This 
results in a limit of 62 virtual machines. (floor(64000 / 1024) = 62)

When installing Openshift by default the option `disableOutboundSNAT` is set to false in Azure. 
With this setting any newly created kubernetes services of type load balancer will create 
new public IP addresses dynamically in Azure. This behavior is surprising to the customer because 
these new IPs will also be used with existing load balancers but does not increase the number of SNAT ports
beyond 64,000. In this scenario the customer has been given a new public IP but not the benefit of more ports.

In addition customers with the need to scale clusters with large amounts of outbound traffic 
also commonly have the business need to be able to define the IPs for those clusters. These 
customers have applications which define a set of well-known IP address sources in their allow-lists.
The dynamic IP address creation also messes with their allow-listing rules when a new service is created.


### User Stories

- As an OpenShift customer I need the ability to configure additional public IPs to be 
used for egress traffic so that I do not experience network errors with my cluster.

- As an OpenShift customer I need the ability to scale up cluster egress traffic using the
recommended performance and scalibity practices so that I can meet my businesses scaling needs.

- As an OpenShift customer I need the ability to set a specific list of IPs for my clusters
egress traffic so that I can set those IP's in allow-lists of other business applications.

### Goals

- Define a set of network resources that used in the default OpenShift installation that
will allow the customer to scale to 200 nodes without much trouble.
- Inform the customer how the `disableOutboundSNAT` property in Azure interacts with OpenShift.

### Non-Goals

- Changes to day 2 operations for scaling are out of scope for this enhancement. The goal 
is to set a good default base at installation to provide smoother day 2 operations.

## Proposal

Expose `disableOutboundSNAT` in the install config's cluster-scoped properties for Azure. 
Update documentation to inform the customer of how to use this option.

### Workflow Description

1. Cluster creator specifies the `disableOutboundSNAT: true` property in their install config.
2. Installer sets this properties using the `CloudProviderConfig` if provided.  The default 
for disableOutboundSNAT is currently false, this default should change to true.
3. Installer sets up three additional public IP addresses for the cluster at install time. (Avoids scanerio 2 below)
4. OpenShift install continues as normal.

#### Variation [optional]

A variation is to have a more opininated approach in the installer about the disableOutboundSNAT
property instead of exposing it. In this approach would set the disableOutboundSNAT property based off
other inputs in the install config.

There are three scenarios in that the installer team should think about to create a more opinionated approach.

Scenario 1 (Private cluster, LB outbound type):
- There is an outbound rule on the LB
- If you set `disableOutboundSNAT = true` in the cloud provider config and all public IP addresses 
the installer creates via terraform, the explicit outbound rule will be used for egress

Scenario 2 (public cluster, LB outbound type):
- There is no outbound rule on the LB
- If you set `disableOutboundSNAT = true` in the cloud provider config and all public IP addresses 
the installer create via terraform set this to true as well, **egress will not work.**

Scenario 3 (public cluster, LB outbound type):
- There is no outbound rule on the LB
- If you set `diableOutboundSNAT = true` **ONLY in the cloud provider config**, egress will work 
through the terraform-created public IP addresses and egress will work.


### API Extensions

This enhancment would modify the [CloudProviderConfig API Object](https://github.com/openshift/installer/blob/master/pkg/asset/manifests/azure/cloudproviderconfig.go) to include the property `disableOutboundSNAT` in the config.

```go

config := config{
		authConfig: authConfig{
			Cloud:                       params.CloudName.Name(),
			TenantID:                    params.TenantID,
			SubscriptionID:              params.SubscriptionID,
			UseManagedIdentityExtension: true,
			UserAssignedIdentityID: "",
		},
		ResourceGroup:     params.ResourceGroupName,
		Location:          params.GroupLocation,
		SubnetName:        params.SubnetName,
		SecurityGroupName: params.NetworkSecurityGroupName,
		VnetName:          params.VirtualNetworkName,
		VnetResourceGroup: params.NetworkResourceGroupName,
		RouteTableName:    params.ResourcePrefix + "-node-routetable",
		CloudProviderRateLimit:       false,
		CloudProviderBackoff:         true,
		CloudProviderBackoffDuration: 6,

		UseInstanceMetadata: true,
		LoadBalancerSku:             "standard",
		ExcludeMasterFromStandardLB: &excludeMasterFromStandardLB,
    DisableOutboundSNAT: true, // <-- Add this property
	}


```

### Implementation Details/Notes/Constraints [optional]

### Risks and Mitigations

There is a risk that a customer with a public that the customer could set disableOutboundSNAT 
to true, without having a public IP address to use. We propose having the installer create 
IP addresses through terraform to mitgate this risk.

### Drawbacks

The complexity of this task is percieved to be quite small. However it does have the drawback
of having yet another configuration option to explain to the customer. This property is
exposed to the customer in Azure already, so a customer might come across it anyway.

## Design Details

### Open Questions [optional]


### Test Plan

The [tests for the CloudProviderConfig API object](https://github.com/openshift/installer/blob/master/pkg/asset/manifests/azure/cloudproviderconfig_test.go) should be updated to the new default behavior.

### Graduation Criteria


#### Dev Preview -> Tech Preview


#### Tech Preview -> GA


#### Removing a deprecated feature


### Upgrade / Downgrade Strategy

This is only exposing a property that already exists in the generated cloud provider config 
to the customer. The upgrade / downgrade should be a non-issue. 

### Version Skew Strategy

Version skew should not be an issue for this enhancement. 

### Operational Aspects of API Extensions

Sets the property `disableOutboundSNAT` to true in the cloud provider config.

#### Failure Modes


#### Support Procedures


## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

Refer to the variation described above.

## Infrastructure Needed [optional]

This enhancement shouldn't need extra infrastructure beyond OpenShift CI.
