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

 > 1. CRI-O either already has or will have an option to specify host directories that should always be mounted 
>into containers, akin to what the Red Hat Docker Daemon used to provide with OpenShift V3.  If / when that feature
>is available could have bearing on the implementation options discussed below, or even when to stage in this 
>enhancement.  With current capabilities, the cluster admin would use the MCO post install to effectively update the
>filesystem of each node with the necessary subscription related files in a well known location that CRI-O would look
>for.  There is concern though using this through MCO from a couple of perspectives.  One perspective of concern is 
>multi-cluster management, and scaling out to many clusters.  As such, enabling entitlement, either for builds only or 
>for pods in general, could be policy bits around the multi-cluster set up experience.  Related to that, but also 
>relevant in a single cluster scenario, is around customers having a specific config resource around entitlements, 
>and engaging through that instead of the more generic MCO.  Ultimately, with respect to this proposal, which is currently
>for builds only, there is first a scope question around devising a broader architecture around entitlements, across 
>a broader range of components.  That is either a separate proposal, or a broadening of the scope of this proposal
>from its original intent.  Second, if we manage this with separate proposals, do we gate this one or make it 
>dependent on the broader proposal, or move forward with this one, and simply get agreement on what the order of 
>precedence would be when both a point solution around builds and broader solution around accessing entitlements 
>from any pod. 
> 2. There has been discussion elsewhere around having a more generic notion of "global resources" that any pod can
>mount.  Would we want this enhancement to tackle that (probably "no")?  Would we gate this enhancement on the 
>availability of such a feature (probably "no")?  Upstream feature work is implied with such an item.  If however such
>an upstream features becomes more likely by the time implementation of this proposal arrives, it might dictate our 
>choosing the CRI-O related path vs. building a path around post-install Secrets, as that later path would
>most likely be obviated by the upstream feature.
> 3. We did provide for multiple sets of entitlement credentials in 3.x.  There have been no official, RFE style  
>registered requirements entitlements at a namespace level.  However, we've started to get (slightly) less official
>feedback that customers want this in a more automated sense (vs. the current manual steps).  At a minimum, and is
>noted below when alternatives are discussed, if namespaced entitlements are added with subsequent implementation
>after global entitlements usability is improved, it needs to be seamless, and we minimally need to agree now
>on what the order of precedence will be (with the take on that listed below).
> 4. There has also been a question in the past needing to shard entitlements due to API rate limiting.  To date, it
>has been stated that is not a concern for builds.  Periodic checkpoints here could be prudent.
> 5. There is also the general question of how entitlements are propagated for our managed service offerings?  Reaching 
>out to the appropriate teams there to understand current capabilities/processes and building from there is needed.
> 6. And after 4.5 epic planning it was made clear that a more general solution around entitlements, beyond just builds,
>is desired.  PM has the to-do to investigate, prioritize, and create/assign epics accordingly.  Investigating in the 
>interim around build specific needs via this proposal has been deemed acceptable, but certainly a subsequent, broader
>proposal may necessitate adjustments to this one. 

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

So the concerns for the implementation center around 4 questions:

- Where do we get the credentials from
- In what form are the credentials provided (pem files, SubscriptionManager, Satellite)
- How does the build consume the credentials
- When does the build consume the credentials (where consuming means telling buildah to mount it)

#### Where do we get the credentials from

##### Current Preferred Option

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

##### Alternatives (can provide multiple options if deemed necessary)

###### User Namespace Option

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

###### Host Injected Option

`HostPath` volumes are already called out as a security concern in [volume mounted injections for builds](volume-secrets.md)
 proposal.

The current mindset for this enhancement proposal is to also cite that concern, and only depend on such a 
mechanism if the CRI-O/MCO based solution noted in open questions is available.

A per build config annotation to opt out would be included in such a solution,  If the CRI-O/MCO solugin was an install 
based, day 1 operation, we could have a new field added to the global build config.

NOTE: presumably a host injected option would provide the credentials "for free" for general tekton or Build V2 usage.  

#### In what form are the credentials provided

There will also be well defined keys within the global secret that account for the various forms:

- set of pem files
- SubscriptionManager config and certs
- Satellite config and instance information, yum repo file if needed, and certs  

#### How does the build consume the credentials

##### Current Preferred Option

The build controller copies the global credential secret into the user's namespace, mounts it
into the build pod, and then the builder image running in the build pod tells buildah to mount it.

This is analogous to how build handle registry CAs today.

The secret can either be another of the predetermined ones the build controller creates for every build or 
the build controller can look for an existing secret with a predetermined annotation.
And certainly both approaches can be supported, where the annotated existing secret takes 
precedence over the predetermined build controller secret. 

##### Alternatives (can provide multiple options if deemed necessary)

###### User Namespace Option 1

The build controller would look for secrets with a predefined annotation in the build's namespace,
where the user has injected the credentials in that secret, and mount the secret into the build pod.

The builder image in the pod then seeds buildah with the correct argument to access the mounted content.

###### User Namespace Option 2

The build controller would look for secrets with a predefined annotation in the build's namespace, 
and it would copy the contents from the global location into the secret, and mount the secret into the build pod.

The builder image in the pod then seeds buildah with the correct argument to access the mounted content.

###### Host Injected Option

Assuming again the build controller does not attempt `HostPath` volumes per the [volume mounted injections for builds](volume-secrets.md)
proposal, and assuming the CRI-O feature is available and injects the credential in a well known location, 
the only build related change would be in the builder image, where it supplies buildah the necessary volume arg
to access the credentials.

#### When does the build consume the credentials

##### Current Preferred Option

To summarize what was noted above, with the global secret option, the credentials would always be available to the build
if the secret exists.  A per build config annotation to opt out would be provided.  A global opt out (in the global
build config) would only be employed if sufficient non-build scenarios leveraging the global secret arose.

##### Host Injection Option

Same rationale as the global config option.

##### User Namespace Options

For these an annotation is needed to opt in.

###### Pre-Build Auto Injection Option

There seems no need at this time to employ more sophisticated mechanism's like the global proxy controller
auto-injecting the credentials defined at the global level into per namespace secrets/secrets with 
a well known label.

Copying when a build is initiated seems sufficient.

If there is a performance concern with copying on each build, then CA injection controller approach employed
by Global Proxy could be a solution, where that controller handles updating when the source resource is changed.

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
