---
title: dynamic-config-manager
authors:
  - "@Miciah"
reviewers:
  - "@frobware"
  - "@candita"
  - "@alebedev87"
approvers:
  - "@candita"
  - "@alebedev87"
api-approvers:
  - "@candita"
  - "@alebedev87"
creation-date: 2024-09-25
last-updated: 2025-10-15
tracking-link:
  - https://issues.redhat.com/browse/RFE-1439
  - https://issues.redhat.com/browse/OCPSTRAT-525
  - https://issues.redhat.com/browse/NE-879
  - https://issues.redhat.com/browse/OCPSTRAT-422
  - https://issues.redhat.com/browse/NE-870
see-also:
replaces:
superseded-by:
---

# OpenShift Router Dynamic Configuration Manager

## Summary

OpenShift 4.18 enables Dynamic Configuration Manager with 1 pre-allocated server
per backend, without blueprint routes, and without any configuration options, as
a **TechPreviewNoUpgrade** feature.  The goal is to deliver a minimum viable
product on which to iterate and eventually enable it in the Default feature set
(that is, GA).  This MVP provides marginal value by avoiding reloading HAProxy
for a single scale-out event or subsequent scale-in event for a route, at
minimal development and operational cost.  More importantly, the MVP gives us CI
signal, enables us to work out defects in DCM, and gives us a starting point
from which to enhance DCM in subsequent OpenShift releases.  In the future, we
intend to extend DCM with capabilities such as adding servers dynamically rather
than pre-allocating them, as well as configuring backends and certificates
dynamically, thereby avoiding reloading HAProxy for most or all updates to
routes or their associated endpoints.

## Motivation

OpenShift router has long suffered from issues related to long-lived connections
and frequent reloads.  The model for updating the router's configuration in
response to route and endpoints updates is to write out a new `haproxy.config`
file and reload HAProxy, which forks a new process and keeps the old process
around until it has closed all the connections that it had open at the time of
the fork.  This fork-and-reload approach has negative implications for
performance, metrics, and balancing.  Foremost, when HAProxy is handling
long-lived connections during repeated configuration updates, old processes
accumulate and use exorbitant amounts of memory.  Additionally, the
fork-and-reload approach reduces accuracy of metrics as the metrics are only
updated for the new process.  For the same reason, the fork-and-reload approach
reduces the accuracy of HAProxy's load balancing algorithms.

The solution to these issues is to configure HAProxy dynamically.  This is
exactly what the Dynamic Configuration Manager (DCM) does: DCM configures a
running HAProxy process through a Unix domain socket.  This means that no
forking is necessary to update configuration and effectuate the changes.

However, DCM requires development work to fix several known issues as well as
extensive testing in order to ensure that enabling it does not introduce
regressions.  DCM was first implemented in OpenShift 3.11 and was never enabled
by default in OpenShift 3 or even allowed as a supported option in OpenShift 4.
This lack of exposure means that DCM now needs extensive testing before we can
be confident that it is safe for production environments.  In addition, DCM in
its present form is difficult to configure, and it has many cases for which it
is not able to handle configuration updates.  When dynamic configuration fails,
DCM falls back to the old fork-and-reload procedure.  Finally, DCM was
implemented for HAProxy 1.8 and does not take full advantage of the capabilities
of newer HAProxy versions.  In sum, DCM requires substantial work to develop and
verify it in order to make it viable.

### User Stories

_As a cluster administrator, I want OpenShift router not to use excessive memory
when the HAProxy configuration changes and the HAProxy process has many
long-lived connections._

Without the Dynamic Config Manager, OpenShift router suffers from a well known
performance issue when the following two conditions are met:

* A router pod reloads its configuration frequently because of route or endpoints updates.
* The same router pod handles long-lived connections.

