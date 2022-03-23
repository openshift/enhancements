---
title: API Conventions
authors:
  - "@Miciah"
  - "@JoelSpeed"
reviewers:
  - ""
approvers:
  - ""
creation-date: 2022-02-23
last-updated: 2022-02-23
status: informational
---

# OpenShift API Conventions

OpenShift APIs follow the [Kubernetes API conventions](https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md)
with some exceptions and additional guidance, outlined below.

## API Author Guidance

### Write User Readable Documentation in Godoc

Godoc text is generated into both Swagger and OpenAPI schemas which are then used
in tools such as `oc explain` or the API reference section of the OpenShift product
documentation to provide users descriptions of how to use and interact with our products.

In general, Godoc comments should be complete sentences and as much as possible, should
adhere to the OpenShift product docs [style guide](https://redhat-documentation.github.io/supplementary-style-guide/#style-guidelines).

The Godoc on any field in our API should be sufficiently explained such that an end user
understands the following:

* What is the purpose of this field? What does it allow them to achieve?
* How does setting this field interact with other fields or features?
* What are the limitations of this field?
  * Does it have any maximum or minimum value?
  * If it is a string value, are the values limited to a specific list or can it be free form,
  must it meet a certain regex?
  * Limitations should be written out within the Godoc as well as added within `kubebuilder` tags. Kubebuilder tags are
  used for validation but are *not* included within any generated documentation.
  * See the [validation docs](https://book.kubebuilder.io/reference/markers/crd-validation.html) for inspiration on more validations to apply.
* Is the field optional or required?
* When optional, what happens when the field is omitted?
  * You may choose to set a default value within the API or have a controller default the value
    at runtime

For example:

```go
// Example enables developers to understand how to write user facing documentation
// within the Godoc of their API types.
// Example is used within the wider Conventions to improve the end user experience
// and is a required convention.
// At least one value must be provided within the example and the type should be set
// appropriately.
// +kubebuilder:validation:Required
// + ---
// + Note that this comment line will not end up in the generated API schema as it is
// + preceded by a `+`. The `---` also prevents anything after it from being added to
// + the swagger docs.
// + This can be used to add notes for developers that aren't intended for end users.
type Example struct {
  // Type allows the user to determine how to interpret the example given.
  // It must be set to one of the following values: Documentation, Convention, or Mixed.
  // +kubebuilder:validation:Enum:=Documentation;Convention;Mixed
  // +kubebuilder:validation:Required
  Type string `json:"type"`

  // Documentation allows the user to define documentation for the example.
  // When this value is provided, the `type` must be set to either `Documentation` or `Mixed`.
  // The content of the documentation is free form text but must be no longer than 512 characters.
  // +kubebuilder:validation:MaxLength:=512
  // +optional
  Documentation string `json:"documentation,omitempty"`

  // Convention allows the user to define the configuration for this API convention.
  // For example, it allows them to set the priority over other conventions and whether
  // this policy should be strictly observed or weakly observed.
  // When this value is provided, the `type` must be set to either `Convention` or `Mixed`.
  // +optional
  Convention ConventionSpec `json:"convention,omitempty"`

  // Author allows the user to denote an author for the example convention.
  // The author is not required. When omitted, this means the user has no opinion and the value is
  // left to the platform to choose a reasonable default, which is subject to change over time.
  // The current platform default is `OpenShift Engineering`.
  // The Author field is free form text.
  // +optional
  // + Note: In this example, the `platform` refers to OpenShift Container Platform and not
  // + a cloud provider (commonly referred to among engineers as platforms).
  Author string `json:"author,omitempty"`
}
```

This API is then explained by `oc explain` as:

```shell
RESOURCE: example <Object>

DESCRIPTION:
  Example enables developers to understand how to write user facing documentation within the Godoc of their API types.
  Example is used within the wider Conventions to improve the end user experience and is a required convention. At
  least one value must be provided within the example and the type should be set appropriately.

FIELDS:
  author <string>
    Author allows the user to denote an author for the example convention. The author is not required. When omitted,
    this means the user has no opinion and the value is left to the platform to choose a reasonable default, which is
    subject to change over time. The current platform default is `OpenShift Engineering`. The Author field is free form
    text.

  convention <Object>
    Convention allows the user to define the configuration for this API convention. For example, it allows them to set
    the priority over other conventions and whether this policy should be strictly observed or weakly observed. When
    this value is provided, the `type` must be set to either `Convention` or `Mixed`.

  documentation <string>
    Documentation allows the user to define documentation for the example. When this value is provided, the `type` must
    be set to either `Documentation` or `Mixed`. The content of the documentation is free form text but must be no
    longer than 512 characters.

  type <string>
    Type allows the user to determine how to interpret the example given. It must be set to one of the following
    values: Documentation, Convention, or Mixed.
```

By providing quality documentation within the API itself, a number of generated API references
benefit from the additional context provided which in turn makes it easier for end users to understand
and use our products.

## Exceptions to Kubernetes API Conventions

### Use JSON Field Names in Godoc

Ensure that the godoc for a field name matches the JSON name, not the Go name,
in Go definitions for API objects.  In particular, this means that the godoc for
field names should use an initial lower-case letter.  For example, don't do the
following:

```go
// Example is [...]
type Example struct {
	// ExampleFieldName specifies [...].
	ExampleFieldName int32 `json:"exampleFieldName"`
}
```

Instead, do the following:

```go
// Example is [...]
type Example struct {
	// exampleFieldName specifies [...].
	ExampleFieldName int32 `json:"exampleFieldName"`
}
```

The godoc for API objects appears in generated API documentation and `oc
explain` output.  Following this convention has the disadvantage that the godoc
does not match the Go definitions that developers use, but it has the advantage
that generated API documentation and `oc explain` output show the correct field
names that end users use, and the end-user experience is more important.

### Use Specific Types for Object References, and Omit "Ref" Suffix

Use resource-specific types for object references.  For example, avoid using the
generic `ObjectReference` type; instead, use a more specific type, such as
`ConfigMapNameReference` or `ConfigMapFileReference` (defined in
[github.com/openshift/api/config/v1](https://github.com/openshift/api/blob/master/config/v1/types.go)).
If necessary, define a new type and use it.  Omit the "Ref" suffix in the field
name.  For example, don't do the following:

```go
// Example is [...]
type Example struct {
	// FrobulatorConfigRef specifies [...].
	FrobulatorConfigRef corev1.LocalObjectReference `json:"frobulatorConfigRef"`

	// DefabulatorRef specifies [...].
	DefabulatorRef corev1.LocalObjectReference `json:"defabulatorRef"`
}
```

Instead, do the following:

```go
// Example is [...].
type Example struct {
	// frobulatorConfig specifies [...].
	FrobulatorConfig configv1.ConfigMapNameReference `json:"frobulatorConfig"`

	// defabulator specifies [...].
	Defabulator LocalDefabulatorReference `json:"defabulator"`
}

// LocalDefabulatorReference references a defabulator.
type LocalDefabulatorReference struct {
	// name is the metadata.name of the referenced defabulator object.
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name"`
}
```

Following this convention has the disadvantage that API developers may need to
define additional types.  However, using custom types has the advantage that the
types can have context-specific godoc that is more useful to the end-user than
the generic boilerplate of the generic types.

### Use Resource Name Rather Than Kind in Object References

Use resource names rather than kinds for object references.  For example, don't
do the following:

```go
// DefabulatorReference references a defabulator.
type DefabulatorReference struct {
	// APIVersion is the API version of the referent.
	APIVersion string `json:"apiVersion"`
	// Kind of the referent.
	Kind string `json:"kind"`
	// Namespace of the referent.
	Namespace string `json:"namespace"`
	// Name of the referent.
	Name string `json:"name"`
}
```

Instead, do the following:

```go
// DefabulatorReference references a defabulator [...]
type DefabulatorReference struct {
	// group of the referent.
	// +kubebuilder:validation:Required
	// +required
	Group string `json:"group"`
	// resource of the referent.
	// +kubebuilder:validation:Required
	// +required
	Resource string `json:"resource"`
	// namespace of the referent.
	// +kubebuilder:validation:Required
	// +required
	Namespace string `json:"namespace"`
	// name of the referent.
	// +kubebuilder:validation:Required
	// +required
	Name string `json:"name"`
}
```

Following this convention has the disadvantage that it deviates from what users
may be accustomed to from upstream APIs, but it has the advantage that it avoids
ambiguity and the need for API consumers to resolve an API version and kind to
the resource group and name that identify the resource.
