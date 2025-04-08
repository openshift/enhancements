---
title: user-namespaces-support
authors:
  - haircommander
reviewers:
  - rphilips
  - giuseppe
  - ibihim
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - mrunalp
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - deads2k
  - JoelSpeed
creation-date: 2024-06-17
last-updated: 2024-07-31
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPNODE-2506
see-also:
  - N/A
replaces:
  - N/A
superseded-by:
  - N/A
---

# User Namespaces Support

## Summary

In Kubernetes 1.30, support for User Namespaces went to Beta. Adding support in Openshift will allow users to gain access to additional users
in a container in a safe way, as well as open up avenues for running podman inside of an unprivileged Openshift pod.
To integrate into Openshift, we must enable the feature UserNamespacesSupport (will already be on), UserNamespacesPodSecurityStandards and ProcMountType
and integrate support into SecurityContextContraints (SCC).

This feature relies on work already done in the kernel to support idmapped mounts, or a mechanism to allow filesystems to be user namespaces aware.
This work was merged in RHEL 9.4.

## Motivation

Originally implemented in the linux kernel 3.8, user namespaces have long been a goal of the Kubernetes community.
[KEP-127](www.github.com/kubernetes/enhancements/issues/127) is one of the oldest still-open KEPs today.
Part of the push for user namespaces is it gets containers closer to the aspirational goal of a virtualized host: putting a process
in a user namespace means it can have "privileges" inside the container, while being unprivileged on the host. Further, this
divide between the container's namespace and the host's means an admin can allow users to gain access to privileges within the container,
while being able to trust that the kernel doesn't grant them on the host. A consequence of this is users can, for instance, run podman
within an Openshift pod without being in a privileged namespace.

### User Stories

* As an Openshift user, I would like to be able to run my container as root without needing to be trusted by the platform.
* As a user of Openshift Devspaces, I would like to run podman within the Devspace.
* As an Openshift admin, I would like to run untrusted users on a tighter security profile than the SCC restricted-v2.
* As an Openshift admin, I would like to ensure pods that request user namespaces are confined to one.

### Goals

- Enable Openshift users to request a pod be put in a user namespace
- Update SCC to take user namespaces into account when choosing the security profile of a container
- Add support for users to run podman within an Openshift pod without being privileged.

### Non-Goals

- Anything not related to user namespaces or running nested containers.

## Proposal

There are three pieces to this proposal:
- Extend SCC to be aware of the `hostUsers` field:
    - Add a new field `UserNamespaceLevel` to SCC, which will be `AllowHostLevel` by default
    - Add a new SCC to the default list: `restricted-v3`
        - This SCC will be identical to `restricted-v2`, but have `UserNamespaceLevel` set to `RequirePodLevel`
    - Add a new SCC to the default list: nested-container. It will have:
        - SELinux context set to `MustRunAs.Type: container_engine_t`
        - `RunAsUserStrategy: RunAsAny`
        - `UserNamespaceLevel`: `RequirePodLevel`
        - And otherwise mirror the `restricted-v2` profile
- Add the features `UserNamespacesSupport`, `UserNamespacesPodSecurityStandards` and `ProcMountType` to the list that qualify a cluster as `TechPreviewNoUpgrade`
    - While `UserNamespacesSupport` is beta in upstream Kubernetes 1.30, it is off by default.
- For GA of this feature, add a feature in the openshift-apiserver that denies kubelets from getting certs if they are too old.

### Workflow Description

- (TechPreview) A cluster admin turns on the `UserNamespacesSupport`, `UserNamespacesPodSecurityStandards` and `ProcMountType` features with the `featuregate` CRD.
- The cluster reconciles, enabling these features in the kubelet and apiservers.
- Pod authors can update their pods and set the `hostUsers` field to `false`.
    - Depending on the SCC the namespace is allowed to use, the pod author may be required to do so.
- Pod authors can also set the `procMount` field to `Unmasked` (if they also set `hostUsers` to `false`).
- Pod authors could then run nested containers, or other operations that can benefit from a user namespace and unmasked `/proc`

### API Extensions

