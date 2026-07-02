---
title: service-account-deprecation
authors:
  - "@pedjak"
reviewers:
  - "@joelanford"
approvers:
  - "@joelanford"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-10-08
last-updated: 2026-06-19
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-3040
status: implementable
see-also:
  - https://github.com/openshift/enhancements/pull/1860
  - https://github.com/openshift/enhancements/pull/1897
---

# Service Account Field Deprecation

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented
- [ ] Test plan is defined
- [ ] Graduation criteria for tech preview and GA
- [ ] User-facing documentation is created in [openshift-docs][]
- [ ] Operational readiness criteria are defined

## Summary

Deprecate the `spec.serviceAccount` field from the `ClusterExtension` API in the
operator-controller and grant the controller cluster-admin privileges. OLMv1 is a
[single-tenant system][ocp-ce-permissions]
where users with `ClusterExtension` write access are effectively delegated cluster-admin trust.
The `spec.serviceAccount` field was originally introduced to enforce least-privilege by
requiring per-extension ServiceAccounts with fine-grained RBAC, but in practice this model
provides a false sense of security while imposing significant UX complexity and code
maintenance burden. This proposal removes the requirement and simplifies the controller
architecture. Access control shifts from per-extension ServiceAccount RBAC to standard
Kubernetes RBAC on who can create `ClusterExtension` resources. A
[ValidatingAdmissionPolicy][k8s-vap]
is used to emit deprecation warnings when the field is set.

## Motivation

The `spec.serviceAccount` field was introduced so that OLMv1 could enforce least-privilege:
cluster-admins would create a purpose-built ServiceAccount with precisely scoped RBAC for
every extension they install, and the controller would impersonate that ServiceAccount for
all cluster interactions on behalf of the extension. The
[required permissions procedure][ocp-ce-required-rbac]
documents this workflow: administrators must inspect bundle manifests, identify every
cluster-scoped and namespace-scoped resource, and manually construct matching ClusterRoles
and RoleBindings — a process that must be repeated on every upgrade when bundle content changes.

Beyond the UX burden, the ServiceAccount model provides a false security boundary:

- **Any ClusterExtension writer can reference any ServiceAccount in any namespace they have
  access to.** If a cluster-admin creates a highly-privileged ServiceAccount for one purpose,
  any user who can create a `ClusterExtension` can reference it for another. The field does not
  prevent privilege escalation — it merely adds a layer of indirection. See
  [Kubernetes RBAC escalation semantics][k8s-rbac-escalation].
- **Extension content is inherently cluster-scoped.** Operators install CRDs, ClusterRoles,
  webhooks, and other cluster-scoped resources. True per-extension least-privilege is impossible
  without restricting what extensions can install, which would break most operators.
- **Internal implementation details leak into required permissions.** The controller needs
  informers for managed resources, which requires list/watch permissions that users must include
  in the ServiceAccount's RBAC — but these are implementation details that change when internals
  change, creating fragility.


Although OLMv1 is GA in OpenShift, production adoption by customers remains minimal. The
field is being deprecated before significant real-world usage develops, making this the
optimal time for the change.

Telemetry data from approximately 199,000 clusters reporting to Red Hat shows that only
59 clusters have any `ClusterExtension` resources, with a total of 80 ClusterExtensions
across the fleet. Of those 59 clusters, 17 are registered as production in OpenShift
Cluster Manager, and only 6 can be confirmed as genuine customer production environments
(across 5 organizations, totaling 11 ClusterExtensions).
<!-- TODO: Add customer impact statement from @dmesser -->

Additionally, the cluster-admin scope enables future UX improvements such as automatic
namespace creation on behalf of extensions, eliminating the requirement for cluster admins
to pre-create installation namespaces.

### User Stories

**New installation without ServiceAccount:** As a cluster admin, I want to install a
ClusterExtension without creating a ServiceAccount and deriving complex RBAC, so that the
installation process is simple and I can focus on choosing the right extension rather than
managing its permissions.

**Existing installation with ServiceAccount set:** As a cluster admin with existing
ClusterExtensions that specify `spec.serviceAccount`, I want the system to continue functioning
after upgrade so that my running extensions are not disrupted, and I want clear deprecation
signals so I know to update my manifests.

**Scoped delegation via RBAC and ValidatingAdmissionPolicy:** As a cluster admin, I want to
allow a non-admin user to install only specific extensions by configuring RBAC and
ValidatingAdmissionPolicy to control who can create or modify ClusterExtension resources and
what values are permitted, without requiring per-extension ServiceAccounts.

