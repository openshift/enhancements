---
title: csi-volume-device-exporter-universal-discovery
authors:
  - "@rvagner"
reviewers:
  - "@jsafrane, for storage aspects, please review CSI device discovery and volume mapping"
  - "@bertinatto, for storage operator integration and deployment lifecycle"
  - "@openshift/openshift-team-storage, for overall storage architecture"
  - "@sinnykumari, for node-level concerns (hostPath, SCC, DaemonSet resource impact)"
  - TBD # ART reviewer for new image
  - TBD # monitoring/alerting reviewer
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2026-09-09
last-updated: 2026-09-21
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-3365
---

# CSI Volume Device Exporter: Universal Block Device Discovery for Storage Path Health

## Summary

Deploy `csi-volume-device-exporter` through the Cluster Storage Operator (CSO)
to identify the persistent volumes and claims affected by degraded or lost
storage paths. The exporter maps CSI volume handles to node block devices;
platform Prometheus combines this mapping with kube-state-metrics and
`node_exporter` storage metrics to produce alerts with PV/PVC context.

The exporter is deployed on all standalone OpenShift platforms with no
configuration API and no supported opt-out. The exporter observes storage; it
does not participate in volume I/O. Clusters without node-visible SAN storage
run the exporter but do not produce mappings or storage-path alerts.

## Motivation

Node-level multipath metrics identify unhealthy devices but do not identify the
PVCs using them. Administrators must correlate kubelet metadata, devices, and
Kubernetes objects manually during an incident. A managed mapping and alerting
pipeline makes that information available before and during an outage.

### User Stories

- As a cluster administrator, I want path degradation alerts to identify affected
  PVs, PVCs, and nodes so I can involve storage and application owners promptly.
- As a support engineer, I want to correlate storage connectivity failures with
  workloads without inspecting each node manually.
- As a platform operator, I want deployment, upgrades, and health monitoring of
  the exporter to follow the cluster lifecycle without any configuration step.

### Goals

- Identify affected PVCs for device-mapper multipath and native NVMe-oF path
  degradation or loss, including supported LUKS device stacks.
- Provide the mapping and alerts automatically on all standalone OpenShift
  platforms with no configuration required.
- Detect exporter failures without affecting storage operations.

### Non-Goals

- Storage remediation, I/O performance monitoring, or replacing node path metrics.
- Non-CSI volumes, CSI inline ephemeral volumes, or network filesystems such as
  NFS and CephFS that do not expose a local block device.
- Visibility into SAN paths hidden behind a hypervisor or cloud storage service.
- A new dashboard, MicroShift integration, or initial hosted control plane support.
- An administrator-facing enable/disable control; deployment is unconditional.

## Proposal

CSO will manage a DaemonSet, security resources, monitoring configuration, and
alert rules. Each Linux node, including control-plane nodes, runs one exporter
pod. The exporter reads local kubelet metadata and sysfs, without Kubernetes API
access. Prometheus supplies PV/PVC metadata through kube-state-metrics.

CSO always deploys the exporter and its monitoring resources on all eligible
Linux nodes of standalone clusters. There is no configuration API. Deleting or
scaling the DaemonSet only causes CSO to reconcile it back to the desired state.
This avoids a new API and its compatibility burden, at the cost of running the
component on every standalone cluster regardless of whether it uses SAN storage.

Implementation spans the exporter, CSO, release/ART image configuration, CI, and
monitoring integration. The release must also provide the required node storage
collectors and retain their metrics and join labels. Availability of those
dependencies must be verified with the Monitoring team; shipping the exporter
alone does not enable the alerts.

### Workflow Description

1. A **cluster administrator** provisions CSI-backed block storage on a supported
   standalone cluster. Filesystem and raw block volume modes are both in scope.
2. **CSO** reconciles the exporter and monitoring resources unconditionally.
3. The **exporter** discovers node-local mappings every 30 seconds. Platform
   **Prometheus** scrapes them every 30 seconds and evaluates the correlation rules.
4. When paths degrade or are lost, the administrator receives an alert identifying
   the node, device or subsystem, PV, and, where bound, PVC name and namespace.
   The PVC identifies the workloads to inspect; pod labels are not added to alerts.
