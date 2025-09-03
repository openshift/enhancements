---
title: must-gather-operator-disable-upload
authors:
  - "@rausingh"
reviewers:
  - "@TrilokGeer"
  - "@Prashanth684"
approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
api-approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
creation-date: 2025-01-27
last-updated: 2025-01-27
tracking-link:
  - https://issues.redhat.com/browse/MG-67
status: implementable
see-also:
---

# Must Gather Operator Enhancement: DisableUpload in MustGatherSpec

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Add an optional field `disableUpload` to `MustGather.spec` that allows users to control whether the must-gather bundle should be uploaded to the SFTP server. When set to `true`, the bundle will be collected but not uploaded, providing users with the flexibility to handle the collected data locally or through alternative means.

Supported values: `true`, `false`
Default: `false` (upload enabled by default for backward compatibility)

## Motivation

### User Stories

* As a cluster admin, I want to collect must-gather data without automatically uploading it to Red Hat's case management system so I can review the data locally first.
* As a security-conscious administrator, I want to disable automatic upload to ensure sensitive cluster information doesn't leave my environment without explicit approval.
* As a developer or QA engineer, I want to collect must-gather data for debugging purposes without creating unnecessary uploads to production case management systems.
* As a support engineer, I want to provide customers with the option to collect data locally when they have concerns about uploading sensitive information.

### Goals

* Add `spec.disableUpload` with boolean validation and a safe default.
* Ensure the upload container respects the disableUpload flag and skips upload when disabled.
* Make `caseID` and `caseManagementAccountSecretRef` optional when upload is disabled.
* Maintain full backward compatibility.
* Provide clear examples and documentation for both use cases.

### Non-Goals

* Introduce alternative upload mechanisms (e.g., different protocols or destinations).
* Implement data obfuscation or filtering capabilities.
* Add compression or encryption features for local storage.

## Proposal

When `spec.disableUpload` is set to `true`, the must-gather process will:
1. Collect all the required cluster data as usual
2. Skip the upload phase entirely
3. Allow the job to complete successfully without requiring case management credentials
4. Make `caseID` and `caseManagementAccountSecretRef` optional fields

When `spec.disableUpload` is set to `false` (default), the operator continues to work exactly as before, maintaining full backward compatibility.

### Workflow Description

**cluster administrator** is a human user responsible for configuring must-gather collection.

1. The cluster administrator creates a MustGather custom resource with an optional `disableUpload` field.
2. If `disableUpload` is set to `true`:
   - The must-gather operator creates a job that collects data but skips upload
   - `caseID` and `caseManagementAccountSecretRef` become optional
   - The upload container receives the `disable_upload=true` environment variable
3. If `disableUpload` is omitted or set to `false`:
   - The operator behaves exactly as before (backward compatibility)
   - `caseID` and `caseManagementAccountSecretRef` remain required
   - The upload container performs the normal upload process

### API Extensions

#### Types: must-gather-operator/api/v1alpha1/mustgather_types.go

```go
// MustGatherSpec defines the desired state of MustGather
// +kubebuilder:validation:XValidation:rule="!(has(self.disableUpload) && self.disableUpload) ? (has(self.caseID) && self.caseID != ” && has(self.caseManagementAccountSecretRef) && self.caseManagementAccountSecretRef.name != ”) : true",message="caseID and caseManagementAccountSecretRef are required when disableUpload is false or unset"
type MustGatherSpec struct {
    // The is of the case this must gather will be uploaded to
    // Required when disableUpload is false, optional when disableUpload is true
    // +kubebuilder:validation:Optional
    CaseID string `json:"caseID,omitempty"`

    // the secret container a username and password field to be used to authenticate with red hat case management systems
    // Required when disableUpload is false, optional when disableUpload is true
    // +kubebuilder:validation:Optional
    CaseManagementAccountSecretRef corev1.LocalObjectReference `json:"caseManagementAccountSecretRef,omitempty"`

    // ...existing fields...

    // A flag to control whether the must-gather bundle should be uploaded to SFTP server.
    // If set to true, the bundle will be collected but not uploaded.
    // +kubebuilder:default:=false
    DisableUpload bool `json:"disableUpload,omitempty"`
}
```

#### CRD OpenAPI Schema

