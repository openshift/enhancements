---
title: upgrade-acknowledgment-gate
authors:
  - "@jottofar"
reviewers:
  - "@LalatenduMohanty"
  - "@bparees"
  - "@sdodson"
  - "@csrwng"
  - "@relyt0925"
  - "@jeremyeder"
approvers:
  - "@sdodson"
creation-date: 2021-07-08
last-updated: 2021-07-23
status: implementable
---

# Upgrade Acknowledgment Gate

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] Operational readiness criteria is defined
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

CVO currently blocks minor level upgrades when overrides are set, a resource deletion is pending, or an operator has set its upgradeable condition to false.
This enhancement adds a generic upgrade blocking mechanism for blocking minor level upgrades.
This generic upgrade blocker can consist of one or more items each of which will block a minor level upgrade.
Each generic upgrade blocker item will be associated with some well-defined task to be completed by an administrator.
Once the task has been completed the administrator will acknowledge completion and its associated item will no longer block minor level upgrades.

## Motivation

OCP 4.9 uses kubernetes v1.22 which is removing a large number of [APIs][k8s-1.22-removed-apis].
We suspect that many customer clusters are still using these APIs through helm charts, operators, generic workloads, administrative tools, etc.
We believe there is no effective way for a Cluster Operator to detect the use of these APIs by external applications.
Relying solely on automated API detection to block upgrades could result in blocking for APIs that are not actively used any longer or not blocking because an application was not active during the check.
Having a human in the loop to drive the analysis and make the final decision is the surest way to prevent customers from upgrading from 4.8.z to 4.9.z and eliminate instances of missing API related breakage by allowing administrators to assess the impact per cluster and impacted application.

### Goals

* Acknowledgment requires the explicit update of a cluster resource by an administrator.
* For the initial release in 4.8.z, administrator acknowledgment of API removal will be required before a minor level upgrade to 4.9 is allowed.
* Upgrade blocking mechanism must be generic such that it can be repurposed in the future.
* It must be clear what an administrator is acknowledging.
* Administrators can bypass the upgrade blocking mechanism by using the existing CVO "force" implementation.
* The UI will clearly indicate when this upgrade blocking mechanism is blocking upgrades, why, and what needs to be done to clear the block.
* This upgrade blocking mechanism must be compatible with SD, ACM, IKS/ROKS, Hive, and Hypershift.

### Non-Goals

## Proposal

### General Implementation

CVO's existing framework for blocking minor level upgrades will be leveraged.
The existing framework periodically checks upgradeable conditions.
A new [upgradeable check method][upgradeableCheck-interface] will be added to the existing framework to implement this upgrade blocking mechanism.
Use of this existing framework also allows this new upgrade blocking mechanism to be bypassed by using the `force` upgrade option.

The new method's logic will be driven by configmap keys each of which represents a potential minor level upgrade blocker.
CVO will consume a new configmap, `admin-gates`, defining the keys. For example:
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: admin-gates
  namespace: openshift-cluster-version
data:
  ack-4.8-kube-122-api-removals-in-4.9: |
     Detailed description of what admin needs to do...Read documentation here https://
  ack-4.8-gate-2: |
     Detailed description of what admin needs to do...
```
The key name will include an OCP major/minor version number that indicates to which OCP version the key applies.
Key names must be in the format `ack-X.Y-<description>` where `X.Y` is the major/minor version number and `<description>` is words describing to what the administrator is agreeing.
For example, a key name beginning with `ack-4.8` would only apply to an OCP cluster currently at version 4.8.
The key's value will be a detailed message pointing to relevant OCP documentation explaining what the administrator is expected to do before acknowledging task completion.
If running within the applicable version, CVO will check a newly defined configmap, `admin-acks`, for the key.
The key represents a specific upgrade blocking condition that must be acknowledged before a minor level upgrade is allowed.
An administrator acknowledges a condition by adding the corresponding key with a value of `true` to the configmap `admin-acks`.
Values other than `true` will be ignored and therefore not clear the upgrade block.
The keys will be named to make it explicitly clear what the administrator is acknowledging and thereby agreeing to.
CVO will set `Upgradeable=false` which in turn blocks cluster upgrades to the next minor release until it finds each of the applicable predefined keys in the configmap with a value of `true`.
For each key blocking an upgrade, CVO will generate a detailed message containing the key and pointing to relevant OCP documentation explaining what the administrator is expected to do before acknowledging task completion, i.e. entering the corresponding key into the configmap with a value of `true`.

The new configmap `admin-acks` will be installed `create-only` as part of the release in the namespace `openshift-cluster-version`.
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  annotations:
    release.openshift.io/create-only="true"
  name: admin-acks
  namespace: openshift-cluster-version
data:
```
It will initially have no keys since the administrator is expected to add the relevant key as part of acknowledgement.

### 4.8.z Specific Implementation

