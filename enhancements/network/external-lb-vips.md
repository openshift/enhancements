---
title: external-lb-vips
authors:
  - "@emilienm"
reviewers: 
  - "@cybertron for SME review on on-premise networking"
  - "@jcpowermac for SME review on installer"
  - "@patrickdillon for SME review and approval on installer"
  - "@JoelSpeed for SME review on API and general feedback"
  - "@deads2k for SME review on API and general feedback"
approvers:
  - "@cybertron"
  - "@patrickdillon"
  - "@JoelSpeed"
api-approvers:
  - "@JoelSpeed"
creation-date: 2023-01-23
last-updated: 2023-02-08
tracking-link:
  - https://issues.redhat.com/browse/OCPBU-156
  - https://issues.redhat.com/browse/OSASINFRA-3069
see-also:
  - "/enhancements/on-prem-service-loadbalancers.md"
replaces:
  - https://github.com/openshift/enhancements/pull/1281
---

# External loadbalancer VIPs

## Summary

Originally the on-premise IPI architecture was designed to deploy an internal
loadbalancer based on HAproxy, Keepalived.
When customers needed more flexibility for the control-plane VIPs management, they would
deploy with the UPI method.

Also, customers have the possibility to use another loadbalancer ([example with OpenStack](https://docs.openshift.com/container-platform/4.12/networking/load-balancing-openstack.html#nw-osp-configuring-external-load-balancer_load-balancing-openstack))
for the control-plane VIPs but OpenShift would still deploy Keepalived and HAproxy in the
control-plane. This can be problematic in some scenarios (if VIPs are not in the same subnet).

This design is proposing the support for using an external loadbalancer
for the API and Ingress VIPs, so this can be managed outside of the cluster,
by the customer.


## Motivation

Combined with `FailureDomains`, it will offer more resiliency by allowing to configure compute and control
plane nodes across multiple subnets for on-premise IPI deployments.
It also offers more flexibility by giving more control to the customer so they can use their own loadbalancer
solution, instead of the built-in that the IPI workflow installs and manages how the VIPs are routed (e.g. via BGP).
It also helps to overcome some performance issues with the self-hosted loadbalancer. when it comes to scalability.

### User Stories

As a deployer of a IPI OpenShift cluster, I want to place my control plane nodes on 3 or
more subnets at installation time. Therefore I need to be able to configure an external load balancer for
the VIPs used on the cluster control plane.

### Goals

Allow to replace the internal loadbalancer by an external one and support this scenario when the customer
prefers to maintain it themselves. Later we provide more details on what is expected from the customer to do.

### Non-Goals

* Making this scenario by default at some point. The internal loadbalancer for on-premise platforms will
  remain the HAproxy/Keepalived/CoreDNS stack.
* Change the default behavior of the VSphere UPI workflow.
* Support the migration from the internal to the external loadbalancer (and vice-versa). This will be done in
  a separate enhancement proposal in the future.
* Support the ability to change the IP addresses of the VIPs on the loadbalancer. This will be done in a
  separate enhancement proposal in the future.
  will have to choose a scenario.

## Proposal

We will make some change in `install-config.yaml` so customers can configure the
deployment to use their own loadbalancer if they want.
Then the components which interact with networking will have to check this
new field and handle this scenario appropriately.

### Workflow Description

1. The customer will configure their external loadbalancer(s), which includes configuring the VIPs
   for API and Ingress and also configure the loadbalancer tool that is being used (e.g. HAproxy, F5, etc)
   Note that some platforms don't support Static IPs yet. When not supported, the deployment will have
   to configure the LB backends with all the IPs that are available in the subnet of the control-plane machines.
   Otherwise, only the IPs of the control-plane machines that will be used need to be configured in the LB backends.
2. The customer will write the `install-config.yaml` file and set the new `loadBalancerType` platform specific field to `UserManaged`, and
   will set the fields for `apiVIP` and `ingressVIP`. Since this is will be TechPreview at first,
   customers will have to set `featureSet: TechPreviewNoUpgrade`.
3. In the installer, if the platform does support external loadbalancer, some validation will be run
   to check that the VIPs can already be resolved (the validation already exists for vsphere, we need to
    make it re-usable). The new field for `LoadBalancerType` will be set in `PlatformStatus`.
4. In machine-config-operator, if LB type is external, the HAproxy and Keepalived will not be
   deployed.

### API Extensions

A new field will be added to the `PlatformStatus` of the infrastructure object to tell
the kind of the loadbalancer.

```golang
// PlatformLoadBalancerType defines the type of load balancer used by the cluster.
type PlatformLoadBalancerType string

const (
	// LoadBalancerTypeUserManaged is a load balancer with control-plane VIPs managed outside of the cluster by the customer.
	LoadBalancerTypeUserManaged PlatformLoadBalancerType = "UserManaged"

	// LoadBalancerTypeOpenShiftManagedDefault is the default load balancer with control-plane VIPs managed by the OpenShift cluster.
	LoadBalancerTypeOpenShiftManagedDefault PlatformLoadBalancerType = "OpenShiftManagedDefault"
)

// VSpherePlatformLoadBalancer defines the load balancer used by the cluster on VSphere platform.
// +union
type VSpherePlatformLoadBalancer struct {
	// type defines the type of load balancer used by the cluster on VSphere platform
	// which can be a user-managed or openshift-managed load balancer
	// that is to be used for the OpenShift API and Ingress endpoints.
	// When set to OpenShiftManagedDefault the static pods in charge of API and Ingress traffic load-balancing
	// defined in the machine config operator will be deployed.
	// When set to UserManaged these static pods will not be deployed and it is expected that
	// the load balancer is configured out of band by the deployer.
	// When omitted, this means no opinion and the platform is left to choose a reasonable default.
	// The default value is OpenShiftManagedDefault.
	// +default="OpenShiftManagedDefault"
	// +kubebuilder:default:="OpenShiftManagedDefault"
	// +kubebuilder:validation:Enum:="OpenShiftManagedDefault";"UserManaged"
	// +kubebuilder:validation:XValidation:rule="oldSelf == '' || self == oldSelf",message="type is immutable once set"
	// +optional
	// +unionDiscriminator
	Type PlatformLoadBalancerType `json:"type,omitempty"`
}

type VSpherePlatformStatus struct {
	// loadBalancer defines how the load balancer used by the cluster is configured.
	// +default={"type": "OpenShiftManagedDefault"}
	// +kubebuilder:default={"type": "OpenShiftManagedDefault"}
	// +openshift:enable:FeatureSets=TechPreviewNoUpgrade
	// +optional
	LoadBalancer *VSpherePlatformLoadBalancer `json:"loadBalancer,omitempty"`
}
```

We decided to put this field in `PlatformStatus`. This is because the `Platform` field will probably have to be validated, which
is common for the infrastructure object. For example, to validate that a transition is valid, or does the cluster accept that change.
We will need the field consumers to consume from `PlatformStatus` rather than `PlatformSpec`.
This will allow the user to express their desire to change the `PlatformSpec`, but the cluster could then reject it thanks to the validation.
So, to allow that option in the future, if we start with a `PlatformStatus` field, we can add the `PlatformSpec` field later.
If we started with a `PlatformSpec` field, moving an unknown number of consumers from `PlatformSpec` to `PlatformStatus` isn't easily possible.
Some of that may seem like it doesn't apply for this particular use-case, but we made that decision to keep the advice consistent for all these
fields that will be added in the future.

This field will be exposed to the user in the `install-config.yaml`:
```yaml
platform:
  vsphere:
    loadBalancer:
      type: UserManaged
featureSet: TechPreviewNoUpgrade
```

If the user wants to be explicit and set the default load balancer which is OpenShift managed:
```yaml
platform:
  vsphere:
    loadBalancer:
      type: OpenShiftManagedDefault
featureSet: TechPreviewNoUpgrade
```

If the user deploys with `featureSet: TechPreviewNoUpgrade` but doesn't set the `loadBalancer` field, the default load balancer will be deployed for backward compatibility.

Note that `loadBalancer` is immutable and once set can't be changed for now.
In the future, we'll be able to add more load balancers and even customize them by adding more options.

The following platforms will have this feature as TechPreview:
* BareMetal
* Nutanix
* OpenStack
* Ovirt
* vSphere

### Implementation Details/Notes/Constraints

* CoreDNS will be deployed if external loadbalancer is being used.
  However, it is not yet supported to change the DNS records for the API and Ingress VIPs in case the
  IPs have changed. This will be supported in the [future](https://issues.redhat.com/browse/RFE-2085).


* We need to make it ready for any platform who wants to support it, and allow
  platforms to turn this on in the installer project. This will be achieved while validating the
  installconfig manifest.

* In the case of a control-plane replacement, the customer has to make sure that
  the loadbalancer backends already have the IP address of the node that will be created during
  the replacement. Since the cluster doesn't host the VIPs nor the loadbalancer, there is nothing
  else to care now.
  However in the future with Static IPs, we'll probably have a new controller for IPAM but this is out
  of scope now.

* For `vsphere` platform, since they're already have a way to disable the internal loadbalancer by
  not specifying the VIPs in `PlatformStatus`, we'll keep this code in place but only for this platform
  so backward compatibility is maintained.

### Risks and Mitigations

Some platforms (vsphere, openstack) have been testing it already and some issues have been identified:
* Having an external LB breaks [nodeip-finder](https://github.com/openshift/baremetal-runtimecfg/blob/master/scripts/nodeip-finder) script.
  This results in kubelet setting an empty node-ip. This in turn means that the CCM (or kubelet in the case of legacy cloud provider) will
  not filter node addresses, and will expose a kubelet endpoint on all local interfaces. This is undesirable and may be a security issue
  in certain cases, for example when nodes are attached to a public-facing dataplane network.
  It's currently under investigation by @mdbooth and our team.
* Validations will have to be implemented to make sure that the customer has prepared their
  environment before the IPI deployment.
* We do not support the update of VIP fields in `PlatformStatus` therefore the external loadbalancer VIPs can never change.
  This will be addressed in the future, but for TechPreview we don't support it yet.

### Drawbacks

This requires more work from the customer to set up the external loadbalancer and the VIPs.
From our perspective, this takes critical infrastructure out of our control. This can complicate support of
such environments since coordination would be required with the owner of the external loadbalancer to debug problems.

## Design Details

### Test Plan

CI will be in charge of testing this feature.
Platforms who want to support this will have their jobs.
Note: vsphere already has a CI job for external loadbalancer. We'll work together to converge our tools.

### Graduation Criteria

The Tech-Preview target is OCP 4.13 and GA would be 4.14.


#### Dev Preview -> Tech Preview

- Ability to use an external loadbalancer from end to end without manual intervention from the customer
  except the documented requirements.
- Customers will pre-populate their loadbalancer backends with all the IPs available in the subnet for the
  control-plane.
- End user documentation, API stability
- Sufficient test coverage
- Gather feedback from customers

#### Tech Preview -> GA

- [Ability to change `apiVIP` and `ingressVIP` on a deployed cluster](https://issues.redhat.com/browse/RFE-2085)
- [Support for Static IPs](https://github.com/openshift/enhancements/pull/1267) would be a must so
  customers don't have to pre-populate their loadbalancer with all IPs in a subnet.
- Migration from internal to external loadbalancer (and vice versa)
- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

#### Removing a deprecated feature

NA

### Upgrade / Downgrade Strategy

An open question about migration from internal loadbalancer to external, but otherwise there should
not be any upgrade impact.

### Version Skew Strategy

NA

### Operational Aspects of API Extensions

NA

#### Failure Modes

NA

#### Support Procedures


## Implementation History

* `vsphere` already doesn't set API fields in `PlatformStatus` for external loadbalancer in IPI/UPI.
   For backward compatibility, we'll keep it supported only on vsphere so their customers aren't broken
   during an update.

## Alternatives

* Implement another internal loadbalancer like it was proposed with [BGP](https://github.com/openshift/enhancements/pull/1281). This idea was rejected for now because it didn't get much traction outside of the OpenStack team. The External LB solution was meeting our customer requirements as was a cross-platform interest so we decided to take that direction for now.
* Change the Keepalived configuration to be active-active, but this will involve complex configuration and still the need
  to have BGP on the same Keepalived nodes, so propagate the VIPs status so BGP can accordingly route the traffic.

## Infrastructure Needed

The customer is responsible for providing the loadbalancer, which can be a container, a virtual machine or a physical server. As long as it's ready prior to the deployment.
