# must-gather

**Category**: Operator Pattern
**Last Updated**: 2026-10-23

---

## Overview

`must-gather` is OpenShift's diagnostic data collection framework. Each operator provides a must-gather image that collects component-specific logs, resources, and configuration for debugging.

## How It Works

```bash
# Collect all platform diagnostics
oc adm must-gather

# Collect from a specific operator's must-gather image
oc adm must-gather --image=quay.io/openshift/must-gather:latest

# Collect from multiple sources
oc adm must-gather --image=image1 --image=image2
```

The `oc adm must-gather` command launches a pod in the cluster that runs the collection scripts and copies the results to the local machine.

## What to Collect

A component's must-gather should collect:

| Category | Examples |
|----------|---------|
| **Custom Resources** | Operator CR instances, status, conditions |
| **Logs** | Operator pod logs, operand logs |
| **Events** | Namespace events related to the component |
| **Configuration** | ConfigMaps, Secrets (redacted), environment |
| **Cluster State** | Nodes, machine status (if relevant) |

## Key Conventions

1. **Never collect secrets in plaintext** — redact sensitive values
2. **Use namespaced collection** — collect only from relevant namespaces
3. **Include recent events** — events provide timeline context for debugging
4. **Collect both operator and operand logs** — the operator's view and the operand's behavior often differ
5. **Note**: must-gather creates namespaces with an `openshift-` prefix and randomly generated suffix — tests must exclude these namespaces

## Creating a must-gather Image

Must-gather images are typically defined in the operator's repository under a `must-gather/` directory with a `Dockerfile` and collection scripts. The image is registered in the release payload.

## Platform Documentation

For operator development conventions, see [dev-guide/operators.md](../../../dev-guide/operators.md). For debugging techniques, see component-specific `ai-docs/DEVELOPMENT.md`.
