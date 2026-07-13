---
title: gomaxprocs-injection
authors:
  - "@pehunt"
reviewers:
  - "@yuqi-zhang"
  - "@harche"
approvers:
  - "@mrunalp"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-06-24
last-updated: 2026-07-07
tracking-link:
  - https://redhat.atlassian.net/browse/OCPNODE-4600
---

# GOMAXPROCS Configuration for Containers and System Services

## Summary

This enhancement enables automatic GOMAXPROCS configuration at two levels:

1. **Container level**: CRI-O automatically injects the GOMAXPROCS environment variable into containers based on their CPU resource requests, allowing Go applications to optimize their runtime parallelism for the allocated CPU resources rather than the total node capacity.

2. **System service level**: The kubelet-auto-node-size service automatically configures GOMAXPROCS for kubelet and CRI-O processes based on system reserved CPU, ensuring these critical Go-based system services also benefit from optimized parallelism.

## Motivation

Go applications default GOMAXPROCS to the number of CPUs visible to the process, which in containerized environments is the entire node's CPU count. This causes inefficient resource usage when containers have CPU requests smaller than the node capacity. Applications may spawn excessive goroutines and experience unnecessary context switching, degrading performance and increasing memory overhead.

**Note**: Go 1.25+ automatically derives GOMAXPROCS from the cgroup CPU quota when a limit is set, addressing the problem for limited containers. This enhancement targets burstable workloads with CPU requests but no limits, where the application benefits from parallelism beyond the request when extra CPU capacity is available.

### User Stories

As a cluster administrator, I want Go applications running in containers to automatically use an appropriate number of OS threads based on their CPU allocation, so that they perform efficiently without manual configuration.

As a developer deploying Go applications, I want GOMAXPROCS to be set proportionally to my container's CPU request, so that my application's parallelism matches its resource allocation without requiring code changes or custom entrypoint scripts.

As a cluster administrator, I want kubelet and CRI-O to automatically optimize their GOMAXPROCS based on the system reserved CPU allocation, so that these critical system services perform efficiently regardless of node size, especially when using auto node sizing.

As a platform engineer, I want to enable auto node sizing and have GOMAXPROCS automatically configured for both system services and workload containers, so that the entire Go runtime stack is optimized without manual tuning.

### Goals

- Add a field to the ContainerRuntimeConfig API to control GOMAXPROCS injection behavior for containers
- Enable CRI-O to automatically set GOMAXPROCS based on container CPU requests
- Add a field to the KubeletConfig API to enable GOMAXPROCS configuration for kubelet and CRI-O system services
- Integrate with auto node sizing to set GOMAXPROCS for system services based on system reserved CPU
- Provide a cluster-wide default behavior that can be overridden per machine config pool

### Non-Goals

- Language detection or runtime-specific injection (CRI-O injects GOMAXPROCS for eligible containers - burstable and best-effort pods without CPU limits; non-Go applications ignore it)
- Dynamic adjustment of GOMAXPROCS based on runtime CPU throttling or actual usage
- Per-pod or per-container override mechanisms beyond the existing `skip-gomaxprocs.crio.io` annotation (this is cluster-level configuration; users can override via pod spec environment variables or the skip annotation)
- Setting GOMAXPROCS for other system services beyond kubelet and CRI-O (these are the only Go-based system services that benefit from this optimization)

## Proposal

### Workflow Description

#### Container GOMAXPROCS Injection

1. The cluster administrator creates or updates a ContainerRuntimeConfig:

    ```yaml
    apiVersion: machineconfiguration.openshift.io/v1
    kind: ContainerRuntimeConfig
    metadata:
      name: enable-gomaxprocs
    spec:
      machineConfigPoolSelector:
        matchLabels:
          pools.operator.machineconfiguration.openshift.io/worker: ''
      containerRuntimeConfig:
        containerGomaxprocsBehavior: Autosize
    ```

2. The Machine Config Operator renders the CRI-O configuration setting `min_injected_gomaxprocs = 1`

3. CRI-O calculates GOMAXPROCS for each container as `max(ceil(cpu_request_in_cores * 2), 1)` and injects it as an environment variable before the container starts

4. Go applications in containers automatically detect and use the injected GOMAXPROCS environment variable

#### System Service GOMAXPROCS Configuration

1. The cluster administrator creates or updates a KubeletConfig to enable GOMAXPROCS for kubelet and CRI-O:

    ```yaml
    apiVersion: machineconfiguration.openshift.io/v1
    kind: KubeletConfig
    metadata:
      name: enable-system-gomaxprocs
    spec:
      autoSizingReserved: true
      systemGomaxprocsBehavior: Autosize
      machineConfigPoolSelector:
        matchLabels:
          pools.operator.machineconfiguration.openshift.io/worker: ''
    ```

2. The Machine Config Operator generates systemd drop-ins and scripts to:
   - Calculate system reserved (if `autoSizingReserved: true`)
   - Calculate GOMAXPROCS based on system reserved CPU
   - Write both to `/etc/node-sizing.env`

3. The kubelet-auto-node-size service runs before kubelet and CRI-O start, writing (example for 8 CPU node):
   ```bash
   SYSTEM_RESERVED_MEMORY=3.5Gi
   SYSTEM_RESERVED_CPU=1.5
   GOMAXPROCS=2
   ```
   (GOMAXPROCS = ceil(1.5) = 2)

4. Both kubelet.service and crio.service source `/etc/node-sizing.env` and set `GOMAXPROCS` environment variable before starting

