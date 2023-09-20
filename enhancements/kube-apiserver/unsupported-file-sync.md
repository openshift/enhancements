---
title: unsupported-file-sync
authors:
  - stlaz
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - tkashem
  - deads2k
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - tkashem
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
tracking-link: 
  - https://issues.redhat.com/browse/AUTH-387
see-also: []
replaces: []
superseded-by: []
---

# Unsupported file synchronization into kube-apiserver static pods

## Summary

Since kube-apiserver pods are running as static, it's hard to test upcoming features
that might require configuration via files. This enhancement describes how to synchronize
files to kube-apiserver pods in an unsupported mode that matches the testing use-case.

## Motivation

Simplify testing features that might require additional files to be installed into
kube-apiserver pods.

### User Stories

* As a developer, I want to be able to create additional configuration/trust files inside of the kube-apiserver pod

### Goals

- allow mounting unsupported content as files in the kube-apiserver static pods

### Non-Goals

- allow creating files anywhere on a node

## Proposal

The `openshift-kube-apiserver-operator` namespace gets a new revisioned
secret `unsupported-kube-apiserver-content` that is going to be used to
install content inside of the `/etc/kubernetes/unsupported/kube-apiserver` directory.
This secret will be populated by an observer inside of the cluster-kube-apiserver-operator.
The observer watches the `unsupported-kube-apiserver-content` secret in
the `openshift-config` namespace and synchronizes its content to the
secret inside of the `openshift-kube-apiserver-namespace`.

### Workflow Description

The user adds content to the `data` and `stringData` maps in the
`openshift-config/unsupported-kube-apiserver-content` secret and
waits for a rollout of a new kube-apiserver revision that contains their changes.

### API Extensions

No API extensions.

### Implementation Details

#### New kube-apiserver operator observer

A new observer is added to the cluster-kube-apiserver-operator. This observer
watches the `data` and `stringData` fields of the `openshift-config/unsupported-kube-apiserver-content`
secrets and synchronizes them to the `openshift-kube-apiserver-operator/unsupported-kube-apiserver-content`
secret.

If the `data` or `stringData` fields of the source secret are non-empty, the observer
sets the `UnsupportedFilesUpgradeable: False` condition in the status of the
`kubeapiserver/cluster` operator object.

#### Installing the unsupported content

`openshift-config/unsupported-kube-apiserver-content` is a revisioned and optional
secret. Its last revision's content gets installed into the `/etc/kubernetes/unsupported/kube-apiserver`
directory.

In order to accommodate these changes, the [installer command](https://github.com/openshift/cluster-kube-apiserver-operator/blob/ab3736b9773f1b9850fb046226a1ca8b4342cb1c/vendor/github.com/openshift/library-go/pkg/operator/staticpod/installerpod/cmd.go#L85-L86)
and the [installer controller](https://github.com/openshift/cluster-kube-apiserver-operator/blob/ab3736b9773f1b9850fb046226a1ca8b4342cb1c/vendor/github.com/openshift/library-go/pkg/operator/staticpod/controller/installer/installer_controller.go#L156)
need to be extended. This functionality should be optional, with an opt-in scheme.

### Risks and Mitigations

**Risk**: somebody breaks their cluster so badly it's no longer recoverable and opens a support case.
**Mitigation**: support shrugs and closes that case when they notice unsupported config was active.

### Drawbacks

This allows installing random files on control-plane nodes' disks. However, we
control the directory where such files are installed, and we specifically mark
any cluster with such a configuration as unsupported.

## Design Details

### Open Questions [optional]

- Should the `*Upgradable: False` condition be reported by a different controller
  that checks an unsupported revision is still active?

### Test Plan

This is unsupported behavior. Test manually.

### Graduation Criteria

Irrelevant.

#### Dev Preview -> Tech Preview

Irrelevant.

#### Tech Preview -> GA

Irrelevant.

#### Removing a deprecated feature

Irrelevant.

### Upgrade / Downgrade Strategy

Irrelevant.

### Version Skew Strategy

Irrelevant.

### Operational Aspects of API Extensions

Irrelevant.

#### Failure Modes

Whoever breaks their cluster this way is the only person responsible to fix it.

#### Support Procedures

Literally none.

## Implementation History

No history at this point.

## Alternatives

N/A.
