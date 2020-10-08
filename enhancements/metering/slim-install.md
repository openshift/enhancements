---
title: slim-install
authors:
  - @timflannagan1
reviewers:
  - @bentito
  - @bparees
  - @cfergeau
  - @russellb
approvers:
  - @operator-framework/team-metering
  - @bparees
creation-date: 2020-10-06
last-updated: 2020-10-20
status: implementable
see-also:
replaces:
superseded-by:
---

# Slim Installation

## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [x] Design details are appropriately documented from clear requirements
- [x] Test plan is defined
- [x] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

Due to the heavy footprint of the current Hadoop-based Metering Operator (Metering) stack, it's difficult to install in smaller clusters, like a demo or developer environment, which is affecting potential adoption and opens the door to replicating existing Metering functionality, with an in-house solution. This proposal outlines a path towards a new, slimmed down version (v2) of the Metering stack, both in terms of reducing the complexity and overall resource footprint, and is centered around replicating the properties that the Hadoop, Hive, and Presto components offer, with Postgresql acting as both the underlying storage and compute engine. This will allow Metering to set more reasonable defaults and reduces the barrier of entry when attempting to install Metering on smaller or medium-sized environments as it requires little-to-none user configuration.

In addition to replacing the existing storage and compute layers, Metering will now utilize recording rules for its out-of-the-box Prometheus metrics it tracks, imports, and stores long term in Postgresql. Recording rules are a way to precompute frequent promQL queries, spreading the compute overhead over time, which increases Metering performance at query-time and reduces the query-time load on Prometheus. In the case a user wants to extend the reporting capabilities of Metering to track and store additional Prometheus metrics, those promQL queries will need to be evaluated on-demand at query-time instead of dynamically creating new recording rules.

## Motivation

- The current, Hadoop-based Metering architecture is designed to horizontally scale out to handle querying and managing massive amounts of data, and in most cases is not needed for smaller and medium-sized clusters.
- In most environments, Metering is only querying and storing a small subset of Prometheus metrics and the total size of that datastore typically doesn't warrant the usage of a distributed query engine, like Presto.
- In the case a user wishes to install Metering in a disconnected, air-gapped, or simply on an environment that does not have access to cloud storage, Metering requires the usage of a StorageClass or PersistentVolume with ReadWriteMany (RWX) access modes.
- At a minimum, Metering requires the user to define a storage configuration in the `MeteringConfig` custom resource and that leaves the door open to potential misconfigurations or difficulties setting up the pre-requisites for Metering usage.
- Difficult maintaining the components (and their images) that make up the current Metering stack.

### Goals

- Reduce the complexity of the Metering stack.
- Reduce the overall CPU and memory footprint of the Metering stack.
- Removal of the HDFS, Hive and Presto components from the metering-ansible-operator and reporting-operator codebases.
- Removal of the `HiveTable` and `PrestoTable` CRDs from the metering-ansible-operator bundle packaging.
- Outline a potential data migration path from the current v1 stack to the new and proposed v2 stack.

### Non-Goals

- Initial support for cloud object storage.
- Maintaining support for AWS billing integration.
- Metering is delivered as a CVO-based Operator.

## Proposal

Currently, and by default, Metering periodically imports and stores a small subset of the available Prometheus metrics in Hive long term, using the Prometheus query_range API. Historically, Presto did not have the ability to directly query Prometheus, but it does have the ability to integrate with Hive datasets. This results in the following workaround, where tables are needed to be first created in Hive, and then newer metrics would be inserted by Presto into existing tables in Hive using the `INSERT INTO ... VALUES(...)` operation. It's also worth noting that type of operation is particularly expensive for Presto in comparison to Postgres, which is designed as a more transactional-based system (OLTP).

Due to the nature of periodic importing, most of the components are sitting idle, especially in the case of Presto which is still utilizing resources despite no queries being processed or evaluated.

With the introduction of Postgres, and the removal of the Hadoop, Hive, and Presto components, we can continue to utilize the existing Prometheus metrics importer implementation, adapting to the native Postgres data types, and the result is a more streamlined approach to data flow and an overall less complex stack.

This implementation would allow Metering to continue using the core, existing CustomResourceDefinitions (CRDs) already exposed in previous releases:

