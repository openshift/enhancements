---
title: update-status-api
authors:
  - "@petr-muller"
reviewers:
  - "@pratikmahajan"    # OTA Team Lead
  - "@wking"            # OTA
  - "@joelspeed"        # API approver
  - "@jupierce"         # Original design author
approvers:
  - "@pratikmahajan"    # OTA Team Lead
  - "@sdodson"          # OCP Architect
api-approvers:
  - "@joelspeed"
creation-date: 2024-10-16
last-updated: 2025-01-22
tracking-link: 
  - https://issues.redhat.com/browse/OTA-1260
  - https://issues.redhat.com/browse/OTA-1339
see-also:
  # Update Health API Controller Proposal
  - https://docs.google.com/document/d/1SuV8mvEQbUMpcV1VTazuPlXgbMeUm2134SIV5so4a5M/edit
  # Update Health API Draft
  - https://docs.google.com/document/d/1aEIWkfhhaVSe-XlSXvmokymOe3X_969pRCJhfhPDwFQ/edit
  # Client-based prototype in 4.16: Easier OpenShift Update Troubleshooting with oc adm upgrade status
  - https://source.redhat.com/groups/public/openshift/openshift_blog/easier_openshift_update_troubleshooting_with_oc_adm_upgrade_status
---

# Update Status API and Controller

## Summary

This enhancement proposes a new API to expose the status and health of the OpenShift cluster update process. The API will be implemented as a new CRD managed by a new controller owned by the OpenShift OTA team. The enhancement aims to deliver the API early, backed by a simple controller, to gather feedback from potential API clients (internal and external) before investing effort into building consensus on a complete, robust, and modular implementation.

## Motivation

The OpenShift update process is complex and often challenging to troubleshoot. It involves a hierarchy of components with distinct responsibility domains and differing interfaces. Administrators need extensive knowledge of resources, conditions, and processes to manage updates across different cluster types and form factors effectively.

There is a need for a centralized API to expose update status and health across different OpenShift tools and interfaces. This API would standardize the interpretation of the update process state, centralize the information provided by various components involved in updates, and integrate with related concepts like pre-checks and maintenance windows. User interfaces enabled by Update Status API will improve the user experience for administrators and support.

Part of the problem is that the way OpenShift works internally does not always correspond well with the established mental models that our users have. An update is usually understood to be a simple transition from one version to another. But in OpenShift, an update is often just a slightly special case of standard Kubernetes distributed system reconciliation, where the specification change that drives the transition happens to be related to a version change, and the underlying controllers do not know about a version update. Therefore, simply exposing the existing state of the system will always create a gap where components cannot inform the users how the update is going because they do not know anything about the update context. Historically, we attempted to teach our users, through support and documentation, to understand OpenShift enough so they can interpret the system state. This enhancement proposes to approach the problem from the other side, moving the responsibility for interpreting the system state to the platform and presenting the system state to the user in a way that more closely relates to established software models.

### User Stories

* As a cluster administrator, I want to easily observe the update process so that I know if the update is going well or if there are issues I need to address.
* As a cluster administrator, I need a single place to observe the update process so that I do not have to dig through different command outputs to understand what is going on.
* As a cluster administrator, I want to be clearly told if there are issues I need to address during the update so that I can do so.
* As a cluster administrator, I only want to be informed about issues relevant to the update and not be overwhelmed with reports that may not be relevant to me so that I can save effort investigating.
* As an engineer building tooling and UXes that interact with OpenShift clusters, I want both status and health information to be available through an API so that I can build software to consume and present it to users.
* As an engineer building tooling and UXes that interact with OpenShift clusters, I want the information available through a designated API so that there is a standardized interpretation of the update process state provided by the platform, and my software does not need to interpret individual component states.

### Goals

* Provide a centralized API to expose cluster update status to consumers.
* Back the API with a simple centralized controller to allow early feedback and drive iterative development.
* Provide features from the 4.16 client-based prototype through an `oc` command consuming the new API
* Design the API to be extensible enough to accommodate individual components contributing information to it in the future.

### Non-Goals

* Achieve consensus on how would the modular backend look like
* Implement the modular backend where different components contribute to the API

## Proposal

