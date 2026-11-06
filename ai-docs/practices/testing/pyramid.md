# Testing Pyramid

**Category**: Testing Practice
**Authoritative Source**: [dev-guide/test-conventions.md](../../../dev-guide/test-conventions.md)
**Last Updated**: 2026-10-23

---

## Overview

OpenShift follows a testing pyramid: heavy unit testing at the base, integration in the middle, and targeted end-to-end tests at the top.

## Test Layers

| Layer | Scope | Where |
|-------|-------|-------|
| **Unit** | Single package, mocked dependencies | `*_test.go` beside source |
| **Integration** | Multiple packages, real APIs where feasible | `test/integration/` or `test/e2e/` |
| **End-to-end** | Full cluster, real workloads | [openshift/origin](https://github.com/openshift/origin) conformance suites |

## Key Conventions

### Test Naming

Tests must have **stable names** — no quantities, no dynamic pod names:
- **Good**: `[sig-install] cluster should complete installation in a reasonable time`
- **Bad**: `[sig-install] cluster should complete installation in 30 minutes`

### Test Ownership

Every test must have a single, clear owner via a Jira `OCPBUGS` component. Ownership is mapped in [ci-test-mapping](https://github.com/openshift-eng/ci-test-mapping).

### Result Requirements

Tests must **always produce a result** (pass or fail). Tests that only emit failure make pass-rate analysis impossible.

### Suites

- `openshift/conformance/parallel` — runs concurrently; tests must not mutate shared cluster state AND must clean up after themselves, including on errors
- `openshift/conformance/serial` — isolated, sequential execution
- `[Early]` and `[Late]` annotations place tests near the start or end of a run; all other ordering is randomized and not guaranteed

### Flake Handling

If a test fails then succeeds on retry, it is a **flake**. Flakes must not fail the overall CI job. Each retry attempt produces a discrete JUnit result.

## OpenShift CI Framework

OpenShift CI uses [Prow](https://github.com/kubernetes/test-infra/tree/master/prow) with [ci-operator](https://github.com/openshift/ci-tools) for test orchestration. Component tests run via step registry workflows defined in [openshift/release](https://github.com/openshift/release).

## Platform Documentation

For full test conventions, see [dev-guide/test-conventions.md](../../../dev-guide/test-conventions.md). For component-specific test patterns, see the component's own `ai-docs/TESTING.md`.