### Goals

- Deprecate `spec.serviceAccount` — the field becomes optional, is ignored by the controller,
  and emits a deprecation warning via
  [ValidatingAdmissionPolicy][k8s-vap]
  when set.
- Grant the operator-controller cluster-admin privileges so that cluster admins no longer need
  to create, maintain, or derive ServiceAccount RBAC to install extensions.
- Preserve backward compatibility — existing ClusterExtensions with `spec.serviceAccount` set
  continue to function after upgrade without any user action.
- Remove the per-ServiceAccount infrastructure (alpha feature gates `PreflightPermissions`
  and `SyntheticPermissions`, impersonation code, per-extension caches) to simplify the
  controller architecture.
- Provide documentation and examples for using RBAC and ValidatingAdmissionPolicy to control
  access to ClusterExtension writes as a replacement for per-SA scoping.

### Non-Goals

- Multi-tenant permission models or namespace-scoped extension management.
- Removal of `spec.serviceAccount` from the API. This proposal covers deprecation only; field
  removal is deferred pending architectural guidance on stable API field removal.
- Building new permission-requesting or permission-derivation systems to replace
  per-ServiceAccount scoping.
- Console/UI changes to remove the ServiceAccount field from ClusterExtension workflows
  (tracked separately).

## Proposal

### Workflow Description

#### Workflow 1: New ClusterExtension installation (no ServiceAccount)

1. The cluster admin creates the installation namespace (if not already present).
2. The cluster admin applies a `ClusterExtension` resource specifying only `namespace`, `source`,
   and optionally `install` configuration — no `serviceAccount` field.
3. The operator-controller resolves the bundle from the configured catalog source.
4. The operator-controller unpacks and applies the bundle content using its own cluster-admin
   ServiceAccount.
5. The `ClusterExtension` status shows the installed bundle version and a `Progressing=False`
   condition.

#### Workflow 2: Existing ClusterExtension with ServiceAccount (upgrade path)

1. The cluster admin upgrades OLMv1 to the deprecation release.
2. During upgrade, `cluster-olm-operator` deploys the new operator-controller manifests.
   Because the ClusterRoleBinding's `roleRef` is [immutable in Kubernetes][k8s-rbac-crb], the
   binding cannot be updated in-place from the old custom ClusterRole to `cluster-admin`. Instead,
   a new ClusterRoleBinding (`operator-controller-cluster-admin-rolebinding`) is created and the
   old ClusterRoleBinding and custom ClusterRole are cleaned up by `cluster-olm-operator`.
3. Existing ClusterExtensions with `spec.serviceAccount` set continue to reconcile normally.
   The field is accepted but ignored — the controller uses its own ServiceAccount.
4. On any `kubectl create` or `kubectl apply` that includes the `spec.serviceAccount` field,
   the ValidatingAdmissionPolicy emits a warning:
   `spec.serviceAccount is deprecated, ignored, and will be removed in a future release.`
5. The controller logs an INFO-level deprecation message on each reconciliation of a
   ClusterExtension with the field set.
6. The cluster admin can clear the field at any time — the relaxed immutability rule allows
   setting `serviceAccount.name` to an empty string.

#### Workflow 3: Scoped access control via RBAC

1. The cluster admin creates RBAC (Roles/ClusterRoles and bindings) to control which users or
   groups can create, update, or delete ClusterExtension resources.
2. This replaces per-ServiceAccount scoping with standard Kubernetes access control mechanisms.
   The security boundary is who can write ClusterExtension resources, not which ServiceAccount
   is referenced.
3. Documentation and examples for common delegation scenarios are provided as part of
   this EP's deliverables ([OPRUN-4674][]), including optional use of
   ValidatingAdmissionPolicies to further restrict allowed values (e.g., package names,
   catalog sources, installation namespaces).

#### Default RBAC and delegation model

By default, only cluster-admins have write access to `ClusterExtension` and
`ClusterObjectSet` resources — there is no delegation and no ClusterRole aggregation out of
the box. This EP does not change the default RBAC. The cluster-admin grant to the
operator-controller is safe precisely because the only users who can create
ClusterExtensions already have cluster-admin equivalent trust.

When a cluster admin wants to delegate ClusterExtension management to non-admin users, two
mechanisms are required together:

