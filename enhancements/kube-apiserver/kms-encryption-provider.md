---
title: kms-encryption-provider
authors:
  - "@swghosh"
reviewers:
  - "@dgrisonnet"
  - "@TrilokGeer"
  - "@tkashem"
  - "@rvanderp"
approvers:
  - ""
creation-date: 2024-08-14
last-updated: 2024-08-14
status: implementable
see-also:
  - "/enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
  - "/enhancements/installer/storage-class-encrypted.md"
api-approvers:
  - ""
tracking-link:
  - https://issues.redhat.com/browse/API-1684
  - https://issues.redhat.com/browse/OCPSTRAT-108
---


# KMS Encryption Provider for etcd Secrets

## Summary

Provide a user-configurable interface to support encryption of data stored in etcd using a supported Key Management Service (KMS).

## Motivation

Today, we support local AES encryption at the datastore layer which protects against etcd data leaks in the event of a etcd backup compromise. However, aescbc and aesgcm which are supported ecncryption technologies today available in OpenShift do not protect against online host compromise i.e. in such cases attacker can decrypt encrypted data from etcd using local keys, KMS managed keys protects against such scenarios.

Users of OpenShift would like the encrypt secret data in etcd using self-managed KMS backed keys.
- https://issues.redhat.com/browse/OCPSTRAT-108

### User Stories

As an OpenShift administrator, I want to encrypt secrets in my cluster at rest using KMS keys so that I can comply with my organization's security requirements.

As an OpenShift administrator, I want to let the KMS provider manage the lifecycle of the encryption keys. 

As an OpenShift user, I enable encryption by setting apiserver.spec.encryption.type to kms.
- After some time passes, user makes a backup of etcd.
- The user confirms that the secret values are encrypted by checking to see if they have the related kms prefix.

### Goals

1. User can enable encryption (globally, with limited config)
2. The keys used to encrypt are managed by the KMS instance (outside of the OpenShift control plane)
3. The keys used to encrypt are periodically rotated automatically 
4. Allow encryption to be disabled once it has been enabled
5. Encryption at rest should not meaningfully degrade the performance of the cluster
6. Allow the user to configure which KMS instance and keys are to be used, adopt KMSv2 from upstream

### Non-Goals

1. Allow the user to force key rotation
2. Support for the complete lifecyle of KMS managed keys directly within OpenShift control plane
3. Support for hardware security modules
4. The user has in-depth understanding of each phase of the encryption process
5. Completely recover the cluster in the event of the KMS instance itself going down or keys getting lost
6. Allow users to configure which resources will be encrypted

### Proposal

OpenShift would need to align closer with KMS evolution upstream with respect to the different Kubernetes Encryption Providers available today. 

Adding a new `EncryptionType` to the existing `APIServer` config:

```diff
diff --git a/config/v1/types_apiserver.go b/config/v1/types_apiserver.go
index 59b89388..abe1c8ae 100644
--- a/config/v1/types_apiserver.go
+++ b/config/v1/types_apiserver.go
@@ -202,6 +202,11 @@ const (
        // aesgcm refers to a type where AES-GCM with random nonce and a 32-byte key
        // is used to perform encryption at the datastore layer.
        EncryptionTypeAESGCM EncryptionType = "aesgcm"
+
+       // kms refers to a type of encryption where the encryption keys are managed
+       // outside the control plane in a Key Management Service instance,
+       // encryption is still performed at the datastore layer.
+       EncryptionTypeKMS EncryptionType = "kms"
 )
 
 type APIServerStatus struct {
```

The default value today is an empty string which implies identity and that no encryption is used in the cluster by default. Other possible local encryption schemes include `aescbc` and `aesgcm` which will remain as-is. Similar to how local AES encryption works, the kube-apiserver operator and openshift-apiserver operator will observe this config to apply the KMS Encryption Provider config onto kube-apiserver(s) and openshift-apiserver(s) respectively.

### Implementation Details/Notes/Constraints

Extend the existing KMSv2 encryption provider from upstream kubernetes apiserver and allow users to configure a KMS plugin provider of their choice to use for etcd secret encryption.

Today, apiserver encryption encrypts only the following resources (and the same would remain unchanged going forward):
- corev1.Secret
- corev1.ConfigMap
- routev1.Route
- oauthv1.AccessToken
- oauthv1.AuthorizeToken

The addition of KMS encryption provider requires addition of a gRPC unix socket running in each of the control plane nodes which will use hostPath mount by the kms provider plugin. The implementation of each KMS provider's plugin is OpenShift agnostic but apiserver(s) will re-use the same directory from the host to perform Encrypt/Decrypt/Status gRPC calls to the running plugin.

For our initial iteration, we plan to manage the lifecycle of the plugin pods however not the plugin itself; this can change in the future.

## Design Details


## Implementation History

1. Local AES encryption: https://github.com/openshift/enhancements/blob/master/enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md
2. Encryption controllers: https://github.com/openshift/library-go/tree/master/pkg/operator/encryption
3. KMS PoC in OpenShift: https://github.com/openshift/cluster-kube-apiserver-operator/pull/1625
4. Upstream kubernetes KMS encryption provider: https://github.com/kubernetes/apiserver/tree/master/pkg/storage/value/encrypt/envelope

## Drawbacks

1. Core apiserver operators need host access to mount and manage permissions for the directory where the kms plugin runs.
2. Can cause problems during disaster recovery and backups in case KMS keys becomes unavailable after


## Alternatives

None

## Infrastructure Needed [optional]

Some extra tooling and or configuration may be required from CI infrastructure pool to request access to cloud KMS instances especially, AWS KMS instances initially. The same can be integrated into an e2e test.