Reloading the HAProxy configuration involves forking a new process with the new
configuration.  The old process remains open until all connections that were
open at the time of the configuration reload have terminated.  In the case of
long-lived connections, this means that the old process can remain for a long
period of time.  If the router has frequent configuration reloads due to changes
to route configuration, this can cause these old processes to accumulate and use
a large amount of memory, on the order of hundreds of megabytes or multiple
gigabytes per process, ultimately causing out-of-memory errors on the node host.

DCM addresses this issue by reducing the need to reload the configuration.
Instead, DCM configures HAProxy using [Unix Socket
commands](https://docs.haproxy.org/2.8/management.html#9.3), which does not
require forking a new HAProxy process.

The degree to which DCM mitigates this issue is dependent on the nature of the
configuration changes.  Some changes will still require a configuration reload,
but the majority of changes can be performed through HAProxy's control socket.

Initially, DCM will allow scale-out and scale-in of 1 server (pod endpoint) per
backend (route).  Scaling out more than 1 server will still require a fork and
reload initially.  In future iterations of the feature, DCM can be enhanced to
enable scale-out and scale-in of arbitrarily many servers as pods are created
and deleted, and also scale-out and scale-in of backends as routes are created
and deleted, as well as changes to certificates and other route options, all
without requiring a fork and reload.

_As a cluster administrator, I want metrics to be accurate when routes and
endpoints are updated._

OpenShift router does take care to preserve metrics values across reloads in
order to avoid resetting counters to zero when the new process starts.  However,
while values from the old process are preserved, the old process cannot update
metrics after the reload.  For example, the metrics reflect the total number of
bytes that the old process had transferred at the point of the configuration
reload, but if the old process continues transferring data after the new process
starts, the metrics do not reflect the count of any additional bytes
transferred.

Dynamic Configuration Manager addresses this issue again by reducing the need to
reload the configuration and fork a new process.  Again, the degree to which DCM
mitigates this issue is dependent on the nature of the configuration changes.

_As a project administrator, I want HAProxy to balance traffic evenly over old
and new pods when I scale out or scale in my application._

HAProxy tracks the number of connections for each of HAProxy's backend servers.
HAProxy uses this information to balance traffic correctly (that is, evenly)
using the "roundrobin", "random", and "leastconn" balancing algorithms.
However, following a reload, the new HAProxy process does not have data on how
many connections the old processes have to each backend server.  This lack of
data can cause uneven traffic load over backend servers from the aggregate set
of haproxy processes because the new process balances over the set of backend
services without any coordination with the old processes.

Dynamic Configuration Manager addresses this issue again by eliminating the need
to reload the configuration for endpoints changes.  Because adding and removing
servers is possible to do through HAProxy's control socket, DCM is able to
eliminate the problem of uneven load resulting from adding or removing
endpoints.

Note that DCM is not able to prevent imbalance in the event of an endpoints
update combined with an update that requires a configuration reload.  DCM also
cannot prevent imbalance in the case of many router pod replicas with relatively
few connections.  For example, multiple router pod replicas each receiving a
single request for the same route can all choose the same backend server; router
pod replicas do not coordinate with each other.

### Goals

- DCM is enabled as TechPreviewNoUpgrade on OpenShift 4.18 clusters.
- DCM is enabled as Default (GA) on OpenShift 4.21 or later.
- Updating a route's endpoints does not trigger an HAProxy configuration reload.
  - Initially, this might be true only for a single scale-out event, or subsequent scale-in event, per route.
- OpenShift router uses marginally more memory and CPU with DCM than without.
- If DCM cannot handle an update, the router forks and reloads, same as before.
- DCM does not cause regressions in throughput, latency, balancing, or metrics.
- DCM does not require any additional configuration by the end user.
- DCM is a behind-the-scenes implementation detail, invisible to the end user.

### Non-Goals

- DCM cannot handle *all* route or endpoints configuration changes.
  - Initially, adding and removing backends will still require a reload.
  - Initially, changing certificates or annotations will still require a reload.
  - Initially, multiple successive scale-out events will still require a reload.
- DCM cannot handle *router* configuration changes (that is, *global* options).
  - DCM does not handle the `timeout`, `maxconn`, `nbthread`, or `log` options.
  - Updates to the router configuration still generally require a pod restart.
- DCM does not coordinate among router pods.
  - Traffic can still be unbalanced if multiple router pods use the same server.

## Proposal

OpenShift router's Dynamic Configuration Manager (DCM) was implemented in
OpenShift Enterprise 3.11 with HAProxy 1.8.  Although DCM was not previously
enabled in OpenShift Container Platform 4, the implementation was never removed
from the source code.  However, it has not been actively developed by
engineering or tested by QA or in CI in OpenShift 4.  Therefore, this
enhancement proposes the following steps:

1. Manually verify that the router functions and passes E2E tests with DCM enabled.
2. Add a TechPreviewNoUpgrade featuregate in openshift/api for DCM.
3. Update cluster-ingress-operator to enable DCM if the featuregate is enabled.
4. Re-enable old E2E tests in openshift/origin for DCM.
5. Possibly remove outdated logic from DCM.
6. Run E2E, payload tests, and performance tests with DCM enabled.
7. Allow DCM to soak as tech preview for at least 1 OCP release.
8. Possibly add new logic to DCM to exploit new HAProxy 2.y features.
9. Graduate the featuregate to the Default feature set and document DCM as GA.

Steps 5 and 8 are stretch goals and may be done in later OpenShift releases.
The other steps are hard requirements, and we intend to complete them before
graduating the feature to the Default feature set (that is, GA).

Step 9 (GA) will occur in some release to be determined, after OpenShift 4.18.

### Workflow Description

Initially, enabling Dynamic Configuration Manager (DCM) will require enabling a
featuregate.  Ultimately, DCM should be enabled by default. This configuration
should be completely transparent to the end-user.  The only effect should be
that OpenShift router uses less CPU and memory, balances traffic more evenly
after endpoints updates, and tracks metrics more accurately after route and
endpoints updates.

#### Variation and form factor considerations [optional]

DCM should function the same on standalone OpenShift, MicroShift, and
HyperShift.

### API Extensions

DCM does not require any API extensions beyond the featuregate.  However, it
does have an unsupported config override in the IngressController API in case a
critical issue is discovered after DCM has been enabled by default.

### Topology Considerations

#### Hypershift / Hosted Control Planes

There are no unique considerations HyperShift.  The benefits of enabling the
Dynamic Configuration Manager on OpenShift carry over to HyperShift, and the
implementation of DCM is not affected by the split between management cluster
and guest cluster.

#### Standalone Clusters

The change is relevant for standalone clusters.

#### Single-node Deployments or MicroShift

Enabling Dynamic Configuration Manager increases HAProxy's baseline memory
footprint, especially with large thread counts and large numbers of backends.
This is unlikely to be an issue for Single-Node OpenShift given that DCM's
memory impact is negligible with smaller thread counts and fewer backends.
Enabling DCM can also reduce CPU and memory usage by reducing the number of
haproxy processes that must be forked for configuration reloads, which means
that DCM could provide a net improvement in CPU and memory usage on SNO.

This proposal does not affect MicroShift at all.  MicroShift has its own
OpenShift router deployment manifest, which does not enable DCM.

### Implementation Details/Notes/Constraints [optional]

DCM was implemented in OpenShift 3.  This initial implementation requires
pre-allocating both backends and servers in order to accommodate routes and
endpoints that are created after HAProxy starts.

Backends are pre-allocated based on *blueprint routes*, which are route objects
that the cluster-admin must create in a designated namespace.  A blueprint route
must specify the TLS termination type and set of annotations.  Backends and
servers are pre-allocated when HAProxy starts.

If DCM cannot configure something dynamically, it falls back to the old
fork-and-reload procedure.  In particular, the following conditions require
falling back to fork and reload:

- More routes are created than the number of pre-allocated backends.
- More endpoints are created for a route than the number of pre-allocated server slots for that specific route's backend.
- A route is created that does not match the TLS termination type and annotations of any blueprint route.

In OpenShift 3, configuring blueprint routes was left up to the cluster-admin to
do.  As such, blueprint routes constitute a user-facing API.  Additionally, the
cluster-admin could configure the number of server slots to pre-allocate for
each backend.

Note that pre-allocated server slots and backends take up memory.  In a cluster
with many routes, pre-allocating multiple server slots for each backend can take
a considerably large amount of memory.  See
https://gist.github.com/frobware/2b527ce3f040797909eff482a4776e0b for an
analysis of the potential memory impact for different choices of balancing
algorithm, number of threads, number of backends, and number of pre-allocated
server slots per backend.

Based on the test results in the referenced analysis, allocating 1 server slot
per backend had the following impact:

- Negligible impact using the default thread of 4 threads with 100 backends.
- 28% memory increase, from 18 MB to 23 MB, using the default thread count with 1000 backends.  This is the maximum number of backend (applications) we support per router.
- 103% memory increase, from 86 MB to 175 MB, for the worst case of 64 threads with 10000 backends.  This is the maximum configurable number of threads and 10x the supported number of backends.

To avoid operational overhead, avoid adding new APIs, and minimize overhead of
enabling DCM, we will initially omit blueprint routes and only pre-allocate 1
server slot per backend.  This makes DCM an implementation detail; ideally it is
completely invisible to the cluster-admin, except that the router will fork
fewer processes and have more accurate metrics and balancing.  Later, we can
enhance DCM by using newer capabilities in HAProxy 2.y to add arbitrarily many
servers dynamically, add backends dynamically, and update certificates
dynamically.

### Risks and Mitigations

Dynamic Configuration Manager has some known defects:

- [OCPBUGS-7466 No prometheus haproxy metrics present for route created by dynamicConfigManager](https://issues.redhat.com/browse/OCPBUGS-7466)
- [OCPBUGSM-20868 Sticky cookies take long to start working if config manager is enabled](https://issues.redhat.com/browse/OCPBUGSM-20868)
- [NE-1815 Fix implementation gaps discovered during the smoke tests](https://issues.redhat.com/browse/NE-1815)

OCPBUGS-7466 only applies when enabling blueprint routes, which we do not intend
to enable.  We intend to fix the other two known defects before enabling DCM by
default as GA.  However, DCM could have additional, unknown defects.  We will
mitigate this risk through E2E tests, payload tests, and working with partners
to test DCM.

### Drawbacks

#### Memory overhead

DCM also has some up-front memory overhead.  For this reason, DCM will be
initially enabled with a minimum configuration of 1 pre-allocated server slot
per backend and without blueprint routes.  Eventually, DCM will be enhanced to
use newer HAProxy capabilities to perform more configuration without
pre-allocating servers or backends, thus preventing reloads in more cases
without adding any additional up-front memory overhead.

#### Redundancy with respect to Istio/Envoy

DCM is similar in concept to [Envoy
xDS](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration).
Both enable a control plane (openshift-router or Istio) to configure a data
plane (HAProxy or Envoy, respectively) without restarting processes.  Investing
in DCM could be considered a redundant effort when Istio/Envoy already exists
and avoids the problem.  However, many customers depend on OpenShift router for
its performance and reliability and will continue using it indefinitely.  For
these customers, DCM could improve OpenShift router's performance and
reliability without forcing a change in APIs (from Route API to Gateway API) or
proxies (from HAProxy to Envoy).

## Open Questions [optional]

### Can we use HAProxy's control socket to add **certificates** dynamically?

Answer: Yes, using the [`new ssl cert` socket
command](https://www.haproxy.com/documentation/haproxy-runtime-api/reference/new-ssl-cert/).
DCM does not currently use this capability, but we could improve DCM to use it
as a followup improvement in a future OpenShift release.  In the meantime, we
need to document this as a limitation of DCM (not HAProxy).

### Can we use HAProxy's control socket to add **backends** dynamically?

Answer: **No**.  This capability has been discussed upstream but was never
implemented.  There remains a possibility that the capability could be
implemented in some future HAProxy release.  In the meantime, we need to
document this as a limitation of DCM and HAProxy.

### Can we configure Dynamic Configuration Manager with 0 pre-allocated servers?

Answer: **Yes**; see
<https://github.com/openshift/enhancements/pull/1687#discussion_r1844154323>.

This means that as an alternative to enabling DCM with 1 pre-allocated server
slot per backend, our MVP could enable DCM with 0 pre-allocated server slots,
and this configuration would still be useful to avoid reloads for scale-in, or
for scale-out following scale-in, while avoiding the memory impact of
pre-allocating server slots.

### Is the cost of 1 pre-allocated server too high?

Answer: **Probably no, but needs a release note**.

Per https://gist.github.com/frobware/2b527ce3f040797909eff482a4776e0b, the cost
of pre-allocating 1 server slot per backend is down to rounding error for 100
routes, 4 to 9 MB for 1000 routes, and around 40 to 80 MB for 10000 routes.  The
cost is highest if the router has many threads and many backends.  Most likely,
if the router is configured with many threads and backends, it is already
running on big machines.  However, it is important to call out the potential
impact of DCM on memory consumption in a release note.

### Are any of the known defects blockers?

Answer: **Probably yes**.

Enabling Dynamic Configuration Manager should not cause any regressions.  We
need to fix known issues regarding inaccurate metrics and session stickiness and
any other regressions that we find (see [Risks and
Mitigations](#risks-and-mitigations)).

### Do we need an override to turn off Dynamic Configuration Manager?

Answer: **Probably yes**.

Ingress is a critical cluster function, and it is an extremely
performance-sensitive one for some customers.  In case we ship DCM enabled and a
customer finds some critical issue, we need an unsupported config override that
support staff can use to turn off DCM temporarily until we fix the issue.

## Test Plan

Dynamic Configuration Manager will be enabled initially using a
TechPreviewNoUpgrade featuregate to provide CI signal.  We will additionally
revisit existing E2E tests, enable any DCM-related E2E tests that are currently
not enabled, and verify that all enabled router-related E2E tests pass with DCM.
As known defects are fixed and any unknown ones are discovered, we will add
additional tests.  Finally, we will work with partners to verify DCM for their
use-cases.

Expanding test coverage is important both for verifying that DCM is ready for GA
as well as for enabling us to improve DCM in future releases with confidence
that we are not introducing regressions.

## Graduation Criteria

We will introduce the feature as TechPreviewNoUpgrade in OpenShift 4.18, with
the goal of graduating it to GA in a later release.  Further improvements to DCM
will follow in subsequent OpenShift releases.

### Dev Preview -> Tech Preview

N/A.

### Tech Preview -> GA

- All known regressions are fixed.
- Payload tests pass with DCM enabled.
- At least 2 partners or customers provide test results on DCM's function.
- Memory overhead is within acceptable limits.
- Limitations (such as which changes still require reload) are documented.

### Removing a deprecated feature

N/A.

## Upgrade / Downgrade Strategy

The feature requires no specific considerations for upgrade or downgrade;
cluster-ingress-operator will handle upgrading the router image and configuring
Dynamic Configuration Manager, using the standard process for a rolling update of the
router deployment.

## Version Skew Strategy

N/A.

## Operational Aspects of API Extensions

N/A.  The operation of this feature should be transparent to the end user.

## Support Procedures

Metrics and logs are the same as without Dynamic Configuration Manager:

- The `reload_seconds`, `reload_failure`, and `write_config_seconds` metrics remain.
- The router pod logs will report reloads as well as any errors from DCM.
- The `dynamicConfigManager` unsupported config override remains available.

If Dynamic Configuration Manager cannot handle a configuration change, it falls
back to the old fork-and-reload procedure.  Thus in the worst case, OpenShift
router should behave no better or worse with DCM than without DCM.

If an unforeseen defect arises, DCM can be inhibited using an unsupported config
override:

```shell
oc -n openshift-ingress-operator patch ingresscontrollers/default --type=merge --patch='{"spec":{"unsupportedConfigOverrides":{"dynamicConfigManager":"false"}}}'
```

## Implementation History

- OpenShift Enterprise 3.11 added DCM and the `ROUTER_HAPROXY_CONFIG_MANAGER` option ([release notes](https://docs.openshift.com/container-platform/3.11/release_notes/ocp_3_11_release_notes.html#ocp-311-haproxy-enhancements), [documentation](https://docs.openshift.com/container-platform/3.11/install_config/router/default_haproxy_router.html#using-the-dynamic-configuration-manager)).
- OpenShift Container Platform 4.9 added the `dynamicConfigManager` config override, default off ([openshift/cluster-ingress-operator@6a8516a](https://github.com/openshift/cluster-ingress-operator/pull/628/commits/6a8516ab247b00b87a5d7b32e20d4cffcefe1c0f)).
- OpenShift Container Platform 4.18 enables DCM as a TechPreviewNoUpgrade feature.

## Alternatives (Not Implemented)

Following are some alternative solutions to mitigate some of the problems that
the Dynamic Configuration Manager is intended to solve.  These alternatives
generally require interventions by the cluster-admin that have undesirable
side-effects or incur high operational costs to implement on a cluster compared
to enabling DCM.

### Hard-Stop-After Option

OpenShift Container Platform 4.7 introduced the `hard-stop-after` option
([openshift/cluster-ingress-operator@7b7327f](https://github.com/openshift/cluster-ingress-operator/pull/522/commits/7b7327fa5e8a48733549ebe1563afc65a871c527),
[documentation](https://docs.openshift.com/container-platform/4.7/networking/routes/route-configuration.html#nw-route-specific-annotations_route-configuration))).
This option causes OpenShift router to terminate old haproxy processes after the
specified duration following a reload as a workaround to prevent old processes
from accumulating.  This has the critical drawback that terminating an old
process also terminates any connections that it had open.  Additionally, the
duration needs to be tuned to find an acceptable balance between how long
connections are allowed to live and how many processes are permitted to
accumulate.

### Reload Interval

The [reload-interval](./haproxy-reload-interval.md) enhancement added an option
to configure the minimum interval for reloads.  This can be used alone or in
conjunction with hard-stop-after to limit the accumulation of haproxy processes.
However, it has the trade-off of making the router slower to respond to route or
endpoints updates, it has operational overhead, and it only reduces the
accumulation of processes to a limited degree.

### Sharding

The [product documentation advises customers to use sharding](https://docs.openshift.com/container-platform/4.16/scalability_and_performance/optimization/routing-optimization.html)
to avoid having a single router deployment handling too many routes.  We also
advise customers to use sharding to separate mission-critical routes from other
routes, or to separate routes with frequent updates from routes that tend to be
involved with long-lived connections.  However, configuring sharding requires
advance planning, has high operational overhead, and cannot solve the problem
when the same route has frequent updates and long-lived connections.

As an example, https://gist.github.com/Miciah/cc308b717a9e8c9b74d3f97393a5827b
demonstrates how to put platform routes and all other routes in separate shards.

### Alternative proxies, such as Envoy

Envoy implements a gRPC-based protocol called [xDS](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/operations/dynamic_configuration),
which control-planes such as Contour and Istio use to configure Envoy
dynamically, similar to the way DCM uses HAProxy's control socket.  However,
there is no Route API implementation for Contour or Envoy, and migrating
customers from HAProxy is not practical.

## Infrastructure Needed [optional]

This feature requires little specific infrastructure.  Some performance testing
is required, but this should not require any special hardware.
