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
last-updated: 2023-04-24
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CM-16
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

- As an OpenShift engineer, I want to be able to update Route API so that I can integrate
  OpenShift Routes with third-party certificate management solutions like cert-manager.

- As an OpenShift engineer, I want to be able to run e2e tests as part of openshift/origin
  so that testcases are executed to signal feature health by CI executions.

### Goals

- Provide users with a configurable option in Route API to reference externally managed certificates via secrets.

### Non-Goals

- Provide certificate life cycle management controls on the Route API (expiryAfter, renewBefore, etc).
- Provide smooth roll out of new certificates on OpenShift router when referenced certificates
  are renewed (secret containing the certificate is updated).

## Proposal

This enhancement proposes extending the openshift/router to read serving certificate data either
from the Route `.spec.tls.certificate` and `.spec.tls.key` or from a new field `.spec.tls.certificateRef`
which is a `kubernetes.io/tls` type secret reference. This `certificateRef` field will enables the
users to provide a reference to a Secret containing the serving cert/key pair that will be parsed
and served by OpenShift router.

### Workflow Description

End users have 2 possible variations for the creation of the route:

- Create a route for the user workload and this is completely managed by the end user.
- Create an ingress for the user workload and a managed route is created automatically
  by the ingress-to-route controller. [Depends on the open question]

Both these workflows will support integrating with third party certificate management
solutions like cert-manager with the secret-reference annotation described under [API Extensions](#api-extensions).

**When a Route is directly used to expose user workload**

- The end user must have generated the serving certificate as a pre requisite
  using third-party systems like cert-manager.
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

**When an Ingress is used to expose user workload**

- The end user must have generated the serving certificate as a pre requisite
  using third-party systems like cert-manager.
- In cert-manager's case, the [Certificate](https://cert-manager.io/docs/usage/certificate/#creating-certificate-resources) CR must be created in the same namespace
  where the Ingress is going to be created.
- To expose a user workload, a new Ingress with the generated secret
  referenced in `.spec.tls.secretName` needs to be created.
- If the secret CR that is referenced exists and has a successfully generated
  cert/key pair, the ingress-to-route controller adds this secret name to
  the created route `.spec.tls.certificateRef`.

#### Variation [optional]

N.A

### API Extensions

A `.spec.tls.certificateRef` field is added to Route `.spec.tls` which can be used to provide a secret name
containing the certificate data instead of using `.spec.tls.certificate` and `spec.tls.key`.

```go
type TLSConfig struct {
	// ...

    // certificateRef provides certificate contents as a secret reference.
    // This should be a single serving certificate, not a certificate
	// chain. Do not include a CA certificate.

    //
    // +kubebuilder:validation:Optional
	// +openshift:enable:FeatureSets=TechPreviewNoUpgrade
	// +optional
	CertificateRef *corev1.LocalObjectReference `json:"certificateRef,omitempty" protobuf:"bytes,7,opt,name=certificateRef"`
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
if the following pre-conditions are met it uses this certificate us configure haproxy.

- The secret created should be in the same namespace as that of the route.
- The secret created is of type `kubernetes.io/tls`.
- The router serviceaccount must have permission to read this secret particular secret.
  - The role and rolebinding to provide this access must be provided by the user.

The router will not have any active watches on the secret and will only
do a single look up when a route has been updated. The router will maintain
a secret hash in order to be able to reload if the secret content has changed.

The `ServiceAliasConfig` creation logic will be updated in the router to also parse
the secret referenced in `.spec.tls.certificateRef`. The router will
use the default certificates only when neither `.spec.tls.certificate` or `.spec.tls.certificateRef`
are provided.

The route validating admission webhook will verify if the `router` serviceaccount
route has permissions to read the secret that is referenced at `.spec.tls.certificateRef`.
This is only performed if `.spec.tls.certificateRef` is non-nil and non-empty.
In addition to the rbac validation, the admission webhook will also validate if only one
of the certificate fields (`.spec.tls.certificate` and `.spec.tls.certificateRef`) is specified.

### Risks and Mitigations

The TechPreview feature will not handle secret updates, meaning upon certificate renewal/rotation
the router will not load the new certificates until the route is updated.

### Drawbacks

The user will need to manually create, provide and maintain the rbac required by the
router so that it can access secrets securely. This becomes tedious when users have
1000s of Routes.

## Design Details

### Open Questions [optional]

- Performance testing of openshift-router?
- What does taking Tech Preview to GA look like?
- Do we make changes to the ingress-to-route controller as well?

### Test Plan

Update router tests in openshift/origin and supplement all existing certificate related tests
with new tests utilizing `.spec.tls.certificateRef`. Ensure the tests cover the following scenarios,

- Updating routes from default certificates to certificate referenced via secrets and vice-versa.

### Graduation Criteria

This feature will initially be released as Tech Preview only.

#### Dev Preview -> Tech Preview

N/A. This feature will go directly to Tech Preview.

#### Tech Preview -> GA

The router will need additional logic to handle secret updates using single item
list/watch for every referenced secret. This pattern needs to be brought over
from kubelet's [secret_manager.go](https://github.com/kubernetes/kubernetes/blob/master/pkg/kubelet/secret/secret_manager.go)

Once this pattern is added to the router, the test plan needs to be updated
to cover all scenarios involving updating referenced secrets.

The ingress-to-route controller in the route-controller-manager will need to
be updated to ensure that the created routes use `.spec.tls.certificateRef`
instead of `.spec.tls.certificate`. Additional tests will need to be added into
o/origin for this scenario.

#### Removing a deprecated feature

N/A.

### Upgrade / Downgrade Strategy

On downgrades, all routes specifying `.spec.tls.certificateRef` will switch over to use the default certificates
unless the route is manually edited and the `.spec.tls` is updated.

Upgrade strategy not considered since this feature is going to be added as TechPreviewNoUpgrade.

### Version Skew Strategy

This feature will be added as TechPreviewNoUprade.

### Operational Aspects of API Extensions

N.A

#### Failure Modes

When using routes with third-party certificate management solutions like cert-manager, this
adds a hard dependency in order of operation. In scenarios where the secret has not been created
by third-party solutions like cert-manager, the route would not be successfully created due
to the dependency on the route.

#### Support Procedures

N.A

## Implementation History

N.A

## Alternatives

An alternative proposal is to introduce a new controller (secret-injector) in [route-controller-manager](https://github.com/openshift/route-controller-manager)
to manage a new annotation (secret-reference) on the Route object. This annotation
enables the users to provide a reference to a Secret containing the serving cert/key
pair that will be injected to `.spec.tls` and will be served by OpenShift router.
This annotation will be given a higher preference if route CR also has `.spec.tls.certificate`
and `.spec.tls.key` fields set.

This approach was dropped after much deliberation as it introduces a confused deputy problem
as well as opens a security flaw where a user could read the contents of an arbitrary secret.

## Infrastructure Needed [optional]

N.A