- `ReportDataSource`: exposes an existing table in Postgresql that can be used for reporting needs.
- `ReportQuery`: defines a SQL query or view that references a `ReportDataSource` table, or a table created by a `Report` object.
- `Report`: evaluates a templated SQL query defined in an existing `ReportQuery` custom resource and stores those query results in a Hive (now Postgres) table.

At a high-level, the following must be done:

- Bump the metering.openshift.io API group versioning to v2alpha1.
- Create an additional Operator bundle for the v2 version of Metering.
- Migrate to using Prometheus recording rules for out-of-the-box `ReportDataSource` promQL queries.
- Create a `PostgresTable` CRD, which is an abstraction that represents an individual table in Postgres.
- When processing `ReportDataSource` and `Report` resources, create the intermediate `PostgresTable` resource.
- The `PostgresTable` resource is responsible for creating a table or view in Postges based on the provided specification.
- Update the existing `ReportDataSource` promQL queries to reference the recording rules defined in the Metering rule group.
- Update the existing `ReportQuery` SQL queries and views to index Prometheus labels stored as `jsonb` objects in Postgres tables.

### Implementation Details/Notes/Constraints [optional]

#### Overview of the Proposed PostgresTable Object

In order to conform with native Postgres data types, any Prometheus labels returned by the query_range API endpoint will need to be represented as the `jsonb` type. This could result in the following `PostgresTable` custom resource instantiation, where metric data is partitioned by datetime and the resultant table column name and types are explicitly defined:

```yaml
apiVersion: metering.openshift.io/v2alpha1
kind: PostgresTable
metadata:
  name: reportdatasource-metering-node-allocatable-cpu-cores
spec:
  columns:
  - name: amount
    type: float8
  - name: timestamp
    type: timestamptz
  - name: timeprecision
    type: float8
  - name: labels
    type: jsonb
  databaseName: metering
  tableName: datasource_metering_node_allocatable_cpu_cores
  managePartitions: false
  partitionedBy:
  - name: dt
    type: string
```

Using a Postgresql go-client, the following implementation could be used to insert Prometheus metrics into an existing Postgresql table:

```golang
_, err = conn.Exec(context.Background(), fmt.Sprintf("INSERT INTO %s VALUES($1, $2, $3, ($4)::jsonb)", tableName), metric.Amount, metric.Timestamp, metric.StepSize, labels)
if err != nil {
  return err
}
```

