# must-gather Pattern

**Category**: Platform Pattern  
**Applies To**: All ClusterOperators  
**Last Updated**: 2026-04-08  

## Overview

must-gather is the standardized diagnostic data collection pattern for OpenShift operators. It collects logs, resource manifests, and operator-specific debug data for support cases.

## Why must-gather

**Problem**: Debugging requires logs from multiple pods, resource states, custom diagnostics - manual collection is error-prone.

**Solution**: Automated data collection via `oc adm must-gather` command. Support teams get complete diagnostic bundle.

## Basic Collection

```bash
# Collect cluster-wide diagnostics
oc adm must-gather

# Collect operator-specific diagnostics
oc adm must-gather --image=quay.io/openshift/origin-must-gather:latest

# Collect from specific operator
oc adm must-gather --image=registry.redhat.io/openshift4/ose-machine-config-operator-must-gather:v4.15
```

## Implementation

### Directory Structure

```
must-gather/
├── Dockerfile
├── collection-scripts/
│   ├── gather                     # Main entry point
│   ├── gather_audit_logs
│   ├── gather_nodes
│   └── gather_<component>
└── README.md
```

### Main gather Script

```bash
#!/bin/bash
# must-gather/collection-scripts/gather

# Output directory (provided by oc adm must-gather)
BASE_COLLECTION_PATH="${BASE_COLLECTION_PATH:-/must-gather}"
mkdir -p "${BASE_COLLECTION_PATH}"

# Collect ClusterOperator status
oc adm inspect clusteroperator/machine-config --dest-dir="${BASE_COLLECTION_PATH}"

# Collect operator namespace resources
oc adm inspect ns/openshift-machine-config-operator --dest-dir="${BASE_COLLECTION_PATH}"

# Collect CRs managed by operator
oc adm inspect machineconfigs --dest-dir="${BASE_COLLECTION_PATH}"
oc adm inspect machineconfigpools --dest-dir="${BASE_COLLECTION_PATH}"

# Collect node information
oc adm inspect nodes --dest-dir="${BASE_COLLECTION_PATH}"

# Collect logs from operator pods
oc logs --all-containers=true -n openshift-machine-config-operator -l k8s-app=machine-config-operator \
  > "${BASE_COLLECTION_PATH}/machine-config-operator.log"

# Run custom diagnostics
/usr/bin/gather_node_configs
/usr/bin/gather_machine_state

# Collect from all nodes (runs on each node)
oc adm node-logs --role=master --path=journal/machine-config-daemon.log \
  > "${BASE_COLLECTION_PATH}/machine-config-daemon-master.log"

echo "Collection complete: ${BASE_COLLECTION_PATH}"
```

### Dockerfile

```dockerfile
FROM registry.access.redhat.com/ubi9/ubi:latest

# Install oc CLI
COPY --from=registry.redhat.io/openshift4/ose-cli:v4.15 /usr/bin/oc /usr/bin/oc

# Copy collection scripts
COPY collection-scripts/ /usr/bin/

# Set gather as entrypoint
ENTRYPOINT ["/usr/bin/gather"]
```

## What to Collect

### Essential Data

| Category | What to Collect | Command |
|----------|----------------|---------|
| Operator status | ClusterOperator CR | `oc adm inspect clusteroperator/<name>` |
| Operator namespace | All resources in operator namespace | `oc adm inspect ns/<namespace>` |
| Managed CRs | CustomResources operator manages | `oc adm inspect <cr-type>` |
| Logs | Operator pod logs | `oc logs -n <namespace> -l <selector>` |
| Events | Namespace events | `oc get events -n <namespace>` |

### Operator-Specific Data

```bash
# Machine Config Operator
oc adm inspect machineconfigs,machineconfigpools,controllerconfigs
oc adm node-logs --role=master --path=journal/machine-config-daemon.log
oc adm node-logs --role=worker --path=journal/kubelet.log

# Network Operator
oc adm inspect networks,clusternetworks,hostsubnets
oc logs -n openshift-network-operator deployment/network-operator
oc logs -n openshift-sdn daemonset/sdn

# Machine API Operator
oc adm inspect machines,machinesets,machinehealthchecks
oc logs -n openshift-machine-api deployment/machine-api-operator
```

### Node Data Collection

```bash
# Run on each node
oc adm node-logs <node> --path=<path>

# Common paths
--path=journal/kubelet.log
--path=journal/crio.log
--path=journal/machine-config-daemon.log
--path=/var/log/messages
```

## Advanced Patterns

### Conditional Collection

