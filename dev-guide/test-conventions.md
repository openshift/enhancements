---
title: Test conventions
authors:
  - "@stbenjam"
reviewers:
  - "@deads2k"
  - "@jupierce"
approvers:
  - "@deads2k"
  - "@jupierce"
creation-date: 2024-09-19
last-updated: 2024-09-19
status: informational
---

# OpenShift Test Conventions

## Overview

This document outlines the conventions for writing and maintaining
OpenShift tests, ensuring consistency, reliability and maintainability
across the platform.

## Concepts

### Test

A test verifies that software behaves as expected during the development
process. This can include unit tests, integration tests, or functional
tests.

### Suite

A suite is a named collection of tests. A test may be in multiple
suites.

### Annotation

An annotation is a bracketed piece of metadata present in a test's name,
for example: `[Driver:ec2]` or `[sig-apiserver]`.

### Component

When referring to a test's component, it is the name of the Jira
`OCPBUGS` component (e.g. `Networking / ovn-kubernetes`). A test must
belong to one and only one component.

### Capability

A capability is a particular piece of functionality that a test is
testing. A test may have multiple capabilities.  Example capabilities:
`Install`, `Operator conditions`, `FeatureGate:UserDefinedNetworking`.

## Guidelines

### New tests

#### Have a clear owner

Tests must have a single, clear owner via an assigned `OCPBUGS` Jira
component. Mapping to Jira components occurs in in the
[ci-test-mapping](https://github.com/openshift-eng/ci-test-mapping)
repository based on matches, for example: tests that mention a
particular namespace or cluster-operator will be routed to that
component.  You can also assign ownership with an annotation, such as
`[Jira:kube-apiserver]`.

#### Always produce a result

Test authors must ensure that a test, when it runs, always produces a
result (either success or failure).  Tests that only produce failure
results can't accurately calculate their pass percentage or do
statistical analysis to monitor the test's reliability over time.

To be consumed by tools like Spyglass and Component Readiness, results
should be emitted in the JUnit XML format.

#### May retry or run more than once

A test can run multiple times in a job run. For example, by retrying on
failure. Each attempt must produce a discrete result.

If a test fails and then succeeds at least once, we call this a `flake`
and should not cause your testing framework to exit with an error, and
should not fail the overall CI job.

Flakes may be synthetically created for cases that should not fail a
job, but still be detectable.

#### Be narrow in scope

As much as possible, tests should be narrow in scope. For tests
enforcing platform invariants, namespaced tests are preferred over
global.

**Note**: Must-gather creates namespaces with an `openshift-` prefix
that have a randomly generated suffix; these must be excluded from
tests.

Examples of good tests:

- `[sig-architecture] platform pods in ns/openshift-monitoring should not fail to start`
- `[sig-auth] all workloads in ns/openshift-operators must set the 'openshift.io/required-scc' annotation`

Examples of less than ideal test names:

- `[sig-architecture] platform pods should not fail to start`
- `[sig-arch][Late] operators should not create watch channels very often`

#### Have a stable name

Tests should not contain quantities, as these are often subject to
change.  Prefer to use terminology like "moderate", "excessive",
"reasonable", instead of "10 times", "30 minutes", etc.

We often compare test results across large spans of time, and
rely on test names being stable.  We do support renaming tests but it
should be avoided as much as possible.

Tests must not include content that changes every run, such as the name
of a pod with a random identifier.

Examples of good tests:

- `[sig-install] cluster should complete installation in a reasonable time`
- `[sig-architecture] platform pods in ns/openshift-monitoring should not exit with error excessively`

Examples of less than ideal test names:

- `[sig-install] cluster should complete installation in 30 minutes`
- `[sig-architecture] platform pods should not exit with error more than 3 times`

Examples of invalid test names:

- `[sig-node] pod e2e-test-4re3x should restart on failure` (contains a dynamic changing pod name)

#### Belong to a test suite

New tests must belong to a test suite that accurately indicates it's
usage. Suite names must be chosen to be unique across OpenShift.
Examples of good suite names: "openshift/conformance/parallel",
and "hypershift-e2e."

Suites must not be called a generic name like "test", "e2e", etc.

### Additional guidelines for conformance tests

Conformance tests are those tests that run the
`openshift/conformance/parallel` or `openshift/conformance/serial`
suites, located currently in the `openshift/origin` repo.

The parallel suite as the name implies runs multiple tests in parallel.
Serial tests are isolated from each other and only run serially.

Parallel tests must be quick, lightweight and able to run concurrently
with other tests. It must not make modifications to the cluster that
could interefere with other tests.  It must clean-up after itself when
it's done, including in error conditions.

Test timing can be controlled to happen at the start of the run by
adding `[Early]` to your test name, or at the end of the run by
appending `[Late]`, otherwise there's no guarantee when your test will
run. Test ordering is otherwise randomized.

### Renaming a test

When unavoidable, a test can be renamed. Once renamed, the test's
component in
[ci-test-mapping](https://github.com/openshift-eng/ci-test-mapping?tab=readme-ov-file#renaming-tests)
must have it's test rename map updated. This causes all versions of
the test to use the same ID, allowing comparisons across names.

### Obsoleting a test

If a test is deemed no longer neccessary, it should be removed from the
relevant testing repo, and marked as obsolete in
[ci-test-mapping](https://github.com/openshift-eng/ci-test-mapping/tree/main/pkg/obsoletetests).

Staff engineer approval is required for marking a test obsolete.