- Add `UserNamespaceLevel` to SCC. This relies on approval from the auth team.

### Topology Considerations

#### Hypershift / Hosted Control Planes

N/A

#### Standalone Clusters

N/A

#### Single-node Deployments or MicroShift

N/A

### Implementation Details/Notes/Constraints

#### SecurityContextConstraint Updates

##### Brief Aside: Impact of User Namespaces on Pod Fields

User namespaces are very effective at increasing the overall security profile of a process while doing a good job of limiting the impact that process can affect on the host.
For pod fields specifically, there are a couple of classes of interactions that change when a user namespace is present:

###### Impact on UID

The most obvious impact is the UID inside of a user namespace is different than on the host. This means a process can request a UID that is privileged inside of the user namespace
(like UID 0), and outside the user namespace that UID will not be privileged.

For specific pod fields, the `runAsGroup`, `runAsUser`, `supplementalGroups`, and `fsGroup` fields all can be considered safe to use, as each of these fields will be unprivileged on the host,
no matter the value inside the pod's user namespace.

This would potentially impact how the corresponding SCC fields `runAsUser`, `supplementalGroups` and `fsGroup` are handled. However, to lower the impact of adding the `UserNamespaceLevel` field,
the presence of `UserNamespaceLevel` won't change the handling of these fields, but rather we will create a different SCC profile that allows `RunAsAny` for these fields if `UserNamespacesLevel` is
`RequirePodLevel`.

###### Impact on Capabilities

A consequence of the UID impact extends to capabilities. In Linux, capabilities are namespaced. Thus, a user with a capability inside of a user namespace doesn't necessarily have those capabilities
outside of the user namespace. Theoretically, this should mean capabilities are unconditionally safe for a pod to use, if that pod is in a user namespace. Practically, however, it is not so clear.
While the kernel *should* prevent exploits where a process gets access to capabilities on the host when it has them in a user namespace, it has historically not been perfect in doing so.
Further, for a very security minded admin, giving access to capabilities inside of a user namespace also increases the kernel attack surface a process can potentially exploit.

