---
title: machine-api-usage-telemetry
authors:
  - "@elmiko"
reviewers:
  - "@enxebre"
  - "@michaelgugino"
  - "@joelspeed"
  - "@brancz"
  - "@lilic"
  - "@wking"
approvers:
  - "@enxebre"
  - "@michaelgugino"
  - "@smarterclayton"
creation-date: 2020-06-17
last-updated: 2020-07-10
status: implementable
see-also:
replaces:
superseded-by:
---

# Machine API Usage Telemetry

## Release Signoff Checklist

- [X] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Summary

This enhancement proposes utilizing alerts and metrics exposed through telemetry
to improve our understanding of how users are operating the Machine API components.
By reporting the amount of activity with the Machine API we can understand the basic
usage patterns that exist within the OpenShift fleet. Correlating this data with
alerting information about failures and remediations will provide opportunities
for prescriptive guidance and insight into infrastructure stability.

## Motivation

### Goals

* Inform about Machine API component (Machine API Operator, Cluster Autoscaler,
and Machine Health Check) usage across the OpenShift fleet.
* Expose Machine Health Check success and failure rates to provide data on
remediation effectiveness and general cluster health trends.
* Create alerts that detail specific failure modes and remediations that are
occurring within OpenShift to capture events related to Machine API function.

### Non-Goals

* Provide any information that would expose a user's identity or confidential
information.

## Proposal

### User Stories

#### Story 1

As an OpenShift developer I would like to improve my understanding of how users
are consuming the Machine API and its related components. By creating time
series telemetry about the number and type of deployed component resources
(for example MachineSets) I will learn about usage patterns and trends. Learning
more about how frequently these components are deployed and utilized will allow me to
identify areas where users might be having difficulties, make better decisions
about the future planning of work, and demonstrate to a wider audience the popularity
level of various features.

#### Story 2

As an OpenShift developer I would like to gain a deeper understanding of the
general health of clusters by examining the behavior of the Machine Health
Check. By analyzing the frequency of success and failures rates as well as
the operational modes of the Machine Health Check I will be able to build
a model of infrastructure health and potential predictive activity. This
information will allow me to gain insight into correlations between
unhealthy cluster remediations and infrastructure providers, give a window
into predictive possibilities for user outreach, and demonstrate the value of
automated remediation.

#### Story 3

As an OpenShift developer I would like to correlate Machine API resource usage
with alerting events to provide presciptive advice about failures conditions
and platform instability. By examining data about Machine API resource management,
failures, and remediations, and corelating that data with alerts and platform
information I will be able to better advise users about errors, and methods for
avoiding them. When combined with a wider fleet view this information will help
me to create advisories about provider instabilities and potential outtages.

### Implementation Details/Notes/Constraints

#### Specific Telemetry Inquiries

This section details some of the specific use cases that have been identified
as initial inquries we would like to gather and report. They have been
categorized into 3 epics: demonstration of value, engineering insights,
and proactive support. Demonstration of value speaks to the raw amount of
usage that is happening for any given component. Engineering insights speaks
to situations where we can learn about user experience and interactions to inform
future planning and guidance. Proactive support speaks to information that we
learn which will lead to better experiences for users by providing opportunity
for preventitive intervention and education.

**Total Remediation Events**

Epics: Demonstration of Value, Proactive Support

This metric will give us insight into how effectively the Machine Health
Check is being used. Seeing a sustained high number of events on a particular
cluster provides opportunities to deliver proactive support. Seeing a sudden
spike across all clusters might give us early warning to a cloud provider’s data
center outage, or a large network or similar event that is impacting many of our
users. Having a total number of remediation events can demonstrate the value
our platform is bringing to our users. It will show the operational burden
we’ve decreased on teams that use OpenShift.

For example, since many clusters operate at a relatively small scale, it’s
possible that losing an Availability Zone in AWS US-East-1 region would be
small enough event on their cluster to trigger the Machine Health Check. Letting
users know we’re seeing this across many clusters would assist in their own troubleshooting
efforts and save them time.

**Remediation is Disabled or Short Circuited**

Epics: Proactive Support

This metric will give us insight into when a particular cluster has too many
unhealthy nodes for the Machine Health Check to do its job. A user may be expecting
that the health check is running as normal, cleaning up broken hosts, when in
reality it has been silently disabled. Any sustained disabling of the Machine
Health Check should potentially trigger a proactive support engagement. Similar
to the Total Remediation Events, experiencing a high volume of these events
across several users or platforms provides opportunities to create alerts for
users of potential outage events.

**Number of Nodes Covered by the Machine Health Check**

Epics: Engineering Insights, Proactive Support

This metric will give us insight into user experiences operating the Machine Health
Check. For example, users might think they are utilizing the Machine Health
Check correctly only to find out that it has been misconfigured and not
performing its role. If users have a large cluster with the health check
enabled, but not covering any nodes, this might be an opportunity to reach out
and see if we can help with their configuration. This can also help us identify
areas where increased education will have value.

