# API Evolution

**Category**: Engineering Practice  
**Applies To**: All OpenShift API changes  
**Last Updated**: 2026-04-08  

## Overview

Guidelines for evolving Kubernetes and OpenShift APIs while maintaining backward compatibility.

## Versioning

### API Versions

| Version | Stability | Compatibility |
|---------|-----------|---------------|
| **v1alpha1** | Unstable, may change | No guarantees |
| **v1beta1** | Stable API, implementation may change | Backward compatible |
| **v1** | Stable, production-ready | Fully supported |

### Version Progression

```
v1alpha1 → v1beta1 → v1
(tech preview) → (stable, not GA) → (GA)
```

## Backward Compatibility Rules

### NEVER Break These

```yaml
# ❌ Removing fields
spec:
  # oldField: removed  # NEVER

# ❌ Changing field types
spec:
  count: 5  # Was string, now int  # NEVER

# ❌ Changing field semantics
spec:
  mode: "auto"  # Behavior changed  # NEVER

# ❌ Renaming fields
spec:
  # nodeCount: renamed to numNodes  # NEVER
```

### SAFE Changes

```yaml
# ✅ Adding optional fields
spec:
  newField: "value"  # OK if optional

# ✅ Adding new API versions
apiVersion: myapi.openshift.io/v2  # OK, v1 still exists

# ✅ Adding validation
spec:
  field: "value"  # New validation: must be <100 chars  # OK if existing values pass

# ✅ Deprecating (not removing)
spec:
  oldField: "value"  # Deprecated, use newField instead  # OK
```

## Adding Fields

### Optional Fields

```go
type MyResourceSpec struct {
    // Existing field
    Name string `json:"name"`
    
    // New optional field (v2)
    // +optional
    Description string `json:"description,omitempty"`
}
```

### Required Fields with Defaults

```go
type MyResourceSpec struct {
    // New field with default value
    // +kubebuilder:default="auto"
    Mode string `json:"mode"`
}

// Webhook sets default if not provided
func (r *MyResource) Default() {
    if r.Spec.Mode == "" {
        r.Spec.Mode = "auto"
    }
}
```

## Deprecating Fields

### Mark as Deprecated

```go
type MyResourceSpec struct {
    // OldField is deprecated, use NewField instead.
    // +optional
    // +kubebuilder:validation:Deprecated
    OldField string `json:"oldField,omitempty"`
    
    // NewField replaces OldField
    // +optional
    NewField string `json:"newField,omitempty"`
}
```

### Deprecation Timeline

```
v1beta1: Field added
v1beta2: Field marked deprecated (3 releases minimum)
v2: Field removed from new version (v1beta2 still supported)
v2+3 releases: v1beta2 support removed
```

### Handle Both Fields

```go
func (r *Reconciler) getFieldValue(spec MyResourceSpec) string {
    // Prefer new field
    if spec.NewField != "" {
        return spec.NewField
    }
    
    // Fall back to deprecated field
    if spec.OldField != "" {
        log.Info("Using deprecated field 'oldField', migrate to 'newField'")
        return spec.OldField
    }
    
    return "" // Neither set
}
```

## API Version Conversion

### Conversion Webhook

```go
// Hub version (storage version)
type MyResourceV2 struct {}

func (*MyResourceV2) Hub() {}

// Spoke version (converted to/from hub)
type MyResourceV1 struct {}

func (src *MyResourceV1) ConvertTo(dstRaw conversion.Hub) error {
    dst := dstRaw.(*MyResourceV2)
    
    // Convert v1 to v2
    dst.ObjectMeta = src.ObjectMeta
    dst.Spec.NewField = src.Spec.OldField
    
    return nil
}

func (dst *MyResourceV1) ConvertFrom(srcRaw conversion.Hub) error {
    src := srcRaw.(*MyResourceV2)
    
    // Convert v2 to v1
    dst.ObjectMeta = src.ObjectMeta
    dst.Spec.OldField = src.Spec.NewField
    
    return nil
}
```

### Conversion Webhook Registration

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: myresources.myapi.openshift.io
spec:
  conversion:
    strategy: Webhook
    webhook:
      clientConfig:
        service:
          name: my-operator-webhook
          namespace: openshift-my-operator
          path: /convert
      conversionReviewVersions: ["v1"]
  versions:
  - name: v1
    served: true
    storage: false  # Not storage version
    schema: {...}
  - name: v2
    served: true
    storage: true  # Storage version
    schema: {...}
```

## Breaking Changes

### When Absolutely Necessary

**Requirements**:
1. Enhancement proposal approved
2. Deprecation period (minimum 3 releases)
3. Migration guide provided
4. Documented in release notes

### Migration Path

```go
// Before (v1)
type OldAPI struct {
    Field string  // Changing to int
}

