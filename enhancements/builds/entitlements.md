---
title: openshift-builds-improve-use-of-rhel-entitlements

authors:
  - "@gabemontero"
  - "@bparees" (provided a lot of input in tracking Jira)

reviewers:
  - "@bparees"
  - "@adambkaplan"
  - "@who else?"
  
approvers:
  - "@bparees"
  - "@adambkaplan"
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
>enhancement.  Presumably the MCO would need updating to leverage this CRI-O feature.
> 2. There has been discussion elsewhere around having a more generic notion of "global resources" that any pod can
>mount.  Would we want this enhancement to tackle that (probably "no")?  Would we gate this enhancement on the 
>availability of such a feature (probably "no")?  Upstream feature work is implied with such an item.
> 3. Do we need to allow for multiple sets of entitlement credentials to be used by Builds in 4.x?  It does not appear 
>that was the case in 3.x.

## Summary

So in V3.x, all OpenShift Builds essentially had the customer RHEL entitlements mounted into the build pod automatically
and without further intervention after install "for free" because:

 - the ansible based installer required/forced providing your subscription information in order to install OCP
 - for reference, see [these docs](https://docs.openshift.com/container-platform/3.11/install/host_preparation.html#host-registration) 
 and [these roles](https://github.com/openshift/openshift-ansible/blob/release-3.11/roles/rhel_subscribe/tasks/main.yml)
 - next, Red Hats's docker daemon installed in 3.x had specific logic that auto-mounted the host's subscriptions 
 into the container

In V4.x, with 

- the removal of the docker daemon
- and the total replacement of the openshift ansible based installer

all those 3.x mechanisms are gone, and a series of steps from either the cluster admin and/or developer attempting
to run OpenShift Builds leveraging entitled content must be performed after install to enable access to the entitled
content during the build.

The highlights of those steps from [the current 4.x doc](https://docs.openshift.com/container-platform/4.3/builds/running-entitled-builds.html):

1) First, either
 
- create an ImageStreamTag to the UBI image from `registry.redhat.io` in the `openshift` namespace, or 
- reference the UBI image directly and include a pull secret to `registry.redhat.io` in your BuildConfig

2) Next, based on your access to the credentials:

- With direct access to the entitlement pem files, create a generic secret from the files and reference the secret in
your BuildConfig's input/source secret list
- With Subscription Manager, and S2I build strategy, create config maps for the Subscription Manager configuration and 
certs and reference the config maps in your BuildConfig's input/source config map list
- With Subscription Manager, and Docker build strategy, you have to modify your Dockerfile and manually copy and 
initialize as needed the information from the base image
- With Satellite, and S2I build strategy, create a config map to reference the repository config file you created for your
satellite instance and reference the config map in you BuildConfig's input/source config map list
- With Satellite, and Docker build strategy, you have to modify your Dockerfile and manually copy and initialize as 
needed the information from the base image as well as your repository config file for your satellite instance
- And remember to set the skip layers optimization option (i.e. squash layers) with Docker Strategy builds

This enhancement proposal aims to introduce more centralized and automated infrastructure to ease the burden of 
using OpenShift Builds with RHEL subscriptions/entitlements.

## Motivation

Simplify overall usability for using RHEL Subscriptions/Entitlements with OpenShift Builds, as we took a hit in 
4.x vs. 3.x.

### Goals

No longer require (though still allow) manual manipulation of the BuildConfig to consume entitlements.

No longer require (though still allow) manual injection of subscription credentials into a user's namespace.

### Non-Goals

Reach a V3.x level, where no post install activity is needed to consume entitlements.

In other words, no changes to the V4.x installer to allow the user to inject the subscription
particulars.

## Proposal

### User Stories [optional]

No new user stories are introduced here.  We are improving the user's 4.x experience for existing stories.

### Implementation Details/Notes/Constraints [optional]

So the concerns for the implementation center around 4 questions:

- Where do we get the credentials from
- In what form are the credentials provided (pem files, SubscriptionManager, Satellite)
- How does the build consume the credentials
- When does the build consume the credentials (where consuming means telling buildah to mount it)

#### Where do we get the credentials from

##### Current Preferred Option

The most likely choice here will be to have a well known config map in the openshift-config
namespace where administrators can store the credentials.
 
The build controller can access that content and mount into build pods as it deems fit.

The current assumption is that only one set of entitlement credentials is needed at the global level.

##### Alternatives (can provide multiple options if deemed necessary)

###### User Namespace Option

Users could provide secrets/config maps with a well known annotation in their namespace that the build controller 
can look for and mount accordingly.

A slight step up from having to manually cite those secrets/config maps in the BuildConfig Spec after creating them,
as documented today. 

Whether we do this one in part may be determined by whether we expect multiple sets of entitlements to be 
used on a given cluster, where a single global copy is not sufficient.

Multiple global copies could also be an alternative here.

The per namespace version would act as an override and take precedence over the global copy.

###### Host Injected Option

`HostPath` volumes are already called out as a security concern in [volume mounted injections for builds](volume-secrets.md)
 proposal.

The current mindset for this enhancement proposal is to also cite that concern, and only depend on such a 
mechanism if the CRI-O/MCO based solution noted in open questions is available.

#### In what form are the credentials provided

There will also be well defined keys within the global config map that account for the various forms:

- set of pem files
- SubscriptionManager config and certs
- Satellite config and instance information, yum repo file if needed, and certs  

#### How does the build consume the credentials

##### Current Preferred Option

The build controller copies the global credential config map into the user's namespace, mounts it
into the build pod, and then the builder image running in the build pod tells buildah to mount it.

This is analogous to how build handle registry CAs today.

The config map can either be another of the predetermined ones the build controller creates for every build or 
the build controller can look for an existing config map with a predetermined annotation.
And certainly both approaches can be supported, where the annotated existing config map takes 
precedence over the predetermined build controller config map. 

##### Alternatives (can provide multiple options if deemed necessary)

###### User Namespace Option

The build controller would look for config maps with a predefined annotation in the build's namespace,
where the user has injected the credentials in that config map, and mount them into the build pod.

The builder image in the pod then seeds buildah with the correct argument to access the mounted content.

There would be different annotations for copying from a global location into the config map vs. simply
using a config map that the user has set up.

###### Host Injected Option

Assuming again the build controller does not attempt `HostPath` volumes per the [volume mounted injections for builds](volume-secrets.md)
proposal, and assuming the CRI-O feature is available and injects the credential in a well known location, 
the only build related change would be in the builder image, where it supplies buildah the necessary volume arg
to access the credentials.

#### When does the build consume the credentials

##### Current Preferred Option

When the build is initiated, we provide:

- Always if the global config map exists, copying to a predetermined build controller map with an opt-out annotation 
on the Build
- Always if `HostPath` volume exists, with an opt-out annotation on the Build
- Conditionally if the user has annotated a config map with the creds if that option is available
- Conditionally if the user has annotated a config map for copy from the global creds if that options is available

The conditional options would be a feature over and above what was provided in 3.x.

###### Pre-Build Auto Injection Option

There seems no need at this time to employ more sophisticated mechanism's like the global proxy controller
auto-injecting the credentials defined at the global level into per namespace secrets/config maps with 
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

Revisiting that assumption / understanding should be done during the implementation of this enhancement.

If it is now possible / containable, new e2e tests should be added.

Otherwise, manual testing from QE that takes in the three forms of credentials
- the actual pem files
- SubscriptionManager configuration, including certs
- Satellite instance and configuration, including certs

It may not be a given that OpenShift QE has access and/or ability to utilize SubscriptionManager and/or
Satellite.  The status around that should be visited during the test case review when implementing this 
enhancement.

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
