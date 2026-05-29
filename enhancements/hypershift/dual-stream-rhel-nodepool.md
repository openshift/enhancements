---
title: dual-stream-rhel-nodepool
authors:
  - "@enxebre"
reviewers:
  - "@sdminonne"
  - "@jparrill"
approvers:
  - "@csrwng"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-05-21
last-updated: 2026-05-29
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-3014
see-also:
  - "/enhancements/ocp-coreos-layering/ocp-coreos-layering.md"
---

# Dual-Stream RHEL 9/10 NodePool Support in HyperShift

## Summary

Starting in OpenShift 4.23, the release payload carries both RHEL 9 (`rhel-coreos`) and RHEL 10 (`rhel-coreos-10`) node images,
and a new `OSImageStream` CRD (TechPreview, behind the `OSStreams` feature gate) allows selecting which RHEL version to use per MachineConfigPool.
HyperShift currently assumes a single OS image per architecture and has no mechanism for stream selection.

This enhancement adds a `spec.osImageStream` field to the NodePool API, threads the stream selection through the token secret
and ignition server to the MCO bootstrap pipeline, and updates the boot image metadata parsing to handle the new multi-stream
ConfigMap format. The result is that each NodePool in a HostedCluster can independently select RHEL 9 or RHEL 10, with correct
boot AMI resolution and ignition payload generation.

## Glossary

- **OS stream** — A RHEL major version variant of the node OS image. Current streams are `rhel-9` and `rhel-10`. Each stream has its own `rhel-coreos*` container image and boot disk images (AMIs, VHDs).
- **OSImageStream** — A TechPreview CRD (`machineconfiguration.openshift.io/v1alpha1`) introduced by the MCO.
  The singleton `cluster` resource declares `spec.defaultStream` and reports `status.availableStreams` discovered from OCI labels on release images.
- **MCO bootstrap pipeline** — Three one-shot binaries (`machine-config-operator`, `machine-config-controller`, `machine-config-server`) run sequentially inside the ignition server pod to generate ignition payloads. The ignition server orchestrates them in `GetPayload()` via three steps:
  1. `runMCO()` — executes `machine-config-operator bootstrap`. Reads `--image-references` from the release payload and produces raw manifests (ControllerConfig, MachineConfigPools, MachineConfigs) in an output directory. Internally calls `copyMCOOutputToMCC()` to copy MCO output manifests plus CPO-generated pool overrides (`*.machineconfigpool.yaml`) into the MCC input directory (`mccDir`).
  2. `runMCC()` — executes `machine-config-controller bootstrap`. Reads all manifests from `mccDir`, including ControllerConfig,
     FeatureGate, MachineConfigPools, and (with this enhancement) OSImageStream. When the `OSStreams` feature gate is active,
     the MCC calls `fetchOSImageStream()` to inspect OCI labels on release images, discover available streams, select the stream
     from `OSImageStream.spec.defaultStream`, and override `ControllerConfig.baseOSContainerImage` with the selected stream's image.
     It then renders final MachineConfigs with the correct `osImageURL`.
  3. `runMCSAndFetchPayload()` — executes `machine-config-server` which reads the rendered MachineConfigs and produces the ignition JSON payload.
  A preparatory step, `runFeatureGateRender()`, runs before `runMCO()` to write the FeatureGate manifest into `mccDir`.
- **Token secret** — A per-NodePool Secret in the control plane namespace containing the ignition token, release image, config hash, and other data needed by the ignition server to generate a payload.
- **Boot image** — The platform-specific disk image (AMI, VHD, qcow2) used to launch a new node. In the layered model, this is a base CoreOS image (kernel, systemd, ignition) without OCP packages. The node rebases to the full node image on first boot.

## Motivation

Starting in OpenShift 4.23, the release payload carries both RHEL 9 and RHEL 10 node images. RHEL 9 remains the default for 4.x releases, while OpenShift 5.0 switches the default to RHEL 10. Standalone clusters handle per-pool stream selection via the OSImageStream CRD. HyperShift has no equivalent mechanism — all NodePools in a HostedCluster get the same OS image regardless of the payload carrying two streams.

Without this enhancement, HyperShift users on 4.23+ cannot:
- Opt specific NodePools into RHEL 10 while keeping others on RHEL 9
- Gradually migrate workloads between RHEL versions
- Use the same dual-stream capabilities available in standalone clusters

### User Stories

- As a **Platform Operator**, I want to create a NodePool running RHEL 10 alongside my existing RHEL 9 NodePools, so that I can validate workload compatibility before migrating the entire cluster.

- As a **Platform Operator**, I want to opt specific NodePools into RHEL 10 on 4.23+ while keeping the default on RHEL 9, so that I can validate workload compatibility before the 5.0 default switch.

- As a **Platform Operator**, I want my NodePools to automatically use the release version's default OS stream (RHEL 9 for 4.x, RHEL 10 for 5.x) without explicit configuration, so that upgrades to 5.0 move nodes to RHEL 10 by default.

- As a **Platform Operator**, I want to see which RHEL stream my NodePool nodes are actually running via `status.osImageStream`, so that I can confirm convergence after a stream change or upgrade.

- As a **Platform SRE**, I want NodePool creation to fail early with a clear condition message if a user requests `rhel-10` on a release that doesn't carry RHEL 10 images, so that users get immediate feedback instead of cryptic ignition errors.

- As a **Platform Operator**, I want NodePools with runc MachineConfigs to automatically stay on RHEL 9 when upgrading to 5.0, so that the upgrade doesn't break workloads that depend on runc (which RHEL 10 does not ship).

### Goals

- Allow per-NodePool RHEL stream selection via `spec.osImageStream`.
- Resolve stream-specific boot images (AMIs, VHDs) from the release payload's multi-stream ConfigMap.
- Generate correct ignition payloads for each stream by injecting an OSImageStream CR into the MCO bootstrap pipeline.
- Isolate ignition token secrets so that NodePools targeting different streams get separate payloads.
- Validate stream selection against the release version and fail early with clear conditions.
- Report observed stream on nodes via `status.osImageStream`.
- Guard against runc incompatibility on RHEL 10.