**Failed Node Cleanups**

Epics: Proactive Support

This metric will help us identify conditions where the Cluster Autoscaler is
silently cleaning up failied node creations. We should trigger an alert if the
autoscaler cleans up more than X hosts in Y time period. This alert would be
useful for users that might not realize they’re having these infrastructure problems.
This is another case where signals across multiple clusters might indicate
that a cloud or network area is having problems.

For example, our internal continuous integration infrastructure team had a
situation where two MachineSets had been configured for autoscaling.
One MachineSet would scale up without issue, then the other would not. While
this was happening, the Machines would fail, alarms would fire, and the autoscaler
would dispose of the nodes that failed to come online. These failed nodes were
being silently cleaned up by the autoscaler.

**Total Scale Events**

Epics: Demonstration of Value

This metric will help us to show the level of activity that the Cluster
Autoscaler is performing. Collecting data on the total number of scale up and
scale down events, possibly as two individual metrics, we could correlate with
instance pricing to demonstrate how much using the autoscaler saves our users.
This can also serve as a good proxy for how many users are actually using the
autoscaler in a meaningful way, not just who has turned it on.

**Unable to Scale Events**

Epics: Proactive Support

This metric will help us identify situations when the Cluster Autoscaler is
unable to perform scaling operations. To maximize the value of this data, we
will want to report categories based on the different types of failures.
Events related to min/max threshold on MachineSets probably don’t need
alerting, but the inability to scale for other reasons (eg, hitting max CPU threshold or
memory, but haven’t hit max node count) are useful to collect. These events can
show opportunities to inform users about resource limitations in their clusters.

**Machines with Old Delete Timestamp**

Epics: Proactive Support

This metric can show us stale resources that exist within clusters. By collecting
data for machines that are stuck in a deletion phase for prolonged periods of time
we can identify opportunities to inform users and create alerts around these
stale resources. When combined with a count of machines that contain old
deletion timestamps this can become a powerful alarm for users. A starting
threshold of 25% of machines with deletion timestamps older than 6 hours is
the suggested line for an alarm.

**MachineSets Usage**

Epics: Engineering Insights

This metric will give us information about how the clusters are being used,
how users are interacting with the various APIs, and where user experience
improvements can be targeted. By collecting data on the number and types of
Machines being utilized, and how many MachineSets are created and destroyed by
users we will learn how users operate the Machine API. This data can provide
a multitude of insights into API and platform usage patterns.

For example, seeing that users are creating many different Machines of varying
flavors through multiple MachineSets will give us insight into how users would
prefer to consume cloud resources. Conversely, seeing a high number of Machine
creations without corresponding MachineSets might indicate that the API is not
clearly understood, or that Machine resource sizing is less important to users.

**Under-utilized Cluster Autoscaler**

Epics: Engineering Insights, Proactive Support

This metric will help us identify when users have enabled the Cluster Autoscaler
but are not using it. By collecting data on the number of MachineAutoscalers
that have been deployed in a cluster correlated with the deployment of the
Cluster Autoscaler we can identify under-utilization cases. This data
shows us opportunties to inform users and increase education around usage of
the autoscaler.

#### Alerts, Metrics, and Telemetry

To fully accomplish the goals proposed in this enhancement several metrics
will need to be exposed through telemetry. These metrics are:

**These metrics exist today**

* MachineSet resource count
  * This metric can be acquired from the cluster telemetry via `cluster:usage:resources:sum{resource="machinesets.machine.openshift.io"}`.
* Cluster Autoscaler scale down count
  * `cluster_autoscaler_scaled_down_nodes_total` - This metric has one label `reason`,
    with a cardinality of 3.
* Cluster Autoscaler scale up count
  * `cluster_autoscaler_scaled_up_nodes_total` - This metric has no labels.

**These metrics will need to be created**

* Machine resource count
  * This should be exported through the `cluster:usage:resources:sum` series with
    a resource type of `machines.machine.openshift.io`.
* MachineAutoscaler resource count
  * This should be exported through the `cluster:usage:resources:sum` series with
    a resource type of `machineautoscalers.autoscaling.openshift.io`.
* MachineHealthCheck resource count
  * This should be exported through the `cluster:usage:resources:sum` series with
    a resource type of `machinehealthchecks.machine.openshift.io`.
* MachineHealthCheck total nodes covered count
  * `mapi_machinehealthcheck_nodes_covered` - This metric has no labels.
* MachineHealthCheck successful remediations count
  * `mapi_machinehealthcheck_remediation_success_total` - This metric has no labels.
* MachineHealthCheck short circuit state
  * `mapi_machinehealthcheck_short_circuit` - This metric has no labels.

**Metric series to be exported**

