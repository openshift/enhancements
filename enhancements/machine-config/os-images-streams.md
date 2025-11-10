---
title: machine-config-os-images-streams
authors:
  - "@pablintino"
  - "@dkhater-redhat"
reviewers:
  - "@yuqi-zhang"
  - "@pablintino"
approvers:
  - "@yuqi-zhang"
api-approvers:
  - "@JoelSpeed"
creation-date: 2025-10-24
tracking-link:
  - https://issues.redhat.com/browse/MCO-1914
see-also:
replaces:
superseded-by:
---

# MachineConfig OS Images Streams

## Summary

This enhancement allows administrators to easily assign different OS images
to specific groups of nodes using a simple "stream" identifier.

It introduces a new, optional stream field in the MCP. When this field is set,
the MCO will provision nodes in that pool using the specific OS image 
associated with that stream name.

This provides a simple, declarative way to run different OS variants within
the same cluster. This can be used to test new major OS versions 
(like RHEL 10) on a subset of nodes or to deploy specialized images,
without affecting the rest of the cluster.

## Motivation

OpenShift is transitioning from RHEL CoreOS 9 to RHEL CoreOS 10. Currently, all nodes
in a cluster share a single default OS image and there's no straightforward per-MCP
upgrade option. During the RHEL 8→9 transition, this made it difficult to validate
compatibility without disrupting production, and coupled platform and OS changes
together, increasing risk.

This enhancement enables administrators to:
- Upgrade OpenShift platform versions without immediately changing OS versions (deprecated streams will eventually require migration to continue platform upgrades)
- Test new OS versions on a subset of nodes before full migration
- Gradually migrate pools from RHEL 9 to RHEL 10

### Key Capabilities

- Set `spec.osImageStream` on a MachineConfigPool to use a different OS version for that pool
- Query the OSImageStream resource to discover available OS streams
- View `status.osImageStream` to monitor when a pool has adopted a new stream
- Existing clusters continue working without changes (opt-in via OSStreams feature gate)

### Goals

#### Phase 1: RHEL 9 to RHEL 10 Transition

- Support dual RHCOS 9 and RHCOS 10 streams (`rhel9-coreos` and `rhel10-coreos`)
- Enable per-pool stream selection and gradual migration
- One-directional migration only (RHEL 9→10, no downgrade support)
- Backward compatibility: pools without explicit stream selection default to `rhel9-coreos` and maintain current OS version during platform upgrades
- Day-zero RHEL 10 deployments (stretch goal)

#### Future Phases (Out of Scope for Initial Release)

- Additional stream variants (minimal OS images, hardened variants, etc.) - the architecture supports multiple streams, but only 2 will be shipped initially
- Bidirectional stream switching (RHEL 10→9 downgrade)
- HCP/Hypershift architecture support
- Image Mode architecture support 

### Non-Goals

- Automated migration orchestration

## Proposal

### Scope and Phasing

The architecture supports multiple OS streams (up to 100 per the API limit), but the initial
release ships with exactly two streams available: `rhel9-coreos` and `rhel10-coreos`.
Success criteria are measured solely on enabling the RHEL 9→10 transition.

Implementation requires changes in the MCO, release payload, and CoreOS images, described in
the following sections.

### Machine Config Pools

To provide the user with the ability to set which stream an MCP's nodes
should use, the MCP CRD must be modified to introduce a new field:

