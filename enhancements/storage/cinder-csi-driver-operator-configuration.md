---
title: cinder-csi-driver-operator-configuration
authors:
  - "@stephenfin"
  - "@mdbooth"
reviewers:
  - "@jsafrane"
  - "@hekumar"
  - "@chuffman"
  - "@fbertina"
approvers:
  - "@jsafrane"
  - "@hekumar"
  - "@chuffman"
  - "@fbertina"
api-approvers:
  - None
creation-date: 2022-05-03
last-updated: 2022-09-27
tracking-link:
  - https://issues.redhat.com/browse/OSASINFRA-2857
see-also:
replaces:
superseded-by:
---

# OpenStack Cinder CSI Driver Operator Configurability

## Summary

The OpenStack Cinder CSI Driver Operator is responsible for deploying and
configuring the Cinder CSI Driver. Currently, there is no way for an end user
to configure this. This is a regression from the in-tree cinder block storage
driver, which could be configured using the user-managed config map
`cloud-provider-config` in `openshift-config` (note: the configuration for the
legacy and external block storage services are similar, but not identical). In
4.11, we enabled CSI migration by default for the OpenStack platform while in
4.12 we will switch the OpenStack clusters from the legacy cloud provider to
the external Cloud Controller Manager (CCM).

This enhancement will describe how we can ensure an end user can configure the
Cinder CSI driver using the same user-managed config map they would have used
for the legacy block storage service.

## Motivation

We wish to avoid regressions when users switch from the legacy, in-tree block
storage service to the Cinder CSI driver.

### User Stories

#### Story 1 - Fresh install

The Tyrell Corporation wants to deploy OCP 4.12 on their OSP cloud. OCP
automatically configures the Cinder CSI driver out-of-the-box with
configuration native to this CSI driver.

#### Story 2 - Upgrade with default configuration

The Wallace Corporation wants to upgrade their OCP 4.11 cluster to OCP 4.12.
They are using the default cloud provider configuration. Upon upgrading, their
cluster should automatically configure the Cinder CSI driver following the
upgrade, and the upgrade should require no additional user input.

#### Story 3 - Upgrade with non-default compatible configuration

The Acme Corporation wants to upgrade their OCP 4.11 cluster to OCP 4.12.
They have configured `openshift-config/cloud-provider-config` to contain:

```ini
[Global]
secret-name = openstack-credentials
secret-namespace = kube-system

[BlockStorage]
trust-device-path = true
```

After upgrading their cluster will automatically configure the Cinder CSI
driver with no additional user input required. After the upgrade, their cluster
should continue to function with legacy configuration options removed.
Specifically, Cinder CSI driver will be using the following configuration:

```ini
[Global]
use-clouds=true
clouds-file = /etc/kubernetes/secret/clouds.yaml
cloud=openstack
```

[cloud-provider-options]: https://docs.openshift.com/container-platform/4.9/installing/installing_openstack/installing-openstack-installer-custom.html#installation-osp-setting-cloud-provider-options_installing-openstack-installer-custom

### Goals

- No user action is required in the common case
- Upgrade legacy block storage service configuration to configuration that is
  compatible with the Cinder CSI driver, where possible
- Mark cluster as non-upgradable where upgrade of configuration is not possible
- Maintain the interface for configuring the block storage service initially,
  via the `openshift-config/cloud-provider-config` config map.

### Non-Goals

- Modify user-managed configuration
- Manage CSI configurations for other, non-OpenStack platforms
- Provide a config map for configuring the Cinder CSI driver that is separate
  from the cloud provider. This will be done later.

## Proposal

- The change will be implemented in the OpenStack Cinder CSI Driver Operator
  (OCCDO)
- OCCDO will create a separate config map for the Cinder CSI driver
- We will prevent upgrade if it is not possible to create the Cinder CSI driver
  config map

### Workflow Description

#### Implemented in the OpenStack Cinder CSI Driver Operator (OCCDO)

