---
title: multus service abstraction
authors:
  - "@s1061123"
reviewers:
  - TBD
  - "@dougbtv"
approvers:
  - TBD
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
status: implementable
see-also:
  - "/enhancements/this-other-neat-thing.md"
replaces:
  - "/enhancements/that-less-than-great-idea.md"
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# Multus Service Abstraction

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This propose introduces to implement Kubernetes service in secondaly network interface, which is created by multus CNI. Currently pods' secondary network interfaces made by multus is out of Kubernetes network, hence Kubernetes network functionality cannot be used, such as network policy and service. This proposal introduces several components into OpenShift and try to implement some of the Kubernetes service for pods' secondary network interfaces.

## Motivation

Kubernetes Service object is commonly used abstraction to access Pod workloads. User can define service with label selector and the service provides load-balancing mechanisms to access the pods. This is very useful for usual network services, such as HTTP server. Kubernetes provides various service modes to access, such as ClusterIP, LoadBalancer, headless service, ExternalName and NodePort, not only for inside cluster also for outside of cluster.

These functionality is implemented in various components (e.g. endpoint controller, kube-proxy, iptables/Open vSwitch) and that is the reason we cannot use it for pods' secondary network because pods' secondary network is not under management of Kubernetes.

### Goals

- Provide a mechanism to access to endpoints (by virtual IP address, DNS names), which is organized by Kubernetes service objects, in phased approach.

### Non-Goals

Due to the variety of service types, this must be handled in a phased approach. Therefore this covers the initial implementation and the structural elements, some of which may share commonalities with the future enhancements.

- Provide whole Kubernetes service functionality for secondary network interface.
- Provide completely same semantics for Kubernetes service mechanism
- LoadBalancer features for pods' secondary network interface (this requires more discussion in upstream community)
- NodePort feature (still in discusssion among upstream community)
- Ingress and other functionalities related to Kubernetes service (this will be addressed in another enhancement)
- Network verification for accesssing services (i.e. user need to make sure reachability to the target network)
- Service for universal forwarding mechanisms (e.g DPDK Pods are not supported), we only focus on the pods which uses Linux network stack (i.e. iptables)

## Proposal

### Target Service Functionality

We targets the following service functionality for this proposal:

- ClusterIP
- headless service

#### Service Reachability

The service can be accessed from the pods that can access the target service network. For example, if 'ServiceA' is created for 'net-attach-def1', then 'ServiceA' can be accessed from the pods which has 'net-attach-def1' network interface and we don't provide any gateway/routers for access from outside of network in this proposal.

#### Cluster IP for multus

When the service is created with 'type: ClusterIP', then Kubernetes assign cluster virtual IP for the services. User can access the services with given virtual IP. The request to virtual IP is automatically replaced with actual pods' network interface IP and send to the target services. User needs to make sure reachability to the target network, otherwise the request packet will be dropped.

#### Headless service

Headless service is implemented by CoreOS as DNS round-robin. Hence service can be reachable if pod can reach the IPs of headless service.

### User Stories

- OpenShift developers require the ability to segregate my Kubernetes service data plane traffic onto different networks for reasons of security and performance.
- OpenShift developers require the ability to segregate my Kubernetes service data plane traffic to a secondary interface that is associated with a different CNI plugin to provide functionality not available in the primary (default) CNI plugin.
- OpenShift networking administrators require the ability to provide access to an internally-only-facing corporate network and an externally-facing DMZ network, so application developers can choose which network (pod interface) to use for the different types of traffic that make up their application (e.g. web server:external, DB:internal).
- OpenShift networking administrators require the ability to use one NIC in my hosts for control plane traffic, and additional NICs for data plane traffic.

### Implementation Details/Notes/Constraints [optional]

In this enhancement, we will introduce following components (components name might be changed at development/community discussion):

- Multus Service Controller
- Multus Proxy

Multus service controller watches all Pods' secondary interfaces, services and network attachment definitions and it creates EndpointSlice for the service. EndpointSlice contains secondary network IP address. Multus proxy watches. Service and EndpointSlice have the label, `service.kubernetes.io/service-proxy-name`, which is defined at [kube-proxy APIs](https://pkg.go.dev/k8s.io/kubernetes/pkg/proxy/apis), to make target service out of Kubernetes management.

