# ClusterVersion

**Type**: OpenShift Platform API  
**API Group**: `config.openshift.io/v1`  
**Last Updated**: 2026-04-08  

## Overview

ClusterVersion represents the desired and current version of the OpenShift cluster, managed by CVO.

## API Structure

```yaml
apiVersion: config.openshift.io/v1
kind: ClusterVersion
metadata:
  name: version  # Singleton
spec:
  channel: stable-4.16
  clusterID: abc123
  desiredUpdate:
    version: 4.16.1
    image: quay.io/openshift-release-dev/ocp-release@sha256:...
status:
  desired:
    version: 4.16.1
    image: quay.io/openshift-release-dev/ocp-release@sha256:...
  history:
  - version: 4.16.1
    state: Completed
    startedTime: "2024-01-15T10:00:00Z"
    completionTime: "2024-01-15T10:45:00Z"
  conditions:
  - type: Available
    status: "True"
  - type: Progressing
    status: "False"
```

## Upgrade Process

### 1. Check Available Updates

```bash
oc get clusterversion version -o jsonpath='{.status.availableUpdates}'
```

### 2. Trigger Upgrade

```bash
oc adm upgrade --to=4.16.1
```

### 3. Monitor Progress

```bash
oc get clusterversion
oc get clusteroperators
```

## Update Channels

| Channel | Purpose |
|---------|---------|
| **stable-4.16** | Production use |
| **fast-4.16** | Early access |
| **candidate-4.16** | Pre-release |
| **eus-4.16** | Extended Update Support |

## Version Skew

- ✅ Supported: N → N+1 (4.15 → 4.16)
- ❌ Not supported: N → N+2 (4.15 → 4.17)
- ✅ Patch updates: Always (4.16.0 → 4.16.5)

## References

- **CVO**: https://github.com/openshift/cluster-version-operator
- **Upgrades**: [../../platform/operator-patterns/upgrade-strategies.md](../../platform/operator-patterns/upgrade-strategies.md)
- **API**: https://github.com/openshift/api/blob/master/config/v1/types_cluster_version.go
