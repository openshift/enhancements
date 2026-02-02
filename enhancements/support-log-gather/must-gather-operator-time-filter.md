---
title: must-gather-operator-time-filter
authors:
  - "@pravekum"
reviewers:
  - "@TrilokGeer"
  - "@Prashanth684"
approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
api-approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
creation-date: 2026-01-15
last-updated: 2026-01-15
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/MG-165
see-also:
  - "/enhancements/support-log-gather/must-gather-operator.md"
  - "/enhancements/oc/must-gather.md"
---

# Must-Gather Time-Based Log Filtering

## Summary

This enhancement proposes adding time-based log filtering capabilities to the Must-Gather Operator API. By allowing users to specify time boundaries for log collection, the operator can significantly reduce the size of must-gather archives, speed up uploads, and make diagnostic data collection more efficient for investigating specific incidents.

## Motivation

Must-gather archives can grow very large on long-running clusters, especially those with verbose logging or many components. This creates several problems:

1. **Slow uploads**: Large archives take significant time to upload to Red Hat support cases
2. **Storage consumption**: Large archives consume cluster storage during collection and local storage after download
3. **Bandwidth constraints**: In air-gapped or bandwidth-limited environments, uploading large archives may be impractical
4. **Noise in diagnostics**: When investigating a specific incident, historical logs add noise and make analysis harder

Time-based filtering addresses these issues by allowing users to collect only the logs relevant to their investigation timeframe.

### User Stories

- As an OpenShift administrator investigating a recent cluster issue, I want to limit log collection to the last 2 hours so that I can quickly gather relevant diagnostic data without collecting weeks of historical logs.

- As a support engineer working on a customer case, I want to request must-gather data for a specific time window (e.g., between 2:00 PM and 4:00 PM yesterday) so that I can focus my analysis on when the incident occurred.

- As an OpenShift user in a bandwidth-constrained environment, I want to minimize the size of must-gather archives so that uploads to Red Hat support complete in a reasonable time.

- As a cluster administrator, I want to use familiar time filtering options (similar to `oc adm must-gather --since`) through the operator API so that I can leverage existing knowledge of the CLI.

### Goals

1. Provide time-based filtering options in the MustGather CR that align with the existing `oc adm must-gather` CLI flags
2. Support relative time filtering via duration strings (e.g., "2h", "30m")
3. Support absolute time filtering via RFC3339 timestamps
4. Reduce must-gather archive sizes for time-bounded investigations
5. Maintain backward compatibility with existing MustGather CRs that don't specify time filters

### Non-Goals

1. Implementing `until` and `untilTime` end-boundary filters (deferred due to upstream complexity)
2. Log level filtering (e.g., only errors)
3. Component-specific or namespace-specific log filtering
4. Size-based log limits (e.g., max bytes per container)
5. Modifying the upstream `oc adm must-gather` CLI behavior

## Proposal

Add two new fields to the `MustGatherSpec` API to support time-based log filtering:

1. **`gatherSpec.since`**: A duration field that specifies a relative time window from the current time
2. **`gatherSpec.sinceTime`**: A timestamp field that specifies an absolute point in time

These fields map directly to the existing `--since` and `--since-time` flags in the `oc adm must-gather` CLI, ensuring consistency between the declarative API and the imperative CLI.

### Workflow Description

**Cluster Administrator** is a human user responsible for collecting diagnostic data from an OpenShift cluster.

1. The cluster administrator identifies an issue that occurred approximately 2 hours ago
2. The administrator creates a MustGather CR with `gatherSpec.since: "2h"` to limit log collection
3. The Must-Gather Operator creates a collection Job with the time filter passed via environment variable
4. The gather container runs the must-gather script, which filters logs to only include entries from the last 2 hours
5. The resulting archive is significantly smaller than a full collection
6. The upload container (if configured) uploads the reduced archive to the support case
7. The administrator or support engineer receives a focused set of logs for analysis

**Alternative: Absolute Time Window**

1. The cluster administrator knows an incident occurred on January 10th, 2026 at 14:00 UTC
2. The administrator creates a MustGather CR with `gatherSpec.sinceTime: "2026-01-10T14:00:00Z"`
3. The operator collects only logs from that timestamp forward
4. This is useful when the incident time is known precisely or when collecting data after the fact

### API Extensions

The following fields are added under `spec.gatherSpec` in the `mustgathers.operator.openshift.io` CRD:

