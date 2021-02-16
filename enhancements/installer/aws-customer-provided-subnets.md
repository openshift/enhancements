---
title: allow-customer-provisioned-aws-subnets
authors:
  - "@wking"
reviewers:
  - "@abhinavdahiya"
  - "@sdodson"
approvers:
  - "@abhinavdahiya"
  - "@sdodson"
creation-date: 2019-10-10
last-updated: 2019-10-10
status: implemented
superseded-by:
  - "https://docs.google.com/document/d/12Nu_OvcNnItD4vaH6G0ky0IeAxnvDSpzp5sVrZv7-8M/"
---

# Allow Customer-Provisioned AWS Subnets

## Release Signoff Checklist

- [ x ] Enhancement is `implementable`
- [ x ] Design details are appropriately documented from clear requirements
- [ x ] Test plan is defined
- [ x ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

In many large organizations, IT groups are separated into teams. A
common separation of duties lies between networking/WAN and
application or enabling technology deployments. One group will
provision or assign AWS accounts with core management technology. A
second group will be responsible for provisioning VPC networks
complete with subnets & zone selection, routing, VPN connections, and
ingress/egress rules from the company. Lastly, an applications team
(or individual business units) are permitted into the account to
provision application-specific items (instances, buckets, databases,
pipelines, load balancers, etc.)

To accomplish this and our infrastructure needs, AWS account access
can be separated into "infrastructure" versus
"application". Infrastructure includes elements related to the VPC and
the networking core components within the VPC (VPC, subnets, routing
tables, internet gateways, NAT, VPN). Application items include things
directly touching nodes within the cluster (ELBs, security groups, S3
buckets, nodes). This is a typical separation of responsibilities
among IT groups and deployment/provisioning pipelines within large
companies.

The Application teams will be enabled to specify the subnets that can be used to deploy OpenShift clusters.

## Motivation

### Goals

As an administrator, I would like to create or reuse my own VPCs, subnets and routing schemes that I can deploy OpenShift to.

### Non-Goals

There are no changes in expectations regarding publicly addressable parts of the cluster.

## Proposal

Installer allow users to provide a list of subnets that should be used
for the cluster. Since there is an expectation that networking is
being shared, the installer cannot modify the networking setup
(i.e. the route tables for the subnets or the VPC options like DHCP
etc.) but, changes required to the shared resources like `Tags` that
do not affect the behavior for other tenants of the network will be
made.

The installer validates the assumptions about the networking setup.

Infrastructure resources that are specific to the cluster and resources owned by the cluster will be created by the installer. So resources like security groups, IAM roles/users, ELBs, S3 buckets and ACL on s3 buckets remain cluster-managed.

The infrastructure resources owned by the cluster continue to be clearly identifiable and distinct from other resources.

Destroying a cluster must make sure that no resources are deleted that didn't belong to the cluster.  Leaking resources is preferred over any possibility of deleting non-cluster resources.

### User Stories

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints

#### Resource provided to the installer

The users provide a list of subnets to the installer.

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
platform:
  aws:
    region: us-west-2
    subnets:
    - subnet-1
    - subnet-2
    - subnet-3
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```

#### Subnets

- Subnets must all belong to the same VPC.

- Each subnet must have space for an additional `kubernetes.io/cluster/.*: shared` tag. AWS currently has a hard limit of [50 tags per resource][aws-tag-limits].
The k8s cloud provider code in kube-controller-manager uses this tag to find the subnets for the cluster to create LoadBalancer Services. Currently Terraform does not support management of resources that already exist, so we cannot tag the subnets using Terraform’s aws_subnet resource or data_sources. Instead, we will tag the subnets from Go immediately before applying the Terraform configuration.

- The CIDR block for each one must be contained by Networking.MachineCIDR.

- Subnets will be classified into `public` and `private` subnets internally using upstream cloud-provider [code][cloud-provider-public-private-classification]
This classification is required for number of reasons like, public and private load balancers, placement of ec2 instances etc.

- A public subnet must exist in every zone which has a private subnet, otherwise load-balancer routing is [impossible][public-lb-to-private-instances].

- No two public subnets or two private subnets can share a single availability zone, because that might be an [error][cloud-provider-multiple-subnets-in-same-az] for future cloud provider load-balancer allocation.

#### VPC

- The VPC for the cluster is discovered based on the subnets provided to the installer.

- VPC needs to have `enableDnsSupport` and `enableDnsHostnames` attributes turned on so that Route53 zones attached to the VPC can be used to resolve cluster’s internal DNS records.

- The VPC’s CIDR block must be contained by Networking.MachineCIDR.

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

#### Destroying cluster

- Destroying a cluster will remain mostly unchanged, hinging on the `kubernetes.io/cluster/<cluster-infra-id>: owned` tag.

- Destroying a cluster removes the `kubernetes.io/cluster/<cluster-infra-id>: shared` tag from resources that have it. These resources are not deleted.

#### Limitations

- Subnets must provide networking (NAT gateways, internet gateways,
  route tables, etc.).  At least for the initial implementation, we
  will not validate this assumption, will attempt to install the
  cluster regardless, and will fail after having created cluster-owned
  resources if the assumption is violated.  Future work can iterate on
  pre-create validation for the networking assumptions, if they turn
  out to be a common tripping point.

- Also there might be subnet level Network ACLs that block or hinder with inbound/outbound traffic for the cluster and those cannot be sanely validated by the installer and at least for the initial implementation, we will not validate this assumption, will attempt to install the cluster regardless.

### Risks and Mitigations

#### Isolation between clusters

Deploying OpenShift clusters to pre-existing network has the side-effect of reduced the isolation of cluster services.

- ICMP ingress is allowed to entire network.

- TCP 22 ingress (SSH) is allowed to the entire network.
We can restrict this to control plane and compute security groups, and require folks setting up a SSH bastion to either add one of those security groups to their bastion instance or alter our rules to allow their bastion.

- Control plane TCP 6443 ingress (Kubernetes API) is allowed to the entire network.

- Control plane TCP 22623 ingress (MCS) is allowed to the entire network.
This should be restricted to the control plane and compute security groups, instead of the current by-VPC-CIDR logic to avoid leaking sensitive Ignition configs to non-cluster entities sharing the VPC.

## Design Details

### Test Plan

To test multiple clusters can be created in same networking deployment, CI account will host a networking deployment and the CI templates will be modified to use the subnets to create a cluster.

### Graduation Criteria

*This enhancement will follow standard graduation criteria.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- Community Testing
- Sufficient time for feedback
- Upgrade testing from 4.3 clusters utilizing this enhancement to later releases
- Downgrade and scale testing are not relevant to this enhancement

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Upgrade / Downgrade Strategy

Not applicable

### Version Skew Strategy

Not applicable.

## Implementation History

## Drawbacks

Customer owned networking components means the cluster cannot automatically change things about the networking during upgrade.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.

Listing these here allows the community to get the process for these resources
started right away.

[aws-tag-limits]: https://docs.aws.amazon.com/AWSEC2/latest/UserGuide/Using_Tags.html#tag-restrictions
[cloud-provider-public-private-classification]: https://github.com/kubernetes/kubernetes/blob/103e926604de6f79161b78af3e792d0ed282bc06/staging/src/k8s.io/legacy-cloud-providers/aws/aws.go#L3355-L3398
[public-lb-to-private-instances]: https://aws.amazon.com/premiumsupport/knowledge-center/public-load-balancer-private-ec2/
[cloud-provider-multiple-subnets-in-same-az]: https://github.com/kubernetes/kubernetes/blob/103e926604de6f79161b78af3e792d0ed282bc06/staging/src/k8s.io/legacy-cloud-providers/aws/aws.go#L3328-L3334
