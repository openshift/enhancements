---
title: must-gather-custom-images
authors:
  - "@shivprakashmuley"
reviewers:
  - "@TrilokGeer"
  - "@Prashanth684"
approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
api-approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
creation-date: 2025-12-11
last-updated: 2025-12-11
tracking-link:
  - https://issues.redhat.com/browse/MG-155
---

# Enhancement Proposal: Custom Must-Gather Images

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in `openshift-docs`

## Summary

This proposal outlines a plan to enhance the must-gather-operator to support custom must-gather images by leveraging native OpenShift `ImageStream` resources. This is achieved by requiring administrators to create `ImageStream`s in the must-gather-operator's own namespace, which then serves as a centrally managed allowlist of approved custom images.

To enable this, the `MustGather` CRD will be extended with a new `imageStreamTag` field. The operator's role is limited to validating the requested image from the allowlist and using the user-provided `ServiceAccount` to run the must-gather job. The administrator remains fully responsible for all RBAC management.

## Motivation

The default must-gather image provides a broad set of diagnostic data. However, users often require specialized data collection not included in the default image for specific applications or third-party components. This proposal aims to provide a native OpenShift mechanism for using custom images for this purpose.

### User Stories

- As a cluster administrator, I want to define a list of approved custom must-gather images by creating `ImageStream`s in the operator's namespace.
- As a cluster administrator, I want to create and manage a set of long-lived `ServiceAccounts` with specific `Roles` or `ClusterRoles` for different diagnostic purposes.
- As a support engineer, I want to run a must-gather job using a pre-approved custom image by specifying an `ImageStreamTag` and the appropriate, pre-configured `serviceAccountName` for my task.
- As an administrator, I want to leverage the `ImageStreamTag` import status to know if an allowlisted image has become unpullable.

### Goals

- Utilize the OpenShift `ImageStream` resource as a centrally managed allowlist for custom must-gather images.
- Add a new `imageStreamTag` field to the `MustGather` CRD.
- Update the must-gather-operator to validate any specified `imageStreamTag` against the allowlisted `ImageStream`s.
- Leverage the built-in import status of an `ImageStreamTag` to asynchronously verify that an image is valid and pullable.
- The operator will use the user-provided `serviceAccountName` directly to run the must-gather job.
- If the `ImageStreamTag` is invalid or the import has failed, the `MustGather` resource will report a failure.
- Ensure that if no custom image is specified, the operator continues to use the default must-gather image.

### Non-Goals

- This proposal does not cover the process of building, hosting, or distributing custom must-gather images.
- The operator will not create, manage, or validate any RBAC resources (`Roles`, `ClusterRoles`, `ServiceAccounts`, or bindings). This is the administrator's responsibility.

## Proposal

### Workflow Description

The workflow is divided into two main parts: the administrative setup and the user request flow.

**Part 1: Administrative Configuration**

1.  **Create Roles:** A cluster-admin creates the `Roles` or `ClusterRoles` that contain the permissions required for various diagnostic tasks.
2.  **Create ServiceAccounts and Bindings:** The admin creates long-lived `ServiceAccount`s and binds each one to the appropriate role using a `RoleBinding` or `ClusterRoleBinding`.
3.  **Define Allowlist via `ImageStream`:** The admin creates `ImageStream`s in operator's namespace, where each `ImageStreamTag` points to an allowed custom image URL.
4.  **Automatic Verification:** OpenShift's native image import mechanism periodically attempts to import the tag, updating the `ImageStreamTag` status, which the operator monitors to verify image pullability.

**Part 2: User Request and Operator Execution**

1.  **User Request:** A user creates a `MustGather` CR, setting the new `spec.imageStreamTag` field to an allowed tag.
2.  **Operator Validation:** The operator validates that the requested `ImageStreamTag` exists and its import status is successful.
3.  **Execution:** If the image is valid, the operator creates the Kubernetes `Job`, specifying the user-provided `serviceAccountName` in the pod spec. The job runs with the permissions granted to that `ServiceAccount`.
4.  **Cleanup:** Once the job completes, the operator deletes the job. The `ServiceAccount` and its associated RBAC resources remain.

### API Extensions

#### `MustGather` CRD Modification

The `MustGather` spec will be modified to include the new `imageStreamTag` field.

```go
type MustGatherSpec struct {
	// ... existing fields ...
	// +kubebuilder:validation:Optional
	// ImageStreamTag is the new field to specify a custom image from the allowlist.
	ImageStreamTag string `json:"imageStreamTag,omitempty"`

	// ... existing fields ...
}
```

### Topology Considerations

This enhancement does not introduce any unique topological considerations and is expected to function identically across all supported OpenShift topologies.

#### Hypershift / Hosted Control Planes

No unique considerations.

#### Standalone Clusters


#### OpenShift Kubernetes Engine


#### Single-node Deployments or MicroShift


### Implementation Details/Notes/Constraints

-   All `ImageStream`s for the allowlist must be created in operator's namespace.
-   The administrator is responsible for communicating to users which `serviceAccountName` to use for a given diagnostic task.