```go
// MustGatherSpec defines the desired state of MustGather
type MustGatherSpec struct {
	// ... existing fields ...

	// gatherSpec holds optional gather configuration that is passed through to the gather container.
	// +optional
	GatherSpec *GatherSpec `json:"gatherSpec,omitempty"`
}

// GatherSpec is a struct introduced by this enhancement to hold gather-time configuration.
type GatherSpec struct {
	// since restricts log collection to entries newer than the specified duration.
	// Accepts a duration string (e.g., "1h", "30m", "24h") relative to the current time.
	// When set, only logs within this time window are collected, reducing archive size.
	// If unset, all available logs are collected (default behavior).
	// This is passed to the gather container via the MUST_GATHER_SINCE environment variable.
	// +optional
	// +kubebuilder:validation:Format=duration
	Since *metav1.Duration `json:"since,omitempty"`

	// sinceTime restricts log collection to entries newer than the specified RFC3339 timestamp.
	// Accepts an RFC3339 compatible date time (e.g., "2024-01-01T00:00:00Z").
	// When set, only logs after this timestamp are collected, reducing archive size.
	// If unset, all available logs are collected (default behavior).
	// This is passed to the gather container via the MUST_GATHER_SINCE_TIME environment variable.
	// +optional
	SinceTime *metav1.Time `json:"sinceTime,omitempty"`
}
```

#### Example MustGather CR with Time Filtering

**Using relative duration:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: incident-investigation
  namespace: openshift-must-gather-operator
spec:
  serviceAccountName: must-gather-admin
  gatherSpec:
    since: "2h"
  uploadTarget:
    type: SFTP
    sftp:
      caseID: "01234567"
      caseManagementAccountSecretRef:
        name: case-management-creds
