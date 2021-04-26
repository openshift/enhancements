---
title: share-secrets-and-configmaps-across-namespaces-via-a-csi-driver

authors:
  - "@gabemontero"

reviewers:
  - "@bparees"
  - "@adambkaplan"
  - "@derekwaynecarr"
  - "@gnufied"
  - "@jsafrane"
  - "@deads2k"

approvers:
  - "@bparees"
  - "@adambkaplan"
  - "@derekwaynecarr"
  - "@deads2k"

replaces:
  - "https://github.com/openshift/enhancements/pull/169"

creation-date: 2020-03-17

last-updated: 2021-04-19

<!-- status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced -->
status: implementable



---

# Share Secrets And ConfigMaps Across Namespaces via a CSI Driver


## Release Signoff Checklist

- [/] Enhancement is `implementable`
- [/] Design details are appropriately documented from clear requirements
- [/] Test plan is defined
- [/] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

> 1. If data on the CSI volume represents data the Pod's SA no longer has access to that data because of ACL/RBAC changes,
>from a storage/CSI volume perspective are there any best practices on how to deal with the data?  The CSI driver can
>unmount associated tmpfs for the SA in questions (assuming tmpfs per SA scales) so the data is no longer available from
>that location.  But since it could have copied the data elsewhere, do we need to kill the Pod?  If so, would the CSI
>driver even be allowed to initiate killing a Pod?  If so, what is required and what approach should be employed?

## Summary

