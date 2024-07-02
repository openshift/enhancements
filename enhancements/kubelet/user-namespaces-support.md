---
title: user-namespaces-support
authors:
  - haircommander
reviewers:
  - rphilips
  - giuseppe
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - mrunalp
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - deads2k
creation-date: 2024-06-17
last-updated: 2024-06-17
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPNODE-2000
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
To integrate into Openshift, we must enable the feature UserNamespacesSupport and UserNamespacesPodSecurityStandards,
integrate support into SecurityContextContraints (SCC), and add a feature that controls for Kubelet version skew.

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

- 

## Proposal

There are three pieces to this proposal:
- Extend SCC to be aware of the hostUsers field:
    - Add a new field AllowHostUser to SCC, which will be true by default
    - Add a new SCC to the default list: restricted-v3
        - This SCC will be identical to restricted-v2, but have AllowHostUsers set to false
    - Add a new SCC to the default list: container-in-pod. It will have:
        - SELinux context set to MustRunAs.Type: container_engine_t
        - RunAsUserStrategy: RunAsAny
        - RequiredDropCapabilities: None
        - AllowHostUser: false
        - And otherwise mirror the restricted-v2 profile
- Add the features UserNamespacesSupport and UserNamespacesPodSecurityStandards to the list that qualify a cluster as TechPreviewNoUpgrade

### Workflow Description

N/A

### API Extensions

- Add AllowHostUser to SCC. This relies on approval from the apiserver team.

### Topology Considerations

#### Hypershift / Hosted Control Planes

- Support for a user to change the node config object will have to be investigated.

#### Standalone Clusters

I don't think there are any special topology considerations for standalone.

#### Single-node Deployments or MicroShift

From my understanding, there should not be any large resource consumption changes for this feature.

### Implementation Details/Notes/Constraints

#### SecurityContextConstraint Updates

##### `AllowHostUsers` field

