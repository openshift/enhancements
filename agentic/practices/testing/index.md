# Testing Practices Index

**Last Updated**: 2026-04-08  

## Overview

Testing practices and policies for all OpenShift components.

## Test Levels

| Practice | Purpose | File |
|----------|---------|------|
| Testing Pyramid | Unit/Integration/E2E distribution strategy | [pyramid.md](./pyramid.md) |
| E2E Framework | openshift-tests usage and patterns | [e2e-framework.md](./e2e-framework.md) |
| CI Integration | Prow and OpenShift CI configuration | [ci-integration.md](./ci-integration.md) |
| Test Flake Policy | Identifying and fixing flaky tests | [test-flake-policy.md](./test-flake-policy.md) |

## Quick Reference

### Writing Tests

**Unit tests**: 60% coverage target, fast (<100ms), no external dependencies  
**Integration tests**: 30% coverage, test component interactions, use envtest  
**E2E tests**: 10% coverage, full cluster tests, critical paths only  

See [pyramid.md](./pyramid.md)

### E2E Test Labels

- `[Slow]` - Takes >1 minute
- `[Serial]` - Cannot run in parallel
- `[Disruptive]` - May affect other tests
- `[sig-NAME]` - Special interest group

See [e2e-framework.md](./e2e-framework.md)

### CI Jobs

**presubmit**: Runs on PR (unit, verify, e2e)  
**postsubmit**: Runs after merge (builds, releases)  
**periodic**: Scheduled jobs (upgrade, scale tests)  

See [ci-integration.md](./ci-integration.md)

### Flaky Tests

Pass rate <98% in 14 days = quarantine  
Add `[Flaky]` label, file issue, fix within 14 days  

See [test-flake-policy.md](./test-flake-policy.md)

## Common Patterns

### Wait for Condition

```go
gomega.Eventually(func() bool {
    return checkCondition()
}, "5m", "10s").Should(gomega.BeTrue())
```

### Clean Up Resources

```go
ginkgo.AfterEach(func() {
    if resource != nil {
        deleteResource(resource)
    }
})
```

### Test Table Pattern

```go
tests := []struct {
    name    string
    input   Input
    wantErr bool
}{
    {"valid", validInput, false},
    {"invalid", invalidInput, true},
}
for _, tt := range tests {
    t.Run(tt.name, func(t *testing.T) {
        // Test logic
    })
}
```

## See Also

- [Security Testing](../security/) - Security-specific test practices
- [Reliability](../reliability/) - SLO monitoring and observability
- [Development Practices](../development/) - Code review, Git workflow
