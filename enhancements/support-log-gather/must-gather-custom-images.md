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

This proposal outlines a plan to enhance the must-gather-operator with a secure and reliable framework for using custom must-gather images. This is achieved by introducing a new cluster-scoped `MustGatherImage` Custom Resource (CR) that serves as a centrally managed registry of approved custom images and their required permissions.

The design introduces two core concepts: a permission model based on references to pre-existing `Roles` or `ClusterRoles` (`RoleRef`), and an asynchronous image verification system. The operator will create ephemeral, single-use `ServiceAccounts` for each job, binding them only to the pre-approved roles. It will also continuously verify that images in the allowlist are pullable and report their status. This ensures that custom diagnostics are run with the principle of least privilege and are reliable.

## Motivation

The default must-gather image provides a broad set of diagnostic data for the OpenShift platform. However, there are scenarios where users and support teams require more specialized data collection that is not included in the default image.

Currently, there is no secure or manageable way to use a custom image. This proposal aims to provide a secure and Kubernetes-native mechanism for this purpose, giving administrators clear control over which images are permitted to run with elevated privileges in their clusters.

### User Stories

- As a cluster administrator, I want to define a list of approved custom must-gather images and bind each one to a specific, narrowly-scoped `Role` or `ClusterRole` to ensure it runs with the principle of least privilege.
- As a support engineer, I want to request a must-gather run using a pre-approved custom image, knowing that its permissions are strictly managed and its source is verified.
- As an administrator, I want to see the status of all configured custom images to know ahead of time if an image is unpullable due to a typo or deletion from the registry.
- As a developer, I want to test a new version of our product's diagnostic scripts by running it as a custom must-gather image in a development cluster.

### Goals

- Introduce a new `MustGatherImage` CRD to act as a centrally managed allowlist for custom must-gather images.
- Modify the `MustGather` CRD to allow users to specify a custom image for the must-gather job.
- Update the must-gather-operator to validate any specified custom image against the `MustGatherImage` allowlist.
- If the custom image is valid, the operator will use it to run the must-gather job.
- If the custom image is invalid or the allowlist does not exist, the `MustGather` resource will report a failure.
- Ensure that if no custom image is specified, the operator continues to use the default must-gather image.
- Implement a secure, reference-based permission model (`RoleRef`) supporting both namespaced `Roles` and `ClusterRoles` to prevent privilege escalation.
- Ensure each must-gather job runs with an ephemeral, single-purpose `ServiceAccount` to guarantee strict privilege and lifecycle isolation.
- Introduce an asynchronous image verification mechanism to ensure all allowlisted images are valid and pullable.
- Provide clear status and observability into the health of the custom image allowlist via the `MustGatherImage` status subresource.

### Non-Goals

- This proposal does not cover the process of building, hosting, or distributing custom must-gather images.
- It will not provide a mechanism for automatically updating or managing the lifecycle of the custom images themselves beyond status reporting.
- It will not alter the content or scripts within the default must-gather image.

## Proposal

### Workflow Description

The workflow is divided into two main parts: the one-time administrative setup and the recurring user request flow.

**Part 1: Administrative Configuration**

1.  **Create Roles:** A cluster-admin first creates the `Roles` (for namespaced diagnostics) or `ClusterRoles` (for cluster-wide diagnostics) that contain the minimal permissions required for a specific diagnostic task.
2.  **Define Allowlist:** The admin creates a single `MustGatherImage` resource named `cluster`. In its spec, they define a list of "security contracts," where each entry pairs a custom image URL with a `roleRef` pointing to one of the pre-created roles.
3.  **Automatic Verification:** The must-gather-operator detects the `MustGatherImage` resource and begins an asynchronous loop. It continuously checks each image in the list to ensure it is pullable, updating the `status` subresource of the `MustGatherImage` CR with `Verified` or `Failed` for each image.

**Part 2: User Request and Operator Execution**

1.  **User Request:** A user creates a `MustGather` CR in a specific namespace, setting the `spec.mustGatherImage` field to an image from the admin's allowlist.
2.  **Operator Validation:** The operator validates the request by checking the `MustGatherImage.status` to ensure the requested image is marked as `Verified`.
3.  **Secure Execution:** If valid, the operator creates an ephemeral `ServiceAccount` and binds it to the corresponding `Role` or `ClusterRole`. It then creates the Kubernetes `Job` using this temporary ServiceAccount and the specified image.
4.  **Cleanup:** Once the job completes, the operator deletes the job and all associated ephemeral resources (`ServiceAccount`, `RoleBinding`/`ClusterRoleBinding`).

### API Extensions

#### `MustGatherImage` CRD

```go
package v1alpha1

import (
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=mgi
// +kubebuilder:subresource:status
type MustGatherImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec   MustGatherImageSpec   `json:"spec,omitempty"`
	Status MustGatherImageStatus `json:"status,omitempty"`
}

type MustGatherImageSpec struct {
	// +listType=map
	// +listMapKey=image
	Images []CustomMustGatherImage `json:"images"`
}

type CustomMustGatherImage struct {
	// +kubebuilder:validation:Required
	Image string `json:"image"`
	// +kubebuilder:validation:Required
	RoleRef rbacv1.RoleRef `json:"roleRef"`
}

type MustGatherImageStatus struct {
	// +optional
	// +listType=map
	// +listMapKey=image
	VerifiedImages []ImageVerificationStatus `json:"verifiedImages,omitempty"`
}

type ImageVerificationStatus struct {
	// +kubebuilder:validation:Required
	Image string `json:"image"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=Verified;Failed;Pending
	State string `json:"state"`
	// +optional
	Reason string `json:"reason,omitempty"`
}

