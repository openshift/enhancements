---
title: openshift-tests
authors:
  - "@deads2k"
reviewers:
  - "@jianzhangbjz"
  - "@mfojtik"
approvers:
  - "@derekwaynecarr"
  - "@smarterclayton"
  - "@jianzhangbjz"
creation-date: 2020-01-21
last-updated: 2020-01-21
status: implementable
see-also:
replaces:
superseded-by:
---

# OpenShift Tests

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Separate openshift-tests into "tests from kube" and "tests built on top of kube".
This split reflects those tests which originate from the k/k repo and must match levels of kubernetes itself
from those tests which do not originate from kube.
This reduces the risk of blocking updates to kubernetes because of incompatible test changes, while maintaining
motivation to watch all tests.

## Motivation

Pulling in new kubernetes levels requires that all code in openshift/origin build against the latest k/k.
Adding substantially more tests to openshift/origin risks that task becoming significantly harder.
The previous splits we've done have significantly increased the stability and ease of updating kubernetes level,
we anticipate this doing the same for our tests.

### Goals

1. Reduce risk to updating kubernetes caused by excessive co-located code.
2. Align incentives for developers working on tests to invest in the infrastructure they rely upon.
3. Improve the confidence that new levels of kubernetes have not degraded any existing functionality.

### Non-Goals

1. Merge tests without review.
2. Abandon existing tests.

## Proposal

We will split our tests into a repo, similar to how we split separate components out of openshift/origin in 4.2.
Doing this...
1. ensures that delivery of new levels of kube will not be blocked on unrelated updates to tests.
2. ensures that people working on these tests have motivation to invest in the upstream testing framework.
3. ensures that new levels of kube don't accidentally break or invalidate unusual tests.

### The realistic, land in 4.4 approach

1. Create openshift/openshift-tests
2. Prime the repo with a `git filter-branch` from origin to keep the history of the tests we have.
3. Create a simple `go.mod` based vendoring and library-go based `Makefile`.
4. Produce images. 
   1. openshift/origin will start producing an openshift-kubernetes-tests image that has `openshift-tests` binary.
   2. openshift/origin will rename the existing `openshift-tests` binary to `openshift-kubernetes-tests` and create
      a new `openshift-tests` binary which looks in a path and runs all the `openshift-*-tests` binaries and aggregates
      the junit results.  These new binaries will go into both the existing openshift-tests image and the new openshift-kubernetes-tests image.
   3. a new job for openshift/origin will be created to run *just* openshift-kubernetes-tests image and binary.
   4. the openshift/openshift-tests repo will create a new binary called `openshift-openshift-tests`.  It will temporarily
      create an extended-platform-tests image based on openshift-kubernetes-tests that layers in the `openshift-openshift-tests`
      binary.
   5. confirm that that the extended-platform-tests image runs all tests from openshift-kubernetes-tests and openshift-openshift-tests.
      we can do this with a CI job ont he openshift/openshift-tests repo
   6. coordinate a swap of ownership of the openshift-tests image.  the extended-platform-tests image will be renamed to openshift-tests
      and openshift/origin will stop creating an openshift-tests image.
      
5. Create CI template for new `openshift-tests run openshift/conformance/parallel` and  `openshift-tests run openshift/conformance/serial`.
6. Wire the new jobs into the repos.

### The ideal "handle the testing gaps we actually need to fix" approach

After we land the realistic solution for our immediate problem, we can consider the solution to the problem we really face.
Essentially, every operator and operand in our payload and some outside of our payload need to be able to easily contribute
tests to a bucket of "these tests must pass before your PR merges" or "these tests must pass before you release".
Today those buckets are `openshift-tests run openshift/conformance/parallel` and `openshift-tests run openshift/conformance/serial`.
The tests these commands run are consistent (roughly) across all clouds and all configurations (proxy, fips, etc).

Individual operators, operands, and teams want to leverage the universal nature of these buckets, but they also want code
locality of tests to the code driving those tests.
Most developers are not CI experts and we don't need to raise the bar that high.
Instead of creating mechanisms to allow this that require modification of critical release templates, 
we can instead use the same technique used to create release payloads.

Every team that wants to contribute to the universal set of tests can do so by creating an image that has an entrypoint
which conforms to the `openshift-tests` CLI definition.
 1. run-test with dry-run
 2. run with dry-run and a common set of defined buckets.  Help lists which buckets are there.
Based on that information it is possible to layer all the binaries into a single image with a new entry point that
knows how to run `openshift-tests` style binaries.
This technique will allow non-CI experts to easily and safely contribute tests to be run in the universal buckets
from the repository of their choosing.

### Risks and Mitigations

1. The creation of a test ghetto that no one cares about.
By moving existing non-kubernetes tests to openshift-tests, we can ensure
that teams that are familiar with how the test framework works have a vested interest in the new repo being successful.

### Version Skew Strategy

There will be a version skew all the time.
A normal running circumstance is tests building on a different level of kube than the kube-apiserver.
This is a good condition both for forcing stability and for separating update cadences.
All the repos where we have done this so far have benefited from the looser coupling in terms of stability, understandability, and 
motivation for investment in upstreams. 

## Alternatives

1. Move kubelet, kube-apiserver, kube-controller-manager, kube-scheduler to a different repo.
This is functionally equivalent to moving the tests, but it has more parts and infrastructure around it.
