# Machine

**Type**: OpenShift Platform API  
**API Group**: `machine.openshift.io/v1beta1`  
**Last Updated**: 2026-04-08  

## Overview

Machine represents a node in the cluster, managed by machine-api-operator. Provides declarative node lifecycle management.

## API Structure

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: Machine
metadata:
  name: worker-1
  namespace: openshift-machine-api
spec:
  providerSpec:
    value:
      # Cloud-specific configuration
      apiVersion: machine.openshift.io/v1beta1
      kind: AWSMachineProviderConfig
      instanceType: m5.large
      ami:
        id: ami-12345
status:
  phase: Running  # Provisioning, Running, Deleting, Failed
  providerStatus:
    instanceId: i-1234567890
  nodeRef:
    name: ip-10-0-1-100.ec2.internal
```

## Machine Lifecycle

1. **Machine created** → phase: Provisioning
2. **Cloud instance created** → providerStatus populated
3. **Node joins cluster** → nodeRef set, phase: Running
4. **Machine deleted** → phase: Deleting, instance terminated
5. **Machine removed** → Node removed from cluster

## MachineSet

Manages a set of Machines (like Deployment for Pods):

```yaml
apiVersion: machine.openshift.io/v1beta1
kind: MachineSet
metadata:
  name: worker-us-west-2a
spec:
  replicas: 3
  selector:
    matchLabels:
      machine.openshift.io/cluster-api-machine-role: worker
  template:
    spec:
      providerSpec:
        value:
          # Cloud config
```

## Autoscaling

```yaml
apiVersion: autoscaling.openshift.io/v1beta1
kind: MachineAutoscaler
metadata:
  name: worker-us-west-2a
spec:
  minReplicas: 1
  maxReplicas: 10
  scaleTargetRef:
    apiVersion: machine.openshift.io/v1beta1
    kind: MachineSet
    name: worker-us-west-2a
```

## References

- **Machine API**: https://github.com/openshift/machine-api-operator
- **API**: https://github.com/openshift/api/blob/master/machine/v1beta1/types_machine.go