The [Cluster Config Operator (CCO)][CCO] is currently responsible for managing
the config for the legacy cloud provider, which means it is also responsible
for managing the configuration for the legacy block storage service. This
applies a [transformation function][transformation-function] to the
user-managed config and writes the result to
`openshift-config-managed/kube-cloud-config`. The transformation function is
selected according to PlatformType in the cluster Infrastructure object.
Components requiring legacy cloud provider configuration, including kubelet and
KCM, read the transformed config map.

CCO will **not** be modified. The new functionality will instead be implemented
as a change to the [OpenStack Cinder CSI Driver Operator (OCCDO)][OCCDO],
functioning in much the same way as CCO's management of the legacy block
storage service's configuration.

[CCO]: https://docs.openshift.com/container-platform/4.9/operators/operator-reference.html#cluster-config-operator_platform-operators-ref
[transformation-function]: https://github.com/openshift/cluster-config-operator/tree/master/pkg/operator/kube_cloud_config
[OCCDO]: https://docs.openshift.com/container-platform/4.9/storage/container_storage_interface/persistent-storage-csi-cinder.html

#### Separate ConfigMaps for legacy cloud provider and Cinder CSI driver

The cloud configuration is read by multiple components which will not all be
upgraded simultaneously. If we simply updated the
`openshift-config-managed/kube-cloud-config` to be compatible with the Cinder
CSI driver, we would risk breaking KCM and kubelets which have not yet been
upgraded. Therefore we are constrained to write two versions of the managed
config map.

CCO will be continue to create `openshift-config-managed/kube-cloud-config`,
which will remain compatible with the legacy cloud provider (and by extension,
the legacy block storage service). OCCDO will create a new
`openshift-cluster-csi-drivers/cloud-conf` config map, which will be compatible
with the Cinder CSI driver. The Cinder CSI driver will then read its
configuration from `openshift-cluster-csi-drivers/cloud-conf`.

In a future release, we can deprecate and remove CCO's management of
`openshift-config-managed/kube-cloud-config` and eventually the config map
itself. We can also remove the `openstack-credentials/openstack-credentials`
secret, which will no longer be used by anything. This effort will be
accomplished as part of the migration to CCM.

#### Preventing upgrade when config cannot be upgraded

If OCCDO is not able to generate `openshift-cluster-csi-drivers/cloud-conf` from
`openshift-config/cloud-provider-config`, it will mark itself as non-upgradable
with a user-actionable error message explaining how
`openshift-config/cloud-provider-config` is incompatible.

### API Extensions

N/A

### Implementation Details/Notes/Constraints [optional]

N/A

### Risks and Mitigations

**Risk** Identical config options could work differently between the legacy
Cinder block device service and Cinder CSI, or the conversion could be lossy.

**Mitigation** Auditing and testing of the configuration options and
combinations of same. At a minimum, we will explicitly test every legacy config
option listed in the user documentation.

**Risk** There may be additional configuration required for the Cinder CSI
driver compared to the legacy Cinder Block Storage service in certain cloud
configurations.

**Mitigation** We currently test multiple cloud configurations. We should test
an upgraded cluster on each of these cloud configurations.

### Drawbacks

- The continued presence of `openshift-config-managed/kube-cloud-config` is
  irritating as no one should be using this anymore. This will have to be
  deprecated at some point in the future.

## Design Details

### Configuration transformation for Cinder CSI Driver

The configuration of the Cinder CSI driver is a superset of the legacy Cinder
block storage service configuration, with five exceptions. Details of how we
will handle the five removed configuration directives are provided below.

