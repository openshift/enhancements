---
title: client-tls
authors:
  - "@Miciah"
reviewers:
  - "@danehans"
  - "@frobware"
  - "@knobunc"
  - "@sgreene570"
approvers:
  - "@knobunc"
creation-date: 2020-08-31
last-updated: 2021-07-21
status: implementable
see-also:
replaces:
superseded-by:
---

# Ingress Client TLS API

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the IngressController API to allow the user to
configure the use of client certificates for mutual TLS.

## Motivation

Some applications require the use of TLS with client certificates in addition to
server certificates in order to provide two-way certificate-based authentication
("mutual TLS", or "mTLS").  OpenShift 3 supports mTLS in the router, and the
absence of mTLS support in OpenShift 4 blocks some users from upgrading.

### Goals

1. Enable the user to configure a client CA certificate bundle.
2. Enable the user to configure optional or mandatory use of client certificates.
3. Respect client CA certificates' certificate revocation lists per [RFC 5820](https://datatracker.ietf.org/doc/html/rfc5280), including those referenced using [X509v3 distribution points](https://datatracker.ietf.org/doc/html/rfc5280#section-4.2.1.13).
4. Allow the cluster administrator to restrict certificates by subject name.

### Non-Goals

1. Configuring client certificates on a per-route basis is out of scope.
2. Allowing the cluster administrator to restrict certificates by issuer is out of scope for this enhancement (it may be pursued as a stretch goal or future enhancement).
3. Implementing support for OCSP is out of scope.
4. Implementing support for LDAP CRL distribution points is out of scope.

## Proposal

The IngressController API is extended by adding an optional `ClientTLS` field
with the eponymous type to `IngressControllerSpec`:

```go
type IngressControllerSpec struct {
	// ...

	// clientTLS specifies settings for requesting and verifying client
	// certificates, which can be used to enable mutual TLS.
	ClientTLS ClientTLS `json:"clientTLS,omitempty"`
}
```

The `ClientTLS` type has a required `ClientCertificatePolicy` field of the
eponymous type for configuring validation of client certificates, a required
`ClientCA` field of type `configv1.ConfigMapNameReference` for specifying a
ConfigMap with a client CA certificate bundle, and an optional
`AllowedSubjectPatterns` field of type `[]string` for configuring a list of
allowed subject names:

```go
// ClientTLS specifies TLS configuration to enable client-to-server
// authentication, which can be used for mutual TLS.
type ClientTLS struct {
	// clientCertificatePolicy specifies whether the ingress controller
	// requires clients to provide certificates.  This field accepts the
	// values "Required" or "Optional".
	//
	// +kubebuilder:validation:Required
	// +required
	ClientCertificatePolicy ClientCertificatePolicy `json:"clientCertificatePolicy"`

	// clientCA is a reference to a configmap containing the PEM-encoded CA
	// certificate bundle that should be used to verify a client's
	// certificate.
	//
	// +kubebuilder:validation:Required
	// +required
	ClientCA configv1.ConfigMapNameReference `json:"clientCA"`

	// allowedSubjectPatterns specifies a list of regular expressions that
	// should be matched against the distinguished name on a valid client
	// certificate to filter requests.  If this list is empty, no filtering
	// is performed.  If the list is nonempty, then at least one pattern
	// must match a client certificate's distinguished name or else the
	// ingress controller rejects the certificate and denies the connection.
	//
	// +optional
	AllowedSubjectPatterns []string `json:"allowedSubjectPatterns,omitempty"`
}
```

The `ClientCertificatePolicy` type accepts either one of two values: `Required`
or `Optional`:

```go
// ClientCertificatePolicy describes the policy for client certificates.
// +kubebuilder:validation:Enum="";Required;Optional
type ClientCertificatePolicy string

const (
	// ClientCertificatePolicyRequired indicates that a client certificate
	// should be required.
	ClientCertificatePolicyRequired ClientCertificatePolicy = "Required"

	// ClientCertificatePolicyOptional indicates that a client certificate
	// should be requested but not required.
	ClientCertificatePolicyOptional ClientCertificatePolicy = "Optional"
)
```

