---
title: default-ingress-cert-configmap
authors:
  - "@Miciah"
reviewers:
  - "@danehans"
  - "@deads2k"
  - "@frobware"
  - "@ironcladlou"
  - "@knobunc"
approvers:
  - "@deads2k"
  - "@ironcladlou"
  - "@knobunc"
creation-date: 2019-11-20
last-updated: 2020-08-21
status: implemented
see-also:
replaces:
superseded-by:
---

# Publishing the Default Ingress Certificate in the `default-ingress-cert` ConfigMap

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [X] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The ingress operator publishes the default certificate of the default
IngressController in a ConfigMap for other operators to consume.  This ConfigMap
is named `default-ingress-cert` and exists in the `openshift-config-managed`
namespace.  The intended consumers are other operators that need to incorporate
the default certificate into their trust bundles in order to connect to Route
resources.

## Motivation

Operators need to be able to verify access via the default IngressController to
the Routes that they create.  The default IngressController uses either a
default certificate that the operator generates using its own self-signed CA or
a custom default certificate that the administrator configures, which may or may
not be signed by a trusted root authority.  In any case, operators need to be
able to trust the default certificate.  Publishing the default certificate in a
well known location enables operators to incorporate it into their trust
bundles when connecting to their Routes.

### Goal

Publish the default IngressController's default certificate in a well known
location so that other components can incorporate the certificate into their
trust bundles.

### Non-Goal

Some operators need the key for the default certificate.  Satisfying this need
is outside the scope of this proposal.

## Proposal

The ingress operator was modified to publish the `default-ingress-cert`
ConfigMap as described in the summary.

### User Stories

#### Using `default-ingress-cert` to Verify a Route

The console operator creates a route for OpenShift Console and needs to verify
access to the route using the following steps:

1. Read the `default-ingress-cert` ConfigMap from the `openshift-config-managed`
   namespace.
2. Define a new certificate pool.
3. Add the certificate from the ConfigMap to the new pool.
4. Define a new TLS config that uses the pool for trust.
5. Send a request to the router using the new TLS config.
6. Verify that the request succeeds.

### Implementation Details

The ingress operator has a "certificate" controller, which lists and watches
IngressControllers and ensures that each has a default certificate configured.
This controller was amended to perform the following additional steps:

1. Check if the IngressController is the default one.  If not, skip the
   following steps.
2. Get the Secret with the IngressController's default certificate.
3. Publish the default certificate to the `default-ingress-cert` ConfigMap.

The ConfigMap will have the following form:

```yaml
apiVersion: v1
data:
  ca-bundle.crt: |
    -----BEGIN CERTIFICATE-----
    [...]
    -----END CERTIFICATE-----
kind: ConfigMap
metadata:
  name: default-ingress-cert
  namespace: openshift-config-managed
```

### Risks and Mitigations

The standard way to validate a certificate is to verify that the certificate is
signed by a trusted CA certificate.  Consumers therefore may expect the
`default-ingress-cert` ConfigMap to include the CA certificate that signed the
default certificate rather than the default certificate itself.

For Go-based clients, this is not a problem as the Go TLS implementation has
looser certificate validation that can be satisfied by configuring the
certificate itself in the trusted certificates pool.  As the ConfigMap is not
intended to be used outside of OpenShift's own operators, which are Go-based,
publishing the certificate itself should not pose a problem.  Furthermore, the
`default-ingress-cert` ConfigMap is an internal API, and to the extent that we
document it at all, we should document that it has the default certificate, not
the signing CA certificate.

## Design Details

### Test Plan

