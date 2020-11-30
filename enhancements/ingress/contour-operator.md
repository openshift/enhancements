---
title: contour-operator
authors:
  - "@danehans"
reviewers:
  - "@Miciah"
  - "@knobunc"
  - "@frobware"
  - "@sgreene570"
approvers:
  - "@knobunc"
creation-date: 2020-08-21
last-updated: 2020-08-26
status: implementable
---
# Contour Operator

## Table of Contents

<!-- toc -->
- [Release Signoff Checklist](#release-signoff-checklist)
- [Summary](#summary)
- [Motivation](#motivation)
  - [Goals](#goals)
  - [Non-Goals](#non-goals)
- [Proposal](#proposal)
  - [User Stories](#user-stories)
  - [Implementation Details](#implementation-details)
  - [Risks and Mitigations](#risks-and-mitigations)
- [Design Details](#design-details)
  - [Open Questions](#open-questions)
  - [Test Plan](#test-plan)
  - [Graduation Criteria](#graduation-criteria)
  - [Upgrade and Downgrade Strategy](#upgradedowngrade-strategy)
  - [Version Skew Strategy](#version-skew-strategy)
- [Implementation History](#implementation-history)
- [Drawbacks](#drawbacks)
- [Alternatives](#alternatives)
- [Infrastructure Needed](#infrastructure-needed)
<!-- /toc -->

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposal is for adding an operator to manage [Contour](https://projectcontour.io/). 
Contour has been chosen for implementing [Service APIs](https://kubernetes-sigs.github.io/service-apis/).
Refer to the [OpenShift Service APIs Project Plan](https://tinyurl.com/y3jwjcp2) for additional background
on why Contour has been selected. The operator will have its own CRD, that will initially expose limited
configuration capabilities. Existing operators will be used to guide the design of the Contour Operator.
Tooling will be included to make it easy to build, run, test, etc. the operator for OpenShift.

## Motivation

The [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) is generally
accepted, and the benefits of operators to OpenShift are well-known.  This enhancement proposal aims
to bring the benefits of operators to Contour. Contour is a controller that will manage Service APIs
and potentially other resources in the future. As with other platform components, Contour requires
an operator to provide lifecycle management of the component.

### Goals

* Explore the use of an operator for managing Contour.
* Create patterns, libraries and tooling so that Contour is of high quality, consistent in its API
surface (common fields on CRDs, consistent labeling of created resources, etc.), yet is easy to build.
* Build an operator that is suitable for production use of Contour. 
  
### Non-Goals

* Replace the functionality of existing operators, i.e.
[Ingress Operator](https://github.com/openshift/cluster-ingress-operator).
* Create an operator that only works with Kubernetes clusters.

## Proposal

The proposal is based on experience gathered and work accomplished with existing OpenShift operators.

* Create a GitHub repository to host the operator source code.
* Leverage frameworks and libraries , i.e. controller-runtime, to simplify development
of the operator.
* Manage dependencies through [Go Modules](https://blog.golang.org/using-go-modules).
* Create user and developer documentation.
* Create tooling to simplify building, running, testing, etc. the operator.
* Create tests that reduce bugs in the code and allow for continuous integration.
* Integrate the operator with the OpenShift toolchain, i.e. openshift/release, required
for productization.
* Provide manifests for deploying the operator.
* Add operator support to [must-gather](https://github.com/openshift/must-gather).
* Add [telemetry support](https://github.com/openshift/cluster-monitoring-operator) for the operator and its operand(s).

The following functionality is expected as part of the operator:

* Introduce a CRD used by the operator to manage Contour and its dependencies. Any API(s) introduced by this enhancement
will follow the Kubernetes API [deprecation policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/).
__Note:__ Although the upstream operator will support projectcontour.io APIs, e.g.
[HTTPProxy](https://projectcontour.io/docs/v1.4.0/httpproxy/), OpenShift will only support Service APIs.
* Common fields in spec that define the configuration of Contour and its dependencies.
* Common fields in spec that define how to deploy Contour, i.e. number of pod replicas.
* Common fields in status that expose the current health & version information
  of Contour.
* The operator will be capable of observing other resources to perform basic sequencing
required for OpenShift integration.

An example can make this easier to understand, here is what a CRD instance for
Contour might look like:

```yaml
apiVersion: projectcontour.io/v1alpha1
kind: Contour
metadata:
  name: default
  namespace: my-contour-namespace
spec:
  replicas: 2
status:
  availableReplicas: 2
  conditions:
  - lastTransitionTime: "2020-08-20T23:01:33Z"
    reason: Valid
    status: "True"
    type: Admitted
  - lastTransitionTime: "2020-08-20T23:07:05Z"
    status: "True"
    type: Available
  - lastTransitionTime: "2020-08-20T23:07:05Z"
    message: The deployment has Available status condition set to True
    reason: DeploymentAvailable
    status: "False"
    type: DeploymentDegraded
```

This manifest creates a Contour deployment that ensures 2 instances of Contour are always
running in the cluster.  Additional `spec` fields may be introduced based on requirements.

### User Stories [optional]

#### Story 1

As a developer, I need the ability to add Service APIs support to OpenShift.

#### Story 2

As a cluster administrator, I need to use Service APIs to provide access to applications
running in my OpenShift cluster.

### Implementation Details [optional]

TBD

### Risks and Mitigations

TBD

## Design Details

TBD

### Open Questions [optional]

1. Should the operator be created upstream to allow for community-based development and support? An
[issue](https://github.com/projectcontour/contour/issues/2187) has been created to address this question
with the Contour developer community.

### Test Plan

- Develop e2e, integration and unit tests.
- Create a CI job to run tests.

### Graduation Criteria

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

##### Removing a deprecated feature

N/A

### Upgrade/Downgrade Strategy

The operator will follow operator best practices for supporting upgrades and downgrades.

### Version Skew Strategy

N/A

## Implementation History

N/A

## Drawbacks

* The amount of time required to build and maintain the operator.
* The amount of time involved in supporting new Kubernetes APIs.

## Alternatives

Use Ingress Operator instead of Contour to implement Service APIs.

## Infrastructure Needed [optional]

A repo to host the operator sourcecode.
