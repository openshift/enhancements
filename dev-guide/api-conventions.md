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

In OpenShift, we talk about two classes of APIs: workload and configuration APIs.
The majority of APIs in OpenShift are configuration APIs, though some are also workload APIs.

A configuration API is one that is typically cluster-scoped, a singleton within the cluster, and managed by a cluster
administrator only. An example of a configuration API would be the Infrastructure API that defines configuration
for the infrastructure of the cluster, or the Network API that configures the networking within the cluster.
A configuration API helps a user to configure a property or feature of the cluster.

A workload API typically is namespaced and is used by end users of the cluster. Workload API objects may be
instantiated many times within a cluster and often many times within a namespace. Many of the Kubernetes core
resources,  such as the Deployment and DaemonSet APIs, are considered to be workload APIs.
A workload API helps a user to run their workload on top of OpenShift.

You should try to identify whether your API is a workload or configuration API as there are different conventions
applied to them based on which class the API falls into.

#### Defaulting

In workload APIs, we typically default fields on create (or update) when the field isn't set. This sets the value
on the resource and forms a part of the contract between the user and the controller fulfilling the API.
This has the effect that changing the default value in the API does not change the value for objects that have
previously been created.
This has the implication that you cannot change the behaviour of a default value once the API is defined as that would
cause the same object to result in different behavior for different versions of the API, which would surprise users and
compromise portability.

To change the default behaviour could constitute a breaking change and disrupt the end user's workload;
the behaviour must remain consistent through the lifetime of the resource.
This also means that defaults cannot be changed without a breaking change to the API.
If a user were to delete their workload API resource and recreate it, the behaviour should remain the same.

With configuration APIs, we typically default fields within the controller and not within the API.
This means that the platform has the ability to make changes to the defaults over time as we improve the capabilities
of OpenShift.

An API author may reserve the right to change the default value of a field or the behavior of a field value within a
configuration API. To reserve this right, the godoc for the API field must clearly indicate that the value or behavior
is subject to change over time.
When changing a default value or behaviour, we must ensure that there is a smooth upgrade process between the old
default and the new default and that we will not break existing clusters.

