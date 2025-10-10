---
title: serviceaccount-field-deprecation
authors:
  - "@rashmigottipati"
reviewers:
  - "@trgeiger"
  - "@grokspawn"
approvers:
  - "@trgeiger"
  - "@grokspawn"
creation-date: 2025-10-08
last-updated: 2025-10-10
status: implementable
---

# Service Account Field Deprecation

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Deprecate the `.spec.serviceAccount` field from the ClusterExtension API in the operator-controller. This field was originally introduced to enforce least privilege by requiring users to provide a ServiceAccount with the necessary RBAC permissions to manage extension content. This proposal removes that requirement and simplifies controller logic and behavior.

**Note**: This deprecation is an interim step. While the intent is to eventually remove the `.spec.serviceAccount` field, we will keep it as optional and mark it as deprecated during this transition period spanning multiple releases. This approach allows users time to adapt without disruption before the field is fully removed.

## Motivation

The original intent of `.spec.serviceAccount` was to support a least-privilege model by allowing users to provide custom `ServiceAccount` with fine-grained permissions. In practice, this design introduced considerable operational and technical complexity, including:

- Token acquisition, rotation, and error handling  
- Custom `rest.Config` generation and dynamic HTTP clients  
- Token expiration edge cases 

Given the limited benefit and high complexity of this approach, we propose simplifying the model:

- The operator-controller will **no longer impersonate the provided ServiceAccount**.
- Instead, it will use **its own identity** (via its default `ServiceAccount` assigned to the controller) for all API calls and reconciliation.
- `.spec.serviceAccount` will remain **optional but ignored**, and marked as deprecated with the intent to remove the field entirely in the future.


### Goals

- Make `.spec.serviceAccount` field **optional** in the `ClusterExtension` API.
- Update the controller to **ignore** the field during reconciliation.
- Log a warning when `.spec.serviceAccount` is set.
- Provide a deprecation and removal plan.

### Non-Goals

- Replacing `serviceAccount` with another RBAC mechanism  
- Managing permissions or pre-flight RBAC validation 

## Proposal

### API Changes

Update the `ClusterExtension` API to mark `.spec.serviceAccount` as:

- **Optional**
- **Deprecated** via struct tags and documentation
- Marked as deprecated in the CRD schema using OpenAPI extensions:
  - x-kubernetes-deprecated: true
  - Include appropriate description to warn users the field is ignored

Also, update CRD validation schema accordingly. 
- This will be done via OpenAPI `x-kubernetes-deprecated: true` annotation in the CRD.

**Example:**

```go
// Before (current)
type ClusterExtensionSpec struct {
    ...
    ServiceAccount ClusterExtensionServiceAccount `json:"serviceAccount"`
    ...
}
```

```go
// After (deprecated and optional)
type ClusterExtensionSpec struct {
    ...
    // Deprecated: This field is ignored and will be removed in a future release.
    // +optional
    // +kubebuilder:validation:XValidation:message="serviceAccount is deprecated and ignored"
    ServiceAccount *ClusterExtensionServiceAccount `json:"serviceAccount,omitempty"`
    ...
}
```

**YAML Example:**

```yaml
# Before (serviceAccount was required)
apiVersion: olm.operatorframework.io/v1alpha1
kind: ClusterExtension
metadata:
  name: argocd-extension
spec:
  installNamespace: argocd
  packageName: argocd-operator
  version: 0.6.0
  serviceAccount:
    name: argocd-installer
```

```yaml
# After (field is deprecated, optional, and therefore ignored and removed)
apiVersion: olm.operatorframework.io/v1alpha1
kind: ClusterExtension
metadata:
  name: argocd-extension
spec:
  installNamespace: argocd
  packageName: argocd-operator
  version: 0.6.0
  # serviceAccount is now deprecated and has no effect
```

### Controller Logic Changes

- Remove all logic that: 
  - Token Acquisition: Eliminate use of TokenRequest API to fetch short-lived tokens for user-provided ServiceAccounts.
  - Rest Config Mapping: Remove the ServiceAccountRestConfigMapper, which dynamically generated rest.Config objects for impersonation.
  - Synthetic Permissions: Remove conditional logic for SyntheticPermissions that depended on impersonated clients.
- The controller now uses a static `rest.Config` created from its own identity (its default ServiceAccount). 
- The RestConfigMapper will now be directly set to `ClusterAdminRestConfigMapper(mgr.GetConfig())`, i.e. the controller always uses its own identity (cluster-admin level config) for all reconciliation and watching operations.
This config is passed to the helm.ActionConfigGetter, ensuring that all Helm operations (install, upgrade, uninstall) use the same identity.

- Log a deprecation warning if `.spec.serviceAccount` is set

  - `[DEPRECATION] 'spec.serviceAccount' is specified in ClusterExtension 'foo', but is ignored and will be removed in a future release.
`

### Risks and Mitigations

Deprecating and ignoring the `.spec.serviceAccount` field introduces potential risks, particularly for users who have built workflows or assumptions around impersonation-based reconciliation. Below are the primary risks, along with mitigations.

