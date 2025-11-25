---
title: olmv1-optional-serviceaccount-field
authors:
  - "@rashmigottipati"
reviewers:
  - "@grokspawn"
  - "@joelanford"
  - "@trgeiger"
approvers:
  - "@joelanford"
api-approvers:
  - "@everettraven"
creation-date: 2025-11-25
last-updated: 2025-11-25
tracking-link:
  - https://issues.redhat.com/browse/OPRUN-4144
replaces:
superseded-by:
---

# OLMv1: Make ServiceAccount Field Optional in the ClusterExtension API

## Summary

One of the core design principles of OLMv1 is Secure By Default. This enhancement aims at upholding and also improving upon that principle while addressing a usability issue.

The proposal is to make the `spec.serviceAccount` field in the ClusterExtension API optional in Tech Preview. For extensions without a ServiceAccount, OLMv1 uses a synthetic identity with zero permissions and relies on Kubernetes impersonation, allowing administrators to explicitly grant the necessary privileges. For extensions with a ServiceAccount, OLMv1 continues to use token based authentication, preserving backward compatibility.

## Motivation

Currently, the `spec.serviceAccount` field is required on every `ClusterExtension`, which creates usability and security challenges. 

From a usability standpoint, users must understand the exact permissions their extension requires, create a corresponding ServiceAccount, and configure ClusterRoleBindings appropriately. Ensuring that the service account has the correct permissions is a manual and often complex process, leading to failed installations and a frustrating experience, particularly for new users. User feedback indicates a strong preference to avoid configuring RBAC for each ClusterExtension, with some users resorting to granting cluster admin privileges just to satisfy the requirement.

From a security perspective, the complexity of correctly configuring RBAC frequently drives users toward over privileged solutions, such as granting cluster-admin access to satisfy the installation requirements. This behavior directly conflicts with OLMv1’s principle of being secure by default and introduces unnecessary risk to the cluster. 

Before arriving at this design, we evaluated simpler alternatives such as removing the field entirely and implicitly using a cluster-admin service account, or making the field optional with cluster-admin as the default. While these approaches improve ease of use, they conflict with the principle of least privilege.

By making the spec.serviceAccount field optional and introducing synthetic identities with zero permissions by default, this enhancement provides a safer and simpler installation experience. It preserves backward compatibility and still allows the use of custom ServiceAccounts when fine-grained control is needed, aligning usability with the principle of least privilege.

### User Stories

- As a cluster admin, I want extensions to run with zero privileges by default, so that the cluster remains secure unless I explicitly grant permissions
- As an extension author, I want to retain the ability to use custom ServiceAccounts for fine grained RBAC, allowing my extensions to operate with the privileges they need
- As a cluster admin, I want to grant permissions to multiple extensions using group bindings to apply the same permissions to all extensions without a ServiceAccount

### Goals

- Make `spec.serviceAccount` optional in Tech Preview
- Ensure extensions run with zero permissions by default when no ServiceAccount is provided, using Kubernetes impersonation
- Introduce synthetic identities for ClusterExtensions without a ServiceAccount
- Preserve existing behavior (token based approach) when a ServiceAccount is specified, maintaining backward compatibility
- Simplify RBAC management for new users by allowing permissions to be granted via standard ClusterRoleBindings and group bindings
- Provide clear documentation and examples for using optional ServiceAccounts, synthetic identities, and RBAC bindings

### Non-Goals

- Removing the ServiceAccount field
- Removing or replacing existing token based authentication
- Auto generating RBAC based on extension requirements
- Making any changes to stable/GA behavior; this enhancement affects only Tech Preview

## Proposal

### How It Works

**When the `spec.serviceAccount` field is unset:**
- OLM impersonates a synthetic identity: 
  - **user**: `olm:clusterextension:<ceName>` 
  - **group:** `olm:clusterextensions`
- The identity has zero permissions by default
- Administrators would explicitly grant permissions via standard Kubernetes RBAC (e.g., `ClusterRoleBinding`).
- No ServiceAccount resource needs to be created

**When the `spec.serviceAccount` is **set:**
- OLM honors it and authenticates via the existing token based mechanism
- The specified `ServiceAccount` must exist and have appropriate RBAC rules
- Tokens are fetched and managed as usual
- Preserves the existing GA behavior and is fully backward compatible with existing extensions

---

### Installation Flows

**Creating an extension without ServiceAccount:**
1. User creates ClusterExtension without specifying `spec.serviceAccount`
2. OLM creates synthetic identity: user `olm:clusterextension:<ceName>`, group `olm:clusterextensions`
3. Installation fails (zero permissions)
4. User creates ClusterRoleBinding granting permissions to the synthetic identity
5. Installation succeeds automatically

**Creating an extension with ServiceAccount:**
1. User creates ServiceAccount with appropriate RBAC
2. User creates ClusterExtension with `spec.serviceAccount` set
3. OLM uses token-based authentication with that ServiceAccount (existing behavior)
4. Installation succeeds using ServiceAccount's permissions

### API Changes

The API structure remains the same, but the field becomes optional only when the SyntheticPermissions feature gate is enabled.
 - Upstream exposes the field with omitempty.
 - Behavior gating is done via the SyntheticPermissions feature gate.
 - OpenShift applies its own TechPreviewNoUpgrade gating on the downstream CRD by adding the `+openshift:enable:FeatureSets=TechPreviewNoUpgrade` annotation to ensure the field is optional only in Tech Preview and remains required in GA.