Typically, optional fields on configuration APIs contain a statement within their Godoc to describe
the default behaviour when they are omitted, along with a note that this is subject to change over time.
[The documentation section](#write-user-readable-documentation-in-godoc) of the API conventions goes into more detail
on how to write good comments on optional fields.

#### Pointers

In custom resource based APIs specifically, we advise to avoid making fields pointers unless there is an absolute need to do so.
An absolute need being the need to distinguish between the zero value and an unset value.

Using pointers makes writing code to interact with the API harder and more error prone.

For APIs backed by aggregated API servers, pointers must be used for all optional fields, else validation code cannot determine whether the field was unset, or was set to the zero value.

##### Pointers to structs

The JSON tag `omitempty` is not compatible with struct references.
Meaning any struct will always, when empty, be marshalled to `{}`.
This means that any struct field must be a pointer to allow it to be omitted when empty.

However, as of Go 1.24, using the `omitzero` tag will allow structs to be properly omitted.

From Go 1.24 onwards, we only need structs to be pointers when there is an explicit need to distinguish between unset and the empty struct.

It is generally bad practice to want to distinguish between unset and the empty struct.
Each struct should have either at least 1 required field, or a `MinProperties` validation to prevent its zero value from being valid.

### Only one phrasing for each idea

Each idea should have a single possible expression in the API structures, without having multiple ways to say the same thing.
From [PEP 20][pep-20]:

> * There should be one-- and preferably only one --obvious way to do it.
> * Although that way may not be obvious at first unless you're Dutch.

For example, [ClusterVersion's `spec.desiredUpdate`][desiredUpdate] is a pointer field.

This does not meet our current API guidelines (see our [pointer
guidelines](#pointers)) as we advise against the use of pointers in
most cases, but, in this example, it also allows the same semantic to
be expressed in two different ways.

Both `nil`:


```yaml
spec: {}
```

and empty-struct:

```yaml
spec:
  desiredUpdate: {}
```

represent the same "I do not desire an update" idea.

Having a single allowed phrasing has a few benefits:

* Users don't have to spend time wondering about which of several identical phrasings to use, or whether those phrasings are actually identical or not.
* Users don't have to debate about which of several pet phrasings are most canonical.
* There is no need to document alternative phrasings.
* Testing and verification for alternative phrasings are simple: the non-canonical phrasing is rejected, with documentation guiding users towards the canonical phrasing.
* When an API structure has multiple consumers, having a single phrasing for each idea reduces the scope of possible semantic divergence between consumers.

For existing APIs, progress toward these ideals needs to happen within the usual [constraints on API changes][api-changes].

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
// + ---
// + Note that this comment line will not end up in the generated API schema as it is
// + preceded by a `+`. The `---` also prevents anything after it from being added to
// + the swagger docs.
// + This can be used to add notes for developers that aren't intended for end users.
type Example struct {
  // Type allows the user to determine how to interpret the example given.
  // It must be set to one of the following values: Documentation, Convention, or Mixed.
  // +kubebuilder:validation:Enum:=Documentation;Convention;Mixed
  // +required
  Type string `json:"type,omitempty"`

  // Documentation allows the user to define documentation for the example.
  // When this value is provided, the type must be set to either Documentation or Mixed.
  // The content of the documentation is free form text but must be no longer than 512 characters.
  // +kubebuilder:validation:MinLength:=1
  // +kubebuilder:validation:MaxLength:=512
  // +optional
  Documentation string `json:"documentation,omitempty"`

  // Convention allows the user to define the configuration for this API convention.
  // For example, it allows them to set the priority over other conventions and whether
  // this policy should be strictly observed or weakly observed.
  // When this value is provided, the type must be set to either Convention or Mixed.
  // +optional
  Convention ConventionSpec `json:"convention,omitempty,omitzero"`

  // Author allows the user to denote an author for the example convention.
  // The author is not required. When omitted, this means the user has no opinion and the value is
  // left to the platform to choose a good default, which is subject to change over time.
  // The current platform default is OpenShift Engineering.
  // The Author field is free form text.
  // +optional
  // +kubebuilder:validation:MinLength:=1
  // +kubebuilder:validation:MaxLength:=1024
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
  PlatformType string `json:"platformType"`

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

### No functions

Do not add functions to the openshift/api.  Functions seem innocuous, but they have significant side effects over time.

1. Dependency chain.
   We want our dependency chain on openshift/api to be as short as possible to avoid conflicts when they are vendored
   into other projects.
2. Building interfaces on APIs.
   Building interfaces on top of our structs is an anti-goal.  Even the interfaces we have today, `runtime.Object` and
   `meta.Accessor`, cause pain when mismatched levels result in structs dropping in and out of type compliance

The simplest line is "no functions".
Functions can be added in a separate repo, possibly library-go if there are sufficient consumers.
Helpers for accessing labels and annotations are not recommended.

### No annotation-based APIs

Do not use annotations for extending an API. Annotations may seem as a good candidate for introducing experimental/new
API. Nevertheless, migration from annotations to formal schema usually never happens as it requires breaking changes
in customer deployments.

1. Validation does not always come with definition. User set values can be too broad and hard to limit later on.
2. Lack of discoverability. There's no pre-existing schema that can be published.
3. Validation is limited. Certain kinds of validators aren't allowed on annotations, so hooks are more frequently used instead.
4. Hard to extend. An annotation value (a string) can not be extended with additional fields under a parent.
5. Unclear versioning. Annotation keys can omit versioning. Or, there are multiple ways to specify a version.
6. Users can "squat" on annotations by adding an unvalidated annotation value for a key that is used in a future version.

#### Example

[Enabling HTTP Strict Transport Security (HSTS) policy](https://docs.openshift.com/container-platform/4.11/networking/routes/route-configuration.html) through an annotation:

```yaml
apiVersion: v1
kind: Route
metadata:
  annotations:
    haproxy.router.openshift.io/hsts_header: max-age=31536000;includeSubDomains;preload
```

The annotation was introduced in OpenShift 3.X. At the time annotations were very popular as a means to provide experimental configuration.
Nevertheless, after customer adoption the configuration was never migrated to a formal schema to avoid breaking changes.

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

Many ideas start as a Boolean value, e.g. `FooEnabled: true|false`, but often evolve into needing 3, 4, or even more
states at some point during the API's lifetime.
As a Boolean value can only ever have 2 or in some cases 3 values (`true`, `false`, `omitted` when a pointer), we have
seen examples in which API authors have later added additional fields, paired with a Boolean field, that are only
meaningful when the original field has a certain state. This makes it confusing for an end user as they have to be
aware that the field they are trying to use only has an effect in certain circumstances.

Rather than creating a Boolean field:
```go
// authenticationEnabled determines whether authentication should be enabled or disabled.
// When omitted, this means the platform can choose a reasonable default.
// +optional
AuthenticationEnabled *bool `json:"authenticationEnabled,omitempty"`
```

Use an enumeration of values that describe the action instead:
```go
// authentication determines the requirements for authentication within the cluster. Allowed values are "Optional",
// "Required", "Disabled" and omitted.
// Optional authentication allows users to optionally authenticate but will not reject an unauthenticated request.
// Required authentication requires all requests to be authenticated.
// Disabled authentication ignores any attempt to authenticate and processes all requests as unauthenticated.
// When omitted, the authentication will be Optional. This default is subject to change over time.
// +kubebuilder:validation:Enum:=Optional;Required;Disabled;""
// +optional
Authentication AuthenticationPolicy `json:authentication,omitempty`
```

With this example, we have described through the enumerated values the action that the API will have.
Should the API need to evolve in the future, for example to add a particular method of Authentication that should be
used, we can do so by adding a new value (e.g. `PublicKey`) to the enumeration and avoid adding a new field to the API.

### Optional fields should not be pointers (in custom resource based APIs)

In custom resource based APIs, we do not follow the upstream guidance of making optional fields pointers.
Pointers are difficult to work with and are more error prone than references.

This topic is expanded in the [Pointers](#pointers) subsection of the
[Configuration vs Workload APIs](#configuration-vs-workload-apis) above.

## Tech Preview
When new API resources or new API fields are added as TechPreviewNoUpgrade, OpenShift has schema generation extensions
that allow generating multiple manifests for the same golang struct for TechPreviewNoUpgrade versus Default.

See also
1. [FeatureSets](https://github.com/openshift/api/blob/458ad9ca9ca536189d70d8b9e43843dc3435564d/config/v1/types_feature.go#L26-L43)
   in openshift/api.
2. [CVO conditional manifests](https://github.com/openshift/enhancements/blob/6704279c30e975368f6f6fa5b6b7ee00adf4aeb9/enhancements/update/cvo-techpreview-manifests.md)
   in openshift/enhancements.
3. [Process](https://github.com/openshift/enhancements/blob/b99a80d976385faa87f251dbbcd4260a37406921/dev-guide/featuresets.md#id-like-to-declare-a-feature-accessible-by-default--what-is-the-process) for declaring a feature Accessible-by-default

This capability is important to use because it requires users to opt-in for TechPreview functionality on their cluster
and prevents the accidental usage of TechPreview fields and types on production clusters.

### New Makefile target
To support TechPreview annotations and tags for your API group you will need to add new targets.
Once you have added these targets, you can make use of the TechPreview generation.

```makefile
$(call add-crd-gen,example,./example/v1,./example/v1,./example/v1)
$(call add-crd-gen-for-featureset,example,./example/v1,./example/v1,./example/v1,TechPreviewNoUpgrade)
$(call add-crd-gen-for-featureset,example,./example/v1,./example/v1,./example/v1,Default)
$(call add-crd-gen,example-alpha,./example/v1alpha1,./example/v1alpha1,./example/v1alpha1)
```

See the openshift/api [example](https://github.com/openshift/api/blob/5eaf4250c423eb7ae6b3139d82c14f14e5fe804a/Makefile#L32-L35).

### New API resource
If you're creating an entirely new CRD manifest and the CVO installs your CRD manifest, adding this annotation will
tell the CVO to only create your manifest if the cluster is using TechPreviewNoUpgrade.

```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    release.openshift.io/feature-set: TechPreviewNoUpgrade
```

See the openshift/api [example](https://github.com/openshift/api/blob/5eaf4250c423eb7ae6b3139d82c14f14e5fe804a/example/v1alpha1/0000_50_notstabletype-techpreview.crd.yaml).

### New field added to an existing CRD
If you're adding a TechPreview field to an existing CRD, you will have to create two yaml files, one for running in Default
and one for running in TechPreviewNoUpgrade.
By convention they are named  

`<normal-name>-default.crd.yaml` with content
```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    release.openshift.io/feature-set: Default
```
and

`<normal-name>-techpreview.crd.yaml` with content
```yaml
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  annotations:
    release.openshift.io/feature-set: TechPreviewNoUpgrade
```

Then in your golang struct, you add a comment tag `// +openshift:enable:FeatureSets=TechPreviewNoUpgrade`
```go
type StableConfigTypeSpec struct {
    // coolNewField is a field that is for tech preview only.  On normal clusters this shouldn't be present
    //
    // +kubebuilder:validation:Optional
    // +openshift:enable:FeatureSets=TechPreviewNoUpgrade
    // +optional
    CoolNewField string `json:"coolNewField"`
}
```

The generator will generate the `coolNewField` into `<normal-name>-techpreview.crd.yaml`, but not into `<normal-name>-default.crd.yaml`.

See the openshift/api [example](https://github.com/openshift/api/tree/5eaf4250c423eb7ae6b3139d82c14f14e5fe804a/example/v1).

### New allowed value for an existing enum
Often you need to add a value to an enumeration. This comes up frequently for the discriminator field in discriminated unions.
To do this, you will use the `// +openshift:validation:FeatureSetAwareEnum:featureSet` tag.

```go
type EvolvingUnion struct {
	// type is the discriminator. It has different values for Default and for TechPreviewNoUpgrade
	Type EvolvingDiscriminator `json:"type"`
}

// EvolvingDiscriminator defines the audit policy profile type.
// +openshift:validation:FeatureSetAwareEnum:featureSet=Default,enum="";StableValue
// +openshift:validation:FeatureSetAwareEnum:featureSet=TechPreviewNoUpgrade,enum="";StableValue;TechPreviewOnlyValue
type EvolvingDiscriminator string

const (
	// "StableValue" is always present.
	StableValue EvolvingDiscriminator = "StableValue"

	// "TechPreviewOnlyValue" should only be allowed when TechPreviewNoUpgrade is set in the cluster
	TechPreviewOnlyValue EvolvingDiscriminator = "TechPreviewOnlyValue"
)
```

The generator will generate the `TechPreviewOnlyValue` into `<normal-name>-techpreview.crd.yaml`, but not into `<normal-name>-default.crd.yaml`.

See the openshift/api [example](hhttps://github.com/openshift/api/blob/5eaf4250c423eb7ae6b3139d82c14f14e5fe804a/example/v1/types_stable.go#L48-L64).

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

[api-changes]: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api_changes.md
[desiredUpdate]: https://github.com/openshift/api/blob/803c45de7ab5567e8d1575138f014226974768a1/config/v1/types_cluster_version.go#L43-L57
[pep-20]: https://peps.python.org/pep-0020/
