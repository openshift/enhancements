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

`oc adm inspect` has been a popular command that was added to the `oc` command. This command helps admins, support, and developers gather information for the controlers and resources that they are managing/writing for debugging. Operators deployed by OLM have their own set of resources that must be captured for easy debugging. These resources may be spread accross multiple namespaces, may include many different resources such as RBAC rules, and OLM specific resources. This means we need to teach `oc adm inspect` to understand OLM resources, such as subscription.

## Motivation

Similar motivation for the [must gather]("./enhancements/must-gather.md#motivation") enhancement. This will be a tool that all types of operators shipping through OLM can use during support of their operator. Similar principles to the `must-gather` will be followed such as `It must be simple` and `this tool is about the first shotgun gathering so you only have to ask the customer once.`. 


### Goals

List the specific goals of the proposal. How will we know that this has succeeded?
1. Gathering should gather exactly the operator, the CR's and relatedObjects for that operator, and the resources created by OLM.
2. A library that can be used to enable this functionality across multiple commands.
3. A small set of user facing CLI flags. 
4. In a failing cluster, gathering should be maximized to collect everything it can even when part of it fails.
5. Should be 100% functional when not in a openshift environment.

### Non-Goals

1. Anything related to the wider cluster. This is not meant to be a OLM debug tool.

## Proposal

A library, in the [api](https://github.com/operator-framework/api/tree/master/pkg) repo that will be vendored by `oc` and `operator-sdk`. This library will include all of the traversal logic for retrieving OLM Operators data. 

The library will include a set of pflags that can be used by commands to create the options structure.

### Implementation Details/Notes/Constraints [optional]

#### Constraints
This enhancement is only meant for operators that are deployed by OLM. The entrypoint should be the `subscription`.

### Risks and Mitigations

- UX will be provided by the command that implements the library.
- No environment variables will be outputed from a pod. 

## Design Details
* Considering that many different CLI's will be interacting with the library, the library will create an interface that the CLI's will need to implement to write files to disk.
* The library will have a default version of this interface internally

```
type ObjectWriter interface {
  // object is the object to be written. The implmenter can choose the output format.
  // file is the relative path, from the starting directory of the inspect command, for the object.
  Write(object interface{}, file string) error

  // Used to determine if the root directory to be used by the library is valid.
  IsDirViable(dir string) error

}
```

* The library options will allow for the implmenter to control what is searched and how

```type OperatorInspectConfig struct {
       // A kubeconfig to be used to contact the cluster
       Kubeconfig *rest.Config
       // PackageName used to filter to a particular operator by it's package name. If empty all packages will be used.
       PackageName string
       // Namespace to look for subscription objects
       // if left blank, will search the entire cluster for subscription.
       Namespace   string
       Writer ObjectWriter
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
3. ClusterServiceVersion(CSV)
4. owned CustomResourceDefinition(CRD)
5. The resources(CR) from the list of defined CRD's
6. Resources that that are defined as a `relatedObject` and are owned by a CR


### Test Plan
- an e2e test will be used to validate that the command can:
1. retrieve a known set of data for a given operator
2. retrieve a know set of data for an operator and it's depdencies data

### Graduation Criteria
- v1 will be considered GA once it is integrated with.

### Upgrade / Downgrade Strategy
- The library must be backwards compatible once integrated with.
- The library will be considered v1, and will follow go-mod versioning by creating a v1 path. 

### Version Skew Strategy
Version skew for the commands will be handled by the commands.

## Implementation History

## Drawbacks

## Alternatives
