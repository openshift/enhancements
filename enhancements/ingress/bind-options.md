---
title: bind-options
authors:
  - "@m-yosefpor"
reviewers:
  - "@alebedev87"
  - "@Miciah"
  - "@frobware"
approvers:
  - @knobunc
  - @tjungblu
creation-date: 2021-08-04
last-updated: 2021-08-04
status: implementable
see-also:
  - https://github.com/openshift/cluster-ingress-operator/issues/633
  - https://github.com/openshift/api/issues/964
replaces:
superseded-by:
---

# Ingress Bind Options

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController API to allow the users to
specify bind options in the HostNetwork strategy and enables them to run multiple
instances of the ingress controller on the same node using the HostNetwork strategy, therefore
mitigating port binding conflicts.

## Motivation

When using the HostNetwork strategy for ingress controllers, the default 80, 443, 10443, 10444, 1936 ports on the host are bound by HAProxy for http, https, no_sni, sni and stats correspondingly.
However, those ports might be occupied by other processes (such as another set of ingress controllers), which makes it impossible to run multiple sets of ingress controllers on the same nodes with HostNetwork strategy.
In OpenShift 3.11, it was possible to listen on custom host ports via setting these environment variables in the router's DeploymentConfig ([documentation link](https://docs.openshift.com/container-platform/3.11/architecture/networking/routes.html#env-variables)):

    - `ROUTER_SERVICE_HTTP_PORT=80`
    - `ROUTER_SERVICE_HTTPS_PORT=443`
    - `ROUTER_SERVICE_SNI_PORT=10444`
    - `ROUTER_SERVICE_NO_SNI_PORT=10443`
    - `STATS_PORT=1936`

However there is no option to specify custom ports in `IngressController.operator.openshift.io/v1` object right now.

