---
title: etcd-size-tuning
authors:
  - "@dusk125"
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - "@hasbro17, etcd team lead"
  - "@tjungblu, etcd team"
  - "@Elbehery, etcd team"
  - "@williamcaban, Openshift product manager"
  - "@soltysh, Openshift staff engineer"
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - "@hasbro17, etcd team lead"
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - None
creation-date: 2024-01-24
last-updated: 2024-03-12
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/ETCD-514
---

# etcd Size Tuning

## Summary

This enhancement would replace the hardcoded value for the etcd backend database quota with a human readable, configurable value; 8GiB. This value does not immediately reserve disk space for etcd; it is an upper limit to the size of the etcd database on disk.
This enhancement only covers the mvp for a tech preview release of this new feature; a future enhancement will be necessary.

## Motivation

Customers have asked for the ability to increase the quota for the backend database to faciliate larger clusters.
In general, CI/CD and pipeline clusters, the number of namespaces and CRDs (and the size of the CRDs) have been increasing which drastically increase the amount of data stored in etcd.
We want to be able to allow for this while minimizing the risk of them setting values that cause issues.

### User Stories

* As an administrator, I want to change the etcd backend size to decrease the risk of quota reached errors in very large clusters.
* As an Openshift support, I want to walk a customer through changing the etcd size in a minimal number of steps.
* As an etcd engineer, I want to easily add and test new sizes for performance impacts on both large and small clusters.

### Goals

* Allow configuration of the backend limit via human readable units: 16GiB.
* Add an API to allow admins to change the value.
* The backend limit can only be increased and not decreased.

### Non-Goals

* Handle consuming value changes without an etcd rollout.

## Proposal

Allow the customer/admin access to set the backend quota directly through a human readable value via the etcd.operator.openshift.io/v1 techpreview field.
They will be allowed to set any integer gibibyte value between the current limit and the maximum supported limit.

Increasing this value will not have etcd immediately reserve this space on disk, it just allows etcd to grow up to that value if etcd grows under the usage of the cluster.

