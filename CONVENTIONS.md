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
of the platform expect this consistency in the experience and operation of the cluster.


### Terminology, Grammar, and Spelling

The language we use in the project matters, and should provide a cohesive
experience across web consoles, command line tooling, and documentation.

1. Always use the Oxford comma, also known as the [serial comma](https://en.wikipedia.org/wiki/Serial_comma).
2. When the English language spelling or grammar differs, use the U.S. English version. This is consistent with the [Kubernetes documentation style guide](https://kubernetes.io/docs/contribute/style/style-guide/#language).

#### Naming

* Image 
  * Operator: 
  * Non-operator:
* GitHub Repository
  * Operator: 
  * Non-operator:
* API: follow the Kubernetes API [naming conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md#naming-conventions)

#### Terms

* baremetal, Bare Metal, bare metal, BareMetal: follow the [style guide from
metal3-io](https://github.com/metal3-io/metal3-docs/blob/master/design/bare-metal-style-guide.md)

### API

OpenShift APIs follow the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md).