### Risks and Mitigations

-   **Risk:** A user specifies an incorrect `ServiceAccount`, either gaining insufficient permissions (job fails) or excessive permissions (security risk).
    -   **Mitigation:** This design relies on administrative controls and clear documentation. The user running `must-gather` is expected to have the permissions to *use* the specified `ServiceAccount`.
-   **Risk:** Long-lived, privileged `ServiceAccount`s increase the cluster's attack surface.
    -   **Mitigation:** Administrators must follow security best practices, granting only the minimal required permissions to each `ServiceAccount`.
-   **Risk:** An administrator misconfigures an image URL.
    -   **Mitigation:** The `ImageStreamTag` import status will fail, making the error visible. The operator will reject `MustGather` requests for failed tags.

### Drawbacks

-   This approach requires the administrator to create, manage, and document RBAC resources.
-   It requires the end-user to have knowledge of the underlying permission model (`ServiceAccount` names).
-   The lack of an automated link between an image and its required permissions can lead to user error when selecting a `ServiceAccount`.

## Test Plan

-   **Unit Tests:**
    -   Test the `MustGather` reconciler logic for finding `ImageStream`s and checking their import status.
-   **E2E Tests:**
    -   Verify the full workflow: admin creates RBAC and `ImageStream`, then a user creates a `MustGather` specifying both, and the job succeeds.
    -   Verify failure mode: request for a non-existent `ImageStreamTag`.
    -   Verify failure mode: `ImageStream` exists but the image tag import has failed.

## Graduation Criteria

### Dev Preview -> Tech Preview

-   Ability to utilize the enhancement end-to-end.
-   End-user documentation and API stability.
-   Sufficient test coverage (unit and E2E).
-   Gather feedback from early adopters.

### Tech Preview -> GA

-   More extensive testing, including upgrade and scale scenarios.
-   Sufficient time for user feedback and adoption.
-   User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/).

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

-   **Upgrade:** This is a backward-compatible change. Existing `MustGather` resources will continue to function as before.
-   **Downgrade:** On downgrade, the older operator will not understand the `imageStreamTag` field, and any `MustGather` resource that specifies it will fail validation.

## Version Skew Strategy

This enhancement does not introduce any version skew concerns.

## Operational Aspects of API Extensions

Administrators will manage a set of `ImageStream` resources in the operator's namespace. The operational status of the allowlist is exposed via the native `status` subresource of each `ImageStreamTag`. Administrators are responsible for creating these resources and ensuring their `importPolicy` is correctly configured to point to a valid internal or external image source. Monitoring the status of these `ImageStream`s is the primary way to observe the health of the custom image configuration.

## Support Procedures

If a `must-gather` run fails, support personnel should inspect the `MustGather` status. If the error is related to the image, they should check the `ImageStream`. If it's a permissions error, they must check the `ServiceAccount` and its associated `RoleBindings`.

## Alternatives (Not Implemented)

A design using a dedicated `MustGatherImage` CRD with an automated, ephemeral `ServiceAccount` model was considered. This would provide a more integrated and secure link between an image and its permissions, but adds more complexity to the operator. The current approach was chosen to maximize simplicity in the operator's logic.

### Examples

#### Example 1: Administrator Configuration

The admin must create all required resources: the `Role`, the `ServiceAccount`, the `RoleBinding`, and the `ImageStream`.

```yaml
# /deploy/examples/admin-full-setup.yaml

# 1. The Role with required permissions
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: custom-must-gather-network-role
rules:
- apiGroups: [""]
  resources: ["pods", "nodes"]
  verbs: ["get", "list", "watch"]
---
# 2. The long-lived ServiceAccount
apiVersion: v1
kind: ServiceAccount
metadata:
  name: must-gather-network-sa
  namespace: openshift-must-gather-operator
---
# 3. The binding to link the Role and ServiceAccount
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: must-gather-network-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: custom-must-gather-network-role
subjects:
- kind: ServiceAccount
  name: must-gather-network-sa
  namespace: openshift-must-gather-operator
---
# 4. The ImageStream to allowlist the image
apiVersion: image.openshift.io/v1
kind: ImageStream
metadata:
  name: network-debug-tools
  namespace: must-gather-operator
spec:
  tags:
  - name: 'v1.2'
    from:
      kind: DockerImage
      name: 'quay.io/my-org/network-debug-tools:v1.2'
```

#### Example 2: User Request (`MustGather`)

The user specifies the new `imageStreamTag` and, optionally, the existing `serviceAccountName` field.

```yaml
# /deploy/examples/must-gather-with-imagestream-and-sa.yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: my-network-diagnostics-run
  namespace: team-a-namespace
spec:
  # Reference to the allowed image via the new field
  imageStreamTag: "network-debug-tools:v1.2"
  # Reference to the pre-configured ServiceAccount via the existing field
  serviceAccountName: "must-gather-network-sa"
  storage:
    type: PersistentVolume
    persistentVolume:
      claim:
        name: my-diagnostics-pvc
```
