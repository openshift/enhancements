---
title: converting-machine-api-to-cluster-api
authors:
  - "@JoelSpeed"
reviewers:
  - "@damdo" # Cluster API and Machine API maintainer
  - "@nrb" # Cluster API and Machine API maintainer
  - "@mdbooth" # OpenStack Cluster API and Machine API maintainer
  - "2uasimojo" # Hive maintainer
approvers: 
  - "@vincepri" # Cluster API maintainer within OpenShift
api-approvers:
  - "@deads2k"
creation-date: 2023-08-30
last-updated: 2024-03-19
tracking-link: 
  - https://issues.redhat.com/browse/OCPCLOUD-1578
see-also: []
replaces: []
superseded-by: []
---
...

# Converting Machine API resources to Cluster API

## Summary

To enable OpenShift to migrate from the OpenShift Machine API to the upstream Kubernetes Cluster API project,
we will need to create a mechanism that allows users to continue using the existing API,
while we internally migrate to using Cluster API controllers.
We will also promote, via the addition of newer features, users to migrate to using the Cluster API directly,
which in turn will allow us to deprecate and eventually, remove (TBC[^1]) the Machine API.

This document outlines the proposed method for that migration process.
This includes how Machines, MachineSets, MachineHealthChecks, the ControlPlaneMachineSet and the Cluster Autoscaler will be migrated.

[^1]: To remove the Machine API we must prove that only a suitably small number of our customers are still using the API to manage their machines.
For example, if the number was suitably small that we could reach out to each customer individually to help them move, then it would be feasible.
For the purposes of this document and this project, we assume that the Machine API will exist in OpenShift 4 forever,
even though the backing implementation will move to Cluster API.
In a future iteration, `ClusterFleetEvaluation` may be used to determine the number of clusters that are still using the Machine API.

## Motivation

The motivation for this project is primarily outlined in this [RFC][rfc-migration-to-capi].

To summarise, we believe that the long term sustainability of OpenShift depends on migrating from Machine API to Cluster API.
By leveraging the upstream Cluster API project, we will gain access to a community of users and developers, 
with which we can all work together towards the goal of Kubernetes native Node lifecycle management.

Having the entire lifecycle of an OpenShift cluster on a single and well-established platform brings in reusability, collaboration, and a more streamlined interface.
With adjacent teams and adjacent projects (Installer, HyperShift, ROSA via CAPA) all using the same APIs, it makes sense to deduplicate our efforts and work together on a single platform.

As we have production users presently using Machine API, and we are currently not sure which, if any,
extensions or automation they may have built on top of this, we will need to provide a way for users to migrate between the two APIs.

This means for a period[^2], both APIs will be supported within OpenShift.

Initially, we will create a mechanism that allows users to leverage both APIs and their respective controllers simultaneously within their clusters.
In a future release, we will remove the Machine API controllers and rely on our migration layer to keep the Machine API available,
but backed by the Cluster API controllers.

[^2]: Based on [^1], this period may actually be indefinite.

[rfc-migration-to-capi]: https://docs.google.com/document/d/1pUPBwLZ3hB1ekS0BquLNFP3RCoRTixo2PuRAfJJ40X8 "RFC: Long term sustainability and the future of the Machine API and Cluster API"

### User Stories

#### Story 1

As a cluster administrator, I would like to test migrating individual Machines and MachineSets across to Cluster API so that I can verify
the migration is working, with the ability to minimise the impact to running clusters.

#### Story 2

As a cluster administrator, I would like to be able to verify the conversion of my Machine configuration from Machine API to Cluster API,
before instructing the cluster to operate on the changed configuration,
so that I can check that the features I have tuned are converted correctly to the new configuration format.

#### Story 3

As a cluster administrator, I would like to test Cluster API before rolling it out to my production clusters, so that I can build confidence in the new APIs.
To do this, I would like to be able to add Cluster API MachineSets to add new Machines to my Cluster without having to have an equivalent Machine API MachineSet to match it.

#### Story 4

As a developer of Machine related APIs, I would like to be able to gather feedback on Machine conversion layers before forcing new APIs onto users.
By gathering feedback, we can ensure the conversion is working, and that we are not breaking existing users when they are upgraded into newer versions
of OpenShift that rely on Cluster API.

### Goals

* Allow users to migrate Machines between Machine API and Cluster API to test the new API
* Allow users a choice (initially[^3]) of when they want to migrate
* Eventually[^3] remove the Machine API controllers and leverage the migration layer to convert Machine API machines to Cluster API machines
* Allow the Cluster Autoscaler, MachineHealthCheck and ControlPlaneMachineSet to continue to work during a semi-migrated cluster state
* Ensure that every Machine API resource is translated to a Cluster API resource to allow users to test the new API
* Allow access to newer features of Cluster API that do not exist in Machine API
* Avoid carry patches in the Cluster API controllers to ensure that we can easily upgrade to newer versions of Cluster API

[^3]: In the first iteration of this project, the goal is to allow users to migrate individual resources.
In later iterations, the plan is to remove the Machine API controllers. The linked goals will be executed in separate releases.

### Non-Goals

* Setting up or configuring any part of Cluster API, this is handled in a separate enhancement
* Modifying any of the existing deployments of Machine API within clusters
* Deleting or modifying any existing resources within clusters (until a user chooses to do so)
* Force creation of new Machine API machines when new Cluster API MachineSets are being created[^4]
* Port any feature from Cluster API into Machine API

[^4]: We want to promote, over time, a reduction in the number of Machine API Machines, as such,
any resources created directly in Cluster API, will not be reflected into Machine API, unless they have an owner,
and, that owner is already reflected into Machine API.

## Proposal

We will implement a two-way sync controller that synchronises Machines and related resources between the Machine API and Cluster API equivalents.

Using a new field on the Machine API resource, users will be able to choose which API is authoritative.
Any discrepancy between the non-authoritative resource and the authoritative resource will be overwritten by the sync controller to match the authoritative resource.

When the API is non-authoritative, the controllers should be paused (ignore the resource),
allowing the authoritative API’s controller to perform the reconciliation actions required.

### Workflow Description

