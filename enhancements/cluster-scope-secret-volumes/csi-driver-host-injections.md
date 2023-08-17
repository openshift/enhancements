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

last-updated: 2021-10-06

<!-- status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced -->
status: implemented



---

# Share Secrets And ConfigMaps Across Namespaces via a CSI Driver


## Release Signoff Checklist

- [/] Enhancement is `implementable`
- [/] Design details are appropriately documented from clear requirements
- [/] Test plan is defined
- [/] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

1. > There is one feature Gabe Montero really wanted working end to end, but has hit a roadblock.  The notion of making
> a volume provided by our new CSI driver here read-only, so the consumer cannot modify it, but still allow the driver to
> update the contents if it wants.  He got it working by defining multiple linux file systems layers on the mount when the
> volume is initially provisioned.  The driver updates the intermediate file system layer.  The consuming `Pod` sees the update
> from the top file system layer, but that is read-only and cannot modify things.  However, if the `DaemonSet` hosting the driver is restarted,
> he has been unable to persist the necessary information, or recreate it, such that it can still manage the volumes provisioned
> before the restart in the same way.  That intermediate layer is lost.  The driver can still delete the data (say if permissions
> are removed), but it can no longer update it.  Went as far as trying to update `/etc/fstab` on the host to preserve things?
> Is there something at the linux or kubelet levels that can help here.
2. > The assumption is that the "atomic reader/writer" support currently in [https://github.com/kubernetes/kubernetes/](https://github.com/kubernetes/kubernetes/)
> will be required for coordinating any updates to the `Secret` / `ConfigMap` data.  We have mitigated this for OpenShift
> Builds and consuming entitlements by a) providing a mode where the `SharedConfigMap` / `SharedSecret` mount is *NOT* updated (that is sufficient
> for the OpenShift Build consumption of enhancements scenario), but we are aware of additional scenarios that will want
> to leverage this new enhancement and cannot fall back on that simplification.  For example, the management of rotating certificates in longer running `Pods`.
> There are a couple of possibilities for exactly *how* we bring in the "atomic reader/writer" support.  Is the choice on
> *how* an implementation detail, or something that needs to be agreed upon here?


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
There are numerous use cases where information should be defined only once on the cluster and shared broadly, such as:

- Distribution of custom PKI certificate authorities, such as corporate self-signed certificates.
- Simple Content Access certificates used to access RHEL subscription content.

### Goals

- Provide easy to use sharing of `Secrets` and `ConfigMaps` defined in one namespace by pods in other namespaces.

- When a shared `Secret` or `ConfigMap` changes, the container filesystem content the CSI driver injects into the Pod
needs to reflect those changes, if the deployer of this feature chooses that level of function.  Conversely, if the deployer
of this enhancement does not want propagation of any data changes from the `Secret` or `ConfigMap` at the time of the creation of the `Pod` and provisioning
of the `Volume`, that level of support is also available.

- When an admin revokes access to shared `Secrets` or `ConfigMaps` that was previously granted, that data should
no longer be accessible at the mount location of the `Volume`.

- Allowing for the update of `Secret` and `ConfigMap` related data provisioned in a volume after a volume is provisioned
requires monitoring of those items, i.e. watches.  This then brings in the concern of *which* namespaces would an admin
want to allow sharing from, and thus which namespaces would the driver and its controller watch.  There are a lot of
`Secrets` and `ConfigMaps` in a cluster, a lot of `Namespaces`.  Scaling and memory concerns for the controller thus
exist.  As such, the driver and its operator supports both filtering of which namespaces will be monitored, as well
as a setting where we do not support update after initial provisioning, and hence the controller only watches the
cluster scoped `SharedResources`, and then uses an API client to fetch `Secrets` and `ConfigMaps` on demand.

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

4. As a cluster admin who has to rotate credentials shared via this enhancement, and I remove access to new versions of
the credentials, I want to stop updating content within the existing pods consuming the content.

