---
title: rapid-recommendations
authors:
  - "@jholecek-rh"
  - "@tremes"
reviewers: 
  - "@deads2k"
approvers: 
  - "@deads2k"
api-approvers: 
  - None
creation-date: 2024-01-03
last-updated: 2024-04-30
tracking-link: 
  - https://issues.redhat.com/browse/CCXDEV-12213
  - https://issues.redhat.com/browse/CCXDEV-12285
see-also:
  - "/enhancements/insights/conditional-data-gathering.md"
replaces:
  - None
superseded-by:
  - None
---

# Insights Rapid Recommendations

This enhancement proposal introduces a new approach to how the data collected 
by the Insights operator can be defined remotely.


## Summary

The Insights Operator collects various data and resources from the OpenShift and Kubernetes APIs. 
The definition of the collected data is mostly hardcoded in the operator's source code and largely 
locked in the corresponding OCP version.

This proposal introduces remote Insights Operator configuration for collected data. 
The feature will allow Red Hat to control what data the Insights Operator gathers, 
within hardcoded boundaries, independently of the cluster version. 


## Motivation

The Insights Operator collects various data and resources from the OpenShift and Kubernetes APIs. 
The definition of the collected data is mostly hardcoded in the operator's source code 
and largely locked in the corresponding OCP version.

The data gathered by the Insights Operator can be divided into several groups based on their source 
and format:

* API resources - API resources are identified by the group, version, kind (GVK) and, optionally, 
  namespace and name. 
  They are represented as JSON or YAML objects.
* Container logs - Container logs are identified by the namespace name, pod name, and messages of 
  interest to be filtered from the log (all the containers for the given Pod will be filtered). 
  They are represented as line-oriented text files.
