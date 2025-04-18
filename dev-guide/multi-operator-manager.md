# MultiOperatorManager

## Summary

HCP and standalone topologies face a problem of inconsistency and duplicate effort to introduce features
because they are two entirely distinct mechanisms for managing the same operands.
While some differences are absolutely required (deployments vs static pods), many are simply capricous.
This is about finding a way to safely refactor ourselves into a single path for managing these operands.

## Motivation

The duplicate effort, delays caused by that duplicate effort, and fragmentation of the ecosystem as
OCP supports different features in same version of different form factors is painful for customers,
platform extension authors, and developers.
Additionally, it is impractical to expect HCP to be an expert in managing (install, configure, and update)
every operand on the management cluster.
Finding way to keep operand teams aligned to how the operand is managed in every form factor is beneficial,
finding a way to do this without simply requiring more effort is even better, and providing a mechanism to 
improve testability at the same time is better still.
This enhancement aims at improving all of those.

### Goals

* Keep teams aligned to units of function across all form-factors (desired by everyone I think)
* Dramatically reduce overhead of running operators (single-node and HCP constraints)
* Dramatically reduce the delta between standalone and hypershift (all of us feel this pain)
* Dramatically improve testability of operators (all of us feel this pain)

### Non-Goals

* Reimplement operators in some meaningful way. We're shooting for a succesive refactor.
* Change any customer facing API

## Proposal

### Definition of Terms
Before we get into the changes, we need to define some terms.

