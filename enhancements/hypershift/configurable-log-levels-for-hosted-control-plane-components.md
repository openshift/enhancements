---
title: configurable-log-levels-for-hosted-control-plane-components
authors:
  - "@rutvik23"
  - "@dhgautam99"
  - "@amogh-redhat"
  - "@PoornimaSingour"
  - "@vismishr"
  - "@vsolanki12"
reviewers:
  - "@devguyio"
  - "@celebdor"
  - "@muraee"
  - "@saschagrunert" 
approvers:
  - "@csrwng"
  - "@devguyio"
api-approvers:
  - "@joelspeed"
  - "@enxebre"
creation-date: 2026-06-09
last-updated: 2026-08-14
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-3156
see-also:
  - https://issues.redhat.com/browse/RFE-7777
  - https://issues.redhat.com/browse/CNTRLPLANE-3290
  - https://issues.redhat.com/browse/CNTRLPLANE-3291
  - https://issues.redhat.com/browse/OSDOCS-19157
replaces: []
superseded-by: []
---

# Configurable Log Levels for Hosted Control Plane Components

## Summary

This enhancement introduces structured, per-component log level configuration for hosted
control plane components managed by the Control Plane Operator (CPO) in HyperShift.
Administrators will be able to set intent-based log levels (`Normal`, `Debug`, `Trace`,
`TraceAll`) on the HostedCluster Custom Resource for kube-apiserver,
kube-controller-manager, kube-scheduler, etcd, openshift-apiserver,
openshift-controller-manager, openshift-oauth-apiserver, and oauth-server. The CPO will
translate these intent-based levels into component-specific mechanisms (`--v=N` for
klog-based components, `ETCD_LOG_LEVEL` env var for etcd), achieving operational parity
with standard OCP's `operatorv1.OperatorSpec.LogLevel` pattern. Each component gets a
dedicated per-component type (e.g., `KubeAPIServerOperatorSpec`) that embeds the shared
`ComponentLogLevelSpec` via struct embedding, added to the existing `OperatorConfiguration`
type. This design is future-proof — each component type can be independently extended
without breaking changes. The new fields are gated behind a feature gate and follow a
**gate first, promote when ready** lifecycle.

## Motivation

HyperShift administrators and support engineers currently cannot adjust log verbosity for
hosted control plane components during troubleshooting. In standard OCP, changing log
levels is a routine operation via `oc patch` on operator resources, but in HyperShift the
control plane runs in a management cluster namespace with no user-facing logging
configuration.

This gap creates significant operational pain:

- **Slower incident resolution:** Without verbose logs, diagnosing obscure control plane
  failures requires escalation and often cluster recreation, extending MTTR from hours to
  days.
- **No proactive debugging:** Teams cannot temporarily increase verbosity in pre-production
  to gather data about component behavior before promoting changes.
- **Operational gap vs standard OCP:** Customers managing both standalone and hosted
  clusters face inconsistent tooling, increasing training burden and operational complexity.
- **Support burden:** CEE engineers cannot guide customers through standard log-level-based
  troubleshooting workflows, leading to more escalations to engineering.

Currently only kube-apiserver has a verbosity annotation
(`hypershift.openshift.io/kube-apiserver-verbosity-level`), which is ad-hoc, unvalidated
(raw integer), and inconsistent with the OCP operator pattern. All other components managed
by CPO have no user-facing logging configuration.