Having multiple sets of ingress controllers is useful for the router sharding, routers with different policies (public, private), routers with different configuration options, etc.
Running in HostNetwork is a strict requirement in some scenarios (e.g. environments with custom PBR rules). Also routers with the HostNetwork strategy have shown to have better performance (see [OKD4 benchmarks](https://docs.okd.io/latest/scalability_and_performance/routing-optimization.html))

Using custom node selectors to ensure different ingress controllers run on different nodes is not always feasible, when there are not many nodes in the cluster.

### Goals


### Goal

Enable cluster administrators to run multiple set of ingress controllers on the same node with `HostNetwork` endpoint publishing strategy, by configuring the bindings for the following ports:
- HTTP
- HTTPS
- Stats

### Non-Goals

1. Enabling cluster administrators to configure custom ports on the ingress controllers that use any endpoint publishing strategy other than `HostNetwork`.
2. Enabling cluster administrators to configure custom ports on the ingress controllers for `SNI` and `NO_SNI` ports.

The reasons for number 2 to be a non-goal are:

- The sysadmin’s goal is really only to configure alternative ports for 80/443/1936 and to be able to point an external LB to these ports.
- Requiring the sysadmin to configure these additional ports would mean we would need additional documentation to explain why the sysadmin needed to care about something that is really an implementation detail of the HAProxy configuration (and this internal implementation detail could conceivably change, in which case the API would become cruft).
- The larger API surface comes with an increased risk of misconfiguration and port conflicts that an automated solution could prevent.
- Requiring 5 ports instead of just 3 reduces scalability for users who want to run a large number of host-network router pods on the same node.
- Requiring HAProxy-specific configuration makes migration to another proxy (such as Contour) less straightforward.


## Proposal

To enable cluster administrators to configure bind options on the ingress controllers that use the `HostNetwork` endpoint publishing strategy:
the IngressController API is extended by adding an optional `BindOptions` field with type `*IngressControllerBindOptions` to the `HostNetworkStrategy` struct:

    ```go
    // HostNetworkStrategy holds parameters for the HostNetwork endpoint publishing
    // strategy.
    type HostNetworkStrategy struct {
        // ...

        // bindOptions defines parameters for binding haproxy in ingress controller pods.
        // All fields are optional and will use their respective defaults if not set.
        // See specific bindOptions fields for more details.
        //
        //
        // Setting fields within bindOptions is generally not recommended. The
        // default values are suitable for most configurations.
        //
        // +optional
        BindOptions *IngressControllerBindOptions `json:"bindOptions,omitempty"`
    }
    ```

`IngressControllerBindOptions` is the following struct:

    ```go
    // IngressControllerBindOptions specifies options for binding haproxy in ingress controller pods
    type IngressControllerBindOptions struct {
        // Ports are used to set the ports on which the ingress controller will be bound.
        //
        // If unset, the default values are applied as follows:
        //
        //   HTTP: 80
        //   HTTPS: 443
        //   STATS: 1936
        //
        // +kubebuilder:validation:Optional
        // +optional
        Ports *IngressControllerPorts `json:"ports,omitempty"`
    }

    // IngressControllerPorts specifies ports on which ingress controller pods will be bound
    type IngressControllerPorts struct {
        // http defines the port number which HAProxy process binds for
        // http connections. Setting this field is generally not recommended. However in
        // HostNetwork strategy, default http 80 port might be occupied by other processess
        //
        // +kubebuilder:validation:Optional
        // +kubebuilder:validation:Minimum=1
        // +kubebuilder:validation:Maximum=30000
        // +kubebuilder:default:=80
        // +optional
        HTTP int32 `json:"http,omitempty"`

        // https defines the port number which HAProxy process binds for
        // https connections. Setting this field is generally not recommended. However in
        // HostNetwork strategy, default https 443 port might be occupied by other processess
        //
        // +kubebuilder:validation:Optional
        // +kubebuilder:validation:Minimum=1
        // +kubebuilder:validation:Maximum=30000
        // +kubebuilder:default:=443
        // +optional
        HTTPS int32 `json:"https,omitempty"`

        // stats is the port number which HAProxy process binds
        // to expose statistics on it. Setting this field is generally not recommended.
        // However in HostNetwork strategy, default stats port 1936 might
        // be occupied by other processess
        //
        // +kubebuilder:validation:Optional
        // +kubebuilder:validation:Minimum=1
        // +kubebuilder:validation:Maximum=30000
        // +kubebuilder:default:=1936
        // +optional
        Stats int32 `json:"stats,omitempty"`
    }
    ```

The following example configures two ingress controllers with the HostNetwork strategy that can run on the same nodes without port conflicts:

    ```yaml
    ---
    apiVersion: operator.openshift.io/v1
    kind: IngressController
    metadata:
    name: default-1
    namespace: openshift-ingress-operator
    spec:
    endpointPublishingStrategy:
        type: HostNetwork
    ---
    apiVersion: operator.openshift.io/v1
    kind: IngressController
    metadata:
    name: default-2
    namespace: openshift-ingress-operator
    spec:
    endpointPublishingStrategy:
        type: HostNetwork
        hostNetwork:
            bindOptions:
                ports:
                    http: 11080
                    https: 11443
                    stats: 1937
    ```

### Validation

Specifying `spec.endpointPublishingStrategy.type: HostNetwork` and omitting
`spec.endpointPublishingStrategy.hostNetwork` or
`spec.endpointPublishingStrategy.hostNetwork.bindOptions` is valid and implies
the default behavior, which is to use the default ports to bind to. The API validates
that any value provided for the ports defined in
`spec.endpointPublishingStrategy.hostNetwork.bindOptions` is greater than `1`, and lower than `30000` (to avoid conflicts with 30000-32767 nodePort services range).


### User Stories

#### As a cluster administrator, I need to configure bind ports for my IngressController

To satisfy this use-case, the cluster administrator can set the ingress controller's `spec.endpointPublishingStrategy.hostNetwork.bindOptions`.


### API Extensions

This enhancement proposal modifies `IngressController.operator.openshift.io/v1`.

### Implementation Details

Implementing this enhancement requires changes in the following repositories:

* openshift/api
* openshift/cluster-ingress-operator

OpenShift Cluster Ingress Operator, creates a deployment for Router with environment variables for port bindings which OpenShift Router already respects:
`ROUTER_SERVICE_HTTP_PORT`, `ROUTER_SERVICE_HTTPS_PORT`, `STATS_PORT`.

Also the deployments should not have ip:port conflicts for `SNI_PORT` and `NO_SNI_PORT`. There are multiple ways to achieve this:

- The cluster ingress operator configures the router to use a unique loopback address for the sni/nosni frontends. This loopback address would be determined deterministically, for example by hashing the ingresscontroller’s name.
As a result, the sni/nosni port numbers wouldn’t need to change to avoid the conflict as the routers would be using different IP addresses. This can be done with the introduction of two new environment variables for the router:`ROUTER_SERVICE_SNI_IP`, `ROUTER_SERVICE_SNI_IP`, and the configuration of the frontend to bind on this IPs.
The cluster ingress operator sets these environment variables for the router deployment from `127.0.0.1/8` CIDR based on the ingress controller name hash.
- The router gets rid of the loopback hop using a Unix domain socket or other solution, which would have other advantages as well ([router's PR](https://github.com/openshift/router/pull/326)).

### Risks and Mitigations

#### Conflicting ports

Users might define conflicting ports which would cause HAProxy process to fail at startup.

A mitigation to this risk is to implement a validation feature in the reconciliation loop of the Cluster Ingress Operator
to ensure all the ports defined in the `bindOptions` section are unique, and return an error with a meaningful message if there are conflicting ports.

#### Usage of X-Forwarded headers

The default forwarded header policy of the ingress controller is to append X-Forwarded headers. This results into the insertion of `X-Forwarded-Port` header on OpenShift router side (HAProxy acts as a reverse proxy).
If this was the first insertion (L4 load balancer in front of the router) the router's port will be the only one seen by the server (application inside OpenShift).
And if the server relies on `X-Forwarded-*` headers (like OpenShift image registry for instance) this may give a wrong impression that the client used the port set in `X-Forwarded-Port` which in case of the custom port binding may be different.

A mitigation to this risk is to change the forwarded header policy on the ingress controller or on the route level: [documentation](https://docs.openshift.com/container-platform/4.8/networking/ingress-operator.html#nw-using-ingress-forwarded_configuring-ingress).

## Design Details

### Test Plan

The controller that manages the ingress controller Deployment and related
resources has unit-test coverage; for this enhancement, the unit tests are
expanded to cover the additional functionality:

1. Create an ingress controller that enables the `HostNetwork` endpoint publishing strategy type without specifying `spec.endpointPublishingStrategy.hostNetwork.bindOptions`.
2. Verify that the IngressController configures:
    - `ROUTER_SERVICE_HTTP_PORT=80`
    - `ROUTER_SERVICE_HTTPS_PORT=443`
    - `STATS_PORT=1936`

3. Update the ingress controller to specify `spec.endpointPublishingStrategy.hostNetwork.bindOptions`.
4. Verify that the ingress controller updates the router deployment to specify the corresponding values for
`ROUTER_SERVICE_HTTP_PORT`, `ROUTER_SERVICE_HTTPS_PORT`, `STATS_PORT`.


The operator has end-to-end tests; for this enhancement, the following test can
be added:

1. deploy host network ingress controller with the default port bindings
2. deploy another host network controller with the same node placements but with `bindOptions` field set with the ports not conflicting with the first ingress controller
3. check that the second ingress controller becomes ready
4. check that the first ingress controller is still ready

### Graduation Criteria

N/A.

#### Dev Preview -> Tech Preview

N/A.  This feature will go directly to GA.

#### Tech Preview -> GA

N/A.  This feature will go directly to GA.

#### Removing a deprecated feature

N/A.  We do not plan to deprecate this feature.

### Upgrade / Downgrade Strategy

On upgrade, the default configuration remains in effect.

If a cluster administrator upgraded to 4.9, then use different ports for bindOptions, and then downgraded to 4.8, the downgrade would turn all the ingress controller ports with HostNetwork strategy back to default ports.
The administrator would be responsible for making sure there are not any port conflicts when downgrading to OpenShift 4.8.


### Version Skew Strategy

N/A.

### Operational Aspects of API Extensions

N/A.

#### Failure Modes

N/A.

#### Support Procedures

N/A.

## Implementation History

This enhancement is being implemented in OpenShift 4.9.

In OpenShift 3.3 to 3.11 it was possible to use `ROUTER_SERVICE_HTTP_PORT`, `ROUTER_SERVICE_HTTPS_PORT`, `ROUTER_SERVICE_SNI_PORT`, `ROUTER_SERVICE_NO_SNI_PORT`, `STATS_PORT` environment variables.


## Alternatives

N/A


## Drawbacks

N/A
