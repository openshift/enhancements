---
title: installer-telemetry-metrics
authors:
  - "@patrickdillon"
  - "@rna-afk"
reviewers:
  - "@abhinavdahiya"
  - "@brancz"
  - "@crawford"
  - "@sdodson"
approvers:
  - "@abhinavdahiya"
  - "@brancz"
  - "@crawford"
  - "@sdodson"
creation-date: 2020-02-07
last-updated: 2020-02-07
status: provisional
see-also:
replaces:
superseded-by:
---

# Installer Telemetry Metrics

## Release Signoff Checklist

- [ ] Enhancement is `implementable`
- [ ] Design details are appropriately documented from clear requirements
- [ ] Test plan is defined
- [ ] Graduation criteria for dev preview, tech preview, GA
- [ ] User-facing documentation is created in [openshift-docs](https://github.com/openshift/openshift-docs/)

## Open Questions

 > 1. How to filter to collect only real user usage and prevent OpenShift developer/Red Hat internal users from skewing metrics? Perhaps filter by version?
 > 2. Is it possible to determine whether a user modified an ignition config or manifest asset by comparing the asset the installer would generate to the one found on disk?
 > 3. How to authenticate using pull secret.
 
## Summary
Telemetry collects metrics from running clusters, but no metrics are obtained from the installer binary.
This enhancement proposes that the installer would push metrics related to user workflows, installation statistics, and feature usage. The installer would be instrumented to collect Prometheus label key-value pairs, convert them to metric samples and push those metrics to an [aggregation pushgateway][weaveworks-aggregation-pushgateway].
This enhancement focuses on the design of the metrics to be pushed to Prometheus.

## Motivation

Obtaining metrics from the installer would allow engineers and product managers to understand different user workflows, environments, experiences, and difficulties, as well as an indication of feature usage.  By pushing these metrics to the Telemeter, OpenShift members would be able to create a variety of queries related to these aspects.

### Goals

1. Design meaningful metrics to collect user experience and behavior with the installer

### Non-Goals

1. Collected metrics will not allow analysis of individual installer invocations and will not display detailed data such as log or error messages. We won't be able to answer "how long did an individual installation take?" but rather 95% of installs finish within, say, 40 minutes.
2. Installer instrumentation/internal implementation will be discussed in a separate design/enhancement
3. Pushgateway implementation will be discussed in a separate design/enhancement

## Proposal

### User Stories

#### Telemetry Analyst
Some examples:

As a telemetry analyst I would like to create queries which analyze aggregate installation data to determine how long the installation process takes in total and broken down by different stages so that I can better understand user experience, expose any abnormalities, and compare different versions of the installer.

As a telemetry analyst I would like to create queries which show aggregate counts of failure by stage of installation so that we can better understand where users are experiencing problems. (This will not provide  specific error messages/logs).

As a telemetry analyst I would like to see how often users modify ignition configs or manifests so that I can better understand how users are interacting with the installer. [(see open question #3)](open-questions)

As a telemetry analyst I would like to see how often the installer is being run on certain operating systems so I can better understand our user environments and needs.

### Implementation Details/Notes/Constraints
The installer does not fit a typical Prometheus use case in that:
* metrics will be pushed rather than scraped
* Prometheus labels will be (ab)used to collect information 
* the aggregation push gateway will be used to form the metrics into a useful time series

The proposed metrics fall into three categories:

#### Commands

##### cluster_installation_create 
`cluster_installation_create` represents an invocation of `openshift-install create cluster`. The values for the labels would be collected throughout the execution of the command and a single sample would be pushed per installer run:

```
# HELP cluster_installation_create represents a single run of create cluster by the OpenShift installer.
# TYPE cluster_installation_create histogram
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="15"} 0
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="20"} 0
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="25"} 0
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="30"} 1
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="35"} 1
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="40"} 1
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="45"} 1
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="50"} 1
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="55"} 1
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="60"} 1
cluster_installation_create_bucket{platform="aws",result="success",version="4.2",os="linux",le="+Inf"} 1
cluster_installation_create_sum{platform="aws",result="success",version="4.2",os="linux"} 30
cluster_installation_create_count{platform="aws",result="success",version="4.2",os="linux"} 1
```
The example above represents a single sample. This sample indicates that a Linux user successfully installed a 4.2 AWS cluster, which took between 30 and 35 minutes to complete installation. 

###### Label Values
* `result` - provides limited context to the outcome of an attempted install:
    - `ProvisioningFailed`
    - `BootstrapFailed`
    - `APIFailed`
    - `OperatorsDegraded`
    - `Success`
* `platform` - currently we deploy to 8 platforms: `aws`, `azure`, `gcp,` `libvirt`, `none`, `openstack`, `ovirt`, and `vsphere`. The list will grow. 
* `os` - You can find all the available combinations of [Go OS and arch values](https://github.com/golang/go/blob/master/src/go/build/syslist.go) with `go tool dist list`. At this stage I assume we only want to colllect operating systems and some of the values are unlikely/impossible:
  - `linux`
  - `darwin` - this could be rewritten to `macos` on the installer client if we prefer
  - `bsd` - this would encompass the `freebsd`, `netbsd`, and `openbsd` values
  - `windows` - future proofing? 
  - `other` - everything else not captured above
* `version` - represents minor releases; assuming we do not backport metrics, `4.5` will be the only value if we make it into the next release and we could expect roughly 20 values over the next five years.
* `le` - the standard Prometheus label for histogram buckets, this represents how long a command took complete. The current proposal is to have a 5 minute step starting at 15 minutes and maxing at 60.

##### cluster_installation_waitfor
UPI installs utilize the wait-for command, which would be tracked with a similar histogram:
```
# HELP cluster_installation_waitfor represents a single run of wait-for by the OpenShift installer.
# TYPE cluster_installation_waitfor histogram
cluster_installation_waitfor_bucket{target="cluster-complete",platform="aws",result="success",version="4.2",os="linux",le="5"} 0
cluster_installation_waitfor_bucket{target="cluster-complete",platform="aws",result="success",version="4.2",os="linux",le="10"} 0
cluster_installation_waitfor_bucket{target="cluster-complete",platform="aws",result="success",version="4.2",os="linux",le="15"} 1
cluster_installation_waitfor_bucket{target="cluster-complete",platform="aws",result="success",version="4.2",os="linux",le="20"} 1
cluster_installation_waitfor_bucket{target="cluster-complete",platform="aws",result="success",version="4.2",os="linux",le="25"} 1
cluster_installation_waitfor_bucket{target="cluster-complete",platform="aws",result="success",version="4.2",os="linux",le="30"} 1
cluster_installation_waitfor_bucket{target="cluster-complete",platform="aws",result="success",version="4.2",os="linux",le="+Inf"} 1
cluster_installation_waitfor_sum{target="cluster-complete",platform="aws",result="success",version="4.2",os="linux"} 15
cluster_installation_waitfor_count{target="cluster-complete",platform="aws",result="success",version="4.2",os="linux"} 1
```

###### Label Values
Potential label values are the same as the previous metric, except:
* `le` has values between 5 and 30 minutes
* `target` and `result` are dependent:
  - `bootstrap-complete`
      - `BootstrapFailed`
      - `APIFailed`
      - `Success`
  - `install-complete`
      - `OperatorsDegraded`
      - `Success`

##### cluster_installation_manifests and cluster_installation_ignition
These metrics are simple counters totalling the number of runs for `openshift-install create manifests` or `openshift-install create ignition-configs`:

```
# HELP cluster_installation_manifests represents a single run of create manifests by the OpenShift installer.
# TYPE cluster_installation_manifests counter
cluster_installation_manifests{platform="aws",result="success",version="4.2",os="linux"} 1

# HELP cluster_installation_ignition represents a single run of create ignition-configs by the OpenShift installer.
# TYPE cluster_installation_ignition counter
cluster_installation_ignition{platform="aws",result="success",version="4.2",os="linux"} 1
```

Possible `result` values are `success` and are `error`, otherwise all possible label values are the same as `cluster_installation_create`. The sample value would always be one in order to aggregate to a total count of runs of this command.

##### cluster_installation_destroy

When a user destroys a cluster a single sample of this metric would be pushed:

```
# HELP cluster_installation_destroy represents a single run of destroy cluster by the OpenShift installer.
# TYPE cluster_installation_destroy histogram
cluster_installation_destroy_bucket{platform="aws",result="success",version="4.2",os="linux",le="5"} 0
cluster_installation_destroy_bucket{platform="aws",result="success",version="4.2",os="linux",le="10"} 1
cluster_installation_destroy_bucket{platform="aws",result="success",version="4.2",os="linux",le="15"} 1
cluster_installation_destroy_bucket{platform="aws",result="success",version="4.2",os="linux",le="20"} 1
cluster_installation_destroy_bucket{platform="aws",result="success",version="4.2",os="linux",le="25"} 1
cluster_installation_destroy_bucket{platform="aws",result="success",version="4.2",os="linux",le="30"} 1
cluster_installation_destroy_bucket{platform="aws",result="success",version="4.2",os="linux",le="+Inf"} 1
cluster_installation_destroy_sum{platform="aws",result="success",version="4.2",os="linux"} 10
cluster_installation_destroy_count{platform="aws",result="success",version="4.2",os="linux"} 1
```
The label values are the same as `openshift_installation_create` except `le` has values between 5 and 30 minutes and `result` is `success` or `error`.

##### cluster_installation_gather
We will count invocations and outcomes of `gather bootstrap`:

```
# HELP cluster_installation_gather represents a single run of gather bootstrap by the OpenShift installer.
# TYPE cluster_installation_gather counter
cluster_installation_gather{platform="aws",result="success",version="4.2",os="linux"} 1
```
Potential label values would be the same as `cluster_installation_manifests` except `result` could return an extra value for `SSHError` which shows that users tried to connect to gather, but were unauthorized.

#### Duration

The `cluster_installation_create` metric above captures the overall duration of an installation. The metrics in this category represent the duration of  each stage of installation in the execution of the `create` or `wait-for` commands. The proposed metrics are: 

* `cluster_installation_duration_infra`
* `cluster_installation_duration_api`
* `cluster_installation_duration_bootstrap`
* `cluster_installation_duration_operators`

Each of these metrics (except infrastructure) would have five labels:
  * `command` - either `create` or `wait-for`
  * `result` - `success` or `error`
  * `version` 
  * `platform`
  * `le` - step size of 3 minutes with a max of 30

An example of a sample tracking how long it took for bootstrap complete after the temporary control plane has come up: 
```
# HELP cluster_installation_bootstrap tracks the duration between the appearance of the temporary control plane and bootstrap completion.
# TYPE cluster_installation_bootstrap histogram
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="3"} 0
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="6"} 0
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="9"} 0
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="12"} 1
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="15"} 1
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="18"} 1
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="21"} 1
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="24"} 1
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="27"} 1
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="30"} 1
cluster_installation_bootstrap_bucket{command="create",platform="aws",result="success",le="+Inf"} 1
cluster_installation_bootstrap_sum{command="create",platform="aws",result="success"} 10
cluster_installation_bootstrap_count{command="create",platform="aws",result="success"} 1
```

#### Modification
The `cluster_installation_modification`- metrics represent whether user-modified manifest or ignition-config assets were used to install the cluster.

The proposed metrics represent categories of one or more asset file modified during an installation:

Metric | Modified Files
-------|---------------
cluster_installation_modification_config_manifest | cloud-provider-config.yaml <br> cluster-config.yaml <br> cluster-infrastructure-02-config.yml <br> kube-cloud-config.yaml
cluster_installation_modification_dns_manifest | cluster-dns-02-config.yml <br> cluster-ingress-02-config.yml
cluster_installation_modification_network_manifest | cluster-network-01-crd.yml <br> cluster-network-02-config.yml <br> cluster-proxy-01-config.yaml
cluster_installation_modification_scheduler_manifest | cluster-scheduler-02-config.yml
cluster_installation_modification_cvo_manifest | cvo-overrides.yaml
cluster_installation_modification_etcd_manifest | etcd-ca-bundle-configmap.yaml <br> etcd-client-secret.yaml <br> etcd-host-service-endpoints.yaml <br> etcd-host-service.yaml <br> etcd-metric-client-secret.yaml <br> etcd-metric-serving-ca-configmap.yaml <br> etcd-metric-signer-secret.yaml <br> etcd-namespace.yaml <br> etcd-service.yaml <br> etcd-serving-ca-configmap.yaml <br> etcd-signer-secret.yaml
cluster_installation_modification_machineconfig_manifest | 04-openshift-machine-config-operator.yaml <br> machine-config-server-tls-secret.yaml
cluster_installation_modification_ca_manifest | machine-config-server-tls-secret.yaml
cluster_installation_modification_pullsecret_manifest | openshift-config-secret-pull-secret.yaml
cluster_installation_modification_bootstrap_ignition | bootstrap.ign
cluster_installation_modification_master_ignition | master.ign
cluster_installation_modification_worker_ignition | worker.ign

Each of these metrics would have a label for result, representing either `success` or `error`.

An example metric:
```
# HELP cluster_installation_modification_network_manifest counts installations where users modified one of: cluster-network-01-crd.yml, cluster-network-02-config.yml, or cluster-proxy-01-config.yaml.
# TYPE cluster_installation_modification_network_manifest counter
cluster_installation_modification_network_manifest{result="success"} 1
```
#### Cardinality

Cardinality represents the total number of timeseries that could be created from the combination of label key values in a metric. 

##### cluster_installation_create

`result` | `platform` | `os` | `version` | `le` + sum + count | subtotal
---------|------------|------|-----------|--------------------|---------
5        |8           |5     |1          | 13                 |2,600

Initially we would only have one version, but if we assume that version will grow at a rate of 4 releases per year, we would have 20 releases over the next five years. Therefore the cardinality over five years would be 2,600 x 20 = 52,000 (this does not include platform growth, which would be expected but slower).

##### cluster_installation_waitfor

`target` `result` | `platform`| `os` | `version` | `le` + sum + count | subtotal
------------------|-----------|------|-----------|------|---------
5                 |8          | 5    | 1         |9     |1,800

With a similar `version` growth as discussed before, five-year cardinality would be 20 x 1,800 = 36,000.

##### cluster_installation_manifests 

`result` | `platform` | `os` | `version` | subtotal
---------|------------|------|-----------|---------
2        |8           |5     |1          |80

Five-year cardinality: 20 * 80 = 1,600

##### cluster_installation_ignition

`result` | `platform` | `os` | `version` | subtotal
---------|------------|------|-----------|---------
2        |8           |5     |1          |80

Five-year cardinality: 20 * 80 = 1,600

##### cluster_installation_destroy

`result` | `platform` | `os` | `version` | `le` + sum + count | subtotal
---------|------------|------|-----------|--------------------|---------
2        |8           |5     |1          |9                   |720

Five-year cardinality: 20 * 720 = 14,400

##### cluster_installation_gather

`result` | `platform` | `os` | `version` | subtotal
---------|------------|------|-----------|---------
3        |8           |5     |1          |120

Five-year cardinality: 20 * 120 = 2,400


##### Duration Metrics

Num of Metrics | `command` | `platform` | `result` | `version` | `le` + sum + count | subtotal
---------------|-----------|------------|----------|-----------|--------------------|---------
|4         | 2         | 8          | 2        |1          | 13                 | 1,664

Five-year cardinality: 20 * 1,664 = 33,280

##### Modification
Num of Metrics | `result` | subtotal
---------------|----------|---------
|12        | 2        | 24

Modification is not tagged with a version. If keeping a version would be beneficial, the five year cardinality would be 20 x 24 = 480.

### Risks and Mitigations

The pushgateway would be an a public endpoint reachable on the internet. Therefore some authentication or security should be in place in order to prevent malicious pushing of metrics.

Users and customers may be wary of exposing information when using the installer.


## Design Details

### Test Plan

**TODO**

**Note:** *Section not required until targeted at a release.*

Consider the following in developing a test plan for this enhancement:
- Will there be e2e and integration tests, in addition to unit tests?
- How will it be tested in isolation vs with other components?

No need to outline all of the test cases, just the general strategy. Anything
that would count as tricky in the implementation and anything particularly
challenging to test should be called out.

All code is expected to have adequate tests (eventually with coverage
expectations).

### Graduation Criteria

#### High-level Rollout Plan
As suggested by @brancz

1. Local testing
2. Deploy the push aggregation gateway as part of openshift telemetry, but only configure CI to push 
3. Send metrics on every install
**Note:** *Section not required until targeted at a release.*

Define graduation milestones.

These may be defined in terms of API maturity, or as something else. Initial proposal
should keep this high-level with a focus on what signals will be looked at to
determine graduation.

Consider the following in developing the graduation criteria for this
enhancement:
- Maturity levels - `Dev Preview`, `Tech Preview`, `GA`
- Deprecation

Clearly define what graduation means.

#### Examples

These are generalized examples to consider, in addition to the aforementioned
[maturity levels][maturity-levels].

##### Dev Preview -> Tech Preview

- Ability to utilize the enhancement end to end
- End user documentation, relative API stability
- Sufficient test coverage
- Gather feedback from users rather than just developers

##### Tech Preview -> GA 

- More testing (upgrade, downgrade, scale)
- Sufficient time for feedback
- Available by default

**For non-optional features moving to GA, the graduation criteria must include
end to end tests.**

##### Removing a deprecated feature

- Announce deprecation and support policy of the existing feature
- Deprecate the feature

### Upgrade / Downgrade Strategy

Not applicable.

### Version Skew Strategy

Not applicable. 

## Implementation History

Major milestones in the life cycle of a proposal should be tracked in `Implementation
History`.

## Drawbacks

By using the aggregation pushgateway, we are losing specificity. We will not be able to tell whether one user heavily skews results.

 We want to understand user events in how they interact with the installer and get specific error messages and logs, but we cannot do that with Prometheus as a backing service. 

## Alternatives


A standard pushgateway could be used to collect individually identified metrics from installer invocations, which could then be summed up, getting us similar results as the aggregation gateway but with finer granularity. We do not pursue this option due to cardinality concerns.

## Infrastructure Needed

An aggregation pushgateway would need to be created and administered, perhaps by a service delivery team.

[weaveworks-aggregation-pushgateway]: https://github.com/weaveworks/prom-aggregation-gateway
[prom-godocs]: https://godoc.org/github.com/prometheus/client_golang/prometheus