---
title: vsphere-per-component-credential-overrides
authors:
  - "@rvanderp3"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-08-17
last-updated: 2026-08-17
tracking-link:
  - https://issues.redhat.com/browse/SPLAT-2889
  - https://issues.redhat.com/browse/SPLAT-2724
see-also:
  - "/enhancements/cloud-integration/cloud-credentials.md"
replaces:
  - []
superseded-by:
  - []
---

# vSphere Per-Component Credential Overrides in CCO

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement extends the Cloud Credential Operator (CCO) vSphere actuator to support per-component credential overrides stored as annotated secrets in the `openshift-config` namespace. Today, vSphere clusters use a single shared credential (`kube-system/vsphere-creds`) that is copied identically to every component (Machine API, CSI, Cloud Controller Manager). This enhancement allows cluster administrators to provide distinct, lower-privilege credentials for each component by creating override secrets annotated with the target CredentialsRequest's `spec.secretRef`.

This is a reduced-scope first phase of the broader vSphere multi-account credential management initiative (SPLAT-2724). It delivers the CCO-side credential resolution mechanism without requiring installer changes, enabling day-2 credential separation for existing and new clusters.

## Motivation

vSphere clusters today operate with a single set of credentials shared across all OpenShift components that interact with the vCenter API. This means Machine API, the CSI driver, and Cloud Controller Manager all authenticate with the same username and password. This shared-credential model creates several operational and security challenges:

- **Blast radius**: A compromised credential exposes all vSphere operations, not just the affected component.
- **Audit opacity**: vCenter audit logs cannot distinguish which OpenShift component performed an action when all components use the same identity.
- **Rotation risk**: Rotating the shared credential requires coordinated updates across all components simultaneously, increasing the risk of partial failures.

### User Stories

**Story 1 - Least-privilege credential separation**: As a cluster administrator, I want to assign distinct vSphere credentials to each OpenShift component (Machine API, CSI driver, Cloud Controller Manager) so that each component operates with only the permissions it requires, reducing the blast radius of a credential compromise.

**Story 2 - Credential rotation without full cluster impact**: As a cluster administrator, I want to rotate a single component's vSphere credential without affecting other components, so that routine credential rotation is lower-risk and can be performed incrementally.

**Story 3 - Audit and compliance separation**: As a security engineer, I want to track vSphere API activity per-component using distinct service accounts, so that audit logs clearly attribute actions to the responsible OpenShift component.

### Goals

- Allow cluster administrators to provide per-component vSphere credentials via secrets in `openshift-config`, mapped to components via annotations
- Maintain full backward compatibility — clusters without override secrets continue to use the shared `kube-system/vsphere-creds` credential with no behavioral change
- Require no installer changes — override secrets can be created manually or via automation at any point in the cluster lifecycle (day 0 via manifests, or day 2)
- Trigger automatic reconciliation when override secrets are created, updated, or deleted

### Non-Goals

- Installer-native `install-config.yaml` support for per-component credentials (planned for a subsequent phase under SPLAT-2769/SPLAT-2772)
- Automatic credential provisioning or rotation — this phase relies on administrator-managed secrets
- Extending this mechanism to non-vSphere platforms (though the annotation-based pattern is designed to be portable)
- Changes to the vSphere cloud provider configuration (`cloud-provider-config` ConfigMap)

## Proposal

### Workflow Description

The administrator creates a Secret in the `openshift-config` namespace with two annotations identifying the target component's credential:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: mapi-vsphere-creds          # any name chosen by the admin
  namespace: openshift-config
  annotations:
    cloudcredential.openshift.io/target-secret-namespace: openshift-machine-api
    cloudcredential.openshift.io/target-secret-name: vsphere-cloud-credentials
data:
  <vcenter-hostname>.username: <base64-encoded-username>
  <vcenter-hostname>.password: <base64-encoded-password>
