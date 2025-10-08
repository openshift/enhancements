---
title: event-ttl
authors:
  - "@tjungblu"
  - "CursorAI"
reviewers:
  - benluddy
  - p0lyn0mial
  - dgrisonnet
approvers:
  - sjenning
api-approvers:
  - JoelSpeed
creation-date: 2025-10-08
last-updated: 2025-10-15
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2095
  - https://issues.redhat.com/browse/CNTRLPLANE-1539
  - https://github.com/openshift/api/pull/2520
  - https://github.com/openshift/api/pull/2525
status: proposed
see-also:
replaces:
superseded-by:
---

# Event TTL Configuration

## Summary

This enhancement describes a configuration option in the operator API to configure the event-ttl setting for the kube-apiserver. The event-ttl setting controls how long events are retained in etcd before being automatically deleted.

Currently, OpenShift uses a default event-ttl of 3 hours (180 minutes), while upstream Kubernetes uses 1 hour. This enhancement allows customers to configure this value based on their specific requirements, with a range of 5 minutes to 3 hours (180 minutes), with a default of 180 minutes (3 hours).

## Motivation

The event-ttl setting in kube-apiserver controls the retention period for events in etcd. Events are automatically deleted after this duration to prevent etcd from growing indefinitely. Different customers have different requirements for event retention:

- Some customers need longer retention for compliance or debugging purposes
- Others may want shorter retention to reduce etcd storage usage
- The current fixed value of 3 hours may not suit all use cases

The maximum value of 3 hours (180 minutes) was chosen to align with the current OpenShift default value. While upstream Kubernetes uses 1 hour as the default, OpenShift's 3-hour default was established to support CI runs that may need to retain events for the entire duration of a test run. For customer use cases, the 3-hour maximum provides sufficient retention for compliance and debugging needs, while the 1-hour upstream default would be more appropriate for general customer workloads.

### Goals

1. Allow customers to configure the event-ttl setting for kube-apiserver through the OpenShift API
2. Provide a reasonable range of values (5 minutes to 3 hours) that covers most customer needs
3. Maintain backward compatibility with the current default of 3 hours (180 minutes)
4. Ensure the configuration is properly validated and applied

### Non-Goals

- Changing the default event-ttl value (will remain 3 hours/180 minutes)
- Supporting event-ttl values outside the recommended range (5-180 minutes)
- Modifying the underlying etcd compaction behavior beyond what the event-ttl setting provides

## Proposal

We propose to add an `eventTTLMinutes` field to the operator API that allows customers to configure the event-ttl setting for kube-apiserver.

### User Stories

#### Story 1: Storage Optimization
As a cluster administrator with limited etcd storage, I want to configure a shorter event retention period so that I can reduce etcd storage usage while maintaining sufficient event history for troubleshooting. Event data can consume significant etcd storage over time, and reducing the retention period can help manage storage growth.

#### Story 2: Default Behavior
As a cluster administrator, I want the current default behavior to be preserved so that existing clusters continue to work without changes.

### API Extensions

This enhancement modifies the operator API by adding a new `eventTTLMinutes` field.

### Workflow Description

The workflow for configuring event-ttl is straightforward:

1. **Cluster Administrator** accesses the OpenShift cluster via CLI or web console
2. **Cluster Administrator** edits the operator configuration resource
3. **Cluster Administrator** sets the `eventTTLMinutes` field to the desired value in minutes (e.g., 60, 180)
4. **kube-apiserver-operator** detects the configuration change
5. **kube-apiserver-operator** updates the kube-apiserver deployment with the new configuration
6. **kube-apiserver** restarts with the new event-ttl setting
7. **etcd** begins using the new event retention policy for future events

The configuration change takes effect immediately for new events, while existing events continue to use their original TTL until they expire.

### Topology Considerations

#### Hypershift / Hosted Control Planes