5. As a cluster admin who manages the sharing of `ConfigMaps` and `Secrets` with this enhancement, I want to be able to distinguish
between which of my users can discover the existence these shared items, view the contents of these shared items, or actually use or mount these shared items
in their `Pods`.

6. As a cluster admin who manages the sharing of `ConfigMaps` and `Secrets` with this enhancement, I want to be able to
choose between me being able to control who can list/view/use these shared entities versus delegating to namespace level admins
who can list/view/use these shared items.

7. As a cluster admin who manages the share of `ConfigMaps` and `Secrets` with this enhancement, I want to be able to minimize
the CPU/Memory resource demands of this enhancement by being able to control whether it watches for updates of namespaced
`Secrets` and `ConfigMaps` at all, as well as control which namespaces are monitored when I am interested in updating
content after a volume is provisioned.

### Implementation Details/Notes/Constraints

#### Install

After much back and forth, this enhancement will be part of the core OCP install.  Short term, it will be installed as
Tech Preview level functionality via OCP feature sets/gates.  Once this enhancement reaches GA / fully supported status,
it will be laid down just like any of the other existing OCP components as part of OCP install.

Some specifics:
- Making it a day 1 install operation meets the ease of use targets we are trying to reach with the broader epics around
Red Hat entitlements.
- To paraphrase Clayton Coleman's comments during the decision-making process, this is the type of enhancement on top
of Kubernetes that core OCP is all about.

#### API, RBAC, Observability and Discoverability

An admin creates a new cluster level custom resource for encapsulating the sharable `Secret` or `ConfigMap`.


```yaml
kind: SharedConfigMap
apiVersion: sharedresource.openshift.io/v1alpha1
metadata:
  name: shared-cool-configmap
spec:
  configMapRef:
    namespace: ns-one
    name: cool-configmap
status:
  conditions:
  ...
```

```yaml
kind: SharedSecret
apiVersion: sharedresource.openshift.io/v1alpha1
metadata:
  name: shared-cool-secret
spec:
  secretRef:
    namespace: ns-one
    name: cool-secret
status:
  conditions:
  ...
```

And sets up ACL/RBAC to control which `ServiceAccounts` can use such objects to mount their referenced resources in their `Pods`:

```yaml
rules:
- apiGroups:
  - sharedresource.openshift.io
  resources:
  - sharedsecrets
  resourceNames:
  - shared-cool-secret
  verbs:
  - use
```

```yaml
rules:
- apiGroups:
  - sharedresource.openshift.io
  resources:
  - sharedconfigmaps
  resourceNames:
  - shared-cool-configmap
  verbs:
  - use
```

And for controlling which human users can list or inspect the `SharedConfigMap` and `SharedSecret` objects available on a cluster:

```yaml
rules:
- apiGroups:
  - sharedresource.openshift.io
  resources:
  - sharedsecrets
  - sharedconfigmaps
  verbs:
  - list
  - get
  - watch
```

of course, additional filtering based on specific `resourceNames` can also make sense in defining what human users
can and cannot inspect on the cluster with respect to `SharedConfigMap` and `SharedSecret`.

Either namespaced or clustered RBAC is supported for either human users and `ServiceAccounts` to view/list/use
`SharedConfigMaps` / `SharedSecrets`.  

As `SharedConfigMaps` / `SharedSecrets` are cluster scoped objects, most likely cluster admins, or OCP operators, will create the
`SharedConfigMaps` / `SharedSecrets`.  Cluster admins may create RBAC for anyone of course, but the expected standard operating procedure
is that they will grant namespace administrators:

- the ability to list and get/view `SharedConfigMaps` / `SharedSecrets`
- the ability to create namespace scoped RBAC so that `ServiceAccounts` in their namespaces can utilize `SharedConfigMaps` / `SharedSecrets`
in `Pods` that use said `ServiceAccounts`.
- the ability to allow users in their namespaces the ability to list and possibly get/view `SharedConfigMaps` / `SharedSecrets`

With that, "observability" and "discoverability" requirments for our new API here are sufficiently met.

#### How Pods and the Objects that include Pods or subcomponents of Pods declare intent to use SharedResources

