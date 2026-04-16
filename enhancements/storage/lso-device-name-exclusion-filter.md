---
title: lso-device-name-exclusion-filter
authors:
  - "@tsmetana"
reviewers:
  - TBD
approvers:
  - TBD
api-approvers:
  - TBD
creation-date: 2026-04-16
last-updated: 2026-04-16
tracking-link:
  - https://issues.redhat.com/browse/OCPBUGS-52262
see-also:
replaces:
superseded-by:
---

# Local Storage Operator: Device Name Exclusion Filter

## Summary

This enhancement adds a new `DeviceExclusionSpec` field to the `LocalVolumeSet`
API that allows administrators to exclude devices from automatic provisioning
based on their kernel name (KName) using glob patterns. This complements the
existing `DeviceInclusionSpec` by providing a deny-list mechanism, enabling
users to prevent specific devices (e.g. `rbd*`, `nbd*`) from being
automatically consumed by the Local Storage Operator.

## Motivation

The Local Storage Operator's `LocalVolumeSet` resource currently supports
filtering devices for automatic provisioning via `DeviceInclusionSpec`, which
acts as an allow-list based on device type, size, model, and vendor. However,
there is no mechanism to exclude specific devices by name. In environments where
certain device classes (such as Ceph RBD devices or network block devices)
appear as local block devices but must not be managed by LSO, administrators
have no straightforward way to prevent their consumption. This forces
administrators into complex workarounds or risks data loss from unintended
device provisioning.

### User Stories

* As a cluster administrator, I want to exclude all `rbd` devices (e.g.
  `rbd0`, `rbd1`, ...) from automatic provisioning by the Local Storage
  Operator, so that Ceph-managed devices are not accidentally consumed and
  formatted by LSO.

* As a cluster administrator, I want to specify multiple device name patterns
  to exclude (e.g. `rbd*` and `nbd*`), so that I can prevent several classes of
  virtual block devices from being provisioned while still allowing LSO to
  manage physical disks automatically.

* As a site reliability engineer operating a cluster at scale, I want the
  exclusion filter to be declarative and part of the `LocalVolumeSet` spec, so
  that I can manage it via GitOps and ensure consistent device exclusion across
  all nodes without manual intervention.

### Goals

