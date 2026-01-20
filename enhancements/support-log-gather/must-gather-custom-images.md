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

# Custom must-gather Images for Support Log Gather operator

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in `openshift-docs`

## Summary

This proposal outlines a plan to enhance the must-gather-operator to support custom must-gather images by leveraging native OpenShift `ImageStream` resources. This is achieved by requiring administrators to manually create `ImageStream`s in the must-gather-operator's own namespace, which then serves as a centrally managed allowlist of approved custom images.

To enable this, the `MustGather` CRD will be extended with new fields to specify a custom image and its configuration. The administrator remains fully responsible for creating the `ImageStream`s and all necessary RBAC (`ServiceAccounts` and `Roles`). The operator's role is limited to validating the requested image from the allowlist and using a user-provided or default `ServiceAccount` to run the must-gather job.

## Motivation

The default must-gather image provides a broad set of diagnostic data. However, users often require specialized data collection not included in the default image for specific applications or third-party components. This proposal aims to provide a native OpenShift mechanism for using custom images for this purpose.

### User Stories

- As a cluster administrator, I want to define a list of approved custom must-gather images by creating `ImageStream`s in the operator's namespace.
- As a cluster administrator, I want to create and manage a set of long-lived `ServiceAccounts` with specific `Roles` or `ClusterRoles` for different diagnostic purposes.
- As a support engineer, I want to run a must-gather job using a pre-approved custom image by specifying an `ImageStreamTag` and the appropriate, pre-configured `serviceAccountName` for my task.
- As a user running a specialized diagnostic image, I want to override its default entrypoint and/or arguments to control its behavior.
- As an administrator, I want to leverage the `ImageStreamTag` import status to know if an allowlisted image has become unpullable.

### Goals

-   Utilize the OpenShift `ImageStream` resource as a centrally managed allowlist for custom must-gather images.
-   Add a new `imageStreamRef` field to the `MustGather` CRD to allow specifying custom images.
-   Add a `gatherSpec` field to the `MustGather` CRD to allow overriding the image's command and arguments.
-   Update the must-gather-operator to validate any specified `imageStreamRef` against the allowlisted `ImageStream`s.
-   Leverage the built-in import status of an `ImageStreamTag` to asynchronously verify that an image is valid and pullable.
-   The operator will use the user-provided `serviceAccountName` directly to run the must-gather job.
-   If the `ImageStreamTag` is invalid or the import has failed, the `MustGather` resource will report a failure.
-   Ensure that if no custom image is specified, the operator continues to use the default must-gather image.

### Non-Goals

- This proposal does not cover the process of building, hosting, or distributing custom must-gather images.
- The operator will not create, manage, or validate any RBAC resources (`Roles`, `ClusterRoles`, `ServiceAccounts`, or bindings). This is the administrator's responsibility.
- This proposal does not include any mechanism for the operator to automatically discover custom must-gather images from other installed operators (e.g., by scanning `ClusterServiceVersion` annotations).

## Proposal

### Workflow Description

The workflow is divided into two main parts: the administrative setup and the user request flow.

**Part 1: Administrative Configuration**

1.  **Create Roles and ServiceAccounts**: The administrator creates the necessary `Roles`, `ServiceAccounts`, and `RoleBindings` for various diagnostic tasks.
2.  **Define Allowlist via `ImageStream`**: The administrator manually creates `ImageStream` resources in the operator's namespace. Each `ImageStreamTag` points to an allowed custom image URL.

**Part 2: User Request and Operator Execution**

1.  **User Request:** A user creates a `MustGather` CR, setting the `spec.imageStreamRef` field and optionally providing a `serviceAccountName` and a `gatherSpec` override.
2.  **Operator Validation:** The operator validates that the requested `ImageStreamRef` exists and its import status is successful.
3.  **Execution:** The operator inspects the `MustGather` CR and creates the Kubernetes `Job` according to the following logic:
    - The operator translates `spec.gatherSpec.audit` or `.metrics` fields into standard signals (e.g., environment variables `MUST_GATHER_AUDIT=true`).
    - If `spec.imageStreamRef` is **not** set (default image run):
        - The operator uses the default must-gather image, which is designed to understand these signals.
        - If `spec.gatherSpec.command` or `.args` are set, the operator rejects the CR with a validation error.
    - If `spec.imageStreamRef` **is** set (custom image run):
        - The operator validates the image reference and uses the resolved custom image.
        - The standard signals for `audit` and `metrics` are still injected, allowing the custom image to optionally respect them.
        - It passes the `spec.gatherSpec.command` and `.args` fields directly to the Job's container spec.
    - The job always runs with the permissions of the `spec.serviceAccountName` if provided, or the namespace default otherwise.
4.  **Cleanup:** Once the job completes, the operator deletes the job. The `ServiceAccount` and its associated RBAC resources remain.

### API Extensions

#### `MustGather` CRD Modification

The `MustGather` spec will be modified to include the new `imageStreamRef` and `gatherSpec` fields.

