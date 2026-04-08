# Testing Pyramid

**Category**: Engineering Practice  
**Applies To**: All OpenShift repositories  
**Last Updated**: 2026-04-08  

## Philosophy

```
         /\
        /  \  E2E (10%)
       /────\
      /      \  Integration (30%)
     /────────\
    /          \  Unit (60%)
   /────────────\
```

## Coverage Targets

| Level | Coverage | Time | Cost | Purpose |
|-------|----------|------|------|---------|
| Unit | 60% | ms | Low | Fast feedback, pinpoint failures |
| Integration | 30% | sec | Medium | Component interactions, API contracts |
| E2E | 10% | min | High | Full system validation, real behavior |

## Rationale

**Why this distribution?**
- **Unit tests** are fast and cheap - catch most bugs early
- **Integration tests** verify contracts between components
- **E2E tests** are slow and expensive - use sparingly for critical paths

**Anti-pattern**: Too many E2E tests
- Slow CI
- Flaky tests
- Hard to debug
- Expensive to maintain

## Unit Tests

### Implementation

```bash
# Run unit tests
make test-unit

# With coverage
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### What to Test

- Business logic
- Edge cases
- Error handling
- Input validation
- Pure functions

### Example

```go
func TestMachineConfigValidation(t *testing.T) {
    tests := []struct {
        name    string
        mc      *MachineConfig
        wantErr bool
    }{
        {
            name: "valid config",
            mc: &MachineConfig{
                Spec: MachineConfigSpec{
                    OSImageURL: "https://example.com/image",
                },
            },
            wantErr: false,
        },
        {
            name: "invalid URL",
            mc: &MachineConfig{
                Spec: MachineConfigSpec{
                    OSImageURL: "invalid://url",
                },
            },
            wantErr: true,
        },
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := ValidateMachineConfig(tt.mc)
            if (err != nil) != tt.wantErr {
                t.Errorf("ValidateMachineConfig() error = %v, wantErr %v", err, tt.wantErr)
            }
        })
    }
}
```

### Best Practices

- Use table-driven tests
- Test error paths
- Mock external dependencies
- Keep tests fast (<100ms each)
- No network or filesystem access

## Integration Tests

### Implementation

```bash
# Run integration tests
make test-integration
```

### What to Test

- Component interactions
- API contracts
- Controller reconciliation loops
- Cache/watch behavior
- Real kubernetes client (envtest)

### Example

```go
func TestMachineConfigController(t *testing.T) {
    // Setup fake kubernetes client
    scheme := runtime.NewScheme()
    _ = machineconfigv1.AddToScheme(scheme)
    
    client := fake.NewClientBuilder().
        WithScheme(scheme).
        Build()
    
    // Create controller
    controller := &MachineConfigController{
        Client: client,
        Scheme: scheme,
    }
    
    // Create MachineConfig
    mc := &machineconfigv1.MachineConfig{
        ObjectMeta: metav1.ObjectMeta{
            Name: "test-mc",
        },
        Spec: machineconfigv1.MachineConfigSpec{
            OSImageURL: "https://example.com/image",
        },
    }
    err := client.Create(context.TODO(), mc)
    if err != nil {
        t.Fatalf("Failed to create MC: %v", err)
    }
    
    // Trigger reconciliation
    result, err := controller.Reconcile(context.TODO(), ctrl.Request{
        NamespacedName: types.NamespacedName{
            Name: "test-mc",
        },
    })
    
    // Verify expected behavior
    if err != nil {
        t.Errorf("Reconcile failed: %v", err)
    }
    if result.Requeue {
        t.Error("Unexpected requeue")
    }
    
    // Verify status updated
    err = client.Get(context.TODO(), types.NamespacedName{Name: "test-mc"}, mc)
    if err != nil {
        t.Fatalf("Failed to get MC: %v", err)
    }
    
    if mc.Status.ObservedGeneration != mc.Generation {
        t.Error("Status not updated")
    }
}
```

### Best Practices

- Use envtest for realistic API server
- Test watch/cache behavior
- Test controller error handling
- Verify status updates
- Keep tests under 5 seconds

## E2E Tests

### Implementation

```bash
# Run E2E tests
make test-e2e

