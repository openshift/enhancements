---
title: Release Blocker Bug Definition
authors:
  - "@mffiedler"
  - "@dgoodwin"
creation-date: 2026-01-20
last-updated: 2026-01-23
status: informational
---

# Release Blocker Bug Definition

## The “Why” behind Blocker bugs

Release blocker `Approved` is applied to bugs which fall below the minimum bar of quality we’re willing to release to customers and/or tolerate in our CI system.

## Determining if a Bug is an X.Y.0 Release Blocker

* When the `Release Blocker` field in an OCPBUG is set to `Approved` (see the _Planning_ tab).

### When to set Release Blocker to `Approved` in an X.Y.0 OCPBUG

When preparing a new X.Y release shipping exciting features, we have the ability to delay the GA X.Y.0 until we are sufficiently comfortable with quality.
There are effectively two categories of bugs where we use the Release Blocker flag:

#### Component Readiness Regressions

All component readiness regressions are treated as release blockers, the issue must be fixed or granted an exception via an [SBAR](https://docs.google.com/document/d/1-Lq4p7KhHRUFhkhpZ1ntDOcvDZgj9YVIBOmLSRlNkq0/edit?usp=sharing) approved by the OpenShift leadership team. Component Readiness attempts to be a forgiving system to smooth out the noise of CI for a complex product like OpenShift, but once an issue has crossed that threshold, it must be fixed. With the vast amount of tests, configurations and platforms we test against, clearing sufficiently unreliable issues out of the system is critical for those who monitor it to make sense of whether we're healthy or not. At the scale we operate, unresolved issues will pile up quickly effectively making it impossible for us to monitor health.

The SBAR process is both a safety mechanism ensuring problems are understood and safe to ship, as well as a request for permission to leave a problem active in the CI system where others will have to deal with the signal it generates.

#### Sufficiently Severe Product Bugs

Based on the criteria in the list below, some bugs are deemed too severe to release even in a .0. This implies that fixing the bug in the ".0" release is more important than meeting our internal and external (partner/customer) commitments for the release.   When considering whether a bug is a Blocker, teams should consider if we can ship it in an early z-stream release, and weigh it against the commitment we make when we set a release date.

The criteria below apply to .0/y-stream/pre-GA releases.   The Release Blocker field in OCPBUGS has a different meaning for z-stream releases.

In general we would not require an SBAR to remove release blocker for a non-component readiness bug, it would just be a bug we've deemed safe to ship and thus not a Release Blocker.

#### Release Blocker Approved conditions for an X.Y.0 OCPBUG

* **Most bugs for new features are NOT release blockers** if they do not regress functionality.  Exceptions would be bugs in release blocking features which meet the criteria below.

* All bugs related to Component Readiness regressions unless covered by approved exceptions, even tech-preview jobs. For more details on why, see the introduction and the FAQ below.  
* Most  bugs that result in Data Loss, Service Unavailability, or Data Corruption are blockers.  
* Bugs that cause failed installs and upgrades may be release blockers based on the scope of the failure.  If the failure is limited to a specific form-factor/platform, consider having a conversation with the relevant stakeholders. More on this below.  
* Bugs which cause the perception of a failed upgrade may be release blockers  
* Most bugs which are a regression are blockers, such as regressions discovered by Layered Product Testing or regressions in functionality not otherwise detected by Component Readiness.  
* No bugs with a severity lower than Important are considered blockers.  
*  UpgradeBlocker label indicates the bug is being assessed for the declaration of a conditional update risk.  The bug may or may not be a ReleaseBlocker after the assessment.  
* Most bugs with the ServiceDeliveryBlocker label are blockers.  
* Cluster CPU usage for Single Node OCP exceeds 2 physical cores and 4 hyperthreads  
* Bugs which severely impact Service Delivery usage are blockers.  
  * An SD blocker includes any regression or bug in a feature that is default for the ROSA, OSD, or ARO fleet where there is no acceptable workaround.  An acceptable workaround is defined as:  
    * workaround is idempotent meaning it can be applied repeatedly to a cluster that has already had the workaround applied, without a resulting change.  
    * workaround can be safely deployed at scale (1000's of clusters) without material risk via automation  
    * SD can implement the workaround before the release is pushed to any more Cincinnati channels (candidate, fast, or stable)  
  * SD has the final say on if a bug is or is not an SD blocker.  In some cases a bug that does not meet the above "acceptable workaround" may be deemed a non-SD blocking bug due to the scope and scale of impact to the ROSA, OSD, and ARO fleet.  This requires SD leadership (VP / Sr. Director) sign-off.

We know there will be edge cases.  When those come up:

* The release team (engineering, documentation, support, service delivery, program management, product management) and the product Sr. Director/VP will make the ultimate decision on whether a bug will delay a release.  
* Engineering teams SHALL use their best judgement in conversation with other stakeholders (PM, QE, CEE, docs, etc) to determine what bugs are blockers.  
* Teams MUST provide justification in Jira if they set a bug in one of the “Most bugs” categories to Release Blocker: Rejected  
* If a workaround is provided as justification for rejecting a blocker, that workaround MUST be safe to apply before updating a cluster so that there's no risk exposure during an upgrade, MUST be documented in release notes, and MUST have plans for ensuring the workaround is safe into the future and/or can automatically be reversed once a final solution is provided.  
* Teams MAY consult with Staff Engineers and managers for guidance on deciding whether or not something should be a blocker.  
* Teams are encouraged to use the Discussion Needed checkboxes on OCPBUGS to have relevant conversations with stakeholders.  The current options for Discussion Needed are:  
  * Architecture Call  
  * Backlog Refinement  
  * Group Conversation  
  * OCP Eng Mgmt  
  * PM Sync  
  * Program Call  
  * Service Delivery Architecture Overview  
  * Stand Up

### When to set Release Blocker to `Approved` in an X.Y.Z OCPBUG

When preparing a new X.Y.Z patch release shipping bug fixes, our ability to delay the release is much more constrained.
If a bug is so terrible that _no cluster_ could possibly benefit from the impacted nightlies, setting `Release Blocker` to `Approved` is appropriate.
But if there are even small subsets of clusters that would not be exposed to the OCPBUG, or if there are steps a cluster-admin could take to mitigate exposure, we prefer releasing the patch release anyway.
This avoids delaying access to other bugfixes in the the clusters that could survive with the OCPBUG being considered.

In some cases, a hot fix is about to merge, or is percolating through nightlies, or is otherwise expected to show up very soon in the release pipeline.
Setting `Release Blocker` to `Approved` in those cases is acceptable as a way to attract ART attention and ask them to wait for that fix.
They may wait for the fix, or they may decide to build a release anyway, even without the fix, depending on capacity and how much time remains for final QE testing before the planned Errata-publication time.

#### Mitigating exposure

There are multiple options for protecting users when a release ships with a known issue.
They include:

* Known issues in the release notes, e.g. [known issues with 4.20][4.20-known-issues].
* [Knowledgebase solutions][kcs].
* [Insights rules][4.20-insights] for the subset of the fleet that enables remote-health reporting.
* [Conditional update risk declarations][4.20-conditional-updates], see [the assessment process](/enhancements/update/update-blocker-lifecycle/README.md) and [the distribution tooling](/enhancements/update/targeted-update-edge-blocking.md).

Both Insights rules and conditional update risk declarations can sometimes be scoped down to the exposed subset of the fleet, to reduce distracting the subset of the fleet that is not exposed.
Both Insights rules and conditional update risk declarations are specific to updating into the risk, and neither provides pre-install warning for users consdering installing an exposed release.

Both known issues, and Knowledgebase solutions are outside the cluster and tooling, and users would need to think to check before updating or installing, and then be successful enough in the check to discover the attempt to call out the issue.
In the event of a failed install, context from the failure like error strings increases the odds that a search would successfully find a known issue or Knowledgebase solution.

# FAQ

## Why are all Component Readiness regressions treated as release blockers?

Component Readiness is our most successful approach to date for maintaining a minimum bar of quality in the product. It flags statistically significant degraded tests when compared to prior releases, while smoothing over less important failures. It also ensures minimum pass rates for new tests we cannot compare to prior releases, and in the future may incorporate broad analysis from other systems such as disruption monitoring and PerfScale reporting.

**Component readiness provides the high level signal that we are safe to ship**. The regressions it detects must be dealt with quickly to restore that view to green so we can make that determination.

Without doing so, the system cannot function. Bugs will pile up untouched, and regressions will accumulate, polluting the system such that we cannot use the tooling to determine if we’re safe to ship.

## What if a bug from Component Readiness is no longer actively regressed?

If a bug was filed for a regression that is now no longer on the board (i.e. it has slipped below the threshold we consider a regression), the answer to whether or not we keep the bug as a release blocker is “it depends”.

Using the “Test Analysis” link from the regression report included with component readiness bugs by default, we can examine if the problem is still occurring in CI at all. If it is, there’s a high likelihood the problem will return, possibly at a very inopportune time. In this case, the bug should remain a release blocker.

If the problem is no longer occurring anywhere, the Release Blocker Approved designation can be safely removed or perhaps the bug closed out entirely. (with supporting evidence)

If we’re near the end of a release, this may also sway the decision in favour of removing release blocker. An SBAR should not be required for an issue that is no longer on the board at GA time.

If unsure please work with the architects, TRT, and quality staff engineers to figure out how to proceed. 

## Why are infrastructure / quota regressions treated as release blockers?

If severe enough, these kinds of problems will surface as regressions in component readiness, typically in the form of regressed install tests. 

While dealing with these can feel laborious and time consuming, we believe it is safest and simplest to maintain a consistent approach to regressions, be they in the product, testing, or CI infrastructure. 

For the macro view of whether we are safe to ship or not, these kinds of issues are indistinguishable from actual install errors unless someone is personally examining every single failure for patterns. We need all three categories resolved in a timely manner as any issue left in the system pollutes the view for everyone using it, requires those monitoring to maintain the mental map of what’s expected and what isn’t, and increases risk that a test has started failing for new reasons.

## Why are new tests being treated as release blockers? 

New tests have no basis in prior releases, so we have to do something to ensure they meet some minimum acceptable standard for a new test coming into the overall system. The 95% pass rate was chosen as a reasonable bar for any new test to hit and prove feature stability.  

## Why is techpreview signal treated as a release blocker?

Techpreview signal is essential for proving new features are ready, backports are safe, conflicts are uncovered, and new changes do regress their functionality. As such we rely on Component Readiness to monitor, and thus we need to keep the regressions list green to be able to sustain monitoring release health and readiness.

## What if a Component Readiness regression cannot be resolved in time for GA?

An SBAR can be submitted to OCP leadership requesting an exception to ship with the issue unfixed outlining why it couldn’t be, and why it’s safe to ship using [this process](https://docs.google.com/document/d/1-Lq4p7KhHRUFhkhpZ1ntDOcvDZgj9YVIBOmLSRlNkq0/edit?usp=sharing).

The **sbar-candidate** label is applied to release blocker bugs as soon as someone believes it’s likely safe to ship the issue without a fix. 

Once an SBAR is approved, Release Blocker can be safely set to Rejected.

## Setting Release Blocker Rejected

Release blocker can be removed if:

1. The issue is no longer occurring at all in CI.  
2. An SBAR has been approved and the bug has a Target Version targetting a z-stream release.
3. The issue is not from component readiness, and has been deemed safe to ship given the above criteria.

[4.20-known-issues]: https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/release_notes/ocp-4-20-release-notes#ocp-release-known-issues_release-notes
[4.20-insights]: https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/support/remote-health-monitoring-with-connected-clusters#using-insights-operator
[4.20-conditional-updates]: https://docs.redhat.com/en/documentation/openshift_container_platform/4.20/html/updating_clusters/understanding-openshift-updates-1#update-evaluate-availability_how-updates-work
[kcs]: https://access.redhat.com/kb/search?document_kinds=Solution&start=0
