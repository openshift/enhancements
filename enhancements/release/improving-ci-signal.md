---
title: improving-ci-signal
authors:
  - "@deads2k"
  - "@stbenjam"
reviewers:
  - "@dgoodwin"
  - "@stbenjam"
  - "@wking"
approvers:
  - "@deads2k"
api-approvers:
  - "@deads2k"
tracking-link:
  - "https://docs.google.com/document/d/16E0dLFLbLBTe0J4fUd_55I-8bJc9t22BwsdWqFuutaQ/edit"
presentation-links:
  - "https://drive.google.com/file/d/1JFe4Lu6pIW-LVCulaVhq8WjnLzz62SZt/view?usp=sharing"
  - "https://drive.google.com/file/d/1dREKNv_VqGOiKAnRb1QDUPxteVdZVkjh/view?usp=sharing"
  - "https://docs.google.com/presentation/d/1YFRNeJm-bNiXZYZubUVgFypOhdbryZtIoD6lYWHI6-0/edit?usp=sharing"
creation-date: 2021-08-16
last-updated: 2022-07-18
---

# Improving CI Signal

We are proposing to get more aggressive in detecting payload health rejections and using PR reverts to restore payload
health to allow us to keep the organization as a whole moving forward toward a successful release. See below for the
details of why we propose to do this and how we propose to do it.

## Summary

We are currently highly susceptible to regression in the quality of our payloads. This is due to a few factors:

- We can’t (too much time, too expensive) run all our CI jobs against all PRs pre-merge, so sometimes we don’t find out
  something regressed until after the PR merges

- Even our payload acceptance jobs assume that they will sometimes fail and therefore set the bar at “at least one
  pass”. This allows changes which make tests less reliable to go undetected as long as we keep getting an occasional
  pass.

## Motivation

Undetected, or detected but unresolved regressions in payload health have a number of impacts

- The longer it takes us to detect it, the harder it is to determine which change introduced the issue because there are
  more changes to sift through

- Unhealthy payloads can impact the merge rate of PRs across the org as they cause more test failures and generate
  distraction for engineers trying to determine if their PR has an actual problem Consumers of bleeding edge payloads (
  QE, developers, partners, field teams) can’t get access to the latest code/features if we don’t have recently accepted
  payloads, so anything that causes more payload rejection to occur impacts our ability to test+verify other unrelated
  changes

- Unhealthy CI data reduces confidence in test results. Test failures become non-actionable when they become normalized.

- Because all tests must pass in the same job run, we are very limited in which tests we can treat as gating (the tests
  we gate on must be highly reliable).

### Goals

- Faster detection when we regress the health of our payload

- Faster reaction to regressions (reverting a change to provide more time to debug)

- Ability to add more semi-reliable tests to our gating bucket and slowly ratchet up their pass rate while ensuring they
  do not regress further

- Provides a mechanism for teams to pre-merge test their PRs for payload regressions

- Fewer changes to sift through when we do have a regression

- Reduced time to restore payload health (at the cost of more frequently reverting previously merged changes)

- Instead of requiring all tests to pass in the same single run of a job, we can pass as long as each individual test
  passes a sufficient number of times across all the runs of the job This will also allow us to grow the number of CI
  jobs+tests we use for payload gating

### Non-Goals

## Proposal

- Instead of accepting payloads based on a “pass all tests once” gate, accept payloads based on running all tests N
  times and ensuring that, statistically, the observed pass percentage of each test is unlikely to have significantly
  regressed relative to our historical pass rate for each test, unless there is an actual regression present.
- Enable that same “run tests N times and pass based on observed pass percentage relative to historical pass rate” check
  to be performed against PRs (pre-merge), though they will not be run automatically or required for merge.

Upon detection of a pass rate regression (As observed via attempted payload acceptance):

- Try to find the PR that introduced the regression by opening revert PRs for all PRs that went in since the last good
  payload and running the payload acceptance check against each revert-PR. The PR that passes the check will tell us
  which change was the source of the regression
- While waiting for test results, contact all teams (via bot PR comments) who own one of the PRs in question to make
  them aware their PR may be the source of a regression so they can begin investigating
