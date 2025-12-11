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

This proposal outlines a plan to enhance the must-gather-operator to support the use of custom must-gather images. A new cluster-scoped Custom Resource, `MustGatherImage`, will be introduced to define a list of allowed custom images that cluster administrators can manage. The existing `MustGather` Custom Resource will be updated with an optional `mustGatherImage` field, allowing users to select an image from the allowed list when triggering a must-gather execution.

## Motivation

The default must-gather image provides a broad set of diagnostic data for the OpenShift platform. However, there are scenarios where users and support teams require more specialized data collection that is not included in the default image. This can include:

-   Running diagnostic scripts for specific applications or layered products.
-   Gathering information from third-party operators or custom infrastructure components.
-   Using a pre-release or patched version of the must-gather tooling for debugging purposes without waiting for an official release.

Currently, there is no secure or manageable way to use a custom must-gather image. This proposal aims to provide a secure and Kubernetes-native mechanism for this purpose, giving administrators clear control over which images are permitted to run with elevated privileges in their clusters.

### User Stories

- As a cluster administrator, I want to define a list of approved custom must-gather images to ensure that only trusted images are used for diagnostics.
- As a support engineer, I want to use a custom must-gather image with specialized tools to debug a specific issue without needing to modify the cluster's default must-gather image.
- As a developer, I want to test a new version of our product's diagnostic scripts by running it as a custom must-gather image in a development cluster.

### Goals

- Introduce a new `MustGatherImage` CRD to act as a centrally managed allowlist for custom must-gather images.
- Modify the `MustGather` CRD to allow users to specify a custom image for the must-gather job.
- Update the must-gather-operator to validate any specified custom image against the `MustGatherImage` allowlist.
- If the custom image is valid, the operator will use it to run the must-gather job.
- If the custom image is invalid or the allowlist does not exist, the `MustGather` resource will report a failure.
- Ensure that if no custom image is specified, the operator continues to use the default must-gather image.

### Non-Goals

- This proposal does not cover the process of building, hosting, or distributing custom must-gather images.
- It will not provide a mechanism for automatically updating or managing the lifecycle of the custom images themselves.
- It will not alter the content or scripts within the default must-gather image.

## Proposal

### Workflow Description

1.  A cluster administrator defines a `MustGatherImage` resource named `cluster`, which contains a list of approved custom must-gather image URLs.
2.  A user creates a `MustGather` resource and specifies an image from the allowlist in the `spec.mustGatherImage` field.
3.  The must-gather-operator reconciles the `MustGather` resource.
4.  The operator fetches the `MustGatherImage` resource named `cluster`.
5.  It validates that the image specified in the `MustGather` resource is present in the `MustGatherImage` resource's list.
6.  If the image is valid, the operator creates a must-gather job using the custom image.
7.  If the image is not valid, or if the `MustGatherImage` resource does not exist, the operator updates the `MustGather` resource's status with an error and does not create the job.

### API Extensions

#### New `MustGatherImage` CRD

A new cluster-scoped Custom Resource Definition `MustGatherImage` will be created. This resource will serve as the allowlist for all custom must-gather images that can be used in the cluster. Only privileged users (like cluster-admins) should have permission to create and modify this resource.

**Example `MustGatherImage` manifest:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGatherImage
metadata:
  # The name is fixed to 'cluster' to provide a single, cluster-wide source of truth.
  name: cluster
spec:
  images:
  - quay.io/my-org/my-custom-must-gather:latest
  - registry.example.com/team-a/debug-must-gather:v1.2
```

The Go definition for this CRD will be created in `must-gather-operator/api/v1alpha1/mustgatherimage_types.go`:

```go
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,shortName=mgi
// +kubebuilder:subresource:status

// MustGatherImage is the Schema for the mustgatherimages API.
type MustGatherImage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MustGatherImageSpec   `json:"spec,omitempty"`
	Status MustGatherImageStatus `json:"status,omitempty"`
}

// MustGatherImageSpec defines the desired state of MustGatherImage.
type MustGatherImageSpec struct {
	// Images is a list of fully qualified container image references that are
	// allowed to be used as a custom must-gather image.
	// +kubebuilder:validation:Required
	// +listType=set
	Images []string `json:"images"`
}

// MustGatherImageStatus defines the observed state of MustGatherImage.
type MustGatherImageStatus struct{}

// +kubebuilder:object:root=true