```

**Using absolute timestamp:**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: specific-time-investigation
  namespace: openshift-must-gather-operator
spec:
  serviceAccountName: must-gather-admin
  gatherSpec:
    sinceTime: "2026-01-10T14:00:00Z"
  uploadTarget:
    type: SFTP
    sftp:
      caseID: "01234567"
      caseManagementAccountSecretRef:
        name: case-management-creds
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations. The time filtering applies to log collection within the guest cluster where the MustGather CR is created. Control plane logs in the management cluster would require a separate MustGather CR in that context.

#### Standalone Clusters

Fully supported. This is the primary deployment model for the Must-Gather Operator.

#### Single-node Deployments or MicroShift

Time filtering reduces resource consumption during must-gather collection, which is beneficial for resource-constrained single-node deployments. Smaller archives also reduce storage pressure on single-node clusters.

### Implementation Details/Notes/Constraints

#### Environment Variable Passing

The operator passes time filter values to the gather container via environment variables:

- `MUST_GATHER_SINCE`: Contains the duration string (e.g., "2h")
- `MUST_GATHER_SINCE_TIME`: Contains the RFC3339 timestamp

These environment variables are already defined and supported by the must-gather toolchain as documented in `/enhancements/oc/must-gather.md`.

#### Validation

- `gatherSpec.since` and `gatherSpec.sinceTime` are mutually exclusive in practice but not enforced at the API level. If both are specified, `sinceTime` takes precedence (matching CLI behavior).
- The `gatherSpec.since` field uses `metav1.Duration` which validates duration format
- The `gatherSpec.sinceTime` field uses `metav1.Time` which validates RFC3339 format
- The operator will normalize the effective start time derived from `gatherSpec.since` / `gatherSpec.sinceTime`:
  - If the computed start time is earlier than the cluster lifecycle start time, it will be clamped to the beginning of the cluster lifecycle (effectively "collect all available logs").
  - This cannot be enforced via CRD validation because the API server does not have access to cluster creation time.

#### Gather Script Integration

The must-gather gather script already supports `--since` and `--since-time` flags. The script reads the environment variables and applies filtering to:

- Container logs via `oc logs --since` or `oc logs --since-time`
- Rotated pod logs by comparing timestamps in log file names
- Other time-stamped data where applicable

### Risks and Mitigations

| Risk | Mitigation |
|------|------------|
| User specifies too narrow a time window and misses relevant logs | Document best practices; recommend starting with broader windows |
| Time skew between cluster nodes causes inconsistent filtering | This is an existing issue with the must-gather toolchain; no new risk introduced |
| User confusion between `gatherSpec.since` (duration) and `gatherSpec.sinceTime` (timestamp) | Clear field documentation and examples in API comments and user docs |

### Drawbacks

- Adds complexity to the API with two similar-sounding fields
- Users must understand the difference between relative and absolute time filtering
- Does not provide end-boundary filtering (`until`/`untilTime`) which would enable true time window specification

## Alternatives (Not Implemented)

### Alternative 1: Single `timeFilter` Object

Instead of two separate fields, use a single structured object:

```go
type TimeFilter struct {
    // Type specifies the filtering mode: "duration" or "timestamp"
    Type string `json:"type"`
    // Value contains either a duration string or RFC3339 timestamp
    Value string `json:"value"`
}
```

**Rejected because**: This approach requires runtime type detection and doesn't leverage Kubernetes' built-in type validation for durations and timestamps.

### Alternative 2: Include `until` and `untilTime` Fields

Add end-boundary fields to enable true time window specification:

```go
Until     metav1.Duration `json:"until,omitempty"`
UntilTime metav1.Time     `json:"untilTime,omitempty"`
```

**Deferred because**: The upstream `oc adm must-gather` CLI does not yet support `--until` and `--until-time` flags. As noted in `/enhancements/oc/must-gather.md`: "Supporting `until` flags considerably increase the complexity of the task, so more research is required before pursuing it."

### Alternative 3: Preset Time Windows

Provide an enum of common time windows:

```go
// +kubebuilder:validation:Enum=LastHour;Last6Hours;Last24Hours;Last7Days;All
TimeWindow string `json:"timeWindow,omitempty"`
```

**Rejected because**: This approach is less flexible than arbitrary duration strings and doesn't align with the existing CLI interface.

## Open Questions

1. Should the API enforce mutual exclusivity between `gatherSpec.since` and `gatherSpec.sinceTime` via CEL validation, or allow both with documented precedence?

2. Should we add `tailLines` as an additional non-time-based filtering option for users who want to limit by line count rather than time?

## Test Plan

- **Unit tests**: Validate that the operator correctly passes `MUST_GATHER_SINCE` and `MUST_GATHER_SINCE_TIME` environment variables to the gather container
- **Manual tests**: Create MustGather CRs with various time filter values and verify the resulting archives contain only logs within the specified time window
- **E2E tests**: Full workflow test including upload to verify time-filtered archives are correctly generated and uploaded

## Graduation Criteria

### Dev Preview -> Tech Preview

- Time filtering fields available in the MustGather CRD
- Basic functionality working end-to-end
- Documentation for time filtering options

### Tech Preview -> GA

- Sufficient user feedback on the API design
- Comprehensive test coverage
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- No breaking changes to the API fields

## Upgrade / Downgrade Strategy

- **Upgrade**: Existing MustGather CRs continue to work. If `gatherSpec.since`/`gatherSpec.sinceTime` are omitted (the default), collection proceeds unfiltered (collect all logs).
- **Downgrade**:
  - If only the operator is downgraded (CRD still includes `gatherSpec.since`/`gatherSpec.sinceTime`), older operator versions will ignore the new fields and collection proceeds without filtering.
  - If the CRD is also downgraded to a version that does not include `gatherSpec.since`/`gatherSpec.sinceTime`, creating or updating MustGather CRs that specify those fields may be rejected by API validation and/or the fields may be pruned. Remove the fields (or recreate the CR) before downgrading.

## Version Skew Strategy

The time filtering feature is self-contained within the Must-Gather Operator. No coordination with other components is required. The operator and its CRD are versioned together.

## Operational Aspects of API Extensions

The `gatherSpec.since` and `gatherSpec.sinceTime` fields are optional additions to an existing CRD. They do not introduce new webhooks, finalizers, or API servers.

- **Impact on existing SLIs**: None. These are optional fields that only affect the behavior of newly created MustGather CRs.
- **Failure modes**: If the selected must-gather image does not honor `MUST_GATHER_SINCE` / `MUST_GATHER_SINCE_TIME`, log filtering will be ineffective and collection will default to full logs.

## Support Procedures

- **Detecting issues**: If time filtering is not working, check the gather container logs for warnings about invalid `MUST_GATHER_SINCE` or `MUST_GATHER_SINCE_TIME` values
- **Disabling**: Simply omit the `gatherSpec.since` and `gatherSpec.sinceTime` fields from the MustGather CR to collect all logs
- **Consequences of disabling**: Archives will be larger but will contain complete log history

## Infrastructure Needed

No additional infrastructure needed. This enhancement uses existing must-gather toolchain capabilities.