- If at the time we have identified the source of the regression (the PR for which a revert resolves the issue), the
  team in question does not have an identified fix in hand, we will work with the team to merge the revert PR in order
  to restore payload health, and the team will be responsible for landing the un-revert along w/ their fix.
- Un-reverts and fixes will need to pass the same payload acceptance checks before being merged.

## Implementation Details/Notes/Constraints

### Current State

Our current interpretation of CI results says that we want every payload blocking job run to pass at least once, when we
are trying to accept a new payload. A blocking job can be run more than once in order to achieve this.

The combination of these qualities has led to a large number of informing jobs that cannot be efficiently promoted into
blocking jobs because the likelihood of a payload being rejected because a single TestRun failed on a single platform
goes up the more tests you run.

We are spending a significant amount of money and effort on JobRuns which cannot block payload promotion in their
current state and thus cannot act as backpressure against product regressions.

### Job aggregation

Provide a path for imperfect informing jobs to become blocking jobs by changing how we measure success. Change the
threshold from the existing, “be nearly perfect and pass every test at the same time”, to reflect the reality of where
we are and become “don’t make a [Job,Test] tuple’s pass rate significantly worse than the average over the last two
weeks”, regardless of whether that Job/Test is currently reliable or unreliable.

#### Why aggregation?

This provides two novel capabilities:

1. Payload informing jobs have a path to become payload blocking jobs before the jobs become nearly perfect. This
   matches the reality of what we have in terms of most of our Jobs and provides a path to ratcheting up reliability for
   a platform. Having a payload job become blocking means that it will no longer be able to silently regress.

2. We can much more quickly discover when the product regresses. Today, it takes several days to develop enough signal
   to realize that we have regressed a platform, job, or test. This change in threshold pre-supposes that we have found
   a way to make that pass rate determination. Our goal is to detect regressions within a single payload promotion run.

#### How do we aggregate?

The approach requires several novel pieces. We need enough JobRun,TestRun tuples to develop a meaningful signal. We need
at least 10 JobRuns per Job to reliably (95% accuracy) detect a 30% drop in pass rate. See appendix for math-y chart.

That change in pass rate seems quite high, but given the current “pass at least once” criteria, we actually regress to
that degree fairly often. The significant drops like this are the sort of things that jam up our merge queue.

We need a JobAggregator to aggregate the JobRun,TestRun tuples into a Job,Test pass percentage.

We need a way to track historical pass rates for a Job,Test tuple in a queryable way so that the job aggregator can
determine pass/fail criteria.

Once you have those two things, it becomes possible to run a job enough times on a single payload and summarize those
results in a JobFooAggregate Job that can pass or fail based on historical criteria.

#### Ok, What then? We’re already busted

Once we know we have a broken payload, TRT must revert the breaking change. Regardless if a likely fix is identified,
the revert must land first. A clean revert is nearly always guaranteed to fix the problem immediately, while a proposed
fix PR is not. Additionally, it is a negligible amount of additional effort to build your fix on top of unreverting the
revert.

### Quick Revert

TRT is expected to watch payload promotion for the main branch (not historical releases). Given a broken payload that
TRT detected quickly, quickly and definitively identify the regressing change. Once identified, merge an immediate
revert. The original author and/or the author's team lead should be given notice and criteria to reintroduce their
change.

#### Why revert?

Blocking payload promotion for an extended period of time has impacts across a large org. Since the payload is made up
of multiple, largely independent pieces, we don’t want a regression in one component to block accepting changes from
other components.

The choices are either

1. Leave the main branch broken during debugging, PR authoring, PR review, PR testing to prove it fixes it, PR merging.
   During this time, we would have no new payloads for QE and degraded/slow ability to merge new code for anything in
   the payload

2. Revert quickly. Debugging happens in the unrevert PR and the PR authoring, review, testing, and merge are still on
   the fix PR. During this time, we would have new payloads for QE and normal merging for the rest of the org.

