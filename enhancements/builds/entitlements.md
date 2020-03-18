---
title: openshift-entitlement-injection

authors:
  - "@gabemontero"
  - "@bparees" (provided a lot of input in tracking Jira)

reviewers:
  - "@bparees"
  - "@adambkaplan"
  - "@derekwaynecarr"
  - "@zvonkok"
  - "@who else?"
  
approvers:
  - "@bparees"
  - "@adambkaplan"
  - "@derekwaynecarr"
  - "@mrunalp"
  - "@who else?"
  
creation-date: 2020-02-18

last-updated: yyyy-mm-dd

<!-- status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced -->
status: provisional

see-also:
  - "/enhancements/builds/volume-secrets.md"  
  
---

# A More Automatic Experience For Use of Entitled RHEL Content In OpenShift V4 Pods and Builds


## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

1. We did *NOT* provide for multiple sets of entitlement credentials in 3.x.  There have been no official, RFE style registered requirements entitlements at a namespace level.  However, we've started to get (slightly) less official feedback that customers want this in a more automated sense (vs. the current manual steps).  At a minimum, and is noted below when alternatives are discussed, if namespaced entitlements are added with a subsequent implementation after global entitlements usability is improved, it needs to be seamless, and we minimally need to agree now on what the order of precedence will be (with the take on that listed below).

2. There has also been a question in the past needing to shard entitlements due to API rate limiting.  To date, it has been stated that is not a concern for builds.  Periodic checkpoints here could be prudent.

3. There is a general question of how entitlements are propagated for our managed service offerings.  There are
some attempts at capturing the particulars for this below, but it needs some vetting.  

4. This solution requires a mutating admission controller that intercepts all pod resources.  There is an open question as to how that is packaged, either as a) a separate admission webhook utilizing libraries like https://github.com/openshift/generic-admission-server, or b) as a patch we carry on the upstream k8s apiserver

## Summary

