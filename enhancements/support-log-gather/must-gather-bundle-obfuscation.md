---
title: must-gather-bundle-obfuscation
authors:
  - "@shivprakashmuley"
reviewers:
  - "@TrilokGeer"
  - "@Prashanth684"
  - "@swghosh"
approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
api-approvers:
  - "@TrilokGeer"
  - "@Prashanth684"
creation-date: 2026-07-06
tracking-link:
  - https://issues.redhat.com/browse/MG-293

---

# Must-Gather Bundle Obfuscation

## Summary

Integrate [must-gather-clean](https://github.com/openshift/must-gather-clean) into the must-gather-operator to enable automatic obfuscation of sensitive data (IP addresses, MAC addresses, Kubernetes Secrets, ConfigMaps) in must-gather bundles before they are uploaded to Red Hat support cases. When a user configures `obfuscate.enabled: true` on a MustGather CR, the operator runs must-gather-clean as a post-gather, pre-upload step inside the existing upload container, requiring no new images, containers, or sidecar coordination. Users can optionally provide a custom obfuscation config via a ConfigMap reference (`obfuscate.obfuscationConfigRef`) to tailor redaction rules to their needs.

## Motivation

When customers submit must-gather bundles to Red Hat Support, those bundles contain cluster state that may include sensitive information: IP addresses revealing network topology, MAC addresses identifying hardware, Secrets containing credentials, and ConfigMaps with internal configuration. Customers in regulated industries (finance, healthcare, government) are often unable or reluctant to share this data, even with their support provider.

Today, obfuscation requires a separate manual step using the `must-gather-clean` CLI tool after collecting the bundle. This creates friction in the support workflow and means most bundles are uploaded without any redaction.

### User Stories

- As an OpenShift cluster administrator, I want to set `obfuscate.enabled: true` on my MustGather CR so that sensitive data is automatically redacted before the bundle is uploaded to my Red Hat support case.

- As a security engineer in a regulated industry, I want to ensure that IP addresses, MAC addresses, Secrets, and ConfigMaps are removed or anonymized from diagnostic bundles so that our organization's network topology and credentials are not exposed to external parties.

- As a Red Hat support engineer, I want obfuscated bundles to use consistent replacements so I can still correlate events across resources (e.g., `x-ipv4-0000001-x` always refers to the same original IP within a bundle).

- As an OpenShift administrator, I want obfuscation logs included in the uploaded bundle so that I can audit what was redacted and verify compliance with my organization's data policies.

- As an OpenShift administrator, I want to obfuscate a previously collected must-gather bundle stored on a PVC without re-running a gather, so that I can redact sensitive data from existing bundles before sharing them.

- As an OpenShift administrator, I want to obfuscate an existing must-gather bundle on a PVC and automatically upload it to a Red Hat support case, so that I can redact and submit previously collected diagnostic data in a single step.

- As a security engineer, I want to provide a custom obfuscation config via a ConfigMap so that I can define organization-specific redaction rules (e.g., obfuscate only IPs but keep ConfigMaps, or add domain-specific patterns) without building a custom operator image.

### Goals

1. Enable one-step gather-and-obfuscate-and-upload via the `obfuscate` config on the MustGather CR (gather -> obfuscate -> upload)
2. Support obfuscation of existing bundles on a PVC without re-running a gather (obfuscate-only, via `obfuscate.source`)
3. Support obfuscation of existing bundles followed by upload without re-running a gather (obfuscate -> upload, via `obfuscate.source` + `uploadTarget`)
4. Obfuscate sensitive data automatically before SFTP upload
5. Use consistent replacement so patterns remain debuggable (e.g., all occurrences of the same IP map to the same anonymized value)
6. Ship a safe default obfuscation config that omits Secrets and ConfigMaps, and consistently replaces IPs and MACs
7. Allow users to provide custom obfuscation rules via a ConfigMap reference (`obfuscationConfigRef`) to override the default config
8. Include obfuscation logs in the output bundle for auditability

### Non-Goals

1. Changes to the must-gather-clean library API (consumed as-is)
2. Domain-specific obfuscation patterns shipped as part of the default config (users provide these via `obfuscationConfigRef`)
3. Obfuscation progress reporting in the MustGather CR status

## Proposal

Add a new `obfuscate` field to `MustGatherSpec` that groups all obfuscation-related configuration:

- **`enabled`** (bool): Activates obfuscation.
- **`obfuscationConfigRef`** (object, optional): References a ConfigMap with custom obfuscation rules.
- **`source`** (object, optional): References an existing must-gather bundle on a PVC for obfuscation without running a new gather.

These sub-fields enable three supported operational modes:

| Mode | Fields Used | Description |
|------|-------------|-------------|
| Gather + Obfuscate + Upload | `obfuscate.enabled: true` + `uploadTarget` | Full pipeline: collect, redact, upload |
| Obfuscate Only | `obfuscate.enabled: true` + `obfuscate.source` | Redact an existing bundle from a PVC to a staging directory, no gather, no upload |
| Obfuscate + Upload | `obfuscate.enabled: true` + `obfuscate.source` + `uploadTarget` | Redact an existing bundle and upload it, no gather |

### Workflow Description

**Cluster administrator** is a human user responsible for collecting and uploading diagnostic data.

#### Mode 1: Gather + Obfuscate + Upload

This is the primary workflow for new must-gather collections with obfuscation.

1. The cluster administrator creates a MustGather CR with `spec.obfuscate.enabled: true` and an `uploadTarget` configured
2. The Must-Gather Operator creates a Job with gather and upload containers
3. The gather container runs `/usr/bin/gather`, writes output to the shared `/must-gather` volume, and appends `chown -R 65534:65534 /must-gather` before exiting (transferring ownership to the non-root upload container's uid)
4. The upload container detects gather completion via `pgrep` polling
5. The upload script checks the `obfuscate` environment variable
6. The operator binary is invoked with a separate output directory:
   `must-gather-operator obfuscate --input /must-gather --output /must-gather-upload/cleaned" -v=3`
7. If `obfuscationConfigRef` is set, the referenced ConfigMap is mounted and used as the config; otherwise the built-in config is used
8. The obfuscation engine walks all files, applying the configured replacements and omissions, writing cleaned output to `/must-gather-upload/cleaned`
9. The upload script redirects `must_gather_output` to the cleaned directory (`must_gather_output="/must-gather-upload/cleaned"`)
10. An `obfuscation.log` is preserved in the cleaned output for auditability
11. The upload script proceeds to tar and SFTP upload from the cleaned directory
12. The MustGather CR status is updated to Completed

#### Mode 2: Obfuscate Only (No Gather, No Upload)

This mode allows administrators to redact an existing must-gather bundle stored on a PVC without re-collecting or uploading. Since the operator does not support updates to an existing MustGather CR, the administrator must create a **new** MustGather CR for the obfuscation run.

1. The cluster administrator has a previously collected must-gather bundle persisted on a PVC (e.g., from a prior MustGather CR with `spec.storage`)
2. The administrator creates a **new** MustGather CR with `spec.obfuscate.enabled: true` and `spec.obfuscate.source` referencing the PVC
3. The operator creates a Job with only an upload container (no gather container)
4. The upload container mounts the PVC at `/must-gather`
5. The operator binary is invoked with the appropriate config (custom or default)
6. Obfuscation reads from the PVC and writes cleaned output to `/must-gather-upload/cleaned`
7. The `obfuscation.log` is written to the cleaned output
8. The MustGather CR status is updated to Completed
9. The original bundle on the PVC remains untouched; the obfuscated bundle is in the staging directory for the administrator to retrieve

#### Mode 3: Obfuscate + Upload (No Gather)

This mode allows administrators to redact an existing bundle and upload it to a support case without re-collecting. As with Mode 2, a **new** MustGather CR must be created (the operator does not support spec updates on existing CRs).

1. The cluster administrator has a previously collected must-gather bundle on a PVC
2. The administrator creates a **new** MustGather CR with `spec.obfuscate.enabled: true`, `spec.obfuscate.source` referencing the PVC, and `spec.uploadTarget` configured
3. The operator creates a Job with only an upload container (no gather container)
4. The upload container mounts the PVC at `/must-gather`
5. Obfuscation runs on the bundle (using custom or default config)
6. The upload script proceeds to tar and SFTP upload
7. The MustGather CR status is updated to Completed

**Error case (all modes)**: If obfuscation fails (e.g., unreadable files, disk full), the upload container exits non-zero, the Job fails.

### API Extensions

One new field added to the existing `MustGatherSpec` in the `mustgathers.operator.openshift.io` CRD:

```go
// MustGatherSpec defines the desired state of MustGather
type MustGatherSpec struct {
    // ... existing fields ...

    // obfuscate configures post-gather obfuscation of sensitive data
    // (IPs, MACs, Secrets, ConfigMaps) before upload using must-gather-clean.
    // When obfuscate.enabled is true, the operator runs obfuscation on the
    // collected bundle before tarring and uploading.
    // +optional
    Obfuscate *ObfuscateConfig `json:"obfuscate,omitempty"`
}

// ObfuscateConfig configures the obfuscation behavior for a MustGather run.
type ObfuscateConfig struct {
    // enabled activates obfuscation of the must-gather bundle.
    // When true, the operator runs must-gather-clean on the collected or
    // referenced bundle before tarring and uploading.
    // +kubebuilder:default:=false
    // +optional
    Enabled *bool `json:"enabled,omitempty"`

    // obfuscationConfigRef references a ConfigMap in the operator namespace
    // containing a must-gather-clean configuration file.
    // The ConfigMap must have a key named "config.yaml" whose value is a
    // valid must-gather-clean obfuscation config.
    // If omitted, the operator uses the built-in default config which
    // consistently replaces IPs and MACs, and omits Secrets and ConfigMaps.
    // +optional
    ObfuscationConfigRef *corev1.LocalObjectReference `json:"obfuscationConfigRef,omitempty"`

    // source references an existing must-gather bundle on a PVC
    // for obfuscation without running a new gather.
    // When set, the operator skips the gather step and runs obfuscation
    // directly on the referenced PVC contents.
    // +optional
    Source *ObfuscateSourceConfig `json:"source,omitempty"`
}

// ObfuscateSourceConfig defines the source of an existing must-gather bundle
// to obfuscate without running a new gather.
type ObfuscateSourceConfig struct {
    // claim references the PersistentVolumeClaim containing the existing
    // must-gather bundle to obfuscate.
    // The PVC must be in the operator namespace.
    // +required
    Claim PersistentVolumeClaimReference `json:"claim"`

    // subPath is the path within the PVC where the must-gather bundle
    // is located. If omitted, the root of the PVC is used.
    // +optional
    SubPath string `json:"subPath,omitempty"`
}
```

No new CRDs. The `obfuscate` field is optional and defaults to `nil`, making this fully backward compatible.

#### Example CRs

**Mode 1: Gather + Obfuscate + Upload (built-in config)**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: obfuscated-gather
  namespace: openshift-must-gather-operator
spec:
  serviceAccountName: must-gather-admin
  obfuscate:
    enabled: true
  uploadTarget:
    type: SFTP
    sftp:
      caseID: "02527285"
      caseManagementAccountSecretRef:
        name: case-management-creds
      internalUser: true
```

**Mode 1: Gather + Obfuscate + Upload (custom config)**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: obfuscated-gather-custom
  namespace: openshift-must-gather-operator
spec:
  serviceAccountName: must-gather-admin
  obfuscate:
    enabled: true
    obfuscationConfigRef:
      name: my-obfuscation-rules
  uploadTarget:
    type: SFTP
    sftp:
      caseID: "02527285"
      caseManagementAccountSecretRef:
        name: case-management-creds
      internalUser: true
```

**Mode 2: Obfuscate Only (existing bundle on PVC, no gather, no upload)**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: obfuscate-existing-bundle
  namespace: openshift-must-gather-operator
spec:
  serviceAccountName: must-gather-admin
  obfuscate:
    enabled: true
    obfuscationConfigRef:
      name: my-obfuscation-rules
    source:
      claim:
        name: my-mustgather-pvc
      subPath: "must-gather-2026-06-25"
```

**Mode 3: Obfuscate + Upload (existing bundle on PVC, no gather)**

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: MustGather
metadata:
  name: obfuscate-and-upload
  namespace: openshift-must-gather-operator
spec:
  serviceAccountName: must-gather-admin
  obfuscate:
    enabled: true
    source:
      claim:
        name: my-mustgather-pvc
      subPath: "must-gather-2026-06-25"
  uploadTarget:
    type: SFTP
    sftp:
      caseID: "02527285"
      caseManagementAccountSecretRef:
        name: case-management-creds
```

#### Custom Obfuscation ConfigMap Example

The ConfigMap referenced by `obfuscationConfigRef` must contain a key `config.yaml` with a valid must-gather-clean config:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-obfuscation-rules
  namespace: openshift-must-gather-operator
data:
  config.yaml: |
    config:
      obfuscate:
        - type: IP
          replacementType: Consistent
          target: All
        - type: MAC
          replacementType: Consistent
          target: All
        - type: Regex
          regex: "my-internal-domain\\.corp\\.example\\.com"
          replacementType: Consistent
      omit:
        - type: Kubernetes
          kubernetesResource:
            kind: "Secret"
```

Users can customize which data types are obfuscated, add regex-based patterns for domain-specific content, and choose which Kubernetes resources to omit. The format follows the [must-gather-clean configuration schema](https://github.com/openshift/must-gather-clean#configuration).

### Topology Considerations

#### Hypershift / Hosted Control Planes

No unique considerations.

#### Standalone Clusters


#### Single-node Deployments or MicroShift


#### OpenShift Kubernetes Engine


### Implementation Details/Notes/Constraints

#### Architecture

Obfuscation runs as a post-gather, pre-upload step inside the existing upload container. No new containers, no new images, no new sidecar coordination.

```
                          Job Pod
┌────────────────────────────────────────────────────────┐
│                                                        │
│  gather container (uid 0)     upload container (uid    │
│                               65534)                   │
│  ┌──────────────────┐        ┌──────────────────────┐  │
│  │ /usr/bin/gather   │        │ 1. poll gather       │  │
│  │                   │        │ 2. obfuscate:        │  │
│  │ writes to         │ output │    read /must-gather  │  │
│  │ /must-gather ─────┼───vol──│    write to staging   │  │
│  │                   │        │    dir on upload vol   │  │
│  │ chown -R 65534    │        │    atomic rename back │  │
│  │ /must-gather      │        │    to /must-gather    │  │
│  │ (when obfuscate)  │        │ 3. tar /must-gather   │  │
│  │                   │ upload │ 4. sftp upload        │  │
│  └──────────────────┘  ──vol──└──────────────────────┘  │
│                                                        │
│  Volumes:                                              │
│    output vol = /must-gather (emptyDir or PVC)         │
│    upload vol = /must-gather-upload (emptyDir,         │
│                 staging area for cleaned files + tar)   │
└────────────────────────────────────────────────────────┘
```

#### Volume Layout

The Job pod uses two volumes shared between the gather and upload containers:

| Volume | Mount Path | Type | Purpose |
|--------|------------|------|---------|
| `must-gather-output` | `/must-gather` | emptyDir or PVC | Gather container writes collected data; upload container reads for obfuscation |
| `must-gather-upload` | `/must-gather-upload` | emptyDir | Upload container workspace for staging cleaned output and tar archives |

#### Component Changes

**1. CRD types (`api/v1alpha1/mustgather_types.go`)**

Added `Obfuscate *ObfuscateConfig` struct (with `Enabled *bool`, `ObfuscationConfigRef` LocalObjectReference, and `Source *ObfuscateSourceConfig`) to `MustGatherSpec`.

**2. Obfuscation logic (`main.go`)**

A `runObfuscate` function is added to `main.go`, invoked by the upload script:
- Accepts `--input` (required), `--output` (required), and `--config` (defaults to built-in config)
- Imports `go.uber.org/automaxprocs` to set `GOMAXPROCS` to the container's CPU limit
- Calls `mgclean.Run()` to perform obfuscation using 4 parallel workers
- Writes cleaned files to the output directory, preserving directory structure
- Writes an `obfuscation.log` file into the output for auditability

**3. Job template (`controllers/mustgather/template.go`)**

Changes when `spec.obfuscate.enabled` is true:

*Gather container*: Appends `chown -R 65534:65534 /must-gather` to the gather command, transferring file ownership to the upload container's uid (65534). This ensures `must-gather-clean` can read input files and `os.Chown` output files without `EPERM` errors. When `spec.obfuscate.source` is set, the gather container is omitted entirely from the Job spec.

*Upload container*: Sets the `obfuscate` environment variable to `"true"`. When `spec.obfuscate.obfuscationConfigRef` is set, the referenced ConfigMap is mounted as a volume at `/etc/must-gather-clean/custom-config/config.yaml` and the `obfuscate_config` environment variable is set to that path. When `spec.obfuscate.source` is set, the referenced PVC is mounted at `/must-gather` instead of the emptyDir volume, and the gather-completion polling is skipped.

Both `getGatherContainer` and `getUploadContainer` function signatures are extended with an `obfuscate *ObfuscateConfig` parameter.

**4. Upload script (`build/bin/upload`)**

Lines added before the tar step:

```sh
if [ "${obfuscate}" = "true" ]; then
  config_flag=""
  if [ -n "${obfuscate_config}" ]; then
    config_flag="--config ${obfuscate_config}"
  fi
  echo "Running obfuscation on $must_gather_output ..."
  /usr/local/bin/must-gather-operator obfuscate \
    --input "$must_gather_output" \
    --output "/must-gather-upload/cleaned" \
    ${config_flag} -v=3
  echo "Obfuscation complete."
  must_gather_output="/must-gather-upload/cleaned"
fi
```

**5. Default obfuscation config (`build/obfuscate-config.yaml`)**

Baked into the operator image at `/etc/must-gather-clean/default-config.yaml`:

```yaml
config:
  obfuscate:
    - type: IP
      replacementType: Consistent
      target: All
    - type: MAC
      replacementType: Consistent
      target: All
  omit:
    - type: Kubernetes
      kubernetesResource:
        kind: "Secret"
    - type: Kubernetes
      kubernetesResource:
        kind: "ConfigMap"
```

**6. Dockerfiles (`build/Dockerfile`, `Dockerfile.openshift`)**

One line added to copy the default config:

```dockerfile
COPY --from=builder .../build/obfuscate-config.yaml /etc/must-gather-clean/default-config.yaml
```

**7. Go module (`go.mod`)**

Added `must-gather-clean` as a dependency.

#### Permission Model

The obfuscation step bridges a permission gap between two containers with different UIDs:

| Container | UID | Permissions |
|-----------|-----|-------------|
| gather | 0 (root) | Creates all files as root-owned in `/must-gather` |
| upload | 65534 (nobody) | Reads input from `/must-gather`, writes cleaned output to `/must-gather-upload` |

This is resolved by a single mechanism:

**`chown` by gather container**: When `obfuscate.enabled: true`, the gather container's command is extended with `chown -R 65534:65534 /must-gather`. This runs as root (which has `CAP_CHOWN`) before the gather container exits, transferring ownership of all collected files to uid 65534. The upload container can then read, write, and `os.Chown` these files without permission errors. No upstream patch to `must-gather-clean` is required.

**Why not run the upload container as root?** Running as root would eliminate the permission gap entirely but violates the principle of least privilege, conflicts with OpenShift's restricted SCC, and would require explicit `anyuid` SCC bindings.

#### Default Obfuscation Behavior

| Data Type | Action | Details |
|-----------|--------|---------|
| IPv4 addresses | Consistent replacement | `10.0.1.5` → `x-ipv4-0000001-x` everywhere in the bundle |
| MAC addresses | Consistent replacement | `0e:a0:e7:92:3a:a3` → `x-mac-0000001-x` |
| Kubernetes Secrets | Omitted entirely | Files containing `kind: Secret` are excluded from output |
| Kubernetes ConfigMaps | Omitted entirely | Files containing `kind: ConfigMap` are excluded from output |
| Local IPs (127.0.0.1, 0.0.0.0, ::1) | Preserved | Not obfuscated |

#### Performance Impact

Obfuscation adds a CPU-bound processing step between gather completion and upload. The following benchmarks were collected on an OCP cluster with 3 control-plane nodes and 3 worker nodes.

**Benchmarks**:

| Workers | Bundle Size | Sensitive Data Density | Obfuscation Time |
|---------|-------------|------------------------|------------------|
| 4 | ~200 MB | Light | ~50 seconds |
| 4 | ~400 MB | Light | ~90 seconds |
| 8 | ~400 MB | Light | ~53 seconds |
| 8 | ~600 MB | Heavy | ~2 minutes |
| 8 | ~1.7 GB | Heavy | ~5 minutes |
| 8 | ~2 GB | Heavy | ~7 minutes |

*"Light" = typical cluster logs with standard IP/MAC occurrences. "Heavy" = logs with dense sensitive data (many unique IPs, MACs, Secrets, ConfigMaps) requiring more regex matching and replacement map lookups.*

**Key observations**:

- Obfuscation is CPU-bound. The bottleneck is regex matching and consistent replacement across file contents.
- Doubling workers from 4 to 8 on a 400 MB bundle cut time from ~90s to ~53s (~41% reduction), confirming CPU-bound scaling.
- Bundles with heavy sensitive data density take significantly longer at the same size (e.g., ~600 MB heavy takes ~2 min vs ~400 MB light at ~53s with 8 workers) due to more regex matches and replacement map operations.
- The operator uses `automaxprocs` to detect the container's cgroup CPU limit and sets `GOMAXPROCS` accordingly. Worker count is hardcoded to 4 initially; this can be revisited in a future performance-focused proposal based on production data.
- The operator writes cleaned output to `/must-gather-upload/cleaned`, requiring temporary disk space equal to the bundle size on the upload volume during processing.
- The upload step (tar + SFTP) is typically the longest phase of the pipeline. Obfuscation adds a smaller fraction of time relative to network-bound upload.

**Mitigation for large bundles**: Users can combine `obfuscate.enabled: true` with `gatherSpec.since` (time-based filtering from the time-filter enhancement) to reduce bundle size before obfuscation, improving both gather and obfuscation performance.

### Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `must-gather-clean` calls `klog.Exitf` on file errors, terminating the upload container | Upload fails hard instead of returning a recoverable error | The obfuscation runs in the upload container (not the operator controller), so the operator process itself is unaffected; document that file-level errors are fatal |
| Obfuscation increases Job duration for large bundles | Longer time to upload for support cases | Obfuscation is CPU-bound and parallelized; see Performance Impact section for benchmarks |
| Container CPU limits may be lower than expected | 4 hardcoded workers may exceed available CPU, causing throttling | `automaxprocs` detects cgroup CPU limits and sets `GOMAXPROCS` accordingly; acceptable since obfuscation is a short-lived batch operation. Worker count can be revisited in a future performance-focused proposal |
| Omitted resources (Secrets, ConfigMaps by default) are permanently excluded from the bundle | Support engineers will not have access to omitted resources, potentially limiting their ability to diagnose issues | Document clearly that any resource type configured for omission will be unavailable to support; users can adjust omission rules via `obfuscationConfigRef` |
| `obfuscate.source` mode requires additional disk space | Cleaned output is written to a staging directory (`/must-gather-upload/cleaned`), requiring temporary disk space equal to the bundle size | Acceptable since the upload volume is an emptyDir sized for staging; the original bundle on the PVC remains untouched |
| Custom must-gather images may produce non-standard output structures | Obfuscation may miss or incorrectly process files from custom images | `must-gather-clean` operates on file content (regex-based), not directory structure, so it is largely format-agnostic. Document that custom image support is best-effort |
| Invalid custom obfuscation ConfigMap causes runtime failure | Obfuscation fails, Job fails, no upload occurs | Operator logs the error clearly; `obfuscation.log` captures the failure reason. Config validation is best-effort at the library level |
| CPU-intensive obfuscation pod scheduled on control-plane or infra nodes could starve critical components | Degraded API server, etcd, or ingress performance during obfuscation of large bundles | The Job pod template includes node affinity and tolerations to schedule obfuscation pods exclusively on worker nodes, avoiding control-plane and infra nodes |

### Drawbacks

- Adds a dependency on the `must-gather-clean` library, increasing the operator binary size and vendor footprint
- The default ConfigMap omission is aggressive -- some ConfigMaps contain non-sensitive operational data useful for debugging (mitigated by custom config support)
- Custom config validation is deferred to runtime: a malformed ConfigMap will not be caught at CR creation time, only when the obfuscation Job runs

## Alternatives (Not Implemented)

### Separate Obfuscation Container (Init or Sidecar)

A dedicated container running the `must-gather-clean` binary between gather and upload.

**Rejected because**:
- No `must-gather-clean` container image exists today; building one adds CI/release complexity
- Container ordering between gather and upload already uses `pgrep`-based polling; adding a third container further complicates coordination
- Invoking the obfuscation logic directly in the upload container achieves the same result with zero additional images

### Running Upload Container as Root (uid 0)

Would eliminate the need for `chown` in the gather container.

**Rejected because**:
- Violates principle of least privilege
- Conflicts with OpenShift's restricted SCC; would require `anyuid` SCC binding
- Pod Security Admission at `restricted` level rejects root containers
- Breaking change for existing deployments

### Obfuscation in the Gather Container

Running obfuscation as part of the gather container's entrypoint.

**Rejected because**:
- The gather container runs a must-gather image (not the operator image), which doesn't contain `must-gather-clean`
- Would require modifying upstream must-gather images or building custom ones
- Separating gather from obfuscation maintains clean responsibility boundaries

### Inline Obfuscation Config in the CR

Embedding the full must-gather-clean config YAML directly in the MustGather CR spec (e.g., as a string field).

**Rejected because**:
- Large YAML configs embedded in a CR are unwieldy and error-prone
- ConfigMaps are the standard Kubernetes pattern for mounting configuration files
- ConfigMap approach allows reuse of the same config across multiple MustGather CRs
- ConfigMap contents can be managed independently by security teams

## Open Questions

1. **Custom must-gather images**: Should obfuscation be supported when users specify custom must-gather images (e.g., product-specific gather images like `registry.redhat.io/openshift-logging/cluster-logging-rhel9-operator`)? Custom images may produce output in different directory structures or file formats that `must-gather-clean` may not handle correctly. Should the operator validate compatibility, or should obfuscation be best-effort for custom images?

2. Should `obfuscate.enabled: true` without an `uploadTarget` and without `obfuscate.source` be rejected via CEL validation, or should it be allowed (e.g., gather + obfuscate to PVC without upload)?

3. Should the obfuscation report (`report.yaml` with replacement mappings) be surfaced in the MustGather CR status, or is inclusion in the bundle sufficient?

4. Should the operator validate that the referenced `obfuscationConfigRef` ConfigMap exists and contains a `config.yaml` key at CR admission time (via webhook), or only at Job execution time?

## Test Plan

### Unit Tests

- `template_test.go`: Verify `getGatherContainer` appends `chown -R 65534:65534` when `obfuscate.enabled` is true, does not when false
- `template_test.go`: Verify `getUploadContainer` passes `obfuscate` env var when `obfuscate.enabled` is true, does not when false
- `template_test.go`: Verify `getUploadContainer` mounts the ConfigMap and sets `obfuscate_config` env var when `obfuscationConfigRef` is set
- `template_test.go`: Verify Job template omits the gather container when `obfuscate.source` is set
- `template_test.go`: Verify PVC from `obfuscate.source` is mounted correctly in the upload container
- Existing tests updated to include the new `obfuscate` parameter (backward compatible with `nil`)

### Integration Tests

- Verify `runObfuscate()` correctly processes a test bundle directory
- Verify output is written to the staging directory without modifying the input
- Verify `obfuscation.log` is present in the output
- Verify IP addresses are consistently replaced across multiple files

### E2E Tests

**Mode 1 (Gather + Obfuscate + Upload)**:
- Create MustGather CR with `obfuscate.enabled: true` and SFTP upload target
- Verify upload container logs contain "Running obfuscation" and "Obfuscation complete"
- Download uploaded bundle from SFTP, verify:
  - IP addresses are replaced with `x-ipv4-*` patterns
  - No files with `kind: Secret` exist in the bundle
  - `obfuscation.log` exists in the bundle
  - `report.yaml` exists with replacement mappings

**Mode 1 with custom config**:
- Create a ConfigMap with a custom config that only obfuscates IPs (no MAC, no Secret/ConfigMap omission)
- Create MustGather CR with `obfuscate.enabled: true` and `obfuscationConfigRef` pointing to the ConfigMap
- Verify IPs are obfuscated but MACs are preserved and Secrets/ConfigMaps are not omitted

**Mode 2 (Obfuscate Only)**:
- Pre-populate a PVC with a must-gather bundle
- Create MustGather CR with `obfuscate.enabled: true` and `obfuscate.source` referencing the PVC
- Verify no gather container is created in the Job
- Verify the original PVC contents are untouched and cleaned output is in the staging directory
- Verify `obfuscation.log` exists on the PVC

**Mode 3 (Obfuscate + Upload)**:
- Pre-populate a PVC with a must-gather bundle
- Create MustGather CR with `obfuscate.enabled: true`, `obfuscate.source`, and `uploadTarget`
- Verify obfuscation runs and the redacted bundle is uploaded to SFTP
- Download and verify the uploaded bundle contains obfuscated data

**Negative tests**:
- Create MustGather CR with `obfuscate` omitted (default), verify no obfuscation occurs
- Create MustGather CR with `obfuscate.source` set but `obfuscate.enabled: false`, verify rejection
- Create MustGather CR with `obfuscationConfigRef` pointing to a non-existent ConfigMap, verify Job failure with clear error

## Graduation Criteria

### Dev Preview -> Tech Preview

Not applicable. This feature enters directly as Tech Preview.

### Tech Preview -> GA

- `obfuscate` field (with `enabled`, `obfuscationConfigRef`, and `source`) available in the MustGather CRD
- All three modes working end-to-end (gather+obfuscate+upload, obfuscate-only, obfuscate+upload)
- Custom obfuscation config via ConfigMap working end-to-end
- Obfuscation logs included in output bundles
- No upstream patches required for `openshift/must-gather-clean`

- Sufficient user feedback on default obfuscation behavior and custom config UX
- Comprehensive test coverage including upgrade/downgrade scenarios
- Performance benchmarking for representative bundle sizes and large bundles (>1GB)
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)
- Documentation of the must-gather-clean config schema for custom ConfigMap authors

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

- **Upgrade**: Existing MustGather CRs continue to work unchanged. The `obfuscate` field defaults to `nil` (obfuscation disabled), so no behavioral change occurs for existing users.
- **Downgrade**:
  - If only the operator is downgraded (CRD still includes `obfuscate`), the older operator version ignores the field and collection proceeds without obfuscation.
  - If the CRD is also downgraded, creating MustGather CRs with the `obfuscate` field may be rejected by API validation or the field may be pruned. Remove the field before downgrading.
  - No data migration is needed since obfuscation is a stateless per-run operation.
  - Custom config ConfigMaps are unaffected by upgrade/downgrade; they are standard Kubernetes resources.

## Version Skew Strategy

The obfuscation feature is self-contained within the Must-Gather Operator. No coordination with other components is required. The operator binary, CRD, and upload script are versioned together in the same image.

The only external dependency is the `must-gather-clean` library which is vendored into the operator binary, eliminating runtime version skew concerns.

## Operational Aspects of API Extensions

The `obfuscate` field is an optional addition to the existing `mustgathers.operator.openshift.io` CRD. It does not introduce new webhooks, finalizers, aggregated API servers, or additional CRDs.

- **Impact on existing SLIs**: None. The field is optional and only affects newly created MustGather CRs that explicitly enable it.
- **Failure modes**: If obfuscation fails (unreadable files, disk full, invalid custom config), the upload container exits non-zero, the Job fails.
- **Resource consumption**: Obfuscation temporarily increases CPU and I/O usage in the upload container. For a typical 200MB bundle, obfuscation completes in 30-60 seconds using parallel workers.

## Support Procedures

- **Detecting issues**: Check upload container logs for "obfuscation failed" messages. The `obfuscation.log` file in the bundle (if it was created before failure) contains detailed processing information.
- **Disabling**: Omit the `spec.obfuscate` field entirely or set `spec.obfuscate.enabled: false` to skip obfuscation.
- **Consequences of disabling**: Bundles will be uploaded without redaction. No data loss occurs; the full unredacted bundle is uploaded instead.
- **Common failure modes**:
  - `permission denied` errors in obfuscation: Indicates the `chown` step in the gather container did not execute. Verify the gather container command includes the `chown -R 65534:65534` suffix.
  - `klog.Exitf` in upload container logs: Indicates a file-level error in the must-gather-clean library. Check for corrupted or unreadable files in the bundle.

## Infrastructure Needed

No new infrastructure is needed. The enhancement uses existing CI infrastructure and the existing SFTP upload target for testing. The `must-gather-clean` library is vendored as a Go dependency. No upstream patches are required.

## Future Enhancements

1. **CR status phases**: Report obfuscation phase (`Obfuscating`) between `Collecting` and `Uploading`
2. **Obfuscation report in CR status**: Surface replacement count and omission count from `report.yaml`
3. **Gather + obfuscate without upload**: Support `obfuscate.enabled: true` without `uploadTarget` for gather + obfuscate + persist to PVC only
4. **must-gather-clean library improvements**: Propose upstream changes to replace `klog.Exitf` with returned errors.