For HyperShift, this enhancement will use an annotation-based approach on the `HostedCluster` resource, just as with other kube-apiserver configurations such as `goaway-chance`. Users can control the event-ttl setting by specifying the `hypershift.openshift.io/event-ttl-minutes` annotation. The control-plane-operator will read this annotation and update the kube-apiserver deployment in the hosted control plane accordingly (following the pattern described in [openshift/hypershift#6019](https://github.com/openshift/hypershift/pull/6019)). HyperShift continues to use the same 3-hour default as standalone OpenShift clusters unless overridden.

#### Standalone Clusters

This enhancement is fully applicable to standalone OpenShift clusters. The event-ttl configuration will be applied to the kube-apiserver running in the control plane, affecting event retention in the cluster's etcd.

#### Single-node Deployments or MicroShift

For single-node OpenShift (SNO) deployments, this enhancement will work as expected. The event-ttl configuration will be applied to the kube-apiserver running on the single node.

For MicroShift, this enhancement is not directly applicable as MicroShift uses a different architecture and may not have the same event-ttl configuration options. MicroShift also uses a 3-hour TTL by default, but since it doesn't use the kube-apiserver operator, the configuration approach described in this enhancement may not work. 

### Implementation Details/Notes/Constraints

The proposed API looks like this:

```yaml
apiVersion: operator.openshift.io/v1
kind: KubeAPIServer
metadata:
  name: cluster
spec:
  eventTTLMinutes: 60  # Integer value in minutes, e.g., 60, 180
```

The `eventTTLMinutes` field will be an integer value representing minutes. The field will be validated to ensure it falls within the required range of 5-180 minutes. In the upstream Kubernetes API server configuration, `event-ttl` is typically set as a standalone parameter, so placing `eventTTLMinutes` directly under the operator spec without additional nesting maintains consistency with upstream patterns.

The API design is based on the changes in [openshift/api PR #2520](https://github.com/openshift/api/pull/2520), and the feature gate implementation is in [openshift/api PR #2525](https://github.com/openshift/api/pull/2525). The API changes include:

```go
type KubeAPIServerSpec struct {
	StaticPodOperatorSpec `json:",inline"`

	// eventTTLMinutes specifies the amount of time that the events are stored before being deleted.
	// The TTL is allowed between 5 minutes minimum up to a maximum of 180 minutes (3 hours).
	//
	// Lowering this value will reduce the storage required in etcd. Note that this setting will only apply
	// to new events being created and will not update existing events.
	//
	// When omitted this means no opinion, and the platform is left to choose a reasonable default, which is subject to change over time.
	// The current default value is 3h (180 minutes).
	//
	// +openshift:enable:FeatureGate=EventTTL
	// +kubebuilder:validation:Minimum=5
	// +kubebuilder:validation:Maximum=180
	// +optional
	EventTTLMinutes int32 `json:"eventTTLMinutes,omitempty"`
}
```

### Impact of Lower TTL Values

etcd uses an optimized lease expiration mechanism where a lessor runs in the background, polling every 500ms for expired leases using a queue ordered by expiration time (not O(N) iteration over all leases). The leader processes expired leases in parallel, and lease deletions are published via raft. 

Setting the event-ttl to values lower than the OpenShift default of 3 hours will primarily impact:

1. **etcd Memory and Disk Usage**: Lower TTL values reduce the number of active leases in etcd, resulting in lower memory and disk space consumption for event storage.

2. **Raft Operations**: The number of expired leases per minute remains roughly the same, as it is dependent on event arrival rate.

3. **Event Availability**: Events will be deleted more quickly, reducing the time window available for debugging and troubleshooting.


#### Fleet Analytics Data

Based on fleet analytics data, the storage impact of reducing event TTL can be quantified:

- **Largest Cluster**: ~3-4 million events with average size of 1.5KB
  - Reducing TTL from 3 hours to 1 hour (by 1/3) would reduce etcd event storage to approximately 1.5GB
- **Median Cluster**: ~1,391 events in storage
- **90th Percentile**: ~6,700 events in storage

This data shows that while the largest clusters would see significant storage savings (reducing from ~4.5GB to ~1.5GB for the biggest outlier), the majority of clusters have much smaller event footprints where the storage impact would be minimal. We expect, even drastic, lowering to not have any observable impact to CPU or bandwidth on the majority of our clusters.

#### Impact of configuring 5m TTL

 After filling etcd with approximately 4GB of events over 3 hours, then switching to a 5-minute TTL, we observed a sharp drop in storage usage and memory consumption on etcd after the 3h events have expired and the storage got compacted and defragmented. 

 CPU usage showed a slight initial increase followed by a reduction (measured across both etcd and apiserver components), while apiserver memory remained relatively stable. The compaction duration on etcd demonstrated the expected linear relationship with the number of keys being processed, confirming predictable performance characteristics under this workload.

There were no long-term increases in CPU/memory usage found after configuring a 5m TTL vs. the existing default. 

### Risks and Mitigations

**Risk**: Customers might set the value to 0, which means that the events will never be deleted
**Mitigation**: The API validation and operator ensures the values are within a reasonable range and never zero (5-180 minutes).

### Drawbacks

- Adds complexity to the configuration API
- Additional validation and error handling required

## Alternatives (Not Implemented)

1. **Hardcoded Values**: Keep the current fixed value of 3 hours
   - **Rejected**: Does not meet customer requirements for configurability

2. **Environment Variable**: Use environment variables instead of API configuration
   - **Rejected**: Less user-friendly and harder to manage

3. **Separate CRD**: Create a separate CRD for event configuration
   - **Rejected**: Overkill for a single setting, better to include in existing APIServer resource

## Test Plan

The test plan will include:

1. **Unit Tests**: Test the API validation and parsing logic
2. **E2E Tests**: Test that the event TTL is properly configured on all apiserver after applying the setting
3. **Performance Tests**: Test the impact of different TTL values on etcd performance

## Tech Preview

The EventTTL feature is controlled by the `EventTTL` feature gate, which is enabled by default in both DevPreview and TechPreview feature sets. This allows the feature to be available for testing and evaluation without requiring additional configuration.

The EventTTL feature gate is implemented in [openshift/api PR #2525](https://github.com/openshift/api/pull/2525) and will be removed when the feature graduates to GA, as the functionality will become a standard part of the platform.

## Graduation Criteria

### Dev Preview -> Tech Preview

- API is implemented and validated
- Basic functionality works end-to-end
- Documentation is available
- Sufficient test coverage
- EventTTL feature gate is enabled in DevPreview and TechPreview feature sets

### Tech Preview -> GA

- More comprehensive testing (upgrade, downgrade, scale)
- Performance testing with various TTL values
- User feedback incorporated
- Documentation updated in openshift-docs
- EventTTL feature gate is removed as the feature becomes GA

### Removing a deprecated feature

This enhancement does not remove any existing features. It only adds new configuration options while maintaining backward compatibility with the existing default behavior.

## Upgrade / Downgrade Strategy

### Upgrade Strategy

- Existing clusters will continue to use the default 3-hour (180-minute) TTL
- No changes required for existing clusters
- New configuration option is available immediately

### Downgrade Strategy

- Configuration will be ignored by older versions
- No impact on cluster functionality
- Events will continue to use the default TTL (180 minutes)

## Version Skew Strategy

- The event-ttl setting is a kube-apiserver configuration
- No coordination required with other components
- Version skew is not a concern for this enhancement

## Operational Aspects of API Extensions

This enhancement modifies the operator API but does not add new API extensions. The impact is limited to:

- Configuration validation in the kube-apiserver-operator
- Application of the setting to kube-apiserver deployment
- No impact on API availability or performance

## Support Procedures

### Detection

- Configuration can be verified by checking the operator configuration resource
- kube-apiserver logs will show the configured event-ttl value
- etcd metrics can be monitored for compaction frequency

### Troubleshooting

- If events are not being deleted as expected, check the event-ttl configuration
- Monitor etcd compaction metrics for unusual patterns

## Implementation History

- 2025-10-08: Initial enhancement proposal

