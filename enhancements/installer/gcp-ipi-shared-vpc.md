---
title: gcp-ipi-shared-vpc
authors:
  - "@jstuever"
reviewers:
  - "@padillon"
approvers:
  - "@padillon"
api-approvers:
  - "@JoelSpeed"
creation-date: 2022-05-24
last-updated: 2022-09-21
tracking-link:
  - "https://issues.redhat.com/browse/CORS-1774"
---

# GCP IPI Shared VPC

## Summary

This enhancement adds the capability to deploy a GCP cluster into a [Shared VPC
(XPN)](https://cloud.google.com/vpc/docs/shared-vpc) configuration using the
Installer Provisioned Infrastructure (IPI) workflow.

## Motivation

The GCP Best Practices include having a separate “Service Project” for each
application, and for those Service Projects to use a Shared VPC (XPN)
configuration. In such a configuration, the VPC, Network, Subnetworks, and
Firewall Rules exist in a different “Host Project”. This enables the
application owners to have significant permissions in the Service Project while
restricting access to the shared networking infrastructure. Several of our
customers are asking for this to be possible from within the IPI workflow. In
addition to this, tooling such as OSD and Hive depend on this workflow.

### User Stories

- As a user, I want the installer to create as many resources as possible in
  the Host Project so I can provide sufficient credentials to maximize the
automation of an install to a shared VPC.
- As a user, I want the installer to use existing resources in the Host Project
  so that I can create infrastructre in advance and minimize the privileges to
the Host Project given to the installer when installing to a shared VPC.

### Goals

- Enable users to deploy GCP clusters with Shared VPC networking using the IPI
  workflow.
- Enable users to provide service account(s) with sufficient permissions in the
  Host Project to enable provisioning the majority of the cluster resources.
- Enable users to alternatively provide pre-provisioned resources to enable
  minimal permission requirements in the Host Project.

### Non-Goals

- Provisioning and/or configuring the VPC, Network, and Subnetworks are to be
  done manually by the user prior to running the installer.
- Provisioning and/or configuring the Service Accounts for the Installer and
  Cluster are to be done manually by the user prior to running the installer.
- Providing solution(s) for manually managing DNS in the Host Project is to be
  solved by another effort.

## Proposal

The installer should be able to install a GCP IPI cluster using Shared VPC
networking by automating the majority of the current GCP UPI workflow. This
requires some resources to be created and configured in advance, such as the
network and subnets. Other resources can be created in advance when the
necessary host project permissions are not desired, such as the firewall rules
and/or DNS configuration. The installer, and the cluster-ingress-operator, will
be updated to enable some of this functionality. They will become capable of
creating and managing resources that exist in a different (Host) project from
the cluster (Service Project). They will also become capable of consuming
pre-existing firewall rules.

### Workflow Description

Prior to executing the installer, the Shared VPC, Network, and Subnets will be
manually created in the Host Project and configured for Shared VPC (XPN)
configuration with the Service Project. A service account will also be created
in the Service Project and granted sufficient permissions in the Host Project.
When minimal permissions are desired in the Host Project, additional resources
will be manually created and configured to include: firewall rules, api compute
address, ingress compute address, and the api, api-int, and \*.apps public
and/or private DNS records.

After all of these resources are created, they will be used in a manually
created install-config.yaml to configure the cluster accordingly. At a minimum,
the credentialsMode will be set to Passthrough, and the network,
computeNetwork, controlPlaneNetwork, and networkProjectID will be configured to
reference the Shared VPC network. When minimial permissions are desired in the
Host Project, the additional parameters will be configured to enable the
cluster to consume them as well.

The installation process can then proceed as any other IPI workflow by running
the create manifests, create ignition, and/or create cluster sub-commands.

### API Extensions

#### api.dns

The current DNSZone struct does not appear to be able to store which project
the zone is in. The ID variable is currently limited to a managedZone ID value.
The cluster-ingress-operator will be updated to also accept a relative resource
name in the following format. This will enable the operator to override the
default project id with the one provided. No functional API changes are
necessary.

projects/{projectid}/managedZones/{zoneid}

#### installconfig.platform.gcp.networkProjectID

A new parameter will be added to the install-config to specify the project id
of the network and subnets. This parameter will then be used where necessary to
validate existing network project resources as well as provision additional
resources using these network resources. The default will be an empty string
and will indicate these resources (should) exist in the default cluster
project.

#### installconfig.platform.gcp.privateDNSZone

A new parameter will be added to the install-config to specify the private DNS
managed zone. This parameter will be used where necessary to validate existing
DNS resources as well as provision additional private DNS records. The default
will be an empty value, which will indicate these resources are to be created
and managed in the cluster project identified by the projectID.

#### installconfig.platform.gcp.publicDNSZone

A new parameter will be added to the install-config to specify the public DNS
managed zone. This parameter will be used where necessary to validate existing
DNS resources as well as provision additional public DNS records. The default
will be an empty value, which will indicate these resources are to be created
and managed in the cluster project identified by the projectID.

#### installconfig.platform.gcp.defaultMachinePlatform.tags

A new parameter will be added to the install-config to specify additional
networking tags to add to the cluster instances. The default will be an empty
list, in which case no additional tags will be added.

#### installconfig.controlPlane.platform.gcp.tags

A new parameter will be added to the install-config to specify additional
networking tags to add to control plane instances. This parameter will override
any tags specified in the defaultMachinePlatform. The default will be an empty
list, in which case no additional tags will be added.

#### installconfig.compute.platform.gcp.tags

A new parameter will be added to the install-config to specify additional
networking tags to add to compute instances. This parameter will override any
tags specified in the defaultMachinePlatform. The default will be an empty
list, in which case no additional tags will be added.

#### installconfig.platform.gcp.createFirewallRules

A new parameter will be added to the install-config to enable toggling of the
creation and management of firewall rules. When set to disabled, the installer
will skip creation of firewall rules. The default is enabled in which firewall
rules will be created as before. This toggle does not modify the behavior of
the cluster-ingress-operator, which is to create firewall rules if permissions
allow.

### Implementation Details/Notes/Constraints

#### VPC, Network, and Subnetworks

Shared VPC network and subnet(s) are to be created and configured in advance.
They can then be configured in the install config using the following
parameters as enabled by prior work. A new parameter (networkProjectID) will be
added to specify in which project they exist. This parameter will be consumed
by the Terraform templates as well as cluster manifests in order to configure
the cluster to use these resources.

- installconfig.platform.gcp.computeSubnet
- installconfig.platform.gcp.controlPlaneSubnet
- installconfig.platform.gcp.network

#### Service Accounts

Service Account(s) are to be created and configured in advance. These accounts
must have additional permissions granted in the Host Project. The specific
permissions depend on the desired level of automation. The install config's
credentialsMode parameter must be set to passthrough so the cluster uses these
accounts.

#### Firewall Rules

Firewall Rules can either be created and configured in advance, or the service
accounts above must have sufficient permissions granted to manage firewall
rules in the Host Project. User specified tags will be added to the instances
to enable cluster specific rules. A new install config parameter will be added
to enable specifying if the Terraform templates should create firewall rules,
or not.

In order for the firewall rules to be created by the installer, the service
account will require the following access in the project where the firewall
rules are to be managed (i.e. networkProjectID)

- Compute Network Admin
- Compute Security Admin

#### DNS Zones and Records

DNS can either be created and configured in advance, or the service accounts
above must have sufficient permissions to create and manage DNS resources in
the Host Project.

The ability to use pre-existing (aka external) DNS solutions is outside the
scope of this enhancement. There are other efforts to provide this capability.

New install config parameters will be added to specify the public and/or
private DNS zones to use when managing cluster records. The default will be an
empty value, which will result in the private zone and public/private records
being managed in the service account as before.

In order for the DNS Zones and Records to be managed by the installer, the
service account will require the following access in the in the project(s)
where the DNS Zones are located.

- DNS Administrator

### Risks and Mitigations

Adding complexity to the Terraform templates may affect the existing IPI
workflows. Our CI and QE processes should catch any issues here.

### Drawbacks

Extending the GCP IPI workflow to enable this functionality will likely require
additional cycles to maintain and test as our product changes over time.

## Design Details

### Test Plan

A CI job will be created to test this functionality, replacing the existing GCP
UPI XPN workflow.

### Graduation Criteria

#### Dev Preview -> Tech Preview

The work necessary to deliver this enhancement can be split up into multiple,
indipendent epics. The majority of these will become Tech Preview as they are
added to the installer.

One exception is the network tags epic, which is immediately useful on it's own
and will be released immediately GA.

#### Tech Preview -> GA

When all of the work is completed and proven effective through Tech Preview,
the whole of this enhancement will move into GA.

#### Removing a deprecated feature

There are no deprecated features.

### Upgrade / Downgrade Strategy

This enhancement should not affect upgrade, as it only affects install time.

### Version Skew Strategy

There is no expected need to manage version skew.

### Operational Aspects of API Extensions

The api extensions are discussed in the proposal section above.

#### Failure Modes

The user provided parameters will be validated, thereby failing early and
reducing failure modes in general.

#### Support Procedures

## Implementation History

## Alternatives
