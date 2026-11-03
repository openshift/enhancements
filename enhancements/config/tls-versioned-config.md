---
title: tls-versioned-config
authors:
  - "@sanchezl"
reviewers:
  - "@JoelSpeed, for openshift/api conventions and APIServerStatus type additions"
  - "@damdo, for TLS profile domain expertise, please review version-indexed cipher evolution"
  - "@p0lyn0mial, for TLS config observer code being replaced by the accessor"
  - "@everettraven, for openshift/api type changes and APIServerStatus additions"
approvers:
  - "@JoelSpeed"
api-approvers:
  - "@JoelSpeed"
creation-date: 2026-08-04
last-updated: 2026-08-04
tracking-link:
  - https://redhat.atlassian.net/browse/OCPSTRAT-2271
see-also:
  - "/enhancements/config/versioned-config-access.md"
  - "/enhancements/kube-apiserver/tls-config.md"
---

# TLS Versioned Config

## Summary

OpenShift TLS security profiles (Old, Intermediate, Modern, Custom) are
currently defined in a static Go map (`var TLSProfiles` in
`config/v1/types_tlssecurityprofile.go`) that cannot vary by cluster version.
When cipher suites need to evolve across OCP versions (for example, adding
post-quantum groups like X25519MLKEM768 in newer versions), the only option is
to modify the compiled-in map for all versions simultaneously.

This enhancement introduces version-indexed TLS profile declarations in
openshift/api, materializes resolved profiles into `APIServerStatus`, and
provides a `TLSProfileAccess` wrapper in library-go built on the generic
`VersionedConfigAccess[ResolvedTLSProfile]` interface from the
[Versioned Config Access](versioned-config-access.md) framework. The result
enables cipher suite evolution per version, gives administrators visibility
into what their selected profile concretely resolves to, and provides operators
with startup synchronization and change notification for TLS configuration.

## Motivation

The static `var TLSProfiles` map has several limitations:

1. **No version indexing.** The map returns the same cipher list regardless of
   cluster version. When OCP 4.22 needs to include post-quantum cipher groups
   that OCP 4.18 does not support, there is no mechanism to serve different
   cipher lists to different versions during an upgrade.

2. **No CRD status visibility.** Administrators who select "Intermediate" have
   no way to see what concrete cipher suites and minimum TLS version that
   resolves to on their cluster without reading source code.

3. **No startup synchronization.** Operators that use TLS profiles read the
   static map at compile time. There is no informer-based observation, no
   startup sync channel, and no change notification. If the platform ever needs
   to evolve TLS configuration dynamically, the static map approach cannot
   support it.

4. **No accessor in library-go.** Each operator that needs the resolved TLS
   profile must implement its own lookup and override logic.

5. **Coordinated revendoring required for changes.** Because cipher lists are
   compiled into every operator via the static map in openshift/api, any change
   to TLS profiles requires revendoring openshift/api across every consuming
   operator. With a materialized status approach, only cluster-config-operator
   needs to revendor openshift/api; all other operators receive updated profiles
   at runtime through their informer watch on CRD status.

### User Stories

* As a cluster administrator, I want to see what the "Intermediate" TLS profile
  concretely resolves to on my cluster's version, so that I can verify the exact
  cipher suites and minimum TLS version in use without reading source code:

  ```bash
  oc get apiserver cluster -o jsonpath='{.status.tlsProfiles}'
  ```

* As a platform developer evolving TLS cipher defaults across OCP versions, I
  want to declare version-indexed TLS profiles in openshift/api, so that OCP
  4.22 can include post-quantum cipher groups while OCP 4.18 continues using
  its existing ciphers, without modifying the static map that all versions
  share.

* As a platform developer building an operator that serves TLS, I want a
  standard `TLSProfileAccess` from library-go with startup sync and change
  notification, so that I do not need to implement my own TLS configuration
  lookup and observation logic.

