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

1. Changes to the must-gather-clean library API (consumed as-is, except for the required `chown` patch)
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
| Obfuscate Only | `obfuscate.enabled: true` + `obfuscate.source` | Redact an existing bundle on a PVC in-place, no gather, no upload |
| Obfuscate + Upload | `obfuscate.enabled: true` + `obfuscate.source` + `uploadTarget` | Redact an existing bundle and upload it, no gather |

### Workflow Description

**Cluster administrator** is a human user responsible for collecting and uploading diagnostic data.

#### Mode 1: Gather + Obfuscate + Upload

This is the primary workflow for new must-gather collections with obfuscation.

1. The cluster administrator creates a MustGather CR with `spec.obfuscate.enabled: true` and an `uploadTarget` configured
2. The Must-Gather Operator creates a Job with gather and upload containers
3. The gather container runs `/usr/bin/gather`, writes output to the shared `/must-gather` volume, and appends `chmod -R a+rwX /must-gather` before exiting (making contents accessible to the non-root upload container)
4. The upload container detects gather completion via `pgrep` polling
5. The upload script checks the `obfuscate` environment variable
6. The operator binary is invoked: `must-gather-operator obfuscate --input /must-gather --config <config-path> -v=3`
7. If `obfuscationConfigRef` is set, the referenced ConfigMap is mounted and used as the config; otherwise the baked-in default config is used
8. The obfuscation engine walks all files, applying the configured replacements and omissions
9. Cleaned output replaces the original bundle via atomic rename
10. An `obfuscation.log` is preserved in the bundle for auditability
11. The upload script proceeds to tar and SFTP upload as normal
12. The MustGather CR status is updated to Completed

#### Mode 2: Obfuscate Only (No Gather, No Upload)

This mode allows administrators to redact an existing must-gather bundle stored on a PVC without re-collecting or uploading.

1. The cluster administrator has a previously collected must-gather bundle persisted on a PVC (e.g., from a prior MustGather CR with `spec.storage`)
2. The administrator creates a MustGather CR with `spec.obfuscate.enabled: true` and `spec.obfuscate.source` referencing the PVC
3. The operator creates a Job with only an upload container (no gather container)
4. The upload container mounts the PVC at `/must-gather`
5. The operator binary is invoked with the appropriate config (custom or default)
6. Obfuscation runs in-place on the PVC contents
7. The `obfuscation.log` is written to the bundle on the PVC
8. The MustGather CR status is updated to Completed
9. The obfuscated bundle remains on the PVC for the administrator to retrieve

#### Mode 3: Obfuscate + Upload (No Gather)

This mode allows administrators to redact an existing bundle and upload it to a support case without re-collecting.

1. The cluster administrator has a previously collected must-gather bundle on a PVC
2. The administrator creates a MustGather CR with `spec.obfuscate.enabled: true`, `spec.obfuscate.source` referencing the PVC, and `spec.uploadTarget` configured
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

**Mode 1: Gather + Obfuscate + Upload (default config)**

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
│  │ chmod -R a+rwX    │        │    atomic rename back │  │
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
- Accepts an input directory path, an optional output directory path (in-place if omitted), and a config file path (defaults to baked-in config)
- Writes an `obfuscation.log` file into the bundle for auditability
- Imports `go.uber.org/automaxprocs` to set `GOMAXPROCS` to the container's CPU limit, then uses `min(runtime.GOMAXPROCS(0), 4)` workers to cap parallelism
- Calls `mgclean.Run()` to perform obfuscation using the capped worker count
- In in-place mode: writes to a temp directory (`/must-gather-upload/.must-gather-cleaned`), then atomically swaps with the original via `os.RemoveAll` + `os.Rename`
- Copies the log file into the cleaned output so it survives the replacement

**3. Job template (`controllers/mustgather/template.go`)**

Changes when `spec.obfuscate.enabled` is true:

*Gather container*: Appends `chmod -R a+rwX /must-gather` to the gather command, making the output volume writable for the non-root upload container. When `spec.obfuscate.source` is set, the gather container is omitted entirely from the Job spec.

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
  /usr/local/bin/must-gather-operator obfuscate --input "$must_gather_output" ${config_flag} -v=3
  echo "Obfuscation complete."
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

Added `must-gather-clean` as a dependency (pinned to upstream release once the `chown` patch is merged).

#### Permission Model

The obfuscation step bridges a permission gap between two containers with different UIDs:

| Container | UID | Permissions |
|-----------|-----|-------------|
| gather | 0 (root) | Creates all files as root-owned in `/must-gather` |
| upload | 65534 (nobody) | Reads input from `/must-gather`, writes cleaned output to `/must-gather-upload` |