Option 1 blocks hundreds of people (developers, QE, downstream teams) by leaving broken code in the main branch. Option
2 unblocks hundreds of people (everyone except the reverted repo) and the work for the reverted repo is identical.

We should be biased towards reversion.

Because our org is distributed amongst multiple repositories, it is relatively easy to revert a single piece of
functionality without interdependencies. No one actually has to diagnose the failure in order to revert the regressing
change. And only a repo is impacted by the revert.

#### How should I revert?

Mechanically, a person who notices the payload is blocked can...

1. Find all the changes made to the payload. The set is small for single payloads.
2. Manually open reverts of the PRs that have changed.
3. /hold the revert PRs and manually request payload promotion jobs on each of the revert PRs.
4. If one of the revert PRs shows the payload promotion problem is fixed, the revert can be immediately merged.

#### So I’m reverted, but my functionality is important, how do I come back in?

We don’t revert just for fun. The PR that was reverted is important to someone for some reason, and the decision to
revert is not expressing a view that the change is not worthwhile. In order to be merged back in:

1. Open the unrevert+fix PR
2. Manually /hold the unrevert+fix
3. Request the payload promotion jobs by running the `/payload` command
4. Demonstrate that the payload promotion jobs passed

The approach requires using new per-PR capabilities.

1. We need a way to run all payload promotion jobs against a single, manually chosen PR and visualize the results on a
   separate page. It needs to be separate due to the large quantity of jobs running against it. These jobs won’t be run
   proactively on PRs, they will be available to be run on-demand either in advance of merging a risky change, or
   retroactively when trying to identify which merged PR may have introduced a regression.

2. We do NOT need a way to auto-prevent a PR from merging based on the result of these promotion jobs running. This is
   exceptional enough that a manual /hold on a PR and manual inspection of the summary is sufficient for us to start
   trying to use the system.

The payload promotion jobs from 3 also provide a fairly precise signal for someone to decide to intentionally regress
our product’s stability in order to merge a feature. I’m not saying it’s a thing that I would encourage, but it is a
thing that can be done if someone wants to stand up and say, “this PR is important enough to regress this Job,Test tuple
by X%”

## Design Details

These are the things that we can start doing to achieve these objectives. Most of these actions have independent value.

### API Extensions

None

### User Stories

#### Run the same Job multiple times on a single payload

POC done!

This is necessary to gain pass/fail percentages on a single payload. The feature was added a couple weeks ago and we’ve
been using it to test the JobAggregator. Thanks to Brad Williams for his quick work here.

Immediate value: increased signal for TRT on the job we enable this on.

#### Track historical test run data in queryable way

In progress in openshift/ci-tools#2166.

The data gathering, upload, and summarizing components are present. They are pushing data to BigQuery in project:
openshift-ci-data-analysis, dataset: ci_data. Right now only a subset of jobs are presented, but you can view a report.

Immediate value: ability to manually inspect Job,Test reliability. We used this to detect a kubernetes update regression
even in its current state.

#### Endpoint availability needs special handling

An additional value-add is the ability to detect per-endpoint, per-platform availability numbers for API and ingress
endpoints. The information is now present, but some bugs in calculation need addressing first: openshift/origin#26373.

#### Build a JobAggregator binary

In progress in openshift/ci-tools#2166, but may split off.

The data gathering is the same as the historical data tracking. The summarization code for a given JobRun is complete.
The historical data querying is not yet complete.

Immediate value: ability to manually inspect PR Job,Test reliability. We used a partially working version of this to
catch a kubernetes update regression in its current state.

#### Add payload promotion blocking job for JobAggregator

Blocked by Building a JobAggregator binary.

Immediate value: the signal on the jobs we apply against this against becomes more reliable and faster even if we do not
make the per PR payload promotion tests work. We’ll be better at detection, but we will be no better at reacting.

#### Run any payload promotion job against a PR in any repo

WIP in openshift/release#20463

This is a prerequisite for being able to run all the payload promotion jobs. I think there are multiple PRs in progress
to bring this home. I’ve heard that it conceptually works, but isn’t yet complete.

