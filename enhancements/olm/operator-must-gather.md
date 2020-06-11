---
title: operator-must-gather
authors:
  - "@shawn-hurley"
reviewers:
  - "@ecordell"
  - "@njhale"
approvers:
  - "@ecordell"
  - "@njhale"
creation-date: 2020-1-6
last-updated: 2020-1-6
status: implementable
see-also:
  - "./enhancements/must-gather.md"
  - "./enhancements/oc/inspect.md"
---

# Operator Must Gather

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions [optional]

## Summary

Components developing [must gather images]("../oc/must-gather.md#must-gather-images") to gather all the information that they will need for debugging has become the defacto mechanism for customers to father information for a support case. When debugging installation of an operator with OLM we need to gather data from objects across the cluster. 

## Motivation

Today back and forth between the customer and the operator team or the OLM team to get info about an operator failing install. This the current state or each operator needs to write their own must gather to capture everything. Operator authors should have a starting point for writing their own must gathers such that gathering the OLM information for their operator should be consistent. 


### Goals

List the specific goals of the proposal. How will we know that this has succeeded?
1. Gathering should gather exactly the operator and the resources created by OLM.
2. A library that can be used to enable this functionality across multiple commands.
3. A small set of user facing CLI flags. 
4. Usable alongside an optional operators must gather or they could integrate with the library.
5. Should be 100% functional when not in a openshift environment.

### Non-Goals

1. Anything related to the wider cluster. This is not meant to be a OCP debug tool.
2. Specficics for a given operator. This is meant to help debug install of the operator, not the operator's functionality. 
3. Handle low level install scenarios, such as manually creating a CSV after applying RBAC.

## Proposal

A library, in the [api](https://github.com/operator-framework/api/tree/master/pkg) repo that will be vendored by `OLM` and `operator-sdk`. This library will include all of the traversal logic for retrieving OLM Operators data. 

The library will include a set of pflags that can be used by commands to create the options structure.

OLM will create a must-gather image and productize it. Allowing customers to run this must gather when needed.

### Implementation Details/Notes/Constraints [optional]

#### Constraints
This enhancement is only meant for operators that are deployed by OLM. The entrypoint should be the `subscription`.

### Risks and Mitigations
- No environment variables will be outputed from a pod.
- We will not save any secret data that may have been created. 

## Design Details
* The library options will allow for the implmenter to control what is searched and how

```type OperatorInspectConfig struct {
       // A kubeconfig to be used to contact the cluster
       Kubeconfig *rest.Config
       // PackageName used to filter to a particular operator by it's package name. If empty all packages will be used.
       PackageName string
       // Namespace to look for subscription objects
       // if left blank, will search the entire cluster for subscription.
       Namespace   string
       Writer io.Writer
}
```

* A Inspector interface will be returned
```
type Inspector interface {
  Inspect() error
}

// Will return the inspector that can be used for insepcting operators from OLM.
func New(configArgs ...InspectorArg) (Inspector, error)
```

The Inspector interface will be responsible for gather the following resource types:
1. Subscription 
2. InstallPlan
3. Resources created from the install plan (CSV's, RBAC, CRD)
4. Operator Deployment created by CSV


### Test Plan
- an e2e test will be used to validate that the command can:
1. Overwrite the writer interface and retireve the results

### Graduation Criteria
- v1 will be considered GA once it is integrated with.

### Upgrade / Downgrade Strategy
- The library must be backwards compatible once integrated with.
- The library will be considered v1, and will follow go-mod versioning by creating a v1 path. 

### Version Skew Strategy
Must conform to the must-gather insterface and set the version `/must-gather/version`.

## Implementation History

## Drawbacks

## Alternatives
