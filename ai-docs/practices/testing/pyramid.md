# Testing Pyramid

**Category**: Engineering Practice  
**Last Updated**: 2026-04-29  

## Overview

OpenShift follows the testing pyramid: many unit tests, fewer integration tests, minimal e2e tests.

## Test Levels

| Level | Speed | Scope | When to Use |
|-------|-------|-------|-------------|
| **Unit** | <100ms | Single function/method | Logic, edge cases, error handling |
| **Integration** | <5s | Multiple components | API contracts, DB interactions |
| **E2E** | 1-30min | Full system | Critical user flows, upgrade paths |

## Unit Tests

**Characteristics**:
- Fast (<100ms per test)
- No external dependencies (mock DB, API calls)
- Test single functions/methods
- High coverage (>80% for new code)

```go
// Example: Unit test with mocking
func TestReconcileDeployment(t *testing.T) {
    client := fake.NewClientBuilder().WithObjects(
        &appsv1.Deployment{...},
    ).Build()
    
    r := &Reconciler{client: client}
    
    result, err := r.reconcileDeployment(context.TODO(), req)
    
    assert.NoError(t, err)
    assert.False(t, result.Requeue)
}
```

**Best practices**:
- ✅ Mock external dependencies
- ✅ Test error paths
- ✅ Test edge cases (nil, empty, max values)
- ❌ Don't test framework code (controller-runtime handles watches)

## Integration Tests

**Characteristics**:
- Moderate speed (<5s per test)
- Real components (etcd via envtest)
- Test API contracts and multi-component interactions
- Run in CI on every PR

```go
// Example: Integration test with envtest
func TestControllerIntegration(t *testing.T) {
    testEnv := &envtest.Environment{
        CRDDirectoryPaths: []string{"config/crd/bases"},
    }
    
    cfg, _ := testEnv.Start()
    defer testEnv.Stop()
    
    k8sClient, _ := client.New(cfg, client.Options{})
    
    // Create resource
    obj := &v1.MyResource{...}
    k8sClient.Create(ctx, obj)
    
    // Wait for reconciliation
    Eventually(func() bool {
        k8sClient.Get(ctx, key, obj)
        return obj.Status.Ready
    }, timeout, interval).Should(BeTrue())
}
```

**Best practices**:
- ✅ Use envtest for realistic API server
- ✅ Test CRD validation, webhooks
- ✅ Test controller watches and reconciliation
- ❌ Don't test full cluster setup (use e2e instead)

## E2E Tests

**Characteristics**:
- Slow (1-30 minutes)
- Real cluster (OpenShift CI)
- Test critical user flows
- Run on merge and nightly

```go
// Example: E2E test
func TestUpgrade(t *testing.T) {
    // Install operator
    installOperator(t)
    
    // Deploy workload
    deployApp(t)
    
    // Trigger upgrade
    upgradeCluster(t, "4.16.0", "4.16.1")
    
    // Verify workload still running
    assertAppHealthy(t)
}
```

