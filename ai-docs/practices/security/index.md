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

OpenShift Security Context Constraints (SCCs) and Kubernetes Pod Security Admission (PSA) are separate, independent admission mechanisms. Both evaluate pod specs during admission, but they operate independently — OpenShift does not treat SCC as an extension of PSA.

The default SCC varies by OpenShift version and cluster upgrade history:
- `restricted` — legacy default (pre-4.11 installations); not available to users on new 4.11+ clusters unless explicitly granted
- `restricted-v2` — default on new 4.11+ installations through some 4.x releases
- `restricted-v3` — current default as of 4.20+; adds further constraints
- `anyuid` — allows running as any UID (avoid unless required)
- `privileged` — full host access (only for infrastructure components)

Clusters upgraded from earlier versions may retain their original default SCC. Check the actual cluster configuration rather than assuming a specific default based on version alone.

**Convention**: The `openshift.io/required-scc` annotation is opt-in. Workloads can use it to pin a specific SCC (ensuring a known security context), to prevent a different SCC from being selected by the admission controller, or to request an SCC other than the cluster default. It is not required on every workload.

## Secret Management

- Secrets must never be logged or included in status conditions
- Use Kubernetes Secrets or external secret stores (e.g., HashiCorp Vault)
- Rotate credentials on a regular schedule
- TLS certificates should use OpenShift's built-in certificate management where possible

## Threat Modeling

Use STRIDE (Spoofing, Tampering, Repudiation, Information Disclosure, Denial of Service, Elevation of Privilege) for enhancement security assessments. The enhancement template includes a "Security" section for this purpose.

## Platform Documentation

For enhancement security sections, see [guidelines/enhancement_template.md](../../../guidelines/enhancement_template.md). For operator RBAC patterns, see component-specific documentation.
