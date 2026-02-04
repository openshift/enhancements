---
title: additional-storage-config-crio
authors:
  - "@sgrunert"
reviewers:
  - "@haircommander"
  - "@QiWang19"
  - "@sairameshv"
  - "@yuqi-zhang"
  - "@bitoku"
  - "@harche"
approvers:
  - "@mrunalp"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-01-29
last-updated: 2026-01-29
status: provisional
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1285
  - https://issues.redhat.com/browse/OCPSTRAT-2623
  - https://issues.redhat.com/browse/OCPNODE-4050
  - https://issues.redhat.com/browse/OCPNODE-4051
  - https://issues.redhat.com/browse/OCPNODE-4052
see-also:
  - "/enhancements/machine-config/on-cluster-layering.md"
  - "/enhancements/machine-config/pin-and-pre-load-images.md"
replaces:
  - https://github.com/openshift/enhancements/pull/1600
superseded-by:
  - N/A
---

# Advanced Container Storage Configuration for CRI-O

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Operational readiness criteria is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Extend OpenShift's `ContainerRuntimeConfig` API to support additional storage configuration options in CRI-O: **additional layer stores** (for lazy image pulling), **additional image stores** (for read-only image caches), and **additional artifact stores** (for configurable artifact storage locations). These features target AI/ML workload performance improvements by reducing container startup times and enabling optimized storage utilization.

All three features share a unified API design pattern and MCO implementation approach, making them natural candidates for a single enhancement proposal.

## Motivation

### User Stories

#### Additional Layer Stores ([OCPSTRAT-1285](https://issues.redhat.com/browse/OCPSTRAT-1285))

- As an AI/ML Platform Operator, I want containers with large model images to start immediately without waiting for full image download, so that my applications are available faster and autoscaling is more responsive.
- As an OpenShift Administrator, I want to reduce pod startup time for AI workloads, so that autoscaling is more responsive and users have better experience.
- As an Application Developer, I want my containers to start quickly even with large images, so that my development iteration cycle is faster.

#### Additional Image Stores

- As a Cluster Admin, I want to pre-populate a read-only image cache on shared network storage (NFS), so that multiple nodes can share images without redundant pulls from external registries.
- As an Edge Deployment Operator, I want to use SSD-backed storage for frequently-used base images, so that container startup is faster and doesn't consume root filesystem space.
- As a Cluster Admin in an air-gapped environment, I want to pre-populate complete container images on nodes, so that pods can start without registry access.

#### Additional Artifact Stores ([OCPSTRAT-2623](https://issues.redhat.com/browse/OCPSTRAT-2623))

- As a RHOAI Platform Operator, I want to store large ML models on SSD storage, so that model loading is faster and doesn't consume root filesystem space.
- As a Cluster Admin in an air-gapped environment, I want to pre-populate artifact caches on nodes, so that pods can start without pulling from external registries.
- As an Edge Deployment Operator, I want to deliver artifacts via removable media (USB), so that edge nodes can operate offline without network connectivity.

### Goals

1. **Additional Layer Stores**: Enable lazy image pulling through storage plugin architecture, reducing container startup time for large images (~70% of container startup time is image pulling for large images)
2. **Additional Image Stores**: Enable read-only image caches on shared or high-performance storage, reducing network overhead and improving container startup times
3. **Additional Artifact Stores**: Enable configurable artifact storage locations, supporting high-performance storage and pre-populated caches
4. **Unified API**: Provide consistent, declarative configuration through ContainerRuntimeConfig
5. **Tech Preview for 4.22**: Deliver all three features behind AdditionalStorageConfig feature gate (enabled in TechPreviewNoUpgrade and DevPreviewNoUpgrade feature sets)

### Non-Goals

1. Shipping storage plugin binaries with OpenShift (BYOS - Bring Your Own Storage approach for layer stores)
2. Artifact pre-loading mechanisms (separate RFE-8441)
3. Write-capable artifact stores (read-only only for Tech Preview)
4. Dynamic artifact mirroring
5. Upstream Kubernetes KEP (deferred post-Tech Preview)
6. GA-level API stability in 4.22 (Tech Preview allows iteration)

## Proposal

### Workflow Description

#### Additional Layer Stores Workflow