Multus proxy provides forwarding plane for multus-service with iptables at Pod's network namespace. It watches Service and EndpointSlice with 'service.kubernetes.io/service-proxy-name: multus-proxy' and creates changes destination to the service pods from Service VIP. It does not provide NAT (ip masquerade) for now because multus network is mainly for 'secondary network' and it assumes default route to primary Kubernetes cluster network.

#### Create Service

1. Service is created

User creates Kubernetes service object. At that time, user needs to add following annotation/label:

- label: service.kubernetes.io/service-proxy-name (well-known label, defined in kubernetes)

User need to set label, `service.kubernetes.io/service-proxy-name`, with value, `multus-proxy`. This label specifies the component that takes care of the service. Without this variable, upstream kubernetes (i.e. kube-proxy) will take care of, then kube-proxy create iptables rules. This label should be set to `multus-proxy`. For OpenShift, we should verify that this label is recognized at openshift-sdn as well as ovn-kubernetes. This enhancement should address it in openshift-sdn and ovn-kubernetes repository.

- annotations: k8s.v1.cni.cncf.io/service-network (newly introduced)

`k8s.v1.cni.cncf.io/service-network` specifies which network (i.e. net-attach-def) is the target to exporse the service.
This label specify only one net-attach-def which is in same namespace of the Service/Pods, hence Service/Network Attachment Definition/Pods should be in same namespace. This annotation only allows user to specify one network, hence one Service VIP should be matched with one of service/network mappings.

Then multus service controller will watch the service and pods.

2. Pods are launched and multus service controller creates the endpointslices for the sevice

Once multus service controller find Pods associated the service, then the controller will create EndpointSlices for the service. The EndpointSlices contains Pod's net-attach-def IP address from Pod's network status annotation and the EndpointSlices has same label, 'service.kubernetes.io/service-proxy-name', and its value from corresponding Service to avoid kube-proxy to add iptables rules for Kubernetes Service forwarding plane. When the pods/services is updated/removed, then multus service controller changes/remove the corresponding endpoitslices to track the changes.

3. Multus proxy generates iptables rules for the service in Pod's network namespace

Multus proxy watches the endpointslices and services. Periodically multus proxy generates iptables rules for the pods and put it in pod's network namespace (not host network namespace as kube-proxy does). Multus proxy tracks the changes of service/endpoints, of course, hence multus proxy update/remove iptables rules when service/endpoints are updated/removed.

### Risks and Mitigations

#### Interworking service.kubernetes.io/service-proxy-name among other Kubernetes network:

This feature depends on the well-known label, service.kubernetes.io/service-proxy-name implementation. If some Kubernetes network solution satisfy the following condition, then this feature does not work:

- The SDN solution provides forwarding plane for Kubernetes service (i.e. Cilium, ovn-kubernetes)
- The SDN solution does not recognize well-known label, service.kubernetes.io/service-proxy-name

Hence we should mention which SDN solution support this enhancement in documents.

## Design Details

### Open Questions [optional]

N/A

### Test Plan

For upstream testing, we should address e2e test in upstream somehow, however, upstream test will be done wit `kind`, hence we could cover some of primitive testing. More detailed testing should be done in baremetal CI job or SR-IOV CI job with SR-IOV devices.

### Graduation Criteria

This feature, multus service abstraction, is pretty complecated because additional network (i.e. network attachment definition) depends on customer environment, so each customer has each network and never be same. So we really need customer feedback not only about its quality also about their use-case and their experiences. We should watch customer feedback and carefully think about the current status of the feature and proceed to graduation.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- Make consensus desgin and implementation among upstream communities
- More testing (upgrade, downgrade, scale)
- Sufficient feedback to cover various use-cases
- Available by default
- Conduct load testing
- End to end tests in CI

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A: This feature is not particularly kubernetes-version sensitive.

## Implementation History

- *4.10*: dev preview or tech preview
- *4.XX*: Tech Preview
- *4.XX*: GA

## Drawbacks

- Redesign users' infrastructure (including network) not to use multus (i.e. additional secondary networks)

## Alternatives

- Implement secondary network and service feature in ovn-kubernetes (but of course, it cannot support macvlan and other interface)

## Infrastructure Needed [optional]

<<<<<<< HEAD
### Required for GA

GA requires following testing infrastructure:

- Baremetal OpenShift deployment (for scale testing), with multiple network (macvlan/ipvlan/sr-iov)
- Baremetal OpenShift deployment (for CI e2e testing), with multiple network. It might be possible to implement in equinix metal. Need to work on.
