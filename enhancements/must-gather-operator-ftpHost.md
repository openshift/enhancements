---
title: must-gather-operator-ftpHost
authors:
  - "@shivprakahsmuley"
reviewers:
  - "@TrilokGeer"
  - "@Prashanth684"
approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
api-approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
creation-date: 2025-08-18
last-updated: 2025-08-18
tracking-link:
  - https://issues.redhat.com/browse/MG-53
status: implementable
see-also:

---

# Must Gather Operator Enhancement: Extensible Upload Targets

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement introduces a flexible `uploadTarget` field to the `MustGather.spec`. This new structure uses a discriminated union to allow specifying different upload destinations, starting with SFTP for Red Hat support case uploads. This provides a scalable and type-safe way to direct must-gather artifacts. If `uploadTarget` is unset, uploading is disabled.

## Motivation

### User Stories

* As a cluster admin, I can configure must-gather to upload artifacts to a designated SFTP server for support cases.
* As a cluster admin, I want a clear and extensible API to configure different upload targets as new requirements emerge.
* As an OpenShift security engineer, I can audit a single, well-defined API field (`uploadTarget`) to enforce policies on data exfiltration.

### Goals

* Replace the monolithic upload configuration with a flexible, discriminated union structure.
* Define an `SFTP` upload type for sending artifacts to a secure FTP server, including Red Hat support.
* Ensure the API is easily extensible for future upload types (e.g., S3, HTTP) without requiring breaking changes.
* Provide clear validation to ensure that only one upload target is configured at a time and that its configuration is valid.

### Non-Goals

* To implement upload types other than SFTP in the initial version.

## Proposal

This enhancement refactors the `MustGather.spec` by introducing a new `uploadTarget` field. This field uses a `unionDiscriminator` on a `type` field to enable extensible upload configurations. The existing top-level fields for upload configuration (`caseID`, `caseManagementAccountSecretRef`, and `ftpHost`) are removed and their functionality is moved into the new `uploadTarget.sftp` struct. This is a breaking change designed to create a more scalable and maintainable API.

### Workflow Description

1. The cluster administrator creates a `MustGather` custom resource.
2. To enable uploading, the administrator defines the `spec.uploadTarget` field.
3. They set `uploadTarget.type` to `SFTP` and provide the necessary configuration under `uploadTarget.sftp`, including the `caseID`, a reference to a secret with credentials, and an optional host override.
4. The must-gather operator validates the `uploadTarget` configuration.
5. The operator configures the must-gather job to use the specified SFTP details for uploading the artifact.
6. If `uploadTarget` is not specified, the must-gather collection runs, but the upload phase is skipped.

### API Extensions

#### Types: must-gather-operator/api/v1alpha1/mustgather_types.go

```go
// MustGatherSpec defines the desired state of MustGather
type MustGatherSpec struct {
    // ... existing non-upload fields ...
    
    // serviceAccountRef is the service account to be used for running must-gather.
    ServiceAccountRef corev1.LocalObjectReference `json:"serviceAccountRef"`

    // uploadTarget sets the target config for uploading the collected must-gather tar.
    // Uploading is disabled if this field is unset.
    // +optional
    UploadTarget *UploadTarget `json:"uploadTarget,omitempty"`
}

// UploadType is a specific method for uploading to a target.
// +kubebuilder:validation:Enum=SFTP
type UploadType string

// UploadTarget defines the configuration for uploading the must-gather tar.
// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'SFTP' ? has(self.sftp) : !has(self.sftp)",message="sftp upload target config is required when upload type is SFTP, and forbidden otherwise"
// +union
type UploadTarget struct {
    // type defines the method used for uploading to a specific target.
    // +unionDiscriminator
    // +required
    Type UploadType `json:"type"`

    // sftp defines the target details for uploading to a valid SFTP server.
    // +unionMember
    // +optional
    SFTP *SFTPUploadTargetConfig `json:"sftp,omitempty"`
}

// SFTPUploadTargetConfig defines the configuration for SFTP uploads.
type SFTPUploadTargetConfig struct {
    // caseID specifies the Red Hat case number for support uploads.
    // +kubebuilder:validation:MaxLength=128
    // +kubebuilder:validation:MinLength=1
    // +required
    CaseID string `json:"caseID"`

    // host specifies the SFTP server hostname.
    // +kubebuilder:default:="access.redhat.com"
    // +optional
    Host string `json:"host,omitempty"`

    // caseManagementAccountSecretRef references a secret containing the upload credentials.
    // +required
    CaseManagementAccountSecretRef corev1.LocalObjectReference `json:"caseManagementAccountSecretRef"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes


#### Standalone Clusters

This change is fully relevant for standalone clusters where must-gather operations are performed directly.

#### Single-node Deployments or MicroShift


### Implementation Details/Notes/Constraints

*   **API**: Introduce `UploadTarget` in `mustgather_types.go` with `union` markers and CEL validation. The previous top-level upload fields are removed.
*   **Controller**: The controller logic will be updated to parse the `uploadTarget` field. If present, it will configure the upload job based on the specified type and its configuration. If absent, no upload will be configured.
*   **Breaking Change**: This is a breaking API change. The top-level fields `caseID`, `caseManagementAccountSecretRef`, and `ftpHost` have been removed. Users must update their `MustGather` custom resources to use the new `uploadTarget` structure.
*   **CRD Generation**: All generated files (deepcopy, OpenAPI, CRDs) must be updated.