1. Cluster administrator identifies slow pod startup times from large images (>5GB) and installs a storage plugin (e.g., stargz-store) on target nodes. Supported installation methods include:
   - **DaemonSet**: Run plugin as a privileged container (recommended for ease of management)
   - **Machine Config with systemd unit**: Install binary and configure as systemd service
   - **Image mode/layered RHCOS**: Pre-install plugin in custom RHCOS image (for disconnected environments)
2. Cluster administrator creates a `ContainerRuntimeConfig` with `additionalLayerStores`:

   ```yaml
   apiVersion: machineconfiguration.openshift.io/v1
   kind: ContainerRuntimeConfig
   metadata:
     name: enable-lazy-pulling
   spec:
     machineConfigPoolSelector:
       matchLabels:
         pools.operator.machineconfiguration.openshift.io/worker: ""
     containerRuntimeConfig:
       additionalLayerStores:
         - path: /var/lib/stargz-store
   ```

   **Important**: When using multiple ContainerRuntimeConfig resources, merge all additional storage configurations into a single ContainerRuntimeConfig per machine pool. Due to how ContainerRuntimeConfig to MachineConfig rendering is implemented, multiple ContainerRuntimeConfig resources affecting the same configuration file may result in only a subset taking effect.

3. MCO generates `storage.conf`, creates MachineConfig, and applies to selected pool; nodes reboot to apply changes (CRI-O reload/restart without reboot is not currently supported but may be considered for future releases)
4. When AI/ML platform operator deploys workload with eStargz/Nydus image, container-libs/storage accesses plugin's FUSE filesystem, triggering metadata download and lazy pulling; container starts after downloading only required chunks

**Fallback behaviors:** Standard pull if registry lacks HTTP range requests, image is standard OCI format, or plugin not running

#### Additional Image Stores Workflow

1. Storage administrator provisions shared NFS storage at `/mnt/nfs-images` and pre-populates with frequently-used container images
2. Cluster administrator creates `ContainerRuntimeConfig` with `additionalImageStores`:
   ```yaml
   apiVersion: machineconfiguration.openshift.io/v1
   kind: ContainerRuntimeConfig
   metadata:
     name: shared-image-cache
   spec:
     machineConfigPoolSelector:
       matchLabels:
         pools.operator.machineconfiguration.openshift.io/worker: ""
     containerRuntimeConfig:
       additionalImageStores:
         - path: /mnt/nfs-images
   ```
3. MCO generates `storage.conf`, creates MachineConfig, and applies to selected pool; nodes reboot
4. When workload requests container image, container-libs/storage checks additional image stores first; if found, uses cached image; otherwise pulls from registry

**Use case variations:** SSD-backed storage for performance-critical images, air-gapped deployments with pre-populated images, multi-node image sharing via NFS

#### Additional Artifact Stores Workflow

1. Storage administrator provisions SSD-backed storage at `/mnt/ssd-artifacts` and pre-populates with ML model artifacts
2. RHOAI administrator creates `ContainerRuntimeConfig` with `additionalArtifactStores`:
   ```yaml
   apiVersion: machineconfiguration.openshift.io/v1
   kind: ContainerRuntimeConfig
   metadata:
     name: ml-artifact-storage
   spec:
     machineConfigPoolSelector:
       matchLabels:
         node-role.kubernetes.io/ml-worker: ""
     containerRuntimeConfig:
       additionalArtifactStores:
         - path: /mnt/ssd-artifacts
   ```
3. MCO generates CRI-O config, creates MachineConfig, and applies to selected pool; nodes reboot
4. When workload references OCI volume artifact, CRI-O checks SSD storage first; if found, serves from there; otherwise pulls from registry

**Air-gapped variation:** Pre-populate artifacts on removable media, copy to additional store path, use without registry access

### API Extensions

#### ContainerRuntimeConfig Type Addition

Extend the existing `ContainerRuntimeConfig` type in `github.com/openshift/api/machineconfiguration/v1`:

```go
type ContainerRuntimeConfig struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    // Spec defines the desired state of ContainerRuntimeConfig
    Spec ContainerRuntimeConfigSpec `json:"spec"`
}

type ContainerRuntimeConfigSpec struct {
    // ... existing fields ...

    // additionalLayerStores configures additional layer store locations.
    //
    // Stores are checked in order until a layer is found.
    // Maximum of 5 stores allowed.
    // Each path must be unique.
    //
    // When omitted, no additional layer stores are configured.
    // When specified, at least one store must be provided.
    //
    // +openshift:enable:FeatureGate=AdditionalStorageConfig
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:MinItems=1
    // +kubebuilder:validation:MaxItems=5
    // +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.path == y.path))",message="additionalLayerStores must not contain duplicate paths"
    AdditionalLayerStores []AdditionalLayerStore `json:"additionalLayerStores,omitempty"`

    // additionalImageStores configures additional read-only container image store
    // locations for complete Open Container Initiative (OCI) images.
    //
    // Images are checked in order: additional stores first, then default location.
    // Stores are read-only.
    // Maximum of 10 stores allowed.
    // Each path must be unique.
    //
    // When omitted, only the default image location is used.
    // When specified, at least one store must be provided.
    //
    // +openshift:enable:FeatureGate=AdditionalStorageConfig
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:MinItems=1
    // +kubebuilder:validation:MaxItems=10
    // +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.path == y.path))",message="additionalImageStores must not contain duplicate paths"
    AdditionalImageStores []AdditionalImageStore `json:"additionalImageStores,omitempty"`

    // additionalArtifactStores configures additional read-only artifact storage
    // locations for Open Container Initiative (OCI) artifacts.
    //
    // Artifacts are checked in order: additional stores first, then default location.
    // Stores are read-only.
    // Maximum of 10 stores allowed.
    // Each path must be unique.
    //
    // When omitted, only the default artifact location (/var/lib/containers/storage/artifacts/) is used.
    // When specified, at least one store must be provided.
    //
    // +openshift:enable:FeatureGate=AdditionalStorageConfig
    // +optional
    // +listType=atomic
    // +kubebuilder:validation:MinItems=1
    // +kubebuilder:validation:MaxItems=10
    // +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x.path == y.path))",message="additionalArtifactStores must not contain duplicate paths"
    AdditionalArtifactStores []AdditionalArtifactStore `json:"additionalArtifactStores,omitempty"`
}

// AdditionalLayerStore defines a storage location for container image layers.
type AdditionalLayerStore struct {
    // path is the absolute path to the additional layer store location.
    //
    // The path must exist on the node before configuration is applied.
    // When a container image is requested, layers found at this location will be used instead of
    // retrieving from the registry.
    //
    // This field is required and must:
    //   - Have length between 1 and 256 characters
    //   - Start with '/' (absolute path)
    //   - Contain only: a-z, A-Z, 0-9, '/', '.', '_', '-' (no spaces or special characters)
    //
    // +required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=256
    // +kubebuilder:validation:XValidation:rule="self.matches('^/[a-zA-Z0-9/._-]+$')",message="path must be absolute and contain only alphanumeric characters, '/', '.', '_', and '-'"
    Path string `json:"path,omitempty"`
}

// AdditionalImageStore defines an additional read-only storage location for complete container images.
type AdditionalImageStore struct {
    // path is the absolute path to the additional image store location.
    //
    // The path must exist on the node before configuration is applied.
    // When a container image is requested, images found at this location will be used instead of
    // retrieving from the registry.
    //
    // This field is required and must:
    //   - Have length between 1 and 256 characters
    //   - Start with '/' (absolute path)
    //   - Contain only: a-z, A-Z, 0-9, '/', '.', '_', '-' (no spaces or special characters)
    //
    // +required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=256
    // +kubebuilder:validation:XValidation:rule="self.matches('^/[a-zA-Z0-9/._-]+$')",message="path must be absolute and contain only alphanumeric characters, '/', '.', '_', and '-'"
    Path string `json:"path,omitempty"`
}

// AdditionalArtifactStore defines an additional storage location for Open Container Initiative (OCI) artifacts.
type AdditionalArtifactStore struct {
    // path is the absolute path to the additional artifact store location.
    //
    // The path must exist on the node before configuration is applied.
    // When an Open Container Initiative (OCI) artifact is requested, artifacts found at this location will be used instead of
    // retrieving from the registry.
    //
    // This field is required and must:
    //   - Have length between 1 and 256 characters
    //   - Start with '/' (absolute path)
    //   - Contain only: a-z, A-Z, 0-9, '/', '.', '_', '-' (no spaces or special characters)
    //
    // +required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=256
    // +kubebuilder:validation:XValidation:rule="self.matches('^/[a-zA-Z0-9/._-]+$')",message="path must be absolute and contain only alphanumeric characters, '/', '.', '_', and '-'"
    Path string `json:"path,omitempty"`
}
```