If the user does not specify `spec.clientTLS`, then client TLS is not enabled,
which means that the IngressController does not request client certificates on
TLS connections.  If `spec.clientTLS` is specified, then the IngressController
does request client certificates, and `spec.clientTLS.clientCertificatePolicy`
must be specified to indicate whether the IngressController should reject
clients that do not provide valid certificates.

The required `ClientCA` field may be used to specify a reference to a ConfigMap
that is in the same namespace as the IngressController and contains a CA
certificate bundle.  The IngressController uses this bundle to verify client
certificates, and the IngressController does not start serving unless and until
it can read the client CA certificate bundle as well as the certificate
revocation list (CRL) for each certificate therein that specifies one or more
distribution points for retrieving that certificate's CRL.

Finally, the optional `AllowedSubjectPatterns` field may be used to specify a
list of patterns.  If the field is specified, then the IngressController rejects
any client certificate that does not match at least one of the provided
patterns.

The following IngressController enables mandatory use of client certificates,
verified using the CA certificates in the `router-ca-certs-default` ConfigMap,
and requiring that client certificates match the provided subject:

```yaml
apiVersion: operator.openshift.io/v1
kind: IngressController
metadata:
  name: default
  namespace: openshift-ingress-operator
spec:
  clientTLS:
    clientCertificatePolicy: Required
    clientCA:
      name: router-ca-certs-default
    allowedSubjectPatterns:
    - "^/CN=example.com/ST=NC/C=US/O=Security/OU=OpenShift$"
```

### Validation

Omitting `spec.clientTLS` has well defined semantics.

The API validates the `spec.clientTLS.clientCertificatePolicy` field value as
described by the field type's `+kubebuilder:validation:Enum` marker.

The API does not validate that the `spec.clientTLS.clientCA.name` field value
references a valid ConfigMap; if the named ConfigMap does not exist, then the
IngressController Deployment is not scheduled until the ConfigMap is created.

The API does not validate the `spec.clientTLS.allowedSubjectPatterns` field
value.

### User Stories

#### As a cluster administrator, I do not want to enable use of client certificates

Client TLS is not enabled by default, so no action is needed.

#### As a cluster administrator, I want to enable optional use of client certificates

The user enables client TLS with the `Optional` client certificate policy and a
custom client CA certificate bundle.  The operator configures the
IngressController to request and verify client certificates using the provided
CA certificate bundle while still allowing clients that do not present a client
certificate.

#### As a cluster administrator, I want to require client certificates using a CA that I control

The user enables client TLS with the `Required` client certificate policy and a
custom client CA certificate bundle.  The operator configures the
IngressController to request and verify client certificates using the provided
CA certificate bundle and reject clients that do not present valid client
certificates.

#### As a cluster administrator, I want to require client certificates using a CA that I control, respecting its certificate revocation list

The user enables client TLS with the `Required` client certificate policy and a
custom client CA certificate bundle with a CA certificate using the X509v3
extension to specify one or more CRL distribution points in the certificate.
The operator configures the IngressController to request and verify client
certificates using the provided CA certificate bundle and reject clients that do
not present valid client certificates.  The operator periodically updates the
CRL using the distribution points.

#### As a cluster administrator, I want to force a refresh of all certificate revocation lists

The user deletes the `router-client-ca-crl-<name>` ConfigMap in the
`openshift-ingress` namespace.  The deletion triggers the operator's
reconciliation logic, which redownloads the CRLs and recreates the ConfigMap.
The kubelet updates the volume once the ConfigMap is recreated.  Note that the
kubelet does not update a ConfigMap volume while the referenced ConfigMap is
absent, so the CRLs from the old ConfigMap remain in effect until the ConfigMap
is recreated.

### Implementation Details

The implementation details are described in two parts.  First, the basic
implementation of client certificate verification is described.  This
functionality mostly relies on functionality that is built in to HAProxy.
Second, support for certificate revocation lists (CRLs) is described, which
requires a new controller in the ingress operator to identify CRL distribution
points from provided client CA certificates and download and periodically
refresh these CRLs.