Introduce a new `UpdateStatus` API (CRD) that exposes the status and health of the OpenShift cluster update process. In a typical OpenShift cluster, the API will be a singleton with an empty `.spec`, and its purpose will be to expose information through its `status` subresource.

Introduce a new Update Status Controller (USC) component in OpenShift that will directly inspect the cluster state during the update and populate the `UpdateStatus` resource. The functionality of the controller will match the features provided in the client-based prototype delivered in 4.16 and extended in 4.17 and 4.18:

* Monitor the control plane update process and report its progress, completion, duration, estimated finish, and high-level healthiness of the process and cluster operators (cluster operators are treated as a control plane "unit").
* Monitor the node pool update process and report progress, completion, and high-level health assessment of the process and individual nodes.
* Monitor the cluster for potentially problematic states and surface them as relevant and actionable "health insights."

Implement a default `oc adm upgrade status` mode that will consume the API and present the information in a human-targeted format, similar to the existing client-based prototype implementation. This is an example of the output of the prototype (find more in the `*.output` files [here][oc-adm-upgrade-status-examples]), illustrating the value provided by the feature:

```
= Control Plane =
Assessment:      Stalled
Target Version:  4.14.1 (from 4.14.0-rc.3)
Updating:        machine-config
Completion:      97% (32 operators updated, 1 updating, 0 waiting)
Duration:        1h59m (Est. Time Remaining: N/A; estimate duration was 1h24m)
Operator Health: 31 Healthy, 1 Unavailable, 1 Available but degraded

Control Plane Nodes
NAME                                        ASSESSMENT   PHASE     VERSION       EST   MESSAGE
ip-10-0-30-217.us-east-2.compute.internal   Progressing  Draining  4.14.0-rc.3   +10m   
ip-10-0-53-40.us-east-2.compute.internal    Outdated     Pending   4.14.0-rc.3   ?
ip-10-0-92-180.us-east-2.compute.internal   Outdated     Pending   4.14.0-rc.3   ?

= Worker Upgrade =

WORKER POOL   ASSESSMENT   COMPLETION   STATUS
worker        Pending      0% (0/3)     2 Available, 0 Progressing, 0 Draining

Worker Pool Nodes: worker
NAME                                        ASSESSMENT   PHASE     VERSION       EST   MESSAGE
ip-10-0-20-162.us-east-2.compute.internal   Unavailable  Pending   ?             ?     Not ready
ip-10-0-4-159.us-east-2.compute.internal    Outdated     Pending   4.14.0-rc.3   ?
ip-10-0-99-40.us-east-2.compute.internal    Outdated     Pending   4.14.0-rc.3   ?

= Update Health =
SINCE     LEVEL     IMPACT             MESSAGE
58m18s    Error     API Availability   Cluster Operator kube-apiserver is degraded (NodeController_MasterNodesReady)
1h0m17s   Error     API Availability   Cluster Operator control-plane-machine-set is unavailable (UnavailableReplicas)
now       Warning   Update Stalled     Cluster Version version is failing to proceed with the update (ClusterOperatorsDegraded)

Run with --details=health for additional description and links to related online documentation
```

The example illustrates the proposal to translate the OpenShift state closer to a typical administrator's mental model. Nodes flipping their versions from old to new is something that humans expect to see in an update, but assigning a version to a `Node` is not a trivial operation in OpenShift because a version is just one of the metadata items on a configuration that MCO is reconciling on Nodes. For MCO, this is a simple config change to reconcile, not an abstract "update."

All functionality will be delivered under the new `UpdateStatus` [Capability][cluster-capabilities] so that clusters that do not want the functionality do not need to spend resources on running the new controller. The Capability will be a part of the `vCurrent` and `v4.X` capability sets, which means the functionality will be enabled by default, and admins need to opt-out at installation or update time. Like all capabilities, once enabled, it cannot be disabled.

This proposal intends to capture only a limited part of the roadmap for a system reporting update health and status. The [Update Health API Controller Proposal][update-health-api-dd] document describes a potential future system where individual (and possibly external) domain-responsible components would produce update insights into the `UpdateStatus` API. This enhancement proposal corresponds to only Stage 1 of the implementation roadmap outlined there. There has yet to be a consensus on further stages, and this enhancement does not aim to achieve it. Any hypothetical modular system will be a much larger effort to implement, and its design will benefit from experience and feedback obtained by seeing API consumed by users, backed by the proposed simple implementation in the USC.

