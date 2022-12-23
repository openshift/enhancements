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
last-updated: 2023-01-27
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/CFE-704
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
management solutions like cert-manager.

### User Stories

- As an end user of Route API, I want to be able to provide a tls secret reference on
  OpenShift Routes so that I can integrate with third-party certificate management solution

- As an OpenShift cluster administrator, I want to use third-party solutions like cert-manager
  for certificate management of user workloads on OpenShift so that no manual process is
  required to renew expired certificates.

- As an end user of Route API, I want OpenShift Routes to support both manual and managed
  mode of operation for certification management so that I can switch between manual certificate
  management and third-party certificate management.

- As an OpenShift engineer, I want to be able to update Route API so that I can integrate
  OpenShift Routes with third-party certificate management solutions like cert-manager.

- An an OpenShift engineer, I want to be able to watch routes and process routes having valid
  annotations so that the certificate data is injected into the route CR.

- As an OpenShift engineer, I want to be able to run e2e tests as part of openshift/origin
  so that testcases are executed to signal feature health by CI executions.

### Goals

- Provide users with a configurable option in Route API to provide externally managed certificate data.
- Provide users with a mechanism to switch between using externally managed certificates and
  manually managed certificates on OpenShift routes and vice-versa.
- Provide smooth roll out of new certificates on OpenShift router when managed certificates
  are renewed.
- Provide latest certificate type information on the Route.

### Non-Goals

- Provide certificate life cycle management controls on the Route API (expiryAfter, renewBefore, etc).
- Provide migration of certificates from current solution.

## Proposal

