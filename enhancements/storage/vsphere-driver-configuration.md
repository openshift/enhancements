---
title: vsphere-driver-configuration
authors:
  - "@rbednar"
reviewers:
  - "@jsafrane"
  - "@gnufied"
  - "@deads2k"
approvers:
  - "@jsafrane"
  - "@gnufied"
  - "@deads2k"
api-approvers:
  - "@deads2k"
creation-date: 2024-02-09
last-updated: 2024-02-09
tracking-link:
  - https://issues.redhat.com/browse/OCPSTRAT-1094
see-also:
  - "None"
replaces:
  - "None"
superseded-by:
  - "None"
---

# vSphere driver configuration

## Summary

The vSphere driver configuration enhancement aims to provide a way to configure the vSphere driver.
The vSphere driver is a CSI driver that allows to use vSphere storage in OpenShift and is deployed by vSphere driver
operator that CSO deploys on vSphere clusters.

Currently, the driver can be configured via a configuration file (`csi_cloud_config.ini`) that we mount into a known path
inside driver controller pods (`/etc/kubernetes/vsphere-csi-config/cloud.conf`) as a ConfigMap. This file is reconciled
by the driver operator. This means that we currently don't allow users to modify the configuration file and need a way to do so.

## Motivation

There is plenty of configuration options that can be set in the configuration file. Some of them are required to be set
and some of them are optional. In this particular moment we are interested in allowing users to set the maximum limit
of snapshots that can be created per volume but also consider a more generic approach that would allow users to set
any configuration option.

Current maximum limit of snapshots per volume is set to 3 by default and if users create 4th snapshot the VolumeSnapshot
will fail with an error: `the number of snapshots on the source volume XYZ reaches the configured maximum (3)`.

The reason we are interested in a more generic approach is that we expect to receive more requests for allowing users to
configure other options in the future - even other CSI drivers. Currently, we're aware of the following RFEs:

- AWS EFS CSI usage metrics: https://issues.redhat.com/browse/RFE-3290
- OpenStack topology: https://issues.redhat.com/browse/RFE-11

### User Stories

- As a cluster administrator, I want to be able to configure the maximum limit of snapshots that can be created per volume
  so that I can adjust it to my needs.
- As a cluster administrator, I want to be able to configure any parameter supported by CSI driver.

### Goals

- Allow users to configure the maximum limit of snapshots that can be created per volume for vSphere CSI driver.
- Allow users to configure any parameter supported by CSI driver.
- Provide a safe way of configuring any CSI driver while being able to drop any of the parameters in case upstream removes them.

### Non-Goals

- Make things more complicated and easy to break.
- Allow users to configure feature flags of vSphere CSI Driver.
- Provide a way to configure drivers in the web UI.

## Proposal

We want to add new fields to ClusterCSIDriver CRD where each field would represent a single
configuration option. We will basically mirror selected configuration options from the driver into the CRD.

For enabling the feature we will follow API conventions for adding new fields to an existing CRD as described here:
https://github.com/openshift/enhancements/blob/2894d936eca7c1fcc9cf38e7dc973bbdfa1d88ff/dev-guide/api-conventions.md#new-field-added-to-an-existing-crd

Example:

1. Cluster administrator creates a ClusterCSIDriver object with the maximum limit of snapshots set to 10 using a
   specific integer field (`clustercsidriver.spec.driverConfig.vSphereConfig.GlobalMaxSnapshotsPerBlockVolume`).
2. Additionally, they can configure two other granular options related to snapshot limits for vSAN and vVOL supported by 
the driver. Administrator does not have to deal with configuring the driver directly.

Configuration mirroring could look like this:

| Fields set by administrator                  | Configuration options provided to driver         |
|----------------------------------------------|--------------------------------------------------|
| GlobalMaxSnapshotsPerBlockVolume = 10        | global-max-snapshots-per-block-volume = 10       |
| GranularMaxSnapshotsPerBlockVolumeInVSAN = 5 | granular-max-snapshots-per-block-volume-vsan = 5 |
| GranularMaxSnapshotsPerBlockVolumeInVVOL = 1 | granular-max-snapshots-per-block-volume-vvol = 1 |

3. The driver controller pods are restarted and new configuration loaded.
4. Users can now create VolumeSnapshot 10 times per volume but only 5 times for VSAN and only once for VVOL.

### Workflow Description