#### Implementing Basic Support for Client Certificate Verification

OpenShift router configures HAProxy using a configuration template.  The
template uses environment variables as input parameters.  Most of the basic
functionality for configuring client TLS already exists in the template and can
be configured using well defined environment variables.

The client certificate policy can be configured using the
`ROUTER_MUTUAL_TLS_AUTH` variable.  If this variable is set to a nonempty value,
the template adds the `verify` option with the given value to the `bind` stanzas
for TLS frontends.  If `spec.clientTLS.clientCertificatePolicy` is specified on
an IngressController, the ingress operator sets `ROUTER_MUTUAL_TLS_AUTH` to the
corresponding value, and otherwise the operator does not set
`ROUTER_MUTUAL_TLS_AUTH`.

Similarly, the path to a client CA certificate bundle can be specified using the
`ROUTER_MUTUAL_TLS_AUTH_CA` variable; if this variable is set to a nonempty
value, the configuration template adds the `ca-file` option with the given value
to the `verify` option on `bind` stanzas.

To inject the client CA certificate bundle into the router Pod, the operator
adds a ConfigMap volume and volume mount to the router Deployment.  The operator
then specifies the mount path using the `ROUTER_MUTUAL_TLS_AUTH_CA` variable in
the router Deployment.  Thus the ingress operator uses these environment
variables to configure the client TLS policy and CA bundle.

To allow configuring a pattern for filtering client certificates by their
subjects, the configuration template uses the `ROUTER_MUTUAL_TLS_AUTH_FILTER`
variable.  If this variable is set to a nonempty value, the template uses this
value and HAProxy's `ssl_c_s_dn` sample fetch method to to configure HAProxy
ACLs that perform regular expression matches against the distinguished names of
the subjects of client certificates and deny access if a match fails.  If
`spec.clientTLS.allowedSubjectPatterns` is nonempty, the operator combines the
patterns therein and sets the result as the value for the
`ROUTER_MUTUAL_TLS_AUTH_FILTER` variable in the router Deployment.

#### Implementing Support for Certificate Revocation Lists

Implementing support for certificate revocation lists (CRLs) is more challenging
because CRLs are typically not provided directly by the user, but instead must
be downloaded and periodically refreshed from "distribution points" specified in
the client CA certificates.  A new controller in the ingress operator performs
this duty.

To obtain the CRLs for client CA certificates, the new controller first scans
the certificates in the client CA certificate bundle ConfigMap that the user
provided for the IngressController in order to determine whether any
certificates specify any CRL distribution points.  If none does, then there are
no CRLs to download, and so no further configuration is required for CRLs.
However, if any certificates do specify any CRL distribution points, then the
controller downloads the CRLs if needed and publishes them in a ConfigMap.

If the CRL ConfigMap already exists, the new controller first checks for each
certificate that specifies any CRL distribution points whether the CRL already
exists in the ConfigMap and whether the CRL's `nextUpdate` period has elapsed.
If the CRL is present and the period has not elapsed, the existing CRL is used;
else if the CRL is not present or its period has elapsed, the controller
downloads the CRL and updates the CRL ConfigMap.

Similarly, the controller that manages router Deployments scans the client CA
certificate bundle ConfigMap to determine whether any CRL distribution points
are specified, in which case this controller can assume that the new controller
will publish a CRL ConfigMap.  Under this assumption, the existing controller
configures the router Deployment with a ConfigMap volume and volume mount for
the CRL ConfigMap and specifies the mount path using the
`ROUTER_MUTUAL_TLS_AUTH_CRL` environment variable in the Deployment.  This
volume mount is configured so that the Deployment's Pods cannot start until the
CRL ConfigMap exists.  This is necessary to ensure that revoked certificates are
not permitted while the CRL is downloaded.  Similarly to how the
`ROUTER_MUTUAL_TLS_AUTH_CA` variable specifies the path to the client CA
certificate bundle, the `ROUTER_MUTUAL_TLS_AUTH_CRL` variable specifies the path
to a file containing CRLs; if the variable is set to a nonempty value, the
configuration template adds the `crl-file` option with the given value to the
`verify` option on `bind` stanzas so that HAProxy checks the CRLs from the CRL
ConfigMap when validating client certificates.