This enhancement proposes introducing a new controller (secret-injector) in [route-controller-manager](https://github.com/openshift/route-controller-manager)
to manage a new annotation (secret-reference) on the Route object. This annotation
enables the users to provide a reference to a Secret containing the serving cert/key
pair that will be injected to `.spec.tls` and will be served by OpenShift router.
This annotation will be given a higher preference if route CR also has `.spec.tls.certificate`
and `.spec.tls.key` fields set.

### Workflow Description

End users have 2 possible variations for the creation of the route:

- Create a route for the user workload and this is completely managed by the end user.
- Create an ingress for the user workload and a managed route is created automatically
  by the ingress-to-route controller.

Both these workflows will support integrating with third party certificate management
solutions like cert-manager with the secret-reference annotation described under [API Extensions](#api-extensions).

**When a Route is directly used to expose user workload**

- The end user must have generated the serving certificate as a pre requisite
  using third-party systems like cert-manager.
- In cert-manager's case, the [Certificate](https://cert-manager.io/docs/usage/certificate/#creating-certificate-resources) CR must be created in the same namespace
  where the Route is going to be created.
- To expose a user workload, the user would create a new Route with the
  secret-reference annotation referencing the generated secret that was created
  in the previous step.
- If the secret that is referenced exists and has a successfully generated
  cert/key pair, the secret-injector controller copies the certificate data
  into the route at `.spec.tls.certificate` and `.spec.tls.key`.

**When an Ingress is used to expose user workload**

- The end user must have generated the serving certificate as a pre requisite
  using third-party systems like cert-manager.
- In cert-manager's case, the [Certificate](https://cert-manager.io/docs/usage/certificate/#creating-certificate-resources) CR must be created in the same namespace
  where the Ingress is going to be created.
- To expose a user workload, a new Ingress with the generated secret
  referenced in `.spec.tls.secretName` needs to be created.
- If the secret CR that is referenced exists and has a successfully generated
  cert/key pair, the ingress-to-route controller copies this certificate data
  into the route at `.spec.tls.certificate` and `.spec.tls.key`.

_Note_: The same workflow would apply to updating an existing Route to integrate
with cert-manager. The new certificate data will overwrite the `.spec.tls.certificate`
and `.spec.tls.key` in the route CR since the secret-reference annotation gets a
higher preference.

#### Certificate Life cycle

Post integrating with third-party solutions like cert-manager, the end user can also
disable automatic certificate management. This can be done in 2 ways,

- The end user can delete the secret-reference annotation on the Route.
- The end user deletes the secret that was associated with the Certificate CR.

**Deleting the secret-reference annotation on the Route**
The end user can delete the annotation on the Route, this will result
in clearing `.spec.tls.certificate` and `.spec.tls.key` and resort to
using the default certificates that are generated by the cluster-ingress-operator.

**Deleting the generated secret that contains the serving cert/key pair**
The end user can delete the secret containing the certificate data, this will result
in clearing `.spec.tls.certificate` and `.spec.tls.key` and resort to using the
default certificates that are generated by the cluster-ingress-operator. Also the secret-reference
annotation that was added by the user will not be deleted by the secret-injector controller.

#### Variation [optional]

N.A

### API Extensions

A secret-reference annotation of type `string` to be introduced as part of the integration
to support third-party certificate management systems,

```yaml
annotations:
  route.openshift.io/tls-secret-name: <secret-name>
```

_Note_: The default value would be N/A. The secret is required to be created
in the same namespace as that of the Route. The secret must be of type
`kubernetes.io/tls` and the tls.key and the tls.crt key must be provided in
the `data` (or `stringData`) field of the Secret configuration.

This annotation can be applied by the end user on Route. The controller that
will be introduced as part of this enhancement will be responsible for the
processing of this annotation on the Route.

The Route API will also be updated to denote certificate status under `.status`,

```go

// RouteStatus provides relevant info about the status of a route, including which routers
// acknowledge it.
type RouteStatus struct {
    // ...

    // CertificateConditions describes the type of certificate that is being served
    // by the router(s) associated with this route.
    Certificate []RouteCertificateCondition `json:"certificate,omitempty" protobuf:"bytes,2,rep,name=certificate"`
}

// RouteCertificateConditionType is a valid value forRouteCertificateCondition
type RouteCertificateConditionType string

// RouteCertificateCondition contains details of the certificate being served by routers associated with
// this route.
type RouteCertificateCondition struct {
	// Type is the type of the condition.
    // Possible values include Default, Custom and Managed.
	Type RouteCertificateConditionType `json:"type" protobuf:"bytes,1,opt,name=type,casttype=RouteCertificateConditionType"`
	// Status is the status of the condition.
	// Can be True, False, Unknown.
	Status corev1.ConditionStatus `json:"status" protobuf:"bytes,2,opt,name=status,casttype=k8s.io/api/core/v1.ConditionStatus"`
	// (brief) reason for the condition's last transition, and is usually a machine and human
	// readable constant
	Reason string `json:"reason,omitempty" protobuf:"bytes,3,opt,name=reason"`
	// Human readable message indicating details about last transition.
	Message string `json:"message,omitempty" protobuf:"bytes,4,opt,name=message"`
	// RFC 3339 date and time when this condition last transitioned
	LastTransitionTime *metav1.Time `json:"lastTransitionTime,omitempty" protobuf:"bytes,5,opt,name=lastTransitionTime"`
}

const (
    // DefaultCertificate denotes that the user has not provided
    // any custom certificate and the default generated certificate is being
    // served on the route.
    DefaultCertificate RouteCertificateConditionType = "Default"

    // CustomCertificate denotes that there is a custom certificate
    // being served on the route.
    CustomCertificate RouteCertificateConditionType = "Custom"

    // ManagedCertificate denotes that the route.openshift.io/tls-secret-name
    // annotation is applied and is used to inject a third-party managed secret
    // containing the certificate data that is being served on the route.
    ManagedCertificate RouteCertificateConditionType = "Managed"
)

```

#### Variation

##### Alternative to the tls-secret-reference annotation

**_Note_**: This idea has been dropped in favor of a secret reference through an annotation.

The alternative to the annotation was to provide a new API field in the Route API,
through which the user could provide the certificate reference.

```go
type CertificateReference struct {
	// certificate resource name.
	// +optional
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
}

Type RouteSpec struct {
	// ...

	CertificateRef CertificateReference `json:"certificateRef,omitempty" protobuf:"bytes,5,opt,name=certificateRef"`
}
```

The reasoning for the field was mainly for future proofing the API to handle integrations
with other third-party certificate management systems. But this API introduces inconsistency
between how Ingresses are supported by cert-manager via `.spec.tls.secretName` and when using
Routes, the user would have to provide the Certificate CR reference.

This also adds coupling with external third-party API(s) and limits future enhancement to be
a more generic solution for integrating with third-party certificate management systems.

##### Alternative to new status condition types

An alternative to introducing 3 new condition types, would be to use `metadata.managedFields`
on Route to denote to the user which fields are managed by the secret-injector controller and
server-side apply would be used by the controller during updates to ensure that `.spec.tls.certificate`
and `.spec.tls.secret` are always managed by the controller as long as the annotation is present.

This although helps in avoiding addition of new condition types will result in a few inconsistencies,

- When transitioning from using managed certificates to manually managing custom certificates,
  the `.managedFields` will still depict `.spec.tls` as managed by the controller since the update
  operation done by the user is only on the `.metadata.annotations`.
- Also since `oc` defaults to `--server-side=false`, mixing CSA and SSA results in ambiguous `.managedFields`.

### Implementation Details/Notes/Constraints [optional]

The new controller (secret-injector) will be responsible for watching routes and processing those
that have the valid annotation for `tls-secret-name`. This controller will also update
`.status` of the route to contain the latest information on the certificate in use.

Since the secrets are the primary resource for the secret-injector controller, the new controller
will also set up watches and re-sync routes in the same namespace as that of the secret (similar
to ingress-to-route controller).

The new controller will watch all routes and process all routes in order to update
`.status.certificate`. The ones with the annotation will have additional
logic to update `.spec.tls.certificate` and `.spec.tls.key` based on the following pre-conditions,

- The secret-reference annotation is present.
- Secret mentioned in the annotation exists and is valid (contains the required fields mentioned [here](#api-extensions))

In scenarios where the user deletes the annotation, it is the controller's
responsibility to reset `.spec.tls` and this will be driven based on `.status` i.e.
the latest certificate status condition has to be `ManagedCertificate` and since the required annotation is
not present this results in the `.spec.tls.certificate` and `.spec.tls.key` being cleared
out and the `.status` updated to `DefaultCertificate`.

If the user deletes the secret associated with the Route without deleting the
`route.openshift.io/tls-secret-name` annotation, the secret-injector controller will
clear out `.spec.tls.certificate` and `.spec.tls.key` since the following pre-conditions
are met,

- The secret-reference annotation is present.
- Secret mentioned in the annotation doesn't exist
- The latest condition on Route's `.status.certificate` has `ManagedCertificate`=`True`

This results in the router using the default certificates that are generated and
the `.status` updated as well.

As a follow up to the above scenario when the secret-reference annotation added by the user is retained
by the secret-injector controller and because the referenced secret is not present,
the controller will publish an `Event` so that the end user is notified regarding this
switch to using the default generated certificates. The `.status` is also updated to `DefaultCertificate`.

When transitioning from using a managed certificate to manually managed certificate, the user
is expected to delete the secret-reference annotation and provide new certificate/key pair under `.spec.tls`.
If the previously used `.spec.tls.certificate` and `.spec.tls.key` is not replaced
by the user, the secret-injector controller will clear `.spec.tls.certificate` and `.spec.tls.key`
and route will serve the default certificates that are generated by cluster-ingress-operator.
If the `.spec.tls.certificate` and `.spec.tls.key` are replaced as well, then the
controller will update the `.status.certificate` to `CustomCertificate`

The new secret-injector controller will not reconcile Routes that are owned by the
ingress-to-route controller.

### Risks and Mitigations

N.A

### Drawbacks

This introduces an inconsistency between Ingress and Route CRD with respect to how
a secret reference can be provided with the certificate data. The ingress has a field
`.spec.tls.secretName` where as the Route will have an annotation.

## Design Details

### Open Questions [optional]

1. Does this warrant making changes to the ingress-to-route controller in how
   `.spec.tls.secretName` is processed? Currently, the controller reads the secret
   data and copies it over to the Route that is created. Since this enhancement
   introduces a difference between managed certificate provided via secrets and  
   manually provided secrets, this distinction isn't possible with Routes created
   via Ingresses. [CLOSED]

   - Proposed change
     The ingress-to-route controller will also process the secret-reference annotation and if
     present copies it over to the route that is created. This annotation will take
     precedence over `.spec.tls.secretName` and this offers enough distinction between
     third-party/manual certificate management.

**Answer**: The behaviour of the ingress-to-route controller will be retained as is and
no changes will be done.

2. How/where to implement e2e tests for [route-controller-manager](https://github.com/openshift/route-controller-manager)?
   [CLOSED]

**Answer**: The e2e tests will be added to openshift/origin.

### Test Plan

This enhancement will be tested in isolation of cert-manager-operator as part of
core OCP payload. This will also be tested with cert-manager-operator as part of the
operators e2e test suite.

1. Test Route API without interfacing with Ingresses

   a. Create a edge terminated route that is using default certificates.
   b. Update route with the secret-reference annotation and ensure the `.spec.tls` and `.status`
   is updated.
   c. Verify that the associated openshift routers are also serving the updated
   certificates and not the default certificates.
   d. Update certificate data and verify if the certificates served by the router
   are updated as well.
   d. Delete the annotation and ensure that the route `.spec.tls` and `.status`
   denotes the usage of default certificates. Also verify the associated
   openshift router is serving the default certificates.

Various other transitions, `Custom`->`Managed`-`Custom` will also be tested.

### Graduation Criteria

This is a user facing change and will directly go to GA. This feature requires an
update to Openshift Docs.

#### Dev Preview -> Tech Preview

N/A. This feature will go directly to GA.

#### Tech Preview -> GA

N/A. This feature will go directly to GA.

#### Removing a deprecated feature

N/A.

### Upgrade / Downgrade Strategy

On downgrades, all routes using `route.openshift.io/tls-secret-name` annotation will continue to use the custom certificates
indefinitely unless the route is manually edited and the `.spec.tls` is updated.

### Version Skew Strategy

This enhancement isn't affected by version skew of core Kubernetes components
during updates/upgrades. The new controller will be capable to re-synchronize
the required components if required and without this controller the annotation
would basically do nothing.

### Operational Aspects of API Extensions

N.A

#### Failure Modes

When using routes with third-party certificate management solutions like cert-manager, this
adds a hard dependency in order of operation. In scenarios where the secret has not been created
by third-party solutions like cert-manager, the route would not be successfully created due
to the dependency on the route.

> A solution to this could be that upon failure to use the secret due to various reasons,
> the controller defaults to using the default generated certificates and updating the route
> status appropriately. The controller also publishes an event indicating the switch has been made.

#### Support Procedures

The new introduced `RouteCertificateConditionType` types will provide the required information into
the current state of route objects with respect to certificates. While using third-party solutions
like cert-manager for certificate management the `RouteCertificateCondition` will indicate which type
of certificate is associated with the route.

## Implementation History

N.A

## Alternatives

N.A

## Infrastructure Needed [optional]

N.A
