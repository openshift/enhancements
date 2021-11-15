---
title: loki-lokistack-gateway-configuration

authors:
  - "@sasagarw"

reviewers:
  - "@periklis"

approvers:
  - "@periklis"
  - "@igor-karpukhin"

creation-date: 2021-08-27
last-updated: 2021-09-27
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# Loki Operator: LokiStack CR extension for the gateway configuration

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

LokiStack Gateway is a component deployed as part of Loki Operator. It provides secure access to Loki's distributor (i.e. for pushing logs) and query-frontend (i.e. for querying logs) via consulting an OAuth/OIDC endpoint for the request subject.

This proposal provides an overview of the required LokiStack API changes to provide a per-tenant OIDC configuration for the LokiStack gateway component.

_Note:_ We are simply providing an API to reconfigure an existing implementation. Most of this context does not need to be implemented as it is already satisfied by [observatorium-api](https://github.com/observatorium/api).

## Motivation

### Goals

The goals of this proposal are:

* Provide required LokiStack API changes to enable a per-tenant OIDC configuration for the gateway component.
* Declare per-tenant OIDC configuration optional but provide auto-configuration for OpenShift.
* Leave per tenant OIDC secrets as an administrator task before creating LokiStack CRs.

We will be successful when:

* Deploying a LokiStack CR with per-tenant OIDC configuration enables secure authentication and authorization to the Loki cluster.
* Deploying a LokiStack CR on OpenShift enables secure authentication and authorization with zero configuration to the Loki cluster.

### Non-Goals

### Risks and Mitigations

In general the key risks to provide the proposed API changes in form of a custom resource definition are:

**Risk:** Extra per-tenant boilerplate.

**Mitigation:** We mitigate this at least in OpenShift Logging mode with secure presets.

**Risk:** Maintaining compatibility to a third-party component `observatorium-api`.

**Mitigation:** Here we are co-maintainers and we certainly want to secure the cluster as Loki does not provide any access security features.

### User Stories

* As a loki cluster admin, I want The LokiStack CR with per-tenant OIDC configuration and static RBAC enables secure authentication and authorization for `static` mode.

* As a loki cluster admin, I want The LokiStack CR with per-tenant OIDC configuration and OPA endpoint enables secure authentication and authorization for `dynamic` mode.

* As a loki cluster admin, I want The LokiStack CR with zero configuration on OpenShift enables secure authentication and authorization for `openshift-logging` mode.

## Design Details

For configuring the LokiStack Gateway, we need information about OIDC provider and OpenPolicyAgent (OPA).

### lokistack_types.go

Add a new field `Tenants` to `LokiStackSpec` struct as below:

```go
// ModeType is the authentication/authorization mode in which LokiStack Gateway will be configured.
//
// +kubebuilder:validation:Enum=static;dynamic;openshift-logging
type ModeType string
// PermissionType is a LokiStack Gateway RBAC permission.
//
// +kubebuilder:validation:Enum=read;write
type PermissionType string

// SubjectKind is a kind of LokiStack Gateway RBAC subject.
//
// +kubebuilder:validation:Enum=user;group
type SubjectKind string

const (
  // Static mode asserts the Authorization Spec's Roles and RoleBindings
	// using an in-process OpenPolicyAgent Rego authorizer.
	Static ModeType = "static"
	// Dynamic mode delegates the authorization to a third-party OPA-compatible endpoint.
	Dynamic ModeType = "dynamic"
	// OpenshiftLogging mode provides fully automatic OpenShift in-cluster authentication and authorization support.
	OpenshiftLogging ModeType = "openshift-logging"

	// Write gives access to write data to a tenant.
	Write PermissionType = "write"
	// Read gives access to read data from a tenant.
	Read PermissionType = "read"

	// User represents a subject that is a user.
	User SubjectKind = "user"
	// Group represents a subject that is a group.
	Group SubjectKind = "group"
)

// LokiStackSpec defines the desired state of LokiStack
type LokiStackSpec struct {
  // Tenants defines the per-tenant authentication and authorization spec for the lokistack-gateway component.
	//
	// +optional
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Tenants Configuration"
	Tenants *TenantsSpec `json:"tenants,omitempty"`
}

// TenantsSpec defines the mode, authentication and authorization
// configuration of the lokiStack gateway component.
type TenantsSpec struct {
	// Mode defines the mode in which lokistack-gateway component will be configured.
	//
	// +required
	// +kubebuilder:validation:Required
	// +kubebuilder:default:=openshift-logging
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:select:static","urn:alm:descriptor:com.tectonic.ui:select:dynamic","urn:alm:descriptor:com.tectonic.ui:select:openshift-logging"},displayName="Mode"
	Mode ModeType `json:"mode"`
	// Authentication defines the lokistack-gateway component authentication configuration spec per tenant.
	//
	// +optional
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Authentication"
	Authentication []AuthenticationSpec `json:"authentication,omitempty"`
	// Authorization defines the lokistack-gateway component authorization configuration spec per tenant.
	//
	// +optional
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Authorization"
	Authorization *AuthorizationSpec `json:"authorization,omitempty"`
}

// AuthenticationSpec defines the OIDC configuration per tenant for lokiStack Gateway component.
type AuthenticationSpec struct {
	// TenantName defines the name of the tenant.
	//
	// +required
	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Tenant Name"
	TenantName string `json:"tenantName"`
	// TenantID defines the id of the tenant.
	//
	// +required
	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Tenant ID"
	TenantID string `json:"tenantId"`
	// OIDC defines the spec for the OIDC tenant's authentication.
	//
	// +required
	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OIDC Configuration"
	OIDC *OIDCSpec `json:"oidc"`
}

// OIDCSpec defines the OIDC configuration spec for lokiStack Gateway component.
type OIDCSpec struct {
 	// Secret defines the spec for the clientID, clientSecret and issuerCAPath for tenant's authentication.
	//
	// +required
	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Tenant Secret"
	Secret *TenantSecretSpec `json:"secret"`
	// IssuerURL defines the URL for issuer.
	//
	// +required
	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Issuer URL"
	IssuerURL string `json:"issuerURL"`
	// RedirectURL defines the URL for redirect.
	//
	// +required
	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Redirect URL"
	RedirectURL   string `json:"redirectURL"`
	GroupClaim    string `json:"groupClaim"`
	UsernameClaim string `json:"usernameClaim"`
}

// TenantSecretSpec is a secret reference containing name only
// for a secret living in the same namespace as the LokiStack custom resource.
type TenantSecretSpec struct {
	// Name of a secret in the namespace configured for tenant secrets.
	//
	// +required
	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors="urn:alm:descriptor:io.kubernetes:Secret",displayName="Tenant Secret Name"
	Name string `json:"name"`
}

// AuthorizationSpec defines the opa, role bindings and roles
// configuration per tenant for lokiStack Gateway component.
type AuthorizationSpec struct {
  // OPA defines the spec for the third-party endpoint for tenant's authorization.
	//
	// +optional
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OPA Configuration"
  OPA *OPASpec `json:"opa"`
  // Roles defines a set of permissions to interact with a tenant.
	//
	// +optional
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Static Roles"
  Roles []*RoleSpec `json:"roles"`
  // RoleBindings defines configuration to bind a set of roles to a set of subjects.
	//
	// +optional
	// +kubebuilder:validation:Optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Static Role Bindings"
  RoleBindings []*RoleBindingsSpec `json:"roleBindings"`
}

// OPASpec defines the opa configuration spec for lokiStack Gateway component.
type OPASpec struct {
  // URL defines the third-party endpoint for authorization.
	//
	// +required
	// +kubebuilder:validation:Required
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="OpenPolicyAgent URL"
    URL string  `json:"url"`
}

// RoleSpec describes a set of permissions to interact with a tenant.
type RoleSpec struct {
	Name        string           `json:"name"`
	Resources   []string         `json:"resources"`
	Tenants     []string         `json:"tenants"`
	Permissions []PermissionType `json:"permissions"`
}

// RoleBindingsSpec binds a set of roles to a set of subjects.
type RoleBindingsSpec struct {
	Name     string    `json:"name"`
	Subjects []Subject `json:"subjects"`
	Roles    []string  `json:"roles"`
}

// Subject represents a subject that has been bound to a role.
type Subject struct {
	Name string      `json:"name"`
	Kind SubjectKind `json:"kind"`
}
```

## Proposal

### LokiStack Gateway configuration using _Modes_

LokiStack Gateway can operate in 3 different modes based on user's choice.

* Static Mode - Enables authorization for an OIDC-authenticated subject based on the static lists of Roles and RoleBindings in `RbacSpec`.

* Dynamic Mode - Enables authorization for an OIDC-authenticated subject by delegating the authorization request to a third-party OPA-compatible endpoint.

* Openshift Logging Mode - Enables authorization for an OpenShift-authenticated subject by translating OPA authorization requests via [opa-openshift](https://github.com/observatorium/opa-openshift) to SubjectAccessReviews against the API server. This mode ensures a per-namespace tenant access for authenticated subjects.

### LokiStack CR configuration - _Static Mode_

In this mode it is mandatory to provide a static list of per-tenant OIDC configuration as well as static RBAC. The LokiStack gateway deployment will be deployed with access to a static Rego file to match the RbacSpec. The static Rego file represents an in-process OPA endpoint.

The LokiStack CR will look like this:

```yaml
tenants:
    mode: static
    authentication:
    - name: tenant-a
      oidc:
        issuerURL: https://127.0.0.1:5556/dex
        redirectURL: https://localhost:8443/oidc/tenant-a/callback
        usernameClaim: test
        groupClaim: test
    authorization:
      roleBindings:
      - name: tenant-a
        roles:
        - read-write
        subjects:
        - kind: user
          name: admin@example.com
      roles:
      - name: read-write
        permissions:
        - read
        - write
        resources:
        - logs
        tenants:
        - tenant-a
```

The LokiStack Gateway deployment will need to create a static rego file which will include:

```rego
package lokistack

import input
import data.roles
import data.roleBindings

default allow = false

allow {
  some roleNames
  roleNames = roleBindings[matched_role_binding[_]].roles
  roles[i].name == roleNames[_]
  roles[i].resources[_] = input.resource
  roles[i].permissions[_] = input.permission
  roles[i].tenants[_] = input.tenant
}

matched_role_binding[i] {
  roleBindings[i].subjects[_] == {"name": input.subject, "kind": "user"}
}

matched_role_binding[i] {
  roleBindings[i].subjects[_] == {"name": input.groups[_], "kind": "group"}
}
```

### LokiStack CR configuration - _Dynamic Mode_

In this mode it is mandatory to provide a static list of per-tenant OIDC configuration and a URL to an OPA endpoint. Latter is used to delegate the subject's information to access the requested tenant.

_Note:_ Mixing static RBAC with dynamic mode results in a LokiStack CR degraded condition.

The LokiStack CR will look like this:

```yaml
tenants:
    mode: dynamic
    authentication:
    - name: tenant-a
      oidc:
        issuerURL: https://127.0.0.1:5556/dex
        redirectURL: https://localhost:8443/oidc/tenant-a/callback
        usernameClaim: test
        groupClaim: test
    authorization:
      opa:
        url: http://opa.example.org/v1/data/lokistack/allow
```

### LokiStack CR configuration - _Openshift Logging Mode_

In this mode, user will need to do zero configuration as everything will be auto-configured by Loki operator itself.

The auto-configuration in this mode includes creating per-tenant (application, audit and infrastructure) subjects and translating OPA authorization requests via `opa-openshift` to SubjectAccessReviews against the API server. This ensures a per-namespace tenant access for authenticated subjects.

We use these three categories (application, audit and infrastructure) as three big tenants in Loki because using namespaces as tenants will make our chunks too small. We ensure namespace-scoping by adding them as label-selectors, e.g.:

Let's say a user in OpenShift requires access to the logs of pod `my-pod` on namespace `my-project`, the call to lokistack will look like (translated in curl call for easier comprehension):

```console
$ curl -G -s  "http://lokistack.svc:8080/api/logs/v1/application/loki/api/v1/query_range" --data-urlencode 'query={pod_name="my-pod"}' --data-urlencode 'step=300' 
```

The lokistack-gateway will ask the authorizer sidecar `opa-openshift` and will retrieve the list of projects the user has access to and in turn amend the query from: `{pod_name="my-pod"}` to `{pod_name="my-pod", namespace=~"my-project"}`

Thus the final upstream call to loki looks like (translated in curl call for easier comprehension):

```console
$ curl -G -s -H "X-Scope-Org-ID: application"  "http://query-frontend.svc:8080/loki/api/v1/query_range" --data-urlencode 'query={pod_name="my-pod", namespace=~"my-project|my-ther-project"}' --data-urlencode 'step=300' 
```

The LokiStack CR which will look like this:

```yaml
tenants:
    mode: openshift-logging
```

The LokiStack Gateway deployment will need to configure everything for the user:

```yaml
tenants:
- name: application
  id: 32e45e3e-b760-43a2-a7e1-02c5631e56e9
  oidc:
    clientID: test
    clientSecret: test
    issuerCAPath: ./tmp/certs/ca.pem
    issuerURL: https://127.0.0.1:5556/dex
    redirectURL: https://localhost:8443/oidc/application/callback
    usernameClaim: name
  opa:
    url: http://127.0.0.1:8080/v1/data/lokistack/allow
- name: infrastructure
  id: 40de0532-10a2-430c-9a00-62c46455c118
  oidc:
    clientID: test
    clientSecret: ZXhhbXBsZS1hcHAtc2VjcmV0
    issuerCAPath: ./tmp/certs/ca.pem
    issuerURL: https://127.0.0.1:5556/dex
    redirectURL: https://localhost:8443/oidc/infrastructure/callback
    usernameClaim: name
  opa:
    url: http://127.0.0.1:8080/v1/data/lokistack/allow
- name: audit
  id: 26d7c49d-182e-4d93-bade-510c6cc3243d
  oidc:
    clientID: test
    clientSecret: test
    issuerCAPath: ./tmp/certs/ca.pem
    issuerURL: https://127.0.0.1:5556/dex
    redirectURL: https://localhost:8443/oidc/audit/callback
    usernameClaim: name
  opa:
    url: http://127.0.0.1:8080/v1/data/lokistack/allow
```

### Creating Secrets

The OIDC configuration also expects `clientID`, `clientSecret` and `issuerCAPath` which should be provided via a Kubernetes secret that the LokiStack admin provides upfront.

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: lokiStack-gateway-tenant-a
data:
  clientID: test
  clientSecret: ZXhhbXBsZS1hcHAtc2VjcmV0
  issuerCAPath: /tmp/certs/ca.pem
```

Each tenant Secret is required to match:
* `metadata.name` with `TenantsSecretsSpec.Name`.
* `metadata.namespace` with `LokiStack.metadata.namespace`.

### Test Plan

#### Unit Testing

The [loki-operator](https://github.com/viaq/loki-operator) includes a framework based on [counterfeiter](github.com/maxbrunsfeld/counterfeiter) to create simple and useful fake client to test configuration in unit tests.

Testing of the reconciliation of a `LokiStack` custom resource will be based upon the same technique where the custom resource describes the inputs and the generated manifests the outputs.

#### Functional Testing

The loki-operator uses [testify](github.com/stretchr/testify) to do functional testing.

- Verify that the user is able to configure the modes correctly.
- Verify that the configMap is correctly getting generated with both tenants and rbac information.
- Verify that the secrets are correctly mapped to it's corresponding OIDCSpec.
- Verify that the runtime rbac and tenant values take precedence over the default values.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback by switching the internal Loki service to be managed by the [loki-operator](https://github.com/viaq/loki-operator).

#### Tech Preview -> GA

- More testing
- Sufficient time for feedback
- Available by default
- Conduct load testing per t-shirt size supported.

#### Removing a deprecated feature

Not applicable here.

### Upgrade / Downgrade Strategy

Not applicable here.

### Version Skew Strategy

Given we are adding a new API there should be minimal concern for upgrading.

## Implementation History

| Release | Description              |
| ---     | ---                      |
| TBD     | **TP** - Initial release |
| TBD     | **GA** - Initial release |

## Drawbacks

The drawback to not implementing this feature is we are unable to integrate Loki access to our org's OIDC provider and the current multi-tenant features of OpenShift Logging.

The multi-tenant feature of Openshift Logging enables namespace-based tenancy since Openshift maintains this global view on resourses. The developer has access only to namespaces (most of the times only application workload namespaces) granted by RBAC. The administrator has access to all namespaces including audit logs.

## Alternatives

TBD