5. The administrator follows the linked runbook, checks storage connectivity, and
   verifies recovery. Discovery and alert evaluation resume automatically after
   exporter restarts or node reboots.

### API Extensions

None. This option intentionally introduces no CRD, field, webhook, aggregated
API server, or finalizer. CSO deploys the exporter and its monitoring resources
on all eligible Linux nodes of standalone clusters using its existing
reconciliation, without reading any new configuration. Administrators cannot
disable the component through the API.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Deferred in the initial implementation. Hosted control planes already support
[bare-metal workers](https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/hosted_control_planes/deploying-hosted-control-planes),
so this is an integration scope decision rather than a storage-device
limitation. CSO runs in the management cluster for HyperShift, while this
node-level DaemonSet and its metrics must run in and be scraped from the guest
cluster. Initial delivery defers the required cross-cluster reconciliation,
guest monitoring integration, RBAC, status reporting, and version-skew
validation. The exporter itself would remain entirely in the guest data plane.

#### Standalone Clusters

The primary target. The exporter deploys on all eligible Linux nodes, including
control-plane nodes that can host CSI volumes, without inspecting the
infrastructure platform. Linux node selection and tolerations must cover nodes
that can host CSI volumes.

#### Single-node Deployments or MicroShift

SNO uses one exporter pod and requires no quorum or replicas. The prototype
requests 10m CPU / 32Mi memory and limits usage to 100m CPU / 64Mi memory per pod.
Requests reserve predictable capacity so the DaemonSet can run on a constrained
SNO node alongside control-plane and workload pods. The prototype limits bound
unexpected use while the component is being evaluated. These are manifest
settings, not measured overhead; scale and SNO tests must establish production
sizing. Production limits require platform resource review because cluster
components normally use requests without limits. Reboots temporarily interrupt
monitoring. Because there is no opt-out, the SNO resource cost applies to every
SNO cluster.

MicroShift is out of scope: this deployment relies on CSO and the OpenShift
platform monitoring stack rather than MicroShift's component lifecycle.

#### OpenShift Kubernetes Engine