### Non-Goals

- Adding a HostedCluster-level stream field. Stream selection is per-NodePool, mirroring the per-MachineConfigPool model in standalone.
- Implementing stream discovery logic in HyperShift. The MCO's existing `fetchOSImageStream()` handles OCI label inspection — HyperShift only needs to provide the OSImageStream CR manifest as input.
- Supporting RHEL 10 on release payloads < 4.23. These payloads do not carry RHEL 10 images.
- Exposing stream selection in the Karpenter NodePool API. Karpenter NodePools always use the version-derived default stream.

## Proposal

### Architecture Overview

The OS stream flows through three layers:

1. **NodePool API** — `spec.osImageStream.name` selects `rhel-9` or `rhel-10`. When unset, defaults to `rhel-9` for release < 5.0 and `rhel-10` for >= 5.0. Explicit opt-in to `rhel-10` is allowed starting from release 4.23 (the first release whose payload carries both streams).
2. **NodePool controller** — writes the resolved stream to the token secret. When an explicit `spec.osImageStream.name` is set, includes the stream in the config hash to trigger a rollout; implicit streams do not change the hash. Resolves stream-specific boot AMIs from the release payload's multi-stream ConfigMap.
3. **Ignition server** — reads the stream from the token secret, generates an OSImageStream CR (`99_osimagestream.yaml`), and places it in the MCC manifest directory. The MCC bootstrap discovers available streams via OCI label inspection and selects the requested stream, producing MachineConfigs with the correct `osImageURL`.

```text
 NodePool API              NodePool Controller              Ignition Server
 ────────────              ───────────────────              ───────────────
 spec.osImageStream:
   name: "rhel-10"
       │
       ▼
 Resolve stream ────────▶ Token Secret
 + boot AMI                 os-stream: "rhel-10"  ◄── NEW
                            token: <uuid>
                            release: <image>
                            config: <compressed>
                            pull-secret-hash: <hash>
                            ...
                                    │
                                    │ TokenSecretReconciler
                                    │ reads token secret
                                    ▼
                            GetPayload(..., osStream)
                                                    │
                                                    ▼
                                              Write 99_osimagestream.yaml
                                              spec.defaultStream = osStream
                                                    │
                                                    ▼
                                              MCC bootstrap renders
                                              MachineConfigs with
                                              osImageURL = rhel-10 image
                                                    │
                                                    ▼
                                              MCS embeds rendered MC in
                                              ignition as encapsulated JSON
```

On the node, the first-boot `machine-config-daemon` (a one-shot podman container, not a long-running daemon) reads the rendered MachineConfig from `/etc/ignition-machine-config-encapsulated.json` and runs `rpm-ostree rebase ostree-unverified-registry:<osImageURL>`. The node reboots into the selected RHEL version.

### Workflow Description

**Cluster administrator** creates a NodePool with an explicit stream:

1. Administrator creates a NodePool with `spec.osImageStream.name: "rhel-10"` on a 5.0 HostedCluster.
2. NodePool controller validates the stream against the release version. Since 5.0 >= 5.0, validation passes.
3. NodePool controller resolves the RHEL 10 boot AMI from the release payload's `streams` ConfigMap key.
4. NodePool controller writes the stream to the token secret as `os-stream`. The config hash includes the stream, producing a unique token secret name.
5. Ignition server's `TokenSecretReconciler` reads `os-stream` from the token secret and calls `GetPayload()` with the stream.
6. `GetPayload()` writes `99_osimagestream.yaml` with `spec.defaultStream: "rhel-10"` to the MCC manifest directory.
7. MCC bootstrap discovers `rhel-9` and `rhel-10` streams from OCI labels, selects `rhel-10` per the OSImageStream CR, renders MachineConfigs with RHEL 10 `osImageURL`.
8. Node boots the RHEL 10 AMI, fetches ignition, first-boot MCD rebases to the RHEL 10 node image, node reboots and joins the cluster.
9. NodePool controller observes `node.Status.NodeInfo.OSImage`, confirms RHEL 10, sets `status.osImageStream.name: "rhel-10"`.

**Cluster administrator creates a NodePool without explicit stream on 5.0:**

1. Administrator creates a NodePool on a 5.0 HostedCluster with no `spec.osImageStream` set.
2. NodePool controller resolves the default stream for release >= 5.0: `rhel-10`.
3. NodePool controller resolves the RHEL 10 boot AMI from the release payload's `streams` ConfigMap key.
4. NodePool controller writes `os-stream: "rhel-10"` to the token secret. The config hash does NOT include the stream (implicit), so existing token secrets are reused — no accidental rollout.
5. Ignition server generates payload with `spec.defaultStream: "rhel-10"`.
6. Node boots RHEL 10 AMI, first-boot MCD rebases to RHEL 10 node image.
7. `status.osImageStream.name` reports `rhel-10`.

**Explicit opt-in to rhel-10 on 4.23:**

1. Administrator creates a NodePool with `spec.osImageStream.name: "rhel-10"` on a 4.23 HostedCluster.
2. NodePool controller validates the stream against the release version. Since 4.23 >= 4.23, validation passes.
3. NodePool controller resolves the RHEL 10 boot AMI from the release payload's `streams` ConfigMap key.
4. The flow continues as in the explicit stream workflow above. The default remains `rhel-9` for 4.x, but the user has explicitly opted in to `rhel-10`.

**Validation failure — rhel-10 on pre-4.23 release:**

1. Administrator creates a NodePool with `spec.osImageStream.name: "rhel-10"` on a 4.19 HostedCluster.
2. NodePool controller checks the release version (4.19 < 4.23) and sets `NodePoolValidMachineConfigConditionType=False` with message "OS stream rhel-10 requires release version >= 4.23, got 4.19.x".
3. Reconciliation short-circuits. No machines are created.