In this workflow, the **cluster admin** (or equivalent controller, e.g. Hive) is responsible for the infrastructure provisioning for the cluster.
Machine API and Cluster API represent different API group versions, but with similar resources.
This workflow allows switching management of the infrastructure resources (eg EC2 instances) from the former API, to the new API.

When the cluster admin wishes to migrate a MachineSet from Machine API to Cluster API, the following procedure is required:
1. Identify the MachineSet to migrate to Cluster API
1. Use the `oc edit` or a patch command to update the value of the `spec.authoritativeAPI` field to `ClusterAPI`
1. The migration controller verifies that the `Snychonrized` condition is currently set to `True`, verifying no long standing synchronization errors
1. The `status.authoritativeAPI` field is updated to `Migrating` by the migration controller
1. The Machine API controller acknowledges the change and sets the `Paused` condition to `True`
1. The sync controller ensures the latest changes are synchronised between the old authoritative resource and the new, the `status.synchronizedGeneration` is then updated to the current generation of the old authoritative resource
1. The migration controller verifies that the move from Machine API to Cluster API is valid by checking that the synchronized generation is up to date
1. The migration controller updates `status.authoritativeAPI`  to `ClusterAPI`
1. The Cluster API controller takes over management of the MachineSet going forwards

To migrate back from Cluster API to Machine API, the procedure is the same, however, the value of the field should be set to `MachineAPI`.
The transitional state will be `Migrating` again but the sync controller will wait for the Cluster API controllers to acknowledge
the change before allowing the Machine API controllers take over management of the resource.
In both cases, the combination of the `Migrating` status value and the spec `MachineAPI`/`ClusterAPI` value allows the migration controller
to determine which direction the intended migration is currently taking.

When the conversion cannot proceed for some reason, for example, a feature is in use in Machine API that is not present in Cluster API, the following will happen:
1. Cluster admin identifies a MachineSet to migrate to Cluster API
1. Cluster admin uses `oc edit` to update the value of the `spec.authoritativeAPI` field to `ClusterAPI`
1. The migration controller determines that the `Synchronized` condition is set to `False`, showing a long standing conversion error
1. The `status.authoritativeAPI` field is not updated and the `status.synchronizedGeneration` is not updated
1. The Machine API controllers continue to manage the MachineSet.
1. Where appropriate, the cluster admin contacts support for additional help/timelines for availability of the missing feature

Long standing syhcnronization errors, where the admin has requested a transition by changing the `authoritativeAPI`, will result in alerts firing to indicate to the
cluster admin that the transition is not occurring as they requested.

#### Workflow extension

When migration between the two authoritative API versions is not possible, the sync controller will add a condition to the resource to indicate the reason for the failure.
However, with the above described workflow, the user is not presented with a synchronous response to the migration request.

To improve the feedback loop, a webhook will be introduced to validate the migration request.

The migration request will always be persisted, however, the webhook will check for the presence of the sync controller's failure condition, and return a warning response to the user if the migration is not possible.

The users intention to migrate will be persisted, and the sync controller will continue to attempt to migrate the resource until the condition is resolved, but, the user is also informed of the reason for the failure synchronously.

This extension is not required for the minimum viable version of this project, but, will be added in a future iteration, prior to the GA release.

If the migration request is not reversed within some reasonable time frame, and alert will be fired to ensure the user has visibility that their request was not fulfilled.

### API Extensions

This enhancement introduces a new `authoritativeAPI` field on `spec` and `status` of Machine API resources.
Users will set the value of this field to determine which of the Machine API or Cluster API controllers will own the resource.

The `status` will also include a `synchronizedGeneration` field to indicate the generation of the authoritative resource that the non-authoritative resource is synchronised with.

Machine API conditions will also include a new `Synchronized` condition type, which will be used to indicate when the non-authoritative resource is fully synchronised with the authoritative resource.

The `Paused` condition will be added to Machine API and Cluster API (upstream first) to allow controllers to acknowledge the change in the authoritative API.

Admission control will prevent changes to non-authoritative resources that do not come from the sync controller, bar a small number of fields that are allowed to be updated by the user.

#### Authority of resources and controller reconciliations

To ensure that only one controller acts on any resource (Machine, MachineSet etc) at any one time,
a new field `status.authoritativeAPI` will be implemented.
The new field will be applied to each Machine API resource and will determine which controller is authoritative for the resource.

The accepted values for the field will be either `MachineAPI` or `ClusterAPI` (the respective API groups for Machine API and Cluster API).
A third value `Migrating` will be used to indicate that a transition between the two APIs is currently in progress.
If the field is not present, it will be added by the migration controller and defaulted to `MachineAPI`.

When the Machine API controller attempts to reconcile an object, it must first check for the presence of the field.
If it is not present, then the Machine API controller should reconcile the object.
It is expected that the migration controller will default it to `MachineAPI` in this case.

In Machine API, checks will be added at the beginning of each reconcile loop so that the controller can determine whether or not to reconcile the resource.
Where the resource is not authoritative, the controller should set the `Paused` condition to `True` and not perform any further reconciliation.

In Cluster API, the same checks will be performed, but instead by the sync controller.
The sync controller will leverage Cluster API's built in pause mechanism to prevent the Cluster API controllers from reconciling the resource. 

Machine API controllers will be allowed to reconcile any object that is either:

* Missing the authoritative field in the status (as today)
* Has the authoritative field set with the `MachineAPI` value

Cluster API controllers will be allowed to reconcile any object that either:

* Has a Machine API equivalent AND has the authoritative field set with the `ClusterAPI` value
* Does not have a Machine API equivalent

An admission time validation will be added to prevent Cluster API resources from being created, that are not paused, when they already have a Machine API equivalent.
These resources should be created by the sync controller, however, we need to make sure that they are created as paused, to prevent any unintended reconciliation in the first instance.

An admission time mutation to ensure the `status.authoritativeAPI` is set to match the `spec.authoritativeAPI` on create only, should also be implemented.

The APIs will be extended as follows:

