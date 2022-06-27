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

## Why do we have API Reviews and Conventions?

As OpenShift developers, we are creating a product where the API and its design play an important part in
how our users interact with the product.
Our users configure and use our product by interacting with the APIs that we define within the [openshift/api](https://github.com/openshift/api) repository.
Whether they interact with the API via a CLI or GUI, the shape of the API contract will play a role in that
experience.

As OpenShift is a large product with many teams working independently to introduce new features,
we must have a centralised set of conventions to ensure that the look and feel of our APIs is consistent
across the board.
By having consistent API design, our end users will feel familiar with the API no matter which of our APIs
they are working with.

API review also plays an important part in making sure that the APIs we release are of high quality and,
where possible, consider the future needs of the API and the possible expansions we may make.

As OpenShift APIs are supported immediately once they are merged, API reviews play an important part in
making sure the API is correct (it has a logical shape and sufficient validation and documentation) and
that, should changes be required later, these changes can be made in a way that is compatible with the
existing, shipped API.

A number of the conventions set out in this document derive from previous mistakes made in past API designs
that became difficult to maintain as the API evolved.
Importantly, this means that, while some of our APIs are not compliant with conventions, all new APIs must
be compliant. Repeating the mistakes we have previously made is not acceptable and will not be approved by
the API review team.

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
    at runtime.
  * If you believe the default value may change over time, the value must be defaulted at runtime and you should    
    include a note in the Godoc which explains that the default value is subject to change. Typically this will look something like `When omitted, this means the user has no opinion and the value is left to the platform to choose a good default, which is subject to change over time. The current default is <default>.`

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
  // When this value is provided, the type must be set to either Documentation or Mixed.
  // The content of the documentation is free form text but must be no longer than 512 characters.
  // +kubebuilder:validation:MaxLength:=512
  // +optional
  Documentation string `json:"documentation,omitempty"`

  // Convention allows the user to define the configuration for this API convention.
  // For example, it allows them to set the priority over other conventions and whether
  // this policy should be strictly observed or weakly observed.
  // When this value is provided, the type must be set to either Convention or Mixed.
  // +optional
  Convention ConventionSpec `json:"convention,omitempty"`

  // Author allows the user to denote an author for the example convention.
  // The author is not required. When omitted, this means the user has no opinion and the value is
  // left to the platform to choose a good default, which is subject to change over time.
  // The current platform default is OpenShift Engineering.
  // The Author field is free form text.
  // +optional
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
    subject to change over time. The current platform default is OpenShift Engineering. The Author field is free form
    text.

  convention <Object>
    Convention allows the user to define the configuration for this API convention. For example, it allows them to set
    the priority over other conventions and whether this policy should be strictly observed or weakly observed. When
    this value is provided, the type must be set to either Convention or Mixed.

  documentation <string>
    Documentation allows the user to define documentation for the example. When this value is provided, the type must
    be set to either Documentation or Mixed. The content of the documentation is free form text but must be no
    longer than 512 characters.

  type <string>
    Type allows the user to determine how to interpret the example given. It must be set to one of the following
    values: Documentation, Convention, or Mixed.
```

By providing quality documentation within the API itself, a number of generated API references
benefit from the additional context provided which in turn makes it easier for end users to understand
and use our products.

### Discriminated Unions

In configuration APIs, we commonly have sections of the API model that are only valid to be configured in certain scenarios. A common example of this within OpenShift is platform specific configuration.
When running on AWS, the other platform configuration blocks are not valid to be set.
To model this within an API, we use a discriminated union, which models an at-most-one-of semantic with a
declarative choice.

#### What is a Discriminated Union?

A discriminated union is a structure within the API that closely resembles a union type.
A number of fields exist within the structure and we are expecting the user to configure precisely one of
the fields.

In particular, for a discriminated union, an extra field exists which allows the user to declaratively
state which of particular fields they are configuring.

We use discriminated unions in Kubernetes APIs so that we force the user to make a choice.
We do not want our code to guess what the user meant, they should tell us which of the choices they made
using the discriminant.

#### Writing a union in Go

Union types in Go have some additional helper tags which signify how the structure should be handled to consumers.
Below is an example based on the idea of platform types.

```go
// MyPlatformConfig is a discriminated union of platform specific configuration.
// It has a +union tag which informs consumers that this is expected to be a union type.
// +union
type MyPlatformConfig struct {
  // PlatformType is the unions discriminator.
  // Users are expected to set this value to the name of the platform.
  // The value of this field should match exactly one field in the union structure.
  // It has a +unionDiscriminator tag to inform consumers that this is the discriminator field.
  // The field should be an enum type, so you may also need an enum tag.
  // The enum values should be in PascalCase.
  // The field should be required.
  // In configuration APIs, you may also want to allow an empty value or "NoOpinion" value to
  // allow the consumer to declare that they do not have an opinion and that the platform
  // should choose a sensible default on their behalf.
  // +unionDiscriminator
  // +kubebuilder:validation:Enum:="AWS";"Azure";"GCP"
  // +kubebuilder:validation:Required
  PlatformType string `json:"platformType,omitempty"`

  // AWS is the AWS configuration.
  // All structures within the union must be optional and pointers.
  // +optional.
  AWS *MyAWSConfig `json:"aws,omitempty"`

  // Azure is the Azure configuration.
  // All structures within the union must be optional and pointers.
  // +optional.
  Azure *MyAzureConfig `json:"azure,omitempty"`

  // GCP is the GCP configuration.
  // All structures within the union must be optional and pointers.
  // +optional.
  GCP *MyGCPConfig `json:"gcp,omitempty"`
}
```

The discriminator here allows the consumer to determine which of the configuration structures they should be consuming, AWS, Azure or GCP.

Important to note:
* All structs within the union **MUST** be pointers
* All structs within the union **MUST** be optional
* The discriminant should be required
* The discriminant **MUST** be a string (or string alias) type
* Discriminant values should be PascalCase and should be equivalent to the camelCase field name (json tag) of one member of the union
* Empty union members (discriminant values without a paired union member) are also permitted

#### Using union types

Below are some examples of how a user may configure the above example.

##### Case 1

```yaml
myPlatformConfig:
  platformType: AWS
  aws:
    ...
```

This is valid. Only one struct is configured and the discriminant is correct.

##### Case 2

```yaml
myPlatformConfig:
  platformType: AWS
  aws:
    ...
  azure:
    ...
```

This is invalid. The Azure configuration should not be configured when the `platformType` is AWS.

##### Case 3

```yaml
myPlatformConfig:
  aws:
    ...
```

This is invalid. Only one struct is configured but the discriminant is omitted.

##### Case 4

```yaml
myPlatformConfig:
  aws:
    ...
  azure:
    ...
```

This is invalid. Multiple structs have been configured and no discriminant is provided.

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

## FAQs

### My proposed design looks like an existing API we have, why am I being told that it must be changed?

There are a few reasons why an API reviewer might want you to change the proposed design of your API addition.

When the API looks similar to an existing API, there are a couple of important things to bear in mind.

Firstly, not all APIs in OpenShift have been through the API review process, therefore, especially early
in the OpenShift 4 lifecycle, many APIs were shipped that were not compliant with the conventions.

Secondly, the conventions have evolved over time as we have learned what does and doesn't work.
Naturally this means that older APIs are not compliant with current conventions.

Thirdly, the API review team is relatively small and API reviews can be very time consuming.
During the review process, sometimes things are missed and lead to APIs being merged that aren't compliant.

No matter the reason for an existing API being non-compliant with current conventions, the reasons above are not
sufficient justification for merging a new API that doesn't meet conventions.
If your proposed API changes look like an existing API, but that API is not compliant, we will ask you to update the
API to meet the latest conventions.

When adhered to the conventions prevent us from making API design mistakes or repeating them.
As such, the existence of non-compliant APIs is not a justification for introducing additional non-compliant APIs.
