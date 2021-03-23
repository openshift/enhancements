---
title: monitoring-topology-mode
authors:
  - "@lilic"
  - "@simonpasquier"
reviewers:
  - "@openshift/openshift-team-monitoring"
  - "@dhellmann"
  - "@romfreiman"
  - "@wking"
approvers:
  - "@openshift/openshift-team-monitoring"
  - "@bparees"
  - "@dhellmann"
creation-date: 2021-03-01
last-updated: 2021-03-01
status: implementable
see-also:
  - "/enhancements/single-node/cluster-high-availability-mode-api.md"
  - "/enhancements/single-node/production-deployment-approach.md"
---

# Monitoring Stack with Single Replica Topology Mode


## Release Signoff Checklist

- [x] Enhancement is `implementable`
- [X] Design details are appropriately documented from clear requirements
- [X] Test plan is defined
- [ ] Operational readiness criteria is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

The Cluster Monitoring Operator (CMO) needs to respect the
`infrastructureTopology` field newly added to the
`infrastructures.config.openshift.io` CRD. Depending on the value, it should
configure the monitoring operands for highly-available operations or not.

## Motivation

As described in the [Cluster High-availability Mode API](https://github.com/openshift/enhancements/blob/master/enhancements/single-node/cluster-high-availability-mode-api.md)
enhancement and implemened in [OpenShift API](https://github.com/openshift/api/pull/827),
OpenShift 4.8 introduces 2 new status fields in the Infrastructure API that
informs operators about the cluster deployment topology.

Depending on whether an operand is deployed on the control nodes or not, the
operator should look at either `controlPlaneTopology` and
`infrastructureTopology`. If the field value is `HighlyAvailable`, the operator
should configure the operand for high-availability operation. If the field value is
`SingleReplica`, it should *not* configure the operand for high-avaibability
operation.

### Goals

- Deploy the monitoring components with one replica for non-HA configuration.
- Deploy the user-workload components with one replica for non-HA configuration.
- Ensure that CMO passes the single-node tests from openshift/origin.

### Non-Goals

- Implement additional enhancements to reduce CPU and memory footprints.
- Allow cluster admins to control the number of replicas per operand.
- Address CodeReady Containers requirements (though it might help).
- Add tests for `SingleReplica` to the existing end-to-end test suite in the CMO repository.

## Proposal

CMO watches the `infrastructures.config.openshift.io/cluster` resource for
updates and triggers a reconcialition whenever it detects a change. In the
reconcialiation loop, CMO reads the value of the `infrastructureTopology`
status field. If the value is equal to `SingleReplica`, CMO adjusts the number
of replicas to 1 for the following resources:

* openshift-monitoring/prometheus-k8s (StatefulSet)
* openshift-monitoring/alertmanager-main (StatefulSet)
* openshift-monitoring/thanos-querier (Deployment)
* openshift-monitoring/prometheus-adapter (Deployment)
* openshift-user-workload-monitoring/prometheus-user-workload (StatefulSet)
* openshift-user-workload-monitoring/thanos-ruler (StatefulSet)

Otherwise CMO deploys the operands with their default number of replicas to
ensure high-availability:

* openshift-monitoring/prometheus-k8s (StatefulSet): 2 replicas
* openshift-monitoring/alertmanager-main (StatefulSet): 3 replicas
* openshift-monitoring/thanos-querier (Deployment): 2 replicas
* openshift-monitoring/prometheus-adapter (Deployment): 2 replicas
* openshift-user-workload-monitoring/prometheus-user-workload (StatefulSet): 2 replicas
* openshift-user-workload-monitoring/thanos-ruler (StatefulSet): 2 replicas

By default, CMO deploys no operand on control nodes thus it should not look at
the `controlPlaneTopology` field.

### Risks and Mitigations

In HA mode, the Prometheus instances are spread on different nodes using
soft anti-affinity and they work completely independently one from another (there's no
data replication between them). Services consuming the metrics (like the
OpenShift console or Grafana) use the Thanos querier API which aggregates and
deduplicates data from both Prometheus instances. This setup provides
redundancy in term of metrics scraping, rule evaluations and querying as long
as one of the Prometheus instances is still up.

The Thanos ruler instances of user workload monitoring work the same as
Prometheus except they only deal with rule evaluations.

The 3 Alertmanager instances receive alerts from both Prometheus instances and
as long as one instance is up and running, the notifications should flow to the
receivers. Alertmanager instances also replicate silences and notification logs
between themselves to prevent duplicate notifications.

The other monitoring components running with multiple replicas in HA mode
(thanos-querier and prometheus-adapter) are stateless. Running multiple
replicas only ensures that their services remain available as long as 1
instance is up.

By definition, the monitoring stack wouldn't be highly-available in the
`SingleReplica` mode. One recommendation would be that cluster admins configure
Prometheus and Alertmanager with persistent storage to ensure that existing
metrics, silences and notification logs are retained upon pod recreation.

## Design Details

### Test Plan

We will setup a CI job that runs the `extended` end-to-end test suite from
[OpenShift Origin](https://github.com/openshift/origin) in a single-node
deployment mode. The test suite has already been extended to verify that
operators honor the `SingleReplica` value (see
[openshift/origin#25812](https://github.com/openshift/origin/pull/25812) and
[openshift/origin/pull/25885](https://github.com/openshift/origin/pull/25885)).

As soon as the implementation in CMO is merged, we will adjust the single-node
topology
[tests](https://github.com/openshift/origin/blob/master/test/extended/single_node/topology.go)
to remove the exception in place for the monitoring components.

Cluster admins can use the `openshift-monitoring/cluster-monitoring-operator`
configmap to tune the configuration of the operator and its operands (
user-workload monitoring enablement, persistent storage, resource
requests/limits, ...). We have [end-to-end tests](https://github.com/openshift/cluster-monitoring-operator/tree/master/test/e2e)
in the cluster-monitoring-operator verifying that these configuration
parameters work as expected. Because the CMO configuration doesn't allow to
change the replica count and it doesn't expose the topology mode either, we
don't plan to add any test specific to the single-node mode to the CMO test
suite.

### Graduation Criteria

N/A

### Upgrade / Downgrade Strategy

As stated in the [Single-node Production Deployment Approach](https://github.com/openshift/enhancements/blob/master/enhancements/single-node/production-deployment-approach.md)
proposal, in-place upgrades are a non-goal right now.

### Version Skew Strategy

If CMO reads a value from the `infrastructures.config.openshift.io/cluster`
resource that is neither equal to `HighAvailable` nor `SingleReplica`, it
considers that the topology is `HighAvailable`.

If CMO gets any error while reading the resource:
* If it has never been able to read the resource/see a topology config (due to
  error or missing value) it defaults to `HighAvailable`.
* If it has previously determined the topology, but now can't read the topology
  (i.e. during a resync/relist), it remains on whatever topology it last was
  able to determine.

## Implementation History

## Drawbacks

N/A

## Alternatives

* Do nothing. CMO would deploy multiple replicas for Prometheus Alertmanager,
  Thanos querier, Thanos ruler and prometheus-adapter. As a result, the
  monitoring stack would use more resources than is actually needed.
* Don't deploy CMO when `SingleReplica` is defined. It would be hard to
  achieve since the monitoring stack is a core component of OpenShift and other
  OpenShift components rely on the presence of the monitoring CRDs.
* Not deploying the monitoring operands when `SingleReplica` is defined. It
  isn't easier to implement than adjusting the number of replicas.
