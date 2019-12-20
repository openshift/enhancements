---
title: service-catalog-and-broker-removal
authors:
  - "@jmrodri"
reviewers:
  - "@dmesser"
  - "@joelanford"
  - "@robszumski"
approvers:
  - TBD
creation-date: 2019-12-17
last-updated: 2019-12-17
status: implementable
see-also:
  - "/enhancements/service-catalog/prepare-service-catalog-and-brokers-for-removal.md"
replaces:
superseded-by:
---

# Service Catalog and Broker Removal

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

N/A

## Summary

In OpenShift 4.1, we announced the deprecation of the Service Catalog, the
Ansible Service Broker, and the Template Service Broker. In 4.4, we plan to
remove them from productization. They will no longer be installed in a new
cluster.

## Motivation

This is the next step in the removal of the Service Catalog and Brokers.

* Deprecated APIs in OpenShift 4.1 [release
  notes](https://docs.openshift.com/container-platform/4.1/release_notes/ocp-4-1-release-notes.html).
* In OpenShift 4.2, a more detailed announcement of their deprecation was
  released: [4.2 release notes](https://docs.openshift.com/container-platform/4.2/release_notes/ocp-4-2-release-notes.html#ocp-41-deprecated-features)
* [Support for templates](https://docs.openshift.com/container-platform/4.2/release_notes/ocp-4-2-release-notes.html#ocp-4-2-general-web-console-updates) without using the Template Service Broker and Service
  Catalog added to OpenShift 4.2.
* Alerts were added to the Service Catalog operators:
  * https://github.com/openshift/cluster-svcat-apiserver-operator/blob/master/manifests/0000_90_cluster-svcat-apiserver-operator_02-operator-servicemonitor.yaml#L28-L45
  * https://github.com/openshift/cluster-svcat-controller-manager-operator/blob/master/manifests/0000_90_cluster-svcat-controller-manager-operator_02_servicemonitor.yaml#L27-L44
* Alerts were added to the Ansible Service Broker operator:
  * https://github.com/openshift/ansible-service-broker/blob/master/operator/roles/ansible-service-broker/templates/broker.service-monitor.yaml.j2
* Alerts were added to the Template Service Broker operator:
  * https://github.com/openshift/template-service-broker-operator/blob/master/roles/template-service-broker/templates/tsb.prometheus-alert.yaml

### Goals

1. Implement a mechanism that ensures the Service Catalog and Service Brokers
   are not present after an upgrade to OpenShift 4.4. (ENG)
1. Add a new operator to remove brokers to the OpenShift 4.4 release payload (ENG/ART)
1. Remove the Service Catalog from the OpenShift 4.4 release payload (ART)
   * Will have to keep the Service Catalog operators
1. Remove the Template Service Broker from productization (ART)
1. Remove the Ansible Service Broker from productization (ART)
1. Remove the Template Service Broker Operator from productization (ART)
1. Remove the Ansible Service Broker Operator from productization (ART)
1. Remove the Service Catalog from productization (ART)

### Non-Goals

N/A

## Proposal

### User Stories

#### Story 1

Implement a mechanism that ensures the Service Catalog is not present after
an upgrade to OpenShift 4.4.

Replace the manifests of the existing cluster-svcat-apiserver-operator and
cluster-svcat-controller-manager-operator, with a [Job](https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/) definition.
This might be accomplished with just one of the operators, that would need
a little more investigation.

Replace the operator's code with a new Job executable that will perform the
deletion of the apiserver associated with the Service Catalog. The following items
would be removed:

- ClusterServiceClasses
- ClusterServicePlans
- ClusterServiceBrokers
- ServiceClasses
- ServicePlans
- ServiceBrokers
- ServiceInstances
- ServiceBindings

The idea is that the Job will run during the upgrade until completion. Once
finished, the Service Catalog will have been removed from the cluster.

The Service Catalog operators would *remain* in the release payload but the
Service Catalog itself would be removed.

There are a few alternatives that are discussed in the Alternatives section of
this proposal, please see below.

#### Story 2

Implement a mechanism that ensures Service Brokers are not present after
an upgrade to OpenShift 4.4.

Create a new CVO managed operator with a Job definition as in Story 1 to remove
the Ansible Service Broker and Template Service Broker. The new CVO operator is
needed because the Ansible Service Broker operator and the Template Service
Broker operator are both OLM managed operators which may or may not be present
during cluster upgrades.

NOTE: this would add a new item to the OpenShift 4.4 release payload.

There is also an alternative to this approach in the Alternatives section below.

#### Story 3

1. Keep the Service Catalog _Operators_ in the OpenShift 4.4 release payload
1. Remove the Service Catalog from the OpenShift 4.4 release payload (ART)

#### Story 4

Remove the Brokers from productization

1. Remove the Template Service Broker from productization (ART)
1. Remove the Ansible Service Broker from productization (ART)
1. Remove the broker sections from the documentation (Docs)

#### Story 5

Remove the broker operators from productization

1. Remove the Template Service Broker Operator from productization (ART)
1. Remove the Ansible Service Broker Operator from productization (ART)
1. Remove the operator instructions from the documentation (Docs)

#### Story 6

1. Remove the Service Catalog from productization (ART)
1. Remove the installation instructions from the documentation (Docs)

### Implementation Details/Notes/Constraints [optional]

N/A

### Risks and Mitigations

N/A

## Design Details

### Test Plan

#### cluster-svcat-apiserver-operator
 - add tests to verify apiservice and associated objects are removed

#### cluster-svcat-controller-manager-operator
 - add tests to verify controller-manager and associated objects are removed

#### Upgrades
 - upgrade testing will be crucial for this feature
 - if there are tests written for upgrades, we will want to add to them

### Graduation Criteria

The Service Catalog and Brokers have reached the removal stage of the graduation
criteria.

##### Removing a deprecated feature

- In 4.1, the Service Catalog and Brokers were marked as deprecated in the
  documentation, see the [release notes](https://docs.openshift.com/container-platform/4.1/release_notes/ocp-4-1-release-notes.html).

- In 4.2, a more detailed announcement of their deprecation was
  released: [4.2 release notes](https://docs.openshift.com/container-platform/4.2/release_notes/ocp-4-2-release-notes.html#ocp-41-deprecated-features)

- [Support for templates](https://docs.openshift.com/container-platform/4.2/release_notes/ocp-4-2-release-notes.html#ocp-4-2-general-web-console-updates) without using the Template Service Broker and Service
  Catalog added to OpenShift 4.2.

- In 4.3, we alert users when the Service Catalog or its Brokers are in use.

- In 4.4, Service Catalog and Brokers will be removed from shipping.

### Upgrade / Downgrade Strategy

Clusters will be allowed to upgrade and the operators will remove the Service
Catalog and Service Brokers during upgrade via a Job.

### Version Skew Strategy

N/A

## Implementation History

- 20190916 - Proposal for removal preparation was written
- 20191217 - Proposal to remove Service Catalog and Brokers
- 20191220 - Revised this proposal

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

- what about Custom Broker installed, how will those be handled?

These Custom Brokers will be orphaned and remain running.

## Alternatives

#### Block upgrades

One alternative to Story 1 is to block upgrades from happening until the
admin removes the Service Catalog and the brokers. In a 4.3.z release,
update the Service Catalog Operators to set `Upgradeable: False` whenever the
`managementState` is `Managed`. The CVO supports blocking upgrades when
operators set this field since [PR #243](https://github.com/openshift/cluster-version-operator/pull/243)

Include detailed directions on how to remove the Service Catalog and it's
associated objects to allow upgrading to OpenShift 4.4.

The OpenShift 4.4 would then not need any Service Catalog bits. But it would put
the onus on the cluster admin to clean things up properly.

NOTE: this would mean clusters would not be upgradeable to 4.3.z either without
a change to the CVO. Allowing minor upgrades is being explored in the "[poc]
honor UpgradeableMinor condition" [PR #285](https://github.com/openshift/cluster-version-operator/pull/285)

#### Use a new Service Catalog / Broker Removal Operator

Another alternative, is to create a *new* CVO managed operator that would handle
the removal of the Service Catalog *and* the Service Brokers in a single
operator.

This would mean replacing the Service Catalog operators with a a new operator in
the OpenShift 4.4 release payload.

The pro to this approach is that it is explicit in what it is doing. Also, we
are already proposing adding a new operator in Story 2 to handle the Service
Broker removal.

#### Repurpose the Service Catalog operator to handle Service Brokers as well

Yet another alternative, is to add support to the Service Catalog operator Job
from Story 1 to also handle removal of Service Brokers. Ideally, it would be a
separate Job defined in the same manifest.

This approach would have *no* additions to the release payload in OpenShift
4.4. The downside is the Service Catalog operator gets confusing since it is
also handling Service Brokers which might not be obvious.

## Infrastructure Needed [optional]

N/A
