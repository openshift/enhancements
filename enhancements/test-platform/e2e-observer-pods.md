---
title: e2e-observer-pods
authors:
  - "@deads2k"
reviewers:
approvers:
  - "@stevek"
creation-date: yyyy-mm-dd
last-updated: yyyy-mm-dd
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also:
replaces:
superseded-by:
---

# e2e Observer Pods

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

e2e tests have multiple dimensions:
 1. which tests are running (parallel, serial, conformance, operator-specific, etc)
 2. which platform (gcp, aws, azure, etc)
 3. which configuration of that platform (proxy, fips, ovs, etc)
 4. which version is running (4.4, 4.5, 4.6, etc)
 5. which version change is running (single upgrade, double upgrade, minor upgrade, etc)
Our individual jobs are an intersection of these dimensions and that intersectionality drove the development of steps
to help handle the complexity.

There is a kind of CI test observer that wants to run across all of these intersections by leveraging the uniformity of
OCP across all the different dimensions above.
It wants to do something like start an observer agent of some kind for every CI cluster ever created and provide o(100M)
of data back to include in the CI job overall.
Working with steps would require integrating in multiple dimensions and lead to gaps in coverage.

## Motivation

We need to run tools like
 1. e2e monitor - doesn't cover installation
 2. resourcewatcher - doesn't cover installation today if we wire it directly into tests.
 3. loki log collection (as created by group-b) - hasn't been able to work in upgrades at all.
 4. loki as provided by the logging team (future)
To debug our existing install and upgrade failures.

### Goals

 1. allow a cluster observing tool to run in the CI cluster (this avoid restarts during upgrades)
 2. allow a cluster observer to provide data (including junit) to be collected by the CI job
 3. collect data from cluster observer regardless of whether the job succeeded or failed
 4. allow multiple observers per CI cluster

### Non-Goals

 1. allow a cluster observer to impact success or failure of a job
 2. allow a cluster observer to impact the running test.  This is an observer author failure.
 3. provide ANY dimension specific data. If an observer needs this, they need to integrate differently.
 4. allow multiple instances of a single cluster observer to run against one CI cluster

## Proposal

The overall goal: 
 1. have an e2e-observer process that is expected to be running before a kubeconfig exists
 2. the kubeconfig should be provided to the e2e-observer at the earliest possible time.  Even before it can be used.
 3. the e2e-observer process is expected to detect the presence of the kubeconfig itself
 4. the e2e-observer process will accept a signal indicating that teardown begins

### Changes to CI
This is a sketch of a possible path.
 1. Allow a non-dptp-developer to produces a manifest outside of any existing resource that can define
    1. image
    2. process
    3. potentially a bash entrypoint
    4. env vars
    that mounts a `secret/<job>-kubeconfig` that will later contain a kubeconfig.
    This happens to neatly align to a PodTemplate, but any file not tied to a particular CI dimension can work.
 2. Allow a developer to bind configured observer(s) to a job by one of the following mechanisms:
    1. attach the observer(s) to a step, so that any job which runs that step will run the observer
    2. attach the observer(s) to a workflow, so that any job which runs the workflow will run the observer
    3. attach the observer(s) to a literal test configuraiton, so that an observer could be added in a repo's test stanza
 3. Allow a developer to opt out of running named observer(s) by one of the following mechanisms:
    1. opt out of the observer(s) in a workflow, so that any job which runs the workflow will not run the observer
    2. opt out of the observer(s) in a literal test configuraiton, so that an observer could be excluded in a repo's test stanza
 4. Before a kubeconfig is present, create an instance of each binary from #1 is created and an empty `secret/<job>-kubeconfig`.
 5. As soon as a kubeconfig is available (these go into a known location in setup container today), write that
    `kubeconfig` into every `secret/<job>-kubeconfig` (you probably want to label them).
 6. When it is time for collection, the existing pod (I think it's teardown container), issues a SIGTERM to the process.
 7. Ten minutes teardown begins, something in CI gathers a well known directory,
    `/var/e2e-observer`, which may contain `/var/e2e-observer/junit` and `/var/e2e-observer/artifacts`.  These contents
    are placed in some reasonable spot.
    
    This could be optimized with a file write in the other direction, but the naive approach doesn't require it.
 8. All resources are cleaned up.  

### Requirements on e2e-observer authors
 1. Your pod must be able to run against *every* dimension. No exceptions.  If your pod needs to quietly no-op, it can do that.
 2. Your pod must handle a case of a kubeconfig file that isn't present when the pod starts and appears afterward.
    Or even doesn't appear at all.
 3. Your pod must be able to tolerate a kubeconfig that doesn't work.
     The kubeconfig may point to a cluster than never comes up or that hasn't come up yet.  Your pod must not fail.
 4. If your pod fails, it will not fail the e2e job.  If you need to fail an e2e job reliably, you need something else.
 5. Your pod must terminate when asked.

## Alternatives

### Modify each type of job
Given the matrix of dimensions, this seems impractical.
Even today, we have a wide variance in the quality of artifacts from different jobs.

### Modify the openshift-tests command
This is easy for some developers, BUT as e2e monitor shows us, it doesn't provide enough information.
We need information from before the test command is run to find many of our problems.
Missing logs and missing intermediate resource states (intermediate operator status) are the most egregious so far. 