The default value or unset value would be the current, hardcoded backend quota: 8GiB. This would allow for upgrades where this configuration value wouldn't nesscessarily be set.
An initial upper limit of 32GiB will be enforced due to the [Perf-Scale work](https://issues.redhat.com/browse/PERFSCALE-2881) that proved 32GiB was a maximally stable value: higher values than 34GiB would introduce increasing instability.

We will need to work with the perf team to understand the memory requirements of largers quotas, and the effect of the quota on things like defragmentation, snapshot restore, etc.
A downside is that with the profiles, discussed in the Alternatives section, there are a discrete, small, number of permutations to test, giving more confidence of the exact effects a given profile has on performance/memory requirements.
This is a minor consideration as setting the quota to a arbitrarily higher value does not mean that etcd will take up that much space; it will depend on the workload.

### Workflow Description

**cluster administrator** is a human user responsible deploying and maintaining the cluster.

1. The cluster administrator decides to change the etcd backend quota from the default (8GiB) to a larger value; e.g. 20GiB.
2. They set the new value via the `etcd.operator.openshift.io/v1` API.
3. An etcd redeployment will be automatically run which restarts the etcd pods which consume the new value.

#### Invalid value

1. The cluster administrator wants to set the quota from a larger value (e.g. 32GiB) to a smaller one (8GiB).
2. They attempt to set the new value via the `etcd.operator.openshift.io/v1` API.
3. The API server should return an error notifying them that the value is invalid - not allowed to shrink the quota.

### API Extensions

In operator/v1/0000_12_etcd-operator_01_config-TechPreviewNoUpgrade.crd.yaml:
- Add BackendQuotaGiB of type int32
  - With default '8GiB'.
  - With validation disallow a decrease in value.
  - With an upper limit of '32GiB'.

```go
type EtcdSpec struct {
	// backendQuotaGiB sets the etcd backend storage size limit in gibibytes.
	// The value should be an integer not less than 8 and not more than 32.
	// When not specified, the default value is 8.
	// +kubebuilder:default:=8
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=32
	// +kubebuilder:validation:XValidation:rule="self>=oldSelf",message="etcd backendQuotaGiB may not be decreased"
	// +openshift:enable:FeatureGate=EtcdBackendQuota
	// +default=8
	// +optional
  BackendQuotaGiB int32 `json:"backendQuotaGiB"`
}
```

### Topology Considerations

#### Hypershift / Hosted Control Planes

Since Hypershift does not deploy the cluster etcd operator it can not make use of this feature automatically; it would need to handle detecting the change in the api and rolling out the etcd containers with the new quota. [It configures the quota itself.](https://github.com/openshift/hypershift/blob/main/control-plane-operator/controllers/hostedcontrolplane/etcd/reconcile.go#L408)

#### Standalone Clusters

This change should operate as expected on standalone clusters.

#### Single-node Deployments or MicroShift

This feature should not change the resource consumption of a single-node OpenShift deployment. If an admin decides to increase the quota, the same caveats, from normal OpenShift, about disk space will apply - etcd will not immediately reserve the quota-size, but could grow up to the quota and exhaust disk space in the process.

This enhancement will not directly impact MicroShift since the API will be consumed by the Cluster Etcd Operator, only.
[Also, changing this value is already supported in the MicroShift configuration](https://github.com/openshift/microshift/blob/main/pkg/config/etcd.go#L18).

### Implementation Details/Notes/Constraints

* Add a configuration option using resource.Quantity to allow for human readable values; i.e. 16GiB.
* Remove the parameters from the podspec rendering and replace with the value.

### Risks and Mitigations

* We will need to test larger sizes to ensure that everything works as expected.
* For a larger database, defragmentation of etcd will take longer which will increase the amount of time that etcd is unavaiable for writes.
  Etcd will still be available for reads during defragmentation, so there should be little impact on the apiserver's availability.
* Downgrades may be impacted if:
  1. The downgrade is to a version without this feature.
  2. The quota has been set higher than the default.
  3. The etcd backends are using more space than the default.
  A mitigation would be to keep the quota at the higher value (with no way of decreasing it), so that the cluster will continue to function and no data will be lost.
* Currently, etcd can not detect whether its backend quota configuration value could lead to a database that's larger than the available storage.
  * We could potentially add a prometheus alert to fire when the etcd node is running out of disk space.
  * We could potentially add some validation to the etcd server to check the quota configuration value against the space available and error if too small.
    * This enhancement would need to take into consideration multiple instances sharing a disk to reduce the likelihood of each individual instance thinking it has enough space, but not realizing the other instances are also vying for that space.
* Etcd puts out a warning if the backend quota is configured to be larger than 8GiB and the documentation calls out that 8GiB is the suggested maximum for normal clusters.
  * We've chosen to allow values higher than this soft-limit as the customers/clusters asking for this feature are larger than would be considered a "normal" cluster.
  * [The Perf-Scale team has tested values](https://issues.redhat.com/browse/PERFSCALE-2881) up to 40GiB and found that instability was introduced around 34GiB which influenced the recommended maximum: 32GiB.

### Drawbacks

* There will be a required etcd rollout when changing values, this happens automatically by the Cluster Etcd Operator to re-render the etcd podspec with the new backend quota value.

## Open Questions [optional]

For each of these, we would need to compare against the current default of 8GiB. Increasing the quota does not necessarily increase the following, but at the extreme case (e.g. increase to 16GiB and using nearly that much) we expect the following to perform differently.
* How long does it take to sync a snapshot of a given size to another peer; for reboot/recovery?
* How long does it take to compact & defrag a database of a given size?
  * Defragmentation will pause writes to etcd, reads from etcd will be unaffected.

## Test Plan

**Note:** *Section not required until targeted at a release.*

We will need to test a cluster with a larger quota to ensure everything works as expected.
We should also load the database with a larger quota and make note of the memory requirements of a defragmentation.
  - We should do this with some values we could assume a customer would use; 16GiB, 32GiB, 100GiB (in the extreme).
We need to test that it is not possible to shrink the quota once it has be set to a larger value.

## Graduation Criteria

**Note:** *Section not required until targeted at a release.*

None

### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

- Known issue in API Server
  - When the API server restarts, it's currently possible for all watchers to be reset at the same time and request a large amount of data from etcd, leading to a crash-loop in the API server.
  - [https://github.com/kubernetes/enhancements/issues/3157](API Streaming)
  - [https://github.com/kubernetes/enhancements/issues/4222](CBOR Serialization)
- Etcd Team to investigate whether a larger etcd database can cause crash-loops in Etcd (via out-of-memory for example) on restarts (similar to the above API server issue).
  - If it is possible to get into a crash-loop because of the database size, the Etcd team will document how to recover the Etcd cluster from this state.
- Customer feedback
- API extension
- Value change with minimal disruption
- Performance testing for some larger-than-default values
  - Need to find reasonable and safe upper bound for maximum allowed value.
- End user documentation
  - Description of the feature and expectations of memory usages for some largers values.
  - Steps to change the value.

### Removing a deprecated feature

N/A

## Upgrade / Downgrade Strategy

N/A

## Version Skew Strategy

N/A

## Operational Aspects of API Extensions

N/A

## Support Procedures

- If the user attempts to set the quota to an invalid value (not in the form of a resource quantity (e.g. 8GiB, etc)), the API will not accept the value and return an error.
- If the user attempts to set the quota to a lower value than the current (e.g. 21GiB -> 16GiB), the API will not accept the value and return an error.
- If the value is accepted, an etcd rollout will automatically occur and re-render the etcd podspec with the new quota - this will not cause the etcd instances to immediately reserve disk space, it will only allow them to grow up to the new value.

## Alternatives

### Discrete Profile values

The profiles are a layer of abstraction that allow a customer to tweak etcd to run more reliably on their system, while not being so open as to allow them to easily harm their cluster by (knowingly or not) setting bad values for them.
The default profile ("" or unset) will allow for upgrades to this feature as it tells the system to use the current default value (8GiB): this is the current behavior.
The values for the two proposed profiles for the Tech Preview of this feature are the current default value and double this value.
Changing to a "larger" profile will incur higher memory usage (especially during defragmentation), but that is likely an acceptable trade for cluster stability.

In this iteration, for the Tech Preview, we will make it clear that changing the profile will require an etcd redeployment.
In the future enhancement, we can discuss a more seamless transition between profiles.
We will not allow the user to set arbitrary values for the parameters, they must conform to the profiles values (by way of the profile).
We will not allow the user to decrease the quota; move from Larger to Default.

The active profile will be set via the API Server, then an etcd rollout will be triggered automatically by the Cluster Etcd Operator env var controller to consume the new profile.

The entry for the profile will be added to the operator/v1 etcd operator config crd in the API server, named QuotaBackendSize, alongside other etcd configuration options.

The profiles that will be added are:
* Default (""):
  - ETCD_BACKEND_QUOTA: 8GiB
* Larger:
  - ETCD_BACKEND_QUOTA: 16GiB

This alternative was rejected because the discrete values don't provide a benefit to performance or safety of the etcd cluster: it just allows the etcd database to be larger if it needs to be. Also, the current enhancment proposal allows for users to more closely configure their etcd cluster to the resources available to them; with discrete values, a user could have to choose between a value that's too small or too large for their purposes.

## Infrastructure Needed [optional]

None
