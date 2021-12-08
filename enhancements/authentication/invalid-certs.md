---
title: invalid-certs
authors:
  - "@s-urbaniak"
reviewers:
  - "@sttts"
  - "@stlaz"
  - "@deads2k"
approvers:
  - "@mfojtik"
creation-date: 2021-12-08
last-updated: 2022-03-10
---

# Detecting invalid HTTPS server certificates

## Summary

OpenShift 4.10 is going to be rebased against Kubernetes 1.23. This requires using Go 1.17.
However, starting with Go 1.17 the support for the CommonName field of server HTTPS certificates is going to be removed.

Formally, the temporary `GODEBUG=x509ignoreCN=0` flag [has been removed](https://go.dev/doc/go1.17#crypto/x509).
This implies that starting from OpenShift 4.10 invalid certificates will not be trusted any more as they will fail verification.

Example:

Given the following certificate:
```plaintext
Certificate:
    Data:
        ...
        Subject: CN=foo-domain.com
        X509v3 extensions:
            X509v3 Key Usage: critical
                Digital Signature, Key Encipherment
            X509v3 Extended Key Usage: 
                TLS Web Server Authentication
            X509v3 Basic Constraints: critical
                CA:FALSE
```

Verification against the `foo-domain.com` hostname of such certificate will fail with the following error in Go 1.17:
```plaintext
x509: certificate relies on legacy Common Name field, use SANs instead
```

Verification of server certificates is executed during the TLS Handshake procedure.
A TLS (https) client observing an invalid certificate will reject the connection attempt.

Cluster internal issued certificates are not affected,
however custom certificates can be configured in various cases:
- custom serving certificates for kube-apiserver
- custom API webhooks
- custom aggregated API endpoints
- custom certificates for route endpoints
- certificates of external auth identity providers

This will lead to broken connections to critical core parts of OpenShift and thus to a degraded cluster if invalid custom certificates are configured.

This enhancement suggests a solution to prevent core workloads involving
invalid certificates, functional on OpenShift 4.9, to break upon the upgrade to
OpenShift 4.10. It therefore proposes:
- a way to detect invalid certificates in kube-apiserver and oauth-server
- a way to prevent upgrades from OpenShift 4.9 to OpenShift 4.10 in the face of invalid certificates

## Motivation

Broken workloads must be prevented due to invalid certificates
when upgrading from OpenShift 4.9 to OpenShift 4.10.

### Goals

A means to detect invalid certificates for TLS connections between:
- kube-apiserver and API webhooks
- kube-apiserver and aggregated API endpoints
- custom serving certificates for kube-apiserver
- oauth-server and its external route
- oauth-server and external identity providers

### Non-Goals

* Other core workloads are out of scope for this enhancement.
* Alerting, although appropriate and desirable, is left outside the scope of
  this very enhancement.

## Proposal

### User Stories

1. As a cluster admin I want to be prevented from upgrading OpenShift 4.9 to OpenShift 4.10
if I configured invalid certificates.

2. As a cluster admin I want to get feedback in OpenShift 4.9 if invalid certificates are being detected.

### API Extensions

N/A

### Detecting invalid certificates

#### kube-apiserver

In Kubernetes detection of invalid certificates has been added as part of https://github.com/kubernetes/kubernetes/pull/95396.
The introduced Prometheus `apiserver_webhooks_x509_missing_san_total` and `apiserver_kube_aggregator_x509_missing_san_total` metrics provides the means to detect invalid certificates against API webhooks and aggregated API endpoints.

To detect invalid certificates this enhancement proposes to add a new controller in `cluster-kube-apiserver-operator` that:

1. Regularly executes queries against the cluster internal Thanos instance
2. Verifies if above metrics have a value >0
3. If invalid certificates are detected, the controller sets a `InvalidCertsUpgradeable=false` condition on the `cluster` `KubeAPIServer.operator.openshift.io` resource which will yield the `Upgradeable=false` status on the `kube-apiserver` `config.openshift.io.ClusterOperator` resource.

#### oauth-server

For oauth-server this enhancement proposes the introduction of the following new metric:

- `openshift_auth_external_x509_missing_san_total`: a metric capturing the count of invalid certificates
presented by an external identity provider.

To detect invalid certificates this enhancement proposes to add a new controller in `cluster-authentication-operator` that:

1. Regularly executes queries against the cluster internal Thanos instance
2. Verifies if above metrics have a value >0
3. If invalid certificates are detected, the controller sets a `InvalidCertsUpgradeable=false` condition on the `cluster` `Authentication.operator.openshift.io` resource which will yield the `Upgradeable=false` status on the `authentication` `config.openshift.io.ClusterOperator` resource.

#### cluster-authentication-operator

In case of having invalid custom certificates configured for the external OAuth route
the existing `RouterCertsDomainValidationController` is extended.
Formally additional logic is added in `validateRouterCertificates` https://github.com/openshift/cluster-authentication-operator/blob/ff156ab2bdfbdd68b49d76547fea5ec28d9c3639/pkg/controllers/routercerts/controller.go#L129.

Additionally, custom routes controller in `openshift-authentication-operator` will reject invalid certificates in https://github.com/openshift/cluster-authentication-operator/blob/d2f6218a6ab2daccbec43f71a495de1c80529ef6/pkg/controllers/customroute/custom_route_controller.go#L55.

#### library-go

A library-go function will be implemented that takes a `*x509.Certificate` and returns a `bool` value
indicating if it is an invalid certificate. The function will check whether the given certificate relies
on the Common Name field rather than SANs.

This function will be used to assert validity of invalid certificates in the external OAuth route case.

The higher level controller which asserts metrics for a given Prometheus client will be provided in libary-go.

### Risks and Mitigations

This enhancement provides a means to protect against broken clusters when upgrading from OpenShift 4.9 to OpenShift 4.10.

## Design Details

### Open Questions [optional]

N/A

### Test Plan

N/A

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The above proposed changes **must** be backported to OpenShift 4.9.
Otherwise, a detection of invalid certificates will not be possible before an upgrade to OpenShift 4.10.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

N/A

## Implementation History

N/A

## Drawbacks

N/A

## Alternatives

N/A
