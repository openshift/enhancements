---
title: generated-resource-types
authors:
  - "@logonoff"
reviewers:
  - "@jhadvig"
  - "@spadgett"
approvers:
  - "@spadgett"
api-approvers:
  - "None"
creation-date: 2025-09-16
last-updated: 2025-09-25
tracking-link:
  - https://issues.redhat.com/browse/CONSOLE-4775
---

# Migration to generated resource types from their OpenAPI specs

## Summary

The OpenShift web console interacts with various Kubernetes (k8s) resources to
provide its functionality. The schemas of these resources which are used by the
web console to interact with the k8s API are currently manually defined as TypeScript
types in the `frontend/public/module/k8s/types.ts` file.

These types are often not reviewed for accuracy or completeness, and may not
always align with the actual OpenAPI specifications of the resources. This leads
to bugs in the web console due to incorrect assumptions about the structure of
the resources.

This enhancement proposes the addition of automated generation of TypeScript types
for Kubernetes resources used by the OpenShift web console. The goal is to ensure
better alignment with the source-of-truth schemas, reduce manual effort, and minimize
he risk of bugs due to schema misalignment.

## Motivation

The effort to manually maintain TypeScript resource types in the web console is
cumbersome and error-prone, and is occasionally the cause of bugs due to misalignment
between the actual resource schema and the manually defined TypeScript types.

To address this, we propose adding automated generation of these TypeScript types
based on the OpenAPI specifications of the k8s resources. This will ensure better
alignment with the source-of-truth schemas, reduce manual effort, and minimize the
risk of bugs due to schema misalignment.


### User Stories

* As an OpenShift engineer, I want accurate TypeScript types for k8s resources used
  by the web console, so that I can make correct assumptions about the structure of
  these resources when developing features.
* As an OpenShift engineer, I want to reduce the manual effort required to maintain
  TypeScript types for k8s resources, so that I can focus on developing features
  rather than maintaining types.

### Goals

The current manual maintenance of TypeScript types was originally done in during
the early development of the web console by CoreOS, when the web console was meant
to be a simple management interface for k8s clusters.

The goals of this enhancement are to:

* Periodically publish a package on npm which contains TypeScript types for k8s
  resources used by the web console, based on their OpenAPI specifications.
* Consume this package in the web console and in associated console dynamic plugins.

### Non-Goals

This enhancement will not seek to implement:

* Validation of objects against the generated types during runtime.
* Generation of types for all CustomResourceDefinitions (CRDs) used by the web console.
  The initial focus will be on resources defined by the `openshift/kubernetes`
  swagger definitions and the CRDs defined by the `openshift/api` repository.

## Proposal

### Workflow Description

**library authors** are responsible for maintaining the type generation library.
1. They will run the generation scripts at some point during the OpenShift release
2. The generated types will be published to a package on npm (e.g. `@openshift/k8s-types`)

**web console developers** will consume the generated types in the web console.
1. They will add or update the dependency to the package in the web console's `package.json`.
2. They will import the types from the package in the web console's codebase.
3. They will fix resulting type errors in the web console's codebase due to
   changes in the generated types.


### API Extensions
NA

### Risks and Mitigations

The proposal aims to mitigate risks which already exist and have manifested through
various bugs.

### Drawbacks

* Web console developers would loose unlimited control over types which are used
  in the codebase.
* The current implementation of the generated types replaces the `K8sResourceCommon`
  `type` with an `interface`, which aligns with TypeScript's recommendations. However,
  this may cause some type errors in the web console and dependent dynamic plugins
  which will need to be fixed. The type errors often occur when the `omit` utility
  function in Lodash is used on objects of type `K8sResourceCommon`. However,
  this is mitigated by the fact that TypeScript is compiled away during build time,
  so the runtime behavior of the web console will not be affected.

## Test Plan

As this enhancement proposal focuses on the generation of TypeScript types,
existing unit and integration tests that utilize these types will serve as the
primary means of validation. To ensure the correctness of the generated types,
we will implement the following test strategies:

* **Unit tests in the generation library**: We will add unit tests to the type
  generation library to verify that it correctly transforms OpenAPI specifications
  into TypeScript types.
* **Integration tests in the web console**: We will leverage existing unit and
  integration tests in the web console that utilize the generated types. These
  tests will help ensure that the generated types are accurate and function as
  expected within the context of the web console.
* **Manual verification**: We will perform manual verification of the generated
  types by inspecting them and comparing them to the original OpenAPI definitions.

## Graduation Criteria
N/A

### Dev Preview -> Tech Preview
N/A

### Tech Preview -> GA
N/A

### Removing a deprecated feature
N/A

## Upgrade / Downgrade Strategy
N/A

## Version Skew Strategy

Updating the types package should only affect the contents of the types when
they are updated by the source resource specifications. As such, version skew
is mitigated by OpenShift's policy of not breaking APIs.

However, when breaking changes do occur, older versions of resource types can
still be exported by the type library to ensure backwards-compatibility with
existing web console code and resources.

## Operational Aspects of API Extensions
N/A

### Deprecated Feature
N/A

### Topology Considerations
N/A

#### Hypershift / Hosted Control Planes
N/A

#### Standalone Clusters
N/A

#### Single-node Deployments or MicroShift
N/A

## Support Procedures
N/A

### Implementation Details/Notes/Constraints

A proof of concept for generating these types was written in [logonoff/openshift-types].
While these types align with the web console naming conventions, there are changes
which cause type errors when directly replaced with the web console equivalents.

[kubernetes-model-ts]: https://github.com/tommy351/kubernetes-models-ts
[logonoff/openshift-types]: https://github.com/logonoff/openshift-types

#### Steps to Implement This Feature

1. **Develop the type generation library**: Create a library that can fetch OpenAPI
   specifications and generate TypeScript types from them. This library should be
   capable of handling both core k8s resources and CRDs defined by the `openshift/api`
   repository. This library will be located in a separate repository from both `openshift/api`
   and `openshift/console`.

2. **Integrate the generation process into the OpenShift release cycle**: Determine
   the appropriate point in the OpenShift release cycle to run the type generation
   scripts and publish the generated types to a package on npm.
3. **Update the web console to consume the generated types**: Add or update the
   dependency to the generated types package in the web console's `package.json`,
   and update the web console's codebase to import and use the generated types.
4. **Fix type errors in the web console**: Address any type errors that arise in
   the web console's codebase due to changes in the generated types.

## Alternatives (Not Implemented)

Prior art exists in the form of npm packages containing TypeScript types for k8s
resources, such as [kubernetes-model-ts]. This package is used by already used
and has contributors from Red Hat Developer Hub (based on Backstage); however,
these packages do not cover OpenShift CustomResourceDefinitions.

Moreover, the type names and overall implementation of the types do not align with
the status quo we have in the web console. This would require further work in the
web console to both adapt to the new type and to implement the OpenShift-specific
CustomResourceDefinitions in the upstream library.

## Open Questions

1. OpenShift web console currently uses manually defined TypeScript types for k8s
   resources, which assume that all resources share the same version. How should
   we handle resources that may exist in different versions (e.g., `v1`,
   `v1beta1`, etc.)?
   - One approach could be to ensure that the code consuming these types can safely
     assume a specific version (e.g., the latest available version), and to request
     the API server to return resources in that version.
