# Conditions

[The ClusterVersion object](../dev/clusterversion.md) sets `conditions` describing the state of the cluster-version operator (CVO).
This document describes those conditions and, where appropriate, suggests possible mitigations.

## Failing

When `Failing` is True, the CVO is failing to reconcile the cluster with the desired release image.
In all cases, the impact on the cluster will be that dependent nodes in [the manifest graph](reconciliation.md#manifest-graph) may not be [reconciled](reconciliation.md#reconciling-the-graph).
Note that the graph [may be flattened](reconciliation.md#manifest-graph), in which case there are no dependent nodes.

Most reconciliation errors will result in `Failing=True`, although [`ClusterOperatorNotAvailable`](#clusteroperatornotavailable) has special handling.

### NoDesiredImage

The CVO has not been given a release image to reconcile.

If this happens it is a CVO coding error, because clearing [`desiredUpdate`][api-desired-update] should return you to the current CVO's release image.

### ClusterOperatorNotAvailable

`ClusterOperatorNotAvailable` (or the consolidated `ClusterOperatorsNotAvailable`) is set when the CVO fails to retrieve the ClusterOperator from the cluster or when the retrieved ClusterOperator does not satisfy [the reconciliation conditions](reconciliation.md#clusteroperator).

Unlike most manifest-reconciliation failures, this error does not immediately result in `Failing=True`.
Under some conditions during installs and updates, the CVO will treat this condition as a `Progressing=True` condition and give the operator up to fourty minutes to level before reporting `Failing=True`.

## RetrievedUpdates

When `RetrievedUpdates` is `True`, the CVO is succesfully retrieving updates, which is good.
When `RetrievedUpdates` is `False`, `reason` will be set to explain why, as discussed in the following subsections.
In all cases, the impact is that the cluster will not be able to retrieve recommended updates, so cluster admins will need to monitor for available updates on their own or risk falling behind on security or other bugfixes.
When CVO is unable to retrieve recommended updates the CannotRetrieveUpdates alert will fire containing the reason. This alert will not fire when the reason updates cannot be retrieved is NoChannel.

### NoUpstream

No `upstream` server has been set to retrieve updates.

Fix by setting `spec.upstream` in ClusterVersion to point to a [Cincinnati][] server, for example https://api.openshift.com/api/upgrades_info/v1/graph .

### InvalidURI

The configured `upstream` URI is not valid.

Fix by setting `spec.upstream` in ClusterVersion to point to a valid [Cincinnati][] URI, for example https://api.openshift.com/api/upgrades_info/v1/graph .

### InvalidID

The configured `clusterID` is not a valid UUID.

Fix by setting `spec.clusterID` to a valid UUID.
The UUID should be unique to a given cluster, because it is the default value used for reporting Telemetry and Insights.
It may also be used by the CVO when making Cincinnati requests, so that Cincinnati can return update recommentations tailored to the specific cluster.

### NoArchitecture

The set of architectures has not been configured.

If this happens it is a CVO coding error.
There is no mitigation short of updating to a new release image with a fixed CVO.

#### Impact

The cluster will not be able to retrieve recommended updates, so cluster admins will need to monitor for available updates on their own or risk falling behind on security or other bugfixes.

If this happens it is a CVO coding error.
There is no mitigation short of updating to a new release image with a fixed CVO.

### NoCurrentVersion

The cluster version does not have a semantic version assigned and cannot calculate valid upgrades.

If this happens it is a release-image creation error.
There is no mitigation short of updating to a new release image with fixed metadata.

### NoChannel

The update `channel` has not been configured.

Fix by setting `channel` to [a valid value][channels], e.g. `stable-4.3`.

### InvalidCurrentVersion

The current cluster version is not a valid semantic version and cannot be used to calculate upgrades.

If this happens it is a release-image creation error.
There is no mitigation short of updating to a new release image with fixed metadata.

### InvalidRequest

The CVO was unable to construct a valid Cincinnati request.

If this happens it is a CVO coding error.
There is no mitigation short of updating to a new release image with a fixed CVO.

### RemoteFailed

The CVO was unable to connect to the configured `upstream`.

This could be caused by a misconfigured `upstream` URI.
It could also be caused by networking/connectivity issues (e.g. firewalls, air gaps, hardware failures, etc.) between the CVO and Cincinnati server.
It could also be caused by a crashed or otherwise broken Cincinnati server.

### ResponseFailed

The Cincinnati server returned a non-200 response or the connection failed before the CVO read the full response body.

This could be the CVO failing to construct a valid request.
It could also be caused by networking/connectivity issues (e.g. hardware failures, network partitions, etc.).
It could also be an overloaded or otherwise failing Cincinnati server.

### ResponseInvalid

The Cincinnati server returned a response that was not valid JSON or is otherwise corrupted.

This could be caused by a buggy Cincinnati server.
It could also be caused by response corruption, e.g. if the configured `upstream` was in the clear over HTTP or via a man-in-the-middle HTTPS proxy, and an intervening component altered the response in flight.

### VersionNotFound

The currently reconciling cluster version was not found in the configured `channel`.

This usually means that the configured `channel` is known to Cincinnati, but the version the cluster is currently applying is not found in that channel's graph.
You have some options to fix:

* Set `channel` to [a valid value][channels].
    For example, `stable-4.7`.
* Clear `channel` if you do not want the operator polling the configured `upstream` for recommended updates.
    For example, if your operator is unable to reach any upstream update service, or if you updated to a release that is not in any channel.
* Update back to a release that occurs in a channel, although you are on your own to determine a safe update path.

### Unknown

If this happens it is a CVO coding error.
There is no mitigation short of updating to a new release image with a fixed CVO.

[api-desired-update]: https://github.com/openshift/api/blob/34f54f12813aaed8822bb5bc56e97cbbfa92171d/config/v1/types_cluster_version.go#L40-L54
[channels]: https://docs.openshift.com/container-platform/4.7/updating/updating-cluster-between-minor.html#understanding-upgrade-channels_updating-cluster-between-minor
[Cincinnati]: https://github.com/openshift/cincinnati/blob/master/docs/design/openshift.md
