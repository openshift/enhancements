# MachineConfig

**Type**: OpenShift Platform API  
**API Group**: `machineconfiguration.openshift.io/v1`  
**Last Updated**: 2026-04-08  

## Overview

MachineConfig defines operating system configuration for OpenShift nodes. Managed by machine-config-operator.

**Key principle**: Nodes are immutable. OS changes require reboot.

## API Structure

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfig
metadata:
  labels:
    machineconfiguration.openshift.io/role: worker
  name: 99-worker-custom
spec:
  kernelArguments:
  - 'systemd.unified_cgroup_hierarchy=0'
  config:
    ignition:
      version: 3.2.0
    storage:
      files:
      - path: /etc/myapp/config.yaml
        mode: 0644
        contents:
          source: data:,example-content
    systemd:
      units:
      - name: myapp.service
        enabled: true
        contents: |
          [Unit]
          Description=My App
          [Service]
          ExecStart=/usr/local/bin/myapp
```

## MachineConfigPool

Groups nodes and manages rollout:

```yaml
apiVersion: machineconfiguration.openshift.io/v1
kind: MachineConfigPool
metadata:
  name: worker
spec:
  machineConfigSelector:
    matchLabels:
      machineconfiguration.openshift.io/role: worker
  nodeSelector:
    matchLabels:
      node-role.kubernetes.io/worker: ""
  maxUnavailable: 1  # Rolling update
status:
  machineCount: 3
  updatedMachineCount: 3
  readyMachineCount: 3
  degradedMachineCount: 0
```

## When to Use

- ✅ OS-level configuration (sysctls, kernel args)
- ✅ System files (/etc, /usr/local)
- ✅ Systemd units
- ❌ Application config (use ConfigMap)
- ❌ Workload management (use Deployment)

## Update Process

1. MachineConfig created/updated
2. MCO renders combined config for pool
3. MCD on each node detects change
4. Node cordoned, drained
5. Config applied, node rebooted
6. Node uncordoned

## References

- **Machine Config Operator**: https://github.com/openshift/machine-config-operator
- **API**: https://github.com/openshift/api/blob/master/machineconfiguration/v1/types_machineconfig.go
