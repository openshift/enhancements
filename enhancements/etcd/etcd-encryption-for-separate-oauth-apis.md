---
title: ETCD-Encryption-For-Separate-OAuth-APIs
authors:
  - "@p0lyn0mial"
reviewers:
  - "@sttts"
approvers:
  - "@derekwaynecarr"
  - "@mfojtik"
creation-date: 2020-03-17
last-updated: 2020-03-17
status: implementable
see-also: https://github.com/openshift/enhancements/blob/master/enhancements/authentication/separate-oauth-resources.md, https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/
replaces:
superseded-by:
---

# ETCD Encryption For Separate OAuth APIs

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Open Questions [optional]

## Summary

The `encryption-config` used by OpenShift API server to encrypt/decrypt resources will also be used by the new `oauth-apiserver` for one release `(4.5)` and will be split in the next `(4.6)`, in order to allow seamless upgrade and downgrade of encrypted servers.
Initially `OAS-O` will be responsible to manage both servers. In the future releases `CAO` will take over the config and will manage its operand.

## Motivation

Starting from version `4.3` customers that want to have additional layer of data security can enable etcd encryption. Once enabled OpenShift API server encrypts among others `OAuth tokens`.
In version `4.5` we decided to split `openshift-apiserver` and create a new server called `oauth-apiserver`.
That means the resources previously managed by a different component now will be served by a brand new API server.
That also means that `oauth-apiserver` needs to be given the encryption keys in order to decrypt previously encrypted resources, like the aforementioned tokens.


Additionally, we would like to provide a smooth transition for our customers. 
Without any manual interaction and with fully tested and working upgrade and downgrade paths. 
This document describes how we are going to achieve that.

### Goals
1. Make it possible to run the OpenShift OAuth API Server on an encrypted cluster. That includes:
 - a cluster upgraded from an encrypted `4.4`
 - a cluster downgraded from an encrypted `4.6`
 - a new `4.5` cluster on which encryption was enabled

### Non-Goals


## Proposal

In OpenShift encryption at rest leverages Kubernetes built-in mechanism: It is based on `EncryptionConfiguration` that controls how API data is encrypted in etcd.
It holds one array of keys for each resource.

OpenShift maintains `EncryptionConfiguration` resource that is created and maintained by a set of controllers called the encryption controllers in `openshift-config-managed` namespace for each API server that needs encryption.
For example `OAS-O` creates `encryption-config-openshift-apiserver` secret in that namespace. At a later stage it is copied to `openshift-apiserver` namespace, revisioned and finally makes its way to the API server.

In order to make two API servers (`openshift-apiserver` and `oauth-apiserver`) use the same `encryption-config` we are going to create a copy for the new API server and let it be managed by `OAS-O` for one release `n` to support downgrades to release `n-1` (which will be `4.5` to `4.4`).
On the next release `n+1` we will copy over the keys to avoid creating new ones. From that point on `authentication-operator` will maintain its own config.

1. Create a new controller in `OAS-O` that will create and annotate `encryption-config-openshift-oauth` secret in `openshift-config-managed` namespace. It also must keep it in sync with `encryption-config-openshift-apiserver`.
 The annotation will prevent `CAO` from managing the newly created `encryption-config`.
2. Prepare `authentication-operator` to manage `encryption-config-openshift-oauth` but only if it doesn't have the annotation from `1`.
   - make use of `encryption.NewControllers` but don't start it if the annotation is present
3. Update `CAO` to revision and plug `encryption-config` for its operand.
4. Create a new deployer (`statemachine.Deployer`) called `UnionRevisionLabelPodDeployer` that will manage multiple `RevisionLabelPodDeployer`.
5. Implement `DeployedEncryptionConfigSecret() (secret *corev1.Secret, converged bool, err error)` function that:
   - calls `DeployedEncryptionConfigSecret` for all components
   - returns "failure" if any component has reported an error (`err !=nil`) or hasn't yet converged (`converged == false`)
   - returns "failure" if the `secret` resource differs among components. In oder to compare the `secret`:
     - use `encryptionconfig.FromSecret` function that returns `EncryptionConfiguration`
     - use `reflect.DeepEqual` function to compare `EncryptionConfigurration.Resources` for example `reflect.DeepEqual(openshiftAPIServerEncryptionCfg.Resources, oauthAPIServerEncryptionCfg.Resources)`
   - returns "success" otherwise
