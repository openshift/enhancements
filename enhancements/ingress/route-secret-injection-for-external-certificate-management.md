---
title: route-secret-injection-for-external-certificate-management
authors:
  - '@thejasn'
reviewers:
  - '@Miciah'
  - '@alebedev87'
  - '@tgeer'
  - '@joelspeed'
  - '@deads2k'
approvers:
  - '@Miciah'
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - '@joelspeed'
creation-date: 2022-12-13
last-updated: 2023-05-03
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CM-815
---

# Route Secret Injection For External Certificate Management

## Summary

Currently, users of OpenShift cannot very easily integrate OpenShift Routes
with third-party certificate management solutions like [cert-manager](https://github.com/cert-manager/cert-manager).
And this is mainly due to the design of Routes API which deliberately requires the certificate
data to be present in the Route object as opposed to having a reference. This is especially
problematic when third-party solutions also manage the life cycle (create/renew/delete)
of the generated certificates which OpenShift Routes does not support and requires
manual intervention to manage certificate life cycle.

This enhancement aims to provide a solution where OpenShift Routes can support
integration with third-party certificate management solutions like cert-manager and
avoid manual certificate management by the user which is more error prone.

## Motivation

OpenShift customers currently manually manage certificates for user workloads
by updating OpenShift Routes with the updated certificate data during expiry/renew
workflow. This is cumbersome activity if users have a huge number of workloads and
is also error prone.

This enhancement adds the support to OpenShift Routes for third-party certificate
management solutions like cert-manager by extending the Route API to read the serving
certificate data via a secret reference.

### User Stories

- As an end user of Route API, I want to be able to provide a tls secret reference on
  OpenShift Routes so that I can integrate with third-party certificate management solution

- As an OpenShift cluster administrator, I want to use third-party solutions like cert-manager
  for certificate management of user workloads on OpenShift so that no manual process is
  required to renew expired certificates.

- As an end user of Route API, I want OpenShift Routes to support both manual and managed
  mode of operation for certification management so that I can switch between manual certificate
  management and third-party certificate management.

- As an Openshift engineer, I want to be update the router so that it is able read secrets directly
  if all the preconditions have been met by the router serviceaccount.

- As an OpenShift engineer, I want to update the route validation in the api-server to add new validations
  required for `.spec.tls.certificateRef`.

- As an OpenShift engineer, I want to be able to update Route API so that I can integrate
  OpenShift Routes with third-party certificate management solutions like cert-manager.

- As an OpenShift engineer, I want to be able to run e2e tests as part of openshift/origin
  so that testcases are executed to signal feature health by CI executions.

### Goals

- Provide users with a configurable option in Route API to reference externally managed certificates via secrets.
- Provide smooth roll out of new certificates on OpenShift router when referenced certificates
  are renewed (secret containing the certificate is updated).

### Non-Goals

- Provide certificate life cycle management controls on the Route API (expiryAfter, renewBefore, etc).
- Modify ingress-to-route controller behaviour to use `.spec.tls.certificateRef`
- Extend this feature to cover CA certificate or destination CA certificate in the Route API.

## Proposal

This enhancement proposes extending the openshift/router to read serving certificate data either
from the Route `.spec.tls.certificate` and `.spec.tls.key` or from a new field `.spec.tls.certificateRef`
which is a `kubernetes.io/tls` type secret reference. This `certificateRef` field will enables the
users to provide a reference to a Secret containing the serving cert/key pair that will be parsed
and served by OpenShift router.

### Workflow Description

The following workflow describes the integration with third party
certificate management solutions like cert-manager with the certificate
reference field described under [API Extensions](#api-extensions).

- The end user must have generated the serving certificate generated
  as a pre requisite using third-party systems like cert-manager.
- In cert-manager's case, the [Certificate](https://cert-manager.io/docs/usage/certificate/#creating-certificate-resources)
  CR must be created in the same namespace where the Route is going to be created.
- The end user must create a role and in the same namespace as the secret containing the certificate from earlier,
  ```bash
  oc create role secret-reader --verb=get,list,watch --resource=secrets --resourceName=<secret-name>
  ```
- The end user must create a rolebinding in the same namespace as the secret
  and bind the router serviceaccount to the above created role.
  ```bash
  oc create rolebinding foo-secret-reader --role=secret-reader --serviceaccount=openshift-ingress:router --namespace=<current-namespace>
  ```
- To expose a user workload, the user would create a new Route with the
  `.spec.tls.certificateRef` referencing the generated secret that was created
  in the previous step.
- If the secret that is referenced exists and has a successfully generated
  cert/key pair, the router will serve this certificate if all preconditions are met.

#### Variation [optional]

N.A

### API Extensions

A `.spec.tls.certificateRef` field is added to Route `.spec.tls` which can be used to provide a secret name
containing the certificate data instead of using `.spec.tls.certificate` and `spec.tls.key`.

```go

// TLSConfig defines config used to secure a route and provide termination
//
// +kubebuilder:validation:XValidation:rule="has(self.termination) && has(self.insecureEdgeTerminationPolicy) ? !((self.termination=='passthrough') && (self.insecureEdgeTerminationPolicy=='Allow')) : true", message="cannot have both spec.tls.termination: passthrough and spec.tls.insecureEdgeTerminationPolicy: Allow"
// +kubebuilder:validation:XValidation:rule="has(self.certificate) && has(self.certificateRef) ? false : true", message="cannot have both spec.tls.certificate and spec.tls.certificateRef"
type TLSConfig struct {
	// ...

	// certificateRef provides certificate contents as a secret reference.
	// This should be a single serving certificate, not a certificate
	// chain. Do not include a CA certificate. The secret referenced should
	// be present in the same namespace as that of the Route.
	//
	// +openshift:enable:FeatureSets=TechPreviewNoUpgrade
	// +optional
	CertificateRef *corev1.LocalObjectReference `json:"certificateRef,omitempty" protobuf:"bytes,7,opt,name=certificateRef"`
}

// LocalObjectReference contains enough information to let you locate the
// referenced object inside the same namespace.
// +structType=atomic
type LocalObjectReference struct {
	// Name of the referent.
	// More info: https://kubernetes.io/docs/concepts/overview/working-with-objects/names/#names
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
}
```

_Note_: The default value would be `nil`. The secret is required to be created
in the same namespace as that of the Route. The secret must be of type
`kubernetes.io/tls` and the tls.key and the tls.crt key must be provided in
the `data` (or `stringData`) field of the Secret configuration.

If neither `.spec.tls.certificateRef` or `.spec.tls.certificate` and `.spec.tls.key` are
provided the router will serve the default generated secret.

All valid and invalid scenarios will be depicted via the existing `RouteIngressCondition`.

#### Variation

N.A

### Implementation Details/Notes/Constraints [optional]

The router will read the secret referenced in `.spec.tls.certificateRef` if present and
if the following pre-conditions (validated in the API server) are met it uses
this certificate us configure haproxy.

The router will bootstrap a watch based secret manager to ensure it can keep the
certificate/secret up-to-date. This means the router pod will maintain active watches
for every secret that is referenced by a route.

Every active watch will be linked to a route, meaning the watch based secret manager
will be linked to the lifecycle of the route. For every new route that is created,
the secret manager will start a watch if the route uses `.spec.tls.certificateRef`.
For every update route event, the secret manager only increments the reference count.
If a route is deleted, the secret manager will unregister the route and teardown the
watch associated with it.

The `ServiceAliasConfig` creation logic will be updated in the router to also parse
the secret referenced in `.spec.tls.certificateRef`. The router will
use the default certificates only when neither `.spec.tls.certificate` or `.spec.tls.certificateRef`
are provided.

Validations done by the router as part of [ExtendedValidateRoute()](https://github.com/openshift/router/blob/c407ebbc5d8d85daea2ef2d1ba539444a06f4d25/pkg/router/routeapihelpers/validation.go#L158) (contents of secret),

- Verify certificate and key (PEM encode/decode)
- Verify private key matches public certificate

Validations done by API server as part of [ValidateRoute()](https://github.com/openshift/openshift-apiserver/blob/aac3dd5bf0547e928103a0f718ca104b1bb13930/pkg/route/apis/route/validation/validation.go#L21),

- The secret created should be in the same namespace as that of the route.
- The secret created is of type `kubernetes.io/tls`.
- The router serviceaccount must have permission to read this secret particular secret.
  - The role and rolebinding to provide this access must be provided by the user.
- CEL validations will enforce that both `.spec.tls.certificate` and `.spec.tls.certificateRef`
  are not specified on the route.

### Risks and Mitigations

There is a possibility of an invalid route being processed by the router (edge case),
if any changes are done to the rbac or the referenced secret is deleted after the API
server validation but before router has processed the route (maybe router pod is not running)
then this can lead to the router processing this incorrect route.

> Will need to duplicate the validations present on the API server to the router.

### Drawbacks

The user will need to manually create, provide and maintain the rbac required by the
router so that it can access secrets securely. This becomes tedious when users have
100s of Routes.

The workaround for this is to document various levels of rbac that can be provided,

- Grant router service account access to secret by secret-name (explicit rbac)
- Grant router service account access to all secrets in a fixed namespace (implicit rbac)

The above variations need to be documented for the end user as part of OpenShift documentation.

## Design Details

### Open Questions [optional]

- Performance testing of openshift-router in tech-preview? Is there
  a workflow present where we can gather some early metrics (memory, cpu)? This will help in
  preemptively addressing performance concerns before going GA.

- Do we make changes to the ingress-to-route controller as well?
  > The ingress-to-route behaviour will remain as is i.e. it will not make use of
  > the newly introduced field.

### Test Plan

Update router tests in openshift/origin and supplement all existing certificate related tests
with new tests utilizing `.spec.tls.certificateRef`. Ensure the tests cover the following scenarios,

- Updating routes from default certificates to certificate referenced via
  secrets and vice-versa.
- Updating secret/certificate referenced in routes and verify serving
  certificate has been updated.
- Updating secret/certificate with incorrect information and verify route
  is not admitted due to validation failure. (eg: mismatched public and private key, etc)

### Graduation Criteria

This feature will initially be released as Tech Preview only.

#### Dev Preview -> Tech Preview

N/A. This feature will go directly to Tech Preview.

#### Tech Preview -> GA (Future work)

The router will need to undergo performance testing as part of OCP payload
to ensure the memory implications of creating and maintains all the active watches
is verified to be efficient.

The ingress-to-route controller in the route-controller-manager will need to
be updated to ensure that the created routes use `.spec.tls.certificateRef`
instead of `.spec.tls.certificate`. Additional tests will need to be added into
o/origin for this scenario.

This behaviour should be extended to both `.spec.tls.caCertificate` and
`.spec.tls.destinationCACertificate` to ensure uniformity and improve security.

#### Removing a deprecated feature

N/A.

### Upgrade / Downgrade Strategy

On downgrades, all routes specifying `.spec.tls.certificateRef` will switch over to use the default certificates
unless the route is manually edited and the `.spec.tls` is updated.

Upgrade strategy not considered since this feature is going to be added as TechPreviewNoUpgrade.

### Version Skew Strategy

This feature will be added as TechPreviewNoUprade.

### Operational Aspects of API Extensions

Route validation in the API server will be modified to validate the following scenarios,

- Check if secret/certificate referenced under `.spec.tls.certificateRef` exists.
- Check if secret/certificate referenced under `.spec.tls.certificateRef` is of the correct type.
- Check if router service account has permissions to read referenced secret.
- Check if route only one of the fields set,
  - `.spec.tls.certificate` and `.spec.tls.key`
  - `.spec.tls.certificateRef`

TODO: SLOs

#### Failure Modes

##### Referenced secret not present

In scenarios where the secret has not been created by third-party solutions
like cert-manager, the route would not be successfully created due to the
dependency. This route will be rejected by the API server with an
`FieldValueNotFound` error and will contain the reason as `referenced secret not present`.

In addition to this validation, the router will also validate the same to
cover an edge case. If a route fails this validation, it is not processed
further and the error will be reflected on the route `.status` with the same reason.

##### Insufficient router permission

As part of the API server validation, if the router does not have permission
to read the secret referenced under `.spec.tls.certificateRef`, the route is
rejected with an `FieldValueForbidden` error and reason as `insufficient permission
to read resource`.

##### Incorrect secret type

As part of `ExtendedValidateRoute()`, the router validates the content of the secret
that is referenced under `.spec.tls.certificateRef`. Failure will result in the route
not being admitted and this will reflect under route `.status` as `FieldValueInvalid`.

#### Support Procedures

N.A

## Implementation History

N.A

## Alternatives

### Secret Injector

An alternative proposal is to introduce a new controller (secret-injector) in [route-controller-manager](https://github.com/openshift/route-controller-manager)
to manage a new annotation (secret-reference) on the Route object. This annotation
enables the users to provide a reference to a Secret containing the serving cert/key
pair that will be injected to `.spec.tls` and will be served by OpenShift router.
This annotation will be given a higher preference if route CR also has `.spec.tls.certificate`
and `.spec.tls.key` fields set.

> This approach was dropped after much deliberation as it introduces a confused deputy problem
> as well as opens a security flaw where a user could read the contents of an arbitrary secret.

### Extend oc CLI

As an alternative to requiring the user create the role and rolebinding to grant the router
access to the secrets, this behaviour can be baked into `oc create route`. This will reduce
the number of manual steps and will be less error prone. But here's the catch, how widely
is `oc create route` used and do users who manage 100s of routes really use it.

## Infrastructure Needed [optional]

N.A
