---
title: openshift-kueue
authors:
  - kannon92
reviewers: # Include a comment about what domain expertise a reviewer is expected to bring and what area of the enhancement you expect them to focus on. For example: - "@networkguru, for networking aspects, please look at IP bootstrapping aspect"
  - haircommander 
  - rphillips
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
Openshift AI (RHOAI) uses kueue for AI workloads but we are also seeing requests for autoscaling, multicluster and general batch uses. 
To better support various teams in their exploration or productization of Kueue, it is
important to bring Kueue into the core platform.

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
the label of the pod or workload to add a queue label. 
If Kueue determines the workload to not fit quota, then the workload is either suspended (via `suspend:true` on the workload spec)
or a scheduling gate is added to the pod.

The major challenge of Kueue for Openshift is configuration and support of their various frameworks and these different approaches.

#### RBAC For Kueue

[Kueue Upstream Docs](https://kueue.sigs.k8s.io/docs/tasks/manage/rbac/) cover this pretty well.

Kueue has the concept of a Kueue admin and a kueue user. 

A kueue admin has the ability to create ClusterQueues, LocalQueues, ResourceFlavors and Workloads.

A kueue user has the ability to manage general jobs (batch/jobs, ray, kubeflow etc) and to view LocalQueues and Workloads.
A kueue user would request the admin to create a localqueue in their namespace.

This separation is important because when a user creates a LocalQueue they are able to link to a 
ClusterQueue. ClusterQueues can control things like resource quotas and autoscaling so
LocalQueues are usually the job of a kueue-admin to create in the user namespace.

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
Provisioning Request (abbr. ProvReq) is a new namespaced Custom Resource that aims to allow users to ask Cluster Autoscaler for capacity for groups of pods. 
Kueue provides integration with ProvReq so that if there is quota constraints on the cluster, one could trigger ProvReq to create capacity at the workload level.

Node Autoscaling is working on ProvReq.

A kueue admin can set up a ClusterQueue so that users that have access to that cluster queue via their localqueue could trigger a ProvReq.
This ProvReq would have the cluster autoscaler create nodes that can fit the workload.

This functionality is possible once [ProvRequest in Autoscaling](https://issues.redhat.com/browse/OCPSTRAT-1331) is done.

This will be out of scope for GA.

### Goals

- Provide a Red Hat Certified Operator for Kueue Deployments
- Provide an API so that users can configure Kueue with their use case in mind.
- Kueue will be forked and maintained as [openshift/kubernetes-sig-kueue](https://github.com/openshift/kubernetes-sigs-kueue)
- [KueueOperator](https://github.com/openshift/kueue-operator) will be created to manage the installation and configuration of Kueue 

### Non-Goals

- UX improvements on the Kueue APIs as part of the operator.
    The operator is designed to deploy Kueue so that cluster admins can use the API as is. 
    
- Kueue will be a cluster wide deployed resource. We can only have 1 kueue deployed in the cluster.

- Even though MultiCluster is called out, this will be out of scope for GA.

- Autoscaling will be out of scope for this enhancement.


## Proposal

We will create a Kueue Operator that will manage the installation of Kueue based on a specified Kueue configuration.

Kueue has a series of parameters to configure the operation of Kueue. 

Various use cases call for the configuration of the integrations and other kueue configurations. 

To avoid breaking changes, we are creating an API on top of the Kueue Configuration [API](https://github.com/kubernetes-sigs/kueue/blob/main/apis/config/v1beta1/configuration_types.go).

The operator reads the kueue configuration and generates a ConfigMap that the kueue manager deployment will use. 

The API is displayed below but we have requested an api-review [here](https://github.com/openshift/api/pull/2222).

We have performed an API review in two stages.

Stage 1 - https://github.com/openshift/api/pull/2222

Stage 1 added Integrations as a required field for the operator.

Stage 2 - https://github.com/openshift/api/pull/2250
Stage 2 added support for ManagedJobWithoutQueueName, Fairsharing, and WaitForPodsReady.

The API is defined in those PRs and in our operator.
We will highlight how it can used in this enhancement rather than list out the API and distract the reader with code.
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
- Disconnected
- FIPS

The operator will be OLM managed and hosted on OperatorHub.

### Feature Support

### Workflow Description

#### Batch Job Administrator

As a admin, I want to enable Kueue to manage batch jobs.

```yaml
apiVersion: operator.openshift.io/v1alpha1
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

#### RHOAI Example


Red Hat Openshift AI is already using Kueue in production. Their GA deployment (as of 2.19) can be replicated:

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
    gangScheduling:
      policy: ByWorkload
      byWorkload: 
        admission: Parallel
```

#### IBM Example

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
    gangScheduling:
      policy: Disabled
    workloadManagement:
      labelPolicy: None
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

If kueue is unavailable, there is a potential for workloads to be blocked for admission. 
To mitigate this issue, we use the opt-in namespace approach
so only workloads that are being submitted to a kueue managed namespace
may be blocked until Kueue is available again.
If one is using labelPolicy of QueueName, one could remove
the kueue label and Kueue will not manage that workload.

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

#### GA Feature Statement

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
- Use of ManagedJobsWithoutQueueName
- Gang admission via `WaitForPodsReady` Kueue configurations.
- Fairsharing

##### Not Supported for initial GA

We will not provide the following features for our 1.0 release.

- MultiKueue
- Autoscaling
- TopologyAwareScheduling
- Resource Transformations
- KueueViz
- [VisibilityOnDemand](https://kueue.sigs.k8s.io/docs/tasks/manage/monitor_pending_workloads/pending_workloads_on_demand/)
- Topology CRD (Topology Aware Scheduling)
- Cohort CRD (hierachial queueing)

##### Feature support post GA

Each of these features are valid. We will add support for them as dedicated RFE.
Our focus for initial phase will be the supported features above.

#### Release Schedule


| Kueue Operator     |  Stage       |  OCP Version   |  Kueue   |
| ------------------ | -------      | ---------------| -------- |
| 0.1                | GA Candidate | 4-18-4.19      | 0.11.z   |
| 1.0                | GA           | 4-18-4.20      | 0.11.z   |
| 1.1                | GA           | 4-18-4.20      | 0.12.z   |
| 1.2                | GA           | 4-18-4.20      | 0.14.z   |

Kueue releases 6 times a year, roughly. 
They will have 2 releases per k8s version so we can take the latest version that is built with the kubernetes version that OCP comes with.

There will be no tech preview for this operator and we will go to a GA release.

#### Stability of APIs

A goal will be to engage with upstream to promote APIs to v1 and aim to graduate these APIs to stable.

Upstream wants to promote [v1beta1 to v1beta2](https://github.com/kubernetes-sigs/kueue/issues/768) in 2025.

[V1 Tracking issue](https://github.com/kubernetes-sigs/kueue/issues/3476)

Until there is V1 APIs, there is more maintenance effort here. Due to the interest of Kueue,
this was decided to be worth the effort. Upstream is moving towards stability
in the APIs for ClusterQueue, LocalQueue, ResourceFlavors and Workloads.
We will work closely with the upstream community to manage the risk of breaking changes by reviewing MRs and testing early and often from upstream.

Over time, we aim to add upgrade testing to upstream so that we can catch potential breakage of APIs in upstream.

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

We will have a release branch in our operator that will correspond to a kueue branch. So 1.0 corresponds to kueue v0.11. 
A patch version of Kueue would be a patch version of the operator (1.0.1 <-> v0.11.1).
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

This integration is out of scope for this enhancement.

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

### Notes

During this operator work, it was decided from Product and Business Unit that there is no
reason for a tech preview for this feature.
RHOAI already supports this for GA so our tech preview will be an internal release.
The goal will be to GA this operator in July 2025 timeframe.

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

We have explained this above. Kueue uses webhooks.

## Support Procedures

### Healthy Startup

The operator consists of the operand and the operator.

A healthy operator will deploy the operator and operand deployments. If neither of these are started,
it is recommended to look at the logs of each component. 

Once both deployments are ready, the Kueue APIs are accessible.

## Alternatives (Not Implemented)

None I can think of.

## Infrastructure Needed [optional]

- New repo added for Kueue Operand
- New repo added for kueue operator
- Konflux repos onboarding and new tenant in konflux org (kueue-operator-tenant)


