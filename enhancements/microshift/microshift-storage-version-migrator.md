---
title: microshift-storage-version-migrator
authors:
  - eggfoobar
reviewers:
  - "dhellmann"
approvers:
  - "dhellmann"
api-approvers:
  - None
creation-date: 2023-07-17
last-updated: 2023-07-25
tracking-link:
  - https://issues.redhat.com/browse/USHIFT-1473
---

# Embedding Storage Version Migrator

## Summary

We have embedded the
[Migrator](https://github.com/openshift/kubernetes-kube-storage-version-migrator/tree/master/cmd/migrator)
and
[Trigger](https://github.com/openshift/kubernetes-kube-storage-version-migrator/tree/master/cmd/trigger)
controllers in MicroShift to assure successive upgrades to MicroShift do not
cause a situation where a an API no longer exists for resource versions stored
in ETCD.

## Motivation

We wanted to provide support in MicroShift for the same solution in OpenShift
for automatic storage version migrations (i.e. v1alpha1 -> v1beta1 -> v1). The
current design in OpenShift revolves around running the [Migrator
Operator](https://github.com/openshift/cluster-kube-storage-version-migrator-operator)
which only deploys the Migrator controller. Migrations are then left up to the
Operator owners to kick off when changes happen to their respective APIs.

In MicroShift we don't run the full scale of OpenShift so we can trigger
migrations on start up to make sure resources are updated for future upgrades.
Furthermore, things will continue to run smoothly when users install an Operator
that might run with the assumption that a Migrator is running in the cluster and
thus try and kick off a migration for it's resource.

### User Stories

As a user, I want to upgrade MicroShift with out worrying about the storage
state of the resources already applied to the cluster.

### Goals

- On start up, MicroShift should run the Migrator controller as a service
- On start up, MicroShift should run the Trigger controller as a service
- MicroShift, should keep the resources updated to the Preferred Server Resource

### Non-Goals

-

## Proposal

We have proposed and implemented a solution that imports the Migrator and
Trigger logic into MicroShift to run as part of the core services. The services
launch on boot and stay running like typical controllers.

This approach was the most straight forward way to avoid pulling in more images
at start and keep our footprint small, as well as delegate the migration logic
to the pre-existing solutions and supply the API objects that other Operators
would use for migrating their own resources.

Areas where we can improve is by shutting down the Trigger service once the
initial migrations are done. Given the nature of MicroShift and its deployment
strategy, running a trigger the whole time is not necessary.

### Workflow Description

**Device Administrator** is a human user responsible for managing Edge Devices.

1. Device Administrator starts MicroShift
2. MicroShift launches the Migrator and Trigger service
3. The Trigger controller discovers preferred API resources
4. API resources are compared against pre-existing stored state via
   `StorageVersionHash`
5. If different, a `StorageVersionMigration` is created for the given API
   resource
6. The Migrator controller picks up the requests and sequentially updates all
   desired resources

### API Extensions

None

### Implementation Details

MicroShift imports the go modules for the Migrator controller and the Trigger
controller. The services are kicked off at launch and managed by the MicroShift
process. The below detail describes what the Trigger and Migrator logic do
behind the scenes, from a MicroShift perspective that is all owned by the
respective upstream components.

On Start the Trigger controller will query the Discovery API to get a list of
the API Server preferred API Resources, (this api request needs to be done in
legacy mode, see Risks and Mitigations). Each APIResource will contain a
`StorageVersionHash` that represents a sha256 sum of `group/version/kind`
truncated to 8 characters and then base64 encoded. This value is stored in a
`StorageState` CR during discovery. They then are compared, if there is a
difference between the `StorageVersionHash` in the discovery information and the
one stored in the `StorageState` for that specific resource, a
`StorageVersionMigration` CR is created by the trigger.

On start the Migrator will create a lister for `StorageVersionMigration`
resources, and wait for a migration request for a resource. Once a migration
request is received, it will query all of the resources for that version of that
API. It will then sequentially migrate them one by one. Migrating a resource
means querying the object, altering the version and performing a noop update.
The result of this action is stored in the status for the
`StorageVersionMigration`.

### Risks and Mitigations

We do have some risks with the feature as a whole but the mitigations are pretty
straight forward.

1. A risk that we have with this feature is that currently there is no real
   upstream solution for storage version migrations. The current implementation
   is in alpha and it does seem like it will be revisited in the future.
   Luckily, until there is more consensus upstream we can continue to use the
   approach outlined here but we will need to keep in mind that we might need to
   re-work this in the future.

2. In that same vein, we do run the risk that the code doesn't get touched much
   due to the nature and life of the current Storage Version Migrator solution.
   So we might run into dependency problems in the future. The good news there
   is that we have good ties to the upstream developers so the procedure for
   resolving any future dependency problems would be the same as the other
   services we run.

3. Another risk is that the new [Aggregated
   Discovery](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/3352-aggregated-discovery)
   removes the `StorageVersionHash`, luckily the change does take into account
   that the Migrator uses it and we just need to make sure our [discovery client
   is in legacy
   mode](https://github.com/kubernetes/enhancements/tree/master/keps/sig-api-machinery/3352-aggregated-discovery#is-the-rollout-accompanied-by-any-deprecations-andor-removals-of-features-apis-fields-of-api-types-flags-etc)

4. Lastly, migrations can fail, so there can be situations when a migration
   might fail on a specific resource. We will be able to catch these failures in
   our own e2e tests for our resources. However, when it comes to user
   resources, it is up to the users to update those resources to resolve the
   failure.

### Drawbacks

The technical aspect of this addition is relatively simple, some drawbacks that
we face are that we now have two services added to our code base. This does mean
a small increase in CPU usage by the core MicroShift binary and more code for us
to keep track of.

### Open Questions

In our look at this tool we found out that we don't run the trigger controller
in OpenShift due to the potential problems that might arise from mass updates of
large clusters and the traffic load on API Server during migrations. This really
isn't a concern for us, but it does beg the question, do we need to run the
trigger logic at all or is it worth us shipping `StorageVersionMigration` with
upgrades?

### Test Plan

The end-to-end testing procedure is implemented for MicroShift integration with
`Robot Framework` simulates the following use cases:

- System should start up with no migrations
- A custom CRD is applied with an `v1beta1` version
- A restart occurs, and a migration is requests for `v1beta1`
- The custom CRD is updated to `v1` version
- After 10min or a restart, a new migration should occur for `v1`

### Graduation Criteria

N/A

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation completed and published
- Sufficient test coverage
- Gather feedback from users
- Available by default

#### Tech Preview -> GA

- Sufficient time for feedback
- End-to-end tests

### Upgrade / Downgrade Strategy

On start, the Migrator will update and keep track of the status in the form of
`StorageVersionMigration` resources per for API resources.

### Operational Aspects of API Extensions

N/A

#### Failure Modes

N/A

#### Support Procedures

Storage migrations can always be checked via the API server. API resource
versions with failed upgrades should be manually updated.

```bash
oc get storageversionmigrations
```

## Implementation History

N/A

## Alternatives

In my searching I did not find a tool alternative for this issue. However, we
did try some alternative ways at approaching this problem for our needs.

Originally the idea was to simply pull in or re-create the logic for the
migrations so that we can run them as part of our startup with out `kubelet` or
`CRIO` running. We would only have `etcd` and the `kube-apiserver` up. This was
successful in implementation but we ran into a problem when dealing with `CRDs`,
because they can implement a [conversion
webhook](https://kubernetes.io/docs/tasks/extend-kubernetes/custom-resources/custom-resource-definition-versioning/#webhook-conversion),
we would need `kubelet` and `CRIO` running in order to successfully run a
migration.

The other alternative, was to simply deploy the Migrator and Trigger as
components. This would mimic the way we currently deploy other components but it
would mean two extra deployments and pods. On the surface that's not too bad but
that would mean pulling in two extra images and running two different runtimes.
With our goal to have our platform stay within a CPU and Memory/Disk budget it
would be ideal to avoid bringing on more pods.
