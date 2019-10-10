---
title: Customer-Provisioned-VPC-Subnets
authors:
  - "@wking"
reviewers:
  - "@abhinavdahiya"
  - "@sdodson"
  - "@crawford"
approvers:
- "@abhinavdahiya"
- "@sdodson"
- "@crawford"
creation-date: 2019-10-10
last-updated: 2019-10-10
status: implementable
replaces:
  - "https://docs.google.com/document/d/12Nu_OvcNnItD4vaH6G0ky0IeAxnvDSpzp5sVrZv7-8M/"
---

# customer-provisioned-vpc-and-subnets

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

## Summary

In many large organizations, IT groups are separated into teams. A common
separation of duties lies between networking/WAN and application or enabling
technology deployments. One group will provision or assign AWS accounts with
core management technology. A second group will be responsible for provisioning
VPC networks complete with subnet & zone selection, routing, VPN connections,
and ingress/egress rules from the company. Lastly, an applications team (or
individual business units) are permitted into the account to provision
application-specific items (instances, buckets, databases, pipelines, load
balancers, etc.)

To accomplish this, our infrastructure needs and AWS account access can be
separated into "infrastructure" versus "application". Infrastructure includes
elements related to the VPC and the networking core components within the VPC
(VPC, subnets, routing tables, internet gateways, NAT, VPN). Application items
include things directly touching nodes within the cluster (ELBs, security
groups, S3 buckets, nodes). This is a typical separation of responsibilities
among IT groups and deployment/provisioning pipelines within large companies.

## Motivation

### Goals

As an administrator, I would like to create or reuse my own VPCs, subnets and
routing schemes that I can deploy OpenShift to.

### Non-Goals

There are no changes in expectations regarding publicly addressable parts of the
cluster.

## Proposal

The installer should be able to install a cluster in an pre-existing VPC.  This
requires providing subnets too as there is no sane way installer can carve out
network ranges for the cluster.

There is an expectation that since the VPC and subnets are being shared, the
installer cannot modify the networking setup (i.e. the route tables for the
subnets or the VPC options like DHCP etc.).  Therefore the installer can only
validate the assumptions about the networking setup.  The installer may assume
that use has NAT gateways, internet gateways, etc. setup, and installation may
fail if these assumptions are violated.  Or the installer may attempt to
validate networking before attempting the install.

Resources that are specific to the cluster and resources owned by the cluster
can be created by the installer.  So resources like security groups, IAM
roles/users, ELBs, S3 buckets and ACL on s3 buckets remain cluster-managed.

Any changes required to the shared resources like Tags that do not affect the
behavior for other tenants can be made. Resources owned by the cluster must be
clearly identifiable and distinct from other resources.

Destroy cluster must make sure that no resources are deleted that didn't belong
to the cluster.  Leaking resources is preferred over any possibility of deleting
non-cluster resources.

### User Stories [optional]

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints [optional]

#### Subnets

- Subnets must all belong to the same VPC.  The install-config wizard will ensure
this by prompting for the VPC first and then providing subnets as a choice
widget (not free-form).  But generic install-config validation needs to cover
user-provided install-config.yaml as well, so it should look up the subnets and
ensure that they all belong to the same VPC.

- Subnets must have space for an additional kubernetes.io/cluster/.\*: shared tag,
and AWS currently has a hard limit of 50 tags per resource.  The cloud provider
code in kube-controller-manager uses this to create LoadBalancer Services.  If
cloud-provider cannot find any subnets based on that that tags, it uses the
deprecated code-path of using the “current” subnets of the EC2 instance.
Control plane EC2 instances are always created in the node subnet, therefore the
cloud provider fails to find a public subnet for the ingress-controller’s
LoadBalancer type Service.

- Subnets’ CIDR blocks must all be contained by Networking.MachineCIDR.

- Unvalidated: Subnets must provide networking (NAT gateways, internet gateways,
route tables, etc.).  At least for the initial implementation, we will not
validate this assumption, will attempt to install the cluster regardless, and
will fail after having created cluster-owned resources if the assumption is
violated.  Future work can iterate on pre-create validation for the networking
assumptions, if they turn out to be a common tripping point.

- The installer will classify subnets as public and private by copying the
cloud-provider logic (unfortunately not provided as a public API that can be
vendored at the moment).  Once classified:

- A public subnet must exist in every zone which has a private subnet, otherwise load-balancer routing is impossible.  This condition is waived for private clusters, since no public load-balancers are created in that case.
- No two public subnets or two private subnets can share a single availability zone, because that might be an error for future cloud provider load-balancer allocation.

#### VPC

- VPC needs to have enableDnsSupport and enableDnsHostnames attributes turned on so that Route53 zones attached to the VPC can be used to resolve cluster’s internal DNS records.
https://docs.aws.amazon.com/vpc/latest/userguide/vpc-dns.html#vpc-dns-support
- VPC must not have a kubernetes.io/cluster/.\*: owned tag.
- The VPC’s CIDR block must contain Networking.MachineCIDR.