**Note**: `systemGomaxprocsBehavior: Autosize` can be enabled independently of `autoSizingReserved`. If auto node sizing is disabled, GOMAXPROCS is still calculated based on the static system reserved values from `/etc/node-sizing-enabled.env`.

#### Combined Example: Full Stack GOMAXPROCS Configuration

To enable GOMAXPROCS optimization for both system services and containerized workloads:

```yaml
---
# Enable auto node sizing + GOMAXPROCS for kubelet and CRI-O
apiVersion: machineconfiguration.openshift.io/v1
kind: KubeletConfig
metadata:
  name: enable-full-gomaxprocs
spec:
  autoSizingReserved: true              # Calculate system reserved dynamically
  systemGomaxprocsBehavior: Autosize    # Set GOMAXPROCS for kubelet/CRI-O
  machineConfigPoolSelector:
    matchLabels:
      pools.operator.machineconfiguration.openshift.io/worker: ''
---
# Enable GOMAXPROCS injection for containers
apiVersion: machineconfiguration.openshift.io/v1
kind: ContainerRuntimeConfig
metadata:
  name: enable-container-gomaxprocs
spec:
  machineConfigPoolSelector:
    matchLabels:
      pools.operator.machineconfiguration.openshift.io/worker: ''
  containerRuntimeConfig:
    containerGomaxprocsBehavior: Autosize
```

After these configurations are applied and the machine config pool rolls out:
- Node boots with automatically calculated system reserved
- kubelet and CRI-O start with `GOMAXPROCS=<calculated>` based on system reserved CPU
- Containers start with `GOMAXPROCS=<calculated>` based on their CPU requests (burstable/best-effort only)

**Examples** (all without CPU limits):
- Container with `requests.cpu: 1500m`, no limit → `GOMAXPROCS=3` (ceil(1.5 * 2) = ceil(3.0) = 3)
- Container with `requests.cpu: 250m`, no limit → `GOMAXPROCS=1` (ceil(0.25 * 2) = ceil(0.5) = 1)
- Container with `requests.cpu: 100m`, no limit → `GOMAXPROCS=1` (ceil(0.1 * 2) = ceil(0.2) = 1)
- Container with no CPU request (best-effort) → `GOMAXPROCS=1` (floor value)
- Container with CPU limit (any QoS class) → No injection (Go 1.25+ handles this)

### API Extensions

#### ContainerRuntimeConfig API

A new field `containerGomaxprocsBehavior` is added to `ContainerRuntimeConfiguration` in `machineconfiguration.openshift.io/v1`:

```go
// ContainerGomaxprocsBehaviorType specifies whether CRI-O should inject GOMAXPROCS into containers
type ContainerGomaxprocsBehaviorType string

const (
    // ContainerGomaxprocsBehaviorAutosize enables automatic GOMAXPROCS injection based on CPU requests
    ContainerGomaxprocsBehaviorAutosize ContainerGomaxprocsBehaviorType = "Autosize"
    // ContainerGomaxprocsBehaviorDisabled disables GOMAXPROCS injection
    ContainerGomaxprocsBehaviorDisabled ContainerGomaxprocsBehaviorType = "Disabled"
)

// ContainerRuntimeConfiguration defines the tuneables of the container runtime
type ContainerRuntimeConfiguration struct {
    // ... existing fields ...
    
    // containerGomaxprocsBehavior controls whether CRI-O automatically injects the GOMAXPROCS environment variable into containers
    // based on their CPU resource requests.
    // Valid values are "Autosize" and "Disabled".
    // When set to "Autosize", CRI-O will automatically set GOMAXPROCS proportional to the container's CPU request,
    // calculated as max(ceil(cpu_request_in_cores * 2), 1). This helps Go applications optimize their runtime parallelism
    // based on the allocated CPU resources rather than the total node capacity.
    // When set to "Disabled", GOMAXPROCS injection is disabled and containers will use Go's default behavior
    // (GOMAXPROCS equals the number of CPUs available on the node).
    // When omitted, this means no opinion and the platform is left to choose a reasonable default, which is subject to change over time.
    // The current default is "Disabled".
    //
    // Containers can override the injected GOMAXPROCS value by:
    // - Setting GOMAXPROCS in the container image Dockerfile (ENV GOMAXPROCS=...)
    // - Setting GOMAXPROCS in the pod spec (env or envFrom)
    // - Calling runtime.GOMAXPROCS() programmatically in Go code
    // - Adding the skip-gomaxprocs.crio.io annotation to the pod
    //
    // +openshift:enable:FeatureGate=GomaxprocsInjection
    // +optional
    // +kubebuilder:validation:Enum=Autosize;Disabled
    ContainerGomaxprocsBehavior ContainerGomaxprocsBehaviorType `json:"containerGomaxprocsBehavior,omitempty"`
}
```

The field is optional. When omitted, the platform chooses a reasonable default, which is subject to change over time. The current default is `Disabled`.

**Feature Gate**: This field is controlled by the `GomaxprocsInjection` feature gate:

```go
// In openshift/api/features/features.go
featureSets := map[configv1.FeatureSet]*FeatureGateEnabledDisabled{
    // ...
    configv1.TechPreviewNoUpgrade: {
        Enabled: []configv1.FeatureGateName{
            // ...
            GomaxprocsInjection,
        },
    },
}

const (
    // ...
    
    // GomaxprocsInjection enables automatic GOMAXPROCS injection for containers
    // via ContainerRuntimeConfig.containerGomaxprocsBehavior
    //
    // owner: @openshift/openshift-team-machine-config-operator
    // alpha: v4.18
    // beta: v4.19
    GomaxprocsInjection configv1.FeatureGateName = "GomaxprocsInjection"
)
```

