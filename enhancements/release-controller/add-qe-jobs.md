---
title: add-qe-jobs
authors:
  - "@jianzhangbjz"
reviewers:
  - "@bradmwilliams"
  - "@liangxia"
  - "@rioliu-rh"
  - "sosiouxme" # for ART things, such as payload pick up
approvers:
  - "@bradmwilliams"
creation-date: 2023-12-14
last-updated: 2023-12-15
tracking-link: 
  - https://docs.google.com/document/d/1uTA_uspXcztcUWnSRtKtv_kaw076QgGroLLWHbdrSOU/edit 
see-also:
  - https://docs.google.com/document/d/1qYbBtCpzytjdiBrwvz-oI4RbA5afLCBUAOJI38UYfAU/edit
replaces:
  - None
superseded-by:
  - None
---

# add QE jobs
We are proposing to get payload shipping with least interruption to the major/minor release effort to implement payload shipping automatically, for now, EC payloads, finally RC ones. So, we need to get QE jobs run in an early phase, such as, `ready` status, but after the `Informing` and `Blocking` jobs pass. See below for the details of why we propose to do this and how we propose to do it.

## Summary
Currently, the [QE jobs](https://github.com/openshift/release/tree/master/ci-operator/jobs/openshift/openshift-tests-private) get run after the payload labeled with `Accepted`. But, the test results are not reflect on the payload after job finished. We are proposing to leverage [release-controller](https://github.com/openshift/release-controller/tree/master) to implement it.

## Motivation
Now, the `Accepted` EC payload can be used by customer directly without the QE tests. We hope to add QE tests for it. And, for RC payloads, we use errata workflow to ship it to the customer, but it needs manual interruption. In the near future, we hope to ship it automatically.

### Goals
- Once the `Informing` and `Blocking` jobs pass, run QE jobs.
- Custom new labels to reflect QE jobs results for nightly payloads, such as `QE Accepted` and `QE Rejected`, and display it on the web console: https://amd64.ocp.releases.ci.openshift.org/ 

### Non-Goals
-  add QE jobs to the [OpenShift Release Gates](https://docs.ci.openshift.org/docs/architecture/release-gating/)

## Proposal
The following sections describe in detail.

### Workflow Description
- select appropicate QE jobs and add them to [openshift release](https://github.com/openshift/release/tree/master/core-services/release-controller/_releases), as for the mechanism and standrad, see [Proposal - Fully automated the OpenShift EC and z-stream releases](https://docs.google.com/document/d/1qYbBtCpzytjdiBrwvz-oI4RbA5afLCBUAOJI38UYfAU/edit#heading=h.601z0umsqvm), and @liangxia is working on it.
- release-controller handle QE jobs
- mark the release with `QEAccepted` or `QERejected` based on QE jobs results
- the ART team select the candidate payloads from those `QEAccepted` ones. 

### API Extensions
- Update [release-controller-api](https://github.com/openshift/release-controller/blob/master/cmd/release-controller-api/http.go#L158-L195) to handle `QEAccepted/QERejected` http requests.

## Implementation Details/Notes/Constraints
- Update `ReleaseVerification` struct to add 
```go
	// QE verifications are run, but failures will cause the release to be "QE rejected".
	QE bool `json:"qe"`
```
- Create a new function to get QE jobs, maybe called `GetQEJobs` under [releasecontroller package](https://github.com/openshift/release-controller/blob/master/pkg/release-controller/release.go)
- Create a method for `Controller` struct called `ensureQEJobs` to query or run QE jobs
- The `ensureQEJobs` method should be called in [syncReady](https://github.com/openshift/release-controller/blob/master/cmd/release-controller/sync.go#L471) after job passed [ensureVerificationJobs](https://github.com/openshift/release-controller/blob/master/cmd/release-controller/sync.go#L484-L520).
- Add new consts under [releasecontroller types](https://github.com/openshift/release-controller/blob/master/pkg/release-controller/types.go#L510), like below
```go
ReleasePhaseQEAccepted = "QEAccepted"
ReleasePhaseQERejected = "QERejected"
ReleaseAnnotationQEVerify = "release.openshift.io/qe-verify"
``` 
- Create a method called `markReleaseQEAccepted` to add `QEAccepted` tag to the release 

### Risks and Mitigations
- Limits time to run QE jobs

### Vocabulary

We often refer to these concepts with an imprecise vernacular, let’s try to codify a couple to make what follows easier
to understand.

1. Job -- example: periodic-ci-openshift-release-master-ci-4.9-e2e-gcp-upgrade. Jobs are a description of what set of
   the environment a cluster is installed into and which Tests will run and how. Jobs have multiple JobRuns associated
   with them.
2. Payload Blocking Job -- click an instance of 4.9-nightly. About 4 jobs. A job that is run on prospective payloads. If
   the JobRun does not succeed, the payload is not promoted.
3. Payload Informing Job -- click an instance of 4.9-nightly. About 34 jobs. A job that is run on prospective payloads.
   If the Job run does not succeed, nothing happens: the payload is promoted anyway.

### Drawbacks

TBD

## Design Details

### Open Questions [optional]

TBD

### Test Plan

TBD

### Graduation Criteria

TBD

#### Dev Preview -> Tech Preview

TBD

#### Tech Preview -> GA

TBD

#### Removing a deprecated feature

None

### Upgrade / Downgrade Strategy

None

### Version Skew Strategy

None

### Operational Aspects of API Extensions

TBD

#### Failure Modes

TBD

#### Support Procedures

TBD

## Implementation History

TBD

## Alternatives

TBD

## Infrastructure Needed [optional]

None
