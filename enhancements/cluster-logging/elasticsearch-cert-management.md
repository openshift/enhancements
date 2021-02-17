---
title: elasticsearch-cert-management
authors:
  - "@ewolinetz"
reviewers:
  - "@jcantrill"
  - "@bparees"
  - "@alanconway"
  - "@jpkrohling"
approvers:
  - "@jcantrill"
  - "@bparees"
  - "@alanconway"
  - "@jpkrohling"
creation-date: 2021-01-05
last-updated: 2021-01-20
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# elasticsearch-cert-management

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Migration plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Currently to use the Elasticsearch Operator, one would need to generate secrets in a specific manner which creates a high amount of complexity for use.
This proposal seeks to outline a mechanism where the Elasticsearch Operator creates and maintains these certificates and the elasticsearch/kibana secret instead of other operators (e.g. Cluster Logging and Jaeger), and allows an annotation mechanism on secrets for injecting required keys and certificates for mTLS with Elasticsearch.

## Motivation

Currently our managed Elasticsearch cluster requires anything using our operator to generate, manage, and provide certs and secrets for the operator and its cluster to use.
This leads to many potential sources to create certificates and can cause a gap between what the managed Elasticsearch requires and what something may provide.
This would instead make the Elasticsearch Operator a single source of truth for how certificates would be generated.

### Goals
The specific goals of this proposal are:

* Outline the responsibility for the Elasticsearch Operator regarding certificates

* Determine an annotation that the Elasticsearch Operator will watch for on secrets in the same namespace as an existing elasticsearch CR

* Discuss upgrade strategies from the current LTS mechanism.

We will be successful when:

* Users of Elasticsearch Operator no longer need to maintain certificates for Elasticsearch and can have certificates injected into their secrets

* There is a clear path forward for automatic upgrades

### Non-Goals

* This does not seek to implement work to adhere to the cluster level TLS definitions, however this does make the implementation less complex for the Elasticsearch use-case.

### Current workflow vs Proposed

Currently, a consumer like Jaeger or Cluster Logging is responsible for the following for their ES cluster:

1. Generating and managing their own keys/certificates including those that are used in Elasticsearch as server serving and inter-cluster so that Elasticsearch can join and form clusters.
1. Generating and managing any client keys/certs to communicate with Elasticsearch. This would include their forwarders, and if applicable, their UIs.
1. Generating and managing secrets to be used by: EO, Elasticsearch, IndexManagement cronjobs, and any clients as denoted in point 2.
1. Creating and updating their elasticsearch CR(s) so that EO can manage their cluster.

With this proposal the workflow would be:

1. Create their elasticsearch CR with an annotation to denote EO will be managing certs for the cluster.
1. Create an empty secret in the same namespace as the elasticsearch CR with a name that matches the annotation they will use.
1. Add an additional annotation on the elasticsearch CR for each client they need to have certs injected into a specific secret (EO will be responsible for rotating the cert).

For example, CLO would use the following annotations to tell EO to do cert management and also inject client keys/certs into the `fluentd` secret with the CN `system.logging.fluentd`:

```yaml
apiVersion: logging.openshift.io/v1
kind: Elasticsearch
metadata:
  name: elasticsearch
  annotations:
    logging.openshift.io/elasticsearch-cert-management: true
    logging.openshift.io/elasticsearch-cert/fluentd: system.logging.fluentd
```

Similarly, Jaeger would do the following to have a client cert with the CN `user.jaeger` injected into the secret `jaeger`:

```yaml
apiVersion: logging.openshift.io/v1
kind: Elasticsearch
metadata:
  name: jaeger-cluster
  annotations:
    logging.openshift.io/elasticsearch-cert-management: true
    logging.openshift.io/elasticsearch-cert/jaeger: user.jaeger
```

## Proposal

Instead of the current implementation where any consumer of the Elasticsearch Operator is responsible for providing certificates and secrets for Elasticsearch and the operator to use, we want to shift this responsibility to the Elasticsearch Operator instead.

This will greatly improve the use case of the operator and make the operator the single source of truth for its certificates.

To allow other components to still be able to communicate with the managed cluster via mTLS the operator will watch for a specific annotation on the elasticsearch CR and will inject into a specified secret that is in the same namespace as the CR.