* As an SRE investigating a TLS issue during a version-skewed upgrade, I want
  operators at both old and new versions to use their version-appropriate cipher
  lists, so that a newly added cipher does not cause failures on operators
  still running the old version.

### Goals

1. Replace the static `var TLSProfiles` map with a version-indexed declaration
   table in openshift/api.
2. Materialize resolved TLS profiles into `APIServerStatus` so administrators
   can inspect them and operators can observe them.
3. Provide a `TLSProfileAccess` wrapper in library-go that uses
   `VersionedConfigAccess[ResolvedTLSProfile]`.
4. Maintain backward compatibility for existing consumers of `var TLSProfiles`
   through a compatibility alias.
5. Enable per-version cipher suite evolution (including post-quantum readiness).

### Non-Goals

1. Adding specific new cipher suites or TLS configuration values. This
   enhancement provides the infrastructure; actual cipher changes are separate
   work.
2. Changing the admin-facing API for selecting TLS profiles. The
   `spec.tlsSecurityProfile` field on the APIServer CRD remains unchanged.
3. Changing per-component TLS override mechanisms. Components that support
   their own `tlsSecurityProfile` field (such as ingress controllers) continue
   to use a stateless `ResolveTLSProfile` merge function.

## Proposal

This proposal applies the three-layer pattern from
[versioned-config-access.md](versioned-config-access.md) to TLS profiles:

1. **Declare**: A version-indexed TLS declaration table in openshift/api
   replaces the static `var TLSProfiles` map.
2. **Materialize**: A controller in cluster-config-operator reads the admin's
   selected profile type from `apiserver.spec.tlsSecurityProfile.type`, looks
   up the declaration table for each version present in ClusterVersion status,
   and writes resolved entries into `apiserver.status.tlsProfiles[]`.
3. **Access**: A `TLSProfileAccess` type in library-go wraps
   `VersionedConfigAccess[ResolvedTLSProfile]`.

### Workflow Description

**cluster administrator** is a human user who selects a TLS profile type.

**cluster-config-operator** is the controller responsible for materializing
resolved TLS profiles into APIServer status.

**consuming operator** is any OpenShift operator that needs the resolved TLS
profile to configure its operand.

#### TLS Profile Resolution

1. The cluster administrator sets
   `apiserver.config.openshift.io/cluster .spec.tlsSecurityProfile.type = Intermediate`.
2. The cluster-config-operator materializer reads the spec, looks up the
   version-indexed Intermediate profile for each version present in
   ClusterVersion status, and writes resolved `TLSProfileSpec` entries into
   `apiserver.status.tlsProfiles[]`.
3. The kube-apiserver-operator instantiates `TLSProfileAccess`, which wraps
   `VersionedConfigAccess[ResolvedTLSProfile]`. On startup, the accessor watches
   the APIServer informer, finds the entry matching the operator's desired
   version, caches it, and closes the `InitialConfigObserved()` channel.
4. The operator calls `CurrentConfig()` to get the resolved `ResolvedTLSProfile`
   containing the concrete cipher list and minimum TLS version for its version.
5. If the administrator changes the profile type or the cluster upgrades (adding
   a new version entry), the change handler fires and the operator restarts to
   pick up the new configuration.
6. The cluster administrator can inspect the resolved configuration:

```bash
oc get apiserver cluster -o jsonpath='{.status.tlsProfiles}'
```

#### Version Skew During Upgrade

1. A cluster begins upgrading from 4.21 to 4.22. The APIServer status contains
   TLS profile entries for both versions.
2. An operator still running at version 4.21 finds the `version: "4.21.x"`
   entry and uses its cipher list (which does not include new 4.22 ciphers).
3. A newly rolled-out operator at version 4.22 finds the `version: "4.22.0"`
   entry and uses its cipher list (which may include new ciphers).
4. Both operators function correctly with their version-appropriate
   configuration throughout the upgrade window.
