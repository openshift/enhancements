---
title: vsphere-ipi-zonal-external-lb
authors:
  - "@jcpowermac"
reviewers:
  - "@rvanderp3"
  - "@bostrt"
approvers:
  - "@rvanderp3"
  - "@patrickdillon"
api-approvers:
  - "@JoelSpeed"
  - "@deads2k"
creation-date: 2022-11-02
last-updated: 2022-11-03
status: implementable
see-also:
  - "/enhancements/installer/vsphere-ipi-zonal.md"
replaces:
  - None
superseded-by:
  - None
tracking-link:
  - https://issues.redhat.com/browse/OCPPLAN-9652

---

# Support External Load Balancer in vSphere Zonal IPI

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The goal of this enhancement is to provide the ability to install vSphere IPI zonal on separate L2 segments without the static pods (keepalived, haproxy, coredns). 
As of this writing the only way to accomplish this is to allow an external load balancer.

## Motivation

Users of OpenShift would like the ability to deploy control plane and compute nodes in more than one L2 segment to increase reliability and ease of network management.

### Goals

- Support external load balancer in vSphere IPI zonal
- Support multiple subnets via the external load balancer
- Support for no static pod deployment (keepalived, haproxy, coredns)

### Non-Goals

- Provide a mechanism to configure a external load balancer from the installer or machine-api-operator 

## Proposal

We propose the use of small L2 segments (e.g. /28). The customer's
load balancer will be pre-configured with the entire subnet as end points
for the required ports. Those L2 segments will be a associated with vSphere port groups that will be used in the `FailureDomains` and provisioning process.  The external load balancer will discover available endpoints and use those accordingly.

- Modification to the Infrastructure status object to include new LoadBalancerType
- Modification to the platform spec object to include new LoadBalancerType
- Modification to the machine-config-operator to use new LoadBalancerType 

### Workflow Description

### api

#### Infrastructure Status

```golang
// LoadBalancerType can be either External or Internal 
type LoadBalancerType string

const (
	// LoadBalancerTypeExternal
	LoadBalancerTypeExternal LoadBalancerType = "External"

	// LoadBalancerTypeInternal
	LoadBalancerTypeInternal LoadBalancerType = "Internal"
)

// VSpherePlatformStatus holds the current status of the vSphere infrastructure provider.
type VSpherePlatformStatus struct {
  // loadBalancerType defines if a internal or external load balancer
  // is to be used for the openshift api and ingress endpoints.
  // When set to Internal the static pods defined in machine config operator
  // will be used.
  // When set to External the static pods will not be deployed. It will be 
  // expected that the LB and DNS A records be pre-configured prior to installation.
  // +kubebuilder:default=External
  // +kubebuilder:validation:Enum=Internal;External
  LoadBalancer LoadBalancerType `json:"loadBalancerType,omitempty"`
}
```

### MCO

The existing switch to deploy keepalived, haproxy and coredns static pods in the machine config operator is the apivip variable. This would change to be the Infrastructure status parameter: `LoadBalancerType`.

### installer

The installer will need to modified to support additional validation of 
`LoadBalancerType` and if the VIPs should be defined.
In addition the infrastructure spec status will need to updated.

With the load balancer being external and the api, api-int and wildcard ingress
A records required prior to installation the MCS url can be changed to the URL.

The installer documentation will need to be updated for the `External` requirements including the machine network cidr(s).

#### Platform Spec

The platform spec needs to be modified to support our initial goals

```golang
// LoadBalancerType can be either External or Internal 
type LoadBalancerType string

const (
	//LoadBalancerTypeExternal
	LoadBalancerTypeExternal LoadBalancerType = "External"

	// LoadBalancerTypeInternal
	LoadBalancerTypeInternal LoadBalancerType = "Internal"
)

// Platform stores any global configuration used for vsphere platforms
type Platform struct {
  // loadBalancerType defines if a internal or external load balancer
  // is to be used for the openshift api and ingress endpoints.
  // When set to Internal the static pods defined in machine config operator
  // will be used.
  // When set to External the static pods will not be deployed. It will be 
  // expected that the LB and DNS A records be pre-configured prior to installation.
  // +kubebuilder:default=External
  // +kubebuilder:validation:Enum=Internal;External
	LoadBalancer LoadBalancerType `json:"loadBalancerType,omitempty"`
}
```

### User Stories

- https://issues.redhat.com/browse/OCPPLAN-9652
- https://issues.redhat.com/browse/SPLAT-860

### API Extensions

### Risks and Mitigations

- Customer acceptance of design

## Design Details

### Open Questions

### Test Plan

- Develop release automation to build a virtual machine that can act like the LB
- Create a haproxy instance with dataplaneapi installed and operational
- Create backend servers via dataplaneapi
- Using the vSphere IPI zonal installation create FailureDomains with 
individual port groups (`network`)

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

### Drawbacks

- External load balancer is not configured by either the installer or machine api.
- There is a 1:1 relationship between a cluster and subnets. In other words the subnets defined for one cluster absolutely cannot be used for another OCP cluster.

## Alternatives

- A mechanism that has plugin functionality to update a load balancer configuration based on IP addressing of RHCOS-based virtual machines. This would need to run at installation time and whenever a new compute node is provisioned.

## Infrastructure Needed

- External load balancer, this most likely will end up being haproxy.
