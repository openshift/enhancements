---
title: cinder-csi-bare-metal
authors:
  - mandre
reviewers:
  - stephenfin # OpenStack: Cinder integration, cloud compatibility, and CI
  - jsafrane # Storage: CSI lifecycle, operator integration, and data safety
  - haircommander # Node and security: host services, privileged access, and SELinux
approvers:
  - stephenfin
api-approvers:
  - yuqi-zhang
creation-date: 2026-11-03
last-updated: 2026-11-03
status: provisional
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3304
see-also:
  - "/enhancements/storage/cinder-csi-driver-operator.md"
  - "/enhancements/storage/csi-driver-operator-merge.md"
replaces: []
superseded-by: []
---

# Cinder Block Storage for Bare-Metal Workers

## Summary

Allow pods running on Ironic-provisioned bare-metal workers in OpenShift on
OpenStack to consume Cinder-backed persistent volumes. Productize the upstream
Cinder CSI driver's direct-attachment mode through OpenShift-managed images and
operator configuration. The initial delivery is Tech Preview, with validation
focused on iSCSI and Ceph RBD backends. Existing Nova-based deployments retain
their current behavior; the initial implementation does not support migration
between attachment modes.

## Motivation

Applications running on bare-metal workers need persistent block storage without
requiring their compute nodes to be virtual machines. This proposal lets those
applications use an OpenStack deployment's Cinder service through the existing
PVC workflow, rather than maintaining a separate storage integration.

