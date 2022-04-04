---
title: conventions
authors:
  - "@smarterclayton"
  - "@derekwaynecarr"
  - "@jwforres"
  - "@crawford"
  - "@eparis"
  - "@russellb"
  - "@markmc"
reviewers:
  - "@smarterclayton"
  - "@derekwaynecarr"
  - "@jwforres"
  - "@crawford"
  - "@eparis"
  - "@russellb"
  - "@markmc"
approvers:
  - "@smarterclayton"
  - "@derekwaynecarr"
  - "@jwforres"
  - "@crawford"
  - "@eparis"
  - "@russellb"
  - "@markmc"
creation-date: 2020-05-19
last-updated: 2020-05-19
status: provisional
see-also:
replaces:
superseded-by:
---

# OpenShift Conventions

## Summary

This document identifies conventions adopted by the OpenShift project
as a whole. Generally the conventions outlined below should be followed by all
components that make up the project, but exceptions are allowed when necessary.


## Motivation

The conventions are intended to help with consistency across the project. Users
of the platform expect consistency in the experience and operation of the cluster.
Developers of the platform expect consistency in the code to quickly identify issues
across codebases. Consistency enables shared understanding, simplifies explanations,
and reduces accidental complexity.


### Terminology, Grammar, and Spelling

The language we use in the project matters, and should provide a cohesive
experience across web consoles, command line tooling, and documentation.