Clusters upgraded to 4.9 will no longer have access to these [APIs][k8s-1.22-removed-apis].
This new upgrade blocking mechanism will be used in an attempt to require administrators to read detailed documentation regarding the removed APIs and the possible impact.
This OCP documentation will list which APIs have been removed and how an administrator checks their cluster for use of those APIs.
4.8.z versions that are upgradeable to 4.9 will contain this enhancement and have the configmaps `openshift-cluster-version/admin-gates` and `openshift-cluster-version/admin-acks`.
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: admin-gates
  namespace: openshift-cluster-version
data:
  ack-4.8-kube-122-api-removals-in-4.9: |
     Detailed description of what admin needs to do...Read documentation here https://
```
CVO will set `Upgradeable=false` which in turn blocks cluster upgrades to 4.9 until the administrator has added the key/value pair `ack-4.8-kube-122-api-removals-in-4.9: "true"` to the `admin-acks` configmap as shown below.
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  annotations:
    release.openshift.io/create-only="true"
  name: admin-acks
  namespace: openshift-cluster-version
data:
    ack-4.8-kube-122-api-removals-in-4.9: "true"
```
Until this is done the `Conditions` console page will display an `Upgradeable` condition message directing the administrator to the relevant OCP documentation.

If the administrator wants to upgrade to 4.9 without performing the acknowledgment `force` can be used.

### User Stories

* As a cluster administrator, I want to be warned that APIs will be removed if I upgrade to 4.9 before I upgrade.

* As a cluster administrator, I want to be instructed on how to check my cluster for potential impacts of API removal.

* As a cluster administrator, I want to be able to override an upgrade block and upgrade anyway.

* As an OCP developer I want a way to introduce additional "acknowledgment based" upgrade blocks for blocking minor level upgrades.

* As an OCP product team we do not want customers to be surprised by workloads breaking due to APIs being removed after they upgrade.

### Implementation Details/Notes/Constraints

### Risks and Mitigations

* A common risk is human error resulting in an incorrect key/value pair being placed in the `admin-acks` configmap.
This type of error will not change `Upgradeable=false` and therefore the administrator will know a mistake has been made since the `Upgradeable` condition message will continue to be displayed.
CVO will log an error whenever an unrecognized key is found in the `admin-acks` configmap.
* Another risk is that the `admin-acks` configmap gets removed.
This will result in `Upgradeable=false` and the standard `Upgradeable` condition message will be displayed with the additional message that the configmap cannot be found and should be created.
CVO will recreate the `admin-gates` configmap if removed since it will be managed by CVO.
CVO will log an error whenever the `admin-acks` configmap has been removed.
* Unable to access the `admin-acks` configmap.
This will result in `Upgradeable=false` and the standard `Upgradeable` condition message will be displayed with the additional message that the configmap cannot be accessed.
CVO will log an error whenever there is an issue accessing the `admin-acks` configmap.
* Unable to access the `admin-gates` configmap.
Since CVO will not be able to determine what if any minor level upgrade blocks are defined CVO will set `Upgradeable=false` and set the displayed `Upgradeable` condition message to indicate that the `admin-gates` configmap cannot be accessed.
CVO will log an error whenever there is an issue accessing the `admin-gates` configmap.

## Design Details

### Test Plan

* CVO unit tests will be expanded and/or created to test the new logic.
* e2e upgrade test will be enhanced.

### Graduation Criteria

GA. When it works, we ship it.

#### Dev Preview -> Tech Preview

N/A. This is not expected to be released as Dev Preview.

#### Tech Preview -> GA

N/A. This is not expected to be released as Tech Preview.

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

Inherent in the key name is an OCP major/minor version number. Based on this version number CVO knows to which OCP versions a given key applies.

### Version Skew Strategy

No special consideration.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation History`.

## Drawbacks

* Keys will not be portable across products that use a different versioning scheme than OCP.
* `Upgradeable=false` is the default state and results if there is any issue accessing either configmap or if an invalid key is found in the `admin-acks` configmap.

## Alternatives

* apiserver could be the component setting `Upgradeable=false` and looking for the acknowledgement, since it is the component that is "removing" the APIs.
However this alternative was rejected because:
  * Whether the kubernetes v1.22 APIs are removed has no true affect on whether the apiserver is upgradeable.
    API removal is a cluster-wide condition that is more accurately represented and managed by the CVO.
  * This mechanism is a generically useful concept so implementing it in a way that allows reuse for other similar upgrade gates requiring manual administrator action felt right.
    CVO already manages upgrades and contains extensive upgrade gating logic that can be reused.
* Another rejected alternative is to use the apiserver's data about what APIs are in use, and block upgrades if removed APIs are in use.
  This was rejected because that information isn't sufficient to make an informed choice.
  As described in [the *Motivation* section](#motivation), just because an API was used last week doesn't mean it's still required, and just because it was not used within the last week doesn't mean a tool or workload is not going to try to use it tomorrow.

[k8s-1.22-removed-apis]: https://kubernetes.io/docs/reference/using-api/deprecation-guide/#v1-22
[upgradeableCheck-interface]: https://github.com/openshift/cluster-version-operator/blob/3a68652568e9075c23f491bc8c037942bd67ec82/pkg/cvo/upgradeable.go#L132