This was reported via [RFE-7777](https://issues.redhat.com/browse/RFE-7777) and accepted
as "crucial even for the most minimal troubleshooting."

### User Stories

- As a **HyperShift cluster administrator**, I want to increase the log verbosity of
  kube-apiserver on my hosted cluster so that I can diagnose API request failures without
  escalating to engineering.

- As a **CEE support engineer**, I want to instruct customers to set `Debug` log level on
  specific hosted control plane components via the HostedCluster CR so that I can follow
  the same troubleshooting playbooks used for standard OCP clusters.

- As a **platform engineer** managing hosted clusters in pre-production, I want to
  temporarily set `Trace` level on openshift-apiserver and openshift-controller-manager so
  that I can capture detailed component behavior before promoting changes to production.

### Goals

1. Enable administrators to configure log verbosity for all 8 core control plane components
   via the HostedCluster CR, replacing the existing ad-hoc annotation-based mechanism for
   kube-apiserver.
2. Establish the `ComponentLogLevelSpec` pattern and `LogLevelToKlogVerbosity()` utility
   reused across all klog-based components, and `LogLevelToEtcdLevel()` for etcd.
3. Provide a deprecation path for the existing
   `hypershift.openshift.io/kube-apiserver-verbosity-level` annotation.
4. Achieve operational parity with standard OCP's `operatorv1.LogLevel` pattern using
   intent-based log levels (`Normal`, `Debug`, `Trace`, `TraceAll`).
5. Ensure log level changes take effect via rolling restart without cluster disruption or
   downtime.

### Non-Goals

1. **Configuring all CPO-managed components in this enhancement.** This enhancement covers
   the 8 core control plane components. Additional components may be addressed in follow-up
   enhancements.

2. **The following components are out of scope for this enhancement** due to their logging
   model not supporting a standard configurable verbosity interface compatible with the
   `Normal/Debug/Trace/TraceAll` abstraction:
   - `control-plane-operator` self-log-level: the CPO binary is deployed by the
     *hypershift-operator*, not by CPO's own reconciliation loop. CPO cannot configure its
     own log level through its own reconciliation.
   - Catalog images (`certified-operators-catalog`, `community-operators-catalog`,
     `redhat-operators-catalog`, `redhat-marketplace-catalog`): use opm/gRPC serving with
     no standard verbosity flag.
   - HAProxy-based components (`router`, `ignition-server-proxy`): log level is
     configuration-file-based, not flag-based.
   - `aws-node-termination-handler`: upstream AWS daemon with a custom logging model not
     compatible with intent-based log level mapping.
   - `olm-collect-profiles`, `featuregate-generator`: cronjob/generator workloads, not
     long-running operators with persistent log level state.
   - `metrics-proxy`: internal sidecar with no user-facing verbosity interface.

3. Implementing log aggregation, forwarding, or retention policies for hosted control plane
   component logs.

4. Providing a real-time log streaming interface or console-based log viewer for hosted
   control plane components.

## Proposal

Introduce a new log level configuration section in the HostedCluster API that allows
per-component log level overrides. The CPO will reconcile these settings and propagate them
to the appropriate control plane component deployments and statefulsets.

HyperShift already defines a `LogLevel` type identical to OCP's `operatorv1.LogLevel`,
mapping to the following glog/klog verbosity levels:

| LogLevel         | klog `--v` | etcd `ETCD_LOG_LEVEL` | Use Case                                          |
|------------------|------------|----------------------|---------------------------------------------------|
| Normal (default) | 2          | info                 | Production                                        |
| Debug            | 4          | debug                | Troubleshooting                                   |
| Trace            | 6          | debug¹               | Deep investigation                                |
| TraceAll         | 8          | debug¹               | Full dumps — perf impact, may expose secrets      |

¹ etcd uses zap logging which has no granularity below `debug` — Trace and TraceAll both
map to `debug`.

The chosen API design uses **structured per-component fields** in `OperatorConfiguration`.
This extends the existing pattern (which already has `ClusterVersionOperator`,
`ClusterNetworkOperator`, and `IngressOperator` fields) with new per-component types, each
embedding the shared `ComponentLogLevelSpec` via `json:",inline"`. Each component gets a
dedicated Go type (e.g., `KubeAPIServerOperatorSpec`) that can be independently extended in
the future without breaking vendoring consumers.

When no log level is specified for a component, the default (`Normal`) is used, preserving
backward compatibility.

### Workflow Description

**cluster administrator** is a human user responsible for managing a hosted cluster via the
HostedCluster CR.

**Control Plane Operator (CPO)** is the operator running in the management cluster that
reconciles hosted control plane components.

1. The cluster administrator identifies a need to increase log verbosity for a specific
   control plane component (e.g., kube-apiserver) to diagnose an issue.
2. The cluster administrator patches the HostedCluster CR to set the desired log level for
   the target component:
   ```bash
   oc patch hostedcluster my-cluster --type=merge -p \
     '{"spec":{"operatorConfiguration":{
       "kubeAPIServer":{"logLevel":"Debug"}}}}'
   ```
3. The CRD validates the log level value at admission time via CEL enum validation.
4. The CPO translates the intent-based log level to the component-specific flag (e.g.,
   `Debug` → `--v=4` for kube-apiserver).
5. The CPO updates the component's deployment/statefulset with the new verbosity flag,
   triggering a rolling restart.
6. The component pods restart with the updated verbosity level.
7. The cluster administrator collects the verbose logs for diagnosis.
8. After troubleshooting, the administrator resets the log level to `Normal` (or removes
   the override), and the CPO triggers another rolling restart to restore default verbosity.

#### Entry Point & Data Flow

```mermaid
flowchart TD
    A["Cluster Administrator\nPatches HostedCluster CR with desired log level"]
    B["HostedCluster CR\nUser-facing entry point — spec.operatorConfiguration"]
    C["HostedControlPlane CR\nInternal resource in management cluster namespace"]
    D["Deployment / StatefulSet\nContainer args (--v=N) or env vars (ETCD_LOG_LEVEL) updated"]
    E["Rolling Restart\nZero downtime on HA — 3 replicas for KAS/etcd, 2 for controllers"]

    A -->|"oc patch"| B
    B -->|"hypershift-operator: DeepCopy"| C
    C -->|"CPO: reads LogLevel, maps to flag/env var"| D
    D -->|"Kubernetes: spec changed"| E
```

#### Sequence Diagram

```mermaid
sequenceDiagram
    participant Admin as Cluster Administrator
    participant HC as HostedCluster CR
    participant CPO as Control Plane Operator
    participant Deploy as Component Deployment
    participant Pod as Component Pod

    Admin->>HC: Patch operatorConfiguration (e.g. kubeAPIServer.logLevel=Debug)
    HC->>CPO: Reconcile event
    CPO->>CPO: Validate log level value
    CPO->>CPO: Map LogLevel to component flag (Debug → --v=4)
    CPO->>Deploy: Update container args
    Deploy->>Pod: Rolling restart
    Pod-->>Pod: Starts with new verbosity
    Admin->>Pod: Collect verbose logs for diagnosis
    Admin->>HC: Reset log level to Normal
    HC->>CPO: Reconcile event
    CPO->>Deploy: Restore default args (--v=2)
    Deploy->>Pod: Rolling restart
```

### API Extensions

This enhancement modifies the `HostedCluster` and `HostedControlPlane` CRDs by adding new
fields to the existing `OperatorConfiguration` struct. The existing
`hypershift.openshift.io/kube-apiserver-verbosity-level` annotation is deprecated in favor
of the new structured API.

#### New Type: ComponentLogLevelSpec

Added to `api/hypershift/v1beta1/operator.go` (where `LogLevel` is already defined):

```go
// ComponentLogLevelSpec specifies the log verbosity for a control plane component.
// +kubebuilder:validation:MinProperties=1
type ComponentLogLevelSpec struct {
    // logLevel sets the log verbosity for the component.
    // Valid values are: "Normal", "Debug", "Trace", "TraceAll".
    // Setting this field triggers a rolling restart of the component.
    // When omitted, this means the user has no opinion and the platform
    // defaults to Normal, which is subject to change over time.
    // +optional
    LogLevel LogLevel `json:"logLevel,omitempty"`
}
```

#### Per-Component Wrapper Types

Each component gets a dedicated type that embeds `ComponentLogLevelSpec`. This allows
future per-component extension (e.g., adding `resourceOverrides` to
`KubeAPIServerOperatorSpec`) without breaking Go types for vendoring consumers. The YAML
wire format is unchanged.

```go
// KubeAPIServerOperatorSpec configures the kube-apiserver component.
// +kubebuilder:validation:MinProperties=1
type KubeAPIServerOperatorSpec struct {
    ComponentLogLevelSpec `json:",inline"`
}

// KubeControllerManagerOperatorSpec configures the kube-controller-manager component.
// +kubebuilder:validation:MinProperties=1
type KubeControllerManagerOperatorSpec struct {
    ComponentLogLevelSpec `json:",inline"`
}

// KubeSchedulerOperatorSpec configures the kube-scheduler component.
// +kubebuilder:validation:MinProperties=1
type KubeSchedulerOperatorSpec struct {
    ComponentLogLevelSpec `json:",inline"`
}

// EtcdOperatorSpec configures the etcd component.
// +kubebuilder:validation:MinProperties=1
type EtcdOperatorSpec struct {
    ComponentLogLevelSpec `json:",inline"`
}

// OpenShiftAPIServerOperatorSpec configures the openshift-apiserver component.
// +kubebuilder:validation:MinProperties=1
type OpenShiftAPIServerOperatorSpec struct {
    ComponentLogLevelSpec `json:",inline"`
}

// OpenShiftControllerManagerOperatorSpec configures the openshift-controller-manager component.
// +kubebuilder:validation:MinProperties=1
type OpenShiftControllerManagerOperatorSpec struct {
    ComponentLogLevelSpec `json:",inline"`
}

// OpenShiftOAuthAPIServerOperatorSpec configures the openshift-oauth-apiserver component.
// +kubebuilder:validation:MinProperties=1
type OpenShiftOAuthAPIServerOperatorSpec struct {
    ComponentLogLevelSpec `json:",inline"`
}

// OAuthServerOperatorSpec configures the oauth-server component.
// +kubebuilder:validation:MinProperties=1
type OAuthServerOperatorSpec struct {
    ComponentLogLevelSpec `json:",inline"`
}
```

#### New Fields in OperatorConfiguration

Added to `api/hypershift/v1beta1/hostedcluster_types.go`, extending the existing
`OperatorConfiguration` struct (which already has CVO, CNO, and Ingress fields):

```go
type OperatorConfiguration struct {
    // ...existing ClusterVersionOperator, ClusterNetworkOperator, IngressOperator fields...

    // kubeAPIServer configures the kube-apiserver component.
    // Setting the logLevel field triggers a rolling restart of the component.
    // kube-apiserver runs with 3 replicas (HA) — 2 continue serving while 1 restarts.
    // +optional
    KubeAPIServer KubeAPIServerOperatorSpec `json:"kubeAPIServer,omitzero"`

    // kubeControllerManager configures the kube-controller-manager component.
    // Setting the logLevel field triggers a rolling restart of the component.
    // kube-controller-manager uses leader election — the standby takes over during restart.
    // +optional
    KubeControllerManager KubeControllerManagerOperatorSpec `json:"kubeControllerManager,omitzero"`

    // kubeScheduler configures the kube-scheduler component.
    // Setting the logLevel field triggers a rolling restart of the component.
    // kube-scheduler uses leader election — the standby takes over during restart.
    // +optional
    KubeScheduler KubeSchedulerOperatorSpec `json:"kubeScheduler,omitzero"`

    // etcd configures the etcd component.
    // Setting the logLevel field triggers a rolling restart of the component.
    // etcd runs with 3 replicas — Raft quorum is maintained during rolling update.
    // +optional
    Etcd EtcdOperatorSpec `json:"etcd,omitzero"`

    // openShiftAPIServer configures the openshift-apiserver component.
    // Setting the logLevel field triggers a rolling restart of the component.
    // openshift-apiserver runs with 3 replicas (HA) — 2 continue serving while 1 restarts.
    // +optional
    OpenShiftAPIServer OpenShiftAPIServerOperatorSpec `json:"openShiftAPIServer,omitzero"`

    // openShiftControllerManager configures the openshift-controller-manager component.
    // Setting the logLevel field triggers a rolling restart of the component.
    // openshift-controller-manager uses leader election — the standby takes over during restart.
    // +optional
    OpenShiftControllerManager OpenShiftControllerManagerOperatorSpec `json:"openShiftControllerManager,omitzero"`

    // openShiftOAuthAPIServer configures the openshift-oauth-apiserver component.
    // Setting the logLevel field triggers a rolling restart of the component.
    // openshift-oauth-apiserver runs with 3 replicas (HA) — 2 continue serving while 1 restarts.
    // +optional
    OpenShiftOAuthAPIServer OpenShiftOAuthAPIServerOperatorSpec `json:"openShiftOAuthAPIServer,omitzero"`

    // oauthServer configures the oauth-server component.
    // Setting the logLevel field triggers a rolling restart of the component.
    // oauth-server runs with 3 replicas (HA) — 2 continue serving while 1 restarts.
    // +optional
    OAuthServer OAuthServerOperatorSpec `json:"oauthServer,omitzero"`
}
```

#### N-1/N+1 Compatibility

| Direction                  | Behavior                                                                              |
|----------------------------|---------------------------------------------------------------------------------------|
| N+1 (new code, old data)   | Old data has no log level fields → zero-value structs → defaults apply                |
| N-1 (old code, new data)   | New data has log level fields → old code ignores unknown JSON keys → no error         |

**Propagation is automatic:** `OperatorConfiguration` is already embedded in
`HostedControlPlaneSpec` and propagated via `DeepCopy()`. Adding new fields requires zero
propagation code — `make update` regenerates the `DeepCopy` method to include them.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specifically designed for HyperShift hosted control planes. The log
level configuration is set on the HostedCluster CR in the management cluster and propagated
to the hosted control plane components running in the management cluster namespace. No
changes are required in the guest cluster.

For managed services (ROSA HCP, ARO HCP), service-level guardrails are required before
promoting this feature to GA. The feature gate prevents the fields from being available on
managed service clusters until promoted. The `TraceAll` access policy (CEL rule or
admission webhook) for managed services is a promotion prerequisite — see Graduation
Criteria.

#### Standalone Clusters

This enhancement does not apply to standalone clusters. Standalone OCP clusters already
have log level configuration via the `operatorv1.OperatorSpec.LogLevel` field on each
operator's CR.

#### Single-node Deployments or MicroShift

This enhancement does not apply to single-node deployments or MicroShift. These topologies
use standalone control plane components with existing log level configuration mechanisms.

#### OpenShift Kubernetes Engine

This enhancement applies to OKE deployments that use HyperShift for hosted control planes.
The log level configuration behavior is identical to standard HyperShift deployments. OKE
does not exclude any functionality required by this enhancement.

### Implementation Details/Notes/Constraints

All 8 components are implemented in a single deliverable.

Log level changes apply to the **main container only**, not sidecars. This matches OCP
behavior — sidecars have different logging models, and `UpdateContainer(ComponentName)`
already targets the main container.

The implementation delivers:

1. **API types:** `ComponentLogLevelSpec` struct, per-component wrapper types
   (`KubeAPIServerOperatorSpec`, etc.), and new fields in `OperatorConfiguration`.
2. **Mapping utilities:** `LogLevelToKlogVerbosity()` and `LogLevelToEtcdLevel()` in
   `support/util/loglevel.go`.
3. **CPO integration for all 8 components:** klog-based components unconditionally inject
   `--v=N` (defaulting to `--v=2` when no log level is set); etcd unconditionally injects
   `ETCD_LOG_LEVEL` (defaulting to `info`).
4. **KAS annotation fallback:** Both the existing annotation
   (`hypershift.openshift.io/kube-apiserver-verbosity-level`) and the new API field are
   honored; the API field takes precedence. A deprecation warning condition is deferred
   (CNTRLPLANE-3998, see Future Work).
5. **Per-component unit tests** following the `TestAdaptDeployment` pattern.

#### CPO Integration by Component

| Component                    | Logging Model  | File                                    | Mechanism                               |
|------------------------------|----------------|-----------------------------------------|-----------------------------------------|
| kube-apiserver               | klog           | `v2/kas/deployment.go`                  | `--v=N` (replaces annotation logic)     |
| kube-controller-manager      | klog           | `v2/kcm/deployment.go`                  | `--v=N` appended to args                |
| kube-scheduler               | klog           | `v2/kube_scheduler/deployment.go`       | `--v=N` (replaces hardcoded `--v=2`)    |
| etcd                         | zap (non-klog) | `v2/etcd/statefulset.go`                | `ETCD_LOG_LEVEL` env var                |
| openshift-apiserver          | klog           | `v2/oapi/deployment.go`                 | `--v=N` appended to args                |
| openshift-controller-manager | klog           | `v2/ocm/deployment.go`                  | `--v=N` appended to args                |
| openshift-oauth-apiserver    | klog           | `v2/oauth_apiserver/deployment.go`      | `--v=N` (replaces hardcoded `--v=2`)    |
| oauth-server                 | klog           | `v2/oauth/deployment.go`                | `--v=N` appended to args                |

#### HA Rolling Restart — Zero Downtime

| Component                    | Replicas (HA) | Why No Impact                                          |
|------------------------------|---------------|--------------------------------------------------------|
| kube-apiserver               | 3             | Load balanced — 2 still serving while 1 restarts       |
| kube-controller-manager      | 2             | Leader election — standby takes over in seconds        |
| kube-scheduler               | 2             | Leader election                                        |
| etcd                         | 3             | Raft quorum — 2 of 3 healthy during rolling update     |
| openshift-apiserver          | 3             | Load balanced                                          |
| openshift-controller-manager | 2             | Leader election                                        |
| openshift-oauth-apiserver    | 3             | Load balanced                                          |
| oauth-server                 | 3             | Load balanced — request-serving component              |

**ROSA HCP / ARO HCP:** `controllerAvailabilityPolicy` is always `HighlyAvailable` for
managed services — control plane components run with full HA replica counts. Zero downtime
guaranteed during rolling restarts.

**Multi-component changes:** When multiple components are changed in a single `oc patch`,
the CPO does not explicitly serialize rollouts across all components. Instead, the CPOv2
dependency graph provides natural layer-based ordering:

- **Layer 0:** etcd (no dependencies) — updated immediately
- **Layer 1:** kube-apiserver (depends on etcd `Available=True` + `RolloutComplete=True`)
- **Layer 2:** KCM, kube-scheduler, openshift-apiserver (implicit KAS dependency)
- **Layer 3:** oauth-apiserver, oauth-server, openshift-controller-manager (depend on
  openshift-apiserver)

Components at the same dependency layer are updated in the same reconciliation pass. HA
safety is guaranteed **per-component** — each component has sufficient replicas (3 for
load-balanced, 2 for leader-elected) to handle rolling restarts without downtime.
Cross-component serialization is not required because each component's rollout is
independent, handled by Kubernetes' own rolling update strategy with proper pod disruption
budgets.

### Risks and Mitigations

**Risk:** `TraceAll` (glog level 8) can dump sensitive data including request bodies and
secrets in component logs.
**Mitigation:** The feature is gated — `TraceAll` cannot reach managed service clusters
until promoted. The `TraceAll` access policy for managed services (CEL rule or admission
webhook) is a prerequisite for promotion to GA. Document operational warnings for
`TraceAll` in API docs and release notes.

**Risk:** High verbosity levels significantly increase log volume, potentially impacting log
storage and cluster performance.
**Mitigation:** Document storage impact guidance per log level. Administrators can inspect
the HostedCluster spec directly to identify clusters with non-default log levels.

**Risk:** `TraceAll` on managed services (ROSA HCP, ARO HCP) could expose secrets in logs
flowing to CloudWatch or Azure Monitor (customer-visible).
**Mitigation:** The feature gate prevents exposure on managed services until promoted.
Before promotion, the specific enforcement mechanism (CEL rule or admission webhook
blocking `TraceAll` on managed clusters) must be implemented and validated.

**Risk:** High verbosity on managed services (ROSA HCP, ARO HCP) increases log volume
flowing to CloudWatch or Azure Monitor, potentially impacting storage costs.
**Mitigation:** Assess cost impact of `Debug` and `Trace` levels per component. Consider
auto-revert to `Normal` after a configurable timeout if needed.

**Risk:** Rolling restarts during log level changes could cause brief API unavailability if
multiple components are changed simultaneously.
**Mitigation:** The CPOv2 dependency graph provides natural layer-based ordering across
components (etcd → KAS → controllers → remaining). Each component's HA guarantees (3
replicas for load-balanced, 2 for leader-elected) ensure zero downtime during its own
rolling restart. See "Multi-component changes" under HA Rolling Restart for details.

