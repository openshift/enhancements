---
title: storage-path-alerting-with-pvc-context
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
last-updated: 2026-09-22
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-3365
---

# Storage Path Alerting with PVC Context

## Summary

OpenShift currently has no automatic way to tell a cluster administrator which
PVC—and therefore which workload—is affected when a storage path degrades or
fails. This enhancement adds multipath storage-path alerts with PVC context to
the platform monitoring stack, gated on the `StoragePathAlert` feature gate.

## Motivation

Node-exporter already exposes multipath and NVMe-oF path health metrics, but
those metrics identify devices by kernel name (`dm-5`, `nvme0n1`), not by PVC.
Administrators must correlate kubelet metadata, kernel devices, and Kubernetes
objects manually during an incident. The missing piece is a metric that maps
each CSI volume handle to its node-local block device, allowing Prometheus to
join path health to PV and PVC context automatically.

### User Stories

- As a cluster administrator, I want path degradation alerts to identify affected
  PVs, PVCs, and nodes so I can involve storage and application owners promptly.
- As a support engineer, I want to correlate storage connectivity failures with
  workloads without inspecting each node manually.
- As a platform operator, I want path-degradation alerts to be active
  automatically across the cluster lifecycle—including upgrades and node
  reboots—without any configuration step on my part.

### Goals

- Alert cluster administrators with PVC and node context when device-mapper
  multipath or NVMe-oF paths degrade or fail, including LUKS-encrypted stacks.
- Deliver these alerts automatically on all supported OpenShift platforms with
  no administrator configuration required.
- Keep storage I/O unaffected: the monitoring path must not participate in
  volume attach, mount, or I/O operations.

### Non-Goals

- Storage remediation, I/O performance monitoring, or replacing node path metrics.
- Non-CSI volumes, CSI inline ephemeral volumes, or network filesystems such as
  NFS and CephFS that do not expose a local block device.
- Visibility into SAN paths hidden behind a hypervisor or cloud storage service.
- Alerting on failure of a single non-multipath block device; path alerts
  require multipath or NVMe-oF health metrics from node-exporter.
- A new dashboard or MicroShift integration.
- A per-cluster opt-out beyond the `StoragePathAlert` feature gate. Once the
  gate is in `Default`, the alerts are unconditionally active on all eligible
  clusters.

## Proposal

CSO ships PrometheusRule alert definitions that fire with PV and PVC context
when a CSI volume's multipath or NVMe-oF storage paths degrade or fail. The
alert rules join three metric streams:

- [`node_exporter`](https://github.com/prometheus/node_exporter) (already
  present) — device health: multipath path state and active-path counts, NVMe
  subsystem controller paths.
