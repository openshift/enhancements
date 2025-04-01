---
title: openshift-kueue
authors:
  - kannon92
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - haircommander 
  - rphillips
  - varshaprasad96 #rhoai expert
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
  - "NA"
---

# Bringing Kueue into Openshift

## Summary

We would like to bring Kueue into the core platform as a managed operator.
Kueue will be hosted as a Red Hat ecosystem operator in OperatorHub.

## Motivation

Kueue has wide applications across various projects in the openshift ecosystem. 
We want to bring them as a core platform so we can better support them in various areas. 
Openshift AI (RHOAI) uses kueue for AI workloads but we are also seeing autoscaling, multicluster and general batch uses. 
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

Kueue uses [Resource Flavors](https://kueue.sigs.k8s.io/docs/concepts/resource_flavor/) to describe what resources are available on a cluster to better support heterogenous clusters. 
Resource flavors are cluster scoped and set up by the cluster adminstrator.

Kueue uses [Cluster Queues](https://kueue.sigs.k8s.io/docs/concepts/cluster_queue/) to governs a pool of resources, defining usage limits and fair sharing rules.
Cluster Queues are cluster scoped. Cluster queues are set up by a cluster administrator.

Kueue uses [Local Queues](https://kueue.sigs.k8s.io/docs/concepts/local_queue/) to group closely related workloads belonging to a single tenant.
LocalQueues are namespaced scoped and they link with the cluster queues that a user can use for their workloads.

Internally Kueue uses a concept of a [Workload](https://kueue.sigs.k8s.io/docs/concepts/workload/) to translate k8s objects into a unit of admission.
Workload API is what Kueue uses to enforce its quota logic.
All supported frameworks of Kueue translate the objects into a Workload.

Kueue provides a choice of integration via either dedicated frameworks or via the use of pod scheduling gates.
Many supported frameworks use the Suspend field and Kueue will unsuspend workloads once their is capacity in their cluster.
Kueue will typically look at all namespaces of a cluster and check if that workload should be gated by Kueue.

The use of pod scheduling gates is another approach that Kueue uses to enforce quota management. 
In this case, Kueue will add a scheduling gate via a mutating webhook if the workload is quota limited. This will gate the pod and kueue will patch the pod to release the gate.
Once that happens the pod will be scheduled as normal.
The use of the pod scheduling gate enables integrations such as Deployments, Statefulsets and LeaderWorkerSet.

Kueue uses webhooks to patch workloads depending on if the namespace is labeled to support Kueue or the workload has a queue name label.
If an admin wants all workloads to be supported by Kueue, then the webhook would patch
the label of the pod or workload.
If Kueue determines the workload to not fit quota, then the workload is either suspended (via `suspend:true` on the workload spec)
or a scheduling gate is added to the pod.

The major challenge of Kueue for Openshift is configuration and support of their various frameworks and these different approaches.

#### RBAC For Kueue

[Kueue Upstream Docs](https://kueue.sigs.k8s.io/docs/tasks/manage/rbac/) cover this pretty well.

Kueue has the concept of a Kueue admin and a kueue user. 

A kueue admin has the ability to create ClusterQueues, Queues, ResourceFlavors and Workloads.

A kueue user has the ability to manage general jobs (batch/jobs, ray, kubeflow etc) and to view Queues and Workloads.
A kueue user would request the admin to create a localqueue in their namespace.

This separation is important because when a user creates a LocalQueue they are able to link to a 
ClusterQueue. ClusterQueues can control things like resource quotas and autoscaling so
localqueues are usually the job of a kueue-admin to create in the user namespace.

Our operator deploys aggregated cluster roles for `kueue-admin` and `batch-user`.

An admin can bind to the kueue-admin clusterroles to get "admin" access for Kueue.

A user would wants to use Kueue can bind to the "batch-user" so they can view their queue
and their workloads.

#### Kueue Upstream Engagement

The operator and the operand will be focused on deploying Kueue into Openshift.
Features and functionality will be implemented in upstream first and we can
use those features once the upstream community has released them.

We want to avoid carrying patches/features in downstream Kueue as this will cause headaches down the road.

### User Stories

#### Machine Learning Serving

As a LLM serving provider, I want to use Kueue to provide quota management for serving workloads. 
Cluster admins can limit access to GPUs so that a single user won’t use all the GPUs on the cluster.

Kueue provides workload support for StatefulSet, Deployments and LeaderWorkerSet. 
Kueue will allow finer control over GPUs with model servers.

Kueue [upstream ticket](https://github.com/kubernetes-sigs/kueue/issues/2717) provides more motivation.

Serving is supported by KServe in Red Hat Openshift AI (RHOAI) Serving. Kueue has a ticket to add support for KServe. 
Red Hat’s Kueue will inherit support of KServe and Kueue once the upstream ticket has been implemented.

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

We are considering MultiKueue out of scope for this enhancement.

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

This will be out of scope for tech preview.

### Goals

- Provide a Red Hat Certified Operator for Kueue Deployments
- Provide an API so that users can configure Kueue with their use case in mind.
- Kueue will be forked and maintained as [openshift/kubernetes-sig-kueue](https://github.com/openshift/kubernetes-sigs-kueue)
- [KueueOperator](https://github.com/openshift/kueue-operator) will be created to manage the installation and configuration of Kueue 

### Non-Goals

- UX improvements on the Kueue APIs as part of the operator.
    The operator is designed to deploy Kueue so that cluster admins can use the API as is. 
    
- Kueue will be a cluster wide deployed resource. We can only have 1 kueue deployed in the cluster.

- Even though MultiCluster is called out, this will be out of scope for tech preview.

- Autoscaling will be out of scope for this enhancement.


## Proposal

We will create a Kueue Operator that will manage the installation of Kueue based on a specified Kueue configuration.

Kueue has a series of parameters to configure the operation of Kueue. 

Various use cases call for the configuration of the integrations and other kueue configurations. 

We will use the kueue configuration [API](https://github.com/kubernetes-sigs/kueue/blob/main/apis/config/v1beta1/configuration_types.go).

The operator reads the kueue configuration and generates a ConfigMap that the kueue manager deployment will use. 

The API is displayed below but we have requested an api-review [here](https://github.com/openshift/api/pull/2222).

During our api-review process, we decided to minimize the scope of our tech preview. 
We will only focus on integrations so we can enable user stories explained above.

We will use the kueue defaults for many of the features.

```golang
package v1alpha1

import (
	operatorv1 "github.com/openshift/api/operator/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// Kueue is the CRD to represent the Kueue operator
// This CRD defines the configuration that the Kueue
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
// +kubebuilder:object:root=true
// +kubebuilder:resource:path=kueue,scope=Cluster
// +k8s:openapi-gen=true
// +genclient
// +genclient:nonNamespaced
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
// +kubebuilder:validation:XValidation:rule="self.metadata.name == 'cluster'",message="Kueue is a singleton, .metadata.name must be 'cluster'"
type Kueue struct {
	...
	// spec holds user settable values for configuration
	// +required
	Spec KueueOperandSpec `json:"spec"`
	// status holds observed values from the cluster. They may not be overridden.
	// +optional
	Status KueueStatus `json:"status,omitempty"`
}

type KueueOperandSpec struct {
	operatorv1.OperatorSpec `json:",inline"`
	// config is the desired configuration
	// for the Kueue operator.
	// +required
	Config KueueConfiguration `json:"config"`
}

type KueueConfiguration struct {
	// integrations is a required field that configures the Kueue's workload integrations.
	// Kueue has both standard integrations, known as job frameworks, and external integrations known as external frameworks.
	// Kueue will only manage workloads that correspond to the specified integrations.
	// +required
	Integrations Integrations `json:"integrations"`
	// queueLabelPolicy controls how kueue manages workloads
	// The default behavior of Kueue will manage workloads that have a queue-name label.
	// This field is optional.
	// +optional
	QueueLabelPolicy QueueLabelPolicy `json:"queueLabelPolicy,omitempty"`
	// kueueGangSchedulingPolicy controls how Kueue admits workloads.
	// Gang Scheduling is the act of all or nothing scheduling.
	// Kueue provides this ability.
	// This field is optional.
	// +optional
	KueueGangSchedulingPolicy KueueGangSchedulingPolicy `json:"kueueGangSchedulingPolicy,omitempty"`
	// premption is the process of evicting one or more admitted Workloads to accommodate another Workload.
	// Kueue has classical premption and preemption via fair sharing.
	// +optional
	Premption Premption `json:"premption,omitempty"`
}

// KueueStatus defines the observed state of Kueue
type KueueStatus struct {
	operatorv1.OperatorStatus `json:",inline"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object

// KueueList contains a list of Kueue
//
// Compatibility level 4: No compatibility is provided, the API can change at any point for any reason. These capabilities should not be used by applications needing long term support.
// +openshift:compatibility-gen:level=4
type KueueList struct {
	metav1.TypeMeta `json:",inline"`
	// metadata for the list
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	// items is a slice of Kueue
	// this is a cluster scoped resource and there can only be 1 Kueue
	// +kubebuilder:validation:MaxItems=1
	// +required
	Items []Kueue `json:"items"`
}

// +kubebuilder:validation:Enum=BatchJob;RayJob;RayCluster;JobSet;MPIJob;PaddleJob;PytorchJob;TFJob;XGBoostJob;AppWrapper;Pod;Deployment;StatefulSet;LeaderWorkerSet
type KueueIntegration string

const (
	KueueIntegrationBatchJob        KueueIntegration = "BatchJob"
	KueueIntegrationRayJob          KueueIntegration = "RayJob"
	KueueIntegrationRayCluster      KueueIntegration = "RayCluster"
	KueueIntegrationJobSet          KueueIntegration = "JobSet"
	KueueIntegrationMPIJob          KueueIntegration = "MPIJob"
	KueueIntegrationPaddeJob        KueueIntegration = "PaddeJob"
	KueueIntegrationPyTorchJob      KueueIntegration = "PyTorchJob"
	KueueIntegrationTFJob           KueueIntegration = "TFJob"
	KueueIntegrationXGBoostJob      KueueIntegration = "XGBoostJob"
	KueueIntegrationAppWrapper      KueueIntegration = "AppWrapper"
	KueueIntegrationPod             KueueIntegration = "Pod"
	KueueIntegrationDeployment      KueueIntegration = "Deployment"
	KueueIntegrationStatefulSet     KueueIntegration = "StatefulSet"
	KueueIntegrationLeaderWorkerSet KueueIntegration = "LeaderWorkerSet"
)

// This is the GVR for an external framework.
// Controller runtime requires this in this format
// for api discoverability.
type ExternalFramework struct {
	// group is the API group of the externalFramework.
	// Must be a valid DNS 1123 subdomain consisting of of lower-case alphanumeric characters,
	// hyphens and periods, of at most 253 characters in length.
	// Each period separated segment within the subdomain must start and end with an alphanumeric character.
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self.size() == 0 || !format.dns1123Label().validate(self).hasValue()",message="a lowercase RFC 1123 subdomain must consist of lower case alphanumeric characters, '-' or '.', and must start and end with an alphanumeric character."
	// +required
	Group string `json:"group"`
	// resource is the Resource type of the external framework.
	// Resource types are lowercase and plural (e.g. pods, deployments).
	// Must be a valid DNS 1123 label consisting of a lower-case alphanumeric string
	// and hyphens of at most 63 characters in length.
	// The value must start and end with an alphanumeric character.
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self.size() == 0 || !format.dns1123Label().validate(self).hasValue()",message="a lowercase RFC 1123 label must consist of lower case alphanumeric characters and '-', and must start and end with an alphanumeric character."
	// +required
	Resource string `json:"resource"`
	// version is the version of the api (e.g. v1alpha1, v1beta1, v1).
	// Must be a valid DNS 1035 label consisting of a lower-case alphanumeric string
	// and hyphens of at most 63 characters in length.
	// The value must start with an alphabetic character and end with an alphanumeric character.
	// +kubebuilder:validation:MaxLength=63
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="self.size() == 0 || !format.dns1035Label().validate(self).hasValue()",message="a lowercase RFC 1035 label must consist of lower case alphanumeric characters, '-' or '.', and must start with an alphabetic character and end with an alphanumeric character."
	// +required
	Version string `json:"version"`
}

// This is the integrations for Kueue.
// Kueue uses these apis to determine
// which jobs will be managed by Kueue.
type Integrations struct {
	// frameworks are a unique list of names to be enabled.
	// This is required and must have at least one element.
	// Each framework represents a type of job that Kueue will manage.
	// Frameworks are a list of frameworks that Kueue has support for.
	// The allowed values are BatchJob, RayJob, RayCluster, JobSet, MPIJob, PaddleJob, PytorchJob, TFJob, XGBoostJob, AppWrapper, Pod, Deployment, StatefulSet and LeaderWorkerSet.
	// +kubebuilder:validation:MaxItems=14
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:XValidation:rule="self.all(x, self.exists_one(y, x == y))",message="each item in frameworks must be unique"
	// +listType=atomic
	// +required
	Frameworks []KueueIntegration `json:"frameworks"`
	// externalFrameworks are a list of GroupVersionResources
	// that are managed for Kueue by external controllers.
	// These are optional and should only be used if you have an external controller
	// that integrates with Kueue.
	// +listType=atomic
	// +kubebuilder:validation:MaxItems=32
	// +optional
	ExternalFrameworks []ExternalFramework `json:"externalFrameworks,omitempty"`
	// labelKeysToCopy are a list of label keys that are copied once a workload is created.
	// These keys are persisted to the internal Kueue workload object.
	// If not specified, only the Kueue labels will be copied.
	// +kubebuilder:validation:MaxItems=64
	// +listType=atomic
	// +optional
	LabelKeysToCopy []LabelKeys `json:"labelKeysToCopy,omitempty"`
}

type LabelKeys struct {
	// key is the label key
	// A label key must be a valid qualified name consisting of a lower-case alphanumeric string,
	// and hyphens of at most 63 characters in length.
	// The name must start and end with an alphanumeric character.
	// The name may be optionally prefixed with a subdomain consisting of lower-case alphanumeric characters,
	// hyphens and periods, of at most 253 characters in length.
	// Each period separated segment within the subdomain must start and end with an alphanumeric character.
	// The optional prefix and the name are separate by a forward slash (/).
	// +kubebuilder:validation:MaxLength=317
	// +kubebuilder:validation:MinLength=1
	// +kubebuilder:validation:XValidation:rule="!format.qualifiedName().validate(self).hasValue()",message="a qualified name must consist of a lower-case alphanumeric and hyphenated string of at most 63 characters in length, starting and ending with alphanumeric chracters. The name may be optionally prefixed with a subdomain consisting of lower-case alphanumeric characters, hyphens and periods, of at most 253 characters in length. Each period separated segment within the subdomain must start and end with an alphanumeric character."
	// +optional
	Key string `json:"key,omitempty"`
}

// +kubebuilder:validation:Enum=ByWorkload;Disabled
type KueueGangSchedulingPolicyOptions string

const (
	KueueGangSchedulingPolicyEvictNotReadyWorkloads KueueGangSchedulingPolicyOptions = "ByWorkload"
	KueueGangSchedulingPolicyDisabled               KueueGangSchedulingPolicyOptions = "Disabled"
)

// +kubebuilder:validation:Enum=Parallel;Sequential
type KueueGangSchedulingAdmissionOptions string

const (
	KueueGangSchedulingAdmissionOptionsSequential KueueGangSchedulingAdmissionOptions = "Sequential"
	KueueGangSchedulingAdmissionOptionsParallel   KueueGangSchedulingAdmissionOptions = "Parallel"
)

// Kueue provides the ability to admit workloads all in one (gang admission)
// and evicts workloads if they are not ready within a specific time.
type KueueGangSchedulingPolicy struct {
	// policy allows for changing the kinds of gang scheduling Kueue does.
	// This is an optional field.
	// The allowed values are ByWorkload and Disabled.
	// The default value will be Disabled.
	// ByWorkload allows for configuration how admission is performed
	// for Kueue.
	// +optional
	Policy KueueGangSchedulingPolicyOptions `json:"policy"`
	// byWorkload controls how admission is done.
	// When admission is set to Sequential, only pods from the currently processing workload will be admitted.
	// Once all pods from the current workload are admitted, and ready, Kueue will process the next workload.
	// Sequential processing may slow down admission when the cluster has sufficient capacity for multiple workloads,
	// but provides a higher guarantee of workloads scheduling all pods together successfully.
	// When set to Parallel, pods from any workload will be admitted at any time.
	// This may lead to a deadlock where workloads are in contention for cluster capacity and
	// pods from another workload having successfully scheduled prevent pods from the current workload scheduling.
	// +kubebuilder:validation:XValidation:rule="self.policy==ByWorkload",message="byWorkload is only valid if policy equals ByWorkload"
	// +optional
	ByWorkload KueueGangSchedulingAdmissionOptions `json:"byWorkload"`
}

// +kubebuilder:validation:Enum=QueueNameRequired;QueueNameOptional
type QueueLabelNamePolicy string

const (
	QueueLabelNamePolicyRequired QueueLabelNamePolicy = "QueueNameRequired"
	QueueLabelNamePolicyOptional QueueLabelNamePolicy = "QueueNameOptional"
)

type QueueLabelPolicy struct {
	// queueLabelPolicy controls whether or not Kueue reconciles
	// jobs that don't set the label kueue.x-k8s.io/queue-name.
	// The allowed values are QueueNameRequired and QueueNameOptional.
	// If set to QueueNameRequired, then those jobs will be suspended and never started unless
	// they are assigned a queue and eventually admitted. This also applies to
	// jobs created before starting the kueue controller.
	// Defaults to QueueNameRequired; therefore, those jobs are not managed and if they are created
	// unsuspended, they will start immediately.
	// +optional
	QueueLabelPolicy QueueLabelNamePolicy `json:"queueLabelPolicy"`
}

// +kubebuilder:validation:Enum=Classical;FairSharing
type PreemptionStrategy string

const (
	PreemeptionStrategyClassical   PreemptionStrategy = "Classical"
	PreemeptionStrategyFairsharing PreemptionStrategy = "FairSharing"
)

type Preemption struct {
	// preemptionStrategy are the types of preemption kueue allows.
	// Kueue has two types of preemption: classical and fair sharing.
	// Classical means that an incoming workload, which does
	// not fit within the unusued quota, is eligible to issue preemptions
	// when the requests of the workload are below the
	// resource flavor's nominal quota or borrowWithinCohort is enabled
	// on the Cluster Queue.
	// FairSharing is a more heavy weight algorithm.
	// ClusterQueues with pending Workloads can preempt other Workloads
	// in their cohort until the preempting ClusterQueue
	// obtains an equal or weighted share of the borrowable resources.
	// The borrowable resources are the unused nominal quota
	// of all the ClusterQueues in the cohort.
	// +optional
	PreemptionStrategy PreemptionStrategy `json:"preemptionStrategy"`
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
- Issuers

The operator will create and apply these resources to the cluster.

The operator has the following requirements.

- Konflux integration
- Support for x64 and ARM
- Disconnected
- FIPS

The operator will be OLM managed and hosted on OperatorHub.

### Feature Support

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
         - BatchJob
```

This will create a Kueue deployment that will manage batch jobs. 

#### Core Openshift Management

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: Kueue
metadata:
  labels:
    app.kubernetes.io/name: kueue-operator
    app.kubernetes.io/managed-by: kustomize
  name: cluster
  namespace: openshift-kueue-operator
spec:
  managementState: Managed
  config:
    integrations:
      frameworks:
      - BatchJob
      - Pod
      - Deployment
      - StatefulSet 
```

This can be used for Kueue to manage all workloads.
A popular request is for Kueue to manage the access of GPUs for Model Serving.

#### RHOAI Enablement

Red Hat Openshift AI is already using Kueue in production. Their deployment can be replicated:

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: Kueue
metadata:
  labels:
    app.kubernetes.io/name: kueue-operator
    app.kubernetes.io/managed-by: kustomize
  name: cluster
  namespace: openshift-kueue-operator
spec:
  managementState: Managed
  config:
    integrations:
      frameworks:
      - RayJob
      - RayCluster
      - PyTorchJob
    kueueGangSchedulingPolicy:
      policy: ByWorkload
      byWorkload: Parallel
    queueLabelPolicy:
      queueLabelPolicy: QueueNameRequired
    preemption:
      preemptionStrategy: Classical
```

#### IBM Enablement

```yaml
apiVersion: operator.openshift.io/v1alpha1
kind: Kueue
metadata:
  labels:
    app.kubernetes.io/name: kueue-operator
    app.kubernetes.io/managed-by: kustomize
  name: cluster
  namespace: openshift-kueue-operator
spec:
  managementState: Managed
  config:
    integrations:
      frameworks:
      - AppWrapper
    kueueGangSchedulingPolicy:
      policy: Disabled
    queueLabelPolicy:
      queueLabelPolicy: QueueNameOptional
    preemption:
      preemptionStrategy: FairSharing
```

### API Extensions

Our operator will have a Kueue CRD that will trigger the installation of Kueue.

Kueue operand has these CRDs:

- ClusterQueue (Tier 1)
- AdmissionChecks (Tier 1)
- LocalQueue (Tier 1)
- Workloads (Tier 1)
- ResourceFlavors (Tier 1)
- WorkloadPriorityClasses (Tier 1)
- ProvisioningRequestConfigs (Not Supported)
- MultiKueueConfigs (Not Supported)
- MultiKueueClusters (Not Supported)
- Cohorts (alpha - Not Supported)
- Topologies (alpha - Not supported)

Tier 1 APIs are considered GA and supported.
These are v1beta1 in Kueue as of kueue v0.11.0,
but they are more stable than others.

Kueue has also validating and mutating webhooks for these CRDs and
it also mutates/validates core kubernetes resources such as pods, deployments, statefulsets and jobs.

Kueue provides a series of metrics that we will expose into `openshift-monitoring`.

For visibility on demand, Kueue also provides an APIService to define a custom apiservice.

### Topology Considerations

#### Hypershift / Hosted Control Planes

Kueue can be hosted either on worker cluster or in the hosted control plane.

We do not see a usecase yet to have Kueue on the hosted control plane but it is possible to install Kueue on the hosted control plane.

##### Webhooks

Kueue has validating and mutating webhooks that run across multiple namespaces.
To reduce the burden on Openshift, we are going to follow an opt-in approach for
workloads. Webhooks will only be applied on namespaces that have a kueue-managed label on the namespace.
This way we reduce the impact of webhooks across multiple namespaces.

Hypershift can scale down the worker nodes (scale to zero) but the webhooks are still registered.
Having namespace opt in can help here as we will not apply webhooks unless there is a namespace that has a label.

Kueue manages webhooks via a deployment and a service. 
To have high availability on the webhooks, we provide the ability to deploy
Kueue with multiple replicas.

Kueue webhooks are related to a service so one can monitor the webhooks service
to determine if the webhooks are done.

##### Machines

Kueue will only create machines in the context of Provisioning Requests.
There is work ongoing to implement Provising Requests in Hypershift.
Once this is complete, Kueue can use this for autoscaling.

We do not foresee any issue with integration with HCP.
Kueue does provide webhooks on pods and that could cause some 
performance issues in HCP. 
We will do performance testing on HCP. 

Kueue is an upstream project and its APIs are not aware of MachineSets.

The one area that Kueue can create new machines is its use of Provision requests.
There will be work to make sure HCP works with ProvisionRequests.

##### Infra / Worker Nodes

There should really be no issue with deploying Kueue on Infra or worker nodes.
Kueue is agnostic to this.

##### Metrics

Autoscaling, based on metrics, may be tricky. Kueue will deploy its own metrics 
on the customer workload nodes.
The control plane will not be able to react to these metrics.

##### APIServices

APIService allow one to set different priorities for apiserver.
Kueue has a feature called [VisibilityOnDemand](https://kueue.sigs.k8s.io/docs/tasks/manage/monitor_pending_workloads/pending_workloads_on_demand/#configure-api-priority-and-fairness) that requires one to deploy
an apiservice.

There are [outstanding security issues](https://github.com/kubernetes-sigs/kueue/issues/4433) for this feature so we are disabling this feature in our operator.

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

Cert Manager will be used to manage certificates so our operator will have a hard dependency on Cert Manager.

#### Tech Preview Feature Statement

##### Supported Features

In Tech preview, we will provide the following features:

- [Preemption](https://kueue.sigs.k8s.io/docs/concepts/preemption/)
- Advanced Resource management: Comprising: resource flavor fungibility, fair sharing, cohorts and preemption with a variety of policies between different tenants.
- Support for GPU model servers via pod-integration
- Namespace opt in for frameworks.
- Metrics for Kueue in OCP monitoring.
- [Workload Priority Classes](https://kueue.sigs.k8s.io/docs/concepts/workload_priority_class/)
- LocalQueue, ClusterQueues, Workloads, Worklow Priorites, Admission Checks are all supported.
- Support for deployments, pods, statefulsets.
- [Use of resource flavors to describe heterogeous clusters](https://kueue.sigs.k8s.io/docs/concepts/resource_flavor/)

##### Not Supported

We will not provide the following features in tech preview.

- MultiKueue
- Autoscaling
- TopologyAwareScheduling
- Resource Transformations
- KueueViz
- LocalQueueDefaulting
- Use of ManagedJobsWithoutQueueName
- Fairsharing
- [VisibilityOnDemand](https://kueue.sigs.k8s.io/docs/tasks/manage/monitor_pending_workloads/pending_workloads_on_demand/)
- Gang admission via `WaitForPodsReady` Kueue configurations.
- Topology CRD (Topology Aware Scheduling)
- Cohort CRD (hierachial queueing)

#### GA Feature Statement

We wanted to limit scope for tech preview so we decided to only support what we considered GA.
As new features are requested, we can enable the not supported features in a selective way via feature requests.
New features should be included in the API of the operator so we can selectively enable these features.

#### Release Schedule

| Kueue Operator     |  Stage       |  OCP Version   |  Kueue   | RHOAI GA
| ------------------ | -------      | ---------------| -------- | -------
| 0.1                | Tech Preview | 4-17-4.19      | 0.11.z   | N
| 1.0                | GA           | 4.18-4.20      | 0.12.z   | Y

Kueue releases 6 times a year, roughly. 
They will have 2 releases per k8s version so we can take the latest version that is built with the kubernetes version that OCP comes with.

GAing Kueue on beta APIs will cause support/upgradability issues. 
Our goal is to engage with upstream to drive stability in these APIs before we open Kueue for a general audience.
We are open to work with internal partners on GAing Kueue but upgrades may be a lot harder to guarantee.

Upstream wants to promote [v1beta1 to v1beta2](https://github.com/kubernetes-sigs/kueue/issues/768) in 2025.

[V1 Tracking issue](https://github.com/kubernetes-sigs/kueue/issues/3476)

#### Kueue Release Committment

Kueue has confirmed that they will support n-2 releases with bug fixes and security patches.
These releases will be tested on the supported versions of Kubernetes at the time.
There are presubmits and periodics in test-grid that will provide a [signal](https://testgrid.k8s.io/sig-scheduling) of the release.

#### Release Workflow

The operand will carry the same release branches as Kueue.
These release branches will be forked in openshift and we will run their unit, integration and e2e tests on the supported versions of Kubernetes
that this release branches were tested with in upstream. To best explain this, I think an example is necessary.

In v0.11, Kueue is tested with upstream CI for 1.30-1.31-1.32. We will provide e2e testing of operand for each of these releases.
Openshift will keep testing these versions with this version of Kueue even after upstream drops support for those Kubernetes versions.
This does mean that eventually v0.11 will be not be supported on newer versions of Kubernetes because upstream Kueue has never tested those.

Due to this, we think release branches for operator and operand will be best.

We will follow Kueue upstream branches and carry their patches into these branches. As Kueue is in support,
we will continue to update and release patches for the operator in that release branch.

We will have a release branch in our operator that will correspond to a kueue branch. So 0.1 corresponds to kueue v0.11. 
A patch version of Kueue would be a patch version of the operator (0.1.1 <-> v0.11.1).
Each patch that gets released for Kueue we will then provide a new patch for our operator in that zstream.

Once a new kueue release is out, we will create a release branch for the operator and the operand.
The operator will be bumped to a new minor version and we repeat our process for patches.
A kueue operator bump that changes k8s version means that our OCP skew changes (ie v0.12 is built with OCP 1.33 then we will now support 1.31-1.33) for that release.

It may be possible to stretch the testing farther but we will need to confirm and support each release with testing to determine that.

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

#### RHOAI Integration

RHOAI has an uber operator called OpenDataHub. This operator provides the ability to install
various upstream projects. 
RHOAI provides the flexibility to install Kueue and treat it as a managed project. 

RHOAI considers clusterqueues, localqueues, workloads, resourceflavors, and workloadpriorities as a GA API for customers.

Customers are able to change the configuration of Kueue but that modification is not persisted upon upgrades.
They can either patch the configmap or in an advanced use cases they can provide kustomize to the RHOAI operator to
deploy a custom Kueue installatoin.

The integrations that are GA for RHOAI are Kubeflow Training Operator and Ray.

RHOAI also uses a single release of Kueue across the latest 4 releases of Openshift (ie 4.14, 4.15, 4.16, 4.17).
A request from them for this operator is supporting Kueue across multiple releases of Openshift.

#### Features that require certain Kubernetes versions

MultiKueue depends on the JobManagedBy feature gate in Kubernetes which is beta in 1.32.

ProvisionACC uses ProvisionRequests which are not yet supported in Openshift. 
Kueue uses the v1beta1 API and Openshift will support V1 API in 4.19.
There will be some upstream work needed to have a smarter switch so that 
we can use the V1 API if it exists.

#### No support of alpha features for kueue

To not break existing users, we will not install Custom Resources 
that are alpha. This will be filtered out in the operator.
Only beta APIs and GA APIs will be available for use from Kueue.

#### Feature gates

Moved to open questions.

### Risks and Mitigations

Kueue is a very fast moving community. They release at least 6 times a year.
APIs are all in beta and there is some movement to graduate them. 
But the project does not have a LTS option yet.

To mitigate risk, we are engaging with upstream to define release policies and aim to graduate their critical APIs.

### Drawbacks

Not relevant here.

## Open Questions

### Autoscaling Future

Autoscaling will be a followup enhancement. 
Kueue does not provide much guidance on the safe enablement of autoscaling.
We know that we need a secure way of enabling autoscaler and we should think through that in more detail.

### RHOAI and Kueue Integration

RHOAI is using Kueue as a GA product. 
We still need to figure out the path with RHOAI and OCP Kueue.

A major requirement of RHOAI is that Kueue should be functional across all supported versions of Openshift.
Some features may not work on certain versions of Openshift but 
the core functionality should still be functional.

### Expermental Feature Support

The following is out of scope for tech preview.

Kueue has a concept of feature gates in their configuration API. These are a series of advanced features.
The development of Kueue is quite fast and many of these features are not yet GA. 
We are engaging with upstream to avoid permanent betas and to focus on graduating feature gates.

Meanwhile, there are cases where one would want to use alpha features or beta features.
In an ideal world, we would be able to stop the upgrades of operators if the operator sets alpha features.
This is not possible so one option is for really risky features we can set a feature gate in OCP to force
the cluster to go into tech-preview no upgrade.

### GA timeframe of Kueue

Kueue does have more stable APIs than others.
ClusterQueue, LocalQueue, Workloads, and WorkloadPriorityClasses are more stable.
One suggestion is to GA these APIs as RHOAI has done and get a commitment
from upstream that these APIs will not undergo breaking changes.

## Test Plan

There are three areas we want to increase testing.

Upstream, downstream operand and the operator.

### Upstream

The Kueue Operator will depend on the Cert Manager integration. 
Kueue does not test or use heavily cert manager so we will work with upstream to add tests to
confirm cert manager functionality.

All features/bugs should be implemented and tested in Kueue upstream.

Kueue runs their e2e tests on all supported versions of Kubernetes (n-2) with its two latest releases.

#### Downstream Operand

Kueue operand will need to be tested on all supported versions of Openshift that the operator will support.
We have to change the operand to be compliant with Openshift so we should run the upstream tests with the operand images.

#### Operator

There will be e2e tests testing the installion of the operator based on user provided configurations.
The operator will also test the installation of Kueue and the removing of Kueue based on OLM bundles.
The operator will also run a smoke test to verify that Kueue APIs are accessible and verify basic functionality.

## Graduation Criteria

### Dev Preview -> Tech Preview

- Konflux releases of operator
- E2E Testing
- Operator functionality complete
- Documentation
- Telemetry

### Tech Preview -> GA

- RHOAI is able to switch their Kueue deployment for Openshift Kueue
- Feature parity with their existing solution
- Confidence to support GAish APIs (ClusterQueues, Workloads, LocalQueues, WorkloadPriorityClasses, ResourceFlavors)


### Removing a deprecated feature

This is not relevant.

## Upgrade / Downgrade Strategy

This is still in flight and we are figuring out the upgrade strategy.

The main questions are how many OCP versions can we support with a single Kueue version?

## Version Skew Strategy

Kueue has tight binding with the Kubernetes API so we recommend following Kubernetes policies.

0.11.0 of Kueue is built with 1.32 APIs. 
We would recommend that this Kueue operator work with n-2 (ie 1.30 at the edge).

## Operational Aspects of API Extensions

TODO: Fill this out as we proceed.

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
- Konflux repos onboarding and new tenant in konflux org (kueue-operator-tenant)