Two mechanisms resolve this:

1. **`chmod` by gather container**: When `obfuscate.enabled: true`, the gather container's command is extended with `chmod -R a+rwX /must-gather`. This runs as root before the gather container exits, making the output volume world-readable and world-writable.

2. **`chown` patch in must-gather-clean**: The library attempts `os.Chown` on output files to match input ownership. A patch makes `syscall.EPERM` non-fatal, since uid 65534 cannot `chown` to uid 0. Output files are created with uid 65534 ownership instead, which has zero functional impact for the ephemeral Job pod.

**Why not run the upload container as root?** Running as root would eliminate both permission issues but violates the principle of least privilege, conflicts with OpenShift's restricted SCC, and would require explicit `anyuid` SCC bindings.

#### Default Obfuscation Behavior

| Data Type | Action | Details |
|-----------|--------|---------|
| IPv4 addresses | Consistent replacement | `10.0.1.5` → `x-ipv4-0000001-x` everywhere in the bundle |
| MAC addresses | Consistent replacement | `0e:a0:e7:92:3a:a3` → `x-mac-0000001-x` |
| Kubernetes Secrets | Omitted entirely | Files containing `kind: Secret` are excluded from output |
| Kubernetes ConfigMaps | Omitted entirely | Files containing `kind: ConfigMap` are excluded from output |
| Local IPs (127.0.0.1, 0.0.0.0, ::1) | Preserved | Not obfuscated |

#### Required Upstream Changes to must-gather-clean

The `openshift/must-gather-clean` library requires the following patch before this integration can ship to production. The patch is to `pkg/fsutil/fsutil_unix.go`.

**Problem**: `must-gather-clean` was designed as a standalone CLI tool that typically runs as root or on user-owned files. It preserves input file ownership on output files via `os.Chown`. In the operator context, input files are owned by root (uid 0, from the gather container) but the `chown` is executed by uid 65534 (the upload container). The Linux kernel requires `CAP_CHOWN` to change file ownership to a different user, which non-root processes lack. Without the patch, every file write fails with `EPERM`.

**Required change**: Modify the `chown` function to treat `syscall.EPERM` as non-fatal:

```go
func chown(path string, stat fs.FileInfo) error {
    uid := stat.Sys().(*syscall.Stat_t).Uid
    gid := stat.Sys().(*syscall.Stat_t).Gid
    err := os.Chown(path, int(uid), int(gid))
    if err != nil {
        if errors.Is(err, syscall.EPERM) {
            klog.V(3).Infof("chown '%s' to (%d, %d) skipped: permission denied (running as non-root)", path, uid, gid)
            return nil
        }
        return err
    }
    return nil
}
```

**Impact of skipping `chown`**: Output files are owned by uid 65534 instead of uid 0. This is purely cosmetic -- the files are immediately tarred, SFTP-uploaded, and the pod is deleted. File ownership within the tar archive is irrelevant to support engineers analyzing the bundle contents.

**Path to production**: This patch should be proposed upstream to `openshift/must-gather-clean` as a bug fix for non-root execution. Once merged and released, the `replace` directive in the operator's `go.mod` should be removed and replaced with a pinned version. Until then, a `replace` directive pointing to a fork is used for development:

```
require github.com/openshift/must-gather-clean v0.0.0-00010101000000-000000000000
replace github.com/openshift/must-gather-clean => ../must-gather-clean
```

#### Performance Impact

**Key observations**:

- Obfuscation is I/O-bound, not CPU-bound. The bottleneck is reading and writing files, not regex processing.
- The operator uses `automaxprocs` to detect the container's cgroup CPU limit and sets `GOMAXPROCS` accordingly. Worker count is further capped at 4 to avoid excessive I/O contention, since the workload is I/O-bound rather than CPU-bound.
- In-place mode (used in the operator) requires temporary disk space equal to the bundle size in `/must-gather-upload/.must-gather-cleaned` during processing. After atomic rename, this space is freed.
- The upload step (tar + SFTP) is typically the longest phase of the pipeline. Obfuscation adds a smaller fraction of time relative to network-bound upload.

**Worst-case scenario**: On a single-node deployment with a 1GB+ bundle and slow storage (e.g., network-attached PV), obfuscation could take 5-10 minutes. This is still within acceptable bounds for a batch diagnostic operation.

**Mitigation for large bundles**: Users can combine `obfuscate.enabled: true` with `gatherSpec.since` (time-based filtering from the time-filter enhancement) to reduce bundle size before obfuscation, improving both gather and obfuscation performance.

### Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| `must-gather-clean` calls `klog.Exitf` on file errors, terminating the upload container | Upload fails hard instead of returning a recoverable error | The obfuscation runs in the upload container (not the operator controller), so the operator process itself is unaffected; document that file-level errors are fatal |
| Obfuscation increases Job duration for large bundles | Longer time to upload for support cases | Obfuscation is I/O-bound and parallelized |
| Container CPU limits may be lower than expected | Obfuscation runs slower with fewer workers | `automaxprocs` detects cgroup CPU limits and sets `GOMAXPROCS` accordingly; worker count is additionally capped at 4 to balance throughput and I/O contention |
| The `chown` patch to `must-gather-clean` has not yet been merged upstream | Development uses a `replace` directive in `go.mod` pointing to a local fork, which cannot ship to production | Propose the `chown` EPERM fix upstream as a prerequisite; once merged, pin to the upstream release and remove the `replace` directive |
| Omitted resources (Secrets, ConfigMaps by default) are permanently excluded from the bundle | Support engineers will not have access to omitted resources, potentially limiting their ability to diagnose issues | Document clearly that any resource type configured for omission will be unavailable to support; users can adjust omission rules via `obfuscationConfigRef` |
| `obfuscate.source` in-place mode is destructive | Original unobfuscated bundle on PVC is permanently replaced | Document this clearly; users should back up the PVC if they need the original. Consider non-destructive mode as follow-up |
| Custom must-gather images may produce non-standard output structures | Obfuscation may miss or incorrectly process files from custom images | `must-gather-clean` operates on file content (regex-based), not directory structure, so it is largely format-agnostic. Document that custom image support is best-effort |
| Invalid custom obfuscation ConfigMap causes runtime failure | Obfuscation fails, Job fails, no upload occurs | Operator logs the error clearly; `obfuscation.log` captures the failure reason. Config validation is best-effort at the library level |

### Drawbacks

- Adds a dependency on the `must-gather-clean` library, increasing the operator binary size and vendor footprint
- The `chown` patch requires coordination with an upstream repository before production readiness
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

Would eliminate both the `chmod` and `chown` patch requirements.

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

4. What is the SLA for the upstream `chown` patch to `openshift/must-gather-clean`?

5. For `obfuscate.source` mode, should the operator obfuscate in-place on the source PVC (destructive, replaces original) or write to a separate output PVC (non-destructive, requires additional storage)?

6. Should the operator validate that the referenced `obfuscationConfigRef` ConfigMap exists and contains a `config.yaml` key at CR admission time (via webhook), or only at Job execution time?

## Test Plan

### Unit Tests

- `template_test.go`: Verify `getGatherContainer` appends `chmod -R a+rwX` when `obfuscate.enabled` is true, does not when false
- `template_test.go`: Verify `getUploadContainer` passes `obfuscate` env var when `obfuscate.enabled` is true, does not when false
- `template_test.go`: Verify `getUploadContainer` mounts the ConfigMap and sets `obfuscate_config` env var when `obfuscationConfigRef` is set
- `template_test.go`: Verify Job template omits the gather container when `obfuscate.source` is set
- `template_test.go`: Verify PVC from `obfuscate.source` is mounted correctly in the upload container
- Existing tests updated to include the new `obfuscate` parameter (backward compatible with `nil`)

### Integration Tests

- Verify `runObfuscate()` correctly processes a test bundle directory
- Verify in-place mode atomically replaces the original directory
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
- Verify the PVC contents are obfuscated in-place
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
- Upstream `chown` patch merged to `openshift/must-gather-clean`
- `replace` directive removed from `go.mod` (pinned to upstream release)
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
  - `permission denied` errors in obfuscation: Indicates the `chmod` step in the gather container did not execute. Verify the gather container command includes the chmod suffix.
  - `klog.Exitf` in upload container logs: Indicates a file-level error in the must-gather-clean library. Check for corrupted or unreadable files in the bundle.

## Infrastructure Needed

No new infrastructure is needed. The enhancement uses existing CI infrastructure and the existing SFTP upload target for testing. The `must-gather-clean` library is vendored as a Go dependency.

A patch to the upstream `openshift/must-gather-clean` repository is required for the `chown` EPERM handling. This should be proposed as a bug fix PR to that repository.

## Future Enhancements

1. **CR status phases**: Report obfuscation phase (`Obfuscating`) between `Collecting` and `Uploading`
2. **Obfuscation report in CR status**: Surface replacement count and omission count from `report.yaml`
3. **Gather + obfuscate without upload**: Support `obfuscate.enabled: true` without `uploadTarget` for gather + obfuscate + persist to PVC only
4. **must-gather-clean library improvements**: Propose upstream changes to replace `klog.Exitf` with returned errors.