- **RBAC** opens the authorization gate so the user can create/update ClusterExtension
  resources. RBAC alone is dangerous because ClusterExtension write access is
  cluster-admin-equivalent — the user could install any extension from any catalog.
- **ValidatingAdmissionPolicy** restricts *what* a user can do with ClusterExtensions (e.g.,
  restrict by package name, catalog source, target namespace). VAP alone has no effect
  because users still lack permission to touch ClusterExtensions at all.

Neither mechanism is sufficient alone. The documentation deliverable ([OPRUN-4674][])
covers common delegation patterns with examples.

### API Extensions

#### ClusterExtension API change

The `spec.serviceAccount` field changes from required to optional and deprecated:

**Before (current):**
```go
// serviceAccount specifies a ServiceAccount used to perform all interactions
// with the cluster that are required to manage the extension.
// The ServiceAccount must be configured with the necessary permissions to
// perform these interactions.
// The ServiceAccount must exist in the namespace referenced in the spec.
// The serviceAccount field is required.
//
// +required
ServiceAccount ServiceAccountReference `json:"serviceAccount"`
```

**After (deprecated):**
```go
// serviceAccount is a deprecated field and is completely ignored.
// OLMv1 is a single-tenant system where users with ClusterExtension write
// access are effectively delegated cluster-admin trust. The operator-controller
// runs with cluster-admin privileges and uses its own service account for all
// cluster interactions.
//
// Deprecated: serviceAccount is no longer used and will be removed in a future
// release.
//
//nolint:kubeapilinter // deprecated field uses omitzero per OpenShift convention;
// pointer would be a breaking API change
//
// +optional
ServiceAccount ServiceAccountReference `json:"serviceAccount,omitzero"`
```

Key design decisions:

- **`omitzero` instead of pointer.** OpenShift API conventions advise against pointer types for
  CRD fields. Using `omitzero` (Go 1.24) allows the field to be omitted from serialized JSON
  when it is the zero value, without changing the Go type signature. A pointer change would be
  a breaking API change for existing clients. See
  [OpenShift API coding style][ocp-api-coding-style].

- **Godoc deprecation format.** The field description comes first, followed by a `Deprecated:`
  paragraph at the end of the comment block. This satisfies both
  [OpenShift API field conventions][ocp-api-field-conventions]
  and Go `staticcheck` deprecation detection.

- **Relaxed immutability.** The `ServiceAccountReference.Name` field's XValidation rule changes
  from `self == oldSelf` to `self == oldSelf || size(self) == 0`, allowing users to clear the
  field as part of migration. The DNS validation rule is also made conditional:
  `size(self) == 0 || self.matches(...)`.

#### ValidatingAdmissionPolicy for deprecation warning

Since `x-kubernetes-deprecated` is [not available][k8s-crd-deprecated-issue] for CRD fields, a
[ValidatingAdmissionPolicy][k8s-vap]
with `validationActions: [Warn]` is used to emit kubectl warnings at admission time:

```yaml
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicy
metadata:
  name: clusterextension-serviceaccount-deprecated
spec:
  matchConstraints:
    resourceRules:
      - apiGroups: ["olm.operatorframework.io"]
        apiVersions: ["v1"]
        operations: ["CREATE", "UPDATE"]
        resources: ["clusterextensions"]
  validations:
    - expression: >-
        !has(object.spec.serviceAccount) ||
        !has(object.spec.serviceAccount.name) ||
        object.spec.serviceAccount.name == ''
      message: >-
        spec.serviceAccount is deprecated, ignored, and will be removed in a
        future release. The operator-controller's cluster-admin service account
        is used for all cluster interactions.
---
apiVersion: admissionregistration.k8s.io/v1
kind: ValidatingAdmissionPolicyBinding
metadata:
  name: clusterextension-serviceaccount-deprecated
spec:
  policyName: clusterextension-serviceaccount-deprecated
  validationActions:
    - Warn
```

The policy uses `Warn` validation action — it does not block admission. The warning is
returned as an HTTP `Warning` header in the [API response][k8s-api-warnings], making it
visible to any API client (kubectl, client-go, web consoles, CI tools), not just kubectl.

#### YAML examples

**Before (serviceAccount was required):**
```yaml
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: argocd-extension
spec:
  namespace: argocd
  source:
    sourceType: Catalog
    catalog:
      packageName: argocd-operator
      version: "0.6.0"
  serviceAccount:
    name: argocd-installer
```

