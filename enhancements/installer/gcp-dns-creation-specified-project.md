---
title: gcp-dns-creation-specified-project
authors:
  - "@barbacbd"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@joelspeed"
  - "@patrickdillon"
  - "@sadasu"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@patrickdillon"
  - "@barbacbd"
  - "@sadasu"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@joelspeed"
creation-date: 2025-06-16
last-updated: 2025-06-16
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/CORS-4012"
superseded-by:
  - "/enhancements/installer/gcp-ipi-shared-vpc.md"
---
# GCP DNS Creation Specified Project

## Summary

The enhancement adds the capability to deploy a GCP XPN ([shared vpc](https://cloud.google.com/vpc/docs/shared-vpc)) cluster where the DNS management can be in a
separate service project following Google's suggested architecture. The enhancement adds the capability to specify
a service project (for the main installation), host project (for network resources), and a [possible] third project 
specifically for DNS zones and records.

## Motivation

The GCP suggested architecture states that placing DNS management in a separate (third) project is recommended for
improved security, organization, and management of DNS resources.

Dedicating a separate project to DNS establishes a centralized location for managing all DNS zones and records. The
separation of resources also increases security by limiting access to DNS resources. Granular permissions can be
granted to those that are required to work with DNS resources. Separating these resources to another project _could_ 
promote lower spending through lower project usages. 

### User Stories

- As a user I want to deploy OCP on GCP XPN within 3 projects; a host project, a service
  project to deploy openshift, and a service project where DNS zones can be managed.

### Goals

- Enable users to deploy GCP clusters with Shared VPC networking using the IPI
  workflow, where users can specify a third project where DNS will be managed.
- Enable users to deploy GCP clusters with Shared VPC networking using the UPI
  workflow, where users can specify a third project where DNS will be managed.
- Enable users to alternatively provide pre-provisioned resources to enable
  minimal permission requirements in the Service Project for DNS management. This
  includes the names of the private DNS Zone.

### Non-Goals

- Provisioning and/or configuring the VPC, Network, and Subnetworks are to be
  done manually by the user prior to running the installer.
- Provisioning and/or configuring the public DNS zone. When the user specifies the
  project for the private DNS Zone, the public zone must exist in the service project
  with the correct base domain.
- Provisioning and/or configuring the Service Accounts for the Installer and
  Cluster are to be done manually by the user prior to running the installer.

## Proposal

The installer should be able to install a GCP IPI cluster using Shared VPC
networking with a specified project and zone for private dns zone management
by automating the majority of the current GCP UPI workflow. This
requires some resources to be created and configured in advance, such as the
DNS public and private zones. The installer, and the cluster-ingress-operator, will
be updated to enable some of this functionality. They will become capable of
creating and managing resources that exist in a different (Service) project from
the cluster (Service and/or Host Project).

### Workflow Description

Prior to executing the installer, the Shared VPC, Network, and Subnets will be
manually created in the Host Project and configured for Shared VPC (XPN)
configuration with the Service Project. A service account will also be created
in the Service Project and granted sufficient permissions in the Host Project.
The DNS zones _may_ be created in a separate Service Project. When a private DNS
zone is specified in the installconfig file, the public zone must exist in that same
project with the base domain found in the installconfig file. The private DNS zone 
provided by the user does **not** have to exist prior to creation, the specified name
will be used during installation, and if the zone does not exist one will be created 
using the matching name. A service account will also be created in the Second Service 
Project and granted sufficient permissions in the Host project.

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

#### installconfig.platform.gcp.dns.privateZone

A new parameter will be added to the install-config to specify the private DNS
managed zone. This parameter will be used where necessary to validate existing
DNS resources as well as provision additional private DNS records. The default
will be an empty value, which will indicate these resources are to be created
and managed in the cluster project identified by the projectID.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Currently, there are no specific considerations for this feature regarding Hypershift / Hosted Control Planes.

#### Standalone Clusters

Currently, there are no specific considerations for this feature regarding Standalone Clusters.

#### Single-node Deployments or MicroShift

Currently, there are no specific considerations for this feature regarding Single-node Deployments or MicroShift.

### Implementation Details/Notes/Constraints

#### Service Accounts

Service Account(s) are to be created and configured in advance. These accounts
must have additional permissions granted in the Second Service Project. The specific
permissions depend on the desired level of automation. The install config's
credentialsMode parameter must be set to passthrough so the cluster uses these
accounts.

#### DNS Zones and Records

DNS can either be created and configured in advance, or the service accounts
above must have sufficient permissions to create and manage DNS resources in
the Specified Second Service Project. The required permissions are listed below.

The ability to use pre-existing (aka external) DNS solutions is outside the
scope of this enhancement. There are other efforts to provide this capability.

New install config parameters will be added to specify the private DNS zone to 
use when managing cluster records. The default will be an
empty value, which will result in the private zone and public/private records
being managed in the service account as before.

In order for the DNS Zones and Records to be managed by the installer, the
service account will require the following access in the project(s)
where the DNS Zones are located.

The following is a list of the minimal required permissions for accessing DNS zones and records in a separate project:
- `dns.managedZones.get`
- `dns.managedZones.list`
- `dns.resourceRecordSets.get`
- `dns.resourceRecordSets.list`


The following is a list of the minimal required permissions for creating DNS zones and records in a separate project:
- `dns.managedZones.create`
- `dns.resourceRecordSets.create`


The following is a list of minimal required missions for deleting DNS zones and records in a separate project:
- `dns.managedZones.delete`
- `dns.resourceRecordSets.delete`


### Risks and Mitigations

Currently, there are no specific considerations for this feature regarding Risks and Mitigations.

### Drawbacks

Extending the GCP IPI workflow to enable this functionality will likely require
additional cycles to maintain and test as our product changes over time.

## Open Questions [optional]

## Test Plan

A CI job will be created to test this functionality, adding to the existing GCP
IPI XPN workflow.

## Graduation Criteria

### Dev Preview -> Tech Preview

The work will go through a spike that will determine the best way forward. It will then be
added as a new feature to API and give a Tech Preview blocker that will affect all work in the
installer and ingress operator. 

### Tech Preview -> GA

When all the work is completed and proven effective through Tech Preview,
the whole of this enhancement will move into GA.

### Removing a deprecated feature

There are no deprecated features.

## Upgrade / Downgrade Strategy

This enhancement should not affect upgrade, as it only affects install time.

## Version Skew Strategy

There is no expected need to manage version skew.

## Operational Aspects of API Extensions

The api extensions are discussed in the proposal section above.

## Support Procedures

## Alternatives (Not Implemented)

Currently, there are no alternatives to the current solution and/or procedures.

## Infrastructure Needed [optional]

