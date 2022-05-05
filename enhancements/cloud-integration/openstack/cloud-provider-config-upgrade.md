---
title: cloud-provider-config-upgrade
authors:
  - "@stephenfin"
  - "@mdbooth"
reviewers:
  - "@jspeed"
  - "@mandre"
approvers:
  - "@jspeed"
api-approvers:
  - None
creation-date: 2022-01-18
last-updated: 2022-05-03
tracking-link:
  - https://issues.redhat.com/browse/OSASINFRA-2758
see-also:
replaces:
superseded-by:
---

# OpenStack Cloud Provider Config Upgrade

## Summary

Cloud configuration is stored in the user-managed config map
`cloud-provider-config` in `openshift-config`.  In 4.11 we want to switch
OpenStack clusters from the legacy cloud provider to the external Cloud
Controller Manager (CCM). The configuration for the legacy and external cloud
providers are similar, but not identical.

This enhancement will describe how we can ensure correct configuration is passed
to the cloud provider in use during and after an upgrade, and how we can prevent
an upgrade to 4.11 if the configuration cannot be upgraded.

## Motivation

The legacy cloud provider is dead. Long live the external cloud provider.

### Goals

- No user action is required in the common case
- Upgrade legacy cloud provider to external cloud controller manager where possible
- Mark cluster as non-upgradable where upgrade to external CCM is not possible
- Maintain the interface for configuring the cloud provider, via the
  `openshift-config/cloud-provider-config` config map.

### Non-Goals

- Modify user-managed configuration
- Manage cloud providers for other, non-OpenStack platforms

## Proposal

- The change will be implemented in the Cluster Cloud Controller Manager
  Operator (CCCMO)
- CCCMO will create a separate config map for the external cloud provider
- We will prevent upgrade if it is not possible to create the external cloud
  provider config map

### Implemented in the Cluster Cloud Controller Manager Operator (CCCMO)

The [Cluster Config Operator (CCO)][CCO] is currently responsible for managing
the config for the legacy cloud provider. This applies a [transformation
function][transformation-function] to the user-managed config and writes the
result to `openshift-config-managed/kube-cloud-config`. The transformation
function is selected according to PlatformType in the cluster Infrastructure
object. Components requiring legacy cloud provider configuration, including
kubelet and KCM, read the transformed config map.

CCO will **not** be modified. The new functionality will instead be implemented
as a change to the [Cluster Cloud Controller Manager Operator (CCCMO)][CCCMO],
functioning in much the same way as CCO's management of the legacy
configuration.

[CCO]: https://docs.openshift.com/container-platform/4.9/operators/operator-reference.html#cluster-config-operator_platform-operators-ref
[transformation-function]: https://github.com/openshift/cluster-config-operator/tree/master/pkg/operator/kube_cloud_config
[CCCMO]: https://docs.openshift.com/container-platform/4.9/operators/operator-reference.html#cluster-cloud-controller-manager-operator_platform-operators-ref

### Separate ConfigMaps for legacy and external cloud providers

The cloud configuration is read by multiple components which will not all be
upgraded simultaneously. If we simply updated the
`openshift-config-managed/kube-cloud-config` to be compatible with external CCM
we would risk breaking KCM and kubelets which have not yet been upgraded.
Therefore we are constrained to write two versions of the managed config map.

CCO will be continue to create `openshift-config-managed/kube-cloud-config`,
which will remain compatible with the legacy cloud provider. CCCMO will create a
new `openshift-config-managed/cloud-controller-manager-config` config map, which
will be compatible with the external CCM. The external CCM will then read its
configuration from `openshift-cloud-controller-manager/cloud-conf`.

In a future release, we can deprecate and remove CCO's management of
`openshift-config-managed/kube-cloud-config` and eventually the config map
itself. We can also remove the `openstack-credentials/openstack-credentials`
secret, which will no longer be used by anything. We will immediately deprecate
this in the 4.11 release and remove it in the 4.12 release.

### Preventing upgrade when config cannot be upgraded

If CCCMO is not able to generate `cloud-controller-manager-config` from
`openshift-config/cloud-provider-config`, it will mark itself as non-upgradable
with a user-actionable error message explaining how
`openshift-config/cloud-provider-config` is incompatible.

As we intend to switch to external CCM in 4.11, this change will be implemented
in OCP 4.10.

### Compatibility constraints on user-managed config

This has implications for the user-managed configuration:

