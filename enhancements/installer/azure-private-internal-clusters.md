---
title: Azure: Internal/Private Clusters
authors:
  - "@jhixson"
reviewers:
  - "@abhinavdahiya"
  - "@sdodson"
approvers:
  - "@abhinavdahiya"
  - "@sdodson"
creation-date: 2019-09-10
last-updated: 2019-10-15
status:
    implemented

---

# azure-internal-private-clusters

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

- Many of our customers require their cluster has entry points which are only
"internal"; requirement for many disconnected environments without direct
Internet connectivity in the VPC where OCP is installed
- Nearly 30% of OSD clusters are internally customer-facing with VPN connectivity
back into their corporate networks.
- Many of our OCP 3.11 customers prefer to deploy their clusters this way
especially for disconnected use cases where they may not always have external
connectivity.
- In addition to AWS, we will also need to support this for our other public
cloud providers.

## Motivation

### Goals

Install an OpenShift cluster on Azure as internal/private, which is only
accessible from my internal network and not visible to the Internet.

### Non-Goals

- No day 2 change: a private cluster cannot become public and a public cluster
cannot be made private
- No additional isolation from other clusters guarantees in addition to the ones
provided by shared VPC

## Proposal

- The bootstrap host will not get a public IP for SSH access
- The gather bootstrap functionality will need to be updated to use the private
IP
- This affects the default ingress setup for the cluster to be internal too

### Details

#### Assumptions made by the installer

- The installer will no longer require the user to provide the
BaseDomainResourceGroup as no public records need to be created for the
cluster.
- The cluster will still require access to the internet.

#### Resources provided to the installer

- Internal/Private clusters will make use of a pre-exisiting virtual network
- No new resources should be necessary

#### Installer Resources

##### The installer will no longer create

- Public IP addresses
- Public DNS records
- Public load balancers
- Public endpoints

##### The installer will continue to create

- Private IP addresses
- Private DNS records
- Private load balancers

#### API

#### Install Config

Internal vs external clusters will be determined based on the new field in
install-config as described in the AWS internal/private clusters design doc.

#### Gather bootstrap

The BootstrapIP function currently uses the
`azurerm_public_ip.bootstrap_public_ip` resource as the IP for the
bootstrap-host. But for internal clusters, the function will have to use the
`azurerm_network_interface.bootstrap.private_ip_address` attribute.

Using the private IP whenever the public IP is not available should be
acceptable because all such cases will only be when the cluster is Private.

#### Isolation between clusters

Since we are installing to internal/private networks, isolation will not be an
issue.

#### Known limitations

- We can’t attach or manipulate network security groups to an existing subnet since it’s not in our control. This means we can’t isolate between clusters also.
- We can’t attach DNS records to the existing VNet, so we will need to use the new Azure private DNS zones.

#### Destroy

There should be no work required. For Azure, we filter on resource group. As a
result, destroy should only delete resources that are in the resource group.

#### CI

CI jobs should setup a private VNet, routing, subnets, and then run the install
using this configuration.
