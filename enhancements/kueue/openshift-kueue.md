---
title: openshift-kueue
authors:
  - kannon92
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - TBD
approvers: # A single approver is preferred, the role of the approver is to raise important questions, help ensure the enhancement receives reviews from all applicable areas/SMEs, and determine when consensus is achieved such that the EP can move forward to implementation.  Having multiple approvers makes it difficult to determine who is responsible for the actual approval.
  - mrunalp
api-approvers: # In case of new or modified APIs or API extensions (CRDs, aggregated apiservers, webhooks, finalizers). If there is no API change, use "None"
  - joelspeed
creation-date: 2025-02-19
last-updated: 2025-02-19
tracking-link: # link to the tracking ticket (for example: Jira Feature or Epic ticket) that corresponds to this enhancement
  - https://issues.redhat.com/browse/OCPSTRAT-1641
see-also:
  - "https://github.com/openshift/enhancements/pull/1736"
replaces:
  - "NA"
superseded-by:
  - "/enhancements/our-past-effort.md"
---

# Bringing Kueue into Openshift

## Summary

We would like to bring Kueue into the core platform as a managed operator.
Kueue will be hosted as a Red Hat ecosystem operator in OperatorHub.

## Motivation

Kueue has wide applications across various projects in the openshift ecosystem. 
We want to bring them as a core platform so we can better support them in various areas. 
RedHat AI uses kueue for AI workloads but we are also seeing autoscaling, multicluster and general batch uses. 
To better support various teams in their exploration or productization of Kueue, it is important to bring Kueue into the core platform.

### High Level View of Kueue

Kueue's [website](https://kueue.sigs.k8s.io/docs/overview/) provides a list of their functionality.

Kueue provides the following functionality:

- Job Management
- Advanced Resource Management
- Integrations with common workloads
- Advanced Autoscaling support
- All-or-nothing with Ready Pods
- Partial Admission and dynamic reclaim
- Mixing Training and Inferencing
- Multi-cluster job dispatching
- Topology-Aware Scheduling

