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

* baremetal, Bare Metal, bare metal, BareMetal: follow the [style guide from
metal3-io](https://github.com/metal3-io/metal3-docs/blob/master/design/bare-metal-style-guide.md)

### API

OpenShift APIs follow the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md).
