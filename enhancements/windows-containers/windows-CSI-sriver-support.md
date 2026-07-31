---
title: windows-csi-node-plugins
authors:
  - "@mansikulkarni96"
reviewers:
  - "@openshift/openshift-team-windows-containers"
  - "@openshift/openshift-team-storage, for CSI operator and image build guidance"
approvers:
  - TBD
api-approvers:
  - None
creation-date: 2026-07-30
last-updated: 2026-07-30
tracking-link:
  - https://redhat.atlassian.net/browse/WINC-26
---

# Enable CSI Node Plugins on Windows Nodes

## Release Sign-off Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The goal of this enhancement is to enable CSI node driver plugins to run on Windows nodes, providing persistent
storage support for Windows workloads through the standard PersistentVolumeClaim workflow.

Currently, all CSI node plugin DaemonSets in OpenShift target Linux nodes exclusively
(`nodeSelector: kubernetes.io/os: linux`). While WMCO already deploys
[CSI Proxy](https://github.com/kubernetes-csi/csi-proxy) on every Windows node (as described in the
[csi-proxy enhancement](csi-proxy.md)), no CSI driver node plugins are deployed to use it. This means Windows pods
cannot mount PVCs.

This enhancement adds Windows node DaemonSets to the CSI driver operators, so that Windows pods can use persistent
storage without any manual DaemonSet deployment by cluster administrators.

## Motivation