### Drawbacks

- Adds API surface to the HostedCluster CRD that requires ongoing maintenance and
  documentation.
- The mapping between intent-based log levels and component-specific flags is not a perfect
  abstraction — etcd uses a different logging model (level-based vs. numeric verbosity) and
  some nuance is lost in translation.
- Log level changes trigger rolling restarts, which is a heavier operation than the in-place
  dynamic reconfiguration some users might expect.
- Etcd's zap-based logging has no granularity below `debug` — setting `Trace` or `TraceAll`
  on etcd silently maps to `debug`. This is a deliberate design tradeoff: maintaining a
  unified `LogLevel` type across all components (enabling a future `defaultLogLevel` field)
  is preferred over per-component stricter enums that would reject `Trace`/`TraceAll` for
  etcd at admission time.

## Alternatives (Not Implemented)

1. **Extend the existing annotation pattern:** Add per-component annotations similar to the
   existing KAS verbosity annotation. This was rejected because annotations are unvalidated,
   not discoverable via `oc explain`, and inconsistent with OCP operator conventions.

2. **Expose raw numeric verbosity levels:** Allow users to set raw `--v=N` values directly.
   This was rejected because it does not provide operational parity with OCP's intent-based
   `LogLevel` pattern, requires users to know component-specific verbosity semantics, and
   does not translate well to etcd's level-based logging.