The ConfigMap and its publication have well defined semantics.
The controller that publishes `default-ingress-cert` has [unit test
coverage](https://github.com/openshift/cluster-ingress-operator/blob/f48a2f92e0c0089e5ca432119fb87d8ebb5c808e/pkg/operator/controller/certificate/publish_ca_test.go).
The operator has end-to-end tests, including
[TestRouterCACertificate](https://github.com/openshift/cluster-ingress-operator/blob/f48a2f92e0c0089e5ca432119fb87d8ebb5c808e/test/e2e/operator_test.go#L394),
to verify correct function of the "certificate" controller; this end-to-end test
was amended to cover the additional functionality.

### Graduation Criteria

N/A.  This is an internal API.  See also "Upgrade / Downgrade Strategy".

### Upgrade / Downgrade Strategy

The `default-ingress-cert` ConfigMap supersedes the `router-ca` ConfigMap (see
"Implementation History").  Components that used the latter have been updated to
use the former, and the latter has been removed:

* In 4.3.3, the ingress operator started publishing `default-ingress-cert` ([openshift/cluster-ingress-operator331](https://github.com/openshift/cluster-ingress-operator/pull/331), [BZ#1788711](https://bugzilla.redhat.com/show_bug.cgi?id=1788711)).
* In 4.4.0, the console operator changed from using `router-ca` to using `default-ingress-cert` ([openshift/console-operator#361](https://github.com/openshift/console-operator/pull/361)).
* In 4.5.0, the ingress operator stopped publishing `router-ca` ([openshift/cluster-ingress-operator#377](https://github.com/openshift/cluster-ingress-operator/pull/377)).

### Version Skew Strategy

N/A.

## Implementation History

In OpenShift 4.3 and earlier, the ingress operator publishes a `router-ca`
ConfigMap in the `openshift-config-managed` namespace, under certain
circumstances.  Namely, if for some IngressController the administrator has
provided no custom default certificate, the ingress operator generates a default
certificate using the "ingress CA": the operator's own self-generated,
self-signed signing certificate.  The ingress operator then publishes the
ingress CA in the named `router-ca` ConfigMap.  The operator does **not**
publish `router-ca` if no IngressController exists that uses an
operator-generated default certificate.

The fact that the ingress operator conditionally publishes the `router-ca`
ConfigMap in these OpenShift versions poses a challenge for potential consumers,
which cannot achieve their goals (typically, verifying a Route) if the ConfigMap
does not exist, and the fact that `router-ca` contains the CA means that it _can
only_ conditionally be published, as the ingress operator may not even have the
CA certificate for a custom default certificate.

The `default-ingress-cert` ConfigMap simplifies matters for Go-based consumers,
which can assume that the new ConfigMap exists, always.

The implementation history for `router-ca` precedes OpenShift 4.1 GA.
[NE-139](https://jira.coreos.com/browse/NE-139) describes the initial
implementation.
Following are the most salient PRs in the feature's history:

* [openshift/cluster-ingress-operator#109 "Use self-signed default router certificate"](https://github.com/openshift/cluster-ingress-operator/pull/109)
  added generation of default certificates and introduced the `router-ca`
  ConfigMap.
* [openshift/cluster-kube-apiserver-operator#222 "Trust cluster-ingress-operator's CA certificate"](https://github.com/openshift/cluster-kube-apiserver-operator/pull/222)
  proposed adding `router-ca` to kube-apiserver's trust bundle; it was not
  merged.  During review of this PR, it was determined that the ingress operator
  must conditionally publish `router-ca`.
* [openshift/cluster-kube-controller-manager-operator#145 "Trust cluster-ingress-operator's CA certificate"](https://github.com/openshift/cluster-kube-controller-manager-operator/pull/145)
  added `router-ca` to kube-controller-manager's trust bundle.
* [openshift/cluster-ingress-operator#110 "Put router-ca configmap in openshift-config-managed iff needed"](https://github.com/openshift/cluster-ingress-operator/pull/110)
  changed the ConfigMap's namespace to `openshift-config-managed` and made its
  publication conditional on its use.
* [openshift/cluster-ingress-operator#142 "controller/certificate: relocate CA publishing"](https://github.com/openshift/cluster-ingress-operator/pull/142)
  moved the `router-ca` publication to a separate controller.
* [openshift/console-operator#328 "Bug 1764704: Sync router-ca to the console namespace"](https://github.com/openshift/console-operator/pull/328)
  added `router-ca` to OpenShift Console's trust bundle.
* [openshift/cluster-ingress-operator#329 "Always publish router-ca configmap"](https://github.com/openshift/cluster-ingress-operator/pull/329)
  proposed making publication unconditional.
* [openshift/cluster-ingress-operator#331 "publish a router-ca that can be used
  to verify routes in golang
  clients"](https://github.com/openshift/cluster-ingress-operator/pull/331)
  implemented the proposed API.

## Drawbacks

Consumers must read the `default-ingress-cert` ConfigMap, which is inconsistent
with other patterns for cluster configuration, described in "Alternatives"
below.

## Alternatives

### Cluster Configuration API

Instead of using a ConfigMap, the ingress certificate could be added to the
`default` cluster `Ingresses.config.openshift.io` resource.  In this approach,
the `status` field would report the effective ingress certificate (or possibly
the CA certificate, if available).  Optionally, the `spec` field could be used
to configure a custom ingress CA certificate.

#### Drawbacks

Having certificate configuration in both the `Ingresses.config.openshift.io`
resource and the `IngressControllers.operator.openshift.io` resource may be
confusing to users.

Additionally, it may be better not to expose such a public API, especially since
publishing the default certificate (rather than the signing CA certificate) may
be useless and confusing for non-Go clients.

### Injection into ConfigMaps with a Well Known Label

Instead of or in addition to publishing a ConfigMap, the ingress operator could
inject the default IngressController's default certificate into all ConfigMaps
with a specific label, for example
`config.openshift.io/inject-default-ingress-cert: "true"`.  This approach is
analogous to the `config.openshift.io/inject-trusted-cabundle: "true"` label
used to configure a trusted CA for [global cluster egress
proxy](../proxy/global-cluster-egress-proxy.md).

#### Drawbacks

Similar drawbacks apply as with the "Cluster Configuration API" alternative.