* Total Machine resource count, using `cluster:usage:resources:sum{resource="machines.machine.openshift.io"}`.
* Total MachineSet resource count, using `cluster:usage:resources:sum{resource="machinesets.machine.openshift.io"}`.
* Total Cluster Autoscaler scale down nodes, using `cluster_autoscaler_scaled_down_nodes_total`
  as a single combined metric of all labels.
* Total Cluster Autoscaler scale up nodes, using `cluster_autoscaler_scaled_up_nodes_total`
  with no labels.
* Total MachineAutoscaler resource count, using `cluster:usage:resources:sum{resource="machineautoscalers.autoscaling.openshift.io"}`.
* Total MachineHealthCheck resource count, using `cluster:usage:resources:sum{resource="machinehealthchecks.machine.openshift.io"}`.
* Total nodes covered by MachineHealthChecks count, using `mapi_machinehealthcheck_nodes_covered` with no labels.
* Total remediations completed by MachineHealthChecks count, using `mapi_machinehealthcheck_remediation_success_total` with no labels.

In addition to the metrics defined above, the alerts generated by the Machine
API components will be used to augment this data. The listings below are
representative of the alerts that will be used to achieve the queries listed
above.

**These alerts exist today**

* machine-api-operator
  * MachineWithoutValidNode
  * MachineWithNoRunningPhase

**These alerts will need to be created**

* machine-api-operator
  * MachineWithOldDeletionTimestamp - This alert will fire when a Machine resource
    is detected that has deletion timestamp that is older than 6 hours.
* machine health check controller
  * MachineHealthCheckUnterminatedShortCircuit - This alert will fire when the
    Machine Health Check has been short circuited for an extended period
    of time (initial setting would be 24 hours).
* cluster autoscaler
  * ClusterAutoscalerExcessiveSilentNodeCleanup - This alert will fire when the
    Cluster Autoscaler has silently cleaned up too many nodes in a short period
    of time (TBD on initial settings).
  * ClusterAutoscalerUnableToScaleCPULimitReached - This alert will fire when
    the Cluster Autoscaler is unable to add more nodes due to reaching the
    maximum CPU resource threshold.
  * ClusterAutoscalerUnableToScaleMemoryLimitReached - This alert will fire when
    the Cluster Autoscaler is unable to add more nodes due to reaching the
    maximum memory resource threshold.

#### Analysis and Publication

Collecting the metrics associated with this enhancement is one part of the
entire story. There will need to be thoughtful analysis of the data to
demonstrate and expose correlations which may exist.

Documentation and code artifacts will be created to inform interested parties
on the primary methods of consumption for the data analyses. The documentation
will take the form of detailed descriptions of the metrics, associated Prometheus
queries that help to explore this information, and Python based applications
and code snippets which expose the analytic findings.

### Risks and Mitigations

One risk, as noted in the implementation section, is the load that these
metrics will add to the telemetry budget for Machine API projects. If an
exception cannot be made then some decisions about the telemetry stories
captured by this enhancement will need to be made.

Another possible risk is the perceived notions of privacy and behavior analysis
with regards to the exposed data. It is possible that users would view this
telemetry as invasive and exposing too much information about their usage habits,
especially with regards to infrastructure providers and resource consumption.

## Design Details

### Test Plan

Although this enhancement does not propose user-facing changes to the code,
there are a few tests that should be created to ensure continued operation.

1. End-to-end tests that ensure the proper metrics continued to be exposed
from the Machine API componets. These tests will be simply metric scrapes to
ensure that the values feeding the telemetry are available.

2. End-to-end tests that ensure the proper alerts are firing under the conditions
their specific conditions. These tests will ensure that our alerting is not
drifting with respect to the underlying events.

3. Functional tests of created artifacts to prove continued operation. There
will be code artifacts generated to examine the results of telemetry, these
components should have functional tests which ensure continued operation.

### Graduation Criteria

As this enhancement speaks to exposing metrics for analysis outside of OpenShift,
there is no plan for graduation within the platform.

### Upgrade / Downgrade Strategy

Not applicable.

### Version Skew Strategy

Not applicable.

## Implementation History

## Drawbacks

One drawback that could be noted from this enhancement is that the information
returned from this telemetry may not provide information which will improve
OpenShift or the experience for users. This is a risk involved with any data-based
approach to analysis. Although the metrics selected here appear to give the best
view into the issues described, it is possible that the resulting analysis of
this data does not produce meaningful results.

Another drawback might be perceived notions of OpenShift breaking down users
privacy by exposing more telemetric information about usage patterns. This drawback
is difficult to demonstrate specifically but falls in line with similar opinions
that are expressed about information gathering in the current era of computing.
While this may be a drawback, it should be tempered against the fact that users
are free to disable telemetry from their operations.

## Alternatives

It might be possible to achieve some of the same metrics observances by watching
associated activity to infer behavior. For example, without instrumenting more
metrics for the Machine Health Check we could use the current alerting mechanisms
to capture failure states of the health check process and build related queries
around this methodology. This would define a more implicit approach to the
gathering of these metrics instead of the explicit options defined in this enhancement.
