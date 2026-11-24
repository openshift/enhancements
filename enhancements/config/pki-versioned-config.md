---
title: pki-versioned-config
authors:
  - "@sanchezl"
reviewers:
  - "@benluddy, for PKI profile resolution and certificate rotation domain expertise"
  - "@JoelSpeed, for openshift/api conventions and PKI CRD status type additions"
  - "@everettraven, for openshift/api type changes and PKI CRD status additions"
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
  - "/enhancements/security/internal-pki-config.md"
---

# PKI Versioned Config

## Summary

The PKI CRD (`config/v1alpha1/types_pki.go`) defines management modes
(Unmanaged, Default, Custom) for internal PKI configuration, but it has no
status subresource with version-indexed resolved profiles, no accessor in
library-go, and no startup synchronization. Operators must implement their own
pull-based configuration resolution with no guarantees about when configuration
is available at startup.

This enhancement adds a status subresource with version-indexed resolved PKI
profiles to the PKI CRD, provides a `PKIProfileAccess` wrapper in library-go
built on the generic `VersionedConfigAccess[*PKIProfile]` interface from the
[Versioned Config Access](versioned-config-access.md) framework, and
centralizes certificate lifetime fields in the versioned table. The result
enables PKI configuration to evolve across versions (for example, migrating
from RSA-2048 to ECDSA P-256 key algorithms), gives operators startup
synchronization and change notification, and provides administrators with
visibility into what their PKI management mode resolves to for their cluster
version.

## Motivation

The PKI CRD is partway through the three-layer pattern described in
[versioned-config-access.md](versioned-config-access.md). It has a spec with
management modes but lacks the other two layers:

1. **No status subresource.** There is no version-indexed resolved profile in
   CRD status. Operators cannot see what "Default" mode concretely means for
   their cluster version without reading source code.

2. **No version indexing.** PKI profiles are not indexed by cluster version.
   When the platform needs to evolve key algorithms across versions (for
   example, introducing ECDSA P-256 in 4.20 while 4.19 uses RSA-2048), there
   is no mechanism to serve different defaults to different versions during
   an upgrade.

3. **No accessor in library-go.** Operators implement pull-based configuration
   resolution with no startup synchronization channel and no change
   notification.

4. **Hardcoded lifetimes.** Certificate lifetime constants (signer validity,
   leaf validity, refresh intervals) are scattered across operator code as
   hardcoded values. There is no central, versioned location where these
   lifetimes are defined.

5. **Coordinated revendoring required for changes.** PKI defaults (key
   algorithms, lifetimes) are compiled into operators via library-go constants
   and per-operator code. Changing defaults requires updating library-go and
   revendoring it across every certificate-generating operator. With a
   materialized status approach, only cluster-config-operator needs to
   revendor; all other operators receive updated PKI profiles at runtime
   through their informer watch on CRD status.

### User Stories

* As a platform developer evolving PKI key algorithm defaults across OCP
  versions, I want the PKI CRD status to contain version-indexed resolved
  profiles, so that operators on different versions during an upgrade each see
  the correct defaults for their version.

* As a cluster administrator, I want to see what the "Default" PKI management
  mode concretely resolves to on my cluster's version (key algorithm, key size,
  signature algorithm, certificate lifetimes), so that I can understand the
  PKI configuration without reading source code.

* As a platform developer building a certificate-generating operator, I want a
  standard `PKIProfileAccess` from library-go with startup sync and change
  notification, so that I do not need to implement my own pull-based
  configuration resolution.

* As a platform developer, I want certificate lifetimes centralized in the
  versioned PKI profile table, so that future versions can adjust lifetimes
  without code changes in each individual operator.

### Goals

1. Add a status subresource with version-indexed resolved profiles to the PKI
   CRD.
2. Provide a `PKIProfileAccess` wrapper in library-go that uses
   `VersionedConfigAccess[*PKIProfile]`.
3. Centralize certificate lifetime fields in the `PKIProfile` type, replacing
   hardcoded constants scattered across operators.
4. Enable per-version PKI configuration evolution (key algorithms, lifetimes).
5. Provide a materializer controller in cluster-config-operator.

### Non-Goals

1. Changing the admin-facing API for selecting PKI management modes. The
   `spec.managementState` field on the PKI CRD remains unchanged.
2. Adding new key algorithms or certificate configurations. This enhancement
   provides the infrastructure; actual algorithm changes are separate work.