The upstream work in [cloud-provider-openstack PR #3215][upstream-pr] provides
an attachment path that bypasses Nova's volume-attachment operations.

### User Stories

- As an application administrator, I want to attach a Cinder-backed PVC to a pod
  on a bare-metal worker so that my application can persist data independently
  of that worker.
- As a cluster administrator, I want to drain a bare-metal worker for maintenance
  and restart its stateful workloads on another worker using their existing
  Cinder volumes, so that I can maintain workers without manually migrating
  application data.

### Goals

- Enable pods on Ironic-provisioned bare-metal workers in OpenShift on OpenStack
  to consume Cinder-backed persistent volumes.
- Initially validate iSCSI and Ceph RBD backends.
- Retain the existing application-facing PVC and StorageClass workflow.

### Non-Goals

- Supporting clusters whose platform type is not OpenStack.
- Using Nova attachment on some workers and direct attachment on others, or
  automatically selecting attachment mode per node.
- Provisioning bare-metal workers or changing the OpenStack installation workflow.
- Supporting changes between Nova and direct attachment in the initial
  implementation.

## Proposal

Extend the existing operator-managed Cinder CSI deployment, retaining the
`cinder.csi.openstack.org` driver identity and its `ClusterCSIDriver` resource.

1. Add a feature-gated attachment-mode setting. Nova remains the default;
   enabling the preview gate alone does not select direct attachment.
2. When direct attachment is selected, configure the CSI controller and node
   plugins with `--attach-mode=direct`. Add a privileged os-brick sidecar to each
   node-plugin pod, communicating through a pod-local Unix socket.
3. Build and ship the sidecar and its dependencies as a release-managed image,
   wired through the existing storage operators.
4. Integrate backend configuration, permissions, status reporting, and diagnostics
   into the operator-managed deployment.

VM and bare-metal workers may coexist, but all workers must use direct attachment
and meet its prerequisites when this mode is selected. The controller selects
attachment mode for the driver instance, not per worker.

### Workflow Description

**OpenStack administrator** manages Cinder, the storage backend, network access,
and the permissions granted to the cluster.

**Cluster administrator** prepares the workers and enables the feature.

**Application administrator** creates PVCs and workloads.

1. The OpenStack administrator provides access to Cinder and the storage backend,
   including the required network connectivity and credentials.
2. The cluster administrator prepares all workers for direct attachment and
   supplies any backend-specific configuration.
3. Before the cluster begins using Cinder storage, the cluster administrator
   enables the Tech Preview feature and selects direct attachment through the
   Cinder `ClusterCSIDriver`, as described in [API Extensions](#api-extensions).
   The operator deploys the required components.
4. Once the driver is ready, the application administrator creates a PVC using a
   Cinder StorageClass and a pod that consumes it. The driver connects the volume
   and makes it available to the pod.
5. For planned worker maintenance, the cluster administrator drains the worker.
   The driver releases the volume so the workload can restart on another prepared
   worker using the same PVC.

See [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy) for attachment-mode
initialization and lifecycle constraints, and
[Support Procedures](#support-procedures) for troubleshooting.

### API Extensions

Extend the existing cluster-scoped `operator.openshift.io/v1` `ClusterCSIDriver`
resource named `cinder.csi.openstack.org` with a Cinder attachment-mode setting.
The existing `spec.driverConfig` field supports driver-specific configuration,
but has no Cinder member in the API source used for this proposal.

The following is a **proposed API shape, not an existing supported API**:

```yaml
apiVersion: operator.openshift.io/v1
kind: ClusterCSIDriver
metadata:
  name: cinder.csi.openstack.org
spec:
  driverConfig:
    driverType: Cinder
    cinder:
      attachMode: Direct
```

Add `Cinder` to the driver-config discriminator and a matching `cinder` member,
valid only for `cinder.csi.openstack.org`. The `attachMode` setting accepts `Nova`
or `Direct`; omission preserves Nova behavior. Direct mode requires the proposed
`CinderCSIDirectAttach` feature gate in `TechPreviewNoUpgrade`. These additions
and their validation and feature gating require API approval and an
`openshift/api` change. In the initial implementation, the mode is selected before
the cluster begins using Cinder storage and remains fixed afterward. Initial
Direct-mode selection requires that no Cinder PVCs or PVs exist, as described in
[Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy). The locking mechanism
remains an open implementation question.

Backend configuration remains separate; see
[Host and backend prerequisites](#host-and-backend-prerequisites). The driver
also uses the upstream node annotation
`cinder.csi.openstack.org/connector-properties`, described under
[Node metadata and permissions](#node-metadata-and-permissions). No new resource
type, aggregated API, admission webhook, conversion webhook, or finalizer is added.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Direct attachment on HyperShift is outside the initial scope. Supporting it
requires validating the split between the hosted CSI controller and guest worker
node plugins, including configuration, credentials, and lifecycle management.
Existing Nova-based Cinder behavior on HyperShift is unchanged.

#### Standalone Clusters

Standalone OpenShift on OpenStack is the initial target. The operator, CSI
controller, and node plugins run in the same cluster. Nodes that also host
control-plane components use the same direct-attachment flow and must meet the
same backend and host prerequisites. Validation will include compact clusters
where application workloads run on control-plane nodes.

#### Single-node Deployments or MicroShift

SNO is not applicable because the OpenStack platform does not support single-node
deployments.

MicroShift integration is outside this proposal, which uses OpenShift's Cinder
operator and `ClusterCSIDriver` configuration rather than a MicroShift interface.

#### OpenShift Kubernetes Engine

The design uses the existing storage operators and requires no console or
OLM-installed operator. No separate OKE implementation is proposed; product
applicability must be confirmed before publishing an OKE support claim.

### Implementation Details/Notes/Constraints

#### Component changes

- **`openshift/cloud-provider-openstack`:** package the direct-mode driver and
  os-brick sidecar with supported storage dependencies and security servicing.
- **`openshift/csi-operator`:** add feature/configuration handling, workload
  arguments, sidecar and mounts, RBAC, placement rules, and status reporting.
  Direct mode reuses the existing Cinder credentials and preserves the cloud
  configuration and credential paths.
- **`openshift/cluster-storage-operator`:** pass the release-managed sidecar image
  reference to the Cinder operator.
- **`openshift/api`:** add the reviewed Cinder configuration and preview gate.

#### Attachment mechanism

1. During registration, the node plugin obtains connector properties from
   os-brick and publishes them on the Node. The controller maps CSI node IDs
   through CSINode objects to find those properties.
2. The controller creates a Cinder attachment and passes its ID and connection
   information to the node plugin through CSI publish context.
3. The node plugin connects the device through os-brick, completes the Cinder
   attachment, and stages the volume. For RBD, os-brick obtains a kernel block
   device. Connection information is persisted on the host for disconnect and
   protocol-specific expansion/rescan.
4. On release, the node plugin unmounts and disconnects the device; the
   controller removes the Cinder attachment.

Direct attachment requires Cinder API microversion `3.44` or later, available
since [OpenStack Queens](https://docs.openstack.org/cinder/latest/contributor/api_microversion_history.html#id41).

The driver uses metadata for node identity, with a deterministic UUID derived
from the node name as a direct-mode fallback. Preserve that identity and the
UUID derivation namespace across releases. Worker replacement and name reuse
require validation because attachments reference the CSI node ID.

#### Node metadata and permissions

The node plugin receives its Kubernetes node name through the Downward API and
patches `cinder.csi.openstack.org/connector-properties`. The controller needs
read/watch access to Nodes and CSINodes. The annotation is driver-owned metadata,
not an administrator configuration interface, and must not contain credentials.
Security review must cover Node patch permission scope and connector spoofing.

The os-brick Unix socket resides in an `emptyDir` shared with the node plugin;
no network-accessible os-brick service is introduced.

#### Host and backend prerequisites

Every worker needs connectivity to the backend data path as well as Keystone
and Cinder, with suitable credentials, trusted CAs, and a valid connector address.
The operator must report invalid configuration or unmet prerequisites.

- **iSCSI:** prepare unique initiator identities, iSCSI services, udev discovery,
  and any supported multipath configuration. Define host-service ownership and
  the RHCOS preparation procedure.
- **RBD:** provide Ceph monitor access, compatible userspace tools and bindings,
  kernel RBD support, and a suitably scoped keyring. Cinder API credentials do
  not themselves grant Ceph data-path access.

When Ceph configuration is needed, the administrator creates `ceph-config` in
`openshift-cluster-csi-drivers`, containing `ceph.conf` and its referenced keyring.
The direct-mode DaemonSet mounts it read-only at `/etc/ceph` in the sidecar.
The Secret volume is optional for deployments that do not need it. No
Secret-reference API or automatic Secret-triggered rollout is proposed. See
[Support Procedures](#support-procedures) for update and rotation requirements.

The node-plugin deployment requires access to host `/dev`, `/sys`, `/run`
(mounted at `/host/run`), `/etc/iscsi`, and `/lib/modules`. The OpenStack, Storage,
and Node teams must validate the mounts, mount propagation, host-runtime access,
and SELinux compatibility on RHCOS. Any required host configuration must have a
documented, supported setup procedure and must not interfere with CRI-O or
unrelated storage consumers.

#### Topology and compatibility

The OpenStack and Storage teams, with QE, will validate single-writer iSCSI and
RBD volumes on bare-metal and VM workers, testing filesystem and raw-block modes
separately. They will document the validated cloud/backend releases, worker
OS/kernel and architectures, filesystems, volume types, multipath behavior, and
attachment limits.

The upstream node path omits CSI topology when availability-zone metadata is
unavailable. The OpenStack and Storage teams must verify Ironic metadata and
Cinder volume-zone behavior before documenting support for topology-aware
provisioning.

Other Cinder transports are not architecturally excluded. They may need additional
os-brick dependencies, configuration, or host integration; a customer/vendor
extension mechanism and its support boundary remain open questions. Connector
availability in os-brick alone does not establish OpenShift support.

### Risks and Mitigations

- **Concurrent access and data corruption:** neither worker unreachability nor
  deletion of a Cinder attachment record proves that device access has stopped.
  The upstream direct-publish path deletes existing single-attach records before
  recreating an attachment. Establish a safe fencing/recovery contract and any
  required upstream changes before shipping. ReadWriteOnce is not fencing.
- **Lost recovery state:** disconnect and expansion depend on host-persisted
  connection information. Validate crashes between connection, attachment
  completion, state writes, mount, and cleanup. Missing state must not be treated
  as proof of disconnection.
- **Credential exposure:** connection information may contain authentication
  material. Review publish context, VolumeAttachment access, local files, logs,
  and must-gather redaction; local `0600` permissions do not protect API data.
- **Privileged host access:** review SCC usage, socket permissions, mounts,
  SELinux handling, and Node patch permissions with Node/security reviewers.
  Connection cleanup must not disrupt unrelated host sessions or devices.
- **Configuration changes and outages:** validate cloud/Ceph credential rotation
  and backend/API failures without disturbing healthy connections. Confirm
  behavior through transport-specific CI and documented recovery procedures.

OpenStack and Storage reviewers cover backend integration, CSI lifecycle, and
user workflows; Node/product-security reviewers cover host access and credential
handling. QE and support participate in recovery and documentation review.

### Drawbacks

Direct attachment adds a privileged Python storage runtime and moves backend
connection responsibilities onto workers. This increases image/dependency
maintenance, host integration work, and compatibility-testing costs, including
for VM workers in mixed clusters.

## Alternatives (Not Implemented)

- **Continue Nova attachment:** does not provide the requested worker-side
  connection path.
- **Deploy upstream manifests manually:** lacks release-managed images,
  reconciliation, status, and lifecycle support.
- **Use a backend-native CSI driver:** may suit some deployments, but replaces
  Cinder as the provisioning and policy interface.
- **Use separate Cinder identities or per-node mode dispatch:** would support
  mixed attachment modes, but adds registration or controller-dispatch complexity
  beyond this proposal's single-mode deployment.
- **Reimplement os-brick in Go:** duplicates transport-specific connection logic
  instead of using the upstream approach.

## Open Questions [optional]

1. When and how is the initial attachment mode selected and locked? How can an
   administrator select direct mode before Cinder storage is used, without
   locking in the operator's Nova default prematurely? How is the initial PVC/PV
   check coordinated with concurrent provisioning and binding?
2. What fencing and stale-attachment recovery contract is required, including
   changes to upstream cleanup and missing-state handling?
3. What RHCOS iSCSI/multipath configuration is needed, who owns the services,
   and which changes require MachineConfig or reboot?
4. What identity and volume-zone semantics apply when Ironic metadata is missing
   or changes during worker replacement?
5. How will publish-context credentials, connector metadata, and local recovery
   state be protected, and how will Ceph configuration/keys be refreshed safely?
6. How can customers or storage vendors provide dependencies and host integration
   for additional backends, and what support boundary applies?

## Test Plan

The OpenStack and Storage teams, with QE, will run the existing Cinder CSI test
suite against the operator-managed direct-mode deployment. Initial validation
will cover iSCSI and RBD on bare-metal and VM workers, including compact clusters.
Existing Nova and HyperShift test coverage remains unchanged.

Operator unit and integration tests will cover initial attachment-mode selection,
enforcement of the initial fixed-mode policy, feature gating, and deployment of
the os-brick sidecar and its configuration.
Initial-selection tests must cover pending Cinder PVCs and retained or statically
provisioned Cinder PVs blocking the change, with the deployed mode unchanged and
a reported reason. Unrelated storage must not block initial Direct-mode selection
when no Cinder PVCs or PVs exist and the feature gate is enabled.

For iSCSI and RBD, failure testing will cover a worker that loses cluster
connectivity while retaining storage access. Recovery must prevent conflicting
writes from the original and replacement workers. Tests will validate the agreed
fencing and stale-attachment recovery procedure, including refusing reattachment
when safe recovery cannot be established.

Additional validation needed for direct-mode recovery, worker replacement, and
credential handling will be identified against the existing suite's coverage.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Resolve implementation-blocking API, host integration, and data-safety questions
  and complete security review.
- Deliver release-managed images with dependency/security maintenance ownership
  and an opt-in operator deployment.
- Establish passing CI for the initial compatibility matrix and health signals
  for registration, attachment, staging, and detach.
- Publish setup, limitations, upgrade restrictions, and recovery documentation
  in `openshift-docs`.

### Tech Preview -> GA

- Demonstrate sustained CI health and complete lifecycle, failure-recovery,
  scale, and compatibility qualification.
- Agree on acceptance thresholds, supported limits, operational objectives,
  alerts, and credential-free telemetry, incorporating preview feedback.
- Stabilize the API and provide a reviewed transition from preview configuration
  to the default feature set without changing the selected attachment mode.
- Complete GA user/support documentation and ownership assignments.

### Removing a deprecated feature

No feature is deprecated or removed by this proposal.

## Upgrade / Downgrade Strategy

In the initial implementation, attachment mode is selected before the cluster
begins using Cinder storage and remains fixed afterward. The operator must
enforce this policy; the initialization point and enforcement mechanism remain
open questions. Whether to support mode changes in a future version is undecided.

Before applying the initial Nova-to-Direct selection, the operator must verify
that no Cinder PVCs or PVs exist across the cluster, including pending claims
and retained or statically provisioned volumes. If either exists, leave the
deployed mode unchanged and report why the change cannot be applied. Unrelated
storage must not block initial selection. Passing this check does not permit
mode changes after prior Cinder use.

The initial preview uses `TechPreviewNoUpgrade`, so ordinary cluster upgrades are
not promised while that feature set is enabled. Changing Cinder configuration
does not remove the feature-set restriction.

For GA, cluster upgrades must retain the selected attachment mode and preserve
I/O for already mounted volumes. Compatibility between component versions during
rolling updates is described in [Version Skew Strategy](#version-skew-strategy).

A driver version without direct-mode support is not a compatible rollback target
for a deployment using direct attachment. This proposal does not provide a
conversion to Nova as a rollback procedure or support arbitrary OpenShift
downgrades.

## Version Skew Strategy

For GA, the OpenStack and Storage teams will validate compatibility between CSI
controller and node-plugin versions that can coexist during a supported rolling
update. Each node-plugin revision must be deployed with a compatible os-brick
sidecar.

Updates must preserve CSI node identities and compatibility with attachment
information passed between components and recovery state persisted on the host.
Any required state-format migration must be defined and tested.

## Operational Aspects of API Extensions

The extension configures one existing cluster-scoped resource and adds node
annotation writes and Node/CSINode watches. It introduces no request-path webhook
or aggregated API. Failures should affect storage operations rather than general
API availability. OpenStack/Storage QE must measure registration bursts and API
load during scale qualification.

Report configuration and rollout failures through `ClusterCSIDriver` conditions
and the storage ClusterOperator reporting path. Health signals must cover
registration, attachment, staging, disconnect, and unmet prerequisites; process
liveness alone does not establish backend connectivity.

OpenStack owns cloud/backend integration, Storage owns operator/CSI lifecycle,
and Node/security owns host-service and SELinux issues.

## Support Procedures

Collect status, consumer events, and controller/node/sidecar logs, correlating
the PVC/PV, Cinder volume, VolumeAttachment, CSI node ID, and attachment ID:

```console
oc get clustercsidriver cinder.csi.openstack.org -o yaml
oc get clusteroperator storage
oc -n openshift-cluster-csi-drivers get pods -o wide
oc get csinode
oc get volumeattachment
oc -n <application-namespace> describe pod <pod-name>
oc -n <application-namespace> describe pvc <pvc-name>
```

For registration failures, inspect the sidecar socket and Node patch permissions.
For attachment failures, check connector metadata and Cinder policy/microversions.
For staging, detach, or expansion failures, inspect host mounts, iSCSI sessions or
RBD mappings, network access, credentials, and SELinux denials. Preserve recovery
state and follow the [data-safety requirements](#risks-and-mitigations) before
force cleanup. Must-gather must exclude Secret values and raw connection
information and redact sensitive log/publish-context data.

Administrators manage `ceph-config` updates and key rotation. Validate and document
configuration refresh, any required node-pod restart, overlapping key validity,
and effects on active/new connections. Updating a Secret does not establish that
active sessions reauthenticate.

After outages, reconcile attachment and device state before resuming operations;
existing connections cannot be assumed to survive reboot or session loss. Retain
a compatible driver, reconciliation, and credentials until connections have been
safely released. The initial implementation does not support switching attachment
mode as a recovery procedure; see
[Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy) for compatibility
constraints.

## Infrastructure Needed [optional]

- Sidecar image builds, supported package repositories, release image wiring,
  disconnected mirroring, and ART/security maintenance ownership.
- Ironic and VM worker capacity with independently testable iSCSI/RBD backends,
  credential rotation, network fault injection, and safe physical-worker fencing.
- Preview presubmit/periodic jobs and backend fixtures owned by OpenStack/Storage
  QE, with support participation in qualification and recovery runbooks.

## References

[upstream-pr]: https://github.com/kubernetes/cloud-provider-openstack/pull/3215

- [Cinder operator implementation](https://github.com/openshift/csi-operator/blob/f5d54d11ed1ba2ee3cefb75a4b2c18b30c6df378/pkg/driver/openstack-cinder/openstack_cinder.go)
- [Upstream controller attachment implementation](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/pkg/csi/cinder/controllerserver.go)
- [Upstream node lifecycle implementation](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/pkg/csi/cinder/nodeserver.go)
- [Cinder attachment API client](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/pkg/csi/cinder/openstack/openstack_attachments.go)
- [Connector metadata lookup](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/pkg/csi/cinder/connprops.go)
- [Upstream direct-mode node manifest](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/manifests/cinder-csi-plugin/cinder-csi-nodeplugin-direct.yaml)
- [Upstream os-brick connector implementation](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/sidecar/os_brick_grpc/server.py)
- [Cinder operator configuration synchronization](https://github.com/openshift/csi-operator/blob/f5d54d11ed1ba2ee3cefb75a4b2c18b30c6df378/pkg/openstack-cinder/config/configsync.go)
- [Downstream os-brick image work](https://github.com/openshift/cloud-provider-openstack/blob/master/images/osbrick-sidecar/Dockerfile)
- [Storage operator Cinder image wiring](https://github.com/openshift/cluster-storage-operator/blob/main/pkg/operator/csidriveroperator/csioperatorclient/cinder.go)
- [OpenShift supportability guidance](../../guidelines/supportability.md)