5. After all operators converge on 4.22, the 4.21 entry is eventually pruned
   from status.

#### Per-Component Override

Components that support their own `tlsSecurityProfile` field (e.g., ingress
controllers) apply a stateless `ResolveTLSProfile` function:

1. If the component has its own `tlsSecurityProfile` set, resolve it directly
   (this does not use the accessor).
2. If the component does not have its own setting, use the cluster-wide resolved
   profile from `TLSProfileAccess.CurrentConfig()`.

This two-tier hierarchy separates concerns: the accessor provides the
cluster-wide resolved profile; per-component overrides are stateless merges.

### API Extensions

This enhancement modifies the status of the existing APIServer CRD. No new
CRDs are added.

#### APIServer Status Addition

The currently empty `APIServerStatus` struct gains a version-indexed list of
resolved TLS profiles:

```go
// in config/v1/types_apiserver.go

type APIServerStatus struct {
    // tlsProfiles contains a list of resolved TLS security profiles keyed by
    // payloadVersion. Operators must locate the version they are managing,
    // find the resolved profile, and configure their operand accordingly.
    // The resolved profile reflects the admin's selected profile type
    // (Old, Intermediate, Modern, Custom) evaluated against the version-indexed
    // declaration table for each version.
    // +listType=map
    // +listMapKey=version
    // +optional
    TLSProfiles []VersionedTLSProfile `json:"tlsProfiles,omitempty"`
}

type VersionedTLSProfile struct {
    // version matches the version provided by ClusterVersion and in the
    // ClusterOperator.Status.Versions field.
    // +required
    Version string `json:"version"`

    // profileType indicates which profile type was resolved (Old, Intermediate,
    // Modern, or Custom).
    // +required
    ProfileType TLSProfileType `json:"profileType"`

    // spec contains the concrete cipher suites, groups, and minimum TLS version
    // that the profile resolves to for this version.
    // +required
    Spec TLSProfileSpec `json:"spec"`
}
```

This mirrors the pattern established by `FeatureGateStatus.FeatureGates`
(`[]FeatureGateDetails` keyed by `version`) and uses existing `TLSProfileType`
and `TLSProfileSpec` types.

#### Behavioral Changes to Existing Resources

- **APIServer**: The status subresource changes from empty to populated. No
  existing consumers read `APIServerStatus` today, so this is purely additive.

### Topology Considerations

#### Hypershift / Hosted Control Planes

In Hypershift, control plane components run in the management cluster at a
potentially different version than data plane components in the guest cluster.
The version-indexed status model handles this naturally:

- Each cluster materializes its own APIServer status independently.
- The accessor's `desiredVersion` parameter ensures each operator reads the
  correct entry for its version.

This is the same model used by FeatureGateAccess in Hypershift today.

#### Standalone Clusters

Fully applicable. Standalone clusters are the primary deployment model.

#### Single-node Deployments or MicroShift

**SNO**: No additional resource consumption. The materializer runs in
cluster-config-operator which is already present. The accessor watches
existing APIServer informers.

**MicroShift**: Out of scope. MicroShift does not use the library-go operator
framework for TLS profile resolution.

### Implementation Details/Notes/Constraints

#### T = ResolvedTLSProfile

```go
type ResolvedTLSProfile struct {
    ProfileType TLSProfileType
    Spec        TLSProfileSpec
}
```

#### Version-Indexed Declaration Table

The static `var TLSProfiles` map is replaced by a version-indexed declaration
table. Only versions where profiles change need entries; the lookup uses
floor-based resolution (the highest `MinVersion` entry at or below the
requested version):

```go
// Pseudocode for the version-indexed TLS declaration table.
// Actual implementation uses the features/util.go builder pattern.
var VersionedTLSProfiles = map[TLSProfileType][]VersionedTLSDeclaration{
    TLSProfileIntermediateType: {
        {MinVersion: "4.18", Spec: intermediateSpec_4_18},
        {MinVersion: "4.22", Spec: intermediateSpec_4_22}, // adds PQC groups
    },
    // Old, Modern follow the same pattern
}
```