1. Always use the Oxford comma, also known as the [serial comma](https://en.wikipedia.org/wiki/Serial_comma).
2. When the English language spelling or grammar differs, use the U.S. English version. This is consistent with the [Kubernetes documentation style guide](https://kubernetes.io/docs/contribute/style/style-guide/#language).

#### Naming

We prefer clear and consistent names that describe the concepts in a human friendly, terse, and jargon-free manner for all aspects of the system - components, API and code types, and concepts.  Jargon is discouraged as it increases friction for new users. Where possible reuse or combine words that are part of other names when those concepts overlap.

For instance, the core component that rolls out the desired version of
an OpenShift cluster is called the cluster-version-operator - it is an
"operator" (a term with appropriate context in this domain) that
controls the "version" of the "cluster". Other components reuse this
pattern - this consistency allows a human to infer similarity and
reorient as new or unfamiliar components are introduced over
time. Likewise the API object that drives the behavior of the cluster
related to versions and upgrades is known as `ClusterVersion`
(allowing a human to guess at its function from either direction).

Once you have decided on a name for a new component, follow these conventions:

* Image - images are tagged into ImageStreams to build OpenShift releases, they should be tagged with the component name
  * Operator: [console-operator](https://github.com/openshift/release/blob/0c3425af83f0c4ba0080d3ee2746455396b4bc40/ci-operator/config/openshift/console-operator/openshift-console-operator-master.yaml#L22)
  * Non-operator: [console](https://github.com/openshift/release/blob/0c3425af83f0c4ba0080d3ee2746455396b4bc40/ci-operator/config/openshift/console/openshift-console-master.yaml#L20)
* GitHub repository - repositories should be easily identifiable and match the component name whenever possible. This allows others to find the code for your component among the hundreds of repositories that make up the OpenShift project.
  * Operator: [openshift/console-operator](https://github.com/openshift/console-operator)
  * Non-operator: [openshift/console](https://github.com/openshift/console-operator)
* Git branches: branch maintenance is automated in OpenShift, breaking from convention will make it harder to support your repository
  * `master` is used for active development
  * `release-4.#` for maintenance branches, e.g. `release-4.5`
* API: follow the Kubernetes API [naming conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#naming-conventions)

#### Terms

* OpenShift or openshift, but NEVER Openshift
  * OpenShift - any text intended for user consumption such as display names, descriptions, or documentation
  * openshift - names that use lower case spellings either by requirement or convention, such as namespaces and images
* baremetal, Bare Metal, bare metal, BareMetal: follow the [style guide from
metal3-io](https://github.com/metal3-io/metal3-docs/blob/master/design/bare-metal-style-guide.md)

### API

OpenShift APIs follow the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
with some exceptions and additional guidance outlined in the [OpenShift API conventions](./dev-guide/api-conventions.md).

### Cluster Conventions

A number of rules exist for individual components of the cluster in order to ensure
the overall goals of the cluster can be achieved. These ensure OpenShift is resilient,
reliable, and consistent for administrators.

#### Use Operators To Manage Components

The OpenShift project is an early adopter of, and makes extensive use of, [the
operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) and
it is used to manage all elements of the cluster.

* All components are managed by operators
  * All cluster component operators are expected to deploy, reconfigure, self-heal, and report status about its health back to the cluster
  * The cluster-version-operator (CVO) provides a minimum lifecycle for the core operators that
    are required to get OLM running, and OLM runs all other operators
  * The core operators are updated together and represent the minimum set of function for an OpenShift cluster
  * Any operator that is an optional part of OpenShift or has a different lifecycle than the core OpenShift release must run under OLM
* All configuration is exposed as API resources on cluster
  * This is often described as "core configuration" and is in the "config.openshift.io" API group
  * Global configuration that controls the cluster is grouped by function and are singletons named "cluster" - each of these has a `spec` field that controls configuration of that type for one or more components
    * For example, the `Proxy` object named `cluster` has a field `spec.httpsProxy` that should be used by every component on the cluster to configure HTTP API requests the component may make outside the cluster
  * The `status` field of global configuration contains either dynamic or static data that other components may need to use to properly function
    * A small set of status configuration is set at install time and is immutable (although this set is intended to be small)
  * All other configuration is expected to work like normal Kube objects - changeable at any time, validated, and any failures reported via the status fields of that API
    * Components that read `status` of configuration should continuously poll / watch that configuration for change
* Every component must remain available to consumers without disruption during upgrades and reconfiguration
  * See the next section for more details

#### High Availability

We focus on minimizing the impact of a failure of individual nodes in a cluster by ensuring operators or operands are spread across multiple nodes.
When OpenShift runs in [cluster high availability mode](https://github.com/openshift/enhancements/pull/555), the supported cluster topologies are 3 control-plane nodes and 2 workers, or 3 control-plane nodes that also run workloads (compact clusters).  The following scenarios are intended to support the minimum 2 worker node configuration.  
Please note that in case of [Single Node OpenShift](https://github.com/openshift/enhancements/blob/master/enhancements/single-node/production-deployment-approach.md), since the replicas needed are always 1, there is no need to have affinities set.



* If the operator or operand wishes to be highly available and can tolerate the loss of one replica, the default configuration for it should be
  * Two replicas
  * Hard pod [anti-affinity](https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/#always-co-located-in-the-same-node) on hostname (no two pods on the same node)
  * Use the maxUnavailable rollout strategy on deployments (prefer 25% by default for a value)
  * If the operator or operand requires persistent storage
    * Operators should keep their state entirely in CustomResources, using PV's is discouraged.
    * If and how persistent storage impacts high availability of an operand depends on the cluster configuration, the deployment resource used (e.g. Deployment vs StatefulSet) and available storage classes, e.g. whether [topology-aware volume provisioning](https://kubernetes.io/docs/concepts/storage/storage-classes/#volume-binding-mode) is available.
    * Operands that are deployed as a StatefulSet, i.e. where replicas get a separate volume instead of all replicas sharing a volume, typically require code to sync state between replicas causing cross-zone traffic, which may be charged.
    * Consider that even with a StatefulSet and node-level anti-affinity, persistent storage can create a single point of failure by either provisioning volumes in the same availability zone or preventing some replicas from getting scheduled (e.g. the scheduler finds one good node permutation, but one node can't access sufficient free storage).
  * This is the recommended approach for all the components unless the component needs 3 replicas
* If the operator or operand requires >= 3 replicas and should be running on worker nodes
  * Set soft pod anti-affinity on the hostname so that pods prefer to land on separate nodes (will be violated on two node clusters)
  * Use maxSurge for deployments if possible since the spreading rules are soft

In the future, when we include the descheduler into OpenShift by default, it will periodically rebalance the cluster to ensure spreading for operand or operator is honored. At that time we will remove hard anti-affinity constraints, and recommend components move to a surge model during upgrades.

##### Handling kube-apiserver disruption

kube-apiserver disruption can happen for multiple reasons, including
1. kube-apiserver rollout on a non-HA cluster
2. networking disruption on the host running the client
3. networking disruption on the host running the server
4. internal load balancer disruption
5. external load balancer disruption

We have seen all of these cases, and more, disrupt connections.
Many workloads, controllers, and operators rely on the kube-apiserver for making authentication, authorization, and leader election.
To avoid disruption and mass pod-death, it is important to
1. /healthz, /readyz, /livez should not require authorization.
   They are already open to unauthenticated, so the delegated authorization check presents a reliability risk without
   a security benefit.
   This is now the default in the delegated authorizer in k8s.io/apiserver based servers.
   Controller-runtime does not protect these by default.
2. Binaries should handle mTLS negotiation using the in-cluster client certificate configuration.
   This allows for authentication without contacting the kube-apiserver.
   The canonical case here is the metrics scraper.
   In 4.9, the metrics scraper will support using in-cluster client-certificates to increase reliability of scraping
   in cases of kube-apiserver disruption.
   This is now the default in the delegated authenticator in k8s.io/apiserver based servers.
   Controller-runtime is missing this feature, but it is critical for secure monitoring.
   This gap in controller-runtime should be addressed, but in the short-term kube-rbac-proxy can honor client certificates.
3. /metrics should use a hardcoded authorizer.
   The metrics scraping identity is `system:serviceaccount:openshift-monitoring:prometheus-k8s`
   In OCP, we know that the metrics-scraping identity will *always* have access to /metrics.
   The delegated authorization check for that user on that endpoint presents a reliability risk without a security benefit.
   You can use a construction like: https://github.com/openshift/library-go/blob/7a65fdb398e28782ee1650959a5e0419121e97ae/pkg/controller/controllercmd/builder.go#L254-L259 .
   Controller-runtime missing this feature, but it is critical for reliability.
   Again, in the short term, kube-rbac-proxy can provide static authorization policy.
4. Leader election needs to be able to tolerate 60s worth of disruption.
   The default in library-go has been upgaded to handle this case in 4.9: https://github.com/openshift/library-go/blob/4b9033d00d37b88393f837a88ff541a56fd13621/pkg/config/leaderelection/leaderelection.go#L84
   In essence, the kube-apiserver downtime tolerance is `floor(renewDeadline/retryPeriod)*retryPeriod-retryPeriod`.
   Recommended defaults are LeaseDuration=137s, RenewDealine=107s, RetryPeriod=26s.
   These are the configurable values in k8s.io/client-go based leases and controller-runtime exposes them.
   This gives us
   1. clock skew tolerance == 30s
   2. kube-apiserver downtime tolerance == 78s
   3. worst non-graceful lease reacquisition == 163s
   4. worst graceful lease reacquisition == 26s


#### Upgrade and Reconfiguration

* Every component must remain available to consumers without disruption during upgrades and reconfiguration
  * Disruption caused by node restarts is allowed but must respect disruption policies (PodDisruptionBudget) and should be optimized to reduce total disruption
    * Administrators may control the impact to workloads by pausing machine configuration pools until they wish to take outage windows, which prevents nodes from being restarted
  * The kube-apiserver and all aggregated APIs must not return errors, reject connections, or pause for abnormal intervals (>5s) during change
    * API servers may close long running connections gracefully (watches, log streams, exec connections) within a reasonable window (60s)
  * Components that support workloads directly must not disrupt end-user workloads during upgrade or reconfiguration
    * E.g. the upgrade of a network plugin must serve pod traffic without disruption (although tiny increases in latency are allowed)
    * All components that currently disrupt end-user workloads must prioritize addressing those issues, and new components may not be introduced that add disruption
* All daemonsets of OpenShift components, especially those which are not limited to control-plane nodes, should use the `maxUnavailable` rollout strategy to avoid slow updates over large numbers of compute nodes.
  * Use 33% `maxUnavailable` if you are a workload that has no impact on other workload.
    This ensure that if there is a bug in the newly rolled out workload, 2/3 of instances remain working.
    Workloads in this category include the spot instance termination signal observer which listens for when the cloud signals a node that it will be shutdown in 30s.
    At worst, only 1/3 of machines would be impacted by a bug and at best the new code would roll out that much faster in very large spot instance machine sets.
  * Use 10% `maxUnavailable` in all other cases, most especially if you have ANY impact on user workloads.
    This limits the additional load placed on the cluster to a more reasonable degree during an upgrade as new pods start and then establish connections.

#### Priority Classes

All components should have [priority class](https://kubernetes.io/docs/concepts/configuration/pod-priority-preemption/#pod-priority) associated with them. Without proper
priority class set, the component may be scheduled onto the node very late in the cluster lifecycle causing disruptions to the end-user workloads or it may be
evicted/preempted/OOMKilled last which may not be desirable for some components. When deciding on which priority class to use for your component, please use the following convention:

* If it is fine for your operator/operand to be preempted by user-workload and OOMKilled use `openshift-user-critical` priority class
* If you want your operator/operand not to be preempted by user-workload but still be OOMKilled use `system-cluster-critical` priority class
* If you want operator/operand not be preempted by user-workload or be OOMKilled last use `system-node-critical` priority class

#### Resources and Limits

Kubernetes allows Pods to declare their CPU and memory resource
requirements in advance using [requests and
limits](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/). Requests
are used to ensure minimum resources are provided and to influence
scheduling, while limits prevent Pods from consuming resources
excessively.

Unlike with user workloads, setting limits for cluster components is
problematic for several reasons:

* Components cannot anticipate how they scale in usage in all customer
  environments, so setting one-size-fits-all limits is not possible.
* Setting static limits prevents administrators from responding to
  changes in their clusters’ needs, such as by resizing control plane
  nodes to provide more resources.
* We do not want cluster components to be restarted based on their
  resource consumption (for example, being killed due to an
  out-of-memory condition). We need to detect and handle those cases
  more gracefully, without degrading cluster performance.

Therefore, cluster components *SHOULD NOT* be configured with resource
limits.

However, cluster components *MUST* declare resource requests for both
CPU and memory.

Specifying requests without limits allows us to ensure that all
components receive their required minimum resources and that they are
able to burst to use more resources as needed. When setting the
resource requests, we need to balance the size between values that
ensure the component has the resources it needs to keep running with
the requirement to be efficient and support small-footprint
deployments. If the request settings for a component are too low, it
could be starved for resources in a very busy cluster, resulting in
degraded performance and lower quality of service. If the request
settings for a component are too high, it could mean that the
scheduler cannot find a place to run it, leading to crash looping or
preventing a cluster from deploying. If the combined resource requests
for the cluster components are too high, users may not have enough
resources available to use OpenShift at all, resulting in inability to
run in reasonable footprints for end users (see especially the
[single-node](enhancements/single-node) use cases).

We divide resource types into [two
categories](https://en.wikipedia.org/wiki/System_resource#Categories)
and treat them differently because they have different runtime
characteristics based on the ability of the component to run with less
than a desired minimal resources.  Resources like CPU time or network
bandwidth are considered *compressible*, because if a component has
too little it will run, but be restricted and perform less well.
Resources like memory or storage are considered *incompressible*,
because if a component does not have enough it will fail rather than
being able to run with less.

For incompressible resources, we need to request an amount based on
the minimum that the component will use. For compressible resources,
we are more concerned about balancing between proportional users --
ensuring system or user workloads are not unfairly preferred. If
system compressible requests are too low, user workloads will take
priority and lead to impact to the user workload. If system
compressible requests are too high, some user workloads will not
schedule. We also need to set compressible resource requests to ensure
that in resource constrained situations there is enough proportional
CPU for the overall system to make progress.

Kubernetes resource requests for CPU time are measured in fractional
"[Kubernetes
CPUs](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-cpu)"
expressed using the unit "millicpu" or "millicore", which is 1/1000th
of a CPU. For example, 250m is one quarter of a CPU. For cloud
instances, 1 Kubernetes CPU is equivalent to 1 virtual CPU in the
VM. For bare metal hosts, 1 Kubernetes CPU is equivalent to 1
hyperthread (or 1 real core, if hyperthreading is disabled). In all
cases, the clock speed and other performance metrics for the CPU are
ignored.

Our heuristic for setting CPU resource requests reaches the necessary
balance by prioritizing critical components and allocating resources
to other components based on their actual consumption. There are two
classes of cluster components to consider, those deployed only to the
control plane nodes and those deployed to all nodes.

The etcd database is the backbone of the entire cluster. If etcd
becomes starved for CPU resources, other components like the API
server and controllers that use the API server will suffer cascading
issues that will eventually cause the cluster to fail. Therefore, we
use etcd's requirements as the baseline for a formula for compressible
resources for other components running on the control plane.

The software defined networking (SDN) components run on every node and
represent a high-value, high-resource component. We use the SDN
system’s requirements as the baseline for a formula for other
components running on all nodes.

CPU requests for other components should start by computing a value
proportional to the appropriate baseline, etcd or SDN, using the
formula

```text
floor(baseline_request / baseline_actual * component_actual)
```

Then, these rules for lower and upper bounds should be applied:

* The CPU request should never be lower than 5m. Setting a 5m limit
  avoids extreme ratio calculations when the node is stressed, while
  still representing the noise of a mostly idle workload.
* If the computed value is more than 100m, use the lower of the
  computed value and 200% of the usage of the component in an idle
  cluster. This cap means components that require bursts of CPU time
  may be throttled on busy hosts, but they are more likely to be
  schedulable in the first place.

For example, if etcd requests 100m CPU and uses 600m during an
end-to-end job run and the component being tuned uses 350m on the same
run, the request for the component being tuned should be set to
100m/600m * 350m ~= 58m.

The following PromQL query illustrates how to calculate the adequate CPU request
for all containers in the `openshift-monitoring` namespace:

```PromQL
# CPU request / usage of the SDN container
scalar(
  max(kube_pod_container_resource_requests{namespace="openshift-sdn", container="sdn", resource="cpu"}) /
  max(max_over_time(node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="openshift-sdn", container="sdn"}[120m]))) *
# CPU usage of each container in the openshift-monitoring namespace
max by (pod, container) (node_namespace_pod_container:container_cpu_usage_seconds_total:sum_irate{namespace="openshift-monitoring"})
```

> Please note that pods which run on control-plane nodes must use the etcd container as their baseline.
The example above uses the SDN container for all pods in the `openshift-monitoring` namespace in order to simplify the
Prometheus query. Since the `cluster-monitoring-operator` runs on control plane nodes, its CPU request should be evaluated against the `etcd` container.

Kubernetes resource requests for memory are [measured in
bytes](https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/#meaning-of-memory).

Our heuristic for setting memory requests is based on the typical use
of the component being tuned instead of a ratio of the resources used
by a baseline component, because the memory consumption patterns for
components vary so much and tend to spike. The memory request of
cluster components should be set to a value 10% higher than their 90th
percentile actual consumption over a standard end-to-end suite
run. This gives components enough space to avoid being evicted when a
node starts running out of memory, without requesting so much that
some memory is permanently idle because the scheduler sees it as
reserved even though a component is not using it.

Both CPU and memory request formulas use numbers based on the
end-to-end parallel conformance test job. After running the tests, use
the Prometheus instance in the cluster to query the
`kube_pod_resource_request` and `kube_pod_resource_limit` metrics and
find numbers for the Pod(s) for the component being tuned.

The following PromQL query illustrates how to calculate the difference
between the requested memory and used memory for each container in the
`openshift-monitoring` namespace:
```PromQL
(
  # Calculate the 90th percentile of memory usage over the past hour and add 10% to that
  1.1 * (max by (pod, container) (
    quantile_over_time(0.9, container_memory_working_set_bytes{namespace="openshift-monitoring", container != "POD", container!=""}[60m]))
  ) -
  # Calculate the maximum requested memory per pod and container
  max by (pod, container) (kube_pod_container_resource_requests{namespace="openshift-monitoring", resource="memory", container!="", container!="POD"})
) / 1024 / 1024
```

Resource requests should be reviewed regularly. Ideally we will build
tools to help us recognize when the requests are far out of line with
the actual resource use of components in CI.

Resource request review history:

* [BZ 1812583](https://bugzilla.redhat.com/show_bug.cgi?id=1812583) --
  to address over-provisioning issues in 4.4
* [BZ 1920159](https://bugzilla.redhat.com/show_bug.cgi?id=1920159) --
  for tracking changes in 4.7/4.8 for single-node RAN

#### Taints and Tolerations

An operator deployed by the CVO should run on control plane nodes and therefore should
tolerate the following taint:

* `node-role.kubernetes.io/master`

For example:

```yaml
spec:
  template:
    spec:
      tolerations:
      - key: node-role.kubernetes.io/master
        operator: Exists
        effect: NoSchedule
```

Tolerating this taint should suffice for the vast majority of core OpenShift
operators.  In exceptional cases, an operator may tolerate one or more of the
following taints if doing so is necessary to form a functional Kubernetes node:

* `node.kubernetes.io/disk-pressure`
* `node.kubernetes.io/memory-pressure`
* `node.kubernetes.io/network-unavailable`
* `node.kubernetes.io/not-ready`
* `node.kubernetes.io/pid-pressure`
* `node.kubernetes.io/unreachable`

Operators should not specify tolerations in their manifests for any of the taints in
the above list without an explicit and credible justification.

Operators should never specify the following toleration:
* `node.kubernetes.io/unschedulable`

Tolerating `node.kubernetes.io/unschedulable` may result in the inability to
drain nodes for upgrade operations.

When an operator configures its operand, the operator likewise may specify
tolerations for the aforementioned taints but should do so only as necessary and only
with explicit justification.

Note that the DefaultTolerationSeconds and PodTolerationRestriction admission plugins
may add time-bound tolerations to an operator or operand in addition to the
tolerations that the operator has specified.

If appropriate, a CRD that corresponds to an operand may provide an API to allow
users to specify a custom list of tolerations for that operand.  For examples, see
the
[imagepruners.imageregistry.operator.openshift.io/v1](https://github.com/openshift/api/blob/34f54f12813aaed8822bb5bc56e97cbbfa92171d/imageregistry/v1/types_imagepruner.go#L67-L69),
[configs.imageregistry.operator.openshift.io/v1](https://github.com/openshift/api/blob/34f54f12813aaed8822bb5bc56e97cbbfa92171d/imageregistry/v1/types.go#L82-L84),
[builds.config.openshift.io/v1](https://github.com/openshift/api/blob/34f54f12813aaed8822bb5bc56e97cbbfa92171d/config/v1/types_build.go#L96-L99),
and
[ingresscontrollers.operator.openshift.io/v1](https://github.com/openshift/api/blob/34f54f12813aaed8822bb5bc56e97cbbfa92171d/operator/v1/types_ingress.go#L183-L191)
APIs.

In exceptional cases, an operand may tolerate all taints:

* if the operand is required to form a functional Kubernetes node, or
* if the operand is required to support workloads sourced from an internal or external registry that core components depend upon,

then the operand should tolerate all taints:

```yaml
spec:
  template:
    spec:
      tolerations:
      - operator: Exists
```

Tolerating all taints should be reserved for DaemonSets and static
pods only.  Tolerating all taints on other types of pods may result in the
inability to drain nodes for upgrade operations.

An example of an operand that matches the first case is kube-proxy, which is required
for services to work.  An example of an operand that matches the second case is the
DNS node resolver, which adds an entry to the `/etc/hosts` file on all node hosts so
that the container runtime is able to resolve the name of the cluster image registry;
absent this entry in `/etc/hosts`, upgrades could fail to pull images of core
components.

If an operand meets neither of the two conditions listed above, it must not tolerate
all taints.  This constraint is enforced by [a CI test
job](https://github.com/openshift/origin/blob/7d07adcf518a846b898cd0958b85f2daf624476a/test/extended/operators/tolerations.go).

#### Runlevels

Runlevels in OpenShift are used to help manage the startup of major API groups
such as the kube-apiserver and openshift-apiserver. They indicate that a
namespace contains pods that must be running before the openshift-apiserver.

There are two main levels:

* `openshift.io/run-level: 0` - starting the kube-apiserver
* `openshift.io/run-level: 1` - starting the openshift-apiserver

However, other than the kube-apiserver and openshift-apiserver API groups,
runlevels *SHOULD NOT* be used. As they indicate pods that must be running
before the openshift-apiserver's pods, no Security Context Constraints (SCCs)
are applied.

This is significant as if no SCC is set then any workload running in that
namespace may be highly privileged, a level reserved for trusted workloads.
Early runlevels are used for namespaces containing pods that provide admission
webhooks for workload pods.

Without the SCC restrictions enforced in these namespaces, the power to create
pods in these namespaces are equivalent to root on the node. Security measures
like requiring workloads to run as random uids (a good thing for multi-tenancy
and helping to protect against container escapes) and dropping some capabilities
are never applied.

It is important to note that by
[default](https://github.com/openshift/origin/blob/0104fb51cb31e1f5920b778b17eec8b3286eefee/vendor/k8s.io/kubernetes/openshift-kube-apiserver/admission/namespaceconditions/decorator.go#L11)
there are a number of statically defined namespaces with runlevels set
in OpenShift: `default`, `kube-system`, `kube-public`, `openshift`, `openshift-infra`
and `openshift-node`. These are defined with either runlevel 0 or 1 specified, but as
runlevel 1 is [inclusive](https://github.com/openshift/origin/blob/03a44ceb76961ad9f97df57367be3db1c8e8b792/vendor/k8s.io/kubernetes/openshift-kube-apiserver/admission/namespaceconditions/decorator.go#L16)
 of 0, all don't receive an SCC.

In addition a namespace can also be annotated with the label:

```yaml
labels:
    openshift.io/run-level: "1"
```

Regardless of a given user's permissions, any pod created in these namespaces
will not receive an SCC context. However, a user must be first granted
permissions to create resources in these namespaces (or to create a namespace
with a runlevel) as by default such requests are denied.

*So why are runlevels still being used?*

Historically, in older versions of OCP (4.4) there was a significant delay in
the bootstrapping flow. Meaning that if a component existed in a namespace which
used SCC there would be a delay before it could start. In recent versions of OCP
(4.6+) this delay has been virtually eliminated, the usage of runlevels should
not be required at all hence the primary alternative is to simply try the
workload without any runlevel specified.
