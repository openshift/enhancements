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

The idea is that the Job will run during the upgrade until completion. Once
finished, the Service Catalog Operators will have been removed from the
cluster. The Job that will delete the Service Catalog operators and their
associated items.

- cluster-svcat-apiserver-operator
  - Role named `prometheus-k8s in `openshift-service-catalog-apiserver-operator`
  - RoleBinding named `prometheus-k8s in `openshift-service-catalog-apiserver-operator`
  - ServiceMonitor in `openshift-service-catalog-apiserver-operator`
  - Namespace `openshift-service-catalog-apiserver-operator` (not sure
    if we can do this from the Job)
  - ServiceCatalogAPIServer (cr)
  - ConfigMap named `openshift-service-catalog-apiserver-operator-config`
  - Service named `metrics`
  - ClusterRoleBinding for `openshift-service-catalog-apiserver-operator` ServiceAccount
  - ServiceAccount `openshift-service-catalog-apiserver-operator`
  - Deployment named `openshift-service-catalog-apiserver-operator`
  - ClusterOperator named `service-catalog-apiserver`
- cluster-svcat-controller-manager-operator
  - Role named `prometheus-k8s in `openshift-service-catalog-controller-manager-operator`
  - RoleBinding named `prometheus-k8s in `openshift-service-catalog-controller-manager-operator`
  - ServiceMonitor in `openshift-service-catalog-controller-manager-operator`
  - Namespace `openshift-service-catalog-controller-manager-operator` (not sure
    if we can do this from the Job)
  - ServiceCatalogControllerManager (cr)
  - ConfigMap named `openshift-service-catalog-controller-manager-operator-config`
  - Service named `metrics`
  - ClusterRoleBinding for `openshift-service-catalog-controller-manager-operator` ServiceAccount
  - ServiceAccount `openshift-service-catalog-controller-manager-operator`
  - Deployment named `openshift-service-catalog-controller-manager-operator`
  - ClusterOperator named `service-catalog-controller-manager`

The Job will NOT touch the Service Catalog resources. These resources will
remain if the admin marked the Service Catalog CRs as `Unmanaged` meaning they
will self manage the Service Catalog relieving the operators of their duty.

The Service Catalog operators would remain in the release payload but the
Service Catalog itself would be removed.

#### Story 2

Keep the Service Catalog _Operators_ in the OpenShift 4.4 release payload.

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

#### Story 6

The plan is to block upgrades from happening until the admin either removes the
Service Catalog or chooses to self-manage their Service Catalog install. These
can be satisified by changing the `managementState` of both Service Catalog CRs
to either `Removed` to remove the Service Catalog bits or `Unmanaged` to relieve
the operators of their management duties.

In a 4.3.z release, update the Service Catalog Operators to set `Upgradeable: False`
whenever the `managementState` is `Managed`. The CVO supports blocking upgrades when
operators set this field since [PR #243](https://github.com/openshift/cluster-version-operator/pull/243)

The `Upgradeable` field is dependent on the value of the `managementState`
field. The following table shows the corresponding values:

| managementState | Upgradeable |
| --------------- | ----------- |
| Managed         | False       |
| Unmanaged       | True        |
| Removed         | True        |

Include detailed directions on how to remove the Service Catalog and it's
associated objects to allow upgrading to OpenShift 4.4.

NOTE: this would mean clusters would not be upgradeable to 4.3.z either without
a change to the CVO. Allowing minor upgrades is being explored in the "loosen
upgradeable condition to allow z-level upgrades" [PR #291](https://github.com/openshift/cluster-version-operator/pull/291)

#### Story 7

Create a bugzilla for docs include detailed directions on how to remove the
Service Catalog and it's associated objects to allow upgrading to OpenShift 4.4.

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
- 20200115 - Revised to show new plan for removing Service Catalog

## Drawbacks

The idea is to find the best form of an argument why this enhancement should _not_ be implemented.

- what about Custom Broker installed, how will those be handled?

These Custom Brokers will be orphaned and remain running.

## Alternatives

N/A

## Infrastructure Needed [optional]

N/A