- Provide a deny-list mechanism for device names in the `LocalVolumeSet` API.
- Support glob patterns (using Go's `filepath.Match` syntax) for flexible
  matching of device kernel names.
- Ensure the exclusion filter integrates seamlessly with existing inclusion
  filters and device discovery logic.

### Non-Goals

- Replacing or deprecating the existing `DeviceInclusionSpec` allow-list
  mechanism.
- Adding exclusion filters based on device properties other than the kernel
  name (size, vendor, model, etc.) in this enhancement. These may be added in
  the future.
- Providing regex-based matching; only glob patterns are supported.

## Proposal

A new optional struct `DeviceExclusionSpec` is added to `LocalVolumeSetSpec`.
This struct contains a single field, `DeviceNameFilter`, which is a list of
glob patterns. During device discovery and reconciliation, after the existing
inclusion filters and matchers have been evaluated, each candidate device's
kernel name (`KName`) is checked against the exclusion patterns. If any pattern
matches, the device is excluded from provisioning.

The glob matching uses Go's `filepath.Match` semantics: `*` matches any
sequence of non-separator characters and `?` matches any single non-separator
character.

### Workflow Description

**cluster administrator** is a human user responsible for configuring local
storage on the cluster.

1. The cluster administrator creates or updates a `LocalVolumeSet` custom
   resource, adding a `deviceExclusionSpec` section with one or more
   `deviceNameFilter` glob patterns. For example:

   ```yaml
   apiVersion: local.storage.openshift.io/v1alpha1
   kind: LocalVolumeSet
   metadata:
     name: local-disks
     namespace: openshift-local-storage
   spec:
     storageClassName: local-sc
     volumeMode: Block
     deviceInclusionSpec:
       deviceTypes:
         - disk
     deviceExclusionSpec:
       deviceNameFilter:
         - "rbd*"
         - "nbd*"
   ```

2. The Local Storage Operator reconciles the `LocalVolumeSet`. The diskmaker
   DaemonSet running on each node discovers available block devices.

3. For each discovered device, the existing inclusion filters (device type,
   size, mechanical properties, vendor, model) are evaluated first. Devices
   that pass inclusion filters are then checked against the exclusion filters.

4. If a device's kernel name matches any of the `deviceNameFilter` patterns,
   the device is excluded and not provisioned. An informational log entry is
   emitted.

5. Devices that pass both inclusion and exclusion filters are provisioned as
   PersistentVolumes as usual.

#### Error Handling

If a `deviceNameFilter` pattern is syntactically invalid (per
`filepath.Match`), an error is logged for that device and the device is skipped
(not provisioned). This fail-closed behavior prevents accidental provisioning of
devices that were intended to be excluded.

### API Extensions

This enhancement modifies the `LocalVolumeSet` CRD
(`local.storage.openshift.io/v1alpha1`):

- A new struct `DeviceExclusionSpec` is added with a single field:
  - `deviceNameFilter` (`[]string`, optional, max 64 items): a list of glob
    patterns to match against device kernel names.
- A new optional field `deviceExclusionSpec` of type `*DeviceExclusionSpec` is
  added to `LocalVolumeSetSpec`.

No other API resources are modified. The CRD is owned by the Local Storage
Operator team.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This change has no impact on HyperShift or hosted control planes. The Local
Storage Operator runs on the guest cluster nodes and the `LocalVolumeSet` CRD
is a guest-cluster resource. No management-cluster components are affected.

#### Standalone Clusters

This change is fully relevant for standalone clusters. It is the primary
deployment model for the Local Storage Operator.

#### Single-node Deployments or MicroShift

This change adds a negligible amount of CPU and memory overhead (one additional
glob match per device per reconciliation cycle). There is no impact on
single-node OpenShift resource consumption.

This change does not affect MicroShift. The Local Storage Operator is not part
of the MicroShift platform.

#### OpenShift Kubernetes Engine

This feature does not depend on any features excluded from the OpenShift
Kubernetes Engine (OKE) product offering. The Local Storage Operator and its
`LocalVolumeSet` CRD are available in OKE. No additional considerations apply.

### Implementation Details/Notes/Constraints

The implementation adds:

1. **API types** (`api/v1alpha1/localvolumeset_types.go`): A new
   `DeviceExclusionSpec` struct with a `DeviceNameFilter` field, and a new
   optional `DeviceExclusionSpec` field in `LocalVolumeSetSpec`.

2. **Exclusion matcher** (`pkg/diskmaker/controllers/lvset/matcher.go`): A new
   `exclusionMap` analogous to the existing `matcherMap`. It contains a single
   matcher `notInDeviceNameFilter` that iterates over the glob patterns and
   returns `false` (device excluded) if any pattern matches the device's
   `KName`.

3. **Reconciliation loop** (`pkg/diskmaker/controllers/lvset/reconcile.go`):
   After evaluating inclusion matchers, the reconciler iterates over the
   `exclusionMap`. If any exclusion matcher returns `false`, the device is
   skipped.

4. **CRD manifest**
   (`config/manifests/stable/local.storage.openshift.io_localvolumesets.yaml`):
   Updated to include the new `deviceExclusionSpec` field with a `maxItems: 64`
   validation on `deviceNameFilter`.

5. **Unit tests** (`pkg/diskmaker/controllers/lvset/matcher_test.go`):
   Comprehensive tests covering nil spec, empty filter list, glob matches,
   exact matches, multi-pattern lists, single-character wildcards, and invalid
   pattern error handling.

### Risks and Mitigations

**Risk:** An administrator specifies an overly broad pattern (e.g. `*`) that
excludes all devices.
**Mitigation:** This is a user configuration choice. The behavior is analogous
to specifying an overly restrictive `DeviceInclusionSpec`. Administrators are
expected to test their configuration. Informational log messages indicate which
devices are excluded and why.

**Risk:** Glob pattern syntax differences from user expectations (e.g. users
expecting regex).
**Mitigation:** The API documentation clearly states that `filepath.Match`
syntax is used and provides examples. The `maxItems: 64` validation prevents
excessively long filter lists.

### Drawbacks

- Adds a new API field that partially overlaps in purpose with the existing
  `DeviceInclusionSpec`. However, exclusion-based filtering solves a
  fundamentally different use case (deny-listing) that cannot be easily
  expressed with inclusion filters alone. The two mechanisms are complementary.

## Open Questions [optional]

None.

## Test Plan

Unit tests are included in the implementation covering all matcher logic:
- Nil and empty exclusion spec (no devices excluded).
- Single and multiple glob pattern matches.
- Exact name matches.
- Single-character wildcard (`?`) matching.
- Non-matching patterns (device passes through).
- Invalid glob pattern error handling (device is skipped, error is logged).

Integration and E2E testing will verify:
- Creating a `LocalVolumeSet` with `deviceExclusionSpec` succeeds.
- Devices matching exclusion patterns are not provisioned.
- Devices not matching exclusion patterns are provisioned normally.
- The exclusion filter works correctly in combination with `DeviceInclusionSpec`.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Ability to create `LocalVolumeSet` resources with `deviceExclusionSpec`.
- Unit and E2E test coverage for the exclusion filter.
- End user documentation for the new field.

### Tech Preview -> GA

- Sufficient time for user feedback.
- Stable API with no breaking changes.
- User-facing documentation in [openshift-docs](https://github.com/openshift/openshift-docs/).
- Upgrade and downgrade testing.

### Removing a deprecated feature

Not applicable.

## Upgrade / Downgrade Strategy

**Upgrade:** The new `deviceExclusionSpec` field is optional. Existing
`LocalVolumeSet` resources without this field continue to work unchanged. No
action is required on upgrade. The CRD update adds the new field
non-destructively.

**Downgrade:** If the cluster is downgraded to a version that does not
recognize the `deviceExclusionSpec` field, the field is ignored by the older
operator. Devices that were previously excluded by name will be subject to
provisioning under the older version's logic (inclusion filters only). This is
the expected behavior and administrators should be aware of it.

The operator remains available during upgrades and does not introduce any
voluntary disruption.

## Version Skew Strategy

The `deviceExclusionSpec` field is evaluated entirely by the diskmaker
DaemonSet, which runs on each node. There is no cross-component coordination
required. During a rolling upgrade, nodes running the new version apply
exclusion filters while nodes running the old version do not. This is acceptable
because device provisioning is node-local and independent.

## Operational Aspects of API Extensions

- The `LocalVolumeSet` CRD is extended with a new optional field. No new CRDs,
  webhooks, or aggregated API servers are introduced.
- The new field has `maxItems: 64` validation to bound the number of glob
  patterns.
- Expected usage involves a small number of patterns (typically 1-5). This has
  no measurable impact on API throughput or scalability.
- The CRD is an existing resource managed by the Local Storage Operator. No new
  SLIs are introduced.

### Failure Modes

- **Invalid glob pattern:** The exclusion matcher logs an error and skips the
  device (fail-closed). The operator continues to process remaining devices.
  This does not affect cluster health.

### Escalation

In case of issues with device exclusion behavior, the Local Storage Operator
team (OCP Storage) should be contacted.

## Support Procedures

- **Detecting issues:** If devices that should be excluded are being
  provisioned, check the diskmaker DaemonSet pod logs on the affected node for
  "exclusion match" or "exclusion error" log entries. Verify the
  `deviceExclusionSpec` in the `LocalVolumeSet` resource.

- **Disabling the extension:** Remove the `deviceExclusionSpec` field from the
  `LocalVolumeSet` resource. This reverts to inclusion-only filtering. There
  are no adverse consequences for existing workloads; only newly discovered
  devices will be affected.

- **Graceful failure:** The feature fails gracefully. If the exclusion spec is
  removed or the operator is downgraded, device provisioning continues with
  inclusion filters only. No data loss or inconsistency occurs.

## Alternatives (Not Implemented)

### Using DeviceInclusionSpec vendor/model filters

Administrators could attempt to use vendor or model filters in the existing
`DeviceInclusionSpec` to exclude unwanted devices. However, virtual block
devices (such as RBD or NBD) may not expose meaningful vendor or model
information, making this approach unreliable. Additionally, inclusion-based
filtering requires enumerating all *desired* device properties, which is
impractical in heterogeneous environments where the goal is simply to exclude a
known set of problematic device names.

### Adding device name matching to DeviceInclusionSpec

The exclusion filter could have been implemented as a `deviceNameFilter` within
the existing `DeviceInclusionSpec`. However, this would conflate allow-list and
deny-list semantics in a single struct, making the API harder to understand. A
separate `DeviceExclusionSpec` clearly communicates the deny-list intent and
leaves room for future exclusion criteria without overloading the inclusion
spec.

## Infrastructure Needed [optional]

None.
