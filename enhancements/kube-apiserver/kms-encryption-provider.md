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
index d815556d2..c9098024f 100644
--- a/config/v1/types_apiserver.go
+++ b/config/v1/types_apiserver.go
@@ -173,6 +173,9 @@ type APIServerNamedServingCert struct {
 	ServingCertificate SecretNameReference `json:"servingCertificate"`
 }
 
+// APIServerEncryption is used to encrypt sensitive resources on the cluster.
+// +openshift:validation:FeatureGateAwareXValidation:featureGate=KMSEncryptionProvider,rule="has(self.type) && self.type == 'KMS' ?  has(self.kms) : !has(self.kms)",message="kms config is required when encryption type is KMS, and forbidden otherwise"
+// +union
 type APIServerEncryption struct {
 	// type defines what encryption type should be used to encrypt resources at the datastore layer.
 	// When this field is unset (i.e. when it is set to the empty string), identity is implied.
@@ -191,9 +194,23 @@ type APIServerEncryption struct {
 	// +unionDiscriminator
 	// +optional
 	Type EncryptionType `json:"type,omitempty"`
+
+	// kms defines the configuration for the external KMS instance that manages the encryption keys,
+	// when KMS encryption is enabled sensitive resources will be encrypted using keys managed by an
+	// externally configured KMS instance.
+	//
+	// The Key Management Service (KMS) instance provides symmetric encryption and is responsible for
+	// managing the lifecyle of the encryption keys outside of the control plane.
+	// This allows integration with an external provider to manage the data encryption keys securely.
+	//
+	// +openshift:enable:FeatureGate=KMSEncryptionProvider
+	// +unionMember
+	// +optional
+	KMS *KMSConfig `json:"kms,omitempty"`
 }
 
-// +kubebuilder:validation:Enum="";identity;aescbc;aesgcm
+// +openshift:validation:FeatureGateAwareEnum:featureGate="",enum="";identity;aescbc;aesgcm
+// +openshift:validation:FeatureGateAwareEnum:featureGate=KMSEncryptionProvider,enum="";identity;aescbc;aesgcm;KMS
 type EncryptionType string
 
 const (
@@ -208,6 +225,11 @@ const (
 	// aesgcm refers to a type where AES-GCM with random nonce and a 32-byte key
 	// is used to perform encryption at the datastore layer.
 	EncryptionTypeAESGCM EncryptionType = "aesgcm"
+
+	// kms refers to a type of encryption where the encryption keys are managed
+	// outside the control plane in a Key Management Service instance,
+	// encryption is still performed at the datastore layer.
+	EncryptionTypeKMS EncryptionType = "KMS"
 )
 
 type APIServerStatus struct {
diff --git a/config/v1/types_kmsencryption.go b/config/v1/types_kmsencryption.go
new file mode 100644
index 000000000..575affae6
--- /dev/null
+++ b/config/v1/types_kmsencryption.go
@@ -0,0 +1,50 @@
+package v1
+
+// KMSConfig defines the configuration for the KMS instance
+// that will be used with KMSEncryptionProvider encryption
+// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'AWS' ?  has(self.aws) : !has(self.aws)",message="aws config is required when kms provider type is AWS, and forbidden otherwise"
+// +union
+type KMSConfig struct {
+	// type defines the kind of platform for the KMS provider.
+	// Available provider types are AWS only.
+	//
+	// +unionDiscriminator
+	// +kubebuilder:validation:Required
+	Type KMSProviderType `json:"type"`
+
+	// aws defines the key config for using an AWS KMS instance
+	// for the encryption. The AWS KMS instance is managed
+	// by the user outside the purview of the control plane.
+	//
+	// +unionMember
+	// +optional
+	AWS *AWSKMSConfig `json:"aws,omitempty"`
+}
+
+// AWSKMSConfig defines the KMS config specific to AWS KMS provider
+type AWSKMSConfig struct {
+	// keyARN specifies the Amazon Resource Name (ARN) of the AWS KMS key used for encryption.
+	// The value must adhere to the format `arn:aws:kms:<region>:<account_id>:key/<key_id>`, where:
+	// - `<region>` is the AWS region consisting of lowercase letters and hyphens followed by a number.
+	// - `<account_id>` is a 12-digit numeric identifier for the AWS account.
+	// - `<key_id>` is a unique identifier for the KMS key, consisting of lowercase hexadecimal characters and hyphens.
+	//
+	// +kubebuilder:validation:Required
+	// +kubebuilder:validation:XValidation:rule="self.matches('^arn:aws:kms:[a-z0-9-]+:[0-9]{12}:key/[a-f0-9-]+$') && self.size() <= 128",message="keyARN must follow the format `arn:aws:kms:<region>:<account_id>:key/<key_id>`. The account ID must be a 12 digit number and the region and key ID should consist only of lowercase hexadecimal characters and hyphens (-)."
+	KeyARN string `json:"keyARN"`
+	// region specifies the AWS region where the KMS intance exists, and follows the format
+	// `<region-prefix>-<region-name>-<number>`, e.g.: `us-east-1`.
+	// Only lowercase letters and hyphens followed by numbers are allowed.
+	//
+	// +kubebuilder:validation:XValidation:rule="self.matches('^[a-z]{2}-[a-z]+-[0-9]+$') && self.size() <= 64",message="region must be a valid AWS region"
+	Region string `json:"region"`
+}
+
+// KMSProviderType is a specific supported KMS provider
+// +kubebuilder:validation:Enum="";AWS
+type KMSProviderType string
+
+const (
+	// AWSKMSProvider represents a supported KMS provider for use with AWS KMS
+	AWSKMSProvider KMSProviderType = "AWS"
+)
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
