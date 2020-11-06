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

For instance, the core component that rolls out the desired version of an OpenShift cluster is called the cluster-version-operator - it is an "operator" (a term with appropriate context in this domain) that controls the "version" of the "cluster". Other components reuse this pattern - this consistency allows a human to infer similarity and reorient as new or unfamiliar components are introduced over time. Likewise the API object that drives the behavior of the cluster related to versions and upgrades is known as `ClusterVersion` (allowing a human to guess at its function from either direction).

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

All cluster components must declare resource requests for CPU and memory, and should not describe limits.

The CPU request of components scheduled onto the control plane should be proportional to existing etcd limits for a 6 node cluster running the standard e2e suite (if etcd on a control plane component uses 600m during an e2e run, and requests 100m, and the component uses 350m, then the component should request `100m/600m * 350m`).  Components scheduled to all nodes should be proportional to the SDN CPU request during the standard e2e suite.  

The memory request of cluster components should be set at 10% higher than their p90 memory usage over a standard e2e suite execution.

Limits should be not set because components cannot anticipate how they scale in usage in all customer environments and a crashlooping OOM component is something we must detect and handle gracefully regardless. Setting memory limits leaves administrators with no option to react to valid changes usage dynamically.


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
