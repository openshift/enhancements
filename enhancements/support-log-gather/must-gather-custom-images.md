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

## Goals

1.  Introduce a new `MustGatherImage` CRD to act as a centrally managed allowlist for custom must-gather images.

2.  Modify the `MustGather` CRD to allow users to specify a custom image for the must-gather job.

3.  Update the must-gather-operator to validate any specified custom image against the `MustGatherImage` allowlist.

4.  If the custom image is valid, the operator will use it to run the must-gather job.

5.  If the custom image is invalid or the allowlist does not exist, the `MustGather` resource will report a failure.

6.  Ensure that if no custom image is specified, the operator continues to use the default must-gather image.

## Non-Goals

1.  This proposal does not cover the process of building, hosting, or distributing custom must-gather images.

2.  It will not provide a mechanism for automatically updating or managing the lifecycle of the custom images themselves.

3.  It will not alter the content or scripts within the default must-gather image.

## Proposal

### 1. API Changes

#### 1.1 New `MustGatherImage` CRD

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

#### 1.2 `MustGather` CRD Modification

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

### 2. Operator Logic Changes

The `MustGatherReconciler` in `controllers/mustgather/mustgather_controller.go` will be updated to handle the new logic.

The image selection logic in the `getJobFromInstance` function will be modified as follows:

1.  Check if `instance.Spec.MustGatherImage` is set.

2.  **If it is not set**, the logic proceeds as it does today, using the default `OPERATOR_IMAGE`.

3.  **If it is set**, the controller will:

    a.  Fetch the `MustGatherImage` resource with the name `cluster`.

    b.  If the `cluster` resource is not found, the reconciliation will fail, and the `MustGather` status will be updated with an error indicating the allowlist is not configured.

    c.  If the resource is found, the controller will check if the image specified in `spec.mustGatherImage` is present in the `spec.images` list.

    d.  If the image is **not** in the list, the reconciliation will fail, and the `MustGather` status will be updated with an `InvalidImage` error.

    e.  If the image **is** in the list, the controller will use this image string when calling `getJobTemplate` to construct the must-gather job, overriding the default image.

### 3. RBAC Changes

The operator's ClusterRole will need to be updated in `deploy/` to grant `get`, `list`, and `watch` permissions on the `mustgatherimages` resource at the cluster scope.

```yaml
# deploy/02_must-gather-operator.ClusterRole.yaml
# ... existing rules ...
- apiGroups:
  - operator.openshift.io
  resources:
  - mustgatherimages
  verbs:
  - get
  - list
  - watch
```

Additionally, a new `ClusterRole` will be beneficial for administrators to manage the allowlist.

```yaml
# deploy/new_must-gather-image-admin.ClusterRole.yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: must-gather-image-admin
rules:
- apiGroups:
  - operator.openshift.io
  resources:
  - mustgatherimages
  verbs:
  - get
  - list
  - watch
  - create
  - update
  - patch
  - delete
```

## Alternatives Considered

A `ConfigMap` could have been used to store the list of allowed images. However, a CRD was chosen because it provides a more robust, Kubernetes-native solution with schema validation, RBAC integration, and better discoverability. This approach aligns with the operator pattern of extending the Kubernetes API to manage application configuration and provides a clearer audit trail.