- `spec.osImageStream`: To set the target stream the pool should use. When
omitted, the pool uses the default stream (see [Default Stream Evolution and Upgrade Behavior](#default-stream-evolution-and-upgrade-behavior)).
Note: When this feature GAs, the concept of "cluster-wide OS images" will be replaced
by the default stream mechanism. Users will be required to explicitly select a stream
only when their install-time default stream is deprecated (e.g., when `rhel9-coreos`
is phased out).
- `status.osImageStream`: To inform the user of the stream currently used
by the pool. This field will reflect the target stream once the pool has
finished updating to it.

The [API Extensions](#api-extensions) section describes these API changes 
in greater detail.

From the perspective of the MCP reconciliation logic, the addition of 
streams is not different from an override of both OS images in the 
MachineConfig of the associated pool. If a user sets a stream in the 
pool, the MCO takes care of picking the proper URLs to use from the 
new, internally populated, OSImageStream resource and injecting them 
as part of the MCP's MachineConfig. This internal change of the URLs 
will force the MCP to update and deploy the image on each node one by 
one.

### CoreOS and Payload Images

The scope of this enhancement is to allow the user to consume streams shipped 
as part of the payload. Therefore, all information about which streams are 
available should be contained in the payload image and the tagged OS images.

To accommodate more than one OS version and the associated stream name, the 
release build process has been updated with the following changes:

The Payload ImageStream now contains extra coreos tags for both OS and 
Extension Images to accommodate more OS versions.

Each OS image will be built with labels that allow the MCO to identify the stream to
which it belongs. The agreed-upon label is `io.openshift.os.streamclass`, which will contain
the stream name for both regular OS images and extension images.

-------UPDATE ME WHEN CONSENSUS IS GIVEN-------

**Note**: The exact stream naming convention is not yet finalized and is pending Product Owner
input. Examples used throughout this document (`rhel9-coreos`, `rhel10-coreos`) are placeholders,
and the "coreos" suffix may be removed in the final naming scheme.

With these labels in place, the MCO has enough information to build the list of available
streams and determine which images should be used for each stream.

### Default Stream Determination
-------UPDATE ME WHEN CONSENSUS IS GIVEN-------

The mechanism for determining which stream should be used as the default (when no explicit
`spec.osImageStream` is set on a MachineConfigPool) is still under discussion. Potential
approaches being considered include:

- Installer-fed: The installer could inject the default stream name during cluster installation
- Hardcoded in ConfigMap: The `machine-config-osimageurl` ConfigMap could specify the default stream

**Current Status**: This design detail is under active discussion and will be finalized
before the initial release.

#### Stream Merging Behavior

When stream information is available from multiple sources (e.g., ConfigMap and Release ImageStream),
the MCO merges them with the precedence order defined in [Stream Sources and Precedence](#stream-sources-and-precedence)
(Release ImageStream takes precedence over ConfigMap). The merging process:

1. **Collection**: Each source is queried for its available streams
2. **Merging**: Streams are merged by stream name. When multiple sources provide the same stream
   name, the higher-precedence source wins
3. **Default Stream Identification**: The MCO identifies the default stream using a version-specific
   name based on the distribution and release version:
   - RHCOS (RHEL 9 default releases): `"rhel9-coreos"`
   - RHCOS (RHEL 10 default releases): `"rhel10-coreos"`
   - FCOS: `"fedora-coreos"` (unversioned for Fedora's rolling release model)
   - SCOS: `"stream-coreos"`
4. **Validation**: If the default stream is not found in the merged streams, the cluster reports
   an error

**Important**: The default stream name is **version-specific and hardcoded** based on the MCO's
build target and release version. This ensures explicit version selection and prevents silent OS
version changes during platform upgrades.

The merging behavior ensures:
- Best-effort processing: if a source fails to provide streams, it's skipped with logging
- Conflict logging: when streams are overridden, the conflict is logged for debugging
- Handling of partial stream data (OS image without Extensions or vice versa)

#### Default Stream Evolution and Upgrade Behavior

The default stream name changes between OpenShift releases to reflect the recommended
OS version for new clusters, while ensuring existing clusters maintain their current OS
version during platform upgrades.

**Initial Releases (RHCOS 9 default):**
- Default stream: `"rhel9-coreos"`
- Available streams: `"rhel9-coreos"`, `"rhel10-coreos"` (Tech Preview)
- New clusters install with RHCOS 9
- Existing clusters remain on their current stream

**Later Releases (RHCOS 10 default):**
- Default stream: `"rhel10-coreos"`
- Available streams: `"rhel9-coreos"`, `"rhel10-coreos"`
- New clusters install with RHCOS 10
- Existing clusters upgrading from earlier releases remain on their current stream (typically `"rhel9-coreos"`)

**Upgrade Behavior:**

When upgrading across the default stream transition (RHCOS 9 → RHCOS 10):

1. **MachineConfigPools with explicit `spec.osImageStream` set**: Continue using the
specified stream unchanged. The stream reference is preserved across upgrades.

2. **MachineConfigPools without `spec.osImageStream` set** (using default):
   - The pool continues using `"rhel9-coreos"` even though the new default is `"rhel10-coreos"`
   - The MCO tracks which stream was being used before the upgrade and maintains it
   - This prevents **silent OS version changes** during platform upgrades

3. **New MachineConfigPools created after the default transition**: Use the new default `"rhel10-coreos"`

**Migration Process:**

To migrate a pool from RHCOS 9 to RHCOS 10, administrators must explicitly:
```yaml
spec:
  osImageStream:
    name: rhel10-coreos
```

This ensures all OS version migrations are **explicit administrator decisions**, not
automatic side effects of platform upgrades. This aligns with the core goal of
separating platform upgrades from OS version transitions.

#### Backward Compatibility with Pre-Streams Clusters

Clusters existing before the streams feature (pre-4.21) do not have `spec.osImageStream`
set on MachineConfigPools. When these clusters upgrade to a streams-enabled OpenShift
version (4.21+ with OSStreams feature gate):

- **Pools without `spec.osImageStream` maintain their current OS version**: The MCO
  internally tracks the current stream (e.g., `rhel9-coreos`) and persists this mapping
  to prevent silent OS changes during platform upgrades
- **Mapping is permanent unless explicitly changed**: Pools continue using their tracked
  stream even when upgrading to releases where the default changes to `rhel10-coreos`
- **Administrators explicitly migrate when ready**: To change OS versions, administrators
  set `spec.osImageStream.name: rhel10-coreos`

This ensures existing clusters don't accidentally change OS versions during platform
upgrades, and all OS migrations are explicit administrator decisions.

### Machine Config OSImageStream

This new resource holds the URLs associated with each stream and is populated
by the MCO using the information from the OS image labels. The logic that 
extracts the URLs and stream names from the OS images differs depending on 
whether the cluster is bootstrapping or undergoing an update. During regular 
operation (i.e., when not bootstrapping or updating), the MCO does not make 
any changes to this resource, and its information can be safely considered 
static.

#### Streams

### API Extensions

This enhancement introduces two API changes: modifications to the existing
MachineConfigPool API and a new OSImageStream cluster-scoped resource.

The `spec.osImageStream` field on MachineConfigPool will be validated using a Validating
Admission Policy (VAP) that checks the referenced stream exists in the OSImageStream
singleton resource. This ensures pools can only reference available streams.

#### MachineConfigPool API Changes

The MachineConfigPool API is extended with new fields to reference OS streams:

```go
type MachineConfigPoolSpec struct {
    // Existing fields omitted for brevity...

    // osImageStream specifies an OS stream to be used for the pool.
    //
    // When set, the referenced stream overrides the cluster-wide OS
    // images for the pool with the OS and Extensions associated to stream.
    // When omitted, the pool uses the cluster-wide default OS images.
    //
    // +openshift:enable:FeatureGate=OSStreams
    // +optional
    OSImageStream OSImageStreamReference `json:"osImageStream,omitempty,omitzero"`
}

type MachineConfigPoolStatus struct {
    // Existing fields omitted for brevity...

    // osImageStream specifies the last updated OSImageStream stream for the pool.
    //
    // When omitted, the pool is using the cluster-wide default OS images.
    // +openshift:enable:FeatureGate=OSStreams
    // +optional
    OSImageStream OSImageStreamReference `json:"osImageStream,omitempty,omitzero"`
}

// OSImageStreamReference references an OS stream.
type OSImageStreamReference struct {
    // name is a reference to an OSImageStream stream to be used for the pool.
    //
    // It must be a lowercase RFC 1123 subdomain, consisting of lowercase
    // alphanumeric characters, hyphens ('-'), and periods ('.').
    //
    // +required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=253
    // +kubebuilder:validation:XValidation:rule="!format.dns1123Subdomain().validate(self).hasValue()",message="a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character."
    Name string `json:"name,omitempty"`
}
```

#### OSImageStream Resource

A new cluster-scoped resource that holds available OS streams and their image URLs:

```go
// OSImageStream is a cluster-scoped resource that holds OS image stream information
// populated by the Machine Config Operator.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Cluster
type OSImageStream struct {
    metav1.TypeMeta   `json:",inline"`
    metav1.ObjectMeta `json:"metadata,omitempty"`

    Spec   OSImageStreamSpec   `json:"spec,omitempty"`
    Status OSImageStreamStatus `json:"status"`
}

type OSImageStreamSpec struct {
    // Reserved for future use
}

type OSImageStreamStatus struct {
    // availableStreams is a list of the available OS Image Streams
    // available and their associated URLs for both OS and Extensions
    // images.
    //
    // It must have at least one item and may not exceed 100 items.
    // +optional
    // +kubebuilder:validation:MinItems=1
    // +kubebuilder:validation:MaxItems=100
    // +listType=map
    // +listMapKey=name
    AvailableStreams []OSImageStreamURLSet `json:"availableStreams,omitempty"`

    // defaultStream is the name of the stream that should be used as the default
    // when no specific stream is requested by a MachineConfigPool.
    // Must reference the name of one of the streams in availableStreams.
    // Must be set when availableStreams is not empty.
    //
    // +optional
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=70
    DefaultStream string `json:"defaultStream,omitempty"`
}

type OSImageStreamURLSet struct {
    // name is the identifier of the stream (e.g., "rhel9-coreos", "rhel10-coreos").
    //
    // Must not be empty and must not exceed 70 characters in length.
    // Must only contain alphanumeric characters, hyphens ('-'), or dots ('.').
    // +required
    // +kubebuilder:validation:MinLength=1
    // +kubebuilder:validation:MaxLength=70
    Name string `json:"name,omitempty"`

    // osImage is an OS Image referenced by digest.
    //
    // The format of the image pull spec is: host[:port][/namespace]/name@sha256:<digest>,
    // where the digest must be 64 characters long, and consist only of lowercase
    // hexadecimal characters, a-f and 0-9.
    // +required
    OSImage string `json:"osImage,omitempty"`

    // osExtensionsImage is an OS Extensions Image referenced by digest.
    //
    // The format of the image pull spec is: host[:port][/namespace]/name@sha256:<digest>,
    // where the digest must be 64 characters long, and consist only of lowercase
    // hexadecimal characters, a-f and 0-9.
    // +required
    OSExtensionsImage string `json:"osExtensionsImage,omitempty"`
}
```

#### Feature Gate

The `OSStreams` feature gate controls the availability of this functionality:

- **Feature set**: TechPreviewNoUpgrade
- **Default**: Disabled
- **Scope**: Cluster-wide

When the feature gate is disabled, the OSImageStream resource is not available,
and the `osImageStream` fields in MachineConfigPool are ignored.

### Topology Considerations

#### Hypershift

This enhancement does **not** apply to HCP/Hypershift architectures. Hypershift
uses NodePool objects instead of MachineConfigPools and has different
architectural requirements. Support for OS streams in Hypershift environments is
deferred to future work and will require a separate design.

#### Standalone OpenShift

This enhancement supports standalone OpenShift clusters using MachineConfigPools,
including Single-Node OpenShift (SNO).

### Implementation Details/Notes/Constraints

#### Stream Sources and Precedence

The MCO obtains OS stream information from multiple sources, with a defined precedence order:

**During Bootstrap:**
- CLI Arguments (optional, for development/testing only)
- Release ImageStream (from the release payload)

**During Runtime:**
- ConfigMap (`machine-config-osimageurl`): Provides backward compatibility for existing configurations
- Release ImageStream: Higher precedence, overrides ConfigMap values

When multiple sources provide the same stream name, later sources in the precedence order override earlier ones. This design provides backward compatibility with existing ConfigMap-based configurations while allowing the release payload to be the authoritative source of stream information.

##### Release ImageStream

The primary source of stream information in production environments. The MCO
extracts stream data from `/release-manifests/image-references` in both bootstrap
and runtime scenarios, using different mechanisms for each.

**Extraction Mechanisms:**

**Bootstrap**: The bootstrap process reads `/release-manifests/image-references` from the file
system via the existing `--image-references` flag and parses it into an ImageStream object.

**Runtime** (new): Extracts `/release-manifests/image-references` directly from the release
image layers using image inspection utilities.

**Stream Discovery Process:**

1. **ImageStream Parsing**: The extracted `image-references` file contains an ImageStream
with tags for OS and extension images.

2. **Image Filtering**: The MCO does not inspect all images in the ImageStream. It only examines
images that are likely to be OS images, filtering based on:
   - Images with the annotation `io.openshift.build.source-location` set to `github.com/openshift/os`, OR
   - Tags matching the regex `^(rhel[\w.+-]*|stream)-coreos[\w.+-]*(-extensions[\w.+-]*)?$`

   Note: This regex will likely change once the first images with the `io.openshift.os.streamclass`
   label are available, and may drop the "coreos" reference requirement.

3. **Image Label Inspection**: For filtered images, the MCO reads the `io.openshift.os.streamclass`
label that identifies stream association. This label contains the stream name (e.g., "rhel9-coreos",
"rhel10-coreos") and is present on both OS images and extension images.

   **Current Status**: The MCO code is ready to parse these labels, but the CoreOS team
   has not yet added them to the images. This work is in progress.

4. **Stream Grouping**: OS/Extensions image pairs are grouped by stream name based on labels.

**Example ImageStream structure:**
```yaml
kind: ImageStream
spec:
  tags:
  - name: rhel9-coreos
    from:
      kind: DockerImage
      name: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
  - name: rhel9-coreos-extensions
    from:
      kind: DockerImage
      name: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
  - name: rhel10-coreos
    from:
      kind: DockerImage
      name: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
  - name: rhel10-coreos-extensions
    from:
      kind: DockerImage
      name: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
```

##### ConfigMap (machine-config-osimageurl)

The existing `machine-config-osimageurl` ConfigMap format is preserved and continues to work
unchanged. The ConfigMap will **not** be extended with a `streams.json` field or multi-stream
format. Instead, stream information is extracted from the `io.openshift.os.streamclass` labels
on the OS images referenced in the ConfigMap.

**ConfigMap Format:**
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: machine-config-osimageurl
  namespace: openshift-machine-config-operator
data:
  baseOSContainerImage: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
  baseOSExtensionsContainerImage: quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...
  releaseVersion: "4.21.0"
```

The MCO inspects the `baseOSContainerImage` and `baseOSExtensionsContainerImage` to read the
`io.openshift.os.streamclass` label, which identifies which stream these images belong to.
This allows the ConfigMap to work with the streams feature without format changes.

**Future Direction:** The ConfigMap will eventually not be needed since the Release ImageStream
(managed by CVO) already contains the stream information. The installer populates the ConfigMap
placeholders based on that ImageStream. The ConfigMap primarily provides backward compatibility
during the transition period.


#### OSImageStream Resource Population

The OSImageStream cluster resource is populated by the MCO during cluster bootstrap and
runtime updates. The MCO reads stream information from the release ImageStream and ConfigMap
(with ImageStream taking precedence), then creates and maintains the OSImageStream resource
to expose available streams to administrators.

**Stream Discovery Mechanism:**

The MCO discovers available streams by inspecting OS container images in the release payload.
Each RHEL CoreOS image will include labels that identify its stream. These labels will contain 
stream names using major-version granularity (`rhel9-coreos`, `rhel10-coreos`) designed for 
administrator use, abstracting over CoreOS's internal minor-version streams.

#### MachineConfigPool Reconciliation

When a user sets `spec.osImageStream` on a MachineConfigPool, the MCO validates the
stream exists, looks up the corresponding OS images from the OSImageStream resource,
and generates a new rendered MachineConfig for the pool. The standard MCP update
mechanism then rolls out the new OS images to nodes one by one. Stream changes are
treated identically to other MachineConfig changes, leveraging existing update mechanisms.

#### Release Payload Integration

The OS stream feature requires coordination with the OpenShift release build
process:

1. **MCO Image**: The MCO container image includes the `machine-config-osimageurl`
ConfigMap manifest with placeholder values.

2. **Image References**: The MCO image also includes an `image-references` file
that the `oc` CLI uses during release image creation.

3. **Placeholder Replacement**: When building the release image, the installer reads the
`image-references` file and replaces placeholders in the ConfigMap manifest with
actual image URLs from the Release ImageStream tags.

4. **CVO Application**: The Cluster Version Operator (CVO) applies the
pre-populated ConfigMap when deploying the MCO.

**Important**: The ConfigMap structure remains unchanged from its current format and does
not directly contain stream information. Stream information is obtained by inspecting the
`io.openshift.os.streamclass` labels on the OS images referenced in the ConfigMap. The primary
source of stream information is the Release ImageStream, which the MCO inspects directly. The
ConfigMap primarily provides backward compatibility during the transition period.

#### RHEL 9-Specific Logic Compatibility

The Machine Config Daemon (MCD) contains conditional logic specific to RHEL 9/8.
As part of this enhancement, this logic must be reviewed to ensure compatibility
with RHEL 10/CentOS 10:

- Audit existing RHEL version conditionals in the MCD codebase
- Determine if conditionals should apply to RHEL 10, require modification, or
remain RHEL 9/8-exclusive
- Update conditionals as needed to support multi-stream deployments
- Validate that Fedora-specific conditionals are still required for OKD (which
now uses CentOS Stream)

This work is tracked separately and is a prerequisite for full RHEL 10 support.

### Implementation Overview

This enhancement is being delivered incrementally across multiple OpenShift releases:

- **OpenShift 4.21**: Dev Preview with OSStreams feature gate, RHEL 9 and RHEL 10 stream support
- **OpenShift 4.22**: Tech Preview with improved stability, version skew enforcement, and expanded testing
- **Later releases**: Continued RHEL 9/10 dual-stream support, with RHEL 10 becoming the default for new clusters

### Risks and Mitigations

**Risk**: Administrators accidentally set wrong stream, causing nodes to provision with incompatible OS version.
**Mitigation**: Validation ensures referenced stream exists in OSImageStream resource before allowing MCP update.

**Risk**: RHEL 10-specific bugs not discovered until production deployment.
**Mitigation**: Separate RHEL 10 test payloads in 4.20/4.21, tech preview jobs in 4.22/4.23, comprehensive OS variant testing (FIPS, real-time kernel).

**Risk**: Complexity of supporting multiple OS versions simultaneously increases operational burden.
**Mitigation**: Limit to 2-3 concurrent streams, provide clear deprecation timeline with upgrade blockers.

### Drawbacks

The feature increases system complexity by allowing different OS versions across pools, which may complicate troubleshooting and support. However, this is acceptable given the benefit of de-risking major OS version transitions.

## Design Details

### Open Questions [optional]

None.

## Test Plan

### Testing Strategy

The OS streams feature requires comprehensive testing to ensure safe RHEL 9 to RHEL 10
transitions and validate component readiness on both OS versions.

### Test Infrastructure

**RHEL 10 Payload Generation:**

Separate RHEL 10 release streams will be produced to enable early testing:
- Custom RHEL 10-based payloads from nightly builds (similar to F-COS/S-COS for OKD)
- Enables testing RHEL 10 before it becomes the default for new clusters
- Timeline approach:
  - Initial releases: RHEL 10 payloads available as separate stream
  - Tech preview releases: Tech preview jobs begin running on RHEL 10 for component readiness
  - Later releases: RHEL 10 becomes default for new clusters

### Testing Areas

**Stream Switching and Transitions:**
- RHEL 9 → RHEL 10 migration (primary MCO focus, one-directional only)
- Stream selection and discovery from multiple sources
- Partial migrations (some pools on RHEL 9, others on RHEL 10)
- Failed update scenarios (manual recovery procedures)

**Backward Compatibility:**
- Upgrade pre-streams clusters and verify implicit stream mapping
- Pools without explicit stream maintain current OS version
- Legacy ConfigMap format compatibility

**Platform Upgrades:**
- OpenShift version upgrades without OS version changes
- Verify existing pools maintain their stream during platform upgrades
- New pools created post-upgrade use appropriate defaults

**OS Variants:**
- FIPS-enabled clusters on RHEL 10
- Real-time kernel on RHEL 10
- Standard RHEL 10
- Note: RHEL 8 → 9 transition revealed distinct bugs with FIPS and real-time kernel
  variants not seen in standard testing

**Image Mode OpenShift:**
- OS image URL overrides in image mode scenarios
- Consistent behavior between traditional and image mode deployments
- Testing mechanism should be consistent across HCP and self-managed

**Topology Coverage:**
- Standalone clusters (multi-node)
- Single-Node OpenShift (SNO)
- Stream selection at install time and runtime

**Component Readiness:**
- OpenShift core components on RHEL 10 (etcd, kube-apiserver, etc.)
- Operators, CNI plugins, CSI drivers
- Platform integrations and cloud providers
- Goal: Identify RHEL 10-specific issues before RHEL 10 becomes the default for new clusters

## Graduation Criteria

This feature follows a phased rollout aligned with RHCOS dual-stream support
timelines. The feature is **not** intended to reach GA in OpenShift 4.x and is
planned as a temporary capability to support the RHCOS 9 → 10 transition.

### OpenShift 4.21: Dev Preview

**Target Release**: OpenShift 4.21 (RHCOS 9.6 default, RHCOS 10.1 in payload)

**Deliverables:**
- OSStreams feature gate, `osImageStream` field in MachineConfigPool, OSImageStream v1alpha1 resource
- MCO parses stream-based ConfigMap format and extracts streams from release ImageStream
- MachineConfigPool reconciliation, bootstrap, and runtime stream population logic

**Status**: TechPreviewNoUpgrade feature set, v1alpha1 API, Dev Preview support level

**Fallback**: Off-cluster image mode if API not ready

### OpenShift 4.22: Tech Preview

**Target Release**: OpenShift 4.22 (RHCOS 9.8 default, RHCOS 10.2 in payload)

**Deliverables:**
- Version skew enforcement for MachineSets
- Improved error handling, validation, logging, and observability
- Bug fixes and stability improvements

**Status**: TechPreviewNoUpgrade feature set, v1alpha1 API, Tech Preview support level

### OpenShift 4.23: Tech Preview (possible removal of TP status)

**Target Release**: OpenShift 4.23 (RHCOS 9.10 default, RHCOS 10.2 in payload)

**Deliverables:**
- Evaluation of removing Tech Preview status
- Additional stability and performance improvements

**Status**: Potentially default-enabled feature set, v1alpha1 or v1beta1 API (TBD), Tech Preview or GA (TBD)

### Later Releases: Continued Support and GA Path

**When RHCOS 10 Becomes Default:**

**Changes:**

- RHCOS 10 uses bootc exclusively for image updates
- RHCOS 9 uses bootc when possible (fallback to rpm-ostree)
- Existing clusters remain on RHCOS 9 until administrators explicitly change streams
- Customers can separate platform upgrades from OS migration (9 → 10)

**Feature Evolution:**

- Feature likely becomes default-enabled
- Possible promotion to v1
- RHEL 9/10 dual-stream support continues for the lifecycle of these OS versions
- Architecture allows for future stream types (minimal, hardened, etc.) but these are not committed

### Dev Preview -> Tech Preview

**Criteria for 4.21 to 4.22:**
- [ ] Core functionality stable, no data loss/stability issues
- [ ] Basic e2e tests passing, documentation complete
- [ ] Upgrade path validated

### Tech Preview -> GA

**Prerequisites:**
- [ ] Multiple releases of production usage, comprehensive test coverage, performance validation
- [ ] API promoted to v1, complete documentation with runbooks
- [ ] Telemetry and observability instrumentation

**Nice-to-Have:**
- [ ] Hypershift/HCP support
- [ ] Additional OS stream variants (minimal, standard)

**GA Target**: Later OpenShift releases after validation in 4.21-4.23 releases

**Post-GA**: RHEL 9/10 dual-stream support continues for the lifecycle of these OS versions. The
architecture is designed to allow future stream types (minimal images, hardened images) if needed,
but these are not part of the current commitment.

### Removing deprecated OS streams

Specific OS streams will be deprecated as they reach end-of-life. The deprecation process
includes proactive upgrade blockers to ensure clusters migrate off deprecated streams before
they become unsupported.

#### RHEL 9 Stream Deprecation Timeline (Example)

While the stream selection capability remains supported, specific OS versions like RHEL 9
will eventually be deprecated. The expected timeline:

**Initial Dual-Stream Support Period:**
- Both `rhel9-coreos` and `rhel10-coreos` streams fully supported
- No warnings or blockers
- Clusters can run either stream

**Deprecation Warning Period (Release N):**
- `rhel9-coreos` stream marked as deprecated
- Warning conditions added to MachineConfigPools using `rhel9-coreos`:
  ```yaml
  status:
    conditions:
    - type: OSStreamDeprecated
      status: True
      reason: StreamEndOfLife
      message: "Stream 'rhel9-coreos' is deprecated and will be removed in a future release.
               Migrate to 'rhel10-coreos' before upgrading."
  ```
- Cluster-level upgrade remains possible (no blocker yet)
- Documentation and alerts guide administrators to plan migration

**Upgrade Blocker Period (Release N+1):**
- Upgrade blocker activated for clusters with pools still using `rhel9-coreos`
- ClusterVersion resource sets `Upgradeable: False`:
  ```yaml
  status:
    conditions:
    - type: Upgradeable
      status: False
      reason: DeprecatedOSStreamInUse
      message: "Cannot upgrade: MachineConfigPool 'worker' is using deprecated stream
               'rhel9-coreos' which is not supported in this release. Migrate pool to
               'rhel10-coreos' before upgrading."
  ```
- Cluster cannot upgrade until all pools migrate to `rhel10-coreos`
- This provides a **forcing function** to complete OS migration

**Stream Removal (Release N+2):**
- `rhel9-coreos` stream removed from release payload
- Existing pools using the removed stream cannot receive updates
- Validation prevents new pools from referencing removed streams

#### General Deprecation Process

For any OS stream deprecation:

1. **Deprecation Notice** (2+ releases before blocker):
   - Announce stream deprecation in release notes
   - Add `OSStreamDeprecated` condition to affected MachineConfigPools
   - Provide migration documentation and timelines

2. **Upgrade Blocker** (1+ releases before removal):
   - Add `Upgradeable: False` condition to ClusterVersion
   - Block upgrades until all pools migrate off deprecated stream
   - Provide clear error messages with migration instructions

3. **Stream Removal** (when support ends):
   - Remove deprecated stream from release payload
   - Add validation to prevent new pools from referencing removed stream
   - Existing pools using removed stream freeze at current version

4. **Migration Support**:
   - Provide automated migration tooling where possible
   - Document manual migration procedures
   - Support side-by-side testing (multiple pools on different streams)

**Note**: The OSImageStream API itself remains supported; only specific RHCOS versions
are deprecated. New OS versions can be added to replace deprecated ones.

## Upgrade / Downgrade Strategy

**Upgrades**: When upgrading OpenShift versions, existing MachineConfigPools maintain their current stream unless explicitly changed by administrators. This prevents unintended OS version changes during platform upgrades. See [Default Stream Evolution and Upgrade Behavior](#default-stream-evolution-and-upgrade-behavior) for details.

**Downgrades**: Stream downgrades (RHEL 10 → RHEL 9) are not supported in the initial Tech Preview implementation. Only forward migration (RHEL 9 → RHEL 10) is supported.

## Version Skew Strategy

Version skew enforcement between OS versions and OpenShift platform versions is planned for OpenShift 4.22. Initially, administrators are responsible for ensuring compatible OS/platform version combinations. The deprecation timeline (see [Removing deprecated OS streams](#removing-deprecated-os-streams)) provides upgrade blockers to prevent unsupported combinations.

## Operational Aspects of API Extensions

#### Failure Modes

**Invalid stream reference**: If a MachineConfigPool references a non-existent stream, the MCO adds a Degraded condition to the pool and does not proceed with updates.

**Missing stream labels on images**: If OS images lack the required stream labels, the MCO falls back to ConfigMap-based stream discovery.

**Conflicting stream data**: When multiple sources provide conflicting stream definitions, the MCO uses the defined precedence order (ImageStream > ConfigMap) and logs conflicts for debugging.

## Support Procedures

None.

## Implementation History

Not applicable.

## Alternatives (Not Implemented)

### Alternative 1: Expand Existing ConfigMap

Extend the `machine-config-osimageurl` ConfigMap without a new API object.

**Why Not Chosen**: Lacks schema validation, cross-object validation via VAP, discoverability through standard tooling, and status semantics. Additionally, stream discovery cannot be implemented using the ConfigMap because it only supports simple placeholder replacement logic in the installer. Adding any new stream would require manual modification to the ConfigMap, making it impractical for dynamic stream management.

### Alternative 2: Status Field on Existing MachineConfiguration Object

Add a status field to the `MachineConfiguration` object instead of creating a new resource.

**Why Not Chosen**: Different ownership (CVO vs MCO), bootstrap incompatibility, harder install-config integration, and API clarity concerns.

### Chosen Approach: New OSImageStream API

A new cluster-scoped `OSImageStream` resource provides proper API validation, MCO ownership, clear semantics, standard tooling support, and future extensibility.