1. configuration cluster - this is a theoretical (doesn't really exist) kuberentes cluster that contains
   the configuration that the user provides and the operators interact with.
   This is mostly where config.openshift.io is, but can contain other things.
   Recall that on standalone this is the one cluster, but on HCP will be the management cluster and in future
   if we ever create a separate configuration server in standalone, it will live there.
2. management cluster - this is the cluster where any non-config resource that exists in the HCP management
   cluster needs to be listed.
   Standalone doesn't have this concept, but we need to indicate which deployments (and the config leading to them)
   need to exist in the HCP management cluster.
3. user workload cluster - this is the cluster where users run their workloads.
   Many operators run things like daemonsets in this category.
   For instance, CSI driver daemonsets, CNI daemonsets, DNS daemonsets, image registry deployment, etc.

### MultiOperatorManager
Currently, every operator establishes its own client connections to the kube-apiserver, fills its own caches,
issues its own updates, and is permanently running to be responsive.
Instead of running each operator individually, we will run one binary (the MOM) that can communicate to the kube-apiserver.
It will have a single set of caches and it will decide which operators need to react to changes.
To make this decision, it will know what resources those operators depend upon, but can also rate limit by
1. debouncing - waiting a few seconds after the first input change to collect more.
2. maximum concurrency - only X operators may be working at the same time
3. cool down - an operator must wait at least M seconds before being called again
4. operator qps - a single operator can only make N changes per minute
5. any other metric we like.

This allows us to limit the peak memory usage and server QPS.
It also allows us to detect conflicts between operator and eliminate hot looping.

The refactor that makes this possible also makes it possible to easily expand our test coverage and reuse the output
of standalone operators on HCP.

### The refactor
We will refactor existing operators to continue running as they do today and also fulfill the
contract of these new commands.

1. `operator input-resources` - outputs a list of kube API input resources that this operator needs
2. `operator output-resources` - outputs a mapping of kube API output resources from this operator
   and whether these resources are for configuration, management, or user workload clusters.
3. `operator apply-configuration --input-dir=<must-gather-like-dir> --output-dir=<dir which-will-list-content-to-write>` -
   takes a must-gather like directory of input and produces an output directory containing content to apply to the cluster.
   Logically, the output is something you would `kubectl create -f <output-dir>/create`, `kubectl apply -f <output-dir>/apply`,
   etc for each mutation verb.
   This command is not provided any kubeconfig and has all network access cut off.
   Doing this makes for something extremely easy to integration test.
4. `operator apply-configuration-live --input-dir=<must-gather-like-dir> --output-dir=<dir which-will-list-content-to-write>` -
   similar to apply-configuration, but this one will get a kubeconfig file to the user-workload cluster and limited
   (determined by TBD proxy configuration) to reach out to other endpoints.
   This is useful for cases where external access is necessary, like to a cloud provider or an external identity provider.
   These commands are encouraged to write back enough to the configuration cluster so that testing flows are very
   easy for each command.
   We do not yet know how we'll get a NO_PROXY configuration for each operator.
5. TBD - some way to determine the service account to use for mutation for each operator.

### Prereqs to such a refactor
There are several pieces that are needed prior to investing in a refactor like this across multiple teams.

1. Ability to read a must-gather and feed a functional fake client that can support informers.
2. Ability to track writes using a client and serialize them for later.
3. Types to define input and output resources.
4. Ability to prune a must-gather to just the content needed for feeding an operator.
5. Ability to declaratively define an operator test for verification.
6. Ability to avoid updating the same resource multiple times (the perma-conflict problem).
7. Sufficient refactoring to avoid the obvious cases of perma-conflict.
8. Ability to know which service account to use for each operator (out of band?).
9. Easy Makefile inclusion of integration testing.
10. Refactor at least one operator to demonstrate this actually works.
11. MOM to enforce HTTP_PROXY rules on operator.
12. IndividualMOM created that is able to handle each request type.
13. Stretch: refactor cluster-authentication-operator to use IndividualMOM

### Suggested flow in managing the refactor
In order to avoid disrupting our existing OCP standalone product, it is important to refactor for equivalence
instead of simply writing something entirely new.
To support this we have a pattern that looks roughly like this.
1. Construct all the clients it will require in one function and returns a struct containing all those clients.
2. From the clients, create all the informers that are required.
3. From the informers, initialize all the control loops for the operator.
4. For each control loop, add a `RunOnce` function.
5. Create an alternative for initial clients built from `manifestclient.MutationTrackingClient`.
6. Wire `libraryapplyconfiguration.NewApplyConfigurationCommand` using the example in MOM.
7. Add `targets/openshift/operator/mom.mk` to the `Makefile` to get `test-operator-integration` and `update-test-operator-integration` targets.

Doing this
1. Leaves the existing standalone operator more or less untouched.
2. Ensures we don't miss control loops in the `RunOnce`.
3. Provides a testing mechanism even for the standalone control loops that is useful for establishing behavior.

### Workflow Description

### API Extensions

### Topology Considerations

#### Hypershift / Hosted Control Planes

##### HCP namespaces
Operators think in terms of standalone and this will remain for the foreseeable future.
HCP will need to adjust the input-resources and output-resources from standalone into something suitable for HCP.
This is a risk calculation.
Because every namespace, name, configmap, and secret are fully exposed in a standalone cluster,
we have no way of safely making sweeping changes to those end users, cluster-admins, and platform extensions
that rely on those names being stable.
HCP does not suffer from a comparable problem and can thus migrate to closer alignment with standalone
comparatively easily.

A key part of this simulated namespaces as operators expect to exist in a single cluster with many
well-known namespaces.

#### Standalone Clusters

#### Single-node Deployments or MicroShift

### Implementation Details/Notes/Constraints

### Risks and Mitigations

1. Mrunal is concerned that spawning processes is too expensive.
   This will be tested by comparing the cost of running operators in standard mode versus MOM mode amortized over 24h.
   Cost to take into account a weighting of the kube-apiserver load, peak memory usage, memory GB-seconds, and potentially more.
   Conclusions to consider the amortized cost of combining ~10 resident operators to one MOM.
2. 

### Drawbacks

## Open Questions [optional]

## Test Plan

## Graduation Criteria

### Dev Preview -> Tech Preview

### Tech Preview -> GA

### Removing a deprecated feature

## Upgrade / Downgrade Strategy

## Version Skew Strategy

## Operational Aspects of API Extensions

## Support Procedures

## Alternatives

## Infrastructure Needed [optional]