#### Risk: Unexpected Behavior for Users Relying on SA

Some users currently set `.spec.serviceAccount` assuming the controller will impersonate that ServiceAccount during reconciliation. Changing this behavior without notice could break their expectations around RBAC scopes and permissions, for example: restricting access to specific namespaces or resources.

#### Mitigation:
The controller will log a clear deprecation warning whenever a ClusterExtension includes the serviceAccount field, indicating it is now ignored and will be removed in a future release. Additionally, the field will be marked as deprecated in the CRD schema and documentation to make this clear during resource creation and review. Migration instructions will be provided in the release notes to assist users with updating their configurations.

---

#### Risk: Broader Permissions Required for Controller’s ServiceAccount
By removing per-resource impersonation and falling back to the controller’s default identity, the controller must operate with a broader set of permissions. This potentially violates the principle of least privilege, since the controller’s ServiceAccount may now need to access resources across multiple namespaces or API groups on behalf of all managed ClusterExtensions.

#### Mitigation:  
Although this centralizes privileges, the controller’s ServiceAccount is cluster-scoped and managed by cluster administrators. Its permissions can be restricted and audited through standard Kubernetes RBAC policies, providing clearer and simpler management compared to multiple impersonated identities. This approach aligns with common practices used by other Kubernetes controllers.

---

#### Risk: Potential Breaking Change for Existing Users

Some users may have been relying on the controller to impersonate the specified serviceAccount during reconciliation. Eventually removing support for this behavior may lead to unexpected changes in how permissions are applied, especially if users were using the field to restrict access.

#### Mitigation:
To ensure a smooth transition, this change will follow a deprecation process. The field will remain in the API but will be ignored by the controller. A clear warning will be logged when the field is used. After multiple releases, the field will be removed entirely from the API and CRD. This timeline gives users enough time to adapt their configurations and permissions.

## Design Details

### Graduation Criteria / Deprecation Plan

We will deprecate the .spec.serviceAccount field over the course of three OpenShift releases following the kubernetes' deprecation policy:

OpenShift 4.20 (current release): 
- No changes. The .spec.serviceAccount field continues to be a required field and function as it currently does.

OpenShift 4.21: (next release - N)
- Mark the field as optional in the API.
- The controller ignores the field entirely.
- Log a warning if it is set, to alert users.

OpenShift 4.22: (N+1) 
- The field remains deprecated and ignored. The controller continues to log a warning if it is used.

OpenShift 4.23: (N+2) 
- Remove all internal references and usage of the field.
- Remove the field from the API and CRD definition. 
**Note**: Any usage of the field in manifests will cause validation errors. 

This phased approach, over the course of three releases, provides notice and a clear migration path for users to remove usage of the deprecated field safely. It gives users time to adjust and avoid disruption.

### Upgrade / Downgrade Strategy

**Upgrading to OpenShift 4.21 (deprecation  release):**
- The `.spec.serviceAccount` field becomes optional and is ignored by the controller.  
- A warning is logged if the field is set.  
- The controller will stop using the `.spec.serviceAccount` field, but all other functionality continues to work as before.  
- If you relied on impersonation, make sure the controller’s ServiceAccount has the necessary permissions.

**Upgrading to OpenShift 4.23 (removal release):**
- The `.spec.serviceAccount` field will no longer be present in the API or CRD.
- Manifests including this field will be rejected during validation.  
- Ensure your manifests no longer include this field before upgrading.

**Downgrading from 4.23 to 4.22 or earlier:**
- Downgrades may fail or cause issues if manifests rely on a CRD that no longer includes the field.  
- You must remove all usage of `.spec.serviceAccount` from existing resources before downgrading to a release that expects the field to exist in the schema.

**Downgrading from 4.22 to 4.21 or 4.20:**
- The field still exists in the API and CRD, so downgrades are safe.  
- However, any functionality tied to impersonation will not resume unless controller logic is reverted or adjusted accordingly.

## Implementation History
- https://github.com/operator-framework/operator-controller/pull/2242

## Drawbacks
- Users lose fine-grained permission control per extension via separate ServiceAccounts.
- The controller’s ServiceAccount needs broader permissions, potentially increasing risk if compromised.
- Cluster administrators must carefully manage the controller’s permissions.
- Possible security concerns when consolidating privileges into a single SA as opposed to separate, per-extension identities.

## Alternatives (Not Implemented)

1. Keep `.spec.serviceAccount` and Improve Token Handling  
   Instead of removing the field, we could improve token management (like automatic refresh and better error handling).  
   - However, this would add more complexity and maintenance work without fixing the main issue: impersonation is complicated and rarely helpful.

2. Create a New RBAC System for Scoped Permissions  
   Build a new way to control permissions per extension without using impersonation or ServiceAccounts (for example, by referencing ClusterRoles directly).  
   - This would make the API more complex, go against common Kubernetes practices, and could confuse users.

3. Do Nothing – Keep Supporting the Field as Is  
   - The added complexity and risks from impersonation outweigh the benefits, so leaving it unchanged is not a good option.
