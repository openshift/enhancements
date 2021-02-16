---
title: azure-internal-with-user-defined-outbound-routing
authors:
  - "@abhinavdahiya"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-05-20
last-updated: 2020-05-20
status: implementable
see-also:
  - "./enhancements/azure-private-internal-clusters.md"
  - https://docs.microsoft.com/en-us/azure/aks/egress-outboundtype
  - https://github.com/openshift/installer/pull/3324
replaces:
superseded-by:
---

# Azure Internal Clusters with User Defined Outbound Routing

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Currently for internal clusters on Azure, OpenShift clusters always
use Public Standard Loadbalancers for Internet egress. This
configuration requires public IPs, public loadbalancers etc. which
some customers do not want for their internal clusters. This proposal
allows the users to choose their own outbound routing for Internet
allowing them to use any pre-existing setups instead of the
per-cluster OpenShift recommended way.

## Motivation

Internal clusters on Azure require explicit setup to allow outbound
routing or egress to Internet to pull container images, updates,
access cloud APIs etc. Currently the cluster uses a public standard
loadbalancer, with a dummy inbound_rule, to provide the outbound
routing to Internet. But this setup forces customers that either have
pre-existing setups like PROXY for egress or do not require any egress
to Internet at all i.e. air-gapped into the above prescribed setup
without allowing them to change it. Often these customers have
policies that disallow creation resources like public IP addresses,
which causes the installations to fail and forces the customers to use
user-provided workflow.

Allowing the customer to take over the responsibility of routing allows the user flexibility of using the installer-provisioned workflow.

### Goals

1. Allow users to choose user defined routing mode for outbound with pre-exiting networks.

### Non-Goals

## Proposal

### Configuration

The install-config.yaml should include a new field for Azure platform that allows the users to configure the outbound type to `Loadbalancer` or `UserDefined`, with the default being `Loadbalancer`.

The user should only be allowed to change the outbound type when using pre-exiting networking as outbound routing needs to be setup by user before installing the cluster into the network.

### Public loadbalancers

For outbound type `Loadbalancer` the installer creates a public
loadbalancer with no inbound_rules but one outbound_rule per IPFamily
to provide egress for bootstrap, control-plane and compute nodes. To
manage the membership of the nodes to the backend of the public load
balancer, the installer creates a Kubernetes service type
`Loadbalancer`. In addition to the service object, the installer also
makes sure the `Machine(set)` objects have the `publicLoadBalancer`
field set to the public loadbalancer. Adding the `publicLoadBalancer`
field ensures that virtual machines use the public loadbalancer for
egress from creation and do not have to switch the egress strategy at
later time when nodes are added to the backend by the
kube-cloud-controller, which can be quite a bit after the virtual
machine has booted and pulled content from Internet.

For outbound type `UserDefined` the installer does not create the outbound_rule or the Kubernetes service type Loadbalancer or set the `Machine(set)s` object's `publicLoadBalancer` field to the public loadbalancer.

### User Defined routing requirements

Here are some of the expectations from the outbound routing setup by users,

