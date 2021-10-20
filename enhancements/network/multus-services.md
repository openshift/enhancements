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

This proposal introduces an implemention of Kubernetes services for secondary network interfaces, which are created
by multus CNI. Currently pods' secondary network interfaces made by multus a sidecar to the Kubernetes network,
henceforth Kubernetes network functionality cannot be used, such as network policy and service. This proposal
introduces several components into OpenShift in order to implement some of the Kubernetes service functionality
for pods' secondary network interfaces.

## Motivation

The Kubernetes Service object is a commonly used abstraction to access Pod workloads. Users can define services with
label selectors and the service provides load-balancing mechanisms to access the pods. This is very useful for
typical network services, such as HTTP servers. Kubernetes provides various service modes to access, such as
ClusterIP, LoadBalancer, headless service, ExternalName and NodePort, not only for inside cluster as well as
outside of clusters.

These functionality is implemented in various components (e.g. endpoint controller, kube-proxy, iptables/Open vSwitch)
and that is the reason we cannot use it for pods' secondary network -- because secondary networks for pods are not
under management of Kubernetes.

### Goals

- Provide a mechanism to access to the endpoints (by virtual IP address, DNS names), which is organized by Kubernetes
service objects, in a phased approach.

### Non-Goals

Due to the variety of service types, this must be handled in a phased approach. Therefore this covers the initial
implementation and the structural elements, some of which may share commonalities with future enhancements.

- Provide all types of Kubernetes service functionality for secondary network interface.
  - In order to focus on quality of an initial subset of service functionality.
- Provide the exact same semantics for Kubernetes service mechanisms.
  - Which would increase the scope substantially for an initial feature.
- LoadBalancer features for pods' secondary network interface (this requires more discussion in upstream community).
  - The approach for which requires more discussion in upstream community, may use other external components.
  - Which may also handle external traffic, this enhancement is for providing service functionality from traffic originating from Kubernetes pods.
- NodePort feature (still in discusssion among upstream community).
  - Still in discusssion among upstream community, and looking for input on use cases.
- Ingress and other functionalities related to Kubernetes service (this will be addressed in another enhancement).
  - This will be addressed in another enhancement.
- Network verification for accesssing services (i.e. users need to ensure network targets are reachabile).
  - This functionality would be separate from this enhancement and the scope is somewhat orthoganal.
  - Additionally, there is on-going upstream discussion regarding secondary networks hierarchy which may also address which networks are reachable.
- Services for universal forwarding mechanisms (e.g DPDK Pods are not supported), we only focus on the pods which uses Linux network stack (i.e. iptables).
  - This makes the scope for this feature reasonable, and we may allow for extensibility which allows for pluggable modules which handle other forwarding mechanisms.
- Headless service (need to do more discussion in community for its design)
  - Due to assumptions of reachability to coreDNS, which may not be available to clients on isolated networks.

## Proposal

### Target Service Functionality

We are targeting the following functionality for services in this proposal:

- ClusterIP

#### Service Reachability

The service can be accessed from the pods that can access the target service network. For example, if `ServiceA` is
created for `net-attach-def1`, then `ServiceA` can be accessed from the pods which has `net-attach-def1` network
interface and we won't provide any gateway/routers for access from outside of network in this proposal.

#### Cluster IP for Multus

When the service is created with `type: ClusterIP`, then Kubernetes assign cluster virtual IP for the services. User
can access the services with given virtual IP. The request to virtual IP is automatically replaced with actual
pods' network interface IP and send to the target services. User needs to make sure reachability to the target
network, otherwise the request packet will be dropped.

### User Stories

- As an OpenShift developer, I require the ability to segregate my Kubernetes service data plane traffic onto different networks for reasons of security and performance.
- As an OpenShift developer, I require the ability to segregate my Kubernetes service data plane traffic to a secondary interface that is associated with a different CNI plugin to provide functionality not available in the primary (default network) CNI plugin.
- As an OpenShift networking administrator, I require the ability to provide access to an internally-facing corporate network and an externally-facing DMZ network, so application developers can choose which network (pod interface) to use for the different types of traffic that make up their application (e.g. web server:external, DB:internal).
- As an OpenShift network administrator, I require the ability to use one NIC on my hosts for control plane traffic, and additional NICs for data plane traffic.

### Implementation Details/Notes/Constraints

In this enhancement, we will introduce following components (component names may be changed during development/community discussion):

- Multus Service Controller
- Multus Proxy