6. Change `NewRevisionLabelPodDeployer` to conditionally keep the secrets in synchronization. 
7. Update `OAS-O` to use `UnionRevisionLabelPodDeployer` passing two `RevisionLabelPodDeployer`. The first one for `openshift-apiserver` (already existing) and the second one for `oauth-apiserver`.
   - it will report whether all instances of `oauth-apiserver` converged to the same revision
   - make sure that `oauth-apiserver` deployer won't synchronize the secrets

### User Stories [optional]

#### Story 1

#### Story 2

### Risks and Mitigations


## Design Details

### Test Plan

To validate if `CAO` is capable of managing its own `encryption-config` we are going to create the following E2E tests:
 - scenario 1: turn on encryption
 - scenario 2: turn encryption on and off
 - scenario 3: turn on encryption and force key rotation
 - scenario 4: measure migration performance
  
Note: the above tests will be created based on the common test library that drives the same set of tests for `OAS-O` and `KAS-O`.

To validate upgrade / downgrade path for `4.n` and `4.(n-1)` we are going to create the following E2E tests:
 - scenario 1: take `4.(n-1)` cluster, encrypt it, upgrade to `4.n`, downgrade and upgrade again
 - scenario 2: take `4.(n-1)` cluster, encrypt it, upgrade to `4.n`, force key rotation, downgrade to `4.(n-1)`, force key rotation and upgrade again
 
Note: At the moment we don't have tests like that so creating them will be significantly harder.

To validate upgrade / downgrade path for future release `4.(n+1)` we are going to create the following E2E tests:
 - scenario 1: take `4.n` cluster, encrypt it but let `OAS-O` manage `encryption-config`, upgrade to the next version in which `CAO` will take over the config then downgrade
 - scenario 2: take `4.n` cluster, encrypt it but let `OAS-O` manage `encryption-config`, upgrade, force key rotation, downgrade, force key rotation and upgrade again

##### Removing a deprecated feature

See upgrade/downgrade.

### Upgrade / Downgrade Strategy for `4.n` and `4.(n-1)`

In an upgrade case, `OAS-O` in version `4.n` will be responsible for synchronizing and maintaining encryption state for both `openshift-apiserver` and `oauth-apiserver`.
Since both will share exactly the same `encryption-config`, the new component will be able to read (decrypt) already encrypted data.

During an upgrade, the new `UnionRevisionLabelPodDeployer` will create back pressure in the system and will make the encryption controllers wait for the new component to synchronize. 
In that state, the status of `OAS-O` won't change and we won't roll out any new encryption keys.

In a downgrade scenario, `OAS-O` in version `4.(n-1)` is responsible for synchronizing and maintaining encryption state only for `openshift-apiserver`.
Since in version `4.n` exactly the same `encryption-config` was used the `openshift-apiserver` will be able to read (decrypt) data. 


### Upgrade / Downgrade Strategy for `4.(n+1)` and `4.n`

In an upgrade case, `CAO` in version `4.(n+1)` takes over `encryption-config-openshift-ouath` by removing the annotation and copying the encryption keys (`encryption-key-openshift-apiserver-{0,1, ...}`).
From that point it will manage its own configuration.

In an downgrade scenario nothing changes because `OAS-O` in version `4.n` will not manage `encryption-config-openshift-ouath` as it doesn't have the annotation. 
Additionally `CAO` in version `4.n` was prepared to take care of its own configuration.

### Version Skew Strategy

See the upgrade/downgrade strategy.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Alternatives

1. Turn off encryption before upgrading to new version and turn it on right after. It would be simple but not desirable by the end users. 

## Infrastructure Needed [optional]