### Risks and Mitigations

*   **API Complexity**: The new API is more complex than a single string field, but this is a necessary trade-off for extensibility and type safety.
    *   **Mitigation**: Clear documentation and examples will be provided. CEL validation will prevent invalid configurations.
*   **Migration**: Users will need to update their `MustGather` resources to the new format.
    *   **Mitigation**: The backward compatibility logic will prevent disruption, and clear deprecation warnings will guide users.

### Drawbacks

*   The introduction of a nested structure makes the API slightly more verbose for the simple case of uploading to the default Red Hat support server.

## Alternatives (Not Implemented)

A simpler approach of keeping a flat structure with optional fields for each provider was considered. However, this lacks the type safety and explicit intent provided by a discriminated union, and it becomes cumbersome as more target types are added.

## Test Plan

### Unit Tests

*   Validation of `UploadTarget` struct: ensure CEL rules correctly reject invalid combinations.
*   Controller logic for parsing `uploadTarget` and configuring the upload job.
*   Backward compatibility logic for handling deprecated fields.

### E2E Tests

*   Create a `MustGather` resource with a valid `uploadTarget` of type `SFTP` and verify the upload succeeds.
*   Test with a custom SFTP host.
*   Verify that uploading is disabled when `uploadTarget` is unset.
*   Test the backward compatibility path by creating a `MustGather` resource using only the deprecated fields.

## Graduation Criteria

### Dev Preview -> Tech Preview

*   Ability to utilize the enhancement end to end for SFTP uploads.
*   End-user documentation and API stability.
*   Sufficient test coverage, including unit and e2e tests.
*   Gather feedback from users on the new API structure.

### Tech Preview -> GA

*   More testing, including upgrade and scale scenarios.
*   Sufficient time for user feedback and adoption.


### Removing a deprecated feature

The deprecated fields `caseID`, `caseManagementAccountSecretRef`, and `ftpHost` are removed.

## Upgrade / Downgrade Strategy

*   **Upgrade**: This is a breaking change. Before upgrading the operator, all `MustGather` custom resources must be migrated to the new `uploadTarget` API structure. The upgrade process for the operator should be blocked until all resources are compliant, or the operator should handle existing resources gracefully (e.g., by reporting an error condition). A migration tool or script may be provided to assist users.
*   **Downgrade**: Downgrading to an operator version that expects the old fields will fail for any `MustGather` resource created with the new `uploadTarget` structure. Manual conversion of the resources back to the old format would be required before a downgrade.

## Version Skew Strategy

The `MustGather` CRD and the must-gather-operator are the only components affected. The operator's controller is designed to handle the specific version of the CRD it is shipped with. Version skew issues are not expected as long as the CRD and the operator are upgraded together, which is standard practice.

## Operational Aspects of API Extensions

This enhancement modifies the `MustGather` CRD by introducing a new `uploadTarget` struct.

*   **SLIs**: This change has no impact on existing SLIs such as API throughput or availability. It is a configuration change for an on-demand job and does not affect the core performance of the cluster.
*   **Failure Modes**:
    *   Invalid `uploadTarget` configuration (e.g., wrong type or missing required fields) will be rejected by the API server via CEL validation.
    *   Runtime failures (e.g., unreachable SFTP host, invalid credentials) will result in a failed must-gather job, with errors reported in the job's logs and in the `MustGather` resource's conditions.
*   **Escalation**: Failures in the must-gather upload process will be escalated to the team responsible for the must-gather-operator.

## Support Procedures

### Detecting Issues

*   **Symptoms**: Must-gather job fails during the upload phase; no data appears at the configured destination.
*   **Logs**: Check the logs of the must-gather operator pod and the upload container within the must-gather job pod for connectivity errors, authentication failures, or other SFTP-related issues.

### Troubleshooting

*   Verify that the SFTP host is reachable from the cluster network.
*   Check cluster-wide proxy settings if applicable.
*   Ensure the secret referenced in `caseManagementAccountSecretRef` contains valid credentials.
*   Confirm that any firewalls between the cluster and the SFTP host allow the required traffic.

### Disabling the Feature

To disable uploading, simply remove the `uploadTarget` field from the `MustGather` resource. The must-gather collection will still run, but the upload step will be skipped.

## Examples

### SFTP Upload to Red Hat Support

This example configures an upload to the default Red Hat support SFTP server.

```yaml
apiVersion: managed.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: example-mustgather-sftp
spec:
  serviceAccountRef:
    name: must-gather-admin
  uploadTarget:
    type: SFTP
    sftp:
      caseID: "01234567"
      caseManagementAccountSecretRef:
        name: case-management-creds
```

### SFTP Upload to a Staging Environment

This example uses the `host` field to target a different SFTP server.

```yaml
apiVersion: managed.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: example-mustgather-sftp-staging
spec:
  serviceAccountRef:
    name: must-gather-admin
  uploadTarget:
    type: SFTP
    sftp:
      caseID: "01234567"
      caseManagementAccountSecretRef:
        name: case-management-creds
      host: access.stage.redhat.com
```

## Implementation History

*   v0: Initial proposal with a simple `ftpHost` string.
*   v1: Redesigned API to use an extensible `uploadTarget` field with a discriminated union for type safety and future scalability.

