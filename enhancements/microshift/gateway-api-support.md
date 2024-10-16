---
title: gateway-api-support
authors:
  - pacevedom
reviewers:
  - copejon
  - dgn
  - eslutsky
  - ggiguash
  - jerpeter1
  - pmtk
  - shaneutt
approvers:
  - jerpeter1
api-approvers:
  - None
creation-date: 2024-10-14
last-updated: 2024-10-14
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1487
---

# Gateway API support in MicroShift

## Summary
At the time of this writing MicroShift supports two types of ingresses:
[OpenShift routes](https://docs.redhat.com/en/documentation/red_hat_build_of_microshift/4.16/html/networking/microshift-configuring-routes)
and [Kubernetes ingress](https://docs.redhat.com/en/documentation/red_hat_build_of_microshift/4.16/html/networking/microshift-configuring-routes#nw-ingress-creating-a-route-via-an-ingress_microshift-configuring-routes).
In order to add more routing features a new project was born in upstream:
[Gateway API](https://gateway-api.sigs.k8s.io/). This is an official kubernetes
project focused on L7 routing (support for L4 is experimental), posed to be the
next standard in ingress, load balancer and service mesh APIs.

This enhancement proposes an implementation to support this new API as an
optional component in MicroShift.

## Motivation
Gateway API support will provide users with another way of expressing their
ingress routing capabilities, also aligned with the way Kubernetes foresees
this functionality.

### User Stories
* As a MicroShift admin, I want to add different Gateway resources to the
  cluster for applications to create routes on them.
* As a MicroShift application developer, I want to add HTTP routes to specific
  Gateways so that I can express routing capabilities using an upstream API.
* As a MicroShift application developer, I want to add GRPC routes to specific
  Gateways so that I can express routing capabilities using an upstream API.

### Goals
- Full support for core, non-experimental resources, including gateways, HTTP routes and GRPC routes.
- Full support for multiple Gateway instances.
- Full support for north/south [use case](https://gateway-api.sigs.k8s.io/concepts/use-cases/#basic-northsouth-use-case).
- Lightweight implementation, consuming as little resources as possible.

### Non-Goals
- Automatically removing Gateway API from the cluster upon RPM uninstall
- Provide support for experimental resources, such as those listed [here](https://gateway-api.sigs.k8s.io/concepts/api-overview/#route-resources).
- Support different GatewayClasses with the same controller.

## Proposal
As the Gateway API is still in development and MicroShift already provides
means for routing capabilities, this feature should be optional.

As with other optional features there will be an optional RPM with all the
required manifests to be applied upon MicroShift's start. The backing
implementation for Gateway API support will be based on the latest OpenShift
Service Mesh component, which is an optional operator from OpenShift.

OpenShift Service Mesh (OSSM) is the service mesh operator from OpenShift.
Since version 3 (which is going to be Tech Preview at the time of this writing)
it is based in the [sail operator](https://github.com/openshift-service-mesh/sail-operator/tree/main),
and is released as an optional operator in the Red Hat operators catalog for
OLM. This operator deploys all of Istio's manifests and is capable of handling
Istio control plane deployments, Istio CNIs and other components. Istio has
first class support for Gateway API resources and is already supported by Red
Hat.

### Workflow Description
**User** is a human user responsible for setting up and managing Edge Devices.
**Application** is user's workload that intends to use routing capabilities.

#### Installation and usage on RHEL For Edge (ostree)
> In this workflow, it doesn't matter if the device is already running R4E with existing MicroShift cluster.
> Deployment of new commit requires reboot which will force recreation of the Pod networking after adding Multus.

1. User gathers all information about the networking environment of the edge device.
1. User prepares ostree commit that contains:
   - MicroShift RPMs.
   - Gateway API for MicroShift RPM.
   - Gateway CRs
   - Application using mentioned Gateway CRs.
1. User deploys the ostree commit onto the edge device.
1. Edge device boots:
1. (optional) Init procedures are configuring OS and networks.
1. MicroShift starts.
1. MicroShift applies Gateway API manifests.
1. Istio controller becomes available.
1. MicroShift applies Application's manifests that may include Gateways and routes.
1. Gateway pods are created and OSSM inspects routes to accept them into each Gateway.
1. Application's pods are running, they are reachable through the routes in the Gateway.

#### Installation and usage on RHEL (rpm)
##### Adding to existing MicroShift cluster (or before first start)
1. MicroShift already runs on the device. (This step may not be true if its before first start)
1. User installs `microshift-gateway-api` RPM
1. User restarts MicroShift service.
1. MicroShift starts and deploys all Gateway API resources from `manifests.d`.
1. Istio controller becomes available.
1. User creates Gateway and routes (HTTP or GRPC) CRs
1. When Gateway pods are created, routes are picked up and configured by the controller which is already running.
1. Application's pods are running, they are reachable through the routes in the Gateway.

### API Extensions
Most of the manifests are CRDs, and users are left only with creating Gateway,
HTTPRoute and GRPCRoute resources. These are explained in the use cases in the
[documentation](https://gateway-api.sigs.k8s.io/concepts/use-cases/#basic-northsouth-use-case).

### Topology Considerations

#### Hypershift / Hosted Control Planes
Not applicable

#### Standalone Clusters
Not applicable

#### Single-node Deployments or MicroShift
Enhancement is solely intended for MicroShift.

### Implementation Details/Notes/Constraints [optional]
N/A

#### Manifests
Gateway API is just a set a CustomResourceDefinitions. In order to support it
we need to install OSSM, which brings Istio with it and more CRDs. Aside from
the required CRDs, the operator from OSSM and the associated resources for the
istio control plane are also deployed automatically.

All these manifests are updated with a manual rebase procedure to extract them
from an OSSM bundle.

#### RPM package
RPM spec to build `microshift-gateway-api` and
`microshift-gateway-api-release-info` RPMs will be part of existing
`microshift.spec` file.
The RPM will include:
- Manifests required to deploy OSSM on MicroShift without OLM.
- Manifests required to create Gateway API resources on MicroShift.
- A GatewayClass manifest to provide the gateway class which Istio will act
  upon.
- An Istio manifest to configure the service mesh control plane.
- A greenboot script to ensure OSSM is ready.

### Risks and Mitigations
The risks for this feature are two-fold: technical and non-technical.

On the technical side we have a dependency between CRDs and CRs when
installing. When a CRD has not yet been established it means the apiserver is
not able to serve it yet. Since there are CRs in the same RPM that need CRDs
to be available this could become a race condition when installing. The
mitigation would be to turn the RPM into a controller with proper mechanisms
to wait and sync resources. An RPM was chosen because of the dev preview nature
of the feature, as it simplifies things on MicroShift.

On the non-technical side, this feature is born as Dev Preview to wait on
customer feedback and adoption. This has no mitigation other than help out
potential users of the feature

### Drawbacks
The inclusion of OpenShift Service Mesh brings extra resource usage. As this is
released as dev preview, users need to opt into it. Additional memory should be
planned when enabling this feature, as OSSM components and the gateways
themselves consume additional resources.

## Open Questions [optional]
N/A

## Test Plan
Gateway API support for MicroShift will be tested using existing test
harness. A new suite will be created with a simple test for the overall setup
with an HTTP route. Another test will ensure compatibility with the current
OpenShift router deployment.

## Graduation Criteria
Gateway API for MicroShift is targeted to be Dev Preview on MicroShift 4.18.

At the time of this writing OSSM is in Tech Preview state, expected to graduate
to GA in 2025.

Graduation criteria will be determined by customer adoption/feedback.

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA
N/A

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy
Gateway API and MicroShift RPMs get built from the same spec so they share
version. Whenever an upgrade happens both of them are also upgraded at the
same time.

The bulk of manifests are CRDs which have their own versioning, so the
upgrade/downgrade strategy relies on the components using them. Extracting the
OSSM manifests from the bundle guarantees that Istio will be compatible with
them.

For Gateway and HTTPRoute/GRPCRoute resources, manual action may be needed but
this is the application's responsibility.

## Version Skew Strategy
Building Gateway API and MicroShift RPMs from the same spec file means both
should be updated together which means there should not be any version skew
between them.

## Operational Aspects of API Extensions
N/A

## Support Procedures
In order to debug error situations with Gateway API and OSSM, Gateways and
routes need to be inspected.

The first thing to check is the gatewayclass status, it needs to be accepted
and handled by the istio controller, as shown in the status:
```
$ oc get gatewayclass openshift-gateway-api -o yaml
apiVersion: gateway.networking.k8s.io/v1
kind: GatewayClass
metadata:
  creationTimestamp: "2024-10-14T07:44:20Z"
  generation: 1
  name: openshift-gateway-api
  resourceVersion: "1060"
  uid: 0707b37e-c3c6-4b8a-a344-24545d8b8351
spec:
  controllerName: openshift.io/gateway-controller
status:
  conditions:
  - lastTransitionTime: "2024-10-14T07:44:30Z"
    message: Handled by Istio controller
    observedGeneration: 1
    reason: Accepted
    status: "True"
    type: Accepted
```

If we have a Gateway `demo-gateway` using the above GatewayClass we should see it has been accepted and programmed by the gateway class:
```
$ oc get gateway -n openshift-ingress demo-gateway -o yaml
apiVersion: gateway.networking.k8s.io/v1
kind: Gateway
metadata:
  annotations:
    gateway.istio.io/controller-version: "5"
  creationTimestamp: "2024-10-14T07:48:16Z"
  generation: 1
  name: demo-gateway
  namespace: openshift-ingress
  resourceVersion: "1697"
  uid: 13275eac-dced-4448-904f-27f78350adca
spec:
  gatewayClassName: openshift-gateway-api
  listeners:
  - allowedRoutes:
      namespaces:
        from: All
    hostname: '*.microshift-9'
    name: demo
    port: 8080
    protocol: HTTP
status:
  addresses:
  - type: IPAddress
    value: 192.168.124.18
  conditions:
  - lastTransitionTime: "2024-10-14T07:48:16Z"
    message: Resource accepted
    observedGeneration: 1
    reason: Accepted
    status: "True"
    type: Accepted
  - lastTransitionTime: "2024-10-14T07:48:17Z"
    message: Resource programmed, assigned to service(s) demo-gateway-openshift-gateway-api.openshift-ingress.svc.cluster.local:8080
    observedGeneration: 1
    reason: Programmed
    status: "True"
    type: Programmed
  listeners:
  - attachedRoutes: 0
    conditions:
    - lastTransitionTime: "2024-10-14T07:48:16Z"
      message: No errors found
      observedGeneration: 1
      reason: Accepted
      status: "True"
      type: Accepted
    - lastTransitionTime: "2024-10-14T07:48:16Z"
      message: No errors found
      observedGeneration: 1
      reason: NoConflicts
      status: "False"
      type: Conflicted
    - lastTransitionTime: "2024-10-14T07:48:16Z"
      message: No errors found
      observedGeneration: 1
      reason: Programmed
      status: "True"
      type: Programmed
    - lastTransitionTime: "2024-10-14T07:48:16Z"
      message: No errors found
      observedGeneration: 1
      reason: ResolvedRefs
      status: "True"
      type: ResolvedRefs
    name: demo
    supportedKinds:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
    - group: gateway.networking.k8s.io
      kind: GRPCRoute
```
If there are no routes we should also see `.status.listeners.attachedRoutes` equal 0.

When having a hostname matching route with a Gateway we need to check whether the route is being served by it. This is checked in the conditions:
```
$ oc get httproute http -o yaml
apiVersion: gateway.networking.k8s.io/v1
kind: HTTPRoute
metadata:
  annotations:
    kubectl.kubernetes.io/last-applied-configuration: |
      {"apiVersion":"gateway.networking.k8s.io/v1beta1","kind":"HTTPRoute","metadata":{"annotations":{},"name":"http","namespace":"default"},"spec":{"hostnames":["test.microshift-9"],"parentRefs":[{"name":"demo-gateway","namespace":"openshift-ingress"}],"rules":[{"backendRefs":[{"name":"hello-microshift","namespace":"default","port":8080}]}]}}
  creationTimestamp: "2024-10-14T07:49:45Z"
  generation: 1
  name: http
  namespace: default
  resourceVersion: "1845"
  uid: f301d74a-8fb1-41cd-b4e8-68c65569b2e8
spec:
  hostnames:
  - test.microshift-9
  parentRefs:
  - group: gateway.networking.k8s.io
    kind: Gateway
    name: demo-gateway
    namespace: openshift-ingress
  rules:
  - backendRefs:
    - group: ""
      kind: Service
      name: hello-microshift
      namespace: default
      port: 8080
      weight: 1
    matches:
    - path:
        type: PathPrefix
        value: /
status:
  parents:
  - conditions:
    - lastTransitionTime: "2024-10-14T07:49:45Z"
      message: Route was valid
      observedGeneration: 1
      reason: Accepted
      status: "True"
      type: Accepted
    - lastTransitionTime: "2024-10-14T07:49:45Z"
      message: All references resolved
      observedGeneration: 1
      reason: ResolvedRefs
      status: "True"
      type: ResolvedRefs
    controllerName: openshift.io/gateway-controller
    parentRef:
      group: gateway.networking.k8s.io
      kind: Gateway
      name: demo-gateway
      namespace: openshift-ingress
```
We can see the route is valid, the backend service is also resolved (meaning it exists), the controller name matches that of the GatewayClass and the parentRef is the gateway that picked up the route.

The Gateway will also show a new attachedRoute now:
```
$ oc get gateway demo-gateway -n openshift-ingress -o yaml | yq '.status.listeners'
- attachedRoutes: 1
  conditions:
    - lastTransitionTime: "2024-10-14T07:48:16Z"
      message: No errors found
      observedGeneration: 1
      reason: Accepted
      status: "True"
      type: Accepted
    - lastTransitionTime: "2024-10-14T07:48:16Z"
      message: No errors found
      observedGeneration: 1
      reason: NoConflicts
      status: "False"
      type: Conflicted
    - lastTransitionTime: "2024-10-14T07:48:16Z"
      message: No errors found
      observedGeneration: 1
      reason: Programmed
      status: "True"
      type: Programmed
    - lastTransitionTime: "2024-10-14T07:48:16Z"
      message: No errors found
      observedGeneration: 1
      reason: ResolvedRefs
      status: "True"
      type: ResolvedRefs
  name: demo
  supportedKinds:
    - group: gateway.networking.k8s.io
      kind: HTTPRoute
    - group: gateway.networking.k8s.io
      kind: GRPCRoute
```

## Alternatives
As listed [here](https://gateway-api.sigs.k8s.io/implementations/) there is a
list of potential implementations for the API. The one selected for this
enhancement (OSSM) is based on Istio. This was done to reduce workload on the
team and match the OpenShift approach, also because this implementation is one
of the most complete in terms of API coverage.

Some other implementations may be explored, but they all come with their own
costs: API coverage, cost of ownership, contributing, etc.

## Infrastructure Needed [optional]
N/A