```go
type xxxSpec struct {
  // authoritativeAPI is the API that is authoritative for this resource.
  // Valid values are MachineAPI and ClusterAPI.
  // When set to MachineAPI, writes to the spec of the machine.openshift.io copy of this resource will be reflected into the cluster.x-k8s.io copy.
  // When set to ClusterAPI, writes to the spec of the cluster.x-k8s.io copy of this resource will be reflected into the machine.openshift.io copy.
  // Updates to the status will be reflected in both copies of the resource, based on the controller implementing the functionality of the API.
  // Currently the authoritative API determines which controller will manage the resource, this will change in a future release.
  // To ensure the change has been accepted, please verify that the `status.authoritativeAPI` field has been updated to the desired value and that the `Synchronized` condition is present and set to `True`.
  // +kubebuilder:validation:Enum=MachineAPI;ClusterAPI
  // +kubebuilder:validation:Default:=MachineAPI
  // +default:=MachineAPI
  // +optional
  AuthoritativeAPI string `json:"authoritativeAPI,omitempty"`
}

type xxxStatus struct {
  // authoritativeAPI is the API that is authoritative for this resource.
  // Valid values are MachineAPI, ClusterAPI and Migrating.
  // This value is updated by the migration controller to reflect the authoritative API.
  // Machine API and Cluster API controllers use this value to determine whether or not to reconcile the resource.
  // When set to Migrating, the migration controller is currently performing the handover of authority from one API to the other.
  // +kubebuilder:validation:Enum=MachineAPI;ClusterAPI;Migrating
  // +optional
  AuthoritativeAPI string `json:"authoritativeAPI,omitempty"`

  // synchronizedGeneration is the generation of the authoritative resource that the non-authoritative resource is synchronised with.
  // This field is set when the authoritative resource is updated and the sync controller has updated the non-authoritative resource to match.
  // +kubebuilder:validation:Minimum=0
  // +optional
  SynchronizedGeneration int64 `json:"synchronizedGeneration,omitempty"`
}
```

These fields will be added to the following resources:
* Machine
* MachineSet
* MachineHealthCheck
* ControlPlaneMachineSet[^5]

[^5]: The ControlPlaneMachineSet is a Machine API resource that is not present in Cluster API.
We will extend the `template` field of the ControlPlaneMachineSet to support Cluster API in a separate enhancement.
These fields will be added when that enhancement is implemented.

#### Admission of non-authoritative resources

A webhook will be introduced that will be used for admission of all Cluster API and Machine API resources
that are involved in the sync operation.

It will be responsible for validating on updates that any update to a non-authoritative resource is only performed by the sync controller itself.

There will be a small number of fields that are allowed to be updated by the user:
* `.metadata.labels` - that are not critical to Machine API or Cluster API functionality, and are not in reserved Kubernetes or OpenShift namespaces, and do not conflict with the authoritative resource
* `.metadata.annotations` - that are not critical to Machine API or Cluster API functionality, and are not in reserved Kubernetes or OpenShift namespaces, and do not conflict with the authoritative resource
* `.spec.authoritativeAPI` - to allow the user to set the authoritative API on the Machine API resource even when it is not authoritative

Creation of resources will be monitored by the admission control and will be allowed to proceed as long as the authoritative field is set to the correct value.
* If a Cluster API resource already exists and a Machine API resource is created, the admission will succeed only if the spec authoritative field is set to `ClusterAPI`
* If a Machine API resource already exists and a Cluster API resource is created, the admission will succeed only if the spec authoritative field is set to `MachineAPI`

Deletion of resources will not be monitored by the admission controller.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This proposal does not affect HyperShift.
HyperShift does not leverage Machine API.

#### Standalone Clusters

This cluster is aimed at standalone clusters that leverage Machine API.
Typically this indicates that they were installed using the Installer-Provisioned-Infrastructure path, however,
we understand that a subset of User-Provisioned-Infrastructure installed clusters retro-fit MachineSets on day 2.

#### Single-node Deployments or MicroShift

Single Node and MicroShift do not leverage Machine API.

### Implementation Details/Notes/Constraints

#### Synchronisation of resources

To ensure that resources are synchronised, we will implement a new controller `machine-api-sync-controller`.
This controller will be responsible for translation of Machine API resources to Cluster API resources, and vice-versa.

This controller will be responsible for creating Cluster API versions of Machine API resources, when they do not exist.
It will observe the authoritative `status` field described [above][authoritative-api] to synchronise between the two resources,
overwriting anything in the non-authoritative resource that does not match with the authoritative resource.

When errors occur during synchronisation, the controller will write conditions to the Machine API resource to report the failure.

On successful synchronisation, the controller will update the Machine API resource's `status` to reflect the current synchronised generation of the authoritative resource.
This will allow users to keep track of, and ensure that the latest changes to the authoritative resource have been synchronised.