The same standalone policy applies. OKE includes
[cluster monitoring](https://access.redhat.com/support/offerings/openshift-engine/sla);
user workload monitoring and other OCP-only features are not required.

### Implementation Details/Notes/Constraints

#### Deployment and lifecycle

Use `openshift-csi-volume-device-exporter` as the proposed namespace. CSO owns
the DaemonSet, service account, SCC access, scrape configuration, and
PrometheusRule. It always maintains both the exporter and its monitoring
resources; there is no state in which they are removed on a managed cluster.

Integrate the image into CSO's payload image references and release builds.
The local `Dockerfile.openshift` uses a RHEL base; the upstream distroless image
is not the proposed production build. Follow the
[new-component policy](https://github.com/openshift/enhancements/blob/master/dev-guide/new-components.md), including maintainer
and CVE-response requirements. Deployment also depends on CSO being present
through the Storage capability.

The prototype uses a PodMonitor and HTTP port 9710. Production integration must
use platform monitoring, with the namespace selection and scrape RBAC it
requires. The namespace label `openshift.io/cluster-monitoring=true` alone does
not establish a secure, working scrape. Agree the TLS/authentication mechanism
and any Service/ServiceMonitor changes with Monitoring and Security before
implementation approval. Keep the scrape job label consistent with health rules.

#### Discovery and coverage

The current implementation runs kubelet discovery first, then optional Trident
and HPE readers to fill gaps:

- **Filesystem volumes:** read
  `pods/<uid>/volumes/kubernetes.io~csi/<volume>/vol_data.json` beneath the kubelet
  root, then use the mounted filesystem's device number and `/sys/dev/block`.
- **Raw block volumes:** inspect
  `pods/<uid>/volumeDevices/kubernetes.io~csi/<volume>/<volume>` and read metadata
  from `plugins/kubernetes.io~csi/volumeDevices/<volume>/data/vol_data.json`.
- **Layered devices:** follow sysfs through supported LUKS layers to the
  underlying multipath device or NVMe namespace, retaining the multipath device
  rather than selecting one of its individual paths.

Kubelet discovery covers volumes published to pods. Driver metadata may also
expose staged volumes. Discovery does not guarantee coverage for every CSI
driver, arbitrary device-mapper stacks, or unattached PVs. Validate driver,
protocol, volume mode, and encryption combinations explicitly. Missing or
ambiguous mappings must not be presented as healthy storage.

#### Behavior on non-multipath deployments

The discovery pipeline is device-agnostic and has no multipath-specific code in
its core logic. On nodes without device-mapper multipath, the exporter operates
normally and the mapping gauge is emitted identically:

- **Physical block devices** (`/dev/sda`, `/dev/nvme0n1`, `/dev/vda`, etc.) are
  returned directly from sysfs without any multipath traversal.
- **Device-mapper targets** (`dm-X`): the exporter inspects the device UUID. If
  the UUID does not carry the `mpath-` prefix it is not a multipath device; the
  exporter walks the slave devices to the underlying physical device and uses
  that. Only devices whose UUID carries `mpath-` are treated as multipath.
- **Network filesystems** (NFS, CephFS): automatically excluded because their
  device major number is 0, which has no `/sys/dev/block` entry; no explicit
  filter is needed.

The result is that `csiaddons_volume_node_device_info` is populated for all
node-visible CSI block devices regardless of whether multipath is configured.
On clusters without multipath or NVMe-oF, `node_dmmultipath_device_info` and
the NVMe subsystem metrics are absent, so the four storage-path alert rules
(`CSIAddonsVolumeMultipath*` and `CSIAddonsVolumeNVMeSubsystem*`) never fire.
The exporter-health alerts (`CSIAddonsVolumeDeviceExporterDown` and
`CSIAddonsVolumeDeviceExporterNodeDown`) remain active on all standalone
clusters because the exporter is always expected to be present.

Before release, deduplicate by `(driver, volume_handle)` per node, rather than
volume handle alone as in the prototype. Use the same identity across all
readers, normalize device names, and prevent stale driver metadata from
reattributing a reused device to an old volume.

#### Metrics and correlation

The mapping gauge has value 1:

```text
csiaddons_volume_node_device_info{node="worker-1", volume_handle="volume-id", driver="csi.example.com", device="dm-5"} 1
```

Recording rules must construct these joins, retaining node identity throughout:

| Relationship | Metric and join keys |
| --- | --- |
| Mapping → PV | `kube_persistentvolume_info`: normalize `csi_driver` and `csi_volume_handle` to `driver` and `volume_handle`. |
| PV → PVC | `kube_persistentvolumeclaim_info`: match its `volumename` to `persistentvolume`, retaining claim name and namespace. |
| Kernel dm device → multipath name | `node_dmmultipath_device_info`: match exporter `device` to `sysfs_name`, plus `node`. |
| NVMe namespace → subsystem | `node_nvmesubsystem_namespace_info`: match `device` and `node`. |
| Device/subsystem → health | DM path-state and active-path metrics; NVMe total-path and live-path metrics. |

The PV metadata keys are documented by
[kube-state-metrics](https://github.com/kubernetes/kube-state-metrics/blob/main/docs/metrics/storage/persistentvolume-metrics.md).
Verify all metrics, retained labels, and node-label normalization against the
target OpenShift release. Deduplicate scrape replicas and join multiple
node-local mappings to a unique PV metadata series on the right. Joining only
on a volume handle, as the prototype does, can collide across drivers or fail
when a volume appears on multiple nodes. Preserve PV alerts without PVC labels
when no current claim binding exists.

The exporter also exposes:

| Metric suffix after `csiaddons_volume_device_exporter_` | Type / purpose |
| --- | --- |
| `discovery_errors_total{discoverer}` | Counter of discoverer-level failures |
| `volumes_discovered{driver}` | Gauge of discovered mappings per driver |
| `last_successful_discovery_timestamp_seconds` | Gauge of last successful kubelet discovery cycle |

A successful scan can skip individual unreadable volumes; these health metrics
do not prove complete mapping coverage. Alert rules must exclude stale discovery
snapshots. Removal of a mapping or disappearance of a path metric is missing
information, not evidence of recovery or a zero path count.

#### Alerts

The proposed rules retain the prototype names and timings:

| Alert | Severity / `for` | Condition |
| --- | --- | --- |
| `CSIAddonsVolumeMultipathDegraded` | warning / 5m | Failed paths with at least one usable path remaining |
| `CSIAddonsVolumeMultipathLost` | critical / 1m | Zero usable paths reported |
| `CSIAddonsVolumeNVMeSubsystemDegraded` | warning / 5m | Non-live controller paths with at least one live path remaining |
| `CSIAddonsVolumeNVMeSubsystemLost` | critical / 1m | Zero live controller paths reported |
| `CSIAddonsVolumeDeviceExporterDown` | warning / 5m | No exporter targets discovered while deployment is expected |
| `CSIAddonsVolumeDeviceExporterNodeDown` | warning / 10m | Expected node target missing, unreachable, or discovery stale |

The prototype storage alerts contain PV context only. Add PVC enrichment and
correct join cardinality before shipping. Make degraded/lost conditions
exclusive. Extend health rules to detect a missing node target even when other
nodes remain healthy; `up == 0` alone cannot do this. Nodes without relevant
volumes or path metrics must not generate storage-path alerts. Because the
exporter is always expected on standalone clusters, the exporter-down alerts are
always armed.

### Risks and Mitigations

- **Host access:** UID 0 is needed for kubelet metadata written with mode `0600`.
  Use a dedicated SCC/service account, drop all capabilities, disable privilege
  escalation and token automounting, use `RuntimeDefault` seccomp, and prohibit
  privileged containers and host PID/IPC/network access.
- **Workload data exposure:** the exporter mounts kubelet directories that sit
  alongside pod volume data. A misconfigured mount could expose secrets or
  volume contents of running workloads. Mount only the required paths, use
  read-only mounts with `HostToContainer` propagation, and restrict the root
  filesystem. Note that nested mounts are not necessarily made read-only by the
  parent flag.
- **Insufficient security confinement:** the prototype's SELinux level `s0` and
  size-limited reads are not sufficient isolation proofs. Security review must
  validate effective mount permissions, SELinux access, symlink handling, and
  actual access to workload data before default deployment. Because there is no
  opt-out, this host access exists on every standalone cluster, which raises the
  bar for the security review.
- **Prototype defect — optional driver paths:** the prototype uses
  `DirectoryOrCreate` for Trident tracking directories, which actively creates
  host directories even when the container has read-only root filesystem access.
  This must be fixed before any default deployment: missing driver files must not
  prevent pod startup or require broadening access to host `/var/lib`.
- **Incorrect or incomplete alerts:** test mapping churn, encryption, missing
  metrics, and path failures. Unsupported device stacks must be documented.
- **Device disappearance breaks alert correlation:** if a block device disappears
  entirely from sysfs (complete path loss), the mapping metric disappears with
  it. The PVC label is lost exactly when a path-loss alert is most needed.
  Alert rules must account for this: absence of a mapping is not evidence of
  recovery and must not suppress a path-loss condition.
- **Resource cost:** measure scanning overhead, scrape cardinality, and
  rule-evaluation cost at scale before setting production resource limits.

### Drawbacks

This adds a payload image, one pod per eligible Linux node, and monitoring
series on every standalone cluster, including clusters with no relevant storage,
with no supported way to turn it off. It also couples supported behavior to
private kubelet/driver metadata and node collector metrics. Mandatory deployment
keeps the design simple and avoids a new API, but removes administrator control
and concentrates the resource and security cost on every cluster.

## Alternatives (Not Implemented)

- **API opt-out:** adds a `Storage` field so administrators can disable the
  component, at the cost of a new API and its compatibility burden. 
- **Kubelet metric:** could remove the extra daemon and host mounts, but needs
  upstream design and implementation. Kubelet does not necessarily know the full
  driver-specific device stack. The closest existing upstream work is KEP-606,
  which exposes device plugin resources (GPUs, accelerators) via the pod-resources
  gRPC endpoint, but does not cover CSI volume-to-block-device mappings. No KEP
  currently proposes exposing the `(CSI volume handle → block device)` mapping
  from the kubelet. Reconsider the exporter if an equivalent metric becomes
  available, but do not wait for it: proposing and landing such a KEP is a
  multi-cycle effort with no guaranteed outcome.
- **CSI driver metrics or RPC changes:** use driver knowledge directly but require
  adoption across vendors. Enriching `VolumeAttachment` also needs node-side data
  collection and conversion into metrics.
- **Extend node_exporter or use its textfile collector:** avoids a separate metrics
  endpoint but still needs Kubernetes-specific discovery, host access, and managed
  updates. The textfile approach additionally needs stale-file cleanup.
- **OLM add-on:** avoids a payload component and decouples releases, but requires
  administrator installation and an agreed way to evaluate joins across platform
  and exporter metrics. Separate Prometheus instances do not make those joins
  automatic.
- **Other lifecycle owner:** a standalone operator adds overhead; the snapshot
  operator has a different responsibility. CSO already owns storage deployment.
- **Platform-gated deployment:** unlike API opt-out (which gives administrators
  an explicit switch), platform-gating would auto-suppress the exporter based on
  infrastructure type (e.g. skip on AWS, deploy on bare metal). This is rejected
  because infrastructure platform is not a reliable proxy for storage topology:
  SAN storage can be present on cloud and virtualized platforms, and absent on
  bare-metal ones. Silently skipping the exporter based on platform type would
  miss real use cases without any administrator awareness.
- **Runtime device detection:** detection itself needs a node agent and must
  handle devices appearing after detection runs. It adds lifecycle complexity
  without removing the need for the exporter to be present before an incident.

## Open Questions

Before marking this proposal implementable:

1. Verify availability of all required node collectors and kube-state-metrics
   labels in the target release; identify any prerequisite monitoring changes.
2. Agree production endpoint security, SCC/SELinux confinement, and optional
   driver-path mounting without unnecessary host access or directory creation.
   The absence of an opt-out makes this review a hard gate.
3. Agree on the minimum validation baseline: at least one driver/protocol/
   encryption combination must be verified end-to-end before implementation
   approval. 

## Test Plan

- **Unit tests** (developer-owned, written alongside the code)
  - Discovery logic: metadata parsing, filesystem and raw block volume discovery,
    LUKS traversal, conflicting driver handles, stale metadata, device reuse, and
    malformed or unreadable files.
  - Container mounts: verify that a missing or changed kubelet mount propagation
    mode produces a diagnostic skip rather than a silent gap.

- **Integration tests** (automated, run in presubmit CI against static rule files)
  - Prometheus recording rules and alerts: use `promtool` to test every join and
    alert rule, covering multiple nodes per volume, shared devices, absent PVCs,
    duplicate scrape series, missing upstream metrics, and recovery after a gap.

- **Operator tests** (automated, require a running cluster)
  - Verify default deployment on every supported standalone platform and confirm
    that unsupported topologies are handled gracefully.
  - Confirm SCC admission and CSO status reporting.
  - Verify that the exporter scrape endpoint is reachable from platform Prometheus
    with the agreed TLS and authentication configuration.
  - Verify that directly deleting or scaling the DaemonSet is reconciled back to
    the desired state.
  - On clusters with no relevant block storage, confirm that neither
    exporter-down nor storage-path alerts fire.

- **End-to-end tests**
  - *Extend existing CSI e2e tests* — volume lifecycle (create, remove, remount,
    pod restart, node reboot) is already covered by existing tests. Add metric
    and alert assertions on top: verify that `csiaddons_volume_node_device_info`
    appears with correct labels, updates on remount, and disappears on teardown.
    No new test infrastructure is needed for this part.
  - *Path failure scenarios* — check the existing OpenShift CSI e2e suite for path
    failure tests. Most probably these scenarios (partial path degradation, full path loss, and
    restoration) will require new tests with software iSCSI or NVMe-oF targets,
    plus the CI infrastructure to run them. For initial delivery these tests are
    expected to be manual.
  - *Manual or lab only* — complete device disappearance from sysfs (as opposed
    to a path error state) is hardware-dependent and cannot be reliably
    reproduced with software targets. 

- **Scale and lifecycle tests**
  - At least 100 PVCs per node at target cluster scale; measure CPU, memory, and
    scrape cardinality against prototype resource settings.
  - SNO: verify resource use and alert behavior with a single exporter pod.
  - Rollout and upgrade: confirm monitoring gaps during DaemonSet rollout are
    within documented bounds and do not produce spurious alerts.
  - Version skew: test old and new exporter against current node collectors and
    kube-state-metrics across the supported skew window.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Open questions resolved, and security and monitoring reviews complete.
- Managed deployment and PV/PVC alerts verified end to end for at least one
  documented driver/backend combination; unvalidated protocols remain excluded
  from advertised coverage.
- Automated happy-path and failure tests, user documentation, and support runbooks.

### Tech Preview -> GA

- Required end-to-end CI covers advertised protocols, volume modes, and encryption
  combinations, with multiple CSI drivers validated.
- Scale, SNO, upgrade, and skew results justify production resource settings and
  alert timings; missing-data behavior is documented.
- Default deployment enabled on all standalone platforms; openshift-docs and
  CEE guidance cover the always-on behavior, coverage limits, and troubleshooting.

### Removing a deprecated feature

No removal is proposed. Any later replacement requires a separate migration
and metric/alert deprecation plan.

## Upgrade / Downgrade Strategy

CSO rolls out the payload image using DaemonSet rolling updates. No persisted
exporter data requires migration. This option adds no API; upgrades always
deploy the component on eligible standalone clusters without manual action.

During rollout, mapping and alert coverage can be interrupted on individual
nodes. Alert `for` durations do not preserve missing series or guarantee
continuous detection; test and document this monitoring gap. Volume I/O is
independent of the exporter.

OpenShift cluster downgrades are not generally supported. Because there is no API
schema, there is no field to handle on downgrade; an older CSO simply manages the
exporter as its own version defines, and a CSO without the component leaves its
resources behind until removed.

## Version Skew Strategy

Kubelet volume files and driver tracking files are private implementation details,
not stable APIs. Test against kubelet versions allowed by the target OpenShift
release and during node upgrades. Tolerate unknown metadata fields, skip
unresolvable mappings, and expose diagnostic information.

Exporter metrics, node collectors, kube-state-metrics, and rule definitions must
remain compatible during rolling updates. Coordinate label/name changes with
Monitoring and test old/new combinations; version skew exists even though the
exporter makes no API calls.

## Operational Aspects of API Extensions

No API extensions are introduced. CSO deploys and reconciles the exporter and its
monitoring resources unconditionally. CSO must report exporter reconciliation
failures through identifiable Storage/ClusterOperator conditions. Per-node
monitoring failures use the health alerts; they must not stop CSI provisioning,
attachment, or mounts.

Track desired/available DaemonSet pods, scrape availability, discovery freshness,
and discovery errors. A zero mapping count may be valid. Measure CSO and
monitoring overhead at scale rather than assuming no impact on existing SLIs.

## Support Procedures

Check `oc get co storage`, exporter DaemonSet/pods and logs in
`openshift-csi-volume-device-exporter`, and platform scrape/rule errors.
For missing mappings, compare the PV's CSI driver/handle with exporter labels;
check discovery freshness, metadata access, mount propagation, and sysfs
resolution. Debug logs expose per-volume skips that may not increment the error
counter. Collect these diagnostics through must-gather without copying workload
contents or raw driver tracking files that may contain credentials.

For path alerts, use the node/device or subsystem and PV/PVC labels to inspect
node collector data and storage connectivity. No alert is not proof of healthy
storage when inputs are missing.

There is no supported disable operation. A DaemonSet cannot be scaled to zero,
and deleting the exporter or its resources directly only triggers CSO
reconciliation. Administrators who must stop the component have to change the
managed state of CSO itself, which affects all storage components and is not a
supported way to disable only the exporter.

## Infrastructure Needed

- Confirm the OpenShift source repository/fork and maintainers for the
  [upstream exporter](https://github.com/csi-addons/csi-volume-device-exporter).
- ART image configuration, CSO image references, and OpenShift CI onboarding.
- Reproducible multipath storage jobs, FC validation access, and approved alert
  runbook hosting.
