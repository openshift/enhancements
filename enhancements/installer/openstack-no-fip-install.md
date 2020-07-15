---
title: openstack-bring-your-own-external-connectivity
authors:
  - "@egarcia"
  - "@adduarte"
reviewers:
  - "@mandre"
  - "@pierre"
approvers:
  - "@mandre"
  - "@pierre"
creation-date: 2020-5-26
last-updated: 2020-5-26
status: implementable
---

# OpenStack BYO External Connectivity

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

Numerous customers have requested the ability to run the OpenShift on OpenStack IPI installer without using Floating IP addresses.
In the installer, we use floating IPs as a way to set up external connectivity to the OpenShift cluster. Depending on how the OpenStack cluster
is set up, the use of floating IPs may be impossible.

## Motivation

### Goals

- Extend the installer to support provider networks
- Allow the installer to be run without setting up external connectivity

### Non-Goals

This enhancement does not intend to provide any guidance or features that go further than allowing customers to deploy without Floating IPs. Operations like attaching loadbalancers to the entrypoints, or any day 2 operations are beyond the scope of this enhancement. It is important to note that while we aim to support installs without external connectivity, we are not supporting installs on networks that are unable to reach the host cluster's OpenStack APIs and services.

## Proposal

In order to implement this feature fully, the following changes must be made:
1. Make `externalNetwork` an optional argument in the install-config when `machinesSubnet` is set
2. Make setting  `lbFloatingIP` when no external network is passed enforced as invalid usage

### Design Details

#### Make `externalNetwork` an optional argument in the install-config when `machinesSubnet` is set

Implementing this can be done by changing the validations and the defaults so that a blank external network can be passed to the installer code. We would also disable all floating IP creation in the installer when this is the case.

#### Disable `lbFloatingIP` when no external network is passed

In the installer, the external network is used to provision floating IPs as well as the router. The router no longer gets created when using `machinesSubnet`, so the only changes that would need to be made are changes in the installers defaults and validations that allow the a blank `lbFloatingIP` to be pased. Then we will have to make a conditional block that does not attach the floating IP when `lbFloatingIP` is blank.

### User Stories

#### Provider Networks

As an enterprise OpenStack cluster administrator, I want to use the IPI installer but my cluster does not support Floating IPs. I need a way to install the OpenShift IPI installer on provider networks instead. I may also need to clear IP addresses I use on the provider network through an internal audit.

##### Example Usage

Using the proposed feature, the user would not set `externalNetwork` or `lbFloatingIP`. Then, they will set`machinesSubnet` to the ID of an existing subnet on a provider network that they want to install the cluster onto, and set the `networking.machineNetwork` to the CIDR of that subnet. They will also set custom ports IPs for the `apiVP` and `ingressVIP` to ensure the IP they take is available. Here is an example install config:

```yaml
apiVersion: v1
baseDomain: example.com
metadata:
  name: test-cluster
networking:
  machineNetwork:
  - cidr: 10.0.1.0/17
platform:
  openstack:
    cloud: mycloud
    computeFlavor: m1.s2.xlarge
    machinesSubnet: fa806b2f-abcd-4bce-b9db-124bc64209bf
    apiVIP: `10.0.1.19`
    ingressVIP: `10.0.1.20`
pullSecret: '{"auths": ...}'
sshKey: ssh-ed25519 AAAA...
```

#### No Public External Access

As an administrator with an on-prem OpenStack cluster, I want to install OpenShift in a way that does not expose API and Ingress endpoints to an external network. Instead, I would prefer to attach my own loadbalancers to the cluster entrypoints post install.


### Test Plan   

Unit tests and validations for this feature will be added to the installer to make sure that correct usage is enforced and that this feature does not hinder the usage of other features. To ensure GA readiness, it will be vetted by the QE team as well to make sure that it works with the followin use cases: self-signed certs, customer-provided networks, baremetal workers, scale-out, upgrades, and a restricted-network install.

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
- E2E testing is not necessary for this feature

### Infrastructure Needs

- OpenStack cluster with provider networks
  - PSI

### Drawbacks

We have to be very careful about how we implement this. Making features that were previously required optional may confuse users.
We also need to be certian that users will not be able to mistakenly install a cluster that was intended to have external connectivity without it.
In order to guard against this, we intend to improved the documentation and validations.

#### Upgrade/Downgrade strategy

This feature will be released in 4.6, and will not be backported. We will also not plan to support migration of existing clusters to support this feature, since the changes are install time only.
