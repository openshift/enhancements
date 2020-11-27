---
title: separating-provider-specific-code-in-the-installer
authors:
  - "@janoszen"
reviewers:
  - "@Gal-Zaidman"
approvers:
  - TBD
creation-date: 2020-11-27
last-updated: 2020-11-27
status: provisional
see-also: []
replaces: []
superseded-by: []
---

# Separating provider-specific code in the installer

## Release Signoff Checklist

TBD

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA

## Summary

This change introduces an internal API in the installer code to move provider-specific code into a separate folder and avoid having large switch-case structures in the installer code.

## Motivation

Several parts of the installer code have large, [unwieldy switch-case structures](https://github.com/openshift/installer/blob/254680f6268a7b8e57091d788256c67b81755b5d/pkg/asset/cluster/tfvars.go#L166) that contain provider-specific codes. This makes it hard to keep track of the code that serves a provider, and it makes it impossible to write component-level tests. Creating a separated API reduces friction between teams and makes changes easier to merge because the risk of accidental side effects between providers is reduced. 

### Goals

Create a Go interface and the hooks in the installer code that separatees the installer core code from the provider-specific code.

### Non-Goals

Remove all provider-specific pieces from the code. That would require a complete refactor of the install config data structure and move the Terraform code. 

## Proposal

### Step 1: Create a Go interface

We should create a Go interface that describes the functionality each provider must implement. For example, from the prototype:

```go
type PlatformV2 interface {
	// Name returns the name of the platform.
	Name() string

	// Metadata translates the installConfig to the appropriate entry in clusterMetadata.
	Metadata(clusterMetadata *types.ClusterMetadata, installConfig *installconfig.InstallConfig) error

    //...
}
```

Not all platforms implement all features. Optional features should be hidden behind a feature flag and a separate interface:

```go
var ErrNotAnIPIPlatform = errors.New("...")

type PlatformV2 interface {
    //...

    // GetIPI returns the IPI-specific interface, or ErrNotAnIPIPlatform if the platform does not support IPI.
	GetIPI() (IPIPlatformV2, error)
}

type IPIPlatformV2 interface {
    // IPI-specific 
}
``` 

### Step 2: Implement registry

In this step we should implement a registry pattern where providers can register with their names.

### Step 3: Implement feature-switch

Once the registry is complete the installer core code must be changed. Provider-specific switch-case structures can be identified by searching for the usage of the `Name` constants in the `types` package. The existing switch-case should be wrapped in an if-else case. This will help with keeping the functionality of providers that have not yet switched.

```go
if v2, err := registry.Get(providerName); err == nil {
    v2.CallSpecifiedFunction(...)
} else if (!errors.Is(err, registry.ProviderNotFound)) {
    return err
} else {
    switch (...) {
        //...
    }
}
```

### Step 4: Migrate providers

Providers need to be migrated to the new API.

### Step 5: Remove fallback code

Once all providers are migrated the fallback code can be completely removed.

### Risks and Mitigations

As with every refactor, this change contains the risk of breaking existing functionality. This can be mitigated by migrating only small portions of the code to the new API at a time.

## Design Details

### Open Questions

1. How should the API look in detail?
2. How should the implementation be merged? All-at-once, or bit by bit?

### Test Plan

Same as testing of the installer for regular releases.

### Graduation Criteria

TBD

### Version Skew Strategy

As the API develops it presents the problem of version skew. As new API calls are introduced they need to either be added to all providers. If that is not possible a new interface should be introduced and a compatibility wrapper should be added. Once all providers are migrated the temporary interface can be removed.

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

### Maintenance of the API

The API needs to be maintained and adding/removing API calls is a multi-step process.

### Slower implementation of a new hook

Until now adding hooks to the core installer code has been easy since the code has been added directly to the installer core code. Now a new hook is time-consuming and API design must be considered. 

## Alternatives

Leave the code as it is.