### Risks and Mitigations

If the underlying IngressController implementation were to change away from
HAProxy to a different implementation, we would need to ensure that the new
implementation supported the same capabilities.

If a client CA certificate specifies a CRL distribution point that is invalid or
inaccessible, and does not specify a valid and accessible distribution point,
then the IngressController cannot start.  This is a deliberate design choice to
avoid potentially allowing revoked client certificates.

A CRL's refresh period may be determined from [the `nextUpdate`
field](https://datatracker.ietf.org/doc/html/rfc5280#section-5.1.2.5) of the CRL
itself or from the HTTP response headers when the operator downloads a CRL, or
the operator could simply refresh the CRL at a fixed period (for example, daily
or hourly).  It is not clear what the best approach is.

If all CRL distribution points for a certificate become inaccessible, the router
may continue running with an outdated CRL list.  The operator may need to report
failures to download CRLs, for example using metrics and alerts, so that the
administrator knows if the router is using an outdated CRL list.

## Design Details

### Test Plan

The controller that manages the IngressController Deployment and related
resources has unit test coverage; for this enhancement, the unit tests are
expanded to cover the additional functionality.

Unit tests can be written for the new controller that manages CRL ConfigMaps.

The operator has end-to-end tests; for this enhancement, the following tests can
be added:

1. Create a simple application Pod and Service that respond with a static CRL file.
2. Create an IngressController that specifies `spec.clientTLS` with the `Optional` client certificate policy and a certificate CA bundle with a certificate that specifies the Service as a CRL distribution point.
3. Open a connection to one of the standard Routes (such as the console Route) without specifying a client certificate, send an HTTP request, and verify that the request succeeds.
4. Open a connection to one of the standard Routes using a **valid** client certificate, send an HTTP request, and verify that the request succeeds.
5. Open a connection to one of the standard Routes using an **invalid** client certificate, and verify that the connection is rejected.
6. Configure the IngressController with the `Required` client certificate policy, and set `spec.clientTLS.allowedSubjectPatterns`.
7. Open a connection to one of the standard Routes without specifying a client certificate, and verify that the request is rejected.
8. Open a connection to one of the standard Routes using a **valid** client certificate with a **valid** subject, send a request, and verify that the request succeeds.
9. Open a connection to one of the standard Routes using a **valid** client certificate with an **invalid** subject, and verify that the connection is rejected.

### Graduation Criteria

N/A.

#### Dev Preview -> Tech Preview

N/A.  This feature will go directly to GA.

#### Tech Preview -> GA

N/A.  This feature will go directly to GA.

#### Removing a deprecated feature

N/A.  We do not plan to deprecate this feature.

### Upgrade / Downgrade Strategy

On upgrade, client TLS is not enabled by default, which is consistent with the
feature's absence in older versions.

On downgrade, the IngressController Deployment is updated, and any additional
volumes, volume mounts, and environment variables are removed from the
Deployment.  A ConfigMap with the client CA certificate bundle and another
ConfigMap with downloaded CRLs may remain.  These ConfigMaps would have an owner
reference on the Deployment and would be cleaned up if the IngressController
were deleted.

### Version Skew Strategy

N/A.

## Implementation History

* A work-in-progress, proof-of-concept implementation was posted in
 [openshift/cluster-ingress-operator#450](https://github.com/openshift/cluster-ingress-operator/pull/450)
 on 2020-09-01.

## Drawbacks

If the underlying IngressController implementation were to change away from
HAProxy to a different implementation, we would need to ensure that the new
implementation supported the same capabilities.


## Alternatives

We could refer users to third-party ingress controllers that implement this
functionality.  Alternatively, we could defer support for this feature until
OpenShift adopts an alternative ingress controller.