<dl>
<dt>secret-name</dt>
<dt>secret-namespace</dt>
<dt>kubeconfig-path</dt>
<dd>
  These are no longer relevant or supported in the Cinder CSI driver. They
  define the cloud credentials that are used by the legacy cloud provider.
  OCCDO will automatically replace these with static config pointing to the
  cluster-managed cloud credentials.
  <br>
  It is possible that the secret contains more information that the cloud
  credentials populated by the installer. If so, the updated configuration will
  no longer include these manual edits. Adding additional information to the
  cloud credentials secret is not supported as we do not test or document it.
  <br>
  We estimate that the false positive risk of attempting to validate all
  possible OpenStack authentication configurations is significantly higher than
  the likelihood of any user actually having done this. We therefore judge that
  it is safer not to attempt to handle this situation explicitly, and simply
  constrain the user to modify their configuration to use the cluster cloud
  credentials. If any user had put non-credential information in the credentials
  secret, this would also be a prompt to them to fix that.
</dd>
<dt>bs-version</dt>
<dt>trust-device-path</dt>
<dd>
  These options are not supported by the Cinder CSI driver and will therefore
  cause parse issues if included in configuration. These are unnecessary knobs
  and we will simply drop them when upgrading this configuration.
</dd>
</dl>

We will always add `use-clouds=true` and `cloud="openstack"` in the generated
`openshift-cluster-csi-drivers/cloud-conf` config map.  This will refer to the
cluster-managed cloud credentials. If the user specifies these values in the
user-managed config map, they will be ignored and overridden.

If the user specifies configuration that would not parse after transformation,
we will mark OCCDO as `Upgradable=False`, and will not update the transformed
configuration.

### Open Questions [optional]

N/A

### Test Plan

This enhancement is an essential feature for the migration to external CCM as
the Cinder CSI driver must be used with this. We cannot continue to provide all
existing functionality without the ability to customise the Cinder CSI
configuration. This feature does not directly provide any new user-facing
functionality; its purpose is to preserve existing functionality when migrating
to the Cinder CSI driver. Therefore the primary validation of this feature will
be that our existing test coverage does not regress when upgrading to external
CCM and switching the Cinder CSI driver.

We will ensure full coverage of edge cases which are impractical to test on real
hardware through unit testing.

#### Existing OpenStack test coverage

QE already regularly runs a full test suite against a matrix of the following
test configurations:

- Networking:
  - OpenshiftSDN
  - OVNKubernetes
  - Kuryr
- Installation:
  - IPI
  - IPI with Proxy
  - UPI
- OpenStack version:
  - OSP 16.1
  - OSP 16.2

Specifically, they test:

- conformance/parallel
- cinder-csi
- manilla
- openstack-test

The test configurations already cover the full range of documented
configurations changes, as can be seen in the [test code][cloud-cm-gerrit].

[cloud-cm-gerrit]: https://code.engineering.redhat.com/gerrit/gitweb?p=openshift-ir-plugin.git;a=blob;f=roles/tests/tasks/change_cloud_cm.yml;h=33ee741b1ef9c45c0417b911d234b4d3d83e68e2;hb=HEAD

### Graduation Criteria

#### Dev Preview -> Tech Preview

N/A. This feature is part of the Cinder CSI driver, which is already GA.

#### Tech Preview -> GA

N/A. This feature is part of the Cinder CSI driver, which is already GA.

#### Removing a deprecated feature

N/A. This is a new feature.

### Upgrade / Downgrade Strategy

N/A. Downgrade from Cinder CSI is not supported.

### Version Skew Strategy

Version skew is handled by maintaining the legacy configuration while writing
the new configuration to a new config map.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

In general, failure of OCCDO will not have a critical impact on the cluster
unless the user has also manually deleted the config maps it creates in
`openshift-cluster-csi-drivers`.

A failure of OCCDO will cause updates to user-managed config to not be reflected
in the cluster's deployed configuration. However, the cluster will continue to
use the previous configuration until the failure of OCCDO is resolved.

#### Support Procedures

If OCCDO has failed it will be evident because it will be marked as
`Upgradable=False`.

## Implementation History

- [2022-05-03] Initial enhancement proposal

## Alternatives

N/A

## Infrastructure Needed [optional]

N/A
