# API Evolution

**Category**: Development Practice
**Authoritative Source**: [dev-guide/api-conventions.md](../../../dev-guide/api-conventions.md)
**Last Updated**: 2026-10-23

---

## Overview

OpenShift APIs are supported immediately once merged. API evolution must preserve backward compatibility while allowing the platform to improve. This guide summarizes the key principles; the authoritative reference is `dev-guide/api-conventions.md`.

## Configuration vs Workload APIs

| Aspect | Configuration API | Workload API |
|--------|------------------|--------------|
| **Scope** | Cluster-scoped, singleton | Namespaced, many instances |
| **Managed by** | Cluster administrator | End users |
| **Defaulting** | Controller-side (can evolve) | API-side (immutable once set) |
| **Example** | Infrastructure, Network | Deployment, DaemonSet |

**Key rule**: Configuration API defaults live in the controller, NOT the API schema. This allows the platform to change defaults during upgrades without breaking existing resources.

## Compatibility Requirements

1. **No breaking changes** to GA APIs — ever. New fields must be optional with backward-compatible defaults
2. **Conversion strategy varies by API type** — most OpenShift APIs use internal or aggregated-server conversion rather than CRD conversion webhooks, but exceptions exist (e.g., `ConsoleDynamicPlugin`). Check the specific API
3. **Pointer avoidance** — avoid pointer fields in CRD-based APIs unless there is an absolute need (distinguishing unset from zero-value)
4. **Validation via CEL** — preferred over webhooks for new APIs; see `dev-guide/api-conventions.md`

## Version Progression

```
v1alpha1 → v1beta1 → v1 (GA)
```

- **Alpha**: May change or be removed between releases. Behind TechPreviewNoUpgrade feature gate
- **Beta**: Deprecated in OpenShift; new APIs should go straight from alpha to GA
- **GA (v1)**: Supported indefinitely. Breaking changes within the same non-beta version are bugs. Transitioning to a new API requires deprecating the existing API and supporting it across upgrades

## Common API Evolution Mistakes

- Adding required fields to an existing API (breaks existing resources)
- Using pointers without justification (complicates JSON merge-patch)
- Defaulting in the schema rather than the controller for configuration APIs
- Changing behavior of an existing field value (breaks user expectations)
- Omitting godoc documentation on optional fields (blocks API review)

## API Review Process

All new and changed OpenShift APIs require review by the API review team before merge. The process is documented in `dev-guide/api-conventions.md`.

**Key convention**: Godoc on optional configuration API fields must describe the default behavior when omitted, with a note that the default is subject to change.

## Platform Documentation

For full API conventions, see [dev-guide/api-conventions.md](../../../dev-guide/api-conventions.md). For the enhancement process for new APIs, see [guidelines/enhancement_template.md](../../../guidelines/enhancement_template.md).
