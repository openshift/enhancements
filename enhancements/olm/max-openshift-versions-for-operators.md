---
title: Allow OLM Managed Operators to Specify a Max OpenShift Version
authors:
  - "@awgreene"
reviewers:
  - TBD
  - "@kevinrizza"
approvers:
  - "@ecordell"
  - "@spadgett"
creation-date: 2021-01-11
last-updated: 2021-01-19
status: provisional
see-also:
  -  N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# Allow OLM Managed Operators to Specify a Max OpenShift Version

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Definitions

**OLM Managed Operators**: Operators that are installed, managed, and upgraded by the [Operator Lifecycle Manager (OLM)](https://github.com/operator-framework/operator-lifecycle-manager) project.

**Operator Author**: The team responsible for the development of an operator.

**Index Maintainers**: The tean that maintains a collection of [Operator Bundles](https://olm.operatorframework.io/docs/glossary/#bundle) in the form of an [Index](https://olm.operatorframework.io/docs/glossary/#index).

**Minor OpenShift Upgrade**: When the OpenShift Cluster is upgraded to the next minor version, for example the upgrade from version 4.6 to 4.7. See [Semantic Versioning](https://semver.org/) for more details.

**Patch OpenShift Upgrades**: When the OpenShift Cluster is upgraded to the next patch version, for example the upgrade from version 4.6.1 to 4.6.2. See [Semantic Versioning](https://semver.org/) for more details.

**maxOpenShiftVersion**: The maximum `{major}.{minor}` OpenShift version that an operator supports, example: `4.5`.

## Summary

OpenShift Cluster Admins introduce new services to their clusters by way of OLM Managed Operators. When performing a Minor OpenShift Upgrade, there is no guarantee that these operators will continue to run on the upgraded cluster version.

Many operators shipped by Red Hat are rigorously tested on a specific set of OpenShift versions. The teams responsible for this testing know exactly which `{major}.{minor}` versions of OpenShift that their operators can run on. OLM should allow Operator Authors to specify the maximum `{major}.{minor}` version of OpenShift that their operator may run on.

With this information OLM should:

- Prevent Minor OpenShift Upgrades when it is possible to determine that one or more installed operators will not run on the next minor OpenShift version.
- Prevent the operator from being installed on OpenShift clusters whose version is greater than the `maxOpenShiftVersion` specified by the operator.

## Motivation

The primary purpose of this enhancement is to define how OpenShift can protect existing services introduced by OLM Managed Operators when performing Minor OpenShift Upgrades.

### Goals

- Prevent Minor OpenShift Upgrades when an installed operator declares that it does not support the next minor OpenShift version.
- Warn Cluster Admins when an installed operator does not explicitly declare support for the next minor OpenShift version.
- Prevent OLM from installing operators whose `maxOpenShiftVersion` is less than the current [ClusterVersion](https://github.com/openshift/api/blob/a9e731090f5ed361e5ab887d0ccd55c1db7fc633/config/v1/types_cluster_version.go#L11-L13).

### Non-Goals

- Ensure that services will not be disrupted from a cluster upgrade.
- Guarantee that all installed OLM Managed Operators will continue to run after a Minor OpenShift Upgrade.
- Prevent Patch OpenShift Upgrades.

## Proposal

The [ClusterVersionOperator](https://github.com/openshift/cluster-version-operator) provides OLM with the means to prevent Minor OpenShift Upgrades by setting the `operator-lifecycle-manager's` [ClusterOperator's upgradeable condition](https://github.com/openshift/api/blob/5935a5beec4bb8e1e81dd0fe9ebc2af36b9a09ae/config/v1/types_cluster_operator.go#L171-L175) to `False`.

OLM currently reports that it is upgradeable as soon its deployments are successfully rolled out. OLM should instead set its `upgradeable condition` to reflect if any OLM Managed Operators indicate that they will not run on the next minor OpenShift version.

The bulk of this enhancement focuses on defining:

- How operators will define their `maxOpenShiftVersion`.
- The steps that OLM will take to determine upgrade safety based on the collection of installed operators.
- The steps that OLM will take to prevent the operator from being installed on OpenShift clusters whose version is greater than the `maxOpenShiftVersion` specified by the operator.

### Allowing Operator Authors to Define a Maximum OpenShift Version

Given that this is an OpenShift specific feature, the OLM team would rather not expand the `Spec` of the [ClusterServiceVersion (CSV)](https://olm.operatorframework.io/docs/concepts/crds/clusterserviceversion/), used on vanilla Kubernetes clusters, to include a `maxOpenShiftVersion` field.

Instead, OLM will rely on the presence of an annotation on a CSV to specify the operator's `maxOpenShiftVersion`. This annotation can be added in one of two ways:

1. By defining the `maxOpenShiftVersion` as a property of the bundle using the [Declarative Index Config](https://github.com/operator-framework/enhancements/blob/1cf4a0363d918e810f638036539388622265d466/enhancements/declarative-index-config.md) (recommended approach).
2. By adding the annotation to the CSV directly.

Let's review the benefits associated with defining an operator's `maxOpenShiftVersion` using the Declarative Index Config.

#### Defining the Maximum OpenShift Version using the Declarative Index Config

The Declarative Index Config provides the means to define [indexes](https://olm.operatorframework.io/docs/glossary/#index) in a declarative way. When this feature is available, OLM will allow Index Maintainers to specify each bundles `maxOpenShiftVersion`, an example of which can be seen below:

```json=
"etcd": {
  "name": "etcd",
  "bundles": [
    {
      "path": "quay.io/foo/etcdv0.0.1",
      "version": "v0.0.1",
      "channels": ["alpha"],
      "operators.coreos.com/maxOpenShiftVersion": "4.6", # Prevent upgrades to 4.7
    },
    {
        "path": "quay.io/foo/etcdv0.0.2",
        "version": "v0.0.2",
        "channels": ["alpha"],
        "operators.coreos.com/maxOpenShiftVersion": "4.7", # Prevent upgrades to 4.8
    },
  ],
  ...
  ...
  ...
}
```

Like all bundle properties, these properties will be propagated to the CSV as an annotation. The value of the bundle's `maxOpenShiftVersion` can be easily updated without releasing a new bundle as soon as the existing bundle has been tested against a new version of OpenShift.

#### How OLM Determines Upgradeability Status

When OLM reconciles a CSV it will check the set of annotations for a key that matches `operators.coreos.com/maxOpenShiftVersion`. The value associated with this key specifies the maximum `{major}.{minor}` OpenShift version that the operators supports. An example of this annotation can be seen below:

```yaml=
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  annotations:
    # Prevent upgrades to OpenShift Version 4.9
    operators.coreos.com/maxOpenShiftVersion: "4.8" 
```

Assume that the CSV shown above is the only CSV on the Cluster. In this case, OLM would compare the value of the `operators.coreos.com/maxOpenShiftVersion` annotation against the version of the cluster to decide its upgradeable status.

As this is an opt-in feature, there are instances where OLM will be unable to determine if the upgrade is safe:

- The CSV is missing the `operators.coreos.com/maxOpenShiftVersion` annotation
- The value associated with the `operators.coreos.com/maxOpenShiftVersion` is not a valid `{major}.{minor}` Semantic Version (Valid Example: x.y).

OLM will report the following upgradeable conditions based on the collection of CSVs present on cluster:

| Key                  | Meaning                                                                                            |
| ---------------------| -------------------------------------------------------------------------------------------------- |
| Upgradeable    CSV   | A CSV with a valid upgradeable annotation that will run on the next Minor version of OpenShift     |
| Non-Upgradeable  CSV | A CSV with a valid upgradeable annotation that will not run on the next Minor version of OpenShift |
| Undeterminable CSV   | A CSV that does not include a valid upgradeable annotation                                         |

| Upgradeable CSV Present | Indeterminate CSV Present | Non-Upgradeable CSV Present | Status          | Message                                                                                            |
| ----------------------- | ------------------------- | --------------------------- |---------------- | -------------------------------------------------------------------------------------------------- |
| Yes                     | No                        | No                          | Upgradeable     | Ready for upgrade                                                                                  |
| Don't Care              | Yes                       | No                          | Upgradeable     | The following operators may not run on the next OpenShift Version: namespace/foo, namespace/bar"   |
| Don't Care              | Don't Care                | Yes                         | Not Upgradeable | The following operators will not run on the next OpenShift Version: namespace/foo, namespace/bar"  |

### Preventing OLM Managed Operators Installation on Unsupported OpenShift Cluster Versions

OLM Managed Operators can already specify a `minKubeVersion` on their Operator Bundles which prevents OLM from installing the operator on a Kubernetes Clusters whose version is less than the specified value.
Similarly, the resolver will be updated to use the `operators.coreos.com/maxOpenShiftVersion` annotation to prevent operators from being installed on an OpenShift Version that is not supported. The resolver will be updated to:

1. Check for the presence of the ClusterVersion API.
2. If present, the resolver will check if the operator being installed includes a `maxOpenShiftVersion`. If a `maxOpenShiftVersion` is specified and the cluster version is less than the specified version, OLM will prevent the operator from being installed.

### User Stories

#### Story 1

As an OpenShift cluster admin, I want OLM to block cluster upgrades if one or more of the operators that it manages will will not run on the next minor OpenShift version.

#### Story 2

As an operator author using OLM to manage the lifecycle of my operator, I want OLM to prevent upgrades to specific `{major}.{minor}` versions of OpenShift that I know are not supported by my operator.

#### Story 3

As an operator author using OLM to manage the lifecycle of my operator, I want OLM to prevent installations of my operators on OpenShift Clusters whose versions are greater the `maxOpenShiftVersion` defined by my operator.

#### Story 4

As the index maintainer, I want to be able to dynamically change which `{major}.{minor}` OpenShift versions that are supported by my operator without releasing a new version of the CSV.

### Risks and Mitigations

- The most immediate risk to this solution is the fact that OLM will prevent Minor OpenShift Upgrades when an OLM Managed Operator does not support the next minor OpenShift version.
This risk is mitigated given that admins will still be able to apply security fixes with patch updates whether or not OLM is blocking Minor OpenShift Upgrades. This gives them a safe window to either remove the minor-blocking operator or update it to a version that is compatible with the next minor OpenShift version.
In extreme cases, cluster admins can override the CVO's upgrade checks via means documented elsewhere.

## Design Details

### Open Questions

- When a CSV is present that does not include a `maxOpenShiftVersion` annotation, how will this information be surfaced by the UI and CLI?

### Test Plan

#### Proposed Unit Tests

- Logic used to determine upgrade safety should be thoroughly tested

#### Proposed E2E Tests

- The cluster upgrade is not blocked by CSVs that do not report a `maxOpenShiftVersion`.
- The cluster upgrade is not blocked by CSVs that report an invalid `maxOpenShiftVersion`.
- The cluster upgrade is not blocked by CSVs that report a `maxOpenShiftVersion` greater than the current OpenShift version.
- The cluster upgrade is blocked by CSVs that report a `maxOpenShiftVersion` less than or equal to the current OpenShift version..

For operators that specify a `maxOpenShiftVersion`:

- The operator can be installed on a Kubernetes cluster.
- The operator can be installed on OpenShift clusters whose version is less than or equal to the `maxOpenShiftVersion` specified by the operator.
- The operator can be installed on OpenShift clusters whose version is greater than the `maxOpenShiftVersion` specified by the operator.

For operators that do not specify a `maxOpenShiftVersion`:

- The operator continues to be installed on both OpenShift and vanilla Kubernetes clusters.

### Graduation Criteria

The goal of this enhancement is to provide Cluster Admins some level of assurance that service made available via OLM Managed Operators continue to remain available on a targeted OpenShift Cluster Version.

This feature will initially be introduced as a Generally Available Feature, but upgrade compatibility checks may be added overtime.

Proposed GA Features:

- OLM determines upgrade safety based on CSV annotations.
- OLM prevents operators from being installed on OpenShift clusters whose versions are greater than than the specified `maxOpenShiftVersion`.
- Console highlights upgrade safety in the UI.
- User facing documentation available at olm.operatorframework.io.
- Comprehensive unit tests exist.
- Comprehensive e2e tests exist.

#### Removing a deprecated feature

Not applicable.

#### Upgrade / Downgrade Strategy

Not applicable.

#### Version Skew Strategy

The proposed feature should only require integration with the CVO component and OLM Managed Operators.

## Implementation History

- Initial enhancement proposal created.

## Drawbacks

- OpenShift users might be upset when a Minor OpenShift Upgrade is block by OLM if they are unfamiliar with this feature.
- There are instances where an operator might not be supported on the targeted version that the operator is being upgraded to. If the customer is unable to remove the existing operator, this may place them in a position where they must choose between upgrading the cluster or removing the operator.
This is arguably better than upgrading the cluster only to find out that a core service is no longer available on cluster, but I suspect that multiple tickets will be opened against OLM and the operators it installs when the customer finds themselves unable to upgrade.

## Possible Future Work

### Determine upgrade safety using OLM Managed Operator RBAC

In addition to giving Operator Authors the ability to declare a maximum OpenShift Version, OLM could determine upgrade readiness based on default OpenShift GVKs and those required by installed operators. At a high level, the following workflow could be implemented:

1. CVO provides OLM with a list of default GVKs at OpenShift version `x.y.z`. This information should be provided by the CVO team, they could possibly retrieve it from the same source that stores OpenShift upgrade channels.
2. OLM retrieves the list of default GVKs available on OpenShift version `x.y.z`.
3. OLM generates a list of required APIs by aggregating the GVKs defined as permissions and ClusterPermissions in existing ClusterServiceVersions. Any operators that uses a wildcard (*) for the Group, Version, or Kind in its permissions/clusterPermissions would automatically get flagged as OLM would be unable to guarantee that all GVK required by the operator would be available on cluster version `x.y.z`.
4. OLM would then compare the list of apis on the cluster at version `x.y.z` against those required by installed OLM Managed Operators. If a required GVK is unavailable, OLM would report that it is no longer upgradeable.
If the required GVKs are available, OLM would mark itself as upgradeable, possibly using a special Message/Reason to signified that it has compared required GVKs against available GVKs at target version.
If OLM is unable to guarantee that the required GVKs are available on OpenShift version `x.y.z` that could be highlighted as well, the cluster admin could then force the upgrade if they feel comfortable doing so.

#### Pros

- This feature is available by default and does not require any additional effort from operator authors, giving some level of assurance that the upgrade is safe.

#### Cons

- This solution forces the CVO (or API team?) to host and provide available GVK information for each `{major}.{minor}` OpenShift version
- This solution could falsely identify an upgrade as safe. Rather than tell customers that their operators will keep working, we should specify that all GVKs required by the operator are available on the `x.y.z` version of OpenShift.
- There are many instances where an operator may not be supported on a specific version of OpenShift outside of required GVKs versus available GVKs.
- Operator authors must specify complete GVKs in their CSVs if OLM is to report true/false for upgrade safety.

## Infrastructure Needed

No additional infrastructure is needed.