Pod objects themselves, or any API object that exposes the Pod's `volumes` array as part of providing an API that is
converted by a controller into a Pod, can directly add a volume of the CSI type with the correct metadata in the volumeAttributes, a la

```yaml
csi:
  driver: csi.sharedresource.openshift.io
    volumeAttributes:
      sharedConfigMap: the-shared-configmap

```
```yaml
csi:
  driver: csi.sharedresource.openshift.io
    volumeAttributes:
      sharedSecret: the-shared-secret

```


- with it as part of a Pod for example:

 ```yaml
apiVersion: v1
kind: Pod
metadata:
  name: some-pod
  namespace: some-ns
spec:
  containers:
    ...
     volumeMounts:
        - name: shared-item-name1
          mountPath: /path/to/shared-item-data-2
        - name: shared-item-name2
          mountPath: /path/to/shared-item-data-2
  volumes:
      - name: shared-secret
        csi:
          driver: csi.sharedresource.openshift.io
          volumeAttributes:
             sharedSecret: the-shared-secret
      - name: shared-configmap
        csi:
          driver: csi-sharedresource.openshift.io
          volumeAttributes:
             sharedConfigMap: the-shared-configmap

```

At this time we are not supporting API objects which do not expose the resulting Pod's volume array, or do not have
a corresponding controller/operator translates their API into a Pod's volume array.

#### CSI starting points and implementation details

On startup:
- Creates k8s watches for `SharedConfigMaps` / `SharedSecrets`
- Optionally for `Secret` / `ConfigMaps` in the namespaces listed in the drivers configuration if we support update after the provisioning of the volume


Once started, the CSI Driver, when contacted by the kubelet because a Pod references the CSI driver it is registered under, performs the following:
- reads the `SharedConfigMap` / `SharedSecret` object referenced in the `volumeMetadata` which is available in the `NodePublishVolumeRequest` that comes into `NodePublishVolume`.
- the name, namespace, and `ServiceAccount` of the Pod wanting the volume is also included by the kubelet in the `NodePublishVolumeRequest` under the keys `csi.storage.k8s.io/pod.name`, `csi,storage.k8s.io/pod.namespace`, and `csi.storage.k8s.io/serviceAccount.name` in the map returned by the `GetVolumeContext()` call.
- the `podInfoOnMount` field of the CSI Driver spec triggers the setting of these values

With all this necessary metadata, the CSI driver then:

- Performs a subject access review (SAR) check to verify the pod's service account has permission to USE the referenced SharedResource.
- If the SAR check succeeds, reads of the `Secret` or `ConfigMap` noted in the `ShareResource`
- Creates tmpfs filesystem(s) for each requesting `ServiceAcount` that contains the `Secret` / `ConfigMap` data.
- Returns a volume that is based off of the `ServiceAccount` specific tmpfs for the data noted in the `volumeAttributes`.

After initial set up of things, steady state CSI driver activities include:

- Regardless of the post provision setting, SAR checks are also done at the regular relist intervals, which will pick up permission changes to associated cluster roles/role bindings.
- For any `ServiceAccounts` that no longer have access, contents in their tmpfs filesystems are removed. Unmounting any volume in a running pod is not feasible (can result in `EBUSY` errors).
- The SAR checks and if need be removal of access to data can facilitate cluster admin scenarios noted in the `User Stories` section like credential rotation.
- But for `ServiceAccounts` that pass the SAR check, the data for the `Secret` / `ConfigMap` remains
- If post provision updates are turned on, as updates of the watched `Secrets` and `ConfigMaps` come in, SAR checks will be done for each of the previous requesting `ServiceAccounts` being tracked against the updated `Secret` and `ConfigMap`.
- If the SAR checks pass, the data on the file system is updated.  See the open question about k8s atomic read/write.

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
- The `volumeAttributes` field for metadata gives us enough capabilities for expressing both the name, and type of sharable API object to be consumed.
  Using pod info on mount lets the driver see which service account is requesting access.

