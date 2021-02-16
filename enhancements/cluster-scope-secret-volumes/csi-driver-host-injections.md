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

last-updated: 2020-09-01

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
> 2. CVO vs. OLM for delivery.  May need @derekwaynecarr to cast the deciding vote.  Latest pro-con list below in the
>implementation details install section.  Top level question to answer is do we want this feature installed on
>all OCP clusters, or have it be an optional component.  If optional, OLM is the defacto answer.  If it always needs to
>be there, does that mandate CVO, or as some reviewers have speculated, *perhaps not in the near future*.

## Summary

This proposal will describe a [CSI driver](https://github.com/container-storage-interface/spec/blob/master/spec.md) based
means for OpenShift cluster administrators to share `Secrets` / `ConfigMaps` defined in one namespace with
other namespaces.

Any owners/authors/users of Pods in those namespaces can opt into the consumption of the content of those shared `Secrets` /
 `ConfigMaps` during their Pod's execution.

This proposal was broken off from [a proposal on injecting RHEL entitlements/subscriptions](https://github.com/openshift/enhancements/pull/214)
**EDITORIAL NOTE:** if/when PR 214 merges the reference link will be adjusted to point to the merged document hosted on
https://github.com/openshift/enhancements.  Or if that PR is replaced by another proposal, or this proposal, this proposal will
point that out.


## Motivation

Simplify provision and consumption of `Secrets` and `ConfigMaps` defined in one namespace by pods in other namespaces.

### Goals

- Provide easy to use sharing of `Secrets` and `ConfigMaps` defined in one namespace by pods in other namespaces.

- When a shared `Secret` or `ConfigMap` changes, the container filesystem content the CSI driver injects into the Pod
needs to reflect those changes.

- When a cluster-admin revokes access to shared `Secrets` or `ConfigMaps` that was previously granted, that data should
no longer be accessible.

### Non-Goals

1. This proposal assumes the sensitive content encapsulated in the `Secret` or `ConfigMap` is valid and correct for the user's purposes.
No validation of the content for specific usage within a Pod will be done.  That said, this proposal provides
for watching for updates to those objects.
So if the cluster administrator realized on their own they have provided bad data, and then corrects it, this proposal
will watch for, discover, and propagate the update when new Pods consuming the content are provisioned.

2. Protecting this content from being viewed/copied by users: if it's available for the Pod to use, the user who can
create a Pod that can utilize this feature can also read / view / exfiltrate the Secret/ConfigMap contents. It is technically
impossible to avoid that.

3. This proposal for now assumes linux only for the underlying host/node.  No windows, etc.

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

As per the open questions, whether to go with a CVO or OLM based approach, or perhaps some combination, is a question
with no current consensus on the answer.

Some specifics:
- During the review for the ["Installation of CSI drivers that replace in-tree drivers](https://github.com/openshift/enhancements/blob/master/enhancements/storage/csi-driver-install.md)
a question posed about whether the exact type of CSI driver this proposal is describing would use the method
described in that document (namely OLM).  The recommendation from the [storage SMEs was to go with CVO](https://github.com/openshift/enhancements/pull/139/files#r391761362)
was to not use OLM, and use CVO instead.
- Using CVO and thus making it a day 1 install operation meets the ease of use targets we are trying to
reach with this proposal
- Crafting OLM dependencies between this proposal and [related proposals](https://github.com/openshift/enhancements/pull/214)
as part of an OLM based install at the time of this writing is immature, at least for some reviewers.
- However, other reviewers wonder if we are in fact close from having either OLM or CVO operators being able to require
an OLM operator to be present.
- Using OLM avoids further increasing the install payload size.
- Using OLM avoids unexpected hiccups in the feature's installation impacting initial cluster install or cluster upgrade.

#### CSI starting points

**WE ARE GOING WITH EPHEMERAL MODE**

**NO SecretProvideClass IS NEEDED**

With those simplifying decisions, a CSI driver with those characteristics still meets our needs.  And the storage
SMEs recommend that approach to us.

Crafting the CSI driver in this way also helps us deal with SELinux.  Specifically (thanks to @jsafrane for details):

- when CRI-O starts any container, it updates the SELinux labels on all volumes for that container, where those labels are unique to the container
- in the context of k8s, multiple containers within a pod have the same filesystem access
- so, only that specific pod can read files on that volume
- however, a file can only have one SELinux label
- so two or more pods cannot access the same file
- but if each pod has its own file, its own copy, of the shared data, things work easily from a SELinux perspective
- this is where using ephemeral CSI storage comes in
- using ephemeral based storage allows for use of a tmpfs (virtual memory filesystem) for storage
- tmpfs is fast and has negligible overhead
- it will have to reconstructed if the Pod moves
- but how it eases dealing with SELinux protection is the bonus we are taking
- so each pod gets its own tmpfs
- the kubelet leverages tmpfs in a similar fashion already

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

So, how does an end user discover and utilize our new CR?

First, a few considerations around discovery:

- one option for cluster admins is to allow "low level" users to get/list `Shares`; i.e. `oc get shares`;
 those users can then discover and use them without further involvement from the cluster admin; i.e. the RBAC example above
 is applied to those "low level" users
- if this is not acceptable, then some sort of exchange needs to occur between the user and cluster admin
- that exchange results in either cluster admin providing "just enough" information to the user to update their objects so
the underlying, associated Pod references CSI volumes using `Shares`
- or the cluster admin modifies the user's object(s) so they get their `Shares`

Second, a few considerations around how we update objects, so they "get" `Shares` associated with them:

- Pod objects themselves, or any API object that exposes the Pod's `volumes` array as part of providing an API that is
converted by a controller into a Pod, can directly add a volume of the CSI type with the correct metadata in the volumeAttributes, a la

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

- API objects which do not expose the resulting Pod's volume array will need to have an annotation set on it which triggers
an admission plugin to mutate the Pod and add the CSI volume.

On that new admission plugin introduced and its validation/mutation of Pods:
- Pods with this annotation are mutated, adding the CSI driver volume specification,
- if the Pod `ServiceAccount` is allowed sufficient
access to the requested `ProjectedResource` (i.e. a SAR check for the SA getting the requested `ProjectedResource` passes)
- if a Pod already happens to have the CSI Volume specified with reference to a `ProjectedResource`, again the plugin will see if the
associated `ServiceAccount` for the Pod has sufficient access to the `ProjectedResource`
- if sufficient access does not exist, the admission plugin marks the Pod as invalid/forbidden.
- if sufficient access does exists, the admission plugin adds a CSI volume into the Pod's `volume` array, to match the
example noted above.


The CSI Driver, aside from all the CSI and filesystem bits (see below for details), performs:
- reads of the `ProjectedResource` object referenced in the `volumeMetadata` which is available in the `NodePublishVolumeRequest`
that comes into `NodePublishVolume`
- the name, namespace, and `ServiceAccount` of the Pod wanting the volume is also included by the kubelet in the
`NodePublishVolumeRequest` under the keys `csi.storage.k8s.io/pod.name`, `csi.storage.k8s.io/pod.namespace`,  and
`csi.storage.k8s.io/serviceAccount.name` in the map returned by the `GetVolumeContext()` call
- the `podInfoOnMount` field of the CSI Driver spec triggers the setting of these values; here is
[an example from the hostpath plugin](https://github.com/kubernetes-csi/csi-driver-host-path/blob/master/deploy/kubernetes-1.17/hostpath/csi-hostpath-driverinfo.yaml#L12)
- with all this necessary metadata, the CSI driver then
- reads of the `Secret` or `ConfigMap` noted in the `ProjectedResource`, storing it on disk on initial
reference to said `Secret` or `ConfigMap`
- creation of a tmpfs filesystem for each requesting `ServiceAcount` that contains the `Secret` / `ConfigMap` data
- creation of k8s watches `ProjectedResources`, and for `Secret` / `ConfigMaps` in the namespaces listed in each discovered
`ProjectedResources`
- final returning of a volume that is based off of the `ServiceAccount` specific tmpfs for the data noted in the
`volumeAttributes`

After initial set up of things, steady state CSI driver activities include:
- as updates of the watched `Secrets` and `ConfigMaps` come in, SAR checks will be done for each of the previous requesting
`ServiceAccounts` being tracked against the updated `Secret` and `ConfigMap`
- for any `ServiceAccounts` that no longer have access, their tmpfs filesystems are unmounted, effectively removing their access to
the data (though in prototyping and implementation, we'll need focused testing that this scales and mount propagation
does not become a gating factor)
- the SAR checks and if need be removal of access to data can facilitate cluster admin scenarios noted in the `User Stories`
section like credential rotation.
- But for `ServiceAccounts` that pass the SAR check, the tmpfs file with the data for the `Secret` / `ConfigMap` is updated


#### An overview of CSI and the main K8S hook points

Research details and notes:

- a [CSI plugin](https://github.com/container-storage-interface/spec) is a gRPC endpoint that implements CSI services and allows storage vendors to develop a storage solution that works across a number of container orchestration systems.
- though for our purposes, we will not provide a generic CSI plugin, but rather a special case one only intended to work on OpenShift
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
- the [CSI spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) details the CSI architecture, concepts, and protocol

#### Current thoughts on implementation, how to inject/consume content on every host/node in an OpenShift cluster

Implementation time approach:
- Two example CSI plugins to use as a starting point are the [secrets
 store csi
 plugin](https://github.com/kubernetes-sigs/secrets-store-csi-driver),
 which includes a `SecretProviderClass` CSI plug point and a simpler
 [single node host path
 plugin](https://github.com/kubernetes-csi/csi-driver-host-path/),
 though for our read only purposes, prototyping has shown we can make
 it multi-node.
- Both provide PV and ephemeral options.  We only need the ephemeral option.
- The host path is simpler, but generally considered "POC" level
- The secret store employs a `SecretProviderClass`, which we do not need.
- So we will create a new plugin repo where we pick elements from either of these samples as it suits our purposes.
- And delete all the PV related stuff we do not need.
- Our new CSI plugin would be hosted as a `DaemonSet` on the k8s cluster, to get per node/host granularity
- and that `DaemonSet` will have privileged containers.  See the
  [csi-node-driver-registrar](https://github.com/kubernetes-csi/csi-driver-host-path/blob/master/deploy/kubernetes-1.17/hostpath/csi-hostpath-plugin.yaml#L48-L52)
  container and the main [csi plugin
  container](https://github.com/kubernetes-csi/csi-driver-host-path/blob/master/deploy/kubernetes-1.17/hostpath/csi-hostpath-plugin.yaml#L82-L83)
  for the hostpath example and [secrets store
  container](https://github.com/kubernetes-sigs/secrets-store-csi-driver/blob/master/deploy/secrets-store-csi-driver.yaml#L61)
  in the secrets store example.
- When provisioning volumes, the plugin can access the node/host
  filesystem, like for example the directory, corresponding to the
  Pod's mount point, where the secret contents will be stored (though
  we have some choices on exactly which OS level file system features
  we employ to store the content ... see the Open Questions)
- All the reference implementations reviewed to date leverage https://github.com/kubernetes/utils/blob/master/mount/mount.go

Some details around inline ephemeral volumes:
- The employ a subset of the CSI specification flows, specifically only the `NodePublish` and `NodeUnpublish` CSI steps
- the underlying k8s api object `CSIVolumeSource` has the same `volumeAttributes` for storing metadata to be passed to the CSI driver
- there is only a single `LocalObjectReference` for a `Secret` reference in the same namespace as the consuming pod.  So that aspect does not help us, and will not factor into the solution.
- The `volumeAttributes` field for metadata gives us enough capabilities for expressing both the name, namespace, and type of sharable API object to be consumed.  And who is requesting access.

#### Some details around prototype efforts to date

I was able to prototype both PVC and Ephemeral based samples with the HostPath example.  Per the above decision, the
notes below here will focus on Ephemeral.

In looking at the [kubernetes-csi hostpath example](https://github.com/kubernetes-csi/csi-driver-host-path)
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

While this proposal settles on installing via CVO, that decision has ramifications whose discussion are noted above.
It is at least conceivable the need to revisit that decision may arise when implementation starts.

Getting the flow and various stages from the CSI specification understood and correctly utilized so that we minimize
the amount of times the Secret is read and stored on the local filesystem of each host/node is important.  We want
to minimize local file system writes to when the Secret actually changes, and not when we provision the volume to
a new Pod.


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

We will not want to GA this EP as is until the related work noted in the Summary is complete.


### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History

> 1. Current milestone: several iterations of this enhancement proposal have been pushed.

## Drawbacks

N/A

## Alternatives

Operator/Controller solutions for specific API Object types, like say OpenShift Builds, would not require a CSI driver/
plugin solution.  But the call for a generic solution around host/node level injection/consumption is clear and
definitive.

Otherwise, at this time, the only means of achieving similar results to what is articulated by this proposal are:

- Declare `HostPath` based PV/PVCs on the Pod spec, on a per Pod basis, by each author of the Pod
- Leverage the 4.x machine configuration operator or MCO to update the host file system

There are several drawbacks with these approaches:

- `HostPath` volumes require permissions that we do not want to give to the users targeted for consumption.
Current understanding based on a conversation between @smarterclayton, @bparees, and @gabemontero:
>> - Typically, to read anything on the `HostPath` volume, the privileged bit needs to be set
>> - Some additional support, centered around `SecurityContextContraints` and SELinux, does exist as an add-on in OpenShift
>> (vs. standard K8S), and it allows users to mount `HostPath` volumes prepared for them, if they include necessary SELinux labelling.
>> - So the cluster admin sets that SELinux label on the `HostPath` volume and namespace where it will be consumed.
>> - a new `SecurityContextConstraint` that is based off of the `hostpath-anyuid` SCC (which is defined on install) and
>> includes the SELinux label can be created.
>> - then the serviceaccount for the user Pod in question would be given access to the new SCC
>> - However, the cluster administrator has to be careful with the mount provided.
>> - Mounts at certain parts of the filesystem can basically root the node
- Host injection via the MCO requires a node reboot for the updates to take effect.  That is generally deemed
undesirable for this feature.
- The MCO API has not been intuitive for the type of users to date who are interested in introducing and then consuming
small collections of files on a host.  The RHCOS team has experienced challenges in coaching consumers who want to put
entitlements on 4.x nodes via the MCO.

### CSI specific alternatives

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

N/A
