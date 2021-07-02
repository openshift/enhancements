---
title: elasticsearch-v2-api-and-infrastructure
authors:
  - "@ewolinetz"
reviewers:
  - "@jcantrill"
  - "@bparees"
  - "@alanconway"
approvers:
  - "@jcantrill"
  - "@bparees"
  - "@alanconway"
creation-date: 2021-07-02
last-updated: 2021-06-02
status: implementable
see-also: []
replaces: []
superseded-by: []
---

# elasticsearch-v2-api-and-infrastructure

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Migration plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Currently when interacting with the Elasticsearch Operator via the Elasticsearch CRD, one needs to have a lot of knowledge on how to best configure
the Elasticsearch cluster node's roles, memory/cpu, and replication count. We improve the user experience of the operator, and provide a more performant
cluster that can operate at scale, by both simplifying the API and moving the knowledge of the desired infrastructure into the operator itself.

## Motivation

Currently the majority of our Elasticsearch bugs are performance related and are usually clusters that have been scaled vertically rather than horizontally.
This can be summed up to the fact that it can be difficult to correctly decide the memory that each node should be allocated, and the number of nodes
that should be in the cluster.

Given each Elasticsearch node's uses its memory differently based on the role is is performing, having nodes that act as "All-In-One" causes resources
to not be used in optimal ways and usually results an over allocation of resources.
This over allocation can make it hard to justify horizontal scaling, which leads to more performance related issues.

Reducing this decision to t-shirt sizes helps abstract this complicated understanding from users and simplifies the experience to picking the
most appropriate t-shirt given their use case.

For EO developers, this has the added benefit of allowing us to move away from the use of UUIDs which would simplify our code.

### Goals
The specific goals of this proposal are:

* Outline the responsibility for the Elasticsearch Operator regarding desired infrastructure

* Provide a v2 elasticsearch CRD API

* Provide upgrade strategies from v1 to v2

We will be successful when:

* Users of Elasticsearch Operator no longer need to specify their node roles and count
* Users can specify a t-shirt size instead that can better fit their needs
* We can continue to improve upon the t-shirt size recommendations to make our customers successful

### Non-Goals

* N/A

### Current workflow vs Proposed

Currently, a consumer like Jaeger or Cluster Logging is responsible for the following for their ES cluster:

1. Creating and updating their elasticsearch CR(s) so that EO can manage their cluster
    * This includes breaking up nodes by roles
    * Specifying (usually pass through) of node requirements

With this proposal the workflow would be:

1. Create their elasticsearch CR specifying a particular sized cluster (based on t-shirt sizes)

Current specification:

```yaml
apiVersion: "logging.openshift.io/v1"
kind: "Elasticsearch"
metadata:
  name: "elasticsearch"
spec:
  managementState: "Managed"
  nodeSpec:
    resources:
      limits:
        memory: 1Gi
      requests:
        cpu: 100m
        memory: 1Gi
  nodes:
  - nodeCount: 1
    roles:
    - client
    - data
    - master
    storage:
      storageClassName: gp2
      size: 10Gi
  redundancyPolicy: ZeroRedundancy
  indexManagement:
    policies:
    - name: infra-policy
      pollInterval: 1m
      phases:
        hot:
          actions:
            rollover:
              maxAge:   2m
        delete:
          minAge: 5m
    mappings:
    - name:  infra
      policyRef: infra-policy
      aliases:
      - infra
      - logs.infra
```

Proposed:

```yaml
apiVersion: "logging.openshift.io/v1"
kind: "Elasticsearch"
metadata:
  name: "elasticsearch"
spec:
  storage:
    storageClassName: gp2
    size: 10Gi
  size: 0x.medium
  indexManagement:
    infraPolicies:
    - phases:
      hot:
        actions:
          rollover:
            maxAge:   2m
      delete:
        minAge: 5m
    appPolicies:
    - phases:
      hot:
        actions:
          rollover:
            maxAge: 2m
      delete:
        minAge: 5m
```

