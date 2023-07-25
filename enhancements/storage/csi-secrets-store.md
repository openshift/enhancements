---
title: csi-secrets-store
authors:
  - "@dobsonj"
reviewers:
  - "@jsafrane"
  - "@bertinatto"
  - "@gnufied"
approvers:
  - "@jsafrane"
api-approvers:
  - "@JoelSpeed"
creation-date: 2023-06-09
last-updated: 2023-06-09
tracking-link:
  - https://issues.redhat.com/browse/STOR-676
see-also:
  - "/enhancements/storage/csi-efs-operator.md" # similar CSI driver operator
  - "/enhancements/storage/csi-driver-install.md" # background on other drivers
  - "/enhancements/storage/csi-inline-vol-security.md" # ephemeral volume security
---

# Secrets Store CSI Driver Operator

## Summary

The [Secrets Store CSI Driver](https://github.com/kubernetes-sigs/secrets-store-csi-driver)
allows users to mount secrets from an external secret store (like Azure Key Vault for
example) as an ephemeral volume on a pod. This is done via a CSI driver that supports
ephemeral volumes, along with a provider plugin that communicates with the external
secret provider. This document describes how this driver can be deployed by an
optional operator on OpenShift.

## Motivation

Customers often use external secret stores to meet key requirements and need
a supported way to access those secrets from pods running on an OpenShift cluster.

### User Stories

- As a compliance manager, I want all application secrets to be stored in a secret management system that meets our security requirements, so that we can limit the impact of potential threats and remediate issues quickly.
- As a cluster administrator, I want to allow applications to fetch secrets from the external provider that our organization uses, so that we can leverage existing secret management procedures with minimal overhead.
- As an application developer, I want my application to fetch secrets from our supported secret store automatically, so that secrets can be updated without manual intervention.

### Goals

- Allow cluster administrators to install the Secrets Store CSI Driver via an optional operator.
- Allow applications to mount external secrets as an ephemeral volume attached to the pod.
- Allow partners to publish plugins that support new external secret providers.

### Non-Goals

- This enhancement will not bundle external provider plugins with the CSI driver--cluster admins will need to decide which provider to install in their environment.
- This enhancement will not diverge the CSI driver from upstream code--any missing functionality must go through the upstream process.
- This enhancement will not include developing new provider plugins that do not yet exist.

## Proposal

The Secrets Store CSI Driver will be deployed via an OLM operator. This operator is
optional, and the admin opts-in to using the Secrets Store CSI Driver by installing
the operator. A provider plugin must also be installed to make use of the driver.
The provider plugin can be installed by the cluster admin via a package in the
[Red Hat Ecosystem Catalog](https://catalog.redhat.com/software/search?target_platforms=Red%20Hat%20OpenShift&p=1), or by installing a third-party upstream provider.

1. Implement a new secrets-store-csi-driver-operator using [CSI driver installation functions of library-go](https://github.com/openshift/library-go/tree/master/pkg/operator/csi/csicontrollerset). This is well tested and used by a dozen other CSI driver operators already.
2. secrets-store-csi-driver-operator watches `ClusterCSIDriver` CR named `secrets-store.csi.k8s.io` and deploys the CSIDriver, RBAC roles, and node DaemonSet for the driver.
3. Ship the operator + CSI driver through ART pipeline, same as other CSI drivers.
4. Admin installs operator via OLM, then chooses a third-party provider plugin to install.
5. User can deploy pods with external secrets mounted as inline ephemeral volumes.

### Workflow Description

#### Installing CSI Driver

1. Cluster admin installs CSI driver operator via OLM.
2. Cluster admin creates ClusterCSIDriver instance for the driver (`secrets-store.csi.k8s.io`).
3. Cluster admin installs a third-party provider plugin for their chosen secret store.

See also: [upstream CSI driver installation docs](https://secrets-store-csi-driver.sigs.k8s.io/getting-started/installation.html)

#### Mounting External Secrets to a Pod

1. Pod is created and scheduled to a node.
2. Kubelet issues request to CSI driver to mount the volume.
3. CSI driver sends request for the secret to provider plugin via a Unix domain socket.
4. Provider retrieves the secret from the external secret store and returns it to the CSI driver.
5. CSI driver creates an ephemeral volume, writes the secret to it, and returns the mount path to kubelet.
6. Kubelet reports the volume is mounted and pod is running.

#### Uninstalling CSI Driver

1. Cluster admin stops all application pods that use the `secrets-store.csi.k8s.io` provider.
2. Cluster admin deletes the `secrets-store.csi.k8s.io` ClusterCSIDriver object.
3. The operator removes the CSI driver and associated manifests. Cluster admin verifies the CSI driver pods are no longer running.
4. Cluster admin uninstalls the Secrets Store CSI driver operator via OLM. This should be done only after the CSI driver itself was already removed. Removing the operator does not automatically remove the driver.

### API Extensions

The only planned OpenShift API change is to add [secrets-store.csi.k8s.io](https://github.com/openshift/api/blob/bdd8865676216305be6065ade196f36752dfd015/operator/v1/types_csi_cluster_driver.go#L86) to the list of supported providers in the ClusterCSIDriver object.

When the operator is installed via OLM, two new CRD's are created as required by the CSI driver and provider plugins:
[SecretProviderClasses](https://github.com/openshift/secrets-store-csi-driver/blob/main/deploy/secrets-store.csi.x-k8s.io_secretproviderclasses.yaml) and
[SecretProviderClassPodStatuses](https://github.com/openshift/secrets-store-csi-driver/blob/main/deploy/secrets-store.csi.x-k8s.io_secretproviderclasspodstatuses.yaml).
Once the operator is installed and ClusterCSIDriver object is created, the operator installs a ClusterRole which is used by the CSI driver to read SecretProviderClass objects created by the user, and to create/modify/delete SecretProviderClassPodStatus objects.

### Implementation Details/Notes/Constraints [optional]

There are two optional features that are enabled by the operator by default:

- [Secret Auto Rotation](https://secrets-store-csi-driver.sigs.k8s.io/topics/secret-auto-rotation.html) allows the driver to poll the secret store and update the pod mount when the secret changes. This is enabled by the operator and applies to all SecretProviderClasses on the cluster. The rotation poll interval defaults to [2 minutes](https://github.com/openshift/secrets-store-csi-driver-operator/blob/3238dccf11e3d4bed9011cc3ee27de2e7ee4ea6f/assets/node.yaml#L42).
- [Sync as Kubernetes Secret](https://secrets-store-csi-driver.sigs.k8s.io/topics/sync-as-kubernetes-secret.html) allows external secrets to be mirrored to a kubernetes secret. This is entirely controlled by the user during creation of the SecretProviderClass.

#### Differences from other solutions

[KMS provider](https://kubernetes.io/docs/tasks/administer-cluster/kms-provider/) allows kubernetes secrets to be encrypted in etcd, whereas Secrets Store allows secrets to be stored outside the cluster and still mountable by pods.
Some organizations have compliance policies around secrets management that would require certain credentials to be managed by an external provider (i.e. etcd is not an option for some use cases).

### Risks and Mitigations

An additional driver and operator will add support and maintenance burden,
but this should be offset by addressing a frequently requested use case.

The driver pods must run as privileged in order to create mounts on the host,
but this is no different from other CSI drivers. Both the driver and operator
will go through product security evaluation.

The user experience of needing to install a separate provider plugin after the
operator is installed is not ideal. However, it is not possible for the operator
to reliably derive which provider should be used based on the chosen platform.
It is valid use case to install the Hashicorp Vault provider on a cluster that
is running on Azure or AWS for example. Therefore, the cluster admin must decide
which provider should be supported in their environment.

### Drawbacks

See [Risks and Mitigations](#risks-and-mitigations).

## Design Details

### Inline Ephemeral Volumes

This driver is different from other CSI drivers in that it supports only
[ephemeral volumes](https://github.com/openshift/secrets-store-csi-driver-operator/blob/d271705b2e56b20581dcc04514c9b307b67fbeec/assets/csidriver.yaml#L14)
and not persistent volumes. By design, the lifecycle of these volumes is the same
as the pod itself: the volume gets deleted when the pod gets deleted.
As such, this driver does not have a controller, since there is no provisioning
of persistent volumes through this driver. Only a node DaemonSet is created to support
mounting ephemeral volumes on the node where the pod runs.

See more on inline ephemeral volumes in the
[OpenShift docs](https://docs.openshift.com/container-platform/4.13/storage/container_storage_interface/ephemeral-storage-csi-inline.html)
and the
[CSI Inline Ephemeral Volume Security enhancement](https://github.com/openshift/enhancements/blob/b12cefd0f3fa438ec08f84f19c0934a35efa1877/enhancements/storage/csi-inline-vol-security.md).

The operator will create the `secrets-store.csi.k8s.io` CSIDriver object with
`security.openshift.io/csi-ephemeral-volume-profile: "restricted"` by default
(see the manifest [here](https://github.com/openshift/secrets-store-csi-driver-operator/blob/d271705b2e56b20581dcc04514c9b307b67fbeec/assets/csidriver.yaml#L8)) so that
the driver can be used from `restricted` namespaces by default. The cluster admin
can change this value if they wish to limit the use of this driver to `privileged`
or `baseline` namespaces.

### Publishing New Provider Plugins

Refer to [upstream guidance for provider implementation](https://secrets-store-csi-driver.sigs.k8s.io/providers.html) when developing a new provider plugin. Each provider is expected to:
- Use the functions and data structures in [service.pb.go](https://github.com/kubernetes-sigs/secrets-store-csi-driver/blob/main/provider/v1alpha1/service.pb.go) to implement the server code.
- Run a DaemonSet that is deployed to the same nodes as the [CSI driver DaemonSet](https://github.com/openshift/secrets-store-csi-driver-operator/blob/main/assets/node.yaml).
- Create a Unix Domain Socket under `/var/run/secrets-store-csi-providers` on the host, which the [CSI driver uses](https://github.com/openshift/secrets-store-csi-driver-operator/blob/d271705b2e56b20581dcc04514c9b307b67fbeec/assets/node.yaml#L38) to communicate with the provider.

Some partners may want to publish a provider to the
[Red Hat Ecosystem Catalog](https://catalog.redhat.com/software/search?target_platforms=Red%20Hat%20OpenShift&p=1)
to deploy the provider plugin. In that case, the provider plugin should
[specify a dependency in OLM](https://docs.openshift.com/container-platform/4.13/operators/understanding/olm/olm-understanding-dependency-resolution.html) on the
[Secret Store CSI Driver Operator](https://github.com/openshift/secrets-store-csi-driver-operator).

### Open Questions [optional]

N/A

### Test Plan

- Run unit tests for driver and operator as part of presubmit hooks
- Implement e2e tests for operator installation
- Use e2eprovider from the driver (dummy provider) to run e2e tests in our CI to test generic CSI driver functionality
- Manual testing by dev and QE with a third-party provider like [Azure Key Vault](https://azure.github.io/secrets-store-csi-driver-provider-azure/docs/) to test full functionality with provider installed

### Graduation Criteria

| OpenShift | Maturity |
| --------- | -------- |
| 4.14 | Tech Preview |
| 4.15(?) | GA |

#### Dev Preview -> Tech Preview

- Basic functionality for operator is implemented.
- e2e tests implemented with dummy provider.
- Manual testing with provider plugin passed.
- End user documentation created.

#### Tech Preview -> GA

- Tech preview operator was available in at lest one OCP release.
- e2e tests implemented with a provider plugin.
- Reliable CI signal, minimal test flakes.
- High severity bugs are fixed.

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

The operator will follow generic OLM upgrade path, and we do not support downgrade of the operator.

### Version Skew Strategy

It's managed by OLM and is allowed to run on any OCP cluster within the boundaries set by the operator metadata.

### Operational Aspects of API Extensions

N/A, the [new provider name](https://github.com/openshift/api/commit/5b630ed5a870d1f38d6bd9e289457e5c85535dc4)
simply allows the operator to create [csidriver.yaml](https://github.com/openshift/secrets-store-csi-driver-operator/blob/d271705b2e56b20581dcc04514c9b307b67fbeec/assets/csidriver.yaml#L4).

#### Failure Modes

Possible failure modes:
1. The operator could fail to create objects required by the CSI driver due to a bug in the operator code.
2. The CSI driver could report a failure, fail to clean up a mount path, or similar failure modes when attempting to attach/detach a volume to a pod.
3. The CSI driver pod could be unavailable for some reason, leaving kubelet to report a failure related to the mount request.
4. The provider plugin could be unavailable, causing the CSI driver to report a failure to reach the plugin over a Unix Domain Socket.
5. The secret store back-end could be unreachable by the provider plugin, which would cause a failure when attempting to attach the volume.

The OCP Storage team would be the point of contact for support escalations related to the CSI driver.

#### Support Procedures

1. Does kubelet report any issues when attempting to mount the volume?
2. Check that the CSI driver pods are running successfully on all nodes.
3. Review logs for the driver and operator pods. Are there mount failures? Issues applying any resources from the operator? Issues reaching the provider plugin from the driver?
4. Does the provider plugin exist, and is it running? Is the back-end secret store available? Review logs for the provider plugin.

## Implementation History

- 2023-06-09: Initial draft

## Alternatives

N/A

## Infrastructure Needed [optional]

Will need ART pipelines for the driver and operator (already discussed with ART team)

Github repos:
- <https://github.com/openshift/secrets-store-csi-driver>
- <https://github.com/openshift/secrets-store-csi-driver-operator>

## References

- [Secrets Store CSI Driver](https://github.com/kubernetes-sigs/secrets-store-csi-driver)
- [Upstream Documentation](https://secrets-store-csi-driver.sigs.k8s.io/)

