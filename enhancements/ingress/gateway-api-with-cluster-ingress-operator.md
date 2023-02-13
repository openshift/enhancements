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
last-updated: 2023-02-17
tracking-link:
  - https://issues.redhat.com/browse/NE-1105
  - https://issues.redhat.com/browse/NE-1107
  - https://issues.redhat.com/browse/NE-1108
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
  selector:                                                         # (1)
    istio: ingressgateway
  gatewayClassName: istio                                           # (2)
  addresses:
  - value: istio-ingressgateway.openshift-ingress.svc.cluster.local # (3)
    type: Hostname
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
[1]: The requirement to specify `selector.istio` is an Istio-ism that needs to be investigated. \
[2]: The requirement to specify `gatewayClassName: istio` is another Istio-ism that is addressed by [istio/istio#38134](https://github.com/istio/istio/issues/38134) and [istio/istio#41884](https://github.com/istio/istio/pull/41884). \
[3]: The requirement to specify the "istio-ingressgateway" service's DNS name in `spec.addresses` is another Istio-ism that needs to be discussed.

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
API CRDs, installing OpenShift Service Mesh Operator, managing a
LoadBalancer-type service for a Gateway, and managing DNS for the same.  This
requires the following changes:

* Refactor the existing service and DNS management logic to enable its re-use.
* Add a new controller to manage Gateway API CRDs and install and configure Service Mesh Operator.
* Add a new controller to manage DNS for Gateways.
* Modify the DNS controller not to require an associated IngressController.

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

#### New Controller to Manage Gateway API CRDs and OpenShift Service Mesh

This enhancement adds a new controller to do the following:

1. Create the gatewayclasses CRD.
2. Reconcile GatewayClasses by installing OSSM and other CRDs when appropriate.

This controller creates the gatewayclasses CRD when it starts, which enables the
cluster admin to create a new GatewayClass.  When the cluster admin does this
and specifies a controller name that OpenShift owns, the Ingress Operator then
does the following:

1. Install the Gateway API CRDs other than the gatewayclasses CRD, namely the gateways and httproutes CRDs.
2. Create a Subscription to install OSSM.
3. Create a ServiceMeshControlPlane to configure OSSM (what if it already exists?; see [Open Questions](#open-questions)).
4. Create a LoadBalancer-type service (or OSSM may create it; see [Open Questions](#open-questions)).
5. Create a DNSRecord for the service (see the following subsection).

The Ingress Operator effectively owns the Gateway API CRDs.  It installs
versions of the CRDs that are compatible with the OSSM version that it installs,
and if the CRDs are already installed, the operator may overwrite them.

When OSSM is installed, it deploys the Istio control-plane and Envoy proxy.  The
Ingress Operator configures the ServiceMeshControlPlane to enable Gateway API
support, which means that Istio watches Gateway and HTTPRoute objects and
configures Envoy accordingly.  Configuring a load balancer and DNS enables
clients to connect through Envoy to backend applications as specified by the
Gateway and HTTPRoute objects.

#### DNS Controller Changes

The Ingress Operator defines a DNSRecord CRD and a DNS controller that
reconciles DNSRecords.  Before this enhancement, this controller required that
each DNSRecord have an associated IngressController and be in the
"openshift-ingress-operator" namespace, alongside its IngressController.

To add DNS management for Gateway API, the DNS controller must allow a DNSRecord
to be associated with a Gateway (as opposed to an IngressController) and be in
the same namespace as its Gateway.  This is a relatively minor change.

#### New Controller to Manage DNS records for Gateway Listeners

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
  addresses:
  - value: istio-ingressgateway.openshift-ingress.svc.cluster.local
    type: Hostname
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

In this configuration, the Gateway specifies that it uses the
"istio-ingressgateway" service and defines three listeners with two different
host names: `*.stage.example.com` and `*.prod.example.com`.  Suppose the
"istio-ingressgateway" has the following service (again, some extraneous details
are abbreviated):

```yaml
apiVersion: v1
kind: Service
metadata:
  name: istio-ingressgateway
  namespace: openshift-ingress
spec:
  ports:
  - name: http
    port: 80
  - name: https
    port: 443
  type: LoadBalancer
status:
  loadBalancer:
    ingress:
    - hostname: lb.example.com
```

Then this new controller creates a DNSRecord for each host name, targeting the
service load-balancer, similar to the following:

```yaml
apiVersion: v1
items:
- apiVersion: ingress.operator.openshift.io/v1
  kind: DNSRecord
  metadata:
    name: example-gateway-9bb4cbc66
    namespace: openshift-ingress
    ownerReferences:
    - apiVersion: v1
      kind: Service
      name: istio-ingressgateway
  spec:
    dnsName: '*.stage.example.com.'
    targets:
    - lb.example.com
- apiVersion: ingress.operator.openshift.io/v1
  kind: DNSRecord
  metadata:
    name: example-gateway-6bbdf64d6c
    namespace: openshift-ingress
    ownerReferences:
    - apiVersion: v1
      kind: Service
      name: istio-ingressgateway
  spec:
    dnsName: '*.prod.example.com.'
    targets:
    - lb.example.com
```

The DNSRecord CRs are reconciled by the operator's existing DNS controller,
which creates corresponding DNS records using the cloud platform API (e.g. Route
53 on AWS).

To avoid duplicate DNSRecord CRs, the operator hashes the host name and uses the
hashed value as part of the DNSRecord CR's name.  For example, "9bb4cbc66" is
the hash of "*.stage.example.com", generated similarly to the way [the
Kubernetes deployment controller generates pod template
hashes](https://github.com/kubernetes/kubernetes/blob/3f6738b8e6c7f3412ec700c757ae22460cf73e1b/pkg/controller/controller_utils.go#L1157-L1172).

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

#### Security Policy

TBD.

#### Automated and Manual Gateway Deployments

OpenShift Service Mesh and Istio have a feature called [automated deployment](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/#automated-deployment)
that creates an Envoy deployment and service in the same namespace for each Gateway
if the Gateway's `spec.addresses` field is left unset. This enables users to seamlessly
create a "shard" per Gateway, much like how each IngressController object is a shard
for routes. It is enabled via the `PILOT_ENABLE_GATEWAY_API_DEPLOYMENT_CONTROLLER`
env variable.

Conversely, with [manual deployments](https://istio.io/latest/docs/tasks/traffic-management/ingress/gateway-api/#manual-deployment),
if the Gateway has the `spec.addresses` field set, then it must manually link
to an [ingress gateway](https://istio.io/latest/docs/tasks/traffic-management/ingress/ingress-control/#configuring-ingress-using-a-gateway).
The user needs to make their own ingress gateway  service and deployment in the same
namespace to manually link to, or they need to use the default ingress gateway (if enabled).

If the ServiceMeshControlPlane's `spec.gateways.ingress.enabled` field is set to `true`,
Istio creates an `istio-ingressgateway` service, in the same namespace as the control plane,
that is a ready-to-use proxy which gateways can be manually linked to. Istio [discourages](https://istio.io/latest/docs/setup/additional-setup/gateway/)
the use of this ingress gateway as it couples the gateway to the control plane.

_To summarize, there are four ways a user could link Gateways to Ingress Gateways:_
1. Use automated deployments
2. Use a manual deployment, manually create a new ingress gateway service and deployment, and link to this service
3. Use a manual deployment and link to the existing `istio-ingressgateway` ingress gateway
4. Use a manual deployment and link to a previously created automated deployment ingress gateway

OpenShift will not inhibit or alter the functionality of automated deployments, except
restricting creation of Gateways to specific users (_see the automated deployment section
in Risks and Mitigations_).

Nor will OpenShift inhibit or alter the functionality of manual deployments. Users
are responsible for understanding and creating links to manual deployments when creating
Gateways.

The choice between these two features depends on the user's desired sharding scheme. Manual
linking, being more expressive, can establish a many-to-one Gateway-to-Gateway-Deployment
relationship, while automated deployments strictly establish a one-to-one
Gateway-to-Gateway-Deployment relationship. Arguably, automated deployments are more portable
among Gateway API implementations due to the fact manually deployments require linking an
Istio-specific service address.

### Risks and Mitigations

#### Automatic Deployments

Enabling automated deployments and also allowing arbitrary users to create Gateways would
create a new attack surface for untrusted cluster users.  For this reason, the
default policy allows only cluster admins to create Gateways.

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

**Resolution**: TBD.

Introducing a new config CRD would lead to another question: _How would we
expose SMCP parameters without duplicating a significant number of definitions
from the SMCP CRD?_

The [Service Mesh control plane profiles](https://docs.openshift.com/container-platform/4.12/service_mesh/v2x/ossm-profiles-users.html#ossm-control-plane-profiles_ossm-profiles-users) is potentially useful to resolve
this last question.  For example, the Ingress Operator could check for a profile
in a well known location, and the cluster admin could put a profile in that
location in order to configure the SMCP that the Ingress Operator manages.

**Resolution**: TBD.

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

**Resolution**: TBD.

#### Should we have a feature gate?

With this enhancement, the Ingress Operator installs the gatewayclasses CRD and
watches this resource.  No other logic that this enhancement adds is executed
until a GatewayClass CR is created.  In effect, the GatewayClass CR behaves as a
feature gate: creating the CR is the equivalent of turning on a feature gate.

_Do we need an explicit feature gate for Gateway API?_

To answer this question, we need to understand the requirements around feature
gates and understand whether just installing the gatewayclasses CRD may be
problematic for some clusters.

**Resolution**: TBD.

### Test Plan

TBD.

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