- Users must ensure that egress to Internet is possible to pull container images unless using a internal registry mirror.
- Users must ensure that the cluster is able to access the Azure APIs from the cluster.
- Various other [whitelisted endpoints](https://docs.openshift.com/container-platform/4.4/installing/install_config/configuring-firewall.html) must also be allowed.

### User Stories

#### Internal cluster with PROXY for Internet access

The user will be using proxy configuration with user defined routing to allow egress to internet. Users must be careful that cluster operators currently do not access the Azure APIs using proxy and therefore access to Azure APIs should be available outside of proxy.

When using the default route table for subnets with 0.0.0.0/0
populated automatically by Azure, all Azure APIs requests are routed
over Azure's internal network even though the IP addresses are
public. So in this scenario as long as the Network Security Group
rules allow egress to Azure API endpoints, proxy with user defined
routing configuration in install-config.yaml will allow users to
create internal clusters with no public endpoints.

#### Internal cluster with Azure Firewall for Internet access

Users can use Azure Firewall to provide outbound routing for the virtual network used to install the cluster. An example of such a setup is outlined [here](https://docs.microsoft.com/en-us/azure/aks/egress-outboundtype#deploy-a-cluster-with-outbound-type-of-udr-and-azure-firewall)

Using a virtual network setup as outlined above and user defined routing configuration the users can create internal clusters with no public endpoints

#### Internal cluster with no Internet access

Users that have virtual networks with no access to Internet depend on internal registry mirrors accessible to the private network to pull container images. So as long as the access to the Azure APIs is ensured from the cluster, the users can use the imageContentSources and user defined outbound type routing to create internal clusters with no public endpoints.

### Risks and Mitigations

1. There are no great ways to validate if the user's outbound routing
   setup is configured correctly for the OpenShift cluster and
   therefore it will result in not easy to debug failure modes and
   will probably require a lot more back and forth to understand the
   setup to triage the issue. The bootstrap failure log bundle is
   based on SSH access to bootstrap machines and therefore those logs
   should help uncover the symptoms to at least narrow down the
   failures.

2. User Defined Routing setup is not easy for customers esp. wrt to ensuring access to Azure APIs from the cluster as Azure does not provide AWS private link like capabilities for all the Azure API endpoints making access to these endpoints difficult when using air-gapped virtual networks. There are plans to provide such a capability and that should make it easy to configure.

3. User Defined Routing makes the load balancer setup for Installer substantially more complex esp. with IPv6 limitations already present in Azure.

## Design Details

### InstallConfig

Update the install config to include

```go
// OutboundType is a strategy for how egress from cluster is achieved.
// +kubebuilder:validation:Enum="";Loadbalancer;UserDefinedRouting
type OutboundType string

const (
	// LoadbalancerOutboundType uses Standard loadbalancer for egress from the cluster.
	// see https://docs.microsoft.com/en-us/azure/load-balancer/load-balancer-outbound-connections#lb
	LoadbalancerOutboundType OutboundType = "Loadbalancer"
	// UserDefinedRoutingOutboundType uses user defined routing for egress from the cluster.
	// see https://docs.microsoft.com/en-us/azure/virtual-network/virtual-networks-udr-overview
	UserDefinedRoutingOutboundType OutboundType = "UserDefinedRouting"
)

// Platform stores all the global configuration that all machinesets
// use.
type Platform struct {
...
	// ComputeSubnet specifies an existing subnet for use by compute nodes
	ComputeSubnet string `json:"computeSubnet,omitempty"`

	// outboundType is a strategy for how egress from cluster is achieved.
	// The default is `Loadbalancer`.
	// +kubebuilder:default=Loadbalancer
	// +optional
	OutboundType OutboundType `json:"outboundType"`
}
```

Following validations should be added,

1. Ensure `UserDefinedRouting` is only used when installing to a pre-existing virtual network.

### Loadbalancer

#### when is public IPv4 address required

- it's required for External clusters
- it's required for Internal clusters when outboundType is `Loadbalancer`
- it's required for IPv6 clusters because AzureRM LoadBalancers cannot have only IPv6 frontend :/

#### when is public IPv6 address required

- it's required for External IPv6 clusters
- it's required for Internal clusters when outboundType is `Loadbalancer`

#### azure_lb_rules

- the k8s API rules are created for External clusters
- the k8s API rules are NOT created for Internal clusters

#### azure_lb_outbound_rules

- the k8s API rules are created for Internal clusters. A outbound rule is created for each IPFamily similar to the frontends for azure_lb.
- the k8s API rules are NOT created for External clusters

#### azure_lb_backend

The backends are only created when the corresponding frontend configurations are created because of the failure from Azure

```console
Load Balancer /subscriptions/xx/resourceGroups/xx/providers/Microsoft.Network/loadBalancers/xx-public-lb does not have Frontend IP Configuration, but it has other child resources. This setup is not supported.
```

Since the backends are not created for certain cases, the master and bootstrap modules need to skip adding the virtual machines to the azure_lb backends. Although it should be simple to switch on whether the backend was created i.e null/vs not null, the terraform [issue](https://github.com/hashicorp/terraform/issues/12570) doesn't allow such conditional in count.

```console
ERROR Error: Invalid count argument
ERROR
ERROR   on ../../../../../../../tmp/openshift-install-941276375/bootstrap/main.tf line 142, in resource "azurerm_netwo
ERROR  142:   count = var.elb_backend_pool_v4_id == null ? 0 : 1
ERROR
ERROR The "count" value depends on resource attributes that cannot be determined
ERROR until apply, so Terraform cannot predict how many instances will be created.
ERROR To work around this, use the -target argument to first apply only the
ERROR resources that the count depends on.
```

And therefore, the master and bootstrap modules need to recreate the conditions of `need_public_ipv{4,6}` using the inputs `use_ipv{4,6}`, `private`, `outbound_udr`.

#### azure_lb

The public loadbalancer is always created to keep the complexity to the minimum. The public IPs, frontend configurations, rules and backends will not be attached when necessary. A loadbalancer with no rules is already free in Azure so it should not add any extra cost to the user.

### Test Plan

The easiest way to test the configuration would be to verify the [user story 2][#Internal-cluster-with-Azure-Firewall-for-Internet-access]. AKS provides it as an example setup for creating internal clusters using AKS with no public IPs and re-using that verifying such a setup works on OpenShift is highly valueable.

### Graduation Criteria

None

### Upgrade / Downgrade Strategy

This configuration will only be supported for new installations and therefore has no upgrade or downgrade action items.

### Version Skew Strategy

This configuration will only be supported for new installations and therefore has no version skew action items.

## Drawbacks

Highlighted in [risks][#Risks-and-Mitigations]

## Alternatives

None yet.
