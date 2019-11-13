---
title: cluster-logging-es-rollover-data-design
authors:
  - "@jcantril"
reviewers:
  - "@bparees"
  - "@ewolinetz"
  - "@richm"
  - "@portante"
  - "@pavolloffay"
  - "@sichvoge"
approvers:
  - "@ewolinetz"
  - "@richm"
creation-date: 2019-11-04
last-updated: 2019-11-04
status: implementable
---

# Cluster Logging Elasticsearch Rollover Data Design

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] ~~Graduation criteria for dev preview, tech preview, GA~~
- [ ] User-facing documentation is created in [openshift/docs]

## Summary

This proposal alters the data design for storing logs in Elasticsearch to co-locate logs
to fewer indices.  It additionally leverages the [Elasticsearch rollover api](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/indices-rollover-index.html) to
help maintain the number of indices and shards in order to align with Elastic's 
[performance](https://www.elastic.co/guide/en/elasticsearch/guide/2.x/scale.html) 
and scaling [recommendations](https://www.elastic.co/blog/how-many-shards-should-i-have-in-my-elasticsearch-cluster).

## Motivation

The initial data design for Cluster Logging segments logs by OpenShift namespace in order to facilitate multi-tenant support and data curation.  This choice was made because
index level security was the only feature available from an open source library.  It additionally facilitated curation as full indicies could be removed when the retention period
expired.  This means, however, at any one time, there are at least "**$noOfNamespaces * $daysRetained**" number 
of indices maintained by the Elasticsearch server.  Each index can additionally be sharded to spread the load across the Elasticsearch nodes.  The end
result of this shard explosion is the Elasticsearch cluster performance is not optimized and is not capable of efficiently processing and storing logs.

Each index adds load and overhead (e.g. mapping, metadata) to the Elasticsearch cluster that needs to be tracked beyond the actual data.   Elasticsearch has recommendations for maximum shard size and the number of shards per node per allocated gigabyte of heap.  Cluster logging typically exceeds these recommendations for any OpenShift clusters that has significant log traffic.


### Goals

The goals of this proposals are:
* Utilize a data design that aligns data schema with Elasticsearch's recommendations.
* Expose data management policy as API in the `cluster-logging-operator` and `elasticsearch-operator` in support of Cluster Logging's mission to gather log sources
* Migrate indicies from the previous schema into the new.  Migrated inidices will be governed by the data management policy exposed by this proposal

### Non-Goals

This change will not:
* Provide a general data management policy API that fully exposes Elasticsearch's rollover API

## Proposal

This proposal introduces two specific changes to to achieve its goals:

* Co-located data in a few opinionated indices
* Index management using [rollover index API](https://www.elastic.co/guide/en/elasticsearch/reference/6.3/indices-rollover-index.html)

Logs of a given type (e.g. app container, infra container, node) are separated by index.  The Cluster Logging collector writes logs to a well-known alias established by the `cluster-logging-operator`.  The ClusterLogging CR instance specifies the management policy for each log type index.  This controls: 
* the maximum age (e.g. 7 days).  

This policy is passed to the Elasticsearch CR for the `elasticsearch-operator` to manage the rollover policy.  The details of the policy are specified by the `cluster-logging-operator` and are based on the guidelines suggested by Elasticsearch.


### Implementation Details

#### Assumptions

* ClusterLogging will expose the minimal needed set of the Elasticsearch CR rollover policy management API in order to achieve the previously described goals
* ClusterLogging will manage rollover as Elasticsearch index management is either restricted by Elastic licensing or not available in the opensource version of OpenDistro
* Security will be addressed by using the OpenDistro security plugin and document level security (DLS). Details TBD.

#### Data Model
Logs of a given type are co-located to the following indices:

| Log Type | Read Alias | Write Alias | Initial Index |
|---------|-----------|---------------|---------------|
| Node (`logs.infra`)|infra, infra.node|infra.node-write|node.infra-00001|
| Container Infra (`logs.infra`)|infra, infra.container|infra.container-write|container.infra-000001|
| Container Application (`logs.app`)|app.logs|app.container-write|container.app-000001|
| Audit (`logs.audit`)|infra.audit|infra.audit-write|audit.infra-000001|

**Note:** Log types are further defined in [LogForwarding](./cluster-logging-log-forwarding.md).

#### ClusterLogging API
```
apiVersion: "logging.openshift.io/v1"
kind: "ClusterLogging"
metadata:
  name: "instance"
spec:
  logStore:
    retentionPolicy: 
      logs.app:
        maxAge: 7d
      logs.infra
        maxAge: 7d

```
#### Cluster Logging Operator
The `cluster-logging-operator` will utilize the ClusterLogging CR retention policy to spec
the desired aforementioned Elasticsearch CR indexManagement.
#### Elasticsearch API
```
apiVersion: "logging.openshift.io/v1"
kind: "Elasticsearch"
metadata:
  name: "elasticsearch"
spec:
  indexManagement:
    policies:
    - name: infra-policy
      pollInterval: 5m
      phases:
        hot:
          actions:
            rollover:
              maxAge:   3d
        delete:
          minAge: 7d
    mappings:
    - name:  node.infra               #creates node.infra-00001 aliased node.infra-write
      policyRef: infra-policy         #policy applies to index patterns node.infra*
      aliases:
      - infra  
```
#### Elasticsearch Operator
The `elasticsearch-operator` will be modified to:
* Expose the index management API
* Create and seed index templates to support the policy
* Create the initial indices as needed
* Block data ingestion if needed until initial index is seeded
* Deploy curation CronJob for each mapping to rollover using the defined policy

#### Collector 
* Update the [`fluent-plugin-viaq-data-model`](https://github.com/ViaQ/fluent-plugin-viaq_data_model) to allow defining a static index to write logs

Example configuration:
```    
  ....
   <elasticsearch_index_name>
     tag "**"
     name_type static 
     static_index_name 'container.app-write'
   </elasticsearch_index_name>
```
#### Deprecations
* The Curator Cronjob deployed by the `cluster-logging-operator` will be removed. The responsibilities for curation will be subsumed by
 implementation of Elasticsearch rollover management.
* Curation configuration by namespace is no longer configurable and is restricted to cluster
wide settings associated with log type 

## Design Details

### Test Plan
* Regression tests will be executed to confirm no regressions from previous releases
#### Unit Tests
* Unit tests will be modified to account for the change in data design
#### Integration Tests
* e2e tests will be modified to account for the change in data design

### Upgrade / Downgrade Strategy

#### Upgrade
The `elasticsearch-operator` will migrate existing log indices to work with the new data design by:
* Indices beginning with `project.*` are aliased to: `app.logs`
* Indices beginning with `.operations.*` are indexed to `infra`
* Migrated indices are deleted after migration

**Note:** The `cluster-logging-operator` will leave the deployed curation CronJob to manage indices from the older data schema.  These indices will be curated as previously and, eventually, removed from the cluster.  The curation CronJob will be removed in fugure releases.

#### Downgrade
Downgrades should be discouraged unless we know for certain the Elasticsearch version managed by cluster logging is the same version.  There is risk that Elasticsearch may have migrated data that is unreadable by an older version.

## Implementation History

| release|Description|
|---|---|
|4.4| GA release of rollover data design

## Drawbacks

The drawback to not implementing this change is that Cluster Logging will:
* Continue to experience performance and scaling issues directly related to a less then optimal data schema

## Alternatives

There are no current alternatives


## References

* [Elasticsearch Rollover API documentation](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/indices-rollover-index.html)

### Commands
#### Create Template
```
curl http://localhost:9200/_template/app_logs?pretty -HContent-Type:application/json -XPUT -d '{
  "index_patterns": ["app.container*"], 
  "settings": {
    "number_of_shards": 1,
    "number_of_replicas": 1,
  },
  "aliases": {
    "app.logs": {}
   }
}'
```
#### Initialize Index
```
curl http://localhost:9200/app.container-000001?pretty -HContent-Type:application/json -XPUT -d '{
  "aliases": {
    "app.container-write": {"is_write_index": true},
    "app.logs": {}
   }
}'
```
#### Insert Data
```
curl http://localhost:9200/app.container-write/_doc/0?pretty \
  -HContent-Type:application/json -XPOST -d '{"value":"1"}'

```
#### Retrieve Data
```
curl http://localhost:9200/app.logs/_search?pretty \
  -HContent-Type:application/json
```
#### Rollover Index
```
curl http://localhost:9200/app.container-write/_rollover?pretty \
  -HContent-Type:application/json -XPOST -d '{"conditions": {"max_docs": 1}}'
```
