---
title: gateway-api-with-cluster-ingress-operator
authors:
  - "@Miciah"
reviewers:
  - "@candita"
  - "@gcs278"
approvers:
  - "@frobware"
api-approvers:
  - "@knobunc"
creation-date: 2022-12-13
last-updated: 2023-04-12
tracking-link:
  - https://issues.redhat.com/browse/NE-1105
  - https://issues.redhat.com/browse/NE-1107
  - https://issues.redhat.com/browse/NE-1108
see-also:
  - "/enhancements/ingress/gateway-api-crd-life-cycle-management.md"
---

# Gateway API with Cluster Ingress Operator

This enhancement describes changes to the [Ingress Operator](https://github.com/openshift/cluster-ingress-operator) to manage the
OpenShift Service Mesh Operator in order to enable Gateway API for ingress
use-cases without interfering with mesh use-case.

## Summary

This enhancement extends the Ingress Operator to install and manage the Gateway
API CRDs and the OpenShift Service Mesh Operator, which manages Istio and Envoy.
This new capability of the Ingress Operator enables a cluster admin to configure
Istio/Envoy using Gateway API's GatewayClass and Gateway custom resources and
enables project admins to configure Envoy using Gateway API's HTTPRoute custom
resource.  (In the future, other Gateway API custom resources will be supported
as APIs stabilize and Istio adds support for them.)  The end result is a
comprehensive turnkey solution very much like what is possible today using
OpenShift's IngressController and Route APIs but with the advantage of using
Gateway API, which is the up-and-coming Kubernetes community standard API for
configuring cluster ingress.

## Motivation

Ingress is a fractured ecosystem.  Red Hat provides a comprehensive but
OpenShift-specific solution, in the form of our Ingress Operator and OpenShift
router and the corresponding IngressController and Route APIs.  Other vendors
and community projects offer alternative APIs and ingress controller
implementations.  Kubernetes does have a standard Ingress API, but the API is
rudimentary, and so implementations thereof are rife with
implementation-specific extensions to fill in missing functionality.  Gateway
API mends this fractured ecosystem by providing a comprehensive and portable API
with broad community and vendor support through numerous implementations.  Istio
is a leading implementation of Gateway API, in addition to being OpenShift's
standard implementation for service mesh.  Integrating Ingress Operator and
Service Mesh Operator enables OpenShift to provide an ingress solution that
aligns with the community that has coalesced around Gateway API as well as the
existing technology on which OpenShift Service Mesh is based.

### User Stories

This enhancement proposal enables the following user stories.

#### Enabling Gateway API

_"As a cluster admin, I want to configure a Gateway so that I can enable project
admins to configure ingress to their applications, using HTTPRoute resources."_

The cluster admin can configure a Gateway with the following three steps:

1. Create a GatewayClass with `spec.controllerName: openshift.io/gateway-controller`.
2. Create a secret with a default certificate.
3. Create a Gateway with listeners for HTTP and HTTPS, using the secret from Step 2 and the GatewayClass from Step 1.

The Ingress Operator and Service Mesh Operator handle all the details
of configuring Envoy, provisioning a load balancer, and configuring DNS,
providing a turnkey ingress solution that immediately enables project admins to
publish their applications using the Gateway that the cluster admin has created.

For example, the cluster admin can use the following GatewayClass manifest:

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: GatewayClass
metadata:
  name: openshift-default
spec:
  controllerName: openshift.io/gateway-controller
```

Then the cluster admin must create a secret with the default certificate:

```shell
oc -n openshift-ingress create secret tls gwapi-wildcard --cert=wildcard.crt --key=wildcard.key
```

Finally, the cluster admin can use a Gateway manifest like the following:

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: Gateway
metadata:
  name: gateway
  namespace: openshift-ingress
spec:
  gatewayClassName: openshift-default
  listeners:
  - name: http
    hostname: "*.gwapi.example.com"
    port: 80
    protocol: HTTP
    allowedRoutes:
      namespaces:
        from: All
  - name: https
    hostname: "*.gwapi.example.com"
    port: 443
    protocol: HTTPS
    tls:
      mode: Terminate
      certificateRefs:
      - name: gwapi-wildcard
    allowedRoutes:
      namespaces:
        from: All
```

Then a project admin can attach an HTTPRoute to the Gateway.

#### Enabling both ingress and mesh

_"As a cluster admin, I want to configure ingress using Gateway API, and I want
to enable application owners to configure mesh using the Istio API, and I want
these two things to work harmoniously."_

The goals of this enhancement include that ingress and mesh use a unified
control-plane (with a single Istio control-plane deployment handling both
ingress and mesh).  The cluster admin should be able to enable mesh and
subsequently enable ingress or vice versa without needing to reconfigure the
mesh or ingress, respectively, in the subsequent step.

#### Exposing an application

_"As a project admin, I want to configure ingress to my application using an
HTTPRoute, the same way I would use an OpenShift Route."_

Once the cluster admin has configured a Gateway as described in the earlier user
story, the project admin can define an HTTPRoute that attaches to the Gateway.
For example:

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: HTTPRoute
metadata:
  name: http
  namespace: example-app
spec:
  parentRefs:
  - name: gateway
    namespace: openshift-ingress
  hostnames: ["test.gwapi.example.com"]
  rules:
  - backendRefs:
    - name: example-app
      port: 8080
```

This example exposes the "example-app" service, using port 8080 of the service.
In this example, the HTTPRoute specifies a host name that matches both listeners
on the Gateway, so the application is exposed using both HTTP and HTTPS.  When a
client connects to the application using HTTPS, the default certificate that the
cluster admin specified on the Gateway is used.  Because the "HTTPS" listener is
configured with `tls.mode: Terminate`, the Envoy proxy acts as an
edge-termination point for the TLS connection and forwards a cleartext HTTP
connection to the "example-app" service.

### Goals

* Provide a Gateway API implementation for OpenShift based on Istio/Envoy.
* Automate the installation of Istio/Envoy by means of OpenShift Service Mesh.
  * Enable cluster admins to configure ingress using [Gateway](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1beta1.Gateway) and [GatewayClass](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1beta1.GatewayClass).
  * Enable project admins to configure ingress to an application using [HTTPRoute](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1beta1.HTTPRoute).
  * Automatically install OSSM when a GatewayClass is created, if needed.
  * Manage a LoadBalancer-type service and wildcard DNS for Gateways.
  * Provide a default security policy, which the cluster admin may modify.
* Enable use of Gateway API v1beta1 features that are supported by OSSM.
  * Allow cleartext HTTP and edge-terminated HTTPS traffic.
* Use a unified control-plane (i.e. a single Istio control-plane deployment for ingress and mesh).
  * Enable the cluster admin to install OSSM and then Gateway API or vice versa.
  * Work harmoniously with OpenShift Service Mesh for service mesh use-cases.
* Co-exist with third-party (non-Red Hat) Gateway API implementations.
  * Ignore a Gateway if it does not specify a GatewayClass that belongs to OpenShift.

### Non-Goals

* Gateway API features that are not yet beta status (as of Gateway API v0.5.1) are not supported.
  * [GRPCRoute](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.GRPCRoute),
    [TCPRoute](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.TCPRoute),
    [TLSRoute](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.TLSRoute),
    and [UDPRoute](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1alpha2.UDPRoute)
    are still alpha, and so they are not supported at this time.
  * Support for TLS passthrough requires TLSRoute, so it is not supported.
  * [Policy attachment](https://gateway-api.sigs.k8s.io/references/policy-attachment/) is experimental, and so it is not supported at this time.
  * Gateway API uses policy attachment for settings such as retries and timeouts, and so these are not supported.
  * [Backend properties](https://gateway-api.sigs.k8s.io/geps/gep-1282/) are experimental, and so they are not supported at this time.
  * Gateway API defines TLS reencrypt in terms of backend properties, and so reencrypt is not supported at this time.
* Not all features of the IngressController API are supported with Gateway API.
  * OpenShift does not automatically generate a default certificate for a Gateway; the cluster admin must do this.
  * This enhancement does not provide the full set of endpoint publishing options that are available in the IngressController API.
    * Using a NodePort-type service is not supported (but the cluster admin could define one manually).
    * Using host networking is not supported.
  * This enhancement does not monitor Envoy using a canary application.
  * None of the following items can be configured on Envoy at this time:
    * HTTP error pages.
    * Access logs.
    * Pod placement.
    * Allowed TLS version or ciphers.
    * HTTP compression.
    * Client TLS.
    * HTTP `forwarded` header or other HTTP headers.
    * Envoy performance tuning options.
* OpenShift does not provide special support for third-party (non-Red Hat) Gateway API implementations.
  * OpenShift does not configure LB, DNS, etc. for third-party implementations.
* OpenShift does not support using Istio APIs to configure ingress (but they may still be used to configure mesh).
  * If an Istio feature cannot be configured using Gateway API, that feature is not supported.
* This enhancement does not deprecate OpenShift router or the Route API.
  * OpenShift router remains a standard OpenShift component.
  * Istio/Envoy do not watch Routes or Ingresses.
  * OpenShift does not translate Routes or Ingresses into HTTPRoutes.

## Proposal

Most of the proposed changes are to Ingress Operator.  Some API changes may also
be warranted.

### Changes to the Ingress Operator

With this enhancement, the Ingress Operator is responsible for managing Gateway
API CRDs, installing the OpenShift Service Mesh Operator, managing a
LoadBalancer-type service for a Gateway, and managing DNS for the same.  This
requires the following changes:

* Refactor the existing DNS management logic to enable its re-use.
* Add a new controller to manage Gateway API CRDs.
* Add a new controller to install and configure the OpenShift Service Mesh Operator.
* Modify the DNS controller not to require an associated IngressController.
* Add a new controller to manage DNS for Gateways.

These changes are explained in more detail in the following sections.

#### Refactoring Service and DNS Management

The Ingress Operator is designed around the IngressController API, and much
logic assumes that there is some IngressController object that specifies the
relevant configuration and reports status.  In particular, the operator has
logic for reconciling services and DNS records for an IngressController, which
specifies the desired service type (NodePort, LoadBalancer, or none) and the
desired DNS domain.  This code needs to be refactored so that it can be re-used
by logic that reconciles Gateway objects.

The initial enhancement provides a subset of the options that are available with
the IngressController API.  In particular, using a LoadBalancer-type service is
supported; using a NodePort-type service or host networking is not.

#### New Controller to Manage Gateway API CRDs

This enhancement adds a new controller to create the gatewayclasses CRD.  For
dev preview, this controller watches for the "GatewayAPI" feature to be enabled
in the cluster featuregate.  When the feature is enabled, this controller
installs the Gateway API CRDs, including the gatewayclasses CRD, which enables
the cluster admin to create a new GatewayClass.

The Ingress Operator effectively owns the Gateway API CRDs.  It installs
versions of the CRDs that are compatible with the OSSM version that it installs,
and if the CRDs are already installed, the operator may overwrite them.

#### New Controller to Install and Configure OpenShift Service Mesh

The enhancement adds another new controller to reconcile GatewayClasses by
installing OpenShift Service Mesh (OSSM) when appropriate.  When the cluster
admin creates a GatewayClass that specifies the
"openshift.io/gateway-controller" controller name, this controller then does the
following:

1. Create a Subscription to install OSSM.
2. Create a ServiceMeshControlPlane to configure OSSM.

When OSSM is installed, it deploys the Istio control-plane.  The Ingress
Operator configures the ServiceMeshControlPlane to enable Gateway API support,
which means that Istio watches Gateway and HTTPRoute objects and configures
Envoy accordingly.  The Ingress Operator also configures the
ServiceMeshControlPlane to enable Istio's "[Automated Deployment](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/#automated-deployment)" feature,
which means that Istio automatically creates an Envoy proxy deployment and a
LoadBalancer-type service for each Gateway.  Suppose the cluster admin creates a
Gateway that specifies a GatewayClass with OpenShift's controller name:

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: Gateway
metadata:
  name: gateway
  namespace: openshift-ingress
spec:
  gatewayClassName: openshift-default
  # ...
```

Then OSSM configures Istio and Envoy and creates a LoadBalancer-type service:

```yaml
apiVersion: v1
kind: Service
metadata:
  labels:
    gateway.istio.io/managed: openshift.io-gateway-controller
  name: gateway
  namespace: openshift-ingress
  ownerReferences:
  - apiVersion: gateway.networking.k8s.io/v1alpha2    # [1]
    kind: Gateway
    name: gateway
spec:
  type: LoadBalancer
  # ...
```
[1]: The `apiVersion` shown here may change in a future version of OSSM as a result of fixing [OCPBUGS-8681](https://issues.redhat.com/browse/OCPBUGS-8681).

The Ingress Operator then configures DNS for this service (see [New Controller
to Manage DNS Records for Gateway Listeners](#new-controller-to-manage-dns-records-for-gateway-listeners)).  With the Ingress Operator and
Service Mesh Operator together configuring a load balancer and DNS, this enables
clients to connect through Envoy to backend applications as specified by the
Gateway and HTTPRoute objects.

Following is the ServiceMeshControlPlane CR that the Ingress Operator creates:

```yaml
apiVersion: maistra.io/v2
kind: ServiceMeshControlPlane
metadata:
  name: openshift-gateway
  namespace: openshift-ingress
spec:
  addons:
    grafana:
      enabled: false
    jaeger:
      name: jaeger
      install: null
    kiali:
      enabled: false
    prometheus:
      enabled: false
  gateways:
    egress:
      enabled: false
    ingress:
      enabled: true
      ingress: true
      service:
        type: LoadBalancer
  mode: ClusterWide
  policy:
    type: Istiod
  profiles:
  - default
  proxy:
    accessLogging:
      envoyService:
        enabled: true
      file:
        name: /dev/stdout
  runtime:
    components:
      pilot:
        container:
          env:
            PILOT_ENABLE_GATEWAY_API: "true"
            PILOT_ENABLE_GATEWAY_API_DEPLOYMENT_CONTROLLER: "true"
            PILOT_ENABLE_GATEWAY_API_STATUS: "true"
            PILOT_GATEWAY_API_CONTROLLER_NAME: openshift.io/gateway-controller
            PILOT_GATEWAY_API_DEFAULT_GATEWAYCLASS: openshift-default
  security:
    manageNetworkPolicy: false
  tracing:
    type: None
  version: v2.4
```

The above configuration enables Gateway API with a cluster-scoped control-plane.
This means that OSSM watches Gateway API resources in all namespaces.  The above
configuration also enables the ingress proxy for service mesh; a namespace can
subsequently be added to the mesh using ServiceMeshMember and
ServiceMeshMemberRoll CRs following existing practices (see [Adding services to
a service mesh](https://docs.openshift.com/container-platform/4.12/service_mesh/v2x/ossm-create-mesh.html) and [Managing users and profiles](https://docs.openshift.com/container-platform/4.12/service_mesh/v2x/ossm-profiles-users.html#ossm-control-plane-profiles_ossm-profiles-users) in the Service Mesh
product documentation).

#### DNS Controller Changes

The Ingress Operator defines a DNSRecord CRD and a DNS controller that
reconciles DNSRecords.  Before this enhancement, this controller required that
each DNSRecord have an associated IngressController and be in the
"openshift-ingress-operator" namespace, alongside its IngressController.

To add DNS management for Gateway API, the DNS controller must allow a DNSRecord
to be associated with a Gateway (as opposed to an IngressController) and be in
the same namespace (i.e. "openshift-ingress") as its Gateway.  This is a
relatively minor change.

During testing of this enhancement, a separate issue was discovered that
sometimes caused problems when a Gateway was deleted and the DNS controller
subsequently attempted to delete the associated DNS records in Route 53 on AWS.
Normally, to delete a DNS record in Route 53, the DNS controller looks up the
target hosted zone id of the ELB associated with the DNS name.  However, Istio
sometimes deletes the ELB before the DNS controller can delete the DNS record,
and so the ELB no longer exists when the controller needs to look up the ELB's
target hosted zone.  To address this issue, the DNS controller was changed to
store the ELB's target hosted zone id in an annotation on the associated
DNSRecord CR when upserting a DNS record in Route 53.  If a subsequent lookup of
this id fails for any reason, the controller now falls back to using the
annotation value.  This change not only resolves the issue for Gateways but also
makes DNS record deletion more robust on AWS for IngressControllers.

#### New Controller to Manage DNS Records for Gateway Listeners

This enhancement adds a new controller to manage DNS records for the host names
specified in Gateway listeners.  For example, suppose a Gateway has the
following definition (some extraneous details are abbreviated):

```yaml
apiVersion: gateway.networking.k8s.io/v1beta1
kind: Gateway
metadata:
  name: example-gateway
  namespace: openshift-ingress
spec:
  listeners:
  - name: stage-http
    hostname: "*.stage.example.com"
    port: 80
  - name: stage-https
    hostname: "*.stage.example.com"
    port: 443
  - name: prod-https
    hostname: "*.prod.example.com"
    port: 443
```

In this configuration, the Gateway defines three listeners with two different
host names: `*.stage.example.com` and `*.prod.example.com`, as well as two
different TCP ports: 80 and 443.  For this Gateway, Istio creates the following
service (again, some extraneous details are abbreviated in the following
example):

```yaml
apiVersion: v1
kind: Service
metadata:
  labels:
    gateway.istio.io/managed: openshift.io-gateway-controller
  name: example-gateway
  namespace: openshift-ingress
  ownerReferences:
  - apiVersion: gateway.networking.k8s.io/v1alpha2
    kind: Gateway
    name: example-gateway
spec:
  ports:
  - name: status-port
    port: 15021
  - name: stage-http
    port: 80
  - name: stage-https
    port: 443
  selector:
    istio.io/gateway-name: example-gateway
  type: LoadBalancer
status:
  loadBalancer:
    ingress:
    - hostname: lb.example.com
```

Note that the service defines each port once.  When multiple listeners specify
the same port, Istio names the service port using the name of the first listener
that specifies the port.  Multiplexing different host names using the same port
number is done using TLS SNI or the HTTP `host` request header.  In addition,
Istio adds a port named "status-port", which load balancers can use for health
checks.  Istio automatically deletes the service when the Gateway is deleted.

Note that making this service configurable is a non-goal for dev preview.
Options such as service type, whether to use PROXY protocol, timeouts, and other
cloud-specific parameters may be made configurable as a follow-up to this
enhancement in tech preview; see
["Do we need a new config object? How can the cluster admin configure OSSM?"](#do-we-need-a-new-config-object--how-can-the-cluster-admin-configure-ossm).

Then this new controller creates a DNSRecord for each host name, targeting the
service load-balancer, similar to the following:

```yaml
apiVersion: v1
items:
- apiVersion: ingress.operator.openshift.io/v1
  kind: DNSRecord
  metadata:
    annotations:
      ingress.operator.openshift.io/target-hosted-zone-id: Z1H1FL5HABSF5
    labels:
      gateway.istio.io/managed: openshift.io-gateway-controller
      istio.io/gateway-name: example-gateway
    name: example-gateway-57b76476b6-wildcard
    namespace: openshift-ingress
    ownerReferences:
    - apiVersion: v1
      kind: Service
      name: example-gateway
  spec:
    dnsName: '*.stage.example.com'
    targets:
    - lb.example.com
- apiVersion: ingress.operator.openshift.io/v1
  kind: DNSRecord
  metadata:
    annotations:
      ingress.operator.openshift.io/target-hosted-zone-id: Z1H1FL5HABSF5
    labels:
      gateway.istio.io/managed: openshift.io-gateway-controller
      istio.io/gateway-name: example-gateway
    name: example-gateway-5bfc88bc87-wildcard
    namespace: openshift-ingress
    ownerReferences:
    - apiVersion: v1
      kind: Service
      name: example-gateway
  spec:
    dnsName: '*.prod.example.com'
    targets:
    - lb.example.com
```

The DNSRecord CRs are reconciled by the operator's existing DNS controller,
which creates corresponding DNS records using the cloud platform API (e.g. Route
53 on AWS).

To avoid duplicate DNSRecord CRs, the operator hashes the host name and uses the
hashed value as part of the DNSRecord CR's name.  For example, "57b76476b6" is
the hash of "*.stage.example.com", generated similarly to the way [the
Kubernetes deployment controller generates pod template hashes](https://github.com/kubernetes/kubernetes/blob/3f6738b8e6c7f3412ec700c757ae22460cf73e1b/pkg/controller/controller_utils.go#L1157-L1172), using the
following function:

```go
func Hash(dnsName string) string {
	hasher := fnv.New32a()
	hasher.Write([]byte(dnsName))
	return rand.SafeEncodeString(fmt.Sprint(hasher.Sum32()))
}
```

To ensure that DNS records are removed when the corresponding listeners are
removed, the DNSRecord CR specifies an owner reference on the service, and the
controller removes any DNSRecord with such an owner reference and no
corresponding listener.  The DNS controller then deletes the DNS record for the
corresponding DNSRecord CR.

### Workflow Description

This example workflow is based on the examples in [User Stories](#user-stories).

1. The cluster admin creates a GatewayClass with `spec.controllerName: openshift.io/gateway-controller`.
2. The cluster admin creates a TLS secret in the "openshift-ingress" namespace with a `*.gwapi.example.com` wildcard default certificate.
3. The cluster admin creates a Gateway, specifying the desired listeners, the secret from Step 2, and the GatewayClass from Step 1.
4. The project admin creates an HTTPRoute, specifying the desired host name, the Gateway from Step 3, and the target service in the project admin's namespace.
5. The service end-user connects to the host name specified in the HTTPRoute from Step 4 and receives a response from the service that the HTTPRoute targets.

### API Extensions

This enhancement imports the v1beta1 versions of the following CRDs into OpenShift:

* [GatewayClass](https://gateway-api.sigs.k8s.io/api-types/gatewayclass/).
* [Gateway](https://gateway-api.sigs.k8s.io/api-types/gateway/).
* [HTTPRoute](https://gateway-api.sigs.k8s.io/api-types/httproute/).

Before this enhancement, DNSRecord CRs were not reconciled unless they were in
the "openshift-ingress-operator" namespace and had an associated
IngressController.  This enhancement enables reconciliation of DNSRecord CRs in
the "openshift-ingress" namespace.

Tentatively, this enhancement may define an additional CRD for specifying
configuration such as the following:

* Whether Gateway API/ingress is enabled.
* Whether Istio API/mesh is enabled.
* Whether to create a default Gateway.
* Istio parameters, such as profile and addons (Jaeger, Kiali, Grafana, Prometheus).
* OSSM automatic deployments.

See [Open Questions](#open-questions) for some related discussion.

### Implementation Details/Notes/Constraints

We have several constraints related to design and release-timing constraints to consider.

#### Ownership of the ServiceMeshControlPlane

OpenShift Service Mesh is configured through the ServiceMeshControlPlane (SMCP)
CR, both for ingress use-cases and for mesh use-cases.  [The existing
installation procedure for Service Mesh](https://docs.openshift.com/container-platform/4.11/service_mesh/v2x/ossm-create-smcp.html#ossm-control-plane-deploy-cli_ossm-create-smcp) instructs the reader to create a
SMCP named "basic" in the "istio-system" namespace.  To ensure backwards
compatibility, the Ingress Operator must do one of the following:

* Use the same name and namespace: "istio-system/basic".
* Migrate an existing SMCP from "istio-system" to "openshift-ingress".
* Provide an API to tell the Ingress Operator what SMCP to use.

Placement, possible migration, and ownership of the SMCP need to be clarified in
this enhancement.  This is an open design question.

#### Managing Istio's Control Plane Scope

OSSM has traditionally only watched resources in namespaces specified in a
ServiceMeshMemberRoll CR associated with the ServiceMeshControlPlane; however,
OSSM recently added support for a cluster-scoped control plane mode in [OSSM-1320](https://issues.redhat.com/browse/OSSM-1320).
In cluster-scoped mode, Istiod watches all namespaces so that Gateway API objects
will be reconciled in any namespaces, removing the need for ServiceMeshMemberRolls.

Cluster-scoped mode is a Tech Preview feature in OSSM 2.3 and fully supported in
OSSM 2.4.

#### Automated Deployments

When a Gateway resource is created the Istio control-plane triggers the creation
of an underlying Deployment resource to deploy the [Envoy] proxy server, and a
Service (of type LoadBalancer by default) to expose it outside of the cluster.
This is all intended to be opaque to the user as an implementation detail, but if
more information on those implementation details is needed, see [Istio's
documentation on "Automated Deployments"][istio-auto].

Automated deployment (triggered by Gateway creation) is the only supported deployment mechanism in OpenShift 4.19. See [RBAC](#rbac) for more information on which roles are allowed to create Gateways.

[Envoy]:https://github.com/envoyproxy/envoy
[istio-auto]:https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/#automated-deployment

#### Gateway Topology

Users have the option to deploy their Gateways using two distinct topologies: shared gateways or dedicated gateways.
Each topology addresses specific customer requirements, comes with different security implications (see [Security Policy](#security-policy)),
and will be supported in OpenShift.

##### Shared Gateway Topology

In a shared Gateway topology, a Deployment serves routes from multiple
namespaces or with multiple hostnames. The Gateway filters allow xRoutes from application
namespaces using the `spec.listeners.allowedRoutes.namespaces` field. The
Gateway serves multiple hostnames using the `spec.listeners.hostnames` field.
This topology is ideal for multi-tenant environments where dedicated performance
may not be as important.

```mermaid
flowchart TD
    subgraph foo-app namespace
        xRoute
    end
    subgraph bar-app namespace
        xRoute2
    end
    subgraph openshift-ingress namespace
        Gateway
        Service
    end
    Gateway[Gateway] -.-> Service(Service)
    xRoute[xRoute] --> Gateway[Gateway]
    xRoute2[xRoute] --> Gateway[Gateway]
```

> **Note**: The default `spec.listeners.allowedRoutes.namespaces` is `Same`,
> which restricts access to the same namespace, meaning that [Dedicated Gateway
> Topology](#dedicated-gateway-topology) is the default unless otherwise
> configured.

> **Warning**: Setting `spec.listeners.allowedRoutes.namespaces` to `All`, or
> anything other than `Same` has security implications. See the [relevant
> section below](#allowing-all-namespaces-for-gateways) for more details.

Users can have multiple shared Gateways, similar to the concept of [sharding](https://docs.openshift.com/container-platform/latest/networking/configuring_ingress_cluster_traffic/configuring-ingress-cluster-traffic-ingress-controller.html#nw-ingress-sharding_configuring-ingress-cluster-traffic-ingress-controller) in OpenShift. 

##### Dedicated Gateway Topology

In a dedicated Gateway topology, the load balancer Service, its proxy, and its source xRoutes are all deployed in the same namespace.  This setup is ideal for for applications that need a dedicated gateway
to satisfy security or performance requirements.  For example, using this topology enables restrictions on who can use the certificates on a Gateway.

```mermaid
flowchart TD
    subgraph foo-app namespace
        xRoute
        Gateway
        LBService
    end
    Gateway[Gateway] -.-> LBService(LB Service)
    xRoute[xRoute] --> Gateway[Gateway]
```

> **Note**: This is the default topology for created Gateways unless
> `spec.listeners.allowedRoutes.namespaces` is explicitly configured to
> something other than `Same`.

#### Security Policy

##### Allowing All Namespaces for Gateways

Unlike OpenShift ingress, neither Gateway API nor Istio restrict Gateway objects to specific namespaces. This design
allows for different operational groups to create and manage their own [dedicated Gateway](#dedicated-gateway-topology)
in an application namespace rather than relying on a [shared Gateway](#shared-gateway-topology) residing in another
namespace. This departs from the current OpenShift ingress approach of all router pods being confined to the
`openshift-ingress` namespace. Our design will allow Gateways to be created in any namespace, provided the user has
sufficient [RBAC](#rbac) permissions.

###### Limiting DNS Across Namespaces

As mentioned in [New Controller to Manage DNS Records for Gateway Listeners](#new-controller-to-manage-dns-records-for-gateway-listeners),
the Ingress Operator automatically creates DNS records for Gateways. However, for our initial implementation, this
will be limited to the `openshift-ingress` namespace to avoid introducing complexities for DNS management while
Gateway API upstream has no standard for DNS record management (see
[kubernetes-sigs/gateway-api#2627](https://github.com/kubernetes-sigs/gateway-api/issues/2627)).

##### Gateway Merging

Some Gateway API implementations support the concept of  Gateway or listener Merging, i.e. multiple Gateways (and their listeners) are virtually merged to optimize traffic management. Merging can be helpful in several
situations, notably to coalesce multiple listeners together behind a single
proxy or load-balancer to reduce costs.  However, this can be a risk for merges that happen across namespace boundaries.

Istio has historically included a form of Gateway merging by way of what is in
part a side-effect of its [Manual Deployments Option], wherein multiple
Gateways could specify the same `spec.addresses`. This kind of merging poses
potential security risks as one Gateway or listener could preempt another in a
different namespace. As such [we made it possible to disable manual
deployments][istio#55053], and then [disabled it by default in OSSM], and now
consider it unsupported due to safety and maintainability concerns.

As such there is no supported mechanism for merging Gateways with our Gateway
4.19 release. We are tracking and participating in [GEP-1713: ListenerSets -
Standard Mechanism to Merge Multiple Gateways][GEP-1713] in upstream Kubernetes
to make standardized merging available in a future release.

[Manual Deployments Option]:https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/#manual-deployment
[istio#55053]:https://github.com/istio/istio/pull/55053
[disabled it by default in OSSM]:https://github.com/openshift-service-mesh/istio/pull/281
[GEP-1713]:https://gateway-api.sigs.k8s.io/geps/gep-1713/

##### RBAC

OpenShift ships with a set of [default ClusterRoles](https://docs.openshift.com/container-platform/latest/post_installation_configuration/preparing-for-users.html#default-roles_post-install-preparing-for-users)
to enable users to implement RBAC. Gateway API has its own [RBAC recommendation](https://gateway-api.sigs.k8s.io/concepts/security-model/#rbac)
defining which personas get write permission for each object. Gateway API users in OpenShift will need the ability to
implement one of these security models. To support this, the Ingress Operator will add additional RBAC
permissions for the following default ClusterRoles:

| OpenShift ClusterRole | GatewayClass | Gateway              | xRoute Types         | ReferenceGrant |
|-----------------------|--------------|----------------------|----------------------|----------------|
| cluster-admin         | All          | All                  | All                  | All            |
| admin                 | None         | **Get, List, Watch** | **All**              | None           |
| edit                  | None         | **Get, List, Watch** | **All**              | None           |
| view                  | None         | **Get, List, Watch** | **Get, List, Watch** | None           |

For this initial RBAC implementation, write access to Gateways (other than `cluster-admin`) is not included due
to their complexity and potential security risks. However, users can still create custom roles, such as a
`Gateway Operator` role, to enable a role with the ability to write Gateways without requiring super-user privileges.

The following diagram maps the personas in Gateway API's [Simple 3 Tier Model](https://gateway-api.sigs.k8s.io/concepts/security-model/#write-permissions-for-simple-3-tier-model) to potential OpenShift ClusterRoles:
```mermaid
flowchart LR
    InfraOperator[Infrastructure Operator] --> ClusterAdmin(cluster-admin)
    subgraph ClusterRole
        ClusterAdmin
        Admin
        Edit
    end
    subgraph Gateway API Persona
        InfraOperator
        ClusterOperator
        AppDev
    end
    ClusterOperator[Cluster Operator] --> ClusterAdmin(cluster-admin)
    AppDev[Application Developer] --> Admin(admin)
    AppDev[Application Developer] --> Edit(edit)
```

The Infrastructure Operator is generally responsible for installing and configuring the Gateway API provider
(e.g., Istio), installing the Gateway API CRDs, and creating the GatewayClass. However, since the Ingress Operator
handles the installation of OSSM and the CRDs, the Infrastructure Operator is only responsible for creating
and managing the GatewayClass. Cluster Operators create and manage the Gateways and ReferenceGrants, while
Application Developers create and manage the xRoutes.

> **Note**: While initially access to ReferenceGrant will be limited to cluster-admin
> scope due to its security sensitive nature, we will consider and re-evaluate
> the need for more specific scopes for this resource because it could be a useful
> feature not currently available in OpenShift. 

The [Advanced 4 Tier Model](https://gateway-api.sigs.k8s.io/concepts/security-model/#write-permissions-for-advanced-4-tier-model)
is not implementable using the default ClusterRoles. However, as mentioned above, users can create a custom
`Gateway Operator` role and bind it to a specific namespace to implement the Application Admin persona.

### Risks and Mitigations

#### Automatic Deployments

Enabling automated deployments and also allowing arbitrary users to create Gateways would
create a new attack surface for untrusted cluster users.  For this reason, the
default policy allows only cluster admins to create Gateways (see [RBAC](#rbac)).

#### Release Alignment

OpenShift Service Mesh is a separate product from OpenShift Container Platform.
This means that we are limited to using the latest version of OSSM that is
released at the time of OCP's feature-complete date.  This limitation may also
impede shipping bug fixes: If OpenShift's Gateway API support is affected by a
bug in OSSM, then first the bug must be fixed in OSSM, a new version of OSSM
must be released, and only then can OCP update to the OSSM release with the bug
fix.  This increases the need for synchronization between teams and increases
the time between receiving a bug report and getting a bug fix onto production
clusters.

### Drawbacks

OpenShift router is a robust, mature, feature-rich, well understood ingress
solution based on an extremely performant proxy: HAProxy.  In contrast,
OpenShift Service Mesh, Istio, and Envoy are disadvantaged in all these
respects.  However, OpenShift router and OpenShift Service Mesh can co-exist on
the same cluster, and OpenShift router remains the default ingress solution for
the time being.  Moreover, Istio and Envoy enjoy rapid community adoption, as
does Gateway API.  In addition to this, using Istio aligns OpenShift's ingress
solution with Red Hat's investments in Istio for mesh, serverless, and API
gateway use-cases, and using Istio's Gateway API implementation aligns with the
community uptake of Gateway API as the standard lowercase-"i" ingress API.

## Design Details

### Open Questions

As mentioned in the
[User Stories](#user-stories),
[Changes to the Ingress Operator](#controller-to-manage-gateway-api-crds-and-openshift-service-mesh),
[API Extensions](#api-extensions),
[Implementation Details](#implementation-detailsnotesconstraints),
and
[Risks and Mitigations](#risks-and-mitigations)
sections, there are several open questions,
described in the following subsections.

#### Do we need a new config object?  How can the cluster admin configure OSSM?

The ServiceMeshControlPlane CR specifies various configuration parameters that
the cluster admin may care about, such as the following:

* Is Gateway API enabled?
* Are ingress and egress proxies enabled?
* Is logging enabled?
* Are addon components (e.g. Kiali) enabled?

In addition, many parameters related to mesh are configured on the SMCP.  See
[the Service Mesh control plane configuration reference](https://docs.openshift.com/container-platform/4.12/service_mesh/v2x/ossm-reference-smcp.html) for the full list of
parameters, which are referenced in the following text.  For some of these
parameters, the appropriate setting can be inferred:

* If a GatewayClass CR exists, enable Gateway API.
* If Gateway API is enabled and the automated deployments feature is not enabled, configure an ingress proxy.
* Enable the "3scale", "grafana", "kiali", and "prometheus" addons if the corresponding components are installed.
* Enable `telemetry` if the "3scale", "kiali", or "prometheus" addon is enabled.
* Enable `tracing` if the "jaeger" addon is enabled.

Some parameters can be hard-coded:

* `cluster.name` and `cluster.network`.
* `policy`, which appears to be mostly a legacy configuration parameter.
* `profiles`, which provides a way to share and re-use OSSM configuration.
* `proxy`, which configures resource requests and limits for sidecar proxies; requests and limits for cluster components are generally not configurable (see [OpenShift Conventions](https://github.com/openshift/enhancements/blob/master/CONVENTIONS.md#resources-and-limits)).
* `version`, which will be tied to the Ingress Operator release.

For other parameters, the appropriate setting depends on the cluster admin's
intent and cannot be readily inferred:

* `cluster.multiCluster` and `cluster.meshExpansion`.
* `gateways.ingress` and `gateways.egress`, which the cluster admin might want for mesh.
* `general.logging` and `general.validationMessages`.
* `runtime`, which contains component-specific configuration, such as scaling and pod placement.
* `security.manageNetworkPolicy`, which enables management of NetworkPolicy CRs for mesh.
* For `addons`, the cluster admin may or may not want Jaeger enabled; the cluster admin could install Jaeger to diagnose some other SMCP or unrelated component, in which case the mere presence of Jaeger doesn't mean it should be enabled on the SMCP.

The cluster admin can always create a custom SMCP and configure it as desired,
but this SMCP would result in a separate control-plane.  Having a unified
control-plane (i.e. a single Istio control-plane deployment for ingress and
mesh) requires using the same SMCP for both, but with the Ingress Operator
managing the SMCP, it isn't practical to permit the cluster admin to configure
these settings directly on the SMCP.

Hence the question: _Do we need a new config CRD, using which the cluster admin
could configure parameters on the SMCP?_

To resolve this question, we must decide how important it is that the cluster
admin be able to configure these parameters for the SMCP that the Ingress
Operator manages, and we might need to devise some general approach for
customizing these parameters.

For the time being, we may defer the ability to customize the parameters
described above and address the question in a followup EP for tech preview.

**Resolution**: Deferred until tech preview.  For dev preview, we do not allow
customization of the SMCP.  The Ingress Operator specifies a fixed set of
configuration options on the SMCP to enable Gateway API for ingress, with
cluster-wide watches for Gateway API CRs, and also to enable mesh; individual
namespaces can be added to this mesh using ServiceMeshMember and
ServiceMeshMemberRoll CRs.  We will revisit the possibility of allowing the
cluster admin to configure parameters on the SMCP in tech preview once we have
some end-user feedback regarding what parameters end-users want to be able to
customize on the SMCP.

Introducing a new config CRD would lead to another question: _How would we
expose SMCP parameters without duplicating a significant number of definitions
from the SMCP CRD?_

The [Service Mesh control plane profiles](https://docs.openshift.com/container-platform/4.12/service_mesh/v2x/ossm-profiles-users.html#ossm-control-plane-profiles_ossm-profiles-users) is potentially useful to resolve
this last question.  For example, the Service Mesh Operator could provide a set
of profiles covering popular configurations, and the Ingress Operator could
provide an API for the cluster admin to choose a subset of these profiles in
order to configure the SMCP that the Ingress Operator manages.

**Resolution**: Deferred until tech preview.  For dev preview, we hard-code the
"default" profile.  In the future, we may allow the cluster admin to specify
which profiles to enable in order to opt in to or opt out of specific features,
such as mesh and various addons.

#### Do we need to integrate with an existing service-mesh control-plane?

In order to provide a unified control-plane for ingress and mesh, it is
necessary to have a single ServiceMeshControlPlane (SMCP) CR.  In this proposal,
the Ingress Operator configures the Service Mesh Operator by creating an SMCP CR
in the "openshift-ingress" namespace.  In contrast, [the installation procedure
for Service Mesh](https://docs.openshift.com/container-platform/4.11/service_mesh/v2x/ossm-create-smcp.html#ossm-control-plane-deploy-cli_ossm-create-smcp) instructs the reader to create an SMCP named "basic" in the
"istio-system" namespace.  Devising a way for the Ingress Operator to use or
migrate an existing SMCP would be non-trivial and error-prone.

One option would be to add or extend some config CRD to allow the cluster admin
optionally to specify that the Ingress Operator should use an existing SMCP; see
["Do we need a new config object? How can the cluster admin configure OSSM?"](#do-we-need-a-new-config-object--how-can-the-cluster-admin-configure-ossm)
above.

If the Ingress Operator doesn't use or migrate any existing SMCP, then it will
instead install a separate SMCP, which goes against the goal of providing a
unified control-plane.

Hence the question: _Is it necessary to use or migrate an existing SMCP if one
exists, or is it sufficient to provide a unified control-plane only when OSSM
isn't already installed and configured?_

And a follow-up question: _Should we change the documented installation
procedure for Service Mesh to use the Ingress Operator?_

**Resolution**: For dev preview, we will not integrate with an existing SMCP.
TBD for tech preview.

#### Do we need ServiceMeshMember or ServiceMeshMemberRoll CRs?

An earlier version of this enhancement proposal used a ServiceMeshMemberRoll
(SMMR) CR to configure OpenShift Service Mesh (OSSM) to manage Gateway API
resources.  This complicates the use of Gateway API as it requires that some
agent (an operator or controller, the cluster admin, or some other privileged
user) update the SMMR for each namespace with an HTTPRoute that should be
managed by Istio.

Hence the question: _How do we manage ServiceMeshMemberRoll or ServiceMeshMember
CRs for Gateway API?_

We may be able to resolve this issue by specifying the
`spec.techPreview.controlPlaneMode: ClusterScoped` setting on the
ServiceMeshControlPlane CR.  This setting enables cluster-wide watches for
Gateway API resources (but not for Istio resources related to mesh use-cases).
Then Istio manages Gateway API resources for all namespaces with no further
configuration needed.  (ServiceMeshMember and ServiceMeshMemberRoll CRs can
still be created to add a namespace to a mesh.)

**Resolution**: No. We do not need to create ServiceMeshMember or ServiceMeshMemberRoll
CRs. OSSM recently added support for a cluster-scoped control plane mode in [OSSM-1320](https://issues.redhat.com/browse/OSSM-1320)
for both OSSM 2.3.1 and OSSM 2.4. It circumvents the role of these CRs by enabling
cluster-wide watches for Gateway API resources as mentioned above.

#### Can we use the Gateway API v1beta1 CRDs?

OSSM 2.3 is based on Istio 1.14, which recently added Gateway API v1beta1 support,
and OSSM 2.4 is based on Istio 1.16, which also supports Gateway API v1beta1.
Supporting v1beta1 is highly desirable, and OSSM 2.4 brings many other changes
that are of interest for this enhancement. We need to determine whether we will
be able to use OSSM 2.4 for this enhancement. Istio 1.14, the version found in
OSSM 2.3, is now [EOL](https://istio.io/latest/news/support/announcing-1.14-eol-final/)
which is less than desirable for dev preview. 

If OSSM 2.4 is not released in time for OpenShift 4.13, we may resort to using
OSSM 2.3 or a pre-release development build of OSSM 2.4 for dev preview.  While
using a pre-release version is not ideal, OSSM 2.4 has sufficient advantages to
outweigh the disadvantages of using development build in the context of a dev
preview.

**Resolution**: Yes. OSSM 2.3.1 is based on Istio 1.14.5, which has support for
Gateway API v1beta1. OSSM 2.4 is based on Istio 1.16, which also supports Gateway
API v1beta1. More specifically, both OSSM versions support Gateway API v0.5.1.
We will support the v1beta1 CRDs that are promoted in Gateway API v0.5.1.

#### Should we use the "openshift-ingress" namespace for Gateway CRs?

The example ServiceMeshControlPlane and Gateway CRs in this enhancement use the
"openshift-ingress" namespace.  The Ingress Operator already uses this namespace
for OpenShift router resources (deployments, services, etc.).

Using the same namespace simplifies some logic slightly.  However, using a
separate namespace might avoid some confusion between the components on the part
of end users, and it would also increase isolation between components.

Hence the question: _Should we use the "openshift-ingress" namespace for
OpenShift Service Mesh resources and Gateway CRs?_

We may proceed with using the "openshift-ingress" namespace for dev preview and
revisit this design decision ahead of tech preview.

**Resolution**: We will use the "openshift-ingress" namespace for dev preview;
however we will revisit this decision for tech preview.

#### Should we enable automated deployments?

When the Ingress Operator installs OpenShift Service Mesh (OSSM) and creates a
ServiceMeshControlPlane (SMCP) CR, OSSM creates an Envoy proxy named
"istio-ingressgateway".  The cluster admin can then configure a Gateway CR that
uses this Envoy proxy.

Istio has a feature called [automated deployment](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/#automated-deployment) whereby it creates a new
Envoy proxy for a Gateway.

_Should we use automated deployments?_

To answer this question, we need to better understand the behavior and security
implications of automated deployments.

**Resolution**: Yes. Automatic deployments offer a more portable and streamline way to
create [ingress gateways](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/#configuring-ingress-using-a-gateway). However, in dev preview we will not provide a way to
enable/disable Automatic Deployments, it will always be enabled (see resolution to
_Do we need a new config object? How can the cluster admin configure OSSM?_ for more
details.). In tech preview, we may expose a customization option for enabling/disabling
Automatic Deployments.

#### Should we enable Gateway API's [ReferenceGrant](https://gateway-api.sigs.k8s.io/references/spec/#gateway.networking.k8s.io/v1beta1.ReferenceGrant) CRD?

The ReferenceGrant CRD enables an HTTPRoute in one namespace to attach to a
service in another namespace; see [GEP-709: Cross Namespace References from
Routes](https://gateway-api.sigs.k8s.io/geps/gep-709/).  To allow cross-namespace attachment, the service owner must define
a ReferenceGrant that points back to the namespace of the HTTPRoute that
references the service.  This feature is considered "Standard" in Gateway API
v1beta1.

OpenShift's Route API has no such capability.

_Should we enable the ReferenceGrant CRD?_

To answer this question, we need to do the following:

* Discuss the requirement with stakeholders.
* Evaluate Istio's support for this feature.
* Verify that OpenShift Service Mesh will include support for this feature in time for this EP.
* Evaluate any potential security concerns around this feature.

**Resolution**: Yes we will be enabling ReferenceGrant, but only for cluster admins.

Previous versions of OSSM (such as 2.3) did not support ReferenceGrants, and
would even sometimes allow the cross-namespace functionality by default.
However, Gateway API GA will be deployed using OSSM 3.x which _does_ fully
support and enforce it, and therefore so will we.

There are security risks to allowing cross-namespace references. A nefarious user
could send network traffic to locations they would otherwise not have access to via a
confused deputy attack as documented by [CVE-2021-25749](https://github.com/kubernetes/kubernetes/issues/103675).
ReferenceGrant was introduced as a safeguard against these types of attacks by requiring
explicit permission from the target object's owner. The ability to do cross-namespace
references in OSSM 2.3 without any safeguards is a risk; however, OSSM 2.4's requirement
for ReferenceGrants mitigates this risk.

In the future, ReferenceGrant will likely be migrated out of Gateway API and into
Kubernetes upstream. Until then, we will support ReferenceGrant as a part of
Gateway API when using OSSM 3.x.

#### Should we have a feature gate?

With this enhancement, the Ingress Operator installs the gatewayclasses CRD and
watches this resource.  No other logic that this enhancement adds is executed
until a GatewayClass CR is created.  To some degree, the GatewayClass CR behaves as a
feature gate: creating the CR is the equivalent of turning on a feature gate.

_Do we need an explicit feature gate for Gateway API?_

To answer this question, we need to understand the requirements around feature
gates and understand whether just installing the gatewayclasses CRD may be
problematic for some clusters.

**Resolution**: Yes, we should use a feature gate.  It is not just the installation of
the CRDs that must be considered.  The additional controllers and watches needed for
managing Gateway API resources should be avoided if not needed. In addition, having an
official feature gate is useful for setting expectations about whether a cluster is supported
and may be upgraded. In Dev and Tech Preview, a cluster with this feature enabled is not
supported and should not be upgradeable.
Finally, if a user wants to install the Gateway API CRDs on their cluster for experimenting
with an implementation other than OSSM/Istio, we should permit them to do this without
interference from OpenShift or OSSM.

In Dev Preview we choose to use the `CustomNoUpgrade` variety of feature gate primarily
because our other choice was named `TechPreviewNoUpgrade`, and we felt this would cause some
confusion over the status of the feature.  Even though `TechPreviewNoUpgrade` has automatic
CI testing in place and is managed by the OpenShift API, we won't need it until the feature has
graduated in maturity to Tech Preview.

The list of feature gate constraints includes:

* The feature gate should be named `GatewayAPI` for all iterations of the feature.
* The GatewayAPI feature gate should be on by default in GA, but off by default in Dev and Tech Preview.
* The GatewayAPI feature gate should be a `CustomNoUpgrade` kind in Dev Preview,
and a `TechPreviewNoUpgrade` kind in Tech Preview.
* The GatewayAPI feature gate will be used to bar the installation of the GatewayAPI CRDs by the Ingress Operator.
* The GatewayAPI feature gate will be used to bar the installation, configuration, or watch of OpenShift Service Mesh
components by the Ingress Operator.
* The GatewayAPI feature gate will be used to bar special operation of the LoadBalancer and DNS by the Ingress Operator.

### Test Plan

The test plan incorporates the following testable work:
* OpenShift Istio Gateway API and OpenShift Istio Service Mesh (OSSM) can work together to promote operational efficiency.
* Implementation of a Gateway management operator for partial installation automation,  and feature parity with OpenShift Ingress Operator (including DNS and load balancer management).

| ID  | Use Case                                                                                                                                                 | Test Case/s                                                                                                                                                                                                                        |
|-----|----------------------------------------------------------------------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| 1a  | As a user, I want to create, modify, and delete a  GatewayClass, Gateway, HTTPRoute, and ReferenceGrant.                                                 | When the GatewayAPI feature gate is enabled, then a user can  create, modify, and delete a GatewayClass, Gateway, HTTPRoute,  and ReferenceGrant.                                                                                  |
| 1b  | As a user, I want to create, modify, and delete a  GatewayClass, Gateway, HTTPRoute, and ReferenceGrant.                                                 | When an instance is created, the status shows Ready. When an instance is deleted, all supporting objects are also deleted.                                                                                                         |
| 2   | As a user, I want to connect a backend application  to an HTTPRoute in the same namespace.                                                               | A user can connect a backend application to an HTTPRoute in  the same namespace.  The HTTPRoute status is Ready.                                                                                                                   |
| 3   | As a user, I want to be able to access the application behind an HTTPRoute via curl to an HTTP or  edge-terminated HTTPS endpoint.                       | A user can access the application behind an HTTPRoute via curl to an HTTP or edge-terminated HTTPS endpoint.                                                                                                                       |
| 4   | As a user, I want to be able to create a ReferenceGrant to target a service in one namespace from an HTTPRoute in another namespace.                     | A user can create a ReferenceGrant to target a service in one namespace from an HTTPRoute in another namespace.  The HTTPRoute status is Ready.  The user can access the application via curl to an HTTP or edge-terminated HTTPS endpoint. |
| 5   | As a user, I want to be able to view the GatewayClass, Gateway, and HTTPRoute CRDs.                                                                      | Gatewayapi_controller creates the GatewayClass, Gateway, HTTPRoute, and ReferenceGrant CRDs when the GatewayAPI feature gate is enabled.                                                                                           |
| 6   | As a user, I want to be able to view the status of OSSM and SMCP.                                                                                        | Gatewayclass_controller installs OSSM and a ServiceMeshControlPlane, if they don’t already exist, after a GatewayClass owned by OpenShift is created.  The status is Ready for both and logs show no error.                        |
| 7   | As a user, I want to find a DNS record ready after I add a Listener to  a Gateway.                                                                       | When a user creates a Gateway with a Listener, a DNS record is created.  If the Listener is modified or deleted, the DNS record is updated or removed.                                                                             |
| 8   | As a user, I want to query an HTTPRoute by the hostname specified on the Gateway Listener.                                                               | Gateway-service-dns controller creates DNSRecord CRs for each Listener on a Gateway.                                                                                                                                               |
| 9   | As a user, I want to be able to view the status of the load balancer service after creating a Gateway.                                                   | When a user creates a Gateway, a load balancer service is created, status is Ready and logs show no error.                                                                                                                         |
| 10  | As a user, I want to be able to use both regular OpenShift Routes and Ingress Controllers in the presence of a cluster configured for Istio Gateway API. | This is a regression test to verify that regular OpenShift Routes and Ingress Controllers are not affected by the changes made for Istio Gateway API.                                                                              |

### Graduation Criteria

#### Dev Preview -> Tech Preview

TBD.

#### Tech Preview -> GA

TBD.

#### Removing a deprecated feature

TBD.

### Upgrade / Downgrade Strategy

TBD.

### Version Skew Strategy

TBD.

### Operational Aspects of API Extensions

TBD.

* Describe Gateway, GatewayClass, and HTTPRoute status.
* Describe how to get OSSM, Istio, and Envoy logs.
* Describe what the Ingress Operator logs in relation to Gateway API.
* Describe how to inspect the services and dnsrecords resources.
* Describe the most likely configuration mistakes.
* Other?

#### Failure Modes

TBD.

#### Support Procedures

TBD.

## Implementation History

TBD.

## Alternatives

### Using external-dns for DNS management

The Ingress Operator supports DNS through bespoke "DNS provider" implementations
that Red Hat and partners have written over time as new OpenShift releases add
support for new platforms.  At the time of writing, these implementations
include the following:

* Alibaba Cloud DNS, using alibaba-cloud-sdk-go.
* AWS Route 53, using aws-sdk-go.
* Azure DNS, using azure-sdk-for-go.
* Google Cloud DNS, using `google.golang.org/api/dns/v1`.
* IBM CIS DNS and IBM Cloud DNS Services, using IBM's go-sdk-core and networking-go-sdk.

In contrast, [the external-dns
project](https://github.com/kubernetes-sigs/external-dns#status-of-providers)
supports [dozens of
providers](https://github.com/kubernetes-sigs/external-dns#status-of-providers),
and external-dns also has [support for Gateway
API](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/gateway-api.md).
In light of this, it is reasonable to consider using external-dns going forward
instead of relying on the Ingress Operator's more limited and bespoke DNS
provider implementations.

However, using the existing DNS logic has advantages:

* This code is mature and tested with the environments that OpenShift supports.
* The Ingress Operator and its DNS controller are running on the cluster anyway.
* The DNSRecord CRD provides a suitable API for the purposes of this enhancement.
* External-dns may be lacking support for particular configurations that OpenShift supports (e.g. private DNS or special regions).
* Installing external-dns involves installing the [ExternalDNS Operator](https://github.com/openshift/external-dns-operator), which would increase the complexity of this enhancement.

For this reason, this enhancement does not use external-dns.  However, we do
have a story, [NE-1016](https://issues.redhat.com/browse/NE-1016), to
investigate the possibility of using external-dns's Gateway API support for some
future purposes.