Kueue uses [Resource Flavors](https://kueue.sigs.k8s.io/docs/concepts/resource_flavor/) to describe what resources are available on a cluster to better support heterogenous clusters. Resource flavors are cluster scoped and set up by the cluster adminstrator.

Kueue uses [Cluster Queues](https://kueue.sigs.k8s.io/docs/concepts/cluster_queue/) to governs a pool of resources, defining usage limits and fair sharing rules.
Cluster Queues are cluster scoped. Cluster queues are set up by a cluster administrator.

Kueue uses [Local Queues](https://kueue.sigs.k8s.io/docs/concepts/local_queue/) to group closely related workloads belonging to a single tenant.
LocalQueues are namespaced scoped and they link with the cluster queues that a user can use for their workloads.

Internally Kueue uses a concept of a [Workload](https://kueue.sigs.k8s.io/docs/concepts/workload/) to translate k8s objects into a unit of admission.
Workload API is what Kueue uses to enforce its quota logic.
All supported frameworks of Kueue translate the objects into a Workload.

Kueue provides a choice of integration via either dedicated frameworks or via the use of pod scheduling gates.
Many support frameworks use the Suspend field and Kueue will unsuspend workloads once their is capacity in their cluster.
Kueue will typically look at all namespaces of a cluster and check if that workload should be gated by Kueue.

The use of pod scheduling gates is another approach that Kueue uses to enforce quota management. 
In this case, Kueue will add a scheduling gate if the workload is quota limited. This will gate the pod and kueue will patch the pod to release the gate.
Once that happens the pod will be scheduled as normal.
The use of the pod scheduling gate enables integrations such as Deployments, Statefulsets and LeaderWorkerSet.

Kueue liberally uses webhooks to patch workloads depending on if the namespace is labeled to support Kueue or the workload has a queue name label.

The major challange of Kueue for Openshift is configuration and support of their various frameworks and these different approaches.

### User Stories

#### Machine Learning Serving

As a LLM serving provider, I want to use Kueue to provide quota management for serving workloads. 
Cluster admins can limit access to GPUs so that a single user won’t use all the GPUs on the cluster.

Kueue provides workload support for StatefulSet, Deployments and LeaderWorkerSet. 
Kueue will allow finer control over GPUs with model servers.

Kueue [upstream ticket](https://github.com/kubernetes-sigs/kueue/issues/2717) provides more motivation.

Serving is supported by KServe in Red Hat Openshift AI (RHOAI) Serving. Kueue has a ticket to add support for KServe. 
Red Hat’s Kueue will inherit support of KServe and Kueue once the upstream ticket has been implemented.

#### Gang Scheduling

Kueue provides gang admission (admit a workload if all quota is satisfied). This can alleviate gang scheduling concerns in clusters as the workload won’t be scheduled unless it is likely to be fit in the cluster.

Users would like to run multiple pods as a single workload and they want to make sure their workload will schedule at similar times. Workloads must be scheduled all at once or the underlying pods will fail.

#### Job Quota Management

As a user, I run a CI platform where I have capacity constraints for my batch jobs. 
I want to be able to set quota limits across various tenants. 
I am using core openshift components like a batch-job. 
I would like to use Kueue and Jobs for basic job quota management.

Generally I want fair sharing, preemption and other common abilities in other job schedulers such as SLURM or LSF.

#### AI Training

As a user, I want the ability to run distributed jobs with some kind of quota management solution. 
AI users tend to use higher level workloads like Ray, Kubeflow Training Operator or JobSet. 
Kueue should provide the ability handle these workloads.

#### Scheduling Batch Workloads Across Multiple Clusters

Kueue provides support for dispatching jobs across multiple clusters. I would like to use Kueue as a way to manage quotas for batch jobs with multiple clusters.

OCM provides a POC for how to use [MultiKueue](https://github.com/open-cluster-management-io/ocm/tree/main/solutions/kueue-admission-check).

We are considering MultiKueue out of scope for tech preview.

#### Build Platforms


Workflows/Pipelines can also be resource constrained. 
One area can be the usage of Kueue’s Pod Integration to preemption workflows in Konflux. 
Openshift operators builds could take precedence over community pipelines. 
Another area can be limiting the amount of pipelines that can execute so we don’t flood the clusters with pending requests.

The internal Tekton team is looking at an integration with Kueue.

- [Upstream Issue](https://github.com/kubernetes-sigs/kueue/issues/4167)
- [Jira Epic](https://issues.redhat.com/browse/SECFLOWOTL-242)

#### Autoscaling

Kueue was designed to allow for the use of quota management with autoscaling. 
Provisioning Request (abbr. ProvReq) is a new namespaced Custom Resource that aims to allow users to ask CA for capacity for groups of pods. 
Kueue provides integration with ProvReq so that if there is quota constrains on the cluster, one could trigger ProvReq to create capacity at the workload level.

Node Autoscaling is working on ProvReq.

A kueue admin can set up a ClusterQueue so that users that have access to that cluster queue via their localqueue could trigger a ProvReq.
This ProvReq would have the cluster autoscaler create nodes that can fit the workload.

This functionality is possible once [ProvRequest in Autoscaling](https://issues.redhat.com/browse/OCPSTRAT-1331) is done.


### Goals

- Provide a Red Hat Certified Operator for Kueue Deployments
- Provide an API so that users can configure Kueue with their use case in mind.
- Kueue will be forked and maintained as [openshift/kubernetes-sig-kueue](https://github.com/openshift/kubernetes-sigs-kueue)
- [KueueOperator](https://github.com/openshift/kueue-operator) will be created to manage the installation and configuration of Kueue 


### Non-Goals

- UX improvements on the Kueue APIs as part of the operator.
    The operator is designed to deploy Kueue so that cluster admins can use the API as is. 
    
- Kueue will be a cluster wided deployment resource. We can only have 1 kueue deployed in the cluster.

- Even though MultiCluster is called out, this will be out of scope for tech preview.

- Autoscaling will be out of scope for tech prewiew.


## Proposal

We will create a Kueue Operator that will manage the installation of Kueue based on a specified Kueue configuration.

Kueue has a series of parameters to configure the operation of Kueue. 

Various use cases call for the configuration of the integrations and other kueue configurations. 


```golang
// Kueue is the Schema for the kueue API
// +k8s:openapi-gen=true
// +genclient
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
type Kueue struct {
	metav1.TypeMeta `json:",inline"`
	// metadata for kueue
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// spec holds user settable values for configuration
	// +required
	Spec KueueOperandSpec `json:"spec"`
	// status holds observed values from the cluster. They may not be overridden.
	// +optional
	Status KueueStatus `json:"status,omitempty"`
}

type KueueOperandSpec struct {
	operatorv1.OperatorSpec `json:",inline"`
	// config that is persisted to a config map
	// +required
	Config KueueConfiguration `json:"config"`
}

type ManageJobsWithoutQueueNameOption string

const (
	// NoQueueName means that all jobs will be gated by Kueue
	NoQueueName ManageJobsWithoutQueueNameOption = "NoQueueName"
	// QueueName means that the jobs require a queue label.
	QueueName ManageJobsWithoutQueueNameOption = "QueueName"
)

type KueueConfiguration struct {
	// waitForPodsReady configures gang admission
	// +optional
	WaitForPodsReady *configapi.WaitForPodsReady `json:"waitForPodsReady,omitempty"`
	// integrations are the types of integrations Kueue will manager
	// +required
	Integrations configapi.Integrations `json:"integrations"`
	// featureGates are advanced features for Kueue
	// +optional
	FeatureGates map[string]bool `json:"featureGates,omitempty"`
	// resources provides additional configuration options for handling the resources.
	// Supports https://github.com/kubernetes-sigs/kueue/blob/release-0.10/keps/2937-resource-transformer/README.md
	// +optional
	Resources *configapi.Resources `json:"resources,omitempty"`
	// ManageJobsWithoutQueueName controls whether or not Kueue reconciles
	// jobs that don't set the annotation kueue.x-k8s.io/queue-name.
	// Allowed values are NoQueueName and QueueName
	// Default will be QueueName
	// +optional
	ManageJobsWithoutQueueName *ManageJobsWithoutQueueNameOption `json:"manageJobsWithoutQueueName,omitempty"`
	// ManagedJobsNamespaceSelector can be used to omit some namespaces from ManagedJobsWithoutQueueName
	// +optional
	ManagedJobsNamespaceSelector *metav1.LabelSelector `json:"managedJobsNamespaceSelector,omitempty"`
	// FairSharing controls the fair sharing semantics across the cluster.
	FairSharing *configapi.FairSharing `json:"fairSharing,omitempty"`
}

// KueueStatus defines the observed state of Kueue
type KueueStatus struct {
	operatorv1.OperatorStatus `json:",inline"`
}
```

We will use the kueue configuration [API](https://github.com/kubernetes-sigs/kueue/blob/main/apis/config/v1beta1/configuration_types.go).

The operator reads the kueue configuration and generates a ConfigMap that the kueue manager deployment will use. 
We will only expose a limited amount of configurations.

The flow is as follows:

1) User specifies a Kueue CRD
1) Configuration is specified
1) A Configmap will be created with the configuration specified
1) Kueue deployment will use that configmap in the operand deployment.

The operator will manage these components necessary for Kueue. 
- Custom Resources
- Cluster Roles
- Cluster Role Bindings
- Services
- Mutating Webhook
- Validating Webhook
- ServiceMonitors
- APIService for visibility
- Roles
- Certificates

The operator will create and apply these resources to the cluster.

The operator has the following requirements.

a. Konflux integration
b. Support for x64 and ARM
c. Disconnected
d. FIPS

The operator will be OLM managed and hosted on OperatorHub.

### Workflow Description

#### Batch Job Administrator

As a admin, I want to enable Kueue to manage batch jobs.

```yaml
apiVersion: operator.openshift.io/v1beta1
kind: Kueue
metadata:
 name: cluster
 namespace: openshift-kueue-operator
spec:
   config:
     integrations:
       frameworks:
         - "batch/job"
```

This will create a Kueue deployment that will manage batch jobs. 

#### RHOAI Enablement

Red Hat Openshift AI is already using Kueue in production. Their deployment can be replicated:

```yaml
apiVersion: operator.openshift.io/v1beta1
kind: Kueue
metadata:
 labels:
 name: cluster
 namespace: openshift-kueue-operator
spec:
  config:
     integrations:
      frameworks:
       - "batch/job"
       - "kubeflow.org/mpijob"
       - "ray.io/rayjob"
       - "ray.io/raycluster"
       - "jobset.x-k8s.io/jobset"
       - "kubeflow.org/mxjob"
       - "kubeflow.org/paddlejob"
       - "kubeflow.org/pytorchjob"
       - "kubeflow.org/tfjob"
       - "kubeflow.org/xgboostjob"
      externalFrameworks:
       - "AppWrapper.v1beta2.workload.codeflare.dev"
```

RHOAI provides developer features and allows for enablement of more advanced features. 
Due to this, it is requested that Kueue Operator will still allow the
changing of feature gates to provide more advanced functionality.

### API Extensions

I listed the API above.

### Topology Considerations

#### Hypershift / Hosted Control Planes

We want to support Hypershift. 
This operator will allow for one to install in a namespace separate from a core openshift namespace.

#### Standalone Clusters

Yes. Standalone clusters will be able to install Kueue from OperatorHub.

#### Single-node Deployments or MicroShift


##### Microshift

To support microshift, one would need to install OLM.
Once OLM is enabled, one can install Cert Manager and the operator.
After that, the operator will also require that metrics be disabled. We will include a special flag for rare cases 
where customers would disable metrics.

Microshift does not enable openshift-monitoring so metrics would be disabled in this case.

##### Single-node Deployments

I don't forsee any issue with SNO.

### Implementation Details/Notes/Constraints

#### Release Schedule

| Kueue Operator     |  Stage   |  OCP Version   |  Kueue  | RHOAI GA
| ------------------ | ------- | ---------------| -------- | -------
| 0.1                | Tech Preview | 4.19 | 0.11.z | N
| 0.1                | Tech Preview | 4.20 | 0.13.z | Y
| ?                | GA | 4.19 | ? | Y

Kueue releases 6 times a year, roughly. 
They will have roughly 2 releases per k8s version so we can take the latest version that is built with the kubernetes version that OCP comes with.

GA release of the Kueue Operator should be with v1 apis from Kueue. We are working with upstream to get the APIs stable. 

GAing Kueue on beta APIs will cause support/upgradability issues. 
Our goal is to engage with upstream to drive stability in these APIs before we open Kueue for a general audience.
We are open to work with internal partners on GAing Kueue but upgrades may be a lot harder to guarantee.

Upstream wants to promote [v1beta1 to v1beta2](https://github.com/kubernetes-sigs/kueue/issues/768) in 2025.

[V1 Tracking issue](https://github.com/kubernetes-sigs/kueue/issues/3476)

#### Safe Enablement of Kueue for Cluster Admins

Kueue is designed to limit quota for users and to preempt based on quota. 
During the installation of a cluster or installing of critical operators, it is necessary to not have Kueue intercept these workloads. 
We do not want kueue gating GPUOperator Pods or any other components like that.

One avenue we have to make this safer is to allow for namespace opt-in. 
This will be necessary for pod integration and we are considering this for Job based integration also.

Kueue has concepts of managedJobsWithoutQueueName and managedJobsWithNamespaceSelector. 
These can add a default name for all workloads or target kueue on specific namespaces. managedJobsWithNamespaceSelector relies on a particular label for the namespace. 
Admins would be required to add a label for all kueue managed namespaces.

For tech preview, we will not enforce Kueue quotas on user workloads unless there is a kueue-managed label on the namespace.


#### Telemetry

Kueue has many useful metrics for adminstrators. We will use openshift-monitoring so that one can view metrics
via the metrics dashboard in OCP console.

The common requests from Product for telemetry are the following:

a. Who is installing Kueue from OperatorHub (Handled by OLM)

b. What kind of resources are customers using in their Cluster Queues
  Kueue has a metric called `kueue_cluster_queue_resource_usage` which will list the resources.

c. The kind of frameworks Kueue is configured with. 
We have created an [upstream issue](https://github.com/kubernetes-sigs/kueue/issues/4336) to get this as a kueue metric.

d. AI usage versus Non AI usage.

This is still up in the air but one idea is that we can use telemetry for GPU or RHOAI metrics to determine AI users versus non AI users.

#### Existing Usage of Kueue

RHOAI is a major user of Kueue and they have already released Kueue as GA. 
The integrations that are GA for RHOAI, as of writing this enhancement, are Kubeflow Training Operator and Ray.

RHOAI also uses a single release of Kueue across the latest 4 releases of Openshift (ie 4.14, 4.15, 4.16, 4.17).
A request from them for this operator is supporting Kueue across multiple releases of Openshift.

#### Feature gates

Kueue has a concept of feature gates in their configuration API. These are a series of advanced features.
The development of Kueue is quite fast and many of these features are not yet GA. We are engaging with upstream to avoid permanent betas and to focus on graduating feature gates.

Meanwhile, there are cases where one would want to test alpha features or beta features. 
To do this, we want to provide an alpha stream in OLM that will allow one to change feature gates and set non standard options.
We will achieve this by building a special alpha bundle that sets a flag in our deployment that will allow the changing of advanced functionality.

### Risks and Mitigations

Kueue is a very fast moving community. They release at least 6 times a year.
APIs are all in beta and there is some movement to graduate them. 
But the project does not have a LTS option yet.

To mitigate risk, we are engaging with upstream to define release policies and aim to graduate their critical APIs.

### Drawbacks

Not relevant here.

## Open Questions [optional]

### Autoscaling Future

Autoscaling will be a followup enhancement. 
Kueue does not provide much guidance on the safe enablement of autoscaling.
We know that we need a secure way of enabling autoscaler and we should think through that in more detail.

### RHOAI and Kueue Integration

RHOAI is using Kueue as a GA product. We still need to figure out the path with RHOAI and OCP Kueue.

## Test Plan

There are three areas we want to increase testing.

Upstream, downstream operand and the operator.

### Upstream

The Kueue Operator will depend on the Cert Manager integration. 
Kueue does not test or use heavily cert manager so we will work with upstream to add tests to
confirm cert manager functionality.

All features/bugs should be implemented and tested in Kueue upstream.

Kueue runs their e2e tests on all supported versions of Kubernetes (n-3) with its two latest releases.

#### Downstream Operand

Kueue operand will need to be tested on all supported versions of Openshift that the operator will support.
We have to change the operand to be compliant with Red Hat policies so we should run the upstream tests with the operand images.

#### Operator

There will be e2e tests testing the installion of the operator based on user provided configurations.
The operator will also test the installation of Kueue and the removing of Kueue based on OLM bundles.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Konflux releases of operator
- E2E Testing
- Operator functionality complete
- Documentation
- Telemetry

### RHOAI GA Adoption

- RHOAI is able to switch their Kueue deployment for Openshift Kueue
- Feature parity with their existing solution
- We are discussing them adopting this as GA before we release the operator as GA for everyone.

### Tech Preview -> GA

- Once kueue APIs are V1 we can GA the kueue operator.

### Removing a deprecated feature

This is not relevant.

## Upgrade / Downgrade Strategy

This is still in flight and we are figuring out the upgrade strategy.


## Version Skew Strategy

Kueue has tight binding with the Kubernetes API so we recommend following Kubernetes policies.

0.11.0 of Kueue is built with 1.32 APIs. We would recommend that this Kueue operator work with n-3 (ie 1.29 at the edge).

## Operational Aspects of API Extensions

Fill this out.

Describe the impact of API extensions (mentioned in the proposal section, i.e. CRDs,
admission and conversion webhooks, aggregated API servers, finalizers) here in detail,
especially how they impact the OCP system architecture and operational aspects.

- For conversion/admission webhooks and aggregated apiservers: what are the SLIs (Service Level
  Indicators) an administrator or support can use to determine the health of the API extensions

  Examples (metrics, alerts, operator conditions)
  - authentication-operator condition `APIServerDegraded=False`
  - authentication-operator condition `APIServerAvailable=True`
  - openshift-authentication/oauth-apiserver deployment and pods health

- What impact do these API extensions have on existing SLIs (e.g. scalability, API throughput,
  API availability)

  Examples:
  - Adds 1s to every pod update in the system, slowing down pod scheduling by 5s on average.
  - Fails creation of ConfigMap in the system when the webhook is not available.
  - Adds a dependency on the SDN service network for all resources, risking API availability in case
    of SDN issues.
  - Expected use-cases require less than 1000 instances of the CRD, not impacting
    general API throughput.

- How is the impact on existing SLIs to be measured and when (e.g. every release by QE, or
  automatically in CI) and by whom (e.g. perf team; name the responsible person and let them review
  this enhancement)

- Describe the possible failure modes of the API extensions.
- Describe how a failure or behaviour of the extension will impact the overall cluster health
  (e.g. which kube-controller-manager functionality will stop working), especially regarding
  stability, availability, performance and security.
- Describe which OCP teams are likely to be called upon in case of escalation with one of the failure modes
  and add them as reviewers to this enhancement.

## Support Procedures

TODO: Fill this out as we proceed.

Describe how to
- detect the failure modes in a support situation, describe possible symptoms (events, metrics,
  alerts, which log output in which component)

  Examples:
  - If the webhook is not running, kube-apiserver logs will show errors like "failed to call admission webhook xyz".
  - Operator X will degrade with message "Failed to launch webhook server" and reason "WehhookServerFailed".
  - The metric `webhook_admission_duration_seconds("openpolicyagent-admission", "mutating", "put", "false")`
    will show >1s latency and alert `WebhookAdmissionLatencyHigh` will fire.

- disable the API extension (e.g. remove MutatingWebhookConfiguration `xyz`, remove APIService `foo`)

  - What consequences does it have on the cluster health?

    Examples:
    - Garbage collection in kube-controller-manager will stop working.
    - Quota will be wrongly computed.
    - Disabling/removing the CRD is not possible without removing the CR instances. Customer will lose data.
      Disabling the conversion webhook will break garbage collection.

  - What consequences does it have on existing, running workloads?

    Examples:
    - New namespaces won't get the finalizer "xyz" and hence might leak resource X
      when deleted.
    - SDN pod-to-pod routing will stop updating, potentially breaking pod-to-pod
      communication after some minutes.

  - What consequences does it have for newly created workloads?

    Examples:
    - New pods in namespace with Istio support will not get sidecars injected, breaking
      their networking.

- Does functionality fail gracefully and will work resume when re-enabled without risking
  consistency?

  Examples:
  - The mutating admission webhook "xyz" has FailPolicy=Ignore and hence
    will not block the creation or updates on objects when it fails. When the
    webhook comes back online, there is a controller reconciling all objects, applying
    labels that were not applied during admission webhook downtime.
  - Namespaces deletion will not delete all objects in etcd, leading to zombie
    objects when another namespace with the same name is created.

## Alternatives

None I can think of.

## Infrastructure Needed [optional]

- New repo added for Kueue Operand
- New repo added for kueue operator
- 
