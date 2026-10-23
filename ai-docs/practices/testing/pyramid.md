# Testing Pyramid

**Category**: Engineering Practice  
**Applies To**: All OpenShift repositories  
**Last Updated**: 2026-10-23  

## Overview

OpenShift follows a testing pyramid strategy: heavy unit testing at the base, integration tests in the middle, and selective end-to-end tests at the top.

**Target ratio**: 60% unit / 30% integration / 10% e2e

## Philosophy

```text
        /  E2E  \         10% — Full cluster, slow, expensive
       /----------\
      / Integration \     30% — API-level, medium speed
     /----------------\
    /    Unit Tests     \  60% — Fast, isolated, cheap
   /--------------------\
```

**Principle**: Push testing down the pyramid. Every bug found in e2e should prompt: "Could we catch this with a unit test?"

## Test Types

| Type | Speed | Scope | When to Use |
|------|-------|-------|-------------|
| **Unit** | Milliseconds | Single function/package | Business logic, helpers, utilities |
| **Integration** | Seconds | API interaction, multiple packages | Controller reconciliation, API validation |
| **E2E** | Minutes | Full cluster | Upgrade paths, multi-component flows |

## Implementation

### Unit Tests
```bash
# Run unit tests
make test-unit
# or
go test ./pkg/... -count=1
```

**What to test**: Pure logic, state machines, error handling, edge cases.

### Integration Tests
```bash
# Run integration tests (often requires envtest or similar)
make test-integration
# or
go test ./test/integration/... -count=1
```

**What to test**: Controller reconciliation, webhook validation, API behavior.

### E2E Tests
```bash
# Run e2e tests (requires running cluster)
make test-e2e
# or
go test ./test/e2e/... -timeout 60m
```

**What to test**: Upgrade paths, multi-operator interactions, real cluster behavior.

## Best Practices

1. **Table-Driven Tests**: Use Go table-driven tests for multiple scenarios
2. **Test Fixtures**: Keep test data in `testdata/` directories
3. **No Flaky Tests**: Flaky tests erode trust — fix or remove them
4. **Parallel Tests**: Use `t.Parallel()` where safe for faster execution
5. **Test Coverage**: Track coverage but don't chase 100% — focus on critical paths

## Anti-Patterns

**Wrong approach**: Writing only e2e tests because "they test everything"  
**Why it's bad**: Slow feedback, expensive to run, hard to debug failures  
**Right approach**: Unit test the logic, integration test the API, e2e test the workflow

## OpenShift Testing Framework

| Framework | Purpose | Location |
|-----------|---------|----------|
| **envtest** | Integration testing with real API server | `sigs.k8s.io/controller-runtime/pkg/envtest` |
| **openshift-tests** | E2E test suite for OpenShift | `github.com/openshift/origin` |
| **Ginkgo/Gomega** | BDD-style test framework | `github.com/onsi/ginkgo` |

## References

- **Guide**: [Testing in OpenShift](https://github.com/openshift/enhancements/blob/master/dev-guide/testing.md)
- **Upstream**: [Go Testing](https://pkg.go.dev/testing)
- **Framework**: [envtest](https://pkg.go.dev/sigs.k8s.io/controller-runtime/pkg/envtest)
