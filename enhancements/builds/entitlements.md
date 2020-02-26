---
title: openshift-builds-improve-use-of-rhel-entitlements

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

# A More Automatic Experience For Use of Entitled RHEL Content In OpenShift V4 Builds


## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

 > 1. CRI-O already has config options (see the [config file doc](https://github.com/cri-o/cri-o/blob/master/docs/crio.conf.5.md)) 
>to specify host directories that should always be mounted into containers
>(i.e. `/etc/share/containers/mounts.conf`), akin to what the Red Hat Docker Daemon used to provide with OpenShift V3.  
>Guidance in the early 4.x time frame has been around the cluster admin using the MCO post install to effectively update the
>filesystem of each node with the necessary subscription related files in a well known location that CRI-O would look
>for.  There is concern though using this through MCO from a couple of perspectives.  One, the MCO api is not a 
>natural for someone who wants to do something higher level like "enable entitlements for builds".  Second, using the
>MCO for any node update requires a restart of the node, which is onerous for this scenario.  Also note, the RHCOS
>team want stop disable their current default setting of auto-mounting secrets from the host (i.e. stuff from 
>`/etc/share/containers/mounts.conf`), since RHCOS does not fully install subscription manager and they have 
>problems when only partial subscription manager metadata is available.  [RHCOS has the subscription-manager-certs 
>installed but missing the subscription-manger package which carries the rhsm.conf.](https://bugzilla.redhat.com/show_bug.cgi?id=1783393)
>They are entertaining an MCO solution to work around this problem, but disabling auto-mount, and then having a 
>solution which adds all the subscription manager in one fell swoop, would be their preference. 
> 2. There has been discussion elsewhere around having a more generic notion of "global resources" that any pod can
>mount.  There is an open question of whether this proposal should tackle that.  Would we gate this enhancement on the 
>availability of such a feature (probably "not sure")?  Upstream feature work could be implied with such an item.  Or 
>exploration of CSI based solutions to read secrets is an option, though perhaps this scenario would be an abuse of the CSI pattern,
>and some form of performance assessment would be prudent.  A slight variant of that would be providing a special
>`SecretProviderClass` for defining where secrets are read from (i.e. the default subscription locations vs. the default
>k8s secret mount points).  And after 4.5 epic planning it was made clear that a more general solution around entitlements, beyond just builds,
>is desired.  PM has the to-do to investigate, prioritize, and create/assign epics accordingly.  Investigating in the 
>interim around build specific needs via this proposal has been deemed acceptable, but certainly a subsequent, broader
>proposal may necessitate adjustments to this one.
> 3. We did *NOT* provide for multiple sets of entitlement credentials in 3.x.  There have been no official, RFE style  
>registered requirements entitlements at a namespace level.  However, we've started to get (slightly) less official
>feedback that customers want this in a more automated sense (vs. the current manual steps).  At a minimum, and is
>noted below when alternatives are discussed, if namespaced entitlements are added with subsequent implementation
>after global entitlements usability is improved, it needs to be seamless, and we minimally need to agree now
>on what the order of precedence will be (with the take on that listed below).
> 4. There has also been a question in the past needing to shard entitlements due to API rate limiting.  To date, it
>has been stated that is not a concern for builds.  Periodic checkpoints here could be prudent.
> 5. There is also the general question of how entitlements are propagated for our managed service offerings.  There are
some attempts at capturing the particulars for this below, but it needs some vetting.

## Summary

So in V3.x, all OpenShift Builds essentially had the customer RHEL entitlements mounted into the build pod automatically
and without further intervention after install "for free" because:

 - the ansible based installer required/forced providing your subscription information in order to install OCP
 - for reference, see [these docs](https://docs.openshift.com/container-platform/3.11/install/host_preparation.html#host-registration) 
 and [these roles](https://github.com/openshift/openshift-ansible/blob/release-3.11/roles/rhel_subscribe/tasks/main.yml)
 - next, Red Hats's docker daemon installed in 3.x had specific logic that auto-mounted the host's subscriptions 
 into the container

In V4.x, with 

- the removal of the docker daemon, meaning no docker socket and no access to the host's content
- and the total replacement of the openshift ansible based installer, so no gathering of entitlements during install
- and the total change in the OCP subscription model between 3.x and 4.x:  you don't subscribe RHCOS nodes (in fact they
do not have or ever will the entire set of subscription manager RPMs, and any as discussed in some of the alternatives
below, at most we will only seed enough subscription manager artifacts in RHCOS so that things like turning auto mount
on won't SEGFAULT).  In 3.x, the underlying linux was classic RHEL, and so hence the subscription manager was fully 
good to go out of the box (and hence could be shared with pods running on that node).

all those 3.x mechanisms are gone, and a series of steps from either the cluster admin and/or developer attempting
to run OpenShift Builds leveraging entitled content must be performed after install to enable access to the entitled
content during the build.

The highlights of those steps from [the current 4.x doc](https://docs.openshift.com/container-platform/4.3/builds/running-entitled-builds.html):

1) First, either
 
- create an ImageStreamTag to the UBI image from `registry.redhat.io` in the `openshift` namespace, or 
- reference the UBI image directly and include a pull secret to `registry.redhat.io` in your BuildConfig

*NOTE*: in 4.5, image registry and build updates are occurring so that image streams and builds will have
access to the `registry.redhat.io` credentials supplied in the install pull secret.  As such, the above steps
in the manual process will no longer be required.

2) Next, based on your access to the credentials:

- With direct access to the entitlement pem files, create a generic secret from the files and reference the secret in
your BuildConfig's input/source secret list
- With Subscription Manager, and S2I build strategy, create secret(s) for the Subscription Manager configuration and 
certs and reference the secret(s) in your BuildConfig's input/source secret list
- With Subscription Manager, and Docker build strategy, you have to modify your Dockerfile and manually copy and 
initialize as needed the information from the base image
- With Satellite, and S2I build strategy, create a secret to reference the repository config file you created for your
satellite instance and reference the secret in you BuildConfig's input/source secret list
- With Satellite, and Docker build strategy, you have to modify your Dockerfile and manually copy and initialize as 
needed the information from the base image as well as your repository config file for your satellite instance
- And remember to set the skip layers optimization option (i.e. squash layers) with Docker Strategy builds

*Another note on Satellite*: you can also subscribe nodes with Satellite (reference the "jump-host"), which results in 
the pem files being placed in a well known location on the file system.  A user with sufficient privilege to the node 
could obtain the pem files. 

This enhancement proposal aims to introduce more centralized and automated infrastructure to ease the burden of 
using OpenShift Builds with RHEL subscriptions/entitlements.

## Motivation

Simplify overall usability for using RHEL Subscriptions/Entitlements with OpenShift Builds, as we took a hit in 
4.x vs. 3.x.

And large, multi cluster administration needs to be simplified by results from our work here.

### Goals

No longer require (though still allow) manual manipulation of the BuildConfig to consume entitlements.

No longer require (though still allow) manual injection of subscription credentials into a user's namespace.

Wherever possible during the implementation, structure the openshift/builder code hits such that they can be leveraged 
by tekton based image building, including OpenShift Build V2, to take the equivalent mounting of subscription 
credential content into the build pod (where something other than the build controller for Build V1 does this), and 
supply the necessary arguments to the `buildah` binary (vs. the `buildah` golang API the openshift/builder image
uses).  The "code hit structuring" implies adding common code that could be referenced by both solutions into a 
separate, simpler, utility github/openshift repository.  

### Non-Goals

1. Reach a V3.x level, where no post install activity is needed to consume entitlements.  In other words, no changes 
to the V4.x installer to allow the user to inject the subscription particulars will be encompassed in this enhancement.

2. Protecting this content from being viewed/copied by users. If it's available for the build to use, the user who can 
create a build can also read / view / exfiltrate the credentials. It is technically impossible to avoid that, since 
anyone can write a build that does a "RUN cp /path/to/creds /otherpath" and then their resulting image will contain 
the creds.

## Proposal

### User Stories [optional]

#### Existing User Stories (that will be improved upon)

1. As a cluster admin with users who need to build an image that needs to include Red Hat content that can only be 
accessed via a subscription, I want an easy way to distribute subscription credentials that can be consumed by my 
user's builds.

2. As a user who needs to build an image that includes subscription content, I want a simple way for my image build to 
have access to the subscription configuration/credentials.

#### New User Stories

1. As a multi-cluster administrator, I can enable my cluster for entitlements via a declarative API that works well 
with GitOps (or similar patterns) so that enabling entitled builds is not a burden as I scale the number of clusters 
under management.

TODO:  Still need some form of implementation details for this one if it remains in this proposal.

#### Out Of Scope Stories

1. As a user I have a (non OpenShift Build) pod whose entry point is a script that installs software from entitled 
sources, and I want a simpler way for the necessary entitlemenet information injected in the pod.

(See the Open Questions for other work that could make this happen). 

### Implementation Details/Notes/Constraints [optional]

So the concerns for the implementation center around 5 questions:

- Mechanism for delivering for subscription config/credentials to the cluster
- Where/How subscription config/credentials are stored on the cluster (e.g. what resources types+namespaces)
- In what form are the credentials provided (pem files, SubscriptionManager, Satellite)
- How does the build consume the credentials
- When does the build consume the credentials (where consuming means telling buildah to mount it)

#### Delivery Mechanism for the "encapsulation" of credentials

##### IBM Mulitcloud Manager (though a rename is coming with transfer to Red Hat)

So "MCM" for future reference introduces a "klusterlet" which is [installed](https://www.ibm.com/support/knowledgecenter/SSBS6K_3.1.2/mcm/installing/klusterlet.html#install_ktl)
in the clusters it manages.

And MCM also introduces a "compliance" with a list of "policies" (all backed by CRDs) with the klusterlet will
poll.  See [this document](https://www.ibm.com/support/knowledgecenter/SSBS6K_3.1.2/mcm/compliance/policy_overview.html) for details.

Each policy then has a list or k8s RBAC to define on the cluster, followed by a list of general k8s API object yaml
which can be applied on the cluster.  As the "klusterlet" polls, if it finds new or updated compliances or policies
within compliances, it performs the API object creates/updates/patches specified to change the system.

Whatever API objects we use to encapsulate the entitlement credentials/configuration in a single
cluster, and the associated RBAC needed to create those objects, will need to be spelled out so that they can be 
injected into MCM compliances and policies.

##### OpenShift Hive

Hive has an analogous feature, "sync sets", to allow for API objects to be created on the clusters it manages.  At this
time, while the "red washed" MCM will leverage Hive for some things, it will not replace its compliance/policy/klusterlet
infrastructure with sync sets.

However, for existing dedicated/managed cluster using Hive without MCM, the same RBAC/Object spelled out for MCM should
suffice for Hive without MCM.
  
##### Manual cluster admin creation

As the title implies, a user with sufficient privilege would create the API objects we decide are the "encapsulation"
for the entitlement/subscription credentials at the namespace / cluster level we end up supporting.

#### Where do we get the credentials from (i.e. the precise form of the "encapsulation")

##### Global secret option

The most likely choice here will be to have a well known secret in the openshift-config
namespace where administrators can store the credentials.
 
The build controller can access that content and mount into build pods as it deems fit.

To mimic 3.x behavior, if the global credential(s) exist, the build controller will always 
mount (and the builder image will always tell buildah to use them).  In other words, they 
are always present.  To add to the old 3.x behavior, we'll define a annotation to set on 
the build config to opt out.  We could also add an option on the global build config object 
to opt out for everyone, but unless we end up supporting non-build entitlement scenarios, this seems 
unnecessary (just don't create the global secret).

The current assumption is that only one set of entitlement credentials is needed at the global level.

##### User Namespace Option

Users could provide secret(s) with a well known annotation in their namespace that the build controller 
can look for and mount in the pod, so the openshift/builder image can instruct buidah to mount.

This would be a slight step up from having to manually cite those secrets in the BuildConfig Spec after creating them,
as documented today. 

Whether we do this one in part may be determined by whether we expect multiple sets of entitlements to be 
used on a given cluster, where a single global copy is not sufficient.

Multiple global secrets, one for ech credential set, where the annotation specifies which global secret to use
for the given build, could also be an alternative here.

The per namespace version would act as an override and take precedence over the global copy.

NOTE: a user namespace option quite possibly will be more attractive to a general tekton implementation, as 
access to the openshift-config namespace is not a lower permission level sort of thing.  In theory, the Build V2
controller may have similar privileges as the current build V1 controller, but we will have to monitor that 
situation with respect to features like this.

##### Host Injected Option

`HostPath` volumes are already called out as a security concern in [volume mounted injections for builds](volume-secrets.md)
 proposal.

The current mindset for this enhancement proposal is to also cite that concern, and only depend on such a 
mechanism if the CRI-O/MCO based solution noted in open questions is available.

A per build config annotation to opt out would be included in such a solution,  If the CRI-O/MCO solugin was an install 
based, day 1 operation, we could have a new field added to the global build config.

NOTE: presumably a host injected option would provide the credentials "for free" for general tekton or Build V2 usage.

NOTE: Host injection via the MCO requires a node reboot for the updates to take effect.  That is generally deemed 
undesirable for this feature.  Also, the MCO API has not been an obvious entryway for users to date when employment
of the existing manual options for entitlements has come up.

###### Current state

1) Currently, auto mount of secrets from the host into the Pods enabled per default. Some people raised concerns about 
this default setting.  Also, RHCOS does not, nor will ever have, the RPM installed by default such that the rhsm.conf
file is present.

2) Disabling auto mount of secrets from the host is under consideration by RHCOS

3) Secrets on the host to provide the creds and rhsm.conf would then be needed.

4) Where again, nobody wants to use the MCO for that.

5) TODO: still searching for clarification on how the "secrets from the host" would be injected, and what they 
would look like exactly.

#### In what form are the credentials provided

There will also be well defined keys within the global secret that account for the various forms:

- set of pem files
- SubscriptionManager config and certs
- Satellite config and instance information, yum repo file if needed, and certs  

#### How does the build consume the credentials

##### Global Secret Option

The build controller copies the global credential secret into the user's namespace, mounts it
into the build pod, and then the builder image running in the build pod tells buildah to mount it.

This is analogous to how build handle registry CAs today.

The secret can either be another of the predetermined ones the build controller creates for every build or 
the build controller can look for an existing secret with a predetermined annotation.
And certainly both approaches can be supported, where the annotated existing secret takes 
precedence over the predetermined build controller secret. 

##### User Namespace Option 1

The build controller would look for secrets with a predefined annotation in the build's namespace,
where the user has injected the credentials in that secret, and mount the secret into the build pod.

The builder image in the pod then seeds buildah with the correct argument to access the mounted content.

##### User Namespace Option 2

The build controller would look for secrets with a predefined annotation in the build's namespace, 
and it would copy the contents from the global location into the secret, and mount the secret into the build pod.

The builder image in the pod then seeds buildah with the correct argument to access the mounted content.

##### Host Injected Option

Assuming again the build controller does not attempt `HostPath` volumes per the [volume mounted injections for builds](volume-secrets.md)
proposal, and assuming the CRI-O feature is available and injects the credential in a well known location, 
the only build related change would be in the builder image, where it supplies buildah the necessary volume arg
to access the credentials.

#### When does the build consume the credentials

##### Global Secret Option

To summarize what was noted above, with the global secret option, the credentials would always be available to the build
if the secret exists.  A per build config annotation to opt out would be provided.  A global opt out (in the global
build config) would only be employed if sufficient non-build scenarios leveraging the global secret arose.

##### Host Injection Option

Same rationale as the global config option.

##### User Namespace Options

For these an annotation is needed to opt in.

###### Pre-Build Auto Injection

If there is a performance concern with copying on each build, then the CA injection controller approach employed
by Global Proxy could be a solution, where that controller handles updating when the source resource is changed.

Today the global proxy support includes auto-injecting the credentials defined at the global level into per namespace 
config maps with a well known label.


### Risks and Mitigations

No additional structural API changes are envisioned for this enhancement.  It may leverage API changes from other sources like 
[volume mounted injections for builds](volume-secrets.md).

Predefined annotations will suffice.

So no new exploit windows from that perspective.

Otherwise, it will be the job of the build controller to ensure that any volume mounts into the build pod are safe from
privileged escalation based wrongdoing if a `HostPath` based solution is employed.

## Design Details

### Test Plan

With both 3.x and 4.x, there have been no automated e2e tests / extended tests out of openshift/origin
for validating consumption of entitled content in OpenShift Builds.  The reason presumably being 
that there is not set of credentials that our CI system can use.

We've revisited this as part of composing this enhancement, and have devised the following recipe, assuming
one of the forms where a secret of some sort is created:
- using the existing build global cluster configuration tests in the serial suite
- do a run where some fake creds are set up in a predetermined secret
- run a docker strategy build that cats the expected mounted secret content 
- search for the cat output in the build logs 

Then there is the question of whether manual testing with actual yum install of entitled content by QE should consider 
with any of the three forms of credentials
- the actual pem files
- SubscriptionManager configuration, including certs
- Satellite instance and configuration, including certs

QE already has existing test cases around using subscription manager with the manual process in play today for 4.x.
They will be adjusted and serve as the minimum for implementations of this proposal.  Most likely this is sufficient,
but due diligence around additional testing for the other scenarios will be assessed during test case review.

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

Alternative to various elements in the implementation proposal are cited in-line in the above section.

## Infrastructure Needed [optional]

QE will need to have actual RHEL subscriptions/entitlements, ideally all 3 forms of input (actual pem's, Subscription
Manager, Satellite), to inject into the system.