Immediate value: a change to any repo can test any job. This commonly comes up for openshift/kubernetes,
openshift/origin, openshift/kube-*-operators, openshift/cluster-etcd-operator. It occasionally comes up for other repos,
but I’m not an approver on those, so I don’t see all the one-offs.

#### Run all payload promotion jobs against a PR

Blocked by running any payload promotion job against a PR in any repo.

There will almost certainly be some fitting required to replicate the release-controller multi-execute behavior and to
the JobAggregator to locate related JobRuns in GCS. The information is present (humans can do this), but we have to find
a way to codify it.

Immediate value: risky changes can be manually checked. Some changes are inherently risky and those merging know it. The
kubernetes update is a good example, but there are others as new features are brought online and may impact esoteric
platforms.

### Risks and Mitigations

N/A

### Test Plan

N/A

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

N/A

#### Tech Preview -> GA

N/A

#### Removing a deprecated feature

N/A

### Upgrade / Downgrade Strategy

N/A

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

\#forum-release-oversight on Slack

### Questions

#### Who is watching for payload regressions and opening reverts?

For now, the Technical Release Team will be watching payload results on the current release branch. They will open the
revert PRs and engage the involved teams (any team that contributed a PR to the payload that regressed).

#### How will we be contacted if our PR is part of a regressed payload?

TRT will ping the relevant teams’ slack aliases in #forum-release-oversight.

#### Who approves the revert for merge?

In order to ensure they are included in the conversation/resolution, we would like the team that delivered the original
PR to /approve the reversion, however if they are unavailable or unresponsive this may be escalated to the
staff-engineering team to ensure we do not remain in a regressed state longer than necessary.

#### What happens if a giant change like a rebase with high business value and on the critical path of the release regresses the pass rate? Will it be reverted? Will we work on it until it passes good enough? What if the kubelet build hours or days later causes the regression?

Since we have a way to run the payload promotion checks before a PR merges, I encourage high risk changes to run the
payload acceptance before they merge. If that payload acceptance test fails, then it is 95% likely (math, not gut) that
the PR is reducing our reliability. That signal is enough to engage other teams if necessary, but since most repos other
than openshift/kubernetes are owned by a single team and that team is the local expert, in the majority of cases the
expert should be local.

If the PR isn’t pre-checked and we catch it during payload promotion, then even a high business value PR is subject to
reversion. The unrevert PR is the place to talk about expected and observed impact to reliability versus the new
feature. Both the revert and the unrevert are human processes, so there is a place for a director and/or staff engineer
to directly comment in a PR and say, “this feature makes our product X% less reliable and that will impact every team,
QE, partner, and customer, but this feature is so important that we will knowingly impact all of them to have feature X”
and it can merge.

#### How can we possibly identify a fix before our PR is reverted? Can’t you just give us more time?

Given the timing of these events, it’s understandable that many teams aren’t going to bother investigating if it was
their PR that caused the issue until there is definitive proof (in the form of a revert PR that passes the check). And
even if they do immediately investigate and open a fix PR, they will be racing against the checks being run against the
revert PRs. 

Remember, anytime our payloads are regressed, the entire org is being impacted. While there may be a small cost to a
single team to land an un-revert, it avoids a greater cost to the org as a whole. We want to get back to green as
quickly as possible and avoid a slippery slope of a team wanting to try “one more fix” before we revert. Teams can also
optionally run the acceptance checks on their original PR before merging it, to reduce the risk that the PR will have to
be reverted.

#### What if these new checks are wrong?

We will need to carefully study the outcomes of this process to ensure that we are getting value from it (finding actual
regressions) and not causing unnecessarily churn (raising revert PRs due to false positives when nothing has actually
regressed, or the regression actually happened in an earlier payload but went undetected at the time). TRT will track
data on how many times this reversion process gets triggered and the outcomes of each incident and then do a
retrospective after the 4.10 release.

#### What if cloud X has a systemic failure causing tests to fail?

That sort of failure will show up as, "I opened all the reverts and none of them pass". The relative frequency of this
sort of failure is difficult to predict in advance because our current signal lacks sufficient granularity, but it
doesn't appear to be the most common case so far.

