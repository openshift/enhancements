# Security Practices

**Category**: Security
**Last Updated**: 2026-10-23

---

## Overview

OpenShift security practices encompass RBAC patterns, SCC (Security Context Constraints), secret management, and threat modeling. Component operators must follow least-privilege principles.

## RBAC Guidelines

- Operators should request **minimum required permissions** via their ClusterRole
- Use separate ServiceAccounts for operands vs operators
- Never bind `cluster-admin` to operator ServiceAccounts
- Use `oc adm policy` to audit RBAC bindings

## Security Context Constraints (SCC)

OpenShift extends Kubernetes PodSecurity with SCCs:
- `restricted` — default for workloads; no privilege escalation
- `anyuid` — allows running as any UID (avoid unless required)
- `privileged` — full host access (only for infrastructure components)

**Convention**: Workloads must declare their required SCC via the `openshift.io/required-scc` annotation.

## Secret Management

- Secrets must never be logged or included in status conditions
- Use Kubernetes Secrets or external secret stores (e.g., HashiCorp Vault)
- Rotate credentials on a regular schedule
- TLS certificates should use OpenShift's built-in certificate management where possible

## Threat Modeling

Use STRIDE (Spoofing, Tampering, Repudiation, Information Disclosure, Denial of Service, Elevation of Privilege) for enhancement security assessments. The enhancement template includes a "Security" section for this purpose.

## Platform Documentation

For enhancement security sections, see [guidelines/enhancement_template.md](../../../guidelines/enhancement_template.md). For operator RBAC patterns, see component-specific documentation.
