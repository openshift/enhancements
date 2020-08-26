---
title: externaldns-operator
authors:
  - "@danehans"
reviewers:
  - "@Miciah"
  - "@knobunc"
  - "@frobware"
  - "@sgreene570"
approvers:
  - "@knobunc"
creation-date: 2020-08-26
last-updated: 2020-08-26
status: implementable
---
# ExternalDNS Operator

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

This enhancement proposal is for adding an operator to manage
[ExternalDNS](https://github.com/kubernetes-sigs/external-dns). ExternalDNS has been chosen for managing external DNS
requirements of OpenShift clusters. Initially, the operator will focus on managing external DNS record(s) of OpenShift
[Routes](https://github.com/openshift/api/blob/master/route/v1/types.go). The operator will have its own CRD that will
initially expose limited configuration capabilities. Existing operators will be used to guide the design of the
ExternalDNS Operator. Tooling will be included to make it easy to build, run, test, etc. the operator for OpenShift.

## Motivation

The [operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/) is generally accepted, and the
benefits of operators to OpenShift are well-known.  This enhancement proposal aims to bring the benefits of operators to
ExternalDNS. ExternalDNS is a controller that will manage external DNS records. As with other platform components,
ExternalDNS requires an operator to provide lifecycle management of the component.

### Goals

* Explore the use of an operator for managing ExternalDNS.
* Create patterns, libraries and tooling so that ExternalDNS is of high quality, consistent in its API
surface (common fields on CRDs, consistent labeling of created resources, etc.), yet is easy to build.
* Build an operator that is suitable for production use of ExternalDNS.
* Support the [CRD source](https://github.com/kubernetes-sigs/external-dns/tree/master/docs/contributing/crd-source) and
ExternalDNS [providers](https://github.com/kubernetes-sigs/external-dns/tree/master/provider) relevant to OpenShift.
  
### Non-Goals

* Replace the functionality of existing operators. __Note:__ The ExternalDNS Operator intends to replace external DNS
management provided by existing components, i.e.
[Ingress Operator](https://github.com/openshift/cluster-ingress-operator) in the long-term.
* Create an operator that only works with Kubernetes clusters.
* To support all ExternalDNS [sources](https://github.com/kubernetes-sigs/external-dns/tree/master/source) and 
[providers](https://github.com/kubernetes-sigs/external-dns/tree/master/provider).

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
* Create tooling to simplify ExternalDNS troubleshooting. For example, add operator support to
[must-gather](https://github.com/openshift/must-gather).
* Integrate ExternalDNS with the OpenShift monitoring toolchain. For example, add
[telemetry support](https://github.com/openshift/cluster-monitoring-operator) for the operator and its operand(s).

The following functionality is expected as part of the operator:

* Introduce a CRD used by the operator to manage ExternalDNS and its dependencies.
* Common fields in spec that define the configuration of ExternalDNS and any dependent resources.
* Common fields in spec that define how to deploy ExternalDNS, i.e. number of pod replicas.
* Common fields in status that expose the current health & version information of ExternalDNS.
* The operator will be capable of observing other resources to perform basic sequencing required for OpenShift
integration.

An example can make this easier to understand, here is what a CRD instance for ExternalDNS may look like:

```yaml
apiVersion: operator.externaldns.io/v1alpha1
kind: externaldns
metadata:
  name: default
  namespace: my-externaldns-namespace
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

This manifest creates a deployment that ensures 2 instances of ExternalDNS are always running in the cluster.
Additional `spec` and/or `status` fields may be introduced based on requirements.

* Introduce a CRD for managing DNS records that satisfies the ExternalDNS
[CRD source](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/contributing/crd-source.md). __Note:__ The
[ingressoperator/dnsrecord](https://github.com/openshift/api/blob/master/operatoringress/v1/types.go) CRD does not meet
ExternalDNS CRD source requirements. If the ingressoperator/dnsrecord can be refactored, use this CRD instead of
introducing a new CRD to manage DNS records.
* Common fields in spec that define the desired configuration of an external DNS record.
* Common fields in status that expose the actual state of the DNS record.
* Platform components that require external DNS will use this CRD for managing DNS records.

An example can make this easier to understand, here is what a CRD instance for an external DNS record may look like:

```yaml
apiVersion: externaldns.operator.openshift.io/v1alpha1
kind: dnsrecord
metadata:
  name: default
  namespace: my-component-namespace
spec:
  endpoints:
  - dnsName: foo.example.com
    targets:
    - 1.2.3.4
    recordType: A
    recordTTL: 30
status:
  zones:
  - conditions:
    - lastTransitionTime: "2020-08-26T16:24:41Z"
      message: The DNS provider succeeded in ensuring the record
      reason: ProviderSuccess
      status: "False"
      type: Failed
    dnsZone:
      id: <MY_PROVIDER_ZONE_ID>
```

This manifest creates a DNS A record for hostname "foo.example.com" that resolves to address "1.2.3.4" in the configured
provider. Additional `spec` and/or `status` fields may be introduced based on requirements.

__Note:__ Any API(s) introduced by this enhancement will follow the Kubernetes API
[deprecation policy](https://kubernetes.io/docs/reference/using-api/deprecation-policy/).

### User Stories [optional]

#### Story 1

As a developer, I need the ability to manage DNS records for OpenShift routes.

#### Story 2

As a cluster administrator, I need the ability use a single operator to manage external DNS requirements for all
platform components.

### Implementation Details [optional]

TBD

### Risks and Mitigations

TBD

## Design Details

TBD

### Open Questions [optional]

1. Should the operator be created upstream to allow for community-based development and support?

### Test Plan

- Develop e2e, integration and unit tests.
- Create a CI job to run tests.

### Graduation Criteria

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end-to-end
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
* A single instance of ExternalDNS cannot manage public and private zones for the same domain, i.e.
*.apps.my.openshift.cluster. See
[this](https://github.com/kubernetes-sigs/external-dns/blob/master/docs/tutorials/public-private-route53.md) for
additional details.

## Alternatives

Use Ingress Operator instead of ExternalDNS to manage openshift/route DNS records.

## Infrastructure Needed [optional]

A repo to host the operator sourcecode.
