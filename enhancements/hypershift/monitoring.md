---
title: monitoring

authors:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
  - "@alvaroaleman"
  - "@ironcladlou"
  - "@derekwaynecarr"
  
reviewers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
  - "@alvaroaleman"
  - "@derekwaynecarr"
  - "@imain"

approvers:
  - "@csrwng"
  - "@enxebre"
  - "@sjenning"
  - "@alvaroaleman"
  - "@derekwaynecarr"
  - "@imain"

api-approvers:
  - None

tracking-link:
  - https://issues.redhat.com/browse/OCPPLAN-5771

creation-date: 2022-07-04
last-updated: 2022-07-04
---

# Monitoring

## Summary

This proposal fleshes out the details for the current monitoring solution for HyperShift form factor i.e. hosed control planes.
This includes which metrics to expose, how to expose them and approaches to collect them.

## Glossary
- Platform monitoring stack - Stack preconfigured, preinstalled, and self-updating that provides monitoring for core platform components, including Kubernetes services.
- UWM (User workloads monitoring) - Stack optionally deployed in OCP to monitor user workloads.
- CMO (Cluster monitoring operator) - A set of platform monitoring components installed in the openshift-monitoring project by default during an OpenShift Container Platform installation.
- Managed service - A service powered by HyperShift managed by RedHat which offers clusters to customers, e.g. SD (ROSA...)
- Self-managed service - A service powered by HyperShift managed by a customer, e.g. ACM/MCE.

See https://docs.openshift.com/container-platform/4.10/monitoring/monitoring-overview.html for more details on OCP monitoring components.

## Motivation
HyperShift differs from standalone OCP in that the components that run in the Hosted Control Plane (HCP) on the management cluster are not on the same cluster service network as the components that run on guest cluster nodes.  
Challenges include but not limited to:
- Which metrics to expose.
- How to expose metrics from guest cluster.
- How to expose metrics from control plane components in management cluster.
- How to send metrics to telemetry from guest cluster.
- How to send metrics to telemetry from control plane components in management cluster.
- How to collect the metrics in a managed service powered by HyperShift scenario.
- How to collect the metrics in self-managed powered by HyperShift scenario.

https://github.com/openshift/enhancements/blob/master/enhancements/monitoring/hypershift-monitoring.md

### User Stories
- As a Service Provider I want to have the ability to monitor metrics that I control and impact the service availability, so I can define SLIs, i.e. hosted control plane.
- As a Service Consumer I want to have the ability to monitor metrics that I control and impact my apps availability, i.e. end workloads.
- As RedHat I want to collect data on clusters run by HyperShift, so I can improve the services powered by it.
- As a Service Provider using HyperShift I want to understand their usage, e.g. management cluster/hostedClusters density, level of usage of autoscaling, etc.
- As a Service Provider, I want to provide my customers metrics for their control plane components
- As a Service Consumer, I want the ability to understand the performance of my managed control plane

### Goals
- Define which metrics have value to expose in both management cluster and guest cluster.
- Define a path to collect metrics in a self-managed scenario (ACM).
- Define a path to collect metrics in a managed service scenario (SD e.g. ROSA).
- Ensure our ability to send desired metrics to telemetry.
- Define which metrics have value to expose in the HyperShift Operator.
- Ensure our ability to send desired metrics to guest cluster CMO.

### Non-Goals

## Proposal
Metrics belonging to a cluster including those needed for telemetry come from 2 places in HyperShift: control plane (management cluster) and guest cluster.
In the guest cluster, the platform monitoring collects and forwards metrics to telemetry in the same way a standalone OCP does.
However, some metrics needed for telemetry come from the control plane, and these are not currently available to the CMO inside the guest cluster.

The following are potential solutions for collecting metrics and send them to telemetry from HyperShift control planes.

