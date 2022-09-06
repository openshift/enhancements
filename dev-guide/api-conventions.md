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

### Configuration vs Workload APIs

In OpenShift, we talk about two classes of APIs, Workload and Configuration APIs.
The majority of APIs in OpenShift are Configuration APIs, though some are also Workload APIs.

A Configuration API is one that is typically cluster scoped, a singleton within the cluster and managed by a Cluster
Administrator only. An example of a configuration API would be the Infrastructure resource that defines configuration
for the Infrastructure of the Cluster, or the Network resource that configures the networking within the cluster.
A Configuration API helps a user to configure a property or feature of the cluster.

A Workload API typically is namespaced and is used by end users of the cluster. Workload APIs exist many times within a
cluster and often many times within a namespace. Many of the Kubernetes core resources such as the Deployment and
DaemonSet APIs are considered to be Workload APIs.
A Workload API helps a user to run their workload on top of OpenShift.

You should try to identify whether your API is a Workload or Configuration API as there are different conventions
applied to them based on which class the API falls into.

#### Defaulting

In Workload APIs, we typically default fields on create (or update) when the field isn't set. This sets the value
on the resource and forms a part of the contract between the user and the controller fulfilling the API.
This has the effect that you cannot change the behaviour of a default value once the resource is created.

To change the default behaviour could constitute a breaking change and disrupt the end users workload,
the behaviour must remain consistent through the lifetime of the resource.
This also means that defaults cannot be changed without a breaking change to the API.
If a user were to delete their Workload API resource and recreate it, the behaviour should remain the same.

With Configuration APIs, we typically default fields within the controller and not within the API.
This means that the platform has the ability to make changes to the defaults over time as we improve the capabilities
of OpenShift. While we reserve the right to change a default in a Configuration API, we must ensure that when there is a
change, that there is a smooth upgrade process between the old default and the new default and that we will not break
existing clusters.

Typically, optional fields on Configuration APIs contain a statement within their Godoc to describe
the default behaviour when they are omitted, along with a note that this is subject to change over time.
[The documentation section](#write-user-readable-documentation-in-godoc) of the API conventions goes into more detail
on how to write good comments on optional fields.

#### Pointers

In Configuration APIs specifically, we advise to avoid making fields pointers unless there is an absolute need to do so.
An absolute need being the need to distinguish between the zero value and a nil value.

Using pointers makes writing code to interact with the API harder and more error prone and it also harms the
discoverability of the API.

When we use references, the marshalled version of a resource will include unset fields with their zero value.
This means, that any end user fetching the resource from the API can observe that a particular field exists and
"discover" a potentially new feature or option that they were not previously aware of.
This has the effect of helping our users to understand how they might be able to configure their cluster, without
having to search for features or review API schemas within the product docs.

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

### Prefer consistency with Kubernetes style APIs

As both Kubernetes and OpenShift have a number of integrations with cloud and infrastructure platforms, there are a
number of different abstractions built into OpenShift that abstract and extend infrastructure capabilities.
For example, the Machine API or platform specific storage integrations within OpenShift.

These integrations often result in adding new API fields to OpenShift to allow users to configure various features of
the platform. Often, different platform's concepts are similar, but their naming of particular features can be specific
to their platform.
If we copy verbatim the API from a platform, this may or may not be compliant with OpenShift conventions and may or
may not make sense in a wider scope.

If we intend to support similar features across platforms, it is preferable to have similar APIs for these features
within OpenShift rather than mimicking the platform API. This has the benefit of providing consistency to users when
using OpenShift across multiple platforms and reducing the learning curve when installing OpenShift on a new platform.
Where possible, we should link to the platform specific documentation for features in the description of our own APIs.

While we appreciate that reusing the platform specific terminology can make it easier for someone familiar with the
platform to understand the API, we prefer to stick to Kubernetes style conventions (eg preferring PascalCase for
enumerated values) when abstracting platform specific APIs as this allows us to build a consistent looking API across
the OpenShift product.
Differences between our APIs and platform APIs can be handled within the controller backing the API.

We have seen examples in the past where the value of a platform API is not intuitive to the value to the end user.
When designing APIs for OpenShift we try to make it clear what the value is to an end user and what exactly will happen
when they configure a particular field.
Where platform APIs may talk about a feature in terms of the implementation, we should aim to talk about a feature
in terms of the action that OpenShift and the platform will take.
This is an easy way to help users understand the effects of their actions and provide additional value over them using
the platform specific APIs directly.

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

### Do not use Boolean fields

While the upstream Kubernetes conventions recommend thinking twice about using Booleans, they are explicitly forbidden
within OpenShift APIs.

Many ideas start as a boolean value, eg `FooEnabled: true|false`, but often evolve into needing 3, 4 or even more states
at some point during the APIs lifetime.
As a Boolean value can only ever have 3 values (`true`, `false`, `omitted` when a pointer), we have seen examples in
where API authors have later added additional fields, paired with the Boolean field that are only meaningful when the
original field has a certain state. This makes it confusing for an end user as they have to be aware that the field
they are trying to use, only has an effect in certain circumstances.

Rather than creating a boolean field:
```go
// authenticationEnabled determines whether authentication should be enabled or disabled.
// When omitted, this means the platform can choose a reasonable default.
// +optional
AuthenticationEnabled *bool `json:"authenticationEnabled,omitempty"`
```

Use an enumeration of values that describe the action instead:
```go
// authentication determines the requirements for authentication within the cluster.
// When omitted, the authentication will be Optional.
// +kubebuilder:validation:Enum:=Optional;Required;Disabled;""
// +optional
Authentication AuthenticationPolicy `json:authentication,omitempty`
```

With this example, we have described through the enumerated values the action that the API will have.
Should the API need to evolve in the future, for example to add a particular method of Authentication that should be
used, we can do so by adding a new value (eg. `PublicKey`) to the enumeration and avoid adding a new field to the API.

### Optional fields should not be pointers (in Configuration APIs)

In [Configuration APIs](#configuration-vs-workload-apis), we do not follow the upstream guidance of making optional
fields pointers.
Pointers are difficult to work with and are more error prone than references and they also harm the discoverability
of the API.

This topic is expanded in the [Pointers](#pointers) subsection of the
[Configuration vs Workload APIs](#configuration-vs-workload-apis) above.

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

### I'm copying an API from an upstream project or platform/cloud provider, can I change the design?

Yes! When designing an API for an abstraction, you should consider the end user experience and the value your
abstraction is providing. If you copy the API verbatim, are you adding any value?

If an upstream/platform API is not intuitive, we can improve the user experience by creating our own naming that better
describes the effects of enabling a specific field with a specific value. Think about why a user would configure a field
when choosing the name.

When writing APIs for OpenShift, we try to make our APIs consistent with one another and "Kube-like" so that users of
OpenShift have an understanding of how to use our APIs intuitively. If an upstream API is not consistent with
those conventions, you should be prepared to change your abstraction to follow conventions to maintain that consistent
user experience within OpenShift.