3. Changing the `ConfigurablePKI` feature gate. This enhancement is gated
   behind the existing feature gate and graduates alongside it.

## Proposal

This proposal applies the three-layer pattern from
[versioned-config-access.md](versioned-config-access.md) to PKI profiles:

1. **Declare**: A version-indexed PKI declaration table in openshift/api defines
   the platform's Default PKI profile for each version range.
2. **Materialize**: A controller in cluster-config-operator reads the PKI spec,
   resolves the management mode, and writes version-indexed entries into
   `pki.status.profiles[]`.
3. **Access**: A `PKIProfileAccess` type in library-go wraps
   `VersionedConfigAccess[*PKIProfile]`.

### Workflow Description

**cluster administrator** is a human user who selects a PKI management mode.

**cluster-config-operator** is the controller responsible for materializing
resolved PKI profiles into PKI status.

**consuming operator** is any OpenShift operator that generates certificates
and needs the resolved PKI profile.

#### PKI Profile Resolution

1. The cluster administrator configures the PKI CRD with a management mode
   (Unmanaged, Default, or Custom).
2. The cluster-config-operator materializer reads the spec and resolves the
   mode:
   - **Unmanaged**: Writes a nil profile entry for each version. Operators use
     their hardcoded defaults.
   - **Default**: Looks up the version-indexed declaration table for each
     version in ClusterVersion status. Writes the resolved profile.
   - **Custom**: Uses the administrator's explicit configuration as-is. Writes
     it for each version.
3. The consuming operator instantiates `PKIProfileAccess`, which wraps
   `VersionedConfigAccess[*PKIProfile]`. On startup, the accessor watches the
   PKI informer, finds the entry matching the operator's desired version,
   caches it, and closes the `InitialConfigObserved()` channel.
4. The operator calls `CurrentPKIProfile()`:
   - If the result is nil, the operator uses its hardcoded defaults (Unmanaged
     mode).
   - If non-nil, the operator calls `ResolveCertificateConfig` to resolve the
     profile's category hierarchy (signer, serving, client overrides falling
     back to defaults).
5. If the management mode changes or the cluster upgrades, the change handler
   fires and the operator restarts.

#### Version Skew During Upgrade

1. A cluster begins upgrading from 4.19 to 4.20. The PKI status contains
   profile entries for both versions.
2. An operator still running at version 4.19 finds its entry and uses
   RSA-2048 as the key algorithm (the 4.19 default).
3. A newly rolled-out operator at version 4.20 finds its entry and uses
   ECDSA P-256 as the key algorithm (the 4.20 default).
4. Both operators generate certificates with their version-appropriate key
   algorithms. Existing certificates are not re-generated; only new
   certificates use the new algorithm.
5. After all operators converge on 4.20, the 4.19 entry is eventually pruned.

### API Extensions

This enhancement adds a status subresource to the existing PKI CRD. No new
CRDs are added.

#### PKI Status Addition

The PKI CRD gains a status subresource with version-indexed resolved profiles:

```go
// in config/v1alpha1/types_pki.go

type PKIStatus struct {
    // conditions represent the observations of the current state.
    // +listType=map
    // +listMapKey=type
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`

    // profiles contains a list of resolved PKI profiles keyed by
    // payloadVersion. Operators must locate the version they are managing
    // and use the resolved profile for certificate generation.
    // A nil PKIProfile entry for a version means PKI is Unmanaged for that
    // version, and operators should use their hardcoded defaults.
    // +listType=map
    // +listMapKey=version
    // +optional
    Profiles []VersionedPKIProfile `json:"profiles,omitempty"`
}

type VersionedPKIProfile struct {
    // version matches the version provided by ClusterVersion and in the
    // ClusterOperator.Status.Versions field.
    // +required
    Version string `json:"version"`

    // profile contains the resolved PKI profile for this version.
    // When nil, PKI is Unmanaged for this version and operators should
    // use their hardcoded defaults.
    // +optional
    Profile *PKIProfile `json:"profile,omitempty"`
}
```

#### PKIProfile Lifetime Fields

The `PKIProfile` type gains certificate lifetime fields, centralizing values
that are currently hardcoded constants scattered across operators:

```go
// PKIProfile is extended with lifetime configuration.
// These fields move hardcoded certificate lifetime constants into the
// versioned table so that future versions can adjust lifetimes without
// code changes in each operator.