**After (serviceAccount is optional and ignored):**
```yaml
apiVersion: olm.operatorframework.io/v1
kind: ClusterExtension
metadata:
  name: argocd-extension
spec:
  namespace: argocd
  source:
    sourceType: Catalog
    catalog:
      packageName: argocd-operator
      version: "0.6.0"
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

OLMv1 is not currently available in HyperShift environments. HyperShift support is tracked
separately in [OCPSTRAT-1818][]. This EP does not affect the HyperShift timeline.

#### Standalone Clusters

Primary deployment target. All changes apply as described.

#### Single-node Deployments or MicroShift

No additional impact. The removal of per-SA caches reduces memory consumption,
which benefits resource-constrained environments.

#### OpenShift Kubernetes Engine

No dependencies on features excluded from OKE.

### Implementation Details/Notes/Constraints

#### RBAC changes

The operator-controller's ClusterRoleBinding is renamed from
`operator-controller-manager-rolebinding` to `operator-controller-cluster-admin-rolebinding`
and its `roleRef` changes from a custom `operator-controller-manager-role` ClusterRole to the
built-in `cluster-admin` ClusterRole.

The ClusterRoleBinding was renamed (not updated in-place) because
[`roleRef` is immutable][k8s-rbac-crb]
in Kubernetes ClusterRoleBindings. The old ClusterRoleBinding and custom ClusterRole are
cleaned up by `cluster-olm-operator` during the upgrade.

An alternative considered was updating the existing custom ClusterRole's rules to match
`cluster-admin`'s wildcard rules, avoiding the rename and cleanup. This was rejected because
the built-in `cluster-admin` role has special superuser handling in the Kubernetes RBAC
authorizer, binding directly to it is self-documenting, and a custom role with wildcard
rules is effectively an unnamed alias that adds indirection without benefit.

#### Controller logic changes

When `spec.serviceAccount` is set, the controller logs an INFO-level deprecation message
during reconciliation. Reconciliation proceeds normally — the field is accepted but completely
ignored. The controller uses its own cluster-admin ServiceAccount for all cluster interactions.

All ServiceAccount impersonation infrastructure is removed: token acquisition, REST config
mapping, ServiceAccount validation, and per-SA annotations. The controller no longer builds
per-extension clients or manages per-extension credentials.

#### Cache consolidation

The per-ClusterExtension/ServiceAccount scoped informer caches are replaced with a single
shared cache from the [boxcutter][boxcutter] project. The shared cache tracks managed resource
types per ClusterExtension rather than per ServiceAccount, reducing memory consumption and
simplifying the cache lifecycle. When the boxcutter feature gate is enabled,
`ClusterObjectSet` resources also use the operator-controller's cluster-admin ServiceAccount.

#### Downstream deprecation tracking

In OpenShift, [`cluster-olm-operator`][cluster-olm-operator] manages OLMv1 and reports its
status using [standard ClusterOperator conditions][ocp-clusteroperator-conditions]. This
existing mechanism already blocks cluster upgrades when ClusterExtensions install bundles
incompatible with the next OpenShift version.

The same mechanism is used for deprecation tracking in two phases:

**Phase 1 (deprecation release):** `cluster-olm-operator` detects ClusterExtensions
that still have `spec.serviceAccount` set and reports `EvaluationConditionsDetected=True`.
`EvaluationConditionsDetected` is a
[standard ClusterOperator condition type][ocp-clusteroperator-conditions] used to surface
advisory information. This does not block upgrades.

**Phase 2 (pre-removal release):** The condition switches to
`Upgradeable=False`, blocking the upgrade to the release that would remove the field. Cluster
admins must clear `spec.serviceAccount` from all ClusterExtensions before upgrading.

### Risks and Mitigations

#### Risk: Privilege escalation via automatic upgrades

With cluster-admin, the operator-controller stamps out whatever manifests a bundle contains. If
a bundle upgrade introduces malicious RBAC or workloads, OLM applies them without restriction.

**Mitigation:** This risk already exists in the current model. Any user who can create a
ClusterExtension can reference any ServiceAccount in any namespace they have access to,
including highly-privileged ones. The ServiceAccount field does not meaningfully prevent
privilege escalation — it only adds a layer of indirection.

The real security boundary is who can create ClusterExtension resources. Cluster admins must
treat ClusterExtension write access as equivalent to cluster-admin delegation. Catalogs provide
a curation layer: admins choose which catalogs to install and can restrict to vetted sources
(Red Hat certified, marketplace). Future OCI signing and provenance verification can add
supply-chain trust. Documentation for access control via RBAC and custom
ValidatingAdmissionPolicies is a deliverable of this EP ([OPRUN-4674][]).

#### Risk: Loss of permission-change signal on upgrades

Previously, if a ServiceAccount lacked permissions for a bundle upgrade, the installation
failed with a clear error. With cluster-admin, the upgrade proceeds and any permission
escalation in the bundle is silently applied.

This is an accepted trade-off. The SA-based signal was incidental — a side effect of
insufficient permissions, not a deliberate auditing mechanism. In practice, most users
granted broad permissions to avoid these failures, so the signal was rarely useful.
Administrators who need to restrict what can be installed can deploy
ValidatingAdmissionPolicies to control which package names, catalog sources, and
installation namespaces are permitted in ClusterExtension resources.

#### Risk: Broader blast radius if operator-controller is compromised

A compromised operator-controller pod with cluster-admin is more dangerous than one with limited
permissions.

**Mitigation:** The operator-controller pod runs with standard security hardening:
- Read-only root filesystem
- Non-root UID
- Dropped capabilities
- Bound ServiceAccount token volumes (short-lived tokens)
- Network policies restricting traffic

This is the same risk profile as every other cluster-admin controller in OpenShift
(kube-controller-manager, cluster-version-operator, machine-api-operator, etc.). The
operator-controller is no more or less a target than these existing components.

#### Risk: CRD schema change from required to optional

Changing a field from required to optional in the CRD schema could cause issues during upgrade
if validation differs between old and new versions.

**Mitigation:** Required-to-optional is a safe CRD schema change (additive, not restrictive).
Existing resources that have the field set remain valid under the new schema. New resources
that omit the field are also valid. No data migration is needed. The `omitzero` serialization
tag ensures that the zero-valued struct is omitted from JSON output, which is consistent with
standard Kubernetes API conventions.

### Drawbacks

- **Reduced permission granularity.** The per-extension ServiceAccount model, despite its
  practical flaws, provided a mechanism for scoping permissions per extension. This mechanism
  is now gone. For environments where per-extension isolation is a hard requirement,
  separate clusters or namespace-level isolation at the workload layer must be used instead.

- **Increased trust surface.** The operator-controller is now a higher-value target. Any
  vulnerability in the controller or in the extension resolution/unpacking pipeline carries
  more impact because the controller can write any resource. In practice, however, the
  controller's existing permission set already includes the ability to mint credentials for
  any ServiceAccount in the cluster, making it effectively cluster-admin with one additional
  step. The explicit cluster-admin grant makes the trust level visible rather than hidden.

- **User transition friction.** Users who invested in ServiceAccount and RBAC management for
  their extensions may be frustrated that this work is no longer relevant. Clear deprecation
  warnings and migration documentation mitigate this, and the minimal production adoption of
  OLMv1 limits the affected user base.

## Open Questions

1. **Timeline for field removal from the API.** The
   [OpenShift API compatibility policy][ocp-api-tiers] allows Tier 1 API elements to be removed
   in a subsequent major release by incrementing the API group version, but in practice this has
   rarely been done. Should `spec.serviceAccount` remain in the API indefinitely as deprecated
   and ignored, or be removed in a future API version bump? This EP proposes the
   `Upgradeable=False` mechanism to ensure the field is cleared before any future removal, but
   defers the removal decision to a separate proposal.

2. **RBAC + VAP documentation scope.** How detailed should the RBAC and ValidatingAdmissionPolicy
   examples be in this EP versus in separate documentation?

## Alternatives (Not Implemented)

### Keep ServiceAccount functional during the deprecation period

[Raised by @everettraven on EP #1860][EP #1860]:
deprecated fields should still function as-is until fully removed.

This would mean keeping the per-SA infrastructure (token acquisition, REST config mapping,
RBAC pre-authorization, per-extension caches) through one or more additional release cycles.
This code is tightly coupled to the current reconciliation model and incompatible with the
boxcutter revision rollout architecture. Maintaining two parallel reconciliation paths
(SA-impersonation for the current runtime, cluster-admin for boxcutter) would double the
testing surface and create confusing behavioral differences depending on runtime.

Given that OLMv1, although GA, has minimal production adoption by customers, the engineering
cost of maintaining this infrastructure does not justify the backward compatibility benefit.

### Three paths: no SA, namespace admin, full least-privilege

[Raised by @JoelSpeed on EP #1860][EP #1860]:
instead of deprecating, fix the SA model by offering three tiers: (1) no SA (cluster-admin
default), (2) namespace-scoped admin SA, (3) fully derived least-privilege SA.

Options 2 and 3 still require the full SA-impersonation infrastructure plus the
derive-service-account tooling. The privilege escalation issue in the SA reference model
remains — any ClusterExtension writer can reference any ServiceAccount in any namespace
they have access to, regardless of which tier is chosen. Building better UX around a
fundamentally flawed security model does not solve the core problem.

JoelSpeed also noted: "I don't think we can do this until we have something new to point
users to that replaces this." The replacement is standard Kubernetes RBAC and
ValidatingAdmissionPolicy for access control, documented as part of this EP's deliverables
([OPRUN-4674][]).

### Better client tooling for ServiceAccount derivation

[Raised by @everettraven on EP #1860][EP #1860]:
build CLI tooling to automatically derive and create the SA and RBAC needed for a given bundle.

Auto-generating RBAC requires the tool to have broad read permissions to inspect bundle content
and map it to policy rules — effectively cluster-admin with extra steps. The
derive-service-account guide acknowledged the complexity but tooling cannot
fundamentally change the fact that bundle content is inherently cluster-scoped and the
permission set changes on every upgrade. Additionally, client tooling would need to know the
controller's internal permission requirements (e.g., list/watch for informers), which are
implementation details that change from release to release. There is also a chicken-and-egg
problem: the ClusterExtension simultaneously performs resolution and applies the bundle, but
the user cannot know which bundle will be resolved without pinning the version, meaning they
cannot correctly configure least-privilege permissions. This forces a two-step process where
the ClusterExtension enters a failure state between steps — a poor UX.

### Permission request/approval system

[Raised by @everettraven on EP #1860][EP #1860]:
OLM creates a "permission request" resource that an admin can approve or deny.

This adds a new API and approval workflow for a system that only cluster-admins use. If the
person creating the ClusterExtension is already a cluster-admin, there is no one else to
approve. For delegation scenarios, standard Kubernetes RBAC is the appropriate mechanism and
does not require a custom approval workflow.

### Lock down ServiceAccount references

[Raised by @JoelSpeed on EP #1860][EP #1860]:
instead of removing ServiceAccount, fix the privilege escalation by restricting which users
can reference which ServiceAccounts, or which ServiceAccounts can be attached to a
ClusterExtension.

This addresses the escalation vector but does not resolve the other problems: the UX
complexity of deriving and maintaining per-extension RBAC remains, the controller's internal
permission needs still leak into user-facing configuration, and the chicken-and-egg problem
between resolution and permission configuration is unchanged. The replacement access control
model (RBAC + ValidatingAdmissionPolicy) provides equivalent scoping without requiring
per-extension ServiceAccounts.

### Keep ServiceAccount but automate its creation

If operator-controller creates RBAC, it needs cluster-admin permissions to do so — making
the ServiceAccount an unnecessary indirection layer. The complexity moves from the user to
the controller without reducing the trust surface.

### Synthetic permissions / impersonation model

[EP #1897][] proposed making
`spec.serviceAccount` optional and using a synthetic permissions model for users who do not
specify one.

This added complexity without addressing the fundamental single-tenant model: OLMv1
explicitly does not support multi-tenancy, so the additional permission indirection did not
serve a real security purpose. The proposal was closed.

### Use the API removal notification mechanism

OpenShift has an existing [API removal notification system][api-removal-notifications] that
alerts administrators when deprecated APIs are in active use before an upgrade. However, this
mechanism tracks whole-API-version removal (e.g., `certificates.k8s.io/v1beta1` being removed
in a future release), not field-level deprecation within a stable API version. It relies on
the `apiserver_requested_deprecated_apis` metric and the `APIRequestCount` resource, both of
which operate at the API group/version/resource level. A deprecated field within
`olm.operatorframework.io/v1` `ClusterExtension` — where the API version itself is not being
removed — is outside the scope of this mechanism.

## Test Plan

- Install, update, uninstall, and recovery scenarios pass with ClusterExtensions that omit
  `spec.serviceAccount`.
- Creating or updating a ClusterExtension with `spec.serviceAccount` set produces a
  deprecation warning in the API response.
- Creating or updating a ClusterExtension without `spec.serviceAccount` produces no warning.
- The `serviceAccount.name` field can be cleared (set to empty string) but not changed to a
  different non-empty value.
- Upgrade test: existing ClusterExtensions with `spec.serviceAccount` set continue to
  function after upgrading operator-controller.
- `cluster-olm-operator` reports `EvaluationConditionsDetected=True` when any
  ClusterExtension has `spec.serviceAccount` set.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A. The `spec.serviceAccount` field is a GA feature being deprecated, not a new feature
progressing through maturity stages.

### Tech Preview -> GA

N/A. See above.

### Removing a deprecated feature

**Deprecation release (this EP):**
- `spec.serviceAccount` becomes optional (removed from the `required` list in the CRD).
- ValidatingAdmissionPolicy emits a warning on create/update when the field is set.
- Controller logs a deprecation INFO message during reconciliation.
- Controller ignores the field — uses its own cluster-admin ServiceAccount for all interactions.
- All SA impersonation, authentication, authorization, and synthetic permissions code removed,
  including the `PreflightPermissions` and `SyntheticPermissions` feature gates (neither is
  in the Default feature set).
- `cluster-olm-operator` sets `EvaluationConditionsDetected=True` with reason
  `DeprecatedServiceAccountInUse` when any ClusterExtension has the field set. This is advisory
  and does not block upgrades.

**Following release (monitoring period):**
- Field remains in the API, deprecated and ignored.

**Subsequent release (potential upgrade blocker):**
- `cluster-olm-operator` switches to `Upgradeable=False` with reason
  `DeprecatedServiceAccountMustBeCleared` when any ClusterExtension has the field set.
  This blocks upgrade to the release that would remove the field, forcing cluster admins to
  clear the field first.

**Field removal (future, deferred):**
- Requires architectural guidance on stable API field removal in OpenShift.
- Not part of this EP's scope. See Open Questions.

## Upgrade / Downgrade Strategy

### Upgrading to the deprecation release

During the upgrade, `cluster-olm-operator` deploys the updated operator-controller
manifests:

1. The new ClusterRoleBinding `operator-controller-cluster-admin-rolebinding` is created,
   binding the operator-controller's ServiceAccount to the `cluster-admin` ClusterRole.
2. The old ClusterRoleBinding (`operator-controller-manager-rolebinding`) and custom ClusterRole
   (`operator-controller-manager-role`) are cleaned up by `cluster-olm-operator`.
3. The CRD schema is updated: `serviceAccount` is removed from the `required` list and the
   `omitzero` serialization tag is applied.
4. Existing ClusterExtensions with `spec.serviceAccount` set are not affected — the field is
   accepted and silently ignored by the new controller.
5. No data migration is required. The required-to-optional CRD change is additive and safe.

The ClusterRoleBinding was renamed (not updated in-place) because `roleRef` is
[immutable in Kubernetes][k8s-rbac-crb].

### Downgrading from the deprecation release

OpenShift [does not support minor version downgrades][ocp-no-downgrade]. If somehow performed:
- The `spec.serviceAccount` field remains in the API with `omitzero` serialization. An older
  controller reading a ClusterExtension where the field was cleared would see the field absent
  from the JSON, which is consistent with standard `omitempty`/`omitzero` conventions.
- An older controller that requires the field would fail to reconcile ClusterExtensions where
  the field is absent. This is expected for unsupported downgrades.
- If the ClusterExtension still has `spec.serviceAccount` set and the referenced
  ServiceAccount still has sufficient permissions, the older controller resumes reconciliation
  normally using the SA.
- If the ClusterExtension still has `spec.serviceAccount` set but the referenced
  ServiceAccount no longer has sufficient permissions, reconciliation fails with permission
  errors propagated to the ClusterExtension status, requiring manual recovery.

### Older client compatibility

Clients that read the API and expect `serviceAccount` to always be present will see it absent
(zero-valued struct omitted by `omitzero`) for newly created ClusterExtensions that do not
specify the field. This is a behavioral change for read-only clients but not a breaking
serialization change — the field deserializes to the zero-valued struct, not a nil pointer.

## Version Skew Strategy

The operator-controller is a single binary deployed by `cluster-olm-operator`. The CRD and
controller are updated atomically in the same deployment. There is no version skew between
the API server CRD schema and the controller for this component.

If an older version of a CI tool or external controller creates ClusterExtensions with
`spec.serviceAccount` set, the new operator-controller accepts them (the field is optional,
not forbidden) and logs a deprecation warning. No functional impact.

## Operational Aspects of API Extensions

### Failure Modes

- **ValidatingAdmissionPolicy deleted or misconfigured.** If the VAP or its binding is deleted,
  deprecation warnings do not appear on `kubectl create`/`kubectl apply`. The controller's
  log-level deprecation message remains as a fallback. No functional impact on reconciliation.
  **Recovery:** Automatic — `cluster-olm-operator` reconciles managed resources periodically
  and recreates missing manifests without manual intervention.

- **ClusterRoleBinding deleted.** If the cluster-admin ClusterRoleBinding is deleted, the
  operator-controller loses permissions and all reconciliation fails with authorization errors.
  **Recovery:** Automatic — `cluster-olm-operator` detects the missing ClusterRoleBinding
  via informers and recreates it immediately.

### Impact on Existing SLIs

No impact on existing SLIs. The deprecation warning is advisory only and does not affect
reconciliation success rate, time-to-install, or operator availability metrics.

The removal of per-extension ContentManager caches reduces memory consumption, which may
improve resource utilization SLIs.

## Support Procedures

**Symptom:** User reports that `spec.serviceAccount` is no longer being used for
reconciliation.
**Diagnosis:** Expected behavior after the deprecation release. Check controller logs for the deprecation INFO
message. Advise the user to clear the field from their ClusterExtension manifests.

**Symptom:** User reports a kubectl warning about the deprecated serviceAccount field.
**Diagnosis:** Expected behavior. The ValidatingAdmissionPolicy is working correctly. Advise
the user to remove the `spec.serviceAccount` field from their manifest.

**Symptom:** Reconciliation fails with authorization errors after upgrade.
**Diagnosis:** The cluster-admin ClusterRoleBinding may be missing. Verify that
`operator-controller-cluster-admin-rolebinding` exists and references the correct ServiceAccount.
If missing, trigger a re-sync of operator-controller manifests through `cluster-olm-operator`.

## Infrastructure Needed

None. 
No new CI jobs, test clusters, or external services are required.

## Implementation History

- **2025-10:** [EP #1860][] opened and closed after reviewer feedback.
- **2025-11:** [EP #1897][] (synthetic permissions alternative) opened and closed.
- **2026-06:** This enhancement proposal.

<!-- Reference-style links -->
[cluster-olm-operator]: https://github.com/openshift/cluster-olm-operator
[boxcutter]: https://github.com/operator-framework/boxcutter
[openshift-docs]: https://github.com/openshift/openshift-docs/
[ocp-ce-permissions]: https://docs.redhat.com/en/documentation/openshift_container_platform/4.22/html-single/extensions/index#olmv1-cluster-extension-permissions_managing-ce
[ocp-ce-required-rbac]: https://docs.redhat.com/en/documentation/openshift_container_platform/4.22/html-single/extensions/index#olmv1-required-rbac-to-install-and-manage-extension-resources_managing-ce
[ocp-no-downgrade]: https://access.redhat.com/solutions/4777861
[ocp-api-tiers]: https://docs.redhat.com/en/documentation/openshift_container_platform/4.19/html/api_overview/understanding-api-support-tiers
[ocp-api-coding-style]: https://github.com/openshift/api/blob/master/guidelines/api_coding_style.md
[ocp-api-field-conventions]: https://github.com/openshift/api/blob/master/guidelines/api_field_conventions.md
[ocp-clusteroperator-conditions]: https://github.com/openshift/enhancements/blob/master/dev-guide/cluster-version-operator/dev/clusteroperator.md
[api-removal-notifications]: https://github.com/openshift/enhancements/blob/master/enhancements/kube-apiserver/stability-api-removal-notifications.md
[k8s-api-warnings]: https://kubernetes.io/blog/2020/09/03/warnings/
[k8s-vap]: https://kubernetes.io/docs/reference/access-authn-authz/validating-admission-policy/
[k8s-rbac-escalation]: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#restrictions-on-role-creation-or-update
[k8s-rbac-crb]: https://kubernetes.io/docs/reference/access-authn-authz/rbac/#clusterrolebinding-example
[k8s-crd-deprecated-issue]: https://github.com/kubernetes/kubernetes/issues/131817
[EP #1860]: https://github.com/openshift/enhancements/pull/1860
[EP #1897]: https://github.com/openshift/enhancements/pull/1897
[OPRUN-4674]: https://issues.redhat.com/browse/OPRUN-4674
[OCPSTRAT-1818]: https://issues.redhat.com/browse/OCPSTRAT-1818
