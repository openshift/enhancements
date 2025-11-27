---
title: kms-encryption-provider-at-datastore-layer
authors:
  - "@ardaguclu"
  - "@dgrisonnet"
  - "@flavianmissi"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@derekwaynecarr"
  - "@ibihim"
  - "@sjenning"
  - "@tkashem"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@benluddy"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - "@JoelSpeed"
creation-date: 2025-10-17
last-updated: yyyy-mm-dd
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - "https://issues.redhat.com/browse/OCPSTRAT-108"  # TP feature
  - "https://issues.redhat.com/browse/OCPSTRAT-1638" # GA feature
see-also:
  - "enhancements/kube-apiserver/encrypting-data-at-datastore-layer.md"
  - "enhancements/etcd/storage-migration-for-etcd-encryption.md"
replaces:
  - "https://github.com/openshift/enhancements/pull/1682"
superseded-by:
  - ""
---

# KMS Encryption Provider at Datastore Layer

## Summary

Provide a user-configurable interface to support encryption of data stored in
etcd using a supported [Key Management Service (KMS)](https://kubernetes.io/docs/tasks/administer-cluster/kms-provider/).

## Motivation

OpenShift supports AES encryption at the datastore layer using local keys.
It protects against etcd data leaks in the event of an etcd backup compromise.
However, aescbc and aesgcm, which are supported encryption technologies today
available in OpenShift do not protect against online host compromise i.e. in
such cases, attackers can decrypt encrypted data from etcd using local keys,
KMS managed keys protects against such scenarios since the keys are stored and
managed externally.

### User Stories

* As a cluster admin, I want the APIServer config to be the single source of
  etcd encryption configuration for my cluster, so that I can easily manage all
  encryption related configuration in a single place
* As a cluster admin, I want the kas-operator to manage KMS plugin lifecycle on
  my behalf, so that I don’t need to do any manual work when configuring KMS
  etcd encryption for my cluster
* As a cluster admin, I want to easily understand the operations done by CKASO
  when managing the KMS plugin lifecycle via Conditions in the APIServer CR’s
  Status
* As a cluster admin, I want to be able to switch to a different KMS plugin,
  i.e. from AWS to a pre-installed Vault, by performing a single configuration
  change without needing to perform any other manual intervention or manually
  migrating data
    * TODO: confirm this requirement
* As a cluster admin, I want to configure my chosen KMS to automatically rotate
  encryption keys and have OpenShift to automatically become aware of these new
  keys, without any manual intervention
* As a cluster admin, I want to know when anything goes wrong during key
  rotation, so that I can manually take the necessary actions to fix the state
  of the cluster

### Goals

* Users have an easy to use interface to configure KMS encryption
* Users will configure OpenShift clusters to use one of the supported KMS
  providers
* Encryption keys managed by the KMS (i.e. KEKs), and are not stored in the
  cluster
* Encryption keys are rotated by the KMS, and the configuration is managed by
  the user
* OpenShift clusters automatically detect KMS key rotation and react
  appropriately
* Users can disable encryption after enabling it
* Overall cluster performance should be similar to other encryption mechanisms
* OpenShift will manage KMS plugins' lifecycle on behalf of the users
* Provide users with the means to monitor the state of KMS plugins and KMS
  itself

### Non-Goals

* Support for users to control what resources they want to encrypt
* Support for OpenShift managed encryption keys in KMS
* Direct support for hardware security models (these might still be supported
  via KMS plugins, i.e. Hashicorp Vault or Thales)
* Full data recovery in cases where the KMS key is lost
* Support for users to specify which resources they want to encrypt
* Immediate encryption: OpenShift's encryption works in an eventual model, i.e
  it takes OpenShift several minutes to encrypt all the configured resources in
  the cluster after encryption is initially enable. This means that even if
  cluster admins enable encryption immediately after cluster creation, OpenShift
  may still store unencrypted secrets in etcd. However, OpenShift will eventually
  migrate all secrets (and other to-be-encrypted resources) to use encryption,
  so they will all _eventually_ be encrypted in etcd

## Proposal

To support KMS encryption in OpenShift, we will leverage the work done in
[upstream Kubernetes](https://github.com/kubernetes/enhancements/tree/master/keps/sig-auth/3299-kms-v2-improvements).
However, we will need to extend and adapt the encryption workflow in OpenShift
to support new constraints introduced by the externalization of encryption keys
in a KMS. Because OpenShift will not own the keys from the KMS, we will detect
the KMS-related failures and surface them to the cluster admins for any
necessary actions.

We focus on supporting KMS v2 only, as KMS v1 has considerable performance
impact in the cluster.

#### API Extensions

We will extend the APIServer config to add a new `kms` encryption type alongside
the existing `aescbc` and `aesgcm` types. Unlike `aescbc` and `aesgcm`, KMS
will require additional input from users to configure their KMS provider, such
as connection details, authentication credentials, and key references. From a
UX perspective, this is the only change the KMS feature introduces—it is
intentionally minimal to reduce user burden and potential for errors.

##### Encryption Controller Extensions

This feature will reuse existing encryption and migration workflows while
extending them to handle externally-managed keys. We will introduce a new
controller to manage KMS plugin pod lifecycle and integrate KMS plugin health
checks into the existing controller precondition system.

##### KMS Plugin Lifecycle

KMS encryption requires KMS plugin pods to bridge communication between the
kube-apiserver and the external KMS. In OpenShift, the kube-apiserver-operator
will manage these plugins on behalf of users, reducing operational complexity
and ensuring consistent behavior across the platform. The operator will handle
plugin deployment, health monitoring, and lifecycle management during key
rotation events.

### Workflow Description

#### Roles

**cluster admin** is a human user responsible for the overall configuration and
maintainenance of a cluster.

**KMS** is the cloud Key Management Service responsible for managing the full
lifecycle of the Key Encryption Key (KEK), including automatic rotation.

#### Initial Resource Encryption

1. The cluster admin creates an encryption key (KEK) in their KMS of choice
1. The cluster admin give the OpenShift apiservers access to the newly created
   KMS KEK
1. The cluster admiin updates the APIServer configuration resource, providing
   the necessary [encryption configuration options](encryption-cfg-opts) for
   the KMS of choice
1. The cluster admin observes the `clusteroperator/kube-apiserver` resource
   for progress on the configuration change and encryption of existing resources

[encryption-cfg-opts]: https://github.com/openshift/api/blob/master/config/v1/types_kmsencryption.go#L7-L22

#### KMS Plugin Management

1. The cluster admin configures encryption in the cluster
1. The KMS plugin controller generates a unix socket name, unique for this
   encryption configuration
1. The KMS plugin controller generates a pod manifest for the configured cloud
   KMS, setting the `key_id`, the unix socket name generated in the previous,
   and any other configurations required by the KMS plugin in question
1. The KMS plugin controller watches the `key_id` from the KMS plugin Status
   gRPC endpoint, and when it detects it has changed, it configures the KMS
   plugin to use the new key _in addition to the current key_
   * TODO: should the KMS plugin controller also watch the encryption key secret
     for changes?
1. The KMS plugin controller watches the migration status of encrypted
   resources, and once migration finishes, it configures the KMS plugin to only
   use the new key, removing the previous one from the plugin configuration

#### Key rotation

1. The cluster admin configures automatic periodic rotation of the KEK in KMS
1. KMS rotates the KEK
1. OpenShift detects the KEK has been rotated, and starts migrating encrypted
   data to use the new KEK
1. The cluster admin eventually checks the `kube-apiserver` `clusteroperator`
   resource, and sees that the KEK was rotated, and the status of the data
   migration

1. TODO: how will `keyController` learn the unix socket name? it needs to be
   able to call the kms plugin's Status gRPC endpont, and it needs the unix
   socket to do that
1. `stateController` generates the `EncryptionConfig`, using the unix socket
   path generated by the KMS plugin controller, and any other configurations
   required
1. `keyController` watches the `key_id` from the KMS plugin Status gRPC call,
   as well as the APIServer config for changes, and when either of these
   change, it creates a new encryption key secret

#### Change of KMS Provider

1. The cluster admin creates a KEK in a KMS different than the one currently
   configured in the cluster
1. The cluster admin configures the new KMS provider in the APIServer
   configuration resource
1. The cluster detects the encryption configuration change, and starts
   migrating the encrypted data to use the new KMS encryption key
1. The cluster admin observes the `kube-apiserver` `clusteroperator` resource,
   for progress on the configuration change, as well as migration of resources

### API Extensions

While in tech-preview, the KMS feature will be placed behind the
`KMSEncryptionProvider` feature-gate.

Similar to the upstream `EncryptionConfig`'s [`ProviderConfiguration`](https://github.com/kubernetes/apiserver/blob/cccad306d649184bf2a0e319ba830c53f65c445c/pkg/apis/apiserver/types_encryption.go#L89-L101),
we will add a new `EncryptionType` to the existing `APIServer` config:
```diff
diff --git a/config/v1/types_apiserver.go b/config/v1/types_apiserver.go
index d815556d2..c9098024f 100644
--- a/config/v1/types_apiserver.go
+++ b/config/v1/types_apiserver.go
@@ -208,6 +225,11 @@ const (
        // aesgcm refers to a type where AES-GCM with random nonce and a 32-byte key
        // is used to perform encryption at the datastore layer.
        EncryptionTypeAESGCM EncryptionType = "aesgcm"
+
+       // kms refers to a type of encryption where the encryption keys are managed
+       // outside the control plane in a Key Management Service instance,
+       // encryption is still performed at the datastore layer.
+       EncryptionTypeKMS EncryptionType = "KMS"
 )
```

The default value today is an empty string, which implies identity, meaning no
encryption is used in the cluster by default. Other possible local encryption
schemes include `aescbc` and `aesgcm`, which will remain as-is. Similar to how
local AES encryption works, the apiserver operators will observe this config
and apply the KMS `EncryptionProvider` to the `EncryptionConfig`.

```diff
@@ -191,9 +194,23 @@ type APIServerEncryption struct {
        // +unionDiscriminator
        // +optional
        Type EncryptionType `json:"type,omitempty"`
+
+       // kms defines the configuration for the external KMS instance that manages the encryption keys,
+       // when KMS encryption is enabled sensitive resources will be encrypted using keys managed by an
+       // externally configured KMS instance.
+       //
+       // The Key Management Service (KMS) instance provides symmetric encryption and is responsible for
+       // managing the lifecyle of the encryption keys outside of the control plane.
+       // This allows integration with an external provider to manage the data encryption keys securely.
+       //
+       // +openshift:enable:FeatureGate=KMSEncryptionProvider
+       // +unionMember
+       // +optional
+       KMS *KMSConfig `json:"kms,omitempty"`
```

The KMS encryption type will have a dedicated configuration:

```diff
diff --git a/config/v1/types_kmsencryption.go b/config/v1/types_kmsencryption.go
new file mode 100644
index 000000000..8841cd749
--- /dev/null
+++ b/config/v1/types_kmsencryption.go
@@ -0,0 +1,49 @@
+package v1
+
+// KMSConfig defines the configuration for the KMS instance
+// that will be used with KMSEncryptionProvider encryption
+// +kubebuilder:validation:XValidation:rule="has(self.type) && self.type == 'AWS' ?  has(self.aws) : !has(self.aws)",message="aws config is required when kms provider type is AWS, and forbidden otherwise"
+// +union
+type KMSConfig struct {
+       // type defines the kind of platform for the KMS provider
+       //
+       // +unionDiscriminator
+       // +kubebuilder:validation:Required
+       Type KMSProviderType `json:"type"`
+
+       // aws defines the key config for using an AWS KMS instance
+       // for the encryption. The AWS KMS instance is managed
+       // by the user outside the purview of the control plane.
+       //
+       // +unionMember
+       // +optional
+       AWS *AWSKMSConfig `json:"aws,omitempty"`
+}

+// KMSProviderType is a specific supported KMS provider
+// +kubebuilder:validation:Enum=AWS
+type KMSProviderType string
+
+const (
+       // AWSKMSProvider represents a supported KMS provider for use with AWS KMS
+       AWSKMSProvider KMSProviderType = "AWS"
+)
```

This configuration will also include an enum of the various KMS supported by
OCP. For Tech-Preview, it will only have the `AWS` type, but we will add more as we
progress on the feature. This enum is essential to signal users which KMS providers
are currently supported by the platform.

Each KMS type will have a dedicated configuration that will be reflected on the
plugin when installed. It will only contain fields that are relevant to end
users.

```diff
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
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

Are there any unique considerations for making this change work with
Hypershift?

See https://github.com/openshift/enhancements/blob/e044f84e9b2bafa600e6c24e35d226463c2308a5/enhancements/multi-arch/heterogeneous-architecture-clusters.md?plain=1#L282

How does it affect any of the components running in the
management cluster? How does it affect any components running split
between the management cluster and guest cluster?

#### Standalone Clusters

Is the change relevant for standalone clusters?

#### Single-node Deployments or MicroShift

How does this proposal affect the resource consumption of a
single-node OpenShift deployment (SNO), CPU and memory?

How does this proposal affect MicroShift? For example, if the proposal
adds configuration options through API resources, should any of those
behaviors also be exposed to MicroShift admins through the
configuration file for MicroShift?

### Implementation Details/Notes/Constraints

This feature may bring slight degradation of performance due to the reliance of
an external system. However,  overall performance should be similar to other
encryption mechanisms. During migrations, performance depends on the number of
resources that will be migrated.

Enabling KMS encryption requires a KMS plugin running in the cluster so that
the apiservers can communicate through the plugin with the external KMS
provider.

Each KMS provider has a different KMS plugin. OpenShift will manage the entire
lifecycle of KMS plugins.

#### Controller Preconditions and KMS Plugin Health

Encryption controllers should only run when the KMS provider plugin is up and
running. All the encryption controllers take in a preconditionsFulfilled
function as a parameter. The controllers use this to decide whether they should
sync or not. We can leverage this existing mechanism to check if the KMS plugin
is healthy, in addition to the existing checks.

#### Encryption Key Secret Management for KMS

The keyController will continue managing encryption key secrets as it does
today. The difference is that for the KMS encryption provider, the encryption
key secret contents will be empty. This secret must be empty because when the
KMS provider is used the root encryption key (KEK) is stored and managed by KMS
itself. We still want the encryption key secret to exist, even if empty, so
that we can leverage functionality in the existing encryption controllers, thus
having full feature parity between existing encryption providers and the new
KMS encryption provider.

#### Key Rotation and Data Migration

Data migration must happen in the following scenarios:

* The cluster admin enables encryption in the cluster for the first time
* The cluster admin updates the `KMSConfig` in APIServer config
* The KMS automatically rotates the KEK
* The cluster admin manually rotates the KEK

KMS Key rotation does not change the identity of the key in the KMS, it
only changes the key materials, and in most KMS providers it results in a
new version of the same key. Despite that, KMS plugins are required to return a
different `key_id` when the KMS key (KEK) is rotated.

Note that the AWS KMS plugin does not change the `key_id` when the KEK is
rotated. This is an [as of now unreported] bug in the AWS KMS plugin. However,
it should not cause any problems to end-users, since AWS does not expire old
versions of a key. TODO: explain what we mean by "expire".

The encryption controllers in library-go already handle migration of encrypted
resources. The `keyController`, response for creating and rotating keys, need
to change so that the encryption key secret it manages becomes a reflection of
what the `key_id` returned by the KMS plugin Status gRPC call.
TODO: elaborate on the above.


TODO: merge below and above blocks
**Key rotation**
KMS Plugins must return a `key_id` as part of the response to a Status gRPC call.
This `key_id` is authoritative, so when it changes, we must consider the key
rotated, and migrate all encrypted resources to use the new key. The
`keyController` will be updated to perform periodic checks of the `key_id` in
the response to a Status call, and recreate the encryption key secret resource
when it detects a change in `key_id`.
The `key_id` will be stored in the encryption secret resource managed by the
`keyController`. Currently, this resource is used to store key materials for
AES-CBC and AES-GCM keys, so we'll simply reuse this logic, but without storing
key materials when the KMS provider is selected.

Once the encryption key secret resource is recreated as a reaction to a change
in `key_id`, the `migrationController` will detect that a migration is needed,
and will do its job without any modifications.

There is no standardized way to configure a KMS plugin to rotate a key.
For example, the AWS KMS plugin supports two KMS keys to be configured for the
same process, allowing the plugin to run with two keys with different ARNs
without the need for another plugin pod to be configured. Azure KMS plugin on
the other hand, can only be configured with a single key, so if a user creates
a new KMS key, OpenShift must create a whole new plugin pod, and run it in
parallel with the one configured with the previous key. These two pods must run
in parallel until all resources are migrated to the new encryption key.


##### Key Rotation For the AWS KMS Plugin

During Tech-Preview, the AWS KMS plugin will be the only plugin supported.

Rotating a KMS key is always a user-invoked operation. It requires users
to edit the APIserver configuration, setting a new key.
Openshift already automates the necessary [step-by-step changes](https://kubernetes.io/docs/tasks/administer-cluster/encrypt-data/#rotating-a-decryption-key)
to kubernetes' `EncryptionConfig`, including migrating resources encrypted by
the old key to use the new key.

When rotating the key, the AWS KMS plugin pods must be updated to run

##### Key Rotation For the Azure KMS Plugin

The Azure KMS plugin will not be supported in Tech-Preview.

TODO: explain process or two plugin pods running until migration finishes.

TODO: confirm rotation detection works as expected with the Azure plugin: it
requires a key version as a parameter to run, and afaik while rotating a key
doesn't cause it's `key_id` to change (only the key materials), the fact that
the Azure KMS plugin takes in a key version is concerning, because a new
version of the key is created when the key is rotated. This might just mean
that the Status `key_id` will change, and then we need a new pod with just a
version bump. My concern is that the `key_id` will not change. This would make
the plugin incompatible with the KMS plugin interface, but still. I want to be
sure.

#### KMS Plugin Management

##### Requirements

* API servers must have access to the Unix domain socket for the KMS plugin
  (this can be achieve via Kubernetes Services)
* Support running multiple instances of the KMS plugin with different
  encryption configurations. This is required for KEK rotation
* KMS plugins must be authorized to communicate with the KMS
* KMS plugin lifecycle must be fully managed by OpenShift, including
  reconciliation based on APIServer configuration changes
* OpenShift must fully report on plugin status and health. This is expanded
  under Recovery section (TODO: write recovery section)

##### Implementation Approach

KMS plugins are deployed as **sidecar containers** running alongside each of
OpenShift's API servers. Each of the 3 apiserver operators manages its own KMS
plugin sidecar instance.

**Deployment Architecture:**

| API Server          | Deployment Type | hostNetwork | Volume Type | Credential Source | Managed By                              |
|---------------------|-----------------|-------------|-------------|-------------------|-----------------------------------------|
| kube-apiserver      | Static Pod      |    true     | hostPath    | TODO              | cluster-kube-apiserver-operator         |
| openshift-apiserver | Deployment      |    false    | emptyDir    | TODO              | cluster-openshift-apiserver-operator    |
| oauth-apiserver     | Deployment      |    false    | emptyDir    | TODO              | cluster-authentication-operator         |

TODO: document vault and thales credential sources

##### Shared library-go Components

The implementation leverages `library-go/pkg/operator/encryption/kms/` which
provides:

1. **Shared Container Specification**: `ContainerConfig` struct that
   encapsulates KMS plugin container configuration
2. **Volume Management**: Functions to create socket and credential volumes
   based on deployment type
3. **Pod Injection Logic**: `AddKMSPluginToPodSpec()` function that handles
   sidecar injection
4. **Socket Path Generation**: Builds Unix socket paths based on APIServer KMS
   configuration

All three API server operators import and use these shared components, ensuring
consistency across the platform.

##### Configuration Detection

All operators watch the cluster-scoped `config.openshift.io/v1 APIServer`
resource. When `spec.encryption.type` is set to `"KMS"`, operators
automatically inject the KMS plugin sidecar into their respective API server
pods.

The KMS plugin image is specified via the `KMS_PLUGIN_IMAGE` environment
variable on each operator deployment.
To fully automate the process of KMS plugin deployment, we will add supported
KMS plugin images to the OpenShift release payload in GA.

##### Credential Management

**For kube-apiserver (Static Pod with hostNetwork: true):**

The KMS plugin sidecar accesses AWS credentials through the EC2 Instance Metadata Service (IMDS). The master node's IAM role must have the following KMS permissions:

```json
{
  "Effect": "Allow",
  "Action": [
    "kms:Encrypt",
    "kms:Decrypt",
    "kms:DescribeKey",
    "kms:GenerateDataKey"
  ],
  "Resource": "<kms-key-arn>"
}
```

Users are responsible for configuring the master node IAM role with these permissions. A helper script is provided in `library-go/pkg/operator/encryption/kms/master-node-iam-setup.sh`.

**For openshift-apiserver and oauth-apiserver (Deployments with hostNetwork: false):**

These API servers cannot access IMDS directly, so they use AWS credentials from Kubernetes Secrets created by the Cloud Credential Operator (CCO).

CredentialsRequest resources are provided in `library-go/pkg/operator/encryption/kms/`:
- `openshift-apiserver-kms-credentials-request.yaml`
- `oauth-apiserver-kms-credentials-request.yaml`

When CCO operates in **Mint mode**, it automatically creates IAM users and provisions the `kms-credentials` secret in each API server's namespace. The operators watch for these secrets and only inject the KMS plugin sidecar once the credentials are available.

**Graceful Degradation:**
If KMS encryption is enabled but credentials aren't ready, operators:
1. Log a warning indicating credentials are pending
2. Skip sidecar injection (return nil, not error)
3. Allow the deployment to proceed without the KMS sidecar
4. Automatically inject the sidecar on the next reconciliation when credentials become available

This prevents blocking API server rollouts while waiting for CCO to provision credentials.

##### Sidecar Injection Mechanism

Each operator injects the KMS plugin sidecar at a specific point in its reconciliation loop:

**kube-apiserver-operator:**
- Injection point: `targetconfigcontroller.managePods()`
- Modifies the static pod manifest before writing to the pod ConfigMap
- Uses `hostPath` volume pointing to `/var/run/kmsplugin` on the host

**openshift-apiserver-operator:**
- Injection point: `workload.manageOpenShiftAPIServerDeployment_v311_00_to_latest()`
- Modifies the deployment spec after setting input hashes
- Uses `emptyDir` volume for socket isolation

**authentication-operator (oauth-apiserver):**
- Injection point: `workload.syncDeployment()`
- Modifies the deployment spec after setting input hashes
- Uses `emptyDir` volume for socket isolation

##### Socket Communication

The API server and KMS plugin communicate via a Unix domain socket:
- **Socket path**: `/var/run/kmsplugin/socket.sock`
- **Volume name**: `kms-plugin-socket`
- **Protocol**: gRPC over Unix domain socket (KMS v2 API)

The socket path can be customized based on the KMS configuration to support multiple concurrent KMS providers if needed.

##### Reactivity and Updates

All operators watch:
1. **APIServer resource**: Triggers reconciliation when encryption type or KMS config changes
2. **Secrets** (for Deployment-based API servers): Triggers reconciliation when credentials are created or updated
3. **Operator environment variables**: `KMS_PLUGIN_IMAGE` changes trigger operator pod restart and subsequent sidecar updates

When the `keyARN` is updated in the APIServer configuration:
1. Operators detect the configuration change
2. New deployment/static pod revision is created with updated KMS configuration
3. Rolling update replaces old pods with new ones
4. Old pods continue serving requests until new pods are ready
5. No socket path conflicts occur due to pod-level volume isolation (emptyDir) or sequential rollout (static pods)

##### Alternative Approaches Considered

See the [Alternatives](#alternatives-not-implemented) section for details on shared KMS plugin deployment models that were not selected.


### Risks and Mitigations

#### Loss of encryption key

TODO

* Ensure KMS key is configured with grace period after key deletion
* In-memory caches of the unencrypted DEK seed
* Monitoring and alerts in place to detect when the KMS key has been deleted
  and not followed by a `key_id` change
  * deleted keys cannot be used for encryption, only decryption. we can use
    use this (along with an unchanged `key_id`) to detect when a key was deleted

#### Temporary Cloud KMS Outages

TODO

* In-memory caches of the unencrypted DEK seed

----

What are the risks of this proposal and how do we mitigate. Think broadly. For
example, consider both security and how this will impact the larger OKD
ecosystem.

How will security be reviewed and by whom?

How will UX be reviewed and by whom?

Consider including folks that also work outside your immediate sub-project.

### Drawbacks

The idea is to find the best form of an argument why this enhancement should
_not_ be implemented.

What trade-offs (technical/efficiency cost, user experience, flexibility,
supportability, etc) must be made in order to implement this? What are the reasons
we might not want to undertake this proposal, and how do we overcome them?

Does this proposal implement a behavior that's new/unique/novel? Is it poorly
aligned with existing user expectations?  Will it be a significant maintenance
burden?  Is it likely to be superceded by something else in the near future?

## Alternatives (Not Implemented)

Similar to the `Drawbacks` section the `Alternatives` section is used
to highlight and record other possible approaches to delivering the
value proposed by an enhancement, including especially information
about why the alternative was not selected.

## Open Questions [optional]

This is where to call out areas of the design that require closure before deciding
to implement the design.  For instance,
 > 1. This requires exposing previously private resources which contain sensitive
  information.  Can we do this?

## Test Plan

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?
- What additional testing is necessary to support managed OpenShift service-based offerings?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:

- Maturity levels
  - [`alpha`, `beta`, `stable` in upstream Kubernetes][maturity-levels]
  - `Dev Preview`, `Tech Preview`, `GA` in OpenShift
- [Deprecation policy][deprecation-policy]

Clearly define what graduation means by either linking to the [API doc definition](https://kubernetes.io/docs/concepts/overview/kubernetes-api/#api-versioning),
or by redefining what graduation means.

In general, we try to use the same stages (alpha, beta, GA), regardless how the functionality is accessed.

[maturity-levels]: https://git.k8s.io/community/contributors/devel/sig-architecture/api_changes.md#alpha-beta-and-stable-versions
[deprecation-policy]: https://kubernetes.io/docs/reference/using-api/deprecation-policy/

**If this is a user facing change requiring new or updated documentation in [openshift-docs](https://github.com/openshift/openshift-docs/),
please be sure to include in the graduation criteria.**

**Examples**: These are generalized examples to consider, in addition
to the aforementioned [maturity levels][maturity-levels].

### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers
- Enumerate service level indicators (SLIs), expose SLIs as metrics
- Write symptoms-based alerts for the component(s)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- User facing documentation created in [openshift-docs](https://github.com/openshift/openshift-docs/)

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

## Upgrade / Downgrade Strategy

If applicable, how will the component be upgraded and downgraded? Make sure this
is in the test plan.

Consider the following in developing an upgrade/downgrade strategy for this
enhancement:
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to keep previous behavior?
- What changes (in invocations, configurations, API use, etc.) is an existing
  cluster required to make on upgrade in order to make use of the enhancement?

Upgrade expectations:
- Each component should remain available for user requests and
  workloads during upgrades. Ensure the components leverage best practices in handling [voluntary
  disruption](https://kubernetes.io/docs/concepts/workloads/pods/disruptions/). Any exception to
  this should be identified and discussed here.
- Micro version upgrades - users should be able to skip forward versions within a
  minor release stream without being required to pass through intermediate
  versions - i.e. `x.y.N->x.y.N+2` should work without requiring `x.y.N->x.y.N+1`
  as an intermediate step.
- Minor version upgrades - you only need to support `x.N->x.N+1` upgrade
  steps. So, for example, it is acceptable to require a user running 4.3 to
  upgrade to 4.5 with a `4.3->4.4` step followed by a `4.4->4.5` step.
- While an upgrade is in progress, new component versions should
  continue to operate correctly in concert with older component
  versions (aka "version skew"). For example, if a node is down, and
  an operator is rolling out a daemonset, the old and new daemonset
  pods must continue to work correctly even while the cluster remains
  in this partially upgraded state for some time.

Downgrade expectations:
- If an `N->N+1` upgrade fails mid-way through, or if the `N+1` cluster is
  misbehaving, it should be possible for the user to rollback to `N`. It is
  acceptable to require some documented manual steps in order to fully restore
  the downgraded cluster to its previous state. Examples of acceptable steps
  include:
  - Deleting any CVO-managed resources added by the new version. The
    CVO does not currently delete resources that no longer exist in
    the target version.

## Version Skew Strategy

How will the component handle version skew with other components?
What are the guarantees? Make sure this is in the test plan.

Consider the following in developing a version skew strategy for this
enhancement:
- During an upgrade, we will always have skew among components, how will this impact your work?
- Does this enhancement involve coordinating behavior in the control plane and
  in the kubelet? How does an n-2 kubelet without this feature available behave
  when this feature is used?
- Will any other components on the node change? For example, changes to CSI, CRI
  or CNI may require updating that component before the kubelet.

## Operational Aspects of API Extensions

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Infrastructure Needed [optional]

Use this section if you need things from the project. Examples include a new
subproject, repos requested, github details, and/or testing infrastructure.
