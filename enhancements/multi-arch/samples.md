---
title: sample-imagestream-and-templates-on-non-x86-platforms
authors:
  - "@gabemontero"
reviewers:
  - "@smarterclayton"
  - "@crawford"
  - "@bparees"
  - "@adambkaplan"
approvers:
  - "@smarterclayton"
  - "@crawford"
  - "@bparees"
creation-date: 2019-09-20
last-updated: 2019-09-20
status: implementable
see-also:
replaces:
superseded-by:
---

# Sample Imagestream and Templates on non-x86 Platforms

As of this writing, all samples are provided and owned by non-OpenShift 
Development teams within Red Hat.  And none of those teams 

 - have updated https://github.com/openshift/library with host images on registry.redhat.io or quay.io that support PPC or Z 
 - declare imagestreams in https://github.com/openshift/library that could reference such images

OpenShift is now looking at providing support for two non-x86 architectures in v4.x.

Given that, until a resonable set of samples are provided for those architectures, the samples operator should ensure that it

 - does not install any x86 imagestreams, or templates referencing those imagestreams, on non-x86 systems
 - and by extension, always provides available/non-degraded ClusterOperator statuses during install and upgrade, as this amounts to a no-op on those platforms

And in case the implication is not clear, OpenShift development will not be developing or maintaining 
any non-x86 images for these existing samples.

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

As part of achieving what is described in the opening section, the samples operator needs
to achieve the desired results within the context of how multi-arch is being achieved for 
the entire product.

Alex Crawford and Trevor King have indicated that 
 - clusters would remain homogeneous at least through 4.5/4.6 at the time of this writing
 - use of golang's runtime.GOARCH is sufficient for determining one's architecture

That said, the samples operator config object currently has a []string array defined "Architectures"
in anticipation of heterogeneous clusters.  If that requirement ever gets into plan, the current understanding
after talking to Alex and Trevor are
 - some sort of list of architectures in a cluster would be provided in the infrastructure config
 - k8s styled selectors of some sort will be provided/utilized to pin pod leveraging non-x86 images to the required nodes; concerns for this pinning thus would not fall on the samples operator

## Motivation

Multi-arch has been declared a key initiative for v4.x.

### Goals

Install available samples for the current architecture. Do not install samples for other architectures while we 
only support homogeneous clusters, to avoid currently confusing error messages when an image for one architecture 
is run on a machine with a different architecture.

### Non-Goals

Devex team supplyling existing samples for non-x86 architectures.

## Proposal

The samples operator config object already supports the specification of multiple 
architectures.

It currently requires that only a single architecture is specified, and that 
it is x86_64.

In conjunction, the import of content from https://github.com/openshift/library supports 
downloading multiple architectures, and segregating multiple architectures.  And 
the tagging in https://github.com/openshift/library allows providers to designate which
samples go to which platforms.

So enabling the above to allow PPC and Z content if it is available is item one.

But as previously stated, there is currently no such content, so the samples operator
should do nothing when it comes up in Managed state, and report Available=true, Progressing=false,
Degraded=false.

In the case where there are no samples for an architecture, we'll also update 
the reason/message field of the conditions to indicate there are no samples for the
architecture. 

There are some interesting details and caveats that will be discussed in the 
implementation details section.

### User Stories [optional]

#### Story 1

#### Story 2

### Implementation Details/Notes/Constraints [optional]

In a somewhat related note, there have been a lot of side conversations regarding the Jenkins imagestream
in particular, as the openshift/jenkins images are included in the 4.1/4.2 payloads.

Clayton weighed in with Ben Parees and they concluded that unless/until the openshift/jenkins images 
are fully removed from the install payload (i.e. the devtools team, say as part of the Jenkins operator work, 
somehow facilitates the removal of the openshift/jenkins images from the payload), the x86 openshift/jenkins 
images will be utilized as part of building the non-x86 install payload.

However as no Jenkins imagestream will be installed on non-x86 clusters, the x86 image will not actually be exposed/consumed on those platforms, it will effectively be inert content.

In addition our e2e tests currently heavily rely on imagestreams+templates that are installed by the samples operator.  
If those things are not installed on non-x86 architectures, those e2e tests will fail on those platforms.  At a
minimum those e2e tests must be disabled, but preferrably they need to be refactored to not rely on content from
the samples operator.  A "middle ground" option is to refactor a subset of the tests that gives us a minimal 
level of confidence, though we will not have as much coverage as we have on x86.


### Risks and Mitigations

Unexpected outcomes with respect to the overall multi-arch design.

Unexpected increased priority/requirement to include non-x86 samples.

Ensuring that we do no prevent support for heterogeneous cluster in the future.

## Design Details

### Test Plan

### Graduation Criteria

#### Examples

##### Dev Preview -> Tech Preview

##### Tech Preview -> GA 

##### Removing a deprecated feature

### Upgrade / Downgrade Strategy

No caveats or issues.  Standard, existing approaches apply.

### Version Skew Strategy

No caveats or issues.  Standard, existing approaches apply.

## Implementation History

## Drawbacks

## Alternatives

Remove samples operator as a CVO operator.  Make it a Day 2, Day X operation, where all of the existing samples 
today are installed via OLM Operator Hub.  The selections on the Hub can then explicitly note whether or not they
provide non-x86 options.

This quite possibly is the long term direction for samples operator.  Such a change cannot be contained at this time.
The primary approach proposed here is the "meets minimum" for non-x86 toleration.

Another alternative is to bootstrap the samples operator as "Removed" on non-x86 platforms. But no meaningful advantages 
over installing it as Managed w/ no imagestreams/templates to install.  And if the cluster admin were to subsequently
change to "Managed", the issue of invalid content arises.

## Infrastructure Needed [optional]

New PR e2e jobs running on non-x86, or hybrid topologies, where clear means for the tests to determine the 
architectures employed such that common tests can be run across multiple architectures.