When the feature gate is disabled, the `containerGomaxprocsBehavior` field is ignored (validation passes but MCO does not render CRI-O configuration).

**Design note**: The API uses an enum (`Autosize`/`Disabled`) rather than exposing CRI-O's integer `min_injected_gomaxprocs` value directly because setting GOMAXPROCS based on CPU request is the correct behavior for how the Kubernetes scheduler bin-packs nodes. Exposing the raw integer would invite misconfiguration (e.g., setting a high static floor that ignores actual CPU requests). If future use cases require a configurable floor or multiplier, a new field can be added to the API without breaking compatibility.

#### KubeletConfig API

A new field `systemGomaxprocsBehavior` is added to `KubeletConfig` in `machineconfiguration.openshift.io/v1` to control GOMAXPROCS configuration for kubelet and CRI-O:

```go
// SystemGomaxprocsBehaviorType specifies whether to automatically configure GOMAXPROCS for system services
type SystemGomaxprocsBehaviorType string

const (
    // SystemGomaxprocsBehaviorAutosize enables automatic GOMAXPROCS calculation based on system reserved CPU
    SystemGomaxprocsBehaviorAutosize SystemGomaxprocsBehaviorType = "Autosize"
    // SystemGomaxprocsBehaviorDisabled disables automatic GOMAXPROCS configuration for system services
    SystemGomaxprocsBehaviorDisabled SystemGomaxprocsBehaviorType = "Disabled"
)

// KubeletConfig is the configuration object for the kubelet
type KubeletConfig struct {
    // ... existing fields ...
    
    // systemGomaxprocsBehavior controls whether the kubelet-auto-node-size service automatically configures
    // GOMAXPROCS for kubelet and CRI-O system services based on the system reserved CPU allocation.
    // GOMAXPROCS is calculated from system reserved CPU, which is determined dynamically by autoSizingReserved
    // when enabled, or from static system reserved values otherwise.
    // Valid values are "Autosize" and "Disabled".
    // When set to "Autosize", the GOMAXPROCS environment variable for kubelet and CRI-O is set to
    // max(ceil(system_reserved_cpu), 1). This optimizes the runtime parallelism of these Go-based system
    // services based on their CPU allocation rather than total node capacity.
    // When set to "Disabled", automatic GOMAXPROCS configuration is disabled and the system services
    // use Go's default behavior (GOMAXPROCS equals the number of CPUs available on the node).
    // When omitted, this means no opinion and the platform is left to choose a reasonable default, which is subject to change over time.
    // The current default is "Disabled".
    //
    // +openshift:enable:FeatureGate=GomaxprocsInjection
    // +optional
    // +kubebuilder:validation:Enum=Autosize;Disabled
    SystemGomaxprocsBehavior SystemGomaxprocsBehaviorType `json:"systemGomaxprocsBehavior,omitempty"`
}
```

The field is optional. When omitted, the platform chooses a reasonable default, which is subject to change over time. The current default is `Disabled`.

**Feature Gate**: This field is controlled by the `GomaxprocsInjection` feature gate (same gate as container injection):

```go
// In openshift/api/features/features.go
const (
    // ...
    
    // GomaxprocsInjection enables automatic GOMAXPROCS configuration for:
    // - Containers: via ContainerRuntimeConfig.containerGomaxprocsBehavior
    // - System services: via KubeletConfig.systemGomaxprocsBehavior
    //
    // owner: @openshift/openshift-team-machine-config-operator
    // alpha: v4.18
    // beta: v4.19
    GomaxprocsInjection configv1.FeatureGateName = "GomaxprocsInjection"
)
```

When the feature gate is disabled, the `systemGomaxprocsBehavior` field is ignored (validation passes but MCO does not render node sizing configuration).

**Note**: System reserved is the CPU allocation FOR system services (kubelet, CRI-O, systemd, sshd, udev, journald, etc.). GOMAXPROCS is set to match the parallelism available within this allocation, while cgroups enforce the actual CPU time limit.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Supported. GOMAXPROCS configuration is applied at the node level and affects the data plane only. Both ContainerRuntimeConfig and KubeletConfig resources are embedded in ConfigMaps via the NodePool `.config` API and baked into ignition payloads by the Ignition Server. CRI-O running on worker nodes in the guest cluster performs container GOMAXPROCS injection, and the kubelet-auto-node-size service configures system service GOMAXPROCS. Control plane components running in the management cluster are unaffected. No HostedCluster API changes are required as both ContainerRuntimeConfig and KubeletConfig are already supported through the existing NodePool configuration mechanism.

#### Standalone Clusters

This is the primary deployment model for this feature. Both the ContainerRuntimeConfig and KubeletConfig APIs apply to all machine config pools (master, worker, custom pools).

#### Single-node Deployments or MicroShift

**Single-node OpenShift (SNO)**: This feature works on SNO with no additional resource overhead. The ContainerRuntimeConfig and KubeletConfig reconciliation happens once at node startup and has negligible CPU/memory impact. System service GOMAXPROCS configuration via `systemGomaxprocsBehavior: Autosize` is particularly beneficial on SNO where system reserved values may vary significantly based on the single node's capacity.

**MicroShift**: This feature is not applicable to MicroShift, which does not use the Machine Config Operator, ContainerRuntimeConfig API, or KubeletConfig API. MicroShift administrators can configure CRI-O's `min_injected_gomaxprocs` directly via the CRI-O configuration file if desired, but system service GOMAXPROCS would require manual configuration.

#### OpenShift Kubernetes Engine

