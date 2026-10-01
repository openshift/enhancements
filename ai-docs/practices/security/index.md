# Security Practices

Security patterns and threat modeling for OpenShift components.

## Guidance

Security practices are primarily documented in:
- [../../dev-guide/](../../../dev-guide/) - Security conventions
- [OpenShift Security Guide](https://docs.openshift.com/container-platform/latest/security/index.html)

## Key Security Patterns

| Pattern | When to Use | Reference |
|---------|-------------|-----------|
| **RBAC** | API access control | [RBAC Docs](https://kubernetes.io/docs/reference/access-authn-authz/rbac/) |
| **SCCs** | Pod security | [SCC Guide](https://docs.openshift.com/container-platform/latest/authentication/managing-security-context-constraints.html) |
| **Secret Handling** | Credentials, tokens | Never log/expose secrets |
| **Network Policies** | Network segmentation | [NetworkPolicy](https://kubernetes.io/docs/concepts/services-networking/network-policies/) |

## Threat Modeling (STRIDE)

| Threat | Mitigation |
|--------|------------|
| **Spoofing** | Use service accounts, RBAC |
| **Tampering** | Immutable infrastructure, admission controllers |
| **Repudiation** | Audit logs, structured logging |
| **Information Disclosure** | Secret encryption, RBAC |
| **Denial of Service** | Resource limits, rate limiting |
| **Elevation of Privilege** | RBAC, SCCs, least privilege |

## Related

- **Dev Guide**: [../../dev-guide/](../../../dev-guide/)
- **Platform Patterns**: [../../platform/](../../platform/)
