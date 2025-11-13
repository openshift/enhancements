---
title: machine-config-os-images-streams
authors:
  - "@pablintino"
  - "@dkhater-redhat"
reviewers:
  - "@yuqi-zhang"
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

This enhancement introduces a permanent multi-OS stream capability, allowing
administrators to easily assign different OS images to specific groups of nodes
using a simple "stream" identifier.

It introduces a new, optional stream field in the MCP. When this field is set,
the MCO will provision nodes in that pool using the specific OS image
associated with that stream name.

This provides a simple, declarative way to run different OS variants within
the same cluster. Use cases include testing new major OS versions (like RHEL 10)
on a subset of nodes, deploying specialized OS variants (minimal, hardened),
or facilitating gradual major version transitions—all without affecting the
rest of the cluster.

## Motivation

Currently, all nodes in an OpenShift cluster must use the same operating system
image, which is defined cluster-wide through the Machine Config Operator. This
one-size-fits-all approach creates challenges when administrators need to:

- **Test new major OS versions** (e.g., RHEL 10) before committing the entire
cluster to an upgrade
- **Gradually migrate** from one OS version to another with minimal risk
- **Validate compatibility** of workloads with new OS releases on production
infrastructure
- **Run specialized OS configurations** for specific node pools without affecting
the entire cluster

### User Stories

- **Specify OS stream per pool**: Set `spec.osImageStream` on a MachineConfigPool to provision nodes with a different OS version than the rest of the cluster
- **Discover available streams**: Query the OSImageStream cluster resource to see available OS streams in the release payload
- **Monitor stream adoption**: View `status.osImageStream` to verify when a pool has successfully adopted a new stream
- **Backward compatibility**: Existing clusters continue working without changes; the feature is opt-in via the OSStreams feature gate

### Goals

- **Enable per-pool OS stream selection**: Specify different OS streams at the MachineConfigPool level for multi-OS deployments (e.g., RHCOS 9 and RHCOS 10)
- **De-risk major OS version upgrades**: Separate platform upgrades from OS upgrades, allowing phased migration
- **Support RHCOS 9 to RHCOS 10 transition**: Enable one-directional migration path from RHEL 9 to RHEL 10 during OpenShift 4.21-5.x timeframe. This is the primary focus for Tech Preview.
- **Day-zero RHEL 10 deployments (stretch goal)**: While the architecture supports installing new clusters directly with RHEL 10, this is a nice-to-have for Tech Preview. If implementation complexity blocks delivery, the feature can ship without day-zero RHEL 10 support, focusing solely on RHEL 9 → RHEL 10 migration for existing clusters.
- **API-driven stream management**: Declarative, Kubernetes-native API for stream selection
- **Automatic stream discovery**: Populate available OS streams from release payload ImageStream metadata
- **Backward compatibility**: Existing clusters continue working; streams are opt-in via feature gate
- **Multi-source stream configuration**: Support CLI arguments, release ImageStream, and ConfigMap sources with defined precedence

### Non-Goals

- **Supporting unlimited concurrent OS streams**: While the architecture supports
multiple OS streams, this initial implementation focuses on enabling 2-3
concurrent streams (e.g., RHCOS 9 + RHCOS 10, or RHCOS 10 standard + minimal).
Supporting large numbers of concurrent streams with complex version matrices is
not a goal for the initial releases.

