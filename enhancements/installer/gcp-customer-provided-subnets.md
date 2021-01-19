---
title: allow-customer-provisioned-gcp-subnets
authors:
  - "@jstuever"
reviewers:
  - "@abhinavdahiya"
  - "@sdodson"
approvers:
  - "@abhinavdahiya"
  - "@sdodson"
creation-date: 2019-08-12
last-updated: 2019-10-21
status: implemented
---

# Allow Customer-Provisioned GCP Subnets

## Release Signoff Checklist

- [ X ] Enhancement is `implementable`
- [ X ] Design details are appropriately documented from clear requirements
- [ X ] Test plan is defined
- [ X ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

In many large organizations, IT groups are separated into teams. A
common separation of duties lies between networking/WAN and
application or enabling technology deployments. One group will
provision or assign AWS accounts with core management technology. A
second group will be responsible for provisioning VPC networks
complete with subnet & zone selection, routing, VPN connections, and
ingress/egress rules from the company. Lastly, an applications team
(or individual business units) are permitted into the account to
provision application-specific items (instances, buckets, databases,
pipelines, load balancers, etc.)

To accomplish this, our infrastructure needs and GCP account access
can be separated into "infrastructure" versus
"application". Infrastructure includes elements related to the VPC and
the networking core components within the VPC (VPC, subnets, routing
tables, internet gateways, NAT, VPN). Application items include things
directly touching nodes within the cluster (LBs, security groups,
storage, nodes). This is a typical separation of responsibilities
among IT groups and deployment/provisioning pipelines within large
companies.

The Application teams will be enabled to specify the subnets that can be used to deploy OpenShift clusters.

## Motivation

### Goals

As an administrator, I would like to create or reuse my own VPCs, subnets and routing schemes that I can deploy OpenShift to.

### Non-Goals

There are no changes in expectations regarding publicly addressable parts of the cluster.

## Proposal

Installer should be able to install cluster in an pre-existing Virtual Network. This requires providing subnets too as there is no sane way installer can carve out network ranges for the cluster.

There is an expectation the Virtual Network and subnets are being shared.

Egress to the internet from the control plane and compute nodes requires Cloud NAT because these machines do not have any public address. And since multiple Cloud NATs cannot be configured on the shared subnets, the installer cannot configure it and therefore the user will be required to have the subnets setup with internet access.

Resources that are specific to the cluster and resources owned by the cluster can be created by the installer. So resources like firewall rules, LBs, service accounts etc stay under the management of the cluster.

Any changes required to the shared resources like Tags that do not affect the behavior for other tenants can be made.

Resources owned by the cluster and resources must be clearly identifiable.

Destroy cluster must make sure that no resources are deleted that didn't belong to the cluster. leaking resources is preferred than any possibility of deleting non-cluster resource.

### Implementation Details/Notes/Constraints

#### Resources provided to the installer

The users provide a list of subnets to the installer.

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
platform:
  gcp:
    region: us-west1
    projectID: example-project
    network: example-network
    controlPlaneSubnet: example-master-subnet
    computeSubnet: example-worker-subnet
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```

- network: The name of the network (vpc).

- controlPlaneSubnet: The name of the subnet where the control plane should be deployed.

- computeSubnet: The name of the subnet where the compute instances should be deployed. This can be the same as the control plane subnet.

#### Network (VPC)

- The network must be in the same project as the cluster. We need to confirm if a shared VPC will work before we can remove this assumption.

#### Subnets

- Subnets must all belong to the same VPC. The install-config wizard will ensure this by prompting for the VPC first and then providing subnets as a choice widget (not free-form). But generic install-config validation needs to cover user-provided install-config.yaml as well, so it should look up the subnets and ensure that they all belong to the same VPC.

- Subnets must all be subset of network_cidr [Networking.MachineCIDR][networking-machinecidr].

- Unvalidated: Subnets must not contain firewall rules which block necessary traffic.

- Unvalidated: Subnets must provide networking (Cloud NATs, Cloud
  Routers, etc.).  At least for the initial implementation, we will
  not validate this assumption, will attempt to install the cluster
  regardless, and will fail after having created cluster-owned
  resources if the assumption is violated.  Future work can iterate on
  pre-create validation for the networking assumptions, if they turn
  out to be a common tripping point.

#### Resources created by the installer

The installer will no longer create (vs. the fully-IPI flow):

- Network (google_compute_network)

- Subnets (google_compute_subnetwork)

- Cloud Router (google_compute_router)

- Cloud NAT (google_compute_router_nat)

- NAT IPs (google_compute_address)

The installer will continue to create (vs. the fully-IPI flow):

- Firewall rules (google_compute_firewall)

- Load Balancers (google_compute_address, google_compute_http_health_check, google_compute_target_pool, google_compute_forwarding_rule)

- Internal DNS zone (google_dns_managed_zone)

- Internal and external DNS entries (google_dns_record_set)

- Service Accounts (google_service_account, google_project_iam_member)

- Bootstrap ignition bucket (google_storage_bucket, google_storage_bucket_object, google_storage_object_signed_url)

- Instances (google_compute_instance, google_compute_address)

#### Changes to resources created by the installer

- The MCS firewall rule currently limits by source_ranges of network_cidr, master_nat_ip, and worker_nat_ip. However, we have no way of knowing what the master_nat_ip or worker_nat_ip is. As a result, the MCS firewall rule will need to become more generic, thus available externally (0.0.0.0/0).

#### API

The install-config YAML will grow two properties on the [gcp platform structure][gcp-platform-structure]

- ControlPlaneSubnet

- ComputeSubnet

The install-config wizard will add three prompts:

- A Select prompt:
  Message: VPC
  Help: The VPC to be used for installation.
  Default: new
  Options: [“new”] + results of a [networks.list][networks-list] call.

- A Select prompt:
  Message: Control-plane subnet
  Help: The subnet to be used for the control plane instances.
  Options: results of a [subnetworks.list][subnetworks-list] call filtered by the selected VPC.
  Only when: VPC is not “new”

- A Select prompt:
  Message: Compute subnet
  Help: The subnet to be used for the compute instances.
  Options: results of a [subnetworks.list][subnetworks-list] call filtered by the selected VPC.
  Only when: VPC is not “new”

#### Destroy

There should be no work required. For gcp, we filter on name containing `${INFRA_ID}`. As a result, destroy should only delete things that match this naming convention.

#### CI

CI jobs should set up a VPC, subnets, routing, and NAT using our [UPI flow][upi-flow] and launch the cluster-under-test into that infrastructure.

### Risks and Mitigations

#### Isolation between clusters

The clusters will be isolated by firewall rules. Specifically, by tagging the instances with `${INFRA_ID}-master` and `${INFRA_ID}-worker`, and using those tags in the firewall rules to ensure only desired traffic is allowed. However, there are a few modifications required to make this absolute:

- ICMP ingress.  We may be able to drop this completely.

- TCP 22 ingress (SSH). The bootstrap instance  allows global access (0.0.0.0/0). This can probably stay the same as it is only temporary and of limited risk.  The control-plane and compute instances allow the `network_cidr` [Networking.MachineCIDR][networking-machinecidr]. This could be restricted to tags if determined the risk is sufficient. Using different ssh keys would help as well.

- TCP 22623 ingress (MCS). The MCS firewall rule is to be opened up externally in order to remove the need for `master_nat_ip` and `worker_nat_ip` as inputs. Switching to using internal load balancers would remediate this.

## Design Details

### Test Plan

To test multiple clusters can be created in same networking deployment, CI account will host a networking deployment and the CI templates will be modified to use the subnets to create a cluster.

### Graduation Criteria

*This enhancement will follow standard graduation criteria.*

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Upgrade / Downgrade Strategy

Not applicable

### Version Skew Strategy

Not applicable

## Implementation History

## Drawbacks

Customer owned networking components means the cluster cannot automatically change things about the networking during upgrade.

## Alternatives

Not applicable

[networking-machinecidr]: https://github.com/openshift/installer/blob/8c9abe40f7616303c03cafdc9ad612cd8fa7bd6b/pkg/types/installconfig.go#L163-L167
[gcp-platform-structure]: https://github.com/openshift/installer/blob/master/pkg/types/gcp/platform.go
[networks-list]: https://cloud.google.com/compute/docs/reference/rest/v1/networks/list
[subnetworks-list]: https://cloud.google.com/compute/docs/reference/rest/v1/networks/list
[upi-flow]: https://github.com/openshift/installer/blob/master/upi/gcp/01_vpc.py