**API Behavior:**

- Optional fields; absence means feature not configured
- Order matters: stores checked sequentially

### Topology Considerations

#### Hypershift / Hosted Control Planes

Supported. All three features (additional layer stores, image stores, and artifact stores) are configured at the node level and affect the data plane only. ContainerRuntimeConfig resources are embedded in ConfigMaps via the NodePool `.config` API and baked into ignition payloads by the Ignition Server. No HostedCluster API changes are required as ContainerRuntimeConfig is already supported through the existing NodePool configuration mechanism.

#### Standalone Clusters

Fully supported. This is the primary target deployment model for all three features. Cluster administrators use ContainerRuntimeConfig to configure additional storage across selected machine pools.

#### Single-node Deployments or MicroShift

- **Single-node OpenShift (SNO)**: Supported. All three features work on SNO deployments, though the BYOS approach for layer stores may be operationally complex on single-node systems with limited resources.
- **MicroShift**: Not supported. MicroShift does not use the Machine Config Operator, so the API is unavailable. These features are deferred for MicroShift environments.

#### OpenShift Kubernetes Engine

Fully supported. OKE clusters can use all three additional storage features through the standard ContainerRuntimeConfig API.

### Implementation Details/Notes/Constraints

#### MCO Implementation

The Machine Config Operator will:

1. **Watch `ContainerRuntimeConfig` resources** for changes to `additionalLayerStores`, `additionalImageStores`, and `additionalArtifactStores`
2. **Generate configuration files**:
   - For `additionalLayerStores`: Update `/etc/containers/storage.conf` with `[storage.options.additionallayerstores]` section
   - For `additionalImageStores`: Update `/etc/containers/storage.conf` with `additionalimagestores` array in `[storage.options]` section
   - For `additionalArtifactStores`: Update CRI-O configuration with `additional_artifact_stores` array
3. **Create MachineConfig**: Bundle generated configuration into a MachineConfig
4. **Apply to nodes**: MCO applies configuration to nodes matching the `machineConfigPoolSelector`
5. **Trigger reboot**: Nodes reboot to apply new storage/runtime configuration

**Generated configuration examples:**

storage.conf (additionalLayerStores):

```toml
[storage.options.additionallayerstores]
"/var/lib/stargz-store"
```

storage.conf (additionalImageStores):

```toml
[storage.options]
additionalimagestores = ["/mnt/nfs-images", "/mnt/ssd-images"]
```

crio.conf (additionalArtifactStores):

```toml
[crio.runtime]
additional_artifact_stores = ["/mnt/ssd-artifacts"]
```

#### Storage Plugin Architecture (BYOS Approach)

**How Plugins Work:**

Plugins expose a FUSE (Filesystem in Userspace) filesystem that `container-libs/storage` accesses during image pulls. When `container-libs/storage` needs a layer, it accesses paths like `<root>/store/<image-ref>/<layer-digest>/diff`. The FUSE filesystem intercepts these operations, allowing the plugin to:

1. Parse the image reference and layer digest from the filesystem path
2. Download only required metadata (manifest, table of contents) from the registry
3. Mount the layer content via FUSE, fetching chunks on-demand as files are accessed
4. Expose the layer through standard files: `diff/` (layer content), `info` (metadata), `blob` (blob data)

This pull-based design means plugins react to filesystem access patterns rather than needing push notifications from CRI-O.

**Supported Image Formats and Technologies:**

| Technology               | Type              | Support Level | Image Format Required | Notes                                                                                                                                                                                                    |
| ------------------------ | ----------------- | ------------- | --------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **stargz-snapshotter**   | Plugin (BYOS)     | Tech Preview  | eStargz, zstd:chunked | Requires image conversion; supports HTTP range requests; zstd:chunked support since v0.8.0 (2021)                                                                                                        |
| **nydus-storage-plugin** | Plugin (BYOS)     | Tech Preview  | Nydus                 | Requires image conversion; supports both eStargz and Nydus formats                                                                                                                                       |
| **zstd:chunked**         | Native (built-in) | GA            | zstd:chunked          | Built into container-libs/storage; partial pulling (not lazy); enabled by default. If a FUSE-based plugin handles zstd:chunked layers, lazy pulling is used; otherwise, standard partial pulling applies |
| **composefs**            | Native (built-in) | Tech Preview  | zstd:chunked required | Built into container-libs/storage; layer deduplication and fsverity support; no external plugin required                                                                                                 |

