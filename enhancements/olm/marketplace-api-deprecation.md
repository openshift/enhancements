---
title: marketplace-api-deprecation
authors:
  - "@ecordell"
reviewers:
  - TBD
approvers:
  - TBD
creation-date: 2020-01-06
last-updated: 2020-01-06
status: implementable
---

# Marketplace Operator API Deprecation

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement covers the remaining steps needed to ensure that everything the marketplace-operator APIs have been providing is covered by other features within OLM or OpenShift.

## Motivation

In OpenShift 4.1+, the marketplace-operator is responsible for pulling repository data into a cluster from an external source (Quay.io Appregistry) for use by OLM.

As of OpenShift 4.4, alternatives for pulling external data have been built into OLM. The appregistry protocol itself is largely replaced by other efforts in OCI, and we cannot continue to rely on it long-term.

### Goals

- Remove support for appregistry-based storage for operator data
- Remove the on-cluster components that support it
- Continue to support configuration for OperatorHub

### Non-Goals

- Preserve non-default OperatorSource and CatalogSourceConfig data

## Proposal

### Blocking upgrades in the presence of non-default OperatorSources

The marketplace-operator will be updated to report `Upgradeable: False` if OperatorSources that are not one of the three defaults are detected in the cluster, or any CatalogSourceConfigs.

This will prevent upgrades to clusters that have the marketplace APIs removed until an administrator cleans up the use of appregistry.

The `Upgradeable: False` message should be accompanied by information about the block, with enough pointers to find the resources that are in need of change.

Note: this relies on [this feature of CVO](https://github.com/openshift/cluster-version-operator/pull/291) to be available.

### Removal of the Marketplace APIs

1. Remove the OperatorSource and CatalogSourceConfig CRDs. This will remove all CR instances.

2. Update the default CatalogSources (that are no longer updated by
   OperatorSources) from `address` type CatalogSources to `image` type
   CatalogSources, with appropriate poll intervals, pointing the the
   released catalog images instead of to appregistry repositories. To
   perform the migration, the OperatorHub Config API will be changed
   to reconcile `image` CatalogSource defaults instead of
   OperatorSource defaults.

3. Remove the reconcilation of the OperatorSource and CatalogSourceConfig APIs either by removing the marketplace operator entirely or by removing the relevant portions of it (if it is being retained to manage the OperatorHub API, see below).

### Migration of the OperatorHub Config API

The OperatorHub config API is used to configure which catalogs are available by default in a cluster, and is an important configuration point for disconnected clusters.

This API is currently managed by the marketplace operator and needs to continue to be supported. Currently, the api configures default OperatorSources, and needs to be changed to configure default `image` CatalogSources.

To start with, this will remain in the marketplace-operator. In the future it may be useful to migrate it to the olm.

### Risks and Mitigations

It is possible that some customers are heavily using the appregistry-based catalogs and marketplace APIs. Because we believe this use to be minimal at best, we have not provided an automated migration plan for those cases. Instead, we will block Y upgrades if these apis are in use, with appropriate messages for users.

If we determine that uninterrupted use of appregistry is desired or required, this proposal will need to include migration steps for all OperatorSource and CatalogSourceConfig APIs in-use in a cluster.

Otherwise, a manual guide for migrating custom catalogs should be straightforward to follow.

## Design Details

### Graduation Criteria / Deprecation Plan

Deprecation of the OperatorSource and CatalogSourceConfig APIs has been communicated in the [4.2 release notes](https://docs.openshift.com/container-platform/4.2/release_notes/ocp-4-2-release-notes.html)

Removal of the marketplace operator will not yet remove the operator-marketplace namespace. This namespace is currently the designated namespace for global catalogs.

### Upgrade / Downgrade Strategy

On upgrade, the default catalog contents (redhat-operators, certified-operators, community-operators) will be converted into image-based CatalogSources that poll for updates.

It should not be necessary to update any Subscriptions, as the new CatalogSources will have the same names as the old ones that were generated off of the OperatorSources.

The OperatorHub config api may cease to be managed by the marketplace operator, and instead may be managed by OLM operator.

### Version Skew Strategy

During upgrade, there may be a period of time when the OperatorHub config API could be managed by two operators (marketplace and OLM).

The new OLM version that is taking ownership of the OperatorHub config API should wait until the marketplace-operator is not available before reconciling the config API. This is also when OLM can perform the migration of the CatalogSources from "grpc/api" to "image/poll" types.

This is only an issue when migrating ownership of the config api, which is a final step that is not required.

#### Downgrades

If OpenShift is downgraded, there are two cases that can occur

 1. defaults are **enabled**

    - The marketplace operator will start up and recreate the default OperatorSources. This will adopt the CatalogSources of the same name, and overwrite their spec to point to the registry pod (which will be managed by marketplace).

 1. defaults are **disabled**

    - The marketplace operator will start up, see no defaults, see no OperatorSources (because we are downgrading from a cluster that has removed them), and idle.
    - The CatalogSources (migrated from OperatorSources on upgrade) will be using the existing image poll mechanism, which will continue to work in the previous version (it is in 4.4). They will continue to recieve updates, even though there is no OperatorSource, which may be confusing.
    - The previous version of openshift will see operator content for version that was downgraded from.

## Implementation History

- Proposal 01/06/20
- Updated based on Y-upgrade blocking 01/09/20
- Added a section on downgrades 01/21/20

## Drawbacks

If there are customers making heavy use of appregistry-backed catalogs, outright removal of the feature may be an issue for them.

## Alternatives

We could continue to support appregistry-backed catalogs indefinitely. The support on the Quay side would need to be negotiated, since Marketplace is one of if not the only consumer of this API.
