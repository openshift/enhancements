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
last-updated: 2026-11-10
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
2. The cluster administrator prepares all workers, including node-local initiator
   identities and any administrator-managed MachineConfigs, and supplies required
   backend files through the Secret references in `backendConfig`.
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

Extend `operator.openshift.io/v1` `ClusterCSIDriver` named
`cinder.csi.openstack.org` with Cinder configuration. These are **proposed API
changes requiring approval**, not an existing supported API.

The excerpts modify `operator/v1/types_csi_cluster_driver.go`; unchanged fields
and rules are omitted. Secret references use
`configv1 "github.com/openshift/api/config/v1"`.

```go
// Replace the existing CSIDriverType enum marker with gate-aware markers.
// +openshift:validation:FeatureGateAwareEnum:featureGate="",enum="";AWS;Azure;GCP;IBMCloud;vSphere;SecretsStore
// +openshift:validation:FeatureGateAwareEnum:featureGate=CinderCSIDirectAttach,enum="";AWS;Azure;GCP;IBMCloud;vSphere;SecretsStore;Cinder
type CSIDriverType string

const CinderDriverType CSIDriverType = "Cinder"

// Add this rule to CSIDriverConfigSpec, retaining its existing union rules.
// +openshift:validation:FeatureGateAwareXValidation:featureGate=CinderCSIDirectAttach,rule="has(self.driverType) && self.driverType == 'Cinder' ? has(self.cinder) : !has(self.cinder)",message="cinder must be set if driverType is Cinder, but remain unset otherwise"
type CSIDriverConfigSpec struct {
    // cinder configures the Cinder CSI driver.
    // +openshift:enable:FeatureGate=CinderCSIDirectAttach
    // +optional
    Cinder CinderCSIDriverConfigSpec `json:"cinder,omitzero"`
}

// Add these rules to ClusterCSIDriver, retaining its existing rules.
// +openshift:validation:FeatureGateAwareXValidation:featureGate=CinderCSIDirectAttach,rule="self.spec.?driverConfig.?driverType.orValue('') == 'Cinder' ? self.metadata.name == 'cinder.csi.openstack.org' : true",message="driverType Cinder requires metadata.name cinder.csi.openstack.org"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=CinderCSIDirectAttach,rule="self.metadata.name == 'cinder.csi.openstack.org' ? self.spec.?driverConfig.?driverType.orValue('') in ['', 'Cinder'] : true",message="the Cinder resource permits only omitted or Cinder driver configuration"
// +openshift:validation:FeatureGateAwareXValidation:featureGate=CinderCSIDirectAttach,rule="has(self.status) && has(self.status.cinder) ? self.metadata.name == 'cinder.csi.openstack.org' : true",message="Cinder status is valid only on the Cinder resource"
type ClusterCSIDriver struct {
    // Existing fields remain unchanged.
}

// CinderCSIDriverConfigSpec configures Cinder attachment and backend files.
// +kubebuilder:validation:XValidation:rule="self.attachMode == 'Direct' || !has(self.backendConfig) || size(self.backendConfig) == 0",message="backendConfig is valid only in Direct mode"
type CinderCSIDriverConfigSpec struct {
    // attachMode selects Nova-mediated or worker-side os-brick attachment.
    // +required
    AttachMode CinderAttachMode `json:"attachMode"`

    // backendConfig supplies optional read-only files to the os-brick sidecar.
    // +listType=map
    // +listMapKey=name
    // +kubebuilder:validation:MaxItems=16
    // +optional
    BackendConfig []CinderBackendConfig `json:"backendConfig,omitempty"`
}

// CinderAttachMode selects the volume attachment mechanism.
// +kubebuilder:validation:Enum=Nova;Direct
type CinderAttachMode string

const (
    CinderAttachModeNova   CinderAttachMode = "Nova"
    CinderAttachModeDirect CinderAttachMode = "Direct"
)

// CinderBackendConfig projects a Secret's files at a supported destination.
type CinderBackendConfig struct {
    // name identifies this source, not a backend type or volume selector.
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=63
    // +kubebuilder:validation:Pattern=`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`
    // +required
    Name string `json:"name"`

    // secretRef names an administrator-managed Secret in
    // openshift-cluster-csi-drivers. All data keys are projected as files.
    // +required
    SecretRef configv1.SecretNameReference `json:"secretRef"`

    // mountPath selects an allowlisted read-only sidecar directory.
    // +kubebuilder:validation:Enum="/etc/ceph"
    // +required
    MountPath string `json:"mountPath"`
}

// Extend the existing status without changing its embedded OperatorStatus.
type ClusterCSIDriverStatus struct {
    OperatorStatus `json:",inline"`

    // cinder reports the last successfully rolled-out Cinder attachment mode.
    // +openshift:enable:FeatureGate=CinderCSIDirectAttach
    // +optional
    Cinder CinderCSIDriverStatus `json:"cinder,omitzero"`
}

// CinderCSIDriverStatus reports applied configuration.
type CinderCSIDriverStatus struct {
    // observedAttachMode is the mode of the last completed controller/node
    // rollout. Omission means no completed rollout has been observed.
    // +optional
    ObservedAttachMode CinderAttachMode `json:"observedAttachMode,omitempty"`
}
```