**Plugin Examples:**

- **stargz-store** (from containerd/stargz-snapshotter): FUSE-based daemon for eStargz images
- **nydus-store** (from containers/nydus-storage-plugin): FUSE-based daemon for Nydus and eStargz images

**Native vs Plugin Technologies:**

- **Native** (zstd:chunked, composefs): Built into container-libs/storage, no external plugin installation required. These technologies work with standard container-libs/storage configuration. If a FUSE-based plugin is configured to handle these formats, the plugin takes precedence; otherwise, the default container-libs/storage behavior applies (partial pulling for zstd:chunked, layer deduplication for composefs).
- **Plugin** (stargz-store, nydus-store): External FUSE-based storage plugins that require BYOS approach, customer-managed installation and lifecycle. Plugins intercept layer access and provide lazy pulling capabilities.

**Customer Responsibilities (for plugins only):**

- Install/run plugin daemon (DaemonSet, systemd service, or layered image)
- Ensure daemon health and manage lifecycle
- Convert images to required format (eStargz, Nydus)

**Support Boundaries:**

- OpenShift: API, MCO config generation, documentation/examples for both native and plugin approaches
- Customer: Plugin installation, daemon management, troubleshooting (plugins only)

### Risks and Mitigations

| Risk                                            | Impact                                        | Likelihood | Mitigation                                                                                                                                         |
| ----------------------------------------------- | --------------------------------------------- | ---------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| Additional Layer Store API is experimental      | Breaking changes possible before GA           | Medium     | Ship as Tech Preview; work with upstream to stabilize API before GA; document that API changes may require user action                             |
| Customer burden with BYOS (plugin installation) | Support complexity, adoption barrier          | High       | Provide detailed documentation and validated installation guides; consider community container images for common plugins; clear support boundaries |
| CRI-O v1.36 delayed                             | Feature delivery delay for artifact stores    | Low        | Maintain upstream communication; PR #9702 already in progress; consider backport to v1.35 if needed                                                |
| Quay compatibility unclear for lazy pulling     | Feature may not work with all registries      | Medium     | Test with different Quay storage backends (S3, local); document limitations; consider registry proxy for compatibility                             |
| Performance may not meet expectations           | User disappointment, low adoption             | Medium     | Validate early with realistic workloads; gather data before setting public targets; RHOAI team validation for artifact stores                      |
| Storage plugin crashes or hangs                 | Container creation failures, node instability | Low        | Document troubleshooting; provide health check recommendations; plugin failures should not crash CRI-O                                             |
| Upgrade path from Tech Preview to GA            | API changes may break existing configs        | Medium     | Version field extensions carefully; provide migration tooling if needed; clear communication                                                       |

### Drawbacks

1. **Experimental API dependency** (additionalLayerStores): Upstream container-libs/storage API may change, requiring user configuration updates
2. **BYOS complexity**: Customers must install and manage storage plugin binaries, increasing operational burden
3. **Registry requirements**: Lazy pulling requires HTTP range request support, limiting registry compatibility
4. **Image conversion**: eStargz requires converting images from standard OCI format, adding workflow complexity
5. **Support boundaries**: Division between OpenShift (API/config) and customer responsibility (plugins) may be unclear to users
6. **Node reboots required**: Configuration changes require node reboots, causing workload disruption

## Alternatives (Not Implemented)

**For Lazy Pulling:**

- **SOCI Integration**: containerd-specific, architecturally incompatible with CRI-O
- **zstd:chunked as alternative**: Partial pulling (not lazy); already enabled by default; complementary rather than alternative
- **Nydus Storage Plugin**: CRI-O plugin unmaintained since August 2022
- **Ship Plugin Binaries**: Increases maintenance burden, unclear which plugins to support
- **Registry Proxy**: Additional complexity, single point of failure

**For Artifact Stores:**