Add the following to the [Openshift API](https://github.com/openshift/api/blob/76a71dac36a08eab1b240c6c8d4e39c813b1b12b/security/v1/types.go):

```
--- a/security/v1/types.go
+++ b/security/v1/types.go
@@ -85,6 +85,9 @@ type SecurityContextConstraints struct {
        AllowHostPID bool `json:"allowHostPID" protobuf:"varint,11,opt,name=allowHostPID"`
        // AllowHostIPC determines if the policy allows host ipc in the containers.
        AllowHostIPC bool `json:"allowHostIPC" protobuf:"varint,12,opt,name=allowHostIPC"`
+       // AllowHostUsers determines if the policy allows host users in the containers.
+       AllowHostUsers bool `json:"allowHostIPC" protobuf:"varint,26,opt,name=allowHostUsers"`
+
        // DefaultAllowPrivilegeEscalation controls the default setting for whether a
        // process can gain more privileges than its parent process.
        // +optional
```

TODO: Does this need to be a pointer to a bool to allow to default to true?

This value will function similarly to its peers corresponding to PID, IPC and Network namespaces, with the
exception that it will default to `true` to begin.

##### restricted-v3

**NOTE TO REVIEWER** I am not very opinionated on this being present. I think it would provide value to customers, but I don't know that folks are asking for it.
If this proposal has too much, this would be the first thing I would drop.

this SCC profile will be identical to the existing restricted-v2, except it will set the `AllowHostUser` to `false`,
thus forcing pods to be in a user namespace. This will make it a more restrictive profile, as the user on the host will not be
the same as the one inside the container.

##### container-in-pod

**NOTE TO REVIEWER** I think the naming here will be a source of contention.

The intention of this SCC is to allow a user to run `podman` or other container engine inside of an Openshift pod.
Since user namespaces allow a process to gain access to the capabilities needed in a safe way, it's a natural addition
to the proposal of adding user namespaces generally.

This SCC will largely mirror the `restricted-v2` SCC, but have a couple of changes.
- SELinux context set to MustRunAs.Type: `container_engine_t`
    - This SELinux type has been developed to allow the majority of podman in pod situations, and can
      continue to be adapted without affecting the normal `container_t` which should be more restrictive.
- RunAsUserStrategy: RunAsAny
    - Any user should be allowed, as the user running in the container is not the same running outside.
- RequiredDropCapabilities: None
    - Inside of a user namespace, the capabilities a pod requests are only present in the user namespace,
      not on the host. Thus, even for a less trusted user, the capabilities should be safe to access.
- AllowHostUser set to false

#### Feature Gates and Sets

Finally, add the features UserNamespacesSupport and UserNamespacesPodSecurityStandards to the list that qualify a cluster as TechPreviewNoUpgrade,
for the 4.17 cycle. We'll address whether we can move the feature out of tech preview after that.

### Risks and Mitigations

- Allowing user namespaces does open Openshift users to theoretical kernel vulnerabilities
    - While user namespaces have existed for a while, kernel concepts like idmapped mounting
      have not, and the newness could be seen as risky.
    - However, the tangible security advantages of allowing user namespaces outweigh the theoretical
      security risks.
    - Plus, for the majority of users, user namespaces will not be enabled to begin with.

### Drawbacks

- The MinmiumKubeletVersion field still needs to be specified by an end user, which functionally is not much different than a cluster admin checking kubelet versions
    - The controller that enables the feature gates in the kubelet can potentially set this field if UserNamespacesSupport is added

## Open Questions [optional]

- Should we include the restricted-v3 profile in this enhancement?
- Is there a better name than `container-in-pod`
    - Do we need to include `container-in-pod` OOTB if we can allow a cluster admin to create it?

## Test Plan

- e2e tests for a user namespaced pod, especially with different volume types to verify kernel idmapped mount support
- long-term: e2e tests for running podman in a pod, so we have an established test path for users to know what works and what doesn't.

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
- User facing documentation created in [Openshift-docs](https://github.com/Openshift/Openshift-docs/)

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy

- To begin with, this feature will be gated by TechPreviewNoUpgrade, so it will not be able to upgrade
- A downgrade of the apiserver will cause this feature to not work as well, as the SCC changes will be lost on older versions
- A downgrade of the kubelet down to 1.28 should continue to work
    - Support for idmapped mounting was added in 1.28, so pods with volumes that have the hostUsers field specified will fail.
    - Since this feature is being added in 4.17, the version skew supported goes down to 4.14, meaning it must stay in TechPreview for 4.17

In the future, this feature will have the TechPreviewNoUpgrade flag removed, at which point all supported Kubelets and apiservers will
be aware of the feature gate and attempt to create a pod with a user namespace. The only special upgrade/downgrade considerations are
for the SCC changes, which users will loose access to if the cluster downgrades.

## Version Skew Strategy

In Kubernetes and Openshift, a version skew of n-3 between the kubelet and apiserver is supported. The key consideration in version skew:
if the kube-apiserver believes the cluster supports user namespaces, will every supported kubelet create a pod with a user namespace?

There is risk if this does not happen, as we intend on having the apiserver relax validation for a pod that it believes is confined by a user namespace,
when in reality it is not, thus leading to security vulnerability.

In this enhancement, we assume homogeneous feature gates, where both the kubelet and kube-apiserver have the same feature gates set.

Support for user namespaces were intitally added in 1.27 without idmapped mount support. However, it used a different feature gate
`UserNamespacesStatelessPodsSupport`. As such, 4.14 kubelet does not create an pod with a user namespace when `UserNamespacesSupport` feature
is added. Thus, the skew from 4.17->4.14 is not supported, and the feature must stay in tech preview.

As for the future possibility of GA'ing as early as 4.18, the similar conversation happens. In this case, 4.15 has support for the `UserNamespacesSupport`
feature gate, and kubelet will create a user namespace for the pod. Further, support was added in CRI-O to deny a pod that was created with ID mapped mounts,
but the kernel doesn't support IDmapped mounts in 4.15/1.28. The kernel won't support them until 4.16 (when RHCOS is released based on RHEL 9.4).

Thus, we are safe to GA as early as 4.18, as CRI-O will fail to create a container in 4.15 that doesn't have idmapped mount support.
Even pods that have no volumes do need to have mounts done, and since the RHEL 9.2 kernel doesn't support the idmapped mount options,
all user namespaced pods will fail on 4.15 and below.

## Operational Aspects of API Extensions

- Describe the possible failure modes of the API extensions.
    - The only API extension here is to SCC. The field will mirror existing ones, and thus have been vetted by time

## Support Procedures

Generally, failures in this feature will result in container creation failures for newly created containers that
use `hostUsers: false`. Some of these are platform problems; the kernel, CRI-O, kubelet and apiserver need to support
idmapped mounting.

However, there will be some configuration needed by the user. For instance, to use user namespaces, the OCI runtime also
needs to support the hostUsers field, but crun is the only packaged version that does as of today. If runc 1.2.0 is released
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

## Alternatives

- Less integration with SCC is possible, as currently proposed it gives the most flexibility for admins to mandate which users can access user namespaces.
- It was previously discussed to have a mechanism to allow a kubelet to fail to run if user namespaces aren't supported.
    - It was determined this was not needed, as the earliest this feature could go GA, all supported kubelets support the feature gate.
      Even though it's not supported by the nodes themselves, there will not be any vulnerabilities opened from an apiserver thinking a pod has user
      namespaces when corresponding kubelet/CRI-O doesn't actually create the pod with them.

## Infrastructure Needed [optional]

N/A