See example in the [Proposal](#Proposal) for details.

#### Variation and form factor considerations [optional]

None.

### API Extensions

Suggested solution requires changes in the ClusterCSIDriver API so that a new field or fields are added to the
spec. No other changes or new CRDs are required.

### Implementation Details/Notes/Constraints [optional]

None. See the [Proposal](#Proposal) for details.

#### Hypershift [optional]

No specifics are required for Hypershift - users already have access to ClusterCSIDriver CR in guest cluster and can modify it.

### Risks and Mitigations

The approach described in the [Proposal](#Proposal) section presents a vulnerability regarding feature removal.

CSI drivers have distinct development and feature removal processes, often separate from the Kubernetes project.
Compared to the rigorous processes followed by the Kubernetes project, CSI driver development and feature removal might
be less controlled.

This implies that even with separate CRD fields, upstream CSI driver could suddenly remove a supported feature, leaving
us unable to simply remove the corresponding field from our releases without breaking compatibility.

We propose releasing the feature initially to be enabled by default as described here (no TechPreview):
https://github.com/openshift/api/blob/6a96294dcfb7587088c5144fcc4c2ef22a6cf063/README.md?plain=1#L45-L53

### Drawbacks

Current implementation is very limiting and drivers offer many configuration options that are useful for users, and we
currently don't allow using them. This is a drawback for users, in the future we expect to get more requests to add
more configuration options.

## Design Details

### Open Questions [optional]

None.

### Test Plan

TBD - depends on chosen solution.

### Graduation Criteria

| OpenShift | Maturity |
|-----------|----------|
| 4.16      | GA       |


#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation
- Sufficient test coverage
- Gather feedback from users

#### Tech Preview -> GA

- Tech Preview was available for at least one release
- High severity bugs are fixed
- Reliable CI signal, minimal test flakes.
- e2e tests are in place

#### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

Feature will not affect standard upgrade or downgrade process of the cluster.

While upgrading to a release with the feature enabled, the driver operator will start reconciling the new field and propagate
the configuration to correct destination, no issues are expected.

Downgrades should be unsupported as any configuration used while the feature was enabled could result in unpredictable
behavior specific to each configuration option. For example if users set the maximum limit of snapshots to 10, then
create 5 snapshots followed by downgrade, the operator would remove any configuration associated with snapshot limits
and driver would start rejecting any new snapshots being created due default snapshot limit which is currently 3.

### Version Skew Strategy

Version skew should not present any issues, if the feature is disabled the driver operator will not reconcile any new
field or fields and driver will apply defaults, this is the current behaviour for most of the options in vSphere driver.

Opposite the scenario where feature is enabled and new fields present in ClusterCSIDriver the driver would either ignore
any unsupported configuration options or fail to load the configuration - this depends on specific implementation of the
driver upstream.

### Operational Aspects of API Extensions

The `ClusterCSIDriver` type is used for operator installation and reporting any errors.
Please see [CSI driver installation](csi-driver-install.md) for more details.

#### Failure Modes

- Any unexpected or unsupported configuration option is provided by users, we can mitigate this by validating the
configuration and warn users if it happens. This could be in console, alerts, logs or must-gather.
- If configuration option is removed upstream, we can mitigate this by removing the field from the CRD and warn users
- Unexpected or sudden removal of configuration option upstream could result in SLA breaches.
- If options are not validated properly it could result in operator failure and cluster degradation.


#### Support Procedures

Inspect must-gather, driver and operator logs. If the issue is related to the configuration, check the ClusterCSIDriver.

## Implementation History

- 2024-02-09: Initial draft

## Alternatives

#### Alternative 1: Add a specific API field for the maximum limit of snapshots

This solution would require adding a new field in the ClusterCSIDriver CRD that would hold the maximum limit of snapshots
and follow the usual API review process. A considerable downside is that there could be other related configuration
options for a CSI driver - for example in vSphere, the maximum number of volumes that can be created per volume can be
configured in 3 different ways: `global-max-snapshots-per-block-volume`, `granular-max-snapshots-per-block-volume-vsan`,
`granular-max-snapshots-per-block-volume-vvol`. In this case we would pick only the first one which is a global setting and
overrides the other two.

Example:

1. Cluster administrator sets integer value of 10 in `clustercsidriver.spec.driverConfig.vSphereConfig.snapshotLimit`
2. The driver operator reconciles the ClusterCSIDriver object and propagates the option to a valid destination. This can
   be a config ini file of the driver that will be updated to contain `global-max-snapshots-per-block-volume = 10` option
   under `[Snapshot]` stanza.
3. The driver controller pods are restarted and new configuration loaded.
4. Users can now create VolumeSnapshot 10 times per volume, limit for VVOL and VSAN is also 10 and can not be changed by
   the administrator.

#### Alternative 2: Configuration via generic map in a dedicated field of ClusterCSIDriver

This solution would require having a new field in the ClusterCSIDriver CRD that would hold the all configuration options
as a map (`map[string]string`), something like `clustercsidriver.spec.vSphere.configParameters`. Then we would merge it
into any other valid destination for the driver - command line arguments, environment variables, ini file, driver
feature gate ConfigMap, etc.

We would also have a separate field for holding feature status also as a map (`map[string]string`), something like
`clustercsidriver.spec.vSphere.featureFlags`. Implementation would be similar to the `configParameters` field for
configuration options.

For both of these fields we would internally maintain whitelists (in vSphere operator) of values that can be applied,
while ignoring any other unknown values providing a visible warning to users. This could be in console, alerts, logs or
must-gather.

Alternatively the configurations could be provided as a ConfigMap directly and the driver operator would reconcile it and
ensure the options are propagated to valid destinations.

Example:

1. Cluster administrator creates a ClusterCSIDriver object with the configuration map set to `{"maxSnapshots": "10"}`
   under a generic `map[string]string` field (`clustercsidriver.spec.vSphere.configuration`)
2. Additionally, they can set any other option that we document for the operator like `{"maxSnapshotsVSAN": "5"}`
3. The driver controller pods are restarted and new configuration loaded.
4. Users can now create VolumeSnapshot 10 times per volume but only 5 times for VSAN.

## Infrastructure Needed [optional]

None.
