---
title: microshift-kubelet-image-credential-provider
authors:
  - "@Neilhamza"
reviewers:
  - "@pacevedom"
  - "@eslutsky"
  - "@copejon"
  - "@ggiguash"
  - "@pmtk"
  - "@kasturinarra"
  - "@qJkee"
  - "@dfroehli"
approvers:
  - "@jerpeter1"
api-approvers:
  - None
creation-date: 2026-09-02
last-updated: 2026-09-03
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-3456
see-also:
  - enhancements/microshift/enabling-user-specified-featuregates.md
  - enhancements/microshift/microshift-dns-resource-configuration.md
---

# MicroShift Kubelet Image Credential Provider Configuration

## Summary

MicroShift runs kubelet as an embedded, in-process component. Kubelet's [image
credential
provider](https://kubernetes.io/docs/tasks/administer-cluster/kubelet-credential-provider/)
feature (GA since Kubernetes 1.26) allows kubelet to obtain container registry
credentials dynamically at image pull time by executing an external provider
binary. The feature is enabled through two kubelet command-line flags,
`--image-credential-provider-config` and `--image-credential-provider-bin-dir`,
which have no equivalent in `KubeletConfiguration`. Because MicroShift exposes
no kubelet command line and only maps its `kubelet:` configuration section into
`KubeletConfiguration`, users currently have no way to enable this feature.

This enhancement introduces two new optional keys,
`imageCredentialProviderConfigPath` and `imageCredentialProviderBinDir`, under
the existing `kubelet:` section of the MicroShift configuration file. MicroShift
reads these keys, validates them at startup, and applies them to the embedded
kubelet's startup flags.

## Motivation

Edge devices frequently pull application images from private, token-based
registries such as Amazon ECR, Google Artifact Registry, or Azure Container
Registry. These registries do not issue long-lived passwords; Amazon ECR, for
example, issues authorization tokens that expire after 12 hours.

Today, MicroShift users work around this with a systemd timer that periodically
obtains a fresh token (e.g. `aws ecr get-login-password`) and rewrites it into
CRI-O's static auth file. This workaround has known drawbacks:

- **Token gap:** if the timer fires late, or the token expires early, image
  pulls fail until the next refresh.
- **Operational overhead:** an additional systemd unit and script must be
  maintained on every device.
- **Not portable:** each registry type (ECR, GCR, ACR) requires its own bespoke
  script.

The kubelet image credential provider is the upstream-standard,
cloud-vendor-recommended solution to this problem. A customer (see the linked
RFE) requires it to pull private images from Amazon ECR on MicroShift edge
devices.

### User Stories

1. As a MicroShift administrator, I want to enable the kubelet image credential
   provider through the MicroShift configuration file so that my edge devices
   can authenticate to private registries at pull time without direct access to
   kubelet command-line flags.
2. As a MicroShift administrator with devices pulling from Amazon ECR, I want
   kubelet to obtain short-lived registry credentials automatically so that I no
   longer maintain a per-device token rotation script.
3. As a MicroShift administrator, I want MicroShift to fail to start with a
   clear error if the credential provider configuration points to paths that do
   not exist or are unsafe, so that misconfigurations are caught at startup
   rather than surfacing as opaque image pull failures.

### Goals

1. Allow users to set `imageCredentialProviderConfigPath` and
   `imageCredentialProviderBinDir` under the `kubelet:` section of the
   MicroShift configuration file.
2. Apply these settings to the embedded kubelet as startup flags when present.
3. Preserve current behavior when the keys are not set, ensuring full backward
   compatibility.
4. Validate the provided paths at startup and fail with a clear error message if
   they are invalid or unsafe.

### Non-Goals

1. A generic mechanism for passing arbitrary kubelet command-line flags through
   the MicroShift configuration file. This enhancement special-cases exactly two
   keys.
2. Packaging or distributing credential provider binaries (e.g.
   `ecr-credential-provider`). These are standalone upstream binaries that users
   install separately, and which binary is needed depends on the registry in
   use.
3. Validating the contents of the credential provider configuration file, or the
   presence and executability of the binaries it names. Kubelet validates the
   configuration file and the presence of each configured binary when it
   registers providers. MicroShift validates ownership and permissions of both
   paths and their contents, but not their semantics.
4. Runtime reconfiguration of the two keys without restarting MicroShift.
5. Protection against a user who already holds root privileges. The validation
   described here defends against unprivileged local users only.

## Proposal

Introduce two new optional keys under the existing `kubelet:` section of the
MicroShift configuration file:

```yaml
kubelet:
  imageCredentialProviderConfigPath: /etc/microshift/credential-providers.yaml
  imageCredentialProviderBinDir: /usr/libexec/microshift/credential-providers
```

This follows the same pattern as user-provided certificates, the hosts file, and
kustomize manifests: MicroShift consumes user-supplied content at configured
paths and does not provide the content itself. The provider binary and its
configuration file are supplied by the user because they depend on which
registry is in use.

The `kubelet:` section is currently a schemaless map (`map[string]any`) whose
contents are marshaled verbatim into the generated `KubeletConfiguration` file.
The two new keys are kubelet **flags**, not `KubeletConfiguration` fields, so
they cannot follow that path: if left in the map, kubelet's strict decoder
rejects them, falls back to a lenient decoder, logs a warning, and silently
ignores them. MicroShift will therefore read these two keys from the map during
configuration processing into typed internal fields, validate them, and set them
on the `KubeletFlags` struct used to start the embedded kubelet. When the
remaining `kubelet:` contents are marshaled into `KubeletConfiguration`, the two
reserved keys are filtered out. The `Config.Kubelet` map itself is not modified,
so `microshift show-config` continues to display the keys exactly where the user
set them. All other keys in the `kubelet:` section continue to be passed through
to `KubeletConfiguration` unchanged.

### Workflow Description

1. MicroShift starts up and loads its configuration file and drop-ins. The
   `kubelet:` section is read into `Config.Kubelet` as a schemaless map.
2. During computed-value processing, MicroShift reads
   `imageCredentialProviderConfigPath` and `imageCredentialProviderBinDir` from
   the map into typed internal fields. The map is not modified. An empty string
   or explicit `null` is treated as unset.
3. During validation, MicroShift checks that either both keys are set or neither
   is, that both paths are absolute, that `imageCredentialProviderConfigPath`
   resolves to a regular file or directory and `imageCredentialProviderBinDir`
   resolves to a directory, and that both paths satisfy the trusted-path rule
   described under Config validation: after resolving symlinks, every path
   component, the final object, and every file inside a directory must be owned
   by root and not writable by group or others. The canonical (symlink-resolved)
   paths are stored in the typed fields.
4. If validation fails, MicroShift exits with a descriptive error message naming
   the field and the path.
5. If validation succeeds, the kubelet component sets
   `KubeletFlags.ImageCredentialProviderConfigPath` and
   `KubeletFlags.ImageCredentialProviderBinDir` to the canonical paths before
   starting the embedded kubelet, and logs an informational message recording
   both the configured and the canonical values.
6. The keys in the `kubelet:` section other than the two reserved keys are
   marshaled into the `KubeletConfiguration` file as today.
7. Kubelet reads the provider configuration file and verifies that each
   configured provider binary exists in the bin directory. Failures here are
   kubelet startup errors, reported in the MicroShift journal.
8. At image pull time, kubelet consults the credential provider configuration;
   for images matching a configured provider, kubelet executes the provider
   binary, caches the returned credentials in memory for the returned or default
   cache duration, and passes them to CRI-O with the pull request. CRI-O uses
   credentials supplied by kubelet in preference to its own auth files; images
   that match no provider use CRI-O's auth files as today.
9. If the keys are not set, kubelet starts without credential provider
   configuration, identical to current behavior.

### API Extensions

The following changes to the MicroShift configuration file are proposed:

```yaml
kubelet:
  imageCredentialProviderConfigPath: <string>  # optional, default: not set
  imageCredentialProviderBinDir: <string>      # optional, default: not set
```

- `imageCredentialProviderConfigPath`: absolute path to a kubelet
  `CredentialProviderConfig` file (JSON or YAML), or a directory of such files
  which kubelet merges in lexicographical order.
- `imageCredentialProviderBinDir`: absolute path to the directory containing
  credential provider plugin binaries.

Both keys must be set together; setting only one is a validation error. Both
paths, every component leading to them, and every file inside a directory must
be owned by root and not writable by group or others (see Config validation). An
empty string or explicit `null` is equivalent to omitting the key. Because user
configuration files and drop-ins under `/etc/microshift/config.d/` are combined
with a JSON merge patch, which merges maps key by key, the two keys may be set
in different files (for example, one in `config.yaml` and the other in a
drop-in) and are merged before validation. Treating `null` as unset is
consistent with merge-patch semantics, where `null` removes a key.

The `kubelet:` section is annotated `+kubebuilder:validation:Schemaless`, so no
change to the generated configuration schema or a schema version bump is
required. As a consequence, the configuration generator cannot emit the keys as
schema entries; they are documented through the doc comment on the `Kubelet`
field, which the generator propagates into the sample configuration file
(`packaging/microshift/config.yaml`) and the OpenAPI description (see Sample
configuration and documentation below).

`microshift show-config --mode effective` displays the keys under `kubelet:` as
set by the user, since the underlying map is not modified. Kubelet receives the
canonical, symlink-resolved form of each path (see Config validation).

The credential provider configuration file itself follows the upstream
`kubelet.config.k8s.io/v1` `CredentialProviderConfig` format. For example, for
Amazon ECR:

```yaml
apiVersion: kubelet.config.k8s.io/v1
kind: CredentialProviderConfig
providers:
  - name: ecr-credential-provider
    matchImages:
      - "*.dkr.ecr.*.amazonaws.com"
    defaultCacheDuration: "6h"
    apiVersion: credentialprovider.kubelet.k8s.io/v1
```

### Topology Considerations

#### Hypershift / Hosted Control Planes
N/A

#### Standalone Clusters
N/A

#### Single-node Deployments or MicroShift
Enhancement is intended for MicroShift only.

#### OpenShift Kubernetes Engine
N/A

### Implementation Details/Notes/Constraints

#### Config struct changes

Add two internal, non-serialized fields to the `Config` struct in
`pkg/config/config.go`, following the existing convention for internal fields
(e.g. `userSettings`). The `Kubelet` field's doc comment is extended to describe
the two keys, because the configuration generator propagates it into the sample
configuration file and the OpenAPI description:

```go
    // Settings specified in this section are transferred as-is into the
    // Kubelet config, with two exceptions that are applied as kubelet startup
    // flags instead:
    //   imageCredentialProviderConfigPath: absolute path to a kubelet
    //     CredentialProviderConfig file, or a directory of such files. Enables
    //     the kubelet image credential provider.
    //   imageCredentialProviderBinDir: absolute path to the directory
    //     containing credential provider plugin binaries. Must be set together
    //     with imageCredentialProviderConfigPath.
    // Both paths, their parent directories, and the files they contain must be
    // owned by root and not writable by group or others.
    // +kubebuilder:validation:Schemaless
    Kubelet map[string]any `json:"kubelet"`

    // Read from the Kubelet map during updateComputedValues().
    // These are kubelet flags, not KubeletConfiguration fields.
    KubeletImageCredentialProviderConfigPath string `json:"-"`
    KubeletImageCredentialProviderBinDir     string `json:"-"`
```

Populating internal fields from the schemaless `Kubelet` map is a new pattern in
the MicroShift configuration code. The map has always been passed through
untouched, and this enhancement preserves that: the map is read, not modified.

#### Config extraction

`updateComputedValues()` in `pkg/config/config.go` reads the two reserved keys
into the typed fields. A present key whose value is a non-string (other than
`null`) is an error. A helper returns the passthrough view of the map for
kubelet:

```go
// KubeletPassthrough returns the user-provided kubelet settings that are
// transferred into KubeletConfiguration, excluding the reserved keys.
func (c *Config) KubeletPassthrough() map[string]any
```

`generateConfig()` in `pkg/node/kubelet.go` marshals `cfg.KubeletPassthrough()`
instead of `cfg.Kubelet`, so the reserved keys never reach the
`KubeletConfiguration` file. The list of reserved keys lives only in the
`config` package.

#### Config validation

`validate()` in `pkg/config/config.go` is extended with the following rules, in
order:

- Neither key set: valid, feature inactive.
- Exactly one key set: error. Kubelet requires both to activate the feature;
  failing early in MicroShift produces a clearer message than kubelet's own
  error.
- Either path not absolute: error. A relative path would be resolved against the
  MicroShift process working directory, which is not a stable location.
- `imageCredentialProviderConfigPath` must resolve to a regular file or a
  directory. Other file types (devices, sockets, FIFOs) are rejected. A
  directory is accepted to match upstream kubelet semantics.
- `imageCredentialProviderBinDir` must resolve to a directory.
- Both paths must satisfy the trusted-path rule below.

**Trusted-path rule.** Both the configuration file and the binaries are inputs
to a process that kubelet executes with root privileges: the bin directory
supplies the executable, and the configuration file supplies its name,
arguments, and environment. An unprivileged local user who can modify either can
therefore obtain root execution during a matching image pull. MicroShift applies
one rule to both paths, implemented once as a helper in the `config` package:

1. Resolve symlinks to obtain the canonical path (`filepath.EvalSymlinks`).
   Symlinks are not rejected, because image-based systems rely on them for
   standard locations (for example `/usr/local` and `/home` on ostree and
   bootc); the checks are applied to what the path actually refers to.
2. Every component of the canonical path, from `/` down to and including the
   final object, must be owned by root (uid 0) and must not have group or other
   write permission. Checking ancestors prevents an unprivileged user who owns a
   parent directory from replacing the validated file or directory after
   startup.
3. If the final object is a directory, every entry in it must satisfy the same
   ownership and permission requirement. A symlinked entry is resolved and the
   full rule, including ancestors, is applied to its target, so an entry cannot
   point into a location an unprivileged user controls.
4. The canonical path, not the configured path, is what MicroShift passes to
   kubelet. Validating the canonical path while handing kubelet the configured
   path would leave a gap: a symlink under a user-writable directory could be
   re-pointed after startup, and kubelet would follow it at invocation time.
   Passing the canonical path removes the symlink from kubelet's view entirely;
   every component kubelet then traverses has been verified. The components of
   the configured path itself therefore do not need to be trusted. `validate()`
   stores the canonical paths in the typed fields; `Config.Kubelet` retains the
   user's values for `show-config`.

Together these checks mean an unprivileged user can neither add, replace, nor
modify anything on either path, nor redirect kubelet to a different path. Only
root can change the ownership or permissions of root-owned objects, so the
properties verified at startup hold until a privileged actor changes them, which
is outside the threat model. Standard layouts (`/etc/microshift/...`,
`/usr/libexec/...`, and their ostree and bootc equivalents) satisfy the rule as
installed; paths under user home directories or `/tmp` do not.

Error messages name the field, the configured path, and the offending component,
e.g. `error validating kubelet.imageCredentialProviderBinDir ("/opt/providers"):
"/opt/providers/ecr-credential-provider" must be owned by root and not writable
by group or others`.

MicroShift deliberately does **not** validate the contents of the credential
provider configuration file, nor the presence or executability of the binaries
it names. Kubelet validates the configuration file and checks that each
configured provider binary exists (`exec.LookPath`) when it registers providers;
failures surface as kubelet startup errors in the journal. Duplicating that
validation would require keeping MicroShift in sync with upstream kubelet
behavior. Checking ownership and permissions of every entry in the bin
directory, rather than only the configured providers, avoids parsing the
configuration file at all.

#### Kubelet flag injection

`configure()` in `pkg/node/kubelet.go` sets the two fields on `KubeletFlags`.
The `KubeletFlags` struct from the vendored
`k8s.io/kubernetes/cmd/kubelet/app/options` package already exposes them, and
the existing in-process startup path passes them through to `NewMainKubelet()`;
no new plumbing is required:

```go
    if cfg.KubeletImageCredentialProviderConfigPath != "" {
        kubeletFlags.ImageCredentialProviderConfigPath =
            cfg.KubeletImageCredentialProviderConfigPath
        kubeletFlags.ImageCredentialProviderBinDir =
            cfg.KubeletImageCredentialProviderBinDir
        klog.InfoS("Kubelet image credential provider configured",
            "configPath", cfg.KubeletImageCredentialProviderConfigPath,
            "binDir", cfg.KubeletImageCredentialProviderBinDir)
    }
```

The typed fields hold the canonical paths produced by validation, so kubelet
receives symlink-resolved paths; the log line records those canonical paths. The
values the user configured remain available from `microshift show-config`. It is emitted
before kubelet registers providers, so it does not by itself indicate that the
providers are usable; a missing or non-executable provider binary is reported by
kubelet as a startup error after this line. Because MicroShift calls
`kubelet.Run()` directly rather than through kubelet's cobra command, kubelet's
usual `FLAG: --image-credential-provider-config=...` startup lines are never
emitted, and kubelet's own credential provider code logs nothing at default
verbosity on successful registration. The MicroShift log line is therefore the
authoritative signal that the keys were applied.

#### Sample configuration and documentation

`packaging/microshift/config.yaml`, `docs/user/howto_config.md`, and the OpenAPI
description in `cmd/generate-config/config/config-openapi-spec.json` are generated
by `scripts/generate-config.sh` and kept in sync by
`scripts/verify/verify-config.sh`; they must not be edited by hand. Because the
`kubelet:` section is schemaless, the generator cannot emit the two keys as
schema entries. They are documented by extending the doc comment on the
`Kubelet` field in `pkg/config/config.go` (shown above). That comment is rendered
into the sample configuration file (`config.yaml`) and the OpenAPI description;
`howto_config.md` renders the sample configuration without comments, so the keys
are not separately described there. End-user documentation is tracked in OSDOCS
(see Documentation requirements). After changing the comment, run
`make generate-config` and commit the regenerated files.

#### Image-based deployments (bootc and rpm-ostree)

The feature behaves identically across RPM, rpm-ostree, and bootc installations;
the configuration and kubelet startup code paths do not branch on install type.
What differs is how the user-supplied files reach the device. On image-based
systems `/usr` is read-only at runtime, so the credential provider binary must
be included in the OS image, while `/etc` content (the MicroShift configuration
or a `/etc/microshift/config.d/` drop-in, and the provider configuration file)
may be included in the image or written after deployment.

- On bootc (image mode, the RHEL 10 edge deployment model, built with
  bootc-image-builder or a Containerfile), the user adds the binary in their
  Containerfile, e.g. `COPY --chmod=755 ecr-credential-provider
  /usr/libexec/microshift/credential-providers/`. Because `/usr` is replaced on
  every image update while `/etc` persists, every subsequent image must also
  contain the binary; otherwise startup validation fails on the new image and
  greenboot rolls the device back to the previous one.
- On rpm-ostree (the RHEL 9 edge deployment model, built with Image Builder
  blueprints), blueprint file customizations cannot place content under `/usr`
  and cannot carry binary content, so the binary must be delivered as an RPM
  added to the blueprint. Packaging the provider binary is out of scope for
  MicroShift (see Non-Goals); users of rpm-ostree deployments must build or
  obtain such an RPM themselves.

Files placed by either method are root-owned with standard permissions and
satisfy the trusted-path rule as installed.

MicroShift component images that are embedded in an OS image are never pulled
from a registry and do not involve the credential provider. The feature applies
to images that are pulled at runtime, which is the normal case for application
images on edge devices.

#### Documentation requirements

The following must be covered by the end-user documentation, tracked in
OSDOCS-20657 (MicroShift-side input in OCPEDGE-2976):

1. Configuration guide for Amazon ECR: obtaining the upstream
   `ecr-credential-provider` binary, placement, the `CredentialProviderConfig`
   format with a `matchImages` example, the device's AWS identity requirement,
   the two MicroShift keys and their semantics, verification steps, and a note
   that GCR and ACR work the same way with their own provider binaries. Include
   that changing the keys or the provider config file requires a MicroShift
   restart, and that keys should be added after upgrading.
2. Trust boundary: provider binaries run with kubelet's privileges and the
   config file controls their arguments and environment; both paths, their
   parent directories, and their contents must be root-owned and not writable
   by group or others, or MicroShift refuses to start. List locations that
   satisfy this as installed.
3. Image-based deployments: the credential provider binary must be present in
   every OS image build (on rpm-ostree, delivered as the user's own RPM);
   a missing binary on a new image fails validation and triggers greenboot
   rollback — check the failed boot's journal. The bin directory must sit at a
   location that carries the `bin_t` SELinux label (`/usr/libexec` or
   `/usr/local/bin`); the confined kubelet (`kubelet_t`) may only execute
   `bin_t` files. A bin directory under `/etc/microshift` (`kubernetes_file_t`)
   or `/opt` (`usr_t`) passes MicroShift's path validation but is denied
   execution under SELinux enforcing — the provider never runs and the only
   trace is an AVC denial (`ausearch -m AVC -ts recent`). MicroShift does not
   validate SELinux labels, so RPM packaging must place the binary accordingly.
4. `defaultCacheDuration` guidance for providers that do not return their own
   cache duration: keep it comfortably shorter than the registry token lifetime.
   The ECR provider returns its own `cacheDuration` (6 hours) and kubelet honors
   it over `defaultCacheDuration`, so for ECR the field has no effect (6-hour
   cache against a 12-hour token).

### Risks and Mitigations

**Risk:** A user places the keys under `kubelet:` in a MicroShift version that
supports the feature, but a typo in the key name (e.g.
`imageCredentialProviderConfig`) causes the key to fall through to the
`KubeletConfiguration` passthrough, where kubelet's lenient decoder silently
ignores it. The feature appears enabled but is inert.
**Mitigation:** Kubelet logs a lenient-decode warning in the journal. A future
improvement could warn on near-miss keys. This risk is not specific to this
enhancement; it applies to any misspelled key in the passthrough.

**Risk:** Kubelet's lenient decoding of unknown `KubeletConfiguration` fields is
marked upstream as temporary, to be removed when `v1beta1` support is dropped.
When that happens, any unknown key left in the passthrough becomes a hard
kubelet startup failure.
**Mitigation:** This enhancement filters its two keys out of the passthrough
before the map is marshaled, so it never depends on lenient decoding. The
general passthrough exposure predates this enhancement.

**Risk:** Credential provider binaries execute with kubelet's (root) privileges,
and the provider configuration file controls their name, arguments, and
environment. An unprivileged local user who can add, replace, or modify a
binary, modify the configuration file, or replace a parent directory of either
path could obtain root execution during a matching image pull.
**Mitigation:** MicroShift refuses to start unless both paths satisfy the
trusted-path rule: after symlink resolution, every path component, the final
object, and every contained file must be root-owned and not writable by group or
others (see Config validation). This closes all three vectors for unprivileged
users. Documentation states the trust boundary and recommends locations that
satisfy it as installed.

**Risk:** The trusted-path checks run at startup. Ownership or permissions could
be changed afterwards, or a symlink in the configured path could be re-pointed.
**Mitigation:** Only root can change the ownership or permissions of root-owned
objects, and root is outside the threat model (see Non-Goals). Kubelet receives
the canonical path, so re-pointing a symlink in the configured path after
startup has no effect. The checks run again at every restart.

**Risk:** On image-based (ostree/bootc) systems, a validation failure at startup
causes greenboot health checks to fail, triggering an automatic rollback to the
previous deployment.
**Mitigation:** This is the intended composition of fail-fast validation with
greenboot: the device returns to a known-good state rather than remaining
unhealthy. Documentation notes that the provider config file and bin directory
must exist in the image before the configuration keys are enabled.

**Risk:** On image-based systems, a user rebuilds the OS image without the
provider binary while the configuration keys persist in `/etc`. The new image
fails startup validation and is rolled back by greenboot, which may not be
immediately attributed to the missing binary.
**Mitigation:** Documentation for image-based deployments states that the binary
must be present in every image build. The validation error names the bin
directory path, and the failure is visible in the journal of the failed boot.

**Risk:** For providers that do not return their own cache duration, users may
set `defaultCacheDuration` in the provider configuration longer than the registry
token lifetime, causing kubelet to serve expired credentials.
**Mitigation:** Documentation recommends a cache duration comfortably shorter than
the token lifetime. Note that the ECR provider returns its own `cacheDuration`
(6 hours), which kubelet honors over `defaultCacheDuration`, so for ECR the field
has no effect (6-hour cache against a 12-hour token).

### Drawbacks

Special-casing two keys inside an otherwise schemaless passthrough section
introduces a small amount of hidden behavior: two keys under `kubelet:` are
treated differently from all others, and the generated schema cannot express
them. This is accepted because the OCPSTRAT acceptance criteria specify
placement under `kubelet:`, and the alternative of a generic flag passthrough
was explicitly ruled out of scope.

The trusted-path rule goes beyond what upstream kubelet enforces and rejects
some unusual but deliberate layouts, such as a bin directory under a non-root
user's home directory. This is accepted because no legitimate layout for
root-executed plugins should be modifiable by other users, and standard system
locations pass as installed.

## Test Plan

### Unit Tests

- Extraction: both keys present, both absent, only one present.
- Extraction: non-string values rejected with an error; explicit `null` treated
  as unset.
- Extraction: `c.Kubelet` is not modified; `KubeletPassthrough()` returns the
  map without the two reserved keys and with all other keys intact.
- Extraction: empty string values are treated as unset.
- Validation: only one key set fails with the both-or-neither error.
- Validation: relative paths are rejected.
- Validation: `imageCredentialProviderConfigPath` missing fails; existing
  regular file passes; existing directory passes; a non-regular path (e.g. a
  FIFO) fails.
- Validation: `imageCredentialProviderBinDir` missing fails; path is a file
  fails; existing directory passes.
- Validation, trusted path (each case for both keys): final object not owned by
  root fails; group-writable fails; world-writable fails; root-owned `0755`
  directory or `0644` file passes.
- Validation, trusted path, ancestors: a group- or world-writable ancestor
  directory fails; a non-root-owned ancestor fails; the error names the
  offending component.
- Validation, trusted path, contents: a writable or non-root-owned file inside
  the bin directory fails; a writable or non-root-owned file inside a
  configuration directory fails; a directory whose entries all pass succeeds.
- Validation, trusted path, symlinks: a symlink whose resolved target satisfies
  the rule passes (ostree-style layout) and the typed field holds the canonical
  path; a symlink to a writable or non-root-owned target fails; a symlinked
  entry inside the bin directory is checked at its target including the target's
  ancestors.
- Validation: neither key set passes (backward compatibility).
- `generateConfig()`: the two keys never appear in the generated
  `KubeletConfiguration` YAML; other user-provided keys still do.
- `configure()`: `KubeletFlags` carries both canonical values when set, and
  empty values when unset.

Unit tests that require non-root ownership use fake ownership through an
injected `stat` function, since tests do not run as root and cannot `chown`.

### Integration Tests (Robot Framework)

- Default behavior: no keys set, MicroShift starts, the "Kubelet image
  credential provider configured" log line is absent.
- Valid configuration: both keys set to an existing file and directory via
  drop-in, restart, verify MicroShift starts and the journal contains the
  "Kubelet image credential provider configured" log line with the configured
  paths.
- `show-config`: verify `microshift show-config --mode effective` displays both
  keys under `kubelet:`.
- Split configuration: one key in `config.yaml` and the other in a
  `/etc/microshift/config.d/` drop-in, verify the merged configuration is valid
  and MicroShift starts.
- Empty strings: both keys set to `""`, verify behavior is identical to omitting
  them.
- Relative path: verify MicroShift fails to start with an error naming the
  field.
- Directory config path: `imageCredentialProviderConfigPath` set to a directory,
  verify MicroShift starts.
- Missing config path: verify MicroShift fails to start with an error naming the
  field and path in the journal.
- Missing bin directory: verify MicroShift fails to start with an error naming
  the field and path in the journal.
- Unsafe bin directory: bin directory world-writable, verify MicroShift fails to
  start with an error naming the field and path; restore `0755`, verify
  MicroShift starts.
- Unsafe provider binary: bin directory `0755` but a binary inside it
  world-writable, verify MicroShift fails to start with an error naming the
  binary; restore `0755`, verify MicroShift starts.
- Unsafe configuration file: configuration file group-writable, verify
  MicroShift fails to start with an error naming the file; restore `0644`,
  verify MicroShift starts.
- Unsafe ancestor: bin directory placed under a world-writable parent, verify
  MicroShift fails to start with an error naming the parent.
- Symlink re-pointing: configure the bin directory through a symlink located in
  a user-writable directory whose target is a compliant root-owned directory;
  verify MicroShift starts and the `configured` line shows the canonical path;
  as the unprivileged user, re-point the symlink to a directory containing a
  different mock provider; verify the next uncached pull still invokes the
  original provider.
- Only one key set: verify MicroShift fails to start with the both-or-neither
  error.
- Missing provider binary: valid keys, provider configuration naming a binary
  that is not present in the bin directory, verify the "configured" log line is
  present and kubelet reports a startup error for the missing plugin binary in
  the journal.
- Recovery: restore valid configuration, restart, verify MicroShift starts.
- Upgrade with pre-staged keys: on the previous minor version (Y-1) with the
  keys present in the configuration, verify MicroShift starts with a kubelet
  lenient-decode warning; upgrade to the version with the feature, verify the
  feature is active without further configuration changes.
- Rollback: on an image-based system, configuration with the new keys present in
  `/etc`, roll back to a deployment without the feature, verify MicroShift
  starts with a kubelet lenient-decode warning and the feature inert; roll
  forward, verify the feature is active.
- End-to-end pull: deploy a mock credential provider (an executable implementing
  the `credentialprovider.kubelet.k8s.io/v1` exec contract returning static
  credentials) and a local password-protected registry, deploy a pod referencing
  a private image with no CRI-O auth pre-populated, verify the pod reaches
  Running.
- End-to-end negative: mock provider returns incorrect credentials, verify the
  pod reaches ImagePullBackOff.
- Coexistence with CRI-O auth: with the credential provider configured and CRI-O
  auth also pre-populated for the same registry, verify the pull succeeds.
- Mixed registries: pods referencing an image from a credential-provider
  registry and an image from a registry using CRI-O static auth, verify both
  pulls succeed independently.
- Caching: with a short `defaultCacheDuration`, verify a second pull within the
  cache window does not re-invoke the provider and a pull after expiry does.
- Binary replacement without restart: as root, replace the mock provider binary
  in place, verify the next uncached invocation runs the new binary without
  restarting MicroShift.
- bootc: a bootc test image layer that copies the mock provider into
  `/usr/libexec/microshift/credential-providers/` and the configuration drop-in
  and provider configuration into `/etc/microshift/` at image build time,
  following the existing `test/image-blueprints-bootc` layering; boot the image
  and run the end-to-end pull scenarios above. This exercises the read-only
  `/usr` placement that distinguishes image-based deployments from RPM installs,
  and verifies that image-installed files satisfy the trusted-path rule without
  manual permission changes.

A one-time manual validation against Amazon ECR with the upstream
`ecr-credential-provider` binary is performed before release to confirm the
customer's exact path and to source the configuration guide. Automated CI
coverage uses the mock provider and local registry only.

## Graduation Criteria

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA

- Ability to utilize the enhancement end to end
- End user documentation completed and published, covering all items in
  Documentation requirements
- Available by default
- End-to-end tests
- Unit tests covering config extraction and validation

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy

When upgrading from a version without support for these keys, the keys remain
unset unless the user adds them, and existing behavior is preserved. The
documented procedure is to add the keys after upgrading to a version that
supports them. No migration is required; the feature holds no persisted state.

Pre-staging the keys before upgrading is tolerated but not the documented
procedure. A pre-feature version passes the keys through to the
`KubeletConfiguration` file, where kubelet's strict decoder rejects them and the
lenient `v1beta1` fallback logs a warning and ignores them; the newer version
then activates the keys on its first start. The lenient fallback is present in
the kubelet codec of every Kubernetes version vendored by a GA MicroShift
release (verified in the 1.25, 1.28, and 1.31 release branches; MicroShift 4.12
vendors 1.25), and released binaries do not change. The upstream plan to remove
the fallback affects only future kubelet versions, which ship in MicroShift
versions that include this feature and never pass the keys through. This
behavior is tested only for the supported Y-1 to feature-version upgrade; users
should add the keys after upgrading unless they have verified pre-staging on
their specific source version.

MicroShift does not support downgrades. The supported scenario that boots an
older MicroShift with a newer configuration is rollback to the previous
deployment on image-based systems (rpm-ostree and bootc), either manually or
automatically through greenboot. Because `/etc` persists across deployments
while `/usr` does not, the rolled-back deployment sees the configuration keys
but not necessarily the provider binary. In that scenario the older MicroShift
passes the keys through to the `KubeletConfiguration` file, where kubelet's
lenient decoder logs a warning and ignores them. MicroShift starts normally with
the credential provider feature inactive, so the rollback itself succeeds.
Private registry image pulls fail on the rolled-back deployment until the
previous workaround is restored or the device is rolled forward again. The
configuration file on disk is not modified. A rollback only ever targets the
immediately previous deployment, which is the transition covered by the rollback
test.

## Version Skew Strategy
N/A

## Operational Aspects of API Extensions

The credential provider is invoked by kubelet only for images matching a
configured `matchImages` pattern; pulls from other registries are unaffected.
Credentials are held in kubelet's memory only, never written to disk, and are
discarded when MicroShift restarts; the first matching pull after a restart
re-invokes the provider.

The provider binary must be able to authenticate to the cloud provider itself
(for Amazon ECR: an EC2 instance role, IAM Roles Anywhere, or static credentials
in `/root/.aws/credentials`). Failures in the provider surface as image pull
failures (`ImagePullBackOff`) with the provider's error in the kubelet journal.

Changes to the two configuration keys or to the provider configuration file
require a MicroShift restart: kubelet reads the provider configuration once
during initialization. Replacing an existing provider binary in place does not
require a restart, because kubelet resolves the binary path at each invocation;
the replacement takes effect on the next invocation that is not served from the
credential cache. Adding a new provider entry to the configuration file requires
a restart. Only root can perform any of these changes on a correctly configured
system.

See Image-based deployments for the placement requirements on bootc and
rpm-ostree.

## Support Procedures

- Confirm the keys were applied: `journalctl -u microshift` at startup contains
  `Kubelet image credential provider configured` with the canonical
  (symlink-resolved) `configPath` and `binDir`. Kubelet does not log `FLAG:` lines in MicroShift
  and logs nothing at default verbosity on successful provider registration, so
  this MicroShift log line is the authoritative signal that the keys reached
  kubelet. It does not confirm that the provider binaries are usable.
- Confirm the providers registered: the absence of a kubelet startup error
  following the "configured" line. A missing or non-executable provider binary
  is reported by kubelet as a startup error naming the plugin binary path.
- Confirm the effective configuration: `microshift show-config --mode effective`
  displays both keys under `kubelet:`.
- Startup validation failures are logged with the field name, the configured
  path, and where applicable the offending component, e.g.
  `error validating kubelet.imageCredentialProviderConfigPath ("/etc/microshift/credential-providers.yaml"): file or directory does not exist`
  or
  `error validating kubelet.imageCredentialProviderBinDir ("/opt/providers"): "/opt/providers/ecr-credential-provider" must be owned by root and not writable by group or others`.
  Fix with `chown root:root` and `chmod go-w` on the named component.
- Image pull failures with the feature active: inspect `journalctl -u
  microshift` for provider execution errors, verify the provider binary runs
  successfully when invoked manually with a `CredentialProviderRequest` on
  stdin, and verify the device's cloud identity has registry pull permissions.
- Image pull failures with no provider execution error in the journal: check for
  an SELinux denial with `ausearch -m AVC -ts recent`. MicroShift runs confined
  as `kubelet_t` and may only execute files labeled `bin_t`. A bin directory
  under `/etc/microshift` (`kubernetes_file_t`) or `/opt` (`usr_t`) passes
  MicroShift's path validation but is denied execution under SELinux enforcing;
  the provider never runs and the only trace is the AVC. Move the bin directory
  under `/usr/libexec` or `/usr/local/bin`, or restore the label with
  `restorecon -Rv`, so the binaries carry `bin_t`.
- A lenient-decode warning in the kubelet logs referencing either key indicates
  a MicroShift version that does not support the feature, or a misspelled key.
- On bootc or rpm-ostree systems, a greenboot rollback after an image update
  with a validation error naming `imageCredentialProviderBinDir` indicates the
  new image was built without the provider binary.

## Alternatives (Not Implemented)

**Generic kubelet flag passthrough.** A `kubelet.args` or similar mechanism for
arbitrary flags was rejected. It exposes the full kubelet flag surface, most of
which MicroShift manages deliberately, and increases the risk of
misconfiguration. The OCPSTRAT explicitly rules this out of scope; it should be
tracked as a separate RFE if needed.

**Typed `kubelet:` section.** Replacing the schemaless map with a typed struct
was rejected because it would break the existing passthrough of arbitrary
`KubeletConfiguration` fields, which users depend on.

**Separate top-level configuration section.** Placing the keys outside
`kubelet:` (e.g. `imageCredentialProvider:`) would avoid special-casing keys
inside the passthrough and would be picked up by schema generation. It was
rejected because the OCPSTRAT acceptance criteria and the originating RFE
specify placement under `kubelet:`, where users familiar with the upstream flags
will look for them.

**Removing the reserved keys from the `Config.Kubelet` map.** An earlier draft
extracted the two keys by deleting them from (a copy of) the map during
configuration processing. This was rejected because
`microshift show-config --mode effective` marshals the `Config` struct, so the
keys would disappear from its output. Filtering at the point where the map is
marshaled into `KubeletConfiguration` keeps the effective configuration faithful
to user input.

**Warning instead of failing on unsafe ownership or permissions.** An earlier
draft only warned. A warning does not prevent a local user from replacing a
provider binary that kubelet executes as root, so the check is a startup
failure.

**Checking only the final directory.** An earlier draft checked ownership and
permissions of the bin directory only. This was rejected because it does not
prevent in-place modification of an existing writable binary, replacement of the
directory through a writable ancestor, or modification of the configuration
file, which controls the provider's arguments and environment. The trusted-path
rule covers all three.

**Validating the configured path's components instead of passing the canonical
path.** Requiring every component of the configured (pre-resolution) path to be
root-owned would also close the symlink re-pointing gap, but it would reject the
standard ostree and bootc layouts where system directories are reached through
symlinks under `/var`. Passing the canonical path to kubelet closes the gap
without constraining where the user writes the configured path.

**Rejecting symlinks outright.** Refusing any symlink in either path would be
simpler than resolving and checking targets, but standard image-based layouts
depend on symlinks for locations such as `/usr/local` and `/home`. Resolving
symlinks and checking every component of the canonical path provides the same
protection without rejecting those layouts.

**Packaging credential provider binaries.** Shipping `ecr-credential-provider`
or equivalents as MicroShift RPMs was rejected. The binaries are cloud-specific,
upstream-maintained, and have their own release and CVE cadence; there is one
per registry type, so shipping any subset would favor particular clouds.
Bundling them would make their lifecycle a MicroShift concern.