[authoritative-api]: (#authority-of-resources-and-controller-reconciliations)

#### Migration of resources

To allow the migration between Machine API and Cluster API resources and vice versa, we will implement a new controller `machine-api-migration-controller`.
This controller will be responsible for co-ordinating the hand over of authority between the Machine API and Cluster API controllers.

The controller will observe the `spec` and `status` authoritative API fields and look for a difference between them.
A difference indicating the cluster admin requested a migration between the two APIs.

When the `spec` and `status` differ, it will first check if the `Synchronized` condition is set to `True`, if it is not,
this indicates an unmigratable resource and therefore should take no action.

If the condition allows, the controller will set the `status.authoritativeAPI` to the `Migrating` value and wait for the old authoritative resource to set the `Paused` condition to `True`.

Once the `Paused` condition is observed, the controller will move the `status` to match the `spec` allowing the new authoritative API to take over.

This is the controller that will be responsible for pausing and un-pausing Cluster API resources as part of the handover.

##### MachineSet Synchronisation

MachineSets in Machine API map to a MachineSet and InfrastructureTemplate in Cluster API.
The sync controller will create the Cluster API MachineSet with the same name as the Machine API MachineSet to ensure a 1:1 mapping between these resources.
The InfrastructureTemplate will be named based on the MachineSet name and a hash of the content within it [^6].
InfrastructureTemplates are immutable, but MachineSet’s in Machine API are not; should the content of the MachineSet change,
a new InfrastructureTemplate will be created and the old template will be removed when no longer required.

The `providerSpec` forms the basis of the conversion to the InfrastructureTemplate in this case and is where errors may occur.
Some values from the `providerSpec` in Machine API are moved to the InfrastructureCluster resource (a cluster wide infrastructure reference) and as such,
if these values do not match that which is set on the InfrastructureCluster, we cannot convert losslessly.

As an example of this, we expect all Machines to exist in the same resource group in Azure today.
The resource group is a property of the InfraCluster in Azure and as such, cannot vary by MachineSet.
If a customer has somehow (we don't believe this is actually possible) created MachineSets that span resource groups,
then this would present as non-convertible.

To understand the likelihood of this, we can use existing CCX data [^7] to determine if there will be clashes in this data before we implement the controller. 

In cases where conversion fails, the controller will add a `Synchronized` condition to the Machine API MachineSet defining the errors present.

We will need to implement conversion logic for each of the currently supported platforms on OpenShift.
The environments which are typically easier to use (eg AWS, GCP, Azure) tend to have more options and will therefore be the harder platforms to migrate.
Platforms such as Baremetal and vSphere require very minimal input and the conversions in these cases will likely be easier.

Once the InfrastructureTemplate is created, this will be set as a reference on the Cluster API MachineSet's Machine template.
Other fields within the MachineSet can be copied verbatim and as such should not have issues in translation.

When Cluster API MachineSets are authoritative, if a MachineAPI MachineSet exists, the controller will synchronise the MachineSet and InfrastructureTemplate onto the Machine API MachineSet.
Since the Machine API MachineSet represents a superset of the Cluster API resources (for supported features), there should be no errors converting in this direction.
When newer features are leveraged in Cluster API MachineSets, backwards conversion will not be possible and the `Synchronized` condition will be updated to represent this.

Machines by default will be managed by the same API group controllers as the MachineSet that created them.
Therefore, when the Cluster API MachineSet is authoritative, new Machines created by it, will be managed by the Cluster API by default.

[^6]: Since a MachineSet may own multiple InfrastructureTemplates over its lifetime, we use a hash of the ProviderSpec content to ensure a unique name
whenever the content of the ProviderSpec is changed. If a change is reverted and the old InfrastructureTemplate is still in use,
this allows us to revert back to a previously existing template, rather than creating new templates sequentially that may be identical.

[^7]: CCX collect data that includes the MachineSet `providerSpec` for every cluster that is opted into telemetry and is using the MachineSet feature.
We can therefore ask them to extract the MachineSets from the data and provide us with enough detail to be able to compare, within each cluster,
if there are discrepancies that cannot be resolved by our existing planned logic for conversion.

##### Machine Synchronisation

Machine synchronisation will happen in much the same way as in MachineSets.
A Machine API Machine will map to a Machine and an InfrastructureMachine in Cluster API.

Independent of whether the Machine is standalone or owned by a MachineSet, the InfrastructureMachine will be generated in the same way as the InfrastructureTemplate is, as described in the MachineSet section.
We should not re-use the InfrastructureTemplate in this case as the MachineSet may no longer be using the same InfrastructureTemplate (or `providerSpec` in Machine API) as was used to create the Machine.

When Cluster API Machines are authoritative, if a Machine API Machine exists for the Cluster API Machine,
the controller will synchronise the Machine and InfrastructureMachine content onto the Machine API Machine.
Since the Machine API Machine represents a superset of the Cluster API resources (for supported features), there should be no errors converting in this direction.
When newer features are leveraged in Cluster API Machines, backwards conversion will not be possible and the `Synchronized` condition will be updated to represent this.

##### MachineHealthCheck Synchronisation

The fields on a Cluster API MachineHealthCheck form a superset of the Machine API MachineHealthCheck.
The additional fields are either optional or can be determined easily by the controller (eg. ClusterName).
Conversion of MachineHealthChecks should be relatively straightforward because of this.

When Cluster API MachineHealthChecks are authoritative, and the optional features of a Cluster API MachineHealthCheck are being leveraged, this will not be synced onto the Machine API MachineHealthCheck.
This restriction is known and will be documented.
Additional features provided by Cluster API will not be ported to Machine API as part of this effort.

##### ControlPlaneMachineSet Synchronisation

ControlPlaneMachineSets are not present in Cluster API.
However, the ControlPlaneMachineSet was designed in such a way that we can extend the `template` section (which is a discriminated union) to support Cluster API.

A separate enhancement will be created for the support of Cluster API in the ControlPlaneMachineSet, however, for the sake of conversion,
we expect the conversion to happen much in the same way as a regular MachineSet.

ControlPlaneMachineSets (additionally to MachineSets) have a concept of failure domains.
This is also a concept in Cluster API.
When converting ControlPlaneMachineSets to Cluster API, we will ensure that the appropriate failure domain configuration is configured inside the Cluster API InfrastructureCluster,
and that the failure domains from the ControlPlaneMachineSet Machine API configuration are reflected across.

When deploying Machines using the future Cluster API extension, the InfrastructureTemplate will be combined with the named failure domains on the InfrastructureCluster
to spread the Machines across multiple failure domains.

#### Creation of new Machines

When a Machine API MachineSet creates a new Machine, the sync controller will create the equivalent Cluster API Machine based on the method described in [Machine Synchronisation](#machine-synchronisation).

When a Cluster API MachineSet creates a new Machine, the sync controller will create its Machine API equivalent if, and only if, the Cluster API MachineSet has a Machine API equivalent.
This will allow users to switch a MachineSet back to Machine API at a later date if they wish to do so.

When the Cluster API MachineSet has been created independently of Machine API, new Machines will not be created in Machine API to reflect the Machines owned by the Cluster API MachineSet.

Optionally, a user will be able to configure a Machine API MachineSet to create Machines that are authoritative in Cluster API from creation.
By setting the `spec.template.spec.authoritativeAPI` to `ClusterAPI`, the Machine API MachineSet will create Machine API Machines with their `authoritativeAPI` pre-filled to `ClusterAPI`.
This will trigger the creation of the Cluster API machine and the Cluster API controllers will implement the creation of the new Machine.
This will eventually become the default action, but will allow users to test the Machine creation flow in Cluster API without having to move over to using a Cluster API MachineSet.

#### Deletion of Machines

To enable the synchronisation of deletion, the sync controller will use its own Finalizer `sync.machine.openshift.io/finalizer` on resources that have Machine API and Cluster API equivalents.

This means that for a Machine, we expect the authoritative controller to perform the actual deletion work within the infrastructure provider.
Once it has completed this effort, it will remove its own Finalizer.

As soon as the synchronisation controller notices the Machine has been deleted, it should ensure that the Machine API/Cluster API equivalent (if present) is also deleted.
It should then wait until the authoritative Machine controller removes its finalizer before removing both its own, and the Machine API/Cluster API Machine finalizer if present.
In scenarios where both Machine API and Cluster API have been authoritative, it is expected that both Machine controllers will have added their own Finalizers, therefore we expect the synchronisation controller should be able to handle this.

The migration controller should, as part of the handover mechanism of authority, handle moving the Finalizer between the old and new authoritative resources when appropriate.
The migration controller must first ensure the snyhconrization is up to date and then, prior to switching the `status.authoritativeAPI`,
first add the Finalizer to the new resource, and then remove the Finalizer from the old resoucre once it has observed the event persisting the addition of the Finalizer on the new resource.

It is also feasible that a customer may want to remove the Machine API resources after they have migrated to Cluster API and no longer require the Machine API synchronisation.
To allow for this, if a non-authoritative Machine API parent[^8] resource is deleted, the deletion event will not be synchronised to the Cluster API equivalent and the synchronisation controller will ensure finalizers are removed as appropriate.
In this scenario, the Cluster API equivalent will continue to operate on its own.
To ensure users are aware of this situation, webhooks will be introduced to warn users that the deletion will not be synchronised.

[^8]: To allow continued operation of MachineSets, ControlPlaneMachineSets and MachineHealthChecks, if a Machine API Machine that is owned is deleted,
then the deletion event is synchronised to Cluster API.
This will allow for a MachineSet/ControlPlaneMachineSet to own mixed instances (i.e. those managed by both Machine API and Cluster API), and continue to support higher level remediation automation.
To remove the Machine API resources, the user should delete the parent, i.e. the MachineSet or ControlPlaneMachineSet on the Machine API side.

#### Scenarios with mixed Machines

The following scenarios explain the logical flow of what must happen if any of the controllers are monitoring a selection of Machines where the authoritative API is mixed between Cluster API and Machine API.

When the selection of Machines all match the same API, the normal flows will be observed.

##### MachineSet scale up

If a MachineSet scales up, it will create a new Machine as it does today.
Because the managed resource in this scenario is the MachineSet the sync controller will ensure that the Machine resources are reflected in both Machine API and Cluster API.

If the MachineSet is a Cluster API MachineSet and there is no Machine API equivalent, this synchronisation will not happen and the machines will be created as Cluster API only.

New Machines created in the scale up will default to the same authoritative API as the MachineSet that creates them.

##### MachineSet scale down

If a MachineSet scales down, this will trigger the deletion of a Machine resource
To ensure consistency between the two MachineSets, the sync controller will synchronise the delete event between the Cluster API and Machine API Machines.

##### MachineHealthCheck remediates Machine

As MachineHealthChecks only remediate Machines that have owners, when the deletion event occurs, the deletion event will be synchronised across between the two APIs.

##### Resource created as Cluster API Resource, wish to migrate to Machine API Resource

To enable a reverse migration, should the customer wish, we will need to enable the user to create a Machine API equivalent of a Cluster API resource if it does not exist.

The user will be able to do this by creating the Machine API equivalent resource and ensuring that the `authoritativeAPI` is present from creation and is set to `ClusterAPI`.
This will trigger the sync controller to copy the spec/status from the Cluster API resource onto the Machine API resource.

At this point, the customer can switch back the `authoritativeAPI` to `MachineAPI` to allow the Machine API controllers to take over the management of the resources.

Creating the Machine in Machine API with the authoritative API set to `MachineAPI` will be considered an error and the API call will be rejected.

#### Summary of the rules outlined above

The following table describes the actions for Machines based on the API to which the action was taken, the action type and whether the Machine has an owner or not.

| Machine Type | Authoritative API | Action | Has Mirror | Has owner | Outcome |
| --- | --- | --- | --- | --- | --- |
| Machine API | `MachineAPI` | Create | No | - | Sync controller creates Cluster API equivalent and starts synchronisation from Machine API to Cluster API for future updates |
| Machine API | `MachineAPI` | Create | Yes | - | Error - this should be rejected by a webhook |
| Machine API | `ClusterAPI`| Create | Yes | - | Sync controller syncs Cluster API spec to Machine API Machine |
| Machine API | `ClusterAPI`| Create | No | - | Sync controller creates Cluster API equivalent and starts synchronisation from Cluster API to Machine API for future updates |
| Machine API | `''` | Create | Yes | - | Annotation is set to `ClusterAPI` and follows above description |
| Machine API | `''` | Create | No | - | Annotation is set to `MachineAPI` and follows above description |
| Machine API | - | Delete | Yes | Yes | Sync controller deletes Cluster API equivalent Machine |
| Machine API | - | Delete | Yes | No | Cluster API equivalent Machine remains in cluster |
| Cluster API | - | Create | N/A | - | Sync controller takes no action. Cluster API Machine acts independently of Machine API |
| Cluster API | `MachineAPI`  | Create | Yes | - | Sync controller syncs Machine API spec to Cluster API Machine |
| Cluster API | `ClusterAPI`  | Create | Yes | - | Sync controller syncs Cluster API spec to Machine API Machine |
| Cluster API | - | Delete | Yes | Yes | Sync controller deletes Machine API equivalent Machine |
| Cluster API | - | Delete | Yes | No | Machine API synchronisation controller will recreate Machine after it is removed[^9] |

[^9]: Any Cluster API Machine that is deleted that does not have an owner, but is reflecting a Machine in Machine API,
is likely to be a control plane Machine without a ControlPlaneMachineSet.
In this case, to prevent accidental deletion, we do not propagate the delete event.
This should not interfere with the semantics of any higher level abstraction or resource such as MachineHealthCheck.

#### Pausing of Cluster API controllers

When a resource is set to be authoritative in Machine API, the Cluster API controllers should be paused.

Cluster API provides either an annotation (`cluster.x-k8s.io/paused`) or field based (`.spec.paused`) approach to pausing controllers.

Controllers in Cluster API respect the pause mechanism to prevent them from taking any action on the resource.
This matches the requirements for the non-authoritative resources described in this document.

The migration controller will leverage the pause mechanism to ensure that the Cluster API controllers do not take any action on the resource when it is non-authoritative.
Admission control will ensure that the value cannot be changed by users when the resource is non-authoritative.

#### Filtering reconciles in Machine API controllers

Each controller will, at the start of the reconcile logic, check for the presence of the `status.authoritativeAPI` and will either skip or continue with the reconciliation based on the presence of this field.

The controller will set the `Paused` condition to `True` when the resource is non-authoritative and will not perform any further reconciliation.

#### The Cluster Autoscaler in mixed MachineSet clusters

The Cluster Autoscaler today looks at Machine API MachineSets and filters these based on a number of annotations to identify node groups that it may scale.
In the future, where we have both Machine API and Cluster API authoritative MachineSets, the Autoscaler must be updated to ensure it scales the authoritative MachineSet.

Since every Machine API MachineSet will be reflected into Cluster API, the autoscaler can be updated to reconcile only the Cluster API MachineSets.
When a scaling decision is made, it must then update the authoritative MachineSet to the correct scale.

To do this, there are two possible approaches to consider:
- Look up the Machine API MachineSet mirror to determine the authoritative API and update accordingly
  - This would be a carry patch to the autoscaler, which, may cause maintenance toil over time
- Allow the Autoscaler to update the Cluster API MachineSet, but allow the sync controller to identify the update has come from the Autoscaler, and mirror it to the authoritative MachineSet as appropriate
  - This could be achieved by observing the managed fields property of the replicas field and determine which actor updated it.
  If the Autoscaler last changed the replica count, it should be mirrored back to the Machine API MachineSet if it is authoritative.
  - This creates an exception to the rules outline above and adds complication to the conversion, but, may be less of a burden than carrying a patch in the autoscaler

Both avenues above will need to be explored further before a decision is made on how the Autoscaler will handle mixed MachineSet clusters.

### Risks and Mitigations

#### Synchronisation logic will need to be solid before it gets into the hand of customers

To ensure that we do not break customer clusters, we need to make sure that our logic that converts the resources between Machine API and Cluster API is thoroughly tested.

To ensure we don’t break customers we will write extensive tests for the conversion, primarily, we expect this to be done via unit testing, however, we will still need to test these conversions in E2E however, since the semantics of a configuration option may differ between the two APIs.

Once this testing has been completed, we will have to run through as many different scenarios and cluster configurations as possible during the QE phase to ensure that the various installation methods for clusters are all covered (for example, cross project networking could be one variant we need to test).

After this, as we will make this available to customers as a preview and not force them to migrate in the first instance, we will have to promote to customers the new features and benefits of the new API and ask that they try it.
The benefit of the design described above is that it caters for allowing customers to try the API with just a single Machine, with an easy way to switch back quickly should things go wrong.

#### A feature implemented in Machine API is not present in Cluster API

When a feature is implemented in Machine API that is not present in Cluster API (we expect this to be a small number if non-zero), in the first instance,
we can fail the synchronisation and add a condition to the Machine API resource to identify that the feature is not yet supported in Cluster API.

When these features are identified, we must prioritise adding the feature in Cluster API as these will become blockers for migration.
Once the feature is implemented in Cluster API, we can unblock the synchronisation for the feature and allow users to migrate.

#### Multiple controllers reconcile the same infrastructure

We aim to prevent multiple controllers (being Machine API and Cluster API) simultaneously reconciling infrastructure as described in the [pausing section][pausing] above.

However, were there to be multiple controllers reconciling the same Machine, there are three cases to consider. Create, Update and Deletion operations.

In a Create operation, if both controllers attempted the create simultaneously, it could lead to an additional host being created and then orphaned within the infrastructure provider.
Once a controller has created the host, it is expected to populate the provider ID and use this to look up the infrastructure in future reconciles.
The synchronisation controller would overwrite the non-authoritative provider ID, effectively orphaning the host.

In an Update operation, since the infrastructure is immutable, the updates are only gathering information.
In this case, the synchronisation controller would overwrite the information, should it differ, in the non-authoritative API.
This could lead to a hot loop/fight between the two controllers.
Rate limiting should be used in the controllers to prevent rapid reconciles in this fashion.

In a Delete operation, both controllers would attempt to delete the same infrastructure.
It is expected that this would not cause issue in most cases as the infrastructure provider should be able to handle multiple requests to delete the same object.
Where Machine lifecycle hooks are in place on Machines, these should be mirrored across both APIs and should prevent either taking action until they are removed.
Machine lifecycle hooks are expected to continue to function as expected.

From this, it is imperative that we ensure Create operations are not reconciled by multiple Machine controllers to prevent the potential of leaking resources.
Testing will be focused on the creation flow.

[pausing]: #pausing-of-cluster-api-controllers

### Drawbacks

#### Mirroring resources may be confusing for end users

We want to provide an easy method for migration for users, however, having multiple representations in OpenShift of the same infrastructure, may be confusing for end users.

They may expect to be able to make changes to either resource and have them reflected.
For that to be safe we would have to make sure all writes to objects are synchronous to both objects.
This is technically very challenging and has been dismissed as an alternative.

Documentation will be provided to explain to users the expected behaviours and the expectations to allow them to migrate their infrastructure across.

## Design Details

### Open Questions

N/A

## Test Plan

### Conversion of provider specs

The conversion of provider specs from Machine API to Cluster API should be able to be statically tested. We will build out extensive conversion testing via unit tests to ensure that, with a desired input, the correct conversion happens for the provider specs and the associated resources.

### Authoritative API reconciliation

Machine API and Cluster API controllers already use envtest as a way to provide integration testing with a "real" API server.
We can extend the tests here to account for the cases where the `authoritativeAPI` feature is in use.

By extending the integration tests, we can sculpt the desired inputs for mirror resources existing, not existing, and authoritative in both API versions,
and then determine whether the reconcile continued or exited based on these conditions.

### Full migration E2E

The behaviours outlined [above][summary-of-rules] will be tested in a new E2E test suite specifically targeted at the conversion logic.

In addition to these tests, the following tests will provide a general coverage of the conversion logic and behaviours defined:
- Scale up and down of a mirrored MachineSet
  - Copy an existing MachineSet in Machine API
  - Observe that the new MachineSet is mirrored in Cluster API
  - Switch the authoritative API to Cluster API
  - Scale up the MachineSet in Cluster API
  - Wait for Machine and mirror Machine to be created
  - Wait for Node to join cluster
  - Scale down MachineSet in Cluster API
  - Observe scale down successfully
  - Remove new MachineSet
- Migration of ControlPlaneMachineSet to Cluster API
  - Observe existing cluster ControlPlaneMachineSet
  - Switch ControlPlaneMachineSet authoritative API to Cluster API
  - Create a new InfrastructureTemplate with a larger instance size
  - Update Cluster API ControlPlaneMachineSet to point to new InfrastructureTemplate
  - Observe rolling update for ControlPlaneMachineSet
- MachineHealthCheck in mixed clusters
  - Ensure a mixed cluster is created with authoritative MachineSets in both APIs
  - Trigger a MachineHealthCheck reconciliation using a Machine API authoritative MachineHealthCheck
  - Ensure that Machines are deleted correctly
  - Switch authority of MachineHealthCheck to Cluster API
  - Trigger a second MachineHealthCheck reconciliation using the now Cluster API authoritative MachineHealthCheck
  - Ensure that Machines are deleted correctly

[summary-of-rules]: #summary-of-the-rules-outlined-above

## Graduation Criteria

The project will initially be introduced under a feature gate `MachineAPIMigration`.
The new sync controller will be deployed on all `TechPreviewNoUpgrade`/`CustomNoUpgrade` clusters,
but will check for the presence of the above feature gate before operating.

### Dev Preview -> Tech Preview

Initially the operator will be introduced behind a feature gate and only accessible with a `CustomNoUpgrade` feature set.
Before promotion to the `TechPreviewNoUpgrade` feature set, we will ensure:
- The operator has reached a minimal level of functionality to be end to end testable
- Manual testing allows at least one platform with basic configuration to be converted between Cluster API and Machine API
- Tests have been run to ensure the operator does not break the existing payload jobs
- The operator can report its status via conditions on the Machine API cluster operator

### Tech Preview -> GA

Once the operator has been released under the `TechPreviewNoUpgrade` feature set, we will continue to enhance the operator
before marking it as GA:
- Full E2E testing of the various behaviours described regarding Machine synchronisation
- E2E testing of MachineSet and ControlPlaneMachineSet conversion
- E2E testing of MachineHealthCheck workflows in mixed instance management clusters
- At least one platform has sufficient conversion logic to be useful and allow users on the platform to start migration

Note, we will not require 100% feature parity for promotion to GA.
Some features supported by Machine API may be supported in conversion only in subsequent releases.

### Removing a deprecated feature

No features will be deprecated as a part of this enhancement.

## Upgrade / Downgrade Strategy

### On Upgrade

When upgrading into a release with the new sync controller,
new Cluster API resources will be created and existing Machine API resources will have the new API fields added.

There should be no effect on the cluster as each of these operations should be a no-op.

### On Downgrade

On downgrade, the synchronisation of resources will stop.
To prevent both Machine API and Cluster API resources being reconciled,
we will make sure that Cluster API controllers in the 4.N-1 release do not reconcile when a mirror resource exists with the same name.
Independent of the authoritative API.

This will mean that on downgrade all Machine management reverts to Machine API controllers.

## Version Skew Strategy

In 4.N-1, all resources that are mirrored into Cluster API will be managed by the Machine API controllers.
The mirroring relationship will be based on the names of the resources to create a 1:1 mapping between Machines and MachineSets/ControlPlaneMachineSets in Machine API and Cluster API.
Cluster API resources will be marked as paused when not authoritative. Since this mechanism exists today, the older controllers should handle the version skew provided the pausing is accurate.

Since the authority of the resource is based on an API field, which is not present in prior releases, there should be no issue during upgrades due to version skew or incompatibility.

## Operational Aspects of API Extensions

The new sync and migration controllers will operate in the `openshift-cluster-api` namespace alongside existing Cluster API operator and controllers.
It forms the bases of the conversion between the two API groups, but as a controller, rather than a conversion webhook.
Its SLIs and impact on the cluster are therefore different to a traditional conversion webhook.

We expect, when the sync controller is failing, that it will report individual errors as conditions on the Machine API resources.
For wider failures, it will report a condition on the `machine-api` ClusterOperator, which will be aggregated with other conditions to determine the health of the Machine API overall.

There should be no impact on the operation of existing systems by the new synchronisation logic; it does not affect the functionality, rather it mirrors the resource for informational purposes primarily.

### Failure Modes

When non-functional, the sync controller will not update the non-authoritative resources in the cluster.
This could lead to stale information being present in the non-authoritative API.

This could lead to a risk that the authoritative API is switched while the data is stale.

Since the sync controller is required to update the `status.authoritativeAPI` to enact the switch, the desired switch will not happen until the sync controller is restored.

## Support Procedures

During failures, we expect the Machine API ClusterOperator to report status of the sync controller.
If the detail is not sufficient to understand the failure, logging will provide additional detail.

Expected symptoms of failure include:
- An inability to switch a resource authoritative API
- Stale data in the non-authoritative API resources

None of the symptoms above impact the day to day operation of a cluster.
They only impact administrator actions to migrate between the Machine API and Cluster API.

Importantly, Machine scaling should be unaffected by the failure of the synchronisation controller.

Given this, the severity of these failures is low.

Stale data may be observed by fetching the non-authoritative API resource and comparing the value of the `spec.authoritativeAPI` and `status.authoritativeAPI`.
Errors in conversion will be reported on the Machine API resource in the `status.conditions` under the `Synchronized` condition.

When the controller is restored, it should, unless there are persistent errors in need of human intervention, continue reconciling and restore the desired state.

## Implementation History

- [ ] Enhancement merged (YYYY-MM-DD)

## Alternatives

### Conversion Alternatives

#### Use an aggregated API server to handle Machine API and Cluster API

As an alterative to synchronising resources post update, we could use an aggregated API server to implement both Machine API and Cluster API endpoints going forward.

This aggregated API server would be responsible for handling all API calls for both API groups and would interact directly with etcd for storage of the resources.

In this case, it could ensure conversion is synchronised and written to etcd atomically, in a single call.
This would prevent any drift between resources and mean a number of the rules above would no longer be required.

This alternative has been dismissed for a number of reasons:
- Aggregated API servers are generally complex and introduce new failure modes for the entire cluster
- We are not certain that it is possible to take over the API endpoints of an existing installed CRD
- Interacting directly at the storage layer is complex and adds other risks that we do not want to accept
- Handling errors writing multiple objects to etcd for a single API call becomes complex
- We still need a way to decide which operator is authoritative

#### Machine API controllers write out Cluster API resources

In this alternative, rather than converting between resources in a separate controller, the implementation of each Machine controller for Machine API would be rewritten.
Instead of interacting directly with the cloud provider, the rewrite would convert the resources from Machine API to Cluster API and then write these to the API server.

The controllers would become shims, leveraging Cluster API as a backend to implement their existing functionality.

This alternative has been dismissed based on the following reasons:
- This doesn't help our maintenance burden, we still have Machine API controllers to manage
- It is unclear how to handle a lack of feature parity between the Machine API and Cluster API resources for this design
- We prefer to keep the conversion logic in a centralised place so that we can run 1 controller in the future, instead of many

#### Machine API controllers convert to Cluster API internally

In this solution, the Machine API controllers would be rewritten to convert Machine API resources internally to Cluster API resources.
Once the conversion has been completed, the controllers would feed the converted Cluster API resources into the Cluster API methods, using them as a library.

This alternative has been dismissed based on:
- No migration from Machine API to Cluster API
- No reduction in maintenance burden
- No longer term plan to remove the Machine API controllers
- It is unclear how to handle conversion when there is a feature gap in Cluster API

#### Convert resources using a webhook

Rather than using a controller to convert the resources after they have been accepted by the API server, we could use a ValidatingAdmissionWebhook to convert resources as updates are applied.
This has the benefit of applying updates synchronously to both API groups.

However, it also adds additional complexity
- If the conversion webhook accepts the change and writes out the Cluster API resources
  - Does this then call the converse webhook and start to write out the Machine API resources, creating a loop?
  - What happens if a later admission webhook rejects the update?
  - What end user webhooks are built on top of our extensions, may this interfere with them?

#### Offline conversion

Rather than converting resources within the cluster, we could provide users with a tool to offline convert their Machine API resources to Cluster API.
This is possible even with the current proposal as an additional feature which may be desirable for some users to create additional MachineSets, based on existing MachineSets.

However, this doesn't help with the goal of removing Machine API controllers in the longer term, so is not sufficient to meet our goals on its own.

### Filtering events in Cluster API

#### Filtering reconciles in Cluster API controllers

To enable the Cluster API controllers to observe the authoritative API, we would need to implement a check at the beginning of the reconcile logic of each controller.

This check would look up the current state of the Machine API equivalent resource and determine whether the controller should be reconciling that specific resource or not.

To ensure we always have the latest version, caches should not be used when performing this check (Cluster API is based on controller-runtime which by default uses a caching client).

Each of the infrastructure Machine controllers will not have an equivalent resource to look up, and therefore must first identify the Machine that they are associated with and look up the equivalent resource for this Machine.

This would require carrying a patch in the Cluster API controllers to implement this logic and would create toil for maintenance.

##### Filtering reconciles via a webhook

An alternative to the above is to have each Cluster API controller perform an equivalent of a SubjectAccessReview.
A request to a webhook with details of the resource they have been requested to reconcile, where the webhook can then check if the resource should be reconciled and return a response.
When the response is denied, the reconcile should be aborted.

If the webhook is unavailable, the mechanism should fail closed and the reconcile should be retried at a later time.

### Deletion events are always synchronised 

In the scenario described above, users may delete Machine API resources without impacting the Cluster API resource providing the Cluster API resource is authoritative.
This means that the user may clean up old Machine API resources without any special consideration.

This eventually will allow the Machine API resources to be deleted and have no effect on the running system.

Instead, we could always synchronise deletion events.
This would mean that any delete of a resource is always mirrored no matter the circumstances.

This would mean that, to allow users to clean up Machine API resources, we would need to introduce some “escape hatch”,
likely an annotation, that a user could set on the resource to allow it to be deleted without also deleting the Cluster API equivalent.

This could lead to users believing that, because the Cluster API resource is authoritative, deleting the Machine API resource is acceptable.
Instead, it would result in deleting a Machine which they had not intended to do.
This could be catastrophic to clusters if users accidentally delete a control plane Machine,
therefore we need to make this as simple as possible, by having the single authoritative switch indicate the behaviour of deletion,
there is less room for error on the customers part.

### Use webhooks to synchronise annotations

Instead of having each controller look up the annotation on the Machine API resource,
we could use webhooks on both Machine API and Cluster API resources to ensure that only one of the resources is authoritative at any time.

With this approach, the authoritative annotation existing on a resource would indicate that it is the authoritative version, and therefore should be reconciled. 
Webhooks would be in place to prevent the annotation from being added to both Machine API and Cluster API mirrors at the same time.

To move between the two versions, the user would first have to remove the annotation from one (in which case no reconciliation happens anywhere), and then add it to the other to make it authoritative.

This approach may seem to be less complex, but has the potential to cause issues if, for some reason, the webhooks do not work correctly (eg a change is made while the webhook is being restarted).

To avoid this potential, our approach suggests to have the authoritative annotation exist in only one location, on the Machine API resource.

### Cluster wide migration of resources

In this alternative implementation, the switch to allow migration between Cluster API and Machine API would be at the cluster level, rather than at the individual resource level.
This alternative prevents users from testing migrations on individual resources and also prevents users from leveraging new Cluster API features until all of their Machine API machines are migratable.
Since we understand that some feature parity may not be present in the initial releases of this feature, we do not want to block users from using the new features, while waiting for the entire cluster to be ready to migrate.

## Infrastructure Needed

A new repository `github.com/openshift/machine-api-synchronisation-operator` will be needed to complete this project.
