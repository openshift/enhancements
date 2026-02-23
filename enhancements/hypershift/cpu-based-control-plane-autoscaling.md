---
title: cpu-based-control-plane-autoscaling
authors:
  - "@csrwng"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-02-23
last-updated: 2026-02-23
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-2915
see-also:
  - "/enhancements/hypershift/monitoring.md"
---

# CPU-Based Control Plane Autoscaling and Per-Size
# Resource Fractions

## Summary

This enhancement extends the existing resource-based control plane
autoscaling in HyperShift to consider CPU usage in addition to memory
usage when determining hosted cluster size. It also introduces the
ability to specify per-size resource fractions, allowing management
cluster administrators to configure what fraction of a machine's
memory and CPU is available for the kube-apiserver container on a
per-size basis, rather than using a single global fraction for all
sizes.

## Motivation

The current resource-based control plane autoscaler
([PR #6102](https://github.com/openshift/hypershift/pull/6102))
determines hosted cluster size based solely on the kube-apiserver's
memory usage as reported by the Vertical Pod Autoscaler (VPA). While
memory is often the dominant resource constraint, there are workloads
and configurations where CPU becomes the bottleneck before memory
does. In these cases, the autoscaler may assign a cluster to a size
that has sufficient memory but insufficient CPU, leading to
performance degradation.

Additionally, the current implementation uses a single global
fraction (`kubeAPIServerMemoryFraction`, defaulting to 0.65) to
determine how much of a machine's memory is available for the
kube-apiserver container. In practice, different machine sizes may
have different ratios of resources consumed by system components
versus the kube-apiserver. For example, a small machine may have a
larger proportion of its resources consumed by fixed-overhead system
components compared to a large machine. The ability to specify
per-size fractions allows administrators to model this more
accurately.

### User Stories

- As a **management cluster administrator**, I want the control plane
  autoscaler to consider both CPU and memory usage when sizing hosted
  clusters, so that clusters are not assigned to a size with
  insufficient CPU capacity even when memory is within bounds.

- As a **management cluster administrator**, I want to specify
  different resource fractions for different cluster sizes, so that I
  can accurately model the varying overhead ratios across machine
  types (e.g., a small machine dedicates a different proportion of
  resources to system components than a large machine).

- As an **SRE**, I want the autoscaler to prevent situations where a
  hosted cluster is under-provisioned for CPU, so that I can avoid
  manual intervention to resize clusters experiencing CPU-bound
  performance degradation.

- As an **SRE**, I want to monitor the autoscaler's CPU-based sizing
  decisions through existing logging and metrics, so that I can
  understand and troubleshoot sizing behavior at scale.

### Goals

- Extend the resource-based control plane autoscaler to read CPU
  recommendations from the VPA in addition to memory recommendations.
- Select the hosted cluster size as the smallest size class that can
  accommodate both the CPU and memory recommendations (i.e., the
  resource requiring the larger size class drives the decision).
- Allow administrators to configure a global CPU fraction analogous
  to the existing global memory fraction.
- Allow administrators to override both the memory fraction and CPU
  fraction on a per-size basis in the `ClusterSizingConfiguration`.
- Maintain backward compatibility: clusters using only memory-based
  autoscaling continue to work without configuration changes.

### Non-Goals

- Autoscaling based on resources other than kube-apiserver CPU and
  memory (e.g., etcd, network I/O).
- Changing how VPA recommendations are generated or configuring VPA
  policies.
- Modifying the size transition logic (concurrency limits, transition
  delays) which is handled by the existing `hostedclustersizing`
  controller.

## Proposal

The proposal modifies the `ClusterSizingConfiguration` API and the
`ResourceBasedControlPlaneAutoscaler` controller to support CPU-based
sizing and per-size resource fractions.

### Workflow Description

**Management cluster administrator** configures the
`ClusterSizingConfiguration` with optional CPU fractions and
per-size overrides.

**ResourceBasedControlPlaneAutoscaler controller** determines cluster
size based on both CPU and memory VPA recommendations.

1. The management cluster administrator creates or updates the
   `ClusterSizingConfiguration` resource named `cluster`. They may
   optionally configure:
   - A global `kubeAPIServerCPUFraction` in the
     `resourceBasedAutoscaling` section.
   - Per-size `kubeAPIServerMemoryFraction` and/or
     `kubeAPIServerCPUFraction` overrides in each size's `capacity`
     section.

2. A HostedCluster with the
   `hypershift.openshift.io/resource-based-cp-auto-scaling: "true"`
   annotation and dedicated request serving topology is reconciled
   by the `ResourceBasedControlPlaneAutoscaler` controller.

3. The controller reads the VPA recommendation for the
   kube-apiserver container, extracting both the memory and CPU
   `UncappedTarget` values.

4. The controller determines the recommended size independently
   for each resource dimension. Size classes are ordered from
   smallest to largest; memory and CPU capacity are assumed to
   be consistently ordered across sizes (i.e., a size with more
   memory also has more CPU).

   For each resource, the effective capacity for a given size is:
   - `effectiveMemory = size.capacity.memory *
     effectiveMemoryFraction(size)`
   - `effectiveCPU = size.capacity.cpu *
     effectiveCPUFraction(size)`

   Where `effectiveMemoryFraction` returns the per-size memory
   fraction if set, otherwise the global memory fraction, otherwise
   the default (0.65). The same precedence applies to
   `effectiveCPUFraction`.

   The controller computes two independent size recommendations:
   - **Memory-based size**: the smallest size where
     `effectiveMemory >= VPA recommended memory`
   - **CPU-based size**: the smallest size where
     `effectiveCPU >= VPA recommended CPU`

5. The final recommended size is the **maximum** of the
   memory-based size and the CPU-based size. Since sizes are
   consistently ordered, the larger of the two is guaranteed to
   satisfy both resource constraints.

   If only memory recommendations are available (CPU
   recommendation is absent), the controller falls back to
   memory-only sizing, preserving current behavior. If only CPU
   recommendations are available, the controller uses CPU-only
   sizing.

6. The controller sets the
   `hypershift.openshift.io/recommended-cluster-size` annotation
   on the HostedCluster with the selected size, as it does today.

7. The existing `hostedclustersizing` controller processes the
   annotation and applies the size transition with the configured
   delays and concurrency limits.

### API Extensions

This enhancement modifies the existing `ClusterSizingConfiguration`
CRD (`scheduling.hypershift.openshift.io/v1alpha1`). No new CRDs,
webhooks, finalizers, or aggregated API servers are introduced.

#### Proposed API Changes

**ResourceBasedAutoscalingConfiguration** -- add a global CPU
fraction:

```go
type ResourceBasedAutoscalingConfiguration struct {
    // kubeAPIServerMemoryFraction is a number between 0 and 1
    // that determines how much of a machine's total memory is
    // available for the Kube API server container. This fraction
    // is used to determine whether a Kube API server container
    // can fit within a particular cluster size. If not specified,
    // a default fraction of 0.65 is used.
    // +optional
    KubeAPIServerMemoryFraction *resource.Quantity
        `json:"kubeAPIServerMemoryFraction,omitempty"`

    // kubeAPIServerCPUFraction is a number between 0 and 1
    // that determines how much of a machine's total CPU is
    // available for the Kube API server container. This fraction
    // is used to determine whether a Kube API server container
    // can fit within a particular cluster size. If not specified,
    // a default fraction of 0.65 is used.
    // +optional
    KubeAPIServerCPUFraction *resource.Quantity
        `json:"kubeAPIServerCPUFraction,omitempty"`
}
```

**SizeCapacity** -- add per-size fraction overrides:

```go
type SizeCapacity struct {
    // memory is the amount of memory available at a specific size
    // +optional
    Memory *resource.Quantity `json:"memory,omitempty"`

    // cpu is the amount of CPU available for a specific size
    // +optional
    CPU *resource.Quantity `json:"cpu,omitempty"`

    // kubeAPIServerMemoryFraction is a number between 0 and 1
    // that overrides the global kubeAPIServerMemoryFraction for
    // this specific size. If not specified, the global fraction
    // (or its default) is used.
    // +optional
    KubeAPIServerMemoryFraction *resource.Quantity
        `json:"kubeAPIServerMemoryFraction,omitempty"`

    // kubeAPIServerCPUFraction is a number between 0 and 1
    // that overrides the global kubeAPIServerCPUFraction for
    // this specific size. If not specified, the global fraction
    // (or its default) is used.
    // +optional
    KubeAPIServerCPUFraction *resource.Quantity
        `json:"kubeAPIServerCPUFraction,omitempty"`
}
```

#### Example Configuration

```yaml
apiVersion: scheduling.hypershift.openshift.io/v1alpha1
kind: ClusterSizingConfiguration
metadata:
  name: cluster
spec:
  resourceBasedAutoscaling:
    kubeAPIServerMemoryFraction: "0.65"
    kubeAPIServerCPUFraction: "0.50"
  sizes:
    - name: small
      criteria:
        from: 0
        to: 10
      capacity:
        memory: 32Gi
        cpu: "8"
        # Small machines have higher fixed overhead, so
        # less is available for kube-apiserver
        kubeAPIServerMemoryFraction: "0.55"
        kubeAPIServerCPUFraction: "0.40"
    - name: medium
      criteria:
        from: 11
        to: 100
      capacity:
        memory: 64Gi
        cpu: "16"
        # Uses global fractions (0.65 memory, 0.50 cpu)
    - name: large
      criteria:
        from: 101
      capacity:
        memory: 128Gi
        cpu: "32"
        # Large machines have lower relative overhead
        kubeAPIServerMemoryFraction: "0.75"
        kubeAPIServerCPUFraction: "0.60"
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is exclusively for the HyperShift topology. It
modifies the `ResourceBasedControlPlaneAutoscaler` controller which
runs in the HyperShift Operator on the management cluster. The
changes affect how hosted cluster control plane sizes are determined
but do not change the control plane components themselves.

The `ClusterSizingConfiguration` CRD is a management cluster
resource. Changes to this CRD affect all hosted clusters on that
management cluster that have resource-based autoscaling enabled.

#### Standalone Clusters

Not applicable. The resource-based control plane autoscaler is
specific to HyperShift.

#### Single-node Deployments or MicroShift

Not applicable. This enhancement is specific to the HyperShift
topology.

#### OpenShift Kubernetes Engine

This enhancement does not depend on features excluded from OKE.

### Implementation Details/Notes/Constraints

#### Controller Changes

The primary changes are in the
`ResourceBasedControlPlaneAutoscaler` controller and its
`machineSizesCache`:

1. **VPA CPU recommendation extraction**: The
   `recommendedClusterSize` function currently extracts only the
   memory recommendation from the VPA. It must be extended to also
   extract the CPU recommendation from the kube-apiserver
   container's `UncappedTarget`.

2. **Size cache updates**: The `machineSizesCache` must track
   per-size fractions in addition to the global fractions. A new
   `recommendedSizeByCPU` method (analogous to the existing
   `recommendedSize` for memory) should be added. The controller
   calls both methods independently and takes the maximum of the
   two results.

3. **Fraction resolution**: Implement a fraction resolution
   function that returns the effective fraction for a given size
   and resource type, following the precedence:
   per-size fraction > global fraction > default (0.65).

4. **Sizing algorithm**: The current algorithm iterates sizes in
   ascending memory order and returns the first size where
   `capacity.memory * fraction >= recommended_memory`. The new
   algorithm runs the same logic independently for each resource:

   - Compute `memorySize` = smallest size where
     `capacity.memory * memoryFraction >= recommended_memory`
   - Compute `cpuSize` = smallest size where
     `capacity.cpu * cpuFraction >= recommended_cpu`
   - Return `max(memorySize, cpuSize)`

   Because memory and CPU capacity are consistently ordered
   across sizes (a size with more memory also has more CPU),
   the larger of the two sizes is guaranteed to satisfy both
   resource constraints. This avoids the need for a combined
   two-dimensional search.

5. **Backward compatibility**: If no CPU recommendation is
   available from the VPA (e.g., the VPA has not yet generated a
   CPU recommendation), the controller must fall back to
   memory-only sizing. Similarly, if no CPU capacity is configured
   for sizes, CPU-based sizing is skipped.

6. **Logging**: Extend the existing log messages to include CPU
   recommendation values and the effective CPU fraction alongside
   the existing memory information.

#### CRD Schema Updates

The `ClusterSizingConfiguration` CRD schema must be updated to
include the new fields. The CRD YAML at
`cmd/install/assets/hypershift-operator/scheduling.hypershift.openshift.io_clustersizingconfigurations.yaml`
must be regenerated.

#### Validation

- `kubeAPIServerCPUFraction` (both global and per-size) must be
  validated to be between 0 (exclusive) and 1 (inclusive), matching
  the existing validation for `kubeAPIServerMemoryFraction`.
- Per-size fractions in `SizeCapacity` follow the same validation.

<!-- TODO: Per dev-guide/feature-zero-to-hero.md, all new features
must be gated behind a feature gate in
https://github.com/openshift/api/blob/master/features/features.go.
Determine whether this enhancement requires a feature gate or
whether it can be delivered as an incremental improvement to the
existing resource-based autoscaling feature which is already
annotation-gated. -->

### Risks and Mitigations

**Risk**: CPU-based sizing could cause more frequent size
transitions if CPU usage is more volatile than memory usage.

**Mitigation**: The existing transition delay and concurrency
controls in the `hostedclustersizing` controller apply equally to
CPU-driven size changes. Administrators can tune
`transitionDelay.increase` and `transitionDelay.decrease` to
dampen oscillations. Additionally, VPA recommendations are
smoothed over time by the VPA recommender, which reduces
sensitivity to short-term CPU spikes.

**Risk**: Per-size fractions increase configuration complexity and
the chance of misconfiguration.

**Mitigation**: Per-size fractions are optional. The default
behavior (using global fractions) remains simple. Validation
ensures fractions are within valid bounds. Documentation should
provide guidance on when per-size fractions are beneficial.

### Drawbacks

- **Increased configuration surface**: Adding per-size fractions
  for two resource types (memory and CPU) significantly expands the
  configuration surface of `ClusterSizingConfiguration`. This
  makes the resource harder to understand and increases the
  likelihood of misconfiguration.

- **VPA CPU recommendation quality**: VPA CPU recommendations may
  be less stable than memory recommendations for some workloads,
  potentially leading to more sizing oscillations. The interaction
  between VPA smoothing and the autoscaler's transition delays
  needs careful tuning.

## Alternatives (Not Implemented)

### Configurable resource selection (memory, CPU, or max)

Instead of always using the maximum of CPU and memory
recommendations, an alternative would allow administrators to
choose which resource drives the sizing decision per cluster
or globally. This was considered but rejected because:

- The "max of both" approach is the safest default -- it prevents
  under-provisioning on either dimension.
- Adding a resource selection mode would increase configuration
  complexity without clear benefit, since administrators who
  do not care about CPU can simply not configure CPU capacity or
  CPU fractions, and the controller will fall back to memory-only
  sizing.

### Separate VPAs for CPU and memory

Instead of reading both CPU and memory from a single VPA, create
separate VPA resources targeting different aspects of the
kube-apiserver. This was rejected because:

- VPA recommendations for all resource types are provided in a
  single VPA status, so separate VPAs would be redundant.
- Managing multiple VPAs per hosted cluster would increase
  operational complexity.

### Weighted combination of CPU and memory

Use a weighted formula (e.g., `0.7 * normalized_memory + 0.3 *
normalized_cpu`) to produce a single sizing score. This was
rejected because:

- The weighting would be arbitrary and difficult to reason about.
- The "max of both" approach is simpler, more predictable, and
  avoids under-provisioning on either resource.

## Open Questions

1. What should the default CPU fraction be? The current memory
   fraction default is 0.65. Should the CPU fraction default also
   be 0.65, or should a different value be used based on observed
   kube-apiserver CPU consumption patterns?

2. Does this enhancement require its own feature gate, or can it
   be delivered as an incremental improvement to the existing
   resource-based autoscaling feature (which is already gated
   behind the
   `hypershift.openshift.io/resource-based-cp-auto-scaling`
   annotation)?

3. When introspecting MachineSets (the fallback path when
   `capacity` is not set in the config), the controller already
   reads both `machine.openshift.io/memoryMb` and
   `machine.openshift.io/vCPU` annotations. Should the MachineSet
   introspection path also support per-size fractions, or should
   per-size fractions only be available when capacity is
   explicitly configured?

## Test Plan

<!-- TODO: Add test labels per dev-guide/feature-zero-to-hero.md:
Tests must include `[OCPFeatureGate:FeatureName]` label for the
feature gate, `[Jira:"Component Name"]` for the component, and
appropriate test type labels. Reference
dev-guide/test-conventions.md for details. -->

- **Unit tests**: Extend `machine_sizes_cache_test.go` to cover:
  - Size selection with both CPU and memory recommendations
    present.
  - Size selection with only CPU recommendation (memory absent).
  - Size selection with only memory recommendation (CPU absent),
    verifying backward compatibility.
  - Per-size fraction overrides taking precedence over global
    fractions.
  - Global CPU fraction with no per-size override.
  - Default fraction behavior when no fractions are configured.
  - Validation of fraction values (out of range, zero, negative).

- **Unit tests**: Extend `controller_test.go` to cover:
  - Controller extracting CPU recommendation from VPA status.
  - Controller behavior when VPA provides only memory
    recommendations (backward compatibility).
  - Correct annotation update when CPU drives the sizing decision
    vs. when memory drives the decision.

- **Integration tests**: Verify that the CRD schema accepts the
  new fields and that the controller correctly reads them from the
  `ClusterSizingConfiguration`.

- **E2E tests**: Extend `cp_autoscaling_test.go` to verify:
  - A HostedCluster is sized correctly when CPU usage is the
    dominant constraint.
  - Per-size fractions are respected in sizing decisions.

## Graduation Criteria

<!-- TODO: Define graduation milestones per
dev-guide/feature-zero-to-hero.md. Minimum requirements include:
5 tests, 7 runs per week, 14 runs per supported platform, 95%
pass rate, and tests running on all supported platforms. -->

### Dev Preview -> Tech Preview

- End-to-end functionality for CPU-based sizing.
- Per-size fraction configuration working.
- Backward compatibility with memory-only configurations
  verified.
- Unit and integration test coverage.
- User-facing documentation for the new configuration fields.

### Tech Preview -> GA

- Extended testing (upgrade, downgrade, scale).
- Load testing with varied CPU/memory workload profiles.
- Sufficient feedback period from managed service operators.
- Documentation updated in openshift-docs.

### Removing a deprecated feature

N/A. This is an extension of an existing feature.

## Upgrade / Downgrade Strategy

**Upgrade**: The new fields (`kubeAPIServerCPUFraction` in
`ResourceBasedAutoscalingConfiguration`, and per-size fraction
fields in `SizeCapacity`) are optional. Existing
`ClusterSizingConfiguration` resources continue to work without
modification. The autoscaler falls back to memory-only sizing when
CPU capacity or CPU fractions are not configured.

No manual action is required on upgrade. The autoscaler begins
considering CPU recommendations automatically when the controller
is updated, but since no CPU fractions or CPU capacity are
configured by default, the behavior remains memory-only until an
administrator explicitly configures CPU-related fields.

**Downgrade**: If the controller is downgraded to a version that
does not support CPU-based sizing, the new fields in the
`ClusterSizingConfiguration` are ignored (they are optional and
have no `kubebuilder:default`). The autoscaler reverts to
memory-only sizing. No manual cleanup is required, though the
unused fields remain in the CRD until the CRD schema is
downgraded.

## Version Skew Strategy

The `ResourceBasedControlPlaneAutoscaler` controller and the
`ClusterSizingConfiguration` CRD are both managed by the
HyperShift Operator. They are upgraded together, so there is no
version skew between the controller and the API it consumes.

During an upgrade, there may be a brief period where the old
controller is running with the new CRD schema (or vice versa).
Since all new fields are optional and the old controller ignores
unknown fields, this does not cause errors. The sizing behavior
during this window is memory-only, matching the pre-upgrade
behavior.

## Operational Aspects of API Extensions

This enhancement modifies an existing CRD
(`ClusterSizingConfiguration`) but does not introduce new API
extensions (no new CRDs, webhooks, finalizers, or aggregated
API servers).

The existing `ClusterSizingConfigurationValid` status condition
on the `ClusterSizingConfiguration` resource should be extended
to validate the new fraction fields. Invalid fractions (outside
the 0-1 range) will cause the condition to report an error.

## Support Procedures

- **Detecting CPU-based sizing issues**: If a hosted cluster
  appears undersized despite high CPU usage, check:
  - The VPA recommendation for the kube-apiserver container
    includes a CPU recommendation: `oc get vpa -n <hcp-namespace>
    -o yaml` and look for the CPU field in
    `status.recommendation.containerRecommendations`.
  - The `ClusterSizingConfiguration` includes CPU capacity for
    each size: `oc get clustersizingconfiguration cluster -o yaml`
    and verify `spec.sizes[*].capacity.cpu` is set.
  - The controller logs show CPU-related sizing decisions:
    filter for the `ResourceBasedControlPlaneAutoscaler` controller
    name and look for CPU values in the log messages.

- **Disabling CPU-based sizing**: To revert to memory-only
  sizing, remove the `kubeAPIServerCPUFraction` field from the
  `ClusterSizingConfiguration` spec and remove any per-size CPU
  fraction overrides. The controller will fall back to memory-only
  sizing on its next reconciliation.

- **Impact of misconfigured fractions**: If per-size fractions are
  set too high (close to 1.0), the autoscaler may assign clusters
  to sizes that are too small in practice, leading to resource
  pressure. If set too low (close to 0.0), the autoscaler will
  assign larger sizes than necessary, wasting resources. In either
  case, the configuration can be corrected by updating the
  `ClusterSizingConfiguration`, and the autoscaler will
  re-evaluate cluster sizes on its next reconciliation.
