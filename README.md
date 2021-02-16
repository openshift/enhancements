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

## Is My Thing an Enhancement?

A rough heuristic for an enhancement is anything that:

- impacts how a cluster is operated including addition or removal of significant
  capabilities
- impacts upgrade/downgrade
- needs significant effort to complete
- requires consensus/code across multiple domains/repositories
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

Create an enhancement here once you:

- have circulated your idea to see if there is interest
- (optionally) have done a prototype in your own fork
- have identified people who agree to work on and maintain the enhancement
  - many enhancements will take several releases to complete

## Why are Enhancements Tracked

As the project evolves, its important that the OKD community understands how we
build, test, and document our work.  Individually it is hard to understand how
all parts of the system interact, but as a community we can lean on each other
to build the right design and approach before getting too deep into an
implementation.

## When to Comment on an Enhancement Issue

Please comment on the enhancement issue to:
- request a review or clarification on the process
- update status of the enhancement effort
- link to relevant issues in other repos

Please do not comment on the enhancement issue to:
- discuss a detail of the design, code or docs. Use a linked-to-issue or design PR
  for that

## Using Labels

The following labels may be applied to enhancements to help categorize them:

- `priority/important-soon` indicates that the enhancement is related to a
top level release priority. These will be highlighted in the
[this-week](this-week/) newsletters.