When interacting with that metric table data in Postgres, you can [index into the `labels` jsonb type](https://www.postgresql.org/docs/12/datatype-json.html#JSON-INDEXING) by running the following query:

```sql
SELECT
  amount,
  timestamp,
  timeprecision,
  labels->'pod' as pod,
  labels->'namespace' as namespace,
  labels->'node' as node
FROM metering_pod_request_cpu_cores;
```

#### Updating the Default ReportDataSource and ReportQuery Objects

The default, out-of-the-box `ReportDataSource` custom resources promQL queries will need to be updated in order to reference the recording rules that will be defined in the metering rule group. In order to do this, the metering-ansible-operator bundle will need to translate the existing set of promQL queries as a list of recording rules defined in the specification of a `PrometheusRule` custom resource. Support for [Operator bundles can include Prometheus-related custom resource objects](https://github.com/operator-framework/operator-lifecycle-manager/pull/1253) was added in the 0.14.1 OLM release.

Internally, the reporting-operator needs to remove all logic for creating/reconciling/watching `HiveTable` and `PrestoTable` custom resources and instead always create a `PostgresTable` custom resource when reconciling `ReportDataSource` and `Report` objects that don't have a database table created as of yet. On a similar note, the proper owner references will need to be injected for all `PostgresTable` custom resources to establish a hierarchy. In the case a user deletes a `Report` (or `ReportDataSource`) object, the Kubernetes garbage collector is responsible for cleaning up the correct `PostgresTable` object, and thus requires no further manual intervention from the user.

#### Extending the `StorageLocation` CRD to Support Postgresql

A `StorageLocation` custom resource is responsible for abstracting away details about the underlying Metering storage backend, exposing the minimal configuration required to show the end-user where Metering data has been persisted. Currently, this object only supports Hive databases and therefore needs to be able to replicate the same functionality we provide with Hive, but with Postgresql interfaces.

When the Metering slim installation has been enabled, the default `StorageLocation` custom resource that gets reconciled by the metering-ansible-operator should attempt to make this `CREATE DATABASE metering ...` database call using the Postgres go-client, and update it's status once that database has been created.

Note: Postgresql has no native support for creating a database if it doesn't already exist, so the following call will need to be made instead:

```SQL
SELECT 'CREATE DATABASE metering' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'metering')
```

#### Outlining a Potential Migration Path

There's currently an undocumented PUSH API for the reporting-operator to statically inject Prometheus data that matches a specific JSON format. This API is largely used in Metering end-to-end testing scenarios and is likely, not robust or stable enough to rely on for a full data set migration from existing Metering installations. In theory, a migration effort is technically possible, but a more robust API implementation would be needed.

Also worth noting that making changes to this API would likely require backporting those changes to previously released versions of Metering.

### Risks and Mitigations

#### TLS and Authentication

**Risk**: Need to support TLS and authentication for communication between the reporting-operator and the Postgres database instance.

**Mitigation**: Postgres has native support for mutual SSL/TLS and client authentication. The reporting-operator, which is deployed with an Openshift auth-proxy sidecar container, can be configured to trust the Postgres instance. The metering-ansible-operator will be responsible for generating these client/server/root certificates for the components.

## Design Details

### Test Plan

- Provide a mechanism for statically injecting cluster metric data into `ReportDataSource` database tables to ensure that the refactored list of `ReportQuery` queries match the expected `Report` output results. Those output results should also match what we expect in the non-slim-install version of Metering.
- Ensure that the refactored list of `ReportDataSource` resources result in at least one row of metric data being dynamically imported from Prometheus into the corresponding table in Postges.

### Graduation Criteria

#### Dev Preview -> Tech Preview

Only tech preview has been planned for this proposal.

#### Tech Preview -> GA

- Provide and document a data migration path for existing Metering installations to this new slimmed down architecture.
- The Metering API versioning will be bumped from `v2alpha1` to `v2` for the CRDs.
- Removal of support for the v1 Metering bundle.
- Sufficient time for feedback.
- Sufficient testing in the following areas: upgrade, downgrade, scale and e2e.

### Upgrade / Downgrade Strategy

Metering will continue to be packaged as an OLM-based Operator, so OLM will be responsible for orchestrating upgrades and downgrades between v2 Metering versions.

The current state of this proposal upgrades between the v1 and v2 versions of the Operator won't be supported, and instead a new installation of the v2 Metering Operator will be required.

### Version Skew Strategy

- N/A

## Implementation History

- N/A

## Drawbacks

- Continue to utilize the existing codebase for periodically importing metrics from Prometheus still relies on the averaged or estimated vector values that get returned by the Prometheus query_range API endpoint.
- Deprecating the Prometheus metric importing process and working towards using the Prometheus remote_write or remote_read APIs to interact with raw metric data will require an additional migration that will be more technically difficult.
- Extending Metering to track additional, user-provided promQL queries that are defined in the ReportDataSource custom resources will need to be done on-demand. Dynamically creating recording rules for user-defined `ReportDataSource` resources is listed as an alernative.
- The storage and compute layers are now combined. Postgresql will now play the role as both an OLTP and OLAP typed system.
- Presto, using the Hive connector, had support for accessing data stored in the various cloud storage platforms (e.g. s3, GCS, Azure Blog Storage, etc.).

## Alternatives

### Dynamically Create Recording Rules

Build off the ideas outlined in this proposal, but dynamically create recording rules in the metering rule group based on a promQL queries defined in a non-default `ReportDataSource` custom resource:

```yaml
apiVersion: metering.openshift.io/v2alpha1
kind: ReportDataSource
metadata:
  name: metering-kube-node-labels
spec:
  prometheusMetricsImporter:
    query: |
      kube_node_labels
```

Here, we can process the promQL query defined in the `.spec.prometheusMetricsImporter.query` field and translate that into a recording rule:

```yaml
apiVersion: monitoring.coreos.com/v1
kind: PrometheusRule
...
spec:
  groups:
  - name: metering.rules
    rules:
    - record: metering:{{ .metadata.name }}
      expr: |
        {{ .spec.prometheusMetricsImporter.query }}
```

As detailed above, we could patch the Metering `PrometheusRule` custom resource that is packaged into the metering-ansible-operator bundle, inject the user-defined promQL query as a rule definition and take the value of the `metadata.name`, sanitizing it to ensure that it's valid for the `groups[*].rules[*].record` property.

Notable cons of this approach is that the computational overhead now falls on Prometheus and that overhead would be spread over time, but at the cost of new times series being generated. Additional concerns may be users defining expensive queries that require Prometheus to load many metric samples in memory at once.

### Continue Reconciling the Hive and Presto Components

This alternative is centered around building off of what is being proposed in this enhancement and providing an avenue for Metering to reconcile Hive and Presto components to scale out to larger (or busier) cluster environments. With this approach, all transactional type operations can be done in Postgres (i.e. the creation of tables and the insertion of new metric values) but all analytical querying will be done using Presto. This implementation would require deploying Presto with the [Postgres connector enabled](https://prestosql.io/docs/current/connector/postgresql.html) and updating the Hive metastore configuration to always use Postgres as the underlying database.

While this approach would provide an a potential avenue for scaling out this new, proposed Metering architecture, it would also increase the supportability matrix and the complexity of the stack, in addition to having to continue maintaining those additional components.

### Integrate the Presto/Prometheus Connector

This distinct alternative is focused around extending the Hadoop-based Metering stack and integrating the Presto/Prometheus connector. This connector, which was [introduced upstream in the 334 release](https://github.com/prestosql/presto/pull/2321), allows Presto to directly query Prometheus metric data. Now, instead of periodically querying Prometheus for metric query results, the connector will stream metric samples from Prometheus, to database tables stored in Presto.

This approach would allow Metering to interact with more raw metric data, in addition to a more streamlined data flow. The process of integrating this connector will require a full overhaul of the existing promQL and SQL queries/views defined throughout the default set of `ReportDataSource` and `ReportQuery` resources, in addition to rethinking how storage would need to be handled.

In theory, Metering can utilize existing CRD functionality to periodically store daily partitions of Prometheus data in Presto schemas. One avenue that could be explored is the usage of scheduled `Reports`, which Metering stores streamed Prometheus data in daily partitions in Presto. Alternatively, a `INSERT INTO ... SELECT * FROM ... WHERE timestamp > .status.lastTimestamp` type operation could be entertained and achieve similar results.

Once that initial integration has happened, it's possible to remove (i.e. stop reconciling) the Hive components (metastore and server). Presto, an in-memory query engine, still requires the usage of a metastore, but it's possible to have Presto use a local, file-based metastore and back up that path in the container with a PersistentVolumeClaim (PVC). With this path, the reporting-operator would always create tables in Presto instead of Hive, i.e. always favor creating `PrestoTable` resources, and the functionality provided by the Hive components would be obsolete.

A couple of things to call out with this approach:

- The Hive components, and especially Hive server are fairly idle in the current architecture due to the nature of periodically importing metric data.
- Removing the Hive components could reduce the installation complexity as the presence of hard-to-come-by storage access modes, like RWX, will no longer be needed.
- As aforementioned, Presto requires the usage of metastore, and favoring a local, file-based one instead of a more robust and dedicated metastore implementation could be problematic.
- The current implementation of AWS billing integration requires the usage of Hive server, namely to manage report partitions. Note: maintaining support for AWS billing is listed as out-of-scope for this proposal.
- Setting reasonable resource limits and configuration the JVM properly for Presto can be challenging and Presto is wired to use all resources that are available to it.
- Need to determine a support matrix between previous versions of the operator with this new integration.

Overall, this alternative stands out the most for providing a more streamlined in-cluster Metering solution, in addition, to providing ways to slim down that implementation.

### Integrate with the Prometheus remote_write API

Utilize the Prometheus remote_write/remote_read configuration and one of the ecosystem's exporters. This would allow Metering to swap out the Hive/Presto stack, and be able to handle more raw, accurate metric data than what the Prometheus query_range API can offer.

Exposing an upgrade and migration path will be difficult in this case, in addition to having the same problems with the integration of the Presto/Prometheus connector, where most/all of the existing promQL and SQL queries defined in the Metering custom resources will need to be reworked.

In addition to this, Metering would likely have to dynamically create recording rules with the `metering:<metric name>` prefix for any extended use case, as most third-party exporters require high memory usage in order to perform expensive operations, like unmarshaling Prometheus labels to a data type the database can consume.

At this time, the remote_write and remote_read APIs are unsupported features of the Openshift monitoring stack, and there's no generalistic way of abstracting those features under-the-hood such that a Red Hat Operator has the ability to consume them internally without requiring the configuration of the user-facing, cluster-monitoring-operator ConfigMap.
