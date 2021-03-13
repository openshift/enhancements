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

OpenShift APIs follow the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md).

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


#### Upgrade and Reconfiguration

* Every component must remain available to consumers without disruption during upgrades and reconfiguration
  * Disruption caused by node restarts is allowed but must respect disruption policies (PodDisruptionBudget) and should be optimized to reduce total disruption
    * Administrators may control the impact to workloads by pausing machine configuration pools until they wish to take outage windows, which prevents nodes from being restarted
  * The kube-apiserver and all aggregated APIs must not return errors, reject connections, or pause for abnormal intervals (>5s) during change
    * API servers may close long running connections gracefully (watches, log streams, exec connections) within a reasonable window (60s)
  * Components that support workloads directly must not disrupt end-user workloads during upgrade or reconfiguration
    * E.g. the upgrade of a network plugin must serve pod traffic without disruption (although tiny increases in latency are allowed)
    * All components that currently disrupt end-user workloads must prioritize addressing those issues, and new components may not be introduced that add disruption


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

Resource requests should be reviewed regularly. Ideally we will build
tools to help us recognize when the requests are far out of line with
the actual resource use of components in CI.

Resource request review history:

* [BZ 1812583](https://bugzilla.redhat.com/show_bug.cgi?id=1812583) --
  to address over-provisioning issues in 4.4
* [BZ 1920159](https://bugzilla.redhat.com/show_bug.cgi?id=1920159) --
  for tracking changes in 4.7/4.8 for single-node RAN

##### Allowed use of limits

There is an exception process for adding limits to payload workloads. In the following
scenarios workloads may be given permission to set limits:

* Workloads that are scale-invariant and have a fixed memory or CPU cost no matter what the scale of the cluster or workload
  * The workload must be demonstrated to use fixed memory or CPU, and the component must have a plan to detect whether the limit causes impact to end user workloads
  * Workloads that have a dynamic range of usage are not a good candidate for fixed limits.
  * Memory limits must be set to 25-50% above the observed maximum in standard e2e runs to ensure additional headroom
  * Examples:
    * A health check sidecar container that performs a very simple request
    * A controller that looks at a fixed set of objects that cannot vary based on cluster size, workload scale, or end user action

In general, per-operator dynamic calculation of limits is discouraged, and instead workloads are expected to regulate their own consumption where possible. In cases where the workload has a very large dynamic range, dynamic sharding or scaling using standard (or per operator workload) is to be preferred over dynamic limit setting.

To get approval for an exception, contact a release architect and an exception will be recorded via code in the standard e2e suite which will allow changes to merge. Non-payload operators are encouraged to seek approval as well to better document use cases - other solutions may be suggested or problems in the platform may be uncovered.


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

An example of an operand that matches the first case is kube-proxy, which is required
for services to work.  An example of an operand that matches the second case is the
DNS node resolver, which adds an entry to the `/etc/hosts` file on all node hosts so
that the container runtime is able to resolve the name of the cluster image registry;
absent this entry in `/etc/hosts`, upgrades could fail to pull images of core
components.

If an operand meets neither of the two conditions listed above, it must not tolerate
all taints.  This constraint is enforced by [a CI test
job](https://github.com/openshift/origin/blob/7d07adcf518a846b898cd0958b85f2daf624476a/test/extended/operators/tolerations.go).