This feature is available in OKE. The ContainerRuntimeConfig and KubeletConfig APIs and Machine Config Operator are all included in OKE, so the functionality works identically to standalone OpenShift Container Platform.

### Implementation Details/Notes/Constraints

#### Container GOMAXPROCS Injection (CRI-O)

CRI-O's GOMAXPROCS injection feature (https://github.com/cri-o/cri-o/pull/9860) uses the `min_injected_gomaxprocs` configuration option as a floor value. When set to a value greater than 0, CRI-O enables the injection hook and uses that value as the minimum GOMAXPROCS. The calculation formula is `max(ceil(cpu_request_in_cores * 2), min_injected_gomaxprocs)`.

**Rationale for doubling**: Doubling the CPU request reduces the likelihood the Go runtime will throttle itself in cases where there is excess CPU capacity beyond the request, allowing applications to burst when CPU is available.

**Behavior for edge cases:**
- Container with no CPU request (best-effort QoS): Receives the floor value directly (`GOMAXPROCS=1`)
- Container with any CPU limit: CRI-O **skips injection entirely**, regardless of QoS class. This includes:
  - Guaranteed pods (requests = limits)
  - Burstable pods with both request and limit set
  - Pods with only a limit (request defaults to limit)
  
  Go 1.25+ automatically derives GOMAXPROCS from the cgroup CPU quota, making injection redundant and potentially incorrect for limited containers.
- Container with CPU request < 500m and no limit: Receives `GOMAXPROCS=1` (e.g., 250m: ceil(0.25 * 2) = ceil(0.5) = 1)
- Pod with `skip-gomaxprocs.crio.io` annotation: CRI-O skips injection for that pod even when enabled globally

The Machine Config Operator will:
1. Translate `containerGomaxprocsBehavior: Autosize` to `min_injected_gomaxprocs = 1` in CRI-O config
2. Translate `containerGomaxprocsBehavior: Disabled` to `min_injected_gomaxprocs = 0` (or omit the setting)
3. Apply the configuration via the standard MCO reconciliation process
4. If the feature gate is disabled, the field is silently ignored and CRI-O config remains unchanged

#### System Service GOMAXPROCS Configuration (Kubelet and CRI-O)

When `systemGomaxprocsBehavior: Autosize` is set in KubeletConfig, the kubelet-auto-node-size service will:

1. Run after system reserved calculation (either from auto node sizing or from static values)
2. Calculate GOMAXPROCS for kubelet and CRI-O based on the system reserved CPU
3. Write the values to `/etc/node-sizing.env` alongside system reserved values

**Calculation formula**: 
```
# In OpenShift, system reserved is the CPU slice allocated TO system services
# (kubelet, CRI-O, systemd, sshd, udev, etc.), not reserved away from them.
# GOMAXPROCS should represent the parallelism available within this allocation.
# Using ceil allows maximum parallelism while cgroups enforce the actual CPU time limit.
gomaxprocs = max(ceil(system_reserved_cpu), 1)
```

**Rationale for ceil**: Using `ceil` allows kubelet and CRI-O to utilize available parallelism within the system reserved slice. For example, with 1.5 CPU reserved, `GOMAXPROCS=2` allows 2-way parallelism while cgroups limit actual CPU time to 1.5 CPU-seconds per period.

**Example calculations**:
- Node with 8 CPUs, system reserved = 0.5 CPU → kubelet/CRI-O GOMAXPROCS = 1 (ceil(0.5) = 1)
- Node with 8 CPUs, system reserved = 1.2 CPU → kubelet/CRI-O GOMAXPROCS = 2 (ceil(1.2) = 2)
- Node with 8 CPUs, system reserved = 2.8 CPU → kubelet/CRI-O GOMAXPROCS = 3 (ceil(2.8) = 3)
- Node with 4 CPUs, system reserved = 0.2 CPU → kubelet/CRI-O GOMAXPROCS = 1 (ceil(0.2) = 1)
- Node with 4 CPUs, system reserved = 0.8 CPU → kubelet/CRI-O GOMAXPROCS = 1 (ceil(0.8) = 1)
- Node with 2 CPUs, system reserved = 0.5 CPU → kubelet/CRI-O GOMAXPROCS = 1 (ceil(0.5) = 1)

**Integration with auto node sizing**:
- If `autoSizingReserved: true` is set, system reserved is calculated dynamically by the auto node sizing script
- If `autoSizingReserved: false`, system reserved uses the static values from `/etc/node-sizing-enabled.env`
- In both cases, `systemGomaxprocsBehavior: Autosize` uses whatever system reserved value is available to calculate GOMAXPROCS

The `/etc/node-sizing.env` file format is extended to include:
```bash
SYSTEM_RESERVED_MEMORY=3.5Gi
SYSTEM_RESERVED_CPU=1.5
GOMAXPROCS=2
```
(GOMAXPROCS = ceil(1.5) = 2)

The kubelet and CRI-O systemd service files will be updated to source this environment file and set `GOMAXPROCS` before starting the respective processes.

### Risks and Mitigations

**Risk**: Applications that explicitly set GOMAXPROCS may have unexpected behavior.

**Mitigation**: CRI-O injects GOMAXPROCS early in the container startup process, but explicit environment variables take precedence using standard environment variable override semantics:
- GOMAXPROCS set in the container image's Dockerfile ENV: **Overrides** CRI-O injection
- GOMAXPROCS set in pod spec `env` or `envFrom`: **Overrides** CRI-O injection  
- GOMAXPROCS set programmatically via `runtime.GOMAXPROCS()`: **Overrides** any environment variable
- If no explicit GOMAXPROCS is set anywhere, the CRI-O injected value is used

