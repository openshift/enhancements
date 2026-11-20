---
title: conmon-rs-default-container-monitor
authors:
  - "@saschagrunert"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on.
  - "@haircommander" # CRI-O runtime aspects, conmon-rs integration
  - "@yuqi-zhang" # MCO integration
  - "@jlebon" # RHCOS and node layer packaging
  - "@rphillips" # Node team oversight
  - "@csrwng" # HyperShift NodePool rendering
approvers:
  - "@mrunalp"
api-approvers:
  - "@JoelSpeed" # containerMonitor field of MachineConfiguration (operator.openshift.io/v1)
creation-date: 2026-06-05
last-updated: 2026-11-06
status: implementable
tracking-link:
  - https://issues.redhat.com/browse/OCPNODE-1288
see-also:
  - "/enhancements/machine-config/additional-storage-config-crio.md"
  - "/enhancements/machine-config/on-cluster-layering.md"
replaces:
superseded-by:
---

# Adopt conmon-rs as the Default Container Monitor in CRI-O

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes adopting [conmon-rs](https://github.com/containers/conmon-rs), a Rust-based pod-level
successor to the traditional C-based [conmon](https://github.com/containers/conmon), as the default container monitor
process in CRI-O on OpenShift. conmon-rs operates at the pod level rather than per-container, communicates with CRI-O
via Cap'n Proto over a Unix domain socket, and provides better resource usage, an async runtime, improved logging
capabilities, and a more maintainable codebase. The feature will be introduced behind a feature gate which adds a
`containerMonitor` field to the `MachineConfiguration` resource of the MCO. Tech Preview clusters opt in through the
field. At GA, conmon-rs becomes the default for new installations, while upgraded clusters keep conmon until they opt
in.

## Motivation

The current container monitor (conmon) is a C program that spawns one process per container. This per-container model
leads to increased resource consumption at scale and limits the ability to share state across containers within a pod.
conmon-rs addresses these limitations by running a single monitor process per pod, using an async Rust runtime with a
Cap'n Proto RPC interface.

CRI-O already supports conmon-rs upstream via the `runtime_type = "pod"` configuration, and the Go client library is
integrated. However, OpenShift has not yet switched to conmon-rs by default. The conmon-rs binary in the Node Layer is
still the 0.6.6 build of OCP 4.18, which all newer releases inherit, so it is effectively stale and not enabled.

Benchmarking (container density testing with conmon-rs optimizations from
[containers/conmon-rs#3267](https://github.com/containers/conmon-rs/pull/3267)) shows that conmon-rs reduces total
system memory by 15-42% compared to conmon across all tested workloads (light to heavy logging, 1 to 50 containers).
CRI-O itself uses 350-1,000 KB less memory per container when using conmon-rs due to Cap'n Proto RPC efficiency
compared to conmon's custom socket protocol. Under non-exec workloads, clusters can run up to 54% more containers per
node; mixed real-world clusters see 20-30% more capacity.

Establishing regular conmon-rs builds in the Node Layer (OCP-aligned versioning) and making it the default container
monitor will reduce per-node resource overhead, improve maintainability, move exec, attach and port forwarding
sessions to the pod-level monitor, and provide a foundation for future pod-level features such as in-pod hook
execution.

### User Stories

- As a cluster administrator, I want to opt in to using conmon-rs as the container monitor so that I can evaluate its
  resource efficiency and stability before it becomes the default. (During Tech Preview, opt-in requires enabling the
  `TechPreviewNoUpgrade` feature set, which enables all Tech Preview features cluster-wide, and setting the
  `containerMonitor` field.)

- As a platform engineer, I want CRI-O to use a single pod-level monitor process instead of one process per container
  so that per-node memory and CPU overhead is reduced at scale.

- As an SRE, I want to monitor the health of the container monitor subsystem through standard OpenShift metrics and
  alerts so that I can detect and remediate issues with conmon-rs in production.

- As a cluster administrator, I want the ability to fall back to the traditional conmon during the transition period so
  that I can maintain cluster stability (see [Reverting to conmon](#reverting-to-conmon)).

### Goals

1. Establish regular conmon-rs builds in the Node Layer, version-aligned to OCP releases, starting with OCP 5.1.

2. Allow cluster administrators to opt in to conmon-rs through the `containerMonitor` field of the
   `MachineConfiguration` resource, which the `ConmonRS` feature gate adds during Tech Preview.

3. Make conmon-rs the default container monitor in CRI-O at GA (see [GA: Default enablement](#ga-default-enablement)),
   replacing the per-container conmon model with the pod-level monitor.

4. Reduce per-node resource consumption (total RSS of monitor processes and total process count) compared to the
   current conmon-based setup.

5. Provide metrics and alerting for conmon-rs health: a CRI-O metric which reports the monitor type of each runtime
   handler, the CRI-O count of containers whose monitor exited, and an alert on that count (see
   [Metrics](#metrics)).

6. Serve exec, attach and port forwarding through the streaming server of conmon-rs (CRI-O's `stream_websockets`), so
   that the sessions of a pod are served by its monitor and survive a CRI-O restart.

### Non-Goals

1. Removing the traditional conmon binary from the node image. conmon will remain available as a fallback.

2. Changing the OCI runtime (runc/crun) selection. This enhancement only affects the container monitor, not the runtime
   that executes containers.

3. Exposing conmon-rs configuration options beyond selecting the monitor. Fine-grained conmon-rs tuning is out of scope
   for the initial implementation.

## Proposal

The proposal has two main parts:

1. **Packaging**: Establish regular conmon-rs builds in the Node Layer so the package is actively maintained and
   version-aligned to OCP releases. See [Packaging](#packaging) for details.

2. **Feature Gate and API**: Introduce a `ConmonRS` feature gate in `TechPreviewNoUpgrade` and, behind it, a
   `containerMonitor` field on the `MachineConfiguration` resource of the MCO (`operator.openshift.io/v1`, see
   [API Extensions](#api-extensions)). The MCO template controller renders the monitor options into the CRI-O base
   configuration of the master, worker and arbiter pools, for both runtime handlers it renders (`crun` and `runc`). An
   omitted field selects a default which depends on the release the cluster was installed with, like the OS image
   stream default of the MCO (`rhel-9` for clusters installed with 4.x, `rhel-10` for 5.x): conmon for clusters
   installed before the GA release, conmon-rs for clusters installed with it or later. At GA, the feature gate is
   promoted to the `Default` feature set.

### Workflow Description

**Cluster administrator** is a human user responsible for managing an OpenShift cluster.

**MCO (Machine Config Operator)** is the operator responsible for managing node configuration, including CRI-O
settings.

**CRI-O** is the container runtime interface implementation running on each node.

#### Tech Preview: Opt-in enablement

1. The cluster administrator enables the `TechPreviewNoUpgrade` feature set on the cluster. This alone changes nothing:
   during Tech Preview, an omitted `containerMonitor` field selects conmon.

2. The cluster administrator sets `spec.containerMonitor: ConmonRS` on the `MachineConfiguration` resource named
   `cluster`.

3. The MCO operator validates the field and records the effective monitor in `status.containerMonitorStatus`. The
   template controller re-renders the `01-<pool>-container-runtime` MachineConfigs of the master, worker and arbiter
   pools with the options of [MCO Changes](#mco-changes). The render controller checks the merged CRI-O configuration
   of each pool for options which CRI-O rejects together with conmon-rs and degrades the pool instead of rendering if
   it finds any. Otherwise the MCO rolls out the change to all nodes, draining and rebooting them.

4. On each updated node, CRI-O starts using conmon-rs for all pods of the `crun` and `runc` handlers. Because the MCO
   drains workloads and reboots the node, all pods (including static pods such as etcd and kube-apiserver on control
   plane nodes) are recreated after reboot, so no pod of these handlers keeps conmon. Handlers of other MachineConfigs
   (e.g., kata with `runtime_type = "vm"`) keep their configuration.

5. The cluster administrator verifies conmon-rs operation through CRI-O logs, the metrics of [Metrics](#metrics) and
   pod health checks.

#### GA: Default enablement

This follows the OS image stream default of the MCO, which derives the default stream from the release a cluster was
installed with, rather than the runc-to-crun migration of OCP 4.18, where the MCO created override MachineConfigs
(`00-override-<pool>-generated-crio-default-container-runtime`) which administrators had to delete, tracked by a
ConfigMap.

1. Once the `ConmonRS` feature gate is promoted to the `Default` feature set, the `containerMonitor` field is served on
   all clusters, and the default for an omitted field becomes conmon-rs for clusters installed with the GA release or
   later. **New installations** get conmon-rs from the first boot on, since the bootstrap render of the MCO applies the
   same default.

2. **Upgraded clusters retain conmon.** The MCO reads the install version of the cluster from the `ClusterVersion`
   history (the oldest completed update, as the OS image stream default reads it). A cluster installed before the GA
   release resolves an omitted field to conmon, which `status.containerMonitorStatus` shows. Administrators opt in to
   conmon-rs by setting `spec.containerMonitor: ConmonRS`, and revert by setting `Conmon`. No MachineConfig has to be
   created or deleted.

3. In a subsequent OCP release (after conmon-rs has been the default for new installations for at least one minor
   release cycle), the MCO drops the install version rule, so that an omitted field selects conmon-rs on every cluster.
   Administrators who want to keep conmon set `spec.containerMonitor: Conmon` before that upgrade.

### API Extensions

The `ConmonRS` feature gate adds a `containerMonitor` field to the spec of the `MachineConfiguration` resource of the
MCO (`machineconfigurations.operator.openshift.io`, singleton `cluster`), where the MCO keeps its other cluster-wide
knobs (`managedBootImages`, `bootImageSkewEnforcement`), and a `containerMonitorStatus` field to its status:

```go
// containerMonitor selects the container monitor process which CRI-O uses for the
// runtime handlers that the Machine Config Operator renders, crun and runc.
// Allowed values are Conmon and ConmonRS.
// Conmon runs one conmon process per container. ConmonRS runs one conmon-rs
// process per pod, which also serves exec, attach and port forwarding.
// When omitted, clusters installed with OpenShift <GA release> or later use ConmonRS
// and clusters installed with an earlier release use Conmon.
// Changing the value renders a new CRI-O configuration, which drains and reboots
// the nodes of all pools.
// +openshift:enable:FeatureGate=ConmonRS
// +optional
ContainerMonitor ContainerMonitor `json:"containerMonitor,omitempty"`

// containerMonitorStatus reflects the container monitor which the Machine Config
// Operator renders into the CRI-O configuration, after resolving an omitted
// spec.containerMonitor to the default of the cluster.
// +openshift:enable:FeatureGate=ConmonRS
// +optional
ContainerMonitorStatus ContainerMonitor `json:"containerMonitorStatus,omitempty"`

// ContainerMonitor is the container monitor process of CRI-O.
// +kubebuilder:validation:Enum=Conmon;ConmonRS
type ContainerMonitor string

const (
	// ContainerMonitorConmon selects conmon, one process per container.
	ContainerMonitorConmon ContainerMonitor = "Conmon"
	// ContainerMonitorConmonRS selects conmon-rs, one process per pod.
	ContainerMonitorConmonRS ContainerMonitor = "ConmonRS"
)
```

During Tech Preview, no release has the conmon-rs default yet, so the field documentation states that an omitted field
selects Conmon; the GA promotion fills in the release. No field is added to `ContainerRuntimeConfig`, which is per
pool and absent on most clusters, see [Alternatives](#add-a-containermonitor-field-to-containerruntimeconfig).

### Topology Considerations

#### Hypershift / Hosted Control Planes

conmon-rs runs on worker nodes as part of CRI-O. Since Hypershift separates the control plane from worker nodes, this
change only affects the data plane (worker nodes in the hosted cluster). The hosted control plane components are not
affected. HyperShift renders the NodePool configuration through the bootstrap render path of the MCO, which receives
the feature gate handler
([openshift/machine-config-operator#6622](https://github.com/openshift/machine-config-operator/pull/6622)), so the
templates render the default of the release without HostedCluster API changes. HyperShift has no `MachineConfiguration`
resource, so exposing the choice in the NodePool API is out of scope.

HyperShift NodePools switch to conmon-rs on the upgrade to the GA release, like they switched to crun on the upgrade
to 4.18 (OCPBUGS-69418). The MCO adds no MachineConfig for them. A NodePool which has to keep conmon either stays on
the previous release or gets a CRI-O drop-in from its administrator in `spec.config`, see
[Reverting to conmon](#reverting-to-conmon). During Tech Preview, a NodePool of a hosted cluster with
`TechPreviewNoUpgrade` opts in the same way, with a drop-in that restates the `crun` and `runc` handler tables of the
template with the options of [MCO Changes](#mco-changes).

#### Standalone Clusters

This enhancement is fully relevant and is the primary target topology. All standalone cluster nodes (control plane and
workers) will use conmon-rs when the feature is enabled.

#### Single-node Deployments or MicroShift

conmon-rs is beneficial for single-node deployments where resources are constrained, since it reduces per-node process
count and total system memory through fewer monitor processes and Cap'n Proto RPC efficiency in CRI-O.

MicroShift uses CRI-O but does not use the MCO or `MachineConfiguration`. MicroShift's CRI-O configuration is managed
through its own configuration file (`/etc/crio/crio.conf.d/`). conmon-rs enablement for MicroShift would require
updating MicroShift's default CRI-O drop-in with the options of [MCO Changes](#mco-changes) for its runtime handlers,
and ensuring the conmon-rs package is included in the MicroShift RPM dependencies. This is out of scope for this
enhancement and should be tracked in a separate MicroShift Jira once conmon-rs reaches GA in OCP.

#### OpenShift Kubernetes Engine

This enhancement does not depend on features excluded from OKE. conmon-rs is a node-level component that is part of
the CRI-O runtime stack, which is included in OKE. The feature gate and the field are available in OKE.

### Implementation Details/Notes/Constraints

#### Feature Gate

A new feature gate `ConmonRS` must be added to
[openshift/api features.go](https://github.com/openshift/api/blob/master/features/features.go) in the
`TechPreviewNoUpgrade` feature set, following the builder pattern:

```go
FeatureGateConmonRS = newFeatureGate("ConmonRS").
    reportProblemsToJiraComponent("node").
    contactPerson("saschagrunert").
    productScope(ocpSpecific).
    enhancementPR("https://github.com/openshift/enhancements/pull/2034").
    enable(inTechPreviewNoUpgrade()).
    mustRegister()
```

#### CRI-O Changes

CRI-O already supports conmon-rs upstream. The runtime selection is controlled per runtime handler via two
configuration fields:

- `runtime_type`: `"oci"` uses traditional conmon, `"pod"` uses conmon-rs.
- `monitor_path`: path to the monitor binary. For `runtime_type = "pod"`, this must point to the conmon-rs binary
  (e.g., `/usr/bin/conmonrs`). An empty value makes CRI-O look the monitor up in `$PATH`.

When `runtime_type = "pod"` is set, CRI-O uses the `runtimePod` implementation (`internal/oci/runtime_pod.go`) which
spawns conmon-rs via the Go client library (`github.com/containers/conmon-rs/pkg/client`).

conmon-rs manages cgroups differently from conmon. When the infra container is created, CRI-O moves the conmon-rs
process into the pod's cgroup via `createContainerPlatform()` in `CreateContainer()`. This is transparent to the
administrator but changes the per-pod process tree layout.

With `stream_websockets = true`, CRI-O hands exec, attach and port forwarding to the streaming server of conmon-rs
instead of its own, which is not available with the traditional conmon. conmon-rs speaks the SPDY protocol of the
kubelet and all WebSocket protocol versions, and listens on `127.0.0.1` with one-time URLs, like the CRI-O streaming
server. The kubelet proxies the sessions as before. CRI-O checks the features of the conmon-rs binary at startup and
keeps its own streaming server for a binary without them. This needs CRI-O v1.37.3 or newer, see
[Packaging](#packaging).

CRI-O starts the conmon-rs servers with the `none` server log driver unless the runtime handler sets
`monitor_env = ["LOG_DRIVER=systemd"]`, in which case the servers log their operational output at the CRI-O log level
to the journal under the `conmonrs` syslog identifier, which the node journal of must-gather includes. With `none`,
only panic messages reach the journal. The MCO template sets it.

Some CRI-O options conflict with a `pod` runtime handler: CRI-O refuses to start with `no_sync_log = true` on a `pod`
handler (`ValidateNoSyncLog`) and with `stream_websockets = true` on an `oci` handler, and passes `no_pivot` only to
conmon. CRI-O merges the top-level options of the drop-ins field by field in lexical order, but decodes each
`[crio.runtime.runtimes.<name>]` table into a new handler: a drop-in of an administrator which sorts after `00-default`
and defines a handler replaces the handler of the MCO template, including `runtime_type = "pod"` and `monitor_path`, so
that handler runs with conmon. The MCO does not reset these options from its template, a later drop-in would win
anyway. Instead, the MCO refuses to render a configuration which CRI-O would reject, see [MCO Changes](#mco-changes).
Container checkpointing goes through the OCI runtime and is unaffected, OpenShift disables restoring
(`container_level_enabled = "checkpoint_only"` in the MCO base template).

CRI-O needs two changes for OpenShift, besides the MCO-generated configuration:

- **Restoring pods after a CRI-O restart.** CRI-O creates the runtime implementation of a container, which for a `pod`
  handler holds the conmon-rs client of its pod, on the first request for the container, and the other containers of
  a pod share the implementation of its infra container. After a CRI-O restart, none exists. The conmon-rs client
  connects to the running server of the pod instead of starting one when CRI-O creates the implementation of the infra
  container (`connectRunningServer` in the client), but a request for another container of the pod which comes first
  finds no pod implementation. Since [cri-o/cri-o#10500](https://github.com/cri-o/cri-o/pull/10500) (v1.37.3 through
  [cri-o/cri-o#10505](https://github.com/cri-o/cri-o/pull/10505)) such a container gets a `runtimePodGone`
  implementation, which fails exec and attach, where older versions panicked. CRI-O has to restore the pod
  implementation of every sandbox with a `pod` handler when it restores the sandbox at startup, before it serves
  requests, so that new exec, attach, port forwarding, status and stop requests work after a restart as they do with
  conmon. The [Test Plan](#test-plan) covers it.

- **Metrics.** A gauge which reports the `runtime_type` of each configured runtime handler, and counting the containers
  whose conmon-rs server exited in the existing stopped monitor counter, see [Metrics](#metrics).

The upstream `container-selinux` policy does not yet include a file context for `/usr/bin/conmonrs`. The traditional
conmon binary uses the `conmon_exec_t` label. A `container-selinux` update is required to add a file context entry for
`/usr/bin/conmonrs` (with `conmon_exec_t`) before conmon-rs can run under enforcing SELinux. This must be completed
before the Tech Preview ships.

#### MCO Changes

The MCO renders the CRI-O base configuration (`/etc/crio/crio.conf.d/00-default`) of the `01-<pool>-container-runtime`
MachineConfigs from its templates for the master, worker and arbiter pools (custom pools inherit the worker templates).
The template controller has no access to feature gates today;
[openshift/machine-config-operator#6622](https://github.com/openshift/machine-config-operator/pull/6622) hands the
feature gate handler to the template render, both for the in-cluster controller and for the bootstrap render which
installs and HyperShift use. The conmon-rs enablement builds on it:

1. **Operator.** The `MachineConfiguration` sync of the operator resolves an omitted `spec.containerMonitor` to the
   default of the cluster, from the install version in the `ClusterVersion` history (the oldest completed update, as
   `GetInstallVersion` of the OS image stream code reads it), and writes the result to `status.containerMonitorStatus`.
   An explicit value is written as is. The API server validates the enum. The operator syncs `MachineConfiguration`
   before it rolls out the controller deployment, so the status exists when a new controller starts. An empty status
   (the operator has not synced yet, or an older operator runs) neither blocks rendering nor means conmon-rs: the
   template controller resolves it with the same helper as the operator, from the spec and the install version, so
   that it never renders a monitor which the status later contradicts.

2. **Template controller.** The template controller takes a `MachineConfiguration` informer, like the `APIServer` one
   it has, and renders the monitor options of `status.containerMonitorStatus` into the base configuration when the
   `ConmonRS` feature gate is enabled; a status change re-renders. A feature gate change restarts the controller (the
   feature gate accessor of library-go exits the process by default), which renders at startup, so a gate flip needs
   no trigger of its own. For `ConmonRS`, the template renders the following on both the `crun` and the `runc` handler:
   ```toml
   [crio.runtime.runtimes.crun]
   runtime_type = "pod"
   monitor_path = "/usr/bin/conmonrs"
   monitor_env = ["LOG_DRIVER=systemd"]
   stream_websockets = true
   ```
   For `Conmon`, the template renders the handlers as today. Rendering both handlers means that the merged
   `default_runtime` of a pool does not matter: a `ContainerRuntimeConfig` with `defaultRuntime: runc`, or a
   MachineConfig with `default_runtime = "runc"`, selects a handler which already carries the options. Handlers which
   other MachineConfigs or ContainerRuntimeConfigs add (e.g., kata with `runtime_type = "vm"`) keep their
   configuration. `monitor_path` is set explicitly rather than relying on CRI-O's build-time default to ensure the path
   matches the Node Layer RPM regardless of how CRI-O was compiled.

3. **Bootstrap render.** The bootstrap render reads an optional `MachineConfiguration` manifest from the install
   manifests, like the `FeatureGate` manifest it already reads, and resolves an omitted field with the release version
   as install version. A new installation therefore gets its default from the first boot, and a day-1 opt-in through
   a manifest needs no second rollout.

4. **Render controller validation.** Like its check against runc on RHEL 10 (`DetectRuncInMachineConfig`), the render
   controller parses the CRI-O drop-ins of the merged MachineConfig of a pool in lexical order and tracks
   `runtime_type`, `monitor_path`, `no_sync_log` and `stream_websockets` per handler, starting a handler over whenever a
   drop-in defines its table, like CRI-O does (see [CRI-O Changes](#cri-o-changes)). It fails the render when a `pod`
   handler ends up with `no_sync_log = true` or an `oci` handler with `stream_websockets = true`, both of which CRI-O
   refuses to start with, or with the monitor binary of the other monitor (conmon on a `pod` handler fails the version
   check of CRI-O, conmon-rs on an `oci` handler runs with the arguments of conmon). The pool reports `RenderDegraded`
   with the MachineConfig, handler and option, and no node changes, since no rendered configuration gets produced.
   Enabling conmon-rs on a cluster whose MachineConfig carries `no_sync_log = true` therefore degrades the pools before
   the first node reboots, and the administrator either removes the option or sets `containerMonitor: Conmon` again. A
   handler which a drop-in of an administrator redefines without `runtime_type = "pod"` while the pool selects
   conmon-rs is a valid configuration which runs with conmon: the controller does not fail the render for it, since the
   drop-in may be intentional, but logs it and records an event on the `MachineConfigPool` naming the MachineConfig and
   the handler. The `container_runtime_crio_runtime_type` metric shows such handlers (see [Metrics](#metrics)). The
   drop-ins of the ContainerRuntimeConfig controller define no handler tables.

No override MachineConfigs, pin MachineConfigs or ConfigMaps are involved. The ContainerRuntimeConfig controller is
unchanged.

The conmon-rs binary path (`/usr/bin/conmonrs`) is defined by the conmon-rs RPM package spec.

#### Packaging

The conmon-rs RPM is already in the Node Layer but has not been built regularly since OCP 4.18. Regular builds must be
established so the package is actively maintained:
- The package must be built as part of the OCP release pipeline, version-aligned to OCP X.Y.
- The package ships alongside CRI-O, crun, and other node runtime components.
- The package version should be conmon-rs >= 1.2.2 for the OCP 5.1 release, with CRI-O >= v1.37.3. CRI-O main and
  v1.37.3 and newer vendor the conmon-rs v1.2.1 client, which works with the v1.2.2 server, and hand port forwarding to
  the streaming server of conmon-rs ([cri-o/cri-o#10458](https://github.com/cri-o/cri-o/pull/10458), backported in
  [cri-o/cri-o#10481](https://github.com/cri-o/cri-o/pull/10481)). Older v1.37 releases keep port forwarding on the
  CRI-O streaming server.

### Risks and Mitigations

**Risk**: conmon-rs introduces a new IPC protocol (Cap'n Proto over Unix sockets) that differs from the well-tested
pipe-based conmon protocol.

**Mitigation**: conmon-rs has been integrated into CRI-O upstream for multiple releases and has extensive upstream CI
coverage. The Tech Preview phase provides time to validate the protocol in OpenShift-specific workloads.

**Risk**: Pod-level monitoring (one process per pod) changes failure semantics. A conmon-rs crash affects all containers
in a pod, not just one.

**Mitigation**: This aligns with Kubernetes pod semantics where a pod is the atomic unit. conmon-rs is written in Rust,
which eliminates entire classes of memory safety bugs (buffer overflows, use-after-free, data races) that are common
sources of crashes in C-based conmon. When a conmon-rs process does exit unexpectedly, CRI-O does not kill the
containers of the pod, but they lose their stdio, like with a dead conmon: the read ends of their pipes and the
terminal masters close, so a container gets SIGPIPE on its next write and a terminal session leader gets SIGHUP. Exec,
attach and port forwarding fail, and CRI-O reports exit code 255 for a container which stops, since no exit file gets
written. The pod has to be recreated, its replacement gets a new conmon-rs process (see [Test Plan](#test-plan)).
CRI-O counts the affected containers and an alert reports them (see [Metrics](#metrics)).

**Risk**: A MachineConfig of an administrator redefines the `crun` or `runc` handler table, for example to add an
`allowed_annotations` entry. CRI-O replaces the handler with it, so the handler silently loses the conmon-rs options
and runs conmon, while `status.containerMonitorStatus` says `ConmonRS`.

**Mitigation**: The render controller logs it and records an event on the pool (see [MCO Changes](#mco-changes)), and
the `container_runtime_crio_runtime_type` metric shows the handler on `oci` (see [Metrics](#metrics)). The
administrator adds the options of the template to the table. The documentation of the field points this out.

**Risk**: A MachineConfig of an administrator carries an option which CRI-O rejects together with the new monitor
(`no_sync_log = true` with conmon-rs, `stream_websockets = true` with conmon). A node which boots into it comes up
without CRI-O.

**Mitigation**: The render controller refuses to render such a configuration and degrades the pool before any node
changes (see [MCO Changes](#mco-changes)). The message names the MachineConfig and option to fix.

**Risk**: After GA, clusters may need a way to revert if conmon-rs causes issues.

**Mitigation**: Upgraded clusters keep conmon until the administrator sets `containerMonitor: ConmonRS`. Any cluster
reverts by setting `containerMonitor: Conmon` (see [Reverting to conmon](#reverting-to-conmon)) without a version
downgrade. The conmon binary remains in the node image through at least two GA releases.

### Drawbacks

- Introducing a new container monitor alongside the existing one increases the testing surface. Both code paths (conmon
  and conmon-rs) must be tested and supported during the transition period.

- Cap'n Proto is a less common serialization format in the OpenShift ecosystem, which may increase the learning curve
  for debugging container monitor communication issues.

- The pod-level monitoring model changes operational assumptions. Support teams need training on the new architecture.

- The `containerMonitor` field is transitional: once conmon is removed, `Conmon` becomes an invalid value and the field
  stays as API surface (see [Removing a deprecated feature](#removing-a-deprecated-feature)).

## Alternatives (Not Implemented)

### Use conmon-v3 instead of conmon-rs

[conmon-v3](https://github.com/containers/conmon-v3) is a new Rust-based container monitor under the containers
organization that follows the traditional per-container model (like the original conmon) rather than the pod-level
model of conmon-rs. It is in early development (no stable release, `3.0.0-dev` version string) and targets the podman
use case. conmon-v3 was not chosen for this enhancement because CRI-O's existing conmon-rs integration uses the
pod-level model with Cap'n Proto RPC, which provides the process-count reduction and pod-level feature foundation that
motivate this proposal. conmon-v3 does not offer these architectural advantages for the CRI-O use case.

### Keep conmon as the default indefinitely

This avoids the transition cost but misses the resource efficiency gains and limits future pod-level features that
conmon-rs enables (in-pod hooks, stats collection).

### Generate override MachineConfigs from the ContainerRuntimeConfig controller

An earlier revision of this enhancement had the ContainerRuntimeConfig controller generate a
`00-override-<pool>-generated-crio-conmon-rs` MachineConfig with a drop-in for the default runtime handler when the
feature gate is enabled, and pin MachineConfigs plus a ConfigMap at GA which upgraded clusters delete to opt in, like
the runc pins of the 4.18 crun migration. It was replaced by the `MachineConfiguration` field for three reasons. The
queue of that controller is only fed by `ContainerRuntimeConfig` events, so on a cluster without such a CR, enabling
the feature gate renders nothing until the controller restarts. The controller does not know the merged
`default_runtime` of a pool, which a MachineConfig of an administrator can set to `runc`, so the drop-in could land on
the wrong handler. And its bootstrap render only runs for a `ContainerRuntimeConfig` CR, so installs and HyperShift
would not get it. The pin MachineConfigs and their ConfigMap caused bugs and confusion during the crun migration and
leave objects behind. Rendering both handlers from the templates, which run at bootstrap and in the cluster, and an
install version default in the API avoid all of it.

### Add a containerMonitor field to ContainerRuntimeConfig

A `containerMonitor` field could be added to the `ContainerRuntimeConfig` CR instead, gated behind the feature gate,
for per-MachineConfigPool control over the container monitor type. This was rejected because `ContainerRuntimeConfig`
is a per-pool CR which does not exist on most clusters, so it gives neither a cluster-wide default nor a place for the
install version default of GA, and its controller has the limitations of the previous alternative. The
`MachineConfiguration` singleton exists on every cluster and holds the other cluster-wide knobs of the MCO. A pool
which needs a different monitor than the cluster can still set it with a MachineConfig drop-in that restates the
handler tables, which the render controller validates like any other.

### Feature gate without an API field

The feature gate alone could switch the templates, so that enabling `TechPreviewNoUpgrade` enables conmon-rs and GA
enables it everywhere. `TechPreviewNoUpgrade` cannot be disabled once enabled, so this gives no way back during Tech
Preview short of a MachineConfig drop-in which restates the handler tables with conmon, and at GA it gives upgraded
clusters no choice at all without the pin MachineConfigs of the previous alternative. The field provides both with a
single `oc patch`.

### Enable conmon-rs via CRI-O configuration only

conmon-rs could be enabled solely through CRI-O drop-in files which administrators write as MachineConfigs, the way
HyperShift NodePools revert. Drop-ins alone give no cluster default and no automation at GA: new installations would
not get conmon-rs, and no later release could move the remaining clusters to it without each administrator acting. The
feature gate and the field add both.

### Switch existing pods without a drain

A drainless migration would switch running pods from conmon to conmon-rs in place. It would need CRI-O to track which
pods use which monitor and to manage the mixed state, for marginal benefit, since the CRI-O configuration change already
comes with the drain and reboot of the MCO.

### Keep conmon-rs builds in the RHCOS layer

Building conmon-rs only in the RHCOS layer (version-aligned to RHEL) would mean slower update cycles and potential
version mismatches with CRI-O. Since conmon-rs and CRI-O are tightly coupled (CRI-O vendors the Go client and relies on
the features of the server), regular OCP-aligned builds in the Node Layer are preferred.

## Test Plan

### Unit and integration tests

- **MCO template rendering**: Unit tests in openshift/machine-config-operator for the template controller and the
  bootstrap render, verifying the CRI-O base configuration for `Conmon`, `ConmonRS` and an omitted field with install
  versions before and after the GA release, with the `ConmonRS` feature gate enabled and disabled, for the master,
  worker and arbiter templates, and that the bootstrap render picks the field up from a `MachineConfiguration`
  manifest.

- **MCO status**: Unit tests for the operator sync which resolves `status.containerMonitorStatus` from the spec and
  the `ClusterVersion` history.

- **MCO render validation**: Unit tests for the render controller check with merged configurations which set
  `no_sync_log = true`, `stream_websockets = true`, `runtime_type` and `monitor_path` on the `crun` and `runc`
  handlers in different drop-ins and orders, and which redefine a handler table with and without `runtime_type`,
  verifying which combinations degrade the pool, which record an event, that the check applies the replacement
  semantics of CRI-O for handler tables, and that the message names the MachineConfig, handler and option.

- **CRI-O restore**: Integration tests in cri-o/cri-o which restart CRI-O with running pods of a `pod` handler and send
  the first request after the restart to a non-infra container.

### End-to-end tests

All e2e tests must carry the `[OCPFeatureGate:ConmonRS]` and `[Jira:"Node"]` labels. Because enabling conmon-rs
requires an MCO rollout (node drain and reboot), the tests are inherently `[Serial]`, `[Slow]`, and `[Disruptive]`. A
dedicated test suite should be created in openshift/origin (`test/extended/node/conmon_rs.go`) with corresponding
periodic Prow jobs for both `TechPreviewNoUpgrade` and `Default` variants.

**Enablement and basic lifecycle:**

- Set `spec.containerMonitor: ConmonRS` on a cluster with the `ConmonRS` feature gate. Verify that
  `status.containerMonitorStatus` reports `ConmonRS`, that the `01-<pool>-container-runtime` MachineConfigs carry the
  options for the `crun` and `runc` handlers, and that CRI-O logs the info-level
  `"Runtime handler ... is using conmon-rs version"` message for both after the MCO rollout completes. Verify that
  `ps aux | grep conmonrs` shows one conmon-rs process per running pod on the updated nodes.

- Run a pod and exercise the full container lifecycle: create, start, exec, attach, logs, stop, delete. Verify all
  operations succeed with conmon-rs as the container monitor.

- Run `kubectl exec`, `kubectl attach` and `kubectl port-forward` against a pod and verify that the conmon-rs process of
  the pod serves them. Restart CRI-O during an exec and a port forwarding session and verify that both keep working.
  After the restart, run a new exec, attach and port forwarding, read the status and stop containers of pods which
  existed before the restart, with the first request after the restart going to a non-infra container, and verify that
  all of them work.

- Run a multi-container pod (including init containers) and verify that a single conmon-rs process is used for the pod
  throughout the entire lifecycle.

- Checkpoint a container of a conmon-rs pod through the kubelet checkpoint API and verify that the archive gets
  written.

**Fallback and revert:**

- Set `spec.containerMonitor: Conmon` and verify with `crio status config` that both handlers use `runtime_type = "oci"`
  and the conmon `monitor_path`, and that nodes revert to the traditional conmon after the MCO rollout. Verify that
  pods created after the revert use conmon (one process per container). Set `ConmonRS` again and verify that conmon-rs
  is re-enabled.

- Apply a MachineConfig which defines the `crun` handler table with `runtime_type = "pod"` and `no_sync_log = true` to
  a cluster with conmon, then set `ConmonRS`. Verify that the pools report `RenderDegraded` naming the MachineConfig
  and option, and that no node reboots. Delete the MachineConfig and verify that the rollout proceeds. Repeat with
  `runtime_type = "oci"` and `stream_websockets = true` on a conmon-rs cluster which gets set to `Conmon`.

- Apply a MachineConfig which defines the `crun` handler table with an additional `allowed_annotations` entry and no
  `runtime_type` to a conmon-rs cluster. Verify that the pool records the event naming the MachineConfig and handler,
  that `container_runtime_crio_runtime_type` reports `oci` for `crun` after the rollout, and that pods of the `crun`
  handler use conmon. Add the options of the template to the table and verify that the handler uses conmon-rs and
  keeps the additional annotation.

**Upgrade and downgrade:**

- Upgrade a cluster to the release where conmon-rs is the default. Verify that `status.containerMonitorStatus` reports
  `Conmon` and that pods created after the upgrade still use conmon. Set `ConmonRS` and verify that after the MCO
  rollout, pods use conmon-rs.

- Install a cluster with that release and verify that the nodes use conmon-rs from the first boot, without a second
  MCO rollout after the installation.

- Upgrade a HyperShift NodePool to that release and verify that it uses conmon-rs, and that a NodePool with the revert
  MachineConfig in its `spec.config` keeps conmon.

- Downgrade a cluster from a version where conmon-rs is the default to one without the `ConmonRS` feature gate and
  verify that CRI-O uses conmon after the MCO of the older release renders its templates. Repeat it with a user-managed
  MachineConfig which sets `runtime_type = "pod"` on another handler, removed before the downgrade.

**Graceful shutdown:**

- Delete a pod with `terminationGracePeriodSeconds` set and verify that containers receive SIGTERM, preStop hooks
  execute, and containers exit gracefully before SIGKILL. Compare the behavior against a conmon baseline to confirm
  that conmon-rs forwards signals correctly at the pod level.

**Negative and failure cases:**

- Kill the conmon-rs process for a running pod and verify that the pod can still be stopped and deleted, that a
  replacement pod runs with a new conmon-rs process, and that `container_runtime_crio_containers_stopped_monitor_count`
  increases by the number of containers of the pod.

- In a custom pool of two nodes with `maxUnavailable: 1`, apply a MachineConfig which points `monitor_path` at a missing
  binary and verify that the first node stays NotReady and that the pool does not update the second one.

**Load testing:**

- Deploy 100+ pods per node on a conmon-rs-enabled cluster and measure:
  - Per-node total RSS of the monitor processes of the pods of the `crun` and `runc` handlers (conmon-rs vs. conmon
    baseline), with the pods of other handlers counted separately. Target: lower total RSS with conmon-rs due to one
    process per pod instead of one per container.
  - Per-node conmon-rs process count. Target: equal to the count of pods of these handlers (not their container count).
  - Container create, start, and stop latency at the P50, P95, and P99 percentiles. Target: no regression compared to
    conmon baseline.
  - Pod startup latency under load. Target: no regression compared to conmon baseline.
- Compare results against a conmon baseline on the same cluster configuration to validate the resource consumption
  improvements claimed in the Goals.

**Metrics:**

- Verify that `container_runtime_crio_runtime_type` reports `pod` for the `crun` and `runc` handlers on a conmon-rs
  node and `oci` on a conmon node, that `container_runtime_crio_containers_stopped_monitor_count` is scraped by the
  in-cluster monitoring stack, and that the `ContainerMonitorExited` alert fires after the conmon-rs process of a pod
  gets killed (see [Metrics](#metrics)).

- Verify that CRI-O exposes container operation metrics when conmon-rs is the active monitor:
  `container_runtime_crio_operations_total`, `container_runtime_crio_operations_latency_seconds`,
  `container_runtime_crio_operations_latency_seconds_total` and `container_runtime_crio_operations_errors_total`, and
  that values are consistent with the conmon baseline (no unexplained regressions).

## Graduation Criteria

Feature promotion requires meeting the automated regression testing thresholds of
[feature-zero-to-hero.md](/dev-guide/feature-zero-to-hero.md) on all supported platforms. Both `TechPreviewNoUpgrade`
and `Default` Prow job variants must exist so that tests continue running after promotion.

### Dev Preview -> Tech Preview

Not applicable. The feature will ship directly to Tech Preview, skipping Dev Preview (see
[Risks and Mitigations](#risks-and-mitigations)).

### Tech Preview (Target: OCP 5.1)

- [ ] Enhancement proposal approved
- [ ] conmon-rs >= v1.2.2 binary shipped in the Node Layer, version-aligned to OCP, with CRI-O >= v1.37.3
- [ ] `ConmonRS` feature gate registered in `TechPreviewNoUpgrade` feature set
- [ ] `containerMonitor` and `containerMonitorStatus` fields of `MachineConfiguration` merged in openshift/api
- [ ] MCO renders the CRI-O options (`runtime_type`, `monitor_path`, `monitor_env` and `stream_websockets`) on the
  `crun` and `runc` handlers from the field, at bootstrap and in the cluster, and the render controller blocks
  conflicting drop-ins
- [ ] CRI-O restores the pod implementation of conmon-rs pods after a restart
- [ ] End-to-end pod lifecycle works with conmon-rs (create, exec, attach, port forward, logs, stop, delete)
- [ ] At least 5 e2e tests passing in CI (OCPNODE-2149, OCPNODE-2150, OCPNODE-2151)
- [ ] `container-selinux` updated with file context for `/usr/bin/conmonrs` (labeled `conmon_exec_t`)
- [ ] CRI-O metrics of [Metrics](#metrics) exposed and enabled in the MCO template
- [ ] Documentation in openshift-docs

### Tech Preview -> GA

**Prerequisites:**

- [ ] conmon-rs has run in Tech Preview for at least one OCP minor release cycle
- [ ] Field feedback collected from Tech Preview users
- [ ] No P0/P1 bugs outstanding against conmon-rs
- [ ] Load testing confirms resource consumption improvements at scale (100+ pods per node)
- [ ] Security review completed

**GA Criteria:**

- [ ] `ConmonRS` feature gate promoted to `Default` feature set (see [GA: Default enablement](#ga-default-enablement))
- [ ] The MCO resolves an omitted `containerMonitor` to `ConmonRS` for clusters installed with the GA release or later
  and to `Conmon` otherwise, and the field documentation names the release
- [ ] Upgrade and downgrade testing validates seamless transition between conmon and conmon-rs
- [ ] Promotion testing thresholds of the guide met
- [ ] User-facing documentation updated for GA
- [ ] Support procedures documented for conmon-rs troubleshooting
- [ ] The `ContainerMonitorExited` alert and its runbook shipped with the MCO

### Removing a deprecated feature

Once conmon-rs is the universal default for all clusters (new and upgraded) for at least two OCP minor releases, the
traditional conmon can be deprecated:

- Announce deprecation of conmon as container monitor and of the `Conmon` value of `containerMonitor`.
- Before removing the conmon binary, the MCO must detect MachineConfigs whose merged CRI-O configuration has any
  runtime handler with `runtime_type = "oci"` or a conmon `monitor_path`, with the parser of the render controller
  check, and `containerMonitor: Conmon`. The MCO should block the upgrade during preflight checks with a clear error
  message directing the administrator to remove or change them. This prevents clusters from upgrading into a broken
  state where a handler references a binary that no longer exists.
- Remove the conmon binary from the node image. The `Conmon` value gets rejected by validation from then on.

CRI-O upstream will continue to support `runtime_type = "oci"` for the foreseeable future, since conmon remains the
default monitor in upstream CRI-O and in other distributions (e.g., Fedora, RHEL standalone). Removing the conmon
binary from the OpenShift node image does not affect upstream CRI-O's support for the OCI monitor path.

## Upgrade / Downgrade Strategy

### Upgrade

Upgraded clusters keep conmon through the install version default of an omitted `containerMonitor`, see
[GA: Default enablement](#ga-default-enablement). A cluster which set the field in Tech Preview cannot upgrade to the
GA release, like any cluster with `TechPreviewNoUpgrade`.

### Downgrade

On a full version downgrade (N+1 to N) to a release without the `ConmonRS` feature gate, the MCO of N renders its
templates, which use conmon, so the nodes revert as part of the downgrade and no MachineConfig has to be applied. The
`containerMonitor` field is unknown to the CRD of N and gets pruned. Any other MachineConfig, or HyperShift NodePool
`spec.config`, which sets `runtime_type = "pod"` or a conmon-rs `monitor_path` on a handler has to be removed before
the downgrade; `crio status config` on a node shows the merged result. After the downgrade, CRI-O uses conmon, so no
conmon-rs binary of N has to work with its CRI-O.

## Version Skew Strategy

The container monitor is a per-node component with no cross-node communication, so nodes which still use conmon and
nodes which already use conmon-rs can coexist during a rollout. CRI-O, which contains the client, and the conmon-rs
binary ship in the same OS image, which a node receives and reboots into at once, so client and server cannot skew
within a node.

## Operational Aspects of API Extensions

The `containerMonitor` field of `MachineConfiguration` is the only API extension. No webhooks, finalizers, or
aggregated API servers are introduced.

- **Health indicators**: `status.containerMonitorStatus` shows the monitor the MCO renders. Rendering problems,
  including the conflicting drop-ins of [MCO Changes](#mco-changes), show up as the `RenderDegraded` condition of the
  MachineConfigPool, update problems as its `NodeDegraded` and `Degraded` conditions. The `ContainerMonitorExited`
  alert reports monitor processes which exited under running containers (see [Metrics](#metrics)).

- **Impact on existing SLIs**: None. The field only affects CRI-O's container monitor selection, which is a node-local
  operation. Changing it rolls out a new configuration to all pools, with the drain and reboot of any MachineConfig
  change.

- **Failure modes**: An invalid value is rejected by the API server. A conflicting drop-in degrades the pool without
  changing a node. If the conmon-rs binary is missing or corrupt on a node, CRI-O fails to start after the reboot,
  since it checks the version of the binary at startup. The MachineConfigDaemon pod cannot run on that node either, so
  the node stays NotReady and counts against `maxUnavailable` of the pool. With the default of 1, the rollout stalls
  before it affects additional nodes; with a larger value, up to that many nodes can fail before it stalls. The
  operator reports the pool as degraded once its timeout for required pools expires.

- **Escalation**: Issues with conmon-rs should be escalated to the Node team (openshift/node Jira component). Issues
  with MCO configuration rollout should be escalated to the MCO team (openshift/machine-config-operator).

## Support Procedures

### Detecting conmon-rs issues

- **CRI-O logs**: When conmon-rs is active, CRI-O logs at startup with an info-level message per runtime handler:
  `"Runtime handler %q is using conmon-rs version: ..."` (from config validation), with the features of the binary.
  During pod creation, a debug-level message `"Using conmonrs version: ..."` is logged for each infra container.

- **Process listing**: On a node using conmon-rs, `ps aux | grep conmonrs` will show one conmon-rs process per running
  pod of the `crun` and `runc` handlers.

- **Metrics**: `container_runtime_crio_runtime_type` shows which handlers use conmon-rs,
  `container_runtime_crio_containers_stopped_monitor_count` and the `ContainerMonitorExited` alert show monitor
  processes which exited under running containers (see [Metrics](#metrics)).

- **Events**: Kubernetes events on pods will report failures in container creation if conmon-rs fails to start or
  crashes.

- **Crash diagnostics**: When conmon-rs exits unexpectedly, CRI-O notices it on the next call for the pod, which fails
  with a dial error naming the socket of the server (see [Risks and Mitigations](#risks-and-mitigations) for the effect
  on the containers). conmon-rs is compiled with `panic = "abort"` in release builds, so panics terminate immediately
  with SIGABRT rather than unwinding. The forked server has `/dev/null` as its stderr, so its panic hook writes the
  panic message to the journal instead (`journalctl -t conmonrs -p err`), also with the `none` server log driver. The
  server logs of the MCO template (see [CRI-O Changes](#cri-o-changes)) show the activity leading up to a crash. On
  RHCOS, systemd-coredump captures core dumps via the default `kernel.core_pattern` pipe; core dumps for conmon-rs can
  be inspected with `coredumpctl list` and `coredumpctl info` on the affected node.

### Metrics

CRI-O exposes its metrics on `127.0.0.1:9537`, which the kube-rbac-proxy static pod of the MCO serves to the in-cluster
monitoring stack. The MCO alert `RuncDeprecated` already builds on the CRI-O gauge
`container_runtime_crio_default_runtime`. conmon-rs adds to it:

- **Monitor type per handler**: a new CRI-O gauge `container_runtime_crio_runtime_type{handler, runtime_type}` with the
  value 1 per configured runtime handler, like `container_runtime_crio_default_runtime{runtime}`. It tells whether a
  node runs conmon-rs (`runtime_type="pod"`) or conmon (`oci`) on which handler, which the operation metrics cannot,
  since they look the same with both monitors.

- **Exited monitors**: the existing CRI-O counter `container_runtime_crio_containers_stopped_monitor_count{name}`
  counts containers whose monitor process exited while the container kept running. CRI-O increments it for conmon in
  `runtimeOCI.UpdateContainerStatus`; the pod runtime increments it for every running container of a pod whose
  conmon-rs server it finds gone. The MCO template adds `containers_stopped_monitor_count` and `runtime_type` to
  `metrics_collectors` of the CRI-O base configuration, which does not enable the former today.

- **Alert**: the MCO ships a `ContainerMonitorExited` alert in its PrometheusRule, next to `RuncDeprecated`:
  ```yaml
  - alert: ContainerMonitorExited
    expr: sum by (instance) (increase(container_runtime_crio_containers_stopped_monitor_count[1h])) > 0
    labels:
      severity: warning
    annotations:
      summary: "A container monitor process exited under running containers"
      description: >-
        CRI-O on {{ $labels.instance }} found the monitor process of running containers gone.
        The containers lost their stdio and have to be recreated. Check `journalctl -t conmonrs -p err`
        and `coredumpctl list` on the node.
  ```
  It covers conmon as well, where a dead conmon has the same effect on its container. A runbook in openshift/runbooks
  describes the recovery of [Risks and Mitigations](#risks-and-mitigations).

### Reverting to conmon

The `TechPreviewNoUpgrade` feature set cannot be disabled once enabled, but the field can be changed: a cluster reverts
with `spec.containerMonitor: Conmon` on the `MachineConfiguration` resource, which renders the base configuration of
today and rolls it out. No MachineConfig is involved.

HyperShift NodePools, which have no `MachineConfiguration`, revert with a MachineConfig in their `spec.config` whose
CRI-O drop-in sorts after `/etc/crio/crio.conf.d/00-default`, like `/etc/crio/crio.conf.d/99-container-monitor`, and
defines the `crun` and `runc` handler tables. CRI-O replaces a handler with the table of a later drop-in (see
[CRI-O Changes](#cri-o-changes)), so the drop-in needs no resets, but it has to repeat the fields of the template which
the handler should keep (`runtime_root`, `allowed_annotations`, and for `crun` `container_min_memory` and
`default_annotations`, from `templates/<role>/01-<role>-container-runtime/_base/files/crio.yaml` of the MCO release in
use), and it sets `runtime_type = "oci"` explicitly. Example for `crun`, `runc` works the same way:

```toml
[crio.runtime.runtimes.crun]
runtime_type = "oci"
runtime_root = "/run/crun"
allowed_annotations = [
    "io.containers.trace-syscall",
    "io.kubernetes.cri-o.Devices",
    "io.kubernetes.cri-o.LinkLogs",
]
container_min_memory = "512KiB"
default_annotations = {"run.oci.systemd.subgroup" = ""}
```

A drop-in which only sets the monitor options would drop the other fields of the handler. The replaced handler has no
`monitor_path`, so CRI-O looks conmon up in `$PATH`, and no `monitor_env` or `stream_websockets`.

This only changes the container monitor: a Tech Preview cluster stays on `TechPreviewNoUpgrade`, which blocks minor
version upgrades, and has no supported migration path to the GA release, like any cluster with that feature set.

### Consequences of reverting to conmon

- Reverting triggers an MCO rollout, which drains and reboots nodes. Running workloads are evicted during the drain
  and rescheduled to other nodes.
- After revert, per-node resource consumption will return to pre-conmon-rs levels.
- No data loss or consistency issues. The container monitor is stateless from the workload perspective.

## Infrastructure Needed [optional]

- Regular conmon-rs package builds in the OCP Node Layer build pipeline.
- CI jobs for conmon-rs-enabled clusters in the `TechPreviewNoUpgrade` variant.
- Jira Epic OCPNODE-1288 tracks the conmon-rs adoption work across CRI-O, MCO, and openshift/api.
