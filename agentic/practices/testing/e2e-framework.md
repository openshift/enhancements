# OpenShift E2E Testing Framework

**Category**: Engineering Practice  
**Applies To**: All OpenShift repositories  
**Last Updated**: 2026-04-08  

## Overview

OpenShift uses `openshift-tests` binary for end-to-end testing across all components.

## Framework Structure

```bash
# Run all conformance tests
openshift-tests run openshift/conformance

# Run specific suite
openshift-tests run openshift/network

# Run regex match
openshift-tests run --dry-run | grep -i machine | openshift-tests run --file -

# List all tests
openshift-tests run --dry-run
```

## Test Organization

```
test/
├── e2e/              # Cloud-agnostic tests (most tests go here)
├── e2e-aws/          # AWS-specific tests
├── e2e-azure/        # Azure-specific tests
├── e2e-gcp/          # GCP-specific tests
└── e2e-agnostic/     # Alternative location for cloud-agnostic
```

**Guideline**: Prefer cloud-agnostic tests unless the test is truly platform-specific.

## Writing Tests

### Basic Test Structure

```go
package e2e

import (
    "context"
    
    "github.com/onsi/ginkgo/v2"
    "github.com/onsi/gomega"
    
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    exutil "github.com/openshift/origin/test/extended/util"
)

var _ = ginkgo.Describe("[sig-machineconfig] MachineConfig", func() {
    defer ginkgo.GinkgoRecover()
    
    var oc *exutil.CLI
    
    ginkgo.BeforeEach(func() {
        oc = exutil.NewCLI("machineconfig-test")
    })
    
    ginkgo.It("should apply file changes [Slow]", func() {
        // Test implementation
        mc := createMachineConfig(oc, "test-mc")
        gomega.Expect(mc).NotTo(gomega.BeNil())
        
        waitForMachineConfigPoolUpdate(oc, "worker")
        
        verifyFileOnNodes(oc, "/etc/test-file")
    })
})
```

### Test Labels

Use Ginkgo labels to categorize tests:

| Label | Meaning | Example Use Case |
|-------|---------|------------------|
| `[Slow]` | Takes >1 minute | Node reboot tests |
| `[Serial]` | Cannot run in parallel | Cluster-wide config changes |
| `[Disruptive]` | May affect other tests | Node deletion, cluster shutdown |
| `[sig-NAME]` | Special interest group | [sig-machineconfig], [sig-network] |
| `[Early]` | Run before other tests | Pre-requisite setup |
| `[Late]` | Run after other tests | Cleanup validation |

### Example with Multiple Labels

```go
var _ = ginkgo.Describe("[sig-machineconfig][Slow][Disruptive] Node Updates", func() {
    ginkgo.It("should reboot nodes when OS image changes", func() {
        // Test that causes node reboots
    })
})
```

## Best Practices

### 1. Use Descriptive Names

**Good**:
```go
ginkgo.It("should update node OS when MachineConfig osImageURL changes", func() {
```

**Bad**:
```go
ginkgo.It("should work", func() {
```

### 2. Clean Up Resources

```go
var _ = ginkgo.Describe("[sig-machineconfig] MachineConfig", func() {
    var (
        oc *exutil.CLI
        mc *machineconfigv1.MachineConfig
    )
    
    ginkgo.BeforeEach(func() {
        oc = exutil.NewCLI("mc-test")
    })
    
    ginkgo.AfterEach(func() {
        // Always clean up
        if mc != nil {
            oc.AdminConfigClient().MachineconfigurationV1().
                MachineConfigs().Delete(context.Background(), 
                    mc.Name, metav1.DeleteOptions{})
        }
    })
    
    ginkgo.It("should create MachineConfig", func() {
        mc = createMachineConfig(oc, "test-mc")
        // Test logic
    })
})
```

### 3. Avoid Sleeps - Use Eventually

**Bad**:
```go
createPod(oc, "test-pod")
time.Sleep(30 * time.Second)  // Hope it's ready
checkPod(oc, "test-pod")
```

**Good**:
```go
createPod(oc, "test-pod")
gomega.Eventually(func() bool {
    pod, err := oc.KubeClient().CoreV1().Pods(oc.Namespace()).
        Get(context.Background(), "test-pod", metav1.GetOptions{})
    if err != nil {
        return false
    }
    return pod.Status.Phase == corev1.PodRunning
}, "2m", "5s").Should(gomega.BeTrue())
```

### 4. Make Tests Idempotent

Tests should work if run multiple times:

```go
ginkgo.It("should create ConfigMap", func() {
    cm := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name: "test-cm",
        },
        Data: map[string]string{
            "key": "value",
        },
    }
    
    // Try to create, ignore if already exists
    _, err := oc.KubeClient().CoreV1().ConfigMaps(oc.Namespace()).
        Create(context.Background(), cm, metav1.CreateOptions{})
    if err != nil && !errors.IsAlreadyExists(err) {
        gomega.Expect(err).NotTo(gomega.HaveOccurred())
    }
})
```

### 5. Use Context Helpers

```go
// Get admin config client
configClient := oc.AdminConfigClient()

// Get kube client
kubeClient := oc.KubeClient()

// Get dynamic client
dynamicClient := oc.AdminDynamicClient()

// Execute in pod
output, err := oc.Run("exec").Args("pod-name", "--", "ls", "/").Output()
```

## Complete Example