In all cases, applications retain control and can override the injected value.

**Risk**: The calculation formula (2x CPU request) may not be optimal for all workloads.

**Mitigation**: This is the default behavior; operators can disable injection globally or per-pool. Future enhancements could add tunability if needed.

**Risk**: Incorrect GOMAXPROCS calculation for kubelet/CRI-O could degrade system service performance.

**Mitigation**: The calculation is simple and straightforward (`ceil(system_reserved_cpu)`), setting GOMAXPROCS to match the parallelism available within the system reserved CPU allocation. The feature is opt-in via `systemGomaxprocsBehavior: Autosize`, so clusters can disable it by omitting the field or setting it to `Disabled`. Additionally, the formula ensures GOMAXPROCS is at least 1, preventing invalid values. Using `ceil` allows better parallelism utilization while cgroups enforce the actual CPU time limits.

**Risk**: The kubelet-auto-node-size service runs before kubelet/CRI-O start, so any failure in the service could delay or prevent node startup.

**Mitigation**: The service is designed to be simple and fast (< 1s execution time). If the service fails, kubelet and CRI-O will start without GOMAXPROCS set and use Go's default behavior (total CPU count), which is safe but potentially suboptimal. The service logs errors to systemd journal for debugging.

### Drawbacks

#### Container GOMAXPROCS Injection

- **Injection scope is limited**: CRI-O only injects GOMAXPROCS for burstable and best-effort pods without CPU limits. This excludes Guaranteed pods and any pod with a limit set, which may be a large share of production workloads. However, this is the correct behavior since Go 1.25+ handles limited containers automatically.

- **Formula may not be optimal for all workloads**: The 2x multiplier is based on upstream research, but some workloads may benefit from different values. However, users can override via pod spec environment variables if needed.

- **GA default flip impact**: If this feature graduates to GA with `Autosize` as the platform default, it creates a **silent behavior change on upgrade** for existing clusters:
  
  **What changes:**
  - Every cluster that does not explicitly set `Disabled` will start injecting GOMAXPROCS on upgrade
  - All burstable and best-effort pods without CPU limits will receive `GOMAXPROCS = ceil(2 * request)` instead of defaulting to node CPU count
  - Only workloads that DON'T explicitly set GOMAXPROCS are affected (i.e., the ones relying on Go's default behavior)
  
  **Risk scenarios:**
  - Go service with 500m CPU request, no limit, no explicit GOMAXPROCS: Changes from `GOMAXPROCS = node CPU count` (e.g., 8 or 16) to `GOMAXPROCS = 1`
  - Could cause performance degradation for services that were unknowingly benefiting from excess parallelism
  - Testing in pre-production may not catch this if test clusters have different node sizes than production
  
  **Mitigations:**
  - Applications with explicit GOMAXPROCS settings (via Dockerfile ENV, pod spec, or `runtime.GOMAXPROCS()`) are unaffected
  - Pod-level opt-out via `skip-gomaxprocs.crio.io` annotation
  - Cluster-wide opt-out via `containerGomaxprocsBehavior: Disabled`

#### System Service GOMAXPROCS

- **Additional systemd service dependency**: The kubelet-auto-node-size service becomes a hard dependency for kubelet and CRI-O startup. If the service fails, it could delay node readiness. However, the service is designed to be fast and fail-safe (falls back to no GOMAXPROCS on error).

- **Tight coupling with auto node sizing**: While `systemGomaxprocsBehavior: Autosize` can work independently, it shares the same systemd service and configuration files as auto node sizing, creating an implicit coupling. This means bugs in the auto node sizing script could affect GOMAXPROCS calculation even when `autoSizingReserved: false`.

- **Limited testing coverage for system services**: Unlike container GOMAXPROCS which affects many workloads and gets broad testing, system service GOMAXPROCS only affects kubelet and CRI-O, making it harder to detect edge cases or performance regressions across different node sizes and workloads.

## Design Details

### Open Questions

#### GA Default Behavior

**Question**: Should the platform default at GA be `Autosize` or `Disabled`?

**Context**: During Tech Preview, the feature is opt-in via feature gate. At GA, we need to decide the default value when users don't explicitly configure `containerGomaxprocsBehavior`.

**Option 1 - Default to `Autosize`**:
- **Pros**: Better out-of-box experience for new deployments; workloads automatically optimized
- **Cons**: Silent behavior change on upgrade for existing clusters; workloads relying on default `GOMAXPROCS = node CPU count` will switch to `ceil(2 * request)`
- **Upgrade impact**: Requires upgrade planning and testing for existing clusters

**Option 2 - Default to `Disabled`**:
- **Pros**: Safer upgrade path; no silent behavior changes; predictable behavior
- **Cons**: Requires manual opt-in to benefit; new deployments miss optimization unless configured
- **Upgrade impact**: Minimal; behavior only changes for clusters that explicitly opt-in

**Decision Criteria** (to be evaluated during Tech Preview):
1. Measured performance impact (positive and negative) from Tech Preview users
2. Percentage of real-world workloads that would be affected (burstable without limits, without explicit GOMAXPROCS)
3. Customer feedback on upgrade behavior preferences
4. Precedent from similar platform-level optimizations

**Target Resolution**: Before Tech Preview -> GA graduation (see Graduation Criteria)

## Test Plan

### Container GOMAXPROCS Injection Tests

**Unit tests** (Machine Config Operator):
- Field validation: Accepts `Autosize` and `Disabled`, rejects invalid values
- CRI-O config rendering: Correctly translates API field to `min_injected_gomaxprocs` setting