The current driver implementation fuses the approaches of the [kubernetes-csi hostpath example](https://github.com/kubernetes-csi/csi-driver-host-path) with the [secrets store csi driver](https://github.com/kubernetes-sigs/secrets-store-csi-driver).
Unlike the secret store CSI driver, the projected resource CSI driver uses the pod's service account to determine if it has access to the requested share volume.
This is accomplished via a subject access review - the pod's service account must have GET permissions on the requested share.


### Risks and Mitigations

**Risk:** Deploying via OLM and the cluster storage operator can lead to confusion

*Mitigations:*

- There will be no OLM operator, as the distributioin of this driver is via the OCP payload.

**Risk:** CSI driver will not scale

*Mitigations:*

- Minimize local file system writes to when the Secret actually changes.
- Scale testing will ensure that a high volume of pods (ex 1000) can consume a single Share resource
- Scale testing will ensure that a high volume of shares can be consumed by small groups of pods - ex 100 shares used by 10 pods each.
- Allow for disablement of watches on `Secrets` and `ConfigMaps`
- Allow for control over which namespaces are watched for updates on `ConfigMaps` and `Secrets`.

## Design Details

### Test Plan

- A full suite of e2e's already exist for the driver and its associated controller in [https://github.com/openshift/csi-driver-shared-resource](https://github.com/openshift/csi-driver-shared-resource)
- Integration into the CSO test suites around install is on path.
- Red Hat Entitlements for Builds will be the sole scenario vetted and claimed for support with the initial Tech Preview offering.
- Subsequent iterations of Tech Preview will take on updates for certificate rotation, where either via e2e's or QE validation, scaling evaluations around the number of namespaces that can be watched will be required.

### Graduation Criteria

#### Current state

Dev Preview is the current state of the offering.

While the CSI driver is in developer preview state, cluster admins can install via YAML manifests defined in GitHub.

#### Dev Preview -> Tech Preview

- Delivery via a CSI delivery operator (integrated with the cluster storage operator) and the Tech Preview feature gate.

#### Tech Preview -> GA

- Admission controls for CSI volumes in pods (ex via via Security Context Constraints)
- Scale testing verifies the driver can handle a large number of pods consuming a single Share
- Security audit
- SharedResource API reaches GA levels of supportability (v1).

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

Once included in the payload, the CSI driver will be installed and managed by the delivery operator.
The delivery operator will be responsible for rolling out the updated DaemonSet.

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

The secret store CSI driver by default only asks for data to be mounted once per pod restart.
There is a feature to [auto rotate secrets](https://secrets-store-csi-driver.sigs.k8s.io/topics/secret-auto-rotation.html), which is currently alpha.

That said, implementing a secret store provider does have its advantages:

- The API to implement the secret store provider is narrower in scope.
- Upstream support for the core CSI driver - this includes scale and [load testing](https://secrets-store-csi-driver.sigs.k8s.io/load-tests.html)
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

## Appendix

### An overview of CSI and the main K8S hook points

Research details and notes:

- a [CSI plugin](https://github.com/container-storage-interface/spec) is a gRPC endpoint that implements CSI services and allows storage vendors to develop a storage solution that works across a number of container orchestration systems.
- though for our purposes, we will not provide a generic CSI plugin, but rather a special case one only intended to work on OpenShift.
- also for our purposes, `plugin`, `driver`, and `endpoint` are the same thing based on which document you are reading.
- The CSI spec outlines lifecycle methods for create/provision/publish, destroy/unprovision/unpublish, resize, etc.
- In the ephemeral case, the Pod spec lists a volume and is considered to originate or "inline" it.
- The `csi` subfield still pertains, citing the storage provider's name, as with the PVC based approach.
- And then the [CSI plugin to the Kubelet](https://github.com/kubernetes/enhancements/blob/master/keps/sig-storage/20190122-csi-inline-volumes.md#ephemeral-inline-volume-proposal), performs the necessary steps to engage the CSI driver.
- the [CSI spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) details the CSI architecture, concepts, and protocol.

#### Initial Prototype Notes

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