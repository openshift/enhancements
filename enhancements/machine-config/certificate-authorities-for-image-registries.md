---
title: certificate-authorities-for-image-registries
authors:
  - dmage
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - @sinnykumari
approvers:
  - TBD, who can serve as an approver?
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - TBD
creation-date: 2022-12-01
last-updated: 2022-12-01
tracking-link:
  - https://issues.redhat.com/browse/MCO-499
see-also:
replaces:
superseded-by:
---

# Additional trusted certificate authorities for image registries

## Summary

This enhancement describes how certificate authorities for image registries
should be distributed to CRI-O, openshift-apiserver, and other clients when the
image registry operator is not installed.

## Motivation

The image registry operator manages a daemon set that distributes certificate
authorities for the integrated image registry, and it also distributes
certificate authorities for external registries that are provided by the
cluster administrator in the image config. This makes the image registry
operator required for the cluster while the integrated image registry is
optional and can be removed.

The goal of this enhancement is to move this required functionality to an
essential cluster operator, and have the image registry operator only manage
the optional registry workload.

### User Stories

* As a <role>, I want to <take some action> so that I can <accomplish a
goal>.

### Goals

* Make additionalTrustedCA be managed by machine-config-operator.
* Create a mechanism for the cluster-image-registry-operator to distribute
  certificate authorities for the integrated image registry.
* Remove the node-ca daemon set.

### Non-Goals

* Change additionalTrustedCA behavior for its users.

## Proposal

Create a mechanism inside machine-config-operator that will replace the
node-ca daemon set.

This mechanism should handle and distribute to all nodes certificate
authorities from the user-provided config map that is specified in
`images.config.openshift.io/cluster`. It should also observe a config map in
the `openshift-config-managed` namespace and merge its content with the
user-provided config map. This will allow cluster-image-registry-operator to
provide certificate authorities for the integrated image registry without
changing the user-provided config map.

Once this mechanism is created, the node-ca daemon set should be removed.

### Workflow Description

Let's suppose the user creates a config map with the certificate authorities
for their registries:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: registry-ca
  namespace: openshift-config
data:
  registry.example.com: |
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
  registry-with-port.example.com..5000:
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
```

Then updates the `images.config.openshift.io/cluster` object to point to this
config map:

```yaml
spec:
  additionalTrustedCA:
    name: registry-ca
```

If the image registry operator is installed, there should be a config map:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: image-registry-ca
  namespace: openshift-config-managed
data:
  image-registry.openshift-image-registry.svc..5000: |
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
  image-registry.openshift-image-registry.svc.cluster.local..5000: |
    -----BEGIN CERTIFICATE-----
    ...
    -----END CERTIFICATE-----
```

The machine-config-operator should update every node to propagate these
certificate authorities to CRI-O, so it should create these files:

* `/etc/docker/certs.d/registry.example.com/ca.crt`
* `/etc/docker/certs.d/registry-with-port.example.com:5000/ca.crt`
* `/etc/docker/certs.d/image-registry.openshift-image-registry.svc:5000/ca.crt`
* `/etc/docker/certs.d/image-registry.openshift-image-registry.svc.cluster.local:5000/ca.crt`

If the config maps are updated, old certificate authorities should be removed
and new ones should be added. Nodes nor applications shouldn't be restarted to
apply changes.

There are also consumers that don't have access to the host file system:
cluster-openshift-apiserver-operator needs a merged config map with all
certificate authorities for image registries, so that openshift-apiserver can
access registries.

For them, the mechanism should create a merged config map in
`openshift-config-managed`.

### API Extensions

TBD

### Risks and Mitigations

There are might be consumers that expect the merged config map with certificate
authorities in the `openshift-image-registry` namespace. The image registry
operator should copy the config map from `openshift-config-managed` to
`openshift-image-registry`.

### Drawbacks

TBD

## Design Details

### Open Questions [optional]

### Test Plan

### Graduation Criteria

#### Dev Preview -> Tech Preview

#### Tech Preview -> GA

#### Removing a deprecated feature

### Upgrade / Downgrade Strategy

TBD

### Version Skew Strategy

### Operational Aspects of API Extensions

#### Failure Modes

#### Support Procedures

## Implementation History

## Alternatives