In 4.10, the user-managed configuration **must** be compatible with the legacy
cloud provider. The user **must not** specify config options which cannot be
transformed to config for the external cloud provider. If either of these are
not satisfied we will mark CCCMO as not upgradeable.

In 4.11, the base `cloud-provider-config` **may** include config options which
are not compatible with the legacy cloud provider. We will have removed the
legacy cloud provider and related config map so we no longer need to be
concerned with this.

### User Stories

#### Story 1 - Fresh install

The Tyrell Corporation wants to deploy OCP 4.11 on their OSP cloud. OCP
automatically configures external CCM out-of-the-box with configuration native
to external OpenStack CCM.

#### Story 2 - Upgrade with default configuration

The Wallace Corporation wants to upgrade their OCP 4.10 cluster to OCP 4.11.
They are using the default cloud provider configuration. Upon upgrading, their
cluster should automatically use CCM following the upgrade, and the upgrade
should require no additional user input.

#### Story 3 - Upgrade with non-default compatible configuration

The Acme Corporation wants to upgrade their OCP 4.10 cluster to OCP 4.11.
Following the user documentation [here][cloud-provider-options], they have
configured a specific floating network ID.
`openshift-config/cloud-provider-config` contains:

```ini
[Global]
secret-name = openstack-credentials
secret-namespace = kube-system

[LoadBalancer]
use-octavia=true
lb-provider = "amphora"
floating-network-id="d3deb660-4190-40a3-91f1-37326fe6ec4a"
```

After upgrading their cluster will automatically use CCM with no additional user
input required. After the upgrade, their cluster should continue to use the same
floating network ID. Specifically, CCM is using the following configuration:

```ini
[Global]
use-clouds=true
clouds-file = /etc/openstack/secret/clouds.yaml
cloud=openstack

[LoadBalancer]
use-octavia=true
lb-provider = "amphora"
floating-network-id="d3deb660-4190-40a3-91f1-37326fe6ec4a"
```

[cloud-provider-options]: https://docs.openshift.com/container-platform/4.9/installing/installing_openstack/installing-openstack-installer-custom.html#installation-osp-setting-cloud-provider-options_installing-openstack-installer-custom

#### Story 4 - Upgrade with non-default incompatible configuration

Omni Consumer Products wants to upgrade their OCP 4.10 cluster to OCP 4.11.
They have configured a separate secret containing their cloud credentials as
well as additional config directives. Specifically, `openshift-config/cloud-provider-config` contains:

```ini
[Global]
secret-name = dick-jones-openstack-credentials
secret-namespace = ed209-project

...
```

Upon attempting to upgrade, they discover they cannot as the Cluster Config
Operator is marked as non-upgradable. The status of this operator informs them
that they must return `secret-name` and `secret-namespace` to their default
values before they can upgrade.

### API Extensions

N/A

### Implementation Details/Notes/Constraints [optional]

N/A

### Risks and Mitigations

**Risk** Identical config options could work differently between the legacy and
external cloud providers, or the conversion could be lossy.

**Mitigation** Auditing and testing of the configuration options and
combinations of same. At a minimum, we will explicitly test every legacy config
option listed in the user documentation.

**Risk** There may be additional configuration required for CCM compared to the
legacy cloud provider in certain cloud configurations.

**Mitigation** We currently test multiple cloud configurations. We should test
an upgraded cluster on each of these cloud configurations.

### Drawbacks

- The second config map is ugly and will have to be deprecated at some point in
  the future.

## Design Details

### Configuration transformation for OpenStack CCM

The configuration of OpenStack CCM is a superset of the legacy cloud provider
configuration, with four exceptions. There are no features which are supported
by the legacy cloud provider which are not supported by OpenStack CCM. Details
of how we will handle the four removed configuration directives are provided
below.

<dl>
<dt>secret-name</dt>
<dt>secret-namespace</dt>
<dt>kubeconfig-path</dt>
<dd>
  These are no longer relevant or supported in CCM. They define the cloud
  credentials that are used by the legacy cloud provider. CCCMO will
  automatically replace these with static config pointing to the cluster-managed
  cloud credentials.
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
  Along with all block storage options, these are now handled by Cinder CSI.
  Unlike other block storage options, these are the only ones that have been
  removed and therefore are the only ones that will cause parse issues if
  included in configuration. We will drop all block storage options when
  transforming the user-provided configuration. A separate enhancement will
  track configuration of this service.
</dd>
</dl>