```yaml
spec:
  type: object
  properties:
    disableUpload:
      type: boolean
      default: false
      description: "A flag to control whether the must-gather bundle should be uploaded to SFTP server. If set to true, the bundle will be collected but not uploaded."
    caseID:
      type: string
      description: "The is of the case this must gather will be uploaded to. Required when disableUpload is false, optional when disableUpload is true"
    caseManagementAccountSecretRef:
      type: object
      description: "the secret container a username and password field to be used to authenticate with red hat case management systems. Required when disableUpload is false, optional when disableUpload is true"
  x-kubernetes-validations:
  - message: caseID and caseManagementAccountSecretRef are required when disableUpload is false or unset
    rule: '!(has(self.disableUpload) && self.disableUpload) ? (has(self.caseID)
      && self.caseID != ” && has(self.caseManagementAccountSecretRef) &&
      self.caseManagementAccountSecretRef.name != ”) : true'
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

This change is relevant for hosted control planes where administrators may want to collect data locally before deciding whether to upload it to Red Hat support.

#### Standalone Clusters

This change is fully relevant for standalone clusters where must-gather operations are performed directly within the cluster.

#### Single-node Deployments or MicroShift

This enhancement is applicable to single-node deployments and MicroShift environments where resource constraints or security policies may require local data collection without automatic upload.

### Implementation Details/Notes/Constraints

* **Prerequisites**: Requires Kubernetes 1.25+ for CEL validation support (enabled by default)
* **API**: Add `DisableUpload` to `MustGatherSpec` with kubebuilder default marker and CEL validation rules.
* **Validation**: Use kubebuilder CEL validation (`+kubebuilder:validation:XValidation`) to enforce that `caseID` and `caseManagementAccountSecretRef` are mandatory when `disableUpload` is false or unset.
* **Template**: Update upload container to pass `disable_upload` environment variable from `spec.disableUpload`.
* **Secret Management**: Skip secret copying to operator namespace when upload is disabled.
* **Defaulting**: Respect defaulting when field is unset to maintain backward compatibility.
* **CRD Generation**: Regenerate deepcopy, OpenAPI, CRDs, and bundle files.

### Risks and Mitigations

* **Data Loss**: Users might forget to manually handle collected data when upload is disabled.
  * *Mitigation*: Clear documentation and examples showing how to access collected data.
* **Support Impact**: Support cases may be harder to handle without automatic upload.
  * *Mitigation*: Documentation should clearly explain when to use this feature and how to manually upload data later if needed.
* **Security**: Collected data remains on the cluster longer when upload is disabled.
  * *Mitigation*: Job cleanup mechanisms remain in place; documentation should advise on data handling best practices.

### Drawbacks

* Adds another user-visible configuration option to the API.
* Requires users to understand the implications of disabling upload.

## Alternatives

* **Post-processing Flag**: Could have implemented a flag to delete uploaded data after upload, but the requirement is to prevent upload entirely.

## Open Questions

None at this time.

## Test Plan

### Unit Tests

* API defaulting validation (`disableUpload` defaults to false)
* CEL validation rules for conditional field requirements:
  - Fields are optional when `disableUpload` is true
  - Fields are required when `disableUpload` is false or unset
  - Validation error messages are clear and helpful
* Job template generation includes `disable_upload` environment variable when set
* Upload container environment variable handling for disabled upload scenarios

### E2E Tests

* Happy path with `disableUpload` set to true - job completes without upload
* Default path (unset or `false`) - normal upload behavior
* CEL validation tests:
  - Missing `caseID` or `caseManagementAccountSecretRef` when `disableUpload` is `false` or unset should fail at admission time
  - Missing `caseID` or `caseManagementAccountSecretRef` or both when `disableUpload` is `true` should succeed
  - Empty string values for required fields when upload is enabled should fail
* Secret management test - no secret copying when upload is disabled

## Graduation Criteria

### Dev Preview -> Tech Preview

* Field added with default validation and CEL validation rules
* Unit tests implemented for API validation and controller logic
* Basic documentation available
* Ability to utilize the enhancement end to end

### Tech Preview -> GA

* E2E tests implemented
* Customer validation in staging environments
* User-facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)
* Performance impact assessment completed

## Upgrade / Downgrade Strategy

* **Backward compatible**: Field is optional with a safe default (`false`)
* **Upgrade**: Older operators ignore the unknown field; no disruption to current flows
* **Downgrade**: If downgrading to an operator version that doesn't support `disableUpload`, the field is ignored and uploads continue as normal

## Version Skew Strategy

* Ensure operator CSV/CRD includes the new field before shipping the controller that uses it
* If controller is older than CRD, it safely ignores the field
* No coordination required between control plane and kubelet components

## Operational Aspects of API Extensions

This enhancement modifies the MustGather CRD by adding an optional field with boolean validation, a default value, and CEL validation rules for conditional field requirements.

**Impact on existing SLIs**:
* No impact on API throughput or availability
* Minimal impact on admission latency due to CEL validation (microseconds)
* No impact on cluster performance as this is a configuration field only used during must-gather operations

**Failure modes**:
* CEL validation failures for missing required fields when upload is enabled are caught at admission time with clear error messages
* Upload container failures are handled the same way regardless of upload status
* Invalid CEL expressions would prevent CRD installation (caught during development/testing)

## Support Procedures

### Detecting Issues

* **Symptoms**: Job completion without upload when upload was expected, or upload attempts when upload was disabled
* **Logs**: Check must-gather operator logs and must-gather job logs for upload-related messages

### Troubleshooting

* Verify `disableUpload` field value in the MustGather resource
* Validate that `caseID` and `caseManagementAccountSecretRef` are provided when upload is enabled

### Disabling the Feature

* Set `disableUpload: false` or omit the field from MustGather resources to use default behavior
* No cluster-wide disable mechanism needed as this is a per-resource configuration

## Examples

### Default Configuration (upload enabled)

```yaml
apiVersion: managed.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: example-mustgather-upload
spec:
  caseID: "01234567"
  caseManagementAccountSecretRef:
    name: case-management-creds
  serviceAccountRef:
    name: must-gather-admin
  # disableUpload omitted → defaults to false (upload enabled)
```

### Upload Disabled Configuration

```yaml
apiVersion: managed.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: example-mustgather-local
spec:
  serviceAccountRef:
    name: must-gather-admin
  disableUpload: true
  # caseID and caseManagementAccountSecretRef are optional when upload is disabled
```

### Minimal Upload Disabled Configuration

```yaml
apiVersion: managed.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: mustgather-upload-disabled-minimal
spec:
  serviceAccountRef:
    name: must-gather-admin
  disableUpload: true
```

## Implementation History

* v0: Draft proposal created based on MG-67 requirements
* v1: API added with boolean field, default, and CEL validation rules; unit tests implemented
* v2: E2E tests and examples; CRDs regenerated with CEL validation; documentation updated
* v3: Ready for tech preview after validation and testing