#### Machine sets

- Platform zone information, if set, must only contain zones for which there are
private subnet entries.

#### Resources created by the installer
The installer will no longer create (vs. the fully-IPI flow):
- Internet gateways (aws_internet_gateway)
- NAT gateways (aws_nat_gateway, aws_eip.nat_eip)
- Subnets (aws_subnet)
- Route tables (aws_route_table, aws_route, aws_route_table_association, aws_main_route_table_association)
- VPCs (aws_vpc)
- VPC DHCP options (aws_vpc_dhcp_options, vpc_dhcp_options_association)
- VPC endpoints (aws_vpc_endpoint)

The installer will continue to create (vs. the fully-IPI flow):
- AMI copies (aws_ami_copy)
- IAM roles and profiles (aws_iam_instance_profile, aws_iam_role, aws_iam_role_policy)
- Instances (aws_instance, aws_network_interface.master)
- Load balancers (aws_lb, aws_lb_listener, aws_lb_target_group, aws_lb_target_group_attachment)
- Route 53 resources (aws_route53_zone, aws_route53_record)
- S3 resources (aws_s3_bucket, aws_s3_bucket_object)
- Security groups (aws_security_group, aws_security_group_rule)

The installer will grow new creation code (vs. the fully-IPI flow) for:
- Adding kubernetes.io/cluster/.*: shared to the user-provided subnets.  This will happen in Go immediately before calling out to Terraform to begin creating new resources.

#### API
The install-config YAML will grow one property on the AWS platform structure:
- Subnets: a slice of subnet ID strings
The install-config wizard will add two prompts:
- A Select prompt:
  - Message: VPC
  - Help: The VPC to be used for installation.
  - Default: installer-created
  - Options: [“installer-created”] + results of a DescribeVpcs call.  Possibly checking tags and excluding VPCs which have kubernetes.io/cluster/.*: owned tags.
- A MultiSelect prompt:
  - Message: Subnets
  - Help: The subnets to use for the cluster.
  - Options: results of a DescribeSubnets call on the selected VPC.  Possibly checking tags and excluding subnets which have kubernetes.io/cluster/.\*: owned tags.

#### Isolation between clusters

Security group rules based on the VPC CIDR (e.g. machine-config server ingress) will be accessible to other clusters sharing the same VPC, and previous work considers the Ignition configs served by the MCS to be sensitive data.  Digging into the affected rules:
- ICMP ingress.  We may be able to drop this completely.  I don’t see a need for it, and folks who want it can always go UPI ;).
- TCP 22 ingress (SSH).  We can restrict this to control plane and compute security groups, and require folks setting up a SSH bastion to either add one of those security groups to their bastion instance or alter our rules to allow their bastion.
- Control plane TCP 6443 ingress (Kubernetes API).  We can restrict this to the API load balancers.  Folks hitting localhost will not get out to the security group level, and everyone else should be coming in via the load balancers.
- Control plane TCP 22623 ingress (MCS): this should be restricted to the control plane and compute security groups, instead of the current by-VPC-CIDR logic to avoid leaking sensitive Ignition configs to non-cluster entities sharing the VPC.


### Risks and Mitigations

#### Tagging subnets

Currently Terraform does not support management of resources that already exist, so we cannot tag the subnets using Terraform’s aws_subnet resource or data_sources.  Instead, we will tag the subnets from Go immediately before applying the Terraform configuration.

#### Destroy

Destroy will remain mostly unchanged, hinging on the kubernetes.io/cluster/.\*: owned tag.  We will need to grow new code to store kubernetes.io/cluster/.\*: shared in metadata.json and remove it from resources on which it is found.

We should probably revert:
- #1268, which began removing instance profiles by name.  That was a workaround to recover from openshift-dev clusters which were partially-deleted by the DPP reaper.  Folks using the installer’s destroy code won’t need it, and while the risk of accidental name collision is low, I don’t think it’s worth taking that risk.
- #1704, which skipped by-tag network interface deletion to reduce AWS API load.  But since #2169, we will be able to have by-tag interface deletion without a throttle-inducing load.

## Design Details

### Test Plan

CI jobs should set up a VPC, subnets, and routing using our UPI flow and launch the cluster-under-test into that infrastructure.  We may want to adjust the resource grouping in our CloudFormation templates to do this, e.g. shifting the S3 endpoint to a later or separate template to show that users don’t need to provide it.  We will not add CI running multiple clusters inside a single VPC at the moment, although we may come back and add this later.

### Graduation Criteria

This enhancement will follow standard graduation criteria.

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA

- Community Testing
- Sufficient time for feedback
- Upgrade testing from 4.3 clusters utilizing this enhancement to later releases
- Downgrade and scale testing are not relevant to this enhancement

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

## Drawbacks

TODO

## Alternatives

TODO, some can be ported from this private document https://docs.google.com/document/d/1eNtnrsMUL2efRC5Y8w6CL4f_GxTHrIij8cnyQQnUhW8/edit#heading=h.ttvcmxr8sl3b