There is an argument to be made that any pod with access to the `clone`, `clone3` or `unshare` syscalls can get access to the full set of capabilities, inside of a user namespace (as creating a new
user namespace grants you the ability to gain capabilities inside of the pod). Such an argument was the basis of a [proposal upstream](https://github.com/kubernetes/kubernetes/pull/125198) to relax
validation for pods with a user namespace to allow all capabilities. However, this proposal was denied on the basis that some clusters have seccomp profiles set by default, which means a pod won't get
access to these syscalls, and thus wouldn't have access to the capabilities (but would suddenly if the pod was in a user namespace).

Additionally, calling `unshare` in a pod to get capabilities is not equivalent to applying the capabilities in the pod spec because explicitly giving a pod `CAP_SYS_ADMIN` also implicitly enables
`allowPrivilegeEscalation`. Thus, if we relaxed validation for capabilities for pods in a user namespace, then these pods would implicitly get `allowPrivilegeEscalation` set to false. This may not be
risky because the pod is in a user namespace, but this interaction should be called out.

Thus, it could be argued that capabilities inside a user namespace may be safe, but out of an abundance of caution, we will consider it unsafe for untrusted users.

This would potentially impact how the corresponding SCC fields `defaultAddCapabilities` and `requiredDropCapabilities` are handled, but because of the above caution, it will not.

###### Impact on masked `/proc`

In the early Docker days, the path `/proc` had some fields that were set to read-only for all unprivileged containers, to prevent containers from reading content from the host, or potentially
changing fields in the virtual filesystem. As time has gone on, a lot of those paths are dropped from the kernel. However, for pods in the host user namespace, it remains useful to prevent pods
from having a fully read-write `/proc`. The problem is: a process that has any path in `proc` mounted as read-only has the entire `proc` read only, which means it cannot edit its own sysctls,
and cannot mount a sub-proc (for things like mounting a container inside of a container).

Thus, control over whether a pod can have an unmasked `/proc` was added in Kubernetes with the `ProcMountType` KEP. Historically, this KEP had nothing to do with user namespaces. As of Kubernetes
1.30, however, the procMount field in the pod spec was updated to require a user namespace. This was done because an unmasked `proc` without a user namespace effectively gives the pod another knob
to gain privileges. It's not as powerful as the pod `privileged` field, but it's not that far away either.

However, within a user namespace, the UID inside of the container is different from the host. With modern kernels, and a nonroot UID, the kernel will
[block write access](https://github.com/kubernetes/kubernetes/pull/126163#issuecomment-2240062247) to all of the fields that were formally deemed risky.
As such, `Unmasked` is now considered to be a safe value of the `procMount` field if the pod is also in a user namespace (which it must be to use the field at all).

At this time, `procMount` is not a field recognized by SCC. It should stay this way, as validation checking the presence of `UserNamespaces` is present in the kubelet and PSA.
If you have a user namespace, `procMount` `Unmasked` is safe to use.

###### Impact on other host namespaces

The Kubelet has validation against allowing a pod with any host namespaces (network, PID) from having access to a pod level user namespace.

Since this is validation done by the kubelet, we do not need to duplicate this for SCC.

###### Summary table

Below is a table that describes the above fields, but more explicitly for each SCC field

| SCC field                       | What the field controls     | Impact from user namespace                                                                                                                                                                                                |
|---------------------------------|-----------------------------|---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|
| AllowPrivilegedContainer        | privileged                  | No impact--the handling of privileged should not change if a pod is in a user namespace.                                                                                                                                  |
| DefaultAddCapabilities          | capabilities.add            | No impact, this is a field setting defaults.                                                                                                                                                                              |
| RequiredDropCapabilities        | capabilities.drop           | It is tempting to always set this to empty if a pod is in a user namespace, but we should not because of the implicit allowPrivilegeEscalation interaction, and because of the increased kernel attack surface.           |
| AllowedCapabilities             | capabilities.add            | It is tempting to always set this to the full list if a pod is in a user namespace, but we should not because of the implicit allowPrivilegeEscalation interaction, and because of the increased kernel attack surface.   |
| AllowHostDirVolumePlugin        | volumes.type == hostDir     | No impact on safety--idmapped mounts means a process in a user namespace will get the same access to volumes, so the kernel will not add any protections.                                                                 |
| Volumes                         | volumes                     | No impact on safety--idmapped mounts means a process in a user namespace will get the same access to volumes, so the kernel will not add any protections.                                                                 |
| AllowedFlexVolumes              | volumes.type == flexVolumes | No impact on safety--idmapped mounts means a process in a user namespace will get the same access to volumes, so the kernel will not add any protections.                                                                 |
| AllowHostNetwork                | hostNetwork                 | No impact on safety--a pod cannot be in a host network namespace and pod level user namespace.                                                                                                                            |
| AllowHostPorts                  | ports                       | No impact on safety--a pod accessing ports doesn't change if it is in a user namespace.                                                                                                                                   |
| AllowHostPID                    | hostPID                     | No impact on safety--a pod cannot be in a host PID namespace and pod level user namespace.                                                                                                                                |
| AllowHostIPC                    | hostIPC                     | No impact on safety--a pod cannot be in a host IPC namespace and pod level user namespace.                                                                                                                                |
| DefaultAllowPrivilegeEscalation | allowPrivilegeEscalation    | No impact, this is a field setting defaults.                                                                                                                                                                              |
| AllowPrivilegeEscalation        | allowPrivilegeEscalation    | If the handling of capabilities is changed based on a user namespace, this can be as well (as adding CAP_SYS_ADMIN sets this field to true).                                                                              |
| SELinuxContext                  | selinux                     | If the handling of capabilities is changed based on a user namespace, this can be as well (as adding CAP_SYS_ADMIN sets this field to true).                                                                              |
| RunAsUser                       | runAsUser                   | If a pod is in a user namespace, this field can safely be RunAsAny, however for the most restrictive policies, continuing to enforce this would add an additional layer of security.                                      |
| SupplementalGroups              | supplementalGroups          | If a pod is in a user namespace, this field can safely be RunAsAny, however for the most restrictive policies, continuing to enforce this would add an additional layer of security.                                      |
| FSGroup                         | fsGroup                     | If a pod is in a user namespace, this field can safely be RunAsAny, however for the most restrictive policies, continuing to enforce this would add an additional layer of security.                                      |
| ReadOnlyRootFilesystem          | readOnlyRootFilesystem      | No impact--the handling of read only rootfs should not change if a pod is in a user namespace.                                                                                                                            |
| Users                           | N/A                         | No impact--this field is dictating which openshift user can access this, and is not related to a pod field.                                                                                                               |
| Groups                          | N/A                         | No impact--this field is dictating which openshift user can access this, and is not related to a pod field.                                                                                                               |
| SeccompProfiles                 | seccompProfile              | No impact if a pod is in a user namespace on this field, but if this field is set to `*` or "empty" then it may be tempting to allow all capabilities, but the privileged escalation piece is another aspect to consider. |
| AllowedUnsafeSysctls            | sysctls                     | No impact--the handling of sysctls should not change if a pod is in a user namespace.                                                                                                                                     |
| ForbiddenSysctls                | sysctls                     | No impact--the handling of sysctls should not change if a pod is in a user namespace.                                                                                                                                     |

##### `UserNamespaceLevel` field

Add the following to the [Openshift API](https://github.com/openshift/api/blob/76a71dac36a08eab1b240c6c8d4e39c813b1b12b/security/v1/types.go):

```
       // userNamespaceLevel determines if the policy allows host users in containers.
       // Valid values are "AllowHostLevel", "RequirePodLevel", and omitted.
       // When "AllowHostLevel" is set, a pod author may set `hostUsers` to either `true` or `false`.
       // When "RequirePodLevel" is set, a pod author must `hostUsers` to `false`.
       // When omitted, this means no opinion and the platform is left to choose a reasonable default,
       // which is subject to change over time.
       // The current default is "AllowHostLevel".
       // +openshift:enable:FeatureGate=UserNamespacesPodSecurityStandards
       // +kubebuilder:validation:Enum="AllowHostLevel";"RequirePodLevel"
       // +default="AllowHostLevel"
       UserNamespaceLevel NamespaceLevelType `json:"userNamespaceLevel,omitempty" protobuf:"bytes,26,opt,name=userNamespaceLevel"`

...
// NamespaceLevelType shows the allowable values for the UserNamespaceLevel field.
type NamespaceLevelType string
...
const (
       // NamespaceLevelAllowHost allows a pod to set `hostUsers` field to either `true` or `false`
       NamespaceLevelAllowHost NamespaceLevelType = "AllowHostLevel"
       // NamespaceLevelRequirePod requires the `hostUsers` field be `false` in a pod.
       NamespaceLevelRequirePod NamespaceLevelType = "RequirePodLevel"
...
)

```

This field branches from its fellow `AllowHost*` namespace fields to follow current API conventions, which maintain boolean fields are not extendable enough.

##### restricted-v3

This SCC profile will be identical to the existing `restricted-v2`, except it will set the `UserNamespaceLevel` to `RequirePodLevel`,
thus forcing pods to be in a user namespace. This will make it a more restrictive profile, as the user on the host will not be
the same as the one inside the container.

After GA, this SCC could be made the default, as it's more secure than `restricted-v2`

Note: upstream Kubernetes deemed `runAsUser` to be a safe field to use for pods with `hostUsers: false`. However, the philosophy
of SCC utilizes defense-in-depth. `restricted-v3` will be the most restrictive policy.

##### nested-container

The intention of this SCC is to allow a user to run `podman` or other container engine inside of an Openshift pod.
Since user namespaces allow a process to gain access to the capabilities needed in a safe way, it's a natural addition
to the proposal of adding user namespaces generally.

This SCC will largely mirror the `restricted-v2` SCC, but have a couple of changes.
- SELinux context set to MustRunAs.SeLinuxOptions.Type: `container_engine_t`
    - This SELinux type has been developed to allow the majority of podman in pod situations, and can
      continue to be adapted without affecting the normal `container_t` which should be more restrictive.
- RunAsUserStrategy: RunAsAny
    - Any user should be allowed, as the user running in the container is not the same running outside.
- RequiredDropCapabilities: None
    - Inside of a user namespace, the capabilities a pod requests are only present in the user namespace,
      not on the host. Thus, even for a less trusted user, the capabilities should be safe to access.
- `UserNamespaceLevel` set to `RequirePodLevel`

Note: to use this SCC, the namespace must be labeled as `privileged` in Pod Security Standards. This is in part because
PSS doesn't recognized `container_engine_t` [in 1.30](https://github.com/kubernetes/kubernetes/pull/126165), and in part because
the baseline policy doesn't allow Unmasked procMount, even if the pod is in a user namespace, [in 1.30](https://github.com/kubernetes/kubernetes/pull/126163).

These are fixed in Kubernetes 1.31, and Openshift 4.18, meaning a namespace can be labeled as `baseline` and successfully utilize all the fields of the `nested-container` SCC.

#### Feature Gates and Sets

There are three feature gates of note for this enhancement: `UserNamespacesSupport`, `UserNamespacesPodSecurityStandards`, and `ProcMountType`.
All three of these feature gates are upstream Kubernetes features.

- `UserNamespacesSupport` is the main toggle for allowing user namespaces in Kubernetes. Without it, the kubelet and kube-apiserver will filter the `hostUsers` field from a pod spec.
- `UserNamespacesPodSecurityStandards` is for toggling the relaxation of Pod Security Standards when a pod is in a user namespace. This feature will remain in alpha, until the oldest supported kubelet
  of the newest kube-apiserver supports denying a pod when the feature is enabled but the kernel doesn't support user namespaces.
- `ProcMountType` is a feature for allowing an `Unmasked` `/proc` mount in a container. While not directly related to user namespaces on the surface, the feature relies on the presence of a user namespace.
  When paired with a user namespace, the feature opens the opportunity for a container to mount `/proc` mounts internally, which is useful for running nested containers.

Despite being in beta in 1.30, `UserNamespacesSupport` is not on by default, and thus won't be in 4.17. It is the main feature gate of this feature.
Enabling `UserNamespacesSupport`, `UserNamespacesPodSecurityStandards` and `ProcMountType` will move a cluster into TechPreviewNoUpgrade in 4.17.

`UserNamespacesPodSecurityStandards` is an auxilliary feature gate that loosens Pod Security Standards (PSS) for pods with user namespaces.
At the time of writing, this relaxation is restricted to the runAsUser/runAsGroup/runAsNonRoot fields.
However, there are [other](https://github.com/kubernetes/kubernetes/pull/126163) relaxations proposed.
Since this feature gate is responsible for relaxing validation done on pods that have user namespaces, it's path to GA needs to be done more carefully.
For instance, Kubernetes supports n-3 skew between the kube-apiserver and kubelet. If the kube-apiserver supports user namespaces, but the kubelet doesn't,
then the kube-apiserver may expect a pod is confined by a user namespace when it is not, and accidentally give it more privileges than it deserves.
Thus, `UserNamespacesPodSecurityStandards` is currently in alpha, and will not advance to GA until three releases after Kubelet supports the `UserNamespacesSupport`
feature gate.
This feature gate is also the main feature gate used to toggle the SCC changes above. This is done because the same version skew issue exists in Openshift,
and it would be better to keep UserNamespacesSupport on by default (not skewing from upstrem).
A consequence of this is the clusterpolicycontroller will also need to be taught to be aware of the `UserNamespacesPodSecurtiyStandards` feature,
in order to consider these relaxations when assigning a PSA label based on the assigned SCC (if the relaxations are applicable).

`ProcMountType` is not directly related to user namespaces, but it does rely on them, and is useful for podman in Openshift pod use cases. Specifically,
it's needed to allow podman to configure networking for the sub containers (as mounting a sub '/proc' requires the whole '/proc' be read-write, and '/proc' needs
to be writable to configure sysctls)

The goal is to graduate this feature in 4.18, meaning `UserNamespacesPodSecurityStandards` and `ProcMountType` will be enabled by default, and will not mark
a cluster as TechPreviewNoUpgrade. Doing so will require MinimumKubeletVersion (see below) to ensure all kubelets in the cluster respect the feature gates.

#### MinimumKubeletVersion in the apiserver

An additional piece is needed for ensuring every node that is in a cluster with `UserNamespacesSupport` enabled actually runs those pods with user namespaces.
Unfortunately, we cannot rely on homogenous feature gates protecting us in this case. For instance, an admin may pause a worker pool while an upgrade happens, but those workers
can still be scheduled to. The MCO should not be enabling alpha feature gates for older kubelets, even if the `featuregate` Openshift object enables them [1].

Since there can be a version skew between the kubelet and the kube-apiserver, and the feature gates are not assumed to be homogeneous, then it cannot be assumed that every
kubelet in the cluster will be new enough to support UserNamespaces.

While this would not be an issue if the feature gates are enabled on kubelet nodes after 4.15, we cannot assume the feature gate will be enabled.

Below is a table that describes some of the version skews between API server and kubelets:

|              | 4.14 api         | 4.15 api         | 4.16 api              | 4.17 api              | 4.18 api              |
|--------------|------------------|------------------|-----------------------|-----------------------|-----------------------|
| 4.14 kubelet | ok-no relaxation | ok-no relaxation | bad                   | bad                   | N/A                   |
| 4.15 kubelet | ok-no relaxation | ok-no relaxation | ok-no idmap in kernel | ok-no idmap in kernel | ok-no idmap in kernel |
| 4.16 kubelet | ok-no relaxation | ok-no relaxation | ok                    | ok-no idmap in kernel | ok-no idmap in kernel |
| 4.17 kubelet | ok-no relaxation | ok               | ok                    | ok                    | ok                    |
| 4.18 kubelet | N/A              | ok               | ok                    | ok                    | ok                    |

This table assumes the feature gate is enabled in the kubelet. Because of the aforementioned lack of homogeneity, this is not always the case. This table
represents the best-case scenario.
The Y column describes the kubelet version and X rows represent the kube-apiserver version. A key of the different values:
- ok-no relaxation: kube-apiserver doesn't expect there to be a user namespace, and thus does no relaxation for PSA or SCC, and there is no risk with this combination.
- ok-no idmap in kernel: the kernel version shipped with this version of the kubelet doesn't support idmapped mounts, and pods in a user namespace will always fail.
  this is OK because kubelet understands the feature, and tries to use it, and fails to create the pod. Thus, relaxation is allowed and is never exploited.
- ok: In these cases, both the kubelet and kube-apiserver, with the feature enabled, will create a pod with a user namespace. It is safe to relax policy in this situation.
- bad: In these cases, the kube-apiserver relaxing policy will be missed by the kubelet. The kubelet in these versions isn't even aware of the UserNamespacesSupport feature
  and thus won't recognize the field in the pod spec.

Thus, we need a method for the kube-apiserver to know for certain a node joining the cluster has a Kubelet that will also have the same feature gate set.
A way to do this that also could set a precedent for Openshift to have the apiserver be more conscious about kubelet version would be an openshift-apiserver
extension that refuses to let a kubelet join a cluster or get leases if it is not new enough.

For such a feature, there could be an extension to the NodeConfig object called `minimumKubeletVersion`. This field would be set on the cluster level.

1: I actually can't find any reference to this, but this is the way it *should* work, and thus we will rely on this.

##### MinimumKubeletVersion API

The API will be added to the NodeSpec object, which is a singleton cluster-wide:

```
type NodeSpec struct {
	...
	// MinimumKubeletVersion is the lowest version of a kubelet that can join the cluster
	// +kubebuilder:validation:Pattern=`^([0-9]*\.[0-9]*\.[0-9]*$`
	// +optional
	MinimumKubeletVersion string `json:"minimumKubeletVersion,omitempty"`
}
```

##### MinimumKubeletVerison authorization plugin

The operative piece of this feature will be an authorization plugin patched into the kube-apiserver. It will be run against requests coming from
kubelets, and if kubelet version reported through `Node.Status.NodeSystemInfo.KubeletVersion` is lower than `MinimumKubeletVersion` then the kube-apiserver
will deny the kubelet from gaining access to any resources that aren't Node get/update and `subjectaccessreviews`. In other words, the kubelet can read it's node
object and learn about what it has access to, but it's not allowed to gain access to any other API objects.

This is a fairly harsh punishment for a node, as it means it's effectively lost and the cluster admin needs to manually intervene to remove it. There will also be
some pieces to mitigate this pain for cluster admins.

##### MinimumKubeletVersion admission plugin

To protect nodes that are too old from being removed immediately, validation will be run on the MinimumKubeletVersion on admission to see if there are kubelets in the cluster
that are running with a version lower than the configured version. If so, the creation will be rejected, and the admin notified that they must upgrade their nodes before applying it.

##### MinimumKubeletVersion MCO awareness

MCO will read the MinimumKubeletVersion and mark machines as degraded if the node is not at least MinimumKubeletVersion.

##### Alternatives (Not Implemented) to MinimumKubeletVersion

It is also possible this feature should be paired with a corresponding kubelet field `minimumKubeletVersion`, where it exits if it is too old. This will prevent the kubelet from
running before it seeks to get credentials from the kube-apiserver, but also adds additional code overhead and backporting, plus given this feature would be added in z-streams, it's not
possible to rely on.

For this feature, there should be extensive documentation on what to do if this condition triggers. For instance, there could be situations where that would make a node completely
unrecoverable, and we should ensure customers can reclaim their nodes and ensure they are new enough.

Another possible alternative is a scheduling plugin that uses the presence of NodeRuntimeHandlerFeatures to check whether the node supports user namespaces. This is unfortunately an
incomplete solution because daemonsets don't go through scheduling.

### Risks and Mitigations

- Allowing user namespaces does open Openshift users to theoretical kernel vulnerabilities
    - While user namespaces have existed for a while, kernel concepts like idmapped mounting
      have not, and the newness could be seen as risky.
    - However, the tangible security advantages of allowing user namespaces outweigh the theoretical
      security risks.
    - Plus, for the majority of users, user namespaces will not be enabled to begin with.

### Drawbacks

## Open Questions [optional]

## Test Plan

- e2e tests for a user namespaced pod, especially with different volume types to verify kernel idmapped mount support
    - Also verify that the pod is actually in a user namespace
- upgrade tests where some worker pools are paused and the apiserver has the feature enabled, to verify kubelets won't create `hostUsers: false` pods if not confined by a user namespace
- long-term: e2e tests for running podman in a pod, so we have an established test path for users to know what works and what doesn't.
  - This is a requirement for moving the nested-container SCC out of tech preview.

## Graduation Criteria

### Dev Preview -> Tech Preview

- All pieces of this enhancement implemented, within the TPNU feature set
- Extensive documentation on enablement and common pitfalls
- Unit and e2e coverage
- Gather feedback from users rather than just developers (there are customers who are interested in trying this out)

### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Backhaul SLI telemetry
- Document SLOs for the component
- Conduct load testing
- Documentation in 4.18 warning admins to check whether any pods are given access to `nested-container` SCC before downgrading.
- MinimumKubeletVersion feature in the apiserver.

### Post GA
- Consider `restricted-v3` SCC for all workloads that formerly were pinned to `restricted-v2`

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy

This feature will be gated by TechPreviewNoUpgrade for 4.17, so it will not be able to upgrade. However, for GA, some considerations need to be made for Upgrade/Downgrade.

Specifically, both of these relate to how a cluster would be affected by the SCCs being created on the cluster, but the feature gate not being enabled (and thus `UserNamespaceLevel`
is not being enforced).

- `restricted-v3` SCC will be allowed for all users by default.
    - Since it is identical to `restricted-v2` with the exception of `UserNamespaceLevel`, `UserNamespaceLevel` being filtered out because an apiserver doesn't recognize it anymore
      (because the feature is no longer enabled) will allow it to be treated just like `restricted-v2` safely.
- `nested-container` SCC will not be enabled for users by default.
    - However, a namespace admin may enable it for a namespace, and then the cluster admin may trigger a downgrade, which would cause the issue spelled out above, but worse.
    - Since the SCC allows users access to `runAsUser: RunAsAny`, the `nested-container` SCC may be enabled on a namespace that gives higher privileges than anticipated without `UserNamespaceLevel: RequirePodLevel`
    - Thus, there should be documentation about 4.18 warning admins to check whether any pods are given access to `nested-container` SCC before downgrading.
        - A script will be provided, to comb through the pods and notify if there are pods that have the labels signifying they're using the `nested-container` SCC.
        - If there are such pods, these pods should be audited and potentially removed before downgrading.

Downgrade of the kubelet will be handled by the section below.

In the future, this feature will have the TechPreviewNoUpgrade flag removed, at which point all supported Kubelets and apiservers will
be aware of the feature gate and attempt to create a pod with a user namespace.

## Version Skew Strategy

In Kubernetes and Openshift, a version skew of n-3 between the kubelet and apiserver is supported. The key consideration in version skew:
if the kube-apiserver believes the cluster supports user namespaces, will every supported kubelet create a pod with a user namespace?

Unfortunately, as described in the `MinimumKubeletVersion in the apiserver` section, this cannot be relied upon because we cannot ensure the kubelet
will have the feature gate enabled. An operative part of this feature is relaxing validation done on kube-apiservers for pods with a user namespace,
but if the apiserver cannot trust the kubelet to fail to create a pod if the feature isn't enabled, it cannot trust the kubelet with relaxed validation.

The `MinimumKubeletVersion` feature is to fix this problem. If all apiservers support this field upon GA, then a cluster admin can set the field and ensure
their kubelets are new enough to certainly support user namespaces. This frees the apiserver to relax validation and GA these SCC fields.

This feature is not required for TechPreview, but is required for GA. The soonest we can GA this feature is 4.18.

## Operational Aspects of API Extensions

- Describe the possible failure modes of the API extensions.
    - The only API extension here is to SCC. The field will mirror existing ones, and thus have been vetted by time

## Support Procedures

Generally, failures in this feature will result in container creation failures for newly created containers that
use `hostUsers: false`. Some of these are platform problems; the kernel, CRI-O, kubelet and apiserver need to support
idmapped mounting.

However, there will be some configuration needed by the user. For instance, to use user namespaces, the OCI runtime also
needs to support the `hostUsers` field, but crun is the only packaged version that does as of today. If runc 1.2.0 is released
before 4.17, then it can be packaged and included in 4.17 and work OOTB. However, some users will need to update the OCI runtime
with a container runtime config

```
apiVersion: machineconfiguration.openshift.io/v1
kind: ContainerRuntimeConfig
metadata:
 name: enable-crun-worker
spec:
 machineConfigPoolSelector:
   matchLabels:
     pools.operator.machineconfiguration.openshift.io/worker: ""
 containerRuntimeConfig:
   defaultRuntime: crun
```

In most cases, the issues can be identified in the pod status, though looking through the kubelet and CRI-O logs may also be
needed.

There should be no issues to pods without `hostUsers: false`.

## Alternatives (Not Implemented)

- Less integration with SCC is possible, as currently proposed it gives the most flexibility for admins to mandate which users can access user namespaces.
- It was previously discussed to have a mechanism to allow a kubelet to fail to run if user namespaces aren't supported.
    - It was determined this was not needed, as the earliest this feature could go GA, all supported kubelets support the feature gate.
      Even though it's not supported by the nodes themselves, there will not be any vulnerabilities opened from an apiserver thinking a pod has user
      namespaces when corresponding kubelet/CRI-O doesn't actually create the pod with them.
- Add `nonroot-v3` SCC which would force `hostUsers: false` and allow `RunAsAny` for `runAsUser`
    - This was omitted to simplify kubelet/apiserver skew situations and may be readdressed in the future.

## Infrastructure Needed [optional]

N/A
