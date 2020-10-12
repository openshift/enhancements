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

### Operators

The OpenShift project is an early adopter of, and makes extensive use of, [the
operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/),
and so it is incumbent on us to establish some conventions around operators.

#### Taints and Tolerations

An operator deployed by the CVO should run on master nodes and therefore should
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
