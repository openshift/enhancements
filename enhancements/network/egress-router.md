---
title: OVN Egress Router Support
authors:
  - "@danielmellado"
  - "@jdesousa"
reviewers:
  - "@squeed"
  - "@danwinship"
  - "@rcarrillocruz"
approvers:
  - "@knobunc"
creation-date: 2020-08-26
last-updated: 2020-08-27
status: provisional
---

# OVN Egress Router Support

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Egress traffic is traffic going from OpenShift pods to external systems,
outside of OpenShift. There are a few  options for enabling egress traffic,
such as allow access to external systems from OpenShift physical node IPs, use
EgressFirewall, EgressIPs or in our case, EgressRouter.

In enterprise environments, egress routers are often preferred. They
allow granular access from a specific pod, group of pods, or project to an
external system or service. Access via node IP means all pods running on a
given node can access external systems.

An egress router is a pod that has two interfaces, (`eth0`) and (e.g. `macvlan0`).
`eth0` is on the cluster network in OpenShift (internal) and macvlan0 has an IP and
gateway from the external physical network.

Pods can access the egress router service thus enabling them to access
external services. The egress router acts as a bridge between pods and an
external system.

Traffic going out the egress router goes via node, but it will have the MAC
address of the macvlan0 interface inside the egress router.

In openshift-sdn, the egress router [was implemented](https://docs.openshift.com/container-platform/3.7/admin_guide/managing_networking.html)
by adding an annotation to allow a pod to request a macvlan interface. In
order to avoid repeating this behavior in ovn-kubernetes, we'd be requesting
such interface using multus to ensure feature-parity with openshift-sdn.

## Motivation

* The egress router images allow people to run containers that have defined
  external addresses.

  Using network policies you can control the scope of who can talk to egress
  routers and thus, when they send traffic outside the cluster it's possible
  to identify the containers in the cluster which are sending it. This allows
  to use external firewalls for auditing and filtering.

  Using multus for requesting the macvlan interface also opens up the
  possibility of use different interface types in the future.

* Ensure that we have images that offer equivalent capabilities to the existing
  egress router images when running ovn-kubernetes.

### Goals

- Create an egress router CNI plugin
- Add feature parity with openshift-sdn's egress router.

## Proposal

## Design Details

We'll be implenting this feature using an additional specific CNI plugin for
it, whose [github repository](https://github.com/openshift/egress-router-cni/)
is already created.

Both kubernetes' pod and multus' net-attach-def are namespaced objects. Due to a
security limitation in OCP, Pod yaml and net-attach-def object should be in the
same namespace, thus egress-router-cni would need to take this into
consideration and put them in the same namespace as well.

The egress router pod would have multus annotations saying to use the
egress-router-cni plugin, and then the CNI plugin would create a macvlan
interface (or, in the future, maybe ipvlan or Amazon ENI).

We're using a CR and a controller in openshift in order to configure both the
pod and the NetworkAttachmentDefinition, so a user would expect to be able to
use both manual and controller-based workflow. All of the controller code will
be handled by Cluster Network Operator (CNO).

### Overview

The `egress-router` plugin creates some sort of
cluster-network-external network interface and assigns a user-provided
static public IP address to it. It is designed for use by the
OpenShift egress-router feature, but may have other uses as well.

### Example configuration

#### Network Attachment Definition

```yaml
apiVersion: "k8s.cni.cncf.io/v1"
kind: NetworkAttachmentDefinition
metadata:
  name: egress-router
  namespace: egress-router-ns
spec:
  config: '{
    "type": "egress-router",
    "ip": {
      "addresses": ["10.10.10.230"],
      }
    }'
```

#### CRD

```yaml
---
apiVersion: network.operator.openshift.io/v1
kind: EgressRouter
metadata:
  name: egress-router
spec:
  addresses: [
    {
      ip: "192.168.3.10/24",
      gateway: "192.168.3.1",
    },
  ]
  allowedDestinations: [
    {
      srcPort: 65,
      destinationCIDR: "203.0.113.25/30",
      protocol: "TCP",
      dstPort: 75,
    },
    {
      destinationCIDR: "203.0.113.26/30",
      srcPort: 65535,
    },
  ]
```

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: egress-file
  namespace: default
data:
  - ip:
      addresses:
        -
      gateway:
      destinations:
        -
```

#### Pod with annotation

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: egress
  labels:
    name: egress
  annotations:
    k8s.v1.cni.cncf.io/networks: egresss-router-ns/egress-router
spec:
  containers:
  - name: openshift-egress
    image:
    securityContext:
      privileged: true
  volumes:
  - name: foo
    configMap:
      name: egressconfigmap
```

### Network configuration reference

The whole set of options can be found at https://github.com/openshift/egress-router-cni/blob/master/README.md

### Interface Types and Platform Support

On bare-metal nodes, `macvlan` and `ipvlan` are supported for
`interfaceType`, with `macvlan` being the default. For `macvlan`,
`interfaceArgs` can include `mode` and `master`, and for `ipvlan` it
can include `master`. However, you do not need to specify `master` if
it can be inferred from the IP address. (That is, if there is exactly
1 network interface on the node whose configured IP is in the same
CIDR range as the pod's configured IP, then that interface will
automatically be used as the `master`, and the associated gateway will
automatically be used as the `gateway`.)

### IP Configuration

The configuration must specify exactly one of `ip`, `podIP`, or `ipConfig`. The first two forms configure IP addresses staticly in the network definition, while `ipConfig` allows dynamic configuration.

The value of `ipConfig` must include at least the name (and optionally
the namespace) of a `ConfigMap` whose `data` must include either an
`ip` entry or a `podIP` entry, in the same format as used by the CNI
configuration. (If there are other fields set in the `ConfigMap` they
will be ignored.) By default, the `ip`/`podIP` value in the
`ConfigMap` will be interpreted just as it would be if it had been in
the CNI config directly. However, if the `ipConfig` specifies
`overrides`, then:

  1. If `overrides.addresses` is set, then the `ConfigMap` is only allowed to assign `addresses` values that are present in `overrides.addresses`.
  2. If `overrides.gateway` is set, then it is used as the default `gateway` value and the `ConfigMap` is not allowed to specify any other value.
  3. If `overrides.destinations` is set, then it is used as the default `destinations` value, and any `destinations` specified in the `ConfigMap` are intersected with it.

### Routing

The newly-created interface will be made the default route for the pod
(with the existing default route being removed). However, the
previously-default interface will still be used as the route to the
cluster and service networks. Additional routes may also be added as
needed. For instance, when using `macvlan`, a route will be added to
the master's IP via the pod network, since it would not be accessible
via the macvlan interface.

### Test Plan

- Unit tests for the feature
- e2e tests covering the feature

### Graduation Criteria

From Tech Preview to GA

#### Tech Preview -> GA

- Ensure feature parity with OpenShift SDN egress router

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History
