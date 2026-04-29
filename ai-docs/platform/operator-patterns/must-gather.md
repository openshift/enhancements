# Must-Gather Pattern

**Category**: Platform Pattern  
**Applies To**: All Operators  
**Last Updated**: 2026-04-29  

## Overview

must-gather is OpenShift's debugging tool for collecting diagnostic data. Operators should provide must-gather scripts to aid in troubleshooting production issues.

**Purpose**: Collect logs, resources, and diagnostics for support cases.

## Key Concepts

- **must-gather**: Command-line tool that runs scripts in a pod
- **Gather Script**: Shell script that collects data
- **Image**: Container image with gather scripts
- **Output**: Tarball with collected data

## must-gather Workflow

```
User runs:
  oc adm must-gather --image=<operator-must-gather-image>

↓

1. Creates temporary namespace
2. Runs gather pod with image
3. Executes gather scripts in pod
4. Copies data from pod to local machine
5. Creates tarball (must-gather.local.XXX/)
6. Deletes temporary namespace
```

## Implementation

### Directory Structure

```
must-gather/
├── Dockerfile
├── collection-scripts/
│   ├── gather                    # Main gather script
│   ├── gather_clusteroperator   # Collect ClusterOperator
│   ├── gather_pods              # Collect pod logs
│   └── gather_custom_resources  # Collect CRs
└── README.md
```

### Main Gather Script

```bash
#!/bin/bash
# collection-scripts/gather

BASE_COLLECTION_PATH="/must-gather"
mkdir -p ${BASE_COLLECTION_PATH}

# Collect namespace resources
oc adm inspect --dest-dir=${BASE_COLLECTION_PATH} \
  --all-namespaces \
  namespace/my-operator-namespace

# Collect custom resources
oc adm inspect --dest-dir=${BASE_COLLECTION_PATH} \
  --all-namespaces \
  myresources.example.com

# Collect ClusterOperator
/usr/bin/gather_clusteroperator

# Collect pod logs
/usr/bin/gather_pods

echo "Collection complete"
exit 0
```

### Collect ClusterOperator

```bash
#!/bin/bash
# collection-scripts/gather_clusteroperator

BASE_COLLECTION_PATH="/must-gather"
OPERATOR_NAME="my-operator"

# Get ClusterOperator
oc get clusteroperator ${OPERATOR_NAME} -o yaml \
  > ${BASE_COLLECTION_PATH}/clusteroperator.yaml

# Get status conditions
oc get clusteroperator ${OPERATOR_NAME} \
  -o jsonpath='{.status.conditions}' | jq . \
  > ${BASE_COLLECTION_PATH}/conditions.json
```

### Collect Pod Logs

```bash
#!/bin/bash
# collection-scripts/gather_pods

BASE_COLLECTION_PATH="/must-gather"
NAMESPACE="my-operator-namespace"

mkdir -p ${BASE_COLLECTION_PATH}/pods

# Get all pods
oc get pods -n ${NAMESPACE} -o yaml \
  > ${BASE_COLLECTION_PATH}/pods/pods.yaml

# Collect logs from each pod
for POD in $(oc get pods -n ${NAMESPACE} -o name); do
  POD_NAME=$(basename ${POD})
  
  # Current logs
  oc logs -n ${NAMESPACE} ${POD_NAME} --all-containers \
    > ${BASE_COLLECTION_PATH}/pods/${POD_NAME}.log 2>&1
  
  # Previous logs (if pod restarted)
  oc logs -n ${NAMESPACE} ${POD_NAME} --all-containers --previous \
    > ${BASE_COLLECTION_PATH}/pods/${POD_NAME}-previous.log 2>&1
done
```

### Dockerfile

```dockerfile
FROM registry.redhat.io/openshift4/ose-cli:latest

# Copy gather scripts
COPY collection-scripts/* /usr/bin/

# Make executable
RUN chmod +x /usr/bin/gather*

# Set entrypoint
ENTRYPOINT ["/usr/bin/gather"]
```

## What to Collect

### Required

| Resource | Why |
|----------|-----|
| **ClusterOperator** | Status conditions, versions |
| **Operator pods** | Current and previous logs |
| **Custom resources** | State of managed resources |
| **Events** | Recent events in operator namespace |