- **HCP/Hypershift architecture support**: Supporting OS streams in
HCP/Hypershift environments (which use NodePools instead of MachinConfigPools)
is deferred to future work. See [Topology Considerations](#topology-considerations)
for details.

- **Automatic migration or upgrade paths**: The enhancement does not include
automated migration logic or upgrade orchestration. Administrators must manually
select streams for their pools.

- **Bidirectional stream switching**: Only RHEL 9 → RHEL 10 migration is supported
in the initial Tech Preview implementation. RHEL 10 → RHEL 9 downgrade is not
supported and may be considered for future releases. The primary use case is
forward migration to newer OS versions.

- **Rollback from failed stream changes**: If a stream migration fails mid-update
(e.g., node fails to boot with new OS), automated rollback to the previous stream
is not guaranteed in Tech Preview. Manual recovery procedures may be required.
Standard MachineConfigPool rollback mechanisms apply, but stream-specific rollback
is out of scope for the initial release.

- **Version skew enforcement**: Enforcing compatibility rules between different
OS versions and OpenShift platform versions is not included in this enhancement.
This functionality is planned for OpenShift 4.22.

- **Boot image management**: Updating boot images or installation media to match
the selected stream is not covered by this enhancement.

- **bootc integration**: This enhancement continues to use rpm-ostree for image
updates in OpenShift 4.x. Migration to bootc-based updates is planned for
OpenShift 5.0+.

- **Changing the update mechanism**: The underlying OS update mechanism
(rpm-ostree) remains unchanged. This enhancement only provides stream selection
capability.

## Proposal

To implement the functionality this enhancement provides, some changes are
required in the MCO, the released images payload, and the CoreOS images.
The following sections describe all the required changes.

### Implementation Status

This enhancement is being delivered incrementally across multiple releases. Features marked as **"(Planned)"** or **"TBD"** indicate functionality that is designed but not yet fully implemented.

#### Completed (4.21 Dev Preview)
- API definitions: `osImageStream` field in MachineConfigPool, OSImageStream v1alpha1 resource
- Basic ConfigMap parsing for legacy format
- ImageStream extraction proof-of-concept

#### In Progress (4.21)
- Stream parsing logic completion (error handling, conflict resolution)
- Multi-stream ConfigMap format (`streams.json`)
- Image labels for RHEL 9/10 (CoreOS team)

#### Planned (4.21)
- OSImageStream controller for bootstrap and runtime population
- MachineConfigPool reconciliation logic
- Stream switching capability

### Machine Config Pools

To provide the user with the ability to set which stream an MCP's nodes
should use, the MCP CRD must be modified to introduce a new field:

- `spec.osImageStream`: To set the target stream the pool should use. We
preserve the current behavior of deploying the cluster-wide OS images if
no stream is set.
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

Each OS image is now built with an extra label that allows the MCO to identify 
the stream to which it belongs.

- Regular OS Images: `io.coreos.oscontainerimage.osstream` pointing to the
stream name, for example, `rhel10-coreos`.
- Extension Images: `io.coreos.osextensionscontainerimage.osstream` pointing
to the stream name, for example, `rhel10-coreos`.

With those changes to the images in place, the MCO has enough information to 
build the list of available streams and determine which images should be used
for each stream.

### Machine Config OSImageStream

This new resource holds the URLs associated with each stream and is populated
by the MCO using the information from the OS image labels. The logic that 
extracts the URLs and stream names from the OS images differs depending on 
whether the cluster is bootstrapping or undergoing an update. During regular 
operation (i.e., when not bootstrapping or updating), the MCO does not make 
any changes to this resource, and its information can be safely considered 
static.

#### TBD: Add details of both OSImageStream generation scenarios

#### Streams

### API Extensions

This enhancement introduces two API changes: modifications to the existing
MachineConfigPool API and a new OSImageStream cluster-scoped resource.

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

See https://github.com/openshift/api/pull/2555#discussion_r2476002858 for
related discussion.

#### Standalone OpenShift

This enhancement supports standalone OpenShift clusters using MachineConfigPools,
including Single-Node OpenShift (SNO).

### Implementation Details/Notes/Constraints

#### Stream Sources and Precedence

The MCO obtains OS stream information from three potential sources. The precedence
order differs between bootstrap and runtime scenarios:

**Bootstrap Precedence** (later sources override earlier):
1. **CLI Arguments** (lowest precedence, loaded first)
2. **Release ImageStream** (highest precedence, overrides CLI args)

**Runtime Precedence** (later sources override earlier):
1. **ConfigMap** (`machine-config-osimageurl`) (lowest precedence, loaded first)
2. **Release ImageStream** (highest precedence, overrides ConfigMap)

This multi-source design provides flexibility for different deployment scenarios
while maintaining backward compatibility. CLI arguments are only available during
bootstrap and are primarily used for development and testing.

##### CLI Arguments

The MCO can accept stream configuration via CLI arguments during bootstrap. This
source has the highest precedence and is primarily used for development and
testing scenarios.

##### Release ImageStream

The primary source of stream information in production environments. The MCO
extracts stream data from the release payload's ImageStream metadata:

**Extraction Process:**

1. **Location**: Stream information is read from `/release-manifests/image-references`
in the release image layers.

2. **Image Labels**: Each OS image in the ImageStream is labeled to identify its
stream association:
   - OS Images: `io.coreos.oscontainerimage.osstream` (e.g., "rhel9-coreos")
   - Extensions Images: `io.coreos.osextensionscontainerimage.osstream` (e.g., "rhel9-coreos")

3. **Grouping**: The MCO parses these labels and groups OS/Extensions image pairs
by stream name.

4. **Implementation**: Located in `pkg/controller/osimagestream/imagestream_provider.go`
and `imagestream_osimages.go`.

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

The existing `machine-config-osimageurl` ConfigMap continues to be supported for
backward compatibility. The ConfigMap schema has been evolved to support multiple
streams:

**New ConfigMap Format (with streams.json):**
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
  streams.json: |
    {
      "default": "rhel9-coreos",
      "streams": {
        "rhel9-coreos": {
          "baseOSContainerImage": "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
          "baseOSExtensionsContainerImage": "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:..."
        },
        "rhel10-coreos": {
          "baseOSContainerImage": "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:...",
          "baseOSExtensionsContainerImage": "quay.io/openshift-release-dev/ocp-v4.0-art-dev@sha256:..."
        }
      }
    }
```

> **Status**: The `streams.json` format parsing is currently in progress. While the
> ConfigMap schema and format are defined, full parsing implementation for the
> multi-stream format is not yet complete. The legacy single-stream format parsing
> is fully functional.

**Backward Compatibility:**

The old single-stream format continues to work:
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

When `streams.json` is not present, the MCO automatically creates a default stream
with a version-specific name based on the release (e.g., `"rhel9-coreos"` for OpenShift 4.x)
using the `baseOSContainerImage` and `baseOSExtensionsContainerImage` fields. This ensures
existing clusters continue to function without modification.

##### Stream Merging and Default Stream Selection

The MCO implements a `StreamSource` interface that allows composition and merging
of streams from multiple sources. The merging process:

1. **Collection**: Each source is queried for its available streams
2. **Merging**: Streams are merged into a map keyed by stream name. When multiple
sources provide the same stream name, the later source (higher precedence) wins
3. **Default Stream Identification**: The MCO searches for a stream with a
version-specific hardcoded name based on the distribution and release version:
   - RHCOS in OpenShift 4.x: `"rhel9-coreos"`
   - RHCOS in OpenShift 5.0+: `"rhel10-coreos"`
   - FCOS: `"fedora-coreos"` (unversioned for Fedora's rolling release model)
   - SCOS: `"stream-coreos"`
4. **Validation**: If the default stream is not found in the merged streams, an
error is returned

**Important**: The default stream name is **version-specific and hardcoded** based on the
MCO's build target and release version, not read from the ConfigMap's `"default"` field in
`streams.json`. The ConfigMap's `"default"` field is informational only. This ensures
explicit version selection and prevents silent OS version changes during platform upgrades.

The implementation ensures:
- Best-effort processing: if a source fails to provide streams, it's skipped with
logging
- Conflict logging: when streams are overridden, the conflict is logged for debugging
- Handling of partial stream data (OS image without Extensions or vice versa)

Implementation is located in `pkg/controller/osimagestream/osimagestream.go` and
`BuildOSImageStreamFromSources()`.

##### Default Stream Evolution and Upgrade Behavior

The default stream name changes between OpenShift releases to reflect the recommended
OS version for new clusters, while ensuring existing clusters maintain their current OS
version during platform upgrades.

**OpenShift 4.x Releases (4.21-4.23):**
- Default stream: `"rhel9-coreos"`
- Available streams: `"rhel9-coreos"`, `"rhel10-coreos"` (Tech Preview)
- New clusters install with RHCOS 9
- Existing clusters remain on their current stream

**OpenShift 5.0+ Releases:**
- Default stream: `"rhel10-coreos"`
- Available streams: `"rhel9-coreos"`, `"rhel10-coreos"`
- New clusters install with RHCOS 10
- Existing clusters upgrading from 4.x remain on their current stream (typically `"rhel9-coreos"`)

**Upgrade Behavior:**

When upgrading from OpenShift 4.x to 5.0:

1. **MachineConfigPools with explicit `spec.osImageStream` set**: Continue using the
specified stream unchanged. The stream reference is preserved across upgrades.

2. **MachineConfigPools without `spec.osImageStream` set** (using default):
   - The pool continues using `"rhel9-coreos"` even though the new default is `"rhel10-coreos"`
   - The MCO tracks which stream was being used before the upgrade and maintains it
   - This prevents **silent OS version changes** during platform upgrades

3. **New MachineConfigPools created in OpenShift 5.0**: Use the new default `"rhel10-coreos"`

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

##### Backward Compatibility with Pre-Streams Clusters

Clusters that existed before the streams feature was introduced (pre-4.21) do not have
`spec.osImageStream` set on their MachineConfigPools. These clusters must be handled
carefully to prevent unexpected OS version changes when the streams feature becomes
available.

**Legacy Behavior Mapping:**

When a cluster upgrades to an OpenShift version with the streams feature enabled
(4.21+ with OSStreams feature gate enabled):

1. **MachineConfigPools without `spec.osImageStream` set** are implicitly using the
   legacy cluster-wide OS images
2. The MCO **persists the current stream mapping** to prevent future changes:
   - For clusters running RHCOS 9: The MCO internally tracks that the pool is using
     `"rhel9-coreos"`
   - This mapping is stored (e.g., in an annotation or internal state)
   - The pool continues using `"rhel9-coreos"` even when the cluster upgrades to
     OpenShift 5.0 where the default is `"rhel10-coreos"`

3. **The mapping is permanent** unless the administrator explicitly changes it:
   - The pool never silently switches to a different OS version
   - To change OS versions, the administrator must explicitly set `spec.osImageStream`

**Implementation Approach:**

The MCO employs a **"map unversioned to versioned once"** strategy.

**Migration Path:**

Administrators can explicitly set the stream to opt out of the implicit mapping:

```yaml
# Before: Pool implicitly using rhel9-coreos
spec:
  # osImageStream not set (legacy behavior)

# After: Explicitly migrating to rhel10-coreos
spec:
  osImageStream:
    name: rhel10-coreos
```

Once `spec.osImageStream` is set, the pool uses that stream and the implicit mapping
no longer applies.

**Rationale:**

This approach ensures:
- **No silent upgrades**: Existing clusters don't
  accidentally change OS versions when upgrading OpenShift
- **Explicit migrations**: OS version changes require explicit administrator action
- **Clean semantics**: New clusters use the current default, old clusters maintain
  their current version
- **Predictable behavior**: Administrators know exactly which OS version their pools
  are running


#### OSImageStream Resource Population **(Planned - Based on PoC)**

> **Status**: The controller logic to populate OSImageStream is not yet implemented.
> The following describes the planned design based on proof-of-concept code.

The OSImageStream cluster resource will be populated by the MCO in two scenarios:

##### Bootstrap Scenario

During cluster bootstrap, the MCO builds the OSImageStream using:

**Source Priority (later sources override earlier):**
1. **CLI Arguments** (if provided): Allows override during bootstrap for
development/testing
2. **ImageStream** (if provided): The release ImageStream passed to bootstrap

**Process:**

1. The MCO reads available sources (CLI args and/or ImageStream)
2. If an ImageStream is provided, the MCO:
   - Extracts image references from the ImageStream tags
   - Inspects each OS/Extensions image to read labels (`io.coreos.oscontainerimage.osstream`
   and `io.coreos.osextensionscontainerimage.osstream`) *(requires labels to be
   added to images - see TBD below)*
   - Groups images by stream name based on these labels
3. Sources are merged with conflicts resolved by priority order
4. The default stream is identified based on the distribution and release version:
   - RHCOS in OpenShift 4.x: `"rhel9-coreos"`
   - RHCOS in OpenShift 5.0+: `"rhel10-coreos"`
   - FCOS: `"fedora-coreos"`
   - SCOS: `"stream-coreos"`
5. The OSImageStream "cluster" resource is created with the merged stream data
6. The default stream is used for initial ControllerConfig and MachineConfigPool
rendering

**Implementation Reference**: `BuildOsImageStreamBootstrap()` in
`pkg/controller/osimagestream/osimagestream.go` (PoC exists, controller
integration pending)

**TBD**: Controller integration to actually create and manage OSImageStream
resource during bootstrap.

##### Runtime/Update Scenario

During normal cluster operation and upgrades, the MCO rebuilds the OSImageStream
whenever the release payload changes:

**Source Priority (later sources override earlier):**
1. **ConfigMap** (`machine-config-osimageurl`): Loaded first
2. **Release ImageStream**: Extracted from current release image, overrides ConfigMap

**Process:**

1. The MCO reads the `machine-config-osimageurl` ConfigMap from etcd
2. The MCO extracts the ImageStream from the release image at
`/release-manifests/image-references` by:
   - Parsing the release image reference
   - Searching image layers (starting from the last layer) for the
   `/release-manifests/image-references` file
   - Reading and deserializing the ImageStream from the tar.gz layer
3. For each OS/Extensions image tag in the ImageStream:
   - Filter tags based on naming patterns or annotations (e.g., tags with
   `github.com/openshift/os` source annotation)
   - Inspect the image to read OS stream labels *(requires labels to be added to
   images - see TBD below)*
   - Extract the stream name from labels
4. Streams from both sources are merged, with ImageStream taking precedence
5. The default stream is identified (same hardcoded logic as bootstrap)
6. The OSImageStream resource is updated with the new stream data
7. Existing MachineConfigPools continue using their current streams unless
explicitly changed by the administrator

**Implementation Reference**: `BuildOsImageStreamRuntime()` in
`pkg/controller/osimagestream/osimagestream.go` (PoC exists, controller
integration pending)

**TBD**: Controller integration to watch for release payload changes and update
OSImageStream resource at runtime.

**Stream Collection Details:**

The stream collection process (`pkg/controller/osimagestream/imagestream_osimages.go`)
includes:
- Concurrent image inspection with rate limiting (max 5 concurrent inspections)
- Best-effort approach: if an image inspection fails, that image is skipped
- Conflict detection: if multiple images claim the same stream with different URLs,
the later one wins (with logging)
- Partial stream support: streams with only OS image or only Extensions image are
kept (though both are required for a functional stream)

**Image Labels Requirement:**

For the ImageStream-based stream extraction to work, RHEL CoreOS images must be
built with the following labels:
- **OS Images**: `io.coreos.oscontainerimage.osstream=<stream-name>`
  (e.g., `"rhel9-coreos"`, `"rhel10-coreos"`)
- **Extensions Images**: `io.coreos.osextensionscontainerimage.osstream=<stream-name>`
  (e.g., `"rhel9-coreos"`, `"rhel10-coreos"`)

**Label Naming Rationale:**

The `io.coreos.*` namespace is used for these labels (rather than `io.openshift.*`)
because:
- These labels are set by the CoreOS build system and are part of the CoreOS image
metadata schema
- The CoreOS team already adds `com.coreos.stream` labels for internal purposes
- The `io.coreos.oscontainerimage.osstream` label contains the **OpenShift-facing stream name**
(e.g., `"rhel9-coreos"`) which abstracts over CoreOS's internal minor-version streams
- This label is specifically for OpenShift's consumption, complementing CoreOS's internal
`com.coreos.stream` label
- Using the `io.coreos` namespace maintains consistency with other CoreOS-originated
image metadata

The stream names in these labels (`"rhel9-coreos"`, `"rhel10-coreos"`) are major-version
granularity and designed for administrator use in OpenShift APIs, unlike CoreOS's internal
minor-version stream names (`rhel-9.6`, `rhel-9.8`).

**Current Status**: The RHEL 9 and RHEL 10 image tags are available in the
ImageStream, but the labels have not yet been added to the images. This work is
tracked with the CoreOS team. See the
[Implementation Status](#implementation-status) section for details.

#### MachineConfigPool Reconciliation **(Planned - Not Yet Implemented)**

> **Status**: This functionality is planned but not yet implemented. The following
> describes the intended design.

When a user sets `spec.osImageStream` on a MachineConfigPool, the planned
reconciliation flow is:

1. **Validation**: The MCO validates that the referenced stream exists in the
OSImageStream resource's `availableStreams` list.

2. **URL Lookup**: The MCO looks up the `osImage` and `osExtensionsImage` for
the specified stream from the OSImageStream resource.

3. **MachineConfig Rendering**: The MCO generates a new rendered MachineConfig
for the pool with the stream's OS images, effectively overriding the
cluster-wide defaults.

4. **Node Rollout**: The standard MCP update mechanism triggers, rolling out the
new OS images to each node in the pool one by one.

5. **Status Update**: Once all nodes in the pool have successfully updated,
`status.osImageStream` is updated to reflect the new stream.

The reconciliation logic will treat stream changes identically to other
MachineConfig changes, leveraging existing update and rollback mechanisms.

**TBD**: Implementation of MCP controller logic to consume `osImageStream` field.

#### Release Payload Integration

The OS stream feature requires coordination with the OpenShift release build
process:

1. **MCO Image**: The MCO container image includes the `machine-config-osimageurl`
ConfigMap manifest with placeholder values in
`install/0000_80_machine-config_05_osimageurl.yaml`.

2. **Image References**: The MCO image also includes an `image-references` file
that the `oc` CLI uses during release image creation.

3. **Placeholder Replacement**: When building the release image, `oc` reads the
`image-references` file and replaces placeholders in the ConfigMap manifest with
actual image URLs from the Release ImageStream tags.

4. **CVO Application**: The Cluster Version Operator (CVO) applies the
pre-populated ConfigMap when deploying the MCO, ensuring stream information is
available from the start.

This workflow ensures that stream URLs are always synchronized with the release
payload without requiring runtime discovery or external configuration.

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

### Risks and Mitigations

**TBD**

### Drawbacks

**TBD**

## Design Details

### Open Questions [optional]

None.

## Test Plan

### Testing Strategy

The OS streams feature requires comprehensive testing to ensure safe RHEL 9 to RHEL 10
transitions and validate component readiness on both OS versions.

### Test Infrastructure

**RHEL 10 Payload Generation:**

Starting in OpenShift 4.20/4.21, separate RHEL 10 release streams will be produced to
enable early testing:
- Custom RHEL 10-based payloads from nightly builds (similar to F-COS/S-COS for OKD)
- Enables testing RHEL 10 before it becomes the default in OpenShift 5.0
- Timeline:
  - 4.21: RHEL 10 payloads available as separate stream
  - 4.22/4.23: Tech preview jobs begin running on RHEL 10 for component readiness
  - 5.0: RHEL 10 becomes default for new clusters

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
- Goal: Identify RHEL 10-specific issues before OpenShift 5.0 GA

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

### OpenShift 5.0+: Continued Support and GA Path

**Target Release**: OpenShift 5.0 (RHCOS 10.2 default, existing clusters remain on 9)

**Changes:**

- RHCOS 10 uses bootc exclusively for image updates
- RHCOS 9 uses bootc when possible (fallback to rpm-ostree)
- Existing clusters remain on RHCOS 9 until administrators explicitly change streams
- Customers can separate platform upgrade (to 5.0) from OS migration (9 → 10)

**Feature Evolution:**

- Feature likely becomes default-enabled
- Possible promotion to v1
- Multi-OS stream capability remains as a permanent feature
- Specific dual-stream support (RHCOS 9 + 10) continues through at least 5.2 (first 5.x EUS)

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

**GA Target**: OpenShift 5.x after validation in 4.21-4.23 releases

**Post-GA**: Remains a permanent capability for major OS transitions, specialized variants, and custom base images

### Removing deprecated OS streams

Specific OS streams will be deprecated as they reach end-of-life. The deprecation process
includes proactive upgrade blockers to ensure clusters migrate off deprecated streams before
they become unsupported.

#### RHEL 9 Stream Deprecation Timeline (Example)

While the multi-OS stream feature is permanent, specific OS versions like RHEL 9 will
eventually be deprecated. The expected timeline:

**OpenShift 5.0-5.1 (estimated):**
- Both `rhel9-coreos` and `rhel10-coreos` streams fully supported
- No warnings or blockers
- Clusters can run either stream

**OpenShift 5.2 (estimated):**
- `rhel9-coreos` stream marked as deprecated
- Warning conditions added to MachineConfigPools using `rhel9-coreos`:
  ```yaml
  status:
    conditions:
    - type: OSStreamDeprecated
      status: True
      reason: StreamEndOfLife
      message: "Stream 'rhel9-coreos' is deprecated and will be removed in OpenShift 5.3.
               Migrate to 'rhel10-coreos' before upgrading."
  ```
- Cluster-level upgrade remains possible (no blocker yet)
- Documentation and alerts guide administrators to plan migration

**OpenShift 5.3 or later (estimated):**
- Upgrade blocker activated for clusters with pools still using `rhel9-coreos`
- ClusterVersion resource sets `Upgradeable: False`:
  ```yaml
  status:
    conditions:
    - type: Upgradeable
      status: False
      reason: DeprecatedOSStreamInUse
      message: "Cannot upgrade: MachineConfigPool 'worker' is using deprecated stream
               'rhel9-coreos' which is not supported in OpenShift 5.3. Migrate pool to
               'rhel10-coreos' before upgrading."
  ```
- Cluster cannot upgrade to 5.3+ until all pools migrate to `rhel10-coreos`
- This provides a **forcing function** to complete OS migration

**Post-5.3 (estimated):**
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

**TBD**

## Version Skew Strategy

**TBD**

## Operational Aspects of API Extensions

#### Failure Modes

**TBD**

## Support Procedures

None.

## Implementation History

Not applicable.

## Alternatives (Not Implemented)

### Alternative 1: Expand Existing ConfigMap

Extend the `machine-config-osimageurl` ConfigMap without a new API object.

**Why Not Chosen**: Lacks schema validation, cross-object validation via VAP, discoverability through standard tooling, and status semantics.

### Alternative 2: Status Field on Existing MachineConfiguration Object

Add a status field to the `MachineConfiguration` object instead of creating a new resource.

**Why Not Chosen**: Different ownership (CVO vs MCO), bootstrap incompatibility, harder install-config integration, and API clarity concerns.

### Chosen Approach: New OSImageStream API

A new cluster-scoped `OSImageStream` resource provides proper API validation, MCO ownership, clear semantics, standard tooling support, and future extensibility.