type PKIProfile struct {
    // ... existing key algorithm and signature algorithm fields ...

    // signerValidity defines the validity duration for signer (CA) certificates.
    // +optional
    SignerValidity *metav1.Duration `json:"signerValidity,omitempty"`

    // leafValidity defines the validity duration for leaf certificates.
    // +optional
    LeafValidity *metav1.Duration `json:"leafValidity,omitempty"`

    // refreshInterval defines how often certificates are refreshed
    // before expiration.
    // +optional
    RefreshInterval *metav1.Duration `json:"refreshInterval,omitempty"`
}
```

#### Behavioral Changes to Existing Resources

- **PKI**: Adds a status subresource. The PKI CRD is `v1alpha1` behind the
  `ConfigurablePKI` feature gate, so there are no backward compatibility
  concerns.

### Topology Considerations

#### Hypershift / Hosted Control Planes

In Hypershift, control plane components run in the management cluster at a
potentially different version than data plane components in the guest cluster.
The version-indexed status model handles this naturally:

- Each cluster materializes its own PKI status independently.
- The accessor's `desiredVersion` parameter ensures each operator reads the
  correct entry for its version.

#### Standalone Clusters

Fully applicable. Standalone clusters are the primary deployment model.

#### Single-node Deployments or MicroShift

**SNO**: No additional resource consumption. The materializer runs in
cluster-config-operator which is already present. The accessor watches
existing PKI informers.

**MicroShift**: Out of scope. MicroShift does not use the library-go operator
framework for PKI profile resolution.

### Implementation Details/Notes/Constraints

#### T = *PKIProfile (nil = Unmanaged)

The use of a pointer type allows nil to represent "Unmanaged" mode, where
operators use their hardcoded defaults. This avoids needing a sentinel value
or a separate "is managed" boolean.

#### Version-Indexed Declaration Table

```go
var VersionedPKIDefaults = []VersionedPKIDeclaration{
    {MinVersion: "4.19", Profile: pkiProfile_RSA2048},
    {MinVersion: "4.20", Profile: pkiProfile_ECDSA_P256},
}
```

The materializer uses floor-based resolution: for a requested version `V`, it
finds the highest `MinVersion` entry that is less than or equal to `V`. Only
versions where PKI defaults change need entries.

Custom mode bypasses the declaration table entirely; the administrator's
explicit configuration is used as-is for all versions.

#### Materializer Controller

A controller (~100 lines) in cluster-config-operator:

1. Watches the PKI spec for management mode changes.
2. Reads all versions from `ClusterVersion.status.history` and
   `ClusterVersion.status.desired.version`.
3. Resolves each version:
   - **Unmanaged**: writes nil profile.
   - **Default**: performs floor-based lookup in declaration table.
   - **Custom**: uses admin's configuration directly.
4. Writes version-indexed entries into `pki.status.profiles[]`.

#### PKIProfileAccess

```go
type PKIProfileAccess struct {
    inner versionedconfig.VersionedConfigAccess[*PKIProfile]
}

func NewPKIProfileAccess(
    desiredVersion, missingVersionMarker string,
    clusterVersionInformer cache.SharedIndexInformer,
    pkiInformer cache.SharedIndexInformer,
    eventRecorder events.Recorder,
) *PKIProfileAccess

