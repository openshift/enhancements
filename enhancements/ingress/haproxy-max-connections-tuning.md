---
title: haproxy-max-connection-tuning
authors:
  - "@frobware"
reviewers:
  - "@Miciah"
  - "@candita"
  - "@gcs278"
  - "@knobunc"
  - "@miheer"
  - "@rfredette"
approvers:
  - "@knobunc"
  - "@miciah"
api-approvers:
  - TBD
creation-date: 2021-04-05
last-updated: 2021-04-05
tracking-link:
  - https://issues.redhat.com/browse/NE-577
see-also:
  - https://github.com/openshift/enhancements/blob/master/enhancements/ingress/haproxy-thread-tuning.md
---

# HAProxy max connection tuning

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Enable cluster administrators to tune the maximum number of simultaneous
connections for OpenShift router deployments.

OpenShift router currently hard-codes the maximum number of
simultaneous connections that HAProxy can handle to 20000, and it has
done so for all OpenShift v4 releases up to and including 4.10. It
should be possible for administrators to tune this value based on the
capability and sizing of their infrastructure nodes. Increasing the
maximum number of simultaneous connections will improve router
throughput at the expense of increased memory usage. Equally,
decreasing the current value may be of interest for Single Node
OpenShift (SNO) deployments. This proposal extends the existing
IngressController API to add a tuning option for max connections.

## Motivation

OpenShift's IngressController implementation is based on HAProxy. For
a given IngressController, OpenShift deploys one or more Pods, each
running an HAProxy instance, which forwards connections for a given
Route to the appropriate backend servers. OpenShift hard-codes each
HAProxy instance to a maximum of 20000 simultaneous connections. New
connections above this threshold are queued until existing connections
are closed. If connections don't close and the maximum is reached then
queued clients will time out.

The capacity and capability of hardware (i.e., RAM and CPU) that
OpenShift is deployed onto has steadily increased yet the value of
`maxconn` has remained a comparatively small constant. Cluster
administrators, intricately aware of both node sizing and their
traffic patterns, should be able to tune this value to maximize the
capabilities of their hardware, particularly where routing has been
configured to run on dedicated infrastructure nodes.

