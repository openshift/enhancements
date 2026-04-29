# API Evolution

**Category**: Engineering Practice  
**Last Updated**: 2026-04-29  

## Overview

Evolving APIs requires backward compatibility. OpenShift follows Kubernetes API conventions for stability and versioning.

**Key Principle**: Never break existing clients.

## API Stability Levels

| Level | Meaning | Breaking Changes Allowed | Removal Allowed |
|-------|---------|-------------------------|-----------------|
| **v1** | Stable | No | No (deprecated only) |
| **v1beta1** | Pre-release | Minimal | After deprecation period |
| **v1alpha1** | Experimental | Yes | Yes |

## Compatibility Rules

### Safe Changes ✅

| Change | Example | Why Safe |
|--------|---------|----------|
| Add optional field | `newField *string` | Old clients ignore unknown fields |
| Add new API version | `v1beta1 → v1` | Conversion preserves old version |
| Add values to enum | `enum: [A, B, C]` | Old clients handle unknown values |
| Deprecate field | `// Deprecated: use newField` | Field still works |

### Breaking Changes ❌

| Change | Example | Why Breaking |
|--------|---------|--------------|
| Remove field | Delete `oldField` | Old clients fail validation |
| Rename field | `replicas → count` | Old clients send wrong field name |
| Change field type | `string → int` | Old clients send wrong type |
| Make optional field required | `*string → string` | Old resources lack required field |
| Remove enum value | `enum: [A, B]` (was `[A, B, C]`) | Old resources have invalid value |

## Versioning Strategy

### Adding New API Version

```go
// v1alpha1 (initial)
type MyResourceSpec struct {
    Replicas int    `json:"replicas"`
    Image    string `json:"image"`
}

// v1beta1 (add features)
type MyResourceSpec struct {
    Replicas int    `json:"replicas"`
    Image    string `json:"image"`
    Strategy string `json:"strategy,omitempty"` // New optional field
}

// v1 (stable)
type MyResourceSpec struct {
    Replicas int    `json:"replicas"`
    Image    string `json:"image"`
    Strategy string `json:"strategy,omitempty"`
}
```

### Deprecating Fields

```go
type MyResourceSpec struct {
    // Deprecated: Use Strategy instead
    // +optional
    OldStrategy string `json:"oldStrategy,omitempty"`
    
    Strategy string `json:"strategy,omitempty"`
}

func (r *Reconciler) reconcile(obj *MyResource) {
    // Handle both old and new fields
    strategy := obj.Spec.Strategy
    if strategy == "" && obj.Spec.OldStrategy != "" {
        strategy = obj.Spec.OldStrategy
    }
}
```

### Deprecation Timeline

```
Version N:   Field announced as deprecated (still works)
Version N+1: Field still works, warnings in logs
Version N+2: Field removed (only if v1alpha1/v1beta1)
```

**Stable APIs (v1)**: Cannot remove deprecated fields (mark deprecated forever)

## Conversion Webhooks

```go
// Hub version (v1)
type MyResource struct {
    Spec MyResourceSpec `json:"spec"`
}

// Spoke version (v1beta1)
func (src *MyResource) ConvertTo(dstRaw conversion.Hub) error {
    dst := dstRaw.(*v1.MyResource)
    
    // Convert fields
    dst.Spec.Replicas = src.Spec.Replicas
    dst.Spec.NewField = convertOldToNew(src.Spec.OldField)
    
    return nil
}

func (dst *MyResource) ConvertFrom(srcRaw conversion.Hub) error {
    src := srcRaw.(*v1.MyResource)
    
    // Reverse conversion
    dst.Spec.Replicas = src.Spec.Replicas
    dst.Spec.OldField = convertNewToOld(src.Spec.NewField)
    
    return nil
}
```

## Default Values

```yaml
# CRD schema with defaults
schema:
  openAPIV3Schema:
    properties:
      spec:
        properties:
          replicas:
            type: integer
            default: 3  # Applied if not specified
          strategy:
            type: string
            default: RollingUpdate
```

**Alternative**: Mutating webhook sets defaults

## Validation

```yaml
# CRD validation
schema:
  openAPIV3Schema:
    properties:
      spec:
        required: ["image"]  # Required field
        properties:
          replicas:
            type: integer
            minimum: 1
            maximum: 100
          tier:
            type: string
            enum: ["dev", "staging", "prod"]
```

## Migration Strategies

### Adding Required Field (Without Breaking)

```go
// Step 1: Add optional field (version N)
type MyResourceSpec struct {
    Image string  `json:"image"`
    Tier  *string `json:"tier,omitempty"` // Optional
}

// Step 2: Set default via webhook (version N)
func (m *Mutator) Default(obj *MyResource) {
    if obj.Spec.Tier == nil {
        tier := "dev"
        obj.Spec.Tier = &tier
    }
}

// Step 3: Make required after all resources migrated (version N+2)
type MyResourceSpec struct {
    Image string `json:"image"`
    Tier  string `json:"tier"` // Now required
}
```

### Renaming Field

```go
// Step 1: Add new field, deprecate old (version N)
type MyResourceSpec struct {
    // Deprecated: Use Count instead
    Replicas *int `json:"replicas,omitempty"`
    Count    *int `json:"count,omitempty"`
}

// Step 2: Handle both in controller
func (r *Reconciler) getCount(obj *MyResource) int {
    if obj.Spec.Count != nil {
        return *obj.Spec.Count
    }
    if obj.Spec.Replicas != nil {
        return *obj.Spec.Replicas
    }
    return 3 // default
}

// Step 3: Remove old field (version N+2, only if not v1)
```

## Storage Version

```yaml
versions:
- name: v1
  served: true
  storage: true  # Stored in etcd as v1
  
- name: v1beta1
  served: true
  storage: false  # Converted to v1 before storing
```

**Migration**: `oc adm migrate storage` to rewrite old versions

## Testing API Changes

```go
func TestBackwardCompatibility(t *testing.T) {
    // Old object (v1beta1)
    old := &v1beta1.MyResource{
        Spec: v1beta1.MyResourceSpec{
            OldField: "value",
        },
    }
    
    // Convert to new version (v1)
    new := &v1.MyResource{}
    old.ConvertTo(new)
    
    // Verify conversion preserves data
    assert.Equal(t, "value", new.Spec.NewField)
    
    // Convert back
    roundtrip := &v1beta1.MyResource{}
    roundtrip.ConvertFrom(new)
    
    // Verify roundtrip preserves data
    assert.Equal(t, old.Spec.OldField, roundtrip.Spec.OldField)
}
```

## Decision Table: Can I Make This Change?

| Current API | Change | Allowed | Alternative |
|-------------|--------|---------|-------------|
| v1 | Remove field | ❌ | Deprecate only |
| v1 | Rename field | ❌ | Add new, keep old deprecated |
| v1 | Add optional field | ✅ | Safe |
| v1 | Add required field | ❌ | Add optional + default |
| v1beta1 | Remove field | ✅ (after deprecation) | Deprecate in N, remove in N+1 |
| v1beta1 | Change type | ✅ (with conversion) | Conversion webhook |
| v1alpha1 | Any change | ✅ | No guarantees |

## References

- **Kubernetes**: [API Conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
- **Dev Guide**: [api-conventions.md](../../../dev-guide/api-conventions.md)
- **OpenShift**: [API Review Process](https://github.com/openshift/api/blob/master/REVIEW.md)