In V3.x, all OpenShift pods (and therefore builds) had the customer RHEL entitlements mounted into the build pod automatically
and without further intervention after install "for free" because:

 - the ansible based installer required/forced providing your subscription information in order to install OCP
 - for reference, see [these docs](https://docs.openshift.com/container-platform/3.11/install/host_preparation.html#host-registration) and [these roles](https://github.com/openshift/openshift-ansible/blob/release-3.11/roles/rhel_subscribe/tasks/main.yml)
 - next, Red Hats's docker daemon installed in 3.x had specific logic that auto-mounted the host's subscriptions 
 into the container

In V4.x, with 

- the change in the OCP subscription model between 3.x and 4.x:  you don't subscribe RHCOS nodes (in fact they
do not have or ever will the entire set of subscription manager RPMs, and as discussed in some of the alternatives
below, at most we will only seed enough subscription manager artifacts in RHCOS so that things like turning auto mount
on won't SEGFAULT).  In 3.x, the underlying linux was classic RHEL, and so hence the subscription manager was fully 
good to go out of the box (and hence could be shared with pods running on that node).
- the replacement of the openshift ansible based installer, so no gathering of entitlements during install
- builds running inside containers instead of on the host so builds cannot benefit from automounting of host content into the container

all those 3.x mechanisms are gone, and a series of steps from either the cluster admin and/or developer are required if entitlements are needed within a pod.  Additional steps are required if entitlements are required within an OpenShift build.

For the steps required in OpenShift Builds, see [the current 4.x doc](https://docs.openshift.com/container-platform/4.3/builds/running-entitled-builds.html)

This enhancement proposal aims to introduce more centralized and automated infrastructure to ease the burden of 
making RHEL subscriptions/entitlements available to both general workloads (pods) and builds.

## Motivation

Simplify overall usability for using RHEL Subscriptions/Entitlements with OpenShift, as we took a hit in 
4.x vs. 3.x.

Large, multi cluster administration needs to be simplified by results from our work here.

### Goals

No longer require developer steps (beyond an annotation) to get entitlements in a pod, assuming the cluster has entitlements configured.

No longer require (though still allow) manual manipulation (beyond a true/false config field) of the BuildConfig to consume entitlements.

No longer require (though still allow) manual injection of subscription credentials into a user's namespace (entitlements will come from a cluster scoped resource accessible to all users based on admin configuration).

Help customers who just started with OpenShift. They can work with `oc` and some sample yaml files but are completely overwhelmed with the entitlement story. Especially if they are used only to satellite and expect things just to work when they subscribe.  The RHCOS team has had to a lot of customer coaching in 4.x wrt entitlements.

Wherever possible during the implementation, identify any openshift/builder image or build controller code hits that could be leveraged by tekton based image building, including OpenShift Build V2, to take the equivalent mounting of subscription credential content into the build pod (where something other than the build controller for Build V1 does this), and supply the necessary arguments to the `buildah` binary (vs. the `buildah` golang API the openshift/builder image uses).  The "code hit structuring" implies adding common code that could be referenced by both solutions into a separate, simpler, utility github/openshift repository.  

### Non-Goals

1. Reach a V3.x level, where no post install activity is needed to consume entitlements.  In other words, no changes to the V4.x installer to allow the user to inject the subscription particulars will be encompassed in this enhancement.

2. Protecting this content from being viewed/copied by users who are otherwise intended to leverage it. If it's available for the pod or build to use, the user who can create a pod or build can also read / view / exfiltrate the credentials. It is technically impossible to avoid that, since anyone can write a build that does a "RUN cp /path/to/creds /otherpath" and then their resulting image will contain the creds.

## Proposal

The high level proposal is as follows:

1. Introduce a concept of generic global secrets that can be created by administrators and mounted into any pod
2. Introduce a configuration mechanism that allows the adminstrator to control which users/namespaces are allowed to mount a given global secret
3. Define an annotation api that users can apply to their pods to request injection of one more more global secrets, and what path to inject into
4. Define a CSI driver capable of copying global secrets into a volume that is made available for pods to mount
5. Define an admission controller that, taking into account the security configuration from (2), will add a PVC+volume mount(to be fullfilled by the CSI driver in (4)) to pods requesting injection, and reject PVCs/pods that include volume mounts but do not meet the security requirements from (2).
 

### User Stories

#### Existing User Stories (that will be improved upon)

1. As a cluster admin with users who need to build an image that needs to include Red Hat content that can only be 
accessed via a subscription, I want an easy way to distribute subscription credentials that can be consumed by my 
user's builds.

2. As a cluster admin with users who need to perform actions within their pods that require entitlements, I want an easy way to provide subscription credentials that those users can consume.

3. As a cluster admin who runs a multitenant cluster, I want to be able to control which users/namespaces can have the entitlements injected and provide different entitlements to different namespaces.

2. As a user who needs to build an image that includes subscription content, I want a simple way for my image build to 
have access to the subscription configuration/credentials.

3. As a user who needs to perform activities within a pod that require entitlements, I want my pod to automatically include entitlements if the cluster has them available.

#### New User Stories

1. As a multi-cluster administrator, I can enable my cluster for entitlements via a declarative API that works well 
with GitOps (or similar patterns) so that enabling entitled builds is not a burden as I scale the number of clusters 
under management.

TODO:  Still need some form of implementation details for this one if it remains in this proposal.

### Implementation Details/Notes/Constraints [optional]

The concerns for the implementation center around these questions:

- Delivery mechanism for subscription config/credentials to the cluster
- Where/How subscription config/credentials are stored on the cluster (e.g. what resources types+namespaces)
- In what form are the credentials provided (pem files, SubscriptionManager, Satellite)
- When/how does the pod or build consume the credentials (where consuming means the pod or build mounting it and, for builds, wiring the mount through to buildah)

#### Delivery Mechanism for the "encapsulation" of credentials

Discussion of how global secrets get defined on a cluster.

##### MultiCloud Manager/Advanced Cluster Management

"MCM" for future reference introduces a "klusterlet" which is [installed](https://www.ibm.com/support/knowledgecenter/SSBS6K_3.1.2/mcm/installing/klusterlet.html#install_ktl)
in the clusters it manages.

MCM also introduces a "compliance" with a list of "policies" (all backed by CRDs) which the klusterlet will
poll.  See [this document](https://www.ibm.com/support/knowledgecenter/SSBS6K_3.1.2/mcm/compliance/policy_overview.html) for details.

Each policy then has a list or k8s RBAC to define on the cluster, followed by a list of general k8s API object yaml
which can be applied on the cluster.  As the "klusterlet" polls, if it finds new or updated compliances or policies
within compliances, it performs the API object creates/updates/patches specified to change the system.

Whatever API objects we use to encapsulate the entitlement credentials/configuration in a single
cluster, and the associated RBAC needed to create those objects, will need to be spelled out so that they can be 
injected into MCM compliances and policies.

The assumption is that since the preferred solutions below do not entail creating/editing API objects specific to
OpenShift only, the existing API client employed by MCM should be able to consume what we provide.

##### OpenShift Hive

Hive has an analogous feature, "sync sets", to allow for API objects to be created on the clusters it manages.  At this
time, while MCM will leverage Hive for some things, it will not replace its compliance/policy/klusterlet
infrastructure with sync sets.

However, for existing dedicated/managed cluster using Hive without MCM, the same RBAC/Object spelled out for MCM should
suffice for Hive without MCM.
  
##### Manual cluster admin creation

As the title implies, a user with sufficient privilege would create the API objects we decide are the "encapsulation"
for the entitlement/subscription credentials at the namespace / cluster level we end up supporting.

#### Where/How do pods/builds get the credentials from (i.e. the precise form of the "encapsulation")

##### Global secret option

This enhancement proposes a new concept of a "global secret".  Administrators will be able to define secrets in a unique namespace (likely openshift-config) and then configure a CSI driver + Admission controller which will be able to inject those secrets into pods that request the content, based on a configuration the administrator provides.

**IF** we were to mimic 3.x the behavior would be: if the global credential(s) exist, then always mount.  As a result, they are always present, however at this time, the current sentiment for 4.x is to opt in to receiving entitlements instead of the default being the credentials are always there, where either an annotation (or perhaps new API field(s) in the case of builds). determines whether they are injected.  

As noted in the open question, simply getting credentials at the global level may not be sufficient, so additional
alternatives / complementary pieces to a global solution are articulated below.

The preferred implementation path at this time is to build the following set of components inject the global secret contents 
in a pod:
 - a [CSI plugin](https://github.com/container-storage-interface/spec) would fetch the secret(s) and write it on the underlying 
 node/host filesystem on a path we control so that CRIO can mount the content
 - though for our purposes, `plugin`, `driver`, and `endpoint` seem to be the same thing based on which document you are
 reading.
 - k8s provides Kubernetes CSI Sidecar Containers: a set of standard containers that aim to simplify the development and 
 deployment of CSI Drivers on Kubernetes.
 - There are CSI volume and persistent volume types in k8s.
 - per [k8s volume docs](https://kubernetes.io/docs/concepts/storage/volumes/#csi) the CSI types do not support direct
 reference from a Pod and may only be referenced in a Pod via a `PersistentVolumeClaim`
 - To that end, one of the sidecar containers, the [external provisioner](https://kubernetes-csi.github.io/docs/external-provisioner.html), helps with this
 - It watches Kubernetes `PersistentVolumeClaim` objects, and if the PVC 
 references a Kubernetes StorageClass, and the name in the provisioner field of the storage class matches the name 
 returned by the specified CSI endpoint `GetPluginInfo` call, it calls the CSI endpoint `CreateVolume` method to provision
 the new volume.
 - there are also lifecycle methods for destroy, resize, etc.  
 - the CSI plugin would be hosted as a `DaemonSet` on the k8s cluster, to get per node/host granularity
 - and that `DaemonSet` would be privileged so as to have access to the pod's associated node/host file system.
 - the [CSI spec](https://github.com/container-storage-interface/spec/blob/master/spec.md) details the entire architecture,
 concepts, and protocol
 - so Volume provisioning could access the node/host filesystem, in particular the directory (and only the directory)
 where the secret contents will be copied to
 - As part of Volume creation/provisioning a key element is this: CSI has two step mounting process. staging and publishing.
 - They correspond to the endpoint's `NodeStageVolume` and `NodePublishVolume` implementations 
 - Staging is node local and publish is pod local. So we will probably want to stage the content once and publish it for each pod 
 (which is nothing but creating a bind mount for original volume)
 - That means we do not have to necessarily read and copy the secret(s) contents on every pod creation
 - the [CSI spec volume lifecycle section](https://github.com/container-storage-interface/spec/blob/master/spec.md#volume-lifecycle) 
 talks about a pre-provisioned volume.
 - If we can implement this, then we can build an associated controller that watches the global secret, or per namespace secrets that have been 
 annotated/labelled, and if possible that engages the CSI plugin and its `ControllerPublishVolume` implementation to redo 
 the staging step and update the secret contents.  This is *somewhat* akin to the injection controller employed by the 
 Global Proxy support.
 - **TODO**: in looking at the [kubernetes-csi hostpath example](https://github.com/kubernetes-csi/csi-driver-host-path)
 it seems very very similar to what we want to do, but with this exception:  it employs a `StatefulSet` for deploying
 the CSI driver, and in the comments there, it says the following, which does not work for us:
 
 ```bash
    # One replica only:
    # Host path driver only works when everything runs
    # on a single node. We achieve that by starting it once and then
    # co-locate all other pods via inter-pod affinity
```

 - that single node notion obviously won't work for us.  Need to understand why that is the case.
 - Moving on from the CSI plugin, an admission controller/webhook path (see open questions regarding "packaging" options there) 
 that based on the presence of the annotation on the pod (or one of several for per namespace perhaps), and the presence 
 of a global whitelist that stipulates which namespaces can annotate their pod.  See risks and mitigations for 
 speculation around the need for more granular control.
 - can mutate the pod, adding volume mounts of the "entitlement" volume 

###### Build Specific consumption

Once the "entitlement" volume is mounted in the build pod, the openshift builder image will need to map the pod mounted volumes to the requisite buildah parameters.

##### User Namespace Option (future goal)

***This section is not in scope for the first pass implementation.***

Users could provide secret(s) with a well known annotation in their namespace that the global solution
noted above can look for and mount in the pod in the same way as the global secret.

The global solution minimally would only look for secrets with such an annotation or label in the namespace of the 
pod who has the annotation for mounting entitlements.

Labels might be preferable in the per namespace case to facilitate queries. 

This would be a slight step up from having to manually cite those secrets in the BuildConfig Spec after creating them,
as documented today. 

The per namespace version would act as an override and take precedence over the global copy.


##### Current state

1) Currently, auto mount of secrets from the host into the Pods enabled per default. Some people raised concerns about 
this default setting.  Also, RHCOS does not, nor will ever have, the RPM installed by default such that the rhsm.conf
file is present.

2) Disabling auto mount of secrets from the host is under consideration by RHCOS

3) Secrets on the host to provide the creds and rhsm.conf would then be needed.

4) Where again, nobody wants to use the MCO for that.


#### In what form are the credentials provided to the cluster

There will also be well defined keys within the global secret that account for the various forms:

- set of pem files
- SubscriptionManager config and certs
- Satellite config and instance information, yum repo file if needed, and certs  

All of these could be contained in a single secret created with multiple `--from-file` options, correlating to 
multiple key/value pairs in the secret.

We would need to proscribe well known key names to be used for each.  And perhaps for each type of thing, the CSI
plugin would need to copy them to the best place possible on the node/host for CRIO consumption.

If this proves untenable, multiple secrets per entitlement, with different annotations for each, would be needed.

The current though is to go with one secret vs. multiple, but this can be adjusted during implementation if need be.

#### When does the pod or build consume the credentials

For pods, an annotation should be used to flag the pod as desiring credentials.  This can also allow the user to pick from multiple available secrets for injection.  The annotation needs to be sufficient to determine:

1. Which global secret to inject
2. Where to inject it (path)

For builds, an opt-in at both the `BuildConfig` level and global build controller config level (`BuildDefaults`) would be provided, where in the `BuildConfig` case it would be an API field addition under the `BuildSource`.  Something like `includeSubscription`.  And an analogous field name and type would be added to `BuildDefaults`. 

The build controller in turn would supply the annotation(s) the admission apparatus respects
onto the build pod.


### Risks and Mitigations

Predefined annotations/labels should largely suffice, but we are proposing new fields in the existing Build V1 API 
objects to facilitate opt-in at both the global level and per BuildConfig level.

How the CSI plugin is packaged (see the open questions) could entail some longer term risk based on which choice is made.

The intent of the associated whitelist for which namespaces can access entitlements is to give administrators sufficient
control around who can access which entitlements.  If not, it would seem we would need to create a new API type that RBAC
could be defined off of to get more granular authorization. 

## Design Details

### Test Plan

With both 3.x and 4.x, there have been no automated e2e tests / extended tests out of openshift/origin
for validating consumption of entitled content.  The reason presumably being 
that there is not set of credentials that our CI system can use.

We've revisited this as part of composing this enhancement, and have devised the following recipe, assuming
one of the forms where a secret of some sort is created:
- using the existing build global cluster configuration tests in the serial suite
- do a run where some fake creds are set up in a predetermined secret
- run a docker strategy build that cats the expected mounted secret content 
- search for the cat output in the build logs 

Since the build pod is dependent on the general pod injection behavior, this should cover both pod injection and build consumption.

Then there is the question of whether manual testing with actual yum install of entitled content by QE should consider 
with any of the three forms of credentials
- the actual pem files
- SubscriptionManager configuration, including certs
- Satellite instance and configuration, including certs

QE already has existing test cases around using subscription manager with the manual process in play today for 4.x.
They will be adjusted and serve as the minimum for implementations of this proposal.  Most likely this is sufficient,
but due diligence around additional testing for the other scenarios will be assessed during test case review.

And a note to QE:  the RHCOS team wanted us to know they tested the "jump-host" scenario with respect to Satellite
in the 4.3 timeframe.  See https://docs.google.com/document/d/1-Il7O86vQBTKq2QNe47kQF-66n1NHgM9drEmtKWeELU/edit#heading=h.igkdrwoly0rc
for details.

### Graduation Criteria

This should be introduced directly as a GA feature when it is implemented.

However, based on QE's ability to deal with SubscriptionManager and/or Satellite, support for all three sources of 
credentials may need to be staged across OpenShift releases.

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

## Implementation History

> 1. Current milestone: initial introduction of this enhancement proposal.

## Drawbacks

N/A

## Alternatives

1. A build only solution could entail the build controller replacing the CSI plugin and admission framework and mounting
secrets into the build pod.

2. CRI-O already has config options (see the [config file doc](https://github.com/cri-o/cri-o/blob/master/docs/crio.conf.5.md))  to specify host directories that should always be mounted into containers (i.e. `/etc/containers/mounts.conf` to override the default `/usr/share/containers/mounts.conf`), akin to what the Red Hat Docker Daemon used to provide with OpenShift V3.  Guidance in the early 4.x time frame has been around the cluster admin using the MCO post install to effectively update the filesystem of each node with the necessary subscription related files in a well known location that CRI-O would look for.  There are some concerns though about using MCO for this purpose.  One, the MCO api is not natural for someone who wants to do something higher level like "enable entitlements for builds".  Second, using the MCO for any node update requires a restart of the node, which is onerous for this scenario.  Also note, the RHCOS team wants to disable their current default setting of auto-mounting secrets from the host (i.e. creating an empty `/etc/containers/mounts.conf`), since RHCOS does not fully install subscription manager (and never will), and they have problems when only partial subscription manager metadata is available.  [RHCOS has the subscription-manager-certs installed but are missing the subscription-manager package which carries the rhsm.conf.](https://bugzilla.redhat.com/show_bug.cgi?id=1783393)

They are entertaining an MCO solution to work around this problem, but disabling auto-mount, and then having a solution which adds all the subscription manager in one fell swoop, would be their preference.  In the end, providing an alternative to an MCO based solution **would seem** to facilitate RHCOS's future intentions and it would seem we should minimally coordinate with them to facilitate how they stage future changes  

3. Hostpath injection.  Hostpath injection is not an option for pods (it would require users have permission to create pods with hostpath mounts which is unacceptable).  

Hostpath injections for builds could be done since we control the build pod and can protect it from user tampering, however there is a general desire to move build pods to be less privileged, not more.  

In addition, storing entitlements on nodes(such that they can be hostmounted) has significant drawbacks as noted above(Namely, MCO being a poor distribution mechanism for this information).

Furthermore we are now trying to solve the entitlement problem for all pods, not just builds.  

As such, this approach doesn't warrant further consideration.

## Infrastructure Needed [optional]

QE will need to have actual RHEL subscriptions/entitlements, ideally all 3 forms of input (actual pem's, Subscription
Manager, Satellite), to inject into the system.
