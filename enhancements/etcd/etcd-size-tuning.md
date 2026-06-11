---
title: etcd-size-tuning
authors:
  - "@atiratree"
  - "@bhperry"
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
last-updated: 2026-06-11
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
* Ensure that the backend limit increase doesn't exceed the available free disk space on the nodes.
* The backend limit can be decreased up to the current DB size (plus a buffer).

## Proposal

Allow the customer/admin access to set the backend quota directly through a human readable value via the etcd.operator.openshift.io/v1 field.
They will be allowed to set any integer gibibyte value between the current limit and the maximum supported limit.

Increasing this value will not have etcd immediately reserve this space on disk, it just allows etcd to grow up to that value if etcd grows under the usage of the cluster.

The default value or unset value would be the current, hardcoded backend quota: 8GiB. This would allow for upgrades where this configuration value wouldn't necessarily be set.

Initial upper limit of 32GiB was identified by the [Perf-Scale work](https://issues.redhat.com/browse/PERFSCALE-2881) as a maximally stable value: higher values than 34GiB would introduce increasing instability.

[Etcd Memory Usage Testing and Requirements](#etcd-memory-usage-testing-and-requirements) revealed that even 16GiB upper limit results
in a substantial increase of node memory, suggesting a change in the recommended size of node memory.

The quota has an effect on things like defragmentation, snapshot restore, etc.
A downside is that with the profiles, discussed in the Alternatives section, there are a discrete, small, number of permutations to test, giving more confidence of the exact effects a given profile has on performance/memory requirements.
This is a minor consideration as setting the quota to an arbitrarily higher value does not mean that etcd will take up that much space; it will depend on the workload.

### Workflow Description

**cluster administrator** is a human user responsible deploying and maintaining the cluster.

1. The cluster administrator decides to change the etcd backend quota from the default (8GiB) to a larger value; e.g. 10GiB.
2. They set the new value via the `etcd.operator.openshift.io/v1` API.
3. An etcd redeployment will be automatically run which restarts the etcd pods which consume the new value.

#### Invalid value

1. The cluster administrator wants to set the quota from a larger value (e.g. 16GiB) to a smaller one (8GiB).
2. They attempt to set the new value via the `etcd.operator.openshift.io/v1` API.
3. The API server should return an error notifying them that the value is invalid - not allowed to shrink the quota.

### API Extensions

In operator/v1/types_etcd.go:
- Add BackendQuotaGiB of type int32
  - With default '8GiB'.
  - With validation disallow a decrease in value.
  - With an upper limit of '16GiB'.

```go
type EtcdSpec struct {
	// backendQuotaGiB sets the etcd backend storage size limit in gibibytes.
	// The value should be an integer not less than 8 and not more than 16.
	// When not specified, the default value is 8.
	// +kubebuilder:default:=8
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=16
	// +kubebuilder:validation:XValidation:rule="self>=oldSelf",message="etcd backendQuotaGiB may not be decreased"
	// +openshift:enable:FeatureGate=EtcdBackendQuota
	// +default=8
	// +optional
  BackendQuotaGiB int32 `json:"backendQuotaGiB"`
}
```

### Etcd Memory Usage Testing and Requirements

As part of testing the increase of the etcd database size limit from 8GB to 16GB, memory usage on control plane nodes was observed under various conditions.
This section captures findings on the relationship between etcd database size and control plane node memory consumption, and provides guidance for clusters
that may approach the new 16GB limit.

Metrics and graphs for the test runs can be found [here](https://drive.google.com/drive/u/0/folders/15HHGTnicj7g6yGFuyW_mdh-87-R9CHrx).

#### Observations

Control plane node memory consumption grows much faster than the size of the etcd database itself. Across multiple test runs with different types of cluster
workloads, the ratio of control plane node memory usage to etcd database size ranged from approximately **4x to 10x**, depending on the types and quantities
of objects stored in etcd.

With the database near the 16GB limit, individual control plane nodes were observed using **60-70GB** of memory at the low end and **130-170GB** at the high
end, depending on the composition of objects in the cluster.

Memory consumption is largely driven by the kube-apiserver due to its cache-based architecture. At a minimum it keeps a full replica of the etcd database in
memory, but with controller watch streams there can be multiple revisions of each object stored in the apiserver. Frequently watched kinds such as Secrets
tend to drive higher memory usage, while a CRD that isn't watched by any control plane components uses much less memory.

Prometheus is another component that can rapidly increase memory usage on large clusters, particularly when there are many pods running in the cluster since
they generate a large quantity of time series per object. Prometheus memory was observed at 60GB on an 8GB etcd DB running primarily container workloads, and
increasing linearly with database size. Default resource requests and limits are too small for clusters of this size, so be sure to
[configure prometheus resources](https://docs.redhat.com/en/documentation/monitoring_stack_for_red_hat_openshift/4.21/html-single/configuring_core_platform_monitoring/index#managing-cpu-and-memory-resources-for-monitoring-components_configuring-performance-and-scalability)
appropriately for your needs.

#### Sizing Guidance for 16GB Database Limit

Given the observed memory multipliers, clusters that are expected to approach the 16GB etcd database limit should ensure control plane nodes are provisioned
with sufficient memory overhead:

| etcd DB Size | Low Memory Ratio (~4x) | High Memory Ratio (up to 10x) |
|--------------|------------------------|-------------------------------|
| 8 GB         | ~32 GB                 | up to 80 GB                   |
| 12 GB        | ~48 GB                 | up to 120 GB                  |
| 16 GB        | ~64 GB                 | up to 160 GB                  |

The type of resources stored in etcd has a significant impact on which end of this range a cluster will land. Clusters with large numbers of secrets or pods
should plan for the higher end of these estimates.

Control plane nodes should be sized with enough headroom to cover resource usage spikes without risking OOM kills. The existing
[control plane node sizing guidelines](https://docs.redhat.com/en/documentation/openshift_container_platform/4.21/html/scalability_and_performance/recommended-performance-and-scalability-practices-2#master-node-sizing_recommended-control-plane-practices)
recommend keeping overall resource usage to at most 60% of available capacity to handle spikes from node failures and upgrades. This headroom is especially
important for clusters operating near the 16GB database limit, where memory demands are substantial.

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
* Currently, etcd can not detect whether its backend quota configuration value could lead to a database that's larger than the available storage.
  * We could potentially add a prometheus alert to fire when the etcd node is running out of disk space.
  * We could potentially add some validation to the etcd server to check the quota configuration value against the space available and error if too small.
    * This enhancement would need to take into consideration multiple instances sharing a disk to reduce the likelihood of each individual instance thinking it has enough space, but not realizing the other instances are also vying for that space.
  Please also see [Checking the Upper Bound](#checking-the-upper-bound) for additional details.
* Etcd puts out a warning if the backend quota is configured to be larger than 8GiB and the documentation calls out that 8GiB is the suggested maximum for normal clusters.
  * We've chosen to allow values higher than this soft-limit as the customers/clusters asking for this feature are larger than would be considered a "normal" cluster.
  * [The Perf-Scale team has tested values](https://issues.redhat.com/browse/PERFSCALE-2881) up to 40GiB and found that instability was introduced around 34GiB which influenced the recommended maximum: 32GiB. This maximum was further decreased to 16GiB as bigger values substantially increase the memory requirements.
* When etcd quota hits the limit, it is not possible to do any writes, which means updating `.spec.backendQuotaGiB` will not be possible. Users should remove unnecessary objects (e.g. events) from the cluster to allow the quota increase. This should be documented.
* The DB size has a big impact on the memory usage. Without the ability to decrease the quota, it is difficult to limit the memory usage.
  This can result in further cluster performance degradation. It is also difficult to downsize master nodes to reduce costs.
* How long does it take to sync a snapshot of a given size to another peer; for reboot/recovery?
  The recovery time and transfer speed scale linearly with the size of the snapshot. On an AWS m5.16xlarge instance, this corresponds approximately
  to 1m15s for 8GiB DB size and 2m15s for 16GiB size. This poses a risk for nodes with hardware specifications below the recommended size, or for
  clusters with congested or slow networks.

### Drawbacks

* There will be a required etcd rollout when changing values, this happens automatically by the Cluster Etcd Operator to re-render the etcd podspec with the new backend quota value.

## Open Questions [optional]

For each of these, we would need to compare against the current default of 8GiB. Increasing the quota does not necessarily increase the following, but at the extreme case (e.g. increase to 16GiB and using nearly that much) we expect the following to perform differently.

* How long does it take to compact & defrag a database of a given size?
  * Defragmentation will pause writes to etcd, reads from etcd will be unaffected.

## Test Plan

- Set the .spec.backendQuotaGiB to the maximum value (16GiB) and check validation: [TestEtcdDBScaling](https://github.com/openshift/cluster-etcd-operator/blob/1fe5873b112029a943e113ba377413b32ae13443/test/e2e/scaling_dbsize.go)

We will need to test a cluster with a larger quota to ensure everything works as expected.
We should also load the database with a larger quota and make note of the memory requirements of a defragmentation.
  - We should do this with some values we could assume a customer would use; 16GiB, 32GiB, 100GiB (in the extreme).
We need to test that it is not possible to shrink the quota once it has be set to a larger value.

## Graduation Criteria

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
  - Description of the feature and expectations of memory usages for some larger values.
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
- If the user attempts to set the quota to a lower value than the current DB size (e.g. 21GiB -> 16GiB), the API will not accept the value and return an error.
- If the value is accepted, an etcd rollout will automatically occur and re-render the etcd podspec with the new quota - this will not cause the etcd instances to immediately reserve disk space, it will only allow them to grow up to the new value.

## Alternatives (Not Implemented)

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

This alternative was rejected because the discrete values don't provide a benefit to performance or safety of the etcd cluster: it just allows the etcd database to be larger if it needs to be. Also, the current enhancement proposal allows for users to more closely configure their etcd cluster to the resources available to them; with discrete values, a user could have to choose between a value that's too small or too large for their purposes.


### Decreasing the Quota

In general increasing the quota is done in response to increased demand on the cluster.
Decreasing or reverting the quota is unlikely to happen often, but it has the following benefit:
- The DB size has a big impact on the memory usage. Decreasing the quota can prevent further cluster
  performance degradation when used correctly together with monitoring and etcd alerts. These alerts
  are proportionate to the quota size and can be used as a signal to observe and limit memory usage.
  Decreasing the quota is also important when downsizing master nodes and reducing cost.

The `.spec.backendQuotaGiB` validation could be relaxed to allow the quota to be decreased when
appropriate. To implement the new validation, we would also report the current DB size in
`.status.BackendSizeKiB`.

```go

// +kubebuilder:validation:XValidation:rule="self.spec.backendQuotaGiB >= oldSelf.spec.backendQuotaGiB || (self.spec.backendQuotaGiB * 1024 * 1024) >= (self.status.BackendSizeKiB + 100 * 1024)",message="etcd .spec.backendQuotaGiB must be greater than .status.backendSizeKiB plus a 100 MiB buffer"
type Etcd struct {
  Spec EtcdSpec `json:"spec"`
  Status EtcdStatus `json:"status"`
}

type EtcdSpec struct {
	// backendQuotaGiB sets the etcd backend storage size limit in gibibytes.
	// The value should be an integer not less than 8 and not more than 16.
	// When not specified, the default value is 8.
	// +kubebuilder:default:=8
	// +kubebuilder:validation:Minimum=8
	// +kubebuilder:validation:Maximum=16
	// +openshift:enable:FeatureGate=EtcdBackendQuota
	// +default=8
	// +optional
  BackendQuotaGiB int32 `json:"backendQuotaGiB"`
}

type EtcdStatus struct {
  // BackendSizeKiB represents the current etcd backend storage database size in kibibytes.
  // +kubebuilder:validation:Minimum=0
  // +openshift:enable:FeatureGate=EtcdBackendQuota
  // +optional
  BackendSizeKiB *int64 `json:"backendSizeMiB"`
}
```

### Checking the Upper Bound

We could also introduce a validation check to determine the maximum size of the database that can
fit on all the master nodes.

Unfortunately, this would bring in additional challenges.

While we can collect the current DB size with `etcd_mvcc_db_total_size_in_bytes` metric, it does not
report the final disk space used. The real space used by etcd in total also includes .snap
(snapshots) and .wal (write ahead log) files.

Further information about the etcd footprint can be found here: https://docs.google.com/document/d/1O2o1IApHWmSioXG3fez4eVlUHOrXICYGNVIzaqNS0IQ

We also need a reliable mechanism to find the currently available free space on the node. The etcd
data is stored on the node hostPath in /var/lib/etcd. This could be achieved using a
`node_filesystem_avail_bytes{mountpoint="/var"}` metric, for example. Although,
[Moving etcd to a different disk](https://docs.redhat.com/en/documentation/openshift_container_platform/4.21/html/etcd/ensuring-reliable-etcd-performance-and-scalability#move-etcd-different-disk_etcd-performance)
should be considered as well when implementing the check.

This feature can be implemented, but better reporting of space usage would probably be needed. The
validation would probably have to be performed late (no CEL), because for example, the available
space is not known during cluster install, when the Etcd object has not yet been created and
reconciled. 

Ultimately, it would be better to implement a standalone feature that solves this issue, for
example, by providing additional metrics and alerts. This would provide cluster admins with
real-time information about the upcoming space exhaustion, and not just when the
`.spec.BackendQuotaGiB` value changes. This would also benefit all clusters (default 8 GiB size)
even those where the EtcdBackendQuota feature is disabled/unavailable.


## Infrastructure Needed [optional]

None