As part of the responsibility for generating, managing, and injecting these certificates, the EO would also be responsible for rotating the certificates and updating the content of the secrets when necessary.

### User Stories

#### As an user of EO, I do not want to have to generate and provide secrets for the Elasticsearch cluster

We want to further simplify using the Elasticsearch Operator and create a single source of truth for the certs the cluster uses.

#### As a consumer of EO, I want a mechanism to have certificates injected into a secret for me to continue mTLS communcation with my Elasticsearch cluster

We want to continue allowing mTLS as this supports the Cluster Logging and Jaeger use cases, however they shouldn't need to do more than notify they need certs injected by EO.

### Implementation Details

#### Assumptions

* The Elasticsearch Operator will implement this in golang (rather than the current script used in CLO)

* The operator will need to be able to store generated certificates for each namespace/unique Elasticsearch cluster so that there is no security concerns of clusters cross communicating.

#### Security

Ensuring that signing certificates and configurations are isolated for different clusters is a must for this implementation. It will also need to be able to do this thread safe manner.

### Risks and Mitigations

## Not Breaking Other Operators who Depend on EO

Currently there are two operators that depend on the Elasticsearch Operator and would need to ensure that their legacy functionality [read: their continuing to do cert management] is not broken.
For instances where one operator is upgraded along with EO, but the other is not, we would need to provide an opt-in model for Elasticsearch to perform cert management (if we make it opt-out, we would break the legacy functionality).

The most simple mechanism for this would be to gate Elasticsearch Operator performing cert management based on an annotation on the elasticsearch CR.

## Design Details

From a user perspective, there should be no change in any of the CRDs. The only major changes would be no longer generating secrets for EO to use, annotating their elasticsearch CR, and providing to an empty secret in the case they want to communicate with ES in a mTLS way.

### elasticsearch cr annotation

We will require an annotation value for opting in to the Elasticsearch Operator generating and managing certificates for the particular cluster:

```yaml
apiVersion: logging.openshift.io/v1
kind: Elasticsearch
metadata:
  name: elasticsearch
  annotations:
    logging.openshift.io/elasticsearch-cert-management: true
```

### secret injection annotation

Note: to utilize this will require the above annotation is also on the elasticsearch CR.
For the sake of simplifying the example of this annotation, it has been left off below.

We will add an annotation value to check for on the elasticsearch CR to denote that we would need to inject keys into a named secret:

```yaml
apiVersion: logging.openshift.io/v1
kind: Elasticsearch
metadata:
  name: elasticsearch
  annotations:
    logging.openshift.io/elasticsearch-cert/fluentd: system.logging.fluentd
```

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: fluentd
type: Opaque
data: {}
```

Where `fluentd` in the key name is the name of the secret and the value will be optional (defaults to the name of the secret) and corresponds to the CN used for the generated certificate.

The keys used for injecting into the secret will be `tls.crt`, `tls.key`, and `ca-bundle.crt` to corespond with the [log forwarding proposal](./cluster-logging-log-forwarding.md).


### Test Plan

#### Unit Testing

* We will need to ensure we have unit tests that verify certificates are generated by the EO

* We will need to ensure we have unit tests that verify secrets are generated by EO with the correct certificates

* We will need to ensure we have unit tests that verify EO generates and injects into secrets specified in an annotation

#### Integration and E2E tests

* Given we are moving logic from one operator to another, we need to ensure that there are no regressions with communication between Fluentd, Kibana, IM cronjobs, and Elasticsearch (this should be handled by our smoke test).

### Graduation Criteria

#### GA

* End user documentation
* Sufficient test coverage
* Sufficient LTS upgrade strategy

### Version Skew Strategy

We need to have a proper upgrade strategy from our LTS version to the version this is released on (currently 4.6).

## Implementation History

| release|Description|
|---|---|
|5.2| **GA** - Initial release

## Drawbacks

Given the OCP cert signing service does not allow for mTLS configuration with custom fields that the ES cluster requires, we are unable to use it so we have to implement something similar to a pre-existing service.https://bugzilla.redhat.com/show_bug.cgi?id=1925627
None

## References

* [Preliminary CLO script to golang reimplementation](https://github.com/openshift/cluster-logging-operator/pull/831)