#### Backward-Compatible Alias

The existing `var TLSProfiles` is retained as a backward-compatible alias that
resolves to the latest version's values. Code that reads `TLSProfiles` today
continues to compile and return reasonable defaults. This alias is deprecated
once all consumers have migrated to the accessor.

#### Materializer Controller

A new controller (~100 lines) in cluster-config-operator:

1. Watches the APIServer spec for `spec.tlsSecurityProfile.type` changes.
2. Reads all versions from `ClusterVersion.status.history` and
   `ClusterVersion.status.desired.version`.
3. For each version, performs a floor-based lookup in the declaration table
   to find the appropriate profile.
4. Writes resolved entries into `apiserver.status.tlsProfiles[]`.

#### TLSProfileAccess

```go
type TLSProfileAccess struct {
    inner versionedconfig.VersionedConfigAccess[ResolvedTLSProfile]
}

func NewTLSProfileAccess(
    desiredVersion, missingVersionMarker string,
    clusterVersionInformer cache.SharedIndexInformer,
    apiServerInformer cache.SharedIndexInformer,
    eventRecorder events.Recorder,
) *TLSProfileAccess

func (a *TLSProfileAccess) Run(ctx context.Context)
func (a *TLSProfileAccess) InitialTLSProfileObserved() <-chan struct{}
func (a *TLSProfileAccess) CurrentTLSProfile() (ResolvedTLSProfile, error)
func (a *TLSProfileAccess) SetChangeHandler(fn func(old, new ResolvedTLSProfile))
```

These methods delegate to the inner `VersionedConfigAccess[ResolvedTLSProfile]`.
The domain-specific naming (`InitialTLSProfileObserved` rather than
`InitialConfigObserved`) follows the convention established by
`FeatureGateAccess.InitialFeatureGatesObserved()`.

#### Override Hierarchy

The accessor provides the cluster-wide resolved TLS profile. Components that
support per-component TLS overrides (e.g., ingress controllers with their own
`tlsSecurityProfile` field) apply a stateless `ResolveTLSProfile` function
that merges the component-level override with the cluster-wide default. This
function does not need the accessor; it operates on the already-resolved value.

### Risks and Mitigations

**Risk: Version-indexed TLS status increases API object size.**
Mitigation: During a normal upgrade, at most two version entries exist in
status. Each `VersionedTLSProfile` entry is approximately 500 bytes (cipher
list + groups + min version). Two entries add ~1KB to the APIServer object,
which is negligible.

**Risk: Materializer controller failure leaves stale status.**
Mitigation: The materializer controller runs in cluster-config-operator, which
has high availability and established monitoring. If the materializer is
temporarily unavailable, operators continue using the last-observed
configuration (cached by the accessor). The accessor's `InitialConfigObserved`
channel ensures operators block at startup until they have valid configuration,
and a timeout (typically 1 minute) converts this into a clear startup failure
rather than silent misconfiguration.

**Risk: Backward compatibility breakage for `var TLSProfiles` consumers.**
Mitigation: The static map is retained as a backward-compatible alias. Migration
to the accessor is incremental, per-operator, with each migration being a small
PR. Unmigrated consumers continue to compile and return reasonable defaults.

### Drawbacks

Migration burden is the primary concern. Existing TLS consumers use the static
`var TLSProfiles` map directly. Migrating them to the accessor is incremental
(per-operator, each one a small PR), and the backward-compatible alias ensures
unmigrated consumers continue to work. However, until all consumers are
migrated, the platform has two paths for TLS profile resolution, which adds
cognitive overhead for developers.

## Design Details

### Open Questions

1. Should the version-indexed TLS declaration table be defined in openshift/api
   (alongside the existing `var TLSProfiles`) or in a separate package? Placing
   it in openshift/api keeps the declaration close to the types but increases
   the API module's surface area.