```go
// GatherSpec allows specifying the execution details for a must-gather run.
type GatherSpec struct {
	// +kubebuilder:validation:Optional
	// Audit specifies whether to collect audit logs. This is translated to a signal
	// (e.g., an environment variable) that can be respected by the default image
	// or any custom image designed to do so.
	Audit bool `json:"audit,omitempty"`

	// +kubebuilder:validation:Optional
	// Metrics specifies whether to collect Prometheus metrics. This is translated to a signal
	// (e.g., an environment variable) that can be respected by the default image
	// or any custom image designed to do so.
	Metrics bool `json:"metrics,omitempty"`

	// --- Fields for a CUSTOM must-gather image ONLY ---
	// These fields are only honored when a custom image IS specified via imageStreamRef.
	// If they are set for a default must-gather run, the request will be rejected.

	// +kubebuilder:validation:Optional
	// Command is a string array representing the entrypoint for the custom image.
	// Each string in the slice is limited to a maximum length of 256 characters by API validation.
	// +kubebuilder:validation:MaxItems=256
	// +kubebuilder:validation:Items:MaxLength=256
	Command []string `json:"command,omitempty"`

	// +kubebuilder:validation:Optional
	// Args is a string array of arguments passed to the custom image's command.
	// Each string in the slice is limited to a maximum length of 256 characters by API validation.
	// +kubebuilder:validation:MaxItems=256
	// +kubebuilder:validation:Items:MaxLength=256
	Args []string `json:"args,omitempty"`
}

// ImageStreamTagRef provides a structured reference to a specific tag within an ImageStream.
type ImageStreamTagRef struct {
	// +kubebuilder:validation:Required
	// Name is the name of the ImageStream resource in the operator's namespace.
	Name string `json:"name"`

	// +kubebuilder:validation:Required
	// Tag is the name of the tag within the ImageStream.
	Tag string `json:"tag"`
}

type MustGatherSpec struct {
	// ... existing fields ...
	// +kubebuilder:validation:Optional
	// ImageStreamRef specifies a custom image from the allowlist to be used for the
	// must-gather run.
	ImageStreamRef *ImageStreamTagRef `json:"imageStreamRef,omitempty"`

	// +kubebuilder:validation:Optional
	// GatherSpec allows overriding the command and/or arguments for the custom must-gather image.
	// This field is ignored if ImageStreamRef is not specified.
	GatherSpec *GatherSpec `json:"gatherSpec,omitempty"`

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

-   All `ImageStream`s for the allowlist must be created manually in the operator's namespace.
-   The `spec.gatherSpec.command` and `spec.gatherSpec.args` fields will only be honored when `spec.imageStreamRef` is set. If they are set for a default must-gather run, the request will be rejected by the operator.
-   The `spec.gatherSpec.audit` and `spec.gatherSpec.metrics` fields are translated into signals (e.g., environment variables) for both default and custom images.
-   Each string within the `command` and `args` slices is limited to a maximum length of 256 characters by API validation.
-   The administrator is responsible for communicating to users which `serviceAccountName` to use for a given diagnostic task.

### Risks and Mitigations

-   **Risk:** A user specifies an incorrect `ServiceAccount`, either gaining insufficient permissions (job fails) or excessive permissions (security risk).
    -   **Mitigation:** This design relies on administrative controls and clear documentation. The user running `must-gather` is expected to have the permissions to *use* the specified `ServiceAccount`.
-   **Risk:** Long-lived, privileged `ServiceAccount`s increase the cluster's attack surface.
    -   **Mitigation:** Administrators must follow security best practices, granting only the minimal required permissions to each `ServiceAccount`.
-   **Risk:** An administrator misconfigures an image URL in a manual `ImageStream`.
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
-   **Downgrade:** On downgrade, the older operator will not understand the `imageStreamRef` field, and any `MustGather` resource that specifies it will fail validation.

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

The administrator must create all required resources manually: the `Role`, the `ServiceAccount`, the `RoleBinding`, and the `ImageStream` allowlist entry.

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

#### Example 2: User Request (Custom Image)

The user specifies a custom image, provides a custom command override, and also enables `audit` collection. The custom image must be designed to respect the `MUST_GATHER_AUDIT=true` environment variable for this to have an effect.

```yaml
# /deploy/examples/must-gather-with-custom-command.yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: my-network-diagnostics-run
  namespace: team-a-namespace
spec:
  # Reference to the allowed image
  imageStreamRef:
    name: "network-debug-tools"
    tag: "v1.2"
  # Reference to the pre-configured ServiceAccount
  serviceAccountName: "must-gather-network-sa"
  # Override the command and enable audit logging
  gatherSpec:
    audit: true
    command:
    - "/usr/bin/custom-gather"
    args:
    - "--verbose"
    - "--subsystem=network"
  storage:
    type: PersistentVolume
    persistentVolume:
      claim:
        name: my-diagnostics-pvc
```

#### Example 3: User Request (Default Image with Flags)

The user runs a standard must-gather but enables the collection of audit logs via the `gatherSpec`.

```yaml
# /deploy/examples/must-gather-with-flags.yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: default-gather-with-audit
  namespace: openshift-must-gather-operator
spec:
  # No imageStreamRef is specified, so the default image will be used.
  # The `command` and `args` fields would be rejected if set here.
  gatherSpec:
    audit: true
  storage:
    type: PersistentVolume
    persistentVolume:
      claim:
        name: my-diagnostics-pvc
```