// +kubebuilder:object:root=true
type MustGatherImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MustGatherImage `json:"items"`
}
```

#### `MustGather` CRD Modification

```go
type MustGatherSpec struct {
	// ... existing fields ...
	// +kubebuilder:validation:Optional
	MustGatherImage string `json:"mustGatherImage,omitempty"`
	// ... existing fields ...
}
```

### Topology Considerations

This enhancement does not introduce any unique topological considerations and is expected to function identically across all supported OpenShift topologies.

#### Hypershift / Hosted Control Planes

No unique considerations. The operator runs in the guest cluster and performs all its functions there.

#### Standalone Clusters


#### Single-node Deployments or MicroShift



### Implementation Details/Notes/Constraints

-   The `MustGatherImage` resource must be named `cluster` to serve as the single source of truth.
-   The operator will require cluster-level RBAC to `get/list/watch` `MustGatherImage` resources and to create `ClusterRoleBindings`.
-   The image verification loop will utilize the cluster's global pull secret to authenticate to private container registries.
-   Failures during the process (e.g., image not verified, role not found) will be reported as conditions in the `MustGather` resource's status.

### Risks and Mitigations

-   **Risk:** Operator has excessive permissions to create arbitrary roles.
    -   **Mitigation:** The `RoleRef` model prevents this. The operator only creates `RoleBindings` to roles pre-approved by an admin.
-   **Risk:** A privileged `ServiceAccount` is left behind after a job.
    -   **Mitigation:** The `ServiceAccount` is ephemeral and garbage collected by the operator upon job completion.
-   **Risk:** An administrator misconfigures an image URL or a role.
    -   **Mitigation:** Image pull errors are made visible by the async verification status. Role binding errors are reported on the `MustGather` CR.

### Drawbacks

-   The feature introduces additional complexity for cluster administrators, who are now responsible for creating and managing roles and the `MustGatherImage` allowlist.

## Test Plan

-   **Unit Tests:**
    -   Test the reconciler logic for the `MustGatherImage` resource, including the image verification loop.
    -   Test the `MustGather` reconciler logic for validating a custom image request and creating ephemeral resources (`ServiceAccount`, bindings, `Job`).
-   **E2E Tests:**
    -   Verify the full workflow for a valid custom image with a `ClusterRole`.
    -   Verify the full workflow for a valid custom image with a namespaced `Role`.
    -   Verify that a request for an unlisted image fails with an appropriate status condition.
    -   Verify that a request for a listed but unpullable (failed verification) image fails.
    -   Verify that after a job completes, the ephemeral `ServiceAccount` and bindings are deleted.

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

-   **Upgrade:** This is a backward-compatible change. Existing `MustGather` resources without the `mustGatherImage` field will continue to function as before. The new `MustGatherImage` CRD will be created upon upgrade.
-   **Downgrade:** On downgrade, the `MustGatherImage` CRD will remain, but the older operator will not be aware of it. Any `MustGather` resource that specifies a `mustGatherImage` will be ignored by the older operator and will likely fail validation if the field is not recognized.

## Version Skew Strategy

This enhancement does not introduce any version skew concerns. The logic is self-contained within the must-gather-operator and its CRDs.

## Operational Aspects of API Extensions

The primary API extension is the `MustGatherImage` CRD. Its operational status is exposed via its `status` subresource. Cluster administrators are responsible for its lifecycle. Failure modes, such as an unpullable image, are surfaced directly in this status, making the system's configuration health easily observable.

## Support Procedures

If a `must-gather` run with a custom image fails, support personnel should first inspect the `MustGather` resource's status and events to check for errors. If the error is related to image verification or permissions, they should then inspect the `MustGatherImage` resource (`oc get mustgatherimage cluster -o yaml`) to verify its configuration and status.

## Alternatives (Not Implemented)

An alternative design was considered where the operator could be given permissions to create `Roles` dynamically. This was rejected because it would require the operator itself to possess dangerously broad permissions, violating the principle of least privilege. The `RoleRef` model ensures the operator remains a low-privilege component.

### Examples

#### Example 1: Administrator Configuration (`MustGatherImage`)

This example shows a cluster administrator configuring the `cluster` allowlist with two custom images. The first image is for cluster-wide network diagnostics and is bound to a `ClusterRole`. The second is for application-specific diagnostics and is bound to a namespaced `Role`.

```yaml
# /deploy/examples/must-gather-image-allowlist.yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGatherImage
metadata:
  # The name is fixed to 'cluster' to provide a single, cluster-wide source of truth.
  name: cluster
spec:
  images:
  - image: quay.io/my-org/network-debug-tools:v1.2
    roleRef:
      apiGroup: rbac.authorization.k-8s.io
      kind: ClusterRole
      name: custom-must-gather-network-role # Admin must pre-create this ClusterRole
  - image: registry.example.com/team-a/app-diagnostics:latest
    roleRef:
      apiGroup: rbac.authorization.k8s.io
      kind: Role
      name: team-a-diagnostics-role # Admin must pre-create this Role in relevant app namespace(s)
```

#### Example 2: User Request (`MustGather`)

This example shows a user in the `team-a-namespace` namespace requesting a must-gather run using the pre-approved application-specific diagnostic image.

```yaml
# /deploy/examples/must-gather-with-custom-image.yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: my-app-diagnostics-run
  namespace: team-a-namespace # The namespace where the Role "team-a-diagnostics-role" exists
spec:
  # Requesting the custom image for app diagnostics.
  # The operator will validate this against the 'cluster' MustGatherImage resource.
  mustGatherImage: registry.example.com/team-a/app-diagnostics:latest

  # Other spec fields, like storage, remain available.
  storage:
    type: PersistentVolume
    persistentVolume:
      claim:
        name: my-diagnostics-pvc
```