## Proposal

Currently the Elasticsearch CR is overly complicated and allows for too much granular control over the data nodes, which places the expectation
of expertise on the user rather than containing it to the operator (which is where it should reside). This proposal seeks to outline a v2 to provide
a more simple means of interacting with the Elasticsearch Operator to ask for a managed Elasticsearch cluster that can be better
configured for performance.

### User Stories

#### As an user of EO, I want to have to provide minimal information for my cluster to be created for me

As an user I want to just be able to provide information such as a t-shirt size (which is outlined further for me) and my storage information
and have an elasticsearch cluster spun up for me so that I do not need to be an expert and configure my node roles and node resources.

### Implementation Details

#### Assumptions

* Currently the EO CRD version is V1 and this would be a V2. We would assume that EO is able to react for both a V1 and V2 and if the user creates a V2
  elasticsearch CR with the same name as an existing V1, the operator would migrate the V1 backing storage naming to be used for V2 (read: it would
  rename the PVCs and rebind the backing PV so that it matches the updated naming convention).

### Risks and Mitigations

## Not Breaking Other Operators who Depend on EO

As part of adding another version of the CRD that EO will respond to, we need to ensure that we can continue to support those that remain on V1 so that
they are able to migrate their usage to V2 in a timely manner that suits them (typically we give 1-2 releases since announcement before deprecating).

## Design Details

### PVC current and proposed

Current storage PVCs + backing:
```bash
$ oc get pvc
NAME                                        STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
elasticsearch-elasticsearch-cd-w5afmzhp-1   Bound    pvc-b7c88b5e-0513-4912-8d13-0d4b81e20fe4   10Gi       RWO            gp2            13m
elasticsearch-elasticsearch-cd-w5afmzhp-2   Bound    pvc-f3193fc1-da91-4267-b819-3b7b0af76cd6   10Gi       RWO            gp2            13m
elasticsearch-elasticsearch-cd-w5afmzhp-3   Bound    pvc-2936caa5-9311-4548-b3ce-7a2ec5c439ea   10Gi       RWO            gp2            13m
elasticsearch-elasticsearch-m-xpk5ybty      Bound    pvc-c90afaab-b4d4-47ff-8dd6-15f411ebe999   10Gi       RWO            gp2            13m
```

Proposed storage PVCs + backing:
```bash
$ oc get pvc
NAME                      STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
elasticsearch-0           Bound    pvc-b7c88b5e-0513-4912-8d13-0d4b81e20fe4   10Gi       RWO            gp2            13m
elasticsearch-1           Bound    pvc-f3193fc1-da91-4267-b819-3b7b0af76cd6   10Gi       RWO            gp2            13m
elasticsearch-2           Bound    pvc-2936caa5-9311-4548-b3ce-7a2ec5c439ea   10Gi       RWO            gp2            13m
elasticsearch-master      Bound    pvc-c90afaab-b4d4-47ff-8dd6-15f411ebe999   10Gi       RWO            gp2            13m
```

### T-Shirt sizing

To take inspiration from EC2 node sizing and the Loki Operator the Elasticsearch Operator would provide a more simple interface to customers
in the form of t-shirt sizes that would encapsulate replication policies, range of node count, resources provided per node.

This would need to be further fleshed out in a spike to determine what a Medium (the 90% use case) would be sized at, however some example sizes
could be:

* 0x.Small - A small (e.g. Dev test cluster) with zero redundancy. This would likely only be able to support a small amount of retention.
  * This could be comprised of just 1 master node and 1-3 data+ingest nodes with 1:1 separate coordinating nodes (matching data nodes)
* 1x.Medium - A cluster that should be able to cover 90% of our customer use cases and provide single redundancy
  * 3 masters, 3-5 data+ingest nodes and 1:1 coordinating nodes
