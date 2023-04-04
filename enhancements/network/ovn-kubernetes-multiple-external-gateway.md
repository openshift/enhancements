---
title: ovn-kubernetes-multiple-external-gateway
authors:
  - "@jordigilh"
reviewers: 
  - "@trozet"
  - "@tssurya"
  - "@fedepaol"
approvers:
  - "@trozet"
api-approvers: 
  - "@trozet"
creation-date: 2023-02-06
last-updated: 2023-02-17
tracking-link: 
  - https://issues.redhat.com/browse/SDN-2481
---

# OVN-Kubernetes Multiple External Gateways

## Summary

When it comes to ingress traffic, load balancers and reverse proxies take care of redirecting traffic to one of potentially multiple destinations. Egress traffic is no different. Although it is already possible to use egress gateways, telco customers require the capability to use a common and secure API to dynamically configure one or more egress next hops per namespace.

Multiple External Gateway is an existing OVN-K feature that uses external gateways for ingress and egress traffic for all the pods in an annotated namespace, using the following pod and namespace. More information can be found in this [white paper](https://www.f5.com/pdf/white-papers/f5-telco-gateway-for-red-hat-openshift-wp.pdf).
The list of annotations used in the current implementation are as follow:

* Namespace
  * `k8s.ovn.org/routing-external-gws`
  * `k8s.ovn.org/bfd-enabled`
* Pod
  * `k8s.ovn.org/routing-namespaces`
  * `k8s.ovn.org/routing-network`
  * `k8s.ovn.org/bfd-enabled`

However, to ensure only trusted namespaces and deployments use these annotations, it is necessary to leverage on an additional service that controls their usage, such as an admission webhook that can filter which pods use these annotations and in which namespaces.

## Motivation

The goal of this enhancement is two fold:
* To encapsulate the functionality in a secure manner using the cluster's RBAC capability to control who manages these resources.
* To expose the functionality in a common API provided by the CRD. 

### User Stories
* As an OpenShift administrator, I want to use a declarative approach to defining the external gateways in a cluster.
* As an OpenShift administrator, I want to use RBAC to control the access to the external gateway configuration.

### Goals

- Migrate the current pod/namespace annotation based configuration for the Multiple External Gateway to a new cluster scoped CRD.
- Define a controller that reimplements the existing logic that handles the annotations but on the contents of the CRD.
- Modify the existing logic to make it CRD aware, so that it can watch for changes in an annotation, as it works today, or fallback to the CR, in case the annotation does not exist.
- Extend test coverage to include the new CRD use cases, matching the same use cases covered in the annotation tests.
- CRD configuration will be the only officially supported and documented mechanism in OpenShift >= 4.14. Annotations will still be available in OpenShift <= 4.14.
- Extend the existing unit tests for the annotation based logic to accommodate for the scenario where CR and annotations exist in 4.13.
- Add a new field per hop that determines if the Source NAT is to be disabled for that gateway.

### Non-Goals

- Removal of the annotation based logic to configure multiple external gateways. This should be carrier out at least no later than OpenShift 4.14 to guarantee a stable deprecation path to existing deployments that leverage on the annotations to configure the external gateways.

## Proposal

Create a new cluster scoped CRD that handles internal and external routing scenarios. For the purpose of this enhancement, only the external use case will be covered.

The implementation will also include a new logic that will watch for instances of this CRD and apply the changes to the OVN network configuration as it is currently done by the annotation logic. 

The logic will use the configuration from both the CR and the annotation when determining the gateway IPs for the pods in a target namespace. This will help to allow a smooth transition between the annotation and the CR without incurring into traffic disruptions. Note that duplicated gateway IPs will be ignored and treated as a single gateway IP instance when configuring the north bound database.

The annotation logic will be removed in OpenShift 4.15.

### Workflow Description
The workflow follows the behavior of a cluster scoped CR.


### API Extensions

A new cluster scoped CRD `AdminPolicyBasedExternalRoute` is created to represent the multiple external gateways configuration. The `spec` structure exposes the following fields:

* `from`: determines where the routing polices should be applied to. Only `namespaceSelector` is supported for external traffic. This field is mandatory.
* `nextHops`: defines the destination where the packets should be forwarded to. There are two types of references: static and dynamic. Static IPs usually refer to interfaces that hold an IP which doesn't change overtime Dynamic IPs link to pod IPs and can potentially change over time as they are linked to the pod's lifecycle.
  *`static`: is a slice of static IPs, each item containing the following fields:
      * `ip`: IPv4 or IPv6 of the next destination hop. This field is mandatory.
      * `skipHostSNAT`: When true, it disables applying the Source NAT to the host IP. This field is optional and defaults to false.
      * `bfdEnabled`: determines if Bi-Directional Forwarding Detection (BFD) is supported in this network and follows the same behavior as with the annotation: it's enabled when set to true and disabled when false or omited in the manifest. It maps to the `k8s.ovn.org/bfd-enabled` annotation in the pod. This field is optional and defaults to false.
  * `dynamic`: references contain the following fields:
    * `podSelector` implements a [set-based](https://kubernetes.io/docs/concepts/overview/working-with-objects/labels/#set-based-requirement) label selector to filter the pods in the namespace that match this network configuration. These are dynamic IPs linked to the lifecycle of the pod. This field is mandatory.
    * `namespaceSelector` It defines a `set-based` selector that is used to filter the namespaces where to apply the `podSelector`. This field is mandatory.
    * `skipHostSNAT`: When true, it disables applying the Source NAT to the host IP. This field is optional and defaults to false.
    * `bfdEnabled`: determines if Bi-Directional Forwarding Detection (BFD) is supported in this network and follows the same behavior as with the annotation: it's enabled when set to true and disabled when false or omited in the manifest. It maps to the `k8s.ovn.org/bfd-enabled` annotation in the pod. This field is optional and defaults to false.
    * `networkAttachmentName`: Name of the network attached definition. The name needs to match from the list of logical networks generated by Multus and associated to the pod via annotation. This field is optional. When this field is empty or not defined, the IP from the pod's host network will be used instead. The pod must have `hostnetwork` enabled in order for this feature to work.

The following yaml examples an instance of the CRD:

```yaml
apiVersion: k8s.ovn.org/v1
kind: AdminPolicyBasedExternalRoute
metadata:
  name: honeypotting
spec:
## gateway example
  from:
    namespaceSelector:
      matchLabels:
          multiple_gws: true
  nextHops:
    static:
      - ip: "172.18.0.2"
        bfdEnabled: true
      - ip: "10.244.0.3"
        skipHostSNAT: true
    dynamic:
      - podSelector:
          matchLabels:
            external-gateway: true
        bfdEnabled: true
        namespaceSelector:
          matchLabels:
            gateway: true
        networkAttachmentName: sriov1
      - podSelector:
          matchLabels:
            secondary-gateway: true
        skipHostSNAT: true
        namespaceSelector:
          matchLabels:
            secondary-gateway: true
        networkAttachmentName: sriov2
```

### Risks and Mitigations
N/A

### Drawbacks
N/A

## Design Details


### Test Plan

* Unit test coverage for CR based to at least match use cases in existing annotation based test cases.
* Unit test for both annotation and CR to include tests cases to validate expected behavior when running in OpenShift 4.14.

### Graduation Criteria
N/A

#### Dev Preview -> Tech Preview
This feature is GA in 4.14, no Dev or Tech previews are required because the logic already exists in OVN-K as an annotation.


#### Tech Preview -> GA
This feature is GA in 4.14, no Dev or Tech previews are required because the logic already exists in OVN-K as an annotation.


#### Removing a deprecated feature
Removal of the annotation based logic to occur in OpenShift 4.15.

### Upgrade / Downgrade Strategy
There are no plans, tools or documentation to assist in migrating the annotation based solution to the CRD based approach.

### Version Skew Strategy
N/A

### Operational Aspects of API Extensions
N/A

#### Failure Modes
N/A

#### Support Procedures
N/A
## Implementation History
First implementation targeting OpenShift 4.14 in GA capacity.

## Alternatives
Currently the annotation based implementation is the only alternative available to this enhancement.