**Integration tests**:
- Verify ContainerRuntimeConfig with `containerGomaxprocsBehavior: Autosize` renders correct CRI-O configuration
- Verify ContainerRuntimeConfig with `containerGomaxprocsBehavior: Disabled` omits or sets `min_injected_gomaxprocs = 0`
- Verify feature gate enforcement: field is ignored when `GomaxprocsInjection` feature gate is disabled

**E2E tests**:
- Deploy ContainerRuntimeConfig with injection enabled, verify GOMAXPROCS is set in burstable containers with CPU requests but no limits
- Deploy ContainerRuntimeConfig with injection disabled, verify GOMAXPROCS is not injected
- Verify containers with explicit GOMAXPROCS environment variable use their own value (not injected)
- Verify best-effort containers (no CPU request) receive the floor value (GOMAXPROCS=1)
- Verify containers with CPU limits do not receive GOMAXPROCS injection (any QoS class)
- Verify Guaranteed pods (requests = limits) do not receive GOMAXPROCS injection

### System Service GOMAXPROCS Tests

**Unit tests** (Machine Config Operator):
- Field validation: `systemGomaxprocsBehavior` accepts `Autosize` and `Disabled`, rejects invalid values
- Script generation: Verify auto node sizing script correctly calculates GOMAXPROCS from system reserved CPU
- Environment file rendering: Verify `/etc/node-sizing.env` includes GOMAXPROCS when `systemGomaxprocsBehavior: Autosize`

**Integration tests**:
- Verify KubeletConfig with `systemGomaxprocsBehavior: Autosize` and `autoSizingReserved: true` calculates both system reserved and GOMAXPROCS
- Verify KubeletConfig with `systemGomaxprocsBehavior: Autosize` and `autoSizingReserved: false` uses static system reserved but calculates GOMAXPROCS
- Verify KubeletConfig with `systemGomaxprocsBehavior: Disabled` does not write GOMAXPROCS to environment file
- Verify KubeletConfig with `systemGomaxprocsBehavior` omitted does not write GOMAXPROCS to environment file
- Verify systemd service ordering: kubelet-auto-node-size runs before kubelet and CRI-O

**E2E tests**:
- Deploy KubeletConfig with `systemGomaxprocsBehavior: Autosize`, verify kubelet process has GOMAXPROCS set in environment
- Deploy KubeletConfig with `systemGomaxprocsBehavior: Autosize`, verify CRI-O process has GOMAXPROCS set in environment
- Verify GOMAXPROCS value matches calculation: `ceil(system_reserved_cpu)`
- Verify GOMAXPROCS updates when system reserved changes (e.g., node resized or auto sizing recalculates)
- Test with various node sizes (2, 4, 8, 16, 32 CPUs) to verify calculation across range

**Upgrade tests**:
- Verify upgrade from version without feature to version with feature (feature gate disabled) has no impact
- Verify upgrade with feature gate enabled transitions smoothly
- Verify existing auto node sizing deployments (without `systemGomaxprocsBehavior`) continue to work unchanged

## Graduation Criteria

### Dev Preview -> Tech Preview

- API field implemented behind `GomaxprocsInjection` feature gate
- MCO correctly renders CRI-O configuration based on API field
- Unit and integration tests passing
- Basic E2E test coverage (injection enabled/disabled scenarios)
- Initial documentation in openshift-docs

### Tech Preview -> GA

- No critical bug reports or performance regressions
- E2E tests passing for sufficient period
- Load testing confirms no performance impact on node or CRI-O
- Complete user-facing documentation in openshift-docs
- Available by default in all cluster profiles
- **Decision on GA default behavior**: Based on Tech Preview feedback, choose between:
  - **Option 1 (Autosize by default)**: Platform default is `Autosize`, optimizing new deployments automatically but requiring upgrade planning for existing clusters
  - **Option 2 (Disabled by default)**: Platform default is `Disabled`, providing safer upgrades but requiring manual opt-in for optimization
  - Decision criteria should include:
    - Measured performance impact (positive and negative) from Tech Preview deployments
    - Percentage of workloads in the field that would be affected (burstable without limits and without explicit GOMAXPROCS)
    - Customer feedback on upgrade behavior preferences
    - Comparison with similar platform-level optimizations and their default strategies

### Removing a deprecated feature

Not applicable. This is a new feature with no deprecation planned.

## Upgrade / Downgrade Strategy

**Upgrades**: 
- Tech Preview: The feature is opt-in via the `GomaxprocsInjection` feature gate. Upgrading to a version with this feature has no impact unless the feature gate is explicitly enabled.
- GA: If the platform default changes to `Autosize` at GA (see GA Default Behavior open question), clusters that do not explicitly configure the field will begin injecting GOMAXPROCS. This is mitigated by:
  - CRI-O only injects GOMAXPROCS if no environment variable is already set
  - Applications that explicitly set GOMAXPROCS (via Dockerfile ENV, pod spec, or `runtime.GOMAXPROCS()`) are unaffected
  - Pod-level opt-out via `skip-gomaxprocs.crio.io` annotation
  - If the default remains `Disabled` at GA, no behavior change occurs on upgrade

**Downgrades**: 
- Downgrading to a version without this feature or with the feature gate disabled will cause the field to be ignored in the API and CRI-O will stop injecting GOMAXPROCS
- Existing containers continue running with their current GOMAXPROCS values until restarted
- No data loss or corruption occurs

## Version Skew Strategy

This is a node-level feature controlled by CRI-O configuration. Version skew scenarios:

