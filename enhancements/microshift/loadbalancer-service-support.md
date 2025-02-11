---
title: loadbalancer-service-support
authors:
  - "@pliurh"
reviewers:
  - "@copejon, MicroShift contributor"
  - "@fzdarsky, MicroShift architect"
  - "@ggiguash, MicroShift contributor"
  - "@dhellmann, MicroShift contributor"
  - "@oglok, MicroShift contributor"
  - "@zshi-redhat, MicroShift "
  - "@pmtk, MicroShift"
  - "@mangelajo, MicroShift"
approvers:
  - "@dhellmann"
api-approvers:
  - None
creation-date: 2023-01-13
last-updated: 2023-01-13
tracking-link:
  - https://issues.redhat.com/browse/NP-604
---

# MicroShift Service of Loadbalancer Type Support

## Summary

Customers using loadbalancer type Service don't want to make changes to their
workloads/deployment for MicroShift as they use other k8s flavors as well. It is
also promised by us that users can create workload manifests that can work on
both OpenShift and MicroShift (as long as they do not use APIs unsupported by
MicroShift).

## Motivation

Normally, the loadbalancer type Service is supported by a Kubernetes cluster
with external load balancers. In MicroShift, as there is only one node in the
cluster, we don't really need to have a load balancer. However, from user
experience perspective, it's important for customers to be able to deploy the
workload on both OpenShift MicroShift with the same manifests. And there are
workload types that require a load balancer service (for example, if they are
not using HTTP/S), so that other features like Ingress or node port service will
not work. Therefore we need to find a way to support the loadbalancer type
Service on MicroShift.

### User Stories

As a Microshift user, I want to be able to create a Service manifest with the
type of `LoadBalancer` and use it on both OCP and MicroShift.  I don't want to
allocate a new IP address to the Node as the loadbalancer VIP. I want to use the
NodeIP as the ingress IP of the loadbalancer Service. 

### Goals

* Allow users to deploy the load balancer type Service to MicroShift.
* The node IP shall be used as the ingress IP of the Service.
* As we will support pluggable CNI in MicroShift. The implementation shall be
also compatible with CNIs other than OVN-Kubernetes.

### Non-Goals

* 

## Proposal

To support the loadbalancer type Service, we need to create a new controller
component that watch the Service objects. It shall be responsible for:

1. work with CNIs to plumb the ingress traffic to the endpoint pods.
2. publish the ingress endpoint information in the Service's
   `.status.loadBalancer` field.

### Workflow Description

**cluster user** is a human user responsible for provisioning Services to a
MicroShift cluster.

1. The cluster user deploys a Service of type: LoadBalancer 
2. The loadbalancer service controller watches the creation of the service. It
will update the Service's `.status.loadBalancer` field, and help the CNIs to 
plumbing the ingress traffic to the endpoint pods.

### API Extensions

None

### Implementation Details

This new controller is part of the microshift binary. It shall be be limited to
the OVN-K CNI implementation, for now. Updating it to support other CNI
implementations will be left to a future enhancement.

### Risks and Mitigations

The pluggable CNI design of MicroShift is still under discussion, we need to
make sure that CNI type can be aware in MicroShift binary.

### Drawbacks

It is not a CNI agnostic design. The loadbalancer Service controller will behave
differently between OVN-Kubernetes and other CNIs.

## Design Details

The Service of loadblancer type is designed to exposes the Service externally
using a cloud provider's load balancer. Normally, there is a loadbalancer
Service controller which is responsible for:

1. provision a loadbalancer for the Service.
2. publish the ingress endpoint information in the Service's
   `.status.loadBalancer` field.

However, in the context of MicroShift, as we only get one node in the cluster,
it doesn't make sense to use a real load balancer to forward the ingress
traffic. So instead of provisioning the loadbalancer instance, the controller
is only responsible for helping the CNIs to plumbing the ingress traffic.

### Traffic Plumbing

Since different CNIs get different implementation on how to support Kubernetes
Services. We need to do the traffic plumbing differently for different CNI. The
first version of the controller shall _not_ be aware of the CNI type from the
configuration. Enhancing it to support other CNI implementations is deferred to
a future release.

#### OVN-Kubernetes

OVN-Kubernetes has already implemented the [loadbalancer ingress
IP](https://github.com/openshift/ovn-kubernetes/blob/master/docs/external-ip-and-loadbalancer-ingress.md)
support. The ovnkube-node component which is running on each host watches the
Service's `.status.loadBalancer` field, and insert iptables rules that does DNAT
to the clusterIP of the Service. Therefore, from the loadbalancer Service
controller perspective, it will only be responsible for the updating the status
of the Service. The ovn-kubernetes will do the rest of the job.

#### Notes for Future Work to Support Other CNIs

For other CNIs, which don't have the similar function as OVN-Kubernetes, neither
do they bypass the kernel for the ingress traffic, we can take the similar
approach as K3S. The K3S's loadbalancer Service controller provisions a
[klipper-lb](https://github.com/k3s-io/klipper-lb) pod to each nodes for each
loadbalancer Service. The pod can insert iptables rules which DNAT ingress
traffic to the nodeIP of the Service.

Another option is manipulating the iptables rule by this controller itself.
Since there is only one node, and the microshsift binary has the access to the
host network.

For those CNIs that relays on kube-proxy to support the kubernetes Service. We
can also consider to use kube-proxy to do the job.

### Update Status
As, we don't want to ask user to allocated additional VIP as the loadbalancer
IP. We can say that the ingress ip of the Service's `.status.loadBalancer` field
shall always be the existing IPs of the node. When there are multiple IPs, the
kubelet `NodeIP` shall be used by default.

The controller shall also check if the Service port has already been allocated
to other loadbalancer Service before updating the Service status. To avoid the
risk of a race condition, we shall implement the controller with a work queue
and one worker. All the service update/creation requests will be handled in
sequence.

### Open Questions [optional]

* Do we want to allow users to choose the IP other than the NodeIP as the
  Ingress IP of loadbalancer Service, when there are multiple IPs available?

  A: We will leave this as an open question for now and wait for an RFE to help
  us shape requirements.

* How do we make this implementation compatible with other loadbalancer
  implementations such as metal-lb?
  
  A: To avoid the possible conflict between loadbalancer implementations, we
  plan to expose a configuration flag that allows a user to turn the built-in
  controller off. But it will not be implemented until we get a definite
  requirement to support other loadbalancer implementations.

### Test Plan

N/A

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

If the port of one Service  has already been occupied by either a node process
or other loadbalancer Services, the controller shall leave the
`.status.loadBalancer` of the Service blank, thus the Service will be put in
pending state. 

#### Support Procedures

N/A

## Implementation History

N/A

## Alternatives

Implement a k8s Cloud Controller Manager for MicroShift. The Cloud Controller
Manager will be running in the pod of the MicroShift Cluster.
