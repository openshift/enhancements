# Test Flake Policy

**Category**: Engineering Practice  
**Applies To**: All OpenShift repositories  
**Last Updated**: 2026-04-08  

## Overview

Flaky tests damage CI reliability and developer productivity. This policy defines how to identify, quarantine, and fix flaky tests.

## Definition

**Flaky test**: A test that passes or fails non-deterministically with the same code.

**Not flaky**:
- Test fails consistently due to bug
- Test fails due to infrastructure issue (outage)
- Test fails due to timing change in code

## Flake Metrics

| Metric | Threshold | Action |
|--------|-----------|--------|
| **Pass rate** | <98% in 14 days | Quarantine |
| **Consecutive failures** | 3+ | Investigate immediately |
| **MTBF** | <7 days | Fix or disable |

## Quarantine Process

### 1. Identify Flake

```bash
# Check test history in Prow
# Look for intermittent failures

# Example flake pattern:
PR #123: ✅ pass
PR #124: ❌ fail
PR #125: ✅ pass  # Same code as #124
PR #126: ❌ fail
```

### 2. Mark as Flaky

```go
// Add [Flaky] label to quarantine
var _ = ginkgo.Describe("[sig-machineconfig][Flaky] MachineConfig", func() {
    ginkgo.It("should update nodes", func() {
        // Flaky test
    })
})
```

**Effect**: Test still runs but doesn't block merges

### 3. File Issue

```markdown
Title: [Flake] Test "should update nodes" is flaky

## Test
`[sig-machineconfig] MachineConfig should update nodes`

## Failure Rate
5/10 runs failed in last 7 days

## Recent Failures
- https://prow.ci.openshift.org/view/gs/.../123
- https://prow.ci.openshift.org/view/gs/.../124

## Error Pattern
```
Timeout waiting for MachineConfigPool update
Expected pool.Status.UpdatedMachineCount == pool.Status.MachineCount
```

## Suspected Cause
Race condition in pool status update
```

### 4. Fix Within 14 Days

**Options**:
1. Fix root cause (preferred)
2. Make test more resilient
3. Skip test if unfixable
4. Delete test if not valuable

### 5. Remove [Flaky] Label

After fix is merged and test passes consistently for 7 days.

## Common Flake Causes

### 1. Race Conditions

**Problem**:
```go
// Create resource
createPod(oc, "test-pod")

// Immediately check status (race!)
pod := getPod(oc, "test-pod")
if pod.Status.Phase != corev1.PodRunning {
    t.Error("Pod not running")  // Flake!
}
```

**Fix**:
```go
createPod(oc, "test-pod")

// Use Eventually to wait
gomega.Eventually(func() bool {
    pod := getPod(oc, "test-pod")
    return pod.Status.Phase == corev1.PodRunning
}, "2m", "5s").Should(gomega.BeTrue())
```

### 2. Insufficient Timeouts

**Problem**:
```go
gomega.Eventually(func() bool {
    return checkCondition()
}, "10s", "1s").Should(gomega.BeTrue())  // Too short!
```

**Fix**:
```go
// Use generous timeouts in E2E tests
gomega.Eventually(func() bool {
    return checkCondition()
}, "5m", "10s").Should(gomega.BeTrue())
```

### 3. Resource Leaks

**Problem**:
```go
ginkgo.It("should create pod", func() {
    createPod(oc, "test-pod")
    // No cleanup!
})
// Next test fails due to resource conflict
```

**Fix**:
```go
var pod *corev1.Pod

ginkgo.AfterEach(func() {
    if pod != nil {
        deletePod(oc, pod.Name)
    }
})

ginkgo.It("should create pod", func() {
    pod = createPod(oc, "test-pod")
})
```

### 4. Shared State

**Problem**:
```go
var globalConfig = &Config{} // Shared across tests

ginkgo.It("test 1", func() {
    globalConfig.Value = "test1"  // Modifies global
})

ginkgo.It("test 2", func() {
    // Expects globalConfig to be fresh (flake!)
})
```

**Fix**:
```go
var config *Config

ginkgo.BeforeEach(func() {
    config = &Config{}  // Fresh per test
})
```

### 5. External Dependencies

**Problem**:
```go
// Test depends on external API
response := callExternalAPI()  // May timeout
```

**Fix**:
```go
// Mock external dependencies in unit/integration tests
mockAPI := &MockAPI{
    Response: "expected",
}

// Or use retries in E2E tests
var response string
gomega.Eventually(func() error {
    var err error
    response, err = callExternalAPI()
    return err
}, "1m", "5s").Should(gomega.Succeed())
```

### 6. Timing Assumptions

