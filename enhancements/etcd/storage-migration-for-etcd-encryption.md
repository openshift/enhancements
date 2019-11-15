---
title: storage-migration-for-etcd-encryption
authors:
  - "@sanchezl"
reviewers:
  - "@sttts"
  - "@deads2k"
  - "@enj"
approvers:
  - TBD
creation-date: 2019-09-11
last-updated: 2019-09-11
status: provisional
see-also:
  - "https://github.com/kubernetes/enhancements/blob/master/keps/sig-api-machinery/0030-storage-migration.md"  
---

# storage-migration-for-etcd-encryption

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Introduce the `openshift-kube-storage-version-migrator-operator` to install the upstream `StorageVersionMigration` APIs CRD and `kube-storage-version-migrator` controller.  `StorageVersionMigration` resources can then be created by components, (initially  `kube-apiserver` and `openshift-apiserver`) to kick off storage migrations for resources.

Example `StorageVersionMigration`:
```yaml
apiVersion: migration.k8s.io/v1alpha1
kind: StorageVersionMigration
metadata:
  name: secrets.v1-key.1
spec:
  resource:
    resource: secrets
    version: v1
status:
  conditions:
  - status: "True"
    type: Running
```

The first known usage is for etcd encryption at rest: `secrets`, `configmaps`, `routes.route.openshift.io`, `oauthaccesstokens.oauth.openshift.io` and `oauthauthorizetokens.oauth.openshift.io`.

## Motivation

The immediate need for storage migration is to support the enablement of etcd data storage encryption. Encrypting etcd data storage involves rotating encryption keys periodically, which requires that stored data be migrated after choosing a new write key and before removing an old read key so that api servers can discard old encryption keys periodically and not lose the ability to decode the pre-existing data.

We will use the upstream `kube-storage-version-migrator` (the controller-based successor to `oc adm migrate`). While upstream usage of the `kube-storage-version-migrator` is focused on migrating to new storage versions, we should be able to explicitly trigger the `kube-storage-version-migrator` when we need to migrate the encryption key used for storage, even when the storage version itself has not changed.

## Goals

* Make the `StorageVersionMigration` API and its corresponding controller available.
* Invoke the `StorageVersionMigration` API from operators requesting that storage be migrated to use new etcd storage encrpyion keys.

## Non-Goals

* Will not provide a higher level API to the `StorageVersionMigration` API controlled by the `kube-storage-version-migrator`.
* Will not implement any general control loops to automatically detect and then initiate a storage migration when needed.

## Proposal

* Introduce `openshift/cluster-kube-storage-version-migrator-operator` repository.
  * Not required for bootstrapping or re-bootstrapping.
  * No need for static pods.
  * Will run on cluster.
* Implement `openshift-kube-storage-version-migrator-operator`. 
* Build `kube-storage-version-migrator` image.
  * Fork `kubernetes-sigs/kube-storage-version-migrator` to `openshift`
  * Enable CI on `openshift/kube-storage-version-migrator` repo.
* Utilize `StorageVersionMigration` API when etcd storage encryption keys are updated.

## User Stories [optional]

* kube-apiserver-operator can request a resource migration and confirm when the migration has completed. 

* openshift-apiserver-operator can request a resource migration and confirm when the migration has completed. 

## Implementation Plan

1. Implement `openshift-kube-storage-version-migrator-operator`.
2. Have CVO manage `openshift-kube-storage-version-migrator-operator`.
2. Add control loops to operators as needed.

## Implementation Details/Notes/Constraints

* Create a `openshift-kube-storage-version-migrator-operator`, managed by the CVO, to manage the resources needed to run the `kube-storage-version-migrator`.
* Initiate a storage migration of resources, even if the storage version has not changed.
* Monitor the progress of storage migration of resources.
* Enable the `kube-apiserver-operator` to utilize the `StorageVersionMigration` API when managing etcd encryption keys.
* Enable the `openshift-apiserver-operator` to utilize the `StorageVersionMigration` API when managing etcd encryption keys.
* Inform the upstream `kube-storage-version-migrator` of our usage and our success/failure to gain influence for future decisions and create tests to ensure our use-cases.

New control loop added to kube-apiserver-operator & openshift-apiserver-operator:

```
for { // control loop

    key := getNextWriteKey()

    for resource := range encryptedResources {
        migration := getStorageVersionMigrationFor(resource, key)
        if migration not found {
            createStorageVersionMigrationFor(resource, key)
        }
    }

    if migrationsAreCompleteFor(key) {
        setOperatorStatusPending(false)
        enableWriteKey(key)
    } else {
        setOperatorStatusPending(false)
    }
        
}
```

## Risks and Mitigations

## Design Details

#### How do operators communicate the need for migration?

They create a `StorageVersionMigration` resource for the GroupVersionResource they wish to migrate.

#### How do operators get progress info?

 By examining `StorageVersionMigration.status.condition[@type='Succeeded|Running']`.

#### What happens if multiple operators need migration?

Each operator creates thier own request (`StorageVersionMigration`) for a specific GVR to be migrated. Each operator examines thier corresponding request for progress info. If multiple migration requests are made for the same GVR, those will be exected serially.

#### Do we need throttling?

If we do, it would be a function of the upstream controller. Some throttling is currently done by the upstream controller.

#### Do we want or need CRD migration as well which might mean to have a user-facing API.

No user-facing API will be provided with this enhancement.

#### When do we trigger migration on upgrade?

From an etcd encryption point of view, we do not trigger migration during an upgrade. Triggering a migration during an upgrade might cause an actual storage version migration that the control plane might not be ready for (e.g. some node are behind).  The apiserver operators have to orchestrate operand updates and migration.

#### Where do we store state of the last successful migration of a resource? We don't want to run a migration again if it was successful.

The `StorageVersionMigration` resource can serve as a record of a successful migration.  

#### Do The `StorageVersionMigration` resources get cleaned up after a while?

After a new write key has been activated, the corresponding `StorageVersionMigration` resources for that key can be cleaned up. 

## Test Plan

* An e2e test verfying migrator functionality: create crd-v1, enable crd-v2, trigger migration, verify storageversionHash.
* E2e tests added for **[Encrypting Data at Datastore Layer]** enhancement should not break.  

## Graduation Criteria

## Upgrade / Downgrade Strategy

## Version Skew Strategy

This component should not be invoked during an upgrade.

## Implementation History

## Drawbacks

## Alternatives

## Infrastructure Needed [optional]

### New github projects:

* `openshift/cluster-kube-storage-version-migrator-operator`
* `openshift/kubernetes-kube-storage-version-migrator`

### New images built:

* `cluster-kube-storage-version-migrator-operator`
* `kube-storage-version-migrator`

[Encrypting Data at Datastore Layer]: https://github.com/openshift/enhancements/pull/32