3. **Dynamic log level reconfiguration without restart:** Some components support runtime
   log level changes. This was considered but rejected for the initial implementation
   because not all target components support it, and the rolling restart approach provides
   consistent behavior across all components.

4. **Flat scalar fields (e.g., `kubeAPIServerLogLevel *LogLevel`):** Simpler for users but
   inconsistent with the existing CVO/CNO/Ingress nested pattern in `OperatorConfiguration`,
   and creates a namespace problem if per-component configuration needs to grow beyond just
   log level. The per-component wrapper type approach was chosen for consistency and
   future-proofing.

## Open Questions

1. Should managed services (ROSA HCP, ARO HCP) expose log level configuration to customers
   via ROSA CLI / OCM, or restrict it to SRE/CEE only? Access model to be resolved before
   feature promotion to GA.

## Test Plan

The testing strategy covers the following areas:

- **Unit tests:** Validate `LogLevelToKlogVerbosity()` mapping for all enum values + nil
  (klog `--v=N`).
- **Unit tests:** Validate `LogLevelToEtcdLevel()` mapping for all enum values + nil
  (`ETCD_LOG_LEVEL`).
- **Serialization compat tests:** N-1/N+1 roundtrip for `OperatorConfiguration` with new
  fields set, mixed, and empty.
- **CRD validation test suite (CEL):** Valid log levels accepted, invalid values rejected
  by enum validation — YAML-based testsuite.yaml under
  `cmd/install/assets/crds/hypershift-operator/tests/`.