**Problem**:
```go
startTime := time.Now()
doWork()
duration := time.Since(startTime)

// Assumes doWork() takes <100ms (flake on slow nodes!)
if duration > 100*time.Millisecond {
    t.Error("Too slow")
}
```

**Fix**:
```go
// Use generous margins or don't test timing
if duration > 1*time.Second {
    t.Error("Unexpectedly slow")
}

// Or test functionality, not timing
result := doWork()
if !result.Success {
    t.Error("Work failed")
}
```

## Fixing Flaky Tests

### Investigation Steps

1. **Reproduce locally**
```bash
# Run test 100 times
for i in {1..100}; do
    make test-e2e || echo "Failed on iteration $i"
done
```

2. **Check test logs**
- Look for timing issues
- Check for resource conflicts
- Identify error patterns

3. **Add debugging**
```go
ginkgo.It("should update nodes", func() {
    ginkgo.GinkgoWriter.Printf("Starting test at %v\n", time.Now())
    
    mc := createMachineConfig()
    ginkgo.GinkgoWriter.Printf("Created MC: %+v\n", mc)
    
    // Add detailed logging
})
```

4. **Isolate the flake**
```go
// Comment out parts of test to find flaky section
ginkgo.It("should update nodes", func() {
    mc := createMachineConfig()
    // waitForUpdate()  // Is this the flaky part?
    // verifyNodes()
})
```

### Fix Patterns

**Pattern 1: Increase timeout**
```go
// Before
gomega.Eventually(check, "10s", "1s")

// After
gomega.Eventually(check, "5m", "10s")
```

**Pattern 2: Add retry logic**
```go
// Before
result := operation()

// After
var result Result
gomega.Eventually(func() error {
    var err error
    result, err = operation()
    return err
}, "2m", "5s").Should(gomega.Succeed())
```

**Pattern 3: Improve test isolation**
```go
// Before
var sharedResource *Resource

// After
ginkgo.BeforeEach(func() {
    sharedResource = &Resource{}
})
```

**Pattern 4: Use more specific checks**
```go
// Before (flaky)
gomega.Expect(pod.Status.Phase).To(gomega.Equal(corev1.PodRunning))

// After (more resilient)
gomega.Eventually(func() bool {
    pod := getPod()
    return pod.Status.Phase == corev1.PodRunning &&
           len(pod.Status.ContainerStatuses) > 0 &&
           pod.Status.ContainerStatuses[0].Ready
}, "2m", "5s").Should(gomega.BeTrue())
```

## When to Skip Tests

If a test cannot be fixed within 14 days:

```go
// Temporarily skip
var _ = ginkgo.Describe("[sig-machineconfig] MachineConfig", func() {
    ginkgo.It("should update nodes [Skip:Flaky]", func() {
        ginkgo.Skip("Skipping due to flake: https://issues.redhat.com/browse/OCPBUGS-12345")
    })
})
```

**Requirements**:
- File issue tracking the skip
- Assign owner to fix
- Set deadline to re-enable or delete

## When to Delete Tests

Delete test if:
- Test provides no value (redundant coverage)
- Feature being tested no longer exists
- Test cannot be made reliable
- Cost of maintenance > value

## Metrics and Monitoring

### Flake Dashboard

Monitor test reliability:
- **TestGrid**: https://testgrid.k8s.io/openshift
- Shows pass rates over time
- Identifies flaky tests

### CI Health Metrics

| Metric | Target |
|--------|--------|
| Overall pass rate | >95% |
| Mean time to green | <90 min |
| Flaky test count | <5% of total |

## Examples

| Component | Flaky Test | Root Cause | Fix |
|-----------|------------|------------|-----|
| MCO | Node update timeout | Slow reboot | Increased timeout to 20min |
| CNO | Network ready check | Race condition | Used Eventually instead of sleep |
| Console | UI element not found | Slow page load | Wait for element with retry |

## Prevention

### Code Review Checklist

- ❌ Uses `time.Sleep` instead of `Eventually`
- ❌ No cleanup in `AfterEach`
- ❌ Shares state between tests
- ❌ Hardcoded tight timeouts
- ❌ No retry for network operations

### CI Enforcement

```yaml
# Run tests multiple times to catch flakes
- as: unit-flake-check
  commands: |
    for i in {1..10}; do
      make test-unit || exit 1
    done
```

## References

- **Ginkgo Best Practices**: https://onsi.github.io/ginkgo/#mental-model-how-ginkgo-traverses-the-spec-hierarchy
- **TestGrid**: https://testgrid.k8s.io/openshift
- **CI Integration**: [ci-integration.md](./ci-integration.md)
- **E2E Framework**: [e2e-framework.md](./e2e-framework.md)
