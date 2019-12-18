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
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

## Summary

In OpenShift 4.1, we announced the deprecation of the Service Catalog, the
Ansible Service Broker, and the Template Service Broker. In 4.4, we plan to
remove them from productization. They will no longer be installed in a new
cluster. They will also block cluster upgrades until they are disabled in older
clusters.

## Motivation

This is the next step in the removal of the Service Catalog and Brokers.

* Deprecated APIs in OpenShift 4.1 [release
  notes](https://docs.openshift.com/container-platform/4.1/release_notes/ocp-4-1-release-notes.html).
* In OpenShift 4.2, a more detailed annoucement of their deprecation was
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
1. Prevent users from upgrading to OpenShift 4.4 if the Service Catalog or the
   Service Brokers are in use. (ENG)
1. Remove the Service Catalog from the OpenShift 4.4 release payload (ART)
   * Will have to keep the Service Catalog operators if we choose to repurpose
     them.
1. Remove the Ansible Service Broker from the OpenShift 4.4 release payload (ART)
1. Remove the Template Service Broker from the OpenShift 4.4 release payload (ART)
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

Implement a mechanism that ensures the Service Catalog and Service Brokers are
not present after an upgrade to OpenShift 4.4.

We could potentially block upgrades from happening until the admin removes the
Service Catalog and the brokers. This would need a mechanism that blocks
upgrades. There is an `Upgradeable: False` in the operator's status but not sure
if that is checked by the CVO during upgrades.

We could let the upgrade occur and change the behavior of the existing operators
to simply remove the objects they own. If we go this route, we will need to keep
the operators in the release payload.

Another mechanism, similar to the one above is to use a [Job](https://kubernetes.io/docs/concepts/workloads/controllers/jobs-run-to-completion/) to do the deletion. The job would be defined in the
manifest directory of the Service Catalog operators.

#### Story 2

Remove Service Catalog and the brokers from the release payload

1. Remove the Service Catalog from the OpenShift 4.4 release payload (ART)
1. Remove the Ansible Service Broker from the OpenShift 4.4 release payload (ART)
1. Remove the Template Service Broker from the OpenShift 4.4 release payload (ART)

#### Story 3

Remove the Brokers from productization

1. Remove the Template Service Broker from productization (ART)
1. Remove the Ansible Service Broker from productization (ART)
1. Remove the broker sections from the documentation (Docs)

#### Story 4

Remove the broker operators from productization

1. Remove the Template Service Broker Operator from productization (ART)
1. Remove the Ansible Service Broker Operator from productization (ART)
1. Remove the operator instructions from the documentation (Docs)

#### Story 5

1. Remove the Service Catalog from productization (ART)
1. Remove the installation instructions from the documentation (Docs)
1. Ensure the how to disable service catalog instructions are correct (Docs/Eng)

### Implementation Details/Notes/Constraints [optional]

There are still a few unknowns for exactly how to accomplish the removal of
the Service Catalog and Service Brokers. I don't think any of them are show
stoppers, just might cause a few changes to this proposal depending on the
approach taken.

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

The Service Catalog and Brokers have reahed the removal stage of the graduation
criteria.

##### Removing a deprecated feature

- In 4.1, the Service Catalog and Brokers were marked as deprecated in the
  documentation, see the [release notes](https://docs.openshift.com/container-platform/4.1/release_notes/ocp-4-1-release-notes.html).

- In 4.2, a more detailed annoucement of their deprecation was
  released: [4.2 release notes](https://docs.openshift.com/container-platform/4.2/release_notes/ocp-4-2-release-notes.html#ocp-41-deprecated-features)

- [Support for templates](https://docs.openshift.com/container-platform/4.2/release_notes/ocp-4-2-release-notes.html#ocp-4-2-general-web-console-updates) without using the Template Service Broker and Service
  Catalog added to OpenShift 4.2.

- In 4.3, we alert users when the Service Catalog or its Brokers are in use.

- In 4.4, Service Catalog and Brokers will be removed from shipping.

### Upgrade / Downgrade Strategy

Either one of these will happen:

- Clusters will be blocked from upgrading if Service Catalog and/or Service
  Brokers are enabled. They must be removed before cluster upgrades may proceed.
- Clusters will be allowed to upgrade and the operators will remove the Service
  Catalog and Service Brokers during upgrade.

### Version Skew Strategy

N/A

## Implementation History

- 20190916 - Proposal for removal preparation was written
- 20191217 - Proposal to remove Service Catalog and Brokers

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

## Alternatives

Similar to the `Drawbacks` section the `Alternatives` section is used to
highlight and record other possible approaches to delivering the value proposed
by an enhancement.

## Infrastructure Needed [optional]

N/A