- **Per-component adapter tests:** Each of the 8 CPO integration points tested via
  `TestAdaptDeployment*LogLevel` (for deployment-based components) or
  `TestAdaptStatefulSet*LogLevel` (for etcd) within each component's own package,
  asserting `--v=N` or `ETCD_LOG_LEVEL` values in the rendered deployment/statefulset.
- **KAS resolver tests:** `TestResolveKASVerbosity` covers all log level enum values and
  the no-configuration default case; `TestAdaptDeploymentKASLogLevel` validates end-to-end
  flag injection including annotation fallback.

Tests should include `[Jira:"Hosted Control Planes"]` and
`[OCPFeatureGate:HCPUserFacingOperatorLogs]` labels for the component.

## Graduation Criteria

This feature follows a **gate first, promote when ready** lifecycle. The new fields are
gated behind the `HCPUserFacingOperatorLogs` feature gate, consistent with the existing `ClusterVersionOperator` field in
`OperatorConfiguration` which is gated behind
`+openshift:enable:FeatureGate=ClusterVersionOperatorConfiguration`.

Under the continuous release model, promotion does not need to wait for the next OCP
release — it happens when the feature is validated. This is already practiced in OCP
(e.g., dualstack support, Karpenter).

### Dev Preview -> Tech Preview

