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

OpenShift extends Kubernetes PodSecurity with SCCs. The default SCC varies by version:
- `restricted` — legacy default (pre-4.11 installations)
- `restricted-v2` — default on 4.11+ new installations; `restricted` is not available to users unless explicitly granted
- `restricted-v3` — introduced in later versions with further constraints
- `anyuid` — allows running as any UID (avoid unless required)
- `privileged` — full host access (only for infrastructure components)

**Convention**: Workloads that need a pinned SCC may opt in by declaring the `openshift.io/required-scc` annotation. This is not required on every workload — only those that need a specific SCC beyond the default.

## Secret Management

- Secrets must never be logged or included in status conditions
- Use Kubernetes Secrets or external secret stores (e.g., HashiCorp Vault)
- Rotate credentials on a regular schedule
- TLS certificates should use OpenShift's built-in certificate management where possible

## Threat Modeling

Use STRIDE (Spoofing, Tampering, Repudiation, Information Disclosure, Denial of Service, Elevation of Privilege) for enhancement security assessments. The enhancement template includes a "Security" section for this purpose.

## Platform Documentation

For enhancement security sections, see [guidelines/enhancement_template.md](../../../guidelines/enhancement_template.md). For operator RBAC patterns, see component-specific documentation.
