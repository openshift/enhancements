---
title: openstack-upi
authors:
  - "@tomassedovic"
reviewers:
  - "@mandre"
  - "@luis5tb"
  - "@wking"
  - "@cuppett"
  - "@iamemilio"
approvers:
  - "@sdodson"
  - "@crawford"
creation-date: 2019-09-10
last-updated: 2019-12-10
status: implementable
---

# OpenStack UPI

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria
- [x] User-facing documentation is created in [openshift/docs]

## Summary

The initial OpenStack support for OpenShift 4 was around the
installer-provisioned (IPI) workflow. While this is the most
convenient approach for the end-users of OpenStack, there are
situations where it falls short. The goal of IPI is to create the
infrastructure elements (network, VMs) needed for installing
OpenShift. In order to reuse pre-existing pieces of infrastructure
instead, users have to either adapt the [bare metal][baremetal-upi]
flow or translate concepts from the AWS documentation.

Similar to AWS and GCP, we want to provide documentation and tooling
for OpenStack specifically -- using what OpenStack users are familiar
with, and highlighting considerations specific to that platform.

## Motivation

In addition to the general use cases supported by UPI (e.g. control
over the creation of the resources of the cloud provider, custom
Ignition configuration), there are additional reasons for having an
OpenStack-specific UPI.

First, unlike for completely hosted solutions such as AWS or GCP there
is no single OpenStack.

Some deployments are public clouds akin to AWS while others are only
available within a single organisation, possibly disconnected from the
internet. Moreover, the actual services and configuration can vary
greatly between deployments.

The OpenStack UPI should be able to support scenarios such as these:

* Lack of Swift Object Storage (which we use for serving the bootstrap
  Ignition config as well as the registry storage)
* No floating IP addresses (e.g. in deployments with provider
  networks)
* Disconnected from the internet
* Desire to integrate all traffic with a load balancer or DNS that
  already exists within the organisation running the OpenStack

### Goals

The goal of user-provisioned is to give users more freedom compared to
installer-provisioned installation.

However, this enhancement is deemed complete once the Installer codebase and
documentation defines manual steps for achieving the equivalent of a regular
IPI installation. Further adaptations can and should be added at a later stage,
on the basis of this work.

* OpenStack UPI documentation available under:
https://github.com/openshift/installer/blob/master/docs/user/openstack/install_upi.md
* Ansible playbooks for automating the OpenStack resource creation
available under:
https://github.com/openshift/installer/tree/master/upi/openstack
* CI job executing these playbooks running wherever the AWS and GCP
  jobs run
* Optional documentation and playbook sections for Kuryr

### Non-Goals

It is not a goal to detail install steps and provide tooling for each and every
possible hardware and configuration permutation.

In particular, it is not the goal of the initial implementation to support:
* Octavia as load balancer
* Designate as DNS
* custom storage integration

## Proposal

This is where we get down to the nitty gritty of what the proposal
actually is.

* Write the Ansible playbook that automate the OpenStack resource
  creation (networks, subnets, ports, servers, etc.)
* Add the Ansible dependencies to the OpenStack CI image
* Write the UPI document, linking the Ansible templates
* Create an UPI job in the [Release repository][openshift-release]
* Add Kuryr documentation and playbooks as an optional SDN alternative

### User Stories

#### Deployment without Swift

In OpenStack systems without Swift and the `tempurl` support, the IPI
deployment will fail when it tries to upload the bootstrap ignition
file to the object store. We can use UPI to generate the Ignition
configs separately, uploading them to a different location and
configuring the servers that way.

The person following the UPI process could upload the bootstrap
ignition into any other location and specify that URL when booting up
the bootstrap node:

1. Create `install-config.yaml`, `manifests` and `ignition-configs`
1. Upload the bootstrap Ignition config file to a location accessible
   by the OpenStack servers you are going to create
   * This can be an internal object storage service, a local HTTP server, etc.
1. Run the UPI Ansible playbooks that create the networking, security
   groups and create OpenStack servers following the UPI documentation
1. After all the servers boot up, feel free to remove the Ignition
   configs


#### Deployment without Floating IP addresses

If the OpenStack cloud uses [provider networks][provider-networks] any
server (VM) created is already accessible via its fixed IP address and
tenant networks, subnets and floating IP addresses might not even be
available.

The IPI installation would fail trying to create these.

A UPI process would get around this by simply not creating these and
relying on the fact that everything is networked already:

1. Create `install-config.yaml`, `manifests` and `ignition-configs`
1. Modify the Ansible playbooks to not create tenant networks, subnets
   or floating IP addresses
1. Upload the boostrap Ignition file to Swift or any other HTTP
   storage available to the servers
1. Run the modified Ansible playbooks


### Implementation Details/Notes/Constraints

#### Scope

The UPI implementation covered here will mirror the existing OpenStack
IPI deployment (with and without Kuryr). The initial goal is to
approach feature parity with IPI.

It will point out places where the OpenShift administrator can specify
their custom configuration, but any specific steps, customisation or
automation not supported by IPI are out of scope for this effort.