#### What if you revert my feature PR after feature freeze?

We will make an exception to the feature freeze requirements to allow un-reverts(along w/ the necessary fix) to land
post feature freeze, even though technically the unrevert is (re)merging a feature PR. Alternatively, consider moving
your team to the no-feature-freeze process!

#### When will we start doing this?

The capabilities to perform these checks (run jobs multiple times, aggregate test results, gate acceptance, run this
check on PRs, etc) are still being built. We are communicating this now to get feedback on the proposed changes.
Additional communication will be sent when we’re ready to turn on this functionality.

## Implementation History

- Job aggregation has been implemented
- TRT revert policy has been implemented

### Drawbacks

N/A

## Alternatives

N/A

## Appendix

### Pass/Fail rates for running jobs 10 times

Using something called Fischer’s Exact Probability Test (
source https://www.itl.nist.gov/div898/handbook/prc/section3/prc33.htm), it is possible to know how whether a payload
being tested is better or worse than the payload(s) which produced a larger dataset. This is the mathy way of finding
the statistical thresholds for how many failed testruns should be a failure. Thanks to SteveK for knowing where to look
and for building the code to make this chart.

The lines in the lower right are the number of passes (or less) out of a sample size of 10, required to be 95% sure that
the payload being tested is worse than the payload before. We can become more certain by running more iterations per
payload or by reducing the threshold for passing. I suspect that reducing thresholds will be enough.

The lines in the upper left are the number of passes (or more) required to be 95% sure that the payload being tested is
better than the payload before. That could be used for faster ratcheting, but for the short term will not be used.

Our corpus/history (n) will be well over 250, so use that line. Our sample size is 10. Find the corpus pass percentage
of the job,test tuple. Then go straight up and find the Y value. That value is the pass percent required to be 95% sure
that the current payload is worse than the corpus (history).

This graph shows us that if the corpus (history) is passing a job,test tuple at 95%, then a payload that only passed
7/10 attempts has a 95% chance of having regressed us.

### Vocabulary

We often refer to these concepts with an imprecise vernacular, let’s try to codify a couple to make what follows easier
to understand.

1. Job -- example: periodic-ci-openshift-release-master-ci-4.9-e2e-gcp-upgrade. Jobs are a description of what set of
   the environment a cluster is installed into and which Tests will run and how. Jobs have multiple JobRuns associated
   with them.

2. JobRun -- example periodic-ci-openshift-release-master-ci-4.9-e2e-gcp-upgrade #1423633387024814080. JobRuns are each
   an instance of a Job. They will have N-payloads associated with them (one to install, N to change versions to).
   JobRuns will have multiple TestRuns associated with them. JobRuns pass when all of their TestRuns also succeed.

3. TestRun -- example `[sig-node] pods should never transition back to pending` from
   periodic-ci-openshift-release-master-ci-4.9-e2e-gcp-upgrade #1423633387024814080. A TestRun refers to an exact
   instance of Test for a particular JobRun. At the files-on-disk level, this is represented by a junit-***.xml file
   which lists a particular testCase. Keep in mind that a single JobRun can have multiple TestRuns for a single Test.

4. Test -- example `[sig-node] pods should never transition back to pending`
   A Test is the bit of code that checks if a particular JobRun is functioning correctly. The same Test can be used by
   many jobs and the overall pass/fail rates can provide information about the different environments set up by other
   jobs.

5. Payload Blocking Job -- click an instance of 4.9-nightly. About 4 jobs. A job that is run on prospective payloads. If
   the JobRun does not succeed, the payload is not promoted.

6. Payload Informing Job -- click an instance of 4.9-nightly. About 34 jobs. A job that is run on prospective payloads.
   If the Job run does not succeed, nothing happens: the payload is promoted anyway.

7. Flake Rate The percentage of the time that a particular TestRun might fail on a given payload, but may succeed if run
   a second time on a different (or sometimes the same) cluster. These are often blamed on environmental conditions that
   result in a cluster-state that impedes success: load, temporary network connectivity failure, other workloads prevent
   test pod scheduling, etc.
