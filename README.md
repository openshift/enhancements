# Enhancements Tracking and Backlog

Enhancement tracking repository for OKD.

Inspired by the [Kubernetes enhancement](https://github.com/kubernetes/enhancements) process.

This repository provides a rally point to discuss, debate, and reach consensus
for how OKD [enhancements](./enhancements) are introduced.  OKD combines
Kubernetes container orchestration services with a broad set of ecosystem
components in order to provide an enterprise ready Kubernetes distribution built
for extension.  OKD assembles innovation across a wide array of repositories and
upstream communities.  Given the breadth of the distribution, it is useful to
have a centralized place to describe OKD enhancements via an actionable design
proposal.

Enhancements may take multiple releases to ultimately complete and thus provide
the basis of a community roadmap.  Enhancements may be filed from anyone in the
community, but require consensus from domain specific project maintainers in
order to implement and accept into the release.

For an overview of the whole project, see [the roadmap](ROADMAP.md).

For a quick-start, FAQ, and template references, see [the guidelines](guidelines/README.md).

## Why are Enhancements Tracked?

As the project evolves, its important that the OKD community understands how we
build, test, and document our work.  Individually it is hard to understand how
all parts of the system interact, but as a community we can lean on each other
to build the right design and approach before getting too deep into an
implementation.

## Is My Thing an Enhancement?

A rough heuristic for an enhancement is anything that:

- impacts how a cluster is operated including addition or removal of significant
  capabilities
- impacts upgrade/downgrade
- needs significant effort to complete
- requires consensus/code across multiple domains/repositories
- proposes adding a new user-facing component
- has phases of maturity (Dev Preview, Tech Preview, GA)
- demands formal documentation to utilize

It is unlikely to require an enhancement if it:

- fixes a bug
- adds more testing
- internally refactors a code or component only visible to that components
  domain
- minimal impact to distribution as a whole

If you are not sure if the proposed work requires an enhancement, file an issue
and ask!

## When to Create a New Enhancement

Enhancements should be related to work to be implemented in the near
future. If you have an idea, but aren't planning to implement it right
away, the conversation should start somewhere else like the mailing
list or Slack.

Create an enhancement here once you:

- have circulated your idea to see if there is interest
- (optionally) have done a prototype in your own fork
- have identified people who agree to work on and maintain the enhancement
  - many enhancements will take several releases to complete

## How are Enhancements Reviewed and Approved?

The author of an enhancement is responsible for managing it through
the review process, including soliciting feedback on the pull request
and in meetings, if necessary.

Each enhancement should have at least one "approver" and several
reviewers designated in the header of the document.

The approver assists authors who may not be familiar with the process,
the project, or the maintainers. They may provide advice about who
should review a specific proposal and point out deadlines or other
time-based criteria for completing work. The approver is responsible
for recognizing when consensus has been reached so that a proposal is
ready to be approved, or formally rejected. In cases where consensus
is not emerging on its own, the approver may also step in as a
mediator. The approver do not need to be a subject-matter expert for
the subject of the design, although it can help if they are.

Choosing the appropriate approver depends on the scope of an
enhancement. If it is limited in scope to a given team or component,
then a peer or lead on that team or pillar is appropriate.  If an
enhancement captures something more broad in scope, then a member of
the OpenShift staff engineers team or someone they delegate would be
appropriate.  Examples of broad scope are proposals that change the
definition of OpenShift in some way, add a new required dependency, or
change the way customers are supported.  Use your best judgement to
determine the level of approval needed.  If you’re not sure, ask a
staff engineer to help find a good approver by posting in
`#forum-arch` on the CoreOS Slack server and tagging
`@aos-staff-engineers`.

The set of reviewers for an enhancement proposal can be anyone that
has an interest in this work or the expertise to provide a useful
input/assessment.  At a minimum the reviewers should include a
representative of any team that will need to do work for this EP, or
whose team will own/support the resulting implementation. Be mindful
of the workload of reviewers, however, and the challenge of finding
consensus as the group of reviewers grows larger. Clearly indicating
what aspect of the EP you expect each reviewer to be concerned with
will allow them to focus their reviews.

## How Can an Author Help Speed Up the Review Process?

Enhancements should have agreement from all stakeholders prior to
being approved and merged. Reviews are not time-boxed (see Life-cycle
below). We manage the rate of churn in OKD by asking component
maintainers to act as reviewers in addition to everything else that
they do.  If it is not possible to attract the attention of enough of
the right maintainers to act as reviewers, that is a signal that the
project's rate of change is maxed out. With that said, there are a few
things that authors can do to help keep the conversation moving along.

1. Respond to comments quickly, so that a reviewer can tell you are
   engaged.
2. Push update patches, rather than force-pushing a replacement, to
   make it easier for reviewers to see what you have changed. Use
   descriptive commit messages on those updates, or plan to use
   `/label tide/merge-method-squash` to have them squashed when the
   pull request merges.
3. Do not rely solely on the enhancement for visibility of the
   proposal. For high priority work, or if the conversation stalls
   out, you can start a thread in `#forum-arch` on the CoreOS Slack
   server or bring the enhancement to one of the weekly architecture
   review meetings for discussion. If you aren't sure which meeting to
   use, work with a staff engineer to find a good fit.
4. If the conversation otherwise seems stuck, pinging reviewers on
   Slack can be used to remind them to look at updates. It's generally
   appropriate to give people at least a business day or two to
   respond in the GitHub thread first, before reaching out to them
   directly on Slack, so that they can manage their work queue and
   disruptions.

## When to Comment on an Enhancement Issue

Please comment on the enhancement issue to:
- request a review or clarification on the process
- update status of the enhancement effort
- link to relevant issues in other repos

Please do not comment on the enhancement issue to:
- discuss a detail of the design, code or docs. Use a linked-to-issue
  or design pull request for that

## Using Labels

The following labels may be applied to enhancements to help categorize them:

- `priority/important-soon` indicates that the enhancement is related to a
top level release priority. These will be highlighted in the
[this-week](this-week/) newsletters.

## Life-cycle

Pull requests to this repository should be short-lived and merged as
soon as there is consensus. Therefore, the normal life-cycle timeouts
are shorter than for most of our code repositories.

Pull requests being actively discussed will stay open
indefinitely. Inactive pull requests will automatically have the
`life-cycle/stale` label applied after 28 days. Removing the
life-cycle label will reset the clock. After 7 days, stale pull
requests are updated to `life-cycle/rotten`. After another 7 days,
rotten pull requests are closed.

Ideally pull requests with enhancement proposals will be merged before
significant coding work begins, since this avoids having to rework the
implementation if the design changes as well as arguing in favor of
accepting a design simply because it is already implemented.

## Template Updates

From time to time the template for enhancement proposals is modified
as we refine our processes. When that happens, open pull requests may
start failing the linter job that ensures that all documents include
the required sections.

If you are working on an enhancement and the linter job fails because
of changes to the template (not other issues with the markdown
formatting), handle it based on the maturity of the enhancement pull
request:

* If the only reason to update your pull request is to make the linter job
  accept it after a template change and there are no substantive
  content changes needed for approval, override the job to allow the
  pull request to merge.
* If your enhancement is still a draft, and consensus hasn't been
  reached, modify the pull request so the new enhancement matches the updated
  template.
* If you are updating an existing (merged) document, go ahead and
  override the job.