### Workflow Description

1. On a cluster where the Status API is enabled, the CVO manages a new Update Status Controller (USC) deployment as its operand.
1. The USC monitors the cluster and maintains an `UpdateStatus` resource called `cluster`.
1. While there is no update happening, respective conditions convey:
   1. `.status.controlPlane.conditions` has a `Updating=False` condition.
   1. `.status.workerPools[*].conditions` each have a `Updating=False` condition.
1. When a user runs `oc adm upgrade status`, the `oc` client reads the `UpdateStatus` and reports that no update is happening.
1. The user triggers the update.
1. The USC monitors `ClusterVersion`, `ClusterOperator`, `MachineConfigPool`, and `Node` resources and reflects the state of the update through the `UpdateStatus` resource via a set of status and health insights.
1. When the user runs `oc adm upgrade status`, the client reads the `UpdateStatus` and uses status insights to convey progress and health insights to convey issues the admin needs to address.

### API Extensions

Introduce a new Cluster-scoped `UpdateStatus` CRD to convey update status information. For now, its `.spec` is empty but can be used in the future to configure the desired form or content of the information surfaced in status.

The `status` subresource content is the primary value of the enhancement. The status and health information about the update are expressed through the `status` subresource.

Introduce a new `update.openshift.io` group for `UpdateStatus`. There is no suitable existing OpenShift API group for where `UpdateStatus` would fit. The `ClusterVersion` CRD, which is used to convey some information about the update process, is in the `config.openshift.io` group. `UpdateStatus` provides no configuration capabilities, so `config.openshift.io` is a poor fit. The new group is well-suited to contain some of the other update-related APIs needed for further incoming features like maintenance windows or update pre-checks.

### Topology Considerations

#### Hypershift / Hosted Control Planes

The enhancement allows adoption in HCP as an extension. The API and controller will be extended to handle the resources that represent the control plane and worker pools (`HostedCluster` and `NodePool`, respectively) and surface update progress through status insights for these resources.

The Update Status Controller will run as a part of the hosted control plane. Unlike in Standalone, where it is a CVO operand, USC will be managed by the Control Plane Controller in HCP, where the CVO does not have access to management cluster API and cannot manage resources there. USC needs to access the management cluster API layer to manage the status-reporting API (which targets cluster administrators tasked with updating clusters) and needs to report insights about both the hosted cluster and management cluster resources. It will also need access to the hosted cluster API layer to report on resources present there.

Because `UpdateStatus` API is cluster-scoped, it is not suitable for HCP use, and we will need to introduce a namespaced HCP-specific flavor, such as `HostedClusterUpdateStatus`. HCP-specific API variant will also need to be able to refer to resources in two API surfaces (management and hosted clusters), while the `UpdateStatus` API can rely on all resources being in the same cluster and use simple resource references.

The updates in HCP are not initiated by the [`hcp` client][hcp-client] binary but by [directly modifying `HostedCluster` and `NodePool` resources][updates-in-hcp] using the `oc` client. Hence, at the moment, there is no apparent suitable place to put the update status UX (a client that would consume a `HostedClusterUpdateStatus` instance and present its content). The best place for that is the [`hcp` client][hcp-client], if necessary. It is expected that managed OCP product layers using HCP (ROSA) would consume `HostedClusterUpdateStatus` to improve their update experience, but that work is outside the scope of this enhancement.

When the management cluster is a standalone OCP cluster, there will also be a standard standalone USC instance in the respective platform namespace, managing an `UpdateStatus` that provides information about the management cluster itself.

#### Standalone Clusters

Standalone clusters are the primary target of the functionality and no special considerations are needed.

#### Single-node Deployments or MicroShift

USC and `UpdateStatus` API can work on SNO. USC is updated early in the procedure and will be able to expose the update status when the apiserver is available. USC and `UpdateStatus` can be disabled at installation time through the Capability mechanism for resource-sensitive SNO deployments. Existing SNO clusters using the `vCurrent` capability set would enable USC unless the cluster administrator intervenes by tweaking the capabilities before they update, which may warrant a release note when the feature goes GA.

MicroShift is updated through `rpm-ostree` or `dnf`, so `UpdateStatus` API or any part of USC is not applicable and should not be included in MicroShift.

