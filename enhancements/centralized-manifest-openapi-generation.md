---
title: centralized-manifest-openapi-generation
authors:
  - "@damemi"
reviewers:
  - "@deads2k"
  - "@sttts"
approvers:
  - "@deads2k"
  - "@sttts"
creation-date: 2019-09-23
last-updated: 2019-09-23
status: provisional
---

# Centralized CRD Manifest OpenAPI Generation

## Release Signoff Checklist
- [ ] Enhancement has been determined `implementable`
- [ ] Design details are appropriately documented
- [ ] Test plan is defined

## Summary
This document proposes migrating operator CRD manifests (and, more importantly, the work associated with updating them)
to be colocated with the Golang types that define them in the [openshift/api](https://github.com/openshift/api) repository.

## Motivation
We have many components that consume custom resources, and almost all of those resources are based on Golang types defined in
`openshift/api`. When those Golang types change, or CRD requirements change such as with [Kubernetes 1.16's
structural schemas](https://kubernetes.io/blog/2019/06/20/crd-structural-schema/), these changes require matching updates
to each operator repo. This is primarily evident in changes to OpenAPIV3 validation schemas, where it is not uncommon
to make small changes to API markers which then need to be individually carried to each operator where they are generated
into new manifest changes. In addition, maintainers currently may have different approaches to generating their manifests
(using different tools and scripts) which can lead to discrepancies and unexpected behavior without support.

By moving CRD manifests to `openshift/api`, we will colocate these types with their consumers and enable
organization-wide changes to these manifests to be enabled at once, included with the corresponding changes to their types.
We also will provide a single approach to maintaining the API/manifest relationship, specifically in regards to generating
OpenAPIV3 validation schemas.

### Goals

1. Operator and API maintainers can make changes to APIs and their corresponding CRD manifests simultaneously
2. Manifest changes can be easily read from `openshift/api` to the operators that consume them
3. Unify organization schema generation under a single tool and approach for CRD schema generation

### Non-goals

- We will not be responsible for maintaining individual component APIs or manifests. Though we are providing a single method
to organize their management, maintainers will still be responsible for changes related to only their operator.

## Proposal

CRD manifests from relevant repositories will be moved to the same package  directories in `openshift/api` as the types that define them.
This provides the benefit of allowing these types to be imported with Go modules, as well as inheriting the existing OWNERS
files for those packages.

Example, for config manifests:
```
config/
├── install.go
└── v1
    ├── 0000_03_config-operator_01_operatorhub.crd.yaml
    ├── 0000_03_config-operator_01_proxy.crd.yaml
    ├── 0000_10_config-operator_01_apiserver.crd.yaml
    ├── 0000_10_config-operator_01_authentication.crd.yaml
    ├── 0000_10_config-operator_01_build.crd.yaml
    ├── 0000_10_config-operator_01_console.crd.yaml
    ├── 0000_10_config-operator_01_dns.crd.yaml
    ├── 0000_10_config-operator_01_featuregate.crd.yaml
    ├── 0000_10_config-operator_01_imagecontentsourcepolicy.crd.yaml
    ├── 0000_10_config-operator_01_image.crd.yaml
    ├── 0000_10_config-operator_01_infrastructure.crd.yaml
    ├── 0000_10_config-operator_01_ingress.crd.yaml
    ├── 0000_10_config-operator_01_network.crd.yaml
    ├── 0000_10_config-operator_01_oauth.crd.yaml
    ├── 0000_10_config-operator_01_project.crd.yaml
    ├── 0000_10_config-operator_01_scheduler.crd.yaml
    ├── doc.go
    ├── register.go
    ├── stringsource.go
    ├── types_apiserver.go
    ├── types_authentication.go
    ├── types_build.go
    ├── types_cluster_operator.go
    ├── types_cluster_version.go
    ├── types_console.go
    ├── types_dns.go
    ├── types_feature.go
    ├── types.go
    ├── types_image.go
    ├── types_infrastructure.go
    ├── types_ingress.go
    ├── types_network.go
    ├── types_oauth.go
    ├── types_operatorhub.go
    ├── types_project.go
    ├── types_proxy.go
    ├── types_scheduling.go
    ├── zz_generated.deepcopy.go
    └── zz_generated.swagger_doc_generated.go
```

Then, makefile targets will be added to `openshift/api` based on the [existing build machinery targets](https://github.com/openshift/library-go/blob/27b03247913b2f6a0abcea95cb9b6c896adc2531/alpha-build-machinery/make/targets/openshift/crd-schema-gen.mk) that currently enables OpenAPI schema generation. This generator will then be run when any API changes require
an update to CRD manifests, and manifests will be verified by CI, similar to how these changes are currently tested in many repos.

These updates will be implemented on a per-group basis, controlled by the `paths` parameter that is [provided by the generator](https://github.com/openshift/library-go/blob/master/alpha-build-machinery/make/targets/openshift/crd-schema-gen.mk#L20).
We can control the list of enabled groups by simply configuring a list of API paths to be passed to that parameter (see one example [here](https://github.com/openshift/cluster-config-operator/pull/89/files#diff-b67911656ef5d18c4ae36cb6741b7965R34)). There are obviously different ways to implement this list, but as long as it can be parsed and passed to the `paths` parameter of the
generator, only those paths will be picked up.

Operators will then reference these manifests by vendoring their respective packages and either
1. Symlinking their current manifest directories to the vendored path or
2. Referencing the vendored path directly in their code

### Risks and Mitigations

The main risk is that by moving all manifests to the same repository under the same generator, we create a single vector for potential
bugs to affect all operators. In other words, the benefit of being able to make organization-wide changes easily is also a risk.
However, for these changes to take effect will still require bumps in individual operator repos, so the risk of a bug becoming
widespread is still mitigated by the need for maintainers to approve the change in their repos.

## Design Details

### Test Plans

Testing for many of these operators is already covered in functional end-to-end tests, and for the generation itself in `library-go`, so
there should not be much need for additional tests as this is essentially an organizational change.

## Implementation History

## Drawbacks

## Alternatives