// During transition (v1beta2)
type TransitionAPI struct {
    FieldString string  // Deprecated
    FieldInt    int     // New
}

// After (v2)
type NewAPI struct {
    Field int
}
```

### User Migration

```yaml
# Old resource (v1)
apiVersion: myapi.openshift.io/v1
kind: MyResource
spec:
  field: "42"

# Migrated resource (v2)
apiVersion: myapi.openshift.io/v2
kind: MyResource
spec:
  field: 42
```

## Validation Changes

### Adding Validation

```go
// Safe: New validation that existing values pass
// +kubebuilder:validation:MaxLength=100
Field string `json:"field"`

// Unsafe: Validation that existing values might fail
// +kubebuilder:validation:Pattern="^[a-z]+$"  
// (If existing values have uppercase)
```

### Relaxing Validation

```go
// Always safe to relax validation
// Before:
// +kubebuilder:validation:Enum=a;b;c
// After:
// +kubebuilder:validation:Enum=a;b;c;d  // OK
```

## Subresources

### Adding Subresources

```yaml
# Safe to add subresources
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
spec:
  versions:
  - name: v1
    subresources:
      status: {}  # Adding status subresource - OK
      scale:      # Adding scale subresource - OK
        specReplicasPath: .spec.replicas
        statusReplicasPath: .status.replicas
```

### Removing Subresources

```yaml
# Unsafe - breaks clients using subresource
# Don't remove subresources in stable versions
```

## Examples

### Adding Optional Field (v1 → v1)

```go
// v1 (before)
type MachineConfigSpec struct {
    OSImageURL string
}

// v1 (after)
type MachineConfigSpec struct {
    OSImageURL string
    // +optional
    KernelArguments []string `json:"kernelArguments,omitempty"`
}
```

### Field Rename (v1 → v2)

```go
// v1
type MachineConfigSpec struct {
    ImageURL string
}

// v2 (keep old field for compatibility)
type MachineConfigSpec struct {
    // +optional
    // +kubebuilder:validation:Deprecated
    ImageURL string `json:"imageURL,omitempty"`
    
    // +optional
    OSImageURL string `json:"osImageURL,omitempty"`
}

// Conversion logic
if v2.Spec.OSImageURL == "" && v1.Spec.ImageURL != "" {
    v2.Spec.OSImageURL = v1.Spec.ImageURL
}
```

## Testing API Changes

```go
func TestBackwardCompatibility(t *testing.T) {
    // Create resource with v1 API
    v1Resource := &MyResourceV1{
        Spec: MyResourceV1Spec{
            OldField: "value",
        },
    }
    
    // Convert to v2
    v2Resource := &MyResourceV2{}
    err := v1Resource.ConvertTo(v2Resource)
    if err != nil {
        t.Fatalf("Conversion failed: %v", err)
    }
    
    // Verify data preserved
    if v2Resource.Spec.NewField != "value" {
        t.Error("Data lost in conversion")
    }
    
    // Convert back to v1
    v1Again := &MyResourceV1{}
    err = v1Again.ConvertFrom(v2Resource)
    if err != nil {
        t.Fatalf("Reverse conversion failed: %v", err)
    }
    
    // Verify round-trip
    if v1Again.Spec.OldField != v1Resource.Spec.OldField {
        t.Error("Round-trip conversion changed data")
    }
}
```

## Documentation

### API Reference

```markdown
## MyResource v2

### Spec

| Field | Type | Description | Required |
|-------|------|-------------|----------|
| name | string | Resource name | Yes |
| mode | string | Operation mode (default: "auto") | No |
| replicas | int | Number of replicas | Yes |

### Deprecated Fields

| Field | Deprecated In | Use Instead | Removed In |
|-------|---------------|-------------|------------|
| oldField | v1beta2 | newField | v2 |
```

### Migration Guide

```markdown
# Migrating from v1 to v2

## Breaking Changes

- `spec.oldField` removed, use `spec.newField`
- `spec.count` type changed from string to int

## Migration Steps

1. Update API version in manifests
2. Rename fields as per table above
3. Convert string counts to integers
4. Test in development cluster
5. Roll out to production
```

## References

- **K8s API Conventions**: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- **API Deprecation Policy**: https://kubernetes.io/docs/reference/using-api/deprecation-policy/
- **OpenShift API Guidelines**: https://github.com/openshift/enhancements/blob/master/dev-guide/api-conventions.md
- **Conversion Webhooks**: https://book.kubebuilder.io/multiversion-tutorial/conversion.html