```go
// serviceAccount specifies the identity for OLM controller operations.
// Optional in Tech Preview.
//
// When OMITTED: OLM uses synthetic identity "olm:clusterextension:<ceName>"
// with ZERO permissions. Admins grant permissions via ClusterRoleBinding.
//
// When SET: OLM uses existing token based authentication.
// The ServiceAccount must exist on the cluster with appropriate RBAC.
//
// +optional
ServiceAccount ServiceAccountReference `json:"serviceAccount,omitempty"`
```

#### CRD Generation Behavior:
- Default CRD (stable channel): Field remains required.
- Tech Preview CRD (experimental channel): Field becomes optional, with description updated to explain synthetic identity behavior.

#### RBAC Examples

Grant cluster-admin to specific extension:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: sample-rolebinding
roleRef:
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: User
  name: "olm:clusterextension:<ceName>"
```

Grant cluster-admin to all extensions in the same group:
```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: sample-rolebinding
roleRef:
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: Group
  name: "olm:clusterextensions"
```

#### Deletion Behavior

- **Extensions with a ServiceAccount:** 
  - Existing behavior applies; all resources owned by the CE are deleted, including associated RBAC
- **Extensions without a ServiceAccount (synthetic identity):** 
  - No actual ServiceAccount exists, so OLM does not delete any RBAC bindings created by the admin, so admins are responsible for removing any ClusterRoleBindings or group bindings associated with synthetic identities if desired.

### Implementation

- The synthetic identity behavior relies on the SyntheticPermissions feature gate in operator-controller.
- This gate is Tech Preview and must be enabled for extensions without a ServiceAccount

**When ServiceAccount is unset**, OLM uses Kubernetes impersonation via client-go:

```go
// Synthetic identity
impersonationConfig := rest.ImpersonationConfig{
    UserName: fmt.Sprintf("olm:clusterextension:%s", ce.Name),
    Groups:   []string{"olm:clusterextensions"},
}
```

**When ServiceAccount is set**, OLM continues using the existing token-based authentication mechanism (no code changes for this path).

Controller changes: Add synthetic identity generation for unset case, configure impersonation when ServiceAccount is empty.

### Risks and Mitigations

1. Users expect instant installation
    - Need to mitigate with clear error messages, copy-paste RBAC examples, documentation
2. Unfamiliar with synthetic identities
    - Provide docs support for generating bindings
3. Overly permissive group bindings
    - Document when to use group vs per extension bindings

### Benefits
This approach satisfies both security and usability: zero-permission default when unset, explicit privilege delegation, simplified installation for new users (one manual step of RBAC binding instead of ServiceAccount + RBAC), and preserves backward compatibility.

### Drawbacks

- Installation requires extra manual step of creating RBAC
- Learning curve for synthetic identities

Choosing this approach despite these drawbacks as there are more advantages: secure by default, better UX than ServiceAccount+RBAC, simplified implementation.

## Test Plan

- Unit tests around ClusterExtension creation for the new behavior to ensure that extensions without a ServiceAccount are correctly handled:
  - Synthetic identity is generated correctly 
  - JSON serialization: ClusterExtension without a ServiceAccount does not include `spec.serviceAccount` in the output
- E2E tests that exercise Tech Preview ClusterExtensions without a ServiceAccount:
  - Installation workflow succeeds after creating the appropriate RBAC for the synthetic identity
  - Scenarios with some extensions using ServiceAccounts and the rest using synthetic identities
  - Failure scenarios: installation fails gracefully when RBAC is missing or insufficient

## Graduation Criteria

This enhancement is introduced as Tech Preview. Promotion to GA will require:

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient e2e and unit test coverage
- Gather feedback from users rather than just developers
- No significant security concerns and bugs discovered during TP

## Upgrade / Downgrade Strategy

**Upgrading to Tech Preview**:
- Existing ClusterExtensions (with ServiceAccount set) continue using token based auth
- ServiceAccount field becomes optional for new ClusterExtensions
- New ClusterExtensions without ServiceAccount use impersonation
- No RBAC changes required for previously installed extensions
- No user action required in the upgrade scenario

**Downgrading from Tech Preview**:
- Before downgrading, all ClusterExtensions that rely on synthetic identities must have a ServiceAccount added
- RBAC bindings must also be applied to ensure extensions have the correct permissions post downgrade
- Without this step, downgrading could break extension installations, as older OLM versions require the `spec.serviceAccount` field to be set

## Alternatives

1. Remove SA field, and default to cluster-admin: 
    - Delegates full responsibility to OLM as the package manager
    - OLM would manage all ClusterExtensions using its own elevated privileges 
    - Even though this solves the delegation usecase, it breaks the principle of least privilege as every extension would run with cluster-admin by default
2. Make SA field optional; honor if provided, default to OLM cluster-admin if not provided: 
    - Not secure by default
    - Users could unintentionally grant cluster-admin privileges to extensions if they ignore setting the SA field as it's optional
3. Keep SA field required, only add impersonation
    - This doesn't solve usability problem
4. Always use impersonation (even when SA is set)
    - Breaks backward compatibility, requires more extensive migration and changes existing behavior
5. Do nothing
   - Poor UX
   - Security issues with binding to privileged SAs remain