**Best practices**:
- ✅ Test critical paths only (upgrade, install, day-2 ops)
- ✅ Clean up resources (don't leak)
- ✅ Use retries and timeouts
- ❌ Don't test every feature (use integration tests)

## When to Use Each Level

| Scenario | Test Level | Rationale |
|----------|-----------|-----------|
| Validate input parsing | Unit | Pure logic, no dependencies |
| Check CRD schema validation | Integration | Needs API server CRD handling |
| Verify operator reconciles resource | Integration | Needs controller-runtime + envtest |
| Confirm cluster upgrade works | E2E | Needs full cluster + CVO |
| Test edge case (nil pointer) | Unit | Fast, isolated |
| Verify webhook rejects invalid config | Integration | Needs admission controller |
| Test N→N+1 version skew | E2E | Needs multi-version cluster |

## Test Organization

```
my-operator/
├── pkg/
│   └── controller/
│       ├── reconcile.go
│       └── reconcile_test.go        # Unit tests
├── test/
│   ├── integration/
│   │   └── controller_test.go       # Integration tests (envtest)
│   └── e2e/
│       ├── install_test.go          # E2E tests
│       └── upgrade_test.go
└── Makefile
    ├── test-unit
    ├── test-integration
    └── test-e2e
```

## CI Integration

```yaml
# .ci-operator.yaml
tests:
- as: unit
  commands: make test-unit
  container:
    from: src

- as: integration
  commands: make test-integration
  container:
    from: src

- as: e2e
  steps:
    cluster_profile: aws
    test:
    - as: test
      commands: make test-e2e
      from: src
```

## Coverage Targets

| Test Level | Coverage Target | Measurement |
|-----------|----------------|-------------|
| Unit | >80% | `go test -cover` |
| Integration | API contracts | All CRDs, webhooks tested |
| E2E | Critical paths | Install, upgrade, day-2 ops |

## Common Antipatterns

❌ **Inverted pyramid**: More e2e than unit tests (slow CI)  
❌ **Testing framework**: Unit testing controller-runtime watch logic  
❌ **No mocks**: Unit tests calling real API server  
❌ **Flaky e2e**: No retries, tight timeouts  
❌ **Missing cleanup**: e2e tests leak resources

## Examples in Components

| Component | Test Strategy | Notes |
|-----------|--------------|-------|
| cluster-version-operator | Heavy integration, critical e2e | Tests upgrade orchestration |
| machine-api-operator | Integration for API, e2e for cloud | Cloud provider interactions |
| kube-apiserver | Unit for logic, e2e for availability | HA and upgrade critical |

## Ginkgo-Specific Practices

Many OpenShift operators use Ginkgo for integration and e2e test suites.

### Always use the `ginkgo` CLI for Ginkgo suites

```bash
# Correct — structured failure output, progress reports, parallel support
ginkgo -vv --trace ./internal/controller/mycontroller/

# Wrong — swallows Ginkgo's structured output
go test -v ./internal/controller/mycontroller/
```

**Why**: Ginkgo writes structured failure output (BeforeSuite errors, step-level progress reports, labeled specs) through its own reporter. When run via `go test`, failures in `SynchronizedBeforeSuite` or `BeforeAll` cause the suite to hang until `go test -timeout` kills it, producing only a goroutine dump. The actual root cause (e.g., a missing binary, a failed envtest startup) is not surfaced. Use the repo's Makefile target (e.g., `make unit`) as the first choice — it invokes `ginkgo` with the right flags. If running directly, use `ginkgo` or `go run github.com/onsi/ginkgo/v2/ginkgo`.

### envtest requires absolute paths for KUBEBUILDER_ASSETS

Integration tests using `controller-runtime/envtest` need `etcd` and `kube-apiserver` binaries, downloaded via `setup-envtest`. The `KUBEBUILDER_ASSETS` environment variable must be an **absolute path**.

`envtest`'s `BinPathFinder` passes `KUBEBUILDER_ASSETS` directly to `exec.Command` without calling `filepath.Abs()`. The `ginkgo` CLI compiles and runs the test binary from a working directory that may not be the repo root, so relative paths fail with `fork/exec .../etcd: no such file or directory` even when the binaries exist.

Most Makefiles already handle this correctly — e.g., `--bin-dir $(shell pwd)/bin` passes an absolute `--bin-dir` to `setup-envtest`, which then returns an absolute path. When running outside the Makefile:

```bash
# Correct — absolute --bin-dir produces absolute output
export KUBEBUILDER_ASSETS="$($(pwd)/bin/setup-envtest use <version> --bin-dir $(pwd)/bin -p path)"

# Wrong — relative --bin-dir produces relative output, breaks under ginkgo
export KUBEBUILDER_ASSETS="$(bin/setup-envtest use <version> --bin-dir bin -p path)"
```

## Tools

- **Unit**: `go test`, `testify/assert`, `gomock`
- **Integration**: `controller-runtime/envtest`, `ginkgo`
- **E2E**: OpenShift CI, `e2e-framework`

## References

- **Dev Guide**: [test-conventions.md](../../../dev-guide/test-conventions.md)
- **E2E Framework**: [origin/test/extended](https://github.com/openshift/origin/tree/master/test/extended)
- **CI**: [ci-operator](https://docs.ci.openshift.org/)