This proposal will describe a [CSI driver](https://github.com/container-storage-interface/spec/blob/master/spec.md) based means for OpenShift cluster administrators to share `Secrets` / `ConfigMaps` defined in one namespace with other namespaces.

Any owners/authors/users of Pods in those namespaces can opt into the consumption of the content of those shared `Secrets` / `ConfigMaps` during their Pod's execution.

## Motivation

Simplify provision and consumption of `Secrets` and `ConfigMaps` defined in one namespace by pods in other namespaces.
There are numerous use cases where information should be defined only once on the cluster and shared broadly, such as:

- Distribution of custom PKI certificate authorities, such as corporate self-signed certificates.
- Simple Content Access certificates used to access RHEL subscription content.
  See also the [Subscription Content Access](/enhancements/subscription-content/subscription-content/access.md) proposal.

### Goals

- Provide easy to use sharing of `Secrets` and `ConfigMaps` defined in one namespace by pods in other namespaces.
- [reach] When a shared `Secret` or `ConfigMap` changes, the container filesystem content the CSI driver injects into the Pod needs to reflect those changes.
- When a cluster-admin revokes access to shared `Secrets` or `ConfigMaps` that was previously granted, that data should no longer be accessible.

### Non-Goals

- Validate the content within a shared `Secret` or `ConfigMap`.
- Prevent content in shared `Secrets` or `ConfigMaps` from being copied.
- Support for Windows nodes/workloads.

## Proposal

### User Stories

1. As a cluster admin with an OpenShift subscription, I want to share the entitlement keys and configuration with my
users' workload Pods so that containers in those Pods can install RHEL subscription content.

2. As a cluster admin with a set of CAs pertinent to various users's workloads, I want to share those CAs with my
users' workload Pods so that containers in those Pods can utilize those CAs when performing TLS based communication.

3. As a cluster admin whose user workloads include install kernel modules on nodes, I want to share the necessary information to
facilitate those scenarios across multiple namespaces.

4. As a cluster admin who has to rotate credentials shared via this proposal, and I remove access to new versions of
the credentials, I want to stop updating content within the existing pods consuming the content.

### Implementation Details/Notes/Constraints

#### Install

For the developer preview release, the CSI driver will be installable via YAML manifests available on GitHub.
This is meant to be a preview only option and allow the CSI driver to be installed on OpenShift outside of the normal release cycle.

When the CSI driver is ready for general availability, it will be installed on OpenShift via the cluster storage operator and an associated delivery operator - see the [CSI driver install proposal](/enhancements/storage/csi-driver-install.md).

#### CSI starting points

The projected resource CSI driver will provision ephemeral volumes and take advantage of upstream's ephemeral CSI storage mechanisms.
Ephemeral storage allows the CSI driver to create tmpfs filessytems for each pod, where the Secret/ConfigMap content will be copied.
The tmpfs filesystems will automatically have appropriate SELinux labeling to ensure content does not leak between pods - the driver does not need to provision or manage these labels.
The process will be similar to what exists in core Kubernetes when Secrets and ConfigMaps are mounted into a pod.
Acutal implementation can borrow a significant amount of logic from the [secrets-store-csi-driver](https://github.com/kubernetes-sigs/secrets-store-csi-driver), which is used to mount secrets from external "sealed" secret providers like [Vault](https://learn.hashicorp.com/tutorials/vault/kubernetes-secret-store-driver).

#### End to end flow (from creation, to opt-in, to validation, and then injections)

An admin creates a new cluster level custom resource for encapsulating the sharable `Secret` or `ConfigMap`.

```yaml
kind: Share
apiVersion: projected-resource.storage.openshift.io/v1
metadata:
  name: shared-cool-secret
spec:
  backingResource:
    kind: Secret
    apiVersion: v1
    namespace: ns-one
    name: cool-secret
  description: "used to access my internal git repo, RHEL entitlements, or something else cool"
status:
  conditions:
  ...
```

And sets up ACL/RBAC to control who can list/get such objects.

```yaml
rules:
- apiGroups:
  - projected-resource.storage.openshift.io
  resources:
  - shares
  resourceNames:
  - shared-cool-secret
  verbs:
  - get
  - list
  - watch

```

So, how does an end user discover and utilize our new CR instance?

First, a few considerations around discovery:

- one option for cluster admins is to allow "low level" users to get/list `Shares`; i.e. `oc get shares`;
 those users can then discover and use them without further involvement from the cluster admin; i.e. the RBAC example above
 is applied to those "low level" users
- if this is not acceptable, then some sort of exchange needs to occur between the user and cluster admin
- that exchange results in either cluster admin providing "just enough" information to the user to update their objects so
the underlying, associated Pod references CSI volumes using `Shares`
- or the cluster admin modifies the user's object(s) so they get their `Shares`

Second, a few considerations around how we update objects, so they "get" `Shares` associated with them:

- Pod objects themselves, or any API object that exposes the Pod's `volumes` array as part of providing an API that is converted by a controller into a Pod, can directly add a volume of the CSI type with the correct metadata in the volumeAttributes:

  ```yaml
  csi:
    driver: projected-resources.storage.openshift.io
      volumeAttributes:
        share: the-projected-resource
  ```

- with it as part of a Pod for example:

  ```yaml
  apiVersion: v1
  kind: Pod
  metadata:
    name: some-pod
  spec:
    containers:
      ...
      volumeMounts:
      - name: shared-item-name
        mountPath: /path/to/shared-item-data
    volumes:
    - name: entitlement
      csi:
        driver: projectedresources.storage.openshift.io
        volumeAttributes:
          share: the-projected-resource
  ```

The CSI Driver, aside from all the CSI and filesystem bits (see below for details), performs:

- reads of the `ProjectedResource` object referenced in the `volumeMetadata` which is available in the `NodePublishVolumeRequest` that comes into `NodePublishVolume`.
- the name, namespace, and `ServiceAccount` of the Pod wanting the volume is also included by the kubelet in the `NodePublishVolumeRequest` under the keys `csi.storage.k8s.io/pod.name`, `csi,storage.k8s.io/pod.namespace`, and `csi.storage.k8s.io/serviceAccount.name` in the map returned by the `GetVolumeContext()` call.
- the `podInfoOnMount` field of the CSI Driver spec triggers the setting of these values; here is
[an example from the hostpath plugin](https://github.com/kubernetes-csi/csi-driver-host-path/blob/master/deploy/kubernetes-1.17/hostpath/csi-hostpath-driverinfo.yaml#L12)

With all this necessary metadata, the CSI driver then:

- Performs a subject access review (SAR) check to verify the pod's service account has permission to GET the referenced Share resource.
- If the SAR check succeeds, reads of the `Secret` or `ConfigMap` noted in the `ProjectedResource`, storing it on disk on initial reference to said `Secret` or `ConfigMap`.
- Creates a tmpfs filesystem for each requesting `ServiceAcount` that contains the `Secret` / `ConfigMap` data.
- Creates k8s watches `ProjectedResources`, and for `Secret` / `ConfigMaps` in the namespaces listed in each discovered `ProjectedResources`.
- Returns a volume that is based off of the `ServiceAccount` specific tmpfs for the data noted in the `volumeAttributes`.

After initial set up of things, steady state CSI driver activities include:

- As updates of the watched `Secrets` and `ConfigMaps` come in, SAR checks will be done for each of the previous requesting `ServiceAccounts` being tracked against the updated `Secret` and `ConfigMap`.
  SAR checks are also done at regular relist intervals, which will pick up permission changes to associated cluster roles/role bindings.
- For any `ServiceAccounts` that no longer have access, contents in their tmpfs filesystems are removed. Unmounting any volume in a running pod is not feasible (can result in `EBUSY` errors).
- The SAR checks and if need be removal of access to data can facilitate cluster admin scenarios noted in the `User Stories` section like credential rotation.
- But for `ServiceAccounts` that pass the SAR check, the tmpfs file with the data for the `Secret` / `ConfigMap` is updated

#### An overview of CSI and the main K8S hook points

Research details and notes:

- a [CSI plugin](https://github.com/container-storage-interface/spec) is a gRPC endpoint that implements CSI services and allows storage vendors to develop a storage solution that works across a number of container orchestration systems.
- though for our purposes, we will not provide a generic CSI plugin, but rather a special case one only intended to work on OpenShift.
- also for our purposes, `plugin`, `driver`, and `endpoint` are the same thing based on which document you are reading.
- The CSI spec outlines lifecycle methods for create/provision/publish, destroy/unprovision/unpublish, resize, etc.
- In the ephemeral case, the Pod spec lists a volume and is considered to originate or "inline" it.
- The `csi` subfield still pertains, citing the storage provider's name, as with the PVC based approach.

  ```yaml
  apiVersion: v1
  kind: Pod
  metadata:
    name: some-pod
  spec:
    containers:
    ...
      volumeMounts:
      - name: shared-item-name
        mountPath: /path/to/shared-item-data
    volumes:
    - name: entitlement
      csi:
        driver: projectedresources.storage.openshift.io
        volumeAttributes:
          share: the-projected-resource
  ```

- And then the [CSI plugin to the Kubelet](https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/20190122-csi-inline-volumes.md#ephemeral-inline-volume-proposal), performs the necessary steps to engage the CSI driver.
- the [CSI spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) details the CSI architecture, concepts, and protocol.

#### Ephemeral CSI driver implementation

The CSI driver will be deployed as a DaemonSet so that shared Secrets/ConfigMaps can be added to pods running on the node.
Pods for this DaemonSet will need to run as privileged to access host paths on the node.
The CSI driver will only need to support "ephemeral" volumes that are specified by the `csi:` volume type on Pods.
There is no requirement for this CSI driver to provision persistent volumes.

Some details around inline ephemeral volumes:
- The employ a subset of the CSI specification flows, specifically only the `NodePublish` and `NodeUnpublish` CSI steps
- The underlying k8s api object `CSIVolumeSource` has the same `volumeAttributes` for storing metadata to be passed to the CSI driver
- There is only a single `LocalObjectReference` for a `Secret` reference in the same namespace as the consuming pod.
  So that aspect does not help us, and will not factor into the solution.
- The `volumeAttributes` field for metadata gives us enough capabilities for expressing both the name, namespace, and type of sharable API object to be consumed.
  Using pod info on mount lets the driver see which service account is requesting access.

The current driver implementation fuses the approaches of the [kubernetes-csi hostpath example](https://github.com/kubernetes-csi/csi-driver-host-path) with the [secrets store csi driver](https://github.com/kubernetes-sigs/secrets-store-csi-driver).
Unlike the secret store CSI driver, the projected resource CSI driver uses the pod's service account to determine if it has access to the requested share volume.
This is accomplished via a subject access review - the pod's service account must have GET permissions on the requested share.

#### Enhancements to Security Context Constraints

OpenShift uses Security Context Constraints (SCCs) to regulate the capabilities granted to Pods, based on the user creating the pod and the service account assigned to the pod.
Amongst other things, SCCs determine if a pod can mount a specific volume type.
SCCs deny mounting a volume unless its type is explicitly allowed.
The most restrictive SCC only allows volume types that are known to be isolated from the host filesystem (`secret`, `configMap`, `persistentVolumeClaim`, etc.).

SCCs as designed today have an "all or nothing" approach when it comes securing CSI volume mounts.
This is not sufficient as cluster admins are free to install third party CSI drivers that support `csi` volume mounts.
Because we want to deliver this CSI driver in the payload, we need ensure that this CSI driver is usable while ensuring third party CSI drivers can be restricted.

The following API extends SCCs to restrict the allowed CSI drivers used for ephemeral CSI volumes, in the same manner that Flexvolumes are restricted:

```go
type SecurityContextConstraints struct {
  // existing API
  ...
  //

  Volumes []FSType

  // AllowedCSIVolumes is an allowlist of permitted ephemeral CSI volumes.
  // Empty or nil indicates that all CSI drivers may be used.
  // This parameter is effective only when the usage of CSI volumes is allowed in the "Volumes" field.
	// +optional
  AllowedCSIVolumes []AllowedCSIVolume
}

// AllowedCSIVolume represents a single CSI volume driver that is allowed to be used.
type AllowedCSIVolume struct {
  // Driver is the name of the CSI driver that supports ephemeral CSI volumes.
  Driver string
}
```

With this API, the default SCCs delivered via OpenShift can be updated to include the projected resource CSI driver as an allowed driver.

#### Some details around prototype efforts to date

I was able to prototype both PVC and Ephemeral based samples with the HostPath example.
Per the above decision, the notes below here will focus on Ephemeral.

In looking at the [kubernetes-csi hostpath example](https://github.com/kubernetes-csi/csi-driver-host-path).
- it is very, very similar to what we want to do, but with this exception:  it employs a `StatefulSet` for deploying the CSI driver that only supports a [running on only one node](https://github.com/kubernetes-csi/csi-driver-host-path/blob/master/deploy/kubernetes-1.17/hostpath/csi-hostpath-plugin.yaml#L18-L28)
- that single node notion obviously won't work for us.
- But prototyping has shown we can move to a `DaemonSet` for the
  plugin and with the volume marked as read only, so we don't have to worry about consistency concerns from users trying
  to change the content on the Pod.
- among other things, Pod/CSI Plugin-Driver affinity held, and host/node filesystem content when seen
  from the Pod was consistent with the CSI Driver Pod on the same host/node as the Pod.
- And when we looked at the other CSI Driver Pods, of course their local filesystems did not have the data.
- So all this will serve the purposes of our scenario when you consider that a) the Volumes provided will be read only for the Pod so the user cannot try to maintain state on the filesystem which won't remain consistent if the Pod is moved, b) each CSI Driver pod will be the single writer to its local filesystem.
- The deploy yaml's at https://github.com/kubernetes-csi/csi-driver-host-path/tree/master/deploy/kubernetes-1.17/hostpath show the use of the `StatefulSet` in the `csi-hostpath-plugin.yaml` ... this was changed to `DaemonSet` in our prototype.
- For ephemeral volumes, the CSI hostpath driver currently manipulates the host/node file system at the `NodePublishVolume` method, which correspondes directly with the step of the same name of the CSI spec
- That method calls [createHostpathVolume](https://github.com/kubernetes-csi/csi-driver-host-path/blob/master/pkg/hostpath/hostpath.go#L208-L250) and this is where the current file manipulations occur (creating the read/write directory which is mounted into the Pod) - there is an analogous method `deleteHostpathVolume` for deleting the directory, that correlates to `NodeUnpublishVolume`.
- In `createHostpathVolume`, you'll see it supports multiple volume requests, which maps to different subdirs off of the same base directory.
- In our implementation, the secret contents could be in a well known location that we could sym link to in the directory created for each volume  ... i.e. /data/myVolID/secret -> /data/secret
- the example in the repo's README shows how you can write data to the volume by exec'ing into the application Pod, and then similarly see that same data if you exec into the CSI driver Hostpath container.
- But instead of this, in our case, we would build a co-located (in the same process as the CSI driver Hostpath container) an operator/controller that watches the secret(s) we want to mount
- the associated Listers for the Operator/Controller would be made available to the CSI plugin code
- And then in methods like `createHostpathVolume`, in addition to creating the directory, access the Secret(s) from the Lister and serialize the secret(s) bytes into files off of the directory the CSI plugin maintains
- However, exactly when we create directories for the mount and update the secrets files does not have to be at the same time.
- We will minimize the writes of the Secret to disk, so that is does not occur every time we provision a Pod
- Per [review of this proposal](https://github.com/openshift/enhancements/pull/250/files#r396278957), the CSI driver is not limited by when CSI calls between K8S and itself, occur.  So we don't have to populate the disk as part of any given call.  As long as the data is there by the time the final provisioning occurs, we are good.

The [secrets store csi plugin](https://github.com/kubernetes-sigs/secrets-store-csi-driver) as very similar to hostpath
in that
- the CSI spec implementation points were easy to find
- there were golang file system level calls during volume provisioning, i.e. `NodePublishVolume`, like `ioutil.ReadDir`
- and use of the k8s util mount API's
- it already employs `DaemonSets`
- the `SecretProviderClass` examples did not work out of the box on OpenShift, but we are not going down that path
anyway.


### Risks and Mitigations

**Risk:** Deploying via OLM and the cluster storage operator can lead to confusion

*Mitigations:*

- There will be no OLM operator at outset, as the intent is to distribute this driver via the OCP payload. 

**Risk:** CSI driver will not scale

*Mitigations:*

- Minimize local file system writes to when the Secret actually changes.
- Scale testing will ensure that a high volume of pods (ex 1000) can consume a single Share resource
- Scale testing will ensure that a high volume of shares can be consumed by small groups of pods - ex 100 shares used by 10 pods each.

## Design Details

### Test Plan

Verification of this proposal should be containable around e2e's that automate various forms of [the basic Hostpath
driver confirmation](https://github.com/kubernetes-csi/csi-driver-host-path/blob/master/docs/deploy-1.17-and-later.md#confirm-hostpath-driver-works), where that
confirmation is augmented to consider:

- the multiple replicas of the `DaemonSet`
- create/update/delete of the K8S Secret(s) designated for host/node injection are properly reflected on the volume
- RBAC revocation of access to `ProjectedResources` results in running pods not longer having access to the associated
data

Also, there is already some e2e's around the existing csi-hostpath example.  The ginkgo test focus  `[sig-storage] CSI Volumes [Driver: csi-hostpath]`
shows up in several of the base e2e suites.

Some links related to that testing:
- https://github.com/openshift/origin/blob/master/cmd/openshift-tests/csi.go
- https://github.com/openshift/origin/tree/master/test/extended/testdata/csi
- https://github.com/openshift/origin/tree/master/vendor/k8s.io/kubernetes/test/e2e/storage/external

### Graduation Criteria

#### Developer Preview

While the CSI driver is in developer preview state, cluster admins should be able to install via YAML manifests defined in GitHub.

#### GA

We will not want to GA this EP as is until the related work noted in the Summary is complete:

- Delivery via a CSI deivery operator (integrated with the cluster storage operator)
- Scale testing verifies the driver can handle a large number of pods consuming a single Share
- Security audit
- Share API reaches GA levels of supportability.

### Upgrade / Downgrade Strategy

Once included in the payload, the CSI driver will be installed and managed by the delivery operator.
The delivery operator will be responsible for rolling out the updated DaemonSet.

### Version Skew Strategy

N/A

## Implementation History

> 1. Current milestone: several iterations of this enhancement proposal have been pushed.

## Drawbacks

TODO

## Alternatives

Operator/Controller solutions for specific API Object types, like say OpenShift Builds, would not require a CSI driver/plugin solution.
But the call for a generic solution around host/node level injection/consumption is clear and definitive.

### Host Path Volumes

Shared content could be managed at the cluster level via files injected to nodes via MachineConfig objects.
Pods would then consume this content by mounting HostPath volumes.

- Declare `HostPath` based PV/PVCs on the Pod spec, on a per Pod basis, by each author of the Pod
- Leverage the 4.x machine configuration operator or MCO to update the host file system

There are several drawbacks with these approaches:

- `HostPath` volumes require permissions that we do not want to give to the users targeted for consumption.
Current understanding based on a conversation between @smarterclayton, @bparees, and @gabemontero:
  - Typically, to read anything on the `HostPath` volume, the privileged bit needs to be set
  - Some additional support, centered around `SecurityContextContraints` and SELinux, does exist as an add-on in OpenShift (vs. standard K8S), and it allows users to mount `HostPath` volumes prepared for them if they include necessary SELinux labelling.
  - So the cluster admin sets that SELinux label on the `HostPath` volume and namespace where it will be consumed.
    Admin would need to create a new `SecurityContextConstraint` that is based off of the `hostpath-anyuid` SCC (which is defined on install) and includes the SELinux label can be created.
  - However, the cluster administrator has to be careful with the mount provided.
    Mounts at certain parts of the filesystem can basically root the node.
- Host injection via the MCO requires a node reboot for the updates to take effect.
  That is generally deemedundesirable for this feature.
- The MCO API has not been intuitive for the type of users to date who are interested in introducing and then consuming small collections of files on a host.
  The RHCOS team has experienced challenges in coaching consumers who want to put entitlements on 4.x nodes via the MCO.

### Implement a provider for the Secret Store CSI Driver

Upstream sig-storage allows their Secret Store CSI driver to be extended via [secret store providers](https://secrets-store-csi-driver.sigs.k8s.io/providers.html).
To use a secret store provider, each namespace needs to have a [SecretProviderClass](https://secrets-store-csi-driver.sigs.k8s.io/getting-started/usage.html#create-your-own-secretproviderclass-object) which configures the secret store provider.
The secret store provider itself runs as a DaemonSet that communicates to the secret store CSI driver via gRPC.

The addition of the `SecretProviderClass` adds an extra layer of overhead to configure secret consumption.
Setting up a share would require the following steps:

- Create the cluster-wide `Share` object.
- Create a `SecretProviderClass` in each namespace that references the `Share`.
- Configure RBAC
  - We can keep the current approach and use SAR checks against the `Share` object.
  - We could also propose SAR checks against the `SecretProviderClass` object as a feature upstream that works for all providers.

That said, implementing a secret store provider does have its advantages:

- The API to implement the secret store provider is narrower in scope.
- Upstream support for the core CSI driver.
- Other secret store providers could also be installed and used on the cluster.
- Status of the pod mount is taken care of by the secret store CSI driver and the `SecretProviderClassPodStatus` object.

### Persistent CSI driver

On the CSI specifics, we are going with in-line ephemeral volumes, but for historical purposes some details around the
persistent volume styled approach with CSI:
- k8s provides Kubernetes CSI Sidecar Containers: a set of standard containers that aim to simplify the development and deployment of CSI Drivers on Kubernetes.
- There are CSI volume and persistent volume types in k8s.
- per [k8s volume docs](https://kubernetes.io/docs/concepts/storage/volumes/#csi) "the CSI types do not support direct reference from a Pod and may only be referenced in a Pod via a `PersistentVolumeClaim` object" (direct quote)
- However, @gnufied tells us that documentation is out of date and incorrect.  To quote him:  That is where [inline CSI volumes come](https://kubernetes-csi.github.io/docs/ephemeral-local-volumes.html).  @jsafrane also echoed this.
- To that end, one of the sidecar containers, the [external provisioner](https://kubernetes-csi.github.io/docs/external-provisioner.html), helps with this
- It watches Kubernetes `PersistentVolumeClaim` objects, and if the PVC% references a Kubernetes StorageClass, and the name in the provisioner field of the storage class matches the name returned by the specified CSI endpoint `GetPluginInfo` call, it calls the CSI endpoint `CreateVolume` method to provision the new volume.
- As part of Volume creation/provisioning a key element is this: CSI has two step mounting process. staging and publishing.
- They correspond to the endpoint's `NodeStageVolume` and `NodePublishVolume` implementations
- Staging is node local and publish is pod local. So we will probably want to stage the content once and publish it for each pod (which is nothing but creating a bind mount for original volume)
- That means we do not have to necessarily read and copy the secret(s) contents on every pod creation
- the [CSI spec volume lifecycle section](https://github.com/container-storage-interface/spec/blob/master/spec.md#volume-lifecycle) talks about a pre-provisioned volume.
- If this is the right choice, perhaps this will help with performance, and how often we copy the Secret(s) contents onto the host/node
- the underlying k8s api object `CSIPersistentVolumeSource` has a generic string map called `volumeAttributes` for storing metadata to be passed to the CSI driver
- and also a set of `SecretReferece` structs that can reference Secrets in namespaces outside of the namespace the consuming Pod is in.  See https://github.com/kubernetes/api/blob/master/core/v1/types.go#L1673-L1708 and https://github.com/kubernetes/api/blob/master/core/v1/types.go#L836-L845

And some details from our PV based prototyping with the hostpath examples (again, we are going with the ephemeral option):
- The CSI Hostpath example employs the side cars previously noted, including the external provisioner that watches for PVC's for its storage provider, and examples cited in the README show how the example Pod's PVC is handled by the storage driver.  See the example files at https://github.com/kubernetes-csi/csi-driver-host-path/tree/master/examples
- and our prototype showed that a Pod's PVC is honored by the CSI Plugin pod on the same node.
- Note the Pod definition, or the App for example, csi-app.yaml, **is not privileged**.
- which is what would have been required if HostPath volumes were used
- Note each of those sidecars are `StatefulSet`.  These were changed as well to  `DaemonSets` in our prototype.
- The CSI hostpath driver currently manipulates the host/node file system at two points:  1) the `CreateVolume` step of the CSI spec in the case of PVCs, 2) the `NodePublishVolume` step of the CSI spec (when dealing with ephemeral volumes)
- Both of those steps call [createHostpathVolume](https://github.com/kubernetes-csi/csi-driver-host-path/blob/master/pkg/hostpath/hostpath.go#L208-L250) and this is where the current file manipulations occur (creating the read/write directory which is mounted into the Pod)
- there is an analogous method `deleteHostpathVolume` for deleting the directory.


## Infrastructure Needed

- GitHub repositories for the CSI driver and operator
- Add the CSI driver and operator to the OpenShift release payload (ART)
