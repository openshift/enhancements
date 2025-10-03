---
title: must-gather-operator-pvc-destination
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
creation-date: 2025-09-04
last-updated: 2025-09-04
tracking-link:
  - https://issues.redhat.com/browse/MG-68

---

# Must-Gather Operator: PVC destination for gathered data

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in `openshift-docs`

## Summary

Introduce an optional field in the `MustGather` custom resource that allows specifying a PersistentVolumeClaim (PVC) as a destination for gathered artifacts. When a PVC is specified, the must-gather operator mounts it into the gather pod, persisting all content written to `/must-gather` to the PVC. If not specified, it defaults to using ephemeral storage (`emptyDir`).

## Motivation

### Current Behavior and Problem Statement

The must-gather operator currently relies on ephemeral storage (`emptyDir` volumes) to store the data it collects. This means the gathered artifacts are tied directly to the lifecycle of the gather pod. This approach presents several significant challenges:

- **Data Loss:** Since the storage is ephemeral, any data collected is permanently lost if the gather pod is evicted, crashes, or is deleted for any reason. This makes the collection process fragile and unreliable, especially in unstable clusters where it is most needed.
- **Storage Capacity Limitations:** Ephemeral `emptyDir` volumes are constrained by the storage capacity of the underlying node. For large-scale data collection, such as gathering extensive logs or coredumps, `must-gather` can easily exhaust this limited space, causing the collection to fail.


### Proposed Solution

This enhancement addresses these issues by enabling the use of a PersistentVolumeClaim (PVC) for storing all `must-gather` artifacts. By writing directly to a persistent volume, we fundamentally change how data is managed:

- **Ensured Data Durability:** The lifecycle of the collected data is decoupled from the gather pod. Artifacts are safely stored on the PVC, surviving pod failures and enabling reliable data collection.
- **Simplified Data Access:** Once the `must-gather` job completes, the data is immediately available on the PVC for analysis, processing, or retrieval.
- **Scalable Data Collection:** Users can leverage persistent storage solutions that are not limited by a single node's capacity. This allows for the collection of much larger datasets without fear of failure due to storage limits.
- **Streamlined Artifact Management:** PVCs can be managed with standard Kubernetes storage policies, such as StorageClasses, quotas, and retention policies. This simplifies long-term retention, backup, and access control for `must-gather` artifacts.

### User Stories

- As a cluster administrator, I want must-gather output stored on a pre-provisioned PVC so that I can collect large datasets without failing due to ephemeral volume limits.
- As a support engineer, I want artifacts retained on a PVC with a defined retention policy so that I can audit and compare multiple runs.
- As a developer, I want to specify a subPath for runs so that I can organize multiple collections on a single PVC.


### Goals

- Introduce an optional `storage` field in the `MustGather` CRD to support PVC-backed storage for must-gather runs.
- If PVC is configured, ensure the gather container writes directly into the PVC by mounting it at `/must-gather`.
- If no storage is specified, continue to use ephemeral storage as the default.

### Non-Goals

- Automatic creation or lifecycle management of PVCs (out of scope for this enhancement; users bring or manage the PVC).
- Remote copies/exports (e.g., to object storage); this enhancement only covers writing to a PVC.

## Proposal

- Introduce an optional `spec.storage` section in the `MustGather` CRD. When specified, it must contain a `type` field (only `PersistentVolume` is supported) and a `persistentVolume` configuration.
- If `spec.storage` is configured, the controller mounts the referenced PVC at `/must-gather` and optionally uses `subPath` to organize runs.
- If `spec.storage` is not provided, the operator defaults to using an `emptyDir` volume for ephemeral storage, preserving the existing behavior.

### Workflow Description

When a user wants to use persistent storage, the workflow is as follows:
1. User creates a PVC in the same namespace as the `MustGather` resource (pre-provisioned by storage admin or dynamic provisioner).
2. User applies a `MustGather` resource with `spec.storage.type: PersistentVolume`, referencing the PVC.
3. The operator schedules the gather pod and mounts the PVC at `/must-gather`.
4. The gather container runs as-is and writes its output to the mounted path.
5. On completion, artifacts are available on the PVC for subsequent retrieval or processing.

### Workflow for Ephemeral Storage (Default)

1. User applies a `MustGather` resource without a `spec.storage` section.
2. The operator schedules the gather pod using an `emptyDir` volume mounted at `/must-gather`.
3. The gather container runs and writes its output to the `emptyDir` volume.
4. Artifacts are available for the lifetime of the pod.

Example `MustGather` CR:

```yaml
apiVersion: must-gather.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: network-debug
  namespace: must-gather
spec:
  storage:
    type: PersistentVolume
    persistentVolume:
      claim:
        name: mg-artifacts
      # Optional: organize multiple runs in a single PVC
      subPath: runs/2025-09-01T12-00Z
      # Artifacts are written to /must-gather in the gather container
```

### API Extensions

This enhancement modifies the `MustGather` CRD schema to include a new `spec.storage` object that controls where artifacts are written.

Proposed schema:

```yaml
spec:
  type: object
  properties:
    storage:
      type: object
      required:
        - type
        - persistentVolume
      properties:
        type:
          type: string
          enum:
            - PersistentVolume
          description: "Select PersistentVolume for artifact storage"
        persistentVolume:
          type: object
          properties:
            claim:
              type: object
              properties:
                name:
                  type: string
                  maxLength: 253
                  description: "PVC name in the same namespace"
              required:
                - name
            # Optional fields
            subPath:
              type: string
              description: "Optional subPath within the PVC to place artifacts"
```

Behavioral notes:

- The operator mounts the configured PVC at `/must-gather` in the gather container.
- The PVC must reside in the same namespace as the `MustGather` resource.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement has no unique considerations for Hypershift. The must-gather operator runs in the guest cluster, and the PVC is expected to be available there.

#### Standalone Clusters

This change is relevant for standalone clusters.

#### Single-node Deployments or MicroShift

This proposal does not significantly affect the resource consumption of a single-node OpenShift deployment. It relies on the underlying storage infrastructure to provide the PVC. This is not applicable to MicroShift as `must-gather` is not a component of MicroShift.

### Implementation Details/Notes/Constraints

- Mount Strategy: Mount the PVC Volume at `/must-gather`.
- Multi-Container: mount the same volume consistently across containers.
- Access Modes: Ensure docs call out that RWO PVCs may schedule gather pods on the bound node; for RWX, any node can mount.
- Node Placement: The gather pod inherits default scheduling; PVC storage class/node affinity may implicitly constrain scheduling.
- Cleanup: This enhancement does not delete or modify the PVC. Users manage lifecycle.

#### Controller and Job template changes

The must-gather operator currently renders a Kubernetes Job from a Go template (see job template for reference: [controllers/mustgather/template.go](https://github.com/openshift/must-gather-operator/blob/master/controllers/mustgather/template.go)). This enhancement requires the controller to alter the Job's volumes and volumeMounts based on `spec.storage`:

- If `spec.storage` is provided and its type is `PersistentVolume`:
  - Replace the volume that backs the output path with a `persistentVolumeClaim` source using `persistentVolume.claim.name`.
  - Ensure the gather container's `volumeMounts` mounts that volume at `/must-gather`.
  - If `persistentVolume.subPath` is provided, set `subPath` on the `volumeMount`.
- If `spec.storage` is not provided:
  - The operator will continue to use an `emptyDir` volume, preserving the current behavior.

Illustrative YAML fragment of the Job spec when PVC is configured:

```yaml
spec:
  template:
    spec:
      volumes:
        - name: must-gather-out
          persistentVolumeClaim:
            claimName: <.spec.storage.persistentVolume.claim.name>
      containers:
        - name: gather
          volumeMounts:
            - name: must-gather-out
              mountPath: /must-gather
              # only set when provided
              subPath: <.spec.storage.persistentVolume.subPath>
```

### Risks and Mitigations

- Incorrect AccessMode: Scheduling or mount may fail; expose clear status conditions and events.
- PVC Pending/Unbound: The controller waits and surfaces a `PVCNotBound` condition; document that the PVC must exist and be bound.
- Insufficient Capacity (ENOSPC): Collection may fail when the PVC fills; surface a `Failed` condition with reason; recommend sizing guidance and quotas.
- SubPath misuse: Using a `subPath` already populated may overwrite data; document best practices and recommend unique run directories.
- Namespace mismatch: PVC must be in the same namespace; validate and surface a `ValidationFailed` condition if not.
- Cleanup/retention: Artifacts persist on PVC; document user responsibility for retention and provide guidance for lifecycle policies.

### Drawbacks

- Users must manage PVC lifecycle and capacity planning.
- Potential for misconfiguration (e.g., wrong access mode) causing gather delays.

### Output Format

Unchanged. Must-gather images continue writing under `/must-gather`; directory structure is preserved, now backed by a PVC when configured.

## Test Plan

- Unit tests for CRD defaulting/validation of `spec.storage.persistentVolume`.
- E2E tests:
  - Happy path: Pre-created PVC (RWO), must-gather completes, artifacts present on the PVC.
  - With `subPath`: Artifacts appear under the provided subpath.
  - PVC Pending: Operator does not start gather until bound.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

- This change is backward compatible.
- Existing `MustGather` resources that do not have the `storage` field will continue to work as before, using ephemeral `emptyDir` storage.
- New `MustGather` resources can optionally include the `storage` field to use a PVC.
- On downgrade, the `storage` field will be ignored by older operators. The CRD will have the new field, but the old operator won't know about it. The behavior will be as if it's not there.

## Version Skew Strategy

This enhancement does not introduce any version skew concerns. The change is self-contained within the must-gather operator and its CRD.

## Operational Aspects of API Extensions

The MustGather CRD is the only API extension. The operator will manage its lifecycle. Failure to provision a PVC or incorrect permissions will be surfaced as status conditions on the MustGather resource.

## Support Procedures

If a `must-gather` run fails, support personnel should first inspect the `MustGather` resource's status and events to check for PVC-related errors (e.g., `PVCNotFound`, `PVCNotBound`). If the PVC is correctly bound, standard `must-gather` debugging procedures apply by inspecting the gather pod's logs.

## Implementation History

- 2025-09-04: Initial proposal.

## Alternatives (Not Implemented)


## Infrastructure Needed

- None beyond a Kubernetes storage class capable of provisioning PVCs appropriate for cluster size and expected artifact volume. 

### MustGather Spec (illustrative)

Spec fields overview:

```go
// +kubebuilder:validation:Enum=PersistentVolume
type StorageType string

const (
	StorageTypePersistentVolume StorageType = "PersistentVolume"
)

type MustGatherSpec struct {
	// +optional
	Storage *Storage `json:"storage,omitempty"`
}

type Storage struct {
	// +required
	Type StorageType `json:"type"`
	// +required
	PersistentVolume PersistentVolumeConfig `json:"persistentVolume"`
}

type PersistentVolumeConfig struct {
	// +required
	Claim  PersistentVolumeClaimReference `json:"claim"`
	// +optional
	SubPath string `json:"subPath,omitempty"`
}

type PersistentVolumeClaimReference struct {
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:XValidation:rule="!format.dns1123Subdomain().validate(self).hasValue()",message="a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character."
	// +required
	Name string `json:"name"`
}
``` 