### Provide metrics using side-cart
A solution based on creating a side-cart container that would scrape info from the data-plain components(API server, etcd, and cluster version operator) and communicate that to the users clusters. This process currently exist within IBM ROKS toolkit called roks-metrics-deployment(https://github.com/openshift/ibm-roks-toolkit/tree/master/assets/roks-metrics). It provides users with metrics from the managed cluster in a similar way that we would want for HyperShift. In short, we would go about this by adding a metrics-pusher container on pods already existing on our managed cluster that we would want to collect metrics from. This metrics-pusher container will then push metrics from the pods to a push-gateway pod that will exist on our Hypershift-cluster. We will then have metrics from the managed cluster as well as our Hypershift cluster avaliable. To make this accessible for the costumer, we will create a ServiceMonitoring-pod that will be in charge of passing the metrics gathered to our cluster monitoring service, in our case Prometheus. There the necessary metrics will be avaliable for the costumer in a simple and well organized way.

### Collecting in a self-managed scenario (ACM/MCE)
We propose to use the UWM stack on the management cluster to scrape metrics from control planes and forward them to telemetry.
This is a good fit because in terms of scalability because The UWM stack’s purpose is to collect metrics for arbitrary workloads.
There are a few challenges that needs to be addressed during the implementation:
- UWM is not enabled by default, must be enabled by changing CMO configuration.
- UWM would also need to be explicitly configured to forward metrics to telemetry. This means maintaining the list of metrics that should be forwarded. They are currently part of the CMO repository:
https://github.com/openshift/cluster-monitoring-operator/blob/de21d1e8e11fef7fa8cd9a94f0efc370ee547a5c/manifests/0000_50_cluster-monitoring-operator_04-config.yaml.
These would need to be filtered to only the ones we need for HyperShift control planes and use them to configure remote write on the UWM prometheus.
It is possible to tell UWM to skip scraping any metrics from control plane namespaces by adding a label to the project (https://docs.openshift.com/container-platform/4.10/monitoring/enabling-monitoring-for-user-defined-projects.html#excluding-a-user-defined-project-from-monitoring_enabling-monitoring-for-user-defined-projects).
- We need to re-define recording rules for UWM for telemetry metrics that come from rules. These are normally handled by the CMO and apply to the platform stack, but not the UWM stack. (medium) This adds a burden to the hypershift team to manage these rules and ensure they are in sync with what’s in the CMO repository.
- The number of metrics produced by control planes has a direct impact on resource requirements of the monitoring stack scraping them.
To have control over control planes producing too much metrics for the single UWM prometheus to handle, we will create a separate `serviceMonitor` per control plane in a different namespace that only keeps t-shirt sized metrics sets that are relevant to telemetry and tell UWM to scrape *that*.

Instead of producing a fixed number of metrics that apply to all situations, HyperShift allows configuration of a "metrics set" that identifies a set of metrics to produce per control plane.
The following metrics sets are supported:
- Telemetry - metrics needed for telemetry. This is the default and the smallest set of metrics.
- SRE - metrics in Telemetry plus those needed for service reliability monitoring of HyperShift control planes. Includes metrics necessary to produce alerts and allow troubleshooting of control plane components.
- All - all the metrics produced by standalone OCP control plane components.

- The metrics set is configured by setting the METRICS_SET environment variable in the HyperShift operator deployment:
```shell
oc set env -n hypershift deployment/operator METRICS_SET=All
```

### Collecting in a Managed service scenario (SD)
For managed HyperShift, metrics will be collected from the HyperShift control plane components by a prometheus agent and forwarded to an Observatorium instance (regular and telemetry metrics). For details see https://github.com/openshift/enhancements/pull/981.

### Metrics
#### Control Plane
See for ongoing implementation for the Metrics Sets mentioned above:
https://github.com/openshift/hypershift/pull/1517
https://github.com/openshift/hypershift/blob/e8f99157b8e37a4b1a6645e41253b2aea66fec5b/support/metrics/sets.go
https://github.com/openshift/hypershift/search?q=ServiceMonitor&type=

#### HyperShift Operator
Metrics exposed by the HyperShift operator itself should be able to answer at least the following questions:
- How many hosted clusters are there in a management clusters?
- What platform do they use? what OCP version are they using?
- How many NodePools are there in a management cluster? are they using autorepair, autoscaling? what OCP version are they using?
- Are the HostedClusters/NodePools healthy? Do they have any conditions in an undesirable state?
- How many NodePools are associated with a HostedCluster?
- How many replicas are there per NodePool?

https://github.com/openshift/hypershift/pull/1376

### Workflow Description
N/A.
### API Extensions
N/A.
### Risks and Mitigations
N/A.
### Drawbacks
N/A.
### Test Plan
N/A.
#### Dev Preview -> Tech Preview
#### Tech Preview -> GA
#### Removing a deprecated feature
### Upgrade / Downgrade Strategy
N/A.
### Version Skew Strategy
N/A.
### Operational Aspects of API Extensions
N/A.
#### Failure Modes
#### Support Procedures

## Alternatives
### Using stack in management cluster
#### Use the Platform monitoring stack
Use the cluster monitoring operator platform stack to collect metrics from control plane components. This requires that we add a label to each control plane namespace and that we add a role/rolebinding to allow the prometheus service account access to services/pods.
Pros:
- It may not be necessary to do anything explicitly to forward metrics to telemetry since the telemetry client running on the management cluster already forwards metrics to telemetry from the CMO platform stack.
- The CMO platform stack is enabled by default in all OpenShift clusters.
- It’s fairly straightforward to enable scraping of metrics for a control plane namespace.
Cons:
- All the metrics from a control plane (130k for HA control plane) may be too much for the single prometheus stack on a cluster.
- Adding non-system workload metrics to the platform prometheus is not officially supported according to our docs.
- Blocker: Currently the telemetry-client pod will overwrite the “_id” label of any metrics it sends to the telemeter service with the uuid of the cluster where it lives. If we collect control plane metrics in the platform prometheus, they will be indistinguishable from metrics for the management cluster when sent to telemetry.

#### Separate prometheus agent
Install a separate prometheus agent that is dedicated to forwarding telemetry metrics from control planes to telemetry
Pros:
- Separate prometheus can be configured with very short retention since all it will be needed for is forwarding metrics. This will alleviate the problem of a high volume of metrics.
Cons:
- We need to introduce (and operate) a component that is separate from existing prometheus.
- A separate monitoring stack is not technically supported by OpenShift.

#### Only expose metrics for telemetry
Use either any of the solutions above but only collect metrics that are relevant to telemetry which is a very small subset per control plane.
Pros:
- Solves the problem with volume of metrics.
Cons:
- Rigid.
- Need to figure out how to do this.

### Using stack in guest cluster
This is considered less secure and less reliable. Relies on a functioning data plane to send metrics to telemetry. These metrics could be potentially manipulated by owners of the guest clusters, or could not be sent if they break the monitoring stack.

#### Deploy push gateway in guest cluster and send metrics from management 
Implement a solution similar to what was done in the ibm-roks-toolkik. We deploy a push gateway to the guest cluster, then use a sidecar to push metrics from workloads in the control plane to the push gateway via the kube-apiserver service proxy.
Pros:
- There’s no scaling issues because metrics are pushed to separate prometheus stacks (in each guest cluster).
- No special networking setup is needed. The kube API server proxy can be used to reach the push gateway service inside the guest cluster.
- As long as all the metrics related to telemetry are pushed into the cluster, the telemetry client inside the guest cluster will take care of sending them to the telemetry service, no other action is needed.
Cons:
- We currently don’t include the prometheus push gateway in our release payload, we would need to add it, so it can be multi-platform compatible and can be mirrored along with other images in the payload for offline use.
- Blocker Pushing metrics like this is considered an abuse of the push gateway since it is meant to be used for short-lived processes that cannot be scraped.

#### Services accessed via the kube apiserver proxy
Configure the guest cluster prometheus to scrape metrics from the control plane through services accessed via the kube apiserver proxy.
Pros:
- Requires no additional components to be installed on the guest cluster
- Accomplishes the goal of getting telemetry metrics into the platform stack, so they can be forwarded to telemetry.
Cons:
- Blocker: It is not possible to access services in the control plane via the kube API server proxy (https://github.com/kubernetes/kubernetes/blob/master/pkg/registry/core/service/storage/storage.go#L445-L474).
- Requires creating stub services in the guest cluster that represent services that exist in the control plane. (Similar to what we do with the openshift-apiserver service).
- Requires adding more routing to the konnectivity agent running in the control plane to allow access to these control plane services.
- Scraping would be done on services and not on individual pods (which is mainly a problem for things like counters, however for telemetry metrics that may not be an issue).

#### Route that provides access to the control plane metrics
Expose a service in the control plane via route that provides access to the control plane metrics.
Pros:
- It is straight forward to implement.
- Same benefits as scraping metrics from the control plane through the kube API server proxy.
Cons:
- We need to write a new service that will expose this for all control plane components (could be accomplished via haproxy)
- Securing the endpoint is an issue. One possibility would be to use MTLS for authentication.
- Scraping would happen across clusters which is less than ideal from a monitoring solution point of view.

## Design Details

### Graduation Criteria

## Implementation History
The initial version of this doc represents implementation as delivered via MCE tech preview.