### Implementation Details/Notes/Constraints

Full API proposal: https://github.com/openshift/api/pull/2012

#### `UpdateStatus` API Overview

```yaml
apiVersion: update.openshift.io/v1alpha1
kind: UpdateStatus
metadata:
  name: cluster
spec: { }
status:
  controlPlane:
    _: ...
    informers:
    - name: cvo-example-informer
      insights: <list of insights reported by the informer>
    - name: mco-example-informer
      insights:
      - type: ClusterVersion # CV status insight
        _: ...
      - type: ClusterOperator # CO status insight
        _: ...
      - type: UpdateHealth # General update health insight
        _: ...
    conditions: <list of standard kubernetes conditions>
  workerPools:
  - name: workers
    _: ...
    informers: <list of informers with reported insights>
    conditions: <list of standard kubernetes conditions>
  - name: infra
    _: ...
  conditions: <list of standard kubernetes conditions>    
```

The API has three conceptual layers:

1. Through the innermost layer `.status.{controlPlane,workerPools[]}.informers`, the API exposes detailed information about individual concerns related to the update, called "Update Insights." The API is prepared to allow multiple external informers to contribute insights, but in this enhancement, the only informer is the USC itself.
1. The aggregation layer `.status.{controlPlane,workerPools[]}` reports higher-level information about the update through this layer, serving as the USC's interpretation of all insights.
1. The outermost layer, `.status.conditions`, is used to report operational matters related to the USC itself (the health of the controller and gathering the insights, not the health of the update process).

We do not expect users to interact with `UpdateStatus` resources directly; the API is intended to be used mainly by tooling. A typical `UpdateStatus` instance is likely to be quite verbose.

#### Update Insights

Update Insights are units of update status/health information. There are two types of update insights: status insights and health insights.

##### Update Status Insights
 
Update Status Insights are directly tied to the update process, regardless of whether it is proceeding smoothly or not. Status Insights expose the status of a single resource that is directly involved in the update process, usually a resource that:

1. Either has a notion of "being updated," such as a `Node` or `ClusterOperator`
1. or represents a higher-level abstraction, such as a `ClusterVersion` resource (represents the control plane) or `MachineConfigPool` (represents a pool of nodes).

Initially, the USC will produce status insights for `ClusterVersion`, `ClusterOperator`, `MachineConfigPool`, and `Node` resources.

##### Update Health Insights

Update Health Insights report a state or condition in the cluster that is abnormal or negative and either affects or is affected by the update. Ideally, none would be generated in a standard healthy update. Health insights should communicate a condition that warrants attention by the cluster administrator and should be actionable. Links to resources helpful in the given situation should accompany health insights. Health insights can reference multiple resources involved in the situation. For example, a health insight exposing a failure to drain a node would reference the `Node`, the `Pod` that fails to be drained, and the `PodDisruptionBudget` that protects it. Individual resources can be involved in multiple insights.

#### Update Status Controller

The Update Status Controller (USC) is a new component in OpenShift that will be deployed directly by the CVO and is being treated as its operand. This is uncommon in OpenShift (listed as a [drawback][this-drawbacks]), as the CVO typically deploys only second-level operators as its operands. In this model, the USC (providing cluster functionality) would normally be an operand, and we would need to invent a new cluster operator to manage it. Because the CVO is directly involved in updates, such an additional layer does not have value, at least for now. If managing the USC shows to be a sufficiently complex problem in practice, we can consider introducing a new operator to manage it.

The Update Status Controller will run a primary controller that will maintain the `UpdateStatus` resource. The resource has no `.spec`, so the controller will ensure the resource exists and its `.status` content is up-to-date and correct. It will determine the `status` subresource content by receiving and processing insights from the other controllers in the USC.

The Update Status Controller will have two additional control loops, both serving as producers of insights for the given domain. One will monitor the control plane and will watch `ClusterVersion` and `ClusterOperator` resources. The other will monitor the node pools and will watch `MachineConfigPool` and `Node` resources. Both will report their observations as status or health insights to the primary controller so it can update the `UpdateStatus` resource. These control loops will reuse the existing cluster check code implemented in the client-side `oc adm upgrade status` prototype.