func (a *PKIProfileAccess) Run(ctx context.Context)
func (a *PKIProfileAccess) InitialPKIProfileObserved() <-chan struct{}
func (a *PKIProfileAccess) CurrentPKIProfile() (*PKIProfile, error)
func (a *PKIProfileAccess) SetChangeHandler(fn func(old, new *PKIProfile))
```

These methods delegate to the inner `VersionedConfigAccess[*PKIProfile]`.
The domain-specific naming (`InitialPKIProfileObserved` rather than
`InitialConfigObserved`) follows the convention established by
`FeatureGateAccess.InitialFeatureGatesObserved()`.

When the returned profile is nil, the operator uses its hardcoded defaults
(Unmanaged mode). When non-nil, the operator calls `ResolveCertificateConfig`
to resolve the profile's category hierarchy (signer, serving, client overrides
falling back to defaults).

#### Override Hierarchy

The accessor provides the version-selected PKI profile (what "Default" means
for this cluster version). The `ResolveCertificateConfig` chain then resolves
the category hierarchy within that profile:

1. Check for a certificate-specific override in the profile.
2. Fall back to the category default (signer, serving, or client).
3. Fall back to the profile-level default.

This two-tier separation keeps version selection (accessor) and category
resolution (`ResolveCertificateConfig`) as independent concerns.

### Risks and Mitigations

**Risk: PKI status increases API object size.**
Mitigation: During a normal upgrade, at most two version entries exist in
status. Each `VersionedPKIProfile` entry is small (key algorithm, signature
algorithm, lifetime fields). Two entries add negligible size to the PKI object.

**Risk: Materializer controller failure leaves stale status.**
Mitigation: Same as TLS (see
[tls-versioned-config.md](tls-versioned-config.md)). The materializer runs in
cluster-config-operator with established health monitoring. Operators block at
startup until they have valid configuration.

**Risk: Lifetime centralization changes existing operator behavior.**
Mitigation: The initial versioned table entries use the same lifetime values
currently hardcoded in operators. The centralization is a "move, not change"
operation. Future lifetime adjustments are separate work with their own
validation.

**Risk: Nil profile semantics (Unmanaged) may confuse consumers.**
Mitigation: The `CurrentPKIProfile()` return type is `*PKIProfile`, making nil
checks natural in Go. Documentation and code comments make the nil = Unmanaged
contract clear.

### Drawbacks

The PKI CRD is `v1alpha1` behind a feature gate, which limits the impact of
this enhancement but also limits its validation surface. Until `ConfigurablePKI`
graduates to a wider audience, the PKI accessor sees limited production use.
However, building the accessor now (while the CRD is still alpha) is the right
time to establish the pattern before the API stabilizes.

## Design Details

### Open Questions

1. Should the PKI profile's lifetime fields be added to `PKIProfile` in the
   same phase as the accessor, or deferred to a later phase? Adding them
   increases scope but avoids a follow-up API change.

2. What is the retention policy for old version entries in PKI status? The
   materializer should prune entries for versions no longer present in
   ClusterVersion status, but the exact cleanup timing needs definition.

### Test Plan

#### Unit Tests

- **Declaration table**: Test floor-based version lookup. Verify that
  requesting version 4.19 returns RSA-2048 and requesting 4.20 returns
  ECDSA P-256.
- **Mode resolution**: Test that Unmanaged returns nil, Default returns the
  version-appropriate profile, and Custom passes through the admin's
  configuration.
- **StatusExtractor**: Test that `extractPKIProfilesFromPKI` correctly reads
  `PKIStatus.Profiles` and converts to `[]VersionedEntry[*PKIProfile]`.
- **EqualityFunc**: Test that `pkiProfileEqual` correctly handles nil vs
  non-nil, identical profiles, and profiles differing in key algorithm or
  lifetime fields.
- **Lifetime fields**: Test that versioned table entries include the expected
  lifetime values for each version.

#### Integration Tests

- **Materializer**: Test that cluster-config-operator correctly writes
  version-indexed PKI profiles into PKI status for each management mode:
  - Unmanaged produces nil profile entries
  - Default produces version-appropriate profile entries
  - Custom produces the admin's explicit configuration for all versions

#### End-to-End Tests

- **PKI profile resolution**: On a TechPreview cluster with `ConfigurablePKI`
  enabled, set the PKI management mode to Default and verify that
  `status.profiles[].profile` contains the expected key algorithm configuration
  for the cluster's version.
- **Unmanaged mode**: Verify that setting Unmanaged mode results in nil profile
  entries and operators use their hardcoded defaults.

### Graduation Criteria

PKI profiles are gated by the `ConfigurablePKI` feature gate. The
version-indexed status and accessor graduate alongside the PKI feature.

#### Dev Preview -> Tech Preview

- PKI CRD gains status subresource with version-indexed profiles
- PKI accessor available in library-go
- Materializer controller running in cluster-config-operator
- At least one certificate-generating operator migrated to the accessor
- Unit tests and integration tests passing

#### Tech Preview -> GA

- All certificate-generating operators migrated to the accessor
- e2e tests for version resolution and lifetime centralization
- Documentation in openshift-docs describing how admins can inspect resolved
  PKI profiles
- `ConfigurablePKI` feature gate moves to Default

#### Removing a deprecated feature

N/A. No features are deprecated by this enhancement.

### Upgrade / Downgrade Strategy

#### Upgrade

**No admin action required.** On upgrade from N to N+1:

1. The CRD schema update adds the status subresource (additive, no breaking
   change). The PKI CRD is `v1alpha1`, so schema evolution is expected.
2. The new cluster-config-operator version starts the PKI materializer
   controller, which populates status entries.
3. New operator versions use the accessor to read their version's entry from
   status.
4. Old operator versions continue functioning because they use their existing
   pull-based configuration resolution (the spec is unchanged).

#### Downgrade

On downgrade from N+1 to N:

1. The old cluster-config-operator does not have the PKI materializer, so
   status entries become stale. Old operators do not read them.
2. For v1alpha1 APIs, unknown fields in status are preserved but not validated.
   The stale entries are harmless.
3. Old operators resume using their existing configuration mechanisms.
4. No manual cleanup is required.

### Version Skew Strategy

See the general version skew strategy in
[versioned-config-access.md](versioned-config-access.md). For PKI specifically:

- During an upgrade, operators at version N generate certificates using N's key
  algorithm; operators at version N+1 generate certificates using N+1's key
  algorithm. Existing certificates are not re-generated; only new certificates
  use the new algorithm.
- This is safe because PKI consumers (TLS clients and servers) support multiple
  key algorithms simultaneously. A certificate signed with RSA-2048 and a
  certificate signed with ECDSA P-256 can coexist in the same cluster without
  conflicts.

### Operational Aspects of API Extensions

This enhancement adds a status subresource to the PKI CRD. It does not add
webhooks, finalizers, or aggregated API servers.

**Impact on existing SLIs:**

- PKI object size increases minimally (two version entries during upgrade).
  The PKI CRD has minimal watch traffic (only certificate-generating operators
  watch it).
- No impact on API latency.

**Monitoring:**

- The existing `cluster-config-operator` health metrics cover the materializer
  controller.
- The accessor emits events on initial config observation and config changes.

**Escalation path:** Control Plane / Cert team.

#### Failure Modes

- **Materializer failure**: If the cluster-config-operator PKI materializer
  controller fails, status entries are not written or updated. Operators using
  the accessor block at startup (if no entry exists yet) or continue using
  the last-observed configuration. Detection: `oc get co cluster-config-operator`
  shows Degraded.
- **Stale status entries**: If ClusterVersion changes but the materializer has
  not yet updated status, operators at the new version may experience a startup
  delay. Detection: operator logs show `missing desired version` errors.
- **Version string mismatch**: If an operator reports a version string that
  does not exactly match any status entry, the accessor returns an error and
  the operator blocks at startup. Detection: operator pod in CrashLoopBackOff
  with `timed out waiting for VersionedConfig detection` in logs.

#### Support Procedures

**Symptom: Certificate-generating operator fails to start with "timed out waiting for PKI config"**

Detection: Operator logs show `timed out waiting for VersionedConfig detection`
or similar messages. The operator pod is in CrashLoopBackOff.

Root cause: The PKI materializer has not written a status entry for the
operator's version.

Remediation:
1. Check `oc get co cluster-config-operator` for degraded conditions.
2. Check `oc get pki cluster -o yaml` to see if version-indexed PKI status
   entries exist.
3. Compare the operator's reported desired version (from operator logs) with
   the versions in CRD status.
4. If cluster-config-operator is healthy, check its logs for PKI materializer
   errors.

**Symptom: Operator using wrong key algorithm after upgrade**

Detection: Newly generated certificates use the old version's key algorithm
instead of the new version's.

Root cause: The operator is reading a stale status entry (old version) instead
of the new version's entry.

Remediation:
1. Check `oc get pki cluster -o jsonpath='{.status.profiles}'` to see which
   version entries exist.
2. Verify the operator's reported version matches an entry in status.
3. If the operator is at the new version but reading the old entry, restart
   the operator and collect logs.

## Implementation History

N/A. This enhancement is newly proposed.

## Alternatives

### Pull-Based Configuration Without Accessor

The current approach (operators read PKI spec directly) works for a single
version but does not support version-indexed configuration. Each operator
independently resolves the management mode, duplicating logic and providing
no startup synchronization. The accessor pattern centralizes this logic and
adds the three guarantees (version matching, startup sync, change detection)
that the pull-based approach lacks.

## Infrastructure Needed

No new subprojects, repositories, or testing infrastructure needed. All changes
are to existing repositories:

- **openshift/api**: `PKIStatus` type with version-indexed profiles, lifetime
  field additions to `PKIProfile`.
- **openshift/library-go**: `PKIProfileAccess` wrapper type.
- **openshift/cluster-config-operator**: PKI materializer controller.