* Node logs - Similar to container logs, but identified/queried using different parameters.
* Prometheus metrics and alerts - See the 
  [MostRecentMetrics](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#mostrecentmetrics) 
  and [ActiveAlerts](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#activealerts) gatherers.
* Aggregated data - This is data where the Insights Operator employs data processing logic 
  that produces a custom representation of raw data. 
  See e.g. the [WorkloadInfo](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#workloadinfo)
  and [HelmInfo](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#helminfo) gatherers.

When an application (e.g. an Insights recommendation) requests new data, a new Insights Operator 
component (a gatherer) must be developed and, optionally, backported to supported y-stream OCP versions.
This is a labor-intesive and error-prone process. 
Moreover, the availability of the new data point depends on customers updating to OCP versions 
that include the new gatherer. 
This delays the fleet-wide impact of the work by many months and forces Insights recommendation 
developers to decline nominations that target existing OCP versions based on data 
availability (specifically ones related to OCP bugs).

This proposal aims at solving the issues for container logs, which is the biggest gap at the moment.
We intend to extend the solution to node logs and API resources in future iterations. 
On the other hand, we do not intend this feature to affect aggregated data. 
The area of Prometheus metrics and alerts remains open with many unknowns at the moment.

### User Stories

* As an Insights recommendation developer, I want the Insights Operator to start collecting new data
  from existing OCP versions so that I can develop a new Insights recommendation that targets existing
  OCP versions even if it needs new data (e.g. a recommendation about a newly-discovered bug).
* As an analytics person or product manager I want the Insights Operator to collect 
  extra data about my OpenShift component temporarily so that I can make data-driven design decisions.

### Goals

* Enable Insights recommendations that target existing OCP versions and that require 
  new container log data.
* Reduce time to fleet-wide impact for Insights recommendations that require new container log data.
* Reduce effort to develop Insights recommendations that require new container log data.
* Enable one-off queries about the fleet that utilize container log data.
* Provide a solid base for future extensions of the remote configuration feature to node logs 
  and API resources.

### Non-Goals

* **Remote code execution.** The remote configuration will set parameters for a fixed data collection scheme.
  It will not be possible to change the structure of gathered data or instruct the Insights Operator
  to collect arbitrary (e.g. aggregated) data.
* **Changing the data gathering frequency.** The default data gathering period will remain 2 hours. 
* **Remote configuration for collecting node logs.** This feature is planned for a future extension 
  of the remote configuration feature.
* **Remote configuration for collecting API resources.** This feature is planned 
  for a future extension of the remote configuration feature.
* **Remote configuration for collecting Prometheus metrics and alerts.** This feature will be considered 
  as a future extension of the remote configuration feature.
* **Remote configuration for collecting data about layered products.** Layered products are often 
  deployed into custom namespaces which poses unique challenges. 
  We will explore options to extend this feature to layered products later.
* **Replacing all existing gatherers.** Data that require advanced processing by the Insights Operator
  (e.g. workload fingerprints) will keep being collected by hardcoded Insights Operator components. 
  There are no plans to change this.
* **Air-gapped and disconnected clusters.** While the proposed solution enables manual workarounds, 
  we do not think they should be presented as intended solutions for these use cases. 
  These use cases should be addressed separately.

 
## Proposal

The proposal is based on a concept already used by the 
[Conditional data gathering](../insights/conditional-data-gathering.md) feature of the Insights Operator:
Before the Insights Operator starts creating a new archive (i.e. every 2 hours), 
it will download remote configuration from a service on console.redhat.com 
and gather data specified in the configuration. 
The Observability Intelligence team (former CCX) will maintain the remote configuration in a repository 
and deploy it to console.redhat.com whenever it changes.

### Workflow Description

This proposal introduces a new workflow for requesting new data to be collected by the Insights Operator.

A **data requester** is a person who analyzes or develops tools to analyze Insights Operator archives
uploaded to Red Hat.

1. A data requester identifies the need for new data in the Insights Operator archive.
2. The data requester describes the data using a format required by the remote configuration service.
3. The data requester creates a pull request (commit must be signed by the author) to the repository where the remote configuration is maintained.
4. The pull request is reviewed (and either approved or denied) 
   by a member of the Observability Intelligence (former CCX) team.
5. When the pull request is merged, the new remote configuration is deployed 
   to the remote configuration service on console.redhat.com.
6. Insights Operators in connected clusters download the new remote configuration 
   and start collecting the new data.
7. The data requester makes use of the new data in newly incoming Insights Operator archives.

### API Extensions

This proposal does not add any API extension.

### Topology Considerations

#### Hypershift / Hosted Control Planes

This proposal does not require any specific details to account for the HyperShift. 
For now, we can simply extend the remote configuration file to 
request gathering of specific HyperShift resources.

#### Standalone Clusters

This proposal is relevant to standalone (and especially connected) clusters. 
Standalone cluster retrieves the remote configuration (based on its OCP version) and collects data accordingly. 

#### Single-node Deployments or MicroShift

This proposal may lead to increased resource consumption by the Insights Operator 
(described in the [Excessive use of cluster resources](#excessive-use-of-cluster-resources) section), 
but the operator can be disabled in these environments or in case of problems. 

### Implementation Details/Notes/Constraints 

#### Remote Configuration Service

The Insights Operator will reuse the configuration option for the [conditional gathering endpoint](../insights/conditional-data-gathering.md).
The built-in default value will change from:

```bash
https://console.redhat.com/api/gathering/gathering_rules.json
```

to:

```bash
https://console.redhat.com/api/gathering/v2/%s/gathering_rules.json
```

The `%s` parameter is the OCP version provided (by the ClusterVersionOpeartor) as `RELEASE_VERSION` 
environment variable in the Insights Operator deployment. 
The remote configuration service will be responsible for serving configuration that can be 
successfully processed by an Insights Operator of the given version.

The new endpoint will return both the current conditional gathering configuration and the new remote
configuration for container logs gathering. 
In future versions, the endpoint may also include remote configuration for node logs and API resources.

The remote configuration would look like this:

```
{
  "conditional_gathering_rules": [
    {
      "conditions": [
        {
          "type": "alert_is_firing",
          "alert": {
            "name": "AlertmanagerFailedReload"
          }
        }
      ],
      "gathering_functions": {
        "containers_logs": {
          "alert_name": "AlertmanagerFailedReload",
          "container": "alertmanager",
          "tail_lines": 50
        }
      }
    }
  ],
  "container_logs": [
    {
      "namespace": "openshift-something",
      "pod_name_regex": "some-pod-.*",
      "previous": false,
      "messages": [ 
        "log-line-regex-filter1",
        "log-line-regex-filter2"
      ]
    }
  ]
}
```

Note that the request for container logs does not include any condition. The
data would be gathered from all clusters supporting this feature. We might
consider merging conditional gathering and this new feature in the future. 


#### Remote Configuration Download

The Insights Operator will try to download the remote configuration (in case of
failure, the request is repeated for a limited number of attempts) before
creating each archive (every 2 hours).
This enables fast recovery time in case a remote configuration change caused issues and had to be reverted.
The frequency is the same as with the 
[conditional gathering configuration](../insights/conditional-data-gathering.md) (since OCP 4.10). 
40k connected clusters requesting a remote configuration update every two hours mean 11 requests per second on average.

If the download fails for any reason (and the number of retries is exceeded), the default configuration shipped in the operator binary is used 
as the default fallback. 

#### Remote Configuration Validation

After a successful download, the remote configuration is validated using a hardcoded 
JSON schema (it is part of the Insights Operator GitHub repository).
The schema can evolve over time; we intend to change it only between x-stream and y-stream versions 
(no backports) to simplify reasoning about differences between data gathered 
by different Insights operator versions. Changes in z-stream versions
(backports) will still be possible in cases that we will be unable to handle at
the level of the remote configuration service.

The remote configuration will be stored in each archive (similarly to what is done today 
for the conditional gathering) to facilitate troubleshooting. 
See also [Status Reporting and Monitoring](#status-reporting-and-monitoring).

# Container Log Data Gathering

In principle, the Insights Operator will process container log data requests as follows:

1. For each individual request, identify containers matching the request
   (namespace, pod, container). These names will be used when storing the data
   in the Insights archive 
   (see [Insights Archive Structure Changes](#insights-archive-structure-changes)). 
2. For each container, use regular expressions from matching requests 
   to build a single regular expression.
3. Use the regular expressions to gather pod log data.

#### Gathering Boundaries

The Insights Operator will have hardcoded boundaries for data that can be requested through 
the remote configuration. 
It will collect container logs only from pods in

* OpenShift and Kubernetes namespaces (prefixes `openshift-` ,`kube-`)

#### Data Redaction/Obfuscation

The Insights Operator will apply existing container log redaction/obfuscation settings to 
container logs gathered using the Rapid Recommendations feature:

* The Insights Operator will apply no data redaction/obfuscation by default
* Customers can enable global obfuscation of IP addresses and cluster domain
  name (this makes it more difficult to follow Insights recommendations)

#### Limits on Processed and Stored Data

The Insights Operator will have hardcoded limits on the amount of processed and stored data:

* The Insights Operator will process log messages at most 6 hours old.
* The total amount of pods for which logs can be requested is limited by the [Gathering Boundaries](#gathering-boundaries).
* The Insights Operator already has a limit on the archive size. The current limit is 8MB (uncompressed).
  A new limit is being discussed separately. 
  The current proposal is 24MB (uncompressed).

#### Status Reporting and Monitoring

The Insights Operator will add two new conditions to the ClusterOperator resource:

* `RemoteConfigurationUnavailable` will indicate whether the Insights Operator can access 
  the configured remote configuration endpoint (host unreachable or HTTP errors).
* `RemoteConfigurationInvalid` will indicate whether the Insights Operator can process 
  the supplied remote configuration. 
  Cluster administrators will not be expected to take any action if this condition becomes true.

The Insights Operator will unconditionally gather:

* The Insights ClusterOperator resource (gathered already). 
  Red Hat (the Observability Intelligence team responsible for operating the pipeline that processes
  incoming archives) will monitor the two conditions to detect issues with remote configuration.
* The remote configuration used for producing the archive. 
  This will help reproducing and troubleshooting issues.
* Metrics about the gathering process.
  * The total real time of data gathering (gathered already)
  * The real execution time of each gatherer (gathered already)
  * The number of records created by each gatherer (gathered already)
  * Insights Operator CPU usage
  * Insights Operator memory usage

#### Insights Archive Structure Changes

The new container log data will be saved in a new directory structure similar to that 
of must-gather archives:

```bash
<archive_root>/namespaces/<namespace_name>/pods/<pod_name>/<container_name>/current.log
<archive_root>/namespaces/<namespace_name>/pods/<pod_name>/<container_name>/previous.log
```

Each file will preserve the original (chronological) line order.

Container logs gathered by existing gatherers will remain in their current locations for now. 
We will seek unification in future updates. This includes data gathered by:

* [ClusterOperatorPodsAndEvents](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#clusteroperatorpodsandevents)
* [ContainersLogs](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#containerslogs) (conditional gathering)
* [KubeControllerManagerLogs](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#kubecontrollermanagerlogs)
* [LogsOfNamespace](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#logsofnamespace) (conditional gathering)
* [OpenShiftAPIServerOperatorLogs](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#openshiftapiserveroperatorlogs)
* [SAPVsystemIptablesLogs](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#sapvsystemiptableslogs)
* [SchedulerLogs](https://github.com/openshift/insights-operator/blob/master/docs/gathered-data.md#schedulerlogs)


### Risks and Mitigations

#### Remote Configuration Endpoint Invalid

Risk: The user overrides the built-in default value of the configuration option
specifying the remote endpoint with an invalid value.

Mitigations:
* The Insights Operator will use the [default configuration](#remote-configuration-download)
  available in the operator binary.
* The Insights Operator archive will contain data to detect this situation.
  A new Insights recommendation will be created to detect this case and
  recommend remediation steps.

#### Remote Configuration Service Inaccessible

Risks:

* The remote configuration is not available (service down, network configuration issues).

Mitigations:

* The Insights Operator will use the [default configuration](#remote-configuration-download)
  available in the operator binary.
* Red Hat will monitor the remote configuration service directly as well as the 
  `RemoteConfigurationUnavailable` condition 
  (see [Status Reporting and Monitoring](#status-reporting-and-monitoring)) 
  in incoming Insights Operator archives.

In disconnected and air-gapped clusters, two behaviors can occur:
* If data gathering or the container logs gatherer will be disabled, the Insights Operator will not 
  attempt to download the configuration at all.
* If data gathering is enabled or triggered manually, the Insights Operator will 
  [report](#status-reporting-and-monitoring) the failure 
  and [use](#remote-configuration-download) the default configuration available
  in the operator binary.

#### Invalid Remote Configuration

Risk: The remote configuration cannot be parsed or does not pass schema validation.

For transparency, the Insights Operator will use empty configuration in this
case. That means it will not gather any pod log data or data specified by
conditional gathering rules. It is acceptable because this situation would be
caused by Red Hat and would have to be fixed quickly anyway.

Mitigations:
* Red Hat will employ a CI/CD pipeline that will attempt to catch invalid
  changes before they get to production.
* The Insights Operator will include the invalid configuration file in the
  archive along with any validation errors.
* Red Hat will monitor the `RemoteConfigurationInvalid` condition 
  (see [Status Reporting and Monitoring](#status-reporting-and-monitoring)) 
  and fix/work around the issue by updating the remote configuration or the remote configuration service.

#### Data Gathering Failures

Risk: No container log data found/gathered.

Mitigations:
* Correctness (fit-for-purpose) of data requests will be the responsibility of
  data requesters. Incorrect data requests will affect only consumers of that
  data.
* The Insights archive metadata will indicate that no data was found/gathered
  for a particular container log request. The metadata will enable data
  requester to troubleshoot their data requests.

#### Excessive Use of Cluster Resources

Risk: Remote configuration causes the Insights Operator to process large amounts of data.

Mitigations:
* Red Hat will [monitor](#status-reporting-and-monitoring) Insights Operator
  resource consumption in the fleet. Excessive resource consumption can be
  fixed within a business day (maximum) by reducing requests in the remote
  configuration. 
* Red Hat will closely [monitor](#status-reporting-and-monitoring) Insights
  Operator resource consumption following the deployment of any changes to the
  remote configuration. The remote configuration change will be rolled back
  within 2 hours in case of any issues.
* The solution also enables version-based canary rollouts at the cost of
  increased remote configuration service complexity.

#### Excessive Archive Size

Risk: The remote configuration will trigger gathering of extensive amount of data.

Mitigations:
* The Insights Operator has a hardcoded [archive size limit](#limits-on-processed-and-stored-data).
* Red Hat will [monitor](#status-reporting-and-monitoring) the size of gathered
  pod logs in the fleet. Excessive increase in archive sizes can be fixed
  within a business day (maximum) by reducing requests in the remote
  configuration. 
* Red Hat will closely [monitor](#status-reporting-and-monitoring) the size of
  gathered pod logs following the deployment of any changes to the
  remote configuration. The remote configuration change will be rolled back
  within 2 hours in case of any issues.
* The solution also enables version-based canary rollouts at the cost of
  increased remote configuration service complexity.

#### Expensive Regular Expressions

Risk: Complex regular expressions (created accidentally or by a malicious
actor) can be expensive to compile in terms of both CPU and memory.

Mitigations:

* Only basic regular expressions will be supported (specifically no back-references).
* The CI/CD pipeline for the remote configuration service will try to compile
  a regular expression built from individual data requests and fail if the
  compilation takes too much time/memory.
* The Insights Operator will employ a hard limit on regular expression
  compilation time.

#### Man-in-the-Middle Attack

Risk: An attacker could use a malicious remote configuration to cause
a denial of service or pull sensitive data from the cluster.

Mitigations:

* The Insights Operator downloads the remote configuration and uploads the
  resulting archive using a TLS connection with server certificate validation.
  The console.redhat.com certificate has a public root certificate authority.
* The same attacks could be caused accidentally - see the risks above for
  mitigations of various cases.
* The risk of pulling really sensitive data (e.g. secrets) from the cluster is
  rather low for container logs. Container logs are not expected to contain
  such data in the first place.

### Drawbacks

#### New Attack Surface

The general drawback is that the remote configuration represents a new attack
surface. We have conducted a security review of this proposal. Potential
issues the review identified are addressed in the [Risks and
mitigations](#risks-and-mitigations) section. 

#### Disconnected Clusters

Another limitation is on the side of disconnected (or "air-gapped") clusters, 
but that is already the case today.


## Design Details

### Open Questions [optional]

* ~~How does this proposal fit with the existing techpreview API defined 
  in the [On demand data gathering](../insights/on-demand-data-gathering.md) enhancement proposal?~~
  The existing techpreview API allows the cluster admin to exclude specific 
  gatherers by name. Container log data will be technically gathered by
  a dedicated gatherer; the cluster admin will be able to disable the whole
  gatherer. We believe this is good enough for now. If there would be demand,
  we might consider that would allow the cluster admin to disable container log
  data gathering for specific namespaces/pods/containers in the future.
* ~~Do we want to "transform" some existing gatherers into the remote configuration?~~ 
  Yes we want, but not in the first iteration. We are focusing on the container log data 
  in the first iteration.
* ~~Does each new change in the configuration file mean a new version?~~ 
  No there is only one version of the remote configuration for the corresponding OCP X.Y.Z version. 
  This is described in the [Remote configuraiton service](#remote-configuration-service) section.
* ~~Should we set [limits](#limits-on-processed-and-stored-data) relative to the number of nodes?~~
  It would result in bigger archives on bigger clusters, but the data would not get truncated. 
  It would also allow us to set other limits, e.g. a limit on the number of pods in one request
  (useful to protect against too broad requests and for extending the feature to layered products). 
  We cannot do it right now because of pods like machine-config-daemon which are very popular in 
  Insights recommendations but whose number grows with the number of nodes. As
  this is largely an independent issue, we decided not to include it in the
  scope of this proposal. We might reconsider in the future based on experience
  with operating the feature.


## Test Plan

This will be tested as part of the existing Insights Operator integration tests. 
Following scenarios will need to be tested:

- successful scenarios:
  - config is valid and it is used for the data gathering - the operator gathers required data and provides info 
    about successful gathering in the Insights archive metadata
  - config is updated (compared to the last known version) - the operator gathers newly required data and provides info
    about successful gathering in the Insights archive metadata
- unsuccessful/failure scenarios:
  - the operator cannot parse the remote configuration - expect appropriate information 
    in the Insights archive metadata
  - the operator cannot validate the remote configuration - expect appropriate information 
    in the Insights archive metadata
  - the operator does not find any required data - expect appropriate information 
    in the Insights archive metadata

For more considerations, see also [Graduation Criteria](#graduation-criteria).

## Graduation Criteria

We plan to work on the implementation and the deployment to the production in the following steps:

1. Define and implement the remote configuration for container logs. 
   The feature will be released in a y-stream OCP version, but we will keep it closed for data requesters.
2. Wait for some customers to update to the OCP version (e.g. 500 customer clusters). 
   This could take a month or two.
3. Experiment with log requests in small steps and monitor how the feature performs. 
   The feature enables fast feedback loop within a day. 
   1. The inclusion of the Insights Operator version into the remote configuration request 
      also enables version-based canary rollouts. 
   For instance, we will be able to test a change on pre-release (i.e. mostly internal) clusters first.
4. Open the feature to data requesters when we are confident that the feature 
   and our monitoring capabilities are robust enough.
   This could be aligned with y+1 or y+2 OCP version.
5. Continue monitoring the feature as more and more customers upgrade to OCP versions with the feature.
   Until we start replacing existing gatherers by remote configuration, 
   we will be able to workaround any issues by serving empty/reduced remote configuration 
   which will affect only new applications specifically depending on the feature.


### Dev Preview -> Tech Preview

N/A

### Tech Preview -> GA

N/A

### Removing a deprecated feature

In fact, this is not a removal, but the remote configuration service endpoint is already 
part of the Insights Operator configuration. 
This means that the cluster administrator can override this endpoint 
(or simply insert an empty string) to disable this feature. 
The obvious consequence for us is that some data from this cluster will be missing, 
but this information must be visible in the Insights archive 
(as mentioned in the [Risk and mitigations](#risks-and-mitigations) section).

## Upgrade / Downgrade Strategy

When a cluster version is upgraded or downgraded, Insights Operator requests a new version 
of the remote configuration (the operator knows and uses the OCP cluster version). 
As mentioned in the 
[Remote configuration service](#remote-configuration-service) section, 
it is the responsibility of the administrator/owner of the remote configuration to provide valid 
and updated corresponding configuration. Other than that, no special upgrade or downgrade strategy 
should be needed. 
We do not expect any changes to the API or the cluster in general. 
There will be changes to the Insights Operator, but the operator must not be marked as Degraded 
if the connection to the remote configuration fails 
(i.e the particular data gathering will be skipped and this information 
will be available in the Insights archive).

## Version Skew Strategy

This enhancement proposal considers only two components (which are not critical to the cluster). 
It is the Insights Operator, which is versioned according to the OCP version and the remote configuration,
which is versioned in the same way. 
The version skew between these two components should not happen even during 
the cluster upgrade (see the [Remote configuration service](#remote-configuration-service) section).
However there may be a non-critical skew between the Insights Operator and the remaining control-plane components
during cluster upgrades - for example the Insights Operator can be already running in version 1.2.4, 
but some other components may still be in version 1.2.3. 
This is acceptable in the sense that the operator will only collect available data 
and provide information about the requested data, but which is not available in the cluster.

## Operational Aspects of API Extensions

The proposal does not add any API extensions and so there is not operational aspects to consider. 

#### Failure Modes

This proposal does not include any new API extension. 
Failure scenarios are described in [Risk and mitigations](#risks-and-mitigations) section and 
in general this enhancement will have no impact on cluster health and stability. 
The main issue or risk is not having the requested Insights data, 
but this is mainly a matter for the Insights/Observability Intelligence team. 

There is also plan to consider security aspects of this proposal as described in 
[Drawbacks](#drawbacks) section - i.e there is plan to do security review/clearance.

## Support Procedures

The main way to identify potential problems or failure modes is to examine 
the recent Insights archive from the relevant cluster. 
The possible problems are described in the [Risk and mitigations](#risks-and-mitigations) section. 
The archive must provide information about the remote configuration used, 
about any issue related to the connection, validation or use of remote content. 
This means that not all the required data is also collected and this information must also be visible
from the archive.
The proposed feature will generally have no impact on the overall health 
and stability of the cluster and therefore the main risk of supporting this feature is the ability 
to obtain the requested Insights data. 
This ability is not guaranteed, and this information available 
in the Insights archive should help to best resolve any issues.  

## Implementation History

The plan is outlined in the [Graduation criteria](#graduation-criteria) section. 
First we want to focus on the definition and implementation of remote configuration for log data, 
then we will continue with data identifiable by its group, version, type. 
As one of the last steps, we can consider some remote configuration for Prometheus metrics data, 
but as mentioned, this will require an update to this proposal.    

## Alternatives

### Keep Status Quo

The current solution has many drawbacks (see the [Motivation](#motivation) section) 
but it could be deemed good enough. 
However, we believe that the proposed solution is feasible and that it can considerably increase 
the value of services for (not only) connected customers.

### Remote Code Execution

We explored a solution based on WebAssembly execution in a project called 
"Thin Insights Operator" in 2020. 
The idea was dismissed eventually based on risks associated with remote code execution.

### Remote Configuration Fingerprints

One suggestion from the security review that we did not implement was a remote
configuration fingerprint that the Insights Operator could use to independently
verify authenticity and integrity of a downloaded remote configuration.

We think the benefits this adds on top of [TLS with server certificate
validation](#man-in-the-middle-attack) and [remote configuration
validation](#remote-configuration-validation) are not worth the added
operational complexity (maintaining keys/certificates used for the fingerprints).


## Infrastructure Needed [optional]

This proposal requires:

* A process (probably with a pipeline) for building the remote configuration from requests by 
  independent/distributed data requesters.
* A remote service (running in console.redhat.com) providing the remote configuration.
* A monitoring process/pipeline.