**Control plane N+1, nodes N**: The new API field exists in the control plane API but is ignored by older MCO versions on N nodes. No impact on cluster operation.

**Control plane N, nodes N+1**: Upgrading nodes first is not a supported pattern, but if it occurs, the newer CRI-O ignores the missing field and uses its default behavior.

**Mixed node versions (during rolling upgrade)**: Some nodes may have injection enabled while others do not. This is safe because:
- GOMAXPROCS injection is per-container and happens at container start time
- Pods scheduled to older nodes use default Go behavior (GOMAXPROCS = total CPU count)
- Pods scheduled to newer nodes use injected GOMAXPROCS
- Pod behavior is consistent once scheduled (does not change based on node upgrade)

No coordination between CRI-O versions across nodes is required.

## Operational Aspects of API Extensions

This enhancement adds a new field to the existing ContainerRuntimeConfig CRD but does not introduce new CRDs, webhooks, aggregated API servers, or finalizers.

**Impact on existing SLIs**:
- No impact on API throughput: The field is read-only after ContainerRuntimeConfig reconciliation
- No impact on pod scheduling latency: CRI-O reads the configuration once at startup
- Minimal impact on container start time: GOMAXPROCS calculation adds <1ms per container

**Health indicators**:
- Machine Config Operator conditions: `Degraded=False`, `Available=True`
- CRI-O service status on nodes: `systemctl status crio`
- MCO logs show successful CRI-O config rendering

**Failure modes**:

*Container GOMAXPROCS:*
- Invalid enum value in API: Admission validation rejects the ContainerRuntimeConfig
- CRI-O fails to parse config: CRI-O falls back to default behavior (no injection), logs error
- Feature gate disabled but field set: Field is silently ignored, no impact on cluster

*System Service GOMAXPROCS:*
- Invalid enum value in API: Admission validation rejects the KubeletConfig
- kubelet-auto-node-size service fails: kubelet/CRI-O start without GOMAXPROCS set, use Go defaults (safe fallback)
- System reserved CPU is 0 or invalid: GOMAXPROCS calculation returns 1 (floor value), logged to systemd journal
- Feature gate disabled but field set: Field is silently ignored, no impact on cluster

**Observability without status fields**:

ContainerRuntimeConfig and KubeletConfig do not have status subresources. Configuration failures are surfaced through Machine Config Operator status conditions (Degraded/Progressing/Available), consistent with other fields in these APIs. Runtime verification is available via Support Procedures below. The fail-safe design (fallback to Go defaults on error) ensures configuration failures do not prevent node startup or break existing workloads.

## Support Procedures

### Container GOMAXPROCS Injection

**How to detect if GOMAXPROCS injection is enabled**:

1. Check ContainerRuntimeConfig:
   ```bash
   oc get containerruntimeconfig -o yaml
   ```
   Look for `containerGomaxprocsBehavior: Autosize`

2. Verify CRI-O configuration on node:
   ```bash
   oc debug node/<node-name>
   chroot /host
   grep min_injected_gomaxprocs /etc/crio/crio.conf.d/*
   ```
   Expected: `min_injected_gomaxprocs = 1`

3. Check container environment:
   ```bash
   oc exec <pod-name> -- printenv GOMAXPROCS
   ```
   Should show calculated value based on CPU request

**How to disable container GOMAXPROCS injection**:

1. Update or create ContainerRuntimeConfig:
   ```yaml
   apiVersion: machineconfiguration.openshift.io/v1
   kind: ContainerRuntimeConfig
   metadata:
     name: disable-gomaxprocs
   spec:
     machineConfigPoolSelector:
       matchLabels:
         pools.operator.machineconfiguration.openshift.io/worker: ''
     containerRuntimeConfig:
       containerGomaxprocsBehavior: Disabled
   ```

2. Wait for MachineConfigPool to roll out updated configuration

**Consequences of disabling**:
- Existing running containers: Continue using current GOMAXPROCS value until restarted
- New containers: Use default Go behavior (GOMAXPROCS = total node CPU count)
- No impact on cluster health or pod scheduling

### System Service GOMAXPROCS Configuration

**How to detect if system service GOMAXPROCS is enabled**:

1. Check KubeletConfig:
   ```bash
   oc get kubeletconfig -o yaml
   ```
   Look for `systemGomaxprocsBehavior: Autosize`

2. Verify node sizing configuration:
   ```bash
   oc debug node/<node-name>
   chroot /host
   cat /etc/node-sizing.env
   ```
   Expected output should include:
   ```bash
   SYSTEM_RESERVED_MEMORY=3.5Gi
   SYSTEM_RESERVED_CPU=1.5
   GOMAXPROCS=2
   ```
   (GOMAXPROCS = ceil(1.5) = 2)

3. Check kubelet process environment:
   ```bash
   oc debug node/<node-name>
   chroot /host
   systemctl show kubelet --property=Environment | grep GOMAXPROCS
   ```
   Should show `GOMAXPROCS=<calculated-value>`

4. Check CRI-O process environment:
   ```bash
   oc debug node/<node-name>
   chroot /host
   systemctl show crio --property=Environment | grep GOMAXPROCS
   ```
   Should show `GOMAXPROCS=<calculated-value>`

**How to disable system service GOMAXPROCS**:

1. Update or create KubeletConfig:
   ```yaml
   apiVersion: machineconfiguration.openshift.io/v1
   kind: KubeletConfig
   metadata:
     name: disable-system-gomaxprocs
   spec:
     systemGomaxprocsBehavior: Disabled
     machineConfigPoolSelector:
       matchLabels:
         pools.operator.machineconfiguration.openshift.io/worker: ''
   ```