```go
package e2e

import (
    "context"
    "time"
    
    "github.com/onsi/ginkgo/v2"
    "github.com/onsi/gomega"
    
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/types"
    
    machineconfigv1 "github.com/openshift/api/machineconfiguration/v1"
    exutil "github.com/openshift/origin/test/extended/util"
)

var _ = ginkgo.Describe("[sig-machineconfig] MachineConfigDaemon", func() {
    defer ginkgo.GinkgoRecover()
    
    var (
        oc   *exutil.CLI
        mc   *machineconfigv1.MachineConfig
        pool *machineconfigv1.MachineConfigPool
    )
    
    ginkgo.BeforeEach(func() {
        oc = exutil.NewCLI("mcd-test")
        
        // Get worker pool
        var err error
        pool, err = oc.AdminConfigClient().MachineconfigurationV1().
            MachineConfigPools().Get(context.Background(), "worker", metav1.GetOptions{})
        gomega.Expect(err).NotTo(gomega.HaveOccurred())
    })
    
    ginkgo.AfterEach(func() {
        if mc != nil {
            oc.AdminConfigClient().MachineconfigurationV1().
                MachineConfigs().Delete(context.Background(), 
                    mc.Name, metav1.DeleteOptions{})
            
            // Wait for pool to stabilize
            gomega.Eventually(func() bool {
                p, err := oc.AdminConfigClient().MachineconfigurationV1().
                    MachineConfigPools().Get(context.Background(), 
                        pool.Name, metav1.GetOptions{})
                if err != nil {
                    return false
                }
                return p.Status.UpdatedMachineCount == p.Status.MachineCount &&
                       p.Status.DegradedMachineCount == 0
            }, "10m", "30s").Should(gomega.BeTrue())
        }
    })
    
    ginkgo.It("should update nodes when MachineConfig changes [Slow]", func() {
        // Create MachineConfig
        mc = &machineconfigv1.MachineConfig{
            ObjectMeta: metav1.ObjectMeta{
                Name: "99-worker-test-file",
                Labels: map[string]string{
                    "machineconfiguration.openshift.io/role": "worker",
                },
            },
            Spec: machineconfigv1.MachineConfigSpec{
                Config: ignition.Config{
                    Ignition: ignition.Ignition{
                        Version: "3.2.0",
                    },
                    Storage: ignition.Storage{
                        Files: []ignition.File{
                            {
                                Path: "/etc/test-file",
                                Mode: intPtr(0644),
                                Contents: ignition.FileContents{
                                    Source: "data:,test-content",
                                },
                            },
                        },
                    },
                },
            },
        }
        
        var err error
        mc, err = oc.AdminConfigClient().MachineconfigurationV1().
            MachineConfigs().Create(context.Background(), mc, metav1.CreateOptions{})
        gomega.Expect(err).NotTo(gomega.HaveOccurred())
        
        // Wait for pool to start updating
        gomega.Eventually(func() bool {
            p, err := oc.AdminConfigClient().MachineconfigurationV1().
                MachineConfigPools().Get(context.Background(), 
                    pool.Name, metav1.GetOptions{})
            if err != nil {
                return false
            }
            // Check if Progressing condition is True
            for _, cond := range p.Status.Conditions {
                if cond.Type == machineconfigv1.MachineConfigPoolProgressing && 
                   cond.Status == corev1.ConditionTrue {
                    return true
                }
            }
            return false
        }, "5m", "10s").Should(gomega.BeTrue())
        
        // Wait for update to complete
        gomega.Eventually(func() bool {
            p, err := oc.AdminConfigClient().MachineconfigurationV1().
                MachineConfigPools().Get(context.Background(), 
                    pool.Name, metav1.GetOptions{})
            if err != nil {
                return false
            }
            return p.Status.UpdatedMachineCount == p.Status.MachineCount &&
                   p.Status.DegradedMachineCount == 0
        }, "20m", "30s").Should(gomega.BeTrue())
        
        // Verify file exists on a worker node
        nodes, err := oc.KubeClient().CoreV1().Nodes().List(context.Background(), 
            metav1.ListOptions{
                LabelSelector: "node-role.kubernetes.io/worker",
            })
        gomega.Expect(err).NotTo(gomega.HaveOccurred())
        gomega.Expect(nodes.Items).NotTo(gomega.BeEmpty())
        
        node := nodes.Items[0].Name
        output := debugNodeExec(oc, node, "cat /etc/test-file")
        gomega.Expect(output).To(gomega.ContainSubstring("test-content"))
    })
})

func debugNodeExec(oc *exutil.CLI, node, cmd string) string {
    output, err := oc.Run("debug").Args(
        "node/"+node, "--", "chroot", "/host", "bash", "-c", cmd).Output()
    gomega.Expect(err).NotTo(gomega.HaveOccurred())
    return output
}

func intPtr(i int) *int {
    return &i
}
```

## CI Integration

E2E tests run in Prow CI:

```yaml
# ci-operator config
tests:
- as: e2e-aws
  steps:
    cluster_profile: aws
    test:
    - as: test
      commands: make test-e2e
      from: src
```

See [ci-integration.md](./ci-integration.md) for CI details.

## References

- **openshift-tests**: https://github.com/openshift/origin/tree/master/test
- **Testing Pyramid**: [pyramid.md](./pyramid.md)
- **Ginkgo**: https://onsi.github.io/ginkgo/
- **Gomega**: https://onsi.github.io/gomega/
