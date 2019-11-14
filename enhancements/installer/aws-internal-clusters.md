---
title: internal-aws-clusters
authors:
  - "@abhinavdahiya"
reviewers:
  - "@wking"
  - "@sdodson"
approvers:
  - "@sdodson"
creation-date: 2019-11-08
last-updated: 2019-11-08
status: implemented
superseded-by:
  - "https://docs.google.com/document/d/1IUCqz6AhxNwYDyx9jPcqbsa1-EloWD13OIONK7YP3JE"
---

# Internal AWS Clusters

## Release Signoff Checklist

- [ x ] Enhancement is `implementable`
- [ x ] Design details are appropriately documented from clear requirements
- [ x ] Test plan is defined
- [ x ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Many customer environments don't require external connectivity from the outside world and as such, they would prefer not to expose the cluster endpoints to public. Currently the OpenShift installer exposes endpoints of the cluster like Kubernetes API, or OpenShift Ingress to the Internet, although most of these endpoints can be made Internal after installation with varying degree of ease individually, creating OpenShift clusters which are Internal by default is highly desirable for users.

## Motivation

### Goals

Install an OpenShift cluster on AWS as internal/private, which is only accessible from my internal network and not visible to the Internet.

### Non-Goals

No additional isolation from other clusters in the network from the ones provided by [shared networking][aws-shared-networking].

## Proposal

To create Internal clusters, the installer binary needs access to the VPC where the cluster will be created to communicate with the cluster’s Kubernetes API, therefore, installing to [existing subnets][aws-shared-networking] is required. In addition to the network connectivity to the endpoints of the cluster, the installer binary should also be able to resolve the newly created DNS records for the cluster.

No public subnets are required, since no public load balancers will be created. And, since public records will not be needed, the requirement for a public Route 53 Zone matching the `BaseDomain` will be relaxed. The installer still creates the private R53 Zone for the cluster.

### User Stories

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints

#### API

A new field `publish` is added to the InstallConfig. Publish controls how the user facing endpoints of the cluster like the Kuberenets API, OpenShift Ingress etc. are exposed. Valid values are `External` (the default) and `Internal`.

#### Resource provided to the installer

The users provide a list of private subnets to the installer and set the `publish` to `Internal`.

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
publish: Internal
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```

#### Basedomain

The `baseDomain` will continue to be used for creating the private Route53 zone for the cluster and all the necessary records, but the requirement that a public Route53 Zone exist corresponding to the `baseDomain` will **NOT** be needed anymore.

The CustomResource [`DNSes.config.openshift.io`][openshift-api-config-dns] `cluster` object's `.spec.publicZone` will be set to empty. This will make sure that the operators do not create any public records for the cluster as per the API [reference][openshift-api-config-dns-empty-public-zone]

#### Public Subnets

No public subnets need to be provided to the installer.

#### Access to Internet

The cluster still continues to require access to Internet.

#### Resources created by the installer

The installer will no longer create (vs. the fully-IPI flow with pre-existing VPC):

- Public Load Balancers for API (6443) (aws_lb.api_external, aws_lb_target_group.api_external, aws_lb_listener.api_external_api)
- No security group that allows 6443 from Internet.
- Public DNS record (aws_route53_record.api_external)
- No public IP address associated to bootstrap-host (associate_public_ip_address: false)
- No security group that allows SSH to bootstrap-host from Internet (aws_security_group_rule.ssh)

The installer will continue to create (vs. the fully-IPI flow with pre-existing VPC):

- Private Load Balancers for Kubernetes API (6443) and Machine Config Server (22623)
- DNS records in the private R53 Zone
- Security Group that allows SSH to the bootstrap-host from the inside the VPC.

#### Bootstrap instance placement

The Bootstrap instance is placed in one of the provided private subnet, compared to the External case it's placed in one of the public subnet.

#### Operators

##### Ingress

The `default` ingresscontroller for the cluster on AWS has LoadBalancer scope set to External as per API [reference][ingresscontroller-default]. But for `Internal` clusters the `default` ingresscontroller for the cluster is explicity set to Loadbalancer scope `Internal`.

#### Limitations

- The Kubernetes API endpoints cannot be made public after installation using some configuration in the cluster. The users will have to manually choose the public subnets from the VPC where the cluster is deployed, created public load balancer with control-plane instances as backend and also ensure the control-plane security groups allow traffic from Internet on 6443 (Kubernetes API port). Also the user will have to make sure they pick public subnets in each Availability Zone as detailed in [documentation][public-lb-to-private-instances].

### Risks and Mitigations

#### Public Service type Load Balancers

Creating Service type Loadbalancer that are public would require users to select public subnets and tag them `kubernetes.io/cluster/<cluster-infra-id>: shared` so that cloud provider can use them to create public load balancers. The choice of public subnets is similarly strict as mentioned [limitation](#limitations).

#### Public OpenShift Ingress

Changing the OpenShift Ingress to be public after installation requires extra steps for users as detailed in previous [section](#public-service-type-load-balancers).

## Design Details

### Test Plan

The containers in the CI cluster that run the installer and the e2e tests will create VPN connection to public VPN endpoint created in the network deployment in the CI AWS account. This will provide the client transparent access to otherwise internal endpoints of the newly created cluster.

### Graduation Criteria

This enhancement will follow standard graduation criteria.

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

Customer owned networking components means the cluster cannot automatically alter those components to track evolving best-practices. The user owns those components and is responsible for maintaining them.

## Alternatives

## Infrastructure Needed

- Network deployment in AWS CI account which includes VPC, subnets, NAT gateways, Internet gateway that provide egress to the Internet for instance in private subnet.
- VPN accessibility to the network deployment created above.

[aws-shared-networking]: aws-customer-provided-subnets.md
[ingresscontroller-default]: https://github.com/openshift/api/blob/6feaabc7037a0688eefb36fd9f4618da7d780dda/operator/v1/types_ingress.go#L75
[openshift-api-config-dns]: https://github.com/openshift/api/blob/6feaabc7037a0688eefb36fd9f4618da7d780dda/config/v1/types_dns.go#L23
[openshift-api-config-dns-empty-public-zone]: https://github.com/openshift/api/blob/6feaabc7037a0688eefb36fd9f4618da7d780dda/config/v1/types_dns.go#L35-L40
[public-lb-to-private-instances]: https://aws.amazon.com/premiumsupport/knowledge-center/public-load-balancer-private-ec2/