2. Wait for MachineConfigPool to roll out updated configuration

**Consequences of disabling**:
- kubelet and CRI-O processes: Will use Go default GOMAXPROCS (= total node CPU count) on next restart
- System reserved calculation: Continues to work normally if `autoSizingReserved: true`
- No impact on cluster health or pod scheduling

**Common troubleshooting scenarios**:

#### Container GOMAXPROCS Issues

- **GOMAXPROCS not injected into container**: Most likely causes:
  - Container has a CPU limit set (injection is skipped for any pod with a limit)
  - Pod is Guaranteed QoS (requests = limits, so has a CPU limit)
  - Feature gate is disabled
  - CRI-O config does not have `min_injected_gomaxprocs = 1`
  - Pod has `skip-gomaxprocs.crio.io` annotation set
- **Wrong GOMAXPROCS value in container**: Verify CPU request matches expected value using formula `ceil(request * 2)`, check for explicit GOMAXPROCS in pod spec overriding injection
- **Container using default GOMAXPROCS (= node CPU count)**: Container likely has a CPU limit set (even Guaranteed pods skip injection), or explicit override in pod/image

#### System Service GOMAXPROCS Issues

- **GOMAXPROCS not set for kubelet/CRI-O**: Most likely causes:
  - `systemGomaxprocsBehavior: Disabled` or not set in KubeletConfig
  - kubelet-auto-node-size service failed to run (check `systemctl status kubelet-auto-node-size`)
  - `/etc/node-sizing.env` file missing or malformed
  - systemd service files not updated to source environment file
- **Wrong GOMAXPROCS value for system services**: 
  - Verify system reserved CPU is calculated correctly in `/etc/node-sizing.env`
  - Check calculation: `GOMAXPROCS = ceil(system_reserved_cpu)`
  - For 8 CPU node with 1.5 system reserved: expected GOMAXPROCS = 2 (ceil(1.5) = 2)
  - For 4 CPU node with 0.5 system reserved: expected GOMAXPROCS = 1 (ceil(0.5) = 1)
- **GOMAXPROCS not updating after node resize**:
  - Node rebooted but kubelet-auto-node-size service didn't recalculate (check service logs)
  - System reserved values stale in `/etc/node-sizing-enabled.env` (if auto sizing disabled)

## Implementation History

- 2026-06-24: Initial proposal and API implementation

## Alternatives (Not Implemented)

### API Placement Alternatives

#### Use a separate CRD instead of fields in ContainerRuntimeConfig and KubeletConfig

A dedicated GOMAXPROCS CR could centralize both container and system service configuration in one place, with its own status subresource for reporting configuration state.

**Rejected**: Each field belongs in the CR that owns the underlying configuration it controls:

- **ContainerRuntimeConfig**: `containerGomaxprocsBehavior` configures CRI-O's `min_injected_gomaxprocs` setting, which is a CRI-O configuration field. ContainerRuntimeConfig is the standard API for editing CRI-O configuration, so this is the natural home.

- **KubeletConfig**: `systemGomaxprocsBehavior` controls GOMAXPROCS for system services via systemd drop-ins rather than kubelet configuration directly. However, KubeletConfig already owns the closely related `autoSizingReserved` field, which established KubeletConfig as the owner of system slice resource configuration (even though kubelet is only the entity that applies the system reserved values, not the only service affected by them). GOMAXPROCS for system services is calculated from the same system reserved CPU values and delivered through the same kubelet-auto-node-size service, so KubeletConfig is the consistent place for it.

A separate CR would also lose per-pool configuration via `machineConfigPoolSelector`, which both ContainerRuntimeConfig and KubeletConfig already support. Per-pool control matters here because master and worker nodes may have different node sizes and different system reserved allocations.

### Container GOMAXPROCS Alternatives

#### Use automaxprocs library

Applications could use `go.uber.org/automaxprocs` or similar libraries to set GOMAXPROCS based on cgroup limits.

**Rejected**: Requires code changes in every Go application and creates runtime dependency overhead. Cluster-level configuration is more transparent and universal.

#### Set GOMAXPROCS via pod environment variables

Operators could manually calculate and inject GOMAXPROCS via pod specs.

**Rejected**: Requires manual calculation and configuration per workload. Does not scale for large clusters with many Go applications.

#### Make the calculation formula configurable

Allow cluster administrators to configure the multiplier (currently hardcoded as 2x).

**Rejected**: The 2x multiplier is based on upstream CRI-O's research and testing.

### System Service GOMAXPROCS Alternatives

#### Set GOMAXPROCS in systemd service files directly

Instead of calculating dynamically, hardcode GOMAXPROCS values in kubelet.service and crio.service files.

**Rejected**: Does not account for different node sizes or system reserved allocations. Would require manual tuning per node type.

#### Use a separate systemd service for GOMAXPROCS calculation

Instead of extending kubelet-auto-node-size, create a new dedicated service for GOMAXPROCS calculation.

**Rejected**: Adds unnecessary complexity and another systemd service dependency. kubelet-auto-node-size already runs at the right time (before kubelet/CRI-O start) and has access to the necessary node capacity information.

#### Calculate GOMAXPROCS inside kubelet/CRI-O code

Modify kubelet and CRI-O to automatically detect and set their own GOMAXPROCS at runtime.

**Rejected**: Requires upstream Kubernetes and CRI-O changes, which are unlikely to be accepted since this is an OpenShift-specific optimization tied to our auto node sizing feature. Additionally, by the time these processes start, Go runtime has already initialized GOMAXPROCS based on environment variables or defaults.