2. What is the retention policy for old version entries in `APIServerStatus`?
   The materializer should prune entries for versions no longer present in
   `ClusterVersion.status`, but the exact cleanup timing needs definition.

### Test Plan

#### Unit Tests

- **Declaration table**: Test floor-based version lookup for each profile type.
  Verify that requesting version 4.20 with entries at 4.18 and 4.22 returns
  the 4.18 entry. Verify that requesting 4.22 returns the 4.22 entry.
- **Backward-compatible alias**: Verify that `var TLSProfiles` returns the same
  values as the latest version entry in the declaration table.
- **StatusExtractor**: Test that `extractTLSProfilesFromAPIServer` correctly
  reads `APIServerStatus.TLSProfiles` and converts to
  `[]VersionedEntry[ResolvedTLSProfile]`.
- **EqualityFunc**: Test that `resolvedTLSProfileEqual` correctly compares
  profiles with different cipher lists, different minimum TLS versions, and
  different profile types.

#### Integration Tests

- **Materializer**: Test that cluster-config-operator correctly writes
  version-indexed TLS profiles into APIServer status when:
  - The admin changes `spec.tlsSecurityProfile.type`
  - The cluster upgrades (new version appears in ClusterVersion status)
  - Custom profile type passes through the admin's explicit cipher configuration

#### End-to-End Tests

- **TLS version resolution**: On a running cluster, set
  `spec.tlsSecurityProfile.type = Intermediate`, verify that
  `status.tlsProfiles[].spec` contains the expected ciphers for the cluster's
  version. Compare against the declaration table entry for that version.
- **Upgrade simulation**: Using a version-skewed test environment, verify that
  operators at both old and new versions find their respective status entries
  and use version-appropriate configuration.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Version-indexed declaration table in openshift/api with backward-compatible
  `var TLSProfiles` alias
- APIServer status populated by cluster-config-operator materializer
- TLS accessor available in library-go
- At least one operator migrated to the accessor
- Unit tests for version lookup and integration tests for materializer

#### Tech Preview -> GA

- All TLS-consuming operators migrated to the accessor
- Static `var TLSProfiles` map deprecated
- e2e tests for version resolution
- Documentation in openshift-docs describing how admins can inspect resolved
  TLS profiles

#### Removing a deprecated feature

The static `var TLSProfiles` map in openshift/api will be deprecated once all
consumers have migrated to the accessor. Removal follows standard deprecation
policy:

1. Mark `var TLSProfiles` as deprecated with a comment pointing to the accessor
2. Allow at least one minor release for downstream consumers to migrate
3. Remove the deprecated map in a subsequent release

### Upgrade / Downgrade Strategy

#### Upgrade

**No admin action required.** On upgrade from N to N+1:

1. The CRD schema update adds the new status fields (additive, no breaking
   change).
2. The new cluster-config-operator version starts the TLS materializer
   controller, which populates status entries for all versions present in
   ClusterVersion status.
3. New operator versions use the accessor to read their version's entry from
   status.
4. Old operator versions (not yet rolled out to the new version) continue
   functioning because they use the static `var TLSProfiles` map (which is
   preserved as a backward-compatible alias).

#### Downgrade

On downgrade from N+1 to N:

1. The old cluster-config-operator does not have the TLS materializer
   controller, so it stops updating the TLS status fields. The status entries
   become stale but are ignored because old operators do not read them.
2. The stale status fields remain in etcd but are pruned on read for v1 APIs
   (unknown fields in status are pruned).
3. Old operators resume using the static `var TLSProfiles` map and function
   exactly as they did before the upgrade.
4. No manual cleanup is required.

### Version Skew Strategy

See the general version skew strategy in
[versioned-config-access.md](versioned-config-access.md). For TLS specifically:

- During an upgrade, operators at version N use the N cipher list; operators at
  version N+1 use the N+1 cipher list. A client connecting to an operator at
  version N will negotiate using N's ciphers; a client connecting to an
  operator at N+1 will negotiate using N+1's ciphers.
- This is safe because TLS cipher negotiation is inherently
  version-independent: the client and server negotiate the best mutually
  supported cipher regardless of what the other party's "default" list contains.

### Operational Aspects of API Extensions

This enhancement adds version-indexed entries to the status subresource of the
APIServer CRD. It does not add webhooks, finalizers, or aggregated API servers.

**Impact on existing SLIs:**

- APIServer object size increases by ~1KB during upgrades (two version entries).
  This has negligible impact on API throughput and watch performance.
- No impact on API latency. The status fields are populated asynchronously by
  the materializer, not in the API server's request path.

**Monitoring:**

- The existing `cluster-config-operator` health metrics cover the materializer
  controller.
- The accessor emits events on initial config observation and config changes.

**Escalation path:** Control Plane / Cert team (kube-apiserver component).

#### Failure Modes

- **Materializer failure**: If the cluster-config-operator TLS materializer
  controller fails, status entries are not written or updated. Operators using
  the accessor block at startup (if no entry exists yet) or continue using
  the last-observed configuration (if an entry was previously written). The
  `cluster-config-operator` already has established health monitoring and
  alerting. Detection: `oc get co cluster-config-operator` shows Degraded.
- **Stale status entries**: If ClusterVersion changes but the materializer has
  not yet updated status, operators at the new version may experience a startup
  delay. This is bounded by the materializer's reconciliation interval
  (seconds, not minutes). Detection: operator logs show
  `missing desired version` errors.

#### Support Procedures

**Symptom: Operator fails to start with "timed out waiting for TLS config"**

Detection: Operator logs show `timed out waiting for VersionedConfig detection`
or similar messages. The operator pod is in CrashLoopBackOff.

Root cause: The TLS materializer has not written a status entry for the
operator's version. This can happen if cluster-config-operator is unhealthy or
if there is an unexpected version string mismatch.

Remediation:
1. Check `oc get co cluster-config-operator` for degraded conditions.
2. Check `oc get apiserver cluster -o yaml` to see if version-indexed TLS
   status entries exist.
3. Compare the operator's reported desired version (from operator logs) with
   the versions in CRD status.
4. If cluster-config-operator is healthy, check its logs for TLS materializer
   errors.

**Symptom: Operator using wrong TLS ciphers after upgrade**

Detection: The operator's TLS handshake fails with clients that expect the new
version's cipher list, or security scans report unexpected ciphers.

Root cause: The operator is reading a stale status entry (old version) instead
of the new version's entry.

Remediation:
1. Check `oc get apiserver cluster -o jsonpath='{.status.tlsProfiles}'` to see
   which version entries exist.
2. Verify the operator's reported version matches an entry in status.
3. If the operator is at the new version but reading the old entry, this
   indicates a bug in version matching. Restart the operator and collect logs.

## Implementation History

N/A. This enhancement is newly proposed.

## Alternatives

### Keep Static Maps for TLS

The status quo (static `var TLSProfiles` map) works when cipher lists change
infrequently and do not need to differ by version. However, the addition of
post-quantum ciphers (X25519MLKEM768) and the ongoing evolution of Mozilla's
recommendations mean that cipher lists will change more frequently. Adding
version-indexed entries to a static Go map would require consumers to pass
their version and perform the lookup themselves, duplicating logic across
operators. The accessor pattern centralizes this logic and adds startup sync
that the static map approach lacks entirely.

## Infrastructure Needed

No new subprojects, repositories, or testing infrastructure needed. All changes
are to existing repositories:

- **openshift/api**: Version-indexed TLS declaration table, `APIServerStatus`
  type additions.
- **openshift/library-go**: `TLSProfileAccess` wrapper type.
- **openshift/cluster-config-operator**: TLS materializer controller.
