---
title: Make Service NodePort Range Configurable
authors:
  - "@abhat"
reviewers:
  - "@dcbw"
  - "@deads2k"
  - "@smarterclayton"
  - "@danwinship"
  - "@knobunc"
approvers:
  - "@knobunc"
creation-date: 2020-07-06
last-updated: 2020-07-06
status: implementable
---
# Configurability of Service NodePort Range
This enhancement targets making the service node-port range of an Openshift cluster configurable.

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Service node-port range is a [configurable parameter](https://github.com/openshift/cluster-kube-apiserver-operator/blob/master/bindata/v4.1.0/config/defaultconfig.yaml#L147)
on the Kubernetes API server. As our customers scale the number of node-port services in their Openshift clusters,
there is a desire to make this node port range configurable.

## Motivation

Service node-port range is a configurable parameter consumed by the Kubernetes API server as
a slice of port ranges. It defaults currently to a single port range (30000-32767).
It is consumed in Openshift via the ConfigMap resource that holds all the api-server configuration.
In order to allow customers to scale the number of node-port services in the cluster to a value greater
than that allowed by the default service node-port range, it is important to allow dynamically changing this
range to allow for more ports. This enhancement focuses on the mechanics of allowing users to configure a
custom service node-port range for their clusters.

### Goals

- Customers can configure the service node-port range for their cluster during install.
- Customers can configure the service node-port range for their cluster post install as their needs change.

### Non-Goals

- Services launched prior to customers changing the service node-port range will not work if the new range is
  not inclusive of the old range.
- This enhancement makes no guarantees of providing non-disruptive change to the service node-port range.
  In other words customers, when they make non-inclusive changes to the service node-port range are responsible
  for deleting old services and recreating new services that will then get node ports assigned from the
  new range.
- Configuration of multiple service node-port ranges even though supported by the Kubernetes API server will not be
  allowed. Users are responsible for configuring the range to be large enough for their needs.
  
## Proposal

The config observation facility in cluster-kube-apiserver-operator will be used to observe the openshift/api's network 
configuration object for changes to the service node port range. The config observation controller will then apply the
change to the `apiServerArguments.service-node-port-range` field. There will only be one entry allowed in the slice.

The following changes are proposed and implemented:

- API changes: [PR #660](https://github.com/openshift/api/pull/660)
- Library-go changes: [PR #826](https://github.com/openshift/library-go/pull/826)
- cluster-kube-apiserver-operator changes: [PR #894](https://github.com/openshift/cluster-kube-apiserver-operator/pull/894)

Additionally, to realize the configuration of service node port range, the necessary ports need to be opened on the 
individual nodes. Traditionally, this has been the responsibility of the Openshift Installer. But since we want this
configuration to be changed post-install, we need some mechanism to allow for a new set of ports to be opened by _an_
_entity_ that knows how to tweak the infrastructure.

### Implementation Details/Notes/Constraints

#### Early CI

#### Upstream Kubernetes

This feature is already supported in upstream Kubernetes.

#### Installer

The installer has been traditionally responsible to [open the ports](https://github.com/openshift/installer/blob/master/data/data/aws/vpc/sg-worker.tf#L146)
belonging to the service node-port range on individual platforms. That functionality will need to be handled via some
other entity that can also monitor for changes to the service node-port range and open up the necessary ports while the cluster 
is up and running.

### Risks and Mitigations

#### Completion Risks

The mechanism to handle the configuration and opening of ports on different infrastructure platforms is still TBD. 
That poses a risk for getting this feature completed in 4.6 timeframe. If we can come to a consensus on how we will
configure the ports on different infrastructure platforms with minimum re-work either in CNO or some other component,
we may be able to shrink the timeline for delivering this.

#### Functional Risks

The design for opening up ports on the infrastructure in response to the changes to the `cluster` network configuration object
needs to be fleshed out. This feature is at risk primarily due to the lack of clarity on that front.

## Design Details

### Test Plan

New e2e tests will have to be added to verify this functionality. Unit-tests
have been added to the necessary PRs that have already been posted as mentioned
in the [proposal section](##Proposal). Likewise, QE will have to add new test-cases to
verify functionality conforming to the design goals.

### Graduation Criteria

##### Dev Preview -> Tech Preview

##### Tech Preview -> GA 

### Upgrade / Downgrade Strategy

### Version Skew Strategy

## Implementation History