The [csi-proxy enhancement](csi-proxy.md) established the prerequisite infrastructure for CSI drivers on Windows
nodes, but explicitly stated that Windows CSI node drivers would not be distributed as part of OpenShift due to the
inability to ship Windows container images. That constraint has since been lifted: Buildah now supports building
Windows containers (containers/buildah#6592), and the Konflux CI pipeline has validated this capability. Red Hat can
now build and ship Windows container images as part of the product.

With this constraint removed, the workaround of requiring cluster administrators to manually deploy and maintain
Windows CSI node driver DaemonSets is no longer necessary. The CSI driver operators can deploy Windows DaemonSets
alongside the existing Linux ones, providing a seamless experience.

### User Stories

* As a cluster administrator, I want persistent storage to be available on Windows nodes without having to manually
  deploy and maintain CSI node driver DaemonSets, so that the operational burden is the same as for Linux nodes.
* As a cluster administrator, I want to attach dynamic PersistentVolumes to Windows worker nodes so that I can run
  stateful applications on Windows in OpenShift.

### Goals

* Windows pods can create, mount, read/write, and delete PVCs using supported StorageClasses and access modes
  (see Windows Storage Support Matrix below).
* Windows CSI node DaemonSets are deployed automatically by the CSI driver operators.
* No changes are required to WMCO beyond what is already shipping (CSI proxy).
* Zero regression on Linux storage functionality.
* The pattern is reusable across all CSI driver operators.

### Non-Goals

* Achieving full feature parity with Linux storage. Windows does not support all volume modes, access modes, and
  volume features available on Linux. See the support matrix below.
* Modifying CSI driver controllers. Controllers run on the Linux control plane and are OS-agnostic.
* HyperShift support. Windows nodes are only supported on standalone clusters.
* Enabling shared volumes between Linux and Windows simultaneously.

### Windows Storage Limitations

Not all Linux storage capabilities are available on Windows. The following limitations are known at the
Kubernetes level and apply regardless of CSI driver:

* Raw block volumes (`volumeMode: Block`) are not supported on Windows.
* `fsGroup` and POSIX file ownership/permissions are not supported on Windows.
* `mountPropagation` is not supported on Windows.

Support for other capabilities (volume expansion, snapshots, specific access modes) depends on the individual
CSI driver and should be verified against upstream driver documentation before making claims in user-facing
documentation.

## Proposal

Each CSI driver operator will deploy a second node DaemonSet targeting Windows nodes, in addition to the existing
Linux DaemonSet. The Windows DaemonSet will use `nodeSelector: kubernetes.io/os: windows`, so it will only schedule
pods on Windows nodes. On clusters with no Windows nodes, the DaemonSet will exist but have zero desired pods.

A single multi-OS DaemonSet is not feasible. Windows and Linux pods require fundamentally different security models
(`windowsOptions.hostProcess` vs `securityContext.privileged`), different filesystem paths (`C:\var\lib\kubelet` vs
`/var/lib/kubelet`), and different volume mounts (no `/dev`, `/sys/fs`, or `/etc/selinux` on Windows). Two separate
DaemonSets is the standard upstream pattern for multi-OS CSI deployments.

The Windows DaemonSet will contain three containers, mirroring the Linux DaemonSet structure:
- The CSI driver itself, communicating with CSI proxy via named pipes
- The node-driver-registrar sidecar, registering the driver with the kubelet
- The liveness-probe sidecar, providing health checking

The Windows DaemonSet runs as a HostProcess pod (`windowsOptions.hostProcess: true`) with `hostNetwork: true`.
HostProcess is required by the CSI driver container for host filesystem access and CSI proxy named pipe
communication. It is a pod-level setting in Kubernetes, so the registrar and liveness-probe sidecars inherit
it even though they only need access to the CSI driver socket. `hostNetwork` is required for health and
metrics endpoints, consistent with the Linux DaemonSet.

All three containers must be Windows executables. They will be built using Buildah multi-stage builds
(cross-compiling with `GOOS=windows` on a Linux builder, packaging into a Windows nanoserver runtime image) and
published under a single image tag that contains both Linux and Windows variants. The kubelet on each node
automatically pulls the variant matching its platform, so the same image reference (e.g. `${DRIVER_IMAGE}`) works
for both DaemonSets without any special configuration.

Two CSI drivers are targeted for the initial implementation:

**Azure File** is deployed through the consolidated
[csi-operator](https://github.com/openshift/csi-operator), which uses a code generator to produce DaemonSet
manifests from base templates, driver-specific patches, and sidecar injection. The changes consist of shared
infrastructure (Windows base template, Windows sidecar templates, generator extensions) and per-driver configuration
(Azure File driver patch with Windows paths and credentials). The shared infrastructure is reusable -- adding
Windows support to another csi-operator driver requires only the per-driver patch and configuration. Azure File uses
the SMB protocol, which has native Windows support.

**VMware vSphere** is deployed through the standalone
[vmware-vsphere-csi-driver-operator](https://github.com/openshift/vmware-vsphere-csi-driver-operator), which uses
static YAML and library-go's `CSIControllerSet` directly. The changes consist of a new Windows node DaemonSet
YAML file and a second `.WithCSIDriverNodeService()` call chained onto the existing `CSIControllerSet` builder. The
upstream vSphere CSI driver already has `csi-windows-support: "true"` enabled in its feature states ConfigMap and
handles Windows plus CSI proxy interactions internally.

### Workflow Description

1. The cluster administrator installs WMCO and creates Windows nodes (via MachineSets or BYOH). WMCO deploys CSI
   proxy on each Windows node as it does today.
2. The CSI driver operators detect the presence of their Windows DaemonSet manifest and deploy it. The DaemonSet's
   node selector ensures pods only land on Windows nodes.
3. The CSI driver pods start on each Windows node, register with the kubelet via node-driver-registrar, and the
   Windows nodes appear in CSINode objects with the driver listed.
4. Windows pods can now mount PVCs using existing StorageClasses. No manual intervention is required.

### API Extensions

N/A

### Risks and Mitigations

* The Azure File Linux DaemonSet uses an `azure-inject-credentials` initContainer (a Linux binary from
  `cluster-cloud-controller-manager-operator`). For the Windows DaemonSet, the simplest approach is to mount the
  Azure credentials secret directly into the driver container, bypassing the injector. If this is not acceptable for
  production, a Windows build of the credentials injector would be needed.


## Design Details

### Affected Repositories

The [csi-operator](https://github.com/openshift/csi-operator) requires the following changes:

Shared infrastructure, reusable across all drivers:
- Windows base DaemonSet template and Windows-specific sidecar templates (node-driver-registrar, liveness-probe)
- Generator extensions to produce a Windows DaemonSet from base template, driver patch, and sidecar injection
  (standalone mode only, skipped for HyperShift)
- A second `WithCSIDriverNodeService()` call in the operator starter to deploy the Windows DaemonSet

Per-driver configuration (example: Azure File):
- A driver-specific patch with Windows paths, credentials, and environment variables
- Driver config updated to reference the Windows DaemonSet template and sidecars

The [vmware-vsphere-csi-driver-operator](https://github.com/openshift/vmware-vsphere-csi-driver-operator) requires:
- `assets/node_windows.yaml` -- new Windows node DaemonSet manifest
- `pkg/operator/vspherecontroller/driver_starter.go` -- chain a second `.WithCSIDriverNodeService()` call for the
  Windows DaemonSet

The [cluster-storage-operator](https://github.com/openshift/cluster-storage-operator) requires no code changes.
Because each image tag contains both Linux and Windows variants, the existing image environment variables
(`DRIVER_IMAGE`, `NODE_DRIVER_REGISTRAR_IMAGE`, `LIVENESS_PROBE_IMAGE`) work as-is for both DaemonSets.
### Container Images

Three components require Windows builds for each driver:

The CSI driver binary is cross-compiled from the driver-specific repository with `GOOS=windows GOARCH=amd64`.

The node-driver-registrar is cross-compiled from `kubernetes-csi/node-driver-registrar`.

The liveness-probe is cross-compiled from `kubernetes-csi/livenessprobe`.

Each is built using a Buildah multi-stage Dockerfile: a Linux-based Go toolchain builder stage produces the Windows
executable, which is then packaged into a `mcr.microsoft.com/windows/nanoserver:ltsc2022` runtime image using
`buildah bud --platform windows/amd64`.

The Linux and Windows images are published under the same image tag. The kubelet automatically pulls the variant
matching its platform.

### Test Plan

#### Unit Tests

The csi-operator generator tests will be extended to verify that Windows DaemonSet YAML is produced correctly for
each driver that enables Windows support. The tests will validate that no Linux-specific fields are present in the
generated output (no `securityContext.privileged`, no `readOnlyRootFilesystem`, no `preStop` with `/bin/sh`, no
`mountPropagation`).

Schema validation using `oc apply --dry-run=server` will be used to confirm that the generated Windows DaemonSet
manifests are accepted by the API server.

#### E2E Tests

WMCO already has e2e storage tests that manually deploy Windows CSI node driver DaemonSets
(`test/e2e/providers/azure/azure.go:ensureWindowsCSIDaemonSet()`). These tests validate the full flow from
DaemonSet deployment through PVC mount and data persistence. Once the CSI driver operators deploy the Windows
DaemonSets natively, the manual deployment in WMCO e2e tests can be removed and replaced with validation that the
operator-deployed DaemonSet is functioning correctly.

The e2e tests will verify:
1. The Windows DaemonSet is created and the desired count matches the number of Windows nodes.
2. All pods are in Running state on Windows nodes.
3. The CSI driver is registered with the kubelet (visible in CSINode objects).
4. A PVC can be created, mounted into a Windows pod, written to, and the data persists across pod restarts.
5. PVC deletion and cleanup complete without errors.

### Graduation Criteria

#### Dev Preview -> Tech Preview

N/A. This feature targets GA directly as it builds on the already-GA CSI proxy infrastructure.

#### Tech Preview -> GA

This feature is targeted for OpenShift 5.x. The initial GA scope includes Azure File and VMware vSphere. Additional
drivers (Azure Disk, AWS EBS, GCP PD) may be added in subsequent releases based on customer demand and upstream
driver readiness.

#### Removing a deprecated feature

The manual Windows CSI DaemonSet deployment workflow described in the [csi-proxy enhancement](csi-proxy.md) will be
superseded by this enhancement. Users who have manually deployed Windows CSI DaemonSets will need to remove them
before the operator-managed DaemonSets are deployed, to avoid conflicts. Documentation should guide users through
this transition.

### Upgrade / Downgrade Strategy

#### Cluster upgrades

When upgrading to a version that includes operator-managed Windows CSI DaemonSets, the DaemonSets will be created
automatically. If the cluster has Windows nodes, the CSI driver pods will start on those nodes. If the cluster has
no Windows nodes, the DaemonSets will exist with zero desired pods and have no effect.

Cluster administrators who have manually deployed Windows CSI DaemonSets (following the workflow from the csi-proxy
enhancement) should remove them before upgrading, to avoid conflicts with the operator-managed DaemonSets. Release
notes should clearly document this requirement.

#### Node upgrades

Windows node upgrades are handled by WMCO independently of the CSI driver operators. When WMCO upgrades a Windows
node, the CSI proxy service is updated as part of the normal service upgrade flow. The CSI driver pods will be
restarted by the DaemonSet controller after the node rejoins the cluster. No special coordination is required
between WMCO and the CSI driver operators.

#### Downgrades

WMCO does not support downgrades. If the CSI driver operators are downgraded to a version that does not include
Windows DaemonSets, the Windows DaemonSets will be removed and Windows pods will lose persistent storage access.

### Version Skew Strategy

Because the Windows and Linux DaemonSets are managed by the same operator and use the same image tags, there is no
version skew between Linux and Windows CSI components. Both DaemonSets are updated
atomically by the operator during cluster upgrades.

This is a significant improvement over the manual deployment approach described in the csi-proxy enhancement, where
version skew between operator-managed Linux drivers and user-managed Windows drivers was an ongoing concern.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

If the Windows CSI driver pods fail to start (e.g. image pull failure, CSI proxy not running), the DaemonSet
controller will report the failure through standard Kubernetes mechanisms (pod status, events). The CSI driver
operator will surface the degraded state through its ClusterOperator status conditions.

Windows pods attempting to mount PVCs will fail with volume attachment errors if the CSI driver is not running on
their node. This is the same behavior as the Linux path when CSI drivers are unavailable.

#### Support Procedures

Because the Windows CSI node drivers are now shipped as part of OpenShift, support covers the full storage stack on
Windows nodes, including the CSI driver pods, node-driver-registrar, liveness-probe, and CSI proxy. This is a change
from the csi-proxy enhancement, which explicitly excluded support for upstream CSI drivers deployed by the user.

The CSI driver pod logs on Windows nodes can be collected through standard `oc logs` commands or through
must-gather. CSI proxy logs continue to be collected as described in the csi-proxy enhancement.

## Implementation History

Prototype implementations have been completed for both the csi-operator (Azure File) and the
vmware-vsphere-csi-driver-operator. The prototypes validate that the generator framework correctly produces Windows
DaemonSet YAML, that the `CSIControllerSet` supports multiple `WithCSIDriverNodeService()` calls, and that both
repositories build successfully with the changes.

## Alternatives (Not Implemented)

The current approach described in the [csi-proxy enhancement](csi-proxy.md) requires cluster administrators to
manually deploy and maintain Windows CSI node driver DaemonSets. This was the only viable option when OpenShift
could not build or ship Windows container images. Now that Buildah Windows support is available and validated in
Konflux, the manual approach is no longer necessary and is superseded by this enhancement.

An alternative to publishing both Linux and Windows variants under the same image tag would be separate image
references for Windows containers (e.g. `WINDOWS_DRIVER_IMAGE`), requiring changes to the cluster-storage-operator
to inject the additional environment variables. Using a single tag per image is preferred because it requires no
changes to the cluster-storage-operator and is consistent with how multi-platform support works elsewhere in
OpenShift.

An alternative for the Azure File credential injection is to build a Windows version of the
`azure-inject-credentials` binary from `cluster-cloud-controller-manager-operator`. This is more complex than
directly mounting the credentials secret but would maintain consistency with the Linux DaemonSet's approach. The
direct secret mount is preferred for the initial implementation due to its simplicity.