Add the proposed gate to the existing registration block in
`features/features.go`, using the existing feature registration helpers:

```go
FeatureGateCinderCSIDirectAttach = newFeatureGate("CinderCSIDirectAttach").
    reportProblemsToJiraComponent("Storage / Kubernetes External Components").
    contactPerson("mandre").
    productScope(ocpSpecific).
    enhancementPR("https://github.com/openshift/enhancements/pull/2174").
    enable(inTechPreviewNoUpgrade()).
    mustRegister()
```

The gate adds the Cinder discriminator and spec/status members to the schema.
Omitting Cinder configuration preserves Nova behavior; explicit Cinder
configuration requires `attachMode`. Enabling the gate alone does not select
Direct. The operator must enforce the initial-selection and fixed-mode policy
in [Upgrade / Downgrade Strategy](#upgrade--downgrade-strategy); cross-resource
checks and durable locking are not implemented by these schema rules.

See [Backend configuration Secrets](#backend-configuration-secrets) for the
backend file contract and [Node metadata and permissions](#node-metadata-and-permissions)
for the driver-owned connector annotation. No new resource type, aggregated API,
webhook, or finalizer is added.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Direct attachment on HyperShift is outside the initial scope. The shared API
fields need not be omitted, but the Cinder operator must reject Direct during
reconciliation using its cluster flavour: report `Degraded=True` with reason
`UnsupportedTopology`, leave both Nova workloads unchanged, and never report
Direct as observed. Supporting Direct requires validating the hosted controller/
guest worker split, including configuration, credentials, and lifecycle management.

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
  arguments, sidecar and mounts, backend Secret watches and validation, RBAC,
  placement rules, observed-mode status, and explicit HyperShift refusal.
  Direct mode reuses the existing Cinder credentials and preserves the cloud
  configuration and credential paths. It does not manage MachineConfigs.
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

- **iSCSI:** provide unique initiator identities, host iSCSI services, udev
  discovery, and supported multipath configuration, as described below.
- **RBD:** provide Ceph monitor access, compatible userspace tools and bindings,
  kernel RBD support, and a suitably scoped keyring. Cinder API credentials do
  not themselves grant Ceph data-path access.

The node-plugin deployment requires access to host `/dev`, `/sys`, `/run`
(mounted at `/host/run`), `/etc/iscsi`, and `/lib/modules`. The OpenStack, Storage,
and Node teams must validate the mounts, mount propagation, host-runtime access,
and SELinux compatibility on RHCOS. Any required host configuration must have a
documented, supported setup procedure and must not interfere with CRI-O or
unrelated storage consumers.

#### Host configuration and service ownership

Administrators own host preparation and MachineConfigs for all participating
nodes. No new MCO templates or Cinder-operator-managed MachineConfigs are proposed.
Host systemd owns iSCSI/multipath daemons; the sidecar supplies tools and per-volume
operations without competing daemons or overwriting host configuration.

Each iSCSI host needs a unique, persistent `/etc/iscsi/initiatorname.iscsi`,
distinct from its CSI node ID. Shared MachineConfigs must not distribute one
literal identity. The os-brick connector reads this file; the inspected sidecar
startup does not generate it. Administrators must verify uniqueness before
Direct mode and after replacement, and preserve identity across pod/configuration
rollouts. Target-RHCOS initialization still requires Node-team validation.

Before Tech Preview, OpenStack, Storage, and Node reviewers must validate host
setup and service actions with active volumes and unrelated storage consumers.
Administrators may use `MachineConfiguration.spec.nodeDisruptionPolicy` where
reboot-free application is validated. These host actions are separate from the
Cinder operator's Secret-triggered node-pod rollouts.

#### Backend configuration Secrets

`spec.driverConfig.cinder.backendConfig` references administrator-managed Secrets
in `openshift-cluster-csi-drivers`, projecting all data keys as read-only files
into every direct-mode sidecar. Omission mounts no files; non-empty configuration
requires Direct mode. There is no fixed-name lookup or new backend credential
mount in the controller. For example:

```yaml
backendConfig:
  - name: backend-files
    secretRef:
      name: storage-backend-config
    mountPath: /etc/ceph
```

Multiple Secrets may contribute distinct filenames to a destination. The operator
must reject missing/empty sources, duplicate destination filenames, unsafe paths,
and unsupported destinations, and restrict file permissions to the required
sidecar identity. Arbitrary mounts, host writes, and executable hooks are not
supported. Initially only `/etc/ceph` is allowlisted; for RBD the Secret supplies
`ceph.conf` and its referenced keyrings. Additional destinations require connector
and dependency review, not per-backend Secret-reference fields. This interface
does not itself establish backend support or credential validity.

The operator watches referenced Secrets. Invalid configuration reports
`Degraded=True` with reason `InvalidBackendConfig` without initiating a new
rollout or deleting healthy workloads. Valid updates trigger a controlled rolling
node-pod update using source identity/version metadata, not Secret contents, as
the rollout identifier. In-place Secret changes can also refresh running pods'
files before rollout; retaining a pod template does not retain old credentials.
Safe refresh, restart, and rotation with active volumes must be validated before
Tech Preview; see [Support Procedures](#support-procedures).

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
- **Configuration changes and outages:** validate cloud/backend credential rotation
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
3. Which exact RHCOS iSCSI/multipath files, units, and preparation steps are
   required under the administrator-managed model? How is a unique initiator
   identity initialized on the target image, and which changes safely use a
   non-reboot disruption policy with active volumes?
4. What identity and volume-zone semantics apply when Ironic metadata is missing
   or changes during worker replacement?
5. How will publish-context credentials, connector metadata, and local recovery
   state be protected, and which transport-specific checks establish safe backend
   configuration/key rotation and node-pod rollout?
6. How can customers or storage vendors provide dependencies and host integration
   for additional backends, and what support boundary applies?

## Test Plan

The OpenStack and Storage teams, with QE, will run the existing Cinder CSI test
suite against the operator-managed direct-mode deployment. Initial validation
will cover iSCSI and RBD on bare-metal and VM workers, including compact clusters.
Existing Nova behavior remains covered; HyperShift coverage adds explicit Direct
mode refusal.

Operator unit and integration tests will cover initial attachment-mode selection,
enforcement of the initial fixed-mode policy, feature gating, and deployment of
the os-brick sidecar and its configuration.
Initial-selection tests must cover pending Cinder PVCs and retained or statically
provisioned Cinder PVs blocking the change, with the deployed mode unchanged and
a reported reason. Unrelated storage must not block initial Direct-mode selection
when no Cinder PVCs or PVs exist and the feature gate is enabled.

Additional API, operator, and host tests must cover:

- Gate-on/off schemas, omitted/default configuration, required fields, and
  invalid discriminator, resource-name, mode, and backend-path combinations.
- Observed mode during successful, pending, rejected, and failed rollouts;
  HyperShift refusal must leave both Nova workloads unchanged.
- Multiple Secret sources, filename collisions, missing/invalid configuration,
  update/deletion reconciliation, and credential-free diagnostics. Rotation must
  preserve I/O and post-restart disconnect/expansion with active volumes.
- Unique initiator identities and persistence across pod restart and host reboot;
  validated MachineConfigs and disruption policies on workers and workload-bearing
  control-plane nodes, without competing daemons or disruption to other storage.

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

The extension configures one existing cluster-scoped resource, adds node
annotation writes and Node/CSINode watches, and watches backend Secrets in
`openshift-cluster-csi-drivers`. It introduces no request-path webhook
or aggregated API. Failures should affect storage operations rather than general
API availability. OpenStack/Storage QE must measure registration bursts and API
load during scale qualification.

Report configuration and rollout failures through `ClusterCSIDriver` conditions
and the storage ClusterOperator reporting path. `status.cinder.observedAttachMode`
records the last completed controller/node rollout, retaining its prior value on
pending, failed, or rejected changes and remaining unset before the first completed
rollout. It is not the durable mode lock. Interpret it with conditions and
`observedGeneration`: a processed spec or a previous mode does not establish
current rollout success. Health signals must cover registration, attachment,
staging, disconnect, and unmet prerequisites without exposing credentials;
process liveness alone does not establish backend connectivity.

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

Administrators own backend Secret updates and key rotation. Create replacement
Secrets before changing references, keep overlapping key validity through rollout,
and verify new attachments and existing-volume cleanup before revoking old keys.
Updating files or restarting pods does not establish that active sessions
reauthenticate. Follow validated host-service and disruption-policy procedures;
do not regenerate initiator identities or restart shared storage services as
generic troubleshooting.

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

- [ClusterCSIDriver API source used for the proposal](https://github.com/openshift/api/blob/f6e8a0876ff972521e8bf3bf687079e6511ac001/operator/v1/types_csi_cluster_driver.go)
- [Cinder operator implementation](https://github.com/openshift/csi-operator/blob/f5d54d11ed1ba2ee3cefb75a4b2c18b30c6df378/pkg/driver/openstack-cinder/openstack_cinder.go)
- [Upstream controller attachment implementation](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/pkg/csi/cinder/controllerserver.go)
- [Upstream node lifecycle implementation](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/pkg/csi/cinder/nodeserver.go)
- [Cinder attachment API client](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/pkg/csi/cinder/openstack/openstack_attachments.go)
- [Connector metadata lookup](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/pkg/csi/cinder/connprops.go)
- [Upstream direct-mode node manifest](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/manifests/cinder-csi-plugin/cinder-csi-nodeplugin-direct.yaml)
- [Upstream os-brick connector implementation](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/sidecar/os_brick_grpc/server.py)
- [Upstream sidecar startup](https://github.com/kubernetes/cloud-provider-openstack/blob/a016eca9343d71a460c9d2f4d4e4bdb0085ab127/sidecar/entrypoint.sh)
- [os-brick iSCSI initiator lookup](https://github.com/openstack/os-brick/blob/master/os_brick/initiator/connectors/iscsi.py)
- [Administrator-defined node disruption policy](../machine-config/admin-defined-node-disruption-policy.md)
- [Cinder operator configuration synchronization](https://github.com/openshift/csi-operator/blob/f5d54d11ed1ba2ee3cefb75a4b2c18b30c6df378/pkg/openstack-cinder/config/configsync.go)
- [Downstream os-brick image work](https://github.com/openshift/cloud-provider-openstack/blob/master/images/osbrick-sidecar/Dockerfile)
- [Storage operator Cinder image wiring](https://github.com/openshift/cluster-storage-operator/blob/main/pkg/operator/csidriveroperator/csioperatorclient/cinder.go)
- [OpenShift supportability guidance](../../guidelines/supportability.md)
