---
title: cluster-logging-es-pipeline-processing

authors:
  - "@lvlcek"

reviewers:
  - "@alanconway"
  - "@jcantrill"
  - "@ewolinetz"

approvers:
  - "@alanconway"
  - "@jcantrill"
  - "@ewolinetz"

creation-date: 2020-08-20
last-updated: 2020-08-20
status: provisional|implementable|implemented|deferred|rejected|withdrawn|replaced
see-also: []
replaces: []
superseded-by: []
---

# Cluster Logging Elasticsearch Pipeline Processing Of Indexed Documents

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This proposal opens possibility to route individual documents (logs) into different Elasticsearch indices
based on data driven criteria.  Under the hood it utilizes Elasticsearch Ingesting [Pipeline Definition](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/pipeline.html) to dynamically inspect incoming documents and apply appropriate processor(s) such as [parsing JSON data](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/json-processor.html) in document field or pointing
the document to a different index (similarly to [date-index-name-processor](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/date-index-name-processor.html)).

This will enable indexing and storing of documents that carry custom data (such as JSON payload) into different index to avoid index mapping collisions and improves Elasticsearch querying and Kibana dashboarding experience.

## Motivation

One of the goals of [introduction of the new data model](cluster-logging-es-rollover-data-design.md) was to significantly reduce the number of indices in Elasticsearch cluster. Keeping the number of indices (and thus index shards) at bay is a critical step towards improved Elasticsearch stability and performance.

We had to make sure we could switch from _"index per namespace model"_ to _"common shared index for all projects"_ (see the new data model proposal for details). In order to store the data in common index Elasticsearch requires the data must share the same data model which directly translates to specific [index mappings](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/mapping.html). Data having conflicting model can not be stored in common index.

However, common data model does not solve another challenge associated with the ask to properly index, store and query any additional (extra) custom data the documents can carry. For example logs can contain project or business domain specific JSON data that is not captured in the common data model and is not shared by all the data sources.

Over the time we have gathered number of requests from customers asking for possibility to process additional data. See <https://issues.redhat.com/browse/LOG-785>.

For the purpose of this proposal we have already investigated various approaches (and their pros and cons) of storing additional JSON data in Elasticsearch indices as part of <https://issues.redhat.com/browse/LOG-842>.


### Goals

The high-level goal of this proposal is to lay the groundwork to enable approaches B) and C) as discussed in [LOG-842](https://issues.redhat.com/browse/LOG-842).

Quick comparison of approaches B) and C):

- Approach B) Whenever user needs to store data with a new data model we solve it by pointing this data to new index (or to existing index if there is already one with exactly the same data model). This is very comfy approach for the user except it can lead to very high number of indices and querying experience can be complicated because there are very little or no common conventions of document fields meaning and types. The chance is that we can achieve this goal with a single general pipeline processing rule (i.e. a pipeline rule provided within EO by default).

- Approach C) is on the other side of the spectrum. All data is stored into common index and whenever user needs to store data with a new data model then it is necessary to transform the data to fit existing data model in existing index. This approach requires attention to data model and users may need to invest time and resources to transform new data sources. On the other hand there is minimum additional indices created in ES cluster and data can be queried uniformly because document fields have predictable meaning and types. With this approach we will need to enable more specific pipeline rules or even custom rules. This will mean we will need to implement some kind of life–cycle management of the rules (**who** can do it, **how** can it be done, **when** it can be done...).

It should be pointed out that Elasticsearch pipeline processes can support both cases. Depending on how much users are willing to pay attention to their data model they can start with approach B) and eventually transition to approach C) once they feel more confident about managing their own data model across more log resources.

In this proposal we assume that the approach B) is the one we would start enabling first. But nothing is stopping us from enabling the approach C) in the future. Though it is fair to say that approach C) will be more complicated to implement because it will require management of custom processing pipelines.
This also means that implementing the approach C) requires implement all that is needed for the approach B) first anyway.

We will be successful when:

* We are able to configure specific logs to be pointed into different (non-default) index in Elasticsearch based on the data contained in the document itself.
* An OCP user has enough information and tooling to undertstand and evaluate the impact of creating additional indices. This specifically means new index level metrics will need to be introduced and relevant alerts must be provided as well. Notice that we have recently turned off index level Prometheus metrics (because they were high-cardinality metrics) thus we will need to revisit index level metrics and provide new aggregated non-high cardinality metrics.
* Any additional non-default indices must be subject to existing retention policy. At the implementation level this means that any new indices have names that comply with data retention implementation OOTB.
* A multi-tenant data visibility works seamlessly.
* A roll-over process can manage additional indices seamlessly.
* An OCP user is able to query data from additional indices.
* An OCP user is provided enough documentation to be able to understand **when** and **how** to use this feature.

Optional points, not required for initial implementation:

* An OCP user (or admin only?) can provide custom index mapping for the data. This is specifically needed if the [default Elasticsearch type inference mechanism](https://www.elastic.co/guide/en/elasticsearch/reference/current/dynamic-field-mapping.html) does not lead to acceptable results.

### Non-Goals

* This proposal does not addresses limitations implemented by Elasticsearch to prevent [mapping explosion](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/mapping.html#mapping-limit-settings) (except we need to provide more metrics if needed, see above).
* This proposal does not enable storing documents with conflicting mappings into common index.


## Proposal

When incoming document contains custom data it can happen that it can not be stored into default index because existing index mapping (the index data schema) would conflict with the data in the incoming document.

The main idea in this proposal is to avoid mapping conflicts by pointing incoming document into diffrent index than the default one (how it is decided is discussed later). If that different index hasn't been created yet then it is created on the fly (this is how Elasticsearch works OOTB). In this proposal this index is called an **additional index**.

When an additional index is created then it can optionally use additional [index templates](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/indices-templates.html) to further specify types of individual custom fields. In the begining we assume that [default Elasticsearch type inference mechanism](https://www.elastic.co/guide/en/elasticsearch/reference/current/dynamic-field-mapping.html) does "good enough" job in most cases and we do not focus on additional index templates (let's leave this for the later).

### Configure Elasticsearch Pipeline Definition

The key mechanism in pointing incoming document into different (additional) index is in using Ingest Node [Pipeline processors](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/pipeline.html).

Let's see basic example:

```
# We assume there is an Elasticsearch cluster running on localhost:9200
# Define pipeline to store incoming document into monthly named index

$ curl -X PUT \
    -H 'Content-Type: application/json' \
    "localhost:9200/_ingest/pipeline/monthlyindex" \
-d'
{
  "description" : "monthly date-time index naming",
  "processors" : [
    {
      "date_index_name" : {
        "field" : "date1",
        "index_name_prefix" : "myindex-",
        "date_rounding" : "M"
      }
    }
  ]
}'

```

Now, let's index document into index called `myindex` and instruct Elasticsearch to execute the `monthlyindex` pipeline when indexing the document.

```
$ curl -X PUT \
    -H 'Content-Type: application/json' \
   "localhost:9200/myindex/_doc/1?pipeline=monthlyindex" \
-d'   
{
  "foo": "bar",
  "date1" : "2016-04-25T12:02:01.789Z"
}'
```

The result is that incoming document was indexed into index called `myindex-2016-04-01`.

```
$ curl localhost:9200/_cat/indices?h=index,docs.count
myindex-2016-04-01 1
```
This example is based on existing pipeline processor called [Date Index Name Processor](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/date-index-name-processor.html).

There are about [30 pipeline processors](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/ingest-processors.html) already available in Elasticsearch but we will need to **implement our own** to make sure it works well with our index naming schema (i.e. additional indices must align perfectly with curation, aliasing, roll-over and other index management processes).

Pipeline processor can be [implemented as Elasticsearch plugin](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/ingest-processors.html#_ingest_processor_plugins).

Now, assume we have processor that can point incoming document into additional index. As of now, this proposal does not go into details about what the naming schema for the additional indices will be (this is something that we still need investigate) but it is fair to say that for the index naming we will be able to use any information from the incoming document (i.e. project name field, namespace field, labels, ... etc) and plugin configuration as well. This is what must be enough to create general naming schema for additional indices.

In combination with other existing processors we will be able to design pipeline that will point the document into additional index based on simple or complex conditions.

### Pipeline Processing Overhead

We have seen in the example above that for the incoming document to go through particular processing pipeline Elasticsearch need to get the name of the pipeline as part of indexing request.

The name of pipeline is an [optional URL parameter](https://www.elastic.co/guide/en/elasticsearch/reference/6.8/ingest.html) in Document Indexing API or optional item in [Bulk indexing request](https://www.elastic.co/guide/en/elasticsearch/reference/7.8/docs-bulk.html#docs-bulk-api-query-params) (found in 7.x doc only, the 6.x doc does not mention this but it works too, this is perhaps a doc bug?).

```
# Example of using the pipeline in Bulk API

$ curl -X POST "localhost:9200/_bulk?pretty" \
  -H 'Content-Type: application/json' \
-d'
{ "index" : { "_index" : "myindex", "_type": "_doc" } }
{ "date1" : "2016-04-25T12:02:01.789Z" }
{ "index" : { "_index" : "myindex", "_type": "_doc", "pipeline": "monthlyindex" } }
{ "date1" : "2016-04-25T12:02:01.789Z" }
'
```

The output is:

```
$ curl localhost:9200/_cat/indices?h=index,docs.count
myindex-2016-04-01 1
myindex            1
```

### ES 6.x is not alreardy getting bug fixes

ES 6.x is already EOL and if there are any significant pipeline bugs that are not backported then this can be serious issue for us (unless we build custom ES 6.x release or upgrade to ES 7.x).

As of writing I was able to find some pipeline related issuse (these were **not backported**):
- https://github.com/elastic/elasticsearch/issues/60437 ([backport ticket](https://github.com/elastic/elasticsearch/issues/58478))
- https://github.com/elastic/elasticsearch/issues/58478 ([backport ticket](https://github.com/elastic/elasticsearch/pull/60818))

We need to investigate if any of these is a red flag for us.

If any bug is found that will impact use of pipeline parameters in Bulk APIs then we still have an option to fallback to the default pipeline. If a default pipeline is configured then the pipeline ID does not have to be named in indexing or bulk requests at all but all documents go through it first. And this **could have performance impact**.

### Additional Prometheus metrics

Indexing documents into additional indices can lead to performance issues because of high number of shards in Elasticsearch. And if users can decide that more indices are created then we must put in place more index level metrics and alerts for the users to be warned heads up.

Possible new metrics and alerts that will need to be introduced:

- Some aggregation of index level metrics (we must still avoid high-cardinality metrics).
- Alerts based on # of indices and # of fields per indices (in relation to max field limit).
- Identify pods/apps that create too many indices.

See <https://issues.redhat.com/browse/LOG-854>