// MustGatherImageList contains a list of MustGatherImage.
type MustGatherImageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []MustGatherImage `json:"items"`
}
```

#### `MustGather` CRD Modification

The `MustGatherSpec` in `must-gather-operator/api/v1alpha1/mustgather_types.go` will be updated to include an optional `mustGatherImage` field. This is in addition to other existing and proposed fields like `storage` for PVC destination and `uploadTarget` for extensible artifact uploading.

```go
// MustGatherSpec defines the desired state of MustGather
type MustGatherSpec struct {
	// the service account to use to run the must gather job pod, defaults to default
	// +optional
	ServiceAccountRef corev1.LocalObjectReference `json:"serviceAccountRef,omitempty"`

	// MustGatherImage allows specifying a custom must-gather image.
	// The image must be listed in the 'cluster' MustGatherImage resource to be accepted.
	// If not specified, the operator's default must-gather image will be used.
	// +kubebuilder:validation:Optional
	MustGatherImage string `json:"mustGatherImage,omitempty"`

	// additionalConfig contains extra parameters used to customize the gather process,
	// currently enabling audit logs is the only supported field.
	// +optional
	AdditionalConfig *AdditionalConfig `json:"additionalConfig,omitempty"`

	// This represents the proxy configuration to be used. If left empty it will default to the cluster-level proxy configuration.
	// +optional
	ProxyConfig ProxySpec `json:"proxyConfig,omitempty"`

	// A time limit for gather command to complete a floating point number with a suffix:
	// "s" for seconds, "m" for minutes, "h" for hours, or "d" for days.
	// Will default to no time limit.
	// +optional
	// +kubebuilder:validation:Format=duration
	MustGatherTimeout metav1.Duration `json:"mustGatherTimeout,omitempty"`

	// A flag to specify if resources (secret, job, pods) should be retained when the MustGather completes.
	// If set to true, resources will be retained. If false or not set, resources will be deleted (default behavior).
	// +optional
	// +kubebuilder:default:=false
	RetainResourcesOnCompletion bool `json:"retainResourcesOnCompletion,omitempty"`

	// uploadTarget sets the target config for uploading the collected must-gather tar.
	// Uploading is disabled if this field is unset.
	// +optional
	UploadTarget *UploadTarget `json:"uploadTarget,omitempty"`

	// storage represents the volume where collected must-gather tar
	// is persisted. Persistent storage is disabled if this field is unset,
	// an ephemeral volume will be used.
	// +optional
	Storage *StorageConfig `json:"storage,omitempty"`
}
```

### Topology Considerations

This enhancement does not introduce any unique topological considerations. The must-gather-operator and the custom must-gather images are expected to run on any supported OpenShift topology.

#### Hypershift / Hosted Control Planes


#### Standalone Clusters


#### Single-node Deployments or MicroShift


### Implementation Details/Notes/Constraints

-   The `MustGatherImage` resource will be named `cluster` to ensure a single source of truth for the allowlist.
-   The operator will require RBAC permissions to `get`, `list`, and `watch` the `mustgatherimages` resource at the cluster scope.
-   The operator's logic will be updated to fetch the `MustGatherImage` resource and validate the custom image specified in the `MustGather` CR.
-   If the `MustGatherImage` resource is not found, or if the specified image is not in the allowlist, the `MustGather` resource's status will be updated with an appropriate error condition.

### Risks and Mitigations

-   **Risk:** A misconfigured `MustGatherImage` resource could prevent all must-gather runs that use custom images.
    -   **Mitigation:** The operator will provide clear status conditions and events on the `MustGather` resource to indicate the reason for failure. Documentation will emphasize the importance of correctly configuring the `MustGatherImage` resource.

### Drawbacks

-   This enhancement introduces a new CRD that cluster administrators must manage.
-   Users must be aware of the `MustGatherImage` resource and the allowed images when creating a `MustGather` resource with a custom image.

## Test Plan

-   **Unit Tests:**
    -   Test the validation logic for the `MustGatherImage` CRD.
    -   Test the operator's logic for fetching the `MustGatherImage` resource and validating the custom image.
-   **E2E Tests:**
    -   Test the successful creation of a must-gather job with a custom image from the allowlist.
    -   Test the failure of a must-gather job with a custom image that is not in the allowlist.
    -   Test the failure of a must-gather job when the `MustGatherImage` resource does not exist and if the custom-image is specified, otherwise run must-gather job with default image.

## Graduation Criteria

### Dev Preview -> Tech Preview

-   Ability to utilize the enhancement end-to-end.
-   End-user documentation and API stability.
-   Sufficient test coverage.

### Tech Preview -> GA

-   More testing, including upgrade and scale scenarios.
-   Sufficient time for user feedback and adoption.
-   User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/).

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

-   This change is backward compatible. Existing `MustGather` resources that do not have the `mustGatherImage` field will continue to work as before.
-   On downgrade, the `mustGatherImage` field will be ignored by older operators.

## Version Skew Strategy

This enhancement does not introduce any version skew concerns. The change is self-contained within the must-gather-operator and its CRDs.

## Operational Aspects of API Extensions

The `MustGatherImage` CRD is the main API extension. The operator will manage its lifecycle. Failure to find the `MustGatherImage` resource or an invalid image will be surfaced as status conditions on the `MustGather` resource.

## Support Procedures

If a `must-gather` run with a custom image fails, support personnel should first inspect the `MustGather` resource's status and events to check for image-related errors (e.g., `InvalidImage`, `AllowlistNotConfigured`). If the image is valid, standard `must-gather` debugging procedures apply.

## Alternatives (Not Implemented)

A `ConfigMap` could have been used to store the list of allowed images. However, a CRD was chosen because it provides a more robust, Kubernetes-native solution with schema validation, RBAC integration, and better discoverability. This approach aligns with the operator pattern of extending the Kubernetes API to manage application configuration and provides a clearer audit trail.