The Multus service controller watches all pods' secondary interfaces, services and network attachment definitions and
it creates `EndpointSlice` for the service. `EndpointSlice` contains secondary network IP address which Multus proxy
watches. Service and EndpointSlice have the label, `service.kubernetes.io/service-proxy-name`, which is defined at
[kube-proxy APIs](https://pkg.go.dev/k8s.io/kubernetes/pkg/proxy/apis), to make target service out of Kubernetes
management.

Multus proxy provides forwarding plane for multus-service with iptables at Pod's network namespace. It watches Service
and EndpointSlice with 'service.kubernetes.io/service-proxy-name: multus-proxy' and it create iptables rules to
change the packet's destination to the service pods from Service VIP. It does not provide NAT (ip masquerade) for now
because multus network is mainly for 'secondary network' and it assumes default route to primary Kubernetes cluster
network.

#### Create Service

1. Service is created

User creates Kubernetes service object. At that time, user needs to add the following annotation/label:

- `label: service.kubernetes.io/service-proxy-name` (well-known label, defined in kubernetes)

Users will need to set the label `service.kubernetes.io/service-proxy-name`, with the value, `multus-proxy`. This
label specifies the component that takes care of the service. Without this variable, upstream kubernetes
(i.e. kube-proxy) will handle this service, then kube-proxy will create iptables rules. This label should be set to
`multus-proxy`. For OpenShift, we should verify that this label is recognized by openshift-sdn as well as
ovn-kubernetes. This enhancement should address openshift-sdn and ovn-kubernetes repositories.

- annotations: k8s.v1.cni.cncf.io/service-network (newly introduced)

`k8s.v1.cni.cncf.io/service-network` specifies which network (i.e. net-attach-def) is the target for exposing the
service. This label specifies only one net-attach-def in same namespace of the Service/Pods, hence Service/Network
Attachment Definition/Pods should be in the same namespace. This annotation only allows users to specify one network,
hence one Service VIP should match one of service/network mappings.

Then multus service controller will watch the service and pods.

2. Pods are launched and multus service controller creates the endpointslices for the sevice.

Once multus service controller finds pods associated with the service, then the controller will create EndpointSlices
for the service. The EndpointSlices contains a pod's net-attach-def IP address from pod's network status annotation
and the EndpointSlices has same label, `service.kubernetes.io/service-proxy-name`, and its value from corresponding
service to avoid kube-proxy adding iptables rules for Kubernetes Service forwarding plane. When the pods/services
is updated/removed, then multus service controller changes/removes the corresponding EndpointSlices to track the
changes.

3. Multus proxy generates iptables rules for the service in Pod's network namespace.

Multus proxy watches the endpointslices and services. Periodically multus proxy generates iptables rules for the pods
and puts it in a pod's network namespace (not host network namespace, as kube-proxy does). Multus proxy tracks the
changes of service/endpoints, hence multus proxy update/remove iptables rules when service/endpoints are
updated/removed.

Here is the example of iptables rules in Pod's network namespace (1 service with 2 service Pods).

```text
*nat
:PREROUTING ACCEPT [0:0]
:INPUT ACCEPT [0:0]
:OUTPUT ACCEPT [0:0]
:POSTROUTING ACCEPT [0:0]
:MULTUS-SEP-LLWTBYDUBFR4XHIV - [0:0]
:MULTUS-SEP-XVNAIRZXTXMCKBXF - [0:0]
:MULTUS-SERVICES - [0:0]
:MULTUS-SVC-HEXNTD6JIC42P6W2 - [0:0]
-A OUTPUT -m comment --comment "multus service portals" -j MULTUS-SERVICES
-A MULTUS-SEP-LLWTBYDUBFR4XHIV -p tcp -m comment --comment "default/multus-nginx-macvlan" -m tcp -j DNAT --to-destination 10.1.1.14:80
-A MULTUS-SEP-XVNAIRZXTXMCKBXF -p tcp -m comment --comment "default/multus-nginx-macvlan" -m tcp -j DNAT --to-destination 10.1.1.15:80
-A MULTUS-SERVICES -d 10.108.162.172/32 -p tcp -m comment --comment "default/multus-nginx-macvlan cluster IP" -m tcp --dport 80 -j MULTUS-SVC-HEXNTD6JIC42P6W2
-A MULTUS-SVC-HEXNTD6JIC42P6W2 -m comment --comment "default/multus-nginx-macvlan" -m statistic --mode random --probability 0.50000000000 -j MULTUS-SEP-LLWTBYDUBFR4XHIV
-A MULTUS-SVC-HEXNTD6JIC42P6W2 -m comment --comment "default/multus-nginx-macvlan" -m statistic --mode random --probability 1.00000000000 -j MULTUS-SEP-XVNAIRZXTXMCKBXF
```

### Risks and Mitigations

#### Interworking service.kubernetes.io/service-proxy-name among other Kubernetes network

This feature depends on the well-known label, `service.kubernetes.io/service-proxy-name` implementation. If some
Kubernetes network solution satisfies the following condition, then this feature does not work:

- The SDN solution provides forwarding plane for Kubernetes service (i.e. Cilium, ovn-kubernetes).
- The SDN solution does not recognize the well-known label, `service.kubernetes.io/service-proxy-name`.

Hence we should mention which SDN solution supports this enhancement in documents.

## Design Details

### Open Questions [optional]

N/A

### Test Plan

For upstream testing, we should address e2e test in upstream in some fashion, however, upstream test will be done
with `kind`, hence we could cover some of primitive testing. More detailed testing should be done in baremetal CI
job or SR-IOV CI job with SR-IOV devices.

### Graduation Criteria

This feature, multus service abstraction, is fairly complicated because additional networks (i.e. network attachment
definition) depend on customer environments, so each customer has each network and these are unlikely to be the same.
So we require customer feedback not only about its quality also about their use-case and their experiences. We
should watch customer feedback and carefully think about the current status of the feature and proceed to graduation.

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users; rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

#### Tech Preview -> GA

- Gain consensus regarding design and implementation among upstream communities
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

This is dependent on the provision of gaining consensus, customer feedback as well as technical gaps which may be uncovered during.

- *4.10*: Developer Preview
- *4.11*: Tech Preview
- *TBD*: GA

## Drawbacks

- Redesign users' infrastructure (including network) not to use multus (i.e. additional secondary networks)

## Alternatives

- Implement secondary network and service feature in ovn-kubernetes (but of course, it cannot support macvlan and other interface)

### Required for GA

GA requires following testing infrastructure:

- Baremetal OpenShift deployment (for scale testing), with multiple network (macvlan/ipvlan/sr-iov)
- Baremetal OpenShift deployment (for CI e2e testing), with multiple network. It might be possible to implement in equinix metal.