- **Hardcoded Paths**: Not user-configurable, requires code changes
- **Symlinks**: Fragile, no filtering support, manual management
- **Volume Mounts**: Doesn't integrate with CRI-O artifact resolution
- **Image Puller DaemonSet**: No CRI-O integration, manual cache management

## Open Questions

1. **Community container images for plugins?** Defer to post-Tech Preview based on adoption

## Test Plan

**Layer Stores:** E2E (plugin + eStargz pulling), performance (>5GB images), registries (Docker Hub, Quay, GHCR, ECR), negative cases (missing plugin, unsupported registry)

**Image Stores:** E2E (pre-populated image cache verification), performance (NFS/SSD comparison), shared storage (multi-node), negative cases (missing paths, permission issues)

**Artifact Stores:** E2E (cache verification), performance (RHOAI SSD validation), edge (air-gapped), negative cases (missing paths)

**Common:** MCO config generation, upgrade/downgrade (4.21 ↔ 4.22), feature gate enforcement, standard behavior regression testing

## Graduation Criteria

### Dev Preview -> Tech Preview

Not applicable. These features will ship directly to Tech Preview in 4.22, skipping Dev Preview.

### Tech Preview -> GA

**Prerequisites:**

- [ ] CRI-O v1.36+ with artifact store support merged and shipped
- [ ] Field feedback from customer deployments running Tech Preview
- [ ] Performance metrics collected and validated against target workloads
- [ ] Security review completed (chunk verification for lazy pulling, plugin isolation)
- [ ] Comprehensive test coverage (multiple registries, performance benchmarks, upgrade/downgrade)
- [ ] RHOAI validation completed with published performance data
- [ ] Migration path defined for any API changes from Tech Preview

**GA Criteria (Target: 4.23+):**

- [ ] Features available by default (promote AdditionalStorageConfig feature gate to Default feature set)
- [ ] API declared stable with backward compatibility guarantees
- [ ] API changes from Tech Preview include automated migration or clear upgrade documentation
- [ ] Comprehensive user-facing documentation and support team training completed
- [ ] Production support processes documented and validated
- [ ] No P0/P1 bugs outstanding

### Tech Preview (4.22)

- [x] Enhancement proposal approved
- [ ] All three features behind AdditionalStorageConfig feature gate (enabled in TechPreviewNoUpgrade/DevPreviewNoUpgrade)
- [ ] API design reviewed and approved by API review team
- [ ] MCO implementation merged and functional
- [ ] E2E tests passing in CI for all three features
- [ ] Documentation in openshift-docs (OSDOCS-10167, OSDOCS-17312)
- [ ] Limitations, BYOS approach, and support boundaries clearly documented
- [ ] Known issues and workarounds documented

### Removing a deprecated feature

Not applicable - these are new features with no deprecation planned.

## Upgrade / Downgrade Strategy

**Upgrade (4.21 → 4.22):** New optional fields ignored by older MCO; opt-in via AdditionalStorageConfig feature gate (enabled in TechPreviewNoUpgrade/DevPreviewNoUpgrade); no impact to existing resources

**Downgrade (4.22 → 4.21):** Delete ContainerRuntimeConfig resources with new fields; nodes reboot to remove config

## Version Skew Strategy

This is a node-level feature with no cross-component dependencies beyond the Machine Config Operator and CRI-O, which are tightly coupled to the node version.

**Control Plane vs Node Skew:**

- Control plane does not interact with these features directly
- API server only stores ContainerRuntimeConfig custom resources
- No version skew concerns between control plane and nodes

**Node Version Skew:**

- MCO on older nodes (4.21) ignores unknown fields in ContainerRuntimeConfig (additionalLayerStores, additionalArtifactStores)
- Nodes running 4.22 MCO can coexist with nodes running 4.21 MCO
- Feature is opt-in via ContainerRuntimeConfig targeting specific machine pools
- Mixed-version node pools work correctly (4.22 nodes use new features, 4.21 nodes ignore them)

**Component Compatibility:**

- CRI-O and container-libs/storage are updated atomically with the node
- No n-2 skew concerns (kubelet is n-2 compatible but does not interact with these features)
- Storage plugin lifecycle is customer-managed (BYOS) and independent of OpenShift versions

## Operational Aspects of API Extensions

**API Impact:**

- Two new optional fields in ContainerRuntimeConfig (backward compatible)
- Uses existing MCO conditions and metrics
- Minimal throughput impact (low-frequency resources)

