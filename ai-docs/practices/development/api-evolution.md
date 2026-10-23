# API Evolution

**Category**: Engineering Practice  
**Applies To**: All OpenShift components with APIs  
**Last Updated**: 2026-10-23  

## Overview

API evolution in OpenShift follows Kubernetes API conventions. APIs progress through maturity levels (alpha → beta → stable) and must maintain backward compatibility within supported version skew.

**Principle**: APIs are contracts. Breaking changes break users.

## API Maturity Levels

| Level | Stability | Support Window | Feature Gates |
|-------|----------|---------------|---------------|
| **v1alpha1** | Experimental | May change/remove between releases | TechPreviewNoUpgrade |
| **v1beta1** | Pre-release | Backward-compatible changes only | May require gate |
| **v1** | Stable | No breaking changes; deprecation required | None (GA) |

## Version Transition Pattern

OpenShift transitions API versions across minor releases without serving multiple versions simultaneously:

```text
Release N:   Serve v1alpha1 (storage: v1alpha1)
Release N+1: Serve v1 + v1alpha1 (storage: v1, v1alpha1 deprecated)
Release N+2: Serve v1 only (v1alpha1 removed)
```

**OpenShift avoids**:
- ❌ Conversion webhooks (complexity rarely justified)
- ❌ Long-term multi-version serving (increases API surface)

## Backward-Compatible Changes

### Allowed
```go
// ✅ Adding optional field (pointer type with omitempty)
type MySpec struct {
    Replicas int      `json:"replicas"`
    NewField *string  `json:"newField,omitempty"` // New, optional
}

// ✅ Adding new enum value
// ✅ Widening validation (accepting more values)
// ✅ Adding new status fields
// ✅ Adding new subresources
```

### Not Allowed
```go
// ❌ Removing existing field
// ❌ Changing field type
// ❌ Narrowing validation (rejecting previously valid values)
// ❌ Changing default values for existing fields
// ❌ Renaming fields (JSON tag changes)
```

## Deprecation Process

1. **Release N**: Mark field/version as deprecated (add `// Deprecated:` comment)
2. **Release N+1**: Log warnings when deprecated field is used
3. **Release N+2** (earliest): Remove deprecated field/version

**Minimum**: 2 release deprecation window before removal.

## Best Practices

1. **Use Pointer Types**: Optional fields must be pointers (`*string`, not `string`)
2. **Omitempty Tag**: All optional fields need `json:"field,omitempty"`
3. **Validation Webhooks**: Enforce constraints beyond OpenAPI schema
4. **Status Subresource**: Always use status subresource for status updates
5. **API Review**: All API changes require API review per [conventions](https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md)

## Enhancement Requirement

All API changes require an enhancement proposal. See `./guidelines/enhancement_template.md` for the template and `./dev-guide/api-conventions.md` for conventions.

## References

- **Conventions**: [OpenShift API Conventions](https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md)
- **Upstream**: [Kubernetes API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- **API Types**: [openshift/api](https://github.com/openshift/api)
- **Pattern**: Related to [upgrade-strategies.md](../../platform/openshift-specifics/upgrade-strategies.md) for version skew handling