* 1x.Large - For very large customer clusters with single redundancy
  * we would not exceed 3 masters but we may require more data and coordinating nodes as well as more resources per node

It is important to note that nodes use their resources differently depending on the role they play, but this makes it so that we can allocate more
or less memory to nodes within our t-shirt sizings.

#### Node memory usage

There is much more details but at a high level the memory requirements for a node is based on what role it plays in the cluster:

* Master nodes: Master nodes need to keep in memory every shard and which node it lives on
  * it doesn't need to know the contents of the shard, just its ID
  * the more shards we will have on the cluster (increased from index sharding and retention) the more memory they will need

* Data nodes: If there are separate coordinating nodes the data node is responsible for returning the shard(s) that it holds when responding to a query.
  It also will "map" (think: reference not copy) in memory its local disk cache so that it can quickly read/write. When we state that ES will
  use up half of its memory for the JVM the other half is used for this. The less the JVM takes up the more it can map.
  * The smaller a shard is, the less heap it can take up in responding to a query.
  * If the data node is not coordinating, it does not need to compile the results of a query to then later return. This helps cut down on its heap usage.
  * Elastic.co recommends also making data nodes do ingestion, so if there is an ingestion plugin that is used, it *may* require more memory

* Coodinating nodes: These are the workers, they need enough memory so that they can build a query response and return it to the caller.
  * Smaller shards and better formed queries along with index structure can help these use less memory.
  * These will likely require the most memory of any other node though
  * We match these 1:1 (at least) with a data node for communication efficiency (they would know more about shards that are on its data node)

### Test Plan

#### Unit Testing

* We will need to add some more unit tests to confirm that we can generate objects with the same names when using V1 and new names when using V2

#### Integration and E2E tests

* A cluster can still be created using a V1 CR
* A cluster can also be created using a V2 CR
* A cluster that was created using V1 can be upgraded to V2 by creating the V2 with the same name (there is no possibility to downgrade)

### Graduation Criteria

#### Dev Preview -> Tech Preview

* Operator is able to create clusters correctly and can scale them so that they are performant

#### Tech Preview -> GA

* Operator has shown consistent and reliable management of clusters that are highly performant
* Operator is able to migrate V1 CRs to V2 CRs without data loss

#### Removing a deprecated feature

* We would seek to remove V1 roughly 2 releases after V2 is GA
  * one release to account for information about V2 and announcing deprecation of V1
  * one more release after that before deprecating

### Upgrade / Downgrade Strategy

* Only upgrade will be allowed from V1 to V2

* Intended plan for upgrading would be:
  * If there exists a V1 CR named "elasticsearch" in the namespace "openshift-logging"
    and then a V2 CR named "elasticsearch" in the namespace "openshift-logging" is created then
    the operator would ignore the V1 one and migrate the objects from V1 to V2 (objects primarily being currently used storage)

  * The main concern with doing a straight migration (via transformation hook) is the need to try to extrapolate a desired t-shirt
    size based on the current configurations which may be very difficult to get correct. It may also be that the customer's current
    configuration does not correctly meet their demands so doing a migration could leave them with similar issues (mainly if their
    storage capacity isn't high enough or if their cluster is under resourced)

### Version Skew Strategy

* N/A

## Implementation History

| release|Description|
|---|---|
| 5.4 | TP |
| 5.5 | GA |

## Drawbacks

* This increases the complexity within the EO as it will need to internally manage the roles and resource requirements for nodes, however
the benefit this brings should not be overlooked and will also help the operator to not need to track node UUIDs or more complex structures
internally.

## Alternatives

* Continue to only use the current V1 API

## References

* https://www.elastic.co/guide/en/elasticsearch/reference/6.8/scalability.html
* https://www.elastic.co/guide/en/elasticsearch/reference/6.8/general-recommendations.html
* https://www.elastic.co/guide/en/elasticsearch/reference/6.8/modules-node.html