For the initial implementation, the only network topology being tested
will be a typical vxlan-based tenant network setup. In other words,
OpenStack end-users will create their own Neutron networks and subnets
and provide external access via Neutron routers and Floating IP
addresses.

Any other topologies (explicit spine and leaf, flat provider networks,
etc.) will not be described or tested initially.

#### Automation

The automation for the OpenStack resource creation (networks, subnets
, ports, servers, security group etc.) will be provided via Ansible
playbooks.

The AWS UPI uses CloudFormation. While OpenStack has a similar project
(called Heat), a lot of existing OpenStack deployments do not run it.
It needs to be set up by the OpenStack operators and if it's not
available, there's nothing the end-user can do about it.

Ansible is a tool system administrators already tend to be familiar
with and it does not depend on any projects or code running inside of
OpenStack itself.

#### Kuryr

Just like in OpenStack IPI, Kuryr is an SDN that can be optionally
used to improve the networking performance of pod to pod traffic.

It is something the OpenShift deployer needs to opt into, but it is in
scope for this enhancement.

We will provide Ansible templates to create the extra resources Kuryr
requires, document the steps necessary to enable it as well as the
extra dependencies and quota requirements.

### Risks and Mitigations

The UPI work will mirror the IPI process closely, just with
documentation and Ansible instead of fully-automated process done by
the installer. As such, we do not anticipate any specific risks or
mitigations connected to the UPI process itself.

Running it in the CI does pose additional risks, however.

The OpenStack cloud which hosts the CI has a limited capacity
available to us. Adding additional jobs can run over that capacity and
might require increasing the quota (which is something the OpenStack
team was granted in the past, but we cannot take it for granted).

This will be exacerbated by the potential addition of Kuryr CI (IPI
and UPI) as Kuryr creates vastly more networks, subnets as well as a
number of load balancers.

In addition, any future work that aims to extend the UPI by e.g.
integrating with OpenStack configurations or projects might require
additional quota or features that the current OpenStack CI provider
does not provide.

## Design Details

### Test Plan

In general, the testing strategy should follow the existing UPI
platforms (e.g. AWS, GCP):

- There will be a new e2e job exercising the UPI Ansible templates
- This proposal does not expect making changes to the existing
  codebase so there is no expectation of unit tests. If we do end up
  making changes, we will add unit tests as appropriate.

### Graduation Criteria

The UPI process in general is well-understood and the OpenStack
platform is not proposing to do anything special. As such, we propose
to `GA` when CI end to end jobs exist and feedback is positive from
the CI, QE and people trying it.

More specifically:

- The UPI document is published in an OpenShift repository
- The OpenStack developers have successfully deployed an UPI OpenShift
  following the document
- The UPI document and Ansible playbooks exist
- They have both been validated by more than one developer
- The deployment has been verified on more than one OpenStack cloud
- The CI jobs exist and are being exercised regularly
- The end to end jobs are stable and passing with the required rate
  (same as the IPI jobs)
- The UPI deployment process has been followed by people outside of
  the development team

We intend to target General Availability in the 4.4 release.

### Upgrade Strategy

Upgrades that will not require changes to the underlying topology of
the OpenStack resources should rely on standard OpenShift upgrade
mechanisms.

If a new OpenShift release does require changes to the OpenStack
resources (e.g. a new Neutron subnet, additional ports, etc.) these
changes will have to be added into the UPI document and OpenShift
errata.

We will attempt to provide adequate upgrade testing in the CI
(focusing form the one-before-last to the latest upgrade), but this
might be limited by the availability of the CI resources.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in
`Implementation History`.

## Drawbacks

Implementing this feature requires development and testing resources
that could otherwise be utilised elsewhere. In addition, it increases
the support surface of the OpenShift and OpenStack integration.

## Alternatives

### Don't Do It

People whose use-cases are not covered by OpenStack IPI can follow the
Bare Metal UPI document. The drawback of this approach is the extra
work on part of the person deploying OpenShift.

They will need to figure the OpenStack-specific parts on their own and
there will be no automation in place to help them out.

### Extend the OpenStack IPI support

We could identify the most common use cases that drive OpenStack users
towards UPI and add them to the IPI installer.

This would likely increase the amount of configuration options we need
to support (going against the IPI spirit of using only the absolute
minimum configuration necessary).

In addition, this approach can be taken in addition to the UPI work --
by providing the UPI documentation and scripts we can look for common
usage patterns and consider including them in the IPI installer on a
case by case basis.


## Infrastructure Needed

The OpenStack developers have all the infrastructure they need right
now. The same OpenStack clouds used for the IPI development can be
used for UPI as well.

Depending on the CI resource usage, they might need to add additional
quota. In addition, any subsequent integrations of additional
projects, storage solutions etc. will require infrastructure that
supports them -- both for dev and CI.

[baremetal-upi]: https://github.com/openshift/installer/blob/master/docs/user/metal/install_upi.md
[openshift-release]: https://github.com/openshift/release/
[provider-networks]: https://docs.openstack.org/networking-ovn/queens/admin/refarch/provider-networks.html
