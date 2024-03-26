---
title: enable-evented-pleg-in-openshift
authors:
  - "@sairameshv"
reviewers:
  - "@mrunalp"
  - "@harche"
  - "@rphillips"
  - "@kikisdeliveryservice"
  - "@sinnykumari"
  - "@yuqi-zhang"
approvers:
  - "@mrunalp"
  - "@sinnykumari"
api-approvers:
  - "@sttts"
creation-date: 2023-03-09
last-updated: 2023-04-06
tracking-link:
  - https://issues.redhat.com/browse/OCPNODE-1525
see-also:
  - Upstream Evented PLEG enhancement:
---
# Enable evented pleg in the Openshift clusters

## Summary

Evented PLEG enablement has progressed to beta in the
[upstream](https://github.com/kubernetes/enhancements/tree/master/keps/sig-node/3386-kubelet-evented-pleg) Kubernetes.
The kubelet and the underlying runtime (cri-o) are now capable of enabling the evented pleg feature.
The evented pleg will *not* be enabled by default in the OCP clusters as
this feature is not yet enabled [by default](https://github.com/kubernetes/enhancements/blob/master/keps/sig-node/3386-kubelet-evented-pleg/README.md#beta-enabled-by-default) in the upstream until a few more tests are thoroughly performed and a good amount of benchmarking data is captured.

## Motivation

Evented pleg requires enabling of the flags present in both the Kubelet and the cri-o command line arguments.
So, one of the motivations of this proposal is to make this process easier. User can update both the above configurations by just updating a single field of a custome resource object. 
Other main motive is that the Evented PLEG feature can play an important role in the Kubelet's performance (compared to the existing Generic PLEG) in the presence of huge number of pods on a given node.
Hence, this feature pushes the Openshift's ability to achieve greater pod density on a node. 


### User Stories

> "As an OCP user, I want to enable the evented pleg feature in the Openshift clusters so that the Kubelet's performance while determining the pod life-cycle improves"

### Goals
- Support/Enable evented pleg feature in the Openshift 
### Non-Goals

- Eliminate the Generic PLEG completely and solely rely on the Evented PLEG

## Proposal

An option to enable the evented pleg will have to reside in a node specific configuration object as the enabling requires the configuration changes in the Kubelet, CRI-O configs.
[OpenShift Node config](https://github.com/openshift/api/blob/master/config/v1/types_node.go#L19) (nodes.config.openshift.io) object contains some of the node related configurations such as CgroupMode, WorkerlatencyProfiles.
This nodes.config object can be a good fit to have an extra field to rely on enabling the evented pleg feature.
This object is monitored by the MCO in order to make the change in the kubelet as well as cri-o configuration on all the nodes of the cluster.

### Workflow Description
- Add an extra field to enable evented pleg in the nodes.config API object. 
- Monitor the above field via Kubelet Config controller and the Container Runtime controller present in the MCO
- Machine config objects get created with the updated kubelet and the cri-o configs.
- Openshift Cluster nodes get rebooted and updated with the evented pleg feature enabled. 

### API Extensions

Create an additional field within the nodes.config object named `EventedPleg` :

```go
package v1

type Node struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds user settable values for configuration
	// +kubebuilder:validation:Required
	// +required
	Spec NodeSpec `json:"spec"`

	// status holds observed values.
	// +optional
	Status NodeStatus `json:"status"`
}


type NodeSpec struct {
	// CgroupMode

	// WorkerLatencyProfile

	// EventedPleg determines whether to enable the evented pleg feature to
	// send the container events from the CRI-O through gRPC to the Kubelet
	// +optional
	EventedPleg *bool `json:"eventedPleg,omitempty"`
}

```
The following machine configs are newly rendered or updated(if already present) when there is an update made to the above-mentioned `EventedPleg` field.
- `97-master-generated-kubelet`
- `97-worker-generated-kubelet`
- `97-master-generated-crio-enable-pod-events`
- `97-worker-generated-crio-enable-pod-events`
The kubelet and the cri-o configurations on the nodes are updated as follows when `EventedPleg` is set to `true`:
1. Kubelet Configuration:
```json
{
  "kind": "KubeletConfiguration",
  "apiVersion": "kubelet.config.k8s.io/v1beta1",
  "featureGates": {
    "EventedPLEG": true,
  },
}
```
2. CRI-O Configuration:
```yaml
[crio]
  [crio.runtime]
    enable_pod_events = true
```

### Risks and Mitigations

- PLEG is very core to the container status handling in the kubelet. Hence any miscalculation there would result in unpredictable behaviour not just for the node but for an entire cluster.
  - To reduce the risk of regression, this feature initially will be available only as an opt-in.
  - Users can disable this feature to make kubelet use existing relisting based PLEG.
- Another risk is the CRI implementation could have a buggy event emitting system, and miss pod lifecycle events.
  - A mitigation is a `kube_pod_missed_events` metric, which the Kubelet could report when a lifecycle event is registered that wasn't triggered by an event, but rather by changes of state between lists.
  - While using the Evented implementation, the periodic relisting functionality would still be used with an increased interval which should work as a fallback mechanism for missed events in case of any disruptions.
  - Evented PLEG will need to update global cache timestamp periodically in order to make sure pod workers don't get stuck at [GetNewerThan](https://github.com/kubernetes/kubernetes/blob/4a894be926adfe51fd8654dcceef4ece89a4259f/pkg/kubelet/pod_workers.go#L924) in case Evented PLEG misses the event for any unforeseen reason.

### Drawbacks

This feature requires changes in the code base of the PLEG module that adjusts the container runtime state, accordngly updates the pod cache.
Kubelet depends on the events generated by the container runtimes(Evented PLEG) to update the pod cache.
Sometimes, there is a chance of events getting missed and hence relying alone on the evented pleg, without having sufficient benchmark data, is not recommended.

One more minor drawback of this feature is that the nodes may get updated/rebooted twice with the two newly rendered machine configs
i.e. one due to the Kubelet Config Controller and the second due to the Container Runtime Config Controller of the MCO.

## Design Details

### Test Plan
- Unit tests must be added to the MCO that monitors the newly added field, updates the kubelet, cri-o configurations.
- e2e bootstrap related testcases must be added to compare the newly rendered, bootstrapped machine config objects.
- An openshift CI job must be added to test the node e2e tests by enabling the evented pleg feature.

### Graduation Criteria
#### Dev Preview -> Tech Preview
`Evented PLEG` will be a tech preview feature on its initial release as this is a featuregated, beta feature in upstream.
There must be a sufficient internal testing performed along with the gathering of the customer feedback before graduating this feature to GA.
#### Tech Preview -> GA
Graduation requirements to GA are:

* No regressions from the Generic PLEG to Evented PLEG
* Processes are correctly OOMed
* No performance issues - PSAP and QE teams will be asked to test their suites for regressions
* Metrics are accurate and correctly submitted to monitoring
* Upstream feature graduation to GA
* Add blocking upgrade ci jobs with the evented pleg feature enabled
* OpenShift's evented pleg enabled upgrade jobs pass percentage is similar or better than the OpenShift's Generic PLEG(default) upgrade job pass percentage

#### Removing a deprecated feature
N/A
### Upgrade / Downgrade Strategy
Upgrading an old cluster to a new cluster supporting the evented pleg feature should work without any issue.
Downgrading a cluster to an OpenShift version not containing the evented pleg support is still expected to work normal.

### Version Skew Strategy

N/A

### Operational Aspects of API Extensions

N/A

#### Failure Modes
##### Configuration update failure
- Both of the kubelet and the cri-o have their own stand-alone way of enabling the evented pleg feature flag. The cluster can still be stable
by enabling the feature flag in any one of them. 
- The `nodes.config.openshift.io` object is monitored by both the kubelet-config and the container runtime controllers of the MCO. 
- Each of the controllers are responsible for updating the respective configurations. 
- There could be a possibility of one of the controllers failing to monitor and update the respective configuration. 
- In such case, only one of the kubelet or cri-o is enabled with the evented pleg feature flag which is still acceptable.
##### Missed Events
- Events generated by the container runtime can be missed and hence the container statuses observed at the CRI, Kubelet's pod cache may not be consistent.

#### Support Procedures
- The support procedures applicable to the MCO failure modes are applicable as the configuration updates happen in the form of the creation/update of the Machine Config objects. 

## Implementation History


## Alternatives
The Kubelet PLEG can be made to utilize the events from cadvisor as well. But we are trying to reduce the kubelet's dependency on cadvisor so that option is not viable. This is also discussed in the older
[enhancement](https://github.com/kubernetes/community/blob/4026287dc3a2d16762353b62ca2fe4b80682960a/contributors/design-proposals/node/pod-lifecycle-event-generator.md#leverage-upstream-container-events) in detail.