We will always add `use-clouds=true` and `cloud="openstack"` in the generated
`cloud-controller-manager-config`.  This will refer to the cluster-managed cloud
credentials. If the user specifies these values in the user-managed config map,
they will be ignored and overridden.

If the user specifies configuration that would not parse after transformation,
we will mark CCCMO as `Upgradable=False`, and will not update the transformed
configuration.

### Open Questions [optional]

1. After the upgrade to 4.11, we should give the user a warning if their
   configuration is being transformed. The user should get a warning if they
   are using static configuration such as `secret-name` and `secret-namespace`
   and should be advised to remove this configuration. They should also see
   warnings if they have any block-storage-related config options defined as
   these are now handled by cinder-csi.

### Test Plan

This enhancement is an essential feature for the migration to external CCM. We
cannot continue to provide all existing functionality without the ability to
customise the CCM configuration. This feature does not directly provide any new
user-facing functionality; its purpose is to preserve existing functionality
when migrating to external CCM. Therefore the primary validation of this feature
will be that our existing test coverage does not regress when upgrading to
external CCM.

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

N/A. This feature is part of CCM, which is already tech preview. It will become
the default for OpenStack-based clusters in 4.11.

#### Tech Preview -> GA

QE's test coverage of TechPreview is limited, but it has been sufficient to
discover this omission. When we enable CCM by default in 4.11 we will be covered
by QE's full test matrix detailed above. This feature will graduate to GA if
these tests are passing.

Additionally:
- We will land the feature as early as possible in the 4.11 development cycle
- User facing documentation is updated to highlight configuration options which
  are deprecated between 4.10 and 4.11
- User facing documentation is updated to show the new configuration

#### Removing a deprecated feature

N/A. This is a new feature.

### Upgrade / Downgrade Strategy

N/A: Downgrade from CCM is not supported

### Version Skew Strategy

Version skew is handled by maintaining the legacy configuration while writing
the new configuration to a new config map. This strategy does not require any
changes to the existing external CCM upgrade workflow: legacy components
continue to read the legacy configuration until they are restarted, and the new
component can read the new configuration at any time.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

In general, failure of CCCMO will not have a critical impact on the cluster
unless the user has also manually deleted the config maps it creates in
`openshift-config-managed`.

A failure of CCCMO will cause updates to user-managed config to not be reflected
in the cluster's deployed configuration. However, the cluster will continue to
use the previous configuration until the failure of CCCMO is resolved.

#### Support Procedures

If CCCMO has failed it will be evident because it will be marked as
`Upgradable=False`. If running 4.10 this will prevent an upgrade to 4.11. If
running 4.11, a valid `cloud-controller-manager-config` will already exist, but
will not be updated to reflect new changes to
`openshift-config/cloud-provider-config`.

It should not be possible to upgrade a cluster to use external CCM without CCCMO
having previously written `cloud-controller-manager-config`. However, if it did
happen, the root cause should be evident because:

- CCCMO will be marked as `Upgradable=False`
- CCCMO will be marked as degraded because CCM cannot start (XXX: verify this)
- Examining the CCM pod will reveal the missing config map

If the user updates `openshift-config/cloud-provider-config` with incompatible
configuration, CCCMO will be marked as `Upgradable=False` and a user-targetted
error will detail the cause.

## Implementation History

- [2022-01-25] Initial enhancement proposal

## Alternatives

- We originally considered reusing `openshift-config-managed/kube-cloud-config`
for CCM, but version skew issues would make the upgrade process more complex and
less robust.

- Instead of using CCCMO, we could write the config map in a sidecar container to
  CCM. This is what Azure is doing.

  This does not give us an opportunity to validate the transformation in 4.10
  and prevent upgrade if required. Additionally, given that CCCMO exists and is
  apparently intended to be used for exactly this purpose, implementing the
  feature elsewhere would violate the principal of least surprise.

- Instead of using CCCMO, we could use CCO.

  CCO is currently managing config for the legacy cloud provider.

- We could create an OpenStack-specific CRD describing all aspects of cloud
  configuration that we expect the user to configure and generate the config
  from the CRD.

  Having previously worked with a system which works this way (namely
  well-documented upstream configuration wrapped in product-specific
  configuration), we are significantly prejudiced against it by design.

  Most significantly for the 4.11 release it would be a large and complex
  user-facing design change. If we were to consider something like this in the
  future, it would make more sense not to try to do it at the same time as the
  change to external CCM.

## Infrastructure Needed [optional]

N/A
