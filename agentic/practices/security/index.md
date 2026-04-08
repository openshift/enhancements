# Security Practices Index

**Last Updated**: 2026-04-08  

## Overview

Security practices and guidelines for all OpenShift components.

## Core Practices

| Practice | Purpose | File |
|----------|---------|------|
| Threat Modeling | STRIDE security analysis | [threat-modeling.md](./threat-modeling.md) |
| RBAC Guidelines | Least privilege access control | [rbac-guidelines.md](./rbac-guidelines.md) |
| Secrets Management | Secure credential handling | [secrets-management.md](./secrets-management.md) |

## Quick Reference

### STRIDE Framework

- **S**poofing - Can an attacker impersonate?
- **T**ampering - Can data be modified?
- **R**epudiation - Can actions be denied?
- **I**nformation Disclosure - Can data be leaked?
- **D**enial of Service - Can availability be disrupted?
- **E**levation of Privilege - Can permissions be escalated?

See [threat-modeling.md](./threat-modeling.md)

### RBAC Principles

- Grant minimum permissions required (least privilege)
- Use Role for namespace-scoped, ClusterRole for cluster-scoped
- One ServiceAccount per component
- Never grant `cluster-admin`, `verbs: ["*"]`, or `resources: ["*"]`

See [rbac-guidelines.md](./rbac-guidelines.md)

### Secret Handling

- Prefer volume mounts over environment variables
- Never log secret values
- Rotate credentials regularly
- Use external secret stores when possible
- Enable etcd encryption at rest

See [secrets-management.md](./secrets-management.md)

## Common Patterns

### ServiceAccount with Least Privilege

```yaml
apiVersion: v1
kind: ServiceAccount
metadata:
  name: my-operator
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: my-operator
rules:
- apiGroups: ["myapi.openshift.io"]
  resources: ["myresources"]
  verbs: ["get", "list", "watch", "update"]
```

### Secret as Volume Mount

```yaml
volumeMounts:
- name: api-credentials
  mountPath: /etc/secrets
  readOnly: true
volumes:
- name: api-credentials
  secret:
    secretName: api-credentials
```

### Redacted Logging

```go
log.Info("Config loaded", "apiKeyPresent", config.APIKey != "")
// NOT: log.Info("Config", "apiKey", config.APIKey)
```

## See Also

- [Testing Practices](../testing/) - Security testing
- [Reliability](../reliability/) - Security monitoring
- [Development Practices](../development/) - Secure code review