### Optional

| Resource | When to Collect |
|----------|----------------|
| **Deployments/StatefulSets** | Operand state |
| **ConfigMaps** | Configuration (redact secrets!) |
| **Node resources** | For node-level operators |
| **Metrics** | Performance issues |

## Best Practices

1. **Redact Sensitive Data**: Never collect secrets or credentials
   ```bash
   # ❌ Don't collect secrets
   oc get secrets -n my-namespace -o yaml
   
   # ✅ Collect metadata only
   oc get secrets -n my-namespace -o jsonpath='{.items[*].metadata.name}'
   ```

2. **Use `oc adm inspect`**: Purpose-built for gathering
   ```bash
   # Collect namespace resources
   oc adm inspect --dest-dir=/must-gather namespace/my-namespace
   
   # Collect specific resources
   oc adm inspect --dest-dir=/must-gather myresources.example.com
   ```

3. **Organize Output**: Use directories for readability
   ```
   must-gather/
   ├── clusteroperator.yaml
   ├── pods/
   │   ├── operator-xyz.log
   │   └── operator-xyz-previous.log
   ├── custom-resources/
   │   └── myresources.yaml
   └── events.yaml
   ```

4. **Handle Errors Gracefully**: Don't fail entire gather on one error
   ```bash
   # Continue on error
   oc logs pod/xyz > logs.txt 2>&1 || echo "Failed to get logs"
   ```

5. **Add Timestamps**: Help correlate issues
   ```bash
   echo "Collection started: $(date)" > /must-gather/metadata.txt
   ```

## Running must-gather

```bash
# Basic usage
oc adm must-gather --image=quay.io/my-org/my-operator-must-gather:latest

# Specify destination
oc adm must-gather \
  --image=quay.io/my-org/my-operator-must-gather:latest \
  --dest-dir=/tmp/my-gather

# Multiple images (collect from multiple operators)
oc adm must-gather \
  --image=quay.io/openshift/must-gather:latest \
  --image=quay.io/my-org/my-operator-must-gather:latest
```

## Output Structure

```
must-gather.local.XXXXXXXXX/
├── timestamp                           # Collection time
├── version                            # OpenShift version
├── my-operator-namespace/
│   ├── core/
│   │   ├── pods.yaml
│   │   ├── services.yaml
│   │   └── events.yaml
│   ├── apps/
│   │   └── deployments.yaml
│   └── example.com/
│       └── myresources.yaml
├── clusteroperator.yaml
└── pods/
    ├── my-operator-xyz.log
    └── my-operator-xyz-previous.log
```

## Testing must-gather

```bash
# Build image
podman build -t my-must-gather:test must-gather/

# Run locally
oc adm must-gather --image=localhost/my-must-gather:test

# Verify output
ls -R must-gather.local.*/
```

## Examples in Components

| Component | must-gather Repo | Key Collections |
|-----------|------------------|----------------|
| Machine API | [machine-api-must-gather](https://github.com/openshift/machine-api-operator/tree/master/must-gather) | Machines, MachineSets, cloud provider data |
| Network | [cluster-network-operator-must-gather](https://github.com/openshift/cluster-network-operator/tree/master/must-gather) | Network configs, OVN logs |
| Storage | [csi-driver-must-gather](https://github.com/openshift/csi-operator/tree/master/must-gather) | PVs, PVCs, CSI driver logs |

## Integration with Support

**Support cases**: Attach must-gather output to Jira/support cases

**Automated analysis**: Support tools parse must-gather for known issues

**Privacy**: Ensure no customer data or secrets in output

## Antipatterns

❌ **Collecting secrets**: Leaks credentials  
❌ **No error handling**: Entire gather fails on one error  
❌ **Huge output**: Collecting full etcd dump (too large)  
❌ **No organization**: All files in one directory  
❌ **Missing previous logs**: Only collecting current pod logs

## References

- **Command**: `oc adm must-gather --help`
- **OpenShift**: [Gathering cluster data](https://docs.openshift.com/container-platform/latest/support/gathering-cluster-data.html)
- **Example**: [openshift/must-gather](https://github.com/openshift/must-gather)
- **Best Practices**: Use `oc adm inspect` for resource collection