### Failure Modes

| Failure                     | Detection                             | Impact                        | Escalation         |
| --------------------------- | ------------------------------------- | ----------------------------- | ------------------ |
| Invalid path                | API validation                        | Config rejected               | Node team          |
| MCO config generation fails | MCO Degraded                          | Previous config continues     | MCO/Node team      |
| Node reboot fails           | MCO Degraded, node SchedulingDisabled | Reduced capacity              | MCO/Node team      |
| Plugin not installed        | CRI-O logs                            | Falls back to standard pull   | Customer (BYOS)    |
| Plugin crashes              | CRI-O logs, plugin exit               | May impact container creation | Customer/Node team |
| Store path missing          | CRI-O warning                         | Falls back to default         | Storage/Node team  |

## Support Procedures

**Troubleshooting:** `oc get containerruntimeconfig -o yaml`, `oc get mco -o yaml | grep conditions`, `oc debug node/<node> -- cat /etc/containers/storage.conf`, `oc adm node-logs <node> -u crio`

**Common Issues:** Layer stores (plugin missing, no range requests, wrong format); artifact stores (path missing, artifacts not pre-populated)

**Disabling:** Delete ContainerRuntimeConfig or remove fields; MCO reverts on reboot

**Recovery:** Graceful fallback to standard behavior; no reconciliation needed

## Infrastructure Needed

**CI Infrastructure:**

- Plugin binaries (stargz-store) and DaemonSet manifests
- Test registries (with/without HTTP range support, private)
- Simulated SSD storage and pre-populated artifacts
- Performance metrics collection and large test images (>5GB)

**Collaboration:**

- RHOAI: Performance validation and success metrics
- Upstream: container-libs/storage API coordination, CRI-O v1.36 planning, stargz-snapshotter compatibility
- QE/Docs: Test and documentation reviews

---

## Appendices

### References

#### Upstream Projects

- CRI-O Additional Layer Store API: https://github.com/containers/storage
- CRI-O PR #9702 (artifact stores): https://github.com/cri-o/cri-o/pull/9702
- CRI-O Issue #9570: https://github.com/cri-o/cri-o/issues/9570
- stargz-snapshotter (stargz-store plugin): https://github.com/containerd/stargz-snapshotter
- Nydus storage plugin: https://github.com/containers/nydus-storage-plugin
- Nydus image service: https://nydus.dev/

#### Previous Work

- OCPNODE-2204: Previous lazy pull attempt (abandoned)
  - Enhancement: https://github.com/openshift/enhancements/pull/1600
  - MCO PR: https://github.com/openshift/machine-config-operator/pull/4248
- Related enhancements:
  - On-cluster layering: /enhancements/machine-config/on-cluster-layering.md
  - Pin and pre-load images: /enhancements/machine-config/pin-and-pre-load-images.md

#### Related Issues

- [OCPSTRAT-1285](https://issues.redhat.com/browse/OCPSTRAT-1285): Speeding Up Pulling Container Images
- [OCPSTRAT-2623](https://issues.redhat.com/browse/OCPSTRAT-2623): Additional Artifact Store
- [OCPNODE-4050](https://issues.redhat.com/browse/OCPNODE-4050): Additional Layer Store Support (Epic)
- [OCPNODE-4051](https://issues.redhat.com/browse/OCPNODE-4051): Additional Artifact Store Support (Epic)
- [OCPNODE-4052](https://issues.redhat.com/browse/OCPNODE-4052): Enhancement Proposal Story
- [OSDOCS-10167](https://issues.redhat.com/browse/OSDOCS-10167): Documentation for layer stores
- [OSDOCS-17312](https://issues.redhat.com/browse/OSDOCS-17312): Documentation for artifact stores
- [RFE-8441](https://issues.redhat.com/browse/RFE-8441): Artifact pre-loading (separate feature, out of scope)
- [RHEL-66490](https://issues.redhat.com/browse/RHEL-66490): zstd:chunked image ID inconsistency (related bug)

#### External Resources

- AWS Fargate SOCI: https://aws.amazon.com/blogs/containers/under-the-hood-lazy-loading-container-images-with-seekable-oci-and-aws-fargate/
- Kubernetes cgroup v2 KEP (example of Tech Preview on experimental API): https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/2254-cgroup-v2
