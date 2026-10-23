# Security Practices

Security patterns and guidelines for OpenShift components.

## Key Areas

### RBAC and Authorization
- Follow least-privilege principle for service accounts
- See [rbac.md](../../platform/operator-patterns/rbac.md) for operator RBAC patterns
- Use Security Context Constraints (SCCs) appropriately

### Secret Management
- Never log or expose secret values
- Use Kubernetes Secrets API for sensitive data
- Rotate credentials on schedule
- Use `must-gather` exclusion patterns for secrets

### Threat Modeling
- Apply STRIDE analysis for new features (Spoofing, Tampering, Repudiation, Information Disclosure, Denial of Service, Elevation of Privilege)
- Document security considerations in enhancement proposals
- Review webhook `failurePolicy` settings

### Container Security
- Run as non-root where possible
- Use read-only root filesystem
- Drop unnecessary capabilities
- Pin image references by digest in production

## OpenShift-Specific

| Topic | Guidance |
|-------|----------|
| **SCCs** | Prefer `restricted-v2` SCC; escalate only when necessary |
| **Service CA** | Use `service.beta.openshift.io/serving-cert-secret-name` for internal TLS |
| **OAuth** | Use OpenShift OAuth proxy for user-facing endpoints |
| **Audit Logging** | Kubernetes audit logs capture API access |

## References

- **Enhancement Template**: Security sections required in all proposals (`./guidelines/enhancement_template.md`)
- **Upstream**: [Kubernetes Security](https://kubernetes.io/docs/concepts/security/)
- **OpenShift Docs**: [Security Context Constraints](https://docs.openshift.com/container-platform/latest/authentication/managing-security-context-constraints.html)
