---
title: must-gather-operator-ftphost
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

Introduce a required field in the `MustGather` custom resource that specifies a PersistentVolumeClaim (PVC) as the destination for gathered artifacts. The must-gather operator mounts the referenced PVC into the gather pod so that all content written to `/must-gather` is persisted to the PVC.

## Motivation

- Today, must-gather content produced by the operator is often stored in ephemeral volumes, requiring additional copying and risking data loss if the pod is evicted or the namespace is deleted.
- Providing a PVC destination enables:
  - Durable storage of artifacts for post-processing
  - Collection of larger datasets without hitting ephemeral volume limits
  - Easier retention by applying PVC retention policies and quotas
  - Lower operational toil by avoiding ad-hoc copy steps from pod ephemeral storage

### User Stories

- As a cluster administrator, I want must-gather output stored on a pre-provisioned PVC so that I can collect large datasets without failing due to ephemeral volume limits.
- As a support engineer, I want artifacts retained on a PVC with a defined retention policy so that I can audit and compare multiple runs.
- As a developer, I want to specify a subPath for runs so that I can organize multiple collections on a single PVC.


### Goals

- Require PVC-backed storage for all must-gather runs by adding a required `storage.persistentVolume` field to the `MustGather` CRD.
- Ensure the gather container writes directly into the PVC by mounting it at `/must-gather`.

### Non-Goals

- Automatic creation or lifecycle management of PVCs (out of scope for this enhancement; users bring or manage the PVC).
- Remote copies/exports (e.g., to object storage); this enhancement only covers writing to a PVC.

## Proposal

- Introduce a `spec.storage` section in the `MustGather` CRD with a required `type` field (only `PersistentVolume` is supported) and a required `persistentVolume` configuration.
- The controller mounts the referenced PVC at `/must-gather` and optionally uses `subPath` to organize runs.
- Ephemeral storage is no longer supported.

### Workflow Description

1. User creates a PVC in the same namespace as the `MustGather` resource (pre-provisioned by storage admin or dynamic provisioner).
2. User applies a `MustGather` resource with `spec.storage.type: PersistentVolume`, referencing the PVC.
3. The operator schedules the gather pod and mounts the PVC at `/must-gather`.
4. The gather container runs as-is and writes its output to the mounted path.
5. On completion, artifacts are available on the PVC for subsequent retrieval or processing.

Example `MustGather` CR:

```yaml
apiVersion: must-gather.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: network-debug
  namespace: must-gather
spec:
  images:
    - quay.io/openshift/origin-must-gather:latest
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
  required:
    - storage
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

### Implementation Details/Notes/Constraints

- Mount Strategy: Mount the PVC Volume at `/must-gather`.
- Multi-Container: mount the same volume consistently across containers.
- Access Modes: Ensure docs call out that RWO PVCs may schedule gather pods on the bound node; for RWX, any node can mount.
- Node Placement: The gather pod inherits default scheduling; PVC storage class/node affinity may implicitly constrain scheduling.
- Cleanup: This enhancement does not delete or modify the PVC. Users manage lifecycle.

#### Controller and Job template changes

The must-gather operator currently renders a Kubernetes Job from a Go template (see job template for reference: [controllers/mustgather/template.go](https://github.com/openshift/must-gather-operator/blob/master/controllers/mustgather/template.go)). This enhancement requires the controller to alter the Job's volumes and volumeMounts based on `spec.storage`:

- Replace the volume that backs the output path with a `persistentVolumeClaim` source using `persistentVolume.claim.name`.
- Ensure the gather container's `volumeMounts` mounts that volume at `/must-gather`.
- If `persistentVolume.subPath` is provided, set `subPath` on the `volumeMount`.

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

## Design Details

### Output Format

Unchanged. Must-gather images continue writing under `/must-gather`; directory structure is preserved, now backed by a PVC when configured.

### Test Plan

- Unit tests for CRD defaulting/validation of `spec.storage.persistentVolume`.
- E2E tests:
  - Happy path: Pre-created PVC (RWO), must-gather completes, artifacts present on the PVC.
  - With `subPath`: Artifacts appear under the provided subpath.
  - PVC Pending: Operator does not start gather until bound.


### Graduation Criteria

- Dev/Tech Preview: Field is required; basic E2E coverage.
- GA: Robust status conditions, documentation, and SRE operational runbooks updated.

### Upgrade / Downgrade Strategy

- Not backwards compatible. The `spec.storage.persistentVolume` field is required and ephemeral storage is removed.
- Existing `MustGather` resources must be updated to include `storage.persistentVolume`.


## Implementation History

- 2025-09-04: Initial proposal.

## Drawbacks

- Users must manage PVC lifecycle and capacity planning.
- Potential for misconfiguration (e.g., wrong access mode) causing gather delays.

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
	Images  []string `json:"images,omitempty"`
	Storage Storage  `json:"storage"`
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