```bash
#!/bin/bash
# Only collect if specific condition exists

if oc get machineconfig -o json | jq -e '.items[] | select(.spec.osImageURL != "")' > /dev/null; then
    echo "Custom OS images detected, collecting additional data..."
    oc adm inspect machineconfigs --dest-dir="${BASE_COLLECTION_PATH}/custom-os"
fi
```

### Parallel Collection

```bash
#!/bin/bash
# Collect from multiple sources in parallel

(
    oc adm inspect nodes --dest-dir="${BASE_COLLECTION_PATH}/nodes"
) &

(
    oc adm inspect machineconfigs --dest-dir="${BASE_COLLECTION_PATH}/mcs"
) &

wait  # Wait for all background jobs
```

### Timeout Protection

```bash
#!/bin/bash
# Prevent long-running collection

timeout 300 oc adm inspect nodes --dest-dir="${BASE_COLLECTION_PATH}/nodes" || {
    echo "ERROR: Node collection timed out after 5 minutes" >&2
}
```

## Best Practices

1. **Namespace everything**: Use `--dest-dir` to organize output
2. **Include timestamps**: Help correlate events across components
3. **Sanitize secrets**: Never collect Secret data in plaintext
4. **Set timeouts**: Prevent hung collection commands
5. **Document output**: Include README in must-gather image
6. **Test regularly**: Ensure collection works in degraded states
7. **Keep focused**: Only collect relevant data for your operator

## Sanitizing Secrets

```bash
# DON'T collect raw secrets
# ❌ oc get secrets -o yaml > secrets.yaml

# DO collect secret metadata only
oc get secrets -o custom-columns=NAME:.metadata.name,TYPE:.type,AGE:.metadata.creationTimestamp

# DO sanitize sensitive data
oc get secret my-secret -o yaml | \
  sed 's/^\([[:space:]]*data:.*\)/  data: <REDACTED>/' \
  > secret-sanitized.yaml
```

## Testing must-gather

```bash
# Build must-gather image
podman build -t my-operator-must-gather:latest must-gather/

# Test locally
oc adm must-gather --image=my-operator-must-gather:latest

# Check output
ls -la must-gather.local.*/
```

## must-gather Output Structure

```
must-gather.local.<timestamp>/
├── cluster-scoped-resources/
│   ├── config.openshift.io/
│   │   └── clusteroperators/
│   │       └── machine-config.yaml
│   └── core/
│       └── nodes/
│           └── master-0.yaml
├── namespaces/
│   └── openshift-machine-config-operator/
│       ├── pods/
│       │   └── machine-config-operator-xxx.yaml
│       ├── deployments/
│       └── logs/
│           └── machine-config-operator.log
└── custom-diagnostics/
    └── node-configs.json
```

## Integration with Support

Support engineers use must-gather data:

```bash
# Extract must-gather bundle
tar xzf must-gather-bundle.tar.gz

# Analyze with insights
insights-client --analyze must-gather.local.*/

# Search for errors
grep -r "error\|Error\|ERROR" must-gather.local.*/

# Check operator status
cat must-gather.local.*/cluster-scoped-resources/config.openshift.io/clusteroperators/*.yaml
```

## Examples in Components

| Component | must-gather Image | Special Collection |
|-----------|-------------------|-------------------|
| machine-config-operator | ose-machine-config-operator-must-gather | Node journals, MachineConfigs |
| cluster-network-operator | ose-network-tools-must-gather | OVN databases, network policies |
| machine-api-operator | ose-machine-api-operator-must-gather | Machine provider state |
| storage | ose-csi-driver-must-gather | CSI driver logs, PV/PVC states |

## Common Pitfalls

1. **Collecting too much**: Gigabytes of logs - filter to relevant timeframe
2. **Missing node data**: Forgetting `oc adm node-logs` for node-level issues
3. **No error handling**: Collection fails silently - add error checks
4. **Hardcoded paths**: Use `${BASE_COLLECTION_PATH}` variable
5. **Incomplete on failures**: Operator degraded → collection partially fails
6. **Secret leakage**: Accidentally collecting sensitive data

## Debugging Collection Failures

```bash
# Run must-gather with verbose output
oc adm must-gather --image=<image> --log-level=6

# Check must-gather pod logs
oc logs -n openshift-must-gather-<hash> <pod-name>

# Run collection manually
oc debug node/<node> -- chroot /host bash -c "cat /var/log/messages"
```

## References

- **oc adm must-gather**: https://docs.openshift.com/container-platform/latest/support/gathering-cluster-data.html
- **must-gather Images**: https://github.com/openshift/must-gather
- **Diagnostics**: [observability.md](../../practices/reliability/observability.md)
