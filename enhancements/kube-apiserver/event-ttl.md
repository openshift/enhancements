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
last-updated: 2026-10-09
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2095
  - https://issues.redhat.com/browse/CNTRLPLANE-1539
  - https://github.com/openshift/api/pull/2520
  - https://github.com/openshift/api/pull/2525
  - https://issues.redhat.com/browse/OCPSTRAT-3679
  - https://issues.redhat.com/browse/CNTRLPLANE-4415
  - https://issues.redhat.com/browse/CNTRLPLANE-4416
status: proposed
see-also:
replaces:
superseded-by:
---

# Event TTL Configuration

## Summary

This enhancement describes a configuration option to configure the event-ttl setting for the kube-apiserver. The event-ttl setting controls how long events are retained in etcd before being automatically deleted.

Currently, OpenShift uses a default event-ttl of 3 hours (180 minutes), while upstream Kubernetes uses 1 hour. This enhancement allows customers to configure this value based on their specific requirements, with a range of 5 minutes to 3 hours (180 minutes), with a default of 180 minutes (3 hours).

The enhancement covers two topologies, each with its own API surface because they have different control plane architectures:

- **Standalone OpenShift** (including SNO): an `eventTTLMinutes` field on the `KubeAPIServer` operator resource, reconciled by the kube-apiserver-operator. This part of the enhancement is already proposed and implemented; see [openshift/enhancements#1857](https://github.com/openshift/enhancements/pull/1857).
- **Hosted control planes (HyperShift)**: a new `eventTTLMinutes` field on the `HostedCluster` resource, reconciled by the HyperShift operator and the control-plane-operator. Hosted control planes have no kube-apiserver operator, so the standalone mechanism cannot be reused. This part of the enhancement is new; the design is described in [Hosted Control Planes (HyperShift)](#hosted-control-planes-hypershift).

Both topologies share the same units (minutes), the same accepted range (5-180), and the same effective default (180 minutes / 3 hours).

## Motivation

The event-ttl setting in kube-apiserver controls the retention period for events in etcd. Events are automatically deleted after this duration to prevent etcd from growing indefinitely. Different customers have different requirements for event retention:

- Some customers need longer retention for compliance or debugging purposes
- Others may want shorter retention to reduce etcd storage usage
- The current fixed value of 3 hours may not suit all use cases

The maximum value of 3 hours (180 minutes) was chosen to align with the current OpenShift default value. While upstream Kubernetes uses 1 hour as the default, OpenShift's 3-hour default was established to support CI runs that may need to retain events for the entire duration of a test run. For customer use cases, the 3-hour maximum provides sufficient retention for compliance and debugging needs, while the 1-hour upstream default would be more appropriate for general customer workloads.

### Motivation for Hosted Control Planes

In a hosted control plane the kube-apiserver is started with `--event-ttl=3h`, hardcoded in the control-plane-operator (`args.Set("event-ttl", "3h")` in [`control-plane-operator/controllers/hostedcontrolplane/v2/kas/config.go`](https://github.com/openshift/hypershift/blob/main/control-plane-operator/controllers/hostedcontrolplane/v2/kas/config.go)).
Neither the HostedCluster owner nor the service provider's SRE team has a supported way to change it.

This matters more for hosted control planes than for standalone clusters because the hosted etcd is part of the managed control plane and is sized and paid for by the service provider:

- Event-heavy workloads (CI/CD, GitOps controllers, pipelines) can retain a large number of events for the full 3 hours, consuming hosted etcd storage that the hosted cluster owner cannot reclaim.
- In a managed fleet, many hosted control planes share the same management cluster, so the aggregate etcd footprint of events scales with the number of hosted clusters.
- Hosted clusters are frequently short-lived or purpose-built, and a 3-hour retention window is often longer than the cluster owner needs.

Because hosted control planes have no kube-apiserver operator, the standalone `operator.openshift.io/v1` `KubeAPIServer` resource does not exist in a hosted cluster, and the standalone mechanism described in this enhancement cannot be reused as-is. The capability therefore has to be exposed through the HyperShift API.

### Goals

1. Allow customers to configure the event-ttl setting for kube-apiserver through the OpenShift API
2. Provide a reasonable range of values (5 minutes to 3 hours) that covers most customer needs
3. Maintain backward compatibility with the current default of 3 hours (180 minutes)
4. Ensure the configuration is properly validated and applied
5. Allow the same configuration for HyperShift hosted clusters through the `HostedCluster` API, with the same units, range, and default as standalone OpenShift
6. Propagate a configured value to the hosted kube-apiserver without requiring the hosted cluster to be recreated

### Non-Goals

- Changing the default event-ttl value (will remain 3 hours/180 minutes) for either standalone OpenShift or hosted control planes
- Supporting event-ttl values outside the recommended range (5-180 minutes)
- Modifying the underlying etcd compaction behavior beyond what the event-ttl setting provides
- Making the standalone `operator.openshift.io/v1` `KubeAPIServer` resource available inside a hosted cluster
- Allowing the setting to be changed from inside the hosted cluster; it is a control plane setting owned by whoever owns the `HostedCluster` resource
- Applying the setting to the management cluster's own kube-apiserver

## Proposal

We propose to add an `eventTTLMinutes` field to the operator API that allows customers to configure the event-ttl setting for kube-apiserver.

For hosted control planes, where no kube-apiserver operator exists, we propose to add an equivalent `eventTTLMinutes` field to the `HostedCluster` API under the existing per-component configuration stanza `spec.operatorConfiguration.kubeAPIServer`.

### User Stories

#### Story 1: Storage Optimization
As a cluster administrator with limited etcd storage, I want to configure a shorter event retention period so that I can reduce etcd storage usage while maintaining sufficient event history for troubleshooting. Event data can consume significant etcd storage over time, and reducing the retention period can help manage storage growth.

#### Story 2: Default Behavior
As a cluster administrator, I want the current default behavior to be preserved so that existing clusters continue to work without changes.

#### Story 3: Hosted Cluster Storage Optimization
As the owner of a HyperShift `HostedCluster` running event-heavy workloads (CI/CD, GitOps, pipelines), I want to lower the event retention period for my hosted cluster so that accumulated events do not consume hosted etcd storage for three hours, without having to recreate the hosted cluster and without needing the service provider to change a hardcoded value.

#### Story 4: Hosted Cluster Default Behavior
As a service provider operating a fleet of hosted control planes, I want hosted clusters that do not set the field to keep the existing 3-hour retention, so that upgrading the HyperShift operator does not change the behavior of any existing hosted cluster.

### API Extensions

This enhancement modifies two APIs:

- `operator.openshift.io/v1` `KubeAPIServer` (standalone OpenShift): adds a new `spec.eventTTLMinutes` field. See [openshift/api PR #2520](https://github.com/openshift/api/pull/2520).
- `hypershift.openshift.io/v1beta1` `HostedCluster` and `HostedControlPlane` (hosted control planes): adds a new `spec.operatorConfiguration.kubeAPIServer.eventTTLMinutes` field. The field is added to the shared `KubeAPIServerOperatorSpec` type, which is already embedded in both resources, so a single type change covers both.

Neither change adds a webhook, aggregated API server, or new CRD. Both are optional fields with CRD-level range validation.

### Workflow Description

#### Standalone OpenShift Workflow

The workflow for configuring event-ttl is straightforward:

1. **Cluster Administrator** accesses the OpenShift cluster via CLI or web console
2. **Cluster Administrator** edits the operator configuration resource
3. **Cluster Administrator** sets the `eventTTLMinutes` field to the desired value in minutes (e.g., 60, 180)
4. **kube-apiserver-operator** detects the configuration change
5. **kube-apiserver-operator** updates the kube-apiserver deployment with the new configuration
6. **kube-apiserver** restarts with the new event-ttl setting
7. **etcd** begins using the new event retention policy for future events

The configuration change takes effect immediately for new events, while existing events continue to use their original TTL until they expire.

#### Hosted Control Planes Workflow

1. **HostedCluster owner** edits the `HostedCluster` resource on the management cluster, setting `spec.operatorConfiguration.kubeAPIServer.eventTTLMinutes` to the desired value in minutes (e.g., 60)
2. The **management cluster kube-apiserver** rejects the request at admission if the value is outside the 5-180 range
3. The **HyperShift operator** copies `spec.operatorConfiguration` onto the corresponding `HostedControlPlane` resource
4. The **control-plane-operator** regenerates the kube-apiserver configuration for the hosted control plane with the new `--event-ttl` value and writes it to the kube-apiserver config `ConfigMap`
5. The **control-plane-operator** recomputes the pod template config hash, which triggers a rollout of the hosted kube-apiserver `Deployment`
6. The hosted **kube-apiserver** comes up with the new event-ttl setting and new events in the hosted etcd use the new retention policy

As with standalone, the change applies to events created after the rollout; events already stored keep the TTL they were created with until they expire.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Hosted control planes are explicitly in scope for this enhancement. Because there is no kube-apiserver operator in a hosted control plane, the setting is exposed as a field on the `HostedCluster` resource rather than on the `KubeAPIServer` operator resource, and is reconciled by the HyperShift operator and the control-plane-operator.
HyperShift continues to use the same 3-hour default as standalone OpenShift clusters unless explicitly configured otherwise.

An earlier iteration of this enhancement proposed an annotation (`hypershift.openshift.io/event-ttl-minutes`) instead, following the pattern used for `goaway-chance` in [openshift/hypershift#6019](https://github.com/openshift/hypershift/pull/6019).
That approach was prototyped in [openshift/hypershift#7202](https://github.com/openshift/hypershift/pull/7202) (closed without merging). This enhancement replaces it with a typed API field; see [Hosted Control Planes (HyperShift)](#hosted-control-planes-hypershift) for the full design and [Alternatives](#alternatives-not-implemented) for the rationale.

#### Standalone Clusters

This enhancement is fully applicable to standalone OpenShift clusters. The event-ttl configuration will be applied to the kube-apiserver running in the control plane, affecting event retention in the cluster's etcd.

#### Single-node Deployments or MicroShift

For single-node OpenShift (SNO) deployments, this enhancement will work as expected. The event-ttl configuration will be applied to the kube-apiserver running on the single node.

For MicroShift, this enhancement is not directly applicable as MicroShift uses a different architecture and may not have the same event-ttl configuration options. MicroShift also uses a 3-hour TTL by default, but since it doesn't use the kube-apiserver operator, the configuration approach described in this enhancement may not work. 

#### OpenShift Kubernetes Engine

OKE clusters run the same kube-apiserver and the same kube-apiserver-operator as standalone OpenShift, so the standalone behavior described here applies unchanged. No OKE-specific work is required.

### Implementation Details/Notes/Constraints

#### Standalone OpenShift

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

#### Hosted Control Planes (HyperShift)

A hosted control plane has no kube-apiserver operator, so there is no `operator.openshift.io/v1` `KubeAPIServer` resource to configure and the standalone design above does not apply. The hosted kube-apiserver's flags are rendered by the control-plane-operator (CPO) from the `HostedControlPlane` resource, and `--event-ttl` is currently a constant.

##### Proposed API

We propose to add `eventTTLMinutes` to the existing `KubeAPIServerOperatorSpec` type, which is reachable on the `HostedCluster` as `spec.operatorConfiguration.kubeAPIServer`:

```yaml
apiVersion: hypershift.openshift.io/v1beta1
kind: HostedCluster
metadata:
  name: example
  namespace: clusters
spec:
  operatorConfiguration:
    kubeAPIServer:
      eventTTLMinutes: 60
```

```go
// KubeAPIServerOperatorSpec specifies the configuration for the Kube API Server.
// +kubebuilder:validation:MinProperties=1
type KubeAPIServerOperatorSpec struct {
	ComponentLogLevelSpec `json:",inline"`

	// eventTTLMinutes specifies the amount of time, in minutes, that events are stored
	// in etcd before being deleted.
	// The TTL is allowed between 5 minutes minimum up to a maximum of 180 minutes (3 hours).
	//
	// Lowering this value will reduce the storage required in etcd. Note that this setting
	// will only apply to new events being created and will not update existing events.
	//
	// Changing this value causes the kube-apiserver to be rolled out with the new setting.
	//
	// When omitted this means no opinion, and the platform is left to choose a reasonable
	// default, which is subject to change over time.
	// The current default value is 180 minutes (3 hours).
	//
	// +kubebuilder:validation:Minimum=5
	// +kubebuilder:validation:Maximum=180
	// +optional
	EventTTLMinutes int32 `json:"eventTTLMinutes,omitempty"`
}
```

`KubeAPIServerOperatorSpec` is embedded in both `HostedCluster.spec.operatorConfiguration` and `HostedControlPlane.spec.operatorConfiguration`, so this single type change exposes the field on both resources. `HostedControlPlane` is an internal API; users configure the `HostedCluster`.

The exact feature gate used to introduce the field is deferred to API review; see [Open Questions](#open-questions). Note that `spec.operatorConfiguration.kubeAPIServer` itself is currently gated on the HyperShift `HCPUserFacingOperatorLogs` feature gate, which is enabled in both the HyperShift `Default` and `TechPreviewNoUpgrade` feature sets.

##### Rationale for location, type, units, defaulting, and validation

**Location.** `spec.operatorConfiguration` is the stanza HyperShift already uses for user-facing, per-control-plane-component settings, and `spec.operatorConfiguration.kubeAPIServer` already exists and already carries a kube-apiserver tuning knob (`logLevel`). Putting `eventTTLMinutes` there groups it with the component it configures and avoids inventing a new top-level field.
The two obvious alternatives were rejected:

- `spec.configuration.apiServer` is `config.openshift.io/v1` `APIServerSpec`, the same type the hosted cluster's own `APIServer` config object uses. It is a hosted-cluster-facing config API shared with standalone OpenShift, and event-ttl is not part of it. Adding a HyperShift-only field there is not possible without changing the shared OpenShift config API.
- A new top-level `HostedClusterSpec` field would be inconsistent with how other kube-apiserver settings are now modeled.

**Type and units.** `int32` minutes, matching the standalone `eventTTLMinutes` field exactly.
Keeping the type, the name, the units, and the bounds identical across the two topologies means a single documented range and a single mental model for users and for support, and it avoids a unit conversion that is only correct in one direction (`metav1.Duration` would accept `90s`, which has no representation in the standalone field).
`metav1.Duration` has precedent in the HyperShift API (`NodePool.spec.nodeDrainTimeout`) and remains an option if API review prefers it; this is listed under [Open Questions](#open-questions).

**Defaulting.** The field is `+optional` with no CRD-level default. An unset field means "no opinion" and the platform picks the default, consistent with the OpenShift API convention already used by the standalone field.
The effective default of 180 minutes (3 hours) is applied by the control-plane-operator when it renders the kube-apiserver arguments, exactly where the `3h` constant lives today.
Deliberately not setting a CRD default keeps the API honest about the platform owning the default, and means the default can be changed in a future release without having to migrate values that were written into every existing `HostedCluster` by the API server.

**Validation.** `Minimum=5` and `Maximum=180` are enforced by the CRD schema, so an out-of-range value is rejected by the management cluster's kube-apiserver at admission time, and the user gets an immediate error on `oc apply`.
A value of `0` is not distinguishable from unset for a non-pointer `int32` with `omitempty`, so it is treated as "unset" and the platform default applies; it cannot be used to disable event expiry. Because validation happens at admission, there is no code path in which an invalid value reaches the hosted kube-apiserver.

##### Why an API field rather than an annotation

An annotation-based design (`hypershift.openshift.io/event-ttl-minutes`) was proposed in the original version of this enhancement and prototyped in [openshift/hypershift#7202](https://github.com/openshift/hypershift/pull/7202), following the pattern used for `goaway-chance`. That PR was approved but closed without merging. We are not carrying that design forward, for the following reasons:

- **No validation.** The prototype read the annotation and did `fmt.Sprintf("%sm", value)` with no parsing or range check.
  An annotation value of `abc` would have produced `--event-ttl=abcm` and a kube-apiserver that fails to start; `0` would have produced `--event-ttl=0m`, which is exactly the "events are never deleted" case that the standalone design explicitly guards against. Annotations are untyped strings, so there is no way to get admission-time validation without adding a webhook.
- **Not discoverable.** Annotations do not appear in the CRD schema, so they are invisible to `oc explain`, to generated API documentation, and to UIs.
- **HyperShift is moving away from annotations for user-facing settings.** The `hypershift.openshift.io/kube-apiserver-verbosity-level` annotation has already been deprecated in favor of `spec.operatorConfiguration.kubeAPIServer.logLevel`, with the field taking precedence and the annotation slated for removal.
  Introducing a new annotation for a sibling setting on the same component would move against that direction and create a second deprecation to carry later.
- **Annotations are a weaker support contract.** HyperShift annotations are generally treated as unsupported or service-provider-level escape hatches; this feature is intended as a supported, customer-facing configuration option, which is what OCPSTRAT-3679 asks for.

The cost of the API field is that it requires openshift/hypershift API review and a feature gate, whereas an annotation would not. We consider that cost appropriate for a supported, customer-facing setting.

##### Propagation to the hosted kube-apiserver

The configured value reaches the hosted kube-apiserver through the existing `operatorConfiguration` path; no new controller or plumbing mechanism is introduced. The verified path in openshift/hypershift is:

1. The user sets `HostedCluster.spec.operatorConfiguration.kubeAPIServer.eventTTLMinutes`.
2. The HyperShift operator's `HostedCluster` controller deep-copies `spec.operatorConfiguration` onto `HostedControlPlane.spec.operatorConfiguration` (in `hypershift-operator/controllers/hostedcluster/hostedcluster_controller.go`). This is the same mechanism that already carries `kubeAPIServer.logLevel`, so nothing has to be added to the list of mirrored settings.
3. In the control-plane-operator, `kas.NewConfigParams(hcp, featureGates)` (`control-plane-operator/controllers/hostedcontrolplane/v2/kas/params.go`) resolves the effective value: if `hcp.Spec.OperatorConfiguration` is non-nil and `KubeAPIServer.EventTTLMinutes` is non-zero, use it; otherwise use the existing default.
   This is the same function that already resolves `goAwayChance`, `maxRequestsInflight`, and friends, and it is the natural place for the `3h` default constant to move to.
4. `generateConfig` (`control-plane-operator/controllers/hostedcontrolplane/v2/kas/config.go`) replaces the current `args.Set("event-ttl", "3h")` with the resolved value rendered as a Go duration string (e.g. `60m`), and the resulting `KubeAPIServerConfig` is written to the kube-apiserver config `ConfigMap` in the hosted control plane namespace.
5. The CPO v2 control-plane-component framework computes a composite hash over the component's mounted ConfigMaps and Secrets and records it on the pod template as `component.hypershift.openshift.io/config-hash` (`support/controlplane-component/defaults.go`). A change to the rendered config changes the hash, which changes the pod template, which causes the kube-apiserver `Deployment` to roll.

Because the field is resolved in `NewConfigParams` rather than at the `HostedCluster` admission layer, the hosted kube-apiserver always has an explicit `--event-ttl` value; there is no case in which the flag is omitted.

##### Mutability and rollout

The field is proposed as **mutable after cluster creation** (day-2), not day-1 only. Rationale: the standalone field is mutable; the sibling `kubeAPIServer.logLevel` field in the same stanza is mutable and is documented as triggering a rolling restart of the component; and the epic acceptance criteria require that changes take effect without recreating the cluster. No immutability CEL rule is proposed.

Rollout behavior follows from the propagation path above and is the same as for any other kube-apiserver configuration change in HyperShift:

- Updating the field triggers reconciliation of the `HostedCluster`, then of the `HostedControlPlane`, then of the kube-apiserver component. The change in the rendered config changes the pod template config hash and the kube-apiserver `Deployment` rolls.
- For `HighlyAvailable` control planes the kube-apiserver runs multiple replicas behind the control plane's service, so the rollout is a normal rolling update and the hosted cluster's API should remain available throughout, subject to the usual brief per-replica connection resets as pods are replaced.
- For `SingleReplica` control planes there is a single kube-apiserver pod, so the rollout causes a short window in which the hosted cluster's API is unavailable, the same as for any other kube-apiserver configuration change (for example, a `logLevel` change) in that topology.
- The hosted cluster's `Available` and `Progressing` conditions reflect the rollout; no new conditions are introduced.
- Convergence: once the new pods are ready, every subsequently created event carries the new TTL. Events already in etcd retain the TTL they were created with, so after lowering the value the storage reduction is only fully realized once the previously created events have expired (up to the old TTL, so at most 3 hours) and etcd has compacted and defragmented.
  Raising the value has no effect on already-stored events either; they still expire on their original schedule.

Operators should expect one kube-apiserver rollout per change to this field. It is therefore not a field to flip repeatedly; it is a configuration setting, and the enhancement does not propose any rate limiting or coalescing beyond what the existing reconciliation already provides.

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
**Mitigation**: The API validation and operator ensures the values are within a reasonable range and never zero (5-180 minutes). On the HyperShift side, `0` is indistinguishable from unset for an `omitempty` `int32`, so it is treated as "no opinion" and the platform default of 180 minutes is applied; the value never reaches `--event-ttl`.

**Risk** (HCP): Lowering the value shortens the debugging window for a hosted cluster, and the hosted cluster's own administrators may not know the control plane setting was changed, because they cannot see the `HostedCluster` resource.
**Mitigation**: The setting is documented as affecting event retention in the hosted cluster, and the configured value is visible on the hosted kube-apiserver's `--event-ttl` flag, which must-gather collects. Hosted cluster administrators who need longer retention should use an event exporter or logging pipeline rather than relying on event TTL.

**Risk** (HCP): Each change to the field triggers a kube-apiserver rollout. For `SingleReplica` control planes this causes a brief API outage.
**Mitigation**: This matches the behavior of every other kube-apiserver configuration change in HyperShift (for example `logLevel`), the field documentation states that changing it rolls the component, and the disruption is bounded by the normal kube-apiserver restart time.

**Risk** (HCP): Lowering the value does not immediately reduce etcd usage, which may be read as the setting not working.
**Mitigation**: Documented explicitly: existing events keep their original TTL, so the full effect is only visible after the old events expire (up to 3 hours) and etcd compacts and defragments.

### Drawbacks

- Adds complexity to the configuration API
- Additional validation and error handling required
- The same capability is now exposed through two different APIs, one per topology, which has to be kept consistent (units, range, default) as either side evolves

## Alternatives (Not Implemented)

1. **Hardcoded Values**: Keep the current fixed value of 3 hours
   - **Rejected**: Does not meet customer requirements for configurability

2. **Environment Variable**: Use environment variables instead of API configuration
   - **Rejected**: Less user-friendly and harder to manage

3. **Separate CRD**: Create a separate CRD for event configuration
   - **Rejected**: Overkill for a single setting, better to include in existing APIServer resource

4. **HyperShift annotation instead of an API field**: Expose the setting as `hypershift.openshift.io/event-ttl-minutes` on the `HostedCluster`, as prototyped in [openshift/hypershift#7202](https://github.com/openshift/hypershift/pull/7202)
   - **Rejected**: Annotations are untyped and cannot be validated at admission without a webhook, are not discoverable via `oc explain` or generated API docs, and carry a weaker support contract.
     It also runs counter to HyperShift's ongoing migration of kube-apiserver settings from annotations to `spec.operatorConfiguration.kubeAPIServer`; the verbosity-level annotation is already deprecated in favor of the `logLevel` field.
     See [Why an API field rather than an annotation](#why-an-api-field-rather-than-an-annotation).

5. **Reuse the standalone `KubeAPIServer` operator resource in hosted clusters**: Make the `operator.openshift.io/v1` `KubeAPIServer` resource available inside the hosted cluster and have something in the hosted control plane read it
   - **Rejected**: There is no kube-apiserver operator in a hosted control plane, and the hosted cluster's control plane configuration is deliberately owned by the `HostedCluster` resource on the management cluster, not by resources inside the hosted cluster. This would also let a hosted cluster administrator change a setting that affects management-cluster-hosted etcd.

6. **A top-level `HostedClusterSpec.eventTTLMinutes` field**: Add the field directly to `HostedClusterSpec` rather than under `spec.operatorConfiguration.kubeAPIServer`
   - **Rejected**: Inconsistent with how other per-component control plane settings are now modeled; `spec.operatorConfiguration.kubeAPIServer` already exists for exactly this purpose.

## Open Questions

These items are not settled and need a decision from API review and/or the HyperShift team before the HCP portion of this enhancement is implemented:

1. **Field type.** This enhancement proposes `int32` minutes named `eventTTLMinutes` for exact parity with the standalone field. `metav1.Duration` has precedent in the HyperShift API (`NodePool.spec.nodeDrainTimeout`) and is more expressive, but would diverge from standalone and would accept sub-minute values that the standalone field cannot represent. API review should confirm the choice.
2. **Feature gate.** HyperShift API fields are introduced behind a feature gate declared in `api/hypershift/v1beta1/featuregates`. The name of the gate for this field, and whether it starts in `TechPreviewNoUpgrade` only or is also enabled in the HyperShift `Default` feature set, has not been agreed.
   Note that the parent field `spec.operatorConfiguration.kubeAPIServer` is itself currently gated on `HCPUserFacingOperatorLogs`, a logs-specific gate name, so the team also needs to decide whether to nest a differently-gated field underneath it or to adjust the parent gating.
3. **Reviewers and approvers.** The `reviewers`/`approvers` metadata on this enhancement currently reflects the standalone kube-apiserver work only. HyperShift representation should be added before merge.
4. **Release target.** No release target is claimed for the HCP portion of this enhancement.

## Test Plan

The test plan will include:

1. **Unit Tests**: Test the API validation and parsing logic
2. **E2E Tests**: Test that the event TTL is properly configured on all apiserver after applying the setting
3. **Performance Tests**: Test the impact of different TTL values on etcd performance

### Hosted Control Planes Test Coverage

Test coverage for the HCP portion, matching the layers that already exist in openshift/hypershift:

1. **API validation (envtest / CRD schema tests)**
   - Values below 5 and above 180 are rejected by the API server at admission, on both create and update.
   - Boundary values 5 and 180 are accepted.
   - An omitted field is accepted and the stored object has no value for it.
   - Updating the field on an existing `HostedCluster` is accepted (the field is mutable).

2. **Defaulting and resolution (unit tests in `kas/params_test.go`)**
   - `HostedControlPlane` with no `operatorConfiguration` resolves to the 3-hour default.
   - `HostedControlPlane` with `operatorConfiguration` set but `eventTTLMinutes` unset (zero) resolves to the 3-hour default.
   - `HostedControlPlane` with `eventTTLMinutes: 60` resolves to `60m`.

3. **Propagation**
   - Unit test that the HyperShift operator copies `spec.operatorConfiguration` (including `eventTTLMinutes`) from `HostedCluster` to `HostedControlPlane`.
   - Unit test on `generateConfig` that the rendered kube-apiserver config contains `--event-ttl` with the resolved value, and that the default case still renders `3h`.

4. **Day-2 update and rollout (e2e)**
   - Create a hosted cluster without the field, assert the hosted kube-apiserver runs with `--event-ttl=3h`.
   - Update the `HostedCluster` to set `eventTTLMinutes`, assert the kube-apiserver `Deployment` rolls and that the new pods run with the new `--event-ttl`.
   - Assert the hosted cluster returns to `Available` after the rollout.
   - Assert that creating an event after the change results in it being removed at approximately the configured TTL (for a short TTL such as 5 minutes, this is feasible within an e2e run).

5. **Upgrade compatibility**
   - A hosted cluster created before the field existed, whose HyperShift operator and CPO are then upgraded to a version that has it, continues to run with `--event-ttl=3h` and does not experience a kube-apiserver rollout attributable to this change.

6. **Standalone regression**
   - The existing standalone kube-apiserver-operator unit and e2e tests for `eventTTLMinutes` continue to pass unchanged; the HCP work must not alter standalone behavior or defaults.

## Tech Preview

The EventTTL feature is controlled by the `EventTTL` feature gate, which is enabled by default in both DevPreview and TechPreview feature sets. This allows the feature to be available for testing and evaluation without requiring additional configuration.

The EventTTL feature gate is implemented in [openshift/api PR #2525](https://github.com/openshift/api/pull/2525) and will be removed when the feature graduates to GA, as the functionality will become a standard part of the platform.

The HyperShift API has its own feature gate sets (`api/hypershift/v1beta1/featuregates`), separate from the OpenShift `EventTTL` gate, so the `HostedCluster` field is introduced behind a HyperShift gate and graduates independently of the standalone field. The gate name and its initial feature set have not been agreed; see [Open Questions](#open-questions).

## Graduation Criteria

### Dev Preview -> Tech Preview

- API is implemented and validated
- Basic functionality works end-to-end
- Documentation is available
- Sufficient test coverage
- EventTTL feature gate is enabled in DevPreview and TechPreview feature sets

For hosted control planes:

- The `HostedCluster` field is implemented behind a HyperShift feature gate enabled in `TechPreviewNoUpgrade`
- Validation, defaulting, and propagation unit tests are in place
- An e2e test demonstrates a day-2 change propagating to the hosted kube-apiserver
- Upstream HyperShift documentation describes the field

### Tech Preview -> GA

- More comprehensive testing (upgrade, downgrade, scale)
- Performance testing with various TTL values
- User feedback incorporated
- Documentation updated in openshift-docs
- EventTTL feature gate is removed as the feature becomes GA

For hosted control planes:

- Upgrade compatibility verified: hosted clusters created before the field existed keep 3-hour retention after the control plane is upgraded
- Rollout behavior verified for both `SingleReplica` and `HighlyAvailable` control planes
- The HyperShift feature gate is enabled by default and then removed as the field becomes GA

### Removing a deprecated feature

This enhancement does not remove any existing features. It only adds new configuration options while maintaining backward compatibility with the existing default behavior.

## Upgrade / Downgrade Strategy

### Upgrade Strategy

- Existing clusters will continue to use the default 3-hour (180-minute) TTL
- No changes required for existing clusters
- New configuration option is available immediately

For hosted control planes:

- Existing `HostedCluster` resources predate the field, so it is absent from their spec. Absent means "no opinion", which the control-plane-operator resolves to the same 180-minute value that is hardcoded today.
  Upgrading the HyperShift operator and control-plane-operator therefore produces a byte-identical `--event-ttl=3h` for every existing hosted cluster, and no kube-apiserver rollout is triggered by this change alone.
- Because the field has no CRD-level default, the API server does not write a value into existing `HostedCluster` objects when the new CRD is applied, so no stored object is modified by the upgrade.
- A hosted cluster upgraded from a version predating the field can opt in at any time after the control plane components support it; the first time the field is set, the kube-apiserver rolls as described in [Mutability and rollout](#mutability-and-rollout).

### Downgrade Strategy

- Configuration will be ignored by older versions
- No impact on cluster functionality
- Events will continue to use the default TTL (180 minutes)

For hosted control planes, if the field is gated and the gate is disabled, or the HyperShift CRDs are rolled back to a version without the field, the value is pruned from the stored object and the control-plane-operator falls back to the 180-minute default on the next reconcile, rolling the kube-apiserver back to `--event-ttl=3h`.
Users who had configured a value will silently lose it, which is the normal consequence of removing an optional field and is why the field is introduced behind a feature gate.

## Version Skew Strategy

- The event-ttl setting is a kube-apiserver configuration
- No coordination required with other components
- Version skew is not a concern for standalone OpenShift

For hosted control planes there is a management/hosted split and a HyperShift-operator/control-plane-operator split to consider:

- There is no skew with the hosted cluster. `--event-ttl` is a kube-apiserver flag only; nothing inside the hosted cluster reads or depends on the configured value.
- Skew between the HyperShift operator and the control-plane-operator is possible during an upgrade. If the HyperShift operator mirrors `operatorConfiguration` containing `eventTTLMinutes` onto a `HostedControlPlane` that is reconciled by an older control-plane-operator, the older CPO simply does not read the field and continues to render `3h`.
  The value is preserved in the spec and takes effect once the CPO is upgraded. This is a no-op, not an error, because the field travels inside the already-mirrored `operatorConfiguration` struct.
- If the CRDs are upgraded before the HyperShift operator, the field can be set but is not yet mirrored or consumed; again a no-op until the operator is upgraded.
- The feature gate is evaluated where the CRDs are served, on the management cluster, so a gate that is off prevents the field from being set at all rather than producing partial behavior.

## Operational Aspects of API Extensions

This enhancement modifies the operator API but does not add new API extensions. The impact is limited to:

- Configuration validation in the kube-apiserver-operator
- Application of the setting to kube-apiserver deployment
- No impact on API availability or performance

For hosted control planes:

- No webhook, aggregated API server, or new CRD is added; the change is one optional field with CRD schema range validation on an existing CRD, so there is no added admission latency and no new failure mode for the management cluster's API server.
- The field has an operational effect on the management cluster rather than the hosted cluster: lowering it reduces the hosted etcd's event storage, memory, and defragmentation work, which in a dense management cluster is the main benefit. Raising it has no effect, since 180 minutes is already the maximum and the default.
- Setting the field causes a kube-apiserver rollout in the hosted control plane. The SLO impact is the same as any other kube-apiserver configuration change: none for `HighlyAvailable` topologies beyond transient connection resets, and a short API outage for `SingleReplica` topologies.

## Support Procedures

### Detection

- Configuration can be verified by checking the operator configuration resource
- kube-apiserver logs will show the configured event-ttl value
- etcd metrics can be monitored for compaction frequency

For hosted control planes:

- The intended value is on the `HostedCluster`: `oc get hostedcluster -n <ns> <name> -o jsonpath='{.spec.operatorConfiguration.kubeAPIServer.eventTTLMinutes}'`
- The mirrored value is on the `HostedControlPlane` in the hosted control plane namespace, which tells you whether the HyperShift operator has propagated it
- The effective value is the `--event-ttl` argument in the kube-apiserver config `ConfigMap` in the hosted control plane namespace, and on the running kube-apiserver pods
- HyperShift must-gather collects the `HostedCluster`, the `HostedControlPlane`, and the hosted control plane namespace's ConfigMaps and pod specs, so all three are available from an existing must-gather

### Troubleshooting

- If events are not being deleted as expected, check the event-ttl configuration
- Monitor etcd compaction metrics for unusual patterns

For hosted control planes:

- If the value on the `HostedCluster` is not reflected on the `HostedControlPlane`, the HyperShift operator is not reconciling; check whether the `HostedCluster` is paused (`spec.pausedUntil`) and check the HyperShift operator logs
- If the value is on the `HostedControlPlane` but not in the kube-apiserver config, the control-plane-operator is older than the field or is not reconciling; check the CPO version and logs
- If the value is in the config but the running pods still show the old flag, the kube-apiserver rollout has not completed; check the `Deployment` rollout status and the hosted cluster's `Progressing` condition
- If etcd usage has not dropped after lowering the value, this is expected until the previously created events expire under their original TTL (up to 3 hours) and etcd compacts and defragments

## Implementation History

- 2025-10-08: Initial enhancement proposal
- 2026-10-09: Extended to cover hosted control planes (HyperShift), replacing the previously proposed annotation-based approach with a `HostedCluster` API field