N/A. This feature ships directly as Tech Preview behind the
`HCPUserFacingOperatorLogs` feature gate.

### Tech Preview

- All 8 per-component wrapper types and `ComponentLogLevelSpec` merged behind feature gate.
- `LogLevelToKlogVerbosity()` and `LogLevelToEtcdLevel()` mapping utilities implemented.
- CPO integration for all 8 components complete; KAS annotation (`hypershift.openshift.io/kube-apiserver-verbosity-level`) honored as fallback with the API field taking precedence.
- Unit tests for all mapping utilities and per-component integration points.
- CRD validation test suite (CEL) passes.

### Tech Preview -> GA
- Serialization compatibility tests (N-1/N+1 roundtrip) pass.
- `TraceAll` access policy for managed services resolved and implemented (CEL rule or
  admission webhook blocking `TraceAll` on ROSA HCP / ARO HCP clusters).
- Managed service access model resolved (customer self-service vs SRE/CEE only).
- Verification on all supported platforms.
- CEE validation with [RFE-7777](https://issues.redhat.com/browse/RFE-7777) reporter
  confirms the feature solves their problem.
- User-facing documentation created in
  [openshift-docs](https://github.com/openshift/openshift-docs/)
  ([OSDOCS-19157](https://issues.redhat.com/browse/OSDOCS-19157)).
- Deprecation notice published for the existing KAS verbosity annotation.
- Feature gate promoted — fields available on all cluster types including managed services.

### Removing a deprecated feature

The existing `hypershift.openshift.io/kube-apiserver-verbosity-level` annotation is
deprecated in OCP 5.0 and will be removed in OCP 5.2.

**Deprecation phase (5.0):** Both the annotation and the new
`operatorConfiguration.kubeAPIServer.logLevel` field are honored. When the annotation is
present, the CPO emits a deprecation warning condition on the HostedCluster. If both the
annotation and the API field are set, the API field takes precedence.

**Removal (5.2):** A follow-up PR will:

1. Delete the annotation-reading code path in the KAS reconciler
   (`v2/kas/deployment.go`) — the fallback that checks
   `hypershift.openshift.io/kube-apiserver-verbosity-level` and maps it to `--v=N`.
2. Remove the deprecation warning condition logic that fires when the annotation is
   detected.
3. Update release notes to state the annotation is no longer honored.

After removal, the annotation becomes inert metadata on any HostedCluster that still
carries it — the CPO simply stops reading it. No CRD schema change is required since
annotations are not schema-defined.

## Upgrade / Downgrade Strategy

**Upgrade:** The new log level fields use `omitzero` (non-pointer struct with
`json:",omitzero"`). Clusters upgrading from a version without this feature will have
zero-value structs, and the CPO will continue to use default verbosity (`Normal`). No
action is required from the administrator to maintain previous behavior. The existing KAS
verbosity annotation will continue to be honored during the transition period; if both the
annotation and the new API field are set, the API field takes precedence.

**Downgrade:** If a cluster is downgraded to a version that does not support the new log
level fields, the fields will be ignored by the older CPO. Components will revert to
default verbosity on the next reconciliation. Administrators should reset log levels to
`Normal` before downgrading to avoid unexpected behavior during the transition.

Each component remains available during log level changes because the CPO uses rolling
restarts with proper pod disruption budgets.

## Version Skew Strategy

During an upgrade, the management cluster CPO may be at version N+1 while some hosted
control plane components are still at version N. This is safe because:

- The log level configuration is applied at the deployment/statefulset level by the CPO.
  The CPO at N+1 will set the verbosity flags on component containers regardless of their
  version.
- The `--v=N` flag and `ETCD_LOG_LEVEL` env var are stable across Kubernetes and etcd
  versions and are not version-specific.
- If the CPO at N+1 sets a log level field that a component at version N does not
  recognize, the flag is still a valid command line argument and the component will honor
  it.

## Operational Aspects of API Extensions

This enhancement adds optional fields to the existing `HostedCluster` and
`HostedControlPlane` CRDs. No new webhooks, aggregated API servers, or finalizers are
introduced.

**Impact on existing SLIs:**

- No impact at `Normal` log level.
- Higher verbosity levels increase log volume proportionally. This may affect log storage
  consumption but does not affect API availability, control plane latency, or scheduling
  performance at `Debug` or `Trace` levels under normal workloads.
- Expected use cases require short-lived non-default log level windows (minutes to hours
  during incident investigation), not permanent elevation.

**How impact is measured:**

- Log volume per component per verbosity level should be characterized by the QE team
  during validation testing.
- No performance team review required — this is a per-cluster operational configuration
  that does not affect management cluster API throughput or scheduling.

**Failure modes:**

- If an invalid log level is specified, CRD enum validation rejects the value at admission
  time. The component continues running at its current verbosity.
- If a log level change causes a failed rolling restart (e.g., resource constraints prevent
  new pod scheduling), the CPO reports a degraded condition on the HostedCluster. Standard
  rollback procedures apply — the previous pod generation remains running.
- If the CPO itself is unavailable during a log level change, the change is queued and
  applied on CPO recovery. No component restarts occur without CPO orchestration.

**Escalation:** The HyperShift / Hosted Control Planes team is responsible for issues
related to log level configuration.

## Support Procedures

**Detecting non-default log levels:**

Inspect the spec directly:

```bash
oc get hostedcluster my-cluster -o jsonpath='{.spec.operatorConfiguration}'
```

**Symptoms of excessive verbosity:**

Increased log volume in the management cluster namespace, potential log storage pressure,
and slightly increased CPU usage on control plane pods. Check pod log rates with:

```bash
oc logs -n <hcp-namespace> deployment/kube-apiserver --tail=100
```

**Resetting to defaults:**

Patch the HostedCluster CR to remove a specific component's log level override:

```bash
oc patch hostedcluster my-cluster --type=json -p \
  '[{"op":"remove",
    "path":"/spec/operatorConfiguration/kubeAPIServer"}]'
```

To reset all components at once:

```bash
oc patch hostedcluster my-cluster --type=json -p \
  '[{"op":"remove","path":"/spec/operatorConfiguration/kubeAPIServer"},
    {"op":"remove","path":"/spec/operatorConfiguration/etcd"}]'
```

**Graceful degradation:**

If log level configuration fields are removed or reset, the CPO restores default verbosity
on the next reconciliation. No data loss or cluster instability results from changing or
removing log level settings. The feature fails safely — components continue running at
their current verbosity until the CPO successfully applies the new setting.

**Disabling the feature:**

This enhancement does not introduce a webhook or aggregated API server that can be disabled
independently. Log level configuration is part of the standard CPO reconciliation loop. To
effectively disable the feature, reset all log level fields to `Normal` or remove them from
the HostedCluster spec.

## Future Work

A future enhancement may introduce a `defaultLogLevel` field in `OperatorConfiguration`
that sets a baseline verbosity for all components at once. When set, it acts as the default
for any component whose per-component `logLevel` is unset. Per-component `logLevel` fields
always take precedence over the default. The shared `LogLevel` type is used for both the
default and per-component fields, maintaining a unified API type regardless of
per-component translation differences (see [Drawbacks](#drawbacks) for the etcd mapping
tradeoff that makes this design possible).

## Implementation History

- 2026-06-09: EP created (provisional)
- 2026-07-22: EP updated to address API review feedback — feature gating, per-component
  wrapper types, Phase 3 removed
- 2026-07-26: Removed NonDefaultLogLevel status condition, removed E2E tests, removed Dev
  Preview phase, consolidated Goals into single delivery, updated feature gate name to
  HCPUserFacingOperatorLogs
- 2026-08-14: Corrected API types (value-type LogLevel, MinProperties=1 on all wrapper
  types), collapsed phase split (all 8 components delivered in single PR), updated test
  plan to per-component adapter tests, deferred annotation deprecation warning to Future
  Work, added etcd silent-clamp drawback, added defaultLogLevel Future Work direction

## Infrastructure Needed

No new infrastructure is required. This enhancement uses existing HyperShift CI
infrastructure and Prow job configurations for testing.