```

The CCO controller detects the new secret in `openshift-config` (via an extended watch) and requeues all CredentialsRequests. When processing the Machine API CredentialsRequest, the vSphere actuator's `GetCredentialsRootSecret()`:

1. Lists secrets in `openshift-config`
2. Finds the secret whose `target-secret-namespace` and `target-secret-name` annotations match the CR's `spec.secretRef.Namespace` / `spec.secretRef.Name`
3. Returns the override secret's data instead of `kube-system/vsphere-creds`

The actuator syncs the override credential data to the component's target secret. Components without an override secret continue to receive the shared root credential.

**Fallback behavior**: If the override secret is deleted, the actuator falls back to `kube-system/vsphere-creds` on the next reconciliation cycle.

### API Extensions

No CRD or API changes are required. The feature uses two new well-known annotations on Kubernetes Secrets:

| Annotation Key | Value | Description |
|---|---|---|
| `cloudcredential.openshift.io/target-secret-namespace` | e.g. `openshift-machine-api` | The namespace of the CredentialsRequest's spec.secretRef this override targets |
| `cloudcredential.openshift.io/target-secret-name` | e.g. `vsphere-cloud-credentials` | The name of the CredentialsRequest's spec.secretRef this override targets |

### Topology Considerations

#### Standalone Clusters

Fully supported. The override mechanism operates entirely within the CCO reconciliation loop, which runs on standalone clusters without modification.

#### Hypershift / Hosted Clusters

Not in scope for this phase. Hypershift uses a different credential management model where the management cluster holds credentials for hosted clusters. The annotation-based pattern could be adapted for Hypershift in a future phase, but the credential topology is sufficiently different to warrant separate design work.

#### Single-node Deployments (SNO)

Supported — the same mechanism applies. The CCO runs on SNO clusters identically to multi-node clusters, and the override secret resolution is purely a control-plane operation with no node-count dependency.

### Implementation Details

**Changed Components:**

- `pkg/operator/constants/constants.go` — Add `VSphereCredOverrideNamespace` (`"openshift-config"`) and annotation key constants (`VSphereCredOverrideTargetNamespaceAnnotation`, `VSphereCredOverrideTargetNameAnnotation`)
- `pkg/vsphere/actuator/actuator.go` — Modify `GetCredentialsRootSecret()` to list secrets in `openshift-config`, match by annotation values against `cr.Spec.SecretRef`, and fall back to `kube-system/vsphere-creds`
- `pkg/operator/credentialsrequest/credentialsrequest_controller.go` — Add watch on `openshift-config` namespace; update `IsVSphereOverrideSecret()` predicate to check annotation keys
- `pkg/vsphere/actuator/actuator_test.go` — New test file with table-driven tests covering override matching, fallback, wrong annotations, multi-component scenarios, and drift detection

**Resolution Algorithm (pseudocode):**

```
func GetCredentialsRootSecret(ctx, cr):
    1. List all Secrets in "openshift-config" namespace
    2. For each secret:
       - Read annotation "cloudcredential.openshift.io/target-secret-namespace"
       - Read annotation "cloudcredential.openshift.io/target-secret-name"
       - If both match cr.Spec.SecretRef.Namespace and cr.Spec.SecretRef.Name:
           -> return this override secret
    3. If no match found:
       -> return kube-system/vsphere-creds (existing behavior)