**Implicit upgrade with runc guard:**

1. Administrator upgrades a HostedCluster from 4.19 to 5.0. An existing NodePool has no `spec.osImageStream` (would default to `rhel-10`) but has a MachineConfig setting `default_runtime = "runc"`.
2. NodePool controller's `validMachineConfigCondition` calls `getRHELStream()` which detects runc and returns `"rhel-9"` (fallback).
3. The condition is set to `NodePoolValidMachineConfigConditionType=True` with message "OS stream defaulted to rhel-9: RHEL 10 is incompatible with default_runtime=runc". The NodePool stays on RHEL 9.

### API Extensions

#### NodePoolSpec

This mirrors [`MachineConfigPoolSpec.osImageStream`](https://github.com/openshift/api/blob/master/machineconfiguration/v1/types.go) in standalone clusters:

```go
type NodePoolSpec struct {
    // ...existing fields...

    // osImageStream specifies an OS stream to be used for nodes in this pool.
    //
    // This field can be optionally set to a known OSImageStream name to change
    // the OS and Extension images with a well-known, tested, release-provided
    // set of images. This enables a streamlined way of switching the pool's
    // node OS to a different version than the cluster default, such as
    // transitioning to a major RHEL version.
    //
    // When set, the referenced stream overrides the default OS images for the
    // pool. Explicit opt-in to rhel-10 is supported starting from OCP 4.23.
    // When omitted, the pool uses the release version's default stream
    // (rhel-9 for OCP < 5.0, rhel-10 for OCP >= 5.0).
    // Changing this field triggers a rollout. Forward transitions
    // (rhel-9 → rhel-10) are allowed; backward transitions
    // (rhel-10 → rhel-9) are rejected by CEL validation because
    // in-place OS downgrades are not supported.
    //
    // +openshift:enable:FeatureGate=OSStreams
    // +optional
    OSImageStream OSImageStreamReference `json:"osImageStream,omitempty,omitzero"`
}

// OSImageStreamReference is a reference to an OSImageStream.
// Matches machineconfiguration/v1.OSImageStreamReference.
type OSImageStreamReference struct {
    // name is a required reference to an OSImageStream to be used for the pool.
    //
    // +required
    // +kubebuilder:validation:Enum=rhel-9;rhel-10
    Name string `json:"name,omitempty"`
}

// CEL transition rule on NodePoolSpec:
// +kubebuilder:validation:XValidation:rule="!has(oldSelf.osImageStream) || !has(oldSelf.osImageStream.name) || oldSelf.osImageStream.name != 'rhel-10' || !has(self.osImageStream) || self.osImageStream.name != 'rhel-9'",message="OS stream downgrade from rhel-10 to rhel-9 is not allowed; create a new NodePool instead"
```

#### NodePoolStatus

```go
type NodePoolStatus struct {
    // ...existing fields...

    // osImageStream specifies the last updated OSImageStream for the pool.
    //
    // When omitted, the pool is using the release version's default OS images.
    // +openshift:enable:FeatureGate=OSStreams
    // +optional
    OSImageStream OSImageStreamReference `json:"osImageStream,omitempty,omitzero"`
}
```

#### UX Behavior

| `spec.osImageStream.name` | Release | What happens |
| ------------------------- | ------- | ------------ |
| unset | < 4.23 | Legacy single-stream behavior. No OSImageStream CR generated. |
| unset | 4.23 – 4.x | Ignition server injects OSImageStream CR with `spec.defaultStream: "rhel-9"`. User can opt-in to `rhel-10` explicitly. |
| unset | 5.x | Ignition server injects OSImageStream CR with `spec.defaultStream: "rhel-10"` |
| `"rhel-9"` | >= 4.23 | OSImageStream CR with `spec.defaultStream: "rhel-9"` |
| `"rhel-10"` | < 4.23 | **Rejected.** `NodePoolValidMachineConfigCondition=False`. Payload does not carry RHEL 10 images. |
| `"rhel-10"` | 4.23 – 4.x | OSImageStream CR with `spec.defaultStream: "rhel-10"` (explicit opt-in) |
| `"rhel-10"` | >= 5.0 | OSImageStream CR with `spec.defaultStream: "rhel-10"` |
| `"rhel-10"` + runc MachineConfig | >= 4.23 | **Rejected.** `NodePoolValidMachineConfigCondition=False`. RHEL 10 does not ship runc. |
| `"rhel-9"` (was `"rhel-10"`) | any | **Rejected by CEL.** OS stream downgrade from rhel-10 to rhel-9 is not allowed. |
| unset + runc MachineConfig | >= 5.0 | Falls back to `rhel-9` for both boot AMI and ignition payload stream. `NodePoolValidMachineConfigCondition=True` with informational message. |

No HostedCluster-level field is needed. Each NodePool independently selects its stream. A cluster can have mixed NodePools (RHEL 9 + RHEL 10 workers).

### Topology Considerations

#### Hypershift / Hosted Control Planes

This enhancement is specific to HyperShift. It adds the OS stream dimension to the NodePool API, token secret, ignition server, and boot image resolution. No changes to management cluster components beyond the control plane namespace.

The key external dependency is the MCO's `ExternalTopologyMode` guard ([PR #5750](https://github.com/openshift/machine-config-operator/pull/5750)) which currently skips OSImageStream processing when `controlPlaneTopology == External`. This guard must be removed for the MCC bootstrap to consume the OSImageStream CR in HyperShift.

#### Standalone Clusters

Standalone clusters already have dual-stream support via the MCO's OSImageStream CRD and the installer's `osImageStream` field. This enhancement does not modify standalone behavior.

#### Single-node Deployments or MicroShift

Not applicable. HyperShift does not support single-node or MicroShift deployments.

#### OpenShift Kubernetes Engine

The `spec.osImageStream` field is additive and optional. OKE clusters using HyperShift benefit from the same stream selection capability. No OKE-specific considerations.

### Implementation Details/Notes/Constraints

#### Implementation Phases

**Phase 0: TechPreview NodePool API**

1. Add `spec.osImageStream` to `NodePoolSpec` — mirrors `MachineConfigPoolSpec.osImageStream` from `openshift/api`. When unset, the system derives the default from the release version.
2. Add `status.osImageStream` — reports the observed RHEL stream running on nodes. The NodePool controller infers this from the CAPI `Machine` objects' `NodeInfo.OSImage` field
   (e.g., `"Red Hat Enterprise Linux CoreOS 419.97.202505081234-0 (Plow)"` for RHEL 9, `"Red Hat Enterprise Linux CoreOS 510.97..."` for RHEL 10).
   The major version in the RHCOS version string (`4xx` = RHEL 9, `5xx` = RHEL 10) determines the stream.
   The status is set once a majority of nodes in the pool report a consistent OS version, matching the pattern used by `status.version` for release version reporting.

**Phase 1: Update CoreOSStreamMetadata Parsing**

1. Update `DeserializeImageMetadata` (`support/releaseinfo/deserialize.go`) — parse the `streams` key when present, fall back to `stream` for older payloads. Return stream-indexed metadata.
2. Update `CoreOSStreamMetadata` (`support/releaseinfo/releaseinfo.go`) — support multiple streams (map of stream name to per-arch metadata).
3. Wire default boot image resolution per platform — accept stream parameter and look up stream-specific boot images from the parsed metadata:
   - **AWS**: `defaultNodePoolAMI` — resolve per-region AMI for the selected stream. Both standard NodePools and Karpenter use this function; for Karpenter, pass the version-derived default.
   - **Azure**: resolve the stream-specific VHD image URL.
   - **GCP**: resolve the stream-specific GCE image.
   - **KubeVirt/OpenStack/Agent**: resolve the stream-specific disk image or container image as applicable.
   All platforms fall back to the legacy `stream` key for payloads that don't carry the multi-stream `streams` key.
   If the `streams` key is present but the requested stream has no boot image for the target platform or region,
   the resolution function returns an error surfaced through `setPlatformConditions` as `NodePoolValidPlatformImageType=False`
   — consistent with how missing boot images are already handled today.

4. **Karpenter** — Karpenter uses the same `defaultNodePoolAMI` function as standard NodePools for boot image resolution.
   Since Karpenter NodePools have no `spec.osImageStream` field, the version-derived default stream is always passed
   (rhel-9 for < 5.0, rhel-10 for >= 5.0). The in-memory NodePool created by `KarpenterIgnitionReconciler.createInMemoryNodePool()`
   carries no `osImageStream`, so the ignition payload uses the default stream. The AMI label scheme (`hypershift.openshift.io/ami`)
   currently assumes one AMI per architecture; extending it with per-stream labels is out of scope for this enhancement
   and would be addressed if Karpenter gains explicit stream selection in a future phase. No changes to `OpenshiftEC2NodeClass` are planned.

**Phase 2: OS Stream Plumbing into Payload Generation**

Thread the OS stream selection from the NodePool controller through the token secret to the ignition server, where it drives OSImageStream CR generation for the MCC bootstrap pipeline. No new NodePool API fields in this phase — the API was added in Phase 0.

The following diagram shows how the OS stream propagates through the ignition server's bootstrap pipeline. The annotated pipeline shows the three existing stages plus the new injection point:

```text
GetPayload(releaseImage, customConfig, ..., osStream)
│
├─ 1. runFeatureGateRender()
│     Writes FeatureGate manifest (with OSStreams gate) to mccDir.
│
├─ 2. runMCO()
│     Executes: machine-config-operator bootstrap \
│       --image-references=<release>/image-references \
│       --payload-version=<version> ...
│     Produces: {destDir}/bootstrap/manifests/
│       ├── 99_openshift-machineconfig_99-master-ssh.yaml
│       ├── 99_openshift-machineconfig_99-worker-ssh.yaml
│       ├── controllerconfig.yaml  (BaseOSContainerImage = "rhel-coreos" tag)
│       └── ...
│     Then internally calls copyMCOOutputToMCC():
│       Copies MCO output manifests → mccDir
│       Copies CPO pool overrides (*.machineconfigpool.yaml) → mccDir
│
├─ 3. *** NEW: Write OSImageStream CR ***
│     Writes 99_osimagestream.yaml directly to mccDir with:
│       apiVersion: machineconfiguration.openshift.io/v1alpha1
│       kind: OSImageStream
│       metadata:
│         name: cluster
│       spec:
│         defaultStream: "<osStream>"   # e.g., "rhel-10"
│
├─ 4. runMCC()
│     Executes: machine-config-controller bootstrap \
│       --manifest-dir=<mccDir> ...
│     MCC reads all manifests from mccDir including:
│       - ControllerConfig → initial BaseOSContainerImage
│       - FeatureGate → checks OSStreams gate
│       - OSImageStream → spec.defaultStream = "rhel-10"
│     When OSStreams gate is active:
│       a. fetchOSImageStream() inspects OCI labels on release images
│          to discover available streams (rhel-9, rhel-10)
│       b. Selects the stream matching spec.defaultStream
│       c. Overrides ControllerConfig.baseOSContainerImage with the
│          selected stream's image pullspec
│     Renders final MachineConfigs with osImageURL pointing to the
│     selected stream's container image.
│
└─ 5. runMCSAndFetchPayload()
      Executes: machine-config-server
      Reads rendered MachineConfigs, produces ignition JSON payload.
```

Implementation steps:

1. **Add `usesRunc` field to `ConfigGenerator`** — during `generateMCORawConfig()` (`config.go`), check whether the NodePool's config references
   a `ContainerRuntimeConfig` CR with `spec.containerRuntimeConfig.defaultRuntime == "runc"`.
   The MCO implements the equivalent runc upgrade guard in [PR #5891](https://github.com/openshift/machine-config-operator/pull/5891).
   In HyperShift, the `ContainerRuntimeConfig` CRs are embedded in ConfigMaps referenced by `NodePool.spec.config` —
   the `ConfigGenerator` already parses these ConfigMaps, so the runc check can inspect the deserialized objects directly.
   Set `cg.usesRunc = true` if found. This makes the runc signal available to downstream consumers without re-parsing the config.

2. **Add `getRHELStream()` pure function** — a unit-testable function that encapsulates all stream resolution logic:

   ```go
   // getRHELStream resolves the RHEL stream for a NodePool.
   // Returns the stream name ("rhel-9" or "rhel-10") or an error for
   // invalid combinations.
   func getRHELStream(
       specStream string,        // NodePool.Spec.OSImageStream.Name (may be "")
       releaseVersion semver.Version,
       usesRunc bool,
   ) (string, error)
   ```

   Decision logic:
   - **Explicit `rhel-10` + runc** → return error (RHEL 10 does not ship runc)
   - **Explicit `rhel-10` + release < 4.23** → return error (not available — payload does not carry RHEL 10 images)
   - **Explicit value set** → return it as-is (allowed for >= 4.23)
   - **Unset + release >= 5.0 + runc** → return `"rhel-9"` (fallback)
   - **Unset + release >= 5.0** → return `"rhel-10"` (default)
   - **Unset + release >= 4.23 and < 5.0** → return `"rhel-9"` (default for 4.x, opt-in only)
   - **Unset + release < 4.23** → return `""` (no stream, legacy behavior)

   When the function returns `""`, the system uses the existing single-stream code path — no OSImageStream CR is generated,
   no stream is included in the hash, and the MCC uses `BaseOSContainerImage` from ControllerConfig as-is.

3. **Call `getRHELStream()` from `NewToken()`** — `NewToken()` (`token.go`) already receives the `ConfigGenerator`
   (which has `usesRunc` and `releaseImage.Version()`), and has access to the NodePool spec.
   It calls `getRHELStream()` and stores the result in a new `rhelStream string` field on the `Token` struct.

   The reconcile flow in the NodePool controller is:
   ```text
   signalConditions loop    → validMachineConfigCondition (EXTENDED, see step 4)
                             → ...
   setPlatformConditions()  → (platform-specific conditions)
   getReleaseImage()        → parsed release version
   NewConfigGenerator()     → assembles config, sets usesRunc
   NewToken()               → calls getRHELStream(), sets rhelStream
   token.Reconcile()        → creates token secret with os-stream
   ```

4. **Extend `validMachineConfigCondition` with RHEL stream validation** — `validMachineConfigCondition` (`conditions.go`) already instantiates
   a `ConfigGenerator` to validate the NodePool's config. After the existing config validation succeeds, call `getRHELStream()` with the
   NodePool's `spec.osImageStream.name`, the release version, and `configGenerator.usesRunc`.
   On error, set `NodePoolValidMachineConfigConditionType=False` and short-circuit:

   - **Explicit `rhel-10` on release < 4.23** → reason `InvalidMachineConfig`, message "OS stream rhel-10 requires release version >= 4.23".
   - **Explicit `rhel-10` with runc** → reason `InvalidMachineConfig`, message "OS stream rhel-10 is incompatible with default_runtime=runc; RHEL 10 does not ship runc".
   - **Implicit upgrade to >= 5.0 with runc (fallback to `rhel-9`)** → set `NodePoolValidMachineConfigConditionType=True`
     with an informational message "OS stream defaulted to rhel-9: RHEL 10 is incompatible with default_runtime=runc".
     This is not an error — reconciliation continues.

   Because `validMachineConfigCondition` runs before `NewToken()`, invalid combinations are caught early and no token secret is created for rejected NodePools.

5. **Write `os-stream` to the token secret** — `token.reconcileTokenSecret()` writes `token.rhelStream` into the token secret's `Data` map as a new `os-stream` key. Add a `TokenSecretOSStreamKey = "os-stream"` constant alongside the existing `TokenSecretReleaseKey`, etc.

6. **Include stream in the config hash** — add a `rhelStream string` field to the `rolloutConfig` struct (`config.go`).
   This field is only set when the user explicitly sets `spec.osImageStream.name` — implicitly resolved streams leave it as `""`.
   Append `rhelStream` to the hash inputs in `Hash()` and `HashWithoutVersion()` alongside the existing fields
   (`mcoRawConfig`, `releaseImage.Version()`, `pullSecretName`, etc.). Since `""` is a no-op in the string concatenation hash,
   implicit streams never change the hash. This ensures that:
   - Existing NodePools with no explicit stream produce the same hash as before the feature is introduced, avoiding accidental rollouts on upgrade.
   - Upgrading to >= 5.0 (where the implicit default shifts to `rhel-10`) does not trigger a rollout from the stream alone — the rollout is already driven by the release version component (`releaseImage.Version()`).

   **Design invariant**: the `rhelStream` field in `rolloutConfig` MUST be populated directly from `spec.osImageStream.name`
   (empty string when unset), never from the resolved return value of `getRHELStream()`. Using the resolved value would inject
   a non-empty default (e.g., `"rhel-10"` for >= 5.0) into the hash for every NodePool without an explicit field,
   triggering a fleet-wide mass rollout on upgrade.

7. **Pass stream through `IgnitionProvider` interface** — add `osStream string` parameter to `GetPayload()` in the `IgnitionProvider` interface (`tokensecret_controller.go`). The `TokenSecretReconciler` reads `os-stream` from the token secret data and passes it to `GetPayload()`. Update the mock `IgnitionProvider` in tests accordingly.

   ```go
   type IgnitionProvider interface {
       GetPayload(ctx context.Context, payloadImage, config, pullSecretHash,
           additionalTrustBundleHash, hcConfigurationHash, osStream string) ([]byte, error)
   }
   ```

8. **Generate `99_osimagestream.yaml` in `GetPayload()`** — after `copyMCOOutputToMCC()` returns and before `runMCC()` is called
   (`local_ignitionprovider.go`, between current lines 742 and 745), write the OSImageStream CR directly to `dirs.mccDir`.
   The CR only needs `spec.defaultStream` set — the MCC's `fetchOSImageStream()` uses this as the selection input and populates
   `status.availableStreams` itself via OCI label inspection. If `osStream` is empty (feature gate disabled or pre-5.0 payload),
   skip writing the CR — the MCC will not run stream discovery and will use the `BaseOSContainerImage` from ControllerConfig as-is.

9. **Coordinate MCO removal of `ExternalTopologyMode` guard** — the MCC bootstrap (`pkg/controller/bootstrap/bootstrap.go:238`) gates stream discovery on two conditions:
   ```go
   if osimagestream.IsFeatureEnabled(fgHandler) &&
       cconfig.Spec.Infra.Status.ControlPlaneTopology != apicfgv1.ExternalTopologyMode {
   ```
   Both conditions must be addressed:
   - **ExternalTopologyMode guard**: MCO PR #5750 added this skip. It must be removed so MCC bootstrap runs stream discovery for HyperShift.
   - **FeatureGate**: the ignition server's `runFeatureGateRender()` already writes the FeatureGate manifest to `mccDir` based on the HCP's feature gate configuration. When the HCP has TechPreview enabled, the rendered FeatureGate includes `OSStreams`. No additional changes needed for the feature gate condition — it is already satisfied by the existing pipeline.

10. **Report `status.osImageStream`** from observed node state. The NodePool controller reads `node.Status.NodeInfo.OSImage` (which contains the RHEL version) and sets `status.osImageStream.name` accordingly.

11. **E2E tests** — add a new test case to the existing `TestNodePool` suite that creates 8 additional NodePools to validate all stream scenarios in parallel. Runs in `e2e-test-preview` until GA. See Test Plan for details.

**Phase 3: GA API**

1. Promote `spec.osImageStream` from TechPreview to GA — remove feature gate. This must be coordinated with the MachineConfigPool `OSStreams` feature gate promotion in `openshift/api`, so both APIs go GA together.
2. Promote E2E tests from TechPreview job to default CI jobs.
3. Documentation and user-facing guidance.

#### Existing Mechanisms

These components are already implemented and not changed by this enhancement. They are documented here for context on how the OS stream selection ultimately takes effect.

##### Boot Image ConfigMap Format

The release payload's boot image ConfigMap (`0000_50_installer_coreos-bootimages.yaml`) ships with three data keys starting in 5.0. You can extract it from a release payload with:

```sh
oc adm release extract --to=/tmp/bootimages <release-image>
cat /tmp/bootimages/0000_50_installer_coreos-bootimages.yaml
```

The format was changed in [installer#10321](https://github.com/openshift/installer/pull/10321) (merged):

- `stream` — legacy single-stream JSON blob (the default stream), preserved for backward compatibility
- `streams` — new multi-stream map keyed by stream name (`"rhel-9"`, `"rhel-10"`), each containing per-architecture, per-region boot images
- `releaseVersion` — payload version string

Abbreviated example extracted from a 5.0 nightly payload:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: coreos-bootimages
  namespace: openshift-machine-config-operator
data:
  releaseVersion: 5.0.0-0.nightly-2026-05-20-043516
  stream: |-          # legacy single-stream (rhel-9, backward compat)
    {
      "stream": "rhcos-4.21",
      "architectures": {
        "x86_64": {
          "images": {
            "aws": {
              "regions": {
                "us-east-1": { "image": "ami-06a6b025350ff1e23" }
              }
            }
          }
        }
      }
    }
  streams: |-         # NEW: multi-stream keyed by name
    {
      "rhel-9": {
        "stream": "rhcos-4.21",
        "architectures": {
          "x86_64": {
            "images": {
              "aws": {
                "regions": {
                  "us-east-1": { "image": "ami-06a6b025350ff1e23" }
                }
              }
            }
          }
        }
      },
      "rhel-10": {
        "stream": "rhcos-4.22",
        "architectures": {
          "x86_64": {
            "images": {
              "aws": {
                "regions": {
                  "us-east-1": { "image": "ami-04b3d999e39d62c5b" }
                }
              }
            }
          }
        }
      }
    }
```

##### MCC Bootstrap: How Stream Discovery Works

The MCC bootstrap (`pkg/controller/bootstrap/bootstrap.go` in the MCO) scans its `--manifest-dir` for all YAML files. When it finds an OSImageStream CR and the `OSStreams` feature gate is enabled, it:

1. Calls `filterImageTag()` to select release ImageStream tags built from `github.com/openshift/os`
2. Calls `BulkInspector.Inspect()` to fetch OCI labels from each image
3. Classifies images by `io.openshift.os.streamclass` label and `containers.bootc=1` / `io.openshift.os.extensions=true` labels
4. Groups into streams via `GroupOSContainerImageMetadataToStream()`
5. Selects the stream matching `spec.defaultStream`
6. Overrides `ControllerConfig.baseOSContainerImage` with the selected stream's image

HyperShift only needs to provide the OSImageStream CR — the MCO handles all discovery and resolution.

##### First-Boot OS Application

The ignition payload includes systemd units from MCC templates that drive OS image application on the node:

1. `machine-config-daemon-pull.service` — pulls the MCD container image
2. `machine-config-daemon-firstboot.service` — runs the MCD as a one-shot podman container (not a long-running daemon)
3. The MCD reads `/etc/ignition-machine-config-encapsulated.json` (the rendered MachineConfig with `osImageURL`) and runs `rpm-ostree rebase ostree-unverified-registry:<osImageURL>`
4. The node reboots into the full OCP node image

The boot AMI does not need to match the target stream. A RHEL 9 boot AMI can rebase to a RHEL 10 node image — `rpm-ostree` deploys a completely new ostree commit. However, resolving stream-matched boot AMIs avoids the cross-stream rebase.

##### OS Image Build Pipeline

The node OS images are built by two repositories:

- [coreos/rhel-coreos-config](https://github.com/coreos/rhel-coreos-config) — builds the base CoreOS image using `coreos-assembler`. Contains treefile manifests per RHEL version. Produces boot disk images (AMIs, VHDs).
- [openshift/os](https://github.com/openshift/os) — builds the OpenShift node image. `Containerfile` does `FROM <base CoreOS>` and adds kubelet, cri-o, oc. Also builds extensions images via `extensions/Containerfile`.

The `rhel-coreos*` images are NOT runtime containers — they are ostree commits packaged as OCI images. The container format reuses registry infrastructure for OS delivery.

### Risks and Mitigations

1. **MCO ExternalTopologyMode guard removal.** The MCO team intentionally deferred HyperShift support. Removing the guard requires demonstrating that HyperShift provides valid OSImageStream and FeatureGate manifests. *Mitigation:* The spike simulation has verified end-to-end success with a fixed nightly payload when the guard is bypassed.

2. **Boot image ConfigMap format evolution.** The `streams` key format may change before GA. *Mitigation:* The parsing code falls back to the legacy `stream` key for payloads that don't have `streams`.

3. **runc detection false negatives.** A user could configure runc via a mechanism other than CRI-O drop-in files. *Mitigation:* Match the MCO's detection logic exactly. If the MCO considers it sufficient for standalone, it is sufficient for HyperShift.

4. **Mixed-stream clusters.** A HostedCluster with RHEL 9 and RHEL 10 NodePools is a new topology. Component compatibility across RHEL versions must be validated. *Mitigation:* This is the same topology supported by standalone clusters via per-MachineConfigPool stream selection.

5. **Disconnected environments require mirroring both streams.** Dual-stream payloads carry two sets of node OS container images
   (one per RHEL stream). Disconnected customers using both streams must mirror both sets. Boot images are platform-specific
   and already handled outside the payload (e.g. pre-uploaded AMIs, VHDs). The OS container images are referenced via
   `ImageDigestMirrorSet` / `ImageTagMirrorSet` — existing IDMS/ITMS mirroring workflows handle this transparently as long as
   both stream images are included in the mirror list. No additional HyperShift-specific mirroring tooling is needed.

### Drawbacks

The primary drawback is additional complexity in the ignition pipeline. The token secret gains a new field,
the `IgnitionProvider` interface changes, and the `GetPayload` function must generate an additional manifest.
However, this complexity mirrors what the installer already does for standalone clusters — it is not HyperShift-specific logic
but rather bringing HyperShift into parity with the standalone bootstrap flow.

## Alternatives (Not Implemented)

### Let MCO default the stream without HyperShift involvement

The MCO's `GetBuiltinDefaultStreamName()` already returns a version-based default. HyperShift could skip generating the OSImageStream CR and let the MCO choose.

This was rejected because:
- It removes per-NodePool stream control — all NodePools would get the same default stream
- The ignition version and config hash would not include the stream dimension, causing incorrect payload matches
- It does not address the ExternalTopologyMode guard, which skips stream processing entirely

## Open Questions

1. ~~Should `spec.osImageStream` be immutable after creation (requiring NodePool replacement to change streams),
   or mutable (triggering a rolling update)?~~ **Resolved**: The field is mutable but only forward transitions are allowed.
   A CEL transition rule prevents changing from `rhel-10` to `rhel-9` — in-place OS downgrades are not supported.
   Forward transitions (`rhel-9` → `rhel-10`) trigger a rollout. To move back to `rhel-9`, users must create a new NodePool.

## Test Plan

- **Unit tests**:
  - `DeserializeImageMetadata` parses both `stream` (legacy) and `streams` (multi-stream) ConfigMap formats.
  - `defaultNodePoolAMI` resolves correct AMI for each stream/arch/region combination.
  - `ConfigGenerator.Hash()` produces different hashes for different explicit streams, but identical hashes when stream is implicit.
  - `getRHELStream()` returns correct defaults for various release versions.
  - runc detection identifies `ContainerRuntimeConfig` CRs with `spec.containerRuntimeConfig.defaultRuntime == "runc"` in NodePool config.
  - Validation rejects `rhel-10` on release < 4.23.
  - Token secret contains `os-stream` key with correct value.
  - `GetPayload` generates `99_osimagestream.yaml` with correct `spec.defaultStream`.
  - Config hash changes when an explicit `spec.osImageStream.name` is set, but remains unchanged when stream is implicit (empty `rhelStream`).

- **E2E tests**: Add a new test case to the existing `TestNodePool` suite that creates 9 additional NodePools to validate all stream scenarios in parallel:
  1. **Explicit rhel-9**: NodePool with `osImageStream.name: "rhel-9"`. Verify nodes report RHEL 9 via `node.Status.NodeInfo.OSImage`.
  2. **Explicit rhel-10**: NodePool with `osImageStream.name: "rhel-10"`. Verify nodes report RHEL 10.
  3. **Implicit default**: NodePool with no `osImageStream`. Verify nodes run the release version's default (RHEL 10 for >= 5.0).
  4. **Validation rejection**: NodePool with `osImageStream.name: "rhel-10"` on a < 4.23 release. Verify `NodePoolValidMachineConfigConditionType=False` and no machines created.
  5. **Explicit opt-in on 4.23**: NodePool with `osImageStream.name: "rhel-10"` on a 4.23 release. Verify nodes boot RHEL 10 while the cluster default remains RHEL 9.
  6. **Runc rejection**: NodePool with runc `ContainerRuntimeConfig` and `osImageStream.name: "rhel-10"`. Verify `NodePoolValidMachineConfigConditionType=False`.
  7. **Runc fallback**: NodePool with runc `ContainerRuntimeConfig` and no explicit `osImageStream` on >= 5.0. Verify it stays on RHEL 9 with informational condition message.
  8. **Upgrade implicit stream switch (Replace)**: NodePool with `upgradeType: Replace` on a < 5.0 release (implicitly rhel-9). Upgrade the NodePool to a 5.0+ release. Verify nodes are replaced and report RHEL 10 as the new implicit default.
  9. **Upgrade implicit stream switch (InPlace)**: NodePool with `upgradeType: InPlace` on a < 5.0 release (implicitly rhel-9). Upgrade the NodePool to a 5.0+ release. Verify nodes rebase to RHEL 10 in place.

  HyperShift will run this test case only in the `e2e-test-preview` test suite until the NodePool API fields GA. Additionally the test will adjust the TestNodePool HostedCluster's TechPreview feature set as needed to pick up the MCO's `OSStreams` feature gate.

## Graduation Criteria

### Dev Preview -> Tech Preview

- `spec.osImageStream` field available behind TechPreview feature gate.
- Boot image ConfigMap parsing supports multi-stream format.
- Stream plumbed through token secret to ignition server.
- OSImageStream CR generated in `GetPayload()`.
- MCO ExternalTopologyMode guard removed (coordinated with MCO team).
- Basic E2E coverage on TechPreview CI job.
- runc upgrade guard implemented.

### Tech Preview -> GA

- Promote `spec.osImageStream` from TechPreview to GA. The NodePool API field promotion must be coordinated with the MachineConfigPool `OSStreams` feature gate promotion in `openshift/api`, so both APIs go GA together.
- Promote E2E tests from TechPreview job to default CI jobs.
- Validation across all supported platforms (AWS, Azure, KubeVirt, etc.).
- Documentation and user-facing guidance.
- Upgrade/downgrade testing.

### Removing a deprecated feature

N/A. This is a new feature.

## Upgrade / Downgrade Strategy

**Upgrade to a version with this feature:**
- Existing NodePools with no `spec.osImageStream` continue to work. The system derives the default from the release version. For < 5.0, this is `rhel-9` (same as before). For 5.0+, this is `rhel-10`.
- If a NodePool has runc MachineConfigs and the release is 5.0+, the system automatically falls back to `rhel-9` and reports the override via a condition.
- No manual action required.

**Downgrade to a version without this feature:**
- Downgrades are not supported. HyperShift does not provide guardrails against NodePool downgrades today, but downgrading to a release that predates this feature is not a tested or supported workflow.

## Version Skew Strategy

The stream is resolved at ignition generation time and baked into the MachineConfig's `osImageURL`. There is no ongoing version skew concern — once a node boots and rebases, it runs the pinned OS image regardless of controller version.

During upgrades, the NodePool controller and ignition server are updated together (both are part of the HyperShift Operator / CPO deployment). There is no window where one component understands streams and the other does not.

## Operational Aspects of API Extensions

The `spec.osImageStream` field is optional and has no default value in the API schema. The default is computed at runtime by the NodePool controller and ignition server based on the release version. This means:

- Existing NodePools are unaffected — no field is set, behavior is unchanged for < 5.0 releases.
- The field can be set or changed at any time, triggering a rollout.
- Removing the field (setting to empty) reverts to the version-derived default.

## Support Procedures

- **Detecting stream issues**: Check `status.osImageStream` on the NodePool. If empty during normal operation (not mid-rollout), nodes may not have converged. Check `NodePoolValidMachineConfigConditionType` condition for validation failures.

- **Diagnosing ignition failures**: If a NodePool with `osImageStream` set fails to create machines, check:
  - Token secret exists and contains `os-stream` key
  - Ignition server logs for `GetPayload` errors
  - MCC bootstrap output for stream discovery failures
  - Whether the release payload contains the requested stream's images

- **runc fallback**: If a NodePool on 5.0+ is unexpectedly on RHEL 9, check for runc MachineConfigs in `spec.config` refs. The runc guard automatically overrides the stream to `rhel-9`.

## Implementation History

- 2026-05-21: Initial enhancement proposal based on spike findings (OCPSTRAT-3014).

## References

### MCO PRs

- [#5518](https://github.com/openshift/machine-config-operator/pull/5518) — Bootstrap MachineConfigPool rendering with osImageStream
- [#5650](https://github.com/openshift/machine-config-operator/pull/5650) — Image classification via OCI labels for multi-stream
- [#5750](https://github.com/openshift/machine-config-operator/pull/5750) — ExternalTopologyMode guard (skip OSImageStream in HyperShift)
- [#5770](https://github.com/openshift/machine-config-operator/pull/5770) — `machine-config-osimagestream` CLI binary
- [#5891](https://github.com/openshift/machine-config-operator/pull/5891) — runc upgrade guard for RHEL 10

### Installer PRs

- [#10321](https://github.com/openshift/installer/pull/10321) — Per-stream boot image metadata
- [#10357](https://github.com/openshift/installer/pull/10357) — Generate OSImageStream CR at day-0

### HyperShift PRs

- [#8101](https://github.com/openshift/hypershift/pull/8101) — `readComponentVersions()` tolerates multiple `machine-os` entries
- [#8128](https://github.com/openshift/hypershift/pull/8128) — Karpenter drift test handles dual RHCOS payloads

### OS Image Build Repos

- [coreos/rhel-coreos-config](https://github.com/coreos/rhel-coreos-config) — Base CoreOS image build (treefiles, boot disk images)
- [openshift/os](https://github.com/openshift/os) — OpenShift node image build (Containerfile, extensions)