- [kube-state-metrics](https://github.com/kubernetes/kube-state-metrics)
  (already present) — PV/PVC metadata: maps a CSI volume handle to its PV and
  bound PVC name and namespace.
- [`csi-volume-device-exporter`](https://github.com/csi-addons/csi-volume-device-exporter)
  (new) — the missing link: maps each CSI volume handle to the block device
  name visible to the node, so the other two streams can be joined.

CSO manages the exporter DaemonSet and the alert rules together, gated on the
`StoragePathAlert` feature gate. When the gate is enabled, CSO reconciles the
exporter and alert rules to the desired state. Path alerts fire only when multipath
or NVMe-oF health metrics from node-exporter are present; a single non-multipath
device becoming unavailableis not detected by these alerts.

### API Extensions

A new OpenShift feature gate `StoragePathAlert` is added to `openshift/api`
`features/features.go`:

```go
FeatureGateStoragePathAlert = newFeatureGate("StoragePathAlert").
    reportProblemsToJiraComponent("Storage / Operators").
    contactPerson("rvagner78").
    productScope(ocpSpecific).
    enhancementPR("https://github.com/openshift/enhancements/pull/2108").
    enable(inTechPreviewNoUpgrade(), inDevPreviewNoUpgrade()).
    mustRegister()
```

No CRD, API field, webhook, aggregated API server, or finalizer is introduced.
The feature gate is the only extension.

### Topology Considerations

#### Hypershift / Hosted Control Planes

HyperShift hosted control planes on AWS, Azure, and GCP support CSI-backed
block storage, so path alerts must be available there as well. The exporter
DaemonSet and its metrics live entirely in the guest data plane. CSO runs in
the management cluster; the guest-side reconciliation, monitoring integration,
RBAC, and version-skew validation are in scope for implementation.

#### Standalone Clusters

The primary target. Path alerts are available on all eligible Linux nodes,
including control-plane nodes that can host CSI volumes.

#### Single-node Deployments or MicroShift

SNO uses one exporter pod. Resource requests and limits must be established by
scale and SNO testing before GA; initial values require platform resource
review because cluster components normally use requests without limits. Reboots
temporarily interrupt alert coverage. Because there is no opt-out, the resource
cost applies to every SNO cluster.

MicroShift is out of scope: the alerting stack here relies on CSO and the
OpenShift platform monitoring stack.

#### OpenShift Kubernetes Engine

The same policy applies. OKE includes
[cluster monitoring](https://access.redhat.com/support/offerings/openshift-engine/sla);
user workload monitoring and other OCP-only features are not required.

### Implementation Details/Notes/Constraints

CSO is responsible for two things:

1. **Deploying the exporter** — as a DaemonSet in the
   `openshift-cluster-storage-operator` namespace, using a RHEL-based image
   following the
   [new-component policy](https://github.com/openshift/enhancements/blob/master/dev-guide/new-components.md).
   Metrics are scraped by platform Prometheus using the cluster-wide TLS
   configuration from the `APIServer` CR.

2. **Shipping the alert rules** — as a `PrometheusRule` that joins
   `csiaddons_volume_node_device_info` (emitted by the exporter for every
   discovered CSI block volume) with kube-state-metrics PV/PVC metadata and
   node-exporter multipath or NVMe-oF health metrics. Alerts only fire when
   the node-exporter health metrics are present; clusters without multipath or
   NVMe-oF produce no path alerts.

| Alert | Severity / `for` | Condition |
| --- | --- | --- |
| `CSIAddonsVolumeMultipathDegraded` | warning / 5m | Failed paths with at least one usable path remaining |
| `CSIAddonsVolumeMultipathLost` | critical / 1m | Zero usable paths reported |
| `CSIAddonsVolumeNVMeSubsystemDegraded` | warning / 5m | Non-live controller paths with at least one live path remaining |
| `CSIAddonsVolumeNVMeSubsystemLost` | critical / 1m | Zero live controller paths reported |

### Risks and Mitigations

- **Host access and workload data exposure:** the exporter runs as UID 0 and
  mounts kubelet directories adjacent to pod volume data. Requires a dedicated
  SCC, read-only mounts, dropped capabilities, and a full security review before
  the gate reaches `Default`.
- **Known defect — optional driver paths:** the upstream exporter uses
  `DirectoryOrCreate` for driver-specific directories, which creates host paths
  even with a read-only root filesystem. Must be fixed before deployment.
- **Device disappearance breaks alert correlation:** when a block device
  disappears entirely from sysfs, the mapping metric disappears with it, losing
  the PVC label exactly when a path-loss alert is most needed. Alert rules must
  treat a missing mapping as an unknown state, not as recovery.

### Drawbacks

At GA, one extra pod runs on every node of every cluster, including clusters
that use only single non-multipath devices and therefore produce no path alerts.

## Alternatives (Not Implemented)

- **API opt-out:** rejected because administrators would not enable an optional
  monitoring component proactively and would realize too late that it was needed.
- **Kubelet metric:** kubelet does not know the full device stack (e.g. LUKS
  resolution) and no upstream KEP proposes it; multi-cycle effort with no
  guaranteed outcome.
- **CSI volume health / driver RPC:** requires all CSI drivers to implement it
  and the feature to reach GA upstream — also a multi-cycle effort.
- **node_exporter textfile collector:** still needs the same Kubernetes-specific
  discovery and host access, plus stale-file cleanup.
- **OLM add-on:** requires administrator installation; joins across separate
  Prometheus instances are not automatic.
- **Other lifecycle owner:** CSO already owns storage deployment; no other
  operator has a better fit.


## Test Plan

- **End-to-end tests** (the primary validation gate)
  - Using a software iSCSI or NVMe-oF target, provision a multipath CSI PVC,
    simulate partial path degradation and full path loss, and verify that the
    correct `CSIAddonsVolumeMultipath*` or `CSIAddonsVolumeNVMeSubsystem*` alert
    fires with the expected PV and PVC labels. Verify the alert clears after
    path restoration.
  - These tests require dedicated CI infrastructure with reproducible multipath
    storage; for initial Tech Preview delivery they could be manual.

- **Scale and lifecycle tests**
  - At least 100 PVCs per node at target cluster scale; measure CPU, memory, and
    scrape cardinality; results must justify production resource settings.
  - Verify the exporter tolerates unknown kubelet metadata fields and skips
    unresolvable mappings without crashing, across kubelet versions allowed by
    the target OpenShift release.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Security review of host access and SELinux confinement complete.
- Path alerts verified end to end for at least one driver/protocol/encryption
  combination using a reproducible multipath or NVMe-oF test setup.

### Tech Preview -> GA

- Automated e2e path-failure CI covering all advertised driver and protocol
  combinations.
- Scale results justify production resource settings.

### Removing a deprecated feature

No removal is proposed.

## Upgrade / Downgrade Strategy

No persisted state requires migration. During DaemonSet rollout, alert coverage
can be interrupted on individual nodes; test and document this gap. Volume I/O
is independent of the monitoring path.


## Operational Aspects of API Extensions

The only API extension is the `StoragePathAlert` feature gate. Exporter or
alert rule failures must not affect CSI provisioning, attachment, or mounts.

## Support Procedures

Debugging guidance and runbooks are published separately. Must-gather must
collect exporter logs and mapping metrics without copying workload volume
contents or driver tracking files that may contain credentials.

## Infrastructure Needed

- Confirm the OpenShift source repository/fork and maintainers for the
  [upstream exporter](https://github.com/csi-addons/csi-volume-device-exporter).
- ART image configuration, CSO image references, and OpenShift CI onboarding.
- Reproducible multipath storage jobs, FC validation access, and approved alert
  runbook hosting.