To avoid inflating OpenShift payload images for all clusters while the feature is in TechPreview, the USC will be delivered in the same image as the CVO. Its code will live in the `openshift/cluster-version-operator` repository, and the USC will be either a separate binary or a subcommand of the CVO binary (the CVO already has subcommands). Prototyping work showed that this delivery method causes issues because the CVO image and manifests inside are treated specially by the code that interacts with them (like hypershift [OCPBUGS-44438](https://issues.redhat.com/browse/OCPBUGS-44438) or release payload build [OCPBUGS-30080](https://issues.redhat.com/browse/OCPBUGS-30080)). We will consider extracting USC into a separate image before the transition to GA, depending on further experience of shipping via CVO, the decision to build a cluster operator for USC, and other tradeoffs, subject to payload inclusion [approvals][becoming-payload].

#### `oc adm upgrade status`

The existing `oc adm upgrade status` command will be changed to consume the `UpdateStatus` resource exclusively from the cluster (doing no further checks) and present its content. The client will gracefully degrade in `UpdateStatus` API absence by falling back to reading `ClusterVersion` conditions and will warn about the absence of `UpdateStatus` API, preventing more details from being provided.

The existing TechPreview client-based functionality will be put behind a dedicated flag and eventually deprecated. While the client-based functionality is present, the plain `oc adm upgrade status` command may hint about using it when it detects that `UpdateStatus` API is not provided by the cluster.

#### Update UX in Web Console

The Web Console presently contains specialized code for update progress presentation, which we hope to replace by simply reading `UpdateStatus` API. Web Console has resource discovery, so when `UpdateStatus` is present, the update progress presentation will read `UpdateStatus` API content instead of the current specialized logic. When `UpdateStatus` is not present, Web Console will gracefully degrade and display minimal information obtained from `ClusterVersion` resource's `Progressing` condition and warn about the disabled `UpdateStatus` capability, preventing more details from being provided. Clusters with enabled Web Console but disabled `UpdateStatus` will have a slightly worse experience than now (where Web Console itself interprets `Node` and `MachineConfigPool` states to show progress bars for node update), but we expect that combination to be rare.

We may consider building a Web Console plugin for cluster update-related matters if the information exposed by `UpdateStatus` API grows sufficiently rich and complex.

### Risks and Mitigations

The proposal to deliver the API and a controller that both manages the API and monitors the cluster (producing insights) before achieving consensus on the eventual modular system architecture risks that the existing API will not accommodate the future architecture well. We are making a tradeoff to deliver the API early to start providing value, which also allows us to learn about how and if such an API is really consumed. Early delivery directly addresses the risk of investing effort into building a much larger system that may not address real user needs.

#### API Size and Footprint

The instances of `UpdateStatus` API, as proposed in https://github.com/openshift/api/pull/2012, are expected to be quite large. A single `UpdateStatus` resource status is a tree of the following shape, with update insight structures as leaves:

```
status:
  # two nodes on this level
  controPlane:
    ...
    informers:
    # one node per informer reporting control plane insights, typically ~2-5
    - name: informer-1
      insights:
      # one leaf per insight
  workerPools:
  # one node per pool
  - name: mcp-1
    informers:
    # one node per informer reporting pool insights, typically ~2-5
    - name: informer-2
      insights
      # one leaf per insight
```

Health insights are not expected to be created in large numbers (like alerts, they are not helpful when too many are reported). A healthy update should have zero. The aspiration for the system is to produce precisely one health insight for a problematic update: the one that clearly describes the problem affecting the update. We are unlikely to achieve this ideal, but a typical problematic update should produce no more than ~10 health insights.

However, there is a single Status Insight for each resource involved in the update, so we can expect `1 + # of ClusterOperators + # of MachineConfigPools + # of Nodes` status insights. That means that for big clusters, there will always be (lower) hundreds of insights, primarily because of Nodes.

The Status Insights are not expected to be large data structures, but some contain Kubernetes conditions, so they cannot be considered entirely trivial, and there can be a high number of them. The Health Insights are larger because they can carry a longer-form description of the issue, expected to be around a paragraph of text.

For an API this large, we need to consider the possibility of hitting Kubernetes API resource limits. Even with too many long insights, hitting the 1.5MiB limit does not seem plausible under regular operation. We can protect against the risk by setting appropriate bounds and limits on API fields (especially lists and the long-form description field of health insights), and these limits will be part of the contract for insight producers.

The size and complexity also impact the readability of the API for humans. This is further discussed in [Drawbacks][this-drawbacks].

### Drawbacks

The pattern of the CVO directly deploying a non-operator component is unusual in OpenShift. We could introduce an entirely new Cluster Operator to manage the USC, but because the update functionality is so closely tied to the CVO, an additional layer seems excessive and unnecessary. Adding this layer can be considered in the future if the proposed model is problematic.

Placing the cluster inspection logic directly into the USC puts the OTA team in a position where we need to maintain the logic while not being experts in a significant part of the domain (interpreting `MachineConfigPool` and `Node` states). This is fine as long as we eventually move the logic to the Machine Config Operator to serve as an insight producer, which depends on the future architecture of the system.

The API-backed `oc adm upgrade status` will lose the ability to run against older clusters that do not have the API (or against non-TechPreview clusters while the feature is still in TechPreview). The feature was requested to be an API from the start, and the client-based prototype was meant to be a temporary solution.

Because of their size, the instances of the `UpdateStatus` API will likely be overwhelming for humans to read. As discussed in [API Size and Footprint][this-api-size] under [Risks][this-risks], the serialized resources will be a lot of YAML for humans to read, so humans would depend on tooling provided in the OpenShift ecosystem to benefit from the feature. Some technical users will likely dislike this because they expect Kubernetes resources to be human-readable. It is not possible to reduce the API verbosity without losing valuable information, but it should be possible to provide a human-oriented counterpart (maybe `UpdateStatusSummary`) that would contain aggregated information only. Such a counterpart could be a separate controller in the USC.

The API consumers need to be aware of the possibility of `UpdateStatus` API being absent because the Capability is not enabled, and they will need to degrade gracefully.

## Open Questions [optional]

* When should the client-based prototype be deprecated and removed?
* What is the bar for the delivery of USC via a new image in the payload? (see note in [Update Status Controller][this-usc])
* What is the best architecture for the future system where the USC only aggregates and summarizes information (possibly provided in the form of Update Insights) from external components that want to contribute update-related information?

## Test Plan

* Tests for the Status API will be added to the `openshift/origin` test suite, covering both update and non-update paths. In non-update paths, we will test that the API correctly reports the cluster as not updating. In update paths, the presence of the expected status insights will be monitored, and their properties checked. We can consider tightening the test suite to fail in the presence of health insights because these signal undesirable conditions during the update.
* USC code will reside in the CVO repository, which has CI jobs for TechPreview installs and both update directions (into the change and out of the change).
* The client `oc adm upgrade status` code that interprets the API to human-readable output will receive similar [integration example-based tests][oc-adm-upgrade-status-examples] as the client-based implementation.
* All code will be appropriately unit-tested.

## Graduation Criteria

### Dev Preview -> Tech Preview

N/A - the `UpdateStatus` feature gate is already Tech Preview

### Tech Preview -> GA

* The API exists and is marked as v1.
* The USC is running in the cluster and maintains the `UpdateStatus` resource in standalone mode.
* The appropriate API support level is decided.
* The `UpdateStatus` capability exists, is enabled by default, and allows admins to opt out through standard mechanisms (pinning a version capability set to one without it or `None`).
* A clear plan to achieve HyperShift support is in place.
* The `oc adm upgrade status` consumes the Status API by default and has at least feature parity with the client-based prototype.
* Meets TRT criteria: e2e tests exist in `openshift/origin`, and a result data corpus proves the feature works and does not lower platform stability.
* Decided whether to extract USC to a separate image or continue to ship with CVO one

### Removing a deprecated feature

The Update Status API replaces the TechPreview client-based behavior from 4.16+ in `oc adm upgrade status`. We will place the client behavior behind a separate flag or command when that happens and announce its deprecation. We will remove the code entirely when the Update Status API reaches GA.

Removing client-based behavior removes some value because it was able to provide information even for clusters with no server-side Update Status API, such as ones running older versions or clusters with the Capability disabled. We may consider keeping it longer to smoothen adoption until enough of the fleet is running the OCP version with API.

## Upgrade / Downgrade Strategy

The USC will be updated by the CVO very early in the update process, immediately after the CVO updates itself. The initial update to a version that first has the feature enabled will result in the installation of the `UpdateStatus` CRD and the USC Deployment, which will then create the `UpdateStatus` singleton CR and start to manage it. Further updates will redeploy the USC, so the CR will be briefly unmanaged, which should not cause issues. The API will still be available, but it may briefly contain stale data.

## Version Skew Strategy

There are two sources of skew:

1. The updated USC needs to monitor resources of potentially old-version CRDs managed by old-version controllers. This should be fine, as CRDs are updated early in the process. The risks here are low because the resources involved in the update are stable, mature types, all in API tier 1, hence unlikely to change significantly to cause issues. The USC must be able to maintain `UpdateStatus` resources from 4.x-1, 4.x, and 4.x+1, but once the feature is GA, the `UpdateStatus` API will also become stable.
1. `oc` client needs to be able to process and display `UpdateStatus` resources for OCP versions following the version skew policy. `oc adm upgrade status` of version 4.x must gracefully handle `UpdateStatus` resources from 4.x-1, 4.x and 4.x+1.

## Operational Aspects of API Extensions

The Update Status Controller will be installed by CVO into a new dedicated `openshift-update-status-controller` namespace. Resources necessary to operate the new controller will be a Deployment, ServiceAccount and RBAC resources to allow the controller to read the necessary state in the cluster (in the initial implementation, watch and read `ClusterVersion`, `ClusterOperator`, `MachineConfigPool`, and `Node` resources) and manage the `UpdateStatus` resource. The USC will be deployed as a single-replica Deployment. The CVO itself will manage this Deployment as its operand.

USC operational matters will be exposed via the `UpdateStatus` resource's `.status` conditions. For the initial implementation, the `Available` condition will suffice. The operational conditions will need to be consumed by the same clients consuming `UpdateStatus` (to assess the trustworthiness of the data), and the user interested in USC operational matters is the same persona as the one interested in the update status: cluster administrator.

We may consider placing a cluster operator layer between CVO and USC in the future for the sake of architecture consistency and to improve the operational reporting layer (use the same concepts as other operators) if necessary, but at the moment, we propose not to introduce this layer, at least while the feature is in Tech Preview. Compliance with the OCP cluster operator pattern can be considered for GA graduation criteria.

### Metrics

For operational purposes, USC will expose simple gauges exposing how many insights are currently known to USC, labeled by source informer and by status/health type.

Some managed service layers above OCP, like OCM, consume information about the cluster in the form of metrics, not by interacting with the cluster API. These layers could benefit from exposing the status information _through_ metrics, but USC is not a good architectural fit for this functionality. USC _manages_ the API by collecting data from informers; it does not interpret it. We could consider making USC contain a separate controller, which serves as an in-cluster _consumer_ of the `UpdateStatus` API, interpreting its state and exposing the update status through metrics. Such effort could benefit from sharing code with the `oc adm upgrade status` command.

## Support Procedures

No specific support procedures are needed for the USC. The `UpdateStatus` resources will be collected by the must-gather tool, which will enable and simplify support because it will be possible to interpret the state of the cluster update process by using the `oc adm upgrade status` command, using a tool such as static-kas.

## Alternatives

### CLI

We could continue improving the `oc adm upgrade status` CLI command we prototyped for 4.16 and extended in 4.17 and 4.18, placing all analysis logic into the `oc` client. This approach even has a significant advantage of being able to run the most recent code against older version clusters. The downside is that without a component continuously running in the cluster, the CLI invocation always only sees the current snapshot of the system state and is unable to implement some desirable features, such as knowing when certain states started or stopped occurring. Additionally, the business case for the feature is to enable multiple UXes (oc, web console, OCM) to report the core platform status/health, so implementing advanced logic in one of the clients would not provide any benefit to the others.

### OLM Operator

USC could be an optional operator delivered via OLM together with the `UpdateStatus` CRD. This means nobody pays any complexity or operational costs unless they explicitly opt in by installing the optional operator. The disadvantage of this approach is that the update is still a core functionality of the platform, performed by platform code through platform-managed resources. To be able to report platform update status/health, either the operator would need to contain all analysis logic (essentially locking the architecture into the state proposed here), or the platform would still need to be modified to expose necessary information.

Explicit opt-in through optional operator installation would likely hamper adoption. Many admins would likely require the feature only after they encounter an issue during the update without previously installing the operator.

Lastly, maintaining such an optional operator would be difficult because it would need to support multiple platform versions somehow. It is also unclear how we would treat form factors such as HyperShift.

### CVO
In standalone OCP, the CVO manages the overall process of updating a cluster and contains some form of status/health reporting through its `ClusterOperator`-like status `Progressing` and `Failing` conditions. The CVO could be extended with the functionality proposed for the USC. However, this would be suboptimal for HyperShift, where the CVO does not manage updates. Additionally, the CVO is a complex, hard-to-maintain component (it is an old-school operator where individual controllers are implemented directly, without utilizing controller library code from library-go), and extending it with new functionality would only increase its complexity.

### Cluster Health API instead of Update Status API

There are requests for a more general Cluster Health reporting system that is not specific to updates. It is currently unknown how such a system would look. One approach would be to invest significant effort into improving the existing status reporting paths in the platform:

* Reporting of the operators
* Reporting of the operands
* Reporting of the managed resources

These reporting paths are currently inconsistent and spread both too wide and deep in the existing system. There is a minimal reporting contract in the form of the `Progressing`/`Failing` conditions on `ClusterOperator` resources, and the platform components publish alerts. For most issues, troubleshooting consists of investigating logs, `status` subresources, events, and metrics. The user must possess the knowledge of where to look and needs to piece together the state. Improving this situation would be beneficial for updates without the need for a dedicated system.

There are three reasons why we are not pursuing this approach now

1. Our users see the update as a special, high-importance operation. From the OCP architecture point of view, it is just a slightly special case of cluster reconciliation (which makes "update" a vaguely defined term -- is MCO rebooting a `Node` after a `MachineConfig` change an "update"? From the MCO point of view, it is no different from any other configuration change. But a typical user would not consider that operation to be an "update" when a version change is not involved). Therefore, we (OpenShift developers) traditionally tended to expose the state of the system in a form that is very close to its architecture model. However, we are receiving feedback that our users' mental model of the system is different and closer to traditional monolithic systems. They expect high-level concepts (like "update" or "control plane") to be reported at this level rather than knowing it is really a distributed system of loosely coupled parts, each of which owns and reports its state.
1. Because we validated the ideas through the client-based prototype, we are confident the features offered by the Status API (and UXes consuming it) are useful and appreciated by the users. The actual business value of the more general system is not entirely clear and would need to be validated. We would need to discover what the users actually want and need and pretty much start from scratch, delaying delivery of the features that we know are useful now. The feedback on Status API concepts can be helpful to inform the design of the cluster health reporting system. For example, we could reuse the concept of insights or the concept of informers based on its success in the Status API.
1. Lastly, the general system would likely lack the notion of "progress" useful for monitoring the update, even if all components are healthy. If we treat the update as a slightly special reconciliation case and nothing more, there is no notion of progress of the high-level concept of "cluster version," just an idea of pending changes of smaller components.

## Infrastructure Needed [optional]

N/A

[cluster-capabilities]: https://docs.openshift.com/container-platform/4.17/installing/overview/cluster-capabilities.html
[update-health-api-dd]: https://docs.google.com/document/d/1aEIWkfhhaVSe-XlSXvmokymOe3X_969pRCJhfhPDwFQ/edit#heading=h.9g05u56hri6y
[oc-adm-upgrade-status-examples]: https://github.com/openshift/oc/tree/master/pkg/cli/admin/upgrade/status/examples
[hcp-client]: https://docs.redhat.com/en/documentation/openshift_container_platform/4.17/html/hosted_control_planes/hosted-control-planes-overview#hcp-versioning-cli_hcp-overview
[updates-in-hcp]: https://docs.redhat.com/en/documentation/openshift_container_platform/4.17/html/hosted_control_planes/updating-hosted-control-planes#hcp-get-upgrade-versions_hcp-updating
[becoming-payload]: https://docs.ci.openshift.org/docs/how-tos/onboarding-a-new-component/#product-builds-and-becoming-part-of-an-openshift-release
[this-usc]: #update-status-controller
[this-drawbacks]: #drawbacks
[this-api-size]: #api-size-and-footprint
[this-risks]: #risks-and-mitigations