# Or via openshift-tests
openshift-tests run openshift/conformance
```

### What to Test

- Critical user workflows
- Upgrade paths
- Disaster recovery
- Multi-component interactions
- Real cluster behavior

### When NOT to Use E2E

- Unit tests can cover it
- Integration tests can cover it
- Flaky or unreliable tests
- Testing internal implementation details

### Example

```go
var _ = ginkgo.Describe("[sig-machineconfig] MachineConfig", func() {
    var (
        oc *exutil.CLI
        mc *machineconfigv1.MachineConfig
    )
    
    ginkgo.BeforeEach(func() {
        oc = exutil.NewCLI("machineconfig")
        
        mc = &machineconfigv1.MachineConfig{
            ObjectMeta: metav1.ObjectMeta{
                Name: "99-worker-test",
                Labels: map[string]string{
                    "machineconfiguration.openshift.io/role": "worker",
                },
            },
            Spec: machineconfigv1.MachineConfigSpec{
                Config: ignition.Config{
                    Storage: ignition.Storage{
                        Files: []ignition.File{
                            {
                                Path: "/etc/test-file",
                                Contents: ignition.FileContents{
                                    Source: "data:,test-content",
                                },
                            },
                        },
                    },
                },
            },
        }
    })
    
    ginkgo.AfterEach(func() {
        if mc != nil {
            oc.AdminConfigClient().MachineconfigurationV1().MachineConfigs().Delete(
                context.Background(), mc.Name, metav1.DeleteOptions{})
        }
    })
    
    ginkgo.It("should update nodes when MachineConfig changes [Slow]", func() {
        // Create MachineConfig
        _, err := oc.AdminConfigClient().MachineconfigurationV1().MachineConfigs().Create(
            context.Background(), mc, metav1.CreateOptions{})
        gomega.Expect(err).NotTo(gomega.HaveOccurred())
        
        // Wait for MachineConfigPool to update
        gomega.Eventually(func() bool {
            pool, err := oc.AdminConfigClient().MachineconfigurationV1().
                MachineConfigPools().Get(context.Background(), "worker", metav1.GetOptions{})
            if err != nil {
                return false
            }
            return pool.Status.UpdatedMachineCount == pool.Status.MachineCount
        }, "10m", "30s").Should(gomega.BeTrue())
        
        // Verify file on node
        node := getWorkerNode(oc)
        output := debugNodeExec(oc, node, "cat /etc/test-file")
        gomega.Expect(output).To(gomega.Equal("test-content"))
    })
})
```

### Best Practices

- Use descriptive test names
- Tag with [Slow], [Serial], [Disruptive] as needed
- Clean up resources in AfterEach
- Use Eventually/Consistently for async operations
- Avoid sleeps - poll with timeouts

## Examples in Components

| Component | Unit | Integration | E2E |
|-----------|------|-------------|-----|
| machine-config-operator | pkg/*_test.go | test/integration/ | test/e2e/ |
| cluster-network-operator | pkg/*_test.go | test/integration/ | test/e2e/ |
| installer | pkg/*_test.go | tests/terraform/ | tests/e2e/ |

## CI Enforcement

```bash
# CI runs all test levels
make verify  # Linting, formatting
make test-unit  # Fast, runs on every PR
make test-integration  # Medium, runs on every PR
make test-e2e  # Slow, runs on merge and periodic
```

See [ci-integration.md](./ci-integration.md) for CI details.

## References

- **E2E Framework**: [e2e-framework.md](./e2e-framework.md)
- **CI Integration**: [ci-integration.md](./ci-integration.md)
- **Test Flake Policy**: [test-flake-policy.md](./test-flake-policy.md)
- **Ginkgo**: https://onsi.github.io/ginkgo/
