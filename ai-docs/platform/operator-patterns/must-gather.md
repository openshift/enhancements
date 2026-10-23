# must-gather Pattern

**Category**: Platform Pattern  
**Applies To**: All ClusterOperators and platform components  
**Last Updated**: 2026-10-23  

## Overview

`must-gather` is OpenShift's standard diagnostic data collection framework. Every operator should provide a must-gather image that collects operator-specific logs, resources, and configuration for debugging.

**Purpose**: Enable offline debugging by collecting all relevant data in a single command.

## Key Concepts

- **must-gather image**: Container image with collection scripts
- **oc adm must-gather**: CLI command that runs collection
- **Collection scripts**: Bash scripts that gather logs, resources, events
- **Artifact directory**: Structured output for analysis

## How It Works

```text
oc adm must-gather --image=<operator-must-gather-image>
    → Launches pod with collection scripts
    → Scripts gather logs, resources, events
    → Artifacts copied to local directory
    → Pod cleaned up
```

## Implementation

### Collection Script Structure
```bash
#!/bin/bash
# gather_my_operator - must-gather collection script

# Collect operator logs
oc logs -n openshift-my-operator deployment/my-operator --all-containers --since=72h \
    > "${ARTIFACT_DIR}/operator-logs.log" 2>&1

# Collect custom resources
oc get myresources -A -o yaml \
    > "${ARTIFACT_DIR}/myresources.yaml" 2>&1

# Collect events
oc get events -n openshift-my-operator --sort-by='.lastTimestamp' \
    > "${ARTIFACT_DIR}/events.txt" 2>&1

# Collect related resources
oc get configmaps -n openshift-my-operator -o yaml \
    > "${ARTIFACT_DIR}/configmaps.yaml" 2>&1

# Collect ClusterOperator status
oc get co my-operator -o yaml \
    > "${ARTIFACT_DIR}/clusteroperator.yaml" 2>&1
```

### Dockerfile
```dockerfile
FROM quay.io/openshift/origin-must-gather:latest
# The CLI runs /usr/bin/gather by default, so the script must be
# installed at that path (or the ENTRYPOINT must be overridden).
COPY gather_my_operator /usr/bin/gather
ENTRYPOINT /usr/bin/gather
```

## What to Collect

| Category | What to Gather | Why |
|----------|---------------|-----|
| **Operator logs** | All containers, last 72h | Debug operator behavior |
| **Custom resources** | All CRs managed by the operator | Current desired state |
| **Events** | Operator namespace events | Recent state changes |
| **ConfigMaps/Secrets** | Operator configuration (not secret values) | Configuration state |
| **ClusterOperator** | Status and conditions | Health summary |
| **Related resources** | Deployments, DaemonSets, Pods | Workload state |

## Best Practices

1. **Collect Broadly**: Gather more than you think you need — missing data means another round-trip
2. **Time-bound Logs**: Use `--since=72h` to keep artifacts manageable
3. **Handle Failures**: Use `2>&1` and `|| true` so collection continues on partial failure
4. **Structured Output**: Use consistent directory layout for artifacts
5. **No Secrets**: Never collect Secret values; collect Secret metadata only

## Running must-gather

```bash
# Default platform must-gather
oc adm must-gather

# Operator-specific must-gather
oc adm must-gather --image=quay.io/openshift/my-operator-must-gather:latest

# Multiple must-gather images
oc adm must-gather \
    --image=quay.io/openshift/must-gather:latest \
    --image=quay.io/openshift/my-operator-must-gather:latest
```

## Antipatterns

❌ **No must-gather image**: Operator has no diagnostic collection at all  
❌ **Collecting secrets**: Including Secret data in must-gather output  
❌ **Unbounded logs**: Collecting all logs without time limits  
❌ **Failing silently**: Script exits on first error, missing remaining data  
❌ **Missing CRs**: Not collecting the operator's custom resources

## References

- **Tool**: [oc adm must-gather](https://docs.openshift.com/container-platform/latest/support/gathering-cluster-data.html)
- **Base Image**: [origin-must-gather](https://github.com/openshift/must-gather)
- **Pattern**: Supports debugging for [status-conditions.md](./status-conditions.md) issues
