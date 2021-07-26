# CVO Metrics

The Cluster Version Operator reports the following metrics:

The cluster version is reported as seconds since the epoch with labels for `version` and
`image`. The `type` label reports which value is reported:

* `current` - the version the operator is applying right now (the running CVO version) and the age of the payload
* `initial` - the initial version of the cluster, and value is the creation timestamp of the cluster version (cluster age)
* `cluster` - the current version of the cluster with `from_version` set to the initial version, and value is the creation timestamp of the cluster version (cluster age)
* `failure` - if the failure condition is set, reports the last transition time for the condition.
* `desired` - reported if different from current as the most recent timestamp on the cluster version
* `completed` - the time the most recent version was completely applied, or absent if not reached
* `updating` - if the operator is moving to a new version, the time the update started

The `from_version` label is set where appropriate and is the previous completed version for the provided `type`. Empty for
`initial`, and otherwise empty if there was no previous completed version (still installing).

```prometheus
# HELP cluster_version Reports the version of the cluster.
# TYPE cluster_version gauge
cluster_version{image="test/image:2",type="current",version="4.0.3",from_version="4.0.2"} 130000000
cluster_version{image="test/image:2",type="failure",version="4.0.3",from_version="4.0.2"} 132000400
cluster_version{image="test/image:4",type="desired",version="4.0.4",from_version="4.0.2"} 132000400
cluster_version{image="test/image:2",type="completed",version="4.0.3",from_version="4.0.2"} 132000100
cluster_version{image="test/image:1",type="initial",version="4.0.1",from_version=""} 131000000
cluster_version{image="test/image:2",type="cluster",version="4.0.3",from_version="4.0.1"} 131000000
cluster_version{image="test/image:3",type="updating",version="4.0.4",from_version="4.0.3"} 132000400
# HELP cluster_version_available_updates Report the count of available versions for an upstream and channel.
# TYPE cluster_version_available_updates gauge
cluster_version_available_updates{channel="fast",upstream="https://api.openshift.com/api/upgrades_info/v1/graph"} 0
```

Metrics about cluster operators:

```prometheus
# HELP cluster_operator_conditions Report the conditions for active cluster operators. 0 is False and 1 is True.
# TYPE cluster_operator_conditions gauge
cluster_operator_conditions{condition="Available",name="version",namespace="openshift-cluster-version",reason="Happy"} 1
cluster_operator_conditions{condition="Degraded",name="version",namespace="openshift-cluster-version",reason=""} 0
cluster_operator_conditions{condition="Progressing",name="version",namespace="openshift-cluster-version",reason=""} 0
cluster_operator_conditions{condition="RetrievedUpdates",name="version",namespace="openshift-cluster-version",reason=""} 0
# HELP cluster_operator_up Reports key highlights of the active cluster operators.
# TYPE cluster_operator_up gauge
cluster_operator_up{name="version",namespace="openshift-cluster-version",version="4.0.1"} 1
```

Metrics reported while applying the image:

```prometheus
# HELP cluster_version_payload Report the number of entries in the image.
# TYPE cluster_version_payload gauge
cluster_version_payload{type="applied",version="4.0.3"} 0
cluster_version_payload{type="pending",version="4.0.3"} 1
# HELP cluster_operator_payload_errors Report the number of errors encountered applying the image.
# TYPE cluster_operator_payload_errors gauge
cluster_operator_payload_errors{version="4.0.3"} 10
```

Metrics about the installation:

`cluster_installer` records information about the installation process.
The type is either "openshift-install", indicating that `openshift-install` was used to install the cluster (IPI) or "other", indicating that an unknown process installed the cluster (UPI).
When `openshift-install` creates a cluster, it will also report its version and invoker.
When an unknown process installed the cluster, the version and invoker reported will be that of the `openshift-install` invocation which created the manifests.
The version is helpful for determining exactly which builds are being used to install (e.g. were they official builds or had they been modified).
The invoker is "user" by default, but it may be overridden by a consuming tool (e.g. Hive, CI, Assisted Installer).

```prometheus
# TYPE cluster_installer gauge
cluster_installer{type="openshift-install",invoker="user",version="unreleased-master-1209-gfd08f44181f2111486749e2fb38399088f315cfb"} 1
cluster_installer{type="other",invoker="user",version="unreleased-master-1209-gfd08f44181f2111486749e2fb38399088f315cfb"} 1
```