```

**Watch Configuration:**

The CredentialsRequest controller adds a new source to its watch list:

```go
source.Kind(&corev1.Secret{},
    handler.EnqueueRequestsFromMapFunc(
        credentialsRequestMapFunc(mgr.GetClient(), constants.VSphereCredOverrideNamespace),
    ),
    predicate.NewPredicateFuncs(IsVSphereOverrideSecret),
)
```

The `IsVSphereOverrideSecret` predicate returns `true` only for secrets in `openshift-config` that carry at least one of the two `cloudcredential.openshift.io/target-secret-*` annotations, minimizing unnecessary reconciliation triggers.

### Risks and Mitigations

**Risk: Administrator provides override secret with incorrect data format.**
Mitigation: Same failure mode as providing bad credentials in `kube-system/vsphere-creds` today — component health checks surface authentication failures. The CCO logs a warning when an override secret is resolved, aiding debugging.

**Risk: Multiple override secrets target the same component.**
Mitigation: First match is used; documentation advises a one-to-one mapping between override secrets and components. A future enhancement could add a validating webhook to enforce uniqueness.

**Risk: Listing all secrets in openshift-config on every reconciliation.**
Mitigation: The `openshift-config` namespace typically contains fewer than 20 secrets; the list operation is inexpensive. A label selector can narrow the scan if the secret count grows, but this is not expected to be necessary.

**Risk: Race condition between override secret creation and CredentialsRequest reconciliation.**
Mitigation: The watch on `openshift-config` triggers requeue of all CredentialsRequests when an override secret is created or modified, ensuring convergence within one reconciliation cycle.

### Drawbacks

- Adds a new credential resolution path alongside the existing root secret, increasing the debugging surface area for credential-related issues
- Annotation-based mapping is not validated at admission time — misconfigurations are only detected at reconciliation time and surfaced through CCO logs
- The feature introduces a dependency on the `openshift-config` namespace for credential storage, which is a shared namespace used by other platform components

## Alternatives

### Naming Convention Approach

Override secrets could use a deterministic naming convention (e.g. `vsphere-creds-machine-api`) derived from the CredentialsRequest's target namespace. This was the initial implementation approach but was replaced because annotations are self-documenting, decouple secret naming from mapping, and reuse `spec.secretRef` as a stable identifier that is less likely to change across releases.

### Installer-Native Configuration

Per-component credentials configured directly in `install-config.yaml` — planned for a subsequent phase (SPLAT-2769/SPLAT-2772) and would layer on top of this CCO-side mechanism. Deferring installer integration reduces the scope of this phase and allows the CCO mechanism to be validated independently.

### Single Multi-Key Secret

A single secret with multiple keys (one set per component) was rejected because separate secrets provide better RBAC granularity, independent lifecycle management, and clearer audit trails. With separate secrets, an administrator can grant access to rotate one component's credentials without exposing others.

### CredentialsRequest Field Extension

Adding a field to `CredentialsRequest.spec` to reference an override secret was considered but rejected because CredentialsRequests are owned by the components themselves, and modifying them would require changes to each component's codebase. The annotation-based approach keeps the override mechanism entirely within the CCO and `openshift-config` namespace.

## Open Questions

1. Should the CCO surface per-component override status via conditions on the CredentialsRequest CR? This would make it visible via `oc get credentialsrequest` whether a component is using an override or the shared root credential.
2. Should a validating webhook prevent multiple override secrets targeting the same component? This would catch misconfiguration at creation time rather than reconciliation time.
3. How should this interact with `credentialsMode: Manual`? In Manual mode, the CCO does not manage credentials — should override secrets still be honored, or should they be ignored?

## Test Plan

### Unit Tests

- Override secret with matching annotations returns override credential data instead of root credential
- No override secret present returns root `kube-system/vsphere-creds` (backward compatibility)
- Override secret with wrong or missing annotations is ignored, falls back to root credential
- Multiple override secrets targeting different components result in each component getting its correct override
- `needsUpdate()` correctly detects drift between the target secret and the resolved source secret (override or root)
- Override secret deleted triggers fallback to root credential on next reconciliation
- `IsVSphereOverrideSecret` predicate correctly identifies secrets with override annotations and rejects secrets without them

### Integration / E2E Tests

- Deploy a vSphere cluster with default shared credentials
- Create an override secret in `openshift-config` for one component (e.g. Machine API)
- Verify the component's target secret is updated with the override credential data
- Verify other components still use the root credential
- Update the override secret with new credential data and verify the target secret is re-synced
- Delete the override secret and verify the component falls back to the root credential
- Create override secrets for all three components and verify each receives its distinct credential

## Graduation Criteria

### Dev Preview -> Tech Preview

- Unit tests covering all override resolution paths (match, no-match, fallback, multi-component)
- Manual testing on a vSphere cluster with per-component overrides
- Documentation of the annotation-based override mechanism in the CCO repository

### Tech Preview -> GA

- E2E test automation for override create/update/delete lifecycle
- Integration with the installer (SPLAT-2769/SPLAT-2772) for day-0 provisioning of per-component credentials
- Status conditions on CredentialsRequest indicating override usage
- Documentation in the official OpenShift credential management guide
- At least one release cycle of Tech Preview soak time with no regressions

### Removing a deprecated feature

N/A — this is a new feature with no deprecation implications.

## Upgrade / Downgrade Strategy

**Upgrade**: Existing clusters upgrading to a version with this feature will see no behavioral change. The new code path only activates when an override secret with the appropriate annotations exists in `openshift-config`. Clusters without override secrets continue to operate exactly as before.

**Downgrade**: If a cluster with override secrets is downgraded to a version without this feature, the override secrets will be ignored. The CCO will revert to using `kube-system/vsphere-creds` for all components. Administrators should ensure the root credential has sufficient permissions for all components before downgrading. The override secrets in `openshift-config` will remain but be inert — they can be cleaned up manually.

## Version Skew Strategy

The feature is entirely within the CCO — no cross-component version skew concerns exist. Consuming components (Machine API, CSI driver, Cloud Controller Manager) are unaware of the credential source; they read their target secret as before. The CCO is the sole component that implements the override resolution logic, so there is no risk of version skew between producers and consumers of this feature.

## Operational Aspects of API Extensions

**Failure Modes:**
- If an override secret contains invalid credentials, the affected component will fail to authenticate with vCenter. This is the same failure mode as providing bad credentials in `kube-system/vsphere-creds` today. The component's health checks and ClusterOperator status will reflect the authentication failure.
- If an override secret's annotations reference a non-existent CredentialsRequest target, the secret is simply never matched and has no effect.

**Monitoring:**
- No new metrics are introduced in this phase. The CCO's existing reconciliation metrics continue to apply. Future phases may add a metric indicating the number of active override secrets.

**Recovery:**
- Deleting a problematic override secret triggers re-reconciliation, and the CCO automatically falls back to the root credential. Recovery is self-healing within one reconciliation cycle (typically under 60 seconds).

## Support Procedures

1. **List active overrides**: `oc get secrets -n openshift-config -o json | jq '.items[] | select(.metadata.annotations["cloudcredential.openshift.io/target-secret-namespace"] != null) | {name: .metadata.name, targetNs: .metadata.annotations["cloudcredential.openshift.io/target-secret-namespace"], targetName: .metadata.annotations["cloudcredential.openshift.io/target-secret-name"]}'`
2. **Verify annotation values** match the target CredentialsRequest's `spec.secretRef` by running `oc get credentialsrequest -A -o json | jq '.items[] | {name: .metadata.name, ns: .spec.secretRef.namespace, secret: .spec.secretRef.name}'`
3. **Check CCO logs** for override resolution messages: `oc logs -n openshift-cloud-credential-operator deployment/cloud-credential-operator | grep -i override`
4. **Verify override secret data format** matches the expected vCenter credential structure (keys are `<vcenter-hostname>.username` and `<vcenter-hostname>.password`)
5. **Force re-reconciliation** by deleting the component's target secret — the CCO will re-create it from the resolved source (override or root)

## Implementation History

- 2026-08-17: Initial enhancement proposal for reduced-scope per-component credential overrides
- 2026-08-17: Implementation PR — [openshift/cloud-credential-operator#1075](https://github.com/openshift/cloud-credential-operator/pull/1075)
- Tracking: [SPLAT-2889](https://issues.redhat.com/browse/SPLAT-2889) (under epic [SPLAT-2724](https://issues.redhat.com/browse/SPLAT-2724))