In
[haproxy-thread-tuning.md](https://github.com/openshift/enhancements/blob/master/enhancements/ingress/haproxy-thread-tuning.md)
we exposed a tuning option to increase the number of threads.
Increasing the number of threads has limited value when the maximum
number of simultaneous connections is still capped to 20000, and not
tunable. Having the ability to increase the number of threads and the
number of simultaneous connections will allow greater throughput
without the need for either sharding and/or scaling the number of
replicas per ingresscontroller.

The ability to tune HAProxy's `maxconn` setting has been available to
OpenShift v3 administrators: they can change the environment variable
`ROUTER_MAX_CONNECTIONS` in the router's deploymentconfig at will.
Adding this tuning option to OpenShift v4 restores parity for
customers migrating from OpenShift v3.

### Goals

1. Provide an API for configuring the maximum number of simultaneous
   connections for HAProxy router pods.

2. Leave the default set at 20000 so that we don't perturb the
   behaviour of existing clusters, particularly during upgrades.

### Non-Goals

1. Propose or advise on any new value for
   `spec.tuningOptions.maxConnections` because hardware
   configurations, workloads, and traffic patterns vary wildly from
   cluster to cluster.

2. Changing the default from 20000 to HAProxy's dynamically computed
   value as that will be significantly larger. For clusters deployed
   today, it would automatically bump `maxconn` from 20000 to >520000
   which may yield operational problems for customers during and post
   an upgrade.

## Proposal

Add a new field `maxConnections` to the IngressController API:

```go
// IngressControllerTuningOptions specifies options for tuning the performance
// of ingress controller pods
type IngressControllerTuningOptions struct {
    ...

	// maxConnections defines the maximum number of simultaneous
	// connections that can be established per HAProxy process.
	// Increasing this value allows each ingress controller pod to
	// handle more connections but at the cost of additional
	// system resources being consumed.
	//
	// If this field is empty or 0, the IngressController will use
	// the default value of 20000, but the default is subject to
	// change in future releases. If the value is -1 then HAProxy
	// will dynamically compute a value based on ulimits and the
	// the configuration file when the haproxy process starts.
	// Selecting -1 (i.e., auto) will result in a large value
	// being computed (typically 524259 on OpenShift >=4.10
	// clusters) and therefore each HAProxy process will incur
	// significant memory usage compared to the current default of
	// 20000.
	//
	// +kubebuilder:validation:Optional
	// +optional
	MaxConnections int32 `json:"maxConnections,omitempty"`
}
```

### User Stories

#### Story 1

> As a cluster administrator, I want to increase the number of
> simultaneous connections my cluster can handle without configuring
> IngressController sharding.

The administrator can patch their existing ingress controllers to
increase the number of simultaneous connections. 

For example, patching the `default` ingresscontroller to support
150000 simultaneous connections.

```sh
$ oc patch ingresscontroller/default --type=merge --patch '{"spec":{"tuningOptions":{"maxConnections":150000}}}' -n openshift-ingress-operator
```

Ingress controller pods will roll out with `maxconn` set to 150000.

#### Story 2

> As a cluster administrator, I have a node with large amounts of
> resources (e.g., 128 cores and 1TB RAM) that I would like to handle
> as much of my ingress as possible.

To do this, the administrator can configure
`spec.nodePlacement.nodeSelector` with labels that match the intended
node, as well as configuring `spec.tuningOptions.maxConnections`.

Specifying `spec.tuningOptions.maxConnections: -1` instructs HAProxy
to dynamically compute the largest possible value based on the ulimits
within the container when HAProxy starts. The nature of HAProxy's
dynamic computation also takes into consideration what is configured
in `haproxy.config` at that time.

Additionally, if these are dedicated infrastructure nodes the `ulimit
-n` value (i.e., maximum number of open files) can be increased by
applying a custom tuned profile for those dedicated infrastructure
nodes.

Example: letting HAProxy compute the maximum value:

```sh
$ oc patch ingresscontroller/default --type=merge --patch '{"spec":{"tuningOptions":{"maxConnections":-1}}}' -n openshift-ingress-operator
```

Ingress controller pods will roll out with a new `maxconn` value as
computed by HAProxy.

#### Story 3

> As a cluster administrator, I want to restore the default value for
> max connections.

```sh
$ oc patch ingresscontroller/default --type=merge --patch '{"spec":{"tuningOptions":{"maxConnections":null}}}' -n openshift-ingress-operator
```

Ingress controller pods will roll out with the default setting.

#### Story 4

> As a cluster administrator, I would like to know what the value of
> `maxconn` is when setting `spec.tuningOptions.maxConnections: -1`.

This can be done in two stages:

First set `spec.tuningOptions.maxConnections` to `-1` and let the
router deployment roll out the new pods.

```sh
$ oc patch ingresscontroller/default --type=merge --patch '{"spec":{"tuningOptions":{"maxConnections":-1}}}' -n openshift-ingress-operator
```

We can now extract the computed value from HAProxy's built-in stats
socket:

```sh
$ oc rsh -n openshift-ingress <ROUTER-POD> bash -c 'echo "show info" | socat /var/lib/haproxy/run/haproxy.sock stdio' | grep Maxconn
Maxconn: 524260
```

### Risks and Mitigations

#### Increased Resource Usage

HAProxy builds its data structures ahead of time. If you specify a
large value for `spec.tuningOptions.maxConnections` then that memory
is allocated up-front when the process starts. It is never released.
It's possible that an administrator could set too high a value, given
the node's configuration, causing other pods on the same node to
become resource starved.

WebSocket connections or, more generally, long-lived connections can
exacerbate memory usage for OpenShift router pods. As OpenShift router
reloads, a new HAProxy process is created to run the new
configuration. The current process will not terminate until the
connections it is serving are all closed. In a busy cluster reloads
could occur every 5 seconds and this has the potential to leave a
long-tail of haproxy processes each consuming a significant amount of
memory. This is particularly concerning when the auto-computed value
(i.e., `-1`) is specified. In this scenario the additional memory used
can be as much as 250MB per process.

#### Unsupportable runtime limits

An administrator can set a large value for
`spec.tuningOptions.maxConnections` that cannot be satisfied because
the computation taken by HAProxy doubles the asked-for-value to allow
connections to queue up when the maximum has been reached. This is
known internally to HAProxy as `maxsock`. Specifying
`spec.tuniningOptions.maxConnections: 1048576` yields the following
alert when the OpenShift router pod starts:

```console
$ oc logs -c router router-default-b696bd6cd-5qhqb
  ...
[ALERT] 095/124957 (30) : [/usr/sbin/haproxy.main()] FD limit (1048576) too low for maxconn=1048576/maxsock=2097208.
Please raise 'ulimit-n' to 2097208 or more to avoid any trouble.
This will fail in >= v2.3
```

Here we see HAProxy's computation as "maxsock = 2 * maxconn + 56". The
additional 56 are based on internal HAProxy housekeeping requirements
(e.g., stats port) and the set of configured frontend/backends in the
`haproxy.config` file.

There are two mitigation paths for this scenario:

1. If you are setting extremely large values, always elect to use `-1`
   (i.e., auto) and let HAProxy compute the value based on actual
   ulimits within the running container, its own housekeeping
   requirements, and the `frontend/backend/listen` entries specified
   in `haproxy.config`.

2. If you want an exact value then set a value that is improbably
   large, heed the warning and the suggested fix, then configure a
   tuned profile that would support the suggested `ulimit -n` value.

If, in later releases of OpenShift, we switch to HAProxy v2.4 then
values for `spec.tuningOptions.maxConnections` that cannot be
satisfied at runtime will prevent the router pods from starting until
a compatible value is selected. We are currently using HAProxy 2.2 and
exceeding the limit in the 2.2 series is just a warning.

## Design Details

### Test Plan

#### Test 1

1. Create a new IngressController. Wait for an ingress controller pod
   to be deployed. Verify the router deployment does not have the
   environment variable `ROUTER_MAX_CONNECTIONS` set. New ingress
   controllers without `spec.tuningOptions.maxConnections` should
   default to 20000.

2. Patch the IngressController to set
   `spec.tuningOptions.maxConnections` to 42000. Wait for the ingress
   controller pod to be updated. Verify the router deployment has the
   environment variable `ROUTER_MAX_CONNECTIONS` set to 42000.
   
3. Patch the IngressController to remove
   `spec.tuningOptions.maxConnections`. Wait for the ingress
   controller pod to be updated. Verify the router deployment does not
   have the environment variable `ROUTER_MAX_CONNECTIONS` set.

4. Patch the IngressController to set
   `spec.tuningOptions.maxConnections` to `-1`. Wait for the ingress
   controller pod to be updated. Verify the router deployment has the
   environment variable `ROUTER_MAX_CONNECTIONS` set to `"auto"`;
   openshift-router processing interprets `"auto"` to mean "dynamic"
   and will omit any specification of `maxconn` when writing the
   `haproxy.config` file. With no `maxconn` setting specified, HAProxy
   will dynamically compute a value.

5. Patch the IngressController to remove
   `spec.tuningOptions.maxConnections`. Wait for the ingress
   controller pod to be updated. Verify the router deployment does not
   have the environment variable `ROUTER_MAX_CONNECTIONS` set.

### Graduation Criteria

N/A

### Upgrade / Downgrade Strategy

Upgrading from a previous release that does not have
`spec.tuningOptions.maxConnections` will leave the field blank, which
is an acceptable state. With the field left blank, the default value
of 20000 will be used.

#### Downgrading to a release without `spec.tuningOptions.maxConnections`

If `spec.tuningOptions.maxConnections` is set when downgrading to a
release without the field, the value will be discarded, and the
ingress controller will revert to the previous default of 20000.

### Version Skew Strategy

N/A

## Implementation History

## Drawbacks

## Alternatives

[IngressController sharding](https://docs.openshift.com/container-platform/4.10/networking/configuring_ingress_cluster_traffic/configuring-ingress-cluster-traffic-ingress-controller.html#nw-ingress-sharding-route-labels_configuring-ingress-cluster-traffic-ingress-controller)
