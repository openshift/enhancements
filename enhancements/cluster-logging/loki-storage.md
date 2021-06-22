---
title: Cluster Logging Loki as an alternative Log Store
authors:
  - "@periklis"
reviewers:
  - "@ewolinetz"
  - "@blockloop"
  - "@lukas-vlcek"
  - "@sichvoge"
approvers:
  - "@ewolinetz"
  - "@alanconway"
  - "@jcantril"
creation-date: 2021-03-29
last-updated: 2021-05-10
status: implemented
see-also: []
replaces: []
superseded-by: []
---

# Cluster Logging: Loki as an alternative Log Store

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The purpose of this is to provide an alternative low-retention log store based on [Loki](https://github.com/grafana/loki) for the cluster logging stack. This aims to provide a cloud-native first solution specialized in storing logs and in turn lessen the operational cost running cluster logging.

Currently our main log store, i.e. Elasticsearch, is a more general-purpose ingestion, indexing and querying cluster-based solution for text search. It is a powerful solution with fine-grained control on how to scale log storage, indexing and querying.

In general it serves its purpose quite well but it is a constant source of labor regarding operations. Its powerfulness and complexity requires a constant toll on the OpenShift cluster administrator side to understand how to individually scale the Elasticsearch cluster as a log store.

Representative actions that an OpenShift cluster administrator needs to figure out besides controlled operations by [elasticsearch operator](https://github.com/openshift/elasticsearch-operator):
- Regular evaluation and balance of the CPU/Memory requests and limits because configuration for ingestion and query paths is not individual configurable.
- Regular evaluation and adaption of redundancy effects on memory and persistent volume usage.
- Usage of `managementState: Unmanaged` to unblock the cluster in situations of bugs, e.g. locked indices, shard allocation.
- Usage of the Elasticsearch cluster API to tune the cluster for options not supported in the Elasticearch custom resource.

On the other hand and based on Openshift Logging team's experience operating Loki as an internal central log aggregation solution, we found that a more specialized solution preserves the same log ingestion/querying user experience while lowering the human operational burden. Representative characteristics that showcase the improvements:
- All components are stateless and can be scaled out individually based on load scenarios.
- Separate scale out for the ingestion and querying path possible.
- Includes rate limiting for fine grained ingestion and query control.
- Persists logs on object storage solutions only, regardless redundancy setting.
- No individual control of any cluster API to tune any operations.
- Provides consistent schema-less model compatible with the rest of our observability tools, i.e. Prometheus, Thanos.

In summary offering an alternative low-retention store based on Loki in cluster logging provides many new benefits in lifting the whole stack to a more cloud-native first experience. This has the potential to lessen the operational and the support burden by a lot.

## Motivation

Currently cluster logging is restricted to providing support for Elasticsearch as a log store. This proposal outlines the API in form of a custom resource definition (CRD) for an alternative log store based on Loki and managed by the [loki-operator](https://github.com/viaq/loki-operator).

In addition it describes how to integrate the custom resource definition into the ClusterLogging custom resource that serves as the single entry point for installing the whole cluster logging stack.

### Goals

The specific goals of this proposals are:

* Outline the API to provide a declarative CRD for Loki deployments managed by the [loki-operator](https://github.com/viaq/loki-operator).

* Propose API support into the ClusterLogging CRD for single entry point deployments.

We will be successful when:

* Users are able to declare a Loki instance via the CRD and the [loki-operator](https://github.com/viaq/loki-operator) rolls out the Loki deployment.

* Users are able to declare Loki as alternative store in the Cluster Logging custom resource.

* Users are able to operate Loki in Cluster Logging without the need to tune every single bit via the proposed CRD.

### Non-Goals

* We will not provide a general purpose API to install Loki at the field. The API is dedicated to cluster logging only.

* We will not expose or implement all possible Loki configuration knobs in the API besides the most crucial to operate in cluster logging.

* We will not expose Loki as a general log store for other ingestion sources than the ones supported and managed by cluster logging.

* We will not offer support for custom labeling of logs other than the defaults provided by the cluster logging collector.

## Proposal

To offer a seamless cluster logging experience with Loki as an alternative store, we propose to limit the API support to the following key features:

* Deploy Loki **only** in microservices mode, i.e. each component is deployed and managed as an individual deployment/statefulset. This enables individual capacity planning and resource utilization per path ingestion/querying.

* Multi-tenancy is enabled by default and requires the collector to apply the required [tenant identification](https://grafana.com/docs/loki/latest/operations/multi-tenancy/) on ingestion time. No user can disable this setting. In turn queries require the same identification information to be successful. Furthermore there is no support for querying across tenants.

* Loki is operated only in a zero-dependency setup mode as described [here](https://grafana.com/docs/loki/latest/configuration/examples/#almost-zero-dependencies-setup). The only externally managed requirement is the availability of an S3-compatible API for object storage to persist logs.

* Support a limited set of Loki scale outs, which are battle-tested on our internally managed Loki deployment. The target scale outs are defined using the format `Nx.{extra-small, small, medium}` where `N` the number of Loki deployments:
  * **1x.extra-small**: Dedicated scale for development and demo purposes only! Not a valid configuration for cluster logging production environments.
  * **1x.small**: Dedicated scale for low-retention log storage without replication factor support. All components are deployed with HA by running at least two replicas.
  * **1x.medium**: Dedicated scale for log-retention log storage with single replication factor support. All components are deployed with HA by running at least two replicas. The only exception is the ingester that requires at minimum three replicas to allow failover when single replication factor enabled.

* Support tuning of CPU/Memory resource and limits for each individual component beyond the selected scale-out size.

* Support adding kubernetes tolerations and node selectors for all Loki components individually. This enables operating the Loki deployment on dedicated nodes.

* Support a limited set of control options from Loki's [limit_config](https://grafana.com/docs/loki/latest/configuration/#limits_config) for the ingestion and query paths. The set is an opinionated list of options to enable tuning of object storage usage. The list of options is offered globally across the whole Loki deployment or per tenant.

* Support for replication factor of two for `1x.small` and `1x.medium` by default.

* Support auto-compaction of the index and chunks stored on the object storage when replication factor of two enabled.

For **1x.extra-small** instances only:

* Support for replication factor of one only.


### User Stories

* As a cluster admin, I want to select a different scale for my Loki deployment to enable single replication factor and auto-compaction.

* As a cluster admin, I want to expand resources/limits for individual Loki components to support load scenarios not declared by the cluster t-shirt size selected.

* As a cluster admin, I want to configure scheduling of individual Loki components to custom selected nodes on my cluster to separate resource usage of the cluster logging from other workloads on my cluster.

* As a cluster admin, I want to tune the ingestion limits globally or per tenant to support custom ingestion characteristics (e.g. ingenstion rate, label cardinality).

* As a cluster admin, I want to tune the query limits globally or per tenant to support custom query characteristics (e.g. chunks per series returned).

### Implementation Details

#### Assumptions

* The proposed API will be owned and managed exclusively as a custom resource definition by the [loki-operator](https://github.com/viaq/loki-operator).

* The [cluster-logging-operator](https://github.com/openshift/cluster-logging-operator) will only create a stanza of this CRD and in addition manage an owner reference on it.

#### Security

The cluster logging admin is responsible for creating and maintaining the secret in the `openshift-logging` namespace to permit access for the Loki deployment to securely connect to an object storage endpoint.

The secret is required to provide all of the following key-value pairs:
- `bucketnames`: Comma separated list of bucket names to evenly distribute chunks over.
- `endpoint`: S3-compatible API Endpoint to connect to.
- `access_key_id`: Access Key ID.
- `access_key_secret`: Secret Access Key.

Optional:
- `region`: Region to use.

### Risks and Mitigations

In general the key risks to provide the proposed API in form of a custom resource definition are:

**Risk:** Providing too many configuration knobs and in turn a brittle API for managing Loki deployments.

**Mitigation:** Limit by maintaining a strong opinionated API for Loki deployments that encompasses only features around dependencies (e.g. object storage), scheduling (e.g. replicas, tolerations/node-selectors) and no more.

**Risk:** Providing poor user experience for day 1 and day 2, e.g. user needs to figure out requests/limits for each component.

**Mitigation:** Provide opinionated and battle-tested scale out formats in form of a T-Shirt size (e.g. `1x.medium`). Leaving room for adding more formats later (e.g. `2x.medium` for hard multi-tenancy) and knobs (e.g. replicas) to adapt the format to live environments.

## Design Details

The discussion about the proposed API is split in two sections. The first section `lokistack_types.go` describes the custom resource definition for the [loki-operator](https://github.com/viaq/loki-operator) to reconcile a Loki deployment.

The API in `lokistack_types.go` includes all required and optional configuration options to support the [proposal](#proposal) above. Furthermore the status struct in `lokistack_types.go` declares conditions for the Loki deployment. The Loki deployment can be under various situations in a degraded state resulting in no deployment.

The second section `clusterlogging_types.go` describes the minimal stanza for translating the `logStore` spec into a `LokiStackSpec`.

### lokistack_types.go

#### Types and constants

A `LokiStack` custom resource can declare one of the following three sizes as values of the `LokiStackSpec`'s field `Size`. Each value has impact to all of the following per Loki component:

1. `Replicas`: The number of replica pods per Loki component. Besides two exceptions all components are deployed with at minimum two replicas. First exception is the `1x.extra-small` resulting in a single replica per component. Second exception is the Loki compactor that is always deployed with a single replica, because multiple replicas are not supported.
2. `Requests/Limits`: The number of requested CPU (in millicores) and Memory (in Bytes) and their limits per component.
3. `PodDisruptionBudgets`: The number of minimum available pods per component to ensure normal operations under selected settings, e.g. `ReplicationFactor = 1`.

```go
// ManagementStateType defines the type for CR management states.
//
// +kubebuilder:validation:Enum=Managed;Unmanaged
type ManagementStateType string

const (
    // ManagementStateManaged when the LokiStack custom resource should be
    // reconciled by the operator.
    ManagementStateManaged ManagementStateType = "Managed"

    // ManagementStateUnmanaged when the LokiStack custom resource should not be
    // reconciled by the operator.
    ManagementStateUnmanaged ManagementStateType = "Unmanaged"
)

// LokiStackSizeType declares the type for Loki deployment scale outs.
//
// +kubebuilder:validation:Enum=OneXExtraSmallSize;OneXSmall;OneXMedium
type LokiStackSizeType string

const (
    // OneXExtraSmallSize defines the size of a single Loki deployment instance
    // with extra small resources/limits requirements and without HA support.
    // This size is ultimately dedicated for development and demo purposes.
    // DO NOT USE THIS IN PRODUCTION!
    OneXExtraSmallSize LokiStackSizeType = "1x.extra-small"

    // OneXSmall defines the size of a single Loki deployment instance
    // with small resources/limits requirements and HA support for all
    // Loki components. This size is dedicated for setup **without** the
    // requirement for replication factor of two and auto-compaction.
    OneXSmall LokiStackSizeType = "1x.small"

    // OneXMedium defines the size of a single Loki deployment instance
    // with small resources/limits requirements and HA support for all
    // Loki components. This size is dedicated for setup **with** the
    // requirement for replication factor of two and auto-compaction.
    OneXMedium LokiStackSizeType = "1x.medium"
)

```

#### Mandatory: Loki deployment specification

The mandatory fields of a `LokiStack` custom resource declaration are:
- `Size`: Selection of one of the supported t-shirt size formats (see [Types and constants](#types-and-constants)) to reconcile the `CPU/Memory Requests`, `CPU/Memory Limits`, `Replicas` and `PodDisruptionBudgets` per Loki component.
- `Storage`: A value of `ObjectStorageSpec` to declare the S3 compatible-API endpoint in a `Secret` located in `openshift-logging` by a OpenShift administrator.
- `ReplicationFactor`: The replication factor declares how many Loki ingester replicas should process each log stream in parallel. This does not always result in replicated data on the object storage. It is a safety measure to allow failover of ingester replicas because of unexpected disruption (e.g. node rescheduling, cluster upgrades, etc.). In turn `1` can result in data loss.

__Note__: Redundancy of the persistent object storage is configured by the administator per bucket based on the object storage provider policies available (e.g. multi-zone/region buckets).

The optional parts are `Limits` (see discussion in section [Optional: Global/Per-Tenant Limits](#optional-globalper-tenant-limits)) and `Template` (see details in section [Optional: Loki component configuration](#optional-Loki-component-configuration)).

__Note__: The `Template` field if left empty will be populated by the [loki-operator](https://github.com/viaq/loki-operator) on the first reconciliation by translating the `Size` selection. This automatic update is to inform the user about the opinionated defaults for each supported scale out beyond documentation. The user can use the `Template` field to override these defaults.

```go
// ObjectStorageSecretSpec is a secret reference containing name only, no namespace.
type ObjectStorageSecretSpec struct {
    // Name of a secret in the namespace configured for object storage secrets.
    //
    // +required
    Name string `json:"name"`
}

// ObjectStorageSpec defines the requirements to access the object
// storage bucket to persist logs by the ingester component.
type ObjectStorageSpec struct {
    // Secret for object storage authentication.
    // Name of a secret in the same namespace as the cluster logging operator.
    //
    // +required
    Secret *ObjectStorageSecretSpec `json:"secret,omitempty"`
}

// LokiStackSpec defines the desired state of LokiStack
type LokiStackSpec struct {

    // ManagementState defines if the CR should be managed by the operator or not.
    // Default is managed.
    //
    // +required
    // +kubebuilder:default:=Managed
    // +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Management State"
    ManagementState ManagementStateType `json:"managementState,omitempty"`

    // Size defines one of the support Loki deployment scale out sizes.
    //
    // +required
    Size LokiStackSizeType `json:"size,omitempty"`

    // Storage defines the spec for the object storage endpoint to store logs.
    //
    // +required
    Storage ObjectStorageSpec `json:"storage,omitempty"`

    // ReplicationFactor defines the policy for log stream replication.
    //
    // +required
    ReplicationFactor int32 `json:"replicationFactor,omitempty"`

    // Limits defines the limits to be applied to log stream processing.
    //
    // +optional
    Limits *LimitsSpec `json:"limits,omitempty"`

    // Template defines the resource/limits/tolerations/nodeselectors per component
    //
    // +optional
    Template *LokiTemplateSpec `json:"template,omitempty"`
}
```

#### Optional: Global/Per-Tenant limits

In general the `LimitsSpec` supports declaring limits for the ingestion and/or the query path individually. The settings can be applied either globally or per tenant. One exception to this rule is the ingestion limit `MaxGlobalStreamsPerTenant` which is applied on ingestion globally.

```go
// LimitsTemplateSpec defines the limits  applied at ingestion or query path.
type LimitsTemplateSpec struct {
    // IngestionLimits defines the limits applied on ingested log streams.
    //
    // +optional
    IngestionLimits IngestionLimitSpec `json:"ingestion,omitempty"`

    // QueryLimits defines the limit applied on querying log streams.
    //
    // +optional
    QueryLimits QueryLimitSpec `json:"queries,omitempty"`
}

// LimitsSpec defines the spec for limits applied at ingestion or query
// path across the cluster or per tenant.
type LimitsSpec struct {

    // Global defines the limits applied globally across the cluster.
    //
    // +optional
    Global LimitsTemplateSpec `json:"global,omitempty"`

    // Tenants defines the limits applied per tenant.
    //
    // +optional
    Tenants map[string]LimitsSpec `json:"tenants,omitempty"`
}
```

##### Query Limits

The global/per-tenant query limits ensure sane default sizing of query result sets. In general users of [Loki HTTP API's](https://grafana.com/docs/loki/latest/api/) query capabilities can provide a `limit` and/or `start`, `end` parameters to control the query result set. However, if none given the limits below reflect sane defaults to keep the query path from unbound operations.

In addition a sane default for `MaxChunksPerQuery` limits the required object storage operations and in turn provides sanity in object storage per-operation cost.

```go
// QueryLimitSpec defines the limits applies at the query path.
type QueryLimitSpec struct {

    // MaxEntriesLimitPerQuery defines the aximum number of log entries
    // that will be returned for a query.
    //
    // +optional
    MaxEntriesLimitPerQuery int32 `json:"maxEntriesLimitPerQuery,omitempty"`

    // MaxChunksPerQuery defines the maximum number of chunks
    // that can be fetched by a single query.
    //
    // +optional
    MaxChunksPerQuery int32 `json:"maxChunksPerQuery,omitempty"`

    // MaxQuerySeries defines the the maximum of unique series
    // that is returned by a metric query.
    //
    // + optional
    MaxQuerySeries int32 `json:"maxQuerySeries,omitempty"`
}
```

##### Ingestion Limits

The global/per-tenant ingestion limits are typical rate limiting control options. The administrator can control the following options:

1. Log volume rate and burst size to omit unbound memory operations, i.e. by setting `IngestionRate` and `IngestionBurstSize`.
2. Label key and value lenghts to ensure faster log stream lookups, i.e. by setting `MaxLabelLength`, `MaxLabelValueLength`, `MaxLabelNamessPerSeries`.
3. Log stream limits to ensure better object storage utilization, i.e. by setting `MaxGlobalStreamsPerTenant`.

From the above the most significant for healthy Loki operations are the the log stream limits. Since Logs are aggregated per tenant and unique set of labels to log streams, a high number of streams has a negative impact on object storage usage.

This translates in general into the pattern that too many unique streams result in too many small chunks. In turn smaller chunks require more object storage operations to download/upload them. The general advice is to control the stream limits as per settings below and ensure a low churn on the label side by controlling cardinality.

```go
// IngestionLimitSpec defines the limits applied at the ingestion path.
type IngestionLimitSpec struct {

    // IngestionRate defines the sample size per second. Units MB.
    //
    // +optional
    IngestionRate int32 `json:"ingestionRate,omitempty"`

    // IngestionBurstSize defines the local rate-limited sample size per
    // distributor replica. It should be set to the set at least to the
    // maximum logs size expected in a single push request.
    //
    // +optional
    IngestionBurstSize int32 `json:"ingestionBurstSize,omitempty"`

    // MaxLabelLength defines the maximum number of characters allowed
    // for label keys in log streams.
    //
    // +optional
    MaxLabelLength int32 `json:"maxLabelLength,omitempty"`

    // MaxLabelValueLength defines the maximum number of characters allowed
    // for label values in log streams.
    //
    // +optional
    MaxLabelValueLength int32 `json:"maxLabelValueLength,omitempty"`

    // MaxLabelNamessPerSeries defines the maximum number of label names per series
    // in each log stream.
    //
    // +optional
    MaxLabelNamessPerSeries int32 `json:"maxLabelNamesPerSeries,omitempty"`

    // MaxGlobalStreamsPerTenant defines the maximum number of active streams
    // per tenant, across the cluster.
    //
    // +optional
    MaxGlobalStreamsPerTenant int32 `json:"maxGlobalStreamsPerTenant,omitempty"`

    // MaxLineSize defines the aximum line size on ingestion path. Units in Bytes.
    //
    // +optional
    MaxLineSize int32 `json:"maxLineSize,omitempty"`
}
```

#### Optional: Loki component configuration

The following optional template for each Loki component is dedicated for tuning resources and scheduling on available cluster nodes.

```go
// LokiComponentSpec defines the requirements to configure scheduling
// of each Loki component individually.
type LokiComponentSpec struct {
    // Replicas defines the number of replica pods of the component.
    //
    // +optional
    Replicas int32 `json:"replicas,omitempty"`

    // NodeSelector defines the labels required by a node to schedule
    // the component onto it.
    //
    // +optional
    NodeSelector map[string]string `json:"nodeSelector,omitempty"`

    // Tolerations defines the tolerations required by a node to schedule
    // the component onto it.
    //
    // +optional
    Tolerations []corev1.Toleration `json:"tolerations,omitempty"`
}

// LokiTemplateSpec defines the template of all requirements to configure
// scheduling of all Loki components to be deployed.
type LokiTemplateSpec struct {

    // Compactor defines the compaction component spec.
    //
    // +optional
    Compactor *LokiComponentSpec `json:"compactor,omitempty"`

    // Distributor defines the distributor component spec.
    //
    // +optional
    Distributor *LokiComponentSpec `json:"distributor,omitempty"`

    // Ingester defines the ingester component spec.
    //
    // +optional
    Ingester *LokiComponentSpec `json:"ingester,omitempty"`

    // Querier defines the querier component spec.
    //
    // +optional
    Querier *LokiComponentSpec `json:"querier,omitempty"`

    // QueryFrontend defines the query frontend component spec.
    //
    // +optional
    QueryFrontend *LokiComponentSpec `json:"queryFrontend,omitempty"`
}
```

### Status

The `LokiStackStatus` is quite spartan and declares the most important conditions of the Loki deployment that cannot be expressed by metrics. A managed Loki deployment can be either in state `Ready` or `Degraded`. In summary conditions under which the Loki deployment is `Degraded` are:

1. The secret resource to access the object storage is either missing or invalid, e.g. missing `bucketnames` or malformed `endpoint`.
2. The selected replication policy is not available for the selected scale out size, e.g. `ReplicationFactor = 1` and `1x.small` are incompatible.

If the configuration given in a `LokiStack` custom resource results in a `Degraded` cluster, **no** Loki deployment will be created.

```go
// LokiStackConditionType deifnes the type of condition types of a Loki deployment.
type LokiStackConditionType string

const (
    // ConditionReady defines the condition that all components in the Loki deployment are ready.
    ConditionReady LokiStackConditionType = "Ready"

    // ConditionDegraded defines the condition that some or all components in the Loki deployment
    // are degraded or the cluster cannot connect to object storage.
    ConditionDegraded LokiStackConditionType = "Degraded"
)

// LokiStackConditionReason defines the type for valid reasons of a Loki deployment conditions.
type LokiStackConditionReason string

const (
    // ReasonMissingObjectStorageSecret when the required secret to store logs to object
    // storage is missing.
    ReasonMissingObjectStorageSecret LokiStackConditionReason = "MissingObjectStorageSecret"

    // ReasonInvalidObjectStorageSecret when the format of the secret is invalid.
    ReasonInvalidObjectStorageSecret LokiStackConditionReason = "InvalidObjectStorageSecret"

    // ReasonInvalidReplicationConfiguration when the configurated replication factor is not valid
    // with the select cluster size.
    ReasonInvalidReplicationConfiguration LokiStackConditionReason = "InvalidReplicationConfiguration"
)

// LokiStackStatus defines the observed state of LokiStack
type LokiStackStatus struct {
    // Conditions of the Loki deployment health.
    //
    // +optional
    Conditions []metav1.Condition `json:"conditions,omitempty"`
}
```

### clusterlogging_types.go

The integration of the `LokiStack` CRD into `ClusterLogging/v1` custom resources is following the present pattern to first into the latter.

```go
type LogStoreType string

const (
    LogStoreTypeElasticsearch LogStoreType = "elasticsearch"

    LogStoreTypeLoki  LogStoreType = "loki"
)

type LogStoreSpec struct {
    // The type of Log Storage to configure
    Type LogStoreType `json:"type`"

    LokiStackSpec `json:"loki,omitempty"`
}
```

### Test Plan

#### Unit Testing

The [loki-operator](https://github.com/viaq/loki-operator) includes a framework based on [counterfeiter](github.com/maxbrunsfeld/counterfeiter) to create simple and useful fake client to test configuration in unit tests.

Testing of the reconciliation of a `LokiStack` custom resource will be based upon the same technique where the custom resource describes the inputs and the generated manifests the outputs.

#### Functional Testing

The [cluster-logging-operator](https://github.com/openshift/cluster-logging-operator) provides a simple [functional test framework](https://github.com/openshift/cluster-logging-operator/tree/master/test/functional) to test the configuration provided in a `ClusterLogging` custom resource to collector configuration.

Testing the integration of the `LokiStack` custom resource in the `ClusterLogging` custom resource will be based upon this technique.

#### E2E Testing

Finally E2E testing requiring a running cluster in form of [kind](https://kind.sigs.k8s.io/) or [OpenShift](https://github.com/openshift/) is dedicated later for:

1. Upgrade `LokiStack` scenarios.
2. Compatibility testing with various S3-compatible object-storage providers (e.g. Openshift Storage).
3. Integration with various caching providers, e.g. Memcached, Redis.

### Graduation Criteria

#### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback by switching the internal Loki service to be managed by the [loki-operator](https://github.com/viaq/loki-operator).
- Write metrics for collecting telemetry across the fleet.
- Write symptoms-based alerts and playbooks for the components.

#### Tech Preview -> GA

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default
- Conduct load testing per t-shirt size supported.

#### Removing a deprecated feature

Not applicable here.

### Upgrade / Downgrade Strategy

Not applicable here.

### Version Skew Strategy

Given we are adding a new API there should be minimal concern for upgrading. The API is considered as an alternative to the Elasticsearch API used in Cluster Logging API.

## Implementation History

| Release | Description              |
| ---     | ---                      |
| TBD     | **TP** - Initial release |
| TBD     | **GA** - Initial release |

## Drawbacks

In summary the general drawbacks to introduce an alternative log store into cluster logging are:

1. Users need to hard switch their store from Elasticsearch to Loki with a small downtime. In turn a manual migration of data might be relevant from case to case.
2. Loki is a fairly new but stable systems designed for cloud first operations. However the misconduct in using `LimitsSpec` might result is a higher cost for object storage usage and operations than expected.
3. The switch from Elasticsearch to Loki requires switching operational mentality. Loki is built upon distributed stateless applications rather than stateful cluster nodes. In turn this requires separate capacity planning for ingestion and query path.
4. Loki is bound to the replication policy and this translates in required ingester replicas and the compactor additionally. It is bound to query load scenarios and might require caching integration. In addition it could be eventually scaled by an autoscaler. In turn all this adds to the learning curve to handle the whole system (e.g. adding caching, autoscaling).

## Alternatives

None.

## Infrastructure Needed

None.
