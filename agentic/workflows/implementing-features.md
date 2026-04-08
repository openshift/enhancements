# Implementing Features

**Last Updated**: 2026-04-08  

## Overview

Workflow for implementing approved enhancements.

## Prerequisites

- ✅ Enhancement proposal approved
- ✅ Design finalized
- ✅ API review complete (if adding APIs)
- ✅ Timeline agreed with release team

## Implementation Steps

### 1. Break Down Work

```markdown
## Implementation Plan
- [ ] API changes (openshift/api) - 1 week
- [ ] Controller implementation - 2 weeks
- [ ] Unit tests - 1 week
- [ ] Integration tests - 1 week
- [ ] E2E tests - 1 week
- [ ] Documentation - 1 week
- [ ] Upgrade testing - 1 week
```

### 2. API Changes First

If adding/changing APIs:

```bash
# 1. Update openshift/api
cd openshift/api
# Add CRD types
git commit -m "API: Add MyResource type"
gh pr create

# 2. After merge, vendor into your repo
cd openshift/my-operator
go get github.com/openshift/api@latest
go mod vendor
git commit -m "Vendor latest openshift/api"
```

### 3. Implement Controller

```go
// pkg/controller/myresource/controller.go
type Reconciler struct {
    client.Client
    Scheme *runtime.Scheme
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    log := ctrl.LoggerFrom(ctx)
    
    // 1. Fetch resource
    resource := &myapiv1.MyResource{}
    if err := r.Get(ctx, req.NamespacedName, resource); err != nil {
        return ctrl.Result{}, client.IgnoreNotFound(err)
    }
    
    // 2. Reconcile to desired state
    if err := r.ensureDeployment(ctx, resource); err != nil {
        return ctrl.Result{}, err
    }
    
    // 3. Update status
    resource.Status.ObservedGeneration = resource.Generation
    if err := r.Status().Update(ctx, resource); err != nil {
        return ctrl.Result{}, err
    }
    
    return ctrl.Result{}, nil
}
```

### 4. Add Tests

#### Unit Tests

```go
func TestReconcile(t *testing.T) {
    scheme := runtime.NewScheme()
    _ = myapiv1.AddToScheme(scheme)
    
    client := fake.NewClientBuilder().
        WithScheme(scheme).
        Build()
    
    reconciler := &Reconciler{
        Client: client,
        Scheme: scheme,
    }
    
    // Test logic
}
```

#### Integration Tests

```go
func TestIntegration(t *testing.T) {
    // Use envtest for real API server
    testEnv := &envtest.Environment{
        CRDDirectoryPaths: []string{
            filepath.Join("..", "..", "config", "crd", "bases"),
        },
    }
    
    cfg, err := testEnv.Start()
    // Test with real API server
}
```

#### E2E Tests

```go
var _ = ginkgo.Describe("[sig-myoperator] MyResource", func() {
    ginkgo.It("should create deployment", func() {
        // Create MyResource
        // Verify deployment created
        // Verify status updated
    })
})
```

### 5. Update Documentation

```markdown
# Updates needed:
- AGENTS.md: Add feature to capabilities
- agentic/: Update relevant exec-plans
- docs/: User-facing documentation
- README.md: Usage examples
```

### 6. Test Upgrades

```bash
# Install old version
openshift-install create cluster --version=4.15.0

# Upgrade to new version with feature
oc adm upgrade --to=4.16.0-rc

# Verify:
# - Feature works after upgrade
# - No data loss
# - Status conditions correct
```

### 7. Submit PRs

#### PR 1: API Changes

```
Title: API: Add MyResource type

This adds the MyResource CRD type as described in:
https://github.com/openshift/enhancements/pull/123

Changes:
- Add MyResource CRD
- Add validation
- Add printer columns
```

#### PR 2: Controller Implementation

```
Title: Implement MyResource controller

This implements the controller for MyResource as described in:
https://github.com/openshift/enhancements/pull/123

Testing:
- Unit tests: pkg/controller/myresource/*_test.go
- Integration tests: test/integration/
- E2E tests: test/e2e/

Manual testing:
- Created MyResource on 3-node cluster
- Verified deployment created
- Verified status updated
- Tested upgrade from 4.15 → 4.16
```

## Best Practices

### 1. Start with Tests

Write failing tests, then implement:

```go
func TestFeature(t *testing.T) {
    // This will fail initially
    result := NewFeature()
    if result != expected {
        t.Error("Feature not working")
    }
}

// Now implement NewFeature() to make test pass
```

### 2. Small PRs

Break large features into reviewable chunks:

```
PR #1: Add API types (100 lines)
PR #2: Add controller skeleton (200 lines)
PR #3: Add reconciliation logic (300 lines)
PR #4: Add tests (400 lines)
```

**Not**: PR #1: Complete feature (1000 lines)

### 3. Document as You Go

Update docs in same PR as code:

```bash
# Single PR includes:
- Code changes
- Unit tests
- Integration tests
- Documentation update
```

### 4. Test Upgrades Early

Don't wait until feature complete:

```bash
# Test upgrade compatibility early
# Week 1: Implement basic feature
# Week 2: Test upgrade, find issues
# Week 3: Fix issues, add upgrade tests
```

### 5. Monitor CI

Fix flakes immediately, don't accumulate:

```
CI flake detected: TestMyFeature
→ Investigate same day
→ Fix or quarantine within 24h
→ Don't let flakes accumulate
```

## Common Pitfalls

### ❌ Implementing before approval

```bash
# Don't do this:
# 1. Write code
# 2. Submit enhancement
# 3. Discover design issues
# 4. Rewrite everything
```

### ✅ Approval before implementation

```bash
# Do this:
# 1. Submit enhancement
# 2. Get approval
# 3. Write code
# 4. Submit PR
```

### ❌ Skipping API review

API changes need review even if enhancement approved.

### ❌ Not testing upgrades

Feature works on fresh cluster but breaks upgrades.

### ❌ Large, monolithic PRs

1000+ line PRs take weeks to review.

### ❌ Missing documentation

Feature implemented but users don't know how to use it.

## Timeline Example

| Week | Activity | Deliverable |
|------|----------|-------------|
| 1 | API design | API PR merged |
| 2 | Controller skeleton | Controller compiles |
| 3-4 | Core logic | Unit tests passing |
| 5 | Integration tests | Integration tests passing |
| 6 | E2E tests | E2E tests passing |
| 7 | Documentation | Docs updated |
| 8 | Upgrade testing | Upgrade tests passing |
| 9 | Code review iterations | PRs approved |
| 10 | Merge | Feature in master |

## Graduation

### Alpha → Beta

Requirements:
- ✅ E2E tests passing
- ✅ Upgrade tests passing
- ✅ No major bugs
- ✅ Basic documentation

### Beta → GA

Requirements:
- ✅ Stable for 2 releases
- ✅ Complete documentation
- ✅ Performance tested
- ✅ Security reviewed

## References

- **Enhancement Process**: [enhancement-process.md](./enhancement-process.md)
- **Testing Practices**: [../practices/testing/](../practices/testing/)
- **API Evolution**: [../practices/development/api-evolution.md](../practices/development/api-evolution.md)
- **Git Workflow**: [../practices/development/git-workflow.md](../practices/development/git-workflow.md)
