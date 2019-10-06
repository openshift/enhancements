---
title: ovirt-rhv-platform-provider
authors:
  - "@rgolangh"
reviewers:
  - "@deads2k"
  - "@crawford"
  - "@abhinavdahiya"
  - "@sdodson"
approvers:
  - "@deads2k"
creation-date: 22019-10-07
last-updated: 2019-10-07
status: implementable
---

# ovirt-rhv-platform-provider

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions [optional]

## Summary

This document describes how `oVirt` becomes a platform provider for Openshift. \
`oVirt` is a virtualization platform and is similar to the `baremetal` platform \
 provider it is lacking DNS and Load-Balancing services, but it has the advantage \
if software-defining your data-center, utilizing existing hardware and making \ 
that pain free and fast.
Like `baremetal` platform it uses the [OpenShift-Metal³ kni-installer](https://github.com/openshift-metal3/kni-installer) method \
- essentially providing an internal cluster-level DNS service using mDNS and
 coreDNS, and load-balancing using `keepalived`.

Components involved:
- github.com/openshift/api
- github.com/openshift/installer
- github.com/openshift/machine-config-operator
- github.com/openshift/cluster-image-registry-operator (@adambkapln?)
- github.com/ovirt/cluster-api-provider-ovirt
- github.com/ovirt/terraform-provider-ovirt

## Motivation

- It has been defined as a key initiative for 4.3
- The feedback RHV team got is that there is a lot of demand for this 
  kind of installation

### Goals

- provide a way to install Openshift on oVirt infrastructure using 
  the installer - an IPI installation.
- implementing a cluster-api provider to provide scaling and managing the cluster
  nodes (used by IPI, and useful for UPI, and also node management/fencing)

### Non-Goals
- UPI implementation will be provided separately.

## Proposal

This provider enables the Openshift Installer to provision VM resources in an \
oVirt data center, that will be used as worker and masters of the clusters. It \
will also create the bootstrap machine, and the configuration needed to get \
the initial cluster running by supplying DNS a service and load balancing, all \
using static pods.

This work is related to the Bare-Metal provider because oVirt does not supply \
DNS and LB services but is a platform provider. See the [baremetal ipi networking doc][baremetal-ipi-networking]

### Implementation Details/Notes/Constraints [optional]

1. Survey

The installation starts and right after the user supplies his public ssh key,\
and then choose `ovirt` the installation will ask for all the relevant details\
of the installation: **url**, **user** and **password** of ovirt api. The installer \
will validate it can communicate with the api, otherwise it will fail to proceed.\
Next the installer will ask to choose the ovirt **cluster**, where the VMs will be\
created, and next would be a *template* from that cluster for those VMs.\
*Note* that it is the user's responsibility to upload the relevant RHCOS template\
to ovirt. This may change in the future.
[What is a VM Template][template-what] and [how to upload a template from qcow image?][template-upload]\
With that the survey continues to the general cluster name, domain name, and\
the rest of the non-ovirt specific question.

2. Resource creation - terraform

All the information from the survey is serialized by the installer into the file\
`terraform.ovirt.auto.tfvars.json` under the current directory and then it invokes\
terraform, which automatically load `.auto.tfvars.json`. `terraform.tfstate` is\
the actual declaration of the resources and with it terraform creates the VMs.\

3. Bootstrap

The bootstrap VM has a huge Ignition config set using terraform and is visible\
in the `terraform.tfstate` file. oVirt boots that VM with that content and the\
bootstraping begins when the `bootkube.service` systemd service starts.\
The bootstrap job is to:
    1. run a DNS service which will listen on the DNS VIP supplied by the survey\
    to allow internal resolution of the api and resolution of each node's FQDN\
    2. run `keepalived` to activate the DNS VIP and API VIP on the machine nic as\
    secondary IP. 
    3. run and expose machine configs, using a static pod of Machine Config Server\
    listening on the API VIP
    4. load and bootstrap `etcd` service and help sign `etcd` members on the masters\
    and boot statically the rest of the cluster pods.
    5. pivot to a real master after its done

4. Masters bootstrap

Master VMs are booting using a stub Ignition config that are waiting early in\
the Ignition service to load their Ignition config from a URL. That url is the\
 `https://<internal-api-vip>/config/master` which is still not available until\
the **bootstrap** VM is exposing it. It takes few minutes till it does.

With the machine config available the masters pull their Ignition and boots up\
starting few static pods that will allow:
1. Internal DNS service for the cluster
2. `keepalived` to ensure highly available `DNS_VIP` and `API_VIP`
3. The rest of the openshift cluster pods

For the following subjects please refer to ['baremetal IPI networking infrastructure'][baremetal-ipi-networking]:\
 - Load-balanced control plane access
 - API Virtual IP
 - DNS Resolution During Bootstrapping
 - DNS Resolution Post-Install
 - Ingress High Availability

### Risks and Mitigations

- Storage

    Recommended storage setup?

- Small setups and VM affinity

Setups with 2 hypervisors for example will have to disable negative VM affinity \
and run 2 masters VMs together on the same hypervisor. This maybe the case for \
larger amount of hosts but with worker VMs which are expected to be larger in numbers\
depending on the setup. How do we handle that? should we allow that and in \
what cost?

## Design Details

### Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

TODO
  - Announce deprecation and support policy of the existing feature
  - Deprecate the feature

### Upgrade / Downgrade Strategy

TODO
  If applicable, how will the component be upgraded and downgraded? Make sure this
  is in the test plan.


  Consider the following in developing an upgrade/downgrade strategy for this
  enhancement:
  - What changes (in invocations, configurations, API use, etc.) is an existing
    cluster required to make on upgrade in order to keep previous behavior?
  - What changes (in invocations, configurations, API use, etc.) is an existing
    cluster required to make on upgrade in order to make use of the enhancement?

### Version Skew Strategy

TODO - how does cluster-api-provider-ovirt needs to versioned and included?
should it be tagged with the release tag, for example `cluster-api-provider-ovirt:4.3`?

TODO
  What are the guarantees? Make sure this is in the test plan.

  Consider the following in developing a version skew strategy for this
  enhancement:
  - During an upgrade, we will always have skew among components, how will this impact your work?
  - Does this enhancement involve coordinating behavior in the control plane and
    in the kubelet? How does an n-2 kubelet without this feature available behave
    when this feature is used?
  - Will any other components on the node change? For example, changes to CSI, CRI
    or CNI may require updating that component before the kubelet.

## Implementation History

June 12th 2019 - Presented a fully working POC

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

- CI
Running and end-to-end job is a must for this feature to graduate and it is a \
non trivial task. oVirt is not a cloud solution and we need to provide a setup \
for a job invocation. We started with deploying a static oVirt deployment on GCP\
and it is working and able start an installation which is initiated from outside\
of GCP. Now we need to make sure the CI job can do the same.
What's left is to make sure this instance network setup can work with the floating\
IP's we assign to it (the DNS and API VIPS). Currently we assume we can make that\
work because we control the dnsmasq inside VMs network.

What could go wrong?
- we may not be able to make the CI play nicely on time and we need as much help\
  and guidance here. 
- multi ci jobs running in parallel will deploy 4 VMs on the infra, and I don't \
know how will it handle the traffic and disk pressure. My guess is that we should\
minimize the load by not supporting parallel job invocations. Not sure its viable.\


[ovirt-terraform-provider]: https://github.com/ovirt/terraform-provider-ovirt
[baremetal-ipi-networking]: https://github.com/openshift/installer/blob/master/docs/design/baremetal/networking-infrastructure.md
[template-what]: https://www.ovirt.org/documentation/vmm-guide/chap-Templates.html]
[template-upload]: https://github.com/oVirt/ovirt-ansible-image-template/blob/master/README
